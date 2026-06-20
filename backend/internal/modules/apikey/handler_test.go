package apikey

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/httpx"
)

// fakeRepo is an in-memory Repository for handler + middleware tests.
type fakeRepo struct {
	byHash map[string]APIKey
	byID   map[string]APIKey
}

func newFake() *fakeRepo { return &fakeRepo{byHash: map[string]APIKey{}, byID: map[string]APIKey{}} }

func (f *fakeRepo) Create(_ context.Context, k APIKey, hash string) (APIKey, error) {
	k.ID = "id-" + hash[:8]
	if k.Tier == "" {
		k.Tier = "free"
	}
	f.byHash[hash] = k
	f.byID[k.ID] = k
	return k, nil
}
func (f *fakeRepo) AuthenticateAndMeter(_ context.Context, hash, month string) (APIKey, error) {
	k, ok := f.byHash[hash]
	if !ok || k.Revoked() {
		return APIKey{}, ErrInvalidKey
	}
	if k.UsageMonth == month && k.UsageCount >= TierOf(k.Tier).MonthlyQuota {
		return APIKey{}, ErrQuotaExceeded
	}
	if k.UsageMonth == month {
		k.UsageCount++
	} else {
		k.UsageMonth, k.UsageCount = month, 1
	}
	f.byHash[hash] = k
	f.byID[k.ID] = k
	return k, nil
}
func (f *fakeRepo) ListByAccount(_ context.Context, acct string) ([]APIKey, error) {
	var out []APIKey
	for _, k := range f.byID {
		if k.AccountID == acct {
			out = append(out, k)
		}
	}
	return out, nil
}
func (f *fakeRepo) SetAccountTier(_ context.Context, acct, tier string) (int, error) {
	n := 0
	for h, k := range f.byHash {
		if k.AccountID == acct && !k.Revoked() {
			k.Tier = tier
			f.byHash[h] = k
			f.byID[k.ID] = k
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) Revoke(_ context.Context, acct, id string) error {
	k, ok := f.byID[id]
	if !ok || k.AccountID != acct {
		return ErrNotFound
	}
	k.RevokedAt = "2026-06-20T00:00:00Z"
	f.byID[id] = k
	for h, kk := range f.byHash {
		if kk.ID == id {
			kk.RevokedAt = k.RevokedAt
			f.byHash[h] = kk
		}
	}
	return nil
}

// fakeAuth mimics the JWT middleware by injecting a fixed account id.
func fakeAuth(userID string) gin.HandlerFunc {
	return func(c *gin.Context) { c.Set(httpx.AuthUserIDKey, userID); c.Next() }
}

func newEngine(svc *Service, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	Register(e.Group("/api/v1"), svc, fakeAuth(userID))
	return e
}

func TestIssueListRevoke(t *testing.T) {
	svc := NewService(newFake())
	e := newEngine(svc, "user-1")

	// issue
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", strings.NewReader(`{"name":"ci","tier":"free"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("issue status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Key, ID, Prefix, Tier string
		}
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.HasPrefix(resp.Data.Key, "sk_live_") || resp.Data.Tier != "free" {
		t.Fatalf("issue payload unexpected: %+v", resp.Data)
	}

	// list shows it (metadata only, no plaintext)
	w = httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/api-keys", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), resp.Data.Prefix) {
		t.Fatalf("list missing key: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), resp.Data.Key) {
		t.Fatal("list must NOT contain the plaintext key")
	}

	// revoke
	w = httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/api-keys/"+resp.Data.ID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", w.Code, w.Body.String())
	}
}

// TestAPIKeyAuthMiddleware: the metered gate — valid key passes, missing/invalid
// → 401, over-quota → 429.
func TestAPIKeyAuthMiddleware(t *testing.T) {
	repo := newFake()
	svc := NewService(repo)
	k, plain, _ := svc.Issue(context.Background(), "acct-1", "k", "free")
	_ = k

	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.GET("/protected", APIKeyAuth(svc), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"account": AccountID(c), "tier": KeyTier(c)})
	})

	call := func(hdr, val string) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/protected", nil)
		if hdr != "" {
			r.Header.Set(hdr, val)
		}
		e.ServeHTTP(w, r)
		return w.Code
	}

	if code := call("X-API-Key", plain); code != http.StatusOK {
		t.Fatalf("valid key: status=%d, want 200", code)
	}
	if code := call("", ""); code != http.StatusUnauthorized {
		t.Fatalf("missing key: status=%d, want 401", code)
	}
	if code := call("X-API-Key", "sk_live_bogus"); code != http.StatusUnauthorized {
		t.Fatalf("invalid key: status=%d, want 401", code)
	}
	// burn the free quota, then expect 429
	for i := 1; i < TierOf("free").MonthlyQuota; i++ {
		_, _ = svc.Authenticate(context.Background(), plain)
	}
	if code := call("Authorization", "Bearer "+plain); code != http.StatusTooManyRequests {
		t.Fatalf("over quota: status=%d, want 429", code)
	}
}
