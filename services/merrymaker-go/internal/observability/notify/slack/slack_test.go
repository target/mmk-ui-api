package slack

import (
	"strings"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/observability/notify"
)

func TestNewClientValidation(t *testing.T) {
	if _, err := NewClient(Config{}); err == nil {
		t.Fatal("expected error when webhook url missing")
	}
}

func TestFormatMessageIncludesFields(t *testing.T) {
	client, err := NewClient(Config{
		WebhookURL: "https://hooks.slack.com/services/test",
		Channel:    "#alerts",
		Username:   "bot",
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg := client.formatMessage(notify.JobFailurePayload{
		JobID:      "123",
		JobType:    "rules",
		SiteID:     "site-1",
		Scope:      "global",
		Error:      "boom",
		ErrorClass: "test_error",
	})

	if msg["username"] != "bot" {
		t.Fatalf("expected username to be preserved, got %v", msg["username"])
	}
	if msg["channel"] != "#alerts" {
		t.Fatalf("expected channel to be set, got %v", msg["channel"])
	}

	text, ok := msg["text"].(string)
	if !ok {
		t.Fatalf("expected text field")
	}
	if !containsAll(text, []string{"Job failure alert", "123", "rules", "site-1", "global", "boom", "test_error"}) {
		t.Fatalf("message text missing fields: %s", text)
	}
}

func TestFormatMessageSiteLink(t *testing.T) {
	client, err := NewClient(Config{
		WebhookURL:    "https://hooks.slack.com/services/test",
		SiteURLPrefix: "https://app.merrymaker.local/sites",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg := client.formatMessage(notify.JobFailurePayload{
		SiteID: "site-123",
	})

	text, ok := msg["text"].(string)
	if !ok {
		t.Fatalf("expected text field")
	}

	expected := "<https://app.merrymaker.local/sites/site-123|site-123>"
	if !strings.Contains(text, expected) {
		t.Fatalf("expected site link %q in text: %s", expected, text)
	}
}

func containsAll(text string, substrs []string) bool {
	for _, s := range substrs {
		if !strings.Contains(text, s) {
			return false
		}
	}
	return true
}
