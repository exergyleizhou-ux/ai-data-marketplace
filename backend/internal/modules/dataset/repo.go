package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/textseg"
)

// searchText builds the segmented text indexed for full-text search.
func searchText(d Dataset) string {
	return textseg.Segment(d.Title + " " + d.Description + " " + d.Domain)
}

// Repository abstracts dataset persistence (owns dataset/dataset_version/
// dataset_file). Service logic is unit-tested against an in-memory fake.
type Repository interface {
	Create(ctx context.Context, d Dataset) (Dataset, error)
	GetByID(ctx context.Context, id string) (Dataset, error)
	UpdateMeta(ctx context.Context, d Dataset) (Dataset, error)
	ListBySeller(ctx context.Context, sellerID string, limit, offset int) ([]Dataset, error)
	SignSource(ctx context.Context, id string) (Dataset, error)
	SetStatus(ctx context.Context, id, status string) error
	// AddVersion creates a version + file row and points the dataset at it,
	// updating size/status — all in one transaction. Returns the version id.
	AddVersion(ctx context.Context, datasetID, contentSHA256, simhash string, f FileInput, newStatus string) (string, error)
	// SaveQualityCheck persists one quality_check row.
	SaveQualityCheck(ctx context.Context, datasetID, versionID, checkType, result string, report any) error
	// ListQualityChecks returns the quality_check rows for a dataset's current
	// version (the buyer-facing quality report), oldest first.
	ListQualityChecks(ctx context.Context, datasetID string) ([]QualityCheck, error)
	// ContentDupExists reports whether another dataset already has a version
	// with the same content hash (exact resale / duplicate upload).
	ContentDupExists(ctx context.Context, contentSHA256, excludeDatasetID string) (bool, error)
	// SetSampleCount records the dataset's sample (non-empty line) count.
	SetSampleCount(ctx context.Context, id string, n int64) error
	// CurrentObjectKey returns the object key of the dataset's current version
	// file (single-file MVP) — used by delivery to stream the bytes.
	CurrentObjectKey(ctx context.Context, datasetID string) (string, error)
	// ListPublished returns published datasets matching the filter (browse/search).
	ListPublished(ctx context.Context, f ListFilter) ([]Dataset, error)
	// SetVersionSimhash stores the near-dup fingerprint computed by the quality worker.
	SetVersionSimhash(ctx context.Context, versionID, simhash string) error
	// ListByStatus returns datasets in a given lifecycle status (ops queues).
	ListByStatus(ctx context.Context, status string, limit, offset int) ([]Dataset, error)
}

// ListFilter is the public catalog query (only published datasets are returned).
type ListFilter struct {
	Keyword       string // substring match on title/description (CJK-safe via ILIKE)
	DataType      string
	LicenseType   string
	Domain        string
	MinPriceCents int64
	MaxPriceCents int64  // 0 = no upper bound
	Sort          string // newest | price_asc | price_desc
	Limit         int
	Offset        int
}

// FileInput describes one stored object to attach to a dataset version.
type FileInput struct {
	ObjectKey   string
	SizeBytes   int64
	SHA256      string
	ContentType string
}

type pgRepo struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

const datasetCols = `id, seller_id, title, description, data_type, COALESCE(domain,''),
	license_type, suggested_price_cents, final_price_cents, status,
	total_size_bytes, sample_count, source_declaration,
	source_signed_at::text, COALESCE(current_version_id::text,''),
	created_at::text, updated_at::text`

func scanDataset(row pgx.Row) (Dataset, error) {
	var d Dataset
	var decl []byte
	var signedAt *string
	if err := row.Scan(
		&d.ID, &d.SellerID, &d.Title, &d.Description, &d.DataType, &d.Domain,
		&d.LicenseType, &d.SuggestedPriceCents, &d.FinalPriceCents, &d.Status,
		&d.TotalSizeBytes, &d.SampleCount, &decl,
		&signedAt, &d.CurrentVersionID, &d.CreatedAt, &d.UpdatedAt,
	); err != nil {
		return Dataset{}, err
	}
	if signedAt != nil {
		d.SourceSignedAt = *signedAt
	}
	if len(decl) > 0 {
		_ = json.Unmarshal(decl, &d.SourceDeclaration)
	}
	return d, nil
}

func (r *pgRepo) Create(ctx context.Context, d Dataset) (Dataset, error) {
	decl, _ := json.Marshal(d.SourceDeclaration)
	const q = `
		INSERT INTO datasets (seller_id, title, description, data_type, domain,
			license_type, suggested_price_cents, status, source_declaration, search_vector)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7,'draft',$8::jsonb, to_tsvector('simple',$9))
		RETURNING ` + datasetCols
	out, err := scanDataset(r.pool.QueryRow(ctx, q,
		d.SellerID, d.Title, d.Description, d.DataType, d.Domain,
		d.LicenseType, d.SuggestedPriceCents, string(decl), searchText(d)))
	if err != nil {
		return Dataset{}, fmt.Errorf("create dataset: %w", err)
	}
	return out, nil
}

func (r *pgRepo) GetByID(ctx context.Context, id string) (Dataset, error) {
	out, err := scanDataset(r.pool.QueryRow(ctx, `SELECT `+datasetCols+` FROM datasets WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Dataset{}, ErrNotFound
	}
	if err != nil {
		return Dataset{}, fmt.Errorf("get dataset: %w", err)
	}
	return out, nil
}

func (r *pgRepo) UpdateMeta(ctx context.Context, d Dataset) (Dataset, error) {
	decl, _ := json.Marshal(d.SourceDeclaration)
	const q = `
		UPDATE datasets SET title=$2, description=$3, data_type=$4, domain=NULLIF($5,''),
			license_type=$6, suggested_price_cents=$7, source_declaration=$8::jsonb,
			search_vector=to_tsvector('simple',$9), updated_at=now()
		WHERE id=$1
		RETURNING ` + datasetCols
	out, err := scanDataset(r.pool.QueryRow(ctx, q,
		d.ID, d.Title, d.Description, d.DataType, d.Domain,
		d.LicenseType, d.SuggestedPriceCents, string(decl), searchText(d)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Dataset{}, ErrNotFound
	}
	if err != nil {
		return Dataset{}, fmt.Errorf("update dataset: %w", err)
	}
	return out, nil
}

func (r *pgRepo) ListBySeller(ctx context.Context, sellerID string, limit, offset int) ([]Dataset, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+datasetCols+` FROM datasets WHERE seller_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		sellerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list datasets: %w", err)
	}
	defer rows.Close()
	var out []Dataset
	for rows.Next() {
		d, err := scanDataset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *pgRepo) SetStatus(ctx context.Context, id, status string) error {
	ct, err := r.pool.Exec(ctx, `UPDATE datasets SET status=$2, updated_at=now() WHERE id=$1`, id, status)
	if err != nil {
		return fmt.Errorf("set status: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgRepo) AddVersion(ctx context.Context, datasetID, contentSHA256, simhash string, f FileInput, newStatus string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	var versionNo int
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(version_no),0)+1 FROM dataset_versions WHERE dataset_id=$1`, datasetID,
	).Scan(&versionNo); err != nil {
		return "", fmt.Errorf("next version_no: %w", err)
	}

	var versionID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO dataset_versions (dataset_id, version_no, content_sha256, simhash)
		 VALUES ($1,$2,$3,NULLIF($4,'')) RETURNING id`,
		datasetID, versionNo, contentSHA256, simhash,
	).Scan(&versionID); err != nil {
		return "", fmt.Errorf("insert version: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO dataset_files (dataset_id, version_id, object_key, size_bytes, sha256, content_type)
		 VALUES ($1,$2,$3,$4,$5,NULLIF($6,''))`,
		datasetID, versionID, f.ObjectKey, f.SizeBytes, f.SHA256, f.ContentType,
	); err != nil {
		return "", fmt.Errorf("insert file: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE datasets SET current_version_id=$2, total_size_bytes=$3, status=$4, updated_at=now() WHERE id=$1`,
		datasetID, versionID, f.SizeBytes, newStatus,
	); err != nil {
		return "", fmt.Errorf("update dataset: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return versionID, nil
}

func (r *pgRepo) SaveQualityCheck(ctx context.Context, datasetID, versionID, checkType, result string, report any) error {
	rep, _ := json.Marshal(report)
	_, err := r.pool.Exec(ctx,
		`INSERT INTO quality_checks (dataset_id, version_id, type, result, report) VALUES ($1,$2,$3,$4,$5::jsonb)`,
		datasetID, versionID, checkType, result, string(rep))
	if err != nil {
		return fmt.Errorf("save quality check: %w", err)
	}
	return nil
}

func (r *pgRepo) ContentDupExists(ctx context.Context, contentSHA256, excludeDatasetID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM dataset_versions WHERE content_sha256=$1 AND dataset_id<>$2)`,
		contentSHA256, excludeDatasetID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("content dup check: %w", err)
	}
	return exists, nil
}

func (r *pgRepo) SetSampleCount(ctx context.Context, id string, n int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE datasets SET sample_count=$2, updated_at=now() WHERE id=$1`, id, n)
	if err != nil {
		return fmt.Errorf("set sample count: %w", err)
	}
	return nil
}

func (r *pgRepo) ListPublished(ctx context.Context, f ListFilter) ([]Dataset, error) {
	// effPrice = COALESCE(final, suggested, 0)
	const effPrice = `COALESCE(final_price_cents, suggested_price_cents, 0)`
	conds := []string{"status = 'published'"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if f.DataType != "" {
		add("data_type = $%d", f.DataType)
	}
	if f.LicenseType != "" {
		add("license_type = $%d", f.LicenseType)
	}
	if f.Domain != "" {
		add("domain = $%d", f.Domain)
	}
	if f.MinPriceCents > 0 {
		add(effPrice+" >= $%d", f.MinPriceCents)
	}
	if f.MaxPriceCents > 0 {
		add(effPrice+" <= $%d", f.MaxPriceCents)
	}
	// Keyword search: segment the query (Chinese word tokens) and match the
	// GIN-indexed tsvector, ranking by relevance unless an explicit sort wins.
	kwArg := 0
	if f.Keyword != "" {
		args = append(args, textseg.Segment(f.Keyword))
		kwArg = len(args)
		conds = append(conds, fmt.Sprintf("search_vector @@ plainto_tsquery('simple', $%d)", kwArg))
	}

	order := "created_at DESC"
	switch f.Sort {
	case "price_asc":
		order = effPrice + " ASC, created_at DESC"
	case "price_desc":
		order = effPrice + " DESC, created_at DESC"
	default:
		if kwArg > 0 {
			order = fmt.Sprintf("ts_rank(search_vector, plainto_tsquery('simple', $%d)) DESC, created_at DESC", kwArg)
		}
	}

	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)
	q := `SELECT ` + datasetCols + ` FROM datasets WHERE ` +
		strings.Join(conds, " AND ") +
		` ORDER BY ` + order +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list published: %w", err)
	}
	defer rows.Close()
	var out []Dataset
	for rows.Next() {
		d, err := scanDataset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.attachQualitySummaries(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// attachQualitySummaries batch-loads a browse-time quality signal for the given
// datasets and sets QualityVerified / AuthenticityBand / AuthenticityScore in
// place. One round-trip for the whole page. AuthenticityBand/Score are only set
// for datasets whose authenticity check was actually applicable (tabular).
func (r *pgRepo) attachQualitySummaries(ctx context.Context, datasets []Dataset) error {
	versionIDs := make([]string, 0, len(datasets))
	for _, d := range datasets {
		if d.CurrentVersionID != "" {
			versionIDs = append(versionIDs, d.CurrentVersionID)
		}
	}
	if len(versionIDs) == 0 {
		return nil
	}
	const q = `
		SELECT version_id::text,
		       count(*)                  AS n_checks,
		       bool_or(result = 'fail')  AS any_fail,
		       max((report->>'score')::int) FILTER (
		           WHERE type = 'authenticity' AND report->>'applicable' = 'true') AS auth_score,
		       max(report->>'band')         FILTER (
		           WHERE type = 'authenticity' AND report->>'applicable' = 'true') AS auth_band
		FROM quality_checks
		WHERE version_id = ANY($1)
		GROUP BY version_id`
	rows, err := r.pool.Query(ctx, q, versionIDs)
	if err != nil {
		return fmt.Errorf("quality summaries: %w", err)
	}
	defer rows.Close()

	type summary struct {
		verified bool
		score    *int
		band     string
	}
	byVersion := map[string]summary{}
	for rows.Next() {
		var (
			vid     string
			n       int
			anyFail *bool
			score   *int
			band    *string
		)
		if err := rows.Scan(&vid, &n, &anyFail, &score, &band); err != nil {
			return fmt.Errorf("scan quality summary: %w", err)
		}
		s := summary{verified: n > 0 && (anyFail == nil || !*anyFail), score: score}
		if band != nil {
			s.band = *band
		}
		byVersion[vid] = s
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range datasets {
		if s, ok := byVersion[datasets[i].CurrentVersionID]; ok {
			v := s.verified
			datasets[i].QualityVerified = &v
			datasets[i].AuthenticityScore = s.score
			datasets[i].AuthenticityBand = s.band
		}
	}
	return nil
}

func (r *pgRepo) ListByStatus(ctx context.Context, status string, limit, offset int) ([]Dataset, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+datasetCols+` FROM datasets WHERE status=$1 ORDER BY updated_at ASC LIMIT $2 OFFSET $3`,
		status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list by status: %w", err)
	}
	defer rows.Close()
	var out []Dataset
	for rows.Next() {
		d, err := scanDataset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *pgRepo) SetVersionSimhash(ctx context.Context, versionID, simhash string) error {
	_, err := r.pool.Exec(ctx, `UPDATE dataset_versions SET simhash=NULLIF($2,'') WHERE id=$1`, versionID, simhash)
	if err != nil {
		return fmt.Errorf("set version simhash: %w", err)
	}
	return nil
}

func (r *pgRepo) ListQualityChecks(ctx context.Context, datasetID string) ([]QualityCheck, error) {
	const q = `
		SELECT qc.type, qc.result, COALESCE(qc.report, '{}'::jsonb), qc.created_at
		FROM quality_checks qc
		JOIN datasets d ON d.current_version_id = qc.version_id
		WHERE d.id=$1
		ORDER BY qc.created_at`
	rows, err := r.pool.Query(ctx, q, datasetID)
	if err != nil {
		return nil, fmt.Errorf("list quality checks: %w", err)
	}
	defer rows.Close()

	var out []QualityCheck
	for rows.Next() {
		var (
			qc      QualityCheck
			raw     []byte
			created time.Time
		)
		if err := rows.Scan(&qc.Type, &qc.Result, &raw, &created); err != nil {
			return nil, fmt.Errorf("scan quality check: %w", err)
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &qc.Report)
		}
		qc.CreatedAt = created.UTC().Format(time.RFC3339)
		out = append(out, qc)
	}
	return out, rows.Err()
}

func (r *pgRepo) CurrentObjectKey(ctx context.Context, datasetID string) (string, error) {
	const q = `
		SELECT f.object_key
		FROM datasets d JOIN dataset_files f ON f.version_id = d.current_version_id
		WHERE d.id=$1 LIMIT 1`
	var key string
	err := r.pool.QueryRow(ctx, q, datasetID).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("current object key: %w", err)
	}
	return key, nil
}

func (r *pgRepo) SignSource(ctx context.Context, id string) (Dataset, error) {
	const q = `UPDATE datasets SET source_signed_at=now(), updated_at=now() WHERE id=$1 RETURNING ` + datasetCols
	out, err := scanDataset(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Dataset{}, ErrNotFound
	}
	if err != nil {
		return Dataset{}, fmt.Errorf("sign source: %w", err)
	}
	return out, nil
}
