// Package main implements the outbox Lambda function for serving ActivityPub outbox endpoints.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/golang-jwt/jwt/v5"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

const contentTypeActivityJSON = "application/activity+json"

type outboxFederationService interface {
	DeliverActivity(ctx context.Context, activity *activitypub.Activity, targetInbox string, signingActor *activitypub.Actor) error
	DeliverToFollowers(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error
	DeliverToRecipients(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error
}

type outboxInstanceStateRepository interface {
	GetInstanceState(ctx context.Context) (*models.InstanceState, error)
}

type outboxFederationActivityRepository interface {
	Create(ctx context.Context, activity *models.FederationActivity) error
}

type outboxFederationCostRepository interface {
	RecordFederationCost(ctx context.Context, cost *models.FederationCostTracking) error
	UpdateBudgetUsage(ctx context.Context, domain, period, activityType, direction string, cost int64) error
	CheckBudgetLimits(ctx context.Context, domain, period, activityType, direction string, estimatedCost int64) (*repositories.BudgetCheckResult, error)
}

// OutboxProcessor handles ActivityPub federation delivery via SQS
type OutboxProcessor struct {
	federationService            outboxFederationService
	db                           dynamormCore.DB
	actorRepository              interfaces.ActorRepository
	activityRepository           interfaces.ActivityRepository
	instanceRepository           outboxInstanceStateRepository
	federationActivityRepository outboxFederationActivityRepository
	federationCostRepository     outboxFederationCostRepository
	logger                       *zap.Logger
	cfg                          interface{} // config.Config interface
	httpClient                   *http.Client
	retryConfig                  RetryConfig
	costCalculator               *federation.CostCalculator
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
		db:                           repos.GetDB(),
		actorRepository:              actorRepo,
		activityRepository:           activityRepo,
		instanceRepository:           repos.Instance(),
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
		lambdaCtx:      lambdaCtx,
	}, nil
}

// HandleSQSMessage processes a single ActivityPub federation message from SQS.
//
// AppTheory handles the batch semantics; this function is invoked per record.
func (op *OutboxProcessor) HandleSQSMessage(ctx context.Context, requestID string, msg events.SQSMessage) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(requestID) == "" {
		requestID = fmt.Sprintf("outbox-%s", msg.MessageId)
		if strings.TrimSpace(requestID) == "outbox-" {
			requestID = fmt.Sprintf("outbox-%d", time.Now().UnixNano())
		}
	}

	op.logger.Info("processing outbox federation message",
		zap.String("request_id", requestID),
		zap.String("message_id", msg.MessageId),
	)

	if err := op.processMessage(ctx, requestID, msg); err != nil {
		op.logger.Error("failed to process federation message",
			zap.String("message_id", msg.MessageId),
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		return err
	}

	return nil
}

// processMessage processes a single SQS federation message
func (op *OutboxProcessor) processMessage(ctx context.Context, requestID string, msg events.SQSMessage) error {
	if ctx == nil {
		ctx = context.Background()
	}
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
	budgetCheck, err := op.federationCostRepository.CheckBudgetLimits(ctx,
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
		outboxRunAsync(func() {
			if err := op.federationCostRepository.RecordFederationCost(context.Background(), cost); err != nil {
				op.logger.Warn("failed to record federation cost", zap.Error(err))
			}
		})

		return deliveryBudgetLimitExceeded()
	}

	// Attempt delivery with retry logic
	result := op.deliverActivityWithRetry(ctx, deliveryMsg, costParams)

	// Record comprehensive cost tracking and metrics
	op.recordComprehensiveCostTracking(deliveryMsg, result, costParams, time.Since(start))

	// Track delivery status (simplified)
	if err := op.trackDeliveryStatus(ctx, deliveryMsg, result); err != nil {
		op.logger.Warn("failed to track delivery status",
			zap.String("message_id", msg.MessageId),
			zap.String("request_id", requestID),
			zap.Error(err),
		)
	}

	// Return error for temporary failures to trigger SQS retry
	if !result.Success && !op.isPermanentError(result.StatusCode) {
		return deliveryRetryableFailure(result.Error)
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
			outboxSleep(delay)
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
	outboxRunAsync(func() {
		// Record detailed cost tracking
		if err := op.federationCostRepository.RecordFederationCost(context.Background(), cost); err != nil {
			op.logger.Warn("failed to record federation cost", zap.Error(err))
		}

		// Update budget usage for this domain
		if err := op.federationCostRepository.UpdateBudgetUsage(context.Background(),
			costParams.Domain, "daily", costParams.ActivityType, "outbound", cost.TotalCostMicroCents); err != nil {
			op.logger.Warn("failed to update budget usage", zap.Error(err))
		}
	})

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

func outboxQueryValue(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	values := ctx.Request.Query[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func outboxHeaderValue(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	key = strings.ToLower(strings.TrimSpace(key))
	values := ctx.Request.Headers[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func outboxRequestID(ctx *apptheory.Context, prefix string) string {
	if ctx != nil && strings.TrimSpace(ctx.RequestID) != "" {
		return ctx.RequestID
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "outbox"
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func outboxJSONError(status int, message string) *apptheory.Response {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "internal server error"
	}
	return apptheory.MustJSON(status, map[string]string{"error": message})
}

func outboxActivityJSON(status int, value any) (*apptheory.Response, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &apptheory.Response{
		Status: status,
		Headers: map[string][]string{
			"content-type": {contentTypeActivityJSON},
		},
		Body: body,
	}, nil
}

// HandleOutboxGet handles GET requests to the ActivityPub outbox endpoint.
func (op *OutboxProcessor) HandleOutboxGet(ctx *apptheory.Context) (*apptheory.Response, error) {
	requestID := outboxRequestID(ctx, "outbox-get")

	username := ""
	if ctx != nil {
		username = ctx.Param("username")
	}
	if err := common.ValidateRequiredParam("username", username); err != nil {
		op.logger.Warn("missing username parameter", zap.String("request_id", requestID))
		return outboxJSONError(http.StatusBadRequest, "missing username parameter"), nil
	}

	runCtx := context.Background()
	if ctx != nil {
		runCtx = ctx.Context()
	}

	state, stateErr := op.instanceRepository.GetInstanceState(runCtx)
	bootstrapUsername := models.DefaultBootstrapUsername
	if stateErr == nil && strings.TrimSpace(state.BootstrapUsername) != "" {
		bootstrapUsername = strings.TrimSpace(state.BootstrapUsername)
	}
	locked := stateErr != nil || state.Locked
	if locked && strings.EqualFold(username, bootstrapUsername) {
		return outboxJSONError(http.StatusForbidden, "bootstrap actor is not available while instance is locked"), nil
	}

	actor, err := op.actorRepository.GetActorByUsername(runCtx, username)
	if err != nil {
		if common.IsNotFound(err) {
			return outboxJSONError(http.StatusNotFound, "actor not found"), nil
		}
		op.logger.Error("failed to get actor for outbox GET",
			zap.String("request_id", requestID),
			zap.String("username", username),
			zap.Error(err),
		)
		return outboxJSONError(http.StatusInternalServerError, "internal server error"), nil
	}

	outboxID := actor.Outbox
	if outboxID == "" {
		outboxID = fmt.Sprintf("%s/users/%s/outbox", op.lambdaCtx.Config.BaseURL(), username)
	}

	page := outboxQueryValue(ctx, "page")
	limit, err := common.ParseAndValidateActivityPubLimit(outboxQueryValue(ctx, "limit"))
	if err != nil {
		limit = 20
	}
	cursor := outboxQueryValue(ctx, "cursor")

	// While locked, outbox should be reachable but empty for all non-bootstrap actors.
	if locked {
		// Collection response (metadata) when `page=true` is not set.
		if page != "true" && page != "1" {
			collection := &activitypub.OrderedCollection{
				Collection: activitypub.Collection{
					BaseObject: activitypub.BaseObject{
						Context: activitypub.Context,
						ID:      outboxID,
						Type:    activitypub.OrderedCollectionType,
					},
					TotalItems: 0,
					First:      fmt.Sprintf("%s?page=true", outboxID),
				},
			}

			return outboxActivityJSON(http.StatusOK, collection)
		}

		collectionPage := &activitypub.OrderedCollectionPage{
			CollectionPage: activitypub.CollectionPage{
				Collection: activitypub.Collection{
					BaseObject: activitypub.BaseObject{
						Context: activitypub.Context,
						ID:      fmt.Sprintf("%s?page=true", outboxID),
						Type:    activitypub.OrderedCollectionPageType,
					},
					OrderedItems: []any{},
				},
				PartOf: outboxID,
			},
		}

		return outboxActivityJSON(http.StatusOK, collectionPage)
	}

	// Collection response (metadata) when `page=true` is not set.
	if page != "true" && page != "1" {
		totalItems, countErr := op.countOutboxActivities(runCtx, username)
		if countErr != nil {
			op.logger.Warn("failed to count outbox activities",
				zap.String("request_id", requestID),
				zap.String("username", username),
				zap.Error(countErr),
			)
			totalItems = 0
		}

		collection := &activitypub.OrderedCollection{
			Collection: activitypub.Collection{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					ID:      outboxID,
					Type:    activitypub.OrderedCollectionType,
				},
				TotalItems: totalItems,
				First:      fmt.Sprintf("%s?page=true", outboxID),
			},
		}

		return outboxActivityJSON(http.StatusOK, collection)
	}

	activities, nextCursor, err := op.activityRepository.GetOutboxActivities(runCtx, username, limit, cursor)
	if err != nil {
		op.logger.Error("failed to get outbox activities",
			zap.String("request_id", requestID),
			zap.String("username", username),
			zap.Error(err),
		)
		return outboxJSONError(http.StatusInternalServerError, "failed to get outbox activities"), nil
	}

	publicActivities := filterPublicOutboxActivities(activities)
	orderedItems := make([]any, len(publicActivities))
	for i, activity := range publicActivities {
		orderedItems[i] = activity
	}

	collectionPage := &activitypub.OrderedCollectionPage{
		CollectionPage: activitypub.CollectionPage{
			Collection: activitypub.Collection{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					ID:      fmt.Sprintf("%s?page=true", outboxID),
					Type:    activitypub.OrderedCollectionPageType,
				},
				OrderedItems: orderedItems,
			},
			PartOf: outboxID,
		},
	}

	if nextCursor != "" {
		collectionPage.Next = fmt.Sprintf("%s?page=true&cursor=%s&limit=%d", outboxID, url.QueryEscape(nextCursor), limit)
	}
	if cursor != "" {
		collectionPage.Prev = fmt.Sprintf("%s?page=true&limit=%d", outboxID, limit)
	}

	return outboxActivityJSON(http.StatusOK, collectionPage)
}

func (op *OutboxProcessor) countOutboxActivities(ctx context.Context, username string) (int, error) {
	pk := "ACTOR#" + username
	const skPrefix = "ACTIVITY#"

	var activities []*models.Activity
	err := op.db.WithContext(ctx).Model(&models.Activity{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", skPrefix).
		OrderBy("SK", "DESC").
		All(&activities)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, record := range activities {
		if record != nil && activitypub.IsPublicAddressedActivity(record.Activity) {
			count++
		}
	}
	return count, nil
}

func filterPublicOutboxActivities(activities []*activitypub.Activity) []*activitypub.Activity {
	if len(activities) == 0 {
		return activities
	}

	filtered := make([]*activitypub.Activity, 0, len(activities))
	for _, activity := range activities {
		if activitypub.IsPublicAddressedActivity(activity) {
			filtered = append(filtered, activity)
		}
	}
	return filtered
}

// HandleOutboxPost handles POST requests to the ActivityPub outbox endpoint
func (op *OutboxProcessor) HandleOutboxPost(ctx *apptheory.Context) (*apptheory.Response, error) {
	requestID := outboxRequestID(ctx, "outbox-post")

	username := ""
	if ctx != nil {
		username = ctx.Param("username")
	}
	if err := common.ValidateRequiredParam("username", username); err != nil {
		op.logger.Warn("missing username parameter", zap.String("request_id", requestID))
		return outboxJSONError(http.StatusBadRequest, "missing username parameter"), nil
	}

	runCtx := context.Background()
	if ctx != nil {
		runCtx = ctx.Context()
	}

	// Block publishing until instance activation completes.
	state, err := op.instanceRepository.GetInstanceState(runCtx)
	if err != nil {
		op.logger.Warn("failed to get instance lock state; defaulting to locked",
			zap.String("request_id", requestID),
			zap.Error(err))
		return outboxJSONError(http.StatusForbidden, "instance is locked"), nil
	}
	if state.Locked {
		return outboxJSONError(http.StatusForbidden, "instance is locked"), nil
	}

	op.logger.Info("processing outbox POST request",
		zap.String("request_id", requestID),
		zap.String("username", username),
	)

	// Authenticate request
	claims, actor, authResp := op.authenticateOutboxRequest(ctx, username)
	if authResp != nil {
		return authResp, nil
	}

	op.logger.Info("authenticated outbox request",
		zap.String("request_id", requestID),
		zap.String("username", claims.Username),
		zap.String("actor_id", actor.ID),
	)

	// Parse the activity from request body
	activity, parseResp := op.parseActivityFromRequest(ctx)
	if parseResp != nil {
		return parseResp, nil
	}

	op.logger.Info("parsed activity from request",
		zap.String("request_id", requestID),
		zap.String("activity_type", activity.Type),
		zap.String("activity_id", activity.ID),
	)

	// Set the actor and published timestamp if not set
	activity.Actor = actor.ID
	if len(activity.Context) == 0 {
		activity.Context = activitypub.DefaultContext
	}
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

	if err := common.ValidateActivityPubActivity(outboxActivityValidationMap(activity)); err != nil {
		op.logger.Warn("invalid ActivityPub activity", zap.Error(err))
		return outboxJSONError(http.StatusUnprocessableEntity, fmt.Sprintf("invalid activity format: %v", err)), nil
	}

	// Store the activity in the outbox
	if err := op.activityRepository.CreateActivity(runCtx, activity); err != nil {
		op.logger.Error("failed to store activity in outbox",
			zap.String("request_id", requestID),
			zap.String("activity_id", activity.ID),
			zap.Error(err),
		)
		return outboxJSONError(http.StatusInternalServerError, "failed to store activity"), nil
	}

	// Trigger federation delivery based on activity type and addressing
	if err := op.triggerFederationDelivery(runCtx, activity, actor); err != nil {
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
	return outboxActivityJSON(http.StatusCreated, activity)
}

// authenticateOutboxRequest authenticates the request and verifies actor matching
func (op *OutboxProcessor) authenticateOutboxRequest(ctx *apptheory.Context, username string) (*auth.Claims, *activitypub.Actor, *apptheory.Response) {
	// Extract Bearer token
	token := op.getBearerToken(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		op.logger.Warn("missing authentication token")
		return nil, nil, outboxJSONError(http.StatusUnauthorized, "authentication required")
	}

	// Validate the token directly using JWT parsing (avoiding complex storage interface)
	claims, err := op.validateJWTToken(token)
	if err != nil {
		op.logger.Warn("invalid access token", zap.Error(err))
		return nil, nil, outboxJSONError(http.StatusUnauthorized, "invalid token")
	}

	// Verify write scope
	if !claims.HasScope(auth.ScopeWrite) {
		op.logger.Warn("insufficient scope", zap.String("username", claims.Username))
		return nil, nil, outboxJSONError(http.StatusForbidden, "insufficient scope - write access required")
	}

	// Verify the authenticated user matches the username in the path
	if claims.Username != username {
		op.logger.Warn("username mismatch",
			zap.String("auth_username", claims.Username),
			zap.String("path_username", username),
		)
		return nil, nil, outboxJSONError(http.StatusForbidden, "cannot post to another user's outbox")
	}

	// Get the actor for the authenticated user
	runCtx := context.Background()
	if ctx != nil {
		runCtx = ctx.Context()
	}

	actor, err := op.actorRepository.GetActor(runCtx, username)
	if err != nil {
		op.logger.Error("failed to get actor", zap.String("username", username), zap.Error(err))
		return nil, nil, outboxJSONError(http.StatusInternalServerError, "failed to get actor information")
	}

	return claims, actor, nil
}

// getBearerToken extracts Bearer token from Authorization header
func (op *OutboxProcessor) getBearerToken(ctx *apptheory.Context) string {
	authHeader := outboxHeaderValue(ctx, "authorization")
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
func (op *OutboxProcessor) parseActivityFromRequest(ctx *apptheory.Context) (*activitypub.Activity, *apptheory.Response) {
	var activity activitypub.Activity

	if ctx == nil || len(ctx.Request.Body) == 0 {
		return nil, outboxJSONError(http.StatusBadRequest, "invalid JSON activity")
	}
	if len(ctx.Request.Body) > common.MaxActivitySize {
		op.logger.Warn("outbox activity body too large", zap.Int("size", len(ctx.Request.Body)), zap.Int("max_size", common.MaxActivitySize))
		return nil, outboxJSONError(http.StatusRequestEntityTooLarge, "activity body too large")
	}
	if err := common.ParseActivityPubObject(ctx.Request.Body, &activity); err != nil {
		op.logger.Error("failed to safely parse activity JSON", zap.Error(err))
		return nil, outboxJSONError(http.StatusBadRequest, "invalid JSON activity")
	}

	// Validate required fields
	if err := common.ValidateRequiredParam("activityType", activity.Type); err != nil {
		return nil, outboxJSONError(http.StatusUnprocessableEntity, "activity type is required")
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
		return nil, outboxJSONError(http.StatusUnprocessableEntity, fmt.Sprintf("unsupported activity type: %s", activity.Type))
	}

	return &activity, nil
}

func outboxActivityValidationMap(activity *activitypub.Activity) map[string]interface{} {
	payload := map[string]interface{}{
		"id":    activity.ID,
		"type":  activity.Type,
		"actor": activity.Actor,
	}

	if activity.Object != nil {
		payload["object"] = activity.Object
	}

	if len(activity.Context) > 0 {
		payload["@context"] = []any(activity.Context)
	}

	if len(activity.To) > 0 {
		to := make([]any, 0, len(activity.To))
		for _, entry := range activity.To {
			to = append(to, entry)
		}
		payload["to"] = to
	}
	if len(activity.CC) > 0 {
		cc := make([]any, 0, len(activity.CC))
		for _, entry := range activity.CC {
			cc = append(cc, entry)
		}
		payload["cc"] = cc
	}
	if len(activity.BTo) > 0 {
		bto := make([]any, 0, len(activity.BTo))
		for _, entry := range activity.BTo {
			bto = append(bto, entry)
		}
		payload["bto"] = bto
	}
	if len(activity.BCC) > 0 {
		bcc := make([]any, 0, len(activity.BCC))
		for _, entry := range activity.BCC {
			bcc = append(bcc, entry)
		}
		payload["bcc"] = bcc
	}

	return payload
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

var (
	outboxSleep    = time.Sleep
	outboxRunAsync = func(fn func()) { go fn() }
	newOutboxProc  = NewOutboxProcessor
	startLambda    = lambda.Start
)

func buildOutboxApp(processor *OutboxProcessor) *apptheory.App {
	app := apptheory.New()

	app.Use(func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ctx != nil {
				ctx.Set("requestID", outboxRequestID(ctx, "outbox"))
			}
			return next(ctx)
		}
	})

	app.Use(func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			start := time.Now()
			resp, err := next(ctx)

			requestID := ""
			if ctx != nil {
				if rid, ok := ctx.Get("requestID").(string); ok {
					requestID = rid
				}
			}

			processor.logger.Info("outbox request completed",
				zap.String("request_id", requestID),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("has_error", err != nil),
			)

			if err != nil {
				processor.logger.Error("outbox handler error",
					zap.String("request_id", requestID),
					zap.Error(err),
				)
			}

			return resp, err
		}
	})

	app.SQS("outbox-delivery", func(evtCtx *apptheory.EventContext, msg events.SQSMessage) error {
		requestID := ""
		runCtx := context.Background()
		if evtCtx != nil {
			requestID = evtCtx.RequestID
			runCtx = evtCtx.Context()
		}
		return processor.HandleSQSMessage(runCtx, requestID, msg)
	})

	app.Get("/users/:username/outbox", processor.HandleOutboxGet)
	app.Post("/users/:username/outbox", processor.HandleOutboxPost)

	return app
}

func main() {
	processor, err := newOutboxProc()
	if err != nil {
		panic(outboxProcessorInitializationFailed())
	}

	app := buildOutboxApp(processor)

	startLambda(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}
