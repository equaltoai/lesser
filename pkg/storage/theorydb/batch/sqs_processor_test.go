package batch

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestNewSQSBatchProcessor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	tracker := &MockCostTracker{}

	// Test with default config
	processor := NewSQSBatchProcessor(mockDB, SQSBatchProcessorConfig{
		Logger:  logger,
		Tracker: tracker,
	})

	assert.NotNil(t, processor)
	assert.Equal(t, MaxBatchWriteSize, processor.maxBatchSize)
	assert.Equal(t, logger, processor.logger)
	assert.Equal(t, tracker, processor.tracker)

	// Test with custom batch size
	processor = NewSQSBatchProcessor(mockDB, SQSBatchProcessorConfig{
		Logger:       logger,
		Tracker:      tracker,
		MaxBatchSize: 10,
	})

	assert.Equal(t, 10, processor.maxBatchSize)

	// Test with oversized batch size (should clamp to max)
	processor = NewSQSBatchProcessor(mockDB, SQSBatchProcessorConfig{
		Logger:       logger,
		Tracker:      tracker,
		MaxBatchSize: 50, // Over the DynamoDB limit
	})

	assert.Equal(t, MaxBatchWriteSize, processor.maxBatchSize)
}

func TestSQSBatchProcessor_ProcessBatch_EmptyEvent(t *testing.T) {
	mockDB := new(mocks.MockDB)
	processor := NewSQSBatchProcessor(mockDB, SQSBatchProcessorConfig{
		Logger: zap.NewNop(),
	})

	event := events.SQSEvent{
		Records: []events.SQSMessage{},
	}

	response, err := processor.ProcessBatch(context.Background(), event)

	assert.NoError(t, err)
	assert.Empty(t, response.BatchItemFailures)
}

func TestSQSBatchProcessor_ProcessBatch_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Set up mock expectations
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.Anything).Return(nil)

	processor := NewSQSBatchProcessor(mockDB, SQSBatchProcessorConfig{
		Logger: zap.NewNop(),
	})

	// Create a valid batch message
	batchMessage := BatchMessage{
		Operation: "create",
		Items:     []any{"item1", "item2", "item3"},
		TableName: "test-table",
	}

	messageBody, _ := json.Marshal(batchMessage)

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId: "msg-1",
				Body:      string(messageBody),
			},
		},
	}

	response, err := processor.ProcessBatch(context.Background(), event)

	assert.NoError(t, err)
	assert.Empty(t, response.BatchItemFailures)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSQSBatchProcessor_ProcessBatch_BatchFailure(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Set up mock to return an error
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.Anything).Return(assert.AnError)

	processor := NewSQSBatchProcessor(mockDB, SQSBatchProcessorConfig{
		Logger: zap.NewNop(),
	})

	// Create a batch message that will fail
	batchMessage := BatchMessage{
		Operation: "create",
		Items:     []any{"item1", "item2"},
		TableName: "test-table",
	}

	messageBody, _ := json.Marshal(batchMessage)

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId: "msg-1",
				Body:      string(messageBody),
			},
		},
	}

	response, err := processor.ProcessBatch(context.Background(), event)

	assert.NoError(t, err) // Processor should not return error, just mark items as failed
	assert.Len(t, response.BatchItemFailures, 1)
	assert.Equal(t, "msg-1", response.BatchItemFailures[0].ItemIdentifier)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSQSBatchProcessor_ProcessBatch_InvalidJSON(t *testing.T) {
	mockDB := new(mocks.MockDB)
	processor := NewSQSBatchProcessor(mockDB, SQSBatchProcessorConfig{
		Logger: zap.NewNop(),
	})

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId: "msg-1",
				Body:      "invalid json",
			},
		},
	}

	response, err := processor.ProcessBatch(context.Background(), event)

	assert.NoError(t, err)
	assert.Len(t, response.BatchItemFailures, 1)
	assert.Equal(t, "msg-1", response.BatchItemFailures[0].ItemIdentifier)
}

func TestSQSBatchProcessor_ProcessBatch_UnsupportedOperation(t *testing.T) {
	mockDB := new(mocks.MockDB)
	processor := NewSQSBatchProcessor(mockDB, SQSBatchProcessorConfig{
		Logger: zap.NewNop(),
	})

	batchMessage := BatchMessage{
		Operation: "unsupported",
		Items:     []any{"item1"},
		TableName: "test-table",
	}

	messageBody, _ := json.Marshal(batchMessage)

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId: "msg-1",
				Body:      string(messageBody),
			},
		},
	}

	response, err := processor.ProcessBatch(context.Background(), event)

	assert.NoError(t, err)
	assert.Len(t, response.BatchItemFailures, 1)
	assert.Equal(t, "msg-1", response.BatchItemFailures[0].ItemIdentifier)
}

func TestSQSBatchProcessor_ProcessTimelineEntries_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Set up mock expectations
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.Anything).Return(nil)

	processor := NewSQSBatchProcessor(mockDB, SQSBatchProcessorConfig{
		Logger: zap.NewNop(),
	})

	timelineMessage := struct {
		FollowerIDs []string  `json:"follower_ids"`
		StatusID    string    `json:"status_id"`
		AuthorID    string    `json:"author_id"`
		CreatedAt   time.Time `json:"created_at"`
	}{
		FollowerIDs: []string{"user1", "user2"},
		StatusID:    "status123",
		AuthorID:    "author456",
		CreatedAt:   time.Now(),
	}

	messageBody, _ := json.Marshal(timelineMessage)

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId: "msg-1",
				Body:      string(messageBody),
			},
		},
	}

	response, err := processor.ProcessTimelineEntries(context.Background(), event)

	assert.NoError(t, err)
	assert.Empty(t, response.BatchItemFailures)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSQSBatchProcessor_ProcessNotifications_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Set up mock expectations
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.Anything).Return(nil)

	processor := NewSQSBatchProcessor(mockDB, SQSBatchProcessorConfig{
		Logger: zap.NewNop(),
	})

	notifMessage := struct {
		UserIDs    []string `json:"user_ids"`
		StatusID   string   `json:"status_id"`
		AuthorID   string   `json:"author_id"`
		Type       string   `json:"type"`
		TargetType string   `json:"target_type"`
	}{
		UserIDs:    []string{"user1", "user2", "user3"},
		StatusID:   "status123",
		AuthorID:   "author456",
		Type:       "mention",
		TargetType: "status",
	}

	messageBody, _ := json.Marshal(notifMessage)

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId: "msg-1",
				Body:      string(messageBody),
			},
		},
	}

	response, err := processor.ProcessNotifications(context.Background(), event)

	assert.NoError(t, err)
	assert.Empty(t, response.BatchItemFailures)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSQSBatchProcessor_ProcessBatch_DeleteOperation(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Set up mock expectations for delete
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("BatchDelete", mock.Anything).Return(nil)

	processor := NewSQSBatchProcessor(mockDB, SQSBatchProcessorConfig{
		Logger: zap.NewNop(),
	})

	// Create a delete batch message with key objects
	deleteKeys := []any{
		map[string]any{"PK": "USER#user1", "SK": "PROFILE"},
		map[string]any{"PK": "USER#user2", "SK": "PROFILE"},
	}

	batchMessage := BatchMessage{
		Operation: "delete",
		Items:     deleteKeys,
		TableName: "test-table",
	}

	messageBody, _ := json.Marshal(batchMessage)

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId: "msg-1",
				Body:      string(messageBody),
			},
		},
	}

	response, err := processor.ProcessBatch(context.Background(), event)

	assert.NoError(t, err)
	assert.Empty(t, response.BatchItemFailures)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestCreateTimelineMessage(t *testing.T) {
	followerIDs := []string{"user1", "user2", "user3"}
	statusID := "status123"
	authorID := "author456"
	createdAt := time.Now()

	message := CreateTimelineMessage(followerIDs, statusID, authorID, createdAt)

	assert.NotNil(t, message)
	assert.Equal(t, "create", message.Operation)
	assert.Equal(t, "timeline", message.TableName)
	assert.Len(t, message.Items, 3)
	assert.Equal(t, statusID, message.Metadata["status_id"])
	assert.Equal(t, authorID, message.Metadata["author_id"])

	// Check that the items are correctly formatted
	for i, item := range message.Items {
		itemMap, ok := item.(map[string]any)
		assert.True(t, ok)
		assert.Equal(t, fmt.Sprintf("USER#%s", followerIDs[i]), itemMap["PK"])
		assert.Contains(t, itemMap["SK"].(string), "TIMELINE#")
		assert.Equal(t, statusID, itemMap["StatusID"])
		assert.Equal(t, authorID, itemMap["AuthorID"])
		assert.Equal(t, "home", itemMap["Type"])
	}
}

func TestCreateNotificationMessage(t *testing.T) {
	userIDs := []string{"user1", "user2"}
	statusID := "status123"
	authorID := "author456"
	notifType := "mention"
	targetType := "status"

	message := CreateNotificationMessage(userIDs, statusID, authorID, notifType, targetType)

	assert.NotNil(t, message)
	assert.Equal(t, "create", message.Operation)
	assert.Equal(t, "notifications", message.TableName)
	assert.Len(t, message.Items, 2)
	assert.Equal(t, statusID, message.Metadata["status_id"])
	assert.Equal(t, authorID, message.Metadata["author_id"])
	assert.Equal(t, notifType, message.Metadata["type"])

	// Check that the items are correctly formatted
	for i, item := range message.Items {
		itemMap, ok := item.(map[string]any)
		assert.True(t, ok)
		assert.Equal(t, fmt.Sprintf("USER#%s", userIDs[i]), itemMap["PK"])
		assert.Contains(t, itemMap["SK"].(string), "NOTIF#")
		assert.Equal(t, notifType, itemMap["Type"])
		assert.Equal(t, authorID, itemMap["ActorID"])
		assert.Equal(t, statusID, itemMap["TargetID"])
		assert.Equal(t, targetType, itemMap["TargetType"])
		assert.Equal(t, false, itemMap["IsRead"])
	}
}

func TestCreateBatchDeleteMessage(t *testing.T) {
	keys := []any{
		map[string]any{"PK": "USER#user1", "SK": "PROFILE"},
		map[string]any{"PK": "USER#user2", "SK": "PROFILE"},
	}
	tableName := "users"

	message := CreateBatchDeleteMessage(keys, tableName)

	assert.NotNil(t, message)
	assert.Equal(t, "delete", message.Operation)
	assert.Equal(t, tableName, message.TableName)
	assert.Equal(t, keys, message.Items)
}

func TestOptimalBatchSize(t *testing.T) {
	// Test empty items
	assert.Equal(t, 0, OptimalBatchSize([]any{}))

	// Test small items
	smallItems := []any{"small", "item", "test"}
	assert.Equal(t, 25, OptimalBatchSize(smallItems))

	// Test medium items (simulated with a struct)
	mediumItem := struct {
		Data string `json:"data"`
	}{
		Data: string(make([]byte, 2000)), // 2KB item
	}
	mediumItems := []any{mediumItem}
	assert.Equal(t, 10, OptimalBatchSize(mediumItems))

	// Test large items
	largeItem := struct {
		Data string `json:"data"`
	}{
		Data: string(make([]byte, 15000)), // 15KB item
	}
	largeItems := []any{largeItem}
	assert.Equal(t, 10, OptimalBatchSize(largeItems))
}

// Benchmark tests

func BenchmarkSQSBatchProcessor_ProcessBatch(b *testing.B) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.Anything).Return(nil)

	processor := NewSQSBatchProcessor(mockDB, SQSBatchProcessorConfig{
		Logger: zap.NewNop(),
	})

	batchMessage := BatchMessage{
		Operation: "create",
		Items:     make([]any, 25), // Full batch
		TableName: "test-table",
	}

	// Fill with test items
	for i := 0; i < 25; i++ {
		batchMessage.Items[i] = map[string]any{
			"PK":   fmt.Sprintf("ITEM#%d", i),
			"SK":   "METADATA",
			"Data": fmt.Sprintf("data-%d", i),
		}
	}

	messageBody, _ := json.Marshal(batchMessage)

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId: "msg-1",
				Body:      string(messageBody),
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = processor.ProcessBatch(context.Background(), event)
	}
}

func BenchmarkCreateTimelineMessage(b *testing.B) {
	followerIDs := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		followerIDs[i] = fmt.Sprintf("user%d", i)
	}

	statusID := "status123"
	authorID := "author456"
	createdAt := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CreateTimelineMessage(followerIDs, statusID, authorID, createdAt)
	}
}
