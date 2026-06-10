// Package db owns the PostgreSQL connection pool shared by all modules.
//
// Modules receive *pgxpool.Pool (or a narrower interface) by dependency
// injection; they never open their own connections. This keeps connection
// limits and transaction boundaries under one roof.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/tracing"
)

// NewPool opens a pgx connection pool and verifies connectivity with a ping.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database dsn: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MaxConnLifetime = time.Hour
	// Per-query OTel spans; no-op (and effectively free) unless tracing.Init
	// installed a real provider at startup.
	cfg.ConnConfig.Tracer = tracing.NewPgxTracer()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}
