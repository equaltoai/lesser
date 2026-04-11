package main

import (
	"reflect"
	"testing"

	apiHandlers "github.com/equaltoai/lesser/cmd/api/handlers"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap/zaptest"
)

type routeAuthState struct {
	authRequired     bool
	optionalAuth     bool
	requiredScopes   []string
	requiredAnyScope []string
}

func TestConfigureRoutes_AuthOptions(t *testing.T) {
	logger = zaptest.NewLogger(t)
	apiHandler = &apiHandlers.Handler{}

	app := apptheory.New()
	configureRoutes(app)

	routes := configuredRouteAuthStates(t, app)

	require.Equal(t, routeAuthState{optionalAuth: true}, routes["POST /api/v1/apps"])
	require.Equal(t, routeAuthState{
		authRequired:   true,
		requiredScopes: []string{"read"},
	}, routes["GET /api/v1/accounts/verify_credentials"])
	require.Equal(t, routeAuthState{
		authRequired: true,
		requiredAnyScope: []string{
			"follow",
			"write:follows",
			"write",
		},
	}, routes["POST /api/v1/accounts/{id}/follow"])
	require.Equal(t, routeAuthState{optionalAuth: true}, routes["GET /api/v1/statuses/{id}"])
	require.Equal(t, routeAuthState{authRequired: true}, routes["GET /api/v1/admin/accounts"])
	require.Equal(t, routeAuthState{
		authRequired:   true,
		requiredScopes: []string{"admin"},
	}, routes["GET /api/v1/admin/agents/policy"])
	require.Equal(t, routeAuthState{authRequired: true}, routes["POST /api/v1/trust/previews"])
	require.Equal(t, routeAuthState{}, routes["POST /api/v1/notifications/deliver"])
	require.Equal(t, routeAuthState{}, routes["POST /api/v1/agents/{username}/access-leases/{leaseID}/token"])
}

func configuredRouteAuthStates(t *testing.T, app *apptheory.App) map[string]routeAuthState {
	t.Helper()

	appValue := reflect.ValueOf(app)
	require.Equal(t, reflect.Ptr, appValue.Kind())

	routerValue := appValue.Elem().FieldByName("router")
	require.True(t, routerValue.IsValid())
	require.False(t, routerValue.IsNil())

	routesValue := routerValue.Elem().FieldByName("routes")
	require.True(t, routesValue.IsValid())

	out := make(map[string]routeAuthState, routesValue.Len())
	for i := 0; i < routesValue.Len(); i++ {
		routeValue := routesValue.Index(i)
		key := routeValue.FieldByName("Method").String() + " " + routeValue.FieldByName("Pattern").String()
		out[key] = routeAuthState{
			authRequired:     routeValue.FieldByName("AuthRequired").Bool(),
			optionalAuth:     routeValue.FieldByName("OptionalAuth").Bool(),
			requiredScopes:   stringSliceValue(routeValue.FieldByName("RequiredScopes")),
			requiredAnyScope: stringSliceValue(routeValue.FieldByName("RequiredAnyScope")),
		}
	}

	return out
}

func stringSliceValue(value reflect.Value) []string {
	if !value.IsValid() || value.Len() == 0 {
		return nil
	}

	out := make([]string, 0, value.Len())
	for i := 0; i < value.Len(); i++ {
		out = append(out, value.Index(i).String())
	}

	return out
}
