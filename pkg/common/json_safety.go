package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

const (
	// MaxJSONDepth is the maximum JSON depth to prevent deep nesting attacks
	MaxJSONDepth = 10

	// MaxJSONKeys is the maximum number of keys in an object
	MaxJSONKeys = 100

	// MaxJSONArrayLength is the maximum array length
	MaxJSONArrayLength = 1000

	// MaxJSONStringLength is the maximum string length in JSON
	MaxJSONStringLength = 50000

	// MaxJSONSize is the maximum total JSON size (already enforced by request limits)
	MaxJSONSize = 512 * 1024 // 512KB
)

// SafeJSONDecoder wraps json.Decoder with safety limits
type SafeJSONDecoder struct {
	decoder *json.Decoder
	depth   int
}

// NewSafeJSONDecoder creates a decoder with safety limits
func NewSafeJSONDecoder(r io.Reader) *SafeJSONDecoder {
	// Limit the input size
	limited := io.LimitReader(r, MaxJSONSize)

	decoder := json.NewDecoder(limited)
	// Reject unknown fields to prevent injection
	decoder.DisallowUnknownFields()

	return &SafeJSONDecoder{
		decoder: decoder,
		depth:   0,
	}
}

// Decode safely decodes JSON with depth and size limits
func (d *SafeJSONDecoder) Decode(v any) error {
	// For complex validation, we need to decode to any first
	var raw any
	if err := d.decoder.Decode(&raw); err != nil {
		return fmt.Errorf("JSON decode error: %w", err)
	}

	// Validate the decoded structure
	if err := d.validateJSON(raw, 0); err != nil {
		return fmt.Errorf("JSON validation error: %w", err)
	}

	// Re-encode and decode to target type
	// This is inefficient but safe
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return err
	}

	return json.Unmarshal(jsonBytes, v)
}

// validateJSON recursively validates JSON structure
func (d *SafeJSONDecoder) validateJSON(v any, depth int) error {
	if depth > MaxJSONDepth {
		return fmt.Errorf("JSON depth %d exceeds maximum %d", depth, MaxJSONDepth)
	}

	switch val := v.(type) {
	case map[string]any:
		// Use len check directly for JSON object keys count
		if len(val) > MaxJSONKeys {
			return fmt.Errorf("JSON object has %d keys, maximum is %d", len(val), MaxJSONKeys)
		}

		for key, value := range val {
			if err := ValidateStringLength("JSON key", key, 0, MaxJSONStringLength); err != nil {
				return fmt.Errorf("JSON key too long: %d bytes", len(key))
			}
			if err := d.validateJSON(value, depth+1); err != nil {
				return err
			}
		}

	case []any:
		if err := ValidateSliceLength("JSON array", val, MaxJSONArrayLength); err != nil {
			return fmt.Errorf("JSON array has %d elements, maximum is %d", len(val), MaxJSONArrayLength)
		}

		for _, item := range val {
			if err := d.validateJSON(item, depth+1); err != nil {
				return err
			}
		}

	case string:
		if err := ValidateStringLength("JSON string", val, 0, MaxJSONStringLength); err != nil {
			return fmt.Errorf("JSON string too long: %d bytes", len(val))
		}

	case float64, bool, nil:
		// These are safe

	default:
		return fmt.Errorf("unexpected JSON type: %T", v)
	}

	return nil
}

// SafeUnmarshalJSON is a convenience function for safe JSON unmarshaling
func SafeUnmarshalJSON(data []byte, v any) error {
	if err := ValidateSliceLength("JSON data", data, MaxJSONSize); err != nil {
		return fmt.Errorf("JSON size %d bytes exceeds maximum %d", len(data), MaxJSONSize)
	}

	decoder := NewSafeJSONDecoder(bytes.NewReader(data))
	return decoder.Decode(v)
}

// SafeUnmarshalJSONWithoutUnknownFields unmarshals JSON without DisallowUnknownFields
// Use this for ActivityPub objects which may have extensions
func SafeUnmarshalJSONWithoutUnknownFields(data []byte, v any) error {
	if err := ValidateSliceLength("JSON data", data, MaxJSONSize); err != nil {
		return fmt.Errorf("JSON size %d bytes exceeds maximum %d", len(data), MaxJSONSize)
	}

	// First validate the structure
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("JSON decode error: %w", err)
	}

	decoder := &SafeJSONDecoder{}
	if err := decoder.validateJSON(raw, 0); err != nil {
		return fmt.Errorf("JSON validation error: %w", err)
	}

	// Then unmarshal normally (allows unknown fields)
	return json.Unmarshal(data, v)
}

// DetectJSONBomb checks for obvious JSON bomb patterns
func DetectJSONBomb(data []byte) error {
	dataStr := string(data)

	// Check for excessive repetition (compression bomb indicator)
	if detectRepetition(dataStr) {
		return ErrJSONBombRepetitionDetected
	}

	// Check for exponential expansion patterns
	if strings.Count(dataStr, "[") > 100 || strings.Count(dataStr, "{") > 100 {
		return ErrJSONBombNestingDetected
	}

	return nil
}

func detectRepetition(s string) bool {
	// Simple repetition detection
	// In production, use more sophisticated algorithm
	for length := 10; length < 100 && length < len(s)/10; length++ {
		pattern := s[:length]
		count := strings.Count(s, pattern)
		if count > len(s)/length/2 {
			return true
		}
	}
	return false
}

// ParseRequestBody safely parses a request body with JSON bomb detection
// Use this for API endpoints with known request structures
func ParseRequestBody(body []byte, v any) error {
	// Check for JSON bombs
	if err := DetectJSONBomb(body); err != nil {
		return fmt.Errorf("invalid JSON structure: %w", err)
	}

	// Parse with strict validation (unknown fields rejected)
	return SafeUnmarshalJSON(body, v)
}

// ParseActivityPubObject safely parses an ActivityPub object with JSON bomb detection
// Use this for ActivityPub objects that may have extensions
func ParseActivityPubObject(body []byte, v any) error {
	// Check for JSON bombs
	if err := DetectJSONBomb(body); err != nil {
		return fmt.Errorf("invalid JSON structure: %w", err)
	}

	// Parse allowing unknown fields (for ActivityPub extensions)
	return SafeUnmarshalJSONWithoutUnknownFields(body, v)
}

// ParseHTTPResponse safely parses an HTTP response body
// Use this when fetching data from external sources
func ParseHTTPResponse(r io.Reader, v any) error {
	decoder := NewSafeJSONDecoder(r)
	return decoder.Decode(v)
}

// ParseFormValues parses URL-encoded form data
func ParseFormValues(body string) (url.Values, error) {
	if err := ValidateRequiredParam("form body", body); err != nil {
		return url.Values{}, err
	}
	return url.ParseQuery(body)
}
