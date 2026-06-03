# 交接文档 · C2D 隐私计算「可用不可见」—— 完整现状、文件位置与落地路径

**日期**：2026-06-03  **基线**：`origin/main @ e5bee18`  **读者**：接手的新会话 / 你本人

> 一句话：**「可用不可见」(Compute-to-Data) 已端到端建成并验证**——信任阶梯 L0→L3 全线打通(L1/L2 有可跑代码,L3 有设计),能演示、能收费、中英双语、沙箱真隔离、隐私真加噪,且**已对生产镜像仓库 + 真 Docker 端到端验证**。本会话共 **23 个 PR(#27–#49)全部 CI 全绿合并**。新会话只读这一份即可完全上手。

---

## 0. 新会话怎么开工(先读这节)

1. **代码在** `~/ai-data-marketplace`(git,远程 `origin` = GitHub `exergyleizhou-ux/ai-data-marketplace`,默认分支 `main`)。**当前那棵主工作树停在旧分支 `feat/h3-settlement-outbox` 且有历史未跟踪文件——不要在那棵树上干活。** 以 `origin/main` 为准。
2. **工作流(务必遵守)**：每个任务 `git worktree add ~/ai-data-marketplace-<name> -b feat/<name> origin/main` → 本地全验证 → push → `gh pr create --base main` → 看 CI 三 job(backend/frontend/sidecar)全绿 → `gh pr merge --squash --delete-branch` → `git worktree remove`。一棵树一件事。
3. **工具链(手装在 ~ 下,需手动 PATH)**：
   - Go 1.23 `~/.local/bin/go`(`export PATH="$HOME/.local/bin:$PATH"`)；gh 也在 `~/.local/bin`。
   - Node 20 `~/sdk/node/bin`。
   - Postgres(server-only,无 psql)`~/sdk/pg/bin`：临时库验证 `initdb -D $T -U postgres --auth=trust` → `pg_ctl -D $T -o "-p <port> -k <sock> -c listen_addresses=''" -w start` → `DATABASE_URL=postgres://postgres@/postgres?host=<sock>&port=<port>`。
   - Python 3.11 venv `~/sdk/sidecar-venv`(装了 PaperGuard、numpy、pandas;算法本地测试用它)。
   - **Docker Desktop 已装**:`open -a Docker` 启动(首启慢,轮询 `docker info`);凭据在 keychain(`docker-credential-desktop get`)。
4. **本地全验证铁律**：`gofmt -l .`、`go build ./...`、`go vet ./...`、`go test -race ./...`(连真 PG)、前端 `npm run typecheck && npm run lint && npm run build`。
5. **要继续 C2D 的下一步**？看 §6(落地剩余)与各设计文档。**真沙箱/真 TEE 要 Docker/TEE 环境**(见 §5 验证门控)。

---

## 1. 这是什么产品

**Verdant Oasis(绿洲)**——面向中国市场的 AI 训练数据交易平台。在原有「买即下载原始数据」之外,本会话新增了 **「可用不可见 / 沙箱计算(Compute-to-Data)」**:买方购买**计算权益**,在平台沙箱里跑**经审核的白名单算法**,只取走**计算结果(模型/统计)**,**不获得原始数据**。

## 2. 信任阶梯 L0→L3(现状)

| 级别 | 含义 | 状态 |
|---|---|---|
| **L0** 下载型 | 原有,交付原始数据 | 已有 |
| **L1** 数据沙箱 | 买方不可见、**平台仍可见**;真 Docker `--network=none` 隔离 | ✅ **代码 + 真 Docker 端到端验证** |
| **L2** 机密计算 | 连平台也不可见;TEE + 远程证明 | ✅ **第一刀:teeRunner + 证明 plumbing + Mock 证明器(真 TEE 门控)** |
| **L3** 数据不出域 | 多方,数据不集中;联邦/MPC | 📄 **设计文档(联邦=N 沙箱+FedAvg 可增量落地)** |

每级叠加**差分隐私**;输出过**闸门**(大小/DP/泄漏/可选人工复核)。诚实立场:不把 L1 吹成 L2。

---

## 3. 本会话完整 PR 清单(#27–#49,均已合并、CI 全绿)

**设计**
- **#27** C2D 立项设计文档；**#28** 设计抗压加固 v1.1(安全 §7.3/7.4、合规 §16 数据二十条/PIPL/等保、可靠性 §17、runner 协议 §18、对抗测试 §19、风险登记 §22)。

**P1 后端闭环**
- **#29** 迁移 `000010`(algorithms / dataset_compute_offers / compute_entitlements / compute_jobs / dp_budget_ledger)+ repo/service 内核(原子额度、乐观状态机、镜像 digest 钉死、幂等键、租约)。
- **#30** 执行引擎：`Runner` 接口 + 进程内 worker + 输出闸门 + 租约/崩溃恢复。
- **#31** HTTP API + server 装配 + 退款→吊销(接 H2)。
- **#33** **真 logreg 算法(纯 numpy,JSON 模型非 pickle)+ 硬化 dockerRunner**(argv 进 CI 单测)。
- **#34** 兑现 `review_output`(ops 人工复核)+ 拒绝退额度(§21)+ 列权益。
- **#35** **真实购买经 order+payment+分账结算**(迁移 `000011`:`orders.product_type` + 可空 `version_id`;compute 订单付款→授予权益(幂等)→交付→结算;dev 直发保留仅 dev)。
- **#36** Prometheus 指标(§17:作业终态/时长/租约回收)。
- **#38** **真差分隐私 `dp_stats`**(Laplace,顺序组合)+ **平台注入 ε**(worker 剥买方 `_epsilon`、注 offer 的 ε)。
- **#39** **§19 对抗性红队套件**(`algorithms/redteam`:attack.py + run-redteam.sh + CI 探针验真)。
- **#42** 修 §19 OOM 探针(`bytearray` 惰性零页 → 改逐页写入,真触发 OOM)。
- **#43** **docker 门控全栈 e2e**(`TestComputeDockerE2E`:真 PG + 真 dockerRunner 拉 digest 钉死镜像 → 买方下载真模型)。

**P2**
- **#40** **可切换容器运行时**(`DockerResources.Runtime` "" runc | runsc gVisor | kata;`COMPUTE_DOCKER_RUNTIME` env;argv 单测)。

**P3 / P4**
- **#48** **L2 机密计算第一刀**：`Attester`/`MockAttester`(HMAC 绑定 算法 digest+作业+输出,防篡改)+ `teeRunner`(包裹任意 base runner,无证明不放行)+ `repo.SetAttestation` + `service.GetAttestation` + `GET /compute/jobs/:id/attestation` + 前端「🔒 机密计算·已验证」徽章；`COMPUTE_RUNNER=tee` 装配。
- **#49** **L3 数据不出域设计文档**(联邦/MPC)。

**前端**
- **#32** 买家沙箱流程 + 卖家 offer 编辑器。
- **#41** i18n 基础设施(`lib/i18n.tsx` LocaleProvider/useT/LangToggle,hydration 安全)+ 导航/页脚/落地/C2D。
- **#44/#46/#47** i18n **全站补齐**(登录/注册/市场浏览 → 账户/订单/订单详情/数据集详情 → 卖家工作台/admin)。**i18n 已完成。**

**合规/部署**
- **#37** ToS 新增 §9「可用不可见/沙箱计算」双语条款(工程草拟,建议律师终审；main 上注册流程无强制重新同意门控)。
- **#45** **生产镜像仓库接入**：`algorithms/publish.sh` + 部署/注册手册 + 发布 digest。

---

## 4. 关键文件位置索引

- **后端 compute 模块** `backend/internal/modules/compute/`：
  - `model.go`(DTO/状态/错误)、`repo.go`(SQL，含 `SetAttestation`)、`service.go`(全部业务不变量 + `GetAttestation`/`PurchaseViaOrder`/`GrantForOrder`)、`worker.go`(管线 + `effectiveParams` 注入 ε + 存证明)、`handler.go`/`router.go`、`doc.go`。
  - `runner.go`(`Runner` 接口 + `MockRunner`)、`runner_docker.go`(`dockerRunner` + `dockerRunArgs` 硬化旗标 + `imageRef` digest 钉死)、`runner_tee.go`(**P3**:`Attester`/`MockAttester`/`teeRunner`)。
  - 测试：`*_test.go`(单测)、`*_integration_test.go`(真 PG，DATABASE_URL 门控)、`docker_e2e_integration_test.go`(docker+image env 门控)、`tee_*test.go`(P3)。
- **迁移** `backend/migrations/000010_compute.*`、`000011_compute_orders.*`。
- **装配** `backend/internal/server/server.go`(搜 `COMPUTE_RUNNER`：mock/docker/tee 选择；compute↔order/dataset/auth 适配器)。
- **指标** `backend/internal/platform/metrics/metrics.go`(`marketplace_compute_*`)。
- **算法** `algorithms/`：`logreg/`、`dp_stats/`、`redteam/`(§19 harness)、`publish.sh`(发布到生产仓库)。
- **前端** `frontend/components/Compute.tsx`(ComputeBuyer/ComputeOfferEditor/AttestationChip)、`frontend/lib/i18n.tsx`、`frontend/lib/api.ts`(C2D 类型 + 方法)。
- **设计/部署文档** `docs/`：
  - `设计文档-隐私计算与可用不可见(Compute-to-Data).md`(v1.1，主文档，§0 上手 + §15 P1 切片 + §16–§22 抗压)。
  - `设计文档-P3-机密计算与远程证明(L2).md`、`设计文档-P4-数据不出域(联邦学习与MPC·L3).md`。
  - `部署-C2D算法镜像与生产Runner.md`(**上线手册**)。

---

## 5. 已验证 vs 门控(诚实边界)

**已真实验证**：
- 全后端 `go test -race` + 真 PG 集成 + HTTP httptest 端到端；前端 tsc/lint/build；迁移 1→11 真 PG 通过。
- 算法本地真跑(logreg holdout 1.0；dp_stats Laplace 接近真值)；进 CI(sidecar job)。
- **§19 红队遏制在真 Docker = 5/5**(外联 TCP/DNS 阻断、只读 rootfs 拒写、4GiB OOM 被杀、超时被杀)。
- **全栈 e2e 在真 Docker + 生产 Docker Hub 镜像仓库**：`TestComputeDockerE2E` 拉 `docker.io/yes0505/c2d-algorithm@sha256:802cbbd…` → 真沙箱 → 买方下载 465B 真模型。

**环境门控(代码就绪,执行需相应环境)**：
- gVisor `runsc` / Kata：需节点装该运行时(`COMPUTE_DOCKER_RUNTIME=runsc`);argv 已单测。
- **真 TEE 执行 + 真硬件证明**:P3 现用 `MockAttester`(HMAC 替身);真 attester 需 TEE 硬件/云(TDX/SEV-SNP/SGX-DCAP)+ 证明服务。
- L3 联邦/MPC:仅设计;实现需多方节点 + Secretflow/MP-SPDZ。

复跑命令(任何有 docker 的机器)：
```bash
# §19 红队遏制
REDTEAM_IMAGE=vo-redteam:test ./algorithms/redteam/run-redteam.sh
# 全栈 e2e（生产镜像；需 docker login）
DATABASE_URL=<pg> COMPUTE_E2E_IMAGE=docker.io/yes0505/c2d-algorithm \
  COMPUTE_E2E_DIGEST=sha256:802cbbd32d3248f7fc4a9d813a05ad25748fcc3cf4135121ac0d392866537cc1 \
  go test -run TestComputeDockerE2E ./backend/internal/modules/compute/
```

---

## 6. 生产上线路径(你的 ops 动作,文件 `docs/部署-C2D算法镜像与生产Runner.md`)

**生产镜像仓库**：`docker.io/yes0505/c2d-algorithm`(**私有**仓库)。已发布并取 digest：
| 算法 | tag | output_kind | **digest(注册钉死)** |
|---|---|---|---|
| logreg | `logreg-1.0.0` | model | `sha256:802cbbd32d3248f7fc4a9d813a05ad25748fcc3cf4135121ac0d392866537cc1` |
| dp_stats | `dp-stats-1.0.0` | aggregate | `sha256:c7155be5dc0aa04127bf10c7ca6bd77c6af2c825d9ea458361915317a4757d45` |

升版本：`docker login && REGISTRY=docker.io/yes0505/c2d-algorithm ./algorithms/publish.sh`(打印新 digest)。

**上线 3 步**：
1. 平台进程设 `COMPUTE_RUNNER=docker`(或 `tee` 走 L2;可选 `COMPUTE_DOCKER_RUNTIME=runsc`)+ `STORAGE_DRIVER=s3`。Runner 节点能 `docker login` 拉该私有仓库。
2. ops API 注册两个算法(`image=docker.io/yes0505/c2d-algorithm`,`image_digest=<上表>`,`output_kind`)→ 审核 `status=approved, trusted=true`(L1 模型输出必须 trusted)。手册有 curl 示例。
3. 卖家 `PUT /datasets/:id/compute-offer`(`enabled=true`,`trust_level=L1`,聚合型设 `dp_epsilon`/`dp_epsilon_total`)。买家即可购买→提交→真沙箱执行→下载。

---

## 7. 落地剩余 / 后续目标(优先级排序,均带文件指引)

1. **真 TEE attester(P3 后续)** —— 在 `runner_tee.go` 把 `MockAttester` 换成真实实现:从 enclave 取 quote(`/dev/tdx_guest` 等),度量值=算法镜像 digest,接云/DCAP 证明服务校验;并做**基于证明的密钥释放(KBS)**让卖方数据密钥仅在证明通过后进 enclave。需 TEE 云(阿里云加密计算/Azure CVM/GCP CVM)。设计见 `docs/设计文档-P3-…(L2).md` §4。
2. **P4-a 联邦学习 MVP** —— 按 `docs/设计文档-P4-…(L3).md` §3/§4/§6:新迁移 `compute_federated_jobs` + `compute_jobs.federated_job_id`;`Aggregator` 接口 + `FedAvgAggregator`(先 Mock 打通编排,像 P1 用 MockRunner);联邦 worker 扇出 N 个现有沙箱子作业 → 聚合 → 联合输出闸门;offer 增 `allow_federated`;一个 `fed-logreg` 算法。**可在现有栈上增量落地,数据不集中。**
3. **真实多方 MPC/PSI(P4-c)** —— 接 Secretflow / MP-SPDZ,做隐私求交 + 联合统计。需多方节点。
4. **L2 商品页 UI** —— offer 编辑器支持选 `trust_level=L2`;详情页 L2 徽章 + 证明摘要展示(前端 `Compute.tsx` 已有 `AttestationChip`,扩展即可)。
5. **ToS 律师终审 + 重新同意机制** —— `#37` 的 §9 工程草拟需律师定稿;若要强制重新同意,需在 register 流程接 `lib/legal.ts` 版本门控(当前 main 未接)。
6. **DP 真加噪用于聚合的端到端** —— `dp_stats` 已真加噪 + 平台注入 ε;聚合型 offer 设 `dp_epsilon` 后端到端走通(真 docker 下已具备,补 UI 引导)。
7. **可选**:数交所对接(登记凭证扩展到计算作业存证)、国密(SM2/SM3/SM4)、GPU 训练(P3+ 评估,见主文档 §14.9)。

---

## 8. 坑与注意(本会话踩过)

- **zsh `:l` 修饰符**:`$VAR:tag` 即便加引号也会被 `:l` 吃掉(把 `:logreg` 变 `:latest`/错 tag)。**永远用 `${VAR}:tag` 花括号**。`publish.sh` 已规避。
- **无 docker 时**:默认 `MockRunner`,全链路可演示但非真沙箱;`COMPUTE_RUNNER=docker/tee` 才真跑。
- **Docker Hub 私有仓库**:未鉴权公共 API 对私有仓库返 404;`docker pull`(已登录)正常。生产仓库 `yes0505/c2d-algorithm` 是私有。
- **Docker Hub 偶发 503**:首发当天拉取侧 503(推送侧正常),非代码问题,重试/换时段。
- **macOS 无 `tac`/`timeout`/`brew`**:用 `tail -r`、Go 的 context 超时、手装工具链。
- **DP 不可复现**:`dp_stats` 故意不设种子(DP 要新鲜随机);logreg 则确定性(可复现争议复算)。
- **L1 + 模型输出 ⇒ 必须 trusted 白名单算法**(硬约束,见主文档 §2/§7.3):沙箱防不住"算法把数据编进模型",只能靠审核代码。

---

## 9. 记忆库指针

`~/.claude/projects/-Users-lei-claudecode----/memory/` 里 `MEMORY.md` 与 `marketplace-build-progress.md` 已更新到本会话现状(origin/main @ e5bee18,C2D 端到端 + 生产仓库验证)。新会话会自动看到。

---

**交接完毕。** 新会话开工建议:先读本文件 + 主设计文档 `docs/设计文档-隐私计算与可用不可见(Compute-to-Data).md`,然后按 §7 选一个方向(我的推荐:**P4-a 联邦 MVP** 最能在现有栈上增量出活,或 **真 TEE attester** 把 L2 做实)。
