package auth

import (
	"context"
	"errors"
	"testing"
)

// TestSubmitThenRevealIDNo proves the at-rest encryption round-trips through the
// service: a submitted ID number is recoverable via the ops reveal path.
func TestSubmitThenRevealIDNo(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	reg, err := svc.Register(ctx, "kyc-reveal@example.com", accountTypeEmail, "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	const idNo = "110101199001011234"
	rec, err := svc.SubmitKYC(ctx, reg.User.ID, kycTypePersonal, "张三", "", idNo, []string{"oss://id.jpg"})
	if err != nil {
		t.Fatalf("submit kyc: %v", err)
	}

	got, err := svc.RevealIDNo(ctx, rec.ID)
	if err != nil {
		t.Fatalf("reveal: %v", err)
	}
	if got != idNo {
		t.Fatalf("revealed id_no = %q, want %q", got, idNo)
	}
}

// TestRevealCompanyKYCHasNoIDNo: company KYC stores no ID number, so reveal
// reports nothing to retrieve rather than returning a bogus value.
func TestRevealCompanyKYCHasNoIDNo(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	reg, err := svc.Register(ctx, "company@example.com", accountTypeEmail, "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	rec, err := svc.SubmitKYC(ctx, reg.User.ID, kycTypeCompany, "", "示例公司", "", []string{"oss://license.pdf"})
	if err != nil {
		t.Fatalf("submit company kyc: %v", err)
	}
	if _, err := svc.RevealIDNo(ctx, rec.ID); !errors.Is(err, ErrIDNoNotEncrypted) {
		t.Fatalf("company reveal: got %v, want ErrIDNoNotEncrypted", err)
	}
}
