# 威胁模型与保证 · Oasis 可验证证据层

# Threat Model & Guarantees · Oasis Verifiable-Evidence Layer

> 一句话 / In one line:
> **证书保证「溯源」——这套审计过的算法镜像 + 这份数据集 产生了这个确切的输出(可重算核验);它不保证正确性、不保证安全、不保证运营方看不到数据。**
> **The certificate guarantees *provenance* — that this audited algorithm image + this dataset produced this exact output (re-hash verifiable). It does NOT guarantee correctness, safety, or operator-invisibility.**

这份文档是 Oasis「可验证证据层」对自己的诚实交代:在什么对手面前、哪些保证成立、哪些失效。可信平台先得对自己诚实——不 over-claim 是这套东西最核心的差异点。

---

## 1. 系统模型 / System model

一次「数据可用不可见」(compute-to-data, C2D)计算的数据流:

1. 买方购买在某数据集上运行某个**已审计算法**的额度。
2. 运营方把数据集**只读**暂存到执行主机,在算法容器**断网之前**完成(`backend/internal/modules/compute/worker.go` `stageData`,design §18.3)。
3. 算法在加固沙箱里运行:`docker run --network=none --read-only --user 65534:65534 --cap-drop=ALL --security-opt=no-new-privileges`,资源受限,数据集挂在 `/data:ro`(`runner_docker.go` `dockerRunArgs`)。
4. **网络被切断**,所以**输出对象是唯一的外泄通道**。算法把单个输出写到 `/out/output.bin`。
5. 输出经过**输出闸**(下文对手 C)后存储,生成一张**结果证书**:`output_sha256` 绑定算法镜像 digest + 源数据集对象;任何人可在 `/verify/:cert` 重算核验。

可调的信任级别(design §2):**L1** 数据沙箱(运营方可见、买方不可见)、**L2** 机密计算 TEE(运营方不可见,需真硬件)、**L3** 数据不出域(联邦 / MPC)。当前默认部署是 **L1**。

---

## 2. 四类对手 / Four adversaries

对每一类:**能力** → **我们的防御** → **失效边界**(在哪成立、哪失效)。

### A. 好奇的买家 · Curious buyer
*想从合法购买的计算里多看到一些不该看的(原始行)。*

- **防御**:沙箱只回传**闸控后的输出**,从不回传原始行;算法契约是「只出聚合」(`algorithms/*` 全部 aggregates-only);中心化差分隐私(Laplace + 原子预算账本,`dp.go`)对支持的算法注入由平台控制、买方无法关闭的 epsilon;输出闸(对手 C)进一步限制输出的信息量。
- **失效边界**:DP 只覆盖显式启用它的算法;它**不能**修复一个本身就把太多信息塞进聚合的算法——那由输出闸兜底,而输出闸是个有界的启发式(见 C)。

### B. 恶意的买家 · Malicious buyer
*想通过提交精心构造的算法/参数来抽取原始数据。*

- **防御**:自定义算法需卖方在 offer 上**显式开启**(`allow_custom`);在 L1 上产出「模型」必须用**可信(已审计)算法**(`ErrModelNeedsTrust`);DP 预算账本是原子的——一连串各自通过提交检查的作业也无法超额(`SpendDP`);每次作业消耗额度;输出闸对所有作业生效。
- **失效边界**:如果卖方开了自定义算法又没要求 review,买方提交的算法就只受**输出闸**约束——见 C,这正是输出闸存在的理由。

### C. 恶意的算法作者 · Malicious algorithm author  ← A2 输出闸
*发布一个号称「聚合」、实则把原始行隐写进输出的算法。这是最强的对手:沙箱断网后,输出对象是唯一外泄口,而单纯的大小上限挡不住——64 MiB 足够塞进上千行。*

- **防御(A2,本次新增,`output_gate.go`)**:输出闸不再只看大小,而是限制输出的**信息量**,fail-closed(违规即扣留输出并退款,**绝不**改写输出):
  1. **结构形状**:输出只能是 (a) 单个 JSON 对象,或 (b) 一个**每个条目都是 `*.json` 且能解析**的 zip(真实算法契约 = `zip{model.json, metrics.json}`)。原始二进制、CSV、tar、往 zip 里夹一个 `.csv`——一律拒绝(`output_not_structured`)。这一条就堵死了「把数据集当 output.bin 直接 dump」。
  2. **信息量上限**(对解析后的 JSON 求和):字符串叶子总字节(聚合类默认 8 KiB——把数据集 base64 进一个字段是最容易的外泄手段,所以这是杠杆最高的检查)、数值叶子总数(聚合 1 万 / 模型 20 万——挡住把数据集摊平成数组)、键数 / 嵌套深度。
  3. **熵**(仅对长字符串):长度 ≥256 字节且香农熵 > 4.7 bit/byte 的字符串(像压缩/加密/base64 的 blob)被拒。阈值保守,SHA/UUID/ID 不会误伤;`MaxStringBytes` 才是主防线,熵是次级信号。
- **失效边界(诚实披露)**:这是**部分**缓解,不是完全消除。一个有决心的作者**仍能在边界内**外泄少量信息——几 KB 字符串、最多约 1 万个数(对聚合类)。要彻底堵死「算法本身是否诚实」需要算法级的形式化保证或可信审计,超出了输出闸的能力。输出闸把外泄从「无界」压到「有界且可审计」,但不归零。

### D. 被攻陷 / 恶意的运营方 · Compromised operator
*平台自身(或攻陷它的人)想看暂存的原始数据。*

- **防御(仅 L2/L3)**:**L2** 机密计算把计算放进 TEE,连运营方也看不见明文,并附远程证明报告供独立核验;**L3**(联邦 / PSI)让数据根本不出卖方域。
- **失效边界(已知的大缺口)**:在**当前默认的 L1** 部署下,运营方**能**看到暂存的数据——这一点**如实披露,不掩盖**。L2 是设计上的答案,其**验证那半已是真的**:`runner_tee_dcap.go` 用 Google 审计过的 `go-tdx-guest` 库对**真实 Intel TDX quote** 做离线验签 + PCK 证书链校验到 Intel 根 + 度量值白名单,已对 Intel 生产样本 quote 单测通过——**不是手搓的、也不是 mock**。仍受限的是**硬件那半**:需要真 TDX 硬件**产出** live quote(以及验时连 Intel PCS 取 collateral 查 TCB/吊销),以及机密性本身(只有真硬件的内存加密才成立)。L3 单进程已验证,真·多节点(Secretflow)受限。**在拿到真 TEE 硬件之前,买方与卖方对 L1 的信任假设里仍包含「信任运营方」。**

---

## 3. 证书保证什么 / 不保证什么 · What the certificate does and does not guarantee

**保证(GUARANTEES):**
- **溯源 / Provenance**:这个 `output_sha256` 由 **这个** digest 钉死的算法镜像、跑在 **这份** 源数据集上产生。任何人可重算下载的输出、比对证书哈希,在 `/verify/:cert` 独立核验。
- **完整性 / Integrity**:输出未被事后篡改;算法镜像未被换成别的(digest 钉死)。
- **可复现 / Reproducibility**:方法 + 数据 + 结果 是一条可重跑、防篡改的记录。

**不保证(DOES NOT guarantee):**
- **不保证正确性 / NOT correctness**:证书不说这个算法的**统计/科学结论是对的**。一个有 bug 或有偏的算法照样产出一张完全有效的证书。
- **不保证安全/良性 / NOT safety**:证书不说这个算法**没恶意**。它跑在沙箱里、过了输出闸,但「闸内」仍可能有有界的外泄(对手 C)。
- **不保证运营方看不到 / NOT operator-invisibility**:在 L1 下运营方能看到暂存数据。机密性是 L2(TEE)的承诺,受限于真硬件。

---

## 4. 已知缺口与如何关闭 · Known gaps and how they close

| 缺口 Gap | 现状 Status | 关闭条件 Closes when |
|---|---|---|
| 运营方可见数据(L1) | 如实披露;L2 **验证半已真**(DCAP quote 离线验签+链+白名单,`runner_tee_dcap.go`,对 Intel 生产样本单测过) | 真 TDX 硬件**产出** live quote + 验时 collateral → L2 完整成立 |
| 算法是否诚实(对手 C 残余) | 输出闸部分缓解 | 算法级形式化保证 / 可信第三方审计 |
| 真·多节点联邦 / PSI | 单进程已验证 | Secretflow/SPU 多节点环境 |
| 真实实验数据 | demo 数据为 model-grounded 合成(已标注) | 真实多方敏感数据合作方 |
| 同态加密 / 恶意安全 | 未实现 | 不 over-claim;按需研究 |

---

*这份文档与代码同源:对手 C 的输出闸是 `backend/internal/modules/compute/output_gate.go`(设计见 `docs/superpowers/specs/2026-06-20-c2d-output-gate-design.md`);沙箱参数是 `runner_docker.go`;DP 是 `dp.go`;信任级别是 `model.go`。诚实分级的实时状态见 `/c2d/honesty`。*
