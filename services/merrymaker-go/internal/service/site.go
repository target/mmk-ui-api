package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// SiteServiceOptions groups dependencies for SiteService.
type SiteServiceOptions struct {
	SiteRepo core.SiteRepository
	Admin    core.ScheduledJobsAdminRepository
}

// SiteService orchestrates site CRUD with scheduler reconciliation.
type SiteService struct {
	sites core.SiteRepository
	adm   core.ScheduledJobsAdminRepository
}

// NewSiteService constructs a new SiteService.
func NewSiteService(opts SiteServiceOptions) *SiteService {
	return &SiteService{sites: opts.SiteRepo, adm: opts.Admin}
}

// Create creates a site and reconciles its scheduled job if enabled.
func (s *SiteService) Create(ctx context.Context, req *model.CreateSiteRequest) (*model.Site, error) {
	site, err := s.sites.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	if reconcileErr := s.reconcileSchedule(ctx, site); reconcileErr != nil {
		return nil, fmt.Errorf("reconcile schedule: %w", reconcileErr)
	}
	return site, nil
}

// Update updates a site and reconciles its scheduled job.
func (s *SiteService) Update(ctx context.Context, id string, req model.UpdateSiteRequest) (*model.Site, error) {
	site, err := s.sites.Update(ctx, id, req)
	if err != nil {
		return nil, err
	}
	if reconcileErr := s.reconcileSchedule(ctx, site); reconcileErr != nil {
		return nil, fmt.Errorf("reconcile schedule: %w", reconcileErr)
	}
	return site, nil
}

// Delete deletes a site and removes its scheduled job if present.
func (s *SiteService) Delete(ctx context.Context, id string) (bool, error) {
	ok, err := s.sites.Delete(ctx, id)
	if err != nil || !ok {
		return ok, err
	}
	_, delErr := s.adm.DeleteByTaskName(ctx, taskNameForSite(id))
	if delErr != nil {
		return ok, fmt.Errorf("delete schedule: %w", delErr)
	}
	return ok, nil
}

// GetByID retrieves a site by ID.
func (s *SiteService) GetByID(ctx context.Context, id string) (*model.Site, error) {
	return s.sites.GetByID(ctx, id)
}

// List returns a page of sites.
func (s *SiteService) List(ctx context.Context, limit, offset int) ([]*model.Site, error) {
	return s.sites.List(ctx, limit, offset)
}

// ListWithOptions returns sites using optional filters when the repository supports it; otherwise falls back to unfiltered list.
func (s *SiteService) ListWithOptions(ctx context.Context, opts model.SitesListOptions) ([]*model.Site, error) {
	repo, ok := any(s.sites).(core.SiteRepositoryListWithOptions)
	if !ok {
		return s.sites.List(ctx, opts.Limit, opts.Offset)
	}

	normalizedOpts := normalizeSiteListOptions(opts)
	return repo.ListWithOptions(ctx, normalizedOpts)
}

func (s *SiteService) reconcileSchedule(ctx context.Context, site *model.Site) error {
	if s.adm == nil || site == nil {
		return nil
	}
	name := taskNameForSite(site.ID)
	if site.Enabled {
		interval := time.Duration(site.RunEveryMinutes) * time.Minute
		if interval <= 0 {
			interval = time.Minute
		}
		payload := struct {
			SiteID   string `json:"site_id"`
			SourceID string `json:"source_id,omitempty"`
		}{SiteID: site.ID, SourceID: site.SourceID}
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		return s.adm.UpsertByTaskName(ctx, domain.UpsertTaskParams{TaskName: name, Payload: b, Interval: interval})
	}
	_, err := s.adm.DeleteByTaskName(ctx, name)
	return err
}

func taskNameForSite(id string) string { return "site:" + id }

func normalizeSiteListOptions(opts model.SitesListOptions) model.SitesListOptions {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	if opts.Sort == "" {
		opts.Sort = "created_at"
	}
	if opts.Dir == "" {
		opts.Dir = "desc"
	}

	return opts
}
