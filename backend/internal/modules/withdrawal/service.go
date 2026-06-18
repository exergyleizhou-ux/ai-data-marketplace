package withdrawal

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
)

type EarningsReader interface {
	SettledCentsOf(ctx context.Context, sellerID string) (int64, error)
}

type Notifier interface {
	NotifyUser(ctx context.Context, userID, kind, title, body, resourceType, resourceID string) error
}

type Service struct {
	repo     Repository
	earnings EarningsReader
	notifier Notifier
	audit    audit.Recorder
}

func NewService(repo Repository, earnings EarningsReader, notifier Notifier) *Service {
	return &Service{repo: repo, earnings: earnings, notifier: notifier, audit: audit.Noop{}}
}

// SetAudit wires the audit-log recorder for ops decisions (approve/reject/
// complete). Without it the service defaults to a no-op recorder.
func (s *Service) SetAudit(rec audit.Recorder) {
	if rec != nil {
		s.audit = rec
	}
}

// auditDecision records an ops withdrawal decision in audit_logs. These are
// high-risk financial admin actions (withdrawal.reject is watched by the
// anomaly HighRiskActionRule); approve/complete round out the trail.
func (s *Service) auditDecision(ctx context.Context, action, opsID, reqID string, detail any) {
	s.audit.Record(ctx, audit.Entry{
		ActorID: opsID, ActorRole: "ops", Action: action,
		ResourceType: "withdrawal", ResourceID: reqID, Detail: detail,
	})
}

func (s *Service) Request(ctx context.Context, sellerID string, amountCents int64, channel, accountLabel string) (Request, error) {
	if amountCents <= 0 || amountCents > 100_000_000 {
		return Request{}, ErrAmountInvalid
	}
	if channel != "wechat" && channel != "alipay" && channel != "bank" {
		return Request{}, ErrChannelInvalid
	}
	accountLabel = strings.TrimSpace(accountLabel)
	if accountLabel == "" || len(accountLabel) > 200 {
		return Request{}, ErrAmountInvalid
	}
	settled, err := s.earnings.SettledCentsOf(ctx, sellerID)
	if err != nil {
		return Request{}, err
	}
	// Atomic, balance-correct insert: the repo locks the seller, subtracts ALL
	// non-rejected withdrawals (including completed payouts) from settled, and
	// inserts only if the amount fits — closing both the completed-payouts-free-
	// the-balance drain and the concurrent-request TOCTOU.
	return s.repo.CreateWithinBudget(ctx, Request{
		SellerID: sellerID, AmountCents: amountCents, Channel: channel,
		AccountLabel: accountLabel, Status: StatusPending,
	}, settled)
}

func (s *Service) Approve(ctx context.Context, opsID, id, note string) (Request, error) {
	r, err := s.repo.Transition(ctx, id, StatusPending, StatusApproved, opsID, note)
	if err != nil {
		return Request{}, err
	}
	s.auditDecision(ctx, "withdrawal.approve", opsID, r.ID, map[string]any{"note": note})
	s.notify(ctx, r, "withdrawal_approved", "提现申请已批准", "您的提现申请已批准,等待打款。")
	return r, nil
}

func (s *Service) Reject(ctx context.Context, opsID, id, reason string) (Request, error) {
	if strings.TrimSpace(reason) == "" {
		return Request{}, ErrReasonRequired
	}
	r, err := s.repo.Transition(ctx, id, StatusPending, StatusRejected, opsID, reason)
	if err != nil {
		return Request{}, err
	}
	s.auditDecision(ctx, "withdrawal.reject", opsID, r.ID, map[string]any{"reason": reason})
	s.notify(ctx, r, "withdrawal_rejected", "提现申请被拒", reason)
	return r, nil
}

func (s *Service) Complete(ctx context.Context, opsID, id, note string) (Request, error) {
	r, err := s.repo.Transition(ctx, id, StatusApproved, StatusCompleted, opsID, note)
	if err != nil {
		return Request{}, err
	}
	s.auditDecision(ctx, "withdrawal.complete", opsID, r.ID, map[string]any{"note": note})
	s.notify(ctx, r, "withdrawal_completed", "提现已到账",
		fmt.Sprintf("提现 ¥%.2f 已到账。", float64(r.AmountCents)/100.0))
	return r, nil
}

func (s *Service) notify(ctx context.Context, r Request, kind, title, body string) {
	if s.notifier != nil {
		if err := s.notifier.NotifyUser(ctx, r.SellerID, kind, title, body, "withdrawal", r.ID); err != nil {
			slog.Warn("withdrawal notify failed", "kind", kind, "seller", r.SellerID, "err", err)
		}
	}
}

func (s *Service) ListMy(ctx context.Context, sellerID string, limit, offset int) ([]Request, error) {
	return s.repo.ListBySeller(ctx, sellerID, limit, offset)
}

func (s *Service) AdminList(ctx context.Context, status string, limit, offset int) ([]Request, error) {
	return s.repo.AdminList(ctx, status, limit, offset)
}
