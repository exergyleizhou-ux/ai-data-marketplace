package storage

import (
	"context"
	"io"
)

// OSS is the production object-storage driver for Aliyun OSS / Tencent COS.
//
// TODO(Spike-1): implement with cloud credentials. The production design is
// browser-DIRECT multipart upload via presigned part URLs (docs §6.2) so large
// files never transit the app server — InitMultipart should return presigned
// PUT URLs, and downloads should hand out short-lived presigned GET URLs rather
// than streaming through Open. Until then this stub returns ErrNotImplemented
// and the app uses the Local driver (STORAGE_DRIVER=local).
type OSS struct {
	Bucket   string
	Endpoint string
}

func (o *OSS) InitMultipart(context.Context, string) (string, error) { return "", ErrNotImplemented }
func (o *OSS) PutPart(context.Context, string, int, io.Reader) (int64, error) {
	return 0, ErrNotImplemented
}
func (o *OSS) CompleteMultipart(context.Context, string) (Object, error) {
	return Object{}, ErrNotImplemented
}
func (o *OSS) Abort(context.Context, string) error                  { return ErrNotImplemented }
func (o *OSS) Stat(context.Context, string) (UploadStat, error)     { return UploadStat{}, ErrNotImplemented }
func (o *OSS) Open(context.Context, string) (io.ReadCloser, int64, error) {
	return nil, 0, ErrNotImplemented
}
