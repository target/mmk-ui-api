package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTMXResponse_Redirect(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{
			name: "redirect to root",
			url:  "/",
		},
		{
			name: "redirect to secrets",
			url:  "/secrets",
		},
		{
			name: "redirect with query params",
			url:  "/sites?page=2&enabled=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			HTMX(w).Redirect(tt.url)

			if got := w.Header().Get("Hx-Redirect"); got != tt.url {
				t.Errorf("Redirect() header = %v, want %v", got, tt.url)
			}
			if w.Code != http.StatusNoContent {
				t.Errorf("Redirect() status = %v, want %v", w.Code, http.StatusNoContent)
			}
		})
	}
}

func TestHTMXResponse_Trigger(t *testing.T) {
	tests := []struct {
		name    string
		event   string
		payload any
		want    string
	}{
		{
			name:    "trigger with nil payload",
			event:   "refresh",
			payload: nil,
			want:    `{"refresh":true}`,
		},
		{
			name:    "trigger with string payload",
			event:   "notify",
			payload: "Success!",
			want:    `{"notify":"Success!"}`,
		},
		{
			name:    "trigger with map payload",
			event:   "update",
			payload: map[string]string{"id": "123", "status": "complete"},
			want:    `{"update":{"id":"123","status":"complete"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			HTMX(w).Trigger(tt.event, tt.payload)

			got := w.Header().Get("Hx-Trigger")
			// Parse both as JSON to compare structure
			var gotJSON, wantJSON map[string]any
			if err := json.Unmarshal([]byte(got), &gotJSON); err != nil {
				t.Fatalf("Failed to parse got JSON: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.want), &wantJSON); err != nil {
				t.Fatalf("Failed to parse want JSON: %v", err)
			}

			// Compare JSON structures
			gotStr, _ := json.Marshal(gotJSON)
			wantStr, _ := json.Marshal(wantJSON)
			if string(gotStr) != string(wantStr) {
				t.Errorf("Trigger() header = %v, want %v", string(gotStr), string(wantStr))
			}
		})
	}
}

func TestHTMXResponse_PushURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{
			name: "push root URL",
			url:  "/",
		},
		{
			name: "push secrets URL",
			url:  "/secrets",
		},
		{
			name: "push URL with query params",
			url:  "/sites?page=2&sort=name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			HTMX(w).PushURL(tt.url)

			if got := w.Header().Get("Hx-Push-Url"); got != tt.url {
				t.Errorf("PushURL() header = %v, want %v", got, tt.url)
			}
		})
	}
}

func TestHTMXResponse_Refresh(t *testing.T) {
	w := httptest.NewRecorder()
	HTMX(w).Refresh()

	if got := w.Header().Get("Hx-Refresh"); got != StrTrue {
		t.Errorf("Refresh() header = %v, want %v", got, StrTrue)
	}
	if w.Code != http.StatusNoContent {
		t.Errorf("Refresh() status = %v, want %v", w.Code, http.StatusNoContent)
	}
}

func TestHTMXResponse_Chaining(t *testing.T) {
	t.Run("Trigger and PushURL chaining", func(t *testing.T) {
		w := httptest.NewRecorder()
		HTMX(w).Trigger("notify", "Saved!").PushURL("/secrets")

		if got := w.Header().Get("Hx-Trigger"); got == "" {
			t.Error("Chaining: Trigger header not set")
		}
		if got := w.Header().Get("Hx-Push-Url"); got != "/secrets" {
			t.Errorf("Chaining: PushURL header = %v, want %v", got, "/secrets")
		}
	})

	t.Run("Multiple Trigger chaining", func(t *testing.T) {
		w := httptest.NewRecorder()
		// Note: Multiple Trigger calls will overwrite the header
		// This test documents the current behavior
		HTMX(w).Trigger("first", "data1").Trigger("second", "data2")

		got := w.Header().Get("Hx-Trigger")
		// The second trigger should overwrite the first
		var gotJSON map[string]any
		if err := json.Unmarshal([]byte(got), &gotJSON); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}
		if _, ok := gotJSON["second"]; !ok {
			t.Error("Chaining: Expected 'second' event in trigger")
		}
	})

	t.Run("PushURL and Trigger chaining", func(t *testing.T) {
		w := httptest.NewRecorder()
		HTMX(w).PushURL("/sites").Trigger("refresh", nil)

		if got := w.Header().Get("Hx-Push-Url"); got != "/sites" {
			t.Errorf("Chaining: PushURL header = %v, want %v", got, "/sites")
		}
		if got := w.Header().Get("Hx-Trigger"); got == "" {
			t.Error("Chaining: Trigger header not set")
		}
	})
}

func TestHTMXResponse_NoStatusCodeForChainableMethods(t *testing.T) {
	t.Run("Trigger does not set status code", func(t *testing.T) {
		w := httptest.NewRecorder()
		HTMX(w).Trigger("event", nil)

		// Status code should be 200 (default) since Trigger doesn't call WriteHeader
		if w.Code != http.StatusOK {
			t.Errorf("Trigger() status = %v, want %v (default)", w.Code, http.StatusOK)
		}
	})

	t.Run("PushURL does not set status code", func(t *testing.T) {
		w := httptest.NewRecorder()
		HTMX(w).PushURL("/test")

		// Status code should be 200 (default) since PushURL doesn't call WriteHeader
		if w.Code != http.StatusOK {
			t.Errorf("PushURL() status = %v, want %v (default)", w.Code, http.StatusOK)
		}
	})
}
