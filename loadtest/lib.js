// lib.js — k6 共享工具函数

import http from "k6/http";
import { sleep } from "k6";

const BASE = __ENV.BASE_URL || "http://localhost:8080/api/v1";

// ---- 在 setup() 里调用,返回 token 数组 ----
export function tokenPool(num) {
  // register 限流 5/min per IP:一个窗口最多放 5 个号。超过 5 必须整窗等待
  // (61s/批),调用方需相应调大 options.setupTimeout。默认就用 ≤5 的池子,
  // 多 VU 共享账号(randomItem)对只读/混合路径完全够用。
  const tokens = [];
  for (let i = 0; i < num; i++) {
    if (i > 0 && i % 5 === 0) sleep(61); // 下一个限流窗口
    const suffix = Math.random().toString(36).slice(2, 8);
    const account = `lt-${suffix}@load.test`;
    const regRes = http.post(`${BASE}/auth/register`, JSON.stringify({
      account, account_type: "email", password: "password123",
    }), { headers: { "Content-Type": "application/json" } });
    if (regRes.status === 429) { i--; sleep(61); continue; } // 整窗退避,不空转
    if (!checkOK(regRes, "register")) throw new Error(`setup: register failed (${regRes.status})`);
    tokens.push({
      account,
      token: regRes.json("data.tokens.access_token"),
      refresh: regRes.json("data.tokens.refresh_token"),
    });
  }
  return tokens;
}

// ---- 统一 headers ----
export function headers(token) {
  return {
    "Content-Type": "application/json",
    "Authorization": `Bearer ${token}`,
  };
}

// ---- 严格检查:envelope code === 0 才算通过 ----
// (一刀切放过 4xx 会让真故障隐身:鉴权配错 → 全 401 → 全"绿"。
//  预期 4xx 用 expectStatus 显式声明。)
export function checkOK(res, label) {
  let body;
  try { body = res.json(); } catch (_) { body = undefined; }
  const ok = body !== undefined && body !== null && body.code === 0;
  if (!ok) console.error(`${label}: status=${res.status} code=${body && body.code} msg=${body && body.message}`);
  return ok;
}

// ---- 显式预期某些状态码(如限流 429、未授权 401)----
export function expectStatus(res, allowed, label) {
  const ok = allowed.indexOf(res.status) !== -1;
  if (!ok) console.error(`${label}: status=${res.status}, want one of ${allowed}`);
  return ok;
}

// ---- 随机取一个元素 ----
export function randomItem(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}

export { BASE };
