package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/agentshare"
	"github.com/equaltoai/lesser/pkg/services/notes"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/golang-jwt/jwt/v5"
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
		agentGovernanceByUsername: map[string]storagemodels.AgentGovernanceState{
			"agent-one": {
				PK:        "USER#agent-one",
				SK:        storagemodels.SKAgentGovernance,
				Username:  "agent-one",
				Verified:  true,
				CreatedAt: now.Add(-24 * time.Hour),
				UpdatedAt: now.Add(-1 * time.Hour),
			},
		},
	}
}

// actAsTestStateWithOwner seeds agent-one with the given AgentOwner form.
func actAsTestStateWithOwner(now time.Time, agentOwner string) *round10QueryState {
	state := actAsTestState(now)
	agent := state.usersByUsername["agent-one"]
	agent.AgentOwner = agentOwner
	state.usersByUsername["agent-one"] = agent
	return state
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

// actAsAgentSubjectToken signs the shipped agent-subject shape: the token subject is
// always the agent and the authorizing human rides in DelegatedBy.
func actAsAgentSubjectToken(t *testing.T, secret, agent, delegatedBy string, scopes []string) string {
	t.Helper()
	now := time.Now()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   agent,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		Username:    agent,
		ClientID:    "test-client",
		Scopes:      scopes,
		IsAgent:     true,
		DelegatedBy: delegatedBy,
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func actAsWriteScopes() []string {
	return []string{auth.ScopeRead, auth.ScopeWrite}
}

func TestActAs_CreateStatus_GranteeActsAgentScopedWithAttribution(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()
	state := actAsTestState(now)

	var gotCmd *notes.CreateNoteCommand
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		NotesSvc:       actAsCaptureNotesStub(now, &gotCmd),
		AgentSharesSvc: actAsActiveGrantStub(),
	})

	// The real OAuth path issues an agent-subject token with the authorizing grantee
	// in DelegatedBy; lesser-body threads X-Lesser-Act-As: {agent}.
	token := actAsAgentSubjectToken(t, cfg.JWTSecret, "agent-one", "@alice", actAsWriteScopes())
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

	// Agent-subject requests emit both the agent audit event (delegated_by) and the
	// act-as attribution event (acted_by). Find and assert the act-as attribution event.
	var actAsAudit *storagemodels.AuthAuditLog
	for _, entry := range state.auditLogsByUser["agent-one"] {
		if strings.Contains(entry.Metadata, `"acted_by":"alice"`) {
			actAsAudit = entry
			break
		}
	}
	require.NotNil(t, actAsAudit, "act-as audit event must carry caller attribution")
	require.Equal(t, "agent.status.create", actAsAudit.EventType)
	require.Contains(t, actAsAudit.Metadata, `"agent_username":"agent-one"`)
	require.Contains(t, actAsAudit.Metadata, `"target_id":"s-act-as-1"`)
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

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", actAsWriteScopes())
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

func TestActAs_CreateStatus_RevokedGrantDeniedOnNextRequest(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()

	grantActive := true
	createCalled := 0
	h, _, _ := round11NewHandler(t, cfg, actAsTestState(now), &RegistryStub{
		NotesSvc: &NotesServiceStub{
			CreateNoteFunc: func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) {
				createCalled++
				return &notes.NoteResult{Note: &storagemodels.Status{
					StatusID:       "s-revoked-1",
					AuthorUsername: "agent-one",
					AuthorID:       "https://example.com/users/agent-one",
					PublishedAt:    now,
					CreatedAt:      now,
					UpdatedAt:      now,
					ModifiedAt:     now,
					Version:        1,
				}}, nil
			},
		},
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(_ context.Context, agent, grantee string) (bool, error) {
				return grantActive, nil
			},
		},
	})

	token := actAsAgentSubjectToken(t, cfg.JWTSecret, "agent-one", "@alice", actAsWriteScopes())
	headers := map[string]string{
		"Authorization":       "Bearer " + token,
		auth.ActAsAgentHeader: "agent-one",
	}

	firstCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, models.CreateStatusRequest{Status: "before revoke"})
	require.NoError(t, err)
	requireStatus(t, http.StatusCreated)(h.HandleCreateStatusLift(firstCtx))
	require.Equal(t, 1, createCalled)

	// Revocation must take effect on the very next request — same token, no refresh.
	grantActive = false

	secondCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, models.CreateStatusRequest{Status: "after revoke"})
	require.NoError(t, err)
	requireStatus(t, http.StatusForbidden)(h.HandleCreateStatusLift(secondCtx))
	require.Equal(t, 1, createCalled, "revoked grant must not reach note creation")
}

func TestActAs_CreateStatus_OwnerActAsDenied(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()

	var checkedGrantee string
	createCalled := false
	h, _, _ := round11NewHandler(t, cfg, actAsTestStateWithOwner(now, "@owner"), &RegistryStub{
		NotesSvc: &NotesServiceStub{
			CreateNoteFunc: func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) {
				createCalled = true
				return nil, errors.New("must not be called")
			},
		},
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(_ context.Context, agent, grantee string) (bool, error) {
				checkedGrantee = grantee
				// Owners never hold a self-grant; resolution stays uniform (no owner
				// special case) and the grant check returns false.
				return false, nil
			},
		},
	})

	token := actAsAgentSubjectToken(t, cfg.JWTSecret, "agent-one", "@owner", actAsWriteScopes())
	headers := map[string]string{
		"Authorization":       "Bearer " + token,
		auth.ActAsAgentHeader: "agent-one",
	}
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, models.CreateStatusRequest{
		Status: "owner cannot act-as their own agent",
	})
	require.NoError(t, err)

	requireStatus(t, http.StatusForbidden)(h.HandleCreateStatusLift(ctx))
	require.False(t, createCalled)
	require.Equal(t, "owner", checkedGrantee, "owner path must go through the same grant check as any non-grantee")
}

func TestActAs_CreateStatus_URLFormOwnerActAsDenied(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()

	createCalled := false
	h, _, _ := round11NewHandler(t, cfg, actAsTestStateWithOwner(now, "https://example.com/users/owner"), &RegistryStub{
		NotesSvc: &NotesServiceStub{
			CreateNoteFunc: func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) {
				createCalled = true
				return nil, errors.New("must not be called")
			},
		},
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				t.Fatal("URL-form owner DelegatedBy must fail closed before the grant check")
				return false, nil
			},
		},
	})

	// The real mint path stores the URL-form owner with the leading "@" that
	// normalizeDelegatedBy prepends to non-"@" values.
	token := actAsAgentSubjectToken(t, cfg.JWTSecret, "agent-one", "@https://example.com/users/owner", actAsWriteScopes())
	headers := map[string]string{
		"Authorization":       "Bearer " + token,
		auth.ActAsAgentHeader: "agent-one",
	}
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, models.CreateStatusRequest{
		Status: "URL-form owner cannot act-as their own agent",
	})
	require.NoError(t, err)

	requireStatus(t, http.StatusForbidden)(h.HandleCreateStatusLift(ctx))
	require.False(t, createCalled)
}

func TestActAs_CreateStatus_BlankDelegatedByDenied(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()

	h, _, _ := round11NewHandler(t, cfg, actAsTestState(now), &RegistryStub{
		NotesSvc: actAsCaptureNotesStub(now, new(*notes.CreateNoteCommand)),
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				t.Fatal("blank DelegatedBy must fail closed before the grant check")
				return false, nil
			},
		},
	})

	for _, delegatedBy := range []string{"", "   "} {
		token := actAsAgentSubjectToken(t, cfg.JWTSecret, "agent-one", delegatedBy, actAsWriteScopes())
		headers := map[string]string{
			"Authorization":       "Bearer " + token,
			auth.ActAsAgentHeader: "agent-one",
		}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, models.CreateStatusRequest{
			Status: "should never post",
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.HandleCreateStatusLift(ctx))
	}
}

func TestActAs_CreateStatus_UnknownAndNonAgentTargetDenied(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()

	state := actAsTestState(now)
	state.usersByUsername["human"] = storagemodels.User{
		PK:        "USER#human",
		SK:        storagemodels.SKMetadata,
		Username:  "human",
		Role:      "user",
		Approved:  true,
		Version:   1,
		CreatedAt: now.Add(-24 * time.Hour),
	}

	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		NotesSvc: actAsCaptureNotesStub(now, new(*notes.CreateNoteCommand)),
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				t.Fatal("unknown/non-agent target must fail closed before the grant check")
				return false, nil
			},
		},
	})

	token := actAsAgentSubjectToken(t, cfg.JWTSecret, "agent-one", "@alice", actAsWriteScopes())

	for _, indicator := range []string{"ghost", "human"} {
		headers := map[string]string{
			"Authorization":       "Bearer " + token,
			auth.ActAsAgentHeader: indicator,
		}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, models.CreateStatusRequest{
			Status: "should never post",
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleCreateStatusLift(ctx))
	}
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

	token := actAsAgentSubjectToken(t, cfg.JWTSecret, "agent-one", "@alice", actAsWriteScopes())
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

	token := actAsAgentSubjectToken(t, cfg.JWTSecret, "agent-one", "@alice", actAsWriteScopes())
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

	token := actAsAgentSubjectToken(t, cfg.JWTSecret, "agent-one", "@alice", actAsWriteScopes())
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
