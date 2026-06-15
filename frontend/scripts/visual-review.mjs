#!/usr/bin/env node
// Visual review: drives a real headless Chromium against a running dev/preview
// server, captures every important page at desktop AND mobile, and writes them
// to disk so a reviewer (or another agent) can do pixel-level QA without needing
// Screen Recording permission.
//
// Usage:
//   node scripts/visual-review.mjs                       # default base http://localhost:3001
//   BASE=http://localhost:3000 node scripts/visual-review.mjs
//   OUT=/tmp/my-review node scripts/visual-review.mjs
//
// Auth pages need a buyer token. By default we hit
//   http://localhost:8080/api/v1/auth/login
// with demo-buyer@oasis.test / Oasis1234! (the seeded demo). Override with:
//   AUTH_API=... AUTH_ACCOUNT=... AUTH_PASSWORD=... node scripts/visual-review.mjs
//
// Pass --no-auth to skip auth pages entirely (useful when the backend isn't up).

import { chromium } from "playwright-core";
import { mkdir, writeFile } from "node:fs/promises";

const BASE = process.env.BASE || "http://localhost:3001";
const AUTH_API = process.env.AUTH_API || "http://localhost:8080/api/v1/auth/login";
const ACCOUNT = process.env.AUTH_ACCOUNT || "demo-buyer@oasis.test";
const PASSWORD = process.env.AUTH_PASSWORD || "Oasis1234!";
const OUT = process.env.OUT || "/tmp/oasis-review";
const SKIP_AUTH = process.argv.includes("--no-auth");

// Pages to sweep. `auth` = needs login. `tab` = label of a button to click
// inside the page (used for the /compute tabs).
const PAGES = [
  { name: "home", path: "/" },
  { name: "trust", path: "/trust" },
  { name: "datasets", path: "/datasets" },
  { name: "login", path: "/login" },
  { name: "register", path: "/register" },
  { name: "verify", path: "/verify" },
  { name: "terms", path: "/terms" },
  { name: "privacy", path: "/privacy" },
  { name: "compute-jobs", path: "/compute", auth: true },
  { name: "compute-entitlements", path: "/compute", auth: true, tab: "Entitlements|算力权益" },
  { name: "compute-federated", path: "/compute", auth: true, tab: "Federated|联邦" },
  { name: "compute-psi", path: "/compute", auth: true, tab: "PSI|隐私求交" },
  { name: "compute-algorithms", path: "/compute", auth: true, tab: "Submit algorithm|申请算法" },
  { name: "sell", path: "/sell", auth: true },
  { name: "orders", path: "/orders", auth: true },
  { name: "account", path: "/account", auth: true },
  { name: "earnings", path: "/earnings", auth: true },
  { name: "notifications", path: "/notifications", auth: true },
];

const VIEWPORTS = [
  { name: "desktop", width: 1280, height: 1000 },
  { name: "mobile", width: 390, height: 844 },
];

async function fetchTokens() {
  const r = await fetch(AUTH_API, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ account: ACCOUNT, password: PASSWORD }),
  });
  if (!r.ok) throw new Error(`auth ${r.status}: ${await r.text()}`);
  const body = await r.json();
  const tokens = body?.data?.tokens;
  if (!tokens?.access_token) throw new Error(`auth: no access_token in response`);
  return tokens;
}

async function main() {
  await mkdir(OUT, { recursive: true });

  let tokens = null;
  if (!SKIP_AUTH) {
    try {
      tokens = await fetchTokens();
      console.log(`[auth] obtained tokens for ${ACCOUNT}`);
    } catch (e) {
      console.warn(`[auth] FAILED: ${e.message} — falling back to public pages only`);
    }
  }

  const browser = await chromium.launch();
  const summary = [];

  for (const vp of VIEWPORTS) {
    const ctx = await browser.newContext({ viewport: { width: vp.width, height: vp.height } });
    // Seed localStorage so the app boots already authenticated. Same keys the
    // app's lib/api.ts uses (ACCESS_KEY/REFRESH_KEY).
    if (tokens) {
      await ctx.addInitScript(
        ({ a, r }) => {
          localStorage.setItem("adm_access", a);
          localStorage.setItem("adm_refresh", r);
        },
        { a: tokens.access_token, r: tokens.refresh_token },
      );
    }
    const page = await ctx.newPage();

    for (const p of PAGES) {
      if (p.auth && !tokens) continue;
      const file = `${OUT}/${vp.name}-${p.name}.png`;
      const t0 = Date.now();
      try {
        await page.goto(BASE + p.path, { waitUntil: "networkidle", timeout: 20_000 });
        if (p.tab) {
          // tab label is "zh|en" — try both.
          const pattern = new RegExp(p.tab);
          await page.getByRole("button", { name: pattern }).first().click().catch(() => {});
          await page.waitForTimeout(400);
        }
        await page.screenshot({ path: file, fullPage: true });
        const ms = Date.now() - t0;
        console.log(`${vp.name.padEnd(7)} ${p.name.padEnd(22)} OK (${ms}ms)`);
        summary.push({ viewport: vp.name, page: p.name, file, ms, ok: true });
      } catch (e) {
        console.log(`${vp.name.padEnd(7)} ${p.name.padEnd(22)} FAIL: ${e.message.split("\n")[0]}`);
        summary.push({ viewport: vp.name, page: p.name, file, ok: false, error: e.message });
      }
    }
    await ctx.close();
  }
  await browser.close();
  await writeFile(`${OUT}/summary.json`, JSON.stringify(summary, null, 2));
  const okN = summary.filter((s) => s.ok).length;
  console.log(`\n${okN}/${summary.length} pages captured → ${OUT}`);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
