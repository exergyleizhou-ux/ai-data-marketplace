import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { randomUUID } from "node:crypto";
import { INFO_PATH } from "./global-setup";

function info() {
  return JSON.parse(readFileSync(INFO_PATH, "utf8")) as { databaseUrl: string; backendBase: string; devsqlBin: string };
}

// Run one or more raw SQL statements via the backend's cmd/devsql helper (which
// refuses to run under APP_ENV=production). Values here are controlled test
// data, not user input. Mirrors how the Go E2E suite seeds published datasets.
export function devsql(...statements: string[]) {
  const { databaseUrl, devsqlBin } = info();
  execFileSync(devsqlBin, statements, {
    env: { ...process.env, APP_ENV: "development", DATABASE_URL: databaseUrl },
    stdio: "pipe",
  });
}

export function seedVerifiedKyc(userId: string) {
  devsql(`UPDATE users SET kyc_status='verified' WHERE id='${userId}'`);
}

// Seed a published dataset + version exactly like TestE2E_FullPurchaseJourney,
// then give it a real price so the order carries a non-zero amount.
export function seedPublishedDataset(opts: { id: string; sellerId: string; title: string }) {
  const verId = randomUUID();
  devsql(
    `INSERT INTO datasets (id, seller_id, title, description, data_type, license_type, status, created_at, updated_at)
     VALUES ('${opts.id}','${opts.sellerId}','${opts.title}','Seeded for browser E2E','text','commercial','published', now(), now())`,
    `INSERT INTO dataset_versions (id, dataset_id, version_no, manifest, created_at)
     VALUES ('${verId}','${opts.id}',1,'[]', now())`,
    `UPDATE datasets SET current_version_id='${verId}', final_price_cents=9900, suggested_price_cents=9900 WHERE id='${opts.id}'`,
  );
}

export type RegisteredUser = { id: string; account: string; access: string; refresh: string };

// Register a fresh user through the real API and return their id + tokens.
export async function apiRegister(prefix: string): Promise<RegisteredUser> {
  const account = `${prefix}-${randomUUID().slice(0, 8)}@e2e.local`;
  const octet = 10 + Math.floor(Math.random() * 220);
  const res = await fetch(`${info().backendBase}/api/v1/auth/register`, {
    method: "POST",
    // The real edge assigns a client IP. Give each isolated E2E actor one too,
    // so repeat runs exercise rate limiting without sharing a synthetic peer.
    headers: { "Content-Type": "application/json", "X-Forwarded-For": `198.51.100.${octet}` },
    body: JSON.stringify({ account, password: "password123", account_type: "email" }),
  });
  const json = await res.json();
  if (!res.ok || json.code !== 0) {
    throw new Error(`register failed: ${res.status} ${JSON.stringify(json)}`);
  }
  return {
    id: json.data.user.id,
    account,
    access: json.data.tokens.access_token,
    refresh: json.data.tokens.refresh_token,
  };
}
