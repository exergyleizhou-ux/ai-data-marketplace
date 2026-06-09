package anomaly

import (
	"context"
	"log/slog"
	"time"
)

type Service struct {
	repo  Repository
	rules []Rule
	db    DBQuerier
}

func NewService(repo Repository, db DBQuerier) *Service {
	return &Service{
		repo: repo,
		db:   db,
		rules: []Rule{
			&RepeatedFailureRule{},
			&BulkModificationRule{},
			&HighRiskActionRule{},
		},
	}
}

// ScanOnce 跑一轮扫描,返回 upsert 数。便于测试。
func (s *Service) ScanOnce(ctx context.Context) (int, error) {
	since := time.Now().Add(-1 * time.Hour)
	total := 0
	for _, rule := range s.rules {
		anomalies, err := rule.Detect(ctx, s.db, since)
		if err != nil {
			slog.Warn("anomaly rule failed", "kind", rule.Kind(), "err", err)
			continue
		}
		for _, a := range anomalies {
			if err := s.repo.Upsert(ctx, a); err != nil {
				slog.Warn("anomaly upsert failed", "kind", a.Kind, "err", err)
			}
			total++
		}
	}
	return total, nil
}

// StartScanner 后台 5 分钟一轮。
func (s *Service) StartScanner(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		// Run once immediately so ops see results on startup.
		_, _ = s.ScanOnce(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = s.ScanOnce(ctx)
			}
		}
	}()
}

func (s *Service) List(ctx context.Context, status string, limit, offset int) ([]Anomaly, error) {
	return s.repo.List(ctx, status, limit, offset)
}

func (s *Service) Acknowledge(ctx context.Context, id, opsID, note string) (Anomaly, error) {
	return s.repo.SetStatus(ctx, id, "acknowledged", opsID, note)
}

func (s *Service) Resolve(ctx context.Context, id, opsID, note string) (Anomaly, error) {
	return s.repo.SetStatus(ctx, id, "resolved", opsID, note)
}
