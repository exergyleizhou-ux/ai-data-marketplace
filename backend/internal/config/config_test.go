package config

import (
	"net"
	"os"
	"reflect"
	"testing"
)

// The built-in defaults must all be valid CIDRs, otherwise gin's
// SetTrustedProxies rejects the whole list and the server silently falls back
// to trusting no proxy.
func TestDefaultTrustedProxies_AreValidCIDRs(t *testing.T) {
	got := DefaultTrustedProxies()
	if len(got) == 0 {
		t.Fatal("DefaultTrustedProxies must not be empty")
	}
	for _, c := range got {
		if _, _, err := net.ParseCIDR(c); err != nil {
			t.Errorf("invalid default CIDR %q: %v", c, err)
		}
	}
}

func TestTrustedProxiesFromEnv(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", " 10.1.2.3/32 , , 192.168.0.0/16 ")
	if got, want := trustedProxiesFromEnv(), []string{"10.1.2.3/32", "192.168.0.0/16"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("override = %v, want %v (trimmed, empties dropped)", got, want)
	}

	os.Unsetenv("TRUSTED_PROXIES")
	if got := trustedProxiesFromEnv(); !reflect.DeepEqual(got, DefaultTrustedProxies()) {
		t.Fatalf("unset env should yield defaults, got %v", got)
	}
}
