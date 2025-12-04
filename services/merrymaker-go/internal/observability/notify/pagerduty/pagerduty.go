package pagerduty

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/observability/notify"
)

// APIEndpoint is the PagerDuty Events API v2 ingest URL.
const APIEndpoint = "https://events.pagerduty.com/v2/enqueue"

// Config captures runtime configuration for the PagerDuty sink.
type Config struct {
	RoutingKey string
	Source     string
	Component  string
	Timeout    time.Duration
	RetryLimit int
	Client     *http.Client
}

// Client publishes events via PagerDuty's Events API v2.
type Client struct {
	routingKey string
	source     string
	component  string
	retryLimit int
	client     *http.Client
}

// NewClient constructs a PagerDuty events client from config. Callers must provide a routing key.
func NewClient(cfg Config) (*Client, error) {
	key := strings.TrimSpace(cfg.RoutingKey)
	if key == "" {
		return nil, errors.New("pagerduty routing key is required")
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	retries := max(cfg.RetryLimit, 0)

	hc := cfg.Client
	if hc == nil {
		hc = &http.Client{Timeout: timeout}
	}

	return &Client{
		routingKey: key,
		source:     fallbackString(strings.TrimSpace(cfg.Source), "merrymaker"),
		component:  fallbackString(strings.TrimSpace(cfg.Component), "merrymaker"),
		retryLimit: retries,
		client:     hc,
	}, nil
}

// SendJobFailure submits a trigger event to PagerDuty.
func (c *Client) SendJobFailure(ctx context.Context, payload notify.JobFailurePayload) error {
	event := c.buildEvent(payload)
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode pagerduty payload: %w", err)
	}

	attempts := c.retryLimit + 1
	var lastErr error
	for attempt := range attempts {
		err = c.submit(ctx, body)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt < attempts-1 {
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

func (c *Client) buildEvent(payload notify.JobFailurePayload) map[string]any {
	severity := fallbackString(strings.ToLower(payload.Severity), notify.SeverityCritical)
	if severity == "" {
		severity = notify.SeverityCritical
	}

	occurredAt := payload.OccurredAt.UTC()
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	custom := map[string]any{
		"job_id":      payload.JobID,
		"job_type":    payload.JobType,
		"site_id":     payload.SiteID,
		"scope":       payload.Scope,
		"error":       payload.Error,
		"error_class": payload.ErrorClass,
	}

	for k, v := range payload.Metadata {
		if _, exists := custom[k]; !exists {
			custom[k] = v
		}
	}

	dedupKey := fmt.Sprintf("%s:%s", payload.JobType, payload.JobID)
	dedupKey = strings.Trim(dedupKey, ":")

	return map[string]any{
		"routing_key":  c.routingKey,
		"event_action": "trigger",
		"dedup_key":    dedupKey,
		"payload": map[string]any{
			"summary": fmt.Sprintf(
				"Job %s (%s) failed",
				fallbackString(payload.JobID, "unknown"),
				fallbackString(payload.JobType, "unknown"),
			),
			"severity":       severity,
			"source":         c.source,
			"component":      c.component,
			"timestamp":      occurredAt.Format(time.RFC3339),
			"custom_details": custom,
		},
	}
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func (c *Client) submit(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, APIEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create pagerduty request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("pagerduty request failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.handleErrorResponse(resp)
	}

	return drainPagerDutySuccess(resp)
}

func drainPagerDutySuccess(resp *http.Response) error {
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		closeErr := resp.Body.Close()
		if closeErr != nil {
			return errors.Join(
				fmt.Errorf("drain pagerduty response body: %w", err),
				fmt.Errorf("close response body: %w", closeErr),
			)
		}
		return fmt.Errorf("drain pagerduty response body: %w", err)
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
				fmt.Errorf("read pagerduty error response: %w", readErr),
				fmt.Errorf("close response body: %w", closeErr),
			)
		}
		return fmt.Errorf("read pagerduty error response: %w", readErr)
	}
	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("close response body: %w", err)
	}

	return fmt.Errorf("pagerduty api %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
}
