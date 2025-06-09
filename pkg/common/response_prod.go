//go:build production
// +build production

package common

import (
	"encoding/json"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// ErrorResponseWithCode creates an error response with a specific code
func ErrorResponseWithCode(statusCode int, code string, err error) *events.APIGatewayV2HTTPResponse {
	// Log the actual error
	Logger().Error("API error",
		zap.String("code", code),
		zap.Int("status", statusCode),
		zap.Error(err))

	errorResp := ErrorResponse{
		Error:   http.StatusText(statusCode),
		Message: http.StatusText(statusCode), // Generic message for production
		Code:    code,
	}

	body, _ := json.Marshal(errorResp)

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: statusCode,
		Headers:    Headers(),
		Body:       string(body),
	}
}
