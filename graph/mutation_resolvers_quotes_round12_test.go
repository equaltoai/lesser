package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound12MutationResolvers_Quotes_UpdateQuotePermissions(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)

	// Avoid pulling in boosted-state checks inside convertStatusToObject.
	originalBoostFn := viewerBoostStateResolverFunc
	viewerBoostStateResolverFunc = func(context.Context, *Resolver, string, string) (bool, error) { return false, nil }
	t.Cleanup(func() { viewerBoostStateResolverFunc = originalBoostFn })

	require.NoError(t, storageRepo.Status().CreateStatus(context.Background(), &models.Status{
		StatusID:       "status-1",
		AuthorUsername: "alice",
		Content:        "hello",
		CreatedAt:      time.Now().Add(-time.Hour),
		UpdatedAt:      time.Now(),
	}))

	payload, err := resolver.Mutation().UpdateQuotePermissions(round12AuthContext("alice"), "status-1", true, model.QuotePermissionFollowers)
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.True(t, payload.Success)
	require.NotNil(t, payload.Note)
	require.True(t, payload.Note.Quoteable)
	require.Equal(t, model.QuotePermissionFollowers, payload.Note.QuotePermissions)
	storedType, err := storageRepo.Object().GetQuoteType(context.Background(), "status-1")
	require.NoError(t, err)
	require.Equal(t, EventTypeFollowers, storedType)

	payload, err = resolver.Mutation().UpdateQuotePermissions(round12AuthContext("alice"), "status-1", false, model.QuotePermissionEveryone)
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.False(t, payload.Note.Quoteable)
	require.Equal(t, model.QuotePermissionNone, payload.Note.QuotePermissions)
	storedType, err = storageRepo.Object().GetQuoteType(context.Background(), "status-1")
	require.NoError(t, err)
	require.Equal(t, "disabled", storedType)

	_, err = resolver.Mutation().UpdateQuotePermissions(round12AuthContext("bob"), "status-1", true, model.QuotePermissionEveryone)
	require.Error(t, err)
}

func TestGraphQLQuoteControl_ProjectsStoredTypes(t *testing.T) {
	tests := []struct {
		name       string
		storedType string
		quoteable  bool
		permission model.QuotePermission
	}{
		{name: "public", storedType: "public", quoteable: true, permission: model.QuotePermissionEveryone},
		{name: "followers", storedType: EventTypeFollowers, quoteable: true, permission: model.QuotePermissionFollowers},
		{name: "mentioned", storedType: "mentioned", quoteable: true, permission: model.QuotePermissionFollowers},
		{name: "deny default", storedType: "unknown", quoteable: false, permission: model.QuotePermissionNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quoteable, permission := graphQLQuoteControl(tt.storedType)
			require.Equal(t, tt.quoteable, quoteable)
			require.Equal(t, tt.permission, permission)
		})
	}
}
