package service

import (
	"context"
	"errors"
	"testing"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service/rules"
	"github.com/stretchr/testify/require"
)

type fakeIOCRepo struct {
	createFn func(context.Context, model.CreateIOCRequest) (*model.IOC, error)
	deleteFn func(context.Context, string) (bool, error)
}

func (f *fakeIOCRepo) Create(ctx context.Context, req model.CreateIOCRequest) (*model.IOC, error) {
	if f.createFn != nil {
		return f.createFn(ctx, req)
	}
	return nil, errors.New("create not implemented")
}

func (f *fakeIOCRepo) Delete(ctx context.Context, id string) (bool, error) {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, id)
	}
	return false, errors.New("delete not implemented")
}

// Unused interface methods panic if called.
func (*fakeIOCRepo) GetByID(context.Context, string) (*model.IOC, error) {
	panic("not implemented")
}

func (*fakeIOCRepo) List(context.Context, model.IOCListOptions) ([]*model.IOC, error) {
	panic("not implemented")
}

func (*fakeIOCRepo) Update(context.Context, string, model.UpdateIOCRequest) (*model.IOC, error) {
	panic("not implemented")
}

func (*fakeIOCRepo) BulkCreate(context.Context, model.BulkCreateIOCsRequest) (int, error) {
	panic("not implemented")
}

func (*fakeIOCRepo) LookupHost(context.Context, model.IOCLookupRequest) (*model.IOC, error) {
	panic("not implemented")
}

func (*fakeIOCRepo) Stats(context.Context) (*core.IOCStats, error) {
	panic("not implemented")
}

var _ core.IOCRepository = (*fakeIOCRepo)(nil)

type recordingVersioner struct {
	bumps int
}

func (r *recordingVersioner) Current(context.Context) (string, error) {
	return "current", nil
}

func (r *recordingVersioner) Bump(context.Context) (string, error) {
	r.bumps++
	return "next", nil
}

var _ rules.IOCVersioner = (*recordingVersioner)(nil)

func TestIOCServiceCreateBumpsCacheVersion(t *testing.T) {
	ctx := context.Background()
	versioner := &recordingVersioner{}
	repo := &fakeIOCRepo{
		createFn: func(context.Context, model.CreateIOCRequest) (*model.IOC, error) {
			return &model.IOC{ID: "ioc-1"}, nil
		},
	}

	svc := MustNewIOCService(IOCServiceOptions{
		Repo:           repo,
		CacheVersioner: versioner,
	})

	_, err := svc.Create(ctx, model.CreateIOCRequest{
		Type:  model.IOCTypeFQDN,
		Value: "example.com",
	})
	require.NoError(t, err)
	require.Equal(t, 1, versioner.bumps)
}

func TestIOCServiceDeleteSkipsBumpWhenNotDeleted(t *testing.T) {
	ctx := context.Background()
	versioner := &recordingVersioner{}
	repo := &fakeIOCRepo{
		deleteFn: func(context.Context, string) (bool, error) {
			return false, nil
		},
	}

	svc := MustNewIOCService(IOCServiceOptions{
		Repo:           repo,
		CacheVersioner: versioner,
	})

	deleted, err := svc.Delete(ctx, "missing")
	require.NoError(t, err)
	require.False(t, deleted)
	require.Equal(t, 0, versioner.bumps)
}
