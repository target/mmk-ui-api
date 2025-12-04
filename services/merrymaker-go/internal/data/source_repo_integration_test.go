package data

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
)

// TestSourceRepo_Integration_ConcurrentCreate tests concurrent source creation with unique names.
func TestSourceRepo_Integration_ConcurrentCreate(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSourceRepo(db)
		ctx := context.Background()

		const numWorkers = 10
		results := make(chan *model.Source, numWorkers)
		errors := make(chan error, numWorkers)
		var wg sync.WaitGroup

		// Create sources concurrently with unique names
		for i := range numWorkers {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				req := &model.CreateSourceRequest{
					Name:    fmt.Sprintf("concurrent-source-%d", id),
					Value:   fmt.Sprintf("console.log('worker %d');", id),
					Test:    id%2 == 0, // Alternate test flag
					Secrets: []string{fmt.Sprintf("SECRET_%d", id)},
				}
				source, err := repo.Create(ctx, req)
				if err != nil {
					errors <- err
				} else {
					results <- source
				}
			}(i)
		}

		wg.Wait()
		close(results)
		close(errors)

		// All should succeed
		var sources []*model.Source
		for source := range results {
			sources = append(sources, source)
		}

		var errs []error
		for err := range errors {
			errs = append(errs, err)
		}

		assert.Len(t, sources, numWorkers, "All workers should succeed")
		assert.Empty(t, errs, "No errors should occur")

		// Verify all sources have unique IDs and names
		seenIDs := make(map[string]bool)
		seenNames := make(map[string]bool)
		for _, source := range sources {
			assert.False(t, seenIDs[source.ID], "Source ID should be unique: %s", source.ID)
			assert.False(t, seenNames[source.Name], "Source name should be unique: %s", source.Name)
			seenIDs[source.ID] = true
			seenNames[source.Name] = true
		}
	})
}

// TestSourceRepo_Integration_ConcurrentCreateDuplicateName tests concurrent creation with duplicate names.
func TestSourceRepo_Integration_ConcurrentCreateDuplicateName(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSourceRepo(db)
		ctx := context.Background()

		const numWorkers = 5
		duplicateName := fmt.Sprintf("duplicate-test-%d", time.Now().UnixNano())
		results := make(chan *model.Source, numWorkers)
		errors := make(chan error, numWorkers)
		var wg sync.WaitGroup

		// Try to create sources with the same name concurrently
		for i := range numWorkers {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				req := &model.CreateSourceRequest{
					Name:  duplicateName,
					Value: fmt.Sprintf("console.log('worker %d');", id),
				}
				source, err := repo.Create(ctx, req)
				if err != nil {
					errors <- err
				} else {
					results <- source
				}
			}(i)
		}

		wg.Wait()
		close(results)
		close(errors)

		// Exactly one should succeed, others should fail with ErrSourceNameExists
		var sources []*model.Source
		for source := range results {
			sources = append(sources, source)
		}

		var errs []error
		for err := range errors {
			errs = append(errs, err)
		}

		assert.Len(t, sources, 1, "Exactly one worker should succeed")
		assert.Len(t, errs, numWorkers-1, "All other workers should fail")

		// All errors should be ErrSourceNameExists
		for _, err := range errs {
			require.ErrorIs(t, err, ErrSourceNameExists)
		}

		// The successful source should have the duplicate name
		assert.Equal(t, duplicateName, sources[0].Name)
	})
}

// TestSourceRepo_Integration_ConcurrentReadWrite tests concurrent read/write operations.
//

func TestSourceRepo_Integration_ConcurrentReadWrite(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSourceRepo(db)
		ctx := context.Background()

		// Create initial source
		req := &model.CreateSourceRequest{
			Name:    fmt.Sprintf("rw-test-%d", time.Now().UnixNano()),
			Value:   "console.log('initial');",
			Secrets: []string{"INITIAL_SECRET"},
		}
		source, err := repo.Create(ctx, req)
		require.NoError(t, err)

		const numReaders = 5
		const numWriters = 3
		var wg sync.WaitGroup
		readResults := make(chan *model.Source, numReaders*10) // Multiple reads per reader
		writeResults := make(chan *model.Source, numWriters)
		errors := make(chan error, numReaders+numWriters)

		// Start readers
		for i := range numReaders {
			wg.Add(1)
			go func(readerID int) {
				defer wg.Done()
				// Each reader performs multiple reads
				for j := range 10 {
					var found *model.Source
					var err error

					// Alternate between GetByID and GetByName
					if j%2 == 0 {
						found, err = repo.GetByID(ctx, source.ID)
					} else {
						found, err = repo.GetByName(ctx, source.Name)
					}

					if err != nil {
						errors <- fmt.Errorf("reader %d iteration %d: %w", readerID, j, err)
						return
					}
					readResults <- found

					// Small delay to increase chance of interleaving
					time.Sleep(time.Millisecond)
				}
			}(i)
		}

		// Start writers
		for i := range numWriters {
			wg.Add(1)
			go func(writerID int) {
				defer wg.Done()
				updateReq := model.UpdateSourceRequest{
					Value: testutil.StringPtr(fmt.Sprintf("console.log('updated by writer %d');", writerID)),
				}
				updated, err := repo.Update(ctx, source.ID, updateReq)
				if err != nil {
					errors <- fmt.Errorf("writer %d: %w", writerID, err)
					return
				}
				writeResults <- updated
			}(i)
		}

		wg.Wait()
		close(readResults)
		close(writeResults)
		close(errors)

		// Check for errors
		var errs []error
		for err := range errors {
			errs = append(errs, err)
		}
		assert.Empty(t, errs, "No errors should occur during concurrent operations")

		// Verify read results
		var reads []*model.Source
		for read := range readResults {
			reads = append(reads, read)
		}
		assert.Len(t, reads, numReaders*10, "All reads should succeed")

		// All reads should return the same ID and name
		for _, read := range reads {
			assert.Equal(t, source.ID, read.ID)
			assert.Equal(t, source.Name, read.Name)
		}

		// Verify write results
		var writes []*model.Source
		for write := range writeResults {
			writes = append(writes, write)
		}
		assert.Len(t, writes, numWriters, "All writes should succeed")

		// All writes should have the same ID and name but potentially different values
		for _, write := range writes {
			assert.Equal(t, source.ID, write.ID)
			assert.Equal(t, source.Name, write.Name)
		}
	})
}

// TestSourceRepo_Integration_BulkOperations tests bulk operations and pagination.
func TestSourceRepo_Integration_BulkOperations(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSourceRepo(db)
		ctx := context.Background()

		// Create many sources
		const numSources = 50

		for i := range numSources {
			req := &model.CreateSourceRequest{
				Name:    fmt.Sprintf("bulk-source-%03d", i),
				Value:   fmt.Sprintf("console.log('bulk source %d');", i),
				Test:    i%3 == 0, // Every third source is a test
				Secrets: []string{fmt.Sprintf("BULK_SECRET_%d", i)},
			}
			_, err := repo.Create(ctx, req)
			require.NoError(t, err)
		}

		// Test pagination
		pageSize := 10
		var allListed []*model.Source

		for offset := 0; offset < numSources; offset += pageSize {
			page, err := repo.List(ctx, pageSize, offset)
			require.NoError(t, err)
			allListed = append(allListed, page...)
		}

		// Should have retrieved all sources
		assert.Len(t, allListed, numSources)

		// Verify ordering (should be by created_at DESC, id DESC)
		for i := 1; i < len(allListed); i++ {
			prev := allListed[i-1]
			curr := allListed[i]

			// Either created_at is later, or same created_at but ID is lexicographically greater
			assert.True(t,
				prev.CreatedAt.After(curr.CreatedAt) ||
					(prev.CreatedAt.Equal(curr.CreatedAt) && prev.ID > curr.ID),
				"Sources should be ordered by created_at DESC, id DESC")
		}

		// Test large page size
		allAtOnce, err := repo.List(ctx, 100, 0)
		require.NoError(t, err)
		assert.Len(t, allAtOnce, numSources)

		// Test beyond available data
		empty, err := repo.List(ctx, 10, numSources+10)
		require.NoError(t, err)
		assert.Empty(t, empty)
	})
}

// TestSourceRepo_Integration_TransactionIsolation tests transaction isolation and rollback behavior.
func TestSourceRepo_Integration_TransactionIsolation(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSourceRepo(db)
		ctx := context.Background()

		// Create initial source
		req := &model.CreateSourceRequest{
			Name:  fmt.Sprintf("isolation-test-%d", time.Now().UnixNano()),
			Value: "console.log('initial');",
		}
		source, err := repo.Create(ctx, req)
		require.NoError(t, err)

		// Test that failed operations don't affect the database
		// Try to update with invalid data (this should fail validation)
		invalidUpdate := model.UpdateSourceRequest{
			Name: testutil.StringPtr(""), // Empty name should fail validation
		}

		updated, err := repo.Update(ctx, source.ID, invalidUpdate)
		require.Error(t, err)
		assert.Nil(t, updated)

		// Verify original source is unchanged
		unchanged, err := repo.GetByID(ctx, source.ID)
		require.NoError(t, err)
		assert.Equal(t, source.Name, unchanged.Name)
		assert.Equal(t, source.Value, unchanged.Value)
		assert.Equal(t, source.CreatedAt.Unix(), unchanged.CreatedAt.Unix())
	})
}

// TestSourceRepo_Integration_DatabaseConstraints tests database-level constraints.
func TestSourceRepo_Integration_DatabaseConstraints(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSourceRepo(db)
		ctx := context.Background()

		// Test unique constraint on name
		baseName := fmt.Sprintf("constraint-test-%d", time.Now().UnixNano())

		req1 := &model.CreateSourceRequest{
			Name:  baseName,
			Value: "console.log('first');",
		}
		source1, err := repo.Create(ctx, req1)
		require.NoError(t, err)

		// Try to create another with same name
		req2 := &model.CreateSourceRequest{
			Name:  baseName,
			Value: "console.log('second');",
		}
		source2, err := repo.Create(ctx, req2)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrSourceNameExists)
		assert.Nil(t, source2)

		// Verify first source still exists and is unchanged
		found, err := repo.GetByID(ctx, source1.ID)
		require.NoError(t, err)
		assert.Equal(t, source1.Name, found.Name)
		assert.Equal(t, source1.Value, found.Value)

		// Test that updating to a duplicate name also fails
		req3 := &model.CreateSourceRequest{
			Name:  baseName + "-different",
			Value: "console.log('third');",
		}
		source3, err := repo.Create(ctx, req3)
		require.NoError(t, err)

		// Try to update source3 to have the same name as source1
		updateReq := model.UpdateSourceRequest{
			Name: &baseName,
		}
		updated, err := repo.Update(ctx, source3.ID, updateReq)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrSourceNameExists)
		assert.Nil(t, updated)

		// Verify source3 is unchanged
		unchanged, err := repo.GetByID(ctx, source3.ID)
		require.NoError(t, err)
		assert.Equal(t, source3.Name, unchanged.Name)
	})
}

// TestSourceRepo_Integration_ConcurrentUpdateDelete tests concurrent update and delete operations.
//

func TestSourceRepo_Integration_ConcurrentUpdateDelete(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSourceRepo(db)
		ctx := context.Background()

		// Create source to be updated/deleted
		req := &model.CreateSourceRequest{
			Name:  fmt.Sprintf("update-delete-test-%d", time.Now().UnixNano()),
			Value: "console.log('original');",
		}
		source, err := repo.Create(ctx, req)
		require.NoError(t, err)

		const numUpdaters = 3
		const numDeleters = 2
		var wg sync.WaitGroup

		updateResults := make(chan *model.Source, numUpdaters)
		deleteResults := make(chan bool, numDeleters)
		errors := make(chan error, numUpdaters+numDeleters)

		// Start updaters
		for i := range numUpdaters {
			wg.Add(1)
			go func(updaterID int) {
				defer wg.Done()
				updateReq := model.UpdateSourceRequest{
					Value: testutil.StringPtr(fmt.Sprintf("console.log('updated by %d');", updaterID)),
				}
				updated, err := repo.Update(ctx, source.ID, updateReq)
				if err != nil {
					errors <- fmt.Errorf("updater %d: %w", updaterID, err)
					return
				}
				updateResults <- updated
			}(i)
		}

		// Start deleters (with slight delay to let some updates happen first)
		for i := range numDeleters {
			wg.Add(1)
			go func(deleterID int) {
				defer wg.Done()
				time.Sleep(10 * time.Millisecond) // Small delay
				deleted, err := repo.Delete(ctx, source.ID)
				if err != nil {
					errors <- fmt.Errorf("deleter %d: %w", deleterID, err)
					return
				}
				deleteResults <- deleted
			}(i)
		}

		wg.Wait()
		close(updateResults)
		close(deleteResults)
		close(errors)

		// Collect results: drain update results to ensure goroutines complete
		drainCount := 0
		for range updateResults {
			drainCount++
		}
		_ = drainCount

		var deletes []bool
		for deleted := range deleteResults {
			deletes = append(deletes, deleted)
		}

		var errs []error
		for err := range errors {
			errs = append(errs, err)
		}

		// Some operations should succeed, some might fail due to the source being deleted
		// The key is that we shouldn't have any unexpected errors
		for _, err := range errs {
			// Only acceptable error is ErrSourceNotFound (if delete happened first)
			require.ErrorIs(t, err, ErrSourceNotFound, "Unexpected error: %v", err)
		}

		// If any delete succeeded, the source should no longer exist
		anyDeleteSucceeded := false
		for _, deleted := range deletes {
			if deleted {
				anyDeleteSucceeded = true
				break
			}
		}

		if anyDeleteSucceeded {
			// Source should not exist
			notFound, err := repo.GetByID(ctx, source.ID)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrSourceNotFound)
			assert.Nil(t, notFound)
		}
	})
}

// TestSourceRepo_Integration_StressTest performs stress testing with many concurrent operations.
//

func TestSourceRepo_Integration_StressTest(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSourceRepo(db)
		ctx := context.Background()

		// Create initial sources for stress testing
		const initialSources = 20
		var sources []*model.Source

		for i := range initialSources {
			req := &model.CreateSourceRequest{
				Name:    fmt.Sprintf("stress-source-%03d-%d", i, time.Now().UnixNano()),
				Value:   fmt.Sprintf("console.log('stress test %d');", i),
				Test:    i%2 == 0,
				Secrets: []string{fmt.Sprintf("STRESS_SECRET_%d", i)},
			}
			source, err := repo.Create(ctx, req)
			require.NoError(t, err)
			sources = append(sources, source)
		}

		// Perform many concurrent operations
		const numWorkers = 20
		const operationsPerWorker = 10
		var wg sync.WaitGroup

		results := make(chan string, numWorkers*operationsPerWorker)
		errors := make(chan error, numWorkers*operationsPerWorker)

		for workerID := range numWorkers {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				for op := range operationsPerWorker {
					// Randomly choose an operation
					switch op % 5 {
					case 0: // Create new source
						req := &model.CreateSourceRequest{
							Name:  fmt.Sprintf("stress-new-%d-%d-%d", id, op, time.Now().UnixNano()),
							Value: fmt.Sprintf("console.log('worker %d op %d');", id, op),
						}
						_, err := repo.Create(ctx, req)
						if err != nil {
							errors <- fmt.Errorf("worker %d create: %w", id, err)
						} else {
							results <- fmt.Sprintf("worker %d created source", id)
						}

					case 1: // Get by ID
						if len(sources) > 0 {
							source := sources[op%len(sources)]
							_, err := repo.GetByID(ctx, source.ID)
							if err != nil {
								errors <- fmt.Errorf("worker %d getByID: %w", id, err)
							} else {
								results <- fmt.Sprintf("worker %d got source by ID", id)
							}
						}

					case 2: // Get by Name
						if len(sources) > 0 {
							source := sources[op%len(sources)]
							_, err := repo.GetByName(ctx, source.Name)
							if err != nil {
								errors <- fmt.Errorf("worker %d getByName: %w", id, err)
							} else {
								results <- fmt.Sprintf("worker %d got source by name", id)
							}
						}

					case 3: // Update
						if len(sources) > 0 {
							source := sources[op%len(sources)]
							updateReq := model.UpdateSourceRequest{
								Value: testutil.StringPtr(
									fmt.Sprintf("console.log('updated by worker %d op %d');", id, op),
								),
							}
							_, err := repo.Update(ctx, source.ID, updateReq)
							if err != nil {
								errors <- fmt.Errorf("worker %d update: %w", id, err)
							} else {
								results <- fmt.Sprintf("worker %d updated source", id)
							}
						}

					case 4: // List
						_, err := repo.List(ctx, 5, op*2)
						if err != nil {
							errors <- fmt.Errorf("worker %d list: %w", id, err)
						} else {
							results <- fmt.Sprintf("worker %d listed sources", id)
						}
					}

					// Small random delay to increase concurrency
					time.Sleep(time.Duration(op%3) * time.Millisecond)
				}
			}(workerID)
		}

		wg.Wait()
		close(results)
		close(errors)

		// Collect results
		var successCount int
		for range results {
			successCount++
		}

		var errs []error
		for err := range errors {
			errs = append(errs, err)
		}

		// Most operations should succeed
		totalOperations := numWorkers * operationsPerWorker
		successRate := float64(successCount) / float64(totalOperations)

		t.Logf("Stress test results: %d/%d operations succeeded (%.1f%%)",
			successCount, totalOperations, successRate*100)

		// We expect a high success rate, but some operations might fail due to concurrency
		// (e.g., trying to update a source that was deleted by another worker)
		assert.Greater(t, successRate, 0.8, "Success rate should be > 80%%")

		// Log any errors for debugging
		if len(errs) > 0 {
			t.Logf("Errors encountered (%d total):", len(errs))
			for i, err := range errs {
				if i < 10 { // Log first 10 errors
					t.Logf("  %v", err)
				}
			}
			if len(errs) > 10 {
				t.Logf("  ... and %d more errors", len(errs)-10)
			}
		}

		// Verify database is still in a consistent state
		allSources, err := repo.List(ctx, 1000, 0)
		require.NoError(t, err)

		// Should have at least the initial sources (some might have been deleted)
		t.Logf("Final source count: %d", len(allSources))
		assert.NotEmpty(t, allSources, "Should have at least some sources remaining")

		// All remaining sources should be valid
		for _, source := range allSources {
			assert.NotEmpty(t, source.ID)
			assert.NotEmpty(t, source.Name)
			assert.NotEmpty(t, source.Value)
			assert.False(t, source.CreatedAt.IsZero())
		}
	})
}
