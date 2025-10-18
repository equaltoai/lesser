package jsonld

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Error constants for JSON-LD canonicalization operations
var (
	// Input normalization errors
	ErrNormalizeInput = errors.ProcessingFailed("input normalization", stdErrors.New("input normalization failed"))

	// N-Quads conversion errors
	ErrConvertToNQuads = errors.ProcessingFailed("N-Quads conversion", stdErrors.New("failed to convert to N-Quads"))

	// JSON structure canonicalization errors
	ErrCanonicalizeJSONStructure = errors.ProcessingFailed("JSON structure canonicalization", stdErrors.New("failed to canonicalize JSON structure"))

	// JSON parsing errors
	ErrParseJSON       = errors.ParsingFailed("JSON", stdErrors.New("JSON parsing failed"))
	ErrParseJSONString = errors.ParsingFailed("JSON string", stdErrors.New("JSON string parsing failed"))

	// Marshaling/unmarshaling errors
	ErrMarshalInput        = errors.MarshalingFailed("input", stdErrors.New("input marshaling failed"))
	ErrUnmarshalNormalized = errors.UnmarshalingFailed("normalized data", stdErrors.New("normalized data unmarshaling failed"))
)
