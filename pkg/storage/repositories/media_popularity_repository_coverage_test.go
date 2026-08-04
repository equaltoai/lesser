package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================================================
// UpsertPopularity Tests
// ============================================================================

func TestUpsertPopularity_MissingMediaID(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	popularity := &models.MediaPopularity{
		MediaID: "", // Missing
		Period:  "WEEK",
	}

	err := repo.UpsertPopularity(ctx, popularity)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mediaID")
}

func TestUpsertPopularity_CreatePath_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	popularity := &models.MediaPopularity{
		MediaID:   "media-123",
		Period:    "WEEK",
		ViewCount: 100,
	}

	// Manually set keys
	popularity.SetForPeriod("media-123", "WEEK", 100)

	// Get returns not found -> triggers create path
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaPopularity")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", popularity.PK).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", popularity.SK).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaPopularity")).Return(errors.ErrItemNotFound)

	// Create path
	mockQuery.On("Create").Return(nil)

	err := repo.UpsertPopularity(ctx, popularity)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestUpsertPopularity_CreatePath_ValidationError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	popularity := &models.MediaPopularity{
		MediaID:   "media-123",
		Period:    "WEEK",
		ViewCount: 100,
	}
	popularity.SetForPeriod("media-123", "WEEK", 100)

	// Get returns not found -> triggers create path
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaPopularity")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", popularity.PK).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", popularity.SK).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaPopularity")).Return(errors.ErrItemNotFound)

	// Create fails
	mockQuery.On("Create").Return(ErrTestMockError)

	err := repo.UpsertPopularity(ctx, popularity)
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestUpsertPopularity_RealGetError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	popularity := &models.MediaPopularity{
		MediaID: "media-123",
		Period:  "WEEK",
	}
	popularity.SetForPeriod("media-123", "WEEK", 100)

	// Get returns a real error (not ErrItemNotFound)
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaPopularity")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", popularity.PK).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", popularity.SK).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaPopularity")).Return(ErrTestMockError)

	err := repo.UpsertPopularity(ctx, popularity)
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestUpsertPopularity_UpdatePath_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	popularity := &models.MediaPopularity{
		MediaID:   "media-123",
		Period:    "WEEK",
		ViewCount: 150,
		QualityViews: map[string]int64{
			"1080p": 50,
		},
	}
	popularity.SetForPeriod("media-123", "WEEK", 150)

	// Existing record with different values
	existingRecord := &models.MediaPopularity{
		PK:        popularity.PK,
		SK:        popularity.SK,
		MediaID:   "media-123",
		Period:    "WEEK",
		ViewCount: 100,
		QualityViews: map[string]int64{
			"720p": 30,
		},
	}

	// Get returns existing record -> triggers update path
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaPopularity")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", popularity.PK).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", popularity.SK).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaPopularity")).Run(func(args mock.Arguments) {
		target := args.Get(0).(*models.MediaPopularity)
		*target = *existingRecord
	}).Return(nil)

	// Update path
	mockQuery.On("Update", mock.Anything).Return(nil)

	err := repo.UpsertPopularity(ctx, popularity)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestUpsertPopularity_UpdatePath_Error(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	popularity := &models.MediaPopularity{
		MediaID:   "media-123",
		Period:    "DAY",
		ViewCount: 50,
	}
	popularity.SetForPeriod("media-123", "DAY", 50)

	existingRecord := &models.MediaPopularity{
		PK:        popularity.PK,
		SK:        popularity.SK,
		MediaID:   "media-123",
		Period:    "DAY",
		ViewCount: 25,
	}

	// Get returns existing record -> triggers update path
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaPopularity")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", popularity.PK).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", popularity.SK).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaPopularity")).Run(func(args mock.Arguments) {
		target := args.Get(0).(*models.MediaPopularity)
		*target = *existingRecord
	}).Return(nil)

	// Update fails
	mockQuery.On("Update", mock.Anything).Return(ErrTestMockError)

	err := repo.UpsertPopularity(ctx, popularity)
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// TTL Branch Tests for UpsertPopularity (update path)
// ============================================================================

func TestUpsertPopularity_TTLBranches(t *testing.T) {
	tests := []struct {
		name     string
		period   string
		expected time.Duration
	}{
		{
			name:     "DAY period sets 7 day TTL",
			period:   "DAY",
			expected: 7 * 24 * time.Hour,
		},
		{
			name:     "WEEK period sets 30 day TTL",
			period:   "WEEK",
			expected: 30 * 24 * time.Hour,
		},
		{
			name:     "MONTH period sets 90 day TTL",
			period:   "MONTH",
			expected: 90 * 24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			getQuery := new(mocks.MockQuery)
			updateQuery := new(mocks.MockQuery)
			repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
			ctx := context.Background()

			popularity := &models.MediaPopularity{
				MediaID:   "media-123",
				Period:    tt.period,
				ViewCount: 100,
			}
			popularity.SetForPeriod("media-123", tt.period, 100)

			existingRecord := &models.MediaPopularity{
				PK:        popularity.PK,
				SK:        popularity.SK,
				MediaID:   "media-123",
				Period:    tt.period,
				ViewCount: 50,
			}

			mockDB.On("WithContext", ctx).Return(mockDB)
			mockDB.On("Model", mock.AnythingOfType("*models.MediaPopularity")).Return(getQuery).Once()
			getQuery.On("Where", "PK", "=", popularity.PK).Return(getQuery)
			getQuery.On("Where", "SK", "=", popularity.SK).Return(getQuery)
			getQuery.On("First", mock.AnythingOfType("*models.MediaPopularity")).Run(func(args mock.Arguments) {
				target := args.Get(0).(*models.MediaPopularity)
				*target = *existingRecord
			}).Return(nil)

			var updated *models.MediaPopularity
			mockDB.On("Model", mock.AnythingOfType("*models.MediaPopularity")).Return(updateQuery).Run(func(args mock.Arguments) {
				updated = args.Get(0).(*models.MediaPopularity)
			}).Once()
			updateQuery.On("Update", mock.Anything).Return(nil)

			err := repo.UpsertPopularity(ctx, popularity)
			require.NoError(t, err)

			// Verify TTL was set correctly (within a reasonable time window)
			expectedTTL := time.Now().Add(tt.expected).Unix()
			require.NotNil(t, updated)
			assert.InDelta(t, expectedTTL, updated.TTL, 60, "TTL should be approximately correct")

			mockDB.AssertExpectations(t)
			getQuery.AssertExpectations(t)
			updateQuery.AssertExpectations(t)
		})
	}
}

// ============================================================================
// GetPopularMediaByPeriod Tests
// ============================================================================

func TestGetPopularMediaByPeriod_DefaultLimit(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	// When limit < 1, should default to 10
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaPopularity")).Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "PERIOD#WEEK").Return(mockQuery)
	mockQuery.On("Limit", 11).Return(mockQuery) // 10 + 1 for pagination
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.MediaPopularity")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]*models.MediaPopularity)
		*records = []*models.MediaPopularity{}
	}).Return(nil)

	result, err := repo.GetPopularMediaByPeriod(ctx, "WEEK", 0, nil)
	require.NoError(t, err)
	assert.Empty(t, result)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetPopularMediaByPeriod_WithCursor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	cursor := "cursor-value-123"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaPopularity")).Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "PERIOD#DAY").Return(mockQuery)
	mockQuery.On("Limit", 21).Return(mockQuery) // 20 + 1
	mockQuery.On("Cursor", cursor).Return(mockQuery)
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.MediaPopularity")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]*models.MediaPopularity)
		*records = []*models.MediaPopularity{
			{MediaID: "media-1", Period: "DAY"},
			{MediaID: "media-2", Period: "DAY"},
		}
	}).Return(nil)

	result, err := repo.GetPopularMediaByPeriod(ctx, "DAY", 20, &cursor)
	require.NoError(t, err)
	assert.Len(t, result, 2)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetPopularMediaByPeriod_NotFoundReturnsEmptySlice(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaPopularity")).Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "PERIOD#MONTH").Return(mockQuery)
	mockQuery.On("Limit", 6).Return(mockQuery)
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.MediaPopularity")).Return(errors.ErrItemNotFound)

	result, err := repo.GetPopularMediaByPeriod(ctx, "MONTH", 5, nil)
	require.NoError(t, err)
	assert.Empty(t, result)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetPopularMediaByPeriod_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaPopularity")).Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "PERIOD#WEEK").Return(mockQuery)
	mockQuery.On("Limit", 11).Return(mockQuery)
	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.MediaPopularity")).Return(ErrTestMockError)

	result, err := repo.GetPopularMediaByPeriod(ctx, "WEEK", 10, nil)
	require.Error(t, err)
	assert.Nil(t, result)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// GetPopularityForMedia Tests
// ============================================================================

func TestGetPopularityForMedia_MissingMediaID(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	result, err := repo.GetPopularityForMedia(ctx, "", "WEEK")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "mediaID")
}

func TestGetPopularityForMedia_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	expectedPK := "MEDIA_POPULARITY#WEEK"
	expectedSK := "MEDIA#media-123"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaPopularity")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", expectedPK).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", expectedSK).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaPopularity")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.MediaPopularity)
		record.MediaID = "media-123"
		record.Period = "WEEK"
		record.ViewCount = 500
	}).Return(nil)

	result, err := repo.GetPopularityForMedia(ctx, "media-123", "WEEK")
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "media-123", result.MediaID)
	assert.Equal(t, int64(500), result.ViewCount)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetPopularityForMedia_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaPopularity")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MEDIA_POPULARITY#DAY").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "MEDIA#media-not-found").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaPopularity")).Return(errors.ErrItemNotFound)

	result, err := repo.GetPopularityForMedia(ctx, "media-not-found", "DAY")
	require.Error(t, err)
	assert.Nil(t, result)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetPopularityForMedia_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaPopularity")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "MEDIA_POPULARITY#MONTH").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "MEDIA#media-error").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaPopularity")).Return(ErrTestMockError)

	result, err := repo.GetPopularityForMedia(ctx, "media-error", "MONTH")
	require.Error(t, err)
	assert.Nil(t, result)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// IncrementViewCount Tests
// ============================================================================

func TestIncrementViewCount_MissingMediaID(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	err := repo.IncrementViewCount(ctx, "", "WEEK", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mediaID")
}

func TestIncrementViewCount_ExistingRecord(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	expectedPK := "MEDIA_POPULARITY#WEEK"
	expectedSK := "MEDIA#media-123"

	// First call: GetPopularityForMedia (finds record)
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaPopularity")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", expectedPK).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", expectedSK).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaPopularity")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.MediaPopularity)
		record.PK = expectedPK
		record.SK = expectedSK
		record.MediaID = "media-123"
		record.Period = "WEEK"
		record.ViewCount = 100
	}).Return(nil).Once()

	// Second call: UpsertPopularity -> Get existing
	mockQuery.On("First", mock.AnythingOfType("*models.MediaPopularity")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.MediaPopularity)
		record.PK = expectedPK
		record.SK = expectedSK
		record.MediaID = "media-123"
		record.Period = "WEEK"
		record.ViewCount = 100
	}).Return(nil).Once()

	// Update call
	mockQuery.On("Update", mock.Anything).Return(nil)

	err := repo.IncrementViewCount(ctx, "media-123", "WEEK", 50)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestIncrementViewCount_GetError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaPopularityRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaPopularity")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", mock.Anything).Return(mockQuery)
	// GetPopularityForMedia returns not found error (wrapped)
	mockQuery.On("First", mock.AnythingOfType("*models.MediaPopularity")).Return(errors.ErrItemNotFound)

	// When GetPopularityForMedia returns wrapped not-found error,
	// IncrementViewCount should return the error from GetPopularityForMedia
	err := repo.IncrementViewCount(ctx, "media-123", "WEEK", 10)
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
