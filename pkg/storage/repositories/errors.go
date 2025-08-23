package repositories

import (
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/storage"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
)

// ErrorUtils provides utilities for standardizing error handling across repositories
type ErrorUtils struct{}

// NewErrorUtils creates a new ErrorUtils instance
func NewErrorUtils() *ErrorUtils {
	return &ErrorUtils{}
}

// HandleNotFound converts DynamORM not found errors to domain-specific errors
func (e *ErrorUtils) HandleNotFound(err error, entityType, identifier string) error {
	if dynamormErrors.IsNotFound(err) {
		return fmt.Errorf("%w: %s %s", ErrEntityNotFound, entityType, identifier)
	}
	return err
}

// HandleGetError standardizes error handling for Get operations
func (e *ErrorUtils) HandleGetError(err error, entityType, identifier string) error {
	if err == nil {
		return nil
	}

	if dynamormErrors.IsNotFound(err) {
		return fmt.Errorf("%w: %s %s", ErrEntityNotFound, entityType, identifier)
	}

	return fmt.Errorf("%w: %s: %w", ErrFailedToGet, entityType, err)
}

// HandleCreateError standardizes error handling for Create operations
func (e *ErrorUtils) HandleCreateError(err error, entityType, identifier string) error {
	if err == nil {
		return nil
	}

	if dynamormErrors.IsConditionFailed(err) {
		return fmt.Errorf("%w: %s %s", ErrEntityAlreadyExists, entityType, identifier)
	}

	return fmt.Errorf("%w: %s: %w", ErrFailedToCreate, entityType, err)
}

// HandleUpdateError standardizes error handling for Update operations
func (e *ErrorUtils) HandleUpdateError(err error, entityType, identifier string) error {
	if err == nil {
		return nil
	}

	if dynamormErrors.IsNotFound(err) {
		return fmt.Errorf("%w: %s %s", ErrEntityNotFoundForUpdate, entityType, identifier)
	}

	return fmt.Errorf("%w: %s: %w", ErrFailedToUpdate, entityType, err)
}

// HandleDeleteError standardizes error handling for Delete operations
func (e *ErrorUtils) HandleDeleteError(err error, entityType, _ string) error {
	if err == nil {
		return nil
	}

	// For deletes, we typically don't treat "not found" as an error
	if dynamormErrors.IsNotFound(err) {
		return nil
	}

	return fmt.Errorf("%w: %s: %w", ErrFailedToDelete, entityType, err)
}

// HandleQueryError standardizes error handling for Query operations
func (e *ErrorUtils) HandleQueryError(err error, entityType, queryType string) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%w: %s (%s): %w", ErrFailedToQuery, entityType, queryType, err)
}

// IsConditionalCheckFailed checks if error is a conditional check failure
func (e *ErrorUtils) IsConditionalCheckFailed(err error) bool {
	return dynamormErrors.IsConditionFailed(err)
}

// IsNotFound checks if error is a not found error
func (e *ErrorUtils) IsNotFound(err error) bool {
	return dynamormErrors.IsNotFound(err)
}

// Common entity type constants for consistent error messages
const (
	EntityUser               = "user"
	EntityActor              = "actor"
	EntityAlert              = "alert"
	EntityObject             = "object"
	EntityFollow             = "follow"
	EntityBlock              = "block"
	EntityMute               = "mute"
	EntityList               = "list"
	EntityHashtag            = "hashtag"
	EntityFeaturedTag        = "featured tag"
	EntityMedia              = "media"
	EntityOAuthState         = "OAuth state"
	EntityAuthCode           = "authorization code"
	EntityRefreshToken       = "refresh token"
	EntityOAuthClient        = "OAuth client"
	EntityOAuthConsent       = "OAuth consent"
	EntityWebAuthnCredential = "WebAuthn credential" //nolint:gosec // This is just an entity name string, not a credential
	EntityWebAuthnChallenge  = "WebAuthn challenge"
	EntityWalletCredential   = "wallet credential" //nolint:gosec // This is just an entity name string, not a credential
	EntityWalletChallenge    = "wallet challenge"
	EntitySession            = "session"
	EntityPasswordReset      = "password reset"
	EntityTimelineEntry      = "timeline entry"
	EntityConversation       = "conversation"
	EntityBookmark           = "bookmark"
	EntityFilter             = "filter"
	EntityFilterKeyword      = "filter keyword"
	EntityFilterStatus       = "filter status"
	EntityReport             = "report"
	EntityFlag               = "flag"
	EntityModerationEvent    = "moderation event"
	EntityModerationDecision = "moderation decision"
	EntityModerationPattern  = "moderation pattern"
	EntityAnnounce           = "announce"
	EntityAccountPin         = "account pin"
	EntityAccountNote        = "account note"
	EntityStatusPin          = "status pin"
	EntityCircuitBreaker     = "circuit breaker"
	EntityCircuitState       = "circuit state"
	EntityCircuitEvent            = "circuit breaker event"
	EntityActivity                = "activity"
	EntityFeature                 = "feature"
	EntityThreatIntel             = "threat intel"
	EntityThreatIndicator         = "threat indicator"
	EntityWebSocketCost           = "websocket cost"
	EntityWebSocketCostBudget     = "websocket cost budget"
	EntityWebSocketCostAggregation = "websocket cost aggregation"
	EntityAudit                   = "audit log"
	EntityAI                      = "ai analysis"
	EntityQueryCache              = "query cache"
	EntityCloudWatchMetrics       = "cloudwatch metrics"
	EntityRateLimit               = "rate limit"
	EntityMarker                  = "marker"
	EntityTrust                   = "trust"
	EntityTrustRelationship       = "trust relationship"
	EntityTrustScore              = "trust score"
	EntityTrustUpdate             = "trust update"
	EntityPublicKeyCache          = "public key cache"
	EntityRoutingMetrics          = "routing metrics"
	EntityNotification            = "notification"
	EntityCSRFToken               = "csrf token"
	EntityStatus                  = "status"
	EntityScheduledStatus         = "scheduled status"
	EntityInstanceDomainBlock     = "instance domain block"
	EntityEmailDomainBlock        = "email domain block"
	EntityDomainAllow             = "domain allow"
	EntityExport                  = "export"
	EntityExportCostTracking      = "export cost tracking"
	EntityImport                  = "import"
	EntityImportCostTracking      = "import cost tracking"
	EntityRecovery                = "recovery"
	EntityRecoveryCode            = "recovery code"
	EntityRecoveryToken           = "recovery token"
	EntityTrustee                 = "trustee"
	EntityRecoveryRequest         = "recovery request"
	EntityInstanceHealth          = "instance health"
	EntityHealthSummary           = "health summary"
	EntityConnectivityTest        = "connectivity test"
	EntityNodeInfo                = "nodeinfo verification"
	EntityWebFinger               = "webfinger resolution"
	EntitySeveredRelationship     = "severed relationship"
	EntityDeliveryRecord          = "delivery record"
	EntityFederationMetrics       = "federation metrics"
	EntitySearchCost              = "search cost"
	EntitySearchBudget            = "search budget"
	EntitySearchMetric            = "search metric"
	EntitySearchEmbedding         = "search embedding"
	EntitySearchSuggestion        = "search suggestion"
	EntitySearchAnalytics         = "search analytics"
	EntityEmoji                   = "emoji"
	EntityRelay                   = "relay"
	EntityFederationCost          = "federation cost"
	EntityFederationBudget        = "federation budget"
	EntityFederationInstance      = "federation instance"
	EntityMediaMetadata           = "media metadata"
	EntityDLQMessage              = "dlq message"
	EntityDNSCache                = "dns cache"
	EntityFederationActivity      = "federation activity"
	EntityQuoteRelationship       = "quote relationship"
	EntityQuotePermissions        = "quote permissions"
)

// Account authentication-specific error constants
var (
	// Validation errors
	ErrAccountValidationFailed   = errors.New("account validation failed")
	ErrDeviceValidationFailed    = errors.New("device validation failed")
	ErrSessionValidationFailed   = errors.New("session validation failed")
	ErrWebAuthnValidationFailed  = errors.New("WebAuthn validation failed")
	ErrWalletValidationFailed    = errors.New("wallet validation failed")
	
	// Not found errors
	ErrDeviceNotFound            = errors.New("device not found")
	ErrWebAuthnCredentialNotFound = errors.New("WebAuthn credential not found")
)

// Account search-specific error constants
var (
	// Validation errors
	ErrAccountSearchInvalidWebfingerFormat = errors.New("invalid webfinger format")
)

// OAuth-specific error constants
var (
	// Validation errors
	ErrOAuthClientNameRequired     = errors.New("client name is required")
	ErrOAuthRedirectURIsRequired   = errors.New("redirect_uris are required")
	ErrOAuthNoUpdatesProvided      = errors.New("no updates provided")
	ErrOAuthClientAlreadyExists    = errors.New("client already exists")
	ErrOAuthStateExpired           = errors.New("OAuth state expired")
)

// Repository operation base error constants
var (
	// Generic operation errors for formatting
	ErrEntityNotFound      = errors.New("entity not found")
	ErrEntityAlreadyExists = errors.New("entity already exists")
	ErrEntityNotFoundForUpdate = errors.New("entity not found for update")
	ErrFailedToGet         = errors.New("failed to get entity")
	ErrFailedToCreate      = errors.New("failed to create entity")
	ErrFailedToUpdate      = errors.New("failed to update entity")
	ErrFailedToDelete      = errors.New("failed to delete entity")
	ErrFailedToQuery       = errors.New("failed to query entity")
	ErrDatabaseOperation   = errors.New("database error")
)

// Query utility-specific error constants
var (
	// Query operation errors
	ErrQueryOperationFailed      = errors.New("query operation failed")
	ErrQueryCollectionAddFailed  = errors.New("failed to add to collection")
	ErrQueryExecutionFailed      = errors.New("query execution failed")
	ErrQueryValidationFailed     = errors.New("query validation failed")
)

// Analytics-specific error constants
var (
	// Type validation errors
	ErrInvalidHashtagTrendType = errors.New("invalid trend type: expected *models.HashtagTrend or *storage.TrendingHashtag")
	ErrInvalidStatusTrendType  = errors.New("invalid trend type: expected *models.StatusTrend or *storage.TrendingStatus")
	ErrInvalidLinkTrendType    = errors.New("invalid trend type: expected *models.LinkTrend or *storage.TrendingLink")

	// Hashtag batch operation errors
	ErrHashtagBatchUnknownModelType = errors.New("unknown model type")

	// Dependency errors
	ErrStatusRepoDependencyMissing = errors.New("statusRepo dependency not available")

	// Parameter validation errors
	ErrInvalidQueryParameters = errors.New("invalid parameters: query cannot be empty and count must be positive")

	// Operation errors
	ErrFailedIndexByEngagement    = errors.New("failed to index by engagement")
	ErrFailedRecordEngagement     = errors.New("failed to record engagement")
	ErrFailedGetEngagementMetrics = errors.New("failed to get engagement metrics")
	ErrFailedGetEngagementByDate  = errors.New("failed to get engagement by date range")
	ErrFailedGetTopContent        = errors.New("failed to get top engaged content")
	ErrFailedUpdateTrendingTag    = errors.New("failed to update trending hashtag")
	ErrFailedGetTrendingTags      = errors.New("failed to get trending hashtags")
	ErrFailedQueryStaleTrends     = errors.New("failed to query stale trends")
	ErrFailedRecordInstanceMetric = errors.New("failed to record instance metric")
	ErrFailedGetInstanceMetrics   = errors.New("failed to get instance metrics")
	ErrFailedGetStartMetric       = errors.New("failed to get start metric")
	ErrFailedGetEndMetric         = errors.New("failed to get end metric")
	ErrFailedRecordManifest       = errors.New("failed to record manifest generation")
	ErrFailedRecordQualityChange  = errors.New("failed to record quality change")
	ErrFailedRecordMediaEvent     = errors.New("failed to record media event")
	ErrFailedQuerySessionEvents   = errors.New("failed to query session events")
	ErrFailedGetModerationAnalytics = errors.New("failed to get existing moderation analytics")
	ErrFailedRecordModerationAction = errors.New("failed to record moderation action")
	ErrFailedGetModerationData      = errors.New("failed to get moderation analytics")
	ErrInvalidQueryForCounting      = errors.New("invalid query for counting")
	ErrInvalidQueryForCount         = errors.New("invalid query for count retrieval")
	ErrFailedGetQueryCount          = errors.New("failed to get query count")
	ErrFailedGetTopQueries          = errors.New("failed to get top queries")
	ErrFailedGetExistingCounter     = errors.New("failed to get existing counter")
	ErrFailedSaveCounter            = errors.New("failed to save counter")
	ErrFailedGetStats               = errors.New("failed to get stats")
)

// Federation cost-specific error constants
var (
	// Operation errors
	ErrFederationCostRecordFailed  = errors.New("failed to record federation cost")
	ErrFederationCostQueryFailed   = errors.New("failed to get federation costs")
	ErrFederationCostActivityQueryFailed = errors.New("failed to get federation costs by activity type")
	ErrFederationBudgetCreateFailed = errors.New("failed to create/update federation budget")
	ErrFederationBudgetNotFound    = errors.New("federation budget not found")
	ErrFederationBudgetQueryFailed = errors.New("failed to get federation budget")
	ErrActiveBudgetsQueryFailed    = errors.New("failed to get active budgets")
)

// Federation instance-specific error constants
var (
	// Operation errors
	ErrFederationInstanceSearchFailed       = errors.New("failed to search federation instances")
	ErrFederationInstanceHealthStoreFailed  = errors.New("failed to store health history")
	ErrFederationInstanceHealthQueryFailed  = errors.New("failed to get health history")
	ErrFederationInstanceBatchGetFailed     = errors.New("failed in batch get chunk")
	ErrFederationInstanceBatchCreateFailed  = errors.New("failed to batch create instances")
	ErrFederationInstanceBatchCreateChunkFailed = errors.New("failed in batch create chunk")
	ErrFederationInstanceBatchUpdateHealthFailed = errors.New("failed to batch update instances health")
	ErrFederationInstanceBatchUpdateHealthChunkFailed = errors.New("failed in batch update health chunk")
	ErrFederationInstanceUsageUpdateFailed  = errors.New("failed to get current instances for usage update")
	ErrFederationInstanceBatchUpdateUsageFailed = errors.New("failed to batch update instances usage")
	ErrFederationInstanceBatchUpdateUsageChunkFailed = errors.New("failed in batch update usage chunk")
	ErrFederationInstanceListFailed         = errors.New("failed to list all instances")
	
	// Validation errors
	ErrFederationInstanceCursorTooLong      = errors.New("cursor too long: maximum 1024 characters")
	ErrFederationInstanceCursorInvalid      = errors.New("invalid cursor format")
	ErrFederationInstanceLimitNegative      = errors.New("limit cannot be negative")
	ErrFederationInstanceLimitTooLarge      = errors.New("limit too large: maximum 1000 items per page")
)

// CSRF-specific error constants
var (
	// Validation errors
	ErrCSRFTokenInvalid         = errors.New("invalid CSRF token")
	ErrCSRFTokenExpired         = errors.New("expired CSRF token")
	ErrCSRFTokenAlreadyExists   = errors.New("token already exists")
	ErrCSRFTooManyTokens        = errors.New("too many active CSRF tokens for user")
)

// Media metadata-specific error constants
var (
	// Operation errors
	ErrMediaMetadataPrepareFailed   = errors.New("failed to prepare media metadata")
	ErrMediaMetadataNotFound       = errors.New("media metadata not found")
	ErrMediaMetadataQueryFailed    = errors.New("failed to get media metadata")
	ErrMediaMetadataStatusQueryFailed = errors.New("failed to get media metadata by status")
	ErrExpiredMediaMetadataQueryFailed = errors.New("failed to find expired media metadata")
)

// DLQ-specific error constants
var (
	// Validation errors
	ErrDLQServiceRequired       = errors.New("service is required for DLQ search")
	ErrDLQMessageNotFound       = errors.New("DLQ message not found")
	ErrDLQMessageNotReprocessable = errors.New("message cannot be reprocessed")
	ErrDLQBatchUpdateFailed     = errors.New("batch update failed")
)

// Notification-specific error constants
var (
	// Validation errors
	ErrNotificationUnknownPreferenceType = errors.New("unknown notification preference type")
)

// DNS cache-specific error constants
var (
	// Validation errors
	ErrDNSCacheEntryRequired = errors.New("DNS cache entry cannot be nil")
	
	// Operation errors
	ErrDNSCacheGetFailed         = errors.New("failed to get DNS cache entry")
	ErrDNSCacheSetFailed         = errors.New("failed to set DNS cache entry")
	ErrDNSCacheInvalidateFailed  = errors.New("failed to invalidate DNS cache entry")
)

// Federation activity-specific error constants
var (
	// Validation errors
	ErrFederationActivityValidationFailed = errors.New("federation activity validation failed")
	
	// Not found errors
	ErrFederationActivityNotFound = errors.New("federation activity not found")
)

// Quote-specific error constants
var (
	// Operation errors
	ErrQuoteRelationshipCreateFailed = errors.New("failed to create quote relationship")
	ErrQuoteRelationshipGetFailed    = errors.New("failed to get quote relationship")
	ErrQuoteRelationshipUpdateFailed = errors.New("failed to update quote relationship")
	ErrQuoteRelationshipDeleteFailed = errors.New("failed to delete quote relationship")
	ErrQuoteRelationshipQueryFailed  = errors.New("failed to query quote relationships")
	ErrQuotePermissionsCreateFailed  = errors.New("failed to create quote permissions")
	ErrQuotePermissionsGetFailed     = errors.New("failed to get quote permissions")
	ErrQuotePermissionsUpdateFailed  = errors.New("failed to update quote permissions")
	ErrQuotePermissionsDeleteFailed  = errors.New("failed to delete quote permissions")
	ErrQuoteCountQueryFailed         = errors.New("failed to get quote count")
)

// Marker-specific error constants
var (
	// Operation errors
	ErrMarkerSaveFailed = errors.New("failed to save marker")
)

// Scheduled job cost-specific error constants
var (
	// Validation errors
	ErrScheduledJobCostBeforeCreateFailed = errors.New("before create validation failed")
	ErrScheduledJobCostBeforeUpdateFailed = errors.New("before update validation failed")
	ErrScheduledJobCostNotFound           = errors.New("scheduled job cost record not found")
	ErrScheduledJobCostAggregationFailed  = errors.New("failed to list job costs for aggregation")
)

// Moderation metrics-specific error constants
var (
	// Query operation errors
	ErrModerationMetricsFalsePositivesQueryFailed = errors.New("failed to get false positives")
	ErrModerationMetricsDecisionSamplesQueryFailed = errors.New("failed to get decision samples")
	ErrModerationMetricsTopPatternsQueryFailed     = errors.New("failed to get top patterns")
	ErrModerationMetricsEntriesQueryFailed         = errors.New("failed to get metrics entries")
)

// Pagination-specific error constants
var (
	// Validation errors
	ErrPaginationParametersInvalid = errors.New("invalid pagination parameters")
	ErrPaginationCursorInvalid     = errors.New("invalid cursor")
	ErrPaginationCursorFormat      = errors.New("invalid cursor format")
	ErrPaginationCursorData        = errors.New("invalid cursor data")
)

// Relationship pagination-specific error constants
var (
	// Validation errors
	ErrRelationshipPaginationModelTypeUnsupported = errors.New("unsupported model type")
	
	// Operation errors
	ErrRelationshipPaginationQueryFailed = errors.New("failed to get relationship data")
)

// Relay-specific error constants
var (
	// Not found errors
	ErrRelayNotFound = errors.New("relay not found")
)

// Timeline-specific error constants
var (
	// Query operation errors
	ErrTimelineQueryFailed                    = errors.New("failed to get timeline entries")
	ErrTimelineEntriesByPostQueryFailed      = errors.New("failed to get timeline entries by post")
	ErrTimelineEntriesByActorQueryFailed     = errors.New("failed to get timeline entries by actor")
	ErrTimelineEntriesByVisibilityQueryFailed = errors.New("failed to get timeline entries by visibility")
	ErrTimelineEntriesByLanguageQueryFailed  = errors.New("failed to get timeline entries by language")
	ErrTimelineEntryQueryFailed              = errors.New("failed to get timeline entry")
	ErrTimelineEntriesForDeletionQueryFailed = errors.New("failed to get timeline entries for deletion")
	ErrTimelineExpiredEntriesScanFailed      = errors.New("failed to scan for expired timeline entries")
	ErrTimelineCountQueryFailed              = errors.New("failed to count timeline entries")
	ErrTimelineEntriesInRangeQueryFailed     = errors.New("failed to get timeline entries in range")
	ErrTimelineFilteredEntriesQueryFailed    = errors.New("failed to get filtered timeline entries")
	
	// Test mock error
	ErrTestMockError = errors.New("mock error")
)

// StreamingConnection-specific error constants
var (
	// Connection limit validation errors
	ErrStreamingConnectionUserLimitReached  = errors.New("user has reached maximum connections")
	ErrStreamingConnectionGlobalLimitReached = errors.New("maximum total connections reached")
	
	// Resource validation errors
	ErrStreamingConnectionMessageSizeExceeded = errors.New("message size exceeds limit")
	ErrStreamingConnectionRateLimitExceeded   = errors.New("rate limit exceeded")
	
	// Connection not found error
	ErrStreamingConnectionNotFound = errors.New("connection not found")
)

// StreamingPreferences-specific error constants
var (
	// Validation errors
	ErrStreamingUsernameRequired     = errors.New("username is required")
	ErrStreamingDeviceParamsRequired = errors.New("username and deviceID are required")
	
	// Operation errors
	ErrStreamingConflictResolutionFailed = errors.New("failed to resolve preference conflict")
)

// ErrorHandler is the global error utils instance
var ErrorHandler = NewErrorUtils()

// MapDynamoDBError maps DynamoDB/DynamORM errors to storage errors
func MapDynamoDBError(err error) error {
	if err == nil {
		return nil
	}

	// Check for DynamORM error types first
	if dynamormErrors.IsNotFound(err) {
		return storage.ErrNotFound
	}

	if dynamormErrors.IsConditionFailed(err) {
		return storage.ErrAlreadyExists
	}

	// Fall back to string matching for other errors
	errStr := err.Error()

	// Validation errors
	if strings.Contains(errStr, "validation failed") || strings.Contains(errStr, "invalid") {
		return storage.ErrInvalidInput
	}

	// Authorization errors
	if strings.Contains(errStr, "unauthorized") || strings.Contains(errStr, "forbidden") {
		return storage.ErrUnauthorized
	}

	// Default to original error wrapped with context
	return fmt.Errorf("%w: %w", ErrDatabaseOperation, err)
}

// MapErrorWithContext wraps an error with additional context
func MapErrorWithContext(err error, context string) error {
	if err == nil {
		return nil
	}

	mappedErr := MapDynamoDBError(err)
	return fmt.Errorf("%s: %w", context, mappedErr)
}
