# DeepSeek v5 任务书 — Wave 1+2+3 机械工作

> 配套 `交接-v5-完成路径与最终方案.md`。每节 = 一条可直接粘贴 DS 的完整 prompt。互相独立可并行。
> 产出落 `~/claudecode 信息站/deepseek-drop/v5/<编号>/`,Claude 验收→合并。
> **统一前提**:不许凭空发明 metric/接口名;Claude 提供"权威清单"的部分必须严格按清单写;CLAUDE.md 的 gotcha 必须遵守(JSONB NOT NULL、uuid[] 类型、optimistic state machine、smart quote、`db.RunMigrations`、CLAUDE.md 内全部 23 条)。

---

## DS-V5-1:Wave 1-2 卖家提现 UI

**目标**:目前后端 `api.requestWithdrawal` / `api.listMyWithdrawals` 已就绪、admin 端审批 UI 已就绪,但 `/earnings` 页面只是 45 行三个 Stat 卡,卖家无法发起提现。这是上线 blocker。

```
作为前端工程师,为 ~/ai-data-marketplace/frontend 修改 app/earnings/page.tsx,增加卖家提现完整流程。

权威清单(不许偏离):
- api.requestWithdrawal(amount_cents:int, channel:string, account_label:string) → Withdrawal
- api.listMyWithdrawals() → Withdrawal[]
- Withdrawal 字段:id, seller_id, amount_cents, channel(alipay|wechat|bank), account_label, status(pending|approved|rejected|completed), ops_note?, requested_at, processed_at?
- 状态机:pending → approved → completed  或  pending → rejected (终态)
- 通知 kind:withdrawal_approved / withdrawal_completed / withdrawal_rejected(已在 notifications 页声明)
- 风格参照 frontend/components/ui.tsx 已有的 Field/Input/Select/Button/Alert/Stat
- i18n 用 useT(zh, en)(参 lib/i18n.tsx)

产出文件(独立文件,不要塞进 page.tsx):
1. frontend/components/WithdrawalForm.tsx
   - 输入金额(显示为元,提交时 *100 转分)、channel(下拉 alipay/wechat/bank)、account_label(文本)
   - 限制:amount 必须 > 0、≤ withdrawable 余额(从 page 传入)
   - 校验失败 inline Alert;提交中按钮 loading
   - 成功后 callback onCreated(),显示 success Alert
   - **i18n 双语**:每个 label/button/error 用 useT
2. frontend/components/WithdrawalHistory.tsx
   - 展示 listMyWithdrawals() 结果的表格:申请时间 / 金额 / 渠道 / 账户 / 状态(用 Badge 颜色:pending=灰、approved=蓝、completed=绿、rejected=红)/ 处理备注
   - 加载中 Spinner、空态 Alert "暂无提现记录 / No withdrawals yet"
3. 修改 frontend/app/earnings/page.tsx:
   - 顶部保留三个 Stat 卡(withdrawable/pending/settled),全部加 useT 翻译
   - 中间放 <WithdrawalForm withdrawable={withdrawableCents} onCreated={refetch}/>
   - 底部放 <WithdrawalHistory key={refreshKey}/>
   - 删除原硬编码的 disclaimer 段,改用 useT
4. frontend/__tests__/WithdrawalForm.test.tsx 写 3 个测试:
   - 渲染时显示"申请提现 / Request withdrawal" 按钮
   - 金额 0 或大于 withdrawable 时点击提交,显示错误 Alert,api 不被调用(jest.mock api)
   - 合法输入提交,调用 api.requestWithdrawal 一次,触发 onCreated

【硬性要求】
- tsx 文件**绝不能**含 U+201C/U+201D 全角引号(CLAUDE.md 第 17 条)
- 所有用户可见文本必须 useT 双语
- `npx tsc --noEmit` 0 错误;`npx next lint` 0 警告;`npm run build` 成功;`npm test` 全过
```

---

## DS-V5-2:Wave 1-3 i18n 硬编码补 EN

**目标**:6 处用户面硬编码中文,违反"每页 i18n"承诺。

```
作为前端工程师,在 ~/ai-data-marketplace/frontend 把以下 6 处硬编码中文改成 useT(zh, en) 双语:

1. frontend/app/earnings/page.tsx — 整个页(h1 "卖家收益"、三个 Stat label、disclaimer 段落)
2. frontend/components/Protected.tsx:22-37 — "请先登录"、"需要运营权限"、"当前账号无权访问运营后台"、"需要完成实名认证"、"买卖数据前必须通过实名认证"、"去登录"、"去实名"
3. frontend/components/StripeCheckout.tsx:39,69,77,91,94 — no-pk Alert、error fallback、"支付处理中…"、"支付"、test-card hint
4. frontend/components/ui.tsx:101 — Spinner 默认 label "加载中…"
5. frontend/app/account/page.tsx:324 — alert(...) 改为 inline <Alert kind="success">,用 useT
6. frontend/components/Datasheet.tsx:105,120 — "语言 / Languages"、"保存中…"、"保存说明卡"
7. frontend/components/QualityReport.tsx:170,201 — innocent-explanations 切换标签、"未检出个人信息"

【EN 翻译参照】(必须用这些,不许自创):
- "卖家收益" → "Seller Earnings"
- "可提现 / 待结算 / 已结算" → "Withdrawable / Pending / Settled"
- "请先登录" → "Please sign in first"
- "需要运营权限" → "Operator access required"
- "当前账号无权访问运营后台" → "This account has no admin access"
- "需要完成实名认证" → "KYC verification required"
- "买卖数据前必须通过实名认证" → "KYC required to buy or sell data"
- "去登录" → "Sign in"
- "去实名" → "Submit KYC"
- "支付处理中…" → "Processing payment…"
- "支付" (按钮) → "Pay"
- "加载中…" → "Loading…"
- "保存中…" → "Saving…"
- "保存说明卡" → "Save datasheet"
- "已提交注销申请, 7 天冷静期内可撤销" → "Account deletion requested. You may revoke within the 7-day cooling-off period."
- "未检出个人信息" → "No PII detected"

【硬性要求】
- 不许把现有 zh 文案删掉,只是把硬编码 → useT
- 不许引入 U+201C/U+201D 全角引号(CLAUDE.md 第 17 条)
- 用 alert() 的两处全部改成 inline 组件
- tsc / lint / build / test 全过
```

---

## DS-V5-3:Wave 1-7 a11y 批量加 aria-label

**目标**:整个前端 3554 LOC 只有 16 个 aria-* 引用、0 个 alt= ,严重不达标。

```
作为前端工程师,在 ~/ai-data-marketplace/frontend 给以下控件补 aria-label / role / aria-live:

1. frontend/components/Nav.tsx:65-75 — 通知铃 <Link> 加 aria-label={t("通知 (#count 未读)", "Notifications (#count unread)")};未读 badge 加 role="status" aria-live="polite"
2. frontend/app/datasets/[id]/page.tsx:104-110 — 收藏星按钮加 aria-label(已有 title 但 title 是 zh 硬编码,改成 useT 并双重设置 title+aria-label)
3. frontend/app/admin/page.tsx:794-796 — JSON 展开三角形 "▲/▼" 加 aria-label={isOpen?t("收起","Collapse"):t("展开","Expand")} aria-expanded={isOpen}
4. frontend/components/ui.tsx Spinner — 加 role="status" aria-label={label}
5. frontend/components/StripeCheckout.tsx 按钮 disabled 时加 aria-busy="true"
6. frontend/app/login/page.tsx, register/page.tsx — form 加 aria-labelledby 指向 h1
7. 所有 <button> 只显示 icon(◀ ▶ ✕)的位置全部加 aria-label

【硬性要求】
- 不引入 zh 硬编码:所有 aria-label 用 useT
- 不引入 U+201C/U+201D
- tsc / lint / build / test 全过;可以新增 jsdom + axe 烟雾测试(如果你引入 axe,加进 devDependencies 并保持版本钉到 patch)
```

---

## DS-V5-4:Wave 1-8 admin 内容审核 Tab

**目标**:admin 9 个 tab 全是结构/订单/资金运营,没有内容举报通道。Q&A 和 review 出现滥用时 ops 无处下手。

```
作为前端工程师,在 ~/ai-data-marketplace/frontend/app/admin/page.tsx 增加第 10 个 tab "内容审核 / Content Moderation"。

后端假设(写给 Claude 后端补):
- GET  /api/v1/admin/qa/flagged?limit=20&offset=0 → 列出 status='flagged' 的 questions
- GET  /api/v1/admin/reviews/flagged?limit=20&offset=0 → 列出 status='flagged' 的 reviews
- POST /api/v1/admin/qa/{id}/resolve {action: "hide"|"restore"}
- POST /api/v1/admin/reviews/{id}/resolve {action: "hide"|"restore"}
- (后端实现 Claude 接管,你只做 UI + api 客户端绑定)

产出:
1. 在 frontend/lib/api.ts 加 4 个 api 函数 typed 签名(adminListFlaggedQuestions/adminListFlaggedReviews/adminResolveQuestion/adminResolveReview),即使后端尚未实现也要让 tsc 过
2. 在 frontend/app/admin/page.tsx 加 <ContentModerationTab/> 内嵌组件:两栏(问题/评论),每栏展示 list + 操作按钮(隐藏/恢复 + 备注 textarea),i18n 双语
3. 操作成功后 Alert 反馈,失败后 inline error
4. 该 tab 加进 ADMIN_TABS 数组的最后一项

【硬性要求】tsc / lint / build / test 全过;UI 与既有 9 个 tab 风格一致(同 Field/Input/Button)
```

---

## DS-V5-5:Wave 2-1 CD 流水线 release.yml

```
作为 DevOps 工程师,在 ~/ai-data-marketplace 增加 .github/workflows/release.yml:

触发:push tag v*.*.*(SemVer)
job 1 build-and-push:
  - actions/checkout@v4 + docker/setup-buildx-action@v3 + docker/login-action@v3 用 ghcr.io(GHCR_TOKEN secret)
  - 用 docker/metadata-action@v5 生成 tags(版本号 + latest)
  - docker/build-push-action@v5 build 并推 backend(backend/Dockerfile)和 frontend(frontend/Dockerfile)两个镜像
  - 输出 image digest 写到 step output
job 2 deploy-staging(needs: build-and-push)
  - environment: staging(必须配置 reviewers 才能进 — 在 manifest 留注释)
  - 用 azure/setup-kubectl@v4 + kubectl apply -k deploy/k8s/overlays/staging/(假设 DS-V5-7 已建)
  - 等 rollout 完成 + 跑 /readyz 烟雾 + 30 秒后查 5xx 率,若 > 1% 自动回滚
job 3 deploy-prod(needs: deploy-staging, environment: production with manual approval)
  - 同 staging,但 -k deploy/k8s/overlays/prod/
  - rollout + smoke

【硬性要求】所有 action 钉到 major tag(@v4/@v5);permissions: contents: read, packages: write;无 continue-on-error;能被 actionlint 通过
```

---

## DS-V5-6:Wave 2-2 Loki + Promtail 日志聚合

```
作为 SRE,在 ~/ai-data-marketplace/deploy/monitoring 增补 Loki + Promtail(或 Grafana Alloy),让后端 stdout 的 slog-JSON 被聚合并可查。

产出:
1. deploy/monitoring/docker-compose.monitoring.yml:加 loki(grafana/loki:3.0.0)+ promtail(grafana/promtail:3.0.0)两个 service;volumes 持久化;promtail 用 docker socket 自动发现
2. deploy/monitoring/loki/config.yml:单机 boltdb-shipper 配置(开发用)
3. deploy/monitoring/promtail/config.yml:抓 docker 容器 stdout,标签按 container_name/compose_service
4. deploy/monitoring/grafana/provisioning/datasources/loki.yml:注册 Loki 数据源 uid=DS_LOKI
5. deploy/monitoring/grafana/dashboards/mkt-logs.json:基础日志面板(LogQL: {container_name="backend"} | json,展示 level=error 计数 + 流式日志窗口)
6. README 加一节"日志聚合"

【硬性要求】镜像钉 minor 版本;loki/promtail 配置可被各自 -verify-config 跑过;能 docker compose config 校验通过
```

---

## DS-V5-7:Wave 2-3 Kustomize staging overlay

```
作为 SRE,在 ~/ai-data-marketplace/deploy/k8s 把现有清单重构为 kustomize base + overlays/{staging,prod}:

1. mv deploy/k8s/*.yaml → deploy/k8s/base/,在 base 加 kustomization.yaml 列出 resources
2. deploy/k8s/overlays/staging/:
   - kustomization.yaml: namespace: marketplace-staging, namePrefix: stg-, commonLabels: env=staging
   - patches:replicas 改 1、imagePullPolicy: Always、resources 减半
   - configMapGenerator: APP_ENV=staging、其他差异化环境变量
3. deploy/k8s/overlays/prod/:
   - kustomization.yaml: namespace: marketplace, replicas 不变,APP_ENV=production
4. .env.staging.example 文件(占位变量 + 注释)
5. 更新 deploy/README.md "如何部署 staging / prod"两节

【硬性要求】kustomize build overlays/staging 和 overlays/prod 都必须本地能跑通(注释里给命令);base 没有任何 env-specific 值
```

---

## DS-V5-8:Wave 2-4 restic 异地备份

```
作为 SRE,在 ~/ai-data-marketplace 把 deploy/backup/backup.sh 扩展为支持 restic 同步到 S3(或兼容对象存储如阿里云 OSS/MinIO):

1. backup.sh:在本地 pg_dump 落 PVC 后,若设置 BACKUP_RESTIC_REPO + BACKUP_RESTIC_PASSWORD 环境变量,运行 restic backup $BACKUP_DIR/marketplace-*.dump --tag oasis,daily 同步到 $BACKUP_RESTIC_REPO,再 restic forget --keep-daily 7 --keep-weekly 4 --keep-monthly 6 --prune;若未设则跳过(向后兼容)
2. deploy/k8s/base/60-backup-cronjob.yaml(在 #102 基础上):
   - 镜像改为同时包含 postgres-client + restic 的轻量层(或在 init 阶段 apk add restic)
   - env:BACKUP_RESTIC_REPO from secretKeyRef marketplace-secrets restic-repo;BACKUP_RESTIC_PASSWORD from restic-password;AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY 从对应 key
3. deploy/backup/README.md:加一节"异地备份配置",示例 restic-server / B2 / S3 / MinIO 各家 repo url 写法
4. deploy/backup/drill.sh:在原 drill 之外增加 RESTIC_DRY_RUN=1 路径(只校验 restic forget --dry-run 不实跑)

【硬性要求】不破现有 backup.sh / restore.sh / drill.sh 行为(可不设 restic 时跑 backup-drill 仍绿);命令 bash -n 语法过
```

---

## DS-V5-9:Wave 2-5 Alertmanager 飞书/钉钉接收器

```
作为 SRE,把 deploy/monitoring/alertmanager/alertmanager.yml 里 http://CHANGE_ME:5001/alerts 占位替换为可实际工作的飞书/钉钉接收器:

1. 修改 alertmanager.yml 加 receivers 列表:
   - 'feishu-critical' 使用 webhook_configs 指向飞书 bot url(从 secret/env 注入)
   - 'feishu-warning' 同上不同 url
   - 抑制规则保留;route.routes 按 severity 分流到上述 receiver
2. 加 deploy/monitoring/alertmanager/templates/feishu.tmpl,用 {{ define "feishu.text" }}…{{ end }} 渲染中文消息(包含 alertname/severity/instance/summary/description + GeneratorURL)
3. 在 alertmanager.yml 加 templates: [ '/etc/alertmanager/templates/*.tmpl' ]
4. deploy/monitoring/docker-compose.monitoring.yml:挂 templates 目录
5. README 加一节"飞书 webhook 配置:进入飞书群 → 设置 → 群机器人 → 自定义机器人 → 复制 webhook → export FEISHU_CRITICAL_URL=…"

【硬性要求】可用 amtool check-config 跑通;alertmanager.yml 仍可被 yaml.safe_load 解析
```

---

## DS-V5-10:Wave 3-2 前端 10 个组件单测批量补

```
作为前端测试工程师,在 ~/ai-data-marketplace/frontend 给以下 10 个无测试的组件补 Vitest+RTL 单测,每个 ≥3 用例,覆盖关键路径:

1. SiteFooter.tsx — 渲染品牌、i18n、链接
2. Nav.tsx — 登录/未登录状态切换、通知 badge、active link
3. SchemaTable.tsx — 渲染空/非空 schema、字段类型 badge
4. Datasheet.tsx — 渲染只读/可编辑模式、保存动作触发 callback
5. MiniChart.tsx — 渲染空数据态、正常数据 svg
6. Compute.tsx 拆出来的子组件(FederatedComputePanel, PSIComputePanel)  — 主要分别 mock api 返回 [] 和 [list],断言渲染
7. DatasetQA.tsx — 提问表单提交、列表渲染
8. StripeCheckout.tsx — 无 pk 时显示 Alert、有 pk 时显示 Elements 占位(mock @stripe/react-stripe-js)
9. QualityReport.tsx — 渲染各种 quality result 形态(empty / PII / OK)
10. Legal.tsx — 双语切换、节渲染

【硬性要求】
- 每个 .test.tsx 用 Vitest + @testing-library/react,跟现有 __tests__/api.test.ts 风格一致
- Mock api 用 vi.mock('@/lib/api')
- 不许引入 U+201C/U+201D
- npm test 全过、tsc 全过、覆盖率 ≥80%(打开 vitest --coverage 自查)
```

---

## DS-V5 元说明

- 完成后落 `~/claudecode 信息站/deepseek-drop/v5/DS-V5-N/` 下
- 每个任务**绝不修改任何已合并 PR 已有代码外的文件**
- 每个任务 Claude 验收 sequence:`tsc → lint → build → npm test → e2e(适用时)→ promtool/k6/actionlint/kustomize build/amtool check-config(适用时)`
- 全部通过后 Claude 开 PR、合并、清理
