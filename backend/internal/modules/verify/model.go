package verify

import (
	"context"
	"errors"
)

// CertInfo is a lightweight lookup row from the certificates table.
type CertInfo struct {
	CertID       string `json:"cert_id"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	CreatedAt    string `json:"created_at"`
}

// CertRegistrar persists a certificate idempotently for public lookup.
// The verify.Repository implements this.
type CertRegistrar interface {
	Register(ctx context.Context, certID, resourceType, resourceID string) error
}

var (
	ErrNotFound = errors.New("certificate not found")
)
