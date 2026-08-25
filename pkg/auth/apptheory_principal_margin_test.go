package auth

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

func TestSecurePrincipalFactoriesAndBridgeEdgePaths(t *testing.T) {
	authResolver := NewAppTheorySecurePrincipalResolverFromAuthService(&AuthService{}, zap.NewNop(), "api")
	require.NotNil(t, authResolver)
	principal, err := authResolver(newTestContext("GET", "/api/v1/accounts", withHeaders(map[string]string{"Authorization": "Bearer invalid"})))
	require.NoError(t, err)
	require.Nil(t, principal)
	require.NotNil(t, NewAppTheorySecurePrincipalResolverFromAuthAndOAuthServices(&AuthService{}, nil, zap.NewNop(), "api"))

	want := errors.New("principal resolution failed")
	resolver := newAppTheoryPrincipalResolver(func(string) (*Claims, error) { return nil, want }, zap.NewNop(), "api")
	ctx := newTestContext("GET", "/api/v1/accounts", withHeaders(map[string]string{"Authorization": "Bearer token"}))
	response, err := createPrincipalContextBridge(resolver)(func(*apptheory.Context) (*apptheory.Response, error) {
		return &apptheory.Response{Status: 204}, nil
	})(ctx)
	require.Equal(t, 204, response.Status)
	require.NoError(t, err, "resolver intentionally downgrades invalid bearer validation to anonymous")

	var nilResolver *appTheoryPrincipalResolver
	nilResolver.logDebug(ctx, "ignored")
	nilResolver.logWarn(ctx, "ignored")
	populateLegacyAuthContext(nil)
	emptyPrincipalContext := newTestContext("GET", "/")
	emptyPrincipalContext.AuthPrincipal = &apptheory.AuthPrincipal{Claims: map[string]any{}}
	populateLegacyAuthContext(emptyPrincipalContext)
	require.Equal(t, false, emptyPrincipalContext.Get("is_authenticated"))
	require.Nil(t, claimsFromPrincipal(nil))

	richPrincipal := &apptheory.AuthPrincipal{Identity: "alice", Claims: map[string]any{
		"delegation_principal":     "owner",
		"delegation_agent":         "alice",
		"delegation_content_class": DelegationContentClassNote,
	}}
	claims := claimsFromPrincipal(richPrincipal)
	require.Equal(t, "owner", claims.DelegationPrincipal)
	require.Equal(t, "alice", claims.DelegationAgent)
	require.Equal(t, DelegationContentClassNote, claims.DelegationContentClass)
}
