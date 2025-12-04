package failurenotifier

import (
	"context"
	"errors"
	"testing"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/observability/notify"
)

func TestServiceNotifyJobFailure(t *testing.T) {
	ctx := context.Background()

	var received []notify.JobFailurePayload
	svc := NewService(Options{
		Sinks: []SinkRegistration{
			{
				Name: "capture",
				Sink: notify.SinkFunc(func(ctx context.Context, payload notify.JobFailurePayload) error {
					received = append(received, payload)
					return nil
				}),
			},
		},
	})

	svc.NotifyJobFailure(ctx, notify.JobFailurePayload{
		JobID:   "123",
		JobType: "rules",
	})

	if len(received) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(received))
	}
	if received[0].Severity != notify.SeverityCritical {
		t.Fatalf("expected severity to default to critical, got %s", received[0].Severity)
	}
}

func TestServiceDisabled(t *testing.T) {
	svc := NewService(Options{})
	if svc.Enabled() {
		t.Fatal("expected Enabled() to be false when no sinks registered")
	}
}

func TestServiceLogsErrors(t *testing.T) {
	// Ensure we don't panic when sink returns an error.
	svc := NewService(Options{
		Sinks: []SinkRegistration{
			{
				Name: "fail",
				Sink: notify.SinkFunc(func(ctx context.Context, payload notify.JobFailurePayload) error {
					return errors.New("boom")
				}),
			},
		},
	})

	svc.NotifyJobFailure(context.Background(), notify.JobFailurePayload{JobID: "123"})
}

func TestServiceSkipsTestBrowserJob(t *testing.T) {
	ctx := context.Background()
	var called bool
	svc := NewService(Options{
		Sinks: []SinkRegistration{
			{
				Name: "capture",
				Sink: notify.SinkFunc(func(ctx context.Context, payload notify.JobFailurePayload) error {
					called = true
					return nil
				}),
			},
		},
	})

	svc.NotifyJobFailure(ctx, notify.JobFailurePayload{
		JobID:   "test-job",
		JobType: string(model.JobTypeBrowser),
		IsTest:  true,
	})

	if called {
		t.Fatal("expected sink not to be invoked for test browser job")
	}
}
