package lift

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// Export status constants
const (
	ExportStatusCompleted = statusCompleted // Using common constant
	ExportStatusFailed    = "failed"
	ExportTypeArchive     = "archive"
	ExportTypeFollowers   = "followers"
	ExportTypeFollowing   = "following"
)

// HandleCreateExportLift handles POST /api/v1/exports
func (h *Handler) HandleCreateExportLift(ctx *lift.Context) error {
	// Authenticate request
	username, handled, err := h.authenticateExportRequest(ctx)
	if err != nil || handled {
		return err
	}

	// Parse and validate request
	req, handled, err := h.parseExportRequest(ctx)
	if err != nil || handled {
		return err
	}

	// Validate export parameters
	if handled, err := h.validateExportParams(ctx, req); err != nil || handled {
		return err
	}

	// Check for existing exports
	if handled, err := h.checkExistingExports(ctx, username, req.Type); err != nil || handled {
		return err
	}

	// Check rate limits
	if handled, err := h.checkExportRateLimit(ctx, username, req.Type); err != nil || handled {
		return err
	}

	// Check budget limits before creating export
	if handled, err := h.checkExportBudgetLimits(ctx, username, req); err != nil || handled {
		return err
	}

	// Create export job
	exportID := uuid.New().String()
	export, handled, err := h.createExportJob(ctx, exportID, username, req)
	if err != nil || handled {
		return err
	}

	// Queue export for processing
	if err := h.queueExportJobSQS(ctx, exportID, username, req); err != nil {
		h.logger.Error("failed to queue export job", zap.Error(err))
		// Don't fail the request, job can be retried later or processed manually
	}

	// Return job status
	job := apimodels.ExportJob{
		ID:        exportID,
		Status:    "pending",
		Type:      req.Type,
		Format:    req.Format,
		CreatedAt: export.CreatedAt.Format(time.RFC3339),
	}

	return ctx.Status(http.StatusAccepted).JSON(job)
}

// authenticateExportRequest handles authentication for export requests
func (h *Handler) authenticateExportRequest(ctx *lift.Context) (string, bool, error) {
	// Check for test username
	// Extract auth header
	authHeader := h.extractExportAuthHeader(ctx)

	// Extract and validate token
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		_ = common.RespondUnauthorized(ctx)
		return "", true, nil
	}

	// Validate token and check scope
	return h.validateExportToken(ctx, token)
}

// extractExportAuthHeader extracts authorization header
func (h *Handler) extractExportAuthHeader(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		authHeader = ctx.Header("authorization")
	}

	if common.ValidateRequiredParam("authHeader", authHeader) != nil && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if common.ValidateRequiredParam("authHeader", authHeader) != nil {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	return authHeader
}

// validateExportToken validates the token and checks scope using centralized validation
func (h *Handler) validateExportToken(ctx *lift.Context, token string) (string, bool, error) {
	if err := common.ValidateRequiredParam("token", token); err != nil {
		_ = common.RespondUnauthorized(ctx)
		return "", true, nil
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		_ = common.RespondUnauthorized(ctx)
		return "", true, nil
	}

	if !claims.HasScope(auth.ScopeRead) {
		_ = common.RespondInsufficientScope(ctx)
		return "", true, nil
	}

	return claims.Username, false, nil
}

// parseExportRequest parses the export request body
func (h *Handler) parseExportRequest(ctx *lift.Context) (*apimodels.ExportRequest, bool, error) {
	var req apimodels.ExportRequest
	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				_ = common.RespondBadRequest(ctx, "invalid request body")
				return nil, true, nil
			}
		} else {
			_ = common.RespondBadRequest(ctx, "invalid request body")
			return nil, true, nil
		}
	}

	// Set defaults
	if err := common.ValidateRequiredParam("type", req.Type); err != nil {
		req.Type = ExportTypeArchive
	}
	if err := common.ValidateRequiredParam("format", req.Format); err != nil {
		req.Format = "activitypub"
	}

	return &req, false, nil
}

// validateExportParams validates export type and format using centralized validation
func (h *Handler) validateExportParams(ctx *lift.Context, req *apimodels.ExportRequest) (bool, error) {
	// Validate required parameters first
	if err := common.ValidateRequiredParam("type", req.Type); err != nil {
		_ = common.RespondBadRequest(ctx, err.Error())
		return true, nil
	}
	if err := common.ValidateRequiredParam("format", req.Format); err != nil {
		_ = common.RespondBadRequest(ctx, err.Error())
		return true, nil
	}

	// Validate export type
	if !h.isValidExportType(req.Type) {
		_ = common.RespondBadRequest(ctx, fmt.Sprintf("invalid export type: %s", req.Type))
		return true, nil
	}

	// Validate format
	if !h.isValidExportFormat(req.Format) {
		_ = common.RespondBadRequest(ctx, fmt.Sprintf("invalid export format: %s", req.Format))
		return true, nil
	}

	// CSV format is only valid for certain types
	if req.Format == "csv" && req.Type == "archive" {
		_ = common.RespondBadRequest(ctx, "CSV format not available for archive exports")
		return true, nil
	}

	return false, nil
}

// isValidExportType checks if the export type is valid
func (h *Handler) isValidExportType(exportType string) bool {
	validTypes := map[string]bool{
		"archive":   true,
		"followers": true,
		"following": true,
		"blocks":    true,
		"mutes":     true,
		"lists":     true,
		"bookmarks": true,
	}
	return validTypes[exportType]
}

// isValidExportFormat checks if the export format is valid
func (h *Handler) isValidExportFormat(format string) bool {
	validFormats := map[string]bool{
		"activitypub": true,
		"mastodon":    true,
		"csv":         true,
	}
	return validFormats[format]
}

// checkExistingExports checks for existing pending/processing exports
func (h *Handler) checkExistingExports(ctx *lift.Context, username, exportType string) (bool, error) {
	existingJobs, err := h.repos.Export().GetUserExportsByStatus(ctx.Context, username, []string{"pending", "processing"})
	if err != nil {
		h.logger.Error("failed to check existing jobs", zap.Error(err))
		return false, nil // Don't fail on check error
	}

	for _, job := range existingJobs {
		if job.Type == exportType {
			_ = common.RespondConflict(ctx, "export already in progress for this type")
			return true, nil
		}
	}

	return false, nil
}

// createExportJob creates the export record
func (h *Handler) createExportJob(ctx *lift.Context, exportID, username string, req *apimodels.ExportRequest) (*storageModels.Export, bool, error) {
	now := time.Now()

	// Convert date range if provided
	dateRange, handled, err := h.processExportDateRange(ctx, req.DateRange)
	if err != nil || handled {
		return nil, handled, err
	}

	export := &storageModels.Export{
		ID:           exportID,
		Username:     username,
		Type:         req.Type,
		Format:       req.Format,
		Status:       "pending",
		Options:      req.Options,
		IncludeMedia: req.IncludeMedia,
		DateRange:    dateRange,
		CreatedAt:    now,
		TTL:          now.Add(30 * 24 * time.Hour).Unix(), // 30 days TTL
	}

	if err := h.repos.Export().CreateExport(ctx.Context, export); err != nil {
		h.logger.Error("failed to create export job", zap.Error(err))
		_ = common.RespondInternalServerError(ctx, "failed to create export job")
		return nil, true, nil
	}

	return export, false, nil
}

// processExportDateRange processes the date range for exports
func (h *Handler) processExportDateRange(ctx *lift.Context, dateRange *apimodels.ExportDateRange) (*storageModels.ExportDateRange, bool, error) {
	if dateRange == nil {
		return nil, false, nil
	}

	exportDateRange, err := storageModels.NewExportDateRangeFromStrings(dateRange.Start, dateRange.End)
	if err != nil {
		_ = common.RespondBadRequest(ctx, fmt.Sprintf("invalid date range: %v", err))
		return nil, true, nil
	}

	return exportDateRange, false, nil
}

// queueExportJobSQS queues the export job using SQS
func (h *Handler) queueExportJobSQS(ctx *lift.Context, exportID, username string, req *apimodels.ExportRequest) error {
	// Create job queue service with config
	jobQueue, err := services.NewJobQueueService(h.cfg, h.logger)
	if err != nil {
		return errors.Join(failedToCreateJobQueueService(), err)
	}

	// Convert DateRange to service format
	var dateRange *services.ExportDateRange
	if req.DateRange != nil {
		startTime, err := time.Parse(common.DateFormat, req.DateRange.Start)
		if err != nil {
			return errors.Join(invalidStartDate(), err)
		}
		endTime, err := time.Parse(common.DateFormat, req.DateRange.End)
		if err != nil {
			return errors.Join(invalidEndDate(), err)
		}
		dateRange = &services.ExportDateRange{
			Start: startTime,
			End:   endTime,
		}
	}

	// Create export job message
	msg := services.ExportJobMessage{
		ExportID:     exportID,
		Username:     username,
		Type:         req.Type,
		Format:       req.Format,
		IncludeMedia: req.IncludeMedia,
		DateRange:    dateRange,
		Options:      req.Options,
		Timestamp:    time.Now().Unix(),
	}

	// Queue the job
	return jobQueue.QueueExportJob(ctx.Context, msg)
}

// HandleGetExportStatusLift handles GET /api/v1/exports/:id
func (h *Handler) HandleGetExportStatusLift(ctx *lift.Context) error {
	// Authenticate request using consolidated pattern
	username, handled, err := h.authenticateExportStatusRequest(ctx)
	if err != nil || handled {
		return err
	}

	// Get export ID from path parameter
	exportID := ctx.Param("id")
	if err := common.ValidateRequiredParam("id", exportID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Get export job
	export, err := h.repos.Export().GetExport(ctx.Context, exportID)
	if err != nil {
		return common.RespondNotFound(ctx, fmt.Sprintf("export not found: %s", exportID))
	}

	// Verify ownership
	if export.Username != username {
		return common.RespondForbidden(ctx, "not authorized to view this export")
	}

	// Build response
	job := apimodels.ExportJob{
		ID:        export.ID,
		Status:    export.Status,
		Type:      export.Type,
		Format:    export.Format,
		CreatedAt: export.CreatedAt.Format(time.RFC3339),
	}

	// Add completed fields if available
	if export.Status == ExportStatusCompleted {
		if export.DownloadURL != "" {
			job.DownloadURL = &export.DownloadURL
		}
		if export.ExpiresAt != nil {
			expires := export.ExpiresAt.Format(time.RFC3339)
			job.ExpiresAt = &expires
		}
		if export.FileSize > 0 {
			job.FileSize = &export.FileSize
		}
		if export.RecordCount > 0 {
			count := int(export.RecordCount)
			job.RecordCount = &count
		}
	}

	// Add error if failed
	if export.Status == ExportStatusFailed {
		if export.Error != "" {
			job.Error = &export.Error
		}
	}

	return ctx.JSON(job)
}

// HandleListExportsLift handles GET /api/v1/exports
func (h *Handler) HandleListExportsLift(ctx *lift.Context) error {
	// Authenticate user
	username, handled, err := h.authenticateListExportsRequest(ctx)
	if err != nil || handled {
		return err
	}

	// Get user's export jobs
	exportModels, handled, err := h.getUserExports(ctx, username)
	if err != nil || handled {
		return err
	}

	// Convert to response format
	exports := h.convertExportsToResponse(exportModels)

	return ctx.JSON(exports)
}

// authenticateListExportsRequest authenticates the list exports request
func (h *Handler) authenticateListExportsRequest(ctx *lift.Context) (string, bool, error) {
	// Check for test username
	testUsername := h.getListExportsTestUsername(ctx)
	if testUsername != "" {
		return testUsername, false, nil
	}

	// Normal authentication flow
	return h.authenticateListExportsWithToken(ctx)
}

// getListExportsTestUsername extracts test username from headers
func (h *Handler) getListExportsTestUsername(ctx *lift.Context) string {
	if !common.RunningUnitTests() {
		return ""
	}

	username := ctx.Header("X-Test-Username")
	if username == "" {
		username = ctx.Header("x-test-username")
	}

	return username
}

// authenticateListExportsWithToken authenticates using bearer token
func (h *Handler) authenticateListExportsWithToken(ctx *lift.Context) (string, bool, error) {
	// Extract auth header
	authHeader := h.extractListExportsAuthHeader(ctx)

	// Extract and validate token
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		_ = common.RespondUnauthorized(ctx)
		return "", true, nil
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		_ = common.RespondUnauthorized(ctx)
		return "", true, nil
	}

	return claims.Username, false, nil
}

// extractListExportsAuthHeader extracts authorization header
func (h *Handler) extractListExportsAuthHeader(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if common.ValidateRequiredParam("authHeader", authHeader) != nil {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if common.ValidateRequiredParam("authHeader", authHeader) != nil && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if common.ValidateRequiredParam("authHeader", authHeader) != nil {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	return authHeader
}

// getUserExports retrieves the user's export jobs
func (h *Handler) getUserExports(ctx *lift.Context, username string) ([]*storageModels.Export, bool, error) {
	exportModels, err := h.repos.Export().GetUserExportsByStatus(ctx.Context, username, nil)
	if err != nil {
		h.logger.Error("failed to get export jobs", zap.Error(err))
		_ = common.RespondInternalServerError(ctx, "failed to retrieve exports")
		return nil, true, nil
	}
	return exportModels, false, nil
}

// convertExportsToResponse converts export models to API response format
func (h *Handler) convertExportsToResponse(exportModels []*storageModels.Export) []apimodels.ExportJob {
	exports := make([]apimodels.ExportJob, 0, len(exportModels))
	for _, export := range exportModels {
		job := h.convertSingleExportToResponse(export)
		exports = append(exports, job)
	}
	return exports
}

// convertSingleExportToResponse converts a single export to response format
func (h *Handler) convertSingleExportToResponse(export *storageModels.Export) apimodels.ExportJob {
	job := apimodels.ExportJob{
		ID:        export.ID,
		Status:    export.Status,
		Type:      export.Type,
		Format:    export.Format,
		CreatedAt: export.CreatedAt.Format(time.RFC3339),
	}

	// Add status-specific fields
	h.addExportStatusFields(&job, export)

	return job
}

// addExportStatusFields adds status-specific fields to the export job
func (h *Handler) addExportStatusFields(job *apimodels.ExportJob, export *storageModels.Export) {
	switch export.Status {
	case ExportStatusCompleted:
		h.addCompletedExportFields(job, export)
	case ExportStatusFailed:
		h.addFailedExportFields(job, export)
	}
}

// addCompletedExportFields adds fields for completed exports
func (h *Handler) addCompletedExportFields(job *apimodels.ExportJob, export *storageModels.Export) {
	if export.DownloadURL != "" {
		job.DownloadURL = &export.DownloadURL
	}
	if export.ExpiresAt != nil {
		expires := export.ExpiresAt.Format(time.RFC3339)
		job.ExpiresAt = &expires
	}
	if export.FileSize > 0 {
		job.FileSize = &export.FileSize
	}
	if export.RecordCount > 0 {
		count := int(export.RecordCount)
		job.RecordCount = &count
	}
}

// addFailedExportFields adds fields for failed exports
func (h *Handler) addFailedExportFields(job *apimodels.ExportJob, export *storageModels.Export) {
	if export.Error != "" {
		job.Error = &export.Error
	}
}

// HandleDownloadExportLift handles GET /api/v1/exports/:id/download
func (h *Handler) HandleDownloadExportLift(ctx *lift.Context) error {
	// Authenticate request using consolidated pattern
	username, handled, err := h.authenticateExportStatusRequest(ctx)
	if err != nil || handled {
		return err
	}

	// Get export ID from path parameter
	exportID := ctx.Param("id")
	if err := common.ValidateRequiredParam("id", exportID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Get export job
	export, err := h.repos.Export().GetExport(ctx.Context, exportID)
	if err != nil {
		return common.RespondNotFound(ctx, fmt.Sprintf("export not found: %s", exportID))
	}

	// Verify ownership
	if export.Username != username {
		return common.RespondForbidden(ctx, "not authorized to download this export")
	}

	// Check if export is completed
	if export.Status != ExportStatusCompleted {
		return common.RespondConflict(ctx, fmt.Sprintf("export not ready (status: %s)", export.Status))
	}

	// Check if download URL is available and not expired
	if err := common.ValidateRequiredParam("downloadURL", export.DownloadURL); err != nil {
		return common.RespondGone(ctx, "download URL not available")
	}

	if export.ExpiresAt != nil && time.Now().After(*export.ExpiresAt) {
		return common.RespondGone(ctx, "download URL has expired")
	}

	// Redirect to the pre-signed S3 URL
	ctx.Set("Location", export.DownloadURL)

	var expiresAt *string
	if export.ExpiresAt != nil {
		v := export.ExpiresAt.Format(time.RFC3339)
		expiresAt = &v
	}

	return ctx.Status(http.StatusFound).JSON(apimodels.ExportDownloadResponse{
		DownloadURL: export.DownloadURL,
		ExpiresAt:   expiresAt,
	})
}

// checkExportBudgetLimits validates that the user has not exceeded their export budget limits
func (h *Handler) checkExportBudgetLimits(ctx *lift.Context, username string, req *apimodels.ExportRequest) (bool, error) {
	// Get import repository to access budget methods
	importRepo := h.repos.Import()

	// Estimate export cost (simplified estimation)
	estimatedCost := h.estimateExportCost(req)

	// Check budget limits (import cost = 0 for exports)
	budget, withinLimits, err := importRepo.CheckBudgetLimits(ctx.Context, username, 0, estimatedCost)
	if err != nil {
		h.logger.Warn("failed to check budget limits, allowing export", zap.Error(err))
		return false, nil // Don't block on budget check errors
	}

	if !withinLimits {
		var limitType string
		var remaining int64

		if budget.IsExportOverLimit(estimatedCost) {
			limitType = "export"
			remaining = budget.GetRemainingExportBudget()
		} else if budget.IsCombinedOverLimit(0, estimatedCost) {
			limitType = "combined"
			remaining = budget.GetRemainingCombinedBudget()
		}

		_ = ctx.Status(http.StatusPaymentRequired).JSON(map[string]any{
			"error":            fmt.Sprintf("%s budget limit exceeded", limitType),
			"estimated_cost":   float64(estimatedCost) / 1_000_000.0, // Convert to dollars
			"remaining_budget": float64(remaining) / 1_000_000.0,     // Convert to dollars
			"budget_period":    budget.Period,
			"budget_resets_at": budget.NextResetAt.Format(time.RFC3339),
		})
		return true, nil
	}

	return false, nil
}

// estimateExportCost provides a rough cost estimate for an export operation
func (h *Handler) estimateExportCost(req *apimodels.ExportRequest) int64 {
	baseCost := int64(50000) // $0.05 base cost in microcents

	// Adjust cost based on export type
	switch req.Type {
	case ExportTypeArchive:
		baseCost *= 10 // Archive exports are more expensive
	case ExportTypeFollowers, ExportTypeFollowing:
		baseCost *= 3 // Relationship exports are moderately expensive
	}

	// Adjust cost based on format
	switch req.Format {
	case "activitypub", "mastodon":
		baseCost *= 2 // JSON formats are more expensive than CSV
	}

	// Media inclusion significantly increases cost
	if req.IncludeMedia {
		baseCost *= 5
	}

	return baseCost
}

// authenticateExportStatusRequest handles authentication for export status/download requests
// This consolidates the duplicate authentication logic from HandleGetExportStatusLift and HandleDownloadExportLift
func (h *Handler) authenticateExportStatusRequest(ctx *lift.Context) (string, bool, error) {
	// Check for test username
	// Extract auth header
	authHeader := h.extractExportAuthHeader(ctx)

	// Extract and validate token
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		_ = common.RespondUnauthorized(ctx)
		return "", true, nil
	}

	// Validate token (no scope check needed for read operations)
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		_ = common.RespondUnauthorized(ctx)
		return "", true, nil
	}

	return claims.Username, false, nil
}

// checkExportRateLimit validates rate limits for export operations
func (h *Handler) checkExportRateLimit(ctx *lift.Context, username string, exportType string) (bool, error) {
	// Basic rate limiting - check for existing pending exports
	existingJobs, err := h.repos.Export().GetUserExportsByStatus(ctx.Context, username, []string{"pending", "processing"})
	if err != nil {
		h.logger.Warn("failed to check existing jobs for rate limiting", zap.Error(err))
		return false, nil // Don't block on check error
	}

	// Count exports of the same type in the last hour
	recentCount := 0
	oneHourAgo := time.Now().Add(-time.Hour)
	for _, job := range existingJobs {
		if job.Type == exportType && job.CreatedAt.After(oneHourAgo) {
			recentCount++
		}
	}

	// Allow 1 export per hour per type for regular users
	if recentCount >= 1 {
		_ = ctx.Status(http.StatusTooManyRequests).JSON(map[string]any{
			"error":          "rate limit exceeded",
			"limit":          1,
			"window_seconds": 3600,
			"retry_after":    3600,
		})
		return true, nil
	}

	return false, nil
}
