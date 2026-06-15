# 隐私计算中心 (Compute Hub) — 可发现性设计

**日期:** 2026-06-15
**问题:** 后端的隐私计算全家桶(常规沙箱计算、联邦学习 L3、PSI、远程证明、存证)已全部
就绪并前后端连通,但买家侧能力**全部埋在 `Compute.tsx` 组件里,只能从「某个数据集详情页拉到
最底部」进入**。没有任何顶部导航入口、没有"我的全部计算活动"的聚合页。结果是:做了很多,
用户发现不了 → "感觉没体现链接"。

## 现状盘点 (后端能力 ↔ 前端入口)

绝大多数能力已有家:数据市场/详情/搜索、卖数据、订单、收益、账户(实名/2FA/导出/注销)、
通知、运营后台(10 tab)、存证验真 `/verify`。**收藏 watchlist 也已完整闭环**(详情页收藏按钮
→ 账户页收藏列表),无需新增。

唯一缺口:买家侧隐私计算无聚合入口。`listMyComputeJobs` / `listMyComputeEntitlements` /
`FederatedComputePanel` / `PSIComputePanel` 全部只在数据集详情页底部出现。

## 方案

顶部导航新增 **「隐私计算 / Compute」**(登录可见)→ 独立页 `/compute`,Protected,标签式:

1. **算力权益** — `MyEntitlementsPanel`(新):列出 active entitlements + 数据集名 + 剩余次数 +
   跳转数据集。
2. **计算作业** — `MyComputeJobsPanel`(新):跨所有数据集的常规作业列表,状态/下载输出/存证/
   L2 远程证明(复用同模块私有 `JobCertificate` / `AttestationChip`)。
3. **联邦学习** — `FederatedComputePanel`(复用现有,无 props)。
4. **隐私求交 PSI** — `PSIComputePanel`(复用现有,无 props)。

数据集详情页那块 `ComputeBuyer` 照旧(那是"针对某个数据集开始一个任务"的起点)。

## 边界与复用

- 新面板加在 `Compute.tsx` 同模块内,以复用私有 helper,避免导出泄漏内部细节。
- 页面遵循既有约定:`Protected` 包裹、admin 风格标签切换、`useT` 双语、`ui.tsx` 组件。
- 无后端改动、无 API 改动 —— 纯前端信息架构(把已就绪能力提成一等入口)。

## 验收 (真实可见可跑)

dev server(:3001)HMR 生效后:登录买家 → 顶部出现「隐私计算」→ 进 `/compute` → 四个标签
均能渲染、拉到真实数据(权益/作业/联邦/PSI),preview 截图为证。
