package compliance

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/storage"
)

// Source provides read access to user data across modules.
type Source interface {
	UserRow(ctx context.Context, userID string) (map[string]any, error)
	Orders(ctx context.Context, userID string) ([]map[string]any, error)
	Datasets(ctx context.Context, userID string) ([]map[string]any, error)
	Notifications(ctx context.Context, userID string) ([]map[string]any, error)
	Watches(ctx context.Context, userID string) ([]map[string]any, error)
	Questions(ctx context.Context, userID string) ([]map[string]any, error)
	Answers(ctx context.Context, userID string) ([]map[string]any, error)
	Reviews(ctx context.Context, userID string) ([]map[string]any, error)
	Withdrawals(ctx context.Context, userID string) ([]map[string]any, error)
	ComputeJobs(ctx context.Context, userID string) ([]map[string]any, error)
}

type ExportService struct {
	repo     ExportRepository
	source   Source
	notifier Notifier
	store    storage.Storage
	cache    map[string][]byte // objectKey → zip bytes
	mu       sync.RWMutex
}

func NewExportService(repo ExportRepository, source Source, notifier Notifier, store storage.Storage) *ExportService {
	return &ExportService{repo: repo, source: source, notifier: notifier, store: store, cache: map[string][]byte{}}
}

func (s *ExportService) RequestExport(ctx context.Context, userID string) (ExportJob, error) {
	j, err := s.repo.FindRecentByUser(ctx, userID)
	if err == nil && j.Status != ExportExpired && j.Status != ExportFailed {
		return j, ErrExportInProgress
	}
	j, err = s.repo.Create(ctx, userID)
	if err != nil {
		return ExportJob{}, err
	}
	go s.generateJob(context.Background(), j.ID, userID)
	return j, nil
}

func (s *ExportService) GetExportStatus(ctx context.Context, userID string) (ExportJob, error) {
	return s.repo.FindRecentByUser(ctx, userID)
}

// PurgeUser deletes all of a user's data-export jobs, the backing object-store
// zips (each a full PII snapshot), and the in-memory cache entries. Called from
// the account-deletion flow so erasure does not leave a PII archive behind.
func (s *ExportService) PurgeUser(ctx context.Context, userID string) error {
	keys, err := s.repo.PurgeByUser(ctx, userID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	for _, k := range keys {
		delete(s.cache, k)
	}
	s.mu.Unlock()
	if s.store != nil {
		for _, k := range keys {
			if derr := s.store.Delete(ctx, k); derr != nil {
				return fmt.Errorf("delete export object %s: %w", k, derr)
			}
		}
	}
	return nil
}

// OpenExport returns a reader for the stored export zip, or io.EOF if not found.
// It reads the in-memory cache first (no-store fallback) and otherwise the object
// store — so exports survive a process restart and are not pinned in memory.
func (s *ExportService) OpenExport(ctx context.Context, key string) (io.ReadCloser, error) {
	s.mu.RLock()
	data, ok := s.cache[key]
	s.mu.RUnlock()
	if ok {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	if s.store != nil {
		if rc, _, err := s.store.Open(ctx, key); err == nil {
			return rc, nil
		}
	}
	return nil, io.EOF
}

func (s *ExportService) generateJob(ctx context.Context, jobID, userID string) {
	_ = s.repo.SetGenerating(ctx, jobID)

	snap, err := s.collect(ctx, userID)
	if err != nil {
		_ = s.repo.SetFailed(ctx, jobID, err.Error())
		return
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for filename, data := range snap.asFiles() {
		w, err := zw.Create(filename)
		if err != nil {
			_ = s.repo.SetFailed(ctx, jobID, "zip create: "+err.Error())
			zw.Close()
			return
		}
		if _, err := w.Write(data); err != nil {
			_ = s.repo.SetFailed(ctx, jobID, "zip write: "+err.Error())
			zw.Close()
			return
		}
	}
	if err := zw.Close(); err != nil {
		_ = s.repo.SetFailed(ctx, jobID, "zip close: "+err.Error())
		return
	}

	objectKey := fmt.Sprintf("exports/%s/%s.zip", userID, jobID)
	zipBytes := make([]byte, buf.Len())
	copy(zipBytes, buf.Bytes())

	// Persist to object storage (production path). OpenExport serves from here, so
	// we do NOT also retain the zip in the in-memory cache when a store is
	// configured — otherwise the process accumulates every export ever generated
	// (unbounded memory). The in-memory cache is only the no-store fallback
	// (tests / single-process dev without object storage).
	stored := false
	if s.store != nil {
		if uploadID, err := s.store.InitMultipart(ctx, objectKey); err == nil {
			if _, perr := s.store.PutPart(ctx, uploadID, 1, bytes.NewReader(zipBytes)); perr == nil {
				if _, cerr := s.store.CompleteMultipart(ctx, uploadID); cerr == nil {
					stored = true
				}
			}
		}
	}
	if !stored {
		s.mu.Lock()
		s.cache[objectKey] = zipBytes
		s.mu.Unlock()
	}

	if err := s.repo.SetReady(ctx, jobID, objectKey, int64(buf.Len()), time.Now().Add(24*time.Hour)); err != nil {
		slog.Warn("export ready failed", "jobID", jobID, "err", err)
		return
	}

	if s.notifier != nil {
		_ = s.notifier.NotifyUser(ctx, userID, "data_export_ready",
			"数据导出已就绪", "您的数据导出已生成，请在账户页下载。",
			"export", jobID)
	}
}

type dataSnapshot struct {
	User          map[string]any   `json:"user"`
	Orders        []map[string]any `json:"orders"`
	Datasets      []map[string]any `json:"datasets"`
	Notifications []map[string]any `json:"notifications"`
	Watches       []map[string]any `json:"watches"`
	Questions     []map[string]any `json:"questions"`
	Answers       []map[string]any `json:"answers"`
	Reviews       []map[string]any `json:"reviews"`
	Withdrawals   []map[string]any `json:"withdrawals"`
	ComputeJobs   []map[string]any `json:"compute_jobs"`
	ExportedAt    string           `json:"exported_at"`
}

func (s *ExportService) collect(ctx context.Context, userID string) (*dataSnapshot, error) {
	snap := &dataSnapshot{ExportedAt: time.Now().UTC().Format(time.RFC3339)}
	var err error
	snap.User, err = s.source.UserRow(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user: %w", err)
	}
	snap.Orders, _ = s.source.Orders(ctx, userID)
	snap.Datasets, _ = s.source.Datasets(ctx, userID)
	snap.Notifications, _ = s.source.Notifications(ctx, userID)
	snap.Watches, _ = s.source.Watches(ctx, userID)
	snap.Questions, _ = s.source.Questions(ctx, userID)
	snap.Answers, _ = s.source.Answers(ctx, userID)
	snap.Reviews, _ = s.source.Reviews(ctx, userID)
	snap.Withdrawals, _ = s.source.Withdrawals(ctx, userID)
	snap.ComputeJobs, _ = s.source.ComputeJobs(ctx, userID)
	return snap, nil
}

func snapOrEmpty(arr []map[string]any) []map[string]any {
	if arr == nil {
		return []map[string]any{}
	}
	return arr
}

func (s *dataSnapshot) asFiles() map[string][]byte {
	out := map[string][]byte{}
	out["user.json"], _ = json.MarshalIndent(s.User, "", "  ")
	out["orders.json"], _ = json.MarshalIndent(snapOrEmpty(s.Orders), "", "  ")
	out["datasets.json"], _ = json.MarshalIndent(snapOrEmpty(s.Datasets), "", "  ")
	out["notifications.json"], _ = json.MarshalIndent(snapOrEmpty(s.Notifications), "", "  ")
	out["watches.json"], _ = json.MarshalIndent(snapOrEmpty(s.Watches), "", "  ")
	out["questions.json"], _ = json.MarshalIndent(snapOrEmpty(s.Questions), "", "  ")
	out["answers.json"], _ = json.MarshalIndent(snapOrEmpty(s.Answers), "", "  ")
	out["reviews.json"], _ = json.MarshalIndent(snapOrEmpty(s.Reviews), "", "  ")
	out["withdrawals.json"], _ = json.MarshalIndent(snapOrEmpty(s.Withdrawals), "", "  ")
	out["compute_jobs.json"], _ = json.MarshalIndent(snapOrEmpty(s.ComputeJobs), "", "  ")
	out["exported_at.txt"] = []byte(s.ExportedAt)
	return out
}
