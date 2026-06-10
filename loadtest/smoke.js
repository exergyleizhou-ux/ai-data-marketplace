// smoke.js — 冒烟测试, 1 VU × 1min 核心只读路径

import http from "k6/http";
import { check, sleep } from "k6";
import { tokenPool, headers, checkOK, BASE } from "./lib.js";

export const options = {
  // register 限流 5/min per IP:连续跑多个脚本时,上一个 setup 可能已耗尽
  // 当前窗口名额,tokenPool 会整窗退避 61s —— 默认 60s setupTimeout 不够。
  setupTimeout: "150s",
  vus: 1,
  duration: "1m",
  thresholds: {
    "http_req_duration{name:datasets}": ["p(95)<500"],
    "http_req_failed": ["rate<0.01"],
  },
};

export function setup() {
  return { tokens: tokenPool(1) };
}

export default function (data) {
  const tok = data.tokens[0].token;

  // 1. 浏览数据集列表
  let res = http.get(`${BASE}/datasets?limit=5`, { headers: headers(tok), tags: { name: "datasets" } });
  check(res, { "datasets OK": (r) => checkOK(r, "datasets") });

  // 从列表中提取一个真实 dataset ID
  const items = res.json("data.items");
  if (items && items.length > 0) {
    const dsID = items[0].id;
    // 2. 查看数据集详情
    res = http.get(`${BASE}/datasets/${dsID}`, { tags: { name: "dataset-detail" } });
    check(res, { "detail OK": (r) => checkOK(r, "dataset-detail") });
  }

  // 3. 搜索
  res = http.get(`${BASE}/search?q=数据`, { headers: headers(tok), tags: { name: "search" } });
  check(res, { "search OK": (r) => checkOK(r, "search") });

  sleep(2);
}
