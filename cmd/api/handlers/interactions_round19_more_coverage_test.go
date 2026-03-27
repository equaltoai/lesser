package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	repomocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type interactionsRound19Repos struct {
	*MockRepositoryStorage
	actor interfaces.ActorRepository
}

func (r *interactionsRound19Repos) Actor() interfaces.ActorRepository {
	return r.actor
}

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

func TestInteractionsRound19_followResolvesNumericAndEncodedTargets(t *testing.T) {
	cfg := round11TestConfig()
	h, repos, _ := round11NewHandler(t, cfg, &round10QueryState{})

	targetActor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("simulacrum"), Type: "Person"},
		PreferredUsername: "simulacrum",
	}
	targetAccount := &storage.Account{
		User: &storage.User{
			ID:       "319442693566094",
			Username: "simulacrum",
		},
		Actor: targetActor,
	}
	actorRepo := repomocks.NewMockActorRepository()
	actorRepo.On("GetActorByNumericID", mock.Anything, "3133216004869690").Return(targetActor, nil).Twice()
	actorRepo.On("GetActor", mock.Anything, "simulacrum").Return(targetActor, nil).Twice()
	h.repos = &interactionsRound19Repos{MockRepositoryStorage: repos, actor: actorRepo}

	var followCalls int
	h.registry = &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, accountID string) (*storage.Account, error) {
				if accountID == "simulacrum" {
					return targetAccount, nil
				}
				return nil, errors.New("not found")
			},
		},
		RelationshipsSvc: &RelationshipsServiceStub{
			FollowFunc: func(_ context.Context, cmd *relationships.FollowCommand) (*relationships.FollowResult, error) {
				followCalls++
				require.Equal(t, "alice", cmd.FollowerID)
				require.Equal(t, targetActor.ID, cmd.FollowingID)
				return &relationships.FollowResult{
					Relationship: &relationships.RelationshipData{ID: "simulacrum", Following: true},
				}, nil
			},
		},
	}

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctxNumeric, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/3133216004869690/follow", headers, nil, nil)
	require.NoError(t, err)
	ctxNumeric.Params["id"] = "3133216004869690"
	respNumeric := requireStatus(t, http.StatusOK)(h.HandleFollowLift(ctxNumeric))
	var numericBody apimodels.Relationship
	require.NoError(t, json.Unmarshal(respNumeric.Body, &numericBody))
	require.Equal(t, targetAccount.User.ID, numericBody.ID)

	doubleEscapedTarget := "https:%252F%252F" + strings.ReplaceAll(strings.TrimPrefix(cfg.ActorURL("simulacrum"), "https://"), "/", "%252F")
	ctxEscaped, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/encoded/follow", headers, nil, nil)
	require.NoError(t, err)
	ctxEscaped.Params["id"] = doubleEscapedTarget
	respEscaped := requireStatus(t, http.StatusOK)(h.HandleFollowLift(ctxEscaped))
	var escapedBody apimodels.Relationship
	require.NoError(t, json.Unmarshal(respEscaped.Body, &escapedBody))
	require.Equal(t, targetAccount.User.ID, escapedBody.ID)

	require.Equal(t, 2, followCalls)
	actorRepo.AssertExpectations(t)
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

func TestInteractionsRound19_HandleFollowLift_ReturnsNotFoundForMissingNumericTarget(t *testing.T) {
	cfg := round11TestConfig()
	h, repos, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
		RelationshipsSvc: &RelationshipsServiceStub{
			FollowFunc: func(context.Context, *relationships.FollowCommand) (*relationships.FollowResult, error) {
				t.Fatal("follow service should not be called when the target account cannot be resolved")
				return nil, nil
			},
		},
	})

	actorRepo := repomocks.NewMockActorRepository()
	actorRepo.On("GetActorByNumericID", mock.Anything, "3133216004869690").
		Return(nil, common.ActorNotFoundError{Username: "3133216004869690"}).
		Once()
	h.repos = &interactionsRound19Repos{MockRepositoryStorage: repos, actor: actorRepo}

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/3133216004869690/follow", headers, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "3133216004869690"

	requireStatus(t, http.StatusNotFound)(h.HandleFollowLift(ctx))
	actorRepo.AssertExpectations(t)
}

func TestInteractionsRound19_statusInteraction_UsesStorageBackedSerialization(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now().UTC()
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-2 * time.Hour)},
			"bob":   {PK: "USER#bob", SK: storagemodels.SKMetadata, Username: "bob", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-3 * time.Hour)},
		},
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {Username: "alice", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("alice"), Type: "Person"}, PreferredUsername: "alice"}},
			"bob":   {Username: "bob", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("bob"), Type: "Person"}, PreferredUsername: "bob", Name: "Bob"}},
		},
	}

	notesSvc := &NotesServiceStub{
		LikeNoteFunc: func(context.Context, *notes.LikeNoteCommand) (*notes.LikeResult, error) {
			return &notes.LikeResult{
				Status: &storagemodels.Status{
					StatusID:       "status-1",
					AuthorUsername: "bob",
					AuthorID:       cfg.ActorURL("bob"),
					Content:        "hello world",
					PublishedAt:    now.Add(-time.Minute),
					CreatedAt:      now.Add(-time.Minute),
				},
			}, nil
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: notesSvc})

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/favourite", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "status-1"

	resp := requireStatus(t, http.StatusOK)(h.HandleFavoriteLift(ctx))

	var body apimodels.Status
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "status-1", body.ID)
	require.Equal(t, "hello world", body.Content)
	require.Equal(t, cfg.ActorURL("bob")+"/statuses/status-1", body.URI)
	require.Equal(t, cfg.BaseURL()+"/@bob/status-1", body.URL)
	require.Equal(t, common.GenerateNumericID("bob"), body.Account.ID)
	require.Equal(t, "bob", body.Account.Username)
	require.True(t, body.Favourited)
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
			},
		},
		agentGovernanceByUsername: map[string]storagemodels.AgentGovernanceState{
			"agent": {
				PK:        "USER#agent",
				SK:        storagemodels.SKAgentGovernance,
				Username:  "agent",
				Verified:  true,
				CreatedAt: now.Add(-time.Hour),
				UpdatedAt: now,
			},
		},
	}

	rel := &relationships.RelationshipData{ID: "bob", Following: true}
	relSvc := &RelationshipsServiceStub{
		FollowFunc: func(_ context.Context, cmd *relationships.FollowCommand) (*relationships.FollowResult, error) {
			require.Equal(t, "agent", cmd.FollowerID)
			require.Equal(t, cfg.ActorURL("bob"), cmd.FollowingID)
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
