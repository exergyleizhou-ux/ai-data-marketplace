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
	return NewRepository(pool), func() { pool.Close() }
}

func TestRegister_Idempotent(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	certID := fmt.Sprintf("VO-IDEM%04d", time.Now().UnixNano()%10000)
	if err := repo.Register(ctx, certID, "dataset", "ds-1"); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := repo.Register(ctx, certID, "dataset", "ds-1"); err != nil {
		t.Fatalf("second register must be idempotent, got: %v", err)
	}
}

func TestRegister_SameCertIDDifferentResource_KeepsFirst(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	certID := fmt.Sprintf("VO-KEEP%04d", time.Now().UnixNano()%10000)
	if err := repo.Register(ctx, certID, "dataset", "ds-original"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Register(ctx, certID, "dataset", "ds-other"); err != nil {
		t.Fatal(err)
	}
	info, err := repo.FindByCertID(ctx, certID)
	if err != nil {
		t.Fatal(err)
	}
	if info.ResourceID != "ds-original" {
		t.Fatalf("ON CONFLICT DO NOTHING should keep first: got %s, want ds-original", info.ResourceID)
	}
}

func TestFindByCertID_NotFound(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	_, err := repo.FindByCertID(context.Background(), "VO-NEVER-EXISTS")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing cert must return ErrNotFound, got %v", err)
	}
}
