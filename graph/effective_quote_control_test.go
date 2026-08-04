package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestEffectiveGraphQLQuoteControlMatchesEnforcementMatrix(t *testing.T) {
	accounts := []struct {
		name        string
		permissions models.QuotePermissions
	}{
		{name: "public default", permissions: models.QuotePermissions{AllowPublic: true, AllowFollowers: true, AllowMentioned: true}},
		{name: "private default", permissions: models.QuotePermissions{AllowFollowers: true, AllowMentioned: true}},
		{name: "mentioned only", permissions: models.QuotePermissions{AllowMentioned: true}},
		{name: "none", permissions: models.QuotePermissions{}},
	}
	perNoteTypes := []string{models.VisibilityPublic, EventTypeFollowers, "mentioned", "disabled"}

	for _, account := range accounts {
		for _, perNote := range perNoteTypes {
			t.Run(account.name+"/"+perNote, func(t *testing.T) {
				allowsPublic := account.permissions.AllowPublic && perNote == models.VisibilityPublic
				allowsFollower := (account.permissions.AllowPublic || account.permissions.AllowFollowers) &&
					(perNote == models.VisibilityPublic || perNote == EventTypeFollowers)
				allowsMentioned := (account.permissions.AllowPublic || account.permissions.AllowMentioned) &&
					(perNote == models.VisibilityPublic || perNote == "mentioned")

				expectedPermission := model.QuotePermissionNone
				switch {
				case allowsPublic:
					expectedPermission = model.QuotePermissionEveryone
				case allowsFollower:
					expectedPermission = model.QuotePermissionFollowers
				case allowsMentioned:
					expectedPermission = model.QuotePermissionMentioned
				}

				quoteable, permission := effectiveGraphQLQuoteControl(
					&models.Status{Visibility: models.VisibilityPublic}, &account.permissions, perNote,
				)
				require.Equal(t, expectedPermission != model.QuotePermissionNone, quoteable)
				require.Equal(t, expectedPermission, permission)
			})
		}
	}

	quoteable, permission := effectiveGraphQLQuoteControl(
		&models.Status{Visibility: models.VisibilityPublic}, nil, models.VisibilityPublic,
	)
	require.False(t, quoteable)
	require.Equal(t, model.QuotePermissionNone, permission)
}

func TestEffectiveGraphQLQuoteControlRejectsNonPublicStatusVisibility(t *testing.T) {
	account := &models.QuotePermissions{AllowPublic: true}
	for _, tt := range []struct {
		visibility string
		quoteable  bool
		permission model.QuotePermission
	}{
		{visibility: models.VisibilityPrivate, permission: model.QuotePermissionNone},
		{visibility: models.VisibilityDirect, permission: model.QuotePermissionNone},
		{visibility: models.VisibilityPublic, quoteable: true, permission: model.QuotePermissionEveryone},
		{visibility: models.VisibilityUnlisted, quoteable: true, permission: model.QuotePermissionEveryone},
	} {
		t.Run(tt.visibility, func(t *testing.T) {
			quoteable, permission := effectiveGraphQLQuoteControl(
				&models.Status{Visibility: tt.visibility}, account, models.VisibilityPublic,
			)
			require.Equal(t, tt.quoteable, quoteable)
			require.Equal(t, tt.permission, permission)
		})
	}
}

func TestDetermineQuoteableRejectsNonPublicVisibilityBeforePermissionLoads(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	quoteCalls := 0
	accountCalls := 0
	loaders := &Loaders{
		QuoteControlLoader: newQuoteControlLoaderWithLookup(
			func(_ context.Context, statusIDs []string) (map[string]string, error) {
				quoteCalls++
				return map[string]string{statusIDs[0]: models.VisibilityPublic}, nil
			}, zap.NewNop()),
		QuoteAccountLoader: newQuoteAccountLoaderWithLookup(
			func(_ context.Context, usernames []string) (map[string]*models.QuotePermissions, error) {
				accountCalls++
				return map[string]*models.QuotePermissions{
					usernames[0]: {Username: usernames[0], AllowPublic: true},
				}, nil
			}, zap.NewNop()),
	}
	ctx := WithLoaders(context.Background(), loaders)

	for _, visibility := range []string{models.VisibilityPrivate, models.VisibilityDirect} {
		quoteable, permission := resolver.determineQuoteable(ctx, &models.Status{
			StatusID: "non-public-" + visibility, AuthorUsername: "alice", Visibility: visibility,
		})
		require.False(t, quoteable)
		require.Equal(t, model.QuotePermissionNone, permission)
	}
	require.Zero(t, quoteCalls)
	require.Zero(t, accountCalls)
}

func TestDetermineQuoteableDoesNotOverPromisePrivateAccountDefault(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	loaders := &Loaders{
		QuoteControlLoader: newQuoteControlLoaderWithLookup(
			func(_ context.Context, statusIDs []string) (map[string]string, error) {
				return map[string]string{statusIDs[0]: models.VisibilityPublic}, nil
			}, zap.NewNop()),
		QuoteAccountLoader: newQuoteAccountLoaderWithLookup(
			func(_ context.Context, usernames []string) (map[string]*models.QuotePermissions, error) {
				return map[string]*models.QuotePermissions{
					usernames[0]: {Username: usernames[0], AllowFollowers: true, AllowMentioned: true},
				}, nil
			}, zap.NewNop()),
	}
	ctx := WithLoaders(context.Background(), loaders)

	quoteable, permission := resolver.determineQuoteable(ctx, &models.Status{
		StatusID: "status-without-metadata", AuthorUsername: "private-author", Visibility: models.VisibilityPublic,
	})
	require.True(t, quoteable)
	require.Equal(t, model.QuotePermissionFollowers, permission)
}
