package handlers

import (
	"context"
	"errors"
	"testing"

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

		replyStatus := h.convertReplyToStatus(context.Background(), &activitypub.Note{
			BaseObject:   activitypub.BaseObject{ID: "https://example.com/objects/s1"},
			AttributedTo: "https://example.com/users/alice",
			Content:      "hello",
		})
		require.NotNil(t, replyStatus)
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
