package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandlerGET(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	healthHandler(rec, req)

	resp := rec.Result()
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", ct)
	}

	body := rec.Body.String()
	if body != `{"status":"ok"}` {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestHealthHandlerHEAD(t *testing.T) {
	req := httptest.NewRequest(http.MethodHead, "/healthz", nil)
	rec := httptest.NewRecorder()

	healthHandler(rec, req)

	resp := rec.Result()
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", ct)
	}

	if bodyLen := rec.Body.Len(); bodyLen != 0 {
		t.Fatalf("expected empty body for HEAD request, got %d bytes", bodyLen)
	}
}
