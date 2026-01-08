package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	jmespath "github.com/jmespath-community/go-jmespath"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// JMESPathEvaluator abstracts JMESPath operations for testability.
type JMESPathEvaluator interface {
	Validate(expr string) error
	Evaluate(expr string, data any) (any, error)
}

// jmespathLibEvaluator implements JMESPathEvaluator using go-jmespath.
type jmespathLibEvaluator struct{}

func (j jmespathLibEvaluator) Validate(expr string) error {
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	_, err := jmespath.Compile(expr)
	return err
}

func (j jmespathLibEvaluator) Evaluate(expr string, data any) (any, error) {
	return jmespath.Search(expr, data)
}

// PreparedHTTPRequest is the result of processing a sink config with a payload.
type PreparedHTTPRequest struct {
	Method   string
	URL      string
	Headers  map[string]string
	Body     []byte
	OkStatus int
	// Secrets maps placeholder tokens (e.g. "__API_KEY__") to their resolved values.
	// This enables downstream consumers (like the job runner) to redact secrets before persisting artifacts.
	Secrets map[string]string
}

// AlertPayload wraps alert data with additional context for JMESPath processing.
// This structure is available to JMESPath expressions in alert sink configurations,
// enabling access to alert details, site information, and UI URLs.
type AlertPayload struct {
	Alert    json.RawMessage `json:"alert"`     // The complete alert object
	SiteName string          `json:"site_name"` // Name of the site that triggered the alert
	AlertURL string          `json:"alert_url"` // URL to view the alert in the UI
}

// AlertSinkServiceOptions groups dependencies for AlertSinkService.
type AlertSinkServiceOptions struct {
	JobRepo    core.JobRepository
	SecretRepo core.SecretRepository
	Evaluator  JMESPathEvaluator
}

// AlertSinkService encapsulates business logic for HTTP alert sinks.
type AlertSinkService struct {
	jobs core.JobRepository
	secs core.SecretRepository
	jems JMESPathEvaluator
}

// NewAlertSinkService constructs a new service.
func NewAlertSinkService(opts AlertSinkServiceOptions) *AlertSinkService {
	jems := opts.Evaluator
	if jems == nil {
		jems = jmespathLibEvaluator{}
	}
	return &AlertSinkService{jobs: opts.JobRepo, secs: opts.SecretRepo, jems: jems}
}

// ResolveSecrets replaces __NAME__ placeholders in URI/Body/Headers/QueryParams using secrets fetched by name.
func (s *AlertSinkService) ResolveSecrets(
	ctx context.Context,
	sink model.HTTPAlertSink,
) (model.HTTPAlertSink, map[string]string, error) {
	vals := make(map[string]string, len(sink.Secrets))
	for _, name := range sink.Secrets {
		sec, err := s.secs.GetByName(ctx, name)
		if err != nil {
			return sink, nil, fmt.Errorf("resolve secret %q: %w", name, err)
		}
		vals[name] = sec.Value
	}

	replaceAll := func(in *string) *string {
		if in == nil {
			return nil
		}
		out := *in
		for k, v := range vals {
			placeholder := "__" + k + "__"
			out = strings.ReplaceAll(out, placeholder, v)
		}
		return &out
	}

	// Replace in URI (string field)
	sink.URI = replaceAllStr(sink.URI, vals)
	if sink.Body != nil {
		sink.Body = replaceAll(sink.Body)
	}
	if sink.Headers != nil {
		sink.Headers = replaceAll(sink.Headers)
	}
	if sink.QueryParams != nil {
		sink.QueryParams = replaceAll(sink.QueryParams)
	}
	placeholders := make(map[string]string, len(vals))
	for name, value := range vals {
		if value == "" {
			continue
		}
		placeholder := "__" + name + "__"
		placeholders[placeholder] = value
	}
	return sink, placeholders, nil
}

// ValidateSinkConfiguration validates JMESPath (if present), resolves secrets, and validates the URI.
func (s *AlertSinkService) ValidateSinkConfiguration(
	ctx context.Context,
	sink model.HTTPAlertSink,
) error {
	if sink.Body != nil && strings.TrimSpace(*sink.Body) != "" {
		if err := s.jems.Validate(*sink.Body); err != nil {
			return fmt.Errorf("invalid body JMESPath: %w", err)
		}
	}
	// Ensure secrets can be fetched and applied
	resolved, _, err := s.ResolveSecrets(ctx, sink)
	if err != nil {
		return err
	}
	// Validate resolved base URI strictly
	u, err := url.Parse(resolved.URI)
	if err != nil {
		return fmt.Errorf("invalid base URI: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid URI scheme: %s", u.Scheme)
	}
	if strings.TrimSpace(u.Host) == "" {
		return errors.New("invalid URI: missing host")
	}
	return nil
}

// ProcessSinkConfiguration produces a PreparedHTTPRequest by applying secrets and deriving body.
func (s *AlertSinkService) ProcessSinkConfiguration(
	ctx context.Context,
	sink model.HTTPAlertSink,
	payload json.RawMessage,
) (*PreparedHTTPRequest, error) {
	resolved, secrets, err := s.ResolveSecrets(ctx, sink)
	if err != nil {
		return nil, err
	}

	method := strings.ToUpper(strings.TrimSpace(resolved.Method))
	if method == "" {
		return nil, errors.New("method is required")
	}

	u, err := buildURLWithQuery(resolved.URI, resolved.QueryParams)
	if err != nil {
		return nil, err
	}

	headers, err := parseHeaders(resolved.Headers)
	if err != nil {
		return nil, err
	}

	bodyBytes, err := s.deriveBody(resolved.Body, payload)
	if err != nil {
		return nil, err
	}

	if len(bodyBytes) > 0 {
		hasCT := false
		for k := range headers {
			if strings.EqualFold(k, "Content-Type") {
				hasCT = true
				break
			}
		}
		if !hasCT {
			headers["Content-Type"] = "application/json"
		}
	}

	okStatus := resolved.OkStatus
	if okStatus == 0 {
		okStatus = 200
	}

	return &PreparedHTTPRequest{
		Method:   method,
		URL:      u,
		Headers:  headers,
		Body:     bodyBytes,
		OkStatus: okStatus,
		Secrets:  secrets,
	}, nil
}

func buildURLWithQuery(base string, qp *string) (string, error) {
	u := strings.TrimRight(base, "?&")
	q := strings.TrimSpace(ptrVal(qp))
	if q != "" {
		q = strings.TrimLeft(q, "?&")
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		u = u + sep + q
	}
	if _, err := url.Parse(u); err != nil {
		return "", fmt.Errorf("invalid URL after resolution: %w", err)
	}
	return u, nil
}

func parseHeaders(hs *string) (map[string]string, error) {
	headers := make(map[string]string)
	s := strings.TrimSpace(ptrVal(hs))
	if s == "" {
		return headers, nil
	}

	// Prefer JSON object if provided
	if strings.HasPrefix(s, "{") {
		return parseJSONHeaders(s)
	}

	// Fallback: line-based "Key: Value" entries (legacy)
	return parseLegacyHeaders(s)
}

// parseJSONHeaders handles JSON object format headers.
func parseJSONHeaders(s string) (map[string]string, error) {
	headers := make(map[string]string)
	var raw map[string]any
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, fmt.Errorf("invalid headers JSON: %w", err)
	}

	for k, v := range raw {
		if k = strings.TrimSpace(k); k == "" {
			continue
		}
		headers[k] = convertHeaderValue(v)
	}
	return headers, nil
}

// convertHeaderValue converts various JSON value types to header string values.
func convertHeaderValue(v any) string {
	switch tv := v.(type) {
	case string:
		return tv
	case []any:
		var parts []string
		for _, item := range tv {
			if str, ok := item.(string); ok {
				parts = append(parts, str)
				continue
			}
			b, err := json.Marshal(item)
			if err != nil {
				parts = append(parts, fmt.Sprintf("%v", item))
				continue
			}
			parts = append(parts, string(b))
		}
		return strings.Join(parts, ", ")
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// parseLegacyHeaders handles line-based "Key: Value" format headers.
func parseLegacyHeaders(s string) (map[string]string, error) {
	headers := make(map[string]string)
	for _, line := range splitHeaders(s) {
		if line == "" {
			continue
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid header entry: %q", line)
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		if k == "" {
			return nil, fmt.Errorf("empty header name in entry: %q", line)
		}
		if existing, ok := headers[k]; ok && existing != "" {
			headers[k] = existing + ", " + v
		} else {
			headers[k] = v
		}
	}
	return headers, nil
}

func (s *AlertSinkService) deriveBody(expr *string, payload json.RawMessage) ([]byte, error) {
	bExpr := strings.TrimSpace(ptrVal(expr))
	if bExpr == "" {
		return payload, nil
	}
	var data any
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("invalid payload JSON: %w", err)
	}
	res, err := s.jems.Evaluate(bExpr, data)
	if err != nil {
		return nil, fmt.Errorf("evaluate body JMESPath: %w", err)
	}
	b, err := json.Marshal(res)
	if err != nil {
		return nil, fmt.Errorf("marshal derived body: %w", err)
	}
	return b, nil
}

// ScheduleAlert enqueues an alert job for the given sink and enriched alert payload.
// The alertPayload should be an AlertPayload struct containing alert data, site name, and alert URL.
func (s *AlertSinkService) ScheduleAlert(
	ctx context.Context,
	sink *model.HTTPAlertSink,
	alertPayload json.RawMessage,
) (*model.Job, error) {
	jobPayload := struct {
		SinkID  string          `json:"sink_id"`
		Payload json.RawMessage `json:"payload"`
	}{SinkID: sink.ID, Payload: alertPayload}
	b, err := json.Marshal(jobPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal job payload: %w", err)
	}
	mr := sink.Retry
	if mr < 0 {
		mr = 0
	}
	req := &model.CreateJobRequest{
		Type:       model.JobTypeAlert,
		Payload:    b,
		MaxRetries: mr,
	}
	return s.jobs.Create(ctx, req)
}

// TestFireResult contains the outcome of a test fire attempt.
type TestFireResult struct {
	Success      bool                        `json:"success"`
	StatusCode   int                         `json:"status_code"`
	ExpectedCode int                         `json:"expected_code"`
	DurationMs   int64                       `json:"duration_ms"`
	Request      AlertDeliveryRequestSummary `json:"request"`
	Response     *AlertDeliveryResponse      `json:"response,omitempty"`
	ErrorMessage string                      `json:"error_message,omitempty"`
}

// TestFire sends a sample alert payload to the sink and returns the result synchronously.
// This allows users to validate their sink configuration (URI, headers, secrets, JMESPath)
// without creating a real alert or queuing a job.
func (s *AlertSinkService) TestFire(
	ctx context.Context,
	sink *model.HTTPAlertSink,
	httpClient HTTPDoer,
) (*TestFireResult, error) {
	if err := validateTestFireInputs(sink, httpClient); err != nil {
		return nil, err
	}

	start := time.Now()
	result := initTestFireResult()

	// Process sink configuration (resolve secrets, build URL/body/headers)
	preq, err := s.ProcessSinkConfiguration(ctx, *sink, buildTestFireSamplePayload())
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("failed to prepare request: %v", err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result, nil
	}

	// Use the prepared request's OkStatus as the single source of truth for expectations
	result.ExpectedCode = preq.OkStatus
	if result.ExpectedCode == 0 {
		result.ExpectedCode = 200
	}

	// Record request details (with secrets redacted)
	recordRedactedRequest(result, preq)

	// Send the HTTP request and finalize result
	response, reqErr := s.sendTestFireRequest(ctx, httpClient, preq)
	result.DurationMs = time.Since(start).Milliseconds()
	finalizeTestFireResult(result, response, reqErr)

	return result, nil
}

func validateTestFireInputs(sink *model.HTTPAlertSink, httpClient HTTPDoer) error {
	if sink == nil {
		return errors.New("sink is required")
	}
	if httpClient == nil {
		return errors.New("http client is required")
	}
	return nil
}

func initTestFireResult() *TestFireResult {
	return &TestFireResult{}
}

func recordRedactedRequest(result *TestFireResult, preq *PreparedHTTPRequest) {
	redactor := NewSecretRedactor(preq.Secrets)
	result.Request = AlertDeliveryRequestSummary{
		Method:   preq.Method,
		URL:      redactor.RedactString(preq.URL),
		Headers:  redactor.RedactHeaders(preq.Headers),
		OkStatus: preq.OkStatus,
	}
	if len(preq.Body) > 0 {
		bodyStr := redactor.RedactString(string(preq.Body))
		if len(bodyStr) > maxTestFireBodyBytes {
			bodyStr = bodyStr[:maxTestFireBodyBytes]
			result.Request.BodyTruncated = true
		}
		result.Request.Body = bodyStr
	}
}

func finalizeTestFireResult(result *TestFireResult, response *AlertDeliveryResponse, reqErr error) {
	if response != nil {
		result.Response = response
		result.StatusCode = response.StatusCode
	}
	if reqErr != nil {
		result.ErrorMessage = reqErr.Error()
		return
	}
	if result.StatusCode == result.ExpectedCode {
		result.Success = true
	} else {
		result.ErrorMessage = fmt.Sprintf("unexpected status: got %d, want %d", result.StatusCode, result.ExpectedCode)
	}
}

// HTTPDoer abstracts http.Client for testing.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

const maxTestFireBodyBytes = 4 * 1024 // 4KB limit for request/response bodies

// sendTestFireRequest executes the HTTP request for test fire.
func (s *AlertSinkService) sendTestFireRequest(
	ctx context.Context,
	client HTTPDoer,
	preq *PreparedHTTPRequest,
) (*AlertDeliveryResponse, error) {
	var body io.Reader
	if len(preq.Body) > 0 {
		body = bytes.NewReader(preq.Body)
	}

	req, err := http.NewRequestWithContext(ctx, preq.Method, preq.URL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	for k, v := range preq.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	// Read response body (with size limit)
	respBody, truncated, readErr := readLimitedBody(resp.Body, maxTestFireBodyBytes)
	closeErr := resp.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read response: %w", readErr)
	}

	response := &AlertDeliveryResponse{
		StatusCode:    resp.StatusCode,
		Headers:       flattenHeaders(resp.Header),
		Body:          string(respBody),
		BodyTruncated: truncated,
	}

	if closeErr != nil {
		return response, fmt.Errorf("close response body: %w", closeErr)
	}

	return response, nil
}

// readLimitedBody reads up to maxBytes from the reader, returning whether it was truncated.
func readLimitedBody(r io.Reader, maxBytes int) ([]byte, bool, error) {
	buf, err := io.ReadAll(io.LimitReader(r, int64(maxBytes+1)))
	if err != nil {
		return nil, false, err
	}
	truncated := len(buf) > maxBytes
	if truncated {
		buf = buf[:maxBytes]
	}
	return buf, truncated, nil
}

// flattenHeaders converts http.Header to a simple map (first value only).
func flattenHeaders(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}

// buildTestFireSamplePayload generates a sample alert payload for test firing.
func buildTestFireSamplePayload() json.RawMessage {
	sampleTime := time.Now().UTC()
	alertID := uuid.NewString()
	eventID := uuid.NewString()
	ruleID := uuid.NewString()

	eventContext := map[string]any{
		"domain":      "test-domain.example.com",
		"host":        "test-domain.example.com",
		"scope":       "test",
		"site_id":     "site-test",
		"job_id":      "job-test-fire",
		"event_id":    eventID,
		"request_url": "https://test-domain.example.com/test.js",
		"page_url":    "https://your-site.example.com/page",
		"referrer":    "https://referrer.example.com",
		"user_agent":  "MerrymakerTestFire/1.0",
	}

	sample := map[string]any{
		"id":            alertID,
		"alert_id":      alertID,
		"site_id":       "site-test",
		"rule_id":       ruleID,
		"rule_type":     "unknown_domain",
		"severity":      "medium",
		"title":         "[TEST] Sample alert for sink validation",
		"description":   "This is a test alert sent to validate the HTTP alert sink configuration.",
		"event_context": eventContext,
		"events": []map[string]any{
			{
				"context":     eventContext,
				"description": "This is a test alert sent to validate the HTTP alert sink configuration.",
				"rule_type":   "unknown_domain",
				"site":        nil,
				"timestamp":   sampleTime,
			},
		},
		"hits":            1,
		"source":          "manual",
		"test":            true,
		"metadata":        map[string]any{"test_fire": true},
		"delivery_status": "pending",
		"fired_at":        sampleTime,
		"timestamp":       sampleTime,
		"resolved_at":     nil,
		"resolved_by":     nil,
		"created_at":      sampleTime,
	}
	b, err := json.Marshal(sample)
	if err != nil {
		// Fallback to empty object on marshal error (should never happen with static data)
		return json.RawMessage(`{}`)
	}
	return b
}

func ptrVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func splitHeaders(s string) []string {
	// Normalize Windows newlines
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	var out []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// replaceAllStr replaces __NAME__ placeholders in a plain string.
func replaceAllStr(in string, vals map[string]string) string {
	out := in
	for k, v := range vals {
		placeholder := "__" + k + "__"
		out = strings.ReplaceAll(out, placeholder, v)
	}
	return out
}

// HTTPAlertSinkServiceOptions groups dependencies for HTTPAlertSinkService.
type HTTPAlertSinkServiceOptions struct {
	Repo   core.HTTPAlertSinkRepository // Required: HTTP alert sink repository
	Logger *slog.Logger                 // Optional: structured logger
}

// HTTPAlertSinkService provides business logic for HTTP alert sink CRUD operations.
type HTTPAlertSinkService struct {
	repo   core.HTTPAlertSinkRepository
	logger *slog.Logger
}

// NewHTTPAlertSinkService constructs a new HTTPAlertSinkService.
func NewHTTPAlertSinkService(opts HTTPAlertSinkServiceOptions) (*HTTPAlertSinkService, error) {
	if opts.Repo == nil {
		return nil, errors.New("HTTPAlertSinkRepository is required")
	}

	// Create component-scoped logger
	var logger *slog.Logger
	if opts.Logger != nil {
		logger = opts.Logger.With("component", "http_alert_sink_service")
		logger.Debug("HTTPAlertSinkService initialized")
	}

	return &HTTPAlertSinkService{
		repo:   opts.Repo,
		logger: logger,
	}, nil
}

// MustNewHTTPAlertSinkService constructs a new HTTPAlertSinkService and panics on error.
// Use this when you want fail-fast behavior during application startup.
func MustNewHTTPAlertSinkService(opts HTTPAlertSinkServiceOptions) *HTTPAlertSinkService {
	service, err := NewHTTPAlertSinkService(opts)
	if err != nil {
		//nolint:forbidigo // Must constructor requires fail-fast on initialization errors
		panic(
			err,
		)
	}
	return service
}

// Create creates a new HTTP alert sink with the given request parameters.
func (s *HTTPAlertSinkService) Create(
	ctx context.Context,
	req *model.CreateHTTPAlertSinkRequest,
) (*model.HTTPAlertSink, error) {
	if req == nil {
		return nil, errors.New("create http alert sink request is required")
	}

	sink, err := s.repo.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create HTTP alert sink: %w", err)
	}

	// Optional: Log success (avoid logging sensitive names)
	if s.logger != nil && sink != nil {
		s.logger.DebugContext(ctx, "HTTP alert sink created", "id", sink.ID)
	}

	return sink, nil
}

// GetByID retrieves an HTTP alert sink by its ID.
func (s *HTTPAlertSinkService) GetByID(
	ctx context.Context,
	id string,
) (*model.HTTPAlertSink, error) {
	sink, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get HTTP alert sink by id: %w", err)
	}
	return sink, nil
}

// GetByName retrieves an HTTP alert sink by its name.
func (s *HTTPAlertSinkService) GetByName(
	ctx context.Context,
	name string,
) (*model.HTTPAlertSink, error) {
	sink, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get HTTP alert sink by name: %w", err)
	}
	return sink, nil
}

// List retrieves a list of HTTP alert sinks with pagination.
func (s *HTTPAlertSinkService) List(
	ctx context.Context,
	limit, offset int,
) ([]*model.HTTPAlertSink, error) {
	sinks, err := s.repo.List(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list HTTP alert sinks: %w", err)
	}
	return sinks, nil
}

// Update updates an existing HTTP alert sink with the given request parameters.
func (s *HTTPAlertSinkService) Update(
	ctx context.Context,
	id string,
	req *model.UpdateHTTPAlertSinkRequest,
) (*model.HTTPAlertSink, error) {
	if req == nil {
		return nil, errors.New("update http alert sink request is required")
	}

	sink, err := s.repo.Update(ctx, id, req)
	if err != nil {
		return nil, fmt.Errorf("update HTTP alert sink: %w", err)
	}

	// Optional: Log success (avoid logging sensitive names)
	if s.logger != nil && sink != nil {
		s.logger.DebugContext(ctx, "HTTP alert sink updated", "id", sink.ID)
	}

	return sink, nil
}

// Delete deletes an HTTP alert sink by its ID.
func (s *HTTPAlertSinkService) Delete(ctx context.Context, id string) (bool, error) {
	deleted, err := s.repo.Delete(ctx, id)
	if err != nil {
		return false, fmt.Errorf("delete HTTP alert sink: %w", err)
	}

	// Optional: Log success
	if s.logger != nil && deleted {
		s.logger.DebugContext(ctx, "HTTP alert sink deleted", "id", id)
	}

	return deleted, nil
}
