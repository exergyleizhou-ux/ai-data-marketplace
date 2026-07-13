package server

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

// TestMain gives this integration-heavy package its own schema. Go executes
// packages concurrently, so using the public schema let unrelated repository
// fixtures truncate server rows midway through a request.
func TestMain(m *testing.M) {
	baseDSN := os.Getenv("DATABASE_URL")
	if baseDSN == "" {
		os.Exit(m.Run())
	}
	pool, err := pgxpool.New(context.Background(), baseDSN)
	if err != nil {
		panic(err)
	}
	schema := "test_server_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
