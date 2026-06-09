package qa

import "errors"

type Question struct {
	ID        string  `json:"id"`
	DatasetID string  `json:"dataset_id"`
	AskerID   string  `json:"asker_id"`
	AskerName string  `json:"asker_name,omitempty"`
	Body      string  `json:"body"`
	Status    string  `json:"status"`
	Answer    *Answer `json:"answer,omitempty"`
	CreatedAt string  `json:"created_at"`
}

type Answer struct {
	ID         string `json:"id"`
	QuestionID string `json:"question_id"`
	AnswererID string `json:"answerer_id"`
	Body       string `json:"body"`
	CreatedAt  string `json:"created_at"`
}

var (
	ErrQuestionNotFound = errors.New("question not found")
	ErrAlreadyAnswered  = errors.New("question already has an answer")
	ErrNotSeller        = errors.New("only the dataset seller can answer")
	ErrEmptyBody        = errors.New("body cannot be empty")
	ErrBodyTooLong      = errors.New("body exceeds 2000 characters")
)
