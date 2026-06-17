package compute

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestValidateAlgo(t *testing.T) {
	tests := []struct {
		name    string
		algo    Algo
		wantErr bool
	}{
		{"valid docker model", Algo{Name: "a", Image: "img", ImageDigest: "sha256:abc", Runtime: "docker", OutputKind: "model"}, false},
		{"valid wasm metrics", Algo{Name: "a", Image: "img", ImageDigest: "sha256:abc", Runtime: "wasm", OutputKind: "metrics"}, false},
		{"missing name", Algo{Image: "img", ImageDigest: "sha256:abc"}, true},
		{"missing image", Algo{Name: "a", ImageDigest: "sha256:abc"}, true},
		{"missing digest", Algo{Name: "a", Image: "img"}, true},
		{"invalid runtime", Algo{Name: "a", Image: "img", ImageDigest: "sha256:abc", Runtime: "k8s"}, true},
		{"invalid output kind", Algo{Name: "a", Image: "img", ImageDigest: "sha256:abc", OutputKind: "video"}, true},
		{"defaults runtime", Algo{Name: "a", Image: "img", ImageDigest: "sha256:abc"}, false},
		{"defaults output_kind", Algo{Name: "a", Image: "img", ImageDigest: "sha256:abc"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAlgo(&tt.algo)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAlgo() error=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestAttestation(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	input := []byte(`{"dataset_id":"ds-1","params":"{}"}`)
	output := []byte(`{"status":"ok","weights":[1,2,3]}`)

	inH, outH, sig, err := AttestResult(input, output, priv)
	if err != nil {
		t.Fatalf("attest: %v", err)
	}
	if inH == "" || outH == "" || sig == "" {
		t.Fatal("empty attestation fields")
	}

	// Verify
	ok, err := VerifyAttestation(inH, outH, sig, pub)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatal("attestation should verify")
	}

	// Tampered output should fail
	ok, err = VerifyAttestation(inH, outH+"x", sig, pub)
	if err != nil {
		t.Fatalf("verify tampered: %v", err)
	}
	if ok {
		t.Fatal("tampered attestation should NOT verify")
	}
}

func TestDefaultParamsSchema(t *testing.T) {
	if s := DefaultParamsSchema("model"); s == "" {
		t.Error("model params schema should not be empty")
	}
	if s := DefaultParamsSchema("metrics"); s == "" {
		t.Error("metrics params schema should not be empty")
	}
}
