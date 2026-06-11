// Package moderation handles user-submitted content reports against questions
// and reviews, and the ops actions that resolve them (hide the content or
// dismiss the report). Questions already carry status='hidden'; reviews gain a
// hidden flag. Reporting and resolving are cross-cutting, so this module owns
// the moderation concern and updates the target tables directly.
package moderation

// TargetType enumerates the content kinds that can be reported.
const (
	TargetQuestion = "question"
	TargetReview   = "review"
)

// Report statuses and resolutions.
const (
	StatusOpen     = "open"
	StatusResolved = "resolved"

	ResolutionHide    = "hide"
	ResolutionDismiss = "dismiss"
)

// Report is a single user-submitted content report.
type Report struct {
	ID         string `json:"id"`
	ReporterID string `json:"reporter_id"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Reason     string `json:"reason"`
	Status     string `json:"status"`
	Resolution string `json:"resolution,omitempty"`
	CreatedAt  string `json:"created_at"`
	ResolvedAt string `json:"resolved_at,omitempty"`
	ResolvedBy string `json:"resolved_by,omitempty"`
}
