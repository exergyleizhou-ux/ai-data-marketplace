# runbook: backend-down

## 触发条件

Prometheus 告警 `BackendDown` — `up{job="backend"} == 0` 持续 1 分钟。

## 影响评估

- 所有 API 端点不可用(前端白屏/报错)
- 支付 webhook 回调丢失(Stripe 会重试,最多 3 天)
- 计算任务中断,lease 回收计数上升(触发 `LeaseReclaimSpike` 警告)

## 诊断步骤

```bash
# 1. Pod 状态
kubectl -n marketplace get pods -l app=backend
kubectl -n marketplace describe pod <pod-name> | tail -30   # 看 Events

# 2. 最近日志
kubectl -n marketplace logs -l app=backend --tail=100

# 3. 区分 liveness(进程存活) 与 readiness(DB 可达)
# /healthz 返回 200 = 进程存活
# /readyz 返回 503 = DB 不可达(最常见原因)
curl -s http://<pod-ip>:8080/readyz

# 4. 如果 readyz 503,查 PG
kubectl -n marketplace exec deploy/backend -- wget -qO- http://<pg-host>:5432 2>&1
# 或用 psql: psql "$DATABASE_URL" -c "SELECT 1"
```

## 处置步骤

### Case A: Pod CrashLoopBackOff

```bash
# 回滚到上一个已知良好的版本
kubectl -n marketplace rollout undo deployment/backend
kubectl -n marketplace rollout status deployment/backend
```

### Case B: PG 不可达(readyz 503)

1. 确认 PG 实例状态(云控制台或 `systemctl status postgresql`)
2. 如果 PG 正常但连接池耗尽:临时重启 backend Pod 重置连接:
   ```bash
   kubectl -n marketplace rollout restart deployment/backend
   ```
3. 如果 PG 宕机:按 `postgres-issues.md` 恢复

### Case C: OOMKilled

```bash
kubectl -n marketplace describe pod <pod> | grep OOMKilled
# 临时提高内存 limit:
kubectl -n marketplace set resources deployment/backend --limits=memory=512Mi
```

## 升级条件

- 回滚后仍 CrashLoopBackOff → 升级到 SRE + 后端开发
- PG 不可达超过 10 分钟 → 升级到 DBA + 云厂商

## 事后动作

1. 查看 `kubectl -n marketplace logs -l app=backend --since=30m | grep ERROR` 定位根因
2. 如果是新部署导致,记录到变更日志;如果是资源耗尽,提交扩容 PR
3. 写 RCA(根因分析)到 `docs/postmortem/`
