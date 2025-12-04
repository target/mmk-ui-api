package bootstrap

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/target/mmk-ui-api/config"
	httpx "github.com/target/mmk-ui-api/internal/http"
	"github.com/target/mmk-ui-api/internal/service"
)

// HTTPServerConfig contains configuration for HTTP server.
type HTTPServerConfig struct {
	Config   *config.AppConfig
	Services ServiceContainer
	Logger   *slog.Logger
}

// StartHTTPServer creates and starts the HTTP server.
// Returns the server instance for graceful shutdown.
func StartHTTPServer(cfg *HTTPServerConfig) *http.Server {
	if cfg == nil {
		return nil
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Build router services
	appCfg := cfg.Config
	if appCfg == nil {
		appCfg = &config.AppConfig{}
	}

	services := httpx.RouterServices{
		Jobs:             cfg.Services.Jobs,
		Events:           cfg.Services.Events,
		Secrets:          cfg.Services.Secrets,
		HTTPAlertSinks:   cfg.Services.HTTPAlertSinks,
		Alerts:           cfg.Services.Alerts,
		Sources:          cfg.Services.Sources,
		Sites:            cfg.Services.Sites,
		Allowlist:        cfg.Services.DomainAllowlist,
		IOC:              cfg.Services.IOC,
		Auth:             cfg.Services.Auth,
		JobResults:       cfg.Services.JobResults,
		CookieDomain:     appCfg.HTTP.CookieDomain,
		Orchestrator:     cfg.Services.Orchestrator,
		Filter:           cfg.Services.Filter,
		AutoEnqueueRules: appCfg.RulesEngine.AutoEnqueue,
		IsDev:            appCfg.IsDev,
		Logger:           logger,
	}

	// Build handler with middleware
	handler := buildHTTPHandler(httpHandlerConfig{
		Logger:   logger,
		Services: services,
		HTTP:     appCfg.HTTP,
	})

	// Start server (logs "starting HTTP server" internally)
	server := startServer(logger, handler, appCfg.HTTP.Addr)

	return server
}

type httpHandlerConfig struct {
	Logger   *slog.Logger
	Services httpx.RouterServices
	HTTP     config.HTTPConfig
}

func buildHTTPHandler(cfg httpHandlerConfig) http.Handler {
	router := httpx.NewRouter(cfg.Services)

	// Apply compression middleware first (innermost) so logging captures compressed sizes
	// Order: Recover -> Logging -> Compression -> Router
	h := router
	if cfg.HTTP.CompressionEnabled {
		cfg.Logger.Info("HTTP compression enabled", "level", cfg.HTTP.CompressionLevel)
		h = httpx.Compression(httpx.CompressionConfig{Level: cfg.HTTP.CompressionLevel})(h)
	}

	h = httpx.Logging(cfg.Logger)(h)
	h = httpx.Recover(cfg.Logger)(h)

	return h
}

func startServer(logger *slog.Logger, handler http.Handler, addr string) *http.Server {
	// Guard against empty addr to avoid listening on Go default
	if addr == "" {
		addr = ":8080"
	}

	server := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Info("starting HTTP server", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server failed", "error", err)
		}
	}()

	return server
}

// ShutdownConfig contains dependencies for HTTP server shutdown.
type ShutdownConfig struct {
	Context    context.Context
	Server     *http.Server
	JobService *service.JobService
	Logger     *slog.Logger
}

// ShutdownHTTPServer gracefully shuts down the HTTP server.
func ShutdownHTTPServer(cfg ShutdownConfig) error {
	if cfg.Server == nil {
		return nil
	}

	if cfg.Logger != nil {
		cfg.Logger.Info("shutting down HTTP server")
	}

	// Stop job service listeners first
	if cfg.JobService != nil {
		cfg.JobService.StopAllListeners()
	}

	// Shutdown HTTP server with timeout
	shutdownCtx, cancel := context.WithTimeout(cfg.Context, 10*time.Second)
	defer cancel()

	if err := cfg.Server.Shutdown(shutdownCtx); err != nil {
		return err
	}

	if cfg.Logger != nil {
		cfg.Logger.Info("HTTP server stopped")
	}

	return nil
}
