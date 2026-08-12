package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

func TestGraphQLWithActAsIndicator_TransportLift(t *testing.T) {
	t.Run("lifts present header into resolver context with trimmed value", func(t *testing.T) {
		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodPost,
				Path:   "/graphql",
				Headers: map[string][]string{
					"x-lesser-act-as": {"  agent:alice:read  "},
				},
			},
		}

		requestCtx := graphqlWithActAsIndicator(context.Background(), ctx)
		require.Equal(t, "agent:alice:read", requestCtx.Value(common.ContextKeyActAsAgent))
	})

	t.Run("absent header leaves resolver context untouched", func(t *testing.T) {
		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodPost,
				Path:    "/graphql",
				Headers: map[string][]string{},
			},
		}

		requestCtx := graphqlWithActAsIndicator(context.Background(), ctx)
		require.Nil(t, requestCtx.Value(common.ContextKeyActAsAgent))
	})

	t.Run("blank header leaves resolver context untouched", func(t *testing.T) {
		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodPost,
				Path:   "/graphql",
				Headers: map[string][]string{
					"x-lesser-act-as": {"   "},
				},
			},
		}

		requestCtx := graphqlWithActAsIndicator(context.Background(), ctx)
		require.Nil(t, requestCtx.Value(common.ContextKeyActAsAgent))
	})

	t.Run("nil apptheory context passes through without panic", func(t *testing.T) {
		base := context.WithValue(context.Background(), contextKeyUser, "owner")
		requestCtx := graphqlWithActAsIndicator(base, nil)
		require.Same(t, base, requestCtx)
		require.Nil(t, requestCtx.Value(common.ContextKeyActAsAgent))
	})
}

func TestHandleGraphQL_ActAsTransportLiftWiring(t *testing.T) {
	originalLogger := logger
	originalHandler := graphQLHandler
	t.Cleanup(func() {
		logger = originalLogger
		graphQLHandler = originalHandler
	})

	logger = zap.NewNop()

	t.Run("handleGraphQL composes act-as indicator into resolver context", func(t *testing.T) {
		graphQLHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "agent:alice:read", r.Context().Value(common.ContextKeyActAsAgent))
			w.WriteHeader(200)
			_, _ = w.Write([]byte("ok"))
		})

		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodPost,
				Path:   "/graphql",
				Body:   []byte(`{"query":"{ me { id } }"}`),
				Headers: map[string][]string{
					"x-lesser-act-as": {"agent:alice:read"},
				},
			},
		}
		ctx.Set("is_authenticated", true)
		ctx.Set("claims", &auth.Claims{Username: "owner", Scopes: []string{"read"}})

		resp, err := handleGraphQL(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)
	})

	t.Run("handleGraphQL without header carries no act-as value", func(t *testing.T) {
		graphQLHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Nil(t, r.Context().Value(common.ContextKeyActAsAgent))
			w.WriteHeader(200)
			_, _ = w.Write([]byte("ok"))
		})

		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodPost,
				Path:    "/graphql",
				Body:    []byte(`{"query":"{ me { id } }"}`),
				Headers: map[string][]string{},
			},
		}
		ctx.Set("is_authenticated", true)
		ctx.Set("claims", &auth.Claims{Username: "owner", Scopes: []string{"read"}})

		resp, err := handleGraphQL(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)
	})
}
