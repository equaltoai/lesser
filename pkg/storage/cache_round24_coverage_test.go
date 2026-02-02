package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	repotesting "github.com/equaltoai/lesser/pkg/storage/theorydb/repositories/testing"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCacheService_CacheUser_Round24(t *testing.T) {
	db := &repotesting.MockDB{}
	query := &repotesting.MockQuery{}

	var captured *models.User
	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.Anything).Return(query).Run(func(args mock.Arguments) {
		u, ok := args.Get(0).(*models.User)
		require.True(t, ok)
		captured = u
	}).Once()
	query.On("Create").Return(nil).Once()

	cs := NewCacheService(db, zap.NewNop())
	require.NoError(t, cs.CacheUser(context.Background(), "user-1", &models.User{Username: "alice"}))

	require.NotNil(t, captured)
	require.Equal(t, "CACHE#USER#user-1", captured.PK)
	require.Equal(t, "DATA", captured.SK)
	require.Equal(t, "alice", captured.Username)
	require.False(t, captured.CreatedAt.IsZero())

	db.AssertExpectations(t)
	query.AssertExpectations(t)
}

func TestCacheService_GetCachedUser_Round24(t *testing.T) {
	db := &repotesting.MockDB{}
	query := &repotesting.MockQuery{}

	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.Anything).Return(query).Once()
	query.On("Where", "PK", "=", "CACHE#USER#user-1").Return(query).Once()
	query.On("Where", "SK", "=", "DATA").Return(query).Once()
	query.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.User)
		dest.Username = "alice"
	}).Once()

	cs := NewCacheService(db, zap.NewNop())
	user, err := cs.GetCachedUser(context.Background(), "user-1")
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, "alice", user.Username)

	db.AssertExpectations(t)
	query.AssertExpectations(t)
}

func TestCacheService_GetCachedUser_ReturnsError_Round24(t *testing.T) {
	db := &repotesting.MockDB{}
	query := &repotesting.MockQuery{}

	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.Anything).Return(query).Once()
	query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Twice()
	query.On("First", mock.Anything).Return(errors.New("not found")).Once()

	cs := NewCacheService(db, zap.NewNop())
	user, err := cs.GetCachedUser(context.Background(), "user-1")
	require.Error(t, err)
	require.Nil(t, user)

	db.AssertExpectations(t)
	query.AssertExpectations(t)
}

func TestCacheService_InvalidateUser_Round24(t *testing.T) {
	db := &repotesting.MockDB{}
	query := &repotesting.MockQuery{}

	var captured *models.User
	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.Anything).Return(query).Run(func(args mock.Arguments) {
		u, ok := args.Get(0).(*models.User)
		require.True(t, ok)
		captured = u
	}).Once()
	query.On("Delete").Return(nil).Once()

	cs := NewCacheService(db, zap.NewNop())
	require.NoError(t, cs.InvalidateUser(context.Background(), "user-1"))

	require.NotNil(t, captured)
	require.Equal(t, "CACHE#USER#user-1", captured.PK)
	require.Equal(t, "DATA", captured.SK)

	db.AssertExpectations(t)
	query.AssertExpectations(t)
}
