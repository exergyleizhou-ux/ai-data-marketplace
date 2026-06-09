package compliance

import (
	"context"
	"errors"
)

type ExportJob struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	Status      string `json:"status"`
	DownloadURL string `json:"download_url,omitempty"`
	ObjectBytes int64  `json:"object_bytes,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	Error       string `json:"error,omitempty"`
	RequestedAt string `json:"requested_at"`
	ReadyAt     string `json:"ready_at,omitempty"`
}

type DeletionRequest struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	Reason       string `json:"reason,omitempty"`
	Status       string `json:"status"`
	CoolingUntil string `json:"cooling_until"`
	OpsNote      string `json:"ops_note,omitempty"`
	RequestedAt  string `json:"requested_at"`
	ProcessedAt  string `json:"processed_at,omitempty"`
	ProcessedBy  string `json:"processed_by,omitempty"`
}

const (
	ExportPending    = "pending"
	ExportGenerating = "generating"
	ExportReady      = "ready"
	ExportFailed     = "failed"
	ExportExpired    = "expired"

	DeletionCooling   = "cooling"
	DeletionApproved  = "approved"
	DeletionRejected  = "rejected"
	DeletionCancelled = "cancelled"
	DeletionDeleted   = "deleted"
)

var (
	ErrExportInProgress      = errors.New("a data export is already in progress")
	ErrExportNotReady        = errors.New("export not ready or expired")
	ErrDeletionExists        = errors.New("an active deletion request already exists")
	ErrDeletionNotCancelable = errors.New("deletion request is not in cooling state")
	ErrCoolingNotElapsed     = errors.New("cooling period has not elapsed")
	ErrNotFound              = errors.New("not found")
	ErrBadTransition         = errors.New("illegal status transition")
)

// Notifier is the notification interface used by this module.
type Notifier interface {
	NotifyUser(ctx context.Context, userID, kind, title, body, resourceType, resourceID string) error
}
