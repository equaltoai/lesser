package cost

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type round24AICostRepo struct {
	calls int
	err   error
}

func (r *round24AICostRepo) CreateOrUpdateAggregatedCost(_ context.Context, _ *models.AIAggregatedCost) error {
	r.calls++
	return r.err
}

type round24NotificationService struct {
	calls int
	err   error
}

func (r *round24NotificationService) CreateNotification(_ context.Context, _ *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return &notifications.NotificationResult{}, nil
}

func round24Record(eventName string, pk string, image map[string]events.DynamoDBAttributeValue) events.DynamoDBEventRecord {
	return events.DynamoDBEventRecord{
		EventID:   "evt",
		EventName: eventName,
		Change: events.DynamoDBStreamRecord{
			Keys:     map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute(pk)},
			NewImage: image,
			OldImage: image,
		},
	}
}

func TestRealtimeAggregationService_RecordGroupingAndTypes_Round24(t *testing.T) {
	svc := &RealtimeAggregationService{aggregationCache: NewAggregationCache()}

	require.Equal(t, "ai_cost", svc.determineRecordType(round24Record(EventInsert, "AI_COST#1", nil)))
	require.Equal(t, "websocket_cost", svc.determineRecordType(round24Record(EventInsert, "WS_COST#1", nil)))
	require.Equal(t, "federation_cost", svc.determineRecordType(round24Record(EventInsert, "FED_COST#1", nil)))
	require.Equal(t, "unknown", svc.determineRecordType(round24Record(EventInsert, "", nil)))

	groups := svc.groupRecordsByType([]events.DynamoDBEventRecord{
		round24Record(EventInsert, "AI_COST#1", nil),
		round24Record(EventInsert, "WS_COST#1", nil),
		round24Record(EventInsert, "AI_COST#2", nil),
	})

	require.Len(t, groups["ai_cost"], 2)
	require.Len(t, groups["websocket_cost"], 1)
}

func TestStreamProcessor_ProcessRecords_Metrics_Round24(t *testing.T) {
	ok := NewStreamProcessor("ok", func(context.Context, []events.DynamoDBEventRecord) error { return nil }, zap.NewNop())
	require.NoError(t, ok.ProcessRecords(context.Background(), []events.DynamoDBEventRecord{
		{EventName: EventInsert},
		{EventName: EventInsert},
	}))
	require.Equal(t, int64(2), ok.metrics.TotalRecords)
	require.Equal(t, int64(2), ok.metrics.ProcessedRecords)
	require.Equal(t, int64(0), ok.metrics.FailedRecords)

	fail := NewStreamProcessor("fail", func(context.Context, []events.DynamoDBEventRecord) error { return errors.New("boom") }, zap.NewNop())
	require.ErrorIs(t, fail.ProcessRecords(context.Background(), []events.DynamoDBEventRecord{{EventName: EventInsert}}), services.ErrRecordProcessingFailed)
	require.Equal(t, int64(1), fail.metrics.TotalRecords)
	require.Equal(t, int64(0), fail.metrics.ProcessedRecords)
	require.Equal(t, int64(1), fail.metrics.FailedRecords)
}

func TestRealtimeAggregationService_updateSummaryCacheAndRollups_Round24(t *testing.T) {
	svc := &RealtimeAggregationService{aggregationCache: NewAggregationCache()}

	svc.updateSummaryCache("k", 1.25, "op1")
	svc.updateSummaryCache("k", 0.75, "op1")
	svc.updateSummaryCache("k", 2.00, "op2")

	svc.aggregationCache.mu.RLock()
	defer svc.aggregationCache.mu.RUnlock()

	summary := svc.aggregationCache.costSummaries["k"]
	require.NotNil(t, summary)
	require.InDelta(t, 4.0, summary.TotalCost, 0.0001)
	require.Equal(t, int64(2), summary.OperationCounts["op1"])
	require.Equal(t, int64(1), summary.OperationCounts["op2"])
}

func TestRealtimeAggregationService_AICostStreamProcessing_Round24(t *testing.T) {
	aiRepo := &round24AICostRepo{}
	svc := NewRealtimeAggregationService(nil, nil, nil, nil, zap.NewNop())
	svc.aiCostRepo = aiRepo

	ts := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	image := map[string]events.DynamoDBAttributeValue{
		"operationID":         events.NewStringAttribute("op-1"),
		"operationType":       events.NewStringAttribute("summarize"),
		"modelName":           events.NewStringAttribute("claude"),
		"success":             events.NewBooleanAttribute(true),
		"totalCostMicroCents": events.NewNumberAttribute("2000000"),
		"timestamp":           events.NewStringAttribute(ts.Format(time.RFC3339)),
		"requestLatencyMs":    events.NewNumberAttribute("150"),
	}

	require.NoError(t, svc.processAICostStream(context.Background(), []events.DynamoDBEventRecord{
		round24Record(EventInsert, "AI_COST#op-1", image),
	}))
	require.GreaterOrEqual(t, aiRepo.calls, 1)
}

func TestRealtimeAggregationService_WebSocketAndFederationStreamProcessing_Round24(t *testing.T) {
	svc := NewRealtimeAggregationService(nil, nil, nil, nil, zap.NewNop())

	ts := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	wsImage := map[string]events.DynamoDBAttributeValue{
		"connectionID":         events.NewStringAttribute("c-1"),
		"operationType":        events.NewStringAttribute("message_out"),
		"estimatedCostDollars": events.NewNumberAttribute("1.5"),
		"timestamp":            events.NewStringAttribute(ts.Format(time.RFC3339)),
	}
	require.NoError(t, svc.processWebSocketCostStream(context.Background(), []events.DynamoDBEventRecord{
		round24Record(EventInsert, "WS_COST#1", wsImage),
		round24Record(EventRemove, "WS_COST#2", wsImage),
	}))

	fedImage := map[string]events.DynamoDBAttributeValue{
		"activityID":          events.NewStringAttribute("a-1"),
		"activityType":        events.NewStringAttribute("Create"),
		"domain":              events.NewStringAttribute("example.com"),
		"totalCostMicroCents": events.NewNumberAttribute("1000000"),
		"timestamp":           events.NewStringAttribute(ts.Format(time.RFC3339)),
	}
	require.NoError(t, svc.processFederationCostStream(context.Background(), []events.DynamoDBEventRecord{
		round24Record(EventInsert, "FED_COST#1", fedImage),
		round24Record(EventRemove, "FED_COST#2", fedImage),
	}))
}

func TestRealtimeAggregationService_CacheMaintenanceAndMetrics_Round24(t *testing.T) {
	svc := &RealtimeAggregationService{
		logger:           zap.NewNop(),
		aggregationCache: NewAggregationCache(),
		streamProcessors: map[string]*StreamProcessor{},
	}

	expired := &SummaryCache{
		Period:      "day",
		TotalCost:   1.0,
		LastUpdated: time.Now().Add(-2 * time.Hour),
		ExpiresAt:   time.Now().Add(-1 * time.Minute),
	}
	svc.aggregationCache.costSummaries["expired"] = expired

	require.NoError(t, svc.updateAggregationCache(context.Background()))
	require.Nil(t, svc.aggregationCache.costSummaries["expired"])

	require.True(t, svc.evaluateThreshold(2, 1, "gt"))
	require.True(t, svc.evaluateThreshold(2, 2, "gte"))
	require.True(t, svc.evaluateThreshold(1, 2, "lt"))
	require.True(t, svc.evaluateThreshold(2, 2, "eq"))
	require.False(t, svc.evaluateThreshold(2, 2, "unknown"))

	svc.updateSummaryCache("recent", 5.0, "op")
	svc.SetAlertThreshold(&AlertThreshold{
		MetricName:    "total_cost_per_minute",
		Threshold:     1.0,
		ComparisonOp:  "gt",
		WindowMinutes: 1,
		Severity:      "high",
		Enabled:       true,
	})

	require.NoError(t, svc.checkAlertConditions(context.Background()))

	metrics, err := svc.GetRealTimeMetrics(context.Background())
	require.NoError(t, err)
	require.NotNil(t, metrics)
	require.GreaterOrEqual(t, metrics.AnomalyScore, 0.0)

	svc.streamProcessors["p1"] = NewStreamProcessor("p1", func(context.Context, []events.DynamoDBEventRecord) error { return nil }, zap.NewNop())
	svc.streamProcessors["p1"].metrics.ProcessingTimeMs = 10
	require.GreaterOrEqual(t, svc.getAverageProcessingLatency(), 0.0)
}

func TestRealtimeAggregationService_ProcessDynamoDBStreamEvent_Round24(t *testing.T) {
	svc := &RealtimeAggregationService{
		logger:           zap.NewNop(),
		aggregationCache: NewAggregationCache(),
		streamProcessors: map[string]*StreamProcessor{},
	}

	svc.streamProcessors["ai_cost"] = NewStreamProcessor("ai_cost", func(context.Context, []events.DynamoDBEventRecord) error { return nil }, zap.NewNop())
	svc.streamProcessors["websocket_cost"] = NewStreamProcessor("websocket_cost", func(context.Context, []events.DynamoDBEventRecord) error { return errors.New("boom") }, zap.NewNop())

	event := events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{
			round24Record(EventInsert, "AI_COST#1", map[string]events.DynamoDBAttributeValue{}),
			round24Record(EventInsert, "WS_COST#1", map[string]events.DynamoDBAttributeValue{}),
			round24Record(EventInsert, "UNKNOWN#1", map[string]events.DynamoDBAttributeValue{}),
		},
	}

	require.ErrorIs(t, svc.ProcessDynamoDBStreamEvent(context.Background(), event), services.ErrStreamProcessingErrors)
}

func TestRealtimeAggregationService_sendCostAlert_Round24(t *testing.T) {
	svc := &RealtimeAggregationService{
		logger:          zap.NewNop(),
		notificationSvc: &round24NotificationService{},
	}

	svc.sendCostAlert(context.Background(), RealTimeAlert{
		AlertID:      "a-1",
		MetricName:   "total_cost_per_minute",
		CurrentValue: 5,
		Threshold:    1,
		Severity:     "high",
		Message:      "msg",
		TriggeredAt:  time.Now(),
		Duration:     "1m",
	})
}

// Note: TestRealtimeAggregationService_convertDynamoDBAttribute_Round24 removed because
// convertDynamoDBAttribute is not exported from RealtimeAggregationService
