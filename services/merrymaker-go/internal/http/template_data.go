package httpx

import (
	"net/http"
)

// PaginationData contains pagination information for list views.
type PaginationData struct {
	Page       int
	PageSize   int
	HasPrev    bool
	HasNext    bool
	StartIndex int
	EndIndex   int
	TotalCount int // Optional: total count of items (0 if not available)
	BasePath   string
	PrevCursor *string
	NextCursor *string
	PrevIndex  int
	NextIndex  int
}

// TemplateDataBuilder provides a fluent API for building template data maps.
type TemplateDataBuilder struct {
	data map[string]any
	r    *http.Request
}

// NewTemplateData creates a new TemplateDataBuilder initialized with basePageData.
func NewTemplateData(r *http.Request, meta PageMeta) *TemplateDataBuilder {
	return &TemplateDataBuilder{
		data: basePageData(r, meta),
		r:    r,
	}
}

// WithPagination adds pagination data and builds PrevURL/NextURL.
func (b *TemplateDataBuilder) WithPagination(opts PaginationData) *TemplateDataBuilder {
	b.data["Page"] = opts.Page
	b.data["PageSize"] = opts.PageSize
	b.data["HasPrev"] = opts.HasPrev
	b.data["HasNext"] = opts.HasNext
	b.data["StartIndex"] = opts.StartIndex
	b.data["EndIndex"] = opts.EndIndex
	if opts.TotalCount > 0 {
		b.data["TotalCount"] = opts.TotalCount
	}

	if opts.HasPrev {
		if opts.PrevCursor != nil {
			b.data["PrevURL"] = buildCursorURL(
				opts.BasePath,
				b.r.URL.Query(),
				"cursor_before",
				*opts.PrevCursor,
				opts.PageSize,
				opts.PrevIndex,
			)
		} else {
			b.data["PrevURL"] = buildPageURL(
				opts.BasePath,
				b.r.URL.Query(),
				pageOpts{Page: opts.Page - 1, PageSize: opts.PageSize},
			)
		}
	}
	if opts.HasNext {
		if opts.NextCursor != nil {
			b.data["NextURL"] = buildCursorURL(
				opts.BasePath,
				b.r.URL.Query(),
				"cursor_after",
				*opts.NextCursor,
				opts.PageSize,
				opts.NextIndex,
			)
		} else {
			b.data["NextURL"] = buildPageURL(
				opts.BasePath,
				b.r.URL.Query(),
				pageOpts{Page: opts.Page + 1, PageSize: opts.PageSize},
			)
		}
	}

	return b
}

// WithError sets a general error message.
func (b *TemplateDataBuilder) WithError(msg string) *TemplateDataBuilder {
	b.data["Error"] = true
	b.data["ErrorMessage"] = msg
	return b
}

// WithFieldErrors adds field-level validation errors.
func (b *TemplateDataBuilder) WithFieldErrors(errs map[string]string) *TemplateDataBuilder {
	if len(errs) > 0 {
		b.data["Errors"] = errs
	}
	return b
}

// With adds a custom field to the template data.
func (b *TemplateDataBuilder) With(key string, value any) *TemplateDataBuilder {
	b.data[key] = value
	return b
}

// Build returns the final template data map.
func (b *TemplateDataBuilder) Build() map[string]any {
	return b.data
}
