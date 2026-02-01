package stream

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterRecordsByEntityType(t *testing.T) {
	// Create test records
	records := []events.DynamoDBEventRecord{
		{
			EventID:   "event1",
			EventName: "INSERT",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute("user#123"),
				},
			},
		},
		{
			EventID:   "event2",
			EventName: "MODIFY",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute("status#456"),
				},
			},
		},
		{
			EventID:   "event3",
			EventName: "REMOVE",
			Change: events.DynamoDBStreamRecord{
				OldImage: map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute("user#789"),
				},
			},
		},
	}

	// Filter by user entity type
	filtered := FilterRecordsByEntityType(records, "user")

	// Verify results
	assert.Len(t, filtered, 2)
	assert.Equal(t, "event1", filtered[0].EventID)
	assert.Equal(t, "event3", filtered[1].EventID)
}

func TestFilterRecordsByEventName(t *testing.T) {
	// Create test records
	records := []events.DynamoDBEventRecord{
		{
			EventID:   "event1",
			EventName: "INSERT",
		},
		{
			EventID:   "event2",
			EventName: "MODIFY",
		},
		{
			EventID:   "event3",
			EventName: "REMOVE",
		},
	}

	// Filter by INSERT and MODIFY events
	filtered := FilterRecordsByEventName(records, "INSERT", "MODIFY")

	// Verify results
	assert.Len(t, filtered, 2)
	assert.Equal(t, "event1", filtered[0].EventID)
	assert.Equal(t, "event2", filtered[1].EventID)
}

func TestGetStringAttribute(t *testing.T) {
	// Create test record
	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"Name": events.NewStringAttribute("John Doe"),
			},
		},
	}

	// Get string attribute
	value, err := GetStringAttribute(record, "Name")

	// Verify results
	require.NoError(t, err)
	assert.Equal(t, "John Doe", value)

	// Test missing attribute
	_, err = GetStringAttribute(record, "Missing")
	assert.Error(t, err)
}

func TestGetNumberAttribute(t *testing.T) {
	// Create test record
	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"Age": events.NewNumberAttribute("30"),
			},
		},
	}

	// Get number attribute
	value, err := GetNumberAttribute(record, "Age")

	// Verify results
	require.NoError(t, err)
	assert.Equal(t, "30", value)

	// Test missing attribute
	_, err = GetNumberAttribute(record, "Missing")
	assert.Error(t, err)
}

func TestGetBooleanAttribute(t *testing.T) {
	// Create test record
	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"Active": events.NewBooleanAttribute(true),
			},
		},
	}

	// Get boolean attribute
	value, err := GetBooleanAttribute(record, "Active")

	// Verify results
	require.NoError(t, err)
	assert.Equal(t, true, value)

	// Test missing attribute
	_, err = GetBooleanAttribute(record, "Missing")
	assert.Error(t, err)
}

func TestExtractEntityIDFromPK(t *testing.T) {
	tests := []struct {
		name     string
		pk       string
		expected string
		wantErr  bool
	}{
		{
			name:     "valid PK",
			pk:       "user#123",
			expected: "123",
			wantErr:  false,
		},
		{
			name:     "complex PK",
			pk:       "status#456#draft",
			expected: "456",
			wantErr:  false,
		},
		{
			name:     "invalid PK",
			pk:       "invalid",
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractEntityIDFromPK(tt.pk)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestProcessStreamEvent(t *testing.T) {
	// Create test event
	event := events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{
			{
				EventID:   "event1",
				EventName: "INSERT",
			},
			{
				EventID:   "event2",
				EventName: "MODIFY",
			},
		},
	}

	// Track processed records
	processed := make(map[string]bool)

	// Create processor function
	processor := func(_ context.Context, record events.DynamoDBEventRecord) error {
		processed[record.EventID] = true
		return nil
	}

	// Process the event
	err := ProcessStreamEvent(context.Background(), event, processor)

	// Verify results
	require.NoError(t, err)
	assert.Len(t, processed, 2)
	assert.True(t, processed["event1"])
	assert.True(t, processed["event2"])
}

func TestCreateStreamHandler(t *testing.T) {
	// Create test event
	event := events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{
			{
				EventID:   "event1",
				EventName: "INSERT",
			},
		},
	}

	// Track processed records
	processed := make(map[string]bool)

	// Create processor function
	processor := func(_ context.Context, record events.DynamoDBEventRecord) error {
		processed[record.EventID] = true
		return nil
	}

	// Create handler
	handler := CreateStreamHandler(nil, processor)

	// Process the event
	err := handler(context.Background(), event)

	// Verify results
	require.NoError(t, err)
	assert.Len(t, processed, 1)
	assert.True(t, processed["event1"])
}
