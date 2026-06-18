package storage

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3 is an S3-compatible object-storage driver (minio-go). One driver serves
// MinIO (free, self-hosted), AWS S3, Aliyun OSS, and Tencent COS — all speak
// the S3 API — so no proprietary SDK or paid account is needed to develop.
//
// Multipart parts are proxied through the backend here (parity with Local); the
// production-grade path is browser-direct presigned PUT and presigned GET for
// download (PresignGet below), so large objects bypass the app server.
type S3 struct {
	core   *minio.Core
	bucket string
}

// S3Config configures the driver.
type S3Config struct {
	Endpoint  string // host:port, no scheme (e.g. localhost:9000, oss-cn-hangzhou.aliyuncs.com)
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Region    string
}

// NewS3 connects, ensures the bucket exists, and returns the driver.
func NewS3(ctx context.Context, cfg S3Config) (*S3, error) {
	core, err := minio.NewCore(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("s3 connect: %w", err)
	}
	s := &S3{core: core, bucket: cfg.Bucket}

	exists, err := core.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("s3 bucket check: %w", err)
	}
	if !exists {
		if err := core.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return nil, fmt.Errorf("s3 make bucket: %w", err)
		}
	}
	return s, nil
}

// uploadID encodes the object key alongside the S3 upload id so PutPart/
// Complete/Stat can recover the key (the Storage interface only passes uploadID).
func encodeUploadID(objectKey, s3ID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(objectKey)) + "." + s3ID
}

func decodeUploadID(id string) (objectKey, s3ID string, err error) {
	i := strings.IndexByte(id, '.')
	if i < 0 {
		return "", "", ErrUploadNotFound
	}
	keyBytes, err := base64.RawURLEncoding.DecodeString(id[:i])
	if err != nil {
		return "", "", ErrUploadNotFound
	}
	return string(keyBytes), id[i+1:], nil
}

func (s *S3) InitMultipart(ctx context.Context, objectKey string) (string, error) {
	s3ID, err := s.core.NewMultipartUpload(ctx, s.bucket, objectKey, minio.PutObjectOptions{})
	if err != nil {
		return "", fmt.Errorf("s3 init multipart: %w", err)
	}
	return encodeUploadID(objectKey, s3ID), nil
}

func (s *S3) PutPart(ctx context.Context, uploadID string, partNumber int, r io.Reader) (int64, error) {
	objectKey, s3ID, err := decodeUploadID(uploadID)
	if err != nil {
		return 0, err
	}
	// S3 requires a known Content-Length per part; buffer the (bounded) part.
	buf, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	part, err := s.core.PutObjectPart(ctx, s.bucket, objectKey, s3ID, partNumber,
		strings.NewReader(string(buf)), int64(len(buf)), minio.PutObjectPartOptions{})
	if err != nil {
		return 0, fmt.Errorf("s3 put part: %w", err)
	}
	return part.Size, nil
}

func (s *S3) CompleteMultipart(ctx context.Context, uploadID string) (Object, error) {
	objectKey, s3ID, err := decodeUploadID(uploadID)
	if err != nil {
		return Object{}, err
	}
	parts, err := s.listParts(ctx, objectKey, s3ID)
	if err != nil {
		return Object{}, err
	}
	complete := make([]minio.CompletePart, 0, len(parts))
	for _, p := range parts {
		complete = append(complete, minio.CompletePart{PartNumber: p.PartNumber, ETag: p.ETag})
	}
	if _, err := s.core.CompleteMultipartUpload(ctx, s.bucket, objectKey, s3ID, complete, minio.PutObjectOptions{}); err != nil {
		return Object{}, fmt.Errorf("s3 complete: %w", err)
	}
	// S3's multipart ETag is not a content hash; compute the real SHA-256 + size
	// by reading the assembled object once (parity with the Local driver).
	sum, size, err := s.hashObject(ctx, objectKey)
	if err != nil {
		return Object{}, err
	}
	return Object{Key: objectKey, Size: size, SHA256: sum}, nil
}

func (s *S3) Abort(ctx context.Context, uploadID string) error {
	objectKey, s3ID, err := decodeUploadID(uploadID)
	if err != nil {
		return err
	}
	return s.core.AbortMultipartUpload(ctx, s.bucket, objectKey, s3ID)
}

func (s *S3) Stat(ctx context.Context, uploadID string) (UploadStat, error) {
	objectKey, s3ID, err := decodeUploadID(uploadID)
	if err != nil {
		return UploadStat{}, err
	}
	parts, err := s.listParts(ctx, objectKey, s3ID)
	if err != nil {
		return UploadStat{}, err
	}
	var bytes int64
	for _, p := range parts {
		bytes += p.Size
	}
	return UploadStat{UploadID: uploadID, ObjectKey: objectKey, Parts: len(parts), Bytes: bytes}, nil
}

func (s *S3) Open(ctx context.Context, objectKey string) (io.ReadCloser, int64, error) {
	obj, err := s.core.Client.GetObject(ctx, s.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("s3 get: %w", err)
	}
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, 0, ErrUploadNotFound
	}
	return obj, info.Size, nil
}

// Delete removes a completed object (idempotent: removing a missing object is a
// no-op in S3). Used for right-to-erasure purges.
func (s *S3) Delete(ctx context.Context, objectKey string) error {
	if err := s.core.Client.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("s3 delete: %w", err)
	}
	return nil
}

// PresignGet returns a short-lived direct-download URL (production best
// practice: bytes don't transit the app server). Satisfies storage.PresignedGetter.
func (s *S3) PresignGet(ctx context.Context, objectKey string, ttl time.Duration) (string, error) {
	u, err := s.core.Client.PresignedGetObject(ctx, s.bucket, objectKey, ttl, nil)
	if err != nil {
		return "", fmt.Errorf("s3 presign get: %w", err)
	}
	return u.String(), nil
}

func (s *S3) listParts(ctx context.Context, objectKey, s3ID string) ([]minio.ObjectPart, error) {
	res, err := s.core.ListObjectParts(ctx, s.bucket, objectKey, s3ID, 0, 10000)
	if err != nil {
		return nil, fmt.Errorf("s3 list parts: %w", err)
	}
	return res.ObjectParts, nil
}

func (s *S3) hashObject(ctx context.Context, objectKey string) (string, int64, error) {
	rc, _, err := s.Open(ctx, objectKey)
	if err != nil {
		return "", 0, err
	}
	defer rc.Close()
	h := sha256.New()
	n, err := io.Copy(h, rc)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
