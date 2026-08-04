package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
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
		return nil, fmt.Errorf("request body too large: exceeds %d bytes", maxSize)
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
func ParseRequestWithFallback(ctx *apptheory.Context, target interface{}) error {
	if ctx == nil {
		return fmt.Errorf("failed to parse request body")
	}
	if len(ctx.Request.Body) == 0 {
		return fmt.Errorf("failed to parse request body")
	}
	if err := json.Unmarshal(ctx.Request.Body, target); err != nil {
		return fmt.Errorf("failed to parse request body")
	}
	return nil
}

// ParseRequestStrict parses a request body and rejects unknown fields. Use this for endpoints where the JSON
// contract is considered frozen (e.g. internal protocol deliveries).
func ParseRequestStrict(ctx *apptheory.Context, target interface{}) error {
	if ctx == nil {
		return apperrors.ValidationFailed("body", "missing request body")
	}
	if len(ctx.Request.Body) == 0 {
		return apperrors.ValidationFailed("body", "missing request body")
	}

	decoder := json.NewDecoder(bytes.NewReader(ctx.Request.Body))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		switch {
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			return apperrors.ValidationFailed("body", err.Error())
		default:
			return apperrors.ValidationFailed("body", "invalid request body")
		}
	}

	// Reject trailing data (multiple JSON values).
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return apperrors.ValidationFailed("body", "invalid request body")
	}

	return nil
}

// ParseRequestWithValidation combines parsing with common validation responses
func ParseRequestWithValidation(ctx *apptheory.Context, target interface{}) (*apptheory.Response, error) {
	if err := ParseRequestWithFallback(ctx, target); err != nil {
		return RespondValidationError(ctx, err)
	}
	return nil, nil
}

// ParseRequestStrictWithValidation combines strict parsing (unknown fields rejected) with common validation responses.
func ParseRequestStrictWithValidation(ctx *apptheory.Context, target interface{}) (*apptheory.Response, error) {
	if err := ParseRequestStrict(ctx, target); err != nil {
		return RespondValidationError(ctx, err)
	}
	return nil, nil
}

// ParseRequestWithCustomError allows custom error handling
func ParseRequestWithCustomError(ctx *apptheory.Context, target interface{}, errorMessage string) (*apptheory.Response, error) {
	if err := ParseRequestWithFallback(ctx, target); err != nil {
		return RespondBadRequest(ctx, errorMessage)
	}
	return nil, nil
}

// Common request parsing patterns found in the codebase

// ParseRequestBodyWithValidation parses request body with validation error response
func ParseRequestBodyWithValidation(ctx *apptheory.Context, target interface{}, fieldName string) (*apptheory.Response, error) {
	if err := ParseRequestWithFallback(ctx, target); err != nil {
		return RespondMissingParameter(ctx, fieldName)
	}
	return nil, nil
}

// Specialized parsing functions for common request types

// RequestPaginationParams extracts common pagination parameters (different from pagination.go version)
type RequestPaginationParams struct {
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
	MaxID   string `json:"max_id"`
	MinID   string `json:"min_id"`
	SinceID string `json:"since_id"`
}

// TimelineParams extracts timeline-specific parameters
type TimelineParams struct {
	RequestPaginationParams
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
func ParseRequestWithComplexFallback(ctx *apptheory.Context, target interface{}) error {
	if err := ParseRequestWithFallback(ctx, target); err != nil {
		return fmt.Errorf("failed to parse request with complex fallback")
	}
	return nil
}
