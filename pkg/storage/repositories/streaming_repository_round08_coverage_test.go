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
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type mockDeviceProvider struct {
	mock.Mock
}

func (m *mockDeviceProvider) GetUserDevices(ctx context.Context, username string) ([]*storage.Device, error) {
	args := m.Called(ctx, username)
	if devices, ok := args.Get(0).([]*storage.Device); ok {
		return devices, args.Error(1)
	}
	return nil, args.Error(1)
}

func TestStreamingRepository_Round08_UpdateAndHistory(t *testing.T) {
	ctx := context.Background()

	t.Run("UpdateStreamingPreferences requires username", func(t *testing.T) {
		repo := NewStreamingRepository(nil, "table", zap.NewNop(), &mockDeviceProvider{}, nil)
		err := repo.UpdateStreamingPreferences(ctx, &storage.StreamingPreferences{})
		require.Error(t, err)
	})

	t.Run("UpdateStreamingPreferences conditional check yields version conflict", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Times(2)
		mockDB.On("Model", mock.Anything).Return(mockQuery).Times(2)

		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.StreamingPreferences)
			*dest = models.StreamingPreferences{
				Username:  "alice",
				Version:   1,
				UpdatedAt: time.Now().Add(-time.Hour),
			}
			_ = dest.UpdateKeys()
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(dynamormerrors.ErrConditionFailed).Once()

		repo := NewStreamingRepository(mockDB, "table", zap.NewNop(), &mockDeviceProvider{}, nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		err := repo.UpdateStreamingPreferences(ctx, &storage.StreamingPreferences{Username: "alice"})
		require.ErrorIs(t, err, storage.ErrVersionConflict)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetStreamingPreferenceHistory not found returns empty slice", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewStreamingRepository(mockDB, "table", zap.NewNop(), &mockDeviceProvider{}, nil)
		history, err := repo.GetStreamingPreferenceHistory(ctx, "alice", 10)
		require.NoError(t, err)
		assert.Empty(t, history)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestStreamingRepository_Round08_SyncAndConflicts(t *testing.T) {
	ctx := context.Background()

	t.Run("SyncStreamingPreferences returns error when device provider fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		// GetStreamingPreferences: Get(current) must succeed.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.StreamingPreferences)
			*dest = models.StreamingPreferences{
				Username:  "alice",
				Version:   1,
				UpdatedAt: time.Now().Add(-time.Hour),
			}
			dest.SetCurrentPreference()
		}).Once()

		// GetStreamingPreferencesByDevice: device override is present so Get succeeds.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.StreamingPreferences)
			*dest = models.StreamingPreferences{
				Username:       "alice",
				DeviceID:       "d1",
				DefaultQuality: "720p",
				Version:        2,
				UpdatedAt:      time.Now().Add(-time.Minute),
			}
			dest.SetDevicePreference("d1")
		}).Once()

		provider := &mockDeviceProvider{}
		provider.On("GetUserDevices", mock.Anything, "alice").Return(nil, assert.AnError).Once()

		repo := NewStreamingRepository(mockDB, "table", zap.NewNop(), provider, nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		err := repo.SyncStreamingPreferences(ctx, "alice", "d1")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
		provider.AssertExpectations(t)
	})

	t.Run("ResolvePreferenceConflict maps not found from pagination query", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewStreamingRepository(mockDB, "table", zap.NewNop(), &mockDeviceProvider{}, nil)
		_, err := repo.ResolvePreferenceConflict(ctx, "alice", storage.ConflictResolutionLatest)
		require.ErrorIs(t, err, storage.ErrNotFound)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("ResolvePreferenceConflict selects latest and updates current preference", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		// ResolvePreferenceConflict: FindWithPagination query.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "STREAMING_PREFS#alice").Return(mockQuery).Once()
		mockQuery.On("Limit", 101).Return(mockQuery).Once()
		mockQuery.On("OrderBy", "SK", SortOrderAsc).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.StreamingPreferences)
			old := &models.StreamingPreferences{Username: "alice", DefaultQuality: "480p", Version: 1, UpdatedAt: time.Now().Add(-2 * time.Hour)}
			old.SetVersionedPreference()
			newer := &models.StreamingPreferences{Username: "alice", DefaultQuality: "720p", Version: 2, UpdatedAt: time.Now().Add(-time.Hour)}
			newer.SetVersionedPreference()
			*dest = []*models.StreamingPreferences{old, newer}
		}).Once()

		// UpdateStreamingPreferences: Get current.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.StreamingPreferences)
			*dest = models.StreamingPreferences{
				Username:  "alice",
				Version:   10,
				UpdatedAt: time.Now().Add(-time.Minute),
			}
			dest.SetCurrentPreference()
		}).Once()

		// UpdateStreamingPreferences: Update succeeds.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()

		// UpdateStreamingPreferences: storePreferenceVersion create succeeds.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Create").Return(nil).Once()

		repo := NewStreamingRepository(mockDB, "table", zap.NewNop(), &mockDeviceProvider{}, nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		resolved, err := repo.ResolvePreferenceConflict(ctx, "alice", storage.ConflictResolutionLatest)
		require.NoError(t, err)
		require.NotNil(t, resolved)
		assert.Equal(t, "alice", resolved.Username)
		assert.Equal(t, "720p", resolved.DefaultQuality)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestStreamingRepository_Round08_ModelConversions(t *testing.T) {
	repo := NewStreamingRepository(nil, "table", zap.NewNop(), &mockDeviceProvider{}, nil)

	t.Run("modelToStorageType handles nil pointers", func(t *testing.T) {
		model := &models.StreamingPreferences{
			Username:          "alice",
			DefaultQuality:    "auto",
			AutoQuality:       true,
			PreloadNext:       true,
			PreferredCodec:    "h264",
			MaxBandwidthMbps:  0,
			BufferSizeSeconds: 10,
			Version:           1,
			SchemaVersion:     1,
			UpdatedAt:         time.Now(),
			HDREnabled:        nil,
			SubtitleEnabled:   nil,
		}
		prefs := repo.modelToStorageType(model)
		assert.False(t, prefs.HDREnabled)
		assert.False(t, prefs.SubtitleEnabled)
	})

	t.Run("storageTypeToModel populates pointer fields", func(t *testing.T) {
		prefs := &storage.StreamingPreferences{
			Username:                "alice",
			DeviceID:                "d1",
			DefaultQuality:          "1080p",
			AutoQuality:             false,
			PreloadNext:             false,
			DataSaverMode:           true,
			PreferredCodec:          "av1",
			MaxBandwidthMbps:        5,
			BufferSizeSeconds:       3,
			Version:                 2,
			SchemaVersion:           1,
			HDREnabled:              true,
			SubtitleEnabled:         true,
			AudioDescriptionEnabled: true,
			ClosedCaptionsEnabled:   true,
			UpdatedAt:               time.Now(),
		}

		model := repo.storageTypeToModel(prefs)
		require.NotNil(t, model.HDREnabled)
		assert.True(t, *model.HDREnabled)
		require.NotNil(t, model.SubtitleEnabled)
		assert.True(t, *model.SubtitleEnabled)
		assert.Equal(t, "alice", model.Username)
	})

	t.Run("modelToStorageType uses non-nil pointers", func(t *testing.T) {
		trueVal := true
		model := &models.StreamingPreferences{
			Username:                "alice",
			DefaultQuality:          "auto",
			AutoQuality:             true,
			PreferredCodec:          "h264",
			Version:                 1,
			SchemaVersion:           1,
			HDREnabled:              &trueVal,
			SubtitleEnabled:         &trueVal,
			AudioDescriptionEnabled: &trueVal,
			ClosedCaptionsEnabled:   &trueVal,
			UpdatedAt:               time.Now(),
		}
		prefs := repo.modelToStorageType(model)
		assert.True(t, prefs.HDREnabled)
		assert.True(t, prefs.SubtitleEnabled)
		assert.True(t, prefs.AudioDescriptionEnabled)
		assert.True(t, prefs.ClosedCaptionsEnabled)
	})
}

func TestStreamingRepository_Round08_DeviceAndUpdateSuccess(t *testing.T) {
	ctx := context.Background()

	t.Run("GetStreamingPreferences success returns converted model", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.StreamingPreferences)
			*dest = models.StreamingPreferences{
				Username:       "alice",
				DefaultQuality: "720p",
				AutoQuality:    true,
				Version:        2,
				SchemaVersion:  1,
				UpdatedAt:      time.Now(),
			}
			dest.SetCurrentPreference()
		}).Once()

		repo := NewStreamingRepository(mockDB, "table", zap.NewNop(), &mockDeviceProvider{}, nil)
		prefs, err := repo.GetStreamingPreferences(ctx, "alice")
		require.NoError(t, err)
		require.NotNil(t, prefs)
		assert.Equal(t, "720p", prefs.DefaultQuality)
	})

	t.Run("UpdateStreamingPreferences success ignores version history failure", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		// Get current.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Times(3)
		mockDB.On("Model", mock.Anything).Return(mockQuery).Times(3)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.StreamingPreferences)
			*dest = models.StreamingPreferences{Username: "alice", Version: 1, UpdatedAt: time.Now().Add(-time.Hour)}
			dest.SetCurrentPreference()
		}).Once()

		// Update succeeds.
		mockQuery.On("Update", mock.Anything).Return(nil).Once()
		// storePreferenceVersion create fails, but UpdateStreamingPreferences ignores it.
		mockQuery.On("Create").Return(assert.AnError).Once()

		repo := NewStreamingRepository(mockDB, "table", zap.NewNop(), &mockDeviceProvider{}, nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		err := repo.UpdateStreamingPreferences(ctx, &storage.StreamingPreferences{
			Username:       "alice",
			DefaultQuality: "1080p",
			Version:        1,
		})
		require.NoError(t, err)
	})

	t.Run("GetStreamingPreferencesByDevice returns device override model", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Times(2)
		mockDB.On("Model", mock.Anything).Return(mockQuery).Times(2)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

		// User prefs.
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.StreamingPreferences)
			*dest = models.StreamingPreferences{Username: "alice", DefaultQuality: "480p", Version: 1, UpdatedAt: time.Now().Add(-time.Hour)}
			dest.SetCurrentPreference()
		}).Once()
		// Device prefs.
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.StreamingPreferences)
			*dest = models.StreamingPreferences{Username: "alice", DeviceID: "d1", DefaultQuality: "4k", Version: 2, UpdatedAt: time.Now().Add(-time.Minute)}
			dest.SetDevicePreference("d1")
		}).Once()

		repo := NewStreamingRepository(mockDB, "table", zap.NewNop(), &mockDeviceProvider{}, nil)
		prefs, err := repo.GetStreamingPreferencesByDevice(ctx, "alice", "d1")
		require.NoError(t, err)
		assert.Equal(t, "4k", prefs.DefaultQuality)
	})

	t.Run("UpdateDeviceStreamingPreferences validates required params", func(t *testing.T) {
		repo := NewStreamingRepository(nil, "table", zap.NewNop(), &mockDeviceProvider{}, nil)
		err := repo.UpdateDeviceStreamingPreferences(ctx, &storage.StreamingPreferences{Username: ""}, "")
		require.Error(t, err)
	})
}

func TestStreamingRepository_Round08_Sweep(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.StreamingPreferences)
		*dest = models.StreamingPreferences{
			Username:         "alice",
			DeviceID:         "d1",
			DefaultQuality:   "720p",
			AutoQuality:      true,
			PreferredCodec:   "h264",
			MaxBandwidthMbps: 5,
			Version:          1,
			SchemaVersion:    1,
			UpdatedAt:        time.Now().Add(-time.Minute),
		}
		dest.SetCurrentPreference()
	}).Maybe()

	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.StreamingPreferences)
		old := &models.StreamingPreferences{Username: "alice", DefaultQuality: "480p", Version: 1, UpdatedAt: time.Now().Add(-2 * time.Hour)}
		old.SetVersionedPreference()
		newer := &models.StreamingPreferences{Username: "alice", DefaultQuality: "4k", Version: 2, UpdatedAt: time.Now().Add(-time.Hour), MaxBandwidthMbps: 1}
		newer.SetVersionedPreference()
		*dest = []*models.StreamingPreferences{old, newer}
	}).Maybe()

	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()

	provider := &mockDeviceProvider{}
	provider.On("GetUserDevices", mock.Anything, "alice").Return([]*storage.Device{
		{DeviceID: "d1"},
		{DeviceID: "d2"},
	}, nil).Maybe()

	repo := NewStreamingRepository(mockDB, "table", zap.NewNop(), provider, nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	_, _ = repo.GetStreamingPreferences(ctx, "alice")
	_ = repo.UpdateStreamingPreferences(ctx, &storage.StreamingPreferences{Username: "alice", DefaultQuality: "1080p"})
	_, _ = repo.GetStreamingPreferencesByDevice(ctx, "alice", "d1")
	_ = repo.UpdateDeviceStreamingPreferences(ctx, &storage.StreamingPreferences{Username: "alice", DefaultQuality: "720p"}, "d2")
	_, _ = repo.GetStreamingPreferenceHistory(ctx, "alice", 0)
	_ = repo.SyncStreamingPreferences(ctx, "alice", "d1")

	_, _ = repo.ResolvePreferenceConflict(ctx, "alice", storage.ConflictResolutionLatest)
	_, _ = repo.ResolvePreferenceConflict(ctx, "alice", storage.ConflictResolutionHighestQuality)
	_, _ = repo.ResolvePreferenceConflict(ctx, "alice", storage.ConflictResolutionLowestBandwidth)

	assert.True(t, isConditionalCheckFailedException(dynamormerrors.ErrConditionFailed))
}

func TestStreamingRepository_Round08_ConflictEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("ResolvePreferenceConflict returns not found when pagination returns no items", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "STREAMING_PREFS#alice").Return(mockQuery).Once()
		mockQuery.On("Limit", 101).Return(mockQuery).Once()
		mockQuery.On("OrderBy", "SK", SortOrderAsc).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.StreamingPreferences)
			*dest = nil
		}).Once()

		repo := NewStreamingRepository(mockDB, "table", zap.NewNop(), &mockDeviceProvider{}, nil)
		_, err := repo.ResolvePreferenceConflict(ctx, "alice", storage.ConflictResolutionLatest)
		require.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("ResolvePreferenceConflict returns error when strategy can't select", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "STREAMING_PREFS#alice").Return(mockQuery).Once()
		mockQuery.On("Limit", 101).Return(mockQuery).Once()
		mockQuery.On("OrderBy", "SK", SortOrderAsc).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.StreamingPreferences)
			a := &models.StreamingPreferences{Username: "alice", DefaultQuality: "unknown", MaxBandwidthMbps: 0, UpdatedAt: time.Now().Add(-time.Hour), Version: 1}
			a.SetVersionedPreference()
			*dest = []*models.StreamingPreferences{a}
		}).Once()

		repo := NewStreamingRepository(mockDB, "table", zap.NewNop(), &mockDeviceProvider{}, nil)
		_, err := repo.ResolvePreferenceConflict(ctx, "alice", storage.ConflictResolutionLowestBandwidth)
		require.Error(t, err)
	})

	t.Run("GetStreamingPreferenceHistory converts returned versions", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.StreamingPreferences)
			m := &models.StreamingPreferences{Username: "alice", DefaultQuality: "720p", Version: 1, UpdatedAt: time.Now().Add(-time.Hour)}
			m.SetVersionedPreference()
			*dest = []*models.StreamingPreferences{m}
		}).Once()

		repo := NewStreamingRepository(mockDB, "table", zap.NewNop(), &mockDeviceProvider{}, nil)
		history, err := repo.GetStreamingPreferenceHistory(ctx, "alice", 1)
		require.NoError(t, err)
		require.Len(t, history, 1)
		assert.Equal(t, "720p", history[0].DefaultQuality)
	})
}

func TestStreamingRepository_Round08_SyncIgnoresDeviceUpdateErrors(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// GetStreamingPreferences (current) + GetStreamingPreferencesByDevice (device override).
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Times(3)
	mockDB.On("Model", mock.Anything).Return(mockQuery).Times(3)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.StreamingPreferences)
		*dest = models.StreamingPreferences{Username: "alice", DefaultQuality: "480p", Version: 1, UpdatedAt: time.Now().Add(-time.Hour)}
		dest.SetCurrentPreference()
	}).Once()
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.StreamingPreferences)
		*dest = models.StreamingPreferences{Username: "alice", DeviceID: "d1", DefaultQuality: "720p", Version: 2, UpdatedAt: time.Now().Add(-time.Minute)}
		dest.SetDevicePreference("d1")
	}).Once()

	// UpdateDeviceStreamingPreferences for another device attempts ValidateAndCreate -> Create and fails.
	mockQuery.On("Create").Return(assert.AnError).Once()

	provider := &mockDeviceProvider{}
	provider.On("GetUserDevices", mock.Anything, "alice").Return([]*storage.Device{
		{DeviceID: "d1"},
		{DeviceID: "d2"},
	}, nil).Once()

	repo := NewStreamingRepository(mockDB, "table", zap.NewNop(), provider, nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	err := repo.SyncStreamingPreferences(ctx, "alice", "d1")
	require.NoError(t, err)
}
