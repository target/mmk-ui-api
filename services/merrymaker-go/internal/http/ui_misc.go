package httpx

import (
	"bytes"
	"errors"
	"net/http"
	"net/url"
)

// SignedOut renders a simple signed-out page with a Sign In button.
func (h *UIHandlers) SignedOut(w http.ResponseWriter, r *http.Request) {
	redirect := safeRedirectPath(r.URL.Query().Get("redirect_uri"))
	data := map[string]any{
		"Title":       "Signed out - Merrymaker",
		"RedirectURI": redirect,
	}
	if h.T != nil {
		// Buffer template to avoid partial writes on error
		var buf bytes.Buffer
		if err := h.T.t.ExecuteTemplate(&buf, "signed-out-page", data); err != nil {
			http.Redirect(w, r, "/auth/login?redirect_uri="+url.QueryEscape(redirect), http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(buf.Bytes()); err != nil {
			h.logger().Error("failed to write signed-out response", "error", err)
		}
		return
	}
	// Fallback if no templates are available
	http.Redirect(w, r, "/auth/login?redirect_uri="+url.QueryEscape(redirect), http.StatusSeeOther)
}

// NotFound handles 404 errors with auth-aware behavior.
// For browser requests, it renders an HTML error page.
// For API requests, it returns a JSON error response.
func (h *UIHandlers) NotFound(w http.ResponseWriter, r *http.Request) {
	if IsBrowserRequest(r) {
		h.renderBrowserNotFound(w, r)
	} else {
		h.renderAPINotFound(w, r)
	}
}

// renderBrowserNotFound renders an HTML 404 page with auth-aware content.
func (h *UIHandlers) renderBrowserNotFound(w http.ResponseWriter, r *http.Request) {
	session := GetSessionFromContext(r.Context())
	isAuthenticated := session != nil

	data := map[string]any{
		"Title":           "Page Not Found - Merrymaker",
		"Code":            "404",
		"Message":         "The page you're looking for doesn't exist.",
		"IsAuthenticated": isAuthenticated,
		"ShowLogin":       !isAuthenticated,
		"RedirectURI":     r.URL.RequestURI(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if h.T != nil {
		if err := h.T.RenderError(w, r, data); err != nil {
			// Fallback to plain text if template rendering fails
			http.Error(w, "Page not found", http.StatusNotFound)
		}
	} else {
		// Fallback to plain text if no template renderer available
		http.Error(w, "Page not found", http.StatusNotFound)
	}
}

// renderAPINotFound renders a JSON 404 response.
func (h *UIHandlers) renderAPINotFound(w http.ResponseWriter, _ *http.Request) {
	WriteError(w, ErrorParams{
		Code:    http.StatusNotFound,
		ErrCode: "not_found",
		Err:     errors.New("not found"),
	})
}
