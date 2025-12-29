package repositories

import (
	stdErrors "errors"
	"strings"

	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
)

// ErrorUtils provides utilities for standardizing error handling across repositories
// This is maintained for backward compatibility while using the centralized error system.
type ErrorUtils struct{}

// NewErrorUtils creates a new ErrorUtils instance
func NewErrorUtils() *ErrorUtils {
	return &ErrorUtils{}
}

// HandleNotFound converts DynamORM not found errors to domain-specific errors
func (e *ErrorUtils) HandleNotFound(err error, entityType, identifier string) error {
	if err == nil {
		return nil
	}

	// Preserve expected (4xx) errors without re-wrapping them.
	if errors.IsClientError(err) {
		return err
	}

	if dynamormErrors.IsNotFound(err) {
		// Preserve the original DynamORM not-found sentinel in the unwrap chain so
		// downstream guards like dynamormErrors.IsNotFound continue to work even
		// after mapping to our domain AppError.
		return errors.ItemNotFoundWithID(entityType, identifier).WithInternalError(err)
	}
	return err
}

// HandleGetError standardizes error handling for Get operations
func (e *ErrorUtils) HandleGetError(err error, entityType, identifier string) error {
	if err == nil {
		return nil
	}

	// Preserve expected (4xx) errors without re-wrapping them.
	if errors.IsClientError(err) {
		return err
	}

	if dynamormErrors.IsNotFound(err) {
		// Preserve the original DynamORM not-found sentinel in the unwrap chain.
		return errors.ItemNotFoundWithID(entityType, identifier).WithInternalError(err)
	}

	return errors.FailedToGet(entityType, err)
}

// HandleCreateError standardizes error handling for Create operations
func (e *ErrorUtils) HandleCreateError(err error, entityType, identifier string) error {
	if err == nil {
		return nil
	}

	// Preserve expected (4xx) errors without re-wrapping them.
	if errors.IsClientError(err) {
		return err
	}

	if dynamormErrors.IsConditionFailed(err) {
		// Preserve the original DynamORM condition-failed sentinel in the unwrap chain.
		return errors.ItemAlreadyExistsWithID(entityType, identifier).WithInternalError(err)
	}

	return errors.FailedToCreate(entityType, err)
}

// HandleUpdateError standardizes error handling for Update operations
func (e *ErrorUtils) HandleUpdateError(err error, entityType, identifier string) error {
	if err == nil {
		return nil
	}

	// Preserve expected (4xx) errors without re-wrapping them.
	if errors.IsClientError(err) {
		return err
	}

	if dynamormErrors.IsNotFound(err) {
		// Preserve the original DynamORM not-found sentinel in the unwrap chain.
		return errors.ItemNotFoundWithID(entityType, identifier).WithInternalError(err)
	}

	return errors.FailedToUpdate(entityType, err)
}

// HandleDeleteError standardizes error handling for Delete operations
func (e *ErrorUtils) HandleDeleteError(err error, entityType, _ string) error {
	if err == nil {
		return nil
	}

	// For deletes, we typically don't treat "not found" as an error
	if errors.HasCode(err, errors.CodeNotFound) {
		return nil
	}
	if dynamormErrors.IsNotFound(err) {
		return nil
	}

	// Preserve expected (4xx) errors without re-wrapping them.
	if errors.IsClientError(err) {
		return err
	}

	return errors.FailedToDelete(entityType, err)
}

// HandleQueryError standardizes error handling for Query operations
func (e *ErrorUtils) HandleQueryError(err error, entityType, queryType string) error {
	if err == nil {
		return nil
	}

	// Preserve expected (4xx) errors without re-wrapping them.
	if errors.IsClientError(err) {
		return err
	}

	return errors.FailedToQuery(entityType+" ("+queryType+")", err)
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
	EntityUser                     = "user"
	EntityActor                    = "actor"
	EntityAlert                    = "alert"
	EntityObject                   = "object"
	EntityFollow                   = "follow"
	EntityBlock                    = "block"
	EntityMute                     = "mute"
	EntityList                     = "list"
	EntityHashtag                  = "hashtag"
	EntityFeaturedTag              = "featured tag"
	EntityMedia                    = "media"
	EntityOAuthState               = "OAuth state"
	EntityAuthCode                 = "authorization code"
	EntityRefreshToken             = "refresh token"
	EntityOAuthClient              = "OAuth client"
	EntityOAuthConsent             = "OAuth consent"
	EntityWebAuthnCredential       = "WebAuthn credential" // #nosec G101 -- entity name string, not a credential
	EntityWebAuthnChallenge        = "WebAuthn challenge"
	EntityWalletCredential         = "wallet credential" // #nosec G101 -- entity name string, not a credential
	EntityWalletChallenge          = "wallet challenge"
	EntitySession                  = "session"
	EntityPasswordReset            = "password reset"
	EntityTimelineEntry            = "timeline entry"
	EntityConversation             = "conversation"
	EntityBookmark                 = "bookmark"
	EntityFilter                   = "filter"
	EntityFilterKeyword            = "filter keyword"
	EntityFilterStatus             = "filter status"
	EntityReport                   = "report"
	EntityFlag                     = "flag"
	EntityModerationEvent          = "moderation event"
	EntityModerationDecision       = "moderation decision"
	EntityModerationPattern        = "moderation pattern"
	EntityAnnounce                 = "announce"
	EntityAccountPin               = "account pin"
	EntityAccountNote              = "account note"
	EntityStatusPin                = "status pin"
	EntityCircuitBreaker           = "circuit breaker"
	EntityCircuitState             = "circuit state"
	EntityCircuitEvent             = "circuit breaker event"
	EntityActivity                 = "activity"
	EntityFeature                  = "feature"
	EntityThreatIntel              = "threat intel"
	EntityThreatIndicator          = "threat indicator"
	EntityWebSocketCost            = "websocket cost"
	EntityWebSocketCostBudget      = "websocket cost budget"
	EntityWebSocketCostAggregation = "websocket cost aggregation"
	EntityAudit                    = "audit log"
	EntityAI                       = "ai analysis"
	EntityQueryCache               = "query cache"
	EntityCloudWatchMetrics        = "cloudwatch metrics"
	EntityRateLimit                = "rate limit"
	EntityMarker                   = "marker"
	EntityTrust                    = "trust"
	EntityTrustRelationship        = "trust relationship"
	EntityTrustScore               = "trust score"
	EntityTrustUpdate              = "trust update"
	EntityPublicKeyCache           = "public key cache"
	EntityRoutingMetrics           = "routing metrics"
	EntityNotification             = "notification"
	EntityCSRFToken                = "csrf token"
	EntityStatus                   = "status"
	EntityScheduledStatus          = "scheduled status"
	EntityInstanceDomainBlock      = "instance domain block"
	EntityEmailDomainBlock         = "email domain block"
	EntityDomainAllow              = "domain allow"
	EntityExport                   = "export"
	EntityExportCostTracking       = "export cost tracking"
	EntityImport                   = "import"
	EntityImportCostTracking       = "import cost tracking"
	EntityRecovery                 = "recovery"
	EntityRecoveryCode             = "recovery code"
	EntityRecoveryToken            = "recovery token"
	EntityTrustee                  = "trustee"
	EntityRecoveryRequest          = "recovery request"
	EntityInstanceHealth           = "instance health"
	EntityHealthSummary            = "health summary"
	EntityConnectivityTest         = "connectivity test"
	EntityNodeInfo                 = "nodeinfo verification"
	EntityWebFinger                = "webfinger resolution"
	EntitySeveredRelationship      = "severed relationship"
	EntityDeliveryRecord           = "delivery record"
	EntityFederationMetrics        = "federation metrics"
	EntitySearchCost               = "search cost"
	EntitySearchBudget             = "search budget"
	EntitySearchMetric             = "search metric"
	EntitySearchEmbedding          = "search embedding"
	EntitySearchSuggestion         = "search suggestion"
	EntitySearchAnalytics          = "search analytics"
	EntityEmoji                    = "emoji"
	EntityRelay                    = "relay"
	EntityFederationCost           = "federation cost"
	EntityFederationBudget         = "federation budget"
	EntityFederationInstance       = "federation instance"
	EntityMediaMetadata            = "media metadata"
	EntityDLQMessage               = "dlq message"
	EntityDNSCache                 = "dns cache"
	EntityFederationActivity       = "federation activity"
	EntityQuoteRelationship        = "quote relationship"
	EntityQuotePermissions         = "quote permissions"
)

// Account authentication-specific errors
// These functions replace the old error constants with centralized error system calls.

// AccountValidationFailed creates an error indicating account validation failed.
func AccountValidationFailed(reason string) *errors.AppError {
	return errors.NewValidationError("account", "Account validation failed").
		WithMetadata("reason", reason)
}

// DeviceValidationFailed creates an error indicating device validation failed.
func DeviceValidationFailed(reason string) *errors.AppError {
	return errors.NewValidationError("device", "Device validation failed").
		WithMetadata("reason", reason)
}

// SessionValidationFailed creates an error indicating session validation failed.
func SessionValidationFailed(reason string) *errors.AppError {
	return errors.NewValidationError("session", "Session validation failed").
		WithMetadata("reason", reason)
}

// WebAuthnValidationFailed creates an error indicating WebAuthn validation failed.
func WebAuthnValidationFailed(reason string) *errors.AppError {
	return errors.NewValidationError("webauthn", "WebAuthn validation failed").
		WithMetadata("reason", reason)
}

// WalletValidationFailed creates an error indicating wallet validation failed.
func WalletValidationFailed(reason string) *errors.AppError {
	return errors.NewValidationError("wallet", "Wallet validation failed").
		WithMetadata("reason", reason)
}

// DeviceNotFound creates an error indicating device was not found.
func DeviceNotFound(deviceID string) *errors.AppError {
	return errors.ItemNotFoundWithID("device", deviceID)
}

// WebAuthnCredentialNotFound creates an error indicating WebAuthn credential was not found.
func WebAuthnCredentialNotFound(credentialID string) *errors.AppError {
	return errors.ItemNotFoundWithID("WebAuthn credential", credentialID)
}

// Account search-specific errors

// AccountSearchInvalidWebfingerFormat creates an error indicating invalid webfinger format.
func AccountSearchInvalidWebfingerFormat(format string) *errors.AppError {
	return errors.InvalidFormat("webfinger", "acct:user@domain.tld").
		WithMetadata("provided_format", format)
}

// OAuth-specific errors

// OAuthClientNameRequired creates an error indicating client name is required.
func OAuthClientNameRequired() *errors.AppError {
	return errors.RequiredFieldMissing("client_name")
}

// OAuthRedirectURIsRequired creates an error indicating redirect URIs are required.
func OAuthRedirectURIsRequired() *errors.AppError {
	return errors.RequiredFieldMissing("redirect_uris")
}

// OAuthNoUpdatesProvided creates an error indicating no updates were provided.
func OAuthNoUpdatesProvided() *errors.AppError {
	return errors.BadRequest("No updates provided")
}

// OAuthClientAlreadyExists creates an error indicating OAuth client already exists.
func OAuthClientAlreadyExists(clientID string) *errors.AppError {
	return errors.ItemAlreadyExistsWithID("OAuth client", clientID)
}

// OAuthStateExpired creates an error indicating OAuth state has expired.
func OAuthStateExpired(state string) *errors.AppError {
	return errors.NewAppError(errors.CodeTokenExpired, errors.CategoryAuth, "OAuth state expired").
		WithMetadata("state", state)
}

// Repository operation base errors
// These are now provided by the centralized error system, but we maintain these for backward compatibility.

// ErrEntityNotFound is deprecated. Use errors.ItemNotFound() or errors.ItemNotFoundWithID() instead.
var ErrEntityNotFound = errors.ItemNotFound("entity")

// ErrEntityAlreadyExists is deprecated. Use errors.ItemAlreadyExists() or errors.ItemAlreadyExistsWithID() instead.
var ErrEntityAlreadyExists = errors.ItemAlreadyExists("entity")

// ErrEntityNotFoundForUpdate is deprecated. Use errors.ItemNotFoundWithID() instead.
var ErrEntityNotFoundForUpdate = errors.ItemNotFound("entity")

// ErrFailedToGet is deprecated. Use errors.FailedToGet() instead.
var ErrFailedToGet = errors.FailedToGet("entity", stdErrors.New("failed to get entity"))

// ErrFailedToCreate is deprecated. Use errors.FailedToCreate() instead.
var ErrFailedToCreate = errors.FailedToCreate("entity", stdErrors.New("failed to create entity"))

// ErrFailedToUpdate is deprecated. Use errors.FailedToUpdate() instead.
var ErrFailedToUpdate = errors.FailedToUpdate("entity", stdErrors.New("failed to update entity"))

// ErrFailedToDelete is deprecated. Use errors.FailedToDelete() instead.
var ErrFailedToDelete = errors.FailedToDelete("entity", stdErrors.New("failed to delete entity"))

// ErrFailedToQuery is deprecated. Use errors.FailedToQuery() instead.
var ErrFailedToQuery = errors.FailedToQuery("entity", stdErrors.New("failed to query entity"))

// ErrDatabaseOperation is deprecated. Use specific error functions from the centralized system.
var ErrDatabaseOperation = errors.NewStorageError(errors.CodeInternal, "Database error")

// Query utility-specific errors

// QueryOperationFailed creates an error indicating query operation failed.
func QueryOperationFailed(operation string, err error) *errors.AppError {
	return errors.FailedToQuery(operation, err)
}

// QueryCollectionAddFailed creates an error indicating failed to add to collection.
func QueryCollectionAddFailed(collection string, err error) *errors.AppError {
	return errors.FailedToStore("collection item", err).
		WithMetadata("collection", collection)
}

// QueryExecutionFailed creates an error indicating query execution failed.
func QueryExecutionFailed(query string, err error) *errors.AppError {
	return errors.FailedToQuery(query, err)
}

// QueryValidationFailed creates an error indicating query validation failed.
func QueryValidationFailed(reason string) *errors.AppError {
	return errors.NewValidationError("query", "Query validation failed").
		WithMetadata("reason", reason)
}

// Analytics-specific errors

// InvalidHashtagTrendType creates an error indicating invalid hashtag trend type.
func InvalidHashtagTrendType(actualType string) *errors.AppError {
	return errors.NewValidationError("trend_type", "Invalid trend type: expected *models.HashtagTrend or *storage.TrendingHashtag").
		WithMetadata("actual_type", actualType)
}

// InvalidStatusTrendType creates an error indicating invalid status trend type.
func InvalidStatusTrendType(actualType string) *errors.AppError {
	return errors.NewValidationError("trend_type", "Invalid trend type: expected *models.StatusTrend or *storage.TrendingStatus").
		WithMetadata("actual_type", actualType)
}

// InvalidLinkTrendType creates an error indicating invalid link trend type.
func InvalidLinkTrendType(actualType string) *errors.AppError {
	return errors.NewValidationError("trend_type", "Invalid trend type: expected *models.LinkTrend or *storage.TrendingLink").
		WithMetadata("actual_type", actualType)
}

// HashtagBatchUnknownModelType creates an error indicating unknown hashtag model type.
func HashtagBatchUnknownModelType(modelType string) *errors.AppError {
	return errors.NewValidationError("model_type", "Unknown model type").
		WithMetadata("model_type", modelType)
}

// StatusRepoDependencyMissing creates an error indicating status repository dependency is missing.
func StatusRepoDependencyMissing() *errors.AppError {
	return errors.ServiceNotAvailable("statusRepo")
}

// InvalidQueryParameters creates an error indicating invalid query parameters.
func InvalidQueryParameters(reason string) *errors.AppError {
	return errors.NewValidationError("query", "Invalid parameters: query cannot be empty and count must be positive").
		WithMetadata("reason", reason)
}

// FailedIndexByEngagement creates an error indicating failed to index by engagement.
func FailedIndexByEngagement(err error) *errors.AppError {
	return errors.FailedToStore("engagement index", err)
}

// FailedRecordEngagement creates an error indicating failed to record engagement.
func FailedRecordEngagement(err error) *errors.AppError {
	return errors.FailedToStore("engagement record", err)
}

// FailedGetEngagementMetrics creates an error indicating failed to get engagement metrics.
func FailedGetEngagementMetrics(err error) *errors.AppError {
	return errors.FailedToGet("engagement metrics", err)
}

// FailedGetEngagementByDate creates an error indicating failed to get engagement by date range.
func FailedGetEngagementByDate(err error) *errors.AppError {
	return errors.FailedToQuery("engagement by date", err)
}

// FailedGetTopContent creates an error indicating failed to get top engaged content.
func FailedGetTopContent(err error) *errors.AppError {
	return errors.FailedToQuery("top engaged content", err)
}

// FailedUpdateTrendingTag creates an error indicating failed to update trending hashtag.
func FailedUpdateTrendingTag(err error) *errors.AppError {
	return errors.FailedToUpdate("trending hashtag", err)
}

// FailedGetTrendingTags creates an error indicating failed to get trending hashtags.
func FailedGetTrendingTags(err error) *errors.AppError {
	return errors.FailedToQuery("trending hashtags", err)
}

// FailedQueryStaleTrends creates an error indicating failed to query stale trends.
func FailedQueryStaleTrends(err error) *errors.AppError {
	return errors.FailedToQuery("stale trends", err)
}

// FailedRecordInstanceMetric creates an error indicating failed to record instance metric.
func FailedRecordInstanceMetric(err error) *errors.AppError {
	return errors.FailedToStore("instance metric", err)
}

// FailedGetInstanceMetrics creates an error indicating failed to get instance metrics.
func FailedGetInstanceMetrics(err error) *errors.AppError {
	return errors.FailedToQuery("instance metrics", err)
}

// FailedGetStartMetric creates an error indicating failed to get start metric.
func FailedGetStartMetric(err error) *errors.AppError {
	return errors.FailedToGet("start metric", err)
}

// FailedGetEndMetric creates an error indicating failed to get end metric.
func FailedGetEndMetric(err error) *errors.AppError {
	return errors.FailedToGet("end metric", err)
}

// FailedRecordManifest creates an error indicating failed to record manifest generation.
func FailedRecordManifest(err error) *errors.AppError {
	return errors.FailedToStore("manifest generation", err)
}

// FailedRecordQualityChange creates an error indicating failed to record quality change.
func FailedRecordQualityChange(err error) *errors.AppError {
	return errors.FailedToStore("quality change", err)
}

// FailedRecordMediaEvent creates an error indicating failed to record media event.
func FailedRecordMediaEvent(err error) *errors.AppError {
	return errors.FailedToStore("media event", err)
}

// FailedQuerySessionEvents creates an error indicating failed to query session events.
func FailedQuerySessionEvents(err error) *errors.AppError {
	return errors.FailedToQuery("session events", err)
}

// FailedGetModerationAnalytics creates an error indicating failed to get existing moderation analytics.
func FailedGetModerationAnalytics(err error) *errors.AppError {
	return errors.FailedToGet("moderation analytics", err)
}

// FailedRecordModerationAction creates an error indicating failed to record moderation action.
func FailedRecordModerationAction(err error) *errors.AppError {
	return errors.FailedToStore("moderation action", err)
}

// FailedGetModerationData creates an error indicating failed to get moderation analytics.
func FailedGetModerationData(err error) *errors.AppError {
	return errors.FailedToQuery("moderation data", err)
}

// InvalidQueryForCounting creates an error indicating invalid query for counting.
func InvalidQueryForCounting(query string) *errors.AppError {
	return errors.NewValidationError("query", "Invalid query for counting").
		WithMetadata("query", query)
}

// InvalidQueryForCount creates an error indicating invalid query for count retrieval.
func InvalidQueryForCount(query string) *errors.AppError {
	return errors.NewValidationError("query", "Invalid query for count retrieval").
		WithMetadata("query", query)
}

// FailedGetQueryCount creates an error indicating failed to get query count.
func FailedGetQueryCount(err error) *errors.AppError {
	return errors.FailedToQuery("query count", err)
}

// FailedGetTopQueries creates an error indicating failed to get top queries.
func FailedGetTopQueries(err error) *errors.AppError {
	return errors.FailedToQuery("top queries", err)
}

// FailedGetExistingCounter creates an error indicating failed to get existing counter.
func FailedGetExistingCounter(err error) *errors.AppError {
	return errors.FailedToGet("counter", err)
}

// FailedSaveCounter creates an error indicating failed to save counter.
func FailedSaveCounter(err error) *errors.AppError {
	return errors.FailedToSave("counter", err)
}

// FailedGetStats creates an error indicating failed to get stats.
func FailedGetStats(err error) *errors.AppError {
	return errors.FailedToQuery("stats", err)
}

// Federation cost-specific errors

// FederationCostRecordFailed creates an error indicating failed to record federation cost.
func FederationCostRecordFailed(err error) *errors.AppError {
	return errors.FailedToStore("federation cost", err)
}

// FederationCostQueryFailed creates an error indicating failed to get federation costs.
func FederationCostQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("federation costs", err)
}

// FederationCostActivityQueryFailed creates an error indicating failed to get federation costs by activity type.
func FederationCostActivityQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("federation costs by activity type", err)
}

// FederationBudgetCreateFailed creates an error indicating failed to create/update federation budget.
func FederationBudgetCreateFailed(err error) *errors.AppError {
	return errors.FailedToStore("federation budget", err)
}

// FederationBudgetNotFound creates an error indicating federation budget not found.
func FederationBudgetNotFound(budgetID string) *errors.AppError {
	return errors.ItemNotFoundWithID("federation budget", budgetID)
}

// FederationBudgetQueryFailed creates an error indicating failed to get federation budget.
func FederationBudgetQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("federation budget", err)
}

// ActiveBudgetsQueryFailed creates an error indicating failed to get active budgets.
func ActiveBudgetsQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("active budgets", err)
}

// Federation instance-specific errors

// FederationInstanceSearchFailed creates an error indicating failed to search federation instances.
func FederationInstanceSearchFailed(err error) *errors.AppError {
	return errors.FailedToQuery("federation instances", err)
}

// FederationInstanceHealthStoreFailed creates an error indicating failed to store health history.
func FederationInstanceHealthStoreFailed(err error) *errors.AppError {
	return errors.FailedToStore("federation instance health history", err)
}

// FederationInstanceHealthQueryFailed creates an error indicating failed to get health history.
func FederationInstanceHealthQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("federation instance health history", err)
}

// FederationInstanceBatchGetFailed creates an error indicating failed in batch get chunk.
func FederationInstanceBatchGetFailed(err error) *errors.AppError {
	return errors.BatchOperationFailed("batch get federation instances", err)
}

// FederationInstanceBatchCreateFailed creates an error indicating failed to batch create instances.
func FederationInstanceBatchCreateFailed(err error) *errors.AppError {
	return errors.BatchOperationFailed("batch create federation instances", err)
}

// FederationInstanceBatchCreateChunkFailed creates an error indicating failed in batch create chunk.
func FederationInstanceBatchCreateChunkFailed(err error) *errors.AppError {
	return errors.BatchOperationFailed("batch create chunk", err)
}

// FederationInstanceBatchUpdateHealthFailed creates an error indicating failed to batch update instances health.
func FederationInstanceBatchUpdateHealthFailed(err error) *errors.AppError {
	return errors.BatchOperationFailed("batch update federation instance health", err)
}

// FederationInstanceBatchUpdateHealthChunkFailed creates an error indicating failed in batch update health chunk.
func FederationInstanceBatchUpdateHealthChunkFailed(err error) *errors.AppError {
	return errors.BatchOperationFailed("batch update health chunk", err)
}

// FederationInstanceUsageUpdateFailed creates an error indicating failed to get current instances for usage update.
func FederationInstanceUsageUpdateFailed(err error) *errors.AppError {
	return errors.FailedToQuery("current instances for usage update", err)
}

// FederationInstanceBatchUpdateUsageFailed creates an error indicating failed to batch update instances usage.
func FederationInstanceBatchUpdateUsageFailed(err error) *errors.AppError {
	return errors.BatchOperationFailed("batch update federation instance usage", err)
}

// FederationInstanceBatchUpdateUsageChunkFailed creates an error indicating failed in batch update usage chunk.
func FederationInstanceBatchUpdateUsageChunkFailed(err error) *errors.AppError {
	return errors.BatchOperationFailed("batch update usage chunk", err)
}

// FederationInstanceListFailed creates an error indicating failed to list all instances.
func FederationInstanceListFailed(err error) *errors.AppError {
	return errors.FailedToList("federation instances", err)
}

// FederationInstanceCursorTooLong creates an error indicating cursor is too long.
func FederationInstanceCursorTooLong(cursorLength int) *errors.AppError {
	return errors.FieldTooLong("cursor", 1024, cursorLength)
}

// FederationInstanceCursorInvalid creates an error indicating invalid cursor format.
func FederationInstanceCursorInvalid(cursor string) *errors.AppError {
	return errors.InvalidFormat("cursor", "base64 encoded").
		WithMetadata("provided_cursor", cursor)
}

// FederationInstanceLimitNegative creates an error indicating limit cannot be negative.
func FederationInstanceLimitNegative(limit int) *errors.AppError {
	return errors.ValueOutOfRange("limit", 0, 1000, limit)
}

// FederationInstanceLimitTooLarge creates an error indicating limit is too large.
func FederationInstanceLimitTooLarge(limit int) *errors.AppError {
	return errors.ValueOutOfRange("limit", 0, 1000, limit)
}

// CSRF-specific errors

// CSRFTokenInvalid creates an error indicating invalid CSRF token.
func CSRFTokenInvalid(token string) *errors.AppError {
	return errors.NewAppError(errors.CodeTokenInvalid, errors.CategoryAuth, "Invalid CSRF token").
		WithMetadata("token", token)
}

// CSRFTokenExpired creates an error indicating expired CSRF token.
func CSRFTokenExpired(token string) *errors.AppError {
	return errors.NewAppError(errors.CodeTokenExpired, errors.CategoryAuth, "Expired CSRF token").
		WithMetadata("token", token)
}

// CSRFTokenAlreadyExists creates an error indicating token already exists.
func CSRFTokenAlreadyExists(token string) *errors.AppError {
	return errors.ItemAlreadyExistsWithID("CSRF token", token)
}

// CSRFTooManyTokens creates an error indicating too many active CSRF tokens for user.
func CSRFTooManyTokens(userID string, count int) *errors.AppError {
	return errors.TooManyItems(count, 10). // Assuming max 10 tokens per user
						WithMetadata("user_id", userID)
}

// Media metadata-specific errors

// MediaMetadataPrepareFailed creates an error indicating failed to prepare media metadata.
func MediaMetadataPrepareFailed(err error) *errors.AppError {
	return errors.FailedToStore("media metadata", err)
}

// MediaMetadataNotFound creates an error indicating media metadata not found.
func MediaMetadataNotFound(mediaID string) *errors.AppError {
	return errors.ItemNotFoundWithID("media metadata", mediaID)
}

// MediaMetadataQueryFailed creates an error indicating failed to get media metadata.
func MediaMetadataQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("media metadata", err)
}

// MediaMetadataStatusQueryFailed creates an error indicating failed to get media metadata by status.
func MediaMetadataStatusQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("media metadata by status", err)
}

// ExpiredMediaMetadataQueryFailed creates an error indicating failed to find expired media metadata.
func ExpiredMediaMetadataQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("expired media metadata", err)
}

// DLQ-specific errors

// DLQServiceRequired creates an error indicating service is required for DLQ search.
func DLQServiceRequired() *errors.AppError {
	return errors.RequiredFieldMissing("service")
}

// DLQMessageNotFound creates an error indicating DLQ message not found.
func DLQMessageNotFound(messageID string) *errors.AppError {
	return errors.ItemNotFoundWithID("DLQ message", messageID)
}

// DLQMessageNotReprocessable creates an error indicating message cannot be reprocessed.
func DLQMessageNotReprocessable(messageID string, reason string) *errors.AppError {
	return errors.InvalidStateForOperation("message state", "reprocess").
		WithMetadata("message_id", messageID).
		WithMetadata("reason", reason)
}

// DLQBatchUpdateFailed creates an error indicating batch update failed.
func DLQBatchUpdateFailed(err error) *errors.AppError {
	return errors.BatchOperationFailed("DLQ batch update", err)
}

// Notification-specific errors

// NotificationUnknownPreferenceType creates an error indicating unknown notification preference type.
func NotificationUnknownPreferenceType(preferenceType string) *errors.AppError {
	return errors.NewValidationError("preference_type", "Unknown notification preference type").
		WithMetadata("preference_type", preferenceType)
}

// DNS cache-specific errors

// DNSCacheEntryRequired creates an error indicating DNS cache entry cannot be nil.
func DNSCacheEntryRequired() *errors.AppError {
	return errors.RequiredFieldMissing("dns_cache_entry")
}

// DNSCacheGetFailed creates an error indicating failed to get DNS cache entry.
func DNSCacheGetFailed(err error) *errors.AppError {
	return errors.FailedToGet("DNS cache entry", err)
}

// DNSCacheSetFailed creates an error indicating failed to set DNS cache entry.
func DNSCacheSetFailed(err error) *errors.AppError {
	return errors.FailedToStore("DNS cache entry", err)
}

// DNSCacheInvalidateFailed creates an error indicating failed to invalidate DNS cache entry.
func DNSCacheInvalidateFailed(err error) *errors.AppError {
	return errors.FailedToDelete("DNS cache entry", err)
}

// Federation activity-specific errors

// FederationActivityValidationFailed creates an error indicating federation activity validation failed.
func FederationActivityValidationFailed(reason string) *errors.AppError {
	return errors.NewValidationError("federation_activity", "Federation activity validation failed").
		WithMetadata("reason", reason)
}

// FederationActivityNotFound creates an error indicating federation activity not found.
func FederationActivityNotFound(activityID string) *errors.AppError {
	return errors.ItemNotFoundWithID("federation activity", activityID)
}

// Quote-specific errors

// QuoteRelationshipCreateFailed creates an error indicating failed to create quote relationship.
func QuoteRelationshipCreateFailed(err error) *errors.AppError {
	return errors.FailedToCreate("quote relationship", err)
}

// QuoteRelationshipGetFailed creates an error indicating failed to get quote relationship.
func QuoteRelationshipGetFailed(err error) *errors.AppError {
	return errors.FailedToGet("quote relationship", err)
}

// QuoteRelationshipUpdateFailed creates an error indicating failed to update quote relationship.
func QuoteRelationshipUpdateFailed(err error) *errors.AppError {
	return errors.FailedToUpdate("quote relationship", err)
}

// QuoteRelationshipDeleteFailed creates an error indicating failed to delete quote relationship.
func QuoteRelationshipDeleteFailed(err error) *errors.AppError {
	return errors.FailedToDelete("quote relationship", err)
}

// QuoteRelationshipQueryFailed creates an error indicating failed to query quote relationships.
func QuoteRelationshipQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("quote relationships", err)
}

// QuotePermissionsCreateFailed creates an error indicating failed to create quote permissions.
func QuotePermissionsCreateFailed(err error) *errors.AppError {
	return errors.FailedToCreate("quote permissions", err)
}

// QuotePermissionsGetFailed creates an error indicating failed to get quote permissions.
func QuotePermissionsGetFailed(err error) *errors.AppError {
	return errors.FailedToGet("quote permissions", err)
}

// QuotePermissionsUpdateFailed creates an error indicating failed to update quote permissions.
func QuotePermissionsUpdateFailed(err error) *errors.AppError {
	return errors.FailedToUpdate("quote permissions", err)
}

// QuotePermissionsDeleteFailed creates an error indicating failed to delete quote permissions.
func QuotePermissionsDeleteFailed(err error) *errors.AppError {
	return errors.FailedToDelete("quote permissions", err)
}

// QuoteCountQueryFailed creates an error indicating failed to get quote count.
func QuoteCountQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("quote count", err)
}

// Marker-specific errors

// MarkerSaveFailed creates an error indicating failed to save marker.
func MarkerSaveFailed(err error) *errors.AppError {
	return errors.FailedToSave("marker", err)
}

// Scheduled job cost-specific errors

// ScheduledJobCostBeforeCreateFailed creates an error indicating before create validation failed.
func ScheduledJobCostBeforeCreateFailed(reason string) *errors.AppError {
	return errors.NewValidationError("scheduled_job_cost", "Before create validation failed").
		WithMetadata("reason", reason)
}

// ScheduledJobCostBeforeUpdateFailed creates an error indicating before update validation failed.
func ScheduledJobCostBeforeUpdateFailed(reason string) *errors.AppError {
	return errors.NewValidationError("scheduled_job_cost", "Before update validation failed").
		WithMetadata("reason", reason)
}

// ScheduledJobCostNotFound creates an error indicating scheduled job cost record not found.
func ScheduledJobCostNotFound(jobID string) *errors.AppError {
	return errors.ItemNotFoundWithID("scheduled job cost record", jobID)
}

// ScheduledJobCostAggregationFailed creates an error indicating failed to list job costs for aggregation.
func ScheduledJobCostAggregationFailed(err error) *errors.AppError {
	return errors.FailedToQuery("job costs for aggregation", err)
}

// Moderation metrics-specific errors

// ModerationMetricsFalsePositivesQueryFailed creates an error indicating failed to get false positives.
func ModerationMetricsFalsePositivesQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("moderation false positives", err)
}

// ModerationMetricsDecisionSamplesQueryFailed creates an error indicating failed to get decision samples.
func ModerationMetricsDecisionSamplesQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("moderation decision samples", err)
}

// ModerationMetricsTopPatternsQueryFailed creates an error indicating failed to get top patterns.
func ModerationMetricsTopPatternsQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("moderation top patterns", err)
}

// ModerationMetricsEntriesQueryFailed creates an error indicating failed to get metrics entries.
func ModerationMetricsEntriesQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("moderation metrics entries", err)
}

// Pagination-specific errors

// PaginationParametersInvalid creates an error indicating invalid pagination parameters.
func PaginationParametersInvalid(reason string) *errors.AppError {
	return errors.NewValidationError("pagination", "Invalid pagination parameters").
		WithMetadata("reason", reason)
}

// PaginationCursorInvalid creates an error indicating invalid cursor.
func PaginationCursorInvalid(cursor string) *errors.AppError {
	return errors.NewValidationError("cursor", "Invalid cursor").
		WithMetadata("cursor", cursor)
}

// PaginationCursorFormat creates an error indicating invalid cursor format.
func PaginationCursorFormat(cursor string) *errors.AppError {
	return errors.InvalidFormat("cursor", "base64 encoded JSON").
		WithMetadata("cursor", cursor)
}

// PaginationCursorData creates an error indicating invalid cursor data.
func PaginationCursorData(cursor string, reason string) *errors.AppError {
	return errors.NewValidationError("cursor", "Invalid cursor data").
		WithMetadata("cursor", cursor).
		WithMetadata("reason", reason)
}

// Relationship pagination-specific errors

// RelationshipPaginationModelTypeUnsupported creates an error indicating unsupported model type.
func RelationshipPaginationModelTypeUnsupported(modelType string) *errors.AppError {
	return errors.NewValidationError("model_type", "Unsupported model type").
		WithMetadata("model_type", modelType)
}

// RelationshipPaginationQueryFailed creates an error indicating failed to get relationship data.
func RelationshipPaginationQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("relationship data", err)
}

// Relay-specific errors

// RelayNotFound creates an error indicating relay not found.
func RelayNotFound(relayID string) *errors.AppError {
	return errors.ItemNotFoundWithID("relay", relayID)
}

// Timeline-specific errors

// TimelineQueryFailed creates an error indicating failed to get timeline entries.
func TimelineQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("timeline entries", err)
}

// TimelineEntriesByPostQueryFailed creates an error indicating failed to get timeline entries by post.
func TimelineEntriesByPostQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("timeline entries by post", err)
}

// TimelineEntriesByActorQueryFailed creates an error indicating failed to get timeline entries by actor.
func TimelineEntriesByActorQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("timeline entries by actor", err)
}

// TimelineEntriesByVisibilityQueryFailed creates an error indicating failed to get timeline entries by visibility.
func TimelineEntriesByVisibilityQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("timeline entries by visibility", err)
}

// TimelineEntriesByLanguageQueryFailed creates an error indicating failed to get timeline entries by language.
func TimelineEntriesByLanguageQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("timeline entries by language", err)
}

// TimelineEntryQueryFailed creates an error indicating failed to get timeline entry.
func TimelineEntryQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("timeline entry", err)
}

// TimelineEntriesForDeletionQueryFailed creates an error indicating failed to get timeline entries for deletion.
func TimelineEntriesForDeletionQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("timeline entries for deletion", err)
}

// TimelineExpiredEntriesScanFailed creates an error indicating failed to scan for expired timeline entries.
func TimelineExpiredEntriesScanFailed(err error) *errors.AppError {
	return errors.FailedToQuery("expired timeline entries", err)
}

// TimelineCountQueryFailed creates an error indicating failed to count timeline entries.
func TimelineCountQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("timeline entry count", err)
}

// TimelineEntriesInRangeQueryFailed creates an error indicating failed to get timeline entries in range.
func TimelineEntriesInRangeQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("timeline entries in range", err)
}

// TimelineFilteredEntriesQueryFailed creates an error indicating failed to get filtered timeline entries.
func TimelineFilteredEntriesQueryFailed(err error) *errors.AppError {
	return errors.FailedToQuery("filtered timeline entries", err)
}

// TestMockError creates a test mock error for testing purposes.
func TestMockError() *errors.AppError {
	return errors.NewAppError(errors.CodeInternal, errors.CategoryInternal, "Mock error for testing")
}

// StreamingConnection-specific errors

// StreamingConnectionUserLimitReached creates an error indicating user has reached maximum connections.
func StreamingConnectionUserLimitReached(userID string, limit int) *errors.AppError {
	return errors.QuotaExceeded("user streaming connections", int64(limit)).
		WithMetadata("user_id", userID)
}

// StreamingConnectionGlobalLimitReached creates an error indicating maximum total connections reached.
func StreamingConnectionGlobalLimitReached(limit int) *errors.AppError {
	return errors.QuotaExceeded("global streaming connections", int64(limit))
}

// StreamingConnectionMessageSizeExceeded creates an error indicating message size exceeds limit.
func StreamingConnectionMessageSizeExceeded(size, limit int64) *errors.AppError {
	return errors.FileSizeExceedsLimit(size, limit)
}

// StreamingConnectionRateLimitExceeded creates an error indicating rate limit exceeded.
func StreamingConnectionRateLimitExceeded(connectionID string) *errors.AppError {
	return errors.RateLimitExceededGeneric("streaming connection").
		WithMetadata("connection_id", connectionID)
}

// StreamingConnectionNotFound creates an error indicating connection not found.
func StreamingConnectionNotFound(connectionID string) *errors.AppError {
	return errors.ItemNotFoundWithID("streaming connection", connectionID)
}

// StreamingPreferences-specific errors

// StreamingUsernameRequired creates an error indicating username is required.
func StreamingUsernameRequired() *errors.AppError {
	return errors.RequiredFieldMissing("username")
}

// StreamingDeviceParamsRequired creates an error indicating username and deviceID are required.
func StreamingDeviceParamsRequired() *errors.AppError {
	return errors.MultipleValidationErrors([]string{"username is required", "deviceID is required"})
}

// StreamingConflictResolutionFailed creates an error indicating failed to resolve preference conflict.
func StreamingConflictResolutionFailed(err error) *errors.AppError {
	return errors.ProcessingFailed("preference conflict resolution", err)
}

// ErrorHandler is the global error utils instance
var ErrorHandler = NewErrorUtils()

// MapDynamoDBError maps DynamoDB/DynamORM errors to storage errors using the centralized error system.
func MapDynamoDBError(err error) error {
	if err == nil {
		return nil
	}

	// Preserve canonical AppErrors (including wrapped AppErrors) without remapping
	// them to internal server errors.
	if errors.IsAppError(err) {
		return err
	}

	// Check for DynamORM error types first
	if dynamormErrors.IsNotFound(err) {
		return errors.NotFound("item").
			WithInternalError(stdErrors.Join(err, storage.ErrNotFound))
	}

	if dynamormErrors.IsConditionFailed(err) {
		return errors.AlreadyExists("item").
			WithInternalError(stdErrors.Join(err, storage.ErrAlreadyExists))
	}

	// Use centralized error system for other errors
	errStr := err.Error()

	// Validation errors
	if containsAny(errStr, "validation failed", "invalid") {
		return errors.ValidationFailedWithField("invalid input").
			WithInternalError(stdErrors.Join(err, storage.ErrInvalidInput))
	}

	// Authorization errors
	if containsAny(errStr, "unauthorized", "forbidden") {
		return errors.AccessDenied("").
			WithInternalError(stdErrors.Join(err, storage.ErrUnauthorized))
	}

	// DynamoDB throttling
	if containsAny(errStr, "ProvisionedThroughputExceededException", "throttling") {
		return errors.DynamoDBProvisionedThroughputExceeded().WithInternalError(err)
	}

	// DynamoDB item size limit
	if containsAny(errStr, "Item size", "ValidationException") {
		return errors.DynamoDBItemTooLarge().WithInternalError(err)
	}

	// Default to database operation error with internal details
	return errors.NewStorageInternalError(errors.CodeInternal, "Database operation failed", err)
}

// MapErrorWithContext wraps an error with additional context using the centralized error system.
func MapErrorWithContext(err error, context string) error {
	if err == nil {
		return nil
	}

	return errors.WrapWithContext(MapDynamoDBError(err), context)
}

// Helper function to check if string contains any of the provided substrings
func containsAny(s string, substrings ...string) bool {
	for _, substr := range substrings {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// Package-level convenience functions that wrap the centralized error system
// These provide a consistent API for repository-level error handling.

// NewRepositoryError creates a new repository error with Storage category.
func NewRepositoryError(code errors.ErrorCode, message string) *errors.AppError {
	return errors.NewStorageError(code, message)
}

// NewRepositoryInternalError creates a repository error with internal details.
func NewRepositoryInternalError(code errors.ErrorCode, message string, internal error) *errors.AppError {
	return errors.NewStorageInternalError(code, message, internal)
}

// WrapRepositoryError wraps an existing error as a repository error.
func WrapRepositoryError(err error, code errors.ErrorCode, message string) *errors.AppError {
	return errors.WrapError(err, code, errors.CategoryStorage, message)
}

// IsRepositoryNotFoundError checks if an error indicates a repository item was not found.
func IsRepositoryNotFoundError(err error) bool {
	return errors.HasCode(err, errors.CodeNotFound)
}

// IsRepositoryConflictError checks if an error indicates a repository conflict (already exists).
func IsRepositoryConflictError(err error) bool {
	return errors.HasCode(err, errors.CodeAlreadyExists)
}

// Deprecated error variables - These are maintained for backward compatibility.
// New code should use the function-based error creation from the centralized system.
var (
	// Account authentication errors
	ErrAccountValidationFailed    = AccountValidationFailed("general validation failure")
	ErrDeviceValidationFailed     = DeviceValidationFailed("general validation failure")
	ErrSessionValidationFailed    = SessionValidationFailed("general validation failure")
	ErrWebAuthnValidationFailed   = WebAuthnValidationFailed("general validation failure")
	ErrWalletValidationFailed     = WalletValidationFailed("general validation failure")
	ErrDeviceNotFound             = DeviceNotFound("unknown")
	ErrWebAuthnCredentialNotFound = WebAuthnCredentialNotFound("unknown")

	// Account search errors
	ErrAccountSearchInvalidWebfingerFormat = AccountSearchInvalidWebfingerFormat("unknown format")

	// OAuth errors
	ErrOAuthClientNameRequired   = OAuthClientNameRequired()
	ErrOAuthRedirectURIsRequired = OAuthRedirectURIsRequired()
	ErrOAuthNoUpdatesProvided    = OAuthNoUpdatesProvided()
	ErrOAuthClientAlreadyExists  = OAuthClientAlreadyExists("unknown")
	ErrOAuthStateExpired         = OAuthStateExpired("unknown")

	// Query utility errors
	ErrQueryOperationFailed     = QueryOperationFailed("unknown", stdErrors.New("unknown error"))
	ErrQueryCollectionAddFailed = QueryCollectionAddFailed("unknown", stdErrors.New("unknown error"))
	ErrQueryExecutionFailed     = QueryExecutionFailed("unknown", stdErrors.New("unknown error"))
	ErrQueryValidationFailed    = QueryValidationFailed("unknown")

	// Analytics errors
	ErrInvalidHashtagTrendType      = InvalidHashtagTrendType("unknown")
	ErrInvalidStatusTrendType       = InvalidStatusTrendType("unknown")
	ErrInvalidLinkTrendType         = InvalidLinkTrendType("unknown")
	ErrHashtagBatchUnknownModelType = HashtagBatchUnknownModelType("unknown")
	ErrStatusRepoDependencyMissing  = StatusRepoDependencyMissing()
	ErrInvalidQueryParameters       = InvalidQueryParameters("unknown")
	ErrFailedIndexByEngagement      = FailedIndexByEngagement(stdErrors.New("unknown error"))
	ErrFailedRecordEngagement       = FailedRecordEngagement(stdErrors.New("unknown error"))
	ErrFailedGetEngagementMetrics   = FailedGetEngagementMetrics(stdErrors.New("unknown error"))
	ErrFailedGetEngagementByDate    = FailedGetEngagementByDate(stdErrors.New("unknown error"))
	ErrFailedGetTopContent          = FailedGetTopContent(stdErrors.New("unknown error"))
	ErrFailedUpdateTrendingTag      = FailedUpdateTrendingTag(stdErrors.New("unknown error"))
	ErrFailedGetTrendingTags        = FailedGetTrendingTags(stdErrors.New("unknown error"))
	ErrFailedQueryStaleTrends       = FailedQueryStaleTrends(stdErrors.New("unknown error"))
	ErrFailedRecordInstanceMetric   = FailedRecordInstanceMetric(stdErrors.New("unknown error"))
	ErrFailedGetInstanceMetrics     = FailedGetInstanceMetrics(stdErrors.New("unknown error"))
	ErrFailedGetStartMetric         = FailedGetStartMetric(stdErrors.New("unknown error"))
	ErrFailedGetEndMetric           = FailedGetEndMetric(stdErrors.New("unknown error"))
	ErrFailedRecordManifest         = FailedRecordManifest(stdErrors.New("unknown error"))
	ErrFailedRecordQualityChange    = FailedRecordQualityChange(stdErrors.New("unknown error"))
	ErrFailedRecordMediaEvent       = FailedRecordMediaEvent(stdErrors.New("unknown error"))
	ErrFailedQuerySessionEvents     = FailedQuerySessionEvents(stdErrors.New("unknown error"))
	ErrFailedGetModerationAnalytics = FailedGetModerationAnalytics(stdErrors.New("unknown error"))
	ErrFailedRecordModerationAction = FailedRecordModerationAction(stdErrors.New("unknown error"))
	ErrFailedGetModerationData      = FailedGetModerationData(stdErrors.New("unknown error"))
	ErrInvalidQueryForCounting      = InvalidQueryForCounting("unknown")
	ErrInvalidQueryForCount         = InvalidQueryForCount("unknown")
	ErrFailedGetQueryCount          = FailedGetQueryCount(stdErrors.New("unknown error"))
	ErrFailedGetTopQueries          = FailedGetTopQueries(stdErrors.New("unknown error"))
	ErrFailedGetExistingCounter     = FailedGetExistingCounter(stdErrors.New("unknown error"))
	ErrFailedSaveCounter            = FailedSaveCounter(stdErrors.New("unknown error"))
	ErrFailedGetStats               = FailedGetStats(stdErrors.New("unknown error"))

	// Federation cost errors
	ErrFederationCostRecordFailed        = FederationCostRecordFailed(stdErrors.New("unknown error"))
	ErrFederationCostQueryFailed         = FederationCostQueryFailed(stdErrors.New("unknown error"))
	ErrFederationCostActivityQueryFailed = FederationCostActivityQueryFailed(stdErrors.New("unknown error"))
	ErrFederationBudgetCreateFailed      = FederationBudgetCreateFailed(stdErrors.New("unknown error"))
	ErrFederationBudgetNotFound          = FederationBudgetNotFound("unknown")
	ErrFederationBudgetQueryFailed       = FederationBudgetQueryFailed(stdErrors.New("unknown error"))
	ErrActiveBudgetsQueryFailed          = ActiveBudgetsQueryFailed(stdErrors.New("unknown error"))

	// Federation instance errors
	ErrFederationInstanceSearchFailed                 = FederationInstanceSearchFailed(stdErrors.New("unknown error"))
	ErrFederationInstanceHealthStoreFailed            = FederationInstanceHealthStoreFailed(stdErrors.New("unknown error"))
	ErrFederationInstanceHealthQueryFailed            = FederationInstanceHealthQueryFailed(stdErrors.New("unknown error"))
	ErrFederationInstanceBatchGetFailed               = FederationInstanceBatchGetFailed(stdErrors.New("unknown error"))
	ErrFederationInstanceBatchCreateFailed            = FederationInstanceBatchCreateFailed(stdErrors.New("unknown error"))
	ErrFederationInstanceBatchCreateChunkFailed       = FederationInstanceBatchCreateChunkFailed(stdErrors.New("unknown error"))
	ErrFederationInstanceBatchUpdateHealthFailed      = FederationInstanceBatchUpdateHealthFailed(stdErrors.New("unknown error"))
	ErrFederationInstanceBatchUpdateHealthChunkFailed = FederationInstanceBatchUpdateHealthChunkFailed(stdErrors.New("unknown error"))
	ErrFederationInstanceUsageUpdateFailed            = FederationInstanceUsageUpdateFailed(stdErrors.New("unknown error"))
	ErrFederationInstanceBatchUpdateUsageFailed       = FederationInstanceBatchUpdateUsageFailed(stdErrors.New("unknown error"))
	ErrFederationInstanceBatchUpdateUsageChunkFailed  = FederationInstanceBatchUpdateUsageChunkFailed(stdErrors.New("unknown error"))
	ErrFederationInstanceListFailed                   = FederationInstanceListFailed(stdErrors.New("unknown error"))
	ErrFederationInstanceCursorTooLong                = FederationInstanceCursorTooLong(1025)
	ErrFederationInstanceCursorInvalid                = FederationInstanceCursorInvalid("unknown")
	ErrFederationInstanceLimitNegative                = FederationInstanceLimitNegative(-1)
	ErrFederationInstanceLimitTooLarge                = FederationInstanceLimitTooLarge(1001)

	// CSRF errors
	ErrCSRFTokenInvalid       = CSRFTokenInvalid("unknown")
	ErrCSRFTokenExpired       = CSRFTokenExpired("unknown")
	ErrCSRFTokenAlreadyExists = CSRFTokenAlreadyExists("unknown")
	ErrCSRFTooManyTokens      = CSRFTooManyTokens("unknown", 11)

	// Media metadata errors
	ErrMediaMetadataPrepareFailed      = MediaMetadataPrepareFailed(stdErrors.New("unknown error"))
	ErrMediaMetadataNotFound           = MediaMetadataNotFound("unknown")
	ErrMediaMetadataQueryFailed        = MediaMetadataQueryFailed(stdErrors.New("unknown error"))
	ErrMediaMetadataStatusQueryFailed  = MediaMetadataStatusQueryFailed(stdErrors.New("unknown error"))
	ErrExpiredMediaMetadataQueryFailed = ExpiredMediaMetadataQueryFailed(stdErrors.New("unknown error"))

	// DLQ errors
	ErrDLQServiceRequired         = DLQServiceRequired()
	ErrDLQMessageNotFound         = DLQMessageNotFound("unknown")
	ErrDLQMessageNotReprocessable = DLQMessageNotReprocessable("unknown", "unknown")
	ErrDLQBatchUpdateFailed       = DLQBatchUpdateFailed(stdErrors.New("unknown error"))

	// Notification errors
	ErrNotificationUnknownPreferenceType = NotificationUnknownPreferenceType("unknown")

	// DNS cache errors
	ErrDNSCacheEntryRequired    = DNSCacheEntryRequired()
	ErrDNSCacheGetFailed        = DNSCacheGetFailed(stdErrors.New("unknown error"))
	ErrDNSCacheSetFailed        = DNSCacheSetFailed(stdErrors.New("unknown error"))
	ErrDNSCacheInvalidateFailed = DNSCacheInvalidateFailed(stdErrors.New("unknown error"))

	// Federation activity errors
	ErrFederationActivityValidationFailed = FederationActivityValidationFailed("unknown")
	ErrFederationActivityNotFound         = FederationActivityNotFound("unknown")

	// Quote errors
	ErrQuoteRelationshipCreateFailed = QuoteRelationshipCreateFailed(stdErrors.New("unknown error"))
	ErrQuoteRelationshipGetFailed    = QuoteRelationshipGetFailed(stdErrors.New("unknown error"))
	ErrQuoteRelationshipUpdateFailed = QuoteRelationshipUpdateFailed(stdErrors.New("unknown error"))
	ErrQuoteRelationshipDeleteFailed = QuoteRelationshipDeleteFailed(stdErrors.New("unknown error"))
	ErrQuoteRelationshipQueryFailed  = QuoteRelationshipQueryFailed(stdErrors.New("unknown error"))
	ErrQuotePermissionsCreateFailed  = QuotePermissionsCreateFailed(stdErrors.New("unknown error"))
	ErrQuotePermissionsGetFailed     = QuotePermissionsGetFailed(stdErrors.New("unknown error"))
	ErrQuotePermissionsUpdateFailed  = QuotePermissionsUpdateFailed(stdErrors.New("unknown error"))
	ErrQuotePermissionsDeleteFailed  = QuotePermissionsDeleteFailed(stdErrors.New("unknown error"))
	ErrQuoteCountQueryFailed         = QuoteCountQueryFailed(stdErrors.New("unknown error"))

	// Marker errors
	ErrMarkerSaveFailed = MarkerSaveFailed(stdErrors.New("unknown error"))

	// Scheduled job cost errors
	ErrScheduledJobCostBeforeCreateFailed = ScheduledJobCostBeforeCreateFailed("unknown")
	ErrScheduledJobCostBeforeUpdateFailed = ScheduledJobCostBeforeUpdateFailed("unknown")
	ErrScheduledJobCostNotFound           = ScheduledJobCostNotFound("unknown")
	ErrScheduledJobCostAggregationFailed  = ScheduledJobCostAggregationFailed(stdErrors.New("unknown error"))

	// Moderation metrics errors
	ErrModerationMetricsFalsePositivesQueryFailed  = ModerationMetricsFalsePositivesQueryFailed(stdErrors.New("unknown error"))
	ErrModerationMetricsDecisionSamplesQueryFailed = ModerationMetricsDecisionSamplesQueryFailed(stdErrors.New("unknown error"))
	ErrModerationMetricsTopPatternsQueryFailed     = ModerationMetricsTopPatternsQueryFailed(stdErrors.New("unknown error"))
	ErrModerationMetricsEntriesQueryFailed         = ModerationMetricsEntriesQueryFailed(stdErrors.New("unknown error"))

	// Pagination errors
	ErrPaginationParametersInvalid = PaginationParametersInvalid("unknown")
	ErrPaginationCursorInvalid     = PaginationCursorInvalid("unknown")
	ErrPaginationCursorFormat      = PaginationCursorFormat("unknown")
	ErrPaginationCursorData        = PaginationCursorData("unknown", "unknown")

	// Relationship pagination errors
	ErrRelationshipPaginationModelTypeUnsupported = RelationshipPaginationModelTypeUnsupported("unknown")
	ErrRelationshipPaginationQueryFailed          = RelationshipPaginationQueryFailed(stdErrors.New("unknown error"))

	// Relay errors
	ErrRelayNotFound = RelayNotFound("unknown")

	// Timeline errors
	ErrTimelineQueryFailed                    = TimelineQueryFailed(stdErrors.New("unknown error"))
	ErrTimelineEntriesByPostQueryFailed       = TimelineEntriesByPostQueryFailed(stdErrors.New("unknown error"))
	ErrTimelineEntriesByActorQueryFailed      = TimelineEntriesByActorQueryFailed(stdErrors.New("unknown error"))
	ErrTimelineEntriesByVisibilityQueryFailed = TimelineEntriesByVisibilityQueryFailed(stdErrors.New("unknown error"))
	ErrTimelineEntriesByLanguageQueryFailed   = TimelineEntriesByLanguageQueryFailed(stdErrors.New("unknown error"))
	ErrTimelineEntryQueryFailed               = TimelineEntryQueryFailed(stdErrors.New("unknown error"))
	ErrTimelineEntriesForDeletionQueryFailed  = TimelineEntriesForDeletionQueryFailed(stdErrors.New("unknown error"))
	ErrTimelineExpiredEntriesScanFailed       = TimelineExpiredEntriesScanFailed(stdErrors.New("unknown error"))
	ErrTimelineCountQueryFailed               = TimelineCountQueryFailed(stdErrors.New("unknown error"))
	ErrTimelineEntriesInRangeQueryFailed      = TimelineEntriesInRangeQueryFailed(stdErrors.New("unknown error"))
	ErrTimelineFilteredEntriesQueryFailed     = TimelineFilteredEntriesQueryFailed(stdErrors.New("unknown error"))
	ErrTestMockError                          = TestMockError()

	// Streaming connection errors
	ErrStreamingConnectionUserLimitReached    = StreamingConnectionUserLimitReached("unknown", 10)
	ErrStreamingConnectionGlobalLimitReached  = StreamingConnectionGlobalLimitReached(100)
	ErrStreamingConnectionMessageSizeExceeded = StreamingConnectionMessageSizeExceeded(1000, 500)
	ErrStreamingConnectionRateLimitExceeded   = StreamingConnectionRateLimitExceeded("unknown")
	ErrStreamingConnectionNotFound            = StreamingConnectionNotFound("unknown")

	// Streaming preferences errors
	ErrStreamingUsernameRequired         = StreamingUsernameRequired()
	ErrStreamingDeviceParamsRequired     = StreamingDeviceParamsRequired()
	ErrStreamingConflictResolutionFailed = StreamingConflictResolutionFailed(stdErrors.New("unknown error"))
)
