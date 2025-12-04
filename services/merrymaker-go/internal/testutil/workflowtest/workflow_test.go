package workflowtest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestCreateSimpleEventBatch tests the event batch creation utility.
func TestCreateSimpleEventBatch(t *testing.T) {
	batchID := "test-batch-1"
	sessionID := ""
	jobID := "job-123"

	batch := CreateSimpleEventBatch(batchID, sessionID, jobID)

	assert.Equal(t, batchID, batch.BatchID)
	assert.NotEmpty(t, batch.SessionID) // Should generate a UUID
	assert.Len(t, batch.Events, 1)
	assert.Equal(t, "event-1", batch.Events[0].ID)
	assert.Equal(t, "Network.requestWillBeSent", batch.Events[0].Method)
	assert.Equal(t, jobID, batch.BatchMetadata.JobID)

	// Test with provided sessionID
	providedSessionID := "550e8400-e29b-41d4-a716-446655440000"
	batch2 := CreateSimpleEventBatch("batch-2", providedSessionID, jobID)
	assert.Equal(t, providedSessionID, batch2.SessionID)
}

// TestWorkflowTestOptions tests the option builders.
func TestWorkflowTestOptions(t *testing.T) {
	// Test default options
	opts := DefaultWorkflowOptions()
	assert.False(t, opts.EnableRedis)
	assert.Equal(t, 30*time.Second, opts.JobLease)
	assert.Equal(t, 1000, opts.EventMaxBatch)

	// Test Redis options
	redisOpts := RedisWorkflowOptions()
	assert.True(t, redisOpts.EnableRedis)
	assert.Equal(t, 30*time.Second, redisOpts.JobLease)
	assert.Equal(t, 1000, redisOpts.EventMaxBatch)
}
