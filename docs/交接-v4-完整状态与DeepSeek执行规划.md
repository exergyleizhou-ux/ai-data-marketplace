# 交接 v4 · 完整状态 + DeepSeek V4 Pro 执行规划

**日期**:2026-06-08  **基线**:`origin/main @ 9e3b059` (PR #72,DeepSeek 上一刀刚合并)
**读者**:接手的新会话 / DeepSeek V4 Pro / 你本人

> **一份顶用**:读这一份即可完全接手 —— 项目状态、Skills 双向磨合实战、四向 C2D 现状、最近运营/对账/搜索新增,以及**为 DeepSeek V4 Pro 准备的下一刀详细执行规划**(§7,DeepSeek 按此规划独立完成,Claude Code 审核)。

---

## 0. 上手即用(铁律)

1. **代码在** `~/ai-data-marketplace`(git;`origin` = `exergyleizhou-ux/ai-data-marketplace`,默认 `main`)
2. **主工作树停在旧分支** `feat/h3-settlement-outbox`;另有 `-docs` / `-h5` 并行 worktree —— **都不要碰**。永远以 `origin/main` 为准,自己开新 worktree。
3. **工作流(一棵树一件事)**:
   ```bash
   git fetch origin
   git worktree add ~/ai-data-marketplace-<name> -b feat/<name> origin/main
   # ...改 + 本地全验证...
   git push -u origin feat/<name>
   gh pr create --base main --title "..." --body "..."
   gh pr checks <n> --watch       # CI: backend / frontend / sidecar
   gh pr merge <n> --squash --delete-branch
   git worktree remove ~/ai-data-marketplace-<name>
   ```
4. **手装工具链**:`export PATH="$HOME/.local/bin:$HOME/sdk/node/bin:$HOME/sdk/pg/bin:$HOME/.bun/bin:$PATH"`
   - Go 1.23+ (`GOTOOLCHAIN=auto` 首次自动下 1.25);`gh` 在 `~/.local/bin`。
   - Node 20 `~/sdk/node/bin`。Postgres server-only `~/sdk/pg/bin`(无 psql)。
   - Python 3.11 venv `~/sdk/sidecar-venv`(numpy/pandas)。Docker Desktop 已装。
5. **Go module 在 `backend/`** —— 所有 `go` 命令 `cd backend` 后再跑。
6. **验证铁律**(从 `backend/` 跑;shell 每次重置 cwd,务必同一条命令里 `cd`):
   ```bash
   cd backend && gofmt -l . && go build ./... && go vet ./...
   # 真 DB 测试需 ephemeral PG(无 Docker / 无 psql):
   T=$(mktemp -d); SOCK=$(mktemp -d); PORT=55440
   initdb -D "$T" -U postgres --auth=trust >/dev/null
   pg_ctl -D "$T" -o "-p $PORT -k $SOCK -c listen_addresses=''" -w start >/dev/null
   DATABASE_URL="postgres://postgres@/postgres?host=$SOCK&port=$PORT&sslmode=disable" \
     go test -race ./...
   pg_ctl -D "$T" stop -m fast >/dev/null
   cd ../frontend && npm ci --fetch-retries=5 && npm run typecheck && npm run lint && npm run build
   ```
   迁移嵌入式,经 `db.RunMigrations(dsn)` / `AUTO_MIGRATE=true` 应用(无 migrate CLI)。前端 node_modules **不跨 worktree 共享**(package-lock 不同),每个 worktree `npm ci`;npm 偶发 ECONNRESET,`--fetch-retries=5` 重试。

---

## 1. 产品与大目标

**绿洲(Verdant Oasis)** = 面向中国市场的 AI 训练数据交易平台。
核心差异化 = **「可用不可见 / 沙箱计算(Compute-to-Data)」**:买方购买**计算权益**,在平台沙箱里跑**经审核的白名单算法**,只取走**结果(模型/统计)**,**不获得原始数据**。合规叙事对齐数据二十条/PIPL(三权分置)。
核心模块:auth / dataset / delivery / order / payment / quality / **search**(从 stub 升为真模块) / **compute(C2D)**。

## 2. 信任阶梯 L0→L3 现状(诚实边界)

| 级别 | 含义 | 后端 | UI | 验证 |
|---|---|---|---|---|
| **L0** 下载 | 交付原始数据 | ✅ | ✅ | — |
| **L1** 数据沙箱 | 买方不可见、**平台仍可见**;`--network=none` 真隔离 | ✅ | ✅ 卖家配/买家用 | **真 Docker**(§19 红队 5/5 + 拉生产镜像 e2e) |
| **L2** 机密计算 | 连平台也不可见;TEE + 远程证明 | ✅ KBS 客户端 + teeRunner + TDX off-hardware fail-closed | ✅ 卖家可选(诚实标注需 TEE 部署) | **本地半已验**(KBS httptest + TDX fail-closed 单测);硬件半门控 TEE 云 |
| **L3 联邦** | 多方,数据不集中;FedAvg | ✅ FedAvg + min_participants 容错 + 中心化 DP + 真 fed-logreg 镜像 | ✅ FederatedComputePanel | **真 Docker 联邦 e2e** |
| **L3 PSI** | 隐私求交;Direction D | ✅ 端到端可用(mockMPC + allow_psi 授权 + 凭证存证) | ✅ PSIComputePanel | **真 PG 集成测试**;真 Secretflow 门控多节点 |

每级叠加**差分隐私**;输出过**闸门**(大小/DP/泄漏/可选人工复核)。**诚实立场**:不把 L1 吹成 L2,不把中心化 FedAvg 吹成安全聚合,不把 mockMPC 吹成密码学私密。

## 3. 已完成的 14+1 个 PR(本会话 + DeepSeek 续作)

**Claude Code 本会话 14 个(#58–#71,2026-06-04)**:

| PR | 方向 | 一句话 |
|---|---|---|
| #58 | A 联邦打磨 | 联邦 Prometheus 指标 + 数据集名(替代 UUID 截断)+ 分页/子作业详情 |
| #59 | 设计 | B/C/D 三份详细落地设计 + 索引 + 三条共同原则 |
| #60 | Skills 磨合 | smart-quote-in-.tsx 坑写进项目 CLAUDE.md(双线磨合首例) |
| #61 | B 阶段1 | L2 KBS 骨架:`KeyBroker` + `mockKBS` + teeRunner 证明门控密钥释放(fail-closed) |
| #62 | C 阶段1 | `MaskedSumAggregator`:成对掩码抵消 = 与明文 FedAvg 数值同,平台只见 Σ |
| #63 | D 阶段1 | `MPCOrchestrator` + `mockMPC` PSI:正确求交语义 |
| #64 | D 阶段1.5 | **PSI 作业端到端**(复用联邦流水线,真 PG 集成测试) |
| #65 | D 阶段2 | PSI 前端面板(/account)— 把 PSI 变成可演示产品 |
| #66 | D 修正 | 专属 `allow_psi` 卖家授权(与 allow_federated 区分,不同隐私暴露) |
| #67 | B 阶段2 | 真 KBS HTTP 客户端 `remoteKBS`(KBS_URL),fail-closed,httptest 覆盖 |
| #68 | B 阶段2 | Intel TDX 硬件 attester 脚手架(off-hardware fail-closed 已验,ioctl 路径需 TEE 节点) |
| #69 | 数据信任 | 计算结果溯源凭证(`VO-<hex>` 一码 + 算法 digest + 数据集) |
| #70 | 数据信任 | 联邦/PSI 联合结果溯源凭证 |
| #71 | 交接 | v3 capstone(被本 v4 取代) |

**DeepSeek V4 Pro 续作 1 个(#72,2026-06-08)**:

| PR | 方向 | 内容 |
|---|---|---|
| #72 | **运营可见性 + 搜索 + 质检引导** | 见 §4 |

## 4. PR #72 详解(DeepSeek 上一刀)

**18 文件,+739/-21**,CI 三 job 全绿(backend 2m37s · frontend 1m16s · sidecar 44s)。

### 后端新增端点

| 方法 | 路径 | 用途 | 权限 |
|---|---|---|---|
| GET | `/admin/reconciliation` | 财务对账 9 项指标(总/已结算 GMV、佣金、纠纷/退款数、失败结算数) | ops |
| GET | `/admin/settlement-outbox` | 出箱列表(status 过滤 + 分页) | ops |
| POST | `/admin/settlement-outbox/:orderId/retry` | 手动重试 failed 结算 | ops |
| GET | `/search` | 搜索(经 `DatasetSearcher` 接口对接 dataset 索引) | 公开 |

### 后端关键改动

- `payment/model.go`:`OutboxEntry` + `ErrOutboxNotFound`/`ErrOutboxNotFailed`。
- `payment/outbox.go`:`OutboxRepository` 接口加 `ListOutbox`/`RetryOutbox`;pg 实现用 `WHERE status='failed'` 乐观锁。
- `payment/service.go`:`ListOutbox` + `RetryOutbox`(写审计 `settlement.outbox.retry`)。
- `payment/handler.go`:`adminListOutbox` + `adminRetryOutbox`;`fail()` 多两个错误映射。
- `payment/router.go`:Register 签名加 `opsGate` 参数;新路由组挂 ops 中间件。
- `order/model.go`:**`Reconciliation` struct**(9 字段:TotalGMV/SettledGMV/PlatformFees/TotalOrders/SettledOrders/PendingOrders/DisputedOrders/RefundedOrders/RefundedAmount/FailedSettlements)。
- `order/repo.go`:`AdminReconciliation` —— 单条 SQL 聚合 orders(SUM/COUNT + FILTER) + 附加查 settlement_outbox failed 计数。
- `order/service.go` + `order/handler.go` + `order/router.go`:暴露 `GET /admin/reconciliation`。
- 新模块 `search/handler.go`:`DatasetSearcher` 接口 + `SearchQuery`/`SearchResult` 类型 + `Register(rg, searcher)` 注册 `GET /search`。
- `dataset/service.go`:新增 `SearchPublished` 适配 `DatasetSearcher`(等同 `List`)。
- `internal/server/server.go`:接 search 模块 + `datasetSearchAdapter` 桥;`payment.Register` 加 ops gate 参数。

### 前端关键改动

- `lib/api.ts`:`OutboxEntry` 类型 + **12 个 admin 方法**(算法注册/审核、计算作业放行/拒绝、结算出箱列表/重试、对账)。
- `app/admin/page.tsx`(+380 行):Tab 从 3 个扩到 5 个:**数据集审核 / 实名审核 / 交易 / 计算作业 / 结算队列**。
  - 新组件:`ComputeJobs`(子 tab:输出审核 + 算法注册)、`JobReviewQueue`、`AlgorithmRegistry` + `AlgoRegisterForm`、`SettlementOutbox`(顶部 3 张统计卡 + 过滤器 + 重试按钮)。
  - `Transactions` 升级为对账仪表盘:从 3 张客户端算的卡 → 8 张后端实时聚合的卡。
- `app/sell/page.tsx`:数据集 `draft` 状态区分两种情况:`sample_count==0` → 「请上传」;`sample_count>0` → 橙色警告「质检失败,查报告修正后重传」。`rejected` 状态加提示。

### 没做的(诚实标注)

- 语义搜索 — pgvector 不可用(需安装扩展或换外部向量 DB);现在 search 模块还是依赖既有 dataset 索引(ILIKE + tsvector,见 PR-Chinese-FTS)。
- C2D 硬件半(B 真 TDX/SEV)、真 Secretflow PSI、C 阶段2 密码学 — 同 v3 表述,均门控外部资源/研究。
- 提现/真实分账 — 微信/支付宝二清红线,需法务。

---

## 5. 代码地图(关键文件锚点)

**后端 compute 模块** `backend/internal/modules/compute/`:
- `model.go` — DTO + 状态机 + 错误哨兵 + `Mode`(Federated/PSI)+ Runtimes(`fed-logreg`/`psi-extract`)。
- `service.go` — 业务不变量;`orchestrator MPCOrchestrator` 字段(默认 mockMPC,`WithOrchestrator` 覆写)。
- `federated.go` — 联邦编排;**SubmitFederatedJob 先定 mode 再 per-mode offer 一致性校验**(allow_federated 不隐含 allow_psi)。
- `aggregator.go` — `FedAvgAggregator` + `MaskedSumAggregator`(掩码求和)+ `parsePartial`。
- `dp.go` — `dpFedAvg` + crypto/rand `laplaceNoise`。
- `mpc.go` — `MPCOrchestrator` 接口 + `mockMPC` PSI(正确求交语义) + `parsePSISet` + `marshalPSIResult`。
- `runner.go` / `runner_docker.go` / `runner_tee.go` + `runner_tee_tdx.go`(TDX 脚手架) / `kbs.go` + `kbs_remote.go`(KBS 客户端)。
- `repo.go` / `repo_federated.go` — 乐观状态机 + `AdminListJobs`/`AdminRelease`/`AdminReject` 等 ops 方法。

**后端其他**:
- `order/model.go` `Reconciliation`、`order/repo.go` `AdminReconciliation` SQL — 对账聚合。
- `payment/outbox.go`、`payment/handler.go` — 结算出箱监控 + 手动重试。
- `search/handler.go` — `DatasetSearcher` 接口 + `GET /search`。

**前端**:
- `components/Compute.tsx` — `ComputeBuyer` / `ComputeOfferEditor` / `FederatedComputePanel` / **`PSIComputePanel`** / `AttestationChip`。
- `app/admin/page.tsx` — 5-tab 运营页(数据集/KYC/交易/计算作业/结算队列)。
- `lib/api.ts` — typed client + admin/compute/federated/PSI/outbox/对账方法 + `OutboxEntry` 类型。

**迁移** `backend/migrations/`:`000001..000013`(`000013_compute_allow_psi.up.sql` 是最新)。

---

## 6. 坑(每个都踩过,务必记住)

1. **JSONB `NOT NULL DEFAULT '{}'` 列**:INSERT 传 `nil` 仍违约 → `toJSONB(nil)` 返回 nil,要改传 `[]byte("{}")`。DEFAULT 只在省略列时生效。
2. **`uuid[]` 参数**:`$N::uuid[]` 显式转型;回读 `dataset_ids::text[]` 进 `[]string`。
3. **乐观状态机**:`UPDATE…WHERE status=$from RETURNING`;0 行 ⇒ `ErrBadTransition`。并发安全,用于幂等编排去重。
4. **enqueue-then-mark-ready 竞态**(本会话踩,沉淀进 `verification-loop` skill):异步编排里,先入队子任务、最后才置「就绪」状态——子任务可能在置位前跑完,其回调因状态守卫 no-op,导致永久卡住。对策:置位后**显式再触发一次推进**(见 `SubmitFederatedJob` 末尾)。
5. **smart-quote-in-.tsx**(本会话踩,沉淀进项目 CLAUDE.md):大块 Write/Edit 可能引入 `"` `"`(U+201C/U+201D)作为 JSX 属性/字符串分隔符,tsc 报 cryptic `TS1127/TS1005`。修复:替换为直引号,**仅允许出现在可见英文文本内**。
6. DTO 时间戳是 `string`(`::text` 扫描),不是 `time.Time`——沿用既有风格。
7. 改完 `gofmt -w`(结构体对齐会变)。
8. macOS 无 `tac`/`timeout`/`migrate`/`brew`;用 `tail -r`、Go context 超时、嵌入式迁移。
9. DP 故意不可复现(新鲜随机);logreg/fed-logreg 确定性(争议复算)。
10. L1+模型输出 ⇒ 必须 trusted 白名单算法(硬约束:沙箱防不住「算法把数据编进模型」)。

---

## 7. **下一刀执行规划:DeepSeek V4 Pro 任务书 — 「时序对账 + 卖家分析」(方向 E)**

> **目标读者:DeepSeek V4 Pro**。按本节独立执行;Claude Code 在 PR 提交后审核。
> **基线**:`origin/main @ 9e3b059`(PR #72)。**单一 PR,纯本地可验证,无外部依赖。**

### 7.1 为什么做这个

PR #72 把 ops 仪表盘从「客户端算」升级为「后端实时聚合」(9 项快照指标)。但仍是**单时点快照**,不能回答:
- 「这周 GMV 比上周涨了多少?」
- 「哪天结算失败最多?」
- 「这个卖家这个月卖了多少?哪个数据集卖得最好?」

时序数据是真正可分析的财务/运营基础。**纯 SQL group-by-day,零外部依赖**,完美的下一刀。

### 7.2 范围 — 一个 PR,三个子任务

**E1 · 运营时序对账**:`GET /admin/reconciliation/timeseries?days=30` → 按日聚合 GMV/结算/失败/纠纷
**E2 · 卖家收益时序**:`GET /sellers/me/earnings/timeseries?days=30` + `GET /sellers/me/earnings/by-dataset` → 卖家自助分析
**E3 · UI 接入**:Admin 对账 tab 加趋势图;卖家 `/account` 加收益分析

明确**不在**本 PR:语义搜索、通知体系、版本管理、提现 —— 留作未来切片。

### 7.3 后端契约(精确到字段)

#### E1.1 `GET /admin/reconciliation/timeseries?days=30`

权限:ops。`days` 默认 30,最大 90。

请求:`?days=<int>`(query)

响应(200):
```json
{
  "days": 30,
  "from": "2026-05-10",
  "to": "2026-06-08",
  "points": [
    { "date": "2026-05-10", "gmv_cents": 0, "settled_gmv_cents": 0,
      "platform_fees_cents": 0, "orders": 0, "settled_orders": 0,
      "refunded_orders": 0, "disputed_orders": 0, "failed_settlements": 0 },
    // ... 30 个连续日点(含零日,前端画图友好)
  ]
}
```

**关键 SQL**(`order/repo.go` 新增方法 `AdminReconciliationTimeseries(ctx, days int)`):

```sql
WITH days AS (
  SELECT (CURRENT_DATE - i)::date AS d
  FROM generate_series(0, $1 - 1) AS i
),
agg AS (
  SELECT
    DATE(created_at AT TIME ZONE 'UTC') AS d,
    COALESCE(SUM(amount_cents), 0)                                              AS gmv_cents,
    COALESCE(SUM(amount_cents) FILTER (WHERE status='settled'), 0)              AS settled_gmv_cents,
    COALESCE(SUM(platform_fee_cents) FILTER (WHERE status='settled'), 0)        AS platform_fees_cents,
    COUNT(*)                                                                    AS orders,
    COUNT(*) FILTER (WHERE status='settled')                                    AS settled_orders,
    COUNT(*) FILTER (WHERE status='refunded')                                   AS refunded_orders,
    COUNT(*) FILTER (WHERE status='disputed')                                   AS disputed_orders
  FROM orders
  WHERE created_at >= CURRENT_DATE - ($1 - 1) * INTERVAL '1 day'
  GROUP BY d
),
outbox_agg AS (
  SELECT
    DATE(updated_at AT TIME ZONE 'UTC') AS d,
    COUNT(*) AS failed_settlements
  FROM settlement_outbox
  WHERE status = 'failed'
    AND updated_at >= CURRENT_DATE - ($1 - 1) * INTERVAL '1 day'
  GROUP BY d
)
SELECT
  days.d::text,
  COALESCE(agg.gmv_cents, 0),
  COALESCE(agg.settled_gmv_cents, 0),
  COALESCE(agg.platform_fees_cents, 0),
  COALESCE(agg.orders, 0),
  COALESCE(agg.settled_orders, 0),
  COALESCE(agg.refunded_orders, 0),
  COALESCE(agg.disputed_orders, 0),
  COALESCE(outbox_agg.failed_settlements, 0)
FROM days
LEFT JOIN agg        USING (d)
LEFT JOIN outbox_agg USING (d)
ORDER BY days.d;
```

> **零日填充**:用 `generate_series` + `LEFT JOIN` 保证前端画图不断点。
> **outbox 表可能不存在**(在某些 schema 老版本上):用 `LEFT JOIN` + 容错(查不到归零)。镜像 #72 中 AdminReconciliation 对 `settlement_outbox` 的容错处理(参考 `order/repo.go:AdminReconciliation` 倒数 10 行的处理)。

#### E1.2 `GET /sellers/me/earnings/timeseries?days=30`

权限:登录用户(自报)。`days` 默认 30,最大 90。

响应(200):
```json
{
  "days": 30,
  "from": "2026-05-10",
  "to": "2026-06-08",
  "points": [
    { "date": "2026-05-10", "gross_cents": 0, "settled_cents": 0,
      "orders": 0, "settled_orders": 0, "refunded_cents": 0 }
    // 30 个日点
  ]
}
```

SQL 模式相同,但 `WHERE seller_id = $2` 而非全表;`gross_cents=SUM(amount_cents)`,`settled_cents=SUM(seller_amount_cents) FILTER (WHERE status='settled')`。

#### E1.3 `GET /sellers/me/earnings/by-dataset`

权限:登录用户(自报)。

响应(200):
```json
{
  "items": [
    { "dataset_id": "uuid", "title": "数据集 X",
      "total_orders": 12, "settled_orders": 10,
      "gross_cents": 100000, "settled_cents": 90000,
      "last_order_at": "2026-06-07T10:30:00Z" }
  ]
}
```

SQL:
```sql
SELECT
  o.dataset_id::text,
  COALESCE(d.title, ''),
  COUNT(*) AS total_orders,
  COUNT(*) FILTER (WHERE o.status='settled') AS settled_orders,
  COALESCE(SUM(o.amount_cents), 0) AS gross_cents,
  COALESCE(SUM(o.seller_amount_cents) FILTER (WHERE o.status='settled'), 0) AS settled_cents,
  COALESCE(MAX(o.created_at)::text, '') AS last_order_at
FROM orders o
LEFT JOIN datasets d ON d.id = o.dataset_id
WHERE o.seller_id = $1
GROUP BY o.dataset_id, d.title
ORDER BY settled_cents DESC, total_orders DESC
LIMIT 200;
```

### 7.4 后端文件清单(精确改动位置)

| 文件 | 改动 |
|---|---|
| `backend/internal/modules/order/model.go` | 新增 `ReconciliationPoint`(9 字段,带 `Date string`)、`EarningsPoint`(6 字段)、`EarningsByDataset`(7 字段)三个 struct |
| `backend/internal/modules/order/repo.go` | `Repository` 接口加三方法 `AdminReconciliationTimeseries(ctx, days int) ([]ReconciliationPoint, error)`、`SellerEarningsTimeseries(ctx, sellerID string, days int) ([]EarningsPoint, error)`、`SellerEarningsByDataset(ctx, sellerID string) ([]EarningsByDataset, error)`。`pgRepo` 实现按 7.3 SQL |
| `backend/internal/modules/order/service.go` | 暴露三方法(权限校验留在 handler 层) |
| `backend/internal/modules/order/handler.go` | `adminReconciliationTimeseries`/`sellerEarningsTimeseries`/`sellerEarningsByDataset`;`days` 参数校验(默认 30,1-90 闭区间) |
| `backend/internal/modules/order/router.go` | 注册 `/admin/reconciliation/timeseries`(ops gate)+ `/sellers/me/earnings/timeseries` + `/sellers/me/earnings/by-dataset`(authed) |
| `backend/internal/modules/order/service_test.go` | fakeRepo 实现新方法(内存遍历) |

### 7.5 前端文件清单

| 文件 | 改动 |
|---|---|
| `frontend/lib/api.ts` | 新增三个 type(`ReconciliationPoint`、`EarningsPoint`、`EarningsByDataset`)+ 三个方法(`adminReconciliationTimeseries(days?)`、`sellerEarningsTimeseries(days?)`、`sellerEarningsByDataset()`) |
| `frontend/components/MiniChart.tsx`(**新文件**) | 极简 SVG 折线图(无外部依赖,~80 行)。Props: `points: {date: string; value: number}[]`、`color?: string`、`height?: number`、`label?: string`。`viewBox="0 0 300 80"`,自动计算 minMax + 等距 X 轴。**纯展示,无交互,后续可换 recharts** |
| `frontend/app/admin/page.tsx` | `Transactions` 组件:在 8 张统计卡下方加「最近 30 天趋势」区,3 张并排小图(GMV 蓝、已结算 GMV 绿、失败结算 红);加 `days` 切换(7/30/90) |
| `frontend/app/account/page.tsx` | 新「卖家收益分析」Card(仅 `user.role==='seller'` 或当前用户拥有 ≥1 个数据集时显示):2 张折线图(总收益 / 已结算)+ 数据集排行榜(表格 6 列:标题/总订单/已结算/总额/已结算额/最近订单) |

### 7.6 TDD 测试清单(先写测试,看 RED,再实现)

**单元(快)**:
1. `TestAdminReconciliationTimeseries_FillsZeroDays` — 在 ephemeral PG 插 3 个跨日订单,断言返回 30 个点(零日填充),目标日 GMV 准确
2. `TestSellerEarningsTimeseries_ScopedToSeller` — 两个 seller,各自只看到自己的数据
3. `TestSellerEarningsByDataset_AggregatesPerDataset` — 一个 seller 两个数据集各 2 单,断言聚合正确 + 排序按 `settled_cents DESC`
4. `TestSellerEarningsTimeseries_DaysClampedTo90` — handler 层 `days=999` 被夹到 90

**HTTP**(handler 层):
5. `TestAdminTimeseriesRequiresOpsRole` — 普通用户调用返回 403
6. `TestSellerTimeseries_Self` — 任意登录用户可调,只返回自己的数据
7. `TestTimeseries_DefaultDays30` — 无 `days` 参数时返回 30 个点

**关键边界**:
- `days=0` / `days=-1` → 400
- 无订单 seller → `points=[30 个零日]`,`items: []`
- 时区:UTC 截断(`DATE(created_at AT TIME ZONE 'UTC')`)

### 7.7 验证清单(本地 CI-equiv,提交 PR 前必跑)

```bash
export PATH="$HOME/.local/bin:$HOME/sdk/pg/bin:$HOME/sdk/node/bin:$PATH"

# 后端
cd ~/ai-data-marketplace-<name>/backend
gofmt -l . && go build ./... && go vet ./...
T=$(mktemp -d); SOCK=$(mktemp -d); PORT=55440
initdb -D "$T" -U postgres --auth=trust >/dev/null
pg_ctl -D "$T" -o "-p $PORT -k $SOCK -c listen_addresses=''" -w start >/dev/null
DATABASE_URL="postgres://postgres@/postgres?host=$SOCK&port=$PORT&sslmode=disable" \
  go test -race -count=1 ./...
pg_ctl -D "$T" stop -m fast >/dev/null

# 前端
cd ../frontend
npm ci --fetch-retries=5
npx tsc --noEmit
npx next lint
npm run build

# smart-quote 扫描(本项目踩过的坑)
python3 -c "
for f in ['components/MiniChart.tsx','components/Compute.tsx','app/admin/page.tsx','app/account/page.tsx','lib/api.ts']:
    try:
        data = open(f).read()
        n = sum(1 for c in data if c in '“”‘’')
        if n > 0:
            print(f, 'curly-quotes:', n)
    except FileNotFoundError:
        pass
"
# curly quotes 只允许出现在可见文本字符串内部,不可作为 JSX 属性/字符串分隔符
```

CI 三 job 必须全绿才合并:backend(gofmt + vet + build + test -race + 真 PG)/ frontend(typecheck + lint + build)/ sidecar(算法 + redteam,本 PR 不动这块所以默认通过)。

### 7.8 PR 模板

标题:`feat: time-series reconciliation + seller earnings analytics (方向 E)`

正文:
```markdown
## 方向 E · 时序对账 + 卖家分析(基于 PR #72)

PR #72 把 ops 仪表盘从「客户端算」升级为「后端实时聚合」(9 项快照)。本 PR 把它从「单时点」升级为「可分析」:

### 后端新增
- `GET /admin/reconciliation/timeseries?days=30` (ops) — 按日聚合 9 项指标,零日填充
- `GET /sellers/me/earnings/timeseries?days=30` (authed) — 卖家自己的日时序
- `GET /sellers/me/earnings/by-dataset` (authed) — 卖家各数据集排行

### 前端新增
- `components/MiniChart.tsx` 极简 SVG 折线图(无外部依赖)
- `/admin` Transactions tab:8 项快照下方加 3 张趋势图 + 7/30/90 天切换
- `/account` 新增「卖家收益分析」Card:2 张折线图 + 数据集排行榜

### TDD
N 个单测 + N 个 handler 测试,见 docs/交接-v4 §7.6。

### 验证
gofmt/vet/build 绿;`go test -race ./...` vs ephemeral PG 全绿;前端 tsc/lint/build 绿;smart-quote 扫描清。

### 诚实边界
零外部依赖,纯 PG group-by-day SQL。**不**含:语义搜索 / 通知体系 / 提现 / 真实 TEE 硬件 — 见 docs/交接-v4 §8。
```

### 7.9 「双线磨合」要求(每刀必做)

执行过程中**踩到的坑**沉淀两类:
- **项目专属** → 写进项目根 `CLAUDE.md` 的「Gotchas」一节(本项目已有此机制,#60 是首例)
- **通用可复用** → 反馈到全局 skill(本项目通过外部记忆机制,DeepSeek 可在 PR 描述末尾的「Skills learned」节列出,Claude Code 审核时回灌)

具体到本 PR 可能踩的:
- pg `generate_series` 零日填充的语法陷阱
- `DATE()` + 时区 — 必须显式 `AT TIME ZONE 'UTC'`,否则跨时区主机结果不一致
- pgxpool 上下文取消的 group-by 查询行为
- `seller_amount_cents` 在 refunded 后是否仍计为 settled(看 `payment` 模块当时的语义)

每条都值得记录,即便没踩到也在 PR 描述里列「试过没踩到但值得记忆的边界」。

---

## 8. 完整未来路径(全部需外部资源 / 决策 / 研究)

| 方向 | 卡点 | 何时启动 |
|---|---|---|
| **B 硬件半** 真 TDX/SEV quote 生成 + DCAP | 需 **TEE 云节点**(阿里云加密计算 / Azure CVM) | 开通环境后,按 `docs/部署-L2-TEE节点与KBS.md` 在节点验证 ioctl |
| **D 真 Secretflow PSI** | 需 **≥2 个 Secretflow 节点** | 任一一方搭节点后,接 `MPCOrchestrator` 接口换实现 |
| **C 阶段2** 沙箱内掩码密钥协商 + 掉队恢复 | **密码学研究 spike**(Bonawitz SecAgg + Shamir) | 先出实施细化文档(协议轮次/掉队/与 DP 叠加),评审后才动手 |
| **真实分账**(微信/支付宝) | **二清红线 + 法务** | 法务定稿 + 持牌方签约后接 Stripe 之外的真实通道 |
| **语义/向量搜索** | **pgvector 不可用 或 选外部向量 DB** | 选定后 search 模块加 `EmbeddingProvider` 接口 |

**纯本地可继续做的**(若环境一直没开通):
- 方向 E(本规划)— 时序对账 + 卖家分析
- 方向 F — 通知体系:订单/质检/结算/作业事件 → 卖家/买家/ops 通知中心
- 方向 G — 数据集版本管理 UI(后端表已有)
- 方向 H — 凭证验证公开页(任何人 paste cert id 验证)
- 方向 I — Buyer 收藏 / 已购列表导出 / 订单历史筛选

---

## 9. 给新会话的开场白(可直接复制)

> 「我接手绿洲项目。先读 `docs/交接-v4-完整状态与DeepSeek执行规划.md`(单一自包含)。
> - **现在的我是 DeepSeek V4 Pro** → 按 §7 任务书执行;PR 提交后等 Claude Code 审核。
> - **现在的我是 Claude Code** → 项目状态见 §3-§5;§7 是 DeepSeek 的任务规划,如果 DeepSeek 已提交 PR,我负责按 §7.7-§7.9 审核(SQL 正确性、TDD 完整性、双线磨合是否回灌);新需求按 §0 工作流开新 worktree,TDD,CI 三 job 绿再合并。
> - 任何时候:一棵 worktree 一件事;不碰 h3/docs/h5 三个老 worktree;诚实标注 mock 不吹成成品。」

---

**交接完毕。**
