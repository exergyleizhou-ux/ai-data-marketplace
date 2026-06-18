package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/pquerna/otp/totp"

	"golang.org/x/crypto/bcrypt"
)

// ---------- 2FA TOTP ----------

// Enroll2FA generates a TOTP secret + recovery codes. totp_enabled stays false
// until the user verifies via Verify2FAEnrollment.
func (s *Service) Enroll2FA(ctx context.Context, userID string) (Enroll2FAResult, error) {
	u, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return Enroll2FAResult{}, err
	}
	if u.TOTPEnabled {
		return Enroll2FAResult{}, ErrAlready2FAEnabled
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Verdant Oasis",
		AccountName: u.Account,
	})
	if err != nil {
		return Enroll2FAResult{}, fmt.Errorf("generate totp: %w", err)
	}
	if err := s.repo.SetTOTPSecret(ctx, userID, key.Secret()); err != nil {
		return Enroll2FAResult{}, err
	}

	// Generate 8 recovery codes (10-char alphanumeric, bcrypt-hashed for storage).
	codes := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		code := generateRecoveryCode()
		h, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.MinCost)
		if err != nil {
			// Extremely rare; skip this code and generate another.
			continue
		}
		if err := s.repo.AddRecoveryCode(ctx, userID, string(h)); err == nil {
			codes = append(codes, code)
		}
	}
	// Fallback: if every attempt failed (vanishingly unlikely), return an error.
	if len(codes) == 0 {
		return Enroll2FAResult{}, fmt.Errorf("generate recovery codes: all attempts failed")
	}
	return Enroll2FAResult{
		OTPAuthURL:    key.URL(),
		Secret:        key.Secret(),
		RecoveryCodes: codes,
	}, nil
}

// Verify2FAEnrollment checks a TOTP code and enables 2FA.
func (s *Service) Verify2FAEnrollment(ctx context.Context, userID, code string) error {
	u, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	secret, err := s.repo.GetTOTPSecret(ctx, userID)
	if err != nil || secret == "" {
		return ErrNot2FAEnrolled
	}
	if u.TOTPEnabled {
		return ErrAlready2FAEnabled
	}
	if !totp.Validate(code, secret) {
		return ErrInvalid2FACode
	}
	return s.repo.EnableTOTP(ctx, userID)
}

// RecoveryCodeStatus returns the count of unused recovery codes.
func (s *Service) RecoveryCodeStatus(ctx context.Context, userID string) (int, error) {
	return s.repo.CountUnusedRecoveryCodes(ctx, userID)
}

// Disable2FA removes TOTP and recovery codes. Requires a valid TOTP code.
func (s *Service) Disable2FA(ctx context.Context, userID, code string) error {
	secret, err := s.repo.GetTOTPSecret(ctx, userID)
	if err != nil || secret == "" {
		return ErrNot2FAEnrolled
	}
	if !totp.Validate(code, secret) {
		return ErrInvalid2FACode
	}
	return s.repo.DisableTOTP(ctx, userID)
}

// Verify2FAChallenge takes a 2FA challenge token + code and returns real tokens.
func (s *Service) Verify2FAChallenge(ctx context.Context, challengeToken, code string) (Tokens, User, error) {
	userID, err := s.tokens.Validate2FAChallenge(challengeToken)
	if err != nil {
		return Tokens{}, User{}, ErrInvalidToken
	}
	u, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return Tokens{}, User{}, err
	}
	// A frozen/banned account must not obtain tokens via the 2FA step either —
	// Login and Refresh both reject frozen users; this completion path did not.
	if u.Status == statusFrozen {
		return Tokens{}, User{}, ErrUserFrozen
	}
	secret, _ := s.repo.GetTOTPSecret(ctx, userID)
	if secret != "" && totp.Validate(code, secret) {
		tokens, err := s.tokens.Issue(u.ID, u.Role)
		if err != nil {
			return Tokens{}, User{}, err
		}
		return tokens, u, nil
	}
	// Recovery code: bcrypt-compare against stored hashes.
	if ok, err := s.repo.ConsumeRecoveryCode(ctx, userID, code); err == nil && ok {
		tokens, err := s.tokens.Issue(u.ID, u.Role)
		if err != nil {
			return Tokens{}, User{}, err
		}
		return tokens, u, nil
	}
	return Tokens{}, User{}, ErrInvalid2FACode
}

// RequestPasswordReset sends a reset link via email. Does NOT reveal whether the
// account exists (anti-enumeration).
func (s *Service) RequestPasswordReset(ctx context.Context, account string) error {
	u, _, err := s.repo.GetUserByAccount(ctx, account)
	if err != nil {
		return nil // silent — don't leak existence
	}
	rawToken := generateSecureToken(32)
	tokenHash := sha256Hex(rawToken)
	_ = s.repo.CreatePasswordResetToken(ctx, tokenHash, u.ID, time.Now().Add(1*time.Hour))

	if s.notifier != nil {
		_ = s.notifier.NotifyUser(ctx, u.ID, "password_reset_requested",
			"密码重置请求",
			"点击以下链接重置密码: "+s.appBaseURL+"/auth/reset?t="+rawToken,
			"user", u.ID)
	}
	return nil
}

// CompletePasswordReset resets the password given a valid raw token.
func (s *Service) CompletePasswordReset(ctx context.Context, rawToken, newPassword string) error {
	if len(newPassword) < 8 {
		return ErrPasswordTooWeak
	}
	tokenHash := sha256Hex(rawToken)
	userID, err := s.repo.ConsumePasswordResetToken(ctx, tokenHash)
	if err != nil {
		return ErrTokenInvalidOrExpired
	}
	// A completed reset proves account ownership — clear any login lockout so
	// the legitimate owner isn't stuck behind an attacker's failed attempts.
	_ = s.repo.ClearLoginFailures(ctx, userID)
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	if err := s.repo.UpdatePassword(ctx, userID, string(hash)); err != nil {
		return err
	}
	// Terminate existing sessions: a reset that doesn't invalidate outstanding
	// refresh tokens leaves a thief who phished/stole a token still logged in.
	// Errors are NOT swallowed — if we can't invalidate, the reset fails.
	if err := s.repo.InvalidateSessions(ctx, userID); err != nil {
		return fmt.Errorf("invalidate sessions on reset: %w", err)
	}
	return nil
}

// ---------- helpers ----------

func generateRecoveryCode() string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	b := make([]byte, 10)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[n.Int64()]
	}
	return string(b)
}

func generateSecureToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
