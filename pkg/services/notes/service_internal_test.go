package notes

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
)

func TestResolveConversationID(t *testing.T) {
	ctx := context.Background()

	t.Run("nil status", func(t *testing.T) {
		assert.Equal(t, "", resolveConversationID(ctx, nil, nil))
	})

	t.Run("existing conversation id", func(t *testing.T) {
		status := &models.Status{
			StatusID:       "status-1",
			ConversationID: "conversation-1",
		}
		assert.Equal(t, "conversation-1", resolveConversationID(ctx, status, nil))
	})

	t.Run("new top level post", func(t *testing.T) {
		status := &models.Status{
			StatusID: "status-2",
		}
		assert.Equal(t, "status-2", resolveConversationID(ctx, status, nil))
	})

	t.Run("reply inherits parent conversation", func(t *testing.T) {
		status := &models.Status{
			StatusID:    "child-status",
			InReplyToID: "parent-status",
		}
		fetcher := func(_ context.Context, _ string) (*models.Status, error) {
			return &models.Status{ConversationID: "parent-conversation"}, nil
		}
		assert.Equal(t, "parent-conversation", resolveConversationID(ctx, status, fetcher))
	})

	t.Run("reply falls back to reply target", func(t *testing.T) {
		status := &models.Status{
			StatusID:    "child-status",
			InReplyToID: "parent-status",
		}
		fetcher := func(_ context.Context, _ string) (*models.Status, error) {
			return nil, errors.New("not found")
		}
		assert.Equal(t, "parent-status", resolveConversationID(ctx, status, fetcher))
	})
}
