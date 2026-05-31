package payment

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns the payment and settlement tables. All money-moving writes are
// idempotent (docs §6.6): duplicate webhooks and retried settlements are no-ops.
type Repository interface {
	// EnsurePayment creates the payment row for an order, or returns the
	// existing one (order_id is unique) — so create is idempotent per order.
	EnsurePayment(ctx context.Context, orderID, channel, channelTxnID string, amountCents int64) error
	// MarkPaidByChannelTxn flips created->paid (escrow frozen) exactly once.
	// newlyPaid is false if the callback was already processed.
	MarkPaidByChannelTxn(ctx context.Context, channelTxnID string) (orderID string, newlyPaid bool, err error)
	// CreateSettlement inserts a pending settlement; created=false if one
	// already exists (unique order_id / idempotency_key) — the double-split guard.
	CreateSettlement(ctx context.Context, orderID, idempotencyKey string, platformFeeCents, sellerAmountCents int64) (created bool, err error)
	MarkSettlementSuccess(ctx context.Context, orderID, splitTxnID string) error
}

type pgRepo struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

const uniqueViolation = "23505"

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

func (r *pgRepo) EnsurePayment(ctx context.Context, orderID, channel, channelTxnID string, amountCents int64) error {
	const q = `
		INSERT INTO payments (order_id, channel, channel_txn_id, amount_cents, status, idempotency_key)
		VALUES ($1,$2,$3,$4,'created',$5)
		ON CONFLICT (order_id) DO NOTHING`
	_, err := r.pool.Exec(ctx, q, orderID, channel, channelTxnID, amountCents, "pay:"+orderID)
	if err != nil {
		return fmt.Errorf("ensure payment: %w", err)
	}
	return nil
}

func (r *pgRepo) MarkPaidByChannelTxn(ctx context.Context, channelTxnID string) (string, bool, error) {
	const upd = `
		UPDATE payments SET status='paid', escrow_state='frozen', paid_at=now()
		WHERE channel_txn_id=$1 AND status='created'
		RETURNING order_id`
	var orderID string
	err := r.pool.QueryRow(ctx, upd, channelTxnID).Scan(&orderID)
	if err == nil {
		return orderID, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("mark paid: %w", err)
	}
	// Not newly updated — either already paid (idempotent) or unknown txn.
	var status string
	err = r.pool.QueryRow(ctx, `SELECT order_id, status FROM payments WHERE channel_txn_id=$1`, channelTxnID).
		Scan(&orderID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("unknown channel_txn_id")
	}
	if err != nil {
		return "", false, fmt.Errorf("lookup payment: %w", err)
	}
	return orderID, false, nil // already handled
}

func (r *pgRepo) CreateSettlement(ctx context.Context, orderID, idempotencyKey string, platformFeeCents, sellerAmountCents int64) (bool, error) {
	const q = `
		INSERT INTO settlements (order_id, platform_fee_cents, seller_amount_cents, status, idempotency_key)
		VALUES ($1,$2,$3,'pending',$4)`
	_, err := r.pool.Exec(ctx, q, orderID, platformFeeCents, sellerAmountCents, idempotencyKey)
	if err != nil {
		if isUnique(err) {
			return false, nil // already settling/settled
		}
		return false, fmt.Errorf("create settlement: %w", err)
	}
	return true, nil
}

func (r *pgRepo) MarkSettlementSuccess(ctx context.Context, orderID, splitTxnID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE settlements SET status='success', split_txn_id=$2, executed_at=now() WHERE order_id=$1`,
		orderID, splitTxnID)
	if err != nil {
		return fmt.Errorf("mark settlement success: %w", err)
	}
	return nil
}
