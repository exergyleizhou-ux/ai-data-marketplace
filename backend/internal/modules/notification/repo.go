package notification

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns the notifications table.
type Repository interface {
	Create(ctx context.Context, n Notification) (Notification, error)
	ListByUser(ctx context.Context, userID string, limit, offset int) ([]Notification, error)
	MarkRead(ctx context.Context, id, userID string) error
	MarkAllRead(ctx context.Context, userID string) (int64, error)
	CountUnread(ctx context.Context, userID string) (int64, error)
}

type pgRepo struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

func (r *pgRepo) Create(ctx context.Context, n Notification) (Notification, error) {
	const q = `
		INSERT INTO notifications (user_id, kind, title, body, resource_type, resource_id)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id, is_read, created_at::text`
	err := r.pool.QueryRow(ctx, q, n.UserID, n.Kind, n.Title, n.Body, n.ResourceType, n.ResourceID).
		Scan(&n.ID, &n.IsRead, &n.CreatedAt)
	if err != nil {
		return Notification{}, fmt.Errorf("create notification: %w", err)
	}
	return n, nil
}

func (r *pgRepo) ListByUser(ctx context.Context, userID string, limit, offset int) ([]Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, kind, title, COALESCE(body,''), COALESCE(resource_type,''), COALESCE(resource_id,''),
			is_read, created_at::text
		 FROM notifications WHERE user_id=$1
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Kind, &n.Title, &n.Body, &n.ResourceType, &n.ResourceID, &n.IsRead, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *pgRepo) MarkRead(ctx context.Context, id, userID string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE notifications SET is_read=true WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgRepo) MarkAllRead(ctx context.Context, userID string) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE notifications SET is_read=true WHERE user_id=$1 AND is_read=false`, userID)
	if err != nil {
		return 0, fmt.Errorf("mark all read: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *pgRepo) CountUnread(ctx context.Context, userID string) (int64, error) {
	var n int64
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND is_read=false`, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count unread: %w", err)
	}
	return n, nil
}
