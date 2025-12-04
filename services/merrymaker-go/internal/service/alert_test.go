package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// mockAlertRepo is a mock implementation of core.AlertRepository for testing.
type mockAlertRepo struct {
	createFunc               func(ctx context.Context, req *model.CreateAlertRequest) (*model.Alert, error)
	getByIDFunc              func(ctx context.Context, id string) (*model.Alert, error)
	listFunc                 func(ctx context.Context, opts *model.AlertListOptions) ([]*model.Alert, error)
	listWithSiteNamesFunc    func(ctx context.Context, opts *model.AlertListOptions) ([]*model.AlertWithSiteName, error)
	deleteFunc               func(ctx context.Context, id string) (bool, error)
	statsFunc                func(ctx context.Context, siteID *string) (*model.AlertStats, error)
	resolveFunc              func(ctx context.Context, params core.ResolveAlertParams) (*model.Alert, error)
	updateDeliveryStatusFunc func(ctx context.Context, params core.UpdateAlertDeliveryStatusParams) (*model.Alert, error)
}

func (m *mockAlertRepo) Create(ctx context.Context, req *model.CreateAlertRequest) (*model.Alert, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, req)
	}
	return nil, errors.New("not implemented")
}

func (m *mockAlertRepo) GetByID(ctx context.Context, id string) (*model.Alert, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockAlertRepo) List(ctx context.Context, opts *model.AlertListOptions) ([]*model.Alert, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, opts)
	}
	return nil, errors.New("not implemented")
}

func (m *mockAlertRepo) ListWithSiteNames(
	ctx context.Context,
	opts *model.AlertListOptions,
) ([]*model.AlertWithSiteName, error) {
	if m.listWithSiteNamesFunc != nil {
		return m.listWithSiteNamesFunc(ctx, opts)
	}
	return nil, errors.New("not implemented")
}

func (m *mockAlertRepo) Delete(ctx context.Context, id string) (bool, error) {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return false, errors.New("not implemented")
}

func (m *mockAlertRepo) Stats(ctx context.Context, siteID *string) (*model.AlertStats, error) {
	if m.statsFunc != nil {
		return m.statsFunc(ctx, siteID)
	}
	return nil, errors.New("not implemented")
}

func (m *mockAlertRepo) Resolve(ctx context.Context, params core.ResolveAlertParams) (*model.Alert, error) {
	if m.resolveFunc != nil {
		return m.resolveFunc(ctx, params)
	}
	return nil, errors.New("not implemented")
}

func (m *mockAlertRepo) UpdateDeliveryStatus(
	ctx context.Context,
	params core.UpdateAlertDeliveryStatusParams,
) (*model.Alert, error) {
	if m.updateDeliveryStatusFunc != nil {
		return m.updateDeliveryStatusFunc(ctx, params)
	}
	return nil, errors.New("not implemented")
}

type mockSiteRepo struct {
	getByIDFunc func(ctx context.Context, id string) (*model.Site, error)
}

func (m *mockSiteRepo) Create(ctx context.Context, req *model.CreateSiteRequest) (*model.Site, error) {
	return nil, errors.New("not implemented")
}

func (m *mockSiteRepo) GetByID(ctx context.Context, id string) (*model.Site, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockSiteRepo) GetByName(ctx context.Context, name string) (*model.Site, error) {
	return nil, errors.New("not implemented")
}

func (m *mockSiteRepo) List(ctx context.Context, limit, offset int) ([]*model.Site, error) {
	return nil, errors.New("not implemented")
}

func (m *mockSiteRepo) Update(ctx context.Context, id string, req model.UpdateSiteRequest) (*model.Site, error) {
	return nil, errors.New("not implemented")
}

func (m *mockSiteRepo) Delete(ctx context.Context, id string) (bool, error) {
	return false, errors.New("not implemented")
}

// mockAlertDispatcher is a mock implementation of AlertDispatcher for testing.
type mockAlertDispatcher struct {
	dispatchFunc func(ctx context.Context, alert *model.Alert) error
	dispatched   []*model.Alert  // Track dispatched alerts
	mu           sync.Mutex      // Protect dispatched slice
	wg           *sync.WaitGroup // Optional: signal when dispatch completes
}

func (m *mockAlertDispatcher) Dispatch(ctx context.Context, alert *model.Alert) error {
	m.mu.Lock()
	m.dispatched = append(m.dispatched, alert)
	m.mu.Unlock()

	if m.wg != nil {
		defer m.wg.Done()
	}

	if m.dispatchFunc != nil {
		return m.dispatchFunc(ctx, alert)
	}
	return nil
}

func TestNewAlertService_RequiresRepo(t *testing.T) {
	svc, err := NewAlertService(AlertServiceOptions{
		Repo: nil,
	})
	require.Error(t, err)
	assert.Nil(t, svc)
	assert.Contains(t, err.Error(), "AlertRepository is required")
}

func TestMustNewAlertService_PanicsOnError(t *testing.T) {
	assert.Panics(t, func() {
		MustNewAlertService(AlertServiceOptions{
			Repo: nil,
		})
	}, "should panic when Repo is nil")
}

func TestNewAlertService_Success(t *testing.T) {
	repo := &mockAlertRepo{}
	dispatcher := &mockAlertDispatcher{}
	logger := slog.Default()

	svc, err := NewAlertService(AlertServiceOptions{
		Repo:       repo,
		Dispatcher: dispatcher,
		Logger:     logger,
	})

	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.Equal(t, repo, svc.repo)
	assert.Equal(t, dispatcher, svc.dispatcher)
	assert.Equal(t, logger, svc.logger)
}

func TestAlertService_Create(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("success without dispatcher", func(t *testing.T) {
		expectedAlert := &model.Alert{
			ID:             "alert-1",
			SiteID:         "site-1",
			RuleType:       "unknown_domain",
			Severity:       "high",
			Title:          "Test Alert",
			Description:    "Test description",
			FiredAt:        now,
			DeliveryStatus: model.AlertDeliveryStatusPending,
		}

		repo := &mockAlertRepo{
			createFunc: func(ctx context.Context, req *model.CreateAlertRequest) (*model.Alert, error) {
				assert.Equal(t, "site-1", req.SiteID)
				assert.Equal(t, "unknown_domain", req.RuleType)
				assert.Equal(t, model.AlertDeliveryStatusPending, req.DeliveryStatus)
				return expectedAlert, nil
			},
		}

		svc := MustNewAlertService(AlertServiceOptions{Repo: repo})

		alert, err := svc.Create(ctx, &model.CreateAlertRequest{
			SiteID:      "site-1",
			RuleType:    "unknown_domain",
			Severity:    "high",
			Title:       "Test Alert",
			Description: "Test description",
		})

		require.NoError(t, err)
		assert.Equal(t, expectedAlert, alert)
		assert.Equal(t, model.AlertDeliveryStatusPending, alert.DeliveryStatus)
	})

	t.Run("success with dispatcher", func(t *testing.T) {
		expectedAlert := &model.Alert{
			ID:             "alert-2",
			SiteID:         "site-2",
			RuleType:       "ioc_domain",
			Severity:       "critical",
			Title:          "IOC Alert",
			Description:    "IOC detected",
			FiredAt:        now,
			DeliveryStatus: model.AlertDeliveryStatusPending,
		}

		repo := &mockAlertRepo{
			createFunc: func(ctx context.Context, req *model.CreateAlertRequest) (*model.Alert, error) {
				assert.Equal(t, model.AlertDeliveryStatusPending, req.DeliveryStatus)
				return expectedAlert, nil
			},
		}

		var wg sync.WaitGroup
		wg.Add(1)
		dispatcher := &mockAlertDispatcher{wg: &wg}

		svc := MustNewAlertService(AlertServiceOptions{
			Repo:       repo,
			Dispatcher: dispatcher,
		})

		alert, err := svc.Create(ctx, &model.CreateAlertRequest{
			SiteID:      "site-2",
			RuleType:    "ioc_domain",
			Severity:    "critical",
			Title:       "IOC Alert",
			Description: "IOC detected",
		})

		require.NoError(t, err)
		assert.Equal(t, expectedAlert, alert)
		assert.Equal(t, model.AlertDeliveryStatusPending, alert.DeliveryStatus)

		// Wait for dispatch to complete
		wg.Wait()
		dispatcher.mu.Lock()
		assert.Len(t, dispatcher.dispatched, 1)
		assert.Equal(t, expectedAlert, dispatcher.dispatched[0])
		dispatcher.mu.Unlock()
	})

	t.Run("muted site skips dispatch", func(t *testing.T) {
		expectedAlert := &model.Alert{ID: "alert-muted", SiteID: "site-muted"}

		repo := &mockAlertRepo{
			createFunc: func(ctx context.Context, req *model.CreateAlertRequest) (*model.Alert, error) {
				assert.Equal(t, model.AlertDeliveryStatusMuted, req.DeliveryStatus)
				return expectedAlert, nil
			},
		}

		sites := &mockSiteRepo{
			getByIDFunc: func(ctx context.Context, id string) (*model.Site, error) {
				return &model.Site{ID: id, AlertMode: model.SiteAlertModeMuted}, nil
			},
		}

		dispatcher := &mockAlertDispatcher{}

		svc := MustNewAlertService(AlertServiceOptions{
			Repo:       repo,
			Sites:      sites,
			Dispatcher: dispatcher,
			Logger:     slog.Default(),
		})

		alert, err := svc.Create(ctx, &model.CreateAlertRequest{
			SiteID:   "site-muted",
			RuleType: "unknown_domain",
		})

		require.NoError(t, err)
		assert.Equal(t, model.AlertDeliveryStatusMuted, alert.DeliveryStatus)

		dispatcher.mu.Lock()
		assert.Empty(t, dispatcher.dispatched)
		dispatcher.mu.Unlock()
	})

	t.Run("repository error", func(t *testing.T) {
		repo := &mockAlertRepo{
			createFunc: func(ctx context.Context, req *model.CreateAlertRequest) (*model.Alert, error) {
				assert.Equal(t, model.AlertDeliveryStatusPending, req.DeliveryStatus)
				return nil, errors.New("database error")
			},
		}

		svc := MustNewAlertService(AlertServiceOptions{Repo: repo})

		alert, err := svc.Create(ctx, &model.CreateAlertRequest{
			SiteID:   "site-1",
			RuleType: "unknown_domain",
		})

		require.Error(t, err)
		assert.Nil(t, alert)
		assert.Contains(t, err.Error(), "create alert")
	})

	t.Run("dispatcher error does not fail create", func(t *testing.T) {
		expectedAlert := &model.Alert{
			ID:             "alert-3",
			SiteID:         "site-3",
			DeliveryStatus: model.AlertDeliveryStatusPending,
		}

		repo := &mockAlertRepo{
			createFunc: func(ctx context.Context, req *model.CreateAlertRequest) (*model.Alert, error) {
				assert.Equal(t, model.AlertDeliveryStatusPending, req.DeliveryStatus)
				return expectedAlert, nil
			},
		}

		var wg sync.WaitGroup
		wg.Add(1)
		dispatcher := &mockAlertDispatcher{
			wg: &wg,
			dispatchFunc: func(ctx context.Context, alert *model.Alert) error {
				return errors.New("dispatch failed")
			},
		}

		svc := MustNewAlertService(AlertServiceOptions{
			Repo:       repo,
			Dispatcher: dispatcher,
			Logger:     slog.Default(),
		})

		alert, err := svc.Create(ctx, &model.CreateAlertRequest{
			SiteID:   "site-3",
			RuleType: "unknown_domain",
		})

		// Create should succeed even if dispatch fails
		require.NoError(t, err)
		assert.Equal(t, expectedAlert, alert)
		assert.Equal(t, model.AlertDeliveryStatusPending, alert.DeliveryStatus)

		// Wait for dispatch to complete
		wg.Wait()
		dispatcher.mu.Lock()
		assert.Len(t, dispatcher.dispatched, 1)
		dispatcher.mu.Unlock()
	})
}

func TestAlertService_GetByID(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		expectedAlert := &model.Alert{
			ID:     "alert-1",
			SiteID: "site-1",
		}

		repo := &mockAlertRepo{
			getByIDFunc: func(ctx context.Context, id string) (*model.Alert, error) {
				assert.Equal(t, "alert-1", id)
				return expectedAlert, nil
			},
		}

		svc := MustNewAlertService(AlertServiceOptions{Repo: repo})
		alert, err := svc.GetByID(ctx, "alert-1")

		require.NoError(t, err)
		assert.Equal(t, expectedAlert, alert)
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mockAlertRepo{
			getByIDFunc: func(ctx context.Context, id string) (*model.Alert, error) {
				return nil, errors.New("not found")
			},
		}

		svc := MustNewAlertService(AlertServiceOptions{Repo: repo})
		alert, err := svc.GetByID(ctx, "nonexistent")

		require.Error(t, err)
		assert.Nil(t, alert)
	})
}

func TestAlertService_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		repo := &mockAlertRepo{
			deleteFunc: func(ctx context.Context, id string) (bool, error) {
				assert.Equal(t, "alert-1", id)
				return true, nil
			},
		}

		svc := MustNewAlertService(AlertServiceOptions{Repo: repo})
		deleted, err := svc.Delete(ctx, "alert-1")

		require.NoError(t, err)
		assert.True(t, deleted)
	})

	t.Run("not found", func(t *testing.T) {
		repo := &mockAlertRepo{
			deleteFunc: func(ctx context.Context, id string) (bool, error) {
				return false, nil
			},
		}

		svc := MustNewAlertService(AlertServiceOptions{Repo: repo})
		deleted, err := svc.Delete(ctx, "nonexistent")

		require.NoError(t, err)
		assert.False(t, deleted)
	})
}
