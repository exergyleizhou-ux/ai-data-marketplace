package compute

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"
)

// TestComputeFederatedDPIntegration: a federated job with dp_epsilon set releases
// a central-DP noised joint model (carries a dp block), the DP ledger records the
// spend per dataset, and the noise is live (two runs differ).
func TestComputeFederatedDPIntegration(t *testing.T) {
	const big = 1 << 20
	svc, repo, buyer, algoID, ds, _ := fedToleranceSetup(t, "fdp", []int64{big, big})
	ctx := context.Background()
	eps := 1.0

	run := func() []float64 {
		fed, err := svc.SubmitFederatedJob(ctx, buyer, FederatedSubmitInput{
			AlgorithmID: algoID, DatasetIDs: ds, DPEpsilon: &eps,
		})
		if err != nil {
			t.Fatalf("submit federated dp: %v", err)
		}
		rel := waitFedStatus(t, repo, fed.ID, FedReleased, 10*time.Second)
		rc, _, _, err := svc.OpenFederatedOutput(ctx, buyer, rel.ID)
		if err != nil {
			t.Fatalf("open dp output: %v", err)
		}
		body, _ := io.ReadAll(rc)
		rc.Close()
		var m struct {
			Weights []float64      `json:"weights"`
			DP      map[string]any `json:"dp"`
		}
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatalf("unmarshal dp model: %v", err)
		}
		if m.DP == nil || m.DP["mechanism"] != "laplace-central" {
			t.Fatalf("joint model missing dp block: %+v", m.DP)
		}
		if e, _ := m.DP["epsilon"].(float64); e != eps {
			t.Fatalf("dp epsilon=%v want %v", m.DP["epsilon"], eps)
		}
		return m.Weights
	}

	w1 := run()
	w2 := run()

	// Live noise → two runs must differ (non-reproducible, per dp_stats stance).
	same := true
	for i := range w1 {
		if w1[i] != w2[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatalf("dp noise not live: identical weights across runs %v", w1)
	}

	// DP ledger recorded for each participating dataset (>= one epsilon spend).
	for _, d := range ds {
		spent, err := repo.SumDP(ctx, d, buyer)
		if err != nil {
			t.Fatalf("sum dp: %v", err)
		}
		if spent < eps {
			t.Fatalf("dp ledger for %s = %v, want >= %v", d, spent, eps)
		}
	}
}
