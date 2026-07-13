package compute

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compute integration tests run asynchronous workers; isolate their durable
// state from package-parallel E2E and repository fixture cleanup.
func TestMain(m *testing.M) {
	baseDSN := os.Getenv("DATABASE_URL")
	if baseDSN == "" {
		os.Exit(m.Run())
	}
	pool, err := pgxpool.New(context.Background(), baseDSN)
	if err != nil {
		panic(err)
	}
	schema := "test_compute_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = pool.Exec(context.Background(), fmt.Sprintf(`CREATE SCHEMA %q`, schema)); err != nil {
		panic(err)
	}
	u, err := url.Parse(baseDSN)
	if err != nil {
		panic(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	_ = os.Setenv("DATABASE_URL", u.String())
	code := m.Run()
	_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema))
	pool.Close()
	os.Exit(code)
}
