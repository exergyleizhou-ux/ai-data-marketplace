// soak.js — 10 VU × 30min 混合负载(80% 浏览 20% auth),验证无内存泄漏

import http from "k6/http";
import { check, sleep } from "k6";
import { tokenPool, headers, checkOK, randomItem, BASE } from "./lib.js";

export const options = {
  // register 限流 5/min per IP:连续跑多个脚本时,上一个 setup 可能已耗尽
  // 当前窗口名额,tokenPool 会整窗退避 61s —— 默认 60s setupTimeout 不够。
  setupTimeout: "150s",
  vus: 10,
  duration: "30m",
  thresholds: {
    "http_req_duration": ["p(95)<2000"],
    "http_req_failed": ["rate<0.05"],
  },
};

export function setup() {
  return { tokens: tokenPool(5) };
}

export default function (data) {
  const tok = randomItem(data.tokens).token;

  if (Math.random() < 0.8) {
    // 80% 浏览路径
    let res = http.get(`${BASE}/datasets?limit=10`, { headers: headers(tok) });
    check(res, { "browse OK": (r) => checkOK(r, "browse") });

    const items = res.json("data.items");
    if (items && items.length > 0) {
      const dsID = randomItem(items).id;
      http.get(`${BASE}/datasets/${dsID}`, { tags: { name: "detail" } });
    }

    const qs = ["数据", "AI", "图像"];
    http.get(`${BASE}/search?q=${qs[Math.floor(Math.random() * qs.length)]}`, { headers: headers(tok) });
  } else {
    // 20% auth 路径
    http.get(`${BASE}/users/me`, { headers: headers(tok) });
    http.get(`${BASE}/users/me/notifications?limit=3`, { headers: headers(tok) });
  }

  sleep(3);
}
