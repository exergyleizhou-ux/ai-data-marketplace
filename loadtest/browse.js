// browse.js — 阶梯 5→50 VU × 5min 浏览+翻页

import http from "k6/http";
import { check, sleep } from "k6";
import { tokenPool, headers, checkOK, randomItem, BASE } from "./lib.js";

export const options = {
  // register 限流 5/min per IP:连续跑多个脚本时,上一个 setup 可能已耗尽
  // 当前窗口名额,tokenPool 会整窗退避 61s —— 默认 60s setupTimeout 不够。
  setupTimeout: "150s",
  stages: [
    { duration: "1m", target: 5 },
    { duration: "2m", target: 20 },
    { duration: "2m", target: 50 },
  ],
  thresholds: {
    "http_req_duration": ["p(95)<800"],
    "http_req_failed": ["rate<0.05"],
  },
};

export function setup() {
  return { tokens: tokenPool(5) };
}

export default function (data) {
  const tok = randomItem(data.tokens).token;
  const pages = [1, 2, 3];
  const offset = pages[Math.floor(Math.random() * pages.length)] * 20;

  // 1. 浏览 + 翻页
  let res = http.get(`${BASE}/datasets?limit=20&offset=${offset}`, { headers: headers(tok), tags: { name: "browse" } });
  check(res, { "browse OK": (r) => checkOK(r, "browse") });

  const items = res.json("data.items");
  if (items && items.length > 0) {
    const dsID = randomItem(items).id;
    res = http.get(`${BASE}/datasets/${dsID}`, { tags: { name: "detail" } });
    check(res, { "detail OK": (r) => checkOK(r, "detail") });
  }

  // 2. 搜索
  const qs = ["数据", "AI", "图像", "文本", "医疗"];
  const q = qs[Math.floor(Math.random() * qs.length)];
  res = http.get(`${BASE}/search?q=${q}&limit=10`, { headers: headers(tok), tags: { name: "search" } });
  check(res, { "search OK": (r) => checkOK(r, "search") });

  sleep(3);
}
