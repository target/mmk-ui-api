package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCacheRepo implements the CacheRepository interface using Redis.
type RedisCacheRepo struct {
	client redis.UniversalClient
}

// NewRedisCacheRepo creates a new RedisCacheRepo with the given Redis client.
func NewRedisCacheRepo(client redis.UniversalClient) *RedisCacheRepo {
	return &RedisCacheRepo{client: client}
}

// Set stores a value in Redis with the given key and TTL.
func (r *RedisCacheRepo) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if key == "" {
		return errors.New("key cannot be empty")
	}

	return r.client.Set(ctx, key, value, ttl).Err()
}

// Get retrieves a value from Redis by key.
func (r *RedisCacheRepo) Get(ctx context.Context, key string) ([]byte, error) {
	if key == "" {
		return nil, errors.New("key cannot be empty")
	}

	result, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil // Key doesn't exist
		}
		return nil, fmt.Errorf("redis get: %w", err)
	}

	return []byte(result), nil
}

// Delete removes a key from Redis.
func (r *RedisCacheRepo) Delete(ctx context.Context, key string) (bool, error) {
	if key == "" {
		return false, errors.New("key cannot be empty")
	}

	result, err := r.client.Del(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis del: %w", err)
	}

	return result > 0, nil
}

// Exists checks if a key exists in Redis.
func (r *RedisCacheRepo) Exists(ctx context.Context, key string) (bool, error) {
	if key == "" {
		return false, errors.New("key cannot be empty")
	}

	result, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists: %w", err)
	}

	return result > 0, nil
}

// SetTTL updates the TTL for an existing key in Redis.
func (r *RedisCacheRepo) SetTTL(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if key == "" {
		return false, errors.New("key cannot be empty")
	}

	result, err := r.client.Expire(ctx, key, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("redis expire: %w", err)
	}

	return result, nil
}

// SetIfNotExists atomically sets a key only if it doesn't already exist.
// Uses Redis SET with NX and TTL options for guaranteed atomicity.
func (r *RedisCacheRepo) SetIfNotExists(
	ctx context.Context,
	key string,
	value []byte,
	ttl time.Duration,
) (bool, error) {
	if key == "" {
		return false, errors.New("key cannot be empty")
	}

	// Use SET with NX (only if not exists) and TTL for atomic operation
	// This is guaranteed to be atomic in Redis and works across all Redis versions
	actualTTL := ttl
	if ttl <= 0 {
		actualTTL = time.Second // Minimum TTL of 1 second
	}

	// Important: SETNX with expiration is not atomic (it performs EXPIRE separately).
	// We must use SET with NX + TTL atomically to avoid race conditions under concurrency.
	cmd := r.client.SetArgs(ctx, key, value, redis.SetArgs{Mode: "NX", TTL: actualTTL})
	status, err := cmd.Result()
	if err != nil {
		// When NX condition is not met (key exists), Redis returns a nil reply.
		// go-redis represents this as redis.Nil; treat it as "was not set" not an error.
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, fmt.Errorf("redis SET NX: %w", err)
	}

	// When the key is set, Redis returns "OK"; when it already exists, empty string (handled above).
	return status == "OK", nil
}

// Health checks the health of the Redis connection.
func (r *RedisCacheRepo) Health(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// RedisConfig holds configuration for Redis connection.
type RedisConfig struct {
	Addr     string `json:"addr"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

// DefaultRedisConfig returns a RedisConfig with sensible defaults.
func DefaultRedisConfig() RedisConfig {
	return RedisConfig{
		Addr:     "localhost:6379",
		Password: "", // No password by default
		DB:       0,  // Default DB
	}
}

// NewRedisClient creates a new Redis client with the given configuration.
func NewRedisClient(cfg RedisConfig) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}
