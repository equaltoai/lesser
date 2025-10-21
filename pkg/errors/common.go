package errors

// Common errors and helper functions for frequently used error patterns
// This consolidates the most common error creation patterns across the application

// CRUD Operation Errors - Most common pattern across repositories
// These eliminate the need for repeated "failed to create", "failed to get", etc.

// FailedToCreate creates an error indicating creation of an item failed.
func FailedToCreate(itemType string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Failed to create "+itemType, err)
}

// FailedToGet creates an error indicating retrieval of an item failed.
func FailedToGet(itemType string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Failed to retrieve "+itemType, err)
}

// FailedToUpdate creates an error indicating update of an item failed.
func FailedToUpdate(itemType string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Failed to update "+itemType, err)
}

// FailedToDelete creates an error indicating deletion of an item failed.
func FailedToDelete(itemType string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Failed to delete "+itemType, err)
}

// FailedToList creates an error indicating listing of items failed.
func FailedToList(itemType string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Failed to list "+itemType, err).AsRetryable()
}

// FailedToQuery creates an error indicating querying of items failed.
func FailedToQuery(itemType string, err error) *AppError {
	return NewStorageInternalError(CodeQueryFailed, "Failed to query "+itemType, err).AsRetryable()
}

// FailedToStore creates an error indicating storing of an item failed.
func FailedToStore(itemType string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Failed to store "+itemType, err).AsRetryable()
}

// FailedToRetrieve creates an error indicating retrieval of an item failed.
func FailedToRetrieve(itemType string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Failed to retrieve "+itemType, err).AsRetryable()
}

// FailedToSave creates an error indicating saving of an item failed.
func FailedToSave(itemType string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Failed to save "+itemType, err).AsRetryable()
}

// FailedToRemove creates an error indicating removal of an item failed.
func FailedToRemove(itemType string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Failed to remove "+itemType, err).AsRetryable()
}

// OperationNotAllowedOnSelf creates an error indicating operation cannot be performed on self.
func OperationNotAllowedOnSelf(operation string) *AppError {
	return NewAppError(CodeOperationNotAllowed, CategoryBusiness, "Cannot perform operation on self").
		WithMetadata("operation", operation)
}

// InsufficientPermissions creates an error indicating insufficient permissions for an operation.
func InsufficientPermissions(operation string) *AppError {
	return NewAppError(CodeForbidden, CategoryAuth, "Insufficient permissions").
		WithMetadata("operation", operation)
}

// ResourceUnavailable creates an error indicating a resource is temporarily unavailable.
func ResourceUnavailable(resourceType string) *AppError {
	return NewAppError(CodeExternalServiceUnavailable, CategoryInternal, "Resource temporarily unavailable").
		WithMetadata("resource_type", resourceType).AsRetryable()
}

// ServiceUnavailable creates an error indicating a service is temporarily unavailable.
func ServiceUnavailable(serviceName string) *AppError {
	return NewAppError(CodeExternalServiceUnavailable, CategoryExternal, "Service temporarily unavailable").
		WithMetadata("service_name", serviceName).AsRetryable()
}

// ProcessingFailed creates an error indicating processing failed.
func ProcessingFailed(processType string, err error) *AppError {
	if err == nil {
		return NewLambdaError(CodeEventProcessingFailed, "Processing failed").
			WithMetadata("process_type", processType).
			WithInternalMessage("no inner error provided").
			AsRetryable()
	}

	return NewLambdaInternalError(CodeEventProcessingFailed, "Processing failed", err).
		WithMetadata("process_type", processType).AsRetryable()
}

// ParsingFailed creates an error indicating parsing failed.
func ParsingFailed(parseType string, err error) *AppError {
	return NewAppError(CodeActivityParsingFailed, CategoryFederation, "Parsing failed").
		WithInternalError(err).
		WithMetadata("parse_type", parseType)
}

// MarshalingFailed creates an error indicating data marshaling failed.
func MarshalingFailed(dataType string, err error) *AppError {
	return NewAppError(CodeInternal, CategoryInternal, "Data marshaling failed").
		WithInternalError(err).
		WithMetadata("data_type", dataType)
}

// UnmarshalingFailed creates an error indicating data unmarshaling failed.
func UnmarshalingFailed(dataType string, err error) *AppError {
	return NewAppError(CodeInternal, CategoryInternal, "Data unmarshaling failed").
		WithInternalError(err).
		WithMetadata("data_type", dataType)
}

// NetworkError creates an error indicating a network operation failed.
func NetworkError(operation string, err error) *AppError {
	return NewAppError(CodeExternalServiceTimeout, CategoryExternal, "Network operation failed").
		WithInternalError(err).
		WithMetadata("operation", operation).AsRetryable()
}

// TimeoutError creates an error indicating an operation timed out.
func TimeoutError(operation string) *AppError {
	return NewAppError(CodeTimeout, CategoryInternal, "Operation timed out").
		WithMetadata("operation", operation).AsRetryable()
}

// ExternalAPIError creates an error indicating an external API error occurred.
func ExternalAPIError(apiName string, statusCode int, err error) *AppError {
	return NewAppError(CodeExternalAPIError, CategoryExternal, "External API error").
		WithInternalError(err).
		WithMetadata("api_name", apiName).
		WithMetadata("status_code", statusCode)
}

// ConfigurationMissing creates an error indicating required configuration is missing.
func ConfigurationMissing(configKey string) *AppError {
	return NewAppError(CodeInternal, CategoryInternal, "Required configuration missing").
		WithMetadata("config_key", configKey)
}

// ConfigurationInvalid creates an error indicating configuration is invalid.
func ConfigurationInvalid(configKey, reason string) *AppError {
	return NewAppError(CodeInternal, CategoryInternal, "Configuration is invalid").
		WithMetadata("config_key", configKey).
		WithMetadata("reason", reason)
}

// EnvironmentVariableRequired creates an error indicating a required environment variable is missing.
func EnvironmentVariableRequired(varName string) *AppError {
	return NewAppError(CodeInternal, CategoryLambda, "Required environment variable missing").
		WithMetadata("variable_name", varName)
}

// DependencyInitializationFailed creates an error indicating dependency initialization failed.
func DependencyInitializationFailed(dependency string, err error) *AppError {
	return NewAppError(CodeInternal, CategoryInternal, "Dependency initialization failed").
		WithInternalError(err).
		WithMetadata("dependency", dependency)
}

// ServiceInitializationFailedGeneric creates an error indicating service initialization failed.
func ServiceInitializationFailedGeneric(service string, err error) *AppError {
	return NewAppError(CodeInternal, CategoryInternal, "Service initialization failed").
		WithInternalError(err).
		WithMetadata("service", service)
}

// ConnectionFailed creates an error indicating connection failed.
func ConnectionFailed(connectionType string, err error) *AppError {
	return NewAppError(CodeDatabaseConnection, CategoryStorage, "Connection failed").
		WithInternalError(err).
		WithMetadata("connection_type", connectionType).AsRetryable()
}

// QuotaExceeded creates an error indicating quota was exceeded.
func QuotaExceeded(quotaType string, limit int64) *AppError {
	return NewAppError(CodeQuotaExceeded, CategoryBusiness, "Quota exceeded").
		WithMetadata("quota_type", quotaType).
		WithMetadata("limit", limit)
}

// RateLimitExceededGeneric creates an error indicating rate limit was exceeded.
func RateLimitExceededGeneric(limitType string) *AppError {
	return NewAppError(CodeRateLimited, CategoryAuth, "Rate limit exceeded").
		WithMetadata("limit_type", limitType).AsRetryable()
}

// TooManyRequests creates an error indicating too many requests for a resource.
func TooManyRequests(resource string) *AppError {
	return NewAppError(CodeRateLimited, CategoryAPI, "Too many requests").
		WithMetadata("resource", resource).AsRetryable()
}

// InvalidStateForOperation creates an error indicating invalid state for an operation.
func InvalidStateForOperation(currentState, operation string) *AppError {
	return NewAppError(CodeInvalidStateTransition, CategoryBusiness, "Invalid state for operation").
		WithMetadata("current_state", currentState).
		WithMetadata("operation", operation)
}

// ConcurrentModification creates an error indicating concurrent modification was detected.
func ConcurrentModification(resourceType string) *AppError {
	return NewAppError(CodeConcurrencyError, CategoryStorage, "Concurrent modification detected").
		WithMetadata("resource_type", resourceType).AsRetryable()
}

// ResourceLocked creates an error indicating a resource is locked.
func ResourceLocked(resourceType, resourceID string) *AppError {
	return NewAppError(CodeConflict, CategoryBusiness, "Resource is locked").
		WithMetadata("resource_type", resourceType).
		WithMetadata("resource_id", resourceID).AsRetryable()
}

// DataCorrupted creates an error indicating data corruption was detected.
func DataCorrupted(dataType string) *AppError {
	return NewAppError(CodeInternal, CategoryInternal, "Data corruption detected").
		WithMetadata("data_type", dataType)
}

// DataInconsistent creates an error indicating data inconsistency was detected.
func DataInconsistent(context string) *AppError {
	return NewAppError(CodeInternal, CategoryInternal, "Data inconsistency detected").
		WithMetadata("context", context)
}

// ContentNotAllowed creates an error indicating content is not allowed.
func ContentNotAllowed(contentType, reason string) *AppError {
	return NewAppError(CodeForbidden, CategoryBusiness, "Content not allowed").
		WithMetadata("content_type", contentType).
		WithMetadata("reason", reason)
}

// SecurityViolation creates an error indicating a security violation was detected.
func SecurityViolation(violationType string) *AppError {
	return NewAppError(CodeForbidden, CategoryAuth, "Security violation detected").
		WithMetadata("violation_type", violationType)
}

// AccessDeniedForResource creates an error indicating access was denied for a resource.
func AccessDeniedForResource(resourceType, resourceID string) *AppError {
	return NewAppError(CodeForbidden, CategoryAuth, "Access denied").
		WithMetadata("resource_type", resourceType).
		WithMetadata("resource_id", resourceID)
}

// TamperingDetected creates an error indicating tampering was detected.
func TamperingDetected(context string) *AppError {
	return NewAppError(CodeForbidden, CategoryAuth, "Tampering detected").
		WithMetadata("context", context)
}

// IsRetryableError checks if an error is retryable.
func IsRetryableError(err error) bool {
	if appErr, ok := AsAppError(err); ok {
		return appErr.Retryable
	}
	return false
}

// IsTemporaryError checks if an error is temporary and should be retried.
func IsTemporaryError(err error) bool {
	if appErr, ok := AsAppError(err); ok {
		switch appErr.Code {
		case CodeTimeout, CodeRateLimited, CodeExternalServiceTimeout,
			CodeExternalServiceUnavailable, CodeDatabaseConnection:
			return true
		}
		return appErr.Retryable
	}
	return false
}

// IsClientError checks if an error is a client error (4xx HTTP status).
func IsClientError(err error) bool {
	if appErr, ok := AsAppError(err); ok {
		return appErr.HTTPStatusCode >= 400 && appErr.HTTPStatusCode < 500
	}
	return false
}

// IsServerError checks if an error is a server error (5xx HTTP status).
func IsServerError(err error) bool {
	if appErr, ok := AsAppError(err); ok {
		return appErr.HTTPStatusCode >= 500
	}
	return false
}

// WrapWithContext wraps an error with additional context information.
func WrapWithContext(err error, context string) *AppError {
	if err == nil {
		return nil
	}

	if appErr, ok := AsAppError(err); ok {
		return appErr.WithInternalMessage(context + ": " + appErr.InternalMessage)
	}

	return InternalWithCause(err, context)
}

// WrapWithOperation wraps an error with operation metadata.
func WrapWithOperation(err error, operation string) *AppError {
	if appErr, ok := AsAppError(err); ok {
		return appErr.WithMetadata("operation", operation)
	}
	return WrapWithContext(err, operation)
}

// WrapWithResource wraps an error with resource metadata.
func WrapWithResource(err error, resourceType, resourceID string) *AppError {
	if appErr, ok := AsAppError(err); ok {
		return appErr.WithMetadata("resource_type", resourceType).
			WithMetadata("resource_id", resourceID)
	}
	return WrapWithContext(err, resourceType+":"+resourceID)
}

// BusinessRuleViolated creates an error indicating a business rule was violated.
func BusinessRuleViolated(rule string, context map[string]interface{}) *AppError {
	appErr := NewAppError(CodeBusinessRuleViolated, CategoryBusiness, "Business rule violation").
		WithMetadata("rule", rule)

	for k, v := range context {
		appErr = appErr.WithMetadata(k, v)
	}

	return appErr
}

// PreConditionFailed creates an error indicating a pre-condition was not met.
func PreConditionFailed(condition string) *AppError {
	return NewAppError(CodeDependencyNotMet, CategoryBusiness, "Pre-condition not met").
		WithMetadata("condition", condition)
}

// PostConditionFailed creates an error indicating a post-condition was not met.
func PostConditionFailed(condition string) *AppError {
	return NewAppError(CodeInternal, CategoryBusiness, "Post-condition not met").
		WithMetadata("condition", condition)
}

// MultipleErrors creates an error aggregating multiple failures.
func MultipleErrors(errors []error, operation string) *AppError {
	errorMessages := make([]string, len(errors))
	for i, err := range errors {
		errorMessages[i] = err.Error()
	}

	return NewAppError(CodeInternal, CategoryInternal, "Multiple errors occurred").
		WithMetadata("operation", operation).
		WithMetadata("errors", errorMessages).
		WithMetadata("error_count", len(errors))
}

// Business Logic Specific Error Functions

// TimelineRequiresField creates an error indicating a timeline requires a specific field.
func TimelineRequiresField(timelineType, field string) *AppError {
	return NewAppError(CodeRequiredFieldMissing, CategoryValidation, "Timeline requires field").
		WithMetadata("timeline_type", timelineType).
		WithMetadata("required_field", field)
}

// UnsupportedTimelineType creates an error for unsupported timeline types.
func UnsupportedTimelineType(timelineType string) *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Unsupported timeline type").
		WithMetadata("timeline_type", timelineType)
}

// RepositoryNotAvailable creates an error when a repository is not available.
func RepositoryNotAvailable(repositoryType string) *AppError {
	return NewAppError(CodeInternal, CategoryStorage, "Repository not available").
		WithMetadata("repository_type", repositoryType)
}

// ServiceNotAvailable creates an error when a service is not available.
func ServiceNotAvailable(serviceName string) *AppError {
	return NewAppError(CodeInternal, CategoryInternal, "Service not available").
		WithMetadata("service_name", serviceName)
}

// ScheduledTimeValidationFailed creates an error for invalid scheduled times.
func ScheduledTimeValidationFailed(reason string) *AppError {
	return NewAppError(CodeValidationFailed, CategoryValidation, "Scheduled time validation failed").
		WithMetadata("reason", reason)
}

// UsernameTaken creates an error when a username is already taken.
func UsernameTaken(username string) *AppError {
	return NewAppError(CodeAlreadyExists, CategoryBusiness, "Username already taken").
		WithMetadata("username", username)
}

// EmailRequired creates an error when email is required but missing.
func EmailRequired() *AppError {
	return NewAppError(CodeRequiredFieldMissing, CategoryValidation, "Email is required")
}

// MustAgreeToTerms creates an error when user must agree to terms of service.
func MustAgreeToTerms() *AppError {
	return NewAppError(CodeValidationFailed, CategoryBusiness, "Must agree to terms of service")
}

// KeypairGenerationFailed creates an error for keypair generation failures.
func KeypairGenerationFailed(err error) *AppError {
	return NewAppError(CodeInternal, CategoryAuth, "Failed to generate keypair").WithInternalError(err)
}

// PublicKeyEncodingFailed creates an error for public key encoding failures.
func PublicKeyEncodingFailed(err error) *AppError {
	return NewAppError(CodeInternal, CategoryAuth, "Failed to encode public key").WithInternalError(err)
}

// InvalidPrivateKeyType creates an error for invalid private key types.
func InvalidPrivateKeyType() *AppError {
	return NewAppError(CodeInvalidInput, CategoryAuth, "Expected *rsa.PrivateKey")
}

// KeyTypeUnsupported creates an error for unsupported key types.
func KeyTypeUnsupported(keyType string) *AppError {
	return NewAppError(CodeInvalidInput, CategoryAuth, "Unsupported key type").
		WithMetadata("key_type", keyType)
}

// DomainHealthScoreRetrievalFailed creates an error for domain health score retrieval failures.
func DomainHealthScoreRetrievalFailed(domain string, err error) *AppError {
	return NewAppError(CodeInternal, CategoryStorage, "Failed to get domain health score").
		WithInternalError(err).WithMetadata("domain", domain)
}

// StorageTypeUnsupported creates an error for unsupported storage types.
func StorageTypeUnsupported(storageType string) *AppError {
	return NewAppError(CodeInvalidInput, CategoryStorage, "Unsupported storage type").
		WithMetadata("storage_type", storageType)
}

// RegistryOptionApplyFailed creates an error for registry option application failures.
func RegistryOptionApplyFailed(err error) *AppError {
	return NewAppError(CodeInternal, CategoryInternal, "Failed to apply registry option").WithInternalError(err)
}

// RegistryValidationFailed creates an error for registry validation failures.
func RegistryValidationFailed(reason string) *AppError {
	return NewAppError(CodeValidationFailed, CategoryInternal, "Registry validation failed").
		WithMetadata("reason", reason)
}

// EventBusNotInitialized creates an error when internal event bus is not initialized.
func EventBusNotInitialized() *AppError {
	return NewAppError(CodeInternal, CategoryInternal, "Internal event bus not initialized")
}

// EventBusSubscriptionFailed creates an error for event bus subscription failures.
func EventBusSubscriptionFailed(err error) *AppError {
	return NewAppError(CodeInternal, CategoryInternal, "Failed to subscribe to internal event bus").WithInternalError(err)
}

// DatabaseTypeUnsupported creates an error for unsupported database types.
func DatabaseTypeUnsupported(dbType string) *AppError {
	return NewAppError(CodeInvalidInput, CategoryStorage, "Unsupported database type").
		WithMetadata("database_type", dbType)
}

// NoDatabaseAvailable creates an error when no database is available.
func NoDatabaseAvailable() *AppError {
	return NewAppError(CodeInternal, CategoryStorage, "No database available")
}

// ActorIDFormatInvalid creates an error for invalid actor ID formats.
func ActorIDFormatInvalid(actorID string) *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Invalid actor ID format").
		WithMetadata("actor_id", actorID)
}

// QueueURLNotConfigured creates an error when queue URL is not configured.
func QueueURLNotConfigured(queueName string) *AppError {
	return NewAppError(CodeInternal, CategoryLambda, "Queue URL not configured").
		WithMetadata("queue_name", queueName)
}

// SQSConnectionFailed creates an error for SQS connection failures.
func SQSConnectionFailed(err error) *AppError {
	return NewAppError(CodeDatabaseConnection, CategoryLambda, "Failed to connect to SQS queue").
		WithInternalError(err).AsRetryable()
}

// MessageMarshalingFailed creates an error for message marshaling failures.
func MessageMarshalingFailed(messageType string, err error) *AppError {
	return NewAppError(CodeInternal, CategoryLambda, "Failed to marshal message").
		WithInternalError(err).WithMetadata("message_type", messageType)
}

// SQSMessageSendFailed creates an error for SQS message send failures.
func SQSMessageSendFailed(err error) *AppError {
	return NewAppError(CodeInternal, CategoryLambda, "Failed to send message to SQS").
		WithInternalError(err).AsRetryable()
}

// FileSizeExceedsLimit creates an error when file size exceeds the configured limit.
func FileSizeExceedsLimit(size, limit int64) *AppError {
	return NewAppError(CodeContentTooLarge, CategoryValidation, "File size exceeds limit").
		WithMetadata("size", size).WithMetadata("limit", limit)
}

// ContentTypeNotAllowed creates an error when content type is not allowed.
func ContentTypeNotAllowed(contentType string) *AppError {
	return NewAppError(CodeUnsupportedMediaType, CategoryValidation, "Content type is not allowed").
		WithMetadata("content_type", contentType)
}

// FormatNotSupported creates an error for unsupported file formats.
func FormatNotSupported(format string) *AppError {
	return NewAppError(CodeUnsupportedMediaType, CategoryValidation, "Format is not supported").
		WithMetadata("format", format)
}

// JSONFormatInvalid creates an error for invalid JSON formats.
func JSONFormatInvalid(reason string) *AppError {
	return NewAppError(CodeInvalidFormat, CategoryValidation, "Invalid JSON format").
		WithMetadata("reason", reason)
}

// CSVValidationFailed creates an error for CSV validation failures.
func CSVValidationFailed(reason string) *AppError {
	return NewAppError(CodeValidationFailed, CategoryValidation, "CSV validation failed").
		WithMetadata("reason", reason)
}

// FileValidationFailed creates an error for file validation failures.
func FileValidationFailed(reason string) *AppError {
	return NewAppError(CodeValidationFailed, CategoryValidation, "File validation failed").
		WithMetadata("reason", reason)
}

// ContentValidationFailed creates an error for content validation failures.
func ContentValidationFailed(field, reason string) *AppError {
	return NewAppError(CodeValidationFailed, CategoryValidation, "Content validation failed").
		WithMetadata("field", field).WithMetadata("reason", reason)
}

// ExpandMediaSettingInvalid creates an error for invalid expand media settings.
func ExpandMediaSettingInvalid(setting string) *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Invalid expand media setting").
		WithMetadata("setting", setting)
}

// TimelineOrderInvalid creates an error for invalid timeline orders.
func TimelineOrderInvalid(order string) *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Invalid timeline order").
		WithMetadata("order", order)
}

// AccountNoActivityPubActor creates an error when account has no ActivityPub actor.
func AccountNoActivityPubActor(username string) *AppError {
	return NewAppError(CodeInternal, CategoryFederation, "Account has no ActivityPub actor").
		WithMetadata("username", username)
}

// AccountAlreadyPinned creates an error when account is already pinned.
func AccountAlreadyPinned(targetUsername string) *AppError {
	return NewAppError(CodeAlreadyExists, CategoryBusiness, "Account already pinned").
		WithMetadata("target_username", targetUsername)
}

// MediaAttachmentValidationFailed creates an error for media attachment validation failures.
func MediaAttachmentValidationFailed(reason string) *AppError {
	return NewAppError(CodeValidationFailed, CategoryMedia, "Media attachment validation failed").
		WithMetadata("reason", reason)
}

// MediaAttachmentNotReady creates an error when media attachment is not ready.
func MediaAttachmentNotReady(mediaID string) *AppError {
	return NewAppError(CodeInvalidStateTransition, CategoryMedia, "Media attachment not ready").
		WithMetadata("media_id", mediaID)
}

// MediaAttachmentExpired creates an error when media attachment has expired.
func MediaAttachmentExpired(mediaID string) *AppError {
	return NewAppError(CodeNotFound, CategoryMedia, "Media attachment has expired").
		WithMetadata("media_id", mediaID)
}

// DateRangeInvalid creates an error for invalid date ranges.
func DateRangeInvalid(reason string) *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Invalid date range").
		WithMetadata("reason", reason)
}

// MetricUnsupported creates an error for unsupported metric types.
func MetricUnsupported(metric string) *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Unsupported metric").
		WithMetadata("metric", metric)
}

// InsufficientHistoricalData creates an error when insufficient historical data is available.
func InsufficientHistoricalData(required int) *AppError {
	return NewAppError(CodeValidationFailed, CategoryBusiness, "Insufficient historical data for prediction").
		WithMetadata("required_points", required)
}

// AlreadyExists creates an error when an item already exists.
func AlreadyExists(itemType string) *AppError {
	return NewAppError(CodeAlreadyExists, CategoryBusiness, "Item already exists").
		WithMetadata("item_type", itemType)
}

// ValidationFailedWithField creates an error when general validation fails.
func ValidationFailedWithField(field string) *AppError {
	return NewAppError(CodeValidationFailed, CategoryValidation, "Validation failed").
		WithMetadata("field", field)
}

// Streaming-specific error functions

// StreamingConnectionClosed creates an error when streaming connection is closed unexpectedly.
func StreamingConnectionClosed(connectionID string, reason string) *AppError {
	return NewAppError(CodeConnectionClosed, CategoryStreaming, "Streaming connection closed").
		WithMetadata("connection_id", connectionID).
		WithMetadata("close_reason", reason).AsRetryable()
}

// StreamingConnectionTimeout creates an error when streaming connection times out.
func StreamingConnectionTimeout(connectionID string) *AppError {
	return NewAppError(CodeStreamingTimeout, CategoryStreaming, "Streaming connection timeout").
		WithMetadata("connection_id", connectionID).AsRetryable()
}

// StreamingRecoveryFailed creates an error when streaming connection recovery fails.
func StreamingRecoveryFailed(connectionID string, retryCount int, err error) *AppError {
	return NewAppError(CodeConnectionClosed, CategoryStreaming, "Streaming connection recovery failed").
		WithInternalError(err).
		WithMetadata("connection_id", connectionID).
		WithMetadata("retry_count", retryCount).AsRetryable()
}

// StreamingCircuitBreakerOpen creates an error when circuit breaker prevents recovery.
func StreamingCircuitBreakerOpen(connectionID string) *AppError {
	return NewAppError(CodeExternalServiceUnavailable, CategoryStreaming, "Circuit breaker is open, preventing streaming recovery").
		WithMetadata("connection_id", connectionID).AsRetryable()
}

// StreamingSyncFailed creates an error when connection synchronization fails.
func StreamingSyncFailed(connectionID string, err error) *AppError {
	return NewAppError(CodeStreamingTimeout, CategoryStreaming, "Failed to synchronize streaming connection").
		WithInternalError(err).
		WithMetadata("connection_id", connectionID).AsRetryable()
}

// StreamingHealthCheckFailed creates an error when health check fails.
func StreamingHealthCheckFailed(connectionID string, err error) *AppError {
	return NewAppError(CodeConnectionClosed, CategoryStreaming, "Streaming connection health check failed").
		WithInternalError(err).
		WithMetadata("connection_id", connectionID).AsRetryable()
}

// Transformation-specific error functions

// TransformFunctionNotSet creates an error when transform function is not configured.
func TransformFunctionNotSet() *AppError {
	return NewAppError(CodeInternal, CategoryInternal, "Transform function not set")
}

// TransformItemFailed creates an error when item transformation fails.
func TransformItemFailed(err error) *AppError {
	return NewAppError(CodeInternal, CategoryInternal, "Failed to transform item").
		WithInternalError(err)
}

// Cost-specific error functions

// CircuitBreakerOpen creates an error when circuit breaker is open due to cost limit.
func CircuitBreakerOpen() *AppError {
	return NewAppError(CodeExternalServiceUnavailable, CategoryInternal, "Circuit breaker open: cost limit exceeded")
}

// CircuitBreakerReopened creates an error when circuit breaker is reopened due to high cost.
func CircuitBreakerReopened() *AppError {
	return NewAppError(CodeExternalServiceUnavailable, CategoryInternal, "Circuit breaker reopened: cost still too high")
}

// HourlyCostLimitExceeded creates an error when hourly cost limit would be exceeded.
func HourlyCostLimitExceeded() *AppError {
	return NewAppError(CodeQuotaExceeded, CategoryBusiness, "Hourly cost limit would be exceeded")
}

// Observability-specific error functions

// LoggerRequired creates an error when logger instance is required.
func LoggerRequired() *AppError {
	return NewAppError(CodeRequiredFieldMissing, CategoryInternal, "Logger is required")
}

// DatabaseRequired creates an error when database connection is required.
func DatabaseRequired() *AppError {
	return NewAppError(CodeDatabaseConnection, CategoryStorage, "Database connection is required")
}

// SNSPublishFailed creates an error when publishing to SNS fails.
func SNSPublishFailed(err error) *AppError {
	return NewAppError(CodeExternalServiceUnavailable, CategoryExternal, "Failed to publish to SNS").
		WithInternalError(err).AsRetryable()
}

// Storage and Media Configuration error functions

// InvalidPlanTier creates an error for invalid plan tier.
func InvalidPlanTier() *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Invalid plan tier")
}

// InvalidFileSize creates an error for invalid file size configuration.
func InvalidFileSize() *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Invalid file size configuration")
}

// VideoDurationInvalid creates an error for invalid video duration.
func VideoDurationInvalid() *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Invalid video duration")
}

// UploadLimitsInvalid creates an error for invalid upload limits.
func UploadLimitsInvalid() *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Invalid upload limits")
}

// BudgetLimitsInvalid creates an error for invalid budget limits.
func BudgetLimitsInvalid() *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Invalid budget limits")
}

// ModerationThresholdInvalid creates an error for invalid moderation threshold.
func ModerationThresholdInvalid() *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Invalid moderation threshold")
}

// InvalidQualitySetting creates an error for invalid quality setting.
func InvalidQualitySetting() *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Invalid quality setting")
}

// PlanUpgradeFailed creates an error for plan upgrade failure.
func PlanUpgradeFailed(err error) *AppError {
	return NewAppError(CodeInternal, CategoryBusiness, "Plan upgrade failed").
		WithInternalError(err)
}

// UserIDRequired creates an error when user ID is required.
func UserIDRequired() *AppError {
	return NewAppError(CodeRequiredFieldMissing, CategoryValidation, "User ID is required")
}

// Pattern Repository error functions

// PatternSaveFailed creates an error for pattern save failure.
func PatternSaveFailed(err error) *AppError {
	return FailedToSave("enhanced moderation pattern", err)
}

// PatternCreateFailed creates an error for pattern creation failure.
func PatternCreateFailed(err error) *AppError {
	return FailedToCreate("enhanced moderation pattern", err)
}

// PatternUpdateFailed creates an error for pattern update failure.
func PatternUpdateFailed(err error) *AppError {
	return FailedToUpdate("enhanced moderation pattern", err)
}

// PatternDeleteFailed creates an error for pattern deletion failure.
func PatternDeleteFailed(err error) *AppError {
	return FailedToDelete("enhanced moderation pattern", err)
}

// PatternQueryFailed creates an error for pattern query failure.
func PatternQueryFailed(err error) *AppError {
	return FailedToQuery("enhanced moderation patterns", err)
}

// PatternCacheCreateFailed creates an error for pattern cache creation failure.
func PatternCacheCreateFailed(err error) *AppError {
	return FailedToCreate("pattern cache entry", err)
}

// PatternCacheUpdateFailed creates an error for pattern cache update failure.
func PatternCacheUpdateFailed(err error) *AppError {
	return FailedToUpdate("pattern cache entry", err)
}

// PatternMetricsCreateFailed creates an error for pattern metrics creation failure.
func PatternMetricsCreateFailed(err error) *AppError {
	return FailedToCreate("pattern performance metrics", err)
}

// PatternMetricsUpdateFailed creates an error for pattern metrics update failure.
func PatternMetricsUpdateFailed(err error) *AppError {
	return FailedToUpdate("pattern performance metrics", err)
}

// PatternTestResultCreateFailed creates an error for pattern test result creation failure.
func PatternTestResultCreateFailed(err error) *AppError {
	return FailedToCreate("pattern test result", err)
}

// PatternTestResultQueryFailed creates an error for pattern test result query failure.
func PatternTestResultQueryFailed(err error) *AppError {
	return FailedToQuery("pattern test results", err)
}

// PatternTestResultNotFound creates an error for pattern test result not found.
func PatternTestResultNotFound() *AppError {
	return NewAppError(CodeNotFound, CategoryBusiness, "Pattern test result not found")
}

// PatternMetricsQueryFailed creates an error for pattern metrics query failure.
func PatternMetricsQueryFailed(err error) *AppError {
	return FailedToQuery("pattern performance metrics", err)
}

// PatternAnalysisFailed creates an error for pattern analysis failure.
func PatternAnalysisFailed(err error) *AppError {
	return ProcessingFailed("pattern analysis", err)
}

// PatternValidationFailed creates an error for pattern validation failure.
func PatternValidationFailed(reason string) *AppError {
	return ValidationFailedWithField("pattern validation: " + reason)
}

// NilPattern creates an error for nil pattern.
func NilPattern() *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Pattern cannot be nil")
}

// NilPatternCache creates an error for nil pattern cache.
func NilPatternCache() *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Pattern cache cannot be nil")
}

// NilPatternMetric creates an error for nil pattern metric.
func NilPatternMetric() *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Pattern metric cannot be nil")
}

// NilPatternTestResult creates an error for nil pattern test result.
func NilPatternTestResult() *AppError {
	return NewAppError(CodeInvalidInput, CategoryValidation, "Pattern test result cannot be nil")
}
