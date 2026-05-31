package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
)

func TestLocalMultipartRoundTrip(t *testing.T) {
	ctx := context.Background()
	l, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("new local: %v", err)
	}

	key := "datasets/abc/data.txt"
	uploadID, err := l.InitMultipart(ctx, key)
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	p1, p2 := "第一部分内容\n", "second part contents\n"
	if _, err := l.PutPart(ctx, uploadID, 1, strings.NewReader(p1)); err != nil {
		t.Fatalf("part 1: %v", err)
	}
	if _, err := l.PutPart(ctx, uploadID, 2, strings.NewReader(p2)); err != nil {
		t.Fatalf("part 2: %v", err)
	}

	st, err := l.Stat(ctx, uploadID)
	if err != nil || st.Parts != 2 {
		t.Fatalf("stat parts = %d (err %v), want 2", st.Parts, err)
	}

	obj, err := l.CompleteMultipart(ctx, uploadID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	want := p1 + p2
	sum := sha256.Sum256([]byte(want))
	if obj.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("sha256 mismatch")
	}
	if obj.Size != int64(len(want)) {
		t.Errorf("size = %d, want %d", obj.Size, len(want))
	}

	// Parts are assembled in order.
	rc, size, err := l.Open(ctx, key)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != want {
		t.Errorf("content = %q, want %q", got, want)
	}
	if size != int64(len(want)) {
		t.Errorf("open size = %d, want %d", size, len(want))
	}

	// After completion the upload id is gone.
	if _, err := l.Stat(ctx, uploadID); err != ErrUploadNotFound {
		t.Errorf("stat after complete = %v, want ErrUploadNotFound", err)
	}
}

func TestLocalRejectsTraversal(t *testing.T) {
	l, _ := NewLocal(t.TempDir())
	if _, err := l.InitMultipart(context.Background(), "../../etc/passwd"); err == nil {
		t.Fatal("expected path-traversal key to be rejected")
	}
}
