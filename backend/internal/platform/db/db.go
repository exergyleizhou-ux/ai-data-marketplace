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
	// Pool sizing is env-tunable for production (defaults preserve the old
	// hardcoded behavior). Rule of thumb: MaxConns ~ (cores*2) on the PG box
	// divided across app replicas; never exceed PG max_connections headroom.
	cfg.MaxConns = envInt32("DB_MAX_CONNS", 10)
	cfg.MinConns = envInt32("DB_MIN_CONNS", 2) // warm conns cut p99 on idle->burst
	cfg.MaxConnLifetime = envDuration("DB_MAX_CONN_LIFETIME", time.Hour)
	cfg.MaxConnIdleTime = envDuration("DB_MAX_CONN_IDLE_TIME", 30*time.Minute)
	cfg.HealthCheckPeriod = envDuration("DB_HEALTH_CHECK_PERIOD", time.Minute)
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
