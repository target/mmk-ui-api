package rules

import (
	"time"

	"github.com/target/mmk-ui-api/internal/core"
)

// LocalCaps defines per-cache local LRU capacities.
// Keep constructor params small by grouping.
type LocalCaps struct {
	SeenDomains    int
	IOCs           int // Global IOCs (new iocs table)
	ProcessedFiles int
}

// DefaultLocalCaps returns recommended local cache sizes aligned with docs.
func DefaultLocalCaps() LocalCaps {
	return LocalCaps{SeenDomains: 10_000, IOCs: 50_000, ProcessedFiles: 5_000}
}

// Caches groups all rule-related caches for DI into rule evaluators.
type Caches struct {
	Seen      SeenDomainsCache
	IOCs      IOCCache // Global IOCs (new iocs table)
	Files     ProcessedFilesCache
	AlertOnce AlertOnceCache
}

// CachesOptions provides dependencies to build typed caches.
// This keeps the composition in the service layer while depending on core interfaces.
type CachesOptions struct {
	TTL       CacheTTL
	LocalCaps LocalCaps

	// Tiers
	Redis core.CacheRepository

	// Postgres repositories
	SeenRepo  core.SeenDomainRepository
	IOCsRepo  core.IOCRepository // Global IOCs (new iocs table)
	FilesRepo core.ProcessedFileRepository

	// Metrics hook
	Metrics CacheMetrics

	// Optional shared versioner (useful for multi-component invalidation).
	IOCVersioner IOCVersioner
}

// DefaultCachesOptions returns sensible defaults for TTLs and local sizes.
func DefaultCachesOptions() CachesOptions {
	return CachesOptions{TTL: DefaultCacheTTL(), LocalCaps: DefaultLocalCaps()}
}

// BuildCaches constructs the rules caches using provided dependencies.
// Callers remain responsible for creating Redis clients and DB-backed repositories.
func BuildCaches(opts CachesOptions) Caches {
	// Local LRUs with TTL support; clock is time.Now
	seenLocal := NewLocalLRU(LocalLRUConfig{Capacity: opts.LocalCaps.SeenDomains, Now: time.Now})
	iocsLocal := NewLocalLRU(LocalLRUConfig{Capacity: opts.LocalCaps.IOCs, Now: time.Now})
	filesLocal := NewLocalLRU(LocalLRUConfig{Capacity: opts.LocalCaps.ProcessedFiles, Now: time.Now})

	versioner := opts.IOCVersioner
	if versioner == nil {
		versioner = NewIOCCacheVersioner(opts.Redis, "", defaultIOCCacheVersionRefresh)
	}

	seen := NewSeenDomainsCache(
		SeenDomainsCacheDeps{
			Local:   seenLocal,
			Redis:   opts.Redis,
			Repo:    opts.SeenRepo,
			TTL:     opts.TTL,
			Metrics: opts.Metrics,
		},
	)
	iocs := NewIOCCache(
		IOCCacheDeps{
			Local:     iocsLocal,
			Redis:     opts.Redis,
			Repo:      opts.IOCsRepo,
			TTL:       opts.TTL,
			Metrics:   opts.Metrics,
			Versioner: versioner,
		},
	)
	files := NewProcessedFilesCache(
		ProcessedFilesCacheDeps{
			Local:   filesLocal,
			Redis:   opts.Redis,
			Repo:    opts.FilesRepo,
			TTL:     opts.TTL,
			Metrics: opts.Metrics,
		},
	)
	// dedicated small local cache for alert-once to avoid eviction by seen-domain churn
	alertLocal := NewLocalLRU(LocalLRUConfig{Capacity: 2_048, Now: time.Now})
	alertOnce := NewAlertOnceCache(alertLocal, opts.Redis)

	return Caches{
		Seen:      seen,
		IOCs:      iocs,
		Files:     files,
		AlertOnce: alertOnce,
	}
}
