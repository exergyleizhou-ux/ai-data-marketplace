package compliance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DeletionRepository interface {
	Create(ctx context.Context, userID, reason string, coolingUntil time.Time) (DeletionRequest, error)
	FindActiveByUser(ctx context.Context, userID string) (DeletionRequest, error)
	Get(ctx context.Context, id string) (DeletionRequest, error)
	List(ctx context.Context, status string, limit, offset int) ([]DeletionRequest, error)
	Transition(ctx context.Context, id, from, to string, opsID, note string) (DeletionRequest, error)
	ExecuteDeletion(ctx context.Context, id, userID, opsID string) error
	SetDeleted(ctx context.Context, id, opsID string) error
}

type pgDeletionRepo struct{ pool *pgxpool.Pool }

func NewDeletionRepository(pool *pgxpool.Pool) DeletionRepository { return &pgDeletionRepo{pool: pool} }

func (r *pgDeletionRepo) Create(ctx context.Context, userID, reason string, coolingUntil time.Time) (DeletionRequest, error) {
	var d DeletionRequest
	err := r.pool.QueryRow(ctx,
		`INSERT INTO account_deletion_requests (user_id, reason, cooling_until)
		 VALUES ($1,$2,$3)
		 RETURNING id::text, user_id::text, COALESCE(reason,''), status,
		     cooling_until::text, COALESCE(ops_note,''), requested_at::text,
		     COALESCE(processed_at::text,''), COALESCE(processed_by::text,'')`,
		userID, reason, coolingUntil).
		Scan(&d.ID, &d.UserID, &d.Reason, &d.Status,
			&d.CoolingUntil, &d.OpsNote, &d.RequestedAt, &d.ProcessedAt, &d.ProcessedBy)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return DeletionRequest{}, ErrDeletionExists
		}
		return DeletionRequest{}, fmt.Errorf("create deletion request: %w", err)
	}
	return d, nil
}

func (r *pgDeletionRepo) FindActiveByUser(ctx context.Context, userID string) (DeletionRequest, error) {
	var d DeletionRequest
	err := r.pool.QueryRow(ctx,
		`SELECT id::text, user_id::text, COALESCE(reason,''), status,
			cooling_until::text, COALESCE(ops_note,''), requested_at::text,
			COALESCE(processed_at::text,''), COALESCE(processed_by::text,'')
		 FROM account_deletion_requests
		 WHERE user_id=$1 AND status IN ('cooling','approved')
		 ORDER BY requested_at DESC LIMIT 1`, userID).
		Scan(&d.ID, &d.UserID, &d.Reason, &d.Status,
			&d.CoolingUntil, &d.OpsNote, &d.RequestedAt, &d.ProcessedAt, &d.ProcessedBy)
	if err != nil {
		if err == pgx.ErrNoRows {
			return DeletionRequest{}, ErrNotFound
		}
		return DeletionRequest{}, fmt.Errorf("find active deletion: %w", err)
	}
	return d, nil
}

func (r *pgDeletionRepo) Get(ctx context.Context, id string) (DeletionRequest, error) {
	var d DeletionRequest
	err := r.pool.QueryRow(ctx,
		`SELECT id::text, user_id::text, COALESCE(reason,''), status,
			cooling_until::text, COALESCE(ops_note,''), requested_at::text,
			COALESCE(processed_at::text,''), COALESCE(processed_by::text,'')
		 FROM account_deletion_requests WHERE id=$1`, id).
		Scan(&d.ID, &d.UserID, &d.Reason, &d.Status,
			&d.CoolingUntil, &d.OpsNote, &d.RequestedAt, &d.ProcessedAt, &d.ProcessedBy)
	if err != nil {
		if err == pgx.ErrNoRows {
			return DeletionRequest{}, ErrNotFound
		}
		return DeletionRequest{}, fmt.Errorf("get deletion: %w", err)
	}
	return d, nil
}

func (r *pgDeletionRepo) List(ctx context.Context, status string, limit, offset int) ([]DeletionRequest, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var rows pgx.Rows
	var err error
	if status == "" {
		rows, err = r.pool.Query(ctx,
			`SELECT id::text, user_id::text, COALESCE(reason,''), status,
				cooling_until::text, COALESCE(ops_note,''), requested_at::text,
				COALESCE(processed_at::text,''), COALESCE(processed_by::text,'')
			 FROM account_deletion_requests ORDER BY requested_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	} else {
		rows, err = r.pool.Query(ctx,
			`SELECT id::text, user_id::text, COALESCE(reason,''), status,
				cooling_until::text, COALESCE(ops_note,''), requested_at::text,
				COALESCE(processed_at::text,''), COALESCE(processed_by::text,'')
			 FROM account_deletion_requests WHERE status=$1 ORDER BY requested_at DESC LIMIT $2 OFFSET $3`,
			status, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("list deletions: %w", err)
	}
	defer rows.Close()
	var out []DeletionRequest
	for rows.Next() {
		var d DeletionRequest
		if err := rows.Scan(&d.ID, &d.UserID, &d.Reason, &d.Status,
			&d.CoolingUntil, &d.OpsNote, &d.RequestedAt, &d.ProcessedAt, &d.ProcessedBy); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *pgDeletionRepo) Transition(ctx context.Context, id, from, to string, opsID, note string) (DeletionRequest, error) {
	if from == DeletionDeleted {
		return DeletionRequest{}, ErrBadTransition
	}
	var d DeletionRequest
	err := r.pool.QueryRow(ctx,
		`UPDATE account_deletion_requests
		 SET status=$3, ops_note=$5, processed_at=now(), processed_by=$4::uuid
		 WHERE id=$1 AND status=$2
		 RETURNING id::text, user_id::text, COALESCE(reason,''), status,
		 cooling_until::text, COALESCE(ops_note,''), requested_at::text,
		 COALESCE(processed_at::text,''), COALESCE(processed_by::text,'')`,
		id, from, to, opsID, note).
		Scan(&d.ID, &d.UserID, &d.Reason, &d.Status,
			&d.CoolingUntil, &d.OpsNote, &d.RequestedAt, &d.ProcessedAt, &d.ProcessedBy)
	if err != nil {
		if err == pgx.ErrNoRows {
			return DeletionRequest{}, ErrBadTransition
		}
		return DeletionRequest{}, fmt.Errorf("transition deletion: %w", err)
	}
	return d, nil
}

func (r *pgDeletionRepo) ExecuteDeletion(ctx context.Context, id, userID, opsID string) error {
	// Scrub PII but preserve audit/financial records.
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("execute deletion begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Scrub user PII
	if _, err := tx.Exec(ctx,
		`UPDATE users SET account = 'deleted-' || id::text,
			password_hash = 'deleted', kyc_status = 'rejected'
		 WHERE id = $1`, userID); err != nil {
		return fmt.Errorf("scrub user: %w", err)
	}
	// 2. Delete notifications
	if _, err := tx.Exec(ctx, `DELETE FROM notifications WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete notifications: %w", err)
	}
	// 3. Delete watches
	if _, err := tx.Exec(ctx, `DELETE FROM dataset_watches WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete watches: %w", err)
	}
	// 4. Scrub QA
	if _, err := tx.Exec(ctx, `UPDATE dataset_questions SET body='[已删除]' WHERE asker_id=$1`, userID); err != nil {
		return fmt.Errorf("scrub questions: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE dataset_answers SET body='[已删除]' WHERE answerer_id=$1`, userID); err != nil {
		return fmt.Errorf("scrub answers: %w", err)
	}
	// 5. Scrub withdrawal account_label
	if _, err := tx.Exec(ctx, `UPDATE withdrawal_requests SET account_label='[已删除]' WHERE seller_id=$1`, userID); err != nil {
		return fmt.Errorf("scrub withdrawals: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *pgDeletionRepo) SetDeleted(ctx context.Context, id, opsID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE account_deletion_requests SET status='deleted', processed_at=now(), processed_by=$2::uuid
		 WHERE id=$1`, id, opsID)
	return err
}
