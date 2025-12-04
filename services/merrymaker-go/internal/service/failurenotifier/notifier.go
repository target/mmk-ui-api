package failurenotifier

import (
	"context"
	"log/slog"
	"sync"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/observability/notify"
)

// SinkRegistration pairs a sink implementation with a human-readable name for logging.
type SinkRegistration struct {
	Name string
	Sink notify.Sink
}

// Options configures the failure notifier service.
type Options struct {
	Logger *slog.Logger
	Sinks  []SinkRegistration
}

// Service dispatches failure events to all registered sinks.
type Service struct {
	logger *slog.Logger
	sinks  []SinkRegistration
}

// NewService constructs a failure notifier.
func NewService(opts Options) *Service {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default().With("component", "failure_notifier")
	}

	var sinks []SinkRegistration
	for _, entry := range opts.Sinks {
		if entry.Sink == nil {
			continue
		}
		name := entry.Name
		if name == "" {
			name = "sink"
		}
		sinks = append(sinks, SinkRegistration{
			Name: name,
			Sink: entry.Sink,
		})
	}

	return &Service{
		logger: logger,
		sinks:  sinks,
	}
}

// NotifyJobFailure fan-outs the job failure payload to all sinks.
func (s *Service) NotifyJobFailure(ctx context.Context, payload notify.JobFailurePayload) {
	if len(s.sinks) == 0 {
		return
	}

	if payload.JobType == string(model.JobTypeBrowser) && payload.IsTest {
		if s.logger != nil {
			s.logger.DebugContext(ctx, "skipping notification for test browser job",
				"job_id", payload.JobID,
				"job_type", payload.JobType,
			)
		}
		return
	}

	if payload.Severity == "" {
		payload.Severity = notify.SeverityCritical
	}

	var wg sync.WaitGroup
	for _, entry := range s.sinks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := entry.Sink.SendJobFailure(ctx, payload); err != nil {
				s.logger.Error("failure notifier delivery error",
					"sink", entry.Name,
					"job_id", payload.JobID,
					"job_type", payload.JobType,
					"error", err,
				)
			}
		}()
	}
	wg.Wait()
}

// Enabled reports whether the notifier has any active sinks.
func (s *Service) Enabled() bool {
	return len(s.sinks) > 0
}
