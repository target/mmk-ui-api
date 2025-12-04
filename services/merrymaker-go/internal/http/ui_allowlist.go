package httpx

import (
	"context"
	"net/http"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// Allowlist serves the Domain Allow List page (list view). HTMX-aware.
func (h *UIHandlers) Allowlist(w http.ResponseWriter, r *http.Request) {
	// Use generic list handler - no filtering needed for allowlist
	HandleList(ListHandlerOpts[*model.DomainAllowlist, struct{}]{
		Handler: h,
		W:       w,
		R:       r,
		Fetcher: func(ctx context.Context, pg pageOpts) ([]*model.DomainAllowlist, error) {
			// Fetch pageSize+1 to detect hasNext
			limit, offset := pg.LimitAndOffset()
			listOpts := model.DomainAllowlistListOptions{Limit: limit, Offset: offset}
			items, err := h.AllowlistSvc.List(ctx, listOpts)
			if err != nil {
				h.logger().Error("failed to load allowlist for UI",
					"error", err,
					"page", pg.Page,
					"page_size", pg.PageSize,
				)
			}
			return items, err
		},
		BasePath:     "/allowlist",
		PageMeta:     PageMeta{Title: "Merrymaker - Allow List", PageTitle: "Allow List", CurrentPage: PageAllowlist},
		ItemsKey:     "Allowlist",
		ErrorMessage: "Unable to load allow list.",
		ServiceAvailable: func() bool {
			return h.AllowlistSvc != nil
		},
		UnavailableMessage: "Unable to load allow list.",
	})
}
