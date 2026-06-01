package payment

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxRepository owns the settlement_outbox table (H3). It records durable
// settlement jobs and drives their retry lifecycle so a confirmed order is
// always eventually settled even if the inline attempt fails.
type OutboxRepository interface {
	// Enqueue records a pending settlement job for an order. Idempotent: a
	// second enqueue for the same order is a no-op (PK on order_id).
	Enqueue(ctx context.Context, orderID string) error
	// DueOrders returns up to limit pending order ids whose next_attempt_at has
	// passed (oldest first).
	DueOrders(ctx context.Context, limit int) ([]string, error)
	// MarkDone marks a job settled.
	MarkDone(ctx context.Context, orderID string) error
	// MarkRetry records a failed attempt: increments attempts, stores the error,
	// and either reschedules with exponential backoff (base*2^attempts, capped)
	// or, once attempts >= maxAttempts, gives up (status 'failed') for ops to
	// inspect.
	MarkRetry(ctx context.Context, orderID, errMsg string, base, capDur time.Duration, maxAttempts int) error
}

// Locker provides mutual exclusion across app instances so the same settlement
// job is not processed concurrently. WithLock runs fn only if the lock for key
// is acquired; it reports whether the lock was held (locked=false means another
// holder has it — skip, don't error).
type Locker interface {
	WithLock(ctx context.Context, key string, fn func(context.Context) error) (locked bool, err error)
}

// --- Postgres implementations ---

type pgOutboxRepo struct{ pool *pgxpool.Pool }

// NewOutboxRepository returns a Postgres-backed OutboxRepository.
func NewOutboxRepository(pool *pgxpool.Pool) OutboxRepository { return &pgOutboxRepo{pool: pool} }

func (r *pgOutboxRepo) Enqueue(ctx context.Context, orderID string) error {
	const q = `
		INSERT INTO settlement_outbox (order_id) VALUES ($1)
		ON CONFLICT (order_id) DO NOTHING`
	if _, err := r.pool.Exec(ctx, q, orderID); err != nil {
		return fmt.Errorf("enqueue settlement: %w", err)
	}
	return nil
}

func (r *pgOutboxRepo) DueOrders(ctx context.Context, limit int) ([]string, error) {
	const q = `
		SELECT order_id FROM settlement_outbox
		WHERE status='pending' AND next_attempt_at <= now()
		ORDER BY next_attempt_at ASC
		LIMIT $1`
	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("due settlements: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *pgOutboxRepo) MarkDone(ctx context.Context, orderID string) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE settlement_outbox SET status='done', last_error=NULL, updated_at=now() WHERE order_id=$1`,
		orderID); err != nil {
		return fmt.Errorf("mark settlement done: %w", err)
	}
	return nil
}

func (r *pgOutboxRepo) MarkRetry(ctx context.Context, orderID, errMsg string, base, capDur time.Duration, maxAttempts int) error {
	// Exponential backoff: base * 2^attempts (attempts = the count BEFORE this
	// failure is recorded), capped. Computed in SQL so it reads the current
	// attempts without a round-trip.
	const q = `
		UPDATE settlement_outbox
		SET attempts = attempts + 1,
		    last_error = $2,
		    status = CASE WHEN attempts + 1 >= $3 THEN 'failed' ELSE 'pending' END,
		    next_attempt_at = now() + make_interval(secs => LEAST($5, $4 * power(2, attempts))),
		    updated_at = now()
		WHERE order_id = $1`
	if _, err := r.pool.Exec(ctx, q, orderID, errMsg, maxAttempts, base.Seconds(), capDur.Seconds()); err != nil {
		return fmt.Errorf("mark settlement retry: %w", err)
	}
	return nil
}

type pgLocker struct{ pool *pgxpool.Pool }

// NewPGLocker returns a Locker backed by Postgres transaction-scoped advisory
// locks (pg_try_advisory_xact_lock). The lock is held by the lock transaction
// and auto-released on commit/rollback (or if the connection dies), so a
// crashed worker never leaves a stuck lock. fn does its own DB work on separate
// pooled connections — the lock transaction is purely the mutex.
func NewPGLocker(pool *pgxpool.Pool) Locker { return &pgLocker{pool: pool} }

func (l *pgLocker) WithLock(ctx context.Context, key string, fn func(context.Context) error) (bool, error) {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("lock begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // releases the advisory lock if not committed

	var got bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtext($1))`, key).Scan(&got); err != nil {
		return false, fmt.Errorf("acquire advisory lock: %w", err)
	}
	if !got {
		return false, nil // someone else holds it
	}
	if err := fn(ctx); err != nil {
		return true, err
	}
	if err := tx.Commit(ctx); err != nil {
		return true, fmt.Errorf("lock commit: %w", err)
	}
	return true, nil
}
