# Direction B · 真实 TEE 远程证明(L2 做实)— 落地设计

**日期**:2026-06-04  **基线**:`origin/main`(L2 脚手架已就绪)
**状态**:设计文档(代码就绪,真验证**门控 TEE 云**)
**前序**:`docs/设计文档-P3-机密计算与远程证明(L2).md`(§4 列出后续);本文把 §4 展开为可执行计划。

> 一句话:L2 当前用 `MockAttester`(HMAC 替身)在密码学层演示「绑定 + 防篡改 + 可独立校验」。本设计把 `MockAttester` 替换为基于真实 TEE 硬件 quote 的 `Attester`,并加上**基于证明的密钥释放(KBS)**——让「连平台也不可见」从承诺变成可验证事实。**真验证必须在 TEE 机器上**;本地只能写代码 + 单测接口契约。

---

## 1. 现状与目标差距

| 维度 | 现状(Mock) | 目标(真 TEE) |
|---|---|---|
| 度量值来源 | 调用方传入算法 digest | enclave 度量寄存器(MRENCLAVE / RTMR / launch measurement) |
| quote 签名者 | 固定 HMAC 密钥 | TEE 硬件证明密钥(Intel/AMD 背书链) |
| 校验方 | 重算 HMAC | DCAP / 云证明服务校验背书链 + 度量策略 |
| 数据密钥 | 无(数据明文可达平台) | 仅在证明通过后释放进 enclave(KBS) |
| 平台能否看明文 | **能** | **不能**(内存加密 + 密钥不出 enclave) |

**目标**:`runner_tee.go` 的 `Attester` 接口不变(已设计正确),换三处实现 + 加一个密钥释放环节。

## 2. 接口契约(已存在,保持不变)

`backend/internal/modules/compute/runner_tee.go`:
```go
type Attester interface {
    Attest(ctx, AttestInput) ([]byte, error)   // 产生 quote 报告
    Verify(ctx, report []byte) (Attestation, error) // 校验并回填 verified
}
```
`AttestInput{Measurement, JobID, OutputSHA}` 三元绑定(WHAT ran / 新鲜性 / 输出完整性)已是正确抽象。真实现只是把「HMAC 签/验」换成「硬件 quote 取/验」。**这是本设计能低风险落地的关键:抽象边界已对。**

## 3. 真实现:三种 TEE 后端(择一起步)

### 3.1 Intel TDX(推荐起步:云支持最广)
- **取 quote**:enclave 内读 `/dev/tdx_guest`(`TDX_CMD_GET_REPORT` ioctl)→ TD report → 经 Quote Generation Service(QGS)签成 quote。度量值 = RTMR[0..3] + MRTD(算法镜像 + 启动度量)。
- **校验**:用 Intel DCAP QVL(Quote Verification Library)或云证明服务(阿里云/Azure Attestation)校验 PCK 证书链 + TCB 状态 + 度量策略(MRTD 必须等于已注册算法镜像的度量)。
- **Go 接入**:cgo 包 DCAP QVL,或 HTTP 调云证明服务(返回 JWT,验签 + 校 claims)。

### 3.2 AMD SEV-SNP(Azure / GCP Confidential VM)
- **取 quote**:`/dev/sev-guest`(`SNP_GET_REPORT`)→ attestation report(含 launch measurement)。
- **校验**:对 AMD AReK/VCEK 证书链验签 + 度量比对。

### 3.3 Intel SGX + Gramine(细粒度 enclave,无需机密虚机)
- 算法以 Gramine 打包成 enclave;`MRENCLAVE` = 度量值;DCAP 远程证明。
- 适合单算法强隔离;运维比机密虚机重。

> **决策**:起步选 **TDX**(度量=镜像 digest 映射最直接,云支持广,与现有「digest 钉死」模型对齐)。SEV-SNP 作为 Azure/GCP 落地备选。

## 4. 基于证明的密钥释放(KBS)— L2 的真正关键

没有 KBS,「平台不可见」只是空话(平台仍能读明文数据)。流程:

```
卖方用自己的 KEK 加密数据集 ──▶ 平台只持密文 + 密钥句柄(看不到明文)
买方提交 L2 作业
  │
  ▼ 平台在 TEE 节点起 enclave(运行 digest 钉死的算法镜像)
  │   ① enclave 取 quote(度量=镜像 digest)
  │   ② enclave 把 quote 发给 KBS(Key Broker Service)
  │   ③ KBS 校验 quote(真 TEE + 度量∈策略白名单)→ 通过则释放数据密钥进 enclave
  │   ④ 数据仅在 enclave 内用释放的密钥解密、计算
  ▼ 输出经输出闸门(同 L1)→ 释放;quote 写入 compute_jobs.attestation
```

- **KBS 选型**:Confidential Containers KBS(开源,attestation-based)/ 云 KMS 的机密计算密钥释放策略 / 自建(enclave 公钥包进 quote,KBS 用该公钥加密数据密钥回传)。
- **密钥绑定**:释放策略 = `measurement ∈ {已注册算法镜像度量}`,杜绝「换个恶意算法骗取密钥」。

## 5. 装配与代码改动(精确锚点)

| 文件 | 改动 |
|---|---|
| `compute/runner_tee.go` | 新增 `tdxAttester`(实现 `Attester`),保留 `MockAttester` 作 dev/test fallback。`teeRunner.Run` 增加「证明前向 KBS 请求数据密钥」步骤(或在 staging 阶段)。 |
| `compute/runner_docker.go` | TEE 模式下 base runner 用 `runtime=kata`(机密容器)或在 TEE 节点跑;`COMPUTE_DOCKER_RUNTIME=kata` 已可切。 |
| `internal/server/server.go` | `COMPUTE_RUNNER=tee` 已能选 teeRunner;新增 `TEE_ATTESTER=tdx\|sev\|mock`(默认 mock)选 Attester 实现;`KBS_URL` 等环境。 |
| `compute/model.go` | `Algorithm` 增 `measurement`(镜像→度量映射,注册时由 ops 填或首次证明锁定)。 |
| 新 `compute/kbs.go` | `KeyBroker` 接口:`ReleaseDataKey(ctx, quote, datasetID) ([]byte, error)`;`mockKBS`(dev)+ `ccKBS`(真)。 |

## 6. 分步落地(和既有节奏一致:先 Mock 通管线,再上真硬件)

1. **接口固化 + 单测**(本地可做):`KeyBroker` 接口 + `mockKBS`(校验 Mock quote 后释放固定 key);`teeRunner` 串上 KBS 步骤;单测覆盖「证明通过→释放→解密」「证明失败→拒绝释放→作业失败」。
2. **TDX Attester 实现**(需 TDX 机器):接 `/dev/tdx_guest` + DCAP 校验;单测用录制的 quote 样本。
3. **真 KBS**(需 TDX 机器 + KMS):接 Confidential Containers KBS 或云 KMS;端到端:卖方加密→平台持密文→L2 作业→证明→释放→enclave 计算→输出。
4. **运维文档**:`docs/部署-L2-TEE节点与KBS.md`(节点选型、镜像度量注册、密钥策略)。

## 7. 验证策略(诚实边界)

- **本地可验**:接口契约单测、Mock KBS 释放/拒绝逻辑、teeRunner 串联、装配选择。
- **门控(必须 TEE 云)**:真 quote 取证 + DCAP 校验 + KBS 真实释放 + 内存加密「平台 root 也读不到明文」的实证(需在 TDX/SEV 节点上 dump 内存验证密文)。
- **不做半成品门面**:UI 的 L2 徽章(#56 已诚实标注「需 TEE 部署」)在真 KBS 打通前不得宣称「平台不可见」已生效。

## 8. 风险与缓解

- **度量值漂移**:镜像/启动栈变更会改度量 → 注册时锁定度量,变更走 ops 重新审核。
- **TCB 过期**:Intel/AMD 定期更新 TCB,QVL 会报 `OutOfDate` → 校验策略需可配置(拒绝 / 告警放行)。
- **KBS 单点**:KBS 宕机则所有 L2 作业不可运行 → 高可用部署 + 密钥分片备份。
- **侧信道**:TEE 不防所有侧信道(如 SGX 历史漏洞)→ 诚实标注 + 跟进微码更新。

## 9. 国密合规(面向数交所)
quote 签名/摘要可叠加 SM2/SM3(主设计 §16.5);KBS 信道用 SM4。属增强项,不阻塞主路径。
