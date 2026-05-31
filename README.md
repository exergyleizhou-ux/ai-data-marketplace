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

## 开发路线（PR 计划见 docs §9）

已完成：**PR-01 脚手架** · **PR-02 数据库 + 迁移** · **PR-03 统一响应/错误码/中间件**。
下一步：PR-04 注册登录 + JWT（依赖 PR-02/03）。

正式编码前必须完成 **Spike-2（分账/担保支付闭环可行性）** —— 拉法务 + 持牌方一起做，确认可行再写支付代码。
