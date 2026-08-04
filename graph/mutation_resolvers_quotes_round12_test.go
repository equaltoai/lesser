package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRound12MutationResolvers_Quotes_UpdateQuotePermissions(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	projectionContext := func() context.Context {
		loaders := NewLoaders(storageRepo, zap.NewNop())
		loaders.QuoteAccountLoader = newQuoteAccountLoaderWithLookup(
			func(_ context.Context, usernames []string) (map[string]*models.QuotePermissions, error) {
				permissions := make(map[string]*models.QuotePermissions, len(usernames))
				for _, username := range usernames {
					permissions[username] = &models.QuotePermissions{
						Username:       username,
						AllowPublic:    true,
						AllowFollowers: true,
						AllowMentioned: true,
					}
				}
				return permissions, nil
			}, zap.NewNop())
		return WithLoaders(context.Background(), loaders)
	}

	// Avoid pulling in boosted-state checks inside convertStatusToObject.
	originalBoostFn := viewerBoostStateResolverFunc
	viewerBoostStateResolverFunc = func(context.Context, *Resolver, string, string) (bool, error) { return false, nil }
	t.Cleanup(func() { viewerBoostStateResolverFunc = originalBoostFn })

	require.NoError(t, storageRepo.Status().CreateStatus(context.Background(), &models.Status{
		StatusID:       "status-1",
		AuthorUsername: "alice",
		Content:        "hello",
		Visibility:     models.VisibilityPublic,
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
	storedStatus, err := storageRepo.Status().GetStatus(context.Background(), "status-1")
	require.NoError(t, err)
	projected := resolver.convertStatusToObject(projectionContext(), storedStatus)
	require.True(t, projected.Quoteable)
	require.Equal(t, model.QuotePermissionFollowers, projected.QuotePermissions)

	payload, err = resolver.Mutation().UpdateQuotePermissions(round12AuthContext("alice"), "status-1", true, model.QuotePermissionMentioned)
	require.NoError(t, err)
	require.True(t, payload.Note.Quoteable)
	require.Equal(t, model.QuotePermissionMentioned, payload.Note.QuotePermissions)
	storedType, err = storageRepo.Object().GetQuoteType(context.Background(), "status-1")
	require.NoError(t, err)
	require.Equal(t, "mentioned", storedType)

	payload, err = resolver.Mutation().UpdateQuotePermissions(round12AuthContext("alice"), "status-1", false, model.QuotePermissionEveryone)
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.False(t, payload.Note.Quoteable)
	require.Equal(t, model.QuotePermissionNone, payload.Note.QuotePermissions)
	storedType, err = storageRepo.Object().GetQuoteType(context.Background(), "status-1")
	require.NoError(t, err)
	require.Equal(t, "disabled", storedType)
	projected = resolver.convertStatusToObject(projectionContext(), storedStatus)
	require.False(t, projected.Quoteable)
	require.Equal(t, model.QuotePermissionNone, projected.QuotePermissions)

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
		{name: "public normalized", storedType: " PUBLIC ", quoteable: true, permission: model.QuotePermissionEveryone},
		{name: "followers", storedType: EventTypeFollowers, quoteable: true, permission: model.QuotePermissionFollowers},
		{name: "followers normalized", storedType: " Followers ", quoteable: true, permission: model.QuotePermissionFollowers},
		{name: "mentioned", storedType: "mentioned", quoteable: true, permission: model.QuotePermissionMentioned},
		{name: "mentioned normalized", storedType: " MENTIONED ", quoteable: true, permission: model.QuotePermissionMentioned},
		{name: "disabled", storedType: "disabled", quoteable: false, permission: model.QuotePermissionNone},
		{name: "disabled normalized", storedType: " DISABLED ", quoteable: false, permission: model.QuotePermissionNone},
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
