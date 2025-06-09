package common

import (
	"fmt"
	"io"
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
