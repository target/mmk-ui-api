package httpx

import (
	"context"
	"net/http"
	"net/url"
)

// ListFetcher is a generic function type for fetching paginated data without filters.
// It takes a context and pagination bounds, and returns a slice of items of type T.
// Maintains ≤3 parameters per project constraints.
type ListFetcher[T any] func(ctx context.Context, pg pageOpts) ([]T, error)

// FilterParser is a function type for parsing URL query parameters into filter data.
// It takes url.Values and returns the parsed filter of type F, or an error if parsing fails.
// The error allows the handler to show meaningful validation errors for invalid filter params.
type FilterParser[F any] func(url.Values) (F, error)

// FilteredFetcher is a function type for fetching data with filters applied.
// It takes context, parsed filters of type F, and pagination bounds.
// Returns items of type T. Maintains ≤3 parameters per project constraints.
type FilteredFetcher[T any, F any] func(ctx context.Context, filters F, pg pageOpts) ([]T, error)

// DataEnricher is a function type for enriching template data after fetching items.
// It receives the template data builder, items, and filters, and can add custom data.
// This allows domain-specific data enrichment (e.g., adding counts, related data).
type DataEnricher[T any, F any] func(builder *TemplateDataBuilder, items []T, filters F)

// ListHandlerOpts contains all options needed for the generic list handler.
// Uses two generic type parameters: T for item type, F for filter type.
// All function types maintain ≤3 parameters per project constraints.
type ListHandlerOpts[T any, F any] struct {
	// Handler is the UIHandlers instance for rendering (required)
	Handler *UIHandlers
	// W is the HTTP response writer (required)
	W http.ResponseWriter
	// R is the HTTP request (required)
	R *http.Request
	// Fetcher is the function to fetch paginated data (simple case, no filtering)
	Fetcher ListFetcher[T]
	// FilteredFetcher is the function to fetch data with filters (complex case)
	// Use this OR Fetcher, not both. If both are provided, FilteredFetcher takes precedence.
	FilteredFetcher FilteredFetcher[T, F]
	// FilterParser is an optional function to parse filters from query params
	FilterParser FilterParser[F]
	// EnrichData is an optional function to add custom data to the template after fetching
	EnrichData DataEnricher[T, F]
	// BasePath is the base URL path for pagination links (e.g., "/secrets", "/sites")
	BasePath string
	// PageMeta contains page metadata for rendering
	PageMeta PageMeta
	// ItemsKey is the template data key for the items (e.g., "Secrets", "Sources")
	ItemsKey string
	// ErrorMessage is the message to display when data fetching fails
	ErrorMessage string
	// ServiceAvailable should return true when the backing service is ready.
	// When provided and it returns false, HandleList renders the unavailable view.
	ServiceAvailable func() bool
	// UnavailableItems are rendered when the service is unavailable. Optional.
	UnavailableItems []T
	// UnavailableMessage is displayed when the service is unavailable. Optional.
	UnavailableMessage string
	// UnavailableData allows handlers to add custom fields when service is unavailable.
	UnavailableData func(builder *TemplateDataBuilder)
}

// HandleList is the generic list view handler that eliminates pagination/filtering duplication.
// It handles pagination, filtering, error handling, and template rendering consistently.
// Uses two generic type parameters: T for item type, F for filter type.
//
// Usage examples:
//
// Simple list (no filtering):
//
//	HandleList(ListHandlerOpts[*types.Secret, struct{}]{
//	    Handler:      h,
//	    W:            w,
//	    R:            r,
//	    Fetcher:      func(ctx context.Context, pg pageOpts) ([]*types.Secret, error) {
//	        return h.SecretSvc.List(ctx, pg.PageSize+1, (pg.Page-1)*pg.PageSize)
//	    },
//	    BasePath:     "/secrets",
//	    PageMeta:     PageMeta{Title: "Secrets", CurrentPage: "secrets"},
//	    ItemsKey:     "Secrets",
//	    ErrorMessage: "Unable to load secrets.",
//	})
//
// With filtering:
//
//	HandleList(ListHandlerOpts[siteRow, sitesFilter]{
//	    Handler:         h,
//	    W:               w,
//	    R:               r,
//	    FilteredFetcher: func(ctx context.Context, f sitesFilter, pg pageOpts) ([]siteRow, error) {
//	        return h.listSiteRows(ctx, f, pageBounds{Limit: pg.PageSize+1, Offset: (pg.Page-1)*pg.PageSize})
//	    },
//	    FilterParser:    parseSitesFilter,
//	    BasePath:        "/sites",
//	    PageMeta:        PageMeta{Title: "Sites", CurrentPage: "sites"},
//	    ItemsKey:        "Sites",
//	    ErrorMessage:    "Unable to load sites.",
//	})
func HandleList[T, F any](opts ListHandlerOpts[T, F]) {
	// Defensive nil checks for required dependencies
	if !validateListHandlerDeps(opts) {
		return
	}

	// If the backing service is unavailable, render the fallback view.
	if opts.ServiceAvailable != nil && !opts.ServiceAvailable() {
		renderUnavailableList(opts)
		return
	}

	// Parse pagination parameters
	page, pageSize := getPageParams(opts.R.URL.Query())

	// Parse filters if parser is provided
	var filters F
	if opts.FilterParser != nil {
		var filterErr error
		filters, filterErr = opts.FilterParser(opts.R.URL.Query())
		if filterErr != nil {
			opts.renderListError(page, pageSize, "Invalid filter parameters: "+filterErr.Error())
			return
		}
	}

	// Create the appropriate fetcher function
	fetchFunc := createListFetcher(opts, filters)
	if fetchFunc == nil {
		opts.renderListError(page, pageSize, "No data fetcher configured.")
		return
	}

	// Fetch and render data
	pg := pageOpts{Page: page, PageSize: pageSize}
	items, err := fetchFunc(opts.R.Context(), pg)
	if err != nil {
		opts.renderListError(page, pageSize, opts.ErrorMessage)
		return
	}

	renderListSuccess(listRenderCtx[T, F]{
		Opts:     opts,
		Page:     page,
		PageSize: pageSize,
		Items:    items,
		Filters:  filters,
	})
}

// listRenderCtx consolidates parameters for rendering list success to maintain ≤3 params constraint.
type listRenderCtx[T any, F any] struct {
	Opts     ListHandlerOpts[T, F]
	Page     int
	PageSize int
	Items    []T
	Filters  F
}

// validateListHandlerDeps checks required dependencies and returns false if any are nil.
func validateListHandlerDeps[T, F any](opts ListHandlerOpts[T, F]) bool {
	if opts.W == nil || opts.R == nil || opts.Handler == nil {
		if opts.W != nil {
			http.Error(opts.W, "Internal configuration error", http.StatusInternalServerError)
		}
		return false
	}
	return true
}

// createListFetcher creates the appropriate fetcher function based on opts configuration.
func createListFetcher[T, F any](opts ListHandlerOpts[T, F], filters F) ListFetcher[T] {
	switch {
	case opts.FilteredFetcher != nil:
		return func(ctx context.Context, pg pageOpts) ([]T, error) {
			return opts.FilteredFetcher(ctx, filters, pg)
		}
	case opts.Fetcher != nil:
		return opts.Fetcher
	default:
		return nil
	}
}

// renderListError renders an error page with pagination metadata.
func (lh *ListHandlerOpts[T, F]) renderListError(page, pageSize int, errMsg string) {
	builder := NewTemplateData(lh.R, lh.PageMeta).
		WithPagination(PaginationData{Page: page, PageSize: pageSize, BasePath: lh.BasePath}).
		WithError(errMsg)
	lh.Handler.renderDashboardPage(lh.W, lh.R, builder.Build())
}

// renderListSuccess renders the list view with items and pagination.
func renderListSuccess[T, F any](ctx listRenderCtx[T, F]) {
	// Calculate pagination metadata
	hasPrev := ctx.Page > 1
	hasNext := len(ctx.Items) > ctx.PageSize
	items := ctx.Items
	if hasNext {
		items = items[:ctx.PageSize]
	}
	var start, end int
	if len(items) > 0 {
		offset := (ctx.Page - 1) * ctx.PageSize
		start = offset + 1
		end = offset + len(items)
	}

	// Build and render template data
	builder := NewTemplateData(ctx.Opts.R, ctx.Opts.PageMeta).
		WithPagination(PaginationData{
			Page:       ctx.Page,
			PageSize:   ctx.PageSize,
			HasPrev:    hasPrev,
			HasNext:    hasNext,
			StartIndex: start,
			EndIndex:   end,
			BasePath:   ctx.Opts.BasePath,
		}).
		With(ctx.Opts.ItemsKey, items)

	// Allow domain-specific data enrichment
	if ctx.Opts.EnrichData != nil {
		ctx.Opts.EnrichData(builder, items, ctx.Filters)
	}

	ctx.Opts.Handler.renderDashboardPage(ctx.Opts.W, ctx.Opts.R, builder.Build())
}

// renderUnavailableList renders the list view when the backing service is unavailable.
func renderUnavailableList[T, F any](opts ListHandlerOpts[T, F]) {
	page, pageSize := getPageParams(opts.R.URL.Query())
	builder := NewTemplateData(opts.R, opts.PageMeta).
		WithPagination(PaginationData{Page: page, PageSize: pageSize, BasePath: opts.BasePath})

	if opts.ItemsKey != "" {
		builder.With(opts.ItemsKey, opts.UnavailableItems)
	}
	if msg := opts.UnavailableMessage; msg != "" {
		builder.WithError(msg)
	}
	if opts.UnavailableData != nil {
		opts.UnavailableData(builder)
	}

	opts.Handler.renderDashboardPage(opts.W, opts.R, builder.Build())
}
