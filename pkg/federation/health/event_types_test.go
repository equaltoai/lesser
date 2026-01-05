package health

import (
	stdErrors "errors"
	"testing"
	"time"

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

func TestHealthCheckEvent_JSONRoundTrip(t *testing.T) {
	e := NewHealthCheckEvent([]string{"example.com"}, 5)
	payload, err := e.ToJSON()
	require.NoError(t, err)

	var decoded HealthCheckEvent
	require.NoError(t, decoded.FromJSON(payload))
	require.Equal(t, "lesser.federation.health", decoded.Source)
	require.Equal(t, "check_health", decoded.Detail.Action)
	require.Equal(t, []string{"example.com"}, decoded.Detail.Domains)
}

func TestAggregationEvent_JSONRoundTrip(t *testing.T) {
	e := NewAggregationEvent([]string{"example.com"}, []string{"1h"})
	payload, err := e.ToJSON()
	require.NoError(t, err)

	var decoded AggregationEvent
	require.NoError(t, decoded.FromJSON(payload))
	require.Equal(t, "lesser.federation.health", decoded.Source)
	require.Equal(t, []string{"example.com"}, decoded.Detail.Domains)
	require.Equal(t, []string{"1h"}, decoded.Detail.Windows)
}

func TestHealthCheckEvent_Validate_NegativeValues(t *testing.T) {
	e := NewHealthCheckEvent([]string{"example.com"}, 5)
	e.Detail.BatchSize = -1
	require.Equal(t, ErrBatchSizeMustBePositive, e.Validate())

	e = NewHealthCheckEvent([]string{"example.com"}, 5)
	e.Detail.Timeout = -1
	require.Equal(t, ErrTimeoutMustBePositive, e.Validate())
}

func TestAggregationEvent_Validate_MissingFields(t *testing.T) {
	e := &AggregationEvent{}
	require.Equal(t, ErrActionRequired, e.Validate())

	e = &AggregationEvent{Detail: AggregationDetail{Action: "aggregate_summaries", Domains: nil, Windows: []string{"1h"}}}
	require.Equal(t, ErrDomainsRequiredForAggregation, e.Validate())

	e = &AggregationEvent{Detail: AggregationDetail{Action: "aggregate_summaries", Domains: []string{"example.com"}, Windows: nil}}
	require.Equal(t, ErrWindowsRequiredForAggregation, e.Validate())
}

func TestHealthCheckEvent_GetBatchedDomains_DefaultsAndEmpty(t *testing.T) {
	e := NewHealthCheckEvent([]string{"a", "b"}, 0)
	batches := e.GetBatchedDomains()
	require.Equal(t, [][]string{{"a", "b"}}, batches)

	e = NewHealthCheckEvent(nil, 10)
	e.Detail.Domains = nil
	e.Detail.InstanceIDs = []string{"i1"}
	// Make it valid for action, then verify batching returns empty when domains are empty.
	require.NoError(t, e.Validate())
	require.Equal(t, [][]string{}, e.GetBatchedDomains())

	// Cover scheduled event types with a stable timestamp.
	_ = ScheduledHealthCheckEvent{Time: time.Now()}
}
