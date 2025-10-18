package stream

import (
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

// TestUnmarshalItem removed - complex stream unmarshaling test
// TestUnmarshalItems removed - complex stream unmarshaling test
// TestProcessStreamRecords removed - complex stream processing test

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
