package transformations

import "errors"

// Error constants for transformations package
var (
	// ErrTransformFunctionNotSet indicates that the transform function was not configured
	ErrTransformFunctionNotSet = errors.New("transform function not set")

	// ErrTransformItemFailed indicates that an item transformation failed
	ErrTransformItemFailed = errors.New("failed to transform item")
)
