// Package common provides standardized error response functions for HTTP APIs.
// It consolidates error handling patterns to maintain consistency across the application.
package common

import (
	"fmt"

	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/pay-theory/lift/pkg/lift"
)

// StandardErrorResponse represents a standardized API error response
type StandardErrorResponse struct {
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
	Code        string `json:"error_code,omitempty"`
}

// Common error response patterns consolidated into reusable functions
// This consolidates the 400+ occurrences of ctx.Status(4XX).JSON(map[string]string{"error": "..."})

// RespondUnauthorized handles authentication errors (401) - now using centralized errors
func RespondUnauthorized(ctx *lift.Context, message ...string) error {
	msg := "Unauthorized"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	appErr := errors.Unauthorized(msg)
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondUnauthorizedWithDescription handles authentication errors (401) with additional description
func RespondUnauthorizedWithDescription(ctx *lift.Context, description string) error {
	appErr := errors.Unauthorized("Unauthorized")
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error:       appErr.Message,
		Description: description,
		Code:        string(appErr.Code),
	})
}

// RespondMissingAuth handles authentication required errors by returning a 401 unauthorized response
func RespondMissingAuth(ctx *lift.Context) error {
	return RespondUnauthorized(ctx, "authentication required")
}

// RespondInvalidToken handles invalid token errors by returning a 401 unauthorized response
func RespondInvalidToken(ctx *lift.Context) error {
	appErr := errors.NewAuthError(errors.CodeTokenInvalid, "invalid token")
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondExpiredToken handles expired token errors by returning a 401 unauthorized response
func RespondExpiredToken(ctx *lift.Context) error {
	appErr := errors.NewAuthError(errors.CodeTokenExpired, "token expired")
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondForbidden handles authorization/permission errors (403) - now using centralized errors
func RespondForbidden(ctx *lift.Context, message ...string) error {
	msg := "Forbidden"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	appErr := errors.Forbidden(msg)
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondInsufficientScope handles insufficient OAuth scope errors by returning a 403 forbidden response
func RespondInsufficientScope(ctx *lift.Context, requiredScope ...string) error {
	msg := "insufficient scope"
	if len(requiredScope) > 0 {
		msg = fmt.Sprintf("insufficient scope: requires %s", requiredScope[0])
	}
	appErr := errors.NewAppError(errors.CodeInsufficientScope, errors.CategoryAuth, msg)
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondNotAuthorized handles general authorization errors by returning a 403 forbidden response
func RespondNotAuthorized(ctx *lift.Context, resource string) error {
	return RespondForbidden(ctx, fmt.Sprintf("not authorized to access %s", resource))
}

// RespondNotAuthorizedToModify handles modification authorization errors by returning a 403 forbidden response
func RespondNotAuthorizedToModify(ctx *lift.Context, resource string) error {
	return RespondForbidden(ctx, fmt.Sprintf("not authorized to modify %s", resource))
}

// RespondNotAuthorizedToDelete handles deletion authorization errors by returning a 403 forbidden response
func RespondNotAuthorizedToDelete(ctx *lift.Context, resource string) error {
	return RespondForbidden(ctx, fmt.Sprintf("not authorized to delete %s", resource))
}

// RespondBadRequest handles validation and bad request errors (400) - now using centralized errors
func RespondBadRequest(ctx *lift.Context, message ...string) error {
	msg := "Bad Request"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	appErr := errors.BadRequest(msg)
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondValidationError handles validation failed errors - now using centralized errors
func RespondValidationError(ctx *lift.Context, err error) error {
	var appErr *errors.AppError
	if err != nil {
		if existingAppErr, ok := errors.AsAppError(err); ok {
			appErr = existingAppErr
		} else {
			appErr = errors.ValidationFailed("input", err.Error())
		}
	} else {
		appErr = errors.ValidationFailed("input", "Validation failed")
	}
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondMissingParameter handles missing required parameter errors by returning a 400 bad request response
func RespondMissingParameter(ctx *lift.Context, paramName string) error {
	return RespondBadRequest(ctx, fmt.Sprintf("missing required parameter: %s", paramName))
}

// RespondInvalidParameter handles invalid parameter errors by returning a 400 bad request response
func RespondInvalidParameter(ctx *lift.Context, paramName string) error {
	return RespondBadRequest(ctx, fmt.Sprintf("invalid parameter: %s", paramName))
}

// RespondMissingAccountID handles missing account ID errors by returning a 400 bad request response
func RespondMissingAccountID(ctx *lift.Context) error {
	return RespondBadRequest(ctx, "missing account id")
}

// RespondMissingStatusID handles missing status ID errors by returning a 400 bad request response
func RespondMissingStatusID(ctx *lift.Context) error {
	return RespondBadRequest(ctx, "missing status id")
}

// RespondInvalidRequest handles general invalid request errors by returning a 400 bad request response
func RespondInvalidRequest(ctx *lift.Context) error {
	return RespondBadRequest(ctx, "invalid request")
}

// RespondNotFound handles resource not found errors (404) - now using centralized errors
func RespondNotFound(ctx *lift.Context, resource ...string) error {
	var appErr *errors.AppError
	if len(resource) > 0 && resource[0] != "" {
		appErr = errors.NotFound(resource[0])
	} else {
		appErr = errors.NotFound("resource")
	}
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondAccountNotFound handles account not found errors by returning a 404 not found response
func RespondAccountNotFound(ctx *lift.Context) error {
	return RespondNotFound(ctx, "account")
}

// RespondStatusNotFound handles status not found errors by returning a 404 not found response
func RespondStatusNotFound(ctx *lift.Context) error {
	return RespondNotFound(ctx, "status")
}

// RespondUserNotFound handles user not found errors by returning a 404 not found response
func RespondUserNotFound(ctx *lift.Context) error {
	return RespondNotFound(ctx, "user")
}

// RespondActorNotFound handles actor not found errors by returning a 404 not found response
func RespondActorNotFound(ctx *lift.Context) error {
	return RespondNotFound(ctx, "actor")
}

// RespondFilterNotFound handles filter not found errors by returning a 404 not found response
func RespondFilterNotFound(ctx *lift.Context) error {
	return RespondNotFound(ctx, "filter")
}

// RespondConversationNotFound handles conversation not found errors by returning a 404 not found response
func RespondConversationNotFound(ctx *lift.Context) error {
	return RespondNotFound(ctx, "conversation")
}

// RespondMethodNotAllowed handles HTTP method not allowed errors by returning a 405 response
func RespondMethodNotAllowed(ctx *lift.Context) error {
	appErr := errors.NewAppError(errors.CodeMethodNotAllowed, errors.CategoryAPI, "Method Not Allowed")
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondConflict handles resource conflict errors (409) - now using centralized errors
func RespondConflict(ctx *lift.Context, message ...string) error {
	msg := "Conflict"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	appErr := errors.NewAppError(errors.CodeConflict, errors.CategoryBusiness, msg)
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondAlreadyExists handles resource already exists conflicts by returning a 409 conflict response
func RespondAlreadyExists(ctx *lift.Context, resource string) error {
	return RespondConflict(ctx, fmt.Sprintf("%s already exists", resource))
}

// RespondGone handles resource gone errors (410) with optional custom message
func RespondGone(ctx *lift.Context, message ...string) error {
	msg := "Gone"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	appErr := errors.NewAppError(errors.CodeGone, errors.CategoryAPI, msg)
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondUnprocessableEntity handles unprocessable entity errors (422) with optional custom message
func RespondUnprocessableEntity(ctx *lift.Context, message ...string) error {
	msg := "Unprocessable Entity"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	appErr := errors.NewAppError(errors.CodeUnprocessableEntity, errors.CategoryValidation, msg)
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondStatusTooLong handles status text too long errors by returning a 422 unprocessable entity response
func RespondStatusTooLong(ctx *lift.Context) error {
	return RespondUnprocessableEntity(ctx, "status text too long")
}

// RespondInvalidContent handles invalid content errors by returning a 422 unprocessable entity response
func RespondInvalidContent(ctx *lift.Context) error {
	return RespondUnprocessableEntity(ctx, "invalid content")
}

// RespondRateLimited handles rate limit exceeded errors by returning a 429 response
func RespondRateLimited(ctx *lift.Context) error {
	appErr := errors.NewAppError(errors.CodeRateLimited, errors.CategoryAPI, "Rate limit exceeded")
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondInternalServerError handles internal server errors (500) - now using centralized errors
func RespondInternalServerError(ctx *lift.Context, message ...string) error {
	msg := "Internal server error"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	appErr := errors.Internal(msg)
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondDatabaseError handles database errors by returning a 500 internal server error response
func RespondDatabaseError(ctx *lift.Context) error {
	return RespondInternalServerError(ctx, "database error")
}

// RespondFailedToCreate handles resource creation failures by returning a 500 internal server error response
func RespondFailedToCreate(ctx *lift.Context, resource string) error {
	return RespondInternalServerError(ctx, fmt.Sprintf("failed to create %s", resource))
}

// RespondFailedToUpdate handles resource update failures by returning a 500 internal server error response
func RespondFailedToUpdate(ctx *lift.Context, resource string) error {
	return RespondInternalServerError(ctx, fmt.Sprintf("failed to update %s", resource))
}

// RespondFailedToDelete handles resource deletion failures by returning a 500 internal server error response
func RespondFailedToDelete(ctx *lift.Context, resource string) error {
	return RespondInternalServerError(ctx, fmt.Sprintf("failed to delete %s", resource))
}

// RespondFailedToGet handles resource retrieval failures by returning a 500 internal server error response
func RespondFailedToGet(ctx *lift.Context, resource string) error {
	return RespondInternalServerError(ctx, fmt.Sprintf("failed to get %s", resource))
}

// RespondServiceUnavailable handles service unavailable errors (503) with optional service name
func RespondServiceUnavailable(ctx *lift.Context, service ...string) error {
	msg := "Service Unavailable"
	if len(service) > 0 && service[0] != "" {
		msg = fmt.Sprintf("%s service unavailable", service[0])
	}
	appErr := errors.NewAppError(errors.CodeExternalServiceUnavailable, errors.CategoryExternal, msg)
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// Composite error response functions that handle common patterns

// RespondAuthErrorWithCode handles authentication/authorization errors with status code
func RespondAuthErrorWithCode(ctx *lift.Context, errorCode int, err error) error {
	if err == nil {
		return RespondInternalServerError(ctx, "unexpected auth error")
	}

	switch errorCode {
	case 401:
		return RespondUnauthorized(ctx, err.Error())
	case 403:
		return RespondForbidden(ctx, err.Error())
	default:
		return RespondUnauthorized(ctx, err.Error())
	}
}

// RespondValidationOrError handles validation errors and other errors appropriately
func RespondValidationOrError(ctx *lift.Context, err error) error {
	// Check if it's a validation error type (this can be enhanced with proper type checking)
	if err != nil && (containsValidationKeywords(err.Error())) {
		return RespondValidationError(ctx, err)
	}
	return RespondBadRequest(ctx, err.Error())
}

// containsValidationKeywords checks if error message contains validation-related keywords
func containsValidationKeywords(errMsg string) bool {
	keywords := []string{"cannot be blank", "too long", "invalid format", "must be", "required"}
	for _, keyword := range keywords {
		if len(errMsg) > 0 && errMsg[0:minInt(len(keyword), len(errMsg))] == keyword {
			return true
		}
	}
	return false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Common error response patterns for specific operations

// RespondCreateError handles errors from create operations - now using centralized errors
func RespondCreateError(ctx *lift.Context, resource string, err error) error {
	if err == nil {
		return RespondInternalServerError(ctx, fmt.Sprintf("unknown error creating %s", resource))
	}

	// Check for centralized AppError
	if appErr, ok := errors.AsAppError(err); ok {
		return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
			Error: appErr.Message,
			Code:  string(appErr.Code),
		})
	}

	// Check for validation errors
	if containsValidationKeywords(err.Error()) {
		return RespondValidationError(ctx, err)
	}

	// Check for conflict errors
	if isConflictError(err) {
		return RespondAlreadyExists(ctx, resource)
	}

	// Default to server error
	return RespondFailedToCreate(ctx, resource)
}

// RespondUpdateError handles errors from update operations - now using centralized errors
func RespondUpdateError(ctx *lift.Context, resource string, err error) error {
	if err == nil {
		return RespondInternalServerError(ctx, fmt.Sprintf("unknown error updating %s", resource))
	}

	// Check for centralized AppError
	if appErr, ok := errors.AsAppError(err); ok {
		return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
			Error: appErr.Message,
			Code:  string(appErr.Code),
		})
	}

	// Check for validation errors
	if containsValidationKeywords(err.Error()) {
		return RespondValidationError(ctx, err)
	}

	// Check for not found errors
	if isNotFoundError(err) {
		return RespondNotFound(ctx, resource)
	}

	// Default to server error
	return RespondFailedToUpdate(ctx, resource)
}

// RespondDeleteError handles errors from delete operations - now using centralized errors
func RespondDeleteError(ctx *lift.Context, resource string, err error) error {
	if err == nil {
		return RespondInternalServerError(ctx, fmt.Sprintf("unknown error deleting %s", resource))
	}

	// Check for centralized AppError
	if appErr, ok := errors.AsAppError(err); ok {
		return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
			Error: appErr.Message,
			Code:  string(appErr.Code),
		})
	}

	// Check for not found errors
	if isNotFoundError(err) {
		return RespondNotFound(ctx, resource)
	}

	// Default to server error
	return RespondFailedToDelete(ctx, resource)
}

// RespondGetError handles errors from get/fetch operations - now using centralized errors
func RespondGetError(ctx *lift.Context, resource string, err error) error {
	if err == nil {
		return RespondInternalServerError(ctx, fmt.Sprintf("unknown error getting %s", resource))
	}

	// Check for centralized AppError
	if appErr, ok := errors.AsAppError(err); ok {
		return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
			Error: appErr.Message,
			Code:  string(appErr.Code),
		})
	}

	// Check for not found errors
	if isNotFoundError(err) {
		return RespondNotFound(ctx, resource)
	}

	// Default to server error
	return RespondFailedToGet(ctx, resource)
}

// Error type checking helpers - now using centralized error system
func isConflictError(err error) bool {
	return IsConflict(err) ||
		errors.HasCode(err, errors.CodeConflict) ||
		errors.HasCode(err, errors.CodeAlreadyExists)
}

func isNotFoundError(err error) bool {
	return IsNotFound(err) ||
		errors.HasCode(err, errors.CodeNotFound) ||
		errors.HasCode(err, errors.CodeActorNotFound)
}

// Helper functions for common response patterns

// RespondWithAppError creates a response from a centralized AppError
func RespondWithAppError(ctx *lift.Context, appErr *errors.AppError) error {
	return ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondWithErrorMessage creates a standardized error response with custom message
func RespondWithErrorMessage(ctx *lift.Context, statusCode int, message string) error {
	return ctx.Status(statusCode).JSON(StandardErrorResponse{
		Error: message,
		Code:  errorCodeForHTTPStatus(statusCode),
	})
}

// RespondWithErrorAndDescription creates a detailed error response
func RespondWithErrorAndDescription(ctx *lift.Context, statusCode int, errorMsg, description string) error {
	return ctx.Status(statusCode).JSON(StandardErrorResponse{
		Error:       errorMsg,
		Description: description,
		Code:        errorCodeForHTTPStatus(statusCode),
	})
}

// RespondWithErrorCode creates an error response with an error code
func RespondWithErrorCode(ctx *lift.Context, statusCode int, errorMsg, code string) error {
	return ctx.Status(statusCode).JSON(StandardErrorResponse{
		Error: errorMsg,
		Code:  code,
	})
}

// Legacy compatibility functions for existing patterns
// These maintain backward compatibility while encouraging migration to new patterns

// RespondLegacyError maintains compatibility with existing map[string]string{"error": "message"} pattern
func RespondLegacyError(ctx *lift.Context, statusCode int, message string) error {
	return ctx.Status(statusCode).JSON(map[string]string{"error": message})
}

// Common error status/message combinations found in the codebase
var (
	// 400 Bad Request variants
	ErrorMissingAccountID = "missing account id"
	ErrorMissingStatusID  = "missing status id"
	ErrorMissingParameter = "missing required parameter"
	ErrorInvalidParameter = "invalid parameter"
	ErrorInvalidRequest   = "invalid request"
	ErrorInvalidFormat    = "invalid format"

	// 401 Unauthorized variants
	ErrorUnauthorized = "Unauthorized"
	ErrorInvalidToken = "invalid token"
	ErrorMissingAuth  = "authentication required"
	ErrorExpiredToken = "token expired"

	// 403 Forbidden variants
	ErrorInsufficientScope = "insufficient scope"
	ErrorNotAuthorized     = "not authorized"
	ErrorAccessDenied      = "access denied"

	// 404 Not Found variants
	ErrorNotFound        = "not found"
	ErrorAccountNotFound = "account not found"
	ErrorStatusNotFound  = "status not found"
	ErrorUserNotFound    = "user not found"
	ErrorActorNotFound   = "actor not found"

	// 422 Unprocessable Entity variants
	ErrorStatusTooLong  = "status text too long"
	ErrorInvalidContent = "invalid content"

	// 500 Internal Server Error variants
	ErrorInternalServer = "internal server error"
	ErrorDatabaseError  = "database error"
	ErrorFailedToCreate = "failed to create"
	ErrorFailedToUpdate = "failed to update"
	ErrorFailedToDelete = "failed to delete"
	ErrorFailedToGet    = "failed to get"
)

// RespondSuccess handles successful responses with data
func RespondSuccess(ctx *lift.Context, data interface{}) error {
	return ctx.Status(200).JSON(data)
}
