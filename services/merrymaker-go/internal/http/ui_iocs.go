package httpx

import (
	"context"
	"net/http"
	"net/url"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

const errMsgUnableLoadIOCs = "Unable to load IOCs."

// iocsFilter holds filter parameters for the IOCs list view.
type iocsFilter struct {
	BaseFilter
	Type   *model.IOCType
	Search *string
}

// IOCs serves the IOCs list page, HTMX-aware.
func (h *UIHandlers) IOCs(w http.ResponseWriter, r *http.Request) {
	// Defensive nil check
	if h == nil {
		h.logger().Error("IOCs handler: UIHandlers is nil")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if h.T == nil {
		h.logger().Error("IOCs handler: TemplateRenderer is nil")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if h.IOCSvc == nil {
		h.logger().Warn("IOCs handler: IOCService is nil, will render unavailable view")
	}

	h.logger().Debug("IOCs handler: starting list handler")

	// Use generic list handler with filtering
	HandleList(ListHandlerOpts[*model.IOC, iocsFilter]{
		Handler: h,
		W:       w,
		R:       r,
		FilteredFetcher: WrapFilteredFetcher(
			func(ctx context.Context, filters iocsFilter, limit, offset int) ([]*model.IOC, error) {
				opts := model.IOCListOptions{
					Limit:   limit,
					Offset:  offset,
					Type:    filters.Type,
					Enabled: filters.Enabled,
					Search:  filters.Search,
				}
				return h.IOCSvc.List(ctx, opts)
			},
			h.logger(),
			"failed to load IOCs for UI",
			func(filters iocsFilter, _ pageOpts) []any {
				return []any{"filters", filters}
			},
		),
		FilterParser: parseIOCsFilter,
		EnrichData:   h.enrichIOCsData(),
		BasePath:     "/iocs",
		PageMeta: PageMeta{
			Title:       "Merrymaker - IOCs",
			PageTitle:   "IOCs",
			CurrentPage: PageIOCs,
		},
		ItemsKey:     "IOCs",
		ErrorMessage: errMsgUnableLoadIOCs,
		ServiceAvailable: func() bool {
			return h.IOCSvc != nil
		},
		UnavailableMessage: errMsgUnableLoadIOCs,
	})
}

// parseIOCsFilter parses filter parameters from query string.
func parseIOCsFilter(q url.Values) (iocsFilter, error) {
	f := iocsFilter{BaseFilter: ParseBaseFilter(q)}

	// Type filter
	if typeStr := q.Get("type"); typeStr != "" {
		iocType := model.IOCType(typeStr)
		f.Type = &iocType
	}

	// Search filter (use BaseFilter.Q, but keep pointer form for backward compat)
	if f.Q != "" {
		f.Search = &f.Q
	}

	return f, nil
}

// enrichIOCsData returns a function that enriches the template data with filter state.
func (h *UIHandlers) enrichIOCsData() DataEnricher[*model.IOC, iocsFilter] {
	return func(builder *TemplateDataBuilder, _ []*model.IOC, f iocsFilter) {
		// Add filter state for template rendering
		typeFilter := ""
		if f.Type != nil {
			typeFilter = string(*f.Type)
		}
		builder.With("TypeFilter", typeFilter)

		enabledFilter := ""
		if f.Enabled != nil {
			if *f.Enabled {
				enabledFilter = StrTrue
			} else {
				enabledFilter = "false"
			}
		}
		builder.With("EnabledFilter", enabledFilter)

		builder.With("SearchQuery", f.Q)

		// Add flag to indicate if any filters are active
		hasActiveFilters := f.Type != nil || f.Enabled != nil || f.Q != ""
		builder.With("HasActiveFilters", hasActiveFilters)
	}
}
