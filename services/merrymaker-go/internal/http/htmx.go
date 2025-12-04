package httpx

import (
	"encoding/json"
	"net/http"
	"strings"
)

// IsHTMX reports whether the request was initiated by htmx (Hx-Request: true).
func IsHTMX(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Hx-Request"), "true")
}

// IsBoosted reports whether the request was initiated by hx-boost (Hx-Boosted: true).
func IsBoosted(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Hx-Boosted"), "true")
}

// IsHistoryRestore reports true when htmx is restoring history (Hx-History-Restore-Request: true).
func IsHistoryRestore(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Hx-History-Restore-Request"), "true")
}

// WantsPartial returns true when the handler should return only the main fragment (not full layout).
// Rule: partial for all HTMX requests, including history restores.
func WantsPartial(r *http.Request) bool {
	return IsHTMX(r)
}

// HXTarget returns the id of the target element being updated.
func HXTarget(r *http.Request) string { return r.Header.Get("Hx-Target") }

// HXTrigger returns the id/name of the element that triggered the request.
func HXTrigger(r *http.Request) string { return r.Header.Get("Hx-Trigger") }

// SetHXRedirect instructs htmx to redirect the browser to the given URL.
func SetHXRedirect(w http.ResponseWriter, url string) { w.Header().Set("Hx-Redirect", url) }

// SetHXPushURL pushes the given URL into the browser history for the new content.
func SetHXPushURL(w http.ResponseWriter, url string) { w.Header().Set("Hx-Push-Url", url) }

// SetHXRefresh forces a full page refresh when true.
func SetHXRefresh(w http.ResponseWriter, refresh bool) {
	if refresh {
		w.Header().Set("Hx-Refresh", "true")
		return
	}
	w.Header().Set("Hx-Refresh", "false")
}

// SetHXTrigger triggers a client-side event after swap with optional payload.
// It sets the Hx-Trigger response header as a JSON object: {"<event>": <payload>}.
// If payload is nil, the value true is used for the event.
func SetHXTrigger(w http.ResponseWriter, event string, payload any) {
	var value any = true
	if payload != nil {
		value = payload
	}
	m := map[string]any{event: value}
	b, err := json.Marshal(m)
	if err != nil {
		// Fall back to a boolean trigger if payload cannot be serialized
		w.Header().Set("Hx-Trigger", "{\""+event+"\":true}")
		return
	}
	w.Header().Set("Hx-Trigger", string(b))
}
