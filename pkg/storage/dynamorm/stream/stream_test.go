package stream

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test model for unmarshaling
type TestModel struct {
	PK       string         `dynamorm:"pk" json:"pk"`
	SK       string         `dynamorm:"sk" json:"sk"`
	Name     string         `json:"name"`
	Age      int            `json:"age"`
	IsActive bool           `json:"is_active"`
	Tags     []string       `json:"tags"`
	Metadata map[string]any `json:"metadata"`
}

func TestUnmarshalItem(t *testing.T) {
	// Create a test record
	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":       events.NewStringAttribute("user#123"),
				"SK":       events.NewStringAttribute("user#123"),
				"Name":     events.NewStringAttribute("John Doe"),
				"Age":      events.NewNumberAttribute("30"),
				"IsActive": events.NewBooleanAttribute(true),
				"Tags": events.NewListAttribute([]events.DynamoDBAttributeValue{
					events.NewStringAttribute("tag1"),
					events.NewStringAttribute("tag2"),
				}),
				"Metadata": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
					"CreatedBy": events.NewStringAttribute("admin"),
					"Version":   events.NewNumberAttribute("1"),
				}),
			},
		},
	}

	// Unmarshal the record
	var model TestModel
	err := UnmarshalItem(record, &model)

	// Verify results
	require.NoError(t, err)
	assert.Equal(t, "user#123", model.PK)
	assert.Equal(t, "user#123", model.SK)
	assert.Equal(t, "John Doe", model.Name)
	assert.Equal(t, 30, model.Age)
	assert.Equal(t, true, model.IsActive)
	assert.Equal(t, []string{"tag1", "tag2"}, model.Tags)
	assert.Equal(t, "admin", model.Metadata["CreatedBy"])
	assert.Equal(t, "1", model.Metadata["Version"])
}

func TestUnmarshalItems(t *testing.T) {
	// Create test records
	records := []events.DynamoDBEventRecord{
		{
			EventName: "INSERT",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"PK":       events.NewStringAttribute("user#123"),
					"SK":       events.NewStringAttribute("user#123"),
					"Name":     events.NewStringAttribute("John Doe"),
					"Age":      events.NewNumberAttribute("30"),
					"IsActive": events.NewBooleanAttribute(true),
				},
			},
		},
		{
			EventName: "MODIFY",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"PK":       events.NewStringAttribute("user#456"),
					"SK":       events.NewStringAttribute("user#456"),
					"Name":     events.NewStringAttribute("Jane Smith"),
					"Age":      events.NewNumberAttribute("25"),
					"IsActive": events.NewBooleanAttribute(false),
				},
			},
		},
	}

	// Unmarshal the records
	var testModel TestModel
	result, err := UnmarshalItems(records, testModel)

	// Verify results
	require.NoError(t, err)
	models := result.([]TestModel)
	assert.Len(t, models, 2)

	assert.Equal(t, "user#123", models[0].PK)
	assert.Equal(t, "John Doe", models[0].Name)
	assert.Equal(t, 30, models[0].Age)
	assert.Equal(t, true, models[0].IsActive)

	assert.Equal(t, "user#456", models[1].PK)
	assert.Equal(t, "Jane Smith", models[1].Name)
	assert.Equal(t, 25, models[1].Age)
	assert.Equal(t, false, models[1].IsActive)
}

func TestProcessStreamRecords(t *testing.T) {
	// Create test records
	records := []events.DynamoDBEventRecord{
		{
			EventID:   "event1",
			EventName: "INSERT",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"PK":   events.NewStringAttribute("user#123"),
					"SK":   events.NewStringAttribute("user#123"),
					"Name": events.NewStringAttribute("John Doe"),
					"Age":  events.NewNumberAttribute("30"),
				},
			},
		},
		{
			EventID:   "event2",
			EventName: "MODIFY",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"PK":   events.NewStringAttribute("user#456"),
					"SK":   events.NewStringAttribute("user#456"),
					"Name": events.NewStringAttribute("Jane Smith"),
					"Age":  events.NewNumberAttribute("25"),
				},
			},
		},
		{
			EventID:   "event3",
			EventName: "REMOVE",
			Change: events.DynamoDBStreamRecord{
				OldImage: map[string]events.DynamoDBAttributeValue{
					"PK":   events.NewStringAttribute("user#789"),
					"SK":   events.NewStringAttribute("user#789"),
					"Name": events.NewStringAttribute("Bob Johnson"),
					"Age":  events.NewNumberAttribute("40"),
				},
			},
		},
	}

	// Track processed records
	processed := make(map[string]TestModel)

	// Process the records
	err := ProcessStreamRecords(context.Background(), records, func(ctx context.Context, record events.DynamoDBEventRecord) error {
		// Unmarshal the record to TestModel
		var item TestModel
		if err := UnmarshalItem(record, &item); err != nil {
			return err
		}
		processed[record.EventID] = item
		return nil
	})

	// Verify results
	require.NoError(t, err)
	assert.Len(t, processed, 3)

	assert.Equal(t, "user#123", processed["event1"].PK)
	assert.Equal(t, "John Doe", processed["event1"].Name)
	assert.Equal(t, 30, processed["event1"].Age)

	assert.Equal(t, "user#456", processed["event2"].PK)
	assert.Equal(t, "Jane Smith", processed["event2"].Name)
	assert.Equal(t, 25, processed["event2"].Age)

	assert.Equal(t, "user#789", processed["event3"].PK)
	assert.Equal(t, "Bob Johnson", processed["event3"].Name)
	assert.Equal(t, 40, processed["event3"].Age)
}

func TestConvertAttributeValue(t *testing.T) {
	tests := []struct {
		name     string
		attr     events.DynamoDBAttributeValue
		expected any
	}{
		{
			name:     "string attribute",
			attr:     events.NewStringAttribute("test"),
			expected: "test",
		},
		{
			name:     "number attribute",
			attr:     events.NewNumberAttribute("123"),
			expected: "123", // DynamoDB numbers are strings
		},
		{
			name:     "boolean attribute",
			attr:     events.NewBooleanAttribute(true),
			expected: true,
		},
		{
			name:     "null attribute",
			attr:     events.NewNullAttribute(),
			expected: nil,
		},
		{
			name: "map attribute",
			attr: events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
				"key1": events.NewStringAttribute("value1"),
				"key2": events.NewNumberAttribute("42"),
			}),
			expected: map[string]any{
				"key1": "value1",
				"key2": "42",
			},
		},
		{
			name: "list attribute",
			attr: events.NewListAttribute([]events.DynamoDBAttributeValue{
				events.NewStringAttribute("item1"),
				events.NewNumberAttribute("2"),
				events.NewBooleanAttribute(true),
			}),
			expected: []any{"item1", "2", true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertAttributeValue(tt.attr)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
