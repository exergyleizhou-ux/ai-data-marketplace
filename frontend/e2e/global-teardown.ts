import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, rmSync } from "node:fs";
import { INFO_PATH, LOCK_PATH, type HarnessInfo } from "./global-setup";

function processTable() {
  const rows = execFileSync("ps", ["-axo", "pid=,ppid=,command="], { encoding: "utf8" });
  return rows.split("\n").flatMap(line => {
    const match = line.trim().match(/^(\d+)\s+(\d+)\s+(.*)$/);
    return match ? [{ pid: Number(match[1]), ppid: Number(match[2]), command: match[3] }] : [];
  });
}

function descendants(roots: number[]) {
  const table = processTable(); const found = new Set(roots);
  let changed = true;
  while (changed) { changed = false; for (const row of table) if (found.has(row.ppid) && !found.has(row.pid)) { found.add(row.pid); changed = true; } }
  return [...found].reverse();
}

function alive(pid: number) { try { process.kill(pid, 0); return true; } catch { return false; } }

export default async function globalTeardown() {
  if (!existsSync(INFO_PATH)) { rmSync(LOCK_PATH, { force: true }); return; }
  const info = JSON.parse(readFileSync(INFO_PATH, "utf8")) as HarnessInfo;
  const owned = descendants(info.pids);
  for (const pid of owned) { try { process.kill(pid, "SIGTERM"); } catch {} }
  for (const pid of [...info.pids].reverse()) { try { process.kill(-pid, "SIGTERM"); } catch {} }
  const termDeadline = Date.now() + 3000;
  while (Date.now() < termDeadline && owned.some(alive)) await new Promise(resolve => setTimeout(resolve, 50));
  for (const pid of owned) if (alive(pid)) { try { process.kill(pid, "SIGKILL"); } catch {} }
  for (const pid of [...info.pids].reverse()) { try { process.kill(-pid, "SIGKILL"); } catch {} }
  const killDeadline = Date.now() + 3000;
  while (Date.now() < killDeadline && owned.some(alive)) await new Promise(resolve => setTimeout(resolve, 50));
  if (info.postgres?.kind === "native") {
    try { execFileSync("pg_ctl", ["-D", info.postgres.dataDir, "stop", "-m", "fast"], { stdio: "ignore" }); } catch {}
  } else if (info.postgres?.kind === "docker") {
    try { execFileSync("docker", ["rm", "-f", info.postgres.name], { stdio: "ignore" }); } catch {}
  }
  const survivors = processTable().filter(row => row.command.includes(info.tempDir));
  if (survivors.length || owned.some(alive)) throw new Error(`E2E teardown left owned processes: ${JSON.stringify(survivors)}`);
  if (process.env.E2E_KEEP_ARTIFACTS !== "1") rmSync(info.tempDir, { recursive: true, force: true });
  rmSync(INFO_PATH, { force: true });
  rmSync(LOCK_PATH, { force: true });
}
