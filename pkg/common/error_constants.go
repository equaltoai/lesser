// Package common provides shared error constants for the Lesser application.
package common

// Common Error Messages
const (
	// Validation errors
	ValidationMustNotBeNull  = "must not be null"
	ValidationPeriodRequired = "period is required"

	// General error categories
	ErrorGeneral  = "general"
	ErrorExpected = "expected"
	ErrorTimeout  = "timeout"
	ErrorOther    = "other"

	// Access errors
	ErrorAdminAccessRequired = "admin access required"

	// Vote related
	ErrorFailedToStoreVote = "Failed to store vote"

	// Delivery related
	ErrorFailedToDeliverToRecipients = "Failed to deliver to recipients"

	// Subscription related
	ErrorSubscriptionManagerNotRunning = "subscription manager is not running"

	// Import related
	ErrorImportNotFound = "import not found: %s"

	// Export related
	ErrorFailedToGetExportsForUser = "failed to get exports for user: %w"

	// Federation related
	ErrorFailedToRecordCircuitBreakerFailure = "failed to record circuit breaker failure"

	// Collection related
	ErrorFailedToCheckCollectionMembership = "failed to check collection membership"

	// Moderation related
	ErrorFailedToStoreAnalysisResult = "failed to store analysis result"

	// DLQ related
	ErrorFailedToGetQueueURL = "failed to get queue URL for %s: %w"

	// Reputation related
	ErrorInvalidPublicKeyEncoding = "invalid public key encoding: %w"

	// WebSocket related
	ErrorFailedToStoreSubscription = "failed to store subscription: %w"

	// Follow request related
	ErrorFollowRequestNotFound    = "follow request not found"
	ErrorFailedToGetFollowerActor = "failed to get follower actor"
	ErrorFailedToGetFollowedActor = "failed to get followed actor"

	// Search related
	ErrorSearchFailed                   = "search failed: %w"
	ErrorPrivacyAwareSearchFailed       = "privacy-aware search failed: %w"
	ErrorStatusSearchFailed             = "status search failed: %w"
	ErrorPrivacyAwareStatusSearchFailed = "privacy-aware status search failed: %w"

	// Deployment/initialization related
	ErrorFailedToGeneratePrivateKey   = "failed to generate private key: %w"
	ErrorFailedToMarshalPrivateKey    = "failed to marshal private key: %w"
	ErrorFailedToConvertToECDHKey     = "failed to convert to ECDH key: %w"
	ErrorFailedToCreateOrUpdateSecret = "failed to create or update secret: %w"

	// Middleware RBAC related
	ErrorUnauthorizedNoValidClaims  = "unauthorized: no valid claims found"
	ErrorForbiddenAdminRequired     = "forbidden: admin access required"
	ErrorForbiddenModeratorRequired = "forbidden: moderator access required"
	ErrorForbiddenViewerRequired    = "forbidden: viewer access required"
	ErrorForbiddenUnknownPermission = "forbidden: unknown permission level"

	// Translation related
	ErrorInvalidSourceLanguageCode = "invalid source language code: %w"
	ErrorInvalidTargetLanguageCode = "invalid target language code: %w"
)
