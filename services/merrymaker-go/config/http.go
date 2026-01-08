package config

// HTTPConfig contains HTTP server configuration.
type HTTPConfig struct {
	// Addr is the address to bind the HTTP server to.
	Addr string `env:"HTTP_ADDR" envDefault:":8080"`

	// BaseURL is the base URL of the application (e.g., "https://app.example.com").
	// Used for generating absolute URLs in alert notifications and other external contexts.
	BaseURL string `env:"APP_BASE_URL" envDefault:"http://localhost:8080"`

	// CookieDomain is the domain for session cookies.
	// Leave empty to use the request domain.
	CookieDomain string `env:"APP_COOKIE_DOMAIN" envDefault:""`

	// CompressionEnabled enables gzip compression for text-based assets.
	CompressionEnabled bool `env:"HTTP_COMPRESSION_ENABLED" envDefault:"false"`

	// CompressionLevel is the gzip compression level (1-9).
	// Default is 6 (standard gzip default).
	CompressionLevel int `env:"HTTP_COMPRESSION_LEVEL" envDefault:"6"`
}

// Sanitize applies guardrails to HTTP configuration values.
func (h *HTTPConfig) Sanitize() {
	// Clamp compression level to valid gzip range (1-9)
	if h.CompressionLevel < 1 {
		h.CompressionLevel = 1
	}
	if h.CompressionLevel > 9 {
		h.CompressionLevel = 9
	}
}
