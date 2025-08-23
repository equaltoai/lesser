package main

import "errors"

// Content extraction errors
var (
	ErrContentExtractionFailed = errors.New("failed to extract content from stream record")
	ErrStreamUnmarshalFailed   = errors.New("failed to unmarshal stream record")
	ErrInvalidObjectPK         = errors.New("invalid object primary key format")
	ErrNotAnalyzableType       = errors.New("object type is not analyzable")
)

// AI processing errors
var (
	ErrAnalysisFailed     = errors.New("AI analysis failed")
	ErrAnalysisSaveFailed = errors.New("failed to save AI analysis")
)