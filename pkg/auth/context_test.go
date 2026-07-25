package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
)

func TestAuthContext_GetSetAndScopeChecks(t *testing.T) {
	ctx := &apptheory.Context{}

	unauth := GetAuthContext(ctx)
	require.NotNil(t, unauth)
	assert.False(t, unauth.Authenticated)

	now := time.Now()
	authCtx := CreateAuthContext("alice", "web", "Bearer", []string{"read"}, now, now.Add(time.Hour))
	SetAuthContext(ctx, authCtx)

	got := GetAuthContext(ctx)
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
	ctx := &apptheory.Context{}

	unauth := CreateUnauthenticatedContext()
	resp, err := unauth.RequireAuthWithResponse(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 401, resp.Status)

	ctx = &apptheory.Context{}
	now := time.Now()
	authCtx := CreateAuthContext("alice", "web", "Bearer", []string{"read"}, now, now.Add(time.Hour))
	resp, err = authCtx.RequireScopeWithResponse(ctx, "write")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 403, resp.Status)

	ctx = &apptheory.Context{}
	resp, err = authCtx.RequireScopeWithResponse(ctx, "read")
	require.NoError(t, err)
	assert.Nil(t, resp)
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
