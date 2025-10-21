package lift

import (
	"context"
	"testing"

	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/require"
)

// Ensures unmatched routes result in a 404 LiftError response end-to-end
func TestHandleRequest_UnmatchedRouteReturns404(t *testing.T) {
	app := New()

	// No routes registered matching /notfound
	event := map[string]any{
		"version":         "2.0",
		"routeKey":        "GET /notfound",
		"rawPath":         "/notfound",
		"rawQueryString":  "",
		"requestContext":  map[string]any{"http": map[string]any{"method": "GET", "path": "/notfound"}},
		"headers":         map[string]any{},
		"isBase64Encoded": false,
		"body":            "",
	}

	respAny, err := app.HandleRequest(context.Background(), event)
	require.NoError(t, err)

	// Response is marshaled via Response.MarshalJSON by the Lambda runtime,
	// but internally it's a *Response. Convert using adapters.Request for simplicity
	// and assert status code.
	resp, ok := respAny.(*Response)
	if !ok {
		// Some tests use map[string]any; in that case, extract statusCode
		if respMap, ok2 := respAny.(map[string]any); ok2 {
			if status, ok3 := respMap["statusCode"].(int); ok3 {
				require.Equal(t, 404, status)
				return
			}
		}
		t.Fatalf("unexpected response type: %T", respAny)
	}

	require.Equal(t, 404, resp.StatusCode)
}

// Ensures router returns a LiftError(NotFound) for unmatched routes
func TestRouterHandle_UnmatchedReturnsLiftNotFound(t *testing.T) {
	router := NewRouter()

	req := NewRequest(&adapters.Request{
		Method:      "GET",
		Path:        "/missing",
		Headers:     map[string]string{},
		QueryParams: map[string]string{},
		TriggerType: TriggerAPIGateway,
	})
	ctx := NewContext(context.Background(), req)

	err := router.Handle(ctx)
	require.Error(t, err)
	if le, ok := err.(*LiftError); ok {
		require.Equal(t, 404, le.StatusCode)
		require.Equal(t, "NOT_FOUND", le.Code)
	} else {
		t.Fatalf("expected LiftError, got %T", err)
	}
}
