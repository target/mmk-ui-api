package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/mocks"
	"github.com/target/mmk-ui-api/internal/ports"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// memSessionStore is a minimal in-memory SessionStore for tests.
type memSessionStore struct{ m map[string]domainauth.Session }

func (s *memSessionStore) Save(_ context.Context, sess domainauth.Session) error {
	if s.m == nil {
		s.m = map[string]domainauth.Session{}
	}
	s.m[sess.ID] = sess
	return nil
}

func (s *memSessionStore) Get(_ context.Context, id string) (domainauth.Session, error) {
	sess, ok := s.m[id]
	if !ok {
		return domainauth.Session{}, errors.New("not found")
	}
	return sess, nil
}
func (s *memSessionStore) Delete(_ context.Context, id string) error { delete(s.m, id); return nil }

func TestRouter_AdminProtectedSecretsRoute(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockSecretRepository(ctrl)
	// For the authorized case, List should be called and return an empty slice
	repo.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return([]*model.Secret{}, nil).AnyTimes()
	secretSvc := service.MustNewSecretService(service.SecretServiceOptions{Repo: repo})

	// Build an AuthService with an in-memory session store containing an admin session
	store := &memSessionStore{m: map[string]domainauth.Session{}}
	authSvc := service.NewAuthService(service.AuthServiceOptions{
		Provider: nil,
		Sessions: ports.SessionStore(store),
		Roles:    nil,
	})
	_ = store.Save(context.Background(), domainauth.Session{
		ID:        "admin",
		UserID:    "admin-user",
		Email:     "admin@example.com",
		Role:      domainauth.RoleAdmin,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	mux := NewRouter(RouterServices{Secrets: secretSvc, Auth: authSvc})

	t.Run("unauthenticated -> 401", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/secrets", nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("admin session -> 200", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/secrets", nil)
		r.AddCookie(&http.Cookie{Name: "session_id", Value: "admin"})
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		_, hasSecrets := resp["secrets"]
		assert.True(t, hasSecrets)
	})
}
