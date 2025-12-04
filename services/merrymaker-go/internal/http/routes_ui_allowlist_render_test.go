package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/stretchr/testify/assert"
)

type fakeAllowlistSvcList struct{ items []*model.DomainAllowlist }

func (f fakeAllowlistSvcList) List(
	_ context.Context,
	_ model.DomainAllowlistListOptions,
) ([]*model.DomainAllowlist, error) {
	return f.items, nil
}

func (f fakeAllowlistSvcList) GetByID(_ context.Context, _ string) (*model.DomainAllowlist, error) {
	return &model.DomainAllowlist{
		ID:          "x",
		Scope:       "global",
		Pattern:     "x",
		PatternType: model.PatternTypeExact,
		Enabled:     true,
		Priority:    100,
	}, nil
}

func (f fakeAllowlistSvcList) Create(
	_ context.Context,
	_ *model.CreateDomainAllowlistRequest,
) (*model.DomainAllowlist, error) {
	return &model.DomainAllowlist{ID: "new"}, nil
}

func (f fakeAllowlistSvcList) Update(
	_ context.Context,
	id string,
	_ model.UpdateDomainAllowlistRequest,
) (*model.DomainAllowlist, error) {
	return &model.DomainAllowlist{ID: id}, nil
}
func (f fakeAllowlistSvcList) Delete(_ context.Context, _ string) error { return nil }
func (f fakeAllowlistSvcList) Stats(_ context.Context, _ *string) (*model.DomainAllowlistStats, error) {
	return &model.DomainAllowlistStats{}, nil
}

func buildAllowlistRenderHandler(t *testing.T, svc DomainAllowlistsService) http.Handler {
	mux := http.NewServeMux()
	ui := CreateUIHandlersForTest(t)
	if ui == nil {
		t.Fatal("cannot create UI handlers for test")
	}
	ui.AllowlistSvc = svc
	registerUIAllowlistRoutes(mux, ui, uiRouteConfig{Auth: nil, CookieDomain: ""})
	return BrowserDetection()(&notFoundHandler{mux: mux, uiHandlers: ui})
}

func TestUIAllowlistRoutes_List_ErrorBanner_WithoutService(t *testing.T) {
	h := buildAllowlistRenderHandler(t, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/allowlist", nil)
	r.Header.Set("Accept", "text/html")

	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "Allow List")
	assert.Contains(t, body, "Unable to load allow list.")
}

func TestUIAllowlistRoutes_List_RendersWithItems(t *testing.T) {
	item := &model.DomainAllowlist{
		ID:          "1",
		Scope:       "global",
		Pattern:     "render-test.com",
		PatternType: model.PatternTypeExact,
		Enabled:     true,
		Priority:    100,
	}
	h := buildAllowlistRenderHandler(t, fakeAllowlistSvcList{items: []*model.DomainAllowlist{item}})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/allowlist", nil)
	r.Header.Set("Accept", "text/html")

	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "Allow List")
	assert.Contains(t, body, "render-test.com")
}
