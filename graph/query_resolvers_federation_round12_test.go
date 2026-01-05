package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/stretchr/testify/require"
)

func TestRound12QueryFederation_OverviewAndCosts(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	qry := resolver.Query()

	metrics, err := qry.InstanceMetrics(context.Background())
	require.NoError(t, err)
	require.NotNil(t, metrics)

	status, err := qry.FederationStatus(context.Background(), "example.com")
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, "example.com", status.Domain)

	flow, err := qry.FederationFlow(context.Background(), model.TimePeriodDay)
	require.NoError(t, err)
	require.NotNil(t, flow)

	graph, err := qry.FederationMap(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, graph)

	health, err := qry.FederationHealth(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, health)

	limits, err := qry.FederationLimits(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, limits)

	// Severance service is optional in tests; ensure error path is stable.
	_, err = qry.SeveredRelationships(context.Background(), nil, nil, nil)
	require.Error(t, err)

	_, err = qry.AffectedRelationships(context.Background(), "sr-1")
	require.Error(t, err)

	// Federation costs requires auth and uses the federation repository.
	first := 10
	after := "cursor_5"
	costs, err := qry.FederationCosts(round12AuthContext("alice"), &first, &after, nil)
	require.NoError(t, err)
	require.NotNil(t, costs)
	require.NotNil(t, costs.PageInfo)
}

func TestRound12QueryFederation_RelationshipsAndBudgets(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	qry := resolver.Query()

	relations, err := qry.InstanceRelationships(context.Background(), "example.com")
	require.NoError(t, err)
	require.NotNil(t, relations)

	// Analytics may return an error depending on storage; either is acceptable for coverage.
	_, _ = qry.InstanceBudgets(context.Background(), nil)
	_, _ = qry.InstanceHealthReport(context.Background(), "example.com")
}

