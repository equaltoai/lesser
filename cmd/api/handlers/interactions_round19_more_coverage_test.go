package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func round19SignAgentAccessToken(t *testing.T, secret, username string, scopes []string) string {
	t.Helper()

	now := time.Now()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
		},
		Username: username,
		ClientID: "test-client",
		Scopes:   scopes,
		IsAgent:  true,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func TestInteractionsRound19_enforceAgentFollowRails_NilSafety(t *testing.T) {
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/follow", nil, nil, nil)
	require.NoError(t, err)

	var nilHandler *Handler
	resp, respErr := nilHandler.enforceAgentFollowRails(ctx, "alice")
	require.NoError(t, respErr)
	require.Nil(t, resp)

	resp, respErr = (&Handler{}).enforceAgentFollowRails(ctx, "alice")
	require.NoError(t, respErr)
	require.Nil(t, resp)
}

func TestInteractionsRound19_relationshipOperationValidationAndServiceUnavailable(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("missing account id is bad request", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts//follow", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleFollowLift(ctx))
	})

	t.Run("nil relationships service is 503 after auth", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/follow", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "bob"

		requireStatus(t, http.StatusServiceUnavailable)(h.HandleFollowLift(ctx))
	})
}

func TestInteractionsRound19_relationshipOperation_InvalidOperation(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	h.registry = &RegistryStub{RelationshipsSvc: &RelationshipsServiceStub{}}

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unknown", headers, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "bob"

	requireStatus(t, http.StatusBadRequest)(h.relationshipOperation(ctx, "unknown"))
}

func TestInteractionsRound19_statusInteraction_ErrorBranches(t *testing.T) {
	cfg := round11TestConfig()

	notesStub := &NotesServiceStub{
		LikeNoteFunc: func(context.Context, *notes.LikeNoteCommand) (*notes.LikeResult, error) {
			return nil, errors.New("not found")
		},
		UnreblogNoteFunc: func(context.Context, *notes.UnreblogNoteCommand) (*notes.LikeResult, error) {
			return nil, errors.New("boom")
		},
	}
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesStub})

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctxFav, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/1/favourite", headers, nil, nil)
	require.NoError(t, err)
	ctxFav.Params["id"] = "1"
	requireStatus(t, http.StatusNotFound)(h.HandleFavoriteLift(ctxFav))

	ctxUnreblog, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/1/unreblog", headers, nil, nil)
	require.NoError(t, err)
	ctxUnreblog.Params["id"] = "1"
	requireStatus(t, http.StatusInternalServerError)(h.HandleUnreblogLift(ctxUnreblog))

	ctxInvalid, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/1/invalid", headers, nil, nil)
	require.NoError(t, err)
	ctxInvalid.Params["id"] = "1"
	requireStatus(t, http.StatusBadRequest)(h.statusInteraction(ctxInvalid, "invalid"))
}

func TestInteractionsRound19_agentFollowRails_AllowsFollowWhenUnderLimit(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"agent": {
				PK:        "USER#agent",
				SK:        storagemodels.SKMetadata,
				Username:  "agent",
				Role:      "user",
				Approved:  true,
				IsAgent:   true,
				Version:   1,
				CreatedAt: now.Add(-time.Hour),
				Metadata: map[string]interface{}{
					"agent_verified": true,
				},
			},
		},
	}

	rel := &relationships.RelationshipData{ID: "bob", Following: true}
	relSvc := &RelationshipsServiceStub{
		FollowFunc: func(_ context.Context, cmd *relationships.FollowCommand) (*relationships.FollowResult, error) {
			require.Equal(t, "agent", cmd.FollowerID)
			require.Equal(t, "bob", cmd.FollowingID)
			return &relationships.FollowResult{Relationship: rel}, nil
		},
	}
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{RelationshipsSvc: relSvc})

	token := round19SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/follow", headers, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "bob"

	requireStatus(t, http.StatusOK)(h.HandleFollowLift(ctx))
}

func TestInteractionsRound19_getBlocks_ServiceErrorAndInsufficientScope(t *testing.T) {
	cfg := round11TestConfig()

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
		RelationshipsSvc: &RelationshipsServiceStub{
			GetBlockedUsersFunc: func(context.Context, *relationships.GetBlockedUsersQuery) (*relationships.BlockedUsersResult, error) {
				return nil, errors.New("boom")
			},
		},
	})

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	ctxErr, err := round10NewLiftContext(http.MethodGet, "/api/v1/blocks", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusInternalServerError)(h.HandleGetBlocksLift(ctxErr))

	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	ctxForbidden, err := round10NewLiftContext(http.MethodGet, "/api/v1/blocks", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusForbidden)(h.HandleGetBlocksLift(ctxForbidden))
}
