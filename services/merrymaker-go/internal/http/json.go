package httpx

import (
	"bytes"
	"encoding/json"
	"net/http"
)

// DecodeJSON decodes JSON from the request body into the destination and handles errors.
// Returns true if successful, false if there was an error (error response already written).
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "invalid_json", Err: err})
		return false
	}

	return true
}

// WriteJSON writes a JSON response with the given status code and data.
func WriteJSON(w http.ResponseWriter, code int, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if _, err := buf.WriteTo(w); err != nil {
		// Response writer errors (e.g., client disconnect) can't be recovered from here.
		return
	}
}

// ErrorParams groups parameters for WriteError to adhere to the ≤3 params guideline.
type ErrorParams struct {
	Code    int
	ErrCode string
	Err     error
}

// WriteError writes a JSON error response using ErrorParams.
func WriteError(w http.ResponseWriter, p ErrorParams) {
	WriteJSON(w, p.Code, map[string]string{"error": p.ErrCode, "message": p.Err.Error()})
}
