package main

import "errors"

// Error constants for trend-aggregator service
var (
	// Hashtag trend aggregation errors
	ErrHashtagRetrieval = errors.New("failed to get recent hashtags")

	// Status trend aggregation errors
	ErrStatusRetrieval = errors.New("failed to get recent statuses")

	// Link trend aggregation errors
	ErrLinkRetrieval = errors.New("failed to get recent links")
)
