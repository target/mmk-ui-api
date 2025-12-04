package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// RuleServiceOptions groups dependencies for RuleService.
type RuleServiceOptions struct {
	Repo   core.RuleRepository // Required: rule repository
	Logger *slog.Logger        // Optional: structured logger
}

// RuleService provides business logic for rule operations.
type RuleService struct {
	repo   core.RuleRepository
	logger *slog.Logger
}

// NewRuleService constructs a new RuleService.
func NewRuleService(opts RuleServiceOptions) (*RuleService, error) {
	if opts.Repo == nil {
		return nil, fmt.Errorf("validate options: %w", errors.New("RuleRepository is required"))
	}

	if opts.Logger != nil {
		opts.Logger.Debug("RuleService initialized")
	}

	return &RuleService{
		repo:   opts.Repo,
		logger: opts.Logger,
	}, nil
}

// Create creates a new rule with the given request parameters.
func (s *RuleService) Create(
	ctx context.Context,
	req model.CreateRuleRequest,
) (*model.Rule, error) {
	req.Normalize()
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate request: %w", err)
	}

	rule, err := s.repo.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create rule: %w", err)
	}

	if s.logger != nil {
		s.logger.InfoContext(ctx, "rule created", "id", rule.ID, "rule_type", rule.RuleType)
	}

	return rule, nil
}

// GetByID retrieves a rule by its ID.
func (s *RuleService) GetByID(ctx context.Context, id string) (*model.Rule, error) {
	rule, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get rule by id: %w", err)
	}

	return rule, nil
}

// List retrieves rules based on the provided options.
func (s *RuleService) List(ctx context.Context, opts model.RuleListOptions) ([]*model.Rule, error) {
	rules, err := s.repo.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}

	return rules, nil
}

// Update updates an existing rule with the given request parameters.
func (s *RuleService) Update(
	ctx context.Context,
	id string,
	req model.UpdateRuleRequest,
) (*model.Rule, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate request: %w", err)
	}

	rule, err := s.repo.Update(ctx, id, req)
	if err != nil {
		return nil, fmt.Errorf("update rule: %w", err)
	}

	if s.logger != nil {
		s.logger.InfoContext(ctx, "rule updated", "id", rule.ID, "rule_type", rule.RuleType)
	}

	return rule, nil
}

// Delete removes a rule by its ID.
func (s *RuleService) Delete(ctx context.Context, id string) (bool, error) {
	deleted, err := s.repo.Delete(ctx, id)
	if err != nil {
		return false, fmt.Errorf("delete rule: %w", err)
	}

	if s.logger != nil && deleted {
		s.logger.InfoContext(ctx, "rule deleted", "id", id)
	}

	return deleted, nil
}

// GetBySite retrieves all rules for a specific site.
func (s *RuleService) GetBySite(
	ctx context.Context,
	siteID string,
	enabled *bool,
) ([]*model.Rule, error) {
	rules, err := s.repo.GetBySite(ctx, siteID, enabled)
	if err != nil {
		return nil, fmt.Errorf("get rules by site: %w", err)
	}

	return rules, nil
}
