package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/ports"
	"github.com/target/mmk-ui-api/internal/service"
)

// fakeAllowlistSvc implements the minimal interface used by UI for edit form rendering.
type fakeAllowlistSvc struct{}

func (f fakeAllowlistSvc) List(
	ctx context.Context,
	opts model.DomainAllowlistListOptions,
) ([]*model.DomainAllowlist, error) {
	return nil, nil
}

func (f fakeAllowlistSvc) GetByID(ctx context.Context, id string) (*model.DomainAllowlist, error) {
	return &model.DomainAllowlist{
		ID:          id,
		Scope:       "global",
		Pattern:     "example.com",
		PatternType: model.PatternTypeExact,
		Description: "test",
		Enabled:     true,
		Priority:    100,
	}, nil
}

func (f fakeAllowlistSvc) Create(
	ctx context.Context,
	req *model.CreateDomainAllowlistRequest,
) (*model.DomainAllowlist, error) {
	return &model.DomainAllowlist{
		ID:          "new",
		Scope:       req.Scope,
		Pattern:     req.Pattern,
		PatternType: req.PatternType,
		Enabled:     true,
		Priority:    100,
	}, nil
}

func (f fakeAllowlistSvc) Update(
	ctx context.Context,
	id string,
	req model.UpdateDomainAllowlistRequest,
) (*model.DomainAllowlist, error) {
	return &model.DomainAllowlist{ID: id}, nil
}
func (f fakeAllowlistSvc) Delete(ctx context.Context, id string) error { return nil }
func (f fakeAllowlistSvc) Stats(ctx context.Context, siteID *string) (*model.DomainAllowlistStats, error) {
	return &model.DomainAllowlistStats{}, nil
}

func buildUIAllowlistTestHandler(t *testing.T, authSvc *service.AuthService, withFakeSvc bool) http.Handler {
	mux := http.NewServeMux()
	ui := CreateUIHandlersForTest(t)
	if ui == nil {
		t.Fatal("cannot create UI handlers for test")
	}
	if withFakeSvc {
		ui.AllowlistSvc = fakeAllowlistSvc{}
	}
	registerUIAllowlistRoutes(mux, ui, uiRouteConfig{Auth: authSvc, CookieDomain: ""})
	return BrowserDetection()(&notFoundHandler{mux: mux, uiHandlers: ui})
}

func TestUIAllowlistRoutes_AdminGating_UnauthenticatedRedirect(t *testing.T) {
	store := &testMemSessionStore{m: map[string]domainauth.Session{}}
	authSvc := service.NewAuthService(
		service.AuthServiceOptions{Provider: nil, Sessions: ports.SessionStore(store), Roles: nil},
	)

	h := buildUIAllowlistTestHandler(t, authSvc, false)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/allowlist/new", nil)
	r.Header.Set("Accept", "text/html")

	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusSeeOther, w.Code)
	loc := w.Header().Get("Location")
	assert.Contains(t, loc, "/auth/login")
	assert.Contains(t, loc, "redirect_uri=%2Fallowlist%2Fnew")
}

func TestUIAllowlistRoutes_AdminGating_ForbiddenForUser(t *testing.T) {
	store := &testMemSessionStore{m: map[string]domainauth.Session{}}
	authSvc := service.NewAuthService(
		service.AuthServiceOptions{Provider: nil, Sessions: ports.SessionStore(store), Roles: nil},
	)
	_ = store.Save(context.Background(), domainauth.Session{
		ID:        "user",
		UserID:    "user1",
		Email:     "u@example.com",
		Role:      domainauth.RoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	h := buildUIAllowlistTestHandler(t, authSvc, false)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/allowlist/new", nil)
	r.Header.Set("Accept", "text/html")
	r.AddCookie(&http.Cookie{Name: "session_id", Value: "user"})

	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Access Denied")
}

type adminOKConfig struct {
	Path        string
	SessionID   string
	WithFakeSvc bool
}

func doAdminOK(t *testing.T, cfg adminOKConfig) {
	store := &testMemSessionStore{m: map[string]domainauth.Session{}}
	authSvc := service.NewAuthService(
		service.AuthServiceOptions{Provider: nil, Sessions: ports.SessionStore(store), Roles: nil},
	)
	_ = store.Save(context.Background(), domainauth.Session{
		ID:        cfg.SessionID,
		UserID:    cfg.SessionID,
		Email:     cfg.SessionID + "@example.com",
		Role:      domainauth.RoleAdmin,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	h := buildUIAllowlistTestHandler(t, authSvc, cfg.WithFakeSvc)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, cfg.Path, nil)
	r.Header.Set("Accept", "text/html")
	r.AddCookie(&http.Cookie{Name: "session_id", Value: cfg.SessionID})

	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
}

func TestUIAllowlistRoutes_AdminGating_AdminOK_New(t *testing.T) {
	doAdminOK(t, adminOKConfig{
		Path:      "/allowlist/new",
		SessionID: "admin",
	})
}

func TestUIAllowlistRoutes_AdminGating_AdminOK_Edit(t *testing.T) {
	// Include fake allowlist service so edit can render 200
	doAdminOK(t, adminOKConfig{
		Path:        "/allowlist/abc123/edit",
		SessionID:   "admin2",
		WithFakeSvc: true,
	})
}
