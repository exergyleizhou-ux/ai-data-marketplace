# AI 训练数据交易市场平台

高信任、可追溯、合规的 AI 训练数据流通基础设施。护城河是三件事：**质量可信 · 来源合规 · 资金安全**（撮合本身门槛极低）。

完整设计见 [`docs/`](docs/)，重点读 [`docs/设计与实施文档-v2.0.md`](docs/设计与实施文档-v2.0.md)（架构 + P3 MVP + PR 计划 + 三条合规生死线）。

**当前状态**：后端 PR-01 ~ 18 全量 + 前端全栈应用，CI 绿，每个 PR 真库 e2e 验证。真实 Stripe Connect 支付 + 90/10 分账已端到端跑通。剩余待办见 [进度](#进度) 节，均不阻塞主干。

## 架构

模块化单体（**不是**微服务）。4–6 人团队 + P3 体量下，微服务的分布式复杂度纯属负债；按清晰的包边界拆分，需要时再沿边界抽服务。

```
ai-data-marketplace/
├── backend/                 Go + Gin 单可部署单元
│   ├── cmd/
│   │   ├── api/             进程入口（含 --healthcheck 探针）
│   │   └── devsql/          运维用临时 SQL（生产禁用，见下）
│   └── internal/
│       ├── config/          环境配置（所有 env 变量在此集中读取）
│       ├── server/          HTTP 引擎 + 路由 + 健康检查 + adapter 桥接
│       ├── platform/        横切基础设施，不依赖任何业务模块
│       │   ├── db redis storage httpx audit metrics
│       │   ├── middleware ratelimit          （Redis + 内存降级限流）
│       │   └── textseg                        （中文分词，go-ego/gse，纯 Go）
│       └── modules/         业务模块，包隔离，仅经导出接口交互
│           ├── auth/        用户/实名 KYC/JWT/RBAC
│           ├── dataset/     数据集元数据/分片上传/来源电子签
│           ├── quality/     异步质检（格式/统计/SimHash 去重/PII）
│           ├── search/      中文全文检索（PG tsvector + GIN）
│           ├── order/       订单 + 严格状态机
│           ├── payment/     支付 + 分账（资金不落平台账户）
│           └── delivery/    一次性 token + 指纹 + 许可签约
├── frontend/                Next.js 14 (App Router) + TS + Tailwind
│   ├── lib/                 API 客户端（统一 envelope + JWT + 401 刷新）+ 认证态
│   ├── components/          UI 原语 / Nav / Protected
│   └── app/                 登录·注册·账户(KYC)·数据市场·详情·卖家·订单·收益·运营后台
├── docker-compose.yml       本地全栈（postgres/redis/minio/backend/frontend）
└── .github/workflows/ci.yml CI（go vet/build/test + 前端 typecheck/lint/build）
```

**铁律**：模块之间只经导出接口交互，`server` 用 adapter 桥接；`platform` 不依赖任何业务模块。每个模块有 Repository 接口 + pgx 实现 + 内存 fake（单测）。统一响应 `httpx.Body{code,message,data,request_id}`；金额一律整数分（`int64`）；`audit_logs` 只追加（触发器禁改删）。

## 三条生死线（动工前必读 docs §2）

1. **资金合规**：托管即「资金二清」（刑事风险）。必须走**分账**，资金全程由持牌方存管，平台只下指令。
2. **数据合规**：来源合法性是命门。上传强制声明 + 电子签约 + 形式审查留痕 + PII 扫描；P3 仅境内。
3. **数据交付**：纯文本无法防复制。不承诺「防盗版」，只承诺「正版、干净、可追责」。

## 本地运行（Docker 全栈，最省事）

前置：Docker。

```bash
make up                      # 等价 docker compose up --build
# backend  → http://localhost:8080/healthz
# frontend → http://localhost:3000
# 自带 postgres / redis / minio，AUTO_MIGRATE 已默认开启
make down                    # 停并清理
make logs                    # 跟踪日志
```

不想用 Docker、要在宿主机上对**真实 Postgres / MinIO / Stripe** 做端到端联调，见下一节。

## 无 Docker 本地联调 / 真库 e2e

这套流程不依赖 Docker，全部用宿主机进程，是日常排障和验收的主路径。本机的 `go / node / postgres / minio / stripe / gh` 都装在用户目录，**每个新终端先设 PATH**：

```bash
export PATH="$HOME/.local/bin:$HOME/sdk/pg/bin:$HOME/sdk/go/bin:$HOME/sdk/node/bin:$PATH"
export GOPATH="$HOME/go" GOMODCACHE="$HOME/go/pkg/mod"
# Go 1.23.4 本体 + GOTOOLCHAIN=auto 会按 go.mod 自动拉 go 1.25（正常，别手动改）
```

工具链位置一览：

| 工具 | 路径 | 备注 |
|------|------|------|
| Go | `~/sdk/go` | go1.23.4；`go.mod` 要求 1.25，`auto` 自动补 |
| Node 20 | `~/sdk/node` | 前端构建 |
| Postgres 16 | `~/sdk/pg/bin` | 只有 `initdb`/`pg_ctl`/`postgres`，**没有** `psql`/`createdb` |
| MinIO | `~/.local/bin/minio` | S3 兼容对象存储 |
| Stripe CLI | `~/.local/bin/stripe` | webhook 转发 + 测试 |
| gh | `~/.local/bin/gh` | 已登录 `exergyleizhou-ux` |

### 1. 起本地 Postgres

没有 `createdb`，所以直接用默认 `postgres` 库。

```bash
rm -rf ~/sdk/pgdata
initdb -D ~/sdk/pgdata -U app --auth=trust -E UTF8
pg_ctl -D ~/sdk/pgdata -o "-p 55432 -k /tmp" -l /tmp/pg.log -w start
# DSN（贯穿后续命令）：
export DSN="postgres://app@127.0.0.1:55432/postgres?sslmode=disable"
```

### 2. 起 API（开发态全开）

```bash
cd ~/ai-data-marketplace/backend
AUTO_MIGRATE=true KYC_AUTO_APPROVE=true APP_ENV=development HTTP_ADDR=":8080" \
  DATABASE_URL="$DSN" \
  REDIS_URL="redis://127.0.0.1:1/0" \
  JWT_SECRET=test PII_SECRET=pii \
  STORAGE_DRIVER=local STORAGE_DIR=/tmp/st \
  go run ./cmd/api
```

- `AUTO_MIGRATE=true`：进程启动时跑 `backend/migrations`（000001..000003，编译进二进制）。
- `KYC_AUTO_APPROVE=true`：实名提交即通过，免去人工审核环节。
- `REDIS_URL` 指向死端口（`127.0.0.1:1`）→ 限流器**自动降级为内存模式**，无需起 Redis。
- 健康检查：`GET /healthz`（liveness）、`GET /readyz`（含 DB ping）、`GET /metrics`（Prometheus）。
- 前端：`cd ~/ai-data-marketplace/frontend && npm install && npm run build && npm run start`（:3000）。

### 3. devsql：本地提权运维账号

`cmd/devsql` 跑临时 SQL（生产态 `APP_ENV=production` 会拒绝运行）。常用来把某账号提权为 `ops` 以进运营后台审核：

```bash
cd ~/ai-data-marketplace/backend
DATABASE_URL="$DSN" APP_ENV=development \
  go run ./cmd/devsql "UPDATE users SET role='ops' WHERE account='op@x.com'"
# 可一次传多条 SQL；每条成功打印 OK: ... -> 影响行数
```

### 4. MinIO（S3 驱动真库 e2e）

```bash
MINIO_ROOT_USER=minioadmin MINIO_ROOT_PASSWORD=minioadmin \
  minio server /tmp/miniodata --address :9000 --console-address :9001
```

然后给 API 加上 S3 驱动环境变量（替换第 2 步的 `STORAGE_DRIVER=local ...`）：

```bash
STORAGE_DRIVER=s3 \
  S3_ENDPOINT=127.0.0.1:9000 S3_BUCKET=ai-data-marketplace \
  S3_ACCESS_KEY=minioadmin S3_SECRET_KEY=minioadmin S3_USE_SSL=false
```

下载走**预签名 URL**（字节不经应用服务器）。同一份驱动通吃 MinIO / AWS S3 / 阿里云 OSS / 腾讯云 COS，切云只改 `S3_ENDPOINT` / `S3_USE_SSL` / key。

> S3 规则提醒：多段上传除最后一段外**每段需 ≥5MB**（不是 bug）；小文件走单段。

### 5. Stripe 测试模式真支付（免费，不动真钱）

真实集成、测试模式零成本。测试密钥放在**仓库外** `~/.config/marketplace/stripe.env`（`sk_test_…`，账户已开通 Connect）。**绝不要把密钥写进任何提交文件**。

```bash
source ~/.config/marketplace/stripe.env

# a) 先起 webhook 转发，从输出里 grep 出 whsec_
stripe listen --api-key "$STRIPE_SECRET_KEY" \
  --forward-to "http://127.0.0.1:8080/api/v1/payments/webhook/stripe"
#   → "Ready! Your webhook signing secret is whsec_xxx"

# b) 用上面拿到的 whsec_ 起 API（替换第 2 步支付相关变量）
PAYMENT_PROVIDER=stripe \
  STRIPE_SECRET_KEY="$STRIPE_SECRET_KEY" \
  STRIPE_WEBHOOK_SECRET="whsec_从a步骤拿到的" \
  STRIPE_CURRENCY=usd

# c) 测试确认支付（bypassPending 让资金立即可用，转账才不会余额不足）
curl https://api.stripe.com/v1/payment_intents/<pi_id>/confirm \
  -u "$STRIPE_SECRET_KEY:" \
  -d payment_method=pm_card_bypassPending \
  -d return_url=https://example.com
```

实现要点：webhook 用 `ConstructEventWithOptions{IgnoreAPIVersionMismatch:true}`（账户 API 版本比 SDK 新）；provider 在 [`payment/stripe.go`](backend/internal/modules/payment/stripe.go)，实现 `PaymentProvider + SplitProvider`（separate charges & transfers，平台分账标准模式）。Connect 收款只需 secret key；分账（向卖家 transfer）需在 dashboard 开通 Connect。

> 没有 Stripe key 时，开发态用沙箱支付：`PAYMENT_PROVIDER=mock` + `POST /api/v1/payments/dev/mark-paid` 模拟支付成功（前端结账页的「沙箱支付」按钮即走此路径）。

### 闭环验证（已在浏览器实测）

注册 → 实名 → 浏览/预览 → 下单 → 支付（沙箱或 Stripe）→ 下载 → 确认收货 → 自动分账（卖家 90% / 平台 10%）→ 评价。卖家「收益」实时反映已结算金额；运营后台（需 `ops` 角色）做数据集审核。

### zsh 踩坑备忘

1. payload 里变量加花括号：`"${a}:${b}:true"`（裸 `$b:true` 会触发 zsh `:t` 修饰符吃字符）。
2. `$(...)` 会吃掉结尾换行；字节级比对用临时文件 + `cmp`，别用命令替换。
3. 别给变量名 `UID` 赋值（zsh 只读）。

## 环境变量参考

所有变量在 [`backend/internal/config/config.go`](backend/internal/config/config.go) 集中读取；本地模板见 [`.env.example`](.env.example)（**切勿提交真实 `.env`**）。

| 变量 | 默认 | 说明 |
|------|------|------|
| `APP_ENV` | `development` | `development` / `staging` / `production` |
| `HTTP_ADDR` | `:8080` | 监听地址；或用 `HTTP_PORT` 只给端口 |
| `DATABASE_URL` | `postgres://app:app@localhost:5432/...` | Postgres DSN |
| `REDIS_URL` | `redis://localhost:6379/0` | 连不上 → 限流自动降级内存 |
| `AUTO_MIGRATE` | `false` | 启动时自动迁移（dev/CI 设 `true`） |
| `JWT_SECRET` | `dev-insecure-change-me` | 生产**必须**覆盖，否则启动失败 |
| `PII_SECRET` | `dev-pii-secret` | 敏感字段 keyed-hash 密钥 |
| `KYC_AUTO_APPROVE` | `false` | dev 自动通过实名 |
| `STORAGE_DRIVER` | `local` | `local` / `s3` |
| `STORAGE_DIR` | `./data/storage` | local 驱动根目录 |
| `S3_ENDPOINT` `S3_BUCKET` `S3_ACCESS_KEY` `S3_SECRET_KEY` `S3_USE_SSL` `S3_REGION` | MinIO 默认 | s3 驱动；`S3_ENDPOINT` 是 `host:port` 无 scheme |
| `PAYMENT_PROVIDER` | `mock` | `mock` / `stripe` / `wechat` / `alipay` |
| `PAYMENT_MOCK_SECRET` | `dev-pay-secret` | 沙箱回调 HMAC 密钥 |
| `STRIPE_SECRET_KEY` `STRIPE_WEBHOOK_SECRET` `STRIPE_CURRENCY` | 空 / `usd` | Stripe 真集成（测试模式免费） |
| `CORS_ALLOW_ORIGIN` | `*` | 允许调用 API 的浏览器源 |

> 密钥卫生：仓库 `.gitignore` 已含 `.env`/`*.local`；真实 Stripe 测试密钥（`sk_test_` 后接长串账户标识）只存在于仓库外的 `~/.config/marketplace/stripe.env`，源码里出现的一律是 `sk_test_…` 占位符。提交前可 `git grep` 真实密钥前缀确认零命中。

## 构建 / 测试 / 迁移

```bash
# 后端（改完必须全绿）
cd backend && gofmt -w <改动文件> && go vet ./... && go build ./... && go test ./...
# 加竞态检测（CI 推荐）
go test -race ./...

# 前端
cd frontend && npm install && npm run build      # typecheck/lint/build 必须绿

# 迁移：进程内自迁移设 AUTO_MIGRATE=true；或用 golang-migrate CLI
make migrate-up        # / migrate-down / migrate-create name=add_foo
```

迁移文件在 [`backend/migrations/`](backend/migrations/)（000001..000005）并编译进二进制。`make help` 列出全部目标。

## 进度

**已完成 PR-01 ~ 18 全部可做范围 + 前端 Web 应用**，每个 PR 真库 e2e + GitHub CI 验证：

- 身份/实名 KYC、RBAC、限流（Redis + 内存降级）
- 数据集元数据 + 来源电子签、分片上传、质检（格式/统计/PII/SimHash）、审核状态机
- 中文分词全文检索（`platform/textseg` + PG `tsvector` + GIN，migration 000003；规模化可平替 zhparser / ES+IK）
- 对象存储 S3 驱动（MinIO/AWS/OSS/COS 通用）+ 预签名下载
- 订单严格状态机、交付（一次性 token + 指纹 + 许可签约）、评分/纠纷/收益/运营后台
- Prometheus `/metrics`、异步质检 worker
- **真实 Stripe Connect 支付 + 90/10 分账**已端到端验证

**支付/结算/安全硬化全部合入 `main`**（H1–H7，2026-06 整合）：

- **[H1]** Stripe 连接账户落库：`sellerID→acct` 映射持久化到 `payout_accounts` 表（auth 模块拥有，migration 000004）。
- **[H2]** 退款 + 分账回退：`order.ResolveDispute` 选「退款」先 Transfer Reversal 再 Refund PI；事务内把 payment→refunded、settlement→reverted。
- **[H3]** 结算可靠性：`settlement_outbox` 表 + worker（PG 事务级 advisory lock，崩溃自释放/多实例互斥）+ 指数退避重试 + Stripe 幂等 key（migration 000005）。
- **[H4]** 刷新令牌吊销：每个 JWT 带 `jti`；`Denylist`（Redis 共享 / 内存降级）支持登出 + **刷新令牌单次使用轮换 + 重放检测**；`POST /auth/logout`。访问令牌仍无状态（短期失效，不查 denylist）。
- **[H5]** 前端真实支付：结账页接 Stripe.js（Payment Element），`pk_test` 真刷卡；未配公钥时回退沙箱按钮。客户端绝不自标支付，靠 webhook 翻 `paid`。
- **[H6]** 本 README + 无 Docker 本地联调文档。
- **[H7]** CI 加 `-race` + gofmt 门 + Postgres 服务；新增真库 HTTP 集成测试（`internal/server/integration_test.go`，跑通注册→刷新轮换→重放 401→登出→401，无 `DATABASE_URL` 时自动跳过）。

**剩余仅外部墙（需用户/法务，别动）**：微信支付宝真实分账（资金二清刑事红线）、真实云部署、三份法律文本。
