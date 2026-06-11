# 性能基线(T4-A 动态半)

用合并进 main 的 k6 套件 + `EXPLAIN ANALYZE`,在**有数据量**的真实后端上验证
索引与连接池调优的实际效果。这是基线快照,不是 SLO —— 复跑命令见末尾。

> 测量环境:单机(Apple Silicon)、单实例 PostgreSQL(临时 socket 库)、
> 后端 `APP_ENV=test` 本地进程。**绝对数字受本机限制**(localhost、单 PG);
> 真实容量需在生产同构环境复跑。这里的价值是**相对结论**(索引被采用、
> 池大小的拐点),它们不随环境变化。

数据量:datasets 5,000 · orders 20,000 · notifications 20,000(单用户持有)。

## 1. 复合索引验证(#106)

每条热分页列表 `WHERE col=$1 ORDER BY created_at DESC LIMIT 20`,`EXPLAIN ANALYZE`
确认走 `(col, created_at DESC)` 复合索引、**无独立 Sort 步**(索引直接提供顺序):

| 查询 | 计划 | 执行时间 |
|------|------|---------|
| orders by buyer | `Index Scan using idx_orders_buyer_created` | 0.037 ms |
| orders by seller | `Index Scan using idx_orders_seller_created` | 0.030 ms |
| datasets by seller | `Index Scan using idx_datasets_seller_created` | 0.022 ms |
| notifications inbox | `Index Scan using idx_notifications_user_created` | 0.020 ms |

四条均为 Index Scan、无 Sort node,20k 行下亚毫秒。改造前的单列索引会满足 `WHERE`
但强制对全部匹配行排序;复合索引消除了排序步。

## 2. 吞吐基线

热只读路径(`GET /datasets?limit=20` + `GET /orders?limit=20`,Bearer 鉴权),
100 VU 无 think-time,30s:

- **5,745 req/s**,p95 **22.9 ms**,p90 21.2 ms,max 126 ms,**0% 错误**(172,462 请求)

## 3. 连接池拐点(`DB_MAX_CONNS`,#106)

同一压测,只变连接池大小 —— 验证 env 旋钮真有效、默认 10 是合理值:

| `DB_MAX_CONNS` | 吞吐 | p95 | max | 结论 |
|----------------|------|-----|-----|------|
| 4 | 4,525 req/s | 33.7 ms | 56 ms | 池饥饿:VU 排队等连接 |
| **10(默认)** | **5,964 req/s** | **23.4 ms** | 82 ms | 甜点,较 4 提升 **+32%** |
| 30 | 5,725 req/s | 22.6 ms | **390 ms** | 过配:吞吐回落 + 尾延迟恶化 |

结论:单实例 PG 下连接数过低会排队、过高会因争用拖尾;默认 10 接近最优。
生产按 `(PG 核数 × 2) ÷ 应用副本数` 调整,并留 `max_connections` 余量。

## 4. 场景套件(节奏化,贴近真实)

`loadtest/` 五个 k6 脚本(带 think-time,反映真实用户节奏)在同一后端实测通过:
smoke/browse/auth-churn/purchase/soak,checks 全过、错误率在阈值内。

## 复跑

```bash
export PATH="$HOME/.local/bin:$HOME/sdk/node/bin:$HOME/sdk/pg/bin:$PATH"
# 1) 起临时 PG + 迁移 + 灌量(见本仓 deploy/backup/drill.sh 的起库范式)
# 2) 起后端:APP_ENV=test ... DB_MAX_CONNS=10 go run ./cmd/api
# 3) 压测:
BASE_URL=http://localhost:8080/api/v1 k6 run loadtest/browse.js
# 4) 索引验证:EXPLAIN (ANALYZE, COSTS OFF) <热查询>
```

> 后续(需生产同构环境):接 OTel→Tempo 看真实 span 火焰图、用本套件在
> 预发压出真实容量曲线、按 §3 方法定生产 `DB_MAX_CONNS`。
