# 交接 v5 — 安全审计 + C2D 全栈：现状与续作（自包含）

> 写于 2026-06-18。**这是新会话的单一自包含起点**，覆盖 **marketplace**（资金/compute 安全审计与修复）+ **lumen**（oasis 作者工具链）双线。全权由 Claude 决定方向；用户的标准指令是「继续 / 全部做完 / 高质量 / 全权交给你」，且开了 **ultracode**（实质任务用 Workflow 编排，token 成本不设限）。

---

## 0. 一句话现状

C2D 作者闭环已端到端跑通并出可验证凭证（`VO-A6D91A034A00`，哈希实证）；lumen `oasis publish` 一条命令可用；marketplace 的 compute + 资金路径经**两轮对抗审计、11 个 PR 全部合并 main**。两仓干净、全绿、live Oasis（:8080）健康、种子数据持久。
**唯一明确的待办续作：审计 deferred 的 #6/#7（offer 字段快照到 job，低真实影响，需一个 compute_jobs 迁移）→ 达 11/11；否则挑新方向。**

---

## 1. 仓库 / commit / GitHub（ground truth）

| 仓 | 本地 worktree | git store（durable） | 分支/HEAD | GitHub |
|---|---|---|---|---|
| marketplace | `/private/tmp/oasis-run` | `~/ai-data-marketplace/.git`（家目录，**即使 /tmp 清了 commit 仍在**） | `main @ 95860e9`（origin 同步） | exergyleizhou-ux/ai-data-marketplace |
| lumen | `~/lumen` | `~/lumen/.git` | `main @ a170b4d`（origin 同步） | exergyleizhou-ux/lumen |

- ⚠️ **/private/tmp 可能被 macOS 清理**。marketplace 的 worktree 文件在 /tmp，但 git 对象在家目录的 `~/ai-data-marketplace/.git`。若 worktree 丢失：去 `~/ai-data-marketplace`（注：那是更老的 `chore/security-hardening` 分支，**先 `git fetch origin && git worktree add /private/tmp/oasis-run main` 或在家目录 `git checkout main`**）。GitHub origin 永远是最终真相。
- gnb 算法源码已在 marketplace main 的 `algorithms/gnb/`（PR #166）。备份 `~/oasis-authoring-backup/gnb`，作者工作区 `/private/tmp/oasis-authoring/gnb`。

---

## 2. 构建 / 测试（关键 gotcha，不照做会卡）

- 每个新终端先：`export PATH="$HOME/.local/bin:$HOME/sdk/go/bin:$HOME/sdk/node/bin:$PATH"`
- **marketplace 后端需 Go 1.25.11 → 用 `GOTOOLCHAIN=auto GOFLAGS=-mod=mod`（不是 local！本地 go 是 1.23.4，严格 local 会编译失败）。** `cd /private/tmp/oasis-run/backend`。
- **lumen 用 `GOTOOLCHAIN=local GOFLAGS=-mod=mod`**；门：`make check` + `go test -race ./...`。
- **活体 DB 集成测试**（withdrawal/compute 的并发/迁移测试，无 DATABASE_URL 自动 skip）：
  `PW=$(docker exec ai-data-marketplace-next16-postgres-1 env | grep '^POSTGRES_PASSWORD=' | cut -d= -f2)`
  `DSN="postgres://app:${PW}@localhost:5432/ai_data_marketplace?sslmode=disable"`
  `DATABASE_URL="$DSN" GOTOOLCHAIN=auto GOFLAGS=-mod=mod go test ./internal/modules/<mod>/ -race`（当前 pw len=3）。
- macOS **无 `timeout` 命令**；**前台 `sleep` 被 harness 禁**（长等待用后台 `run_in_background` 或 Monitor）。
- 推送偶发 `LibreSSL SSL_ERROR_SYSCALL`（瞬时）→ 重试即可（我都用 `for i in 1 2 3; do git push && break; sleep 2; done`）。

---

## 3. 活体环境（重连 / 重启 runbook）

docker 现状（`docker ps`）：`ai-data-marketplace-next16-postgres-1`(:5432, healthy)、`-redis-1`(:6379)、持久 registry `oasis-registry`(127.0.0.1:5000, 卷 oasis-registry-data)、ember 容器（无关）。

重启后端（若挂了）：
```bash
docker compose -p ai-data-marketplace-next16 up -d postgres redis   # 复用带种子的旧卷，必须这个 project 名
cd /private/tmp/oasis-run/backend
APP_ENV=development COMPUTE_RUNNER=docker AUTO_MIGRATE=1 STORAGE_DIR=./data/storage \
  GOTOOLCHAIN=auto nohup go run ./cmd/api > /tmp/oasis-backend.log 2>&1 &
curl -fsS http://localhost:8080/healthz   # → {"status":"ok"}
```

**种子数据（持久在 pgdata 卷）**：
- 账号（pw `Oasis1234!`）：`demo-ops@oasis.test`(ops)、`demo-seller@oasis.test`(seller, KYC verified, 数据集 owner)、`demo-buyer@oasis.test`(buyer, KYC verified)。
- 数据集 `2e3896e2-bac3-4e34-b9c1-4290b954b981`（标题「Demo C2D 训练集」，磁盘 CSV `backend/data/storage/objects/demo/c2d-train.csv`，200 行 f1,f2,label，二分类 120/80）。
- 已注册算法（DB `algorithms` 表）：`K-Means 聚类`(0a0f27a2…) + **`高斯朴素贝叶斯 (GNB)`**(`89b3b79b-23e5-4152-9bea-1decb09ac415`，approved+trusted，image `127.0.0.1:5000/vo-gnb`，digest `sha256:e7ea247e9cad3baa68598ad79291b78157e15684b01222858a664752464af7a4`)。offer 白名单含两者。
- 重建脚本：`scripts/demo-kmeans-up.sh`（参数化，ALGO/NAME/DATASET_ID 可覆盖）+ `scripts/demo-gnb-up.sh`（薄封装）。

**端到端 API 流程**（curl，已验证多次）：`POST /api/v1/auth/login {account,password}` → `.data.tokens.access_token`；ops `POST /admin/compute/algorithms {name,runtime,image,image_digest,source_ref,output_kind,version}` → `/review {status:"approved",trusted:true}`；seller `GET/PUT /datasets/:id/compute-offer`（PUT 要回传全量字段，把算法 id 加进 allowed_algorithm_ids）；buyer `POST /datasets/:id/compute/purchase {quota}` → entitlement；buyer `POST /compute/jobs {dataset_id,entitlement_id,algorithm_id,params}` → 轮询 `GET /compute/jobs/:id`(queued→running→output_reviewing)；ops `POST /admin/compute/jobs/:id/release`；buyer `GET /compute/jobs/:id/certificate` + `/output`。

---

## 4. C2D 作者闭环（北极星，已端到端证完）

**Lumen 写算法 → 真 Oasis 隐私执行 → 可验证凭证。** 本程实证：
用 `lumen run`（DeepSeek agent）一回合写出纯 stdlib 高斯朴素贝叶斯 `train.py`（我先写 TDD 测试契约把关）→ `lumen oasis build` 构建+push → 真 Oasis 注册+信任 → 真 `--network none --read-only --cap-drop ALL` docker 沙箱执行 → buyer 取凭证 **`VO-A6D91A034A00`**（绑定 输出 SHA-256 + 钉死镜像 digest + 源数据集），**买方重算下载结果 SHA-256 与凭证逐字符匹配**。GNB 在 200 行数据上 accuracy 1.0。

**真容器契约（ground truth，别再搞错）**：算法读 `/data`（ro，首个 csv/tsv）+ `/params.json`（ro，可选 params），写 `/out/output.bin` = `zip(model.json, metrics.json)`（output_kind=model）。runner（`backend/internal/modules/compute/runner_docker.go`）只读 output.bin → 哈希 + Ed25519 签。⚠️ 历史误区：旧脚手架/旧 §4 说 params 在 `/out/input.json` 是**错的**（已在 lumen PR #3 修正为 `/params.json`）。

---

## 5. lumen oasis 工具链现状（已修齐、可用）

子命令：`init | validate | check | build | deploy | publish`（`cmd/lumen/oasis.go` + `internal/oasis/{oasis.go,check.go}`）。
- `init`：脚手架现生成**纯 Python stdlib** `train.py`（真契约 /data+/params.json→/out/output.bin）+ python:3.11-slim Dockerfile（USER 65534），不再是 Go。
- `validate`：manifest 校验，不再误warn Go 入口；`check`：真 docker 跑镜像验 output.bin（params 挂 /params.json）。
- `build`：`ImageTag` 识别 registry 端口冒号 / 已带 tag（不再生成 `repo:1:1`）；`oasis.toml` 的 image 要**不带 tag**。
- `deploy` / `publish`：`publish` = build→check→deploy 一条命令；deploy 的 conveyor belt 打真 admin 端点 `/api/v1/admin/compute/algorithms`，params_schema 作 JSON 对象 + source_ref；`MARKETPLACE_TRUST=1` 时 approve+trust。env：`MARKETPLACE_URL`（默认 localhost:8080）、`MARKETPLACE_TOKEN`（ops token）、`MARKETPLACE_TRUST`。
- **跑 lumen agent（dogfood）**：lumen 从 CWD 找 `lumen.toml`。`~/.config/lumen/lumen.toml` 里有一把**内联 DeepSeek key**（即下面「用户专属」要轮换的那把）。跑法：`cp ~/lumen/lumen.toml <cwd>/`（env-based，无内联密钥）+ `KEY=$(grep -E '^\s*api_key' ~/.config/lumen/lumen.toml | cut -d'"' -f2)` + `DEEPSEEK_API_KEY="$KEY" lumen run --mode bypass "..."`。用完删 cwd 的 lumen.toml，别让密钥进镜像/提交。
- **dogfood 诚实账（target A「连续不用救场」）**：gnb 算法、B1(ImageTag) = 2 张干净一回合；B3(Python 脚手架) 撞 max_steps 我救场（Go 模板 `%` 转义）；其余 oasis 修复我亲自做。结论：lumen 单文件良定义任务无监督可成，复杂多文件模板会卡。

---

## 6. 这一程合并的 11 个 PR（全在 main）

**lumen（2）**：
- #3 `fix/oasis-author-toolchain`：4 修复（ImageTag tag bug / validate 去 Go 误warn / init 改 Python 真契约 / deploy 打对 admin 端点+payload）+ 审查整改（ComputeSrcHash 碰撞框架、check params 路径对齐 /params.json、params_schema TOML 往返 strconv.Unquote）。merge 12d808e。
- #4 `feat/oasis-publish`：`oasis publish` 一条命令。merge a170b4d。

**marketplace（7）**：
- #166 gnb 算法 + demo-gnb-up.sh + 审查整改（类别基数上限防 L1 泄露 / 泄露测试限 dict 键数 / 高基数拒绝测试）。merge f857355。
- #167 compute 沙箱硬化：`--user=65534`（非 root）+ digest-pin 运行门（每个执行算法必须 sha256 digest，关镜像替换）+ image 前 `--` 分隔。merge a265cdd。
- #168 DP 预算并发原子：`SpendDP` 加 total + advisory-lock 条件插入，worker 超预算 reject；活体 DB 真并发测试**验红 13→5**。merge ee1fb82。
- #169 **withdrawal（CRITICAL）**：completed 提现不减余额→**无限重提掏空资金池** + TOCTOU 双提；原子 `CreateWithinBudget`（advisory lock + sum 含 completed）；活体并发测试**验红 11-12→5**。merge 9cbc0d3。
- #170 runner：/out OOM→有界 io.LimitReader（MaxOutputBytes）+ **修我 #167 引入的 `--user`→outDir 0700 回归**（Linux 上破坏所有 docker job，macOS Docker Desktop 掩盖；chmod 0777）+ 超时容器 `--name`+`docker kill` 清理。真 docker e2e 验证。merge 19d4869。
- #171 payment：争议中禁结算（settleOnce 把 order 状态守卫移出 `if !exists`，守护每次 ExecuteSplit）+ Stripe Refund 两调用加幂等键（reverse:/refund:）。merge 4c817c4。
- #172 compute cert/authz：cert 报**钉死的 job.AlgorithmVersion**(不是 live algo) + GetAttestation 补**联邦 sub-job 守卫**（漏的那个 per-job read）。merge 95860e9。

---

## 7. 两轮对抗审计 + 续作

**第一轮**（2 个 agent）审 compute 模块 → 修了 #167 的 3 项（隔离/digest/arg-injection）。
**第二轮**（ultracode Workflow，**16 agent 多透镜，compute + 资金路径，每条独立 skeptic 验证**）→ **11 条确认真问题（0 误报）**，9 条已修（PR #169-172），2 条 deferred。
- Workflow 脚本可复跑/迭代：`/Users/lei/.claude/projects/-Users-lei-claudecode----/5b9dec3f-aa2b-473d-b448-e7443fc955f1/workflows/scripts/audit-marketplace-security-wf_3da0e82a-632.js`（可加 dataset/quality/notification 等更多领域再跑）。

### ⏭ 明确续作：#6/#7（offer 字段快照到 job）→ 达 11/11
verifier 评**低真实影响**（卖家自有 offer，是配置时序一致性，非跨方利用），我有意 deferred。修法（**照 `DPEpsilon *float64` 的现成快照先例做**，已 model.go/repo.go 有迹可循）：
1. 迁移 `backend/migrations/000028_compute_job_offer_snapshot.{up,down}.sql`：`ALTER TABLE compute_jobs ADD COLUMN review_output boolean, ADD COLUMN max_output_bytes bigint;`（nullable，向后兼容；旧 job NULL → 回退 offer）。**注：migrations 在 `backend/migrations/`，最新 000027**。
2. `model.go` Job：加 `ReviewOutput *bool`、`MaxOutputBytes *int64`。
3. `service.go` SubmitJob（job 创建处，~590，旁边就是 `DPEpsilon: jobEps`）：`ReviewOutput: &offer.ReviewOutput, MaxOutputBytes: &offer.MaxOutputBytes`。
4. `repo.go` CreateJob（jobCols + INSERT）+ scanJob：加这 2 列（镜像 dp_epsilon 的 NULLIF/nullF 处理）。
5. `worker.go`：review 门（~273）`if job.ReviewOutput != nil { 用它 } else { 用 offer.ReviewOutput }`；size 门（~215）+ RunRequest（~203）同理用 job.MaxOutputBytes 回退 offer。
6. fake repo（compute service_test.go 的 fakeRepo CreateJob/GetJob）round-trip 这俩字段。
7. 测试：真 DB CreateJob 往返（repo_integration_test）+ fake SubmitJob 快照断言。
8. 验证：`GOTOOLCHAIN=auto go test ./internal/modules/compute/` + 真 docker e2e。一卡一 PR 合并。

---

## 8. 仍 deferred / 用户专属 / gated（别当 bug 反复挖）

- **#6/#7**（上面）—— 低影响一致性，明确修法，达 11/11 用。
- **/out 磁盘填充 DoS**：代码层已做有界读（防 OOM）；运行期磁盘填充需**宿主机配额/tmpfs-size**（部署级，非代码）。
- **lumen DeepSeek key 轮换**：`~/.config/lumen/lumen.toml` 内联的那把仍在 lumen git 历史里曝露过 → **用户在 DeepSeek 控制台轮换**（用户专属，我做不了）。
- **L2 真 TEE 硬件 / L3 Secretflow 多节点 / 合规法律**：卡外部基建，长线 gated。
- cert 的 `trusted` 字段仍报 live（ops 重审可变）—— 完整修需把 trusted 也快照到 job（并入 #6/#7 的迁移即可，低优先）。

---

## 9. 怎么开始下个会话

1. 记忆会自动加载 `c2d-author-loop-status.md`（NEXT-SESSION PICKUP）→ 指到本 v5 文档。读本文档。
2. 验环境：`curl localhost:8080/healthz`；`docker ps`；两仓 `git status`（应 main 干净）。挂了按 §3 重启。
3. 选方向（全权由你定）：
   - **A. 收尾 11/11**：做 §7 的 #6/#7 快照迁移（低风险、修法明确）。
   - **B. 继续审计硬化**：复跑/扩 §7 的 Workflow 到更多模块（dataset/quality/moderation/notification/auth），修新确认的真问题。
   - **C. 新产品面**：marketplace 已很成熟（search 有 ts_rank、发现有富过滤）；找具体高价值增量。
   - **D. lumen 主力**：继续 lumen 改 lumen 刷 target A 连击 / lumen 其它能力。
   - **E. gated 前沿**：真 TEE / Secretflow（需外部基建）。
4. 纪律不变：TDD（先写失败测试，钱/并发的修复务必活体 DB 验红验绿）、verify-before-claiming、一卡一 PR、信 go test 不信报告、诚实记录 deferred。ultracode 下实质任务优先用 Workflow 编排。

---

**这一程**：C2D 作者闭环跑通出证 → lumen oasis 工具链修齐 + 一条命令 → 两轮对抗审计挖出并修掉 9 条真问题（含 1 个资金池 critical、1 个我自己引入的 `--user` 回归），11 个 PR 全部合并。两仓干净全绿，live Oasis 健康，记忆铺好。下一程从最有价值处起手。
