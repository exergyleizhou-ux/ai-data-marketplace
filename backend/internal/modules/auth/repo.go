package auth

import (
	"context"
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
