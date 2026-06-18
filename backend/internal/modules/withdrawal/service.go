package withdrawal

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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
}

func NewService(repo Repository, earnings EarningsReader, notifier Notifier) *Service {
	return &Service{repo: repo, earnings: earnings, notifier: notifier}
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
	s.notify(ctx, r, "withdrawal_rejected", "提现申请被拒", reason)
	return r, nil
}

func (s *Service) Complete(ctx context.Context, opsID, id, note string) (Request, error) {
	r, err := s.repo.Transition(ctx, id, StatusApproved, StatusCompleted, opsID, note)
	if err != nil {
		return Request{}, err
	}
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
