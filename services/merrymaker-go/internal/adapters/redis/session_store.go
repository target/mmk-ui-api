package redis

// Package redis provides Redis-based adapters for the merrymaker system.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
)

// SessionStore is a Redis-based session store for production use.
// It handles TTL semantics automatically based on session ExpiresAt.
type SessionStore struct {
	client redis.UniversalClient
	prefix string
}

// NewSessionStore creates a new Redis-based session store.
func NewSessionStore(client redis.UniversalClient) *SessionStore {
	return &SessionStore{
		client: client,
		prefix: "session:",
	}
}

// NewSessionStoreWithPrefix creates a Redis session store with a custom key prefix.
func NewSessionStoreWithPrefix(client redis.UniversalClient, prefix string) *SessionStore {
	return &SessionStore{
		client: client,
		prefix: prefix,
	}
}

func (s *SessionStore) Save(ctx context.Context, sess domainauth.Session) error {
	if sess.ID == "" {
		return errors.New("session ID cannot be empty")
	}

	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	key := s.prefix + sess.ID
	ttl := time.Until(sess.ExpiresAt)
	if ttl <= 0 {
		// Session is already expired, don't save it
		return errors.New("session is expired")
	}

	return s.client.Set(ctx, key, data, ttl).Err()
}

func (s *SessionStore) Get(ctx context.Context, id string) (domainauth.Session, error) {
	if id == "" {
		return domainauth.Session{}, ErrNotFound
	}

	key := s.prefix + id
	data, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return domainauth.Session{}, ErrNotFound
		}
		return domainauth.Session{}, fmt.Errorf("redis get: %w", err)
	}

	var sess domainauth.Session
	if unmarshalErr := json.Unmarshal([]byte(data), &sess); unmarshalErr != nil {
		return domainauth.Session{}, fmt.Errorf("unmarshal session: %w", unmarshalErr)
	}

	// Double-check expiration (Redis TTL should handle this, but be defensive)
	if time.Now().After(sess.ExpiresAt) {
		// Clean up expired session; if cleanup fails bubble the error up.
		if deleteErr := s.Delete(ctx, id); deleteErr != nil {
			return domainauth.Session{}, fmt.Errorf("cleanup expired session: %w", deleteErr)
		}
		return domainauth.Session{}, ErrNotFound
	}

	return sess, nil
}

func (s *SessionStore) Delete(ctx context.Context, id string) error {
	if id == "" {
		return nil // Nothing to delete
	}

	key := s.prefix + id
	return s.client.Del(ctx, key).Err()
}

// ErrNotFound is returned when a session is not found.
type notFoundError struct{}

func (notFoundError) Error() string { return "session not found" }

var ErrNotFound error = notFoundError{}
