// Package main defines error constants for the federation-aggregator Lambda function.
package main

import "errors"

// Error constants for federation aggregation operations
var (
	// AWS Service Errors
	ErrAWSClientsInit         = errors.New("failed to initialize AWS clients")
	ErrLambdaFunctionError    = errors.New("lambda function returned error")
	ErrLambdaInvocationFailed = errors.New("failed to invoke lambda and send SQS message")

	// Processing Errors
	ErrAggregationEventUnmarshal = errors.New("failed to unmarshal aggregation event")
	ErrAggregationEventMarshal   = errors.New("failed to marshal aggregation event")
	ErrAggregationStore          = errors.New("failed to store aggregation")
	ErrFederationActivitiesGet   = errors.New("failed to get federation activities")
	ErrMessageProcessingFailed   = errors.New("failed to process SQS message")
)
