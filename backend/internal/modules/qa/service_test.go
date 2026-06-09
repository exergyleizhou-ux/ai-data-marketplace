package qa

import (
	"context"
	"strings"
	"sync"
	"testing"
)

type fakeQARepo struct {
	questions map[string]Question
	answers   map[string]Answer
}

func (r *fakeQARepo) CreateQuestion(_ context.Context, q Question) (Question, error) {
	q.ID = "q-" + q.Body[:min(4, len(q.Body))]
	q.Status = "open"
	q.CreatedAt = "2026-01-01T00:00:00Z"
	if r.questions == nil {
		r.questions = map[string]Question{}
	}
	r.questions[q.ID] = q
	return q, nil
}
func (r *fakeQARepo) CreateAnswer(_ context.Context, a Answer) (Answer, error) {
	a.ID = "a-" + a.Body[:min(4, len(a.Body))]
	a.CreatedAt = "2026-01-01T00:00:01Z"
	if r.answers == nil {
		r.answers = map[string]Answer{}
	}
	r.answers[a.QuestionID] = a
	return a, nil
}
func (r *fakeQARepo) ListByDataset(_ context.Context, _ string, _, _ int) ([]Question, error) {
	return nil, nil
}
func (r *fakeQARepo) GetQuestion(_ context.Context, id string) (Question, error) {
	q, ok := r.questions[id]
	if !ok {
		return Question{}, ErrQuestionNotFound
	}
	if a, y := r.answers[id]; y {
		q.Answer = &a
	}
	return q, nil
}
func (r *fakeQARepo) SetQuestionStatus(_ context.Context, id, status string) error {
	q, ok := r.questions[id]
	if !ok {
		return ErrQuestionNotFound
	}
	q.Status = status
	r.questions[id] = q
	return nil
}

type fakeDSReader struct {
	sellers map[string]string
	status  map[string]string
}

func (f *fakeDSReader) SellerOf(_ context.Context, datasetID string) (string, string, error) {
	s, ok := f.sellers[datasetID]
	if !ok {
		return "", "", ErrQuestionNotFound
	}
	st := f.status[datasetID]
	if st == "" {
		st = "published"
	}
	return s, st, nil
}

type fakeQANotifier struct {
	mu    sync.Mutex
	calls []qaNotify
}
type qaNotify struct{ UserID, Kind, Title, Body, ResourceType, ResourceID string }

func (f *fakeQANotifier) NotifyUser(_ context.Context, userID, kind, title, body, resourceType, resourceID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, qaNotify{userID, kind, title, body, resourceType, resourceID})
	return nil
}

func newSvc(ds *fakeDSReader, notifier *fakeQANotifier) *Service {
	return NewService(&fakeQARepo{}, ds, notifier)
}

func TestAskQuestion_RejectsEmptyBody(t *testing.T) {
	svc := newSvc(&fakeDSReader{}, nil)
	_, err := svc.AskQuestion(context.Background(), "u1", "ds1", "")
	if err != ErrEmptyBody {
		t.Fatalf("want ErrEmptyBody, got %v", err)
	}
}

func TestAskQuestion_RejectsBodyOver2000(t *testing.T) {
	svc := newSvc(&fakeDSReader{}, nil)
	body := strings.Repeat("x", 2001)
	_, err := svc.AskQuestion(context.Background(), "u1", "ds1", body)
	if err != ErrBodyTooLong {
		t.Fatalf("want ErrBodyTooLong, got %v", err)
	}
}

func TestAskQuestion_RejectsDraftDataset(t *testing.T) {
	ds := &fakeDSReader{
		sellers: map[string]string{"ds-draft": "seller"},
		status:  map[string]string{"ds-draft": "draft"},
	}
	svc := newSvc(ds, nil)
	_, err := svc.AskQuestion(context.Background(), "buyer", "ds-draft", "valid body")
	if err != ErrQuestionNotFound {
		t.Fatalf("want ErrQuestionNotFound for draft dataset, got %v", err)
	}
}

func TestAskQuestion_NotifiesSeller(t *testing.T) {
	ds := &fakeDSReader{
		sellers: map[string]string{"ds-pub": "seller"},
		status:  map[string]string{"ds-pub": "published"},
	}
	notifier := &fakeQANotifier{}
	svc := newSvc(ds, notifier)
	_, err := svc.AskQuestion(context.Background(), "buyer", "ds-pub", "good?")
	if err != nil {
		t.Fatal(err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(notifier.calls))
	}
	c := notifier.calls[0]
	if c.Kind != "question_asked" {
		t.Errorf("kind = %q, want question_asked", c.Kind)
	}
	if c.UserID != "seller" {
		t.Errorf("userID = %q, want seller", c.UserID)
	}
}

func TestAskQuestion_DoesNotNotifySelfWhenAskerIsSeller(t *testing.T) {
	ds := &fakeDSReader{
		sellers: map[string]string{"ds-self": "seller"},
		status:  map[string]string{"ds-self": "published"},
	}
	notifier := &fakeQANotifier{}
	svc := newSvc(ds, notifier)
	_, err := svc.AskQuestion(context.Background(), "seller", "ds-self", "self question")
	if err != nil {
		t.Fatal(err)
	}
	if len(notifier.calls) != 0 {
		t.Fatalf("seller asking about own dataset must not self-notify, got %d calls", len(notifier.calls))
	}
}

func TestAnswerQuestion_RejectsNonSellerAnswerer(t *testing.T) {
	repo := &fakeQARepo{}
	q, _ := repo.CreateQuestion(context.Background(), Question{DatasetID: "ds1", AskerID: "buyer", Body: "ask"})
	repo.questions[q.ID] = q

	ds := &fakeDSReader{
		sellers: map[string]string{"ds1": "seller"},
		status:  map[string]string{"ds1": "published"},
	}
	svc := NewService(repo, ds, nil)

	_, err := svc.AnswerQuestion(context.Background(), "intruder", q.ID, "bad answer")
	if err != ErrNotSeller {
		t.Fatalf("want ErrNotSeller (IDOR guard), got %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
