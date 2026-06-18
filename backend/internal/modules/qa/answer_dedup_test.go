package qa

import (
	"context"
	"errors"
	"testing"
)

// AnswerQuestion guards with a read (q.Answer != nil) then a separate write, so
// two concurrent/double-submitted answers can both pass the check and both
// insert — leaving a question with two answers (the 1:1 model + LEFT JOIN then
// surface an arbitrary one, and the asker is notified twice). CreateAnswer must
// reject the second answer.
func TestCreateAnswer_SecondAnswerRejected(t *testing.T) {
	repo, cleanup := testRepo(t)
	defer cleanup()
	ctx := context.Background()
	pool := repo.(*pgRepo).pool
	asker, _ := seedUser(t, pool, "dup-asker")
	seller, _ := seedUser(t, pool, "dup-seller")
	dsID := seedDataset(t, pool, seller)
	q, _ := repo.CreateQuestion(ctx, Question{DatasetID: dsID, AskerID: asker, Body: "?"})

	if _, err := repo.CreateAnswer(ctx, Answer{QuestionID: q.ID, AnswererID: seller, Body: "first"}); err != nil {
		t.Fatal(err)
	}
	_, err := repo.CreateAnswer(ctx, Answer{QuestionID: q.ID, AnswererID: seller, Body: "second"})
	if !errors.Is(err, ErrAlreadyAnswered) {
		t.Fatalf("second CreateAnswer err = %v, want ErrAlreadyAnswered", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM dataset_answers WHERE question_id=$1`, q.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("answer rows for question = %d, want 1", count)
	}
}
