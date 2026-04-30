package handlers

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestHelpers_RecoveredActorAndStatusLinks_Round29(t *testing.T) {
	h := &Handler{cfg: &config.Config{Domain: "example.com"}}

	t.Run("remoteStatusURL prefers note id then first http url", func(t *testing.T) {
		require.Empty(t, h.remoteStatusURL(nil))
		require.Equal(t, "https://remote.example/notes/1", h.remoteStatusURL(&storagemodels.Status{
			Note: &activitypub.Note{BaseObject: activitypub.BaseObject{ID: " https://remote.example/notes/1 "}},
			URLs: []string{"https://fallback.example/status/1"},
		}))
		require.Equal(t, "https://remote.example/status/2", h.remoteStatusURL(&storagemodels.Status{
			URLs: []string{"not-a-url", " https://remote.example/status/2 "},
		}))
		require.Empty(t, h.remoteStatusURL(&storagemodels.Status{URLs: []string{"not-a-url"}}))
	})

	t.Run("status author profile url separates remote and local authors", func(t *testing.T) {
		require.Empty(t, h.statusAuthorProfileURL(nil))
		require.Equal(t, "https://remote.example/users/bob", h.statusAuthorProfileURL(&storagemodels.Status{
			AuthorUsername: "bob@remote.example",
			AuthorID:       " https://remote.example/users/bob ",
		}))
		require.Equal(t, "https://example.com/@alice", h.statusAuthorProfileURL(&storagemodels.Status{
			AuthorUsername: "alice",
			AuthorID:       "https://example.com/users/alice",
		}))
	})

	t.Run("status links use remote urls or local actor/status urls", func(t *testing.T) {
		uri, url := h.statusLinks(nil)
		require.Empty(t, uri)
		require.Empty(t, url)

		uri, url = h.statusLinks(&storagemodels.Status{
			StatusID:       "remote-status-id",
			AuthorUsername: "bob@remote.example",
			AuthorID:       "https://remote.example/users/bob",
			Note:           &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "https://remote.example/notes/1"}},
		})
		require.Equal(t, "https://remote.example/notes/1", uri)
		require.Equal(t, "https://remote.example/notes/1", url)

		uri, url = h.statusLinks(&storagemodels.Status{
			StatusID:       "status/with slash",
			AuthorUsername: "alice",
			AuthorID:       "https://example.com/users/alice",
		})
		require.Equal(t, "https://example.com/users/alice/statuses/status%2Fwith%20slash", uri)
		require.Equal(t, "https://example.com/@alice/status%2Fwith%20slash", url)
	})
}
