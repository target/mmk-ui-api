package httpx

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

type siteRow struct {
	ID              string
	Name            string
	Enabled         bool
	RunEveryMinutes int
	ScopeDisplay    string
}

type sitesFilter struct {
	BaseFilter
	Scope string
}

type pageBounds struct {
	Limit  int
	Offset int
}

func parseSitesFilter(q url.Values) (sitesFilter, error) {
	return sitesFilter{
		BaseFilter: ParseBaseFilter(q),
		Scope:      strings.TrimSpace(q.Get("scope")),
	}, nil
}

func toSiteRows(sites []*model.Site) []siteRow {
	out := make([]siteRow, 0, len(sites))
	for _, s := range sites {
		row := siteRow{ID: s.ID, Name: s.Name, Enabled: s.Enabled, RunEveryMinutes: s.RunEveryMinutes}
		row.ScopeDisplay = scopeDisplay(s.Scope)
		out = append(out, row)
	}
	return out
}

func (h *UIHandlers) listSiteRows(ctx context.Context, f sitesFilter, pg pageBounds) ([]siteRow, error) {
	// Prefer options-aware list if available (explicit named interface)
	svc, ok := h.SiteSvc.(SitesServiceWithOptions)
	if !ok {
		return h.listSiteRowsLegacy(ctx, pg)
	}

	sites, err := svc.ListWithOptions(ctx, buildSiteListOptions(f, pg))
	if err != nil {
		return nil, err
	}
	return toSiteRows(sites), nil
}

func (h *UIHandlers) renderSitesError(w http.ResponseWriter, r *http.Request, msg string) {
	page, pageSize := getPageParams(r.URL.Query())
	filters, err := parseSitesFilter(r.URL.Query())
	if err != nil {
		h.logger().Warn("failed to parse sites filter for error view", "error", err)
		filters = sitesFilter{}
	}

	builder := NewTemplateData(r, PageMeta{Title: "Merrymaker - Sites", PageTitle: "Sites", CurrentPage: PageSites}).
		WithPagination(PaginationData{Page: page, PageSize: pageSize, BasePath: "/sites"}).
		With("Query", filters.Q).
		WithError(msg)

	// Add filter values if present
	if filters.Enabled != nil {
		builder.With("EnabledFilterSet", true).With("Enabled", strconv.FormatBool(*filters.Enabled))
	}
	builder.With("Scope", filters.Scope).With("Sort", filters.Sort).With("Dir", filters.Dir)

	h.renderDashboardPage(w, r, builder.Build())
}

func scopeDisplay(scope *string) string {
	if scope == nil || *scope == "" {
		return "default"
	}
	return *scope
}

func (h *UIHandlers) listSiteRowsLegacy(ctx context.Context, pg pageBounds) ([]siteRow, error) {
	sites, err := h.SiteSvc.List(ctx, pg.Limit, pg.Offset)
	if err != nil {
		return nil, err
	}
	return toSiteRows(sites), nil
}

func buildSiteListOptions(f sitesFilter, pg pageBounds) model.SitesListOptions {
	var qPtr *string
	if f.Q != "" {
		qLocal := f.Q
		qPtr = &qLocal
	}

	var scopePtr *string
	if f.Scope != "" {
		sLocal := f.Scope
		scopePtr = &sLocal
	}

	return model.SitesListOptions{
		Limit:   pg.Limit,
		Offset:  pg.Offset,
		Q:       qPtr,
		Enabled: f.Enabled,
		Scope:   scopePtr,
		Sort:    f.Sort,
		Dir:     f.Dir,
	}
}

// Sites renders the Sites list page with Dry Run actions, aligned with Sources UX.
func (h *UIHandlers) Sites(w http.ResponseWriter, r *http.Request) {
	// Use generic list handler with complex filtering
	HandleList(ListHandlerOpts[siteRow, sitesFilter]{
		Handler: h,
		W:       w,
		R:       r,
		FilteredFetcher: WrapFilteredFetcher(
			func(ctx context.Context, filters sitesFilter, limit, offset int) ([]siteRow, error) {
				return h.listSiteRows(ctx, filters, pageBounds{Limit: limit, Offset: offset})
			},
			h.logger(),
			"failed to load sites for UI",
			func(filters sitesFilter, pg pageOpts) []any {
				return []any{
					"query", filters.Q,
					"enabled", filters.Enabled,
					"scope", filters.Scope,
					"sort", filters.Sort,
					"dir", filters.Dir,
				}
			},
		),
		FilterParser: parseSitesFilter,
		EnrichData: func(builder *TemplateDataBuilder, _ []siteRow, filters sitesFilter) {
			// Add filter values to template
			builder.With("Query", filters.Q)
			if filters.Enabled != nil {
				builder.With("EnabledFilterSet", true).With("Enabled", strconv.FormatBool(*filters.Enabled))
			}
			builder.With("Scope", filters.Scope).With("Sort", filters.Sort).With("Dir", filters.Dir)
		},
		BasePath:     "/sites",
		PageMeta:     PageMeta{Title: "Merrymaker - Sites", PageTitle: "Sites", CurrentPage: PageSites},
		ItemsKey:     "Sites",
		ErrorMessage: "Unable to load sites.",
		ServiceAvailable: func() bool {
			return h.SiteSvc != nil
		},
		UnavailableItems: []siteRow{},
	})
}

// SiteDelete handles deleting a site from the UI.
func (h *UIHandlers) SiteDelete(w http.ResponseWriter, r *http.Request) {
	h.handleDelete(w, r, deleteHandlerOpts{
		ServiceAvailable: func() bool { return h.SiteSvc != nil },
		Delete: func(ctx context.Context, id string) (bool, error) {
			return h.SiteSvc.Delete(ctx, id)
		},
		RedirectPath: "/sites",
		OnError: func(w http.ResponseWriter, r *http.Request, _ error) {
			h.renderSitesError(w, r, "Unable to delete site. It may be in use or have associated jobs.")
		},
	})
}
