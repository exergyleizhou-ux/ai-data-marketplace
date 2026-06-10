# 监控栈 — README

## 启动

```bash
export GF_SECURITY_ADMIN_PASSWORD=<强密码>
export PG_EXPORTER_DSN="postgres://user:pass@postgres-host:5432/oasis?sslmode=require"
docker compose -f deploy/monitoring/docker-compose.monitoring.yml up -d
```

## 访问

| 服务 | 地址 | 备注 |
|------|------|------|
| Prometheus | http://localhost:9090 | 指标查询 + 告警状态 |
| Alertmanager | http://localhost:9093 | 告警分组/抑制/路由 |
| Grafana | http://localhost:3000 | 仪表板(admin / 设的密码) |

## 仪表板

Grafana 启动后自动加载 3 张仪表板(mkt-http / mkt-business / mkt-runtime),数据源 DS_PROMETHEUS 自动注册。

## 告警规则

`prometheus/alert-rules.yml` 和 `alertmanager/alertmanager.yml` 由 DS-1 产出,复制到此目录后重启 Prometheus 和 Alertmanager。

## 接入生产

修改 `prometheus/prometheus.yml` 中 `scrape_configs`:

```yaml
# 本地开发
- targets: ["host.docker.internal:8080"]

# Kubernetes 生产
- job_name: backend
  kubernetes_sd_configs:
    - role: pod
      namespaces: { names: [oasis] }
  relabel_configs:
    - source_labels: [__meta_kubernetes_pod_label_app]
      action: keep
      regex: backend
```
