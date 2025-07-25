package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ExportRequest represents a data export request
type ExportRequest struct {
	Type         string         `json:"type"`          // archive, followers, following, blocks, mutes, lists, bookmarks
	Format       string         `json:"format"`        // activitypub, mastodon, csv
	IncludeMedia bool           `json:"include_media"` // Include media attachments
	DateRange    *DateRange     `json:"date_range"`    // Optional date filtering
	Options      map[string]any `json:"options"`       // Additional format-specific options
}

// DateRange for filtering exports
type DateRange struct {
	Start string `json:"start"` // ISO date
	End   string `json:"end"`   // ISO date
}

// ExportJob represents an export job status
type ExportJob struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"` // pending, processing, completed, failed
	Type        string  `json:"type"`
	Format      string  `json:"format"`
	CreatedAt   string  `json:"created_at"`
	DownloadURL *string `json:"download_url"`
	ExpiresAt   *string `json:"expires_at"`
	FileSize    *int64  `json:"file_size"`
	RecordCount *int    `json:"record_count"`
	Error       *string `json:"error"`
}

// HandleCreateExport handles POST /api/v1/exports
func (h *Handler) HandleCreateExport(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Check read scope
	if !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Parse request
	var req ExportRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Set defaults
	if req.Type == "" {
		req.Type = "archive"
	}
	if req.Format == "" {
		req.Format = "activitypub"
	}

	// Validate export type
	validTypes := map[string]bool{
		"archive":   true,
		"followers": true,
		"following": true,
		"blocks":    true,
		"mutes":     true,
		"lists":     true,
		"bookmarks": true,
	}
	if !validTypes[req.Type] {
		return common.BadRequest(fmt.Errorf("invalid export type: %s", req.Type)), nil
	}

	// Validate format
	validFormats := map[string]bool{
		"activitypub": true,
		"mastodon":    true,
		"csv":         true,
	}
	if !validFormats[req.Format] {
		return common.BadRequest(fmt.Errorf("invalid export format: %s", req.Format)), nil
	}

	// CSV format is only valid for certain types
	if req.Format == "csv" && req.Type == "archive" {
		return common.BadRequest(errors.New("CSV format not available for archive exports")), nil
	}

	// Check for existing pending/processing export of same type
	existingJobs, err := h.getUserExportJobs(ctx, claims.Username, "pending", "processing")
	if err != nil {
		h.logger.Error("failed to check existing jobs", zap.Error(err))
	} else {
		for _, job := range existingJobs {
			if job["Type"] == req.Type {
				return common.Conflict(errors.New("export already in progress for this type")), nil
			}
		}
	}

	// Create export job
	exportID := uuid.New().String()
	now := time.Now()

	jobRecord := map[string]any{
		"PK":           fmt.Sprintf("EXPORT#%s", exportID),
		"SK":           fmt.Sprintf("EXPORT#%s", exportID),
		"GSI1PK":       fmt.Sprintf("USER#%s", claims.Username),
		"GSI1SK":       fmt.Sprintf("CREATED#%s", now.Format(time.RFC3339)),
		"ExportID":     exportID,
		"Username":     claims.Username,
		"Type":         req.Type,
		"Format":       req.Format,
		"Status":       "pending",
		"Options":      req.Options,
		"IncludeMedia": req.IncludeMedia,
		"CreatedAt":    now,
		"TTL":          now.Add(30 * 24 * time.Hour).Unix(), // 30 days TTL
	}

	// Add date range if provided
	if req.DateRange != nil {
		jobRecord["DateRangeStart"] = req.DateRange.Start
		jobRecord["DateRangeEnd"] = req.DateRange.End
	}

	if err := h.store.CreateObject(ctx, jobRecord); err != nil {
		h.logger.Error("failed to create export job", zap.Error(err))
		return common.InternalServerError(errors.New("failed to create export job")), nil
	}

	// Trigger export generator Lambda via SQS
	if err := h.triggerExportProcessor(ctx, exportID, claims.Username, req.Type); err != nil {
		h.logger.Error("failed to trigger export processor", zap.Error(err))
		// Don't fail the request, export can be retried later
	}

	// Return job status
	job := ExportJob{
		ID:        exportID,
		Status:    "pending",
		Type:      req.Type,
		Format:    req.Format,
		CreatedAt: now.Format(time.RFC3339),
	}

	return common.Accepted(job), nil
}

// HandleGetExportStatus handles GET /api/v1/exports/:id
func (h *Handler) HandleGetExportStatus(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Get export ID
	exportID := request.PathParameters["id"]
	if exportID == "" {
		return common.BadRequest(errors.New("missing export ID")), nil
	}

	// Get export job
	obj, err := h.store.GetObject(ctx, fmt.Sprintf("EXPORT#%s", exportID))
	if err != nil {
		return common.NotFound(fmt.Errorf("export not found: %s", exportID)), nil
	}

	jobData, ok := obj.(map[string]any)
	if !ok {
		return common.InternalServerError(errors.New("invalid export data")), nil
	}

	// Verify ownership
	if jobData["Username"] != claims.Username {
		return common.Forbidden(errors.New("not authorized to view this export")), nil
	}

	// Build response
	job := ExportJob{
		ID:        exportID,
		Status:    getStringFromJobData(jobData, "Status"),
		Type:      getStringFromJobData(jobData, "Type"),
		Format:    getStringFromJobData(jobData, "Format"),
		CreatedAt: getTimeFromJobData(jobData, "CreatedAt").Format(time.RFC3339),
	}

	// Add completed fields if available
	if job.Status == "completed" {
		if url := getStringFromJobData(jobData, "DownloadURL"); url != "" {
			job.DownloadURL = &url
		}
		if expiresAt := getTimeFromJobData(jobData, "ExpiresAt"); !expiresAt.IsZero() {
			expires := expiresAt.Format(time.RFC3339)
			job.ExpiresAt = &expires
		}
		if size := getInt64FromJobData(jobData, "FileSize"); size > 0 {
			job.FileSize = &size
		}
		if count := getIntFromJobData(jobData, "RecordCount"); count > 0 {
			job.RecordCount = &count
		}
	}

	// Add error if failed
	if job.Status == "failed" {
		if err := getStringFromJobData(jobData, "Error"); err != "" {
			job.Error = &err
		}
	}

	return common.OK(job), nil
}

// HandleListExports handles GET /api/v1/exports
func (h *Handler) HandleListExports(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Get all user's export jobs
	jobs, err := h.getUserExportJobs(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get export jobs", zap.Error(err))
		return common.InternalServerError(errors.New("failed to retrieve exports")), nil
	}

	// Convert to response format
	exports := make([]ExportJob, 0, len(jobs))
	for _, jobData := range jobs {
		job := ExportJob{
			ID:        getStringFromJobData(jobData, "ExportID"),
			Status:    getStringFromJobData(jobData, "Status"),
			Type:      getStringFromJobData(jobData, "Type"),
			Format:    getStringFromJobData(jobData, "Format"),
			CreatedAt: getTimeFromJobData(jobData, "CreatedAt").Format(time.RFC3339),
		}

		// Add completed fields if available
		if job.Status == "completed" {
			if url := getStringFromJobData(jobData, "DownloadURL"); url != "" {
				job.DownloadURL = &url
			}
			if expiresAt := getTimeFromJobData(jobData, "ExpiresAt"); !expiresAt.IsZero() {
				expires := expiresAt.Format(time.RFC3339)
				job.ExpiresAt = &expires
			}
			if size := getInt64FromJobData(jobData, "FileSize"); size > 0 {
				job.FileSize = &size
			}
			if count := getIntFromJobData(jobData, "RecordCount"); count > 0 {
				job.RecordCount = &count
			}
		}

		// Add error if failed
		if job.Status == "failed" {
			if err := getStringFromJobData(jobData, "Error"); err != "" {
				job.Error = &err
			}
		}

		exports = append(exports, job)
	}

	return common.OK(exports), nil
}

// Helper to get user's export jobs
func (h *Handler) getUserExportJobs(ctx context.Context, username string, statuses ...string) ([]map[string]any, error) {
	// Query GSI1 for user's exports
	// GSI1PK: USER#username, GSI1SK: CREATED#timestamp

	// Check if DynamoDB client is available
	if h.dynamoClient == nil {
		h.logger.Error("DynamoDB client not initialized")
		return nil, errors.New("DynamoDB client not initialized")
	}

	// Build the query input for GSI1
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(h.cfg.DynamoTableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
		},
		ScanIndexForward: aws.Bool(false), // Most recent first
	}

	// If specific statuses requested, add filter
	if len(statuses) > 0 {
		filterExpressions := make([]string, 0)
		for i, status := range statuses {
			filterExpressions = append(filterExpressions, fmt.Sprintf("#status = :status%d", i))
			queryInput.ExpressionAttributeValues[fmt.Sprintf(":status%d", i)] = &types.AttributeValueMemberS{Value: status}
		}
		queryInput.FilterExpression = aws.String(strings.Join(filterExpressions, " OR "))
		queryInput.ExpressionAttributeNames = map[string]string{
			"#status": "Status",
		}
	}

	// Execute query
	result, err := h.dynamoClient.Query(ctx, queryInput)
	if err != nil {
		h.logger.Error("failed to query GSI1 for exports",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("query GSI1 for exports: %w", err)
	}

	// Filter for EXPORT items only (GSI1 may contain other user data)
	exports := make([]map[string]any, 0)
	for _, item := range result.Items {
		// Check if this is an export job by looking at PK
		if pk, ok := item["PK"].(*types.AttributeValueMemberS); ok {
			if strings.HasPrefix(pk.Value, "EXPORT#") {
				// Convert DynamoDB item to map
				var jobData map[string]any
				if err := attributevalue.UnmarshalMap(item, &jobData); err != nil {
					h.logger.Error("failed to unmarshal export job",
						zap.String("pk", pk.Value),
						zap.Error(err))
					continue
				}
				exports = append(exports, jobData)
			}
		}
	}

	h.logger.Info("retrieved export jobs",
		zap.String("username", username),
		zap.Int("count", len(exports)))

	return exports, nil
}

// Helper functions for job data extraction
func getStringFromJobData(data map[string]any, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}

func getIntFromJobData(data map[string]any, key string) int {
	if val, ok := data[key].(float64); ok {
		return int(val)
	}
	if val, ok := data[key].(int); ok {
		return val
	}
	return 0
}

func getInt64FromJobData(data map[string]any, key string) int64 {
	if val, ok := data[key].(float64); ok {
		return int64(val)
	}
	if val, ok := data[key].(int64); ok {
		return val
	}
	return 0
}

func getTimeFromJobData(data map[string]any, key string) time.Time {
	if val, ok := data[key].(string); ok {
		t, _ := time.Parse(time.RFC3339, val)
		return t
	}
	if val, ok := data[key].(time.Time); ok {
		return val
	}
	return time.Time{}
}

// triggerExportProcessor sends a message to SQS to trigger export processing
func (h *Handler) triggerExportProcessor(ctx context.Context, exportID, username, exportType string) error {
	// Initialize SQS client
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}
	sqsClient := sqs.NewFromConfig(awsCfg)

	// Create message payload
	message := map[string]any{
		"exportID":  exportID,
		"username":  username,
		"type":      exportType,
		"timestamp": time.Now().Unix(),
	}

	messageBody, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Get SQS queue URL from environment or config
	queueURL := h.cfg.ExportProcessorQueueURL
	if queueURL == "" {
		// Default queue name pattern
		queueURL = fmt.Sprintf("https://sqs.%s.amazonaws.com/%s/export-processor-queue",
			awsCfg.Region, h.cfg.AWSAccountID)
	}

	// Send message to SQS
	_, err = sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    &queueURL,
		MessageBody: aws.String(string(messageBody)),
		MessageAttributes: map[string]sqstypes.MessageAttributeValue{
			"JobType": {
				DataType:    aws.String("String"),
				StringValue: aws.String("data-export"),
			},
			"ExportType": {
				DataType:    aws.String("String"),
				StringValue: aws.String(exportType),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to send SQS message: %w", err)
	}

	h.logger.Info("export processing job queued",
		zap.String("export_id", exportID),
		zap.String("username", username),
		zap.String("type", exportType),
		zap.String("queue_url", queueURL))

	return nil
}
