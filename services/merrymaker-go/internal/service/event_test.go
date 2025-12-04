package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDefaultEventServiceConfig(t *testing.T) {
	config := DefaultEventServiceConfig()
	assert.Equal(t, 1000, config.MaxBatch)
	assert.InDelta(t, 0.7, config.ThreatScoreProcessCutoff, 0.001)
}

func TestNewEventService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockEventRepository(ctrl)

	t.Run("success", func(t *testing.T) {
		svc, err := NewEventService(EventServiceOptions{
			Repo:   repo,
			Config: DefaultEventServiceConfig(),
		})
		require.NoError(t, err)
		assert.NotNil(t, svc)
		assert.Equal(t, repo, svc.repo)
		assert.Equal(t, 1000, svc.config.MaxBatch)
		assert.InDelta(t, 0.7, svc.config.ThreatScoreProcessCutoff, 0.001)
	})

	t.Run("success with logger", func(t *testing.T) {
		logger := slog.Default()
		svc, err := NewEventService(EventServiceOptions{
			Repo:   repo,
			Config: DefaultEventServiceConfig(),
			Logger: logger,
		})
		require.NoError(t, err)
		assert.NotNil(t, svc)
		assert.NotNil(t, svc.logger)
	})

	t.Run("missing repo", func(t *testing.T) {
		svc, err := NewEventService(EventServiceOptions{
			Config: DefaultEventServiceConfig(),
		})
		require.Error(t, err)
		assert.Nil(t, svc)
		assert.Contains(t, err.Error(), "EventRepository is required")
	})

	t.Run("invalid max batch", func(t *testing.T) {
		config := DefaultEventServiceConfig()
		config.MaxBatch = 0
		svc, err := NewEventService(EventServiceOptions{
			Repo:   repo,
			Config: config,
		})
		require.Error(t, err)
		assert.Nil(t, svc)
		assert.Contains(t, err.Error(), "MaxBatch must be positive")
	})

	t.Run("invalid threat score cutoff - negative", func(t *testing.T) {
		config := DefaultEventServiceConfig()
		config.ThreatScoreProcessCutoff = -0.1
		svc, err := NewEventService(EventServiceOptions{
			Repo:   repo,
			Config: config,
		})
		require.Error(t, err)
		assert.Nil(t, svc)
		assert.Contains(t, err.Error(), "ThreatScoreProcessCutoff must be between 0 and 1")
	})

	t.Run("invalid threat score cutoff - too high", func(t *testing.T) {
		config := DefaultEventServiceConfig()
		config.ThreatScoreProcessCutoff = 1.1
		svc, err := NewEventService(EventServiceOptions{
			Repo:   repo,
			Config: config,
		})
		require.Error(t, err)
		assert.Nil(t, svc)
		assert.Contains(t, err.Error(), "ThreatScoreProcessCutoff must be between 0 and 1")
	})
}

func TestMustNewEventService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockEventRepository(ctrl)

	t.Run("success", func(t *testing.T) {
		svc := MustNewEventService(EventServiceOptions{
			Repo:   repo,
			Config: DefaultEventServiceConfig(),
		})
		assert.NotNil(t, svc)
	})

	t.Run("panic on error", func(t *testing.T) {
		assert.Panics(t, func() {
			MustNewEventService(EventServiceOptions{
				Config: DefaultEventServiceConfig(),
				// Missing repo
			})
		})
	})
}

func TestEventService_BulkInsert(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockEventRepository(ctrl)
	svc := MustNewEventService(EventServiceOptions{
		Repo:   repo,
		Config: DefaultEventServiceConfig(),
	})

	req := model.BulkEventRequest{
		SessionID: "session-123",
		Events: []model.RawEvent{
			{Type: "page_load", Data: json.RawMessage(`"test1"`), Timestamp: time.Now()},
			{Type: "click", Data: json.RawMessage(`"test2"`), Timestamp: time.Now()},
		},
	}

	repo.EXPECT().BulkInsert(gomock.Any(), req, true).Return(2, nil)

	count, err := svc.BulkInsert(context.Background(), req, true)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestEventService_BulkInsertWithProcessingFlags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockEventRepository(ctrl)
	svc := MustNewEventService(EventServiceOptions{
		Repo:   repo,
		Config: DefaultEventServiceConfig(),
	})

	req := model.BulkEventRequest{
		SessionID: "session-123",
		Events: []model.RawEvent{
			{Type: "page_load", Data: json.RawMessage(`"test1"`), Timestamp: time.Now()},
			{Type: "click", Data: json.RawMessage(`"test2"`), Timestamp: time.Now()},
		},
	}
	shouldProcessMap := map[int]bool{0: true, 1: false}

	repo.EXPECT().BulkInsertWithProcessingFlags(gomock.Any(), req, shouldProcessMap).Return(2, nil)

	count, err := svc.BulkInsertWithProcessingFlags(context.Background(), req, shouldProcessMap)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestEventService_ListByJob(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockEventRepository(ctrl)
	svc := MustNewEventService(EventServiceOptions{
		Repo:   repo,
		Config: DefaultEventServiceConfig(),
	})

	expectedPage := &model.EventListPage{
		Events: []*model.Event{
			{ID: "event-1", EventType: "page_load"},
			{ID: "event-2", EventType: "click"},
		},
	}
	nextCursor := "next-123"
	prevCursor := "prev-123"
	expectedCursorPage := &model.EventListPage{
		Events:     expectedPage.Events,
		NextCursor: &nextCursor,
		PrevCursor: &prevCursor,
	}

	t.Run("with valid pagination", func(t *testing.T) {
		opts := model.EventListByJobOptions{
			JobID:  "job-123",
			Limit:  10,
			Offset: 0,
		}

		repo.EXPECT().ListByJob(gomock.Any(), opts).Return(expectedPage, nil)

		page, err := svc.ListByJob(context.Background(), opts)
		require.NoError(t, err)
		assert.Equal(t, expectedPage, page)
	})

	t.Run("pagination normalization - default limit", func(t *testing.T) {
		opts := model.EventListByJobOptions{
			JobID:  "job-123",
			Limit:  0, // Should be normalized to 50
			Offset: 0,
		}

		expectedOpts := opts
		expectedOpts.Limit = 50

		repo.EXPECT().ListByJob(gomock.Any(), expectedOpts).Return(expectedPage, nil)

		page, err := svc.ListByJob(context.Background(), opts)
		require.NoError(t, err)
		assert.Equal(t, expectedPage, page)
	})

	t.Run("pagination normalization - max limit", func(t *testing.T) {
		opts := model.EventListByJobOptions{
			JobID:  "job-123",
			Limit:  2000, // Should be clamped to 1000
			Offset: 0,
		}

		expectedOpts := opts
		expectedOpts.Limit = 1000

		repo.EXPECT().ListByJob(gomock.Any(), expectedOpts).Return(expectedPage, nil)

		page, err := svc.ListByJob(context.Background(), opts)
		require.NoError(t, err)
		assert.Equal(t, expectedPage, page)
	})

	t.Run("pagination normalization - negative offset", func(t *testing.T) {
		opts := model.EventListByJobOptions{
			JobID:  "job-123",
			Limit:  10,
			Offset: -5, // Should be normalized to 0
		}

		expectedOpts := opts
		expectedOpts.Offset = 0

		repo.EXPECT().ListByJob(gomock.Any(), expectedOpts).Return(expectedPage, nil)

		page, err := svc.ListByJob(context.Background(), opts)
		require.NoError(t, err)
		assert.Equal(t, expectedPage, page)
	})

	t.Run("keyset pagination prefers cursor and normalizes limits", func(t *testing.T) {
		cursor := "cursor-abc"
		opts := model.EventListByJobOptions{
			JobID:       "job-123",
			Limit:       -5,
			Offset:      10,
			CursorAfter: &cursor,
		}

		expectedOpts := opts
		expectedOpts.Limit = 50
		expectedOpts.Offset = 0

		repo.EXPECT().ListByJob(gomock.Any(), expectedOpts).Return(expectedCursorPage, nil)

		page, err := svc.ListByJob(context.Background(), opts)
		require.NoError(t, err)
		assert.Equal(t, expectedCursorPage, page)
	})
}

func TestEventService_ListWithFilters(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockEventRepository(ctrl)
	svc := MustNewEventService(EventServiceOptions{
		Repo:   repo,
		Config: DefaultEventServiceConfig(),
	})

	expectedPage := &model.EventListPage{
		Events: []*model.Event{
			{ID: "event-1", EventType: "page_load"},
			{ID: "event-2", EventType: "click"},
		},
	}

	t.Run("delegates to ListByJob with filters", func(t *testing.T) {
		// ListWithFilters is now a deprecated wrapper that delegates to ListByJob
		eventType := "page_load"
		opts := model.EventListByJobOptions{
			JobID:     "job-123",
			EventType: &eventType,
			Limit:     10,
			Offset:    0,
		}

		// Should call ListByJob with the same options (including filters)
		repo.EXPECT().ListByJob(gomock.Any(), opts).Return(expectedPage, nil)

		page, err := svc.ListWithFilters(context.Background(), opts)
		require.NoError(t, err)
		assert.Equal(t, expectedPage, page)
	})
}

func TestEventService_GetConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockEventRepository(ctrl)
	config := EventServiceConfig{
		MaxBatch:                 500,
		ThreatScoreProcessCutoff: 0.8,
	}
	svc := MustNewEventService(EventServiceOptions{
		Repo:   repo,
		Config: config,
	})

	returnedConfig := svc.GetConfig()
	assert.Equal(t, config, returnedConfig)
}
