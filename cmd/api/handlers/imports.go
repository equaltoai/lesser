package handlers

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// Import status constants
const (
	ImportStatusFailed    = "failed"
	ImportStatusCancelled = "cancelled"
)

// HandleCreateImportLift handles POST /api/v1/imports
func (h *Handler) HandleCreateImportLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate request
	username, err := h.authenticateUser(ctx, []string{auth.ScopeWrite})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Parse and validate request
	req, resp, err := h.parseImportRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Validate import parameters
	if err := h.validateImportParams(req); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Process file data
	fileData, resp, err := h.processImportFileData(ctx, req.Data)
	if resp != nil || err != nil {
		return resp, err
	}

	// Check for existing imports
	if resp, err := h.checkExistingImports(ctx, username, req.Type); resp != nil || err != nil {
		return resp, err
	}

	// Check rate limits
	if resp, err := h.checkImportRateLimit(ctx, username, req.Type); resp != nil || err != nil {
		return resp, err
	}

	// Check budget limits before creating import
	if resp, err := h.checkImportBudgetLimits(ctx, username, req, len(fileData)); resp != nil || err != nil {
		return resp, err
	}

	// Create and store import
	importID := uuid.New().String()
	s3Key, resp, err := h.storeImportFile(ctx, username, importID, req.Type, fileData)
	if resp != nil || err != nil {
		return resp, err
	}

	// Create import record and queue job
	if resp, err := h.createImportRecord(ctx, importID, username, req, s3Key); resp != nil || err != nil {
		return resp, err
	}

	// Return job status
	job := apimodels.ImportJob{
		ID:        importID,
		Status:    "pending",
		Type:      req.Type,
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	return apptheory.JSON(http.StatusAccepted, job)
}

// authenticateImportRequest handles authentication for import requests
func (h *Handler) authenticateImportRequest(ctx *apptheory.Context) (string, error) {
	// Extract auth header
	authHeader := h.extractImportAuthHeader(ctx)

	// Extract and validate token
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", helperUnauthorized()
	}

	// Validate token and check scope
	return h.validateImportToken(ctx, token)
}

// extractImportAuthHeader extracts authorization header
func (h *Handler) extractImportAuthHeader(ctx *apptheory.Context) string {
	return common.ExtractAuthHeader(ctx)
}

// validateImportToken validates the token and checks scope using centralized validation
func (h *Handler) validateImportToken(ctx *apptheory.Context, token string) (string, error) {
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return "", helperUnauthorized()
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", helperUnauthorized()
	}

	if !claims.HasScope(auth.ScopeWrite) {
		return "", helperInsufficientScope()
	}

	return claims.Username, nil
}

// parseImportRequest parses the import request body
func (h *Handler) parseImportRequest(ctx *apptheory.Context) (*apimodels.ImportRequest, *apptheory.Response, error) {
	var req apimodels.ImportRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		resp, respErr := common.RespondBadRequest(ctx, "invalid request body")
		return nil, resp, respErr
	}

	// Set defaults
	if err := common.ValidateRequiredParam("mode", req.Mode); err != nil {
		req.Mode = "merge"
	}

	return &req, nil, nil
}

// validateImportParams validates import type and mode using centralized validation
func (h *Handler) validateImportParams(req *apimodels.ImportRequest) error {
	// Validate required parameters first
	if err := common.ValidateRequiredParam("type", req.Type); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("mode", req.Mode); err != nil {
		return err
	}

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
		h.logger.Error("invalid import type provided",
			zap.String("type", req.Type),
			zap.Strings("valid_types", []string{"followers", "following", "blocks", "mutes", "lists", "bookmarks"}))
		return invalidImportType()
	}

	// Validate mode
	if req.Mode != "merge" && req.Mode != "overwrite" {
		return invalidImportMode()
	}

	return nil
}

// processImportFileData decodes and validates the file data using centralized validation
func (h *Handler) processImportFileData(ctx *apptheory.Context, data string) ([]byte, *apptheory.Response, error) {
	// Validate required data parameter
	if err := common.ValidateRequiredParam("data", data); err != nil {
		resp, respErr := common.RespondBadRequest(ctx, err.Error())
		return nil, resp, respErr
	}

	// Decode file data
	fileData, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		resp, respErr := common.RespondBadRequest(ctx, "invalid base64 data")
		return nil, resp, respErr
	}

	// Validate file size (max 10MB)
	if err := common.ValidateIntRange("file_size", len(fileData), 1, 10*1024*1024); err != nil {
		resp, respErr := common.RespondBadRequest(ctx, "file too large (max 10MB)")
		return nil, resp, respErr
	}

	// Perform comprehensive file validation
	if resp, err := h.validateImportFile(ctx, fileData, "unknown"); resp != nil || err != nil {
		return nil, resp, err
	}

	return fileData, nil, nil
}

// checkExistingImports checks for existing pending/processing imports
func (h *Handler) checkExistingImports(ctx *apptheory.Context, username, importType string) (*apptheory.Response, error) {
	existingJobs, err := h.repos.Import().GetUserImportsByStatus(ctx.Context(), username, []string{"pending", "processing"})
	if err != nil {
		h.logger.Error("failed to check existing jobs", zap.Error(err))
		return nil, nil // Don't fail on check error
	}

	for _, job := range existingJobs {
		if job.Type == importType {
			return common.RespondConflict(ctx, "import already in progress for this type")
		}
	}

	return nil, nil
}

// storeImportFile uploads the import file to S3
func (h *Handler) storeImportFile(ctx *apptheory.Context, username, importID, importType string, fileData []byte) (string, *apptheory.Response, error) {
	s3Key := fmt.Sprintf("imports/%s/%s/%s.data", username, importID, importType)

	// Initialize S3 client
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx.Context())
	if err != nil {
		h.logger.Error("failed to load AWS config", zap.Error(err))
		resp, respErr := common.RespondInternalServerError(ctx, "failed to initialize S3 client")
		return "", resp, respErr
	}
	s3Client := s3.NewFromConfig(awsCfg)

	// Validate bucket configuration
	bucketName := h.cfg.S3BucketName
	if err := common.ValidateRequiredParam("bucketName", bucketName); err != nil {
		resp, respErr := common.RespondInternalServerError(ctx, "S3 bucket not configured")
		return "", resp, respErr
	}

	// Upload to S3
	putInput := &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(s3Key),
		Body:   bytes.NewReader(fileData),
	}

	_, err = s3Client.PutObject(ctx.Context(), putInput)
	if err != nil {
		h.logger.Error("failed to upload import file to S3",
			zap.String("bucket", bucketName),
			zap.String("key", s3Key),
			zap.Error(err))
		resp, respErr := common.RespondInternalServerError(ctx, "failed to store import file")
		return "", resp, respErr
	}

	return s3Key, nil, nil
}

// createImportRecord creates the import record and queues processing
func (h *Handler) createImportRecord(ctx *apptheory.Context, importID, username string, req *apimodels.ImportRequest, s3Key string) (*apptheory.Response, error) {
	now := time.Now()

	// Create import record
	importRecord := &storageModels.Import{
		ID:        importID,
		Username:  username,
		Type:      req.Type,
		Mode:      req.Mode,
		Status:    "pending",
		S3Key:     s3Key,
		TTL:       now.Add(30 * 24 * time.Hour).Unix(), // 30 days TTL
		CreatedAt: now,
	}

	if err := h.repos.Import().CreateImport(ctx.Context(), importRecord); err != nil {
		h.logger.Error("failed to create import job", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to create import job")
	}

	// Queue import processing job
	if err := h.queueImportJobSQS(ctx, importID, username, req, s3Key); err != nil {
		h.logger.Error("failed to queue import job", zap.Error(err))
		// Don't fail the request, job can be retried later or processed manually
	}

	return nil, nil
}

// queueImportJobSQS queues the import job using SQS
func (h *Handler) queueImportJobSQS(ctx *apptheory.Context, importID, username string, req *apimodels.ImportRequest, s3Key string) error {
	// Create job queue service with config
	jobQueue, err := services.NewJobQueueService(h.cfg, h.logger)
	if err != nil {
		h.logger.Error("failed to create job queue service",
			zap.String("import_id", importID),
			zap.String("username", username),
			zap.Error(err))
		return errors.Join(jobQueueServiceCreationFailed(), err)
	}

	// Create import job message
	msg := services.ImportJobMessage{
		ImportID:  importID,
		Username:  username,
		Type:      req.Type,
		Mode:      req.Mode,
		S3Key:     s3Key,
		Timestamp: time.Now().Unix(),
		Options:   make(map[string]any), // Could be extended for future options
	}

	// Queue the job
	return jobQueue.QueueImportJob(ctx.Context(), msg)
}

// HandleGetImportStatusLift handles GET /api/v1/imports/:id
func (h *Handler) HandleGetImportStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate request using consolidated pattern
	username, err := h.authenticateUser(ctx, nil)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	// Get import ID from path parameter
	importID := ctx.Param("id")
	if err := common.ValidateRequiredParam("id", importID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Get import job
	importRec, err := h.repos.Import().GetImport(ctx.Context(), importID)
	if err != nil {
		return common.RespondNotFound(ctx, fmt.Sprintf("import not found: %s", importID))
	}

	// Verify ownership
	if importRec.Username != username {
		return common.RespondForbidden(ctx, "not authorized to view this import")
	}

	// Build response
	job := apimodels.ImportJob{
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
	if err := common.ValidateSliceNotEmpty("errors", importRec.Errors); err == nil {
		job.Errors = importRec.Errors
	}

	// Add results if completed
	if importRec.Status == statusCompleted {
		job.Results = &apimodels.ImportResults{
			Success: importRec.SuccessCount,
			Skipped: importRec.SkipCount,
			Failed:  importRec.ErrorCount,
		}
	}

	return okJSON(job)
}

// HandleListImportsLift handles GET /api/v1/imports
func (h *Handler) HandleListImportsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate request using consolidated pattern
	username, err := h.authenticateUser(ctx, nil)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	// Get all user's import jobs (no status filter)
	importRecords, err := h.repos.Import().GetUserImportsByStatus(ctx.Context(), username, nil)
	if err != nil {
		h.logger.Error("failed to get import jobs", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to retrieve imports")
	}

	// Convert to response format
	imports := make([]apimodels.ImportJob, 0, len(importRecords))
	for _, importRec := range importRecords {
		job := apimodels.ImportJob{
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
		if err := common.ValidateSliceNotEmpty("errors", importRec.Errors); err == nil {
			job.Errors = importRec.Errors
		}

		// Add results if completed
		if importRec.Status == statusCompleted {
			job.Results = &apimodels.ImportResults{
				Success: importRec.SuccessCount,
				Skipped: importRec.SkipCount,
				Failed:  importRec.ErrorCount,
			}
		}

		imports = append(imports, job)
	}

	return okJSON(imports)
}

// HandleCancelImportLift handles DELETE /api/v1/imports/:id
func (h *Handler) HandleCancelImportLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	username, err := h.authenticateUser(ctx, []string{auth.ScopeWrite})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Get import ID from path parameter
	importID := ctx.Param("id")
	if err := common.ValidateRequiredParam("id", importID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Get import job
	importRec, err := h.repos.Import().GetImport(ctx.Context(), importID)
	if err != nil {
		return common.RespondNotFound(ctx, fmt.Sprintf("import not found: %s", importID))
	}

	// Verify ownership
	if importRec.Username != username {
		return common.RespondForbidden(ctx, "not authorized to cancel this import")
	}

	// Check if import can be cancelled
	if importRec.Status == statusCompleted {
		return common.RespondConflict(ctx, "import already completed")
	}

	if importRec.Status == ImportStatusFailed {
		return common.RespondConflict(ctx, "import already failed")
	}

	if importRec.Status == ImportStatusCancelled {
		return common.RespondConflict(ctx, "import already cancelled")
	}

	// Update import status to cancelled
	if err := h.repos.Import().UpdateImportStatus(ctx.Context(), importID, "cancelled", nil, "cancelled by user"); err != nil {
		h.logger.Error("failed to cancel import", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to cancel import")
	}

	h.logger.Info("import cancelled by user",
		zap.String("import_id", importID),
		zap.String("username", username))

	// Return updated job status
	job := apimodels.ImportJob{
		ID:        importRec.ID,
		Status:    "cancelled",
		Type:      importRec.Type,
		CreatedAt: importRec.CreatedAt.Format(time.RFC3339),
		Processed: importRec.Progress,
	}

	// Add total if available
	if importRec.Total > 0 {
		job.Total = &importRec.Total
	}

	return okJSON(job)
}

// Helper methods

func (h *Handler) detectContentType(data []byte) string {
	// Simple content type detection
	if err := common.ValidateSliceNotEmpty("data", data); err != nil {
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

// checkImportRateLimit validates rate limits for import operations
func (h *Handler) checkImportRateLimit(ctx *apptheory.Context, username string, importType string) (*apptheory.Response, error) {
	// Basic rate limiting - check for existing pending imports
	existingJobs, err := h.repos.Import().GetUserImportsByStatus(ctx.Context(), username, []string{"pending", "processing"})
	if err != nil {
		h.logger.Warn("failed to check existing jobs for rate limiting", zap.Error(err))
		return nil, nil // Don't block on check error
	}

	// Count imports of the same type in the last day
	recentCount := 0
	oneDayAgo := time.Now().Add(-24 * time.Hour)
	for _, job := range existingJobs {
		if job.Type == importType && job.CreatedAt.After(oneDayAgo) {
			recentCount++
		}
	}

	// Allow 1 import per day per type for regular users (more restrictive than exports)
	if recentCount >= 1 {
		return apptheory.JSON(http.StatusTooManyRequests, map[string]any{
			"error":          "rate limit exceeded",
			"limit":          1,
			"window_seconds": 86400, // 24 hours
			"retry_after":    86400,
		})
	}

	return nil, nil
}

// checkImportBudgetLimits validates that the user has not exceeded their import budget limits
func (h *Handler) checkImportBudgetLimits(ctx *apptheory.Context, username string, req *apimodels.ImportRequest, fileSize int) (*apptheory.Response, error) {
	// Get import repository to access budget methods
	importRepo := h.repos.Import()

	// Estimate import cost based on file size and type
	estimatedCost := h.estimateImportCost(req, fileSize)

	// Check budget limits (export cost = 0 for imports)
	budget, withinLimits, err := importRepo.CheckBudgetLimits(ctx.Context(), username, estimatedCost, 0)
	if err != nil {
		h.logger.Warn("failed to check budget limits, allowing import", zap.Error(err))
		return nil, nil // Don't block on budget check errors
	}

	if !withinLimits {
		var limitType string
		var remaining int64

		if budget.IsImportOverLimit(estimatedCost) {
			limitType = "import"
			remaining = budget.GetRemainingImportBudget()
		} else if budget.IsCombinedOverLimit(estimatedCost, 0) {
			limitType = "combined"
			remaining = budget.GetRemainingCombinedBudget()
		}

		return apptheory.JSON(http.StatusPaymentRequired, map[string]any{
			"error":            fmt.Sprintf("%s budget limit exceeded", limitType),
			"estimated_cost":   float64(estimatedCost) / 1_000_000.0, // Convert to dollars
			"remaining_budget": float64(remaining) / 1_000_000.0,     // Convert to dollars
			"budget_period":    budget.Period,
			"budget_resets_at": budget.NextResetAt.Format(time.RFC3339),
		})
	}

	return nil, nil
}

// estimateImportCost provides a rough cost estimate for an import operation
func (h *Handler) estimateImportCost(req *apimodels.ImportRequest, fileSize int) int64 {
	baseCost := int64(30000) // $0.03 base cost in microcents

	// Scale cost based on file size (per KB)
	fileSizeKB := int64(fileSize) / 1024
	if fileSizeKB < 1 {
		fileSizeKB = 1
	}
	sizeCost := fileSizeKB * 1000 // $0.001 per KB

	// Adjust cost based on import type
	switch req.Type {
	case ExportTypeFollowers, ExportTypeFollowing:
		baseCost *= 3 // Relationship imports require external lookups
	case "lists":
		baseCost *= 2 // List processing is moderately expensive
	case "bookmarks":
		baseCost *= 2 // Bookmark processing requires object resolution
	}

	// Overwrite mode costs more than merge
	if req.Mode == "overwrite" {
		baseCost *= 2
	}

	return baseCost + sizeCost
}

// validateImportFile performs comprehensive file validation
func (h *Handler) validateImportFile(ctx *apptheory.Context, data []byte, importType string) (*apptheory.Response, error) {
	// Create file validation service
	fileValidator, err := services.NewFileValidationService(h.logger)
	if err != nil {
		h.logger.Warn("failed to create file validator, skipping advanced validation", zap.Error(err))
		// Fall back to basic validation
		if err := h.basicFileValidation(data); err != nil {
			return common.RespondValidationError(ctx, err)
		}
		return nil, nil
	}

	// Configure validation based on import type
	config := services.DefaultImportValidationConfig()

	// Adjust limits based on import type
	switch importType {
	case ExportTypeFollowers, ExportTypeFollowing:
		config.MaxSizeBytes = 50 * 1024 * 1024 // 50MB for relationship lists
	case "blocks", "mutes":
		config.MaxSizeBytes = 10 * 1024 * 1024 // 10MB for smaller lists
	case "bookmarks", "lists":
		config.MaxSizeBytes = 20 * 1024 * 1024 // 20MB for medium lists
	}

	// Perform validation
	result, err := fileValidator.ValidateFile(ctx.Context(), data, config)
	if err != nil {
		h.logger.Error("file validation failed", zap.Error(err))
		return common.RespondInternalServerError(ctx, "file validation failed")
	}

	// Check validation result
	if !result.Valid {
		return apptheory.JSON(http.StatusBadRequest, map[string]any{
			"error":    "file validation failed",
			"details":  result.Errors,
			"warnings": result.Warnings,
		})
	}

	// Log warnings if any
	if err := common.ValidateSliceNotEmpty("warnings", result.Warnings); err == nil {
		h.logger.Warn("file validation warnings",
			zap.String("import_type", importType),
			zap.Strings("warnings", result.Warnings))
	}

	// Log validation metadata
	h.logger.Info("file validation successful",
		zap.String("import_type", importType),
		zap.String("content_type", result.ContentType),
		zap.String("detected_format", result.DetectedFormat),
		zap.Int64("file_size", result.FileSize),
		zap.String("md5_hash", result.MD5Hash))

	return nil, nil
}

// basicFileValidation performs basic validation as fallback
func (h *Handler) basicFileValidation(data []byte) error {
	// Basic content type detection
	contentType := h.detectContentType(data)
	if !h.isValidImportFormat(contentType) {
		h.logger.Error("unsupported file format detected",
			zap.String("content_type", contentType),
			zap.Strings("supported_formats", []string{"application/json", "text/csv", "text/plain"}))
		return unsupportedFileFormat()
	}
	return nil
}

// authenticateImportStatusRequest handles authentication for import status/list requests
// This consolidates the duplicate authentication logic from HandleGetImportStatusLift and HandleListImportsLift
func (h *Handler) authenticateImportStatusRequest(ctx *apptheory.Context) (string, error) {
	// Extract auth header
	authHeader := h.extractImportAuthHeader(ctx)

	// Extract and validate token
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", helperUnauthorized()
	}

	// Validate token (no scope check needed for read operations)
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", helperUnauthorized()
	}

	return claims.Username, nil
}
