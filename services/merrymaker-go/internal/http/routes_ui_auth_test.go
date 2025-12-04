package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	"github.com/target/mmk-ui-api/internal/ports"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/stretchr/testify/assert"
)

var errSessionNotFound = errors.New("session not found")

// Minimal in-memory session store for AuthService.
type testMemSessionStore struct{ m map[string]domainauth.Session }

func (s *testMemSessionStore) Save(_ context.Context, sess domainauth.Session) error {
	if s.m == nil {
		s.m = map[string]domainauth.Session{}
	}
	s.m[sess.ID] = sess
	return nil
}

func (s *testMemSessionStore) Get(_ context.Context, id string) (domainauth.Session, error) {
	sess, ok := s.m[id]
	if !ok {
		return domainauth.Session{}, errSessionNotFound
	}
	return sess, nil
}
func (s *testMemSessionStore) Delete(_ context.Context, id string) error { delete(s.m, id); return nil }

func TestUIRoutes_RequireAuth_UnauthenticatedRedirect(t *testing.T) {
	// Build AuthService so UI routes are protected
	store := &testMemSessionStore{m: map[string]domainauth.Session{}}
	authSvc := service.NewAuthService(
		service.AuthServiceOptions{Provider: nil, Sessions: ports.SessionStore(store), Roles: nil},
	)

	// Build router like other UI integration tests using explicit template path
	mux := http.NewServeMux()
	uiHandlers := CreateUIHandlersForTest(t)
	if uiHandlers == nil {
		t.Fatal("cannot create UI handlers for test")
	}
	registerUIRoutes(mux, uiHandlers, uiRouteConfig{Auth: authSvc, CookieDomain: ""})
	// Wrap with NotFound and browser detection like production
	h := BrowserDetection()(&notFoundHandler{mux: mux, uiHandlers: uiHandlers})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	r.Header.Set("Accept", "text/html")

	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusSeeOther, w.Code)
	loc := w.Header().Get("Location")
	assert.Contains(t, loc, "/auth/login")
	assert.Contains(t, loc, "redirect_uri=%2Fdashboard")
}

func TestUIRoutes_RequireAuth_AuthenticatedOK(t *testing.T) {
	// Build AuthService with a valid session
	store := &testMemSessionStore{m: map[string]domainauth.Session{}}
	authSvc := service.NewAuthService(
		service.AuthServiceOptions{Provider: nil, Sessions: ports.SessionStore(store), Roles: nil},
	)
	_ = store.Save(context.Background(), domainauth.Session{
		ID:        "sess1",
		UserID:    "user1",
		Email:     "u@example.com",
		Role:      domainauth.RoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	// Build router like other UI integration tests using explicit template path
	mux := http.NewServeMux()
	uiHandlers := CreateUIHandlersForTest(t)
	if uiHandlers == nil {
		t.Fatal("cannot create UI handlers for test")
	}
	registerUIRoutes(mux, uiHandlers, uiRouteConfig{Auth: authSvc, CookieDomain: ""})
	// Wrap with NotFound and browser detection like production
	h := BrowserDetection()(&notFoundHandler{mux: mux, uiHandlers: uiHandlers})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept", "text/html")
	r.AddCookie(&http.Cookie{Name: "session_id", Value: "sess1"})

	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
}
