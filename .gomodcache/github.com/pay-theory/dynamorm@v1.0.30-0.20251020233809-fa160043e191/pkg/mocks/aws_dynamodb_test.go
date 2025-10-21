package mocks_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestMockDynamoDBClientCreateTable tests the CreateTable mock
func TestMockDynamoDBClientCreateTable(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)
	ctx := context.Background()

	// Setup expectations
	input := &dynamodb.CreateTableInput{
		TableName: aws.String("test-table"),
	}
	expectedOutput := mocks.NewMockCreateTableOutput("test-table")

	mockClient.On("CreateTable", ctx, input, mock.Anything).Return(expectedOutput, nil)

	// Execute
	result, err := mockClient.CreateTable(ctx, input)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-table", *result.TableDescription.TableName)
	assert.Equal(t, types.TableStatusCreating, result.TableDescription.TableStatus)

	mockClient.AssertExpectations(t)
}

// TestMockDynamoDBClientCreateTableError tests CreateTable error handling
func TestMockDynamoDBClientCreateTableError(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)
	ctx := context.Background()

	// Setup expectations for error
	input := &dynamodb.CreateTableInput{
		TableName: aws.String("test-table"),
	}
	expectedErr := errors.New("table already exists")

	mockClient.On("CreateTable", ctx, input, mock.Anything).Return(nil, expectedErr)

	// Execute
	result, err := mockClient.CreateTable(ctx, input)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedErr, err)

	mockClient.AssertExpectations(t)
}

// TestMockDynamoDBClientDescribeTable tests the DescribeTable mock
func TestMockDynamoDBClientDescribeTable(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)
	ctx := context.Background()

	// Setup expectations
	input := &dynamodb.DescribeTableInput{
		TableName: aws.String("test-table"),
	}
	expectedOutput := mocks.NewMockDescribeTableOutput("test-table", types.TableStatusActive)

	mockClient.On("DescribeTable", ctx, input, mock.Anything).Return(expectedOutput, nil)

	// Execute
	result, err := mockClient.DescribeTable(ctx, input)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-table", *result.Table.TableName)
	assert.Equal(t, types.TableStatusActive, result.Table.TableStatus)

	mockClient.AssertExpectations(t)
}

// TestMockDynamoDBClientDeleteTable tests the DeleteTable mock
func TestMockDynamoDBClientDeleteTable(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)
	ctx := context.Background()

	// Setup expectations
	input := &dynamodb.DeleteTableInput{
		TableName: aws.String("test-table"),
	}
	expectedOutput := mocks.NewMockDeleteTableOutput("test-table")

	mockClient.On("DeleteTable", ctx, input, mock.Anything).Return(expectedOutput, nil)

	// Execute
	result, err := mockClient.DeleteTable(ctx, input)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-table", *result.TableDescription.TableName)
	assert.Equal(t, types.TableStatusDeleting, result.TableDescription.TableStatus)

	mockClient.AssertExpectations(t)
}

// TestMockDynamoDBClientUpdateTimeToLive tests the UpdateTimeToLive mock
func TestMockDynamoDBClientUpdateTimeToLive(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)
	ctx := context.Background()

	// Setup expectations
	input := &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String("test-table"),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("ttl"),
			Enabled:       aws.Bool(true),
		},
	}
	expectedOutput := mocks.NewMockUpdateTimeToLiveOutput("test-table")

	mockClient.On("UpdateTimeToLive", ctx, input, mock.Anything).Return(expectedOutput, nil)

	// Execute
	result, err := mockClient.UpdateTimeToLive(ctx, input)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "ttl", *result.TimeToLiveSpecification.AttributeName)
	assert.True(t, *result.TimeToLiveSpecification.Enabled)

	mockClient.AssertExpectations(t)
}

// TestMockDynamoDBClientDataOperations tests basic data operations
func TestMockDynamoDBClientDataOperations(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)
	ctx := context.Background()

	// Test GetItem
	getInput := &dynamodb.GetItemInput{
		TableName: aws.String("test-table"),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		},
	}
	getOutput := &dynamodb.GetItemOutput{
		Item: map[string]types.AttributeValue{
			"id":   &types.AttributeValueMemberS{Value: "123"},
			"name": &types.AttributeValueMemberS{Value: "test"},
		},
	}
	mockClient.On("GetItem", ctx, getInput, mock.Anything).Return(getOutput, nil)

	result, err := mockClient.GetItem(ctx, getInput)
	assert.NoError(t, err)
	assert.NotNil(t, result.Item)

	// Test PutItem
	putInput := &dynamodb.PutItemInput{
		TableName: aws.String("test-table"),
		Item: map[string]types.AttributeValue{
			"id":   &types.AttributeValueMemberS{Value: "123"},
			"name": &types.AttributeValueMemberS{Value: "test"},
		},
	}
	putOutput := &dynamodb.PutItemOutput{}
	mockClient.On("PutItem", ctx, putInput, mock.Anything).Return(putOutput, nil)

	_, err = mockClient.PutItem(ctx, putInput)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
}

// TestMockTableExistsWaiter tests the table exists waiter mock
func TestMockTableExistsWaiter(t *testing.T) {
	mockWaiter := new(mocks.MockTableExistsWaiter)
	ctx := context.Background()

	// Setup expectations
	input := &dynamodb.DescribeTableInput{
		TableName: aws.String("test-table"),
	}
	maxWait := 5 * time.Minute

	mockWaiter.On("Wait", ctx, input, maxWait, mock.Anything).Return(nil)

	// Execute
	err := mockWaiter.Wait(ctx, input, maxWait)

	// Assert
	assert.NoError(t, err)
	mockWaiter.AssertExpectations(t)
}

// TestMockTableExistsWaiterError tests waiter error handling
func TestMockTableExistsWaiterError(t *testing.T) {
	mockWaiter := new(mocks.MockTableExistsWaiter)
	ctx := context.Background()

	// Setup expectations for timeout
	input := &dynamodb.DescribeTableInput{
		TableName: aws.String("test-table"),
	}
	maxWait := 5 * time.Minute
	expectedErr := errors.New("waiter timeout")

	mockWaiter.On("Wait", ctx, input, maxWait, mock.Anything).Return(expectedErr)

	// Execute
	err := mockWaiter.Wait(ctx, input, maxWait)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	mockWaiter.AssertExpectations(t)
}

// TestMockTableNotExistsWaiter tests the table not exists waiter mock
func TestMockTableNotExistsWaiter(t *testing.T) {
	mockWaiter := new(mocks.MockTableNotExistsWaiter)
	ctx := context.Background()

	// Setup expectations
	input := &dynamodb.DescribeTableInput{
		TableName: aws.String("test-table"),
	}
	maxWait := 5 * time.Minute

	mockWaiter.On("Wait", ctx, input, maxWait, mock.Anything).Return(nil)

	// Execute
	err := mockWaiter.Wait(ctx, input, maxWait)

	// Assert
	assert.NoError(t, err)
	mockWaiter.AssertExpectations(t)
}

// TestHelperFunctions tests the helper functions for creating mock responses
func TestHelperFunctions(t *testing.T) {
	// Test NewMockCreateTableOutput
	createOutput := mocks.NewMockCreateTableOutput("test-table")
	assert.Equal(t, "test-table", *createOutput.TableDescription.TableName)
	assert.Equal(t, types.TableStatusCreating, createOutput.TableDescription.TableStatus)

	// Test NewMockDescribeTableOutput
	describeOutput := mocks.NewMockDescribeTableOutput("test-table", types.TableStatusActive)
	assert.Equal(t, "test-table", *describeOutput.Table.TableName)
	assert.Equal(t, types.TableStatusActive, describeOutput.Table.TableStatus)

	// Test NewMockDeleteTableOutput
	deleteOutput := mocks.NewMockDeleteTableOutput("test-table")
	assert.Equal(t, "test-table", *deleteOutput.TableDescription.TableName)
	assert.Equal(t, types.TableStatusDeleting, deleteOutput.TableDescription.TableStatus)

	// Test NewMockUpdateTimeToLiveOutput
	ttlOutput := mocks.NewMockUpdateTimeToLiveOutput("test-table")
	assert.Equal(t, "ttl", *ttlOutput.TimeToLiveSpecification.AttributeName)
	assert.True(t, *ttlOutput.TimeToLiveSpecification.Enabled)
}

// TestTypeAliases verifies that type aliases work for AWS mocks
func TestAWSMockTypeAliases(t *testing.T) {
	// These should compile without issues
	var _ *mocks.DynamoDBClient = new(mocks.MockDynamoDBClient)
	var _ *mocks.TableExistsWaiter = new(mocks.MockTableExistsWaiter)
	var _ *mocks.TableNotExistsWaiter = new(mocks.MockTableNotExistsWaiter)
}

// TestIntegrationScenario demonstrates a complete infrastructure testing scenario
func TestIntegrationScenario(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)
	mockWaiter := new(mocks.MockTableExistsWaiter)
	ctx := context.Background()

	tableName := "connections"

	// Scenario: Create table, wait for it to be active, then enable TTL

	// 1. CreateTable
	createInput := &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
	}
	createOutput := mocks.NewMockCreateTableOutput(tableName)
	mockClient.On("CreateTable", ctx, createInput, mock.Anything).Return(createOutput, nil)

	// 2. Wait for table to exist
	describeInput := &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}
	mockWaiter.On("Wait", ctx, describeInput, 5*time.Minute, mock.Anything).Return(nil)

	// 3. Enable TTL
	ttlInput := &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(tableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("ttl"),
			Enabled:       aws.Bool(true),
		},
	}
	ttlOutput := mocks.NewMockUpdateTimeToLiveOutput(tableName)
	mockClient.On("UpdateTimeToLive", ctx, ttlInput, mock.Anything).Return(ttlOutput, nil)

	// Execute the scenario
	_, err := mockClient.CreateTable(ctx, createInput)
	assert.NoError(t, err)

	err = mockWaiter.Wait(ctx, describeInput, 5*time.Minute)
	assert.NoError(t, err)

	_, err = mockClient.UpdateTimeToLive(ctx, ttlInput)
	assert.NoError(t, err)

	// Verify all expectations were met
	mockClient.AssertExpectations(t)
	mockWaiter.AssertExpectations(t)
}
