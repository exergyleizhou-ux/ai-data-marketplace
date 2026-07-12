import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, rmSync } from "node:fs";
import { INFO_PATH, type HarnessInfo } from "./global-setup";

export default async function globalTeardown() {
  if (!existsSync(INFO_PATH)) return;
  const info = JSON.parse(readFileSync(INFO_PATH, "utf8")) as HarnessInfo;
  for (const pid of [...info.pids].reverse()) {
    try { process.kill(-pid, "SIGTERM"); } catch { try { process.kill(pid, "SIGTERM"); } catch {} }
  }
  await new Promise(resolve => setTimeout(resolve, 500));
  for (const pid of [...info.pids].reverse()) {
    try { process.kill(-pid, "SIGKILL"); } catch { try { process.kill(pid, "SIGKILL"); } catch {} }
  }
  if (info.postgres?.kind === "native") {
    try { execFileSync("pg_ctl", ["-D", info.postgres.dataDir, "stop", "-m", "fast"], { stdio: "ignore" }); } catch {}
  } else if (info.postgres?.kind === "docker") {
    try { execFileSync("docker", ["rm", "-f", info.postgres.name], { stdio: "ignore" }); } catch {}
  }
  if (process.env.E2E_KEEP_ARTIFACTS !== "1") rmSync(info.tempDir, { recursive: true, force: true });
  rmSync(INFO_PATH, { force: true });
}
