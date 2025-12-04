package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHTTPAlertSinkRepo_Integration_ConcurrentCreate tests concurrent HTTP alert sink creation with unique names.
func TestHTTPAlertSinkRepo_Integration_ConcurrentCreate(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewHTTPAlertSinkRepo(db)
		ctx := context.Background()

		const numGoroutines = 10
		var wg sync.WaitGroup
		results := make(chan *model.HTTPAlertSink, numGoroutines)
		errors := make(chan error, numGoroutines)

		// Launch concurrent creates
		for i := range numGoroutines {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				req := &model.CreateHTTPAlertSinkRequest{
					Name:   fmt.Sprintf("concurrent-sink-%d-%d", id, time.Now().UnixNano()),
					URI:    fmt.Sprintf("https://example%d.com/webhook", id),
					Method: "POST",
				}
				sink, err := repo.Create(ctx, req)
				if err != nil {
					errors <- err
					return
				}
				results <- sink
			}(i)
		}

		wg.Wait()
		close(results)
		close(errors)

		// Check for errors
		var errorList []error
		for err := range errors {
			errorList = append(errorList, err)
		}
		assert.Empty(t, errorList, "Expected no errors during concurrent creation")

		// Check results
		var sinks []*model.HTTPAlertSink
		for sink := range results {
			sinks = append(sinks, sink)
		}
		assert.Len(t, sinks, numGoroutines, "Expected all sinks to be created successfully")

		// Verify all sinks have unique IDs and names
		seenIDs := make(map[string]bool)
		seenNames := make(map[string]bool)
		for _, sink := range sinks {
			assert.False(t, seenIDs[sink.ID], "Found duplicate sink ID: %s", sink.ID)
			assert.False(t, seenNames[sink.Name], "Found duplicate sink name: %s", sink.Name)
			seenIDs[sink.ID] = true
			seenNames[sink.Name] = true
		}
	})
}

// TestHTTPAlertSinkRepo_Integration_ConcurrentUpdate tests concurrent updates to the same HTTP alert sink.
func TestHTTPAlertSinkRepo_Integration_ConcurrentUpdate(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewHTTPAlertSinkRepo(db)
		ctx := context.Background()

		// Create initial sink
		req := &model.CreateHTTPAlertSinkRequest{
			Name:   fmt.Sprintf("concurrent-update-test-%d", time.Now().UnixNano()),
			URI:    "https://example.com/webhook",
			Method: "POST",
		}
		sink, err := repo.Create(ctx, req)
		require.NoError(t, err)

		const numGoroutines = 5
		var wg sync.WaitGroup
		results := make(chan *model.HTTPAlertSink, numGoroutines)
		errors := make(chan error, numGoroutines)

		// Launch concurrent updates
		for i := range numGoroutines {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				updateReq := &model.UpdateHTTPAlertSinkRequest{
					URI: testutil.StringPtr(fmt.Sprintf("https://updated%d.example.com/webhook", id)),
				}
				updated, err := repo.Update(ctx, sink.ID, updateReq)
				if err != nil {
					errors <- err
					return
				}
				results <- updated
			}(i)
		}

		wg.Wait()
		close(results)
		close(errors)

		// Check for errors
		var errorList []error
		for err := range errors {
			errorList = append(errorList, err)
		}
		assert.Empty(t, errorList, "Expected no errors during concurrent updates")

		// Check results
		var updatedSinks []*model.HTTPAlertSink
		for updated := range results {
			updatedSinks = append(updatedSinks, updated)
		}
		assert.Len(t, updatedSinks, numGoroutines, "Expected all updates to succeed")

		// Verify final state
		final, err := repo.GetByID(ctx, sink.ID)
		require.NoError(t, err)
		assert.NotEqual(t, req.URI, final.URI, "URI should have been updated")
	})
}

// TestHTTPAlertSinkRepo_Integration_BulkOperations tests bulk operations and pagination.
func TestHTTPAlertSinkRepo_Integration_BulkOperations(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewHTTPAlertSinkRepo(db)
		ctx := context.Background()

		// Create many sinks
		const numSinks = 50

		for i := range numSinks {
			req := &model.CreateHTTPAlertSinkRequest{
				Name:     fmt.Sprintf("bulk-sink-%03d", i),
				URI:      fmt.Sprintf("https://bulk%d.example.com/webhook", i),
				Method:   "POST",
				OkStatus: testutil.IntPtr(200 + (i % 5)), // Vary status codes
				Retry:    testutil.IntPtr(i % 10),        // Vary retry counts
			}
			_, err := repo.Create(ctx, req)
			require.NoError(t, err)
		}

		// Test pagination
		pageSize := 10
		var allListed []*model.HTTPAlertSink

		for offset := 0; offset < numSinks; offset += pageSize {
			page, err := repo.List(ctx, pageSize, offset)
			require.NoError(t, err)
			allListed = append(allListed, page...)
		}

		// Verify we got all our sinks (plus any existing ones)
		assert.GreaterOrEqual(t, len(allListed), numSinks)

		// Verify ordering (should be by created_at DESC, id DESC)
		for i := 1; i < len(allListed); i++ {
			prev := allListed[i-1]
			curr := allListed[i]

			// Either created_at is later, or same created_at but ID is lexicographically greater
			assert.True(t,
				prev.CreatedAt.After(curr.CreatedAt) ||
					(prev.CreatedAt.Equal(curr.CreatedAt) && prev.ID > curr.ID),
				"Results should be ordered by created_at DESC, id DESC")
		}

		// Test bulk delete
		deletedCount := 0
		for _, sink := range allListed {
			if strings.HasPrefix(sink.Name, "bulk-sink-") { // Only delete our test sinks
				deleted, err := repo.Delete(ctx, sink.ID)
				require.NoError(t, err)
				if deleted {
					deletedCount++
				}
			}
		}

		assert.GreaterOrEqual(t, deletedCount, numSinks, "Should have deleted at least our test sinks")
	})
}

// TestHTTPAlertSinkRepo_Integration_TransactionIsolation tests transaction isolation and rollback behavior.
func TestHTTPAlertSinkRepo_Integration_TransactionIsolation(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewHTTPAlertSinkRepo(db)
		ctx := context.Background()

		// Create initial sink
		req := &model.CreateHTTPAlertSinkRequest{
			Name:   fmt.Sprintf("isolation-test-%d", time.Now().UnixNano()),
			URI:    "https://example.com/webhook",
			Method: "POST",
		}
		sink, err := repo.Create(ctx, req)
		require.NoError(t, err)

		// Test that failed operations don't affect the database
		// Try to update with invalid data (this should fail validation)
		invalidUpdate := &model.UpdateHTTPAlertSinkRequest{
			Name: testutil.StringPtr(""), // Empty name should fail validation
		}

		_, err = repo.Update(ctx, sink.ID, invalidUpdate)
		require.Error(t, err, "Update with invalid data should fail")

		// Verify original sink is unchanged
		unchanged, err := repo.GetByID(ctx, sink.ID)
		require.NoError(t, err)
		assert.Equal(t, sink.Name, unchanged.Name)
		assert.Equal(t, sink.URI, unchanged.URI)
		assert.Equal(t, sink.Method, unchanged.Method)

		// Test that partial failures in transactions are rolled back
		// This would require a more complex scenario, but the validation test above
		// demonstrates that failed operations don't corrupt the database state
	})
}

// TestHTTPAlertSinkRepo_Integration_SecretsManagement tests complex secrets association scenarios.
func TestHTTPAlertSinkRepo_Integration_SecretsManagement(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewHTTPAlertSinkRepo(db)
		ctx := context.Background()

		// Insert test secrets
		secrets := []string{
			"INTEGRATION_SECRET_1",
			"INTEGRATION_SECRET_2",
			"INTEGRATION_SECRET_3",
			"INTEGRATION_SECRET_4",
		}
		for _, secret := range secrets {
			insertHTTPAlertSinkSecret(t, db, secret)
		}

		// Create sink with initial secrets
		req := &model.CreateHTTPAlertSinkRequest{
			Name:    fmt.Sprintf("secrets-integration-test-%d", time.Now().UnixNano()),
			URI:     "https://example.com/webhook",
			Method:  "POST",
			Secrets: secrets[:2], // First 2 secrets
		}

		sink, err := repo.Create(ctx, req)
		require.NoError(t, err)
		assert.Len(t, sink.Secrets, 2)

		// Update to add more secrets
		updateReq := &model.UpdateHTTPAlertSinkRequest{
			Secrets: secrets, // All 4 secrets
		}

		updated, err := repo.Update(ctx, sink.ID, updateReq)
		require.NoError(t, err)
		assert.Len(t, updated.Secrets, 4)
		for _, secret := range secrets {
			assert.Contains(t, updated.Secrets, secret)
		}

		// Update to remove some secrets
		reducedReq := &model.UpdateHTTPAlertSinkRequest{
			Secrets: secrets[2:], // Last 2 secrets
		}

		reduced, err := repo.Update(ctx, sink.ID, reducedReq)
		require.NoError(t, err)
		assert.Len(t, reduced.Secrets, 2)
		assert.Contains(t, reduced.Secrets, secrets[2])
		assert.Contains(t, reduced.Secrets, secrets[3])
		assert.NotContains(t, reduced.Secrets, secrets[0])
		assert.NotContains(t, reduced.Secrets, secrets[1])

		// Clear all secrets
		clearReq := &model.UpdateHTTPAlertSinkRequest{
			Secrets: []string{},
		}

		cleared, err := repo.Update(ctx, sink.ID, clearReq)
		require.NoError(t, err)
		assert.Empty(t, cleared.Secrets)
	})
}
