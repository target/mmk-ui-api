//revive:disable-next-line:var-naming // legacy package name widely used across the project
package model

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobType_Valid_IncludesAlert(t *testing.T) {
	assert.True(t, JobTypeBrowser.Valid())
	assert.True(t, JobTypeRules.Valid())
	assert.True(t, JobTypeAlert.Valid())
	assert.False(t, JobType("unknown").Valid())
}

func TestJobType_UnmarshalText_Alert(t *testing.T) {
	var jt JobType
	err := jt.UnmarshalText([]byte("alert"))
	require.NoError(t, err)
	assert.Equal(t, JobTypeAlert, jt)
}

func TestCreateJobRequest_Validate_AllowsAlert(t *testing.T) {
	payload := json.RawMessage(`{"sink_id":"abc","payload":{"k":"v"}}`)
	req := &CreateJobRequest{
		Type:       JobTypeAlert,
		Payload:    payload,
		MaxRetries: 0,
	}
	err := req.Validate()
	assert.NoError(t, err)
}

func TestBulkEventRequest_Validate_SourceJobID(t *testing.T) {
	tests := []struct {
		name        string
		sourceJobID *string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil source job id (allowed)",
			sourceJobID: nil,
			expectError: false,
		},
		{
			name:        "empty source job id (allowed)",
			sourceJobID: stringPtr(""),
			expectError: false,
		},
		{
			name:        "valid UUID",
			sourceJobID: stringPtr("550e8400-e29b-41d4-a716-446655440000"),
			expectError: false,
		},
		{
			name:        "invalid UUID format",
			sourceJobID: stringPtr("invalid-uuid"),
			expectError: true,
			errorMsg:    "source job id must be a valid UUID",
		},
		{
			name:        "malformed UUID - missing digit",
			sourceJobID: stringPtr("550e8400-e29b-41d4-a716-44665544000"),
			expectError: true,
			errorMsg:    "source job id must be a valid UUID",
		},
		{
			name:        "malformed UUID - wrong length",
			sourceJobID: stringPtr("550e8400-e29b-41d4-a716"),
			expectError: true,
			errorMsg:    "source job id must be a valid UUID",
		},
		{
			name:        "valid UUID without hyphens",
			sourceJobID: stringPtr("550e8400e29b41d4a716446655440000"),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := BulkEventRequest{
				SessionID:   "session-123",
				SourceJobID: tt.sourceJobID,
				Events: []RawEvent{
					{
						Type:      "test_event",
						Timestamp: time.Now(),
					},
				},
			}

			err := req.Validate(100)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
