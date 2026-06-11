package moderation

import "errors"

var (
	ErrInvalidTarget     = errors.New("invalid target type")
	ErrEmptyReason       = errors.New("reason is required")
	ErrInvalidResolution = errors.New("invalid resolution")
	ErrReportNotFound    = errors.New("report not found")
)

func validTarget(t string) bool { return t == TargetQuestion || t == TargetReview }

func validResolution(r string) bool { return r == ResolutionHide || r == ResolutionDismiss }
