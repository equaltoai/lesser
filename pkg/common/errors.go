package common

import (
	"errors"
	"fmt"

	"go.uber.org/zap"
)

// Domain-specific error types

// Token-related errors
var (
	ErrTokenNotFound = errors.New("token not found")
	ErrTokenExpired  = errors.New("token expired")
	ErrTokenRevoked  = errors.New("token revoked")
)

// Auth helper errors
var (
	ErrMissingAuthorizationHeader = errors.New("missing authorization header")
	ErrAuthHeaderEmpty            = errors.New("authorization header is empty")
	ErrAuthHeaderInvalidPrefix    = errors.New("authorization header must start with 'Bearer '")
	ErrAuthenticationRequired     = errors.New("authentication required for write operations")
	ErrAuthenticationRequiredRead = errors.New("authentication required for read operations")
	ErrInsufficientScope          = errors.New("insufficient scope")
	ErrInsufficientScopeWrite     = errors.New("insufficient scope: write access required")
	ErrInsufficientScopeRead      = errors.New("insufficient scope: read access required")
)

// Business logic validation errors
var (
	ErrDataCannotBeNil                         = errors.New("data cannot be nil")
	ErrRequiredFieldMissing                    = errors.New("required field is missing or empty")
	ErrFieldExceedsMaxLength                   = errors.New("field exceeds maximum length")
	ErrFieldBelowMinLength                     = errors.New("field is below minimum length")
	ErrCannotPerformOperationOnSelf            = errors.New("cannot perform operation on self")
	ErrActorAndTargetIDsRequired               = errors.New("both actor and target IDs are required for operation")
	ErrContentExceedsMaxLength                 = errors.New("content exceeds maximum length")
	ErrContentBelowMinLength                   = errors.New("content below minimum length")
	ErrContentContainsForbiddenWord            = errors.New("content contains forbidden word")
	ErrInvalidVisibilityLevel                  = errors.New("invalid visibility level")
	ErrActorIDRequiredForQuotaValidation       = errors.New("actorID is required for quota validation")
	ErrActionTypeRequiredForQuotaValidation    = errors.New("actionType is required for quota validation")
	ErrMetricTypeRequired                      = errors.New("metricType is required")
	ErrActorIDRequiredForMetrics               = errors.New("actorID is required for metrics")
	ErrInvalidCurrentState                     = errors.New("invalid current state")
	ErrInvalidStateTransition                  = errors.New("invalid state transition")
	ErrAuthenticationRequiredForAccess         = errors.New("authentication required to access resource")
	ErrResourceIDRequiredForAccessValidation   = errors.New("resourceID is required for access validation")
	ErrResourceTypeRequiredForAccessValidation = errors.New("resourceType is required for access validation")
	ErrInvalidAccessLevel                      = errors.New("invalid access level")
	ErrOperationValidationFailed               = errors.New("operation validation failed")
	ErrOperationExecutionFailed                = errors.New("operation execution failed")
)

// ActorNotFoundError indicates an actor was not found
type ActorNotFoundError struct {
	Username string
}

func (e ActorNotFoundError) Error() string {
	return fmt.Sprintf("actor not found: %s", e.Username)
}

// ActivityNotFoundError indicates an activity was not found
type ActivityNotFoundError struct {
	ID string
}

func (e ActivityNotFoundError) Error() string {
	return fmt.Sprintf("activity not found: %s", e.ID)
}

// ValidationError indicates input validation failed
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation failed for %s: %s", e.Field, e.Message)
}

// AuthenticationError indicates authentication failed
type AuthenticationError struct {
	Message string
}

func (e AuthenticationError) Error() string {
	return fmt.Sprintf("authentication failed: %s", e.Message)
}

// AuthorizationError indicates authorization failed
type AuthorizationError struct {
	Action   string
	Resource string
}

func (e AuthorizationError) Error() string {
	return fmt.Sprintf("not authorized to %s %s", e.Action, e.Resource)
}

// ConflictError indicates a resource conflict
type ConflictError struct {
	Resource string
	Message  string
}

func (e ConflictError) Error() string {
	return fmt.Sprintf("conflict with %s: %s", e.Resource, e.Message)
}

// UserNotFoundError indicates a user was not found
type UserNotFoundError struct {
	Username string
}

func (e UserNotFoundError) Error() string {
	return fmt.Sprintf("user not found: %s", e.Username)
}

// AccountSuspendedError indicates an account is suspended
type AccountSuspendedError struct {
	Username string
}

func (e AccountSuspendedError) Error() string {
	return fmt.Sprintf("account suspended: %s", e.Username)
}

// InvalidPasswordError indicates password validation failed
type InvalidPasswordError struct{}

func (e InvalidPasswordError) Error() string {
	return "invalid password"
}

// InvalidTokenError indicates a token is invalid
type InvalidTokenError struct {
	Token string
}

func (e InvalidTokenError) Error() string {
	return fmt.Sprintf("invalid token: %s", e.Token)
}

// ExpiredTokenError indicates a token has expired
type ExpiredTokenError struct {
	Token string
}

func (e ExpiredTokenError) Error() string {
	return fmt.Sprintf("token expired: %s", e.Token)
}

// UsedTokenError indicates a token has already been used
type UsedTokenError struct {
	Token string
}

func (e UsedTokenError) Error() string {
	return fmt.Sprintf("token already used: %s", e.Token)
}

// SessionNotFoundError indicates a session was not found
type SessionNotFoundError struct {
	SessionID string
}

func (e SessionNotFoundError) Error() string {
	return fmt.Sprintf("session not found: %s", e.SessionID)
}

// AlreadyFollowingError indicates already following the user
type AlreadyFollowingError struct {
	Follower string
	Followee string
}

func (e AlreadyFollowingError) Error() string {
	return fmt.Sprintf("%s is already following %s", e.Follower, e.Followee)
}

// ListNotFoundError indicates a list was not found
type ListNotFoundError struct {
	ID string
}

func (e ListNotFoundError) Error() string {
	return fmt.Sprintf("list not found: %s", e.ID)
}

// FederationError indicates a federation operation failed
type FederationError struct {
	Operation string
	Remote    string
	Err       error
}

func (e FederationError) Error() string {
	return fmt.Sprintf("federation %s failed for %s: %v", e.Operation, e.Remote, e.Err)
}

func (e FederationError) Unwrap() error {
	return e.Err
}

// AppError represents a safe application error that separates internal details from user messages
type AppError struct {
	Code          string // Internal error code for logging/monitoring
	UserMessage   string // Safe message for users
	InternalError error  // Detailed error for logging
	StatusCode    int    // HTTP status code
}

// Error implements the error interface
func (e AppError) Error() string {
	return e.UserMessage
}

// ErrUnauthorized creates an unauthorized error
func ErrUnauthorized(internal error) AppError {
	return AppError{
		Code:          "AUTH_FAILED",
		UserMessage:   "Authentication failed",
		InternalError: internal,
		StatusCode:    401,
	}
}

// ErrNotFound creates a not found error
func ErrNotFound(resource string) AppError {
	return AppError{
		Code:          "NOT_FOUND",
		UserMessage:   "Resource not found",
		InternalError: errors.New(resource + " not found"),
		StatusCode:    404,
	}
}

// ErrForbidden creates a forbidden error
func ErrForbidden(internal error) AppError {
	return AppError{
		Code:          "FORBIDDEN",
		UserMessage:   "Access denied",
		InternalError: internal,
		StatusCode:    403,
	}
}

// ErrBadRequest creates a bad request error
func ErrBadRequest(userMessage string, internal error) AppError {
	return AppError{
		Code:          "BAD_REQUEST",
		UserMessage:   userMessage,
		InternalError: internal,
		StatusCode:    400,
	}
}

// ErrInternal creates an internal server error
func ErrInternal(internal error) AppError {
	return AppError{
		Code:          "INTERNAL_ERROR",
		UserMessage:   "An error occurred processing your request",
		InternalError: internal,
		StatusCode:    500,
	}
}

// ErrValidation creates a validation error
func ErrValidation(field string, message string) AppError {
	return AppError{
		Code:          "VALIDATION_ERROR",
		UserMessage:   message,
		InternalError: errors.New("validation failed for field " + field + ": " + message),
		StatusCode:    400,
	}
}

// HandleError processes an error and returns safe HTTP response values
func HandleError(logger *zap.Logger, err error) (int, string) {
	if appErr, ok := err.(AppError); ok {
		// Log the internal details
		logger.Error("Request failed",
			zap.String("code", appErr.Code),
			zap.Error(appErr.InternalError),
			zap.Int("status", appErr.StatusCode))

		// Return safe user message
		return appErr.StatusCode, fmt.Sprintf(`{"error": "%s", "code": "%s"}`, appErr.UserMessage, appErr.Code)
	}

	// Unknown errors get generic message
	logger.Error("Unexpected error", zap.Error(err))
	return 500, `{"error": "An error occurred processing your request", "code": "INTERNAL_ERROR"}`
}

// WrapError wraps an error with context while keeping it safe for users
func WrapError(err error, context string) error {
	if appErr, ok := err.(AppError); ok {
		// Already an AppError, add context to internal error
		appErr.InternalError = fmt.Errorf("%s: %w", context, appErr.InternalError)
		return appErr
	}

	// Regular error, wrap as internal
	return ErrInternal(fmt.Errorf("%s: %w", context, err))
}

// Error checking functions

// IsNotFound returns true if the error is a not found error
func IsNotFound(err error) bool {
	switch err.(type) {
	case ActorNotFoundError, ActivityNotFoundError:
		return true
	}
	return false
}

// IsValidation returns true if the error is a validation error
func IsValidation(err error) bool {
	_, ok := err.(ValidationError)
	return ok
}

// IsAuthentication returns true if the error is an authentication error
func IsAuthentication(err error) bool {
	_, ok := err.(AuthenticationError)
	return ok
}

// IsAuthorization returns true if the error is an authorization error
func IsAuthorization(err error) bool {
	_, ok := err.(AuthorizationError)
	return ok
}

// IsConflict returns true if the error is a conflict error
func IsConflict(err error) bool {
	_, ok := err.(ConflictError)
	return ok
}

// IsFederation returns true if the error is a federation error
func IsFederation(err error) bool {
	_, ok := err.(FederationError)
	return ok
}

// ActivityPub-specific errors
var (
	ErrActorURIEmpty           = errors.New("actor URI cannot be empty")
	ErrActivityTypeEmpty       = errors.New("activity type cannot be empty")
	ErrSignatureHeaderEmpty    = errors.New("signature header is empty")
	ErrInvalidSignature        = errors.New("invalid signature: missing keyId or signature")
	ErrActorURIMustUseHTTPS    = errors.New("actor URI must use HTTPS")
	ErrActorDomainBlocked      = errors.New("actor domain is blocked")
	ErrActorDomainNotAllowed   = errors.New("actor domain not in allowed list")
	ErrUnsupportedActivityType = errors.New("unsupported activity type")
)

// Status validation errors
var (
	ErrStatusContentTooLong     = errors.New("status content exceeds maximum length")
	ErrStatusMustHaveContent    = errors.New("status must have content or media attachments")
	ErrStatusTooManyMedia       = errors.New("status cannot have more than 4 media attachments")
	ErrStatusTooManyPollOptions = errors.New("poll cannot have more than maximum options")
)

// Account validation errors
var (
	ErrDisplayNameTooLong             = errors.New("display name exceeds maximum length")
	ErrUsernameEmpty                  = errors.New("username cannot be empty")
	ErrUsernameInvalidCharacters      = errors.New("username can only contain letters, numbers, and underscores")
	ErrUsernameConsecutiveUnderscores = errors.New("username cannot contain consecutive underscores")
	ErrUsernameInvalidLength          = errors.New("username must be between 1 and 30 characters")
	ErrUsernameInvalidFormat          = errors.New("username cannot start or end with underscore")
	ErrBioTooLong                     = errors.New("bio exceeds maximum length")
)

// Media validation errors
var (
	ErrVideoFileTooLarge    = errors.New("video file size exceeds limit")
	ErrMediaFileTooLarge    = errors.New("media file size exceeds limit")
	ErrInvalidImageMimeType = errors.New("invalid image MIME type")
	ErrInvalidVideoMimeType = errors.New("invalid video MIME type")
	ErrInvalidAudioMimeType = errors.New("invalid audio MIME type")
)

// Filter validation errors
var (
	ErrFilterKeywordEmpty   = errors.New("filter keyword cannot be empty")
	ErrFilterKeywordTooLong = errors.New("filter keyword exceeds maximum length")
)

// Poll validation errors
var (
	ErrPollTooFewOptions  = errors.New("poll must have at least 2 options")
	ErrPollTooManyOptions = errors.New("poll cannot have more than maximum options")
	ErrPollOptionEmpty    = errors.New("poll option cannot be empty")
	ErrPollOptionTooLong  = errors.New("poll option exceeds maximum length")
	ErrPollExpiryInvalid  = errors.New("poll expiry must be positive")
	ErrPollExpiryTooLong  = errors.New("poll expiry cannot exceed maximum seconds")
)

// Form parsing errors
var (
	ErrNoBoundaryInContentType = errors.New("no boundary found in content type")
)

// General validation errors
var (
	ErrInvalidNotificationType = errors.New("invalid notification type")
	ErrInvalidOAuthScope       = errors.New("invalid OAuth scope")
	ErrIDEmpty                 = errors.New("ID cannot be empty")
	ErrInvalidIDFormat         = errors.New("invalid ID format")
	ErrInvalidTimestampFormat  = errors.New("invalid timestamp format")
)

// JSON safety validation errors
var (
	ErrJSONDepthExceedsMaximum    = errors.New("JSON depth exceeds maximum")
	ErrJSONObjectTooManyKeys      = errors.New("JSON object has too many keys")
	ErrJSONKeyTooLong             = errors.New("JSON key too long")
	ErrJSONArrayTooManyElements   = errors.New("JSON array has too many elements")
	ErrJSONStringTooLong          = errors.New("JSON string too long")
	ErrUnexpectedJSONType         = errors.New("unexpected JSON type")
	ErrJSONSizeExceedsMaximum     = errors.New("JSON size exceeds maximum")
	ErrJSONBombRepetitionDetected = errors.New("possible JSON bomb: excessive repetition detected")
	ErrJSONBombNestingDetected    = errors.New("possible JSON bomb: excessive nesting detected")
)

// Lambda helper errors
var (
	ErrDynamoTableRequired             = errors.New("DYNAMODB_TABLE is required for storage initialization")
	ErrDynamORMNotImplemented          = errors.New("DynamORM initialization to be implemented in service-specific code")
	ErrRepositoryFactoryNotImplemented = errors.New("repository factory initialization to be implemented in service-specific code")
	ErrAuthServicesNotImplemented      = errors.New("auth services initialization to be implemented in service-specific code")
)

// Redirect validation errors
var (
	ErrRedirectURLEmpty               = errors.New("redirect URL cannot be empty")
	ErrProtocolRelativeURLsNotAllowed = errors.New("protocol-relative URLs not allowed")
	ErrJavascriptDataURLsNotAllowed   = errors.New("javascript: and data: URLs not allowed")
	ErrExternalHostNotAllowed         = errors.New("redirect to external host not allowed")
)

// Request handling errors
var (
	ErrRequestBodyTooLarge              = errors.New("request body too large")
	ErrFailedToParseRequestBody         = errors.New("failed to parse request body")
	ErrFailedToParseWithComplexFallback = errors.New("failed to parse request with complex fallback")
)

// Resource monitoring errors
var (
	ErrLambdaTimeoutApproaching = errors.New("operation approaching Lambda timeout")
	ErrMemoryLimitExceeded      = errors.New("memory limit exceeded")
)

// Transformer errors
var (
	ErrTransformFunctionNotSet       = errors.New("transform function not set")
	ErrTransformationConditionNotMet = errors.New("transformation condition not met")
	ErrUnknownNumber                 = errors.New("unknown number")
)

// Mastodon business logic errors
var (
	ErrMastodonOperationValidationFailed = errors.New("mastodon operation validation failed")
	ErrMastodonOperationExecutionFailed  = errors.New("mastodon operation execution failed")
	ErrMastodonAPIIncompatibility        = errors.New("mastodon API incompatibility")
	ErrInvalidBusinessRule               = errors.New("invalid business rule")
	ErrBusinessValidationFailed          = errors.New("business validation failed")
	ErrInvalidRequestFormat              = errors.New("invalid request format")
	ErrUnsupportedOperation              = errors.New("unsupported operation")
)
