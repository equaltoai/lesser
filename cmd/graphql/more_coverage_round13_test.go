package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	apiHandlers "github.com/equaltoai/lesser/cmd/api/handlers"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func TestGraphQLHelperFunctions_Round13(t *testing.T) {
	t.Run("graphqlBuildURL encodes query parameters", func(t *testing.T) {
		u := graphqlBuildURL("/graphql", map[string][]string{
			"a": {"1", "2"},
			"b": {"x"},
		})
		require.Equal(t, "/graphql", u.Path)
		require.Contains(t, u.RawQuery, "a=1")
		require.Contains(t, u.RawQuery, "a=2")
		require.Contains(t, u.RawQuery, "b=x")
	})

	t.Run("graphqlRequestID prefers existing request id", func(t *testing.T) {
		ctx := &apptheory.Context{RequestID: "rid"}
		require.Equal(t, "rid", graphqlRequestID(ctx, "graphql"))
	})

	t.Run("graphqlRequestID uses prefix fallback", func(t *testing.T) {
		id := graphqlRequestID(&apptheory.Context{}, "")
		require.True(t, strings.HasPrefix(id, "graphql-"))
	})

	t.Run("graphqlContextRequestID uses stored requestID", func(t *testing.T) {
		ctx := &apptheory.Context{RequestID: "rid"}
		ctx.Set("requestID", "stored")
		require.Equal(t, "stored", graphqlContextRequestID(ctx))
	})

	t.Run("graphqlErrorResponse defaults message when empty", func(t *testing.T) {
		resp := graphqlErrorResponse(http.StatusInternalServerError, "")
		require.Equal(t, http.StatusInternalServerError, resp.Status)
		require.Contains(t, string(resp.Body), "Internal server error")
	})
}

func TestGraphQLInstanceLockMiddleware_Round13(t *testing.T) {
	origRepos := repos
	t.Cleanup(func() { repos = origRepos })

	nextCalled := false
	next := func(*apptheory.Context) (*apptheory.Response, error) {
		nextCalled = true
		return apptheory.Text(http.StatusOK, "ok"), nil
	}

	mw := graphqlInstanceLockMiddleware()

	t.Run("nil ctx passes through", func(t *testing.T) {
		nextCalled = false
		resp, err := mw(next)(nil)
		require.NoError(t, err)
		require.True(t, nextCalled)
		require.Equal(t, http.StatusOK, resp.Status)
	})

	t.Run("non-POST passes through", func(t *testing.T) {
		nextCalled = false
		ctx := &apptheory.Context{Request: apptheory.Request{Method: http.MethodGet, Path: "/graphql"}}
		resp, err := mw(next)(ctx)
		require.NoError(t, err)
		require.True(t, nextCalled)
		require.Equal(t, http.StatusOK, resp.Status)
	})

	t.Run("non-GraphQL paths pass through", func(t *testing.T) {
		nextCalled = false
		ctx := &apptheory.Context{Request: apptheory.Request{Method: http.MethodPost, Path: "/not-graphql"}}
		resp, err := mw(next)(ctx)
		require.NoError(t, err)
		require.True(t, nextCalled)
		require.Equal(t, http.StatusOK, resp.Status)
	})

	t.Run("returns forbidden when instance repo unavailable", func(t *testing.T) {
		repos = nil
		nextCalled = false
		ctx := &apptheory.Context{Request: apptheory.Request{Method: http.MethodPost, Path: "/graphql"}}
		resp, err := mw(next)(ctx)
		require.NoError(t, err)
		require.False(t, nextCalled)
		require.Equal(t, http.StatusForbidden, resp.Status)
	})

	t.Run("returns forbidden when instance repo is nil", func(t *testing.T) {
		mockRepos := &apiHandlers.MockRepositoryStorage{}
		mockRepos.On("Instance").Return((*repositories.InstanceRepository)(nil)).Maybe()
		repos = mockRepos

		nextCalled = false
		ctx := &apptheory.Context{Request: apptheory.Request{Method: http.MethodPost, Path: "/api/graphql"}}
		resp, err := mw(next)(ctx)
		require.NoError(t, err)
		require.False(t, nextCalled)
		require.Equal(t, http.StatusForbidden, resp.Status)
	})
}

func TestGraphQLWithCostTracker_Round13(t *testing.T) {
	ctx := &apptheory.Context{}
	ctx.Set("cost_tracker", "x")
	requestCtx := graphqlWithCostTracker(context.Background(), ctx)
	require.Equal(t, "x", requestCtx.Value(contextKeyCostTracker))
}
