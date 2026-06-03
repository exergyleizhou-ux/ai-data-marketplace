# P4 · 数据不出域(联邦学习与 MPC) — L3 设计

**目标**:信任阶梯最高级 **L3 —— 数据不出域(data stays home)**:**多方**数据在**各自的域内**参与计算,原始数据**从不集中**到平台或任何单一方,买方/联合方只得到**联合计算的结果**(联合模型 / 联合统计 / 求交)。对应主设计文档 §2(信任阶梯)、§9(L3 路线)、§11(Phase 4)。

> 本文给出**可落地路径**与数据模型/编排草案;**真实多方运行**(联邦聚合服务、MPC 框架、多方节点)是后续实现,需多方基础设施,故本期为**设计 + 在现有沙箱之上的最小切片定义**,不做半成品实现。

## 1. L3 与前级的区别

| | L1 沙箱 | L2 机密计算(P3) | **L3 数据不出域(本文)** |
|---|---|---|---|
| 数据在哪计算 | 平台沙箱(单数据集) | 平台 TEE 内(单数据集) | **各方域内**(多方,数据不集中) |
| 谁看原始数据 | 平台可见 | 平台不可见 | **任何单一方都拿不到他方原始数据** |
| 典型场景 | 单数据集训练/统计 | 同左 + 抗平台 | **多卖方联合建模 / 跨库联合统计 / 隐私求交(PSI)** |

L3 的本质是**多方**:它不是"把数据搬进一个更强的盒子",而是"**数据各待各家,只交换中间量/秘密分享**"。

## 2. 两条技术路线

### 2.1 联邦学习(FedAvg)—— 可在现有沙箱之上增量落地
**思路**:一个"联邦作业"= 在 **N 个数据集各自的沙箱**里各跑一次训练(复用 P1/P2/P3 的 runner,每个卖方数据**只在自己的沙箱内**被读),只把**模型参数/梯度**交给平台的**安全聚合器**做 **FedAvg**,产出联合模型。原始数据从不集中。

```
联邦作业(算法=fed-logreg, 数据集 [D1..DN])
  ├─ 子作业1: 在 D1 沙箱训练 → 局部参数 w1（+可选 DP-SGD 噪声）
  ├─ 子作业2: 在 D2 沙箱训练 → w2
  └─ 子作业N: ...                → wN
  ▼ 安全聚合器: w* = Σ(nk·wk)/Σnk   （仅聚合参数，不见原始数据）
  ▼ 输出闸门（大小/DP/泄漏）→ 释放联合模型
```
- **复用**:N 个子作业 = N 个现有单数据集作业(沙箱隔离、输出闸门、记账、状态机全复用)。新增的只是**编排(扇出 N 个子作业)+ 聚合(FedAvg)+ 联合输出闸门**。
- **隐私加固**:子作业参数可叠加 **DP-SGD**(训练时加噪);聚合可用**安全聚合**(掩码求和,聚合器看不到单方参数)。
- **这是 L3 最务实的第一刀**:不需要全新的多方密码栈,就能实现"原始数据不出各自沙箱、只联合模型"。

### 2.2 MPC / 安全多方计算 —— 用成熟框架,不自研
适合**联合统计 / 隐私求交(PSI)/ 联合风控建模**等无法靠"各训各的再平均"解决的场景:
- **PSI(隐私求交)**:两方求用户交集而不暴露各自全集(广告归因、联合风控名单)。
- **联合统计 / 线性模型**:秘密分享 + 联合计算。
- **框架**(不自研密码原语):**蚂蚁 Secretflow(开源)/ FATE / MP-SPDZ**。平台做**编排 + 结果闸门**,密码学交给框架。
- 面向数交所可叠加**国密(SM2/SM3/SM4)**(§16.5)。

## 3. 数据模型草案(后续迁移)

```sql
-- 联邦/多方作业:一个作业引用 N 个数据集,扇出 N 个子作业 + 一个聚合步骤
CREATE TABLE compute_federated_jobs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    buyer_id      UUID NOT NULL REFERENCES users(id),
    algorithm_id  UUID REFERENCES algorithms(id),     -- runtime 如 fed-logreg / psi / secretflow
    dataset_ids   UUID[] NOT NULL,                     -- 参与的多方数据集
    mode          TEXT NOT NULL,                       -- 'federated' | 'mpc'
    status        TEXT NOT NULL DEFAULT 'created',     -- created→fanout→aggregating→released/failed/rejected
    output_key    TEXT, output_bytes BIGINT,
    dp_epsilon    DOUBLE PRECISION,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- 子作业复用 compute_jobs,新增 federated_job_id 外键关联
ALTER TABLE compute_jobs ADD COLUMN federated_job_id UUID REFERENCES compute_federated_jobs(id);
```

## 4. 编排(复用现有 worker)
- `Aggregator` 接口:`Aggregate(partials [][]byte) (joint []byte, err error)`;实现 `FedAvgAggregator`(参数加权平均)、`MPCAggregator`(委托 Secretflow/MP-SPDZ 协调)。
- 联邦 worker:扇出 N 个子作业 → 全部 released 后取各方参数 → `Aggregator.Aggregate` → 联合输出过闸门 → 释放。**每个子作业的卖方需对联邦用途授权**(offer 增 `allow_federated`)。
- 失败/部分参与策略:可设最小参与方数;掉队方按超时剔除(联邦容错)。

## 5. 治理与合规
- **授权**:多方联合需各卖方对"联合用途 + 参与方"知情同意(ToS 增条款;延续 §16 PIPL/数据二十条三权分置——L3 是"加工使用权联合行使、持有权各自保留"的最纯形态)。
- **诚实标注**:商品页 L3 徽章 + "数据不出域";联邦仍可能经模型泄漏 → 叠加 DP;PSI/MPC 给形式化保证。延续"信号非结论 / 不夸大"。

## 6. 分步落地建议
1. **P4-a 联邦最小切片**(可在现有栈上做):`compute_federated_jobs` + 扇出 N 个现有沙箱子作业 + `FedAvgAggregator` + 联合输出闸门 + 一个 `fed-logreg` 算法。**Mock 聚合器先打通编排与状态机**(像 P1 用 MockRunner),再接真训练镜像。
2. **P4-b 安全聚合**:掩码求和,聚合器不见单方参数。
3. **P4-c MPC/PSI**:接 Secretflow / MP-SPDZ,做隐私求交 + 联合统计。
4. **P4-d 跨域节点**:真正把子作业下推到卖方域内节点运行(数据物理不出域),平台只收聚合量。

> 与 P1–P3 一致:每一步先用 Mock 把**编排/状态机/闸门/计费**打通并单测,再接真实多方密码栈;真实多方运行门控(需多方节点 + 框架),不做半成品。
