package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

func TestDetermineJobTypeFromTaskName(t *testing.T) {
	tests := []struct {
		name         string
		taskName     string
		expectedType model.JobType
		expectedOk   bool
	}{
		{
			name:         "secret refresh task",
			taskName:     "secret-refresh:abc123",
			expectedType: model.JobTypeSecretRefresh,
			expectedOk:   true,
		},
		{
			name:         "secret refresh task with UUID",
			taskName:     "secret-refresh:550e8400-e29b-41d4-a716-446655440000",
			expectedType: model.JobTypeSecretRefresh,
			expectedOk:   true,
		},
		{
			name:         "site task (no specific type)",
			taskName:     "site:abc123",
			expectedType: "",
			expectedOk:   false,
		},
		{
			name:         "generic task (no specific type)",
			taskName:     "some-other-task",
			expectedType: "",
			expectedOk:   false,
		},
		{
			name:         "empty task name",
			taskName:     "",
			expectedType: "",
			expectedOk:   false,
		},
		{
			name:         "task name too short to be secret-refresh",
			taskName:     "secret-refresh",
			expectedType: "",
			expectedOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotOk := determineJobTypeFromTaskName(tt.taskName)
			assert.Equal(t, tt.expectedOk, gotOk, "ok value mismatch")
			if tt.expectedOk {
				assert.Equal(t, tt.expectedType, gotType, "job type mismatch")
			}
		})
	}
}
