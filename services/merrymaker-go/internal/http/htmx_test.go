package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTMX_RequestDetection(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Hx-Request", "true")
	r.Header.Set("Hx-Boosted", "true")
	if !IsHTMX(r) {
		t.Fatal("expected IsHTMX true")
	}
	if !IsBoosted(r) {
		t.Fatal("expected IsBoosted true")
	}

	r2 := httptest.NewRequest(http.MethodGet, "/x", nil)
	if IsHTMX(r2) || IsBoosted(r2) {
		t.Fatal("expected defaults to false")
	}
}

func TestHTMX_HistoryRestore_WantsPartial(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Hx-Request", "true")
	if !WantsPartial(r) {
		t.Fatal("htmx request should want partial")
	}
	r.Header.Set("Hx-History-Restore-Request", "true")
	if !WantsPartial(r) {
		t.Fatal("history restore should still want partial")
	}
}

func TestHTMX_TargetAndTrigger_Read(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Header.Set("Hx-Target", "main")
	r.Header.Set("Hx-Trigger", "btn1")
	if HXTarget(r) != "main" {
		t.Fatalf("HXTarget mismatch: %q", HXTarget(r))
	}
	if HXTrigger(r) != "btn1" {
		t.Fatalf("HXTrigger mismatch: %q", HXTrigger(r))
	}
}

func TestHTMX_ResponseHeaders_Setters(t *testing.T) {
	rr := httptest.NewRecorder()
	SetHXRedirect(rr, "/auth/login")
	SetHXPushURL(rr, "/secrets")
	SetHXRefresh(rr, true)
	SetHXTrigger(rr, "saved", map[string]any{"id": "123"})
	res := rr.Result()
	t.Cleanup(func() { _ = res.Body.Close() })
	if got := res.Header.Get("Hx-Redirect"); got != "/auth/login" {
		t.Fatalf("HX-Redirect: %q", got)
	}
	if got := res.Header.Get("Hx-Push-Url"); got != "/secrets" {
		t.Fatalf("HX-Push-Url: %q", got)
	}
	if got := res.Header.Get("Hx-Refresh"); got != "true" {
		t.Fatalf("HX-Refresh: %q", got)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(res.Header.Get("Hx-Trigger")), &payload); err != nil {
		t.Fatalf("unmarshal trigger: %v", err)
	}
	if _, ok := payload["saved"]; !ok {
		t.Fatalf("expected 'saved' key in HX-Trigger: %v", payload)
	}
}
