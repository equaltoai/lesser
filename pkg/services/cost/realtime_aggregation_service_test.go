package cost

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRealtimeAggregationService_determineRecordType(t *testing.T) {
	svc := &RealtimeAggregationService{}

	tests := []struct {
		name     string
		pk       string
		expected string
	}{
		{name: "ai_cost", pk: "AI_COST#2025-01-01#op", expected: "ai_cost"},
		{name: "websocket_cost", pk: "WS_COST#2025-01-01#op", expected: "websocket_cost"},
		{name: "federation_cost", pk: "FED_COST#2025-01-01#op", expected: "federation_cost"},
		{name: "unknown_prefix", pk: "OTHER#x", expected: "unknown"},
		{name: "empty_pk", pk: "", expected: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := events.DynamoDBEventRecord{
				Change: events.DynamoDBStreamRecord{
					Keys: map[string]events.DynamoDBAttributeValue{
						"PK": events.NewStringAttribute(tt.pk),
					},
				},
			}
			if tt.pk == "" {
				record.Change.Keys = map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute(""),
				}
			}
			assert.Equal(t, tt.expected, svc.determineRecordType(record))
		})
	}

	t.Run("missing_keys", func(t *testing.T) {
		record := events.DynamoDBEventRecord{}
		assert.Equal(t, "unknown", svc.determineRecordType(record))
	})
}

func TestRealtimeAggregationService_groupRecordsByType(t *testing.T) {
	svc := &RealtimeAggregationService{}

	records := []events.DynamoDBEventRecord{
		{EventName: EventInsert, Change: events.DynamoDBStreamRecord{Keys: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("AI_COST#1")}}},
		{EventName: EventInsert, Change: events.DynamoDBStreamRecord{Keys: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("WS_COST#1")}}},
		{EventName: EventInsert, Change: events.DynamoDBStreamRecord{Keys: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("FED_COST#1")}}},
		{EventName: EventInsert, Change: events.DynamoDBStreamRecord{Keys: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("OTHER#1")}}},
	}

	groups := svc.groupRecordsByType(records)

	require.Len(t, groups["ai_cost"], 1)
	require.Len(t, groups["websocket_cost"], 1)
	require.Len(t, groups["federation_cost"], 1)
	require.Len(t, groups["unknown"], 1)
}

func TestStreamProcessor_ProcessRecords(t *testing.T) {
	t.Run("success_updates_metrics", func(t *testing.T) {
		processor := NewStreamProcessor("test", func(context.Context, []events.DynamoDBEventRecord) error { return nil }, zap.NewNop())

		records := []events.DynamoDBEventRecord{
			{EventName: EventInsert},
			{EventName: EventModify},
		}

		err := processor.ProcessRecords(context.Background(), records)
		require.NoError(t, err)

		assert.Equal(t, int64(2), processor.metrics.TotalRecords)
		assert.Equal(t, int64(2), processor.metrics.ProcessedRecords)
		assert.Equal(t, int64(0), processor.metrics.FailedRecords)
	})

	t.Run("failure_returns_error_and_increments_failed", func(t *testing.T) {
		processor := NewStreamProcessor("test", func(_ context.Context, records []events.DynamoDBEventRecord) error {
			if len(records) > 0 && records[0].EventName == "FAIL" {
				return stderrors.New("boom")
			}
			return nil
		}, zap.NewNop())

		records := []events.DynamoDBEventRecord{
			{EventName: "FAIL"},
			{EventName: EventInsert},
		}

		err := processor.ProcessRecords(context.Background(), records)
		require.ErrorIs(t, err, services.ErrRecordProcessingFailed)

		assert.Equal(t, int64(2), processor.metrics.TotalRecords)
		assert.Equal(t, int64(1), processor.metrics.ProcessedRecords)
		assert.Equal(t, int64(1), processor.metrics.FailedRecords)
	})
}
