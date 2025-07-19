package main

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/dynamorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/lesser/pkg/common"
	"github.com/lesser/pkg/storage/models"
)

// MockDB is a mock implementation of the dynamorm.LambdaDB interface
type MockDB struct {
	mock.Mock
}

func (m *MockDB) WithLambdaTimeoutBuffer(milliseconds int) *dynamorm.LambdaDB {
	args := m.Called(milliseconds)
	return args.Get(0).(*dynamorm.LambdaDB)
}

func TestActivityHandler_ProcessRecord(t *testing.T) {
	// Initialize logger for tests
	logger, _ := zap.NewDevelopment()
	common.SetLogger(logger)

	// Create mock DB
	mockDB := new(MockDB)
	mockDB.On("WithLambdaTimeoutBuffer", mock.Anything).Return(&dynamorm.LambdaDB{})

	// Create test handler
	handler := NewActivityHandler(&dynamorm.LambdaDB{}, "test-table")

	// Create test activity record
	activityJSON := `{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id": "https://example.org/activity/123",
		"type": "Create",
		"actor": "https://example.org/users/alice",
		"object": {
			"id": "https://example.org/notes/123",
			"type": "Note",
			"content": "Hello, world!"
		}
	}`

	now := time.Now()
	activity := models.Activity{
		PK:         "activity#123",
		SK:         "inbox#alice#" + now.Format(time.RFC3339),
		Username:   "alice",
		Timestamp:  now.Format(time.RFC3339),
		ActivityID: "123",
		Activity:   activityJSON,
		Direction:  "inbox",
		CreatedAt:  now,
	}

	// Create DynamoDB stream record
	record := events.DynamoDBEventRecord{
		EventID:   "event1",
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":         events.NewStringAttribute(activity.PK),
				"SK":         events.NewStringAttribute(activity.SK),
				"Username":   events.NewStringAttribute(activity.Username),
				"Timestamp":  events.NewStringAttribute(activity.Timestamp),
				"ActivityID": events.NewStringAttribute(activity.ActivityID),
				"Activity":   events.NewStringAttribute(activity.Activity),
				"Direction":  events.NewStringAttribute(activity.Direction),
				"CreatedAt":  events.NewStringAttribute(activity.CreatedAt.Format(time.RFC3339)),
			},
		},
	}

	// Test processing the record
	err := handler.processRecord(context.Background(), record)

	// Since we're not implementing the full activity processing logic,
	// we just check that the record was processed without errors
	require.NoError(t, err)
}

func TestHandleDynamoDBStream(t *testing.T) {
	// Initialize logger for tests
	logger, _ := zap.NewDevelopment()
	common.SetLogger(logger)

	// Create mock DB
	mockDB := new(MockDB)
	mockDB.On("WithLambdaTimeoutBuffer", mock.Anything).Return(&dynamorm.LambdaDB{})

	// Create test handler and set global handler
	handler = NewActivityHandler(&dynamorm.LambdaDB{}, "test-table")

	// Create test event
	event := events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{
			{
				EventID:   "event1",
				EventName: "INSERT",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{
						"PK": events.NewStringAttribute("activity#123"),
						"SK": events.NewStringAttribute("inbox#alice#2023-01-01T12:00:00Z"),
					},
				},
			},
			{
				EventID:   "event2",
				EventName: "INSERT",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{
						"PK": events.NewStringAttribute("user#456"),
						"SK": events.NewStringAttribute("user#456"),
					},
				},
			},
		},
	}

	// Test handling the event
	err := handleDynamoDBStream(context.Background(), event)

	// Since we're not implementing the full activity processing logic,
	// we just check that the event was handled without errors
	assert.NoError(t, err)
}
