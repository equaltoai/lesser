package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	dynamormMocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap/zaptest"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

func TestMergeActorDataForUpdate_PreservesIdentifiersAndAppliesUpdates(t *testing.T) {
	repo := NewAccountRepository(nil, "test-table", "dev.lesser.host", zaptest.NewLogger(t))

	existing := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID: "https://dev.lesser.host/users/admin",
		},
		PreferredUsername: "admin",
		URL:               "https://dev.lesser.host/@admin",
		Inbox:             "https://dev.lesser.host/users/admin/inbox",
		Outbox:            "https://dev.lesser.host/users/admin/outbox",
		Followers:         "https://dev.lesser.host/users/admin/followers",
		Following:         "https://dev.lesser.host/users/admin/following",
		Liked:             "https://dev.lesser.host/users/admin/liked",
		Icon: &activitypub.Image{
			URL: "https://cdn.dev.lesser.host/avatars/admin.png",
		},
	}

	incoming := &activitypub.Actor{
		Name:    "Administrator QA",
		Summary: "Updated summary",
		Icon: &activitypub.Image{
			URL: "https://cdn.dev.lesser.host/avatars/admin-updated.png",
		},
		Discoverable:              true,
		ManuallyApprovesFollowers: true,
	}

	merged := repo.mergeActorDataForUpdate("admin", existing, incoming)

	require.Equal(t, "https://dev.lesser.host/users/admin", merged.ID)
	require.Equal(t, "Administrator QA", merged.Name)
	require.Equal(t, "Updated summary", merged.Summary)
	require.True(t, merged.Discoverable)
	require.True(t, merged.ManuallyApprovesFollowers)
	require.NotNil(t, merged.Icon)
	require.Equal(t, "https://cdn.dev.lesser.host/avatars/admin-updated.png", merged.Icon.URL)
	require.Equal(t, "admin", merged.PreferredUsername)
	require.Equal(t, "https://dev.lesser.host/@admin", merged.URL)
}

func TestMergeActorDataForUpdate_DerivesIdentifiersWhenMissing(t *testing.T) {
	repo := NewAccountRepository(nil, "test-table", "dev.lesser.host", zaptest.NewLogger(t))

	incoming := &activitypub.Actor{
		Name: "Test User",
	}

	merged := repo.mergeActorDataForUpdate("tester", nil, incoming)

	require.Equal(t, "https://dev.lesser.host/users/tester", merged.ID)
	require.Equal(t, "https://dev.lesser.host/@tester", merged.URL)
	require.Equal(t, "https://dev.lesser.host/users/tester/inbox", merged.Inbox)
	require.Equal(t, "https://dev.lesser.host/users/tester/outbox", merged.Outbox)
	require.Equal(t, "https://dev.lesser.host/users/tester/followers", merged.Followers)
	require.Equal(t, "https://dev.lesser.host/users/tester/following", merged.Following)
	require.Equal(t, "https://dev.lesser.host/users/tester/liked", merged.Liked)
	require.Equal(t, "tester", merged.PreferredUsername)
	require.Equal(t, "Test User", merged.Name)
}

func TestUpdateAccountActorProfile_RepairsMissingActorProfileRow(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormMocks.MockDB)
	mockQuery := new(dynamormMocks.MockQuery)
	var created *models.Actor

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Run(func(args mock.Arguments) {
		if actorModel, ok := args.Get(0).(*models.Actor); ok && actorModel.Actor != nil {
			created = actorModel
		}
	}).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
	mockQuery.On("Create").Return(nil).Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	err := repo.updateAccountActorProfile(ctx, "alice", &storage.Account{
		Actor: &activitypub.Actor{
			Name:                      "Della Updated",
			Summary:                   "same bio",
			Discoverable:              true,
			ManuallyApprovesFollowers: true,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, created)
	require.Equal(t, "alice", created.Username)
	require.Empty(t, created.PrivateKey)
	require.Equal(t, commonActorURL("example.com", "alice"), created.Actor.ID)
	require.Equal(t, "Della Updated", created.Actor.Name)
	require.Equal(t, "same bio", created.Actor.Summary)
	require.True(t, created.Actor.Discoverable)
	require.True(t, created.Actor.ManuallyApprovesFollowers)
	mockQuery.AssertExpectations(t)
}

func commonActorURL(domain, username string) string {
	return "https://" + domain + "/users/" + username
}
