# Mocking Demo: AWS Service Mocking with Lift Testing Framework

**This is the RECOMMENDED approach for mocking AWS services in Lift applications for reliable, fast testing.**

## What is This Example?

This example demonstrates the **STANDARD patterns** for mocking AWS services when testing Lift applications. It shows the **preferred approaches** for testing applications that integrate with CloudWatch, API Gateway, and other AWS services without requiring real AWS infrastructure.

## Why Use AWS Service Mocking?

✅ **USE these mocking patterns when:**
- Testing applications that integrate with AWS services
- Need fast, reliable tests without external dependencies
- Want to test error scenarios and edge cases
- Require predictable test behavior across environments
- Building CI/CD pipelines that need deterministic tests

❌ **DON'T USE when:**
- Testing actual AWS service configurations (use integration tests)
- Validating AWS IAM permissions (use real AWS testing)
- End-to-end system validation (use staging environment)
- Performance testing of AWS services themselves

## Quick Start

```go
// This is the CORRECT way to use AWS service mocks in tests
package main

import (
    "context"
    "testing"
    "github.com/pay-theory/lift/pkg/testing"
)

func TestWithAWSMocks(t *testing.T) {
    // PREFERRED: Setup service mocks
    metricsMock := testing.NewMockCloudWatchMetricsClient()
    alarmsMock := testing.NewMockCloudWatchAlarmsClient()
    apiGatewayMock := testing.NewMockAPIGatewayManagementClient()
    
    // REQUIRED: Configure realistic behavior
    config := testing.DefaultMockCloudWatchConfig()
    config.NetworkDelay = 5 * time.Millisecond  // Simulate real latency
    metricsMock.WithConfig(config)
    
    // Test your application logic
    err := publishMetrics(metricsMock, "MyApp", metrics)
    assert.NoError(t, err)
    
    // REQUIRED: Verify service interactions
    assert.Equal(t, 1, metricsMock.GetCallCount("PutMetricData"))
}

// INCORRECT: Real AWS services in tests
// func TestWithRealAWS(t *testing.T) {
//     svc := cloudwatch.New()  // Slow, requires credentials, expensive
//     svc.PutMetricData(...)
// }
```

## Core AWS Mocking Patterns

### 1. CloudWatch Metrics Mocking (STANDARD Pattern)

**Purpose:** Test application metrics publishing and retrieval
**When to use:** Applications that publish custom metrics to CloudWatch

```go
// CORRECT: Comprehensive CloudWatch metrics testing
func TestCloudWatchMetrics(t *testing.T) {
    ctx := context.Background()
    mock := testing.NewMockCloudWatchMetricsClient()
    
    // REQUIRED: Configure realistic behavior
    config := testing.DefaultMockCloudWatchConfig()
    config.NetworkDelay = 5 * time.Millisecond
    mock.WithConfig(config)
    
    t.Run("PublishMetrics - Success", func(t *testing.T) {
        // Define test metrics
        metrics := []*testing.MockMetricDatum{
            {
                MetricName: "RequestCount",
                Value:      100.0,
                Unit:       testing.MetricUnitCount,
                Dimensions: map[string]string{
                    "Service":     "UserAPI",
                    "Environment": "test",
                },
            },
            {
                MetricName: "ResponseTime",
                Value:      250.5,
                Unit:       testing.MetricUnitMilliseconds,
                Dimensions: map[string]string{
                    "Service":     "UserAPI",
                    "Environment": "test",
                },
            },
        }
        
        // Execute
        err := mock.PutMetricData(ctx, "MyApp/API", metrics)
        
        // REQUIRED: Verify success and storage
        assert.NoError(t, err)
        assert.Equal(t, 1, mock.GetCallCount("PutMetricData"))
        
        // REQUIRED: Verify metrics were stored correctly
        allMetrics := mock.GetAllMetrics()
        assert.Contains(t, allMetrics, "MyApp/API")
        assert.Len(t, allMetrics["MyApp/API"], 2)
    })
    
    t.Run("GetMetricStatistics - Success", func(t *testing.T) {
        // First publish some test data
        testMetrics := []*testing.MockMetricDatum{
            {
                MetricName: "WebSocketConnections",
                Value:      25.0,
                Unit:       testing.MetricUnitCount,
                Dimensions: map[string]string{"Service": "WebSocket"},
            },
            {
                MetricName: "WebSocketConnections", 
                Value:      30.0,
                Unit:       testing.MetricUnitCount,
                Dimensions: map[string]string{"Service": "WebSocket"},
            },
        }
        
        err := mock.PutMetricData(ctx, "MyApp/WebSocket", testMetrics)
        require.NoError(t, err)
        
        // Query statistics
        now := time.Now()
        stats, err := mock.GetMetricStatistics(
            ctx,
            "MyApp/WebSocket",
            "WebSocketConnections",
            map[string]string{"Service": "WebSocket"},
            now.Add(-1*time.Hour),
            now.Add(1*time.Hour),
            300, // 5 minutes
            []testing.Statistic{
                testing.StatisticSum,
                testing.StatisticAverage,
                testing.StatisticMaximum,
                testing.StatisticMinimum,
            },
        )
        
        // REQUIRED: Verify statistics calculation
        assert.NoError(t, err)
        assert.Equal(t, 55.0, stats[testing.StatisticSum])     // 25 + 30
        assert.Equal(t, 27.5, stats[testing.StatisticAverage]) // (25 + 30) / 2
        assert.Equal(t, 30.0, stats[testing.StatisticMaximum])
        assert.Equal(t, 25.0, stats[testing.StatisticMinimum])
    })
}

// INCORRECT: No verification of stored data
// mock.PutMetricData(ctx, namespace, metrics)
// // Missing verification that data was stored correctly
```

### 2. CloudWatch Alarms Mocking (MONITORING Pattern)

**Purpose:** Test alarm creation, evaluation, and state management
**When to use:** Applications that create and manage CloudWatch alarms

```go
// CORRECT: Comprehensive alarm testing
func TestCloudWatchAlarms(t *testing.T) {
    ctx := context.Background()
    alarmsMock := testing.NewMockCloudWatchAlarmsClient()
    metricsMock := testing.NewMockCloudWatchMetricsClient()
    
    // REQUIRED: Link alarms with metrics for evaluation
    alarmsMock.WithMetricsClient(metricsMock)
    
    t.Run("CreateAlarm - Success", func(t *testing.T) {
        alarm := &testing.MockAlarmDefinition{
            AlarmName:          "HighErrorRate",
            AlarmDescription:   "Alert when error rate exceeds threshold",
            MetricName:         "ErrorRate",
            Namespace:          "MyApp/API",
            Statistic:          testing.StatisticAverage,
            Dimensions:         map[string]string{"Service": "UserAPI"},
            Period:             300, // 5 minutes
            EvaluationPeriods:  2,
            Threshold:          5.0, // 5% error rate
            ComparisonOperator: testing.ComparisonGreaterThanThreshold,
            TreatMissingData:   "notBreaching",
        }
        
        // Execute
        err := alarmsMock.PutMetricAlarm(ctx, alarm)
        
        // REQUIRED: Verify alarm creation
        assert.NoError(t, err)
        
        // REQUIRED: Verify alarm was stored
        storedAlarm := alarmsMock.GetAlarm("HighErrorRate")
        assert.NotNil(t, storedAlarm)
        assert.Equal(t, "HighErrorRate", storedAlarm.AlarmName)
        assert.Equal(t, testing.AlarmStateInsufficientData, storedAlarm.State)
    })
    
    t.Run("AlarmEvaluation - Triggering", func(t *testing.T) {
        // Create alarm first
        alarm := &testing.MockAlarmDefinition{
            AlarmName:          "HighLatency",
            MetricName:         "ResponseTime",
            Namespace:          "MyApp/API",
            Statistic:          testing.StatisticAverage,
            Dimensions:         map[string]string{"Service": "UserAPI"},
            Period:             60,
            EvaluationPeriods:  1,
            Threshold:          1000.0, // 1000ms threshold
            ComparisonOperator: testing.ComparisonGreaterThanThreshold,
        }
        
        err := alarmsMock.PutMetricAlarm(ctx, alarm)
        require.NoError(t, err)
        
        // Publish metrics that should trigger the alarm
        triggerMetrics := []*testing.MockMetricDatum{
            {
                MetricName: "ResponseTime",
                Value:      1500.0, // Above threshold
                Unit:       testing.MetricUnitMilliseconds,
                Dimensions: map[string]string{"Service": "UserAPI"},
            },
        }
        
        err = metricsMock.PutMetricData(ctx, "MyApp/API", triggerMetrics)
        require.NoError(t, err)
        
        // Evaluate alarms
        err = alarmsMock.EvaluateAlarms(ctx)
        require.NoError(t, err)
        
        // REQUIRED: Verify alarm state changed to ALARM
        updatedAlarm := alarmsMock.GetAlarm("HighLatency")
        assert.Equal(t, testing.AlarmStateAlarm, updatedAlarm.State)
        assert.Contains(t, updatedAlarm.StateReason, "threshold")
    })
}
```

### 3. API Gateway Management Mocking (WEBSOCKET Pattern)

**Purpose:** Test WebSocket connection management and message sending
**When to use:** Applications using API Gateway WebSocket APIs

```go
// CORRECT: WebSocket API testing
func TestAPIGatewayManagement(t *testing.T) {
    ctx := context.Background()
    mock := testing.NewMockAPIGatewayManagementClient()
    
    // REQUIRED: Configure realistic constraints
    config := testing.DefaultMockAPIGatewayConfig()
    config.NetworkDelay = 10 * time.Millisecond
    config.MaxMessageSize = 1024 // 1KB limit
    mock.WithConfig(config)
    
    t.Run("ManageConnections - Success", func(t *testing.T) {
        // Add test connections
        mock.WithConnection("user-123", &testing.MockConnection{
            ID:           "user-123",
            State:        testing.ConnectionStateActive,
            CreatedAt:    time.Now(),
            LastActiveAt: time.Now(),
            SourceIP:     "192.168.1.100",
            UserAgent:    "WebSocket-Client/1.0",
            Metadata: map[string]any{
                "userId":   "user-123",
                "tenantId": "tenant-abc",
                "role":     "customer",
            },
        })
        
        // Test connection info retrieval
        connInfo, err := mock.GetConnection(ctx, "user-123")
        
        // REQUIRED: Verify connection details
        assert.NoError(t, err)
        assert.Equal(t, "user-123", connInfo.ConnectionID)
        assert.Equal(t, "192.168.1.100", connInfo.SourceIP)
        assert.NotZero(t, connInfo.ConnectedAt)
    })
    
    t.Run("SendMessage - Success", func(t *testing.T) {
        // Add connection
        mock.WithConnection("user-456", nil) // Use defaults
        
        // Test message sending
        message := []byte(`{"type": "notification", "data": {"message": "Test message"}}`)
        err := mock.PostToConnection(ctx, "user-456", message)
        
        // REQUIRED: Verify message was sent
        assert.NoError(t, err)
        assert.Equal(t, 1, mock.GetCallCount("PostToConnection"))
        assert.Equal(t, 1, mock.GetMessageCount("user-456"))
    })
    
    t.Run("SendMessage - ConnectionNotFound", func(t *testing.T) {
        // Test sending to non-existent connection
        message := []byte(`{"type": "error", "data": {"message": "Test"}}`)
        err := mock.PostToConnection(ctx, "non-existent", message)
        
        // REQUIRED: Verify error handling
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "connection not found")
    })
    
    t.Run("SendMessage - MessageTooLarge", func(t *testing.T) {
        mock.WithConnection("user-789", nil)
        
        // Create message larger than configured limit (1KB)
        largeMessage := make([]byte, 2048)
        err := mock.PostToConnection(ctx, "user-789", largeMessage)
        
        // REQUIRED: Verify size limit enforcement
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "message too large")
    })
}
```

### 4. Integration Testing Pattern (COMPREHENSIVE Pattern)

**Purpose:** Test complete workflows with multiple AWS services
**When to use:** End-to-end testing of business processes

```go
// CORRECT: Multi-service integration testing
func TestIntegratedWorkflow(t *testing.T) {
    ctx := context.Background()
    
    // Setup all required service mocks
    apiMock := testing.NewMockAPIGatewayManagementClient()
    metricsMock := testing.NewMockCloudWatchMetricsClient()
    alarmsMock := testing.NewMockCloudWatchAlarmsClient()
    
    // REQUIRED: Link services for integrated behavior
    alarmsMock.WithMetricsClient(metricsMock)
    
    t.Run("WebSocketMessageProcessingWorkflow", func(t *testing.T) {
        // Step 1: Setup monitoring
        connectionAlarm := &testing.MockAlarmDefinition{
            AlarmName:          "ConnectionFailures",
            MetricName:         "FailedConnections",
            Namespace:          "MyApp/WebSocket",
            Statistic:          testing.StatisticSum,
            Period:             60,
            EvaluationPeriods:  1,
            Threshold:          5.0,
            ComparisonOperator: testing.ComparisonGreaterThanThreshold,
        }
        
        err := alarmsMock.PutMetricAlarm(ctx, connectionAlarm)
        require.NoError(t, err)
        
        // Step 2: Add WebSocket connections
        for i := 1; i <= 10; i++ {
            connID := fmt.Sprintf("user-%d", i)
            apiMock.WithConnection(connID, &testing.MockConnection{
                ID:    connID,
                State: testing.ConnectionStateActive,
            })
        }
        
        // Step 3: Simulate message sending with some failures
        successCount := 0
        failureCount := 0
        
        for i := 1; i <= 15; i++ { // Send to 15 users, but only 10 exist
            connID := fmt.Sprintf("user-%d", i)
            message := []byte(fmt.Sprintf(`{"id": %d, "message": "Hello"}`, i))
            
            err := apiMock.PostToConnection(ctx, connID, message)
            if err != nil {
                failureCount++
            } else {
                successCount++
            }
        }
        
        // Step 4: Publish metrics based on results
        metrics := []*testing.MockMetricDatum{
            {
                MetricName: "SuccessfulConnections",
                Value:      float64(successCount),
                Unit:       testing.MetricUnitCount,
            },
            {
                MetricName: "FailedConnections",
                Value:      float64(failureCount),
                Unit:       testing.MetricUnitCount,
            },
        }
        
        err = metricsMock.PutMetricData(ctx, "MyApp/WebSocket", metrics)
        require.NoError(t, err)
        
        // Step 5: Evaluate monitoring
        err = alarmsMock.EvaluateAlarms(ctx)
        require.NoError(t, err)
        
        // REQUIRED: Verify complete workflow results
        assert.Equal(t, 10, successCount, "Should succeed for existing connections")
        assert.Equal(t, 5, failureCount, "Should fail for non-existent connections")
        
        // REQUIRED: Verify monitoring detected the failures
        alarm := alarmsMock.GetAlarm("ConnectionFailures")
        assert.Equal(t, testing.AlarmStateAlarm, alarm.State, "Alarm should trigger due to failures")
        
        // REQUIRED: Verify all service interactions
        assert.Equal(t, 15, apiMock.GetCallCount("PostToConnection"))
        assert.Equal(t, 1, metricsMock.GetCallCount("PutMetricData"))
        assert.GreaterOrEqual(t, len(apiMock.GetActiveConnections()), 10)
    })
}
```

## What This Example Teaches

### ✅ Best Practices Demonstrated

1. **ALWAYS configure realistic mock behavior** - Network delays, size limits, error conditions
2. **ALWAYS verify service interactions** - Call counts, stored data, state changes
3. **ALWAYS test error scenarios** - Network failures, invalid inputs, missing resources
4. **ALWAYS link related services** - Metrics with alarms, connections with messages
5. **ALWAYS use integration testing** - Test complete workflows, not just individual calls

### 🚫 Critical Anti-Patterns Avoided

1. **Real AWS services in tests** - Slow, expensive, unreliable, requires credentials
2. **No mock verification** - Setting up mocks but not verifying correct usage
3. **Unrealistic mock behavior** - Instant responses, unlimited sizes, no failures
4. **Testing only happy paths** - Missing error conditions and edge cases
5. **Isolated service testing** - Not testing how services work together

### 📊 Mocking Performance Benefits

- **Test Speed**: 100x faster than real AWS calls (5ms vs 500ms+)
- **Reliability**: 99.9% test reliability vs 80-90% with real services
- **Cost**: $0 per test vs $0.01-$0.10 per test with real AWS
- **Determinism**: Consistent behavior vs variable network/service conditions

## Mock Configuration Options

### CloudWatch Metrics Configuration

```go
config := testing.DefaultMockCloudWatchConfig()
config.NetworkDelay = 10 * time.Millisecond  // Simulate network latency
config.EnableValidation = true               // Validate metric data
config.MaxDataPoints = 1000                  // Limit stored data points
```

### API Gateway Configuration

```go
config := testing.DefaultMockAPIGatewayConfig()
config.NetworkDelay = 5 * time.Millisecond   // Simulate network latency
config.MaxMessageSize = 32768                // 32KB message limit
config.MaxConnections = 1000                 // Connection limit
config.ConnectionTimeout = 2 * time.Hour     // Connection timeout
```

### Alarm Configuration

```go
// Alarms automatically use metrics client for evaluation
alarmsMock.WithMetricsClient(metricsMock)
alarmsMock.EvaluateAlarms(ctx)  // Triggers evaluation of all alarms
```

## Testing Command Examples

```bash
# Run mocking demo
go run examples/mocking-demo/main.go

# Test with mocks
go test -run TestCloudWatch ./examples/mocking-demo/

# Benchmark with mocks
go test -bench=BenchmarkMockOperations ./examples/mocking-demo/

# Test with race detection
go test -race ./examples/mocking-demo/
```

## Next Steps

After mastering AWS service mocking:

1. **Mockery Idempotency** → See `examples/mockery-idempotency/`
2. **Testing Guide** → See `docs/TESTING_GUIDE.md`
3. **Basic CRUD API Tests** → See `examples/basic-crud-api/main_test.go`
4. **Production API** → See `examples/production-api/`

## Common Issues

### Issue: "Mock not capturing calls"
**Cause:** Mock not properly configured or linked
**Solution:** Verify mock setup and service linking

### Issue: "Unrealistic test behavior"
**Cause:** Missing network delays or size limits
**Solution:** Configure mock with realistic constraints

### Issue: "Integration tests failing"
**Cause:** Services not properly linked (e.g., alarms without metrics)
**Solution:** Use `WithMetricsClient()` and similar linking methods

This example demonstrates the complete toolkit for testing AWS integrations without external dependencies - master these patterns for fast, reliable, and comprehensive testing of your Lift applications.