package common

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/pay-theory/lift/pkg/lift"
)

const (
	// MaxRequestSize is the default maximum size for general requests (1MB)
	MaxRequestSize = 1 * 1024 * 1024 // 1MB

	// MaxActivitySize is the maximum size for ActivityPub activities
	MaxActivitySize = 512 * 1024 // 512KB

	// MaxMediaSize is the maximum size for media uploads (50MB)
	MaxMediaSize = 50 * 1024 * 1024 // 50MB

	// MaxImportSize is the maximum size for import files (100MB)
	MaxImportSize = 100 * 1024 * 1024 // 100MB
)

// ReadRequestBody safely reads a request body with a size limit
// If maxSize is 0 or negative, MaxRequestSize is used as default
func ReadRequestBody(body io.Reader, maxSize int64) ([]byte, error) {
	if maxSize <= 0 {
		maxSize = MaxRequestSize
	}

	// Use LimitReader to prevent reading more than maxSize bytes
	// Add 1 extra byte to detect if the body exceeds the limit
	limited := io.LimitReader(body, maxSize+1)

	// Read all data from the limited reader
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	// Check if we read more than maxSize (the extra byte)
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("%w: exceeds %d bytes", ErrRequestBodyTooLarge, maxSize)
	}

	return data, nil
}

// ReadRequestBodyString is a convenience function that returns the body as a string
func ReadRequestBodyString(body io.Reader, maxSize int64) (string, error) {
	data, err := ReadRequestBody(body, maxSize)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Request Parsing Consolidation Framework
// This consolidates the 57+ occurrences of ctx.ParseRequest patterns

// ParseRequestWithFallback attempts to parse request using ctx.ParseRequest with fallback strategies
// This consolidates the common pattern of:
//
//	if err := ctx.ParseRequest(&req); err != nil {
//	  // Fallback logic for test environments
//	  ...
//	}
func ParseRequestWithFallback(ctx *lift.Context, target interface{}) error {
	// First try the standard ParseRequest
	if err := ctx.ParseRequest(target); err == nil {
		return nil
	}

	// Fallback for test environments - try parsing from ctx.Request.Body
	if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
		if err := json.Unmarshal(ctx.Request.Body, target); err == nil {
			return nil
		}
	}

	// Alternative fallback - try parsing from ctx.Request.Request.Body if available
	if ctx.Request != nil && ctx.Request.Request != nil && ctx.Request.Request.Body != nil {
		if err := json.Unmarshal(ctx.Request.Request.Body, target); err == nil {
			return nil
		}
	}

	return ErrFailedToParseRequestBody
}

// ParseRequestWithValidation combines parsing with common validation responses
func ParseRequestWithValidation(ctx *lift.Context, target interface{}) error {
	if err := ParseRequestWithFallback(ctx, target); err != nil {
		return RespondValidationError(ctx, err)
	}
	return nil
}

// ParseRequestWithCustomError allows custom error handling
func ParseRequestWithCustomError(ctx *lift.Context, target interface{}, errorMessage string) error {
	if err := ParseRequestWithFallback(ctx, target); err != nil {
		return RespondBadRequest(ctx, errorMessage)
	}
	return nil
}

// Common request parsing patterns found in the codebase

// ParseRequestBodyWithValidation parses request body with validation error response
func ParseRequestBodyWithValidation(ctx *lift.Context, target interface{}, fieldName string) error {
	if err := ParseRequestWithFallback(ctx, target); err != nil {
		return RespondMissingParameter(ctx, fieldName)
	}
	return nil
}

// Specialized parsing functions for common request types

// PaginationParams extracts common pagination parameters
type PaginationParams struct {
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
	MaxID   string `json:"max_id"`
	MinID   string `json:"min_id"`
	SinceID string `json:"since_id"`
}

// TimelineParams extracts timeline-specific parameters
type TimelineParams struct {
	PaginationParams
	Local     bool `json:"local"`
	OnlyMedia bool `json:"only_media"`
}

// FilterParams extracts filter-specific parameters
type FilterParams struct {
	Phrase       string   `json:"phrase"`
	Context      []string `json:"context"`
	ExpiresIn    int      `json:"expires_in"`
	Irreversible bool     `json:"irreversible"`
	WholeWord    bool     `json:"whole_word"`
}

// Helper functions for the common fallback patterns

// ParseRequestWithComplexFallback handles the complex fallback pattern found in quotes.go and other files
func ParseRequestWithComplexFallback(ctx *lift.Context, target interface{}) error {
	// First attempt: standard parsing
	if err := ctx.ParseRequest(target); err == nil {
		return nil
	}

	// Second attempt: fallback with ValidateSliceNotEmpty pattern
	if ctx.Request != nil {
		bodyBytes := ctx.Request.Body
		if err := ValidateSliceNotEmpty("bodyBytes", bodyBytes); err != nil &&
			ctx.Request.Request != nil {
			bodyBytes = ctx.Request.Request.Body
		}

		if err := ParseRequestBody(bodyBytes, target); err == nil {
			return nil
		}
	}

	return ErrFailedToParseWithComplexFallback
}
