package lift

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// ImportRequest represents a data import request
type ImportRequest struct {
	Type string `json:"type"` // followers, following, blocks, mutes, lists, bookmarks
	Data string `json:"data"` // Base64 encoded file content
	Mode string `json:"mode"` // merge, overwrite
}

// ImportJob represents an import job status
type ImportJob struct {
	ID        string   `json:"id"`
	Status    string   `json:"status"` // pending, processing, completed, failed
	Type      string   `json:"type"`
	CreatedAt string   `json:"created_at"`
	Processed int      `json:"processed"`
	Total     *int     `json:"total"`
	Errors    []string `json:"errors,omitempty"`
	Results   *Results `json:"results,omitempty"`
}

// Results for import completion
type Results struct {
	Success int `json:"success"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

// HandleCreateImportLift handles POST /api/v1/imports
func (h *Handler) HandleCreateImportLift(ctx *lift.Context) error {
	// Authenticate request
	username, err := h.authenticateImportRequest(ctx)
	if err != nil {
		return err
	}

	// Parse and validate request
	req, err := h.parseImportRequest(ctx)
	if err != nil {
		return err
	}

	// Validate import parameters
	if err := h.validateImportParams(req); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]any{"error": err.Error()})
	}

	// Process file data
	fileData, err := h.processImportFileData(ctx, req.Data)
	if err != nil {
		return err
	}

	// Check for existing imports
	if err := h.checkExistingImports(ctx, username, req.Type); err != nil {
		return err
	}

	// Create and store import
	importID := uuid.New().String()
	s3Key, err := h.storeImportFile(ctx, username, importID, req.Type, fileData)
	if err != nil {
		return err
	}

	// Create import record and queue job
	if err := h.createImportRecord(ctx, importID, username, req, s3Key); err != nil {
		return err
	}

	// Return job status
	job := ImportJob{
		ID:        importID,
		Status:    "pending",
		Type:      req.Type,
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	return ctx.Status(http.StatusAccepted).JSON(job)
}

// authenticateImportRequest handles authentication for import requests
func (h *Handler) authenticateImportRequest(ctx *lift.Context) (string, error) {
	// Check for test username
	testUsername := h.getImportTestUsername(ctx)
	if testUsername != "" {
		return testUsername, nil
	}

	// Extract auth header
	authHeader := h.extractImportAuthHeader(ctx)

	// Extract and validate token
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", ctx.Status(http.StatusUnauthorized).JSON(map[string]any{"error": "Unauthorized"})
	}

	// Validate token and check scope
	return h.validateImportToken(ctx, token)
}

// getImportTestUsername extracts test username from headers
func (h *Handler) getImportTestUsername(ctx *lift.Context) string {
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}
	return testUsername
}

// extractImportAuthHeader extracts authorization header
func (h *Handler) extractImportAuthHeader(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if authHeader == "" {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	return authHeader
}

// validateImportToken validates the token and checks scope
func (h *Handler) validateImportToken(ctx *lift.Context, token string) (string, error) {
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", ctx.Status(http.StatusUnauthorized).JSON(map[string]any{"error": "Unauthorized"})
	}

	if !claims.HasScope(auth.ScopeWrite) {
		return "", ctx.Status(http.StatusForbidden).JSON(map[string]any{"error": "insufficient scope"})
	}

	return claims.Username, nil
}

// parseImportRequest parses the import request body
func (h *Handler) parseImportRequest(ctx *lift.Context) (*ImportRequest, error) {
	var req ImportRequest
	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				return nil, ctx.Status(http.StatusBadRequest).JSON(map[string]any{"error": "invalid request body"})
			}
		} else {
			return nil, ctx.Status(http.StatusBadRequest).JSON(map[string]any{"error": "invalid request body"})
		}
	}

	// Set defaults
	if req.Mode == "" {
		req.Mode = "merge"
	}

	return &req, nil
}

// validateImportParams validates import type and mode
func (h *Handler) validateImportParams(req *ImportRequest) error {
	// Validate import type
	validTypes := map[string]bool{
		"followers": true,
		"following": true,
		"blocks":    true,
		"mutes":     true,
		"lists":     true,
		"bookmarks": true,
	}
	if !validTypes[req.Type] {
		return fmt.Errorf("invalid import type: %s", req.Type)
	}

	// Validate mode
	if req.Mode != "merge" && req.Mode != "overwrite" {
		return fmt.Errorf("mode must be 'merge' or 'overwrite'")
	}

	return nil
}

// processImportFileData decodes and validates the file data
func (h *Handler) processImportFileData(ctx *lift.Context, data string) ([]byte, error) {
	// Decode file data
	fileData, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, ctx.Status(http.StatusBadRequest).JSON(map[string]any{"error": "invalid base64 data"})
	}

	// Validate file size (max 10MB)
	if len(fileData) > 10*1024*1024 {
		return nil, ctx.Status(http.StatusBadRequest).JSON(map[string]any{"error": "file too large (max 10MB)"})
	}

	// Detect and validate file format
	contentType := h.detectContentType(fileData)
	if !h.isValidImportFormat(contentType) {
		return nil, ctx.Status(http.StatusBadRequest).JSON(map[string]any{"error": fmt.Sprintf("unsupported file format: %s", contentType)})
	}

	return fileData, nil
}

// checkExistingImports checks for existing pending/processing imports
func (h *Handler) checkExistingImports(ctx *lift.Context, username, importType string) error {
	existingJobs, err := h.repos.Import().GetUserImportsByStatus(ctx.Context, username, []string{"pending", "processing"})
	if err != nil {
		h.logger.Error("failed to check existing jobs", zap.Error(err))
		return nil // Don't fail on check error
	}

	for _, job := range existingJobs {
		if job.Type == importType {
			return ctx.Status(http.StatusConflict).JSON(map[string]any{"error": "import already in progress for this type"})
		}
	}

	return nil
}

// storeImportFile uploads the import file to S3
func (h *Handler) storeImportFile(ctx *lift.Context, username, importID, importType string, fileData []byte) (string, error) {
	s3Key := fmt.Sprintf("imports/%s/%s/%s.data", username, importID, importType)

	// Initialize S3 client
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx.Context)
	if err != nil {
		h.logger.Error("failed to load AWS config", zap.Error(err))
		return "", ctx.Status(http.StatusInternalServerError).JSON(map[string]any{"error": "failed to initialize S3 client"})
	}
	s3Client := s3.NewFromConfig(awsCfg)

	// Validate bucket configuration
	bucketName := h.cfg.S3BucketName
	if bucketName == "" {
		return "", ctx.Status(http.StatusInternalServerError).JSON(map[string]any{"error": "S3 bucket not configured"})
	}

	// Upload to S3
	putInput := &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(s3Key),
		Body:   bytes.NewReader(fileData),
	}

	_, err = s3Client.PutObject(ctx.Context, putInput)
	if err != nil {
		h.logger.Error("failed to upload import file to S3",
			zap.String("bucket", bucketName),
			zap.String("key", s3Key),
			zap.Error(err))
		return "", ctx.Status(http.StatusInternalServerError).JSON(map[string]any{"error": "failed to store import file"})
	}

	return s3Key, nil
}

// createImportRecord creates the import record and queues processing
func (h *Handler) createImportRecord(ctx *lift.Context, importID, username string, req *ImportRequest, s3Key string) error {
	now := time.Now()

	// Create import record
	importRecord := &models.Import{
		ID:        importID,
		Username:  username,
		Type:      req.Type,
		Mode:      req.Mode,
		Status:    "pending",
		S3Key:     s3Key,
		TTL:       now.Add(30 * 24 * time.Hour).Unix(), // 30 days TTL
		CreatedAt: now,
	}

	if err := h.repos.Import().CreateImport(ctx.Context, importRecord); err != nil {
		h.logger.Error("failed to create import job", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]any{"error": "failed to create import job"})
	}

	// Queue import processing job
	h.queueImportJob(ctx, importID, username, req, s3Key, now)

	return nil
}

// queueImportJob queues the import for processing
func (h *Handler) queueImportJob(ctx *lift.Context, importID, username string, req *ImportRequest, s3Key string, now time.Time) {
	queueJob := map[string]any{
		"importID":  importID,
		"username":  username,
		"type":      req.Type,
		"mode":      req.Mode,
		"s3Key":     s3Key,
		"timestamp": now.Unix(),
	}

	if err := h.repos.Object().CreateObject(ctx.Context, map[string]any{
		"PK":        fmt.Sprintf("IMPORT_QUEUE#%s", importID),
		"SK":        fmt.Sprintf("JOB#%s", importID),
		"Type":      "ImportQueueJob",
		"JobData":   queueJob,
		"CreatedAt": now,
		"TTL":       now.Add(1 * time.Hour).Unix(), // 1 hour TTL for queue items
	}); err != nil {
		h.logger.Error("failed to queue import processor", zap.Error(err))
		// Don't fail the request, import can be retried later
	}
}

// HandleGetImportStatusLift handles GET /api/v1/imports/:id
func (h *Handler) HandleGetImportStatusLift(ctx *lift.Context) error {
	// Test mode support
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
		// Extract and validate token
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			authHeader = ctx.Header("authorization")
		}

		// Try direct access to headers if ctx.Header doesn't work
		if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
			authHeader = ctx.Request.Request.Headers["Authorization"]
			if authHeader == "" {
				authHeader = ctx.Request.Request.Headers["authorization"]
			}
		}

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]any{"error": "Unauthorized"})
		}

		// Validate token
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]any{"error": "Unauthorized"})
		}

		username = claims.Username
	}

	// Get import ID from path parameter
	importID := ctx.Param("id")
	if importID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]any{"error": "missing import ID"})
	}

	// Get import job
	importRec, err := h.repos.Import().GetImport(ctx.Context, importID)
	if err != nil {
		return ctx.Status(http.StatusNotFound).JSON(map[string]any{"error": fmt.Sprintf("import not found: %s", importID)})
	}

	// Verify ownership
	if importRec.Username != username {
		return ctx.Status(http.StatusForbidden).JSON(map[string]any{"error": "not authorized to view this import"})
	}

	// Build response
	job := ImportJob{
		ID:        importRec.ID,
		Status:    importRec.Status,
		Type:      importRec.Type,
		CreatedAt: importRec.CreatedAt.Format(time.RFC3339),
		Processed: importRec.Progress,
	}

	// Add total if available
	if importRec.Total > 0 {
		job.Total = &importRec.Total
	}

	// Add errors if any
	if len(importRec.Errors) > 0 {
		job.Errors = importRec.Errors
	}

	// Add results if completed
	if importRec.Status == statusCompleted {
		job.Results = &Results{
			Success: importRec.SuccessCount,
			Skipped: importRec.SkipCount,
			Failed:  importRec.ErrorCount,
		}
	}

	return ctx.JSON(job)
}

// HandleListImportsLift handles GET /api/v1/imports
func (h *Handler) HandleListImportsLift(ctx *lift.Context) error {
	// Test mode support
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
		// Extract and validate token
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			authHeader = ctx.Header("authorization")
		}

		// Try direct access to headers if ctx.Header doesn't work
		if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
			authHeader = ctx.Request.Request.Headers["Authorization"]
			if authHeader == "" {
				authHeader = ctx.Request.Request.Headers["authorization"]
			}
		}

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]any{"error": "Unauthorized"})
		}

		// Validate token
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]any{"error": "Unauthorized"})
		}

		username = claims.Username
	}

	// Get all user's import jobs (no status filter)
	importRecords, err := h.repos.Import().GetUserImportsByStatus(ctx.Context, username, nil)
	if err != nil {
		h.logger.Error("failed to get import jobs", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]any{"error": "failed to retrieve imports"})
	}

	// Convert to response format
	imports := make([]ImportJob, 0, len(importRecords))
	for _, importRec := range importRecords {
		job := ImportJob{
			ID:        importRec.ID,
			Status:    importRec.Status,
			Type:      importRec.Type,
			CreatedAt: importRec.CreatedAt.Format(time.RFC3339),
			Processed: importRec.Progress,
		}

		// Add total if available
		if importRec.Total > 0 {
			job.Total = &importRec.Total
		}

		// Add errors if any
		if len(importRec.Errors) > 0 {
			job.Errors = importRec.Errors
		}

		// Add results if completed
		if importRec.Status == statusCompleted {
			job.Results = &Results{
				Success: importRec.SuccessCount,
				Skipped: importRec.SkipCount,
				Failed:  importRec.ErrorCount,
			}
		}

		imports = append(imports, job)
	}

	return ctx.JSON(imports)
}

// Helper methods

func (h *Handler) detectContentType(data []byte) string {
	// Simple content type detection
	if len(data) == 0 {
		return "application/octet-stream"
	}

	// Check for JSON
	if data[0] == '{' || data[0] == '[' {
		return "application/json"
	}

	// Check for CSV
	if strings.Contains(string(data[:min(100, len(data))]), ",") {
		return "text/csv"
	}

	return "application/octet-stream"
}

func (h *Handler) isValidImportFormat(contentType string) bool {
	validTypes := []string{
		"application/json",
		"text/csv",
		"text/plain",
	}

	for _, t := range validTypes {
		if strings.HasPrefix(contentType, t) {
			return true
		}
	}
	return false
}
