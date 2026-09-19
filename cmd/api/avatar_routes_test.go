package main

import (
	"net/http"
	"testing"

	apiHandlers "github.com/equaltoai/lesser/cmd/api/handlers"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/auth/publicsurface"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap/zaptest"
)

const avatarRouteTestID = "550e8400-e29b-41d4-a716-446655440000"

func TestAvatarPublicSurfaceAllowlist(t *testing.T) {
	require.True(t, publicsurface.IsPublic(http.MethodGet, "/api/v1/avatars/"+avatarRouteTestID))
	require.True(t, publicsurface.IsPublic(http.MethodHead, "/api/v1/avatars/"+avatarRouteTestID))
	require.False(t, publicsurface.IsPublic(http.MethodGet, "/api/v1/avatars/"+avatarRouteTestID+"/extra"))
	require.False(t, publicsurface.IsPublic(http.MethodPost, "/api/v1/avatars/"+avatarRouteTestID))
	require.False(t, publicsurface.IsPublic(http.MethodPost, "/api/v1/accounts/avatar"))
	require.False(t, publicsurface.IsPublic(http.MethodDelete, "/api/v1/accounts/avatar"))
	require.False(t, publicsurface.IsPublic(http.MethodPatch, "/api/v1/accounts/avatar"))
}

func TestAvatarRoutesSurvivePublicSurfaceMiddleware(t *testing.T) {
	middleware := createPublicSurfaceMiddleware()
	handler := middleware(func(*apptheory.Context) (*apptheory.Response, error) {
		return apptheory.Text(http.StatusOK, "ok"), nil
	})

	serveCtx := &apptheory.Context{
		Request: apptheory.Request{
			Method:  http.MethodGet,
			Path:    "/api/v1/avatars/" + avatarRouteTestID,
			Headers: map[string][]string{},
		},
	}
	serveResp, err := handler(serveCtx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, serveResp.Status)

	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		uploadCtx := &apptheory.Context{
			Request: apptheory.Request{
				Method:  method,
				Path:    "/api/v1/accounts/avatar",
				Headers: map[string][]string{},
			},
		}
		resp, err := handler(uploadCtx)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.Status, method)
	}
}

func TestConfigureRoutes_AvatarPostures(t *testing.T) {
	logger = zaptest.NewLogger(t)
	apiHandler = &apiHandlers.Handler{}

	app := apptheory.NewSecure(apptheory.SecureOptions{Tier: apptheory.TierP2})
	configureRoutes(app)
	routes := configuredSecureRoutes(app)

	upload := routes["POST /api/v1/accounts/avatar"]
	require.Equal(t, apptheory.AuthPostureAuthenticated, upload.Posture)
	require.Equal(t, []string{auth.ScopeWrite}, upload.Scopes)

	clear := routes["DELETE /api/v1/accounts/avatar"]
	require.Equal(t, apptheory.AuthPostureAuthenticated, clear.Posture)
	require.Equal(t, []string{auth.ScopeWrite}, clear.Scopes)

	serve := routes["GET /api/v1/avatars/{id}"]
	require.Equal(t, apptheory.AuthPosturePublic, serve.Posture)
	require.Empty(t, serve.Scopes)
}
