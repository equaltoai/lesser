package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestStatusesRound12_CreateStatusAsAgent(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	now := time.Now().UTC()

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"agent": {
				PK:        "USER#agent",
				SK:        storagemodels.SKMetadata,
				Username:  "agent",
				Role:      "user",
				Approved:  true,
				Version:   1,
				CreatedAt: now.Add(-24 * time.Hour),
				IsAgent:   true,
				AgentCapabilities: &agents.Capabilities{
					CanPost:        true,
					MaxPostsPerHour: 10,
				},
			},
		},
		statusByID: map[string]storagemodels.Status{
			"orig1": {
				StatusID:       "orig1",
				AuthorUsername: "agent",
				Content:        "original",
				PublishedAt:    now.Add(-1 * time.Hour),
			},
		},
	}

	var sawCommand *notes.CreateNoteCommand
	reg := &RegistryStub{
		NotesSvc: &NotesServiceStub{
			CreateNoteFunc: func(_ context.Context, cmd *notes.CreateNoteCommand) (*notes.NoteResult, error) {
				sawCommand = cmd
				return &notes.NoteResult{
					Note: &storagemodels.Status{
						StatusID:       "created1",
						AuthorUsername: cmd.AuthorID,
						Content:        cmd.Content,
						PublishedAt:    now,
					},
				}, nil
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state, reg)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	agentToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeWrite, auth.ScopeRead, "follow"})
	headers := map[string]string{"Authorization": "Bearer " + agentToken}

	req := apimodels.CreateStatusRequest{
		Status:     "correction",
		Visibility: VisibilityPublic,
		MemoryEvent: &apimodels.AgentMemoryEventRequest{
			EventType:  "correction",
			OriginalID: "orig1",
			Reason:     "fix",
		},
		AgentAttribution: &apimodels.AgentPostAttribution{
			TriggerType:    "mention",
			TriggerDetails: "why",
			MemoryCitations: []string{
				"orig1",
			},
		},
	}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, req)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusCreated)(h.HandleCreateStatusLift(ctx))
	require.NotEmpty(t, resp.Body)

	require.NotNil(t, sawCommand)
	require.Equal(t, "agent", sawCommand.AuthorID)
	require.Equal(t, "orig1", sawCommand.InReplyToID)
	require.NotNil(t, sawCommand.AgentAttribution)
	require.Equal(t, "mention", sawCommand.AgentAttribution.TriggerType)
}

func TestStatusesRound12_CreateStatusNotesServiceFailure(t *testing.T) {
	cfg := round10TestConfig()

	reg := &RegistryStub{
		NotesSvc: &NotesServiceStub{
			CreateNoteFunc: func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) {
				return nil, assertErr("boom")
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, reg)

	userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + userToken}

	req := apimodels.CreateStatusRequest{Status: "hello"}
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, req)
	require.NoError(t, err)

	requireStatus(t, http.StatusInternalServerError)(h.HandleCreateStatusLift(ctx))
}

func TestStatusesRound12_UpdateStatusBlockedForAgents(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	reg := &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
				return &storage.Account{
					Actor: activitypub.NewActor(activitypub.PersonType, "https://example.com/users/agent", "agent"),
					User:  &storage.User{Username: "agent"},
				}, nil
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, reg)

	agentToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + agentToken}

	ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", headers, nil, apimodels.UpdateStatusRequest{Status: "edit"})
	require.NoError(t, err)
	ctx.Params["id"] = "s1"

	requireStatus(t, http.StatusForbidden)(h.HandleUpdateStatusLift(ctx))
}

func TestStatusesRound12_DeleteStatusRecordsAgentTombstone(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	now := time.Now().UTC()
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"agent": {
				PK:        "USER#agent",
				SK:        storagemodels.SKMetadata,
				Username:  "agent",
				Role:      "user",
				Approved:  true,
				Version:   1,
				CreatedAt: now.Add(-24 * time.Hour),
				IsAgent:   true,
			},
		},
	}

	reg := &RegistryStub{
		NotesSvc: &NotesServiceStub{
			GetNoteFunc: func(context.Context, string) (*storagemodels.Status, error) {
				return &storagemodels.Status{
					StatusID:       "s1",
					AuthorUsername: "agent",
					Content:        "hello",
					PublishedAt:    now.Add(-5 * time.Minute),
				}, nil
			},
			DeleteNoteFunc: func(context.Context, *notes.DeleteNoteCommand) error { return nil },
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state, reg)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	agentToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + agentToken}

	ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1", headers, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "s1"

	requireStatus(t, http.StatusOK)(h.HandleDeleteStatusLift(ctx))
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

var _ error = assertErr("")
