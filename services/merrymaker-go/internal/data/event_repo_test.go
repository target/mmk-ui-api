package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testSessionID1 = "550e8400-e29b-41d4-a716-446655440001"
	testSessionID2 = "550e8400-e29b-41d4-a716-446655440002"
)

func TestEventRepo_BulkInsert_Success_WithSourceJobID(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		eventRepo := &EventRepo{DB: db}
		jobRepo := NewJobRepo(db, RepoConfig{})

		// Create a source job to reference
		job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
			Type:     model.JobTypeBrowser,
			Payload:  json.RawMessage(`{"url":"https://example.com"}`),
			Priority: 25,
		})
		require.NoError(t, err)

		sessionID := "550e8400-e29b-41d4-a716-446655440000"
		srcID := job.ID

		req := model.BulkEventRequest{
			SessionID:   sessionID,
			SourceJobID: &srcID,
			Events: []model.RawEvent{
				{
					Type:       "domain_seen",
					Data:       json.RawMessage(`{"domain":"example.com"}`),
					StorageKey: evStringPtr("s3://bucket/key1"),
					Priority:   intPtr(42),
				},
				{
					Type:       "file_seen",
					Data:       json.RawMessage(`{"sha256":"abc"}`),
					StorageKey: nil, // no storage key
					Priority:   nil, // defaults to 0
				},
			},
		}

		created, err := eventRepo.BulkInsert(ctx, req, true)
		require.NoError(t, err)
		assert.Equal(t, 2, created)

		// Verify rows
		rows, err := db.Query(`
            SELECT session_id::text, source_job_id::text, event_type, event_data::text, storage_key, priority, should_process, processed
            FROM events
            WHERE session_id = $1
            ORDER BY created_at ASC`, sessionID)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()

		type rowData struct {
			sessionID     string
			sourceJobID   sql.NullString
			eventType     string
			eventData     sql.NullString
			storageKey    sql.NullString
			priority      int
			shouldProcess bool
			processed     bool
		}

		var got []rowData
		for rows.Next() {
			var r rowData
			err := rows.Scan(
				&r.sessionID,
				&r.sourceJobID,
				&r.eventType,
				&r.eventData,
				&r.storageKey,
				&r.priority,
				&r.shouldProcess,
				&r.processed,
			)
			require.NoError(t, err)
			got = append(got, r)
		}
		require.NoError(t, rows.Err())
		require.Len(t, got, 2)

		// Common expectations
		for _, r := range got {
			assert.Equal(t, sessionID, r.sessionID)
			require.True(t, r.sourceJobID.Valid)
			assert.Equal(t, srcID, r.sourceJobID.String)
			assert.True(t, r.shouldProcess)
			assert.False(t, r.processed)
		}

		// First event assertions
		assert.Equal(t, "domain_seen", got[0].eventType)
		require.True(t, got[0].eventData.Valid)
		assert.JSONEq(t, `{"domain":"example.com"}`, got[0].eventData.String)
		require.True(t, got[0].storageKey.Valid)
		assert.Equal(t, "s3://bucket/key1", got[0].storageKey.String)
		assert.Equal(t, 42, got[0].priority)

		// Second event assertions
		assert.Equal(t, "file_seen", got[1].eventType)
		require.True(t, got[1].eventData.Valid)
		assert.JSONEq(t, `{"sha256":"abc"}`, got[1].eventData.String)
		assert.False(t, got[1].storageKey.Valid) // NULL storage key
		assert.Equal(t, 0, got[1].priority)      // default priority
	})
}

func TestEventRepo_BulkInsert_ShouldProcessFalse(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		eventRepo := &EventRepo{DB: db}

		sessionID := testSessionID1
		req := model.BulkEventRequest{
			SessionID:   sessionID,
			SourceJobID: nil,
			Events: []model.RawEvent{
				{
					Type:     "noop",
					Data:     json.RawMessage(`{"ok":true}`),
					Priority: intPtr(5),
				},
			},
		}

		created, err := eventRepo.BulkInsert(ctx, req, false)
		require.NoError(t, err)
		assert.Equal(t, 1, created)

		var shouldProcess, processed bool
		err = db.QueryRow(`SELECT should_process, processed FROM events WHERE session_id = $1`, sessionID).
			Scan(&shouldProcess, &processed)
		require.NoError(t, err)
		assert.False(t, shouldProcess)
		assert.False(t, processed)
	})
}

func TestEventRepo_BulkInsert_RollbackOnError(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		eventRepo := &EventRepo{DB: db}

		sessionID := testSessionID2
		req := model.BulkEventRequest{
			SessionID: sessionID,
			Events: []model.RawEvent{
				{
					Type:     "ok_event",
					Data:     json.RawMessage(`{"n":1}`),
					Priority: intPtr(10), // valid
				},
				{
					Type:     "bad_event",
					Data:     json.RawMessage(`{"n":2}`),
					Priority: intPtr(200), // invalid due to CHECK (0..100)
				},
			},
		}

		created, err := eventRepo.BulkInsert(ctx, req, true)
		require.Error(t, err)
		assert.Equal(t, 0, created)

		// Ensure no rows were inserted due to transaction rollback
		var cnt int
		err = db.QueryRow(`SELECT COUNT(*) FROM events WHERE session_id = $1`, sessionID).Scan(&cnt)
		require.NoError(t, err)
		assert.Equal(t, 0, cnt)
	})
}

func TestEventRepo_BulkInsert_NoSourceJobID(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		eventRepo := &EventRepo{DB: db}

		sessionID := "550e8400-e29b-41d4-a716-446655440003"
		req := model.BulkEventRequest{
			SessionID:   sessionID,
			SourceJobID: nil,
			Events: []model.RawEvent{
				{Type: "event_without_source", Data: json.RawMessage(`{"a":1}`)},
			},
		}

		created, err := eventRepo.BulkInsert(ctx, req, true)
		require.NoError(t, err)
		assert.Equal(t, 1, created)

		var src sql.NullString
		err = db.QueryRow(`SELECT source_job_id::text FROM events WHERE session_id = $1`, sessionID).
			Scan(&src)
		require.NoError(t, err)
		assert.False(t, src.Valid)
	})
}

func TestEventRepo_BulkInsertCopy_Success(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		eventRepo := &EventRepo{DB: db}
		jobRepo := NewJobRepo(db, RepoConfig{})

		// Create a source job to reference
		job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
			Type:     model.JobTypeBrowser,
			Payload:  json.RawMessage(`{"url":"https://example.com"}`),
			Priority: 25,
		})
		require.NoError(t, err)

		sessionID := "550e8400-e29b-41d4-a716-446655440004"
		srcID := job.ID

		req := model.BulkEventRequest{
			SessionID:   sessionID,
			SourceJobID: &srcID,
			Events: []model.RawEvent{
				{
					Type:       "domain_seen",
					Data:       json.RawMessage(`{"domain":"example.com"}`),
					StorageKey: evStringPtr("s3://bucket/key1"),
					Priority:   intPtr(42),
				},
				{
					Type:       "file_seen",
					Data:       json.RawMessage(`{"sha256":"abc"}`),
					StorageKey: nil, // no storage key
					Priority:   nil, // defaults to 0
				},
			},
		}

		created, err := eventRepo.BulkInsertCopy(ctx, req, true)
		require.NoError(t, err)
		assert.Equal(t, 2, created)

		// Verify the events were inserted correctly
		rows, err := db.Query(`
			SELECT session_id, source_job_id, event_type, event_data, storage_key, priority, should_process, processed
			FROM events
			WHERE session_id = $1
			ORDER BY event_type`, sessionID)
		require.NoError(t, err)
		defer rows.Close()

		type rowData struct {
			sessionID     string
			sourceJobID   sql.NullString
			eventType     string
			eventData     sql.NullString
			storageKey    sql.NullString
			priority      int
			shouldProcess bool
			processed     bool
		}

		var got []rowData
		for rows.Next() {
			var r rowData
			err := rows.Scan(
				&r.sessionID,
				&r.sourceJobID,
				&r.eventType,
				&r.eventData,
				&r.storageKey,
				&r.priority,
				&r.shouldProcess,
				&r.processed,
			)
			require.NoError(t, err)
			got = append(got, r)
		}
		require.NoError(t, rows.Err())
		require.Len(t, got, 2)

		// Common expectations
		for _, r := range got {
			assert.Equal(t, sessionID, r.sessionID)
			require.True(t, r.sourceJobID.Valid)
			assert.Equal(t, srcID, r.sourceJobID.String)
			assert.True(t, r.shouldProcess)
			assert.False(t, r.processed)
		}

		// First event assertions (domain_seen comes first alphabetically)
		assert.Equal(t, "domain_seen", got[0].eventType)
		require.True(t, got[0].eventData.Valid)
		assert.JSONEq(t, `{"domain":"example.com"}`, got[0].eventData.String)
		require.True(t, got[0].storageKey.Valid)
		assert.Equal(t, "s3://bucket/key1", got[0].storageKey.String)
		assert.Equal(t, 42, got[0].priority)

		// Second event assertions
		assert.Equal(t, "file_seen", got[1].eventType)
		require.True(t, got[1].eventData.Valid)
		assert.JSONEq(t, `{"sha256":"abc"}`, got[1].eventData.String)
		assert.False(t, got[1].storageKey.Valid) // NULL storage key
		assert.Equal(t, 0, got[1].priority)      // default priority
	})
}

func TestEventRepo_ListByJob_Success(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		eventRepo := &EventRepo{DB: db}

		// Create a job first to get a valid job ID
		jobRepo := NewJobRepo(db, RepoConfig{})
		job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
			Type:    model.JobTypeBrowser,
			Payload: json.RawMessage(`{"url": "https://example.com"}`),
		})
		require.NoError(t, err)

		jobID := job.ID
		sessionID := testSessionID1

		// Insert multiple events for this job
		req := model.BulkEventRequest{
			SessionID:   sessionID,
			SourceJobID: &jobID,
			Events: []model.RawEvent{
				{
					Type:       "domain_seen",
					Data:       json.RawMessage(`{"domain":"example.com"}`),
					StorageKey: evStringPtr("s3://bucket/key1"),
					Priority:   intPtr(10),
					Timestamp:  time.Now().Add(-3 * time.Minute), // Oldest
				},
				{
					Type:      "file_seen",
					Data:      json.RawMessage(`{"sha256":"abc123"}`),
					Priority:  intPtr(20),
					Timestamp: time.Now().Add(-2 * time.Minute), // Middle
				},
				{
					Type:      "alert_triggered",
					Data:      json.RawMessage(`{"severity":"high"}`),
					Priority:  intPtr(30),
					Timestamp: time.Now().Add(-1 * time.Minute), // Newest
				},
			},
		}

		created, err := eventRepo.BulkInsert(ctx, req, true)
		require.NoError(t, err)
		assert.Equal(t, 3, created)

		// Test ListByJob - should return events ordered by created_at ASC
		page, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{JobID: jobID, Limit: 10, Offset: 0})
		require.NoError(t, err)
		events := page.Events
		require.Len(t, events, 3)

		// Verify all events have the correct job ID and basic properties
		for _, event := range events {
			require.NotNil(t, event.SourceJobID)
			assert.Equal(t, jobID, *event.SourceJobID)
			assert.Equal(t, sessionID, event.SessionID)
			assert.True(t, event.ShouldProcess)
			assert.False(t, event.Processed)
		}

		// Verify ordering by created_at ASC (each event should have created_at >= previous)
		for i := 1; i < len(events); i++ {
			assert.True(
				t,
				events[i].CreatedAt.After(events[i-1].CreatedAt) ||
					events[i].CreatedAt.Equal(events[i-1].CreatedAt),
				"Events should be ordered by created_at ASC",
			)
		}

		// Verify we have all expected event types (order may vary due to timing)
		eventTypes := make(map[string]bool)
		for _, event := range events {
			eventTypes[event.EventType] = true
		}
		assert.True(t, eventTypes["domain_seen"], "Should have domain_seen event")
		assert.True(t, eventTypes["file_seen"], "Should have file_seen event")
		assert.True(t, eventTypes["alert_triggered"], "Should have alert_triggered event")

		// Find and verify specific events by type
		for _, event := range events {
			switch event.EventType {
			case "domain_seen":
				assert.JSONEq(t, `{"domain":"example.com"}`, string(event.EventData))
				assert.Equal(t, 10, event.Priority)
				require.NotNil(t, event.StorageKey)
				assert.Equal(t, "s3://bucket/key1", *event.StorageKey)
			case "file_seen":
				assert.JSONEq(t, `{"sha256":"abc123"}`, string(event.EventData))
				assert.Equal(t, 20, event.Priority)
				assert.Nil(t, event.StorageKey)
			case "alert_triggered":
				assert.JSONEq(t, `{"severity":"high"}`, string(event.EventData))
				assert.Equal(t, 30, event.Priority)
			}
		}
	})
}

func TestEventRepo_ListByJob_Pagination(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		eventRepo := &EventRepo{DB: db}

		// Create a job first
		jobRepo := NewJobRepo(db, RepoConfig{})
		job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
			Type:    model.JobTypeBrowser,
			Payload: json.RawMessage(`{"url": "https://example.com"}`),
		})
		require.NoError(t, err)

		jobID := job.ID
		sessionID := testSessionID2

		// Insert 5 events for pagination testing
		events := make([]model.RawEvent, 5)
		for i := range 5 {
			events[i] = model.RawEvent{
				Type:      fmt.Sprintf("event_%d", i),
				Data:      json.RawMessage(fmt.Sprintf(`{"index":%d}`, i)),
				Priority:  intPtr(i * 10),
				Timestamp: time.Now().Add(time.Duration(i) * time.Minute), // Ascending order
			}
		}

		req := model.BulkEventRequest{
			SessionID:   sessionID,
			SourceJobID: &jobID,
			Events:      events,
		}

		created, err := eventRepo.BulkInsert(ctx, req, true)
		require.NoError(t, err)
		assert.Equal(t, 5, created)

		// Test pagination: first page (limit=2, offset=0)
		page1, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{JobID: jobID, Limit: 2, Offset: 0})
		require.NoError(t, err)
		require.Len(t, page1.Events, 2)
		// Verify ordering within page
		assert.True(
			t,
			page1.Events[1].CreatedAt.After(page1.Events[0].CreatedAt) ||
				page1.Events[1].CreatedAt.Equal(page1.Events[0].CreatedAt),
		)

		// Test pagination: second page (limit=2, offset=2)
		page2, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{JobID: jobID, Limit: 2, Offset: 2})
		require.NoError(t, err)
		require.Len(t, page2.Events, 2)
		// Verify ordering within page
		assert.True(
			t,
			page2.Events[1].CreatedAt.After(page2.Events[0].CreatedAt) ||
				page2.Events[1].CreatedAt.Equal(page2.Events[0].CreatedAt),
		)

		// Test pagination: third page (limit=2, offset=4) - only 1 result
		page3, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{JobID: jobID, Limit: 2, Offset: 4})
		require.NoError(t, err)
		require.Len(t, page3.Events, 1)

		// Test pagination: beyond available data
		page4, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{JobID: jobID, Limit: 2, Offset: 10})
		require.NoError(t, err)
		assert.Empty(t, page4.Events)

		// Verify all pages together contain all events and maintain order
		allPage, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{JobID: jobID, Limit: 10, Offset: 0})
		require.NoError(t, err)
		require.Len(t, allPage.Events, 5)

		// Verify overall ordering
		for i := 1; i < len(allPage.Events); i++ {
			assert.True(
				t,
				allPage.Events[i].CreatedAt.After(allPage.Events[i-1].CreatedAt) ||
					allPage.Events[i].CreatedAt.Equal(allPage.Events[i-1].CreatedAt),
				"Events should be ordered by created_at ASC",
			)
		}
	})
}

func TestEventRepo_ListByJob_KeysetPagination(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	t.Run("timestamp_sort_forward_and_backward_with_filter", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			ctx := context.Background()
			eventRepo := &EventRepo{DB: db}
			jobRepo := NewJobRepo(db, RepoConfig{})

			job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			})
			require.NoError(t, err)

			jobID := job.ID
			filterType := "alpha.event"
			baseTime := time.Now().Add(-10 * time.Minute)
			_, err = eventRepo.BulkInsert(ctx, model.BulkEventRequest{
				SessionID:   testSessionID1,
				SourceJobID: &jobID,
				Events: []model.RawEvent{
					{Type: filterType, Data: json.RawMessage(`{"i":1}`), Timestamp: baseTime.Add(1 * time.Minute)},
					{Type: "other.event", Data: json.RawMessage(`{"i":2}`), Timestamp: baseTime.Add(2 * time.Minute)},
					{Type: filterType, Data: json.RawMessage(`{"i":3}`), Timestamp: baseTime.Add(3 * time.Minute)},
					{Type: filterType, Data: json.RawMessage(`{"i":4}`), Timestamp: baseTime.Add(4 * time.Minute)},
				},
			}, true)
			require.NoError(t, err)

			// initial page via offset path for compatibility
			firstPage, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{
				JobID:     jobID,
				EventType: &filterType,
				Limit:     2,
				Offset:    0,
			})
			require.NoError(t, err)
			require.Len(t, firstPage.Events, 2)

			encodeCursor := func(ev *model.Event) string {
				token, cursorErr := encodeEventCursorPayload(newEventCursorFromEvent(ev, defaultEventSortField, "ASC"))
				require.NoError(t, cursorErr)
				return token
			}

			after := encodeCursor(firstPage.Events[len(firstPage.Events)-1])
			secondPage, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{
				JobID:       jobID,
				EventType:   &filterType,
				Limit:       2,
				CursorAfter: &after,
			})
			require.NoError(t, err)
			require.Len(t, secondPage.Events, 1)
			assert.Nil(t, secondPage.NextCursor)
			require.NotNil(t, secondPage.PrevCursor)

			before := encodeCursor(secondPage.Events[0])
			backPage, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{
				JobID:        jobID,
				EventType:    &filterType,
				Limit:        2,
				CursorBefore: &before,
			})
			require.NoError(t, err)
			require.Len(t, backPage.Events, 2)
			assert.Equal(t, firstPage.Events[0].ID, backPage.Events[0].ID)
			assert.Equal(t, firstPage.Events[1].ID, backPage.Events[1].ID)
			require.NotNil(t, backPage.NextCursor)
			assert.Nil(t, backPage.PrevCursor)
		})
	})

	t.Run("event_type_sort_descending_keyset", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			ctx := context.Background()
			eventRepo := &EventRepo{DB: db}
			jobRepo := NewJobRepo(db, RepoConfig{})

			job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			})
			require.NoError(t, err)

			jobID := job.ID
			baseTime := time.Now().Add(-5 * time.Minute)
			_, err = eventRepo.BulkInsert(ctx, model.BulkEventRequest{
				SessionID:   testSessionID2,
				SourceJobID: &jobID,
				Events: []model.RawEvent{
					{Type: "alpha.event", Data: json.RawMessage(`{"i":1}`), Timestamp: baseTime.Add(3 * time.Minute)},
					{Type: "beta.event", Data: json.RawMessage(`{"i":2}`), Timestamp: baseTime.Add(1 * time.Minute)},
					{Type: "beta.event", Data: json.RawMessage(`{"i":3}`), Timestamp: baseTime.Add(2 * time.Minute)},
					{Type: "gamma.event", Data: json.RawMessage(`{"i":4}`), Timestamp: baseTime},
				},
			}, true)
			require.NoError(t, err)

			sortBy := evStringPtr("event_type")
			sortDir := evStringPtr("desc")
			firstPage, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{
				JobID:   jobID,
				Limit:   2,
				SortBy:  sortBy,
				SortDir: sortDir,
			})
			require.NoError(t, err)
			require.Len(t, firstPage.Events, 2)
			assert.Equal(t, "gamma.event", firstPage.Events[0].EventType)
			assert.Equal(t, "beta.event", firstPage.Events[1].EventType)
			require.Nil(t, firstPage.NextCursor)

			encodeCursor := func(ev *model.Event) string {
				token, cursorErr := encodeEventCursorPayload(newEventCursorFromEvent(ev, sortByEventType, "DESC"))
				require.NoError(t, cursorErr)
				return token
			}

			after := encodeCursor(firstPage.Events[1])
			secondPage, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{
				JobID:       jobID,
				Limit:       2,
				SortBy:      sortBy,
				SortDir:     sortDir,
				CursorAfter: &after,
			})
			require.NoError(t, err)
			require.Len(t, secondPage.Events, 2)
			assert.Equal(t, "beta.event", secondPage.Events[0].EventType)
			assert.Equal(t, "alpha.event", secondPage.Events[1].EventType)

			require.NotNil(t, secondPage.PrevCursor)
			assert.Nil(t, secondPage.NextCursor)

			before := encodeCursor(secondPage.Events[0])
			prevPage, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{
				JobID:        jobID,
				Limit:        2,
				SortBy:       sortBy,
				SortDir:      sortDir,
				CursorBefore: &before,
			})
			require.NoError(t, err)
			require.Len(t, prevPage.Events, 2)
			assert.Equal(t, firstPage.Events[0].ID, prevPage.Events[0].ID)
			assert.Equal(t, firstPage.Events[1].ID, prevPage.Events[1].ID)
		})
	})

	t.Run("cursor_with_no_results_returns_empty_page", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			ctx := context.Background()
			eventRepo := &EventRepo{DB: db}
			jobRepo := NewJobRepo(db, RepoConfig{})

			job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			})
			require.NoError(t, err)

			token, err := encodeEventCursorPayload(eventCursorPayload{
				SortBy:    defaultEventSortField,
				SortDir:   "ASC",
				CreatedAt: time.Now().Add(-time.Hour),
				ID:        uuid.NewString(),
			})
			require.NoError(t, err)

			page, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{
				JobID:       job.ID,
				Limit:       5,
				CursorAfter: &token,
			})
			require.NoError(t, err)
			assert.Empty(t, page.Events)
			assert.Nil(t, page.NextCursor)
			assert.Nil(t, page.PrevCursor)
		})
	})
}

func TestEventRepo_ListByJob_NoEvents(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		eventRepo := &EventRepo{DB: db}

		// Test with non-existent job ID (proper UUID format)
		eventsPage, err := eventRepo.ListByJob(
			ctx,
			model.EventListByJobOptions{JobID: "550e8400-e29b-41d4-a716-446655440999", Limit: 10, Offset: 0},
		)
		require.NoError(t, err)
		assert.Empty(t, eventsPage.Events)
	})
}

func TestEventRepo_ListByJob_LimitDefaults(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		eventRepo := &EventRepo{DB: db}

		// Create a job first
		jobRepo := NewJobRepo(db, RepoConfig{})
		job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
			Type:    model.JobTypeBrowser,
			Payload: json.RawMessage(`{"url": "https://example.com"}`),
		})
		require.NoError(t, err)

		jobID := job.ID

		// Test with limit <= 0 (should default to 50)
		events, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{JobID: jobID, Limit: 0, Offset: 0})
		require.NoError(t, err)
		assert.Empty(t, events.Events) // No events inserted, so empty result

		// Test with limit > 1000 (should cap at 1000)
		events, err = eventRepo.ListByJob(ctx, model.EventListByJobOptions{JobID: jobID, Limit: 2000, Offset: 0})
		require.NoError(t, err)
		assert.Empty(t, events.Events) // No events inserted, so empty result
	})
}

func TestEventRepo_BulkInsertWithProcessingFlags(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		eventRepo := &EventRepo{DB: db}

		// Create a test job
		jobRepo := NewJobRepo(db, RepoConfig{})
		job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
			Type:     model.JobTypeBrowser,
			Priority: 10,
			Payload:  json.RawMessage(`{"url":"https://example.com"}`),
		})
		require.NoError(t, err)

		sessionID := "550e8400-e29b-41d4-a716-446655440000"
		srcID := job.ID

		req := model.BulkEventRequest{
			SessionID:   sessionID,
			SourceJobID: &srcID,
			Events: []model.RawEvent{
				{
					Type:       "Network.requestWillBeSent",
					Data:       json.RawMessage(`{"url":"https://example.com"}`),
					StorageKey: evStringPtr("s3://bucket/key1"),
					Priority:   intPtr(42),
				},
				{
					Type:       "Runtime.consoleAPICalled",
					Data:       json.RawMessage(`{"type":"log"}`),
					StorageKey: nil,
					Priority:   nil,
				},
				{
					Type:       "domain_seen",
					Data:       json.RawMessage(`{"domain":"example.com"}`),
					StorageKey: nil,
					Priority:   intPtr(10),
				},
			},
		}

		// Set processing flags: process first and third events, skip second
		shouldProcessMap := map[int]bool{
			0: true,  // Network.requestWillBeSent - should process
			1: false, // Runtime.consoleAPICalled - should not process
			2: true,  // domain_seen - should process
		}

		count, err := eventRepo.BulkInsertWithProcessingFlags(ctx, req, shouldProcessMap)
		require.NoError(t, err)
		assert.Equal(t, 3, count)

		// Verify events were inserted with correct processing flags
		eventsPage, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{
			JobID:  job.ID,
			Limit:  10,
			Offset: 0,
		})
		require.NoError(t, err)
		require.Len(t, eventsPage.Events, 3)

		// Check processing flags match expectations
		eventsByType := make(map[string]*model.Event)
		for _, event := range eventsPage.Events {
			eventsByType[event.EventType] = event
		}

		// Network.requestWillBeSent should be marked for processing
		networkEvent := eventsByType["Network.requestWillBeSent"]
		require.NotNil(t, networkEvent)
		assert.True(t, networkEvent.ShouldProcess)

		// Runtime.consoleAPICalled should not be marked for processing
		consoleEvent := eventsByType["Runtime.consoleAPICalled"]
		require.NotNil(t, consoleEvent)
		assert.False(t, consoleEvent.ShouldProcess)

		// domain_seen should be marked for processing
		domainEvent := eventsByType["domain_seen"]
		require.NotNil(t, domainEvent)
		assert.True(t, domainEvent.ShouldProcess)
	})
}

func TestEventRepo_GetByIDs(t *testing.T) {
	t.Run("invalid_uuid_returns_error", func(t *testing.T) {
		repo := &EventRepo{DB: nil}
		_, err := repo.GetByIDs(context.Background(), []string{"not-a-uuid"})
		require.Error(t, err)
	})

	t.Run("returns_events_for_ids", func(t *testing.T) {
		testutil.SkipIfNoTestDB(t)

		testutil.WithAutoDB(t, func(db *sql.DB) {
			ctx := context.Background()
			eventRepo := &EventRepo{DB: db}
			jobRepo := NewJobRepo(db, RepoConfig{})

			job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			})
			require.NoError(t, err)

			jobID := job.ID
			_, err = eventRepo.BulkInsert(ctx, model.BulkEventRequest{
				SessionID:   testSessionID1,
				SourceJobID: &jobID,
				Events: []model.RawEvent{
					{Type: "console.log", Data: json.RawMessage(`{"msg":"hello"}`), Timestamp: time.Now()},
					{
						Type:      "network.request",
						Data:      json.RawMessage(`{"url":"https://example.com"}`),
						Timestamp: time.Now(),
					},
				},
			}, true)
			require.NoError(t, err)

			page, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{
				JobID:  jobID,
				Limit:  10,
				Offset: 0,
			})
			require.NoError(t, err)
			require.Len(t, page.Events, 2)

			ids := []string{page.Events[0].ID, page.Events[1].ID}

			got, err := eventRepo.GetByIDs(ctx, ids)
			require.NoError(t, err)
			require.Len(t, got, 2)

			gotIDs := map[string]bool{
				got[0].ID: true,
				got[1].ID: true,
			}
			assert.True(t, gotIDs[ids[0]])
			assert.True(t, gotIDs[ids[1]])
		})
	})
}

// helpers.
func intPtr(i int) *int            { return &i }
func evStringPtr(s string) *string { return &s }
