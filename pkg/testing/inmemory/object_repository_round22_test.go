package inmemory

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestObjectRepository_round22_update_paths_create_missing_objects(t *testing.T) {
	t.Run("UpdateObject creates a missing note and indexes metadata", func(t *testing.T) {
		repo := NewObjectRepository()
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "https://example.com/users/alice/statuses/status-1",
				Type:      activitypub.NoteType,
				InReplyTo: "https://example.com/users/alice/statuses/root",
			},
			Content:      "hello world",
			AttributedTo: "https://example.com/users/alice",
		}

		require.NoError(t, repo.UpdateObject(context.Background(), note))

		stored, err := repo.GetObject(context.Background(), note.ID)
		require.NoError(t, err)
		storedNote, ok := stored.(*activitypub.Note)
		require.True(t, ok)
		require.Equal(t, note.ID, storedNote.ID)
		require.Equal(t, note.Content, storedNote.Content)

		byActor, _, err := repo.GetObjectsByActor(context.Background(), note.AttributedTo, "", 10)
		require.NoError(t, err)
		require.Len(t, byActor, 1)

		replies, _, err := repo.GetReplies(context.Background(), note.InReplyTo, 10, "")
		require.NoError(t, err)
		require.Len(t, replies, 1)
	})

	t.Run("UpdateObjectWithHistory creates a missing note before history exists", func(t *testing.T) {
		repo := NewObjectRepository()
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "https://example.com/users/alice/statuses/status-2",
				Type:      activitypub.NoteType,
				InReplyTo: "https://example.com/users/alice/statuses/root",
			},
			Content:      "edited content",
			AttributedTo: "https://example.com/users/alice",
		}

		require.NoError(t, repo.UpdateObjectWithHistory(context.Background(), note, "https://example.com/users/alice"))

		stored, err := repo.GetObject(context.Background(), note.ID)
		require.NoError(t, err)
		storedNote, ok := stored.(*activitypub.Note)
		require.True(t, ok)
		require.Equal(t, note.Content, storedNote.Content)

		history, err := repo.GetUpdateHistory(context.Background(), note.ID, 10)
		require.NoError(t, err)
		require.Empty(t, history)

		byActor, _, err := repo.GetObjectsByActor(context.Background(), note.AttributedTo, "", 10)
		require.NoError(t, err)
		require.Len(t, byActor, 1)

		replies, _, err := repo.GetReplies(context.Background(), note.InReplyTo, 10, "")
		require.NoError(t, err)
		require.Len(t, replies, 1)
	})
}

func TestObjectRepository_round22_update_with_history_tracks_existing_objects(t *testing.T) {
	repo := NewObjectRepository()
	original := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/users/alice/statuses/status-3",
			Type:      activitypub.NoteType,
			InReplyTo: "https://example.com/users/alice/statuses/root-a",
		},
		Content:      "before",
		AttributedTo: "https://example.com/users/alice",
	}
	require.NoError(t, repo.CreateObject(context.Background(), original))

	updated := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        original.ID,
			Type:      activitypub.NoteType,
			InReplyTo: "https://example.com/users/alice/statuses/root-b",
		},
		Content:      "after",
		AttributedTo: "https://example.com/users/alice",
	}
	require.NoError(t, repo.UpdateObjectWithHistory(context.Background(), updated, "https://example.com/users/alice"))

	stored, err := repo.GetObject(context.Background(), original.ID)
	require.NoError(t, err)
	storedNote, ok := stored.(*activitypub.Note)
	require.True(t, ok)
	require.Equal(t, "after", storedNote.Content)

	history, err := repo.GetUpdateHistory(context.Background(), original.ID, 10)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, "https://example.com/users/alice", history[0].UpdatedBy)
}

func TestObjectRepository_round22_extract_object_metadata_helpers(t *testing.T) {
	id, objectType, actorID, inReplyTo := extractObjectMetadata(map[string]any{
		"id":           "https://example.com/objects/1",
		"type":         activitypub.NoteType,
		"attributedTo": "https://example.com/users/alice",
		"inReplyTo":    "https://example.com/objects/root",
	})
	require.Equal(t, "https://example.com/objects/1", id)
	require.Equal(t, activitypub.NoteType, objectType)
	require.Equal(t, "https://example.com/users/alice", actorID)
	require.Equal(t, "https://example.com/objects/root", inReplyTo)

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/objects/2",
			Type:      activitypub.NoteType,
			InReplyTo: "https://example.com/objects/root-2",
		},
		AttributedTo: "https://example.com/users/bob",
	}
	id, objectType, actorID, inReplyTo = extractObjectMetadata(note)
	require.Equal(t, "https://example.com/objects/2", id)
	require.Equal(t, activitypub.NoteType, objectType)
	require.Equal(t, "https://example.com/users/bob", actorID)
	require.Equal(t, "https://example.com/objects/root-2", inReplyTo)

	id, objectType, actorID, inReplyTo = extractObjectMetadata(make(chan int))
	require.Empty(t, id)
	require.Empty(t, objectType)
	require.Empty(t, actorID)
	require.Empty(t, inReplyTo)
}

func TestObjectRepository_round22_helper_storage_paths(t *testing.T) {
	repo := NewObjectRepository()

	repo.SetMissingReplies("status-1", []string{"reply-1", "reply-2"})
	missing, err := repo.GetMissingReplies(context.Background(), "status-1")
	require.NoError(t, err)
	require.Equal(t, []string{"reply-1", "reply-2"}, []string{missing[0].StatusID, missing[1].StatusID})

	ctxValue := &storage.ThreadContext{
		StatusID:    "status-1",
		Ancestors:   []string{"ancestor-1"},
		Descendants: []string{"desc-1"},
	}
	repo.SetThreadContext("status-1", ctxValue)
	threadCtx, err := repo.GetThreadContext(context.Background(), "status-1")
	require.NoError(t, err)
	require.Equal(t, ctxValue, threadCtx)

	require.Equal(t, []string{"a", "c"}, removeString([]string{"a", "b", "c"}, "b"))
	require.Equal(t, []string{"a", "c"}, removeString([]string{"a", "c"}, "z"))
}
