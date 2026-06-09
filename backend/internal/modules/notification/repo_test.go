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

func TestMarkRead_RejectsOtherUserIDOR(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	uniq := time.Now().UnixNano()
	userA := fmt.Sprintf("user-a-%d", uniq)
	userB := fmt.Sprintf("user-b-%d", uniq)

	n, err := repo.Create(ctx, Notification{
		UserID: userA, Kind: "order_paid", Title: "test", Body: "body",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = repo.MarkRead(ctx, n.ID, userB)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user MarkRead must be ErrNotFound, got %v", err)
	}

	if err := repo.MarkRead(ctx, n.ID, userA); err != nil {
		t.Fatalf("self MarkRead: %v", err)
	}
}

func TestListByUser_RespectsScope(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
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
	if err != nil {
		t.Fatal(err)
	}
	if len(listA) != 2 {
		t.Fatalf("userA listed %d, want 2", len(listA))
	}
	for _, n := range listA {
		if n.UserID != userA {
			t.Fatal("ListByUser leaked another user's row")
		}
	}
}

func TestMarkAllRead_OnlySelfUnread(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	uniq := time.Now().UnixNano()
	userA := fmt.Sprintf("all-a-%d", uniq)
	userB := fmt.Sprintf("all-b-%d", uniq)

	for i := 0; i < 3; i++ {
		if _, err := repo.Create(ctx, Notification{UserID: userA, Kind: "k", Title: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := repo.Create(ctx, Notification{UserID: userB, Kind: "k", Title: "y"}); err != nil {
			t.Fatal(err)
		}
	}

	n, err := repo.MarkAllRead(ctx, userA)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("MarkAllRead(userA) marked %d, want 3", n)
	}

	cnt, err := repo.CountUnread(ctx, userA)
	if err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Fatalf("userA unread after MarkAllRead = %d, want 0", cnt)
	}

	cntB, err := repo.CountUnread(ctx, userB)
	if err != nil {
		t.Fatal(err)
	}
	if cntB != 2 {
		t.Fatalf("userB unread = %d, want 2 (userA's MarkAllRead must NOT touch userB)", cntB)
	}
}

func TestCountUnread_ExcludesRead(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	uniq := time.Now().UnixNano()
	user := fmt.Sprintf("count-%d", uniq)

	n1, _ := repo.Create(ctx, Notification{UserID: user, Kind: "k", Title: "1"})
	_, _ = repo.Create(ctx, Notification{UserID: user, Kind: "k", Title: "2"})
	_, _ = repo.Create(ctx, Notification{UserID: user, Kind: "k", Title: "3"})

	if err := repo.MarkRead(ctx, n1.ID, user); err != nil {
		t.Fatal(err)
	}

	cnt, err := repo.CountUnread(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if cnt != 2 {
		t.Fatalf("unread = %d, want 2", cnt)
	}
}

func TestListByUser_OrdersByCreatedAtDesc(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	uniq := time.Now().UnixNano()
	user := fmt.Sprintf("order-%d", uniq)

	titles := []string{"first", "second", "third"}
	for _, ttl := range titles {
		if _, err := repo.Create(ctx, Notification{UserID: user, Kind: "k", Title: ttl}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	list, err := repo.ListByUser(ctx, user, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d, want 3", len(list))
	}
	if list[0].Title != "third" || list[2].Title != "first" {
		t.Fatalf("order wrong: %s,%s,%s want third,second,first", list[0].Title, list[1].Title, list[2].Title)
	}
}
