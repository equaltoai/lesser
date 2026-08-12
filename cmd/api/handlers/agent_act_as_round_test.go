package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/agentshare"
	"github.com/equaltoai/lesser/pkg/services/notes"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

// actAsShareServiceStub is a configurable AgentShareService stub for act-as tests.
type actAsShareServiceStub struct {
	isActiveFunc func(context.Context, string, string) (bool, error)
}

func (s *actAsShareServiceStub) Grant(context.Context, agentshare.ManageInput) (*storagemodels.AgentShareGrant, error) {
	return nil, errors.New("not implemented")
}

func (s *actAsShareServiceStub) Revoke(context.Context, agentshare.ManageInput) (*storagemodels.AgentShareGrant, error) {
	return nil, errors.New("not implemented")
}

func (s *actAsShareServiceStub) ListByAgent(context.Context, string, string, bool) ([]*storagemodels.AgentShareGrant, error) {
	return nil, nil
}

func (s *actAsShareServiceStub) ListSharedWith(context.Context, string) ([]*storagemodels.AgentShareGrant, error) {
	return nil, nil
}

func (s *actAsShareServiceStub) IsActive(ctx context.Context, agent, grantee string) (bool, error) {
	return s.isActiveFunc(ctx, agent, grantee)
}

func actAsTestState(now time.Time) *round10QueryState {
	return &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"agent-one": {
				PK:        "USER#agent-one",
				SK:        storagemodels.SKMetadata,
				Username:  "agent-one",
				Role:      "user",
				Approved:  true,
				Version:   1,
				CreatedAt: now.Add(-24 * time.Hour),
				IsAgent:   true,
			},
		},
	}
}

func actAsActiveGrantStub() *actAsShareServiceStub {
	return &actAsShareServiceStub{
		isActiveFunc: func(_ context.Context, agent, grantee string) (bool, error) {
			return agent == "agent-one" && grantee == "alice", nil
		},
	}
}

func actAsCaptureNotesStub(now time.Time, gotCmd **notes.CreateNoteCommand) *NotesServiceStub {
	return &NotesServiceStub{
		CreateNoteFunc: func(_ context.Context, cmd *notes.CreateNoteCommand) (*notes.NoteResult, error) {
			*gotCmd = cmd
			return &notes.NoteResult{
				Note: &storagemodels.Status{
					StatusID:       "s-act-as-1",
					AuthorUsername: cmd.AuthorID,
					AuthorID:       "https://example.com/users/" + cmd.AuthorID,
					Visibility:     cmd.Visibility,
					PublishedAt:    now,
					CreatedAt:      now,
					UpdatedAt:      now,
					ModifiedAt:     now,
					Version:        1,
					Note: &activitypub.Note{
						BaseObject:       activitypub.BaseObject{ID: "https://example.com/objects/s-act-as-1"},
						Content:          cmd.Content,
						AttributedTo:     "https://example.com/users/" + cmd.AuthorID,
						AgentAttribution: cmd.AgentAttribution,
					},
				},
			}, nil
		},
	}
}

func TestActAs_CreateStatus_GrantHolderActsAgentScopedWithAttribution(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()
	state := actAsTestState(now)

	var gotCmd *notes.CreateNoteCommand
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		NotesSvc:       actAsCaptureNotesStub(now, &gotCmd),
		AgentSharesSvc: actAsActiveGrantStub(),
	})

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
	headers := map[string]string{
		"Authorization":       "Bearer " + token,
		auth.ActAsAgentHeader: "agent-one",
	}
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, models.CreateStatusRequest{
		Status: "hello from the agent",
	})
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusCreated)(h.HandleCreateStatusLift(ctx))

	// Agent-scoped authorship with mandatory caller attribution on the mutation.
	require.NotNil(t, gotCmd)
	require.Equal(t, "agent-one", gotCmd.AuthorID)
	require.NotNil(t, gotCmd.AgentAttribution)
	require.Equal(t, cfg.ActorURL("alice"), gotCmd.AgentAttribution.ActedBy)

	// Attribution surfaced on the API response.
	var got models.Status
	require.NoError(t, json.Unmarshal(resp.Body, &got))
	require.NotNil(t, got.AgentAttribution)
	require.Equal(t, cfg.ActorURL("alice"), got.AgentAttribution.ActedBy)

	// Attribution recorded in audit: agent in the username position, caller in acted_by.
	entries := state.auditLogsByUser["agent-one"]
	require.Len(t, entries, 1)
	require.Equal(t, "agent.status.create", entries[0].EventType)
	require.Contains(t, entries[0].Metadata, `"acted_by":"alice"`)
	require.Contains(t, entries[0].Metadata, `"agent_username":"agent-one"`)
	require.Contains(t, entries[0].Metadata, `"target_id":"s-act-as-1"`)
}

func TestActAs_CreateStatus_OwnerPathByteIdentical(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()
	state := actAsTestState(now)

	var gotCmd *notes.CreateNoteCommand
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		NotesSvc: actAsCaptureNotesStub(now, &gotCmd),
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				t.Fatal("grant check must not run without the act-as indicator")
				return false, nil
			},
		},
	})

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, models.CreateStatusRequest{
		Status: "ordinary post",
	})
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusCreated)(h.HandleCreateStatusLift(ctx))

	require.NotNil(t, gotCmd)
	require.Equal(t, "alice", gotCmd.AuthorID)
	require.Nil(t, gotCmd.AgentAttribution, "owner path must not gain attribution")

	var got models.Status
	require.NoError(t, json.Unmarshal(resp.Body, &got))
	require.Nil(t, got.AgentAttribution, "owner path must not surface agent attribution for non-agent authors")

	require.Empty(t, state.auditLogsByUser, "owner path must not emit act-as audit events")
}

func TestActAs_CreateStatus_RevokedGrantFailsClosed(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()

	createCalled := false
	h, _, _ := round11NewHandler(t, cfg, actAsTestState(now), &RegistryStub{
		NotesSvc: &NotesServiceStub{
			CreateNoteFunc: func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) {
				createCalled = true
				return nil, errors.New("must not be called")
			},
		},
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				return false, nil // revoked or never granted
			},
		},
	})

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
	headers := map[string]string{
		"Authorization":       "Bearer " + token,
		auth.ActAsAgentHeader: "agent-one",
	}
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, models.CreateStatusRequest{
		Status: "should never post",
	})
	require.NoError(t, err)

	requireStatus(t, http.StatusForbidden)(h.HandleCreateStatusLift(ctx))
	require.False(t, createCalled)
}

func TestActAs_CreateStatus_MalformedIndicator(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()

	createCalled := false
	h, _, _ := round11NewHandler(t, cfg, actAsTestState(now), &RegistryStub{
		NotesSvc: &NotesServiceStub{
			CreateNoteFunc: func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) {
				createCalled = true
				return nil, errors.New("must not be called")
			},
		},
		AgentSharesSvc: actAsActiveGrantStub(),
	})

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
	headers := map[string]string{
		"Authorization":       "Bearer " + token,
		auth.ActAsAgentHeader: "agent-one@example.com",
	}
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, models.CreateStatusRequest{
		Status: "should never post",
	})
	require.NoError(t, err)

	requireStatus(t, http.StatusBadRequest)(h.HandleCreateStatusLift(ctx))
	require.False(t, createCalled)
}

func TestActAs_CreateStatus_GrantCheckErrorFailsClosed(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()

	h, _, _ := round11NewHandler(t, cfg, actAsTestState(now), &RegistryStub{
		NotesSvc: actAsCaptureNotesStub(now, new(*notes.CreateNoteCommand)),
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				return false, errors.New("dynamodb timeout")
			},
		},
	})

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
	headers := map[string]string{
		"Authorization":       "Bearer " + token,
		auth.ActAsAgentHeader: "agent-one",
	}
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, models.CreateStatusRequest{
		Status: "should never post",
	})
	require.NoError(t, err)

	requireStatus(t, http.StatusInternalServerError)(h.HandleCreateStatusLift(ctx))
}

func TestActAs_CreateStatus_ScheduledStatusRejected(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()

	h, _, _ := round11NewHandler(t, cfg, actAsTestState(now), &RegistryStub{
		NotesSvc:       actAsCaptureNotesStub(now, new(*notes.CreateNoteCommand)),
		AgentSharesSvc: actAsActiveGrantStub(),
	})

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
	headers := map[string]string{
		"Authorization":       "Bearer " + token,
		auth.ActAsAgentHeader: "agent-one",
	}
	scheduled := now.Add(time.Hour).Format(time.RFC3339)
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, models.CreateStatusRequest{
		Status:      "future post",
		ScheduledAt: &scheduled,
	})
	require.NoError(t, err)

	requireStatus(t, http.StatusBadRequest)(h.HandleCreateStatusLift(ctx))
}
