# 规划:Oasis 可验证溯源(Compute-to-Data)完整路线图

> 约束:**没有更多外部数据**。一切只靠现有代码 + 5 个仓库(Oasis / Lumen / ember /
> bos-platform / PaperGuard)的现有资产 + 从 bos/PaperGuard 的模型与参考区间生成的
> **演示数据(必须如实标注 `model-grounded`,绝不冒充真实实验数据)**。

## 0. 第一性原理

- **主线 = 可验证溯源 / 可信证据**,不是「隐私计算」这个标签(隐私计算在 5 个 repo 里
  只有 Oasis C2D 一处是真的;详见 `portfolio-strategy` 记忆)。
- 需要「真实数据方 / 真 TEE 硬件 / 多节点 Secretflow / 持牌支付」的,一律 **gated**,
  不做、不假装。
- 定位:一个**真实可跑、可验证、零造假**的「可验证科研计算」平台 demo + 一组真证书 +
  一个诚实叙事。对 portfolio / 科研诚信社区 / 基金 / 技术品牌是强资产;它**不是**有付费
  客户的生产隐私平台(那缺的三样——真实数据方、生产级隐私保证、合规支付——没有外部
  输入就突破不了)。

## 1. 现状(已建成的底座)

5 个旗舰 C2D 算法,各一张真证书(`--network=none` 沙箱、只出聚合、可重算哈希验真):

| 算法 | 来源 | PR | 证书 |
|---|---|---|---|
| paperguard 完整性筛查 | PaperGuard | #212 | VO-795A4D76D4FE |
| causal-mediation (Pearl NDE/NIE) | bos | #213 | VO-1DFD9CBEFFAB |
| causal-sensitivity (Cinelli-Hazlett) | bos | #214 | VO-639F1C2A367C |
| bioprocess-kinetics (Logistic/Gompertz) | bos | #216 | VO-6B9E6ACC8A5F |
| causal-refutation (placebo/RCC/subset) | bos | #218 | VO-D9342583F9B4 |

\+ 买家凭证卡 (#215)、公开 `/c2d` 叙事页 (#217)、导航/首页可达 (#219)。

## 2. 路线图(全部「只靠现有资产、零新数据」)

### Phase A — 算法广度

把 bos/PaperGuard 的引擎批量 C2D 化。**质量判断**:只做「吃一个数据集、回传聚合、保护
原始行」的(dataset-analyzer);纯计算器型(给参数算结果、没有要保护的数据)要么用
「一个数据集 = N 次运行」的框架重塑成 dataset-analyzer,要么不做。

| 算法 | 形态 | 产出(聚合) |
|---|---|---|
| `causal-estimate` | 因果五件套地基 | ATE(OLS)+ DML 正交估计、SE/CI |
| `mass-balance` | N 次转化运行 | 物料闭合率 + 残差 ε + 在容差内占比 |
| `process-economics` (TEA) | N 次运行的成本/产出 | 平均 NPV/回收期/单位成本(跨运行聚合) |
| `lca-footprint` | N 次运行的物料/能耗 | 平均 GWP / 单位产品碳足迹 |
| `paperguard-full` | 表格数据 | 扩到全部离线检测器 |

### Phase B — 可验证研究档案 (dossier)

对**同一个数据集**跑一串算法 → 一**组**证书,组合成一份可验证研究档案(原 pitch 的
release dossier 的诚实落地版)。一个前端档案页:列出 + 逐一实时验真这组证书,展示
`数据集 → N 个算法 → N 张证书` 的完整证据链。

### Phase C — 联邦 / 多方(单进程已验证那条)

用 Oasis 现成的 FedAvg/DP-FedAvg/DDH-PSI(单进程已测过),跨 N 个合成「参与方」数据集
跑联邦作业 → 联邦证书。故事:「数据不出各方域,只有联合模型出来」。
**真·多节点 = gated**(需 Secretflow/多机),如实标注。

### Phase D — 产品 & 叙事面

- 公开可分享的证书页(`/verify/VO-…` permalink + 凭证卡);
- 一份诚实的「什么已验证 / 什么仍 gated」说明页(不吹 = 差异化);
- 「复现此结果」指南(拉 digest 钉死镜像、在你自己数据上跑、比对哈希);
- bos 作为旗舰垂直:「可验证的生物转化/固废研究」。

## 3. Gated(诚实标注,不做)

| 想要 | 缺什么 |
|---|---|
| 真实实验数据驱动 | 无(bos 也未提交)→ 一律 model-grounded 合成 + 标注 |
| L2 真 TEE | TEE 云/硬件 → 保留 mock+scaffold |
| 真·多节点联邦/MPC | Secretflow 多机 → 只做单进程演示 |
| 同态加密 / 恶意安全 | 密码学研究 → 不做,不 over-claim |
| 真实分账/付费 | 持牌支付 + 真实数据方 → 支付保持 sandbox |

## 4. 诚实的天花板

- ✅ 能成为:真实可跑、可验证、零造假的「可验证隐私感知科研计算」demo + 一组真证书 +
  诚实叙事。强 portfolio / 科研 / 基金 / 品牌资产。
- ❌ 不能(当前约束下)成为:有付费客户的生产隐私平台 —— 缺真实数据方、生产级隐私
  保证、合规支付。没有外部输入(数据/硬件/钱/合规)突破不了。

## 5. 执行顺序

1. Phase A `causal-estimate` + `mass-balance`(凑齐因果地基 + 过程侧)。
2. Phase B dossier(capstone:把零散算法变成一个有冲击力的整体证据链)。
3. Phase D 叙事面(让它对外可展示)。
4. Phase C 联邦单进程 demo。
5. Phase A 其余按需。
