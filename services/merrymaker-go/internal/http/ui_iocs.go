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
	Type    *model.IOCType
	Enabled *bool
	Search  *string
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
		Handler:         h,
		W:               w,
		R:               r,
		FilteredFetcher: h.fetchIOCsWithFilters,
		FilterParser:    parseIOCsFilter,
		EnrichData:      h.enrichIOCsData(),
		BasePath:        "/iocs",
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

// fetchIOCsWithFilters fetches IOCs with applied filters and pagination.
func (h *UIHandlers) fetchIOCsWithFilters(ctx context.Context, f iocsFilter, pg pageOpts) ([]*model.IOC, error) {
	limit, offset := pg.LimitAndOffset()

	opts := model.IOCListOptions{
		Limit:   limit,
		Offset:  offset,
		Type:    f.Type,
		Enabled: f.Enabled,
		Search:  f.Search,
	}

	iocs, err := h.IOCSvc.List(ctx, opts)
	if err != nil {
		h.logger().ErrorContext(ctx, "failed to load IOCs for UI",
			"error", err,
			"page", pg.Page,
			"page_size", pg.PageSize,
			"filters", f,
		)
	}
	return iocs, err
}

// parseIOCsFilter parses filter parameters from query string.
func parseIOCsFilter(q url.Values) (iocsFilter, error) {
	var f iocsFilter

	// Type filter
	if typeStr := q.Get("type"); typeStr != "" {
		iocType := model.IOCType(typeStr)
		f.Type = &iocType
	}

	// Enabled filter
	if enabledStr := q.Get("enabled"); enabledStr != "" {
		enabled := enabledStr == StrTrue || enabledStr == "1"
		f.Enabled = &enabled
	}

	// Search filter
	if search := q.Get("q"); search != "" {
		f.Search = &search
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

		searchQuery := ""
		if f.Search != nil {
			searchQuery = *f.Search
		}
		builder.With("SearchQuery", searchQuery)

		// Add flag to indicate if any filters are active
		hasActiveFilters := f.Type != nil || f.Enabled != nil || (f.Search != nil && *f.Search != "")
		builder.With("HasActiveFilters", hasActiveFilters)
	}
}
