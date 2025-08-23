package main

import "errors"

// Error constants for metrics-aggregator service
var (
	// Validation errors
	ErrMissingRequiredFields = errors.New("missing required fields")

	// AWS/Repository operation errors
	ErrServiceStatsRetrieval = errors.New("failed to get service stats")

	// Processing errors
	ErrMetricsAggregation = errors.New("failed to aggregate metrics")
	ErrMetricsCleanup     = errors.New("failed to cleanup metrics")

	// Parsing/Serialization errors
	ErrStreamRecordUnmarshal = errors.New("failed to unmarshal metric from stream record")
)