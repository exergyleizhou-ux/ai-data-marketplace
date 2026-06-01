package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	tokenTypeAccess  = "access"
	tokenTypeRefresh = "refresh"
)

// Claims is the JWT payload. uid/role let downstream middleware authorize
// without a DB round-trip; typ distinguishes access from refresh tokens so a
// refresh token can never be replayed as an access token.
type Claims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"`
	Type   string `json:"typ"`
	jwt.RegisteredClaims
}

// Tokens is the issued token pair plus the access-token lifetime in seconds.
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// TokenManager issues and verifies HS256 JWTs.
//
// Every token carries a unique jti (RegisteredClaims.ID). Refresh-token
// revocation (logout + single-use rotation, docs §6.8) is implemented in the
// service via a Denylist keyed by that jti — see denylist.go.
type TokenManager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewTokenManager(secret string, accessTTL, refreshTTL time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL}
}

// Issue mints a fresh access+refresh pair for the user.
func (tm *TokenManager) Issue(userID, role string) (Tokens, error) {
	now := time.Now()
	access, err := tm.sign(userID, role, tokenTypeAccess, now, tm.accessTTL)
	if err != nil {
		return Tokens{}, err
	}
	refresh, err := tm.sign(userID, role, tokenTypeRefresh, now, tm.refreshTTL)
	if err != nil {
		return Tokens{}, err
	}
	return Tokens{AccessToken: access, RefreshToken: refresh, ExpiresIn: int64(tm.accessTTL.Seconds())}, nil
}

func (tm *TokenManager) sign(userID, role, typ string, now time.Time, ttl time.Duration) (string, error) {
	jti, err := newJTI()
	if err != nil {
		return "", err
	}
	claims := Claims{
		UserID: userID,
		Role:   role,
		Type:   typ,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(tm.secret)
}

// newJTI returns a random 128-bit token id, hex-encoded.
func newJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate jti: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Parse verifies a token's signature and expiry and asserts its type matches
// wantType. It returns ErrInvalidToken on any failure (no detail leak).
func (tm *TokenManager) Parse(tokenStr, wantType string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return tm.secret, nil
	})
	if err != nil || claims.Type != wantType {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
