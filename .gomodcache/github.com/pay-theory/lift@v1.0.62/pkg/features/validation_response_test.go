package features_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pay-theory/lift/pkg/features"
	"github.com/pay-theory/lift/pkg/lift"
)

func TestValidationMiddlewareResponseValidation(t *testing.T) {
	schema := features.NewSchema().AddProperty("message", features.ValidationRule{Type: "string", Required: true})

	middleware := features.NewValidationMiddleware(features.ValidationConfig{
		ResponseSchema:   schema,
		ValidateResponse: true,
		ErrorHandler: func(ctx *lift.Context, errs []features.ValidationError) error {
			return lift.NewLiftError(lift.ErrorCodeValidationError, "response validation failed", 400).
				WithDetails(map[string]any{"errors": errs})
		},
	})

	app := lift.New()
	app.Use(middleware.Validate())

	require.NoError(t, app.GET("/ok", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"message": "ok"})
	}))

	require.NoError(t, app.GET("/invalid", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"status": "missing"})
	}))

	okRespAny, err := app.HandleRequest(context.Background(), newAPIGatewayV2Event("GET", "/ok"))
	require.NoError(t, err)
	okResp := extractResponse(t, okRespAny)
	require.Equal(t, 200, okResp.StatusCode)

	invalidRespAny, err := app.HandleRequest(context.Background(), newAPIGatewayV2Event("GET", "/invalid"))
	require.NoError(t, err)
	invalidResp := extractResponse(t, invalidRespAny)
	require.Equal(t, 400, invalidResp.StatusCode)

	body := parseBody(t, invalidResp.Body)
	require.Equal(t, lift.ErrorCodeValidationError, body["code"])
}

func newAPIGatewayV2Event(method, path string) map[string]any {
	return map[string]any{
		"version":  "2.0",
		"routeKey": method + " " + path,
		"rawPath":  path,
		"requestContext": map[string]any{
			"http": map[string]any{"method": method, "path": path},
		},
		"headers":         map[string]any{},
		"isBase64Encoded": false,
	}
}

func extractResponse(t *testing.T, respAny any) *lift.Response {
	switch v := respAny.(type) {
	case *lift.Response:
		return v
	case map[string]any:
		response := lift.NewResponse()
		if status, ok := v["statusCode"].(float64); ok {
			response.Status(int(status))
		}
		response.Body = v["body"]
		return response
	default:
		require.FailNow(t, "unexpected response type", "got %T", respAny)
	}
	return nil
}

func parseBody(t *testing.T, body any) map[string]any {
	var data []byte
	switch v := body.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		encoded, err := json.Marshal(v)
		require.NoError(t, err)
		data = encoded
	}

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	return parsed
}
