package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestCreateActor_MissingUsername(t *testing.T) {
	repo := &ActorRepository{}

	actor := &activitypub.Actor{
		Name: "Test User",
	}
	privateKey := "test-private-key"

	err := repo.CreateActor(context.Background(), actor, privateKey)

	assert.Error(t, err)
	// Error should be an AppError from validation
	assert.NotNil(t, err)
}

func TestUpdateActor_MissingUsername(t *testing.T) {
	repo := &ActorRepository{}

	actor := &activitypub.Actor{
		Name: "Updated User",
	}

	err := repo.UpdateActor(context.Background(), actor)

	assert.Error(t, err)
	// Error should be an AppError from validation
	assert.NotNil(t, err)
}

func TestSearchAccounts_EmptyQuery(t *testing.T) {
	repo := &ActorRepository{}

	// With empty query, should return nil, nil (handled by guard clause)
	results, err := repo.SearchAccounts(context.Background(), "", 10, false, 0)

	assert.NoError(t, err)
	assert.Nil(t, results)
}

func TestGetSearchSuggestions_ShortPrefix(t *testing.T) {
	repo := &ActorRepository{}

	results, err := repo.GetSearchSuggestions(context.Background(), "a")

	assert.NoError(t, err)
	assert.Empty(t, results)
}

func TestNewActorRepository(t *testing.T) {
	logger := zap.NewNop()
	tableName := "test-table"
	repo := NewActorRepository(nil, tableName, logger)

	assert.NotNil(t, repo)
	assert.Nil(t, repo.db)
	assert.Equal(t, tableName, repo.tableName)
	assert.Equal(t, logger, repo.logger)
}
