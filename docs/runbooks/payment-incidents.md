# runbook: payment-incidents

## 触发条件

- 运维/客服报告「用户付款了但订单还是 created」
- 支付 webhook 日志异常:签名校验失败堆积/回调重试
- 对账报表(admin/reconciliation)中 created 订单 txn 有误

## 影响评估

- 用户已付款但未收到数据 → **严重**; 数据不丢,需手动推进状态
- 签名校验失败 → 资金侧已扣款但平台不知道; 或假回调被拒(正确行为)
- 订单状态机「卡住」→ 需查 payment/order/audit_log 三表对账

## 诊断步骤

### 1. 三表对账 SQL(线上只读)

```sql
-- 查卡在 created 状态超过 15 分钟的已支付订单
SELECT o.id AS order_id, o.status, o.amount_cents, o.created_at,
       p.channel_txn_id, p.status AS payment_status,
       a.action, a.created_at AS audit_time
FROM orders o
JOIN payments p ON p.order_id = o.id
LEFT JOIN audit_logs a ON a.resource_id = o.id::text AND a.action = 'payment.paid'
WHERE o.status = 'created'
  AND p.channel_txn_id IS NOT NULL
  AND o.created_at < now() - interval'15 minutes'
ORDER BY o.created_at DESC;
```

**注意**: `audit_logs` 是 append-only(触发器保护),**绝不可 UPDATE/DELETE**。

### 2. Webhook 签名校验

```bash
# 查 webhook 日志
kubectl -n marketplace logs -l app=backend --tail=500 | grep -i 'webhook\|signature\|invalid'

# 检查 payment provider 配置
kubectl -n marketplace exec deploy/backend -- env | grep PAYMENT

# 有效签名失败 → webhook secret 可能轮换后没更新配置
# 假回调被拒 → 正确行为,无需处置
```

### 3. 单笔对账(已知 order_id)

```sql
-- 查完整链路
SELECT 'order' AS src, id, status, amount_cents, created_at FROM orders WHERE id='<id>'
UNION ALL
SELECT 'payment', order_id, channel||':'||channel_txn_id, amount_cents, created_at FROM payments WHERE order_id='<id>'
UNION ALL
SELECT 'audit', resource_id, action, 0, created_at FROM audit_logs WHERE resource_id='<id>' ORDER BY created_at;
```

## 处置步骤

### Case A: 支付已到但 webhook 未到(网络)

```bash
# 1. 确认 Stripe/mock Console 里该 payment 是 paid
# 2. 手动触发 webhook 重放(Stripe Dashboard: resend webhook)
#    或通过 /api/v1/payments/dev/mark-paid (仅 test 环境)
# ⚠️ 生产环境不用 dev 端点;用 provider 后台补发 webhook
```

### Case B: 订单 stuck 在 paid(未自动交付)

```bash
# 下载请求未触发 MarkDelivered
# 查 delivery 日志
kubectl -n marketplace logs -l app=backend | grep 'delivery.*<order_id>'
# 如果买家报告无法下载:确认订单 status=paid,指导买家 POST /orders/:id/download
```

### Case C: 签名密钥不匹配

```bash
# 症状:webhook 200 但日志 "invalid callback signature"
# 1. 查 config 中的 webhook secret
kubectl -n marketplace get secret backend-secrets -o jsonpath='{.data.STRIPE_WEBHOOK_SECRET}' | base64 -d
# 2. 对比 Stripe Dashboard 中的 webhook signing secret
# 3. 如不一致 → 更新 secret + restart backend
```

## 升级条件

- 多笔订单(>5)同时卡在 created 且 channel_txn_id 非空 → 支付模块开发
- 有证据表明真金白银已扣但平台未入账 → **紧急**,法务 + 支付机构 + 开发三方连线

## 事后动作

1. 对账确认所有 affected 订单最终状态一致
2. 如果是 webhook 延迟问题 → 增加 webhook 监控/告警
3. 向受影响用户发送通知 + 优惠券(如有)
