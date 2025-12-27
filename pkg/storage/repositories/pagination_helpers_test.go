package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Test Types
// ============================================================================

// testPaginatedItem is a minimal implementation of skGetter for testing
type testPaginatedItem struct {
	PK string
	SK string
}

func (t testPaginatedItem) GetSK() string {
	return t.SK
}

// ============================================================================
// Tests: listByPKSKPrefixPaginated
// ============================================================================

func TestListByPKSKPrefixPaginated_DefaultLimit(t *testing.T) {
	tests := []struct {
		name          string
		inputLimit    int
		expectedLimit int
	}{
		{
			name:          "zero_limit_becomes_25",
			inputLimit:    0,
			expectedLimit: 25,
		},
		{
			name:          "negative_limit_becomes_25",
			inputLimit:    -1,
			expectedLimit: 25,
		},
		{
			name:          "negative_100_becomes_25",
			inputLimit:    -100,
			expectedLimit: 25,
		},
		{
			name:          "positive_limit_preserved",
			inputLimit:    10,
			expectedLimit: 10,
		},
		{
			name:          "limit_of_1_preserved",
			inputLimit:    1,
			expectedLimit: 1,
		},
		{
			name:          "large_limit_preserved",
			inputLimit:    100,
			expectedLimit: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			ctx := context.Background()

			pk := "TEST#pk"
			skPrefix := "ITEM#"

			// Set up mock expectations
			mockDB.On("WithContext", ctx).Return(mockDB)
			mockDB.On("Model", mock.Anything).Return(mockQuery)
			mockQuery.On("Where", "PK", "=", pk).Return(mockQuery)
			mockQuery.On("Where", "SK", "BEGINS_WITH", skPrefix).Return(mockQuery)
			mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery)
			// Verify the limit passed is expectedLimit + 1
			mockQuery.On("Limit", tt.expectedLimit+1).Return(mockQuery)
			mockQuery.On("All", mock.AnythingOfType("*[]repositories.testPaginatedItem")).Run(func(args mock.Arguments) {
				// Return empty slice
				items := args.Get(0).(*[]testPaginatedItem)
				*items = []testPaginatedItem{}
			}).Return(nil)

			// Execute
			items, nextCursor, err := listByPKSKPrefixPaginated[testPaginatedItem](ctx, mockDB, &testPaginatedItem{}, pk, skPrefix, tt.inputLimit, "")

			// Assert
			require.NoError(t, err)
			assert.Empty(t, items)
			assert.Empty(t, nextCursor)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

func TestListByPKSKPrefixPaginated_CursorTrimming(t *testing.T) {
	tests := []struct {
		name           string
		cursor         string
		expectCursor   bool
		expectedCursor string
	}{
		{
			name:         "empty_cursor_no_filter",
			cursor:       "",
			expectCursor: false,
		},
		{
			name:           "non_empty_cursor_applies_filter",
			cursor:         "ITEM#123",
			expectCursor:   true,
			expectedCursor: "ITEM#123",
		},
		{
			name:           "whitespace_only_cursor_no_filter",
			cursor:         "   ",
			expectCursor:   false,
			expectedCursor: "",
		},
		{
			name:           "cursor_with_leading_whitespace_trimmed",
			cursor:         "  ITEM#456",
			expectCursor:   true,
			expectedCursor: "ITEM#456",
		},
		{
			name:           "cursor_with_trailing_whitespace_trimmed",
			cursor:         "ITEM#789  ",
			expectCursor:   true,
			expectedCursor: "ITEM#789",
		},
		{
			name:           "cursor_with_both_whitespace_trimmed",
			cursor:         "  ITEM#abc  ",
			expectCursor:   true,
			expectedCursor: "ITEM#abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			ctx := context.Background()

			pk := "TEST#pk"
			skPrefix := "ITEM#"
			limit := 10

			// Set up base mock expectations
			mockDB.On("WithContext", ctx).Return(mockDB)
			mockDB.On("Model", mock.Anything).Return(mockQuery)
			mockQuery.On("Where", "PK", "=", pk).Return(mockQuery)
			mockQuery.On("Where", "SK", "BEGINS_WITH", skPrefix).Return(mockQuery)
			mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery)

			// Only expect cursor filter if cursor is non-empty after trimming
			if tt.expectCursor {
				mockQuery.On("Where", "SK", ">", tt.expectedCursor).Return(mockQuery)
			}

			mockQuery.On("Limit", limit+1).Return(mockQuery)
			mockQuery.On("All", mock.AnythingOfType("*[]repositories.testPaginatedItem")).Run(func(args mock.Arguments) {
				items := args.Get(0).(*[]testPaginatedItem)
				*items = []testPaginatedItem{}
			}).Return(nil)

			// Execute
			items, nextCursor, err := listByPKSKPrefixPaginated[testPaginatedItem](ctx, mockDB, &testPaginatedItem{}, pk, skPrefix, limit, tt.cursor)

			// Assert
			require.NoError(t, err)
			assert.Empty(t, items)
			assert.Empty(t, nextCursor)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

func TestListByPKSKPrefixPaginated_QueryShape(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()

	pk := "USER#testuser"
	skPrefix := "FOLLOW#"
	limit := 5
	cursor := "FOLLOW#someuser"

	// Set up expectations in the correct order
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)

	// Verify exact query chain
	mockQuery.On("Where", "PK", "=", pk).Return(mockQuery)
	mockQuery.On("Where", "SK", "BEGINS_WITH", skPrefix).Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery)
	mockQuery.On("Where", "SK", ">", cursor).Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery) // limit + 1 for pagination detection
	mockQuery.On("All", mock.AnythingOfType("*[]repositories.testPaginatedItem")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]testPaginatedItem)
		*items = []testPaginatedItem{
			{PK: pk, SK: "FOLLOW#user1"},
			{PK: pk, SK: "FOLLOW#user2"},
			{PK: pk, SK: "FOLLOW#user3"},
		}
	}).Return(nil)

	// Execute
	items, nextCursor, err := listByPKSKPrefixPaginated[testPaginatedItem](ctx, mockDB, &testPaginatedItem{}, pk, skPrefix, limit, cursor)

	// Assert
	require.NoError(t, err)
	assert.Len(t, items, 3)
	assert.Empty(t, nextCursor) // No next cursor when items <= limit

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestListByPKSKPrefixPaginated_NextCursorLogic_NoMoreResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()

	pk := "TEST#pk"
	skPrefix := "ITEM#"
	limit := 5

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", pk).Return(mockQuery)
	mockQuery.On("Where", "SK", "BEGINS_WITH", skPrefix).Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]repositories.testPaginatedItem")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]testPaginatedItem)
		// Return fewer items than limit - no more results
		*items = []testPaginatedItem{
			{PK: pk, SK: "ITEM#1"},
			{PK: pk, SK: "ITEM#2"},
			{PK: pk, SK: "ITEM#3"},
		}
	}).Return(nil)

	// Execute
	items, nextCursor, err := listByPKSKPrefixPaginated[testPaginatedItem](ctx, mockDB, &testPaginatedItem{}, pk, skPrefix, limit, "")

	// Assert
	require.NoError(t, err)
	assert.Len(t, items, 3)
	assert.Empty(t, nextCursor, "No cursor when results <= limit")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestListByPKSKPrefixPaginated_NextCursorLogic_HasMoreResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()

	pk := "TEST#pk"
	skPrefix := "ITEM#"
	limit := 3

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", pk).Return(mockQuery)
	mockQuery.On("Where", "SK", "BEGINS_WITH", skPrefix).Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]repositories.testPaginatedItem")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]testPaginatedItem)
		// Return limit + 1 items to trigger cursor generation
		*items = []testPaginatedItem{
			{PK: pk, SK: "ITEM#1"},
			{PK: pk, SK: "ITEM#2"},
			{PK: pk, SK: "ITEM#3"},
			{PK: pk, SK: "ITEM#4"}, // Extra item indicates more results
		}
	}).Return(nil)

	// Execute
	items, nextCursor, err := listByPKSKPrefixPaginated[testPaginatedItem](ctx, mockDB, &testPaginatedItem{}, pk, skPrefix, limit, "")

	// Assert
	require.NoError(t, err)
	assert.Len(t, items, limit, "Results should be trimmed to limit")
	// Next cursor should be items[limit-1].GetSK() = "ITEM#3" (index 2 when limit=3)
	assert.Equal(t, "ITEM#3", nextCursor, "Cursor should be SK of last retained item (items[limit-1])")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestListByPKSKPrefixPaginated_ExactlyLimitResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()

	pk := "TEST#pk"
	skPrefix := "ITEM#"
	limit := 3

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", pk).Return(mockQuery)
	mockQuery.On("Where", "SK", "BEGINS_WITH", skPrefix).Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]repositories.testPaginatedItem")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]testPaginatedItem)
		// Return exactly limit items
		*items = []testPaginatedItem{
			{PK: pk, SK: "ITEM#1"},
			{PK: pk, SK: "ITEM#2"},
			{PK: pk, SK: "ITEM#3"},
		}
	}).Return(nil)

	// Execute
	items, nextCursor, err := listByPKSKPrefixPaginated[testPaginatedItem](ctx, mockDB, &testPaginatedItem{}, pk, skPrefix, limit, "")

	// Assert
	require.NoError(t, err)
	assert.Len(t, items, limit)
	assert.Empty(t, nextCursor, "No cursor when results == limit")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestListByPKSKPrefixPaginated_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()

	pk := "TEST#pk"
	skPrefix := "ITEM#"
	limit := 10
	testErr := errors.New("database connection error")

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", pk).Return(mockQuery)
	mockQuery.On("Where", "SK", "BEGINS_WITH", skPrefix).Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]repositories.testPaginatedItem")).Return(testErr)

	// Execute
	items, nextCursor, err := listByPKSKPrefixPaginated[testPaginatedItem](ctx, mockDB, &testPaginatedItem{}, pk, skPrefix, limit, "")

	// Assert
	require.Error(t, err)
	assert.Equal(t, testErr, err)
	assert.Nil(t, items)
	assert.Empty(t, nextCursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestListByPKSKPrefixPaginated_EmptyResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	ctx := context.Background()

	pk := "TEST#pk"
	skPrefix := "NONEXISTENT#"
	limit := 10

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", pk).Return(mockQuery)
	mockQuery.On("Where", "SK", "BEGINS_WITH", skPrefix).Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]repositories.testPaginatedItem")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]testPaginatedItem)
		*items = []testPaginatedItem{}
	}).Return(nil)

	// Execute
	items, nextCursor, err := listByPKSKPrefixPaginated[testPaginatedItem](ctx, mockDB, &testPaginatedItem{}, pk, skPrefix, limit, "")

	// Assert
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Empty(t, nextCursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
