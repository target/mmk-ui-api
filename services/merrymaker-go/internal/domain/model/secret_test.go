//revive:disable-next-line:var-naming // legacy package name widely used across the project
package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateSecretRequest_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     CreateSecretRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request",
			req: CreateSecretRequest{
				Name:  "TEST_SECRET",
				Value: "secret-value-123",
			},
			wantErr: false,
		},
		{
			name: "empty name",
			req: CreateSecretRequest{
				Name:  "",
				Value: "secret-value-123",
			},
			wantErr: true,
			errMsg:  "name is required and cannot be empty",
		},
		{
			name: "whitespace only name",
			req: CreateSecretRequest{
				Name:  "   ",
				Value: "secret-value-123",
			},
			wantErr: true,
			errMsg:  "name is required and cannot be empty",
		},
		{
			name: "name too long",
			req: CreateSecretRequest{
				Name:  strings.Repeat("a", 256), // 256 chars, exceeds 255 limit
				Value: "secret-value-123",
			},
			wantErr: true,
			errMsg:  "name cannot exceed 255 characters",
		},
		{
			name: "name exactly 255 chars",
			req: CreateSecretRequest{
				Name:  strings.Repeat("a", 255), // exactly 255 chars
				Value: "secret-value-123",
			},
			wantErr: false,
		},
		{
			name: "empty value is not allowed in CreateSecretRequest validation",
			req: CreateSecretRequest{
				Name:  "TEST_SECRET",
				Value: "", // Empty value should trigger required error
			},
			wantErr: true,
			errMsg:  "value is required for static secrets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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

func TestUpdateSecretRequest_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     UpdateSecretRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request with all fields",
			req: UpdateSecretRequest{
				Name:  stringPtr("TEST_SECRET"),
				Value: stringPtr("secret-value-123"),
			},
			wantErr: false,
		},
		{
			name: "valid request with only name",
			req: UpdateSecretRequest{
				Name: stringPtr("TEST_SECRET"),
			},
			wantErr: false,
		},

		{
			name: "valid request with only value",
			req: UpdateSecretRequest{
				Value: stringPtr("secret-value-123"),
			},
			wantErr: false,
		},
		{
			name:    "empty request requires at least one field",
			req:     UpdateSecretRequest{},
			wantErr: true,
			errMsg:  "at least one field must be updated",
		},
		{
			name: "empty name",
			req: UpdateSecretRequest{
				Name: stringPtr(""),
			},
			wantErr: true,
			errMsg:  "name cannot be empty",
		},
		{
			name: "whitespace only name",
			req: UpdateSecretRequest{
				Name: stringPtr("   "),
			},
			wantErr: true,
			errMsg:  "name cannot be empty",
		},

		{
			name: "name too long",
			req: UpdateSecretRequest{
				Name: stringPtr(strings.Repeat("a", 256)), // 256 chars, exceeds 255 limit
			},
			wantErr: true,
			errMsg:  "name cannot exceed 255 characters",
		},

		{
			name: "empty value is not allowed when provided",
			req: UpdateSecretRequest{
				Value: stringPtr(""), // Empty value is not allowed when provided
			},
			wantErr: true,
			errMsg:  "value cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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
