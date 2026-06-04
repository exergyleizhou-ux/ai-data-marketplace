# 部署 L2 机密计算:TEE 节点 + KBS（Direction B 阶段2）

**目标**：把 L2「连平台也不可见」从 Mock 证明推进到真实硬件。本文档说明两半工作:
- **平台侧（已落地、本地可验证）**：远程 KBS 客户端 `remoteKBS`（`KBS_URL` 选中）——平台把证明报告转发给 KBS，KBS 校验后释放数据密钥。fail-closed,httptest 覆盖。
- **TEE 节点侧（需硬件,本文档指导操作者落地）**：真实硬件 quote 生成 + DCAP 校验 + KBS 密钥释放策略。

> 诚实边界:本仓库**未**附带未经硬件验证的 quote 生成代码(无 TEE 硬件无法 TDD/验证,不做半成品门面)。本文档给出 TEE 节点上要实现/部署的精确步骤。

---

## 0. 全景流程（设计 P3 §4 / Direction B §4）

```
卖方用自己的 KEK 加密数据集 ──▶ 平台只持密文 + 密钥句柄（看不到明文）
买方提交 L2 作业
  │
  ▼ 平台在 TEE 节点起 enclave（运行 digest 钉死的算法镜像）
  │   ① enclave 取硬件 quote（度量值 = 算法镜像 digest）         ← TEE 节点侧
  │   ② teeRunner 把 quote 作为「证明报告」交给 KBS（remoteKBS） ← 平台侧（本 PR）
  │   ③ KBS 校验 quote（真 TEE + 度量∈策略）→ 释放数据密钥进 enclave  ← KBS 侧
  │   ④ 数据仅在 enclave 内用释放的密钥解密、计算
  ▼ 输出经输出闸门 → 释放;quote 写入 compute_jobs.attestation
```

平台侧的 `teeRunner.Run` 在执行算法**前**调用 `KeyBroker.ReleaseDataKey(report, datasetID)`,被拒则作业 fail-closed(算法永不接触数据)。`KBS_URL` 设置后走真实 `remoteKBS`。

---

## 1. 平台侧配置（已就绪）

```bash
COMPUTE_RUNNER=tee
COMPUTE_DOCKER_RUNTIME=kata        # 机密容器(或在 TEE CVM 内跑 runc)
KBS_URL=https://kbs.internal/key-release   # 设置后用 remoteKBS;不设则 dev mockKBS
STORAGE_DRIVER=s3
```

`remoteKBS` 协议(简单、可适配具体 KBS):
- 请求 `POST {KBS_URL}` body `{"report": <base64 证明报告>, "dataset_id": "<id>"}`
- 释放 `200 {"data_key": "<base64 数据密钥>"}`
- 拒绝 任意非 200(KBS 校验失败/策略不允许)→ 平台 fail-closed,无密钥

> 接具体 KBS(Confidential Containers KBS / 云 KMS)时,按其握手协议实现一个适配器即可——稳定接缝是 `KeyBroker.ReleaseDataKey`。

---

## 2. TEE 节点侧:硬件 quote 生成（需实现）

在 TEE 节点上把 `Attester.Attest` 的实现从 `MockAttester`(HMAC 替身)换成真硬件:

### 2.1 Intel TDX（推荐起步）
- enclave 内读 `/dev/tdx_guest`(`TDX_CMD_GET_REPORT` ioctl)→ TD report;
- 经 Quote Generation Service(QGS)签成 quote;度量值 = MRTD/RTMR(算法镜像 + 启动度量)。
- Go 侧:cgo 包 DCAP 的 quote 生成库,或调本地 QGS。

### 2.2 AMD SEV-SNP（Azure / GCP CVM 备选）
- 读 `/dev/sev-guest`(`SNP_GET_REPORT`)→ attestation report(含 launch measurement)。

### 2.3 度量值与算法注册
- 度量值必须等于已注册算法的 `image_digest`(平台已 digest 钉死);
- 注册算法时锁定度量,镜像/启动栈变更走 ops 重新审核。

---

## 3. KBS 侧:基于证明的密钥释放（需部署）

- **选型**:Confidential Containers KBS(开源,attestation-based)/ 云 KMS 的机密计算释放策略 / 自建(enclave 公钥包进 quote,KBS 用该公钥加密数据密钥回传)。
- **校验**:对 quote 做 DCAP/云证明服务校验(PCK 证书链 + TCB 状态 + 度量策略 `measurement ∈ {已注册算法度量}`)。
- **释放策略**:仅当 quote 真实 + 度量在白名单时释放;杜绝「换恶意算法骗取密钥」。
- **信道**:数据密钥仅向通过证明的 enclave 释放;可叠加国密 SM2/SM3/SM4。

---

## 4. 验证（诚实边界）

| 部分 | 状态 |
|---|---|
| `remoteKBS` HTTP 客户端(请求成形/释放/拒绝/fail-closed) | ✅ httptest 覆盖,本地可验 |
| `KeyBroker` 接口 + teeRunner 前置门控 | ✅ 单测(PR #61) |
| 硬件 quote 生成(TDX/SEV) | ⛔ 需 TEE 节点 |
| DCAP/云证明校验 + 真 KBS 释放 | ⛔ 需 TEE 节点 + KBS 部署 |
| 「平台 root 也读不到明文」实证(内存加密) | ⛔ 需在 TEE 节点 dump 内存验证密文 |

---

## 5. 上线检查单
1. TEE 节点(TDX/SEV CVM)就绪,`COMPUTE_DOCKER_RUNTIME=kata`。
2. TEE 节点实现真 `Attester.Attest`(读硬件 quote,度量=镜像 digest)。
3. 部署 KBS,配置度量白名单 = 已批准算法 digest;`KBS_URL` 指向它。
4. 卖方用 KEK 加密数据集,平台只持密文 + 句柄。
5. 端到端:L2 作业 → 硬件 quote → KBS 校验释放 → enclave 内解密计算 → 输出闸门 → 释放。
6. 红队:篡改 quote / 换未注册算法 → KBS 拒绝释放 → 作业 fail-closed,无明文。
