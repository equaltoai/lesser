package testing

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/mock"
)

// =============================================================================
// AWS SDK-Compatible CloudWatch Client Mock
// =============================================================================

// MockCloudWatchClient provides a testify-based mock of the AWS CloudWatch client
// that implements the exact AWS SDK interface, following the DynamORM pattern.
type MockCloudWatchClient struct {
	mock.Mock
}

// NewMockCloudWatchClient creates a new mock CloudWatch client
func NewMockCloudWatchClient() *MockCloudWatchClient {
	return &MockCloudWatchClient{}
}

// PutMetricData publishes metric data to CloudWatch
func (m *MockCloudWatchClient) PutMetricData(
	ctx context.Context,
	input *cloudwatch.PutMetricDataInput,
	optFns ...func(*cloudwatch.Options),
) (*cloudwatch.PutMetricDataOutput, error) {
	args := m.Called(ctx, input, optFns)
	v := args.Get(0)
	if v != nil {
		if output, ok := v.(*cloudwatch.PutMetricDataOutput); ok {
			return output, args.Error(1)
		}
		return nil, fmt.Errorf("unexpected type in mock return value")
	}
	return nil, args.Error(1)
}

// GetMetricStatistics retrieves statistics for a metric
func (m *MockCloudWatchClient) GetMetricStatistics(
	ctx context.Context,
	input *cloudwatch.GetMetricStatisticsInput,
	optFns ...func(*cloudwatch.Options),
) (*cloudwatch.GetMetricStatisticsOutput, error) {
	args := m.Called(ctx, input, optFns)
	v := args.Get(0)
	if v != nil {
		if output, ok := v.(*cloudwatch.GetMetricStatisticsOutput); ok {
			return output, args.Error(1)
		}
		return nil, fmt.Errorf("unexpected type in mock return value")
	}
	return nil, args.Error(1)
}

// PutMetricAlarm creates or updates an alarm
func (m *MockCloudWatchClient) PutMetricAlarm(
	ctx context.Context,
	input *cloudwatch.PutMetricAlarmInput,
	optFns ...func(*cloudwatch.Options),
) (*cloudwatch.PutMetricAlarmOutput, error) {
	args := m.Called(ctx, input, optFns)
	v := args.Get(0)
	if v != nil {
		if output, ok := v.(*cloudwatch.PutMetricAlarmOutput); ok {
			return output, args.Error(1)
		}
		return nil, fmt.Errorf("unexpected type in mock return value")
	}
	return nil, args.Error(1)
}

// DescribeAlarms retrieves alarm information
func (m *MockCloudWatchClient) DescribeAlarms(
	ctx context.Context,
	input *cloudwatch.DescribeAlarmsInput,
	optFns ...func(*cloudwatch.Options),
) (*cloudwatch.DescribeAlarmsOutput, error) {
	args := m.Called(ctx, input, optFns)
	v := args.Get(0)
	if v != nil {
		if output, ok := v.(*cloudwatch.DescribeAlarmsOutput); ok {
			return output, args.Error(1)
		}
		return nil, fmt.Errorf("unexpected type in mock return value")
	}
	return nil, args.Error(1)
}

// DeleteAlarms deletes one or more alarms
func (m *MockCloudWatchClient) DeleteAlarms(
	ctx context.Context,
	input *cloudwatch.DeleteAlarmsInput,
	optFns ...func(*cloudwatch.Options),
) (*cloudwatch.DeleteAlarmsOutput, error) {
	args := m.Called(ctx, input, optFns)
	v := args.Get(0)
	if v != nil {
		if output, ok := v.(*cloudwatch.DeleteAlarmsOutput); ok {
			return output, args.Error(1)
		}
		return nil, fmt.Errorf("unexpected type in mock return value")
	}
	return nil, args.Error(1)
}

// ListMetrics lists available metrics
func (m *MockCloudWatchClient) ListMetrics(
	ctx context.Context,
	input *cloudwatch.ListMetricsInput,
	optFns ...func(*cloudwatch.Options),
) (*cloudwatch.ListMetricsOutput, error) {
	args := m.Called(ctx, input, optFns)
	v := args.Get(0)
	if v != nil {
		if output, ok := v.(*cloudwatch.ListMetricsOutput); ok {
			return output, args.Error(1)
		}
		return nil, fmt.Errorf("unexpected type in mock return value")
	}
	return nil, args.Error(1)
}

// GetMetricData retrieves metric data using metric queries
func (m *MockCloudWatchClient) GetMetricData(
	ctx context.Context,
	input *cloudwatch.GetMetricDataInput,
	optFns ...func(*cloudwatch.Options),
) (*cloudwatch.GetMetricDataOutput, error) {
	args := m.Called(ctx, input, optFns)
	v := args.Get(0)
	if v != nil {
		if output, ok := v.(*cloudwatch.GetMetricDataOutput); ok {
			return output, args.Error(1)
		}
		return nil, fmt.Errorf("unexpected type in mock return value")
	}
	return nil, args.Error(1)
}

// PutAnomalyDetector creates or updates an anomaly detector
func (m *MockCloudWatchClient) PutAnomalyDetector(
	ctx context.Context,
	input *cloudwatch.PutAnomalyDetectorInput,
	optFns ...func(*cloudwatch.Options),
) (*cloudwatch.PutAnomalyDetectorOutput, error) {
	args := m.Called(ctx, input, optFns)
	v := args.Get(0)
	if v != nil {
		if output, ok := v.(*cloudwatch.PutAnomalyDetectorOutput); ok {
			return output, args.Error(1)
		}
		return nil, fmt.Errorf("unexpected type in mock return value")
	}
	return nil, args.Error(1)
}

// DescribeAnomalyDetectors retrieves anomaly detector information
func (m *MockCloudWatchClient) DescribeAnomalyDetectors(
	ctx context.Context,
	input *cloudwatch.DescribeAnomalyDetectorsInput,
	optFns ...func(*cloudwatch.Options),
) (*cloudwatch.DescribeAnomalyDetectorsOutput, error) {
	args := m.Called(ctx, input, optFns)
	v := args.Get(0)
	if v != nil {
		if output, ok := v.(*cloudwatch.DescribeAnomalyDetectorsOutput); ok {
			return output, args.Error(1)
		}
		return nil, fmt.Errorf("unexpected type in mock return value")
	}
	return nil, args.Error(1)
}

// DeleteAnomalyDetector deletes an anomaly detector
func (m *MockCloudWatchClient) DeleteAnomalyDetector(
	ctx context.Context,
	input *cloudwatch.DeleteAnomalyDetectorInput,
	optFns ...func(*cloudwatch.Options),
) (*cloudwatch.DeleteAnomalyDetectorOutput, error) {
	args := m.Called(ctx, input, optFns)
	v := args.Get(0)
	if v != nil {
		if output, ok := v.(*cloudwatch.DeleteAnomalyDetectorOutput); ok {
			return output, args.Error(1)
		}
		return nil, fmt.Errorf("unexpected type in mock return value")
	}
	return nil, args.Error(1)
}

// TagResource adds tags to a CloudWatch resource
func (m *MockCloudWatchClient) TagResource(
	ctx context.Context,
	input *cloudwatch.TagResourceInput,
	optFns ...func(*cloudwatch.Options),
) (*cloudwatch.TagResourceOutput, error) {
	args := m.Called(ctx, input, optFns)
	v := args.Get(0)
	if v != nil {
		if output, ok := v.(*cloudwatch.TagResourceOutput); ok {
			return output, args.Error(1)
		}
		return nil, fmt.Errorf("unexpected type in mock return value")
	}
	return nil, args.Error(1)
}

// UntagResource removes tags from a CloudWatch resource
func (m *MockCloudWatchClient) UntagResource(
	ctx context.Context,
	input *cloudwatch.UntagResourceInput,
	optFns ...func(*cloudwatch.Options),
) (*cloudwatch.UntagResourceOutput, error) {
	args := m.Called(ctx, input, optFns)
	v := args.Get(0)
	if v != nil {
		if output, ok := v.(*cloudwatch.UntagResourceOutput); ok {
			return output, args.Error(1)
		}
		return nil, fmt.Errorf("unexpected type in mock return value")
	}
	return nil, args.Error(1)
}

// ListTagsForResource lists tags for a CloudWatch resource
func (m *MockCloudWatchClient) ListTagsForResource(
	ctx context.Context,
	input *cloudwatch.ListTagsForResourceInput,
	optFns ...func(*cloudwatch.Options),
) (*cloudwatch.ListTagsForResourceOutput, error) {
	args := m.Called(ctx, input, optFns)
	v := args.Get(0)
	if v != nil {
		if output, ok := v.(*cloudwatch.ListTagsForResourceOutput); ok {
			return output, args.Error(1)
		}
		return nil, fmt.Errorf("unexpected type in mock return value")
	}
	return nil, args.Error(1)
}

// =============================================================================
// Helper Functions for Output Creation (Like DynamORM Pattern)
// =============================================================================

// NewMockPutMetricDataOutput creates a mock PutMetricDataOutput
func NewMockPutMetricDataOutput() *cloudwatch.PutMetricDataOutput {
	return &cloudwatch.PutMetricDataOutput{}
}

// NewMockGetMetricStatisticsOutput creates a mock GetMetricStatisticsOutput with datapoints
func NewMockGetMetricStatisticsOutput(datapoints []types.Datapoint) *cloudwatch.GetMetricStatisticsOutput {
	return &cloudwatch.GetMetricStatisticsOutput{
		Datapoints: datapoints,
		Label:      aws.String("MockMetric"),
	}
}

// NewMockPutMetricAlarmOutput creates a mock PutMetricAlarmOutput
func NewMockPutMetricAlarmOutput() *cloudwatch.PutMetricAlarmOutput {
	return &cloudwatch.PutMetricAlarmOutput{}
}

// NewMockDescribeAlarmsOutput creates a mock DescribeAlarmsOutput with alarms
func NewMockDescribeAlarmsOutput(alarms []types.MetricAlarm) *cloudwatch.DescribeAlarmsOutput {
	return &cloudwatch.DescribeAlarmsOutput{
		MetricAlarms: alarms,
	}
}

// NewMockDeleteAlarmsOutput creates a mock DeleteAlarmsOutput
func NewMockDeleteAlarmsOutput() *cloudwatch.DeleteAlarmsOutput {
	return &cloudwatch.DeleteAlarmsOutput{}
}

// NewMockListMetricsOutput creates a mock ListMetricsOutput with metrics
func NewMockListMetricsOutput(metrics []types.Metric) *cloudwatch.ListMetricsOutput {
	return &cloudwatch.ListMetricsOutput{
		Metrics: metrics,
	}
}

// NewMockGetMetricDataOutput creates a mock GetMetricDataOutput with results
func NewMockGetMetricDataOutput(results []types.MetricDataResult) *cloudwatch.GetMetricDataOutput {
	return &cloudwatch.GetMetricDataOutput{
		MetricDataResults: results,
	}
}

// NewMockPutAnomalyDetectorOutput creates a mock PutAnomalyDetectorOutput
func NewMockPutAnomalyDetectorOutput() *cloudwatch.PutAnomalyDetectorOutput {
	return &cloudwatch.PutAnomalyDetectorOutput{}
}

// NewMockDescribeAnomalyDetectorsOutput creates a mock DescribeAnomalyDetectorsOutput
func NewMockDescribeAnomalyDetectorsOutput(detectors []types.AnomalyDetector) *cloudwatch.DescribeAnomalyDetectorsOutput {
	return &cloudwatch.DescribeAnomalyDetectorsOutput{
		AnomalyDetectors: detectors,
	}
}

// NewMockDeleteAnomalyDetectorOutput creates a mock DeleteAnomalyDetectorOutput
func NewMockDeleteAnomalyDetectorOutput() *cloudwatch.DeleteAnomalyDetectorOutput {
	return &cloudwatch.DeleteAnomalyDetectorOutput{}
}

// NewMockTagResourceOutput creates a mock TagResourceOutput
func NewMockTagResourceOutput() *cloudwatch.TagResourceOutput {
	return &cloudwatch.TagResourceOutput{}
}

// NewMockUntagResourceOutput creates a mock UntagResourceOutput
func NewMockUntagResourceOutput() *cloudwatch.UntagResourceOutput {
	return &cloudwatch.UntagResourceOutput{}
}

// NewMockListTagsForResourceOutput creates a mock ListTagsForResourceOutput
func NewMockListTagsForResourceOutput(tags []types.Tag) *cloudwatch.ListTagsForResourceOutput {
	return &cloudwatch.ListTagsForResourceOutput{
		Tags: tags,
	}
}

// =============================================================================
// Helper Functions for Input Creation
// =============================================================================

// NewMockMetricDatum creates a mock MetricDatum for testing
func NewMockMetricDatum(name string, value float64, unit types.StandardUnit) types.MetricDatum {
	return types.MetricDatum{
		MetricName: aws.String(name),
		Value:      aws.Float64(value),
		Unit:       unit,
	}
}

// NewMockDatapoint creates a mock Datapoint for testing
func NewMockDatapoint(value float64, unit types.StandardUnit) types.Datapoint {
	return types.Datapoint{
		Average:   aws.Float64(value),
		Unit:      unit,
		Timestamp: aws.Time(time.Unix(1640995200, 0)), // 2022-01-01 00:00:00 UTC
	}
}

// NewMockMetricAlarm creates a mock MetricAlarm for testing
func NewMockMetricAlarm(name, metricName string, threshold float64) types.MetricAlarm {
	return types.MetricAlarm{
		AlarmName:          aws.String(name),
		MetricName:         aws.String(metricName),
		Threshold:          aws.Float64(threshold),
		ComparisonOperator: types.ComparisonOperatorGreaterThanThreshold,
		Statistic:          types.StatisticAverage,
		Period:             aws.Int32(300),
		EvaluationPeriods:  aws.Int32(1),
		StateValue:         types.StateValueOk,
	}
}

// NewMockMetric creates a mock Metric for testing
func NewMockMetric(name, namespace string) types.Metric {
	return types.Metric{
		MetricName: aws.String(name),
		Namespace:  aws.String(namespace),
	}
}

// NewMockDimension creates a mock Dimension for testing
func NewMockDimension(name, value string) types.Dimension {
	return types.Dimension{
		Name:  aws.String(name),
		Value: aws.String(value),
	}
}

// NewMockTag creates a mock Tag for testing
func NewMockTag(key, value string) types.Tag {
	return types.Tag{
		Key:   aws.String(key),
		Value: aws.String(value),
	}
}
