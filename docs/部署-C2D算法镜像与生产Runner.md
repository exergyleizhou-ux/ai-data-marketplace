# C2D 算法镜像与生产 Runner 部署

把「可用不可见」沙箱计算从 MockRunner（开发默认）切到**真 Docker 沙箱**(生产)的全部步骤。

## 1. 生产镜像仓库

```
docker.io/yes0505/c2d-algorithm
```

已发布(`algorithms/publish.sh`,2026-06-03 首发 v1.0.0):

| 算法 | tag | `output_kind` | **digest(注册时钉死这个)** |
|---|---|---|---|
| logreg | `logreg-1.0.0` | `model` | `sha256:802cbbd32d3248f7fc4a9d813a05ad25748fcc3cf4135121ac0d392866537cc1` |
| dp_stats | `dp-stats-1.0.0` | `aggregate` | `sha256:c7155be5dc0aa04127bf10c7ca6bd77c6af2c825d9ea458361915317a4757d45` |

重新发布 / 升版本:

```bash
docker login                       # 以 yes0505 账号登录
REGISTRY=docker.io/yes0505/c2d-algorithm ./algorithms/publish.sh   # 构建+推送+打印 digest
```

> **可信算法必须按 digest 钉死,不能用可变 tag**(设计 §4/§7.3):`:latest` 漂移会悄悄让平台的审核失效。`publish.sh` 打印的 `RepoDigests` 就是要钉的值。

## 2. 生产 Runner 配置

平台进程的环境变量:

```
COMPUTE_RUNNER=docker             # 启用真 Docker 沙箱(默认 mock)
# COMPUTE_DOCKER_RUNTIME=runsc    # 可选 P2:gVisor(需在节点装 runsc);默认 runc
STORAGE_DRIVER=s3                  # 输出 + 数据集对象存储(或 local 开发)
```

Runner 节点需要:可达的 Docker 守护进程、能拉 `docker.io/yes0505/c2d-algorithm`、磁盘暂存数据集。容器以平台硬化旗标运行(见 `backend/internal/modules/compute/runner_docker.go` `dockerRunArgs`):`--network=none --read-only --security-opt=no-new-privileges --cap-drop=ALL --pids-limit --memory --cpus --tmpfs`,数据集只读挂 `/data`,输出收 `/out`。

## 3. 注册算法(ops)

经 ops API 注册并按 digest 审核为 trusted(L1 模型输出**必须** trusted):

```bash
# 1) 注册 logreg(替换 <OPS_TOKEN>)
curl -sX POST $API/api/v1/admin/compute/algorithms -H "Authorization: Bearer <OPS_TOKEN>" \
  -H 'Content-Type: application/json' -d '{
    "name":"logreg","runtime":"python-sklearn",
    "image":"docker.io/yes0505/c2d-algorithm",
    "image_digest":"sha256:802cbbd32d3248f7fc4a9d813a05ad25748fcc3cf4135121ac0d392866537cc1",
    "output_kind":"model","source_ref":"git:algorithms/logreg@v1.0.0"
  }'   # → 返回 {id}

# 2) 审核通过 + trusted
curl -sX POST $API/api/v1/admin/compute/algorithms/<ID>/review -H "Authorization: Bearer <OPS_TOKEN>" \
  -H 'Content-Type: application/json' -d '{"status":"approved","trusted":true}'

# dp_stats 同理:output_kind=aggregate,digest=sha256:c7155be5...
```

卖家在数据集上开 offer(`PUT /datasets/:id/compute-offer`,`enabled=true`,`trust_level=L1`,聚合型设 `dp_epsilon` / `dp_epsilon_total`);买家即可购买→提交→在真沙箱执行→下载结果。

## 4. 端到端复验

平台层全栈(真 PG + 真 dockerRunner + 真镜像)已有门控测试:

```bash
docker login
DATABASE_URL=<pg> \
COMPUTE_E2E_IMAGE=docker.io/yes0505/c2d-algorithm \
COMPUTE_E2E_DIGEST=sha256:802cbbd32d3248f7fc4a9d813a05ad25748fcc3cf4135121ac0d392866537cc1 \
  go test -run TestComputeDockerE2E ./backend/internal/modules/compute/
```

> 已用**本地镜像仓库**端到端验证通过(提交→真沙箱→下载 465B 真模型);对生产 Docker Hub 的同一验证在 Hub 可拉取时执行(首发当日 Hub 拉取侧 503,推送侧正常)。§19 沙箱遏制(外联/只读/OOM/超时)在真 Docker 上 5/5 通过。
