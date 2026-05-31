// Command devsql runs ad-hoc SQL statements against DATABASE_URL. It is a
// development/operations convenience (e.g. promoting an ops account locally)
// and REFUSES to run when APP_ENV=production. Not part of the request path.
//
//	APP_ENV=development DATABASE_URL=... go run ./cmd/devsql "UPDATE users SET role='ops' WHERE account='x'"
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
)

func main() {
	if os.Getenv("APP_ENV") == "production" {
		fmt.Fprintln(os.Stderr, "devsql is disabled in production")
		os.Exit(1)
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" || len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: DATABASE_URL=... devsql \"<sql>\" [\"<sql>\" ...]")
		os.Exit(2)
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect:", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	for _, q := range os.Args[1:] {
		ct, err := conn.Exec(ctx, q)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ERR:", q, "->", err)
			os.Exit(1)
		}
		fmt.Println("OK:", q, "->", ct.String())
	}
}
