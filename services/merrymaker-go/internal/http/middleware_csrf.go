package httpx

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

const (
	// DefaultCSRFCookieName is the default name for the CSRF cookie.
	DefaultCSRFCookieName = "csrf_token"
	// DefaultCSRFHeaderName is the default name for the CSRF header (canonical form).
	DefaultCSRFHeaderName = "X-Csrf-Token"
	// DefaultCSRFTokenLength is the default length of the CSRF token in bytes.
	DefaultCSRFTokenLength = 32
)

// CSRFConfig holds configuration for CSRF protection middleware.
type CSRFConfig struct {
	// CookieName is the name of the CSRF cookie (default: "csrf_token")
	CookieName string
	// HeaderName is the name of the CSRF header to check (default: "X-Csrf-Token")
	HeaderName string
	// FormFieldName is the name of the form field to check (default: "csrf_token")
	FormFieldName string
	// CookieDomain is the domain for the CSRF cookie
	CookieDomain string
	// TokenLength is the length of the CSRF token in bytes (default: 32)
	TokenLength int
}

// CSRFProtection returns a middleware that protects against CSRF attacks using the double-submit cookie pattern.
// It generates a random token, stores it in a cookie, and validates it on state-changing requests (POST, PUT, DELETE, PATCH).
// The token can be submitted via:
// - X-Csrf-Token header (for HTMX/AJAX requests)
// - csrf_token form field (for standard form submissions)
//
// GET, HEAD, OPTIONS, and TRACE requests are exempt from CSRF validation.
func CSRFProtection(cfg CSRFConfig) func(http.Handler) http.Handler {
	// Set defaults
	if cfg.CookieName == "" {
		cfg.CookieName = DefaultCSRFCookieName
	}
	if cfg.HeaderName == "" {
		cfg.HeaderName = DefaultCSRFHeaderName
	}
	if cfg.FormFieldName == "" {
		cfg.FormFieldName = DefaultCSRFCookieName
	}
	if cfg.TokenLength == 0 {
		cfg.TokenLength = DefaultCSRFTokenLength
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get existing CSRF token from cookie
			token := getCSRFToken(r, cfg.CookieName)
			tokenGenerated := false

			// Generate token only if missing
			if token == "" {
				var err error
				token, err = generateCSRFToken(cfg.TokenLength)
				if err != nil {
					http.Error(w, "unable to generate CSRF token", http.StatusInternalServerError)
					return
				}
				tokenGenerated = true
			}

			// Set CSRF cookie only when token was newly generated
			if tokenGenerated {
				setCSRFCookie(w, r, csrfCookieParams{
					Name:   cfg.CookieName,
					Domain: cfg.CookieDomain,
					Token:  token,
				})
			}

			// Add token to request context for template access
			ctx := setCSRFTokenInContext(r.Context(), token)
			r = r.WithContext(ctx)

			// Validate CSRF token for state-changing methods
			if requiresCSRFValidation(r.Method) {
				if !validateCSRFToken(r, token, cfg) {
					http.Error(w, "CSRF token validation failed", http.StatusForbidden)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// requiresCSRFValidation returns true if the HTTP method requires CSRF validation.
// Safe methods (GET, HEAD, OPTIONS, TRACE) are exempt.
func requiresCSRFValidation(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

// getCSRFToken retrieves the CSRF token from the cookie.
func getCSRFToken(r *http.Request, cookieName string) string {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// generateCSRFToken generates a cryptographically secure random CSRF token.
// Returns an error if random generation fails - we fail closed rather than
// falling back to a predictable token.
func generateCSRFToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("csrf token generation failed: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// csrfCookieParams groups optional attributes needed to set the CSRF cookie.
type csrfCookieParams struct {
	Name   string
	Domain string
	Token  string
}

// setCSRFCookie sets the CSRF token cookie.
func setCSRFCookie(w http.ResponseWriter, r *http.Request, params csrfCookieParams) {
	// Check if request is over HTTPS, accounting for proxies
	isSecure := r.TLS != nil || isForwardedHTTPS(r)

	http.SetCookie(w, &http.Cookie{
		Name:     params.Name,
		Value:    params.Token,
		Path:     "/",
		Domain:   params.Domain,
		HttpOnly: false, // Must be readable by JavaScript for HTMX to include it
		Secure:   isSecure,
		SameSite: http.SameSiteStrictMode, // Strict for CSRF tokens
		MaxAge:   3600 * 12,               // 12 hours
	})
}

// isForwardedHTTPS checks if the request was forwarded over HTTPS.
// Handles comma-separated values in X-Forwarded-Proto header.
func isForwardedHTTPS(r *http.Request) bool {
	xfProto := r.Header.Get("X-Forwarded-Proto")
	if xfProto == "" {
		return false
	}

	// Handle comma-separated values (e.g., "https,http")
	for _, proto := range strings.Split(xfProto, ",") {
		if strings.EqualFold(strings.TrimSpace(proto), "https") {
			return true
		}
	}

	return false
}

// validateCSRFToken validates the CSRF token from the request against the cookie value.
// It checks both the X-Csrf-Token header (for HTMX/AJAX) and the csrf_token form field.
// Uses constant-time comparison to prevent timing side-channel attacks.
func validateCSRFToken(r *http.Request, cookieToken string, cfg CSRFConfig) bool {
	if cookieToken == "" {
		return false
	}

	// Check header first (for HTMX/AJAX requests)
	headerToken := r.Header.Get(cfg.HeaderName)
	if headerToken != "" {
		return subtle.ConstantTimeCompare([]byte(headerToken), []byte(cookieToken)) == 1
	}

	// Check form field (for standard form submissions)
	// Only parse form for form-encoded content types
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") ||
		strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseForm(); err != nil {
			return false
		}
		formToken := r.FormValue(cfg.FormFieldName)
		if formToken != "" {
			return subtle.ConstantTimeCompare([]byte(formToken), []byte(cookieToken)) == 1
		}
	}

	return false
}

// csrfTokenKey is an unexported context key type for CSRF token storage.
type csrfTokenKey struct{}

// setCSRFTokenInContext stores the CSRF token in the request context.
func setCSRFTokenInContext(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, csrfTokenKey{}, token)
}

// GetCSRFToken retrieves the CSRF token from the request context.
// This is used by templates to include the token in forms and HTMX requests.
func GetCSRFToken(r *http.Request) string {
	if val := r.Context().Value(csrfTokenKey{}); val != nil {
		if token, ok := val.(string); ok {
			return token
		}
	}
	return ""
}
