package domain_test

import (
	"testing"

	"github.com/target/mmk-ui-api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestParseOverrunStateMask(t *testing.T) {
	mask, err := domain.ParseOverrunStateMask("running, pending")
	require.NoError(t, err)
	require.True(t, mask.Has(domain.OverrunStateRunning))
	require.True(t, mask.Has(domain.OverrunStatePending))
	require.False(t, mask.Has(domain.OverrunStateRetrying))
	require.Equal(t, "running,pending", mask.String())
}

func TestParseOverrunStateMaskInvalid(t *testing.T) {
	_, err := domain.ParseOverrunStateMask("unknown")
	require.Error(t, err)
}

func TestOverrunStateMaskMarshal(t *testing.T) {
	mask := domain.OverrunStatePending | domain.OverrunStateRetrying
	text, err := mask.MarshalText()
	require.NoError(t, err)
	require.Equal(t, "pending,retrying", string(text))

	var roundTrip domain.OverrunStateMask
	require.NoError(t, roundTrip.UnmarshalText(text))
	require.Equal(t, mask, roundTrip)
}
