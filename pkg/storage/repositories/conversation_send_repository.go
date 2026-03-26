package repositories

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
)

// TransactionalDirectMessageSendEnabled reports whether the repository can apply
// DM send transitions as a single database transaction.
func (r *ConversationRepository) TransactionalDirectMessageSendEnabled() bool {
	return r.transactWriteFn != nil
}

// ApplyDirectMessageSend writes the canonical DM send transition as one transaction.
func (r *ConversationRepository) ApplyDirectMessageSend(ctx context.Context, transition *models.DirectMessageSendTransition) error {
	if transition == nil || transition.Conversation == nil || transition.Status == nil {
		return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, EntityConversation, "dm send transition")
	}

	conversation, status, participantStates, err := prepareDirectMessageSendTransition(transition)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityConversation, transition.Conversation.ID)
	}
	if r.transactWriteFn == nil {
		return r.applyDirectMessageSendWithoutTransaction(ctx, conversation, status, participantStates, transition.CreateConversation)
	}

	err = r.transactWriteFn(ctx, func(tx core.TransactionBuilder) error {
		tx = tx.WithContext(ctx)

		if transition.CreateConversation {
			tx.Create(conversation)
			for _, state := range participantStates {
				tx.Create(state)
			}
			tx.Create(newConversationParticipantLookup(conversation.ID, conversation.Participants))
		} else {
			tx.Put(conversation)
			for _, state := range participantStates {
				tx.Put(state)
			}
		}

		tx.Create(status)
		return nil
	})
	if err != nil {
		if transition.CreateConversation && errors.IsConditionFailed(err) {
			return storage.ErrAlreadyExists
		}
		return ErrorHandler.HandleCreateError(err, EntityConversation, conversation.ID)
	}

	return nil
}

func (r *ConversationRepository) applyDirectMessageSendWithoutTransaction(ctx context.Context, conversation *models.Conversation, _ *models.Status, participantStates []*models.UserConversationState, createConversation bool) error {
	if createConversation {
		if err := r.createConversationLegacy(ctx, r.logger.With(), conversation, participantStates, true); err != nil {
			return err
		}
	} else {
		if err := conversation.UpdateKeys(); err != nil {
			return ErrorHandler.HandleUpdateError(err, EntityConversation, conversation.ID)
		}
		if err := r.GetDB().WithContext(ctx).Model(conversation).Update(); err != nil {
			return ErrorHandler.HandleUpdateError(err, EntityConversation, conversation.ID)
		}
		if err := r.createOrUpdateUserConversationStates(ctx, participantStates); err != nil {
			return ErrorHandler.HandleUpdateError(err, EntityConversation, conversation.ID)
		}
	}

	return nil
}

func prepareDirectMessageSendTransition(transition *models.DirectMessageSendTransition) (*models.Conversation, *models.Status, []*models.UserConversationState, error) {
	conversation := *transition.Conversation
	status := *transition.Status

	if conversation.ID == "" || status.StatusID == "" {
		return nil, nil, nil, storage.ErrInvalidInput
	}

	conversation.Participants = models.CanonicalConversationParticipants(conversation.Participants)
	if len(conversation.Participants) == 0 {
		return nil, nil, nil, storage.ErrInvalidInput
	}

	status.ConversationID = conversation.ID
	if err := status.BeforeCreate(); err != nil {
		return nil, nil, nil, err
	}

	lastMessageTime := status.PublishedAt.UTC()
	if lastMessageTime.IsZero() {
		lastMessageTime = status.CreatedAt.UTC()
	}
	if lastMessageTime.IsZero() {
		lastMessageTime = time.Now().UTC()
	}

	if conversation.CreatedAt.IsZero() {
		conversation.CreatedAt = lastMessageTime
	}
	conversation.UpdatedAt = lastMessageTime
	conversation.LastStatusID = status.StatusID
	conversation.LastMessageTime = lastMessageTime
	conversation.TotalMessageCount++
	if err := conversation.UpdateKeys(); err != nil {
		return nil, nil, nil, err
	}

	participantStates, err := normalizeExplicitConversationParticipantStates(&conversation, conversation.Participants, transition.ParticipantStates)
	if err != nil {
		return nil, nil, nil, err
	}

	return &conversation, &status, participantStates, nil
}
