package dataset

import "errors"

// QualityErrKind classifies a quality-processing failure as transient
// (retriable) or permanent (fatal — bounce back to draft).
type QualityErrKind int

const (
	// QualityErrTransient means the failure is likely temporary:
	// network blip, sidecar 5xx, process crash, DB hiccup.
	QualityErrTransient QualityErrKind = iota
	// QualityErrPermanent means the failure is unrecoverable:
	// corrupt file, missing object, schema mismatch.
	QualityErrPermanent
)

// Permanent-error sentinels that processQuality may return.
var (
	ErrObjectNotFound = errors.New("quality: source object not found")
	ErrInvalidContent = errors.New("quality: content is invalid (cannot decode)")
)

// classifyQualityError returns Transient unless the error chain contains a
// known permanent sentinel. Conservative default = Transient (retry beats
// wrongly bouncing a good upload).
func classifyQualityError(err error) QualityErrKind {
	if err == nil {
		return QualityErrTransient
	}
	if errors.Is(err, ErrObjectNotFound) || errors.Is(err, ErrInvalidContent) {
		return QualityErrPermanent
	}
	return QualityErrTransient
}
