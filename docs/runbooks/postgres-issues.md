# runbook: postgres-issues

## 触发条件

- `PostgresDown` 告警(pg_up==0 持续 1m)
- 后端日志高频 ERROR:"connection refused" / "too many clients"
- 结构化日志中 `slog.Error` 伴随 `"ping database"` 或 `pgx` 前缀

## 影响评估

- 所有需要 DB 的操作全部失败(读写均不可用)
- 订单/支付/提现等资金操作**不会丢失**(支付 webhook 会重试;订单状态机有乐观锁)
- 读操作(浏览/搜索)也受影响(无缓存层)

## 诊断步骤

```bash
# 1. PG 是否存活
psql "$DATABASE_URL" -c "SELECT 1" 2>&1
# 或
pg_isready -d "$DATABASE_URL"

# 2. 连接数
psql "$DATABASE_URL" -c "
  SELECT count(*) AS total,
         count(*) FILTER (WHERE state='active') AS active,
         count(*) FILTER (WHERE state='idle') AS idle
  FROM pg_stat_activity;
"

# 3. 锁等待
psql "$DATABASE_URL" -c "
  SELECT pid, now() - pg_stat_activity.query_start AS duration,
         query, state
  FROM pg_stat_activity
  WHERE wait_event_type = 'Lock' AND state != 'idle'
  ORDER BY duration DESC;
"

# 4. 磁盘空间(wal 堆积是最常见原因)
df -h /var/lib/postgresql/data
psql "$DATABASE_URL" -c "SELECT pg_size_pretty(pg_database_size(current_database()));"
```

## 处置步骤

### Case A: 连接数耗尽

```bash
# 查持有最多连接的来源
psql "$DATABASE_URL" -c "
  SELECT application_name, count(*) FROM pg_stat_activity
  GROUP BY application_name ORDER BY count(*) DESC;
"
# 终止长时间 idle 连接(>10min)
psql "$DATABASE_URL" -c "
  SELECT pg_terminate_backend(pid)
  FROM pg_stat_activity
  WHERE state='idle' AND now()-state_change > interval'10 minutes';
"
```

### Case B: WAL 堆积 → 磁盘满

```bash
# 检查 replication slot 积压
psql "$DATABASE_URL" -c "SELECT slot_name, pg_size_pretty(pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn)) FROM pg_replication_slots;"
# 清理旧 WAL(谨慎:确认无复制延迟)
pg_archivecleanup /var/lib/postgresql/data/pg_wal <oldest-wal-file>
```

### Case C: 慢查询阻塞

```bash
# 查当前运行的慢查询
psql "$DATABASE_URL" -c "
  SELECT pid, now()-query_start AS runtime, left(query,80)
  FROM pg_stat_activity WHERE state='active' AND now()-query_start > interval'5 seconds'
  ORDER BY runtime DESC;
"
# 终止单个慢查询
psql "$DATABASE_URL" -c "SELECT pg_terminate_backend(<pid>);"
```

## 升级条件

- 锁等待超过 5 分钟且无法手动终止 → DBA
- 磁盘 >90% 且无法清理 → 云厂商紧急扩容
- 实例完全不可达且无法重启 → 云厂商紧急恢复

## 事后动作

1. 导出 `pg_stat_statements` 慢查询 top 20,提交给 backend team 优化
2. 如果是连接池配置不足,调整 `db.MaxConns`(当前 10)到生产规模(建议 20-50)
3. 配置 PG 自动备份频次
