package testing

import (
	"context"
	"testing"
	"time"
)

// TestMockAPIGatewayManagementClient tests the API Gateway Management mock
func TestMockAPIGatewayManagementClient(t *testing.T) {
	ctx := context.Background()
	mock := NewMockAPIGatewayManagementClient()

	// Test PostToConnection with non-existent connection
	err := mock.PostToConnection(ctx, "non-existent", []byte("test message"))
	if err == nil {
		t.Error("Expected error for non-existent connection")
	}

	// Add a connection
	mock.WithConnection("conn-123", nil)

	// Test successful PostToConnection
	message := []byte("Hello WebSocket!")
	err = mock.PostToConnection(ctx, "conn-123", message)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify message was stored
	messages := mock.GetMessages("conn-123")
	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}
	if string(messages[0]) != string(message) {
		t.Errorf("Expected message %s, got %s", string(message), string(messages[0]))
	}

	// Test GetConnection
	connInfo, err := mock.GetConnection(ctx, "conn-123")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if connInfo.ConnectionID != "conn-123" {
		t.Errorf("Expected connection ID conn-123, got %s", connInfo.ConnectionID)
	}

	// Test DeleteConnection
	err = mock.DeleteConnection(ctx, "conn-123")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify connection state changed
	conn := mock.GetConnectionState("conn-123")
	if conn.State != ConnectionStateDisconnected {
		t.Errorf("Expected connection state %s, got %s", ConnectionStateDisconnected, conn.State)
	}

	// Test call counting
	if mock.GetCallCount("PostToConnection") != 2 {
		t.Errorf("Expected 2 PostToConnection calls, got %d", mock.GetCallCount("PostToConnection"))
	}
}

// TestMockAPIGatewayManagementClientErrors tests error scenarios
func TestMockAPIGatewayManagementClientErrors(t *testing.T) {
	ctx := context.Background()
	mock := NewMockAPIGatewayManagementClient()

	// Test message size limit
	config := DefaultMockAPIGatewayConfig()
	config.MaxMessageSize = 10
	mock.WithConfig(config)
	mock.WithConnection("conn-123", nil)

	largeMessage := make([]byte, 20)
	err := mock.PostToConnection(ctx, "conn-123", largeMessage)
	if err == nil {
		t.Error("Expected error for oversized message")
	}

	// Test configured error
	customError := &MockAPIGatewayError{
		Code:       "TestException",
		Message:    "Test error",
		StatusCode: 500,
		Retryable:  true,
	}
	mock.WithError("conn-456", customError)

	err = mock.PostToConnection(ctx, "conn-456", []byte("test"))
	if err != customError {
		t.Errorf("Expected custom error, got %v", err)
	}
}

// TestMockAPIGatewayManagementClientTTL tests connection TTL functionality
func TestMockAPIGatewayManagementClientTTL(t *testing.T) {
	ctx := context.Background()
	mock := NewMockAPIGatewayManagementClient()

	// Set very short TTL for testing
	config := DefaultMockAPIGatewayConfig()
	config.ConnectionTTL = 1 // 1 second
	mock.WithConfig(config)

	// Add connection with past creation time
	pastTime := time.Now().Add(-2 * time.Second)
	conn := &MockConnection{
		ID:           "conn-expired",
		State:        ConnectionStateActive,
		CreatedAt:    pastTime,
		LastActiveAt: pastTime,
		SourceIP:     "127.0.0.1",
		UserAgent:    "TestClient/1.0",
		Metadata:     make(map[string]any),
	}
	mock.WithConnection("conn-expired", conn)

	// Try to send message to expired connection
	err := mock.PostToConnection(ctx, "conn-expired", []byte("test"))
	if err == nil {
		t.Error("Expected error for expired connection")
	}

	// Verify connection state changed to stale
	updatedConn := mock.GetConnectionState("conn-expired")
	if updatedConn.State != ConnectionStateStale {
		t.Errorf("Expected connection state %s, got %s", ConnectionStateStale, updatedConn.State)
	}
}

// TestMockCloudWatchMetricsClient tests the CloudWatch Metrics mock
func TestMockCloudWatchMetricsClient(t *testing.T) {
	ctx := context.Background()
	mock := NewMockCloudWatchMetricsClient()

	// Test PutMetricData
	metrics := []*MockMetricDatum{
		{
			MetricName: "RequestCount",
			Value:      10.0,
			Unit:       MetricUnitCount,
			Dimensions: map[string]string{
				"Service": "WebSocket",
			},
		},
		{
			MetricName: "ResponseTime",
			Value:      150.5,
			Unit:       MetricUnitMilliseconds,
			Dimensions: map[string]string{
				"Service": "WebSocket",
			},
		},
	}

	err := mock.PutMetricData(ctx, "PayTheory/Streamer", metrics)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify metrics were stored
	storedMetrics := mock.GetMetrics("PayTheory/Streamer")
	if len(storedMetrics) != 2 {
		t.Errorf("Expected 2 metrics, got %d", len(storedMetrics))
	}

	// Test GetMetricStatistics
	now := time.Now()
	startTime := now.Add(-1 * time.Hour)
	endTime := now.Add(1 * time.Hour)

	stats, err := mock.GetMetricStatistics(
		ctx,
		"PayTheory/Streamer",
		"RequestCount",
		map[string]string{"Service": "WebSocket"},
		startTime,
		endTime,
		300, // 5 minutes
		[]Statistic{StatisticSum, StatisticAverage, StatisticSampleCount},
	)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if stats[StatisticSum] != 10.0 {
		t.Errorf("Expected sum 10.0, got %f", stats[StatisticSum])
	}
	if stats[StatisticAverage] != 10.0 {
		t.Errorf("Expected average 10.0, got %f", stats[StatisticAverage])
	}
	if stats[StatisticSampleCount] != 1.0 {
		t.Errorf("Expected sample count 1.0, got %f", stats[StatisticSampleCount])
	}

	// Test call counting
	if mock.GetCallCount("PutMetricData") != 1 {
		t.Errorf("Expected 1 PutMetricData call, got %d", mock.GetCallCount("PutMetricData"))
	}
}

// TestMockCloudWatchAlarmsClient tests the CloudWatch Alarms mock
func TestMockCloudWatchAlarmsClient(t *testing.T) {
	ctx := context.Background()
	alarmsMock := NewMockCloudWatchAlarmsClient()
	metricsMock := NewMockCloudWatchMetricsClient()

	// Link alarms client with metrics client for evaluation
	alarmsMock.WithMetricsClient(metricsMock)

	// Create an alarm
	alarm := &MockAlarmDefinition{
		AlarmName:          "HighRequestCount",
		AlarmDescription:   "Alert when request count is high",
		MetricName:         "RequestCount",
		Namespace:          "PayTheory/Streamer",
		Statistic:          StatisticSum,
		Dimensions:         map[string]string{"Service": "WebSocket"},
		Period:             300, // 5 minutes
		EvaluationPeriods:  1,
		Threshold:          100.0,
		ComparisonOperator: ComparisonGreaterThanThreshold,
		TreatMissingData:   "notBreaching",
	}

	err := alarmsMock.PutMetricAlarm(ctx, alarm)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify alarm was created
	storedAlarm := alarmsMock.GetAlarm("HighRequestCount")
	if storedAlarm == nil {
		t.Error("Alarm was not stored")
		return
	}
	if storedAlarm.State != AlarmStateInsufficientData {
		t.Errorf("Expected initial state %s, got %s", AlarmStateInsufficientData, storedAlarm.State)
	}

	// Add metrics that should trigger the alarm
	metrics := []*MockMetricDatum{
		{
			MetricName: "RequestCount",
			Value:      150.0, // Above threshold
			Unit:       MetricUnitCount,
			Dimensions: map[string]string{"Service": "WebSocket"},
		},
	}

	err = metricsMock.PutMetricData(ctx, "PayTheory/Streamer", metrics)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Evaluate alarms
	err = alarmsMock.EvaluateAlarms(ctx)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify alarm state changed
	updatedAlarm := alarmsMock.GetAlarm("HighRequestCount")
	if updatedAlarm.State != AlarmStateAlarm {
		t.Errorf("Expected alarm state %s, got %s", AlarmStateAlarm, updatedAlarm.State)
	}

	// Test DescribeAlarms
	alarms, err := alarmsMock.DescribeAlarms(ctx, []string{"HighRequestCount"})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(alarms) != 1 {
		t.Errorf("Expected 1 alarm, got %d", len(alarms))
	}

	// Test DeleteAlarms
	err = alarmsMock.DeleteAlarms(ctx, []string{"HighRequestCount"})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify alarm was deleted
	deletedAlarm := alarmsMock.GetAlarm("HighRequestCount")
	if deletedAlarm != nil {
		t.Error("Alarm was not deleted")
	}
}

// TestMockCloudWatchMetricsClientValidation tests input validation
func TestMockCloudWatchMetricsClientValidation(t *testing.T) {
	ctx := context.Background()
	mock := NewMockCloudWatchMetricsClient()

	// Test empty namespace
	err := mock.PutMetricData(ctx, "", []*MockMetricDatum{
		{MetricName: "Test", Value: 1.0, Unit: MetricUnitCount},
	})
	if err == nil {
		t.Error("Expected error for empty namespace")
	}

	// Test empty metrics
	err = mock.PutMetricData(ctx, "TestNamespace", []*MockMetricDatum{})
	if err == nil {
		t.Error("Expected error for empty metrics")
	}

	// Test too many metrics
	config := DefaultMockCloudWatchConfig()
	config.MaxMetricsPerCall = 1
	mock.WithConfig(config)

	tooManyMetrics := []*MockMetricDatum{
		{MetricName: "Test1", Value: 1.0, Unit: MetricUnitCount},
		{MetricName: "Test2", Value: 2.0, Unit: MetricUnitCount},
	}

	err = mock.PutMetricData(ctx, "TestNamespace", tooManyMetrics)
	if err == nil {
		t.Error("Expected error for too many metrics")
	}
}

// TestMockCloudWatchAlarmsClientValidation tests alarm validation
func TestMockCloudWatchAlarmsClientValidation(t *testing.T) {
	ctx := context.Background()
	mock := NewMockCloudWatchAlarmsClient()

	// Test missing alarm name
	alarm := &MockAlarmDefinition{
		MetricName: "TestMetric",
		Namespace:  "TestNamespace",
	}
	err := mock.PutMetricAlarm(ctx, alarm)
	if err == nil {
		t.Error("Expected error for missing alarm name")
	}

	// Test missing metric name
	alarm = &MockAlarmDefinition{
		AlarmName: "TestAlarm",
		Namespace: "TestNamespace",
	}
	err = mock.PutMetricAlarm(ctx, alarm)
	if err == nil {
		t.Error("Expected error for missing metric name")
	}

	// Test missing namespace
	alarm = &MockAlarmDefinition{
		AlarmName:  "TestAlarm",
		MetricName: "TestMetric",
	}
	err = mock.PutMetricAlarm(ctx, alarm)
	if err == nil {
		t.Error("Expected error for missing namespace")
	}
}

// TestMockReset tests the Reset functionality
func TestMockReset(t *testing.T) {
	ctx := context.Background()

	// Test API Gateway mock reset
	apiMock := NewMockAPIGatewayManagementClient()
	apiMock.WithConnection("conn-123", nil)
	apiMock.PostToConnection(ctx, "conn-123", []byte("test"))

	if len(apiMock.GetActiveConnections()) == 0 {
		t.Error("Expected active connections before reset")
	}

	apiMock.Reset()

	if len(apiMock.GetActiveConnections()) != 0 {
		t.Error("Expected no active connections after reset")
	}
	if apiMock.GetCallCount("PostToConnection") != 0 {
		t.Error("Expected call count to be reset")
	}

	// Test CloudWatch metrics mock reset
	metricsMock := NewMockCloudWatchMetricsClient()
	metricsMock.PutMetricData(ctx, "TestNamespace", []*MockMetricDatum{
		{MetricName: "Test", Value: 1.0, Unit: MetricUnitCount},
	})

	if len(metricsMock.GetAllMetrics()) == 0 {
		t.Error("Expected metrics before reset")
	}

	metricsMock.Reset()

	if len(metricsMock.GetAllMetrics()) != 0 {
		t.Error("Expected no metrics after reset")
	}
	if metricsMock.GetCallCount("PutMetricData") != 0 {
		t.Error("Expected call count to be reset")
	}

	// Test CloudWatch alarms mock reset
	alarmsMock := NewMockCloudWatchAlarmsClient()
	alarmsMock.PutMetricAlarm(ctx, &MockAlarmDefinition{
		AlarmName:  "TestAlarm",
		MetricName: "TestMetric",
		Namespace:  "TestNamespace",
	})

	if len(alarmsMock.GetAllAlarms()) == 0 {
		t.Error("Expected alarms before reset")
	}

	alarmsMock.Reset()

	if len(alarmsMock.GetAllAlarms()) != 0 {
		t.Error("Expected no alarms after reset")
	}
	if alarmsMock.GetCallCount("PutMetricAlarm") != 0 {
		t.Error("Expected call count to be reset")
	}
}

// TestMockAPIGatewayError tests the MockAPIGatewayError implementation
func TestMockAPIGatewayError(t *testing.T) {
	err := &MockAPIGatewayError{
		Code:       "GoneException",
		Message:    "Connection not found",
		StatusCode: 410,
		Retryable:  false,
	}

	if err.Error() != "GoneException: Connection not found" {
		t.Errorf("Unexpected error string: %s", err.Error())
	}
	if err.HTTPStatusCode() != 410 {
		t.Errorf("Expected status code 410, got %d", err.HTTPStatusCode())
	}
	if err.ErrorCode() != "GoneException" {
		t.Errorf("Expected error code GoneException, got %s", err.ErrorCode())
	}
	if err.IsRetryable() != false {
		t.Errorf("Expected retryable false, got %t", err.IsRetryable())
	}
}
