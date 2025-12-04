package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
)

// insertHTTPAlertSinkSecret is a test helper to insert a secret for testing.
func insertHTTPAlertSinkSecret(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO secrets (name, value)
		VALUES ($1, 'encrypted-value')
		ON CONFLICT (name) DO NOTHING
	`, name)
	require.NoError(t, err)
}

func TestHTTPAlertSinkRepo_Create(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	tests := getCreateTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.WithAutoDB(t, func(db *sql.DB) {
				repo := NewHTTPAlertSinkRepo(db)

				// Ensure secrets exist when provided (skip invalid/empty names)
				for _, s := range tt.req.Secrets {
					if strings.TrimSpace(s) == "" {
						continue
					}
					insertHTTPAlertSinkSecret(t, db, s)
				}

				sink, err := repo.Create(context.Background(), tt.req)

				if tt.wantErr {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.errMsg)
					assert.Nil(t, sink)
					return
				}

				assertValidCreatedSink(t, tt.req, sink)
			})
		})
	}
}

func getCreateTestCases() []struct {
	name    string
	req     *model.CreateHTTPAlertSinkRequest
	wantErr bool
	errMsg  string
} {
	return []struct {
		name    string
		req     *model.CreateHTTPAlertSinkRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request",
			req: &model.CreateHTTPAlertSinkRequest{
				Name:   "test-alert-sink",
				URI:    "https://example.com/webhook",
				Method: "POST",
			},
			wantErr: false,
		},
		{
			name: "valid request with all fields",
			req: &model.CreateHTTPAlertSinkRequest{
				Name:        "test-alert-sink-full",
				URI:         "https://example.com/webhook",
				Method:      "POST",
				Body:        testutil.StringPtr(`{"message": "alert"}`),
				QueryParams: testutil.StringPtr("token=abc123"),
				Headers:     testutil.StringPtr("Content-Type: application/json"),
				OkStatus:    testutil.IntPtr(201),
				Retry:       testutil.IntPtr(5),
				Secrets:     []string{"TEST_SECRET_1", "TEST_SECRET_2"},
			},
			wantErr: false,
		},
		{
			name: "invalid name too short",
			req: &model.CreateHTTPAlertSinkRequest{
				Name:   "ab",
				URI:    "https://example.com/webhook",
				Method: "POST",
			},
			wantErr: true,
			errMsg:  "name must be at least 3 characters",
		},
		{
			name: "invalid URI",
			req: &model.CreateHTTPAlertSinkRequest{
				Name:   "test-alert-sink",
				URI:    "ftp://example.com/webhook",
				Method: "POST",
			},
			wantErr: true,
			errMsg:  "uri must use http or https scheme",
		},
		{
			name: "invalid method",
			req: &model.CreateHTTPAlertSinkRequest{
				Name:   "test-alert-sink",
				URI:    "https://example.com/webhook",
				Method: "INVALID",
			},
			wantErr: true,
			errMsg:  "method must be one of: GET, POST, PUT, PATCH, DELETE",
		},
		{
			name: "empty secret in slice",
			req: &model.CreateHTTPAlertSinkRequest{
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
			req: &model.CreateHTTPAlertSinkRequest{
				Name:    "test-alert-sink",
				URI:     "https://example.com/webhook",
				Method:  "POST",
				Secrets: []string{"SECRET_1", "SECRET_2", "SECRET_1"},
			},
			wantErr: true,
			errMsg:  "secrets cannot contain duplicate entries",
		},
	}
}

func assertValidCreatedSink(t *testing.T, req *model.CreateHTTPAlertSinkRequest, sink *model.HTTPAlertSink) {
	t.Helper()

	require.NotNil(t, req)
	require.NotNil(t, sink)

	assert.NotEmpty(t, sink.ID)
	assert.Equal(t, req.Name, sink.Name)
	assert.Equal(t, req.URI, sink.URI)
	assert.Equal(t, strings.ToUpper(req.Method), sink.Method) // Should be normalized to uppercase
	assert.NotZero(t, sink.CreatedAt)

	// Check optional fields
	if req.Body != nil {
		assert.Equal(t, *req.Body, *sink.Body)
	} else {
		assert.Nil(t, sink.Body)
	}

	if req.QueryParams != nil {
		assert.Equal(t, *req.QueryParams, *sink.QueryParams)
	} else {
		assert.Nil(t, sink.QueryParams)
	}

	if req.Headers != nil {
		assert.Equal(t, *req.Headers, *sink.Headers)
	} else {
		assert.Nil(t, sink.Headers)
	}

	expectedOkStatus := 200
	if req.OkStatus != nil {
		expectedOkStatus = *req.OkStatus
	}
	assert.Equal(t, expectedOkStatus, sink.OkStatus)

	expectedRetry := 3
	if req.Retry != nil {
		expectedRetry = *req.Retry
	}
	assert.Equal(t, expectedRetry, sink.Retry)

	// Check secrets
	assert.Len(t, sink.Secrets, len(req.Secrets))
	for _, expectedSecret := range req.Secrets {
		assert.Contains(t, sink.Secrets, expectedSecret)
	}
}

func TestHTTPAlertSinkRepo_Create_DuplicateName(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewHTTPAlertSinkRepo(db)
		ctx := context.Background()

		req := &model.CreateHTTPAlertSinkRequest{
			Name:   "duplicate-test",
			URI:    "https://example.com/webhook",
			Method: "POST",
		}

		// Create first sink
		sink1, err := repo.Create(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, sink1)

		// Try to create second sink with same name
		req.URI = "https://different.example.com/webhook"
		sink2, err := repo.Create(ctx, req)
		require.Error(t, err)
		assert.Nil(t, sink2)
		assert.ErrorIs(t, err, ErrHTTPAlertSinkNameExists)
	})
}

func TestHTTPAlertSinkRepo_GetByID(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewHTTPAlertSinkRepo(db)
		ctx := context.Background()

		// Insert test secrets
		insertHTTPAlertSinkSecret(t, db, "GET_SECRET_1")
		insertHTTPAlertSinkSecret(t, db, "GET_SECRET_2")

		req := &model.CreateHTTPAlertSinkRequest{
			Name:        "get-by-id-test",
			URI:         "https://example.com/webhook",
			Method:      "POST",
			Body:        testutil.StringPtr(`{"test": true}`),
			QueryParams: testutil.StringPtr("param=value"),
			Headers:     testutil.StringPtr("Authorization: Bearer token"),
			OkStatus:    testutil.IntPtr(202),
			Retry:       testutil.IntPtr(2),
			Secrets:     []string{"GET_SECRET_1", "GET_SECRET_2"},
		}

		created, err := repo.Create(ctx, req)
		require.NoError(t, err)

		// Test getting by ID
		found, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		require.NotNil(t, found)

		assert.Equal(t, created.ID, found.ID)
		assert.Equal(t, created.Name, found.Name)
		assert.Equal(t, created.URI, found.URI)
		assert.Equal(t, created.Method, found.Method)
		assert.Equal(t, created.Body, found.Body)
		assert.Equal(t, created.QueryParams, found.QueryParams)
		assert.Equal(t, created.Headers, found.Headers)
		assert.Equal(t, created.OkStatus, found.OkStatus)
		assert.Equal(t, created.Retry, found.Retry)
		assert.Equal(t, created.Secrets, found.Secrets)
		assert.Equal(t, created.CreatedAt.Unix(), found.CreatedAt.Unix())

		// Test getting non-existent ID
		notFound, err := repo.GetByID(ctx, "550e8400-e29b-41d4-a716-446655440000")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrHTTPAlertSinkNotFound)
		assert.Nil(t, notFound)
	})
}

func TestHTTPAlertSinkRepo_GetByName(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewHTTPAlertSinkRepo(db)
		ctx := context.Background()

		// Insert test secrets
		insertHTTPAlertSinkSecret(t, db, "NAME_SECRET_1")
		insertHTTPAlertSinkSecret(t, db, "NAME_SECRET_2")

		req := &model.CreateHTTPAlertSinkRequest{
			Name:    "get-by-name-test",
			URI:     "https://example.com/webhook",
			Method:  "PUT",
			Secrets: []string{"NAME_SECRET_1", "NAME_SECRET_2"},
		}

		created, err := repo.Create(ctx, req)
		require.NoError(t, err)

		// Test getting by name
		found, err := repo.GetByName(ctx, created.Name)
		require.NoError(t, err)
		require.NotNil(t, found)

		assert.Equal(t, created.ID, found.ID)
		assert.Equal(t, created.Name, found.Name)
		assert.Equal(t, created.URI, found.URI)
		assert.Equal(t, created.Method, found.Method)
		assert.Equal(t, created.Secrets, found.Secrets)

		// Test getting non-existent name
		notFound, err := repo.GetByName(ctx, "non-existent-sink")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrHTTPAlertSinkNotFound)
		assert.Nil(t, notFound)
	})
}

func TestHTTPAlertSinkRepo_List(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewHTTPAlertSinkRepo(db)
		ctx := context.Background()

		// Create multiple sinks for testing pagination
		for i := range 5 {
			req := &model.CreateHTTPAlertSinkRequest{
				Name:   fmt.Sprintf("list-test-sink-%d-%d", i, time.Now().UnixNano()),
				URI:    "https://example.com/webhook",
				Method: "POST",
			}
			_, err := repo.Create(ctx, req)
			require.NoError(t, err)
		}

		// Test listing with default pagination
		sinks, err := repo.List(ctx, 10, 0)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(sinks), 5) // At least our 5 sinks

		// Test pagination
		firstPage, err := repo.List(ctx, 2, 0)
		require.NoError(t, err)
		assert.Len(t, firstPage, 2)

		secondPage, err := repo.List(ctx, 2, 2)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(secondPage), 1)

		// Ensure no overlap between pages
		firstPageIDs := make(map[string]bool)
		for _, sink := range firstPage {
			firstPageIDs[sink.ID] = true
		}
		for _, sink := range secondPage {
			assert.False(t, firstPageIDs[sink.ID], "Found duplicate sink ID between pages")
		}

		// Test with invalid pagination parameters
		sinks, err = repo.List(ctx, -1, -1)
		require.NoError(t, err)
		assert.NotEmpty(t, sinks) // Should use defaults
	})
}

func TestHTTPAlertSinkRepo_Update(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewHTTPAlertSinkRepo(db)
		ctx := context.Background()

		// Insert test secrets
		insertHTTPAlertSinkSecret(t, db, "UPDATE_SECRET_1")
		insertHTTPAlertSinkSecret(t, db, "UPDATE_SECRET_2")
		insertHTTPAlertSinkSecret(t, db, "UPDATE_SECRET_3")

		// Create initial sink
		req := &model.CreateHTTPAlertSinkRequest{
			Name:        "update-test-sink",
			URI:         "https://example.com/webhook",
			Method:      "POST",
			Body:        testutil.StringPtr(`{"initial": true}`),
			QueryParams: testutil.StringPtr("initial=true"),
			Headers:     testutil.StringPtr("Initial-Header: value"),
			OkStatus:    testutil.IntPtr(200),
			Retry:       testutil.IntPtr(3),
			Secrets:     []string{"UPDATE_SECRET_1"},
		}

		created, err := repo.Create(ctx, req)
		require.NoError(t, err)

		// Test updating all fields
		updateReq := &model.UpdateHTTPAlertSinkRequest{
			Name:        testutil.StringPtr("updated-sink-name"),
			URI:         testutil.StringPtr("https://updated.example.com/webhook"),
			Method:      testutil.StringPtr("PATCH"),
			Body:        testutil.StringPtr(`{"updated": true}`),
			QueryParams: testutil.StringPtr("updated=true"),
			Headers:     testutil.StringPtr("Updated-Header: value"),
			OkStatus:    testutil.IntPtr(201),
			Retry:       testutil.IntPtr(5),
			Secrets:     []string{"UPDATE_SECRET_2", "UPDATE_SECRET_3"},
		}

		updated, err := repo.Update(ctx, created.ID, updateReq)
		require.NoError(t, err)
		require.NotNil(t, updated)

		assert.Equal(t, created.ID, updated.ID) // ID should not change
		assert.Equal(t, *updateReq.Name, updated.Name)
		assert.Equal(t, *updateReq.URI, updated.URI)
		assert.Equal(t, *updateReq.Method, updated.Method)
		assert.Equal(t, *updateReq.Body, *updated.Body)
		assert.Equal(t, *updateReq.QueryParams, *updated.QueryParams)
		assert.Equal(t, *updateReq.Headers, *updated.Headers)
		assert.Equal(t, *updateReq.OkStatus, updated.OkStatus)
		assert.Equal(t, *updateReq.Retry, updated.Retry)
		assert.Equal(t, updateReq.Secrets, updated.Secrets)
		assert.Equal(t, created.CreatedAt.Unix(), updated.CreatedAt.Unix()) // CreatedAt should not change

		// Test partial update (only name)
		partialReq := &model.UpdateHTTPAlertSinkRequest{
			Name: testutil.StringPtr("partially-updated-name"),
		}

		partialUpdated, err := repo.Update(ctx, created.ID, partialReq)
		require.NoError(t, err)
		require.NotNil(t, partialUpdated)

		assert.Equal(t, *partialReq.Name, partialUpdated.Name)
		// Other fields should remain from previous update
		assert.Equal(t, *updateReq.URI, partialUpdated.URI)
		assert.Equal(t, *updateReq.Method, partialUpdated.Method)

		// Test updating non-existent sink
		nonExistentUpdate := &model.UpdateHTTPAlertSinkRequest{
			Name: testutil.StringPtr("non-existent-update"),
		}
		notFound, err := repo.Update(ctx, "550e8400-e29b-41d4-a716-446655440000", nonExistentUpdate)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrHTTPAlertSinkNotFound)
		assert.Nil(t, notFound)
	})
}

func TestHTTPAlertSinkRepo_Update_ValidationErrors(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewHTTPAlertSinkRepo(db)
		ctx := context.Background()

		// Create initial sink
		req := &model.CreateHTTPAlertSinkRequest{
			Name:   "validation-test-sink",
			URI:    "https://example.com/webhook",
			Method: "POST",
		}

		created, err := repo.Create(ctx, req)
		require.NoError(t, err)

		tests := []struct {
			name      string
			updateReq *model.UpdateHTTPAlertSinkRequest
			wantErr   bool
			errMsg    string
		}{
			{
				name:      "no updates provided",
				updateReq: &model.UpdateHTTPAlertSinkRequest{
					// No fields set
				},
				wantErr: true,
				errMsg:  "at least one field must be updated",
			},
			{
				name: "invalid name too short",
				updateReq: &model.UpdateHTTPAlertSinkRequest{
					Name: testutil.StringPtr("ab"),
				},
				wantErr: true,
				errMsg:  "name must be at least 3 characters",
			},
			{
				name: "invalid URI scheme",
				updateReq: &model.UpdateHTTPAlertSinkRequest{
					URI: testutil.StringPtr("ftp://example.com/webhook"),
				},
				wantErr: true,
				errMsg:  "uri must use http or https scheme",
			},
			{
				name: "invalid method",
				updateReq: &model.UpdateHTTPAlertSinkRequest{
					Method: testutil.StringPtr("INVALID"),
				},
				wantErr: true,
				errMsg:  "method must be one of: GET, POST, PUT, PATCH, DELETE",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				updated, err := repo.Update(ctx, created.ID, tt.updateReq)

				if tt.wantErr {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.errMsg)
					assert.Nil(t, updated)
					return
				}

				require.NoError(t, err)
				require.NotNil(t, updated)
			})
		}
	})
}

func TestHTTPAlertSinkRepo_Delete(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewHTTPAlertSinkRepo(db)
		ctx := context.Background()

		// Create a sink to delete
		req := &model.CreateHTTPAlertSinkRequest{
			Name:   "delete-test-sink",
			URI:    "https://example.com/webhook",
			Method: "DELETE",
		}

		created, err := repo.Create(ctx, req)
		require.NoError(t, err)

		// Verify it exists
		found, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		require.NotNil(t, found)

		// Delete it
		deleted, err := repo.Delete(ctx, created.ID)
		require.NoError(t, err)
		assert.True(t, deleted)

		// Verify it's gone
		notFound, err := repo.GetByID(ctx, created.ID)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrHTTPAlertSinkNotFound)
		assert.Nil(t, notFound)

		// Try to delete non-existent sink
		notDeleted, err := repo.Delete(ctx, "550e8400-e29b-41d4-a716-446655440000")
		require.NoError(t, err)
		assert.False(t, notDeleted)
	})
}

func TestHTTPAlertSinkRepo_Update_DuplicateName(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewHTTPAlertSinkRepo(db)
		ctx := context.Background()

		// Create first sink
		req1 := &model.CreateHTTPAlertSinkRequest{
			Name:   "duplicate-name-test-1",
			URI:    "https://example.com/webhook1",
			Method: "POST",
		}
		sink1, err := repo.Create(ctx, req1)
		require.NoError(t, err)

		// Create second sink
		req2 := &model.CreateHTTPAlertSinkRequest{
			Name:   "duplicate-name-test-2",
			URI:    "https://example.com/webhook2",
			Method: "POST",
		}
		sink2, err := repo.Create(ctx, req2)
		require.NoError(t, err)

		// Try to update second sink to have same name as first
		updateReq := &model.UpdateHTTPAlertSinkRequest{
			Name: &sink1.Name,
		}
		updated, err := repo.Update(ctx, sink2.ID, updateReq)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrHTTPAlertSinkNameExists)
		assert.Nil(t, updated)
	})
}

func TestHTTPAlertSinkRepo_SecretsAssociation(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewHTTPAlertSinkRepo(db)
		ctx := context.Background()

		// Insert test secrets
		insertHTTPAlertSinkSecret(t, db, "ASSOC_SECRET_1")
		insertHTTPAlertSinkSecret(t, db, "ASSOC_SECRET_2")
		insertHTTPAlertSinkSecret(t, db, "ASSOC_SECRET_3")

		// Create sink with secrets
		req := &model.CreateHTTPAlertSinkRequest{
			Name:    "secrets-test-sink",
			URI:     "https://example.com/webhook",
			Method:  "POST",
			Secrets: []string{"ASSOC_SECRET_1", "ASSOC_SECRET_2"},
		}

		created, err := repo.Create(ctx, req)
		require.NoError(t, err)
		assert.Len(t, created.Secrets, 2)
		assert.Contains(t, created.Secrets, "ASSOC_SECRET_1")
		assert.Contains(t, created.Secrets, "ASSOC_SECRET_2")

		// Update to replace secrets
		updateReq := &model.UpdateHTTPAlertSinkRequest{
			Secrets: []string{"ASSOC_SECRET_3"},
		}

		updated, err := repo.Update(ctx, created.ID, updateReq)
		require.NoError(t, err)
		assert.Len(t, updated.Secrets, 1)
		assert.Contains(t, updated.Secrets, "ASSOC_SECRET_3")
		assert.NotContains(t, updated.Secrets, "ASSOC_SECRET_1")
		assert.NotContains(t, updated.Secrets, "ASSOC_SECRET_2")

		// Update to clear all secrets
		clearReq := &model.UpdateHTTPAlertSinkRequest{
			Secrets: []string{},
		}

		cleared, err := repo.Update(ctx, created.ID, clearReq)
		require.NoError(t, err)
		assert.Empty(t, cleared.Secrets)
	})
}
