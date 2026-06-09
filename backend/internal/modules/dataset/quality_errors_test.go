package dataset

import (
	"testing"
	"time"
)

func TestClassifyQualityError_ObjectNotFound_Permanent(t *testing.T) {
	if classifyQualityError(ErrObjectNotFound) != QualityErrPermanent {
		t.Fatal("ErrObjectNotFound must be Permanent")
	}
}

func TestClassifyQualityError_InvalidContent_Permanent(t *testing.T) {
	if classifyQualityError(ErrInvalidContent) != QualityErrPermanent {
		t.Fatal("ErrInvalidContent must be Permanent")
	}
}

func TestClassifyQualityError_GenericError_Transient(t *testing.T) {
	if classifyQualityError(ErrNotFound) != QualityErrTransient {
		t.Fatal("generic error must be Transient")
	}
}

func TestClassifyQualityError_Nil_Transient(t *testing.T) {
	if classifyQualityError(nil) != QualityErrTransient {
		t.Fatal("nil error must be Transient")
	}
}

func TestComputeRetryBackoff_30_60_120(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{0, 30 * time.Second},
		{1, 60 * time.Second},
		{2, 120 * time.Second},
		{5, 120 * time.Second}, // 上限封顶
	}
	for _, c := range cases {
		if got := computeRetryBackoff(c.attempts); got != c.want {
			t.Errorf("attempts=%d backoff=%v want %v", c.attempts, got, c.want)
		}
	}
}
