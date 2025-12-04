package httpx

import (
	"context"
	"testing"

	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	"github.com/stretchr/testify/assert"
)

func TestGetUserSessionFromContext(t *testing.T) {
	// No session
	if s, ok := GetUserSessionFromContext(context.Background()); assert.False(t, ok) {
		assert.Nil(t, s)
	}

	// With session
	sess := &domainauth.Session{ID: "abc", Role: domainauth.RoleUser}
	ctx := SetSessionInContext(context.Background(), sess)
	s, ok := GetUserSessionFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, sess, s)
}

func TestIsGuestUser(t *testing.T) {
	// No session => guest
	assert.True(t, IsGuestUser(context.Background()))

	// Guest role => guest
	guest := &domainauth.Session{ID: "g", Role: domainauth.RoleGuest}
	assert.True(t, IsGuestUser(SetSessionInContext(context.Background(), guest)))

	// User/Admin => not guest
	user := &domainauth.Session{ID: "u", Role: domainauth.RoleUser}
	admin := &domainauth.Session{ID: "a", Role: domainauth.RoleAdmin}
	assert.False(t, IsGuestUser(SetSessionInContext(context.Background(), user)))
	assert.False(t, IsGuestUser(SetSessionInContext(context.Background(), admin)))
}
