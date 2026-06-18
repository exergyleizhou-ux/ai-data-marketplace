package auth

import (
	"context"
	"testing"
	"time"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
)

type fakeAuditRecorder struct{ entries []audit.Entry }

func (r *fakeAuditRecorder) Record(_ context.Context, e audit.Entry) {
	r.entries = append(r.entries, e)
}

// Admin KYC decisions are high-risk identity actions: every approve/reject must
// land in audit_logs (it is what the anomaly HighRiskActionRule's `kyc.reject`
// allowlist entry reads, and a compliance requirement in its own right).
func TestReviewKYC_RecordsDecisionAudit(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	rec := &fakeAuditRecorder{}
	tm := NewTokenManager("test-secret", time.Minute, time.Hour)
	svc := NewService(repo, tm, WithAudit(rec))

	k1, _ := repo.SubmitKYC(ctx, KYCRecord{UserID: "u1"}, "", []byte("x"))
	k2, _ := repo.SubmitKYC(ctx, KYCRecord{UserID: "u2"}, "", []byte("y"))

	if _, err := svc.ReviewKYC(ctx, k1.ID, false, "ops-1"); err != nil { // reject
		t.Fatal(err)
	}
	if _, err := svc.ReviewKYC(ctx, k2.ID, true, "ops-2"); err != nil { // approve
		t.Fatal(err)
	}

	if len(rec.entries) != 2 {
		t.Fatalf("recorded %d audit entries, want 2 (one per decision)", len(rec.entries))
	}
	reject := rec.entries[0]
	if reject.Action != "kyc.reject" {
		t.Errorf("action = %q, want kyc.reject", reject.Action)
	}
	if reject.ActorID != "ops-1" || reject.ActorRole != "ops" {
		t.Errorf("actor = %q/%q, want ops-1/ops", reject.ActorID, reject.ActorRole)
	}
	if reject.ResourceType != "kyc" || reject.ResourceID != k1.ID {
		t.Errorf("resource = %q:%q, want kyc:%s", reject.ResourceType, reject.ResourceID, k1.ID)
	}
	if got := rec.entries[1].Action; got != "kyc.approve" {
		t.Errorf("second action = %q, want kyc.approve", got)
	}
}
