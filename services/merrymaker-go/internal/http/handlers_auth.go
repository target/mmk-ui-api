package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	"github.com/target/mmk-ui-api/internal/service"
)

// AuthServiceInterface defines the interface for auth service operations.
type AuthServiceInterface interface {
	BeginLogin(ctx context.Context, redirectURL string) (*service.BeginLoginResult, error)
	CompleteLogin(ctx context.Context, input service.CompleteLoginInput) (*service.CompleteLoginResult, error)
	GetSession(ctx context.Context, sessionID string) (*domainauth.Session, error)
	Logout(ctx context.Context, sessionID string) error
}

// AuthHandlers provides HTTP handlers for authentication operations.
type AuthHandlers struct {
	Svc          AuthServiceInterface
	CookieDomain string
	Logger       *slog.Logger
}

func (h *AuthHandlers) logger() *slog.Logger {
	if h != nil && h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

// Login handles the login initiation endpoint.
// GET /auth/login?redirect_uri=<optional_redirect>.
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	// Get the redirect URI from query params, default to root
	redirectURI := r.URL.Query().Get("redirect_uri")
	if redirectURI == "" {
		redirectURI = "/"
	}

	// Validate redirect URI: allow only relative paths (no scheme/host), must start with "/"
	u, err := url.Parse(redirectURI)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") {
		redirectURI = "/"
	}

	// Begin login flow
	result, err := h.Svc.BeginLogin(r.Context(), redirectURI)
	if err != nil {
		WriteError(w, ErrorParams{
			Code:    http.StatusInternalServerError,
			ErrCode: "login_failed",
			Err:     err,
		})
		return
	}

	// Store state, nonce, and the original redirect URI in secure cookies
	h.setOAuthCookies(w, r, oauthCookieParams{State: result.State, Nonce: result.Nonce, RedirectURI: redirectURI})

	// Redirect to the identity provider
	http.Redirect(w, r, result.AuthURL, http.StatusFound)
}

// Callback handles the OAuth callback endpoint.
// GET /auth/callback?code=<code>&state=<state>.
func (h *AuthHandlers) Callback(w http.ResponseWriter, r *http.Request) {
	// Read and validate basic params
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" {
		WriteError(w, ErrorParams{
			Code:    http.StatusBadRequest,
			ErrCode: "missing_code",
			Err:     errors.New("authorization code is required"),
		})
		return
	}
	if state == "" {
		WriteError(w, ErrorParams{
			Code:    http.StatusBadRequest,
			ErrCode: "missing_state",
			Err:     errors.New("state parameter is required"),
		})
		return
	}

	// Verify state and read nonce
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != state {
		WriteError(w, ErrorParams{
			Code:    http.StatusBadRequest,
			ErrCode: "invalid_state",
			Err:     errors.New("invalid or missing state parameter"),
		})
		return
	}
	nonceCookie, err := r.Cookie("oauth_nonce")
	if err != nil {
		WriteError(w, ErrorParams{
			Code:    http.StatusBadRequest,
			ErrCode: "missing_nonce",
			Err:     errors.New("missing nonce parameter"),
		})
		return
	}

	// Complete the login flow
	result, err := h.Svc.CompleteLogin(r.Context(), service.CompleteLoginInput{
		Code:  code,
		State: state,
		Nonce: nonceCookie.Value,
	})
	if err != nil {
		WriteError(w, ErrorParams{
			Code:    http.StatusInternalServerError,
			ErrCode: "login_completion_failed",
			Err:     err,
		})
		return
	}

	// Set session cookie and clear temporary OAuth cookies
	h.setSessionCookie(w, r, result.Session)
	h.clearCookie(w, r, "oauth_state")
	h.clearCookie(w, r, "oauth_nonce")

	// Redirect to the original destination
	redirectURI := h.getPostLoginRedirect(w, r)
	http.Redirect(w, r, redirectURI, http.StatusFound)
}

// Logout handles the logout endpoint.
// POST /auth/logout.
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	// Get session ID from cookie and invalidate server-side session if present
	if sessionCookie, err := r.Cookie("session_id"); err == nil {
		if logoutErr := h.Svc.Logout(r.Context(), sessionCookie.Value); logoutErr != nil {
			h.logger().WarnContext(r.Context(), "logout failed", "error", logoutErr)
		}
	}

	// Clear session cookie on the client
	h.clearCookie(w, r, "session_id")

	// Determine desired post-login destination (where user wanted to be after re-auth)
	redirectURI := r.FormValue("redirect_uri")
	if redirectURI == "" {
		redirectURI = r.URL.Query().Get("redirect_uri")
	}
	if redirectURI == "" {
		redirectURI = "/"
	}
	// Enforce safe, relative redirect only (defense-in-depth)
	redirectURI = safeRedirectPath(redirectURI)

	// Build signed-out URL using url.Values to avoid edge cases
	u := url.URL{Path: "/auth/signed-out"}
	q := url.Values{}
	q.Set("redirect_uri", redirectURI)
	u.RawQuery = q.Encode()
	signedOutURL := u.String()

	// AJAX/HTMX requests get a JSON payload; regular requests redirect
	isAJAX := strings.Contains(r.Header.Get("Accept"), "application/json") ||
		strings.EqualFold(r.Header.Get("Hx-Request"), "true") ||
		strings.EqualFold(r.Header.Get("X-Requested-With"), "XMLHttpRequest")
	if isAJAX {
		WriteJSON(w, http.StatusOK, map[string]string{
			"status":      "success",
			"redirect_to": signedOutURL,
		})
		return
	}

	http.Redirect(w, r, signedOutURL, http.StatusFound)
}

// Status returns the current authentication status.
// GET /auth/status.
func (h *AuthHandlers) Status(w http.ResponseWriter, r *http.Request) {
	// Get session ID from cookie
	sessionCookie, err := r.Cookie("session_id")
	if err != nil {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"authenticated": false,
		})
		return
	}

	// Get session from auth service
	session, err := h.Svc.GetSession(r.Context(), sessionCookie.Value)
	if err != nil {
		// Session is invalid or expired, clear the cookie
		h.clearCookie(w, r, "session_id")
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"authenticated": false,
		})
		return
	}

	// Return session information
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"user": map[string]interface{}{
			"id":         session.UserID,
			"first_name": session.FirstName,
			"last_name":  session.LastName,
			"email":      session.Email,
			"role":       session.Role,
		},
		"expires_at": session.ExpiresAt,
	})
}

// clearCookie clears a cookie by setting it to expire immediately.
// It mirrors key attributes (Secure, Path, Domain, SameSite) used when setting cookies
// to maximize compatibility across browsers during deletion.
func (h *AuthHandlers) clearCookie(w http.ResponseWriter, r *http.Request, name string) {
	isSecure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	cd := h.CookieDomain
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		Domain:   cd,
		HttpOnly: true,
		Secure:   isSecure,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
		SameSite: http.SameSiteLaxMode,
	})
}

// oauthCookieParams groups values needed to set OAuth cookies (≤3 params rule).
type oauthCookieParams struct {
	State       string
	Nonce       string
	RedirectURI string
}

// setOAuthCookies stores OAuth state, nonce, and the post-login redirect in secure cookies.
func (h *AuthHandlers) setOAuthCookies(w http.ResponseWriter, r *http.Request, p oauthCookieParams) {
	isSecure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	cd := h.CookieDomain

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    p.State,
		Path:     "/",
		Domain:   cd,
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600, // 10 minutes
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_nonce",
		Value:    p.Nonce,
		Path:     "/",
		Domain:   cd,
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600, // 10 minutes
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "post_login_redirect",
		Value:    p.RedirectURI,
		Path:     "/",
		Domain:   cd,
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600, // 10 minutes
	})
}

// setSessionCookie writes the session cookie based on the session's expiry.
func (h *AuthHandlers) setSessionCookie(w http.ResponseWriter, r *http.Request, s domainauth.Session) {
	isSecure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	cd := h.CookieDomain
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    s.ID,
		Path:     "/",
		Domain:   cd,
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(time.Until(s.ExpiresAt).Seconds()),
	})
}

// getPostLoginRedirect returns the post-login redirect URL and clears the cookie.
func (h *AuthHandlers) getPostLoginRedirect(w http.ResponseWriter, r *http.Request) string {
	redirectURI := "/"
	if redirectCookie, err := r.Cookie("post_login_redirect"); err == nil {
		candidate := redirectCookie.Value
		// Defensive re-validation: allow only relative paths
		u, parseErr := url.Parse(candidate)
		if parseErr == nil && !u.IsAbs() && u.Host == "" && strings.HasPrefix(u.Path, "/") {
			redirectURI = candidate
		}
		h.clearCookie(w, r, "post_login_redirect")
	}
	return redirectURI
}

// safeRedirectPath ensures the provided redirect is a same-origin relative path
// starting with "/" and not an absolute URL. Returns "/" when invalid.
func safeRedirectPath(candidate string) string {
	if candidate == "" {
		return "/"
	}
	u, err := url.Parse(candidate)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") {
		return "/"
	}
	return candidate
}
