package cloudwatch

import (
	"context"
	"errors"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	"github.com/pay-theory/lift/pkg/observability"
)

// MockCloudWatchLogsClient provides a mock implementation for testing
type MockCloudWatchLogsClient struct {
	calls                     map[string]int64
	errors                    map[string]error
	logGroups                 map[string]*types.LogGroup
	logStreams                map[string]map[string]*types.LogStream
	sequenceTokens            map[string]string
	logEvents                 []types.InputLogEvent
	mu                        sync.RWMutex
	shouldFailCreateLogGroup  bool
	shouldFailCreateLogStream bool
	shouldFailPutLogEvents    bool
}

// NewMockCloudWatchLogsClient creates a new mock CloudWatch Logs client
func NewMockCloudWatchLogsClient() *MockCloudWatchLogsClient {
	return &MockCloudWatchLogsClient{
		calls:          make(map[string]int64),
		errors:         make(map[string]error),
		logGroups:      make(map[string]*types.LogGroup),
		logStreams:     make(map[string]map[string]*types.LogStream),
		sequenceTokens: make(map[string]string),
	}
}

// CreateLogGroup mocks creating a log group
func (m *MockCloudWatchLogsClient) CreateLogGroup(_ context.Context, params *cloudwatchlogs.CreateLogGroupInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error) {
	m.incrementCall("CreateLogGroup")

	if err := m.getError("CreateLogGroup"); err != nil {
		return nil, err
	}

	if m.shouldFailCreateLogGroup {
		return nil, errors.New("mock error: failed to create log group")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	logGroupName := aws.ToString(params.LogGroupName)

	// Check if already exists
	if _, exists := m.logGroups[logGroupName]; exists {
		return nil, &types.ResourceAlreadyExistsException{
			Message: aws.String("Log group already exists"),
		}
	}

	// Create log group
	m.logGroups[logGroupName] = &types.LogGroup{
		LogGroupName: params.LogGroupName,
		CreationTime: aws.Int64(1234567890),
	}

	// Initialize log streams map for this group
	m.logStreams[logGroupName] = make(map[string]*types.LogStream)

	return &cloudwatchlogs.CreateLogGroupOutput{}, nil
}

// CreateLogStream mocks creating a log stream
func (m *MockCloudWatchLogsClient) CreateLogStream(_ context.Context, params *cloudwatchlogs.CreateLogStreamInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogStreamOutput, error) {
	m.incrementCall("CreateLogStream")

	if err := m.getError("CreateLogStream"); err != nil {
		return nil, err
	}

	if m.shouldFailCreateLogStream {
		return nil, errors.New("mock error: failed to create log stream")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	logGroupName := aws.ToString(params.LogGroupName)
	logStreamName := aws.ToString(params.LogStreamName)

	// Check if log group exists
	if _, exists := m.logGroups[logGroupName]; !exists {
		return nil, &types.ResourceNotFoundException{
			Message: aws.String("Log group not found"),
		}
	}

	// Check if log stream already exists
	if streams, exists := m.logStreams[logGroupName]; exists {
		if _, streamExists := streams[logStreamName]; streamExists {
			return nil, &types.ResourceAlreadyExistsException{
				Message: aws.String("Log stream already exists"),
			}
		}
	}

	// Create log stream
	m.logStreams[logGroupName][logStreamName] = &types.LogStream{
		LogStreamName: params.LogStreamName,
		CreationTime:  aws.Int64(1234567890),
	}

	return &cloudwatchlogs.CreateLogStreamOutput{}, nil
}

// PutLogEvents mocks putting log events
func (m *MockCloudWatchLogsClient) PutLogEvents(_ context.Context, params *cloudwatchlogs.PutLogEventsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutLogEventsOutput, error) {
	m.incrementCall("PutLogEvents")

	if err := m.getError("PutLogEvents"); err != nil {
		return nil, err
	}

	if m.shouldFailPutLogEvents {
		return nil, errors.New("mock error: failed to put log events")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	logGroupName := aws.ToString(params.LogGroupName)
	logStreamName := aws.ToString(params.LogStreamName)

	// Check if log group and stream exist
	if _, exists := m.logGroups[logGroupName]; !exists {
		return nil, &types.ResourceNotFoundException{
			Message: aws.String("Log group not found"),
		}
	}

	if streams, exists := m.logStreams[logGroupName]; !exists {
		return nil, &types.ResourceNotFoundException{
			Message: aws.String("Log stream not found"),
		}
	} else if _, streamExists := streams[logStreamName]; !streamExists {
		return nil, &types.ResourceNotFoundException{
			Message: aws.String("Log stream not found"),
		}
	}

	// Store log events
	m.logEvents = append(m.logEvents, params.LogEvents...)

	// Generate next sequence token
	key := logGroupName + ":" + logStreamName
	nextToken := "next-sequence-token-" + string(rune(len(m.logEvents)))
	m.sequenceTokens[key] = nextToken

	return &cloudwatchlogs.PutLogEventsOutput{
		NextSequenceToken: aws.String(nextToken),
	}, nil
}

// DescribeLogGroups mocks describing log groups
func (m *MockCloudWatchLogsClient) DescribeLogGroups(_ context.Context, _ *cloudwatchlogs.DescribeLogGroupsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	m.incrementCall("DescribeLogGroups")

	if err := m.getError("DescribeLogGroups"); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	logGroups := make([]types.LogGroup, 0, len(m.logGroups))
	for _, lg := range m.logGroups {
		logGroups = append(logGroups, *lg)
	}

	return &cloudwatchlogs.DescribeLogGroupsOutput{
		LogGroups: logGroups,
	}, nil
}

// DescribeLogStreams mocks describing log streams
func (m *MockCloudWatchLogsClient) DescribeLogStreams(_ context.Context, params *cloudwatchlogs.DescribeLogStreamsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogStreamsOutput, error) {
	m.incrementCall("DescribeLogStreams")

	if err := m.getError("DescribeLogStreams"); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	logGroupName := aws.ToString(params.LogGroupName)

	var logStreams []types.LogStream
	if streams, exists := m.logStreams[logGroupName]; exists {
		for _, ls := range streams {
			logStreams = append(logStreams, *ls)
		}
	}

	return &cloudwatchlogs.DescribeLogStreamsOutput{
		LogStreams: logStreams,
	}, nil
}

// Test helper methods

// GetCallCount returns the number of times a method was called
func (m *MockCloudWatchLogsClient) GetCallCount(method string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.calls[method]
}

// SetError sets an error to be returned for a specific method
func (m *MockCloudWatchLogsClient) SetError(method string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[method] = err
}

// ClearErrors clears all set errors
func (m *MockCloudWatchLogsClient) ClearErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors = make(map[string]error)
}

// GetLogEvents returns all stored log events
func (m *MockCloudWatchLogsClient) GetLogEvents() []types.InputLogEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]types.InputLogEvent{}, m.logEvents...)
}

// ClearLogEvents clears all stored log events
func (m *MockCloudWatchLogsClient) ClearLogEvents() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logEvents = nil
}

// GetLogGroups returns all created log groups
func (m *MockCloudWatchLogsClient) GetLogGroups() map[string]*types.LogGroup {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]*types.LogGroup)
	for k, v := range m.logGroups {
		result[k] = v
	}
	return result
}

// GetLogStreams returns all created log streams for a log group
func (m *MockCloudWatchLogsClient) GetLogStreams(logGroupName string) map[string]*types.LogStream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if streams, exists := m.logStreams[logGroupName]; exists {
		result := make(map[string]*types.LogStream)
		for k, v := range streams {
			result[k] = v
		}
		return result
	}
	return nil
}

// Reset clears all state
func (m *MockCloudWatchLogsClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = make(map[string]int64)
	m.errors = make(map[string]error)
	m.logGroups = make(map[string]*types.LogGroup)
	m.logStreams = make(map[string]map[string]*types.LogStream)
	m.logEvents = nil
	m.sequenceTokens = make(map[string]string)
	m.shouldFailCreateLogGroup = false
	m.shouldFailCreateLogStream = false
	m.shouldFailPutLogEvents = false
}

// SetShouldFail sets whether specific operations should fail
func (m *MockCloudWatchLogsClient) SetShouldFail(operation string, shouldFail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch operation {
	case "CreateLogGroup":
		m.shouldFailCreateLogGroup = shouldFail
	case "CreateLogStream":
		m.shouldFailCreateLogStream = shouldFail
	case "PutLogEvents":
		m.shouldFailPutLogEvents = shouldFail
	}
}

// Private helper methods

func (m *MockCloudWatchLogsClient) incrementCall(method string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls[method]++
}

func (m *MockCloudWatchLogsClient) getError(method string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.errors[method]
}

// Ensure MockCloudWatchLogsClient implements the interface
var _ observability.CloudWatchLogsClient = (*MockCloudWatchLogsClient)(nil)
