package monitoring

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type stubCloudWatch struct {
	mu sync.Mutex

	putMetricDataInputs []*cloudwatch.PutMetricDataInput
	putMetricDataErr    error

	getMetricStatisticsInputs []*cloudwatch.GetMetricStatisticsInput
	getMetricStatisticsOutput *cloudwatch.GetMetricStatisticsOutput
	getMetricStatisticsErr    error

	putMetricAlarmInputs []*cloudwatch.PutMetricAlarmInput
	putMetricAlarmErr    error
}

func (s *stubCloudWatch) PutMetricData(_ context.Context, params *cloudwatch.PutMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.putMetricDataInputs = append(s.putMetricDataInputs, params)
	return &cloudwatch.PutMetricDataOutput{}, s.putMetricDataErr
}

func (s *stubCloudWatch) GetMetricStatistics(_ context.Context, params *cloudwatch.GetMetricStatisticsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.getMetricStatisticsInputs = append(s.getMetricStatisticsInputs, params)
	if s.getMetricStatisticsErr != nil {
		return nil, s.getMetricStatisticsErr
	}
	if s.getMetricStatisticsOutput != nil {
		return s.getMetricStatisticsOutput, nil
	}
	return &cloudwatch.GetMetricStatisticsOutput{}, nil
}

func (s *stubCloudWatch) PutMetricAlarm(_ context.Context, params *cloudwatch.PutMetricAlarmInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricAlarmOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.putMetricAlarmInputs = append(s.putMetricAlarmInputs, params)
	return &cloudwatch.PutMetricAlarmOutput{}, s.putMetricAlarmErr
}

func cwDimensionsToMap(dimensions []cwTypes.Dimension) map[string]string {
	out := make(map[string]string, len(dimensions))
	for _, dim := range dimensions {
		name := ""
		if dim.Name != nil {
			name = *dim.Name
		}
		value := ""
		if dim.Value != nil {
			value = *dim.Value
		}
		out[name] = value
	}
	return out
}

