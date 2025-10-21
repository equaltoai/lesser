package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pay-theory/lift/pkg/testing"
)

func main() {
	fmt.Println("🚀 Lift Mocking Demo - Extended AWS Services Coverage")
	fmt.Println(strings.Repeat("=", 60))

	// Demo API Gateway Management API Mock
	demoAPIGatewayMock()

	fmt.Println()

	// Demo CloudWatch Metrics Mock
	demoCloudWatchMetricsMock()

	fmt.Println()

	// Demo CloudWatch Alarms Mock
	demoCloudWatchAlarmsMock()

	fmt.Println()

	// Demo Integration Testing
	demoIntegrationTesting()
}

func demoAPIGatewayMock() {
	fmt.Println("📡 API Gateway Management API Mock Demo")
	fmt.Println(strings.Repeat("-", 40))

	ctx := context.Background()
	mock := testing.NewMockAPIGatewayManagementClient()

	// Configure mock behavior
	config := testing.DefaultMockAPIGatewayConfig()
	config.NetworkDelay = 10 * time.Millisecond // Simulate network latency
	config.MaxMessageSize = 1024                // 1KB limit
	mock.WithConfig(config)

	// Add some test connections
	mock.WithConnection("user-123", &testing.MockConnection{
		ID:           "user-123",
		State:        testing.ConnectionStateActive,
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
		SourceIP:     "192.168.1.100",
		UserAgent:    "PayTheory-WebSocket-Client/1.0",
		Metadata: map[string]any{
			"userId":   "user-123",
			"tenantId": "tenant-abc",
			"role":     "customer",
		},
	})

	mock.WithConnection("admin-456", nil) // Will use defaults

	// Test successful message sending
	message := []byte(`{"type": "notification", "data": {"message": "Payment processed successfully"}}`)
	err := mock.PostToConnection(ctx, "user-123", message)
	if err != nil {
		log.Printf("❌ Error sending message: %v", err)
	} else {
		fmt.Printf("✅ Message sent successfully to user-123\n")
	}

	// Test connection info retrieval
	connInfo, err := mock.GetConnection(ctx, "user-123")
	if err != nil {
		log.Printf("❌ Error getting connection info: %v", err)
	} else {
		fmt.Printf("📋 Connection Info: ID=%s, Connected=%s, IP=%s\n",
			connInfo.ConnectionID, connInfo.ConnectedAt, connInfo.SourceIP)
	}

	// Test error scenarios
	err = mock.PostToConnection(ctx, "non-existent", message)
	if err != nil {
		fmt.Printf("⚠️  Expected error for non-existent connection: %s\n", err.Error())
	}

	// Test oversized message
	largeMessage := make([]byte, 2048) // Exceeds 1KB limit
	err = mock.PostToConnection(ctx, "user-123", largeMessage)
	if err != nil {
		fmt.Printf("⚠️  Expected error for oversized message: %s\n", err.Error())
	}

	// Show statistics
	fmt.Printf("📊 Statistics:\n")
	fmt.Printf("   - PostToConnection calls: %d\n", mock.GetCallCount("PostToConnection"))
	fmt.Printf("   - GetConnection calls: %d\n", mock.GetCallCount("GetConnection"))
	fmt.Printf("   - Active connections: %d\n", len(mock.GetActiveConnections()))
	fmt.Printf("   - Messages sent to user-123: %d\n", mock.GetMessageCount("user-123"))
}

func demoCloudWatchMetricsMock() {
	fmt.Println("📈 CloudWatch Metrics Mock Demo")
	fmt.Println(strings.Repeat("-", 40))

	ctx := context.Background()
	mock := testing.NewMockCloudWatchMetricsClient()

	// Configure mock behavior
	config := testing.DefaultMockCloudWatchConfig()
	config.NetworkDelay = 5 * time.Millisecond
	mock.WithConfig(config)

	// Publish some metrics
	metrics := []*testing.MockMetricDatum{
		{
			MetricName: "WebSocketConnections",
			Value:      25.0,
			Unit:       testing.MetricUnitCount,
			Dimensions: map[string]string{
				"Service":     "Streamer",
				"Environment": "production",
			},
		},
		{
			MetricName: "MessageLatency",
			Value:      45.2,
			Unit:       testing.MetricUnitMilliseconds,
			Dimensions: map[string]string{
				"Service":     "Streamer",
				"Environment": "production",
			},
		},
		{
			MetricName: "ErrorRate",
			Value:      0.02,
			Unit:       testing.MetricUnitPercent,
			Dimensions: map[string]string{
				"Service":     "Streamer",
				"Environment": "production",
			},
		},
	}

	err := mock.PutMetricData(ctx, "PayTheory/Streamer", metrics)
	if err != nil {
		log.Printf("❌ Error publishing metrics: %v", err)
	} else {
		fmt.Printf("✅ Published %d metrics to PayTheory/Streamer namespace\n", len(metrics))
	}

	// Add more data points for statistics
	time.Sleep(10 * time.Millisecond) // Simulate time passage
	additionalMetrics := []*testing.MockMetricDatum{
		{
			MetricName: "WebSocketConnections",
			Value:      30.0,
			Unit:       testing.MetricUnitCount,
			Dimensions: map[string]string{
				"Service":     "Streamer",
				"Environment": "production",
			},
		},
		{
			MetricName: "MessageLatency",
			Value:      52.8,
			Unit:       testing.MetricUnitMilliseconds,
			Dimensions: map[string]string{
				"Service":     "Streamer",
				"Environment": "production",
			},
		},
	}

	err = mock.PutMetricData(ctx, "PayTheory/Streamer", additionalMetrics)
	if err != nil {
		log.Printf("❌ Error publishing additional metrics: %v", err)
	}

	// Retrieve statistics
	now := time.Now()
	startTime := now.Add(-1 * time.Hour)
	endTime := now.Add(1 * time.Hour)

	stats, err := mock.GetMetricStatistics(
		ctx,
		"PayTheory/Streamer",
		"WebSocketConnections",
		map[string]string{
			"Service":     "Streamer",
			"Environment": "production",
		},
		startTime,
		endTime,
		300, // 5 minutes
		[]testing.Statistic{
			testing.StatisticSum,
			testing.StatisticAverage,
			testing.StatisticMaximum,
			testing.StatisticMinimum,
			testing.StatisticSampleCount,
		},
	)

	if err != nil {
		log.Printf("❌ Error retrieving statistics: %v", err)
	} else {
		fmt.Printf("📊 WebSocketConnections Statistics:\n")
		fmt.Printf("   - Sum: %.1f\n", stats[testing.StatisticSum])
		fmt.Printf("   - Average: %.1f\n", stats[testing.StatisticAverage])
		fmt.Printf("   - Maximum: %.1f\n", stats[testing.StatisticMaximum])
		fmt.Printf("   - Minimum: %.1f\n", stats[testing.StatisticMinimum])
		fmt.Printf("   - Sample Count: %.0f\n", stats[testing.StatisticSampleCount])
	}

	// Show all metrics
	allMetrics := mock.GetAllMetrics()
	fmt.Printf("📋 Total metrics stored: %d namespaces\n", len(allMetrics))
	for namespace, namespaceMetrics := range allMetrics {
		fmt.Printf("   - %s: %d metrics\n", namespace, len(namespaceMetrics))
	}
}

func demoCloudWatchAlarmsMock() {
	fmt.Println("🚨 CloudWatch Alarms Mock Demo")
	fmt.Println(strings.Repeat("-", 40))

	ctx := context.Background()
	alarmsMock := testing.NewMockCloudWatchAlarmsClient()
	metricsMock := testing.NewMockCloudWatchMetricsClient()

	// Link alarms with metrics for evaluation
	alarmsMock.WithMetricsClient(metricsMock)

	// Create alarms
	alarms := []*testing.MockAlarmDefinition{
		{
			AlarmName:          "HighConnectionCount",
			AlarmDescription:   "Alert when WebSocket connections exceed threshold",
			MetricName:         "WebSocketConnections",
			Namespace:          "PayTheory/Streamer",
			Statistic:          testing.StatisticAverage,
			Dimensions:         map[string]string{"Service": "Streamer"},
			Period:             300, // 5 minutes
			EvaluationPeriods:  1,
			Threshold:          50.0,
			ComparisonOperator: testing.ComparisonGreaterThanThreshold,
			TreatMissingData:   "notBreaching",
		},
		{
			AlarmName:          "HighLatency",
			AlarmDescription:   "Alert when message latency is too high",
			MetricName:         "MessageLatency",
			Namespace:          "PayTheory/Streamer",
			Statistic:          testing.StatisticAverage,
			Dimensions:         map[string]string{"Service": "Streamer"},
			Period:             300,
			EvaluationPeriods:  2,
			Threshold:          100.0,
			ComparisonOperator: testing.ComparisonGreaterThanThreshold,
			TreatMissingData:   "breaching",
		},
	}

	for _, alarm := range alarms {
		err := alarmsMock.PutMetricAlarm(ctx, alarm)
		if err != nil {
			log.Printf("❌ Error creating alarm %s: %v", alarm.AlarmName, err)
		} else {
			fmt.Printf("✅ Created alarm: %s\n", alarm.AlarmName)
		}
	}

	// Publish metrics that should trigger alarms
	triggerMetrics := []*testing.MockMetricDatum{
		{
			MetricName: "WebSocketConnections",
			Value:      75.0, // Above threshold of 50
			Unit:       testing.MetricUnitCount,
			Dimensions: map[string]string{"Service": "Streamer"},
		},
		{
			MetricName: "MessageLatency",
			Value:      150.0, // Above threshold of 100
			Unit:       testing.MetricUnitMilliseconds,
			Dimensions: map[string]string{"Service": "Streamer"},
		},
	}

	err := metricsMock.PutMetricData(ctx, "PayTheory/Streamer", triggerMetrics)
	if err != nil {
		log.Printf("❌ Error publishing trigger metrics: %v", err)
	}

	// Evaluate alarms
	err = alarmsMock.EvaluateAlarms(ctx)
	if err != nil {
		log.Printf("❌ Error evaluating alarms: %v", err)
	} else {
		fmt.Printf("🔍 Evaluated all alarms\n")
	}

	// Check alarm states
	allAlarms := alarmsMock.GetAllAlarms()
	fmt.Printf("📊 Alarm States:\n")
	for name, alarm := range allAlarms {
		fmt.Printf("   - %s: %s (%s)\n", name, alarm.State, alarm.StateReason)
	}

	// Describe specific alarms
	describedAlarms, err := alarmsMock.DescribeAlarms(ctx, []string{"HighConnectionCount"})
	if err != nil {
		log.Printf("❌ Error describing alarms: %v", err)
	} else {
		fmt.Printf("📋 Alarm Details:\n")
		for _, alarm := range describedAlarms {
			fmt.Printf("   - %s: Threshold=%.1f, Current State=%s\n",
				alarm.AlarmName, alarm.Threshold, alarm.State)
		}
	}
}

func demoIntegrationTesting() {
	fmt.Println("🔗 Integration Testing Demo")
	fmt.Println(strings.Repeat("-", 40))

	ctx := context.Background()

	// Create integrated mock environment
	apiMock := testing.NewMockAPIGatewayManagementClient()
	metricsMock := testing.NewMockCloudWatchMetricsClient()
	alarmsMock := testing.NewMockCloudWatchAlarmsClient()

	// Link components
	alarmsMock.WithMetricsClient(metricsMock)

	// Set up monitoring
	connectionAlarm := &testing.MockAlarmDefinition{
		AlarmName:          "ConnectionFailures",
		MetricName:         "FailedConnections",
		Namespace:          "PayTheory/Streamer",
		Statistic:          testing.StatisticSum,
		Period:             60,
		EvaluationPeriods:  1,
		Threshold:          5.0,
		ComparisonOperator: testing.ComparisonGreaterThanThreshold,
	}
	if err := alarmsMock.PutMetricAlarm(ctx, connectionAlarm); err != nil {
		log.Printf("Warning: failed to put metric alarm: %v", err)
	}

	// Simulate application behavior
	fmt.Printf("🎭 Simulating application behavior...\n")

	// Add connections
	for i := 1; i <= 10; i++ {
		connID := fmt.Sprintf("user-%d", i)
		apiMock.WithConnection(connID, nil)
	}

	// Simulate message sending with some failures
	successCount := 0
	failureCount := 0

	for i := 1; i <= 15; i++ {
		connID := fmt.Sprintf("user-%d", i)
		message := []byte(fmt.Sprintf(`{"id": %d, "message": "Hello user %d"}`, i, i))

		err := apiMock.PostToConnection(ctx, connID, message)
		if err != nil {
			failureCount++
		} else {
			successCount++
		}
	}

	// Publish metrics based on simulation
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

	if err := metricsMock.PutMetricData(ctx, "PayTheory/Streamer", metrics); err != nil {
		log.Printf("Warning: failed to put metric data: %v", err)
	}

	// Evaluate monitoring
	if err := alarmsMock.EvaluateAlarms(ctx); err != nil {
		log.Printf("Warning: failed to evaluate alarms: %v", err)
	}

	// Report results
	fmt.Printf("📊 Simulation Results:\n")
	fmt.Printf("   - Successful connections: %d\n", successCount)
	fmt.Printf("   - Failed connections: %d\n", failureCount)
	fmt.Printf("   - Active connections: %d\n", len(apiMock.GetActiveConnections()))

	alarm := alarmsMock.GetAlarm("ConnectionFailures")
	if alarm != nil {
		fmt.Printf("   - Connection failures alarm: %s\n", alarm.State)
		if alarm.State == testing.AlarmStateAlarm {
			fmt.Printf("     ⚠️  Alert: High connection failure rate detected!\n")
		}
	}

	fmt.Printf("✅ Integration testing completed successfully\n")
}
