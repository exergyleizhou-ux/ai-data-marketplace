# P4-a 联邦学习 MVP — 实现 Spec

**日期**：2026-06-03  **分支**：`feat/p4a-federated`(off `origin/main @ ffb6e3b`)
**来源**：`docs/设计文档-P4-数据不出域(联邦学习与MPC·L3).md` §2.1/§3/§4/§6.1 + 交接文档 §7.2
**范围决定(已批准)**：编排闭环 + **真 FedAvg**;子作业用现有 runner(默认 MockRunner 产出数值参数);真 fed-logreg 训练镜像 + docker 联邦 e2e **延后到 P4-b**。

## 1. 目标与非目标

**目标**：在现有 `compute` 模块上加一层**联邦编排**。一个联邦作业引用 N 个数据集,扇出 N 个**现有沙箱子作业**(各方数据只进各自沙箱),各子作业产出**局部模型参数**(不放行买家),平台用**真实 FedAvg**(按样本数加权平均)聚合成**联合模型**,过现有输出闸门后释放给买家。原始数据从不集中。

**非目标(本切片不做)**：真 fed-logreg docker 训练镜像、docker 联邦 e2e(P4-b)、安全聚合掩码求和(P4-b)、真多方 MPC/PSI(P4-c)、跨域下推节点(P4-d)、前端联邦 UI(可选后续;本切片仅加 `allow_federated` offer 字段到 API+类型)。

## 2. 架构(方案 A:复用 compute_jobs + 事件驱动)

```
POST /compute/federated-jobs (algorithm_id, dataset_ids[N], params, dp_epsilon?)
  → SubmitFederatedJob: 校验每个 dataset offer.allow_federated=true 且买家持 active 权益
  → 建 compute_federated_jobs(status=created) → 为每个 dataset 建 1 个 compute_job(federated_job_id 设上, 扣该 dataset 权益) → 联邦 status=fanout → 子作业入现有队列
  → 子作业走现有 processJob 管线(沙箱/闸门/记账复用);federated_job_id != "" 时:
       局部参数存 output_key, **不放行买家**, 到内部终态(released)后调 tryAdvanceFederated(fedID)
  → tryAdvanceFederated: 计数兄弟作业; 全部 released → 联邦 status=aggregating
       → 读各方 output 参数 → FedAvgAggregator.Aggregate → 联合输出过现有 size+DP 闸门 → 联邦 status=released
       任一子作业 Fail/Reject 且活跃方 < min_participants(MVP=N) → 联邦 status=failed(退还已扣权益)
GET /compute/federated-jobs/:id        → 联邦状态 + N 子作业各自状态
GET /compute/federated-jobs/:id/output → 联合模型(仅 released; 子作业局部参数买家永不可下载)
```

## 3. 数据模型(迁移 `000012_compute_federated.up.sql` / `.down.sql`)

```sql
CREATE TABLE compute_federated_jobs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    buyer_id      UUID NOT NULL REFERENCES users(id),
    algorithm_id  UUID REFERENCES algorithms(id),
    dataset_ids   UUID[] NOT NULL,
    mode          TEXT NOT NULL DEFAULT 'federated',   -- 预留 'mpc'
    status        TEXT NOT NULL DEFAULT 'created',      -- created→fanout→aggregating→released/failed/rejected
    min_participants INT NOT NULL DEFAULT 0,            -- 0 视为 = len(dataset_ids)(MVP 全员)
    params        JSONB NOT NULL DEFAULT '{}',
    dp_epsilon    DOUBLE PRECISION,
    output_key    TEXT,
    output_bytes  BIGINT NOT NULL DEFAULT 0,
    failure_code  TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE compute_jobs ADD COLUMN federated_job_id UUID REFERENCES compute_federated_jobs(id);
CREATE INDEX idx_compute_jobs_federated ON compute_jobs(federated_job_id) WHERE federated_job_id IS NOT NULL;
ALTER TABLE dataset_compute_offers ADD COLUMN allow_federated BOOLEAN NOT NULL DEFAULT false;
```
down: drop 列、索引、表(逆序)。遵循 `database-migrations`:新列均有默认值、可空;新索引 `WHERE` 局部。

## 4. 局部参数 / 联合模型 schema(本切片约定)

子作业局部参数(runner 输出,`OutputKind=model`):
```json
{ "_format": "fedparams-v1", "weights": [f64...], "intercept": f64, "n": int }
```
联合模型(FedAvgAggregator 输出):
```json
{ "_format": "fedmodel-v1", "weights": [f64...], "intercept": f64, "n_total": int, "participants": int }
```
MVP:`MockRunner` 对 `fed-logreg` 算法(按 `Algorithm.Runtime=="fed-logreg"` 或新 `OutputKind`)产出**确定但随 dataset_id 变化**的 `fedparams-v1`(便于聚合有真数可算、可断言)。真训练镜像 P4-b 接,schema 不变。

## 5. 聚合器(`aggregator.go`)

```go
type Partial struct { Weights []float64; Intercept float64; N int }
type Aggregator interface { Aggregate(partials []Partial) ([]byte, error); Kind() string }
type FedAvgAggregator struct{}
// w* = Σ(n_k · w_k) / Σ n_k ; intercept 同理; 输出 fedmodel-v1 JSON。
```
真实现。校验:partials 非空、各方 weights 维度一致(否则 `ErrDimMismatch`)、Σn>0。`MPCAggregator` 仅留接口注释(P4-c),不实现。解析子作业 `fedparams-v1` → `Partial`。

## 6. Repository 扩展(`repo.go` + `Repository` 接口)

新增方法(pgRepo 实现 + 接口声明):
- `CreateFederatedJob(ctx, FederatedJob) (FederatedJob, error)`
- `GetFederatedJob(ctx, id) (FederatedJob, error)`
- `ListSubJobs(ctx, fedID) ([]Job, error)`
- `TransitionFederated(ctx, id, from, to string) (FederatedJob, error)`
- `ReleaseFederated(ctx, id, outputKey string, outputBytes int64) (FederatedJob, error)`
- `FailFederated(ctx, id, code string) (FederatedJob, error)`
- `CreateJob` 扩展:`Job` 增 `FederatedJobID string`(可空),INSERT/SELECT 带上该列。

## 7. Service / Worker 扩展

- `model.go`:`FederatedJob` 结构 + 状态常量(`FedCreated/FedFanout/FedAggregating/FedReleased/FedFailed/FedRejected`);`Job` 增 `FederatedJobID`;`Offer`+`OfferInput` 增 `AllowFederated`。
- `service.go`:`SubmitFederatedJob(ctx, buyerID, FederatedSubmitInput) (FederatedJob, error)`、`GetFederatedJob(ctx, userID, id)`、`OpenFederatedOutput(ctx, userID, id)`;`ConfigureOffer` 透传 `allow_federated`。
- `federated_worker.go`(新):`tryAdvanceFederated(ctx, fedID)`(幂等:仅当全部子作业 released 且联邦仍 fanout 时推进)+ `aggregateAndRelease`(读参数→FedAvg→size/DP 闸门→ReleaseFederated)。
- `worker.go` `processJob`:子作业(`FederatedJobID != ""`)走相同管线,但**跳过买家放行路径**,改存内部 output 并在终态后调 `tryAdvanceFederated`。失败→联邦失败+退权益。
- 装配 `server.go`:联邦 worker 复用现有 runner/worker 池;聚合器默认 `FedAvgAggregator`。

## 8. API(`handler.go`/`router.go`)

- `POST /compute/federated-jobs` · `GET /compute/federated-jobs/:id` · `GET /compute/federated-jobs/:id/output`
- `OfferInput` JSON 增 `allow_federated`;`frontend/lib/api.ts` 类型补字段(前端 UI 编辑器扩展可选,本切片至少类型对齐不破坏 build)。

## 9. 测试(TDD,本地全可跑)

- **单测**:`FedAvgAggregator`(2/3 方加权平均数值正确;单方=恒等;维度不一致报错;Σn=0 报错);状态机转移合法/非法;授权校验(offer 未开 `allow_federated`→拒;缺权益→拒)。
- **真 PG 集成** `federated_integration_test.go`(DATABASE_URL 门控):2 个数据集各建 offer(allow_federated)+授权 → SubmitFederatedJob → MockRunner 产 `fedparams-v1` → 事件驱动聚合 → 断言:联邦 released、联合 `weights`=真 FedAvg、子作业各 released 但 `OpenFederatedOutput` 只给联合模型、买家无法下载子作业局部参数、每 dataset 权益各扣 1。
- **失败路径**:一个子作业 Fail → 联邦 failed + 两个权益均退还。
- 验证铁律:`gofmt -l . && go build ./... && go vet ./... && go test -race ./...`(连真 PG);前端 `npm run typecheck && lint && build`。

## 10. 验证边界(诚实)
- 真验证:编排闭环 + 真 FedAvg 数学 + 状态机 + 授权 + 记账,真 PG 集成。
- 门控延后:真 fed-logreg docker 镜像 + docker 联邦 e2e(P4-b)、安全聚合(P4-b)、MPC/PSI(P4-c)。诚实标注:MVP 子作业为 MockRunner,非真训练;联邦仍可能经模型泄漏 → 后续叠加 DP-SGD。
