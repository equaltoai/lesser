package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestStatusesMoreCoverageHelpers_Round12(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("normalizeStatusIDForUpdate", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg)
		require.Equal(t, cfg.BaseURL()+"/objects/s1", handler.normalizeStatusIDForUpdate("s1"))
		require.Equal(t, "https://remote.example/objects/123", handler.normalizeStatusIDForUpdate("https://remote.example/objects/123"))
	})

	t.Run("extractInReplyTo", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg)

		note := &activitypub.Note{BaseObject: activitypub.BaseObject{InReplyTo: cfg.ObjectURL("objects", "parent")}}
		require.Equal(t, note.InReplyTo, handler.extractInReplyTo(note))

		statusWithEmbeddedNote := &storagemodels.Status{
			StatusID: "s1",
			Note:     &activitypub.Note{BaseObject: activitypub.BaseObject{InReplyTo: cfg.ObjectURL("objects", "p1")}},
		}
		require.Equal(t, cfg.ObjectURL("objects", "p1"), handler.extractInReplyTo(statusWithEmbeddedNote))

		statusWithInReplyToID := &storagemodels.Status{
			StatusID:    "s2",
			InReplyToID: "p2",
		}
		require.Equal(t, cfg.ObjectURL("objects", "p2"), handler.extractInReplyTo(statusWithInReplyToID))

		statusNoReply := &storagemodels.Status{StatusID: "s3"}
		require.Equal(t, "", handler.extractInReplyTo(statusNoReply))

		objMap := map[string]any{"inReplyTo": "https://example.net/objects/x"}
		require.Equal(t, "https://example.net/objects/x", handler.extractInReplyTo(objMap))

		type replyString struct {
			InReplyTo string
		}
		require.Equal(t, "https://example.net/objects/y", handler.extractInReplyTo(replyString{InReplyTo: "https://example.net/objects/y"}))

		replyPtr := "https://example.net/objects/z"
		type replyPtrString struct {
			InReplyTo *string
		}
		require.Equal(t, replyPtr, handler.extractInReplyTo(replyPtrString{InReplyTo: &replyPtr}))
		require.Equal(t, "", handler.extractInReplyTo(replyPtrString{}))
		require.Equal(t, "", handler.extractInReplyTo("not-a-struct"))
	})

	t.Run("convertObjectToNoteWithOwnershipCheck covers branches", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg)
		actorID := cfg.ActorURL("alice")

		ctx, err := round10NewLiftContext(http.MethodGet, "/statuses", nil, nil, nil)
		require.NoError(t, err)

		otherNote := &activitypub.Note{AttributedTo: cfg.ActorURL("bob")}
		note, resp, err := handler.convertObjectToNoteWithOwnershipCheck(ctx, otherNote, actorID)
		require.NoError(t, err)
		require.Nil(t, note)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)

		ctx2, err := round10NewLiftContext(http.MethodGet, "/statuses", nil, nil, nil)
		require.NoError(t, err)

		ownedNote := &activitypub.Note{
			BaseObject:   activitypub.BaseObject{ID: cfg.ObjectURL("objects", "s1"), Type: activitypub.NoteType},
			AttributedTo: actorID,
			Content:      "hello",
		}
		note, resp, err = handler.convertObjectToNoteWithOwnershipCheck(ctx2, ownedNote, actorID)
		require.NoError(t, err)
		require.Nil(t, resp)
		require.NotNil(t, note)
		require.Equal(t, "hello", note.Content)

		ctx3, err := round10NewLiftContext(http.MethodGet, "/statuses", nil, nil, nil)
		require.NoError(t, err)

		statusOtherAuthor := &storagemodels.Status{
			StatusID:       "s2",
			AuthorID:       cfg.ActorURL("bob"),
			AuthorUsername: "bob",
			Note:           ownedNote,
		}
		note, resp, err = handler.convertObjectToNoteWithOwnershipCheck(ctx3, statusOtherAuthor, actorID)
		require.NoError(t, err)
		require.Nil(t, note)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)

		ctx4, err := round10NewLiftContext(http.MethodGet, "/statuses", nil, nil, nil)
		require.NoError(t, err)

		statusUsernameMismatch := &storagemodels.Status{
			StatusID:       "s3",
			AuthorUsername: "bob",
			Note:           ownedNote,
		}
		note, resp, err = handler.convertObjectToNoteWithOwnershipCheck(ctx4, statusUsernameMismatch, actorID)
		require.NoError(t, err)
		require.Nil(t, note)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)

		ctx5, err := round10NewLiftContext(http.MethodGet, "/statuses", nil, nil, nil)
		require.NoError(t, err)

		statusMissingNote := &storagemodels.Status{StatusID: "s4", AuthorUsername: "alice"}
		note, resp, err = handler.convertObjectToNoteWithOwnershipCheck(ctx5, statusMissingNote, actorID)
		require.NoError(t, err)
		require.Nil(t, note)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)

		ctx6, err := round10NewLiftContext(http.MethodGet, "/statuses", nil, nil, nil)
		require.NoError(t, err)

		statusNoteMismatch := &storagemodels.Status{
			StatusID:       "s5",
			AuthorUsername: "alice",
			AuthorID:       actorID,
			Note:           &activitypub.Note{AttributedTo: cfg.ActorURL("bob")},
		}
		note, resp, err = handler.convertObjectToNoteWithOwnershipCheck(ctx6, statusNoteMismatch, actorID)
		require.NoError(t, err)
		require.Nil(t, note)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)

		ctx7, err := round10NewLiftContext(http.MethodGet, "/statuses", nil, nil, nil)
		require.NoError(t, err)
		note, resp, err = handler.convertObjectToNoteWithOwnershipCheck(ctx7, map[string]any{"attributedTo": cfg.ActorURL("bob")}, actorID)
		require.NoError(t, err)
		require.Nil(t, note)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)

		ctx8, err := round10NewLiftContext(http.MethodGet, "/statuses", nil, nil, nil)
		require.NoError(t, err)
		note, resp, err = handler.convertObjectToNoteWithOwnershipCheck(ctx8, map[string]any{
			"id":           cfg.ObjectURL("objects", "m1"),
			"type":         activitypub.NoteType,
			"attributedTo": actorID,
			"content":      "map",
		}, actorID)
		require.NoError(t, err)
		require.Nil(t, resp)
		require.NotNil(t, note)
		require.Equal(t, "map", note.Content)

		type noteLike struct {
			ID           string `json:"id"`
			Type         string `json:"type"`
			AttributedTo string `json:"attributedTo"`
			Content      string `json:"content"`
		}
		ctx9, err := round10NewLiftContext(http.MethodGet, "/statuses", nil, nil, nil)
		require.NoError(t, err)
		note, resp, err = handler.convertObjectToNoteWithOwnershipCheck(ctx9, noteLike{
			ID:           cfg.ObjectURL("objects", "u1"),
			Type:         activitypub.NoteType,
			AttributedTo: actorID,
			Content:      "unknown",
		}, actorID)
		require.NoError(t, err)
		require.Nil(t, resp)
		require.NotNil(t, note)
		require.Equal(t, "unknown", note.Content)
	})

	t.Run("object filters handle map branches", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg)

		require.True(t, handler.objectHasMedia(map[string]any{"attachment": []any{map[string]any{"url": "x"}}}))
		require.False(t, handler.objectHasMedia(map[string]any{"attachment": []any{}}))
		require.False(t, handler.objectHasMedia(map[string]any{"attachment": "nope"}))

		require.True(t, handler.objectIsReply(map[string]any{"inReplyTo": "x"}))
		require.False(t, handler.objectIsReply(map[string]any{"inReplyTo": ""}))
		require.False(t, handler.objectIsReply(map[string]any{}))

		require.True(t, handler.objectIsReblog(map[string]any{"reblog_of_id": "x"}))
		require.False(t, handler.objectIsReblog(map[string]any{"reblog_of_id": ""}))
		require.True(t, handler.objectIsReblog(map[string]any{"type": "Announce"}))
		require.False(t, handler.objectIsReblog(map[string]any{"type": "Create"}))

		require.Equal(t, []string{"go", "rust"}, handler.extractHashtagsFromObject(map[string]any{"hashtags": []string{"Go", "Rust"}}))
		require.Equal(t, []string{"go"}, handler.extractHashtagsFromObject(map[string]any{"tag": []any{map[string]any{"type": "Hashtag", "name": "#Go"}}}))
		require.Equal(t, []string{}, handler.extractHashtagsFromObject(123))
	})
}

func TestStatusesMoreCoverageStatusContext_Round12(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("validateStatusIDForContext bad id", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, makeRegistry(&NotesServiceStub{}, &AccountsServiceStub{}))

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/%/context", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "%"
		requireStatus(t, http.StatusBadRequest)(handler.HandleGetStatusContextLift(ctx))
	})

	t.Run("validateStatusIDForContext not found", func(t *testing.T) {
		notesSvc := &NotesServiceStub{
			GetNoteFunc: func(context.Context, string) (*storagemodels.Status, error) { return nil, errors.New("not found") },
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, makeRegistry(notesSvc, &AccountsServiceStub{}))

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/missing/context", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "missing"
		requireStatus(t, http.StatusNotFound)(handler.HandleGetStatusContextLift(ctx))
	})
}

func TestStatusesMoreCoverageUpdateStorageFailures_Round12(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("saveUpdatedStatus missing username writes 500", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg)

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(handler.saveUpdatedStatus(ctx, &activitypub.Note{BaseObject: activitypub.BaseObject{ID: cfg.ObjectURL("objects", "s1")}}))
	})

	t.Run("saveUpdatedStatus storage error writes 500", func(t *testing.T) {
		state := &round10QueryState{updateErrorOnce: errors.New("boom")}
		handler, _, _ := round11NewHandler(t, cfg, state)

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", headers, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(handler.saveUpdatedStatus(ctx, &activitypub.Note{
			BaseObject: activitypub.BaseObject{ID: cfg.ObjectURL("objects", "s1"), Type: activitypub.NoteType},
			Content:    "edit",
		}))
	})

	t.Run("createStatusUpdateActivity error writes 500", func(t *testing.T) {
		state := &round10QueryState{createErrorOnce: errors.New("boom")}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/statuses/s1", nil, nil, nil)
		require.NoError(t, err)

		actor := &activitypub.Actor{
			BaseObject:                activitypub.BaseObject{ID: cfg.ActorURL("alice"), Type: "Person"},
			PreferredUsername:         "alice",
			ManuallyApprovesFollowers: false,
		}
		note := &activitypub.Note{
			BaseObject:   activitypub.BaseObject{ID: cfg.ObjectURL("objects", "s1"), Type: activitypub.NoteType},
			AttributedTo: cfg.ActorURL("alice"),
			Content:      "edit",
		}
		requireStatus(t, http.StatusInternalServerError)(handler.createStatusUpdateActivity(ctx, note, actor))
	})
}

func TestStatusesMoreCoveragePublicTimeline_Round12(t *testing.T) {
	cfg := round11TestConfig()

	notesSvc := &NotesServiceStub{
		ListNotesFunc: func(context.Context, *notes.ListNotesQuery) (*notes.Result, error) {
			return nil, errors.New("boom")
		},
	}
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, makeRegistry(notesSvc, &AccountsServiceStub{}))

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/public", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusInternalServerError)(handler.HandleGetPublicTimelineLift(ctx))
}
