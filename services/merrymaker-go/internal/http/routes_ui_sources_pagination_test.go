package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/stretchr/testify/assert"
)

type fakeSourcesSvc struct{}

func (f fakeSourcesSvc) List(_ context.Context, limit, _ int) ([]*model.Source, error) {
	items := make([]*model.Source, 0, limit)
	now := time.Now()
	for i := range limit {
		items = append(
			items,
			&model.Source{
				ID:        "id-" + strconvI(i),
				Name:      "src-" + strconvI(i),
				Value:     "console.log('x')",
				CreatedAt: now,
			},
		)
	}
	return items, nil
}

func (f fakeSourcesSvc) GetByID(_ context.Context, _ string) (*model.Source, error) {
	return &model.Source{ID: "x", Name: "x"}, nil
}

func (f fakeSourcesSvc) Create(_ context.Context, _ *model.CreateSourceRequest) (*model.Source, error) {
	return &model.Source{ID: "new"}, nil
}
func (f fakeSourcesSvc) Delete(_ context.Context, _ string) (bool, error) { return true, nil }
func (f fakeSourcesSvc) ResolveScript(_ context.Context, src *model.Source) (string, error) {
	if src == nil {
		return "", nil
	}
	return src.Value, nil
}

func (f fakeSourcesSvc) CountJobsBySource(_ context.Context, _ string, _ bool) (int, error) {
	return 0, nil
}

func (f fakeSourcesSvc) CountBrowserJobsBySource(_ context.Context, _ string, _ bool) (int, error) {
	return 0, nil
}

func buildSourcesListHandler(t *testing.T, svc SourcesService) http.Handler {
	mux := http.NewServeMux()
	ui := CreateUIHandlersForTest(t)
	if ui == nil {
		t.Fatal("cannot create UI handlers for test")
	}
	ui.SourceSvc = svc
	registerUISourcesRoutes(mux, ui, uiRouteConfig{Auth: nil, CookieDomain: ""})
	return BrowserDetection()(&notFoundHandler{mux: mux, uiHandlers: ui})
}

func TestUIRoutes_Sources_PaginationQueryPersistence(t *testing.T) {
	h := buildSourcesListHandler(t, fakeSourcesSvc{})

	q := url.Values{}
	q.Set("q", "test-src")
	q.Set("include_tests", "true")
	q.Set("page", "2")
	q.Set("page_size", "5")

	r := httptest.NewRequest(http.MethodGet, "/sources?"+q.Encode(), nil)
	r.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	// Prev URL should preserve filters and set page=1
	assert.Contains(t, body, "hx-get=\"/sources?")
	assert.Contains(t, body, "href=\"/sources?")
	assert.Contains(t, body, "q=test-src")
	assert.Contains(t, body, "include_tests=true")
	assert.Contains(t, body, "page_size=5")
	assert.Contains(t, body, "page=1")
	// Next URL should preserve filters and set page=3
	assert.Contains(t, body, "page=3")
}
