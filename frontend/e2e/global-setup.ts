import { execFileSync, spawn } from "node:child_process";
import { createServer } from "node:net";
import { appendFileSync, chmodSync, existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { randomUUID } from "node:crypto";

const RUN_ID = process.env.E2E_RUN_ID;
if (!RUN_ID || !/^[a-f0-9-]{36}$/.test(RUN_ID)) throw new Error("E2E_RUN_ID is required");
export const INFO_PATH = join(tmpdir(), `oasis-e2e-${RUN_ID}-info.json`);
export const LOCK_PATH = join(tmpdir(), "oasis-e2e-harness.lock");
const FRONTEND_PORT = Number(process.env.E2E_FRONTEND_PORT);
const BACKEND_PORT = Number(process.env.E2E_BACKEND_PORT);
const BACKEND_BASE = `http://127.0.0.1:${BACKEND_PORT}`;
const LUMEN_ROOT = process.env.E2E_LUMEN_ROOT ?? "/Users/lei/lumen/.worktrees/lumen-production-runtime";
const WORKBENCH_SECRET = "oasis-e2e-workbench-secret-at-least-32-bytes";
const INGEST_SECRET = "oasis-e2e-ingest-secret-at-least-32-bytes";

async function freePort() {
  return await new Promise<number>((resolvePort, reject) => {
    const server = createServer();
    server.unref();
    server.on("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (!address || typeof address === "string") return reject(new Error("could not reserve port"));
      const port = address.port;
      server.close(() => resolvePort(port));
    });
  });
}

async function requireFreePort(port: number) {
  await new Promise<void>((resolvePort, reject) => {
    const server = createServer();
    server.once("error", () => reject(new Error(`E2E fixed port ${port} is already owned; refusing stale-service reuse`)));
    server.listen(port, "127.0.0.1", () => server.close(() => resolvePort()));
  });
}

async function waitForReady(url: string, timeoutMs = 120_000) {
  const start = Date.now();
  let last = "";
  while (Date.now() - start < timeoutMs) {
    try { const response = await fetch(url); if (response.ok) return; last = `HTTP ${response.status}`; }
    catch (error) { last = String(error); }
    await new Promise((resolveWait) => setTimeout(resolveWait, 250));
  }
  throw new Error(`${url} was not ready after ${timeoutMs}ms (${last})`);
}

function has(command: string) {
  try { execFileSync("sh", ["-c", `command -v ${command}`], { stdio: "ignore" }); return true; }
  catch { return false; }
}

export type HarnessInfo = {
  databaseUrl: string; lumenDatabaseUrl: string; backendBase: string; devsqlBin: string; storageDir: string; tempDir: string;
  postgres?: { kind: "native"; dataDir: string } | { kind: "docker"; name: string };
  pids: number[]; logs: string[];
};

export default async function globalSetup() {
  if (existsSync(LOCK_PATH)) {
    const owner = Number(readFileSync(LOCK_PATH, "utf8"));
    try { process.kill(owner, 0); throw new Error(`another E2E harness is active (pid ${owner})`); }
    catch (error) { if (error instanceof Error && error.message.startsWith("another E2E")) throw error; rmSync(LOCK_PATH, { force: true }); }
  }
  writeFileSync(LOCK_PATH, String(process.pid), { flag: "wx" });
  await Promise.all([requireFreePort(BACKEND_PORT), requireFreePort(FRONTEND_PORT)]);
  const backendDir = resolve(process.cwd(), "..", "backend");
  const tempDir = mkdtempSync(join(tmpdir(), "oasis-e2e-"));
  const info: HarnessInfo = { databaseUrl: "", lumenDatabaseUrl: "", backendBase: BACKEND_BASE, devsqlBin: join(tempDir, "devsql"), storageDir: join(tempDir, "objects"), tempDir, pids: [], logs: [] };

  if (process.env.DATABASE_URL) {
    info.databaseUrl = process.env.DATABASE_URL;
    info.lumenDatabaseUrl = process.env.LUMEN_DATABASE_URL ?? process.env.DATABASE_URL;
  } else if (has("initdb") && has("pg_ctl")) {
    const dataDir = join(tempDir, "postgres");
    const port = await freePort();
    execFileSync("initdb", ["-D", dataDir, "-U", "postgres", "--auth=trust"], { stdio: "ignore" });
    execFileSync("pg_ctl", ["-D", dataDir, "-o", `-p ${port} -h 127.0.0.1`, "-w", "start"], { stdio: "ignore" });
    info.databaseUrl = `postgres://postgres@127.0.0.1:${port}/postgres?sslmode=disable`;
    info.lumenDatabaseUrl = info.databaseUrl;
    info.postgres = { kind: "native", dataDir };
  } else if (has("docker")) {
    const port = await freePort();
    const name = `oasis-e2e-pg-${randomUUID().slice(0, 12)}`;
    execFileSync("docker", ["run", "--detach", "--rm", "--name", name, "-e", "POSTGRES_HOST_AUTH_METHOD=trust", "-p", `127.0.0.1:${port}:5432`, "postgres:16-alpine"], { stdio: "ignore" });
    info.databaseUrl = `postgres://postgres@127.0.0.1:${port}/postgres?sslmode=disable`;
    info.postgres = { kind: "docker", name };
    const started = Date.now();
    let created = false;
    while (Date.now() - started < 60_000) {
      try {
        // The image briefly accepts connections during initdb and then restarts;
        // require the final server to remain ready across two probes.
        execFileSync("docker", ["exec", name, "pg_isready", "-U", "postgres"], { stdio: "ignore" });
        await new Promise((resolveWait) => setTimeout(resolveWait, 500));
        execFileSync("docker", ["exec", name, "pg_isready", "-U", "postgres"], { stdio: "ignore" });
        created = true;
        break;
      } catch { await new Promise((resolveWait) => setTimeout(resolveWait, 250)); }
    }
    if (!created) throw new Error("Docker Postgres did not become ready");
    info.lumenDatabaseUrl = info.databaseUrl;
  } else {
    throw new Error("E2E requires DATABASE_URL, initdb/pg_ctl, or Docker");
  }

  const apiBin = join(tempDir, "oasis-api");
  const lumenBin = join(tempDir, "lumen");
  const codePort = await freePort();
  const labPort = await freePort();
  execFileSync("go", ["build", "-o", apiBin, "./cmd/api"], { cwd: backendDir, stdio: "inherit" });
  execFileSync("go", ["build", "-o", info.devsqlBin, "./cmd/devsql"], { cwd: backendDir, stdio: "inherit" });
  execFileSync("go", ["build", "-o", lumenBin, "./cmd/lumen"], { cwd: LUMEN_ROOT, stdio: "inherit" });

  const providerPort = await freePort();
  const providerScript = join(tempDir, "provider.mjs");
  writeFileSync(providerScript, `import http from "node:http";
const send=(res,value,finish)=>{res.write('data: '+JSON.stringify({choices:[{delta:value,...(finish?{finish_reason:finish}:{})}]})+'\\n\\n')};
http.createServer((req,res)=>{if(req.url==="/v1/models"){res.setHeader("content-type","application/json");return res.end(JSON.stringify({data:[{id:"e2e-model"}]}));}let raw="";req.on("data",c=>raw+=c);req.on("end",()=>{res.writeHead(200,{"content-type":"text/event-stream"});let body={};try{body=JSON.parse(raw)}catch{};const messages=body.messages||[];const latest=[...messages].reverse().find(m=>m.role==="user"&&!String(m.content||"").startsWith("⚠ verify failed"));const text=String(latest?.content||"");if(text.includes("E2E_CANCEL")){return setTimeout(()=>{send(res,{content:"This response should have been canceled."});res.end('data: [DONE]\\n\\n')},30000)}const usedTool=messages.slice(messages.lastIndexOf(latest)+1).some(m=>m.role==="tool");if(usedTool){send(res,{content:"Controlled task completed and verified."});return res.end('data: [DONE]\\n\\n')};let name="",args={};if(text.includes("E2E_CODE_FIX")){name="write_file";args={path:"main.go",content:"package main\\n\\nfunc main() {}\\n"}}else if(text.includes("E2E_CODE_INVALID")){name="write_file";args={path:"main.go",content:"package main\\nfunc main( {\\n"}}else if(text.includes("E2E_LAB_REPORT")){name="write_file";args={path:"reports/result.md",content:"# Controlled Lab Report\\n\\nEvidence-backed conclusion.\\n"}}else if(text.includes("E2E_APPROVAL")){name="bash";args={command:"touch rejected-side-effect.txt"}};if(name){send(res,{tool_calls:[{index:0,id:"e2e-tool",type:"function",function:{name,arguments:JSON.stringify(args)}}]});send(res,{},"tool_calls");return res.end('data: [DONE]\\n\\n')};send(res,{content:"Controlled E2E response"});res.end('data: [DONE]\\n\\n');});}).listen(${providerPort},"127.0.0.1");
`);

  const home = join(tempDir, "home");
  const configDir = join(home, ".lumen");
  const xdgConfigHome = join(home, ".config");
  const linuxConfigDir = join(xdgConfigHome, "lumen");
  const macConfigDir = join(home, "Library", "Application Support", "lumen");
  execFileSync("mkdir", ["-p", configDir, linuxConfigDir, macConfigDir, info.storageDir]);
  const lumenConfig = `default_model = "e2e-model"\n[[providers]]\nname = "e2e"\nkind = "openai"\nbase_url = "http://127.0.0.1:${providerPort}/v1"\nmodel = "e2e-model"\napi_key = "e2e-key"\n`;
  writeFileSync(join(configDir, "lumen.toml"), lumenConfig);
  writeFileSync(join(linuxConfigDir, "lumen.toml"), lumenConfig);
  writeFileSync(join(macConfigDir, "lumen.toml"), lumenConfig);
  chmodSync(join(configDir, "lumen.toml"), 0o600);
  chmodSync(join(linuxConfigDir, "lumen.toml"), 0o600);
  chmodSync(join(macConfigDir, "lumen.toml"), 0o600);

  const launch = (command: string, args: string[], cwd: string, env: NodeJS.ProcessEnv, label: string) => {
    const log = join(tempDir, `${label}.log`); info.logs.push(log);
    const child = spawn(command, args, { cwd, env, detached: true, stdio: ["ignore", "pipe", "pipe"] });
    const record = (chunk: Buffer) => { try { appendFileSync(log, chunk); } catch {} };
    child.stdout?.on("data", record);
    child.stderr?.on("data", record);
    child.unref(); if (!child.pid) throw new Error(`failed to launch ${label}`); info.pids.push(child.pid); writeFileSync(INFO_PATH, JSON.stringify(info)); return child;
  };

  launch(process.execPath, [providerScript], tempDir, process.env, "provider");
  const lumenDatabase = new URL(info.lumenDatabaseUrl); lumenDatabase.searchParams.set("application_name", `lumen-e2e-${randomUUID().slice(0, 8)}`);
  const common = { ...process.env, HOME: home, XDG_CONFIG_HOME: xdgConfigHome, LUMEN_HOSTED: "true", HOSTED_WORKSPACE_ROOT: join(tempDir, "workspaces"), WORKBENCH_JWT_SECRET: WORKBENCH_SECRET, WORKBENCH_DATABASE_URL: lumenDatabase.toString(), WORKBENCH_CONTROL_PLANE_URL: BACKEND_BASE, WORKBENCH_RUNTIME_INGEST_SECRET: INGEST_SECRET, WORKBENCH_OBJECT_DIR: info.storageDir, E2E_LUMEN_BUILD_MARKER: `worktree-${randomUUID()}` };
  // Lumen's config loader consults the parent process environment while
  // resolving its dotenv before command dispatch; keep both views identical.
  process.env.WORKBENCH_CONTROL_PLANE_URL = BACKEND_BASE;
  process.env.WORKBENCH_DATABASE_URL = info.lumenDatabaseUrl;
  launch(apiBin, [], backendDir, { ...process.env, HTTP_ADDR: `127.0.0.1:${BACKEND_PORT}`, APP_ENV: "test", JWT_SECRET: "oasis-e2e-jwt-secret", WORKBENCH_JWT_SECRET: WORKBENCH_SECRET, WORKBENCH_RUNTIME_INGEST_SECRET: INGEST_SECRET, PAYMENT_PROVIDER: "mock", STORAGE_DRIVER: "local", STORAGE_DIR: info.storageDir, KYC_AUTO_APPROVE: "true", AUTO_MIGRATE: "true", DATABASE_URL: info.databaseUrl }, "backend");
  await waitForReady(`${BACKEND_BASE}/readyz`);
  launch(lumenBin, ["serve", "--addr", `127.0.0.1:${codePort}`], tempDir, common, "code");
  launch(lumenBin, ["science", "lab", "--addr", `127.0.0.1:${labPort}`, "--no-browser"], tempDir, common, "lab");
  try {
    await Promise.all([waitForReady(`http://127.0.0.1:${codePort}/`), waitForReady(`http://127.0.0.1:${labPort}/api/lab/health`)]);
  } catch (error) {
    for (const log of info.logs) {
      try { console.error(`\n--- ${log} ---\n${readFileSync(log, "utf8")}`); } catch {}
    }
    throw error;
  }
  execFileSync("npm", ["run", "build"], { cwd: process.cwd(), env: { ...process.env, E2E_ALLOW_HTTP: "1", NEXT_OUTPUT_STANDALONE: "0", BACKEND_API_BASE_URL: `${BACKEND_BASE}/api/v1`, LUMEN_SERVE_URL: `http://127.0.0.1:${codePort}`, LUMEN_LAB_URL: `http://127.0.0.1:${labPort}` }, stdio: "inherit" });
  launch(process.execPath, [join(process.cwd(), "node_modules", "next", "dist", "bin", "next"), "start", "-p", String(FRONTEND_PORT)], process.cwd(), { ...process.env, E2E_ALLOW_HTTP: "1", BACKEND_API_BASE_URL: `${BACKEND_BASE}/api/v1`, LUMEN_SERVE_URL: `http://127.0.0.1:${codePort}`, LUMEN_LAB_URL: `http://127.0.0.1:${labPort}`, LUMEN_PROVIDER_CONFIGURED: "true", WORKBENCH_DATABASE_URL: "e2e", STORAGE_DRIVER: "local", COMPUTE_RUNNER: "controlled" }, "frontend");
  await waitForReady(`http://127.0.0.1:${FRONTEND_PORT}/`);
}
