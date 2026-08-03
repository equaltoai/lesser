package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestHashtagRepository_FollowAndUnfollow(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	createQuery := new(dynamormmocks.MockQuery)
	deleteQuery := new(dynamormmocks.MockQuery)
	muteCleanupQuery := new(dynamormmocks.MockQuery)
	settingsCleanupQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)

	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		f, ok := model.(*models.HashtagFollow)
		if !ok {
			return false
		}
		return f.PK == "user#alice" && f.SK == "hashtag#golang" && f.NotificationsEnabled && !f.Muted
	})).Return(createQuery).Once()
	createQuery.On("Create").Return(nil).Once()

	repo := NewHashtagRepository(mockDB, "test-table", zap.NewNop(), "example.com")

	require.NoError(t, repo.FollowHashtag(ctx, "alice", "#Golang"))

	// Unfollow
	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		f, ok := model.(*models.HashtagFollow)
		if !ok {
			return false
		}
		return f.PK == "user#alice" && f.SK == "hashtag#golang"
	})).Return(deleteQuery).Once()
	deleteQuery.On("Delete").Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.HashtagMute")).Return(muteCleanupQuery).Once()
	muteCleanupQuery.On("Delete").Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.HashtagNotificationSettings")).Return(settingsCleanupQuery).Once()
	settingsCleanupQuery.On("Delete").Return(nil).Once()

	require.NoError(t, repo.UnfollowHashtag(ctx, "alice", "golang"))

	mockDB.AssertExpectations(t)
	createQuery.AssertExpectations(t)
	deleteQuery.AssertExpectations(t)
	muteCleanupQuery.AssertExpectations(t)
	settingsCleanupQuery.AssertExpectations(t)
}

func TestHashtagRepository_GetFollowedHashtagsPagination(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	query := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagFollow")).Return(query).Once()

	query.On("Where", "PK", "=", "user#bob").Return(query).Once()
	query.On("Where", "SK", "BEGINS_WITH", "hashtag#").Return(query).Once()
	query.On("OrderBy", "SK", "ASC").Return(query).Once()
	query.On("Limit", 2).Return(query).Once()
	query.On("All", mock.AnythingOfType("*[]*models.HashtagFollow")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.HashtagFollow)
		*dest = []*models.HashtagFollow{
			{PK: "user#bob", SK: "hashtag#golang", UserID: "bob", Hashtag: "golang"},
			{PK: "user#bob", SK: "hashtag#rust", UserID: "bob", Hashtag: "rust"},
		}
	}).Return(nil).Once()

	repo := NewHashtagRepository(mockDB, "test-table", zap.NewNop(), "example.com")

	results, cursor, err := repo.GetFollowedHashtags(ctx, "bob", 1, "")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "golang", results[0].Hashtag)
	assert.Equal(t, "hashtag#rust", cursor)

	mockDB.AssertExpectations(t)
	query.AssertExpectations(t)
}

func TestHashtagRepository_MuteAndNotificationSettings(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormmocks.MockDB)
	muteCreateQuery := new(dynamormmocks.MockQuery)
	settingsLookupQuery := new(dynamormmocks.MockQuery)
	settingsCreateQuery := new(dynamormmocks.MockQuery)
	followLookupQuery := new(dynamormmocks.MockQuery)
	followUpdateQuery := new(dynamormmocks.MockQuery)
	isMutedQuery := new(dynamormmocks.MockQuery)
	unmuteQuery := new(dynamormmocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)

	until := time.Now().Add(1 * time.Hour).UTC()
	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		m, ok := model.(*models.HashtagMute)
		if !ok {
			return false
		}
		return m.PK == "user#carol" && m.SK == "mute#golang" && m.TTL == until.Unix()
	})).Return(muteCreateQuery).Once()
	muteCreateQuery.On("Create").Return(nil).Once()

	repo := NewHashtagRepository(mockDB, "test", zap.NewNop(), "example.com")
	require.NoError(t, repo.MuteHashtag(ctx, "carol", "golang", &until))

	// Notification update path (no existing record)
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagNotificationSettings")).Return(settingsLookupQuery).Once()
	settingsLookupQuery.On("Where", "PK", "=", "user#carol").Return(settingsLookupQuery).Once()
	settingsLookupQuery.On("Where", "SK", "=", "settings#golang").Return(settingsLookupQuery).Once()
	settingsLookupQuery.On("First", mock.Anything).Return(errors.ErrItemNotFound).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.HashtagNotificationSettings")).Return(settingsCreateQuery).Once()
	settingsCreateQuery.On("Create").Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.HashtagFollow")).Return(followLookupQuery).Once()
	followLookupQuery.On("Where", "PK", "=", "user#carol").Return(followLookupQuery).Once()
	followLookupQuery.On("Where", "SK", "=", "hashtag#golang").Return(followLookupQuery).Once()
	followLookupQuery.On("First", mock.AnythingOfType("*models.HashtagFollow")).Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.HashtagFollow")).Return(followUpdateQuery).Once()
	followUpdateQuery.On("Create").Return(nil).Once()

	settings := &storage.HashtagNotificationSettings{
		Level: "none",
		Muted: true,
	}
	require.NoError(t, repo.UpdateHashtagNotificationSettings(ctx, "carol", "golang", settings))

	// Expired mute check should trigger cleanup
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagMute")).Return(isMutedQuery).Once()
	isMutedQuery.On("Where", "PK", "=", "user#carol").Return(isMutedQuery).Once()
	isMutedQuery.On("Where", "SK", "=", "mute#golang").Return(isMutedQuery).Once()
	isMutedQuery.On("First", mock.AnythingOfType("*models.HashtagMute")).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.HashtagMute)
		m.PK = "user#carol"
		m.SK = "mute#golang"
		m.TTL = time.Now().Add(-time.Minute).Unix()
	}).Return(nil).Once()

	mockDB.On("Model", mock.AnythingOfType("*models.HashtagMute")).Return(unmuteQuery).Once()
	unmuteQuery.On("Delete").Return(nil).Once()

	muted, err := repo.IsHashtagMuted(ctx, "carol", "golang")
	require.NoError(t, err)
	assert.False(t, muted)

	mockDB.AssertExpectations(t)
	muteCreateQuery.AssertExpectations(t)
	settingsCreateQuery.AssertExpectations(t)
	unmuteQuery.AssertExpectations(t)
}
