package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestListStatusesForAdmin(t *testing.T) {
	tests := []struct {
		name           string
		filter         *interfaces.StatusFilter
		mockSetup      func(*mocks.MockDB, *mocks.MockQuery)
		expectedCount  int
		expectedCursor string
		expectedError  bool
	}{
		{
			name: "filter_by_local",
			filter: &interfaces.StatusFilter{
				Local: boolPtr(true),
			},
			mockSetup: func(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery) {
				mockDB.On("WithContext", mock.Anything).Return(mockDB)
				mockDB.On("Model", &models.Status{}).Return(mockQuery)
				mockQuery.On("Filter", "Deleted", "=", false).Return(mockQuery)
				mockQuery.On("Filter", "AuthorID", "CONTAINS", mock.Anything).Return(mockQuery)
				mockQuery.On("Limit", 10).Return(mockQuery)
				mockQuery.On("Scan", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
					statuses := args.Get(0).(*[]models.Status)
					*statuses = []models.Status{
						{StatusID: "1", AuthorID: "user@test.local"},
						{StatusID: "2", AuthorID: "admin@test.local"},
					}
				}).Return(nil)
			},
			expectedCount: 2,
		},
		{
			name: "filter_by_flagged",
			filter: &interfaces.StatusFilter{
				Flagged: boolPtr(true),
			},
			mockSetup: func(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery) {
				mockDB.On("WithContext", mock.Anything).Return(mockDB)
				mockDB.On("Model", &models.Status{}).Return(mockQuery)
				mockQuery.On("Filter", "Deleted", "=", false).Return(mockQuery)
				mockQuery.On("Filter", "Flagged", "=", true).Return(mockQuery)
				mockQuery.On("Limit", 10).Return(mockQuery)
				mockQuery.On("Scan", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
					statuses := args.Get(0).(*[]models.Status)
					*statuses = []models.Status{
						{StatusID: "3", Flagged: true},
					}
				}).Return(nil)
			},
			expectedCount: 1,
		},
		{
			name: "filter_by_visibility",
			filter: &interfaces.StatusFilter{
				Visibility: "public",
			},
			mockSetup: func(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery) {
				mockDB.On("WithContext", mock.Anything).Return(mockDB)
				mockDB.On("Model", &models.Status{}).Return(mockQuery)
				mockQuery.On("Filter", "Deleted", "=", false).Return(mockQuery)
				mockQuery.On("Filter", "Visibility", "=", "public").Return(mockQuery)
				mockQuery.On("Limit", 10).Return(mockQuery)
				mockQuery.On("Scan", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
					statuses := args.Get(0).(*[]models.Status)
					*statuses = []models.Status{
						{StatusID: "4", Visibility: "public"},
						{StatusID: "5", Visibility: "public"},
						{StatusID: "6", Visibility: "public"},
					}
				}).Return(nil)
			},
			expectedCount: 3,
		},
		{
			name: "filter_by_date_range",
			filter: &interfaces.StatusFilter{
				MinDate: timePtr(time.Now().Add(-24 * time.Hour)),
				MaxDate: timePtr(time.Now()),
			},
			mockSetup: func(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery) {
				mockDB.On("WithContext", mock.Anything).Return(mockDB)
				mockDB.On("Model", &models.Status{}).Return(mockQuery)
				mockQuery.On("Filter", "Deleted", "=", false).Return(mockQuery)
				mockQuery.On("Filter", "PublishedAt", ">=", mock.Anything).Return(mockQuery)
				mockQuery.On("Filter", "PublishedAt", "<=", mock.Anything).Return(mockQuery)
				mockQuery.On("Limit", 10).Return(mockQuery)
				mockQuery.On("Scan", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
					statuses := args.Get(0).(*[]models.Status)
					now := time.Now()
					*statuses = []models.Status{
						{StatusID: "7", PublishedAt: now.Add(-12 * time.Hour)},
						{StatusID: "8", PublishedAt: now.Add(-6 * time.Hour)},
					}
				}).Return(nil)
			},
			expectedCount: 2,
		},
		{
			name: "filter_with_media",
			filter: &interfaces.StatusFilter{
				WithMedia: boolPtr(true),
			},
			mockSetup: func(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery) {
				mockDB.On("WithContext", mock.Anything).Return(mockDB)
				mockDB.On("Model", &models.Status{}).Return(mockQuery)
				mockQuery.On("Filter", "Deleted", "=", false).Return(mockQuery)
				mockQuery.On("Filter", "MediaCount", ">", 0).Return(mockQuery)
				mockQuery.On("Limit", 10).Return(mockQuery)
				mockQuery.On("Scan", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
					statuses := args.Get(0).(*[]models.Status)
					*statuses = []models.Status{
						{StatusID: "9", MediaCount: 2},
						{StatusID: "10", MediaCount: 1},
					}
				}).Return(nil)
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			logger := zap.NewNop()
			repo := NewStatusRepository(mockDB, "test-table", logger, nil)

			tt.mockSetup(mockDB, mockQuery)

			// Execute
			statuses, cursor, err := repo.ListStatusesForAdmin(context.Background(), tt.filter, 10, "")

			// Assert
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, statuses, tt.expectedCount)
				if tt.expectedCursor != "" {
					assert.Equal(t, tt.expectedCursor, cursor)
				}
			}

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

func TestCountStatusesForAdmin(t *testing.T) {
	tests := []struct {
		name          string
		filter        *interfaces.StatusFilter
		mockSetup     func(*mocks.MockDB, *mocks.MockQuery)
		expectedCount int64
		expectedError bool
	}{
		{
			name:   "count_all_non_deleted",
			filter: &interfaces.StatusFilter{},
			mockSetup: func(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery) {
				mockDB.On("WithContext", mock.Anything).Return(mockDB)
				mockDB.On("Model", &models.Status{}).Return(mockQuery)
				mockQuery.On("Filter", "Deleted", "=", false).Return(mockQuery)
				mockQuery.On("Count").Return(int64(100), nil)
			},
			expectedCount: 100,
		},
		{
			name: "count_flagged",
			filter: &interfaces.StatusFilter{
				Flagged: boolPtr(true),
			},
			mockSetup: func(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery) {
				mockDB.On("WithContext", mock.Anything).Return(mockDB)
				mockDB.On("Model", &models.Status{}).Return(mockQuery)
				mockQuery.On("Filter", "Deleted", "=", false).Return(mockQuery)
				mockQuery.On("Filter", "Flagged", "=", true).Return(mockQuery)
				mockQuery.On("Count").Return(int64(15), nil)
			},
			expectedCount: 15,
		},
		{
			name: "count_public_with_media",
			filter: &interfaces.StatusFilter{
				Visibility: "public",
				WithMedia:  boolPtr(true),
			},
			mockSetup: func(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery) {
				mockDB.On("WithContext", mock.Anything).Return(mockDB)
				mockDB.On("Model", &models.Status{}).Return(mockQuery)
				mockQuery.On("Filter", "Deleted", "=", false).Return(mockQuery)
				mockQuery.On("Filter", "Visibility", "=", "public").Return(mockQuery)
				mockQuery.On("Filter", "MediaCount", ">", 0).Return(mockQuery)
				mockQuery.On("Count").Return(int64(42), nil)
			},
			expectedCount: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			logger := zap.NewNop()
			repo := NewStatusRepository(mockDB, "test-table", logger, nil)

			tt.mockSetup(mockDB, mockQuery)

			// Execute
			count, err := repo.CountStatusesForAdmin(context.Background(), tt.filter)

			// Assert
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedCount, count)
			}

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

// Helper functions
func boolPtr(b bool) *bool {
	return &b
}

func timePtr(t time.Time) *time.Time {
	return &t
}
