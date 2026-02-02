package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound10_MarkerRepository_GetMarkers_DefaultTimelines(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound)

	repo := NewMarkerRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	markers, err := repo.GetMarkers(ctx, "user-1", nil)
	require.NoError(t, err)
	require.Empty(t, markers)
}

func TestRound10_MarkerRepository_SaveMarker_ConflictAndCreate(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().UTC()

	t.Run("version conflict is ignored", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewMarkerRepository(mockDB, "test-table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.NoError(t, repo.SaveMarker(ctx, "user-1", "home", "status-1", 1))
	})

	t.Run("create error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound)
		mockQuery.On("Create").Return(errors.New("boom"))

		repo := NewMarkerRepository(mockDB, "test-table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		err := repo.SaveMarker(ctx, "user-1", "home", "status-1", 2)
		require.ErrorIs(t, err, ErrMarkerSaveFailed)
	})

	t.Run("create success when no existing marker", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound)
		mockQuery.On("Create").Return(nil)

		repo := NewMarkerRepository(mockDB, "test-table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.NoError(t, repo.SaveMarker(ctx, "user-1", "home", "status-2", 1))
	})
}

func TestRound10_MarkerRepository_GetMarkers_Found(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewMarkerRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	markers, err := repo.GetMarkers(ctx, "user-1", []string{"home"})
	require.NoError(t, err)
	require.Contains(t, markers, "home")
	require.NotNil(t, markers["home"])
}

func TestRound10_MarkerRepository_GetMarkers_StringNotFoundFallback(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(errors.New("item not found"))

	repo := NewMarkerRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	markers, err := repo.GetMarkers(ctx, "user-1", []string{"home"})
	require.NoError(t, err)
	require.Empty(t, markers)
}
