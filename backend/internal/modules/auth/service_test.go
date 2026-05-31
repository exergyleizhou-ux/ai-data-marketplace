package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeRepo is an in-memory Repository so the service can be tested without a DB.
type fakeRepo struct {
	byAccount map[string]userWithHash
	byID      map[string]User
	seq       int
}

type userWithHash struct {
	user User
	hash string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byAccount: map[string]userWithHash{}, byID: map[string]User{}}
}

func (r *fakeRepo) CreateUser(_ context.Context, account, accountType, passwordHash string) (User, error) {
	if _, ok := r.byAccount[account]; ok {
		return User{}, ErrAccountExists
	}
	r.seq++
	u := User{
		ID:          "user-" + itoa(r.seq),
		Account:     account,
		AccountType: accountType,
		Role:        "buyer",
		KYCStatus:   "none",
		Status:      statusActive,
	}
	r.byAccount[account] = userWithHash{user: u, hash: passwordHash}
	r.byID[u.ID] = u
	return u, nil
}

func (r *fakeRepo) GetUserByAccount(_ context.Context, account string) (User, string, error) {
	v, ok := r.byAccount[account]
	if !ok {
		return User{}, "", ErrUserNotFound
	}
	return v.user, v.hash, nil
}

func (r *fakeRepo) GetUserByID(_ context.Context, id string) (User, error) {
	u, ok := r.byID[id]
	if !ok {
		return User{}, ErrUserNotFound
	}
	return u, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func newTestService() (*Service, *fakeRepo) {
	repo := newFakeRepo()
	tm := NewTokenManager("test-secret", time.Minute, time.Hour)
	return NewService(repo, tm), repo
}

func TestRegisterAndLogin(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	res, err := svc.Register(ctx, "13800000000", accountTypePhone, "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if res.Tokens.AccessToken == "" || res.Tokens.RefreshToken == "" {
		t.Fatal("expected non-empty token pair")
	}
	if res.User.Role != "buyer" || res.User.Status != statusActive {
		t.Fatalf("unexpected user defaults: %+v", res.User)
	}

	// Duplicate account.
	if _, err := svc.Register(ctx, "13800000000", accountTypePhone, "password123"); !errors.Is(err, ErrAccountExists) {
		t.Fatalf("want ErrAccountExists, got %v", err)
	}

	// Login with correct/incorrect password.
	if _, err := svc.Login(ctx, "13800000000", "password123"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := svc.Login(ctx, "13800000000", "wrongpass"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
	// Unknown account must not leak existence.
	if _, err := svc.Login(ctx, "nope", "password123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials for unknown account, got %v", err)
	}
}

func TestRegisterValidation(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	cases := []struct{ account, typ, pw string }{
		{"", accountTypePhone, "password123"},
		{"a@b.com", "carrier-pigeon", "password123"},
		{"a@b.com", accountTypeEmail, "short"},
	}
	for i, tc := range cases {
		if _, err := svc.Register(ctx, tc.account, tc.typ, tc.pw); !errors.Is(err, ErrValidation) {
			t.Errorf("case %d: want ErrValidation, got %v", i, err)
		}
	}
}

func TestRefreshFlow(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	res, err := svc.Register(ctx, "user@example.com", accountTypeEmail, "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// A valid refresh token yields a new pair.
	refreshed, err := svc.Refresh(ctx, res.Tokens.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if refreshed.Tokens.AccessToken == "" {
		t.Fatal("expected new access token")
	}

	// An access token must NOT be accepted as a refresh token.
	if _, err := svc.Refresh(ctx, res.Tokens.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken when replaying access token, got %v", err)
	}
}

func TestLoginFrozenUser(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	if _, err := svc.Register(ctx, "frozen@example.com", accountTypeEmail, "password123"); err != nil {
		t.Fatalf("register: %v", err)
	}
	// Freeze the user directly in the fake store.
	v := repo.byAccount["frozen@example.com"]
	v.user.Status = statusFrozen
	repo.byAccount["frozen@example.com"] = v

	if _, err := svc.Login(ctx, "frozen@example.com", "password123"); !errors.Is(err, ErrUserFrozen) {
		t.Fatalf("want ErrUserFrozen, got %v", err)
	}
}

func TestTokenTypeIsolation(t *testing.T) {
	tm := NewTokenManager("test-secret", time.Minute, time.Hour)
	tokens, err := tm.Issue("user-1", "buyer")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := tm.Parse(tokens.AccessToken, tokenTypeAccess); err != nil {
		t.Fatalf("parse access: %v", err)
	}
	if _, err := tm.Parse(tokens.AccessToken, tokenTypeRefresh); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("access token must not parse as refresh, got %v", err)
	}
	if _, err := tm.Parse("garbage", tokenTypeAccess); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("garbage must fail, got %v", err)
	}
}
