package lift_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pay-theory/lift/pkg/lift"
)

func TestHandleRequestRejectsOversizedBody(t *testing.T) {
	app := lift.New(lift.WithConfig(&lift.Config{MaxRequestSize: 32}))

	require.NoError(t, app.POST("/upload", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"status": "accepted"})
	}))

	event := newAPIGatewayV2Event("POST", "/upload")
	event["body"] = strings.Repeat("a", 64)
	event["isBase64Encoded"] = false

	respAny, err := app.HandleRequest(context.Background(), event)
	require.NoError(t, err)

	resp := extractResponse(t, respAny)
	require.Equal(t, 413, resp.StatusCode)

	bodyJSON := parseResponseBody(t, resp.Body)
	require.Equal(t, lift.ErrorCodePayloadTooLarge, bodyJSON["code"])
}

func TestHandleRequestRejectsOversizedResponse(t *testing.T) {
	app := lift.New(lift.WithConfig(&lift.Config{MaxResponseSize: 16 * 3}))

	require.NoError(t, app.GET("/large", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"body": strings.Repeat("x", 128)})
	}))

	event := newAPIGatewayV2Event("GET", "/large")

	respAny, err := app.HandleRequest(context.Background(), event)
	require.NoError(t, err)

	resp := extractResponse(t, respAny)
	require.Equal(t, 413, resp.StatusCode)
	bodyJSON := parseResponseBody(t, resp.Body)
	require.Equal(t, lift.ErrorCodePayloadTooLarge, bodyJSON["code"])
}

func TestHandleRequestRequiresTenant(t *testing.T) {
	app := lift.New(lift.WithConfig(&lift.Config{RequireTenantID: true}))

	require.NoError(t, app.GET("/tenant", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"status": "ok"})
	}))

	ctx := context.Background()

	// Request without tenant header should fail
	respAny, err := app.HandleRequest(ctx, newAPIGatewayV2Event("GET", "/tenant"))
	require.NoError(t, err)
	resp := extractResponse(t, respAny)
	require.Equal(t, 400, resp.StatusCode)
	bodyJSON := parseResponseBody(t, resp.Body)
	require.Equal(t, lift.ErrorCodeTenantRequired, bodyJSON["code"])

	// Request with tenant header should pass
	eventWithTenant := newAPIGatewayV2Event("GET", "/tenant")
	headers := eventWithTenant["headers"].(map[string]any)
	headers["x-tenant-id"] = "tenant-123"

	respAny, err = app.HandleRequest(context.Background(), eventWithTenant)
	require.NoError(t, err)
	resp = extractResponse(t, respAny)
	require.Equal(t, 200, resp.StatusCode)
}

func newAPIGatewayV2Event(method, path string) map[string]any {
	return map[string]any{
		"version":  "2.0",
		"routeKey": method + " " + path,
		"rawPath":  path,
		"requestContext": map[string]any{
			"http": map[string]any{
				"method": method,
				"path":   path,
			},
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
		statusAny := v["statusCode"]
		status := 0
		if statusFloat, ok := statusAny.(float64); ok {
			status = int(statusFloat)
		}
		response := lift.NewResponse()
		response.Status(status)
		response.Body = v["body"]
		return response
	default:
		require.FailNow(t, "unexpected response type", "got %T", respAny)
	}
	return nil
}

func parseResponseBody(t *testing.T, body any) map[string]any {
	var bodyBytes []byte
	switch v := body.(type) {
	case string:
		bodyBytes = []byte(v)
	case []byte:
		bodyBytes = v
	default:
		encoded, err := json.Marshal(v)
		require.NoError(t, err)
		bodyBytes = encoded
	}

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(bodyBytes, &parsed))
	return parsed
}
