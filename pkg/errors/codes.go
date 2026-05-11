package errors // nolint:revive // Legacy package name; import with an alias when also using stdlib errors.

// ErrorCode represents standardized error codes across the application
type ErrorCode string

// Common error codes
const (
	// Generic errors
	CodeNotFound            ErrorCode = "NOT_FOUND"
	CodeAlreadyExists       ErrorCode = "ALREADY_EXISTS"
	CodeInvalidInput        ErrorCode = "INVALID_INPUT"
	CodeUnauthorized        ErrorCode = "UNAUTHORIZED"
	CodeForbidden           ErrorCode = "FORBIDDEN"
	CodeTimeout             ErrorCode = "TIMEOUT"
	CodeRateLimited         ErrorCode = "RATE_LIMITED"
	CodeInternal            ErrorCode = "INTERNAL_ERROR"
	CodeGone                ErrorCode = "GONE"
	CodeUnprocessableEntity ErrorCode = "UNPROCESSABLE_ENTITY"
)

// Authentication and authorization error codes
const (
	CodeAuthFailed        ErrorCode = "AUTH_FAILED"
	CodeTokenExpired      ErrorCode = "TOKEN_EXPIRED"
	CodeTokenInvalid      ErrorCode = "TOKEN_INVALID"
	CodeTokenRevoked      ErrorCode = "TOKEN_REVOKED"
	CodeTokenReuse        ErrorCode = "TOKEN_REUSE"
	CodeSessionExpired    ErrorCode = "SESSION_EXPIRED"
	CodeSessionInvalid    ErrorCode = "SESSION_INVALID"
	CodeInsufficientScope ErrorCode = "INSUFFICIENT_SCOPE"
	CodeAccountSuspended  ErrorCode = "ACCOUNT_SUSPENDED"
	CodeInvalidPassword   ErrorCode = "INVALID_PASSWORD"
)

// Storage and database error codes
const (
	CodeDatabaseConnection   ErrorCode = "DATABASE_CONNECTION_FAILED"
	CodeQueryFailed          ErrorCode = "QUERY_FAILED"
	CodeTransactionFailed    ErrorCode = "TRANSACTION_FAILED"
	CodeIndexError           ErrorCode = "INDEX_ERROR"
	CodeConcurrencyError     ErrorCode = "CONCURRENCY_ERROR"
	CodeConstraintViolated   ErrorCode = "CONSTRAINT_VIOLATED"
	CodeStorageQuotaExceeded ErrorCode = "STORAGE_QUOTA_EXCEEDED"
)

// Federation and ActivityPub error codes
const (
	CodeActivityParsingFailed   ErrorCode = "ACTIVITY_PARSING_FAILED"
	CodeSignatureVerifyFailed   ErrorCode = "SIGNATURE_VERIFICATION_FAILED"
	CodeRemoteFetchFailed       ErrorCode = "REMOTE_FETCH_FAILED"
	CodeDeliveryFailed          ErrorCode = "DELIVERY_FAILED"
	CodeInboxProcessingFailed   ErrorCode = "INBOX_PROCESSING_FAILED"
	CodeOutboxProcessingFailed  ErrorCode = "OUTBOX_PROCESSING_FAILED"
	CodeUnsupportedActivityType ErrorCode = "UNSUPPORTED_ACTIVITY_TYPE"
	CodeFederationBlocked       ErrorCode = "FEDERATION_BLOCKED"
	CodeActorNotFound           ErrorCode = "ACTOR_NOT_FOUND"
	CodeInvalidActorURI         ErrorCode = "INVALID_ACTOR_URI"
)

// Validation error codes
const (
	CodeValidationFailed         ErrorCode = "VALIDATION_FAILED"
	CodeRequiredFieldMissing     ErrorCode = "REQUIRED_FIELD_MISSING"
	CodeFieldTooLong             ErrorCode = "FIELD_TOO_LONG"
	CodeFieldTooShort            ErrorCode = "FIELD_TOO_SHORT"
	CodeInvalidFormat            ErrorCode = "INVALID_FORMAT"
	CodeInvalidCharacters        ErrorCode = "INVALID_CHARACTERS"
	CodeValueOutOfRange          ErrorCode = "VALUE_OUT_OF_RANGE"
	CodeDirectSelfPostNotAllowed ErrorCode = "DIRECT_SELF_POST_NOT_ALLOWED"
)

// API error codes
const (
	CodeBadRequest              ErrorCode = "BAD_REQUEST"
	CodeMethodNotAllowed        ErrorCode = "METHOD_NOT_ALLOWED"
	CodeContentTooLarge         ErrorCode = "CONTENT_TOO_LARGE"
	CodeAPIUnsupportedMediaType ErrorCode = "UNSUPPORTED_MEDIA_TYPE_API"
	CodeAPIVersionNotSupported  ErrorCode = "API_VERSION_NOT_SUPPORTED"
	CodeMissingHeader           ErrorCode = "MISSING_HEADER"
	CodeInvalidHeader           ErrorCode = "INVALID_HEADER"
)

// Lambda-specific error codes
const (
	CodeLambdaTimeout         ErrorCode = "LAMBDA_TIMEOUT"
	CodeLambdaColdStart       ErrorCode = "LAMBDA_COLD_START"
	CodeLambdaMemoryExceeded  ErrorCode = "LAMBDA_MEMORY_EXCEEDED"
	CodeSQSProcessingFailed   ErrorCode = "SQS_PROCESSING_FAILED"
	CodeEventProcessingFailed ErrorCode = "EVENT_PROCESSING_FAILED"
	CodeDLQRetryExhausted     ErrorCode = "DLQ_RETRY_EXHAUSTED"
)

// Media processing error codes
const (
	CodeMediaTooLarge         ErrorCode = "MEDIA_TOO_LARGE"
	CodeUnsupportedMediaType  ErrorCode = "UNSUPPORTED_MEDIA_TYPE"
	CodeMediaProcessingFailed ErrorCode = "MEDIA_PROCESSING_FAILED"
	CodeTranscodingFailed     ErrorCode = "TRANSCODING_FAILED"
	CodeThumbnailFailed       ErrorCode = "THUMBNAIL_FAILED"
	CodeMediaUploadFailed     ErrorCode = "MEDIA_UPLOAD_FAILED"
)

// Moderation error codes
const (
	CodeContentBlocked     ErrorCode = "CONTENT_BLOCKED"
	CodeModerationFailed   ErrorCode = "MODERATION_FAILED"
	CodePatternMatchFailed ErrorCode = "PATTERN_MATCH_FAILED"
	CodeContentFlagged     ErrorCode = "CONTENT_FLAGGED"
	CodeSpamDetected       ErrorCode = "SPAM_DETECTED"
)

// Streaming error codes
const (
	CodeConnectionClosed   ErrorCode = "CONNECTION_CLOSED"
	CodeSubscriptionFailed ErrorCode = "SUBSCRIPTION_FAILED"
	CodeStreamingTimeout   ErrorCode = "STREAMING_TIMEOUT"
	CodeMessageTooLarge    ErrorCode = "MESSAGE_TOO_LARGE"
	CodeTooManyConnections ErrorCode = "TOO_MANY_CONNECTIONS"
)

// Business logic error codes
const (
	CodeOperationNotAllowed    ErrorCode = "OPERATION_NOT_ALLOWED"
	CodeInvalidStateTransition ErrorCode = "INVALID_STATE_TRANSITION"
	CodeQuotaExceeded          ErrorCode = "QUOTA_EXCEEDED"
	CodeConflict               ErrorCode = "CONFLICT"
	CodeDependencyNotMet       ErrorCode = "DEPENDENCY_NOT_MET"
	CodeBusinessRuleViolated   ErrorCode = "BUSINESS_RULE_VIOLATED"
)

// External service error codes
const (
	CodeExternalServiceUnavailable  ErrorCode = "EXTERNAL_SERVICE_UNAVAILABLE"
	CodeExternalServiceTimeout      ErrorCode = "EXTERNAL_SERVICE_TIMEOUT"
	CodeExternalAPIError            ErrorCode = "EXTERNAL_API_ERROR"
	CodeThirdPartyIntegrationFailed ErrorCode = "THIRD_PARTY_INTEGRATION_FAILED"
)

// String returns the string representation of the error code
func (c ErrorCode) String() string {
	return string(c)
}

// IsValid checks if the error code is valid
func (c ErrorCode) IsValid() bool {
	// This could be expanded with a comprehensive list validation
	return c != ""
}

// GetHTTPStatusCode returns the appropriate HTTP status code for the error code
func (c ErrorCode) GetHTTPStatusCode() int {
	switch c {
	case CodeNotFound, CodeActorNotFound:
		return 404
	case CodeUnauthorized, CodeAuthFailed, CodeTokenExpired, CodeTokenInvalid, CodeTokenRevoked:
		return 401
	case CodeForbidden, CodeInsufficientScope, CodeAccountSuspended:
		return 403
	case CodeAlreadyExists, CodeConflict:
		return 409
	case CodeGone:
		return 410
	case CodeUnprocessableEntity:
		return 422
	case CodeInvalidInput, CodeValidationFailed, CodeRequiredFieldMissing, CodeFieldTooLong,
		CodeFieldTooShort, CodeInvalidFormat, CodeInvalidCharacters, CodeValueOutOfRange,
		CodeDirectSelfPostNotAllowed, CodeBadRequest, CodeContentTooLarge:
		return 400
	case CodeMethodNotAllowed:
		return 405
	case CodeUnsupportedMediaType:
		return 415
	case CodeRateLimited, CodeQuotaExceeded:
		return 429
	case CodeTimeout, CodeLambdaTimeout, CodeStreamingTimeout, CodeExternalServiceTimeout:
		return 408
	case CodeExternalServiceUnavailable:
		return 503
	default:
		return 500
	}
}
