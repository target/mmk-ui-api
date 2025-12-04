package data

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/testutil"
)

// setupTestRedis creates a Redis client for testing.
// Tests will be skipped if Redis is not available.
func setupTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	return testutil.SetupTestRedis(t)
}

func TestRedisCacheRepo_Set_Get_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Setup Redis client for testing
	client := setupTestRedis(t)
	defer client.Close()

	repo := NewRedisCacheRepo(client)
	ctx := context.Background()

	t.Run("set and get", func(t *testing.T) {
		key := "test:key:1"
		value := []byte("test value")
		ttl := 5 * time.Minute

		// Set value
		err := repo.Set(ctx, key, value, ttl)
		require.NoError(t, err)

		// Get value
		result, err := repo.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, result)

		// Check TTL is set
		actualTTL := client.TTL(ctx, key).Val()
		assert.True(t, actualTTL > 0 && actualTTL <= ttl)
	})

	t.Run("get non-existent key", func(t *testing.T) {
		result, err := repo.Get(ctx, "non:existent:key")
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("delete existing key", func(t *testing.T) {
		key := "test:key:2"
		value := []byte("to be deleted")

		// Set value first
		err := repo.Set(ctx, key, value, time.Minute)
		require.NoError(t, err)

		// Delete it
		deleted, err := repo.Delete(ctx, key)
		require.NoError(t, err)
		assert.True(t, deleted)

		// Verify it's gone
		result, err := repo.Get(ctx, key)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("delete non-existent key", func(t *testing.T) {
		deleted, err := repo.Delete(ctx, "non:existent:key")
		require.NoError(t, err)
		assert.False(t, deleted)
	})

	t.Run("exists", func(t *testing.T) {
		key := "test:key:3"
		value := []byte("exists test")

		// Key should not exist initially
		exists, err := repo.Exists(ctx, key)
		require.NoError(t, err)
		assert.False(t, exists)

		// Set value
		err = repo.Set(ctx, key, value, time.Minute)
		require.NoError(t, err)

		// Key should exist now
		exists, err = repo.Exists(ctx, key)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("set TTL", func(t *testing.T) {
		key := "test:key:4"
		value := []byte("ttl test")

		// Set value with initial TTL
		err := repo.Set(ctx, key, value, time.Minute)
		require.NoError(t, err)

		// Update TTL
		updated, err := repo.SetTTL(ctx, key, 2*time.Minute)
		require.NoError(t, err)
		assert.True(t, updated)

		// Check new TTL
		actualTTL := client.TTL(ctx, key).Val()
		assert.True(t, actualTTL > time.Minute && actualTTL <= 2*time.Minute)
	})

	t.Run("set TTL on non-existent key", func(t *testing.T) {
		updated, err := repo.SetTTL(ctx, "non:existent:key", time.Minute)
		require.NoError(t, err)
		assert.False(t, updated)
	})

	t.Run("set if not exists - new key", func(t *testing.T) {
		key := "test:key:5"
		value := []byte("setnx test")
		ttl := time.Minute

		// Key should not exist initially
		exists, err := repo.Exists(ctx, key)
		require.NoError(t, err)
		assert.False(t, exists)

		// SetIfNotExists should succeed
		wasSet, err := repo.SetIfNotExists(ctx, key, value, ttl)
		require.NoError(t, err)
		assert.True(t, wasSet)

		// Key should exist now with correct value
		result, err := repo.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, result)

		// Check TTL is set
		actualTTL := client.TTL(ctx, key).Val()
		assert.True(t, actualTTL > 0 && actualTTL <= ttl)
	})

	t.Run("set if not exists - existing key", func(t *testing.T) {
		key := "test:key:6"
		originalValue := []byte("original value")
		newValue := []byte("new value")
		ttl := time.Minute

		// Set original value first
		err := repo.Set(ctx, key, originalValue, ttl)
		require.NoError(t, err)

		// SetIfNotExists should fail (key exists)
		wasSet, err := repo.SetIfNotExists(ctx, key, newValue, ttl)
		require.NoError(t, err)
		assert.False(t, wasSet)

		// Original value should be unchanged
		result, err := repo.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, originalValue, result)
	})

	t.Run("health check", func(t *testing.T) {
		err := repo.Health(ctx)
		assert.NoError(t, err)
	})
}

func TestRedisCacheRepo_Validation(t *testing.T) {
	// Note: This test only validates input parameters and doesn't actually connect to Redis
	// since validation errors occur before any Redis operations
	client := setupTestRedis(t)
	defer client.Close()

	repo := NewRedisCacheRepo(client)
	ctx := context.Background()

	t.Run("empty key validation", func(t *testing.T) {
		// Set with empty key
		err := repo.Set(ctx, "", []byte("value"), time.Minute)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key cannot be empty")

		// Get with empty key
		_, err = repo.Get(ctx, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key cannot be empty")

		// Delete with empty key
		_, err = repo.Delete(ctx, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key cannot be empty")

		// Exists with empty key
		_, err = repo.Exists(ctx, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key cannot be empty")

		// SetTTL with empty key
		_, err = repo.SetTTL(ctx, "", time.Minute)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key cannot be empty")

		// SetIfNotExists with empty key
		_, err = repo.SetIfNotExists(ctx, "", []byte("value"), time.Minute)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key cannot be empty")
	})
}

func TestDefaultRedisConfig(t *testing.T) {
	cfg := DefaultRedisConfig()
	assert.Equal(t, "localhost:6379", cfg.Addr)
	assert.Empty(t, cfg.Password)
	assert.Equal(t, 0, cfg.DB)
}

func TestNewRedisClient(t *testing.T) {
	cfg := RedisConfig{
		Addr:     "localhost:6379",
		Password: "test-password",
		DB:       2,
	}

	client := NewRedisClient(cfg)
	assert.NotNil(t, client)

	// Check that options are set correctly
	opts := client.Options()
	assert.Equal(t, cfg.Addr, opts.Addr)
	assert.Equal(t, cfg.Password, opts.Password)
	assert.Equal(t, cfg.DB, opts.DB)

	client.Close()
}
