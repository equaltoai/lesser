package main

import (
	"bufio"
	"errors"
	"os"
	"strings"
	"testing"

	apiHandlers "github.com/equaltoai/lesser/cmd/api/handlers"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap/zaptest"
)

func TestConfigureRoutes_PostureInventoryParity(t *testing.T) {
	logger = zaptest.NewLogger(t)
	apiHandler = &apiHandlers.Handler{}
	app := apptheory.NewSecure(apptheory.SecureOptions{Tier: apptheory.TierP2})
	configureRoutes(app)
	actual := configuredSecureRoutes(app)

	file, err := os.Open("testdata/secure_route_posture_inventory.tsv") // #nosec G304 -- committed test fixture.
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })

	scanner := bufio.NewScanner(file)
	require.True(t, scanner.Scan())
	require.Equal(t, "route\tsecure_posture\tsecure_scopes\tpre_classification\tlegacy_route_auth", scanner.Text())
	seen := make(map[string]struct{})
	for scanner.Scan() {
		columns := strings.Split(scanner.Text(), "\t")
		require.Len(t, columns, 5, scanner.Text())
		route, posture, scopes, preClassification, legacyAuth := columns[0], columns[1], columns[2], columns[3], columns[4]
		registered, ok := actual[route]
		require.True(t, ok, route)
		require.Equal(t, posture, string(registered.Posture), route)
		require.Equal(t, scopes, strings.Join(registered.Scopes, ","), route)

		expectedPosture := string(apptheory.AuthPosturePublic)
		switch {
		case preClassification == "contract_auth/internal_only":
			expectedPosture = string(apptheory.AuthPostureInternalOnly)
		case legacyAuth == "optionalAuth":
			expectedPosture = string(apptheory.AuthPostureOptional)
		case strings.HasPrefix(legacyAuth, "require"), preClassification == "auth_required":
			expectedPosture = string(apptheory.AuthPostureAuthenticated)
		}
		require.Equal(t, expectedPosture, posture, route+" changed its pre-migration public/authenticated split")
		seen[route] = struct{}{}
	}
	require.NoError(t, scanner.Err())
	require.Len(t, seen, len(actual), "every registered API route must have a pre-migration parity row")
}

func TestConfigureRoutes_SecurePostures(t *testing.T) {
	logger = zaptest.NewLogger(t)
	apiHandler = &apiHandlers.Handler{}

	app := apptheory.NewSecure(apptheory.SecureOptions{Tier: apptheory.TierP2})
	configureRoutes(app)
	routes := configuredSecureRoutes(app)

	require.Equal(t, apptheory.AuthPostureOptional, routes["POST /api/v1/apps"].Posture)
	require.Equal(t, apptheory.AuthPosturePublic, routes["POST /api/v1/auth/webauthn/signup/begin"].Posture)
	require.Equal(t, apptheory.AuthPostureAuthenticated, routes["GET /api/v1/accounts/verify_credentials"].Posture)
	require.Equal(t, []string{"read"}, routes["GET /api/v1/accounts/verify_credentials"].Scopes)
	require.Equal(t, apptheory.AuthPostureAuthenticated, routes["POST /api/v1/accounts/{id}/follow"].Posture)
	require.Empty(t, routes["POST /api/v1/accounts/{id}/follow"].Scopes) // any-of compatibility wrapper
	require.Equal(t, apptheory.AuthPostureOptional, routes["GET /api/v1/statuses/{id}"].Posture)
	require.Equal(t, apptheory.AuthPostureAuthenticated, routes["GET /api/v1/admin/accounts"].Posture)
	require.Equal(t, []string{"admin"}, routes["GET /api/v1/admin/agents/policy"].Scopes)
	require.Equal(t, apptheory.AuthPostureInternalOnly, routes["POST /api/v1/notifications/deliver"].Posture)
	require.Equal(t, apptheory.AuthPosturePublic, routes["POST /api/v1/agents/{username}/access-leases/{leaseID}/token"].Posture)
	require.Equal(t, []string{"read"}, routes["GET /api/v1/conversations/lookup"].Scopes)
	require.Equal(t, apptheory.AuthPostureOptional, routes["GET /api/v1/skills/catalog"].Posture)
}

func configuredSecureRoutes(app *apptheory.SecureApp) map[string]apptheory.SecureRoute {
	out := make(map[string]apptheory.SecureRoute)
	for _, route := range app.Routes() {
		out[route.Method+" "+route.Path] = route
	}
	return out
}

func TestRequireAnySecureScopePreservesAliasSemantics(t *testing.T) {
	called := false
	handler := requireAnySecureScope(func(*apptheory.Context) (*apptheory.Response, error) {
		called = true
		return &apptheory.Response{Status: 204}, nil
	}, "read:accounts", "read")

	ctx := &apptheory.Context{AuthPrincipal: &apptheory.AuthPrincipal{Identity: "alice", Scopes: []string{"read"}}}
	response, err := handler(ctx)
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, 204, response.Status)

	called = false
	ctx.AuthPrincipal.Scopes = []string{"write"}
	response, err = handler(ctx)
	require.Nil(t, response)
	require.False(t, called)
	var appErr *apptheory.AppTheoryError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, "app.forbidden", appErr.Code)
}
