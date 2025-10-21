package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/mock"

	lifttesting "github.com/pay-theory/lift/pkg/testing"
)

// Example infrastructure code that uses CloudWatch
type MetricsPublisher struct {
	client CloudWatchClient
}

// MetricsPublisher is responsible for publishing metrics to AWS CloudWatch.
// It provides methods to put metric data, get metric statistics, and manage alarms.

// CloudWatchClient interface (what your infrastructure code expects)
type CloudWatchClient interface {
	PutMetricData(ctx context.Context, input *cloudwatch.PutMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error)
	GetMetricStatistics(ctx context.Context, input *cloudwatch.GetMetricStatisticsInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error)
	PutMetricAlarm(ctx context.Context, input *cloudwatch.PutMetricAlarmInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricAlarmOutput, error)
	DescribeAlarms(ctx context.Context, input *cloudwatch.DescribeAlarmsInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAlarmsOutput, error)
}

// CloudWatchClient defines the interface for interacting with AWS CloudWatch.
// It includes methods for putting metric data, getting metric statistics,
// putting metric alarms, and describing alarms.

func NewMetricsPublisher(client CloudWatchClient) *MetricsPublisher {
	return &MetricsPublisher{client: client}
}

// PublishRequestMetrics publishes request count and latency metrics
func (m *MetricsPublisher) PublishRequestMetrics(ctx context.Context, namespace string, requestCount int, avgLatency float64) error {
	input := &cloudwatch.PutMetricDataInput{
		Namespace: aws.String(namespace),
		MetricData: []types.MetricDatum{
			{
				MetricName: aws.String("RequestCount"),
				Value:      aws.Float64(float64(requestCount)),
				Unit:       types.StandardUnitCount,
				Timestamp:  aws.Time(time.Now()),
			},
			{
				MetricName: aws.String("AverageLatency"),
				Value:      aws.Float64(avgLatency),
				Unit:       types.StandardUnitMilliseconds,
				Timestamp:  aws.Time(time.Now()),
			},
		},
	}

	_, err := m.client.PutMetricData(ctx, input)
	return err
}

// GetRequestStats retrieves request statistics
func (m *MetricsPublisher) GetRequestStats(ctx context.Context, namespace string, startTime, endTime time.Time) (float64, error) {
	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String(namespace),
		MetricName: aws.String("RequestCount"),
		StartTime:  aws.Time(startTime),
		EndTime:    aws.Time(endTime),
		Period:     aws.Int32(300),
		Statistics: []types.Statistic{types.StatisticSum},
	}

	output, err := m.client.GetMetricStatistics(ctx, input)
	if err != nil {
		return 0, err
	}

	if len(output.Datapoints) == 0 {
		return 0, nil
	}

	// Return the sum of all datapoints
	var total float64
	for _, datapoint := range output.Datapoints {
		if datapoint.Sum != nil {
			total += *datapoint.Sum
		}
	}

	return total, nil
}

// CreateHighLatencyAlarm creates an alarm for high latency
func (m *MetricsPublisher) CreateHighLatencyAlarm(ctx context.Context, namespace string, threshold float64) error {
	input := &cloudwatch.PutMetricAlarmInput{
		AlarmName:          aws.String("HighLatencyAlarm"),
		AlarmDescription:   aws.String("Alarm when average latency exceeds threshold"),
		MetricName:         aws.String("AverageLatency"),
		Namespace:          aws.String(namespace),
		Statistic:          types.StatisticAverage,
		Period:             aws.Int32(300),
		EvaluationPeriods:  aws.Int32(2),
		Threshold:          aws.Float64(threshold),
		ComparisonOperator: types.ComparisonOperatorGreaterThanThreshold,
		TreatMissingData:   aws.String("notBreaching"),
	}

	_, err := m.client.PutMetricAlarm(ctx, input)
	return err
}

// CheckAlarmStatus checks the status of alarms
func (m *MetricsPublisher) CheckAlarmStatus(ctx context.Context, alarmNames []string) ([]types.MetricAlarm, error) {
	input := &cloudwatch.DescribeAlarmsInput{
		AlarmNames: alarmNames,
	}

	output, err := m.client.DescribeAlarms(ctx, input)
	if err != nil {
		return nil, err
	}

	return output.MetricAlarms, nil
}

func main() {
	fmt.Println("=== CloudWatch Mocking Demo ===")
	fmt.Println()

	// Demonstrate testing with mocks
	demonstrateMetricsPublishing()
	fmt.Println()
	demonstrateMetricsRetrieval()
	fmt.Println()
	demonstrateAlarmManagement()
	fmt.Println()
	demonstrateAdvancedUsage()
}

func demonstrateMetricsPublishing() {
	fmt.Println("1. Testing Metrics Publishing")
	fmt.Println("-----------------------------")

	// Create mock client (like DynamORM pattern)
	mockClient := lifttesting.NewMockCloudWatchClient()
	publisher := NewMetricsPublisher(mockClient)
	ctx := context.Background()

	// Setup expectation
	mockClient.On("PutMetricData",
		ctx,
		mock.MatchedBy(func(input *cloudwatch.PutMetricDataInput) bool {
			return *input.Namespace == "MyApp/API" && len(input.MetricData) == 2
		}),
		mock.Anything,
	).Return(lifttesting.NewMockPutMetricDataOutput(), nil)

	// Test the infrastructure code
	err := publisher.PublishRequestMetrics(ctx, "MyApp/API", 100, 250.5)
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		fmt.Println("✅ Successfully published metrics")
	}

	// Verify expectations
	mockClient.AssertExpectations(&mockTesting{})
	fmt.Println("✅ All expectations met")
}

func demonstrateMetricsRetrieval() {
	fmt.Println("2. Testing Metrics Retrieval")
	fmt.Println("-----------------------------")

	// Create mock client
	mockClient := lifttesting.NewMockCloudWatchClient()
	publisher := NewMetricsPublisher(mockClient)
	ctx := context.Background()

	// Setup mock response with data
	datapoints := []types.Datapoint{
		lifttesting.NewMockDatapoint(150.0, types.StandardUnitCount),
		lifttesting.NewMockDatapoint(200.0, types.StandardUnitCount),
	}
	expectedOutput := lifttesting.NewMockGetMetricStatisticsOutput(datapoints)
	// Set Sum values for the datapoints
	expectedOutput.Datapoints[0].Sum = aws.Float64(150.0)
	expectedOutput.Datapoints[1].Sum = aws.Float64(200.0)

	mockClient.On("GetMetricStatistics",
		ctx,
		mock.AnythingOfType("*cloudwatch.GetMetricStatisticsInput"),
		mock.Anything,
	).Return(expectedOutput, nil)

	// Test the infrastructure code
	startTime := time.Now().Add(-1 * time.Hour)
	endTime := time.Now()
	total, err := publisher.GetRequestStats(ctx, "MyApp/API", startTime, endTime)
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		fmt.Printf("✅ Total requests: %.0f\n", total)
	}

	// Verify expectations
	mockClient.AssertExpectations(&mockTesting{})
	fmt.Println("✅ All expectations met")
}

func demonstrateAlarmManagement() {
	fmt.Println("3. Testing Alarm Management")
	fmt.Println("---------------------------")

	// Create mock client
	mockClient := lifttesting.NewMockCloudWatchClient()
	publisher := NewMetricsPublisher(mockClient)
	ctx := context.Background()

	// Setup expectations for alarm creation
	mockClient.On("PutMetricAlarm",
		ctx,
		mock.MatchedBy(func(input *cloudwatch.PutMetricAlarmInput) bool {
			return *input.AlarmName == "HighLatencyAlarm" && *input.Threshold == 500.0
		}),
		mock.Anything,
	).Return(lifttesting.NewMockPutMetricAlarmOutput(), nil)

	// Setup expectations for alarm status check
	alarms := []types.MetricAlarm{
		lifttesting.NewMockMetricAlarm("HighLatencyAlarm", "AverageLatency", 500.0),
	}
	mockClient.On("DescribeAlarms",
		ctx,
		mock.AnythingOfType("*cloudwatch.DescribeAlarmsInput"),
		mock.Anything,
	).Return(lifttesting.NewMockDescribeAlarmsOutput(alarms), nil)

	// Test alarm creation
	err := publisher.CreateHighLatencyAlarm(ctx, "MyApp/API", 500.0)
	if err != nil {
		log.Printf("Error creating alarm: %v", err)
	} else {
		fmt.Println("✅ Successfully created high latency alarm")
	}

	// Test alarm status check
	alarmStatus, err := publisher.CheckAlarmStatus(ctx, []string{"HighLatencyAlarm"})
	if err != nil {
		log.Printf("Error checking alarm status: %v", err)
	} else {
		fmt.Printf("✅ Found %d alarm(s)\n", len(alarmStatus))
		for _, alarm := range alarmStatus {
			fmt.Printf("   - %s: threshold=%.1f, state=%s\n",
				*alarm.AlarmName, *alarm.Threshold, alarm.StateValue)
		}
	}

	// Verify expectations
	mockClient.AssertExpectations(&mockTesting{})
	fmt.Println("✅ All expectations met")
}

func demonstrateAdvancedUsage() {
	fmt.Println("4. Advanced Usage Patterns")
	fmt.Println("--------------------------")

	// Create mock client
	mockClient := lifttesting.NewMockCloudWatchClient()
	ctx := context.Background()

	// Demonstrate error simulation
	fmt.Println("Testing error scenarios:")
	mockClient.On("PutMetricData",
		ctx,
		mock.AnythingOfType("*cloudwatch.PutMetricDataInput"),
		mock.Anything,
	).Return((*cloudwatch.PutMetricDataOutput)(nil), fmt.Errorf("throttling error"))

	publisher := NewMetricsPublisher(mockClient)
	err := publisher.PublishRequestMetrics(ctx, "MyApp/API", 100, 250.5)
	if err != nil {
		fmt.Printf("✅ Successfully simulated error: %v\n", err)
	}

	// Demonstrate conditional matching
	fmt.Println("Testing conditional matching:")
	mockClient2 := lifttesting.NewMockCloudWatchClient()
	publisher2 := NewMetricsPublisher(mockClient2)

	mockClient2.On("PutMetricData",
		ctx,
		mock.MatchedBy(func(input *cloudwatch.PutMetricDataInput) bool {
			// Only match if namespace contains "Production"
			return *input.Namespace == "MyApp/Production"
		}),
		mock.Anything,
	).Return(lifttesting.NewMockPutMetricDataOutput(), nil)

	// This should work
	err = publisher2.PublishRequestMetrics(ctx, "MyApp/Production", 100, 250.5)
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		fmt.Println("✅ Production metrics published successfully")
	}

	mockClient2.AssertExpectations(&mockTesting{})
	fmt.Println("✅ Conditional matching worked correctly")
}

// mockTesting implements testify's TestingT interface for demonstration
type mockTesting struct{}

// mockTesting implements the testify TestingT interface for demonstration purposes.
// It is used to mock testing functionality in the absence of a real testing framework.

func (m *mockTesting) Errorf(format string, args ...any) {
	// Errorf logs an error message with the given format and arguments.
	// This is a mock implementation for demonstration purposes.
	fmt.Printf("MOCK ERROR: "+format+"\n", args...)
}

func (m *mockTesting) Logf(format string, args ...any) {
	fmt.Printf("MOCK LOG: "+format+"\n", args...)
}

func (m *mockTesting) FailNow() {
	fmt.Println("MOCK FAIL: Test failed")
}
