package routing

import (
	"context"
	"testing"
	"time"

	fedTypes "github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestRoutingMetrics_ProcessEventSyncPaths(t *testing.T) {
	rm := NewRoutingMetrics(nil, zap.NewNop())

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

func TestRoutingMetrics_GetRouteMetrics_IncludesStoredAndCurrentWindow(t *testing.T) {
	ctx := context.Background()
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)

	db.On("WithContext", ctx).Return(db).Once()
	db.On("Model", mock.Anything).Return(query).Once()
	query.On("Where", "PK", "=", "METRICS#ROUTE#route-1").Return(query).Once()
	query.On("Where", "SK", ">", mock.Anything).Return(query).Once()
	query.On("Limit", 100).Return(query).Once()
	query.On("All", mock.AnythingOfType("*[]*models.RouteMetricsWindow")).
		Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.RouteMetricsWindow)
			*dest = []*models.RouteMetricsWindow{
				{
					MessageCount: 5,
					SuccessCount: 3,
					FailureCount: 2,
					TotalBytes:   100,
					TotalCost:    0.5,
					AvgLatency:   200,
				},
			}
		}).
		Return(nil).
		Once()

	rm := NewRoutingMetrics(db, zap.NewNop())
	rm.aggregator.mu.Lock()
	rm.aggregator.routeMetrics["route-1"] = &aggregatedRouteMetrics{
		RouteID:      "route-1",
		MessageCount: 2,
		SuccessCount: 1,
		FailureCount: 1,
		TotalBytes:   50,
		TotalCost:    0.2,
		TotalLatency: 500 * time.Millisecond,
	}
	rm.aggregator.mu.Unlock()

	metrics, err := rm.GetRouteMetrics(ctx, "route-1", 1*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	assert.Equal(t, int64(7), metrics.TotalMessages)
	assert.Equal(t, int64(4), metrics.SuccessfulCount)
	assert.Equal(t, int64(3), metrics.FailedCount)
	assert.Equal(t, int64(150), metrics.TotalBytes)
	assert.InDelta(t, 0.7, metrics.TotalCost, 0.0001)
	assert.Equal(t, 275*time.Millisecond, metrics.AvgLatency)

	db.AssertExpectations(t)
	query.AssertExpectations(t)
}

func TestRoutingMetrics_GetInstanceMetrics_IncludesStoredAndCurrentWindow(t *testing.T) {
	ctx := context.Background()
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)

	db.On("WithContext", ctx).Return(db).Once()
	db.On("Model", mock.Anything).Return(query).Once()
	query.On("Where", "PK", "=", "METRICS#INSTANCE#instance-1").Return(query).Once()
	query.On("Where", "SK", ">", mock.Anything).Return(query).Once()
	query.On("Limit", 100).Return(query).Once()
	query.On("OrderBy", "SK", "DESC").Return(query).Once()
	query.On("All", mock.AnythingOfType("*[]*models.InstanceMetricsWindow")).
		Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.InstanceMetricsWindow)
			*dest = []*models.InstanceMetricsWindow{
				{
					TotalMessages: 10,
					TotalBytes:    123,
					TotalCost:     0.01,
					Availability:  0.9,
					MessageTypes:  `{"create":2}`,
				},
			}
		}).
		Return(nil).
		Once()

	rm := NewRoutingMetrics(db, zap.NewNop())
	rm.aggregator.mu.Lock()
	rm.aggregator.instanceMetrics["instance-1"] = &aggregatedInstanceMetrics{
		InstanceID:    "instance-1",
		TotalMessages: 3,
		TotalBytes:    7,
		TotalCost:     0.02,
		Availability:  0.5,
		MessageTypes:  map[fedTypes.MessageType]int64{fedTypes.MessageTypeCreate: 1, fedTypes.MessageTypeUpdate: 1},
	}
	rm.aggregator.mu.Unlock()

	metrics, err := rm.GetInstanceMetrics(ctx, "instance-1", 1*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	assert.Equal(t, "instance-1", metrics.InstanceID)
	assert.Equal(t, int64(13), metrics.TotalMessages)
	assert.Equal(t, int64(130), metrics.TotalBytes)
	assert.InDelta(t, 0.03, metrics.TotalCost, 0.0001)
	assert.InDelta(t, 0.9, metrics.Availability, 0.0001)
	assert.Equal(t, int64(3), metrics.MessageTypes[fedTypes.MessageTypeCreate])
	assert.Equal(t, int64(1), metrics.MessageTypes[fedTypes.MessageTypeUpdate])

	db.AssertExpectations(t)
	query.AssertExpectations(t)
}

func TestRoutingMetrics_GetGlobalMetrics_IncludesStoredAndCurrentWindow(t *testing.T) {
	ctx := context.Background()
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)

	db.On("WithContext", ctx).Return(db).Once()
	db.On("Model", mock.Anything).Return(query).Once()
	query.On("Index", "gsi1").Return(query).Once()
	query.On("Where", "gsi1PK", "=", "METRICS#GLOBAL").Return(query).Once()
	query.On("Where", "gsi1SK", ">", mock.Anything).Return(query).Once()
	query.On("Limit", 100).Return(query).Once()
	query.On("OrderBy", "gsi1SK", "DESC").Return(query).Once()
	query.On("All", mock.AnythingOfType("*[]*models.GlobalMetricsWindow")).
		Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.GlobalMetricsWindow)
			*dest = []*models.GlobalMetricsWindow{
				{
					TotalMessages:   100,
					TotalBytes:      1000,
					TotalCost:       0.5,
					UniqueInstances: 3,
					ActiveRoutes:    4,
					HourlyVolume:    `[0,2,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0]`,
				},
			}
		}).
		Return(nil).
		Once()

	rm := NewRoutingMetrics(db, zap.NewNop())
	rm.aggregator.mu.Lock()
	rm.aggregator.globalMetrics.TotalMessages = 5
	rm.aggregator.globalMetrics.TotalBytes = 50
	rm.aggregator.globalMetrics.TotalCost = 0.1
	rm.aggregator.globalMetrics.HourlyVolume[1] = 3
	rm.aggregator.mu.Unlock()

	metrics, err := rm.GetGlobalMetrics(ctx, 1*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	assert.Equal(t, int64(105), metrics.TotalMessages)
	assert.Equal(t, int64(1050), metrics.TotalBytes)
	assert.InDelta(t, 0.6, metrics.TotalCost, 0.0001)
	assert.Equal(t, int64(4), metrics.ActiveRoutes)
	assert.Equal(t, int64(3), metrics.ActiveInstances)
	assert.Equal(t, int64(5), metrics.HourlyVolume[1])

	db.AssertExpectations(t)
	query.AssertExpectations(t)
}

func TestRoutingMetrics_Flush_PersistsAndResetsWhenExpired(t *testing.T) {
	ctx := context.Background()

	db := new(mocks.MockDB)
	routeQuery := new(mocks.MockQuery)
	instanceQuery := new(mocks.MockQuery)
	globalQuery := new(mocks.MockQuery)

	db.On("WithContext", ctx).Return(db).Times(3)
	db.On("Model", mock.Anything).Return(routeQuery).Once()
	db.On("Model", mock.Anything).Return(instanceQuery).Once()
	db.On("Model", mock.Anything).Return(globalQuery).Once()

	routeQuery.On("BatchCreate", mock.Anything).Return(nil).Once()
	instanceQuery.On("BatchCreate", mock.Anything).Return(nil).Once()
	globalQuery.On("CreateOrUpdate").Return(nil).Once()

	rm := NewRoutingMetrics(db, zap.NewNop())
	rm.aggregator.mu.Lock()
	rm.aggregator.windowStart = time.Now().Add(-10 * time.Minute)
	rm.aggregator.windowSize = 1 * time.Millisecond
	rm.aggregator.routeMetrics["route-1"] = &aggregatedRouteMetrics{
		RouteID:        "route-1",
		MessageCount:   1,
		SuccessCount:   1,
		FailureCount:   0,
		TotalBytes:     1,
		TotalCost:      0.01,
		TotalLatency:   100 * time.Millisecond,
		LatencyBuckets: map[int]int64{0: 1},
		ErrorTypes:     make(map[string]int64),
	}
	rm.aggregator.instanceMetrics["instance-1"] = &aggregatedInstanceMetrics{
		InstanceID:   "instance-1",
		MessageTypes: make(map[fedTypes.MessageType]int64),
	}
	rm.aggregator.mu.Unlock()

	require.NoError(t, rm.Flush(ctx))

	rm.aggregator.mu.RLock()
	assert.Empty(t, rm.aggregator.routeMetrics)
	assert.Empty(t, rm.aggregator.instanceMetrics)
	rm.aggregator.mu.RUnlock()

	db.AssertExpectations(t)
	routeQuery.AssertExpectations(t)
	instanceQuery.AssertExpectations(t)
	globalQuery.AssertExpectations(t)
}
