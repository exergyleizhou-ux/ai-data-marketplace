# Oasis Verify — 上线赚钱 Go-Live Runbook

> 从「已合并的代码」到「线上 + 能收钱」的精确剩余步骤。代码侧能做的都做了;这份清单是**你**要做的外部动作(部署 / Stripe / 获客),每步都标了状态。

## 0. 现状(代码侧已就绪)

| 能力 | 状态 | 位置 |
|---|---|---|
| 自助 API key(签发/列表/吊销,sha256 存储,月计量) | ✅ 建好 + TDD | `backend/internal/modules/apikey`(PR #240/#241) |
| API-key 鉴权 + 计量中间件(401/429) | ✅ | `apikey.APIKeyAuth` |
| **自助验证端点 `POST /api/v1/screen`**(上传数据集 → 报告 + 可验证证书) | ✅ 建好 + TDD | `compute.ScreenAdhoc` / `RegisterVerifyAPI` |
| 免费档(5 次/月)即开即用 | ✅ | tier 默认 free |
| 英文落地页 + 定价 + Nav | ✅ | `frontend/app/verify-api/page.tsx` |
| 订阅升级的目标(`SetTier` 改一个账号所有 key 的 tier) | ✅ 建好 + TDD | `apikey.Service.SetTier` |
| Stripe webhook → SetTier 的接线 | 🟡 待接(下面 §2,小活,需你的 Stripe) | — |
| 生产部署配置 | 🟡 已有 compose,需你的云 + 域名(§1) | `docker-compose.prod.yml` |

## 1. 上线(go-live)——把它放到公网

1. **开一台云 VM**(国际先用便宜的:Hetzner / DigitalOcean / Fly.io;国内:阿里云 ECS)。装 Docker。
2. **部署**:`docker-compose.prod.yml` 已就绪(postgres + redis + backend + frontend + 本地 registry)。设置 `.env`:`POSTGRES_*`、`APP_ENV=production`、`COMPUTE_RUNNER=docker`、`STORAGE_DIR`(挂一个持久卷,**别用 /tmp**)、`NEXT_PUBLIC_API_BASE_URL=https://你的域名/api/v1`。`docker compose -f docker-compose.prod.yml up -d`。
3. **域名 + TLS**:用 Caddy/Nginx 反代 + Let's Encrypt(几行配置)。
4. **注册筛查算法**(关键——`/screen` 靠它):**直接跑 `scripts/register-verify-screener.sh`**(幂等:构建+digest-钉死推 registry → ops 注册+批准+trusted)。没它 `/screen` 会返回「no trusted integrity-screen algorithm registered」。
5. **冒烟测试**:登录 → `POST /api/v1/api-keys` 拿 key → `curl -X POST https://域名/api/v1/screen -H "X-API-Key: sk_live_…" -F file=@a.csv` → 应返回报告 + `certificate_id` → `GET /api/v1/verify/<cert>` 可验证。

**做完这步 = 免费档已上线、能注册、能用、能引流。**

## 2. 赚钱(charge)——接 Stripe(剩下的唯一代码小活 + 你的账号)

Stripe SDK 已是依赖(`stripe-go/v79`,市场支付在用)。剩下:

1. **Stripe 账号**:建 Products + Prices(Pro $29/mo、Scale 自定义),记下 price IDs。
2. **环境变量**:`STRIPE_SECRET_KEY`、`STRIPE_WEBHOOK_SECRET`、一个 `PRICE_ID→tier` 映射(如 `price_xxx=pro`)。**先用 test-mode key 验证全流程。**
3. **接两个端点**(小活,目标 `SetTier` 已建好):
   - `POST /billing/checkout`(JWT-authed)→ `stripe.CheckoutSession` 创建订阅会话,`metadata.account_id = 当前用户`,成功跳回落地页。
   - `POST /billing/stripe/webhook`(公开,**验签** `webhook.ConstructEvent`)→ 处理 `checkout.session.completed` / `customer.subscription.updated|deleted`:从 metadata 取 account_id,price→tier 映射,调 **`apikeySvc.SetTier(ctx, accountID, tier)`**(已建)。取消订阅 → SetTier "free"。
4. **落地页加按钮**:Pro 卡片的 CTA 调 `/billing/checkout`。
5. **上线切换**:test-mode 跑通后换 live key。

> 为什么这步留给你接:它**必须**用你的 Stripe 账号(price IDs、密钥、webhook 端点登记)才能测;处理钱的代码不该在没有真实账号时盲写。目标 `SetTier` 已建好 + 测过,接线是 ~半天的活。

## 3. 获客(distribution)——代码替不了,但有素材

- **病毒钩子**:可嵌入「Oasis-verified ✓」徽章(`/verify/:cert_id/badge.svg` 已有)。
- **内容引流**:写一篇「我把 HuggingFace 上最热的 N 个数据集做了完整性体检,发现了这些」——用 `/screen` 真跑,诚实有料。
- **渠道**:Show HN、r/MachineLearning、X(AI 数据质量是热点)、Product Hunt、相关 Discord/Slack。
- **落地页**已就绪(`/verify-api`),把流量导到那里 → 注册 → 拿免费 key → 试 → 转 Pro。

## 4. 诚实边界

代码侧把「能自助收钱的产品」建到了门口:免费档现在就能上线用;付费只差**你接 Stripe**(必须你的账号);**钱来自客户 = 你的获客**,代码替不了。先用免费档上线验证需求,有信号了再花半天接 Stripe 收第一笔。
