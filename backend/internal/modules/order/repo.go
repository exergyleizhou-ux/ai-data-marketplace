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
	// CreateCompute creates a compute (C2D) order: no version, license_type and
	// product_type both 'compute'. Same fee split + state machine as a download.
	CreateCompute(ctx context.Context, o Order) (Order, error)
	GetByID(ctx context.Context, id string) (Order, error)
	ListByBuyer(ctx context.Context, buyerID string, limit, offset int) ([]Order, error)
	ListBySeller(ctx context.Context, sellerID string, limit, offset int) ([]Order, error)
	// Transition moves an order from->to atomically; ErrBadTransition if the
	// current status is not `from`. setAutoConfirm sets auto_confirm_at = now()+7d
	// when moving to delivered.
	Transition(ctx context.Context, id, from, to string, setAutoConfirm bool) (Order, error)
	CreateDispute(ctx context.Context, orderID, raisedBy, reason string) error
	ResolveDispute(ctx context.Context, orderID, status, note, handledBy string) error
	SellerEarnings(ctx context.Context, sellerID string) (Earnings, error)
	CreateReview(ctx context.Context, r Review) (Review, error)
	ListReviewsByDataset(ctx context.Context, datasetID string, limit, offset int) ([]Review, error)
	AdminList(ctx context.Context, limit, offset int) ([]Order, error)
	// AdminReconciliation returns aggregate financial stats for the ops dashboard.
	AdminReconciliation(ctx context.Context) (Reconciliation, error)
	// AdminReconciliationTimeseries returns daily aggregates for the last N days (zero-filled).
	AdminReconciliationTimeseries(ctx context.Context, days int) ([]ReconciliationPoint, error)
	// SellerEarningsTimeseries returns the seller's daily earnings for the last N days.
	SellerEarningsTimeseries(ctx context.Context, sellerID string, days int) ([]EarningsPoint, error)
	// SellerEarningsByDataset returns per-dataset earnings for a seller.
	SellerEarningsByDataset(ctx context.Context, sellerID string) ([]EarningsByDataset, error)
}

type pgRepo struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

const orderCols = `id, buyer_id, seller_id, dataset_id, COALESCE(version_id::text,''), license_type,
	amount_cents, platform_fee_cents, seller_amount_cents, status, product_type,
	COALESCE(auto_confirm_at::text,''), created_at::text, updated_at::text`

func scanOrder(row pgx.Row) (Order, error) {
	var o Order
	err := row.Scan(&o.ID, &o.BuyerID, &o.SellerID, &o.DatasetID, &o.VersionID, &o.LicenseType,
		&o.AmountCents, &o.PlatformFeeCents, &o.SellerAmountCents, &o.Status, &o.ProductType,
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

func (r *pgRepo) CreateCompute(ctx context.Context, o Order) (Order, error) {
	const q = `
		INSERT INTO orders (buyer_id, seller_id, dataset_id, version_id, license_type,
			amount_cents, platform_fee_cents, seller_amount_cents, status, product_type)
		VALUES ($1,$2,$3,NULL,'compute',$4,$5,$6,'created','compute')
		RETURNING ` + orderCols
	out, err := scanOrder(r.pool.QueryRow(ctx, q,
		o.BuyerID, o.SellerID, o.DatasetID, o.AmountCents, o.PlatformFeeCents, o.SellerAmountCents))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return Order{}, ErrDuplicateOrder
		}
		return Order{}, fmt.Errorf("create compute order: %w", err)
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

func (r *pgRepo) ResolveDispute(ctx context.Context, orderID, status, note, handledBy string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE disputes SET status=$2, resolution_note=$3, handled_by=$4, resolved_at=now()
		 WHERE order_id=$1 AND status IN ('open','reviewing')`,
		orderID, status, note, handledBy)
	if err != nil {
		return fmt.Errorf("resolve dispute: %w", err)
	}
	return nil
}

func (r *pgRepo) SellerEarnings(ctx context.Context, sellerID string) (Earnings, error) {
	const q = `
		SELECT
		  COALESCE(SUM(seller_amount_cents) FILTER (WHERE status='settled'),0),
		  COALESCE(SUM(seller_amount_cents) FILTER (WHERE status IN ('paid','delivered','confirmed')),0),
		  COUNT(*) FILTER (WHERE status='settled'),
		  COUNT(*) FILTER (WHERE status IN ('paid','delivered','confirmed'))
		FROM orders WHERE seller_id=$1`
	var e Earnings
	if err := r.pool.QueryRow(ctx, q, sellerID).
		Scan(&e.SettledCents, &e.PendingCents, &e.SettledOrders, &e.PendingOrders); err != nil {
		return Earnings{}, fmt.Errorf("seller earnings: %w", err)
	}
	e.WithdrawableCents = e.SettledCents
	return e, nil
}

func (r *pgRepo) CreateReview(ctx context.Context, rv Review) (Review, error) {
	const q = `
		INSERT INTO reviews (order_id, dataset_id, buyer_id, score, comment, issue_flag)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),$6)
		RETURNING id, created_at::text`
	err := r.pool.QueryRow(ctx, q, rv.OrderID, rv.DatasetID, rv.BuyerID, rv.Score, rv.Comment, rv.IssueFlag).
		Scan(&rv.ID, &rv.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return Review{}, ErrReviewExists
		}
		return Review{}, fmt.Errorf("create review: %w", err)
	}
	return rv, nil
}

func (r *pgRepo) ListReviewsByDataset(ctx context.Context, datasetID string, limit, offset int) ([]Review, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, order_id, dataset_id, buyer_id, score, COALESCE(comment,''), issue_flag, created_at::text
		 FROM reviews WHERE dataset_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		datasetID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list reviews: %w", err)
	}
	defer rows.Close()
	var out []Review
	for rows.Next() {
		var rv Review
		if err := rows.Scan(&rv.ID, &rv.OrderID, &rv.DatasetID, &rv.BuyerID, &rv.Score, &rv.Comment, &rv.IssueFlag, &rv.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rv)
	}
	return out, rows.Err()
}

func (r *pgRepo) AdminList(ctx context.Context, limit, offset int) ([]Order, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+orderCols+` FROM orders ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("admin list orders: %w", err)
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

func (r *pgRepo) AdminReconciliation(ctx context.Context) (Reconciliation, error) {
	const q = `
		SELECT
			COALESCE(SUM(amount_cents), 0),
			COALESCE(SUM(amount_cents) FILTER (WHERE status='settled'), 0),
			COALESCE(SUM(platform_fee_cents) FILTER (WHERE status='settled'), 0),
			COUNT(*),
			COUNT(*) FILTER (WHERE status='settled'),
			COUNT(*) FILTER (WHERE status IN ('paid','delivered','confirmed')),
			COUNT(*) FILTER (WHERE status='disputed'),
			COUNT(*) FILTER (WHERE status='refunded'),
			COALESCE(SUM(amount_cents) FILTER (WHERE status='refunded'), 0)
		FROM orders`
	var rec Reconciliation
	if err := r.pool.QueryRow(ctx, q).Scan(
		&rec.TotalGMV, &rec.SettledGMV, &rec.PlatformFees, &rec.TotalOrders,
		&rec.SettledOrders, &rec.PendingOrders, &rec.DisputedOrders,
		&rec.RefundedOrders, &rec.RefundedAmount); err != nil {
		return Reconciliation{}, fmt.Errorf("admin reconciliation: %w", err)
	}
	_ = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM settlement_outbox WHERE status='failed'`).Scan(&rec.FailedSettlements)
	return rec, nil
}

func (r *pgRepo) AdminReconciliationTimeseries(ctx context.Context, days int) ([]ReconciliationPoint, error) {
	if days < 1 || days > 90 {
		days = 30
	}
	const q = `
		WITH days AS (
			SELECT (CURRENT_DATE - i)::date AS d
			FROM generate_series(0, $1 - 1) AS i
		),
		agg AS (
			SELECT
				DATE(created_at AT TIME ZONE 'UTC') AS d,
				COALESCE(SUM(amount_cents), 0)                                              AS gmv_cents,
				COALESCE(SUM(amount_cents) FILTER (WHERE status='settled'), 0)              AS settled_gmv_cents,
				COALESCE(SUM(platform_fee_cents) FILTER (WHERE status='settled'), 0)        AS platform_fees_cents,
				COUNT(*)                                                                    AS orders,
				COUNT(*) FILTER (WHERE status='settled')                                    AS settled_orders,
				COUNT(*) FILTER (WHERE status='refunded')                                   AS refunded_orders,
				COUNT(*) FILTER (WHERE status='disputed')                                   AS disputed_orders
			FROM orders
			WHERE created_at >= CURRENT_DATE - ($1 - 1) * INTERVAL '1 day'
			GROUP BY d
		),
		outbox_agg AS (
			SELECT
				DATE(updated_at AT TIME ZONE 'UTC') AS d,
				COUNT(*) AS failed_settlements
			FROM settlement_outbox
			WHERE status = 'failed'
				AND updated_at >= CURRENT_DATE - ($1 - 1) * INTERVAL '1 day'
			GROUP BY d
		)
		SELECT
			days.d::text,
			COALESCE(agg.gmv_cents, 0),
			COALESCE(agg.settled_gmv_cents, 0),
			COALESCE(agg.platform_fees_cents, 0),
			COALESCE(agg.orders, 0),
			COALESCE(agg.settled_orders, 0),
			COALESCE(agg.refunded_orders, 0),
			COALESCE(agg.disputed_orders, 0),
			COALESCE(outbox_agg.failed_settlements, 0)
		FROM days
		LEFT JOIN agg        USING (d)
		LEFT JOIN outbox_agg USING (d)
		ORDER BY days.d`
	rows, err := r.pool.Query(ctx, q, days)
	if err != nil {
		// settlement_outbox may not exist — retry without the outbox join.
		const qNoOutbox = `
		WITH days AS (
			SELECT (CURRENT_DATE - i)::date AS d
			FROM generate_series(0, $1 - 1) AS i
		),
		agg AS (
			SELECT
				DATE(created_at AT TIME ZONE 'UTC') AS d,
				COALESCE(SUM(amount_cents), 0)                                              AS gmv_cents,
				COALESCE(SUM(amount_cents) FILTER (WHERE status='settled'), 0)              AS settled_gmv_cents,
				COALESCE(SUM(platform_fee_cents) FILTER (WHERE status='settled'), 0)        AS platform_fees_cents,
				COUNT(*)                                                                    AS orders,
				COUNT(*) FILTER (WHERE status='settled')                                    AS settled_orders,
				COUNT(*) FILTER (WHERE status='refunded')                                   AS refunded_orders,
				COUNT(*) FILTER (WHERE status='disputed')                                   AS disputed_orders
			FROM orders
			WHERE created_at >= CURRENT_DATE - ($1 - 1) * INTERVAL '1 day'
			GROUP BY d
		)
		SELECT days.d::text,
			COALESCE(agg.gmv_cents, 0),
			COALESCE(agg.settled_gmv_cents, 0),
			COALESCE(agg.platform_fees_cents, 0),
			COALESCE(agg.orders, 0),
			COALESCE(agg.settled_orders, 0),
			COALESCE(agg.refunded_orders, 0),
			COALESCE(agg.disputed_orders, 0),
			0
		FROM days LEFT JOIN agg USING (d) ORDER BY days.d`
		rows, err = r.pool.Query(ctx, qNoOutbox, days)
	}
	if err != nil {
		return nil, fmt.Errorf("admin timeseries: %w", err)
	}
	defer rows.Close()
	var out []ReconciliationPoint
	for rows.Next() {
		var p ReconciliationPoint
		if err := rows.Scan(&p.Date, &p.GMVCents, &p.SettledGMVCents, &p.PlatformFeesCents,
			&p.Orders, &p.SettledOrders, &p.RefundedOrders, &p.DisputedOrders, &p.FailedSettlements); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *pgRepo) SellerEarningsTimeseries(ctx context.Context, sellerID string, days int) ([]EarningsPoint, error) {
	if days < 1 || days > 90 {
		days = 30
	}
	const q = `
		WITH days AS (
			SELECT (CURRENT_DATE - i)::date AS d
			FROM generate_series(0, $1 - 1) AS i
		),
		agg AS (
			SELECT
				DATE(created_at AT TIME ZONE 'UTC') AS d,
				COALESCE(SUM(amount_cents), 0)                                              AS gross_cents,
				COALESCE(SUM(seller_amount_cents) FILTER (WHERE status='settled'), 0)       AS settled_cents,
				COUNT(*)                                                                    AS orders,
				COUNT(*) FILTER (WHERE status='settled')                                    AS settled_orders,
				COALESCE(SUM(amount_cents) FILTER (WHERE status='refunded'), 0)             AS refunded_cents
			FROM orders
			WHERE seller_id = $2
				AND created_at >= CURRENT_DATE - ($1 - 1) * INTERVAL '1 day'
			GROUP BY d
		)
		SELECT
			days.d::text,
			COALESCE(agg.gross_cents, 0),
			COALESCE(agg.settled_cents, 0),
			COALESCE(agg.orders, 0),
			COALESCE(agg.settled_orders, 0),
			COALESCE(agg.refunded_cents, 0)
		FROM days
		LEFT JOIN agg USING (d)
		ORDER BY days.d`
	rows, err := r.pool.Query(ctx, q, days, sellerID)
	if err != nil {
		return nil, fmt.Errorf("seller earnings timeseries: %w", err)
	}
	defer rows.Close()
	var out []EarningsPoint
	for rows.Next() {
		var p EarningsPoint
		if err := rows.Scan(&p.Date, &p.GrossCents, &p.SettledCents, &p.Orders, &p.SettledOrders, &p.RefundedCents); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *pgRepo) SellerEarningsByDataset(ctx context.Context, sellerID string) ([]EarningsByDataset, error) {
	const q = `
		SELECT
			o.dataset_id::text,
			COALESCE(d.title, ''),
			COUNT(*) AS total_orders,
			COUNT(*) FILTER (WHERE o.status='settled') AS settled_orders,
			COALESCE(SUM(o.amount_cents), 0) AS gross_cents,
			COALESCE(SUM(o.seller_amount_cents) FILTER (WHERE o.status='settled'), 0) AS settled_cents,
			COALESCE(MAX(o.created_at)::text, '') AS last_order_at
		FROM orders o
		LEFT JOIN datasets d ON d.id = o.dataset_id
		WHERE o.seller_id = $1
		GROUP BY o.dataset_id, d.title
		ORDER BY settled_cents DESC, total_orders DESC
		LIMIT 200`
	rows, err := r.pool.Query(ctx, q, sellerID)
	if err != nil {
		return nil, fmt.Errorf("seller earnings by dataset: %w", err)
	}
	defer rows.Close()
	var out []EarningsByDataset
	for rows.Next() {
		var e EarningsByDataset
		if err := rows.Scan(&e.DatasetID, &e.Title, &e.TotalOrders, &e.SettledOrders, &e.GrossCents, &e.SettledCents, &e.LastOrderAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
