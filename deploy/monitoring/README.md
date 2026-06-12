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

## 日志聚合(Loki + Promtail)

`docker compose -f docker-compose.monitoring.yml up -d` 现在还拉起 Loki(3100)
和 Promtail。Promtail 用 `docker_sd_configs` 通过 docker socket 发现并抓取
**backend / frontend** 容器的 stdout(JSON 日志),按 `level` / `route` /
`trace_id` 打标签推给 Loki。Grafana 自动注册 Loki 数据源 + `mkt-logs` 看板
(error 计数 + 流式日志窗口,LogQL `{container_name="backend"} | json`)。

## 告警投递(诚实边界)

`alertmanager.yml` 的**路由树 + 抑制规则**是完整可用的(`amtool check-config`
通过):critical→`feishu-critical`(repeat 1h)、warning→`feishu-warning`
(repeat 4h),BackendDown/PostgresDown 抑制下游延迟/5xx 告警。

URL 用 `url_file`(alertmanager **不展开** `${ENV}` 变量),挂在
`alertmanager/secrets/{default,feishu_critical,feishu_warning}_url`(见 `*.example`,
真文件已 gitignore)。

**重要**:`webhook_configs` 发送的是 alertmanager 标准 JSON,**不是**飞书自定义
机器人要求的 `{"msg_type":"text","content":{...}}` 格式。要真正投递到飞书群,
url_file 必须指向一个**桥接服务**(把 alertmanager webhook 转成飞书 API 格式);
`templates/feishu.tmpl` 是给该桥接渲染消息体用的参考(alertmanager 的 generic
webhook 不会用模板渲染 body)。直接指向飞书机器人 URL 会被飞书拒收。
