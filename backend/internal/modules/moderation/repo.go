package moderation

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository is the moderation persistence boundary. Reporting and hiding span
// the content_reports, dataset_questions and reviews tables; moderation owns
// the cross-cutting concern and updates them directly.
type Repository interface {
	// CreateReport files a report. Re-reporting the same target while a prior
	// report is still open is a no-op (returns the existing open report) thanks
	// to the partial unique index — never an error.
	CreateReport(ctx context.Context, reporterID, targetType, targetID, reason string) (Report, error)
	ListReports(ctx context.Context, status string, limit, offset int) ([]Report, error)
	GetReport(ctx context.Context, id string) (Report, error)
	// Resolve marks the report resolved with the given resolution and, when
	// resolution=hide, hides the target content atomically in one transaction.
	Resolve(ctx context.Context, id, resolution, opsID string) (Report, error)
}

type pgRepo struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

const reportCols = `id::text, reporter_id::text, target_type, target_id::text, reason, status,
	COALESCE(resolution,''), created_at::text,
	COALESCE(resolved_at::text,''), COALESCE(resolved_by::text,'')`

func scanReport(row pgx.Row) (Report, error) {
	var r Report
	err := row.Scan(&r.ID, &r.ReporterID, &r.TargetType, &r.TargetID, &r.Reason, &r.Status,
		&r.Resolution, &r.CreatedAt, &r.ResolvedAt, &r.ResolvedBy)
	return r, err
}

func (r *pgRepo) CreateReport(ctx context.Context, reporterID, targetType, targetID, reason string) (Report, error) {
	// ON CONFLICT on the partial unique index (reporter, target) WHERE open:
	// a duplicate open report does nothing; we then read back the winning row.
	row := r.pool.QueryRow(ctx,
		`INSERT INTO content_reports (reporter_id, target_type, target_id, reason)
		 VALUES ($1::uuid, $2, $3::uuid, $4)
		 ON CONFLICT (reporter_id, target_type, target_id) WHERE status='open'
		 DO UPDATE SET reason = content_reports.reason
		 RETURNING `+reportCols,
		reporterID, targetType, targetID, reason)
	rep, err := scanReport(row)
	if err != nil {
		return Report{}, fmt.Errorf("create report: %w", err)
	}
	return rep, nil
}

func (r *pgRepo) ListReports(ctx context.Context, status string, limit, offset int) ([]Report, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	// status="" lists all; otherwise filter. The (status, created_at DESC)
	// index serves both.
	rows, err := r.pool.Query(ctx,
		`SELECT `+reportCols+` FROM content_reports
		 WHERE ($1 = '' OR status = $1)
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	defer rows.Close()
	var out []Report
	for rows.Next() {
		rep, err := scanReport(rows)
		if err != nil {
			return nil, fmt.Errorf("scan report: %w", err)
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}

func (r *pgRepo) GetReport(ctx context.Context, id string) (Report, error) {
	rep, err := scanReport(r.pool.QueryRow(ctx,
		`SELECT `+reportCols+` FROM content_reports WHERE id=$1::uuid`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrReportNotFound
	}
	if err != nil {
		return Report{}, fmt.Errorf("get report: %w", err)
	}
	return rep, nil
}

func (r *pgRepo) Resolve(ctx context.Context, id, resolution, opsID string) (Report, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// Atomic optimistic transition: only an 'open' report can be resolved.
	rep, err := scanReport(tx.QueryRow(ctx,
		`UPDATE content_reports
		 SET status='resolved', resolution=$2, resolved_at=now(), resolved_by=$3::uuid
		 WHERE id=$1::uuid AND status='open'
		 RETURNING `+reportCols,
		id, resolution, opsID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrReportNotFound
	}
	if err != nil {
		return Report{}, fmt.Errorf("resolve report: %w", err)
	}

	if resolution == ResolutionHide {
		if err := hideTarget(ctx, tx, rep.TargetType, rep.TargetID); err != nil {
			return Report{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Report{}, fmt.Errorf("commit: %w", err)
	}
	return rep, nil
}

func hideTarget(ctx context.Context, tx pgx.Tx, targetType, targetID string) error {
	switch targetType {
	case TargetQuestion:
		_, err := tx.Exec(ctx,
			`UPDATE dataset_questions SET status='hidden' WHERE id=$1::uuid`, targetID)
		if err != nil {
			return fmt.Errorf("hide question: %w", err)
		}
	case TargetReview:
		_, err := tx.Exec(ctx,
			`UPDATE reviews SET hidden=true WHERE id=$1::uuid`, targetID)
		if err != nil {
			return fmt.Errorf("hide review: %w", err)
		}
	}
	return nil
}
