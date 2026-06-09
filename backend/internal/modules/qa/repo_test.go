package qa

import (
	"context"
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
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	if err := db.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	pool.Exec(context.Background(), `TRUNCATE TABLE dataset_answers, dataset_questions`)
	return NewRepository(pool), func() { pool.Close() }
}

func seedUser(t *testing.T, pool *pgxpool.Pool, accountPrefix string) (userID, account string) {
	t.Helper()
	uniq := fmt.Sprintf("%d", time.Now().UnixNano())
	account = accountPrefix + "-" + uniq + "@example.com"
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (account, account_type, password_hash, role, kyc_status)
		 VALUES ($1,'email','x','buyer','verified') RETURNING id::text`,
		account).Scan(&userID)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return userID, account
}

func seedDataset(t *testing.T, pool *pgxpool.Pool, sellerID string) string {
	t.Helper()
	var id string
	uniq := fmt.Sprintf("%d", time.Now().UnixNano())
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO datasets (seller_id, title, data_type, license_type, status)
		 VALUES ($1, $2, 'text', 'commercial', 'published') RETURNING id::text`,
		sellerID, "qa-ds-"+fmt.Sprint(uniq)).Scan(&id); err != nil {
		t.Fatalf("seed dataset: %v", err)
	}
	return id
}

func TestCreateQuestion_PersistsAndReturnsID(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	asker, _ := seedUser(t, pool, "q-asker")
	dsID := seedDataset(t, pool, asker)

	q, err := repo.CreateQuestion(ctx, Question{DatasetID: dsID, AskerID: asker, Body: "body"})
	if err != nil {
		t.Fatal(err)
	}
	if q.ID == "" {
		t.Fatal("ID must not be empty")
	}
	if q.CreatedAt == "" {
		t.Fatal("created_at must not be empty")
	}
}

func TestCreateAnswer_LinksToQuestion(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	asker, _ := seedUser(t, pool, "qa-asker")
	seller, _ := seedUser(t, pool, "qa-seller")
	dsID := seedDataset(t, pool, seller)

	q, _ := repo.CreateQuestion(ctx, Question{DatasetID: dsID, AskerID: asker, Body: "?"})
	a, err := repo.CreateAnswer(ctx, Answer{QuestionID: q.ID, AnswererID: seller, Body: "ans"})
	if err != nil {
		t.Fatal(err)
	}
	if a.QuestionID != q.ID {
		t.Fatalf("questionID = %q, want %q", a.QuestionID, q.ID)
	}
	if a.ID == "" || a.CreatedAt == "" {
		t.Fatal("id/created_at must not be empty")
	}
}

func TestListByDataset_OrdersByCreatedAtDesc(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	asker, _ := seedUser(t, pool, "list-asker")
	dsID := seedDataset(t, pool, asker)

	q1, _ := repo.CreateQuestion(ctx, Question{DatasetID: dsID, AskerID: asker, Body: "first"})
	time.Sleep(2 * time.Millisecond)
	q2, _ := repo.CreateQuestion(ctx, Question{DatasetID: dsID, AskerID: asker, Body: "second"})

	items, err := repo.ListByDataset(ctx, dsID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 2 {
		t.Fatalf("got %d, want at least 2", len(items))
	}
	if items[0].ID != q2.ID {
		t.Fatal("most recent must be first (desc order)")
	}
	if items[1].ID != q1.ID {
		t.Fatal("oldest must be last")
	}
}

func TestListByDataset_ExcludesHidden(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	asker, _ := seedUser(t, pool, "hid-asker")
	dsID := seedDataset(t, pool, asker)

	q1, _ := repo.CreateQuestion(ctx, Question{DatasetID: dsID, AskerID: asker, Body: "visible"})
	q2, _ := repo.CreateQuestion(ctx, Question{DatasetID: dsID, AskerID: asker, Body: "visible2"})
	_ = repo.SetQuestionStatus(ctx, q1.ID, "hidden")
	_ = repo.SetQuestionStatus(ctx, q2.ID, "hidden")

	items, err := repo.ListByDataset(ctx, dsID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Status == "hidden" {
			t.Fatal("ListByDataset must exclude hidden questions")
		}
	}
}

func TestListByDataset_AttachesAnswerWhenPresent(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	asker, _ := seedUser(t, pool, "att-asker")
	seller, _ := seedUser(t, pool, "att-seller")
	dsID := seedDataset(t, pool, seller)

	q1, _ := repo.CreateQuestion(ctx, Question{DatasetID: dsID, AskerID: asker, Body: "q1"})
	q2, _ := repo.CreateQuestion(ctx, Question{DatasetID: dsID, AskerID: asker, Body: "q2"})
	repo.CreateAnswer(ctx, Answer{QuestionID: q1.ID, AnswererID: seller, Body: "a1"})

	items, _ := repo.ListByDataset(ctx, dsID, 10, 0)
	for _, it := range items {
		if it.ID == q1.ID && it.Answer == nil {
			t.Fatal("q1 must have Answer attached")
		}
		if it.ID == q2.ID && it.Answer != nil {
			t.Fatal("q2 must not have Answer")
		}
	}
}

func TestListByDataset_AskerNameIsPrefixOfAccount(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	askerID, account := seedUser(t, pool, "nameuser")
	seller, _ := seedUser(t, pool, "qa-seller3")
	dsID := seedDataset(t, pool, seller)

	q, err := repo.CreateQuestion(ctx, Question{DatasetID: dsID, AskerID: askerID, Body: "testing names"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetQuestion(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.AskerName) == 0 {
		t.Fatal("AskerName must be non-empty")
	}
	expected := account[:8]
	if got.AskerName != expected {
		t.Fatalf("AskerName = %q, want first 8 chars of account = %q", got.AskerName, expected)
	}
}

func TestSetQuestionStatus_LeavesHiddenAlone(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	asker, _ := seedUser(t, pool, "sh-asker")
	dsID := seedDataset(t, pool, asker)

	q, _ := repo.CreateQuestion(ctx, Question{DatasetID: dsID, AskerID: asker, Body: "x"})
	repo.SetQuestionStatus(ctx, q.ID, "hidden")
	// Try to change hidden → answered — must fail
	if err := repo.SetQuestionStatus(ctx, q.ID, "answered"); err == nil {
		t.Fatal("SetQuestionStatus must NOT update hidden questions")
	}
}
