package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap/zaptest"
)

func TestStatusesRound14_NormalizeAndConvertHelpers(t *testing.T) {
	cfg := round10TestConfig()
	h := &Handler{cfg: cfg, logger: zaptest.NewLogger(t)}

	t.Run("normalizeStatusIDForUpdate prefixes local ids", func(t *testing.T) {
		require.Contains(t, h.normalizeStatusIDForUpdate("abc123"), "/objects/abc123")
		require.Equal(t, "https://remote.example/objects/abc123", h.normalizeStatusIDForUpdate("https://remote.example/objects/abc123"))
	})

	t.Run("convertObjectToNoteWithOwnershipCheck handles common object shapes", func(t *testing.T) {
		ctx := &apptheory.Context{}
		actorID := cfg.ActorURL("alice")

		note := &activitypub.Note{AttributedTo: "someone-else"}
		_, resp, err := h.convertObjectToNoteWithOwnershipCheck(ctx, note, actorID)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)

		st := &models.Status{StatusID: "s1", AuthorID: "someone-else", Note: &activitypub.Note{AttributedTo: "someone-else"}}
		_, resp, err = h.convertObjectToNoteWithOwnershipCheck(ctx, st, actorID)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)

		st = &models.Status{StatusID: "s2", AuthorUsername: "bob", Note: &activitypub.Note{AttributedTo: actorID}}
		_, resp, err = h.convertObjectToNoteWithOwnershipCheck(ctx, st, actorID)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)

		st = &models.Status{StatusID: "s3", AuthorID: actorID, Note: nil}
		_, resp, err = h.convertObjectToNoteWithOwnershipCheck(ctx, st, actorID)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)

		_, resp, err = h.convertObjectToNoteWithOwnershipCheck(ctx, map[string]any{"attributedTo": "someone-else"}, actorID)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)

		_, resp, err = h.convertObjectToNoteWithOwnershipCheck(ctx, map[string]any{"attributedTo": actorID, "bad": make(chan int)}, actorID)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)

		type noAttr struct{ ID string }
		_, resp, err = h.convertObjectToNoteWithOwnershipCheck(ctx, noAttr{ID: "x"}, actorID)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)

		type attrStruct struct {
			AttributedTo string `json:"attributedTo"`
			Content      any    `json:"content,omitempty"`
		}
		_, resp, err = h.convertObjectToNoteWithOwnershipCheck(ctx, attrStruct{AttributedTo: "someone-else"}, actorID)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)

		_, resp, err = h.convertObjectToNoteWithOwnershipCheck(ctx, attrStruct{AttributedTo: actorID, Content: make(chan int)}, actorID)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)

		converted, resp, err := h.convertObjectToNoteWithOwnershipCheck(ctx, attrStruct{AttributedTo: actorID, Content: "hello"}, actorID)
		require.NoError(t, err)
		require.Nil(t, resp)
		require.NotNil(t, converted)
	})
}

func TestStatusesRound14_ExtractInReplyToBranches(t *testing.T) {
	cfg := round10TestConfig()
	h := &Handler{cfg: cfg}

	t.Run("storage status falls back to in_reply_to_id", func(t *testing.T) {
		st := &models.Status{InReplyToID: "parent", Note: nil}
		require.Contains(t, h.extractInReplyTo(st), "/objects/parent")
	})

	t.Run("map extracts inReplyTo", func(t *testing.T) {
		require.Equal(t, "https://example.com/objects/p", h.extractInReplyTo(map[string]any{"inReplyTo": "https://example.com/objects/p"}))
	})

	t.Run("reflection extracts InReplyTo field", func(t *testing.T) {
		type withPtr struct{ InReplyTo *string }
		v := "https://example.com/objects/p2"
		require.Equal(t, v, h.extractInReplyTo(&withPtr{InReplyTo: &v}))
	})
}

func TestStatusesRound14_ValidateStatusIDForContext(t *testing.T) {
	cfg := round10TestConfig()

	t.Run("success returns normalized object id", func(t *testing.T) {
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetNoteFunc: func(_ context.Context, statusID string) (*models.Status, error) {
					return &models.Status{StatusID: statusID}, nil
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, reg)

		ctx := &apptheory.Context{Params: map[string]string{"id": "abc"}}
		objectID, resp, err := h.validateStatusIDForContext(ctx)
		require.NoError(t, err)
		require.Nil(t, resp)
		require.Contains(t, objectID, "/objects/abc")
	})

	t.Run("not found returns 404 when tombstone missing", func(t *testing.T) {
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetNoteFunc: func(context.Context, string) (*models.Status, error) {
					return nil, errors.New("missing")
				},
			},
		}

		objectID := cfg.BaseURL() + "/objects/missing"
		state := &round10QueryState{
			notFoundPKSK: map[string]bool{
				"OBJECT#" + objectID + "#TOMBSTONE": true,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, reg)

		ctx := &apptheory.Context{Params: map[string]string{"id": "missing"}}
		_, resp, err := h.validateStatusIDForContext(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("tombstoned status returns 410 gone with details", func(t *testing.T) {
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetNoteFunc: func(context.Context, string) (*models.Status, error) {
					return nil, errors.New("gone")
				},
			},
		}

		objectID := cfg.BaseURL() + "/objects/dead"
		state := &round10QueryState{
			tombstonesByObjectID: map[string]models.Tombstone{
				objectID: {
					ID:         objectID,
					FormerType: activitypub.NoteType,
					Deleted:    time.Now().Add(-1 * time.Hour),
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, reg)

		ctx := &apptheory.Context{Params: map[string]string{"id": "dead"}}
		_, resp, err := h.validateStatusIDForContext(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusGone, resp.Status)
	})
}

