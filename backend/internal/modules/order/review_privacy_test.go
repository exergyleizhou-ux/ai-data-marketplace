package order

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestReviewJSON_OmitsPurchaserLinkage: the reviews list is public, so a
// serialized Review must NOT expose buyer_id or order_id (which would
// deanonymize every purchaser and link them to an order).
func TestReviewJSON_OmitsPurchaserLinkage(t *testing.T) {
	b, err := json.Marshal(Review{
		ID: "rv1", OrderID: "ord-secret", DatasetID: "ds1", BuyerID: "buyer-secret",
		Score: 5, Comment: "good", IssueFlag: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, leak := range []string{"buyer_id", "buyer-secret", "order_id", "ord-secret"} {
		if strings.Contains(s, leak) {
			t.Errorf("public review JSON leaks %q: %s", leak, s)
		}
	}
	// The public fields are still present.
	if !strings.Contains(s, `"score":5`) || !strings.Contains(s, `"dataset_id":"ds1"`) {
		t.Errorf("public review JSON missing expected fields: %s", s)
	}
}
