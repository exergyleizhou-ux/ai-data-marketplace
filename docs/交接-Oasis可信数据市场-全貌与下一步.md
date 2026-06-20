# 交接 · Oasis 可信数据市场 — 全貌与下一步(capstone)

> 一个会话从"给 C2D 加输出闸"一路做到"一个真上线、有真实公开数据、定位清晰的可信数据市场 + 一条变现产品线"。这是单一自包含的接手文档。

## 0. 一句话

**Oasis(绿洲)= 一个「可信数据市场」**(创业项目/公司)。卖家来上架卖数据,平台提供「完整性体检 + 可溯源/可复现证书」这层信任并抽成。**主攻科研/科学数据当滩头**;**冷启动靠"验证先行"**(先收卖家的验证费当快钱,撮合抽成是第二幕)。**代码侧到顶了;瓶颈 100% 在 GTM——创始人去拉第一批科研卖家。**

## 1. 战略(已锁定,别再推翻)

详见 [`docs/战略-可信数据市场-定位与冷启动.md`](战略-可信数据市场-定位与冷启动.md)。要点:
- **模式 B**:别人上架卖、平台抽成 + 盖"已验证"戳。护城河是平台级"可验证",不是某个 SaaS。
- **滩头 = 科研数据**:PaperGuard(已发表、有真实用户)= 拉第一批卖家的现成渠道 + 信任背书;踩 AI-for-science + 可复现风口。
- **冷启动 = 验证先行**:卖家付费做"完整性体检 + 拿证书" → 第一笔快钱 + 把数据吸进货架 → 一键上架卖 → 买家为可信数据而来 → 抽成。
- **快钱在卖家的验证/上架费,不在撮合抽成**(抽成是第二幕)。
- **别做**:什么都卖的大超市;只盯抽成;把公司缩成单个 SaaS;对证书 over-claim。

## 2. 已建好 + 在跑的(产品已基本支撑战略)

- **数据市场**(H1–H7):上架/买卖/卖家/订单/搜索;**每个数据集上架自动跑质量验证**(`quality` 模块 → authenticity band + `quality_verified`),列表显示"已验证"徽章。→ "每份数据都已验证"在产品里**已经是真的**(缺的是 framing,本轮已补)。
- **可用不可见 C2D**(9 个旗舰算法 + 证书 + `/verify` + 真 DCAP TDX 验证 + L2 fail-closed + 输出闸)。
- **Oasis Verify(变现产品线,PR #240–247)**:自助 API key + 计量中间件、`POST /api/v1/screen`(传数据集→报告+可验证证书)、Stripe 计费(代码全接、TDD,差你的 key)、密钥控制台 `/verify-api/keys`、落地页 `/verify-api`、GTM 工具包 + 一键 VPS 部署脚本。**整条流在 live 端到端验证过**(cert `VO-614914DB868B` 等)。
- **定位已上主干**(#247):首页"每一份数据,都经过验证";卖家工作台冷启动钩子。

## 3. 现在机器上跑着什么(临时演示)

| 服务 | 地址 | 说明 |
|---|---|---|
| 主后端 | `:8080`(DB `ai_data_marketplace`,storage `/Users/lei/oasis-live-storage`) | 主 live,**重启要 `AUTO_MIGRATE=true` 不是 =1**(配置 gotcha) |
| 主前端 | `:3100` | 指向 :8080 |
| **公开 demo 后端** | `:8090`(隔离 DB `verify_public`,storage `oasis-verify-public-storage`) | 不碰主数据,安全;已 seed **4 个真实 UCI 科研数据集(全已验证)** |
| **公开 demo 前端** | `:3200` → cloudflared 隧道 | seed 脚本 `/tmp/seed-public-market.sh` |
| 公网隧道 | 见 `/tmp/fe_url.txt` / `/tmp/be_url.txt` | **不稳——用户网络在拦 cloudflared(QUIC/HTTP2 到 argotunnel 被拦),时通时断。临时隧道在此网络下天生不稳,稳定只能上 VPS。** |

公开 demo 市场已截图验证:4 个数据集各带绿色 "Clean(已验证)" 徽章,跨 生物/材料/植物/化学。

## 4. 下一步(诚实分工)

**创始人(绕不开,只有你能做)——这是唯一能让它转起来的事:**
1. 用 PaperGuard 用户 + 科研圈,列 20–50 个**手上有数据集**的目标卖家。
2. 一对一聊一句:"我做了个市场,能给你的数据集出一张可验证的'已认证'证书,挂上来卖、买家更敢买更愿付高价,愿意挂一份试试吗?"
3. 拿 3–5 个真卖家上架 → 有了供给 + 第一批可收验证费的对象 = 真启动。

**代码侧(我能做,但已非瓶颈;按需):**
- 给我**一台 VPS 的 SSH**(部署密钥 `~/.ssh/oasis_deploy.pub` 已生成)→ 我跑 `scripts/vps-deploy.sh` 真部署上稳定公网(含已 seed 的科研数据)。
- 接 **Stripe**(你建 test key)→ 端到端跑通 free→pro。
- 更多科研垂直样板数据 / 文案。

## 5. 关键 gotcha / 配方

- 重启主后端:`AUTO_MIGRATE=true COMPUTE_RUNNER=docker STORAGE_DIR=/Users/lei/oasis-live-storage go run ./cmd/api`(=true!)。
- seed 公开市场:`/tmp/seed-public-market.sh`(targets `verify_public` + 公开 storage;quality_checks 行驱动"已验证"徽章:一条 format=pass + 一条 authenticity report `{applicable:true,band:"clean",score}`)。
- 真实数据出证书:`/tmp/realdata-cert.sh`(主库)。
- 隧道:`/tmp/cloudflared tunnel --url http://localhost:<port>`(本网络不稳)。
- 全部 PR #230–247 已并入 origin/main。记忆见 `portfolio-strategy.md`(自动加载)。
