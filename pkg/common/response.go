//go:build !production
// +build !production

// Package common provides shared utilities for the Lesser application.
// revive:disable-next-line var-naming // legacy package name used across public APIs
package common

import (
	"encoding/json"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/errors"
	"go.uber.org/zap"
)

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
}

// Headers returns common headers for responses
func Headers() map[string]string {
	return map[string]string{
		"Content-Type":                 "application/json",
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS, PATCH, HEAD",
		"Access-Control-Allow-Headers": "Content-Type, Authorization, X-Requested-With, Accept, Accept-Encoding, Accept-Language, Date, Digest, Host, Signature, User-Agent, X-Forwarded-For, X-Forwarded-Proto",
	}
}

// ActivityPubHeaders returns headers for ActivityPub responses
func ActivityPubHeaders() map[string]string {
	headers := Headers()
	headers["Content-Type"] = "application/activity+json"
	return headers
}

// JSONResponse creates a successful JSON response
func JSONResponse(statusCode int, body any) *events.APIGatewayV2HTTPResponse {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		Logger().Error("failed to marshal response body", zap.Error(err))
		return InternalServerError(err)
	}

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: statusCode,
		Headers:    Headers(),
		Body:       string(jsonBody),
	}
}

// ActivityPubResponse creates a successful ActivityPub response
func ActivityPubResponse(statusCode int, body any) *events.APIGatewayV2HTTPResponse {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		Logger().Error("failed to marshal ActivityPub response", zap.Error(err))
		return InternalServerError(err)
	}

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: statusCode,
		Headers:    ActivityPubHeaders(),
		Body:       string(jsonBody),
	}
}

// ErrorResponseWithCode creates an error response with a specific code
func ErrorResponseWithCode(statusCode int, code string, err error) *events.APIGatewayV2HTTPResponse {
	errorResp := ErrorResponse{
		Error:   http.StatusText(statusCode),
		Message: err.Error(),
		Code:    code,
	}

	body, _ := json.Marshal(errorResp)

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: statusCode,
		Headers:    Headers(),
		Body:       string(body),
	}
}

// Error response helpers

// BadRequest returns a 400 Bad Request response
func BadRequest(err error) *events.APIGatewayV2HTTPResponse {
	return ErrorResponseWithCode(http.StatusBadRequest, "BAD_REQUEST", err)
}

// Unauthorized returns a 401 Unauthorized response
func Unauthorized(err error) *events.APIGatewayV2HTTPResponse {
	return ErrorResponseWithCode(http.StatusUnauthorized, "UNAUTHORIZED", err)
}

// Forbidden returns a 403 Forbidden response
func Forbidden(err error) *events.APIGatewayV2HTTPResponse {
	return ErrorResponseWithCode(http.StatusForbidden, "FORBIDDEN", err)
}

// NotFound returns a 404 Not Found response
func NotFound(err error) *events.APIGatewayV2HTTPResponse {
	return ErrorResponseWithCode(http.StatusNotFound, "NOT_FOUND", err)
}

// MethodNotAllowed returns a 405 error response
func MethodNotAllowed(err error) *events.APIGatewayV2HTTPResponse {
	return ErrorResponseWithCode(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", err)
}

// Conflict returns a 409 Conflict response
func Conflict(err error) *events.APIGatewayV2HTTPResponse {
	return ErrorResponseWithCode(http.StatusConflict, "CONFLICT", err)
}

// UnprocessableEntity returns a 422 Unprocessable Entity response
func UnprocessableEntity(err error) *events.APIGatewayV2HTTPResponse {
	return ErrorResponseWithCode(http.StatusUnprocessableEntity, "VALIDATION_ERROR", err)
}

// TooManyRequests returns a 429 Too Many Requests response
func TooManyRequests(err error) *events.APIGatewayV2HTTPResponse {
	return ErrorResponseWithCode(http.StatusTooManyRequests, "RATE_LIMITED", err)
}

// InternalServerError returns a 500 Internal Server Error response
func InternalServerError(err error) *events.APIGatewayV2HTTPResponse {
	// Log the actual error but don't expose internal details
	Logger().Error("internal server error", zap.Error(err))

	genericErr := ErrorResponse{
		Error:   http.StatusText(http.StatusInternalServerError),
		Message: "An internal error occurred",
		Code:    "INTERNAL_ERROR",
	}

	body, _ := json.Marshal(genericErr)

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusInternalServerError,
		Headers:    Headers(),
		Body:       string(body),
	}
}

// ErrorFromType returns an appropriate error response based on error type
func ErrorFromType(err error) *events.APIGatewayV2HTTPResponse {
	if appErr, ok := errors.AsAppError(err); ok {
		return ErrorResponseWithCode(appErr.HTTPStatusCode, string(appErr.Code), err)
	}

	switch {
	case IsNotFound(err):
		return NotFound(err)
	case IsValidation(err):
		return UnprocessableEntity(err)
	case IsAuthentication(err):
		return Unauthorized(err)
	case IsAuthorization(err):
		return Forbidden(err)
	case IsConflict(err):
		return Conflict(err)
	case IsFederation(err):
		// Federation errors might be various status codes
		// Default to bad gateway for remote server issues
		return ErrorResponseWithCode(http.StatusBadGateway, "FEDERATION_ERROR", err)
	default:
		return InternalServerError(err)
	}
}

// Success response helpers

// OK returns a 200 OK response
func OK(body any) *events.APIGatewayV2HTTPResponse {
	return JSONResponse(http.StatusOK, body)
}

// Created returns a 201 Created response
func Created(body any) *events.APIGatewayV2HTTPResponse {
	return JSONResponse(http.StatusCreated, body)
}

// Accepted returns a 202 Accepted response
func Accepted(body any) *events.APIGatewayV2HTTPResponse {
	return JSONResponse(http.StatusAccepted, body)
}

// NoContent returns a 204 No Content response
func NoContent() *events.APIGatewayV2HTTPResponse {
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusNoContent,
		Headers:    Headers(),
	}
}
