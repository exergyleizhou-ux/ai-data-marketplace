# 安全审计报告 — Verdant Oasis 数据市场（Rounds 2–3 完整 posture）

> 截至 marketplace main `@0c7c55b`（2026-06-18）。本文是把分散在 ~30 个 PR 里的两轮对抗审计整理成一份**可上线安全态势**：范围、方法、每条发现（严重度 + 状态 + PR）、有意延后项（带理由）、**残余风险登记表**（部署级 / gated / 设计权衡）、以及**上线安全 checklist**。代码细节见各 PR；本文是审计的权威记录。

---

## 1. 范围与方法

**被审系统**：Go 后端（模块化单体，gin + pgx + Postgres + Redis）+ Next.js 前端 + C2D 隐私计算沙箱（digest-pin 的 docker，`--network none --read-only --cap-drop ALL`）。

**两轮 + 一次平台/前端横扫，覆盖每一层：**

| 轮次 | 范围 | 方法 |
|---|---|---|
| Round 2 | compute 执行路径 + 资金路径（order/payment/withdrawal/DP） | 多 agent 对抗 Workflow（16 agent）+ 每条独立 skeptic 验证（0 误报） |
| Round 3 | 此前未深审的 13 个业务模块（auth/dataset/order/delivery/compliance/quality/moderation/notification/anomaly/watchlist/auditlog/verify/search） | 13 审计 agent + 3 透镜 skeptic；verify 阶段卡死后**逐条对真代码自验** |
| 横扫 | 平台/中间件/配置/JWT + 前端 | 人工审阅（两条独立会话交叉印证） |

**纪律**：每条发现先复现/红，TDD 修复（钱/并发务必活体 DB 验红验绿，compute 务必真 Docker e2e），一卡一 PR，verify-before-claim。**两条会话并行**审同一仓（本文统一记录两条会话成果）。

---

## 2. 已修复发现（全部合并 main）

### 2.1 CRITICAL

| # | 模块 | 问题 | PR |
|---|---|---|---|
| C1 | withdrawal | completed 提现不减可用余额 → **无限重提掏空资金池** + TOCTOU 双提 | #169 |
| C2 | quality/pii | 相邻数字让校验通过的手机/身份证/卡号**绕过检测+脱敏**，`PIIRedaction` 假报清白（脱敏保证失效） | #180 |

### 2.2 HIGH（资金 / compute / 隐私 / authz / 完整性）

| 主题 | 问题 | PR |
|---|---|---|
| compute 沙箱 | 非 root(`--user=65534`)、**每个执行算法强制 digest-pin**（防 `:latest` 镜像替换）、arg-injection `--` 分隔 | #167 |
| compute 输出 | /out OOM→有界读、`--user` 引入的 0700 回归、超时容器 reap | #170 |
| compute DP | DP 预算并发 spend 原子化（advisory-lock，worker 超预算 reject） | #168 |
| compute cert | cert 钉死执行版本、联邦子任务 attestation 守卫 | #172 |
| payment | 争议中禁结算（守卫移出 if-exists）+ Stripe Refund 幂等键 | #171 |
| compute offer | offer 输出门(review/size)在 submit 时**快照到 job**（config-TOCTOU，迁移 028） | #174 |
| dataset 可见性 | 公开 by-id 端点(`/datasets/:id` +versions/quality/cert/croissant)泄露未发布/已下架数据集(provenance/PII flag/seller) | #177 |
| delivery 订单态 | 下载不复查订单状态 → 争议/退款后仍交付 | #178 |
| delivery 版本 | 下载现版本而非购买版本（卖家可换包/投毒） | #187 |
| notification | 邮件 HTML 未转义(存储型 XSS) + SMTP CRLF 头注入 | #179 |
| watchlist | 关注 reviewing 数据集泄露其存在+标题 | #181 |
| moderation | hide/dismiss 无 append-only 审计留痕 | #182 |
| verify | 公开 `/verify/:id` 回显内部 UUID + 无限流(枚举预言机) | #183 |
| compliance | 账户删除遗留完整 PII 导出包(zip + job 行)永久存活（GDPR/PIPL 抹除漏洞，加 `storage.Delete`） | #184 |
| anomaly | `ARRAY_AGG(... LIMIT)` 非法 SQL 让 2/3 检测规则静默失效 | #185 |
| auth 改密 | 改密调用打到**不存在的表**且吞错 → 不撤销任何会话；改为 `tokens_valid_after` epoch（迁移 029），refresh 拒过期 token | #186 |
| auth 撤销 | refresh 撤销在 Redis 故障时 fail-open → 落地为 Postgres 持久撤销表（迁移 031） | #195 |
| audit_logs | append-only 可被 `TRUNCATE` 绕过（行级触发器不在 TRUNCATE 触发）→ 加语句级触发器（迁移 030） | #188 |
| anomaly 检测面 | HighRiskActionRule 白名单引用从未发射的动作 → 补 KYC/withdrawal 管理决策的审计发射 | #199 |
| server 限流 key | X-Forwarded-For 可伪造限流 key → 只信配置的可信代理 | #193 |
| server /metrics | token 用 `!=` 比较(时序预言机) → `crypto/subtle.ConstantTimeCompare` | #198 |

### 2.3 MEDIUM（净正向已修）

| # | 问题 | PR |
|---|---|---|
| M2/M4/M7 | search / `/files/:token` / watch 三个公开/写端点缺限流 | #190 |
| M18 | 2FA 挑战完成漏查冻结账户（Login/Refresh 都查） | #191 |
| M1 | 公开 reviews 泄露 buyer_id + order_id（购买者去匿名化） | #192 |
| M14/M15 | 导出缓存无界增长 + OpenExport 不回退对象存储 | #194 |
| M16 | 举报 target_id 不校验存在 → 举报刷垃圾 | #196 |
| M5 | anomaly 高风险动作白名单失配（见 #199 补发射） | #199 |

---

## 3. 有意延后项（已记录，非疏漏——修复非明显净正向或属设计/部署级）

| 发现 | 为何不在代码层修 |
|---|---|
| audit_logs **完整**防篡改 | 语句触发器(#188)已挡 TRUNCATE；彻底修=**DB 权限分离**（app 用非 owner 角色，仅 INSERT+SELECT，迁移用单独 owner）——部署级 |
| `/out` 磁盘填充 DoS | 代码层已有界读防 OOM；运行期磁盘配额需**宿主机 tmpfs-size/quota**——部署级 |
| cert_id 可重算（sha256[:12]） | 设计属性；改方案=迁移 + 作废已发凭证。verify 端点已不回显内部 id(#183)，枚举面已收 |
| access token 改密后未即时失效 | 设计权衡：access token 短 TTL(15min)自然过期；refresh 已 epoch 失效(#186)。避免每请求 DB 查 |
| 前端 token 存 localStorage | 标准 SPA 权衡；本系统无 XSS sink（无 `dangerouslySetInnerHTML`/`eval`）+ CSP，实际风险低。改 httpOnly cookie 引入 CSRF 等新权衡 |
| CSP `script-src 'unsafe-inline'` | Next.js 内联脚本的已知权衡；收紧需 nonce/hash 化构建 |
| M3/M6/M8/M17 等 nit | 软上限/审计补字段/已 async+已限流/0-行回报——价值低，修即 churn |

---

## 4. 残余风险登记表（上线前需外部动作）

**部署/运维（非代码）：**
- **DB 权限分离**：app 以非 owner 角色连库（audit_logs 仅 INSERT+SELECT；REVOKE TRUNCATE/UPDATE/DELETE/DDL），迁移用单独 owner —— 让 #188 的 append-only 真正不可绕过。
- **宿主机配额**：compute 沙箱 `/out` 的 tmpfs-size / 磁盘 quota，封死磁盘填充。
- **密钥**：生产必须设 `JWT_SECRET`（已 fail-closed，空则拒启动）；`lumen.toml` 内联的 DeepSeek key 在控制台轮换（曾入 git 历史）。
- **TLS / 反代**：`TRUSTED_PROXIES` 配真实可信代理（#193 依赖它）；HSTS 仅 HTTPS 生效。

**Gated（卡外部基建，长线）：**
- L2 真 TEE 硬件（远程证明的硬件半边）；L3 Secretflow 多节点 MPC/PSI；合规法务。

---

## 5. 上线安全 checklist

- [ ] `JWT_SECRET`、`PII_SECRET`、`PAYMENT_*`/`STRIPE_*` 均经环境注入，非默认值
- [ ] app 数据库角色非 owner，audit_logs 仅 INSERT+SELECT；迁移用单独 owner
- [ ] `TRUSTED_PROXIES` = 真实入口代理；`CORS_ALLOW_ORIGIN` = 具体生产源（非 `*`）
- [ ] compute runner = docker + 宿主 `/out` 配额 + 仅 digest-pin 算法
- [ ] DeepSeek key 轮换；`METRICS_TOKEN` 设强随机值
- [ ] 迁移线性应用至 head（注意多分支/多会话的迁移号协调——见交接 v5 的跨分支 DB 陷阱）
- [ ] CI（ci.yml + security.yml）全绿；后端 race+真 DB 集成测试通过

---

## 6. 结论

两轮对抗审计 + 平台/前端横扫覆盖了**资金、compute 隐私执行、13 个业务模块、跨切面平台层、客户端**。所有 **critical/high + 净正向 medium 已修复并合并**，每条带 TDD/活体验证；其余为设计权衡或部署级，已登记。残余风险均为**上线时的外部动作**（权限分离 / 配额 / 密钥 / 可信代理）或 gated 前沿。系统的安全态势：**核心业务+资金+隐私路径已硬化到代码层可达的程度**。
