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
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================================================
// Tests: clampDomainLimit
// ============================================================================

func TestClampDomainLimit(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{
			name:     "zero_becomes_default_20",
			input:    0,
			expected: 20,
		},
		{
			name:     "negative_becomes_default_20",
			input:    -1,
			expected: 20,
		},
		{
			name:     "negative_100_becomes_default_20",
			input:    -100,
			expected: 20,
		},
		{
			name:     "1_preserved",
			input:    1,
			expected: 1,
		},
		{
			name:     "20_preserved",
			input:    20,
			expected: 20,
		},
		{
			name:     "50_preserved",
			input:    50,
			expected: 50,
		},
		{
			name:     "100_preserved_at_max",
			input:    100,
			expected: 100,
		},
		{
			name:     "101_clamped_to_100",
			input:    101,
			expected: 100,
		},
		{
			name:     "1000_clamped_to_100",
			input:    1000,
			expected: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clampDomainLimit(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// Tests: generateNextCursor
// ============================================================================

func TestGenerateNextCursor(t *testing.T) {
	tests := []struct {
		name           string
		resultsLen     int
		requestedLimit int
		gsiSKValue     string
		expectedCursor string
	}{
		{
			name:           "more_results_than_limit_returns_cursor",
			resultsLen:     6,
			requestedLimit: 5,
			gsiSKValue:     "2024-01-01T12:00:00Z",
			expectedCursor: "2024-01-01T12:00:00Z",
		},
		{
			name:           "results_equal_limit_returns_empty",
			resultsLen:     5,
			requestedLimit: 5,
			gsiSKValue:     "2024-01-01T12:00:00Z",
			expectedCursor: "",
		},
		{
			name:           "fewer_results_than_limit_returns_empty",
			resultsLen:     3,
			requestedLimit: 5,
			gsiSKValue:     "2024-01-01T12:00:00Z",
			expectedCursor: "",
		},
		{
			name:           "zero_requested_limit_returns_empty",
			resultsLen:     10,
			requestedLimit: 0,
			gsiSKValue:     "2024-01-01T12:00:00Z",
			expectedCursor: "",
		},
		{
			name:           "negative_requested_limit_returns_empty",
			resultsLen:     10,
			requestedLimit: -5,
			gsiSKValue:     "2024-01-01T12:00:00Z",
			expectedCursor: "",
		},
		{
			name:           "empty_results_returns_empty",
			resultsLen:     0,
			requestedLimit: 5,
			gsiSKValue:     "2024-01-01T12:00:00Z",
			expectedCursor: "",
		},
		{
			name:           "one_more_than_limit_returns_cursor",
			resultsLen:     2,
			requestedLimit: 1,
			gsiSKValue:     "cursor-value",
			expectedCursor: "cursor-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getGSI1SK := func() string {
				return tt.gsiSKValue
			}
			result := generateNextCursor(tt.resultsLen, tt.requestedLimit, getGSI1SK)
			assert.Equal(t, tt.expectedCursor, result)
		})
	}
}

// ============================================================================
// Tests: buildPaginationQuery
// ============================================================================

func TestBuildPaginationQuery_NoCursor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()

	config := DomainPaginationConfig{
		GSIPKValue:  "EMAIL_DOMAIN_BLOCKS",
		ErrorPrefix: "email domain blocks",
	}
	limit := 10

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmailDomainBlock")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", config.GSIPKValue).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)

	// Execute
	query, safeLimit := buildPaginationQuery(ctx, mockDB, limit, "", config, &models.EmailDomainBlock{})

	// Assert
	assert.NotNil(t, query)
	assert.Equal(t, limit, safeLimit)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBuildPaginationQuery_WithCursor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()

	config := DomainPaginationConfig{
		GSIPKValue:  "DOMAIN_ALLOWS",
		ErrorPrefix: "domain allows",
	}
	limit := 20
	cursor := "2024-01-15T10:30:00Z"

	// Set up expectations including cursor filter
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.DomainAllow")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", config.GSIPKValue).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", "<", cursor).Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)

	// Execute
	query, safeLimit := buildPaginationQuery(ctx, mockDB, limit, cursor, config, &models.DomainAllow{})

	// Assert
	assert.NotNil(t, query)
	assert.Equal(t, limit, safeLimit)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBuildPaginationQuery_LimitClamping(t *testing.T) {
	tests := []struct {
		name          string
		inputLimit    int
		expectedLimit int
	}{
		{
			name:          "zero_becomes_default",
			inputLimit:    0,
			expectedLimit: 20,
		},
		{
			name:          "over_max_clamped",
			inputLimit:    200,
			expectedLimit: 100,
		},
		{
			name:          "normal_preserved",
			inputLimit:    50,
			expectedLimit: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			ctx := context.Background()

			config := DomainPaginationConfig{
				GSIPKValue:  "TEST",
				ErrorPrefix: "test",
			}

			mockDB.On("WithContext", ctx).Return(mockDB)
			mockDB.On("Model", mock.Anything).Return(mockQuery)
			mockQuery.On("Index", "gsi1").Return(mockQuery)
			mockQuery.On("Where", "gsi1PK", "=", config.GSIPKValue).Return(mockQuery)
			mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
			mockQuery.On("Limit", tt.expectedLimit+1).Return(mockQuery)

			// Execute
			_, safeLimit := buildPaginationQuery(ctx, mockDB, tt.inputLimit, "", config, &models.EmailDomainBlock{})

			// Assert
			assert.Equal(t, tt.expectedLimit, safeLimit)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

// ============================================================================
// Tests: getPaginatedEmailDomainBlocks (end-to-end with mocks)
// ============================================================================

func TestGetPaginatedEmailDomainBlocks_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()
	logger := zap.NewNop()

	config := DomainPaginationConfig{
		GSIPKValue:  "EMAIL_DOMAIN_BLOCKS",
		ErrorPrefix: "email domain blocks",
	}
	limit := 2
	createdAt := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmailDomainBlock")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", config.GSIPKValue).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.EmailDomainBlock")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]models.EmailDomainBlock)
		*items = []models.EmailDomainBlock{
			{
				ID:        "id-1",
				Domain:    "spam.com",
				CreatedBy: "admin1",
				CreatedAt: createdAt,
				GSI1SK:    createdAt.Format(time.RFC3339),
			},
			{
				ID:        "id-2",
				Domain:    "evil.org",
				CreatedBy: "admin2",
				CreatedAt: createdAt.Add(-1 * time.Hour),
				GSI1SK:    createdAt.Add(-1 * time.Hour).Format(time.RFC3339),
			},
		}
	}).Return(nil)

	// Execute
	results, nextCursor, err := getPaginatedEmailDomainBlocks(ctx, mockDB, logger, limit, "", config)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Empty(t, nextCursor, "No cursor when results <= limit")

	// Verify conversion preserves fields
	assert.Equal(t, "id-1", results[0].ID)
	assert.Equal(t, "spam.com", results[0].Domain)
	assert.Equal(t, "admin1", results[0].CreatedBy)
	assert.Equal(t, createdAt, results[0].CreatedAt)

	assert.Equal(t, "id-2", results[1].ID)
	assert.Equal(t, "evil.org", results[1].Domain)
	assert.Equal(t, "admin2", results[1].CreatedBy)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetPaginatedEmailDomainBlocks_WithPagination(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()
	logger := zap.NewNop()

	config := DomainPaginationConfig{
		GSIPKValue:  "EMAIL_DOMAIN_BLOCKS",
		ErrorPrefix: "email domain blocks",
	}
	limit := 2
	createdAt := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmailDomainBlock")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", config.GSIPKValue).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.EmailDomainBlock")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]models.EmailDomainBlock)
		// Return limit+1 items to trigger pagination
		*items = []models.EmailDomainBlock{
			{
				ID:        "id-1",
				Domain:    "spam.com",
				CreatedBy: "admin1",
				CreatedAt: createdAt,
				GSI1SK:    createdAt.Format(time.RFC3339),
			},
			{
				ID:        "id-2",
				Domain:    "evil.org",
				CreatedBy: "admin2",
				CreatedAt: createdAt.Add(-1 * time.Hour),
				GSI1SK:    createdAt.Add(-1 * time.Hour).Format(time.RFC3339),
			},
			{
				ID:        "id-3",
				Domain:    "bad.net",
				CreatedBy: "admin3",
				CreatedAt: createdAt.Add(-2 * time.Hour),
				GSI1SK:    createdAt.Add(-2 * time.Hour).Format(time.RFC3339),
			},
		}
	}).Return(nil)

	// Execute
	results, nextCursor, err := getPaginatedEmailDomainBlocks(ctx, mockDB, logger, limit, "", config)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, limit, "Results should be trimmed to limit")
	// Cursor should be GSI1SK of items[safeLimit-1]
	expectedCursor := createdAt.Add(-1 * time.Hour).Format(time.RFC3339)
	assert.Equal(t, expectedCursor, nextCursor)

	// Verify only first 2 items are returned
	assert.Equal(t, "id-1", results[0].ID)
	assert.Equal(t, "id-2", results[1].ID)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetPaginatedEmailDomainBlocks_WithCursor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()
	logger := zap.NewNop()

	config := DomainPaginationConfig{
		GSIPKValue:  "EMAIL_DOMAIN_BLOCKS",
		ErrorPrefix: "email domain blocks",
	}
	limit := 2
	inputCursor := "2024-01-15T09:00:00Z"
	createdAt := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)

	// Set up expectations including cursor filter
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmailDomainBlock")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", config.GSIPKValue).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", "<", inputCursor).Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.EmailDomainBlock")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]models.EmailDomainBlock)
		*items = []models.EmailDomainBlock{
			{
				ID:        "id-older",
				Domain:    "older.com",
				CreatedBy: "admin",
				CreatedAt: createdAt,
				GSI1SK:    createdAt.Format(time.RFC3339),
			},
		}
	}).Return(nil)

	// Execute
	results, nextCursor, err := getPaginatedEmailDomainBlocks(ctx, mockDB, logger, limit, inputCursor, config)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Empty(t, nextCursor)
	assert.Equal(t, "id-older", results[0].ID)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetPaginatedEmailDomainBlocks_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()
	logger := zap.NewNop()

	config := DomainPaginationConfig{
		GSIPKValue:  "EMAIL_DOMAIN_BLOCKS",
		ErrorPrefix: "email domain blocks",
	}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmailDomainBlock")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", config.GSIPKValue).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.EmailDomainBlock")).Return(ErrTestMockError)

	// Execute
	results, nextCursor, err := getPaginatedEmailDomainBlocks(ctx, mockDB, logger, 10, "", config)

	// Assert
	require.Error(t, err)
	assert.Nil(t, results)
	assert.Empty(t, nextCursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetPaginatedEmailDomainBlocks_EmptyResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()
	logger := zap.NewNop()

	config := DomainPaginationConfig{
		GSIPKValue:  "EMAIL_DOMAIN_BLOCKS",
		ErrorPrefix: "email domain blocks",
	}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmailDomainBlock")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", config.GSIPKValue).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.EmailDomainBlock")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]models.EmailDomainBlock)
		*items = []models.EmailDomainBlock{}
	}).Return(nil)

	// Execute
	results, nextCursor, err := getPaginatedEmailDomainBlocks(ctx, mockDB, logger, 10, "", config)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, results)
	assert.Empty(t, nextCursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// Tests: getPaginatedDomainAllows (end-to-end with mocks)
// ============================================================================

func TestGetPaginatedDomainAllows_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()
	logger := zap.NewNop()

	config := DomainPaginationConfig{
		GSIPKValue:  "DOMAIN_ALLOWS",
		ErrorPrefix: "domain allows",
	}
	limit := 2
	createdAt := time.Date(2024, 6, 20, 14, 30, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.DomainAllow")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", config.GSIPKValue).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.DomainAllow")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]models.DomainAllow)
		*items = []models.DomainAllow{
			{
				ID:        "allow-1",
				Domain:    "trusted.org",
				CreatedBy: "admin1",
				CreatedAt: createdAt,
				GSI1SK:    createdAt.Format(time.RFC3339),
			},
			{
				ID:        "allow-2",
				Domain:    "partner.com",
				CreatedBy: "admin2",
				CreatedAt: createdAt.Add(-1 * time.Hour),
				GSI1SK:    createdAt.Add(-1 * time.Hour).Format(time.RFC3339),
			},
		}
	}).Return(nil)

	// Execute
	results, nextCursor, err := getPaginatedDomainAllows(ctx, mockDB, logger, limit, "", config)

	// Assert
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Empty(t, nextCursor)

	// Verify conversion preserves fields
	assert.Equal(t, "allow-1", results[0].ID)
	assert.Equal(t, "trusted.org", results[0].Domain)
	assert.Equal(t, "admin1", results[0].CreatedBy)
	assert.Equal(t, createdAt, results[0].CreatedAt)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// Tests: genericDeleteByID
// ============================================================================

// testDomainItem is a minimal implementation of DomainItem for testing
type testDomainItem struct {
	ID string
	PK string
	SK string
}

func (t *testDomainItem) GetID() string { return t.ID }
func (t *testDomainItem) GetPK() string { return t.PK }
func (t *testDomainItem) GetSK() string { return t.SK }

func TestGenericDeleteByID_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()
	logger := zap.NewNop()

	gsipkValue := "TEST_DOMAIN_ITEMS"
	targetID := "nonexistent-id"

	// Set up expectations for GSI query
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmailDomainBlock")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", gsipkValue).Return(mockQuery)
	mockQuery.On("Limit", 100).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.EmailDomainBlock")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]*models.EmailDomainBlock)
		// Return items that don't match the target ID
		*items = []*models.EmailDomainBlock{
			{ID: "other-id-1", PK: "PK1", SK: "SK1"},
			{ID: "other-id-2", PK: "PK2", SK: "SK2"},
		}
	}).Return(nil)

	// Execute
	err := genericDeleteByID(ctx, mockDB, logger, targetID, gsipkValue, &models.EmailDomainBlock{})

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, storage.ErrNotFound)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGenericDeleteByID_Found_DeletesItem(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	deleteQuery := new(mocks.MockQuery)
	ctx := context.Background()
	logger := zap.NewNop()

	gsipkValue := "EMAIL_DOMAIN_BLOCKS"
	targetID := "target-id"
	targetPK := "EMAIL_DOMAIN_BLOCK#target.com"
	targetSK := "EMAIL_DOMAIN_BLOCK#target.com"

	// Set up expectations for GSI query
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmailDomainBlock")).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", gsipkValue).Return(mockQuery)
	mockQuery.On("Limit", 100).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.EmailDomainBlock")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]*models.EmailDomainBlock)
		*items = []*models.EmailDomainBlock{
			{ID: "other-id", PK: "OTHER_PK", SK: "OTHER_SK"},
			{ID: targetID, PK: targetPK, SK: targetSK}, // This is the target
			{ID: "another-id", PK: "ANOTHER_PK", SK: "ANOTHER_SK"},
		}
	}).Return(nil)

	// Set up expectations for delete
	mockDB.On("Model", mock.AnythingOfType("*models.EmailDomainBlock")).Return(deleteQuery).Once()
	deleteQuery.On("Where", "PK", "=", targetPK).Return(deleteQuery)
	deleteQuery.On("Where", "SK", "=", targetSK).Return(deleteQuery)
	deleteQuery.On("Delete").Return(nil)

	// Execute
	err := genericDeleteByID(ctx, mockDB, logger, targetID, gsipkValue, &models.EmailDomainBlock{})

	// Assert
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	deleteQuery.AssertExpectations(t)
}

func TestGenericDeleteByID_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()
	logger := zap.NewNop()

	gsipkValue := "EMAIL_DOMAIN_BLOCKS"
	targetID := "target-id"

	// Set up expectations for GSI query that fails
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmailDomainBlock")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", gsipkValue).Return(mockQuery)
	mockQuery.On("Limit", 100).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.EmailDomainBlock")).Return(ErrTestMockError)

	// Execute
	err := genericDeleteByID(ctx, mockDB, logger, targetID, gsipkValue, &models.EmailDomainBlock{})

	// Assert
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGenericDeleteByID_DeleteError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	deleteQuery := new(mocks.MockQuery)
	ctx := context.Background()
	logger := zap.NewNop()

	gsipkValue := "EMAIL_DOMAIN_BLOCKS"
	targetID := "target-id"
	targetPK := "EMAIL_DOMAIN_BLOCK#target.com"
	targetSK := "EMAIL_DOMAIN_BLOCK#target.com"

	// Set up expectations for GSI query
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmailDomainBlock")).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", gsipkValue).Return(mockQuery)
	mockQuery.On("Limit", 100).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.EmailDomainBlock")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]*models.EmailDomainBlock)
		*items = []*models.EmailDomainBlock{
			{ID: targetID, PK: targetPK, SK: targetSK},
		}
	}).Return(nil)

	// Set up expectations for delete that fails
	mockDB.On("Model", mock.AnythingOfType("*models.EmailDomainBlock")).Return(deleteQuery).Once()
	deleteQuery.On("Where", "PK", "=", targetPK).Return(deleteQuery)
	deleteQuery.On("Where", "SK", "=", targetSK).Return(deleteQuery)
	deleteQuery.On("Delete").Return(ErrTestMockError)

	// Execute
	err := genericDeleteByID(ctx, mockDB, logger, targetID, gsipkValue, &models.EmailDomainBlock{})

	// Assert
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	deleteQuery.AssertExpectations(t)
}

func TestGenericDeleteByID_EmptyResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()
	logger := zap.NewNop()

	gsipkValue := "EMAIL_DOMAIN_BLOCKS"
	targetID := "target-id"

	// Set up expectations for GSI query returning empty
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.EmailDomainBlock")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", gsipkValue).Return(mockQuery)
	mockQuery.On("Limit", 100).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.EmailDomainBlock")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]*models.EmailDomainBlock)
		*items = []*models.EmailDomainBlock{}
	}).Return(nil)

	// Execute
	err := genericDeleteByID(ctx, mockDB, logger, targetID, gsipkValue, &models.EmailDomainBlock{})

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, storage.ErrNotFound)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// Tests: Converter implementations
// ============================================================================

func TestEmailDomainBlockConverter_Convert(t *testing.T) {
	converter := EmailDomainBlockConverter{}
	createdAt := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)

	model := models.EmailDomainBlock{
		ID:        "test-id",
		Domain:    "blocked.com",
		CreatedBy: "admin@example.com",
		CreatedAt: createdAt,
		GSI1SK:    createdAt.Format(time.RFC3339),
	}

	result := converter.Convert(model)

	assert.Equal(t, "test-id", result.ID)
	assert.Equal(t, "blocked.com", result.Domain)
	assert.Equal(t, "admin@example.com", result.CreatedBy)
	assert.Equal(t, createdAt, result.CreatedAt)
}

func TestEmailDomainBlockConverter_GetGSI1SK(t *testing.T) {
	converter := EmailDomainBlockConverter{}
	createdAt := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)

	model := models.EmailDomainBlock{
		GSI1SK: createdAt.Format(time.RFC3339),
	}

	result := converter.GetGSI1SK(model)

	assert.Equal(t, createdAt.Format(time.RFC3339), result)
}

func TestDomainAllowConverter_Convert(t *testing.T) {
	converter := DomainAllowConverter{}
	createdAt := time.Date(2024, 5, 20, 14, 0, 0, 0, time.UTC)

	model := models.DomainAllow{
		ID:        "allow-id",
		Domain:    "allowed.org",
		CreatedBy: "moderator",
		CreatedAt: createdAt,
		GSI1SK:    createdAt.Format(time.RFC3339),
	}

	result := converter.Convert(model)

	assert.Equal(t, "allow-id", result.ID)
	assert.Equal(t, "allowed.org", result.Domain)
	assert.Equal(t, "moderator", result.CreatedBy)
	assert.Equal(t, createdAt, result.CreatedAt)
}

func TestDomainAllowConverter_GetGSI1SK(t *testing.T) {
	converter := DomainAllowConverter{}
	createdAt := time.Date(2024, 5, 20, 14, 0, 0, 0, time.UTC)

	model := models.DomainAllow{
		GSI1SK: createdAt.Format(time.RFC3339),
	}

	result := converter.GetGSI1SK(model)

	assert.Equal(t, createdAt.Format(time.RFC3339), result)
}
