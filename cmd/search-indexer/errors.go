package main

import "errors"

// Batch processing errors
var (
	ErrPartialBatchFailure = errors.New("partial batch failure during search indexing")
)

// Content extraction errors
var (
	ErrExtractIndexableContent = errors.New("failed to extract indexable content")
	ErrUnmarshalStreamImage    = errors.New("failed to unmarshal stream image")
)

// Search indexing errors
var (
	ErrCreateSearchIndex      = errors.New("failed to create search index")
	ErrStoreSearchIndex       = errors.New("failed to store search index")
	ErrCreateActorSearchIndex = errors.New("failed to create actor search index")
)
