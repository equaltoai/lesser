package jsonld

import "errors"

// Error constants for JSON-LD canonicalization operations
var (
	// Input normalization errors
	ErrNormalizeInput = errors.New("failed to normalize input")
	
	// N-Quads conversion errors
	ErrConvertToNQuads = errors.New("failed to convert to N-Quads")
	
	// JSON structure canonicalization errors
	ErrCanonicalizeJSONStructure = errors.New("failed to canonicalize JSON structure")
	
	// JSON parsing errors
	ErrParseJSON = errors.New("failed to parse JSON")
	ErrParseJSONString = errors.New("failed to parse JSON string")
	
	// Marshaling/unmarshaling errors
	ErrMarshalInput = errors.New("failed to marshal input")
	ErrUnmarshalNormalized = errors.New("failed to unmarshal normalized")
)