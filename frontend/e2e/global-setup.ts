import { execFileSync, spawn } from "node:child_process";
import { createServer } from "node:net";
import { appendFileSync, chmodSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { randomUUID } from "node:crypto";

export const INFO_PATH = join(tmpdir(), "oasis-e2e-info.json");
const BACKEND_BASE = "http://127.0.0.1:8080";
const LUMEN_ROOT = process.env.LUMEN_ROOT ?? "/Users/lei/lumen/.worktrees/lumen-production-runtime";
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
  databaseUrl: string; devsqlBin: string; storageDir: string; tempDir: string;
  postgres?: { kind: "native"; dataDir: string } | { kind: "docker"; name: string };
  pids: number[]; logs: string[];
};

export default async function globalSetup() {
  const backendDir = resolve(process.cwd(), "..", "backend");
  const tempDir = mkdtempSync(join(tmpdir(), "oasis-e2e-"));
  const info: HarnessInfo = { databaseUrl: "", devsqlBin: join(tempDir, "devsql"), storageDir: join(tempDir, "objects"), tempDir, pids: [], logs: [] };

  if (process.env.DATABASE_URL) {
    info.databaseUrl = process.env.DATABASE_URL;
  } else if (has("initdb") && has("pg_ctl")) {
    const dataDir = join(tempDir, "postgres");
    const port = await freePort();
    execFileSync("initdb", ["-D", dataDir, "-U", "postgres", "--auth=trust"], { stdio: "ignore" });
    execFileSync("pg_ctl", ["-D", dataDir, "-o", `-p ${port} -h 127.0.0.1`, "-w", "start"], { stdio: "ignore" });
    info.databaseUrl = `postgres://postgres@127.0.0.1:${port}/postgres?sslmode=disable`;
    info.postgres = { kind: "native", dataDir };
  } else if (has("docker")) {
    const port = await freePort();
    const name = `oasis-e2e-pg-${randomUUID().slice(0, 12)}`;
    execFileSync("docker", ["run", "--detach", "--rm", "--name", name, "-e", "POSTGRES_HOST_AUTH_METHOD=trust", "-p", `127.0.0.1:${port}:5432`, "postgres:16-alpine"], { stdio: "ignore" });
    info.databaseUrl = `postgres://postgres@127.0.0.1:${port}/postgres?sslmode=disable`;
    info.postgres = { kind: "docker", name };
    const started = Date.now();
    while (Date.now() - started < 60_000) {
      try { execFileSync("docker", ["exec", name, "pg_isready", "-U", "postgres"], { stdio: "ignore" }); break; }
      catch { await new Promise((resolveWait) => setTimeout(resolveWait, 250)); }
    }
  } else {
    throw new Error("E2E requires DATABASE_URL, initdb/pg_ctl, or Docker");
  }

  const apiBin = join(tempDir, "oasis-api");
  const lumenBin = join(tempDir, "lumen");
  execFileSync("go", ["build", "-o", apiBin, "./cmd/api"], { cwd: backendDir, stdio: "inherit" });
  execFileSync("go", ["build", "-o", info.devsqlBin, "./cmd/devsql"], { cwd: backendDir, stdio: "inherit" });
  execFileSync("go", ["build", "-o", lumenBin, "./cmd/lumen"], { cwd: LUMEN_ROOT, stdio: "inherit" });

  const providerPort = await freePort();
  const providerScript = join(tempDir, "provider.mjs");
  writeFileSync(providerScript, `import http from "node:http";\nhttp.createServer((req,res)=>{if(req.url==="/v1/models"){res.setHeader("content-type","application/json");return res.end(JSON.stringify({data:[{id:"e2e-model"}]}));}let body="";req.on("data",c=>body+=c);req.on("end",()=>{res.writeHead(200,{"content-type":"text/event-stream"});res.write('data: '+JSON.stringify({choices:[{delta:{content:"Controlled E2E response"}}]})+'\\n\\n');res.end('data: [DONE]\\n\\n');});}).listen(${providerPort},"127.0.0.1");\n`);

  const home = join(tempDir, "home");
  const configDir = join(home, ".lumen");
  execFileSync("mkdir", ["-p", configDir, info.storageDir]);
  writeFileSync(join(configDir, "lumen.toml"), `default_model = "e2e-model"\n[[providers]]\nname = "e2e"\nkind = "openai"\nbase_url = "http://127.0.0.1:${providerPort}/v1"\nmodel = "e2e-model"\napi_key = "e2e-key"\n`);
  chmodSync(join(configDir, "lumen.toml"), 0o600);

  const launch = (command: string, args: string[], cwd: string, env: NodeJS.ProcessEnv, label: string) => {
    const log = join(tempDir, `${label}.log`); info.logs.push(log);
    const child = spawn(command, args, { cwd, env, detached: true, stdio: ["ignore", "pipe", "pipe"] });
    const record = (chunk: Buffer) => { try { appendFileSync(log, chunk); } catch {} };
    child.stdout?.on("data", record);
    child.stderr?.on("data", record);
    child.unref(); if (!child.pid) throw new Error(`failed to launch ${label}`); info.pids.push(child.pid); return child;
  };

  launch(process.execPath, [providerScript], tempDir, process.env, "provider");
  const common = { ...process.env, HOME: home, LUMEN_HOSTED: "true", HOSTED_WORKSPACE_ROOT: join(tempDir, "workspaces"), WORKBENCH_JWT_SECRET: WORKBENCH_SECRET, WORKBENCH_DATABASE_URL: info.databaseUrl, WORKBENCH_CONTROL_PLANE_URL: `${BACKEND_BASE}/api/v1/workbench/runtime`, WORKBENCH_RUNTIME_INGEST_SECRET: INGEST_SECRET, WORKBENCH_OBJECT_DIR: info.storageDir };
  launch(apiBin, [], backendDir, { ...process.env, APP_ENV: "test", JWT_SECRET: "oasis-e2e-jwt-secret", WORKBENCH_JWT_SECRET: WORKBENCH_SECRET, WORKBENCH_RUNTIME_INGEST_SECRET: INGEST_SECRET, PAYMENT_PROVIDER: "mock", STORAGE_DRIVER: "local", STORAGE_DIR: info.storageDir, KYC_AUTO_APPROVE: "true", AUTO_MIGRATE: "true", DATABASE_URL: info.databaseUrl }, "backend");
  writeFileSync(INFO_PATH, JSON.stringify(info));
  await waitForReady(`${BACKEND_BASE}/readyz`);
  launch(lumenBin, ["serve", "--addr", "127.0.0.1:8787"], LUMEN_ROOT, common, "code");
  launch(lumenBin, ["science", "lab", "--addr", "127.0.0.1:18992", "--no-browser"], LUMEN_ROOT, common, "lab");
  await Promise.all([waitForReady("http://127.0.0.1:8787/"), waitForReady("http://127.0.0.1:18992/api/lab/health")]);
}
