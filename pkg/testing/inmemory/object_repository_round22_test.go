package inmemory

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
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
