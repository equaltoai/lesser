package lambda

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestStandardAuthPattern_CreateAuthMiddleware_AllowsAnonymous(t *testing.T) {
	sap := NewStandardAuthPattern(&common.LambdaContext{
		Config: &config.Config{JWTSecret: "secret"},
		Logger: zap.NewNop(),
	})

	mw := sap.CreateAuthMiddleware(AuthConfig{AllowAnonymous: true})

	ctx := newAuthTestContext(nil)

	nextCalled := false
	handler := mw(liftPkg.HandlerFunc(func(*liftPkg.Context) error {
		nextCalled = true
		return nil
	}))

	require.NoError(t, handler.Handle(ctx))
	require.True(t, nextCalled)
}

func TestStandardAuthPattern_CreateAuthMiddleware_StopsOnAuthError(t *testing.T) {
	sap := NewStandardAuthPattern(&common.LambdaContext{
		Config: &config.Config{JWTSecret: "secret"},
		Logger: zap.NewNop(),
	})

	mw := sap.CreateAuthMiddleware(AuthConfig{AllowAnonymous: false})

	ctx := newAuthTestContext(nil)

	nextCalled := false
	handler := mw(liftPkg.HandlerFunc(func(*liftPkg.Context) error {
		nextCalled = true
		return nil
	}))

	err := handler.Handle(ctx)
	require.Error(t, err)
	require.False(t, nextCalled)

	appErr, ok := apperrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, apperrors.CodeUnauthorized, appErr.Code)
}

func TestAuthConfigFactories(t *testing.T) {
	readOnly := ReadOnlyAuthConfig()
	require.Equal(t, []string{auth.ScopeRead}, readOnly.RequiredScopes)
	require.True(t, readOnly.RequireUser)
	require.False(t, readOnly.RequireAdmin)
	require.False(t, readOnly.AllowAnonymous)

	write := WriteAccessAuthConfig()
	require.Equal(t, []string{auth.ScopeWrite}, write.RequiredScopes)
	require.True(t, write.RequireUser)
	require.False(t, write.RequireAdmin)
	require.False(t, write.AllowAnonymous)

	admin := AdminOnlyAuthConfig()
	require.Equal(t, []string{auth.ScopeAdmin}, admin.RequiredScopes)
	require.True(t, admin.RequireUser)
	require.True(t, admin.RequireAdmin)
	require.False(t, admin.AllowAnonymous)

	public := PublicAccessAuthConfig()
	require.Empty(t, public.RequiredScopes)
	require.False(t, public.RequireUser)
	require.False(t, public.RequireAdmin)
	require.True(t, public.AllowAnonymous)

	federation := ActivityPubFederationAuthConfig()
	require.Empty(t, federation.RequiredScopes)
	require.False(t, federation.RequireUser)
	require.False(t, federation.RequireAdmin)
	require.True(t, federation.AllowAnonymous)
}
