package patterns

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap/zaptest"
)

// MockDynamoDBClient is a mock implementation of DynamoDBClient
type MockDynamoDBClient struct {
	mock.Mock
}

func (m *MockDynamoDBClient) GetItem(input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*dynamodb.GetItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) PutItem(input *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*dynamodb.PutItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) UpdateItem(input *dynamodb.UpdateItemInput) (*dynamodb.UpdateItemOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*dynamodb.UpdateItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) DeleteItem(input *dynamodb.DeleteItemInput) (*dynamodb.DeleteItemOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*dynamodb.DeleteItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) Query(input *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*dynamodb.QueryOutput), args.Error(1)
}

func (m *MockDynamoDBClient) Scan(input *dynamodb.ScanInput) (*dynamodb.ScanOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*dynamodb.ScanOutput), args.Error(1)
}

func (m *MockDynamoDBClient) BatchGetItem(input *dynamodb.BatchGetItemInput) (*dynamodb.BatchGetItemOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*dynamodb.BatchGetItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) BatchWriteItem(input *dynamodb.BatchWriteItemInput) (*dynamodb.BatchWriteItemOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*dynamodb.BatchWriteItemOutput), args.Error(1)
}

func TestSoftDeleteModel_SoftDelete(t *testing.T) {
	model := &SoftDeleteModel{}
	
	// Initially not deleted
	assert.False(t, model.IsDeleted())
	assert.Nil(t, model.GetDeletedAt())
	assert.Empty(t, model.GetDeletedBy())

	// Soft delete
	model.SoftDelete()
	
	assert.True(t, model.IsDeleted())
	assert.NotNil(t, model.GetDeletedAt())
	assert.True(t, time.Since(*model.GetDeletedAt()) < time.Second)
	assert.Empty(t, model.GetDeletedBy()) // SoftDelete doesn't set user
}

func TestSoftDeleteModel_SoftDeleteBy(t *testing.T) {
	model := &SoftDeleteModel{}
	userID := "user123"
	
	// Soft delete by user
	model.SoftDeleteBy(userID)
	
	assert.True(t, model.IsDeleted())
	assert.NotNil(t, model.GetDeletedAt())
	assert.Equal(t, userID, model.GetDeletedBy())
	assert.True(t, time.Since(*model.GetDeletedAt()) < time.Second)
}

func TestSoftDeleteModel_Restore(t *testing.T) {
	model := &SoftDeleteModel{}
	
	// Soft delete first
	model.SoftDeleteBy("user123")
	assert.True(t, model.IsDeleted())
	
	// Restore
	model.Restore()
	
	assert.False(t, model.IsDeleted())
	assert.Nil(t, model.GetDeletedAt())
	assert.Empty(t, model.GetDeletedBy())
}

func TestSoftDeleteModel_SettersGetters(t *testing.T) {
	model := &SoftDeleteModel{}
	now := time.Now()
	userID := "user456"
	
	// Test setters
	model.SetDeletedAt(&now)
	model.SetDeletedBy(userID)
	
	// Test getters
	assert.Equal(t, &now, model.GetDeletedAt())
	assert.Equal(t, userID, model.GetDeletedBy())
	assert.True(t, model.IsDeleted())
}

func TestExampleModel_ImplementsSoftDeletable(t *testing.T) {
	model := NewExampleModel("id123", "John Doe", "john@example.com")
	
	// Verify it implements SoftDeletable
	var _ SoftDeletable = model
	
	// Test initial state
	assert.False(t, model.IsDeleted())
	assert.Equal(t, "id123", model.ID)
	assert.Equal(t, "John Doe", model.Name)
	assert.Equal(t, "john@example.com", model.Email)
	
	// Test soft delete
	model.SoftDeleteBy("admin")
	assert.True(t, model.IsDeleted())
	assert.Equal(t, "admin", model.GetDeletedBy())
	
	// Test restore
	model.Restore()
	assert.False(t, model.IsDeleted())
}

func TestSoftDeleteRepository_NewSoftDeleteRepository(t *testing.T) {
	client := &MockDynamoDBClient{}
	logger := zaptest.NewLogger(t)
	tableName := "test-table"
	
	repo := NewSoftDeleteRepository(client, tableName, logger)
	
	assert.NotNil(t, repo)
	assert.Equal(t, client, repo.client)
	assert.Equal(t, tableName, repo.tableName)
	assert.Equal(t, logger, repo.logger)
	assert.False(t, repo.includeDeleted)
}

func TestSoftDeleteRepository_WithDeleted(t *testing.T) {
	client := &MockDynamoDBClient{}
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(client, "test-table", logger)
	
	withDeletedRepo := repo.WithDeleted()
	
	assert.NotSame(t, repo, withDeletedRepo) // Different instances
	assert.True(t, withDeletedRepo.includeDeleted)
	assert.False(t, repo.includeDeleted) // Original unchanged
}

func TestSoftDeleteRepository_OnlyDeleted(t *testing.T) {
	client := &MockDynamoDBClient{}
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(client, "test-table", logger)
	
	onlyDeletedRepo := repo.OnlyDeleted()
	
	assert.NotSame(t, repo, onlyDeletedRepo) // Different instances
	assert.False(t, onlyDeletedRepo.includeDeleted) // Will be handled in query methods
}

func TestSoftDeleteRepository_HardDelete(t *testing.T) {
	client := &MockDynamoDBClient{}
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(client, "test-table", logger)
	
	keys := map[string]*dynamodb.AttributeValue{
		"pk": {S: aws.String("test-id")},
	}
	
	// Mock the DeleteItem call
	client.On("DeleteItem", mock.MatchedBy(func(input *dynamodb.DeleteItemInput) bool {
		return *input.TableName == "test-table" && 
			   input.Key["pk"] != nil && 
			   *input.Key["pk"].S == "test-id"
	})).Return(&dynamodb.DeleteItemOutput{}, nil)
	
	err := repo.HardDelete(context.Background(), keys)
	assert.NoError(t, err)
	
	client.AssertExpectations(t)
}

func TestSoftDeleteRepository_Get(t *testing.T) {
	client := &MockDynamoDBClient{}
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(client, "test-table", logger)
	
	keys := map[string]*dynamodb.AttributeValue{
		"pk": {S: aws.String("test-id")},
	}
	
	t.Run("get active item", func(t *testing.T) {
		// Mock GetItem response with active item
		activeItem := map[string]*dynamodb.AttributeValue{
			"pk":   {S: aws.String("test-id")},
			"name": {S: aws.String("Test Item")},
		}
		
		client.On("GetItem", mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
			return *input.TableName == "test-table"
		})).Return(&dynamodb.GetItemOutput{
			Item: activeItem,
		}, nil).Once()
		
		result, err := repo.Get(context.Background(), keys)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "test-id", *result["pk"].S)
	})
	
	t.Run("get soft deleted item - filtered out", func(t *testing.T) {
		// Mock GetItem response with soft-deleted item
		deletedItem := map[string]*dynamodb.AttributeValue{
			"pk":         {S: aws.String("test-id")},
			"name":       {S: aws.String("Test Item")},
			"deleted_at": {S: aws.String("2023-10-15T14:30:00Z")},
		}
		
		client.On("GetItem", mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
			return *input.TableName == "test-table"
		})).Return(&dynamodb.GetItemOutput{
			Item: deletedItem,
		}, nil).Once()
		
		result, err := repo.Get(context.Background(), keys)
		assert.NoError(t, err)
		assert.Nil(t, result) // Should be filtered out
	})
	
	t.Run("get soft deleted item - included when WithDeleted", func(t *testing.T) {
		withDeletedRepo := repo.WithDeleted()
		
		// Mock GetItem response with soft-deleted item
		deletedItem := map[string]*dynamodb.AttributeValue{
			"pk":         {S: aws.String("test-id")},
			"name":       {S: aws.String("Test Item")},
			"deleted_at": {S: aws.String("2023-10-15T14:30:00Z")},
		}
		
		client.On("GetItem", mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
			return *input.TableName == "test-table"
		})).Return(&dynamodb.GetItemOutput{
			Item: deletedItem,
		}, nil).Once()
		
		result, err := withDeletedRepo.Get(context.Background(), keys)
		assert.NoError(t, err)
		assert.NotNil(t, result) // Should be included
		assert.Equal(t, "test-id", *result["pk"].S)
	})
	
	t.Run("item not found", func(t *testing.T) {
		client.On("GetItem", mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
			return *input.TableName == "test-table"
		})).Return(&dynamodb.GetItemOutput{
			Item: nil,
		}, nil).Once()
		
		result, err := repo.Get(context.Background(), keys)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestSoftDeleteRepository_Query(t *testing.T) {
	client := &MockDynamoDBClient{}
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(client, "test-table", logger)
	
	queryInput := &dynamodb.QueryInput{
		TableName: aws.String("test-table"),
	}
	
	t.Run("query active items only", func(t *testing.T) {
		// Mock query response with mixed items
		items := []map[string]*dynamodb.AttributeValue{
			{
				"pk":   {S: aws.String("item1")},
				"name": {S: aws.String("Active Item 1")},
			},
			{
				"pk":         {S: aws.String("item2")},
				"name":       {S: aws.String("Deleted Item")},
				"deleted_at": {S: aws.String("2023-10-15T14:30:00Z")},
			},
			{
				"pk":   {S: aws.String("item3")},
				"name": {S: aws.String("Active Item 2")},
			},
		}
		
		client.On("Query", mock.AnythingOfType("*dynamodb.QueryInput")).Return(&dynamodb.QueryOutput{
			Items: items,
			Count: aws.Int64(3),
		}, nil).Once()
		
		result, err := repo.Query(context.Background(), queryInput)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(result.Items)) // Only active items
		assert.Equal(t, "item1", *result.Items[0]["pk"].S)
		assert.Equal(t, "item3", *result.Items[1]["pk"].S)
	})
	
	t.Run("query with deleted items included", func(t *testing.T) {
		withDeletedRepo := repo.WithDeleted()
		
		items := []map[string]*dynamodb.AttributeValue{
			{
				"pk":   {S: aws.String("item1")},
				"name": {S: aws.String("Active Item")},
			},
			{
				"pk":         {S: aws.String("item2")},
				"name":       {S: aws.String("Deleted Item")},
				"deleted_at": {S: aws.String("2023-10-15T14:30:00Z")},
			},
		}
		
		client.On("Query", mock.AnythingOfType("*dynamodb.QueryInput")).Return(&dynamodb.QueryOutput{
			Items: items,
			Count: aws.Int64(2),
		}, nil).Once()
		
		result, err := withDeletedRepo.Query(context.Background(), queryInput)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(result.Items)) // All items included
	})
}

func TestSoftDeleteRepository_QueryOnlyDeleted(t *testing.T) {
	client := &MockDynamoDBClient{}
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(client, "test-table", logger)
	
	queryInput := &dynamodb.QueryInput{
		TableName: aws.String("test-table"),
	}
	
	// Mock query response with only deleted items
	deletedItems := []map[string]*dynamodb.AttributeValue{
		{
			"pk":         {S: aws.String("item1")},
			"name":       {S: aws.String("Deleted Item 1")},
			"deleted_at": {S: aws.String("2023-10-15T14:30:00Z")},
		},
		{
			"pk":         {S: aws.String("item2")},
			"name":       {S: aws.String("Deleted Item 2")},
			"deleted_at": {S: aws.String("2023-10-16T14:30:00Z")},
		},
	}
	
	client.On("Query", mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
		// Should have filter for only deleted items
		return input.FilterExpression != nil
	})).Return(&dynamodb.QueryOutput{
		Items: deletedItems,
		Count: aws.Int64(2),
	}, nil).Once()
	
	result, err := repo.QueryOnlyDeleted(context.Background(), queryInput)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(result.Items))
	
	// Verify all items have deleted_at
	for _, item := range result.Items {
		assert.NotNil(t, item["deleted_at"])
	}
}

func TestSoftDeleteRepository_GetSoftDeleteStats(t *testing.T) {
	client := &MockDynamoDBClient{}
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(client, "test-table", logger)
	
	// Mock total count scan
	client.On("Scan", mock.MatchedBy(func(input *dynamodb.ScanInput) bool {
		return *input.Select == "COUNT" && input.FilterExpression == nil
	})).Return(&dynamodb.ScanOutput{
		Count: aws.Int64(100),
	}, nil).Once()
	
	// Mock deleted count scan
	client.On("Scan", mock.MatchedBy(func(input *dynamodb.ScanInput) bool {
		return *input.Select == "COUNT" && input.FilterExpression != nil
	})).Return(&dynamodb.ScanOutput{
		Count: aws.Int64(15),
	}, nil).Once()
	
	stats, err := repo.GetSoftDeleteStats(context.Background())
	assert.NoError(t, err)
	
	assert.Equal(t, 100, stats.TotalItems)
	assert.Equal(t, 15, stats.DeletedItems)
	assert.Equal(t, 85, stats.ActiveItems)
	assert.Equal(t, 15.0, stats.GetDeletionPercentage())
}

func TestSoftDeleteRepository_CleanupOldDeletes(t *testing.T) {
	client := &MockDynamoDBClient{}
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(client, "test-table", logger)
	
	// Create old deleted items
	oldDeletedItems := []map[string]*dynamodb.AttributeValue{
		{
			"pk":         {S: aws.String("item1")},
			"sk":         {S: aws.String("sort1")},
			"name":       {S: aws.String("Old Item 1")},
			"deleted_at": {S: aws.String("2023-09-01T14:30:00Z")},
		},
		{
			"pk":         {S: aws.String("item2")},
			"sk":         {S: aws.String("sort2")},
			"name":       {S: aws.String("Old Item 2")},
			"deleted_at": {S: aws.String("2023-09-02T14:30:00Z")},
		},
	}
	
	// Mock scan for old items
	client.On("Scan", mock.MatchedBy(func(input *dynamodb.ScanInput) bool {
		return input.FilterExpression != nil
	})).Return(&dynamodb.ScanOutput{
		Items: oldDeletedItems,
	}, nil).Once()
	
	// Mock batch delete
	client.On("BatchWriteItem", mock.MatchedBy(func(input *dynamodb.BatchWriteItemInput) bool {
		return len(input.RequestItems["test-table"]) == 2
	})).Return(&dynamodb.BatchWriteItemOutput{
		UnprocessedItems: map[string][]*dynamodb.WriteRequest{},
	}, nil).Once()
	
	deleted, err := repo.CleanupOldDeletes(context.Background(), 30*24*time.Hour, 25)
	assert.NoError(t, err)
	assert.Equal(t, 2, deleted)
	
	client.AssertExpectations(t)
}

func TestSoftDeleteRepository_GetDeletedItemsOlderThan(t *testing.T) {
	client := &MockDynamoDBClient{}
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(client, "test-table", logger)
	
	oldDeletedItems := []map[string]*dynamodb.AttributeValue{
		{
			"pk":         {S: aws.String("item1")},
			"name":       {S: aws.String("Old Item 1")},
			"deleted_at": {S: aws.String("2023-09-01T14:30:00Z")},
		},
	}
	
	// Mock scan for old items
	client.On("Scan", mock.MatchedBy(func(input *dynamodb.ScanInput) bool {
		return input.FilterExpression != nil
	})).Return(&dynamodb.ScanOutput{
		Items: oldDeletedItems,
	}, nil).Once()
	
	items, err := repo.GetDeletedItemsOlderThan(context.Background(), 30*24*time.Hour)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(items))
	assert.Equal(t, "item1", *items[0]["pk"].S)
	
	client.AssertExpectations(t)
}

func TestSoftDeleteRepository_IsSoftDeleted(t *testing.T) {
	client := &MockDynamoDBClient{}
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(client, "test-table", logger)
	
	tests := []struct {
		name     string
		item     map[string]*dynamodb.AttributeValue
		expected bool
	}{
		{
			name: "active item",
			item: map[string]*dynamodb.AttributeValue{
				"pk":   {S: aws.String("item1")},
				"name": {S: aws.String("Active Item")},
			},
			expected: false,
		},
		{
			name: "soft deleted item",
			item: map[string]*dynamodb.AttributeValue{
				"pk":         {S: aws.String("item1")},
				"name":       {S: aws.String("Deleted Item")},
				"deleted_at": {S: aws.String("2023-10-15T14:30:00Z")},
			},
			expected: true,
		},
		{
			name: "item with empty deleted_at",
			item: map[string]*dynamodb.AttributeValue{
				"pk":         {S: aws.String("item1")},
				"name":       {S: aws.String("Item")},
				"deleted_at": {S: aws.String("")},
			},
			expected: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.isSoftDeleted(tt.item)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSoftDeleteStats_String(t *testing.T) {
	stats := SoftDeleteStats{
		TotalItems:   100,
		ActiveItems:  85,
		DeletedItems: 15,
	}
	
	str := stats.String()
	assert.Contains(t, str, "Total: 100")
	assert.Contains(t, str, "Active: 85")
	assert.Contains(t, str, "Deleted: 15")
}

func TestSoftDeleteStats_GetDeletionPercentage(t *testing.T) {
	tests := []struct {
		name     string
		stats    SoftDeleteStats
		expected float64
	}{
		{
			name: "normal case",
			stats: SoftDeleteStats{
				TotalItems:   100,
				DeletedItems: 15,
			},
			expected: 15.0,
		},
		{
			name: "no items",
			stats: SoftDeleteStats{
				TotalItems:   0,
				DeletedItems: 0,
			},
			expected: 0.0,
		},
		{
			name: "all deleted",
			stats: SoftDeleteStats{
				TotalItems:   50,
				DeletedItems: 50,
			},
			expected: 100.0,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.stats.GetDeletionPercentage()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvenienceFunctions(t *testing.T) {
	model := NewExampleModel("id123", "Test User", "test@example.com")
	
	t.Run("IsItemDeleted", func(t *testing.T) {
		assert.False(t, IsItemDeleted(model))
		
		model.SoftDelete()
		assert.True(t, IsItemDeleted(model))
	})
	
	t.Run("GetItemDeletionInfo", func(t *testing.T) {
		model.Restore()
		
		deletedAt, deletedBy, isDeleted := GetItemDeletionInfo(model)
		assert.Nil(t, deletedAt)
		assert.Empty(t, deletedBy)
		assert.False(t, isDeleted)
		
		model.SoftDeleteBy("admin")
		deletedAt, deletedBy, isDeleted = GetItemDeletionInfo(model)
		assert.NotNil(t, deletedAt)
		assert.Equal(t, "admin", deletedBy)
		assert.True(t, isDeleted)
	})
}

func TestSoftDeleteRepository_FilterSoftDeleted(t *testing.T) {
	client := &MockDynamoDBClient{}
	logger := zaptest.NewLogger(t)
	repo := NewSoftDeleteRepository(client, "test-table", logger)
	
	items := []map[string]*dynamodb.AttributeValue{
		{
			"pk":   {S: aws.String("item1")},
			"name": {S: aws.String("Active Item 1")},
		},
		{
			"pk":         {S: aws.String("item2")},
			"name":       {S: aws.String("Deleted Item")},
			"deleted_at": {S: aws.String("2023-10-15T14:30:00Z")},
		},
		{
			"pk":   {S: aws.String("item3")},
			"name": {S: aws.String("Active Item 2")},
		},
	}
	
	filtered := repo.filterSoftDeleted(items)
	
	assert.Equal(t, 2, len(filtered))
	assert.Equal(t, "item1", *filtered[0]["pk"].S)
	assert.Equal(t, "item3", *filtered[1]["pk"].S)
}

// Test that would verify actual DynamoDB marshaling/unmarshaling
// In practice, this would integrate with your existing DynamoDB marshaling logic
func TestSoftDeleteModel_Integration(t *testing.T) {
	// This test demonstrates how the soft delete model would work
	// in practice with actual data marshaling
	
	model := NewExampleModel("test-id", "Test User", "test@example.com")
	
	// Test the complete lifecycle
	assert.False(t, model.IsDeleted())
	
	// Soft delete
	model.SoftDeleteBy("admin-user")
	assert.True(t, model.IsDeleted())
	assert.Equal(t, "admin-user", model.GetDeletedBy())
	assert.NotNil(t, model.GetDeletedAt())
	
	// Restore
	model.Restore()
	assert.False(t, model.IsDeleted())
	assert.Nil(t, model.GetDeletedAt())
	assert.Empty(t, model.GetDeletedBy())
	
	// The model maintains all its original data
	assert.Equal(t, "test-id", model.ID)
	assert.Equal(t, "Test User", model.Name)
	assert.Equal(t, "test@example.com", model.Email)
}