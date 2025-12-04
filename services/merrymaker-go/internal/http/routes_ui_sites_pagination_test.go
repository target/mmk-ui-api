package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

type fakeSitesSvc struct{}

func (f fakeSitesSvc) List(_ context.Context, limit, _ int) ([]*model.Site, error) {
	// Return exactly `limit` items so paginate() sees > pageSize and sets hasNext=true
	items := make([]*model.Site, 0, limit)
	for i := range limit {
		items = append(items, &model.Site{ID: "id-" + strconvI(i), Name: "site-" + strconvI(i)})
	}
	return items, nil
}

func (f fakeSitesSvc) GetByID(_ context.Context, _ string) (*model.Site, error) {
	return &model.Site{ID: "x", Name: "x"}, nil
}

func (f fakeSitesSvc) Create(_ context.Context, _ *model.CreateSiteRequest) (*model.Site, error) {
	return &model.Site{ID: "new"}, nil
}

func (f fakeSitesSvc) Update(_ context.Context, id string, _ model.UpdateSiteRequest) (*model.Site, error) {
	return &model.Site{ID: id}, nil
}
func (f fakeSitesSvc) Delete(_ context.Context, _ string) (bool, error) { return true, nil }

func buildSitesListHandler(t *testing.T, svc SitesService) http.Handler {
	mux := http.NewServeMux()
	ui := CreateUIHandlersForTest(t)
	if ui == nil {
		t.Fatal("cannot create UI handlers for test")
	}
	ui.SiteSvc = svc
	registerUISitesRoutes(mux, ui, uiRouteConfig{Auth: nil, CookieDomain: ""})
	return BrowserDetection()(&notFoundHandler{mux: mux, uiHandlers: ui})
}

func TestUIRoutes_Sites_PaginationQueryPersistence(t *testing.T) {
	h := buildSitesListHandler(t, fakeSitesSvc{})

	q := url.Values{}
	q.Set("q", "acme")
	q.Set("enabled", "true")
	q.Set("scope", "prod")
	q.Set("sort", "name")
	q.Set("dir", "desc")
	q.Set("page", "2")
	q.Set("page_size", "5")

	r := httptest.NewRequest(http.MethodGet, "/sites?"+q.Encode(), nil)
	r.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	// Prev URL should preserve filters and set page=1
	assert.Contains(t, body, "hx-get=\"/sites?")
	assert.Contains(t, body, "href=\"/sites?")
	assert.Contains(t, body, "q=acme")
	assert.Contains(t, body, "enabled=true")
	assert.Contains(t, body, "scope=prod")
	assert.Contains(t, body, "sort=name")
	assert.Contains(t, body, "dir=desc")
	assert.Contains(t, body, "page_size=5")
	assert.Contains(t, body, "page=1")
	// Next URL should preserve filters and set page=3
	assert.Contains(t, body, "page=3")
}

// strconvI is a tiny helper to avoid importing strconv for tests.
func strconvI(i int) string { return string('0' + rune(i%10)) }
