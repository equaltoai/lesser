package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestHelpersRound18_ResolveAccountID_CacheHits(t *testing.T) {
	cfg := round11TestConfig()

	now := time.Now()
	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/alice",
			Type: "Person",
		},
		PreferredUsername: "alice",
		Name:              "Alice",
		Inbox:             "https://remote.example/users/alice/inbox",
		Outbox:            "https://remote.example/users/alice/outbox",
	}

	cached := storagemodels.RemoteActor{
		Handle:    "alice@remote.example",
		Actor:     remoteActor,
		ExpiresAt: now.Add(1 * time.Hour),
		CachedAt:  now.Add(-1 * time.Minute),
		UpdatedAt: now.Add(-1 * time.Minute),
	}
	cached.UpdateKeys()

	state := &round10QueryState{
		remoteActorsByPK: map[string]storagemodels.RemoteActor{
			cached.PK: cached,
		},
	}
	h, _, _ := round11NewHandler(t, cfg, state)

	t.Run("remote actor URL uses cached remote actor", func(t *testing.T) {
		actor, err := h.resolveAccountID(context.Background(), "https://remote.example/users/alice")
		require.NoError(t, err)
		require.NotNil(t, actor)
		require.Equal(t, remoteActor.ID, actor.ID)
	})

	t.Run("remote handle uses cached remote actor", func(t *testing.T) {
		actor, err := h.resolveAccountID(context.Background(), "alice@remote.example")
		require.NoError(t, err)
		require.NotNil(t, actor)
		require.Equal(t, remoteActor.ID, actor.ID)
	})

	t.Run("local handle resolves to local actor", func(t *testing.T) {
		actor, err := h.resolveAccountID(context.Background(), "alice@"+cfg.Domain)
		require.NoError(t, err)
		require.NotNil(t, actor)
		require.Equal(t, cfg.ActorURL("alice"), actor.ID)
	})

	t.Run("double escaped local actor URL is normalized before lookup", func(t *testing.T) {
		actor, err := h.resolveAccountID(context.Background(), "https:%252F%252F"+cfg.Domain+"%252Fusers%252Falice")
		require.NoError(t, err)
		require.NotNil(t, actor)
		require.Equal(t, cfg.ActorURL("alice"), actor.ID)
	})
}

func TestHelpersRound18_ResolveAccountID_InvalidURLBranches(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("URL without hostname rejects", func(t *testing.T) {
		_, err := h.resolveAccountID(context.Background(), "https:///users/alice")
		require.Error(t, err)
	})

	t.Run("local URL without /users/{username} rejects", func(t *testing.T) {
		_, err := h.resolveAccountID(context.Background(), cfg.BaseURL()+"/@alice")
		require.Error(t, err)
	})
}

func TestHelpersRound18_NormalizeDelegatedByActorURI(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("returns empty string unchanged", func(t *testing.T) {
		var h *Handler
		require.Empty(t, h.normalizeDelegatedByActorURI("   "))
	})

	t.Run("returns trimmed value when handler config missing", func(t *testing.T) {
		h := &Handler{}
		require.Equal(t, "owner", h.normalizeDelegatedByActorURI("  owner  "))
	})

	t.Run("passes through actor urls", func(t *testing.T) {
		h := &Handler{cfg: cfg}
		require.Equal(t, "HTTPS://remote.example/users/owner", h.normalizeDelegatedByActorURI("HTTPS://remote.example/users/owner"))
	})

	t.Run("normalizes local usernames to actor urls", func(t *testing.T) {
		h := &Handler{cfg: cfg}
		require.Equal(t, cfg.ActorURL("owner"), h.normalizeDelegatedByActorURI("@owner"))
	})
}

func TestHelpersRound18_BuildStatusAgentAttribution(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		agentGovernanceByUsername: map[string]storagemodels.AgentGovernanceState{
			"bot": {
				Username:        "bot",
				DelegatedScopes: []string{"read", "write:statuses"},
			},
		},
	})

	t.Run("non agent returns nil", func(t *testing.T) {
		out := h.buildStatusAgentAttribution(context.Background(), &storage.Account{User: &storage.User{Username: "alice"}}, nil)
		require.Nil(t, out)
	})

	t.Run("agent uses attribution from note when present", func(t *testing.T) {
		status := &storagemodels.Status{
			Note: &activitypub.Note{
				AgentAttribution: &activitypub.AgentPostAttribution{
					TriggerType:     "mention",
					TriggerDetails:  "test",
					MemoryCitations: []string{"m1"},
					DelegatedBy:     "operator",
					Scopes:          []string{"read"},
					Constraints:     []string{"requires_approval"},
					SchemaVersion:   activitypub.AgentAttributionSchemaVersion,
					ModelID:         "v2",
				},
			},
		}

		account := &storage.Account{
			User: &storage.User{IsAgent: true},
		}

		out := h.buildStatusAgentAttribution(context.Background(), account, status)
		require.NotNil(t, out)
		require.Equal(t, "mention", out.TriggerType)
		require.Equal(t, "test", out.TriggerDetails)
		require.Equal(t, []string{"m1"}, out.MemoryCitations)
		require.Equal(t, cfg.ActorURL("operator"), out.DelegatedBy)
		require.Equal(t, []string{"read"}, out.Scopes)
		require.Equal(t, []string{"requires_approval"}, out.Constraints)
		require.Equal(t, activitypub.AgentAttributionSchemaVersion, out.SchemaVersion)
		require.Equal(t, "v2", out.ModelID)
	})

	t.Run("agent falls back to stored delegation and constraints", func(t *testing.T) {
		account := &storage.Account{
			User: &storage.User{
				Username:     "bot",
				IsAgent:      true,
				AgentOwner:   "",
				AgentVersion: "",
				AgentCapabilities: &agents.Capabilities{
					MaxPostsPerHour:   5,
					RequiresApproval:  true,
					RestrictedDomains: []string{"example.org"},
				},
			},
			Actor: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("bot"), Type: "Service"},
				AgentManifest: &activitypub.AgentManifest{
					OperatedBy: "manifest-owner",
					Version:    "manifest-v1",
				},
				PreferredUsername: "bot",
			},
		}

		out := h.buildStatusAgentAttribution(context.Background(), account, &storagemodels.Status{Note: &activitypub.Note{}})
		require.NotNil(t, out)
		require.Equal(t, cfg.ActorURL("manifest-owner"), out.DelegatedBy)
		require.Equal(t, []string{"read", "write:statuses"}, out.Scopes)
		require.Contains(t, out.Constraints, "max_posts_per_hour:5")
		require.Contains(t, out.Constraints, "requires_approval")
		require.Contains(t, out.Constraints, "restricted_domains:example.org")
		require.Equal(t, activitypub.AgentAttributionSchemaVersion, out.SchemaVersion)
		require.Equal(t, "manifest-v1", out.ModelID)
	})

	t.Run("agent keeps delegated did and falls back to account metadata", func(t *testing.T) {
		account := &storage.Account{
			User: &storage.User{
				Username:     "bot",
				IsAgent:      true,
				AgentOwner:   "https://remote.example/users/owner",
				AgentVersion: "user-v2",
			},
		}

		status := &storagemodels.Status{
			Note: &activitypub.Note{
				AgentAttribution: &activitypub.AgentPostAttribution{
					DelegatedByDID: "did:key:z6Mkexample",
				},
			},
		}

		hRemote, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			agentGovernanceByUsername: map[string]storagemodels.AgentGovernanceState{
				"bot": {
					Username:        "bot",
					DelegatedScopes: []string{"read"},
				},
			},
		})

		out := hRemote.buildStatusAgentAttribution(context.Background(), account, status)
		require.NotNil(t, out)
		require.Equal(t, "https://remote.example/users/owner", out.DelegatedBy)
		require.Equal(t, "did:key:z6Mkexample", out.DelegatedByDID)
		require.Equal(t, []string{"read"}, out.Scopes)
		require.Equal(t, activitypub.AgentAttributionSchemaVersion, out.SchemaVersion)
		require.Equal(t, "user-v2", out.ModelID)
	})

	t.Run("agent fills identity semantics from workflow metadata and soul binding", func(t *testing.T) {
		metadata, err := agents.SetDroneWorkflowMetadata(nil, &agents.DroneWorkflowState{
			CurrentPhase: agents.DroneWorkflowPhaseContinuity,
			CurrentState: agents.DroneWorkflowStateContinuityStable,
			SoulAgentID:  "0xagent-soul",
		})
		require.NoError(t, err)

		state := &round10QueryState{
			soulBodyBindingsByAgentID: map[string]storagemodels.InstanceSoulBodyBinding{
				"0xagent-soul": *storagemodels.NewInstanceSoulBodyBinding("0xagent-soul", "bot", "0xprincipal"),
			},
			soulBodyBindingUsernames: map[string]storagemodels.InstanceSoulBodyBindingUsername{
				"bot": *storagemodels.NewInstanceSoulBodyBindingUsername("bot", "0xagent-soul"),
			},
		}

		hIdentity, _, _ := round11NewHandler(t, cfg, state)
		account := &storage.Account{
			User: &storage.User{
				Username:     "bot",
				IsAgent:      true,
				AgentOwner:   "@owner",
				AgentVersion: "v3",
				Metadata:     metadata,
			},
		}

		out := hIdentity.buildStatusAgentAttribution(context.Background(), account, &storagemodels.Status{Note: &activitypub.Note{}})
		require.NotNil(t, out)
		require.Equal(t, agents.DroneIdentityStateSouled, out.IdentityState)
		require.Equal(t, "Souled", out.IdentityLabel)
		require.Equal(t, agents.DroneContinuityStateStable, out.ContinuityState)
		require.Equal(t, "0xagent-soul", out.SoulAgentID)
		require.Equal(t, "Souled", out.ModerationLabel)
	})
}

func TestHelpersRound18_StatusAccountUsesActorData(t *testing.T) {
	cfg := round11TestConfig()
	h := &Handler{cfg: cfg}
	createdAt := time.Now().Add(-2 * time.Hour).UTC()

	account := h.statusAccount(&storagemodels.Status{
		StatusID:       "status-1",
		AuthorUsername: "alice",
		AuthorID:       cfg.ActorURL("alice"),
	}, &storage.Account{
		User: &storage.User{
			Username:    "alice",
			DisplayName: "Alice",
			CreatedAt:   createdAt,
		},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   cfg.ActorURL("alice"),
				Type: "Person",
			},
			PreferredUsername: "alice",
		},
	})

	require.Equal(t, common.GenerateNumericID("alice"), account.ID)
	require.Equal(t, "alice", account.Username)
	require.Equal(t, "alice", account.Acct)
	require.Equal(t, "Alice", account.DisplayName)
	require.Equal(t, createdAt.Format(time.RFC3339), account.CreatedAt)
}

func TestHelpersRound18_StatusAccountFallbackBranches(t *testing.T) {
	cfg := round11TestConfig()
	h := &Handler{cfg: cfg}

	t.Run("builds local fallback account when author account missing", func(t *testing.T) {
		account := h.statusAccount(&storagemodels.Status{
			StatusID:       "status-2",
			AuthorUsername: "bot",
			AuthorID:       cfg.ActorURL("bot"),
		}, nil)

		require.Equal(t, "bot", account.Username)
		require.Equal(t, "bot", account.DisplayName)
		require.Equal(t, "bot", account.Acct)
	})

	t.Run("returns remote actor account when actor is not local", func(t *testing.T) {
		account := h.statusAccount(&storagemodels.Status{
			StatusID:       "status-3",
			AuthorUsername: "alice",
			AuthorID:       "https://remote.example/users/alice",
		}, &storage.Account{
			User: &storage.User{
				Username:    "alice",
				DisplayName: "Alice Remote",
			},
			Actor: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID:   "https://remote.example/users/alice",
					Type: "Person",
				},
				PreferredUsername: "alice",
				Name:              "Alice Remote",
				URL:               "https://remote.example/@alice",
			},
		})

		require.Equal(t, "alice", account.Username)
		require.Equal(t, "alice@remote.example", account.Acct)
		require.Equal(t, "Alice Remote", account.DisplayName)
	})
}

func TestHelpersRound18_StatusReblogTargetID(t *testing.T) {
	require.Equal(t, "reblog", statusReblogTargetID(&storagemodels.Status{ReblogOfID: "reblog"}))
	require.Equal(t, "boost", statusReblogTargetID(&storagemodels.Status{BoostOfStatusID: "boost"}))
	require.Empty(t, statusReblogTargetID(nil))
}

func TestHelpersRound18_StatusHelpersHandleNilInputs(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	authorAccount := h.loadStatusAuthorAccount(context.Background(), nil)
	require.NotNil(t, authorAccount)
	require.NotNil(t, authorAccount.User)
	require.Empty(t, authorAccount.User.Username)

	require.False(t, h.statusBookmarked(context.Background(), nil, "alice"))
	require.Nil(t, h.loadReblogStatus(context.Background(), nil, "alice"))
}

func TestHelpersRound34_LoadStoredStatusAuthorAccount_RemotePaths(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/alice",
			Type: "Person",
		},
		PreferredUsername: "alice",
		Name:              "Alice Remote",
		URL:               "https://remote.example/@alice",
		Inbox:             "https://remote.example/users/alice/inbox",
		Outbox:            "https://remote.example/users/alice/outbox",
	}

	cached := storagemodels.RemoteActor{
		Handle:    "alice@remote.example",
		Actor:     remoteActor,
		ExpiresAt: now.Add(1 * time.Hour),
		CachedAt:  now.Add(-1 * time.Minute),
		UpdatedAt: now.Add(-1 * time.Minute),
	}
	cached.UpdateKeys()

	t.Run("uses cached remote actor without local account projection", func(t *testing.T) {
		state := &round10QueryState{
			remoteActorsByPK: map[string]storagemodels.RemoteActor{
				cached.PK: cached,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		account := h.loadStoredStatusAuthorAccount(context.Background(), &storagemodels.Status{
			StatusID:       "status-1",
			AuthorID:       remoteActor.ID,
			AuthorUsername: "alice@remote.example",
			Note:           &activitypub.Note{AttributedTo: remoteActor.ID},
		})

		require.NotNil(t, account)
		require.NotNil(t, account.Actor)
		require.Equal(t, remoteActor.ID, account.Actor.ID)
		require.Equal(t, "alice@remote.example", account.User.Username)
	})

	t.Run("degrades from stored identity when remote actor cache is absent", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		account := h.loadStoredStatusAuthorAccount(context.Background(), &storagemodels.Status{
			StatusID:       "status-2",
			AuthorID:       "https://remote.example/users/bob",
			AuthorUsername: "bob@remote.example",
			Note:           &activitypub.Note{AttributedTo: "https://remote.example/users/bob"},
		})

		require.NotNil(t, account)
		require.NotNil(t, account.User)
		require.Equal(t, "bob@remote.example", account.User.Username)
		require.Equal(t, "bob@remote.example", account.User.DisplayName)
		require.Nil(t, account.Actor)
	})
}

func TestHelpersRound18_ResponseDefaults(t *testing.T) {
	h := &Handler{logger: round10TestLogger(t)}

	ctxBad, err := round10NewLiftContext(http.MethodGet, "/test", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusBadRequest)(h.respondBadRequest(ctxBad, ""))

	ctxConflict, err := round10NewLiftContext(http.MethodGet, "/test", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusConflict)(h.respondConflict(ctxConflict, ""))
}
