package core

import (
	"context"
	"errors"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// stubSecretRepo provides a minimal SecretRepository implementation for tests.
type stubSecretRepo struct {
	values map[string]*model.Secret
	err    error
}

func newStubSecretRepo(values map[string]*model.Secret, err error) *stubSecretRepo {
	return &stubSecretRepo{values: values, err: err}
}

func (s *stubSecretRepo) Create(context.Context, model.CreateSecretRequest) (*model.Secret, error) {
	return nil, errors.New("not implemented")
}

func (s *stubSecretRepo) GetByID(context.Context, string) (*model.Secret, error) {
	return nil, errors.New("not implemented")
}

func (s *stubSecretRepo) GetByName(_ context.Context, name string) (*model.Secret, error) {
	if s.err != nil {
		return nil, s.err
	}
	if secret, ok := s.values[name]; ok {
		return secret, nil
	}
	return nil, errors.New("secret not found")
}

func (s *stubSecretRepo) List(context.Context, int, int) ([]*model.Secret, error) {
	return nil, errors.New("not implemented")
}

func (s *stubSecretRepo) Update(context.Context, string, model.UpdateSecretRequest) (*model.Secret, error) {
	return nil, errors.New("not implemented")
}

func (s *stubSecretRepo) Delete(context.Context, string) (bool, error) {
	return false, errors.New("not implemented")
}

func (s *stubSecretRepo) FindDueForRefresh(context.Context, int) ([]*model.Secret, error) {
	return nil, errors.New("not implemented")
}

func (s *stubSecretRepo) UpdateRefreshStatus(context.Context, UpdateSecretRefreshStatusParams) error {
	return errors.New("not implemented")
}
