package core

import (
	"context"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// AlertDispatcher dispatches alerts to configured HTTP alert sinks.
type AlertDispatcher interface {
	// Dispatch sends an alert to all configured HTTP alert sinks.
	// Returns error if dispatch fails for all sinks, but logs individual failures.
	Dispatch(ctx context.Context, alert *model.Alert) error
}
