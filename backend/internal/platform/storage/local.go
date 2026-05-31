package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Local is a filesystem-backed Storage for development and tests. Parts live
// under baseDir/.uploads/<uploadID>/; completed objects under baseDir/objects/.
// Not for production — use the OSS/COS driver there.
type Local struct{ baseDir string }

// NewLocal creates the base directory and returns a Local driver.
func NewLocal(baseDir string) (*Local, error) {
	if err := os.MkdirAll(filepath.Join(baseDir, "objects"), 0o755); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, ".uploads"), 0o755); err != nil {
		return nil, fmt.Errorf("create uploads dir: %w", err)
	}
	return &Local{baseDir: baseDir}, nil
}

func (l *Local) uploadDir(id string) string { return filepath.Join(l.baseDir, ".uploads", id) }

// safeObjectPath rejects path traversal and maps an object key to a FS path.
func (l *Local) safeObjectPath(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("empty object key")
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == ".." {
			return "", fmt.Errorf("invalid object key %q", key)
		}
	}
	clean := filepath.Clean("/" + key) // force absolute, collapse, strip leading root
	return filepath.Join(l.baseDir, "objects", clean), nil
}

func (l *Local) InitMultipart(_ context.Context, objectKey string) (string, error) {
	if _, err := l.safeObjectPath(objectKey); err != nil {
		return "", err
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf)
	dir := l.uploadDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "meta"), []byte(objectKey), 0o644); err != nil {
		return "", err
	}
	return id, nil
}

func (l *Local) PutPart(_ context.Context, uploadID string, partNumber int, r io.Reader) (int64, error) {
	dir := l.uploadDir(uploadID)
	if _, err := os.Stat(dir); err != nil {
		return 0, ErrUploadNotFound
	}
	if partNumber < 1 {
		return 0, fmt.Errorf("part number must be >= 1")
	}
	f, err := os.Create(filepath.Join(dir, fmt.Sprintf("part-%06d", partNumber)))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n, err := io.Copy(f, r)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (l *Local) CompleteMultipart(_ context.Context, uploadID string) (Object, error) {
	dir := l.uploadDir(uploadID)
	keyBytes, err := os.ReadFile(filepath.Join(dir, "meta"))
	if errors.Is(err, os.ErrNotExist) {
		return Object{}, ErrUploadNotFound
	}
	if err != nil {
		return Object{}, err
	}
	objectKey := string(keyBytes)
	dst, err := l.safeObjectPath(objectKey)
	if err != nil {
		return Object{}, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return Object{}, err
	}

	parts, err := sortedParts(dir)
	if err != nil {
		return Object{}, err
	}
	out, err := os.Create(dst)
	if err != nil {
		return Object{}, err
	}
	defer out.Close()

	h := sha256.New()
	var size int64
	for _, p := range parts {
		in, err := os.Open(p)
		if err != nil {
			return Object{}, err
		}
		n, err := io.Copy(io.MultiWriter(out, h), in)
		in.Close()
		if err != nil {
			return Object{}, err
		}
		size += n
	}
	_ = os.RemoveAll(dir)
	return Object{Key: objectKey, Size: size, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

func (l *Local) Abort(_ context.Context, uploadID string) error {
	return os.RemoveAll(l.uploadDir(uploadID))
}

func (l *Local) Stat(_ context.Context, uploadID string) (UploadStat, error) {
	dir := l.uploadDir(uploadID)
	keyBytes, err := os.ReadFile(filepath.Join(dir, "meta"))
	if errors.Is(err, os.ErrNotExist) {
		return UploadStat{}, ErrUploadNotFound
	}
	if err != nil {
		return UploadStat{}, err
	}
	parts, err := sortedParts(dir)
	if err != nil {
		return UploadStat{}, err
	}
	var bytes int64
	for _, p := range parts {
		if fi, err := os.Stat(p); err == nil {
			bytes += fi.Size()
		}
	}
	return UploadStat{UploadID: uploadID, ObjectKey: string(keyBytes), Parts: len(parts), Bytes: bytes}, nil
}

func (l *Local) Open(_ context.Context, objectKey string) (io.ReadCloser, int64, error) {
	path, err := l.safeObjectPath(objectKey)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, fi.Size(), nil
}

func sortedParts(dir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "part-*"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches) // zero-padded names sort in part order
	return matches, nil
}
