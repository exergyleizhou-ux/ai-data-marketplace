package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	saved, err := s.repo.SubmitKYC(ctx, rec, s.hashIDNo(idNo))
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

// ReviewKYC is the ops action approving or rejecting a submission.
func (s *Service) ReviewKYC(ctx context.Context, kycID string, approve bool, reviewerID string) (KYCRecord, error) {
	status := kycRejected
	if approve {
		status = kycVerified
	}
	return s.repo.ReviewKYC(ctx, kycID, status, reviewerID)
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
