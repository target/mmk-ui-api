package redis

import (
	"context"
	"testing"
	"time"

	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestRedis creates a Redis client for testing.
// Tests will be skipped if Redis is not available.
func setupTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	return testutil.SetupTestRedis(t)
}

func TestSessionStore_SaveAndGet(t *testing.T) {
	client := setupTestRedis(t)
	defer client.Close()

	store := NewSessionStore(client)
	ctx := context.Background()

	session := domainauth.Session{
		ID:        "test-session-1",
		UserID:    "user-123",
		Email:     "user@example.com",
		Role:      domainauth.RoleUser,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}

	// Save session
	err := store.Save(ctx, session)
	require.NoError(t, err)

	// Get session
	retrieved, err := store.Get(ctx, "test-session-1")
	require.NoError(t, err)
	assert.Equal(t, session.ID, retrieved.ID)
	assert.Equal(t, session.UserID, retrieved.UserID)
	assert.Equal(t, session.Email, retrieved.Email)
	assert.Equal(t, session.Role, retrieved.Role)
	assert.WithinDuration(t, session.ExpiresAt, retrieved.ExpiresAt, time.Second)
}

func TestSessionStore_GetNonExistent(t *testing.T) {
	client := setupTestRedis(t)
	defer client.Close()

	store := NewSessionStore(client)
	ctx := context.Background()

	_, err := store.Get(ctx, "non-existent")
	assert.Equal(t, ErrNotFound, err)
}

func TestSessionStore_Delete(t *testing.T) {
	client := setupTestRedis(t)
	defer client.Close()

	store := NewSessionStore(client)
	ctx := context.Background()

	session := domainauth.Session{
		ID:        "test-session-delete",
		UserID:    "user-123",
		Email:     "user@example.com",
		Role:      domainauth.RoleUser,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}

	// Save session
	err := store.Save(ctx, session)
	require.NoError(t, err)

	// Verify it exists
	_, err = store.Get(ctx, "test-session-delete")
	require.NoError(t, err)

	// Delete session
	err = store.Delete(ctx, "test-session-delete")
	require.NoError(t, err)

	// Verify it's gone
	_, err = store.Get(ctx, "test-session-delete")
	assert.Equal(t, ErrNotFound, err)
}

func TestSessionStore_TTLExpiration(t *testing.T) {
	client := setupTestRedis(t)
	defer client.Close()

	store := NewSessionStore(client)
	ctx := context.Background()

	// Create session with very short TTL
	session := domainauth.Session{
		ID:        "test-session-ttl",
		UserID:    "user-123",
		Email:     "user@example.com",
		Role:      domainauth.RoleUser,
		ExpiresAt: time.Now().Add(100 * time.Millisecond),
	}

	// Save session
	err := store.Save(ctx, session)
	require.NoError(t, err)

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)

	// Should be expired
	_, err = store.Get(ctx, "test-session-ttl")
	assert.Equal(t, ErrNotFound, err)
}

func TestSessionStore_CustomPrefix(t *testing.T) {
	client := setupTestRedis(t)
	defer client.Close()

	store := NewSessionStoreWithPrefix(client, "test-prefix:")
	ctx := context.Background()

	session := domainauth.Session{
		ID:        "prefix-test",
		UserID:    "user-123",
		Email:     "user@example.com",
		Role:      domainauth.RoleUser,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}

	// Save session
	err := store.Save(ctx, session)
	require.NoError(t, err)

	// Verify it was stored with the custom prefix
	exists := client.Exists(ctx, "test-prefix:prefix-test").Val()
	assert.Equal(t, int64(1), exists)

	// Get session should work normally
	retrieved, err := store.Get(ctx, "prefix-test")
	require.NoError(t, err)
	assert.Equal(t, session.ID, retrieved.ID)
}

func TestSessionStore_SaveEmptyID(t *testing.T) {
	client := setupTestRedis(t)
	defer client.Close()

	store := NewSessionStore(client)
	ctx := context.Background()

	session := domainauth.Session{
		ID:        "", // Empty ID should cause error
		UserID:    "user-123",
		Email:     "user@example.com",
		Role:      domainauth.RoleUser,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}

	err := store.Save(ctx, session)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session ID cannot be empty")
}

func TestSessionStore_SaveExpiredSession(t *testing.T) {
	client := setupTestRedis(t)
	defer client.Close()

	store := NewSessionStore(client)
	ctx := context.Background()

	session := domainauth.Session{
		ID:        "expired-session",
		UserID:    "user-123",
		Email:     "user@example.com",
		Role:      domainauth.RoleUser,
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Already expired
	}

	err := store.Save(ctx, session)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session is expired")
}

func TestSessionStore_GetEmptyID(t *testing.T) {
	client := setupTestRedis(t)
	defer client.Close()

	store := NewSessionStore(client)
	ctx := context.Background()

	_, err := store.Get(ctx, "")
	assert.Equal(t, ErrNotFound, err)
}
