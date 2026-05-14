package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestAccountRepository_ActorHelpers_Coverage(t *testing.T) {
	repo := &AccountRepository{domain: ""}

	t.Run("mergeActivityPubImage", func(t *testing.T) {
		mergeActivityPubImage(nil, nil)
		mergeActivityPubImage(&activitypub.Image{}, nil)
		mergeActivityPubImage(nil, &activitypub.Image{})

		dest := &activitypub.Image{}
		src := &activitypub.Image{
			BaseObject: activitypub.BaseObject{Type: "Image"},
			URL:        "https://example.com/img.png",
			MediaType:  "image/png",
			Width:      10,
			Height:     20,
		}
		mergeActivityPubImage(dest, src)
		require.Equal(t, src.Type, dest.Type)
		require.Equal(t, src.URL, dest.URL)
		require.Equal(t, src.MediaType, dest.MediaType)
		require.Equal(t, src.Width, dest.Width)
		require.Equal(t, src.Height, dest.Height)
	})

	t.Run("deriveActorBaseURL", func(t *testing.T) {
		require.Empty(t, deriveActorBaseURL(nil, "alice"))
		require.Equal(t, "https://example.com", deriveActorBaseURL(&activitypub.Actor{URL: "https://example.com/@alice"}, "alice"))
		require.Equal(t, "https://example.com", deriveActorBaseURL(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}, "alice"))
	})

	t.Run("extractBaseURL", func(t *testing.T) {
		require.Empty(t, extractBaseURL("", "/@alice"))
		require.Equal(t, "https://example.com", extractBaseURL("https://example.com/@alice", "/@alice"))
		require.Empty(t, extractBaseURL("/@alice", "/@alice"))
	})

	t.Run("ensureActorIdentifiers", func(t *testing.T) {
		actor := &activitypub.Actor{
			Name: "Alice",
			URL:  "https://example.com/@alice",
			PublicKey: &activitypub.PublicKey{
				PublicKeyPem: "pem",
			},
		}
		repo.ensureActorIdentifiers("alice", actor, nil)
		require.NotEmpty(t, actor.ID)
		require.Equal(t, "alice", actor.PreferredUsername)
		require.NotNil(t, actor.PublicKey)
		require.NotEmpty(t, actor.PublicKey.Owner)
		require.NotEmpty(t, actor.PublicKey.ID)
	})

	t.Run("mergeActorDataForUpdate", func(t *testing.T) {
		existing := &activitypub.Actor{
			PreferredUsername: "alice",
			BaseObject:        activitypub.BaseObject{Type: activitypub.PersonType},
			Icon:              &activitypub.Image{URL: "old"},
		}

		incoming := &activitypub.Actor{
			Name:                      "Alice Updated",
			Summary:                   "bio",
			Discoverable:              true,
			ManuallyApprovesFollowers: true,
			Attachment:                []activitypub.Attachment{{Name: "x", Value: "y"}},
			Icon:                      &activitypub.Image{URL: "new"},
			Image:                     &activitypub.Image{URL: "banner"},
			PublicKey:                 &activitypub.PublicKey{PublicKeyPem: "pem"},
			Endpoints:                 &activitypub.Endpoints{SharedInbox: "https://example.com/inbox"},
		}

		merged := repo.mergeActorDataForUpdate("alice", existing, incoming)
		require.Equal(t, "Alice Updated", merged.Name)
		require.Equal(t, "bio", merged.Summary)
		require.True(t, merged.Discoverable)
		require.True(t, merged.ManuallyApprovesFollowers)
		require.NotNil(t, merged.Icon)
		require.Equal(t, "new", merged.Icon.URL)
		require.NotNil(t, merged.Image)
		require.Equal(t, "banner", merged.Image.URL)
		require.NotNil(t, merged.PublicKey)
		require.NotNil(t, merged.Endpoints)

		require.Equal(t, existing, repo.mergeActorDataForUpdate("alice", existing, nil))
	})
}

func TestAccountRepository_UserAndDomainBranchCoverage(t *testing.T) {
	baseTime := time.Date(2025, 9, 10, 11, 12, 13, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	require.Empty(t, repo.canonicalUsername(""))
	require.Equal(t, "alice", repo.canonicalUsername("@alice"))
	require.Equal(t, "alice", repo.canonicalUsername("acct:alice"))
	require.Equal(t, "alice", repo.canonicalUsername("https://example.com/@alice"))
	require.Equal(t, "alice", repo.canonicalUsername("https://example.com/profile/alice"))
	require.Equal(t, "alice@remote.example", repo.canonicalUsername("https://remote.example/@alice"))
	require.False(t, repo.isLocalDomain(""))
	require.False(t, repo.isLocalDomain("remote.example"))
}

func TestAccountRepository_GetPreferenceAndBlockState_Branches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 9, 10, 11, 12, 13, 0, time.UTC)

	t.Run("preference_notfound", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		value, err := repo.GetPreference(ctx, "alice", "lang")
		require.NoError(t, err)
		require.Empty(t, value)
	})

	t.Run("preference_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.GetPreference(ctx, "alice", "lang")
		require.Error(t, err)
	})

	t.Run("follow_request_notfound", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		state, err := repo.GetFollowRequestState(ctx, "req", "tgt")
		require.NoError(t, err)
		require.Empty(t, state)
	})

	t.Run("follow_request_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.GetFollowRequestState(ctx, "req", "tgt")
		require.Error(t, err)
	})

	t.Run("blocked_domain_notfound", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		blocked, err := repo.IsBlockedDomain(ctx, "alice", "remote.example")
		require.NoError(t, err)
		require.False(t, blocked)
	})

	t.Run("blocked_domain_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.IsBlockedDomain(ctx, "alice", "remote.example")
		require.Error(t, err)
	})
}

func TestAccountRepository_CreateAndDeletePin_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 9, 10, 11, 12, 13, 0, time.UTC)

	t.Run("create_pin_condition_failed_is_ok", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(dynamormErrors.ErrConditionFailed).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.NoError(t, repo.CreateAccountPin(ctx, &storage.AccountPin{Username: "alice", PinnedActorID: "id"}))
	})

	t.Run("create_pin_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.CreateAccountPin(ctx, &storage.AccountPin{Username: "alice", PinnedActorID: "id"}))
	})
}

func TestAccountRepository_GetUser_NotFoundAndError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 9, 10, 11, 12, 13, 0, time.UTC)

	t.Run("not_found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
			_, ok := dest.(*userCoreProjection)
			return ok
		})).Return(dynamormErrors.ErrItemNotFound).Twice()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		user, err := repo.GetUser(ctx, "alice")
		require.Error(t, err)
		require.Nil(t, user)
	})

	t.Run("query_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
			_, ok := dest.(*userCoreProjection)
			return ok
		})).Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		user, err := repo.GetUser(ctx, "alice")
		require.Error(t, err)
		require.Nil(t, user)
	})
}

func TestAccountRepository_UpdateUser_VersionHydrationErrors(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 9, 10, 11, 12, 13, 0, time.UTC)

	t.Run("hydration_error_skips", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
			_, ok := dest.(*userVersionProjection)
			return ok
		})).Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.NoError(t, repo.UpdateUser(ctx, "alice", map[string]interface{}{"display_name": "x"}))
	})

	t.Run("hydration_zero_defaults_to_one", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
			_, ok := dest.(*userVersionProjection)
			return ok
		})).Run(func(args mock.Arguments) {
			proj := args.Get(0).(*userVersionProjection)
			proj.Value = 0
		}).Return(nil).Once()

		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.NoError(t, repo.UpdateUser(ctx, "alice", map[string]interface{}{"display_name": "x"}))
	})
}

func TestAccountRepository_DiscoveryAndPreferences_CursorAndTypeBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 9, 10, 11, 12, 13, 0, time.UTC)

	t.Run("suggested_accounts_has_more_and_cursor", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.User)
			*dest = []models.User{
				{Username: "a", CreatedAt: baseTime, UpdatedAt: baseTime},
				{Username: "b", CreatedAt: baseTime, UpdatedAt: baseTime},
			}
			for i := range *dest {
				_ = (*dest)[i].UpdateKeys()
			}
		}).Return(nil).Once()

		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		result, err := repo.GetSuggestedAccounts(ctx, "", interfaces.PaginationOptions{Limit: 1})
		require.NoError(t, err)
		require.True(t, result.HasMore)
		require.NotEmpty(t, result.NextCursor)
	})

	t.Run("featured_accounts_limit_and_cursor", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.User)
			*dest = []models.User{
				{Username: "admin", CreatedAt: baseTime, UpdatedAt: baseTime, Role: "admin"},
				{Username: "mod", CreatedAt: baseTime, UpdatedAt: baseTime, Role: "moderator"},
			}
			for i := range *dest {
				_ = (*dest)[i].UpdateKeys()
			}
		}).Return(nil).Once()

		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		cursor := Utils.Pagination.EncodeCursor("", "admin")
		result, err := repo.GetFeaturedAccounts(ctx, interfaces.PaginationOptions{Limit: 0, Cursor: cursor})
		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("account_features_ignores_non_bool", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.UserPreference)
			*dest = []models.UserPreference{
				{Key: "feature_new_ui", Value: "maybe"},
			}
		}).Return(nil).Once()

		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		features, err := repo.GetAccountFeatures(ctx, "alice")
		require.NoError(t, err)
		require.Empty(t, features)
	})
}
