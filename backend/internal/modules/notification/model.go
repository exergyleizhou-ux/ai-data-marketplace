package notification

import (
	"context"
	"errors"
)

// Notification is a user-facing event record.
type Notification struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	Kind         string `json:"kind"`
	Title        string `json:"title"`
	Body         string `json:"body,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	IsRead       bool   `json:"is_read"`
	CreatedAt    string `json:"created_at,omitempty"`
}

// Notification kinds.
const (
	KindOrderPaid       = "order_paid"
	KindOrderSettled    = "order_settled"
	KindOrderDisputed   = "order_disputed"
	KindQualityDone     = "quality_done"
	KindComputeReleased = "compute_released"
)

var (
	ErrNotFound = errors.New("notification not found")
)

// Notifier is the cross-module interface for emitting notifications.
// Implemented by the notification module; injected into order/dataset/compute
// so they can emit events without importing notification internals.
type Notifier interface {
	NotifyUser(ctx context.Context, userID, kind, title, body, resourceType, resourceID string) error
}
