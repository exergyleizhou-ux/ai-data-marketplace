# 验证:PaperGuard 数据完整性筛查的统计操作特性

# Validation: operating characteristics of the PaperGuard integrity screen

> 一句话 / In one line:
> **在配对差分实验里,筛查对 6 类注入异常的灵敏度 = 100%,目标检测器的差分响应率 = 100%;但「假阳性率/特异度」我们诚实地不声称——合成 RNG 数据不是数字分布检测器的有效阴性对照,真实 FPR 需要真实干净数据(Part B)。**
> **In a paired differential design, the screen's sensitivity to 6 injected anomaly families = 100% and the targeted detector's differential response = 100%. We do NOT claim a false-positive rate / specificity: synthetic RNG data is not a valid negative control for the digit-distribution detectors; a real-world FPR needs a real clean-data partner (Part B).**

这份文档给 [[PaperGuard]] 数据完整性筛查(`algorithms/paperguard/screen.py`,作为 Oasis C2D 算法运行)补上一个**可复现的统计操作特性验证**,并诚实标注它能证明什么、不能证明什么。脚本:`algorithms/paperguard/validate.py`;结果:`algorithms/paperguard/validation_results.json`。

---

## 1. 为什么是「配对差分」而不是裸 FPR / Why paired-differential, not a raw FPR

一个裸的假阳性率需要一个**有效的阴性对照**:真实的干净数据。我们没有(整个项目的硬约束:无真实数据)。而且——这是验证中发现的一个**真实的事实**——

> **PaperGuard 的数字分布检测器(A1 末位数字、A7 末位 0/5 偏好)会在合成 RNG 数据上触发**:numpy 生成的浮点数的数字分布不满足这些检测器的零假设。即使是均匀末位的整数、Poisson 计数,A1 在 200 行样本上也几乎必然在 `CONCERN` 触发。

所以在合成阴性样本上测出的「特异度/FPR」是**没有意义的**(且会是 over-claim)。诚实的做法是改用**配对差分**:同一个合成基底,一份注入异常、一份不注入,问「目标检测器的信号是否因为注入而增强(更高严重度 / 更多 findings / 更小的 min p 值)」。配对设计**消去了合成基底的噪声**,得到一个有效的因果陈述:**筛查能在自己的噪声地板之上检出该异常**。

---

## 2. 方法 / Method

- **生产筛查,原样运行**:在生产镜像 `vo-paperguard`(digest `sha256:46ca9a23e080…`,paperguard 2.17.0 / py3.11,与 live 证书 `VO-795A4D76D4FE` 同一镜像)内,`--network=none`,跑 `screen()` 的 8 个离线表格检测器(A1/A2/A3/A5/A6/A7/D1/D2)。
- **6 类注入异常,各与其目标检测器对齐**(读了检测器源码后构造,确保命中它真正的触发条件):

  | 异常类型 Anomaly | 注入 Injection | 目标检测器 Target |
  |---|---|---|
  | `digit_heaping` | 70% 数值四舍五入到末位 0/5(数据堆积) | A1 / A7 |
  | `benford_violation` | 首位数字强制偏离 Benford(全是 8/9) | A2 |
  | `implausible_values` | 百分比列注入不可能值(>100、负数) | A6 |
  | `decimal_overconsistency` | 一列小数部分被一个值(".50")主导 | A5 |
  | `smooth_residual` | 过于光滑的捏造线性关系(R²≈1) | D1 |
  | `uniform_variance` | 「过于干净」:0 缺失 + ≥5 列方差近乎一致 | D2 |

- **配对差分**:每类 30 对(同种子的基底 vs 基底+注入);响应 = 目标检测器的严重度/findings/min-p 之一相对基底增强。
- **灵敏度**:筛查的二元判定(`verdict == anomalies_flagged`,即最严重度 ≥ `CONCERN`)对注入数据是否报警。
- **确定性**:固定种子(`seed=20260620`),逐字节可复现。

---

## 3. 结果 / Results

| 异常类型 | 目标检测器 | 灵敏度 Sensitivity | 差分响应率 Differential response |
|---|---|---|---|
| digit_heaping | A1, A7 | **1.00** | **1.00** |
| benford_violation | A2 | **1.00** | **1.00** |
| implausible_values | A6 | **1.00** | **1.00** |
| decimal_overconsistency | A5 | **1.00** | **1.00** |
| smooth_residual | D1 | **1.00** | **1.00** |
| uniform_variance | D2 | **1.00** | **1.00** |
| **总体 Overall** | — | **1.00** | **1.00** |

每一类注入异常都被筛查的二元判定报警,且都让其目标检测器在配对差分中增强信号。

---

## 4. 诚实边界 / Honest scope —— 这能证明什么,不能证明什么

**能证明(GUARANTEES):**
- 筛查对这 6 类有意注入的异常**有响应**,且响应来自正确的检测器(差分把响应归因到目标检测器,而非合成噪声)。
- 结果**确定性、可复现**(固定种子 + digest 钉死的镜像)。

**不能证明(does NOT establish):**
- **不是真实世界的假阳性率/特异度**:合成 RNG 数据不是数字分布检测器的有效阴性对照(A1/A7 会在它上面触发)。真实 FPR 需要**真实的干净数据**——这是 Part B 的门槛(真实多方敏感数据合作方),不是代码能补的。
- **不是真实欺诈检测性能**:这里测的是「对受控注入异常的响应」,不是对真实人为造假的检测率。
- 二元判定应读作**「值得人工复核 warrants review」,不是「认定造假 fraud determination」**——与 [[威胁模型与保证-C2D可验证证据层]] 一致:证书保证溯源,不保证正确性。

---

## 5. 复现 / Reproduce

```bash
# 在仓库根目录,需要本地有 vo-paperguard 镜像(见 algorithms/paperguard/README)
docker run --rm --network=none \
  -v "$PWD/algorithms/paperguard:/app" -w /app \
  --entrypoint python vo-paperguard:dev validate.py
# 输出与 algorithms/paperguard/validation_results.json 逐字节一致(固定种子)。
```

镜像 digest:`sha256:46ca9a23e080ca2bdf4ba010b400341ecc30b587f3b72810196f7c2ed4692eb3`(与 live 证书 `VO-795A4D76D4FE` 同一镜像)。

诚实分级见 `/c2d/honesty`;完整对手分析见 `docs/威胁模型与保证-C2D可验证证据层.md`。
