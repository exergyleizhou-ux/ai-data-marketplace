package qa

import (
	"context"
	"strings"
)

type DatasetReader interface {
	// SellerOf returns the dataset's seller_id and status.
	SellerOf(ctx context.Context, datasetID string) (sellerID, status string, err error)
}

type Notifier interface {
	NotifyUser(ctx context.Context, userID, kind, title, body, resourceType, resourceID string) error
}

type Service struct {
	repo     Repository
	ds       DatasetReader
	notifier Notifier
}

func NewService(repo Repository, ds DatasetReader, notifier Notifier) *Service {
	return &Service{repo: repo, ds: ds, notifier: notifier}
}

func (s *Service) AskQuestion(ctx context.Context, askerID, datasetID, body string) (Question, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Question{}, ErrEmptyBody
	}
	if len(body) > 2000 {
		return Question{}, ErrBodyTooLong
	}
	sellerID, status, err := s.ds.SellerOf(ctx, datasetID)
	if err != nil {
		return Question{}, ErrQuestionNotFound
	}
	if status != "published" && status != "reviewing" {
		return Question{}, ErrQuestionNotFound
	}
	q, err := s.repo.CreateQuestion(ctx, Question{
		DatasetID: datasetID, AskerID: askerID, Body: body, Status: "open",
	})
	if err != nil {
		return Question{}, err
	}
	if s.notifier != nil && sellerID != askerID {
		_ = s.notifier.NotifyUser(ctx, sellerID, "question_asked",
			"数据集有新提问", "您的数据集收到一条新提问,请前往详情页查看。",
			"dataset", datasetID)
	}
	return q, nil
}

func (s *Service) AnswerQuestion(ctx context.Context, answererID, questionID, body string) (Answer, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Answer{}, ErrEmptyBody
	}
	if len(body) > 2000 {
		return Answer{}, ErrBodyTooLong
	}
	q, err := s.repo.GetQuestion(ctx, questionID)
	if err != nil {
		return Answer{}, ErrQuestionNotFound
	}
	sellerID, _, err := s.ds.SellerOf(ctx, q.DatasetID)
	if err != nil {
		return Answer{}, ErrQuestionNotFound
	}
	if sellerID != answererID {
		return Answer{}, ErrNotSeller
	}
	if q.Answer != nil {
		return Answer{}, ErrAlreadyAnswered
	}
	a, err := s.repo.CreateAnswer(ctx, Answer{
		QuestionID: questionID, AnswererID: answererID, Body: body,
	})
	if err != nil {
		return Answer{}, err
	}
	_ = s.repo.SetQuestionStatus(ctx, questionID, "answered")
	if s.notifier != nil && q.AskerID != answererID {
		_ = s.notifier.NotifyUser(ctx, q.AskerID, "question_answered",
			"您的提问已被回答", "卖家已回答您关于数据集的提问。",
			"dataset", q.DatasetID)
	}
	return a, nil
}

func (s *Service) ListByDataset(ctx context.Context, datasetID string, limit, offset int) ([]Question, error) {
	return s.repo.ListByDataset(ctx, datasetID, limit, offset)
}
