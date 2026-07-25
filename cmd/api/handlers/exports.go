package handlers

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
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
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
func (h *Handler) HandleCreateExportLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate request
	username, resp, err := h.authenticateExportRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Parse and validate request
	req, resp, err := h.parseExportRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Validate export parameters
	resp, err = h.validateExportParams(ctx, req)
	if resp != nil || err != nil {
		return resp, err
	}

	// Check for existing exports
	resp, err = h.checkExistingExports(ctx, username, req.Type)
	if resp != nil || err != nil {
		return resp, err
	}

	// Check rate limits
	resp, err = h.checkExportRateLimit(ctx, username, req.Type)
	if resp != nil || err != nil {
		return resp, err
	}

	// Check budget limits before creating export
	resp, err = h.checkExportBudgetLimits(ctx, username, req)
	if resp != nil || err != nil {
		return resp, err
	}

	// Create export job
	exportID := uuid.New().String()
	export, resp, err := h.createExportJob(ctx, exportID, username, req)
	if resp != nil || err != nil {
		return resp, err
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

	return apptheory.JSON(http.StatusAccepted, job)
}

// authenticateExportRequest handles authentication for export requests
func (h *Handler) authenticateExportRequest(ctx *apptheory.Context) (string, *apptheory.Response, error) {
	// Check for test username
	// Extract auth header
	authHeader := h.extractExportAuthHeader(ctx)

	// Extract and validate token
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		resp, respErr := common.RespondUnauthorized(ctx)
		return "", resp, respErr
	}

	// Validate token and check scope
	return h.validateExportToken(ctx, token)
}

// extractExportAuthHeader extracts authorization header
func (h *Handler) extractExportAuthHeader(ctx *apptheory.Context) string {
	authHeader := headerValue(ctx, "Authorization")
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		authHeader = headerValue(ctx, "authorization")
	}

	return authHeader
}

// validateExportToken validates the token and checks scope using centralized validation
func (h *Handler) validateExportToken(ctx *apptheory.Context, token string) (string, *apptheory.Response, error) {
	if err := common.ValidateRequiredParam("token", token); err != nil {
		resp, respErr := common.RespondUnauthorized(ctx)
		return "", resp, respErr
	}

	claims, resp, err := h.authenticatedClaimsWithResponder(
		ctx,
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
	)
	if resp != nil || err != nil {
		resp, respErr := common.RespondUnauthorized(ctx)
		return "", resp, respErr
	}

	if !claims.HasScope(auth.ScopeRead) {
		resp, respErr := common.RespondInsufficientScope(ctx)
		return "", resp, respErr
	}

	return claims.Username, nil, nil
}

// parseExportRequest parses the export request body
func (h *Handler) parseExportRequest(ctx *apptheory.Context) (*apimodels.ExportRequest, *apptheory.Response, error) {
	var req apimodels.ExportRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		// Fallback for test environments
		if len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				resp, respErr := common.RespondBadRequest(ctx, "invalid request body")
				return nil, resp, respErr
			}
		} else {
			resp, respErr := common.RespondBadRequest(ctx, "invalid request body")
			return nil, resp, respErr
		}
	}

	// Set defaults
	if err := common.ValidateRequiredParam("type", req.Type); err != nil {
		req.Type = ExportTypeArchive
	}
	if err := common.ValidateRequiredParam("format", req.Format); err != nil {
		req.Format = "activitypub"
	}

	return &req, nil, nil
}

// validateExportParams validates export type and format using centralized validation
func (h *Handler) validateExportParams(ctx *apptheory.Context, req *apimodels.ExportRequest) (*apptheory.Response, error) {
	// Validate required parameters first
	if err := common.ValidateRequiredParam("type", req.Type); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}
	if err := common.ValidateRequiredParam("format", req.Format); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Validate export type
	if !h.isValidExportType(req.Type) {
		return common.RespondBadRequest(ctx, fmt.Sprintf("invalid export type: %s", req.Type))
	}

	// Validate format
	if !h.isValidExportFormat(req.Format) {
		return common.RespondBadRequest(ctx, fmt.Sprintf("invalid export format: %s", req.Format))
	}

	// CSV format is only valid for certain types
	if req.Format == "csv" && req.Type == "archive" {
		return common.RespondBadRequest(ctx, "CSV format not available for archive exports")
	}

	return nil, nil
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
func (h *Handler) checkExistingExports(ctx *apptheory.Context, username, exportType string) (*apptheory.Response, error) {
	existingJobs, err := h.repos.Export().GetUserExportsByStatus(ctx.Context(), username, []string{"pending", "processing"})
	if err != nil {
		h.logger.Error("failed to check existing jobs", zap.Error(err))
		return nil, nil // Don't fail on check error
	}

	for _, job := range existingJobs {
		if job.Type == exportType {
			return common.RespondConflict(ctx, "export already in progress for this type")
		}
	}

	return nil, nil
}

// createExportJob creates the export record
func (h *Handler) createExportJob(ctx *apptheory.Context, exportID, username string, req *apimodels.ExportRequest) (*storageModels.Export, *apptheory.Response, error) {
	now := time.Now()

	// Convert date range if provided
	dateRange, resp, err := h.processExportDateRange(ctx, req.DateRange)
	if resp != nil || err != nil {
		return nil, resp, err
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

	if err := h.repos.Export().CreateExport(ctx.Context(), export); err != nil {
		h.logger.Error("failed to create export job", zap.Error(err))
		resp, respErr := common.RespondInternalServerError(ctx, "failed to create export job")
		return nil, resp, respErr
	}

	return export, nil, nil
}

// processExportDateRange processes the date range for exports
func (h *Handler) processExportDateRange(ctx *apptheory.Context, dateRange *apimodels.ExportDateRange) (*storageModels.ExportDateRange, *apptheory.Response, error) {
	if dateRange == nil {
		return nil, nil, nil
	}

	exportDateRange, err := storageModels.NewExportDateRangeFromStrings(dateRange.Start, dateRange.End)
	if err != nil {
		resp, respErr := common.RespondBadRequest(ctx, fmt.Sprintf("invalid date range: %v", err))
		return nil, resp, respErr
	}

	return exportDateRange, nil, nil
}

// queueExportJobSQS queues the export job using SQS
func (h *Handler) queueExportJobSQS(ctx *apptheory.Context, exportID, username string, req *apimodels.ExportRequest) error {
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
	return jobQueue.QueueExportJob(ctx.Context(), msg)
}

// HandleGetExportStatusLift handles GET /api/v1/exports/:id
func (h *Handler) HandleGetExportStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate request using consolidated pattern
	username, resp, err := h.authenticateExportStatusRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Get export ID from path parameter
	exportID := ctx.Param("id")
	if err := common.ValidateRequiredParam("id", exportID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Get export job
	export, err := h.repos.Export().GetExport(ctx.Context(), exportID)
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

	return okJSON(job)
}

// HandleListExportsLift handles GET /api/v1/exports
func (h *Handler) HandleListExportsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user
	username, resp, err := h.authenticateListExportsRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Get user's export jobs
	exportModels, resp, err := h.getUserExports(ctx, username)
	if resp != nil || err != nil {
		return resp, err
	}

	// Convert to response format
	exports := h.convertExportsToResponse(exportModels)

	return okJSON(exports)
}

// authenticateListExportsRequest authenticates the list exports request
func (h *Handler) authenticateListExportsRequest(ctx *apptheory.Context) (string, *apptheory.Response, error) {
	// Check for test username
	testUsername := h.getListExportsTestUsername(ctx)
	if testUsername != "" {
		return testUsername, nil, nil
	}

	// Normal authentication flow
	return h.authenticateListExportsWithToken(ctx)
}

// getListExportsTestUsername extracts test username from headers
func (h *Handler) getListExportsTestUsername(ctx *apptheory.Context) string {
	if !common.RunningUnitTests() {
		return ""
	}

	username := headerValue(ctx, "X-Test-Username")
	if username == "" {
		username = headerValue(ctx, "x-test-username")
	}

	return username
}

// authenticateListExportsWithToken authenticates using bearer token
func (h *Handler) authenticateListExportsWithToken(ctx *apptheory.Context) (string, *apptheory.Response, error) {
	// Extract auth header
	authHeader := h.extractListExportsAuthHeader(ctx)

	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		resp, respErr := common.RespondUnauthorized(ctx)
		return "", resp, respErr
	}

	claims, resp, err := h.authenticatedClaimsWithResponder(
		ctx,
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
	)
	if resp != nil || err != nil {
		resp, respErr := common.RespondUnauthorized(ctx)
		return "", resp, respErr
	}

	return claims.Username, nil, nil
}

// extractListExportsAuthHeader extracts authorization header
func (h *Handler) extractListExportsAuthHeader(ctx *apptheory.Context) string {
	authHeader := headerValue(ctx, "Authorization")
	if common.ValidateRequiredParam("authHeader", authHeader) != nil {
		authHeader = headerValue(ctx, "authorization")
	}

	return authHeader
}

// getUserExports retrieves the user's export jobs
func (h *Handler) getUserExports(ctx *apptheory.Context, username string) ([]*storageModels.Export, *apptheory.Response, error) {
	exportModels, err := h.repos.Export().GetUserExportsByStatus(ctx.Context(), username, nil)
	if err != nil {
		h.logger.Error("failed to get export jobs", zap.Error(err))
		resp, respErr := common.RespondInternalServerError(ctx, "failed to retrieve exports")
		return nil, resp, respErr
	}
	return exportModels, nil, nil
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
func (h *Handler) HandleDownloadExportLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate request using consolidated pattern
	username, resp, err := h.authenticateExportStatusRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Get export ID from path parameter
	exportID := ctx.Param("id")
	if err := common.ValidateRequiredParam("id", exportID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Get export job
	export, err := h.repos.Export().GetExport(ctx.Context(), exportID)
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

	var expiresAt *string
	if export.ExpiresAt != nil {
		v := export.ExpiresAt.Format(time.RFC3339)
		expiresAt = &v
	}

	resp, err = apptheory.JSON(http.StatusFound, apimodels.ExportDownloadResponse{
		DownloadURL: export.DownloadURL,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return nil, err
	}
	setHeader(resp, "Location", export.DownloadURL)
	return resp, nil
}

// checkExportBudgetLimits validates that the user has not exceeded their export budget limits
func (h *Handler) checkExportBudgetLimits(ctx *apptheory.Context, username string, req *apimodels.ExportRequest) (*apptheory.Response, error) {
	// Get import repository to access budget methods
	importRepo := h.repos.Import()

	// Estimate export cost (simplified estimation)
	estimatedCost := h.estimateExportCost(req)

	// Check budget limits (import cost = 0 for exports)
	budget, withinLimits, err := importRepo.CheckBudgetLimits(ctx.Context(), username, 0, estimatedCost)
	if err != nil {
		h.logger.Warn("failed to check budget limits, allowing export", zap.Error(err))
		return nil, nil // Don't block on budget check errors
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
func (h *Handler) authenticateExportStatusRequest(ctx *apptheory.Context) (string, *apptheory.Response, error) {
	// Check for test username
	// Extract auth header
	authHeader := h.extractExportAuthHeader(ctx)

	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		resp, respErr := common.RespondUnauthorized(ctx)
		return "", resp, respErr
	}

	claims, resp, err := h.authenticatedClaimsWithResponder(
		ctx,
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
	)
	if resp != nil || err != nil {
		resp, respErr := common.RespondUnauthorized(ctx)
		return "", resp, respErr
	}

	return claims.Username, nil, nil
}

// checkExportRateLimit validates rate limits for export operations
func (h *Handler) checkExportRateLimit(ctx *apptheory.Context, username string, exportType string) (*apptheory.Response, error) {
	// Basic rate limiting - check for existing pending exports
	existingJobs, err := h.repos.Export().GetUserExportsByStatus(ctx.Context(), username, []string{"pending", "processing"})
	if err != nil {
		h.logger.Warn("failed to check existing jobs for rate limiting", zap.Error(err))
		return nil, nil // Don't block on check error
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
		return apptheory.JSON(http.StatusTooManyRequests, map[string]any{
			"error":          "rate limit exceeded",
			"limit":          1,
			"window_seconds": 3600,
			"retry_after":    3600,
		})
	}

	return nil, nil
}
