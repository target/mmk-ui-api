package httpx

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

const errMsgUnableLoadSources = "Unable to load sources."

// --- helpers to keep handlers small and readable ---

type sourcesFilters struct {
	Q            string
	IncludeTests bool
}

func readSourcesFilters(v url.Values) sourcesFilters {
	q := strings.TrimSpace(v.Get("q"))
	inc := false
	switch strings.ToLower(strings.TrimSpace(v.Get("include_tests"))) {
	case "1", StrTrue, "on", "yes":
		inc = true
	}
	return sourcesFilters{Q: q, IncludeTests: inc}
}

func (h *UIHandlers) buildSourceCounts(
	ctx context.Context,
	items []*model.Source,
	includeTests bool,
) (map[string]map[string]int, bool) {
	m := map[string]map[string]int{}
	hadErr := false
	if counts, success, errOccurred := h.tryBatchCount(ctx, items, includeTests); success {
		return counts, false
	} else if errOccurred {
		hadErr = true
	}
	// Fallback to per-source queries
	for _, s := range items {
		if s == nil {
			continue
		}
		total, err1 := h.SourceSvc.CountJobsBySource(ctx, s.ID, includeTests)
		browser, err2 := h.SourceSvc.CountBrowserJobsBySource(ctx, s.ID, includeTests)
		if err1 != nil || err2 != nil {
			hadErr = true
		}
		m[s.ID] = map[string]int{"total": total, "browser": browser}
	}
	return m, hadErr
}

func (h *UIHandlers) renderSourcesError(w http.ResponseWriter, r *http.Request, msg string) {
	page, pageSize := getPageParams(r.URL.Query())
	filters := readSourcesFilters(r.URL.Query())

	data := NewTemplateData(r, PageMeta{Title: "Merrymaker - Sources", PageTitle: "Sources", CurrentPage: PageSources}).
		WithPagination(PaginationData{Page: page, PageSize: pageSize, BasePath: "/sources"}).
		With("Query", filters.Q).
		With("IncludeTests", filters.IncludeTests).
		WithError(msg).
		Build()

	h.renderDashboardPage(w, r, data)
}

func (h *UIHandlers) tryBatchCount(
	ctx context.Context,
	items []*model.Source,
	includeTests bool,
) (map[string]map[string]int, bool, bool) {
	type countsBatcher interface {
		CountAggregatesBySources(
			ctx context.Context,
			ids []string,
			includeTests bool,
		) (map[string]map[string]int, error)
	}

	batcher, ok := any(h.SourceSvc).(countsBatcher)
	if !ok {
		return nil, false, false
	}

	ids := make([]string, 0, len(items))
	for _, s := range items {
		if s == nil {
			continue
		}
		ids = append(ids, s.ID)
	}
	if len(ids) == 0 {
		return nil, false, false
	}

	counts, err := batcher.CountAggregatesBySources(ctx, ids, includeTests)
	if err != nil {
		return nil, false, true
	}

	return counts, true, false
}

// parseSourcesFilter parses filter parameters for the generic list handler.
func parseSourcesFilter(q url.Values) (sourcesFilters, error) {
	return readSourcesFilters(q), nil
}

// Sources serves the Sources list page, HTMX-aware.
func (h *UIHandlers) Sources(w http.ResponseWriter, r *http.Request) {
	// Use generic list handler with filtering
	HandleList(ListHandlerOpts[*model.Source, sourcesFilters]{
		Handler:         h,
		W:               w,
		R:               r,
		FilteredFetcher: h.fetchSourcesWithFilters,
		FilterParser:    parseSourcesFilter,
		EnrichData:      h.enrichSourcesData(r),
		BasePath:        "/sources",
		PageMeta:        PageMeta{Title: "Merrymaker - Sources", PageTitle: "Sources", CurrentPage: PageSources},
		ItemsKey:        "Sources",
		ErrorMessage:    errMsgUnableLoadSources,
		ServiceAvailable: func() bool {
			return h.SourceSvc != nil
		},
		UnavailableMessage: errMsgUnableLoadSources,
		UnavailableData: func(builder *TemplateDataBuilder) {
			filters := readSourcesFilters(r.URL.Query())
			builder.With("Query", filters.Q).With("IncludeTests", filters.IncludeTests)
		},
	})
}

// fetchSourcesWithFilters fetches sources with optional filtering.
func (h *UIHandlers) fetchSourcesWithFilters(
	ctx context.Context,
	filters sourcesFilters,
	pg pageOpts,
) ([]*model.Source, error) {
	limit, offset := pg.LimitAndOffset()

	var sources []*model.Source
	var err error
	if filters.Q != "" {
		sources, err = h.listSourcesByQuery(ctx, filters.Q, limit, offset)
	} else {
		sources, err = h.SourceSvc.List(ctx, limit, offset)
	}

	if err != nil {
		h.logger().ErrorContext(ctx, "failed to load sources for UI",
			"error", err, "query", filters.Q, "include_tests", filters.IncludeTests,
			"page", pg.Page, "page_size", pg.PageSize,
		)
	}
	return sources, err
}

// enrichSourcesData returns a data enricher that adds filter values and scan counts.
func (h *UIHandlers) enrichSourcesData(r *http.Request) DataEnricher[*model.Source, sourcesFilters] {
	return func(builder *TemplateDataBuilder, items []*model.Source, filters sourcesFilters) {
		builder.With("Query", filters.Q).With("IncludeTests", filters.IncludeTests)
		counts, hadErr := h.buildSourceCounts(r.Context(), items, filters.IncludeTests)
		if hadErr {
			builder.With("CountsError", true)
		}
		builder.With("SourceScanCounts", counts)
	}
}

func (h *UIHandlers) listSourcesByQuery(ctx context.Context, query string, limit, offset int) ([]*model.Source, error) {
	if svc, ok := any(h.SourceSvc).(interface {
		ListByNameContains(ctx context.Context, q string, limit, offset int) ([]*model.Source, error)
	}); ok {
		return svc.ListByNameContains(ctx, query, limit, offset)
	}

	return h.SourceSvc.List(ctx, limit, offset)
}

// SourceDelete handles deleting a source from the UI.
func (h *UIHandlers) SourceDelete(w http.ResponseWriter, r *http.Request) {
	h.handleDelete(w, r, deleteHandlerOpts{
		ServiceAvailable: func() bool { return h.SourceSvc != nil },
		Delete: func(ctx context.Context, id string) (bool, error) {
			return h.SourceSvc.Delete(ctx, id)
		},
		RedirectPath: "/sources",
		OnError:      h.handleSourceDeleteError,
		OnSuccess: func(w http.ResponseWriter, r *http.Request, deleted bool) {
			if IsHTMX(r) {
				message := "Source deleted successfully"
				msgType := "success"
				status := http.StatusOK

				if !deleted {
					message = "Source not found. It may have already been deleted."
					msgType = "warning"
					status = http.StatusNoContent
				}

				triggerToast(w, message, msgType)
				w.WriteHeader(status)
				return
			}

			if deleted {
				http.Redirect(w, r, "/sources", http.StatusSeeOther)
				return
			}

			h.NotFound(w, r)
		},
	})
}

// handleSourceDeleteError handles errors from source deletion attempts, surfacing toast notifications for HTMX requests.
func (h *UIHandlers) handleSourceDeleteError(w http.ResponseWriter, r *http.Request, err error) {
	errMsg := processError(err, nil)
	if strings.TrimSpace(errMsg) == "" || errMsg == errMsgFixBelow || errMsg == "An error occurred. Please try again." {
		errMsg = "Unable to delete source. It may be in use by a Site."
	}

	if IsHTMX(r) {
		triggerToast(w, errMsg, "error")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	h.renderSourcesError(w, r, errMsg)
}
