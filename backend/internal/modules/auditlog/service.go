package auditlog

import "context"

// Service provides read-only access to the audit log for ops.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// List returns audit log entries matching the filter.
func (s *Service) List(ctx context.Context, f ListFilter) ([]LogEntry, error) {
	return s.repo.List(ctx, f)
}
