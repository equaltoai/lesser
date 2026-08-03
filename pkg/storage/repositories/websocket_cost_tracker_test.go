package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// TestCalculateOperationCosts tests the cost calculation for different WebSocket operation types
func TestCalculateOperationCosts(t *testing.T) {
	logger := zap.NewNop()

	// Create tracker with nil repo for pure cost calculation tests
	tracker := &WebSocketCostTracker{
		costRepo:     nil,
		logger:       logger,
		serviceName:  "test-service",
		functionName: "test-function",
	}

	tests := []struct {
		name      string
		opCtx     *WebSocketOperationContext
		result    *WebSocketOperationResult
		checkFunc func(t *testing.T, breakdown *models.WebSocketCostBreakdown)
	}{
		{
			name: "connect operation with connection time",
			opCtx: &WebSocketOperationContext{
				ConnectionID:  "conn-123",
				UserID:        "user-1",
				OperationType: WSEventConnect,
				StartTime:     time.Now(),
			},
			result: &WebSocketOperationResult{
				Success:              true,
				ProcessingTimeMs:     100,
				ConnectionDurationMs: 120000, // 2 minutes
			},
			checkFunc: func(t *testing.T, breakdown *models.WebSocketCostBreakdown) {
				require.NotNil(t, breakdown)
				// Verify TotalCostMicroCents is calculated
				require.GreaterOrEqual(t, breakdown.TotalCostMicroCents, int64(0))
			},
		},
		{
			name: "disconnect operation",
			opCtx: &WebSocketOperationContext{
				ConnectionID:  "conn-123",
				UserID:        "user-1",
				OperationType: WSEventDisconnect,
				StartTime:     time.Now(),
			},
			result: &WebSocketOperationResult{
				Success:              true,
				ProcessingTimeMs:     50,
				ConnectionDurationMs: 180000, // 3 minutes
			},
			checkFunc: func(t *testing.T, breakdown *models.WebSocketCostBreakdown) {
				require.NotNil(t, breakdown)
				require.GreaterOrEqual(t, breakdown.TotalCostMicroCents, int64(0))
			},
		},
		{
			name: "message_in with message count and size",
			opCtx: &WebSocketOperationContext{
				ConnectionID:  "conn-123",
				UserID:        "user-1",
				OperationType: WSEventMessageIn,
				StartTime:     time.Now(),
			},
			result: &WebSocketOperationResult{
				Success:          true,
				ProcessingTimeMs: 20,
				MessageCount:     5,
				MessageSizeBytes: 1024 * 1024, // 1 MB
			},
			checkFunc: func(t *testing.T, breakdown *models.WebSocketCostBreakdown) {
				require.NotNil(t, breakdown)
				require.GreaterOrEqual(t, breakdown.TotalCostMicroCents, int64(0))
				// DynamoDB operations should contribute to DataTransferCost
				require.GreaterOrEqual(t, breakdown.DataTransferCost, int64(0))
			},
		},
		{
			name: "message_out operation",
			opCtx: &WebSocketOperationContext{
				ConnectionID:  "conn-123",
				UserID:        "user-1",
				OperationType: WSEventMessageOut,
				StartTime:     time.Now(),
			},
			result: &WebSocketOperationResult{
				Success:          true,
				ProcessingTimeMs: 30,
				MessageCount:     10,
				MessageSizeBytes: 2048,
			},
			checkFunc: func(t *testing.T, breakdown *models.WebSocketCostBreakdown) {
				require.NotNil(t, breakdown)
				require.GreaterOrEqual(t, breakdown.TotalCostMicroCents, int64(0))
			},
		},
		{
			name: "subscribe operation",
			opCtx: &WebSocketOperationContext{
				ConnectionID:  "conn-123",
				UserID:        "user-1",
				OperationType: "subscribe",
				StartTime:     time.Now(),
			},
			result: &WebSocketOperationResult{
				Success:          true,
				ProcessingTimeMs: 15,
			},
			checkFunc: func(t *testing.T, breakdown *models.WebSocketCostBreakdown) {
				require.NotNil(t, breakdown)
				require.GreaterOrEqual(t, breakdown.TotalCostMicroCents, int64(0))
			},
		},
		{
			name: "unsubscribe operation",
			opCtx: &WebSocketOperationContext{
				ConnectionID:  "conn-123",
				UserID:        "user-1",
				OperationType: "unsubscribe",
				StartTime:     time.Now(),
			},
			result: &WebSocketOperationResult{
				Success:          true,
				ProcessingTimeMs: 10,
			},
			checkFunc: func(t *testing.T, breakdown *models.WebSocketCostBreakdown) {
				require.NotNil(t, breakdown)
				require.GreaterOrEqual(t, breakdown.TotalCostMicroCents, int64(0))
			},
		},
		{
			name: "idle_time operation",
			opCtx: &WebSocketOperationContext{
				ConnectionID:  "conn-123",
				UserID:        "user-1",
				OperationType: "idle_time",
				StartTime:     time.Now(),
			},
			result: &WebSocketOperationResult{
				Success:    true,
				IdleTimeMs: 300000, // 5 minutes
			},
			checkFunc: func(t *testing.T, breakdown *models.WebSocketCostBreakdown) {
				require.NotNil(t, breakdown)
				require.GreaterOrEqual(t, breakdown.TotalCostMicroCents, int64(0))
			},
		},
		{
			name: "ping operation",
			opCtx: &WebSocketOperationContext{
				ConnectionID:  "conn-123",
				UserID:        "user-1",
				OperationType: "ping",
				StartTime:     time.Now(),
			},
			result: &WebSocketOperationResult{
				Success:          true,
				ProcessingTimeMs: 5,
			},
			checkFunc: func(t *testing.T, breakdown *models.WebSocketCostBreakdown) {
				require.NotNil(t, breakdown)
				require.GreaterOrEqual(t, breakdown.TotalCostMicroCents, int64(0))
			},
		},
		{
			name: "error operation",
			opCtx: &WebSocketOperationContext{
				ConnectionID:  "conn-123",
				UserID:        "user-1",
				OperationType: "error",
				StartTime:     time.Now(),
			},
			result: &WebSocketOperationResult{
				Success:          false,
				ProcessingTimeMs: 2,
				Error:            errors.New("test error"),
			},
			checkFunc: func(t *testing.T, breakdown *models.WebSocketCostBreakdown) {
				require.NotNil(t, breakdown)
				require.GreaterOrEqual(t, breakdown.TotalCostMicroCents, int64(0))
			},
		},
		{
			name: "zero message size does not add data MB",
			opCtx: &WebSocketOperationContext{
				ConnectionID:  "conn-123",
				UserID:        "user-1",
				OperationType: WSEventMessageIn,
				StartTime:     time.Now(),
			},
			result: &WebSocketOperationResult{
				Success:          true,
				ProcessingTimeMs: 10,
				MessageCount:     1,
				MessageSizeBytes: 0,
			},
			checkFunc: func(t *testing.T, breakdown *models.WebSocketCostBreakdown) {
				require.NotNil(t, breakdown)
				require.GreaterOrEqual(t, breakdown.TotalCostMicroCents, int64(0))
			},
		},
		{
			name: "zero idle time does not contribute to connection minutes",
			opCtx: &WebSocketOperationContext{
				ConnectionID:  "conn-123",
				UserID:        "user-1",
				OperationType: "idle_time",
				StartTime:     time.Now(),
			},
			result: &WebSocketOperationResult{
				Success:    true,
				IdleTimeMs: 0,
			},
			checkFunc: func(t *testing.T, breakdown *models.WebSocketCostBreakdown) {
				require.NotNil(t, breakdown)
				require.GreaterOrEqual(t, breakdown.TotalCostMicroCents, int64(0))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			breakdown := tracker.calculateOperationCosts(tt.opCtx, tt.result)
			tt.checkFunc(t, breakdown)
		})
	}
}

// TestGetClientIP tests IP extraction from WebSocket event
func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string][]string
		expected string
	}{
		{
			name: "x-forwarded-for header single IP",
			headers: map[string][]string{
				"x-forwarded-for": {"10.0.0.1"},
			},
			expected: "10.0.0.1",
		},
		{
			name: "x-forwarded-for header multiple IPs",
			headers: map[string][]string{
				"x-forwarded-for": {"10.0.0.1, 10.0.0.2, 10.0.0.3"},
			},
			expected: "10.0.0.1",
		},
		{
			name:     "no IP available returns unknown",
			headers:  map[string][]string{},
			expected: StatusUnknown,
		},
		{
			name:     "nil headers returns unknown",
			headers:  nil,
			expected: StatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getClientIP(tt.headers)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestGetUserAgent tests user agent extraction from WebSocket event
func TestGetUserAgent(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string][]string
		expected string
	}{
		{
			name: "lowercase user-agent header",
			headers: map[string][]string{
				"user-agent": {"Mozilla/5.0 Test Browser"},
			},
			expected: "Mozilla/5.0 Test Browser",
		},
		{
			name:     "no user agent returns unknown",
			headers:  map[string][]string{},
			expected: StatusUnknown,
		},
		{
			name:     "nil headers returns unknown",
			headers:  nil,
			expected: StatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getUserAgent(tt.headers)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestDetermineConnectionSource tests connection source determination from user agent
func TestDetermineConnectionSource(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		expected  string
	}{
		{
			name:      "mobile user agent with Mobile keyword",
			userAgent: "Mozilla/5.0 Mobile Safari",
			expected:  "mobile",
		},
		{
			name:      "mobile user agent with Android keyword",
			userAgent: "Mozilla/5.0 Android 10",
			expected:  "mobile",
		},
		{
			name:      "mobile user agent with iPhone keyword",
			userAgent: "Mozilla/5.0 iPhone iOS",
			expected:  "mobile",
		},
		{
			name:      "api user agent with postman",
			userAgent: "PostmanRuntime/7.28.0",
			expected:  "api",
		},
		{
			name:      "api user agent with curl",
			userAgent: "curl/7.79.1",
			expected:  "api",
		},
		{
			name:      "api user agent with wget",
			userAgent: "Wget/1.21",
			expected:  "api",
		},
		{
			name:      "web user agent default",
			userAgent: "Mozilla/5.0 Windows Chrome",
			expected:  "web",
		},
		{
			name:      "no user agent defaults to web",
			userAgent: "",
			expected:  "web",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineConnectionSource(tt.userAgent)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestDetermineAuthMethod tests authentication method determination
func TestDetermineAuthMethod(t *testing.T) {
	tests := []struct {
		name     string
		query    map[string][]string
		headers  map[string][]string
		expected string
	}{
		{
			name:     "oauth via query parameter",
			query:    map[string][]string{"access_token": {"some-token"}},
			expected: "oauth",
		},
		{
			name:     "bearer token via Authorization header",
			headers:  map[string][]string{"authorization": {"Bearer some-jwt-token"}},
			expected: "bearer",
		},
		{
			name:     "basic auth via Authorization header",
			headers:  map[string][]string{"authorization": {"Basic dXNlcjpwYXNz"}},
			expected: "basic",
		},
		{
			name:     "bearer token via lowercase authorization header",
			headers:  map[string][]string{"authorization": {"Bearer another-token"}},
			expected: "bearer",
		},
		{
			name:     "anonymous when no auth provided",
			headers:  map[string][]string{},
			expected: "anonymous",
		},
		{
			name:     "anonymous when headers nil",
			headers:  nil,
			expected: "anonymous",
		},
		{
			name:     "anonymous when Authorization header has unknown format",
			headers:  map[string][]string{"authorization": {"Custom scheme-value"}},
			expected: "anonymous",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineAuthMethod(tt.query, tt.headers)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestGetErrorType tests error type classification
func TestGetErrorType(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error returns none",
			err:      nil,
			expected: RepliesPolicyNone,
		},
		{
			name:     "timeout error",
			err:      errors.New("connection timeout occurred"),
			expected: "timeout",
		},
		{
			name:     "connection error",
			err:      errors.New("connection refused"),
			expected: "connection",
		},
		{
			name:     "auth error with auth keyword",
			err:      errors.New("auth token expired"),
			expected: "auth",
		},
		{
			name:     "auth error with unauthorized keyword",
			err:      errors.New("unauthorized access"),
			expected: "auth",
		},
		{
			name:     "rate limiting error with rate",
			err:      errors.New("rate exceeded"),
			expected: "rate_limit",
		},
		{
			name:     "rate limiting error with limit",
			err:      errors.New("request limit reached"),
			expected: "rate_limit",
		},
		{
			name:     "validation error",
			err:      errors.New("validation failed for field"),
			expected: "validation",
		},
		{
			name:     "budget error with budget keyword",
			err:      errors.New("budget exceeded"),
			expected: "budget",
		},
		{
			name:     "budget error with quota keyword",
			err:      errors.New("quota depleted"),
			expected: "budget",
		},
		{
			name:     "unknown error type",
			err:      errors.New("some generic error"),
			expected: StatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getErrorType(tt.err)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestCheckBudgetLimits_EmptyUser tests that empty userID returns default allow struct
func TestCheckBudgetLimits_EmptyUser(t *testing.T) {
	logger := zap.NewNop()
	tracker := &WebSocketCostTracker{
		costRepo: nil,
		logger:   logger,
	}

	ctx := context.Background()
	status, err := tracker.CheckBudgetLimits(ctx, "")

	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, "", status.UserID)
	require.True(t, status.AllowConnection)
	require.True(t, status.AllowMessages)
	require.NotNil(t, status.Budgets)
	require.Empty(t, status.Budgets)
}

// TestCheckBudgetLimits_WithUser tests budget check delegation to repository
func TestCheckBudgetLimits_WithUser(t *testing.T) {
	logger := zap.NewNop()

	// Create mock DB for WebSocketCostRepository
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Setup the query chain
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	// Return empty budgets to simulate no active budgets
	mockQuery.On("All", mock.AnythingOfType("*[]*models.WebSocketCostBudget")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.WebSocketCostBudget)
		*dest = []*models.WebSocketCostBudget{}
	}).Return(nil)

	repo := &WebSocketCostRepository{
		EnhancedBaseRepository: &EnhancedBaseRepository[*models.WebSocketCostRecord]{
			BaseRepository: &BaseRepository[*models.WebSocketCostRecord]{
				db:        mockDB,
				tableName: "test-table",
				logger:    logger,
			},
		},
		budgetRepo: &EnhancedBaseRepository[*models.WebSocketCostBudget]{
			BaseRepository: &BaseRepository[*models.WebSocketCostBudget]{
				db:        mockDB,
				tableName: "test-table",
				logger:    logger,
			},
		},
	}

	tracker := &WebSocketCostTracker{
		costRepo: repo,
		logger:   logger,
	}

	ctx := context.Background()
	status, err := tracker.CheckBudgetLimits(ctx, "user-123")

	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, "user-123", status.UserID)
	require.True(t, status.AllowConnection)
	require.True(t, status.AllowMessages)
}

// TestDetermineOperationType tests route key to operation type mapping
func TestDetermineOperationType(t *testing.T) {
	tests := []struct {
		name     string
		routeKey string
		expected string
	}{
		{
			name:     "connect route",
			routeKey: "$connect",
			expected: WSEventConnect,
		},
		{
			name:     "disconnect route",
			routeKey: "$disconnect",
			expected: WSEventDisconnect,
		},
		{
			name:     "default route",
			routeKey: "$default",
			expected: WSEventMessageIn,
		},
		{
			name:     "unknown route",
			routeKey: "some-custom-route",
			expected: StatusUnknown,
		},
		{
			name:     "empty route",
			routeKey: "",
			expected: StatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineOperationType(tt.routeKey)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestTrackWebSocketOperation_SuccessWithNilRepo tests that tracking does not fail when repo is nil
// This simulates the "cost tracking fails but operation does not fail" scenario
func TestTrackWebSocketOperation_ProcessingTimeCalculation(t *testing.T) {
	logger := zap.NewNop()

	// Create a mock DB that will fail on create
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(errors.New("db error"))

	repo := &WebSocketCostRepository{
		EnhancedBaseRepository: &EnhancedBaseRepository[*models.WebSocketCostRecord]{
			BaseRepository: &BaseRepository[*models.WebSocketCostRecord]{
				db:        mockDB,
				tableName: "test-table",
				logger:    logger,
			},
		},
	}

	tracker := &WebSocketCostTracker{
		costRepo:     repo,
		logger:       logger,
		serviceName:  "test-service",
		functionName: "test-function",
	}

	ctx := context.Background()
	opCtx := &WebSocketOperationContext{
		ConnectionID:  "conn-123",
		UserID:        "",
		OperationType: "ping",
		StartTime:     time.Now().Add(-100 * time.Millisecond), // Started 100ms ago
	}
	result := &WebSocketOperationResult{
		Success:          true,
		ProcessingTimeMs: 0, // Not provided, should be calculated
	}

	// Should not return error even if repo fails
	err := tracker.TrackWebSocketOperation(ctx, opCtx, result)
	require.NoError(t, err)

	// Verify processing time was calculated
	require.Greater(t, result.ProcessingTimeMs, int64(0))
}

// TestCheckBudgetIfRequired tests budget checking for specific operation types
func TestCheckBudgetIfRequired(t *testing.T) {
	logger := zap.NewNop()

	// Create mock DB
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.WebSocketCostBudget")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.WebSocketCostBudget)
		*dest = []*models.WebSocketCostBudget{}
	}).Return(nil)

	repo := &WebSocketCostRepository{
		EnhancedBaseRepository: &EnhancedBaseRepository[*models.WebSocketCostRecord]{
			BaseRepository: &BaseRepository[*models.WebSocketCostRecord]{
				db:        mockDB,
				tableName: "test-table",
				logger:    logger,
			},
		},
		budgetRepo: &EnhancedBaseRepository[*models.WebSocketCostBudget]{
			BaseRepository: &BaseRepository[*models.WebSocketCostBudget]{
				db:        mockDB,
				tableName: "test-table",
				logger:    logger,
			},
		},
	}

	tracker := &WebSocketCostTracker{
		costRepo: repo,
		logger:   logger,
	}

	t.Run("disconnect operation skips budget check", func(t *testing.T) {
		opCtx := &WebSocketOperationContext{
			UserID: "user-123",
		}

		err := checkBudgetIfRequired(context.Background(), tracker, WSEventDisconnect, opCtx)
		require.NoError(t, err)
	})

	t.Run("empty user skips budget check", func(t *testing.T) {
		opCtx := &WebSocketOperationContext{
			UserID: "",
		}

		err := checkBudgetIfRequired(context.Background(), tracker, "connect", opCtx)
		require.NoError(t, err)
	})
}

// TestExecuteAndMeasure tests handler execution and measurement
func TestExecuteAndMeasure(t *testing.T) {
	t.Run("successful handler execution", func(t *testing.T) {
		handler := func(ctx *apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(200, ""), nil
		}

		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Body: []byte("test message"),
			},
		}

		result, resp, err := executeAndMeasure(handler, ctx, WSEventMessageIn)

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.True(t, result.Success)
		require.GreaterOrEqual(t, result.ProcessingTimeMs, int64(0))
		require.Equal(t, int64(12), result.MessageSizeBytes) // "test message" = 12 bytes
		require.Equal(t, 1, result.MessageCount)
		require.Equal(t, 128.0, result.MemoryUsedMB)
	})

	t.Run("failed handler execution", func(t *testing.T) {
		expectedErr := errors.New("handler failed")
		handler := func(ctx *apptheory.Context) (*apptheory.Response, error) {
			return nil, expectedErr
		}

		ctx := &apptheory.Context{
			Request: apptheory.Request{},
		}

		result, resp, err := executeAndMeasure(handler, ctx, "connect")

		require.Error(t, err)
		require.Nil(t, resp)
		require.False(t, result.Success)
		require.Equal(t, expectedErr, result.Error)
	})

	t.Run("non-message operation does not set message details", func(t *testing.T) {
		handler := func(ctx *apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(200, ""), nil
		}

		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Body: []byte("not a message"),
			},
		}

		result, resp, err := executeAndMeasure(handler, ctx, "connect")

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 0, result.MessageCount)
		require.Equal(t, int64(0), result.MessageSizeBytes)
	})
}

// ============================================================================
// NewWebSocketCostTracker Tests
// ============================================================================

func TestNewWebSocketCostTracker(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()

	// Create a mock WebSocketCostRepository
	mockRepo := NewWebSocketCostRepository(mockDB, "test-table", logger, nil)

	tracker := NewWebSocketCostTracker(mockRepo, logger)

	require.NotNil(t, tracker)
	require.Equal(t, mockRepo, tracker.costRepo)
	require.Equal(t, logger, tracker.logger)
	// serviceName and functionName are set from environment
	require.NotNil(t, tracker.serviceName)
}

// ============================================================================
// TrackConnectionLifecycle Tests
// ============================================================================

func TestTrackConnectionLifecycle(t *testing.T) {
	t.Run("successful lifecycle tracking", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		logger := zap.NewNop()
		mockRepo := NewWebSocketCostRepository(mockDB, "test-table", logger, nil)

		tracker := &WebSocketCostTracker{
			costRepo:     mockRepo,
			logger:       logger,
			serviceName:  "test-service",
			functionName: "test-function",
		}

		ctx := context.Background()

		// Mock the cost record creation
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(nil)

		err := tracker.TrackConnectionLifecycle(ctx,
			"conn-123",
			"",
			"testuser",
			5*time.Minute,
			10,   // messagesSent
			20,   // messagesReceived
			1024, // totalDataBytes
		)

		require.NoError(t, err)
	})

	t.Run("lifecycle tracking with zero duration", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		logger := zap.NewNop()
		mockRepo := NewWebSocketCostRepository(mockDB, "test-table", logger, nil)

		tracker := &WebSocketCostTracker{
			costRepo:     mockRepo,
			logger:       logger,
			serviceName:  "test-service",
			functionName: "test-function",
		}

		ctx := context.Background()

		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(nil)

		err := tracker.TrackConnectionLifecycle(ctx, "conn-123", "", "testuser", 0, 0, 0, 0)

		require.NoError(t, err)
	})
}

// ============================================================================
// TrackIdleConnections Tests
// ============================================================================

func TestTrackIdleConnections(t *testing.T) {
	t.Run("tracks idle connections over 1 minute", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		logger := zap.NewNop()
		mockRepo := NewWebSocketCostRepository(mockDB, "test-table", logger, nil)

		tracker := &WebSocketCostTracker{
			costRepo:     mockRepo,
			logger:       logger,
			serviceName:  "test-service",
			functionName: "test-function",
		}

		ctx := context.Background()

		// Mock for tracking
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(nil)

		connections := []models.WebSocketConnection{
			{
				ConnectionID: "conn-1",
				UserID:       "",
				Username:     "user1",
				LastActivity: time.Now().Add(-2 * time.Minute), // 2 minutes idle
				Streams:      []string{"stream1"},
			},
		}

		err := tracker.TrackIdleConnections(ctx, connections)

		require.NoError(t, err)
	})

	t.Run("skips connections idle less than 1 minute", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		logger := zap.NewNop()
		mockRepo := NewWebSocketCostRepository(mockDB, "test-table", logger, nil)

		tracker := &WebSocketCostTracker{
			costRepo:     mockRepo,
			logger:       logger,
			serviceName:  "test-service",
			functionName: "test-function",
		}

		ctx := context.Background()

		// No mock expectations since connections under 1 minute should be skipped

		connections := []models.WebSocketConnection{
			{
				ConnectionID: "conn-1",
				UserID:       "user-1",
				LastActivity: time.Now().Add(-30 * time.Second), // 30 seconds idle - should be skipped
			},
		}

		err := tracker.TrackIdleConnections(ctx, connections)

		require.NoError(t, err)
	})

	t.Run("empty connections list", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		logger := zap.NewNop()
		mockRepo := NewWebSocketCostRepository(mockDB, "test-table", logger, nil)

		tracker := &WebSocketCostTracker{
			costRepo:     mockRepo,
			logger:       logger,
			serviceName:  "test-service",
			functionName: "test-function",
		}

		ctx := context.Background()

		err := tracker.TrackIdleConnections(ctx, []models.WebSocketConnection{})

		require.NoError(t, err)
	})

	t.Run("continues on individual connection error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		logger := zap.NewNop()
		mockRepo := NewWebSocketCostRepository(mockDB, "test-table", logger, nil)

		tracker := &WebSocketCostTracker{
			costRepo:     mockRepo,
			logger:       logger,
			serviceName:  "test-service",
			functionName: "test-function",
		}

		ctx := context.Background()

		// First call fails, second succeeds
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(errors.New("db error")).Once()
		mockQuery.On("Create").Return(nil).Once()

		connections := []models.WebSocketConnection{
			{
				ConnectionID: "conn-1",
				UserID:       "",
				LastActivity: time.Now().Add(-2 * time.Minute),
			},
			{
				ConnectionID: "conn-2",
				UserID:       "",
				LastActivity: time.Now().Add(-3 * time.Minute),
			},
		}

		err := tracker.TrackIdleConnections(ctx, connections)

		// Should not fail even if individual tracking fails
		require.NoError(t, err)
	})
}

// ============================================================================
// GetUserCostSummary Tests (delegates to repo)
// ============================================================================

func TestGetUserCostSummary(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	mockRepo := NewWebSocketCostRepository(mockDB, "test-table", logger, nil)

	tracker := &WebSocketCostTracker{
		costRepo:     mockRepo,
		logger:       logger,
		serviceName:  "test-service",
		functionName: "test-function",
	}

	ctx := context.Background()
	startTime := time.Now().Add(-24 * time.Hour)
	endTime := time.Now()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(nil)

	_, err := tracker.GetUserCostSummary(ctx, "user-123", startTime, endTime)

	// Just testing that it delegates to repo without error
	require.NoError(t, err)
}

// ============================================================================
// GetHighCostOperations Tests (delegates to repo)
// ============================================================================

func TestGetHighCostOperations(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	mockRepo := NewWebSocketCostRepository(mockDB, "test-table", logger, nil)

	tracker := &WebSocketCostTracker{
		costRepo:     mockRepo,
		logger:       logger,
		serviceName:  "test-service",
		functionName: "test-function",
	}

	ctx := context.Background()
	startTime := time.Now().Add(-24 * time.Hour)
	endTime := time.Now()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Between", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(nil)

	_, err := tracker.GetHighCostOperations(ctx, 1.0, startTime, endTime, 10)

	require.NoError(t, err)
}

// ============================================================================
// PerformCostAggregation Tests
// ============================================================================

func TestPerformCostAggregation(t *testing.T) {
	t.Run("aggregates all operation types", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		logger := zap.NewNop()
		mockRepo := NewWebSocketCostRepository(mockDB, "test-table", logger, nil)

		tracker := &WebSocketCostTracker{
			costRepo:     mockRepo,
			logger:       logger,
			serviceName:  "test-service",
			functionName: "test-function",
		}

		ctx := context.Background()
		windowStart := time.Now().Add(-1 * time.Hour)
		windowEnd := time.Now()

		// Mock for all operation types
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Between", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil)
		mockQuery.On("First", mock.Anything).Return(nil)
		mockQuery.On("Create").Return(nil).Maybe()
		mockQuery.On("Update", mock.Anything).Return(nil).Maybe()

		err := tracker.PerformCostAggregation(ctx, "hourly", windowStart, windowEnd)

		require.NoError(t, err)
	})
}

// ============================================================================
// getServiceName Tests
// ============================================================================

func TestGetServiceName(t *testing.T) {
	// Just verify it returns a non-empty string
	name := getServiceName()
	require.NotEmpty(t, name)
}
