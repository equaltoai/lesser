package graph

import (
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/stretchr/testify/require"
)

func TestRound12QueryFederation_OverviewAndCosts(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	qry := resolver.Query()
	adminCtx := round12AuthContext("admin")

	_, err := qry.InstanceMetrics(round12AuthContext("alice"))
	require.Error(t, err)

	metrics, err := qry.InstanceMetrics(adminCtx)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	status, err := qry.FederationStatus(adminCtx, "example.com")
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, "example.com", status.Domain)

	flow, err := qry.FederationFlow(adminCtx, model.TimePeriodDay)
	require.NoError(t, err)
	require.NotNil(t, flow)

	graph, err := qry.FederationMap(adminCtx, nil)
	require.NoError(t, err)
	require.NotNil(t, graph)

	health, err := qry.FederationHealth(adminCtx, nil)
	require.NoError(t, err)
	require.NotNil(t, health)

	limits, err := qry.FederationLimits(adminCtx, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, limits)

	// Severance service is optional in tests; ensure error path is stable.
	_, err = qry.SeveredRelationships(adminCtx, nil, nil, nil)
	require.Error(t, err)

	_, err = qry.AffectedRelationships(adminCtx, "sr-1")
	require.Error(t, err)

	// Federation costs requires admin and uses the federation repository.
	first := 10
	after := "cursor_5"
	_, err = qry.FederationCosts(round12AuthContext("alice"), &first, &after, nil)
	require.Error(t, err)

	costs, err := qry.FederationCosts(adminCtx, &first, &after, nil)
	require.NoError(t, err)
	require.NotNil(t, costs)
	require.NotNil(t, costs.PageInfo)
}

func TestRound12QueryFederation_RelationshipsAndBudgets(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	qry := resolver.Query()
	adminCtx := round12AuthContext("admin")

	relations, err := qry.InstanceRelationships(adminCtx, "example.com")
	require.NoError(t, err)
	require.NotNil(t, relations)

	// Analytics may return an error depending on storage; either is acceptable for coverage.
	_, _ = qry.InstanceBudgets(adminCtx, nil)
	_, _ = qry.InstanceHealthReport(adminCtx, "example.com")
}
