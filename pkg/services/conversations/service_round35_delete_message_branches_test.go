package conversations

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestService_DeleteMessage_Round35_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("validation errors", func(t *testing.T) {
		service := NewService(
			&mockConversationRepository{},
			&mockNoteRepository{},
			&mockDirectMessageTombstoneRepository{},
			&mockAccountRepository{},
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			zaptest.NewLogger(t),
			"example.com",
		)

		ok, err := service.DeleteMessage(ctx, nil)
		require.False(t, ok)
		require.ErrorIs(t, err, ErrConversationValidationFailed)

		ok, err = service.DeleteMessage(ctx, &DeleteMessageCommand{MessageID: "m1", UserID: ""})
		require.False(t, ok)
		require.ErrorIs(t, err, ErrConversationValidationFailed)
	})

	t.Run("requires dm tombstone repo", func(t *testing.T) {
		service := NewService(
			&mockConversationRepository{},
			&mockNoteRepository{},
			nil,
			&mockAccountRepository{},
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			zaptest.NewLogger(t),
			"example.com",
		)

		ok, err := service.DeleteMessage(ctx, &DeleteMessageCommand{MessageID: "m1", UserID: "alice"})
		require.False(t, ok)
		require.ErrorIs(t, err, ErrDeleteMessage)
	})

	t.Run("status not found is idempotent", func(t *testing.T) {
		noteRepo := &mockNoteRepository{}
		service := NewService(
			&mockConversationRepository{},
			noteRepo,
			&mockDirectMessageTombstoneRepository{},
			&mockAccountRepository{},
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			zaptest.NewLogger(t),
			"example.com",
		)

		noteRepo.On("GetStatus", mock.Anything, "m1").Return((*models.Status)(nil), storage.ErrNotFound).Once()

		ok, err := service.DeleteMessage(ctx, &DeleteMessageCommand{MessageID: "m1", UserID: "alice"})
		require.True(t, ok)
		require.NoError(t, err)
	})

	t.Run("status nil is idempotent", func(t *testing.T) {
		noteRepo := &mockNoteRepository{}
		service := NewService(
			&mockConversationRepository{},
			noteRepo,
			&mockDirectMessageTombstoneRepository{},
			&mockAccountRepository{},
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			zaptest.NewLogger(t),
			"example.com",
		)

		noteRepo.On("GetStatus", mock.Anything, "m1").Return((*models.Status)(nil), nil).Once()

		ok, err := service.DeleteMessage(ctx, &DeleteMessageCommand{MessageID: "m1", UserID: "alice"})
		require.True(t, ok)
		require.NoError(t, err)
	})

	t.Run("rejects non-direct messages", func(t *testing.T) {
		noteRepo := &mockNoteRepository{}
		service := NewService(
			&mockConversationRepository{},
			noteRepo,
			&mockDirectMessageTombstoneRepository{},
			&mockAccountRepository{},
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			zaptest.NewLogger(t),
			"example.com",
		)

		noteRepo.On("GetStatus", mock.Anything, "m1").Return(&models.Status{
			StatusID:       "m1",
			Visibility:     models.VisibilityPublic,
			ConversationID: "conv123",
		}, nil).Once()

		ok, err := service.DeleteMessage(ctx, &DeleteMessageCommand{MessageID: "m1", UserID: "alice"})
		require.False(t, ok)
		require.ErrorIs(t, err, ErrConversationValidationFailed)
	})

	t.Run("rejects missing conversation id", func(t *testing.T) {
		noteRepo := &mockNoteRepository{}
		service := NewService(
			&mockConversationRepository{},
			noteRepo,
			&mockDirectMessageTombstoneRepository{},
			&mockAccountRepository{},
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			zaptest.NewLogger(t),
			"example.com",
		)

		noteRepo.On("GetStatus", mock.Anything, "m1").Return(&models.Status{
			StatusID:       "m1",
			Visibility:     models.VisibilityDirect,
			ConversationID: "",
		}, nil).Once()

		ok, err := service.DeleteMessage(ctx, &DeleteMessageCommand{MessageID: "m1", UserID: "alice"})
		require.False(t, ok)
		require.ErrorIs(t, err, ErrConversationValidationFailed)
	})

	t.Run("conversation not found is idempotent", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		noteRepo := &mockNoteRepository{}

		service := NewService(
			conversationRepo,
			noteRepo,
			&mockDirectMessageTombstoneRepository{},
			&mockAccountRepository{},
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			zaptest.NewLogger(t),
			"example.com",
		)

		noteRepo.On("GetStatus", mock.Anything, "m1").Return(&models.Status{
			StatusID:       "m1",
			Visibility:     models.VisibilityDirect,
			ConversationID: "conv123",
		}, nil).Once()
		conversationRepo.On("GetConversation", mock.Anything, "conv123").Return((*models.Conversation)(nil), storage.ErrNotFound).Once()

		ok, err := service.DeleteMessage(ctx, &DeleteMessageCommand{MessageID: "m1", UserID: "alice"})
		require.True(t, ok)
		require.NoError(t, err)
	})

	t.Run("account lookup failure returns get account error", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		noteRepo := &mockNoteRepository{}
		accountRepo := &mockAccountRepository{}

		service := NewService(
			conversationRepo,
			noteRepo,
			&mockDirectMessageTombstoneRepository{},
			accountRepo,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			zaptest.NewLogger(t),
			"example.com",
		)

		noteRepo.On("GetStatus", mock.Anything, "m1").Return(&models.Status{
			StatusID:       "m1",
			Visibility:     models.VisibilityDirect,
			ConversationID: "conv123",
		}, nil).Once()
		conversationRepo.On("GetConversation", mock.Anything, "conv123").Return(&models.Conversation{
			ID:           "conv123",
			Participants: []string{"https://example.com/users/alice"},
		}, nil).Once()
		accountRepo.On("GetAccount", mock.Anything, "alice").Return((*storage.Account)(nil), errors.New("boom")).Once()

		ok, err := service.DeleteMessage(ctx, &DeleteMessageCommand{MessageID: "m1", UserID: "alice"})
		require.False(t, ok)
		require.ErrorIs(t, err, ErrGetAccount)
	})

	t.Run("allows actor-id participant; tombstone errors propagate", func(t *testing.T) {
		conversationRepo := &mockConversationRepository{}
		noteRepo := &mockNoteRepository{}
		accountRepo := &mockAccountRepository{}
		dmTombstoneRepo := &mockDirectMessageTombstoneRepository{}

		service := NewService(
			conversationRepo,
			noteRepo,
			dmTombstoneRepo,
			accountRepo,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			zaptest.NewLogger(t),
			"example.com",
		)

		status := &models.Status{
			StatusID:       "m1",
			Visibility:     models.VisibilityDirect,
			ConversationID: "conv123",
		}
		noteRepo.On("GetStatus", mock.Anything, "m1").Return(status, nil).Once()

		conversation := &models.Conversation{
			ID:           "conv123",
			Participants: []string{"https://example.com/users/alice"},
		}
		conversationRepo.On("GetConversation", mock.Anything, "conv123").Return(conversation, nil).Once()

		accountRepo.On("GetAccount", mock.Anything, "alice").Return(&storage.Account{
			Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}},
		}, nil).Once()

		dmTombstoneRepo.On("CreateTombstone", mock.Anything, "alice", "m1").Return(errors.New("tombstone-failed")).Once()

		ok, err := service.DeleteMessage(ctx, &DeleteMessageCommand{MessageID: "m1", UserID: "alice"})
		require.False(t, ok)
		require.ErrorIs(t, err, ErrDeleteMessage)
	})
}

func TestService_GetConversationLastStatus_Round35_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid input", func(t *testing.T) {
		service := NewService(
			&mockConversationRepository{},
			&mockNoteRepository{},
			&mockDirectMessageTombstoneRepository{},
			&mockAccountRepository{},
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			zaptest.NewLogger(t),
			"example.com",
		)

		_, err := service.GetConversationLastStatus(ctx, "", "alice")
		require.ErrorIs(t, err, ErrConversationValidationFailed)
	})

	t.Run("not found is idempotent", func(t *testing.T) {
		noteRepo := &mockNoteRepository{}
		service := NewService(
			&mockConversationRepository{},
			noteRepo,
			&mockDirectMessageTombstoneRepository{},
			&mockAccountRepository{},
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			zaptest.NewLogger(t),
			"example.com",
		)

		noteRepo.
			On("GetConversationThreadReverse", mock.Anything, "conv123", mock.Anything).
			Return((*interfaces.PaginatedResult[*models.Status])(nil), storage.ErrNotFound).
			Once()

		status, err := service.GetConversationLastStatus(ctx, "conv123", "alice")
		require.NoError(t, err)
		require.Nil(t, status)
	})

	t.Run("tombstone lookup errors propagate", func(t *testing.T) {
		noteRepo := &mockNoteRepository{}
		dmTombstoneRepo := &mockDirectMessageTombstoneRepository{}
		service := NewService(
			&mockConversationRepository{},
			noteRepo,
			dmTombstoneRepo,
			&mockAccountRepository{},
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			zaptest.NewLogger(t),
			"example.com",
		)

		status := &models.Status{
			StatusID:       "m1",
			Visibility:     models.VisibilityDirect,
			AuthorUsername: "bob",
			ConversationID: "conv123",
			ToRecipients:   []string{"https://example.com/users/alice"},
		}

		noteRepo.
			On("GetConversationThreadReverse", mock.Anything, "conv123", mock.MatchedBy(func(opts interfaces.PaginationOptions) bool {
				return opts.Cursor == "" && opts.Limit == 50
			})).
			Return(&interfaces.PaginatedResult[*models.Status]{Items: []*models.Status{status}, HasMore: false}, nil).
			Once()

		dmTombstoneRepo.
			On("TombstonesByStatusID", mock.Anything, "alice", mock.Anything).
			Return((map[string]bool)(nil), errors.New("boom")).
			Once()

		_, err := service.GetConversationLastStatus(ctx, "conv123", "alice")
		require.Error(t, err)
	})

	t.Run("iterates pages until it finds visible message", func(t *testing.T) {
		noteRepo := &mockNoteRepository{}
		service := NewService(
			&mockConversationRepository{},
			noteRepo,
			nil,
			&mockAccountRepository{},
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			zaptest.NewLogger(t),
			"example.com",
		)

		invisible := &models.Status{
			StatusID:       "m0",
			Visibility:     models.VisibilityDirect,
			AuthorUsername: "bob",
			ConversationID: "conv123",
			ToRecipients:   []string{"https://example.com/users/bob"},
		}
		visible := &models.Status{
			StatusID:       "m1",
			Visibility:     models.VisibilityDirect,
			AuthorUsername: "bob",
			ConversationID: "conv123",
			ToRecipients:   []string{"https://example.com/users/alice"},
		}

		noteRepo.
			On("GetConversationThreadReverse", mock.Anything, "conv123", mock.MatchedBy(func(opts interfaces.PaginationOptions) bool {
				return opts.Cursor == "" && opts.Limit == 50
			})).
			Return(&interfaces.PaginatedResult[*models.Status]{Items: []*models.Status{invisible}, HasMore: true, NextCursor: "c1"}, nil).
			Once()

		noteRepo.
			On("GetConversationThreadReverse", mock.Anything, "conv123", mock.MatchedBy(func(opts interfaces.PaginationOptions) bool {
				return opts.Cursor == "c1" && opts.Limit == 50
			})).
			Return(&interfaces.PaginatedResult[*models.Status]{Items: []*models.Status{visible}, HasMore: false}, nil).
			Once()

		status, err := service.GetConversationLastStatus(ctx, "conv123", "alice")
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, "m1", status.StatusID)
	})
}
