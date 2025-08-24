package main

import "errors"

// Error constants for note-processor Lambda function

// Stream processing errors
var (
	ErrPartialBatchFailure = errors.New("partial batch failure processing stream records")
)

// Repository and data access errors
var (
	ErrGetNote            = errors.New("failed to get note")
	ErrGetVotes           = errors.New("failed to get votes")
	ErrUpdateNoteAnalysis = errors.New("failed to update note analysis")
	ErrUpdateNoteScore    = errors.New("failed to update note score")
)

// AWS service errors
var (
	ErrDetectSentiment = errors.New("failed to detect sentiment")
)
