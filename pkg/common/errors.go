package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	stdErrors "errors"
	"fmt"

	"github.com/equaltoai/lesser/pkg/errors"
	"go.uber.org/zap"
)

// Domain-specific error types

// Token-related errors
var (
	ErrTokenNotFound = errors.TokenNotFound()
	ErrTokenExpired  = errors.TokenExpired()
	ErrTokenRevoked  = errors.TokenRevoked()
)

// Auth helper errors
var (
	ErrMissingAuthorizationHeader = errors.NewAuthError(errors.CodeAuthFailed, "Missing authorization header")
	ErrAuthHeaderEmpty            = errors.NewAuthError(errors.CodeAuthFailed, "Authorization header is empty")
	ErrAuthHeaderInvalidPrefix    = errors.NewAuthError(errors.CodeAuthFailed, "Authorization header must start with 'Bearer '")
	ErrAuthenticationRequired     = errors.NewAuthError(errors.CodeAuthFailed, "Authentication required for write operations")
	ErrAuthenticationRequiredRead = errors.NewAuthError(errors.CodeAuthFailed, "Authentication required for read operations")
	ErrInsufficientScope          = errors.InsufficientScope("")
	ErrInsufficientScopeWrite     = errors.InsufficientScope("write access required")
	ErrInsufficientScopeRead      = errors.InsufficientScope("read access required")
)

// Business logic validation errors
var (
	ErrDataCannotBeNil                         = errors.RequiredFieldMissing("data")
	ErrRequiredFieldMissing                    = errors.RequiredFieldMissing("field")
	ErrFieldExceedsMaxLength                   = errors.FieldTooLong("field", 0, 0)
	ErrFieldBelowMinLength                     = errors.FieldTooShort("field", 0, 0)
	ErrCannotPerformOperationOnSelf            = errors.OperationNotAllowedOnSelf("operation")
	ErrActorAndTargetIDsRequired               = errors.MultipleValidationErrors([]string{"Both actor and target IDs are required for operation"})
	ErrContentExceedsMaxLength                 = errors.ContentTooLong("content", 0)
	ErrContentBelowMinLength                   = errors.ContentEmpty("content")
	ErrContentContainsForbiddenWord            = errors.ContentContainsForbiddenWord("word")
	ErrInvalidVisibilityLevel                  = errors.StatusInvalidVisibility("")
	ErrActorIDRequiredForQuotaValidation       = errors.RequiredFieldMissing("actorID")
	ErrActionTypeRequiredForQuotaValidation    = errors.RequiredFieldMissing("actionType")
	ErrMetricTypeRequired                      = errors.RequiredFieldMissing("metricType")
	ErrActorIDRequiredForMetrics               = errors.RequiredFieldMissing("actorID")
	ErrInvalidCurrentState                     = errors.InvalidStateForOperation("", "operation")
	ErrInvalidStateTransition                  = errors.InvalidStateForOperation("", "state_transition")
	ErrAuthenticationRequiredForAccess         = errors.InsufficientPermissions("access")
	ErrResourceIDRequiredForAccessValidation   = errors.RequiredFieldMissing("resourceID")
	ErrResourceTypeRequiredForAccessValidation = errors.RequiredFieldMissing("resourceType")
	ErrInvalidAccessLevel                      = errors.InvalidValue("access_level", []string{"read", "write", "admin"}, "")
	ErrOperationValidationFailed               = errors.ValidationFailedWithField("operation")
	ErrOperationExecutionFailed                = errors.ProcessingFailed("operation", stdErrors.New("operation execution failed"))
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
		InternalError: fmt.Errorf("%s not found", resource),
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
		InternalError: fmt.Errorf("validation failed for field %s: %s", field, message),
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
	ErrActorURIEmpty           = errors.ActivityPubActorURIEmpty()
	ErrActivityTypeEmpty       = errors.ActivityPubActivityTypeEmpty()
	ErrSignatureHeaderEmpty    = errors.ActivityPubSignatureHeaderEmpty()
	ErrInvalidSignature        = errors.ActivityPubInvalidSignature()
	ErrActorURIMustUseHTTPS    = errors.ActivityPubActorURIMustUseHTTPS()
	ErrActorDomainBlocked      = errors.ContentNotAllowed("actor_domain", "domain is blocked")
	ErrActorDomainNotAllowed   = errors.ContentNotAllowed("actor_domain", "domain not in allowed list")
	ErrUnsupportedActivityType = errors.ActivityPubUnsupportedActivityType("")
)

// Status validation errors
var (
	ErrStatusContentTooLong     = errors.ContentTooLong("status", 500)
	ErrStatusMustHaveContent    = errors.ContentMustHaveContentOrMedia()
	ErrStatusTooManyMedia       = errors.StatusTooManyMedia(4, 0)
	ErrStatusTooManyPollOptions = errors.PollTooManyOptions(4, 0)
)

// Account validation errors
var (
	ErrDisplayNameTooLong             = errors.DisplayNameTooLong(30)
	ErrUsernameEmpty                  = errors.UsernameEmpty()
	ErrUsernameInvalidCharacters      = errors.UsernameInvalidCharacters()
	ErrUsernameConsecutiveUnderscores = errors.UsernameConsecutiveUnderscores()
	ErrUsernameInvalidLength          = errors.UsernameInvalidLength(1, 30)
	ErrUsernameInvalidFormat          = errors.UsernameStartsOrEndsWithUnderscore()
	ErrBioTooLong                     = errors.BioTooLong(160)
)

// Media validation errors
var (
	ErrVideoFileTooLarge    = errors.VideoFileTooLarge(0, 0)
	ErrMediaFileTooLarge    = errors.MediaFileTooLarge(0, 0)
	ErrInvalidImageMimeType = errors.ImageInvalidFormat("")
	ErrInvalidVideoMimeType = errors.VideoInvalidFormat("")
	ErrInvalidAudioMimeType = errors.AudioInvalidFormat("")
)

// Filter validation errors
var (
	ErrFilterKeywordEmpty   = errors.FilterKeywordEmpty()
	ErrFilterKeywordTooLong = errors.FilterKeywordTooLong(100)
)

// Poll validation errors
var (
	ErrPollTooFewOptions  = errors.PollTooFewOptions(2)
	ErrPollTooManyOptions = errors.PollTooManyOptions(4, 0)
	ErrPollOptionEmpty    = errors.PollOptionEmpty()
	ErrPollOptionTooLong  = errors.PollOptionTooLong(50)
	ErrPollExpiryInvalid  = errors.PollExpiryInvalid()
	ErrPollExpiryTooLong  = errors.PollExpiryTooLong(604800)
)

// Form parsing errors
var (
	ErrNoBoundaryInContentType = errors.FormBoundaryMissing()
)

// General validation errors
var (
	ErrInvalidNotificationType = errors.NotificationTypeInvalid("")
	ErrInvalidOAuthScope       = errors.OAuthScopeInvalid("")
	ErrIDEmpty                 = errors.IDEmpty("")
	ErrInvalidIDFormat         = errors.IDInvalidFormat("")
	ErrInvalidTimestampFormat  = errors.TimestampInvalidFormat("")
)

// JSON safety validation errors
var (
	ErrJSONDepthExceedsMaximum    = errors.JSONTooDeep(10)
	ErrJSONObjectTooManyKeys      = errors.JSONTooManyKeys(100)
	ErrJSONKeyTooLong             = errors.JSONKeyTooLong(256)
	ErrJSONArrayTooManyElements   = errors.JSONArrayTooLarge(1000)
	ErrJSONStringTooLong          = errors.JSONStringTooLong(10000)
	ErrUnexpectedJSONType         = errors.JSONInvalid("unexpected type")
	ErrJSONSizeExceedsMaximum     = errors.JSONSizeTooLarge(1048576)
	ErrJSONBombRepetitionDetected = errors.JSONBombDetected("excessive repetition")
	ErrJSONBombNestingDetected    = errors.JSONBombDetected("excessive nesting")
)

// Lambda helper errors
var (
	ErrDynamoTableRequired             = errors.EnvironmentVariableRequired("DYNAMODB_TABLE")
	ErrDynamORMNotImplemented          = errors.ServiceInitializationFailedGeneric("DynamORM", nil)
	ErrRepositoryFactoryNotImplemented = errors.ServiceInitializationFailedGeneric("repository factory", nil)
	ErrAuthServicesNotImplemented      = errors.ServiceInitializationFailedGeneric("auth services", nil)
)

// Redirect validation errors
var (
	ErrRedirectURLEmpty               = errors.RequiredFieldMissing("redirect_url")
	ErrProtocolRelativeURLsNotAllowed = errors.URLSchemeNotAllowed("", "protocol-relative")
	ErrJavascriptDataURLsNotAllowed   = errors.URLSchemeNotAllowed("", "javascript/data")
	ErrExternalHostNotAllowed         = errors.URLHostNotAllowed("", "external")
)

// Request handling errors
var (
	ErrRequestBodyTooLarge              = errors.FileSizeExceedsLimit(0, 0)
	ErrFailedToParseRequestBody         = errors.ParsingFailed("request body", nil)
	ErrFailedToParseWithComplexFallback = errors.ParsingFailed("request with complex fallback", nil)
)

// Resource monitoring errors
var (
	ErrLambdaTimeoutApproaching = errors.TimeoutError("Lambda operation")
	ErrMemoryLimitExceeded      = errors.LambdaMemoryExceeded("", 0)
)

// Transformer errors
var (
	ErrTransformFunctionNotSet       = errors.TransformFunctionNotSet()
	ErrTransformationConditionNotMet = errors.PreConditionFailed("transformation condition")
	ErrUnknownNumber                 = errors.InvalidValue("number", []string{"valid number"}, "unknown")
)

// Mastodon business logic errors
var (
	ErrMastodonOperationValidationFailed = errors.ValidationFailedWithField("mastodon operation")
	ErrMastodonOperationExecutionFailed  = errors.ProcessingFailed("mastodon operation", stdErrors.New("mastodon operation execution failed"))
	ErrMastodonAPIIncompatibility        = errors.BusinessRuleViolated("mastodon API compatibility", nil)
	ErrInvalidBusinessRule               = errors.BusinessRuleViolated("business rule", nil)
	ErrBusinessValidationFailed          = errors.ValidationFailedWithField("business rule")
	ErrInvalidRequestFormat              = errors.InvalidFormat("request", "expected format")
	ErrUnsupportedOperation              = errors.FormatNotSupported("operation")
)
