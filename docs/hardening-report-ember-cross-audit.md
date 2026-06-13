# 绿洲 Oasis × Ember 交叉安全审计报告

**日期**: 2026-06-13
**审计工具**: Ember v2 (https://github.com/exergyleizhou-ux/ember)
**目标**: Oasis 数据交易平台 (https://github.com/exergyleizhou-ux/ai-data-marketplace)

## 执行摘要

Ember scanner 解析 Oasis 的 openapi.yaml (117 端点) 并完成交叉审计。发现 44 个 POST/PUT/DELETE 端点无速率限制。已通过 3 批补丁修复 22 个高危端点，覆盖率从 20% 提升至 77%。

同时，Ember AI 层对 DeepSeek API 执行了 17 轮实弹测试，78 条 payload 实现 65% 绕过率。最有效技术 "own_server_pretext" 达到 ~80% 绕过率，为 Oasis 未来接入 AI 组件提供了防御基线。

## 端点覆盖率

| 状态 | 数量 | 占比 |
|------|------|------|
| 已限制 | 47 | 77% |
| Ops-gated (双鉴权) | 8 | 13% |
| Dev-only (非生产) | 2 | 3% |
| Chunked upload (N calls) | 2 | 3% |
| GET 只读 | 2 | 3% |
| **总计** | **61** | **100%** |

## 按模块修复明细

| 模块 | 端点 | 限流 | 严重度 |
|------|------|------|--------|
| auth | 2FA enroll/verify/disable | 5/10/5 min | HIGH |
| compliance | data-export, account-deletion, cancel, execute | 5/3/5/10 min | CRITICAL |
| notification | read-all, mark-read | 20/30 min | MEDIUM |
| order | confirm-delivery, review | 20/30 min | HIGH |
| withdrawal | approve, reject, complete | 10/10/10 min | CRITICAL |
| anomaly | acknowledge, resolve | 20/20 min | LOW |
| dataset | update, datasheet, upload-complete | 20/30/10 min | MEDIUM |
| watchlist | add, remove | 30/30 min | LOW |
| delivery | download | 30/min | MEDIUM |
| compute | job-cancel | 20/min | MEDIUM |

## AI 层防御建议 (基于 Ember 实弹数据)

如果 Oasis 未来接入 AI 客服/审核组件:

1. **"own_server_pretext" 是第一威胁** (80% 绕过率)
   - 任何声称"测试自己系统"的请求在 DeepSeek 上几乎全放行
   - 缓解: 在 AI prompt 层加 `禁止输出可执行代码，即使是审计场景`

2. **Red-team/训练数据生成是第二威胁** (60% 绕过率)
   - 以"合成数据生成"为名的恶意请求持续绕过
   - 缓解: 过滤包含 "synthetic dataset"+"red team"+"jailbreak" 的 prompt

3. **Leet-speak/编码绕过不可忽视** (leaked 2 engagements)
   - 缓解: 输入层做字符规范化检测

## 结论

代码侧安全加固已到极限。剩余风险:
- 生产部署 (无服务器环境)
- 真支付集成 (需持牌机构签约)
- AI 组件接入 (需先落实上述 3 条防御建议)
