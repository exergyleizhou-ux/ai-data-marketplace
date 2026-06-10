// auth-churn.js — 20 VU × 3min,登录/通知/刷新循环

import http from "k6/http";
import { check, sleep } from "k6";
import { tokenPool, headers, checkOK, randomItem, BASE } from "./lib.js";

export const options = {
  // register 限流 5/min per IP:连续跑多个脚本时,上一个 setup 可能已耗尽
  // 当前窗口名额,tokenPool 会整窗退避 61s —— 默认 60s setupTimeout 不够。
  setupTimeout: "150s",
  vus: 20,
  duration: "3m",
  thresholds: {
    "http_req_duration{name:login}": ["p(95)<1000"],
    "http_req_duration{name:me}": ["p(95)<500"],
    "http_req_failed": ["rate<0.05"],
  },
};

export function setup() {
  return { tokens: tokenPool(5) }; // register 限流 5/min;5 个号被 20 VU 共享
}

export default function (data) {
  const account = randomItem(data.tokens);

  // 鉴权读热路径(无限流):/users/me + notifications
  let res = http.get(`${BASE}/users/me`, { headers: headers(account.token), tags: { name: "me" } });
  check(res, { "me OK": (r) => checkOK(r, "me") });

  res = http.get(`${BASE}/users/me/notifications`, { headers: headers(account.token), tags: { name: "notifications" } });
  check(res, { "notifications OK": (r) => checkOK(r, "notifications") });

  // refresh 不进循环:refresh token 单次使用(旋转),k6 的 setup 数据是
  // 按 VU 拷贝的,无法跨 VU 同步旋转后的新 token,churn 会制造大量假 401。
  // refresh 路径由后端集成测试覆盖。

  // login:限流 10/min per IP。20 VU / sleep(2) ≈ 10 iter/s,p=0.01 → ~6/min,
  // 留余量不触限;断言真正成功(code===0),不许 4xx 蒙混。
  if (Math.random() < 0.01) {
    res = http.post(`${BASE}/auth/login`, JSON.stringify({
      account: account.account, password: "password123",
    }), { headers: { "Content-Type": "application/json" }, tags: { name: "login" } });
    check(res, { "login OK": (r) => checkOK(r, "login") });
  }

  sleep(2);
}
