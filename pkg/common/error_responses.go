// Package common provides standardized error response functions for HTTP APIs.
// It consolidates error handling patterns to maintain consistency across the application.
package common

import (
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/errors"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
)

// StandardErrorResponse represents a standardized API error response
type StandardErrorResponse struct {
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
	Code        string `json:"error_code,omitempty"`
}

// Common error response patterns consolidated into reusable functions
// This consolidates the 400+ occurrences of apptheory.JSON(4XX, map[string]string{"error": "..."})

// RespondUnauthorized handles authentication errors (401) - now using centralized errors
func RespondUnauthorized(ctx *apptheory.Context, message ...string) (*apptheory.Response, error) {
	msg := "Unauthorized"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	if isBearerAPIAuthPath(ctx) {
		desc := strings.TrimSpace(msg)
		if desc == "" || strings.EqualFold(desc, "unauthorized") {
			desc = "authentication required"
		}
		return RespondBearerInvalidToken(ctx, desc)
	}
	appErr := errors.Unauthorized(msg)
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondUnauthorizedWithDescription handles authentication errors (401) with additional description
func RespondUnauthorizedWithDescription(ctx *apptheory.Context, description string) (*apptheory.Response, error) {
	if isBearerAPIAuthPath(ctx) {
		return RespondBearerInvalidToken(ctx, description)
	}
	appErr := errors.Unauthorized("Unauthorized")
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error:       appErr.Message,
		Description: description,
		Code:        string(appErr.Code),
	})
}

// RespondMissingAuth handles authentication required errors by returning a 401 unauthorized response
func RespondMissingAuth(ctx *apptheory.Context) (*apptheory.Response, error) {
	if isBearerAPIAuthPath(ctx) {
		return RespondBearerMissingAuth(ctx)
	}
	return RespondUnauthorized(ctx, "authentication required")
}

// RespondInvalidToken handles invalid token errors by returning a 401 unauthorized response
func RespondInvalidToken(ctx *apptheory.Context) (*apptheory.Response, error) {
	if isBearerAPIAuthPath(ctx) {
		return RespondBearerInvalidToken(ctx, "invalid token")
	}
	appErr := errors.NewAuthError(errors.CodeTokenInvalid, "invalid token")
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondExpiredToken handles expired token errors by returning a 401 unauthorized response
func RespondExpiredToken(ctx *apptheory.Context) (*apptheory.Response, error) {
	if isBearerAPIAuthPath(ctx) {
		return RespondBearerExpiredToken(ctx, "token expired")
	}
	appErr := errors.NewAuthError(errors.CodeTokenExpired, "token expired")
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondForbidden handles authorization/permission errors (403) - now using centralized errors
func RespondForbidden(_ *apptheory.Context, message ...string) (*apptheory.Response, error) {
	msg := "Forbidden"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	appErr := errors.Forbidden(msg)
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondInsufficientScope handles insufficient OAuth scope errors by returning a 403 forbidden response
func RespondInsufficientScope(ctx *apptheory.Context, requiredScope ...string) (*apptheory.Response, error) {
	if isBearerAPIAuthPath(ctx) {
		return RespondBearerInsufficientScope(ctx, requiredScope...)
	}
	msg := "insufficient scope"
	if len(requiredScope) > 0 {
		msg = fmt.Sprintf("insufficient scope: requires %s", requiredScope[0])
	}
	appErr := errors.NewAppError(errors.CodeInsufficientScope, errors.CategoryAuth, msg)
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondNotAuthorized handles general authorization errors by returning a 403 forbidden response
func RespondNotAuthorized(ctx *apptheory.Context, resource string) (*apptheory.Response, error) {
	return RespondForbidden(ctx, fmt.Sprintf("not authorized to access %s", resource))
}

// RespondNotAuthorizedToModify handles modification authorization errors by returning a 403 forbidden response
func RespondNotAuthorizedToModify(ctx *apptheory.Context, resource string) (*apptheory.Response, error) {
	return RespondForbidden(ctx, fmt.Sprintf("not authorized to modify %s", resource))
}

// RespondNotAuthorizedToDelete handles deletion authorization errors by returning a 403 forbidden response
func RespondNotAuthorizedToDelete(ctx *apptheory.Context, resource string) (*apptheory.Response, error) {
	return RespondForbidden(ctx, fmt.Sprintf("not authorized to delete %s", resource))
}

// RespondBadRequest handles validation and bad request errors (400) - now using centralized errors
func RespondBadRequest(_ *apptheory.Context, message ...string) (*apptheory.Response, error) {
	msg := "Bad Request"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	appErr := errors.BadRequest(msg)
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondValidationError handles validation failed errors - now using centralized errors
func RespondValidationError(_ *apptheory.Context, err error) (*apptheory.Response, error) {
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
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondMissingParameter handles missing required parameter errors by returning a 400 bad request response
func RespondMissingParameter(ctx *apptheory.Context, paramName string) (*apptheory.Response, error) {
	return RespondBadRequest(ctx, fmt.Sprintf("missing required parameter: %s", paramName))
}

// RespondInvalidParameter handles invalid parameter errors by returning a 400 bad request response
func RespondInvalidParameter(ctx *apptheory.Context, paramName string) (*apptheory.Response, error) {
	return RespondBadRequest(ctx, fmt.Sprintf("invalid parameter: %s", paramName))
}

// RespondMissingAccountID handles missing account ID errors by returning a 400 bad request response
func RespondMissingAccountID(ctx *apptheory.Context) (*apptheory.Response, error) {
	return RespondBadRequest(ctx, "missing account id")
}

// RespondMissingStatusID handles missing status ID errors by returning a 400 bad request response
func RespondMissingStatusID(ctx *apptheory.Context) (*apptheory.Response, error) {
	return RespondBadRequest(ctx, "missing status id")
}

// RespondInvalidRequest handles general invalid request errors by returning a 400 bad request response
func RespondInvalidRequest(ctx *apptheory.Context) (*apptheory.Response, error) {
	return RespondBadRequest(ctx, "invalid request")
}

// RespondNotFound handles resource not found errors (404) - now using centralized errors
func RespondNotFound(_ *apptheory.Context, resource ...string) (*apptheory.Response, error) {
	var appErr *errors.AppError
	if len(resource) > 0 && resource[0] != "" {
		appErr = errors.NotFound(resource[0])
	} else {
		appErr = errors.NotFound("resource")
	}
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondAccountNotFound handles account not found errors by returning a 404 not found response
func RespondAccountNotFound(ctx *apptheory.Context) (*apptheory.Response, error) {
	return RespondNotFound(ctx, "account")
}

// RespondStatusNotFound handles status not found errors by returning a 404 not found response
func RespondStatusNotFound(ctx *apptheory.Context) (*apptheory.Response, error) {
	return RespondNotFound(ctx, "status")
}

// RespondUserNotFound handles user not found errors by returning a 404 not found response
func RespondUserNotFound(ctx *apptheory.Context) (*apptheory.Response, error) {
	return RespondNotFound(ctx, "user")
}

// RespondActorNotFound handles actor not found errors by returning a 404 not found response
func RespondActorNotFound(ctx *apptheory.Context) (*apptheory.Response, error) {
	return RespondNotFound(ctx, "actor")
}

// RespondFilterNotFound handles filter not found errors by returning a 404 not found response
func RespondFilterNotFound(ctx *apptheory.Context) (*apptheory.Response, error) {
	return RespondNotFound(ctx, "filter")
}

// RespondConversationNotFound handles conversation not found errors by returning a 404 not found response
func RespondConversationNotFound(ctx *apptheory.Context) (*apptheory.Response, error) {
	return RespondNotFound(ctx, "conversation")
}

// RespondMethodNotAllowed handles HTTP method not allowed errors by returning a 405 response
func RespondMethodNotAllowed(_ *apptheory.Context) (*apptheory.Response, error) {
	appErr := errors.NewAppError(errors.CodeMethodNotAllowed, errors.CategoryAPI, "Method Not Allowed")
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondConflict handles resource conflict errors (409) - now using centralized errors
func RespondConflict(_ *apptheory.Context, message ...string) (*apptheory.Response, error) {
	msg := "Conflict"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	appErr := errors.NewAppError(errors.CodeConflict, errors.CategoryBusiness, msg)
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondAlreadyExists handles resource already exists conflicts by returning a 409 conflict response
func RespondAlreadyExists(ctx *apptheory.Context, resource string) (*apptheory.Response, error) {
	return RespondConflict(ctx, fmt.Sprintf("%s already exists", resource))
}

// RespondGone handles resource gone errors (410) with optional custom message
func RespondGone(_ *apptheory.Context, message ...string) (*apptheory.Response, error) {
	msg := "Gone"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	appErr := errors.NewAppError(errors.CodeGone, errors.CategoryAPI, msg)
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondUnprocessableEntity handles unprocessable entity errors (422) with optional custom message
func RespondUnprocessableEntity(_ *apptheory.Context, message ...string) (*apptheory.Response, error) {
	msg := "Unprocessable Entity"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	appErr := errors.NewAppError(errors.CodeUnprocessableEntity, errors.CategoryValidation, msg)
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondStatusTooLong handles status text too long errors by returning a 422 unprocessable entity response
func RespondStatusTooLong(ctx *apptheory.Context) (*apptheory.Response, error) {
	return RespondUnprocessableEntity(ctx, "status text too long")
}

// RespondInvalidContent handles invalid content errors by returning a 422 unprocessable entity response
func RespondInvalidContent(ctx *apptheory.Context) (*apptheory.Response, error) {
	return RespondUnprocessableEntity(ctx, "invalid content")
}

// RespondRateLimited handles rate limit exceeded errors by returning a 429 response
func RespondRateLimited(_ *apptheory.Context) (*apptheory.Response, error) {
	appErr := errors.NewAppError(errors.CodeRateLimited, errors.CategoryAPI, "Rate limit exceeded")
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondInternalServerError handles internal server errors (500) - now using centralized errors
func RespondInternalServerError(_ *apptheory.Context, message ...string) (*apptheory.Response, error) {
	msg := "Internal server error"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	appErr := errors.Internal(msg)
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondDatabaseError handles database errors by returning a 500 internal server error response
func RespondDatabaseError(ctx *apptheory.Context) (*apptheory.Response, error) {
	return RespondInternalServerError(ctx, "database error")
}

// RespondFailedToCreate handles resource creation failures by returning a 500 internal server error response
func RespondFailedToCreate(ctx *apptheory.Context, resource string) (*apptheory.Response, error) {
	return RespondInternalServerError(ctx, fmt.Sprintf("failed to create %s", resource))
}

// RespondFailedToUpdate handles resource update failures by returning a 500 internal server error response
func RespondFailedToUpdate(ctx *apptheory.Context, resource string) (*apptheory.Response, error) {
	return RespondInternalServerError(ctx, fmt.Sprintf("failed to update %s", resource))
}

// RespondFailedToDelete handles resource deletion failures by returning a 500 internal server error response
func RespondFailedToDelete(ctx *apptheory.Context, resource string) (*apptheory.Response, error) {
	return RespondInternalServerError(ctx, fmt.Sprintf("failed to delete %s", resource))
}

// RespondFailedToGet handles resource retrieval failures by returning a 500 internal server error response
func RespondFailedToGet(ctx *apptheory.Context, resource string) (*apptheory.Response, error) {
	return RespondInternalServerError(ctx, fmt.Sprintf("failed to get %s", resource))
}

// RespondServiceUnavailable handles service unavailable errors (503) with optional service name
func RespondServiceUnavailable(_ *apptheory.Context, service ...string) (*apptheory.Response, error) {
	msg := "Service Unavailable"
	if len(service) > 0 && service[0] != "" {
		msg = fmt.Sprintf("%s service unavailable", service[0])
	}
	appErr := errors.NewAppError(errors.CodeExternalServiceUnavailable, errors.CategoryExternal, msg)
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// Composite error response functions that handle common patterns

// RespondAuthErrorWithCode handles authentication/authorization errors with status code
func RespondAuthErrorWithCode(ctx *apptheory.Context, errorCode int, err error) (*apptheory.Response, error) {
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
func RespondValidationOrError(ctx *apptheory.Context, err error) (*apptheory.Response, error) {
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
func RespondCreateError(ctx *apptheory.Context, resource string, err error) (*apptheory.Response, error) {
	if err == nil {
		return RespondInternalServerError(ctx, fmt.Sprintf("unknown error creating %s", resource))
	}

	// Check for centralized AppError
	if appErr, ok := errors.AsAppError(err); ok {
		return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
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
func RespondUpdateError(ctx *apptheory.Context, resource string, err error) (*apptheory.Response, error) {
	if err == nil {
		return RespondInternalServerError(ctx, fmt.Sprintf("unknown error updating %s", resource))
	}

	// Check for centralized AppError
	if appErr, ok := errors.AsAppError(err); ok {
		return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
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
func RespondDeleteError(ctx *apptheory.Context, resource string, err error) (*apptheory.Response, error) {
	if err == nil {
		return RespondInternalServerError(ctx, fmt.Sprintf("unknown error deleting %s", resource))
	}

	// Check for centralized AppError
	if appErr, ok := errors.AsAppError(err); ok {
		return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
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
func RespondGetError(ctx *apptheory.Context, resource string, err error) (*apptheory.Response, error) {
	if err == nil {
		return RespondInternalServerError(ctx, fmt.Sprintf("unknown error getting %s", resource))
	}

	// Check for centralized AppError
	if appErr, ok := errors.AsAppError(err); ok {
		return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
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
func RespondWithAppError(_ *apptheory.Context, appErr *errors.AppError) (*apptheory.Response, error) {
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// RespondWithErrorMessage creates a standardized error response with custom message
func RespondWithErrorMessage(_ *apptheory.Context, statusCode int, message string) (*apptheory.Response, error) {
	return apptheory.JSON(statusCode, StandardErrorResponse{
		Error: message,
		Code:  errorCodeForHTTPStatus(statusCode),
	})
}

// RespondWithErrorAndDescription creates a detailed error response
func RespondWithErrorAndDescription(_ *apptheory.Context, statusCode int, errorMsg, description string) (*apptheory.Response, error) {
	return apptheory.JSON(statusCode, StandardErrorResponse{
		Error:       errorMsg,
		Description: description,
		Code:        errorCodeForHTTPStatus(statusCode),
	})
}

// RespondWithErrorCode creates an error response with an error code
func RespondWithErrorCode(_ *apptheory.Context, statusCode int, errorMsg, code string) (*apptheory.Response, error) {
	return apptheory.JSON(statusCode, StandardErrorResponse{
		Error: errorMsg,
		Code:  code,
	})
}

// Legacy compatibility functions for existing patterns
// These maintain backward compatibility while encouraging migration to new patterns

// RespondLegacyError maintains compatibility with existing map[string]string{"error": "message"} pattern
func RespondLegacyError(_ *apptheory.Context, statusCode int, message string) (*apptheory.Response, error) {
	return apptheory.JSON(statusCode, map[string]string{"error": message})
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
func RespondSuccess(_ *apptheory.Context, data interface{}) (*apptheory.Response, error) {
	return apptheory.JSON(200, data)
}
