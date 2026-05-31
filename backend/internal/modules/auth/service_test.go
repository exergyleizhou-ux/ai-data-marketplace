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
	kyc       map[string]KYCRecord // by kyc id
	seq       int
	kycSeq    int
}

type userWithHash struct {
	user User
	hash string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byAccount: map[string]userWithHash{}, byID: map[string]User{}, kyc: map[string]KYCRecord{}}
}

func (r *fakeRepo) setKYCStatus(userID, status string) {
	u := r.byID[userID]
	u.KYCStatus = status
	r.byID[userID] = u
	if v, ok := r.byAccount[u.Account]; ok {
		v.user.KYCStatus = status
		r.byAccount[u.Account] = v
	}
}

func (r *fakeRepo) UpdateUserRole(_ context.Context, id, role string) (User, error) {
	u, ok := r.byID[id]
	if !ok {
		return User{}, ErrUserNotFound
	}
	u.Role = role
	r.byID[id] = u
	if v, ok := r.byAccount[u.Account]; ok {
		v.user.Role = role
		r.byAccount[u.Account] = v
	}
	return u, nil
}

func (r *fakeRepo) SubmitKYC(_ context.Context, rec KYCRecord, _ string) (KYCRecord, error) {
	r.kycSeq++
	rec.ID = "kyc-" + itoa(r.kycSeq)
	rec.VerifyStatus = kycPending
	r.kyc[rec.ID] = rec
	r.setKYCStatus(rec.UserID, kycPending)
	return rec, nil
}

func (r *fakeRepo) GetLatestKYC(_ context.Context, userID string) (KYCRecord, error) {
	var latest KYCRecord
	var found bool
	for _, rec := range r.kyc {
		if rec.UserID == userID {
			latest = rec
			found = true
		}
	}
	if !found {
		return KYCRecord{}, ErrKYCNotFound
	}
	return latest, nil
}

func (r *fakeRepo) ReviewKYC(_ context.Context, kycID, newStatus, _ string) (KYCRecord, error) {
	rec, ok := r.kyc[kycID]
	if !ok {
		return KYCRecord{}, ErrKYCNotFound
	}
	rec.VerifyStatus = newStatus
	r.kyc[kycID] = rec
	r.setKYCStatus(rec.UserID, newStatus)
	return rec, nil
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

func TestKYCManualReviewFlow(t *testing.T) {
	svc, repo := newTestService() // default ManualVerifier
	ctx := context.Background()

	reg, err := svc.Register(ctx, "seller@example.com", accountTypeEmail, "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	uid := reg.User.ID

	rec, err := svc.SubmitKYC(ctx, uid, kycTypePersonal, "张三", "", "110101199001011234", []string{"oss://id-front.jpg"})
	if err != nil {
		t.Fatalf("submit kyc: %v", err)
	}
	if rec.VerifyStatus != kycPending {
		t.Fatalf("manual verifier should leave kyc pending, got %q", rec.VerifyStatus)
	}
	if u, _ := svc.Me(ctx, uid); u.KYCStatus != kycPending {
		t.Fatalf("user kyc_status = %q, want pending", u.KYCStatus)
	}

	// Ops approves.
	if _, err := svc.ReviewKYC(ctx, rec.ID, true, "ops-1"); err != nil {
		t.Fatalf("review: %v", err)
	}
	if u, _ := svc.Me(ctx, uid); u.KYCStatus != kycVerified {
		t.Fatalf("after approve user kyc_status = %q, want verified", u.KYCStatus)
	}
	_ = repo
}

func TestKYCValidationAndAutoApprove(t *testing.T) {
	repo := newFakeRepo()
	tm := NewTokenManager("test-secret", time.Minute, time.Hour)
	svc := NewService(repo, tm, WithKYC(AutoApproveVerifier{}, "pii-secret"))
	ctx := context.Background()

	reg, _ := svc.Register(ctx, "u@example.com", accountTypeEmail, "password123")

	// personal without id_no -> validation error.
	if _, err := svc.SubmitKYC(ctx, reg.User.ID, kycTypePersonal, "张三", "", "", nil); !errors.Is(err, ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
	// auto-approve verifies immediately.
	rec, err := svc.SubmitKYC(ctx, reg.User.ID, kycTypeCompany, "", "示例公司", "", []string{"oss://license.pdf"})
	if err != nil {
		t.Fatalf("submit kyc: %v", err)
	}
	if rec.VerifyStatus != kycVerified {
		t.Fatalf("auto-approve should verify, got %q", rec.VerifyStatus)
	}
}

func TestUpdateRole(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	reg, _ := svc.Register(ctx, "u2@example.com", accountTypeEmail, "password123")

	u, err := svc.UpdateRole(ctx, reg.User.ID, roleBoth)
	if err != nil || u.Role != roleBoth {
		t.Fatalf("update role both: %v role=%q", err, u.Role)
	}
	// Privileged roles cannot be self-assigned.
	if _, err := svc.UpdateRole(ctx, reg.User.ID, roleAdmin); !errors.Is(err, ErrValidation) {
		t.Fatalf("want ErrValidation for admin self-assign, got %v", err)
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
