# 数据库备份与恢复

逻辑备份(`pg_dump -Fc`)+ 经过验证的恢复流程。**CI 的 `backup-drill` job 每次
push 都真跑一遍 备份→删库→恢复→校验,所以这套脚本的可恢复性是持续验证的,
不是"写了没试过"。**

## 文件

| 文件 | 作用 |
|------|------|
| `backup.sh` | `pg_dump -Fc` → 原子落盘 → 保留期清理 → TOC 完整性自检 |
| `restore.sh` | `pg_restore --clean --if-exists --exit-on-error`,必须显式 `--yes` |
| `drill.sh` | 演练:真迁移→播种→备份→`DROP SCHEMA`→恢复→验证(行数 + audit 触发器) |
| `../k8s/60-backup-cronjob.yaml` | 每日 03:00 CronJob(postgres:16 自带 pg_dump)+ 20Gi PVC |

## 日常使用

```bash
# 手动备份
DATABASE_URL=postgres://... BACKUP_DIR=/backups ./backup.sh

# 恢复(先恢复到全新空库验证,再切流量)
DATABASE_URL=postgres://...new_db ./restore.sh --yes /backups/marketplace-20260611-030000.dump

# 演练(对一次性数据库;需要 psql + go)
DATABASE_URL=postgres://...throwaway ./drill.sh
```

## 恢复决策树

1. **误删/改坏部分数据** → 恢复最近 dump 到**新库**,用 `pg_restore -t <table>`
   按表导出/比对,手工修补生产 — 不要整库覆盖。
2. **实例损毁** → 新实例 → `restore.sh --yes` 最近 dump → 跑
   `go run ./cmd/api --migrate`(幂等,补齐 dump 之后新增的迁移)→ /readyz 验证 → 切流量。
3. **整库迁移** → 同上,但先在目标跑一次 drill.sh 验证目标环境工具链。

## 诚实边界

- 这是**逻辑备份**:RPO = 备份间隔(默认 24h)。要 RPO≈0 需要 PITR
  (`archive_command`/WAL-G + 对象存储)或云厂商托管 PG 的连续备份 —
  两者都依赖外部存储,属上线清单外部门控项,此处不做假实现。
- dump 落在集群内 PVC 上只是第一站;**必须**再同步到异地对象存储
  (restic/rclone/云快照任选),否则集群级故障会同时带走库和备份。
- `users.password_hash`/PII 在 dump 里是明文(库里什么样就什么样)——
  dump 文件按生产数据同级别管控(加密存储、最小访问)。
