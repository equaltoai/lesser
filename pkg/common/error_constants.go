package common

// Common Error Messages
const (
	// Validation errors
	ValidationMustNotBeNull = "must not be null"
	ValidationPeriodRequired = "period is required"
	
	// General error categories
	ErrorGeneral = "general"
	ErrorExpected = "expected"
	ErrorTimeout = "timeout"
	ErrorOther = "other"
	
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
)