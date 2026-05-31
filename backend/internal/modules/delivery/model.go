package delivery

import (
	"errors"
	"time"
)

// Grant is a buyer's right to download a purchased dataset, via a one-time,
// short-lived token. We do NOT promise anti-piracy (plain text/code/CSV can be
// copied once downloaded, docs §2.3) — only traceability + accountability.
type Grant struct {
	ID            string
	OrderID       string
	ExpiresAt     time.Time
	MaxDownloads  int
	DownloadCount int
	Fingerprint   string
}

const (
	tokenTTL     = 15 * time.Minute // short-lived presigned-style link
	maxDownloads = 3                // a few retries; not unlimited
)

var (
	ErrForbidden       = errors.New("not the buyer of this order")
	ErrNotPaid         = errors.New("order is not paid; nothing to deliver")
	ErrLicenseRequired = errors.New("must accept the data license agreement")
	ErrTokenInvalid    = errors.New("download link is invalid, expired, or used up")
)
