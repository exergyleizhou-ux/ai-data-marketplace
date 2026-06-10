import { execFileSync, spawn } from "node:child_process";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// Full-stack E2E harness: stands up a REAL backend against a REAL Postgres, the
// same way the Go test suite does. Locally we spin an ephemeral Postgres via
// initdb/pg_ctl (no Docker here); in CI a service Postgres is provided through
// DATABASE_URL and we skip the initdb dance.
//
// The frontend itself is started by Playwright's `webServer` (see config); its
// default NEXT_PUBLIC_API_BASE_URL already points at :8080, which is where the
// backend binds.

export const INFO_PATH = join(tmpdir(), "oasis-e2e-info.json");
const BACKEND_BASE = "http://localhost:8080";

async function waitForReady(url: string, timeoutMs = 90_000) {
  const start = Date.now();
  let lastErr = "";
  while (Date.now() - start < timeoutMs) {
    try {
      const res = await fetch(url);
      if (res.ok) return;
      lastErr = `status ${res.status}`;
    } catch (e) {
      lastErr = String(e);
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`backend not ready at ${url} after ${timeoutMs}ms (last: ${lastErr})`);
}

export default async function globalSetup() {
  const backendDir = join(process.cwd(), "..", "backend");
  const info: {
    databaseUrl: string;
    startedPg: boolean;
    pgDir?: string;
    sockDir?: string;
    storageDir: string;
    devsqlBin: string;
    backendPid?: number;
  } = { databaseUrl: "", startedPg: false, storageDir: "", devsqlBin: "" };

  // 1. Database: reuse a CI-provided one, else spin ephemeral Postgres.
  if (process.env.DATABASE_URL) {
    info.databaseUrl = process.env.DATABASE_URL;
  } else {
    const pgDir = mkdtempSync(join(tmpdir(), "oasis-e2e-pg-"));
    const sockDir = mkdtempSync(join(tmpdir(), "oasis-e2e-sock-"));
    const port = "55600";
    execFileSync("initdb", ["-D", pgDir, "-U", "postgres", "--auth=trust"], { stdio: "ignore" });
    execFileSync(
      "pg_ctl",
      ["-D", pgDir, "-o", `-p ${port} -k ${sockDir} -c listen_addresses=''`, "-w", "start"],
      { stdio: "ignore" },
    );
    info.databaseUrl = `postgres://postgres@/postgres?host=${sockDir}&port=${port}&sslmode=disable`;
    info.startedPg = true;
    info.pgDir = pgDir;
    info.sockDir = sockDir;
  }

  // 2. Build the backend + the devsql seed helper once (fast to re-invoke later).
  const apiBin = join(tmpdir(), "oasis-e2e-api");
  info.devsqlBin = join(tmpdir(), "oasis-e2e-devsql");
  execFileSync("go", ["build", "-o", apiBin, "./cmd/api"], { cwd: backendDir, stdio: "inherit" });
  execFileSync("go", ["build", "-o", info.devsqlBin, "./cmd/devsql"], { cwd: backendDir, stdio: "inherit" });

  // 3. Launch the backend (mock payments, auto-migrate, KYC auto-approve).
  info.storageDir = mkdtempSync(join(tmpdir(), "oasis-e2e-store-"));
  const backend = spawn(apiBin, [], {
    cwd: backendDir,
    env: {
      ...process.env,
      APP_ENV: "test",
      JWT_SECRET: "e2e-secret-please-change",
      PAYMENT_PROVIDER: "mock",
      STORAGE_DRIVER: "local",
      STORAGE_DIR: info.storageDir,
      KYC_AUTO_APPROVE: "true",
      AUTO_MIGRATE: "true",
      DATABASE_URL: info.databaseUrl,
    },
    stdio: "ignore",
    detached: true,
  });
  backend.unref();
  info.backendPid = backend.pid;

  writeFileSync(INFO_PATH, JSON.stringify(info));

  // 4. Gate on the readiness probe (pings the DB) before any test runs.
  await waitForReady(`${BACKEND_BASE}/readyz`);
}
