# 交接文档 v2 · 绿洲 C2D「可用不可见」+ Skills 双线磨合 — 完整现状、文件位置、四向落地路径

**日期**：2026-06-04  **基线**：`origin/main @ 9466313`
**读者**：接手的新会话 / 你本人。**读这一份即可完全上手**(项目 + skills 体系 + 下一步四个方向都在此)。

> 一句话：绿洲的差异化「可用不可见」(Compute-to-Data) **信任阶梯 L0→L3 已全线落地——可跑、可见、可配置、可演示、可用**(L1 真 Docker;L2 TEE 代码就绪+Mock 证明;L3 联邦真 Docker e2e + 端到端 UI)。本份交接要把 **四个方向**(本地打磨 / 真 TEE / 安全聚合 / MPC)以及 **"持续磨合 skills 与真实项目并行"** 的机制讲透。

---

## 0. 新会话怎么开工(先读这节)

1. **代码在** `~/ai-data-marketplace`(git;远程 `origin` = GitHub `exergyleizhou-ux/ai-data-marketplace`,默认分支 `main`)。
   ⚠️ **主工作树停在旧分支 `feat/h3-settlement-outbox`,且有别的并行线 worktree(`-h5` / `-docs`)——不要在那些树上干活,也别动它们。** 一律以 `origin/main` 为准,自己开新 worktree。
2. **工作流(铁律,一棵树一件事)**：
   ```bash
   git fetch origin
   git worktree add ~/ai-data-marketplace-<name> -b feat/<name> origin/main
   # ...改 + 本地全验证...
   git push -u origin feat/<name>
   gh pr create --base main --title "..." --body "..."
   gh pr checks <n> --watch     # CI 3 job: backend / frontend / sidecar
   gh pr merge <n> --squash --delete-branch
   git worktree remove ~/ai-data-marketplace-<name>
   ```
3. **工具链(手装在 ~ 下,需手动 PATH)**：
   `export PATH="$HOME/.local/bin:$HOME/sdk/node/bin:$HOME/sdk/pg/bin:$HOME/.bun/bin:$PATH"`
   - Go 1.23 `~/.local/bin/go`(`GOTOOLCHAIN=auto` 首次自动下 1.25);gh 也在 `~/.local/bin`。
   - Node 20 `~/sdk/node/bin`。Postgres(server-only,无 psql)`~/sdk/pg/bin`。
   - Python 3.11 venv `~/sdk/sidecar-venv`(numpy/pandas;算法本地测试用)。
   - **Docker Desktop 已装**:`open -a Docker` 启动,轮询 `docker info`。
4. **Go module 在 `backend/` 子目录** —— 所有 `go` 命令 `cd backend` 后再跑(否则 `go.mod not found`)。
5. **验证铁律**(从 `backend/` 跑;shell 每次会重置 cwd,务必在同一条命令里 `cd`):
   ```bash
   cd backend && gofmt -l . && go build ./... && go vet ./...
   # 真 DB 测试需 ephemeral PG(无 Docker / 无 psql 的 docker-less 配方):
   T=$(mktemp -d); SOCK=$(mktemp -d); PORT=55440
   initdb -D "$T" -U postgres --auth=trust >/dev/null
   pg_ctl -D "$T" -o "-p $PORT -k $SOCK -c listen_addresses=''" -w start >/dev/null
   DATABASE_URL="postgres://postgres@/postgres?host=$SOCK&port=$PORT&sslmode=disable" go test -race ./...
   pg_ctl -D "$T" stop -m fast >/dev/null
   cd ../frontend && npm ci --fetch-retries=5 && npm run typecheck && npm run lint && npm run build
   ```
   迁移是**嵌入式**,经 `db.RunMigrations(dsn)` / `AUTO_MIGRATE=true` 应用(**无 migrate CLI**;集成测试自动走这条路)。前端 `node_modules` 不跨 worktree 共享(package-lock 不同),每个 worktree `npm ci`;npm 偶发 ECONNRESET,`--fetch-retries=5` 重试。

---

## 1. 产品与大目标

**绿洲(Verdant Oasis)** = 面向中国市场的 AI 训练数据交易平台。
核心差异化 = **「可用不可见 / 沙箱计算(Compute-to-Data)」**:买方购买**计算权益**,在平台沙箱里跑**经审核的白名单算法**,只取走**结果(模型/统计)**,**不获得原始数据**。合规叙事对齐数据二十条/PIPL(三权分置:持有权各自保留、加工使用权受控行使)。
核心模块(H1–H7 + data-trust 已完成):auth / dataset / delivery / order / payment / quality / search / **compute(C2D)**。

## 2. 信任阶梯 L0→L3(现状)

| 级别 | 含义 | 后端 | UI | 验证 |
|---|---|---|---|---|
| **L0** 下载型 | 交付原始数据 | ✅ | ✅ | — |
| **L1** 数据沙箱 | 买方不可见、**平台仍可见**;`--network=none` 真隔离 | ✅ | ✅ 卖家配/买家用 | **真 Docker**(§19 红队 5/5 + e2e 拉生产镜像) |
| **L2** 机密计算 | 连平台也不可见;TEE + 远程证明 | ✅ `runner_tee.go`(Attester/teeRunner) | ✅ 卖家可选(诚实标注需 TEE 部署) | **Mock 证明器**(真证明门控 TEE 云) |
| **L3** 数据不出域 | 多方,数据不集中;联邦/MPC | ✅ FedAvg+容错+中心化DP+真镜像 | ✅ 买家发起/下载、卖家开启 | **真 Docker 联邦 e2e** |

每级叠加**差分隐私**;输出过**闸门**(大小/DP/泄漏/可选人工复核)。**诚实立场**:不把 L1 吹成 L2,不把中心化 FedAvg 吹成安全聚合。

## 3. 本会话增量(PR #51–#56,均已合并、CI 全绿)

L3 联邦从一纸设计 → 全线可用:
- **#51** 联邦 MVP:真 FedAvg(`aggregator.go`)+ 编排(`federated.go`)+ 子作业沙箱隔离 + 迁移 `000012`。
- **#52** `min_participants` 容错:全部 settle 后,released≥min 则聚合幸存者(掉队退款),否则全失败全退款;无重复退款。
- **#53** 中心化 DP:`dp.go` `dpFedAvg`(clip→加权均值→Laplace(Δ/ε))+ crypto/rand 噪声;接入聚合;**CI 抓出并根治一个真启动竞态**(enqueue-then-mark-ready:子作业在转 fanout 前跑完→回调 no-op→卡死;修复=fanout 后显式 kick 一次 tryAdvance)。
- **#54** **真 fed-logreg 沙箱训练镜像**(`algorithms/fed-logreg/`,输出 `fedparams-v1`)+ docker 门控联邦 e2e(`federated_docker_e2e_integration_test.go`)—— 真验证:2 个 `--network=none` 沙箱→真 FedAvg→联合模型。
- **#55** 联邦 UX 端到端可用:`GET /users/me/compute/federated-jobs` + 前端 `FederatedComputePanel`(/account)+ offer `allow_federated` 开关 + 全 i18n。
- **#56** 卖家信任级别选择(L1/L2)+ 诚实部署提示。

> 更早:**#27–#49** 整个 C2D L1/L2 栈 + i18n + 生产镜像;**#50** 上一版交接文档(`docs/交接-C2D隐私计算-完整现状与落地路径.md`,仍有效,讲 L1/L2 细节)。

## 4. 代码地图(精确位置)

**后端 compute 模块** `backend/internal/modules/compute/`:
- `model.go` — DTO/状态机/错误哨兵(Job、Offer、Entitlement、Algorithm、**FederatedJob** + Fed* 状态 + RuntimeFedLogreg)。
- `service.go` — 业务不变量(SubmitJob、ConfigureOffer、PurchaseViaOrder、CancelJob…)。
- `federated.go` — **联邦编排**:`SubmitFederatedJob`(扇出+校验+min_participants)、`tryAdvanceFederated`(事件驱动,容错决策)、`aggregateAndRelease`、`ListFederatedJobs`、accessors。
- `aggregator.go` — `Aggregator` 接口 + **真 `FedAvgAggregator`**(加权均值)+ `parsePartial`(fedparams-v1)。`MPCAggregator` 仅留接口注释(P4-c)。
- `dp.go` — `dpFedAvg`(中心化 DP)+ `laplaceNoise`。
- `runner.go`(MockRunner)/`runner_docker.go`(dockerRunner,硬化旗标 + digest 钉死)/`runner_tee.go`(**L2**:Attester/MockAttester/teeRunner)。
- `repo.go` + `repo_federated.go` — pgRepo(乐观状态机 `UPDATE…WHERE status=from`);联邦 CRUD + `ListFederatedJobsByBuyer`。
- `worker.go` — 作业管线(租约/重试/闸门);联邦子作业走相同管线但输出走聚合、不放行买家。
- `handler.go` / `handler_federated.go` / `router.go` — HTTP。
- 测试:`*_test.go`(单测)、`*_integration_test.go`(真 PG,DATABASE_URL 门控)、`*docker_e2e*`(docker+image env 门控)、`tee_*`(L2)。
**迁移** `backend/migrations/000010_compute.* / 000011_compute_orders.* / 000012_compute_federated.*`。
**装配** `backend/internal/server/server.go`(搜 `COMPUTE_RUNNER`:mock/docker/tee;聚合器默认 FedAvg)。
**算法** `algorithms/`:`logreg/`、`dp_stats/`、**`fed-logreg/`**、`redteam/`(§19)、`publish.sh`(发布生产镜像并打印 digest)。
**前端** `frontend/`:`components/Compute.tsx`(ComputeBuyer / ComputeOfferEditor / **FederatedComputePanel** / AttestationChip)、`lib/api.ts`(C2D 类型+方法)、`lib/i18n.tsx`(`t(zh,en)` 内联,无 key 文件)。`app/account/page.tsx` 挂联邦面板;`app/datasets/[id]/page.tsx` 挂 ComputeBuyer;`app/sell/page.tsx` 挂 OfferEditor。
**spec/plan** 历史:`docs/superpowers/specs/` 与 `docs/superpowers/plans/`(每刀的设计与计划)。

## 5. 坑(每个都踩过一次,务必记住)

- **JSONB `NOT NULL DEFAULT '{}'` 列**:INSERT 传 `nil` 仍违约 → `toJSONB(nil)` 返回 nil,要改传 `[]byte("{}")`。DEFAULT 只在省略列时生效。
- **`uuid[]` 参数**:`$N::uuid[]` 显式转型;回读 `dataset_ids::text[]` 进 `[]string`。
- **乐观状态机**:`UPDATE…WHERE status=$from RETURNING`;0 行 ⇒ `ErrBadTransition`。并发安全,用于幂等编排去重。
- **enqueue-then-mark-ready 竞态**:异步编排里,先入队子任务、最后才置"就绪"状态——子任务可能在置位前跑完,其回调因状态守卫 no-op,导致永久卡住。**对策**:置位后显式再触发一次推进(见 `SubmitFederatedJob` 末尾)。
- DTO 时间戳是 `string`(`::text` 扫描),不是 `time.Time`——沿用既有风格。
- 改完 `gofmt -w`(结构体对齐会变)。
- macOS 无 `tac`/`timeout`/`migrate`/`brew`;用 `tail -r`、Go context 超时、嵌入式迁移。
- DP 故意不可复现(新鲜随机);logreg/fed-logreg 确定性(争议复算)。
- L1+模型输出 ⇒ 必须 trusted 白名单算法(硬约束:沙箱防不住"算法把数据编进模型")。

## 6. Skills 体系 与「双线磨合」机制(本会话的元产出)

**这台机器有一套精筛的 skill 体系(306→64),分三层,详见 `~/.claude/SKILLS-INDEX.md`、路由见 `~/.claude/CLAUDE.md`:**
- **方法论层(hook 自动注入)**:`superpowers:*`(brainstorm→writing-plans→executing-plans→TDD→systematic-debugging→verification-before-completion…)。**rigid,严格遵守**。
- **记忆层(hook 自动)**:`claude-mem`(向量记忆 worker + chroma;sleep 自愈已加)。
- **模式层(按需)**:golang/postgres/redis/docker/react/api-design/security-review/**verification-loop** 等 18 个;贴合 Go+Next+PG 栈。
- **工具层(slash)**:gstack 44 个(`/review` `/qa` `/ship` `/cso` `/spec` `/browse` `/make-pdf` 等)。
- 注:gbrain 评估后**已诚实移除**(与 claude-mem 重复 + 检索 bug)。

**双线磨合机制(用户要求长期保持)——每做一刀真实项目,就回灌一次 skill 质量:**
1. 做项目切片时**严格走方法论**(brainstorm→spec(`docs/superpowers/specs/`)→plan(`docs/superpowers/plans/`)→TDD→真验证→PR→CI→合并)。
2. 切片中**踩到的坑/学到的命令** → 立即沉淀:
   - 项目专属 → 写进项目根 `CLAUDE.md`(本仓库,会随会话自动加载)的 "坑" 一节。
   - 通用可复用 → 写进全局 skill(如 `~/.claude/skills/verification-loop/SKILL.md` 本会话加了 Phase-0 找 go.mod + ephemeral-PG 配方 + JSONB 检查)。
3. 下一刀因此更快更稳;形成"项目用磨过的 skills 做、skills 用项目实战磨"的正循环。
4. **本会话已磨**:verification-loop(实战命令)、项目 CLAUDE.md(从无到有 + 5 条坑)。**继续保持这个习惯。**

> 跨会话记忆在 `~/.claude/projects/-Users-lei-claudecode----/memory/`:`MEMORY.md`(索引)、`marketplace-build-progress.md`(逐 PR 进度,已更新到 #56)、`skills-setup.md`(skill 体系)、`local-toolchain.md`、`project-location.md`、`worktree-layout.md`。新会话会自动看到。

## 7. 四个方向 — 详细落地路径

### 方向 A:本地小打磨(纯本地可验证,适合开胃 / 保持动量)
1. **联邦 Prometheus 指标**:仿现有 `backend/internal/platform/metrics/metrics.go` 的 `marketplace_compute_*`,加 `marketplace_federated_jobs_total{status}`、聚合时长、参与方数。在 `aggregateAndRelease`/`tryAdvanceFederated` 终态处打点。验证:单测指标注册 + `go test`。
2. **联邦面板显示数据集名**(而非 id 前 8 位):前端 `FederatedComputePanel` 现用 `e.dataset_id.slice(0,8)`;加一个按 id 批量取 dataset 名的调用(看 `api.ts` 是否已有 `getDataset`;没有则后端加轻量批量名查询)。验证:前端 build。
3. **联邦作业分页/详情展开**:`listMyFederatedJobs` 已支持 limit/offset;前端加"加载更多" + 点开调 `getFederatedJob` 展示各子作业状态(后端已返回 `{federated_job, sub_jobs}`)。
→ 一个 PR 可全包。**TDD + 前端 build,无需特殊环境。**

### 方向 B:真 TEE attester(L2 做实)—— **需 TEE 云环境**
- 目标:`runner_tee.go` 把 `MockAttester`(HMAC 替身)换成真实现:
  1. 从 enclave 取 quote(TDX `/dev/tdx_guest` / SEV-SNP / SGX-DCAP);度量值=算法镜像 digest。
  2. 接 DCAP / 云证明服务校验 quote。
  3. **基于证明的密钥释放(KBS)**:卖方数据密钥仅在证明通过后进 enclave(数据"连平台也不可见"的关键)。
- 文件:`runner_tee.go`(Attester 接口已在,换实现)、装配 `server.go`(`COMPUTE_RUNNER=tee`)。设计见 `docs/设计文档-P3-机密计算与远程证明(L2).md` §4。
- **门控**:需 TEE 云(阿里云加密计算 / Azure CVM / GCP Confidential VM)。本地只能写代码 + 单测 Attester 接口契约,**真验证必须在 TEE 机器**。
- 落地节奏(和既有一致):先用 Mock 打通接口/装配/单测,再在 TEE 机器接真 quote+KBS;**不做半成品门面**。
- ⚠️ 提醒:这类需外部环境/凭据的步骤,准备好 TEE 云访问后再上;别让 UI 承诺超过部署能力(#56 的 L2 提示已诚实标注"需 TEE 部署")。

### 方向 C:安全聚合(掩码求和)—— **研究级,可先出设计**
- 目标(P4-b 续):聚合器**看不到单方参数**。各子作业在沙箱内对本地参数加**成对掩码**,Σ 掩码=0,平台求和后掩码抵消,只得到聚合值。
- 难点(诚实):需"沙箱内生成成对密钥/掩码"的协调机制 + 掉队方处理(掉队会破坏掩码抵消,需恢复协议)。这是真密码学工程,**不要手搓易错版**。
- 建议路径:
  1. 先出**设计文档** `docs/设计文档-P4b-安全聚合.md`:掩码方案(成对 PRG 种子 via DH/预共享)、与现有 `Aggregator` 接口如何对接、掉队恢复、与中心化 DP 的关系。
  2. 评估直接用成熟实现(如基于 `tf-encrypted`/`SecAgg` 思路)而非自研原语。
  3. MVP 切片:在 `Aggregator` 加 `MaskedSumAggregator`,fed-logreg 镜像加掩码输出;先双方固定掩码打通,再上成对掩码 + 掉队恢复。
- 文件锚点:`aggregator.go`(加实现)、`algorithms/fed-logreg/train.py`(产出掩码后的参数)、`federated.go`(协调)。

### 方向 D:MPC / PSI —— **需多方节点 + 框架**
- 目标:无法靠"各训各的再平均"的场景——隐私求交(PSI,联合风控名单/广告归因)、联合统计、联合线性模型。
- 框架(**不自研密码原语**):蚂蚁 **Secretflow**(开源)/ FATE / MP-SPDZ。平台做**编排 + 结果闸门**,密码学交给框架。
- 落地:`Aggregator`/新 runner 类型委托 Secretflow 协调;`compute_federated_jobs.mode='mpc'`(字段已预留);面向数交所可叠加国密(SM2/SM3/SM4)。设计见 `docs/设计文档-P4-数据不出域(联邦学习与MPC·L3).md` §2.2。
- **门控**:需多方节点 + 框架部署。本地先出设计 + 编排骨架(Mock 框架),真跑需基础设施。

### (附)产品其他面(若想换战场)
H 系列核心(auth/dataset/order/payment/delivery/quality/search)已建;可深化:搜索(语义/向量)、质量校验流水线、结算对账、数交所登记凭证扩展到计算作业存证。各模块同构 `handler→service→repo`,照 §4/§5 模式即可。

## 8. 已验证 vs 门控(诚实边界)

**已真实验证**:全后端 `go test -race`+真 PG;前端 tsc/lint/build;迁移 1→12;算法本地真跑+CI(sidecar);§19 红队真 Docker 5/5;**L1 全栈 e2e 拉生产镜像**;**L3 联邦 e2e 真 2 沙箱→真 FedAvg**。
**门控(代码就绪,执行需环境)**:gVisor/Kata(装 runtime,argv 已单测);真 TEE 执行+硬件证明(TEE 云);安全聚合(研究);MPC/PSI(多方+框架)。

## 9. 生产上线(ops 动作,详见 `docs/部署-C2D算法镜像与生产Runner.md`)

生产镜像仓库 `docker.io/yes0505/c2d-algorithm`(私有)。已发布 logreg / dp_stats(digest 见部署手册);**fed-logreg 已进 `publish.sh`,上线前 `docker login && REGISTRY=... ./algorithms/publish.sh` 取 digest 并注册**。
3 步:① 平台设 `COMPUTE_RUNNER=docker`(或 `tee`)+ `STORAGE_DRIVER=s3`,runner 节点能 `docker login` 拉私有仓库;② ops API 注册算法(image+digest 钉死,fed-logreg 设 `runtime=fed-logreg, output_kind=model, trusted=true`)→ 审核 approved+trusted;③ 卖家 `PUT /datasets/:id/compute-offer`(enabled,trust_level,聚合型设 dp_epsilon,联邦设 allow_federated)。

## 10. 新会话开场白建议

> 「读 `docs/交接-v2-C2D与Skills双线-现状与四向路径.md`。按 §7 推进四个方向:先做 **A 本地打磨**(一个 PR);再出 **C 安全聚合 / B 真 TEE / D MPC 的设计文档**;有 TEE 云后实现 B。全程保持 §6 的双线磨合(每刀回灌 skill)。一棵 worktree 一件事,CI 三 job 绿再合并。」

**交接完毕。** 项目 + skills 双线、四向路径、文件位置、验证配方、坑、上线步骤——全部在此,新会话可无缝接手。
