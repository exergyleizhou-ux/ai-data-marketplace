package qa

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	CreateQuestion(ctx context.Context, q Question) (Question, error)
	CreateAnswer(ctx context.Context, a Answer) (Answer, error)
	ListByDataset(ctx context.Context, datasetID string, limit, offset int) ([]Question, error)
	GetQuestion(ctx context.Context, id string) (Question, error)
	SetQuestionStatus(ctx context.Context, id, status string) error
}

type pgRepo struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepo{pool: pool} }

func (r *pgRepo) CreateQuestion(ctx context.Context, q Question) (Question, error) {
	err := r.pool.QueryRow(ctx,
		`INSERT INTO dataset_questions (dataset_id, asker_id, body)
		 VALUES ($1,$2,$3)
		 RETURNING id::text, status, created_at::text`,
		q.DatasetID, q.AskerID, q.Body).Scan(&q.ID, &q.Status, &q.CreatedAt)
	if err != nil {
		return Question{}, fmt.Errorf("create question: %w", err)
	}
	return q, nil
}

func (r *pgRepo) CreateAnswer(ctx context.Context, a Answer) (Answer, error) {
	err := r.pool.QueryRow(ctx,
		`INSERT INTO dataset_answers (question_id, answerer_id, body)
		 VALUES ($1,$2,$3)
		 RETURNING id::text, created_at::text`,
		a.QuestionID, a.AnswererID, a.Body).Scan(&a.ID, &a.CreatedAt)
	if err != nil {
		return Answer{}, fmt.Errorf("create answer: %w", err)
	}
	return a, nil
}

func (r *pgRepo) ListByDataset(ctx context.Context, datasetID string, limit, offset int) ([]Question, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.pool.Query(ctx,
		`SELECT q.id::text, q.dataset_id::text, q.asker_id::text,
			COALESCE(SUBSTRING(u.account, 1, 8), ''),
			q.body, q.status, q.created_at::text,
			a.id::text, a.answerer_id::text, a.body, a.created_at::text
		 FROM dataset_questions q
		 JOIN users u ON u.id = q.asker_id
		 LEFT JOIN dataset_answers a ON a.question_id = q.id
		 WHERE q.dataset_id = $1 AND q.status != 'hidden'
		 ORDER BY q.created_at DESC
		 LIMIT $2 OFFSET $3`, datasetID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list questions: %w", err)
	}
	defer rows.Close()
	var out []Question
	for rows.Next() {
		var q Question
		var aID, aAnswererID, aBody, aCreatedAt sql.NullString
		if err := rows.Scan(&q.ID, &q.DatasetID, &q.AskerID, &q.AskerName,
			&q.Body, &q.Status, &q.CreatedAt,
			&aID, &aAnswererID, &aBody, &aCreatedAt); err != nil {
			return nil, err
		}
		if aID.Valid {
			q.Answer = &Answer{
				ID: aID.String, QuestionID: q.ID,
				AnswererID: aAnswererID.String, Body: aBody.String,
				CreatedAt: aCreatedAt.String,
			}
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

func (r *pgRepo) GetQuestion(ctx context.Context, id string) (Question, error) {
	var q Question
	var aID, aAnswererID, aBody, aCreatedAt sql.NullString
	err := r.pool.QueryRow(ctx,
		`SELECT q.id::text, q.dataset_id::text, q.asker_id::text,
			COALESCE(SUBSTRING(u.account, 1, 8), ''),
			q.body, q.status, q.created_at::text,
			a.id::text, a.answerer_id::text, a.body, a.created_at::text
		 FROM dataset_questions q
		 JOIN users u ON u.id = q.asker_id
		 LEFT JOIN dataset_answers a ON a.question_id = q.id
		 WHERE q.id = $1`, id).
		Scan(&q.ID, &q.DatasetID, &q.AskerID, &q.AskerName,
			&q.Body, &q.Status, &q.CreatedAt,
			&aID, &aAnswererID, &aBody, &aCreatedAt)
	if err != nil {
		return Question{}, fmt.Errorf("get question: %w", err)
	}
	if aID.Valid {
		q.Answer = &Answer{
			ID: aID.String, QuestionID: q.ID,
			AnswererID: aAnswererID.String, Body: aBody.String,
			CreatedAt: aCreatedAt.String,
		}
	}
	return q, nil
}

func (r *pgRepo) SetQuestionStatus(ctx context.Context, id, status string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE dataset_questions SET status=$2 WHERE id=$1 AND status!='hidden'`,
		id, status)
	if err != nil {
		return fmt.Errorf("set question status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrQuestionNotFound
	}
	return nil
}
