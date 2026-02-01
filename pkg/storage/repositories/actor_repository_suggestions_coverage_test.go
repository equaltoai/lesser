package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestActorRepository_popular_and_discoverable_helpers(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	repo := NewActorRepository(mockDB, "test-table", logger)

	// getPopularActors: exercise error-continue and discoverable filtering
	mockQuery.On("All", mock.Anything).Return(errors.New("bucket query failed")).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Actor)
		*out = []models.Actor{
			{Username: "alice", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "id1"}, PreferredUsername: "alice", Discoverable: true}},
			{Username: "bob", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "id2"}, PreferredUsername: "bob", Discoverable: false}},
		}
	}).Once()
	actors, err := repo.getPopularActors(ctx, 1)
	assert.NoError(t, err)
	assert.Len(t, actors, 1)

	// getRecentActiveActors fallback to popularity when no recent activity is found
	mockQuery.On("All", mock.Anything).Return(nil).Times(7)
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Actor)
		*out = []models.Actor{
			{Username: "carla", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "id3"}, PreferredUsername: "carla", Discoverable: true}},
		}
	}).Once()
	actors, err = repo.getRecentActiveActors(ctx, 1)
	assert.NoError(t, err)
	assert.Len(t, actors, 1)

	// getDiscoverableActors is a thin wrapper
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Actor)
		*out = []models.Actor{
			{Username: "dana", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "id4"}, PreferredUsername: "dana", Discoverable: true}},
			{Username: "dana", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "id4"}, PreferredUsername: "dana", Discoverable: true}},
		}
	}).Once()
	actors, err = repo.getDiscoverableActors(ctx, 1)
	assert.NoError(t, err)
	assert.Len(t, actors, 1)
}

func TestActorRepository_fillRemainingSuggestions_and_shouldIncludeDiscoverable(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	repo := NewActorRepository(mockDB, "test-table", logger)

	existing := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "id1"}, PreferredUsername: "existing", Discoverable: true}
	suggestions := []*activitypub.Actor{existing}

	// fillRemainingSuggestions: needs getDiscoverableActors(remaining*2) -> getRecentActiveActors(2) -> requires >=4 records to stop.
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Actor)
		*out = []models.Actor{
			{Username: "followed", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "id2"}, PreferredUsername: "followed", Discoverable: true}},
			{Username: "new1", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "id3"}, PreferredUsername: "new1", Discoverable: true}},
			{Username: "new2", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "id4"}, PreferredUsername: "new2", Discoverable: true}},
			{Username: "new3", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "id5"}, PreferredUsername: "new3", Discoverable: true}},
		}
	}).Once()

	userFollows := map[string]bool{
		"id2": true,
	}

	out := repo.fillRemainingSuggestions(ctx, suggestions, userFollows, 2)
	assert.Len(t, out, 2)
	assert.Equal(t, "id1", out[0].ID)
	assert.Equal(t, "id3", out[1].ID)

	// Early return when already full
	out2 := repo.fillRemainingSuggestions(ctx, out, userFollows, 2)
	assert.Equal(t, out, out2)
}
