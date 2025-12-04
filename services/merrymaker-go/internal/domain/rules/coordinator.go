package rules

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// JobCoordinatorOptions configure the behavior of DefaultJobCoordinator.
type JobCoordinatorOptions struct {
	Cache     core.CacheRepository
	TTL       time.Duration
	BatchSize int
	Logger    *slog.Logger
}

// DefaultJobCoordinator encapsulates dedupe locking, payload parsing, and batch sizing decisions.
type DefaultJobCoordinator struct {
	cache     core.CacheRepository
	ttl       time.Duration
	batchSize int
	logger    *slog.Logger
}

// NewJobCoordinator constructs a DefaultJobCoordinator from the provided options.
func NewJobCoordinator(opts JobCoordinatorOptions) *DefaultJobCoordinator {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}

	return &DefaultJobCoordinator{
		cache:     opts.Cache,
		ttl:       ttl,
		batchSize: opts.BatchSize,
		logger:    logger,
	}
}

// BuildPayload marshals a job payload from the enqueue request.
func (c *DefaultJobCoordinator) BuildPayload(req *EnqueueJobRequest) ([]byte, error) {
	if req == nil {
		return nil, errors.New("build payload: request is nil")
	}
	payload := JobPayload{
		EventIDs: req.EventIDs,
		SiteID:   req.SiteID,
		Scope:    req.Scope,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	return b, nil
}

// ShouldProcess attempts to acquire a dedupe lock and reports whether processing should continue.
func (c *DefaultJobCoordinator) ShouldProcess(
	ctx context.Context,
	req *EnqueueJobRequest,
) (bool, error) {
	if req == nil {
		return false, errors.New("should process: request is nil")
	}
	if c.cache == nil {
		return true, nil
	}

	ids := make([]string, len(req.EventIDs))
	copy(ids, req.EventIDs)
	sort.Strings(ids)

	sum := sha256.Sum256([]byte(strings.Join(ids, ",")))
	key := fmt.Sprintf(
		"rules:dedupe:rules_job:site:%s:scope:%s:events:%x",
		req.SiteID,
		req.Scope,
		sum,
	)

	ok, err := c.cache.SetIfNotExists(ctx, key, []byte("1"), c.ttl)
	if err != nil {
		if c.logger != nil {
			c.logger.WarnContext(
				ctx,
				"dedupe lock set failed; proceeding without dedupe",
				"error",
				err,
			)
		}
		return true, nil
	}
	if ok {
		return true, nil
	}

	if c.logger != nil {
		c.logger.InfoContext(ctx, "skipping enqueue: duplicate rules job request",
			"site_id", req.SiteID,
			"scope", req.Scope,
			"event_count", len(req.EventIDs))
	}
	return false, nil
}

// ParsePayload parses and validates a job payload from the provided job.
func (c *DefaultJobCoordinator) ParsePayload(job *model.Job) (*JobPayload, error) {
	if job == nil {
		return nil, errors.New("parse payload: job is nil")
	}
	var payload JobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal job payload: %w", err)
	}
	return &payload, nil
}

// LimitEventIDs enforces the configured batch size on the provided set of event IDs.
func (c *DefaultJobCoordinator) LimitEventIDs(ids []string, jobID string) []string {
	if c.batchSize > 0 && len(ids) > c.batchSize {
		if c.logger != nil {
			c.logger.Info("truncating event IDs to batch size",
				"from", len(ids),
				"to", c.batchSize,
				"job_id", jobID)
		}
		out := make([]string, c.batchSize)
		copy(out, ids[:c.batchSize])
		return out
	}
	return ids
}

var _ JobCoordinator = (*DefaultJobCoordinator)(nil)
