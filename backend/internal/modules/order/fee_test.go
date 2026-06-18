package order

import "testing"

// platformFee computed `amount * platformFeeBps / 10000`, so `amount * 1000`
// overflowed int64 for amounts above ~9.2e15 cents and produced a NEGATIVE fee
// — which makes seller = amount - fee LARGER than amount, corrupting the
// earnings ledger. A dataset's price_cents is seller-controlled, so this is
// reachable. The fee must stay non-negative and proportional at any amount.
func TestPlatformFee_NoOverflowOnHugeAmount(t *testing.T) {
	const huge = int64(9_300_000_000_000_000) // 9.3e15, where amount*1000 overflows

	fee, seller := platformFee(huge)

	if fee < 0 {
		t.Fatalf("fee overflowed to negative: %d", fee)
	}
	if seller > huge {
		t.Fatalf("seller %d exceeds the amount paid %d (ledger inflation)", seller, huge)
	}
	if fee+seller != huge {
		t.Fatalf("fee(%d)+seller(%d) = %d, want %d — money must be conserved", fee, seller, fee+seller, huge)
	}
	if want := huge / 10; fee != want { // 1000 bps = 10%
		t.Fatalf("fee = %d, want %d (10%%)", fee, want)
	}
}

func TestPlatformFee_NormalAmounts(t *testing.T) {
	for _, tc := range []struct{ amount, fee int64 }{
		{0, 0}, {99, 9}, {1000, 100}, {12345, 1234},
	} {
		if fee, seller := platformFee(tc.amount); fee != tc.fee || seller != tc.amount-tc.fee {
			t.Errorf("platformFee(%d) = (%d,%d), want fee %d", tc.amount, fee, seller, tc.fee)
		}
	}
}
