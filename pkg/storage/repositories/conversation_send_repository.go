package repositories

import (
	"context"
	stdErrors "errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
)

var errDirectMessageStatusStageRequired = stdErrors.New("direct message status stage callback required")

type preparedDirectMessageSendTransition struct {
	conversation              *models.Conversation
	expectedConversation      *models.Conversation
	status                    *models.Status
	participantStates         []*models.UserConversationState
	expectedParticipantStates []*models.UserConversationState
	createConversation        bool
}

// TransactionalDirectMessageSendEnabled reports whether the repository can apply
// DM send transitions as a single database transaction.
func (r *ConversationRepository) TransactionalDirectMessageSendEnabled() bool {
	return r.transactWriteFn != nil
}

// ApplyDirectMessageSend writes the canonical DM send transition as one transaction.
func (r *ConversationRepository) ApplyDirectMessageSend(ctx context.Context, transition *models.DirectMessageSendTransition, stageStatusCreate interfaces.DirectMessageStatusStageFn) error {
	if transition == nil || transition.Conversation == nil || transition.Status == nil {
		return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, EntityConversation, "dm send transition")
	}

	prepared, err := prepareDirectMessageSendTransition(transition)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityConversation, transition.Conversation.ID)
	}
	if r.transactWriteFn == nil {
		return r.applyDirectMessageSendWithoutTransaction(ctx, prepared.conversation, prepared.status, prepared.participantStates, prepared.createConversation)
	}
	if stageStatusCreate == nil {
		return ErrorHandler.HandleCreateError(errDirectMessageStatusStageRequired, EntityConversation, prepared.conversation.ID)
	}

	err = r.transactWriteFn(ctx, func(tx core.TransactionBuilder) error {
		tx = tx.WithContext(ctx)

		if prepared.createConversation {
			tx.Create(prepared.conversation)
			for _, state := range prepared.participantStates {
				tx.Create(state)
			}
			tx.Create(newConversationParticipantLookupForConversation(prepared.conversation, prepared.conversation.Participants))
		} else {
			tx.UpdateWithBuilder(prepared.expectedConversation, func(update core.UpdateBuilder) error {
				// DynamoDB ADD initializes a missing numeric attribute, so this remains
				// compatible with legacy rows without generating an overlapping path error.
				update.Add("TotalMessageCount", int64(1)).
					Set("LastStatusID", prepared.conversation.LastStatusID).
					Set("LastMessageTime", prepared.conversation.LastMessageTime).
					Set("UpdatedAt", prepared.conversation.UpdatedAt)
				return nil
			}, directMessageSendConversationConditions(prepared.expectedConversation)...)
			for index, state := range prepared.participantStates {
				expected := prepared.expectedParticipantStates[index]
				tx.UpdateWithBuilder(expected, func(update core.UpdateBuilder) error {
					applyDirectMessageParticipantStateUpdate(update, state)
					return nil
				}, directMessageSendParticipantStateConditions(expected)...)
			}
		}

		if err := stageStatusCreate(tx, prepared.status); err != nil {
			return fmt.Errorf("stage direct message status create %s: %w", prepared.status.StatusID, err)
		}
		return nil
	})
	if err != nil {
		if errors.IsConditionFailed(err) {
			if prepared.createConversation {
				return storage.ErrAlreadyExists
			}
			return storage.ErrVersionConflict
		}
		return ErrorHandler.HandleCreateError(err, EntityConversation, prepared.conversation.ID)
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

func prepareDirectMessageSendTransition(transition *models.DirectMessageSendTransition) (*preparedDirectMessageSendTransition, error) {
	expectedConversation := cloneConversationModel(transition.Conversation)
	if expectedConversation == nil {
		return nil, storage.ErrInvalidInput
	}

	status := *transition.Status

	if expectedConversation.ID == "" || status.StatusID == "" {
		return nil, storage.ErrInvalidInput
	}

	expectedConversation.Participants = models.CanonicalConversationParticipants(expectedConversation.Participants)
	expectedConversation.ParticipantRefs = models.NormalizeConversationParticipantRefs(expectedConversation.ParticipantRefs)
	if len(expectedConversation.ParticipantRefs) > 0 {
		expectedConversation.Participants = models.ConversationParticipantIDsFromRefs(expectedConversation.ParticipantRefs)
	}
	if len(expectedConversation.Participants) == 0 {
		return nil, storage.ErrInvalidInput
	}
	if err := expectedConversation.UpdateKeys(); err != nil {
		return nil, err
	}

	status.ConversationID = expectedConversation.ID

	lastMessageTime := status.PublishedAt.UTC()
	if lastMessageTime.IsZero() {
		lastMessageTime = status.CreatedAt.UTC()
	}
	if lastMessageTime.IsZero() {
		lastMessageTime = time.Now().UTC()
	}
	if status.PublishedAt.IsZero() {
		status.PublishedAt = lastMessageTime
	}

	conversation := cloneConversationModel(expectedConversation)
	if conversation == nil {
		return nil, storage.ErrInvalidInput
	}
	if conversation.CreatedAt.IsZero() {
		conversation.CreatedAt = lastMessageTime
	}
	conversation.UpdatedAt = lastMessageTime
	conversation.LastStatusID = status.StatusID
	conversation.LastMessageTime = lastMessageTime
	conversation.TotalMessageCount++
	if err := conversation.UpdateKeys(); err != nil {
		return nil, err
	}

	participantStates, err := normalizeExplicitConversationParticipantStates(conversation, conversation.Participants, transition.ParticipantStates)
	if err != nil {
		return nil, err
	}

	prepared := &preparedDirectMessageSendTransition{
		conversation:         conversation,
		expectedConversation: expectedConversation,
		status:               &status,
		participantStates:    participantStates,
		createConversation:   transition.CreateConversation,
	}
	if transition.CreateConversation {
		return prepared, nil
	}

	if len(transition.ExpectedParticipantStates) != len(transition.ParticipantStates) {
		return nil, storage.ErrInvalidInput
	}

	expectedParticipantStates, err := normalizeExplicitConversationParticipantStates(expectedConversation, expectedConversation.Participants, transition.ExpectedParticipantStates)
	if err != nil {
		return nil, err
	}
	prepared.expectedParticipantStates = expectedParticipantStates

	return prepared, nil
}

func cloneConversationModel(conversation *models.Conversation) *models.Conversation {
	if conversation == nil {
		return nil
	}

	cloned := *conversation
	if conversation.Participants != nil {
		cloned.Participants = append([]string(nil), conversation.Participants...)
	}
	if conversation.ParticipantRefs != nil {
		cloned.ParticipantRefs = append([]models.ConversationParticipantRef(nil), conversation.ParticipantRefs...)
	}

	return &cloned
}

func directMessageSendConversationConditions(conversation *models.Conversation) []core.TransactCondition {
	if conversation == nil {
		return nil
	}

	conditions := []core.TransactCondition{
		{Kind: core.TransactConditionKindPrimaryKeyExists},
		{Field: "TotalMessageCount", Operator: "=", Value: conversation.TotalMessageCount},
	}
	if !conversation.UpdatedAt.IsZero() {
		conditions = append(conditions, core.TransactCondition{Field: "UpdatedAt", Operator: "=", Value: conversation.UpdatedAt})
	}

	return conditions
}

func directMessageSendParticipantStateConditions(state *models.UserConversationState) []core.TransactCondition {
	if state == nil {
		return nil
	}

	conditions := []core.TransactCondition{
		{Kind: core.TransactConditionKindPrimaryKeyExists},
	}
	if !state.UpdatedAt.IsZero() {
		conditions = append(conditions, core.TransactCondition{Field: "UpdatedAt", Operator: "=", Value: state.UpdatedAt})
	}

	return conditions
}

func applyDirectMessageParticipantStateUpdate(update core.UpdateBuilder, state *models.UserConversationState) {
	update.Set("CounterpartID", state.CounterpartID).
		Set("Folder", state.Folder).
		Set("SortAt", state.SortAt).
		Set("Unread", state.Unread).
		Set("UpdatedAt", state.UpdatedAt)

	if state.CounterpartType != "" {
		update.Set("CounterpartType", state.CounterpartType)
	} else {
		update.Remove("CounterpartType")
	}
	if state.CounterpartAcct != "" {
		update.Set("CounterpartAcct", state.CounterpartAcct)
	} else {
		update.Remove("CounterpartAcct")
	}
	if state.CounterpartDomain != "" {
		update.Set("CounterpartDomain", state.CounterpartDomain)
	} else {
		update.Remove("CounterpartDomain")
	}
	if state.RequestState != "" {
		update.Set("RequestState", state.RequestState)
	} else {
		update.Remove("RequestState")
	}
	if state.PreviewStatusID != "" {
		update.Set("PreviewStatusID", state.PreviewStatusID)
	} else {
		update.Remove("PreviewStatusID")
	}
	if !state.PreviewStatusPublishedAt.IsZero() {
		update.Set("PreviewStatusPublishedAt", state.PreviewStatusPublishedAt)
	} else {
		update.Remove("PreviewStatusPublishedAt")
	}
	if state.LastReadAt != nil {
		update.Set("LastReadAt", *state.LastReadAt)
	} else {
		update.Remove("LastReadAt")
	}
	if state.DeletedAt != nil {
		update.Set("DeletedAt", *state.DeletedAt)
	} else {
		update.Remove("DeletedAt")
	}
	if state.RequestedAt != nil {
		update.Set("RequestedAt", *state.RequestedAt)
	} else {
		update.Remove("RequestedAt")
	}
	if state.AcceptedAt != nil {
		update.Set("AcceptedAt", *state.AcceptedAt)
	} else {
		update.Remove("AcceptedAt")
	}
	if state.DeclinedAt != nil {
		update.Set("DeclinedAt", *state.DeclinedAt)
	} else {
		update.Remove("DeclinedAt")
	}
	if state.CounterpartResolvedAt != nil {
		update.Set("CounterpartResolvedAt", *state.CounterpartResolvedAt)
	} else {
		update.Remove("CounterpartResolvedAt")
	}
}
