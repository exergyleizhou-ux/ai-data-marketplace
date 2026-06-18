package moderation

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

func testRepo(t *testing.T) (Repository, *pgxpool.Pool, func()) {
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
	pool.Exec(context.Background(), `TRUNCATE TABLE content_reports`)
	return NewRepository(pool), pool, func() { pool.Close() }
}

func seedUser(t *testing.T, pool *pgxpool.Pool, role string) string {
	t.Helper()
	var id string
	acct := fmt.Sprintf("mod-%s-%d@example.com", role, time.Now().UnixNano())
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (account, account_type, password_hash, role, kyc_status)
		 VALUES ($1,'email','x',$2,'verified') RETURNING id::text`, acct, role).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// seedQuestion inserts a dataset + question and returns the question id.
func seedQuestion(t *testing.T, pool *pgxpool.Pool, sellerID string) string {
	t.Helper()
	ctx := context.Background()
	var dsID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO datasets (seller_id, title, data_type, license_type, status)
		 VALUES ($1, $2, 'text', 'commercial', 'published') RETURNING id::text`,
		sellerID, "mod-ds-"+fmt.Sprint(time.Now().UnixNano())).Scan(&dsID); err != nil {
		t.Fatalf("seed dataset: %v", err)
	}
	asker := seedUser(t, pool, "buyer")
	var qID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO dataset_questions (dataset_id, asker_id, body, status)
		 VALUES ($1::uuid, $2::uuid, 'spammy question', 'open') RETURNING id::text`,
		dsID, asker).Scan(&qID); err != nil {
		t.Fatalf("seed question: %v", err)
	}
	return qID
}

// TestReport_RejectsNonexistentTarget: a report against a target id that doesn't
// exist (or a malformed uuid) must be rejected, not persisted — otherwise the
// attacker-controlled target_id is a report-spam vector.
func TestReport_RejectsNonexistentTarget(t *testing.T) {
	repo, pool, done := testRepo(t)
	defer done()
	reporter := seedUser(t, pool, "buyer")
	ctx := context.Background()

	// A well-formed but non-existent question id.
	if _, err := repo.CreateReport(ctx, reporter, TargetQuestion, "00000000-0000-0000-0000-0000000000ff", "abuse"); err != ErrTargetNotFound {
		t.Fatalf("nonexistent target = %v, want ErrTargetNotFound", err)
	}
	// A malformed uuid.
	if _, err := repo.CreateReport(ctx, reporter, TargetReview, "not-a-uuid", "abuse"); err != ErrTargetNotFound {
		t.Fatalf("malformed target = %v, want ErrTargetNotFound", err)
	}
	var n int
	pool.QueryRow(ctx, `SELECT count(*) FROM content_reports WHERE reporter_id=$1::uuid`, reporter).Scan(&n)
	if n != 0 {
		t.Fatalf("no report row should be created for a bad target, got %d", n)
	}
}

func TestReport_DedupesOpenReports(t *testing.T) {
	repo, pool, done := testRepo(t)
	defer done()
	seller := seedUser(t, pool, "seller")
	reporter := seedUser(t, pool, "buyer")
	qID := seedQuestion(t, pool, seller)
	ctx := context.Background()

	r1, err := repo.CreateReport(ctx, reporter, TargetQuestion, qID, "abuse")
	if err != nil {
		t.Fatalf("first report: %v", err)
	}
	r2, err := repo.CreateReport(ctx, reporter, TargetQuestion, qID, "abuse again")
	if err != nil {
		t.Fatalf("second report: %v", err)
	}
	if r1.ID != r2.ID {
		t.Fatalf("re-reporting an open target must return the same report (got %s vs %s)", r1.ID, r2.ID)
	}
	var n int
	pool.QueryRow(ctx, `SELECT count(*) FROM content_reports WHERE target_id=$1::uuid`, qID).Scan(&n)
	if n != 1 {
		t.Fatalf("expected 1 report row, got %d", n)
	}
}

func TestResolve_HideHidesQuestion(t *testing.T) {
	repo, pool, done := testRepo(t)
	defer done()
	seller := seedUser(t, pool, "seller")
	reporter := seedUser(t, pool, "buyer")
	ops := seedUser(t, pool, "ops")
	qID := seedQuestion(t, pool, seller)
	ctx := context.Background()

	rep, err := repo.CreateReport(ctx, reporter, TargetQuestion, qID, "abuse")
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	resolved, err := repo.Resolve(ctx, rep.ID, ResolutionHide, ops)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Status != StatusResolved || resolved.Resolution != ResolutionHide {
		t.Fatalf("report not resolved/hidden: %+v", resolved)
	}
	var status string
	pool.QueryRow(ctx, `SELECT status FROM dataset_questions WHERE id=$1::uuid`, qID).Scan(&status)
	if status != "hidden" {
		t.Fatalf("question status = %q, want hidden", status)
	}
	// Resolving an already-resolved report must fail (optimistic guard).
	if _, err := repo.Resolve(ctx, rep.ID, ResolutionDismiss, ops); err != ErrReportNotFound {
		t.Fatalf("re-resolve should fail with ErrReportNotFound, got %v", err)
	}
}

func TestResolve_DismissKeepsContent(t *testing.T) {
	repo, pool, done := testRepo(t)
	defer done()
	seller := seedUser(t, pool, "seller")
	reporter := seedUser(t, pool, "buyer")
	ops := seedUser(t, pool, "ops")
	qID := seedQuestion(t, pool, seller)
	ctx := context.Background()

	rep, _ := repo.CreateReport(ctx, reporter, TargetQuestion, qID, "not actually abuse")
	if _, err := repo.Resolve(ctx, rep.ID, ResolutionDismiss, ops); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	var status string
	pool.QueryRow(ctx, `SELECT status FROM dataset_questions WHERE id=$1::uuid`, qID).Scan(&status)
	if status == "hidden" {
		t.Fatalf("dismiss must NOT hide the question")
	}
}

func TestListReports_FilterByStatus(t *testing.T) {
	repo, pool, done := testRepo(t)
	defer done()
	seller := seedUser(t, pool, "seller")
	ops := seedUser(t, pool, "ops")
	q1 := seedQuestion(t, pool, seller)
	q2 := seedQuestion(t, pool, seller)
	ctx := context.Background()

	r1, _ := repo.CreateReport(ctx, seedUser(t, pool, "buyer"), TargetQuestion, q1, "a")
	repo.CreateReport(ctx, seedUser(t, pool, "buyer"), TargetQuestion, q2, "b")
	repo.Resolve(ctx, r1.ID, ResolutionDismiss, ops)

	open, err := repo.ListReports(ctx, StatusOpen, 50, 0)
	if err != nil {
		t.Fatalf("list open: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("expected 1 open report, got %d", len(open))
	}
	all, _ := repo.ListReports(ctx, "", 50, 0)
	if len(all) != 2 {
		t.Fatalf("expected 2 total reports, got %d", len(all))
	}
}
