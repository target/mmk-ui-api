package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

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
func (s *AlertSinkService) ValidateSinkConfiguration(ctx context.Context, sink model.HTTPAlertSink) error {
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

// ScheduleAlert enqueues an alert job for the given sink and payload.
func (s *AlertSinkService) ScheduleAlert(
	ctx context.Context,
	sink *model.HTTPAlertSink,
	eventPayload json.RawMessage,
) (*model.Job, error) {
	jobPayload := struct {
		SinkID  string          `json:"sink_id"`
		Payload json.RawMessage `json:"payload"`
	}{SinkID: sink.ID, Payload: eventPayload}
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
		panic(err) //nolint:forbidigo // Must constructor fails fast when dependencies are invalid during startup
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
func (s *HTTPAlertSinkService) GetByID(ctx context.Context, id string) (*model.HTTPAlertSink, error) {
	sink, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get HTTP alert sink by id: %w", err)
	}
	return sink, nil
}

// GetByName retrieves an HTTP alert sink by its name.
func (s *HTTPAlertSinkService) GetByName(ctx context.Context, name string) (*model.HTTPAlertSink, error) {
	sink, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get HTTP alert sink by name: %w", err)
	}
	return sink, nil
}

// List retrieves a list of HTTP alert sinks with pagination.
func (s *HTTPAlertSinkService) List(ctx context.Context, limit, offset int) ([]*model.HTTPAlertSink, error) {
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
