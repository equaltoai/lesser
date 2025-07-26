package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	. "github.com/equaltoai/lesser/pkg/storage/dynamorm/repositories/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestDefaultCostThresholds(t *testing.T) {
	thresholds := DefaultCostThresholds()

	assert.Equal(t, 0.0001, thresholds.WarningCostPerOp)
	assert.Equal(t, 0.001, thresholds.MaxCostPerOp)
	assert.Equal(t, 0.001, thresholds.WarningCostPerRequest)
	assert.Equal(t, 0.01, thresholds.MaxCostPerRequest)
	assert.Equal(t, 0.01, thresholds.WarningCostPerMinute)
	assert.Equal(t, 0.1, thresholds.MaxCostPerMinute)
	assert.Equal(t, 100, thresholds.MaxOperationsPerRequest)
	assert.Equal(t, 1000, thresholds.MaxOperationsPerMinute)
}

func TestNewCostAwareRepository(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()
	tracker := cost.New()
	tableName := "test-table"

	repo := NewCostAwareRepository(mockDB, tableName, logger, tracker)

	assert.NotNil(t, repo)
	assert.NotNil(t, repo.BaseRepository)
	assert.NotNil(t, repo.costTracker)
	assert.Equal(t, DefaultCostThresholds(), repo.costThresholds)
	assert.Equal(t, logger, repo.logger)
	assert.NotNil(t, repo.operationStats)
}

func TestNewCostAwareRepositoryWithRequest(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()
	tracker := cost.New()
	tableName := "test-table"
	requestID := "req-123"
	operationType := "api_call"

	repo := NewCostAwareRepositoryWithRequest(mockDB, tableName, requestID, operationType, logger, tracker)

	assert.NotNil(t, repo)
	assert.NotNil(t, repo.BaseRepository)
	assert.NotNil(t, repo.costTracker)
	assert.Equal(t, tableName, repo.GetTableName())
}

func TestCostAwareRepository_SetCostThresholds(t *testing.T) {
	mockDB := &MockDB{}
	repo := NewCostAwareRepository(mockDB, "test", zap.NewNop(), cost.New())

	customThresholds := CostThresholds{
		WarningCostPerOp:        0.0005,
		MaxCostPerOp:            0.002,
		WarningCostPerRequest:   0.005,
		MaxCostPerRequest:       0.02,
		WarningCostPerMinute:    0.05,
		MaxCostPerMinute:        0.2,
		MaxOperationsPerRequest: 50,
		MaxOperationsPerMinute:  500,
	}

	repo.SetCostThresholds(customThresholds)

	assert.Equal(t, customThresholds, repo.costThresholds)
}

func TestCostAwareRepository_UpdateOperationStats(t *testing.T) {
	mockDB := &MockDB{}
	repo := NewCostAwareRepository(mockDB, "test", zap.NewNop(), cost.New())

	operationName := "test_operation"
	operationCost := 0.001
	duration := 100 * time.Millisecond

	// Update stats multiple times
	repo.updateOperationStats(operationName, operationCost, duration, false)
	repo.updateOperationStats(operationName, operationCost*2, duration*2, false)
	repo.updateOperationStats(operationName, operationCost, duration, true) // with error

	stats := repo.GetOperationStats()

	assert.Contains(t, stats, operationName)
	opStats := stats[operationName]

	assert.Equal(t, int64(3), opStats.TotalOperations)
	assert.Equal(t, operationCost*4, opStats.TotalCost) // 0.001 + 0.002 + 0.001
	assert.Equal(t, int64(1), opStats.ErrorCount)
	assert.Equal(t, operationCost*4/3, opStats.AverageCost)
}

func TestCostAwareRepository_GetCostSummary(t *testing.T) {
	mockDB := &MockDB{}
	repo := NewCostAwareRepository(mockDB, "test_table", zap.NewNop(), cost.New())

	// Add some operation stats
	repo.updateOperationStats("get", 0.001, 50*time.Millisecond, false)
	repo.updateOperationStats("get", 0.002, 75*time.Millisecond, false)
	repo.updateOperationStats("create", 0.005, 100*time.Millisecond, false)
	repo.updateOperationStats("create", 0.003, 80*time.Millisecond, true) // with error

	summary := repo.GetCostSummary()

	assert.Equal(t, "test_table", summary.TableName)
	assert.Equal(t, int64(4), summary.TotalOperations)
	assert.Equal(t, 0.011, summary.TotalCost) // 0.001 + 0.002 + 0.005 + 0.003
	assert.Equal(t, int64(1), summary.TotalErrors)
	assert.Equal(t, 0.011/4, summary.AverageCostPerOperation)
	assert.Equal(t, 1.0/4, summary.ErrorRate) // 1 error out of 4 operations

	// Check operation summaries
	assert.Contains(t, summary.OperationSummary, "get")
	assert.Contains(t, summary.OperationSummary, "create")

	getStats := summary.OperationSummary["get"]
	assert.Equal(t, int64(2), getStats.Count)
	assert.Equal(t, 0.003, getStats.TotalCost)
	assert.Equal(t, 0.0, getStats.ErrorRate)

	createStats := summary.OperationSummary["create"]
	assert.Equal(t, int64(2), createStats.Count)
	assert.Equal(t, 0.008, createStats.TotalCost)
	assert.Equal(t, 0.5, createStats.ErrorRate) // 1 error out of 2 operations
}

func TestCostAwareRepository_CheckCostAlerts(t *testing.T) {
	mockDB := &MockDB{}
	repo := NewCostAwareRepository(mockDB, "test_table", zap.NewNop(), cost.New())

	// Set low thresholds to trigger alerts
	repo.SetCostThresholds(CostThresholds{
		WarningCostPerOp: 0.001,
		MaxCostPerOp:     0.002,
	})

	// Add operations that will trigger alerts
	repo.updateOperationStats("expensive_op", 0.0015, 100*time.Millisecond, false)     // Warning
	repo.updateOperationStats("very_expensive_op", 0.003, 200*time.Millisecond, false) // Critical
	repo.updateOperationStats("error_prone_op", 0.0005, 50*time.Millisecond, true)     // Error
	repo.updateOperationStats("error_prone_op", 0.0005, 50*time.Millisecond, true)     // Another error

	alerts := repo.CheckCostAlerts()

	assert.Len(t, alerts, 3) // Warning, Critical, and Error rate alerts

	// Check for critical alert
	criticalAlerts := make([]*CostAlert, 0)
	warningAlerts := make([]*CostAlert, 0)
	errorAlerts := make([]*CostAlert, 0)

	for _, alert := range alerts {
		switch alert.Severity {
		case "critical":
			criticalAlerts = append(criticalAlerts, alert)
		case "warning":
			if alert.AlertType == "average_operation_cost" {
				warningAlerts = append(warningAlerts, alert)
			} else {
				errorAlerts = append(errorAlerts, alert)
			}
		}
	}

	assert.Len(t, criticalAlerts, 1)
	assert.Equal(t, "very_expensive_op", criticalAlerts[0].Operation)
	assert.Equal(t, "average_operation_cost", criticalAlerts[0].AlertType)

	assert.Len(t, warningAlerts, 1)
	assert.Equal(t, "expensive_op", warningAlerts[0].Operation)

	assert.Len(t, errorAlerts, 1)
	assert.Equal(t, "error_prone_op", errorAlerts[0].Operation)
	assert.Equal(t, "error_rate", errorAlerts[0].AlertType)
}

func TestCostAwareRepository_ResetStats(t *testing.T) {
	mockDB := &MockDB{}
	repo := NewCostAwareRepository(mockDB, "test", zap.NewNop(), cost.New())

	// Add some stats
	repo.updateOperationStats("test_op", 0.001, 100*time.Millisecond, false)

	stats := repo.GetOperationStats()
	assert.Len(t, stats, 1)

	// Reset stats
	repo.ResetStats()

	stats = repo.GetOperationStats()
	assert.Len(t, stats, 0)
}

func TestCostAwareRepository_OptimizeQuery(t *testing.T) {
	mockDB := &MockDB{}
	repo := NewCostAwareRepository(mockDB, "test", zap.NewNop(), cost.New())

	// Create a simple mock query that just returns basic suggestions
	// Since OptimizeQuery doesn't actually use the query parameter in the implementation
	suggestion := repo.OptimizeQuery(context.Background(), nil)

	assert.NotNil(t, suggestion)
	assert.NotEmpty(t, suggestion.Suggestions)
	assert.Contains(t, suggestion.Suggestions[0], "projections")
}

// Test context integration

func TestWithCostAwareRepository(t *testing.T) {
	mockDB := &MockDB{}
	repo := NewCostAwareRepository(mockDB, "test", zap.NewNop(), cost.New())

	ctx := WithCostAwareRepository(context.Background(), repo)

	retrievedRepo := FromContext(ctx)
	assert.Equal(t, repo, retrievedRepo)
}

func TestFromContext_NotFound(t *testing.T) {
	ctx := context.Background()

	retrievedRepo := FromContext(ctx)
	assert.Nil(t, retrievedRepo)
}

// Test NewCostAwareQuery

func TestNewCostAwareQuery(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}
	repo := NewCostAwareRepository(mockDB, "test", zap.NewNop(), cost.New())

	mockDB.On("Model", mock.Anything).Return(mockQuery)

	model := map[string]any{"id": "test"}
	ctx := context.Background()

	costAwareQuery := repo.NewCostAwareQuery(ctx, model)

	assert.NotNil(t, costAwareQuery)
	assert.Equal(t, mockQuery, costAwareQuery.query)
	assert.Equal(t, repo, costAwareQuery.repository)
	assert.Equal(t, ctx, costAwareQuery.ctx)
}

func TestCostAwareQuery_Chaining(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}
	repo := NewCostAwareRepository(mockDB, "test", zap.NewNop(), cost.New())

	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "field", "=", "value").Return(mockQuery)
	mockQuery.On("Index", "test-index").Return(mockQuery)
	mockQuery.On("Limit", 10).Return(mockQuery)

	model := map[string]any{"id": "test"}
	ctx := context.Background()

	costAwareQuery := repo.NewCostAwareQuery(ctx, model).
		Where("field", "=", "value").
		Index("test-index").
		Limit(10)

	assert.NotNil(t, costAwareQuery)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// Test actual cost tracking operations (these will be limited by mock constraints)

func TestCostAwareRepository_CreateWithCostTracking_MockLimitations(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}
	repo := NewCostAwareRepository(mockDB, "test", zap.NewNop(), cost.New())

	model := map[string]any{"id": "test"}
	ctx := context.Background()

	mockDB.On("Model", model).Return(mockQuery)
	mockQuery.On("Create").Return(errors.New("mock limitation"))

	err := repo.CreateWithCostTracking(ctx, model)

	// Due to mock limitations, this will fail, but we can verify the method structure
	assert.Error(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)

	// Verify stats were updated despite the error
	stats := repo.GetOperationStats()
	assert.Contains(t, stats, "create")
	assert.Equal(t, int64(1), stats["create"].TotalOperations)
	assert.Equal(t, int64(1), stats["create"].ErrorCount)
}

func TestCostAwareRepository_GetWithCostTracking_MockLimitations(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}
	repo := NewCostAwareRepository(mockDB, "test", zap.NewNop(), cost.New())

	model := map[string]any{"id": "test"}
	key := map[string]any{"pk": "test-key"}
	ctx := context.Background()

	mockDB.On("Model", model).Return(mockQuery)
	mockQuery.On("Where", "pk", "=", "test-key").Return(mockQuery)
	mockQuery.On("First", model).Return(nil)

	err := repo.GetWithCostTracking(ctx, model, key)

	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)

	// Verify stats were updated
	stats := repo.GetOperationStats()
	assert.Contains(t, stats, "get")
	assert.Equal(t, int64(1), stats["get"].TotalOperations)
	assert.Equal(t, int64(0), stats["get"].ErrorCount)
}

// Benchmark tests

func BenchmarkCostAwareRepository_UpdateOperationStats(b *testing.B) {
	mockDB := &MockDB{}
	repo := NewCostAwareRepository(mockDB, "test", zap.NewNop(), cost.New())

	operationName := "benchmark_op"
	operationCost := 0.001
	duration := 50 * time.Millisecond

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repo.updateOperationStats(operationName, operationCost, duration, false)
	}
}

func BenchmarkCostAwareRepository_GetOperationStats(b *testing.B) {
	mockDB := &MockDB{}
	repo := NewCostAwareRepository(mockDB, "test", zap.NewNop(), cost.New())

	// Pre-populate with stats
	for i := 0; i < 100; i++ {
		repo.updateOperationStats("test_op", 0.001, 50*time.Millisecond, false)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repo.GetOperationStats()
	}
}

func BenchmarkCostAwareRepository_CheckCostAlerts(b *testing.B) {
	mockDB := &MockDB{}
	repo := NewCostAwareRepository(mockDB, "test", zap.NewNop(), cost.New())

	// Pre-populate with stats that will trigger alerts
	repo.updateOperationStats("expensive_op", 0.01, 100*time.Millisecond, false)
	repo.updateOperationStats("error_op", 0.001, 50*time.Millisecond, true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repo.CheckCostAlerts()
	}
}
