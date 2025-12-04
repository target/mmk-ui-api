package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test data structures.
type testItem struct {
	ID   string
	Name string
}

type testFilter struct {
	Query   string
	Enabled bool
}

// Mock fetchers for testing.
func mockSimpleFetcher(items []testItem, returnErr error) ListFetcher[testItem] {
	return func(_ context.Context, pg pageOpts) ([]testItem, error) {
		if returnErr != nil {
			return nil, returnErr
		}

		// Simulate pagination - fetch pageSize+1 to detect hasNext
		limit, offset := pg.LimitAndOffset()
		start := offset
		end := offset + limit
		if start >= len(items) {
			return []testItem{}, nil
		}
		if end > len(items) {
			end = len(items)
		}

		return items[start:end], nil
	}
}

func mockFilteredFetcher(items []testItem, returnErr error) FilteredFetcher[testItem, testFilter] {
	return func(_ context.Context, f testFilter, pg pageOpts) ([]testItem, error) {
		if returnErr != nil {
			return nil, returnErr
		}

		// Apply filter
		var filtered []testItem
		for _, item := range items {
			if f.Query == "" || item.Name == f.Query {
				if !f.Enabled || item.ID != "disabled" {
					filtered = append(filtered, item)
				}
			}
		}

		// Simulate pagination - fetch pageSize+1 to detect hasNext
		limit, offset := pg.LimitAndOffset()
		start := offset
		end := offset + limit
		if start >= len(filtered) {
			return []testItem{}, nil
		}
		if end > len(filtered) {
			end = len(filtered)
		}

		return filtered[start:end], nil
	}
}

func mockFilterParser(q url.Values) (testFilter, error) {
	return testFilter{
		Query:   q.Get("q"),
		Enabled: q.Get("enabled") == "true",
	}, nil
}

func mockFilterParserWithError(_ url.Values) (testFilter, error) {
	return testFilter{}, errors.New("invalid filter format")
}

// testSetup contains common test data and setup.
type testSetup struct {
	items   []testItem
	handler *UIHandlers
}

// newTestSetup creates a common test setup.
func newTestSetup(t *testing.T) *testSetup {
	return &testSetup{
		items: []testItem{
			{ID: "1", Name: "item1"},
			{ID: "2", Name: "item2"},
			{ID: "3", Name: "item3"},
		},
		handler: CreateUIHandlersForTest(t),
	}
}

func TestHandleList_SimpleFetcher_FirstPage(t *testing.T) {
	setup := newTestSetup(t)
	require.NotNil(t, setup.handler)

	// Create request
	r := httptest.NewRequest(http.MethodGet, "/test?page=1&page_size=2", nil)
	w := httptest.NewRecorder()

	// Call HandleList - use struct{} for F when no filtering
	HandleList(ListHandlerOpts[testItem, struct{}]{
		Handler:      setup.handler,
		W:            w,
		R:            r,
		Fetcher:      mockSimpleFetcher(setup.items, nil),
		BasePath:     "/test",
		PageMeta:     PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: PageSecrets},
		ItemsKey:     "Items",
		ErrorMessage: "Unable to load items.",
	})

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// Should contain pagination info for first page
	assert.Contains(t, body, "Showing 1–2")
	assert.Contains(t, body, "page=2")                             // Next page link
	assert.Contains(t, body, `aria-disabled="true" tabindex="-1"`) // Disabled prev button on first page
	assert.Contains(t, body, `<span>Prev</span>`)                  // Prev button text
}

func TestHandleList_SimpleFetcher_MiddlePage(t *testing.T) {
	// Create test data with enough items for multiple pages
	items := make([]testItem, 10)
	for i := range items {
		items[i] = testItem{ID: string(rune('1' + i)), Name: "item" + string(rune('1'+i))}
	}

	handler := CreateUIHandlersForTest(t)
	require.NotNil(t, handler)

	r := httptest.NewRequest(http.MethodGet, "/test?page=2&page_size=3", nil)
	w := httptest.NewRecorder()

	HandleList(ListHandlerOpts[testItem, struct{}]{
		Handler:      handler,
		W:            w,
		R:            r,
		Fetcher:      mockSimpleFetcher(items, nil),
		BasePath:     "/test",
		PageMeta:     PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: PageSecrets},
		ItemsKey:     "Items",
		ErrorMessage: "Unable to load items.",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// Should have both prev and next links
	assert.Contains(t, body, "page=1")      // Prev page link
	assert.Contains(t, body, "page=3")      // Next page link
	assert.Contains(t, body, "Showing 4–6") // Correct range for page 2
}

func TestHandleList_SimpleFetcher_LastPage(t *testing.T) {
	setup := newTestSetup(t)
	require.NotNil(t, setup.handler)

	r := httptest.NewRequest(http.MethodGet, "/test?page=2&page_size=2", nil)
	w := httptest.NewRecorder()

	HandleList(ListHandlerOpts[testItem, struct{}]{
		Handler:      setup.handler,
		W:            w,
		R:            r,
		Fetcher:      mockSimpleFetcher(setup.items, nil),
		BasePath:     "/test",
		PageMeta:     PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: PageSecrets},
		ItemsKey:     "Items",
		ErrorMessage: "Unable to load items.",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// Should have prev link but no next link
	assert.Contains(t, body, "page=1")      // Prev page link
	assert.NotContains(t, body, "page=3")   // No next page
	assert.Contains(t, body, "Showing 3–3") // Last item only
}

func TestHandleList_FilteredFetcher_WithFilters(t *testing.T) {
	items := []testItem{
		{ID: "1", Name: "alpha"},
		{ID: "2", Name: "beta"},
		{ID: "disabled", Name: "gamma"},
	}

	handler := CreateUIHandlersForTest(t)
	require.NotNil(t, handler)

	r := httptest.NewRequest(http.MethodGet, "/test?q=alpha&enabled=true&page=1&page_size=10", nil)
	w := httptest.NewRecorder()

	HandleList(ListHandlerOpts[testItem, testFilter]{
		Handler:         handler,
		W:               w,
		R:               r,
		FilteredFetcher: mockFilteredFetcher(items, nil),
		FilterParser:    mockFilterParser,
		BasePath:        "/test",
		PageMeta:        PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: PageSecrets},
		ItemsKey:        "Items",
		ErrorMessage:    "Unable to load items.",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	// The filtered fetcher should only return items matching the filter
}

func TestHandleList_EmptyResults(t *testing.T) {
	handler := CreateUIHandlersForTest(t)
	require.NotNil(t, handler)

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	HandleList(ListHandlerOpts[testItem, struct{}]{
		Handler:      handler,
		W:            w,
		R:            r,
		Fetcher:      mockSimpleFetcher([]testItem{}, nil),
		BasePath:     "/test",
		PageMeta:     PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: PageSecrets},
		ItemsKey:     "Items",
		ErrorMessage: "Unable to load items.",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// Should show empty state with disabled pagination
	assert.Contains(t, body, "&nbsp;")                              // Empty pagination info
	assert.Contains(t, body, `aria-disabled="true" tabindex="-1">`) // Disabled buttons
	assert.Contains(t, body, `<span>Prev</span>`)                   // Prev button text
	assert.Contains(t, body, `<span>Next</span>`)                   // Next button text
}

func TestHandleList_ErrorHandling(t *testing.T) {
	handler := CreateUIHandlersForTest(t)
	require.NotNil(t, handler)

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	HandleList(ListHandlerOpts[testItem, struct{}]{
		Handler:      handler,
		W:            w,
		R:            r,
		Fetcher:      mockSimpleFetcher(nil, errors.New("database error")),
		BasePath:     "/test",
		PageMeta:     PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: PageSecrets},
		ItemsKey:     "Items",
		ErrorMessage: "Unable to load items.",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// Should show error message
	assert.Contains(t, body, "Unable to load items.")
}

func TestHandleList_QueryParamPreservation(t *testing.T) {
	items := make([]testItem, 10)
	for i := range items {
		items[i] = testItem{ID: string(rune('1' + i)), Name: "item" + string(rune('1'+i))}
	}

	handler := CreateUIHandlersForTest(t)
	require.NotNil(t, handler)

	// Request with filters and pagination
	r := httptest.NewRequest(http.MethodGet, "/test?q=search&enabled=true&page=2&page_size=3", nil)
	w := httptest.NewRecorder()

	HandleList(ListHandlerOpts[testItem, testFilter]{
		Handler:         handler,
		W:               w,
		R:               r,
		FilteredFetcher: mockFilteredFetcher(items, nil),
		FilterParser:    mockFilterParser,
		BasePath:        "/test",
		PageMeta:        PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: PageSecrets},
		ItemsKey:        "Items",
		ErrorMessage:    "Unable to load items.",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// Pagination URLs should preserve filter params
	assert.Contains(t, body, "q=search")
	assert.Contains(t, body, "enabled=true")
	assert.Contains(t, body, "page_size=3")
}

func TestHandleList_NoFetcherProvided(t *testing.T) {
	handler := CreateUIHandlersForTest(t)
	require.NotNil(t, handler)

	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	HandleList(ListHandlerOpts[testItem, struct{}]{
		Handler: handler,
		W:       w,
		R:       r,
		// No Fetcher or FilteredFetcher provided
		BasePath:     "/test",
		PageMeta:     PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: PageSecrets},
		ItemsKey:     "Items",
		ErrorMessage: "Unable to load items.",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// Should show configuration error
	assert.Contains(t, body, "No data fetcher configured.")
}

func TestHandleList_FilterParsingError(t *testing.T) {
	handler := CreateUIHandlersForTest(t)
	require.NotNil(t, handler)

	items := []testItem{
		{ID: "1", Name: "item1"},
		{ID: "2", Name: "item2"},
	}

	r := httptest.NewRequest(http.MethodGet, "/test?invalid=param", nil)
	w := httptest.NewRecorder()

	HandleList(ListHandlerOpts[testItem, testFilter]{
		Handler:         handler,
		W:               w,
		R:               r,
		FilteredFetcher: mockFilteredFetcher(items, nil),
		FilterParser:    mockFilterParserWithError,
		BasePath:        "/test",
		PageMeta:        PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: PageSecrets},
		ItemsKey:        "Items",
		ErrorMessage:    "Unable to load items.",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// Should show filter parsing error
	assert.Contains(t, body, "Invalid filter parameters")
	assert.Contains(t, body, "invalid filter format")
}

func TestHandleList_NilDependencies(t *testing.T) {
	t.Run("nil ResponseWriter", func(t *testing.T) {
		handler := CreateUIHandlersForTest(t)
		r := httptest.NewRequest(http.MethodGet, "/test", nil)

		// Should not panic
		HandleList(ListHandlerOpts[testItem, struct{}]{
			Handler:      handler,
			W:            nil,
			R:            r,
			Fetcher:      mockSimpleFetcher([]testItem{}, nil),
			BasePath:     "/test",
			PageMeta:     PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: PageSecrets},
			ItemsKey:     "Items",
			ErrorMessage: "Unable to load items.",
		})
	})

	t.Run("nil Request", func(t *testing.T) {
		handler := CreateUIHandlersForTest(t)
		w := httptest.NewRecorder()

		// Should not panic
		HandleList(ListHandlerOpts[testItem, struct{}]{
			Handler:      handler,
			W:            w,
			R:            nil,
			Fetcher:      mockSimpleFetcher([]testItem{}, nil),
			BasePath:     "/test",
			PageMeta:     PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: PageSecrets},
			ItemsKey:     "Items",
			ErrorMessage: "Unable to load items.",
		})

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Internal configuration error")
	})

	t.Run("nil Handler", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()

		// Should not panic
		HandleList(ListHandlerOpts[testItem, struct{}]{
			Handler:      nil,
			W:            w,
			R:            r,
			Fetcher:      mockSimpleFetcher([]testItem{}, nil),
			BasePath:     "/test",
			PageMeta:     PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: PageSecrets},
			ItemsKey:     "Items",
			ErrorMessage: "Unable to load items.",
		})

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Internal configuration error")
	})
}
