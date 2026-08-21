package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
)

func TestWebFingerStrictRouteInventoryParity_Round13(t *testing.T) {
	artifactBytes, err := os.ReadFile("testdata/route_inventory.json")
	require.NoError(t, err)

	var artifact []webfingerRouteInventoryEntry
	require.NoError(t, json.Unmarshal(artifactBytes, &artifact))
	require.Equal(t, []webfingerRouteInventoryEntry{{Method: http.MethodGet, Path: "/.well-known/webfinger"}}, artifact)
	require.Equal(t, artifact, webfingerRouteInventory())

	hits := 0
	app := apptheory.NewSecure(apptheory.SecureOptions{Tier: apptheory.TierP2})
	require.NoError(t, registerWebFingerRoutes(app, func(*apptheory.Context) (*apptheory.Response, error) {
		hits++
		return &apptheory.Response{Status: http.StatusNoContent}, nil
	}))

	resp := app.Serve(context.Background(), apptheory.Request{
		Method: http.MethodGet,
		Path:   "/.well-known/webfinger",
	})
	require.Equal(t, http.StatusNoContent, resp.Status)
	require.Equal(t, 1, hits)

	resp = app.Serve(context.Background(), apptheory.Request{
		Method: http.MethodPost,
		Path:   "/.well-known/webfinger",
	})
	require.Equal(t, http.StatusMethodNotAllowed, resp.Status)
	require.Equal(t, 1, hits)

	resp = app.Serve(context.Background(), apptheory.Request{
		Method: http.MethodGet,
		Path:   "/.well-known/nodeinfo",
	})
	require.Equal(t, http.StatusNotFound, resp.Status)
	require.Equal(t, 1, hits)
}

func TestRegisterWebFingerRoutesNilAppNoop_Round13(t *testing.T) {
	err := registerWebFingerRoutes(nil, func(*apptheory.Context) (*apptheory.Response, error) {
		return &apptheory.Response{Status: http.StatusNoContent}, nil
	})
	require.NoError(t, err)
}
