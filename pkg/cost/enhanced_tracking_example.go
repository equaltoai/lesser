package cost

import (
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// ExampleEnhancedUsage demonstrates how to use the enhanced cost tracking
func ExampleEnhancedUsage() {
	logger := zap.NewNop()

	// Create base tracker
	baseTracker := New()

	// Create a mock DB for demonstration
	// In real usage, this would be your actual DynamORM client
	var mockDB core.DB

	// Create enhanced tracking database wrapper
	trackingDB := NewTrackingDB(mockDB, baseTracker, logger)

	// This query will now track:
	// - Operation type (Query vs Scan)
	// - GSI usage when Index() is called
	// - Filter expressions and their impact
	// - Projection fields optimization
	// - Consistent read cost multiplier
	// - Actual item counts returned

	type ExampleModel struct {
		ID   string `dynamorm:"pk"`
		Name string `json:"name"`
	}

	var results []ExampleModel

	// Enhanced tracking captures all these details automatically
	query := trackingDB.Model(&ExampleModel{}).
		Index("name-index").             // GSI usage tracked
		Where("Name", "=", "test").      // Query condition tracked
		Filter("Status", "=", "active"). // Filter expression tracked
		Select("ID", "Name").            // Projection tracked
		ConsistentRead().                // 2x cost multiplier tracked
		Limit(10)                        // Limit tracked

	_ = query.All(&results) // Actual item count will be tracked

	// The enhanced tracker now contains detailed metadata about:
	// - Exact operation type
	// - GSI name and usage
	// - Filter expressions that may increase cost
	// - Projection optimization opportunities
	// - Consistent read cost impact
	// - Precise item counts for accurate cost calculation
}

// EnhancedCostAnalysis shows how to analyze tracked operations
func EnhancedCostAnalysis(tracker *EnhancedOperationTracker) {
	operations := tracker.GetAllOperations()

	for operationID, metadata := range operations {
		// Analyze cost optimization opportunities
		var suggestions []string

		// Check for expensive scan operations
		if metadata.OperationType == "Scan" && len(metadata.FilterExpressions) > 0 {
			suggestions = append(suggestions,
				"Consider using a GSI instead of scanning with filters")
		}

		// Check for missing projections on large result sets
		if metadata.ItemCount > 100 && len(metadata.ProjectionFields) == 0 {
			suggestions = append(suggestions,
				"Consider using Select() to project only needed fields")
		}

		// Check for expensive consistent reads
		if metadata.ConsistentRead && metadata.ItemCount > 10 {
			suggestions = append(suggestions,
				"Evaluate if eventual consistency is acceptable to reduce costs")
		}

		// Check GSI usage efficiency
		if metadata.IndexName != "" && len(metadata.Conditions) < 2 {
			suggestions = append(suggestions,
				"Ensure GSI key design supports efficient queries")
		}

		// Log analysis results
		logger := zap.NewNop()
		logger.Info("operation_cost_analysis",
			zap.String("operation_id", operationID),
			zap.String("operation_type", metadata.OperationType),
			zap.String("table_name", metadata.TableName),
			zap.String("index_name", metadata.IndexName),
			zap.Int("item_count", metadata.ItemCount),
			zap.Int("filter_count", len(metadata.FilterExpressions)),
			zap.Int("projection_count", len(metadata.ProjectionFields)),
			zap.Bool("consistent_read", metadata.ConsistentRead),
			zap.Strings("optimization_suggestions", suggestions),
		)
	}
}

// BatchOperationExample demonstrates enhanced batch operation tracking
func BatchOperationExample(trackingDB *TrackingDB) {
	type ExampleItem struct {
		ID   string `dynamorm:"pk"`
		Data string `json:"data"`
	}

	// Create multiple items
	items := make([]ExampleItem, 50) // 50 items
	for i := range items {
		items[i] = ExampleItem{
			ID:   string(rune(i)),
			Data: "test data",
		}
	}

	// Enhanced batch tracking will:
	// - Count exact number of items (50)
	// - Calculate batch count (2 batches of 25 each)
	// - Track precise write capacity units (50 WCUs)
	// - Log detailed batch operation metadata

	query := trackingDB.Model(&ExampleItem{})
	_ = query.BatchCreate(items)

	// The tracker now contains precise information about:
	// - Actual item count: 50
	// - Calculated write units: 50
	// - Batch count: 2 (due to 25-item DynamoDB limit)
	// - Operation type: BatchWriteItem
}

// TransactionExample demonstrates transaction cost tracking
func TransactionExample(trackingDB *TrackingDB) {
	// Transaction tracking uses conservative estimates since implementing
	// full transaction operation counting would require complex interface wrapping

	_ = trackingDB.Transaction(func(_ *core.Tx) error {
		// Operations within this transaction are estimated
		// Future enhancement: Implement precise transaction operation counting
		return nil
	})

	// Current approach:
	// - Uses conservative estimates (3 operations average)
	// - Tracks both read and write capacity
	// - Logs estimation approach for transparency
	// - Maintains performance while providing useful cost tracking
}

// RealTimeOptimization shows how enhanced tracking enables real-time optimization
func RealTimeOptimization(tracker *EnhancedOperationTracker, logger *zap.Logger) {
	operations := tracker.GetAllOperations()

	// Real-time cost optimization recommendations
	for _, metadata := range operations {
		// Calculate cost impact
		estimatedCost := calculateOperationCost(metadata)

		if estimatedCost > 0.001 { // High cost operation (> $0.001)
			logger.Warn("high_cost_operation_detected",
				zap.String("operation_type", metadata.OperationType),
				zap.String("table_name", metadata.TableName),
				zap.Float64("estimated_cost", estimatedCost),
				zap.Int("item_count", metadata.ItemCount),
				zap.Bool("has_filters", len(metadata.FilterExpressions) > 0),
				zap.Bool("has_projections", len(metadata.ProjectionFields) > 0),
				zap.String("optimization_priority", "immediate"),
			)
		}
	}
}

// calculateOperationCost estimates the cost of an operation based on metadata
func calculateOperationCost(metadata *OperationMetadata) float64 {
	baseCost := 0.0

	switch metadata.OperationType {
	case "GetItem":
		baseCost = 0.000025 // $0.25 per million read units
	case "PutItem", "UpdateItem", "DeleteItem":
		baseCost = 0.000125 // $1.25 per million write units
	case "Query":
		// Query cost depends on items scanned vs returned
		baseCost = float64(metadata.ItemCount) * 0.000025
		if len(metadata.FilterExpressions) > 0 {
			baseCost *= 1.5 // Filter expressions increase cost
		}
	case "Scan":
		// Scan is expensive, especially with filters
		baseCost = float64(metadata.ItemCount) * 0.000025 * 2.0
		if len(metadata.FilterExpressions) > 0 {
			baseCost *= 2.0 // Filters are very expensive on scans
		}
	case "BatchWriteItem":
		baseCost = float64(metadata.ItemCount) * 0.000125
	}

	// Consistent read doubles the cost
	if metadata.ConsistentRead {
		baseCost *= 2.0
	}

	return baseCost
}
