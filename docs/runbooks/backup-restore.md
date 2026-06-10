# runbook: backup-restore

> 具体实现已落地:`deploy/backup/backup.sh`(每日 03:00 CronJob,见 `deploy/k8s/60-backup-cronjob.yaml`)、
> `restore.sh`(需显式 `--yes`)、`drill.sh`(CI `backup-drill` job 每次 push 真跑 备份→删库→恢复→校验)。
> 当前为**逻辑备份**:RPO = 备份间隔(默认 24h);PITR 需对象存储,属上线外部门控。

## 触发条件

- 备份作业失败告警(如有)
- 数据误删/损坏的运维或用户报告
- 整库不可恢复的实例故障

## 影响评估(按场景)

| 场景 | 数据丢失范围 | 恢复紧迫度 |
|------|-------------|-----------|
| 误删单表数据 | 特定业务数据(PII/订单/数据集) | 高 — 业务中断局部功能 |
| PG 实例损毁 | 全部数据 | 紧急 — 全站停服 |
| 整库迁移(非故障) | 无丢失,计划内 | 低 — 选低峰窗口 |

## 诊断步骤

```bash
# 1. 确认当前库大小
psql "$DATABASE_URL" -c "SELECT pg_size_pretty(pg_database_size(current_database()));"

# 2. 查最新可用备份(CronJob 落在 pg-backups PVC 的 /backups)
kubectl -n marketplace exec -it deploy/backend -- ls -lt /backups 2>/dev/null \
  || kubectl -n marketplace run -it --rm lsbk --image=busybox --overrides='{"spec":{"containers":[{"name":"lsbk","image":"busybox","command":["ls","-lt","/backups"],"volumeMounts":[{"name":"b","mountPath":"/backups"}]}],"volumes":[{"name":"b","persistentVolumeClaim":{"claimName":"pg-backups"}}]}}'
# 异地副本(对象存储)按你们的同步通道查询

# 3. 最近一次备份 CronJob 是否成功
kubectl -n marketplace get jobs -l job-name --sort-by=.metadata.creationTimestamp | tail -3
```

## 处置步骤 — 决策树

### 场景 A: 误删单表数据(如 `DELETE FROM notifications WHERE user_id='x'` 误执行)

```
→ 不需要全库恢复
→ 从最近 pg_dump 中提取该表数据还原:
  pg_restore -t <table> --data-only -d "$DATABASE_URL" <backup-file>
→ 更安全的做法:先 restore.sh --yes 整库到**新建临时库**,人工比对后只回填该表
→ 注意:会丢失 dump 之后的新数据;精确时刻恢复需 PITR(当前未启用,外部门控)
```

### 场景 B: PG 实例损毁(云实例崩溃/磁盘损坏)

```
→ 紧急:全站不可用
→ 步骤:
  1. 通知上下游(on-call + 用户公告)
  2. 创建新 PG 实例(与原配置相同)
  3. DATABASE_URL 指向新实例,恢复最近 dump:
     deploy/backup/restore.sh --yes /backups/marketplace-<最新>.dump
     然后补迁移:cd backend && go run ./cmd/api --migrate(幂等)
  4. 恢复完成后更新 DATABASE_URL secret
  5. kubectl -n marketplace rollout restart deployment/backend
  6. 验证:curl /readyz → 200
```

### 场景 C: 整库迁移(计划内 — 升级 PG 版本/换云厂商)

```
→ 不紧急,选低峰窗口
→ 步骤:
  1. 在新实例上 pg_dump --schema-only + pg_restore 建表
  2. pg_dump --data-only + pg_restore 导数据(大表并行)
  3. 切换前停 write(如短暂 maintenance mode: /readyz 返回 503)
  4. 增量同步(如有逻辑复制)
  5. 切 DATABASE_URL → kubectl rollout restart
  6. 验证 → 关旧实例
```

## 升级条件

- 备份文件损坏/过期不可用 → 紧急:需重建备份策略
- PITR WAL 缺失导致只能恢复到较早时刻 → 升级到 DBA

## 事后动作

1. 每次恢复后执行数据完整性检查:
   ```sql
   -- 关键表行数验证
   SELECT 'users' AS tbl, count(*) FROM users
   UNION ALL SELECT 'orders', count(*) FROM orders
   UNION ALL SELECT 'payments', count(*) FROM payments
   UNION ALL SELECT 'datasets', count(*) FROM datasets;
   ```
2. 记录恢复耗时,优化备份策略(全量频率/WAL 保留期)
3. 如果恢复演练发现步骤遗漏,更新本文档与 deploy/backup/ 脚本(CI backup-drill 会守住回归)
