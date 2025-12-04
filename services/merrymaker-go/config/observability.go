package config

import (
	"strings"
	"time"
)

const defaultObservabilityName = "merrymaker"

// ObservabilityConfig groups configuration that controls metrics, logging, and alert fan-out.
type ObservabilityConfig struct {
	Metrics       ObservabilityMetricsConfig
	Notifications ObservabilityNotificationsConfig
}

// Sanitize applies guardrails to observability sub-configs.
func (c *ObservabilityConfig) Sanitize() {
	c.Metrics.Sanitize()
	c.Notifications.Sanitize()
}

// ObservabilityMetricsConfig controls emission of metrics to external sinks such as StatsD.
type ObservabilityMetricsConfig struct {
	Enabled       bool   `env:"OBSERVABILITY_METRICS_ENABLED"        envDefault:"false"`
	StatsdAddress string `env:"OBSERVABILITY_METRICS_STATSD_ADDRESS" envDefault:"127.0.0.1:8125"`
}

// Sanitize normalises derived fields and enforces safe defaults.
func (c *ObservabilityMetricsConfig) Sanitize() {
	c.StatsdAddress = strings.TrimSpace(c.StatsdAddress)
	if c.StatsdAddress == "" {
		c.Enabled = false
	}
}

// IsEnabled returns true when metrics emission is active after sanitisation.
func (c *ObservabilityMetricsConfig) IsEnabled() bool {
	return c.Enabled && c.StatsdAddress != ""
}

// ObservabilityNotificationsConfig controls outbound failure notifications (Option C).
type ObservabilityNotificationsConfig struct {
	Enabled    bool                        `env:"OBSERVABILITY_NOTIFICATIONS_ENABLED"     envDefault:"false"`
	Timeout    time.Duration               `env:"OBSERVABILITY_NOTIFICATIONS_TIMEOUT"     envDefault:"5s"`
	RetryLimit int                         `env:"OBSERVABILITY_NOTIFICATIONS_RETRY_LIMIT" envDefault:"3"`
	Slack      SlackNotificationConfig     `                                                                 envPrefix:"OBSERVABILITY_NOTIFICATIONS_SLACK_"`
	PagerDuty  PagerDutyNotificationConfig `                                                                 envPrefix:"OBSERVABILITY_NOTIFICATIONS_PAGERDUTY_"`
}

// Sanitize normalises notification configuration values.
func (c *ObservabilityNotificationsConfig) Sanitize() {
	if c.Timeout <= 0 {
		c.Timeout = 5 * time.Second
	}
	if c.RetryLimit < 0 {
		c.RetryLimit = 0
	}

	c.Slack.sanitize()
	c.PagerDuty.sanitize()

	if !c.Enabled {
		c.Slack.Enabled = false
		c.PagerDuty.Enabled = false
		return
	}

	if c.Slack.Enabled && c.Slack.WebhookURL == "" {
		c.Slack.Enabled = false
	}

	if c.PagerDuty.Enabled && c.PagerDuty.RoutingKey == "" {
		c.PagerDuty.Enabled = false
	}
}

// SlackNotificationConfig controls Slack webhook fan-out.
type SlackNotificationConfig struct {
	Enabled       bool   `env:"ENABLED"         envDefault:"false"`
	WebhookURL    string `env:"WEBHOOK_URL"`
	Channel       string `env:"CHANNEL"`
	Username      string `env:"USERNAME"        envDefault:"merrymaker"`
	SiteURLPrefix string `env:"SITE_URL_PREFIX"`
}

func (c *SlackNotificationConfig) sanitize() {
	c.WebhookURL = strings.TrimSpace(c.WebhookURL)
	c.Channel = strings.TrimSpace(c.Channel)
	c.SiteURLPrefix = strings.TrimSpace(c.SiteURLPrefix)
	if c.Username == "" {
		c.Username = defaultObservabilityName
	}
}

// PagerDutyNotificationConfig controls PagerDuty Events API v2 fan-out.
type PagerDutyNotificationConfig struct {
	Enabled    bool   `env:"ENABLED"     envDefault:"false"`
	RoutingKey string `env:"ROUTING_KEY"`
	Source     string `env:"SOURCE"      envDefault:"merrymaker"`
	Component  string `env:"COMPONENT"   envDefault:"merrymaker"`
}

func (c *PagerDutyNotificationConfig) sanitize() {
	c.RoutingKey = strings.TrimSpace(c.RoutingKey)
	if c.Source = strings.TrimSpace(c.Source); c.Source == "" {
		c.Source = defaultObservabilityName
	}
	if c.Component = strings.TrimSpace(c.Component); c.Component == "" {
		c.Component = defaultObservabilityName
	}
}
