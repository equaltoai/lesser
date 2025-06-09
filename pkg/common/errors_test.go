package common

import (
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap/zaptest"
)

func TestAppError(t *testing.T) {
	tests := []struct {
		name           string
		err            AppError
		expectedMsg    string
		expectedCode   string
		expectedStatus int
	}{
		{
			name:           "unauthorized error",
			err:            ErrUnauthorized(errors.New("invalid token details")),
			expectedMsg:    "Authentication failed",
			expectedCode:   "AUTH_FAILED",
			expectedStatus: 401,
		},
		{
			name:           "not found error",
			err:            ErrNotFound("user"),
			expectedMsg:    "Resource not found",
			expectedCode:   "NOT_FOUND",
			expectedStatus: 404,
		},
		{
			name:           "forbidden error",
			err:            ErrForbidden(errors.New("insufficient permissions")),
			expectedMsg:    "Access denied",
			expectedCode:   "FORBIDDEN",
			expectedStatus: 403,
		},
		{
			name:           "bad request error",
			err:            ErrBadRequest("Invalid input format", errors.New("json parse error")),
			expectedMsg:    "Invalid input format",
			expectedCode:   "BAD_REQUEST",
			expectedStatus: 400,
		},
		{
			name:           "internal error",
			err:            ErrInternal(errors.New("database connection failed")),
			expectedMsg:    "An error occurred processing your request",
			expectedCode:   "INTERNAL_ERROR",
			expectedStatus: 500,
		},
		{
			name:           "validation error",
			err:            ErrValidation("email", "Invalid email format"),
			expectedMsg:    "Invalid email format",
			expectedCode:   "VALIDATION_ERROR",
			expectedStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Error() method returns user message
			if tt.err.Error() != tt.expectedMsg {
				t.Errorf("Error() = %v, want %v", tt.err.Error(), tt.expectedMsg)
			}

			// Test fields
			if tt.err.Code != tt.expectedCode {
				t.Errorf("Code = %v, want %v", tt.err.Code, tt.expectedCode)
			}

			if tt.err.StatusCode != tt.expectedStatus {
				t.Errorf("StatusCode = %v, want %v", tt.err.StatusCode, tt.expectedStatus)
			}

			// Verify internal error is preserved
			if tt.err.InternalError == nil {
				t.Error("InternalError should not be nil")
			}
		})
	}
}

func TestHandleError(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedInBody string
		notInBody      string
	}{
		{
			name:           "app error returns safe message",
			err:            ErrUnauthorized(errors.New("token expired at 2024-01-01")),
			expectedStatus: 401,
			expectedInBody: "Authentication failed",
			notInBody:      "token expired",
		},
		{
			name:           "unknown error returns generic message",
			err:            errors.New("panic: runtime error: index out of range"),
			expectedStatus: 500,
			expectedInBody: "An error occurred processing your request",
			notInBody:      "panic",
		},
		{
			name:           "validation error includes user message",
			err:            ErrValidation("username", "Username must be at least 3 characters"),
			expectedStatus: 400,
			expectedInBody: "Username must be at least 3 characters",
			notInBody:      "validation failed for field",
		},
		{
			name:           "internal error hides details",
			err:            ErrInternal(errors.New("pq: SSL is not enabled on the server")),
			expectedStatus: 500,
			expectedInBody: "An error occurred processing your request",
			notInBody:      "pq: SSL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := HandleError(logger, tt.err)

			if status != tt.expectedStatus {
				t.Errorf("HandleError() status = %v, want %v", status, tt.expectedStatus)
			}

			if !strings.Contains(body, tt.expectedInBody) {
				t.Errorf("Response body should contain '%s', got '%s'", tt.expectedInBody, body)
			}

			if tt.notInBody != "" && strings.Contains(body, tt.notInBody) {
				t.Errorf("Response body should not contain '%s', got '%s'", tt.notInBody, body)
			}
		})
	}
}

func TestWrapError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		context    string
		expectType string
	}{
		{
			name:       "wrap app error preserves type",
			err:        ErrNotFound("user"),
			context:    "database query",
			expectType: "NOT_FOUND",
		},
		{
			name:       "wrap regular error creates internal error",
			err:        errors.New("connection refused"),
			context:    "redis connection",
			expectType: "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := WrapError(tt.err, tt.context)

			appErr, ok := wrapped.(AppError)
			if !ok {
				t.Fatal("WrapError should return AppError")
			}

			if appErr.Code != tt.expectType {
				t.Errorf("Expected error code %s, got %s", tt.expectType, appErr.Code)
			}

			// Check context is added to internal error
			if !strings.Contains(appErr.InternalError.Error(), tt.context) {
				t.Errorf("Internal error should contain context '%s'", tt.context)
			}
		})
	}
}

func TestErrorResponseFormat(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Test JSON format of error response
	err := ErrBadRequest("Invalid request format", errors.New("json unmarshal error"))
	_, body := HandleError(logger, err)

	// Should be valid JSON
	if !strings.HasPrefix(body, "{") || !strings.HasSuffix(body, "}") {
		t.Error("Response should be JSON object")
	}

	// Should contain error and code fields
	if !strings.Contains(body, `"error":`) {
		t.Error("Response should contain 'error' field")
	}

	if !strings.Contains(body, `"code":`) {
		t.Error("Response should contain 'code' field")
	}

	// Should not contain internal error details
	if strings.Contains(body, "json unmarshal") {
		t.Error("Response should not contain internal error details")
	}
}

func TestErrorPreventsInfoLeak(t *testing.T) {
	sensitiveErrors := []struct {
		name      string
		err       error
		sensitive string
	}{
		{
			name:      "database connection string",
			err:       errors.New("failed to connect to postgres://user:password@localhost/db"),
			sensitive: "password",
		},
		{
			name:      "file paths",
			err:       errors.New("open /home/user/secrets/key.pem: permission denied"),
			sensitive: "/home/user",
		},
		{
			name:      "stack traces",
			err:       errors.New("panic: runtime error at main.go:42"),
			sensitive: "main.go:42",
		},
		{
			name:      "AWS credentials",
			err:       errors.New("InvalidUserID.NotFound: AWS_ACCESS_KEY_REDACTED"),
			sensitive: "AKIA",
		},
	}

	logger := zaptest.NewLogger(t)

	for _, tt := range sensitiveErrors {
		t.Run(tt.name, func(t *testing.T) {
			// Wrap in internal error
			appErr := ErrInternal(tt.err)
			_, body := HandleError(logger, appErr)

			// Ensure sensitive info is not in response
			if strings.Contains(body, tt.sensitive) {
				t.Errorf("Response contains sensitive information '%s'", tt.sensitive)
			}

			// Should only contain generic message
			if !strings.Contains(body, "An error occurred processing your request") {
				t.Error("Response should contain generic error message")
			}
		})
	}
}
