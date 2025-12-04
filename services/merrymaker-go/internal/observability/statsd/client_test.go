package statsd

import (
	"net"
	"strings"
	"testing"
)

func TestSanitizePrefix(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"  metrics.app  ": "metrics.app",
		"..foo..":         "foo",
		".":               "",
		"":                "",
	}

	for input, want := range tests {
		if got := sanitizePrefix(input); got != want {
			t.Fatalf("sanitizePrefix(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeMetricName(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		" job/metric ":  "job_metric",
		"foo..bar":      "foo.bar",
		"multi  space":  "multi__space",
		"slash/name/id": "slash_name_id",
	}

	for input, want := range tests {
		if got := normalizeMetricName(input); got != want {
			t.Fatalf("normalizeMetricName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFormatTags(t *testing.T) {
	t.Parallel()

	global := map[string]string{
		"env": "prod",
		// Intentionally padded key/value to ensure trimming logic works.
		//nolint:gocritic // whitespace is part of the test case
		" service ": " rules ",
	}
	local := map[string]string{
		"result": " success ",
		"":       "ignored",
		"env":    "stage",
	}

	got := formatTags(global, local)
	want := "|#env:stage,result:success,service:rules"

	if got != want {
		t.Fatalf("formatTags mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestFormatTagsEmpty(t *testing.T) {
	t.Parallel()

	if got := formatTags(nil, nil); got != "" {
		t.Fatalf("formatTags(nil, nil) = %q, want empty string", got)
	}
}

func TestCloneTagsReturnsCopy(t *testing.T) {
	t.Parallel()

	original := map[string]string{
		"env": "prod",
		"":    "ignored",
	}

	cloned := cloneTags(original)
	if cloned == nil {
		t.Fatal("cloneTags returned nil map")
	}

	cloned["env"] = "stage"
	if original["env"] != "prod" {
		t.Fatal("cloneTags did not copy values")
	}

	if _, ok := cloned[""]; ok {
		t.Fatal("cloneTags kept empty key")
	}
}

func TestClientEnabledAndClose(t *testing.T) {
	t.Parallel()

	clientConn, peerConn := net.Pipe()
	defer peerConn.Close()

	client := &Client{
		enabled: true,
		conn:    clientConn,
	}

	if !client.Enabled() {
		t.Fatal("expected client.Enabled to report true with active connection")
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	if client.Enabled() {
		t.Fatal("expected client.Enabled to report false after Close")
	}

	// Verify Close can be called again without error.
	if err := client.Close(); err != nil {
		t.Fatalf("Close (second call) error: %v", err)
	}

	var nilClient *Client
	if nilClient.Enabled() {
		t.Fatal("nil client should report disabled")
	}
	if err := nilClient.Close(); err != nil {
		t.Fatalf("nil client Close error: %v", err)
	}
}

func TestNewClientDisabledWithoutAddress(t *testing.T) {
	t.Parallel()

	client, err := NewClient(Config{
		Enabled: true,
		Address: "   ",
	})
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	if client.Enabled() {
		t.Fatal("expected client to stay disabled when address is empty")
	}
}

func TestNewClientDialError(t *testing.T) {
	t.Parallel()

	_, err := NewClient(Config{
		Enabled: true,
		Address: "bad address",
	})
	if err == nil {
		t.Fatal("expected NewClient to error for invalid address")
	}
	if !strings.Contains(err.Error(), "statsd dial") {
		t.Fatalf("unexpected error: %v", err)
	}
}
