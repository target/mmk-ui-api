package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/target/mmk-ui-api/internal/testutil"
)

// RequireTemplateRenderer creates a TemplateRenderer for tests, skipping the test if templates are not available.
// This centralizes the common pattern of template guard checks in tests.
func RequireTemplateRenderer(t *testing.T) *TemplateRenderer {
	t.Helper()
	// For tests, use minimal config (no resolver, no critical CSS, no dev mode)
	tr, err := NewTemplateRenderer(TemplateRendererConfig{
		TemplateFS: os.DirFS(TemplatePathFromTest),
	})
	if err != nil {
		t.Skipf("Templates not available, skipping: %v", err)
		return nil
	}
	return tr
}

// RequireTemplateRendererFromRoot creates a TemplateRenderer using the root path for tests.
// Used when the test is running from the project root directory.
func RequireTemplateRendererFromRoot(t *testing.T) *TemplateRenderer {
	t.Helper()
	// For tests, use minimal config (no resolver, no critical CSS, no dev mode)
	tr, err := NewTemplateRenderer(TemplateRendererConfig{
		TemplateFS: os.DirFS(TemplatePathFromRoot),
	})
	if err != nil {
		t.Skipf("Templates not available from root, skipping: %v", err)
		return nil
	}
	return tr
}

// SkipIfNoTemplates checks if templates are available and skips the test if not.
// This is useful for tests that need templates but don't immediately create a renderer.
func SkipIfNoTemplates(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(TemplatePathFromTest); os.IsNotExist(err) {
		t.Skip("Templates not available, skipping integration test")
	}
}

// SkipIfNoTemplatesFromRoot checks if templates are available from root and skips the test if not.
func SkipIfNoTemplatesFromRoot(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(TemplatePathFromRoot); os.IsNotExist(err) {
		t.Skip("Templates not available from root, skipping test")
	}
}

// ContainsAll checks if a string contains all the given substrings.
// This is a common utility function used in template rendering tests.
func ContainsAll(s string, subs []string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

// CreateUIHandlersForTest creates UIHandlers with a template renderer for testing.
// Returns nil if templates are not available and skips the test.
func CreateUIHandlersForTest(t *testing.T) *UIHandlers {
	t.Helper()
	tr := RequireTemplateRenderer(t)
	if tr == nil {
		return nil
	}
	return &UIHandlers{T: tr}
}

// CreateUIHandlersForTestFromRoot creates UIHandlers with a template renderer for testing from root.
// Returns nil if templates are not available and skips the test.
func CreateUIHandlersForTestFromRoot(t *testing.T) *UIHandlers {
	t.Helper()
	tr := RequireTemplateRendererFromRoot(t)
	if tr == nil {
		return nil
	}
	return &UIHandlers{T: tr}
}

// JSONRequest encapsulates the parameters needed to execute a JSON HTTP request.
type JSONRequest struct {
	Method  string
	URL     string
	Payload any
}

// DoJSON creates a request with context and performs it using an explicit client.
// This is a shared helper to avoid code duplication across test files.
func DoJSON(t testutil.TestingTB, req JSONRequest) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := getTestHTTPClient()

	var body *bytes.Reader
	if req.Payload != nil {
		b, err := json.Marshal(req.Payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = bytes.NewReader(b)
	} else {
		body = bytes.NewReader(nil)
	}

	if req.Method == "" {
		t.Fatalf("DoJSON requires Method")
	}
	if req.URL == "" {
		t.Fatalf("DoJSON requires URL")
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if req.Payload != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

var (
	testHTTPClientOnce sync.Once    //nolint:gochecknoglobals // cached for test helper
	testHTTPClient     *http.Client //nolint:gochecknoglobals // cached for test helper
)

func getTestHTTPClient() *http.Client {
	testHTTPClientOnce.Do(func() {
		testHTTPClient = &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		}
	})
	return testHTTPClient
}

// ---------------------------------------------------------------------------
// Higher-level assertion helpers (Issue #178)
// ---------------------------------------------------------------------------

// AssertBodyContains verifies that the response body contains all expected substrings.
// Each missing substring is reported as a separate test failure with context.
func AssertBodyContains(t *testing.T, body string, expected ...string) {
	t.Helper()
	for _, exp := range expected {
		assert.Contains(t, body, exp, "expected body to contain: %s", exp)
	}
}

// AssertHTMXResponse verifies an HTMX response: status 200 and a non-empty HX-Trigger header.
func AssertHTMXResponse(t *testing.T, resp *httptest.ResponseRecorder, expectedTrigger string) {
	t.Helper()
	assert.Equal(t, http.StatusOK, resp.Code)
	if expectedTrigger != "" {
		assert.Contains(t, resp.Header().Get("Hx-Trigger"), expectedTrigger)
	} else {
		assert.NotEmpty(t, resp.Header().Get("Hx-Trigger"))
	}
}

// AssertHTMXToast verifies that the response carries a showToast trigger.
func AssertHTMXToast(t *testing.T, resp *httptest.ResponseRecorder, statusCode int) {
	t.Helper()
	assert.Equal(t, statusCode, resp.Code)
	assert.Contains(t, resp.Header().Get("Hx-Trigger"), "showToast")
}

// AssertStatus verifies the response has the expected HTTP status code.
func AssertStatus(t *testing.T, resp *httptest.ResponseRecorder, expectedCode int) {
	t.Helper()
	assert.Equal(t, expectedCode, resp.Code)
}

// AssertStatusAndBody verifies the response status code and that the body contains all expected substrings.
func AssertStatusAndBody(t *testing.T, resp *httptest.ResponseRecorder, expectedCode int, expected ...string) {
	t.Helper()
	assert.Equal(t, expectedCode, resp.Code)
	body := resp.Body.String()
	for _, exp := range expected {
		assert.Contains(t, body, exp, "expected body to contain: %s", exp)
	}
}
