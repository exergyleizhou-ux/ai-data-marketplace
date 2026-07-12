package workbench

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("workspace not found")

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (r *Repository) GetOrCreatePersonalWorkspace(ctx context.Context, uid string) (Workspace, error) {
	var w Workspace
	err := r.db.QueryRow(ctx, `INSERT INTO workbench_workspaces(account_id,slug,display_name) VALUES($1,'personal','Personal') ON CONFLICT(account_id,slug) DO UPDATE SET updated_at=workbench_workspaces.updated_at RETURNING id,account_id,slug,display_name,status`, uid).Scan(&w.ID, &w.AccountID, &w.Slug, &w.DisplayName, &w.Status)
	return w, err
}
func (r *Repository) GetOwned(ctx context.Context, id, uid string) (Workspace, error) {
	var w Workspace
	err := r.db.QueryRow(ctx, `SELECT id,account_id,slug,display_name,status FROM workbench_workspaces WHERE id=$1 AND account_id=$2 AND status='active'`, id, uid).Scan(&w.ID, &w.AccountID, &w.Slug, &w.DisplayName, &w.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return w, err
}
