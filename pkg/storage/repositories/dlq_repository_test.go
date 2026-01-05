package repositories

import (
	"context"
	"testing"
	"time"

	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ============================================================================
// Pure Function Tests: Clamp Helpers
// ============================================================================

func TestClampDLQPageLimit(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{
			name:     "zero_returns_default",
			input:    0,
			expected: dlqDefaultPageLimit,
		},
		{
			name:     "negative_returns_default",
			input:    -10,
			expected: dlqDefaultPageLimit,
		},
		{
			name:     "valid_within_range",
			input:    50,
			expected: 50,
		},
		{
			name:     "exactly_default",
			input:    20,
			expected: 20,
		},
		{
			name:     "exactly_max_limit",
			input:    dlqMaxPageLimit,
			expected: dlqMaxPageLimit,
		},
		{
			name:     "exceeds_max_returns_max",
			input:    500,
			expected: dlqMaxPageLimit,
		},
		{
			name:     "one_above_max",
			input:    dlqMaxPageLimit + 1,
			expected: dlqMaxPageLimit,
		},
		{
			name:     "one_below_max",
			input:    dlqMaxPageLimit - 1,
			expected: dlqMaxPageLimit - 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clampDLQPageLimit(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClampDLQReprocessLimit(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{
			name:     "zero_returns_default",
			input:    0,
			expected: dlqReprocessDefaultLimit,
		},
		{
			name:     "negative_returns_default",
			input:    -5,
			expected: dlqReprocessDefaultLimit,
		},
		{
			name:     "valid_within_range",
			input:    30,
			expected: 30,
		},
		{
			name:     "exactly_default",
			input:    dlqReprocessDefaultLimit,
			expected: dlqReprocessDefaultLimit,
		},
		{
			name:     "exceeds_max_returns_max",
			input:    300,
			expected: dlqMaxPageLimit,
		},
		{
			name:     "exactly_max",
			input:    dlqMaxPageLimit,
			expected: dlqMaxPageLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clampDLQReprocessLimit(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClampDLQSearchLimit(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{
			name:     "zero_returns_default",
			input:    0,
			expected: dlqSearchDefaultPageLimit,
		},
		{
			name:     "negative_returns_default",
			input:    -1,
			expected: dlqSearchDefaultPageLimit,
		},
		{
			name:     "valid_within_range",
			input:    100,
			expected: 100,
		},
		{
			name:     "exactly_default",
			input:    dlqSearchDefaultPageLimit,
			expected: dlqSearchDefaultPageLimit,
		},
		{
			name:     "exceeds_max_returns_max",
			input:    500,
			expected: dlqSearchMaxPageLimit,
		},
		{
			name:     "exactly_max",
			input:    dlqSearchMaxPageLimit,
			expected: dlqSearchMaxPageLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clampDLQSearchLimit(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// In-Memory Filtering Tests (Pure Helpers)
// ============================================================================

func TestMessageMatchesText(t *testing.T) {
	// Create a minimal DLQRepository for testing the pure helper
	mockDB := new(mocks.MockDB)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	tests := []struct {
		name       string
		message    *models.DLQMessage
		searchText string
		expected   bool
	}{
		{
			name: "matches_error_message_exact",
			message: &models.DLQMessage{
				ErrorMessage:  "Connection timeout exceeded",
				FailureReason: "",
				FunctionName:  "",
				MessageBody:   "",
			},
			searchText: "connection timeout",
			expected:   true,
		},
		{
			name: "matches_error_message_case_insensitive",
			message: &models.DLQMessage{
				ErrorMessage:  "DATABASE CONNECTION FAILED",
				FailureReason: "",
				FunctionName:  "",
				MessageBody:   "",
			},
			searchText: "database connection",
			expected:   true,
		},
		{
			name: "matches_failure_reason",
			message: &models.DLQMessage{
				ErrorMessage:  "error",
				FailureReason: "External API unavailable",
				FunctionName:  "",
				MessageBody:   "",
			},
			searchText: "api unavailable",
			expected:   true,
		},
		{
			name: "matches_function_name",
			message: &models.DLQMessage{
				ErrorMessage:  "error",
				FailureReason: "",
				FunctionName:  "NotificationProcessor",
				MessageBody:   "",
			},
			searchText: "notificationprocessor",
			expected:   true,
		},
		{
			name: "matches_message_body",
			message: &models.DLQMessage{
				ErrorMessage:  "error",
				FailureReason: "",
				FunctionName:  "",
				MessageBody:   `{"user_id": "12345", "action": "send_notification"}`,
			},
			searchText: "send_notification",
			expected:   true,
		},
		{
			name: "no_match",
			message: &models.DLQMessage{
				ErrorMessage:  "Connection refused",
				FailureReason: "Network error",
				FunctionName:  "Handler",
				MessageBody:   "test body",
			},
			searchText: "database",
			expected:   false,
		},
		{
			name: "partial_match",
			message: &models.DLQMessage{
				ErrorMessage:  "timeout error occurred",
				FailureReason: "",
				FunctionName:  "",
				MessageBody:   "",
			},
			searchText: "time",
			expected:   true,
		},
		{
			name: "empty_search_text",
			message: &models.DLQMessage{
				ErrorMessage:  "some error",
				FailureReason: "",
				FunctionName:  "",
				MessageBody:   "",
			},
			searchText: "",
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.messageMatchesText(tt.message, tt.searchText)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterByText(t *testing.T) {
	// Create a minimal DLQRepository for testing the pure helper
	mockDB := new(mocks.MockDB)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	messages := []*models.DLQMessage{
		{
			ID:            "msg-1",
			ErrorMessage:  "Connection timeout to database",
			FailureReason: "",
			FunctionName:  "",
			MessageBody:   "",
		},
		{
			ID:            "msg-2",
			ErrorMessage:  "Authentication failed",
			FailureReason: "",
			FunctionName:  "",
			MessageBody:   "",
		},
		{
			ID:            "msg-3",
			ErrorMessage:  "Database connection pool exhausted",
			FailureReason: "",
			FunctionName:  "",
			MessageBody:   "",
		},
		{
			ID:            "msg-4",
			ErrorMessage:  "Rate limit exceeded",
			FailureReason: "",
			FunctionName:  "",
			MessageBody:   "",
		},
	}

	tests := []struct {
		name        string
		searchText  string
		expectedIDs []string
	}{
		{
			name:        "filter_by_database",
			searchText:  "database",
			expectedIDs: []string{"msg-1", "msg-3"},
		},
		{
			name:        "filter_by_connection",
			searchText:  "connection",
			expectedIDs: []string{"msg-1", "msg-3"},
		},
		{
			name:        "filter_by_authentication",
			searchText:  "authentication",
			expectedIDs: []string{"msg-2"},
		},
		{
			name:        "filter_no_matches",
			searchText:  "nonexistent",
			expectedIDs: []string{},
		},
		{
			name:        "filter_by_rate",
			searchText:  "rate limit",
			expectedIDs: []string{"msg-4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.filterByText(messages, tt.searchText)

			var resultIDs []string
			for _, msg := range result {
				resultIDs = append(resultIDs, msg.ID)
			}

			if len(tt.expectedIDs) == 0 {
				assert.Empty(t, result)
			} else {
				assert.Equal(t, tt.expectedIDs, resultIDs)
			}
		})
	}
}

// ============================================================================
// Query-Based Fetch Tests: GetDLQMessagesByErrorType
// ============================================================================

func TestGetDLQMessagesByErrorType_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	ctx := context.Background()
	errorType := "connection_error"
	limit := 10

	// Set up expectations: the chain should be
	// WithContext -> Model -> Index -> Where -> OrderBy -> Limit -> All
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.DLQMessage")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "DLQ_ERROR#connection_error").Return(mockQuery)
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.DLQMessage")).Run(func(args mock.Arguments) {
		messages := args.Get(0).(*[]*models.DLQMessage)
		*messages = []*models.DLQMessage{
			{
				ID:        "msg-1",
				ErrorType: errorType,
				GSI1SK:    "2024-01-01T10:00:00Z#svc#msg-1",
			},
			{
				ID:        "msg-2",
				ErrorType: errorType,
				GSI1SK:    "2024-01-01T09:00:00Z#svc#msg-2",
			},
		}
	}).Return(nil)

	// Execute
	messages, cursor, err := repo.GetDLQMessagesByErrorType(ctx, errorType, limit, "")

	// Assert
	require.NoError(t, err)
	assert.Len(t, messages, 2)
	assert.Empty(t, cursor, "No cursor when results < limit")
	assert.Equal(t, "msg-1", messages[0].ID)
	assert.Equal(t, "msg-2", messages[1].ID)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetDLQMessagesByErrorType_WithCursor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	ctx := context.Background()
	errorType := "timeout_error"
	limit := 10
	cursor := "2024-01-01T08:00:00Z#svc#cursor-msg"

	// Set up expectations with cursor
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.DLQMessage")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "DLQ_ERROR#timeout_error").Return(mockQuery)
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
	// When cursor is provided, additional Where clause is added
	mockQuery.On("Where", "gsi1SK", "<", cursor).Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.DLQMessage")).Run(func(args mock.Arguments) {
		messages := args.Get(0).(*[]*models.DLQMessage)
		*messages = []*models.DLQMessage{
			{ID: "msg-3", ErrorType: errorType, GSI1SK: "2024-01-01T07:30:00Z#svc#msg-3"},
		}
	}).Return(nil)

	// Execute
	messages, nextCursor, err := repo.GetDLQMessagesByErrorType(ctx, errorType, limit, cursor)

	// Assert
	require.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Empty(t, nextCursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetDLQMessagesByErrorType_HasMoreResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	ctx := context.Background()
	errorType := "validation_error"
	limit := 2

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.DLQMessage")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "DLQ_ERROR#validation_error").Return(mockQuery)
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.DLQMessage")).Run(func(args mock.Arguments) {
		messages := args.Get(0).(*[]*models.DLQMessage)
		// Return limit+1 messages to trigger next cursor generation
		*messages = []*models.DLQMessage{
			{ID: "msg-1", GSI1SK: "2024-01-01T12:00:00Z#svc#msg-1"},
			{ID: "msg-2", GSI1SK: "2024-01-01T11:00:00Z#svc#msg-2"},
			{ID: "msg-3", GSI1SK: "2024-01-01T10:00:00Z#svc#msg-3"},
		}
	}).Return(nil)

	// Execute
	messages, nextCursor, err := repo.GetDLQMessagesByErrorType(ctx, errorType, limit, "")

	// Assert
	require.NoError(t, err)
	assert.Len(t, messages, limit, "Should be trimmed to limit")
	// Next cursor should be the GSI1SK of the last item before trimming (index safeLimit-1)
	assert.Equal(t, "2024-01-01T11:00:00Z#svc#msg-2", nextCursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetDLQMessagesByErrorType_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	ctx := context.Background()

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.DLQMessage")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "DLQ_ERROR#error_type").Return(mockQuery)
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.DLQMessage")).Return(ErrTestMockError)

	// Execute
	messages, cursor, err := repo.GetDLQMessagesByErrorType(ctx, "error_type", 10, "")

	// Assert
	require.Error(t, err)
	assert.Nil(t, messages)
	assert.Empty(t, cursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// Query-Based Fetch Tests: GetDLQMessagesByStatus
// ============================================================================

func TestGetDLQMessagesByStatus_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	ctx := context.Background()
	service := "notification-service"
	status := "new"
	limit := 20

	// Set up expectations: the chain uses gsi2
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.DLQMessage")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "DLQ_RETRY#notification-service#new").Return(mockQuery)
	mockQuery.On("OrderBy", "gsi2SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.DLQMessage")).Run(func(args mock.Arguments) {
		messages := args.Get(0).(*[]*models.DLQMessage)
		*messages = []*models.DLQMessage{
			{ID: "msg-1", Status: status, GSI2SK: "2024-01-01T10:00:00Z#msg-1"},
			{ID: "msg-2", Status: status, GSI2SK: "2024-01-01T09:00:00Z#msg-2"},
		}
	}).Return(nil)

	// Execute
	messages, cursor, err := repo.GetDLQMessagesByStatus(ctx, service, status, limit, "")

	// Assert
	require.NoError(t, err)
	assert.Len(t, messages, 2)
	assert.Empty(t, cursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetDLQMessagesByStatus_WithCursor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	ctx := context.Background()
	service := "activity-service"
	status := "reprocessing"
	limit := 10
	cursor := "2024-01-01T08:00:00Z#prev-msg"

	// Set up expectations with cursor
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.DLQMessage")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "DLQ_RETRY#activity-service#reprocessing").Return(mockQuery)
	mockQuery.On("OrderBy", "gsi2SK", "DESC").Return(mockQuery)
	mockQuery.On("Where", "gsi2SK", "<", cursor).Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.DLQMessage")).Run(func(args mock.Arguments) {
		messages := args.Get(0).(*[]*models.DLQMessage)
		*messages = []*models.DLQMessage{}
	}).Return(nil)

	// Execute
	messages, nextCursor, err := repo.GetDLQMessagesByStatus(ctx, service, status, limit, cursor)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, messages)
	assert.Empty(t, nextCursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetDLQMessagesByStatus_HasMoreResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	ctx := context.Background()
	service := "media-service"
	status := "failed"
	limit := 2

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.DLQMessage")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "DLQ_RETRY#media-service#failed").Return(mockQuery)
	mockQuery.On("OrderBy", "gsi2SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", limit+1).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.DLQMessage")).Run(func(args mock.Arguments) {
		messages := args.Get(0).(*[]*models.DLQMessage)
		*messages = []*models.DLQMessage{
			{ID: "msg-1", GSI2SK: "2024-01-01T12:00:00Z#msg-1"},
			{ID: "msg-2", GSI2SK: "2024-01-01T11:00:00Z#msg-2"},
			{ID: "msg-3", GSI2SK: "2024-01-01T10:00:00Z#msg-3"},
		}
	}).Return(nil)

	// Execute
	messages, nextCursor, err := repo.GetDLQMessagesByStatus(ctx, service, status, limit, "")

	// Assert
	require.NoError(t, err)
	assert.Len(t, messages, limit)
	assert.Equal(t, "2024-01-01T11:00:00Z#msg-2", nextCursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// Query-Based Fetch Tests: SearchDLQMessages
// ============================================================================

func TestSearchDLQMessages_MissingServiceReturnsError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	ctx := context.Background()
	filter := &DLQSearchFilter{
		Service: "", // Missing service
		Limit:   10,
	}

	// Execute
	messages, cursor, err := repo.SearchDLQMessages(ctx, filter)

	// Assert
	require.Error(t, err)
	assert.Nil(t, messages)
	assert.Empty(t, cursor)
	// The error should contain information about missing service
	appErr, ok := pkgErrors.AsAppError(err)
	require.True(t, ok)
	assert.True(t, pkgErrors.HasCode(err, pkgErrors.CodeRequiredFieldMissing))
	assert.Equal(t, "service", appErr.Metadata["field"])
}

func TestSearchDLQMessages_BasicQuery(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	ctx := context.Background()
	filter := &DLQSearchFilter{
		Service: "notification-service",
		Limit:   20,
	}

	// Set up expectations for basic search (no filters beyond service)
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.DLQMessage")).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery)
	mockQuery.On("Where", "gsi3PK", "=", "DLQ_SERVICE#notification-service").Return(mockQuery)
	mockQuery.On("OrderBy", "gsi3SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 21).Return(mockQuery) // limit + 1
	mockQuery.On("All", mock.AnythingOfType("*[]*models.DLQMessage")).Run(func(args mock.Arguments) {
		messages := args.Get(0).(*[]*models.DLQMessage)
		*messages = []*models.DLQMessage{
			{ID: "msg-1", Service: "notification-service"},
			{ID: "msg-2", Service: "notification-service"},
		}
	}).Return(nil)

	// Execute
	messages, cursor, err := repo.SearchDLQMessages(ctx, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, messages, 2)
	assert.Empty(t, cursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSearchDLQMessages_WithFilters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	ctx := context.Background()
	isPermanent := true
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	filter := &DLQSearchFilter{
		Service:     "activity-service",
		ErrorType:   "validation_error",
		Status:      "failed",
		Priority:    "high",
		IsPermanent: &isPermanent,
		StartTime:   startTime,
		EndTime:     endTime,
		Limit:       10,
	}

	// Set up expectations with all filters applied
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.DLQMessage")).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery)
	mockQuery.On("Where", "gsi3PK", "=", "DLQ_SERVICE#activity-service").Return(mockQuery)
	mockQuery.On("OrderBy", "gsi3SK", "DESC").Return(mockQuery)
	// Filter calls
	mockQuery.On("Filter", "ErrorType", "=", "validation_error").Return(mockQuery)
	mockQuery.On("Filter", "Status", "=", "failed").Return(mockQuery)
	mockQuery.On("Filter", "Priority", "=", "high").Return(mockQuery)
	mockQuery.On("Filter", "IsPermanent", "=", true).Return(mockQuery)
	mockQuery.On("Filter", "FirstSeenAt", ">=", startTime.Unix()).Return(mockQuery)
	mockQuery.On("Filter", "FirstSeenAt", "<=", endTime.Unix()).Return(mockQuery)
	mockQuery.On("Limit", 11).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.DLQMessage")).Run(func(args mock.Arguments) {
		messages := args.Get(0).(*[]*models.DLQMessage)
		*messages = []*models.DLQMessage{
			{ID: "msg-filtered-1"},
		}
	}).Return(nil)

	// Execute
	messages, cursor, err := repo.SearchDLQMessages(ctx, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Empty(t, cursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSearchDLQMessages_WithCursor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	ctx := context.Background()
	cursor := "2024-01-01T08:00:00Z#error#msg-cursor"

	filter := &DLQSearchFilter{
		Service: "search-service",
		Cursor:  cursor,
		Limit:   10,
	}

	// Set up expectations with cursor
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.DLQMessage")).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery)
	mockQuery.On("Where", "gsi3PK", "=", "DLQ_SERVICE#search-service").Return(mockQuery)
	mockQuery.On("OrderBy", "gsi3SK", "DESC").Return(mockQuery)
	mockQuery.On("Where", "gsi3SK", "<", cursor).Return(mockQuery)
	mockQuery.On("Limit", 11).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.DLQMessage")).Run(func(args mock.Arguments) {
		messages := args.Get(0).(*[]*models.DLQMessage)
		*messages = []*models.DLQMessage{}
	}).Return(nil)

	// Execute
	messages, nextCursor, err := repo.SearchDLQMessages(ctx, filter)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, messages)
	assert.Empty(t, nextCursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSearchDLQMessages_WithTextSearch(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	ctx := context.Background()
	filter := &DLQSearchFilter{
		Service:    "text-search-service",
		SearchText: "database connection",
		Limit:      10,
	}

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.DLQMessage")).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery)
	mockQuery.On("Where", "gsi3PK", "=", "DLQ_SERVICE#text-search-service").Return(mockQuery)
	mockQuery.On("OrderBy", "gsi3SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 11).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.DLQMessage")).Run(func(args mock.Arguments) {
		messages := args.Get(0).(*[]*models.DLQMessage)
		// Return multiple messages, some matching text filter, some not
		*messages = []*models.DLQMessage{
			{
				ID:           "msg-match-1",
				ErrorMessage: "Database connection timeout",
			},
			{
				ID:           "msg-no-match",
				ErrorMessage: "Rate limit exceeded",
			},
			{
				ID:           "msg-match-2",
				ErrorMessage: "Failed to establish database connection",
			},
		}
	}).Return(nil)

	// Execute
	messages, cursor, err := repo.SearchDLQMessages(ctx, filter)

	// Assert
	require.NoError(t, err)
	// Only messages matching "database connection" should be returned
	assert.Len(t, messages, 2)
	assert.Equal(t, "msg-match-1", messages[0].ID)
	assert.Equal(t, "msg-match-2", messages[1].ID)
	assert.Empty(t, cursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSearchDLQMessages_HasMoreResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	ctx := context.Background()
	filter := &DLQSearchFilter{
		Service: "paginated-service",
		Limit:   2,
	}

	// Set up expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.DLQMessage")).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery)
	mockQuery.On("Where", "gsi3PK", "=", "DLQ_SERVICE#paginated-service").Return(mockQuery)
	mockQuery.On("OrderBy", "gsi3SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 3).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.DLQMessage")).Run(func(args mock.Arguments) {
		messages := args.Get(0).(*[]*models.DLQMessage)
		*messages = []*models.DLQMessage{
			{ID: "msg-1", GSI3SK: "2024-01-01T12:00:00Z#err#msg-1"},
			{ID: "msg-2", GSI3SK: "2024-01-01T11:00:00Z#err#msg-2"},
			{ID: "msg-3", GSI3SK: "2024-01-01T10:00:00Z#err#msg-3"},
		}
	}).Return(nil)

	// Execute
	messages, nextCursor, err := repo.SearchDLQMessages(ctx, filter)

	// Assert
	require.NoError(t, err)
	assert.Len(t, messages, 2)
	assert.Equal(t, "2024-01-01T11:00:00Z#err#msg-2", nextCursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// Cleanup Expired Messages Tests
// ============================================================================

func TestCleanupExpiredMessages_NoExpiredMessages(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	ctx := context.Background()
	beforeTime := time.Now()

	// Set up expectations - query returns empty results
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.DLQMessage")).Return(mockQuery)
	mockQuery.On("Filter", "ExpiresAt", "<", beforeTime.Unix()).Return(mockQuery)
	mockQuery.On("Limit", 100).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.DLQMessage")).Run(func(args mock.Arguments) {
		messages := args.Get(0).(*[]*models.DLQMessage)
		*messages = []*models.DLQMessage{} // Empty result
	}).Return(nil)

	// Execute
	deletedCount, err := repo.CleanupExpiredMessages(ctx, beforeTime)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 0, deletedCount)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestCleanupExpiredMessages_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := &DLQRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.DLQMessage](
			mockDB, "test-table", nil, nil, "DLQRepository", "dlq",
		),
	}

	ctx := context.Background()
	beforeTime := time.Now()

	// Set up expectations - query returns error
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.DLQMessage")).Return(mockQuery)
	mockQuery.On("Filter", "ExpiresAt", "<", beforeTime.Unix()).Return(mockQuery)
	mockQuery.On("Limit", 100).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.DLQMessage")).Return(ErrTestMockError)

	// Execute
	deletedCount, err := repo.CleanupExpiredMessages(ctx, beforeTime)

	// Assert
	require.Error(t, err)
	assert.Equal(t, 0, deletedCount)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// Limit Clamping Edge Case Tests
// ============================================================================

func TestClampDLQPageLimit_EdgeCases(t *testing.T) {
	// Verify constants are as expected
	assert.Equal(t, 20, dlqDefaultPageLimit)
	assert.Equal(t, 200, dlqMaxPageLimit)

	// Test boundary values
	// Value of 1 is valid (1 > 0 && 1 <= max), so returns 1, not default
	assert.Equal(t, 1, clampDLQPageLimit(1))
	assert.Equal(t, dlqMaxPageLimit, clampDLQPageLimit(dlqMaxPageLimit))
	assert.Equal(t, dlqMaxPageLimit, clampDLQPageLimit(dlqMaxPageLimit+1))
}

func TestClampDLQReprocessLimit_EdgeCases(t *testing.T) {
	// Verify constants are as expected
	assert.Equal(t, 50, dlqReprocessDefaultLimit)

	// Test boundary values
	assert.Equal(t, 1, clampDLQReprocessLimit(1))
	assert.Equal(t, dlqMaxPageLimit, clampDLQReprocessLimit(dlqMaxPageLimit))
	assert.Equal(t, dlqMaxPageLimit, clampDLQReprocessLimit(1000))
}

func TestClampDLQSearchLimit_EdgeCases(t *testing.T) {
	// Verify constants are as expected
	assert.Equal(t, 50, dlqSearchDefaultPageLimit)
	assert.Equal(t, 200, dlqSearchMaxPageLimit)

	// Test boundary values
	assert.Equal(t, 1, clampDLQSearchLimit(1))
	assert.Equal(t, dlqSearchMaxPageLimit, clampDLQSearchLimit(dlqSearchMaxPageLimit))
	assert.Equal(t, dlqSearchMaxPageLimit, clampDLQSearchLimit(500))
}
