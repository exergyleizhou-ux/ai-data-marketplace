// purchase.js — 10 VU × 5min,订单创建与查询(需 APP_ENV=test)

import http from "k6/http";
import { check, sleep } from "k6";
import { tokenPool, headers, checkOK, randomItem, BASE } from "./lib.js";

export const options = {
  // register 限流 5/min per IP:连续跑多个脚本时,上一个 setup 可能已耗尽
  // 当前窗口名额,tokenPool 会整窗退避 61s —— 默认 60s setupTimeout 不够。
  setupTimeout: "150s",
  vus: 10,
  duration: "5m",
  thresholds: {
    "http_req_duration{name:order-create}": ["p(95)<1500"],
    "http_req_failed": ["rate<0.10"],
  },
};

// 注意:
// - 此脚本假设后端为 APP_ENV=test(mock payment + seed dataset 可用)
// - 需要测试环境有一个已发布的 dataset;如果没有,脚本会跳过订单创建部分
// - 不压 register/login(有限流),order create 也有 20/min 限流,10 VU × 每 5s 一次 = 2 req/s = 120/min,会超过限流
//   所以加 sleep 控制在 ~12/min(10 VU × 每 50s 一次)

let cachedDatasetID = undefined;

export function setup() {
  const tokens = tokenPool(5);

  // 获取一个已发布 dataset ID 用于后续下单
  const tok = tokens[0].token;
  const res = http.get(`${BASE}/datasets?limit=5`, { headers: headers(tok) });
  const items = res.json("data.items");
  if (items && items.length > 0) {
    cachedDatasetID = items[0].id;
  }

  return { tokens, datasetID: cachedDatasetID };
}

export default function (data) {
  const tok = randomItem(data.tokens).token;

  // 1. 查询订单列表
  let res = http.get(`${BASE}/orders?limit=5`, { headers: headers(tok) });
  check(res, { "orders list OK": (r) => checkOK(r, "orders") });

  // 2. 创建订单(低频,避免触发 20/min 限流)
  if (data.datasetID && Math.random() < 0.15) {
    res = http.post(`${BASE}/orders`, JSON.stringify({
      dataset_id: data.datasetID,
      license_type: "commercial",
    }), { headers: headers(tok), tags: { name: "order-create" } });
    check(res, {
      "order create OK or expected 4xx": (r) => r.status >= 200 && r.status < 500,
    });

    // 获取刚创建的订单详情(429/409 等无 data.id,先判状态)
    let ordID;
    try { ordID = res.status === 200 ? res.json("data.id") : undefined; } catch (_) {}
    if (ordID) {
      res = http.get(`${BASE}/orders/${ordID}`, { headers: headers(tok) });
      check(res, { "order detail OK": (r) => checkOK(r, "order-detail") });
    }
  }

  sleep(5);
}
