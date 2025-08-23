package main

import "errors"

// Error constants for cost-aggregator function
var (
	// Processing errors
	ErrAggregationFailed = errors.New("failed to aggregate costs")
	
	// AWS service errors
	ErrSNSMessageMarshal  = errors.New("failed to marshal SNS message")
	ErrSNSPublish         = errors.New("failed to publish SNS message")
	ErrCloudWatchMetric   = errors.New("failed to put CloudWatch metric")
	ErrEventMarshal       = errors.New("failed to marshal aggregation event")
	ErrLambdaInvoke       = errors.New("failed to invoke lambda and send SQS message")
	ErrLambdaFunctionError = errors.New("lambda function returned error")
)