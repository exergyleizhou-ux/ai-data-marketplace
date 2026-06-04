# B/C/D 方向 — 详细落地设计索引

**日期**:2026-06-04
**背景**:v2 交接文档(`docs/交接-v2-C2D与Skills双线-现状与四向路径.md`)§7 给出四个方向的高层路径。方向 A(联邦打磨)已实现并合并(PR #58)。本索引指向 B/C/D 三个方向的**详细可执行设计文档**。

## 优先级与依赖

```
A 本地打磨 ───────────────── ✅ 已实现合并(PR #58)
                              指标 + 数据集名 + 分页/详情

B 真 TEE(L2 做实)────────── 设计就绪,门控 TEE 云  ★ 投入产出比最高
   └ 只需把已有 L2 脚手架(runner_tee.go 的 Attester 接口)接真硬件 + KBS

C 安全聚合(掩码求和)─────── 设计就绪,研究级,可先做骨架
   └ 是 MPC 的特例(只做安全求和);最易出错处=掉队恢复

D MPC / PSI ──────────────── 设计就绪,门控多方节点  ★ 商业价值最高(PSI 风控)
   └ 平台退为编排者;密码学交给 Secretflow,不自研
```

## 三份详细设计

| 方向 | 文档 | 门控 | 本地可做的部分 |
|---|---|---|---|
| **B 真 TEE** | `2026-06-04-direction-b-real-tee-design.md` | TEE 云(TDX/SEV) | `KeyBroker` 接口 + mockKBS 单测、teeRunner 串 KBS |
| **C 安全聚合** | `2026-06-04-direction-c-secure-aggregation-design.md` | 研究/密码学评审 | `MaskedSumAggregator` 骨架 + 固定掩码 Σ 抵消单测 |
| **D MPC/PSI** | `2026-06-04-direction-d-mpc-psi-design.md` | 多方节点 + 框架 | `MPCOrchestrator` 接口 + mockMPC PSI 编排单测 |

## 三条共同原则(均已写入各文档)

1. **不自研密码原语**:TEE 用 DCAP/云证明;SecAgg/MPC 用 Secretflow/FATE/MP-SPDZ 等成熟实现。
2. **先 Mock 通管线,再上真环境**:与既有节奏(dockerRunner、fed-logreg)一致;不做半成品门面。
3. **诚实标注**:真环境打通前,UI 不得宣称对应安全属性「已生效」。

## 建议下一刀
**B 的阶段 1**(`KeyBroker` 接口 + mockKBS + teeRunner 串联 KBS,本地可验证单测)——纯本地、低风险、把 L2 从「Mock 证明」推进到「证明 + 密钥释放骨架」,为接真 TDX 硬件铺好接口。
