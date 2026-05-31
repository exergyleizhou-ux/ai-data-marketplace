package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository abstracts user persistence so the service can be unit-tested with
// an in-memory fake (see service_test.go) and swapped without touching logic.
type Repository interface {
	CreateUser(ctx context.Context, account, accountType, passwordHash string) (User, error)
	GetUserByAccount(ctx context.Context, account string) (User, string, error) // also returns password hash
	GetUserByID(ctx context.Context, id string) (User, error)
	UpdateUserRole(ctx context.Context, id, role string) (User, error)

	// SubmitKYC inserts a KYC record and flips the user's kyc_status to pending
	// atomically. idNoHash is the hashed ID number (raw value never persisted).
	SubmitKYC(ctx context.Context, rec KYCRecord, idNoHash string) (KYCRecord, error)
	// GetLatestKYC returns the user's most recent KYC submission.
	GetLatestKYC(ctx context.Context, userID string) (KYCRecord, error)
	// ReviewKYC sets a record's verify_status and syncs the owner's kyc_status,
	// atomically. Returns the updated record.
	ReviewKYC(ctx context.Context, kycID, newStatus, reviewerID string) (KYCRecord, error)
	// ListPendingKYC returns KYC submissions awaiting ops review (oldest first).
	ListPendingKYC(ctx context.Context, limit, offset int) ([]KYCRecord, error)
}

type pgRepo struct{ pool *pgxpool.Pool }

// NewRepository returns a Postgres-backed Repository.
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

const uniqueViolation = "23505"

func (r *pgRepo) CreateUser(ctx context.Context, account, accountType, passwordHash string) (User, error) {
	const q = `
		INSERT INTO users (account, account_type, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, account, account_type, role, kyc_status, status`
	var u User
	err := r.pool.QueryRow(ctx, q, account, accountType, passwordHash).
		Scan(&u.ID, &u.Account, &u.AccountType, &u.Role, &u.KYCStatus, &u.Status)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return User{}, ErrAccountExists
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

func (r *pgRepo) GetUserByAccount(ctx context.Context, account string) (User, string, error) {
	const q = `
		SELECT id, account, account_type, role, kyc_status, status, password_hash
		FROM users WHERE account = $1`
	var u User
	var hash string
	err := r.pool.QueryRow(ctx, q, account).
		Scan(&u.ID, &u.Account, &u.AccountType, &u.Role, &u.KYCStatus, &u.Status, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrUserNotFound
	}
	if err != nil {
		return User{}, "", fmt.Errorf("get user by account: %w", err)
	}
	return u, hash, nil
}

func (r *pgRepo) GetUserByID(ctx context.Context, id string) (User, error) {
	const q = `
		SELECT id, account, account_type, role, kyc_status, status
		FROM users WHERE id = $1`
	var u User
	err := r.pool.QueryRow(ctx, q, id).
		Scan(&u.ID, &u.Account, &u.AccountType, &u.Role, &u.KYCStatus, &u.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

func (r *pgRepo) UpdateUserRole(ctx context.Context, id, role string) (User, error) {
	const q = `
		UPDATE users SET role = $2, updated_at = now()
		WHERE id = $1
		RETURNING id, account, account_type, role, kyc_status, status`
	var u User
	err := r.pool.QueryRow(ctx, q, id, role).
		Scan(&u.ID, &u.Account, &u.AccountType, &u.Role, &u.KYCStatus, &u.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("update user role: %w", err)
	}
	return u, nil
}

func (r *pgRepo) SubmitKYC(ctx context.Context, rec KYCRecord, idNoHash string) (KYCRecord, error) {
	materials, err := json.Marshal(rec.MaterialURLs)
	if err != nil {
		return KYCRecord{}, fmt.Errorf("marshal material urls: %w", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return KYCRecord{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	const insertKYC = `
		INSERT INTO kyc_records (user_id, type, real_name, company_name, id_no_hash, material_urls, verify_status)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, 'pending')
		RETURNING id, verify_status`
	var out KYCRecord
	if err := tx.QueryRow(ctx, insertKYC,
		rec.UserID, rec.Type, nullify(rec.RealName), nullify(rec.CompanyName), idNoHash, string(materials),
	).Scan(&out.ID, &out.VerifyStatus); err != nil {
		return KYCRecord{}, fmt.Errorf("insert kyc: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE users SET kyc_status = 'pending', updated_at = now() WHERE id = $1`, rec.UserID,
	); err != nil {
		return KYCRecord{}, fmt.Errorf("mark user pending: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return KYCRecord{}, fmt.Errorf("commit: %w", err)
	}

	out.UserID = rec.UserID
	out.Type = rec.Type
	out.RealName = rec.RealName
	out.CompanyName = rec.CompanyName
	out.MaterialURLs = rec.MaterialURLs
	return out, nil
}

func (r *pgRepo) GetLatestKYC(ctx context.Context, userID string) (KYCRecord, error) {
	const q = `
		SELECT id, user_id, type, COALESCE(real_name, ''), COALESCE(company_name, ''),
		       material_urls, verify_status, created_at::text
		FROM kyc_records WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`
	var rec KYCRecord
	var materials []byte
	err := r.pool.QueryRow(ctx, q, userID).Scan(
		&rec.ID, &rec.UserID, &rec.Type, &rec.RealName, &rec.CompanyName,
		&materials, &rec.VerifyStatus, &rec.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return KYCRecord{}, ErrKYCNotFound
	}
	if err != nil {
		return KYCRecord{}, fmt.Errorf("get latest kyc: %w", err)
	}
	_ = json.Unmarshal(materials, &rec.MaterialURLs)
	return rec, nil
}

func (r *pgRepo) ListPendingKYC(ctx context.Context, limit, offset int) ([]KYCRecord, error) {
	const q = `
		SELECT id, user_id, type, COALESCE(real_name,''), COALESCE(company_name,''),
		       material_urls, verify_status, created_at::text
		FROM kyc_records WHERE verify_status='pending' ORDER BY created_at ASC LIMIT $1 OFFSET $2`
	rows, err := r.pool.Query(ctx, q, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list pending kyc: %w", err)
	}
	defer rows.Close()
	var out []KYCRecord
	for rows.Next() {
		var rec KYCRecord
		var materials []byte
		if err := rows.Scan(&rec.ID, &rec.UserID, &rec.Type, &rec.RealName, &rec.CompanyName,
			&materials, &rec.VerifyStatus, &rec.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(materials, &rec.MaterialURLs)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *pgRepo) ReviewKYC(ctx context.Context, kycID, newStatus, reviewerID string) (KYCRecord, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return KYCRecord{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	const updKYC = `
		UPDATE kyc_records SET verify_status = $2, reviewed_by = $3, reviewed_at = now()
		WHERE id = $1
		RETURNING id, user_id, type, COALESCE(real_name, ''), COALESCE(company_name, ''),
		          material_urls, verify_status, created_at::text`
	var rec KYCRecord
	var materials []byte
	err = tx.QueryRow(ctx, updKYC, kycID, newStatus, nullify(reviewerID)).Scan(
		&rec.ID, &rec.UserID, &rec.Type, &rec.RealName, &rec.CompanyName,
		&materials, &rec.VerifyStatus, &rec.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return KYCRecord{}, ErrKYCNotFound
	}
	if err != nil {
		return KYCRecord{}, fmt.Errorf("update kyc: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE users SET kyc_status = $2, updated_at = now() WHERE id = $1`, rec.UserID, newStatus,
	); err != nil {
		return KYCRecord{}, fmt.Errorf("sync user kyc_status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return KYCRecord{}, fmt.Errorf("commit: %w", err)
	}
	_ = json.Unmarshal(materials, &rec.MaterialURLs)
	return rec, nil
}

// nullify maps an empty string to nil so it lands as SQL NULL.
func nullify(s string) any {
	if s == "" {
		return nil
	}
	return s
}
