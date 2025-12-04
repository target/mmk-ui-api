package pagerduty

import (
	"strings"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/observability/notify"
)

func TestNewClientValidation(t *testing.T) {
	if _, err := NewClient(Config{}); err == nil {
		t.Fatal("expected error when routing key missing")
	}
}

func TestBuildEventDefaults(t *testing.T) {
	client, err := NewClient(Config{
		RoutingKey: "key",
		Source:     "",
		Component:  "",
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	payload := notify.JobFailurePayload{
		JobID:      "123",
		JobType:    "rules",
		Error:      "boom",
		ErrorClass: "err_class",
	}
	event := client.buildEvent(payload)

	payloadSection, ok := event["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload section")
	}
	if payloadSection["severity"] != notify.SeverityCritical {
		t.Fatalf("expected default severity, got %v", payloadSection["severity"])
	}
	if payloadSection["source"] != "merrymaker" {
		t.Fatalf("expected default source, got %v", payloadSection["source"])
	}
	if payloadSection["component"] != "merrymaker" {
		t.Fatalf("expected default component, got %v", payloadSection["component"])
	}

	custom, ok := payloadSection["custom_details"].(map[string]any)
	if !ok {
		t.Fatalf("expected custom details")
	}

	required := []string{"job_id", "job_type", "error", "error_class"}
	for _, key := range required {
		if _, exists := custom[key]; !exists {
			t.Fatalf("expected key %s in custom details", key)
		}
	}

	dedup, _ := event["dedup_key"].(string)
	if !strings.Contains(dedup, "123") {
		t.Fatalf("expected dedup key to reference job id, got %s", dedup)
	}
}
