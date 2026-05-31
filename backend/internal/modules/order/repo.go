package order

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns the order/dispute tables. Status changes go through
// Transition (optimistic WHERE status=from) so the state machine is enforced at
// the DB level too.
type Repository interface {
	Create(ctx context.Context, o Order) (Order, error)
	GetByID(ctx context.Context, id string) (Order, error)
	ListByBuyer(ctx context.Context, buyerID string, limit, offset int) ([]Order, error)
	ListBySeller(ctx context.Context, sellerID string, limit, offset int) ([]Order, error)
	// Transition moves an order from->to atomically; ErrBadTransition if the
	// current status is not `from`. setAutoConfirm sets auto_confirm_at = now()+7d
	// when moving to delivered.
	Transition(ctx context.Context, id, from, to string, setAutoConfirm bool) (Order, error)
	CreateDispute(ctx context.Context, orderID, raisedBy, reason string) error
}

type pgRepo struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

const orderCols = `id, buyer_id, seller_id, dataset_id, version_id, license_type,
	amount_cents, platform_fee_cents, seller_amount_cents, status,
	COALESCE(auto_confirm_at::text,''), created_at::text, updated_at::text`

func scanOrder(row pgx.Row) (Order, error) {
	var o Order
	err := row.Scan(&o.ID, &o.BuyerID, &o.SellerID, &o.DatasetID, &o.VersionID, &o.LicenseType,
		&o.AmountCents, &o.PlatformFeeCents, &o.SellerAmountCents, &o.Status,
		&o.AutoConfirmAt, &o.CreatedAt, &o.UpdatedAt)
	return o, err
}

const uniqueViolation = "23505"

func (r *pgRepo) Create(ctx context.Context, o Order) (Order, error) {
	const q = `
		INSERT INTO orders (buyer_id, seller_id, dataset_id, version_id, license_type,
			amount_cents, platform_fee_cents, seller_amount_cents, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'created')
		RETURNING ` + orderCols
	out, err := scanOrder(r.pool.QueryRow(ctx, q,
		o.BuyerID, o.SellerID, o.DatasetID, o.VersionID, o.LicenseType,
		o.AmountCents, o.PlatformFeeCents, o.SellerAmountCents))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return Order{}, ErrDuplicateOrder
		}
		return Order{}, fmt.Errorf("create order: %w", err)
	}
	return out, nil
}

func (r *pgRepo) GetByID(ctx context.Context, id string) (Order, error) {
	out, err := scanOrder(r.pool.QueryRow(ctx, `SELECT `+orderCols+` FROM orders WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("get order: %w", err)
	}
	return out, nil
}

func (r *pgRepo) list(ctx context.Context, col, id string, limit, offset int) ([]Order, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+orderCols+` FROM orders WHERE `+col+`=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		id, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()
	var out []Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *pgRepo) ListByBuyer(ctx context.Context, buyerID string, limit, offset int) ([]Order, error) {
	return r.list(ctx, "buyer_id", buyerID, limit, offset)
}
func (r *pgRepo) ListBySeller(ctx context.Context, sellerID string, limit, offset int) ([]Order, error) {
	return r.list(ctx, "seller_id", sellerID, limit, offset)
}

func (r *pgRepo) Transition(ctx context.Context, id, from, to string, setAutoConfirm bool) (Order, error) {
	autoConfirm := "auto_confirm_at"
	if setAutoConfirm {
		autoConfirm = "now() + interval '7 days'"
	}
	q := `UPDATE orders SET status=$3, auto_confirm_at=` + autoConfirm + `, updated_at=now()
	      WHERE id=$1 AND status=$2 RETURNING ` + orderCols
	out, err := scanOrder(r.pool.QueryRow(ctx, q, id, from, to))
	if errors.Is(err, pgx.ErrNoRows) {
		// Either the order doesn't exist or its status != from.
		return Order{}, ErrBadTransition
	}
	if err != nil {
		return Order{}, fmt.Errorf("transition order: %w", err)
	}
	return out, nil
}

func (r *pgRepo) CreateDispute(ctx context.Context, orderID, raisedBy, reason string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO disputes (order_id, raised_by, reason) VALUES ($1,$2,$3)`,
		orderID, raisedBy, reason)
	if err != nil {
		return fmt.Errorf("create dispute: %w", err)
	}
	return nil
}
