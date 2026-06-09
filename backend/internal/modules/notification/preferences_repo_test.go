package notification

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/db"
)

func testPrefsRepo(t *testing.T) (PreferencesRepository, func()) {
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
	pool.Exec(context.Background(), `TRUNCATE TABLE notification_preferences`)
	// Seed two users for FK constraints.
	pool.Exec(context.Background(),
		`INSERT INTO users (id, account, account_type, password_hash, role)
		 VALUES ('00000000-0000-0000-0000-000000000001','pref-a@x.com','email','x','buyer')
		 ON CONFLICT DO NOTHING`)
	pool.Exec(context.Background(),
		`INSERT INTO users (id, account, account_type, password_hash, role)
		 VALUES ('00000000-0000-0000-0000-000000000002','pref-b@x.com','email','x','buyer')
		 ON CONFLICT DO NOTHING`)
	return NewPreferencesRepository(pool), func() { pool.Close() }
}

func TestPreferences_DefaultsToAllEnabled(t *testing.T) {
	repo, cleanup := testPrefsRepo(t)
	defer cleanup()
	prefs, err := repo.GetForUser(context.Background(), "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if len(prefs) != 0 {
		t.Fatalf("no stored prefs → map must be empty, got %d entries", len(prefs))
	}
}

func TestPreferences_UpdateUpsertsByKindAndUser(t *testing.T) {
	repo, cleanup := testPrefsRepo(t)
	defer cleanup()
	ctx := context.Background()
	uid := "00000000-0000-0000-0000-000000000001"

	if err := repo.UpdateForUser(ctx, uid, "order_paid", false, true); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateForUser(ctx, uid, "order_paid", true, false); err != nil {
		t.Fatal(err)
	}

	prefs, _ := repo.GetForUser(ctx, uid)
	if len(prefs) != 1 {
		t.Fatalf("prefs = %d, want 1", len(prefs))
	}
	if prefs["order_paid"].EmailEnabled != true || prefs["order_paid"].InAppEnabled != false {
		t.Fatalf("second update must overwrite: %+v", prefs["order_paid"])
	}
}

func TestPreferences_PerUserIsolation(t *testing.T) {
	repo, cleanup := testPrefsRepo(t)
	defer cleanup()
	ctx := context.Background()
	userA := "00000000-0000-0000-0000-000000000001"
	userB := "00000000-0000-0000-0000-000000000002"

	repo.UpdateForUser(ctx, userA, "order_paid", false, true)
	repo.UpdateForUser(ctx, userB, "order_paid", true, true)

	prefsA, _ := repo.GetForUser(ctx, userA)
	prefsB, _ := repo.GetForUser(ctx, userB)
	if !prefsA["order_paid"].InAppEnabled || prefsA["order_paid"].EmailEnabled {
		t.Fatal("userA must have email disabled, in-app enabled")
	}
	if !prefsB["order_paid"].EmailEnabled {
		t.Fatal("userB must have email enabled")
	}
}
