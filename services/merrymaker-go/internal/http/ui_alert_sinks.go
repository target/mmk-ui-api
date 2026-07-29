package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
)

const alertSinksBasePath = "/alert-sinks"

func newDefaultTestFireHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// alertSinkTestFireAdapter bridges the UI interface to the concrete AlertSinkService without
// coupling UI to service package internals.
type alertSinkTestFireAdapter struct {
	Svc *service.AlertSinkService
}

func (a alertSinkTestFireAdapter) TestFire(
	ctx context.Context,
	sink *model.HTTPAlertSink,
	httpClient HTTPDoer,
) (*service.TestFireResult, error) {
	if a.Svc == nil {
		return nil, errors.New("test fire service not configured")
	}
	return a.Svc.TestFire(ctx, sink, httpClient)
}

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
		Fetcher: WrapListFetcher(func(ctx context.Context, limit, offset int) ([]*model.HTTPAlertSink, error) {
			return h.Sinks.List(ctx, limit, offset)
		}, h.logger(), "failed to load alert sinks for UI"),
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
		OnError: DefaultDeleteOnError(DeleteOnErrorOpts{
			Logger: h.logger(),
			ListRender: func(w http.ResponseWriter, r *http.Request, msg string) {
				h.renderAlertSinksListError(w, r, msg)
			},
		}),
		OnSuccess: func(w http.ResponseWriter, r *http.Request, _ bool) {
			if IsHTMX(r) {
				HTMX(w).Redirect(alertSinksBasePath)
				return
			}
			http.Redirect(w, r, alertSinksBasePath, http.StatusSeeOther)
		},
	})
}

// renderAlertSinksListError re-renders the alert sinks list page with an error banner.
func (h *UIHandlers) renderAlertSinksListError(w http.ResponseWriter, r *http.Request, msg string) {
	page, pageSize := getPageParams(r.URL.Query())

	builder := NewTemplateData(r, alertSinkListMeta()).
		WithPagination(PaginationData{Page: page, PageSize: pageSize, BasePath: alertSinksBasePath}).
		WithError(msg)

	// Try to preserve list data for better UX
	if items, hasPrev, hasNext, start, end, err := paginate(
		r.Context(),
		pageOpts{Page: page, PageSize: pageSize},
		func(ctx context.Context, limit, offset int) ([]*model.HTTPAlertSink, error) {
			return h.Sinks.List(ctx, limit, offset)
		},
	); err == nil {
		builder.With("AlertSinks", items).
			With("HasPrev", hasPrev).With("HasNext", hasNext).
			With("StartIndex", start).With("EndIndex", end)
		if hasPrev {
			builder.With("PrevURL", buildPageURL(alertSinksBasePath, r.URL.Query(), pageOpts{Page: page - 1, PageSize: pageSize}))
		}
		if hasNext {
			builder.With("NextURL", buildPageURL(alertSinksBasePath, r.URL.Query(), pageOpts{Page: page + 1, PageSize: pageSize}))
		}
	}

	h.renderDashboardPage(w, r, builder.Build())
}

// AlertSinkTestFire handles test-firing an alert sink to validate configuration.
func (h *UIHandlers) AlertSinkTestFire(w http.ResponseWriter, r *http.Request) {
	if h.Sinks == nil {
		h.NotFound(w, r)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.NotFound(w, r)
		return
	}

	// Load the sink
	sink, err := h.Sinks.GetByID(r.Context(), id)
	if err != nil || sink == nil {
		h.logger().Warn("alert sink not found for test fire", "id", id, "error", err)
		h.NotFound(w, r)
		return
	}

	// Check if test fire service is configured
	if h.SinkTestFire == nil {
		h.renderTestFireResult(w, r, sink, &service.TestFireResult{
			Success:      false,
			ErrorMessage: "Test fire is not configured. Please contact your administrator.",
		})
		return
	}

	// Execute test fire
	client := h.HTTPClient
	if client == nil {
		client = newDefaultTestFireHTTPClient()
	}

	result, err := h.SinkTestFire.TestFire(r.Context(), sink, client)
	if err != nil {
		h.logger().Error("test fire failed", "sink_id", id, "error", err)
		if result == nil {
			result = &service.TestFireResult{Success: false}
		}
		result.ErrorMessage = fmt.Sprintf("Test fire error: %v", err)
	}

	if result != nil {
		h.logger().Info("alert sink test fire request", "sink_id", id, "body", result.Request.Body)
	}

	h.renderTestFireResult(w, r, sink, result)
}

// renderTestFireResult renders the test fire result partial.
func (h *UIHandlers) renderTestFireResult(
	w http.ResponseWriter,
	r *http.Request,
	sink *model.HTTPAlertSink,
	result *service.TestFireResult,
) {
	data := map[string]any{
		"SinkID":   sink.ID,
		"SinkName": sink.Name,
		"Result":   result,
	}

	// For HTMX requests, render just the partial
	if IsHTMX(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := h.T.t.ExecuteTemplate(w, "alert-sink-test-result", data); err != nil {
			h.logger().Error("failed to render test fire result", "error", err)
			http.Error(w, "Failed to render result", http.StatusInternalServerError)
		}
		return
	}

	// For non-HTMX requests, redirect back to the sink view
	http.Redirect(w, r, fmt.Sprintf("/alert-sinks/%s", sink.ID), http.StatusSeeOther)
}
