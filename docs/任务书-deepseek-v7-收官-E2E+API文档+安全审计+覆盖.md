# 任务书 v7 给 DeepSeek V4 Pro — 收官阶段(方向 W+X+Y+Z)

**基线**:`origin/main`(等 PR-V 2FA 合并后)
**审核人**:Claude Code(Opus 4.8)
**这是「本地可做」工作的最后一批**。做完 W+X+Y+Z,**所有不需要外部资源的工作就彻底完成了**,项目进入「只差部署凭证/法务/硬件」的终态。

---

## 0. 本阶段定位(读懂再动手)

绿洲项目的**产品功能面已经完整**:16 后端模块、22 迁移、C2D L1/L2/L3、ops 看板、通知、合规(PIPL 导出/注销)、2FA、邮件、异常告警、审计、提现、Q&A、收藏、凭证。

剩下能在本地做的只有**「成熟度/收官」**四件事:
- **W**:全栈 HTTP 端到端测试(目前只有 gated docker e2e,没有跨模块关键旅程 e2e)
- **X**:OpenAPI 3.0 API 文档(目前完全没有)
- **Y**:安全 & 限流覆盖审计 + 补齐(系统性查漏)
- **Z**:补 DeepSeek 之前跳过的 handler 集成测试 + 薄模块覆盖

**做完这四个,本地 runway 真正清零。** 之后是外部门控项(见 Part FINAL)。

---

## 1. 通用铁律(每个 PR)

```bash
cat ~/ai-data-marketplace/CLAUDE.md      # 23+ 条 gotchas,全读
git fetch origin && git log origin/main -3
```

- `gofmt -w . && goimports -w .` 两个都跑
- `db.RunMigrations(dsn)`,禁裸 CREATE/DROP TABLE
- 新 `service.go`/`repo.go` 配 `_test.go`,测试名跟 spec 一字不改
- `.tsx` smart-quote 扫描 = 0(你已经连续几个 PR 做到了,保持)
- **CLAUDE.md gotcha 末尾单独 `docs(claude):` commit**(PR-T 漏过一次,别再漏)
- **4 commits 序**:① test → ② backend → ③ frontend(无前端则跳)→ ④ docs(claude)
- **必须开 PR + 等 CI 绿 + 等我审过才 merge**(PR-V 你忘了开 PR — 这次每个都要开 PR 让 CI 真跑)
- **不许自 merge**

---

## 2. PR 顺序

| PR | 方向 | 价值 | 估算 | 测试 |
|---|---|---|---|---|
| **PR-W** | 全栈 HTTP E2E 测试套件 | 关键旅程回归网,纯 Go 无浏览器(**对你最友好**) | ~700 行 | 6 e2e 场景 |
| **PR-X** | OpenAPI 3.0 规范 + /docs 路由 | API 可被集成/审计 | ~600 行 | 3 |
| **PR-Y** | 安全 & 限流覆盖审计 + 补齐 | 生产安全基线 | ~400 行 | 8 |
| **PR-Z** | handler 集成测试补全 + 薄模块覆盖 | 清最后测试债 | ~400 行 | 12 |

严格顺序:W 合并后才开 X。

---

# PR-W · 全栈 HTTP 端到端测试套件

## W.0 为什么(必读)

现有测试都是**模块级**(repo/service 集成 + 少量 handler)。**没有一个测试**走完整个跨模块旅程:注册 → KYC → 上架 → 审核 → 浏览 → 下单 → 支付 → 交付 → 评价。任何一个模块改动破坏了跨模块契约(如 order→payment→delivery 的状态传递),现有测试**抓不到**。

**对 DeepSeek 特别友好**:这是**纯 Go HTTP 客户端 e2e**,不需要浏览器/多模态。起一个真 `httptest.Server`(挂完整 gin 路由)+ 真 ephemeral PG,用 `http.Client` 打真实 HTTP 请求走完整流程。

## W.1 文件结构

```
backend/internal/e2e/
  harness_test.go    (起完整 server + PG + http client 帮手)
  journey_test.go    (6 个端到端场景)
```

**注意**:放在新包 `internal/e2e`,导入 `internal/server` 起完整应用。用 build tag `//go:build e2e` 隔离(默认 `go test ./...` 不跑,CI 加一个 `go test -tags=e2e ./internal/e2e/` job;**或**不加 tag 直接靠 DATABASE_URL skip — 与现有集成测试一致,**选后者更简单**)。

## W.2 Harness(harness_test.go)

```go
package e2e

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    "os"
    "testing"

    "github.com/lei/ai-data-marketplace/backend/internal/config"
    "github.com/lei/ai-data-marketplace/backend/internal/platform/db"
    "github.com/lei/ai-data-marketplace/backend/internal/server"
)

// e2eEnv holds a running test server + helpers.
type e2eEnv struct {
    t      *testing.T
    srv    *httptest.Server
    client *http.Client
}

func newE2E(t *testing.T) *e2eEnv {
    t.Helper()
    dsn := os.Getenv("DATABASE_URL")
    if dsn == "" {
        t.Skip("DATABASE_URL not set; skipping e2e")
    }
    if err := db.RunMigrations(dsn); err != nil {
        t.Fatalf("migrate: %v", err)
    }
    // 关键:用真实 server 构建,把 KYC 自动通过、mock payment、in-process compute 都打开
    // (参考 server.New 的构建方式 + 现有集成测试如何起 server)
    cfg := config.Config{ /* 看 config.Config 字段,设 DATABASE_URL、APP_ENV=test、
                             KYC_AUTO_APPROVE=true、PAYMENT_PROVIDER=mock、COMPUTE_RUNNER=mock 等 */ }
    app, err := server.New(cfg)  // ← 看 server 包真实构造函数签名,可能是 server.New(cfg) 或 NewServer
    if err != nil {
        t.Fatalf("server: %v", err)
    }
    srv := httptest.NewServer(app.Handler())  // ← 看 server 暴露 http.Handler 的方法名
    t.Cleanup(srv.Close)
    return &e2eEnv{t: t, srv: srv, client: srv.Client()}
}

// post/get/put 帮手:带 bearer token,返回 status + 解析 body。
func (e *e2eEnv) do(method, path, token string, body any) (int, map[string]any) {
    e.t.Helper()
    var rdr *bytes.Reader
    if body != nil {
        b, _ := json.Marshal(body)
        rdr = bytes.NewReader(b)
    } else {
        rdr = bytes.NewReader(nil)
    }
    req, _ := http.NewRequestWithContext(context.Background(), method, e.srv.URL+"/api/v1"+path, rdr)
    if token != "" {
        req.Header.Set("Authorization", "Bearer "+token)
    }
    req.Header.Set("Content-Type", "application/json")
    resp, err := e.client.Do(req)
    if err != nil {
        e.t.Fatalf("%s %s: %v", method, path, err)
    }
    defer resp.Body.Close()
    var out struct {
        Data    map[string]any `json:"data"`
        Code    int            `json:"code"`
        Message string         `json:"message"`
    }
    _ = json.NewDecoder(resp.Body).Decode(&out)
    return resp.StatusCode, out.Data
}

// registerUser 帮手:注册并返回 access token + user id。
func (e *e2eEnv) registerUser(account, role string) (token, userID string) {
    // POST /auth/register → 返回 tokens + user
    // 注意 account 用 crypto/rand 后缀防唯一冲突(CLAUDE.md gotcha)
}
```

**⚠️ 实现前必做**:`grep "func New" backend/internal/server/server.go` 看真实构造签名 + 暴露 handler 的方法(可能是 `app.Router()` / `app.Handler()` / `app.Engine()`)。**照真实签名写**,不要照我这里猜的。

## W.3 六个端到端场景(journey_test.go)

```go
// 1. 完整买卖闭环
func TestE2E_FullPurchaseJourney(t *testing.T) {
    // register seller → register buyer
    // seller: 创建 dataset → 上传(或直接 seed 到 published,看现有 upload 流程能否走通)
    //   ⚠️ 如果分片上传 e2e 太重,可以用 ops 直接把 dataset 置 published(看有无 ops 端点),
    //      或 seed 一个 published dataset 后走购买流程。重点是 order→pay→deliver→review。
    // buyer: 浏览 GET /datasets → GET /datasets/:id
    // buyer: POST /orders → POST /payments (mock) → 确认 paid
    // buyer: 确认交付 → 评价
    // 断言:每步 HTTP status 正确,最终 order status=settled/confirmed,review 落库
}

// 2. 2FA 登录全流程(PR-V 的端到端验证)
func TestE2E_TwoFactorLoginFlow(t *testing.T) {
    // register → enroll 2FA → verify-enrollment(用 totp 库生成真码)
    // → login(返回 need_2fa + challenge_token,无 access)
    // → /auth/2fa/verify(challenge + totp 码)→ 拿到真 tokens
    // 断言:login 不直接给 access;challenge 后才给
}

// 3. 密码重置全流程
func TestE2E_PasswordResetFlow(t *testing.T) {
    // register → request reset(拿不到 token,因为走邮件;
    //   ⚠️ e2e 没真邮件 → 直接查 password_reset_tokens 表拿 hash 对应的明文?不行,存的是 hash。
    //   方案:request reset 后,从 DB 读最近一条 token 记录;但明文不存。
    //   折中:让 e2e 直接调 service 层 RequestPasswordReset 返回的 token(如果 service 在 test 模式返回),
    //   或这个场景只断言「request 总返回 ok(防枚举)」+ 「不存在的账号也返回 ok」,
    //   complete 部分用 service 层单测覆盖(已有)。诚实标注 e2e 限制。)
}

// 4. 卖家提现流程(PR-P)
func TestE2E_WithdrawalApprovalFlow(t *testing.T) {
    // seller 有 settled 收益(需先走一单 settled,或 seed)
    // seller: POST /sellers/me/withdrawals
    // ops: GET /admin/withdrawals → approve → complete
    // 断言:状态机流转 + seller 收到通知(查 GET /users/me/notifications)
}

// 5. 收藏 + 通知联动(PR-L)
func TestE2E_WatchlistNotificationFlow(t *testing.T) {
    // buyer watch 一个 published dataset
    // seller 发新版本(触发 review→published)
    // 断言:buyer 的 GET /users/me/notifications 出现 dataset_updated
}

// 6. C2D 沙箱计算闭环(L1, mock runner)
func TestE2E_ComputeJobJourney(t *testing.T) {
    // seller 开 compute offer → buyer 购买 entitlement → 提交 job(mock runner)
    // → 轮询 job 直到 released → 下载输出
    // 断言:输出非空,job status=released
}
```

**实用主义**:有些场景(如分片上传、真邮件 token)e2e 化成本高,**允许用 seed 简化前置**(直接 INSERT 一个 published dataset / settled order),把测试重心放在**跨模块契约**上。每个简化点写注释说明。

## W.4 CI

`.github/workflows/ci.yml` 的 backend job 已经设 DATABASE_URL → e2e 测试会自动跑(因为不 skip)。**确认** e2e 包被 `go test -race ./...` 覆盖到(在 internal/ 下就会)。

## W.5 我会查的

- [ ] `internal/e2e/` 包真起 `httptest.Server` + 完整 gin handler(不是 mock handler)
- [ ] 6 个场景都走**真 HTTP**(`http.Client.Do`),不是直接调 service
- [ ] harness 用真实 `server.New`(照实际签名,不照我猜的)
- [ ] 账号用 crypto/rand 后缀防唯一冲突
- [ ] 简化点(seed 前置)都有注释说明为什么
- [ ] 不 skip(DATABASE_URL 设了就真跑)
- [ ] CLAUDE.md 加 1 条 gotcha(e2e harness 的真实构造经验)

## W.6 不许做

| ❌ | 原因 |
|---|---|
| 引入 Playwright / chromedp / 浏览器 | 纯 Go HTTP e2e 足够,且你无多模态 |
| 让 e2e 依赖真 Stripe / 真 SMTP / 真 docker | 全用 mock provider |
| 把 e2e 写成 mock 一切(失去 e2e 意义) | 必须真 server + 真 PG |

---

# PR-X · OpenAPI 3.0 规范 + /docs 路由

## X.0 为什么

项目有 **~80 个 HTTP 端点**,但**零 API 文档**。任何前端/第三方/审计想用 API 都得读源码。补一份 OpenAPI 3.0 spec + 一个 /docs 页面渲染它。

## X.1 交付物

```
backend/api/openapi.yaml          (手写 OpenAPI 3.0 规范,覆盖所有端点)
backend/internal/modules/docs/    (serve openapi.yaml + Swagger UI 静态页)
  handler.go
backend/api/openapi_test.go       (校验 yaml 合法 + 覆盖率检查)
```

## X.2 openapi.yaml 内容

OpenAPI 3.0,按 tag 分组:
- `auth`(register/login/refresh/logout/2fa/password-reset)
- `datasets`(CRUD/upload/quality/certificate/croissant/datasheet/questions)
- `orders` / `payments` / `delivery`
- `compute`(C2D 家族)
- `notifications` / `watchlist` / `withdrawals` / `compliance`
- `admin`(ops-gated 全家:reconciliation/outbox/audit-logs/anomalies/withdrawals/account-deletions)
- `search` / `verify`(公开)

每个端点至少写:`summary`, `parameters`/`requestBody`, `responses`(200 + 主要错误码), `security`(bearer / 无)。

**统一响应 envelope** 用 components/schemas 定义一次:
```yaml
components:
  schemas:
    Envelope:
      type: object
      properties:
        code: { type: integer }
        message: { type: string }
        data: { type: object }
        request_id: { type: string }
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT
```

## X.3 /docs 路由

```go
// docs/handler.go
func Register(rg *gin.RouterGroup, specFS embed.FS) {
    // GET /docs/openapi.yaml → 返回 embed 的 yaml(Content-Type: application/yaml)
    // GET /docs → 返回一个极简 HTML 页,用 CDN 的 Swagger UI 或 Redoc 加载 /docs/openapi.yaml
    //   (HTML 内联,引一个 <script src="https://cdn.../swagger-ui...">,零本地 deps)
}
```

`openapi.yaml` 用 `//go:embed` 嵌入(参考现有 migrations 的 embed 模式)。

## X.4 测试(3 个)

```go
func TestOpenAPI_IsValidYAML(t *testing.T)
//   解析 openapi.yaml(用 gopkg.in/yaml.v3,看 go.mod 是否已有;没有则纯 yaml unmarshal 到 map)
//   断言无错 + 顶层有 openapi/info/paths

func TestOpenAPI_CoversAllRegisteredRoutes(t *testing.T)
//   起 gin engine,枚举 engine.Routes(),
//   对每条非静态路由,断言在 openapi.yaml 的 paths 里有对应条目
//   (path 参数 :id → {id} 转换)
//   ⚠️ 这个测试会强制 spec 与真实路由同步 —— 高价值

func TestDocsHandler_ServesYAML(t *testing.T)
//   httptest GET /docs/openapi.yaml → 200 + 非空 body
```

`TestOpenAPI_CoversAllRegisteredRoutes` 是关键:它让文档**不会漂移**。如果有路由没文档,测试挂。

## X.5 我会查的

- [ ] openapi.yaml 是合法 OpenAPI 3.0(version、info、paths)
- [ ] **覆盖率测试**:枚举真实路由 vs spec paths,缺一即 fail
- [ ] /docs 页能加载(httptest 验 yaml 端点)
- [ ] 用 //go:embed 不是运行时读文件
- [ ] 统一 envelope + bearerAuth 在 components 定义
- [ ] CLAUDE.md 加 1 条 gotcha

## X.6 不许做

| ❌ | 原因 |
|---|---|
| 引入重型 swagger 生成框架(swaggo 注解满天飞) | 手写 yaml + 覆盖率测试更可控 |
| /docs 暴露在生产无鉴权(如果含敏感信息) | /docs 可公开(只是 API 形状),但别把 secret 写进 example |

---

# PR-Y · 安全 & 限流覆盖审计 + 补齐

## Y.0 为什么

各 PR 零散加了 rate limit / role gate,但**从没系统性查过全集**。可能有 mutation 端点漏了限流,或 admin 端点漏了 role gate。这一刀做系统审计 + 补齐。

## Y.1 第一步:产出覆盖矩阵(写进 PR description)

枚举所有路由(`engine.Routes()` 或读各 router.go),做一张表:

| 端点 | 方法 | 鉴权 | 限流 | 状态 |
|---|---|---|---|---|
| /auth/register | POST | 公开 | 5/min | ✅ |
| ... | ... | ... | ... | 缺/有 |

**判定规则**:
- 所有**公开 mutation**(POST/PUT/DELETE 无需登录)**必须**有 rate limit
- 所有 `/admin/*` **必须**经 `auth.RequireRole("ops","admin")`
- 所有 `/users/me/*` `/sellers/me/*` **必须**经 authMW + 自报范围(`httpx.UserID(c)`)
- 公开 GET(浏览/搜索/verify)可无限流,但**预览类**(`/datasets/:id/preview`)应有限流(防爬)

## Y.2 第二步:补齐发现的缺口

对矩阵里标「缺」的,补上。**预期可能发现**:
- 某些 `/datasets/:id/questions` POST(PR-O)可能没限流 → 加(防垃圾提问轰炸)
- `/sellers/me/withdrawals` POST(PR-P)可能没限流 → 加(防重复提交)
- `/datasets/:id/watch`(PR-L)→ 评估是否需要

**每个补的限流写注释说明阈值依据。**

## Y.3 第三步:跨模块安全不变量测试

```go
// backend/internal/server/security_coverage_test.go
func TestAllAdminRoutesRequireOpsRole(t *testing.T)
//   枚举 engine.Routes(),对每条 path 以 /admin/ 开头的,
//   用一个普通 buyer token 打 → 必须 403(不是 200/404)
//   ⚠️ 这是真 e2e 风格的安全回归网

func TestAllUserScopedRoutesRejectAnonymous(t *testing.T)
//   对 /users/me/* /sellers/me/* 无 token 打 → 必须 401

func TestPublicMutationsAreRateLimited(t *testing.T)
//   对已知公开 mutation 端点,快速打 N+1 次 → 第 N+1 次 429
//   (至少覆盖 register/login/2fa-verify/password-reset)
```

## Y.4 测试(8 个)

- 3 个上述安全不变量测试(admin role / user-scoped auth / public rate limit)
- 5 个针对补齐的限流:每个新加限流的端点 1 个测试(超限 → 429)

## Y.5 我会查的

- [ ] PR description 含完整覆盖矩阵(所有路由)
- [ ] `TestAllAdminRoutesRequireOpsRole` 枚举真实路由,不是硬编码列表
- [ ] 补的限流都有阈值注释
- [ ] 不破坏现有端点行为(回归全过)
- [ ] CLAUDE.md 加 1 条 gotcha(系统性覆盖审计的发现)

## Y.6 不许做

| ❌ | 原因 |
|---|---|
| 给公开 GET 浏览端点加激进限流 | 影响正常用户 |
| 改鉴权中间件核心逻辑 | 只补覆盖,不改机制 |
| 把限流阈值设得没依据 | 每个阈值要能说出理由 |

---

# PR-Z · handler 集成测试补全 + 薄模块覆盖

## Z.0 为什么

PR-V 时 DeepSeek 跳过了 `auth` 的 handler 集成测试(「gin middleware 环境问题」)。`compute/handler_integration_test.go` 存在但可能覆盖不全。一些模块(notification/verify/anomaly)只有 repo+service 测试,没 handler 层测试。这一刀补齐。

## Z.1 交付

```
backend/internal/modules/auth/handler_integration_test.go    (补 PR-V 跳过的)
backend/internal/modules/<thin>/handler_test.go              (薄模块 handler 层)
```

**用 httptest + 真 gin engine**(不是 mock)。解决 DeepSeek 之前遇到的「gin middleware 环境问题」:正确的方法是用 `gin.New()` + 手动 `Use()` 需要的中间件 + 注册目标路由,或直接用 `server.New` 起完整栈(同 PR-W harness)。**复用 PR-W 的 harness 思路。**

## Z.2 必补的测试(12 个)

```go
// auth handler(PR-V 跳过的,现在补)
func TestAuthHandler_Enroll2FA_RequiresAuth(t *testing.T)          // 无 token → 401
func TestAuthHandler_Verify2FA_WrongCode_Returns401(t *testing.T)
func TestAuthHandler_PasswordResetRequest_AlwaysReturns200(t *testing.T)  // 防枚举
func TestAuthHandler_Login_With2FA_ReturnsChallenge(t *testing.T)

// notification handler(只有 repo 测试,补 handler 层)
func TestNotificationHandler_List_RequiresAuth(t *testing.T)
func TestNotificationHandler_MarkRead_OtherUser_Returns404(t *testing.T)  // IDOR via HTTP

// verify handler
func TestVerifyHandler_UnknownCert_Returns404(t *testing.T)
func TestVerifyHandler_KnownCert_Returns200(t *testing.T)

// anomaly handler
func TestAnomalyHandler_List_RequiresOps(t *testing.T)   // buyer token → 403
func TestAnomalyHandler_Acknowledge_TransitionsStatus(t *testing.T)

// qa handler
func TestQAHandler_Ask_RequiresAuth(t *testing.T)
func TestQAHandler_Answer_NonSeller_Returns403(t *testing.T)  // IDOR via HTTP
```

**重点**:这些 handler 测试验证的是 **HTTP 层契约**(status code、auth gate、IDOR 经 HTTP),与 service 层单测互补 —— service 测业务逻辑,handler 测 HTTP 边界。

## Z.3 我会查的

- [ ] auth handler 集成测试真起 gin engine(解决了之前的环境问题)
- [ ] IDOR 测试经**真 HTTP**(不是直接调 service)验 403/404
- [ ] admin 端点 buyer token → 403 验证
- [ ] 12 个测试名对齐
- [ ] CLAUDE.md 加 1 条(handler 集成测试 gin 环境的正确搭法 —— 这正是 PR-V 卡住的点,沉淀下来)

---

# Part FINAL · 做完 W+X+Y+Z 之后 —— 外部门控清单(交给用户)

W+X+Y+Z 合并后,**本地能做的全部完成**。以下是**只有用户能解锁**的项,DeepSeek 和 Claude Code 都做不了,需要真实凭证/法务/硬件/资金:

| 类别 | 待办 | 谁来做 |
|---|---|---|
| **支付** | 微信/支付宝**持牌方分账**签约 + 资金流向法务意见(二清刑事红线) | 用户 + 法务 + 持牌支付机构 |
| **部署** | 实名 / ICP 备案 / EDI 增值电信许可 / 云资源采购 | 用户 |
| **邮件** | 真 SMTP / 邮件服务商(SendGrid/阿里云邮推)凭证 | 用户 |
| **TEE(L2)** | TDX/SEV 机密计算云节点 + DCAP/KBS 部署 → 让 `runner_tee_tdx.go` 真跑 | 用户(开 TEE 云) |
| **MPC(L3)** | ≥2 个 Secretflow/SPU 节点 → 让真 PSI 跑(替换 mockMPC) | 用户(多方节点) |
| **法务** | ToS/隐私政策/数据交易许可协议**执业律师定稿** | 用户 + 律师 |
| **存管** | 资金存管机构选定(ToS 里的 placeholder) | 用户 |

> 代码侧**全部就绪**:支付有 PaymentProvider/SplitProvider/RefundProvider 接口(Stripe 已真跑测试模式);TEE 有 attester + KBS 客户端 + TDX 脚手架;MPC 有 MPCOrchestrator 接口;邮件有 EmailSender 接口 + SMTP 实现。**全是「换配置/接真实例」而非「写代码」。**

做完 v7,给用户一份**部署就绪清单**(可作为 PR-X OpenAPI 之外的一份 `docs/上线就绪清单.md`),把上面这张表 + 每项对应的代码接入点写清楚。

---

# Part X · 我审核会查的(跨 PR 通用)

```
[ ] 开了 PR(不是只推分支)+ CI 三 job 真跑绿
[ ] 4 commits 序:test → backend → frontend → docs(claude)
[ ] gofmt -l . && goimports -l . 空
[ ] go test -race -p 1 ./... 全过(含新 e2e,无 skip)
[ ] 前端(若改):tsc --noEmit + next lint + next build + smart-quote 0
[ ] CLAUDE.md 末尾 commit 加 ≥1 条本次实战 gotcha
[ ] PR description:改动 + 测试 + Skills learned + 自检表打勾
[ ] 不自 merge,等我点头
```

---

# Part Y · 执行顺序

```
1. (先确保 PR-V 已合并到 main)
2. git worktree add ~/ai-data-marketplace-W -b feat/e2e-suite origin/main
3. PR-W → 开 PR → CI 绿 → 等我审 → merge → remove worktree
4. git worktree add ~/ai-data-marketplace-X -b feat/openapi-docs origin/main
5. PR-X → ... 同上
6. git worktree add ~/ai-data-marketplace-Y -b feat/security-audit origin/main
7. PR-Y → ... 同上
8. git worktree add ~/ai-data-marketplace-Z -b test/handler-coverage origin/main
9. PR-Z → ... 同上
10. 全部合并后:写 docs/上线就绪清单.md(Part FINAL 那张表展开)→ 最后一个 PR
```

**做完这些,绿洲项目的本地工程就彻底完成了。** 剩下的是用户拿凭证/法务/硬件去「接真实例 + 部署」—— 代码已全部就位。

---

**从 PR-W 开始。每个开 PR、等 CI、等我审。完成一个告诉我。**
