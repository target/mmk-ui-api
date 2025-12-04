package core

import (
	"context"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// AlertService is a minimal adapter for creating alerts.
// This exists solely to support the rules package tests without creating import cycles.
// Production code should use service.AlertService instead.
//
// Deprecated: This is only for internal/service/rules tests. Use service.AlertService in production.
type AlertService struct {
	Repo AlertRepository
}

// Create creates a new alert with the given request parameters.
// This is a minimal implementation that just delegates to the repository.
func (s *AlertService) Create(ctx context.Context, req *model.CreateAlertRequest) (*model.Alert, error) {
	return s.Repo.Create(ctx, req)
}
