# 交接 v3 · 绿洲 C2D 四向 + 数据信任 — 完整现状与下一步

**日期**：2026-06-04  **基线**：`origin/main @ 9e20b2c`
**读者**：接手的新会话 / 你本人。读这一份 + `docs/交接-v2-C2D与Skills双线-现状与四向路径.md`(讲 skills 双线 + 工作流铁律)即可完全上手。

> 一句话:v2 之后本会话连发 **13 个 PR(#58–#70,全 CI 绿)**,把 C2D 四个方向推到**本地干净 runway 的尽头**,并开了「数据信任/存证」新产品面。本份讲清每个 PR 干了什么、四向现状(已验证 vs 门控)、以及下一步只能往哪走。

---

## 0. 工作流铁律(同 v2,务必遵守)
- 代码在 `~/ai-data-marketplace`(远程 `exergyleizhou-ux/ai-data-marketplace`,默认分支 `main`)。**Go module 在 `backend/`**。
- **主工作树停在旧分支,3 棵预存 worktree(h3/docs/h5)是早期并行线——别碰**。一律 `git worktree add ~/ai-data-marketplace-<name> -b feat/<name> origin/main`。
- 验证:`cd backend && gofmt -l . && go build ./... && go vet ./...`;真 DB 测试用 docker-less ephemeral PG(`initdb`/`pg_ctl` + `DATABASE_URL` → `go test -race ./...`);前端 `npm ci && npx tsc --noEmit && npx next lint && npm run build`。
- PATH:`export PATH="$HOME/.local/bin:$HOME/sdk/node/bin:$HOME/sdk/pg/bin:$PATH"`。
- 一棵 worktree 一件事 → PR → `gh pr checks <n> --watch`(3 job:backend/frontend/sidecar)→ squash 合并 → 删 worktree。
- **坑**(项目 CLAUDE.md 有全量):JSONB-NOT-NULL 传 `[]byte("{}")`;`uuid[]` 显式 `::uuid[]`;乐观状态机 `UPDATE…WHERE status=$from`;enqueue-then-mark-ready 竞态;DTO 时间戳是 string;改完 `gofmt -w`;**Edit 大块 .tsx 会引入 curly quote(U+201C/U+201D)当分隔符→ tsc 报 TS1127,只在可见文案里用 curly,分隔符用直引号**。

## 1. 本会话 13 个 PR(#58–#70)

**方向 A — 联邦打磨**
- **#58** 联邦 Prometheus 指标 + 数据集名(替 UUID)+ 分页/子作业详情展开。

**B/C/D 设计 + 元产出**
- **#59** B/C/D 详细设计文档(`docs/superpowers/specs/2026-06-04-direction-{b,c,d}-*` + 索引)。
- **#60** smart-quote-in-.tsx 坑写进项目 `CLAUDE.md`(双线磨合)。

**方向 B — 真 TEE / L2(分两半:本地半已落地,硬件半门控)**
- **#61** L2 KBS 骨架:`KeyBroker` + `mockKBS` + teeRunner 证明门控密钥释放(fail-closed),接入 server tee 路径。
- **#67** 真 KBS HTTP 客户端 `remoteKBS`(`KBS_URL` 选中,fail-closed,httptest)+ `docs/部署-L2-TEE节点与KBS.md`。
- **#68** TDX 硬件 attester 脚手架 `tdxAttester`(读 `/dev/tdx_guest`,`TEE_ATTESTER=tdx`)。**只 off-hardware fail-closed 路径经单测;ioctl 成功路径只在 TDX 节点验证;Verify 不做进程内 DCAP(返回 false,真伪由 KBS/DCAP)**。

**方向 C — 安全聚合(building block)**
- **#62** `MaskedSumAggregator`(成对掩码抵消 → 与明文 FedAvg 数值相同,平台只见 Σ)。**仅聚合数学;沙箱内掩码密钥协商 + 掉队恢复 = 阶段2 研究**。

**方向 D — MPC/PSI(端到端可用)**
- **#63** `MPCOrchestrator` + `mockMPC` PSI(正确求交语义,building block)。
- **#64** PSI 作业端到端:**PSI 作业 = 带 psi-extract 算法的联邦作业**,复用扇出/权益/闸门;真 PG 集成测试。
- **#65** 前端 `PSIComputePanel`(/account):选数据集→求交→内联看交集。
- **#66** 专属 `allow_psi` 卖家授权(迁移 000013;PSI≠联邦,不同隐私暴露,不再复用 allow_federated)。

**数据信任 / 存证(换战场产品面)**
- **#69** 计算结果存证:`VO-<hex>` 绑定输出 SHA-256 + 已审核算法 digest + 数据集;`GET /compute/jobs/:id/certificate` + 前端 `<JobCertificate>` 徽章。
- **#70** 联合结果存证:扩展到联邦/PSI(`GET /compute/federated-jobs/:id/certificate`)。**存证现覆盖所有计算结果类型**。

## 2. 四向现状(诚实边界)

| 方向 | 本地已落地(可验证) | 门控(需外部资源/研究) |
|---|---|---|
| **A 联邦打磨** | ✅ 全部(指标/名字/分页) | — |
| **B 真 TEE L2** | ✅ KBS 客户端 + 证明门控 + TDX off-hardware fail-closed | ⛔ 真 TDX/SEV quote 生成 + DCAP + 真 KBS 部署 → **需 TEE 云节点**(指南 `docs/部署-L2-TEE节点与KBS.md`) |
| **C 安全聚合** | ✅ MaskedSumAggregator 求和数学 | ⛔ 沙箱内掩码密钥协商 + 掉队恢复 → **密码学研究 spike**(然后像 PSI 那样接成 secagg 联邦 mode) |
| **D MPC/PSI** | ✅ 端到端可用(backend+UI+consent+存证),mockMPC | ⛔ 真 Secretflow/SPU 密码学求交 → **需 ≥2 个 Secretflow 节点** |

信任阶梯 L0→L3 全部可跑/可见/可配置/可演示;L1 真 Docker,L2 代码就绪(硬件门控),L3 联邦真 Docker e2e + PSI 端到端。每级叠加 DP;输出过闸门;计算结果有溯源存证。

## 3. 代码地图(本会话新增/改动锚点)
`backend/internal/modules/compute/`:
- `kbs.go`(KeyBroker+mockKBS)、`kbs_remote.go`(remoteKBS)、`runner_tee.go`(teeRunner KBS 门控)、`runner_tee_tdx.go`(TDX 脚手架)。
- `aggregator.go`(FedAvg + **MaskedSumAggregator**)、`mpc.go`(MPCOrchestrator+mockMPC+parsePSISet)。
- `federated.go`(SubmitFederatedJob 按算法 runtime 定 mode;aggregateAndRelease 分流 PSI;GetFederatedCertificate)、`runner.go`(MockRunner 产 fedparams/psi-set)。
- `certificate.go`(BuildJobCertificate + BuildFederatedCertificate)、`service.go`(GetJobCertificate + orchestrator 字段)、`handler*.go`/`router.go`(新端点)。
- 迁移 `000013_compute_allow_psi.*`。装配 `internal/server/server.go`(`COMPUTE_RUNNER=tee` 时 `TEE_ATTESTER`/`KBS_URL`)。
前端 `frontend/components/Compute.tsx`(PSIComputePanel + JobCertificate + allow_psi 开关)、`lib/api.ts`。

## 4. 下一步(全部需用户决定/外部资源)
**本地干净的纯软件 runway 已走完。** 剩余三类,均需投入:
1. **B 硬件半**:在 TDX/SEV 节点上验证 `tdxAttester` ioctl + 接 QGS/DCAP + 部署真 KBS(`KBS_URL`)。按 `docs/部署-L2-TEE节点与KBS.md`。**需 TEE 云**。
2. **D 真 PSI**:`mockMPC` → Secretflow/SPU。**需多方节点**。
3. **C 阶段2**:沙箱内掩码密钥协商(研究),再像 PSI 那样接成 `mode='secagg'` 的联邦聚合。**密码学 spike**。

**或换战场(纯本地)**:卖家质检退回-remediation 引导、ops 计算作业可见性、搜索(语义/向量,注意 pgvector 可能门控)、结算对账。各模块同构 `handler→service→repo`,照现有模式。

## 5. 给新会话的开场白
> 「读 `docs/交接-v3-C2D四向与数据信任-完整现状.md` + v2(skills 双线)。四向本地 runway 已走完:B/D/C 的下一步分别需 TEE 云 / Secretflow 节点 / 密码学研究——若用户开通环境再上;否则换战场做纯本地产品面(质检 remediation / ops 可见性 / 搜索 / 结算对账)。一棵 worktree 一件事,TDD,CI 三 job 绿再合并,每刀回灌 skill(双线磨合)。」
