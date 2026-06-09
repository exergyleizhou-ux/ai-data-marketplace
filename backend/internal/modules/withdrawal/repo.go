package withdrawal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Create(ctx context.Context, r Request) (Request, error)
	Get(ctx context.Context, id string) (Request, error)
	ListBySeller(ctx context.Context, sellerID string, limit, offset int) ([]Request, error)
	AdminList(ctx context.Context, status string, limit, offset int) ([]Request, error)
	Transition(ctx context.Context, id, from, to, opsID, note string) (Request, error)
	SumApprovedAndPending(ctx context.Context, sellerID string) (int64, error)
}

type pgRepo struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

func (r *pgRepo) Create(ctx context.Context, req Request) (Request, error) {
	err := r.pool.QueryRow(ctx,
		`INSERT INTO withdrawal_requests (seller_id, amount_cents, channel, account_label)
		 VALUES ($1,$2,$3,$4)
		 RETURNING id::text, status, requested_at::text`,
		req.SellerID, req.AmountCents, req.Channel, req.AccountLabel).
		Scan(&req.ID, &req.Status, &req.RequestedAt)
	if err != nil {
		return Request{}, fmt.Errorf("create withdrawal: %w", err)
	}
	return req, nil
}

func (r *pgRepo) Get(ctx context.Context, id string) (Request, error) {
	var req Request
	err := r.pool.QueryRow(ctx,
		`SELECT id::text, seller_id::text, amount_cents, channel, account_label, status,
			COALESCE(ops_note,''), requested_at::text,
			COALESCE(processed_at::text,''), COALESCE(processed_by::text,'')
		 FROM withdrawal_requests WHERE id=$1`, id).
		Scan(&req.ID, &req.SellerID, &req.AmountCents, &req.Channel, &req.AccountLabel,
			&req.Status, &req.OpsNote, &req.RequestedAt, &req.ProcessedAt, &req.ProcessedBy)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Request{}, ErrNotFound
		}
		return Request{}, fmt.Errorf("get withdrawal: %w", err)
	}
	return req, nil
}

func (r *pgRepo) ListBySeller(ctx context.Context, sellerID string, limit, offset int) ([]Request, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id::text, seller_id::text, amount_cents, channel, account_label, status,
			COALESCE(ops_note,''), requested_at::text,
			COALESCE(processed_at::text,''), COALESCE(processed_by::text,'')
		 FROM withdrawal_requests WHERE seller_id=$1
		 ORDER BY requested_at DESC LIMIT $2 OFFSET $3`, sellerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list withdrawals: %w", err)
	}
	defer rows.Close()
	var out []Request
	for rows.Next() {
		var req Request
		if err := rows.Scan(&req.ID, &req.SellerID, &req.AmountCents, &req.Channel, &req.AccountLabel,
			&req.Status, &req.OpsNote, &req.RequestedAt, &req.ProcessedAt, &req.ProcessedBy); err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, rows.Err()
}

func (r *pgRepo) AdminList(ctx context.Context, status string, limit, offset int) ([]Request, error) {
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
			`SELECT id::text, seller_id::text, amount_cents, channel, account_label, status,
				COALESCE(ops_note,''), requested_at::text,
				COALESCE(processed_at::text,''), COALESCE(processed_by::text,'')
			 FROM withdrawal_requests ORDER BY requested_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	} else {
		rows, err = r.pool.Query(ctx,
			`SELECT id::text, seller_id::text, amount_cents, channel, account_label, status,
				COALESCE(ops_note,''), requested_at::text,
				COALESCE(processed_at::text,''), COALESCE(processed_by::text,'')
			 FROM withdrawal_requests WHERE status=$1 ORDER BY requested_at DESC LIMIT $2 OFFSET $3`,
			status, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("admin list withdrawals: %w", err)
	}
	defer rows.Close()
	return scanRequests(rows)
}

func scanRequests(rows pgx.Rows) ([]Request, error) {
	var out []Request
	for rows.Next() {
		var req Request
		if err := rows.Scan(&req.ID, &req.SellerID, &req.AmountCents, &req.Channel, &req.AccountLabel,
			&req.Status, &req.OpsNote, &req.RequestedAt, &req.ProcessedAt, &req.ProcessedBy); err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, rows.Err()
}

func (r *pgRepo) Transition(ctx context.Context, id, from, to, opsID, note string) (Request, error) {
	// Guard: no transition FROM completed (terminal state).
	if from == StatusCompleted {
		return Request{}, ErrBadTransition
	}
	var req Request
	err := r.pool.QueryRow(ctx,
		`UPDATE withdrawal_requests
		 SET status = $3, ops_note = $5, processed_at = now(), processed_by = $4::uuid
		 WHERE id = $1::uuid AND status = $2
		 RETURNING id::text, seller_id::text, amount_cents, channel, account_label, status,
			COALESCE(ops_note,''), requested_at::text,
			COALESCE(processed_at::text,''), COALESCE(processed_by::text,'')`,
		id, from, to, opsID, note).
		Scan(&req.ID, &req.SellerID, &req.AmountCents, &req.Channel, &req.AccountLabel,
			&req.Status, &req.OpsNote, &req.RequestedAt, &req.ProcessedAt, &req.ProcessedBy)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Request{}, ErrBadTransition
		}
		return Request{}, fmt.Errorf("transition withdrawal: %w", err)
	}
	return req, nil
}

func (r *pgRepo) SumApprovedAndPending(ctx context.Context, sellerID string) (int64, error) {
	var sum int64
	if err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount_cents), 0) FROM withdrawal_requests
		 WHERE seller_id = $1 AND status IN ('pending', 'approved')`, sellerID).Scan(&sum); err != nil {
		return 0, fmt.Errorf("sum withdrawals: %w", err)
	}
	return sum, nil
}
