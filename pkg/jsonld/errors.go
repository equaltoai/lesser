package jsonld

import "github.com/equaltoai/lesser/pkg/errors"

// Error constants for JSON-LD canonicalization operations
var (
	// Input normalization errors
	ErrNormalizeInput = errors.ProcessingFailed("input normalization", nil)

	// N-Quads conversion errors
	ErrConvertToNQuads = errors.ProcessingFailed("N-Quads conversion", nil)

	// JSON structure canonicalization errors
	ErrCanonicalizeJSONStructure = errors.ProcessingFailed("JSON structure canonicalization", nil)

	// JSON parsing errors
	ErrParseJSON       = errors.ParsingFailed("JSON", nil)
	ErrParseJSONString = errors.ParsingFailed("JSON string", nil)

	// Marshaling/unmarshaling errors
	ErrMarshalInput        = errors.MarshalingFailed("input", nil)
	ErrUnmarshalNormalized = errors.UnmarshalingFailed("normalized data", nil)
)
