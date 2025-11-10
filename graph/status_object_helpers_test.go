package graph

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestResolveStatusAuthorUsername(t *testing.T) {
	t.Run("prefers explicit author username", func(t *testing.T) {
		status := &models.Status{
			AuthorUsername: "alice",
			AuthorID:       "https://dev.lesser.host/users/alice",
		}
		require.Equal(t, "alice", resolveStatusAuthorUsername(status))
	})

	t.Run("falls back to actor identifier", func(t *testing.T) {
		status := &models.Status{
			AuthorID: "https://dev.lesser.host/users/bob",
		}
		require.Equal(t, "bob", resolveStatusAuthorUsername(status))
	})

	t.Run("returns empty for nil status", func(t *testing.T) {
		require.Equal(t, "", resolveStatusAuthorUsername(nil))
	})
}
