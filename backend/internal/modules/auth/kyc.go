package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
)

// KYCVerifier abstracts the real-name verification backend (实名认证). MVP ships
// two: ManualVerifier (await ops review — the safe default) and
// AutoApproveVerifier (dev only). A real provider (实人认证 / 营业执照 OCR,
// Spike-5) implements this same interface.
type KYCVerifier interface {
	// Verify returns the post-submission status: kycPending to await human
	// review, or kycVerified/kycRejected if the backend decides synchronously.
	Verify(ctx context.Context, rec KYCRecord) (string, error)
}

// ManualVerifier leaves submissions pending for ops to review.
type ManualVerifier struct{}

func (ManualVerifier) Verify(context.Context, KYCRecord) (string, error) { return kycPending, nil }

// AutoApproveVerifier instantly approves — DEVELOPMENT ONLY.
type AutoApproveVerifier struct{}

func (AutoApproveVerifier) Verify(context.Context, KYCRecord) (string, error) {
	return kycVerified, nil
}

// SubmitKYC records a real-name submission and runs the configured verifier.
// The raw ID number is hashed and never persisted in the clear.
func (s *Service) SubmitKYC(ctx context.Context, userID, kycType, realName, companyName, idNo string, materialURLs []string) (KYCRecord, error) {
	switch kycType {
	case kycTypePersonal:
		if realName == "" || idNo == "" {
			return KYCRecord{}, fmt.Errorf("%w: personal kyc requires real_name and id_no", ErrValidation)
		}
	case kycTypeCompany:
		if companyName == "" {
			return KYCRecord{}, fmt.Errorf("%w: company kyc requires company_name", ErrValidation)
		}
	default:
		return KYCRecord{}, fmt.Errorf("%w: type must be personal or company", ErrValidation)
	}

	rec := KYCRecord{
		UserID:       userID,
		Type:         kycType,
		RealName:     realName,
		CompanyName:  companyName,
		MaterialURLs: materialURLs,
	}
	// Store the ID number two ways: a blind index (HMAC) for dedup/equality, and
	// reversible AES-GCM ciphertext for lawful retrieval. Raw value never stored.
	var idNoCiphertext []byte
	if idNo != "" {
		ct, err := encryptIDNo(s.piiSecret, idNo, userID)
		if err != nil {
			return KYCRecord{}, fmt.Errorf("encrypt id_no: %w", err)
		}
		idNoCiphertext = ct
	}
	saved, err := s.repo.SubmitKYC(ctx, rec, s.hashIDNo(idNo), idNoCiphertext)
	if err != nil {
		return KYCRecord{}, err
	}

	status, err := s.verifier.Verify(ctx, saved)
	if err != nil {
		return KYCRecord{}, fmt.Errorf("kyc verify: %w", err)
	}
	if status != kycPending {
		return s.repo.ReviewKYC(ctx, saved.ID, status, "") // system reviewer (NULL)
	}
	return saved, nil
}

// GetKYC returns the user's latest KYC submission.
func (s *Service) GetKYC(ctx context.Context, userID string) (KYCRecord, error) {
	return s.repo.GetLatestKYC(ctx, userID)
}

// ListPendingKYC returns submissions awaiting ops review (ops-gated at router).
func (s *Service) ListPendingKYC(ctx context.Context, limit, offset int) ([]KYCRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListPendingKYC(ctx, limit, offset)
}

// ReviewKYC is the ops action approving or rejecting a submission. The decision
// is recorded in audit_logs (kyc.approve / kyc.reject) — a high-risk identity
// action that the anomaly HighRiskActionRule watches and compliance requires.
func (s *Service) ReviewKYC(ctx context.Context, kycID string, approve bool, reviewerID string) (KYCRecord, error) {
	status := kycRejected
	action := "kyc.reject"
	if approve {
		status = kycVerified
		action = "kyc.approve"
	}
	rec, err := s.repo.ReviewKYC(ctx, kycID, status, reviewerID)
	if err != nil {
		return KYCRecord{}, err
	}
	s.audit.Record(ctx, audit.Entry{
		ActorID: reviewerID, ActorRole: "ops", Action: action,
		ResourceType: "kyc", ResourceID: kycID,
	})
	return rec, nil
}

// RevealIDNo decrypts a KYC submission's raw ID number for lawful retrieval.
// It is the ONLY path that returns plaintext. Authorization (ops/admin) is
// enforced at the router (RequireRole); the handler MUST record an audit entry.
// Returns ErrIDNoNotEncrypted when the record has no stored ciphertext.
func (s *Service) RevealIDNo(ctx context.Context, kycID string) (string, error) {
	blob, ownerID, err := s.repo.GetIDNoCiphertext(ctx, kycID)
	if err != nil {
		return "", err
	}
	return decryptIDNo(s.piiSecret, blob, ownerID)
}

// UpdateRole lets a user change their own role among buyer/seller/both
// (US-1.5). Privileged roles (ops/admin) can never be self-assigned.
func (s *Service) UpdateRole(ctx context.Context, userID, role string) (User, error) {
	switch role {
	case roleBuyer, roleSeller, roleBoth:
	default:
		return User{}, fmt.Errorf("%w: role must be buyer, seller or both", ErrValidation)
	}
	return s.repo.UpdateUserRole(ctx, userID, role)
}

// hashIDNo computes a keyed hash of the raw ID number so equality/dedup is
// possible without storing the plaintext. (Production should encrypt instead,
// to allow lawful retrieval — tracked for a later hardening pass.)
func (s *Service) hashIDNo(idNo string) string {
	if idNo == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(s.piiSecret))
	mac.Write([]byte(idNo))
	return hex.EncodeToString(mac.Sum(nil))
}
