package rules

import (
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// NetworkEventExtractor provides utilities for extracting information from network events.
type NetworkEventExtractor struct{}

// NewNetworkEventExtractor creates a new network event extractor.
func NewNetworkEventExtractor() *NetworkEventExtractor {
	return &NetworkEventExtractor{}
}

// ExtractDomainFromNetworkEvent extracts the domain (host) from CDP Network.* events.
// It supports common payload shapes we emit:
// - {"request":{"url":"https://example.com/path"}}
// - {"url":"https://example.com/path"}
// - {"response":{"url":"https://example.com/path"}}
// Returns the lower-cased domain (without port) and true when successful.
func (e *NetworkEventExtractor) ExtractDomainFromNetworkEvent(event model.RawEvent) (string, bool) {
	if !strings.HasPrefix(event.Type, "Network.") {
		return "", false
	}
	// Parse a few likely shapes without overfitting. Keep structs local and small.
	type reqShape struct {
		Request struct {
			URL string `json:"url"`
		} `json:"request"`
		URL      string `json:"url"`
		Response struct {
			URL string `json:"url"`
		} `json:"response"`
	}
	var p reqShape
	if len(event.Data) == 0 {
		return "", false
	}
	if err := json.Unmarshal(event.Data, &p); err != nil {
		return "", false
	}
	// Candidate URL preference: request.url -> url -> response.url
	u := strings.TrimSpace(p.Request.URL)
	if u == "" {
		u = strings.TrimSpace(p.URL)
	}
	if u == "" {
		u = strings.TrimSpace(p.Response.URL)
	}
	if u == "" {
		return "", false
	}
	parsed, err := url.Parse(u)
	//nolint:nestif // layered parsing handles scheme-less URLs without splitting logic across helpers
	if err != nil || parsed.Host == "" {
		// Retry with default scheme for scheme-less URLs like example.com/path
		if !strings.Contains(u, "://") {
			prefixed := u
			if strings.HasPrefix(prefixed, "//") {
				prefixed = "http:" + prefixed
			} else {
				prefixed = "http://" + prefixed
			}
			if p2, err2 := url.Parse(prefixed); err2 == nil && p2.Host != "" {
				parsed = p2
			} else {
				return "", false
			}
		} else {
			return "", false
		}
	}
	host := strings.ToLower(parsed.Host)
	// Strip port if present
	if i := strings.LastIndexByte(host, ':'); i > -1 {
		host = host[:i]
	}
	// Strip IPv6 brackets
	host = strings.Trim(host, "[]")
	if host == "" {
		return "", false
	}
	return host, true
}

// ExtractFileHashFromFileEvent extracts a SHA256 (64-hex) from file events like
// "file_seen" or "file_downloaded". Returns lower-cased hash and true when valid.
func (e *NetworkEventExtractor) ExtractFileHashFromFileEvent(event model.RawEvent) (string, bool) {
	if event.Type != "file_seen" && event.Type != "file_downloaded" {
		return "", false
	}
	if len(event.Data) == 0 {
		return "", false
	}
	// Support common shapes: {"sha256":"..."} or {"hash":"..."}
	type fileShape struct {
		SHA256 string `json:"sha256"`
		Hash   string `json:"hash"`
	}
	var p fileShape
	if err := json.Unmarshal(event.Data, &p); err != nil {
		return "", false
	}
	h := strings.TrimSpace(p.SHA256)
	if h == "" {
		h = strings.TrimSpace(p.Hash)
	}
	// Validate hex via encoding/hex; require length 64
	if len(h) != 64 {
		return "", false
	}
	if _, err := hex.DecodeString(h); err != nil {
		return "", false
	}
	return strings.ToLower(h), true
}
