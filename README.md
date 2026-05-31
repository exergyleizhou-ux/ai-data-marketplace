# AI 训练数据交易市场平台

高信任、可追溯、合规的 AI 训练数据流通基础设施。护城河是三件事：**质量可信 · 来源合规 · 资金安全**（撮合本身门槛极低）。

完整设计见 [`docs/`](docs/)。本仓库当前处于 **PR-01：脚手架 + 基础架构**。

## 架构

模块化单体（**不是**微服务）。4–6 人团队 + P3 体量下，微服务的分布式复杂度纯属负债；按清晰的包边界拆分，需要时再沿边界抽服务。

```
ai-data-marketplace/
├── backend/                 Go + Gin 单可部署单元
│   ├── cmd/api/             进程入口（含 --healthcheck）
│   └── internal/
│       ├── config/          环境配置
│       ├── server/          HTTP 引擎 + 路由 + 健康检查
│       ├── platform/        横切基础设施（db/redis/queue/httpx/audit）
│       └── modules/         业务模块，包隔离，仅经接口交互
│           ├── auth/        用户/实名/JWT/RBAC
│           ├── dataset/     数据集元数据/上传/来源签约
│           ├── quality/     异步质检（格式/统计/去重/PII）
│           ├── search/      中文全文检索（zhparser，非 pg_trgm）
│           ├── order/       订单 + 严格状态机
│           ├── payment/     支付 + 分账（资金不落平台账户）
│           └── delivery/    临时链接 + 指纹 + 许可签约
├── frontend/                Next.js 14 (App Router) + TS + Tailwind
│   ├── lib/                 API 客户端（统一 envelope + JWT + 401 刷新）+ 认证态
│   ├── components/          UI 原语 / Nav / Protected
│   └── app/                 登录·注册·账户(KYC)·数据市场·详情·卖家·订单·收益·运营后台
├── docker-compose.yml       本地全栈（postgres/redis/backend/frontend）
└── .github/workflows/ci.yml CI（go vet/build/test + 前端 typecheck/lint/build）
```

## 三条生死线（动工前必读 docs §2）

1. **资金合规**：托管即「资金二清」（刑事风险）。必须走**分账**，资金全程由持牌方存管，平台只下指令。
2. **数据合规**：来源合法性是命门。上传强制声明 + 电子签约 + 形式审查留痕 + PII 扫描；P3 仅境内。
3. **数据交付**：纯文本无法防复制。不承诺「防盗版」，只承诺「正版、干净、可追责」。

## 本地运行

前置：Docker（或本机装好 Go 1.25 / Node 20 + 本地 Postgres/Redis）。

```bash
# 全栈一键起（推荐）
make up                      # 等价 docker compose up --build
# backend  → http://localhost:8080/healthz
# frontend → http://localhost:3000

# 或分别在宿主机跑
make backend-tidy            # 生成 go.sum
make migrate-up              # 建表（需 golang-migrate CLI；或设 AUTO_MIGRATE=true 让进程自迁移）
make backend-run
make frontend-dev
```

> 注意：首次构建后端需联网拉取 Go 依赖并生成 `go.sum`（`make backend-tidy`）；前端需 `npm install`。
> 数据库迁移：`make migrate-up`/`migrate-down`/`migrate-create name=xxx`；进程内自迁移设 `AUTO_MIGRATE=true`（compose 已默认开启）。迁移文件在 [`backend/migrations/`](backend/migrations/) 并编译进二进制。

## 演示（沙箱全栈，无需 Docker）

后端二进制 + 本地 Postgres + 前端 `next start` 即可跑通完整闭环。后端用沙箱支付（`PAYMENT_PROVIDER=mock`）与本地存储驱动；开发态启用 `POST /payments/dev/mark-paid` 模拟支付成功，便于演示。

```bash
# 后端（开发态：自动迁移 + 自动实名 + 沙箱支付）
cd backend && AUTO_MIGRATE=true KYC_AUTO_APPROVE=true APP_ENV=development \
  DATABASE_URL='postgres://app@localhost:5432/app?sslmode=disable' \
  STORAGE_DIR=./data/storage go run ./cmd/api      # :8080
# 前端
cd frontend && npm install && npm run build && npm run start   # :3000
```

闭环（已在浏览器端实测）：注册 → 实名 → 浏览/预览 → 下单 → 沙箱支付 → 下载 → 确认收货 → 自动分账(卖家90%/平台10%) → 评价；卖家「收益」实时反映已结算金额。`cmd/devsql` 可把某账号提权为 `ops` 以使用运营后台审核。

## 进度（PR 计划见 docs §9）

**已完成 PR-01 ~ 18 全部可做范围 + 前端 Web 应用**，每个 PR 真库 e2e + GitHub CI 验证。

**尚需外部介入（外部墙，代码已留可插拔适配点）**：
- **支付分账真集成**：替换 `payment.MockProvider` 前必须完成 **Spike-2 + 法务**（资金二清是刑事红线，docs §2.1）。
- **对象存储**：用云凭证实现 `storage/oss.go`（生产为浏览器直传预签名）。

**中文检索已升级**：应用层结巴分词（`go-ego/gse`，纯 Go 无 cgo）+ PG `simple` `tsvector` + GIN 索引，词级全文检索 + `ts_rank` 排序（见 `platform/textseg`、migration 000003）。规模化时可平替 `zhparser` / ES+IK。
