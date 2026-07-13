package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func TestObjectsStrictRouteInventoryParity_Round13(t *testing.T) {
	artifactBytes, err := os.ReadFile("testdata/route_inventory.json")
	require.NoError(t, err)

	var artifact []objectsRouteInventoryEntry
	require.NoError(t, json.Unmarshal(artifactBytes, &artifact))
	require.Equal(t, []objectsRouteInventoryEntry{
		{Method: http.MethodGet, Path: "/objects/:id"},
		{Method: http.MethodGet, Path: "/articles/:slug"},
		{Method: http.MethodGet, Path: "/users/:username/statuses/:id"},
	}, artifact)
	require.Equal(t, artifact, objectsRouteInventory())

	var params []map[string]string
	app := apptheory.New()
	require.NoError(t, registerObjectsRoutes(app, func(ctx *apptheory.Context) (*apptheory.Response, error) {
		params = append(params, map[string]string{
			"id":       ctx.Param("id"),
			"slug":     ctx.Param("slug"),
			"username": ctx.Param("username"),
		})
		return &apptheory.Response{Status: http.StatusNoContent}, nil
	}))

	resp := app.Serve(context.Background(), apptheory.Request{
		Method: http.MethodGet,
		Path:   "/objects/note-1",
	})
	require.Equal(t, http.StatusNoContent, resp.Status)
	require.Equal(t, map[string]string{"id": "note-1", "slug": "", "username": ""}, params[0])

	resp = app.Serve(context.Background(), apptheory.Request{
		Method: http.MethodGet,
		Path:   "/articles/hello-world",
	})
	require.Equal(t, http.StatusNoContent, resp.Status)
	require.Equal(t, map[string]string{"id": "", "slug": "hello-world", "username": ""}, params[1])

	resp = app.Serve(context.Background(), apptheory.Request{
		Method: http.MethodGet,
		Path:   "/users/alice/statuses/note-2",
	})
	require.Equal(t, http.StatusNoContent, resp.Status)
	require.Equal(t, map[string]string{"id": "note-2", "slug": "", "username": "alice"}, params[2])

	resp = app.Serve(context.Background(), apptheory.Request{
		Method: http.MethodPost,
		Path:   "/objects/note-1",
	})
	require.Equal(t, http.StatusMethodNotAllowed, resp.Status)
	require.Len(t, params, 3)

	resp = app.Serve(context.Background(), apptheory.Request{
		Method: http.MethodGet,
		Path:   "/users/alice/statuses",
	})
	require.Equal(t, http.StatusNotFound, resp.Status)
	require.Len(t, params, 3)
}

func TestRegisterObjectsRoutesNilAppNoop_Round13(t *testing.T) {
	err := registerObjectsRoutes(nil, func(*apptheory.Context) (*apptheory.Response, error) {
		return &apptheory.Response{Status: http.StatusNoContent}, nil
	})
	require.NoError(t, err)
}
