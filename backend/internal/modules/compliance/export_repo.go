package compliance

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ExportRepository interface {
	Create(ctx context.Context, userID string) (ExportJob, error)
	FindRecentByUser(ctx context.Context, userID string) (ExportJob, error)
	SetGenerating(ctx context.Context, id string) error
	SetReady(ctx context.Context, id string, objectKey string, objectBytes int64, expiresAt time.Time) error
	SetFailed(ctx context.Context, id, errMsg string) error
	ExpireOldJobs(ctx context.Context) error
	// PurgeByUser deletes all of a user's export job rows and returns the object
	// keys that were set, so the caller can delete the backing objects too
	// (right-to-erasure: the export zip is a full PII snapshot).
	PurgeByUser(ctx context.Context, userID string) ([]string, error)
}

type pgExportRepo struct{ pool *pgxpool.Pool }

func NewExportRepository(pool *pgxpool.Pool) ExportRepository { return &pgExportRepo{pool: pool} }

func (r *pgExportRepo) PurgeByUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`DELETE FROM data_export_jobs WHERE user_id=$1 RETURNING object_key`, userID)
	if err != nil {
		return nil, fmt.Errorf("purge export jobs: %w", err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k *string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		if k != nil && *k != "" {
			keys = append(keys, *k)
		}
	}
	return keys, rows.Err()
}

func (r *pgExportRepo) Create(ctx context.Context, userID string) (ExportJob, error) {
	var j ExportJob
	err := r.pool.QueryRow(ctx,
		`INSERT INTO data_export_jobs (user_id) VALUES ($1)
		 RETURNING id::text, user_id::text, status, requested_at::text`,
		userID).Scan(&j.ID, &j.UserID, &j.Status, &j.RequestedAt)
	if err != nil {
		return ExportJob{}, fmt.Errorf("create export job: %w", err)
	}
	return j, nil
}

func (r *pgExportRepo) FindRecentByUser(ctx context.Context, userID string) (ExportJob, error) {
	var j ExportJob
	err := r.pool.QueryRow(ctx,
		`SELECT id::text, user_id::text, status,
			COALESCE(object_key,''), COALESCE(object_bytes,0),
			COALESCE(expires_at::text,''), COALESCE(error,''),
			requested_at::text, COALESCE(ready_at::text,'')
		 FROM data_export_jobs
		 WHERE user_id = $1
		 ORDER BY requested_at DESC LIMIT 1`, userID).
		Scan(&j.ID, &j.UserID, &j.Status,
			&j.DownloadURL, &j.ObjectBytes,
			&j.ExpiresAt, &j.Error,
			&j.RequestedAt, &j.ReadyAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ExportJob{}, ErrNotFound
		}
		return ExportJob{}, fmt.Errorf("find recent export: %w", err)
	}
	return j, nil
}

func (r *pgExportRepo) SetGenerating(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE data_export_jobs SET status='generating' WHERE id=$1`, id)
	return err
}

func (r *pgExportRepo) SetReady(ctx context.Context, id string, objectKey string, objectBytes int64, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE data_export_jobs SET status='ready', object_key=$2, object_bytes=$3, expires_at=$4, ready_at=now()
		 WHERE id=$1`, id, objectKey, objectBytes, expiresAt)
	return err
}

func (r *pgExportRepo) SetFailed(ctx context.Context, id, errMsg string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE data_export_jobs SET status='failed', error=$2 WHERE id=$1`, id, errMsg)
	return err
}

func (r *pgExportRepo) ExpireOldJobs(ctx context.Context) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE data_export_jobs SET status='expired' WHERE status='ready' AND expires_at < now()`)
	return err
}
