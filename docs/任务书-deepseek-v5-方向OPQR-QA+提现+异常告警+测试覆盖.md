# 任务书 v5 给 DeepSeek V4 Pro — 方向 O+P+Q+R(4 PR 顺序交付)

**基线**:`origin/main`(等 PR #83 合后)
**审核人**:Claude Code(Opus 4.7)
**重要原则**:
- **4 PR 严格顺序**:O → P → Q → R,前一个合并后才开下一个
- 每 PR 独立 worktree、独立审、独立合并
- **等我审过才 merge**,即使 CI 绿,**永远不许自 merge**
- **你是纯文本模型(无多模态)**:所有 UI 描述用 state shape + behavior 表 + 组件骨架,绝不依赖截图

---

## 0. 操作前必做(每次新 PR 都做)

```bash
cat ~/ai-data-marketplace/CLAUDE.md
git fetch origin && git log origin/main -3
```

**铁律 checklist(每次 PR 都核对)**:
- [ ] `gofmt -w . && goimports -w .` 两个都跑
- [ ] `db.RunMigrations(dsn)` 不许裸 `CREATE TABLE`(M+N 已封禁,本次必须守住)
- [ ] 新 `service.go`/`repo.go` 必须配 `_test.go`,**测试名跟我 spec 一字不改**
- [ ] 通知 emit:`if s.notifier != nil { _ = s.notifier.NotifyUser(...) }`
- [ ] `seedUser` 必须 `crypto/rand` 后缀 + `ON CONFLICT DO UPDATE`(PR-N 模式)
- [ ] `.tsx` smart-quote 扫描 = 0(在 JSX 分隔符/字符串字面量位置)
- [ ] CLAUDE.md gotcha 末尾单独 `docs(claude):` commit,记录本 PR 真实战学到的坑
- [ ] **commits 序**:① test → ② backend → ③ frontend → ④ docs(claude),不许 1-commit 全塞

**严禁的反模式(过去踩过)**:
- ❌ 改 spec 锁定的测试名(PR #81 改了 `TestAdd_InitializesLastNotifiedToCurrentVersion` → 失语义)
- ❌ 把 service 层 validation 错误吞了不向上传播(PR #80 第一版 handler)
- ❌ 用 `t.Skip` 当防御性 guard(PR #81 临时补救,现已删)
- ❌ 硬编码常量替代参数(PR #79 固定 30s 退避)
- ❌ 把 4 个 commit 塞成 1 个

---

## 1. PR 顺序与依赖

| PR | 方向 | 价值 | 估算 | 测试数 |
|---|---|---|---|---|
| **PR-O** | 数据集 Q&A(买家提问 + 卖家回答 + 通知) | 用户互动新维度,复用 #75 通知 + #81 watch | ~600 行 | 12 |
| **PR-P** | 卖家提现申请 + ops 审批(只记账不触发分账) | 闭合卖家全流程,**绕开二清红线** | ~500 行 | 11 |
| **PR-Q** | 审计日志异常告警 + ops 处理 | 复用 #77 audit-logs + 后台扫描器,接 ops 看板 | ~400 行 | 9 |
| **PR-R** | 给 PR #72 的 ops/payment/order/search 补 service+repo 测试 | 清最后一块测试债 | ~300 行 | 14 |

**铁律**:O 合并后才开 P 的 worktree。

---

## 2. 工作流模板(每个 PR 都这样,从 PR-K 那次起这套流程很顺)

```bash
export PATH="$HOME/.local/bin:$HOME/sdk/node/bin:$HOME/sdk/pg/bin:$PATH"

git fetch origin
git worktree add ~/ai-data-marketplace-<name> -b feat/<name> origin/main

# 实现 + 验证(同 v4 模板)

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

python3 -c "
for f in ['frontend/app/admin/page.tsx', '<新 .tsx 文件路径>']:
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

# PR-O · 数据集 Q&A(买家问 + 卖家答 + 通知)

## O.0 现状(必读)

- `backend/internal/modules/dataset/` 是产品核心模块,详情页有 `<DatasetDetail>`(`frontend/app/datasets/[id]/page.tsx`)
- 通知模块已就绪(#75 `backend/internal/modules/notification/`)
- 收藏模块已就绪(#81 `backend/internal/modules/watchlist/`),`dataset_updated` 通知 kind 已渲染

**真缺口**:买家想问数据集的具体信息(format / 字段定义 / 更新频率),卖家想公开回答 — 当前**完全没有渠道**,只能私下聊。

## O.1 数据模型

**新迁移** `backend/migrations/000017_dataset_qa.up.sql`:

```sql
-- 000017: dataset Q&A.  Buyer asks public question on a dataset; seller answers.
-- Both sides public so future buyers see prior answers (discovery value).
CREATE TABLE IF NOT EXISTS dataset_questions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dataset_id      UUID NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
    asker_id        UUID NOT NULL REFERENCES users(id),
    body            TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'open'
                        CHECK (status IN ('open', 'answered', 'hidden')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS dataset_answers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    question_id     UUID NOT NULL REFERENCES dataset_questions(id) ON DELETE CASCADE,
    answerer_id     UUID NOT NULL REFERENCES users(id),
    body            TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_dataset_questions_dataset
    ON dataset_questions (dataset_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dataset_answers_question
    ON dataset_answers (question_id, created_at);
```

**down**:`DROP TABLE IF EXISTS dataset_answers; DROP TABLE IF EXISTS dataset_questions;`

## O.2 新模块 `backend/internal/modules/qa/`

```
qa/
  model.go    (Question/Answer DTO + 接口 + 错误哨兵)
  repo.go     (Repository 接口 + pgRepo)
  service.go  (Service + DatasetReader 接口注入 + Notifier 接口注入)
  handler.go  (Register + 4 个 handler)
```

### O.2.1 DTO

```go
type Question struct {
    ID         string `json:"id"`
    DatasetID  string `json:"dataset_id"`
    AskerID    string `json:"asker_id"`
    AskerName  string `json:"asker_name,omitempty"`  // join 时填(account 前 8 位)
    Body       string `json:"body"`
    Status     string `json:"status"`
    Answer     *Answer `json:"answer,omitempty"`
    CreatedAt  string `json:"created_at"`
}

type Answer struct {
    ID          string `json:"id"`
    QuestionID  string `json:"question_id"`
    AnswererID  string `json:"answerer_id"`
    Body        string `json:"body"`
    CreatedAt   string `json:"created_at"`
}

var (
    ErrQuestionNotFound = errors.New("question not found")
    ErrAlreadyAnswered  = errors.New("question already has an answer")
    ErrNotSeller        = errors.New("only the dataset seller can answer")
    ErrEmptyBody        = errors.New("body cannot be empty")
    ErrBodyTooLong      = errors.New("body exceeds 2000 characters")
)
```

### O.2.2 Repository 接口 + SQL

```go
type Repository interface {
    CreateQuestion(ctx context.Context, q Question) (Question, error)
    CreateAnswer(ctx context.Context, a Answer) (Answer, error)
    ListByDataset(ctx context.Context, datasetID string, limit, offset int) ([]Question, error)
    GetQuestion(ctx context.Context, id string) (Question, error)
    SetQuestionStatus(ctx context.Context, id, status string) error
}
```

**关键 SQL** `ListByDataset`(左连 answers + users):

```sql
SELECT q.id::text, q.dataset_id::text, q.asker_id::text,
       COALESCE(SUBSTRING(u.account, 1, 8), ''),    -- asker_name = account 前 8 位脱敏
       q.body, q.status, q.created_at::text,
       a.id::text, a.answerer_id::text, a.body, a.created_at::text
FROM dataset_questions q
JOIN users u ON u.id = q.asker_id
LEFT JOIN dataset_answers a ON a.question_id = q.id
WHERE q.dataset_id = $1 AND q.status != 'hidden'
ORDER BY q.created_at DESC
LIMIT $2 OFFSET $3
```

Scan 时:answer 字段全空(`a.id IS NULL`)则 `Question.Answer = nil`。

**`SetQuestionStatus`** 用乐观状态机(参考 order):
```sql
UPDATE dataset_questions
SET status = $2
WHERE id = $1 AND status != 'hidden'
```

### O.2.3 Service(关键业务逻辑)

```go
type DatasetReader interface {
    // SellerOf 返回数据集的 seller_id,不存在/未发布返回错误。
    // 业务约束:只能问已发布的数据集。
    SellerOf(ctx context.Context, datasetID string) (sellerID, status string, err error)
}

type Notifier interface {
    NotifyUser(ctx context.Context, userID, kind, title, body, resourceType, resourceID string) error
}

type Service struct {
    repo     Repository
    ds       DatasetReader
    notifier Notifier
}

func NewService(repo Repository, ds DatasetReader, notifier Notifier) *Service {
    return &Service{repo: repo, ds: ds, notifier: notifier}
}
```

#### Service.AskQuestion

```go
func (s *Service) AskQuestion(ctx context.Context, askerID, datasetID, body string) (Question, error) {
    body = strings.TrimSpace(body)
    if body == "" {
        return Question{}, ErrEmptyBody
    }
    if len(body) > 2000 {
        return Question{}, ErrBodyTooLong
    }
    sellerID, status, err := s.ds.SellerOf(ctx, datasetID)
    if err != nil {
        return Question{}, ErrQuestionNotFound  // 不暴露存在性
    }
    if status != "published" && status != "reviewing" {
        return Question{}, ErrQuestionNotFound
    }
    q, err := s.repo.CreateQuestion(ctx, Question{
        DatasetID: datasetID, AskerID: askerID, Body: body, Status: "open",
    })
    if err != nil {
        return Question{}, err
    }
    // 通知卖家(异步、吞错)
    if s.notifier != nil && sellerID != askerID {
        _ = s.notifier.NotifyUser(ctx, sellerID, "question_asked",
            "数据集有新提问", "您的数据集收到一条新提问,请前往详情页查看。",
            "dataset", datasetID)
    }
    return q, nil
}
```

#### Service.AnswerQuestion

```go
func (s *Service) AnswerQuestion(ctx context.Context, answererID, questionID, body string) (Answer, error) {
    body = strings.TrimSpace(body)
    if body == "" {
        return Answer{}, ErrEmptyBody
    }
    if len(body) > 2000 {
        return Answer{}, ErrBodyTooLong
    }
    q, err := s.repo.GetQuestion(ctx, questionID)
    if err != nil {
        return Answer{}, ErrQuestionNotFound
    }
    // 只有卖家本人能答
    sellerID, _, err := s.ds.SellerOf(ctx, q.DatasetID)
    if err != nil {
        return Answer{}, ErrQuestionNotFound
    }
    if sellerID != answererID {
        return Answer{}, ErrNotSeller
    }
    // 已有 answer 拒绝(一问一答,改用编辑接口 — 不在本 PR 范围)
    if q.Answer != nil {
        return Answer{}, ErrAlreadyAnswered
    }
    a, err := s.repo.CreateAnswer(ctx, Answer{
        QuestionID: questionID, AnswererID: answererID, Body: body,
    })
    if err != nil {
        return Answer{}, err
    }
    _ = s.repo.SetQuestionStatus(ctx, questionID, "answered")
    // 通知提问者
    if s.notifier != nil && q.AskerID != answererID {
        _ = s.notifier.NotifyUser(ctx, q.AskerID, "question_answered",
            "您的提问已被回答", "卖家已回答您关于数据集的提问。",
            "dataset", q.DatasetID)
    }
    return a, nil
}
```

#### Service.ListByDataset

```go
func (s *Service) ListByDataset(ctx context.Context, datasetID string, limit, offset int) ([]Question, error) {
    if limit <= 0 || limit > 100 { limit = 50 }
    if offset < 0 { offset = 0 }
    return s.repo.ListByDataset(ctx, datasetID, limit, offset)
}
```

### O.2.4 Handler + 路由

```
GET    /datasets/:id/questions?limit=&offset=    (公开)
POST   /datasets/:id/questions                    (authed: body={body})
POST   /questions/:id/answer                      (authed seller-only: body={body})
```

**route 挂载位置**(参考 `dataset/router.go`):
- `GET /datasets/:id/questions` 挂在公开组(`rg.GET`)
- `POST` 两条挂在 `authed` 组

### O.2.5 Server.go 接线

```go
// 紧挨着 watchlist 注册后:
qaRepo := qa.NewRepository(s.db)
qaSvc := qa.NewService(qaRepo, qaDatasetAdapter{ds: dsSvc}, notifySvc)
qa.Register(api, qaSvc, authMW)

// 适配器:
type qaDatasetAdapter struct{ ds *dataset.Service }
func (a qaDatasetAdapter) SellerOf(ctx context.Context, datasetID string) (string, string, error) {
    d, err := a.ds.Get(ctx, datasetID)
    if err != nil { return "", "", err }
    return d.SellerID, d.Status, nil
}
```

## O.3 前端

### O.3.1 新组件 `frontend/components/DatasetQA.tsx`

**状态形状**(state shape — DeepSeek 文本友好版):

```ts
interface DatasetQAState {
    questions: Question[];
    askBody: string;
    answerBodies: Record<string, string>;  // questionID → 卖家回答输入框
    busy: boolean;
    err: string;
}
```

**行为表**(behavior table — 替代截图):

| 用户类型 | 看到 | 能操作 |
|---|---|---|
| 未登录访客 | 所有问答列表 | 跳转登录后才能提问 |
| 已登录买家(非卖家) | 所有问答列表 + 提问输入框 | 提问 |
| 已登录卖家(本数据集) | 所有问答列表 + 提问输入框(自己也能问) + 每条未答问题的「回答」输入框 + 提交按钮 | 提问 / 回答 |

**渲染骨架**:

```tsx
<Card>
    <h3>{t("数据集问答", "Dataset Q&A")}</h3>

    {/* 提问表单(authed only) */}
    {user && (
        <div>
            <Textarea value={askBody} placeholder={t("有问题想问卖家?", "Ask the seller a question...")} />
            <Button onClick={submitAsk}>{t("发布提问", "Post question")}</Button>
        </div>
    )}
    {!user && <Alert>{t("登录后提问", "Sign in to ask")}</Alert>}

    {/* 问答列表 */}
    <ul>
        {questions.map(q => (
            <li key={q.id}>
                <div>
                    <span>{q.asker_name}</span> · <span>{q.created_at}</span>
                </div>
                <p>{q.body}</p>
                {q.answer ? (
                    <div className="ml-4 border-l-2 pl-3">
                        <span>{t("卖家回答", "Seller answered")} · {q.answer.created_at}</span>
                        <p>{q.answer.body}</p>
                    </div>
                ) : isSeller && (
                    <div>
                        <Textarea value={answerBodies[q.id] || ""}
                                  onChange={e => updateAnswer(q.id, e.target.value)}
                                  placeholder={t("回答这个问题...", "Answer this question...")} />
                        <Button onClick={() => submitAnswer(q.id)}>{t("回答", "Reply")}</Button>
                    </div>
                )}
            </li>
        ))}
    </ul>
</Card>
```

**挂载位置**:`frontend/app/datasets/[id]/page.tsx` 详情页底部,在 quality 报告之后。

### O.3.2 `lib/api.ts` 加方法

```ts
export type DatasetQuestion = {
    id: string;
    dataset_id: string;
    asker_id: string;
    asker_name?: string;
    body: string;
    status: string;
    answer?: { id: string; answerer_id: string; body: string; created_at: string };
    created_at: string;
};

// 加方法:
listDatasetQuestions: (id: string, limit?: number, offset?: number) =>
    request<{ items: DatasetQuestion[] }>(`/datasets/${id}/questions`, {
        query: { limit, offset }, auth: false
    }),
askDatasetQuestion: (id: string, body: string) =>
    request<DatasetQuestion>(`/datasets/${id}/questions`, { body: { body } }),
answerQuestion: (qid: string, body: string) =>
    request<{ id: string; question_id: string; body: string; created_at: string }>(
        `/questions/${qid}/answer`, { body: { body } }),
```

### O.3.3 通知页 kind 渲染

`frontend/app/notifications/page.tsx` 加两条 kind 翻译:
- `question_asked` → 「数据集有新提问」/`New question on your dataset`
- `question_answered` → 「您的提问已被回答」/`Your question was answered`

## O.4 测试(**必须 12 个**,**测试名不许改**)

**新文件** `backend/internal/modules/qa/repo_test.go`(用 `db.RunMigrations`):

```go
func TestCreateQuestion_PersistsAndReturnsID(t *testing.T)
func TestCreateAnswer_LinksToQuestion(t *testing.T)
func TestListByDataset_OrdersByCreatedAtDesc(t *testing.T)
func TestListByDataset_ExcludesHidden(t *testing.T)
//   插 3 条 questions,1 条 status='hidden' → List 只返回 2

func TestListByDataset_AttachesAnswerWhenPresent(t *testing.T)
//   q1 有 answer,q2 没有 → q1.Answer 非 nil,q2.Answer == nil

func TestListByDataset_AskerNameIsPrefixOfAccount(t *testing.T)
//   asker.account = "alice@example.com" → q.AskerName == "alice@ex"(前 8 位)

func TestSetQuestionStatus_LeavesHiddenAlone(t *testing.T)
//   status='hidden' 的 q,SetQuestionStatus(qid, 'answered') 不生效(状态保持 hidden)
```

**新文件** `backend/internal/modules/qa/service_test.go`(用 fakes):

```go
type fakeQARepo struct {
    questions map[string]Question
    answers   map[string]Answer
}
// 实现 Repository 接口

type fakeDSReader struct {
    sellers map[string]string  // datasetID → sellerID
    status  map[string]string  // datasetID → status
}

type fakeQANotifier struct { calls []notifyCall }

func TestAskQuestion_RejectsEmptyBody(t *testing.T)
//   svc.AskQuestion(_, _, "") → ErrEmptyBody

func TestAskQuestion_RejectsBodyOver2000(t *testing.T)
//   body = strings.Repeat("x", 2001) → ErrBodyTooLong

func TestAskQuestion_RejectsDraftDataset(t *testing.T)
//   fakeDSReader 返回 status='draft' → ErrQuestionNotFound

func TestAskQuestion_NotifiesSeller(t *testing.T)
//   成功后 notifier.calls 长度=1, kind="question_asked", UserID=sellerID
//   且 sellerID != askerID

func TestAskQuestion_DoesNotNotifySelfWhenAskerIsSeller(t *testing.T)
//   askerID == sellerID → notifier.calls 长度=0

func TestAnswerQuestion_RejectsNonSellerAnswerer(t *testing.T)
//   answererID != sellerID → ErrNotSeller(IDOR 防护回归)
```

## O.5 我会查的(每个 spec 项打钩)

- [ ] 迁移文件 `000017_dataset_qa.up.sql` + `down.sql` 都在
- [ ] `Service.AskQuestion` 检查 dataset status,non-published/reviewing → `ErrQuestionNotFound`(不暴露存在性)
- [ ] `Service.AnswerQuestion` 检查 `sellerID != answererID → ErrNotSeller`(IDOR)
- [ ] `Service.AskQuestion` 当 `askerID == sellerID` 时**不**给自己发通知
- [ ] `Service.AnswerQuestion` 当 `answererID == askerID`(理论上不可能但守一手)时不通知自己
- [ ] 通知 emit 全部 `nil` 守卫 + `_ =` 吞错
- [ ] `listByDataset` SQL 用 `LEFT JOIN dataset_answers`(不是 INNER)
- [ ] `asker_name = SUBSTRING(u.account, 1, 8)` 脱敏
- [ ] body 长度上限 2000(前后都校验)
- [ ] CLAUDE.md 单独 commit 追加 1 条 gotcha(本 PR 学到的真坑)

## O.6 不许做

| ❌ | 原因 |
|---|---|
| 给 question 加 edit / delete 接口 | 本 PR 范围:一问一答,编辑/删除留下次 |
| 让买家 reply seller's answer(threaded) | YAGNI |
| 加 rich text / markdown 渲染 | 纯文本 + autolink 由前端 utility 处理足够 |
| 引入 Q&A 上的投票 / 点赞 | 不在范围 |

---

# PR-P · 卖家提现申请 + ops 审批

## P.0 现状(必读)

- `order/SellerEarnings` (`backend/internal/modules/order/repo.go`) 已经能算 settled 总额
- 没有提现概念 — 卖家 settled 钱卡在「账上」**实际**没出账
- 真支付分账(微信/支付宝)= 二清红线,**本 PR 不碰**

**真缺口**:卖家想知道「我的钱什么时候到账」、ops 想有正式审批流程

**架构原则**:这不是真分账,是**记账系统**(book-keeping)。状态机 + 审计 + ops 操作面板。真实银行转账由 ops 线下完成,系统**只记录**。

## P.1 数据模型

**新迁移** `backend/migrations/000018_withdrawals.up.sql`:

```sql
-- 000018: seller withdrawal requests (book-keeping; bank transfer is off-system).
CREATE TABLE IF NOT EXISTS withdrawal_requests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    seller_id       UUID NOT NULL REFERENCES users(id),
    amount_cents    BIGINT NOT NULL CHECK (amount_cents > 0),
    channel         TEXT NOT NULL,  -- 'wechat' | 'alipay' | 'bank' (display label only)
    account_label   TEXT NOT NULL,  -- 卖家提供的账户标签(脱敏后)
    status          TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'approved', 'completed', 'rejected')),
    ops_note        TEXT,
    requested_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at    TIMESTAMPTZ,
    processed_by    UUID REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_withdrawals_seller
    ON withdrawal_requests (seller_id, requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_withdrawals_pending
    ON withdrawal_requests (status, requested_at) WHERE status IN ('pending', 'approved');
```

## P.2 模块结构 `backend/internal/modules/withdrawal/`

```
withdrawal/
  model.go    (Request DTO + 状态常量 + 错误哨兵)
  repo.go     (Repository + pgRepo)
  service.go  (Service + EarningsReader 接口 + Notifier 接口)
  handler.go  (Register + 5 个 handler)
```

### P.2.1 DTO + 状态

```go
type Request struct {
    ID            string `json:"id"`
    SellerID      string `json:"seller_id"`
    AmountCents   int64  `json:"amount_cents"`
    Channel       string `json:"channel"`
    AccountLabel  string `json:"account_label"`
    Status        string `json:"status"`
    OpsNote       string `json:"ops_note,omitempty"`
    RequestedAt   string `json:"requested_at"`
    ProcessedAt   string `json:"processed_at,omitempty"`
    ProcessedBy   string `json:"processed_by,omitempty"`
}

const (
    StatusPending   = "pending"
    StatusApproved  = "approved"
    StatusCompleted = "completed"
    StatusRejected  = "rejected"
)

var (
    ErrInsufficientBalance = errors.New("insufficient settled balance")
    ErrBadTransition       = errors.New("illegal status transition")
    ErrNotFound            = errors.New("withdrawal not found")
    ErrForbidden           = errors.New("not your withdrawal")
    ErrAmountInvalid       = errors.New("amount must be > 0 and <= 1,000,000 yuan")
    ErrChannelInvalid      = errors.New("channel must be wechat|alipay|bank")
)
```

### P.2.2 状态机

```
pending ─→ approved ─→ completed   (正常路径)
   ↓         ↓
rejected   rejected                 (任何阶段可拒)
```

**乐观状态转移** SQL 模式:`UPDATE … WHERE status = $from RETURNING …`,0 行 ⇒ `ErrBadTransition`。

### P.2.3 Repository 接口 + 关键 SQL

```go
type Repository interface {
    Create(ctx context.Context, r Request) (Request, error)
    Get(ctx context.Context, id string) (Request, error)
    ListBySeller(ctx context.Context, sellerID string, limit, offset int) ([]Request, error)
    AdminList(ctx context.Context, status string, limit, offset int) ([]Request, error)
    Transition(ctx context.Context, id, from, to, opsID, note string) (Request, error)

    // SumApprovedAndPending 返回卖家所有 pending+approved 提现的累计金额
    // 用于「剩余可申请额度 = settled - 这个值」的计算。
    SumApprovedAndPending(ctx context.Context, sellerID string) (int64, error)
}
```

**`Transition`** SQL(乐观状态机 + 审计字段):

```sql
UPDATE withdrawal_requests
SET status = $3,
    ops_note = $5,
    processed_at = now(),
    processed_by = $4::uuid
WHERE id = $1::uuid AND status = $2
RETURNING id::text, seller_id::text, amount_cents, channel, account_label, status,
          COALESCE(ops_note,''), requested_at::text, COALESCE(processed_at::text,''),
          COALESCE(processed_by::text,'')
```

0 行 ⇒ `ErrBadTransition`。

**`SumApprovedAndPending`** SQL:

```sql
SELECT COALESCE(SUM(amount_cents), 0)
FROM withdrawal_requests
WHERE seller_id = $1 AND status IN ('pending', 'approved')
```

### P.2.4 Service(关键业务逻辑)

```go
type EarningsReader interface {
    SettledCentsOf(ctx context.Context, sellerID string) (int64, error)
}

type Notifier interface {
    NotifyUser(ctx context.Context, userID, kind, title, body, resourceType, resourceID string) error
}

type Service struct {
    repo     Repository
    earnings EarningsReader
    notifier Notifier
}
```

#### Service.Request (卖家发起提现)

```go
func (s *Service) Request(ctx context.Context, sellerID string, amountCents int64, channel, accountLabel string) (Request, error) {
    if amountCents <= 0 || amountCents > 100_000_000 {  // 100w cents = 1,000,000 yuan 单笔上限
        return Request{}, ErrAmountInvalid
    }
    if channel != "wechat" && channel != "alipay" && channel != "bank" {
        return Request{}, ErrChannelInvalid
    }
    if strings.TrimSpace(accountLabel) == "" || len(accountLabel) > 200 {
        return Request{}, ErrAmountInvalid  // 复用,或新增 ErrAccountInvalid
    }

    settled, err := s.earnings.SettledCentsOf(ctx, sellerID)
    if err != nil { return Request{}, err }

    pending, err := s.repo.SumApprovedAndPending(ctx, sellerID)
    if err != nil { return Request{}, err }

    available := settled - pending
    if amountCents > available {
        return Request{}, ErrInsufficientBalance
    }

    r, err := s.repo.Create(ctx, Request{
        SellerID: sellerID, AmountCents: amountCents, Channel: channel,
        AccountLabel: accountLabel, Status: StatusPending,
    })
    return r, err
}
```

#### Service.Approve / Reject / Complete (ops)

```go
func (s *Service) Approve(ctx context.Context, opsID, id, note string) (Request, error) {
    r, err := s.repo.Transition(ctx, id, StatusPending, StatusApproved, opsID, note)
    if err != nil { return Request{}, err }
    s.notify(ctx, r, "withdrawal_approved", "提现申请已批准", "ops 已批准您的提现申请,等待打款。")
    return r, nil
}

func (s *Service) Reject(ctx context.Context, opsID, id, reason string) (Request, error) {
    // 只能从 pending 拒(approved 后就该走 complete 或者新发起,简化)
    r, err := s.repo.Transition(ctx, id, StatusPending, StatusRejected, opsID, reason)
    if err != nil { return Request{}, err }
    s.notify(ctx, r, "withdrawal_rejected", "提现申请被拒", "您的提现申请被拒:"+reason)
    return r, nil
}

func (s *Service) Complete(ctx context.Context, opsID, id, note string) (Request, error) {
    r, err := s.repo.Transition(ctx, id, StatusApproved, StatusCompleted, opsID, note)
    if err != nil { return Request{}, err }
    s.notify(ctx, r, "withdrawal_completed", "提现已到账", "您的提现已完成,金额 ¥"+yuan(r.AmountCents)+"。")
    return r, nil
}

func (s *Service) notify(ctx context.Context, r Request, kind, title, body string) {
    if s.notifier != nil {
        _ = s.notifier.NotifyUser(ctx, r.SellerID, kind, title, body, "withdrawal", r.ID)
    }
}

func yuan(cents int64) string {
    return strconv.FormatFloat(float64(cents)/100.0, 'f', 2, 64)
}
```

#### Service.ListMy / AdminList

```go
func (s *Service) ListMy(ctx context.Context, sellerID string, limit, offset int) ([]Request, error) {
    if limit <= 0 || limit > 100 { limit = 50 }
    return s.repo.ListBySeller(ctx, sellerID, limit, offset)
}

func (s *Service) AdminList(ctx context.Context, status string, limit, offset int) ([]Request, error) {
    if limit <= 0 || limit > 100 { limit = 50 }
    return s.repo.AdminList(ctx, status, limit, offset)
}
```

### P.2.5 Handler + 路由

```
authed:
    POST   /sellers/me/withdrawals                 (Body: {amount_cents, channel, account_label})
    GET    /sellers/me/withdrawals?limit=&offset=  
ops:
    GET    /admin/withdrawals?status=&limit=&offset=
    POST   /admin/withdrawals/:id/approve  (Body: {note?})
    POST   /admin/withdrawals/:id/reject   (Body: {reason})  ← reason 必填
    POST   /admin/withdrawals/:id/complete (Body: {note?})
```

`reason` 必填的校验在 handler 层 + `fail(c, ErrValidation)`。

### P.2.6 Server.go 接线

```go
withdrawRepo := withdrawal.NewRepository(s.db)
withdrawSvc := withdrawal.NewService(withdrawRepo, withdrawEarningsAdapter{order: orderSvc}, notifySvc)
withdrawal.Register(api, withdrawSvc, authMW, auth.RequireRole("ops", "admin"))

type withdrawEarningsAdapter struct{ order *order.Service }
func (a withdrawEarningsAdapter) SettledCentsOf(ctx context.Context, sellerID string) (int64, error) {
    e, err := a.order.SellerEarnings(ctx, sellerID)
    if err != nil { return 0, err }
    return e.SettledCents, nil  // 看现有 Earnings struct 字段名,可能是 Settled / TotalSettled — 实际查证
}
```

## P.3 前端

### P.3.1 新组件 `frontend/components/WithdrawalCard.tsx`

**行为表**:

| 用户 | 看到 | 操作 |
|---|---|---|
| 卖家 | 当前 settled / 已申请 / 可申请额度 + 历史申请列表 + 「申请提现」按钮 | 填表单(amount, channel, account_label)+ submit |
| ops(admin 页) | 全部申请列表 + 按 status 过滤 + 每条上的「批准 / 拒 / 完成」按钮 | 三个操作各对应一个 modal/inline form |

**挂载**:
- 卖家:`frontend/app/account/page.tsx` 在 `<SellerAnalytics />` 之后
- ops:`frontend/app/admin/page.tsx` 加第 7 Tab「提现审批」(`Tab` union 加 `"withdraw"`)

### P.3.2 `lib/api.ts`

```ts
export type Withdrawal = {
    id: string; seller_id: string; amount_cents: number;
    channel: string; account_label: string; status: string;
    ops_note?: string; requested_at: string;
    processed_at?: string; processed_by?: string;
};

// 卖家:
requestWithdrawal: (b: { amount_cents: number; channel: string; account_label: string }) =>
    request<Withdrawal>("/sellers/me/withdrawals", { body: b }),
listMyWithdrawals: (limit?: number, offset?: number) =>
    request<{ items: Withdrawal[] }>("/sellers/me/withdrawals", { query: { limit, offset } }),

// ops:
adminListWithdrawals: (status?: string, limit?: number, offset?: number) =>
    request<{ items: Withdrawal[] }>("/admin/withdrawals", { query: { status, limit, offset } }),
adminApproveWithdrawal: (id: string, note?: string) =>
    request<Withdrawal>(`/admin/withdrawals/${id}/approve`, { body: { note } }),
adminRejectWithdrawal: (id: string, reason: string) =>
    request<Withdrawal>(`/admin/withdrawals/${id}/reject`, { body: { reason } }),
adminCompleteWithdrawal: (id: string, note?: string) =>
    request<Withdrawal>(`/admin/withdrawals/${id}/complete`, { body: { note } }),
```

### P.3.3 通知 kind 渲染

`frontend/app/notifications/page.tsx` 加 3 条:
- `withdrawal_approved` → 「提现已批准」/`Withdrawal approved`
- `withdrawal_completed` → 「提现已到账」/`Withdrawal completed`
- `withdrawal_rejected` → 「提现被拒」/`Withdrawal rejected`

## P.4 测试(**必须 11 个**)

**`backend/internal/modules/withdrawal/repo_test.go`**(用 `db.RunMigrations`):

```go
func TestCreate_PersistsRequest(t *testing.T)
func TestTransition_PendingToApproved(t *testing.T)
func TestTransition_ApprovedToCompleted(t *testing.T)
func TestTransition_PendingToRejected(t *testing.T)
func TestTransition_FromCompletedReturnsErrBadTransition(t *testing.T)
//   completed → approved → 0 行影响 → ErrBadTransition

func TestSumApprovedAndPending_ExcludesRejectedAndCompleted(t *testing.T)
//   插 pending=100, approved=200, rejected=300, completed=400 → sum = 300
```

**`backend/internal/modules/withdrawal/service_test.go`**(用 fakes):

```go
type fakeEarnings struct{ settled int64 }
func (f *fakeEarnings) SettledCentsOf(ctx context.Context, sellerID string) (int64, error) {
    return f.settled, nil
}

func TestRequest_RejectsAmountExceedingAvailable(t *testing.T)
//   settled=1000, pending=500, available=500;Request(700) → ErrInsufficientBalance

func TestRequest_AcceptsAmountAtAvailable(t *testing.T)
//   settled=1000, pending=500, available=500;Request(500) → 成功

func TestRequest_RejectsInvalidChannel(t *testing.T)
func TestRequest_RejectsZeroOrNegativeAmount(t *testing.T)
func TestApprove_NotifiesSellerAndNotOps(t *testing.T)
//   fakeNotifier.calls 长度=1, UserID=sellerID(不是 opsID)
```

## P.5 我会查的

- [ ] 迁移 000018 含 `CHECK (amount_cents > 0)` + status CHECK
- [ ] 部分索引 `WHERE status IN ('pending', 'approved')`(查待办高效)
- [ ] `Service.Request` 计算 available = settled - SumApprovedAndPending,**rejected 和 completed 不算**
- [ ] 状态机:pending→approved→completed,任何阶段→rejected,**不**支持 reverse
- [ ] `Reject` body 含 `reason`,且 reason 校验非空
- [ ] 通知发给 `r.SellerID`,**不是** opsID
- [ ] 所有 admin handler 经 `auth.RequireRole("ops", "admin")`
- [ ] 单笔上限 100w cents(1,000,000 元)
- [ ] CLAUDE.md 单独 commit 追加 1 条 gotcha

## P.6 不许做

| ❌ | 原因 |
|---|---|
| 接微信/支付宝实际转账 API | **二清红线** — 本 PR 纯记账,真转账线下做 |
| 加自动审批逻辑(比如 < ¥1000 自动过) | ops 必须人工审 |
| 让 settled 钱「扣减」(改 order 表) | settled 是不可变事实,withdrawal 只是申请记录 |
| 让卖家撤销已 approved 的申请 | ops 已经在跑账,不该卖家撤销;要撤改用 ops reject |
| 在卖家端显示具体 ops 操作员 id | `processed_by` 仅 ops 自己看 |

---

# PR-Q · 审计日志异常告警

## Q.0 现状(必读)

- PR #77 加了 `auditlog` 模块只读 viewer + admin tab #6「审计日志」
- `audit_logs` 表 append-only,30+ 调用点
- **缺口**:ops 要主动盯异常(频繁失败/批量改/敏感动作)— 现状全靠肉眼翻日志

## Q.1 数据模型

**新迁移** `backend/migrations/000019_audit_anomalies.up.sql`:

```sql
-- 000019: audit anomaly detections (computed periodically; ops triage).
CREATE TABLE IF NOT EXISTS audit_anomalies (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind            TEXT NOT NULL,  -- 'repeated_failure' | 'bulk_modification' | 'high_risk_action'
    actor_id        UUID,           -- 可空(高敏感动作可能 actor=null)
    resource_pattern TEXT NOT NULL,  -- e.g. "dataset" or "compute_job:<prefix>"
    sample_audit_ids BIGINT[] NOT NULL,  -- 触发的 audit_logs.id 样本(最多 5 个)
    count           INT NOT NULL,
    first_seen_at   TIMESTAMPTZ NOT NULL,
    last_seen_at    TIMESTAMPTZ NOT NULL,
    status          TEXT NOT NULL DEFAULT 'open'
                        CHECK (status IN ('open', 'acknowledged', 'resolved')),
    ops_note        TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 同一 actor + kind + 24h 内不重复(扫描器 upsert key)
CREATE UNIQUE INDEX IF NOT EXISTS uq_audit_anomalies_dedup
    ON audit_anomalies (kind, actor_id, resource_pattern, DATE(first_seen_at));
CREATE INDEX IF NOT EXISTS idx_audit_anomalies_open
    ON audit_anomalies (status, last_seen_at DESC) WHERE status = 'open';
```

## Q.2 模块结构 `backend/internal/modules/anomaly/`

```
anomaly/
  model.go    (Anomaly DTO + Rule 接口 + 错误哨兵)
  rules.go    (3 个内置规则:RepeatedFailure / BulkModification / HighRiskAction)
  repo.go     (Repository + pgRepo)
  service.go  (Service + ScanRecent 入口)
  scanner.go  (后台 goroutine 每 5 分钟扫一次)
  handler.go  (Register + 3 个 handler: list / acknowledge / resolve)
```

### Q.2.1 规则定义

```go
type Rule interface {
    Kind() string
    // Detect 扫描最近 since 时间窗口的 audit_logs,返回触发的异常列表。
    Detect(ctx context.Context, db DBQuerier, since time.Time) ([]Anomaly, error)
}

// DBQuerier 是 pgxpool.Pool 的轻包装,便于测试 mock。
type DBQuerier interface {
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}
```

#### Rule 1: RepeatedFailure

**定义**:同一 actor 在 1 小时窗口内 >=10 次 `action` 以 `fail` / `reject` / `error` 结尾的操作。

SQL:
```sql
SELECT actor_id::text, action, COUNT(*) as cnt,
       MIN(created_at) as first_at, MAX(created_at) as last_at,
       ARRAY_AGG(id ORDER BY created_at DESC LIMIT 5) as sample_ids
FROM audit_logs
WHERE created_at >= $1
  AND (action LIKE '%.reject' OR action LIKE '%.fail' OR action LIKE '%.error')
  AND actor_id IS NOT NULL
GROUP BY actor_id, action
HAVING COUNT(*) >= 10
```

#### Rule 2: BulkModification

**定义**:同一 actor 在 5 分钟窗口内对**不同** resource_id 做相同 action ≥20 次。

SQL:
```sql
SELECT actor_id::text, action, resource_type,
       COUNT(DISTINCT resource_id) as cnt,
       MIN(created_at) as first_at, MAX(created_at) as last_at,
       ARRAY_AGG(id ORDER BY created_at DESC LIMIT 5) as sample_ids
FROM audit_logs
WHERE created_at >= $1
  AND actor_id IS NOT NULL
  AND resource_id IS NOT NULL
GROUP BY actor_id, action, resource_type
HAVING COUNT(DISTINCT resource_id) >= 20
```

#### Rule 3: HighRiskAction

**定义**:特定 action(无论次数)需关注。白名单:
- `dataset.reject`
- `kyc.reject`
- `withdrawal.reject`(PR-P 加的)
- `dataset.delist`

每次出现都登记一条(去重靠 unique index)。

SQL:
```sql
SELECT actor_id::text, action, resource_type, resource_id,
       1 as cnt, created_at as first_at, created_at as last_at,
       ARRAY[id] as sample_ids
FROM audit_logs
WHERE created_at >= $1
  AND action IN ('dataset.reject', 'kyc.reject', 'withdrawal.reject', 'dataset.delist')
```

### Q.2.2 Repository

```go
type Repository interface {
    Upsert(ctx context.Context, a Anomaly) error  // unique 索引会防重,更新 count + last_seen
    List(ctx context.Context, status string, limit, offset int) ([]Anomaly, error)
    Get(ctx context.Context, id string) (Anomaly, error)
    SetStatus(ctx context.Context, id, status, opsID, note string) error
}
```

**Upsert** SQL(ON CONFLICT 更新 count + last_seen):

```sql
INSERT INTO audit_anomalies (kind, actor_id, resource_pattern, sample_audit_ids, count,
                              first_seen_at, last_seen_at, status)
VALUES ($1, $2::uuid, $3, $4::bigint[], $5, $6, $7, 'open')
ON CONFLICT (kind, actor_id, resource_pattern, DATE(first_seen_at)) DO UPDATE
SET count = EXCLUDED.count,
    last_seen_at = GREATEST(audit_anomalies.last_seen_at, EXCLUDED.last_seen_at),
    sample_audit_ids = EXCLUDED.sample_audit_ids,
    updated_at = now()
WHERE audit_anomalies.status = 'open'  -- acknowledged/resolved 不被新触发更新
```

### Q.2.3 Scanner

```go
type Service struct {
    repo  Repository
    rules []Rule
    db    DBQuerier
}

func NewService(repo Repository, db DBQuerier) *Service {
    return &Service{
        repo: repo,
        db:   db,
        rules: []Rule{
            &RepeatedFailureRule{},
            &BulkModificationRule{},
            &HighRiskActionRule{},
        },
    }
}

// ScanOnce 跑一轮扫描,返回新增/更新的 anomaly 数。便于测试。
func (s *Service) ScanOnce(ctx context.Context) (int, error) {
    since := time.Now().Add(-1 * time.Hour)
    total := 0
    for _, rule := range s.rules {
        anomalies, err := rule.Detect(ctx, s.db, since)
        if err != nil {
            slog.Warn("anomaly rule failed", "kind", rule.Kind(), "err", err)
            continue
        }
        for _, a := range anomalies {
            if err := s.repo.Upsert(ctx, a); err != nil {
                slog.Warn("anomaly upsert failed", "kind", a.Kind, "err", err)
            }
            total++
        }
    }
    return total, nil
}

// StartScanner 后台 5 分钟一轮。
func (s *Service) StartScanner(ctx context.Context) {
    go func() {
        ticker := time.NewTicker(5 * time.Minute)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                _, _ = s.ScanOnce(ctx)
            }
        }
    }()
}
```

### Q.2.4 Handler 路由(全部 ops gated)

```
GET    /admin/anomalies?status=&limit=&offset=
POST   /admin/anomalies/:id/acknowledge   (Body: {note?})
POST   /admin/anomalies/:id/resolve       (Body: {note?})
```

### Q.2.5 Server.go 接线

```go
anomalyRepo := anomaly.NewRepository(s.db)
anomalySvc := anomaly.NewService(anomalyRepo, s.db)
anomalySvc.StartScanner(context.Background())  // 永续 ctx
anomaly.Register(api, anomalySvc, auth.RequireRole("ops", "admin"))
```

## Q.3 前端

admin 页加第 7 Tab「异常告警」(`Tab` union 加 `"anomaly"`):

**显示规则**:
- 默认显示 `status=open`,toggle 切到 `acknowledged` / `resolved` / 全部
- 每条:kind badge + actor 简码(account 前 8 位)+ resource_pattern + count + last_seen
- 每条上的「确认」「解决」按钮 + note 输入

`lib/api.ts`:
```ts
export type Anomaly = {
    id: string; kind: string; actor_id?: string;
    resource_pattern: string; sample_audit_ids: number[];
    count: number; first_seen_at: string; last_seen_at: string;
    status: string; ops_note?: string;
};

adminListAnomalies: (status?: string, limit?: number, offset?: number) =>
    request<{ items: Anomaly[] }>("/admin/anomalies", { query: { status, limit, offset } }),
adminAcknowledgeAnomaly: (id: string, note?: string) =>
    request<Anomaly>(`/admin/anomalies/${id}/acknowledge`, { body: { note } }),
adminResolveAnomaly: (id: string, note?: string) =>
    request<Anomaly>(`/admin/anomalies/${id}/resolve`, { body: { note } }),
```

## Q.4 测试(**必须 9 个**)

**`backend/internal/modules/anomaly/rules_test.go`**(用 mock DBQuerier 或真 PG):

```go
func TestRepeatedFailureRule_BelowThreshold_NoDetection(t *testing.T)
//   9 条 fail audit_logs → Detect 返回 0

func TestRepeatedFailureRule_AtThreshold_Detected(t *testing.T)
//   10 条 fail → 返回 1 条 Anomaly,count=10,kind="repeated_failure"

func TestBulkModificationRule_DetectsDistinctResources(t *testing.T)
//   20 个 distinct resource_id 同 actor 同 action → 检出

func TestHighRiskActionRule_EachOccurrenceLogged(t *testing.T)
//   `dataset.reject` 出现 3 次 → 返回 3 条 Anomaly
```

**`backend/internal/modules/anomaly/service_test.go`**(用 fakes):

```go
type fakeRule struct {
    kind     string
    output   []Anomaly
    callErr  error
}
// 实现 Rule 接口

type fakeAnomalyRepo struct {
    upserted []Anomaly
    upsertErr error
}

func TestScanOnce_RunsAllRulesAndUpsertsResults(t *testing.T)
//   2 个 fakeRule 各返回 1 条 → repo.upserted 长度=2

func TestScanOnce_RuleFailureDoesNotBlockOthers(t *testing.T)
//   rule1 返回 callErr → rule2 仍执行,upserted 含 rule2 的结果

func TestSetStatus_TransitionsOpenToAcknowledged(t *testing.T)

func TestSetStatus_RejectsInvalidStatus(t *testing.T)
//   传 status="foo" → 应该 reject(handler 层校验)

func TestUpsert_DoesNotOverrideResolvedAnomaly(t *testing.T)
//   插一条 status=resolved 的 anomaly,再 Upsert 同样 key → 行**不变**(WHERE status='open')
```

## Q.5 我会查的

- [ ] 3 个 Rule 都实现 `Rule` 接口
- [ ] `Upsert` 用 `ON CONFLICT (kind, actor_id, resource_pattern, DATE(first_seen_at))`
- [ ] `ON CONFLICT DO UPDATE WHERE status = 'open'` — acknowledged/resolved 不被覆盖
- [ ] Scanner 用 5 分钟 ticker + `ctx.Done()` 退出
- [ ] **不**读 `audit_anomalies` 的工作队列 channel(教训 PR-J)
- [ ] 所有 admin handler 经 `auth.RequireRole("ops", "admin")`
- [ ] `sample_audit_ids` 限 5 个(SQL `LIMIT 5`)
- [ ] CLAUDE.md 单独 commit 追加 1 条 gotcha

## Q.6 不许做

| ❌ | 原因 |
|---|---|
| 把异常用 webhook 推到外部 | 范围外,先做 ops 看板 |
| 让规则可由 ops 配置(rule engine) | YAGNI,3 个硬编码规则够 |
| 让 anomaly auto-resolve(过 X 天) | 必须 ops 手动 |
| 给 anomaly 加 audit_logs 写入路径 | 用现有 `s.audit.Record` 即可(在 SetStatus 处) |

---

# PR-R · 测试覆盖回灌(PR #72 的 ops/payment/order/search 模块)

## R.0 现状(必读)

PR #72 加了大量 ops 端点和 search 模块,但**测试覆盖**:
- `payment/ListOutbox` `RetryOutbox` → 仅 stub fake,无 repo 集成测试
- `order/AdminReconciliation` → 仅 fakeRepo,无 repo 集成测试
- `search/handler.go` → 0 测试

PR-R 把这些补齐。**不改业务代码**,只加测试。

## R.1 新增/扩展测试文件

### R.1.1 `backend/internal/modules/payment/outbox_repo_test.go`(新文件)

**必须 4 个**,用 `db.RunMigrations`:

```go
func TestPgOutbox_ListOutbox_FiltersByStatus(t *testing.T)
//   插 5 条:3 failed + 2 pending → ListOutbox(status="failed") 返回 3

func TestPgOutbox_ListOutbox_OrdersByUpdatedAtDesc(t *testing.T)

func TestPgOutbox_RetryOutbox_FailedToPending(t *testing.T)
//   插 failed → Retry → status=pending

func TestPgOutbox_RetryOutbox_NonFailedReturnsErrOutboxNotFailed(t *testing.T)
//   插 pending → Retry → ErrOutboxNotFailed
```

### R.1.2 `backend/internal/modules/order/admin_reconciliation_repo_test.go`(新文件)

**必须 5 个**,用 `db.RunMigrations`(参考 timeseries_test 的 seedUser/seedDataset):

```go
func TestPgRepo_AdminReconciliation_EmptyDatabase(t *testing.T)
//   无 orders → 全部 0

func TestPgRepo_AdminReconciliation_AggregatesAcrossStatuses(t *testing.T)
//   插 settled / refunded / pending / disputed 各 1 条 → 各计数 = 1

func TestPgRepo_AdminReconciliation_FailedSettlements_ToleratesAbsentTable(t *testing.T)
//   drop settlement_outbox 不存在的场景,AdminReconciliation 不 crash,FailedSettlements=0
//   ⚠️ 这里**不许 DROP TABLE**(我会 grep);用 TRUNCATE 或 conditional skip

func TestPgRepo_AdminReconciliation_PlatformFeesOnlyCountSettled(t *testing.T)
//   插 paid (platform_fee=100) + settled (platform_fee=200) → PlatformFees=200

func TestPgRepo_AdminReconciliation_RefundedAmountSumsCorrectly(t *testing.T)
```

### R.1.3 `backend/internal/modules/search/handler_test.go`(新文件)

**必须 5 个**,用 fake DatasetSearcher:

```go
type fakeSearcher struct {
    searches []search.SearchQuery
    results  []search.SearchResult
    err      error
}
func (f *fakeSearcher) SearchPublished(ctx context.Context, q search.SearchQuery) ([]search.SearchResult, error) {
    f.searches = append(f.searches, q)
    return f.results, f.err
}

func TestSearchHandler_PassesQueryStringToSearcher(t *testing.T)
//   GET /search?q=hello → fakeSearcher.searches[0].Q == "hello"

func TestSearchHandler_PassesAllFilters(t *testing.T)
//   GET /search?q=x&type=text&domain=finance&sort=newest&limit=20&offset=10
//   → 全部参数正确传入

func TestSearchHandler_LimitClampedAt100(t *testing.T)
//   GET /search?limit=500 → fakeSearcher.searches[0].Limit == 100

func TestSearchHandler_EmptyResultsReturnsEmptyItemsArray(t *testing.T)
//   fakeSearcher 返回 [],handler 返回 {"items": []}(不是 null)

func TestSearchHandler_SearcherErrorReturns500(t *testing.T)
//   fakeSearcher 返回 err → response 500
```

## R.2 我会查的

- [ ] 全部 14 个测试都用 `db.RunMigrations`(或 fake),**不许裸 CREATE/DROP TABLE**
- [ ] 测试名严格对齐 spec(我会逐个 grep)
- [ ] 测试断言**具体数值**(不是 `len(items) > 0` 这种弱断言)
- [ ] 无 `t.Skip` 用作防御性 guard(教训 PR #81)
- [ ] CLAUDE.md 单独 commit 追加 1 条 gotcha(本次新学的)

## R.3 不许做

| ❌ | 原因 |
|---|---|
| 改 payment/order/search 业务代码 | 本 PR 只补测试,不改逻辑 |
| 用 fake 替代真 PG 的 repo 集成测试 | 集成测试必须真 PG;handler 测试用 fake 可以 |
| 把多个测试合并成 table-driven 一个测试 | 单独的命名测试便于审 + 失败时定位 |

---

# 跨 PR 通用 — Part X · 我审核会查的清单

每 PR 我会逐条核对:

```
通用
[ ] 4 commits 序:① test → ② backend → ③ frontend → ④ docs(claude)
[ ] gofmt -l . && goimports -l . 两个都空
[ ] go vet ./...
[ ] 真 PG: go test -race -p 1 -count=1 ./... 全过(无 skip)
[ ] 前端 tsc/lint/build
[ ] smart-quote 扫修改 .tsx == 0(显示文本里的不计)
[ ] CLAUDE.md 末尾 commit 追加 ≥1 条本次实战学到的 gotcha
[ ] PR description 自检表打勾 + 关键 grep 输出粘贴

每 PR 专项:见各 PR 的 X.5 章节

严禁
[ ] 改 spec 锁定的测试名(包括大小写、下划线)
[ ] 自 merge(即使 CI 绿也不许)
[ ] 把 spec 没要求的「优化」塞进 PR
[ ] 用 t.Skip 当防御
```

---

# Part Y · 执行顺序(execute exactly)

```
1. git fetch origin && git log origin/main -1   # 确认基线
2. git worktree add ~/ai-data-marketplace-O -b feat/dataset-qa origin/main
3. 实现 PR-O + 自检 → push → gh pr create → 等我审 → 我说 merge 你才 merge
4. PR-O 合并 → git worktree remove ~/ai-data-marketplace-O

5. git worktree add ~/ai-data-marketplace-P -b feat/withdrawal origin/main
6. 实现 PR-P + 自检 → push → 等我审 → 等我 OK 才 merge
7. PR-P 合并 → git worktree remove ~/ai-data-marketplace-P

8. git worktree add ~/ai-data-marketplace-Q -b feat/audit-anomaly origin/main
9. 实现 PR-Q + 自检 → push → 等我审 → 等我 OK 才 merge
10. PR-Q 合并 → git worktree remove ~/ai-data-marketplace-Q

11. git worktree add ~/ai-data-marketplace-R -b test/coverage-backfill-72 origin/main
12. 实现 PR-R + 自检 → push → 等我审 → 等我 OK 才 merge
13. PR-R 合并 → git worktree remove ~/ai-data-marketplace-R
```

**铁律 reminder**:每个 PR 必须**先**等我审过、点头才能 merge。CI 绿不是 merge 的充分条件,**我点头才是**。

---

# Part Z · 每个 PR 的 description 模板

```markdown
## PR-X · <方向名>

### 改动文件
- ...

### 新增端点
- ...

### 测试(N 个,逐条 PASS)
- TestXxx (核心断言摘要)
- ...

### Skills learned(本次新坑,已同步 CLAUDE.md)
- ...

### 自检清单(我打勾的版本)
[各 PR 的 X.5 表 + 通用表逐项打勾,粘 Bash 输出]

### CI
backend / frontend / sidecar 全绿
```

---

**就这些。4 个 PR 顺序执行,每个等我审过再 merge。从 PR-O 开始。**
