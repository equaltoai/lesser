package common

import (
	"fmt"

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

// Authentication Errors (401)
func RespondUnauthorized(ctx *lift.Context, message ...string) error {
	msg := "Unauthorized"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	return ctx.Status(401).JSON(StandardErrorResponse{Error: msg})
}

func RespondUnauthorizedWithDescription(ctx *lift.Context, description string) error {
	return ctx.Status(401).JSON(StandardErrorResponse{
		Error:       "Unauthorized",
		Description: description,
	})
}

// Common unauthorized variants found in codebase
func RespondMissingAuth(ctx *lift.Context) error {
	return RespondUnauthorized(ctx, "authentication required")
}

func RespondInvalidToken(ctx *lift.Context) error {
	return RespondUnauthorized(ctx, "invalid token")
}

func RespondExpiredToken(ctx *lift.Context) error {
	return RespondUnauthorized(ctx, "token expired")
}

// Authorization/Permission Errors (403)
func RespondForbidden(ctx *lift.Context, message ...string) error {
	msg := "Forbidden"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	return ctx.Status(403).JSON(StandardErrorResponse{Error: msg})
}

// Common forbidden variants found in codebase
func RespondInsufficientScope(ctx *lift.Context, requiredScope ...string) error {
	msg := "insufficient scope"
	if len(requiredScope) > 0 {
		msg = fmt.Sprintf("insufficient scope: requires %s", requiredScope[0])
	}
	return RespondForbidden(ctx, msg)
}

func RespondNotAuthorized(ctx *lift.Context, resource string) error {
	return RespondForbidden(ctx, fmt.Sprintf("not authorized to access %s", resource))
}

func RespondNotAuthorizedToModify(ctx *lift.Context, resource string) error {
	return RespondForbidden(ctx, fmt.Sprintf("not authorized to modify %s", resource))
}

func RespondNotAuthorizedToDelete(ctx *lift.Context, resource string) error {
	return RespondForbidden(ctx, fmt.Sprintf("not authorized to delete %s", resource))
}

// Validation/Bad Request Errors (400)
func RespondBadRequest(ctx *lift.Context, message ...string) error {
	msg := "Bad Request"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	return ctx.Status(400).JSON(StandardErrorResponse{Error: msg})
}

func RespondValidationError(ctx *lift.Context, err error) error {
	msg := "Validation failed"
	if err != nil {
		msg = err.Error()
	}
	return ctx.Status(400).JSON(StandardErrorResponse{Error: msg})
}

// Common bad request variants found in codebase
func RespondMissingParameter(ctx *lift.Context, paramName string) error {
	return RespondBadRequest(ctx, fmt.Sprintf("missing required parameter: %s", paramName))
}

func RespondInvalidParameter(ctx *lift.Context, paramName string) error {
	return RespondBadRequest(ctx, fmt.Sprintf("invalid parameter: %s", paramName))
}

func RespondMissingAccountID(ctx *lift.Context) error {
	return RespondBadRequest(ctx, "missing account id")
}

func RespondMissingStatusID(ctx *lift.Context) error {
	return RespondBadRequest(ctx, "missing status id")
}

func RespondInvalidRequest(ctx *lift.Context) error {
	return RespondBadRequest(ctx, "invalid request")
}

// Resource Not Found Errors (404)
func RespondNotFound(ctx *lift.Context, resource ...string) error {
	msg := "Not Found"
	if len(resource) > 0 && resource[0] != "" {
		msg = fmt.Sprintf("%s not found", resource[0])
	}
	return ctx.Status(404).JSON(StandardErrorResponse{Error: msg})
}

// Common not found variants found in codebase
func RespondAccountNotFound(ctx *lift.Context) error {
	return RespondNotFound(ctx, "account")
}

func RespondStatusNotFound(ctx *lift.Context) error {
	return RespondNotFound(ctx, "status")
}

func RespondUserNotFound(ctx *lift.Context) error {
	return RespondNotFound(ctx, "user")
}

func RespondActorNotFound(ctx *lift.Context) error {
	return RespondNotFound(ctx, "actor")
}

func RespondFilterNotFound(ctx *lift.Context) error {
	return RespondNotFound(ctx, "filter")
}

func RespondConversationNotFound(ctx *lift.Context) error {
	return RespondNotFound(ctx, "conversation")
}

// Method Not Allowed (405)
func RespondMethodNotAllowed(ctx *lift.Context) error {
	return ctx.Status(405).JSON(StandardErrorResponse{Error: "Method Not Allowed"})
}

// Conflict Errors (409)
func RespondConflict(ctx *lift.Context, message ...string) error {
	msg := "Conflict"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	return ctx.Status(409).JSON(StandardErrorResponse{Error: msg})
}

func RespondAlreadyExists(ctx *lift.Context, resource string) error {
	return RespondConflict(ctx, fmt.Sprintf("%s already exists", resource))
}

// Gone Errors (410)
func RespondGone(ctx *lift.Context, message ...string) error {
	msg := "Gone"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	return ctx.Status(410).JSON(StandardErrorResponse{Error: msg})
}

// Unprocessable Entity (422)
func RespondUnprocessableEntity(ctx *lift.Context, message ...string) error {
	msg := "Unprocessable Entity"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	return ctx.Status(422).JSON(StandardErrorResponse{Error: msg})
}

// Common 422 variants found in codebase
func RespondStatusTooLong(ctx *lift.Context) error {
	return RespondUnprocessableEntity(ctx, "status text too long")
}

func RespondInvalidContent(ctx *lift.Context) error {
	return RespondUnprocessableEntity(ctx, "invalid content")
}

// Rate Limiting (429)
func RespondRateLimited(ctx *lift.Context) error {
	return ctx.Status(429).JSON(StandardErrorResponse{Error: "Rate limit exceeded"})
}

// Server Errors (500)
func RespondInternalServerError(ctx *lift.Context, message ...string) error {
	msg := "Internal server error"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	return ctx.Status(500).JSON(StandardErrorResponse{Error: msg})
}

// Common server error variants found in codebase
func RespondDatabaseError(ctx *lift.Context) error {
	return RespondInternalServerError(ctx, "database error")
}

func RespondFailedToCreate(ctx *lift.Context, resource string) error {
	return RespondInternalServerError(ctx, fmt.Sprintf("failed to create %s", resource))
}

func RespondFailedToUpdate(ctx *lift.Context, resource string) error {
	return RespondInternalServerError(ctx, fmt.Sprintf("failed to update %s", resource))
}

func RespondFailedToDelete(ctx *lift.Context, resource string) error {
	return RespondInternalServerError(ctx, fmt.Sprintf("failed to delete %s", resource))
}

func RespondFailedToGet(ctx *lift.Context, resource string) error {
	return RespondInternalServerError(ctx, fmt.Sprintf("failed to get %s", resource))
}

// Service Unavailable (503)
func RespondServiceUnavailable(ctx *lift.Context, service ...string) error {
	msg := "Service Unavailable"
	if len(service) > 0 && service[0] != "" {
		msg = fmt.Sprintf("%s service unavailable", service[0])
	}
	return ctx.Status(503).JSON(StandardErrorResponse{Error: msg})
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
		if len(errMsg) > 0 && errMsg[0:min(len(keyword), len(errMsg))] == keyword {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Common error response patterns for specific operations

// RespondCreateError handles errors from create operations
func RespondCreateError(ctx *lift.Context, resource string, err error) error {
	if err == nil {
		return RespondInternalServerError(ctx, fmt.Sprintf("unknown error creating %s", resource))
	}

	// Check for validation errors
	if err != nil && containsValidationKeywords(err.Error()) {
		return RespondValidationError(ctx, err)
	}

	// Check for conflict errors
	if isConflictError(err) {
		return RespondAlreadyExists(ctx, resource)
	}

	// Default to server error
	return RespondFailedToCreate(ctx, resource)
}

// RespondUpdateError handles errors from update operations
func RespondUpdateError(ctx *lift.Context, resource string, err error) error {
	if err == nil {
		return RespondInternalServerError(ctx, fmt.Sprintf("unknown error updating %s", resource))
	}

	// Check for validation errors
	if err != nil && containsValidationKeywords(err.Error()) {
		return RespondValidationError(ctx, err)
	}

	// Check for not found errors
	if isNotFoundError(err) {
		return RespondNotFound(ctx, resource)
	}

	// Default to server error
	return RespondFailedToUpdate(ctx, resource)
}

// RespondDeleteError handles errors from delete operations
func RespondDeleteError(ctx *lift.Context, resource string, err error) error {
	if err == nil {
		return RespondInternalServerError(ctx, fmt.Sprintf("unknown error deleting %s", resource))
	}

	// Check for not found errors
	if isNotFoundError(err) {
		return RespondNotFound(ctx, resource)
	}

	// Default to server error
	return RespondFailedToDelete(ctx, resource)
}

// RespondGetError handles errors from get/fetch operations
func RespondGetError(ctx *lift.Context, resource string, err error) error {
	if err == nil {
		return RespondInternalServerError(ctx, fmt.Sprintf("unknown error getting %s", resource))
	}

	// Check for not found errors
	if isNotFoundError(err) {
		return RespondNotFound(ctx, resource)
	}

	// Default to server error
	return RespondFailedToGet(ctx, resource)
}

// Error type checking helpers (these would need to be implemented based on actual error types)
func isConflictError(err error) bool {
	// Implementation would check for specific conflict error types
	// This is a placeholder for the actual logic
	return false
}

func isNotFoundError(err error) bool {
	// Implementation would check for specific not found error types
	// This is a placeholder for the actual logic
	return false
}

// Helper functions for common response patterns

// RespondWithErrorMessage creates a standardized error response with custom message
func RespondWithErrorMessage(ctx *lift.Context, statusCode int, message string) error {
	return ctx.Status(statusCode).JSON(StandardErrorResponse{Error: message})
}

// RespondWithErrorAndDescription creates a detailed error response
func RespondWithErrorAndDescription(ctx *lift.Context, statusCode int, error, description string) error {
	return ctx.Status(statusCode).JSON(StandardErrorResponse{
		Error:       error,
		Description: description,
	})
}

// RespondWithErrorCode creates an error response with an error code
func RespondWithErrorCode(ctx *lift.Context, statusCode int, error, code string) error {
	return ctx.Status(statusCode).JSON(StandardErrorResponse{
		Error: error,
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
	ErrorMissingAccountID   = "missing account id"
	ErrorMissingStatusID    = "missing status id"
	ErrorMissingParameter   = "missing required parameter"
	ErrorInvalidParameter   = "invalid parameter"
	ErrorInvalidRequest     = "invalid request"
	ErrorInvalidFormat      = "invalid format"

	// 401 Unauthorized variants
	ErrorUnauthorized       = "Unauthorized"
	ErrorInvalidToken       = "invalid token"
	ErrorMissingAuth        = "authentication required"
	ErrorExpiredToken       = "token expired"

	// 403 Forbidden variants
	ErrorInsufficientScope  = "insufficient scope"
	ErrorNotAuthorized      = "not authorized"
	ErrorAccessDenied       = "access denied"

	// 404 Not Found variants
	ErrorNotFound           = "not found"
	ErrorAccountNotFound    = "account not found"
	ErrorStatusNotFound     = "status not found"
	ErrorUserNotFound       = "user not found"
	ErrorActorNotFound      = "actor not found"

	// 422 Unprocessable Entity variants
	ErrorStatusTooLong      = "status text too long"
	ErrorInvalidContent     = "invalid content"

	// 500 Internal Server Error variants
	ErrorInternalServer     = "internal server error"
	ErrorDatabaseError      = "database error"
	ErrorFailedToCreate     = "failed to create"
	ErrorFailedToUpdate     = "failed to update"
	ErrorFailedToDelete     = "failed to delete"
	ErrorFailedToGet        = "failed to get"
)