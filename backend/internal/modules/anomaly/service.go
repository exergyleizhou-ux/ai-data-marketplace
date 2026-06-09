package anomaly

import (
	"context"
	"log/slog"
	"time"
)

type Service struct {
	repo    Repository
	rules   []Rule
	db      DBQuerier
	alerter Alerter
}

func NewService(repo Repository, db DBQuerier, alerter Alerter) *Service {
	if alerter == nil {
		alerter = NopAlerter{}
	}
	return &Service{
		repo:    repo,
		db:      db,
		alerter: alerter,
		rules: []Rule{
			&RepeatedFailureRule{},
			&BulkModificationRule{},
			&HighRiskActionRule{},
		},
	}
}

// ScanOnce runs all detection rules and alerts on newly created anomalies.
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
			isNew, err := s.repo.UpsertReturningIsNew(ctx, a)
			if err != nil {
				slog.Warn("anomaly upsert failed", "kind", a.Kind, "err", err)
				continue
			}
			if isNew {
				alertNew(s.alerter, ctx, a)
			}
			total++
		}
	}
	return total, nil
}

// StartScanner starts a background goroutine that scans every 5 minutes.
func (s *Service) StartScanner(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
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
