package stream

import (
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEventType(t *testing.T) {
	tests := []struct {
		name     string
		record   events.DynamoDBEventRecord
		expected string
		wantErr  bool
	}{
		{
			name: "INSERT event with valid PK",
			record: events.DynamoDBEventRecord{
				EventName: "INSERT",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{
						"PK": events.NewStringAttribute("user#123"),
					},
				},
			},
			expected: "user",
			wantErr:  false,
		},
		{
			name: "MODIFY event with valid PK",
			record: events.DynamoDBEventRecord{
				EventName: "MODIFY",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{
						"PK": events.NewStringAttribute("status#456"),
					},
				},
			},
			expected: "status",
			wantErr:  false,
		},
		{
			name: "REMOVE event with valid PK",
			record: events.DynamoDBEventRecord{
				EventName: "REMOVE",
				Change: events.DynamoDBStreamRecord{
					OldImage: map[string]events.DynamoDBAttributeValue{
						"PK": events.NewStringAttribute("actor#789"),
					},
				},
			},
			expected: "actor",
			wantErr:  false,
		},
		{
			name: "Invalid PK format",
			record: events.DynamoDBEventRecord{
				EventName: "INSERT",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{
						"PK": events.NewStringAttribute("invalid"),
					},
				},
			},
			expected: "",
			wantErr:  true,
		},
		{
			name: "Missing PK",
			record: events.DynamoDBEventRecord{
				EventName: "INSERT",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{},
				},
			},
			expected: "",
			wantErr:  true,
		},
		{
			name: "Unknown event type",
			record: events.DynamoDBEventRecord{
				EventName: "UNKNOWN",
			},
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetEventType(tt.record)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
