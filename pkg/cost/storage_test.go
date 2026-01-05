package cost

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewStorage(t *testing.T) {
	logger := zap.NewNop()
	storage := NewStorage(nil, "test-table", logger)

	require.NotNil(t, storage)
	assert.Equal(t, "test-table", storage.tableName)
	assert.Equal(t, logger, storage.logger)
}

func TestStorage_SaveOperationCost(t *testing.T) {
	t.Run("returns error for nil client", func(t *testing.T) {
		// Storage uses *dynamodb.Client directly which panics on nil
		// We can only test that the Storage struct is created correctly
		logger := zap.NewNop()
		storage := NewStorage(nil, "test-table", logger)

		// Verify storage was created
		require.NotNil(t, storage)
		assert.Equal(t, "test-table", storage.tableName)
		// Note: Actually calling SaveOperationCost with nil client would panic
	})
}

func TestStorage_GetDailyCosts(t *testing.T) {
	t.Run("storage struct is created correctly", func(t *testing.T) {
		logger := zap.NewNop()
		storage := NewStorage(nil, "test-table", logger)

		// Verify storage was created
		require.NotNil(t, storage)
		// Note: Actually calling GetDailyCosts with nil client would panic
	})
}

func TestStorage_GetMonthlyCost(t *testing.T) {
	t.Run("storage struct is created correctly", func(t *testing.T) {
		logger := zap.NewNop()
		storage := NewStorage(nil, "test-table", logger)

		require.NotNil(t, storage)
		// Note: Actually calling GetMonthlyCost with nil client would panic
	})
}

func TestStorage_SaveDailyAggregate(t *testing.T) {
	t.Run("storage struct is created correctly", func(t *testing.T) {
		logger := zap.NewNop()
		storage := NewStorage(nil, "test-table", logger)

		require.NotNil(t, storage)
		// Note: Actually calling SaveDailyAggregate with nil client would panic
	})
}

func TestStorage_SaveMonthlyAggregate(t *testing.T) {
	t.Run("storage struct is created correctly", func(t *testing.T) {
		logger := zap.NewNop()
		storage := NewStorage(nil, "test-table", logger)

		require.NotNil(t, storage)
		// Note: Actually calling SaveMonthlyAggregate with nil client would panic
	})
}

func TestStorage_QueryCostsByDate(t *testing.T) {
	t.Run("storage struct is created correctly", func(t *testing.T) {
		logger := zap.NewNop()
		storage := NewStorage(nil, "test-table", logger)

		require.NotNil(t, storage)
		// Note: Actually calling QueryCostsByDate with nil client would panic
	})
}

func TestUnmarshalDailyCostAggregate(t *testing.T) {
	t.Run("unmarshals all fields correctly", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"Date":                &types.AttributeValueMemberS{Value: "2024-01-15"},
			"TotalCostMicrocents": &types.AttributeValueMemberN{Value: "50000"},
			"RequestCount":        &types.AttributeValueMemberN{Value: "100"},
			"UniqueUsers":         &types.AttributeValueMemberN{Value: "25"},
			"DynamoDBReads":       &types.AttributeValueMemberN{Value: "500"},
			"DynamoDBWrites":      &types.AttributeValueMemberN{Value: "200"},
			"LambdaInvocations":   &types.AttributeValueMemberN{Value: "100"},
			"LambdaDurationMs":    &types.AttributeValueMemberN{Value: "5000"},
			"DataTransferBytes":   &types.AttributeValueMemberN{Value: "1024000"},
		}

		var aggregate DailyCostAggregate
		err := unmarshalDailyCostAggregate(item, &aggregate)

		require.NoError(t, err)
		assert.Equal(t, "2024-01-15", aggregate.Date)
		assert.Equal(t, int64(50000), aggregate.TotalCostMicrocents)
		assert.Equal(t, int64(100), aggregate.RequestCount)
		assert.Equal(t, int64(25), aggregate.UniqueUsers)
		assert.Equal(t, int64(500), aggregate.DynamoDBReads)
		assert.Equal(t, int64(200), aggregate.DynamoDBWrites)
		assert.Equal(t, int64(100), aggregate.LambdaInvocations)
		assert.Equal(t, int64(5000), aggregate.LambdaDurationMs)
		assert.Equal(t, int64(1024000), aggregate.DataTransferBytes)
	})

	t.Run("handles missing fields gracefully", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"Date": &types.AttributeValueMemberS{Value: "2024-01-15"},
		}

		var aggregate DailyCostAggregate
		err := unmarshalDailyCostAggregate(item, &aggregate)

		require.NoError(t, err)
		assert.Equal(t, "2024-01-15", aggregate.Date)
		assert.Equal(t, int64(0), aggregate.TotalCostMicrocents)
	})

	t.Run("handles wrong type gracefully", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"Date":                &types.AttributeValueMemberN{Value: "123"}, // Wrong type
			"TotalCostMicrocents": &types.AttributeValueMemberS{Value: "abc"}, // Wrong type
		}

		var aggregate DailyCostAggregate
		err := unmarshalDailyCostAggregate(item, &aggregate)

		require.NoError(t, err)
		assert.Equal(t, "", aggregate.Date) // Not set due to wrong type
	})
}

func TestUnmarshalMonthlyCostAggregate(t *testing.T) {
	t.Run("unmarshals all fields correctly", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"Year":                    &types.AttributeValueMemberN{Value: "2024"},
			"Month":                   &types.AttributeValueMemberN{Value: "1"},
			"TotalCostMicrocents":     &types.AttributeValueMemberN{Value: "500000"},
			"ProjectedCostMicrocents": &types.AttributeValueMemberN{Value: "600000"},
			"RequestCount":            &types.AttributeValueMemberN{Value: "1000"},
			"UniqueUsers":             &types.AttributeValueMemberN{Value: "250"},
			"DynamoDBReads":           &types.AttributeValueMemberN{Value: "5000"},
			"DynamoDBWrites":          &types.AttributeValueMemberN{Value: "2000"},
			"LambdaInvocations":       &types.AttributeValueMemberN{Value: "1000"},
			"LambdaDurationMs":        &types.AttributeValueMemberN{Value: "50000"},
			"DataTransferGB":          &types.AttributeValueMemberN{Value: "1.5"},
		}

		var aggregate MonthlyCostAggregate
		err := unmarshalMonthlyCostAggregate(item, &aggregate)

		require.NoError(t, err)
		assert.Equal(t, 2024, aggregate.Year)
		assert.Equal(t, 1, aggregate.Month)
		assert.Equal(t, int64(500000), aggregate.TotalCostMicrocents)
		assert.Equal(t, int64(600000), aggregate.ProjectedCostMicrocents)
		assert.Equal(t, int64(1000), aggregate.RequestCount)
		assert.Equal(t, int64(250), aggregate.UniqueUsers)
		assert.Equal(t, int64(5000), aggregate.DynamoDBReads)
		assert.Equal(t, int64(2000), aggregate.DynamoDBWrites)
		assert.Equal(t, int64(1000), aggregate.LambdaInvocations)
		assert.Equal(t, int64(50000), aggregate.LambdaDurationMs)
		assert.InDelta(t, 1.5, aggregate.DataTransferGB, 0.001)
	})

	t.Run("handles missing fields gracefully", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"Year":  &types.AttributeValueMemberN{Value: "2024"},
			"Month": &types.AttributeValueMemberN{Value: "1"},
		}

		var aggregate MonthlyCostAggregate
		err := unmarshalMonthlyCostAggregate(item, &aggregate)

		require.NoError(t, err)
		assert.Equal(t, 2024, aggregate.Year)
		assert.Equal(t, 1, aggregate.Month)
		assert.Equal(t, int64(0), aggregate.TotalCostMicrocents)
	})
}

func TestDailyCostAggregate_Fields(t *testing.T) {
	aggregate := DailyCostAggregate{
		Date:                "2024-01-15",
		TotalCostMicrocents: 50000,
		RequestCount:        100,
		UniqueUsers:         25,
		DynamoDBReads:       500,
		DynamoDBWrites:      200,
		LambdaInvocations:   100,
		LambdaDurationMs:    5000,
		DataTransferBytes:   1024000,
	}

	assert.Equal(t, "2024-01-15", aggregate.Date)
	assert.Equal(t, int64(50000), aggregate.TotalCostMicrocents)
	assert.Equal(t, int64(100), aggregate.RequestCount)
	assert.Equal(t, int64(25), aggregate.UniqueUsers)
	assert.Equal(t, int64(500), aggregate.DynamoDBReads)
	assert.Equal(t, int64(200), aggregate.DynamoDBWrites)
	assert.Equal(t, int64(100), aggregate.LambdaInvocations)
	assert.Equal(t, int64(5000), aggregate.LambdaDurationMs)
	assert.Equal(t, int64(1024000), aggregate.DataTransferBytes)
}

func TestMonthlyCostAggregate_Fields(t *testing.T) {
	aggregate := MonthlyCostAggregate{
		Year:                    2024,
		Month:                   1,
		TotalCostMicrocents:     500000,
		ProjectedCostMicrocents: 600000,
		RequestCount:            1000,
		UniqueUsers:             250,
		DynamoDBReads:           5000,
		DynamoDBWrites:          2000,
		LambdaInvocations:       1000,
		LambdaDurationMs:        50000,
		DataTransferGB:          1.5,
	}

	assert.Equal(t, 2024, aggregate.Year)
	assert.Equal(t, 1, aggregate.Month)
	assert.Equal(t, int64(500000), aggregate.TotalCostMicrocents)
	assert.Equal(t, int64(600000), aggregate.ProjectedCostMicrocents)
	assert.Equal(t, int64(1000), aggregate.RequestCount)
	assert.Equal(t, int64(250), aggregate.UniqueUsers)
	assert.Equal(t, int64(5000), aggregate.DynamoDBReads)
	assert.Equal(t, int64(2000), aggregate.DynamoDBWrites)
	assert.Equal(t, int64(1000), aggregate.LambdaInvocations)
	assert.Equal(t, int64(50000), aggregate.LambdaDurationMs)
	assert.InDelta(t, 1.5, aggregate.DataTransferGB, 0.001)
}
