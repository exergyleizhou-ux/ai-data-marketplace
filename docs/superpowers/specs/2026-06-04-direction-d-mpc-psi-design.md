# Direction D · MPC / PSI(安全多方计算)— 落地设计

**日期**:2026-06-04  **基线**:`origin/main`(`mode` 字段已预留)
**状态**:设计文档(需多方节点 + 框架部署;本地先出设计 + 编排骨架)
**前序**:`docs/设计文档-P4-数据不出域(联邦学习与MPC·L3).md` §2.2;本文展开为可执行计划。

> 一句话:联邦 FedAvg 解决「各训各的再平均」;但有些场景无法这样切——**隐私求交(PSI)**(联合风控名单、广告归因)、**联合统计/联合线性模型**(数据按列切分而非按行)。这类用 **MPC(安全多方计算)**:多方在加密态下协同计算,谁也看不到他方输入。**平台只做编排 + 结果闸门,密码学交给成熟框架,绝不自研原语。**

---

## 1. 何时需要 MPC(与联邦的分界)

| 场景 | 数据切分 | 方案 |
|---|---|---|
| 各方有同schema的不同样本,训同一模型 | 按行(horizontal) | **联邦 FedAvg**(已建) |
| 各方持同一批人的不同特征,要联合建模 | 按列(vertical) | **MPC / 纵向联邦** |
| 各方有用户名单,要算交集(不暴露非交集) | — | **PSI(隐私求交)** |
| 各方要算联合统计(联合计数/求和/分位) | 任意 | **MPC** |

PSI 是最常见、最有商业价值的入口(数交所、联合风控、广告归因),建议**从 PSI 起步**。

## 2. 框架选型(不自研密码原语)

| 框架 | 能力 | 备注 |
|---|---|---|
| **蚂蚁 Secretflow / SPU** | PSI、纵向联邦、MPC 通用 | 开源,国产,数交所友好,**首选** |
| FATE | 纵向联邦、PSI | 微众,生态成熟 |
| MP-SPDZ | 通用 MPC 协议(SPDZ/BMR…) | 研究级,协议最全,运维重 |

> **决策**:起步用 **Secretflow**(PSI + SPU)——国产合规叙事好,对接数交所自然,可叠加国密。

## 3. 架构:平台做编排,框架做密码学

```
买方/发起方提交 MPC 作业(mode='mpc', 指定 PSI / 联合统计 / 纵向模型)
  │
  ▼ 平台编排器(复用 compute worker 模式)
  │   ① 校验各方 offer 允许 MPC + 买方权益
  │   ② 在各参与方节点拉起 Secretflow 计算参与者(平台不持有任何方明文)
  │   ③ 各方在 MPC 协议下协同计算(密文态)
  ▼ 结果(交集/统计/模型)经输出闸门(大小/DP/泄漏)→ 释放
作业元数据 + 结果句柄写入 compute_federated_jobs(mode='mpc')
```

**关键差异 vs 联邦**:联邦各方在**平台沙箱**内跑(数据上传到平台密文沙箱);MPC 各方**在自己的节点**跑(数据根本不进平台)——更强的「数据不出域」。平台从「执行者」退为「编排者 + 闸门」。

## 4. 数据模型(复用 + 扩展)

`compute_federated_jobs` 已有 `mode` 字段(`federated`|`mpc`,后者预留)。MPC 需补:
- `mpc_protocol`(psi | join_stats | vertical_lr)
- `party_endpoints`(各方计算节点地址,加密存储)
- 结果不再是「平台存的 model.json」,而是「各方可取的结果句柄」或「平台中继的密文结果」。

迁移:`000013_compute_mpc.up.sql`(新增列,nullable,向后兼容)。

## 5. 落地路径(先 PSI 骨架,Mock 框架)

### 阶段 0:设计评审(本文)

### 阶段 1:PSI 编排骨架 + Mock 框架(本地可验证)
- 新 `compute/mpc.go`:`MPCOrchestrator` 接口 `RunPSI(ctx, parties, input) (result, error)`;`mockMPC`(本地直接算交集,模拟协议形状)。
- `federated.go` / 新 handler:`mode='mpc'` + `mpc_protocol='psi'` 分支走编排器。
- API:`POST /compute/mpc-jobs`(对称于 federated-jobs)。
- **本地可验**:Mock PSI 交集正确 + 权益/闸门逻辑;单测 + 真 PG 集成。

### 阶段 2:接 Secretflow PSI(需多方节点)
- `secretflowMPC` 实现 `MPCOrchestrator`,委托 Secretflow SPU 跑 ECDH-PSI / KKRT。
- 各方节点部署 Secretflow worker;平台编排器协调。

### 阶段 3:联合统计 / 纵向线性模型
- 扩展 `mpc_protocol`;复用编排骨架。

### 阶段 4:国密 + 数交所对接
- SM2/SM3/SM4 替换协议中的签名/摘要/对称;结果存证对接数交所(`/datasets/:id/certificate` 模式扩展到 MPC 作业)。

## 6. 代码锚点

| 文件 | 角色 |
|---|---|
| 新 `compute/mpc.go` | `MPCOrchestrator` 接口 + `mockMPC` + `secretflowMPC` |
| `compute/aggregator.go` | `MPCAggregator`(接口注释已预留)如需 |
| `compute/federated.go` 或新 `compute/mpc_job.go` | `mode='mpc'` 编排 |
| `compute/model.go` | `FederatedJob.Mode` 已存在;补 MPC 字段 |
| 新 `compute/handler_mpc.go` + `router.go` | `POST/GET /compute/mpc-jobs` |
| 新迁移 `000013_compute_mpc.*` | mode/protocol/endpoints 列 |
| `internal/server/server.go` | `MPC_BACKEND=mock\|secretflow` 装配 |

## 7. 验证策略(诚实边界)

- **本地可验**:Mock PSI/统计的编排逻辑、权益校验、输出闸门、状态机、API 契约(httptest)。
- **门控(需多方 + 框架)**:真 Secretflow PSI 在 ≥2 个节点上的端到端;真实「各方数据不进平台」的网络证明。
- **诚实标注**:Mock 阶段 UI 标「MPC 编排(实验,Mock 协议)」;真框架打通前不宣称「密码学安全」已生效。

## 8. 商业切入建议
**PSI 联合风控名单** 是最快见价值的场景:两家机构想知道共同坏客户,但都不愿暴露自己的全量名单。先把这一条端到端做实(哪怕先 Secretflow 双节点 demo),比铺开所有 MPC 协议更有说服力。

## 9. 与方向 B/C 的关系
- B(真 TEE)是**硬件路线**的「数据不出域」;D(MPC)是**密码学路线**。两者可互补:TEE 跑 MPC 参与方可再降信任假设。
- C(安全聚合)是 MPC 的一个特例(只做安全求和);D 是通用 MPC。先 C 后 D 或并行皆可,但都**晚于** B 的优先级(B 只需把已有 L2 脚手架接真硬件,投入产出比最高)。
