package health

import (
	stdErrors "errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHealthCheckEvent_Validate(t *testing.T) {
	e := NewHealthCheckEvent([]string{"example.com"}, 5)
	require.NoError(t, e.Validate())

	invalid := &HealthCheckEvent{}
	require.Equal(t, ErrActionRequired, invalid.Validate())

	invalid = NewHealthCheckEvent(nil, 5)
	invalid.Detail.Domains = nil
	invalid.Detail.InstanceIDs = nil
	require.Equal(t, ErrDomainsOrInstanceIDsRequired, invalid.Validate())
}

func TestAggregationEvent_Validate_InvalidWindow(t *testing.T) {
	e := NewAggregationEvent([]string{"example.com"}, []string{"1h", "bad"})
	err := e.Validate()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidWindowFormat)
	require.True(t, stdErrors.Is(err, ErrInvalidWindowFormat))
}

func TestHealthCheckEvent_GetBatchedDomains(t *testing.T) {
	e := NewHealthCheckEvent([]string{"a", "b", "c"}, 2)
	batches := e.GetBatchedDomains()
	require.Equal(t, [][]string{{"a", "b"}, {"c"}}, batches)
}
