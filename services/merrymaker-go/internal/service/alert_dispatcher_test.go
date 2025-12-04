package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockHTTPAlertSinkRepository struct {
	getByIDFunc func(ctx context.Context, id string) (*model.HTTPAlertSink, error)
}

func (m *mockHTTPAlertSinkRepository) Create(
	ctx context.Context,
	req *model.CreateHTTPAlertSinkRequest,
) (*model.HTTPAlertSink, error) {
	return nil, errors.New("not implemented")
}

func (m *mockHTTPAlertSinkRepository) GetByID(ctx context.Context, id string) (*model.HTTPAlertSink, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockHTTPAlertSinkRepository) GetByName(ctx context.Context, name string) (*model.HTTPAlertSink, error) {
	return nil, errors.New("not implemented")
}

func (m *mockHTTPAlertSinkRepository) List(ctx context.Context, limit, offset int) ([]*model.HTTPAlertSink, error) {
	return nil, errors.New("not implemented")
}

func (m *mockHTTPAlertSinkRepository) Update(
	ctx context.Context,
	id string,
	req *model.UpdateHTTPAlertSinkRequest,
) (*model.HTTPAlertSink, error) {
	return nil, errors.New("not implemented")
}

func (m *mockHTTPAlertSinkRepository) Delete(ctx context.Context, id string) (bool, error) {
	return false, errors.New("not implemented")
}

type mockSiteRepository struct {
	getByIDFunc func(ctx context.Context, id string) (*model.Site, error)
}

func (m *mockSiteRepository) Create(ctx context.Context, req *model.CreateSiteRequest) (*model.Site, error) {
	return nil, errors.New("not implemented")
}

func (m *mockSiteRepository) GetByID(ctx context.Context, id string) (*model.Site, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockSiteRepository) GetByName(ctx context.Context, name string) (*model.Site, error) {
	return nil, errors.New("not implemented")
}

func (m *mockSiteRepository) List(ctx context.Context, limit, offset int) ([]*model.Site, error) {
	return nil, errors.New("not implemented")
}

func (m *mockSiteRepository) Update(ctx context.Context, id string, req model.UpdateSiteRequest) (*model.Site, error) {
	return nil, errors.New("not implemented")
}

func (m *mockSiteRepository) Delete(ctx context.Context, id string) (bool, error) {
	return false, errors.New("not implemented")
}

type mockAlertSinkService struct {
	scheduleAlertFunc func(ctx context.Context, sink *model.HTTPAlertSink, eventPayload json.RawMessage) (*model.Job, error)
}

func (m *mockAlertSinkService) ScheduleAlert(
	ctx context.Context,
	sink *model.HTTPAlertSink,
	eventPayload json.RawMessage,
) (*model.Job, error) {
	if m.scheduleAlertFunc != nil {
		return m.scheduleAlertFunc(ctx, sink, eventPayload)
	}
	return nil, errors.New("not implemented")
}

func TestAlertDispatchService_Dispatch_Success(t *testing.T) {
	alert := &model.Alert{
		ID:     "alert-1",
		SiteID: "site-1",
		Title:  "Unknown domain observed",
	}

	sink := &model.HTTPAlertSink{
		ID:     "sink-1",
		Name:   "test-sink",
		Method: "POST",
		URI:    "https://example.test",
	}

	siteRepo := &mockSiteRepository{
		getByIDFunc: func(ctx context.Context, id string) (*model.Site, error) {
			require.Equal(t, alert.SiteID, id)
			return &model.Site{
				ID:              alert.SiteID,
				HTTPAlertSinkID: &sink.ID,
			}, nil
		},
	}

	sinkRepo := &mockHTTPAlertSinkRepository{
		getByIDFunc: func(ctx context.Context, id string) (*model.HTTPAlertSink, error) {
			require.Equal(t, sink.ID, id)
			return sink, nil
		},
	}

	alertSinkSvc := &mockAlertSinkService{
		scheduleAlertFunc: func(ctx context.Context, s *model.HTTPAlertSink, payload json.RawMessage) (*model.Job, error) {
			assert.Equal(t, sink.ID, s.ID)
			var decoded model.Alert
			require.NoError(t, json.Unmarshal(payload, &decoded))
			assert.Equal(t, alert.ID, decoded.ID)
			return &model.Job{ID: "job-1"}, nil
		},
	}

	dispatcher := NewAlertDispatchService(AlertDispatchServiceOptions{
		Sites:     siteRepo,
		Sinks:     sinkRepo,
		AlertSink: alertSinkSvc,
		Logger:    slog.Default(),
	})

	err := dispatcher.Dispatch(context.Background(), alert)
	require.NoError(t, err)
}

func TestAlertDispatchService_Dispatch_NoConfiguredSink(t *testing.T) {
	alert := &model.Alert{
		ID:     "alert-1",
		SiteID: "site-1",
	}

	siteRepo := &mockSiteRepository{
		getByIDFunc: func(ctx context.Context, id string) (*model.Site, error) {
			return &model.Site{ID: id}, nil
		},
	}

	sinkRepo := &mockHTTPAlertSinkRepository{}
	alertSinkSvc := &mockAlertSinkService{
		scheduleAlertFunc: func(ctx context.Context, sink *model.HTTPAlertSink, payload json.RawMessage) (*model.Job, error) {
			return nil, errors.New("should not be called")
		},
	}

	dispatcher := NewAlertDispatchService(AlertDispatchServiceOptions{
		Sites:     siteRepo,
		Sinks:     sinkRepo,
		AlertSink: alertSinkSvc,
		Logger:    slog.Default(),
	})

	err := dispatcher.Dispatch(context.Background(), alert)
	require.NoError(t, err)
}

func TestAlertDispatchService_Dispatch_MutedSiteSkips(t *testing.T) {
	alert := &model.Alert{ID: "alert-muted", SiteID: "site-muted"}

	sink := &model.HTTPAlertSink{ID: "sink-1", Name: "Muted Sink"}

	siteRepo := &mockSiteRepository{
		getByIDFunc: func(ctx context.Context, id string) (*model.Site, error) {
			return &model.Site{
				ID:              id,
				AlertMode:       model.SiteAlertModeMuted,
				HTTPAlertSinkID: &sink.ID,
			}, nil
		},
	}

	sinkRepo := &mockHTTPAlertSinkRepository{
		getByIDFunc: func(ctx context.Context, id string) (*model.HTTPAlertSink, error) {
			return sink, nil
		},
	}

	alertSinkSvc := &mockAlertSinkService{
		scheduleAlertFunc: func(ctx context.Context, sink *model.HTTPAlertSink, payload json.RawMessage) (*model.Job, error) {
			return nil, errors.New("should not dispatch when muted")
		},
	}

	dispatcher := NewAlertDispatchService(AlertDispatchServiceOptions{
		Sites:     siteRepo,
		Sinks:     sinkRepo,
		AlertSink: alertSinkSvc,
		Logger:    slog.Default(),
	})

	err := dispatcher.Dispatch(context.Background(), alert)
	require.NoError(t, err)
}

func TestAlertDispatchService_Dispatch_SiteLookupError(t *testing.T) {
	alert := &model.Alert{
		ID:     "alert-1",
		SiteID: "missing-site",
	}

	expectedErr := errors.New("boom")
	siteRepo := &mockSiteRepository{
		getByIDFunc: func(ctx context.Context, id string) (*model.Site, error) {
			return nil, expectedErr
		},
	}

	dispatcher := NewAlertDispatchService(AlertDispatchServiceOptions{
		Sites:     siteRepo,
		Sinks:     &mockHTTPAlertSinkRepository{},
		AlertSink: &mockAlertSinkService{},
		Logger:    slog.Default(),
	})

	err := dispatcher.Dispatch(context.Background(), alert)
	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
}

func TestAlertDispatchService_Dispatch_SinkLookupError(t *testing.T) {
	alert := &model.Alert{
		ID:     "alert-1",
		SiteID: "site-1",
	}

	sinkID := "sink-missing"
	siteRepo := &mockSiteRepository{
		getByIDFunc: func(ctx context.Context, id string) (*model.Site, error) {
			return &model.Site{
				ID:              id,
				HTTPAlertSinkID: &sinkID,
			}, nil
		},
	}

	expectedErr := errors.New("sink not found")
	sinkRepo := &mockHTTPAlertSinkRepository{
		getByIDFunc: func(ctx context.Context, id string) (*model.HTTPAlertSink, error) {
			return nil, expectedErr
		},
	}

	dispatcher := NewAlertDispatchService(AlertDispatchServiceOptions{
		Sites:     siteRepo,
		Sinks:     sinkRepo,
		AlertSink: &mockAlertSinkService{},
		Logger:    slog.Default(),
	})

	err := dispatcher.Dispatch(context.Background(), alert)
	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
}

func TestAlertDispatchService_Dispatch_SchedulerFailure(t *testing.T) {
	alert := &model.Alert{
		ID:     "alert-1",
		SiteID: "site-1",
	}

	sinkID := "sink-1"
	siteRepo := &mockSiteRepository{
		getByIDFunc: func(ctx context.Context, id string) (*model.Site, error) {
			return &model.Site{
				ID:              id,
				HTTPAlertSinkID: &sinkID,
			}, nil
		},
	}

	sink := &model.HTTPAlertSink{ID: sinkID}
	sinkRepo := &mockHTTPAlertSinkRepository{
		getByIDFunc: func(ctx context.Context, id string) (*model.HTTPAlertSink, error) {
			return sink, nil
		},
	}

	expectedErr := errors.New("dispatch failed")
	alertSinkSvc := &mockAlertSinkService{
		scheduleAlertFunc: func(ctx context.Context, s *model.HTTPAlertSink, payload json.RawMessage) (*model.Job, error) {
			return nil, expectedErr
		},
	}

	dispatcher := NewAlertDispatchService(AlertDispatchServiceOptions{
		Sites:     siteRepo,
		Sinks:     sinkRepo,
		AlertSink: alertSinkSvc,
		Logger:    slog.Default(),
	})

	err := dispatcher.Dispatch(context.Background(), alert)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all sink schedules failed")
}

func TestAlertDispatchService_Dispatch_NoSiteRepoConfigured(t *testing.T) {
	alert := &model.Alert{
		ID:     "alert-1",
		SiteID: "site-1",
	}

	dispatcher := NewAlertDispatchService(AlertDispatchServiceOptions{
		Sinks:     &mockHTTPAlertSinkRepository{},
		AlertSink: &mockAlertSinkService{},
		Logger:    slog.Default(),
	})

	err := dispatcher.Dispatch(context.Background(), alert)
	require.Error(t, err)
	assert.ErrorIs(t, err, errSiteRepoNotConfigured)
}

func TestAlertDispatchService_Dispatch_NoSchedulerConfigured(t *testing.T) {
	alert := &model.Alert{
		ID:     "alert-1",
		SiteID: "site-1",
	}

	sinkID := "sink-1"
	siteRepo := &mockSiteRepository{
		getByIDFunc: func(ctx context.Context, id string) (*model.Site, error) {
			return &model.Site{
				ID:              id,
				HTTPAlertSinkID: &sinkID,
			}, nil
		},
	}

	sinkRepo := &mockHTTPAlertSinkRepository{
		getByIDFunc: func(ctx context.Context, id string) (*model.HTTPAlertSink, error) {
			return &model.HTTPAlertSink{ID: id}, nil
		},
	}

	dispatcher := NewAlertDispatchService(AlertDispatchServiceOptions{
		Sites: siteRepo,
		Sinks: sinkRepo,
		// AlertSink intentionally nil to simulate misconfiguration.
		Logger: slog.Default(),
	})

	err := dispatcher.Dispatch(context.Background(), alert)
	require.Error(t, err)
	assert.ErrorIs(t, err, errAlertSinkSchedulerNotConfigured)
}
