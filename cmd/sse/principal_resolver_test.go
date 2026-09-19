package main

import (
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

func ssePrincipalContext(authorization string) *apptheory.Context {
	headers := map[string][]string{}
	if authorization != "" {
		headers["authorization"] = []string{authorization}
	}
	return &apptheory.Context{Request: apptheory.Request{Headers: headers}}
}

// TestResolveSSEPrincipal pins when the secure app treats a streaming request as
// already authenticated. The resolver is deliberately permissive: an absent,
// malformed, or rejected credential resolves to "no principal" with no error, so
// the streaming handlers keep ownership of their legacy 401 payloads. Reporting
// an error here would replace those bodies with the framework's own response.
func TestResolveSSEPrincipal(t *testing.T) {
	original := authService
	t.Cleanup(func() { authService = original })

	t.Run("an absent context or an unwired validator resolves nobody", func(t *testing.T) {
		authService = &fakeValidator{claims: &auth.EnhancedClaims{Username: "alice"}}
		principal, err := resolveSSEPrincipal(nil)
		require.NoError(t, err)
		require.Nil(t, principal)

		authService = nil
		principal, err = resolveSSEPrincipal(ssePrincipalContext("Bearer token"))
		require.NoError(t, err)
		require.Nil(t, principal)
	})

	t.Run("a credential that cannot be extracted resolves nobody", func(t *testing.T) {
		validator := &fakeValidator{claims: &auth.EnhancedClaims{Username: "alice"}}
		authService = validator

		for _, authorization := range []string{
			"",
			"Basic dXNlcjpwYXNz",
			"Bearer",
			"Bearer   ",
		} {
			principal, err := resolveSSEPrincipal(ssePrincipalContext(authorization))
			require.NoError(t, err)
			require.Nil(t, principal, "authorization %q must not resolve a principal", authorization)
		}
		require.Zero(t, validator.calls, "an unextractable credential must not reach the validator")
	})

	t.Run("a rejected token resolves nobody", func(t *testing.T) {
		validator := &fakeValidator{err: auth.ErrInvalidToken}
		authService = validator

		principal, err := resolveSSEPrincipal(ssePrincipalContext("Bearer revoked"))
		require.NoError(t, err)
		require.Nil(t, principal)
		require.Equal(t, 1, validator.calls)
	})

	t.Run("a validator that reports no claims resolves nobody", func(t *testing.T) {
		authService = &fakeValidator{}

		principal, err := resolveSSEPrincipal(ssePrincipalContext("Bearer token"))
		require.NoError(t, err)
		require.Nil(t, principal)
	})

	t.Run("a valid token resolves an external principal", func(t *testing.T) {
		authService = &fakeValidator{claims: &auth.EnhancedClaims{
			Username: "alice",
			Scopes:   []string{"read", "write"},
		}}

		principal, err := resolveSSEPrincipal(ssePrincipalContext("Bearer token"))
		require.NoError(t, err)
		require.NotNil(t, principal)
		require.Equal(t, apptheory.PrincipalExternal, principal.Kind)
		require.NotEmpty(t, principal.Identity)
	})
}

// TestResolveSSEPrincipalNeverFails keeps the resolver's contract with the
// framework honest: the route table depends on this function never turning a bad
// credential into an error, because an error would let the framework answer
// before the streaming handler's own authorization runs.
func TestResolveSSEPrincipalNeverFails(t *testing.T) {
	original := authService
	t.Cleanup(func() { authService = original })

	authService = &fakeValidator{err: errors.New("validator exploded")}
	principal, err := resolveSSEPrincipal(ssePrincipalContext("Bearer token"))
	require.NoError(t, err)
	require.Nil(t, principal)
}
