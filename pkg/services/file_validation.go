package services

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // MD5 used for file checksums only, not security
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

// File format constants
const (
	FormatJSON = "json"
	FormatCSV  = "csv"
)

// FileValidationService handles comprehensive file validation
type FileValidationService struct {
	s3Client   *s3.Client
	logger     *zap.Logger
	virusCheck bool
}

// FileValidationResult represents the result of file validation
type FileValidationResult struct {
	Valid          bool           `json:"valid"`
	ContentType    string         `json:"content_type"`
	DetectedFormat string         `json:"detected_format"`
	FileSize       int64          `json:"file_size"`
	MD5Hash        string         `json:"md5_hash"`
	Errors         []string       `json:"errors,omitempty"`
	Warnings       []string       `json:"warnings,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// FileValidationConfig represents validation configuration
type FileValidationConfig struct {
	MaxSizeBytes    int64    `json:"max_size_bytes"`    // Maximum file size allowed
	AllowedTypes    []string `json:"allowed_types"`     // Allowed MIME types
	RequiredFormats []string `json:"required_formats"`  // Required formats (json, csv, etc.)
	EnableVirusScan bool     `json:"enable_virus_scan"` // Whether to perform virus scanning
	ValidateContent bool     `json:"validate_content"`  // Whether to validate content structure
}

// DefaultImportValidationConfig returns default config for imports
func DefaultImportValidationConfig() FileValidationConfig {
	return FileValidationConfig{
		MaxSizeBytes: 100 * 1024 * 1024, // 100MB
		AllowedTypes: []string{
			"application/json",
			"text/csv",
			"text/plain",
			"application/octet-stream", // For base64 decoded files
		},
		RequiredFormats: []string{"json", "csv"},
		EnableVirusScan: true,
		ValidateContent: true,
	}
}

// NewFileValidationService creates a new file validation service
func NewFileValidationService(logger *zap.Logger) (*FileValidationService, error) {
	ctx := context.Background()

	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client (used for advanced scanning if needed)
	s3Client := s3.NewFromConfig(cfg)

	return &FileValidationService{
		s3Client:   s3Client,
		logger:     logger,
		virusCheck: true, // Enable virus checking by default
	}, nil
}

// ValidateFile performs comprehensive file validation
func (fv *FileValidationService) ValidateFile(_ context.Context, data []byte, config FileValidationConfig) (*FileValidationResult, error) {
	result := &FileValidationResult{
		Valid:    true,
		FileSize: int64(len(data)),
		Errors:   make([]string, 0),
		Warnings: make([]string, 0),
		Metadata: make(map[string]any),
	}

	// Calculate MD5 hash for file checksum (not for security)
	hash := md5.Sum(data) //nolint:gosec // MD5 used for file integrity check, not cryptography
	result.MD5Hash = hex.EncodeToString(hash[:])

	// Validate file size
	if err := fv.validateFileSize(data, config, result); err != nil {
		return result, err
	}

	// Detect content type
	if err := fv.detectContentType(data, result); err != nil {
		return result, err
	}

	// Validate content type
	if err := fv.validateContentType(config, result); err != nil {
		return result, err
	}

	// Detect and validate format
	if err := fv.validateFormat(data, config, result); err != nil {
		return result, err
	}

	// Perform virus scanning if enabled
	if config.EnableVirusScan && fv.virusCheck {
		if err := fv.performVirusScan(data, result); err != nil {
			return result, err
		}
	}

	// Validate content structure if enabled
	if config.ValidateContent {
		if err := fv.validateContentStructure(data, result); err != nil {
			return result, err
		}
	}

	// Check for suspicious content
	if err := fv.checkSuspiciousContent(data, result); err != nil {
		return result, err
	}

	return result, nil
}

// validateFileSize checks if file size is within limits
func (fv *FileValidationService) validateFileSize(data []byte, config FileValidationConfig, result *FileValidationResult) error {
	size := int64(len(data))

	if size == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, "file is empty")
		return nil
	}

	if size > config.MaxSizeBytes {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("file size (%d bytes) exceeds limit (%d bytes)", size, config.MaxSizeBytes))
		return nil
	}

	result.Metadata["file_size_mb"] = float64(size) / (1024 * 1024)
	return nil
}

// detectContentType detects the content type of the file
func (fv *FileValidationService) detectContentType(data []byte, result *FileValidationResult) error {
	// Use http.DetectContentType for basic detection
	contentType := http.DetectContentType(data)
	result.ContentType = contentType

	// Enhanced detection for common import formats
	if len(data) > 0 {
		switch {
		case bytes.HasPrefix(data, []byte("{")):
			result.ContentType = "application/json"
			result.DetectedFormat = FormatJSON
		case bytes.HasPrefix(data, []byte("[")):
			result.ContentType = "application/json"
			result.DetectedFormat = FormatJSON
		case strings.Contains(string(data[:minInt(200, len(data))]), ","):
			result.ContentType = "text/csv"
			result.DetectedFormat = FormatCSV
		case strings.Contains(contentType, "text"):
			result.DetectedFormat = "text"
		default:
			result.DetectedFormat = "unknown"
		}
	}

	return nil
}

// validateContentType validates that content type is allowed
func (fv *FileValidationService) validateContentType(config FileValidationConfig, result *FileValidationResult) error {
	if len(config.AllowedTypes) == 0 {
		return nil // No restrictions
	}

	allowed := false
	for _, allowedType := range config.AllowedTypes {
		if strings.HasPrefix(result.ContentType, allowedType) {
			allowed = true
			break
		}
	}

	if !allowed {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("content type '%s' is not allowed", result.ContentType))
	}

	return nil
}

// validateFormat validates the detected format
func (fv *FileValidationService) validateFormat(data []byte, config FileValidationConfig, result *FileValidationResult) error {
	if len(config.RequiredFormats) == 0 {
		return nil // No format requirements
	}

	allowed := false
	for _, requiredFormat := range config.RequiredFormats {
		if result.DetectedFormat == requiredFormat {
			allowed = true
			break
		}
	}

	if !allowed {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("format '%s' is not supported (allowed: %v)", result.DetectedFormat, config.RequiredFormats))
		return nil
	}

	// Format-specific validation
	switch result.DetectedFormat {
	case FormatJSON:
		return fv.validateJSONFormat(data, result)
	case FormatCSV:
		return fv.validateCSVFormat(data, result)
	}

	return nil
}

// validateJSONFormat validates JSON structure
func (fv *FileValidationService) validateJSONFormat(data []byte, result *FileValidationResult) error {
	// Basic JSON validation - check if it's valid JSON
	var jsonData interface{}
	if err := fv.parseJSON(data, &jsonData); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("invalid JSON format: %v", err))
		return nil
	}

	// Add metadata about JSON structure
	switch v := jsonData.(type) {
	case map[string]interface{}:
		result.Metadata["json_type"] = "object"
		result.Metadata["json_keys"] = len(v)
	case []interface{}:
		result.Metadata["json_type"] = "array"
		result.Metadata["json_items"] = len(v)
	default:
		result.Metadata["json_type"] = "primitive"
	}

	return nil
}

// validateCSVFormat validates CSV structure
func (fv *FileValidationService) validateCSVFormat(data []byte, result *FileValidationResult) error {
	content := string(data)
	lines := strings.Split(content, "\n")

	if len(lines) < 1 {
		result.Valid = false
		result.Errors = append(result.Errors, "CSV file has no content")
		return nil
	}

	// Check for consistent column count
	headerCols := len(strings.Split(lines[0], ","))
	if headerCols == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, "CSV file has no columns")
		return nil
	}

	inconsistentRows := 0
	for i, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue // Skip empty lines
		}
		cols := len(strings.Split(line, ","))
		if cols != headerCols {
			inconsistentRows++
			if inconsistentRows <= 5 { // Only report first 5 inconsistent rows
				result.Warnings = append(result.Warnings, fmt.Sprintf("row %d has %d columns, expected %d", i+2, cols, headerCols))
			}
		}
	}

	result.Metadata["csv_columns"] = headerCols
	result.Metadata["csv_rows"] = len(lines) - 1 // Exclude header
	result.Metadata["csv_inconsistent_rows"] = inconsistentRows

	if inconsistentRows > len(lines)/2 {
		result.Valid = false
		result.Errors = append(result.Errors, "CSV file has too many inconsistent rows")
	}

	return nil
}

// performVirusScan performs basic virus scanning
func (fv *FileValidationService) performVirusScan(data []byte, result *FileValidationResult) error {
	// Basic virus scanning - check for common virus signatures
	suspiciousPatterns := []string{
		"<script", "javascript:", "vbscript:",
		"eval(", "exec(", "system(",
		"<?php", "<%", "<jsp:",
		"\\x90\\x90\\x90\\x90", // NOP sled
		"MZ",                   // PE header
	}

	content := strings.ToLower(string(data))
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(content, strings.ToLower(pattern)) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("potentially suspicious content detected: %s", pattern))
		}
	}

	// Check for excessive binary content in text files
	if result.DetectedFormat == "json" || result.DetectedFormat == "csv" {
		binaryBytes := 0
		for _, b := range data {
			if b < 32 && b != 9 && b != 10 && b != 13 { // Not printable, tab, LF, or CR
				binaryBytes++
			}
		}

		binaryRatio := float64(binaryBytes) / float64(len(data))
		if binaryRatio > 0.1 { // More than 10% binary content
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("file contains too much binary content (%.1f%%)", binaryRatio*100))
		}
	}

	return nil
}

// validateContentStructure validates content structure for imports
func (fv *FileValidationService) validateContentStructure(data []byte, result *FileValidationResult) error {
	switch result.DetectedFormat {
	case FormatJSON:
		return fv.validateJSONImportStructure(data, result)
	case FormatCSV:
		return fv.validateCSVImportStructure(data, result)
	}

	return nil
}

// validateJSONImportStructure validates JSON structure for imports
func (fv *FileValidationService) validateJSONImportStructure(data []byte, result *FileValidationResult) error {
	var jsonData interface{}
	if err := fv.parseJSON(data, &jsonData); err != nil {
		return nil // Already validated in format validation
	}

	// For imports, expect either an object or array of objects
	switch v := jsonData.(type) {
	case map[string]interface{}:
		// Single object - validate it has expected fields
		if len(v) == 0 {
			result.Warnings = append(result.Warnings, "JSON object is empty")
		}
	case []interface{}:
		// Array of objects
		if len(v) == 0 {
			result.Warnings = append(result.Warnings, "JSON array is empty")
		} else {
			// Sample first few items to validate structure
			sampleSize := minInt(5, len(v))
			for i := 0; i < sampleSize; i++ {
				if _, ok := v[i].(map[string]interface{}); !ok {
					result.Warnings = append(result.Warnings, fmt.Sprintf("array item %d is not an object", i))
				}
			}
		}
	default:
		result.Warnings = append(result.Warnings, "JSON is not an object or array")
	}

	return nil
}

// validateCSVImportStructure validates CSV structure for imports
func (fv *FileValidationService) validateCSVImportStructure(data []byte, result *FileValidationResult) error {
	content := string(data)
	lines := strings.Split(content, "\n")

	if len(lines) < 2 {
		result.Warnings = append(result.Warnings, "CSV file has no data rows")
		return nil
	}

	// Check for common import fields in header
	header := strings.ToLower(lines[0])
	expectedFields := []string{"username", "name", "id", "url", "email"}
	foundFields := 0

	for _, field := range expectedFields {
		if strings.Contains(header, field) {
			foundFields++
		}
	}

	if foundFields == 0 {
		result.Warnings = append(result.Warnings, "CSV header does not contain expected fields for import")
	}

	return nil
}

// checkSuspiciousContent performs additional security checks
func (fv *FileValidationService) checkSuspiciousContent(data []byte, result *FileValidationResult) error {
	content := string(data)

	// Check for excessively long lines (potential DoS)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if len(line) > 100000 { // 100KB per line
			result.Warnings = append(result.Warnings, fmt.Sprintf("line %d is excessively long (%d characters)", i+1, len(line)))
		}
	}

	// Check for deeply nested structures in JSON
	if result.DetectedFormat == "json" {
		depth := fv.calculateJSONDepth(content)
		if depth > 50 {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("JSON structure is too deeply nested (%d levels)", depth))
		}
		result.Metadata["json_depth"] = depth
	}

	return nil
}

// Helper functions

func (fv *FileValidationService) parseJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func (fv *FileValidationService) calculateJSONDepth(content string) int {
	maxDepth := 0
	currentDepth := 0
	inString := false
	escaped := false

	for _, char := range content {
		if escaped {
			escaped = false
			continue
		}

		switch char {
		case '\\':
			escaped = true
		case '"':
			inString = !inString
		case '{', '[':
			if !inString {
				currentDepth++
				if currentDepth > maxDepth {
					maxDepth = currentDepth
				}
			}
		case '}', ']':
			if !inString {
				currentDepth--
			}
		}
	}

	return maxDepth
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
