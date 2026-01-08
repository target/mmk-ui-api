package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/observability/notify"
)

// Config captures the subset of Slack webhook behaviour we need.
type Config struct {
	WebhookURL    string
	Channel       string
	Username      string
	Timeout       time.Duration
	RetryLimit    int
	Client        *http.Client
	SiteURLPrefix string
}

// Client delivers job failure notifications to a Slack webhook.
type Client struct {
	webhookURL    string
	channel       string
	username      string
	retryLimit    int
	siteURLPrefix string
	client        *http.Client
}

// NewClient builds a Slack webhook client. Callers should pass a validated config.
func NewClient(cfg Config) (*Client, error) {
	webhookURL := strings.TrimSpace(cfg.WebhookURL)
	if webhookURL == "" {
		return nil, errors.New("slack webhook url is required")
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	retries := cfg.RetryLimit
	if retries < 0 {
		retries = 0
	}

	hc := cfg.Client
	if hc == nil {
		hc = &http.Client{Timeout: timeout}
	}

	return &Client{
		webhookURL:    webhookURL,
		channel:       strings.TrimSpace(cfg.Channel),
		username:      fallbackString(strings.TrimSpace(cfg.Username), "merrymaker"),
		retryLimit:    retries,
		siteURLPrefix: strings.TrimSpace(cfg.SiteURLPrefix),
		client:        hc,
	}, nil
}

// SendJobFailure posts a formatted message to Slack.
func (c *Client) SendJobFailure(ctx context.Context, payload notify.JobFailurePayload) error {
	msg := c.formatMessage(payload)
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("encode slack payload: %w", err)
	}

	attempts := c.retryLimit + 1
	var lastErr error
	for attempt := range attempts {
		err = c.post(ctx, body)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt < attempts-1 {
			// Simple linear backoff to avoid thundering retries.
			delay := time.Duration(attempt+1) * 200 * time.Millisecond
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return ctx.Err()
			case <-timer.C:
			}
		}
	}

	return lastErr
}

func (c *Client) formatMessage(payload notify.JobFailurePayload) map[string]any {
	timestamp := payload.OccurredAt
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	text := strings.Builder{}
	writeSlackHeader(&text, payload)
	siteDisplay := c.formatSiteValue(payload.SiteID, payload.SiteName)
	appendSlackDetails(&text, payload, siteDisplay)
	appendSlackMetadata(&text, payload.Metadata)
	writeSlackTimestamp(&text, timestamp)

	msg := map[string]any{
		"text":     text.String(),
		"username": c.username,
	}
	if c.channel != "" {
		msg["channel"] = c.channel
	}
	return msg
}

func fallbackString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (c *Client) post(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack request failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.handleErrorResponse(resp)
	}

	return drainSlackSuccess(resp)
}

func writeSlackHeader(text *strings.Builder, payload notify.JobFailurePayload) {
	text.WriteString("*Job failure alert*")
	if payload.JobID != "" {
		text.WriteString(" `")
		text.WriteString(payload.JobID)
		text.WriteByte('`')
	}
	if payload.JobType != "" {
		text.WriteString(" (")
		text.WriteString(payload.JobType)
		text.WriteByte(')')
	}
	text.WriteByte('\n')
}

func appendSlackDetails(text *strings.Builder, payload notify.JobFailurePayload, siteValue string) {
	fields := []struct {
		label string
		value string
	}{
		{"Severity", fallbackString(payload.Severity, notify.SeverityCritical)},
		{"Site", siteValue},
		{"Scope", payload.Scope},
		{"Error class", payload.ErrorClass},
		{"Error", payload.Error},
	}

	for _, field := range fields {
		appendSlackField(text, field.label, field.value)
	}
}

func (c *Client) formatSiteValue(siteID, siteName string) string {
	rawID := strings.TrimSpace(siteID)
	rawName := strings.TrimSpace(siteName)
	id := escapeSlackText(rawID)
	name := escapeSlackText(rawName)

	if id == "" && name == "" {
		return ""
	}

	link := ""
	if rawID != "" {
		link = c.buildSiteLink(rawID)
	}

	switch {
	case link != "" && name != "":
		return fmt.Sprintf("<%s|%s> (%s)", link, name, id)
	case link != "":
		return fmt.Sprintf("<%s|%s>", link, id)
	case name != "" && id != "":
		return fmt.Sprintf("%s (%s)", name, id)
	case name != "":
		return name
	default:
		return id
	}
}

func escapeSlackText(value string) string {
	if value == "" {
		return ""
	}
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	).Replace(value)
}

func (c *Client) buildSiteLink(siteID string) string {
	prefix := strings.TrimSpace(c.siteURLPrefix)
	if prefix == "" {
		return ""
	}

	u, err := url.Parse(prefix)
	if err != nil {
		return ""
	}
	if u.Scheme == "" || u.Host == "" {
		return ""
	}

	link, err := url.JoinPath(u.String(), siteID)
	if err != nil {
		return ""
	}

	return link
}

func drainSlackSuccess(resp *http.Response) error {
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		closeErr := resp.Body.Close()
		if closeErr != nil {
			return errors.Join(
				fmt.Errorf("drain slack response body: %w", err),
				fmt.Errorf("close response body: %w", closeErr),
			)
		}
		return fmt.Errorf("drain slack response body: %w", err)
	}
	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("close response body: %w", err)
	}
	return nil
}

func (c *Client) handleErrorResponse(resp *http.Response) error {
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		closeErr := resp.Body.Close()
		if closeErr != nil {
			return errors.Join(
				fmt.Errorf("read slack error response: %w", readErr),
				fmt.Errorf("close response body: %w", closeErr),
			)
		}
		return fmt.Errorf("read slack error response: %w", readErr)
	}
	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("close response body: %w", err)
	}

	return fmt.Errorf("slack webhook %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
}

func appendSlackField(text *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	text.WriteString("• ")
	text.WriteString(label)
	text.WriteString(": ")
	text.WriteString(value)
	text.WriteByte('\n')
}

func appendSlackMetadata(text *strings.Builder, metadata map[string]string) {
	if len(metadata) == 0 {
		return
	}
	text.WriteString("• Metadata:\n")
	keys := make([]string, 0, len(metadata))
	for k := range metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := metadata[k]
		text.WriteString("    • ")
		text.WriteString(k)
		text.WriteString(": ")
		text.WriteString(v)
		text.WriteByte('\n')
	}
}

func writeSlackTimestamp(text *strings.Builder, timestamp time.Time) {
	text.WriteString("• Timestamp: ")
	text.WriteString(timestamp.UTC().Format(time.RFC3339))
}
