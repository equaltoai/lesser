package routing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	fedTypes "github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeDynamoClient struct {
	queryFn func(ctx context.Context, input *dynamodb.QueryInput) (*dynamodb.QueryOutput, error)

	batchWriteFn func(ctx context.Context, input *dynamodb.BatchWriteItemInput) (*dynamodb.BatchWriteItemOutput, error)
}

func (f fakeDynamoClient) Query(ctx context.Context, input *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	if f.queryFn != nil {
		return f.queryFn(ctx, input)
	}
	return &dynamodb.QueryOutput{}, nil
}

func (f fakeDynamoClient) BatchWriteItem(ctx context.Context, input *dynamodb.BatchWriteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	if f.batchWriteFn != nil {
		return f.batchWriteFn(ctx, input)
	}
	return &dynamodb.BatchWriteItemOutput{}, nil
}

func TestRoutingMetrics_ProcessEventSyncPaths(t *testing.T) {
	rm := NewRoutingMetrics(nil, "table", zap.NewNop())

	rm.RecordRouteSelection("route-1", "example.com", fedTypes.MessageTypeCreate)
	rm.RecordDelivery(&fedTypes.DeliveryResult{
		RouteID:      "route-1",
		Timestamp:    time.Now(),
		Duration:     250 * time.Millisecond,
		Success:      true,
		BytesSent:    100,
		Cost:         0.1,
		ErrorMessage: "",
	})
	rm.RecordDelivery(&fedTypes.DeliveryResult{
		RouteID:      "route-1",
		Timestamp:    time.Now(),
		Duration:     150 * time.Millisecond,
		Success:      false,
		BytesSent:    10,
		Cost:         0.01,
		ErrorMessage: "timeout",
	})
	rm.RecordCircuitChange("route-1", fedTypes.CircuitClosed, fedTypes.CircuitOpen)
	rm.RecordHealthCheck("instance-1", &fedTypes.HealthStatus{Timestamp: time.Now(), Reachable: true, ResponseTime: 50 * time.Millisecond})
	rm.RecordHealthCheck("instance-1", &fedTypes.HealthStatus{Timestamp: time.Now(), Reachable: false, ResponseTime: 10 * time.Millisecond})

	rm.aggregator.mu.RLock()
	defer rm.aggregator.mu.RUnlock()

	require.NotNil(t, rm.aggregator.routeMetrics["route-1"])
	assert.Equal(t, int64(1), rm.aggregator.routeMetrics["route-1"].MessageCount)
	assert.Equal(t, int64(1), rm.aggregator.routeMetrics["route-1"].SuccessCount)
	assert.Equal(t, int64(1), rm.aggregator.routeMetrics["route-1"].FailureCount)
	assert.GreaterOrEqual(t, rm.aggregator.routeMetrics["route-1"].CircuitChanges, int64(1))
	assert.NotEmpty(t, rm.aggregator.routeMetrics["route-1"].LatencyBuckets)

	require.NotNil(t, rm.aggregator.instanceMetrics["instance-1"])
	assert.Equal(t, int64(2), rm.aggregator.instanceMetrics["instance-1"].HealthChecks)

	assert.GreaterOrEqual(t, rm.aggregator.globalMetrics.TotalMessages, int64(1))
	assert.GreaterOrEqual(t, rm.aggregator.globalMetrics.TotalBytes, int64(1))
	assert.GreaterOrEqual(t, rm.aggregator.globalMetrics.TotalCost, 0.0)
}

func TestRoutingMetrics_GetMetricsAndFlush_DBPaths(t *testing.T) {
	now := time.Now()

	db := fakeDynamoClient{
		queryFn: func(_ context.Context, input *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
			// Ensure the query has a PK expression placeholder.
			require.NotNil(t, input.ExpressionAttributeValues)
			return &dynamodb.QueryOutput{
				Items: []map[string]ddbTypes.AttributeValue{
					{
						"MessageCount": &ddbTypes.AttributeValueMemberN{Value: "5"},
						"SuccessCount": &ddbTypes.AttributeValueMemberN{Value: "3"},
						"FailureCount": &ddbTypes.AttributeValueMemberN{Value: "2"},
						"TotalBytes":   &ddbTypes.AttributeValueMemberN{Value: "100"},
						"TotalCost":    &ddbTypes.AttributeValueMemberN{Value: "0.5"},
					},
				},
			}, nil
		},
		batchWriteFn: func(_ context.Context, input *dynamodb.BatchWriteItemInput) (*dynamodb.BatchWriteItemOutput, error) {
			require.NotNil(t, input.RequestItems)
			require.NotEmpty(t, input.RequestItems["table"])
			return &dynamodb.BatchWriteItemOutput{}, nil
		},
	}

	rm := NewRoutingMetrics(nil, "table", zap.NewNop())
	rm.db = db

	// Current window data should be included in query results.
	rm.aggregator.mu.Lock()
	rm.aggregator.windowStart = now.Add(-10 * time.Minute)
	rm.aggregator.windowSize = 1 * time.Millisecond
	rm.aggregator.routeMetrics["route-1"] = &aggregatedRouteMetrics{
		RouteID:      "route-1",
		MessageCount: 2,
		SuccessCount: 1,
		FailureCount: 1,
		TotalBytes:   50,
		TotalCost:    0.2,
	}
	rm.aggregator.instanceMetrics["instance-1"] = &aggregatedInstanceMetrics{
		InstanceID:   "instance-1",
		MessageTypes: make(map[fedTypes.MessageType]int64),
	}
	rm.aggregator.globalMetrics.TotalMessages = 2
	rm.aggregator.globalMetrics.TotalBytes = 50
	rm.aggregator.globalMetrics.TotalCost = 0.2
	rm.aggregator.mu.Unlock()

	routeMetrics, err := rm.GetRouteMetrics(context.Background(), "route-1", 1*time.Minute)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, routeMetrics.TotalMessages, int64(5))
	assert.GreaterOrEqual(t, routeMetrics.TotalBytes, int64(100))
	assert.GreaterOrEqual(t, routeMetrics.TotalCost, 0.5)

	instanceMetrics, err := rm.GetInstanceMetrics(context.Background(), "instance-1", 1*time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, "instance-1", instanceMetrics.InstanceID)

	globalMetrics, err := rm.GetGlobalMetrics(context.Background(), 1*time.Minute)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, globalMetrics.TotalMessages, int64(1))

	// Flush persists and resets window when expired.
	assert.NoError(t, rm.Flush(context.Background()))
	rm.aggregator.mu.RLock()
	assert.Empty(t, rm.aggregator.routeMetrics)
	assert.Empty(t, rm.aggregator.instanceMetrics)
	rm.aggregator.mu.RUnlock()

	// Query errors are surfaced with wrapped errors.
	rm.db = fakeDynamoClient{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
			return nil, errors.New("query failed")
		},
	}
	_, err = rm.GetRouteMetrics(context.Background(), "route-1", 1*time.Minute)
	assert.Error(t, err)

	// Batch write errors bubble through Flush.
	rm.db = fakeDynamoClient{
		queryFn: db.queryFn,
		batchWriteFn: func(_ context.Context, _ *dynamodb.BatchWriteItemInput) (*dynamodb.BatchWriteItemOutput, error) {
			return nil, errors.New("batch write failed")
		},
	}
	rm.aggregator.mu.Lock()
	rm.aggregator.windowStart = time.Now().Add(-10 * time.Minute)
	rm.aggregator.windowSize = 1 * time.Millisecond
	rm.aggregator.routeMetrics["route-2"] = &aggregatedRouteMetrics{
		RouteID:        "route-2",
		MessageCount:   1,
		SuccessCount:   1,
		FailureCount:   0,
		TotalBytes:     1,
		TotalCost:      0,
		TotalLatency:   100 * time.Millisecond,
		LatencyBuckets: map[int]int64{0: 1},
		ErrorTypes:     make(map[string]int64),
	}
	rm.aggregator.mu.Unlock()
	assert.Error(t, rm.Flush(context.Background()))
}

func TestRoutingMetrics_AggregateParseWarnings(t *testing.T) {
	rm := NewRoutingMetrics(nil, "table", zap.NewNop())

	route := &fedTypes.RouteMetrics{}
	rm.aggregateRouteMetric(route, map[string]ddbTypes.AttributeValue{
		"MessageCount": &ddbTypes.AttributeValueMemberN{Value: "not-a-number"},
	})
	assert.Equal(t, int64(0), route.TotalMessages)

	instance := &InstanceMetrics{MessageTypes: make(map[fedTypes.MessageType]int64)}
	rm.aggregateInstanceMetric(instance, map[string]ddbTypes.AttributeValue{
		"TotalMessages": &ddbTypes.AttributeValueMemberN{Value: "not-a-number"},
		"Availability":  &ddbTypes.AttributeValueMemberN{Value: "not-a-number"},
	})
	assert.Equal(t, int64(0), instance.TotalMessages)

	global := &GlobalMetrics{HourlyVolume: make(map[int]int64)}
	rm.aggregateGlobalMetric(global, map[string]ddbTypes.AttributeValue{
		"TotalCost": &ddbTypes.AttributeValueMemberN{Value: "not-a-number"},
	})
	assert.InDelta(t, 0.0, global.TotalCost, 0.0001)
}
