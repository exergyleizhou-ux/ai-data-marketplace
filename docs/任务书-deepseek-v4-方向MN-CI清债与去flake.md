# 任务书 v4 给 DeepSeek V4 Pro — 方向 M + N(单 PR 收尾 CI 技术债)

**基线**:`origin/main`(等 PR #81 合后)
**审核人**:Claude Code(Opus 4.7)
**重要原则**:这是**纯 CI 清债**(无新功能),**一个 PR** 解决两个问题:
- **M**:修 `order/timeseries_test.go` 的裸 `DROP TABLE CASCADE`(造成 `-p 1` 全量测试时其它包测试 schema 被破坏)
- **N**:去 flake `TestComputeFederatedBelowThreshold`(`seedUser` 的 `users.account` 唯一键在并发/快速调用时撞)

**两者根因关联**:都是测试 setup 的不严谨。M 是 PR #74 你自己写的、N 是 PR #52 时代遗留 `seedUser` 不防御。修完 CLAUDE.md 也减一条 skip 守卫。

---

## 0. 操作前必做

```bash
cat ~/ai-data-marketplace/CLAUDE.md      # Gotchas 节,你已经追加 6 条,**注意第 1 条**(裸 CREATE TABLE)就是 M 要修的根因
git fetch origin && git log origin/main -3
```

继续守住的已记录规则:
- `gofmt -w . && goimports -w .`(两个都跑)
- `db.RunMigrations(dsn)` 不许裸 CREATE/DROP TABLE
- 新函数 → 新 `_test.go`
- 通知 emit `nil` 守卫 + `_ =` 吞错
- `.tsx` smart-quote 扫描 = 0
- CLAUDE.md gotcha 末尾单独 `docs(claude):` commit
- **等我审过才 merge**,即使 CI 绿(本次也铁律)

---

## 1. 方向 M — 修 `order/timeseries_test.go` 让它别破坏 schema

### M.0 现状(必读)

`backend/internal/modules/order/timeseries_test.go:13-66` `testPool` 函数:

```go
// 当前(坏):
pool.Exec(ctx, `DROP TABLE IF EXISTS settlement_outbox, orders, datasets CASCADE`)
pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    buyer_id TEXT NOT NULL,           ← 与生产 schema(UUID REFERENCES users)冲突
    seller_id TEXT NOT NULL,
    ...
)`);
pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS settlement_outbox (...)`);
pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS datasets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL DEFAULT ''   ← 缺 seller_id 等列,其它包测试要用 datasets.seller_id 直接挂
)`)
```

**后果**:
- `-p 1` 全量测试时 timeseries_test 先跑 → 把生产 schema 干掉 → 后跑的 watchlist/notification 等模块测试 schema 残缺 → 要么 fail 要么 skip(PR #81 加了 skip 守卫绕开)
- `buyer_id TEXT NOT NULL` 与生产 `UUID REFERENCES users(id)` 不一致 → FK 相关 bug 测不出(CLAUDE.md gotcha #1 原话)

### M.1 修复方案

**完全重写** `testPool` 用 `db.RunMigrations(dsn)` + 真 schema,辅助函数 seed 真用户/数据集:

```go
package order

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "os"
    "testing"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

// testPool opens an ephemeral PG and runs the production migrations.
// Caller must clean up its OWN test data (TRUNCATE the rows it inserted) — never
// DROP TABLE.  Other packages share this PG via -p 1.
func testPool(t *testing.T) *pgxpool.Pool {
    t.Helper()
    dsn := os.Getenv("DATABASE_URL")
    if dsn == "" {
        t.Skip("DATABASE_URL not set; skipping PG integration test")
    }
    if err := db.RunMigrations(dsn); err != nil {
        t.Fatalf("migrate: %v", err)
    }
    pool, err := pgxpool.New(context.Background(), dsn)
    if err != nil {
        t.Fatalf("connect: %v", err)
    }
    // ✓ DO clean OUR test rows so reruns are deterministic.  Use TRUNCATE
    // (idempotent, no error if table missing) NOT DROP.
    _, _ = pool.Exec(context.Background(), `
        TRUNCATE TABLE settlement_outbox CASCADE;
        TRUNCATE TABLE orders CASCADE;
    `)
    return pool
}

// uniqSuffix returns a hex-encoded random 8-byte string, guaranteed unique
// across rapid successive calls (unlike time.Now().UnixNano() which collides
// under nano-clock resolution or parallel runs).
func uniqSuffix() string {
    b := make([]byte, 8)
    _, _ = rand.Read(b)
    return hex.EncodeToString(b)
}

// seedUser inserts a real user row with a UUID id and returns the UUID string.
// Account is suffixed with uniqSuffix() so concurrent test runs never collide.
func seedUser(t *testing.T, pool *pgxpool.Pool, role string) string {
    t.Helper()
    suf := uniqSuffix()
    var id string
    if err := pool.QueryRow(context.Background(),
        `INSERT INTO users (account, account_type, password_hash, role, kyc_status)
         VALUES ($1,'email','x',$2,'verified') RETURNING id::text`,
        "ts-"+role+"-"+suf+"@x.com", role).Scan(&id); err != nil {
        t.Fatalf("seed user: %v", err)
    }
    return id
}

// seedDataset inserts a real datasets row with the given seller_id and a unique
// title.  Returns the dataset UUID string.
func seedDataset(t *testing.T, pool *pgxpool.Pool, sellerID string) string {
    t.Helper()
    var id string
    if err := pool.QueryRow(context.Background(),
        `INSERT INTO datasets (seller_id, title, data_type, license_type, status)
         VALUES ($1, $2, 'text', 'commercial', 'published') RETURNING id::text`,
        sellerID, "ts-ds-"+uniqSuffix()).Scan(&id); err != nil {
        t.Fatalf("seed dataset: %v", err)
    }
    return id
}
```

**修改 `insertOrder`** 让它接受真 UUID 的 buyer/seller(对应生产 schema):

```go
// 改 insertOrder 让 BuyerID/SellerID 必须是真 UUID
func insertOrder(t *testing.T, pool *pgxpool.Pool, o Order) {
    t.Helper()
    _, err := pool.Exec(context.Background(),
        `INSERT INTO orders (id, buyer_id, seller_id, dataset_id, version_id, license_type,
            amount_cents, platform_fee_cents, seller_amount_cents, status, product_type, created_at)
         VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, NULLIF($5,'')::uuid, $6, $7, $8, $9, $10, $11, $12::timestamptz)`,
        o.ID, o.BuyerID, o.SellerID, o.DatasetID, o.VersionID, o.LicenseType,
        o.AmountCents, o.PlatformFeeCents, o.SellerAmountCents, o.Status, o.ProductType, o.CreatedAt)
    if err != nil {
        t.Fatalf("insert order: %v", err)
    }
}
```

### M.2 重写两个测试用真 UUID

`TestAdminReconciliationTimeseries_FillsZeroDays`(line 70 之后):

```go
func TestAdminReconciliationTimeseries_FillsZeroDays(t *testing.T) {
    ctx := context.Background()
    pool := testPool(t)
    defer pool.Close()
    repo := &pgRepo{pool: pool}

    // 真 seed:用户 + 数据集 都用 UUID
    buyer := seedUser(t, pool, "buyer")
    seller := seedUser(t, pool, "seller")
    dsA := seedDataset(t, pool, seller)

    today := time.Now().UTC().Truncate(24 * time.Hour)
    yesterday := today.AddDate(0, 0, -1)
    yesterdayStr := yesterday.Format("2006-01-02")

    // 改 BuyerID/SellerID/DatasetID 用真 UUID,VersionID 留空字符串
    insertOrder(t, pool, Order{
        ID: uniqOrderID(t), BuyerID: buyer, SellerID: seller, DatasetID: dsA,
        VersionID: "", LicenseType: "commercial",
        AmountCents: 100000, PlatformFeeCents: 10000, SellerAmountCents: 90000,
        Status: StatusSettled, ProductType: ProductDownload,
        CreatedAt: yesterdayStr + "T10:00:00Z",
    })
    // ... 第二条 / 第三条同理 ...

    pts, err := repo.AdminReconciliationTimeseries(ctx, 30)
    // ... 原有断言保留(GMV=150000, settled=100000, refunded=1, 等等)...
}

// 帮手:返回独立 UUID(无须 google/uuid 依赖)
func uniqOrderID(t *testing.T) string {
    t.Helper()
    return "00000000-0000-0000-0000-" + uniqSuffix()[:12]
}
```

`TestSellerEarningsByDataset_AggregatesPerDataset` 同样改 — 用 `seedUser` / `seedDataset` 返回的真 UUID。

### M.3 必须删除的反模式

| 删除 | 替换 |
|---|---|
| `pool.Exec(ctx, "DROP TABLE IF EXISTS ... CASCADE")` | 删,改用 `TRUNCATE TABLE orders, settlement_outbox CASCADE`(只清自己用的两表) |
| `pool.Exec(ctx, "CREATE TABLE IF NOT EXISTS orders ...")` | 删,`db.RunMigrations(dsn)` 已经建好 |
| `pool.Exec(ctx, "CREATE TABLE IF NOT EXISTS settlement_outbox ...")` | 删,同上 |
| `pool.Exec(ctx, "CREATE TABLE IF NOT EXISTS datasets ...")` | 删,同上 |
| `buyer_id TEXT NOT NULL`(测试 schema 里) | 用真 UUID(来自 `seedUser`),`buyer_id::uuid` 转 |
| 硬编码 `BuyerID: "b1"` `SellerID: "s1"` | 用 `seedUser` 返回值 |
| 硬编码 `DatasetID: "c0000000-..."`(假 UUID) | 用 `seedDataset` 返回值 |

### M.4 验收

- 两个测试名**不许改**:`TestAdminReconciliationTimeseries_FillsZeroDays`、`TestSellerEarningsByDataset_AggregatesPerDataset`
- 原有数值断言**全部保留**(GMV=150000, settled_cents=135000, 等等)
- 跑 `go test -race -p 1 -count=1 ./...` 应**所有包**绿(以前会因 schema 污染挂)

---

## 2. 方向 N — 去 flake `TestComputeFederatedBelowThreshold`

### N.0 现状(必读)

`backend/internal/modules/compute/repo_integration_test.go` 里的 `seedUser`(被多个 compute 集成测试共享):

```go
func seedUser(t *testing.T, pool *pgxpool.Pool, account, role string) string {
    var id string
    if err := pool.QueryRow(context.Background(),
        `INSERT INTO users (account, account_type, password_hash, role, kyc_status)
         VALUES ($1,'email','x',$2,'verified') RETURNING id::text`,
         account+"@example.com", role).Scan(&id); err != nil {
        t.Fatalf("seed user: %v", err)   // ⚠️ 没 ON CONFLICT,撞 users.account UNIQUE 就 Fatal
    }
    return id
}
```

`fedToleranceSetup` 用 `time.Now().UnixNano()` 作 account 后缀。在快速连续调用(并发或纳秒级)时,两次 `seedUser` 可能产生相同 account 字符串 → `users.account` UNIQUE 冲突 → 测试 fail。

CI 上 `TestComputeFederatedBelowThreshold` 偶发 fail 就是这个根因。重跑就过(因为时间戳变了)。

### N.1 修复方案

把 `seedUser` 改成**用 crypto/rand 后缀**(纳秒分辨率不够)+ **加 ON CONFLICT 防御**:

```go
// 改 backend/internal/modules/compute/repo_integration_test.go 的 seedUser:
import (
    "crypto/rand"
    "encoding/hex"
)

// uniqAccountSuffix 返回 16 个十六进制字符,足以让 1 秒内 2^64 次调用不撞。
func uniqAccountSuffix() string {
    b := make([]byte, 8)
    _, _ = rand.Read(b)
    return hex.EncodeToString(b)
}

func seedUser(t *testing.T, pool *pgxpool.Pool, prefix, role string) string {
    t.Helper()
    account := prefix + "-" + uniqAccountSuffix() + "@example.com"
    var id string
    err := pool.QueryRow(context.Background(),
        `INSERT INTO users (account, account_type, password_hash, role, kyc_status)
         VALUES ($1,'email','x',$2,'verified')
         ON CONFLICT (account) DO UPDATE SET role = EXCLUDED.role
         RETURNING id::text`,
        account, role).Scan(&id)
    if err != nil {
        t.Fatalf("seed user: %v", err)
    }
    return id
}
```

**两层防御**:
1. `uniqAccountSuffix()` 用 `crypto/rand` 8 字节十六进制 → 实际碰撞概率约 `1 / 2^64`,纳秒撞不到
2. `ON CONFLICT (account) DO UPDATE` → 万一撞了也不报错,RETURNING 仍返回 row id(等于 idempotent)

### N.2 fedToleranceSetup 调用点改

```go
// 之前:
uniq := time.Now().UnixNano()
seller := seedUser(t, pool, fmt.Sprintf("%sseller-%d", prefix, uniq), "seller")
buyer = seedUser(t, pool, fmt.Sprintf("%sbuyer-%d", prefix, uniq), "buyer")

// 之后(seedUser 已经自带后缀,prefix 传 fix 串即可):
seller := seedUser(t, pool, prefix+"seller", "seller")
buyer = seedUser(t, pool, prefix+"buyer", "buyer")
```

`fedToleranceSetup` 的 `uniq := time.Now().UnixNano()` 也可以**删掉**(seedUser 自己用 crypto/rand 了)。

### N.3 全局检查 — 所有 compute 测试用 seedUser 的地方

```bash
# 你提 PR 前 grep 一下:
git grep -n "seedUser(t, pool," backend/internal/modules/compute/ | wc -l
# 至少 6 处,确认全部改成新签名 seedUser(t, pool, prefix, role) — 不传 nano 后缀,seedUser 自带
```

可能涉及修改的文件(基于现有调用点):
- `repo_integration_test.go`
- `federated_integration_test.go`
- `federated_tolerance_test.go`
- `psi_integration_test.go`
- `allow_psi_test.go`
- `service_test.go` 部分(如果用了)

**改动签名**:`seedUser(t, pool, account_full, role)` → `seedUser(t, pool, prefix, role)`(只传前缀,函数自己加 suffix)。

### N.4 验收

- 跑 `TestComputeFederatedBelowThreshold` **20 次**(`go test -race -count=20 -run TestComputeFederatedBelowThreshold ./internal/modules/compute/`)全 PASS
- `compute` 包所有其它集成测试全 PASS(改 seedUser 签名不能破其它测试)

---

## 3. 方向 M 完成后的收尾(顺手做)

### 3.1 删 PR #81 加的 watchlist skip 守卫

`backend/internal/modules/watchlist/repo_test.go`:

```go
// 删这段(共 9 行):
var hasSeller bool
_ = pool.QueryRow(context.Background(),
    `SELECT exists(SELECT 1 FROM information_schema.columns WHERE table_name='datasets' AND column_name='seller_id')`,
).Scan(&hasSeller)
if !hasSeller {
    t.Skip("datasets.seller_id missing — likely -p 1 CASCADE conflict (pre-existing); run watchlist tests in isolation")
}
pool.Exec(context.Background(), `DELETE FROM dataset_watches`)
```

替换为:

```go
// 改成简单的 TRUNCATE 自己的表:
pool.Exec(context.Background(), `TRUNCATE TABLE dataset_watches`)
```

### 3.2 CLAUDE.md 清理(在 Gotchas 节末尾追加 1 条)

注:第 1 条「不许裸 CREATE TABLE」已经在;但**没说 DROP TABLE CASCADE 同样禁止**。把它写明:

```markdown
- **Integration tests must NEVER `DROP TABLE … CASCADE`**: even with `IF EXISTS`, dropping a
  production table destroys schema for every other test in a `-p 1` run.  Use `TRUNCATE TABLE`
  (idempotent, schema-preserving) to clean rows between tests, never DROP.  PR #74
  timeseries_test.go originally did this and PR #81 had to add a defensive skip guard.  PR-M fix.
- **`seedUser` for integration tests must use crypto/rand suffix + ON CONFLICT DO UPDATE**:
  `time.Now().UnixNano()` collides under nano-clock resolution or parallel test runs.  The
  combination guarantees zero `users.account` UNIQUE conflicts even at high call rates.  PR-N fix.
```

---

## 4. 不许做

| ❌ | 原因 |
|---|---|
| 改 `Order` / `Entitlement` 的字段或迁移 | 这是 CI 清债,不动产品代码 |
| 引新依赖(google/uuid 等) | `crypto/rand` 标准库够用 |
| 改其它包的测试 setup(除非 compute 包 seedUser 改了签名要同步) | 范围控制 |
| `t.Cleanup(func(){ pool.Exec("DROP TABLE...") })` | 永远禁 DROP TABLE,TRUNCATE 即可 |
| 把 `db.RunMigrations` 放到 `TestMain` 而不是 `testPool` | 现有模式是 per-test pool,保持一致 |

---

## 5. 自检清单(逐项打勾再 push)

```
通用
[ ] cat ~/ai-data-marketplace/CLAUDE.md 已读
[ ] cd backend && gofmt -l . && goimports -l .  (两个都空)
[ ] cd backend && go vet ./...
[ ] cd backend && go build ./...
[ ] 真 PG -p 1 全量回归(无 skip):
    DATABASE_URL=... go test -race -p 1 -count=1 ./... 必须**所有包**绿,
    且 grep '_skip' 或 '— SKIP' 应只剩 (要么 0 个,要么是真的没设 DATABASE_URL)
[ ] cd frontend && npx tsc --noEmit && npx next lint && npm run build  (本 PR 无前端改动也跑一遍)
[ ] CLAUDE.md 末尾 commit 追加 2 条 gotchas(M 的 DROP CASCADE + N 的 seedUser)

M 专项
[ ] testPool 不再有 DROP TABLE / CREATE TABLE,只剩 db.RunMigrations + TRUNCATE
[ ] seedUser / seedDataset 帮手用 crypto/rand suffix
[ ] insertOrder 用真 UUID(从 seedUser/seedDataset 返回)
[ ] 2 个测试名不变,数值断言不变
[ ] 删 watchlist skip 守卫(同 PR 内做掉)

N 专项
[ ] seedUser 签名改成 (t, pool, prefix, role),内部 crypto/rand + ON CONFLICT DO UPDATE
[ ] grep 所有 seedUser 调用都改了
[ ] 跑 TestComputeFederatedBelowThreshold 20 次全 PASS:
    DATABASE_URL=... go test -race -count=20 -run TestComputeFederatedBelowThreshold \
        ./internal/modules/compute/
[ ] compute 包其它测试全 PASS

Commits
[ ] 4 commits:
    ① test/fix(order): replace DROP CASCADE with db.RunMigrations + TRUNCATE (M)
    ② test/fix(compute): seedUser crypto/rand suffix + ON CONFLICT (N)
    ③ test/fix(watchlist): remove skip guard, use TRUNCATE (cleanup)
    ④ docs(claude): add gotchas for DROP CASCADE and seedUser (M+N learning)
```

---

## 6. 我审核会查的清单

```
[ ] timeseries_test.go 不许有 "DROP TABLE" 字串(grep 验)
[ ] timeseries_test.go 必须 import "github.com/lei/ai-data-marketplace/backend/internal/platform/db"
[ ] testPool 必须调 db.RunMigrations(dsn)
[ ] compute/repo_integration_test.go seedUser 必须有 crypto/rand + ON CONFLICT (account)
[ ] grep "seedUser(t, pool" backend/internal/modules/compute/ 每个调用都新签名
[ ] watchlist/repo_test.go 不许有 "datasets.seller_id missing" 字串(skip 守卫已删)
[ ] CLAUDE.md 有 2 条新 gotcha(DROP CASCADE + seedUser)
[ ] 跑 20 次 TestComputeFederatedBelowThreshold 全过(你 PR description 贴输出)
[ ] CI 三 job 全绿
```

---

## 7. 工作流(同 v3 模板)

```bash
export PATH="$HOME/.local/bin:$HOME/sdk/node/bin:$HOME/sdk/pg/bin:$PATH"
git fetch origin
git worktree add ~/ai-data-marketplace-MN -b fix/ci-debt-mn origin/main

# ... 实现 + 验证 ...

cd backend && gofmt -w . && goimports -w .
go vet ./...
go build ./...
T=$(mktemp -d); SOCK=$(mktemp -d); PORT=55440
initdb -D "$T" -U postgres --auth=trust >/dev/null
pg_ctl -D "$T" -o "-p $PORT -k $SOCK -c listen_addresses=''" -w start >/dev/null
DATABASE_URL="postgres://postgres@/postgres?host=$SOCK&port=$PORT&sslmode=disable" \
    go test -race -p 1 -count=1 ./...

# ★ 关键的 20 次 deflake 验证:
DATABASE_URL="postgres://postgres@/postgres?host=$SOCK&port=$PORT&sslmode=disable" \
    go test -race -count=20 -run TestComputeFederatedBelowThreshold ./internal/modules/compute/

pg_ctl -D "$T" stop -m fast >/dev/null

cd ../frontend && npm ci --fetch-retries=5 && npx tsc --noEmit && npx next lint && npm run build

git push -u origin fix/ci-debt-mn
gh pr create --base main --title "fix(ci): clear DROP CASCADE debt + deflake seedUser (M+N)" --body "..."
gh pr checks <n> --watch
# 等我审过才 merge,不许自 merge
```

---

## 8. PR description 模板

```markdown
## fix(ci): clear DROP CASCADE debt + deflake seedUser (方向 M+N)

### 方向 M:order/timeseries_test.go 不再破坏全局 schema
- 重写 testPool 用 db.RunMigrations(dsn) + TRUNCATE,**不再 DROP TABLE**
- 新增 seedUser/seedDataset 帮手用 crypto/rand 后缀
- 用真 UUID(buyer_id/seller_id UUID REFERENCES users),数值断言全部保留

### 方向 N:seedUser crypto/rand + ON CONFLICT 防御
- compute/repo_integration_test.go 重写 seedUser 签名 (t, pool, prefix, role)
- crypto/rand 8 字节后缀 → 碰撞概率 1/2^64
- ON CONFLICT (account) DO UPDATE → 双保险
- N 个测试调用点同步改

### 收尾
- 删 watchlist/repo_test.go 的 skip 守卫(PR #81 临时补救)
- CLAUDE.md 追加 2 条 gotcha:DROP CASCADE 禁止 + seedUser 模式

### 20 次 deflake 验证
[paste `go test -race -count=20 -run TestComputeFederatedBelowThreshold` 输出]

### 自检清单
[Part 5 那张表逐项打勾]

### CI
backend / frontend / sidecar 全绿
```

---

**就这些。一个 PR 收尾两个 CI 技术债。修完等我审,绿了我点头才 merge。**
