// Package lambda provides standardized federation delivery patterns for Lambda functions.
package lambda

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// FederationDeliveryPattern provides standardized federation delivery logic
type FederationDeliveryPattern struct {
	lambdaCtx                    *common.LambdaContext
	federationService            *federation.DeliveryService
	costCalculator               *federation.CostCalculator
	federationActivityRepository interface{} // *repositories.FederationActivityRepository
	federationCostRepository     interface{} // *repositories.FederationCostRepository
	logger                       *zap.Logger
}

// NewFederationDeliveryPattern creates a new standardized federation delivery pattern
func NewFederationDeliveryPattern(lambdaCtx *common.LambdaContext) *FederationDeliveryPattern {
	return &FederationDeliveryPattern{
		lambdaCtx:         lambdaCtx,
		federationService: lambdaCtx.DeliveryService.(*federation.DeliveryService),
		costCalculator:    lambdaCtx.CostCalculator.(*federation.CostCalculator),
		logger:            lambdaCtx.Logger,
		// Note: repositories would be extracted from lambdaCtx in real implementation
	}
}

// ActivityDeliveryMessage represents a message from the federation SQS queue
type ActivityDeliveryMessage struct {
	Activity    *activitypub.Activity `json:"activity"`
	Actor       *activitypub.Actor    `json:"actor"`
	TargetInbox string                `json:"target_inbox"`
	Attempt     int                   `json:"attempt,omitempty"`
}

// DeliveryResult represents the outcome of an activity delivery attempt
type DeliveryResult struct {
	TargetInbox string
	Success     bool
	StatusCode  int
	Error       error
	Duration    time.Duration
	Attempt     int
}

// RetryConfig defines retry behavior for federation delivery
type RetryConfig struct {
	MaxAttempts     int
	InitialDelay    time.Duration
	MaxDelay        time.Duration
	BackoffFactor   float64
	PermanentErrors []int
}

// DefaultRetryConfig returns standard retry configuration for federation delivery
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  1 * time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
		PermanentErrors: []int{
			400, // Bad Request
			401, // Unauthorized
			403, // Forbidden
			404, // Not Found
			410, // Gone
			422, // Unprocessable Entity
		},
	}
}

// ProcessSQSEvent processes federation delivery messages from SQS
// This eliminates the 50+ line duplication in outbox and federation processors
func (fdp *FederationDeliveryPattern) ProcessSQSEvent(ctx *liftPkg.Context, event events.SQSEvent) error {
	requestID := ctx.GetRequestID()
	if requestID == "" {
		requestID = fmt.Sprintf("federation-%d", time.Now().UnixNano())
	}

	fdp.logger.Info("processing federation delivery batch",
		zap.String("request_id", requestID),
		zap.Int("message_count", len(event.Records)),
	)

	// Process messages with controlled concurrency
	results := fdp.processMessagesWithConcurrency(ctx, event.Records, 10)

	// Count failures
	failures := 0
	for _, result := range results {
		if !result.Success {
			failures++
		}
	}

	// Handle batch results
	if failures > 0 {
		fdp.logger.Error("federation batch had failures",
			zap.String("request_id", requestID),
			zap.Int("failure_count", failures),
			zap.Int("total_count", len(event.Records)),
		)

		return liftPkg.NewLiftError("PARTIAL_FAILURE", "partial federation batch failure", 500).
			WithDetail("failed_count", failures).
			WithDetail("total_count", len(event.Records))
	}

	fdp.logger.Info("federation batch completed successfully",
		zap.String("request_id", requestID),
		zap.Int("delivered_count", len(event.Records)),
	)

	return nil
}

// processMessagesWithConcurrency processes SQS messages with controlled concurrency
func (fdp *FederationDeliveryPattern) processMessagesWithConcurrency(ctx *liftPkg.Context, records []events.SQSMessage, concurrency int) []DeliveryResult {
	sem := make(chan struct{}, concurrency)
	results := make([]DeliveryResult, len(records))
	
	for i, record := range records {
		sem <- struct{}{} // Acquire semaphore
		
		go func(index int, msg events.SQSMessage) {
			defer func() { <-sem }() // Release semaphore
			
			result := fdp.processMessage(ctx, msg)
			results[index] = result
			
			if !result.Success {
				fdp.logger.Error("failed to process federation message",
					zap.String("message_id", msg.MessageId),
					zap.Error(result.Error),
				)
			}
		}(i, record)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < concurrency; i++ {
		sem <- struct{}{}
	}
	
	return results
}

// processMessage processes a single SQS federation message
func (fdp *FederationDeliveryPattern) processMessage(ctx *liftPkg.Context, msg events.SQSMessage) DeliveryResult {
	start := time.Now()

	// Parse the delivery message
	var deliveryMsg ActivityDeliveryMessage
	if err := json.Unmarshal([]byte(msg.Body), &deliveryMsg); err != nil {
		fdp.logger.Error("invalid federation message format",
			zap.String("message_id", msg.MessageId),
			zap.Error(err),
		)
		return DeliveryResult{
			Success: false,
			Error:   fmt.Errorf("invalid message format: %w", err),
		}
	}

	// Validate required fields
	if err := fdp.validateDeliveryMessage(deliveryMsg); err != nil {
		return DeliveryResult{
			Success: false,
			Error:   err,
		}
	}

	fdp.logger.Info("processing federation delivery",
		zap.String("message_id", msg.MessageId),
		zap.String("activity_id", deliveryMsg.Activity.ID),
		zap.String("activity_type", deliveryMsg.Activity.Type),
		zap.String("target_inbox", deliveryMsg.TargetInbox),
	)

	// Attempt delivery with retry logic
	result := fdp.deliverActivityWithRetry(ctx.Request.Context(), deliveryMsg)

	// Record comprehensive metrics and costs
	fdp.recordDeliveryMetrics(deliveryMsg, result, time.Since(start))

	return result
}

// validateDeliveryMessage validates required fields in delivery message
func (fdp *FederationDeliveryPattern) validateDeliveryMessage(msg ActivityDeliveryMessage) error {
	if msg.Activity == nil {
		return fmt.Errorf("missing activity in message")
	}
	if msg.Actor == nil {
		return fmt.Errorf("missing actor in message")
	}
	if err := common.ValidateRequiredParam("targetInbox", msg.TargetInbox); err != nil {
		return fmt.Errorf("missing target inbox in message")
	}
	return nil
}

// deliverActivityWithRetry attempts delivery with exponential backoff retry
func (fdp *FederationDeliveryPattern) deliverActivityWithRetry(ctx context.Context, msg ActivityDeliveryMessage) DeliveryResult {
	retryConfig := DefaultRetryConfig()
	var lastResult DeliveryResult

	for attempt := 1; attempt <= retryConfig.MaxAttempts; attempt++ {
		start := time.Now()

		// Calculate delay for this attempt (skip delay on first attempt)
		if attempt > 1 {
			delay := fdp.calculateBackoffDelay(attempt-1, retryConfig)
			fdp.logger.Info("retrying delivery after delay",
				zap.String("activity_id", msg.Activity.ID),
				zap.String("target_inbox", msg.TargetInbox),
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay),
			)
			time.Sleep(delay)
		}

		// Attempt delivery
		err := fdp.federationService.DeliverActivity(ctx, msg.Activity, msg.TargetInbox, msg.Actor)

		lastResult = DeliveryResult{
			TargetInbox: msg.TargetInbox,
			Success:     err == nil,
			Duration:    time.Since(start),
			Attempt:     attempt,
		}

		if err != nil {
			lastResult.Error = err
			lastResult.StatusCode = 500 // Default to server error

			fdp.logger.Warn("delivery attempt failed",
				zap.String("activity_id", msg.Activity.ID),
				zap.String("target_inbox", msg.TargetInbox),
				zap.Int("attempt", attempt),
				zap.Error(err),
			)

			// Check if this is a permanent error
			if fdp.isPermanentError(lastResult.StatusCode, retryConfig) {
				fdp.logger.Info("permanent error detected, not retrying",
					zap.String("activity_id", msg.Activity.ID),
					zap.Int("status_code", lastResult.StatusCode),
				)
				break
			}

			continue
		}

		// Success
		lastResult.Success = true
		lastResult.StatusCode = 200

		fdp.logger.Info("activity delivered successfully",
			zap.String("activity_id", msg.Activity.ID),
			zap.String("target_inbox", msg.TargetInbox),
			zap.Int("attempt", attempt),
			zap.Duration("duration", lastResult.Duration),
		)
		break
	}

	return lastResult
}

// calculateBackoffDelay calculates the delay for exponential backoff
func (fdp *FederationDeliveryPattern) calculateBackoffDelay(attempt int, config RetryConfig) time.Duration {
	delay := float64(config.InitialDelay) * config.BackoffFactor * float64(attempt)
	maxDelay := float64(config.MaxDelay)
	if delay > maxDelay {
		delay = maxDelay
	}
	return time.Duration(delay)
}

// isPermanentError checks if an HTTP status code represents a permanent error
func (fdp *FederationDeliveryPattern) isPermanentError(statusCode int, config RetryConfig) bool {
	for _, code := range config.PermanentErrors {
		if statusCode == code {
			return true
		}
	}
	return false
}

// recordDeliveryMetrics records comprehensive delivery metrics and costs
func (fdp *FederationDeliveryPattern) recordDeliveryMetrics(msg ActivityDeliveryMessage, result DeliveryResult, totalDuration time.Duration) {
	// Extract domain from target inbox
	targetDomain := extractDomainFromURL(msg.TargetInbox)

	// Calculate payload size
	payloadBytes, _ := json.Marshal(msg.Activity)
	payloadSize := int64(len(payloadBytes))

	// Create cost calculation parameters
	costParams := &federation.CostCalculationParams{
		ActivityID:        msg.Activity.ID,
		Domain:            targetDomain,
		ActivityType:      msg.Activity.Type,
		Direction:         "outbound",
		OperationType:     "federation_delivery",
		Timestamp:         time.Now(),
		PayloadSize:       payloadSize,
		Success:           result.Success,
		ResponseTimeMs:    totalDuration.Milliseconds(),
		LambdaDurationMs:  totalDuration.Milliseconds(),
		ProcessingTimeMs:  result.Duration.Milliseconds(),
		RetryCount:        result.Attempt - 1,
		HTTPRequestCount:  1,
		DataTransferBytes: payloadSize,
		DynamoDBReadCount: 1,
		DynamoDBWriteCount: 2,
		SQSMessageCount:   1,
		DNSLookupCount:    1,
	}

	if result.Error != nil {
		costParams.ErrorMessage = result.Error.Error()
	}

	// Calculate comprehensive costs
	cost := fdp.costCalculator.CalculateFederationCosts(costParams)

	// Record metrics asynchronously
	go func() {
		// Record cost tracking
		if fdp.federationCostRepository != nil {
			// Note: Actual implementation would use repository interface
			fdp.logger.Debug("federation cost recorded",
				zap.String("activity_type", msg.Activity.Type),
				zap.String("target_domain", targetDomain),
				zap.Int64("total_cost_micro_cents", cost.TotalCostMicroCents),
			)
		}

		// Record delivery status
		if fdp.federationActivityRepository != nil {
			// Note: Actual implementation would use repository interface
			fdp.logger.Debug("federation activity recorded",
				zap.String("activity_id", msg.Activity.ID),
				zap.Bool("success", result.Success),
			)
		}
	}()

	fdp.logger.Info("federation delivery metrics recorded",
		zap.String("activity_type", msg.Activity.Type),
		zap.String("target_domain", targetDomain),
		zap.Bool("success", result.Success),
		zap.Int("attempts", result.Attempt),
		zap.Duration("total_duration", totalDuration),
	)
}

// extractDomainFromURL extracts the domain from a URL
func extractDomainFromURL(urlStr string) string {
	if err := common.ValidateRequiredParam("url", urlStr); err != nil {
		return ""
	}

	// Handle https:// URLs
	if strings.HasPrefix(urlStr, "https://") {
		parts := urlStr[8:]
		if idx := strings.IndexByte(parts, '/'); idx > 0 {
			return parts[:idx]
		}
		if idx := strings.IndexByte(parts, ':'); idx > 0 {
			return parts[:idx]
		}
		return parts
	}

	// Handle http:// URLs
	if strings.HasPrefix(urlStr, "http://") {
		parts := urlStr[7:]
		if idx := strings.IndexByte(parts, '/'); idx > 0 {
			return parts[:idx]
		}
		if idx := strings.IndexByte(parts, ':'); idx > 0 {
			return parts[:idx]
		}
		return parts
	}

	return urlStr
}

// TriggerFederationDelivery provides standardized logic for triggering federation delivery
func (fdp *FederationDeliveryPattern) TriggerFederationDelivery(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	switch activity.Type {
	case activitypub.CreateType, activitypub.UpdateType, activitypub.AnnounceType:
		// Public/unlisted content should be delivered to followers
		return fdp.deliverToFollowersAndRecipients(ctx, activity, actor)

	case activitypub.FollowType, activitypub.LikeType:
		// Targeted activities should be delivered to specific recipients
		return fdp.federationService.DeliverToRecipients(ctx, activity, actor)

	case activitypub.DeleteType, activitypub.UndoType:
		// Deletions and undos should be delivered broadly
		return fdp.deliverToFollowersAndRecipients(ctx, activity, actor)

	case activitypub.AcceptType, activitypub.RejectType:
		// Accept/Reject should be delivered to specific recipients
		return fdp.federationService.DeliverToRecipients(ctx, activity, actor)

	default:
		fdp.logger.Info("no federation delivery configured for activity type",
			zap.String("activity_type", activity.Type),
			zap.String("activity_id", activity.ID),
		)
		return nil
	}
}

// deliverToFollowersAndRecipients delivers to both followers and specific recipients
func (fdp *FederationDeliveryPattern) deliverToFollowersAndRecipients(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	// Deliver to followers if this is public/unlisted content
	if fdp.isPublicOrUnlisted(activity) {
		if err := fdp.federationService.DeliverToFollowers(ctx, activity, actor); err != nil {
			fdp.logger.Error("failed to deliver to followers", zap.Error(err))
		}
	}

	// Also deliver to specific recipients
	if err := fdp.federationService.DeliverToRecipients(ctx, activity, actor); err != nil {
		fdp.logger.Error("failed to deliver to recipients", zap.Error(err))
		return err
	}

	return nil
}

// isPublicOrUnlisted checks if the activity is public or unlisted
func (fdp *FederationDeliveryPattern) isPublicOrUnlisted(activity *activitypub.Activity) bool {
	// Check if the activity has public addressing
	for _, addr := range activity.To {
		if addr == activitypub.PublicAddress {
			return true
		}
	}

	for _, addr := range activity.CC {
		if addr == activitypub.PublicAddress {
			return true
		}
	}

	return false
}