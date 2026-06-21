# Oasis Verify — 分发工具包(Distribution Kit)

> **这是"广撒网"的公开渠道工具包(HN / Reddit / X / 冷邮件)。** 要"一对一拉到头 3–5 个真实
> 科研卖家"(真正的瓶颈),配套看 [`创始人-科研卖家拉新手册.md`](创始人-科研卖家拉新手册.md)。
>
> 每条都**可直接粘贴**。发之前把 `<URL>` 换成你部署后的稳定地址(VPS 上线后我会给你)。
> 真实数据来自 live `/screen`:UCI Wine(1599 行,5/8 触发,46 findings,cert `VO-65348C48B4DA`)、
> Iris(150 行,4/8,12,`VO-BCC7451AF229`)、Glass(214 行,5/8,30,`VO-AA113A72D8A4`)。
> **节奏建议**:美西时间周二/周三 8–10am 发 HN;同日发 r/MachineLearning;X thread 当天;冷启动邮件随时。

---

## 1. Show HN(标题 + 正文)

**Title:**
`Show HN: Oasis Verify – integrity-screen any dataset, get a re-hashable certificate`

**Body:**
```
I built an API that screens a dataset for statistical-integrity anomalies and hands
back a certificate that anyone can re-verify by re-hashing the result — it binds the
output to a digest-pinned, audited algorithm running in a network-severed sandbox
(raw rows never leave; only the aggregate report does).

To dogfood it I ran three datasets every ML person has touched. All three trip
integrity detectors — and the honest punchline is that's NOT fraud, it's the screen
doing its job (digit-heaping, rounding, fixed-precision are normal in real data):

  UCI Wine Quality  1,599 rows  5/8 detectors flagged  46 findings   VO-65348C48B4DA
  UCI Iris            150 rows  4/8 detectors flagged  12 findings   VO-BCC7451AF229
  UCI Glass           214 rows  5/8 detectors flagged  30 findings   VO-AA113A72D8A4

Each cert is public + re-hashable: re-run the digest-pinned image on the same data,
re-hash, compare; or look it up at <URL>/verify/<cert> with no account.

Why a cert instead of just running checks locally: you can run the checks — what you
can't easily do is *prove* you ran them, on exactly this data, with exactly this
method, verifiably to someone who doesn't trust you. That's the product.

It's honest about its limits (there's a /c2d/honesty page): the cert guarantees
provenance + integrity, not that the data is "correct" or rights-cleared. Flags ≠
fraud, and it says which is which.

Free tier: 5 scans/month. Try it: <URL>/verify-api

  curl -X POST <URL>/api/v1/screen -H "X-API-Key: sk_live_…" -F "file=@data.csv"

Happy to answer anything about the sandbox/cert design.
```

**回复评论的常见点(备着)**:
- *"How is this different from great-expectations / pandas-profiling?"* → 那些是本地校验,产出留在你机器里;我们产出的是**第三方可验证、可分享的证书**(溯源 + 完整性),且**数据不出沙箱**——适合给别人证明你的数据、或买别人数据前验货。
- *"flags on Iris? so the screen is noisy?"* → 不是噪声,是诚实:它标"异常"(数字分布)不判"造假"。证书如实记录操作特性,不下结论。
- *"open source?"* → 算法 digest 钉死、可审;沙箱姿态公开(`--network=none --read-only`);证书可独立重算。

---

## 2. r/MachineLearning(self-post)

**Title:** `[P] I integrity-screened Wine/Iris/Glass — all 3 "fail", and that's the point`

**Body:**
```
I built a dataset integrity-screening API (Oasis Verify) and ran it on three classic
UCI datasets. All three flag anomalies. Before anyone panics: that's expected — the
screen surfaces digit-distribution / rounding patterns that are *normal* in real
measured data. A flag is "look here," not "fraud."

Results (every number is a real run; every cert is publicly re-verifiable):

| dataset | rows | detectors flagged | findings | cert |
|---|---|---|---|---|
| UCI Wine Quality | 1599 | 5/8 | 46 | VO-65348C48B4DA |
| UCI Iris | 150 | 4/8 | 12 | VO-BCC7451AF229 |
| UCI Glass | 214 | 5/8 | 30 | VO-AA113A72D8A4 |

The interesting bit isn't the flags — it's that the output is a **re-hashable
certificate** binding the result to a digest-pinned algorithm + the source data,
runnable inside a `--network=none` sandbox so raw rows never leave. You can prove a
screen happened, on this data, with this method, without anyone trusting you.

It's deliberately honest about scope (flags ≠ fraud; provenance ≠ correctness).
Free tier, API-first. Curious what failure modes you'd want a screen like this to
catch. <URL>/verify-api
```

---

## 3. X / Twitter(thread)

```
1/ I ran an integrity screen on Wine, Iris, and Glass — the 3 datasets every ML
person has touched.

All 3 "fail."

And that's exactly what should happen. 🧵

2/ The screen flags digit-heaping, rounding, fixed-precision patterns. Those are
NORMAL in real measured data. A flag means "look here," not "fraud."

Real runs, publicly re-verifiable certs:
• Wine 1599 rows → 5/8 flagged
• Iris 150 → 4/8
• Glass 214 → 5/8

3/ The actual product isn't the flags. It's the **certificate**: a re-hashable
record binding the result to a digest-pinned algorithm + the source data, produced
in a network-severed sandbox (raw rows never leave).

You can *prove* a screen happened — verifiably, without anyone trusting you.

4/ Honest about limits (there's a whole /honesty page): it guarantees provenance +
integrity, not that data is "correct" or rights-cleared. Flags ≠ fraud, and it tells
you which.

5/ Free tier, API-first:
curl -X POST <URL>/api/v1/screen -H "X-API-Key: sk_…" -F "file=@data.csv"

Try it → <URL>/verify-api
```

---

## 4. 冷启动邮件 / DM(给 ML 团队 / 数据供应商,1 段)

```
Subject: a 30-second integrity check for your training data (+ a verifiable cert)

Hi <name> — saw you work with <dataset/domain> data. I built a small API that
screens a dataset for integrity anomalies and returns a re-hashable certificate
(provenance + integrity), with the raw rows never leaving a sandbox. I ran it on
Wine/Iris/Glass to dogfood — all flagged the usual digit-distribution patterns
(normal, not fraud), each with a public cert. If you ever need to *prove* a
dataset's quality to a buyer/reviewer/procurement, this might save you a fight.
Free tier, no commitment: <URL>/verify-api . Happy to run your dataset myself if
useful.
```

---

## 5. 嵌入徽章(放进 README / 数据集卡 → 病毒回路)
```html
<a href="<URL>/verify?cert=VO-65348C48B4DA">
  <img src="<URL>/api/v1/verify/VO-65348C48B4DA/badge.svg" alt="Oasis-verified">
</a>
```

---

### 诚实提醒
钱来自客户,客户来自这些帖子真的发出去 + 跟进评论。代码侧的内容我都备齐了(真实数据、可验证证书、诚实框架);**发与跟进是你的动作**。先发 HN + r/ML 同日,盯前 2 小时的评论回复,通常决定一帖的命运。
