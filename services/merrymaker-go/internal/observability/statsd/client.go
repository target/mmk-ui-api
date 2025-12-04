package statsd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sink describes the minimal interface required to emit StatsD-style metrics.
type Sink interface {
	Count(name string, value int64, tags map[string]string)
	Gauge(name string, value float64, tags map[string]string)
	Timing(name string, value time.Duration, tags map[string]string)
}

// Config describes how to connect to a StatsD-compatible sink.
type Config struct {
	Enabled    bool
	Address    string
	Prefix     string
	Logger     *slog.Logger
	GlobalTags map[string]string
}

// Client emits metrics over UDP using the StatsD line protocol.
// It is safe for concurrent use.
type Client struct {
	enabled    bool
	address    string
	prefix     string
	globalTags map[string]string

	logger *slog.Logger
	conn   net.Conn
	mu     sync.Mutex
}

var _ Sink = (*Client)(nil)

// NewClient dials the configured StatsD endpoint unless disabled.
func NewClient(cfg Config) (*Client, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	address := strings.TrimSpace(cfg.Address)
	enabled := cfg.Enabled && address != ""

	client := &Client{
		enabled:    enabled,
		address:    address,
		prefix:     sanitizePrefix(cfg.Prefix),
		globalTags: cloneTags(cfg.GlobalTags),
		logger:     logger,
	}

	if !enabled {
		return client, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", address)
	if err != nil {
		return nil, fmt.Errorf("statsd dial %s: %w", address, err)
	}
	client.conn = conn

	return client, nil
}

// Enabled reports whether the client actively emits metrics.
func (c *Client) Enabled() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enabled && c.conn != nil
}

// Count increments a counter metric.
func (c *Client) Count(name string, value int64, tags map[string]string) {
	if c == nil {
		return
	}
	c.write(name, strconv.FormatInt(value, 10)+"|c", tags)
}

// Gauge records the current value for a gauge metric.
func (c *Client) Gauge(name string, value float64, tags map[string]string) {
	if c == nil {
		return
	}
	c.write(name, formatFloat(value)+"|g", tags)
}

// Timing records a timing metric using milliseconds.
func (c *Client) Timing(name string, value time.Duration, tags map[string]string) {
	if c == nil {
		return
	}
	ms := float64(value) / float64(time.Millisecond)
	c.write(name, formatFloat(ms)+"|ms", tags)
}

// Close releases the underlying UDP connection if one was established.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		c.enabled = false
		return nil
	}

	err := c.conn.Close()
	c.conn = nil
	c.enabled = false
	return err
}

func (c *Client) write(name, payload string, tags map[string]string) {
	if c == nil {
		return
	}

	metric := c.metricName(name)
	if metric == "" {
		return
	}

	line := metric + ":" + payload + formatTags(c.globalTags, tags)

	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.enabled || c.conn == nil {
		return
	}

	if _, err := c.conn.Write([]byte(line)); err != nil {
		c.logger.Debug("statsd write failed", "error", err)
	}
}

func (c *Client) metricName(name string) string {
	if name == "" {
		return ""
	}
	normalized := normalizeMetricName(name)
	if c.prefix == "" {
		return normalized
	}
	if normalized == "" {
		return c.prefix
	}
	return c.prefix + "." + normalized
}

func sanitizePrefix(prefix string) string {
	p := strings.TrimSpace(prefix)
	p = strings.Trim(p, ".")
	return p
}

func normalizeMetricName(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return ""
	}
	// Replace spaces and slashes with underscores for compatibility.
	n = strings.ReplaceAll(n, " ", "_")
	n = strings.ReplaceAll(n, "/", "_")
	// Collapse repeated dots introduced by sanitisation.
	for strings.Contains(n, "..") {
		n = strings.ReplaceAll(n, "..", ".")
	}
	return strings.Trim(n, ".")
}

func formatTags(global, local map[string]string) string {
	total := len(global) + len(local)
	if total == 0 {
		return ""
	}

	// Pre-allocate merged map with total capacity to avoid rehashing when merging local tags.
	merged := make(map[string]string, total)
	for k, v := range global {
		key := strings.TrimSpace(k)
		if key != "" {
			merged[key] = strings.TrimSpace(v)
		}
	}
	for k, v := range local {
		if key := strings.TrimSpace(k); key != "" {
			merged[key] = strings.TrimSpace(v)
		}
	}

	if len(merged) == 0 {
		return ""
	}

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	values := make([]string, len(keys))
	for i, k := range keys {
		values[i] = k + ":" + merged[k]
	}
	return "|#" + strings.Join(values, ",")
}

func cloneTags(tags map[string]string) map[string]string {
	cp := make(map[string]string, len(tags))
	for k, v := range tags {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		cp[key] = strings.TrimSpace(v)
	}
	return cp
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
