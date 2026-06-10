package anomaly

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookAlerter_PostsPayloadOnAlert(t *testing.T) {
	var received bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.Header.Get("Content-Type") == "application/json" {
			received = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := NewWebhookAlerter(srv.URL, nil)
	err := a.Alert(context.Background(), Anomaly{
		Kind: "high_risk_action", Count: 5, ResourcePattern: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !received {
		t.Fatal("webhook must receive POST")
	}
}

func TestWebhookAlerter_SkipsKindNotInWhitelist(t *testing.T) {
	var received bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := NewWebhookAlerter(srv.URL, []string{"high_risk_action"})
	err := a.Alert(context.Background(), Anomaly{
		Kind: "bulk_modification", ResourcePattern: "y",
	})
	if err != nil {
		t.Fatal(err)
	}
	if received {
		t.Fatal("webhook must NOT be called for non-whitelisted kind")
	}
}

func TestWebhookAlerter_5xxReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := NewWebhookAlerter(srv.URL, nil)
	err := a.Alert(context.Background(), Anomaly{
		Kind: "high_risk_action", ResourcePattern: "z",
	})
	if err == nil {
		t.Fatal("5xx must return error")
	}
}

func TestNopAlerter_AlwaysNil(t *testing.T) {
	var a NopAlerter
	err := a.Alert(context.Background(), Anomaly{})
	if err != nil {
		t.Fatal("NopAlerter must always return nil")
	}
}
