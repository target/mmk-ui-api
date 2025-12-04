package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// DomainAllowlistServiceOptions groups dependencies for DomainAllowlistService.
type DomainAllowlistServiceOptions struct {
	Repo   core.DomainAllowlistRepository // Required: domain allowlist repository
	Logger *slog.Logger                   // Optional: structured logger
}

// DomainAllowlistService provides business logic for domain allowlist operations.
type DomainAllowlistService struct {
	repo   core.DomainAllowlistRepository
	logger *slog.Logger
}

// NewDomainAllowlistService constructs a new DomainAllowlistService.
func NewDomainAllowlistService(opts DomainAllowlistServiceOptions) *DomainAllowlistService {
	if opts.Repo == nil {
		//nolint:forbidigo // Service construction must fail fast during wiring when dependencies are missing
		panic("DomainAllowlistRepository is required")
	}

	if opts.Logger != nil {
		opts.Logger.Debug("DomainAllowlistService initialized")
	}

	return &DomainAllowlistService{
		repo:   opts.Repo,
		logger: opts.Logger,
	}
}

// Create creates a new domain allowlist entry with the given request parameters.
func (s *DomainAllowlistService) Create(
	ctx context.Context,
	req *model.CreateDomainAllowlistRequest,
) (*model.DomainAllowlist, error) {
	if req == nil {
		return nil, errors.New("create domain allowlist request is required")
	}

	allowlist, err := s.repo.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create domain allowlist: %w", err)
	}

	if s.logger != nil {
		s.logger.InfoContext(ctx,
			"domain allowlist entry created",
			"id",
			allowlist.ID,
			"pattern",
			allowlist.Pattern,
		)
	}

	return allowlist, nil
}

// GetByID retrieves a domain allowlist entry by its ID.
func (s *DomainAllowlistService) GetByID(
	ctx context.Context,
	id string,
) (*model.DomainAllowlist, error) {
	allowlist, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get domain allowlist by id: %w", err)
	}

	return allowlist, nil
}

// Update updates an existing domain allowlist entry with the given request parameters.
func (s *DomainAllowlistService) Update(
	ctx context.Context,
	id string,
	req model.UpdateDomainAllowlistRequest,
) (*model.DomainAllowlist, error) {
	req.Normalize()
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate request: %w", err)
	}

	allowlist, err := s.repo.Update(ctx, id, req)
	if err != nil {
		return nil, fmt.Errorf("update domain allowlist: %w", err)
	}

	if s.logger != nil {
		s.logger.InfoContext(ctx,
			"domain allowlist entry updated",
			"id",
			allowlist.ID,
			"pattern",
			allowlist.Pattern,
		)
	}

	return allowlist, nil
}

// Delete deletes a domain allowlist entry by its ID.
func (s *DomainAllowlistService) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete domain allowlist: %w", err)
	}

	if s.logger != nil {
		s.logger.InfoContext(ctx, "domain allowlist entry deleted", "id", id)
	}

	return nil
}

// List retrieves a list of domain allowlist entries with the given options.
func (s *DomainAllowlistService) List(
	ctx context.Context,
	opts model.DomainAllowlistListOptions,
) ([]*model.DomainAllowlist, error) {
	allowlists, err := s.repo.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list domain allowlists: %w", err)
	}

	return allowlists, nil
}

// GetForScope retrieves all enabled domain allowlist entries for a specific site and scope.
// This includes both site-specific entries and global entries, ordered by priority.
func (s *DomainAllowlistService) GetForScope(
	ctx context.Context,
	req model.DomainAllowlistLookupRequest,
) ([]*model.DomainAllowlist, error) {
	allowlists, err := s.repo.GetForScope(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get domain allowlists for scope: %w", err)
	}

	return allowlists, nil
}

// Stats retrieves domain allowlist statistics, optionally filtered by site ID.
func (s *DomainAllowlistService) Stats(
	ctx context.Context,
	siteID *string,
) (*model.DomainAllowlistStats, error) {
	stats, err := s.repo.Stats(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("get domain allowlist stats: %w", err)
	}

	return stats, nil
}
