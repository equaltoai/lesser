package main

import "errors"

// Error constants for status-indexer package
var (
	// Processing errors
	ErrPartialBatchFailure = errors.New("partial batch failure")
	ErrProcessStatusEvent  = errors.New("failed to process status event")
	
	// Data extraction errors
	ErrNoNewImage   = errors.New("no new image")
	ErrNoObjectData = errors.New("no object data")
	
	// Engagement calculation errors
	ErrCountReplies = errors.New("failed to count replies")
)