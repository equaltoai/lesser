package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestStatusesRound19_UnusedHelpers(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("getParentStatusID handles error and validation cases", func(t *testing.T) {
		t.Run("first lookup error returns empty", func(t *testing.T) {
			notesStub := &NotesServiceStub{
				GetNoteFunc: func(context.Context, string) (*storagemodels.Status, error) {
					return nil, errors.New("boom")
				},
			}
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesStub})
			require.Empty(t, h.getParentStatusID(context.Background(), "child"))
		})

		t.Run("missing inReplyTo returns empty", func(t *testing.T) {
			notesStub := &NotesServiceStub{
				GetNoteFunc: func(context.Context, string) (*storagemodels.Status, error) {
					return &storagemodels.Status{Note: &activitypub.Note{}}, nil
				},
			}
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesStub})
			require.Empty(t, h.getParentStatusID(context.Background(), "child"))
		})

		t.Run("parent not found returns empty", func(t *testing.T) {
			notesStub := &NotesServiceStub{
				GetNoteFunc: func(_ context.Context, statusID string) (*storagemodels.Status, error) {
					if statusID == "child" {
						return &storagemodels.Status{Note: &activitypub.Note{BaseObject: activitypub.BaseObject{InReplyTo: "parent"}}}, nil
					}
					return nil, errors.New("not found")
				},
			}
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesStub})
			require.Empty(t, h.getParentStatusID(context.Background(), "child"))
		})

		t.Run("success returns parent id", func(t *testing.T) {
			notesStub := &NotesServiceStub{
				GetNoteFunc: func(_ context.Context, statusID string) (*storagemodels.Status, error) {
					switch statusID {
					case "child":
						return &storagemodels.Status{Note: &activitypub.Note{BaseObject: activitypub.BaseObject{InReplyTo: "parent"}}}, nil
					case "parent":
						return &storagemodels.Status{Note: &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "parent"}}}, nil
					default:
						return nil, errors.New("not found")
					}
				},
			}
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesStub})
			require.Equal(t, "parent", h.getParentStatusID(context.Background(), "child"))
		})
	})

	t.Run("getActorForObject and convertReplyToStatus handle common object shapes", func(t *testing.T) {
		accountsStub := &AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
				if username == "alice" {
					return &storage.Account{
						Actor: &activitypub.Actor{
							BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice"},
							PreferredUsername: "alice",
						},
					}, nil
				}
				return nil, errors.New("not found")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsStub})

		require.Nil(t, h.getActorForObject(context.Background(), &activitypub.Note{}))
		require.Nil(t, h.getActorForObject(context.Background(), &activitypub.Note{AttributedTo: "/"}))

		actor := h.getActorForObject(context.Background(), &activitypub.Note{AttributedTo: "https://example.com/users/alice"})
		require.NotNil(t, actor)
		require.Equal(t, "https://example.com/users/alice", actor.ID)

		remoteActor := h.getActorForObject(context.Background(), &activitypub.Note{AttributedTo: "https://remote.example/users/alice"})
		require.NotNil(t, remoteActor)
		require.Equal(t, "https://remote.example/users/alice", remoteActor.ID)
		require.NotEqual(t, "https://example.com/users/alice", remoteActor.ID)

		replyStatus := h.convertReplyToStatus(context.Background(), &activitypub.Note{
			BaseObject:   activitypub.BaseObject{ID: "https://example.com/objects/s1"},
			AttributedTo: "https://example.com/users/alice",
			Content:      "hello",
		})
		require.NotNil(t, replyStatus)
	})

	t.Run("loadStatusWithActor and convertReplyToStatus use stored remote author seam", func(t *testing.T) {
		now := time.Now().UTC()
		remoteActor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://remote.example/users/steward",
				Type: activitypub.PersonType,
			},
			PreferredUsername: "steward",
			Name:              "Steward Remote",
			URL:               "https://remote.example/@steward",
			Inbox:             "https://remote.example/users/steward/inbox",
			Outbox:            "https://remote.example/users/steward/outbox",
		}
		cachedRemote := storagemodels.RemoteActor{
			Handle:    "steward@remote.example",
			Actor:     remoteActor,
			CachedAt:  now,
			UpdatedAt: now,
			ExpiresAt: now.Add(time.Hour),
		}
		cachedRemote.UpdateKeys()

		remoteStatus := &storagemodels.Status{
			StatusID:       "remote-status",
			AuthorUsername: "steward@remote.example",
			AuthorID:       remoteActor.ID,
			Content:        "remote content",
			Visibility:     storagemodels.VisibilityPublic,
			PublishedAt:    now,
			CreatedAt:      now,
			UpdatedAt:      now,
			Note: &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:        "https://remote.example/users/steward/statuses/remote-status",
					Type:      activitypub.NoteType,
					Published: &now,
				},
				AttributedTo: remoteActor.ID,
				Content:      "remote content",
			},
		}

		notesStub := &NotesServiceStub{
			GetNoteFunc: func(_ context.Context, statusID string) (*storagemodels.Status, error) {
				if statusID == remoteStatus.StatusID {
					return remoteStatus, nil
				}
				return nil, errors.New("not found")
			},
		}

		state := &round10QueryState{
			remoteActorsByPK: map[string]storagemodels.RemoteActor{
				cachedRemote.PK: cachedRemote,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{NotesSvc: notesStub})

		loaded := h.loadStatusWithActor(context.Background(), remoteStatus.StatusID)
		require.NotNil(t, loaded)
		require.Equal(t, remoteActor.ID, loaded.Account.ID)
		require.Equal(t, "steward@remote.example", loaded.Account.Acct)

		replyStatus := h.convertReplyToStatus(context.Background(), remoteStatus)
		require.NotNil(t, replyStatus)
		require.Equal(t, remoteActor.ID, replyStatus.Account.ID)
		require.Equal(t, "steward@remote.example", replyStatus.Account.Acct)
	})

	t.Run("extractInReplyTo and extractFromGenericMap cover common branches", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		require.Equal(t, "parent-1", h.extractInReplyTo(&activitypub.Note{BaseObject: activitypub.BaseObject{InReplyTo: "parent-1"}}))
		require.Equal(t, "parent-2", h.extractInReplyTo(map[string]any{"inReplyTo": "parent-2"}))
		require.Equal(t, h.cfg.BaseURL()+"/objects/p3", h.extractInReplyTo(&storagemodels.Status{InReplyToID: "p3"}))

		type withString struct{ InReplyTo string }
		require.Equal(t, "parent-4", h.extractInReplyTo(withString{InReplyTo: "parent-4"}))

		parent5 := "parent-5"
		type withPtr struct{ InReplyTo *string }
		require.Equal(t, "parent-5", h.extractInReplyTo(&withPtr{InReplyTo: &parent5}))
		require.Empty(t, h.extractInReplyTo(123))

		require.Equal(t, []string{"a", "b"}, h.extractFromGenericMap(map[string]any{"hashtags": []string{"A", "b"}}))
		require.Equal(t, []string{"go"}, h.extractFromGenericMap(map[string]any{"tag": []any{map[string]any{"type": "Hashtag", "name": "#Go"}}}))
		require.Empty(t, h.extractFromGenericMap(map[string]any{}))
	})
}
