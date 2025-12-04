package httpx

import (
	"context"
	"net/http"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

const alertSinksBasePath = "/alert-sinks"

func alertSinkListMeta() PageMeta {
	return PageMeta{
		Title:       "Merrymaker - HTTP Alert Sinks",
		PageTitle:   "HTTP Alert Sinks",
		CurrentPage: PageAlertSinks,
	}
}

func alertSinkDetailMeta() PageMeta {
	return PageMeta{
		Title:       "Merrymaker - Alert Sink",
		PageTitle:   "Alert Sink",
		CurrentPage: PageAlertSink,
	}
}

// AlertSinks serves the alert sinks list page, HTMX-aware.
func (h *UIHandlers) AlertSinks(w http.ResponseWriter, r *http.Request) {
	HandleList(ListHandlerOpts[*model.HTTPAlertSink, struct{}]{
		Handler: h,
		W:       w,
		R:       r,
		Fetcher: func(ctx context.Context, pg pageOpts) ([]*model.HTTPAlertSink, error) {
			limit, offset := pg.LimitAndOffset()
			items, err := h.Sinks.List(ctx, limit, offset)
			if err != nil {
				h.logger().Error("failed to load alert sinks for UI",
					"error", err,
					"page", pg.Page,
					"page_size", pg.PageSize,
				)
			}
			return items, err
		},
		BasePath:     alertSinksBasePath,
		PageMeta:     alertSinkListMeta(),
		ItemsKey:     "AlertSinks",
		ErrorMessage: "Unable to load alert sinks.",
		ServiceAvailable: func() bool {
			return h.Sinks != nil
		},
		UnavailableItems:   []*model.HTTPAlertSink{},
		UnavailableMessage: "Unable to load alert sinks.",
	})
}

// AlertSinkView serves the alert sink detail page for a specific id, HTMX-aware.
func (h *UIHandlers) AlertSinkView(w http.ResponseWriter, r *http.Request) {
	if h.Sinks == nil {
		h.NotFound(w, r)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		h.NotFound(w, r)
		return
	}

	entity, err := h.Sinks.GetByID(r.Context(), id)
	if err != nil {
		h.logger().Warn("failed to load alert sink",
			"id", id,
			"error", err,
		)
		h.NotFound(w, r)
		return
	}
	if entity == nil {
		h.logger().Debug("alert sink not found", "id", id)
		h.NotFound(w, r)
		return
	}

	data := NewTemplateData(r, alertSinkDetailMeta()).
		With("AlertSink", buildAlertSinkTemplateData(entity)).
		Build()
	h.renderDashboardPage(w, r, data)
}

// AlertSinkDelete handles deleting an alert sink from the UI.
func (h *UIHandlers) AlertSinkDelete(w http.ResponseWriter, r *http.Request) {
	h.handleDelete(w, r, deleteHandlerOpts{
		ServiceAvailable: func() bool { return h.Sinks != nil },
		Delete: func(ctx context.Context, id string) (bool, error) {
			return h.Sinks.Delete(ctx, id)
		},
		RedirectPath: alertSinksBasePath,
		OnError: func(w http.ResponseWriter, r *http.Request, err error) {
			h.handleAlertSinkDeleteError(w, r, err)
		},
		OnSuccess: func(w http.ResponseWriter, r *http.Request, _ bool) {
			if IsHTMX(r) {
				HTMX(w).Redirect(alertSinksBasePath)
				return
			}
			http.Redirect(w, r, alertSinksBasePath, http.StatusSeeOther)
		},
	})
}

// handleAlertSinkDeleteError handles errors during alert sink deletion.
func (h *UIHandlers) handleAlertSinkDeleteError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger().Error("failed to delete alert sink", "error", err, "path", r.URL.Path)

	page, pageSize := getPageParams(r.URL.Query())

	// Try to preserve list data for better UX
	var additionalData map[string]any
	if sinks, pagData := h.fetchAlertSinksForError(r.Context(), page, pageSize); sinks != nil {
		additionalData = map[string]any{
			"AlertSinks": sinks,
			"HasPrev":    pagData.HasPrev,
			"HasNext":    pagData.HasNext,
			"StartIndex": pagData.StartIndex,
			"EndIndex":   pagData.EndIndex,
			"Page":       pagData.Page,
			"PageSize":   pagData.PageSize,
		}
		if pagData.HasPrev {
			additionalData["PrevURL"] = buildPageURL(
				alertSinksBasePath,
				r.URL.Query(),
				pageOpts{Page: page - 1, PageSize: pageSize},
			)
		}
		if pagData.HasNext {
			additionalData["NextURL"] = buildPageURL(
				alertSinksBasePath,
				r.URL.Query(),
				pageOpts{Page: page + 1, PageSize: pageSize},
			)
		}
	}

	// Determine appropriate status code based on error type
	// Only set 409 Conflict for actual FK violations; otherwise use default (200 for HTMX)
	status := DetermineErrorStatus(err)

	// Use the error renderer for consistent error handling
	RenderError(ErrorOpts{
		W:          w,
		R:          r,
		Err:        err,
		Renderer:   h.renderDashboardPage,
		PageMeta:   alertSinkListMeta(),
		Data:       additionalData,
		StatusCode: status,
	})
}

// fetchAlertSinksForError attempts to fetch alert sinks for error page display.
func (h *UIHandlers) fetchAlertSinksForError(
	ctx context.Context,
	page, pageSize int,
) ([]*model.HTTPAlertSink, PaginationData) {
	limit := pageSize + 1
	offset := (page - 1) * pageSize
	sinks, err := h.Sinks.List(ctx, limit, offset)
	if err != nil {
		return nil, PaginationData{}
	}

	hasPrev := page > 1
	hasNext := len(sinks) > pageSize
	if hasNext {
		sinks = sinks[:pageSize]
	}

	start, end := 0, 0
	if len(sinks) > 0 {
		start, end = offset+1, offset+len(sinks)
	}

	return sinks, PaginationData{
		Page: page, PageSize: pageSize, HasPrev: hasPrev, HasNext: hasNext,
		StartIndex: start, EndIndex: end, BasePath: alertSinksBasePath,
	}
}
