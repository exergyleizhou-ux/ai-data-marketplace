# Direction C · 安全聚合(掩码求和)— 落地设计

**日期**:2026-06-04  **基线**:`origin/main`(联邦 FedAvg + 中心化 DP 已就绪)
**状态**:设计文档(研究级;建议先出设计,MVP 切片可本地验证骨架)
**前序**:`docs/设计文档-P4-数据不出域(联邦学习与MPC·L3).md`;本文是 P4-b 的「安全聚合」续集。

> 一句话:当前是**中心化 FedAvg**——平台聚合时**能看到每一方的模型参数**(虽非原始数据)。安全聚合(Secure Aggregation)让平台**只能看到参数之和**,看不到任何单方贡献。各方对本地参数加**成对掩码**(pairwise masks),全体求和时掩码两两抵消,聚合者只得到 Σ。**这是真密码学工程,不自研易错原语**;先固化设计,MVP 分阶段。

---

## 1. 现状诚实短板

FederatedComputePanel 底部已诚实标注:「中心化 FedAvg——平台聚合时可见各方模型参数」。`aggregateAndRelease`(`federated.go`)读每个子作业的 `fedparams-v1` 明文参数 → `FedAvgAggregator.Aggregate` 求加权均值。**平台在聚合那一刻看到了每一方的 `weights/intercept`**。模型参数可能反推训练数据(梯度泄漏攻击),所以这是真实隐私缺口,DP 只能缓解不能堵死「平台看见单方参数」。

## 2. 目标:平台只见 Σ,不见单方

经典 Bonawitz et al. (2017) SecAgg 思路,落到本平台的两方/N 方:

```
每方 k 持本地参数 x_k(向量)
任两方 (k,j) 通过密钥协商得共享种子 s_{kj}
方 k 发送:  y_k = x_k + Σ_{j<k} PRG(s_{kj}) − Σ_{j>k} PRG(s_{kj})   (mod q)
聚合者求和: Σ_k y_k = Σ_k x_k   ← 成对掩码两两抵消
聚合者全程只见 y_k(看不出 x_k),只能得到 Σ x_k
```

加权 FedAvg 需要 Σ(n_k·x_k) 与 Σ n_k:可让各方上传 `n_k·x_k` 的掩码值 + `n_k`(或把 n_k 也纳入安全求和)。

## 3. 难点(诚实列出,决定可行性)

1. **沙箱内密钥协商**:各子作业在**各自隔离的 `--network=none` 沙箱**里跑——它们**无法直接通信**做 DH 密钥交换。需要平台做「密文中继」:各 enclave/沙箱产出公钥 → 平台转发(平台见公钥但不见私钥)→ 各方导出成对种子。这要求沙箱能输出/输入一轮公钥材料(改 `fedparams` 协议为两轮)。
2. **掉队恢复**:若方 j 掉线,它与所有方的成对掩码无法抵消 → 求和被污染。SecAgg 用 Shamir 秘密分享让存活方协作恢复掉队方的掩码(或恢复其私种子)。**这是最复杂的部分**,与现有 `min_participants` 容错语义要重新对齐(现在是「掉队就少算一方」,SecAgg 下「掉队需主动恢复其掩码」)。
3. **与中心化 DP 的关系**:SecAgg 隐藏单方参数;DP 限制聚合结果的信息量。两者**正交可叠加**——SecAgg 后对 Σ 再加 DP 噪声(本地 DP 或聚合后中心 DP)。dp.go 的 `dpFedAvg` 需调整为对「安全求和结果」加噪。

## 4. 不自研原语 — 选型

- **优先复用成熟实现**:基于 `tf-encrypted`/`FATE SecureAggregation`/ 蚂蚁 Secretflow 的 SecAgg 模块,而非手搓 PRG + Shamir。
- 若自建,严格按 Bonawitz SecAgg 论文 + 用经过审计的 DH(X25519)+ AES-CTR 作 PRG + 标准 Shamir 库;**禁止自创掩码方案**。

## 5. 落地路径(分阶段,先骨架后密码)

### 阶段 0:设计评审(本文)
固化掩码方案、两轮协议、掉队恢复、与 `Aggregator` 接口/DP 的对接。

### 阶段 1:接口骨架 + 固定掩码(本地可验证)
- `aggregator.go` 新增 `MaskedSumAggregator`(实现 `Aggregator` 接口):入参为已掩码的 `y_k`,输出 Σ。
- `fed-logreg` 镜像 `train.py` 增加「输出掩码后参数」模式(先用**预共享固定掩码**双方打通,证明 Σ 正确抵消)。
- `federated.go` 增加掩码协调:`mode='secagg'`(新增,`compute_federated_jobs.mode` 已是自由字符串)。
- **本地可验**:两方固定掩码 → MaskedSum == 明文 FedAvg;单测。

### 阶段 2:成对掩码 + 一轮密钥协商
- 两轮 `fedparams` 协议:第一轮各沙箱出 X25519 公钥;平台中继;第二轮各沙箱用成对种子掩码参数。
- 平台只见公钥与掩码值,不见私钥/明文参数。

### 阶段 3:掉队恢复(Shamir)
- 存活方协作恢复掉队方掩码;与 `min_participants` 语义统一。
- **研究 + 重测**:这一阶段最易出错,需对照论文 + 充分对抗测试。

### 阶段 4:叠加 DP
- 对安全求和结果加噪;`dp.go` 适配。

## 6. 代码锚点

| 文件 | 角色 |
|---|---|
| `compute/aggregator.go` | 新增 `MaskedSumAggregator`;`Aggregator` 接口已支持多实现 |
| `algorithms/fed-logreg/train.py` | 产出掩码后参数 + 公钥材料(两轮) |
| `compute/federated.go` | 掩码协调(中继公钥、串两轮)、`mode='secagg'` 分支 |
| `compute/dp.go` | DP 噪声适配到安全求和结果 |
| `compute/model.go` | `FederatedJob.Mode` 已存在;新增 secagg 相关字段如需 |

## 7. 验证策略(诚实边界)

- **本地可验**:阶段 1 固定掩码的 Σ 抵消正确性、`MaskedSumAggregator` 单测、与明文 FedAvg 数值一致。
- **研究门控**:阶段 2-3 的密钥协商 + 掉队恢复需密码学评审 + 对抗测试,**不要在未评审前宣称「安全聚合已生效」**。
- **诚实标注**:UI 在阶段 3 完成前保持「中心化 FedAvg」标注;阶段性进展可标「安全求和(实验)」。

## 8. 为什么先出设计而非直接写
SecAgg 的掉队恢复 + 沙箱隔离下的密钥协商是已知易错点;一个错误的掩码方案会**静默泄漏**单方参数却看起来「能跑」。先评审协议、优先选成熟实现,是对「诚实立场」的负责。
