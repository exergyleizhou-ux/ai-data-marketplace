package compute

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// --- remote Key Broker Service (KBS) client — Direction B 阶段2 ---
//
// remoteKBS talks to a REAL Key Broker Service over HTTP: it forwards the
// attestation report (which a real TEE attester produced on a TEE node) and the
// dataset id, and the KBS — a separate trust domain — verifies the attestation
// against the hardware root of trust + measurement policy and releases the
// dataset's data key only if it passes. The platform never holds the key unless
// a genuine attestation earned it (design P3 §4 / Direction B §4-§5).
//
// This client is the locally-verifiable half of "real TEE": a real KBS is just a
// URL (Confidential Containers KBS / a cloud KMS attestation-gated release). The
// hardware quote GENERATION (reading /dev/tdx_guest etc.) is the TEE-node half,
// documented in docs/部署-L2-TEE节点与KBS.md. Fail-closed: any non-200, empty
// key, or transport error yields NO key.
//
// Protocol (simple, adaptable to a concrete KBS): POST {report(base64), dataset_id}
// → 200 {data_key(base64)} on release, non-200 on denial.
type remoteKBS struct {
	url    string
	client *http.Client
}

// NewRemoteKBS returns a KeyBroker that requests attestation-gated key release
// from the KBS at url.
func NewRemoteKBS(url string) KeyBroker {
	return &remoteKBS{url: url, client: &http.Client{Timeout: 10 * time.Second}}
}

type kbsReleaseRequest struct {
	Report    string `json:"report"`     // base64 of the attestation report
	DatasetID string `json:"dataset_id"` // which dataset's key to release
}

type kbsReleaseResponse struct {
	DataKey string `json:"data_key"` // base64 of the released data key
}

// ReleaseDataKey implements KeyBroker by asking the remote KBS to release the key.
func (k *remoteKBS) ReleaseDataKey(ctx context.Context, report []byte, datasetID string) ([]byte, error) {
	body, err := json.Marshal(kbsReleaseRequest{
		Report:    base64.StdEncoding.EncodeToString(report),
		DatasetID: datasetID,
	})
	if err != nil {
		return nil, fmt.Errorf("kbs: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, k.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("kbs: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := k.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kbs: request failed: %w", err) // fail closed on transport error
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Non-200 = the KBS denied release (bad attestation / policy) — no key.
		return nil, fmt.Errorf("%w: kbs returned status %d", ErrAttestationInvalid, resp.StatusCode)
	}
	var out kbsReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("kbs: decode response: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(out.DataKey)
	if err != nil {
		return nil, fmt.Errorf("kbs: decode data key: %w", err)
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("kbs: released an empty data key")
	}
	return key, nil
}
