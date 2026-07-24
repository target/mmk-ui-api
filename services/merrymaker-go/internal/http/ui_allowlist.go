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
		Fetcher: WrapListFetcher(
			func(ctx context.Context, limit, offset int) ([]*model.DomainAllowlist, error) {
				return h.AllowlistSvc.List(ctx, model.DomainAllowlistListOptions{Limit: limit, Offset: offset})
			},
			h.logger(),
			"failed to load allowlist for UI",
		),
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
