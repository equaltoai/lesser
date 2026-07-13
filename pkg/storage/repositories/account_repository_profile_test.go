package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	dynamormMocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap/zaptest"
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
	mockQuery.On("IfNotExists").Return(mockQuery).Once()
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

func TestCreateRecoveredActorProfile_ConditionalConflictPreservesExistingKeyMaterial(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormMocks.MockDB)
	mockQuery := new(dynamormMocks.MockQuery)
	mockUpdate := new(dynamormMocks.MockUpdateBuilder)
	lastStatusAt := time.Date(2026, 7, 13, 12, 30, 0, 0, time.UTC)
	actorID := commonActorURL("example.com", "alice")
	existing := &models.Actor{
		Username:   "alice",
		Actor:      fullLocalActor("example.com", "alice", "Original Alice"),
		PrivateKey: "encrypted-private-key",
		KeyType:    "RSA",
		NumericID:  "123456",
		Fields: []models.ActorField{{
			Name:  "site",
			Value: "https://example.com/about",
		}},
		LastStatusAt:   &lastStatusAt,
		FollowerCount:  11,
		FollowingCount: 12,
		StatusCount:    13,
		Version:        7,
	}
	require.NoError(t, existing.UpdateKeys())

	var attemptedRepair *models.Actor
	populateExisting := func(args mock.Arguments) {
		dest, ok := args.Get(0).(*models.Actor)
		if !ok {
			return
		}
		*dest = *cloneActorModel(existing)
	}

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Run(func(args mock.Arguments) {
		if actorModel, ok := args.Get(0).(*models.Actor); ok && actorModel.Actor != nil && actorModel.PrivateKey == "" {
			attemptedRepair = actorModel
		}
	}).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("IfNotExists").Return(mockQuery).Once()
	mockQuery.On("Create").Return(dynamormErrors.ErrConditionFailed).Once()
	mockQuery.On("ConsistentRead").Return(mockQuery).Once()
	mockQuery.On("First", mock.Anything).Run(populateExisting).Return(nil).Twice()
	mockQuery.On("UpdateBuilder").Return(mockUpdate).Once()
	mockUpdate.On("Set", mock.Anything, mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("Remove", mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("ConditionVersion", int64(7)).Return(mockUpdate).Once()
	mockUpdate.On("Execute").Return(nil).Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	err := repo.createRecoveredActorProfile(ctx, "alice", &activitypub.Actor{
		Name:    "Updated Alice",
		Summary: "updated profile summary",
	})
	require.NoError(t, err)

	require.NotNil(t, attemptedRepair)
	require.Empty(t, attemptedRepair.PrivateKey)
	require.Empty(t, attemptedRepair.KeyType)
	require.Equal(t, 0, attemptedRepair.FollowerCount)
	require.Equal(t, 0, attemptedRepair.FollowingCount)
	require.Equal(t, 0, attemptedRepair.StatusCount)
	mockQuery.AssertCalled(t, "IfNotExists")

	mockUpdate.AssertCalled(t, "ConditionVersion", int64(7))
	mockUpdate.AssertCalled(t, "Set", "Version", 8)
	mockUpdate.AssertNotCalled(t, "Set", "PrivateKey", mock.Anything)
	mockUpdate.AssertNotCalled(t, "Set", "privateKey", mock.Anything)
	mockUpdate.AssertNotCalled(t, "Set", "KeyType", mock.Anything)
	mockUpdate.AssertNotCalled(t, "Set", "keyType", mock.Anything)
	mockUpdate.AssertNotCalled(t, "Set", "FollowerCount", mock.Anything)
	mockUpdate.AssertNotCalled(t, "Set", "FollowingCount", mock.Anything)
	mockUpdate.AssertNotCalled(t, "Set", "StatusCount", mock.Anything)

	require.Equal(t, "encrypted-private-key", existing.PrivateKey)
	require.Equal(t, "RSA", existing.KeyType)
	require.Equal(t, "123456", existing.NumericID)
	require.Equal(t, 11, existing.FollowerCount)
	require.Equal(t, 12, existing.FollowingCount)
	require.Equal(t, 13, existing.StatusCount)
	require.NotNil(t, existing.LastStatusAt)
	require.True(t, existing.LastStatusAt.Equal(lastStatusAt))
	require.Equal(t, []models.ActorField{{Name: "site", Value: "https://example.com/about"}}, existing.Fields)
	require.Equal(t, 7, existing.Version)
	require.NotNil(t, existing.Actor)
	require.Equal(t, actorID, existing.Actor.ID)
	require.NotNil(t, existing.Actor.PublicKey)
	require.Equal(t, actorID+"#main-key", existing.Actor.PublicKey.ID)
	require.Equal(t, "public-key-pem", existing.Actor.PublicKey.PublicKeyPem)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdate.AssertExpectations(t)
}

func commonActorURL(domain, username string) string {
	return "https://" + domain + "/users/" + username
}

func fullLocalActor(domain, username, name string) *activitypub.Actor {
	actorID := commonActorURL(domain, username)
	return &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   actorID,
			Type: "Person",
		},
		PreferredUsername: username,
		Name:              name,
		URL:               "https://" + domain + "/@" + username,
		Inbox:             actorID + "/inbox",
		Outbox:            actorID + "/outbox",
		Followers:         actorID + "/followers",
		Following:         actorID + "/following",
		Liked:             actorID + "/liked",
		PublicKey: &activitypub.PublicKey{
			ID:           actorID + "#main-key",
			Owner:        actorID,
			PublicKeyPem: "public-key-pem",
		},
	}
}

func cloneActorModel(src *models.Actor) *models.Actor {
	if src == nil {
		return nil
	}

	clone := *src
	if src.Actor != nil {
		actorClone := *src.Actor
		if src.Actor.PublicKey != nil {
			publicKeyClone := *src.Actor.PublicKey
			actorClone.PublicKey = &publicKeyClone
		}
		clone.Actor = &actorClone
	}
	if src.LastStatusAt != nil {
		lastStatusAt := *src.LastStatusAt
		clone.LastStatusAt = &lastStatusAt
	}
	clone.Fields = append([]models.ActorField(nil), src.Fields...)

	return &clone
}
