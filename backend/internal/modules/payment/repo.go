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
	// ChannelTxnByOrder returns the payment's channel txn id for an order
	// (used by the dev-only simulate-paid helper).
	ChannelTxnByOrder(ctx context.Context, orderID string) (string, error)
	// CreateSettlement inserts a pending settlement; created=false if one
	// already exists (unique order_id / idempotency_key) — the double-split guard.
	CreateSettlement(ctx context.Context, orderID, idempotencyKey string, platformFeeCents, sellerAmountCents int64) (created bool, err error)
	MarkSettlementSuccess(ctx context.Context, orderID, splitTxnID string) error
	// SettlementState returns the settlement row's status for an order, with
	// exists=false if there is none yet. Lets the (retriable) settle path be
	// idempotent regardless of the order's current status.
	SettlementState(ctx context.Context, orderID string) (status string, exists bool, err error)
	// RefundContext returns the data needed to reverse a payment (H2): the
	// payment's channel txn id, and the settlement's split txn id if it already
	// settled successfully (else "" — funds still escrowed, nothing to reverse).
	RefundContext(ctx context.Context, orderID string) (channelTxnID, splitTxnID string, err error)
	// MarkRefunded flips the payment to refunded/escrow reverted and any
	// successful settlement to reverted, atomically.
	MarkRefunded(ctx context.Context, orderID string) error
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

func (r *pgRepo) ChannelTxnByOrder(ctx context.Context, orderID string) (string, error) {
	var txn string
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(channel_txn_id,'') FROM payments WHERE order_id=$1`, orderID).Scan(&txn)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("no payment for order")
	}
	if err != nil {
		return "", fmt.Errorf("channel txn by order: %w", err)
	}
	return txn, nil
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

func (r *pgRepo) SettlementState(ctx context.Context, orderID string) (string, bool, error) {
	var status string
	err := r.pool.QueryRow(ctx, `SELECT status FROM settlements WHERE order_id=$1`, orderID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("settlement state: %w", err)
	}
	return status, true, nil
}

func (r *pgRepo) RefundContext(ctx context.Context, orderID string) (string, string, error) {
	var channelTxn string
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(channel_txn_id,'') FROM payments WHERE order_id=$1`, orderID).Scan(&channelTxn)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", fmt.Errorf("no payment for order")
	}
	if err != nil {
		return "", "", fmt.Errorf("refund context payment: %w", err)
	}
	// split_txn_id only matters if the order already settled successfully.
	var splitTxn string
	err = r.pool.QueryRow(ctx,
		`SELECT COALESCE(split_txn_id,'') FROM settlements WHERE order_id=$1 AND status='success'`, orderID).
		Scan(&splitTxn)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", fmt.Errorf("refund context settlement: %w", err)
	}
	return channelTxn, splitTxn, nil
}

func (r *pgRepo) MarkRefunded(ctx context.Context, orderID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
	if _, err := tx.Exec(ctx,
		`UPDATE payments SET status='refunded', escrow_state='reverted' WHERE order_id=$1`, orderID); err != nil {
		return fmt.Errorf("mark payment refunded: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET status='reverted' WHERE order_id=$1 AND status='success'`, orderID); err != nil {
		return fmt.Errorf("mark settlement reverted: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit refund: %w", err)
	}
	return nil
}
