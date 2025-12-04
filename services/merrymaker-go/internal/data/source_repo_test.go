package data

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
)

// insertSecret is a test helper to create a secret by name with dummy value.
func insertSecret(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO secrets (name, value) VALUES ($1, $2) ON CONFLICT (name) DO NOTHING`, name, "dummy")
	require.NoError(t, err)
}

func TestSourceRepo_Create(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	tests := []struct {
		name    string
		req     *model.CreateSourceRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid source creation",
			req: &model.CreateSourceRequest{
				Name:    "test-source",
				Value:   "console.log('hello world');",
				Test:    false,
				Secrets: []string{"API_KEY", "SECRET_TOKEN"},
			},
			wantErr: false,
		},
		{
			name: "source with test flag",
			req: &model.CreateSourceRequest{
				Name:    "test-source-2",
				Value:   "console.log('test');",
				Test:    true,
				Secrets: []string{},
			},
			wantErr: false,
		},
		{
			name: "source without secrets",
			req: &model.CreateSourceRequest{
				Name:  "simple-source",
				Value: "console.log('simple');",
				Test:  false,
			},
			wantErr: false,
		},
		{
			name: "empty name",
			req: &model.CreateSourceRequest{
				Name:  "",
				Value: "console.log('test');",
			},
			wantErr: true,
			errMsg:  "name is required and cannot be empty",
		},
		{
			name: "empty value",
			req: &model.CreateSourceRequest{
				Name:  "test-source",
				Value: "",
			},
			wantErr: true,
			errMsg:  "value is required and cannot be empty",
		},
		{
			name: "name too long",
			req: &model.CreateSourceRequest{
				Name:  string(make([]byte, 256)), // 256 characters, exceeds 255 limit
				Value: "console.log('test');",
			},
			wantErr: true,
			errMsg:  "name cannot exceed 255 characters",
		},
		{
			name: "empty secret in slice",
			req: &model.CreateSourceRequest{
				Name:    "test-source",
				Value:   "console.log('test');",
				Secrets: []string{"VALID_SECRET", ""},
			},
			wantErr: true,
			errMsg:  "secrets cannot contain empty or whitespace-only entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.WithAutoDB(t, func(db *sql.DB) {
				repo := NewSourceRepo(db)

				// Ensure secrets exist when provided (skip invalid/empty names)
				for _, s := range tt.req.Secrets {
					if strings.TrimSpace(s) == "" {
						continue
					}
					insertSecret(t, db, s)
				}

				source, err := repo.Create(context.Background(), tt.req)

				if tt.wantErr {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.errMsg)
					assert.Nil(t, source)
					return
				}

				require.NoError(t, err)
				require.NotNil(t, source)

				// Verify the created source
				assert.NotEmpty(t, source.ID)
				assert.Equal(t, tt.req.Name, source.Name)
				assert.Equal(t, tt.req.Value, source.Value)
				assert.Equal(t, tt.req.Test, source.Test)

				// Handle secrets comparison - PostgreSQL returns empty array for nil input due to NOT NULL DEFAULT '{}'
				expectedSecrets := tt.req.Secrets
				if expectedSecrets == nil {
					expectedSecrets = []string{}
				}
				assert.Equal(t, expectedSecrets, source.Secrets)
				assert.False(t, source.CreatedAt.IsZero())
			})
		})
	}
}

func TestSourceRepo_Create_DuplicateName(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSourceRepo(db)
		ctx := context.Background()

		req := &model.CreateSourceRequest{
			Name:  "duplicate-test",
			Value: "console.log('first');",
		}

		// Create first source
		source1, err := repo.Create(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, source1)

		// Try to create second source with same name
		req.Value = "console.log('second');"
		source2, err := repo.Create(ctx, req)
		require.Error(t, err)
		assert.Nil(t, source2)
		assert.ErrorIs(t, err, ErrSourceNameExists)
	})
}

func TestSourceRepo_GetByID(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSourceRepo(db)
		ctx := context.Background()

		// Create a source first
		req := &model.CreateSourceRequest{
			Name:    "get-by-id-test",
			Value:   "console.log('get by id');",
			Test:    true,
			Secrets: []string{"SECRET1", "SECRET2"},
		}

		for _, s := range req.Secrets {
			insertSecret(t, db, s)
		}

		created, err := repo.Create(ctx, req)
		require.NoError(t, err)

		// Test getting by ID
		found, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		require.NotNil(t, found)

		assert.Equal(t, created.ID, found.ID)
		assert.Equal(t, created.Name, found.Name)
		assert.Equal(t, created.Value, found.Value)
		assert.Equal(t, created.Test, found.Test)
		assert.Equal(t, created.Secrets, found.Secrets)
		assert.Equal(t, created.CreatedAt.Unix(), found.CreatedAt.Unix())

		// Test getting non-existent ID
		notFound, err := repo.GetByID(ctx, "550e8400-e29b-41d4-a716-446655440000")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrSourceNotFound)
		assert.Nil(t, notFound)
	})
}

func TestSourceRepo_GetByName(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSourceRepo(db)
		ctx := context.Background()

		// Create a source first
		req := &model.CreateSourceRequest{
			Name:    "get-by-name-test",
			Value:   "console.log('get by name');",
			Test:    false,
			Secrets: []string{"SECRET"},
		}

		for _, s := range req.Secrets {
			insertSecret(t, db, s)
		}

		created, err := repo.Create(ctx, req)
		require.NoError(t, err)

		// Test getting by name
		found, err := repo.GetByName(ctx, created.Name)
		require.NoError(t, err)
		require.NotNil(t, found)

		assert.Equal(t, created.ID, found.ID)
		assert.Equal(t, created.Name, found.Name)
		assert.Equal(t, created.Value, found.Value)
		assert.Equal(t, created.Test, found.Test)
		assert.Equal(t, created.Secrets, found.Secrets)

		// Test getting non-existent name
		notFound, err := repo.GetByName(ctx, "non-existent-source")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrSourceNotFound)
		assert.Nil(t, notFound)
	})
}

func TestSourceRepo_List(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSourceRepo(db)
		ctx := context.Background()

		// Create multiple sources
		sources := []*model.CreateSourceRequest{
			{Name: "source-1", Value: "console.log('1');", Test: false},
			{Name: "source-2", Value: "console.log('2');", Test: true},
			{Name: "source-3", Value: "console.log('3');", Test: false, Secrets: []string{"SECRET"}},
		}

		for _, req := range sources {
			for _, s := range req.Secrets {
				insertSecret(t, db, s)
			}
			_, err := repo.Create(ctx, req)
			require.NoError(t, err)
		}

		// Test listing all sources
		listed, err := repo.List(ctx, 10, 0)
		require.NoError(t, err)
		assert.Len(t, listed, 3)

		// Verify sources are ordered by created_at DESC, name ASC
		// Since they were created in sequence, the most recent should be first
		assert.Equal(t, "source-3", listed[0].Name)
		assert.Equal(t, "source-2", listed[1].Name)
		assert.Equal(t, "source-1", listed[2].Name)

		// Test pagination
		page1, err := repo.List(ctx, 2, 0)
		require.NoError(t, err)
		assert.Len(t, page1, 2)

		page2, err := repo.List(ctx, 2, 2)
		require.NoError(t, err)
		assert.Len(t, page2, 1)

		// Test empty result with high offset
		empty, err := repo.List(ctx, 10, 100)
		require.NoError(t, err)
		assert.Empty(t, empty)

		// Test default limit
		defaultLimit, err := repo.List(ctx, 0, 0)
		require.NoError(t, err)
		assert.Len(t, defaultLimit, 3) // Should use default limit of 50

		// Test negative offset
		negativeOffset, err := repo.List(ctx, 10, -5)
		require.NoError(t, err)
		assert.Len(t, negativeOffset, 3) // Should treat as 0
	})
}

func TestSourceRepo_Update(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSourceRepo(db)
		ctx := context.Background()

		// Create a source first
		req := &model.CreateSourceRequest{
			Name:    "update-test",
			Value:   "console.log('original');",
			Test:    false,
			Secrets: []string{"ORIGINAL_SECRET"},
		}

		for _, s := range req.Secrets {
			insertSecret(t, db, s)
		}

		created, err := repo.Create(ctx, req)
		require.NoError(t, err)

		// Test updating name only
		nameUpdate := model.UpdateSourceRequest{
			Name: testutil.StringPtr("updated-name"),
		}
		updated, err := repo.Update(ctx, created.ID, nameUpdate)
		require.NoError(t, err)
		require.NotNil(t, updated)
		assert.Equal(t, "updated-name", updated.Name)
		assert.Equal(t, created.Value, updated.Value) // Should remain unchanged
		assert.Equal(t, created.Test, updated.Test)
		assert.Equal(t, created.Secrets, updated.Secrets)

		// Ensure new secrets exist before updating
		insertSecret(t, db, "NEW_SECRET1")
		insertSecret(t, db, "NEW_SECRET2")

		// Test updating value only
		valueUpdate := model.UpdateSourceRequest{
			Value: testutil.StringPtr("console.log('updated');"),
		}
		updated, err = repo.Update(ctx, created.ID, valueUpdate)
		require.NoError(t, err)
		assert.Equal(t, "console.log('updated');", updated.Value)

		// Ensure multi-update secret exists
		insertSecret(t, db, "MULTI_SECRET")

		assert.Equal(t, "updated-name", updated.Name) // Should remain from previous update

		// Test updating test flag only
		testUpdate := model.UpdateSourceRequest{
			Test: testutil.BoolPtr(true),
		}
		updated, err = repo.Update(ctx, created.ID, testUpdate)
		require.NoError(t, err)
		assert.True(t, updated.Test)

		// Test updating secrets only
		secretsUpdate := model.UpdateSourceRequest{
			Secrets: []string{"NEW_SECRET1", "NEW_SECRET2"},
		}
		updated, err = repo.Update(ctx, created.ID, secretsUpdate)
		require.NoError(t, err)
		assert.Equal(t, []string{"NEW_SECRET1", "NEW_SECRET2"}, updated.Secrets)

		// Test updating multiple fields
		multiUpdate := model.UpdateSourceRequest{
			Name:    testutil.StringPtr("multi-update"),
			Value:   testutil.StringPtr("console.log('multi');"),
			Test:    testutil.BoolPtr(false),
			Secrets: []string{"MULTI_SECRET"},
		}
		updated, err = repo.Update(ctx, created.ID, multiUpdate)
		require.NoError(t, err)
		assert.Equal(t, "multi-update", updated.Name)
		assert.Equal(t, "console.log('multi');", updated.Value)
		assert.False(t, updated.Test)
		assert.Equal(t, []string{"MULTI_SECRET"}, updated.Secrets)

		// Test updating non-existent source
		notFound, err := repo.Update(ctx, "550e8400-e29b-41d4-a716-446655440000", nameUpdate)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrSourceNotFound)
		assert.Nil(t, notFound)

		// Test validation errors
		invalidUpdate := model.UpdateSourceRequest{
			Name: testutil.StringPtr(""),
		}
		_, err = repo.Update(ctx, created.ID, invalidUpdate)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name cannot be empty")

		// Test no updates
		noUpdate := model.UpdateSourceRequest{}
		_, err = repo.Update(ctx, created.ID, noUpdate)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one field must be updated")
	})
}

func TestSourceRepo_Delete(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewSourceRepo(db)
		ctx := context.Background()

		// Create a source first
		req := &model.CreateSourceRequest{
			Name:  "delete-test",
			Value: "console.log('to be deleted');",
		}

		created, err := repo.Create(ctx, req)
		require.NoError(t, err)

		// Verify source exists
		found, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		require.NotNil(t, found)

		// Delete the source
		deleted, err := repo.Delete(ctx, created.ID)
		require.NoError(t, err)
		assert.True(t, deleted)

		// Verify source no longer exists
		notFound, err := repo.GetByID(ctx, created.ID)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrSourceNotFound)
		assert.Nil(t, notFound)

		// Try to delete non-existent source
		notDeleted, err := repo.Delete(ctx, "550e8400-e29b-41d4-a716-446655440000")
		require.NoError(t, err)
		assert.False(t, notDeleted)
	})
}

func TestSourceRepo_WithTimeProvider(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		mockTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
		timeProvider := NewFixedTimeProvider(mockTime)
		repo := NewSourceRepoWithTimeProvider(db, timeProvider)
		ctx := context.Background()

		req := &model.CreateSourceRequest{
			Name:  "time-test",
			Value: "console.log('time test');",
		}

		created, err := repo.Create(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, mockTime.Unix(), created.CreatedAt.Unix())
	})
}
