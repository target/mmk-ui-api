//revive:disable-next-line:var-naming // legacy package name widely used across the project
package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateSourceRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateSourceRequest
		wantErr string
	}{
		{
			name: "valid request",
			req: CreateSourceRequest{
				Name:    "test-source",
				Value:   "console.log('hello world');",
				Test:    false,
				Secrets: []string{"API_KEY"},
			},
			wantErr: "",
		},
		{
			name: "valid request with no secrets (omitted field)",
			req: CreateSourceRequest{
				Name:  "test-source-no-secrets",
				Value: "console.log('no secrets needed');",
				Test:  false,
				// Secrets field omitted - should be valid
			},
			wantErr: "",
		},
		{
			name: "valid request with test flag",
			req: CreateSourceRequest{
				Name:    "test-source",
				Value:   "console.log('test');",
				Test:    true,
				Secrets: []string{},
			},
			wantErr: "",
		},
		{
			name: "empty name",
			req: CreateSourceRequest{
				Name:  "",
				Value: "console.log('hello');",
			},
			wantErr: "name is required and cannot be empty",
		},
		{
			name: "whitespace only name",
			req: CreateSourceRequest{
				Name:  "   ",
				Value: "console.log('hello');",
			},
			wantErr: "name is required and cannot be empty",
		},
		{
			name: "empty value",
			req: CreateSourceRequest{
				Name:  "test-source",
				Value: "",
			},
			wantErr: "value is required and cannot be empty",
		},
		{
			name: "whitespace only value",
			req: CreateSourceRequest{
				Name:  "test-source",
				Value: "   ",
			},
			wantErr: "value is required and cannot be empty",
		},
		{
			name: "name exactly 255 characters",
			req: CreateSourceRequest{
				Name:  strings.Repeat("a", 255),
				Value: "console.log('hello');",
			},
			wantErr: "",
		},
		{
			name: "name too long (256 characters)",
			req: CreateSourceRequest{
				Name:  strings.Repeat("a", 256),
				Value: "console.log('hello');",
			},
			wantErr: "name cannot exceed 255 characters",
		},
		{
			name: "name with unicode characters within limit",
			req: CreateSourceRequest{
				Name:  strings.Repeat("🚀", 255), // Each emoji is multiple bytes but counts as 1 character
				Value: "console.log('hello');",
			},
			wantErr: "",
		},
		{
			name: "name with unicode characters over limit",
			req: CreateSourceRequest{
				Name:  strings.Repeat("🚀", 256), // 256 unicode characters
				Value: "console.log('hello');",
			},
			wantErr: "name cannot exceed 255 characters",
		},
		{
			name: "empty secret in secrets array",
			req: CreateSourceRequest{
				Name:    "test-source",
				Value:   "console.log('hello');",
				Secrets: []string{"VALID_SECRET", ""},
			},
			wantErr: "secrets cannot contain empty or whitespace-only entries",
		},
		{
			name: "whitespace-only secret in secrets array",
			req: CreateSourceRequest{
				Name:    "test-source",
				Value:   "console.log('hello');",
				Secrets: []string{"VALID_SECRET", "   "},
			},
			wantErr: "secrets cannot contain empty or whitespace-only entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestUpdateSourceRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     UpdateSourceRequest
		wantErr string
	}{
		{
			name:    "no updates provided",
			req:     UpdateSourceRequest{},
			wantErr: "at least one field must be updated",
		},
		{
			name: "valid request with name update",
			req: UpdateSourceRequest{
				Name: new("updated-source"),
			},
			wantErr: "",
		},
		{
			name: "valid request with value update",
			req: UpdateSourceRequest{
				Value: new("console.log('updated');"),
			},
			wantErr: "",
		},
		{
			name: "valid request with test flag update",
			req: UpdateSourceRequest{
				Test: new(true),
			},
			wantErr: "",
		},
		{
			name: "valid request with secrets update",
			req: UpdateSourceRequest{
				Secrets: []string{"NEW_SECRET"},
			},
			wantErr: "",
		},
		{
			name: "empty name",
			req: UpdateSourceRequest{
				Name: new(""),
			},
			wantErr: "name cannot be empty",
		},
		{
			name: "whitespace only name",
			req: UpdateSourceRequest{
				Name: new("   "),
			},
			wantErr: "name cannot be empty",
		},
		{
			name: "empty value",
			req: UpdateSourceRequest{
				Value: new(""),
			},
			wantErr: "value cannot be empty",
		},
		{
			name: "whitespace only value",
			req: UpdateSourceRequest{
				Value: new("   "),
			},
			wantErr: "value cannot be empty",
		},
		{
			name: "name exactly 255 characters",
			req: UpdateSourceRequest{
				Name: new(strings.Repeat("a", 255)),
			},
			wantErr: "",
		},
		{
			name: "name too long (256 characters)",
			req: UpdateSourceRequest{
				Name: new(strings.Repeat("a", 256)),
			},
			wantErr: "name cannot exceed 255 characters",
		},
		{
			name: "name with unicode characters within limit",
			req: UpdateSourceRequest{
				Name: new(strings.Repeat("🚀", 255)),
			},
			wantErr: "",
		},
		{
			name: "name with unicode characters over limit",
			req: UpdateSourceRequest{
				Name: new(strings.Repeat("🚀", 256)),
			},
			wantErr: "name cannot exceed 255 characters",
		},
		{
			name: "empty secret in secrets array",
			req: UpdateSourceRequest{
				Secrets: []string{"VALID_SECRET", ""},
			},
			wantErr: "secrets cannot contain empty or whitespace-only entries",
		},
		{
			name: "whitespace-only secret in secrets array",
			req: UpdateSourceRequest{
				Secrets: []string{"VALID_SECRET", "   "},
			},
			wantErr: "secrets cannot contain empty or whitespace-only entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestUpdateSourceRequest_HasUpdates(t *testing.T) {
	tests := []struct {
		name string
		req  UpdateSourceRequest
		want bool
	}{
		{
			name: "no updates",
			req:  UpdateSourceRequest{},
			want: false,
		},
		{
			name: "name update",
			req: UpdateSourceRequest{
				Name: new("new-name"),
			},
			want: true,
		},
		{
			name: "value update",
			req: UpdateSourceRequest{
				Value: new("new value"),
			},
			want: true,
		},
		{
			name: "test flag update",
			req: UpdateSourceRequest{
				Test: new(true),
			},
			want: true,
		},
		{
			name: "secrets update",
			req: UpdateSourceRequest{
				Secrets: []string{"SECRET"},
			},
			want: true,
		},
		{
			name: "secrets cleared (explicit empty slice)",
			req: UpdateSourceRequest{
				Secrets: []string{},
			},
			want: true,
		},
		{
			name: "multiple updates",
			req: UpdateSourceRequest{
				Name:    new("new-name"),
				Value:   new("new value"),
				Test:    new(false),
				Secrets: []string{"SECRET1", "SECRET2"},
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
//
//go:fix inline
func stringPtr(s string) *string {
	return new(s)
}

//go:fix inline
func boolPtr(b bool) *bool {
	return new(b)
}
