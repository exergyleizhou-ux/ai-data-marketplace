package dataset

import (
	"testing"
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
