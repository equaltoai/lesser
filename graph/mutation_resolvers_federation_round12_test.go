package graph

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/stretchr/testify/require"
)

func TestRound12MutationResolvers_Federation_StubsAndSeveranceGuards(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	mut := resolver.Mutation()

	opt, err := mut.OptimizeFederationCosts(round12AuthContext("alice"), 100)
	require.NoError(t, err)
	require.NotNil(t, opt)
	require.Greater(t, opt.SavedMonthlyUsd, 0.0)

	limit, err := mut.SetFederationLimit(round12AuthContext("alice"), "example.com", model.FederationLimitInput{})
	require.NoError(t, err)
	require.NotNil(t, limit)
	require.Equal(t, "example.com", limit.Domain)

	until := model.Time(time.Now().Add(time.Hour))
	paused, err := mut.PauseFederation(round12AuthContext("alice"), "example.com", "maintenance", &until)
	require.NoError(t, err)
	require.NotNil(t, paused)
	require.Equal(t, model.FederationStatePaused, paused.Status)

	resumed, err := mut.ResumeFederation(round12AuthContext("alice"), "example.com")
	require.NoError(t, err)
	require.NotNil(t, resumed)
	require.Equal(t, model.FederationStateActive, resumed.Status)

	autoLimit := true
	budget, err := mut.SetInstanceBudget(round12AuthContext("alice"), "example.com", 10, &autoLimit)
	require.NoError(t, err)
	require.NotNil(t, budget)
	require.True(t, budget.AutoLimit)

	_, err = mut.AcknowledgeSeverance(round12AuthContext("alice"), "sev-1")
	require.Error(t, err)
	_, err = mut.AttemptReconnection(round12AuthContext("alice"), "sev-1")
	require.Error(t, err)
}

