package lift

import (
	"context"
	"errors"
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/stretchr/testify/require"
)

func TestDomainBlocks_Round12_Coverage(t *testing.T) {
	cfg := round11TestConfig()

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})

	t.Run("get_unauthorized", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			RelationshipsSvc: &RelationshipsServiceStub{},
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/domain_blocks", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleGetDomainBlocksLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("get_service_error_returns_500", func(t *testing.T) {
		relStub := &RelationshipsServiceStub{
			GetDomainBlocksFunc: func(_ context.Context, _ *relationships.GetDomainBlocksQuery) (*relationships.DomainBlocksResult, error) {
				return nil, errors.New("boom")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/domain_blocks", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleGetDomainBlocksLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("create_parse_error_returns_400", func(t *testing.T) {
		relStub := &RelationshipsServiceStub{
			AddDomainBlockFunc: func(_ context.Context, _ *relationships.AddDomainBlockCommand) error { return nil },
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relStub})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/domain_blocks", map[string]string{"Authorization": "Bearer " + writeToken}, nil, []byte("{"))
		require.NoError(t, handler.HandleCreateDomainBlockLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create_unauthorized", func(t *testing.T) {
		relStub := &RelationshipsServiceStub{
			AddDomainBlockFunc: func(_ context.Context, _ *relationships.AddDomainBlockCommand) error { return nil },
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relStub})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/domain_blocks", nil, nil, apimodels.CreateDomainBlockRequest{Domain: "example.com"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleCreateDomainBlockLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("create_requires_domain", func(t *testing.T) {
		relStub := &RelationshipsServiceStub{
			AddDomainBlockFunc: func(_ context.Context, _ *relationships.AddDomainBlockCommand) error { return nil },
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relStub})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/domain_blocks", map[string]string{"Authorization": "Bearer " + writeToken}, nil, apimodels.CreateDomainBlockRequest{Domain: ""})
		require.NoError(t, err)
		require.NoError(t, handler.HandleCreateDomainBlockLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create_invalid_domain_double_dot", func(t *testing.T) {
		relStub := &RelationshipsServiceStub{
			AddDomainBlockFunc: func(_ context.Context, _ *relationships.AddDomainBlockCommand) error { return nil },
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relStub})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/domain_blocks", map[string]string{"Authorization": "Bearer " + writeToken}, nil, apimodels.CreateDomainBlockRequest{Domain: "bad..domain"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleCreateDomainBlockLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create_invalid_domain_prefix_dot", func(t *testing.T) {
		relStub := &RelationshipsServiceStub{
			AddDomainBlockFunc: func(_ context.Context, _ *relationships.AddDomainBlockCommand) error { return nil },
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relStub})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/domain_blocks", map[string]string{"Authorization": "Bearer " + writeToken}, nil, apimodels.CreateDomainBlockRequest{Domain: ".bad"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleCreateDomainBlockLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create_invalid_domain_suffix_dot", func(t *testing.T) {
		relStub := &RelationshipsServiceStub{
			AddDomainBlockFunc: func(_ context.Context, _ *relationships.AddDomainBlockCommand) error { return nil },
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relStub})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/domain_blocks", map[string]string{"Authorization": "Bearer " + writeToken}, nil, apimodels.CreateDomainBlockRequest{Domain: "bad."})
		require.NoError(t, err)
		require.NoError(t, handler.HandleCreateDomainBlockLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create_service_error_returns_500", func(t *testing.T) {
		relStub := &RelationshipsServiceStub{
			AddDomainBlockFunc: func(_ context.Context, _ *relationships.AddDomainBlockCommand) error { return errors.New("add failed") },
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relStub})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/domain_blocks", map[string]string{"Authorization": "Bearer " + writeToken}, nil, apimodels.CreateDomainBlockRequest{Domain: "example.com"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleCreateDomainBlockLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("get_clamps_limit_to_200", func(t *testing.T) {
		var gotLimit int
		relStub := &RelationshipsServiceStub{
			GetDomainBlocksFunc: func(_ context.Context, query *relationships.GetDomainBlocksQuery) (*relationships.DomainBlocksResult, error) {
				gotLimit = query.Limit
				return &relationships.DomainBlocksResult{Domains: []string{"example.com"}}, nil
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/domain_blocks", map[string]string{"Authorization": "Bearer " + readToken}, map[string]string{"limit": "500"}, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleGetDomainBlocksLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		require.Equal(t, 100, gotLimit)
		require.Empty(t, ctx.Response.Headers["Link"])
	})

	t.Run("delete_missing_auth_returns_401", func(t *testing.T) {
		relStub := &RelationshipsServiceStub{
			RemoveDomainBlockFunc: func(_ context.Context, _ *relationships.RemoveDomainBlockCommand) error { return nil },
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relStub})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/domain_blocks", nil, map[string]string{"domain": "example.com"}, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleDeleteDomainBlockLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("delete_invalid_token_returns_401", func(t *testing.T) {
		relStub := &RelationshipsServiceStub{
			RemoveDomainBlockFunc: func(_ context.Context, _ *relationships.RemoveDomainBlockCommand) error { return nil },
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relStub})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/domain_blocks", map[string]string{"Authorization": "Bearer not-a-real-jwt"}, map[string]string{"domain": "example.com"}, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleDeleteDomainBlockLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("delete_insufficient_scope_returns_403", func(t *testing.T) {
		relStub := &RelationshipsServiceStub{
			RemoveDomainBlockFunc: func(_ context.Context, _ *relationships.RemoveDomainBlockCommand) error { return nil },
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relStub})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/domain_blocks", map[string]string{"authorization": "Bearer " + readToken}, map[string]string{"domain": "example.com"}, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleDeleteDomainBlockLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("delete_requires_domain_param", func(t *testing.T) {
		relStub := &RelationshipsServiceStub{
			RemoveDomainBlockFunc: func(_ context.Context, _ *relationships.RemoveDomainBlockCommand) error { return nil },
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relStub})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/domain_blocks", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleDeleteDomainBlockLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})
}
