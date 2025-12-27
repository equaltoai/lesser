package api

import (
	"net/http"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

func TestFederationAnalyticsHandler_RegisterRoutes(t *testing.T) {
	router := mux.NewRouter()
	handler := NewFederationAnalyticsHandler(nil, nil)
	handler.RegisterRoutes(router)

	routes := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/admin/federation/graph/nodes"},
		{method: http.MethodGet, path: "/admin/federation/graph/edges"},
		{method: http.MethodGet, path: "/admin/federation/clusters"},
		{method: http.MethodGet, path: "/admin/federation/relationships/a/b"},
		{method: http.MethodGet, path: "/admin/federation/recommendations/example.com"},
		{method: http.MethodGet, path: "/admin/federation/instances/example.com/metadata"},
		{method: http.MethodGet, path: "/admin/federation/timeseries/example.com"},
	}

	for _, tc := range routes {
		req, err := http.NewRequest(tc.method, tc.path, nil)
		require.NoError(t, err)

		var match mux.RouteMatch
		require.True(t, router.Match(req, &match), tc.path)
	}
}
