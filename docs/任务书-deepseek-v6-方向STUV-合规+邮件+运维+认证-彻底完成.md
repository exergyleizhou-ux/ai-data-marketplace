# 任务书 v6 给 DeepSeek V4 Pro — 方向 S+T+U+V(最终阶段,4 PR 顺序交付,**彻底完成项目**)

**基线**:`origin/main @ bd8791f`(PR #88 已合)
**审核人**:Claude Code(Opus 4.7)
**重要原则**:
- **这是项目最后阶段**。完成 S+T+U+V 之后,剩余只有 **硬件/牌照门控**(TEE 云 / Secretflow 节点 / 微信支付宝二清),那些本地完不成
- **4 PR 严格顺序**:S → T → U → V(V 的密码重置依赖 T 的 email 通道,顺序不可调)
- 每 PR 独立 worktree、独立审、独立合并
- **等我审过才 merge**,**永远不许自 merge**
- **你是纯文本模型(无多模态)**:所有 UI 描述用 state shape + behavior 表 + 组件骨架

---

## 0. 总览:**为什么是这 4 个方向**

### 0.1 已经做到的(本会话至 PR #88)

**12 个 DeepSeek PR + 我的 14 个前置 PR + 7 份任务书 + 3 份交接文档** = 完整的数据交易平台 + C2D 隐私计算栈 + ops 看板 + 通知/收藏/Q&A + 提现/异常告警

**核心模块**:auth / dataset / quality / order / payment / delivery / compute (L1+L2+L3) / search / notification / verify / auditlog / watchlist / qa / withdrawal / anomaly

### 0.2 这 4 个 PR 填的最后 4 个真缺口

| PR | 缺口 | 为何重要 | 难度 |
|---|---|---|---|
| **S** PIPL 合规 | 用户数据导出 + 账号注销没做 | **PIPL Article 45/47 强制要求**,中国市场不做这个不能上线 | 中(状态机 + zip 流式) |
| **T** 邮件通知 | 通知只在 app feed,用户不在线就漏了 | 真实用户期望邮件提醒,且 PR-V 密码重置要用 | 中(SMTP 接口 + 偏好表) |
| **U** 运维硬化 | 没 trace_id、anomaly 只存 DB 不告警 | 生产部署必需:慢请求定位、告警外溢 | 低-中(middleware + webhook) |
| **V** 2FA + 密码重置 | 认证不成熟,无 2FA、不能重置密码 | 安全基线,合规要求,**依赖 PR-T 的 email 通道** | 中(TOTP + 邮件链接) |

### 0.3 4 PR 之后还剩什么(诚实交接)

**硬件 / 外部资源门控**(本地无法完成):
- C2D 硬件半 (TEE 真证明 + KBS 真实部署) → 需 TDX/SEV 节点
- 真 Secretflow PSI → 需 ≥2 个 Secretflow 节点
- 真分账(微信/支付宝) → 二清牌照 + 法务

**未来可选(不紧迫)**:
- 语义搜索(pgvector / 外部向量 DB)
- 国际化(en/zh 已就绪,可加更多语言)
- OpenAPI/Swagger 文档自动生成
- 订阅/recurring 购买

**结论**:S+T+U+V 合完 = 本地能做的"彻底完成",剩下的等基础设施/法务。

---

## 1. 操作前必做(每次新 PR 都做)

```bash
cat ~/ai-data-marketplace/CLAUDE.md
git fetch origin && git log origin/main -3
ls backend/migrations/*.up.sql | sort -V | tail -3   # 确认下个迁移号
```

**铁律 checklist**(每次都过):
- [ ] `gofmt -w . && goimports -w .` 两个都跑
- [ ] `db.RunMigrations(dsn)` 不许裸 `CREATE TABLE`
- [ ] 新 `service.go`/`repo.go` 必须配 `_test.go`,**测试名跟我 spec 一字不改**
- [ ] 通知 emit:`if s.notifier != nil { _ = s.notifier.NotifyUser(...) }`
- [ ] `seedUser` 用 `crypto/rand` 后缀 + `ON CONFLICT DO UPDATE`(PR-N 模式)
- [ ] `.tsx` smart-quote 扫描 = 0
- [ ] CLAUDE.md gotcha 末尾单独 `docs(claude):` commit
- [ ] **commits 序**:① test → ② backend → ③ frontend → ④ docs(claude)
- [ ] **不许自 merge** 即使 CI 绿

**已记录的 23 条 gotchas 都还守着**(CLAUDE.md `## Conventions / gotchas` 节)。

---

## 2. PR 顺序与依赖

| PR | 方向 | 估算 | 测试数 | 依赖 |
|---|---|---|---|---|
| **PR-S** | PIPL 合规:数据导出 + 账号注销(state machine) | ~600 行 | 12 | 无 |
| **PR-T** | 邮件通知渠道 + 通知偏好(SMTP) | ~500 行 | 10 | 无 |
| **PR-U** | 运维硬化:`trace_id` + anomaly webhook 告警 | ~400 行 | 8 | 无 |
| **PR-V** | 2FA(TOTP)+ 密码重置(via email) | ~600 行 | 12 | **PR-T**(密码重置链接走 email) |

S/T/U 互不依赖,但**保持 S→T→U→V 顺序**(便于审核 + V 真依赖 T)。

---

## 3. 工作流模板(同 v5,沿用)

```bash
export PATH="$HOME/.local/bin:$HOME/sdk/node/bin:$HOME/sdk/pg/bin:$PATH"
git fetch origin
git worktree add ~/ai-data-marketplace-<name> -b feat/<name> origin/main

# 实现 + 验证

cd backend
gofmt -w . && goimports -w .
go vet ./...
go build ./...
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

# smart-quote 扫描
python3 -c "
for f in ['<改/新增的 .tsx 路径>']:
    d = open(f).read()
    bad = [c for c in d if c in '“”‘’']
    if bad: print(f, 'curly quotes:', len(bad))
"

git push -u origin feat/<name>
gh pr create --base main --title "..." --body "<按 Part 7 模板>"
gh pr checks <n> --watch
# 等我审 → 我说 merge 你才 merge
```

---

# PR-S · PIPL 合规:数据导出 + 账号注销

## S.0 现状(必读)

- `users` 表已有 `agreements` 记录 + KYC,**但**:
  - **没有数据导出接口**(PIPL Article 45 — 知情权/访问权要求)
  - **没有账号注销流程**(PIPL Article 47 — 删除权要求)
- 现在用户想要他们的数据只能 ops 手动捞 SQL → 不合规

**真实需求**:
- 用户能下载所有自己产生的数据(订单/通知/上传/收藏/Q&A 等)
- 用户能申请注销账号 → 7 天冷静期 → ops 审批 → 真正 scrub(保留审计 trail 但 PII 全删)

## S.1 数据模型

**新迁移** `backend/migrations/000020_compliance.up.sql`:

```sql
-- 000020: PIPL Article 45 (data export) + Article 47 (account deletion).

-- 数据导出任务表(异步生成,避免阻塞 HTTP)
CREATE TABLE IF NOT EXISTS data_export_jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id),
    status          TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'generating', 'ready', 'failed', 'expired')),
    object_key      TEXT,                       -- 生成的 zip 存储 key
    object_bytes    BIGINT,
    expires_at      TIMESTAMPTZ,                -- 24 小时后过期
    error           TEXT,
    requested_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    ready_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_data_export_user_recent
    ON data_export_jobs (user_id, requested_at DESC);

-- 账号注销申请表
CREATE TABLE IF NOT EXISTS account_deletion_requests (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id) UNIQUE,  -- 一个用户最多一个 active 申请
    reason            TEXT,
    status            TEXT NOT NULL DEFAULT 'cooling'             -- 提交即进入冷静期
                          CHECK (status IN ('cooling', 'approved', 'rejected', 'cancelled', 'deleted')),
    cooling_until     TIMESTAMPTZ NOT NULL,                       -- 申请时 + 7 days
    ops_note          TEXT,
    requested_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at      TIMESTAMPTZ,
    processed_by      UUID REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_account_deletion_pending
    ON account_deletion_requests (status, cooling_until)
    WHERE status IN ('cooling', 'approved');
```

**down**:`DROP TABLE IF EXISTS data_export_jobs, account_deletion_requests;`

## S.2 模块结构 `backend/internal/modules/compliance/`

```
compliance/
  model.go            (DTO + state + sentinels)
  export_repo.go      (data_export_jobs CRUD)
  deletion_repo.go    (account_deletion_requests CRUD)
  export_service.go   (CollectUserData + Service.RequestExport / GetExportStatus)
  deletion_service.go (Service.RequestDeletion / CancelDeletion / ApproveDeletion / RejectDeletion / ExecuteDeletion)
  scanner.go          (后台 goroutine: 1h tick, 处理过期 export + 检查冷静期满的待批准注销)
  handler.go          (Register + 7 handlers)
```

### S.2.1 DTO + 状态

```go
type ExportJob struct {
    ID           string `json:"id"`
    UserID       string `json:"user_id"`
    Status       string `json:"status"`
    DownloadURL  string `json:"download_url,omitempty"`  // 仅 status=ready 时填
    ObjectBytes  int64  `json:"object_bytes,omitempty"`
    ExpiresAt    string `json:"expires_at,omitempty"`
    Error        string `json:"error,omitempty"`
    RequestedAt  string `json:"requested_at"`
    ReadyAt      string `json:"ready_at,omitempty"`
}

type DeletionRequest struct {
    ID            string `json:"id"`
    UserID        string `json:"user_id"`
    Reason        string `json:"reason,omitempty"`
    Status        string `json:"status"`
    CoolingUntil  string `json:"cooling_until"`
    OpsNote       string `json:"ops_note,omitempty"`
    RequestedAt   string `json:"requested_at"`
    ProcessedAt   string `json:"processed_at,omitempty"`
    ProcessedBy   string `json:"processed_by,omitempty"`
}

const (
    ExportPending    = "pending"
    ExportGenerating = "generating"
    ExportReady      = "ready"
    ExportFailed     = "failed"
    ExportExpired    = "expired"

    DeletionCooling   = "cooling"
    DeletionApproved  = "approved"
    DeletionRejected  = "rejected"
    DeletionCancelled = "cancelled"
    DeletionDeleted   = "deleted"
)

var (
    ErrExportInProgress     = errors.New("a data export is already in progress")
    ErrExportNotReady       = errors.New("export not ready or expired")
    ErrDeletionExists       = errors.New("an active deletion request already exists")
    ErrDeletionNotCancelable = errors.New("deletion request is not in cooling state")
    ErrCoolingNotElapsed    = errors.New("cooling period has not elapsed")
    ErrNotFound             = errors.New("not found")
    ErrBadTransition        = errors.New("illegal status transition")
)
```

### S.2.2 数据导出 — `CollectUserData`(核心逻辑)

```go
// UserDataSnapshot 是要打包成 zip 的所有用户数据。每个字段对应 zip 内的一个 JSON 文件。
type UserDataSnapshot struct {
    User           map[string]any   `json:"user"`               // users 表 + agreements
    Orders         []map[string]any `json:"orders"`             // 买家 + 卖家订单
    Datasets       []map[string]any `json:"datasets"`           // 上传的数据集
    Notifications  []map[string]any `json:"notifications"`
    Watches        []map[string]any `json:"dataset_watches"`
    Questions      []map[string]any `json:"qa_questions"`
    Answers        []map[string]any `json:"qa_answers"`
    Reviews        []map[string]any `json:"reviews"`
    Withdrawals    []map[string]any `json:"withdrawals"`
    ComputeJobs    []map[string]any `json:"compute_jobs"`
    ExportedAt     string           `json:"exported_at"`        // RFC3339
}

// Source 是注入到 export service 的接口,避免直接 import 别的模块
type Source interface {
    UserRow(ctx context.Context, userID string) (map[string]any, error)
    Orders(ctx context.Context, userID string) ([]map[string]any, error)
    Datasets(ctx context.Context, userID string) ([]map[string]any, error)
    Notifications(ctx context.Context, userID string) ([]map[string]any, error)
    Watches(ctx context.Context, userID string) ([]map[string]any, error)
    Questions(ctx context.Context, userID string) ([]map[string]any, error)
    Answers(ctx context.Context, userID string) ([]map[string]any, error)
    Reviews(ctx context.Context, userID string) ([]map[string]any, error)
    Withdrawals(ctx context.Context, userID string) ([]map[string]any, error)
    ComputeJobs(ctx context.Context, userID string) ([]map[string]any, error)
}

// 在 server.go 中实现一个 complianceSourceAdapter 把各 service 转 Source 接口
// (沿用 watchlistDatasetAdapter / qaDatasetAdapter 同款反向注入)
```

### S.2.3 数据导出 — 流式 zip 生成

```go
// GenerateExportZip 流式生成 zip,写入 storage(避免内存爆)。
// 模式借鉴 PR-K 的 BundleStream:archive/zip + io.Copy + defer Close
func (s *ExportService) GenerateExportZip(ctx context.Context, jobID, userID string) error {
    // 1. 转入 generating
    _ = s.repo.SetStatus(ctx, jobID, ExportGenerating, "")

    // 2. 收集
    snap, err := s.collect(ctx, userID)
    if err != nil {
        _ = s.repo.SetFailed(ctx, jobID, err.Error())
        return err
    }

    // 3. 流式 zip → storage(用 io.Pipe + 后台 zip writer + storage.Put,或直接生成到 bytes.Buffer 如果 < 50MB)
    objectKey := fmt.Sprintf("exports/%s/%s.zip", userID, jobID)
    pr, pw := io.Pipe()
    go func() {
        defer pw.Close()
        zw := zip.NewWriter(pw)
        defer zw.Close()
        for filename, data := range snap.asFiles() {  // snap.asFiles 把 fields 转 filename→json bytes
            w, _ := zw.Create(filename)
            _, _ = w.Write(data)
        }
    }()
    size, err := s.store.Put(ctx, objectKey, pr, -1)  // 假设 storage.Put 支持流
    if err != nil {
        _ = s.repo.SetFailed(ctx, jobID, err.Error())
        return err
    }

    // 4. 转 ready,设 expires_at = now + 24h
    return s.repo.SetReady(ctx, jobID, objectKey, size, time.Now().Add(24*time.Hour))
}

func (s *UserDataSnapshot) asFiles() map[string][]byte {
    out := map[string][]byte{}
    out["user.json"], _ = json.MarshalIndent(s.User, "", "  ")
    out["orders.json"], _ = json.MarshalIndent(s.Orders, "", "  ")
    // ... 其余字段同理 ...
    out["exported_at.txt"] = []byte(s.ExportedAt)
    return out
}
```

**如果 `storage.Storage` 当前没有支持 `io.Reader` 的 `Put` 签名**,新增一个简单方法 `PutFromReader(ctx, key, r) (int64, error)`,或退而求其次:把整个 zip buf 到内存(用户数据通常 < 10MB,可接受)。**优先用流式**。

### S.2.4 账号注销状态机

```
                ┌─→ rejected (ops 拒)
                │
cooling (7d) ──┼─→ cancelled (用户撤销,仅 cooling 状态可撤)
                │
                └─→ approved (cooling 期满,ops 批) ──→ deleted (执行 scrub)
```

**乐观状态机** SQL `UPDATE … WHERE status = $from RETURNING …`。

### S.2.5 执行注销:**scrub 但保留审计 trail**

```go
// ExecuteDeletion 在 status=approved 后由 ops 触发,做真删除
// 原则:scrub PII 但保留法律必要的审计/财务记录
func (s *DeletionService) ExecuteDeletion(ctx context.Context, opsID, requestID string) error {
    req, err := s.repo.GetDeletion(ctx, requestID)
    if err != nil { return err }
    if req.Status != DeletionApproved {
        return ErrBadTransition
    }
    userID := req.UserID

    // 用 transaction 包起来
    err = s.exec(ctx, func(tx pgx.Tx) error {
        // 1. users 表:scrub PII 字段但保留 id + role(让 orders/audit FK 不挂)
        if _, err := tx.Exec(ctx, `
            UPDATE users SET account = 'deleted-' || id::text,
                             password_hash = 'deleted',
                             kyc_status = 'deleted',
                             kyc_data = '{}'::jsonb
            WHERE id = $1`, userID); err != nil { return err }

        // 2. notifications:全删
        if _, err := tx.Exec(ctx, `DELETE FROM notifications WHERE user_id::text = $1::text`, userID); err != nil { return err }

        // 3. dataset_watches:全删
        if _, err := tx.Exec(ctx, `DELETE FROM dataset_watches WHERE user_id::text = $1::text`, userID); err != nil { return err }

        // 4. dataset_questions / dataset_answers:scrub body 但保留 row(让公开 QA 链不断)
        if _, err := tx.Exec(ctx, `UPDATE dataset_questions SET body = '[已删除]' WHERE asker_id = $1`, userID); err != nil { return err }
        if _, err := tx.Exec(ctx, `UPDATE dataset_answers SET body = '[已删除]' WHERE answerer_id = $1`, userID); err != nil { return err }

        // 5. withdrawal_requests:保留(财务记录),仅 scrub account_label
        if _, err := tx.Exec(ctx, `UPDATE withdrawal_requests SET account_label = '[已删除]' WHERE seller_id = $1`, userID); err != nil { return err }

        // 6. 不动:orders / payments / audit_logs(法律必须留)

        return nil
    })
    if err != nil { return err }

    // 7. 转 status=deleted + 审计记录
    return s.repo.SetDeleted(ctx, requestID, opsID)
}
```

**保留**:`orders` / `payments` / `audit_logs`(中国《会计法》要求保留 ≥10 年)
**Scrub**:`users.kyc_data` / `notifications` / `watches` / Q&A body / withdrawal account_label
**全删**:无(数据保留是义务)

### S.2.6 Scanner — 后台 goroutine(1h tick)

```go
func (s *Service) StartScanner(ctx context.Context) {
    go func() {
        ticker := time.NewTicker(1 * time.Hour)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done(): return
            case <-ticker.C:
                // 1. 标记过期 export jobs(expires_at < now → status=expired)
                _ = s.exportRepo.ExpireOldJobs(ctx)
                // 2. 检查冷静期满的 cooling 注销 → 仍然 cooling 但 cooling_until < now
                //    这些就让 ops 看到(state 不变,前端在 status=cooling 且 cooling_until < now 时显示「可批准」)
                //    **不**自动 approve,必须 ops 手动
            }
        }
    }()
}
```

**铁律**(继承 PR-J / PR-Q):**不许**读 `s.qCh` 或其他模块的工作队列 channel!**只**用 ctx.Done + ticker.C。

### S.2.7 Handler + 路由

```
authed:
    POST   /users/me/data-export              # 发起导出(若 24h 内已有 pending/generating/ready 则返回那个)
    GET    /users/me/data-export              # 查最新导出状态
    GET    /users/me/data-export/download     # status=ready 时下载 zip(基于 storage.PresignedGet 或重定向)
    POST   /users/me/account/deletion         # body: {reason} 发起注销 → cooling
    DELETE /users/me/account/deletion         # 撤销 cooling 状态的注销

ops:
    GET    /admin/account-deletions?status=&limit=&offset=
    POST   /admin/account-deletions/:id/approve   # cooling_until 必须已过
    POST   /admin/account-deletions/:id/reject    # body: {reason}
    POST   /admin/account-deletions/:id/execute   # status=approved → 执行 scrub
```

## S.3 前端

### S.3.1 `frontend/app/account/page.tsx` 新增 `<DataRightsCard />`

**显示**:
- 「下载我的数据(PIPL Article 45)」按钮 → 调 `POST /users/me/data-export`
- 显示最近一次 export 状态 + 下载链接(若 ready)
- 「注销账号(PIPL Article 47)」按钮 → 弹 confirm + reason 输入 + 7 天冷静期说明
- 若已有 cooling 状态 → 显示倒计时 + 「撤销」按钮

### S.3.2 `frontend/app/admin/page.tsx` 新增第 9 Tab「注销审批」

| 列 | 内容 |
|---|---|
| 用户 | account 前 8 位脱敏 |
| 申请时间 | requested_at |
| 冷静期至 | cooling_until |
| 状态 | cooling / approved / rejected |
| 操作 | cooling + 已过期 → 「批准 / 拒绝」按钮;approved → 「执行删除」按钮 |

### S.3.3 `lib/api.ts`

```ts
export type DataExportJob = {
    id: string; user_id: string; status: string;
    download_url?: string; object_bytes?: number;
    expires_at?: string; error?: string;
    requested_at: string; ready_at?: string;
};
export type DeletionRequest = {
    id: string; user_id: string; reason?: string;
    status: string; cooling_until: string;
    ops_note?: string; requested_at: string;
    processed_at?: string; processed_by?: string;
};

requestDataExport: () => request<DataExportJob>("/users/me/data-export", { body: {} }),
getMyDataExport: () => request<DataExportJob>("/users/me/data-export"),
downloadMyDataExport: async () => { /* 同 PR-K bundle 模式 */ },

requestAccountDeletion: (reason: string) =>
    request<DeletionRequest>("/users/me/account/deletion", { body: { reason } }),
cancelAccountDeletion: () =>
    request<{ ok: boolean }>("/users/me/account/deletion", { method: "DELETE" }),

adminListDeletions: (status?: string, limit?: number, offset?: number) =>
    request<{ items: DeletionRequest[] }>("/admin/account-deletions", { query: { status, limit, offset } }),
adminApproveDeletion: (id: string, note?: string) =>
    request<DeletionRequest>(`/admin/account-deletions/${id}/approve`, { body: { note } }),
adminRejectDeletion: (id: string, reason: string) =>
    request<DeletionRequest>(`/admin/account-deletions/${id}/reject`, { body: { reason } }),
adminExecuteDeletion: (id: string, note?: string) =>
    request<DeletionRequest>(`/admin/account-deletions/${id}/execute`, { body: { note } }),
```

### S.3.4 通知 kind 翻译(`notifications/page.tsx`)

- `data_export_ready` → 「数据导出已就绪」/`Data export ready`
- `account_deletion_cooling` → 「账号注销冷静期已开始」/`Account deletion cooling period started`
- `account_deletion_approved` → 「账号注销已批准」/`Account deletion approved`
- `account_deletion_rejected` → 「账号注销被拒」/`Account deletion rejected`

## S.4 测试(**必须 12 个**,**测试名锁定**)

**`backend/internal/modules/compliance/export_repo_test.go`**(`db.RunMigrations`):

```go
func TestExportRepo_CreatesPendingJob(t *testing.T)
func TestExportRepo_SetReadyPopulatesObjectKeyAndExpiresAt(t *testing.T)
func TestExportRepo_ExpireOldJobs_MarksReadyJobsBeyondExpiresAt(t *testing.T)
//   插 status=ready, expires_at=now-1h → ExpireOldJobs → status=expired
func TestExportRepo_ExpireOldJobs_LeavesUnexpiredAlone(t *testing.T)
//   插 ready, expires_at=now+24h → ExpireOldJobs 不动它
```

**`backend/internal/modules/compliance/deletion_repo_test.go`**:

```go
func TestDeletionRepo_UniquePerUser(t *testing.T)
//   同 user 第二次 Create → 唯一约束错误 → repo 转 ErrDeletionExists
func TestDeletionRepo_Transition_CoolingToApproved(t *testing.T)
func TestDeletionRepo_Transition_CoolingToCancelled(t *testing.T)
func TestDeletionRepo_Transition_FromDeletedReturnsErrBadTransition(t *testing.T)
```

**`backend/internal/modules/compliance/deletion_service_test.go`**(fakes):

```go
func TestRequestDeletion_SetsCoolingUntilSevenDaysOut(t *testing.T)
//   时间窗口 ±5s 容差
func TestApproveDeletion_RejectsBeforeCoolingElapsed(t *testing.T)
//   cooling_until=now+24h, Approve → ErrCoolingNotElapsed
func TestExecuteDeletion_OnlyAcceptsApproved(t *testing.T)
//   status=cooling → ExecuteDeletion → ErrBadTransition
func TestExecuteDeletion_ScrubsPIIPreservesAuditTrail(t *testing.T)
//   先 seed 用户 + orders + notifications;Execute 后:
//     users.account 应是 'deleted-<id>'(scrub PII)
//     notifications 应空(全删)
//     orders 应保留(不删)
```

## S.5 我会查的

- [ ] 迁移 000020 双表 + 部分索引 `WHERE status IN ('cooling', 'approved')`
- [ ] `account_deletion_requests.user_id UNIQUE`(防止并发申请)
- [ ] `ExportService.GenerateExportZip` 用流式(io.Pipe 或 bytes.Buffer < 50MB)
- [ ] `ExecuteDeletion` 用 transaction 包(全成功或全回滚)
- [ ] **不删** orders / payments / audit_logs(财务/法律要求)
- [ ] scanner 不读工作队列 channel(继承 PR-J 教训)
- [ ] `ApproveDeletion` 检查 `cooling_until < now`,否则 `ErrCoolingNotElapsed`
- [ ] 用户 cancel 只能在 status=cooling 时(approved 后不能撤)
- [ ] 通知 emit:cooling 开始 / approved / rejected / export ready 4 种 kind
- [ ] CLAUDE.md 单独 commit + 追加 1 条 gotcha

## S.6 不许做

| ❌ | 原因 |
|---|---|
| 删 orders / audit_logs / payments | 中国《会计法》/合规审计要求保留 |
| 让用户跳过冷静期立刻删 | PIPL 推荐 7 天可撤回期 |
| 在 cancel 后让用户重新发起注销 | 应该可以,**允许**(同 user 历史可有多个 cancelled,UNIQUE 约束只挡 active 状态) |
| 数据导出不限速 | 加 ratelimit 每用户每 24h 最多 1 次 |

---

# PR-T · 邮件通知渠道 + 通知偏好

## T.0 现状(必读)

`backend/internal/modules/notification/service.go` 现在 `NotifyUser` 只写 DB。用户**没在线就漏了**。

## T.1 数据模型

**新迁移** `backend/migrations/000021_notification_email.up.sql`:

```sql
-- 000021: per-user notification preferences + email_send_log.
CREATE TABLE IF NOT EXISTS notification_preferences (
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind           TEXT NOT NULL,
    email_enabled  BOOLEAN NOT NULL DEFAULT true,
    in_app_enabled BOOLEAN NOT NULL DEFAULT true,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, kind)
);

-- 已发送邮件日志(防重 + 调试)
CREATE TABLE IF NOT EXISTS email_send_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id),
    kind            TEXT NOT NULL,
    to_address      TEXT NOT NULL,
    subject         TEXT NOT NULL,
    status          TEXT NOT NULL,    -- 'sent' | 'failed' | 'skipped'
    error           TEXT,
    sent_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    idempotency_key TEXT UNIQUE      -- {user_id}:{resource_type}:{resource_id}:{kind},防同事件重发
);

CREATE INDEX IF NOT EXISTS idx_email_send_log_user
    ON email_send_log (user_id, sent_at DESC);
```

## T.2 模块改造 `backend/internal/modules/notification/`

```
notification/  (现有)
  service.go         (改:Notifier 现在 emit 走 dispatcher)
  email.go           (新:EmailSender 接口 + SMTPSender 实现 + MockSender 测试用)
  preferences.go     (新:Repository 接口 + pg 实现)
  preferences_test.go (新)
  email_test.go      (新)
```

### T.2.1 EmailSender 接口

```go
// EmailSender 是 SMTP 抽象,允许测试用 mock。
type EmailSender interface {
    Send(ctx context.Context, to, subject, htmlBody, textBody string) error
}

// SMTPSender 用 net/smtp(标准库)实现
type SMTPSender struct {
    host, port, user, pass, fromAddr, fromName string
}

func NewSMTPSender(host, port, user, pass, fromAddr, fromName string) *SMTPSender {
    return &SMTPSender{host, port, user, pass, fromAddr, fromName}
}

func (s *SMTPSender) Send(ctx context.Context, to, subject, htmlBody, textBody string) error {
    // 用 net/smtp.SendMail,multipart/alternative 同发 text + html
    // 实现细节:从 "from" 字段 + Date + Subject + MIME multipart 拼字符串
}

// MockSender 给测试用
type MockSender struct {
    Sent []EmailLog
    Fail bool
}
type EmailLog struct { To, Subject, HtmlBody, TextBody string }
func (m *MockSender) Send(ctx context.Context, to, subject, html, text string) error {
    if m.Fail { return errors.New("mock smtp fail") }
    m.Sent = append(m.Sent, EmailLog{to, subject, html, text})
    return nil
}
```

### T.2.2 偏好 Repository

```go
type PreferencesRepository interface {
    GetForUser(ctx context.Context, userID string) (map[string]NotificationPreference, error)
    UpdateForUser(ctx context.Context, userID, kind string, emailEnabled, inAppEnabled bool) error
}

type NotificationPreference struct {
    UserID         string `json:"-"`
    Kind           string `json:"kind"`
    EmailEnabled   bool   `json:"email_enabled"`
    InAppEnabled   bool   `json:"in_app_enabled"`
}

// Default(用户没显式设过):所有 kind email + in_app 都 enabled
```

### T.2.3 Service.NotifyUser 改造

```go
type Service struct {
    repo     Repository
    prefs    PreferencesRepository
    email    EmailSender               // 可为 nil (测试 / 未配置 SMTP)
    emailLog EmailLogRepository
    userLookup UserLookup              // 查 user.account 拿 email 地址
}

type UserLookup interface {
    EmailOf(ctx context.Context, userID string) (string, error)  // 返回 user.account(就是 email)
}

func (s *Service) NotifyUser(ctx context.Context, userID, kind, title, body, resourceType, resourceID string) error {
    // 1. 查偏好(默认全 enabled)
    prefs, _ := s.prefs.GetForUser(ctx, userID)
    pref, has := prefs[kind]
    if !has { pref = NotificationPreference{InAppEnabled: true, EmailEnabled: true} }

    // 2. In-app:写 DB(已有逻辑)
    if pref.InAppEnabled {
        _, err := s.repo.Create(ctx, Notification{
            UserID: userID, Kind: kind, Title: title, Body: body,
            ResourceType: resourceType, ResourceID: resourceID,
        })
        if err != nil { slog.Warn("in-app notify failed", "err", err) }
    }

    // 3. Email:异步发(不阻塞 caller)
    if pref.EmailEnabled && s.email != nil {
        idemKey := fmt.Sprintf("%s:%s:%s:%s", userID, resourceType, resourceID, kind)
        go s.sendEmailWithLog(context.Background(), userID, kind, title, body, idemKey)
    }
    return nil
}

func (s *Service) sendEmailWithLog(ctx context.Context, userID, kind, title, body, idemKey string) {
    // 1. 查 idempotency_key 是否已发过 — 防重
    if exists, _ := s.emailLog.HasKey(ctx, idemKey); exists { return }

    addr, err := s.userLookup.EmailOf(ctx, userID)
    if err != nil || addr == "" {
        _ = s.emailLog.Log(ctx, userID, kind, addr, title, "skipped", "no email", idemKey)
        return
    }
    htmlBody := buildHTML(title, body)  // 简单模板
    textBody := title + "\n\n" + body
    if err := s.email.Send(ctx, addr, "[绿洲] "+title, htmlBody, textBody); err != nil {
        _ = s.emailLog.Log(ctx, userID, kind, addr, title, "failed", err.Error(), idemKey)
        return
    }
    _ = s.emailLog.Log(ctx, userID, kind, addr, title, "sent", "", idemKey)
}
```

### T.2.4 端点

```
authed:
    GET  /users/me/notification-preferences   → 当前用户的所有 kind preferences
    PUT  /users/me/notification-preferences   → body: {kind, email_enabled, in_app_enabled}
```

### T.2.5 Server.go 接线

```go
// 环境变量:
//   SMTP_HOST / SMTP_PORT / SMTP_USER / SMTP_PASS / SMTP_FROM_ADDR / SMTP_FROM_NAME
// 若 SMTP_HOST 为空 → 不启用 email(notifier.email = nil)

var emailSender notification.EmailSender
if host := os.Getenv("SMTP_HOST"); host != "" {
    emailSender = notification.NewSMTPSender(host, os.Getenv("SMTP_PORT"), ...)
    slog.Info("notification email channel enabled", "smtp", host)
}

prefsRepo := notification.NewPreferencesRepository(s.db)
emailLogRepo := notification.NewEmailLogRepository(s.db)
notifySvc := notification.NewServiceWithChannels(
    notification.NewRepository(s.db),
    prefsRepo,
    emailSender,
    emailLogRepo,
    notificationUserLookup{auth: authSvc},  // 适配器
)
```

## T.3 前端

### T.3.1 `frontend/app/account/page.tsx` 新增 `<NotificationPreferencesCard />`

| kind | 中文 | email | in-app |
|---|---|---|---|
| order_paid | 订单已支付 | ☑️ | ☑️ |
| order_settled | 订单结算 | ☑️ | ☑️ |
| ... | ... | ... | ... |

(checkboxes,onChange 调 PUT)

### T.3.2 `lib/api.ts`

```ts
export type NotificationPreference = {
    kind: string;
    email_enabled: boolean;
    in_app_enabled: boolean;
};

getNotificationPreferences: () =>
    request<{ items: NotificationPreference[] }>("/users/me/notification-preferences"),
updateNotificationPreference: (kind: string, email: boolean, inApp: boolean) =>
    request<NotificationPreference>("/users/me/notification-preferences", {
        method: "PUT",
        body: { kind, email_enabled: email, in_app_enabled: inApp },
    }),
```

## T.4 测试(**必须 10 个**)

**`backend/internal/modules/notification/email_test.go`**(MockSender):

```go
func TestMockSender_RecordsSentEmails(t *testing.T)
func TestSMTPSender_BuildMessageMultipart(t *testing.T)
//   测 multipart/alternative MIME 拼装,不真发 SMTP
```

**`backend/internal/modules/notification/preferences_repo_test.go`**(`db.RunMigrations`):

```go
func TestPreferences_DefaultsToAllEnabled(t *testing.T)
//   用户没记录 → GetForUser 返回空 map,服务层默认 enabled

func TestPreferences_UpdateUpsertsByKindAndUser(t *testing.T)
//   两次 update 同 (user, kind) → 1 行

func TestPreferences_PerUserIsolation(t *testing.T)
//   userA disable email,userB 不受影响
```

**`backend/internal/modules/notification/service_email_test.go`**(MockSender + fake repos):

```go
func TestNotifyUser_RespectsEmailDisabledPref(t *testing.T)
//   pref.EmailEnabled=false → MockSender.Sent 长度=0

func TestNotifyUser_RespectsInAppDisabledPref(t *testing.T)
//   pref.InAppEnabled=false → repo.Create 未被调

func TestNotifyUser_SkipsEmailWhenUserHasNoAddress(t *testing.T)
//   UserLookup.EmailOf 返回 "" → 日志 status=skipped, MockSender.Sent 长度=0

func TestNotifyUser_PreventsDoubleEmailViaIdempotencyKey(t *testing.T)
//   连续 2 次 NotifyUser 同 (user, kind, resource_id) → MockSender.Sent 长度=1

func TestNotifyUser_SMTPFailureLogsButDoesNotPanic(t *testing.T)
//   MockSender.Fail=true → email_send_log status=failed, NotifyUser 不返回错误
```

## T.5 我会查的

- [ ] `NotifyUser` email 发送**异步**(`go s.sendEmailWithLog(...)`),不阻塞 caller
- [ ] `sendEmailWithLog` 查 `idempotency_key` 防重
- [ ] `EmailSender` 接口允许 nil(SMTP_HOST 未设)
- [ ] 偏好默认全 enabled(用户没显式 opt-out)
- [ ] `email_send_log` 记录所有尝试(sent / failed / skipped),便于排查
- [ ] CLAUDE.md 追加 1 条 gotcha(本次新坑)

## T.6 不许做

| ❌ | 原因 |
|---|---|
| 引入第三方 mail 库(gomail / sendgrid SDK) | `net/smtp` 标准库 + multipart 拼装够用 |
| 让 email 发送阻塞 NotifyUser | 必须 `go` 异步 |
| 不记 idempotency_key | 重复发邮件骚扰用户 |
| 让 email 用户改地址(改 account) | 出 PR-V 配套 |

---

# PR-U · 运维硬化 — trace_id + anomaly webhook 告警

## U.0 现状(必读)

- `/healthz` 和 `/readyz` **已存在**(`server.go:198-199`),不要重复造
- Prometheus metrics、slog 结构化、audit/anomaly 都有
- **真缺口**:
  - 日志没 trace_id,跨服务调用难关联(单服务也难追踪一个请求经过的代码路径)
  - PR-Q anomaly 只存 DB,**没**主动告警 — ops 必须每天上 admin 看,**太被动**

## U.1 trace_id 中间件

**新文件** `backend/internal/platform/middleware/trace.go`:

```go
package middleware

import (
    "context"
    "crypto/rand"
    "encoding/hex"

    "github.com/gin-gonic/gin"
)

const TraceIDHeader = "X-Trace-ID"

type traceIDKey struct{}

// TraceID 中间件:从 header 读 trace_id,没有则生成 16 字节 hex。注入 ctx + 响应 header。
// slog 配套的 handler 会从 ctx 提取 trace_id 自动放进日志。
func TraceID() gin.HandlerFunc {
    return func(c *gin.Context) {
        tid := c.GetHeader(TraceIDHeader)
        if tid == "" {
            b := make([]byte, 16)
            _, _ = rand.Read(b)
            tid = hex.EncodeToString(b)
        }
        c.Header(TraceIDHeader, tid)
        ctx := context.WithValue(c.Request.Context(), traceIDKey{}, tid)
        c.Request = c.Request.WithContext(ctx)
        c.Set("trace_id", tid)
        c.Next()
    }
}

// TraceIDFromContext 让其他代码(service/repo)能拿到 trace_id 放进 slog。
func TraceIDFromContext(ctx context.Context) string {
    if v := ctx.Value(traceIDKey{}); v != nil {
        if s, ok := v.(string); ok { return s }
    }
    return ""
}
```

**集成 slog**:在 `server.go` setup slog handler 时,包一个会从 ctx 取 trace_id 的 wrapper(可用 `slog.Handler` 的自定义实现,或就在每个有 ctx 的 log 点手动 `slog.InfoContext(ctx, ..., "trace_id", TraceIDFromContext(ctx))`)。

**简化策略**:不改 slog 全局 handler(改动太大),在中间件之后,把 trace_id 也写到 c.Set("trace_id", tid),让既有 logger 知道这个 key 即可。

**中间件挂载**:server.go `s.engine.Use(...)` 列表里,**第一个** middleware(在 RecoveryWithSlog / RateLimit 之前)。

## U.2 anomaly webhook 告警

### U.2.1 配置

**环境变量**:
- `ANOMALY_WEBHOOK_URL` — 接收 POST 的 URL(空则禁用)
- `ANOMALY_WEBHOOK_KINDS` — 逗号分隔的 kind 白名单,默认 `high_risk_action,repeated_failure`(bulk_modification 通常噪音大,不默认告警)

### U.2.2 Webhook payload

```json
POST {ANOMALY_WEBHOOK_URL}
Content-Type: application/json

{
  "kind": "high_risk_action",
  "actor_id": "...",
  "resource_pattern": "dataset.reject",
  "count": 1,
  "first_seen_at": "...",
  "last_seen_at": "...",
  "sample_audit_ids": [123, 456],
  "anomaly_id": "...",
  "view_url": "https://platform/admin?tab=anomaly#anomaly-..."
}
```

### U.2.3 实现 — anomaly 模块新增 Alerter

```go
// backend/internal/modules/anomaly/alerter.go (新)

type Alerter interface {
    Alert(ctx context.Context, a Anomaly) error
}

type WebhookAlerter struct {
    url   string
    kinds map[string]bool   // 启用的 kind 集合
    client *http.Client
}

func NewWebhookAlerter(url string, kinds []string) Alerter {
    m := make(map[string]bool, len(kinds))
    for _, k := range kinds { m[k] = true }
    return &WebhookAlerter{
        url: url, kinds: m,
        client: &http.Client{Timeout: 5 * time.Second},
    }
}

func (a *WebhookAlerter) Alert(ctx context.Context, an Anomaly) error {
    if !a.kinds[an.Kind] { return nil }  // kind 不在白名单,不发
    payload := map[string]any{
        "kind": an.Kind, "actor_id": an.ActorID,
        "resource_pattern": an.ResourcePattern, "count": an.Count,
        "first_seen_at": an.FirstSeenAt, "last_seen_at": an.LastSeenAt,
        "sample_audit_ids": an.SampleAuditIDs,
        "anomaly_id": an.ID,
        "view_url": "",  // 由 server.go 填补 ANOMALY_VIEW_URL_PREFIX
    }
    body, _ := json.Marshal(payload)
    req, _ := http.NewRequestWithContext(ctx, "POST", a.url, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    resp, err := a.client.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode >= 500 {  // 4xx 可能是 webhook 端配错,不重试;5xx 才报错
        return fmt.Errorf("webhook returned %d", resp.StatusCode)
    }
    return nil
}

type NopAlerter struct{}
func (NopAlerter) Alert(ctx context.Context, a Anomaly) error { return nil }
```

### U.2.4 Service 改造

```go
// Service 增加 alerter 字段
type Service struct {
    repo     Repository
    rules    []Rule
    db       DBQuerier
    alerter  Alerter   // 新
}

func NewService(repo Repository, db DBQuerier, alerter Alerter) *Service {
    if alerter == nil { alerter = NopAlerter{} }
    return &Service{repo, []Rule{...}, db, alerter}
}

// ScanOnce 修改:每 upsert 一条**新**异常,触发一次 Alert(对**已存在被更新**的不重复告警)
func (s *Service) ScanOnce(ctx context.Context) (int, error) {
    // ... 现有逻辑 ...
    for _, a := range anomalies {
        isNew, err := s.repo.UpsertReturningIsNew(ctx, a)  // 改 repo 接口:返回是否新建
        if err != nil { ... }
        if isNew {
            if err := s.alerter.Alert(ctx, a); err != nil {
                slog.Warn("anomaly alert failed", "kind", a.Kind, "err", err)
            }
        }
        total++
    }
    return total, nil
}
```

**关键**:`Alerter.Alert` 不能阻塞 scanner 主线程太久(已经 5s timeout)。失败只记日志。

## U.3 前端

**`frontend/app/admin/page.tsx`** 异常告警 Tab(PR-Q 加的)显示 trace_id(若 anomaly detail 含触发的 audit_log,展开时显示该 audit_log 的 trace_id 字段 — 但 trace_id 是新加的,**老数据可能没**)。**不强求展示**,本 PR 主要是后端。

## U.4 测试(**必须 8 个**)

**`backend/internal/platform/middleware/trace_test.go`**(新):

```go
func TestTraceID_GeneratesWhenMissing(t *testing.T)
//   请求无 X-Trace-ID → 响应 header 含 16 字节 hex
func TestTraceID_PassesThroughWhenPresent(t *testing.T)
//   请求 X-Trace-ID: my-id → 响应 X-Trace-ID: my-id
func TestTraceID_PutsIntoContext(t *testing.T)
//   后续 handler 用 TraceIDFromContext 能取到
```

**`backend/internal/modules/anomaly/alerter_test.go`**(httptest):

```go
func TestWebhookAlerter_PostsPayloadOnAlert(t *testing.T)
//   起 httptest server,Alert(anomaly) → 收到 POST + payload 含 kind/count
func TestWebhookAlerter_SkipsKindNotInWhitelist(t *testing.T)
//   kinds=["high_risk_action"], Alert(kind="bulk_modification") → httptest 0 收到
func TestWebhookAlerter_5xxReturnsError(t *testing.T)
//   httptest 返回 500 → Alert 返回 error
func TestNopAlerter_AlwaysNil(t *testing.T)
```

**`backend/internal/modules/anomaly/service_alerter_test.go`**:

```go
func TestScanOnce_AlertsOnNewAnomalyOnly(t *testing.T)
//   1 条新 anomaly + 1 条更新已有 → fakeAlerter 调用 1 次(不是 2)
```

## U.5 我会查的

- [ ] middleware/trace.go 用 crypto/rand 16 字节(不许 time/rand)
- [ ] 中间件挂在 engine.Use 列表**最早**(在 recovery / ratelimit / metrics 之前)
- [ ] `c.Header(TraceIDHeader, tid)` 真写响应头
- [ ] WebhookAlerter 用 5s timeout(防 hang)
- [ ] **WebhookAlerter 4xx 不算失败**(目标方配错不该让我们整个 scanner 失败)
- [ ] **WebhookAlerter 5xx 算失败**(对方暂时故障,记日志即可)
- [ ] `UpsertReturningIsNew` 准确(已有=false,新建=true)
- [ ] CLAUDE.md 追加 1 条 gotcha

## U.6 不许做

| ❌ | 原因 |
|---|---|
| 改 slog 全局 handler 复杂逻辑 | 用 c.Set("trace_id") + 简单 wrapper 够 |
| 给 trace_id 加 retry on alert failure | 网络层失败靠 webhook 端自重试机制(or 让 anomaly 留在 DB,ops 兜底看) |
| 把 anomaly DB 也加 trace_id 列 | 不向下游传 trace_id,本 PR 只前端→后端 propagate |
| 同步 send 慢 | 必须 5s timeout |

---

# PR-V · 2FA(TOTP)+ 密码重置 — **依赖 PR-T email**

## V.0 现状(必读)

- `auth/router.go`:`/auth/register` `/auth/login` `/auth/refresh` `/auth/logout` 已有
- **缺**:`/auth/2fa/*` `/auth/password-reset/*`

## V.1 数据模型

**新迁移** `backend/migrations/000022_auth_2fa_pwd_reset.up.sql`:

```sql
-- 000022: 2FA TOTP + password reset.

-- TOTP 秘密(加密存储 — 用 platform/crypto 的 EncryptedString 模式 / 或先明文+TODO 加密)
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret TEXT;         -- base32,enroll 后填
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_enabled BOOLEAN NOT NULL DEFAULT false;

-- 恢复码(hash 存储)
CREATE TABLE IF NOT EXISTS totp_recovery_codes (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash  TEXT NOT NULL,        -- bcrypt of the recovery code
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, code_hash)
);

-- 密码重置 token
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    token_hash  TEXT PRIMARY KEY,     -- sha256 of the random token
    user_id     UUID NOT NULL REFERENCES users(id),
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_password_reset_user
    ON password_reset_tokens (user_id, created_at DESC);
```

## V.2 2FA TOTP 实现

**用库**:`github.com/pquerna/otp/totp` — 标准 TOTP(RFC 6238),最广泛使用的 Go OTP 库。如果 go.mod 没,加上(`go get github.com/pquerna/otp/totp`)。

### V.2.1 Service.Enroll2FA

```go
// Enroll2FA 生成新 secret + 返回 otpauth URL(扫描器扫码)+ 8 个 recovery codes。
// 此时 totp_enabled=false,等用户输入第一个有效 code 才 Enable。
func (s *Service) Enroll2FA(ctx context.Context, userID string) (Enroll2FAResult, error) {
    u, err := s.repo.GetByID(ctx, userID)
    if err != nil { return Enroll2FAResult{}, err }
    if u.TOTPEnabled {
        return Enroll2FAResult{}, ErrAlready2FAEnabled
    }
    key, err := totp.Generate(totp.GenerateOpts{
        Issuer:      "Verdant Oasis",
        AccountName: u.Account,
    })
    if err != nil { return Enroll2FAResult{}, err }

    // 写 secret(尚未 enable)
    _ = s.repo.SetTOTPSecret(ctx, userID, key.Secret())

    // 生成 8 个 recovery codes(每个 10 字符 base32),hash 存
    codes := make([]string, 8)
    for i := range codes {
        codes[i] = generateRecoveryCode()  // crypto/rand 10 字符
        h, _ := bcrypt.GenerateFromPassword([]byte(codes[i]), bcrypt.DefaultCost)
        _ = s.repo.AddRecoveryCode(ctx, userID, string(h))
    }
    return Enroll2FAResult{
        OTPAuthURL:    key.URL(),       // otpauth://totp/...
        RecoveryCodes: codes,           // 仅这一次返回明文
    }, nil
}
```

### V.2.2 Service.Verify2FAEnrollment

```go
func (s *Service) Verify2FAEnrollment(ctx context.Context, userID, code string) error {
    u, _ := s.repo.GetByID(ctx, userID)
    if u.TOTPSecret == "" { return ErrNot2FAEnrolled }
    if u.TOTPEnabled { return ErrAlready2FAEnabled }
    if !totp.Validate(code, u.TOTPSecret) {
        return ErrInvalid2FACode
    }
    return s.repo.EnableTOTP(ctx, userID)
}
```

### V.2.3 登录改造

```go
// 现有 Login 返回 access/refresh
// 改:若 user.totp_enabled,**不**直接发 access,返回一个 2fa_challenge_token
//     用户带 challenge token + TOTP code 调 /auth/2fa/verify 才换 access

func (s *Service) Login(ctx context.Context, account, password string) (LoginResult, error) {
    u, err := s.repo.GetByAccount(ctx, account)
    if err != nil || bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(password)) != nil {
        return LoginResult{}, ErrInvalidCredentials
    }
    if u.TOTPEnabled {
        challenge := s.tokenMgr.Issue2FAChallenge(u.ID, 5*time.Minute)
        return LoginResult{Need2FA: true, ChallengeToken: challenge}, nil
    }
    // 老路径:发 access + refresh
    tokens := s.tokenMgr.IssueTokens(u)
    return LoginResult{Tokens: &tokens, User: u}, nil
}

func (s *Service) Verify2FAAndIssueTokens(ctx context.Context, challengeToken, code string) (Tokens, User, error) {
    userID, err := s.tokenMgr.ValidateChallenge(challengeToken)
    if err != nil { return Tokens{}, User{}, ErrInvalidCredentials }
    u, _ := s.repo.GetByID(ctx, userID)
    // 先 TOTP code,失败再 recovery code
    if totp.Validate(code, u.TOTPSecret) {
        return s.issueAndReturn(u)
    }
    if s.consumeRecoveryCode(ctx, userID, code) {  // bcrypt match + mark used
        return s.issueAndReturn(u)
    }
    return Tokens{}, User{}, ErrInvalid2FACode
}
```

### V.2.4 Disable2FA

需要当前 TOTP code 验证才能关闭(防 session 劫持后关 2FA)。

## V.3 密码重置

### V.3.1 Service.RequestPasswordReset

```go
func (s *Service) RequestPasswordReset(ctx context.Context, account string) error {
    u, err := s.repo.GetByAccount(ctx, account)
    if err != nil {
        // **关键**:不暴露存在性,无论是否存在都返回 nil(攻击者不能枚举账号)
        return nil
    }
    // 生成 32 字节 token,hash 后存,明文走 email
    rawToken := generateSecureToken(32)
    tokenHash := sha256Hex(rawToken)
    _ = s.repo.CreatePasswordResetToken(ctx, tokenHash, u.ID, time.Now().Add(1*time.Hour))

    // 发 email(via PR-T 通道)
    if s.notifier != nil {
        _ = s.notifier.NotifyUser(ctx, u.ID, "password_reset_requested",
            "密码重置请求", "点击链接重置密码: "+s.appBaseURL+"/auth/reset?t="+rawToken,
            "user", u.ID)
    }
    return nil
}
```

### V.3.2 Service.CompletePasswordReset

```go
func (s *Service) CompletePasswordReset(ctx context.Context, rawToken, newPassword string) error {
    tokenHash := sha256Hex(rawToken)
    t, err := s.repo.GetPasswordResetToken(ctx, tokenHash)
    if err != nil || t.UsedAt != nil || time.Now().After(t.ExpiresAt) {
        return ErrTokenInvalidOrExpired
    }
    if len(newPassword) < 8 { return ErrPasswordTooWeak }

    hash, _ := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
    if err := s.repo.UpdatePassword(ctx, t.UserID, string(hash)); err != nil { return err }
    _ = s.repo.MarkPasswordResetTokenUsed(ctx, tokenHash)

    // 撤销所有 refresh token(强制重新登录)
    _ = s.repo.RevokeAllRefreshTokens(ctx, t.UserID)

    return nil
}
```

## V.4 端点

```
authed:
    POST   /auth/2fa/enroll                # 返回 {otpauth_url, recovery_codes[]}
    POST   /auth/2fa/verify-enrollment      # body: {code} → 启用 totp_enabled
    POST   /auth/2fa/disable                # body: {code}

公开(无 auth):
    POST   /auth/login                     # 改造:若 totp_enabled,返回 {need_2fa: true, challenge_token}
    POST   /auth/2fa/verify                 # body: {challenge_token, code} → 真换 access/refresh
    POST   /auth/password-reset/request     # body: {account} → 总是 200
    POST   /auth/password-reset/complete    # body: {token, new_password}
```

## V.5 前端

**改造 `/login` 页**:
1. POST /login 后,若 `response.need_2fa == true` → 显示 6 位 TOTP 输入框 + recovery code 链接
2. 提交 → POST /auth/2fa/verify
3. 成功 → 拿 tokens 进入产品

**新 `/auth/reset` 页**:从 URL `?t=token` 取,显示 new password + confirm。提交 → POST /auth/password-reset/complete。

**`/account` 加 `<TwoFactorSetupCard />`**:enroll → 显示 QR(用纯 base64 SVG QR generator,或直接显示 secret 让用户手输到 app)+ recovery codes。

**简化方案**:不画 QR,直接显示 `otpauth_url` + secret 文本,让用户手输或长按复制到 Google Authenticator。**节省一个 deps**。

## V.6 测试(**必须 12 个**)

**`backend/internal/modules/auth/twofactor_service_test.go`**(用 fakes):

```go
func TestEnroll2FA_GeneratesSecretAndRecoveryCodes(t *testing.T)
func TestEnroll2FA_RejectsWhenAlreadyEnabled(t *testing.T)
func TestVerify2FAEnrollment_RejectsInvalidCode(t *testing.T)
func TestVerify2FAEnrollment_EnablesOnValidCode(t *testing.T)
func TestLogin_With2FAEnabled_ReturnsChallenge(t *testing.T)
//   user.TOTPEnabled=true → Login 返回 Need2FA=true,no tokens
func TestVerify2FA_AcceptsRecoveryCodeWhenTOTPFails(t *testing.T)
func TestVerify2FA_RecoveryCodeUsedOnce(t *testing.T)
//   recovery code 用一次 → 第二次失败
func TestDisable2FA_RequiresCurrentCode(t *testing.T)
```

**`backend/internal/modules/auth/password_reset_service_test.go`**(用 fakes + MockSender):

```go
func TestRequestPasswordReset_DoesNotRevealUserExistence(t *testing.T)
//   不存在的 account → 仍返回 nil,但 fakeNotifier.calls 长度=0
func TestRequestPasswordReset_SendsEmailWithToken(t *testing.T)
func TestCompletePasswordReset_RejectsExpiredToken(t *testing.T)
func TestCompletePasswordReset_UpdatesPasswordAndRevokesRefresh(t *testing.T)
```

## V.7 我会查的

- [ ] `pquerna/otp/totp` 加进 go.mod
- [ ] enroll 返回的 recovery codes 是**明文**(只这一次)
- [ ] recovery code 用 bcrypt hash 存
- [ ] enroll 后 totp_enabled=false,**必须**verify-enrollment 才 enable
- [ ] Disable 需要当前 TOTP code(防 session 劫持)
- [ ] Login 改造:若 totp_enabled → 不直接发 access,返回 challenge
- [ ] challenge token 短 TTL(5 min)
- [ ] RequestPasswordReset **不暴露**用户存在性(不存在仍 200 OK + 空通知)
- [ ] password_reset_tokens 存 hash(sha256)而非明文
- [ ] token 1 小时过期、单次使用、used_at 标记
- [ ] Complete 后撤销所有 refresh token
- [ ] CLAUDE.md 追加 1 条 gotcha

## V.8 不许做

| ❌ | 原因 |
|---|---|
| SMS 2FA | TOTP 标准,SMS 不安全(SIM swap) |
| 直接信任 login 后立刻 disable 2FA | 必须验证当前 TOTP code |
| password reset email 直接发明文 token + 不 hash 存 | 数据库泄露 = 全用户密码可改 |
| 长 TTL 的 reset token(>1h) | 增加窗口期 |
| 让用户自定义 recovery code | 用户记不住,系统随机 8 个就好 |

---

# Part X · 跨 PR 通用 — 我审核会查的清单

```
通用
[ ] 4 commits 序:① test → ② backend → ③ frontend → ④ docs(claude)
[ ] gofmt -l . && goimports -l . 两个都空
[ ] go vet ./...
[ ] 真 PG: go test -race -p 1 -count=1 ./... 全过(无 skip)
[ ] 前端 tsc/lint/build 全过
[ ] smart-quote 扫修改 .tsx == 0
[ ] CLAUDE.md 末尾 commit 追加 ≥1 条本次实战 gotcha
[ ] PR description 自检表打勾

每 PR 专项:见各 PR 的 X.5

严禁
[ ] 改 spec 锁定测试名
[ ] 自 merge(即使 CI 绿)
[ ] 用 t.Skip 当防御
[ ] 把 spec 没要求的"优化"塞进 PR
```

---

# Part Y · 执行顺序

```
1. cat ~/ai-data-marketplace/docs/任务书-deepseek-v6-方向STUV-合规+邮件+运维+认证-彻底完成.md
2. git fetch origin && git log origin/main -1
3. git worktree add ~/ai-data-marketplace-S -b feat/pipl-compliance origin/main
4. 实现 PR-S + 自检 → push → 等我审 → 我说 merge 你才 merge
5. PR-S 合并 → 清 worktree

6. git worktree add ~/ai-data-marketplace-T -b feat/email-notifications origin/main
7. 实现 PR-T → push → 审 → merge
8. PR-T 合并 → 清 worktree

9. git worktree add ~/ai-data-marketplace-U -b feat/trace-and-anomaly-alert origin/main
10. 实现 PR-U → push → 审 → merge
11. PR-U 合并 → 清 worktree

12. git worktree add ~/ai-data-marketplace-V -b feat/2fa-and-password-reset origin/main
13. 实现 PR-V → push → 审 → merge(这是项目本地能做完的最后一刀)
14. PR-V 合并 → 清 worktree
```

---

# Part Z · 每个 PR 的 description 模板

```markdown
## PR-X · <方向名>

### 改动文件
- ...

### 新增端点 / 数据模型
- ...

### 测试(N 个,逐条 PASS)
- TestXxx (核心断言摘要)

### Skills learned(本次新坑,已同步 CLAUDE.md)
- ...

### 自检清单
[Part X + X.5 表逐项打勾,粘 Bash 输出]

### CI
backend / frontend / sidecar 全绿
```

---

# 最后的话:**v6 完成 = 本地能做完的全部**

完成 S+T+U+V 后,项目状态:
- ✅ 完整的 C2D 数据交易市场(L1/L2/L3 完整,只缺硬件)
- ✅ 完整的支付+结算+提现+对账+异常告警
- ✅ 完整的用户体验(通知+收藏+Q&A+邮件)
- ✅ 完整的合规栈(KYC + 数据导出 + 账号注销 + 同意 + 审计)
- ✅ 完整的认证(JWT + 2FA + 密码重置)
- ✅ 完整的运维(metrics + healthz + readyz + trace_id + 异常告警)
- ✅ 完整的 ops 看板(9 个 admin tab)
- ✅ 完整的测试网(每模块 _test.go,集成 + 单元)

**剩余**:全是外部基础设施/牌照(TEE 云、Secretflow 多节点、二清牌照),不在你的责任范围。

**就这些。从 PR-S 开始,严格顺序。完成 PR-S 告诉我,我立刻审。**
