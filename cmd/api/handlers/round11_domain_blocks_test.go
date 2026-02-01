package handlers

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

func TestDomainBlocksHandlers(t *testing.T) {
	cfg := round11TestConfig()

	relStub := &RelationshipsServiceStub{
		GetDomainBlocksFunc: func(_ context.Context, _ *relationships.GetDomainBlocksQuery) (*relationships.DomainBlocksResult, error) {
			return &relationships.DomainBlocksResult{Domains: []string{"example.com"}, NextCursor: "next"}, nil
		},
		AddDomainBlockFunc:    func(_ context.Context, _ *relationships.AddDomainBlockCommand) error { return nil },
		RemoveDomainBlockFunc: func(_ context.Context, _ *relationships.RemoveDomainBlockCommand) error { return nil },
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relStub})
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})

	ctxGet, err := round10NewLiftContext(http.MethodGet, "/api/v1/domain_blocks", map[string]string{"Authorization": "Bearer " + readToken}, map[string]string{"limit": "1"}, nil)
	require.NoError(t, err)
	respGet := requireStatus(t, http.StatusOK)(handler.HandleGetDomainBlocksLift(ctxGet))
	require.Contains(t, firstStringValue(respGet.Headers, "link"), "max_id=")

	ctxCreate, err := round10NewLiftContext(http.MethodPost, "/api/v1/domain_blocks", map[string]string{"Authorization": "Bearer " + writeToken}, nil, apimodels.CreateDomainBlockRequest{Domain: "example.com"})
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleCreateDomainBlockLift(ctxCreate))

	ctxInvalid, err := round10NewLiftContext(http.MethodPost, "/api/v1/domain_blocks", map[string]string{"Authorization": "Bearer " + writeToken}, nil, apimodels.CreateDomainBlockRequest{Domain: "bad domain"})
	require.NoError(t, err)
	requireStatus(t, http.StatusBadRequest)(handler.HandleCreateDomainBlockLift(ctxInvalid))

	ctxDelete, err := round10NewLiftContext(http.MethodDelete, "/api/v1/domain_blocks", map[string]string{"Authorization": "Bearer " + writeToken}, map[string]string{"domain": "example.com"}, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleDeleteDomainBlockLift(ctxDelete))
}

func TestDomainBlocks_DeleteErrors(t *testing.T) {
	cfg := round11TestConfig()
	relStub := &RelationshipsServiceStub{
		RemoveDomainBlockFunc: func(_ context.Context, _ *relationships.RemoveDomainBlockCommand) error {
			return errors.New("failed")
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relStub})
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})

	ctxDelete, err := round10NewLiftContext(http.MethodDelete, "/api/v1/domain_blocks", map[string]string{"Authorization": "Bearer " + writeToken}, map[string]string{"domain": "example.com"}, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusInternalServerError)(handler.HandleDeleteDomainBlockLift(ctxDelete))
}
