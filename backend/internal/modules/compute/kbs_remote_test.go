package compute

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemoteKBS_ReleasesKeyOn200(t *testing.T) {
	wantKey := []byte("sixteen-byte-key")
	var gotReport []byte
	var gotDataset string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Report    string `json:"report"`
			DatasetID string `json:"dataset_id"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		gotReport, _ = base64.StdEncoding.DecodeString(req.Report)
		gotDataset = req.DatasetID
		_ = json.NewEncoder(w).Encode(map[string]string{"data_key": base64.StdEncoding.EncodeToString(wantKey)})
	}))
	defer srv.Close()

	key, err := NewRemoteKBS(srv.URL).ReleaseDataKey(context.Background(), []byte(`{"q":1}`), "ds-7")
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if string(key) != string(wantKey) {
		t.Fatalf("key = %q, want %q", key, wantKey)
	}
	if string(gotReport) != `{"q":1}` {
		t.Fatalf("KBS got report %q", gotReport)
	}
	if gotDataset != "ds-7" {
		t.Fatalf("KBS got dataset %q", gotDataset)
	}
}

func TestRemoteKBS_FailsClosedOnDenied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	if _, err := NewRemoteKBS(srv.URL).ReleaseDataKey(context.Background(), []byte("r"), "ds"); err == nil {
		t.Fatal("a 403 from the KBS must fail closed (no key)")
	}
}

func TestRemoteKBS_FailsClosedOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := NewRemoteKBS(srv.URL).ReleaseDataKey(context.Background(), []byte("r"), "ds"); err == nil {
		t.Fatal("a 5xx from the KBS must fail closed")
	}
}

func TestRemoteKBS_FailsClosedOnEmptyKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"data_key": ""})
	}))
	defer srv.Close()
	if _, err := NewRemoteKBS(srv.URL).ReleaseDataKey(context.Background(), []byte("r"), "ds"); err == nil {
		t.Fatal("an empty released key must be treated as a failure")
	}
}

func TestRemoteKBS_FailsClosedOnMalformedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()
	if _, err := NewRemoteKBS(srv.URL).ReleaseDataKey(context.Background(), []byte("r"), "ds"); err == nil {
		t.Fatal("a malformed KBS response must fail closed")
	}
}
