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
		SiteName:   "Friendly Site",
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
	if !containsAll(
		text,
		[]string{"Job failure alert", "123", "rules", "site-1", "Friendly Site", "global", "boom", "test_error"},
	) {
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

func TestFormatMessageEscapesSiteName(t *testing.T) {
	client, err := NewClient(Config{
		WebhookURL: "https://hooks.slack.com/services/test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg := client.formatMessage(notify.JobFailurePayload{
		SiteID:   "site-123",
		SiteName: "test & <site>",
	})

	text, ok := msg["text"].(string)
	if !ok {
		t.Fatalf("expected text field")
	}

	if !strings.Contains(text, "test &amp; &lt;site&gt;") {
		t.Fatalf("expected escaped site name, got: %s", text)
	}
}

func TestFormatSiteValuePermutations(t *testing.T) {
	tcs := []struct {
		name    string
		siteID  string
		site    string
		prefix  string
		want    string
		wantErr bool
	}{
		{
			name:   "id with link",
			siteID: "site-1",
			prefix: "https://app.example/sites",
			want:   "<https://app.example/sites/site-1|site-1>",
		},
		{
			name:   "name only",
			site:   "Friendly",
			prefix: "https://app.example/sites",
			want:   "Friendly",
		},
		{
			name:   "id and name with link",
			siteID: "site-2",
			site:   "Friendly",
			prefix: "https://app.example/sites",
			want:   "<https://app.example/sites/site-2|Friendly> (site-2)",
		},
		{
			name:   "id and name without link",
			siteID: "site-3",
			site:   "Friendly",
			prefix: "not a url",
			want:   "Friendly (site-3)",
		},
		{
			name:   "empty inputs",
			want:   "",
			site:   "",
			prefix: "https://app.example/sites",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewClient(Config{
				WebhookURL:    "https://hooks.slack.com/services/test",
				SiteURLPrefix: tc.prefix,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := client.formatSiteValue(tc.siteID, tc.site)
			if got != tc.want {
				t.Fatalf("formatSiteValue(%q,%q) = %q, want %q", tc.siteID, tc.site, got, tc.want)
			}
		})
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
