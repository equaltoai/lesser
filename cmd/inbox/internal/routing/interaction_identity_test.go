package routing

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type canonicalizingStatusRepo struct {
	interfaces.StatusRepository
	status *models.Status
}

func (r *canonicalizingStatusRepo) GetStatusByURL(context.Context, string) (*models.Status, error) {
	return nil, storage.ErrNotFound
}

func (r *canonicalizingStatusRepo) GetStatus(_ context.Context, statusID string) (*models.Status, error) {
	if r.status != nil && r.status.StatusID == statusID {
		return r.status, nil
	}
	return nil, storage.ErrNotFound
}

func TestInboxInteractionIdentity_CanonicalizesKnownLocalStatusAlias(t *testing.T) {
	status := &models.Status{
		StatusID:       "status-1",
		AuthorID:       "https://example.com/users/alice",
		AuthorUsername: "alice",
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice/statuses/status-1",
				Type: activitypub.NoteType,
			},
			AttributedTo: "https://example.com/users/alice",
		},
	}
	handler := &InboxHandler{
		statusRepository: &canonicalizingStatusRepo{status: status},
		baseURL:          "https://example.com",
	}

	canonical, obj := handler.canonicalizeKnownInteractionObject(context.Background(), "https://example.com/objects/status-1")

	assert.Equal(t, status.Note.ID, canonical)
	require.NotNil(t, obj)
	note, ok := obj.(*activitypub.Note)
	require.True(t, ok)
	assert.Equal(t, status.Note.ID, note.ID)
}

func TestInboxInteractionIdentity_LeavesUnknownRemoteObjectAsReceived(t *testing.T) {
	handler := &InboxHandler{
		statusRepository: &canonicalizingStatusRepo{},
		baseURL:          "https://example.com",
	}

	objectID := "https://remote.example/users/bob/statuses/unknown"
	canonical, obj := handler.canonicalizeKnownInteractionObject(context.Background(), objectID)

	assert.Equal(t, objectID, canonical)
	assert.Nil(t, obj)
}
