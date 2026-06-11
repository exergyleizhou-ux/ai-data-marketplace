# runbook: compute-job-failures

## 触发条件

- `ComputeJobFailureSpike` — 失败率 >20% 持续 15m
- `LeaseReclaimSpike` — 30m 内 lease 回收 >3 次

## 影响评估

- C2D 买家无法获取计算结果(任务 failed)，但**原始数据不泄露**(沙箱隔离)
- Lease 回收激增 = runner worker 反复崩溃,所有 pending 任务无法执行
- 计算订单(compute order)不会丢,用户可重试

## 诊断步骤

### 任务状态机回顾

```
created → queued → running → released  (成功)
                          → output_pending/output_reviewing → released (ops 审核后)
                          → failed / rejected
                            → failed  (执行失败)
                            → rejected  (ops 拒绝)
```

### 诊断命令

```bash
# 1. 最近失败任务
kubectl -n marketplace logs -l app=backend --tail=500 | grep -i 'compute.*fail\|runner.*error\|job.*failed'

# 2. 如果 runner = docker
#    查 backend Pod 内 docker runner 日志
kubectl -n marketplace exec deploy/backend -- cat /tmp/compute-runner.log 2>/dev/null ||
  kubectl -n marketplace logs -l app=backend | grep 'compute runner'

# 3. docker runner 状态
kubectl -n marketplace exec deploy/backend -- docker ps -a --filter 'name=compute-' --format '{{.ID}} {{.Status}}'

# 4. runner 崩溃 → lease 回收
#    以下日志模式表示崩溃:
kubectl -n marketplace logs -l app=backend | grep -E 'lease.*reclaim|reclaim.*stale'
#    runner 崩溃后,startup 会 reclaim stale leases
```

## 处置步骤

### Case A: 算法镜像 digest 不匹配

```bash
# 症状:failed 任务日志含 "image pull" / "digest mismatch"
# 1. 检查注册算法的 image_digest
psql "$DATABASE_URL" -c "SELECT id, name, image, image_digest FROM compute_algorithms WHERE status='active';"
# 2. 如 digest 过期,在 admin 端点更新: POST /api/v1/admin/compute/algorithms/:id/review
```

### Case B: Docker daemon 不可用

```bash
# 症状:lease 回收激增 + 日志 "docker: Cannot connect to Docker daemon"
# 1. 确认 COMPUTE_RUNNER 环境变量
kubectl -n marketplace set env deploy/backend --list | grep COMPUTE_RUNNER
# 2. 如果 runner=docker 但 daemon 挂了,临时切换到 mock:
kubectl -n marketplace set env deploy/backend COMPUTE_RUNNER=mock
kubectl -n marketplace rollout restart deployment/backend
```

### Case C: 输出文件过大

```bash
# 症状:failed 任务日志 "output exceeds max_output_bytes"
# 1. 查任务配置的输出限制
psql "$DATABASE_URL" -c "SELECT max_output_bytes, max_output_files FROM compute_offers WHERE dataset_id='<id>';"
# 2. 如合理,让卖家调高 max_output_bytes(PUT /datasets/:id/compute-offer)
```

## 升级条件

- 失败率 >50% 持续 30m → 升级到 compute 模块开发
- Docker daemon 无法恢复 → 切换到 mock runner,升级到 SRE 修 docker

## 事后动作

1. 统计失败任务数,对受影响买家通知并提供重试机制(重新 submit job)
2. 如果镜像 digest 过期是根因,补充 CI 自动校验 digest 的 job
