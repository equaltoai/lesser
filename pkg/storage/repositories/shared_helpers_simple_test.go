package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
)

// ============================================================================
// NormalizePaginationLimit Tests
// ============================================================================

func TestNormalizePaginationLimit(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		// Valid values - should return input unchanged
		{
			name:     "valid limit of 20",
			input:    20,
			expected: 20,
		},
		{
			name:     "valid limit of 1",
			input:    1,
			expected: 1,
		},
		{
			name:     "valid limit at max (100)",
			input:    100,
			expected: 100,
		},
		{
			name:     "valid limit of 50",
			input:    50,
			expected: 50,
		},
		{
			name:     "zero limit is valid (returns 0)",
			input:    0,
			expected: 0,
		},

		// Invalid values - should return default 20
		{
			name:     "negative limit returns default",
			input:    -1,
			expected: 20,
		},
		{
			name:     "limit exceeding max (101) returns default",
			input:    101,
			expected: 20,
		},
		{
			name:     "very large limit returns default",
			input:    1000,
			expected: 20,
		},
		{
			name:     "large negative limit returns default",
			input:    -100,
			expected: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizePaginationLimit(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// AuditLogQueryHelper Tests (with DynamORM mocks)
// ============================================================================

func TestAuditLogQueryHelper_QueryChainInvocation(t *testing.T) {
	t.Run("basic query without time range", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		// Setup mock chain
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", "user").Return(mockQuery)
		mockQuery.On("Where", "userPK", "=", "USER#test123").Return(mockQuery)
		mockQuery.On("Limit", 10).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil)

		ctx := context.Background()
		_, err := AuditLogQueryHelper(
			ctx,
			mockDB,
			"USER",
			"USER#test123",
			10,
			time.Time{}, // zero time - no range filter
			time.Time{}, // zero time - no range filter
			"user",
		)

		require.NoError(t, err)

		// Verify all expectations were met
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("query with time range filter", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		endTime := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

		// Setup mock chain - note time range adds two Where clauses
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", "session").Return(mockQuery)
		mockQuery.On("Where", "sessionPK", "=", "SESSION#abc").Return(mockQuery)
		mockQuery.On("Where", "sessionSK", ">=", mock.MatchedBy(func(s string) bool {
			return s == "AUDIT#1704067200" // 2024-01-01 00:00:00 UTC
		})).Return(mockQuery)
		mockQuery.On("Where", "sessionSK", "<=", mock.MatchedBy(func(s string) bool {
			return s == "AUDIT#1706745599" // 2024-01-31 23:59:59 UTC
		})).Return(mockQuery)
		mockQuery.On("Limit", 25).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil)

		ctx := context.Background()
		_, err := AuditLogQueryHelper(
			ctx,
			mockDB,
			"SESSION",
			"SESSION#abc",
			25,
			startTime,
			endTime,
			"session",
		)

		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("query without limit when limit is zero", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		// Setup mock chain - Limit should NOT be called when limit <= 0
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", "account").Return(mockQuery)
		mockQuery.On("Where", "accountPK", "=", "ACCOUNT#xyz").Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil)
		// Note: Limit is NOT expected to be called

		ctx := context.Background()
		_, err := AuditLogQueryHelper(
			ctx,
			mockDB,
			"ACCOUNT",
			"ACCOUNT#xyz",
			0, // zero limit
			time.Time{},
			time.Time{},
			"account",
		)

		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)

		// Explicitly verify Limit was not called
		mockQuery.AssertNotCalled(t, "Limit", mock.Anything)
	})

	t.Run("query error returns wrapped error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", "device").Return(mockQuery)
		mockQuery.On("Where", "devicePK", "=", "DEVICE#123").Return(mockQuery)
		mockQuery.On("Limit", 5).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(ErrTestMockError)

		ctx := context.Background()
		logs, err := AuditLogQueryHelper(
			ctx,
			mockDB,
			"DEVICE",
			"DEVICE#123",
			5,
			time.Time{},
			time.Time{},
			"device",
		)

		require.Error(t, err)
		assert.Nil(t, logs)
		// Error should contain information about the entity
		assert.Contains(t, err.Error(), "device audit logs")

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("partial time range - only startTime set", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		// When only one of startTime/endTime is set, no time range filter is applied
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", "user").Return(mockQuery)
		mockQuery.On("Where", "userPK", "=", "USER#partial").Return(mockQuery)
		mockQuery.On("Limit", 10).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil)

		ctx := context.Background()
		_, err := AuditLogQueryHelper(
			ctx,
			mockDB,
			"USER",
			"USER#partial",
			10,
			startTime,   // startTime set
			time.Time{}, // endTime not set
			"user",
		)

		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)

		// Verify that the time range Where clauses were NOT called
		// (because endTime is zero)
		for _, call := range mockQuery.Calls {
			if call.Method == "Where" {
				args := call.Arguments
				field := args.Get(0).(string)
				assert.NotContains(t, field, "SK", "SK Where clause should not be called when endTime is zero")
			}
		}
	})
}

// TestAuditLogQueryHelper_IndexNameNormalization tests that index names are lowercased
func TestAuditLogQueryHelper_IndexNameNormalization(t *testing.T) {
	tests := []struct {
		name            string
		inputIndex      string
		expectedIndex   string
		expectedPKField string
	}{
		{
			name:            "uppercase USER becomes lowercase",
			inputIndex:      "USER",
			expectedIndex:   "user",
			expectedPKField: "userPK",
		},
		{
			name:            "mixed case Session becomes lowercase",
			inputIndex:      "Session",
			expectedIndex:   "session",
			expectedPKField: "sessionPK",
		},
		{
			name:            "already lowercase",
			inputIndex:      "account",
			expectedIndex:   "account",
			expectedPKField: "accountPK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", mock.Anything).Return(mockDB)
			mockDB.On("Model", mock.Anything).Return(mockQuery)
			mockQuery.On("Index", tt.expectedIndex).Return(mockQuery)
			mockQuery.On("Where", tt.expectedPKField, "=", "TEST#123").Return(mockQuery)
			mockQuery.On("Limit", 10).Return(mockQuery)
			mockQuery.On("All", mock.Anything).Return(nil)

			ctx := context.Background()
			_, err := AuditLogQueryHelper(
				ctx,
				mockDB,
				tt.inputIndex,
				"TEST#123",
				10,
				time.Time{},
				time.Time{},
				"test",
			)

			require.NoError(t, err)
			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}
