package quality

import (
	"strings"
	"testing"
)

// 440524188001010014 is a checksum-valid PRC ID; 4111111111111111 is a
// Luhn-valid card (Visa test number). Their one-off mutations are invalid.
const (
	validID      = "440524188001010014"
	invalidID    = "440524188001010015" // wrong check digit
	validCard    = "4111111111111111"
	invalidCard  = "4111111111111112" // breaks Luhn
	validPhone   = "13800138000"
	validEmail   = "ops@verdantoasis.cn"
	validIP      = "192.168.31.7"
	invalidIP    = "999.168.31.7" // octet > 255
	validPass    = "E12345678"
	validPlate   = "京A12345"
	validGPS     = "30.2741,120.1551"
	validAddress = "杭州市西湖区文三路100号"
)

func piiCounts(content string) map[string]int {
	counts := map[string]int{}
	for _, m := range scan(content) {
		counts[m.name]++
	}
	return counts
}

func TestValidatorsCutFalsePositives(t *testing.T) {
	cases := []struct {
		name, content, detector string
		wantHit                 bool
	}{
		{"valid id", "身份证 " + validID + " 。", "id_card", true},
		{"invalid id checksum", "编号 " + invalidID + " 。", "id_card", false},
		{"valid card", "卡号 " + validCard + " 。", "bank_card", true},
		{"invalid card luhn", "订单 " + invalidCard + " 。", "bank_card", false},
		{"valid ipv4", "来自 " + validIP + " 访问", "ipv4", true},
		{"invalid ipv4 octet", "版本 " + invalidIP + " 发布", "ipv4", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := piiCounts(c.content)[c.detector] > 0
			if got != c.wantHit {
				t.Errorf("%s: detector %q hit=%v, want %v (counts=%v)", c.name, c.detector, got, c.wantHit, piiCounts(c.content))
			}
		})
	}
}

func TestAllDetectorsFire(t *testing.T) {
	content := strings.Join([]string{
		"身份证 " + validID,
		"银行卡 " + validCard,
		"手机 " + validPhone,
		"邮箱 " + validEmail,
		"地址IP " + validIP,
		"护照 " + validPass,
		"车牌 " + validPlate,
		"坐标 " + validGPS,
		"住址 " + validAddress,
	}, "；")
	counts := piiCounts(content)
	for _, want := range []string{"id_card", "bank_card", "phone", "email", "ipv4", "passport", "plate", "gps", "address"} {
		if counts[want] == 0 {
			t.Errorf("detector %q did not fire (counts=%v)", want, counts)
		}
	}
}

func TestMaskPreservesFlankingText(t *testing.T) {
	// The old ReplaceAllString ate the boundary chars; span masking must not.
	got := MaskPII("证号" + validID + "。结束")
	want := "证号" + piiMask + "。结束"
	if got != want {
		t.Errorf("MaskPII flanking: got %q, want %q", got, want)
	}
}

func TestMaskCatchesAdjacentPII(t *testing.T) {
	// Two phones separated by a single space — boundary-consuming regexes miss
	// the second one; the flank-rule engine must catch both.
	got := MaskPII(validPhone + " " + "13900139000")
	want := piiMask + " " + piiMask
	if got != want {
		t.Errorf("adjacent PII: got %q, want %q", got, want)
	}
}

func TestRedactionInvariantHolds(t *testing.T) {
	// Anything MaskPII produces must re-scan to zero PII — the core guarantee.
	corpus := []string{
		"clean training text with no personal data 没有个人信息",
		"客户 " + validID + " 手机 " + validPhone + " 卡 " + validCard,
		validEmail + "," + validIP + "," + validPlate,
		validPhone + validPhone, // pathological back-to-back
	}
	for _, c := range corpus {
		if residual := scan(MaskPII(c)); len(residual) != 0 {
			t.Errorf("residual PII after masking %q: %v", c, residual)
		}
	}
}

func TestPIIRedactionCheck(t *testing.T) {
	clean := PIIRedaction([]byte("纯净语料，无任何个人信息。clean corpus."))
	if clean.Result != ResultPass || clean.Report["detected_total"].(int) != 0 {
		t.Errorf("clean redaction check: %s %v", clean.Result, clean.Report)
	}

	dirty := PIIRedaction([]byte("联系 " + validPhone + " 身份证 " + validID))
	if dirty.Result != ResultPass {
		t.Errorf("redaction of detectable PII should still verify clean, got %s %v", dirty.Result, dirty.Report)
	}
	if v, _ := dirty.Report["verified"].(bool); !v {
		t.Errorf("redaction must report verified=true, got %v", dirty.Report)
	}
	if dirty.Report["residual_total"].(int) != 0 {
		t.Errorf("residual_total must be 0, got %v", dirty.Report)
	}
	if dirty.Report["detected_total"].(int) < 2 {
		t.Errorf("detected_total should count both phone and id, got %v", dirty.Report)
	}
}

func TestOverlapCountedOnce(t *testing.T) {
	// A valid ID is also a long digit run; it must count once, not as id+card.
	counts := piiCounts("证件 " + validID + " 完")
	total := 0
	for _, n := range counts {
		total += n
	}
	if total != 1 {
		t.Errorf("overlapping spans should count once, got %d (%v)", total, counts)
	}
}

func TestMaskTokenIsInert(t *testing.T) {
	// The mask token itself must never be detected as PII (else redaction loops).
	if c := scan(strings.Repeat(piiMask, 50)); len(c) != 0 {
		t.Errorf("mask token detected as PII: %v", c)
	}
}
