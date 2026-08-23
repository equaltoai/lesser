package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestMediaRepositoryUpdateMediaPublishedStateIsFieldScoped(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	update := new(mocks.MockUpdateBuilder)
	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.AnythingOfType("*models.Media")).Return(query).Once()
	query.On("Where", "PK", "=", "media#m1").Return(query).Once()
	query.On("Where", "SK", "=", "version#original").Return(query).Once()
	query.On("UpdateBuilder").Return(update).Once()
	update.On("Set", "PublishedS3Key", "published/media/m1.png").Return(update).Once()
	update.On("Set", "PublishedURL", "https://cdn.example.test/published/media/m1.png").Return(update).Once()
	update.On("Set", "PublishedAt", now).Return(update).Once()
	update.On("ConditionExists", "PK").Return(update).Once()
	update.On("ConditionVersion", int64(3)).Return(update).Once()
	update.On("Set", "ModelVersion", 4).Return(update).Once()
	update.On("Execute").Return(nil).Once()

	repo := NewMediaRepository(db, "test-table", zap.NewNop(), nil)
	err := repo.UpdateMediaPublishedState(ctx, "m1", "published/media/m1.png", "https://cdn.example.test/published/media/m1.png", now, 3)
	require.NoError(t, err)
	db.AssertExpectations(t)
	query.AssertExpectations(t)
	update.AssertExpectations(t)
}

func TestMediaRepositoryUpdateMediaEditorialStateIsFieldScoped(t *testing.T) {
	ctx := context.Background()
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	update := new(mocks.MockUpdateBuilder)
	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.AnythingOfType("*models.Media")).Return(query).Once()
	query.On("Where", "PK", "=", "media#m1").Return(query).Once()
	query.On("Where", "SK", "=", "version#original").Return(query).Once()
	query.On("UpdateBuilder").Return(update).Once()
	update.On("Set", "EditorialState", models.EditorialLifecycleWithdrawn).Return(update).Once()
	update.On("Remove", "SupersededByMediaID").Return(update).Once()
	update.On("ConditionExists", "PK").Return(update).Once()
	update.On("ConditionVersion", int64(3)).Return(update).Once()
	update.On("Set", "ModelVersion", 4).Return(update).Once()
	update.On("Execute").Return(nil).Once()

	repo := NewMediaRepository(db, "test-table", zap.NewNop(), nil)
	err := repo.UpdateMediaEditorialState(ctx, "m1", models.EditorialLifecycleWithdrawn, "", 3)
	require.NoError(t, err)
	db.AssertExpectations(t)
	query.AssertExpectations(t)
	update.AssertExpectations(t)
}

func TestMediaRepositoryUpdateMediaEditorialStateSupersededNamesSuccessor(t *testing.T) {
	ctx := context.Background()
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	update := new(mocks.MockUpdateBuilder)
	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.AnythingOfType("*models.Media")).Return(query).Once()
	query.On("Where", "PK", "=", "media#m1").Return(query).Once()
	query.On("Where", "SK", "=", "version#original").Return(query).Once()
	query.On("UpdateBuilder").Return(update).Once()
	update.On("Set", "EditorialState", models.EditorialLifecycleSuperseded).Return(update).Once()
	update.On("Set", "SupersededByMediaID", "m2").Return(update).Once()
	update.On("ConditionExists", "PK").Return(update).Once()
	update.On("ConditionVersion", int64(3)).Return(update).Once()
	update.On("Set", "ModelVersion", 4).Return(update).Once()
	update.On("Execute").Return(nil).Once()

	repo := NewMediaRepository(db, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.UpdateMediaEditorialState(ctx, "m1", models.EditorialLifecycleSuperseded, "m2", 3))
	require.Error(t, repo.UpdateMediaEditorialState(ctx, "m1", models.EditorialLifecycleSuperseded, "  ", 3))
	db.AssertExpectations(t)
	query.AssertExpectations(t)
	update.AssertExpectations(t)
}

func TestMediaRepositoryClearMediaPublishedStateIsFieldScoped(t *testing.T) {
	ctx := context.Background()
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	update := new(mocks.MockUpdateBuilder)
	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.AnythingOfType("*models.Media")).Return(query).Once()
	query.On("Where", "PK", "=", "media#m1").Return(query).Once()
	query.On("Where", "SK", "=", "version#original").Return(query).Once()
	query.On("UpdateBuilder").Return(update).Once()
	update.On("Remove", "PublishedS3Key").Return(update).Once()
	update.On("Remove", "PublishedURL").Return(update).Once()
	update.On("Remove", "PublishedAt").Return(update).Once()
	update.On("ConditionExists", "PK").Return(update).Once()
	update.On("ConditionVersion", int64(3)).Return(update).Once()
	update.On("Set", "ModelVersion", 4).Return(update).Once()
	update.On("Execute").Return(nil).Once()

	repo := NewMediaRepository(db, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.ClearMediaPublishedState(ctx, "m1", 3))
	db.AssertExpectations(t)
	query.AssertExpectations(t)
	update.AssertExpectations(t)
}
