import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, rmSync } from "node:fs";
import { INFO_PATH } from "./global-setup";

// Tear down everything global-setup started: the backend process, and (locally)
// the ephemeral Postgres + its temp dirs. Best-effort; never throws.
export default async function globalTeardown() {
  if (!existsSync(INFO_PATH)) return;
  const info = JSON.parse(readFileSync(INFO_PATH, "utf8"));

  if (info.backendPid) {
    try {
      process.kill(info.backendPid, "SIGTERM");
    } catch {
      /* already gone */
    }
  }

  if (info.startedPg && info.pgDir) {
    try {
      execFileSync("pg_ctl", ["-D", info.pgDir, "stop", "-m", "fast"], { stdio: "ignore" });
    } catch {
      /* ignore */
    }
    for (const dir of [info.pgDir, info.sockDir, info.storageDir]) {
      if (dir) {
        try {
          rmSync(dir, { recursive: true, force: true });
        } catch {
          /* ignore */
        }
      }
    }
  }

  try {
    rmSync(INFO_PATH, { force: true });
  } catch {
    /* ignore */
  }
}
