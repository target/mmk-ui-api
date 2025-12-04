package httpx

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCSRFProtection_GetRequestsAllowed(t *testing.T) {
	cfg := CSRFConfig{
		CookieName:    DefaultCSRFCookieName,
		HeaderName:    DefaultCSRFHeaderName,
		FormFieldName: DefaultCSRFCookieName,
		TokenLength:   DefaultCSRFTokenLength,
	}

	handler := CSRFProtection(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Check that CSRF cookie was set
	resp := w.Result()
	defer resp.Body.Close()
	cookies := resp.Cookies()
	var csrfCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == DefaultCSRFCookieName {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatal("CSRF cookie not set")
	}
	if csrfCookie.Value == "" {
		t.Error("CSRF token is empty")
	}
}

func TestCSRFProtection_PostWithoutTokenFails(t *testing.T) {
	cfg := CSRFConfig{
		CookieName:    DefaultCSRFCookieName,
		HeaderName:    DefaultCSRFHeaderName,
		FormFieldName: DefaultCSRFCookieName,
		TokenLength:   DefaultCSRFTokenLength,
	}

	handler := CSRFProtection(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}

func TestCSRFProtection_PostWithValidHeaderToken(t *testing.T) {
	cfg := CSRFConfig{
		CookieName:    DefaultCSRFCookieName,
		HeaderName:    DefaultCSRFHeaderName,
		FormFieldName: DefaultCSRFCookieName,
		TokenLength:   DefaultCSRFTokenLength,
	}

	handler := CSRFProtection(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))

	// First request to get token
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	// Extract token from cookie
	resp1 := w1.Result()
	defer resp1.Body.Close()
	var token string
	for _, c := range resp1.Cookies() {
		if c.Name == DefaultCSRFCookieName {
			token = c.Value
			break
		}
	}

	// Second request with token in header
	req2 := httptest.NewRequest(http.MethodPost, "/test", nil)
	req2.AddCookie(&http.Cookie{Name: DefaultCSRFCookieName, Value: token})
	req2.Header.Set(DefaultCSRFHeaderName, token)
	w2 := httptest.NewRecorder()

	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w2.Code)
	}
}

func TestCSRFProtection_PostWithValidFormToken(t *testing.T) {
	cfg := CSRFConfig{
		CookieName:    DefaultCSRFCookieName,
		HeaderName:    DefaultCSRFHeaderName,
		FormFieldName: DefaultCSRFCookieName,
		TokenLength:   DefaultCSRFTokenLength,
	}

	handler := CSRFProtection(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))

	// First request to get token
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	// Extract token from cookie
	resp1 := w1.Result()
	defer resp1.Body.Close()
	var token string
	for _, c := range resp1.Cookies() {
		if c.Name == DefaultCSRFCookieName {
			token = c.Value
			break
		}
	}

	// Second request with token in form
	form := url.Values{}
	form.Set(DefaultCSRFCookieName, token)
	req2 := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(form.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.AddCookie(&http.Cookie{Name: DefaultCSRFCookieName, Value: token})
	w2 := httptest.NewRecorder()

	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w2.Code)
	}
}

func TestCSRFProtection_PostWithMismatchedToken(t *testing.T) {
	cfg := CSRFConfig{
		CookieName:    DefaultCSRFCookieName,
		HeaderName:    DefaultCSRFHeaderName,
		FormFieldName: DefaultCSRFCookieName,
		TokenLength:   DefaultCSRFTokenLength,
	}

	handler := CSRFProtection(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.AddCookie(&http.Cookie{Name: DefaultCSRFCookieName, Value: "cookie-token"})
	req.Header.Set(DefaultCSRFHeaderName, "different-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}

func TestCSRFProtection_SafeMethodsExempt(t *testing.T) {
	cfg := CSRFConfig{
		CookieName:    DefaultCSRFCookieName,
		HeaderName:    DefaultCSRFHeaderName,
		FormFieldName: DefaultCSRFCookieName,
		TokenLength:   DefaultCSRFTokenLength,
	}

	handler := CSRFProtection(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	safeMethods := []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace}

	for _, method := range safeMethods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/test", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("method %s: expected status 200, got %d", method, w.Code)
			}
		})
	}
}

func TestCSRFProtection_TokenInContext(t *testing.T) {
	cfg := CSRFConfig{
		CookieName:    DefaultCSRFCookieName,
		HeaderName:    DefaultCSRFHeaderName,
		FormFieldName: DefaultCSRFCookieName,
		TokenLength:   DefaultCSRFTokenLength,
	}

	var capturedToken string
	handler := CSRFProtection(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedToken = GetCSRFToken(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if capturedToken == "" {
		t.Error("CSRF token not available in context")
	}

	// Verify token matches cookie
	resp := w.Result()
	defer resp.Body.Close()
	var cookieToken string
	for _, c := range resp.Cookies() {
		if c.Name == DefaultCSRFCookieName {
			cookieToken = c.Value
			break
		}
	}

	if capturedToken != cookieToken {
		t.Errorf("context token %q does not match cookie token %q", capturedToken, cookieToken)
	}
}

func TestCSRFProtection_CookieAttributes_HTTPS(t *testing.T) {
	cfg := CSRFConfig{
		CookieName:   DefaultCSRFCookieName,
		CookieDomain: "example.com",
		TokenLength:  DefaultCSRFTokenLength,
	}

	handler := CSRFProtection(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "https://example.com/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()
	var csrfCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == DefaultCSRFCookieName {
			csrfCookie = c
			break
		}
	}

	if csrfCookie == nil {
		t.Fatal("CSRF cookie not set")
	}

	if !csrfCookie.Secure {
		t.Error("expected Secure flag to be true for HTTPS request")
	}
	if csrfCookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("expected SameSite=Strict, got %v", csrfCookie.SameSite)
	}
	if csrfCookie.HttpOnly {
		t.Error("expected HttpOnly to be false (must be readable by JavaScript)")
	}
	if csrfCookie.Domain != "example.com" {
		t.Errorf("expected Domain=example.com, got %q", csrfCookie.Domain)
	}
	if csrfCookie.Path != "/" {
		t.Errorf("expected Path=/, got %q", csrfCookie.Path)
	}
}

func TestCSRFProtection_CookieAttributes_ForwardedProto(t *testing.T) {
	cfg := CSRFConfig{
		CookieName:   DefaultCSRFCookieName,
		CookieDomain: "example.com",
		TokenLength:  DefaultCSRFTokenLength,
	}

	handler := CSRFProtection(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()
	var csrfCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == DefaultCSRFCookieName {
			csrfCookie = c
			break
		}
	}

	if csrfCookie == nil {
		t.Fatal("CSRF cookie not set")
	}

	if !csrfCookie.Secure {
		t.Error("expected Secure flag to be true when X-Forwarded-Proto=https")
	}
}

func TestCSRFProtection_CookieNotSetWhenExists(t *testing.T) {
	cfg := CSRFConfig{
		CookieName:  DefaultCSRFCookieName,
		TokenLength: DefaultCSRFTokenLength,
	}

	handler := CSRFProtection(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request generates token
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	resp1 := w1.Result()
	defer resp1.Body.Close()
	cookies1 := resp1.Cookies()
	if len(cookies1) == 0 {
		t.Fatal("expected cookie to be set on first request")
	}

	// Second request with existing cookie should not set cookie again
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.AddCookie(cookies1[0])
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	resp2 := w2.Result()
	defer resp2.Body.Close()
	cookies2 := resp2.Cookies()
	if len(cookies2) > 0 {
		t.Error("expected no Set-Cookie header when token already exists")
	}
}

func TestCSRFProtection_ContentTypeFiltering(t *testing.T) {
	cfg := CSRFConfig{
		CookieName:    DefaultCSRFCookieName,
		HeaderName:    DefaultCSRFHeaderName,
		FormFieldName: DefaultCSRFCookieName,
		TokenLength:   DefaultCSRFTokenLength,
	}

	handler := CSRFProtection(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Get token first
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	resp1 := w1.Result()
	defer resp1.Body.Close()
	var token string
	for _, c := range resp1.Cookies() {
		if c.Name == DefaultCSRFCookieName {
			token = c.Value
			break
		}
	}

	t.Run("JSON POST without header fails", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"key":"value"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: DefaultCSRFCookieName, Value: token})
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected status 403 for JSON POST without header, got %d", w.Code)
		}
	})

	t.Run("JSON POST with header succeeds", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"key":"value"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(DefaultCSRFHeaderName, token)
		req.AddCookie(&http.Cookie{Name: DefaultCSRFCookieName, Value: token})
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200 for JSON POST with header, got %d", w.Code)
		}
	})
}

func TestGetCSRFToken_NoToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	token := GetCSRFToken(req)
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}
