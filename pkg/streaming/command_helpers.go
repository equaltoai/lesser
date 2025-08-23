// Package streaming provides shared helper functions for WebSocket command handlers
package streaming

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// Constants for operation statuses
const (
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
)

// BulkAccountValidationConfig holds configuration for bulk account validation
type BulkAccountValidationConfig struct {
	RequiredFields []string
	MaxAccounts    int
	MinAccounts    int
}

// DefaultBulkAccountConfig returns default configuration for bulk account operations
func DefaultBulkAccountConfig() *BulkAccountValidationConfig {
	return &BulkAccountValidationConfig{
		RequiredFields: []string{"account_ids"},
		MaxAccounts:    100,
		MinAccounts:    1,
	}
}

// ValidateBulkAccountCommand performs common validation for bulk account operations
func (bch *BaseCommandHandler) ValidateBulkAccountCommand(
	conn *ConnectionInfo,
	cmd *Command,
	config *BulkAccountValidationConfig,
) ([]string, *CommandResponse) {
	// Check authentication
	if authErr := bch.RequireAuth(conn, cmd.ID); authErr != nil {
		return nil, authErr
	}

	// Validate required fields
	if validationErr := bch.ValidatePayload(cmd.Payload, config.RequiredFields, cmd.ID); validationErr != nil {
		return nil, validationErr
	}

	// Extract and validate account IDs
	accountIDs := bch.GetStringSlice(cmd.Payload, "account_ids")
	if err := common.ValidateEntityIDsList(accountIDs, "account"); err != nil {
		return nil, bch.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid account_ids", err.Error())
	}

	// Validate account count range
	if err := common.ValidateIntRange("account_ids_count", len(accountIDs), config.MinAccounts, config.MaxAccounts); err != nil {
		return nil, bch.CreateErrorResponse(cmd.ID, "VALIDATION_ERROR", "Invalid account_ids count", err.Error())
	}

	return accountIDs, nil
}

// CreateBulkOperationResponse creates a standardized response for bulk operations
func (bch *BaseCommandHandler) CreateBulkOperationResponse(
	commandID string,
	operationID string,
	status string,
	total int,
	message string,
) *CommandResponse {
	data := map[string]interface{}{
		"operation_id": operationID,
		"status":       status,
		"total":        total,
		"processed":    0,
		"message":      message,
	}
	return bch.CreateSuccessResponse(commandID, data)
}

// CreateBulkServiceResponse creates a response from a bulk service result
func (bch *BaseCommandHandler) CreateBulkServiceResponse(
	commandID string,
	operation interface{},
	_ string,
	_ string,
) *CommandResponse {
	data, err := bch.ConvertToJSON(operation)
	if err != nil {
		return bch.CreateErrorResponse(commandID, "CONVERSION_ERROR",
			"Failed to format response", err.Error())
	}
	return bch.CreateSuccessResponse(commandID, data)
}

// BulkProcessingConfig holds configuration for bulk processing operations
type BulkProcessingConfig struct {
	BatchSize        int
	ProgressInterval int // Send progress every N operations
	BatchDelay       time.Duration
}

// DefaultBulkProcessingConfig returns default configuration for bulk processing
func DefaultBulkProcessingConfig() *BulkProcessingConfig {
	return &BulkProcessingConfig{
		BatchSize:        10,
		ProgressInterval: 5,
		BatchDelay:       100 * time.Millisecond,
	}
}

// BulkProcessingTracker tracks progress of bulk operations
type BulkProcessingTracker struct {
	Total      int
	Processed  int
	Successful int
	Failed     int
	Errors     []string
}

// NewBulkProcessingTracker creates a new processing tracker
func NewBulkProcessingTracker(total int) *BulkProcessingTracker {
	return &BulkProcessingTracker{
		Total:  total,
		Errors: make([]string, 0),
	}
}

// AddSuccess records a successful operation
func (bpt *BulkProcessingTracker) AddSuccess() {
	bpt.Processed++
	bpt.Successful++
}

// AddFailure records a failed operation
func (bpt *BulkProcessingTracker) AddFailure(err error, entityID string) {
	bpt.Processed++
	bpt.Failed++
	bpt.Errors = append(bpt.Errors, fmt.Sprintf("Failed to process %s: %v", entityID, err))
}

// ShouldSendProgress determines if progress update should be sent
func (bpt *BulkProcessingTracker) ShouldSendProgress(config *BulkProcessingConfig) bool {
	return bpt.Processed%config.ProgressInterval == 0 || bpt.Processed == bpt.Total
}

// GetStatus returns current processing status
func (bpt *BulkProcessingTracker) GetStatus() string {
	if bpt.Processed == bpt.Total {
		return StatusCompleted
	}
	return StatusProcessing
}

// ProgressUpdateHelper handles WebSocket progress updates for async operations
type ProgressUpdateHelper struct {
	publisher Publisher
	logger    *zap.Logger
}

// NewProgressUpdateHelper creates a new progress update helper
func NewProgressUpdateHelper(publisher Publisher, logger *zap.Logger) *ProgressUpdateHelper {
	return &ProgressUpdateHelper{
		publisher: publisher,
		logger:    logger,
	}
}

// SendProgressUpdate sends a progress update via WebSocket
func (puh *ProgressUpdateHelper) SendProgressUpdate(
	conn *ConnectionInfo,
	operationID string,
	tracker *BulkProcessingTracker,
	message string,
) {
	event := &Event{
		Type:   "operation.progress",
		Stream: fmt.Sprintf("user:%s", conn.UserID),
		Payload: map[string]interface{}{
			"operation_id": operationID,
			"status":       tracker.GetStatus(),
			"processed":    tracker.Processed,
			"total":        tracker.Total,
			"message":      message,
			"progress":     float64(tracker.Processed) / float64(tracker.Total) * 100,
		},
		Timestamp: time.Now(),
	}

	if puh.publisher != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := puh.publisher.PublishToUser(ctx, conn.UserID, event); err != nil {
			puh.logger.Warn("failed to send progress update via WebSocket",
				zap.String("operation_id", operationID),
				zap.String("user_id", conn.UserID),
				zap.Error(err))
		}
	}
}

// SendFinalUpdate sends the final completion update with error details
func (puh *ProgressUpdateHelper) SendFinalUpdate(
	conn *ConnectionInfo,
	operationID string,
	tracker *BulkProcessingTracker,
	finalMessage string,
) {
	event := &Event{
		Type:   "operation.completed",
		Stream: fmt.Sprintf("user:%s", conn.UserID),
		Payload: map[string]interface{}{
			"operation_id":   operationID,
			"status":         "completed",
			"processed":      tracker.Processed,
			"total":          tracker.Total,
			"message":        finalMessage,
			"successful":     tracker.Successful,
			"failed":         tracker.Failed,
			"errors":         tracker.Errors,
			"progress":       100.0,
			"completed_at":   time.Now().Format(time.RFC3339),
		},
		Timestamp: time.Now(),
	}

	if puh.publisher != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := puh.publisher.PublishToUser(ctx, conn.UserID, event); err != nil {
			puh.logger.Warn("failed to send final update via WebSocket",
				zap.String("operation_id", operationID),
				zap.String("user_id", conn.UserID),
				zap.Error(err))
		}
	}
}

// ListCommandValidationConfig holds configuration for list command validation
type ListCommandValidationConfig struct {
	RequiredFields []string
	RequireID      bool
	RequireTitle   bool
}

// ValidateListCommand performs common validation for list operations
func (bch *BaseCommandHandler) ValidateListCommand(
	conn *ConnectionInfo,
	cmd *Command,
	config *ListCommandValidationConfig,
) *CommandResponse {
	// Check authentication
	if authErr := bch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr
	}

	// Validate required fields
	if validationErr := bch.ValidatePayload(cmd.Payload, config.RequiredFields, cmd.ID); validationErr != nil {
		return validationErr
	}

	return nil
}

// PublisherConnectionConfig holds configuration for publisher connection operations
type PublisherConnectionConfig struct {
	EventType      string
	StreamTemplate string // Template for stream name, e.g., "user:%s"
	Timeout        time.Duration
}

// DefaultPublisherConnectionConfig returns default configuration for publisher connections
func DefaultPublisherConnectionConfig() *PublisherConnectionConfig {
	return &PublisherConnectionConfig{
		Timeout: 5 * time.Second,
	}
}

// PublishConnectionHelper handles common connection publishing logic
type PublishConnectionHelper struct {
	logger *zap.Logger
}

// NewPublishConnectionHelper creates a new publish connection helper
func NewPublishConnectionHelper(logger *zap.Logger) *PublishConnectionHelper {
	return &PublishConnectionHelper{
		logger: logger,
	}
}

// PublishToConnections publishes an event to multiple connections with common error handling
func (pch *PublishConnectionHelper) PublishToConnections(
	ctx context.Context,
	connections []*StreamConnection,
	event *Event,
	publishFunc func(ctx context.Context, connectionID string, event *Event) error,
	logContext map[string]interface{},
) error {
	if err := common.ValidateSliceNotEmpty("connections", connections); err != nil {
		pch.logger.Debug("no active connections", zap.Any("context", logContext))
		return nil
	}

	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Publish to each connection
	var publishErrors []error
	successCount := 0

	for _, conn := range connections {
		if err := publishFunc(ctx, conn.ConnectionID, event); err != nil {
			publishErrors = append(publishErrors, fmt.Errorf("connection %s: %w", conn.ConnectionID, err))
			pch.logger.Warn("failed to publish to connection",
				zap.String("connection_id", conn.ConnectionID),
				zap.Any("context", logContext),
				zap.Error(err))
		} else {
			successCount++
		}
	}

	// Log results
	logFields := []zap.Field{
		zap.String("event_type", event.Type),
		zap.Int("total_connections", len(connections)),
		zap.Int("successful", successCount),
		zap.Int("failed", len(publishErrors)),
	}
	for k, v := range logContext {
		logFields = append(logFields, zap.Any(k, v))
	}
	pch.logger.Info("published event to connections", logFields...)

	// Return error only if all deliveries failed
	if err := common.ValidateSliceNotEmpty("publishErrors", publishErrors); err == nil && successCount == 0 {
		return fmt.Errorf("failed to publish to any connection: %v", publishErrors)
	}

	return nil
}

// CommandHandlerConfig holds configuration for generic command handling
type CommandHandlerConfig struct {
	RequiredFields   []string                                                                                                      // Fields required in payload validation
	ParameterName    string                                                                                                        // Name of the parameter to extract (e.g., "operation_id", "id")
	ErrorCodePrefix  string                                                                                                        // Prefix for error codes (e.g., "GET_OPERATION", "MARK_READ")
	OperationName    string                                                                                                        // Name for error messages (e.g., "get bulk operation", "mark notification as read")
	ResultExtractor  func(result interface{}) interface{}                                                                          // Function to extract data from service result
	ServiceCall      func(ctx context.Context, conn *ConnectionInfo, extractedParam string) (interface{}, error)                 // Function to call the service
}

// ExecuteStandardCommandFlow handles the common command flow pattern: auth → validate → extract → service call → error handling → convert → return
func (bch *BaseCommandHandler) ExecuteStandardCommandFlow(
	ctx context.Context,
	conn *ConnectionInfo,
	cmd *Command,
	config *CommandHandlerConfig,
) (*CommandResponse, error) {
	// Step 1: Authentication check
	if authErr := bch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Step 2: Payload validation
	if validationErr := bch.ValidatePayload(cmd.Payload, config.RequiredFields, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	// Step 3: Parameter extraction
	extractedParam := bch.GetString(cmd.Payload, config.ParameterName, "")

	// Step 4: Service call execution
	result, err := config.ServiceCall(ctx, conn, extractedParam)
	if err != nil {
		errorCode := fmt.Sprintf("%s_FAILED", config.ErrorCodePrefix)
		errorMessage := fmt.Sprintf("Failed to %s", config.OperationName)
		return bch.CreateErrorResponse(cmd.ID, errorCode, errorMessage, err.Error()), nil
	}

	// Step 5: Extract the relevant data from result
	data := config.ResultExtractor(result)

	// Step 6: JSON conversion
	jsonData, err := bch.ConvertToJSON(data)
	if err != nil {
		return bch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR",
			"Failed to format response", err.Error()), nil
	}

	// Step 7: Success response creation
	return bch.CreateSuccessResponse(cmd.ID, jsonData), nil
}