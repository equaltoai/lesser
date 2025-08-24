// Package main implements the outbox Lambda function for serving ActivityPub outbox endpoints.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// OutboxProcessor handles ActivityPub federation delivery via SQS
type OutboxProcessor struct {
	federationService            *federation.DeliveryService
	db                           interface{} // DynamORM client interface
	actorRepository              *repositories.ActorRepository
	activityRepository           *repositories.ActivityRepository
	federationActivityRepository *repositories.FederationActivityRepository
	federationCostRepository     *repositories.FederationCostRepository
	logger                       *zap.Logger
	cfg                          interface{} // config.Config interface
	httpClient                   *http.Client
	retryConfig                  RetryConfig
	costCalculator               *federation.CostCalculator
	repos                        core.RepositoryStorage
	lambdaCtx                    *common.LambdaContext
}

// RetryConfig defines retry behavior for federation delivery
type RetryConfig struct {
	MaxAttempts     int
	InitialDelay    time.Duration
	MaxDelay        time.Duration
	BackoffFactor   float64
	PermanentErrors []int // HTTP status codes that shouldn't be retried
}

// ActivityDeliveryMessage represents a message from the outbox SQS queue
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

// NewOutboxProcessor creates a new outbox processor using standardized Lambda initialization
func NewOutboxProcessor() (*OutboxProcessor, error) {
	// Initialize Lambda with federation-specific configuration
	lambdaCtx := common.MustInitializeLambda(common.LambdaConfig{
		ServiceName: "outbox",
		LambdaType:  common.LambdaTypeFederation,
	})

	// Initialize federation-specific services
	options := common.DefaultLambdaInitOptions(common.LambdaTypeFederation)
	if err := lambdaCtx.InitializeWithOptions(options); err != nil {
		return nil, lambdaServicesInitializationFailed()
	}

	// Extract initialized services with type safety
	repos, ok := lambdaCtx.Repos.(core.RepositoryStorage)
	if !ok {
		return nil, repositoryStorageFromContextFailed()
	}

	// Get individual repositories from the repository storage
	actorRepo := repos.Actor()
	activityRepo := repos.Activity()

	// Create federation-specific repositories manually until they're added to core interface
	federationActivityRepo := repositories.NewFederationActivityRepository(
		repos.GetDB(), repos.GetTableName(), lambdaCtx.Logger, nil)
	costTrackingBaseRepo := repositories.NewBaseRepository[*models.FederationCostTracking](repos.GetDB(), repos.GetTableName(), lambdaCtx.Logger)
	budgetBaseRepo := repositories.NewBaseRepository[*models.FederationBudget](repos.GetDB(), repos.GetTableName(), lambdaCtx.Logger)
	federationCostRepo := repositories.NewFederationCostRepositoryFromBase(costTrackingBaseRepo, budgetBaseRepo, nil)

	// Extract federation services from Lambda context
	federationService, ok := lambdaCtx.DeliveryService.(*federation.DeliveryService)
	if !ok {
		return nil, federationServiceFromContextFailed()
	}

	costCalculator, ok := lambdaCtx.CostCalculator.(*federation.CostCalculator)
	if !ok {
		return nil, costCalculatorFromContextFailed()
	}

	return &OutboxProcessor{
		federationService:            federationService,
		db:                           lambdaCtx.DynamoDB,
		actorRepository:              actorRepo,
		activityRepository:           activityRepo,
		federationActivityRepository: federationActivityRepo,
		federationCostRepository:     federationCostRepo,
		logger:                       lambdaCtx.Logger,
		cfg:                          lambdaCtx.Config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		retryConfig: RetryConfig{
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
		},
		costCalculator: costCalculator,
		repos:          repos,
		lambdaCtx:      lambdaCtx,
	}, nil
}

// HandleSQS processes ActivityPub federation messages from SQS
func (op *OutboxProcessor) HandleSQS(ctx *lift.Context, event events.SQSEvent) error {
	requestID, _ := ctx.Get("requestID").(string)
	if err := common.ValidateRequiredParam("requestID", requestID); err != nil {
		requestID = fmt.Sprintf("outbox-%d", time.Now().UnixNano())
		ctx.Set("requestID", requestID)
	}

	op.logger.Info("processing outbox federation batch",
		zap.String("request_id", requestID),
		zap.Int("message_count", len(event.Records)),
	)

	// Process messages concurrently with controlled concurrency
	concurrency := 10 // Limit concurrent federation requests
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	failures := make([]error, 0)
	var failureMutex sync.Mutex

	for _, record := range event.Records {
		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore

		go func(msg events.SQSMessage) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			if err := op.processMessage(ctx, msg); err != nil {
				failureMutex.Lock()
				failures = append(failures, err)
				failureMutex.Unlock()

				op.logger.Error("failed to process federation message",
					zap.String("message_id", msg.MessageId),
					zap.String("request_id", requestID),
					zap.Error(err),
				)
			}
		}(record)
	}

	wg.Wait()

	// Handle batch failures
	if len(failures) > 0 {
		op.logger.Error("federation batch had failures",
			zap.String("request_id", requestID),
			zap.Int("failure_count", len(failures)),
			zap.Int("total_count", len(event.Records)),
		)

		// Return error to trigger SQS retry for failed messages
		return lift.NewLiftError("PARTIAL_FAILURE", "partial federation batch failure", 500).
			WithDetail("failed_count", len(failures)).
			WithDetail("total_count", len(event.Records))
	}

	op.logger.Info("federation batch completed successfully",
		zap.String("request_id", requestID),
		zap.Int("delivered_count", len(event.Records)),
	)

	return nil
}

// processMessage processes a single SQS federation message
func (op *OutboxProcessor) processMessage(ctx *lift.Context, msg events.SQSMessage) error {
	start := time.Now()

	// Parse the delivery message
	var deliveryMsg ActivityDeliveryMessage
	if err := json.Unmarshal([]byte(msg.Body), &deliveryMsg); err != nil {
		op.logger.Error("invalid federation message format",
			zap.String("message_id", msg.MessageId),
			zap.Error(err),
		)
		return invalidMessageFormat()
	}

	// Extract domain from target inbox
	targetDomain := extractDomainFromURL(deliveryMsg.TargetInbox)

	// Calculate payload size
	payloadBytes, err := json.Marshal(deliveryMsg.Activity)
	payloadSize := int64(len(payloadBytes))
	if err != nil {
		payloadSize = int64(len(deliveryMsg.Activity.ID)) // Fallback
	}

	// Validate required fields
	if deliveryMsg.Activity == nil {
		return missingActivityInMessage()
	}
	if deliveryMsg.Actor == nil {
		return missingActorInMessage()
	}
	if err := common.ValidateRequiredParam("targetInbox", deliveryMsg.TargetInbox); err != nil {
		return missingTargetInbox()
	}

	op.logger.Info("processing federation delivery",
		zap.String("message_id", msg.MessageId),
		zap.String("activity_id", deliveryMsg.Activity.ID),
		zap.String("activity_type", deliveryMsg.Activity.Type),
		zap.String("target_inbox", deliveryMsg.TargetInbox),
		zap.String("actor", deliveryMsg.Actor.ID),
		zap.Int("attempt", deliveryMsg.Attempt),
	)

	// Prepare comprehensive cost tracking parameters
	costParams := &federation.CostCalculationParams{
		ActivityID:        deliveryMsg.Activity.ID,
		Domain:            targetDomain,
		ActivityType:      deliveryMsg.Activity.Type,
		Direction:         "outbound",
		OperationType:     "outbox_delivery",
		Timestamp:         start,
		PayloadSize:       payloadSize,
		LambdaMemoryMB:    512,                     // Standard memory allocation
		HTTPRequestCount:  1,                       // One HTTP request for delivery
		DataTransferBytes: payloadSize,             // Outbound data transfer
		DynamoDBReadCount: 1,                       // Delivery status lookup
		SQSMessageCount:   1,                       // This SQS message
		RetryCount:        deliveryMsg.Attempt - 1, // Previous attempts
	}

	// Check budget limits before delivery
	budgetCheck, err := op.federationCostRepository.CheckBudgetLimits(ctx.Request.Context(),
		targetDomain, "daily", deliveryMsg.Activity.Type, "outbound",
		op.costCalculator.EstimateOutboundActivityCost(deliveryMsg.Activity.Type, payloadSize, 1))

	if err != nil {
		op.logger.Warn("failed to check budget limits", zap.Error(err))
	} else if !budgetCheck.Allowed {
		op.logger.Warn("delivery blocked by budget limits",
			zap.String("domain", targetDomain),
			zap.String("reason", budgetCheck.Message))

		// Record cost tracking for budget block
		costParams.Success = false
		costParams.ErrorMessage = "Budget limit exceeded: " + budgetCheck.Message
		costParams.ResponseTimeMs = time.Since(start).Milliseconds()
		costParams.LambdaDurationMs = time.Since(start).Milliseconds()

		cost := op.costCalculator.CalculateFederationCosts(costParams)
		go func() {
			if err := op.federationCostRepository.RecordFederationCost(context.Background(), cost); err != nil {
				op.logger.Warn("failed to record federation cost", zap.Error(err))
			}
		}()

		return deliveryBudgetLimitExceeded()
	}

	// Attempt delivery with retry logic
	result := op.deliverActivityWithRetry(ctx.Request.Context(), deliveryMsg, costParams)

	// Record comprehensive cost tracking and metrics
	op.recordComprehensiveCostTracking(deliveryMsg, result, costParams, time.Since(start))

	// Track delivery status (simplified)
	if err := op.trackDeliveryStatus(ctx.Request.Context(), deliveryMsg, result); err != nil {
		op.logger.Warn("failed to track delivery status",
			zap.String("message_id", msg.MessageId),
			zap.Error(err),
		)
	}

	// Return error for temporary failures to trigger SQS retry
	if !result.Success && !op.isPermanentError(result.StatusCode) {
		return deliveryRetryableFailure(err)
	}

	return nil
}

// deliverActivityWithRetry attempts delivery with exponential backoff retry
func (op *OutboxProcessor) deliverActivityWithRetry(ctx context.Context, msg ActivityDeliveryMessage, _ *federation.CostCalculationParams) DeliveryResult {
	var lastResult DeliveryResult

	for attempt := 1; attempt <= op.retryConfig.MaxAttempts; attempt++ {
		start := time.Now()

		// Calculate delay for this attempt (skip delay on first attempt)
		if attempt > 1 {
			delay := op.calculateBackoffDelay(attempt - 1)
			op.logger.Info("retrying delivery after delay",
				zap.String("activity_id", msg.Activity.ID),
				zap.String("target_inbox", msg.TargetInbox),
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay),
			)
			time.Sleep(delay)
		}

		// Attempt delivery
		err := op.federationService.DeliverActivity(ctx, msg.Activity, msg.TargetInbox, msg.Actor)

		lastResult = DeliveryResult{
			TargetInbox: msg.TargetInbox,
			Success:     err == nil,
			Duration:    time.Since(start),
			Attempt:     attempt,
		}

		if err != nil {
			lastResult.Error = err

			// Try to extract status code from error message
			// This is a simplified approach - in production you'd have proper error types
			lastResult.StatusCode = 500 // Default to server error

			op.logger.Warn("delivery attempt failed",
				zap.String("activity_id", msg.Activity.ID),
				zap.String("target_inbox", msg.TargetInbox),
				zap.Int("attempt", attempt),
				zap.Int("status_code", lastResult.StatusCode),
				zap.Error(err),
			)

			// Check if this is a permanent error
			if op.isPermanentError(lastResult.StatusCode) {
				op.logger.Info("permanent error detected, not retrying",
					zap.String("activity_id", msg.Activity.ID),
					zap.Int("status_code", lastResult.StatusCode),
				)
				break
			}

			// Continue to next attempt if not permanent and not at max attempts
			continue
		}

		// Success
		lastResult.Success = true
		lastResult.StatusCode = 200 // Assume success

		op.logger.Info("activity delivered successfully",
			zap.String("activity_id", msg.Activity.ID),
			zap.String("target_inbox", msg.TargetInbox),
			zap.Int("attempt", attempt),
			zap.Duration("duration", lastResult.Duration),
		)
		break
	}

	return lastResult
}

// HTTP signature verification is handled within the federation delivery service

// calculateBackoffDelay calculates the delay for exponential backoff
func (op *OutboxProcessor) calculateBackoffDelay(attempt int) time.Duration {
	delay := float64(op.retryConfig.InitialDelay) *
		op.retryConfig.BackoffFactor * float64(attempt)

	maxDelay := float64(op.retryConfig.MaxDelay)
	if delay > maxDelay {
		delay = maxDelay
	}

	return time.Duration(delay)
}

// isPermanentError checks if an HTTP status code represents a permanent error
func (op *OutboxProcessor) isPermanentError(statusCode int) bool {
	for _, code := range op.retryConfig.PermanentErrors {
		if statusCode == code {
			return true
		}
	}
	return false
}

// trackDeliveryStatus records the delivery attempt in storage
func (op *OutboxProcessor) trackDeliveryStatus(ctx context.Context, msg ActivityDeliveryMessage, result DeliveryResult) error {
	now := time.Now()

	// Calculate payload size
	payloadBytes, err := json.Marshal(msg.Activity)
	payloadSize := int64(len(payloadBytes))
	if err != nil {
		// Fallback to ID length if marshaling fails
		payloadSize = int64(len(msg.Activity.ID))
	}

	// Record the federation delivery status using DynamORM repository
	federationActivity := &models.FederationActivity{
		Domain:       extractDomainFromURL(msg.TargetInbox),
		ActivityType: msg.Activity.Type,
		OutboundSize: payloadSize, // This is outbound federation
		Success:      result.Success,
		ResponseTime: float64(result.Duration.Milliseconds()),
		ErrorMessage: "",
		Timestamp:    now,
	}

	if result.Error != nil {
		federationActivity.ErrorMessage = result.Error.Error()
	}

	if err := op.federationActivityRepository.Create(ctx, federationActivity); err != nil {
		op.logger.Error("failed to record federation delivery status",
			zap.String("activity_id", msg.Activity.ID),
			zap.Error(err),
		)
		return federationDeliveryStatusRecordFailed()
	}

	op.logger.Info("federation delivery status recorded",
		zap.String("activity_id", msg.Activity.ID),
		zap.String("target_inbox", msg.TargetInbox),
		zap.Bool("success", result.Success),
		zap.Int("status_code", result.StatusCode),
		zap.Int("attempts", result.Attempt),
	)

	return nil
}

// recordComprehensiveCostTracking records comprehensive cost tracking for outbound federation
func (op *OutboxProcessor) recordComprehensiveCostTracking(msg ActivityDeliveryMessage, result DeliveryResult, costParams *federation.CostCalculationParams, totalDuration time.Duration) {
	// Update cost parameters with final results
	costParams.Success = result.Success
	costParams.ResponseTimeMs = totalDuration.Milliseconds()
	costParams.LambdaDurationMs = totalDuration.Milliseconds()
	costParams.ProcessingTimeMs = result.Duration.Milliseconds()
	costParams.RetryCount = result.Attempt - 1
	costParams.DynamoDBWriteCount = 2 // Delivery status + cost tracking

	if result.Error != nil {
		costParams.ErrorMessage = result.Error.Error()
	}

	// Add DNS lookup for delivery
	costParams.DNSLookupCount = 1

	// Calculate comprehensive costs
	cost := op.costCalculator.CalculateFederationCosts(costParams)

	// Record cost tracking asynchronously
	go func() {
		// Record detailed cost tracking
		if err := op.federationCostRepository.RecordFederationCost(context.Background(), cost); err != nil {
			op.logger.Warn("failed to record federation cost", zap.Error(err))
		}

		// Update budget usage for this domain
		if err := op.federationCostRepository.UpdateBudgetUsage(context.Background(),
			costParams.Domain, "daily", costParams.ActivityType, "outbound", cost.TotalCostMicroCents); err != nil {
			op.logger.Warn("failed to update budget usage", zap.Error(err))
		}
	}()

	op.logger.Info("comprehensive federation cost recorded",
		zap.String("activity_type", msg.Activity.Type),
		zap.String("target_domain", costParams.Domain),
		zap.Int64("total_cost_micro_cents", cost.TotalCostMicroCents),
		zap.Float64("total_cost_dollars", cost.GetTotalCostDollars()),
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

	// Return as-is if no protocol prefix
	return urlStr
}

// HandleOutboxPost handles POST requests to the ActivityPub outbox endpoint
func (op *OutboxProcessor) HandleOutboxPost(ctx *lift.Context) error {
	requestID, _ := ctx.Get("requestID").(string)
	if err := common.ValidateRequiredParam("requestID", requestID); err != nil {
		requestID = fmt.Sprintf("outbox-post-%d", time.Now().UnixNano())
		ctx.Set("requestID", requestID)
	}

	username := ctx.Param("username")
	if err := common.ValidateRequiredParam("username", username); err != nil {
		op.logger.Warn("missing username parameter", zap.String("request_id", requestID))
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error": "missing username parameter",
		})
	}

	op.logger.Info("processing outbox POST request",
		zap.String("request_id", requestID),
		zap.String("username", username),
	)

	// Authenticate request
	claims, actor, err := op.authenticateOutboxRequest(ctx, username)
	if err != nil {
		return err
	}

	op.logger.Info("authenticated outbox request",
		zap.String("request_id", requestID),
		zap.String("username", claims.Username),
		zap.String("actor_id", actor.ID),
	)

	// Parse the activity from request body
	activity, err := op.parseActivityFromRequest(ctx)
	if err != nil {
		return err
	}

	op.logger.Info("parsed activity from request",
		zap.String("request_id", requestID),
		zap.String("activity_type", activity.Type),
		zap.String("activity_id", activity.ID),
	)

	// Set the actor and published timestamp if not set
	activity.Actor = actor.ID
	if activity.Published == nil {
		now := time.Now()
		activity.Published = &now
	}

	// Generate activity ID if not provided
	if err := common.ValidateRequiredParam("activity.ID", activity.ID); err != nil {
		activity.ID = fmt.Sprintf("%s/activities/%s-%d-%s",
			actor.ID,
			strings.ToLower(activity.Type),
			time.Now().Unix(),
			generateRandomStringOutbox())
	}

	// Store the activity in the outbox
	if err := op.activityRepository.CreateActivity(ctx.Request.Context(), activity); err != nil {
		op.logger.Error("failed to store activity in outbox",
			zap.String("request_id", requestID),
			zap.String("activity_id", activity.ID),
			zap.Error(err),
		)
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{
			"error": "failed to store activity",
		})
	}

	// Trigger federation delivery based on activity type and addressing
	if err := op.triggerFederationDelivery(ctx.Request.Context(), activity, actor); err != nil {
		op.logger.Warn("failed to trigger federation delivery",
			zap.String("request_id", requestID),
			zap.String("activity_id", activity.ID),
			zap.Error(err),
		)
		// Don't fail the request - federation delivery will be retried
	}

	op.logger.Info("outbox POST completed successfully",
		zap.String("request_id", requestID),
		zap.String("activity_id", activity.ID),
		zap.String("activity_type", activity.Type),
	)

	// Return the activity as JSON
	ctx.Status(http.StatusCreated)
	return ctx.JSON(activity)
}

// authenticateOutboxRequest authenticates the request and verifies actor matching
func (op *OutboxProcessor) authenticateOutboxRequest(ctx *lift.Context, username string) (*auth.Claims, *activitypub.Actor, error) {
	// Extract Bearer token
	token := op.getBearerToken(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		op.logger.Warn("missing authentication token")
		return nil, nil, ctx.Status(http.StatusUnauthorized).JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate the token directly using JWT parsing (avoiding complex storage interface)
	claims, err := op.validateJWTToken(token)
	if err != nil {
		op.logger.Warn("invalid access token", zap.Error(err))
		return nil, nil, ctx.Status(http.StatusUnauthorized).JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Verify write scope
	if !claims.HasScope(auth.ScopeWrite) {
		op.logger.Warn("insufficient scope", zap.String("username", claims.Username))
		return nil, nil, ctx.Status(http.StatusForbidden).JSON(map[string]string{
			"error": "insufficient scope - write access required",
		})
	}

	// Verify the authenticated user matches the username in the path
	if claims.Username != username {
		op.logger.Warn("username mismatch",
			zap.String("token_username", claims.Username),
			zap.String("path_username", username),
		)
		return nil, nil, ctx.Status(http.StatusForbidden).JSON(map[string]string{
			"error": "cannot post to another user's outbox",
		})
	}

	// Get the actor for the authenticated user
	actor, err := op.actorRepository.GetActor(ctx.Request.Context(), username)
	if err != nil {
		op.logger.Error("failed to get actor", zap.String("username", username), zap.Error(err))
		return nil, nil, ctx.Status(http.StatusInternalServerError).JSON(map[string]string{
			"error": "failed to get actor information",
		})
	}

	return claims, actor, nil
}

// getBearerToken extracts Bearer token from Authorization header
func (op *OutboxProcessor) getBearerToken(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		authHeader = ctx.Header("authorization")
	}

	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		return ""
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return ""
	}

	return token
}

// parseActivityFromRequest parses the ActivityPub activity from the request body
func (op *OutboxProcessor) parseActivityFromRequest(ctx *lift.Context) (*activitypub.Activity, error) {
	var activity activitypub.Activity

	// Parse the request body as JSON
	if err := ctx.ParseRequest(&activity); err != nil {
		op.logger.Error("failed to parse activity JSON", zap.Error(err))
		return nil, ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error": "invalid JSON activity",
		})
	}

	// Validate required fields
	if err := common.ValidateRequiredParam("activityType", activity.Type); err != nil {
		return nil, ctx.Status(http.StatusUnprocessableEntity).JSON(map[string]string{
			"error": "activity type is required",
		})
	}

	// Validate ActivityPub activity structure
	activityMap := map[string]interface{}{
		"id":    activity.ID,
		"type":  activity.Type,
		"actor": activity.Actor,
		"to":    activity.To,
		"cc":    activity.CC,
	}
	if err := common.ValidateActivityPubActivity(activityMap); err != nil {
		op.logger.Warn("invalid ActivityPub activity", zap.Error(err))
		return nil, ctx.Status(http.StatusUnprocessableEntity).JSON(map[string]string{
			"error": fmt.Sprintf("invalid activity format: %v", err),
		})
	}

	// Validate activity type
	validTypes := []string{
		activitypub.CreateType,
		activitypub.UpdateType,
		activitypub.DeleteType,
		activitypub.FollowType,
		activitypub.UndoType,
		activitypub.LikeType,
		activitypub.AnnounceType,
		activitypub.AcceptType,
		activitypub.RejectType,
	}

	validType := false
	for _, vt := range validTypes {
		if activity.Type == vt {
			validType = true
			break
		}
	}

	if !validType {
		return nil, ctx.Status(http.StatusUnprocessableEntity).JSON(map[string]string{
			"error": fmt.Sprintf("unsupported activity type: %s", activity.Type),
		})
	}

	return &activity, nil
}

// triggerFederationDelivery triggers appropriate federation delivery based on activity
func (op *OutboxProcessor) triggerFederationDelivery(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	switch activity.Type {
	case activitypub.CreateType, activitypub.UpdateType, activitypub.AnnounceType:
		// Public/unlisted content should be delivered to followers
		return op.deliverToFollowersAndRecipients(ctx, activity, actor)

	case activitypub.FollowType, activitypub.LikeType:
		// Targeted activities should be delivered to specific recipients
		return op.federationService.DeliverToRecipients(ctx, activity, actor)

	case activitypub.DeleteType, activitypub.UndoType:
		// Deletions and undos should be delivered broadly
		return op.deliverToFollowersAndRecipients(ctx, activity, actor)

	case activitypub.AcceptType, activitypub.RejectType:
		// Accept/Reject should be delivered to specific recipients
		return op.federationService.DeliverToRecipients(ctx, activity, actor)

	default:
		op.logger.Info("no federation delivery configured for activity type",
			zap.String("activity_type", activity.Type),
			zap.String("activity_id", activity.ID),
		)
		return nil
	}
}

// deliverToFollowersAndRecipients delivers to both followers and specific recipients
func (op *OutboxProcessor) deliverToFollowersAndRecipients(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	// Deliver to followers if this is public/unlisted content
	if op.isPublicOrUnlisted(activity) {
		if err := op.federationService.DeliverToFollowers(ctx, activity, actor); err != nil {
			op.logger.Error("failed to deliver to followers", zap.Error(err))
			// Continue to deliver to specific recipients
		}
	}

	// Also deliver to specific recipients (mentions, replies, etc.)
	if err := op.federationService.DeliverToRecipients(ctx, activity, actor); err != nil {
		op.logger.Error("failed to deliver to recipients", zap.Error(err))
		return err
	}

	return nil
}

// isPublicOrUnlisted checks if the activity is public or unlisted
func (op *OutboxProcessor) isPublicOrUnlisted(activity *activitypub.Activity) bool {
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

// generateRandomStringOutbox generates a random string for activity IDs
func generateRandomStringOutbox() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

// validateJWTToken validates a JWT token and returns claims
func (op *OutboxProcessor) validateJWTToken(tokenString string) (*auth.Claims, error) {
	// Extract config with reflection or direct import for now
	// This will be improved when config interface is standardized
	cfg := op.lambdaCtx.Config

	token, err := jwt.ParseWithClaims(tokenString, &auth.Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, unexpectedJWTSigningMethod()
		}
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, jwtTokenParsingFailed(err)
	}

	if claims, ok := token.Claims.(*auth.Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, invalidToken()
}

func main() {
	processor, err := NewOutboxProcessor()
	if err != nil {
		panic(outboxProcessorInitializationFailed())
	}

	app := lift.New()

	// Use standardized Lambda handler wrapper for observability
	standardHandler := processor.lambdaCtx.CreateStandardizedLambdaHandler

	// Add request ID middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("outbox-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Add logging middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			err := next.Handle(ctx)

			processor.logger.Info("outbox request completed",
				zap.String("request_id", ctx.Get("requestID").(string)),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("has_error", err != nil),
			)

			if err != nil {
				processor.logger.Error("outbox handler error",
					zap.String("request_id", ctx.Get("requestID").(string)),
					zap.Error(err),
				)
			}
			return err
		})
	})

	// Add error handling middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			err := next.Handle(ctx)
			if err != nil {
				// Determine if this is a retryable error
				if liftErr, ok := err.(*lift.LiftError); ok && liftErr.StatusCode >= 500 {
					// 5xx errors are typically retryable
					return err
				}
				if liftErr, ok := err.(*lift.LiftError); ok && liftErr.StatusCode >= 400 {
					// 4xx errors are typically permanent
					processor.logger.Warn("permanent error in outbox processing",
						zap.Int("status_code", liftErr.StatusCode),
						zap.String("message", liftErr.Message),
					)
				}
			}
			return err
		})
	})

	// Set SQS handler for federation delivery
	_ = app.SQS("outbox-delivery", func(ctx *lift.Context) error {
		// Extract SQS event from Lift context - proper implementation
		if ctx.Request.RawEvent == nil {
			return lift.NewLiftError("MISSING_EVENT", "no SQS event in request", 400)
		}

		// Parse the raw event as SQS event
		var event events.SQSEvent
		if sqsEvent, ok := ctx.Request.RawEvent.(events.SQSEvent); ok {
			event = sqsEvent
		} else {
			// Try to parse from interface if it's a map
			eventBytes, err := json.Marshal(ctx.Request.RawEvent)
			if err != nil {
				return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to marshal raw event", 500).WithCause(err)
			}

			if err := json.Unmarshal(eventBytes, &event); err != nil {
				return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to parse SQS event", 500).WithCause(err)
			}
		}

		return processor.HandleSQS(ctx, event)
	})

	// Add REST API handlers for ActivityPub outbox endpoint
	_ = app.POST("/users/:username/outbox", func(ctx *lift.Context) error {
		return processor.HandleOutboxPost(ctx)
	})

	// Wrap the main Lambda handler with standardized observability
	lambda.Start(standardHandler(func(ctx context.Context, event interface{}) (interface{}, error) {
		return app.HandleRequest(ctx, event)
	}))
}
