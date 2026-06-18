package withdrawal

import (
	"context"
	"testing"

	"github.com/lei/ai-data-marketplace/backend/internal/platform/audit"
)

type fakeAuditRecorder struct{ entries []audit.Entry }

func (r *fakeAuditRecorder) Record(_ context.Context, e audit.Entry) {
	r.entries = append(r.entries, e)
}

// Ops withdrawal decisions are high-risk financial admin actions: each
// approve/reject/complete must be recorded in audit_logs (withdrawal.reject is
// watched by the anomaly HighRiskActionRule; the trail is a compliance need).
func TestWithdrawalOpsDecisions_Audited(t *testing.T) {
	ctx := context.Background()
	rec := &fakeAuditRecorder{}
	repo := &fakeRepo{reqs: map[string]Request{
		"wd-1": {ID: "wd-1", SellerID: "s1", AmountCents: 1000, Status: StatusPending},
		"wd-2": {ID: "wd-2", SellerID: "s2", AmountCents: 2000, Status: StatusPending},
		"wd-3": {ID: "wd-3", SellerID: "s3", AmountCents: 3000, Status: StatusApproved},
	}}
	svc := NewService(repo, &fakeEarnings{}, nil)
	svc.SetAudit(rec)

	if _, err := svc.Reject(ctx, "ops-1", "wd-1", "bad account"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(ctx, "ops-2", "wd-2", "ok"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Complete(ctx, "ops-3", "wd-3", "paid"); err != nil {
		t.Fatal(err)
	}

	got := map[string]audit.Entry{}
	for _, e := range rec.entries {
		got[e.Action] = e
	}
	for _, want := range []struct {
		action, opsID, resID string
	}{
		{"withdrawal.reject", "ops-1", "wd-1"},
		{"withdrawal.approve", "ops-2", "wd-2"},
		{"withdrawal.complete", "ops-3", "wd-3"},
	} {
		e, ok := got[want.action]
		if !ok {
			t.Errorf("missing audit entry for %s", want.action)
			continue
		}
		if e.ActorID != want.opsID || e.ActorRole != "ops" || e.ResourceType != "withdrawal" || e.ResourceID != want.resID {
			t.Errorf("%s entry = %s/%s %s:%s, want %s/ops withdrawal:%s",
				want.action, e.ActorID, e.ActorRole, e.ResourceType, e.ResourceID, want.opsID, want.resID)
		}
	}
}
