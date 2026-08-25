package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

func TestPublicSurfaceMiddleware_ProtectedAPIUsesBearerContract(t *testing.T) {
	middleware := createPublicSurfaceMiddleware()
	handler := middleware(func(ctx *apptheory.Context) (*apptheory.Response, error) {
		return apptheory.Text(http.StatusOK, "ok"), nil
	})

	ctx := &apptheory.Context{
		Request: apptheory.Request{
			Method:  http.MethodGet,
			Path:    "/api/v1/accounts/verify_credentials",
			Headers: map[string][]string{},
		},
	}

	resp, err := handler(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.Status)

	var body common.BearerAuthErrorResponse
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, common.BearerErrorInvalidToken, body.Error)
	require.Equal(t, "authentication required", body.Description)
	require.Contains(t, resp.Headers["www-authenticate"][0], `error="invalid_token"`)
}
