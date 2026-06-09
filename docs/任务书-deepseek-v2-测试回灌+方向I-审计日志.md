# 任务书 v2 给 DeepSeek V4 Pro — Part A: 测试回灌 + Part B: 方向 I 审计日志 viewer

**基线**:`origin/main @ 15059f8`(PR #75 已合)
**审核人**:Claude Code(Opus 4.7)— 你提 PR 后我会逐项审,**请把本文档每一条都当强制项**
**重要前置**:你是**纯文本模型**(无多模态)。本文档所有 UI 都用文本描述 + 数据结构 + 示例 JSON,绝不依赖截图。所有代码位置都给到 `file:line`。

---

## 0. 操作前**必须**做的事(顺序不可变)

```bash
# 0.1 把项目根 CLAUDE.md 完整读一遍(你前两个 PR 都没读,踩了已记录的坑)
cat ~/ai-data-marketplace/CLAUDE.md

# 0.2 把 v2 交接的 Gotchas 节读一遍(项目专属坑)
sed -n '/## Conventions \/ gotchas/,/## C2D/p' ~/ai-data-marketplace/CLAUDE.md
```

**踩过且不许再踩的坑(摘自 CLAUDE.md):**

| 坑 | 怎么避免 |
|---|---|
| 编辑 `.go` 后 `struct` 字段会重新对齐,**忘 `gofmt -w`** CI 必挂 | 每次 push 前 `cd backend && gofmt -w .` 一遍 |
| 编辑 `.tsx` 时 Edit 工具偶尔把 `"` 替换为 curly quote `"`/`"` → tsc 报神秘 TS1127 | push 前 `python3 -c "import sys; d=open('frontend/components/X.tsx').read(); print(sum(1 for c in d if c in '""''))"` 必须 = 0(除非是已存在的英文显示文本里的) |
| JSONB `NOT NULL DEFAULT '{}'` 列传 `nil` 仍违约 | 传 `[]byte("{}")` 不传 `nil` |
| `uuid[]` 参数 | `$N::uuid[]` 显式转,扫回用 `dataset_ids::text[]` 进 `[]string` |
| 乐观状态机 | `UPDATE … WHERE status=$from RETURNING …`,0 行 ⇒ `ErrBadTransition` |
| **不要用裸 `CREATE TABLE` 起集成测试**(PR #74 你犯过) | 用 `db.RunMigrations(dsn)`,fake schema 与生产 schema 不一致会掩盖真 bug |
| 时间戳是 `string`(`::text` 扫描)不是 `time.Time` — 沿用既有风格 | DTO 全部 `string` |

---

## 1. 工作流铁律(每次)

```bash
git fetch origin
git worktree add ~/ai-data-marketplace-<name> -b feat/<name> origin/main
# 一棵 worktree 一件事
# 实现 + 真验证 + 推
git push -u origin feat/<name>
gh pr create --base main --title "..." --body "..."
gh pr checks <n> --watch
gh pr merge <n> --squash --delete-branch
git worktree remove ~/ai-data-marketplace-<name>
```

PATH 一次性设好:
```bash
export PATH="$HOME/.local/bin:$HOME/sdk/node/bin:$HOME/sdk/pg/bin:$PATH"
```

验证铁律(每个 PR 推之前**必须**全过):
```bash
cd backend
gofmt -l .              # ← 必须空
go vet ./...
go build ./...
# 真 PG 集成测试 (无 docker / 无 psql)
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
```

**Smart-quote 检查**(新增/修改的 `.tsx` 必须跑):
```bash
python3 -c "
import sys
for f in sys.argv[1:]:
    d = open(f).read()
    bad = [(i, hex(ord(c))) for i,c in enumerate(d) if c in '"""‘’']
    if bad:
        print(f, 'curly quotes at:', bad[:5])
" frontend/app/admin/page.tsx frontend/components/*.tsx
```
只允许出现在**显示给用户的英文/中文文本**里(例如 `"Allow federated learning"` 里的引号是 ToS 风格故意保留),**绝不能出现在**JSX 属性、字符串字面量分隔符、`useState("")`、`api.foo("")` 这些地方。

---

# Part A — PR #75 测试回灌(必做)

PR #75 通知+凭证模块功能没问题,**但 0 个新增 `_test.go`**(我审过)。补 3 个测试文件保证回归覆盖。

## A.1 `backend/internal/modules/notification/repo_test.go` (新文件)

**必须**包含以下 5 个测试,**必须**用 `db.RunMigrations(dsn)` 起 schema(**不许裸 CREATE**)。

```go
package notification

import (
    "context"
    "errors"
    "fmt"
    "os"
    "testing"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

func testRepo(t *testing.T) (Repository, func()) {
    t.Helper()
    dsn := os.Getenv("DATABASE_URL")
    if dsn == "" { t.Skip("DATABASE_URL not set") }
    if err := db.RunMigrations(dsn); err != nil { t.Fatalf("migrate: %v", err) }
    pool, err := pgxpool.New(context.Background(), dsn)
    if err != nil { t.Fatalf("pool: %v", err) }
    return NewRepository(pool), func() { pool.Close() }
}

// 关键测试 1:IDOR 防护回归 — 用户 B 不能改用户 A 的通知
func TestMarkRead_RejectsOtherUserIDOR(t *testing.T) {
    repo, cleanup := testRepo(t); defer cleanup()
    ctx := context.Background()
    uniq := time.Now().UnixNano()
    userA := fmt.Sprintf("user-a-%d", uniq)
    userB := fmt.Sprintf("user-b-%d", uniq)

    n, err := repo.Create(ctx, Notification{
        UserID: userA, Kind: "order_paid", Title: "test", Body: "body",
    })
    if err != nil { t.Fatalf("create: %v", err) }

    // userB 尝试改 userA 的通知 → 必须报 ErrNotFound (而不是静默成功)
    err = repo.MarkRead(ctx, n.ID, userB)
    if !errors.Is(err, ErrNotFound) {
        t.Fatalf("cross-user MarkRead must be ErrNotFound, got %v", err)
    }

    // userA 自己改可以
    if err := repo.MarkRead(ctx, n.ID, userA); err != nil {
        t.Fatalf("self MarkRead: %v", err)
    }
}

// 关键测试 2:ListByUser 严格按 user_id 过滤
func TestListByUser_RespectsScope(t *testing.T) {
    repo, cleanup := testRepo(t); defer cleanup()
    ctx := context.Background()
    uniq := time.Now().UnixNano()
    userA := fmt.Sprintf("scope-a-%d", uniq)
    userB := fmt.Sprintf("scope-b-%d", uniq)

    for i := 0; i < 2; i++ {
        if _, err := repo.Create(ctx, Notification{UserID: userA, Kind: "k", Title: "a"}); err != nil {
            t.Fatal(err)
        }
    }
    if _, err := repo.Create(ctx, Notification{UserID: userB, Kind: "k", Title: "b"}); err != nil {
        t.Fatal(err)
    }

    listA, err := repo.ListByUser(ctx, userA, 10, 0)
    if err != nil { t.Fatal(err) }
    if len(listA) != 2 { t.Fatalf("userA listed %d, want 2", len(listA)) }
    for _, n := range listA {
        if n.UserID != userA { t.Fatal("ListByUser leaked another user's row") }
    }
}

// 关键测试 3:MarkAllRead 只影响该用户的未读
func TestMarkAllRead_OnlySelfUnread(t *testing.T) {
    repo, cleanup := testRepo(t); defer cleanup()
    ctx := context.Background()
    uniq := time.Now().UnixNano()
    userA := fmt.Sprintf("all-a-%d", uniq)
    userB := fmt.Sprintf("all-b-%d", uniq)

    // userA: 3 条
    for i := 0; i < 3; i++ {
        if _, err := repo.Create(ctx, Notification{UserID: userA, Kind: "k", Title: "x"}); err != nil { t.Fatal(err) }
    }
    // userB: 2 条
    for i := 0; i < 2; i++ {
        if _, err := repo.Create(ctx, Notification{UserID: userB, Kind: "k", Title: "y"}); err != nil { t.Fatal(err) }
    }

    n, err := repo.MarkAllRead(ctx, userA)
    if err != nil { t.Fatal(err) }
    if n != 3 { t.Fatalf("MarkAllRead(userA) marked %d, want 3", n) }

    cnt, err := repo.CountUnread(ctx, userA)
    if err != nil { t.Fatal(err) }
    if cnt != 0 { t.Fatalf("userA unread after MarkAllRead = %d, want 0", cnt) }

    cntB, err := repo.CountUnread(ctx, userB)
    if err != nil { t.Fatal(err) }
    if cntB != 2 { t.Fatalf("userB unread = %d, want 2 (userA's MarkAllRead must NOT touch userB)", cntB) }
}

// 关键测试 4:CountUnread 只数未读
func TestCountUnread_ExcludesRead(t *testing.T) {
    repo, cleanup := testRepo(t); defer cleanup()
    ctx := context.Background()
    uniq := time.Now().UnixNano()
    user := fmt.Sprintf("count-%d", uniq)

    n1, _ := repo.Create(ctx, Notification{UserID: user, Kind: "k", Title: "1"})
    _, _ = repo.Create(ctx, Notification{UserID: user, Kind: "k", Title: "2"})
    _, _ = repo.Create(ctx, Notification{UserID: user, Kind: "k", Title: "3"})

    if err := repo.MarkRead(ctx, n1.ID, user); err != nil { t.Fatal(err) }

    cnt, err := repo.CountUnread(ctx, user)
    if err != nil { t.Fatal(err) }
    if cnt != 2 { t.Fatalf("unread = %d, want 2", cnt) }
}

// 关键测试 5:ListByUser 按 created_at DESC
func TestListByUser_OrdersByCreatedAtDesc(t *testing.T) {
    repo, cleanup := testRepo(t); defer cleanup()
    ctx := context.Background()
    uniq := time.Now().UnixNano()
    user := fmt.Sprintf("order-%d", uniq)

    titles := []string{"first", "second", "third"}
    for _, ttl := range titles {
        if _, err := repo.Create(ctx, Notification{UserID: user, Kind: "k", Title: ttl}); err != nil { t.Fatal(err) }
        time.Sleep(5 * time.Millisecond) // 保证 created_at 严格递增
    }

    list, err := repo.ListByUser(ctx, user, 10, 0)
    if err != nil { t.Fatal(err) }
    if len(list) != 3 { t.Fatalf("got %d, want 3", len(list)) }
    if list[0].Title != "third" || list[2].Title != "first" {
        t.Fatalf("order wrong: %s,%s,%s want third,second,first", list[0].Title, list[1].Title, list[2].Title)
    }
}
```

## A.2 `backend/internal/modules/verify/repo_test.go` (新文件)

**必须**3 个测试:

```go
package verify

import (
    "context"
    "errors"
    "fmt"
    "os"
    "testing"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

func testRepo(t *testing.T) (Repository, func()) {
    t.Helper()
    dsn := os.Getenv("DATABASE_URL")
    if dsn == "" { t.Skip("DATABASE_URL not set") }
    if err := db.RunMigrations(dsn); err != nil { t.Fatalf("migrate: %v", err) }
    pool, err := pgxpool.New(context.Background(), dsn)
    if err != nil { t.Fatalf("pool: %v", err) }
    return NewRepository(pool), func() { pool.Close() }
}

// 同 cert_id 注册两次必须不报错(ON CONFLICT DO NOTHING 回归)
func TestRegister_Idempotent(t *testing.T) {
    repo, cleanup := testRepo(t); defer cleanup()
    ctx := context.Background()
    certID := fmt.Sprintf("VO-IDEM%04d", time.Now().UnixNano()%10000)
    if err := repo.Register(ctx, certID, "dataset", "ds-1"); err != nil {
        t.Fatalf("first register: %v", err)
    }
    if err := repo.Register(ctx, certID, "dataset", "ds-1"); err != nil {
        t.Fatalf("second register must be idempotent, got: %v", err)
    }
}

// 同 cert_id 不同 resource → 第二次不覆盖(ON CONFLICT DO NOTHING 不更新)
func TestRegister_SameCertIDDifferentResource_KeepsFirst(t *testing.T) {
    repo, cleanup := testRepo(t); defer cleanup()
    ctx := context.Background()
    certID := fmt.Sprintf("VO-KEEP%04d", time.Now().UnixNano()%10000)
    if err := repo.Register(ctx, certID, "dataset", "ds-original"); err != nil { t.Fatal(err) }
    if err := repo.Register(ctx, certID, "dataset", "ds-other"); err != nil { t.Fatal(err) }
    info, err := repo.FindByCertID(ctx, certID)
    if err != nil { t.Fatal(err) }
    if info.ResourceID != "ds-original" {
        t.Fatalf("ON CONFLICT DO NOTHING should keep first: got %s, want ds-original", info.ResourceID)
    }
}

func TestFindByCertID_NotFound(t *testing.T) {
    repo, cleanup := testRepo(t); defer cleanup()
    _, err := repo.FindByCertID(context.Background(), "VO-NEVER-EXISTS")
    if !errors.Is(err, ErrNotFound) {
        t.Fatalf("missing cert must return ErrNotFound, got %v", err)
    }
}
```

## A.3 `backend/internal/modules/order/service_test.go` (扩展,**不新建文件**)

在现有 `order/service_test.go` 末尾追加(**保留**已有内容):

```go
// --- Notifier 集成测试(回归 PR #75 通知 emit)---

type fakeNotifier struct {
    calls []notifyCall
}
type notifyCall struct {
    UserID, Kind, Title, Body, ResourceType, ResourceID string
}
func (f *fakeNotifier) NotifyUser(ctx context.Context, userID, kind, title, body, resourceType, resourceID string) error {
    f.calls = append(f.calls, notifyCall{userID, kind, title, body, resourceType, resourceID})
    return nil
}

func TestMarkPaid_DownloadOrder_EmitsBuyerNotification(t *testing.T) {
    // 用既有 fakeRepo + fakeIdentity + fakeDatasets;具体构造看本文件已有 TestXxx 范例
    // 关键断言:
    //   - 创建 1 个 download 类型 order, MarkPaid → fake.calls 长度 == 1
    //   - calls[0].Kind == "order_paid"
    //   - calls[0].UserID == buyerID
    //   - calls[0].ResourceType == "order"
    //   - calls[0].ResourceID == order.ID
    // (skeleton 由你按本文件已有 TestCreate / TestMarkPaid 仿写)
}

func TestMarkPaid_ComputeOrder_DoesNotEmitBuyerNotification(t *testing.T) {
    // compute 类型 order MarkPaid → 走 granter,不发买家通知
    // 断言:fake.calls 长度 == 0
}

func TestMarkSettled_EmitsSellerNotification(t *testing.T) {
    // 流程:Create → MarkPaid → MarkDelivered → ConfirmDelivery → settled
    // 断言:fake.calls 中有 1 条 kind=order_settled, UserID=sellerID,
    //       Body 包含 ¥ 符号(格式化金额)
}

func TestDispute_BuyerInitiated_NotifiesSeller(t *testing.T) {
    // userID=buyer 调 Dispute → calls 中有 1 条 kind=order_disputed,target=seller
}

func TestDispute_SellerInitiated_NotifiesBuyer(t *testing.T) {
    // userID=seller 调 Dispute → calls 中有 1 条 kind=order_disputed,target=buyer
}

func TestMarkPaid_NilNotifier_NoPanic(t *testing.T) {
    // svc 不调 SetNotifier(s.notifier==nil)
    // MarkPaid 必须不 panic、不报错(if s.notifier != nil 守卫回归)
}
```

**如果**追加后单元测试编译不过(例如 `fakeRepo` 字段缺啥),按缺啥补啥,**不许**改动现有测试的断言。

---

# Part B — 方向 I:审计日志 Ops Viewer

## B.0 现状(必读)

- `backend/internal/platform/audit/audit.go`:`Recorder` 接口 + `Entry{ActorID, ActorRole, Action, ResourceType, ResourceID, Detail map[string]any}`
- `backend/migrations/000001_init.up.sql` 已有表 + 3 索引 + **append-only trigger**(`UPDATE`/`DELETE` 抛异常 — 你**只能**读,绝对不许写删改)
- 30+ 调用点(`s.audit.Record(ctx, audit.Entry{...})`),涵盖 order/payment/dataset/compute/auth 各模块
- **当前 0 UI 入口** — ops 没法翻审计日志,合规复盘只能登 DB,这是真缺口

## B.1 后端:新模块 `backend/internal/modules/auditlog/`

```
auditlog/
  model.go     (定义 LogEntry DTO + ListFilter struct)
  repo.go      (Repository 接口 + pgRepo;只读!不许有 Insert/Update/Delete)
  service.go   (Service 包一层,留 audit 调用钩子用)
  handler.go   (Register + 1 个 handler: ListAuditLogs)
```

### B.1.1 DTO

```go
type LogEntry struct {
    ID           int64          `json:"id"`
    ActorID      string         `json:"actor_id,omitempty"`      // UUID 转 string,nil 时空
    ActorRole    string         `json:"actor_role,omitempty"`
    Action       string         `json:"action"`
    ResourceType string         `json:"resource_type,omitempty"`
    ResourceID   string         `json:"resource_id,omitempty"`
    IP           string         `json:"ip,omitempty"`            // INET 转 string
    UserAgent    string         `json:"user_agent,omitempty"`
    Detail       map[string]any `json:"detail,omitempty"`        // JSONB
    CreatedAt    string         `json:"created_at"`              // ::text
}

type ListFilter struct {
    ActorID      string // 空字符串=不过滤
    Action       string
    ResourceType string
    ResourceID   string
    From         string // RFC3339 字符串,空=不过滤
    To           string
    Limit        int
    Offset       int
}
```

### B.1.2 SQL(repo.go)

**必须**用可选过滤模式(空字符串 = 跳过该过滤),用 pgx 参数化:

```sql
SELECT id, COALESCE(actor_id::text, ''), COALESCE(actor_role, ''), action,
       COALESCE(resource_type, ''), COALESCE(resource_id, ''),
       COALESCE(host(ip), ''), COALESCE(user_agent, ''),
       COALESCE(detail, '{}'::jsonb),
       created_at::text
FROM audit_logs
WHERE ($1 = '' OR actor_id::text = $1)
  AND ($2 = '' OR action = $2)
  AND ($3 = '' OR resource_type = $3)
  AND ($4 = '' OR resource_id = $4)
  AND ($5 = '' OR created_at >= $5::timestamptz)
  AND ($6 = '' OR created_at <  $6::timestamptz)
ORDER BY created_at DESC, id DESC
LIMIT $7 OFFSET $8
```

Limit 规则:
- 输入 ≤ 0 或为空 ⇒ 默认 50
- 输入 > 200 ⇒ 夹到 200
- Offset < 0 ⇒ 改为 0

**Detail** 列:scan 到 `[]byte`,然后 `json.Unmarshal` 到 `map[string]any`;空/失败时返回 `nil`(让 `omitempty` 省掉)。

### B.1.3 端点

```
GET /admin/audit-logs?actor=&action=&resource_type=&resource_id=&from=&to=&limit=&offset=
```

- 路由组挂在 `admin`(已有 ops gate,**复用现有**`auth.RequireRole("ops","admin")`)
- 响应:`{"items": [...], "limit": N, "offset": M, "next_offset": M+N (当 len(items)==limit 时,否则缺省)}`

### B.1.4 服务端接线 `backend/internal/server/server.go`

在已注册其他 admin 模块之后插入(按现有模块的注册风格):

```go
auditSvc := auditlog.NewService(auditlog.NewRepository(s.db))
auditlog.Register(api, auditSvc, auth.RequireRole("ops", "admin"))
```

不要**改动**`audit.Recorder` 接口或写入路径 — `auditlog` 模块只**读** `audit_logs` 表。

## B.2 测试(强制,**不许少**)

**新文件** `backend/internal/modules/auditlog/repo_test.go`,**必须**用 `db.RunMigrations`:

```go
package auditlog

// 6 个测试,缺一不可:

func TestList_NoFilter_ReturnsAllRecent(t *testing.T) {
    // 直接 INSERT 3 条到 audit_logs(测试用 INSERT 允许,append-only trigger 只挡 UPDATE/DELETE)
    // List(ListFilter{Limit: 10}) → len == 3
    // 顺序:created_at DESC
}

func TestList_FiltersByAction(t *testing.T) {
    // 插 2 条 action=foo, 3 条 action=bar
    // List(ListFilter{Action: "bar", Limit: 10}) → len == 3,全部 Action=="bar"
}

func TestList_FiltersByActor(t *testing.T) {
    // 用真 UUID,不要用 "user-1"
    // List(ListFilter{ActorID: uuidA, Limit: 10}) → 只返回 actor=A 的行
}

func TestList_FiltersByResource(t *testing.T) {
    // resource_type + resource_id 两个都过滤
}

func TestList_FiltersByDateRange(t *testing.T) {
    // 插条目时显式设置 created_at(用 INSERT ... (created_at) VALUES ($1, ...))
    // 时间区间 [from, to) 半开,边界对齐
}

func TestList_LimitClampedTo200(t *testing.T) {
    // List(ListFilter{Limit: 500}) → 实际 SQL limit == 200
    // 用反射或者通过插 250 条然后查到 200 来验
}

func TestList_NegativeOffsetClampedToZero(t *testing.T) {
    // List(ListFilter{Offset: -5}) → 不报错且返回首页
}
```

## B.3 前端 — `frontend/app/admin/page.tsx` 加新 Tab

**当前** admin 页面有 5 个 Tab(`review` / `kyc` / `tx` / `compute` / `outbox`,见 PR #72)。
**做** 加第 6 个 Tab:`audit`(显示名「审计日志」/`Audit logs`)。

### B.3.1 结构

```tsx
// 新组件,加在 page.tsx 文件内(同已有模式)
function AuditLogs() {
  const { t } = useT();
  const [items, setItems] = useState<AuditLogEntry[]>([]);
  const [filters, setFilters] = useState({
    actor: "", action: "", resource_type: "", resource_id: "",
    from: "", to: "",
  });
  const [offset, setOffset] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [busy, setBusy] = useState(false);
  const [expanded, setExpanded] = useState<Set<number>>(new Set());

  const fetchPage = useCallback(async (nextOffset: number, append: boolean) => {
    setBusy(true);
    try {
      const r = await api.adminListAuditLogs({ ...filters, limit: 50, offset: nextOffset });
      setItems(prev => append ? [...prev, ...r.items] : r.items);
      setHasMore(r.next_offset !== undefined);
      setOffset(nextOffset);
    } finally {
      setBusy(false);
    }
  }, [filters]);

  useEffect(() => { void fetchPage(0, false); }, []);

  // UI:
  // - 6 个过滤输入框(横排,每行 3 个)+ "应用过滤" 按钮 (调 fetchPage(0,false))
  // - 表格列:时间 / actor / action / resource_type / resource_id / detail(折叠图标)
  // - 点 detail 图标 → expanded 集合 toggle id → 展开下方一行显示 <pre>{JSON.stringify(item.detail, null, 2)}</pre>
  // - 底部 "加载更多" 按钮 (hasMore && !busy 时显示)
}
```

### B.3.2 i18n

参考已有 admin Tab 用 `t("中文", "English")` 双语,如:
- 「审计日志」/`Audit logs`
- 「时间」/`Time`、「操作者」/`Actor`、「动作」/`Action`、「资源」/`Resource`、「详情」/`Detail`
- 「应用过滤」/`Apply filters`、「加载更多」/`Load more`、「查看 JSON」/`View JSON`

### B.3.3 在 Tab 切换器加 `audit`

找到 page.tsx 现有 Tab 数组(应该是 `["review","kyc","tx","compute","outbox"]`),加 `"audit"`。Tab 类型 union 同步加 `"audit"`。

## B.4 `frontend/lib/api.ts`

加类型 + 方法(模仿 PR #72 / #74 的方法风格):

```ts
export type AuditLogEntry = {
  id: number;
  actor_id?: string;
  actor_role?: string;
  action: string;
  resource_type?: string;
  resource_id?: string;
  ip?: string;
  user_agent?: string;
  detail?: Record<string, unknown>;
  created_at: string;
};

// 在 api 对象里加:
adminListAuditLogs: (q: {
  actor?: string;
  action?: string;
  resource_type?: string;
  resource_id?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}) => request<{ items: AuditLogEntry[]; limit: number; offset: number; next_offset?: number }>(
  "/admin/audit-logs",
  { query: q as Record<string, string | number | undefined> }
),
```

## B.5 不许做的事

| ❌ | 原因 |
|---|---|
| 改 `audit.Recorder` 接口 | 调用点 30+,改动放大 |
| 写入/修改/删除 `audit_logs` 表 | append-only trigger 会让你的代码爆;就算绕开也违反审计的根本意义 |
| 加新迁移 | 现有 3 索引(actor/resource/created_at)足够 |
| 在 audit 模块里调 `s.audit.Record` | 别自我递归 |
| 复制 PR #74 的「裸 CREATE TABLE 起测试」 | 必须 `db.RunMigrations(dsn)`,这是 PR #74 你被点名的事 |
| 引入图表库 | 审计日志是表格,不需要图,引入 deps 就拒 |

---

# Part C — 双线磨合(本次**强制**,上次没做)

**PR #74 #75 commit body 写了 "Skills learned" 但没改 CLAUDE.md** — 这次必须改,作为本 PR 的**最后一个 commit**(单独 commit 方便审):

1. 找到 `~/ai-data-marketplace/CLAUDE.md` 的 `## Conventions / gotchas` 节
2. **必须**追加至少 2 条本次新学到的坑(从下面里选,**或**你自己踩的别的真坑):
   - 「**集成测试必须 `db.RunMigrations`,不许裸 `CREATE TABLE`**:fake schema 与生产不一致(如 `buyer_id TEXT` vs `UUID REFERENCES users`)会掩盖 FK / 类型相关的 bug。PR #74 timeseries_test.go 已踩。」
   - 「**新模块要写测试**:PR #75 通知/凭证模块 0 个 `_test.go`,IDOR 和幂等都没覆盖。从今往后任何 `service.go` / `repo.go` 新文件必须配同名 `_test.go`。」
   - 「**通知 emit 必须 `nil` 守卫 + 吞错**:`if s.notifier != nil { _ = s.notifier.NotifyUser(...) }`,业务主流程不可阻塞在通知失败。模式见 order/service.go MarkPaid。」
3. 这一 commit 的 message:`docs(claude): add gotchas learned from PRs #74/#75/#<this>`,**只**改 CLAUDE.md,不混功能代码

---

# Part D — 我会怎么审(审核清单,你照着自检)

提 PR 前**自己**跑一遍这张表;每条都过再 push:

```
[ ] 0.  CLAUDE.md 已读(可在 PR description 写一句 "Read CLAUDE.md ✓")
[ ] 1.  cd backend && gofmt -l .       (输出为空)
[ ] 2.  cd backend && go vet ./...     (无 warning)
[ ] 3.  cd backend && go build ./...
[ ] 4.  真 PG: go test -race -p 1 -count=1 ./internal/modules/notification/
[ ] 5.  真 PG: go test -race -p 1 -count=1 ./internal/modules/verify/
[ ] 6.  真 PG: go test -race -p 1 -count=1 ./internal/modules/order/
[ ] 7.  真 PG: go test -race -p 1 -count=1 ./internal/modules/auditlog/
[ ] 8.  真 PG: go test -race -p 1 -count=1 ./...        (回归全过)
[ ] 9.  cd frontend && npx tsc --noEmit
[ ] 10. cd frontend && npx next lint                    (0 warnings 0 errors)
[ ] 11. cd frontend && npm run build
[ ] 12. smart-quote 扫 frontend/app/admin/page.tsx + 新 .tsx == 0(除非已有英文文本)
[ ] 13. 5 个 notification 测试 + 3 个 verify 测试 + 5+ 个 order 测试 + 7 个 auditlog 测试
        每个都跑过(看 -v 输出 PASS 不只是包级 ok)
[ ] 14. CLAUDE.md 末尾 commit 追加了 ≥2 条 gotchas
[ ] 15. PR description 含:Part A 改动总结 / Part B 改动总结 / 审计点逐项打勾的截图(你 paste 输出文本即可)
```

## 我会**额外**抽查:

- 通知模块 `MarkRead` SQL 必须有 `WHERE id=$1 AND user_id=$2`(再次确认 IDOR 防护未被偶然回归)
- audit-logs SQL 6 个过滤位的 `($N = '' OR ...)` 模式无 SQL 注入(参数化)
- audit-logs handler 必须挂在 `admin` 路由组(经 `auth.RequireRole("ops","admin")`)— 我会 grep
- 前端 `expanded` set 状态 toggle 正常,JSON 不空
- `Tab` 类型 union 加了 `"audit"`(否则 TypeScript 编译会过但运行时 tab 切不过去)
- 新增 `.tsx` 文件 smart-quote 扫描:0
- 新增 `_test.go` 全部用 `db.RunMigrations`,**没有**裸 `CREATE TABLE`(grep 验)
- 6 commits 序看你 TDD 流程:理想是 ① 加测试(RED)→ ② 实现(GREEN)→ ③ 前端 → ④ docs/claude gotcha

---

# Part E — 提 PR 的格式(请按这个模板填)

```markdown
## Part A · 测试回灌(PR #75 修补)
- notification/repo_test.go(5 tests):IDOR 防护 / 范围 / MarkAllRead / 未读计数 / 排序
- verify/repo_test.go(3 tests):幂等 / 同 cert 不同 resource 保留首次 / 未找到
- order/service_test.go 追加(6 tests):通知 emit 各场景 + 无 notifier 不崩

## Part B · 方向 I:审计日志 Ops Viewer
- 新模块 backend/internal/modules/auditlog/(model+repo+service+handler)
- 端点 GET /admin/audit-logs(6 维过滤 + 分页 + JSON detail 折叠)
- 复用 auth.RequireRole("ops","admin"),零新迁移(复用既有 3 索引)
- frontend admin 第 6 Tab「审计日志」+ lib/api.ts 加 adminListAuditLogs

## Part C · 双线磨合
- CLAUDE.md 追加 2+ 条 gotchas(commit XYZ)

## 自检清单
[Part D 那张表 paste 进来,每条打勾]

## CI
backend / frontend / sidecar 三 job 绿
```

---

# 附:本次执行的 worktree 推荐分支名

```
git worktree add ~/ai-data-marketplace-deepseek-v2 -b feat/tests-and-auditlog origin/main
```

提一个 PR 即可,**Part A + Part B + Part C 合一个 PR**,commits 分清楚(便于审):
1. `test: notification repo regression coverage (IDOR + scope)`
2. `test: verify repo regression coverage (idempotency)`
3. `test: order service notifier integration coverage`
4. `feat(auditlog): backend module + GET /admin/audit-logs`
5. `feat(auditlog): admin tab + lib/api.ts method`
6. `docs(claude): gotchas learned from PRs #74/#75/this`

---

**就这些。有疑问看本文档对应小节,不要发挥。完成后我审。**
