package conversations

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

type federationServiceWithError struct {
	err error
}

func (f *federationServiceWithError) QueueActivity(context.Context, *activitypub.Activity) error {
	return f.err
}

func TestService_validateSendMessageCommandBasic_InReplyToValidation(t *testing.T) {
	t.Parallel()

	service, _, noteRepo, _, _, _ := createTestService()
	ctx := context.Background()

	cmd := &SendDirectMessageCommand{
		SenderID:    "sender-1",
		Recipients:  []string{"recipient-1"},
		Content:     "hello",
		InReplyToID: "parent-1",
	}

	t.Run("repo error returns invalid in reply error", func(t *testing.T) {
		noteRepo.On("GetStatus", ctx, "parent-1").Return((*models.Status)(nil), errors.New("boom")).Once()
		err := service.validateSendMessageCommandBasic(ctx, cmd)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrInvalidInReplyToIDConversation)
	})

	t.Run("non-direct parent rejects reply", func(t *testing.T) {
		noteRepo.On("GetStatus", ctx, "parent-1").Return(&models.Status{Visibility: "public"}, nil).Once()
		err := service.validateSendMessageCommandBasic(ctx, cmd)
		require.ErrorIs(t, err, ErrCanOnlyReplyToDirectMessages)
	})

	t.Run("direct parent allows reply", func(t *testing.T) {
		noteRepo.On("GetStatus", ctx, "parent-1").Return(&models.Status{Visibility: VisibilityDirect}, nil).Once()
		err := service.validateSendMessageCommandBasic(ctx, cmd)
		require.NoError(t, err)
	})
}

func TestService_queueFederationDelivery_CoversBranches(t *testing.T) {
	t.Parallel()

	service, _, _, _, _, federation := createTestService()
	ctx := context.Background()

	t.Run("nil federation skips", func(t *testing.T) {
		service.federation = nil
		service.queueFederationDelivery(ctx, &models.Status{StatusID: "m1", ToRecipients: []string{"https://remote/users/bob"}})
	})

	t.Run("no remote recipients skips", func(t *testing.T) {
		service.federation = federation
		service.queueFederationDelivery(ctx, &models.Status{StatusID: "m2", ToRecipients: []string{"https://example.com/users/alice"}})
		require.Empty(t, federation.GetQueuedActivities())
	})

	t.Run("remote recipients queue create activity", func(t *testing.T) {
		federation.activities = nil
		service.federation = federation
		service.queueFederationDelivery(ctx, &models.Status{
			StatusID:       "m3",
			ToRecipients:   []string{"https://remote/users/bob"},
			CcRecipients:   []string{},
			ConversationID: "conv",
			Note:           &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-1", Type: "Note"}},
		})
		require.Len(t, federation.GetQueuedActivities(), 1)
	})

	t.Run("queue error is swallowed", func(t *testing.T) {
		service.federation = &federationServiceWithError{err: errors.New("boom")}
		service.queueFederationDelivery(ctx, &models.Status{
			StatusID:     "m4",
			ToRecipients: []string{"https://remote/users/bob"},
			Note:         &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-2", Type: "Note"}},
		})
	})
}
