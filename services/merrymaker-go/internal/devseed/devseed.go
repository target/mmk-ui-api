package devseed

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/data/cryptoutil"
	"github.com/target/mmk-ui-api/internal/domain"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
)

// Services bundles the dependencies needed for development seeding.
type Services struct {
	DB         *sql.DB
	secrets    *service.SecretService
	sources    *service.SourceService
	alertSinks *service.HTTPAlertSinkService
	sites      *service.SiteService
	admin      *data.ScheduledJobsAdminRepo
}

// NewServices constructs all required services for seeding using the provided DB.
func NewServices(db *sql.DB) Services {
	encryptor := &cryptoutil.NoopEncryptor{} // Use noop for dev
	secretRepo := data.NewSecretRepo(db, encryptor)
	secretService := service.MustNewSecretService(service.SecretServiceOptions{
		Repo: secretRepo,
	})

	sourceRepo := data.NewSourceRepo(db)
	jobRepo := data.NewJobRepo(db, data.RepoConfig{})
	sourceService := service.NewSourceService(service.SourceServiceOptions{
		SourceRepo: sourceRepo,
		Jobs:       jobRepo,
		SecretRepo: secretRepo,
	})

	alertSinkRepo := data.NewHTTPAlertSinkRepo(db)
	alertSinkService := service.MustNewHTTPAlertSinkService(service.HTTPAlertSinkServiceOptions{
		Repo: alertSinkRepo,
	})

	siteRepo := data.NewSiteRepo(db)
	scheduledAdmin := data.NewScheduledJobsAdminRepo(db)
	siteService := service.NewSiteService(service.SiteServiceOptions{
		SiteRepo: siteRepo,
		Admin:    scheduledAdmin,
	})

	return Services{
		DB:         db,
		secrets:    secretService,
		sources:    sourceService,
		alertSinks: alertSinkService,
		sites:      siteService,
		admin:      scheduledAdmin,
	}
}

// Run executes the full development seeding workflow against the provided DB.
func Run(ctx context.Context, svcs Services, logger *slog.Logger) error {
	d := seedDeps{Sites: svcs.sites, Sources: svcs.sources, Alerts: svcs.alertSinks, Logger: logger}
	failures := 0
	failures += seedSecrets(ctx, svcs.secrets, logger)
	failures += seedSources(ctx, svcs.sources, logger)
	failures += seedHTTPAlertSinks(ctx, svcs.alertSinks, logger)
	if err := seedSites(ctx, d); err != nil {
		return err
	}
	if err := cleanupOrphanSiteSchedules(ctx, svcs.DB, logger); err != nil {
		if logger != nil {
			logger.WarnContext(ctx, "failed to cleanup orphan site schedules", "error", err)
		}
	}
	if err := reconcileSiteSchedules(ctx, svcs, logger); err != nil {
		if logger != nil {
			logger.WarnContext(ctx, "failed to reconcile site schedules", "error", err)
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d seed errors; check logs", failures)
	}
	return nil
}

type seedDeps struct {
	Sites   *service.SiteService
	Sources *service.SourceService
	Alerts  *service.HTTPAlertSinkService
	Logger  *slog.Logger
}

func seedSecrets(ctx context.Context, svc *service.SecretService, logger *slog.Logger) int {
	failures := 0
	secrets := []model.CreateSecretRequest{
		{Name: "api-key", Value: "sk-dev-12345"},
		{Name: "webhook-token", Value: "wh_dev_abcdef"},
		{Name: "basic-auth", Value: "dev:password123"},
		{Name: "bearer-token", Value: "bearer_dev_xyz789"},
	}

	for _, req := range secrets {
		created, err := createSecret(ctx, svc, req)
		if err != nil {
			if logger != nil {
				logger.ErrorContext(ctx, "failed to create secret", "name", req.Name, "error", err)
			}
			failures++
			continue
		}
		if logger != nil {
			msg := "secret already exists"
			if created {
				msg = "created secret"
			}
			logger.InfoContext(ctx, msg, "name", req.Name)
		}
	}

	return failures
}

func createSecret(ctx context.Context, svc *service.SecretService, req model.CreateSecretRequest) (bool, error) {
	if _, err := svc.Create(ctx, req); err != nil {
		if errors.Is(err, data.ErrSecretNameExists) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func seedSources(ctx context.Context, svc *service.SourceService, logger *slog.Logger) int {
	failures := 0
	for _, req := range defaultSources() {
		params := sourceOperationParams{
			ctx:     ctx,
			svc:     svc,
			logger:  logger,
			request: req,
		}
		if err := createOrUpdateSource(params); err != nil {
			failures++
		}
	}
	return failures
}

func defaultSources() []*model.CreateSourceRequest {
	return []*model.CreateSourceRequest{
		{
			Name: "example-login-flow",
			Value: `// Example login flow (Puppeteer)
      await page.goto('https://example.com/login');
      await page.type('#username', 'testuser');
      await page.type('#password', 'testpass');
      await page.click('#login-btn');
      await page.waitForNavigation({waitUntil: 'domcontentloaded'});`,
			Test:    true,
			Secrets: []string{"api-key"},
		},
		{
			Name: "e-commerce-checkout",
			Value: `// E-commerce checkout flow (Puppeteer)
      await page.goto('https://shop.example.com');
      await page.click('.product-item:first-child');
      await page.click('#add-to-cart');
      await page.goto('https://shop.example.com/checkout');
      await page.type('#email', 'test@example.com');`,
			Test:    false,
			Secrets: []string{"webhook-token", "basic-auth"},
		},
		{
			Name: "api-health-check",
			Value: `// API health monitoring (Puppeteer)
      await page.goto('https://example.com');
      const response = await page.evaluate(async () => {
        const r = await fetch('/api/health');
        try { return await r.json(); } catch (_) { return { status: r.status }; }
      });
      console.log('Health status:', response.status || response);`,
			Test:    true,
			Secrets: []string{"bearer-token"},
		},
	}
}

type sourceOperationParams struct {
	ctx     context.Context
	svc     *service.SourceService
	logger  *slog.Logger
	request *model.CreateSourceRequest
}

func createOrUpdateSource(params sourceOperationParams) error {
	_, err := params.svc.Create(params.ctx, params.request)
	if err == nil {
		params.logSourceCreated()
		return nil
	}

	if !errors.Is(err, data.ErrSourceNameExists) {
		params.logSourceCreateError(err)
		return err
	}

	return updateExistingSource(params)
}

func updateExistingSource(params sourceOperationParams) error {
	if params.logger != nil {
		params.logger.InfoContext(
			params.ctx,
			"source already exists",
			"name",
			params.request.Name,
			"action",
			"updating",
		)
	}

	source, err := params.svc.GetByName(params.ctx, params.request.Name)
	if err != nil {
		if params.logger != nil {
			params.logger.ErrorContext(
				params.ctx,
				"failed to load source for update",
				"name",
				params.request.Name,
				"error",
				err,
			)
		}
		return err
	}

	val := params.request.Value
	test := params.request.Test
	upd := model.UpdateSourceRequest{Value: &val, Test: &test}
	if params.request.Secrets != nil {
		upd.Secrets = params.request.Secrets
	}
	if _, updateErr := params.svc.Update(params.ctx, source.ID, upd); updateErr != nil {
		if params.logger != nil {
			params.logger.ErrorContext(
				params.ctx,
				"failed to update source",
				"name",
				params.request.Name,
				"error",
				updateErr,
			)
		}
		return updateErr
	}
	if params.logger != nil {
		params.logger.InfoContext(params.ctx, "updated source", "name", params.request.Name)
	}
	return nil
}

func (p sourceOperationParams) logSourceCreated() {
	if p.logger == nil {
		return
	}

	p.logger.InfoContext(p.ctx, "created source", "name", p.request.Name)
}

func (p sourceOperationParams) logSourceCreateError(err error) {
	if p.logger == nil {
		return
	}

	p.logger.ErrorContext(p.ctx, "failed to create source", "name", p.request.Name, "error", err)
}

func seedHTTPAlertSinks(ctx context.Context, svc *service.HTTPAlertSinkService, logger *slog.Logger) int {
	failures := 0
	for _, req := range defaultHTTPAlertSinkSeeds() {
		created, err := createAlertSink(ctx, svc, req)
		if err != nil {
			if logger != nil {
				logger.ErrorContext(ctx, "failed to create alert sink", "name", req.Name, "error", err)
			}
			failures++
			continue
		}
		if logger != nil {
			msg := "alert sink already exists"
			if created {
				msg = "created alert sink"
			}
			logger.InfoContext(ctx, msg, "name", req.Name)
		}
	}

	return failures
}

func defaultHTTPAlertSinkSeeds() []*model.CreateHTTPAlertSinkRequest {
	return []*model.CreateHTTPAlertSinkRequest{
		{
			Name:   "slack-alerts",
			Method: "POST",
			URI:    "https://hooks.slack.com/services/dev/webhook",
			Body: stringPtr(`{
        "text": '🧪 Merrymaker manual alert test',
        "attachments": [{
          "color": '#439FE0',
          "fields": [
            {"title": 'Site ID', "value": site_id, "short": true},
            {"title": 'Severity', "value": severity, "short": true},
            {"title": 'Title', "value": title, "short": false},
            {"title": 'Description', "value": description, "short": false}
          ]
        }]
      }`),
			Secrets:  []string{"webhook-token"},
			Headers:  stringPtr(`{"Content-Type": "application/json"}`),
			OkStatus: intPtr(200),
			Retry:    intPtr(3),
		},
		{
			Name:     "discord-notifications",
			Method:   "POST",
			URI:      "https://discord.com/api/webhooks/dev/webhook",
			Secrets:  []string{"api-key"},
			Headers:  stringPtr(`{"Content-Type": "application/json", "User-Agent": "Merrymaker-Dev"}`),
			OkStatus: intPtr(204),
			Retry:    intPtr(2),
		},
		{
			Name:     "email-service",
			Method:   "POST",
			URI:      "https://api.sendgrid.com/v3/mail/send",
			Secrets:  []string{"bearer-token"},
			Headers:  stringPtr(`{"Content-Type": "application/json", "Authorization": "Bearer __BEARER_TOKEN__"}`),
			OkStatus: intPtr(202),
			Retry:    intPtr(3),
		},
	}
}

func createAlertSink(
	ctx context.Context,
	svc *service.HTTPAlertSinkService,
	req *model.CreateHTTPAlertSinkRequest,
) (bool, error) {
	if _, err := svc.Create(ctx, req); err != nil {
		if errors.Is(err, data.ErrHTTPAlertSinkNameExists) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

var errNoSources = errors.New("no sources available for site creation")

func seedSites(ctx context.Context, d seedDeps) error {
	sources, sinks, err := getSourcesAndSinks(ctx, d)
	if err != nil {
		return err
	}

	sites, err := createSiteRequests(sources, sinks)
	if err != nil {
		return fmt.Errorf("prepare site requests: %w", err)
	}
	params := siteCreationParams{
		ctx:    ctx,
		svc:    d.Sites,
		logger: d.Logger,
	}
	failures := createSites(params, sites)
	if failures > 0 && d.Logger != nil {
		d.Logger.WarnContext(ctx, "some sites failed to create", "failures", failures)
	}
	return nil
}

func getSourcesAndSinks(ctx context.Context, d seedDeps) ([]*model.Source, []*model.HTTPAlertSink, error) {
	sources, err := fetchAllSources(ctx, d.Sources)
	if err != nil {
		return nil, nil, fmt.Errorf("list sources: %w", err)
	}
	if len(sources) == 0 {
		return nil, nil, errNoSources
	}

	sinks, err := fetchAllHTTPAlertSinks(ctx, d.Alerts)
	if err != nil {
		if d.Logger != nil {
			d.Logger.WarnContext(ctx, "failed to list alert sinks", "error", err)
		}
		sinks = nil
	}

	return sources, sinks, nil
}

type siteSeedSpec struct {
	siteName   string
	enabled    bool
	scope      string
	minutes    int
	sourceName string
	sinkName   string
}

func createSiteRequests(sources []*model.Source, sinks []*model.HTTPAlertSink) ([]*model.CreateSiteRequest, error) {
	sourceByName := indexSourcesByName(sources)
	sinkByName := indexSinksByName(sinks)
	specs := defaultSiteSeedSpecs()

	requests := make([]*model.CreateSiteRequest, 0, len(specs))
	for _, spec := range specs {
		sourceID, err := getSourceByName(sourceByName, spec.sourceName)
		if err != nil {
			return nil, fmt.Errorf("site %q: %w", spec.siteName, err)
		}
		requests = append(requests, &model.CreateSiteRequest{
			Name:            spec.siteName,
			Enabled:         boolPtr(spec.enabled),
			Scope:           stringPtr(spec.scope),
			RunEveryMinutes: spec.minutes,
			SourceID:        sourceID,
			HTTPAlertSinkID: getSinkByName(sinkByName, spec.sinkName),
		})
	}

	return requests, nil
}

func defaultSiteSeedSpecs() []siteSeedSpec {
	return []siteSeedSpec{
		{
			siteName:   "observability-manual-test",
			enabled:    false,
			scope:      "manual.test",
			minutes:    60,
			sourceName: "api-health-check",
			sinkName:   "slack-alerts",
		},
		{
			siteName:   "production-login-monitor",
			enabled:    true,
			scope:      "*.example.com",
			minutes:    15,
			sourceName: "example-login-flow",
			sinkName:   "slack-alerts",
		},
		{
			siteName:   "staging-checkout-test",
			enabled:    false,
			scope:      "staging.shop.example.com",
			minutes:    30,
			sourceName: "e-commerce-checkout",
			sinkName:   "discord-notifications",
		},
		{
			siteName:   "api-health-monitor",
			enabled:    true,
			scope:      "api.example.com",
			minutes:    5,
			sourceName: "api-health-check",
			sinkName:   "email-service",
		},
		{
			siteName:   "fast-smoke-monitor",
			enabled:    true,
			scope:      "fast.example.com",
			minutes:    1,
			sourceName: "api-health-check",
			sinkName:   "slack-alerts",
		},
	}
}

func indexSourcesByName(sources []*model.Source) map[string]string {
	out := make(map[string]string, len(sources))
	for _, s := range sources {
		out[s.Name] = s.ID
	}
	return out
}

func indexSinksByName(sinks []*model.HTTPAlertSink) map[string]string {
	out := make(map[string]string, len(sinks))
	for _, s := range sinks {
		out[s.Name] = s.ID
	}
	return out
}

func getSourceByName(sourceByName map[string]string, name string) (string, error) {
	if id, ok := sourceByName[name]; ok {
		return id, nil
	}
	return "", fmt.Errorf("source name %q not found in available sources", name)
}

func fetchAllSources(ctx context.Context, svc *service.SourceService) ([]*model.Source, error) {
	const pageSize = 100
	offset := 0
	var out []*model.Source
	for {
		page, err := svc.List(ctx, pageSize, offset)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		out = append(out, page...)
		offset += len(page)
		if len(page) < pageSize {
			break
		}
	}
	return out, nil
}

func fetchAllHTTPAlertSinks(ctx context.Context, svc *service.HTTPAlertSinkService) ([]*model.HTTPAlertSink, error) {
	const pageSize = 100
	offset := 0
	var out []*model.HTTPAlertSink
	for {
		page, err := svc.List(ctx, pageSize, offset)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		out = append(out, page...)
		offset += len(page)
		if len(page) < pageSize {
			break
		}
	}
	return out, nil
}

func getSinkByName(sinkByName map[string]string, name string) *string {
	if id, ok := sinkByName[name]; ok {
		return &id
	}
	return nil
}

type siteCreationParams struct {
	ctx    context.Context
	svc    *service.SiteService
	logger *slog.Logger
}

func createSites(params siteCreationParams, sites []*model.CreateSiteRequest) int {
	failures := 0
	for _, req := range sites {
		created, err := createSite(params.ctx, params.svc, req)
		if err != nil {
			if params.logger != nil {
				params.logger.ErrorContext(params.ctx, "failed to create site", "name", req.Name, "error", err)
			}
			failures++
			continue
		}
		if params.logger != nil {
			msg := "site already exists"
			if created {
				msg = "created site"
			}
			params.logger.InfoContext(params.ctx, msg, "name", req.Name)
		}
	}
	return failures
}

func createSite(
	ctx context.Context,
	svc *service.SiteService,
	req *model.CreateSiteRequest,
) (bool, error) {
	if _, err := svc.Create(ctx, req); err != nil {
		if errors.Is(err, data.ErrSiteNameExists) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func cleanupOrphanSiteSchedules(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	const q = `
		DELETE FROM scheduled_jobs s
		WHERE s.task_name LIKE 'site:%'
		  AND NOT EXISTS (
			SELECT 1 FROM sites WHERE id = split_part(s.task_name, ':', 2)::uuid
		  )
	`
	res, err := db.ExecContext(ctx, q)
	if err != nil {
		return fmt.Errorf("delete orphan site schedules: %w", err)
	}
	if logger != nil {
		if n, rowsErr := res.RowsAffected(); rowsErr != nil {
			logger.WarnContext(ctx, "deleted orphan site schedules: rows affected unknown", "error", rowsErr)
		} else if n > 0 {
			logger.InfoContext(ctx, "deleted orphan site schedules", "count", n)
		}
	}
	return nil
}

func reconcileSiteSchedules(ctx context.Context, svcs Services, logger *slog.Logger) error {
	const limit = 100
	offset := 0
	for {
		list, err := svcs.sites.List(ctx, limit, offset)
		if err != nil {
			return fmt.Errorf("list sites: %w", err)
		}
		if len(list) == 0 {
			break
		}
		for _, site := range list {
			params := scheduleOperationParams{
				ctx:    ctx,
				admin:  svcs.admin,
				site:   site,
				logger: logger,
			}
			if scheduleErr := upsertOrDeleteSchedule(params); scheduleErr != nil {
				if logger != nil {
					logger.WarnContext(ctx, "reconcile schedule failed", "site", site.ID, "error", scheduleErr)
				}
			}
		}
		offset += len(list)
		if len(list) < limit {
			break
		}
	}
	return nil
}

type scheduleOperationParams struct {
	ctx    context.Context
	admin  *data.ScheduledJobsAdminRepo
	site   *model.Site
	logger *slog.Logger
}

func upsertOrDeleteSchedule(params scheduleOperationParams) error {
	name := "site:" + params.site.ID
	if !params.site.Enabled {
		_, err := params.admin.DeleteByTaskName(params.ctx, name)
		return err
	}

	interval := siteInterval(params.site)
	payload, err := schedulePayload(params.site)
	if err != nil {
		params.logSchedulePayloadError(err)
		return nil
	}
	return params.admin.UpsertByTaskName(
		params.ctx,
		domain.UpsertTaskParams{TaskName: name, Payload: payload, Interval: interval},
	)
}

func (p scheduleOperationParams) logSchedulePayloadError(err error) {
	if p.logger == nil {
		return
	}

	p.logger.WarnContext(p.ctx, "marshal schedule payload failed", "site", p.site.ID, "error", err)
}

func siteInterval(site *model.Site) time.Duration {
	d := time.Duration(site.RunEveryMinutes) * time.Minute
	if d <= 0 {
		return time.Minute
	}
	return d
}

func schedulePayload(site *model.Site) ([]byte, error) {
	payload := struct {
		SiteID   string `json:"site_id"`
		SourceID string `json:"source_id,omitempty"`
	}{SiteID: site.ID, SourceID: site.SourceID}
	return json.Marshal(payload)
}

func boolPtr(b bool) *bool       { return &b }
func stringPtr(s string) *string { return &s }
func intPtr(i int) *int          { return &i }
