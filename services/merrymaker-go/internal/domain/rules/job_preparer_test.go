package rules_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/domain/model"
	domainrules "github.com/target/mmk-ui-api/internal/domain/rules"
	"github.com/target/mmk-ui-api/internal/domain/rules/rulestest"
)

func TestJobPreparationService_ResolveReturnsSiteAlertMode(t *testing.T) {
	t.Helper()

	mode := model.SiteAlertModeMuted
	service := domainrules.NewJobPreparationService(domainrules.JobPreparationOptions{
		Sites: &rulestest.SiteRepositoryStub{
			GetByIDFn: func(ctx context.Context, id string) (*model.Site, error) {
				return &model.Site{AlertMode: mode}, nil
			},
		},
	})

	result := service.Resolve(context.Background(), domainrules.AlertResolutionParams{
		SiteID: "site-123",
		JobID:  "job-1",
	})
	assert.Equal(t, mode, result, "expected site alert mode to be returned")
}

func TestJobPreparationService_ResolveFallsBackOnError(t *testing.T) {
	t.Helper()

	service := domainrules.NewJobPreparationService(domainrules.JobPreparationOptions{
		Sites: &rulestest.SiteRepositoryStub{
			GetByIDFn: func(ctx context.Context, id string) (*model.Site, error) {
				return nil, errors.New("boom")
			},
		},
	})

	result := service.Resolve(context.Background(), domainrules.AlertResolutionParams{
		SiteID: "site-123",
		JobID:  "job-1",
	})
	assert.Equal(t, model.SiteAlertModeActive, result)
}

func TestJobPreparationService_FetchReturnsEvents(t *testing.T) {
	t.Helper()

	expected := []*model.Event{{ID: "evt-1"}}
	service := domainrules.NewJobPreparationService(domainrules.JobPreparationOptions{
		Events: &rulestest.EventRepositoryStub{
			GetByIDsFn: func(ctx context.Context, ids []string) ([]*model.Event, error) {
				require.Equal(t, []string{"evt-1"}, ids)
				return expected, nil
			},
		},
	})

	events, err := service.Fetch(context.Background(), domainrules.EventFetchParams{
		EventIDs: []string{"evt-1"},
		JobID:    "job-1",
	})
	require.NoError(t, err)
	assert.Equal(t, expected, events)
}

func TestJobPreparationService_FetchWrapsError(t *testing.T) {
	t.Helper()

	service := domainrules.NewJobPreparationService(domainrules.JobPreparationOptions{
		Events: &rulestest.EventRepositoryStub{
			GetByIDsFn: func(ctx context.Context, ids []string) ([]*model.Event, error) {
				return nil, errors.New("db down")
			},
		},
	})

	events, err := service.Fetch(context.Background(), domainrules.EventFetchParams{
		EventIDs: []string{"evt-1"},
		JobID:    "job-1",
	})
	require.Error(t, err)
	assert.Nil(t, events)
	assert.EqualError(t, err, "fetch events: db down")
}

func TestJobPreparationService_FetchSkipsWhenNoIDs(t *testing.T) {
	t.Helper()

	called := false
	service := domainrules.NewJobPreparationService(domainrules.JobPreparationOptions{
		Events: &rulestest.EventRepositoryStub{
			GetByIDsFn: func(ctx context.Context, ids []string) ([]*model.Event, error) {
				called = true
				return nil, nil
			},
		},
	})

	events, err := service.Fetch(context.Background(), domainrules.EventFetchParams{
		EventIDs: nil,
		JobID:    "job-1",
	})
	require.NoError(t, err)
	assert.Nil(t, events)
	assert.False(t, called, "expected repository to be skipped when no event IDs provided")
}
