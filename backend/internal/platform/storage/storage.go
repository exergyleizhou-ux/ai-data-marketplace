// Package storage abstracts object storage behind a multipart-upload + read
// interface. Two drivers:
//
//   - Local (dev/test): proxies parts through the backend to the local FS.
//   - OSS/COS (prod): the production path is browser-DIRECT multipart upload
//     via presigned part URLs (docs §6.2) so big files bypass the app server.
//     That driver is a stub here — fill it in with cloud credentials (Spike-1).
//
// Modules depend only on this interface, so swapping drivers needs no logic
// changes.
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// Object identifies stored bytes plus integrity metadata.
type Object struct {
	Key    string
	Size   int64
	SHA256 string
}

// UploadStat reports progress of an in-flight multipart upload.
type UploadStat struct {
	UploadID  string
	ObjectKey string
	Parts     int
	Bytes     int64
}

// Storage is a minimal multipart object store.
type Storage interface {
	// InitMultipart starts an upload targeting objectKey and returns an upload id.
	InitMultipart(ctx context.Context, objectKey string) (uploadID string, err error)
	// PutPart stores one part (1-based partNumber); returns bytes written.
	PutPart(ctx context.Context, uploadID string, partNumber int, r io.Reader) (int64, error)
	// CompleteMultipart assembles parts in order into the target object and
	// returns its size and SHA-256.
	CompleteMultipart(ctx context.Context, uploadID string) (Object, error)
	// Abort discards an in-flight upload.
	Abort(ctx context.Context, uploadID string) error
	// Stat reports progress of an in-flight upload.
	Stat(ctx context.Context, uploadID string) (UploadStat, error)
	// Open returns a reader for a completed object.
	Open(ctx context.Context, objectKey string) (io.ReadCloser, int64, error)
}

// PresignedGetter is an OPTIONAL capability: drivers that can hand out a
// short-lived direct-download URL implement it, so the delivery module can
// redirect buyers to object storage instead of streaming bytes through the app
// server (the production best practice). Drivers without it (Local) are streamed.
type PresignedGetter interface {
	PresignGet(ctx context.Context, objectKey string, ttl time.Duration) (string, error)
}

// ErrNotImplemented is returned by driver stubs awaiting real integration.
var ErrNotImplemented = errors.New("storage driver not implemented")

// ErrUploadNotFound is returned for an unknown/expired upload id.
var ErrUploadNotFound = errors.New("upload not found")
