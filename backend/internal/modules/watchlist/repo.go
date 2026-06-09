package watchlist

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns the dataset_watches table.
type Repository interface {
	Add(ctx context.Context, userID, datasetID string) error
	Remove(ctx context.Context, userID, datasetID string) error
	ListByUser(ctx context.Context, userID string) ([]Watch, error)
	ListByDataset(ctx context.Context, datasetID string) ([]userVersion, error)
	MarkNotified(ctx context.Context, userID, datasetID, versionID string) error
}

type userVersion struct {
	UserID    string
	VersionID string
}

type pgRepo struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

func (r *pgRepo) Add(ctx context.Context, userID, datasetID string) error {
	const q = `
		INSERT INTO dataset_watches (user_id, dataset_id, last_notified_version_id)
		SELECT $1, $2, current_version_id FROM datasets WHERE id = $2
		ON CONFLICT (user_id, dataset_id) DO NOTHING`
	if _, err := r.pool.Exec(ctx, q, userID, datasetID); err != nil {
		return fmt.Errorf("add watch: %w", err)
	}
	return nil
}

func (r *pgRepo) Remove(ctx context.Context, userID, datasetID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM dataset_watches WHERE user_id=$1 AND dataset_id=$2`, userID, datasetID)
	if err != nil {
		return fmt.Errorf("remove watch: %w", err)
	}
	return nil
}

func (r *pgRepo) ListByUser(ctx context.Context, userID string) ([]Watch, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT w.dataset_id::text, COALESCE(d.title, ''),
			COALESCE(w.last_notified_version_id::text, ''),
			w.created_at::text
		 FROM dataset_watches w
		 LEFT JOIN datasets d ON d.id = w.dataset_id
		 WHERE w.user_id = $1
		 ORDER BY w.created_at DESC
		 LIMIT 100`, userID)
	if err != nil {
		return nil, fmt.Errorf("list watches: %w", err)
	}
	defer rows.Close()
	var out []Watch
	for rows.Next() {
		var w Watch
		if err := rows.Scan(&w.DatasetID, &w.DatasetTitle, &w.LastNotifiedVersionID, &w.CreatedAt); err != nil {
			return nil, err
		}
		w.UserID = userID
		out = append(out, w)
	}
	return out, rows.Err()
}

func (r *pgRepo) ListByDataset(ctx context.Context, datasetID string) ([]userVersion, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, COALESCE(last_notified_version_id::text, '')
		 FROM dataset_watches WHERE dataset_id = $1`, datasetID)
	if err != nil {
		return nil, fmt.Errorf("list watchers: %w", err)
	}
	defer rows.Close()
	var out []userVersion
	for rows.Next() {
		var uv userVersion
		if err := rows.Scan(&uv.UserID, &uv.VersionID); err != nil {
			return nil, err
		}
		out = append(out, uv)
	}
	return out, rows.Err()
}

func (r *pgRepo) MarkNotified(ctx context.Context, userID, datasetID, versionID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE dataset_watches
		 SET last_notified_version_id = $3
		 WHERE user_id = $1 AND dataset_id = $2`, userID, datasetID, versionID)
	if err != nil {
		return fmt.Errorf("mark notified: %w", err)
	}
	return nil
}
