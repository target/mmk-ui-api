//revive:disable-next-line:var-naming // legacy package name widely used across the project
package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateHTTPAlertSinkRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateHTTPAlertSinkRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request",
			req: CreateHTTPAlertSinkRequest{
				Name:   "test-alert-sink",
				URI:    "https://example.com/webhook",
				Method: "POST",
			},
			wantErr: false,
		},
		{
			name: "valid request with all fields",
			req: CreateHTTPAlertSinkRequest{
				Name:        "test-alert-sink-full",
				URI:         "https://example.com/webhook",
				Method:      "PUT",
				Body:        httpAlertSinkStringPtr(`{"message": "alert"}`),
				QueryParams: httpAlertSinkStringPtr("token=abc123"),
				Headers:     httpAlertSinkStringPtr("Content-Type: application/json"),
				OkStatus:    httpAlertSinkIntPtr(201),
				Retry:       httpAlertSinkIntPtr(5),
				Secrets:     []string{"API_KEY", "SECRET_TOKEN"},
			},
			wantErr: false,
		},
		{
			name: "empty name",
			req: CreateHTTPAlertSinkRequest{
				Name:   "",
				URI:    "https://example.com/webhook",
				Method: "POST",
			},
			wantErr: true,
			errMsg:  "name is required and cannot be empty",
		},
		{
			name: "name too short",
			req: CreateHTTPAlertSinkRequest{
				Name:   "ab",
				URI:    "https://example.com/webhook",
				Method: "POST",
			},
			wantErr: true,
			errMsg:  "name must be at least 3 characters",
		},
		{
			name: "name too long",
			req: CreateHTTPAlertSinkRequest{
				Name:   string(make([]byte, 513)), // 513 characters
				URI:    "https://example.com/webhook",
				Method: "POST",
			},
			wantErr: true,
			errMsg:  "name cannot exceed 512 characters",
		},
		{
			name: "empty URI",
			req: CreateHTTPAlertSinkRequest{
				Name:   "test-alert-sink",
				URI:    "",
				Method: "POST",
			},
			wantErr: true,
			errMsg:  "uri is required and cannot be empty",
		},
		{
			name: "invalid URI - no scheme",
			req: CreateHTTPAlertSinkRequest{
				Name:   "test-alert-sink",
				URI:    "example.com/webhook",
				Method: "POST",
			},
			wantErr: true,
			errMsg:  "uri must use http or https scheme",
		},
		{
			name: "invalid URI - wrong scheme",
			req: CreateHTTPAlertSinkRequest{
				Name:   "test-alert-sink",
				URI:    "ftp://example.com/webhook",
				Method: "POST",
			},
			wantErr: true,
			errMsg:  "uri must use http or https scheme",
		},
		{
			name: "invalid URI - no host",
			req: CreateHTTPAlertSinkRequest{
				Name:   "test-alert-sink",
				URI:    "https://",
				Method: "POST",
			},
			wantErr: true,
			errMsg:  "uri must have a valid host",
		},
		{
			name: "URI too long",
			req: CreateHTTPAlertSinkRequest{
				Name:   "test-alert-sink",
				URI:    "https://example.com/path/" + strings.Repeat("a", 1000), // > 1024 chars
				Method: "POST",
			},
			wantErr: true,
			errMsg:  "uri cannot exceed 1024 characters",
		},
		{
			name: "empty method",
			req: CreateHTTPAlertSinkRequest{
				Name:   "test-alert-sink",
				URI:    "https://example.com/webhook",
				Method: "",
			},
			wantErr: true,
			errMsg:  "method is required and cannot be empty",
		},
		{
			name: "invalid method",
			req: CreateHTTPAlertSinkRequest{
				Name:   "test-alert-sink",
				URI:    "https://example.com/webhook",
				Method: "INVALID",
			},
			wantErr: true,
			errMsg:  "method must be one of: GET, POST, PUT, PATCH, DELETE",
		},
		{
			name: "invalid ok_status too low",
			req: CreateHTTPAlertSinkRequest{
				Name:     "test-alert-sink",
				URI:      "https://example.com/webhook",
				Method:   "POST",
				OkStatus: httpAlertSinkIntPtr(99),
			},
			wantErr: true,
			errMsg:  "ok_status must be between 100 and 599",
		},
		{
			name: "invalid ok_status too high",
			req: CreateHTTPAlertSinkRequest{
				Name:     "test-alert-sink",
				URI:      "https://example.com/webhook",
				Method:   "POST",
				OkStatus: httpAlertSinkIntPtr(600),
			},
			wantErr: true,
			errMsg:  "ok_status must be between 100 and 599",
		},
		{
			name: "invalid retry negative",
			req: CreateHTTPAlertSinkRequest{
				Name:   "test-alert-sink",
				URI:    "https://example.com/webhook",
				Method: "POST",
				Retry:  httpAlertSinkIntPtr(-1),
			},
			wantErr: true,
			errMsg:  "retry must be non-negative",
		},
		{
			name: "empty secret in slice",
			req: CreateHTTPAlertSinkRequest{
				Name:    "test-alert-sink",
				URI:     "https://example.com/webhook",
				Method:  "POST",
				Secrets: []string{"VALID_SECRET", ""},
			},
			wantErr: true,
			errMsg:  "secrets cannot contain empty or whitespace-only entries",
		},
		{
			name: "duplicate secrets",
			req: CreateHTTPAlertSinkRequest{
				Name:    "test-alert-sink",
				URI:     "https://example.com/webhook",
				Method:  "POST",
				Secrets: []string{"SECRET_1", "SECRET_2", "SECRET_1"},
			},
			wantErr: true,
			errMsg:  "secrets cannot contain duplicate entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUpdateHTTPAlertSinkRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     UpdateHTTPAlertSinkRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid update with name",
			req: UpdateHTTPAlertSinkRequest{
				Name: httpAlertSinkStringPtr("updated-name"),
			},
			wantErr: false,
		},
		{
			name: "valid update with all fields",
			req: UpdateHTTPAlertSinkRequest{
				Name:        httpAlertSinkStringPtr("updated-name"),
				URI:         httpAlertSinkStringPtr("https://updated.example.com/webhook"),
				Method:      httpAlertSinkStringPtr("PATCH"),
				Body:        httpAlertSinkStringPtr(`{"updated": true}`),
				QueryParams: httpAlertSinkStringPtr("updated=true"),
				Headers:     httpAlertSinkStringPtr("X-Updated: true"),
				OkStatus:    httpAlertSinkIntPtr(202),
				Retry:       httpAlertSinkIntPtr(2),
				Secrets:     []string{"NEW_SECRET"},
			},
			wantErr: false,
		},
		{
			name:    "no updates",
			req:     UpdateHTTPAlertSinkRequest{},
			wantErr: true,
			errMsg:  "at least one field must be updated",
		},
		{
			name: "invalid name too short",
			req: UpdateHTTPAlertSinkRequest{
				Name: httpAlertSinkStringPtr("ab"),
			},
			wantErr: true,
			errMsg:  "name must be at least 3 characters",
		},
		{
			name: "invalid URI - wrong scheme",
			req: UpdateHTTPAlertSinkRequest{
				URI: httpAlertSinkStringPtr("ftp://example.com/webhook"),
			},
			wantErr: true,
			errMsg:  "uri must use http or https scheme",
		},
		{
			name: "invalid method",
			req: UpdateHTTPAlertSinkRequest{
				Method: httpAlertSinkStringPtr("INVALID"),
			},
			wantErr: true,
			errMsg:  "method must be one of: GET, POST, PUT, PATCH, DELETE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUpdateHTTPAlertSinkRequest_HasUpdates(t *testing.T) {
	tests := []struct {
		name string
		req  UpdateHTTPAlertSinkRequest
		want bool
	}{
		{
			name: "no updates",
			req:  UpdateHTTPAlertSinkRequest{},
			want: false,
		},
		{
			name: "has name update",
			req: UpdateHTTPAlertSinkRequest{
				Name: httpAlertSinkStringPtr("test"),
			},
			want: true,
		},
		{
			name: "has secrets update",
			req: UpdateHTTPAlertSinkRequest{
				Secrets: []string{"SECRET"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.req.HasUpdates())
		})
	}
}

// Helper functions for creating pointers.
func httpAlertSinkStringPtr(s string) *string {
	return &s
}

func httpAlertSinkIntPtr(i int) *int {
	return &i
}

func TestCreateHTTPAlertSinkRequest_Normalize(t *testing.T) {
	req := CreateHTTPAlertSinkRequest{
		Name:   "  test-alert-sink  ",
		URI:    "  https://example.com/webhook  ",
		Method: "  post  ",
	}

	req.Normalize()

	assert.Equal(t, "test-alert-sink", req.Name)
	assert.Equal(t, "https://example.com/webhook", req.URI)
	assert.Equal(t, "POST", req.Method)
}

func TestUpdateHTTPAlertSinkRequest_Normalize(t *testing.T) {
	req := UpdateHTTPAlertSinkRequest{
		Name:   httpAlertSinkStringPtr("  updated-name  "),
		URI:    httpAlertSinkStringPtr("  https://updated.example.com  "),
		Method: httpAlertSinkStringPtr("  patch  "),
	}

	req.Normalize()

	assert.Equal(t, "updated-name", *req.Name)
	assert.Equal(t, "https://updated.example.com", *req.URI)
	assert.Equal(t, "PATCH", *req.Method)
}
