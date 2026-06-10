# 前端测试 + 容错对等 — 设计文档

**日期**:2026-06-10
**分支**:`feat/frontend-test-resilience`(基于 `origin/main @ 5816cca`)
**作者**:独立工程会话

## 背景与动机

独立审计(`origin/main @ 5816cca`)结论:后端已接近生产级——`/readyz` 真 ping 数据库、安全头中间件(CSP/HSTS/X-Frame/nosniff,带测试)、CORS、优雅关闭、compute 幂等键、30/60/120 退避重试、append-only 审计、乐观锁状态机、22 套 race 测试零失败、完整 k8s 部署清单。

**前端是明显短板**:

- 零测试(`package.json` 仅 `dev/build/lint/typecheck`,无 Vitest/Jest/Playwright)。后端有 E2E + 22 套单测,前端一行测试都没有。
- 无 App Router 容错边界(无 `error.tsx`/`global-error.tsx`/`loading.tsx`/`not-found.tsx`)。组件抛错 = 白屏。
- 前端侧 `next.config` 无 `headers()`,SSR/静态资源层无安全头兜底。

**目标**:把前端工程严谨度拉到后端水平——组件测试 + 全栈浏览器 E2E + 容错边界 + CI 门禁。

## 范围决策(已与用户确认)

- **E2E 深度**:全栈真 E2E——Playwright 拉起真后端(`go run`)+ 临时 PG + 前端,跑真实注册→浏览→下单→支付(mock provider)→交付。与后端 HTTP E2E 严谨度对等。
- **CI 接入**:接入。组件测试每次 push 跑;E2E 独立 job 挂 push/PR-to-main。

## 组件设计

### 1. 测试基建(Vitest + RTL + MSW)

- Vitest,`jsdom` 环境;`@/*` 别名镜像 `tsconfig.json`(`@/* → ./*`)。
- `@testing-library/react` + `@testing-library/jest-dom` + `@testing-library/user-event`。
- MSW(Mock Service Worker)拦截后端 envelope `{code,message,data,request_id}`。
- 配置文件:`frontend/vitest.config.ts`、`frontend/vitest.setup.ts`。
- 新增脚本:`test`、`test:watch`、`test:coverage`。
- 选 Vitest 而非 Jest:与 Next 14 + ESM 摩擦小、启动快、配置少。

### 2. 组件/单元测试(挑有逻辑的单元,不追 100% 覆盖)

| 目标 | 重点验证 |
|------|----------|
| `lib/api.ts` | envelope 解析、Bearer 注入、**401 自动刷新重试**(401→refresh→retry;refresh 失败→登出清 token)、错误传播。**最高价值。** |
| `lib/auth.tsx` | AuthProvider 登录/登出/token 持久化(localStorage `adm_access`/`adm_refresh`) |
| `lib/i18n.tsx` | locale 切换、缺失键兜底 |
| `components/Protected.tsx` | 未登录跳转 |
| `register` / `login` 表单 | 校验、提交、错误展示、2FA 分支(`need_2fa` → challenge) |
| `components/ui.tsx` | 基元渲染 smoke + a11y role 断言 |

**TDD 立场**:对已实现逻辑写"特征化回归测试"锁住当前行为;若测出真 bug 就修(并在 commit 里说明)。

### 3. App Router 容错边界(双语 + 复用 `ui.tsx` 设计)

- `app/global-error.tsx`:顶层兜底,自渲染 `<html>`。
- `app/error.tsx`:段级边界 + `reset()` 重试按钮。
- `app/not-found.tsx`:404 页。
- `app/loading.tsx`:根加载骨架。
- 数据密集动态路由补段级 loading:`app/datasets/loading.tsx`、`app/datasets/[id]/loading.tsx`、`app/orders/loading.tsx`。
- 全部走现有 `lib/i18n.tsx` 双语模式,用 `components/ui.tsx` 组件着色。

### 4. `next.config.mjs` 安全头

新增 `async headers()`,对所有路由返回:

- `Strict-Transport-Security: max-age=63072000; includeSubDomains; preload`
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Referrer-Policy: strict-origin-when-cross-origin`
- `Permissions-Policy: camera=(), microphone=(), geolocation=()`

(后端 API 响应已有自己的安全头中间件;这是 Next 自服务 SSR/静态/资源响应的兜底。)

### 5. 全栈 Playwright E2E

- `frontend/playwright.config.ts` + `frontend/e2e/global-setup.ts`:
  1. 复用后端测试的 `initdb`/`pg_ctl` 起临时 PG。
  2. `go run ./cmd/api`,环境:mock 支付 provider、`AUTO_MIGRATE=true`、`KYC_AUTO_APPROVE=true`、`STORAGE_DRIVER=local`。
  3. `next build && next start`(standalone)指向后端。
  4. 启动前轮询 `/readyz` 做就绪门(防 flake)。
- specs(浏览器级镜像后端 journey):
  - `e2e/register-login.spec.ts`:注册→进站;登录;2FA-off 路径。
  - `e2e/purchase-journey.spec.ts`:浏览数据集→详情→下单→支付(mock)→交付可见。
  - `e2e/resilience.spec.ts`:未知路由命中 not-found;强制报错命中 error boundary。
- 脚本:`e2e`、`e2e:ui`。
- 防 flake:Playwright 自动等待 + web-first 断言;`/readyz` 就绪门;teardown 优雅停 PG/后端。

### 6. CI 接入(`.github/workflows/ci.yml`)

- `frontend-unit` job:`npm ci → typecheck → lint → test(Vitest) → build`,每次 push。
- `frontend-e2e` job:装 Playwright 浏览器(缓存)、编 Go 后端 + 前端、起 PG、跑 Playwright。挂 push/PR-to-main。

## 测试方式

- `lib/api.ts` 刷新逻辑:先写失败测试(401→refresh→retry),再验证现有实现通过(特征化回归);测出 bug 则修。
- 容错边界:写断言验证边界渲染,再加边界文件。

## 风险与缓解

- Playwright 在 CI 需下载浏览器 → 缓存 `~/.cache/ms-playwright`。
- E2E job 最慢(~分钟级)→ 独立 job,不阻塞快反馈的 unit job。
- MSW + Next 环境分歧 → `api.ts` 用 node 环境,组件用 jsdom(Vitest 按文件 `// @vitest-environment` 或 workspace)。
- npm 安装可能 ECONNRESET → `--fetch-retries=5`。

## 本轮不做(YAGNI)

视觉回归/截图 diff、完整 a11y 审计(只做基础 role 断言)、其余三条线(可观测性/安全纵深/压测/容量)。

## 验收标准

1. `npm run test` 绿,覆盖 `api.ts` 401-刷新关键路径 + 至少 auth 表单。
2. 四个容错边界文件存在,Playwright `resilience.spec` 验证 not-found 与 error 渲染。
3. `next.config` `headers()` 返回上述安全头(E2E 或单测断言)。
4. 三个 Playwright spec 全栈跑通。
5. `ci.yml` 含 `frontend-unit` + `frontend-e2e` 两个 job。
6. 既有检查不回归:`tsc --noEmit`、`next lint`、`next build` 仍全绿。
