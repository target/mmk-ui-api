package core

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

func TestResolveSecretPlaceholders(t *testing.T) {
	ctx := context.Background()

	t.Run("no repo or secrets returns content", func(t *testing.T) {
		out, err := ResolveSecretPlaceholders(ctx, nil, nil, "hello")
		require.NoError(t, err)
		assert.Equal(t, "hello", out)
	})

	t.Run("replaces placeholders", func(t *testing.T) {
		repo := newStubSecretRepo(map[string]*model.Secret{
			"TOKEN": {Name: "TOKEN", Value: "abc123"},
		}, nil)
		out, err := ResolveSecretPlaceholders(ctx, repo, []string{"TOKEN"}, "Bearer __TOKEN__")
		require.NoError(t, err)
		assert.Equal(t, "Bearer abc123", out)
	})

	t.Run("skips missing placeholders", func(t *testing.T) {
		repo := newStubSecretRepo(map[string]*model.Secret{
			"TOKEN": {Name: "TOKEN", Value: "abc123"},
		}, nil)
		out, err := ResolveSecretPlaceholders(ctx, repo, []string{"TOKEN"}, "No secrets here")
		require.NoError(t, err)
		assert.Equal(t, "No secrets here", out)
	})

	t.Run("propagates repo error", func(t *testing.T) {
		repo := newStubSecretRepo(nil, errors.New("boom"))
		out, err := ResolveSecretPlaceholders(ctx, repo, []string{"TOKEN"}, "__TOKEN__")
		require.Error(t, err)
		assert.Empty(t, out)
	})

	t.Run("error when repo missing", func(t *testing.T) {
		out, err := ResolveSecretPlaceholders(ctx, nil, []string{"TOKEN"}, "__TOKEN__")
		require.Error(t, err)
		assert.Empty(t, out)
	})
}
