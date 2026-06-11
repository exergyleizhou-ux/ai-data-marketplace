# runbook: high-5xx

## 触发条件

- `High5xxRate` 告警 — 5xx 占比 >5% 持续 5 分钟
- 前端用户报告「操作失败」「服务异常」等

## 影响评估

- ≥5% 的请求返回 500/502/503/504
- **不影响已有数据完整性**(PG 事务已提交的数据不丢)
- 资金操作:支付创建/提现可能半途失败,用户需重试(幂等设计保证不重复扣款)

## 诊断步骤

```bash
# 1. 按 route + status 分解 5xx 来源
curl -s 'http://localhost:9090/api/v1/query?query=sum(rate(marketplace_http_requests_total%7Bstatus%3D~%225..%22%7D%5B5m%5D))by(route%2Cstatus)' | jq '.data.result'

# 2. 后端结构化日志中查错误
kubectl -n marketplace logs -l app=backend --tail=200 | grep '"level":"ERROR"' | jq -r '[.msg,.route,.error] | @tsv'

# 3. 区分 429(限流) vs 真 5xx
# 429 不是故障 — 是限流正常工作,不需要处置
# 502/503 通常意味着 Pod 重启或 upstream 不可达
# 500 = 未捕获异常:查 stack trace
kubectl -n marketplace logs -l app=backend --tail=500 | grep -A5 'panic\|stack trace'

# 4. 最近部署
kubectl -n marketplace rollout history deployment/backend | head -5
```

## 处置步骤

### Case A: 单个路由 500 集中爆发

```bash
# 1. 定位问题路由后,检查对应模块的 data 层
#    例:POST /orders 大量 500 → 查 orders 表是否有异常数据
# 2. 如果是业务逻辑 bug(如 nil pointer),回滚该模块相关 PR
kubectl -n marketplace rollout undo deployment/backend
```

### Case B: 全路由 503 (Pod 频繁重启)

```bash
kubectl -n marketplace get pods -l app=backend
kubectl -n marketplace describe pod <restarting-pod> | grep -A5 'Last State'
# 如 OOM → 提内存; 如 Readiness probe fail → DB 问题
```

### Case C: 429 告警触发(限流生效,非故障)

在 Prometheus 中确认是 `status="429"` 而非 `5xx`:
```
sum(rate(marketplace_http_requests_total{status="429"}[5m])) by (route)
```
如果 429 占比高,通知产品:当前限流阈值可能过激,需评估调整。

## 升级条件

- 5xx 率 >20% 持续 5 分钟 → 升级到后端 on-call
- 特定路由 100% 500 → 立即回滚 + 通知开发

## 事后动作

1. 按 route 统计受影响请求数,判断是否需要数据修复
2. 如果是新部署引入 → 在 PR 中补充 handler 集成测试覆盖该路由
3. 如果限流误杀 → 评估调整 `ratelimit` 阈值
