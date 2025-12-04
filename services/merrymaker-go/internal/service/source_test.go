package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/mocks"
	"go.uber.org/mock/gomock"
)

const testSourceID = "source-1"

func TestSourceService_Create_CachesSourceContent_WhenTestTrue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockSrc := mocks.NewMockSourceRepository(ctrl)
	mockJobs := mocks.NewMockJobRepository(ctrl)
	mockCache := core.NewMockCacheRepository(ctrl)

	created := &model.Source{ID: testSourceID, Name: "src", Value: "console.log('x')", Test: true}

	// Cache expectations: not cached -> set resolved content
	mockSrc.EXPECT().GetByID(gomock.Any(), gomock.Any()).Times(0)
	mockCache.EXPECT().Get(gomock.Any(), "source:content:"+created.ID).Return(nil, nil)
	mockCache.EXPECT().Set(gomock.Any(), "source:content:"+created.ID, []byte(created.Value), gomock.Any()).Return(nil)

	cacheSvc := core.NewSourceCacheService(core.SourceCacheServiceOptions{
		Cache:   mockCache,
		Sources: mockSrc,
		Secrets: nil,
		Config:  core.DefaultSourceCacheConfig(),
	})
	svc := NewSourceService(SourceServiceOptions{SourceRepo: mockSrc, Jobs: mockJobs, Cache: cacheSvc})

	req := &model.CreateSourceRequest{Name: created.Name, Value: created.Value, Test: true}
	mockSrc.EXPECT().Create(ctx, req).Return(created, nil)
	mockJobs.EXPECT().
		Create(ctx, gomock.AssignableToTypeOf(&model.CreateJobRequest{})).
		Return(&model.Job{ID: "job-1"}, nil)

	got, err := svc.Create(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created, got)
}

func TestSourceService_Create_AutoEnqueue_WhenTestTrue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockSrc := mocks.NewMockSourceRepository(ctrl)
	mockJobs := mocks.NewMockJobRepository(ctrl)
	svc := NewSourceService(SourceServiceOptions{SourceRepo: mockSrc, Jobs: mockJobs})

	req := &model.CreateSourceRequest{Name: "src", Value: "console.log('x')", Test: true}
	created := &model.Source{ID: testSourceID, Name: req.Name, Value: req.Value, Test: true}

	mockSrc.EXPECT().Create(ctx, req).Return(created, nil)
	mockJobs.EXPECT().Create(ctx, gomock.AssignableToTypeOf(&model.CreateJobRequest{})).DoAndReturn(
		func(_ context.Context, jreq *model.CreateJobRequest) (*model.Job, error) {
			assert.Equal(t, model.JobTypeBrowser, jreq.Type)
			assert.True(t, jreq.IsTest)
			if assert.NotNil(t, jreq.SourceID) {
				assert.Equal(t, created.ID, *jreq.SourceID)
			}
			// payload should contain source_id
			var p struct {
				SourceID string `json:"source_id"`
			}
			require.NoError(t, json.Unmarshal(jreq.Payload, &p))
			assert.Equal(t, created.ID, p.SourceID)
			return &model.Job{ID: "job-1"}, nil
		},
	).Times(1)

	got, err := svc.Create(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created, got)
}

func TestSourceService_Create_NoEnqueue_WhenTestFalse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockSrc := mocks.NewMockSourceRepository(ctrl)
	mockJobs := mocks.NewMockJobRepository(ctrl)
	svc := NewSourceService(SourceServiceOptions{SourceRepo: mockSrc, Jobs: mockJobs})

	req := &model.CreateSourceRequest{Name: "src", Value: "console.log('x')", Test: false}
	created := &model.Source{ID: testSourceID, Name: req.Name, Value: req.Value, Test: false}

	mockSrc.EXPECT().Create(ctx, req).Return(created, nil)
	// no job create expected

	got, err := svc.Create(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created, got)
}

func TestSourceService_Create_ResolvesSecretsInTestPayload(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockSrc := mocks.NewMockSourceRepository(ctrl)
	mockJobs := mocks.NewMockJobRepository(ctrl)
	mockSecrets := mocks.NewMockSecretRepository(ctrl)

	req := &model.CreateSourceRequest{
		Name:    "src",
		Value:   "console.log('__API_KEY__')",
		Test:    true,
		Secrets: []string{"API_KEY"},
	}
	created := &model.Source{ID: testSourceID, Name: req.Name, Value: req.Value, Test: true, Secrets: req.Secrets}

	mockSrc.EXPECT().Create(ctx, req).Return(created, nil)
	mockSecrets.EXPECT().GetByName(ctx, "API_KEY").Return(&model.Secret{Name: "API_KEY", Value: "resolved"}, nil)
	mockJobs.EXPECT().Create(ctx, gomock.AssignableToTypeOf(&model.CreateJobRequest{})).DoAndReturn(
		func(_ context.Context, jreq *model.CreateJobRequest) (*model.Job, error) {
			var p struct {
				Script string `json:"script"`
			}
			require.NoError(t, json.Unmarshal(jreq.Payload, &p))
			assert.Equal(t, "console.log('resolved')", p.Script)
			return &model.Job{ID: "job-1"}, nil
		},
	).Times(1)

	svc := NewSourceService(SourceServiceOptions{SourceRepo: mockSrc, Jobs: mockJobs, SecretRepo: mockSecrets})
	_, err := svc.Create(ctx, req)
	require.NoError(t, err)
}

func TestSourceService_Create_CachesResolvedSecrets(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockSrc := mocks.NewMockSourceRepository(ctrl)
	mockJobs := mocks.NewMockJobRepository(ctrl)
	mockCache := core.NewMockCacheRepository(ctrl)
	mockSecrets := mocks.NewMockSecretRepository(ctrl)

	created := &model.Source{
		ID:      testSourceID,
		Name:    "src",
		Value:   "console.log('__API_KEY__')",
		Test:    true,
		Secrets: []string{"API_KEY"},
	}
	req := &model.CreateSourceRequest{
		Name:    created.Name,
		Value:   created.Value,
		Test:    created.Test,
		Secrets: created.Secrets,
	}

	key := "source:content:" + created.ID
	mockSrc.EXPECT().GetByID(gomock.Any(), gomock.Any()).Times(0)
	mockCache.EXPECT().Get(gomock.Any(), key).Return(nil, nil)
	mockSecrets.EXPECT().
		GetByName(ctx, "API_KEY").
		Return(&model.Secret{Name: "API_KEY", Value: "resolved"}, nil).
		Times(2)
	mockCache.EXPECT().Set(gomock.Any(), key, []byte("console.log('resolved')"), gomock.Any()).Return(nil)

	cacheSvc := core.NewSourceCacheService(core.SourceCacheServiceOptions{
		Cache:   mockCache,
		Sources: mockSrc,
		Secrets: mockSecrets,
		Config:  core.DefaultSourceCacheConfig(),
	})
	svc := NewSourceService(
		SourceServiceOptions{SourceRepo: mockSrc, Jobs: mockJobs, Cache: cacheSvc, SecretRepo: mockSecrets},
	)

	mockSrc.EXPECT().Create(ctx, req).Return(created, nil)
	mockJobs.EXPECT().Create(ctx, gomock.AssignableToTypeOf(&model.CreateJobRequest{})).DoAndReturn(
		func(_ context.Context, jreq *model.CreateJobRequest) (*model.Job, error) {
			var p struct {
				Script string `json:"script"`
			}
			require.NoError(t, json.Unmarshal(jreq.Payload, &p))
			assert.Equal(t, "console.log('resolved')", p.Script)
			return &model.Job{ID: "job-1"}, nil
		},
	).Times(1)

	_, err := svc.Create(ctx, req)
	require.NoError(t, err)
}

func TestSourceService_ResolveScript(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockSecrets := mocks.NewMockSecretRepository(ctrl)
	svc := NewSourceService(SourceServiceOptions{SecretRepo: mockSecrets})

	res, err := svc.ResolveScript(ctx, &model.Source{Value: "console.log('hi')"})
	require.NoError(t, err)
	assert.Equal(t, "console.log('hi')", res)

	mockSecrets.EXPECT().GetByName(ctx, "TOKEN").Return(&model.Secret{Name: "TOKEN", Value: "abc"}, nil)
	res, err = svc.ResolveScript(ctx, &model.Source{Value: "const t = '__TOKEN__'", Secrets: []string{"TOKEN"}})
	require.NoError(t, err)
	assert.Equal(t, "const t = 'abc'", res)

	svcNoSecrets := NewSourceService(SourceServiceOptions{})
	_, err = svcNoSecrets.ResolveScript(ctx, &model.Source{Value: "", Secrets: []string{"TOKEN"}})
	assert.Error(t, err)
}

func TestSourceService_Update_AutoEnqueue_WhenTestTrue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockSrc := mocks.NewMockSourceRepository(ctrl)
	mockJobs := mocks.NewMockJobRepository(ctrl)
	svc := NewSourceService(SourceServiceOptions{SourceRepo: mockSrc, Jobs: mockJobs})

	id := testSourceID
	v := "new value"
	trueVal := true
	req := model.UpdateSourceRequest{Value: &v, Test: &trueVal}
	updated := &model.Source{ID: id, Name: "src", Value: v, Test: true}

	mockSrc.EXPECT().Update(ctx, id, req).Return(updated, nil)
	mockJobs.EXPECT().Create(ctx, gomock.AssignableToTypeOf(&model.CreateJobRequest{})).DoAndReturn(
		func(_ context.Context, jreq *model.CreateJobRequest) (*model.Job, error) {
			assert.True(t, jreq.IsTest)
			if assert.NotNil(t, jreq.SourceID) {
				assert.Equal(t, id, *jreq.SourceID)
			}
			return &model.Job{ID: "job-2"}, nil
		},
	).Times(1)

	got, err := svc.Update(ctx, id, req)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, updated, got)
}

func TestSourceService_Update_NoEnqueue_WhenTestFalse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockSrc := mocks.NewMockSourceRepository(ctrl)
	mockJobs := mocks.NewMockJobRepository(ctrl)
	svc := NewSourceService(SourceServiceOptions{SourceRepo: mockSrc, Jobs: mockJobs})

	id := testSourceID
	falseVal := false
	req := model.UpdateSourceRequest{Test: &falseVal}
	updated := &model.Source{ID: id, Name: "src", Value: "v", Test: false}

	mockSrc.EXPECT().Update(ctx, id, req).Return(updated, nil)
	// no job create expected

	got, err := svc.Update(ctx, id, req)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, updated, got)
}

func TestSourceService_Update_RefreshesCacheWithResolvedSecrets(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockSrc := mocks.NewMockSourceRepository(ctrl)
	mockJobs := mocks.NewMockJobRepository(ctrl)
	mockCache := core.NewMockCacheRepository(ctrl)
	mockSecrets := mocks.NewMockSecretRepository(ctrl)

	cacheSvc := core.NewSourceCacheService(core.SourceCacheServiceOptions{
		Cache:   mockCache,
		Sources: mockSrc,
		Secrets: mockSecrets,
		Config:  core.DefaultSourceCacheConfig(),
	})
	svc := NewSourceService(
		SourceServiceOptions{SourceRepo: mockSrc, Jobs: mockJobs, Cache: cacheSvc, SecretRepo: mockSecrets},
	)

	id := testSourceID
	newValue := "console.log('__API_KEY__')"
	req := model.UpdateSourceRequest{Value: &newValue}
	updated := &model.Source{ID: id, Name: "src", Value: newValue, Secrets: []string{"API_KEY"}, Test: false}

	mockSrc.EXPECT().Update(ctx, id, req).Return(updated, nil)

	key := "source:content:" + id
	mockSrc.EXPECT().GetByID(gomock.Any(), gomock.Any()).Times(0)
	mockCache.EXPECT().Delete(gomock.Any(), key).Return(true, nil)
	mockCache.EXPECT().Get(gomock.Any(), key).Return(nil, nil)
	mockSecrets.EXPECT().GetByName(ctx, "API_KEY").Return(&model.Secret{Name: "API_KEY", Value: "rotated"}, nil)
	mockCache.EXPECT().Set(gomock.Any(), key, []byte("console.log('rotated')"), gomock.Any()).Return(nil)

	got, err := svc.Update(ctx, id, req)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, updated, got)
}
