package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
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

// HandleCreateImport handles POST /api/v1/imports
func (h *Handler) HandleCreateImport(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope for imports
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Parse request
	var req ImportRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
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
		return common.BadRequest(fmt.Errorf("invalid import type: %s", req.Type)), nil
	}

	// Set default mode
	if req.Mode == "" {
		req.Mode = "merge"
	}
	if req.Mode != "merge" && req.Mode != "overwrite" {
		return common.BadRequest(fmt.Errorf("invalid import mode: %s", req.Mode)), nil
	}

	// Decode file data
	fileData, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		return common.BadRequest(fmt.Errorf("invalid base64 data: %w", err)), nil
	}

	// Check file size (5MB limit for imports)
	maxSize := int64(5 * 1024 * 1024)
	if int64(len(fileData)) > maxSize {
		return common.UnprocessableEntity(fmt.Errorf("file size exceeds %dMB limit", maxSize/1024/1024)), nil
	}

	// Check for existing pending/processing import of same type
	existingJobs, err := h.getUserImportJobs(ctx, claims.Username, "pending", "processing")
	if err != nil {
		h.logger.Error("failed to check existing jobs", zap.Error(err))
	} else {
		for _, job := range existingJobs {
			if job["Type"] == req.Type {
				return common.Conflict(errors.New("import already in progress for this type")), nil
			}
		}
	}

	// Create import job
	importID := uuid.New().String()
	now := time.Now()

	// Upload file to S3 for processing
	s3Key := fmt.Sprintf("imports/%s/%s/%s.data", claims.Username, importID, req.Type)

	// Initialize S3 client
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		h.logger.Error("failed to load AWS config", zap.Error(err))
		return common.InternalServerError(errors.New("failed to initialize S3 client")), nil
	}
	s3Client := s3.NewFromConfig(awsCfg)

	// Upload to S3
	bucketName := h.cfg.S3BucketName
	if bucketName == "" {
		return common.InternalServerError(errors.New("S3 bucket not configured")), nil
	}

	putInput := &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(s3Key),
		Body:   bytes.NewReader(fileData),
	}

	_, err = s3Client.PutObject(ctx, putInput)
	if err != nil {
		h.logger.Error("failed to upload import file to S3",
			zap.String("bucket", bucketName),
			zap.String("key", s3Key),
			zap.Error(err))
		return common.InternalServerError(errors.New("failed to store import file")), nil
	}

	// Create job record
	jobRecord := map[string]interface{}{
		"PK":        fmt.Sprintf("IMPORT#%s", importID),
		"SK":        fmt.Sprintf("IMPORT#%s", importID),
		"GSI1PK":    fmt.Sprintf("USER#%s", claims.Username),
		"GSI1SK":    fmt.Sprintf("CREATED#%s", now.Format(time.RFC3339)),
		"ImportID":  importID,
		"Username":  claims.Username,
		"Type":      req.Type,
		"Mode":      req.Mode,
		"Status":    "pending",
		"S3Key":     s3Key,
		"Progress":  0,
		"Errors":    []string{},
		"CreatedAt": now,
		"TTL":       now.Add(7 * 24 * time.Hour).Unix(), // 7 days TTL
	}

	if err := h.store.CreateObject(ctx, jobRecord); err != nil {
		h.logger.Error("failed to create import job", zap.Error(err))
		return common.InternalServerError(errors.New("failed to create import job")), nil
	}

	// TODO: Trigger import processor Lambda
	// This would normally be done via SQS or EventBridge

	// Return job status
	job := ImportJob{
		ID:        importID,
		Status:    "pending",
		Type:      req.Type,
		CreatedAt: now.Format(time.RFC3339),
		Processed: 0,
		Total:     nil,
	}

	return common.Accepted(job), nil
}

// HandleGetImportStatus handles GET /api/v1/imports/:id
func (h *Handler) HandleGetImportStatus(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Get import ID
	importID := request.PathParameters["id"]
	if importID == "" {
		return common.BadRequest(errors.New("missing import ID")), nil
	}

	// Get import job
	obj, err := h.store.GetObject(ctx, fmt.Sprintf("IMPORT#%s", importID))
	if err != nil {
		return common.NotFound(fmt.Errorf("import not found: %s", importID)), nil
	}

	jobData, ok := obj.(map[string]interface{})
	if !ok {
		return common.InternalServerError(errors.New("invalid import data")), nil
	}

	// Verify ownership
	if jobData["Username"] != claims.Username {
		return common.Forbidden(errors.New("not authorized to view this import")), nil
	}

	// Build response
	job := ImportJob{
		ID:        importID,
		Status:    getStringFromJobData(jobData, "Status"),
		Type:      getStringFromJobData(jobData, "Type"),
		CreatedAt: getTimeFromJobData(jobData, "CreatedAt").Format(time.RFC3339),
		Processed: getIntFromJobData(jobData, "Progress"),
	}

	// Add total if available
	if total := getIntFromJobData(jobData, "Total"); total > 0 {
		job.Total = &total
	}

	// Add errors if any
	if errorsData, ok := jobData["Errors"].([]interface{}); ok {
		for _, err := range errorsData {
			if errStr, ok := err.(string); ok {
				job.Errors = append(job.Errors, errStr)
			}
		}
	}

	// Add results if completed
	if job.Status == "completed" {
		results := &Results{
			Success: getIntFromJobData(jobData, "SuccessCount"),
			Skipped: getIntFromJobData(jobData, "SkipCount"),
			Failed:  getIntFromJobData(jobData, "ErrorCount"),
		}
		job.Results = results
	}

	return common.OK(job), nil
}

// HandleListImports handles GET /api/v1/imports
func (h *Handler) HandleListImports(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Get all user's import jobs
	jobs, err := h.getUserImportJobs(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get import jobs", zap.Error(err))
		return common.InternalServerError(errors.New("failed to retrieve imports")), nil
	}

	// Convert to response format
	imports := make([]ImportJob, 0, len(jobs))
	for _, jobData := range jobs {
		job := ImportJob{
			ID:        getStringFromJobData(jobData, "ImportID"),
			Status:    getStringFromJobData(jobData, "Status"),
			Type:      getStringFromJobData(jobData, "Type"),
			CreatedAt: getTimeFromJobData(jobData, "CreatedAt").Format(time.RFC3339),
			Processed: getIntFromJobData(jobData, "Progress"),
		}

		// Add total if available
		if total := getIntFromJobData(jobData, "Total"); total > 0 {
			job.Total = &total
		}

		// Add errors if any
		if errorsData, ok := jobData["Errors"].([]interface{}); ok && len(errorsData) > 0 {
			job.Errors = make([]string, 0, len(errorsData))
			for _, err := range errorsData {
				if errStr, ok := err.(string); ok {
					job.Errors = append(job.Errors, errStr)
				}
			}
		}

		// Add results if completed
		if job.Status == "completed" {
			results := &Results{
				Success: getIntFromJobData(jobData, "SuccessCount"),
				Skipped: getIntFromJobData(jobData, "SkipCount"),
				Failed:  getIntFromJobData(jobData, "ErrorCount"),
			}
			job.Results = results
		}

		imports = append(imports, job)
	}

	return common.OK(imports), nil
}

// Helper to get user's import jobs
func (h *Handler) getUserImportJobs(_ context.Context, _ string, _ ...string) ([]map[string]interface{}, error) {
	// Query GSI1 for user's imports
	// This would normally use a proper DynamoDB query
	// For now, return empty to avoid errors
	return []map[string]interface{}{}, nil
}
