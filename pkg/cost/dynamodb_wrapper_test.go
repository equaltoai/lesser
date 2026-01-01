package cost

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockDynamoDBAPI is a mock implementation of DynamoDBAPI
type MockDynamoDBAPI struct {
	mock.Mock
}

func (m *MockDynamoDBAPI) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.PutItemOutput), args.Error(1)
}

func (m *MockDynamoDBAPI) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.GetItemOutput), args.Error(1)
}

func (m *MockDynamoDBAPI) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.UpdateItemOutput), args.Error(1)
}

func (m *MockDynamoDBAPI) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.DeleteItemOutput), args.Error(1)
}

func (m *MockDynamoDBAPI) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.QueryOutput), args.Error(1)
}

func (m *MockDynamoDBAPI) Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.ScanOutput), args.Error(1)
}

func (m *MockDynamoDBAPI) BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.BatchWriteItemOutput), args.Error(1)
}

func (m *MockDynamoDBAPI) BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.BatchGetItemOutput), args.Error(1)
}

func TestNewDynamoDBWrapper(t *testing.T) {
	mockClient := &MockDynamoDBAPI{}
	wrapper := NewDynamoDBWrapper(mockClient)
	require.NotNil(t, wrapper)
	assert.Equal(t, mockClient, wrapper.client)
}

func TestDynamoDBWrapper_PutItem(t *testing.T) {
	mockClient := &MockDynamoDBAPI{}
	wrapper := NewDynamoDBWrapper(mockClient)

	tracker := New()
	tracker.circuitBreaker = nil
	ctx := WithTracker(context.Background(), tracker)

	input := &dynamodb.PutItemInput{
		TableName: aws.String("test-table"),
		Item: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		},
	}

	output := &dynamodb.PutItemOutput{
		ConsumedCapacity: &types.ConsumedCapacity{
			WriteCapacityUnits: aws.Float64(2.0),
		},
	}

	mockClient.On("PutItem", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.PutItem(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Should track writes
	assert.True(t, tracker.dynamoWrites.Load() > 0)
}

func TestDynamoDBWrapper_GetItem(t *testing.T) {
	mockClient := &MockDynamoDBAPI{}
	wrapper := NewDynamoDBWrapper(mockClient)

	tracker := New()
	tracker.circuitBreaker = nil
	ctx := WithTracker(context.Background(), tracker)

	input := &dynamodb.GetItemInput{
		TableName: aws.String("test-table"),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		},
	}

	output := &dynamodb.GetItemOutput{
		Item: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		},
		ConsumedCapacity: &types.ConsumedCapacity{
			ReadCapacityUnits: aws.Float64(1.0),
		},
	}

	mockClient.On("GetItem", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.GetItem(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Should track reads
	assert.True(t, tracker.dynamoReads.Load() > 0)
}

func TestDynamoDBWrapper_GetItem_EventualConsistency(t *testing.T) {
	mockClient := &MockDynamoDBAPI{}
	wrapper := NewDynamoDBWrapper(mockClient)

	tracker := New()
	tracker.circuitBreaker = nil
	ctx := WithTracker(context.Background(), tracker)

	input := &dynamodb.GetItemInput{
		TableName:      aws.String("test-table"),
		ConsistentRead: aws.Bool(false),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		},
	}

	output := &dynamodb.GetItemOutput{}
	mockClient.On("GetItem", ctx, input, mock.Anything).Return(output, nil)

	_, err := wrapper.GetItem(ctx, input)
	assert.NoError(t, err)
}

func TestDynamoDBWrapper_UpdateItem(t *testing.T) {
	mockClient := &MockDynamoDBAPI{}
	wrapper := NewDynamoDBWrapper(mockClient)

	tracker := New()
	tracker.circuitBreaker = nil
	ctx := WithTracker(context.Background(), tracker)

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String("test-table"),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		},
	}

	output := &dynamodb.UpdateItemOutput{
		ConsumedCapacity: &types.ConsumedCapacity{
			WriteCapacityUnits: aws.Float64(1.0),
			ReadCapacityUnits:  aws.Float64(1.0), // Use 1.0 since int(0.5) truncates to 0
		},
	}

	mockClient.On("UpdateItem", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.UpdateItem(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Should track both reads and writes
	assert.True(t, tracker.dynamoWrites.Load() > 0)
	assert.True(t, tracker.dynamoReads.Load() > 0)
}

func TestDynamoDBWrapper_DeleteItem(t *testing.T) {
	mockClient := &MockDynamoDBAPI{}
	wrapper := NewDynamoDBWrapper(mockClient)

	tracker := New()
	tracker.circuitBreaker = nil
	ctx := WithTracker(context.Background(), tracker)

	input := &dynamodb.DeleteItemInput{
		TableName: aws.String("test-table"),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		},
	}

	output := &dynamodb.DeleteItemOutput{
		ConsumedCapacity: &types.ConsumedCapacity{
			WriteCapacityUnits: aws.Float64(1.0),
		},
	}

	mockClient.On("DeleteItem", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.DeleteItem(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	assert.True(t, tracker.dynamoWrites.Load() > 0)
}

func TestDynamoDBWrapper_Query(t *testing.T) {
	mockClient := &MockDynamoDBAPI{}
	wrapper := NewDynamoDBWrapper(mockClient)

	tracker := New()
	tracker.circuitBreaker = nil
	ctx := WithTracker(context.Background(), tracker)

	input := &dynamodb.QueryInput{
		TableName: aws.String("test-table"),
	}

	output := &dynamodb.QueryOutput{
		Count: 5,
		ConsumedCapacity: &types.ConsumedCapacity{
			ReadCapacityUnits: aws.Float64(5.0),
		},
	}

	mockClient.On("Query", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.Query(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	assert.True(t, tracker.dynamoReads.Load() > 0)
}

func TestDynamoDBWrapper_Query_NoConsumedCapacity(t *testing.T) {
	mockClient := &MockDynamoDBAPI{}
	wrapper := NewDynamoDBWrapper(mockClient)

	tracker := New()
	tracker.circuitBreaker = nil
	ctx := WithTracker(context.Background(), tracker)

	input := &dynamodb.QueryInput{
		TableName: aws.String("test-table"),
	}

	output := &dynamodb.QueryOutput{
		Count: 5,
	}

	mockClient.On("Query", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.Query(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Should estimate based on count
	assert.Equal(t, int64(5), tracker.dynamoReads.Load())
}

func TestDynamoDBWrapper_Scan(t *testing.T) {
	mockClient := &MockDynamoDBAPI{}
	wrapper := NewDynamoDBWrapper(mockClient)

	tracker := New()
	tracker.circuitBreaker = nil
	ctx := WithTracker(context.Background(), tracker)

	input := &dynamodb.ScanInput{
		TableName: aws.String("test-table"),
	}

	output := &dynamodb.ScanOutput{
		ScannedCount: 100,
		ConsumedCapacity: &types.ConsumedCapacity{
			ReadCapacityUnits: aws.Float64(50.0),
		},
	}

	mockClient.On("Scan", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.Scan(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	assert.True(t, tracker.dynamoReads.Load() > 0)
}

func TestDynamoDBWrapper_Scan_NoConsumedCapacity(t *testing.T) {
	mockClient := &MockDynamoDBAPI{}
	wrapper := NewDynamoDBWrapper(mockClient)

	tracker := New()
	tracker.circuitBreaker = nil
	ctx := WithTracker(context.Background(), tracker)

	input := &dynamodb.ScanInput{
		TableName: aws.String("test-table"),
	}

	output := &dynamodb.ScanOutput{
		ScannedCount: 100,
	}

	mockClient.On("Scan", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.Scan(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Should estimate based on scanned count
	assert.Equal(t, int64(100), tracker.dynamoReads.Load())
}

func TestDynamoDBWrapper_BatchWriteItem(t *testing.T) {
	mockClient := &MockDynamoDBAPI{}
	wrapper := NewDynamoDBWrapper(mockClient)

	tracker := New()
	tracker.circuitBreaker = nil
	ctx := WithTracker(context.Background(), tracker)

	input := &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			"test-table": {
				{PutRequest: &types.PutRequest{Item: map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "1"}}}},
				{PutRequest: &types.PutRequest{Item: map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "2"}}}},
			},
		},
	}

	output := &dynamodb.BatchWriteItemOutput{
		ConsumedCapacity: []types.ConsumedCapacity{
			{WriteCapacityUnits: aws.Float64(2.0)},
		},
	}

	mockClient.On("BatchWriteItem", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.BatchWriteItem(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	assert.True(t, tracker.dynamoWrites.Load() > 0)
}

func TestDynamoDBWrapper_BatchGetItem(t *testing.T) {
	mockClient := &MockDynamoDBAPI{}
	wrapper := NewDynamoDBWrapper(mockClient)

	tracker := New()
	tracker.circuitBreaker = nil
	ctx := WithTracker(context.Background(), tracker)

	input := &dynamodb.BatchGetItemInput{
		RequestItems: map[string]types.KeysAndAttributes{
			"test-table": {
				Keys: []map[string]types.AttributeValue{
					{"id": &types.AttributeValueMemberS{Value: "1"}},
					{"id": &types.AttributeValueMemberS{Value: "2"}},
				},
			},
		},
	}

	output := &dynamodb.BatchGetItemOutput{
		ConsumedCapacity: []types.ConsumedCapacity{
			{ReadCapacityUnits: aws.Float64(2.0)},
		},
	}

	mockClient.On("BatchGetItem", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.BatchGetItem(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	assert.True(t, tracker.dynamoReads.Load() > 0)
}
