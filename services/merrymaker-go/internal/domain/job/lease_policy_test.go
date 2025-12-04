package job

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLeasePolicy(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		policy, err := NewLeasePolicy(30 * time.Second)
		require.NoError(t, err)
		assert.Equal(t, 30*time.Second, policy.Default())
	})

	t.Run("invalid default lease", func(t *testing.T) {
		policy, err := NewLeasePolicy(0)
		require.ErrorIs(t, err, ErrInvalidDefaultLease)
		assert.Nil(t, policy)
	})
}

func TestLeasePolicy_Resolve(t *testing.T) {
	policy, err := NewLeasePolicy(30 * time.Second)
	require.NoError(t, err)

	t.Run("explicit duration uses whole seconds", func(t *testing.T) {
		decision := policy.Resolve(45 * time.Second)
		assert.Equal(t, 45, decision.Seconds)
		assert.Equal(t, LeaseSourceExplicit, decision.Source)
		assert.False(t, decision.Clamped())
	})

	t.Run("default duration when request is zero", func(t *testing.T) {
		decision := policy.Resolve(0)
		assert.Equal(t, 30, decision.Seconds)
		assert.Equal(t, LeaseSourceDefault, decision.Source)
		assert.True(t, decision.UsedDefault())
	})

	t.Run("sub-second duration clamps to minimum", func(t *testing.T) {
		decision := policy.Resolve(500 * time.Millisecond)
		assert.Equal(t, 1, decision.Seconds)
		assert.Equal(t, LeaseSourceClamped, decision.Source)
		assert.True(t, decision.Clamped())
	})

	t.Run("negative duration clamps to minimum", func(t *testing.T) {
		decision := policy.Resolve(-5 * time.Second)
		assert.Equal(t, 1, decision.Seconds)
		assert.Equal(t, LeaseSourceClamped, decision.Source)
		assert.True(t, decision.Clamped())
	})
}
