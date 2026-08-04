package graph

import (
	"context"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	quoteservice "github.com/equaltoai/lesser/pkg/services/quotes"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	pkgtesting "github.com/equaltoai/lesser/pkg/testing"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type quoteControlOracleObjectRepository struct {
	interfaces.ObjectRepository
	quoteType string
}

func (r quoteControlOracleObjectRepository) GetQuoteType(context.Context, string) (string, error) {
	return r.quoteType, nil
}

type quoteControlOracleRelationshipRepository struct {
	interfaces.ConcreteRelationshipRepository
	following bool
}

func (r quoteControlOracleRelationshipRepository) IsFollowing(context.Context, string, string) (bool, error) {
	return r.following, nil
}

type quoteControlOracleStorage struct {
	core.RepositoryStorage
	object       interfaces.ObjectRepository
	relationship interfaces.ConcreteRelationshipRepository
}

func (s quoteControlOracleStorage) Object() interfaces.ObjectRepository { return s.object }
func (s quoteControlOracleStorage) Relationship() interfaces.ConcreteRelationshipRepository {
	return s.relationship
}

func TestEffectiveGraphQLQuoteControlMatchesEnforcementMatrix(t *testing.T) {
	_, storageRepo := newRound12GraphResolver(t)
	ctx := context.Background()
	perNoteTypes := []string{
		models.VisibilityPublic, " PUBLIC ",
		EventTypeFollowers, " Followers ",
		"mentioned", " MENTIONED ",
		"disabled", "unknown",
	}
	viewers := []struct {
		name      string
		username  string
		following bool
		mentioned bool
	}{
		{name: "public", username: "stranger"},
		{name: "follower", username: "follower", following: true},
		{name: "mentioned", username: "mentioned", mentioned: true},
	}

	cells := 0
	for permissionMask := range 8 {
		account := &models.QuotePermissions{
			Username:       "bob",
			AllowPublic:    permissionMask&1 != 0,
			AllowFollowers: permissionMask&2 != 0,
			AllowMentioned: permissionMask&4 != 0,
		}
		storageRepo.SeedQuotePermissions(account)
		for perNoteIndex, perNote := range perNoteTypes {
			t.Run(fmt.Sprintf("mask-%d/%q", permissionMask, perNote), func(t *testing.T) {
				allowedByViewer := make(map[string]bool, len(viewers))
				for viewerIndex, viewer := range viewers {
					status := &models.Status{
						StatusID:       fmt.Sprintf("oracle-%d-%d-%d", permissionMask, perNoteIndex, viewerIndex),
						AuthorID:       "https://localhost/users/bob",
						AuthorUsername: "bob",
						Visibility:     models.VisibilityPublic,
					}
					if viewer.mentioned {
						status.Mentions = []string{viewer.username}
					}
					require.NoError(t, storageRepo.Status().CreateStatus(ctx, status))

					oracleStorage := quoteControlOracleStorage{
						RepositoryStorage: storageRepo,
						object: quoteControlOracleObjectRepository{
							ObjectRepository: storageRepo.Object(), quoteType: perNote,
						},
						relationship: quoteControlOracleRelationshipRepository{
							ConcreteRelationshipRepository: storageRepo.Relationship(), following: viewer.following,
						},
					}
					allowed, err := quoteservice.NewQuoteService(oracleStorage, zap.NewNop()).
						CheckQuotePermissions(ctx, viewer.username, status)
					require.NoError(t, err)
					allowedByViewer[viewer.name] = allowed
					cells++
				}

				expectedPermission := model.QuotePermissionNone
				switch {
				case allowedByViewer["public"]:
					expectedPermission = model.QuotePermissionEveryone
				case allowedByViewer["follower"]:
					expectedPermission = model.QuotePermissionFollowers
				case allowedByViewer["mentioned"]:
					expectedPermission = model.QuotePermissionMentioned
				}
				expectedQuoteable := expectedPermission != model.QuotePermissionNone
				quoteable, permission := effectiveGraphQLQuoteControl(
					&models.Status{Visibility: models.VisibilityPublic}, account, perNote,
				)
				require.Equal(t, expectedQuoteable, quoteable, "projection must neither invent nor hide a real permission arm")
				require.Equal(t, expectedPermission, permission, "projection must match the real service's tightest arm")

				switch permission {
				case model.QuotePermissionEveryone:
					require.True(t, allowedByViewer["public"], "EVERYONE must not over-promise")
				case model.QuotePermissionFollowers:
					require.False(t, allowedByViewer["public"])
					require.True(t, allowedByViewer["follower"], "FOLLOWERS must not over-promise")
				case model.QuotePermissionMentioned:
					require.False(t, allowedByViewer["public"])
					require.False(t, allowedByViewer["follower"])
					require.True(t, allowedByViewer["mentioned"], "MENTIONED must not over-promise")
				case model.QuotePermissionNone:
					require.False(t, allowedByViewer["public"])
					require.False(t, allowedByViewer["follower"])
					require.False(t, allowedByViewer["mentioned"], "NONE must not hide an allowed arm")
				}
			})
		}
	}
	require.Equal(t, 192, cells)

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

func TestDetermineQuoteableUsesInMemoryMissingMetadataProductionDefault(t *testing.T) {
	storageRepo := pkgtesting.NewMockRepositoryStorage()
	resolver := &Resolver{Storage: storageRepo, Logger: zap.NewNop()}
	loaders := NewLoaders(storageRepo, zap.NewNop())
	loaders.QuoteAccountLoader = newQuoteAccountLoaderWithLookup(
		func(_ context.Context, usernames []string) (map[string]*models.QuotePermissions, error) {
			return map[string]*models.QuotePermissions{
				usernames[0]: {Username: usernames[0], AllowFollowers: true, AllowMentioned: true},
			}, nil
		}, zap.NewNop())
	ctx := WithLoaders(context.Background(), loaders)

	quoteable, permission := resolver.determineQuoteable(ctx, &models.Status{
		StatusID: "status-without-metadata", AuthorUsername: "private-author", Visibility: models.VisibilityPublic,
	})
	require.True(t, quoteable)
	require.Equal(t, model.QuotePermissionFollowers, permission)
}
