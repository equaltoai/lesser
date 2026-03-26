package repositories

import (
	"context"
	stdErrors "errors"
	"fmt"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/errors"
)

var _ interfaces.DirectMessageRepository = (*ConversationRepository)(nil)

func userConversationStatePK(viewerID string) string {
	return fmt.Sprintf("USER_CONVERSATION_STATE#%s", models.CanonicalConversationParticipantID(viewerID))
}

func userConversationStateSK(conversationID string) string {
	return fmt.Sprintf("CONVERSATION#%s", conversationID)
}

func userConversationFolderIndexPK(viewerID string, folder models.UserConversationFolder) string {
	return fmt.Sprintf("USER_CONVERSATION_FOLDER#%s#%s", models.CanonicalConversationParticipantID(viewerID), folder)
}

func userConversationUnreadIndexPK(viewerID string) string {
	return fmt.Sprintf("USER_CONVERSATION_UNREAD#%s", models.CanonicalConversationParticipantID(viewerID))
}

func userConversationLegacyPK(viewerID string) string {
	return fmt.Sprintf("USER_CONVERSATIONS#%s", models.CanonicalConversationParticipantID(viewerID))
}

func userConversationLegacyGSI1SK(viewerID string) string {
	return fmt.Sprintf("PARTICIPANT#%s", models.CanonicalConversationParticipantID(viewerID))
}

func counterpartForConversation(viewerID string, participants []string) string {
	canonicalViewerID := models.CanonicalConversationParticipantID(viewerID)
	for _, participantID := range participants {
		if models.CanonicalConversationParticipantID(participantID) == canonicalViewerID {
			continue
		}
		return participantID
	}
	return ""
}

func defaultUserConversationState(conversation *models.Conversation, viewerID string) *models.UserConversationState {
	now := time.Now().UTC()
	sortAt := now
	conversationID := ""
	participants := []string(nil)
	if conversation != nil {
		conversationID = conversation.ID
		participants = conversation.Participants
		if !conversation.LastMessageTime.IsZero() {
			sortAt = conversation.LastMessageTime.UTC()
		} else if !conversation.UpdatedAt.IsZero() {
			sortAt = conversation.UpdatedAt.UTC()
		}
	}

	state := &models.UserConversationState{
		ViewerID:       viewerID,
		ConversationID: conversationID,
		CounterpartID:  counterpartForConversation(viewerID, participants),
		Folder:         models.UserConversationFolderHidden,
		SortAt:         sortAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if conversation != nil {
		state.CreatedAt = conversation.CreatedAt.UTC()
		state.UpdatedAt = conversation.UpdatedAt.UTC()
		if state.CreatedAt.IsZero() {
			state.CreatedAt = now
		}
		if state.UpdatedAt.IsZero() {
			state.UpdatedAt = now
		}
		state.PreviewStatusID = conversation.LastStatusID
		if !conversation.LastMessageTime.IsZero() {
			state.PreviewStatusPublishedAt = conversation.LastMessageTime.UTC()
			state.SortAt = conversation.LastMessageTime.UTC()
		}
	}
	return state
}

func cloneConversationForViewer(conversation *models.Conversation, unread bool) *models.Conversation {
	if conversation == nil {
		return nil
	}

	clonedParticipants := make([]string, len(conversation.Participants))
	copy(clonedParticipants, conversation.Participants)

	return &models.Conversation{
		PK:                conversation.PK,
		SK:                conversation.SK,
		GSI1PK:            conversation.GSI1PK,
		GSI1SK:            conversation.GSI1SK,
		ID:                conversation.ID,
		Participants:      clonedParticipants,
		LastStatusID:      conversation.LastStatusID,
		Unread:            unread,
		CreatedAt:         conversation.CreatedAt,
		UpdatedAt:         conversation.UpdatedAt,
		TotalMessageCount: conversation.TotalMessageCount,
		LastMessageTime:   conversation.LastMessageTime,
	}
}

func stateContractFromModel(state *models.UserConversationState) *interfaces.UserConversationStateContract {
	if state == nil {
		return nil
	}
	return &interfaces.UserConversationStateContract{
		ViewerID:                 state.ViewerID,
		ConversationID:           state.ConversationID,
		CounterpartID:            state.CounterpartID,
		Folder:                   state.Folder,
		RequestState:             state.RequestState,
		PreviewStatusID:          state.PreviewStatusID,
		PreviewStatusPublishedAt: state.PreviewStatusPublishedAt,
		SortAt:                   state.SortAt,
		Unread:                   state.Unread,
		LastReadAt:               state.LastReadAt,
		DeletedAt:                state.DeletedAt,
		RequestedAt:              state.RequestedAt,
		AcceptedAt:               state.AcceptedAt,
		DeclinedAt:               state.DeclinedAt,
		CreatedAt:                state.CreatedAt,
		UpdatedAt:                state.UpdatedAt,
	}
}

func stateRecordFromModel(state *models.UserConversationState, conversation *models.Conversation) *models.ConversationParticipantRecord {
	if state == nil {
		return nil
	}

	record := &models.ConversationParticipantRecord{
		PK:                       userConversationLegacyPK(state.ViewerID),
		SK:                       state.LegacyListCursor(),
		GSI1PK:                   fmt.Sprintf(models.KeyPatternConversation, state.ConversationID),
		GSI1SK:                   userConversationLegacyGSI1SK(state.ViewerID),
		ViewerID:                 state.ViewerID,
		ConversationID:           state.ConversationID,
		CounterpartID:            state.CounterpartID,
		Folder:                   state.Folder,
		RequestState:             state.RequestState,
		RequestedAt:              state.RequestedAt,
		AcceptedAt:               state.AcceptedAt,
		DeclinedAt:               state.DeclinedAt,
		DeletedAt:                state.DeletedAt,
		Unread:                   state.Unread,
		LastReadAt:               state.LastReadAt,
		PreviewStatusID:          state.PreviewStatusID,
		PreviewStatusPublishedAt: state.PreviewStatusPublishedAt,
		SortAt:                   state.SortAt,
		Conversation:             cloneConversationForViewer(conversation, state.Unread),
	}
	return record
}

func (r *ConversationRepository) getUserConversationStateModel(ctx context.Context, viewerID, conversationID string) (*models.UserConversationState, error) {
	var state models.UserConversationState
	err := r.GetDB().WithContext(ctx).Model(&models.UserConversationState{}).
		Where("PK", "=", userConversationStatePK(viewerID)).
		Where("SK", "=", userConversationStateSK(conversationID)).
		First(&state)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return &state, nil
}

func (r *ConversationRepository) ensureUserConversationStateModel(ctx context.Context, viewerID, conversationID string) (*models.UserConversationState, error) {
	state, err := r.getUserConversationStateModel(ctx, viewerID, conversationID)
	if err == nil {
		return state, nil
	}
	if !stdErrors.Is(err, storage.ErrNotFound) {
		return nil, err
	}

	conversation, convErr := r.GetConversation(ctx, conversationID)
	if convErr != nil {
		return nil, err
	}

	state = defaultUserConversationState(conversation, viewerID)
	if createErr := r.createOrUpdateUserConversationState(ctx, state); createErr != nil {
		return nil, createErr
	}
	return state, nil
}

func (r *ConversationRepository) createOrUpdateUserConversationState(ctx context.Context, state *models.UserConversationState) error {
	if state == nil {
		return storage.ErrInvalidInput
	}

	existing, err := r.getUserConversationStateModel(ctx, state.ViewerID, state.ConversationID)
	switch {
	case err == nil && existing != nil:
		state.CreatedAt = existing.CreatedAt
		if err := state.BeforeUpdate(); err != nil {
			return err
		}
		return r.GetDB().WithContext(ctx).Model(state).Update()
	case stdErrors.Is(err, storage.ErrNotFound):
		if err := state.BeforeCreate(); err != nil {
			return err
		}
		return r.GetDB().WithContext(ctx).Model(state).Create()
	case err != nil:
		return err
	default:
		return nil
	}
}

func (r *ConversationRepository) initializeUserConversationStates(ctx context.Context, conversation *models.Conversation) error {
	if conversation == nil {
		return storage.ErrInvalidInput
	}

	for _, participantID := range conversation.Participants {
		state := defaultUserConversationState(conversation, participantID)
		existing, err := r.getUserConversationStateModel(ctx, participantID, conversation.ID)
		if err == nil && existing != nil {
			state.Folder = existing.Folder
			state.RequestState = existing.RequestState
			state.Unread = existing.Unread
			state.LastReadAt = existing.LastReadAt
			state.DeletedAt = existing.DeletedAt
			state.RequestedAt = existing.RequestedAt
			state.AcceptedAt = existing.AcceptedAt
			state.DeclinedAt = existing.DeclinedAt
			state.CreatedAt = existing.CreatedAt
			state.UpdatedAt = conversation.UpdatedAt.UTC()
			if existing.PreviewStatusID != "" {
				state.PreviewStatusID = existing.PreviewStatusID
			}
			if !existing.PreviewStatusPublishedAt.IsZero() {
				state.PreviewStatusPublishedAt = existing.PreviewStatusPublishedAt
			}
			if !existing.SortAt.IsZero() {
				state.SortAt = existing.SortAt
			}
		} else if err != nil && !stdErrors.Is(err, storage.ErrNotFound) {
			return err
		}

		if err := r.createOrUpdateUserConversationState(ctx, state); err != nil {
			return err
		}
	}

	return nil
}

func (r *ConversationRepository) listUserConversationStatesByFolderModels(ctx context.Context, viewerID string, folder models.UserConversationFolder, opts interfaces.PaginationOptions) ([]*models.UserConversationState, string, bool, error) {
	limit := clampListLimit(opts.Limit, 20, 100)
	query := r.GetDB().WithContext(ctx).Model(&models.UserConversationState{}).
		Index("gsi1").
		Where("gsi1PK", "=", userConversationFolderIndexPK(viewerID, folder)).
		OrderBy("gsi1SK", "DESC").
		Limit(limit + 1)
	if opts.Cursor != "" {
		query = query.Where("gsi1SK", "<", opts.Cursor)
	}

	var states []*models.UserConversationState
	if err := query.All(&states); err != nil {
		if errors.IsNotFound(err) {
			return nil, "", false, nil
		}
		return nil, "", false, ErrorHandler.HandleQueryError(err, EntityConversation, "user conversation state by folder")
	}

	hasMore := len(states) > limit
	if hasMore {
		states = states[:limit]
	}

	nextCursor := ""
	if hasMore && len(states) > 0 {
		nextCursor = states[len(states)-1].LegacyListCursor()
	}

	return states, nextCursor, hasMore, nil
}

func (r *ConversationRepository) loadConversationsForStates(ctx context.Context, states []*models.UserConversationState) ([]*models.Conversation, error) {
	conversations := make([]*models.Conversation, 0, len(states))
	for _, state := range states {
		if state == nil {
			continue
		}
		conversation, err := r.GetConversation(ctx, state.ConversationID)
		if err != nil {
			if stdErrors.Is(err, storage.ErrNotFound) {
				continue
			}
			return nil, err
		}
		conversations = append(conversations, cloneConversationForViewer(conversation, state.Unread))
	}
	return conversations, nil
}

func folderFromRequestState(requestState models.DmRequestState) models.UserConversationFolder {
	switch requestState {
	case models.DmRequestStatePending:
		return models.UserConversationFolderRequests
	case models.DmRequestStateDeclined:
		return models.UserConversationFolderDeclined
	default:
		return models.UserConversationFolderInbox
	}
}

func mergeVisibleConversationStatePages(inbox []*models.UserConversationState, requests []*models.UserConversationState, limit int) ([]*models.UserConversationState, string, bool) {
	merged := make([]*models.UserConversationState, 0, len(inbox)+len(requests))
	merged = append(merged, inbox...)
	merged = append(merged, requests...)

	sort.Slice(merged, func(i, j int) bool {
		if merged[i].SortAt.Equal(merged[j].SortAt) {
			return merged[i].ConversationID > merged[j].ConversationID
		}
		return merged[i].SortAt.After(merged[j].SortAt)
	})

	hasMore := len(merged) > limit
	if !hasMore {
		return merged, "", false
	}

	merged = merged[:limit]
	return merged, merged[len(merged)-1].LegacyListCursor(), true
}

func participantRecordFolder(record *models.ConversationParticipantRecord) models.UserConversationFolder {
	if record == nil {
		return models.UserConversationFolderHidden
	}
	if record.Folder != "" {
		return record.Folder
	}
	if record.DeletedAt != nil && !record.DeletedAt.IsZero() {
		return models.UserConversationFolderHidden
	}
	return folderFromRequestState(record.RequestState)
}

// GetUserConversationState point-reads the viewer's canonical DM state for a conversation.
func (r *ConversationRepository) GetUserConversationState(ctx context.Context, viewerID, conversationID string) (*interfaces.UserConversationStateContract, error) {
	state, err := r.getUserConversationStateModel(ctx, viewerID, conversationID)
	if err != nil {
		return nil, err
	}
	return stateContractFromModel(state), nil
}

// ListUserConversationStatesByFolder queries the viewer's folder index without scan-side filtering.
func (r *ConversationRepository) ListUserConversationStatesByFolder(ctx context.Context, viewerID string, folder interfaces.UserConversationFolder, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*interfaces.UserConversationStateContract], error) {
	states, nextCursor, hasMore, err := r.listUserConversationStatesByFolderModels(ctx, viewerID, folder, opts)
	if err != nil {
		return nil, err
	}

	items := make([]*interfaces.UserConversationStateContract, 0, len(states))
	for _, state := range states {
		items = append(items, stateContractFromModel(state))
	}

	return &interfaces.PaginatedResult[*interfaces.UserConversationStateContract]{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      -1,
	}, nil
}

// ListUnreadUserConversationStates queries the viewer's sparse unread index.
func (r *ConversationRepository) ListUnreadUserConversationStates(ctx context.Context, viewerID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*interfaces.UserConversationStateContract], error) {
	limit := clampListLimit(opts.Limit, 20, 100)
	query := r.GetDB().WithContext(ctx).Model(&models.UserConversationState{}).
		Index("gsi2").
		Where("gsi2PK", "=", userConversationUnreadIndexPK(viewerID)).
		OrderBy("gsi2SK", "DESC").
		Limit(limit + 1)
	if opts.Cursor != "" {
		query = query.Where("gsi2SK", "<", opts.Cursor)
	}

	var states []*models.UserConversationState
	if err := query.All(&states); err != nil {
		if errors.IsNotFound(err) {
			return &interfaces.PaginatedResult[*interfaces.UserConversationStateContract]{
				Items:   []*interfaces.UserConversationStateContract{},
				Total:   0,
				HasMore: false,
			}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, EntityConversation, "unread user conversation states")
	}

	hasMore := len(states) > limit
	if hasMore {
		states = states[:limit]
	}

	nextCursor := ""
	if hasMore && len(states) > 0 {
		nextCursor = states[len(states)-1].LegacyListCursor()
	}

	items := make([]*interfaces.UserConversationStateContract, 0, len(states))
	for _, state := range states {
		items = append(items, stateContractFromModel(state))
	}

	return &interfaces.PaginatedResult[*interfaces.UserConversationStateContract]{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      -1,
	}, nil
}

// ListConversationParticipantStates reverse-queries all per-user DM rows for a shared conversation.
func (r *ConversationRepository) ListConversationParticipantStates(ctx context.Context, conversationID string) ([]*interfaces.UserConversationStateContract, error) {
	var states []*models.UserConversationState
	err := r.GetDB().WithContext(ctx).Model(&models.UserConversationState{}).
		Index("gsi3").
		Where("gsi3PK", "=", fmt.Sprintf("CONVERSATION#%s", conversationID)).
		OrderBy("gsi3SK", "ASC").
		All(&states)
	if err != nil {
		if errors.IsNotFound(err) {
			return []*interfaces.UserConversationStateContract{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, EntityConversation, "conversation participant states")
	}

	items := make([]*interfaces.UserConversationStateContract, 0, len(states))
	for _, state := range states {
		items = append(items, stateContractFromModel(state))
	}
	return items, nil
}
