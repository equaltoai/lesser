package auth

import (
	"context"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthContext_GetSetAndScopeChecks(t *testing.T) {
	liftCtx := lift.NewContext(context.Background(), lift.NewRequest(nil))

	unauth := GetAuthContext(liftCtx)
	require.NotNil(t, unauth)
	assert.False(t, unauth.Authenticated)

	now := time.Now()
	authCtx := CreateAuthContext("alice", "web", "Bearer", []string{"read"}, now, now.Add(time.Hour))
	SetAuthContext(liftCtx, authCtx)

	got := GetAuthContext(liftCtx)
	require.True(t, got.Authenticated)
	assert.Equal(t, "alice", got.Username)
	assert.True(t, got.HasScope("read"))
	assert.False(t, got.HasScope("write"))
	assert.False(t, got.IsAdmin())
}

func TestAuthContext_RequireAuthAndScope(t *testing.T) {
	authCtx := CreateUnauthenticatedContext()
	require.Error(t, authCtx.RequireAuth())
	require.Error(t, authCtx.RequireScope("read"))

	now := time.Now()
	authCtx = CreateAuthContext("alice", "web", "Bearer", []string{"read"}, now, now.Add(time.Hour))
	require.NoError(t, authCtx.RequireAuth())
	require.NoError(t, authCtx.RequireScope("read"))
	require.Error(t, authCtx.RequireScope("write"))

	adminCtx := CreateAuthContext("root", "web", "Bearer", []string{"admin"}, now, now.Add(time.Hour))
	require.NoError(t, adminCtx.RequireScope("anything"))
	require.True(t, adminCtx.IsAdmin())
}

func TestAuthContext_ResponseHelpers(t *testing.T) {
	liftCtx := lift.NewContext(context.Background(), lift.NewRequest(nil))

	unauth := CreateUnauthenticatedContext()
	require.NoError(t, unauth.RequireAuthWithResponse(liftCtx))
	assert.Equal(t, 401, liftCtx.Response.StatusCode)

	liftCtx = lift.NewContext(context.Background(), lift.NewRequest(nil))
	now := time.Now()
	authCtx := CreateAuthContext("alice", "web", "Bearer", []string{"read"}, now, now.Add(time.Hour))
	require.NoError(t, authCtx.RequireScopeWithResponse(liftCtx, "write"))
	assert.Equal(t, 403, liftCtx.Response.StatusCode)

	liftCtx = lift.NewContext(context.Background(), lift.NewRequest(nil))
	require.NoError(t, authCtx.RequireScopeWithResponse(liftCtx, "read"))
}

func TestAuthContext_ExpiryHelpersAndToMap(t *testing.T) {
	now := time.Now()
	authCtx := CreateAuthContext("alice", "web", "Bearer", []string{"read"}, now, now.Add(25*time.Millisecond))
	assert.False(t, authCtx.IsExpired())
	assert.Greater(t, authCtx.TimeUntilExpiry(), time.Duration(0))

	m := authCtx.ToMap()
	assert.Equal(t, true, m["authenticated"])
	assert.Equal(t, "alice", m["username"])

	expired := CreateAuthContext("alice", "web", "Bearer", []string{"read"}, now.Add(-time.Hour), now.Add(-time.Minute))
	assert.True(t, expired.IsExpired())
	assert.Equal(t, time.Duration(0), CreateUnauthenticatedContext().TimeUntilExpiry())
}
