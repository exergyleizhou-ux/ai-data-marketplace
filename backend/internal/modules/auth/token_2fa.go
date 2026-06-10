package auth

import (
	"fmt"
	"time"
)

// Issue2FAChallenge issues a short-lived JWT (5 min) for 2FA verification.
func (tm *TokenManager) Issue2FAChallenge(userID string) (string, error) {
	return tm.sign(userID, "", "2fa_challenge", time.Now(), 5*time.Minute)
}

// Validate2FAChallenge validates a 2FA challenge token and returns the userID.
func (tm *TokenManager) Validate2FAChallenge(tokenStr string) (string, error) {
	claims, err := tm.Parse(tokenStr, "2fa_challenge")
	if err != nil {
		return "", fmt.Errorf("%w: invalid 2fa challenge", ErrInvalidToken)
	}
	return claims.UserID, nil
}
