package main

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

)

// MockDB is a mock implementation of the core.DB interface
type MockDB struct {
	mock.Mock
}

// Model implements core.DB interface
func (m *MockDB) Model(model any) core.Query {
	args := m.Called(model)
	return args.Get(0).(core.Query)
}

// Transaction implements core.DB interface
func (m *MockDB) Transaction(fn func(tx *core.Tx) error) error {
	args := m.Called(fn)
	return args.Error(0)
}

// Migrate implements core.DB interface
func (m *MockDB) Migrate() error {
	args := m.Called()
	return args.Error(0)
}

// AutoMigrate implements core.DB interface
func (m *MockDB) AutoMigrate(models ...any) error {
	args := m.Called(models)
	return args.Error(0)
}

// Close implements core.DB interface
func (m *MockDB) Close() error {
	args := m.Called()
	return args.Error(0)
}

// WithContext implements core.DB interface
func (m *MockDB) WithContext(ctx context.Context) core.DB {
	args := m.Called(ctx)
	return args.Get(0).(core.DB)
}

// MockQuery is a mock implementation of the core.Query interface
type MockQuery struct {
	mock.Mock
}

// Create implements core.Query interface
func (m *MockQuery) Create() error {
	args := m.Called()
	return args.Error(0)
}

// Update implements core.Query interface
func (m *MockQuery) Update() error {
	args := m.Called()
	return args.Error(0)
}

// Delete implements core.Query interface
func (m *MockQuery) Delete() error {
	args := m.Called()
	return args.Error(0)
}

// Find implements core.Query interface
func (m *MockQuery) Find() error {
	args := m.Called()
	return args.Error(0)
}

// First implements core.Query interface
func (m *MockQuery) First() error {
	args := m.Called()
	return args.Error(0)
}

var handler *ActivityHandler

func handleDynamoDBStream(ctx context.Context, event events.DynamoDBEvent) error {
	if handler == nil {
		return nil
	}
	// Process each record
	for _, record := range event.Records {
		if err := handler.processRecord(ctx, record); err != nil {
			return err
		}
	}
	return nil
}


func TestActivityHandler_ProcessRecord(t *testing.T) {
	// Create mock DB
	mockDB := new(MockDB)

	// Create test handler
	handler := NewActivityHandler(mockDB, "test-table")

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
	activity := ActivityRecord{
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
	// Create mock DB
	mockDB := new(MockDB)

	// Create test handler and set global handler
	handler = NewActivityHandler(mockDB, "test-table")

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
