package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventRepo_ListWithFilters_CategoryFilter(t *testing.T) {
	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := &EventRepo{DB: db}
		jobRepo := NewJobRepo(db, RepoConfig{})
		ctx := context.Background()

		// Create a test job
		job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
			Type:     model.JobTypeBrowser,
			Payload:  json.RawMessage(`{"url":"https://example.com"}`),
			Priority: 25,
		})
		require.NoError(t, err)
		jobID := job.ID

		// Insert test events with different types
		req := model.BulkEventRequest{
			SessionID:   "550e8400-e29b-41d4-a716-446655440000",
			SourceJobID: &jobID,
			Events: []model.RawEvent{
				{Type: "Network.requestWillBeSent", Data: []byte(`{"url": "https://example.com"}`)},
				{Type: "Runtime.consoleAPICalled", Data: []byte(`{"type": "log", "args": ["Hello"]}`)},
				{Type: "Security.monitoringInitialized", Data: []byte(`{"enabled": true}`)},
				{Type: "Page.goto", Data: []byte(`{"url": "https://test.com"}`)},
				{Type: "Page.click", Data: []byte(`{"selector": "button"}`)},
				{Type: "Runtime.exceptionThrown", Data: []byte(`{"exception": "Error"}`)},
				{Type: "Other.customEvent", Data: []byte(`{"custom": true}`)},
			},
		}

		_, err = repo.BulkInsert(ctx, req, false)
		require.NoError(t, err)

		// Test network category filter
		networkOpts := model.EventListByJobOptions{
			JobID:    jobID,
			Category: evStringPtr("network"),
			Limit:    10,
			Offset:   0,
		}
		networkEvents, err := repo.ListWithFilters(ctx, networkOpts)
		require.NoError(t, err)
		assert.Len(t, networkEvents, 1)
		assert.Equal(t, "Network.requestWillBeSent", networkEvents[0].EventType)

		// Test console category filter
		consoleOpts := model.EventListByJobOptions{
			JobID:    jobID,
			Category: evStringPtr("console"),
			Limit:    10,
			Offset:   0,
		}
		consoleEvents, err := repo.ListWithFilters(ctx, consoleOpts)
		require.NoError(t, err)
		assert.Len(t, consoleEvents, 1)
		assert.Equal(t, "Runtime.consoleAPICalled", consoleEvents[0].EventType)

		// Test security category filter
		securityOpts := model.EventListByJobOptions{
			JobID:    jobID,
			Category: evStringPtr("security"),
			Limit:    10,
			Offset:   0,
		}
		securityEvents, err := repo.ListWithFilters(ctx, securityOpts)
		require.NoError(t, err)
		assert.Len(t, securityEvents, 1)
		assert.Equal(t, "Security.monitoringInitialized", securityEvents[0].EventType)

		// Test action category filter
		actionOpts := model.EventListByJobOptions{
			JobID:    jobID,
			Category: evStringPtr("action"),
			Limit:    10,
			Offset:   0,
		}
		actionEvents, err := repo.ListWithFilters(ctx, actionOpts)
		require.NoError(t, err)
		assert.Len(t, actionEvents, 1)
		assert.Equal(t, "Page.click", actionEvents[0].EventType)

		// Test error category filter
		errorOpts := model.EventListByJobOptions{
			JobID:    jobID,
			Category: evStringPtr("error"),
			Limit:    10,
			Offset:   0,
		}
		errorEvents, err := repo.ListWithFilters(ctx, errorOpts)
		require.NoError(t, err)
		assert.Len(t, errorEvents, 1)
		assert.Equal(t, "Runtime.exceptionThrown", errorEvents[0].EventType)
	})
}

func TestEventRepo_ListWithFilters_SearchQuery(t *testing.T) {
	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := &EventRepo{DB: db}
		jobRepo := NewJobRepo(db, RepoConfig{})
		ctx := context.Background()

		// Create a test job
		job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
			Type:     model.JobTypeBrowser,
			Payload:  json.RawMessage(`{"url":"https://example.com"}`),
			Priority: 25,
		})
		require.NoError(t, err)
		jobID := job.ID

		// Insert test events with different data
		req := model.BulkEventRequest{
			SessionID:   "550e8400-e29b-41d4-a716-446655440001",
			SourceJobID: &jobID,
			Events: []model.RawEvent{
				{Type: "Network.requestWillBeSent", Data: []byte(`{"url": "https://example.com", "method": "GET"}`)},
				{Type: "Runtime.consoleAPICalled", Data: []byte(`{"type": "log", "args": ["Hello World"]}`)},
				{Type: "Page.goto", Data: []byte(`{"url": "https://test.com"}`)},
			},
		}

		_, err = repo.BulkInsert(ctx, req, false)
		require.NoError(t, err)

		// Test search for "example.com"
		searchOpts := model.EventListByJobOptions{
			JobID:       jobID,
			SearchQuery: evStringPtr("example.com"),
			Limit:       10,
			Offset:      0,
		}
		searchEvents, err := repo.ListWithFilters(ctx, searchOpts)
		require.NoError(t, err)
		assert.Len(t, searchEvents, 1)
		assert.Equal(t, "Network.requestWillBeSent", searchEvents[0].EventType)

		// Test search for "Hello"
		helloOpts := model.EventListByJobOptions{
			JobID:       jobID,
			SearchQuery: evStringPtr("Hello"),
			Limit:       10,
			Offset:      0,
		}
		helloEvents, err := repo.ListWithFilters(ctx, helloOpts)
		require.NoError(t, err)
		assert.Len(t, helloEvents, 1)
		assert.Equal(t, "Runtime.consoleAPICalled", helloEvents[0].EventType)

		// Test search for non-existent term
		noMatchOpts := model.EventListByJobOptions{
			JobID:       jobID,
			SearchQuery: evStringPtr("nonexistent"),
			Limit:       10,
			Offset:      0,
		}
		noMatchEvents, err := repo.ListWithFilters(ctx, noMatchOpts)
		require.NoError(t, err)
		assert.Empty(t, noMatchEvents)
	})
}

func TestEventRepo_ListWithFilters_Sorting(t *testing.T) {
	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := &EventRepo{DB: db}
		jobRepo := NewJobRepo(db, RepoConfig{})
		ctx := context.Background()

		// Create a test job
		job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
			Type:     model.JobTypeBrowser,
			Payload:  json.RawMessage(`{"url":"https://example.com"}`),
			Priority: 25,
		})
		require.NoError(t, err)
		jobID := job.ID

		// Insert test events
		req := model.BulkEventRequest{
			SessionID:   "550e8400-e29b-41d4-a716-446655440002",
			SourceJobID: &jobID,
			Events: []model.RawEvent{
				{Type: "ZZZ.lastEvent", Data: []byte(`{"order": 1}`)},
				{Type: "AAA.firstEvent", Data: []byte(`{"order": 2}`)},
				{Type: "MMM.middleEvent", Data: []byte(`{"order": 3}`)},
			},
		}

		_, err = repo.BulkInsert(ctx, req, false)
		require.NoError(t, err)

		// Test sort by event_type ASC
		sortAscOpts := model.EventListByJobOptions{
			JobID:   jobID,
			SortBy:  evStringPtr("event_type"),
			SortDir: evStringPtr("asc"),
			Limit:   10,
			Offset:  0,
		}
		sortAscEvents, err := repo.ListWithFilters(ctx, sortAscOpts)
		require.NoError(t, err)
		assert.Len(t, sortAscEvents, 3)
		assert.Equal(t, "AAA.firstEvent", sortAscEvents[0].EventType)
		assert.Equal(t, "MMM.middleEvent", sortAscEvents[1].EventType)
		assert.Equal(t, "ZZZ.lastEvent", sortAscEvents[2].EventType)

		// Test sort by event_type DESC
		sortDescOpts := model.EventListByJobOptions{
			JobID:   jobID,
			SortBy:  evStringPtr("event_type"),
			SortDir: evStringPtr("desc"),
			Limit:   10,
			Offset:  0,
		}
		sortDescEvents, err := repo.ListWithFilters(ctx, sortDescOpts)
		require.NoError(t, err)
		assert.Len(t, sortDescEvents, 3)
		assert.Equal(t, "ZZZ.lastEvent", sortDescEvents[0].EventType)
		assert.Equal(t, "MMM.middleEvent", sortDescEvents[1].EventType)
		assert.Equal(t, "AAA.firstEvent", sortDescEvents[2].EventType)
	})
}
