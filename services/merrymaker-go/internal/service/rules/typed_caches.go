package rules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// ErrNotFound is a sentinel error used when cached items are absent.
var ErrNotFound = errors.New("not found")

// ---------- Redis Key Prefixes ----------

// seenScopeKeyPrefix is the Redis key prefix for seen domain tracking.
const seenScopeKeyPrefix = "rules:seen:scope:"

// fileScopeKeyPrefix is the Redis key prefix for processed file tracking.
const fileScopeKeyPrefix = "rules:file:scope:"

// ---------- Common helpers ----------

func seenKeyRedis(k SeenKey) string {
	// rules:seen:scope:<scope>:domain:<domain> (scope-wide across sites)
	return seenScopeKeyPrefix + k.Scope.Scope + ":domain:" + k.Domain
}

func fileKeyRedis(k FileKey) string {
	// rules:file:scope:<scope>:sha:<sha256> (scope-wide across sites)
	return fileScopeKeyPrefix + k.Scope.Scope + ":sha:" + strings.ToLower(k.FileHash)
}

// ---------- Seen Domains Cache ----------

type SeenDomainsCacheDeps struct {
	Local   *LocalLRU
	Redis   core.CacheRepository
	Repo    core.SeenDomainRepository
	TTL     CacheTTL
	Metrics CacheMetrics
}

type SeenDomainsCacheImpl struct {
	local   *LocalLRU
	redis   core.CacheRepository
	repo    core.SeenDomainRepository
	ttl     CacheTTL
	metrics CacheMetrics
}

func NewSeenDomainsCache(deps SeenDomainsCacheDeps) *SeenDomainsCacheImpl {
	m := deps.Metrics
	if m == nil {
		m = NoopCacheMetrics{}
	}
	return &SeenDomainsCacheImpl{
		local:   deps.Local,
		redis:   deps.Redis,
		repo:    deps.Repo,
		ttl:     deps.TTL,
		metrics: m,
	}
}

func (c *SeenDomainsCacheImpl) emit(e CacheEvent) {
	c.metrics.RecordCacheEvent(e)
}

// helpers to reduce complexity for SeenDomainsCacheImpl.
func (c *SeenDomainsCacheImpl) redisExistsSeen(ctx context.Context, k string) (bool, error) {
	if c.redis == nil {
		return false, nil
	}
	exists, err := c.redis.Exists(ctx, k)
	if err != nil {
		c.emit(CacheEvent{Name: CacheSeen, Tier: TierRedis, Op: OpMiss, Ok: false})
		return false, err
	}
	if exists && c.local != nil {
		c.local.Set(k, []byte("1"), c.ttl.SeenDomainsLocal)
	}
	op := OpMiss
	if exists {
		op = OpHit
	}
	c.emit(CacheEvent{Name: CacheSeen, Tier: TierRedis, Op: op, Ok: true})
	return exists, nil
}

func (c *SeenDomainsCacheImpl) repoExistsSeen(
	ctx context.Context,
	key SeenKey,
	k string,
) (bool, error) {
	if c.repo == nil {
		return false, nil
	}
	d := strings.ToLower(strings.TrimSpace(key.Domain))
	res, err := c.repo.Lookup(
		ctx,
		model.SeenDomainLookupRequest{SiteID: key.Scope.SiteID, Domain: d, Scope: key.Scope.Scope},
	)
	if err != nil {
		return false, err
	}
	if res == nil {
		c.emit(CacheEvent{Name: CacheSeen, Tier: TierRepo, Op: OpMiss, Ok: true})
		return false, nil
	}
	c.emit(CacheEvent{Name: CacheSeen, Tier: TierRepo, Op: OpHit, Ok: true})

	if c.redis != nil {
		if err2 := c.redis.Set(ctx, k, []byte("1"), c.ttl.SeenDomainsRedis); err2 != nil {
			c.emit(CacheEvent{Name: CacheSeen, Tier: TierRedis, Op: OpWrite, Ok: false})
		} else {
			c.emit(CacheEvent{Name: CacheSeen, Tier: TierRedis, Op: OpWrite, Ok: true})
		}
	}
	if c.local != nil {
		c.local.Set(k, []byte("1"), c.ttl.SeenDomainsLocal)
		c.emit(CacheEvent{Name: CacheSeen, Tier: TierLocal, Op: OpWrite, Ok: true})
	}

	return true, nil
}

func (c *SeenDomainsCacheImpl) Exists(ctx context.Context, key SeenKey) (bool, error) {
	if err := key.Validate(); err != nil {
		return false, err
	}
	k := seenKeyRedis(
		SeenKey{Scope: key.Scope, Domain: strings.ToLower(strings.TrimSpace(key.Domain))},
	)
	if c.local != nil {
		if c.local.Exists(k) {
			c.emit(CacheEvent{Name: CacheSeen, Tier: TierLocal, Op: OpHit, Ok: true})
			return true, nil
		}
		c.emit(CacheEvent{Name: CacheSeen, Tier: TierLocal, Op: OpMiss, Ok: true})
	}
	if c.redis != nil {
		if ok, err := c.redisExistsSeen(ctx, k); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
	}
	ok, err := c.repoExistsSeen(ctx, key, k)
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (c *SeenDomainsCacheImpl) Record(ctx context.Context, key SeenKey) error {
	if err := key.Validate(); err != nil {
		return err
	}
	k := seenKeyRedis(
		SeenKey{Scope: key.Scope, Domain: strings.ToLower(strings.TrimSpace(key.Domain))},
	)
	if c.local != nil {
		c.local.Set(k, []byte("1"), c.ttl.SeenDomainsLocal)
		c.emit(CacheEvent{Name: CacheSeen, Tier: TierLocal, Op: OpWrite, Ok: true})
	}
	if c.redis != nil {
		if err := c.redis.Set(ctx, k, []byte("1"), c.ttl.SeenDomainsRedis); err != nil {
			c.emit(CacheEvent{Name: CacheSeen, Tier: TierRedis, Op: OpWrite, Ok: false})
			return err
		}
		c.emit(CacheEvent{Name: CacheSeen, Tier: TierRedis, Op: OpWrite, Ok: true})
	}
	if c.repo != nil {
		d := strings.ToLower(strings.TrimSpace(key.Domain))
		_, err := c.repo.RecordSeen(
			ctx,
			model.RecordDomainSeenRequest{
				SiteID: key.Scope.SiteID,
				Domain: d,
				Scope:  key.Scope.Scope,
			},
		)
		if err != nil {
			c.emit(CacheEvent{Name: CacheSeen, Tier: TierRepo, Op: OpWrite, Ok: false})
		} else {
			c.emit(CacheEvent{Name: CacheSeen, Tier: TierRepo, Op: OpWrite, Ok: true})
		}
		return err
	}
	return nil
}

// ---------- IOC Domains Cache ----------

// ---------- Processed Files Cache ----------

type ProcessedFilesCacheDeps struct {
	Local   *LocalLRU
	Redis   core.CacheRepository
	Repo    core.ProcessedFileRepository
	TTL     CacheTTL
	Metrics CacheMetrics
}

type ProcessedFilesCacheImpl struct {
	local   *LocalLRU
	redis   core.CacheRepository
	repo    core.ProcessedFileRepository
	ttl     CacheTTL
	metrics CacheMetrics
}

func (c *ProcessedFilesCacheImpl) emit(e CacheEvent) {
	c.metrics.RecordCacheEvent(e)
}

func NewProcessedFilesCache(deps ProcessedFilesCacheDeps) *ProcessedFilesCacheImpl {
	m := deps.Metrics
	if m == nil {
		m = NoopCacheMetrics{}
	}
	return &ProcessedFilesCacheImpl{
		local:   deps.Local,
		redis:   deps.Redis,
		repo:    deps.Repo,
		ttl:     deps.TTL,
		metrics: m,
	}
}

// helpers to reduce complexity for ProcessedFilesCacheImpl.
func (c *ProcessedFilesCacheImpl) redisExistsFile(ctx context.Context, k string) (bool, error) {
	if c.redis == nil {
		return false, nil
	}
	exists, err := c.redis.Exists(ctx, k)
	if err != nil {
		c.emit(CacheEvent{Name: CacheFiles, Tier: TierRedis, Op: OpMiss, Ok: false})
		return false, err
	}
	if exists && c.local != nil {
		c.local.Set(k, []byte("1"), c.ttl.ProcessedFilesLocal)
	}
	op := OpMiss
	if exists {
		op = OpHit
	}
	c.emit(CacheEvent{Name: CacheFiles, Tier: TierRedis, Op: op, Ok: true})
	return exists, nil
}

func (c *ProcessedFilesCacheImpl) repoExistsFile(
	ctx context.Context,
	key FileKey,
	k string,
) (bool, error) {
	if c.repo == nil {
		return false, nil
	}
	res, err := c.repo.Lookup(
		ctx,
		model.ProcessedFileLookupRequest{
			SiteID:   key.Scope.SiteID,
			FileHash: strings.ToLower(key.FileHash),
			Scope:    key.Scope.Scope,
		},
	)
	if err != nil {
		return false, err
	}
	if res == nil {
		c.emit(CacheEvent{Name: CacheFiles, Tier: TierRepo, Op: OpMiss, Ok: true})
		return false, nil
	}
	c.emit(CacheEvent{Name: CacheFiles, Tier: TierRepo, Op: OpHit, Ok: true})
	if c.redis != nil {
		if err2 := c.redis.Set(ctx, k, []byte("1"), c.ttl.ProcessedFilesRedis); true {
			c.emit(CacheEvent{Name: CacheFiles, Tier: TierRedis, Op: OpWrite, Ok: err2 == nil})
		}
	}
	if c.local != nil {
		c.local.Set(k, []byte("1"), c.ttl.ProcessedFilesLocal)
		c.emit(CacheEvent{Name: CacheFiles, Tier: TierLocal, Op: OpWrite, Ok: true})
	}
	return true, nil
}

func (c *ProcessedFilesCacheImpl) IsProcessed(ctx context.Context, key FileKey) (bool, error) {
	if err := key.Validate(); err != nil {
		return false, err
	}
	k := fileKeyRedis(key)
	if c.local != nil {
		if c.local.Exists(k) {
			c.emit(CacheEvent{Name: CacheFiles, Tier: TierLocal, Op: OpHit, Ok: true})
			return true, nil
		}
		c.emit(CacheEvent{Name: CacheFiles, Tier: TierLocal, Op: OpMiss, Ok: true})
	}
	if ok, err := c.redisExistsFile(ctx, k); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}
	ok, err := c.repoExistsFile(ctx, key, k)
	if err != nil {
		return false, err
	}
	return ok, nil
}

// ensureRepoFileParams groups parameters for ensureRepoFile to keep param count ≤3.
type ensureRepoFileParams struct {
	key        FileKey
	cacheKey   string
	storageKey string
}

func (c *ProcessedFilesCacheImpl) ensureRepoFile(
	ctx context.Context,
	params ensureRepoFileParams,
) (bool, error) {
	if c.repo == nil {
		return false, nil
	}
	ok, err := c.repoExistsFile(ctx, params.key, params.cacheKey)
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}
	_, err = c.repo.Create(ctx, model.CreateProcessedFileRequest{
		SiteID:     params.key.Scope.SiteID,
		FileHash:   strings.ToLower(params.key.FileHash),
		StorageKey: params.storageKey,
		Scope:      params.key.Scope.Scope,
		// ProcessedAt left nil -> repo/db defaults
	})
	if err != nil {
		return false, err
	}
	return false, nil
}

func (c *ProcessedFilesCacheImpl) writeProcessedCaches(ctx context.Context, k string) error {
	if c.redis != nil {
		if err := c.redis.Set(ctx, k, []byte("1"), c.ttl.ProcessedFilesRedis); err != nil {
			return err
		}
		c.emit(CacheEvent{Name: CacheFiles, Tier: TierRedis, Op: OpWrite, Ok: true})
	}
	if c.local != nil {
		c.local.Set(k, []byte("1"), c.ttl.ProcessedFilesLocal)
		c.emit(CacheEvent{Name: CacheFiles, Tier: TierLocal, Op: OpWrite, Ok: true})
	}
	return nil
}

func (c *ProcessedFilesCacheImpl) MarkProcessed(
	ctx context.Context,
	key FileKey,
	storageKey string,
) error {
	if err := key.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(storageKey) == "" {
		return errors.New("storage_key is required")
	}
	k := fileKeyRedis(key)
	// Ensure DB state first (if repo provided). If record already exists, we are done.
	existed, err := c.ensureRepoFile(ctx, ensureRepoFileParams{
		key:        key,
		cacheKey:   k,
		storageKey: storageKey,
	})
	if err != nil {
		return err
	}
	if existed {
		// Prime caches even when DB already had the record
		return c.writeProcessedCaches(ctx, k)
	}
	// Write-through to caches.
	return c.writeProcessedCaches(ctx, k)
}

// ---------- IOC Cache (Global IOCs) ----------

type IOCCacheDeps struct {
	Local     *LocalLRU
	Redis     core.CacheRepository
	Repo      core.IOCRepository
	TTL       CacheTTL
	Metrics   CacheMetrics
	Versioner IOCVersioner
}

type IOCCacheImpl struct {
	local     *LocalLRU
	redis     core.CacheRepository
	repo      core.IOCRepository
	ttl       CacheTTL
	metrics   CacheMetrics
	versioner IOCVersioner
}

func NewIOCCache(deps IOCCacheDeps) *IOCCacheImpl {
	m := deps.Metrics
	if m == nil {
		m = NoopCacheMetrics{}
	}
	return &IOCCacheImpl{
		local:     deps.Local,
		redis:     deps.Redis,
		repo:      deps.Repo,
		ttl:       deps.TTL,
		metrics:   m,
		versioner: deps.Versioner,
	}
}

func (c *IOCCacheImpl) emit(e CacheEvent) {
	c.metrics.RecordCacheEvent(e)
}

func iocHostKey(version, host string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		version = "0"
	}
	return "rules:ioc:host:v" + version + ":" + strings.ToLower(strings.TrimSpace(host))
}

func (c *IOCCacheImpl) currentVersion(ctx context.Context) (string, error) {
	if c.versioner == nil {
		return "0", nil
	}
	version, err := c.versioner.Current(ctx)
	version = strings.TrimSpace(version)
	if version == "" {
		version = "0"
	}
	return version, err
}

// LookupHost checks if a host (domain or IP) matches any enabled IOC.
// Uses three-tier caching: local LRU -> Redis -> Postgres (with matcher).
// Supports negative caching to avoid repeated DB lookups for non-IOC hosts.
func (c *IOCCacheImpl) LookupHost(ctx context.Context, host string) (*model.IOC, error) {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return nil, errors.New("host is required")
	}

	version, err := c.currentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("current IOC version: %w", err)
	}
	k := iocHostKey(version, h)

	// Try local cache first
	if ioc, found := c.fromLocal(k); found {
		return ioc, nil
	}

	// Try Redis cache
	if ioc, found := c.fromRedis(ctx, k); found {
		return ioc, nil
	}

	// Fall back to repository (with matcher)
	ioc, found, err := c.fromRepo(ctx, h, k)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}

	return ioc, nil
}

// fromLocal checks the local LRU cache.
func (c *IOCCacheImpl) fromLocal(k string) (*model.IOC, bool) {
	if c.local == nil {
		return nil, false
	}

	b, ok := c.local.Get(k)
	if !ok {
		c.emit(CacheEvent{Name: CacheIOC, Tier: TierLocal, Op: OpMiss, Ok: true})
		return nil, false
	}

	c.emit(CacheEvent{Name: CacheIOC, Tier: TierLocal, Op: OpHit, Ok: true})

	// Check for negative cache marker
	if string(b) == negativeMarker {
		return nil, true // Found negative marker, return nil IOC
	}

	var ioc model.IOC
	if err := json.Unmarshal(b, &ioc); err != nil {
		// Unmarshal error - evict corrupted entry to self-heal
		if c.local != nil {
			c.local.Delete(k)
		}
		return nil, false
	}

	return &ioc, true
}

// fromRedis checks the Redis cache.
func (c *IOCCacheImpl) fromRedis(ctx context.Context, k string) (*model.IOC, bool) {
	if c.redis == nil {
		return nil, false
	}

	b, err := c.redis.Get(ctx, k)
	if err != nil || b == nil {
		c.emit(CacheEvent{Name: CacheIOC, Tier: TierRedis, Op: OpMiss, Ok: err == nil})
		return nil, false
	}

	c.emit(CacheEvent{Name: CacheIOC, Tier: TierRedis, Op: OpHit, Ok: true})

	// Check for negative cache marker
	if string(b) == negativeMarker {
		// Write to local cache
		if c.local != nil {
			c.local.Set(k, b, c.ttl.IOCsLocal)
			c.emit(CacheEvent{Name: CacheIOC, Tier: TierLocal, Op: OpWrite, Ok: true})
		}
		return nil, true // Found negative marker, return nil IOC
	}

	var ioc model.IOC
	if parseErr := json.Unmarshal(b, &ioc); parseErr != nil {
		c.evictCorruptedRedisEntry(ctx, k)
		return nil, false
	}

	// Write to local cache
	if c.local != nil {
		c.local.Set(k, b, c.ttl.IOCsLocal)
		c.emit(CacheEvent{Name: CacheIOC, Tier: TierLocal, Op: OpWrite, Ok: true})
	}

	return &ioc, true
}

func (c *IOCCacheImpl) evictCorruptedRedisEntry(ctx context.Context, key string) {
	if c.redis == nil {
		return
	}
	deleted, err := c.redis.Delete(ctx, key)
	c.emit(CacheEvent{Name: CacheIOC, Tier: TierRedis, Op: OpWrite, Ok: err == nil && deleted})
}

// fromRepo queries the repository and caches the result (positive or negative).
func (c *IOCCacheImpl) fromRepo(ctx context.Context, host, k string) (*model.IOC, bool, error) {
	if c.repo == nil {
		return nil, false, nil
	}

	ioc, err := c.repo.LookupHost(ctx, model.IOCLookupRequest{Host: host})
	if err != nil {
		// Not found is expected, cache negative result
		// Check both ErrNotFound (rules layer) and data.ErrIOCNotFound (data layer)
		if errors.Is(err, ErrNotFound) || isIOCNotFoundErr(err) {
			c.emit(CacheEvent{Name: CacheIOC, Tier: TierRepo, Op: OpMiss, Ok: true})
			c.cacheNegative(ctx, k)
			return nil, false, nil
		}
		return nil, false, err
	}

	c.emit(CacheEvent{Name: CacheIOC, Tier: TierRepo, Op: OpHit, Ok: true})

	// Cache positive result
	b, err := json.Marshal(ioc)
	if err != nil {
		return nil, false, fmt.Errorf("marshal IOC: %w", err)
	}
	if c.redis != nil {
		if err2 := c.redis.Set(ctx, k, b, c.ttl.IOCsRedis); err2 != nil {
			c.emit(CacheEvent{Name: CacheIOC, Tier: TierRedis, Op: OpWrite, Ok: false})
		} else {
			c.emit(CacheEvent{Name: CacheIOC, Tier: TierRedis, Op: OpWrite, Ok: true})
		}
	}
	if c.local != nil {
		c.local.Set(k, b, c.ttl.IOCsLocal)
		c.emit(CacheEvent{Name: CacheIOC, Tier: TierLocal, Op: OpWrite, Ok: true})
	}

	return ioc, true, nil
}

// isIOCNotFoundErr checks if the error is the data layer's ErrIOCNotFound.
// This avoids a direct import of internal/data in the rules package.
func isIOCNotFoundErr(err error) bool {
	return err != nil && err.Error() == "IOC not found"
}

// cacheNegative stores a negative cache marker to avoid repeated DB lookups.
func (c *IOCCacheImpl) cacheNegative(ctx context.Context, k string) {
	marker := []byte(negativeMarker)
	if c.redis != nil {
		if err := c.redis.Set(ctx, k, marker, c.ttl.IOCsRedis); err == nil {
			c.emit(CacheEvent{Name: CacheIOC, Tier: TierRedis, Op: OpWrite, Ok: true})
		}
	}
	if c.local != nil {
		c.local.Set(k, marker, c.ttl.IOCsLocal)
		c.emit(CacheEvent{Name: CacheIOC, Tier: TierLocal, Op: OpWrite, Ok: true})
	}
}

const negativeMarker = "__NOT_FOUND__"

// Ensure IOCCacheImpl implements IOCCache.
var _ IOCCache = (*IOCCacheImpl)(nil)
