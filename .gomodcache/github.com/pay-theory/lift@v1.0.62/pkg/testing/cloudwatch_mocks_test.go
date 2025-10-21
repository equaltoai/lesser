package testing

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestMockCloudWatchClient_PutMetricData(t *testing.T) {
	mockClient := NewMockCloudWatchClient()
	ctx := context.Background()

	// Setup expectation
	input := &cloudwatch.PutMetricDataInput{
		Namespace: aws.String("TestNamespace"),
		MetricData: []types.MetricDatum{
			NewMockMetricDatum("TestMetric", 42.0, types.StandardUnitCount),
		},
	}

	expectedOutput := NewMockPutMetricDataOutput()
	mockClient.On("PutMetricData", ctx, input, mock.Anything).Return(expectedOutput, nil)

	// Execute
	output, err := mockClient.PutMetricData(ctx, input)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, expectedOutput, output)
	mockClient.AssertExpectations(t)
}

func TestMockCloudWatchClient_GetMetricStatistics(t *testing.T) {
	mockClient := NewMockCloudWatchClient()
	ctx := context.Background()

	// Setup expectation
	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("TestNamespace"),
		MetricName: aws.String("TestMetric"),
		StartTime:  aws.Time(time.Now().Add(-1 * time.Hour)),
		EndTime:    aws.Time(time.Now()),
		Period:     aws.Int32(300),
		Statistics: []types.Statistic{types.StatisticAverage},
	}

	datapoints := []types.Datapoint{
		NewMockDatapoint(42.5, types.StandardUnitCount),
	}
	expectedOutput := NewMockGetMetricStatisticsOutput(datapoints)
	mockClient.On("GetMetricStatistics", ctx, input, mock.Anything).Return(expectedOutput, nil)

	// Execute
	output, err := mockClient.GetMetricStatistics(ctx, input)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, expectedOutput, output)
	assert.Len(t, output.Datapoints, 1)
	assert.Equal(t, 42.5, *output.Datapoints[0].Average)
	mockClient.AssertExpectations(t)
}

func TestMockCloudWatchClient_PutMetricAlarm(t *testing.T) {
	mockClient := NewMockCloudWatchClient()
	ctx := context.Background()

	// Setup expectation
	input := &cloudwatch.PutMetricAlarmInput{
		AlarmName:          aws.String("TestAlarm"),
		MetricName:         aws.String("TestMetric"),
		Namespace:          aws.String("TestNamespace"),
		Threshold:          aws.Float64(100.0),
		ComparisonOperator: types.ComparisonOperatorGreaterThanThreshold,
		Statistic:          types.StatisticAverage,
		Period:             aws.Int32(300),
		EvaluationPeriods:  aws.Int32(2),
	}

	expectedOutput := NewMockPutMetricAlarmOutput()
	mockClient.On("PutMetricAlarm", ctx, input, mock.Anything).Return(expectedOutput, nil)

	// Execute
	output, err := mockClient.PutMetricAlarm(ctx, input)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, expectedOutput, output)
	mockClient.AssertExpectations(t)
}

func TestMockCloudWatchClient_DescribeAlarms(t *testing.T) {
	mockClient := NewMockCloudWatchClient()
	ctx := context.Background()

	// Setup expectation
	input := &cloudwatch.DescribeAlarmsInput{
		AlarmNames: []string{"TestAlarm"},
	}

	alarms := []types.MetricAlarm{
		NewMockMetricAlarm("TestAlarm", "TestMetric", 100.0),
	}
	expectedOutput := NewMockDescribeAlarmsOutput(alarms)
	mockClient.On("DescribeAlarms", ctx, input, mock.Anything).Return(expectedOutput, nil)

	// Execute
	output, err := mockClient.DescribeAlarms(ctx, input)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, expectedOutput, output)
	assert.Len(t, output.MetricAlarms, 1)
	assert.Equal(t, "TestAlarm", *output.MetricAlarms[0].AlarmName)
	mockClient.AssertExpectations(t)
}

func TestMockCloudWatchClient_DeleteAlarms(t *testing.T) {
	mockClient := NewMockCloudWatchClient()
	ctx := context.Background()

	// Setup expectation
	input := &cloudwatch.DeleteAlarmsInput{
		AlarmNames: []string{"TestAlarm"},
	}

	expectedOutput := NewMockDeleteAlarmsOutput()
	mockClient.On("DeleteAlarms", ctx, input, mock.Anything).Return(expectedOutput, nil)

	// Execute
	output, err := mockClient.DeleteAlarms(ctx, input)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, expectedOutput, output)
	mockClient.AssertExpectations(t)
}

func TestMockCloudWatchClient_ListMetrics(t *testing.T) {
	mockClient := NewMockCloudWatchClient()
	ctx := context.Background()

	// Setup expectation
	input := &cloudwatch.ListMetricsInput{
		Namespace: aws.String("TestNamespace"),
	}

	metrics := []types.Metric{
		NewMockMetric("TestMetric", "TestNamespace"),
	}
	expectedOutput := NewMockListMetricsOutput(metrics)
	mockClient.On("ListMetrics", ctx, input, mock.Anything).Return(expectedOutput, nil)

	// Execute
	output, err := mockClient.ListMetrics(ctx, input)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, expectedOutput, output)
	assert.Len(t, output.Metrics, 1)
	assert.Equal(t, "TestMetric", *output.Metrics[0].MetricName)
	mockClient.AssertExpectations(t)
}

func TestMockCloudWatchClient_GetMetricData(t *testing.T) {
	mockClient := NewMockCloudWatchClient()
	ctx := context.Background()

	// Setup expectation
	input := &cloudwatch.GetMetricDataInput{
		StartTime: aws.Time(time.Now().Add(-1 * time.Hour)),
		EndTime:   aws.Time(time.Now()),
		MetricDataQueries: []types.MetricDataQuery{
			{
				Id: aws.String("m1"),
				MetricStat: &types.MetricStat{
					Metric: &types.Metric{
						MetricName: aws.String("TestMetric"),
						Namespace:  aws.String("TestNamespace"),
					},
					Period: aws.Int32(300),
					Stat:   aws.String("Average"),
				},
			},
		},
	}

	results := []types.MetricDataResult{
		{
			Id:     aws.String("m1"),
			Label:  aws.String("TestMetric"),
			Values: []float64{42.5},
		},
	}
	expectedOutput := NewMockGetMetricDataOutput(results)
	mockClient.On("GetMetricData", ctx, input, mock.Anything).Return(expectedOutput, nil)

	// Execute
	output, err := mockClient.GetMetricData(ctx, input)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, expectedOutput, output)
	assert.Len(t, output.MetricDataResults, 1)
	assert.Equal(t, "m1", *output.MetricDataResults[0].Id)
	mockClient.AssertExpectations(t)
}

func TestMockCloudWatchClient_TagResource(t *testing.T) {
	mockClient := NewMockCloudWatchClient()
	ctx := context.Background()

	// Setup expectation
	input := &cloudwatch.TagResourceInput{
		ResourceARN: aws.String("arn:aws:cloudwatch:us-east-1:123456789012:alarm:TestAlarm"),
		Tags: []types.Tag{
			NewMockTag("Environment", "Test"),
		},
	}

	expectedOutput := NewMockTagResourceOutput()
	mockClient.On("TagResource", ctx, input, mock.Anything).Return(expectedOutput, nil)

	// Execute
	output, err := mockClient.TagResource(ctx, input)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, expectedOutput, output)
	mockClient.AssertExpectations(t)
}

func TestMockCloudWatchClient_ListTagsForResource(t *testing.T) {
	mockClient := NewMockCloudWatchClient()
	ctx := context.Background()

	// Setup expectation
	input := &cloudwatch.ListTagsForResourceInput{
		ResourceARN: aws.String("arn:aws:cloudwatch:us-east-1:123456789012:alarm:TestAlarm"),
	}

	tags := []types.Tag{
		NewMockTag("Environment", "Test"),
		NewMockTag("Team", "Platform"),
	}
	expectedOutput := NewMockListTagsForResourceOutput(tags)
	mockClient.On("ListTagsForResource", ctx, input, mock.Anything).Return(expectedOutput, nil)

	// Execute
	output, err := mockClient.ListTagsForResource(ctx, input)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, expectedOutput, output)
	assert.Len(t, output.Tags, 2)
	assert.Equal(t, "Environment", *output.Tags[0].Key)
	assert.Equal(t, "Test", *output.Tags[0].Value)
	mockClient.AssertExpectations(t)
}

// Test helper functions
func TestHelperFunctions(t *testing.T) {
	t.Run("NewMockMetricDatum", func(t *testing.T) {
		datum := NewMockMetricDatum("TestMetric", 42.0, types.StandardUnitCount)
		assert.Equal(t, "TestMetric", *datum.MetricName)
		assert.Equal(t, 42.0, *datum.Value)
		assert.Equal(t, types.StandardUnitCount, datum.Unit)
	})

	t.Run("NewMockDatapoint", func(t *testing.T) {
		datapoint := NewMockDatapoint(42.5, types.StandardUnitCount)
		assert.Equal(t, 42.5, *datapoint.Average)
		assert.Equal(t, types.StandardUnitCount, datapoint.Unit)
		assert.NotNil(t, datapoint.Timestamp)
	})

	t.Run("NewMockMetricAlarm", func(t *testing.T) {
		alarm := NewMockMetricAlarm("TestAlarm", "TestMetric", 100.0)
		assert.Equal(t, "TestAlarm", *alarm.AlarmName)
		assert.Equal(t, "TestMetric", *alarm.MetricName)
		assert.Equal(t, 100.0, *alarm.Threshold)
		assert.Equal(t, types.ComparisonOperatorGreaterThanThreshold, alarm.ComparisonOperator)
	})

	t.Run("NewMockMetric", func(t *testing.T) {
		metric := NewMockMetric("TestMetric", "TestNamespace")
		assert.Equal(t, "TestMetric", *metric.MetricName)
		assert.Equal(t, "TestNamespace", *metric.Namespace)
	})

	t.Run("NewMockDimension", func(t *testing.T) {
		dimension := NewMockDimension("InstanceId", "i-1234567890abcdef0")
		assert.Equal(t, "InstanceId", *dimension.Name)
		assert.Equal(t, "i-1234567890abcdef0", *dimension.Value)
	})

	t.Run("NewMockTag", func(t *testing.T) {
		tag := NewMockTag("Environment", "Production")
		assert.Equal(t, "Environment", *tag.Key)
		assert.Equal(t, "Production", *tag.Value)
	})
}

// Test error scenarios
func TestMockCloudWatchClient_ErrorScenarios(t *testing.T) {
	mockClient := NewMockCloudWatchClient()
	ctx := context.Background()

	t.Run("PutMetricData Error", func(t *testing.T) {
		input := &cloudwatch.PutMetricDataInput{
			Namespace: aws.String("TestNamespace"),
		}

		expectedError := assert.AnError
		mockClient.On("PutMetricData", ctx, input, mock.Anything).Return((*cloudwatch.PutMetricDataOutput)(nil), expectedError)

		output, err := mockClient.PutMetricData(ctx, input)

		assert.Error(t, err)
		assert.Equal(t, expectedError, err)
		assert.Nil(t, output)
		mockClient.AssertExpectations(t)
	})

	t.Run("GetMetricStatistics Error", func(t *testing.T) {
		input := &cloudwatch.GetMetricStatisticsInput{
			Namespace:  aws.String("TestNamespace"),
			MetricName: aws.String("TestMetric"),
		}

		expectedError := assert.AnError
		mockClient.On("GetMetricStatistics", ctx, input, mock.Anything).Return((*cloudwatch.GetMetricStatisticsOutput)(nil), expectedError)

		output, err := mockClient.GetMetricStatistics(ctx, input)

		assert.Error(t, err)
		assert.Equal(t, expectedError, err)
		assert.Nil(t, output)
		mockClient.AssertExpectations(t)
	})
}

// Test advanced usage patterns
func TestMockCloudWatchClient_AdvancedUsage(t *testing.T) {
	mockClient := NewMockCloudWatchClient()
	ctx := context.Background()

	t.Run("Conditional Matching", func(t *testing.T) {
		// Setup expectation with conditional matching
		mockClient.On("PutMetricData",
			ctx,
			mock.MatchedBy(func(input *cloudwatch.PutMetricDataInput) bool {
				return *input.Namespace == "TestNamespace" && len(input.MetricData) > 0
			}),
			mock.Anything,
		).Return(NewMockPutMetricDataOutput(), nil)

		// Execute with matching input
		input := &cloudwatch.PutMetricDataInput{
			Namespace: aws.String("TestNamespace"),
			MetricData: []types.MetricDatum{
				NewMockMetricDatum("TestMetric", 42.0, types.StandardUnitCount),
			},
		}

		output, err := mockClient.PutMetricData(ctx, input)

		assert.NoError(t, err)
		assert.NotNil(t, output)
		mockClient.AssertExpectations(t)
	})

	t.Run("Multiple Expectations", func(t *testing.T) {
		// Setup multiple expectations
		mockClient.On("PutMetricData", ctx, mock.AnythingOfType("*cloudwatch.PutMetricDataInput"), mock.Anything).
			Return(NewMockPutMetricDataOutput(), nil).Once()

		mockClient.On("GetMetricStatistics", ctx, mock.AnythingOfType("*cloudwatch.GetMetricStatisticsInput"), mock.Anything).
			Return(NewMockGetMetricStatisticsOutput([]types.Datapoint{NewMockDatapoint(42.5, types.StandardUnitCount)}), nil).Once()

		// Execute both operations
		_, err1 := mockClient.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
			Namespace: aws.String("Test"),
		})
		_, err2 := mockClient.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
			Namespace:  aws.String("Test"),
			MetricName: aws.String("TestMetric"),
		})

		assert.NoError(t, err1)
		assert.NoError(t, err2)
		mockClient.AssertExpectations(t)
	})
}
