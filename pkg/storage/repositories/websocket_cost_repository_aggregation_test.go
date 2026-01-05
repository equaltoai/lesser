package repositories

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Section 1: Percentile Helper Tests (Pure Functions - No Repository Needed)
// =============================================================================

func TestCalculateWebSocketPercentiles_EmptySlice(t *testing.T) {
	// Empty slice should return all-zero percentiles
	result := calculateWebSocketPercentiles(nil)

	assert.Equal(t, float64(0), result["p50"], "p50 should be 0 for empty slice")
	assert.Equal(t, float64(0), result["p90"], "p90 should be 0 for empty slice")
	assert.Equal(t, float64(0), result["p95"], "p95 should be 0 for empty slice")
	assert.Equal(t, float64(0), result["p99"], "p99 should be 0 for empty slice")

	// Also test with explicitly empty slice
	result2 := calculateWebSocketPercentiles([]float64{})
	assert.Equal(t, float64(0), result2["p50"], "p50 should be 0 for empty slice")
	assert.Equal(t, float64(0), result2["p90"], "p90 should be 0 for empty slice")
	assert.Equal(t, float64(0), result2["p95"], "p95 should be 0 for empty slice")
	assert.Equal(t, float64(0), result2["p99"], "p99 should be 0 for empty slice")
}

func TestCalculateWebSocketPercentiles_SingleElement(t *testing.T) {
	// Single element should return that value for all percentiles
	values := []float64{42.5}
	result := calculateWebSocketPercentiles(values)

	assert.Equal(t, 42.5, result["p50"], "p50 should be single value")
	assert.Equal(t, 42.5, result["p90"], "p90 should be single value")
	assert.Equal(t, 42.5, result["p95"], "p95 should be single value")
	assert.Equal(t, 42.5, result["p99"], "p99 should be single value")
}

func TestCalculateWebSocketPercentiles_MultiElement(t *testing.T) {
	// Test with a known dataset: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	result := calculateWebSocketPercentiles(values)

	// For 10 elements:
	// p50: index = 0.5 * 9 = 4.5, interpolate between values[4]=5 and values[5]=6 → 5.5
	// p90: index = 0.9 * 9 = 8.1, interpolate between values[8]=9 and values[9]=10 → 9.1
	// p95: index = 0.95 * 9 = 8.55, interpolate between values[8]=9 and values[9]=10 → 9.55
	// p99: index = 0.99 * 9 = 8.91, interpolate between values[8]=9 and values[9]=10 → 9.91
	assert.InDelta(t, 5.5, result["p50"], 0.01, "p50 should be ~5.5")
	assert.InDelta(t, 9.1, result["p90"], 0.01, "p90 should be ~9.1")
	assert.InDelta(t, 9.55, result["p95"], 0.01, "p95 should be ~9.55")
	assert.InDelta(t, 9.91, result["p99"], 0.01, "p99 should be ~9.91")
}

func TestCalculateWebSocketPercentiles_UnsortedInput(t *testing.T) {
	// Verify the function sorts the input before calculating
	unsorted := []float64{10, 3, 7, 1, 5, 9, 2, 6, 4, 8}
	result := calculateWebSocketPercentiles(unsorted)

	// Should produce same results as sorted input
	assert.InDelta(t, 5.5, result["p50"], 0.01, "p50 should be ~5.5")
	assert.InDelta(t, 9.1, result["p90"], 0.01, "p90 should be ~9.1")
}

func TestGetWebSocketPercentileValue_EmptySlice(t *testing.T) {
	result := getWebSocketPercentileValue(nil, 50)
	assert.Equal(t, float64(0), result, "empty slice should return 0")

	result2 := getWebSocketPercentileValue([]float64{}, 50)
	assert.Equal(t, float64(0), result2, "empty slice should return 0")
}

func TestGetWebSocketPercentileValue_SingleElement(t *testing.T) {
	sorted := []float64{42.5}

	testCases := []struct {
		name       string
		percentile float64
	}{
		{"p0", 0},
		{"p50", 50},
		{"p99", 99},
		{"p100", 100},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getWebSocketPercentileValue(sorted, tc.percentile)
			assert.Equal(t, 42.5, result, "single element should return that value")
		})
	}
}

func TestGetWebSocketPercentileValue_ExactIndex(t *testing.T) {
	// Test when percentile calculation results in an exact index
	// For 5 elements [10, 20, 30, 40, 50], index = percentile/100 * (5-1)
	sorted := []float64{10, 20, 30, 40, 50}

	// p0: index = 0 → value = 10
	assert.Equal(t, 10.0, getWebSocketPercentileValue(sorted, 0))

	// p25: index = 0.25 * 4 = 1 → value = 20
	assert.Equal(t, 20.0, getWebSocketPercentileValue(sorted, 25))

	// p50: index = 0.5 * 4 = 2 → value = 30
	assert.Equal(t, 30.0, getWebSocketPercentileValue(sorted, 50))

	// p75: index = 0.75 * 4 = 3 → value = 40
	assert.Equal(t, 40.0, getWebSocketPercentileValue(sorted, 75))

	// p100: index = 1.0 * 4 = 4 → value = 50
	assert.Equal(t, 50.0, getWebSocketPercentileValue(sorted, 100))
}

func TestGetWebSocketPercentileValue_Interpolation(t *testing.T) {
	// Test linear interpolation between values
	sorted := []float64{0, 100}

	// p50: index = 0.5 * 1 = 0.5, interpolate between 0 and 100 → 50
	assert.InDelta(t, 50.0, getWebSocketPercentileValue(sorted, 50), 0.01)

	// p25: index = 0.25 * 1 = 0.25, interpolate → 25
	assert.InDelta(t, 25.0, getWebSocketPercentileValue(sorted, 25), 0.01)

	// p75: index = 0.75 * 1 = 0.75, interpolate → 75
	assert.InDelta(t, 75.0, getWebSocketPercentileValue(sorted, 75), 0.01)
}

// =============================================================================
// Section 2: Aggregation Initialization Tests
// Note: These methods on WebSocketCostRepository don't require DB access,
// so we can test them using a nil repository pattern.
// =============================================================================

// newTestWebSocketCostRepo creates a minimal WebSocketCostRepository for testing
// pure aggregation helpers that don't need a database connection.
func newTestWebSocketCostRepo() *WebSocketCostRepository {
	return &WebSocketCostRepository{}
}

func TestWSCostAggregation_InitializeAggregation(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	windowStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC)

	agg := repo.initializeAggregation("hourly", "connect", windowStart, windowEnd)

	// Verify basic fields
	assert.Equal(t, "hourly", agg.Period)
	assert.Equal(t, "connect", agg.OperationType)
	assert.Equal(t, windowStart, agg.WindowStart)
	assert.Equal(t, windowEnd, agg.WindowEnd)

	// Verify all maps are initialized (non-nil)
	assert.NotNil(t, agg.CostPercentiles, "CostPercentiles should be initialized")
	assert.NotNil(t, agg.LatencyPercentiles, "LatencyPercentiles should be initialized")
	assert.NotNil(t, agg.ConnectionDurationPercentiles, "ConnectionDurationPercentiles should be initialized")
	assert.NotNil(t, agg.StreamPopularity, "StreamPopularity should be initialized")
	assert.NotNil(t, agg.StreamTypeBreakdown, "StreamTypeBreakdown should be initialized")
	assert.NotNil(t, agg.CostByTier, "CostByTier should be initialized")
}

func TestWSCostAggregation_CreateMetricCollectors(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	testCases := []struct {
		name     string
		capacity int
	}{
		{"zero capacity", 0},
		{"small capacity", 10},
		{"large capacity", 1000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			collectors := repo.createMetricCollectors(tc.capacity)

			// Verify maps are initialized
			assert.NotNil(t, collectors.uniqueUsers, "uniqueUsers map should be initialized")
			assert.NotNil(t, collectors.uniqueConnections, "uniqueConnections map should be initialized")
			assert.NotNil(t, collectors.uniqueStreams, "uniqueStreams map should be initialized")

			// Verify slices are initialized with proper capacity
			assert.NotNil(t, collectors.costValues, "costValues slice should be initialized")
			assert.Equal(t, 0, len(collectors.costValues), "costValues should be empty")
			assert.Equal(t, tc.capacity, cap(collectors.costValues), "costValues should have expected capacity")

			// Other slices don't have pre-allocated capacity
			assert.NotNil(t, collectors.latencyValues, "latencyValues slice should be initialized")
			assert.NotNil(t, collectors.durationValues, "durationValues slice should be initialized")

			// Verify numeric fields are zero
			assert.Equal(t, float64(0), collectors.totalProcessingTime)
			assert.Equal(t, float64(0), collectors.totalResponseLatency)
			assert.Equal(t, float64(0), collectors.totalMemoryUsage)
			assert.Equal(t, int64(0), collectors.measurementCount)
		})
	}
}

// =============================================================================
// Section 3: Per-Record Processing Tests
// =============================================================================

func TestWSCostAggregation_TrackUniqueEntities(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	testCases := []struct {
		name            string
		record          *models.WebSocketCostRecord
		expectedUsers   int
		expectedConns   int
		expectedStreams int
	}{
		{
			name: "full record with user and streams",
			record: &models.WebSocketCostRecord{
				UserID:        "user-1",
				ConnectionID:  "conn-1",
				ActiveStreams: []string{"stream-a", "stream-b"},
				StreamTypes:   []string{"public", "notification"},
			},
			expectedUsers:   1,
			expectedConns:   1,
			expectedStreams: 2,
		},
		{
			name: "record without user ID",
			record: &models.WebSocketCostRecord{
				UserID:        "",
				ConnectionID:  "conn-2",
				ActiveStreams: []string{"stream-c"},
				StreamTypes:   []string{"private"},
			},
			expectedUsers:   0,
			expectedConns:   1,
			expectedStreams: 1,
		},
		{
			name: "record without streams",
			record: &models.WebSocketCostRecord{
				UserID:        "user-2",
				ConnectionID:  "conn-3",
				ActiveStreams: nil,
				StreamTypes:   nil,
			},
			expectedUsers:   1,
			expectedConns:   1,
			expectedStreams: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			collectors := repo.createMetricCollectors(10)
			aggregation := repo.initializeAggregation("hourly", "connect",
				time.Now(), time.Now().Add(time.Hour))

			repo.trackUniqueEntities(tc.record, collectors, aggregation)

			assert.Equal(t, tc.expectedUsers, len(collectors.uniqueUsers))
			assert.Equal(t, tc.expectedConns, len(collectors.uniqueConnections))
			assert.Equal(t, tc.expectedStreams, len(collectors.uniqueStreams))

			// Verify stream popularity is updated
			for _, stream := range tc.record.ActiveStreams {
				assert.Equal(t, int64(1), aggregation.StreamPopularity[stream])
			}

			// Verify stream type breakdown is updated
			for _, streamType := range tc.record.StreamTypes {
				assert.Equal(t, int64(1), aggregation.StreamTypeBreakdown[streamType])
			}
		})
	}
}

func TestWSCostAggregation_TrackUniqueEntities_AccumulatesMultipleRecords(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	collectors := repo.createMetricCollectors(10)
	aggregation := repo.initializeAggregation("hourly", "connect",
		time.Now(), time.Now().Add(time.Hour))

	// Process first record
	record1 := &models.WebSocketCostRecord{
		UserID:        "user-1",
		ConnectionID:  "conn-1",
		ActiveStreams: []string{"stream-a"},
		StreamTypes:   []string{"public"},
	}
	repo.trackUniqueEntities(record1, collectors, aggregation)

	// Process second record with same user, different connection
	record2 := &models.WebSocketCostRecord{
		UserID:        "user-1",
		ConnectionID:  "conn-2",
		ActiveStreams: []string{"stream-a", "stream-b"},
		StreamTypes:   []string{"public", "private"},
	}
	repo.trackUniqueEntities(record2, collectors, aggregation)

	// Verify unique counts
	assert.Equal(t, 1, len(collectors.uniqueUsers), "same user should be counted once")
	assert.Equal(t, 2, len(collectors.uniqueConnections), "different connections should both be counted")
	assert.Equal(t, 2, len(collectors.uniqueStreams), "unique streams should be counted")

	// Verify stream popularity accumulates
	assert.Equal(t, int64(2), aggregation.StreamPopularity["stream-a"], "stream-a should be counted twice")
	assert.Equal(t, int64(1), aggregation.StreamPopularity["stream-b"], "stream-b should be counted once")
}

func TestWSCostAggregation_ProcessOperationMetrics(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	testCases := []struct {
		name                            string
		operationType                   string
		record                          *models.WebSocketCostRecord
		expectedConnections             int64
		expectedMessagesIn              int64
		expectedMessagesOut             int64
		expectedBytes                   int64
		expectedStreamSubscriptions     int64
		expectedMessageDeliveryFailures int64
		expectedDroppedConnections      int64
	}{
		{
			name:          "connect operation",
			operationType: WSEventConnect,
			record: &models.WebSocketCostRecord{
				OperationType:        WSEventConnect,
				ConnectionDurationMs: 60000, // 1 minute
			},
			expectedConnections: 1,
		},
		{
			name:          "disconnect operation",
			operationType: WSEventDisconnect,
			record: &models.WebSocketCostRecord{
				OperationType: WSEventDisconnect,
			},
			expectedDroppedConnections: 1,
		},
		{
			name:          "message_in operation",
			operationType: WSEventMessageIn,
			record: &models.WebSocketCostRecord{
				OperationType:    WSEventMessageIn,
				MessageCount:     5,
				MessageSizeBytes: 1024,
			},
			expectedMessagesIn: 5,
			expectedBytes:      1024,
		},
		{
			name:          "message_out operation",
			operationType: WSEventMessageOut,
			record: &models.WebSocketCostRecord{
				OperationType:    WSEventMessageOut,
				MessageCount:     3,
				MessageSizeBytes: 512,
			},
			expectedMessagesOut: 3,
			expectedBytes:       512,
		},
		{
			name:          "subscribe operation",
			operationType: WSEventSubscribe,
			record: &models.WebSocketCostRecord{
				OperationType: WSEventSubscribe,
			},
			expectedStreamSubscriptions: 1,
		},
		{
			name:          "error operation",
			operationType: "error",
			record: &models.WebSocketCostRecord{
				OperationType: "error",
			},
			expectedMessageDeliveryFailures: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			collectors := repo.createMetricCollectors(10)
			aggregation := repo.initializeAggregation("hourly", tc.operationType,
				time.Now(), time.Now().Add(time.Hour))

			repo.processOperationMetrics(tc.record, aggregation, collectors)

			assert.Equal(t, tc.expectedConnections, aggregation.TotalConnections)
			assert.Equal(t, tc.expectedMessagesIn, aggregation.TotalMessagesIn)
			assert.Equal(t, tc.expectedMessagesOut, aggregation.TotalMessagesOut)
			assert.Equal(t, tc.expectedBytes, aggregation.TotalMessageBytes)
			assert.Equal(t, tc.expectedStreamSubscriptions, aggregation.TotalStreamSubscriptions)
			assert.Equal(t, tc.expectedMessageDeliveryFailures, aggregation.MessageDeliveryFailures)
			assert.Equal(t, tc.expectedDroppedConnections, aggregation.DroppedConnections)
		})
	}
}

func TestWSCostAggregation_ProcessConnectOperation(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	testCases := []struct {
		name                            string
		connectionDurationMs            int64
		expectedTotalConnectionMinutes  int64
		expectedDurationValuesPopulated bool
	}{
		{
			name:                            "connection with duration",
			connectionDurationMs:            120000, // 2 minutes
			expectedTotalConnectionMinutes:  2,
			expectedDurationValuesPopulated: true,
		},
		{
			name:                            "connection without duration",
			connectionDurationMs:            0,
			expectedTotalConnectionMinutes:  0,
			expectedDurationValuesPopulated: false,
		},
		{
			name:                            "short connection",
			connectionDurationMs:            30000, // 30 seconds = 0.5 minutes
			expectedTotalConnectionMinutes:  0,     // Truncated to int64
			expectedDurationValuesPopulated: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			collectors := repo.createMetricCollectors(10)
			aggregation := repo.initializeAggregation("hourly", WSEventConnect,
				time.Now(), time.Now().Add(time.Hour))

			record := &models.WebSocketCostRecord{
				OperationType:        WSEventConnect,
				ConnectionDurationMs: tc.connectionDurationMs,
			}

			repo.processConnectOperation(record, aggregation, collectors)

			assert.Equal(t, int64(1), aggregation.TotalConnections, "TotalConnections should be incremented")
			assert.Equal(t, tc.expectedTotalConnectionMinutes, aggregation.TotalConnectionMinutes)

			if tc.expectedDurationValuesPopulated {
				assert.Equal(t, 1, len(collectors.durationValues), "duration values should be populated")
				expectedMinutes := float64(tc.connectionDurationMs) / (60 * 1000)
				assert.InDelta(t, expectedMinutes, collectors.durationValues[0], 0.01)
			} else {
				assert.Equal(t, 0, len(collectors.durationValues), "duration values should be empty")
			}
		})
	}
}

func TestWSCostAggregation_AggregateCostComponents(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	aggregation := repo.initializeAggregation("hourly", "connect",
		time.Now(), time.Now().Add(time.Hour))

	record := &models.WebSocketCostRecord{
		APIGatewayConnectionCost: 100,
		APIGatewayMessageCost:    200,
		LambdaExecutionCost:      300,
		DynamoDBCost:             50,
		DataTransferCost:         25,
		TotalCostMicroCents:      675,
	}

	repo.aggregateCostComponents(record, aggregation)

	assert.Equal(t, int64(100), aggregation.TotalAPIGatewayConnectionCost)
	assert.Equal(t, int64(200), aggregation.TotalAPIGatewayMessageCost)
	assert.Equal(t, int64(300), aggregation.TotalLambdaExecutionCost)
	assert.Equal(t, int64(50), aggregation.TotalDynamoDBCost)
	assert.Equal(t, int64(25), aggregation.TotalDataTransferCost)
	assert.Equal(t, int64(675), aggregation.TotalCostMicroCents)

	// Add another record and verify accumulation
	record2 := &models.WebSocketCostRecord{
		APIGatewayConnectionCost: 50,
		APIGatewayMessageCost:    100,
		LambdaExecutionCost:      150,
		DynamoDBCost:             25,
		DataTransferCost:         12,
		TotalCostMicroCents:      337,
	}

	repo.aggregateCostComponents(record2, aggregation)

	assert.Equal(t, int64(150), aggregation.TotalAPIGatewayConnectionCost)
	assert.Equal(t, int64(300), aggregation.TotalAPIGatewayMessageCost)
	assert.Equal(t, int64(450), aggregation.TotalLambdaExecutionCost)
	assert.Equal(t, int64(75), aggregation.TotalDynamoDBCost)
	assert.Equal(t, int64(37), aggregation.TotalDataTransferCost)
	assert.Equal(t, int64(1012), aggregation.TotalCostMicroCents)
}

func TestWSCostAggregation_CollectPerformanceMetrics(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	testCases := []struct {
		name                   string
		record                 *models.WebSocketCostRecord
		expectedCostValuesLen  int
		expectedLatencyLen     int
		expectedMeasurements   int64
		expectedProcessingTime float64
		expectedLatency        float64
		expectedMemory         float64
	}{
		{
			name: "full performance metrics",
			record: &models.WebSocketCostRecord{
				EstimatedCostDollars: 0.001,
				ProcessingTimeMs:     100,
				ResponseLatencyMs:    50,
				MemoryUsedMB:         256,
			},
			expectedCostValuesLen:  1,
			expectedLatencyLen:     1,
			expectedMeasurements:   1,
			expectedProcessingTime: 100,
			expectedLatency:        50,
			expectedMemory:         256,
		},
		{
			name: "zero processing time",
			record: &models.WebSocketCostRecord{
				EstimatedCostDollars: 0.002,
				ProcessingTimeMs:     0,
				ResponseLatencyMs:    25,
				MemoryUsedMB:         128,
			},
			expectedCostValuesLen:  1,
			expectedLatencyLen:     1,
			expectedMeasurements:   0, // Processing time is 0, so not counted
			expectedProcessingTime: 0,
			expectedLatency:        25,
			expectedMemory:         128,
		},
		{
			name: "zero latency",
			record: &models.WebSocketCostRecord{
				EstimatedCostDollars: 0.003,
				ProcessingTimeMs:     75,
				ResponseLatencyMs:    0,
				MemoryUsedMB:         64,
			},
			expectedCostValuesLen:  1,
			expectedLatencyLen:     0, // Zero latency not added
			expectedMeasurements:   1,
			expectedProcessingTime: 75,
			expectedLatency:        0,
			expectedMemory:         64,
		},
		{
			name: "zero memory",
			record: &models.WebSocketCostRecord{
				EstimatedCostDollars: 0.004,
				ProcessingTimeMs:     50,
				ResponseLatencyMs:    30,
				MemoryUsedMB:         0,
			},
			expectedCostValuesLen:  1,
			expectedLatencyLen:     1,
			expectedMeasurements:   1,
			expectedProcessingTime: 50,
			expectedLatency:        30,
			expectedMemory:         0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			collectors := repo.createMetricCollectors(10)

			repo.collectPerformanceMetrics(tc.record, collectors)

			assert.Equal(t, tc.expectedCostValuesLen, len(collectors.costValues))
			assert.Equal(t, tc.expectedLatencyLen, len(collectors.latencyValues))
			assert.Equal(t, tc.expectedMeasurements, collectors.measurementCount)
			assert.InDelta(t, tc.expectedProcessingTime, collectors.totalProcessingTime, 0.01)
			assert.InDelta(t, tc.expectedLatency, collectors.totalResponseLatency, 0.01)
			assert.InDelta(t, tc.expectedMemory, collectors.totalMemoryUsage, 0.01)
		})
	}
}

func TestWSCostAggregation_CollectPerformanceMetrics_Accumulates(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	collectors := repo.createMetricCollectors(10)

	records := []*models.WebSocketCostRecord{
		{
			EstimatedCostDollars: 0.001,
			ProcessingTimeMs:     100,
			ResponseLatencyMs:    50,
			MemoryUsedMB:         256,
		},
		{
			EstimatedCostDollars: 0.002,
			ProcessingTimeMs:     200,
			ResponseLatencyMs:    75,
			MemoryUsedMB:         512,
		},
		{
			EstimatedCostDollars: 0.003,
			ProcessingTimeMs:     150,
			ResponseLatencyMs:    60,
			MemoryUsedMB:         384,
		},
	}

	for _, record := range records {
		repo.collectPerformanceMetrics(record, collectors)
	}

	assert.Equal(t, 3, len(collectors.costValues))
	assert.Equal(t, 3, len(collectors.latencyValues))
	assert.Equal(t, int64(3), collectors.measurementCount)
	assert.InDelta(t, 450, collectors.totalProcessingTime, 0.01)  // 100+200+150
	assert.InDelta(t, 185, collectors.totalResponseLatency, 0.01) // 50+75+60
	assert.InDelta(t, 1152, collectors.totalMemoryUsage, 0.01)    // 256+512+384
}

// =============================================================================
// Section 4: Finalization Computation Tests
// =============================================================================

func TestWSCostAggregation_CalculateAverages_WithMeasurements(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	aggregation := repo.initializeAggregation("hourly", "connect",
		time.Now(), time.Now().Add(time.Hour))
	collectors := repo.createMetricCollectors(10)

	// Set up collectors with data
	collectors.measurementCount = 4
	collectors.totalProcessingTime = 400
	collectors.totalMemoryUsage = 1024
	collectors.latencyValues = []float64{10, 20, 30, 40}

	// Set connections for duration average
	aggregation.TotalConnections = 2
	aggregation.AverageConnectionDuration = 10 // Will be divided by TotalConnections

	repo.calculateAverages(aggregation, collectors)

	// Average processing time = 400 / 4 = 100
	assert.InDelta(t, 100, aggregation.AverageProcessingTime, 0.01)

	// Average memory usage = 1024 / 4 = 256
	assert.InDelta(t, 256, aggregation.AverageMemoryUsage, 0.01)

	// Average response latency = (10+20+30+40) / 4 = 25
	assert.InDelta(t, 25, aggregation.AverageResponseLatency, 0.01)

	// Average connection duration = 10 / 2 = 5
	assert.InDelta(t, 5, aggregation.AverageConnectionDuration, 0.01)
}

func TestWSCostAggregation_CalculateAverages_ZeroMeasurements(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	aggregation := repo.initializeAggregation("hourly", "connect",
		time.Now(), time.Now().Add(time.Hour))
	collectors := repo.createMetricCollectors(10)

	// No measurements
	collectors.measurementCount = 0

	repo.calculateAverages(aggregation, collectors)

	// Averages should remain zero
	assert.Equal(t, float64(0), aggregation.AverageProcessingTime)
	assert.Equal(t, float64(0), aggregation.AverageMemoryUsage)
	assert.Equal(t, float64(0), aggregation.AverageResponseLatency)
}

func TestWSCostAggregation_CalculateMessageMetrics(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	testCases := []struct {
		name                   string
		messagesIn             int64
		messagesOut            int64
		messageBytes           int64
		windowDuration         time.Duration
		expectedAverageSize    float64
		expectedThroughput     float64
		shouldCalculateMetrics bool
	}{
		{
			name:                   "normal message processing",
			messagesIn:             100,
			messagesOut:            200,
			messageBytes:           30000,
			windowDuration:         time.Hour,
			expectedAverageSize:    100,   // 30000 / 300 = 100
			expectedThroughput:     0.083, // 300 / 3600 ≈ 0.083
			shouldCalculateMetrics: true,
		},
		{
			name:                   "no messages",
			messagesIn:             0,
			messagesOut:            0,
			messageBytes:           0,
			windowDuration:         time.Hour,
			expectedAverageSize:    0,
			expectedThroughput:     0,
			shouldCalculateMetrics: false,
		},
		{
			name:                   "zero duration window",
			messagesIn:             50,
			messagesOut:            50,
			messageBytes:           10000,
			windowDuration:         0,
			expectedAverageSize:    100, // 10000 / 100 = 100
			expectedThroughput:     0,   // Can't calculate with zero duration
			shouldCalculateMetrics: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			windowStart := time.Now()
			windowEnd := windowStart.Add(tc.windowDuration)

			aggregation := repo.initializeAggregation("hourly", "message",
				windowStart, windowEnd)
			aggregation.TotalMessagesIn = tc.messagesIn
			aggregation.TotalMessagesOut = tc.messagesOut
			aggregation.TotalMessageBytes = tc.messageBytes

			repo.calculateMessageMetrics(aggregation, windowStart, windowEnd)

			assert.InDelta(t, tc.expectedAverageSize, aggregation.AverageMessageSize, 0.1)
			assert.InDelta(t, tc.expectedThroughput, aggregation.MessageThroughputPerSec, 0.01)
		})
	}
}

func TestWSCostAggregation_CalculatePercentiles(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	aggregation := repo.initializeAggregation("hourly", "connect",
		time.Now(), time.Now().Add(time.Hour))
	collectors := repo.createMetricCollectors(10)

	// Populate collectors
	collectors.costValues = []float64{1, 2, 3, 4, 5}
	collectors.latencyValues = []float64{10, 20, 30, 40, 50}
	collectors.durationValues = []float64{100, 200, 300, 400, 500}

	repo.calculatePercentiles(aggregation, collectors)

	// Verify cost percentiles are populated
	require.NotNil(t, aggregation.CostPercentiles)
	assert.Contains(t, aggregation.CostPercentiles, "p50")
	assert.Contains(t, aggregation.CostPercentiles, "p90")
	assert.Contains(t, aggregation.CostPercentiles, "p95")
	assert.Contains(t, aggregation.CostPercentiles, "p99")

	// Verify latency percentiles are populated
	require.NotNil(t, aggregation.LatencyPercentiles)
	assert.Contains(t, aggregation.LatencyPercentiles, "p50")

	// Verify duration percentiles are populated
	require.NotNil(t, aggregation.ConnectionDurationPercentiles)
	assert.Contains(t, aggregation.ConnectionDurationPercentiles, "p50")
}

func TestWSCostAggregation_CalculatePercentiles_EmptyValues(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	aggregation := repo.initializeAggregation("hourly", "connect",
		time.Now(), time.Now().Add(time.Hour))
	collectors := repo.createMetricCollectors(10)

	// Empty collectors - no values
	repo.calculatePercentiles(aggregation, collectors)

	// Percentile maps should remain at their initialized empty state
	assert.Equal(t, 0, len(aggregation.CostPercentiles))
	assert.Equal(t, 0, len(aggregation.LatencyPercentiles))
	assert.Equal(t, 0, len(aggregation.ConnectionDurationPercentiles))
}

func TestWSCostAggregation_FinalizeAggregation_FullPipeline(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	windowStart := time.Now()
	windowEnd := windowStart.Add(time.Hour)

	aggregation := repo.initializeAggregation("hourly", "message", windowStart, windowEnd)
	collectors := repo.createMetricCollectors(10)

	// Simulate processed records
	collectors.uniqueUsers["user-1"] = true
	collectors.uniqueUsers["user-2"] = true
	collectors.uniqueStreams["stream-a"] = true
	collectors.uniqueStreams["stream-b"] = true
	collectors.uniqueStreams["stream-c"] = true

	collectors.measurementCount = 2
	collectors.totalProcessingTime = 200
	collectors.totalMemoryUsage = 512
	collectors.latencyValues = []float64{10, 20}

	collectors.costValues = []float64{0.001, 0.002, 0.003}
	collectors.durationValues = []float64{1, 2, 3}

	// Set message data
	aggregation.TotalMessagesIn = 50
	aggregation.TotalMessagesOut = 100
	aggregation.TotalMessageBytes = 15000
	aggregation.TotalConnections = 2
	aggregation.AverageConnectionDuration = 6.0

	repo.finalizeAggregation(aggregation, collectors, windowStart, windowEnd)

	// Verify unique counts
	assert.Equal(t, int64(2), aggregation.UniqueUsers)
	assert.Equal(t, int64(3), aggregation.UniqueStreamsUsed)

	// Verify averages
	assert.InDelta(t, 100, aggregation.AverageProcessingTime, 0.01)   // 200/2
	assert.InDelta(t, 256, aggregation.AverageMemoryUsage, 0.01)      // 512/2
	assert.InDelta(t, 15, aggregation.AverageResponseLatency, 0.01)   // (10+20)/2
	assert.InDelta(t, 3, aggregation.AverageConnectionDuration, 0.01) // 6/2

	// Verify message metrics
	assert.InDelta(t, 100, aggregation.AverageMessageSize, 0.1) // 15000/150

	// Verify percentiles are populated
	assert.NotEmpty(t, aggregation.CostPercentiles)
	assert.NotEmpty(t, aggregation.LatencyPercentiles)
	assert.NotEmpty(t, aggregation.ConnectionDurationPercentiles)
}

// =============================================================================
// Section 5: Integration-style Tests (Pure Logic Only)
// =============================================================================

func TestWSCostAggregation_ProcessCostRecord_FullFlow(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	windowStart := time.Now()
	windowEnd := windowStart.Add(time.Hour)

	aggregation := repo.initializeAggregation("hourly", "connect", windowStart, windowEnd)
	collectors := repo.createMetricCollectors(10)

	record := &models.WebSocketCostRecord{
		OperationType:            WSEventConnect,
		UserID:                   "user-123",
		ConnectionID:             "conn-456",
		ActiveStreams:            []string{"public:timeline", "user:notifications"},
		StreamTypes:              []string{"public", "user"},
		ConnectionDurationMs:     120000, // 2 minutes
		APIGatewayConnectionCost: 100,
		APIGatewayMessageCost:    0,
		LambdaExecutionCost:      50,
		DynamoDBCost:             25,
		DataTransferCost:         10,
		TotalCostMicroCents:      185,
		EstimatedCostDollars:     0.000185,
		ProcessingTimeMs:         50,
		ResponseLatencyMs:        25,
		MemoryUsedMB:             256,
	}

	repo.processCostRecord(record, aggregation, collectors)

	// Verify entities tracked
	assert.True(t, collectors.uniqueUsers["user-123"])
	assert.True(t, collectors.uniqueConnections["conn-456"])
	assert.True(t, collectors.uniqueStreams["public:timeline"])
	assert.True(t, collectors.uniqueStreams["user:notifications"])

	// Verify counts
	assert.Equal(t, int64(1), aggregation.TotalConnections)
	assert.Equal(t, int64(2), aggregation.TotalConnectionMinutes)

	// Verify costs
	assert.Equal(t, int64(100), aggregation.TotalAPIGatewayConnectionCost)
	assert.Equal(t, int64(185), aggregation.TotalCostMicroCents)

	// Verify performance metrics collected
	assert.Equal(t, 1, len(collectors.costValues))
	assert.Equal(t, 1, len(collectors.latencyValues))
	assert.Equal(t, int64(1), collectors.measurementCount)
}

func TestWSCostAggregation_Pipeline_MultipleRecords(t *testing.T) {
	repo := newTestWebSocketCostRepo()

	windowStart := time.Now()
	windowEnd := windowStart.Add(time.Hour)

	aggregation := repo.initializeAggregation("hourly", "connect", windowStart, windowEnd)
	collectors := repo.createMetricCollectors(10)

	records := []*models.WebSocketCostRecord{
		{
			OperationType:        WSEventConnect,
			UserID:               "user-1",
			ConnectionID:         "conn-1",
			ActiveStreams:        []string{"stream-a"},
			ConnectionDurationMs: 60000,
			TotalCostMicroCents:  100,
			EstimatedCostDollars: 0.0001,
			ProcessingTimeMs:     100,
			ResponseLatencyMs:    50,
			MemoryUsedMB:         256,
		},
		{
			OperationType:        WSEventConnect,
			UserID:               "user-1", // Same user
			ConnectionID:         "conn-2",
			ActiveStreams:        []string{"stream-a", "stream-b"},
			ConnectionDurationMs: 120000,
			TotalCostMicroCents:  200,
			EstimatedCostDollars: 0.0002,
			ProcessingTimeMs:     150,
			ResponseLatencyMs:    75,
			MemoryUsedMB:         512,
		},
		{
			OperationType:        WSEventConnect,
			UserID:               "user-2", // Different user
			ConnectionID:         "conn-3",
			ActiveStreams:        []string{"stream-b"},
			ConnectionDurationMs: 90000,
			TotalCostMicroCents:  150,
			EstimatedCostDollars: 0.00015,
			ProcessingTimeMs:     75,
			ResponseLatencyMs:    40,
			MemoryUsedMB:         384,
		},
	}

	// Process all records
	for _, record := range records {
		repo.processCostRecord(record, aggregation, collectors)
	}

	// Finalize
	repo.finalizeAggregation(aggregation, collectors, windowStart, windowEnd)

	// Verify unique counts
	assert.Equal(t, 2, len(collectors.uniqueUsers), "should have 2 unique users")
	assert.Equal(t, 3, len(collectors.uniqueConnections), "should have 3 unique connections")
	assert.Equal(t, 2, len(collectors.uniqueStreams), "should have 2 unique streams")

	// Verify aggregated values
	assert.Equal(t, int64(3), aggregation.TotalConnections)
	assert.Equal(t, int64(450), aggregation.TotalCostMicroCents)

	// Verify stream popularity
	assert.Equal(t, int64(2), aggregation.StreamPopularity["stream-a"])
	assert.Equal(t, int64(2), aggregation.StreamPopularity["stream-b"])

	// Verify percentiles are calculated
	assert.NotEmpty(t, aggregation.CostPercentiles)
	assert.Contains(t, aggregation.CostPercentiles, "p50")
}
