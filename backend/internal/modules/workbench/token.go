package workbench

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/golang-jwt/jwt/v5"
	"time"
)

var Permissions = []string{"code:run", "lab:run", "run:read", "run:cancel", "approval:decide", "artifact:read"}

type Claims struct {
	UID         string   `json:"uid"`
	WorkspaceID string   `json:"workspace_id"`
	Permissions []string `json:"permissions"`
	jwt.RegisteredClaims
}
type TokenManager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewTokenManager(secret string, ttl time.Duration) *TokenManager {
	if ttl <= 0 || ttl > 10*time.Minute {
		ttl = 5 * time.Minute
	}
	return &TokenManager{[]byte(secret), ttl, time.Now}
}
func (m *TokenManager) Issue(uid, wid string) (string, int64, error) {
	now := m.now()
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", 0, e
	}
	c := Claims{uid, wid, Permissions, jwt.RegisteredClaims{Issuer: "oasis", Audience: jwt.ClaimStrings{"lumen-workbench"}, Subject: uid, ID: hex.EncodeToString(b), IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl))}}
	t, e := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(m.secret)
	return t, int64(m.ttl.Seconds()), e
}
