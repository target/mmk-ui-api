package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/bootstrap"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
)

const (
	defaultFireAlertTimeout = 20 * time.Second
	defaultAlertSeverity    = "high"
	defaultAlertRuleType    = "custom"
	defaultAlertTitle       = "Merrymaker HTTP sink test alert"
	defaultAlertDescription = "Manual Merrymaker alert to verify HTTP sink delivery and observability integrations."
)

type fireAlertOptions struct {
	SiteID       string
	SiteName     string
	Severity     string
	RuleType     string
	Title        string
	Description  string
	EventContext string
	Metadata     string
	SkipDispatch bool
	Timeout      time.Duration
}

type scheduledAlertJob struct {
	sink *model.HTTPAlertSink
	job  *model.Job
}

type recordingAlertSinkScheduler struct {
	inner *service.AlertSinkService
	jobs  []scheduledAlertJob
}

type fireAlertDeps struct {
	db         *sql.DB
	SiteRepo   *data.SiteRepo
	AlertRepo  *data.AlertRepo
	SinkRepo   *data.HTTPAlertSinkRepo
	JobRepo    *data.JobRepo
	SecretRepo *data.SecretRepo
}

type createManualAlertParams struct {
	Ctx          context.Context
	Repo         *data.AlertRepo
	Site         *model.Site
	Opts         fireAlertOptions
	EventContext json.RawMessage
	Metadata     json.RawMessage
}

type fireAlertDispatchParams struct {
	Ctx     context.Context
	Deps    fireAlertDeps
	Site    *model.Site
	Alert   *model.Alert
	Skip    bool
	BaseURL string
	Logger  *slog.Logger
}

type createAndPrintAlertParams struct {
	Ctx  context.Context
	Deps fireAlertDeps
	Site *model.Site
	Opts fireAlertOptions
}

func (r *recordingAlertSinkScheduler) ScheduleAlert(
	ctx context.Context,
	sink *model.HTTPAlertSink,
	eventPayload json.RawMessage,
) (*model.Job, error) {
	job, err := r.inner.ScheduleAlert(ctx, sink, eventPayload)
	if err == nil {
		r.jobs = append(r.jobs, scheduledAlertJob{sink: sink, job: job})
	}
	return job, err
}

func runFireHTTPAlert(cmdCtx *commandContext, args []string) (err error) {
	opts, err := parseFireAlertFlags(args)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmdCtx.Ctx, opts.Timeout)
	defer cancel()

	deps, err := openFireAlertDeps(cmdCtx)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := deps.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close fire alert dependencies: %w", cerr))
		}
	}()

	site, err := lookupSite(ctx, deps.SiteRepo, &opts)
	if err != nil {
		return err
	}

	if sinkErr := ensureSiteSink(site); sinkErr != nil {
		return sinkErr
	}

	alert, _, _, err := createAndPrintAlert(&createAndPrintAlertParams{
		Ctx:  ctx,
		Deps: deps,
		Site: site,
		Opts: opts,
	})
	if err != nil {
		return err
	}

	return dispatchManualAlert(&fireAlertDispatchParams{
		Ctx:     ctx,
		Deps:    deps,
		Site:    site,
		Alert:   alert,
		Skip:    opts.SkipDispatch,
		BaseURL: cmdCtx.Config.HTTP.BaseURL,
		Logger:  cmdCtx.Logger,
	})
}

func createAndPrintAlert(params *createAndPrintAlertParams) (*model.Alert, json.RawMessage, json.RawMessage, error) {
	eventContext, metadata, err := buildFireAlertPayloads(params.Site, &params.Opts)
	if err != nil {
		return nil, nil, nil, err
	}

	alertParams := &createManualAlertParams{
		Ctx:          params.Ctx,
		Repo:         params.Deps.AlertRepo,
		Site:         params.Site,
		Opts:         params.Opts,
		EventContext: eventContext,
		Metadata:     metadata,
	}
	alert, err := createManualAlert(alertParams)
	if err != nil {
		return nil, nil, nil, err
	}

	if summaryErr := printManualAlertSummary(alert, params.Site, manualAlertSummary{
		EventContext: eventContext,
		Metadata:     metadata,
	}); summaryErr != nil {
		return nil, nil, nil, summaryErr
	}

	return alert, eventContext, metadata, nil
}

func openFireAlertDeps(cmdCtx *commandContext) (fireAlertDeps, error) {
	db, _, err := connectInfraWithOptions(&connectInfraOptions{
		Logger:    cmdCtx.Logger,
		Config:    &cmdCtx.Config,
		WantDB:    true,
		WantRedis: false,
	})
	if err != nil {
		return fireAlertDeps{}, err
	}

	encryptor := bootstrap.CreateEncryptor(cmdCtx.Config.SecretsEncryptionKey, cmdCtx.Logger)
	return fireAlertDeps{
		db:         db,
		SiteRepo:   data.NewSiteRepo(db),
		AlertRepo:  data.NewAlertRepo(db),
		SinkRepo:   data.NewHTTPAlertSinkRepo(db),
		JobRepo:    data.NewJobRepo(db, data.RepoConfig{}),
		SecretRepo: data.NewSecretRepo(db, encryptor),
	}, nil
}

func (d fireAlertDeps) Close() error {
	if d.db == nil {
		return nil
	}
	return d.db.Close()
}

func ensureSiteSink(site *model.Site) error {
	if site.HTTPAlertSinkID == nil || strings.TrimSpace(*site.HTTPAlertSinkID) == "" {
		return fmt.Errorf("site %q has no HTTP alert sink configured", site.Name)
	}
	return nil
}

func buildFireAlertPayloads(site *model.Site, opts *fireAlertOptions) (json.RawMessage, json.RawMessage, error) {
	eventContext, err := resolveJSONPayload(opts.EventContext, buildDefaultEventContext(site))
	if err != nil {
		return nil, nil, fmt.Errorf("event context: %w", err)
	}

	metadata, err := resolveJSONPayload(opts.Metadata, buildDefaultMetadata(site))
	if err != nil {
		return nil, nil, fmt.Errorf("metadata: %w", err)
	}

	return eventContext, metadata, nil
}

func createManualAlert(params *createManualAlertParams) (*model.Alert, error) {
	firedAt := time.Now().UTC()
	alert, err := params.Repo.Create(params.Ctx, &model.CreateAlertRequest{
		SiteID:       params.Site.ID,
		RuleType:     params.Opts.RuleType,
		Severity:     params.Opts.Severity,
		Title:        params.Opts.Title,
		Description:  params.Opts.Description,
		EventContext: params.EventContext,
		Metadata:     params.Metadata,
		FiredAt:      &firedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("create alert: %w", err)
	}
	return alert, nil
}

type manualAlertSummary struct {
	EventContext json.RawMessage
	Metadata     json.RawMessage
}

func printManualAlertSummary(alert *model.Alert, site *model.Site, summary manualAlertSummary) error {
	if err := writef(
		os.Stdout,
		"Created alert %s for site %s (%s)\n",
		alert.ID,
		site.Name,
		site.ID,
	); err != nil {
		return fmt.Errorf("print alert summary: %w", err)
	}
	if err := writef(os.Stdout, "  Severity: %s | Rule: %s\n", alert.Severity, alert.RuleType); err != nil {
		return fmt.Errorf("print alert summary: %w", err)
	}
	if err := writef(os.Stdout, "  Title: %s\n", alert.Title); err != nil {
		return fmt.Errorf("print alert summary: %w", err)
	}
	if err := writef(os.Stdout, "Event context:\n%s\n", indentJSON(summary.EventContext)); err != nil {
		return fmt.Errorf("print alert summary: %w", err)
	}
	if err := writef(os.Stdout, "Metadata:\n%s\n", indentJSON(summary.Metadata)); err != nil {
		return fmt.Errorf("print alert summary: %w", err)
	}
	return nil
}

func dispatchManualAlert(params *fireAlertDispatchParams) error {
	if params.Skip {
		if err := writeln(os.Stdout, "Dispatch skipped (--no-dispatch). Alert record created only."); err != nil {
			return fmt.Errorf("print dispatch status: %w", err)
		}
		return nil
	}

	alertSinkSvc := service.NewAlertSinkService(service.AlertSinkServiceOptions{
		JobRepo:    params.Deps.JobRepo,
		SecretRepo: params.Deps.SecretRepo,
	})
	recorder := &recordingAlertSinkScheduler{inner: alertSinkSvc}
	dispatcher := service.NewAlertDispatchService(service.AlertDispatchServiceOptions{
		Sites:     params.Deps.SiteRepo,
		Sinks:     params.Deps.SinkRepo,
		AlertSink: recorder,
		BaseURL:   params.BaseURL,
		Logger:    params.Logger,
	})

	if err := dispatcher.Dispatch(params.Ctx, params.Alert); err != nil {
		return fmt.Errorf("dispatch alert: %w", err)
	}

	if len(recorder.jobs) == 0 {
		if err := writeln(os.Stdout, "No HTTP alert sinks were scheduled. Check the site's sink configuration."); err != nil {
			return fmt.Errorf("print dispatch summary: %w", err)
		}
		return nil
	}

	if err := writef(os.Stdout, "Scheduled %d HTTP alert job(s):\n", len(recorder.jobs)); err != nil {
		return fmt.Errorf("print dispatch summary: %w", err)
	}
	for i := range recorder.jobs {
		record := &recorder.jobs[i]
		if err := writef(
			os.Stdout,
			"  - sink %s (%s), job %s\n",
			record.sink.Name,
			record.sink.ID,
			record.job.ID,
		); err != nil {
			return fmt.Errorf("print dispatch summary: %w", err)
		}
	}

	return nil
}

func parseFireAlertFlags(args []string) (fireAlertOptions, error) {
	fs := flag.NewFlagSet("fire-http-alert", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	opts := fireAlertOptions{
		Severity:    defaultAlertSeverity,
		RuleType:    defaultAlertRuleType,
		Title:       defaultAlertTitle,
		Description: defaultAlertDescription,
		Timeout:     defaultFireAlertTimeout,
	}

	fs.StringVar(&opts.SiteID, "site-id", "", "Target site ID (mutually exclusive with --site-name)")
	fs.StringVar(&opts.SiteName, "site-name", "", "Target site name (mutually exclusive with --site-id)")
	fs.StringVar(&opts.Severity, "severity", opts.Severity, "Alert severity (critical|high|medium|low|info)")
	fs.StringVar(&opts.RuleType, "rule-type", opts.RuleType, "Rule type value stored on the alert record")
	fs.StringVar(&opts.Title, "title", opts.Title, "Alert title")
	fs.StringVar(&opts.Description, "description", opts.Description, "Alert description")
	fs.StringVar(
		&opts.EventContext,
		"event-context",
		"",
		"Optional JSON payload for alert.event_context (defaults to a manual test payload)",
	)
	fs.StringVar(
		&opts.Metadata,
		"metadata",
		"",
		"Optional JSON payload for alert.metadata (defaults to a manual test payload)",
	)
	fs.BoolVar(
		&opts.SkipDispatch,
		"no-dispatch",
		false,
		"Create the alert record but skip scheduling HTTP sink delivery",
	)
	fs.DurationVar(&opts.Timeout, "timeout", opts.Timeout, "Timeout for database operations and dispatch")

	if err := fs.Parse(args); err != nil {
		return fireAlertOptions{}, err
	}

	normalizeFireAlertOptions(&opts)
	if err := validateFireAlertOptions(&opts); err != nil {
		return fireAlertOptions{}, err
	}

	return opts, nil
}

func normalizeFireAlertOptions(opts *fireAlertOptions) {
	opts.SiteID = strings.TrimSpace(opts.SiteID)
	opts.SiteName = strings.TrimSpace(opts.SiteName)
	opts.Severity = strings.ToLower(strings.TrimSpace(opts.Severity))
	opts.RuleType = strings.ToLower(strings.TrimSpace(opts.RuleType))
	opts.Title = strings.TrimSpace(opts.Title)
	opts.Description = strings.TrimSpace(opts.Description)
	opts.EventContext = strings.TrimSpace(opts.EventContext)
	opts.Metadata = strings.TrimSpace(opts.Metadata)
}

func validateFireAlertOptions(opts *fireAlertOptions) error {
	if (opts.SiteID == "" && opts.SiteName == "") || (opts.SiteID != "" && opts.SiteName != "") {
		return errors.New("specify exactly one of --site-id or --site-name")
	}
	if !model.AlertSeverity(opts.Severity).Valid() {
		return fmt.Errorf("invalid severity %q", opts.Severity)
	}
	if !model.AlertRuleType(opts.RuleType).Valid() {
		return fmt.Errorf("invalid rule type %q", opts.RuleType)
	}
	if opts.Title == "" {
		return errors.New("title is required")
	}
	if opts.Description == "" {
		return errors.New("description is required")
	}
	if opts.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	return nil
}

func lookupSite(
	ctx context.Context,
	repo *data.SiteRepo,
	opts *fireAlertOptions,
) (*model.Site, error) {
	if opts.SiteID != "" {
		return getSiteByID(ctx, repo, opts.SiteID)
	}
	return getSiteByName(ctx, repo, opts.SiteName)
}

func resolveJSONPayload(input string, fallback json.RawMessage) (json.RawMessage, error) {
	if input == "" {
		return fallback, nil
	}
	data := []byte(input)
	if !json.Valid(data) {
		return nil, errors.New("must be valid JSON")
	}
	// Make a copy to avoid retaining the backing array of flag args.
	out := make([]byte, len(data))
	copy(out, data)
	return json.RawMessage(out), nil
}

func getSiteByID(ctx context.Context, repo *data.SiteRepo, siteID string) (*model.Site, error) {
	site, err := repo.GetByID(ctx, siteID)
	if err == nil {
		return site, nil
	}

	if errors.Is(err, data.ErrSiteNotFound) {
		return nil, fmt.Errorf("site id %q not found", siteID)
	}

	return nil, fmt.Errorf("get site by id: %w", err)
}

func getSiteByName(ctx context.Context, repo *data.SiteRepo, siteName string) (*model.Site, error) {
	site, err := repo.GetByName(ctx, siteName)
	if err == nil {
		return site, nil
	}

	if errors.Is(err, data.ErrSiteNotFound) {
		return nil, fmt.Errorf("site %q not found", siteName)
	}

	return nil, fmt.Errorf("get site by name: %w", err)
}

func indentJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

func buildDefaultEventContext(site *model.Site) json.RawMessage {
	payload := map[string]any{
		"manual_test": true,
		"origin":      "merrymaker-admin fire-http-alert",
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"site": map[string]string{
			"id":   site.ID,
			"name": site.Name,
		},
	}
	if scope := trimmedPtr(site.Scope); scope != "" {
		payload["scope"] = scope
	}
	if user := currentUsername(); user != "" {
		payload["triggered_by"] = user
	}
	if host := localHostname(); host != "" {
		payload["host"] = host
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage(`{"manual_test":true}`)
	}
	return json.RawMessage(b)
}

func buildDefaultMetadata(site *model.Site) json.RawMessage {
	meta := map[string]any{
		"category":  "manual-test",
		"component": "merrymaker-admin",
		"site_name": site.Name,
		"tags":      []string{"observability", "http-sink", "manual"},
	}
	if scope := trimmedPtr(site.Scope); scope != "" {
		meta["scope"] = scope
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return json.RawMessage(`{"category":"manual-test"}`)
	}
	return json.RawMessage(b)
}

func trimmedPtr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func currentUsername() string {
	for _, key := range []string{"USER", "USERNAME"} {
		if val := strings.TrimSpace(os.Getenv(key)); val != "" {
			return val
		}
	}
	return ""
}

func localHostname() string {
	host, err := os.Hostname()
	if err != nil {
		return ""
	}
	return host
}

func writef(w io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(w, format, args...)
	return err
}

func write(w io.Writer, args ...any) error {
	_, err := fmt.Fprint(w, args...)
	return err
}

func writeln(w io.Writer, args ...any) error {
	if len(args) == 0 {
		_, err := fmt.Fprintln(w)
		return err
	}
	_, err := fmt.Fprintln(w, args...)
	return err
}
