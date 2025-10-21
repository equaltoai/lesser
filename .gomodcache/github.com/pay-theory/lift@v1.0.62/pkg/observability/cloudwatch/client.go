package cloudwatch

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"

	"github.com/pay-theory/lift/pkg/observability"
)

// CloudWatchLogsClientImpl wraps the AWS CloudWatch Logs client to implement our interface
type CloudWatchLogsClientImpl struct {
	client *cloudwatchlogs.Client
}

// NewCloudWatchLogsClient creates a new CloudWatch Logs client wrapper
func NewCloudWatchLogsClient(awsConfig aws.Config) observability.CloudWatchLogsClient {
	return &CloudWatchLogsClientImpl{
		client: cloudwatchlogs.NewFromConfig(awsConfig),
	}
}

// CreateLogGroup creates a log group
func (c *CloudWatchLogsClientImpl) CreateLogGroup(ctx context.Context, params *cloudwatchlogs.CreateLogGroupInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error) {
	return c.client.CreateLogGroup(ctx, params, optFns...)
}

// CreateLogStream creates a log stream
func (c *CloudWatchLogsClientImpl) CreateLogStream(ctx context.Context, params *cloudwatchlogs.CreateLogStreamInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogStreamOutput, error) {
	return c.client.CreateLogStream(ctx, params, optFns...)
}

// PutLogEvents puts log events to CloudWatch
func (c *CloudWatchLogsClientImpl) PutLogEvents(ctx context.Context, params *cloudwatchlogs.PutLogEventsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutLogEventsOutput, error) {
	return c.client.PutLogEvents(ctx, params, optFns...)
}

// DescribeLogGroups describes log groups
func (c *CloudWatchLogsClientImpl) DescribeLogGroups(ctx context.Context, params *cloudwatchlogs.DescribeLogGroupsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	return c.client.DescribeLogGroups(ctx, params, optFns...)
}

// DescribeLogStreams describes log streams
func (c *CloudWatchLogsClientImpl) DescribeLogStreams(ctx context.Context, params *cloudwatchlogs.DescribeLogStreamsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogStreamsOutput, error) {
	return c.client.DescribeLogStreams(ctx, params, optFns...)
}
