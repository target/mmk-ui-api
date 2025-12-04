package rules

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// ScopeKey identifies a site/scope tuple used to enforce per-scope semantics.
// Keep parameter counts low by grouping into this struct.
type ScopeKey struct {
	SiteID string
	Scope  string
}

func (k ScopeKey) Validate() error {
	if strings.TrimSpace(k.SiteID) == "" {
		return errors.New("site_id is required")
	}
	if strings.TrimSpace(k.Scope) == "" {
		return errors.New("scope is required")
	}
	return nil
}

// SeenKey identifies a seen-domain lookup or update operation.
type SeenKey struct {
	Scope ScopeKey
	// Domain should be normalized to lower-case by callers.
	Domain string
}

func (k SeenKey) Validate() error {
	if err := k.Scope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(k.Domain) == "" {
		return errors.New("domain is required")
	}
	return nil
}

// FileKey identifies a processed-file lookup or update operation.
type FileKey struct {
	Scope ScopeKey
	// FileHash is expected to be a 64-char lower-case hex SHA256.
	FileHash string
}

func (k FileKey) Validate() error {
	if err := k.Scope.Validate(); err != nil {
		return err
	}
	if !isHexString64(k.FileHash) {
		return errors.New("file_hash must be 64 hex chars")
	}
	return nil
}

// isHexString64 reports whether s is a 64-character hexadecimal string (case-insensitive).
func isHexString64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, ch := range s {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') {
			continue
		}
		return false
	}
	return true
}

// CacheTTL holds the TTL configuration for caches; values should mirror docs and remain configurable.
type CacheTTL struct {
	SeenDomainsLocal    time.Duration
	SeenDomainsRedis    time.Duration
	IOCsLocal           time.Duration // Global IOCs (new iocs table)
	IOCsRedis           time.Duration
	ProcessedFilesLocal time.Duration
	ProcessedFilesRedis time.Duration
}

func DefaultCacheTTL() CacheTTL {
	return CacheTTL{
		SeenDomainsLocal:    5 * time.Minute,
		SeenDomainsRedis:    time.Hour,
		IOCsLocal:           15 * time.Minute,
		IOCsRedis:           4 * time.Hour,
		ProcessedFilesLocal: 10 * time.Minute,
		ProcessedFilesRedis: 24 * time.Hour,
	}
}

// SeenDomainsCache provides typed operations for seen domain tracking.
// Implementations should use a three-tier strategy: local LRU -> Redis -> Postgres.
// All methods should be idempotent where appropriate.
type SeenDomainsCache interface {
	// Exists checks if the domain has been seen for the given scope.
	Exists(ctx context.Context, key SeenKey) (bool, error)
	// Record marks the domain as seen (create or touch) for the given scope.
	Record(ctx context.Context, key SeenKey) error
}

// IOCCache provides typed operations for global IOC lookup (new iocs table).
// Supports both positive (IOC found) and negative (not found) caching.
type IOCCache interface {
	// LookupHost checks if a host (domain or IP) matches any enabled IOC.
	// Returns the matching IOC or nil if not found.
	// Uses three-tier strategy: local LRU -> Redis -> Postgres (with matcher).
	LookupHost(ctx context.Context, host string) (*model.IOC, error)
}

// ProcessedFilesCache provides typed operations for processed file tracking to enforce
// "skip already-processed" semantics per scope.
type ProcessedFilesCache interface {
	// IsProcessed returns true if the file hash has been processed for the given scope.
	IsProcessed(ctx context.Context, key FileKey) (bool, error)
	// MarkProcessed marks the file hash as processed for the given scope.
	// storageKey is required by the DB schema and should be a stable reference to the file content.
	MarkProcessed(ctx context.Context, key FileKey, storageKey string) error
}

// AlertOnceCache is used to enforce alert-once-per-scope semantics for various rules.
// The key should encode the scope and a stable dedupe key (e.g., domain, IOC id).
type AlertOnceCache interface {
	// Seen reports and records whether the dedupe key has already alerted for the scope.
	// If not seen, it records it and returns false. If already seen, returns true.
	// Uses AlertSeenRequest struct to keep parameter count ≤3.
	Seen(ctx context.Context, req AlertSeenRequest) (bool, error)
	// Peek checks whether the dedupe key has already alerted without mutating cache state.
	// Returns true if the key exists, false otherwise.
	Peek(ctx context.Context, req AlertSeenRequest) (bool, error)
}

// AlertSeenRequest groups parameters for AlertOnceCache.Seen to keep parameter count ≤3.
type AlertSeenRequest struct {
	Scope     ScopeKey
	DedupeKey string
	TTL       time.Duration
}
