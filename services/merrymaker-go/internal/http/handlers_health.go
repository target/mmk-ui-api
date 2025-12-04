package httpx

import (
	"io"
	"net/http"
)

const healthResponse = `{"status":"ok"}`

// healthHandler returns a simple 200 OK status for readiness/liveness checks.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.WriteString(w, healthResponse); err != nil {
		// Nothing more to do if the client connection is gone.
		return
	}
}
