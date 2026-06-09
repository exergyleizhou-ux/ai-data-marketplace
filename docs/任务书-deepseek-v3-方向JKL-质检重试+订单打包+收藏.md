# 任务书 v3 给 DeepSeek V4 Pro — 方向 J + K + L(3 PR 顺序交付)

**基线**:`origin/main @ 1580d63`(PR #77 已合)
**审核人**:Claude Code(Opus 4.7)
**重要原则**:**3 个 PR 必须严格顺序**(J → K → L),前一个合并后才能开下一个。每 PR 独立审核,独立合并,独立 worktree。

---

## 0. 操作前必做(每次新 PR 都做一遍)

```bash
cat ~/ai-data-marketplace/CLAUDE.md          # 完整读一遍
git fetch origin && git log origin/main -3   # 确认最新基线
```

**继续守住的已记录坑**(`CLAUDE.md` Gotchas 节,PR #77 你刚追加的 3 条 + 之前的):
- 集成测试**必须** `db.RunMigrations(dsn)`,绝不许裸 `CREATE TABLE`
- 每个新 `service.go` / `repo.go` 必须有 `_test.go`
- 通知 emit:`if s.notifier != nil { _ = s.notifier.NotifyUser(...) }`
- 编辑 `.go` 后 **`cd backend && gofmt -w . && goimports -w .`**(PR #77 你的第 5 个 commit 是因为只跑 gofmt 没跑 goimports —— 本次必须**两个都跑**)
- `.tsx` smart-quote 扫描必须 0(在 JSX 分隔符/字符串字面量位置)
- DTO 时间戳 `string` 不是 `time.Time`,沿用既有风格
- JSONB `NOT NULL DEFAULT '{}'` 列传 `[]byte("{}")` 不是 `nil`
- `uuid[]` 用 `$N::uuid[]` 显式转

---

## 1. PR 顺序与依赖

| PR | 方向 | 业务价值 | 估算 |
|---|---|---|---|
| **PR-J** | 质检自动重试 + 持久化 | 质检瞬时失败(sidecar 5xx / 进程崩溃)自动恢复,杜绝数据集卡死 `checking` | ~600 行,10 测试 |
| **PR-K** | 订单合并打包下载 | 买家选 N 个 settled 订单 → 一次下载 zip,提升体验 | ~500 行,8 测试 |
| **PR-L** | 数据集收藏 + 新版本通知 | 买家关注数据集,卖家发新版自动通知,复用 #75 通知模块 | ~600 行,9 测试 |

**铁律**:PR-J 合并后才开 PR-K worktree;PR-K 合并后才开 PR-L worktree。每个 PR 都对齐我的审核清单(本文档 Part 5)。

---

## 2. 工作流模板(每个 PR 都这样)

```bash
export PATH="$HOME/.local/bin:$HOME/sdk/node/bin:$HOME/sdk/pg/bin:$PATH"

git fetch origin
git worktree add ~/ai-data-marketplace-<name> -b feat/<name> origin/main

# ... 实现 + 验证 ...

cd backend && gofmt -w . && goimports -w .   # 必须两个都跑
go vet ./...
go build ./...

# 真 PG 集成测试
T=$(mktemp -d); SOCK=$(mktemp -d); PORT=55440
initdb -D "$T" -U postgres --auth=trust >/dev/null
pg_ctl -D "$T" -o "-p $PORT -k $SOCK -c listen_addresses=''" -w start >/dev/null
DATABASE_URL="postgres://postgres@/postgres?host=$SOCK&port=$PORT&sslmode=disable" \
  go test -race -p 1 -count=1 ./...
pg_ctl -D "$T" stop -m fast >/dev/null

cd ../frontend
npm ci --fetch-retries=5
npx tsc --noEmit
npx next lint
npm run build

# smart-quote 扫描(新增/修改的 .tsx)
python3 -c "
for f in ['frontend/app/admin/page.tsx', 'frontend/app/<新文件>.tsx']:
    d = open(f).read()
    bad = [c for c in d if c in '“”‘’']
    if bad: print(f, 'curly quotes:', len(bad))
"

git push -u origin feat/<name>
gh pr create --base main --title "..." --body "..."
gh pr checks <n> --watch
gh pr merge <n> --squash --delete-branch
git worktree remove ~/ai-data-marketplace-<name>
```

---

# PR-J · 质检自动重试 + 持久化

## J.0 现状(必读)

- **入队**:`backend/internal/modules/dataset/upload.go:112` `s.enqueueQuality(qualityJob{...})`
- **执行**:`backend/internal/modules/dataset/service.go:92-100` 一个 in-memory channel `qCh` + goroutine 池,无持久化
- **失败处理**:`processQuality` 返回 error → 数据集**永远卡在 `checking`**(代码注释说 "retriable" 但**没有任何 retry 机制**)
- **进程崩溃**:`qCh` 是 in-memory,重启即丢失,数据集永远卡死

这是 PR-J 要解决的真缺口。

## J.1 数据模型

**新迁移** `backend/migrations/000015_quality_retries.up.sql`:

```sql
-- 000015: persist quality jobs so transient failures retry and process restarts don't lose work.
CREATE TABLE IF NOT EXISTS quality_retries (
    dataset_id     UUID PRIMARY KEY REFERENCES datasets(id) ON DELETE CASCADE,
    version_id     UUID NOT NULL,
    content_sha256 TEXT NOT NULL,
    attempts       INT  NOT NULL DEFAULT 0,
    max_attempts   INT  NOT NULL DEFAULT 3,
    next_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 找到期待重试的任务:next_at 已到 + 仍可重试
CREATE INDEX IF NOT EXISTS idx_quality_retries_due
    ON quality_retries (next_at) WHERE attempts < max_attempts;
```

**down 文件**(`000015_quality_retries.down.sql`):
```sql
DROP TABLE IF EXISTS quality_retries;
```

## J.2 错误分类(关键)

**新增** `backend/internal/modules/dataset/quality_errors.go`:

```go
package dataset

import "errors"

// QualityErrKind 区分质检失败的两类错误,决定是否值得重试。
type QualityErrKind int

const (
    // QualityErrTransient 是瞬时错误:网络/sidecar 5xx/进程崩溃/DB 暂时不可用。
    // 处理:重试(带指数退避)。
    QualityErrTransient QualityErrKind = iota
    // QualityErrPermanent 是永久错误:文件损坏/对象不存在/SaveQualityCheck schema 冲突。
    // 处理:不重试,直接打回 draft。
    QualityErrPermanent
)

// 用于 processQuality 内部分类返回。
type qualityError struct {
    kind QualityErrKind
    err  error
}

func (e *qualityError) Error() string { return e.err.Error() }
func (e *qualityError) Unwrap() error { return e.err }

// 永久错误的哨兵:processQuality 遇到这类直接打 permanent
var (
    ErrObjectNotFound       = errors.New("quality: source object not found")
    ErrInvalidContent       = errors.New("quality: content is invalid (cannot decode)")
)

// classifyQualityError 把 processQuality 内部错误归类:
//   - context.DeadlineExceeded / io 错误 / HTTP 5xx → Transient
//   - errors.Is(err, ErrObjectNotFound) / ErrInvalidContent → Permanent
//   - 默认 → Transient(保守:宁可重试,不要冤枉买家上传)
func classifyQualityError(err error) QualityErrKind {
    if err == nil { return QualityErrTransient }
    if errors.Is(err, ErrObjectNotFound) || errors.Is(err, ErrInvalidContent) {
        return QualityErrPermanent
    }
    // 未来可扩展:net.OpError / pgconn.PgError 等
    return QualityErrTransient
}
```

## J.3 Repo 接口扩展

**改** `backend/internal/modules/dataset/repo.go`:在 `Repository` 接口加 4 个方法,在 `pgRepo` 实现:

```go
// 在 Repository 接口里加:
EnqueueQualityRetry(ctx context.Context, datasetID, versionID, contentSHA string, maxAttempts int) error
ListDueQualityRetries(ctx context.Context, limit int) ([]QualityRetryRow, error)
MarkQualityRetryAttempt(ctx context.Context, datasetID string, nextAt time.Time, lastErr string) error
DeleteQualityRetry(ctx context.Context, datasetID string) error

// 新 DTO:
type QualityRetryRow struct {
    DatasetID     string
    VersionID     string
    ContentSHA256 string
    Attempts      int
    MaxAttempts   int
    LastError     string
}
```

**EnqueueQualityRetry** SQL:
```sql
INSERT INTO quality_retries (dataset_id, version_id, content_sha256, attempts, max_attempts, next_at)
VALUES ($1, $2, $3, 0, $4, now())
ON CONFLICT (dataset_id) DO UPDATE
  SET version_id = EXCLUDED.version_id,
      content_sha256 = EXCLUDED.content_sha256,
      attempts = 0,
      max_attempts = EXCLUDED.max_attempts,
      next_at = now(),
      last_error = NULL,
      updated_at = now()
```

**ListDueQualityRetries** SQL(带 advisory-lock 风格的原子取出):
```sql
SELECT dataset_id::text, version_id::text, content_sha256, attempts, max_attempts, COALESCE(last_error, '')
FROM quality_retries
WHERE next_at <= now() AND attempts < max_attempts
ORDER BY next_at ASC
LIMIT $1
```

**MarkQualityRetryAttempt** SQL:
```sql
UPDATE quality_retries
SET attempts = attempts + 1,
    next_at = $2,
    last_error = $3,
    updated_at = now()
WHERE dataset_id = $1
```

**DeleteQualityRetry** SQL:
```sql
DELETE FROM quality_retries WHERE dataset_id = $1
```

## J.4 Service 改动

**改** `backend/internal/modules/dataset/service.go`:

```go
// 1. 启动 + 后台扫描器
// 在 NewService 现有 goroutine 启动后增加:
go svc.qualityRetryLoop(context.Background())

// 2. 新方法 qualityRetryLoop:
func (s *Service) qualityRetryLoop(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done(): return
        case <-ticker.C:
            rows, err := s.repo.ListDueQualityRetries(ctx, 10)
            if err != nil {
                slog.Warn("quality retry list failed", "err", err); continue
            }
            for _, r := range rows {
                s.enqueueQuality(qualityJob{
                    DatasetID: r.DatasetID, VersionID: r.VersionID,
                    ContentSHA256: r.ContentSHA256,
                })
            }
        }
    }
}
```

**改造** `enqueueQuality` 的失败处理(原来 sync 路径 `if err := s.processQuality(...); err != nil`):

```go
func (s *Service) enqueueQuality(job qualityJob) {
    select {
    case s.qCh <- job:
    default:
        // 通道满 → 走持久化重试(60s 后)
        _ = s.repo.EnqueueQualityRetry(context.Background(),
            job.DatasetID, job.VersionID, job.ContentSHA256, 3)
    }
}
```

**改造** worker goroutine(`service.go:97`):

```go
if err := svc.processQuality(context.Background(), job); err != nil {
    kind := classifyQualityError(err)
    if kind == QualityErrPermanent {
        // 永久错误:打回 draft + 通知 + 删除 retry 记录
        _ = svc.repo.SetStatus(context.Background(), job.DatasetID, StatusDraft)
        _ = svc.repo.DeleteQualityRetry(context.Background(), job.DatasetID)
        // 通知卖家(走已有的 quality_done 通知,reason="invalid_content")
        if d, gerr := svc.repo.GetByID(context.Background(), job.DatasetID); gerr == nil && svc.notifier != nil {
            _ = svc.notifier.NotifyUser(context.Background(), d.SellerID, "quality_done",
                "质检无法处理", "数据集「"+d.Title+"」内容无法解析,请检查后重新上传。",
                "dataset", d.ID)
        }
        slog.Warn("quality permanent fail", "dataset", job.DatasetID, "err", err)
        continue
    }
    // 瞬时错误:登记重试 + 指数退避
    nextAt := computeRetryBackoff(job)
    _ = svc.repo.MarkQualityRetryAttempt(context.Background(), job.DatasetID, nextAt, err.Error())
    slog.Info("quality retry scheduled", "dataset", job.DatasetID, "next_at", nextAt, "err", err)
}
```

**新辅助函数**(同文件或新 `quality_retry.go`):

```go
// 指数退避:第 N 次重试在 2^N * 30 秒后(0→30s, 1→60s, 2→120s)
func computeRetryBackoff(job qualityJob) time.Time {
    // 这里需要查 repo 拿到 attempts 数;为了简化,可让 MarkQualityRetryAttempt 自增 attempts 后再 SELECT,
    // 或在 ListDueQualityRetries 返回 attempts 时直接传进来。
    // 简化方案:在 service 这层用一个 lookup attempts 的辅助,或者:
    //   在 worker goroutine 调用前先 query 当前 attempts → 计算 backoff
    // 实现自己选,但 backoff 表必须是:0→30s, 1→60s, 2→120s
}
```

**最终成功时**(`processQuality` 返回 nil 后):

```go
// 在 worker goroutine 内,processQuality 成功:
_ = svc.repo.DeleteQualityRetry(context.Background(), job.DatasetID)
```

## J.5 一处关键 hook:首次 enqueue 也写持久层

**改** `upload.go:112`(上传 finalize 后):

```go
// 原来只 enqueueQuality(job)
// 改为:先 EnqueueQualityRetry(持久化) 再 enqueueQuality(尝试 in-memory)
_ = s.repo.EnqueueQualityRetry(ctx, d.ID, versionID, obj.SHA256, 3)
s.enqueueQuality(qualityJob{DatasetID: d.ID, VersionID: versionID, ContentSHA256: obj.SHA256})
```

这样即使 in-memory 通道丢任务、进程崩溃,后台扫描器会接管。**成功后才 DELETE retry 记录**(见 J.4 worker 末尾)。

## J.6 前端(可选,**强烈推荐**)

**改** `frontend/app/sell/page.tsx`:

数据集状态显示扩展,当数据集 status='checking' 且 `quality_retries.attempts > 0` 时显示重试进度。

**简化方案**:在 `GET /datasets/:id` 响应里加 `quality_retry_state?: { attempts: int, max_attempts: int, next_at: string, last_error: string }` 字段(repo 查 quality_retries 表;无记录则 nil 不返回)。前端在 sell 页 status='checking' 行下展示:

```
质检中... 重试 {attempts}/{max_attempts} (上次错误: {last_error_truncated_50}, 下次重试: {humanize next_at})
```

**Honest 标注**:重试进度仅供卖家观察,buyer 看不到。

## J.7 测试(**必须** 10 个)

**新文件** `backend/internal/modules/dataset/quality_retry_test.go`:

```go
// 1. 错误分类
func TestClassifyQualityError_ObjectNotFound_Permanent(t *testing.T)
func TestClassifyQualityError_InvalidContent_Permanent(t *testing.T)
func TestClassifyQualityError_GenericError_Transient(t *testing.T)
func TestClassifyQualityError_Nil_Transient(t *testing.T)  // (退化默认值)

// 2. Backoff 表
func TestComputeRetryBackoff_30_60_120(t *testing.T)
//   断言 attempts=0 → 30s, attempts=1 → 60s, attempts=2 → 120s(允许 ±2s 容差)
```

**新文件** `backend/internal/modules/dataset/quality_retry_repo_test.go`(用 `db.RunMigrations`):

```go
func TestEnqueueQualityRetry_InsertThenUpsert(t *testing.T)
//   同 dataset_id 第二次 EnqueueQualityRetry → attempts 重置为 0, last_error 清空

func TestListDueQualityRetries_OnlyReturnsDueRows(t *testing.T)
//   插 2 条 next_at 已过 + 1 条 next_at 是 1 小时后 → List 只返回 2

func TestListDueQualityRetries_ExcludesMaxedOut(t *testing.T)
//   插 attempts=3 max_attempts=3 的行 → List 不返回它

func TestMarkQualityRetryAttempt_IncrementsAndUpdatesNextAt(t *testing.T)
//   Mark 后查 attempts+1, next_at == 传入值, last_error 持久化

func TestDeleteQualityRetry_RemovesRow(t *testing.T)
//   Delete 后再 ListDueQualityRetries → 空
```

## J.8 接线 `backend/internal/server/server.go`

`dataset.NewService` 已经接了 repo,**不需要新接口**。但要确认 `qualityRetryLoop` 在 NewService 启动 worker 后自动跑(在 J.4 写了 `go svc.qualityRetryLoop(...)`,不要忘记)。

## J.9 不许做

| ❌ | 原因 |
|---|---|
| 改 `quality.Check` / `quality.Authenticity` 等算法层 | 算法层稳定,只改 orchestration |
| 改 `qCh` 大小或 worker 数量 | 性能调优另开 PR |
| 用 cron 包(robfig/cron 等) | `time.Ticker` 够用,不引入 deps |
| 让 retry 改 `dataset.Status` 离开 `checking` | 状态机由 processQuality 控制,retry 只是触发器 |

---

# PR-K · 订单合并打包下载

## K.0 现状(必读)

- `backend/internal/modules/delivery/service.go:89` 既有单文件下载,基于 `storage.Open(ctx, key)` + io.Copy 或 presigned URL
- `backend/internal/modules/order/model.go` 定义 `StatusSettled`、`ProductType` (`download` / `compute`)
- 当前无打包下载;买家若有 10 个 settled 订单只能逐个下载

## K.1 端点

```
POST /users/me/orders/bundle
Body: { "order_ids": ["uuid1", "uuid2", ...] }
Response: zip 流,Content-Type: application/zip,
          Content-Disposition: attachment; filename="oasis-bundle-<timestamp>.zip"
Auth: authed(自报范围)
```

**约束**:
- order_ids 长度:1-20(超出 400)
- 必须**全部**属于当前买家(任一不是 → 403)
- 必须**全部** `status='settled'`(任一不是 → 409 with message)
- 必须**全部** `product_type='download'`(compute 订单不打包 → 400)

## K.2 后端文件

**新** `backend/internal/modules/order/bundle.go`:

```go
package order

import (
    "archive/zip"
    "context"
    "fmt"
    "io"

    "github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// BundleSource 是 server 注入的依赖,根据 dataset_id 查 storage 对象 key + 文件名。
type BundleSource interface {
    // CurrentObjectKey 返回数据集当前版本的存储 key。
    CurrentObjectKey(ctx context.Context, datasetID string) (string, error)
    // SuggestFilename 返回 zip 内对该订单使用的文件名(数据集 title slug + 扩展名)。
    SuggestFilename(ctx context.Context, datasetID string) (string, error)
}

// BundleOrders 把多个 settled 订单的对象文件流式打包到 w。
// 在写第一个 zip 字节之前完成所有校验,如果中途某个对象打开失败,zip 在该项处中断
// (Close 仍调用,文件名表/中央目录写出,前面的文件仍可解出)。
func (s *Service) BundleOrders(ctx context.Context, buyerID string, orderIDs []string, w io.Writer) error {
    if len(orderIDs) == 0 { return fmt.Errorf("%w: no orders", ErrValidation) }
    if len(orderIDs) > 20 { return fmt.Errorf("%w: max 20 orders per bundle", ErrValidation) }

    // 1. 全部预校验(原子性)
    orders := make([]Order, 0, len(orderIDs))
    for _, id := range orderIDs {
        o, err := s.repo.GetByID(ctx, id)
        if err != nil { return err }
        if o.BuyerID != buyerID { return ErrForbidden }
        if o.Status != StatusSettled { return fmt.Errorf("%w: order %s not settled", ErrBadTransition, id) }
        if o.ProductType != ProductDownload {
            return fmt.Errorf("%w: compute orders cannot be bundled (%s)", ErrValidation, id)
        }
        orders = append(orders, o)
    }

    if s.bundle == nil || s.store == nil {
        return fmt.Errorf("bundle not configured: storage or source missing")
    }

    // 2. 写 zip
    zw := zip.NewWriter(w)
    defer zw.Close()  // 即使中途失败也写中央目录

    for _, o := range orders {
        key, err := s.bundle.CurrentObjectKey(ctx, o.DatasetID)
        if err != nil { return fmt.Errorf("source key for order %s: %w", o.ID, err) }
        filename, err := s.bundle.SuggestFilename(ctx, o.DatasetID)
        if err != nil { return fmt.Errorf("filename for order %s: %w", o.ID, err) }
        // 在 zip 里用 <orderID短码>_<datasetname> 避免重名
        entryName := o.ID[:8] + "_" + filename

        rc, _, err := s.store.Open(ctx, key)
        if err != nil { return fmt.Errorf("open object for order %s: %w", o.ID, err) }

        entry, err := zw.Create(entryName)
        if err != nil { rc.Close(); return fmt.Errorf("zip entry for order %s: %w", o.ID, err) }

        if _, err := io.Copy(entry, rc); err != nil {
            rc.Close()
            return fmt.Errorf("copy for order %s: %w", o.ID, err)
        }
        rc.Close()
        s.audit.Record(ctx, audit.Entry{
            ActorID: buyerID, Action: "order.bundle_download",
            ResourceType: "order", ResourceID: o.ID,
        })
    }
    return nil
}
```

**改** `backend/internal/modules/order/service.go` 加 `store storage.Storage` 字段 + `bundle BundleSource` 字段 + `SetStorage` / `SetBundleSource` setter(late-bind 同 SetNotifier 模式):

```go
type Service struct {
    // ... 原有字段
    store  storage.Storage  // 新
    bundle BundleSource     // 新
}

func (s *Service) SetStorage(st storage.Storage)    { s.store = st }
func (s *Service) SetBundleSource(b BundleSource)   { s.bundle = b }
```

**新 handler** `backend/internal/modules/order/handler.go`:

```go
func (h *handler) bundleDownload(c *gin.Context) {
    var req struct { OrderIDs []string `json:"order_ids"` }
    if err := c.ShouldBindJSON(&req); err != nil {
        httpx.Fail(c, httpx.ErrInvalidParam); return
    }

    // 在写任何字节之前先做完整校验(BundleOrders 自身也做,这里不重复)。
    // 设置流式响应头
    ts := time.Now().UTC().Format("20060102-150405")
    c.Header("Content-Type", "application/zip")
    c.Header("Content-Disposition", `attachment; filename="oasis-bundle-`+ts+`.zip"`)
    c.Header("X-Content-Type-Options", "nosniff")

    if err := h.svc.BundleOrders(c.Request.Context(), httpx.UserID(c), req.OrderIDs, c.Writer); err != nil {
        // 已经写过 header,只能尽力 trailers/日志
        slog.Warn("bundle download failed", "err", err)
        // 如果 zip writer 尚未写任何字节(校验阶段失败),可正常返回 JSON 错误
        // 这里需要 BundleOrders 区分校验阶段 vs 写入阶段错误
    }
}
```

**注意 errors 处理边界**:校验阶段错误 → 用 `fail(c, err)` 返回标准 JSON;写入阶段错误 → 只能日志(已经送了 zip header,buyer 拿到的是部分 zip)。

最简策略:在 `BundleOrders` 内,先**全部预校验**(GetByID 循环),通过后**才**开始写 zip。校验阶段返回 error → 调用方判定`if !headersWritten { fail(c, err) }`。

**新 route** `backend/internal/modules/order/router.go`:
```go
authed.POST("/users/me/orders/bundle", h.bundleDownload)
```

## K.3 接线 `backend/internal/server/server.go`

`OrderModule` 注册后加:

```go
// 既有 storage、dataset svc 已存在;注入到 order
orderSvc.SetStorage(store)
orderSvc.SetBundleSource(orderBundleAdapter{ds: dsSvc})

// 文件末尾加适配器(避免 order 模块直接 import dataset):
type orderBundleAdapter struct{ ds *dataset.Service }
func (a orderBundleAdapter) CurrentObjectKey(ctx context.Context, datasetID string) (string, error) {
    return a.ds.CurrentObjectKey(ctx, datasetID)
}
func (a orderBundleAdapter) SuggestFilename(ctx context.Context, datasetID string) (string, error) {
    d, err := a.ds.GetByID(ctx, datasetID)
    if err != nil { return "", err }
    return slugify(d.Title) + ".bin", nil
}
func slugify(s string) string {
    // 简单 slugify:小写、空格变下划线、移除 / \\ : * ? " < > |
    // 自己实现,不引 deps
}
```

**dataset.Service 需新增 public method** `GetByID` 暴露(若已有则跳过)。

## K.4 前端 `frontend/app/orders/page.tsx`

- 加 `selected: Set<string>` state
- 每个 settled + download 类型的订单行加 checkbox
- 顶部加「打包下载所选 (N)」按钮(N>0 时显示)
- 按钮 onClick:
  ```ts
  async function bundleDownload() {
    const res = await fetch(buildURL("/users/me/orders/bundle"), {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(tokenStore.access ? { Authorization: `Bearer ${tokenStore.access}` } : {}),
      },
      body: JSON.stringify({ order_ids: [...selected] }),
    });
    if (!res.ok) { throw new Error("打包下载失败"); }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `oasis-bundle-${Date.now()}.zip`;
    document.body.appendChild(a);
    a.click(); a.remove();
    URL.revokeObjectURL(url);
  }
  ```

**i18n**:中英对照「打包下载所选」/`Download selected as zip`。

## K.5 `frontend/lib/api.ts`

加方法(类似 `downloadFederatedOutput` 的 fetch + blob 模式):

```ts
bundleOrders: async (orderIds: string[]) => {
  const res = await fetch(buildURL("/users/me/orders/bundle"), {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...(tokenStore.access ? { Authorization: `Bearer ${tokenStore.access}` } : {}),
    },
    body: JSON.stringify({ order_ids: orderIds }),
  });
  if (!res.ok) throw new ApiError(-1, res.status, "打包下载失败");
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `oasis-bundle-${Date.now()}.zip`;
  document.body.appendChild(a);
  a.click(); a.remove();
  URL.revokeObjectURL(url);
},
```

## K.6 测试(**必须** 8 个)

**新文件** `backend/internal/modules/order/bundle_test.go`(用 fakes,不需要 real PG):

```go
type fakeStorage struct {
    files map[string][]byte  // key → bytes
}
func (f *fakeStorage) Open(ctx context.Context, key string) (io.ReadCloser, int64, error) {
    if b, ok := f.files[key]; ok {
        return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
    }
    return nil, 0, fmt.Errorf("not found: %s", key)
}
// 其他 Storage 方法返回 panic("not used") 或 zero value

type fakeBundleSource struct {
    keys      map[string]string  // dataset_id → key
    filenames map[string]string  // dataset_id → filename
}
func (f *fakeBundleSource) CurrentObjectKey(ctx context.Context, did string) (string, error) {
    return f.keys[did], nil
}
func (f *fakeBundleSource) SuggestFilename(ctx context.Context, did string) (string, error) {
    return f.filenames[did], nil
}

func TestBundleOrders_PacksAllSettledIntoValidZip(t *testing.T)
//   2 个 settled download 订单 + 2 个文件,BundleOrders 输出 zip → zip.NewReader 解析后 2 个 entry,内容匹配

func TestBundleOrders_RejectsEmptyOrderIDs(t *testing.T)
//   空 order_ids → ErrValidation

func TestBundleOrders_RejectsMoreThan20(t *testing.T)
//   21 个 → ErrValidation

func TestBundleOrders_RejectsForeignOrder(t *testing.T)
//   订单 BuyerID 不匹配 → ErrForbidden,并且 w(bytes.Buffer) 未被写入

func TestBundleOrders_RejectsNonSettledOrder(t *testing.T)
//   订单 status='paid' → ErrBadTransition

func TestBundleOrders_RejectsComputeOrder(t *testing.T)
//   订单 product_type='compute' → ErrValidation

func TestBundleOrders_PreflightFailureDoesNotWriteZipBytes(t *testing.T)
//   2 个订单 + 第二个属于别人,bytes.Buffer 在校验失败时仍为空(没有写 zip header)

func TestBundleOrders_StorageOpenFailureMidStreamReturnsError(t *testing.T)
//   第一个文件成功打开,第二个 storage.Open 返回错误 → BundleOrders 返回 err,
//   zip writer Close 仍被调用(zip 仍可解,只是少第二个文件)
```

## K.7 不许做

| ❌ | 原因 |
|---|---|
| 把所有文件 buffer 到内存再 zip | 必须流式(`io.Copy`) |
| 让 bundle 写订单状态 | 只读,不更新 status |
| 加 zip 加密 | 超出范围;真要密文走 delivery 现有 presign |
| 用 archive/tar | 用户期望 zip;archive/zip 标准库够用 |
| 暴露超过 20 个/请求 | 防御性上限,保护 storage I/O |

---

# PR-L · 数据集收藏 + 新版本通知

## L.0 现状(必读)

- `backend/internal/modules/dataset/review.go:30` `Review(approve=true)` → `SetStatus(StatusPublished)` 是发布钩子
- `backend/internal/modules/notification/` 模块已就绪(#75)
- 当前买家没有"关注/收藏"概念,新版本发布无通知

## L.1 数据模型

**新迁移** `backend/migrations/000016_dataset_watches.up.sql`:

```sql
-- 000016: buyer watchlist + last-notified version tracking.
CREATE TABLE IF NOT EXISTS dataset_watches (
    user_id                    TEXT NOT NULL,
    dataset_id                 UUID NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
    last_notified_version_id   UUID,             -- 设为当前 current_version_id 时只通知未来变化
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, dataset_id)
);

CREATE INDEX IF NOT EXISTS idx_dataset_watches_dataset ON dataset_watches (dataset_id);
CREATE INDEX IF NOT EXISTS idx_dataset_watches_user    ON dataset_watches (user_id, created_at DESC);
```

**down**:
```sql
DROP TABLE IF EXISTS dataset_watches;
```

## L.2 新模块 `backend/internal/modules/watchlist/`

```
watchlist/
  model.go      (Watch DTO + WatchersNotifier 接口 + 错误哨兵)
  repo.go       (Add/Remove/ListByUser/ListByDataset/MarkNotified)
  service.go    (Add/Remove/ListMy)
  handler.go    (POST /datasets/:id/watch, DELETE, GET /users/me/watched)
```

### L.2.1 DTO

```go
type Watch struct {
    UserID                  string `json:"user_id"`
    DatasetID               string `json:"dataset_id"`
    DatasetTitle            string `json:"dataset_title,omitempty"`  // join 时填
    LastNotifiedVersionID   string `json:"last_notified_version_id,omitempty"`
    CreatedAt               string `json:"created_at"`
}

var ErrNotFound = errors.New("watch not found")
```

### L.2.2 Repo SQL

```go
// Add: 加入收藏,初始 last_notified 为当前 current_version_id(只通知未来变化)
INSERT INTO dataset_watches (user_id, dataset_id, last_notified_version_id)
SELECT $1, $2, current_version_id FROM datasets WHERE id = $2
ON CONFLICT (user_id, dataset_id) DO NOTHING

// Remove
DELETE FROM dataset_watches WHERE user_id=$1 AND dataset_id=$2

// ListByUser(连 datasets 表拿 title)
SELECT w.dataset_id::text, COALESCE(d.title, ''),
       COALESCE(w.last_notified_version_id::text, ''),
       w.created_at::text
FROM dataset_watches w
LEFT JOIN datasets d ON d.id = w.dataset_id
WHERE w.user_id = $1
ORDER BY w.created_at DESC
LIMIT 100

// ListByDataset(发布钩子用)
SELECT user_id, COALESCE(last_notified_version_id::text, '')
FROM dataset_watches
WHERE dataset_id = $1

// MarkNotified
UPDATE dataset_watches
SET last_notified_version_id = $3
WHERE user_id = $1 AND dataset_id = $2
```

### L.2.3 服务

```go
type Service struct {
    repo Repository
}

func (s *Service) Add(ctx context.Context, userID, datasetID string) error
func (s *Service) Remove(ctx context.Context, userID, datasetID string) error
func (s *Service) ListMy(ctx context.Context, userID string) ([]Watch, error)
// 给 dataset 模块调用:通知所有 watchers 数据集有新版本
func (s *Service) NotifyDatasetPublished(ctx context.Context, datasetID, newVersionID, datasetTitle string)
```

`NotifyDatasetPublished` 实现:
```go
// 1. ListByDataset
// 2. for each watcher whose last_notified_version_id != newVersionID:
//      _ = s.notifier.NotifyUser(ctx, w.UserID, "dataset_updated",
//          "关注的数据集有更新", "数据集「"+datasetTitle+"」已发布新版本。",
//          "dataset", datasetID)
//      _ = s.repo.MarkNotified(ctx, w.UserID, datasetID, newVersionID)
// 3. 任一失败 _ = 吞错,不阻塞
```

### L.2.4 Handler

```go
POST   /datasets/:id/watch           → svc.Add(ctx, userID, datasetID); httpx.OK 200 {ok:true}
DELETE /datasets/:id/watch           → svc.Remove(...); httpx.OK 200 {ok:true}
GET    /users/me/watched              → svc.ListMy(...); httpx.OK 200 {items: [Watch...]}
```

**Auth**:三个都挂 `authed`(authMW);用户自身范围.

**特别注意**:`POST /datasets/:id/watch` 应**对任何 logged-in user 开放**(包括非 buyer,因为发现阶段就需要关注),但 dataset 必须已发布或处于 reviewing(防止 watch 草稿)。这层校验放 `Service.Add` 里:`GetByID(datasetID).Status in {Reviewing, Published}` 否则 `ErrNotFound`(不暴露存在性)。

## L.3 dataset 模块的跨模块 hook

**改** `backend/internal/modules/dataset/review.go:30` `Review` 函数末尾:

```go
// 既有逻辑结尾后,在 status → published 时通知 watchers
if to == StatusPublished && s.watchersNotifier != nil {
    // 异步通知,不阻塞 ops review
    go s.watchersNotifier.NotifyDatasetPublished(
        context.Background(), d.ID, d.CurrentVersionID, d.Title,
    )
}
```

**改** `backend/internal/modules/dataset/service.go`,Service struct 加字段 + setter:

```go
type WatchersNotifier interface {
    NotifyDatasetPublished(ctx context.Context, datasetID, newVersionID, datasetTitle string)
}

// Service 加字段:
watchersNotifier WatchersNotifier

// Setter:
func (s *Service) SetWatchersNotifier(w WatchersNotifier) { s.watchersNotifier = w }
```

**特别注意**:dataset 模块**不许 import** watchlist 模块。watchlist.Service 实现这个本地接口即可(已经满足)。

## L.4 接线 `backend/internal/server/server.go`

```go
watchRepo := watchlist.NewRepository(s.db)
watchSvc  := watchlist.NewService(watchRepo, notifySvc)  // 复用 #75 已注册的 notifySvc
watchlist.Register(api, watchSvc, authMW)
dsSvc.SetWatchersNotifier(watchSvc)
```

## L.5 前端

### L.5.1 `frontend/app/datasets/[id]/page.tsx`

数据集详情页右上加 ⭐ 按钮:
- 加载时 GET /users/me/watched(或加 GET /datasets/:id/watch-status 端点查单个,**两选一**;轻量起见**首选**前端缓存全表后筛选)
- 已收藏:实心 ⭐ + "取消收藏"
- 未收藏:空心 ☆ + "收藏"
- Click 调 POST/DELETE,本地状态同步

### L.5.2 `frontend/app/account/page.tsx`

加 `<WatchlistCard />` 组件,在 FederatedComputePanel 之前:
- 列出用户收藏的数据集(标题 / 关注时间 / 链接到详情页)
- 空状态:「还没有关注的数据集」

### L.5.3 `frontend/app/notifications/page.tsx`

PR #75 已经有 `kind` 渲染,**追加** `dataset_updated` 的 badge 颜色 + 中英翻译:

```ts
case "dataset_updated":
  return t("数据集有更新", "Dataset updated");
```

### L.5.4 `frontend/lib/api.ts`

```ts
export type Watch = {
  dataset_id: string;
  dataset_title?: string;
  last_notified_version_id?: string;
  created_at: string;
};

// 加:
watchDataset:   (id: string) => request<{ ok: boolean }>(`/datasets/${id}/watch`, { method: "POST" }),
unwatchDataset: (id: string) => request<{ ok: boolean }>(`/datasets/${id}/watch`, { method: "DELETE" }),
listMyWatches: () => request<{ items: Watch[] }>("/users/me/watched"),
```

## L.6 测试(**必须** 9 个)

**新文件** `backend/internal/modules/watchlist/repo_test.go`(用 `db.RunMigrations`):

```go
func TestAdd_Idempotent(t *testing.T)
//   连续 2 次 Add(userA, dsX) → 不报错,只 1 行

func TestAdd_InitializesLastNotifiedToCurrentVersion(t *testing.T)
//   先 INSERT datasets (current_version_id=verA), 然后 Add → 查 dataset_watches 这行 last_notified_version_id == verA

func TestRemove_DeletesRow(t *testing.T)

func TestRemove_NonExistent_NoError(t *testing.T)
//   Remove(userX, dsX) 即使不存在也不报错(DELETE 影响 0 行不视为错误)

func TestListByUser_ReturnsOwnOnly(t *testing.T)
//   userA 加 2, userB 加 1 → ListByUser(A) 返回 2,内容全部 user_id=A

func TestListByDataset_ReturnsAllWatchers(t *testing.T)
//   3 个 user 关注同一 ds → ListByDataset 返回 3

func TestMarkNotified_UpdatesOnlyMatchingRow(t *testing.T)
//   userA 关注 ds1+ds2,MarkNotified(A, ds1, "new-ver") → 只 ds1 行更新, ds2 不动
```

**新文件** `backend/internal/modules/watchlist/service_test.go`(用 fake notifier + fake repo):

```go
func TestNotifyDatasetPublished_NotifiesAllWatchersAndUpdatesLastNotified(t *testing.T)
//   ListByDataset 返回 3 个 user,有 1 个 last_notified_version_id == "new-ver"(跳过)
//   fakeNotifier 应收到 2 次调用,2 次 MarkNotified

func TestNotifyDatasetPublished_NotifierErrorDoesNotBlockOthers(t *testing.T)
//   fakeNotifier 第一次返回 error → 后续 watcher 仍被通知
```

## L.7 IDOR / 安全审计点

| 检查 | 必须做 |
|---|---|
| `Remove`:`WHERE user_id=$1 AND dataset_id=$2` 双键 | ✅ |
| `MarkNotified`:同样双键 | ✅ |
| `ListByUser`:`WHERE user_id = $1`,不许泄露他人 | ✅ |
| `Add` 不能 watch 未发布数据集(防 enumeration) | ✅ |

## L.8 不许做

| ❌ | 原因 |
|---|---|
| 同步通知所有 watchers(可能 1000+) | 必须 `go svc.NotifyDatasetPublished(...)` 异步 |
| 给每个 watcher 发邮件/短信 | 只走 #75 通知模块 |
| 让 watchlist 模块写其他用户的通知偏好 | 只读 ListByDataset,只写 dataset_watches |
| import dataset 模块进 watchlist | dataset → watchlist 单向依赖(经接口) |

---

# Part 5 · 我审核会查的清单(逐 PR 自检)

每个 PR 提交前你自己跑一遍这张表,**全过**再 push:

```
通用(每个 PR 都查)
[ ] cat ~/ai-data-marketplace/CLAUDE.md 确认读了(PR description 写一句 "Read CLAUDE.md ✓")
[ ] cd backend && gofmt -l . && goimports -l .   (两个都为空)
[ ] cd backend && go vet ./...                   (无 warning)
[ ] cd backend && go build ./...
[ ] 真 PG: go test -race -p 1 -count=1 ./...     (回归全过 + 新测试全过)
[ ] cd frontend && npx tsc --noEmit              (0 错误)
[ ] cd frontend && npx next lint                 (0 warning 0 error)
[ ] cd frontend && npm run build                 (0 错误)
[ ] smart-quote 扫修改的 .tsx == 0(显示文本里的不计)
[ ] CLAUDE.md 末尾 commit 追加 ≥1 条 gotcha(写本次新学的真坑;commit 单独 docs:)
[ ] commits 序清晰:① test (RED) → ② backend (GREEN) → ③ frontend → ④ docs/claude

PR-J 专项
[ ] 错误分类:Permanent 哨兵清单 ≥2 (ErrObjectNotFound, ErrInvalidContent)
[ ] Backoff 表 0→30s / 1→60s / 2→120s(测试用 ±2s 容差断言)
[ ] EnqueueQualityRetry 用 ON CONFLICT (dataset_id) DO UPDATE
[ ] ListDueQualityRetries 排除 attempts >= max_attempts
[ ] upload.go 首次入队**先**持久化**再** in-memory enqueue
[ ] worker goroutine 永久错误 → 打回 draft + 删除 retry 行 + 通知
[ ] worker goroutine 瞬时错误 → MarkQualityRetryAttempt + 不改状态
[ ] qualityRetryLoop 10s tick,每轮拉 ≤10 条
[ ] 测试 10 个(4 分类 + 1 backoff + 5 repo)

PR-K 专项
[ ] BundleOrders 写第一个 zip 字节前**完整**预校验
[ ] 校验失败时 io.Writer 完全未被写入(测试断言 bytes.Buffer.Len()==0)
[ ] 中途 storage.Open 失败:返回 error,但 zw.Close() 仍被调用
[ ] order_ids 上限 20,超过 400
[ ] compute order 走 400 不是 500
[ ] 外人订单走 403 不是 500
[ ] 测试 8 个(全部 fakes,不需 PG)

PR-L 专项
[ ] Add 用 ON CONFLICT (user_id, dataset_id) DO NOTHING
[ ] Add 初始化 last_notified_version_id = 数据集当前 current_version_id
[ ] Remove/MarkNotified 双键 WHERE
[ ] ListByUser 严格 user_id 隔离(IDOR 测试断言)
[ ] dataset/review.go 通知 watchers 必须 go(异步)
[ ] dataset 模块**不** import watchlist
[ ] 测试 9 个(7 repo + 2 service)
```

---

# Part 6 · 不许做(跨 PR)

| ❌ | 原因 |
|---|---|
| 一次提 3 个方向到同一 PR | 我审不动;且回滚粒度太粗 |
| 跳过测试 | PR #75 你已被点名,这次会更严 |
| 改 `audit.Recorder` 接口 / 改 `Notifier` 接口 | 影响面太大,要做先单独 PR |
| 引入新 deps(yaml/cron/zip 第三方库) | 标准库够用 |
| 把 CLAUDE.md 更新塞进功能 commit | 必须单独 `docs(claude):` commit 在 PR 末尾 |

---

# Part 7 · 完成顺序(execute exactly)

```
1. cd ~/ai-data-marketplace && git fetch origin
2. git worktree add ~/ai-data-marketplace-J -b feat/quality-retry origin/main
3. 实现 PR-J + 全部验证清单 → push → gh pr create → 等我审 → 我说 merge 你才 merge
4. PR-J 合并 → git worktree remove ~/ai-data-marketplace-J
5. git worktree add ~/ai-data-marketplace-K -b feat/order-bundle origin/main
6. 实现 PR-K + 全部验证清单 → push → gh pr create → 等我审 → 我说 merge 你才 merge
7. PR-K 合并 → git worktree remove ~/ai-data-marketplace-K
8. git worktree add ~/ai-data-marketplace-L -b feat/dataset-watchlist origin/main
9. 实现 PR-L + 全部验证清单 → push → gh pr create → 等我审 → 我说 merge 你才 merge
10. PR-L 合并 → git worktree remove ~/ai-data-marketplace-L
```

**铁律**:每个 PR 必须**先**等我审过、点头才能 merge。不要自己 merge —— 即使 CI 绿。

---

# Part 8 · 每个 PR 的 description 模板

```markdown
## PR-X · <方向 J/K/L 名称>

### 改动文件
- backend/internal/modules/<...>/...
- backend/migrations/...
- frontend/...

### 新增端点
- ...

### 测试(N 个,全部 PASS)
- TestXxx (描述断言)
- ...

### Skills learned(本次新坑,已同步 CLAUDE.md)
- ...

### 自检清单(Part 5)
[逐项打勾,粘 Bash 输出片段]

### CI
backend / frontend / sidecar 全绿
```

---

**就这些。3 个 PR 顺序执行,完成 J 等我点头再做 K。**
