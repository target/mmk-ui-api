package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUI_Index_RendersLayout(t *testing.T) {
	h := CreateUIHandlersForTest(t)
	if h == nil {
		return
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	h.Index(rr, r)

	res := rr.Result()
	t.Cleanup(func() { _ = res.Body.Close() })

	if got := res.StatusCode; got != http.StatusOK {
		// Default status is 200 if WriteHeader isn't called; enforce it.
		t.Fatalf("expected status 200, got %d", got)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html content type, got %q", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "<main id=\"main\">") {
		t.Fatalf("expected body to contain main container, got: %s", body)
	}
}
