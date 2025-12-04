package httpx

import (
	"net/http"
)

// HTMXResponse provides a fluent API for building HTMX responses.
type HTMXResponse struct {
	w http.ResponseWriter
}

// HTMX creates a new HTMXResponse for fluent response building.
func HTMX(w http.ResponseWriter) *HTMXResponse {
	return &HTMXResponse{w: w}
}

// Redirect instructs htmx to redirect the browser to the given URL.
// It sets the HX-Redirect header and returns a 204 No Content status.
// The handler should return immediately after calling this method to avoid
// accidental writes that would be ignored.
func (h *HTMXResponse) Redirect(url string) {
	SetHXRedirect(h.w, url)
	h.w.WriteHeader(http.StatusNoContent)
}

// Trigger triggers a client-side event after swap with optional payload.
// This method is chainable.
func (h *HTMXResponse) Trigger(event string, payload any) *HTMXResponse {
	SetHXTrigger(h.w, event, payload)
	return h
}

// PushURL pushes the given URL into the browser history for the new content.
// This method is chainable.
func (h *HTMXResponse) PushURL(url string) *HTMXResponse {
	SetHXPushURL(h.w, url)
	return h
}

// Refresh forces a full page refresh.
// It sets the HX-Refresh header and returns a 204 No Content status.
// The handler should return immediately after calling this method to avoid
// accidental writes that would be ignored.
func (h *HTMXResponse) Refresh() {
	SetHXRefresh(h.w, true)
	h.w.WriteHeader(http.StatusNoContent)
}
