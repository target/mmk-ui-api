package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service/rules"
)

// IOCServiceOptions groups dependencies for IOCService.
type IOCServiceOptions struct {
	Repo           core.IOCRepository // Required: IOC repository
	Logger         *slog.Logger       // Optional: structured logger
	CacheVersioner rules.IOCVersioner // Optional: notify caches when IOCs mutate
}

// IOCService provides business logic for IOC operations.
type IOCService struct {
	repo      core.IOCRepository
	logger    *slog.Logger
	versioner rules.IOCVersioner
}

// NewIOCService constructs a new IOCService with validation.
func NewIOCService(opts IOCServiceOptions) (*IOCService, error) {
	if opts.Repo == nil {
		return nil, errors.New("IOCRepository is required")
	}

	// Create component-scoped logger
	var logger *slog.Logger
	if opts.Logger != nil {
		logger = opts.Logger.With("component", "ioc_service")
		logger.Debug("IOCService initialized")
	}

	return &IOCService{
		repo:      opts.Repo,
		logger:    logger,
		versioner: opts.CacheVersioner,
	}, nil
}

// MustNewIOCService constructs a new IOCService and panics on error.
func MustNewIOCService(opts IOCServiceOptions) *IOCService {
	svc, err := NewIOCService(opts)
	if err != nil {
		panic(err) //nolint:forbidigo // Must constructor fails fast when dependencies are invalid during startup
	}
	return svc
}

// Create creates a new IOC with the given request parameters.
func (s *IOCService) Create(ctx context.Context, req model.CreateIOCRequest) (*model.IOC, error) {
	ioc, err := s.repo.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create IOC: %w", err)
	}

	if s.logger != nil {
		s.logger.DebugContext(
			ctx,
			"IOC created",
			"id",
			ioc.ID,
			"type",
			ioc.Type,
			"value",
			ioc.Value,
		)
	}

	s.bumpCache(ctx)

	return ioc, nil
}

// GetByID retrieves an IOC by its ID.
func (s *IOCService) GetByID(ctx context.Context, id string) (*model.IOC, error) {
	ioc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get IOC by ID: %w", err)
	}
	return ioc, nil
}

// List retrieves IOCs with filtering and pagination.
func (s *IOCService) List(ctx context.Context, opts model.IOCListOptions) ([]*model.IOC, error) {
	iocs, err := s.repo.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list IOCs: %w", err)
	}
	return iocs, nil
}

// Update updates an existing IOC with the given request parameters.
func (s *IOCService) Update(
	ctx context.Context,
	id string,
	req model.UpdateIOCRequest,
) (*model.IOC, error) {
	ioc, err := s.repo.Update(ctx, id, req)
	if err != nil {
		return nil, fmt.Errorf("update IOC: %w", err)
	}

	if s.logger != nil {
		s.logger.DebugContext(ctx, "IOC updated", "id", ioc.ID)
	}

	s.bumpCache(ctx)

	return ioc, nil
}

// Delete deletes an IOC by its ID.
// Returns (true, nil) if deleted, (false, nil) if not found, or (false, error) on failure.
func (s *IOCService) Delete(ctx context.Context, id string) (bool, error) {
	deleted, err := s.repo.Delete(ctx, id)
	if err != nil {
		return false, fmt.Errorf("delete IOC: %w", err)
	}

	if s.logger != nil && deleted {
		s.logger.DebugContext(ctx, "IOC deleted", "id", id)
	}

	if deleted {
		s.bumpCache(ctx)
	}

	return deleted, nil
}

// BulkCreate creates multiple IOCs in bulk.
func (s *IOCService) BulkCreate(ctx context.Context, req model.BulkCreateIOCsRequest) (int, error) {
	count, err := s.repo.BulkCreate(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("bulk create IOCs: %w", err)
	}

	if s.logger != nil {
		s.logger.DebugContext(ctx, "IOCs bulk created", "count", count, "type", req.Type)
	}

	if count > 0 {
		s.bumpCache(ctx)
	}

	return count, nil
}

// Stats retrieves IOC statistics.
func (s *IOCService) Stats(ctx context.Context) (*core.IOCStats, error) {
	stats, err := s.repo.Stats(ctx)
	if err != nil {
		return nil, fmt.Errorf("get IOC stats: %w", err)
	}
	return stats, nil
}

func (s *IOCService) bumpCache(ctx context.Context) {
	if s.versioner == nil {
		return
	}
	if _, err := s.versioner.Bump(ctx); err != nil && s.logger != nil {
		s.logger.WarnContext(ctx, "failed to bump IOC cache version", "error", err)
	}
}
