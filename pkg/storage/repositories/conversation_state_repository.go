package repositories

import (
	"context"
	stdErrors "errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
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

func counterpartRefForConversation(conversation *models.Conversation, counterpartID string) *models.ConversationParticipantRef {
	if conversation == nil || counterpartID == "" {
		return nil
	}
	canonicalCounterpartID := models.CanonicalConversationParticipantID(counterpartID)
	for _, ref := range models.NormalizeConversationParticipantRefs(conversation.ParticipantRefs) {
		if models.CanonicalConversationParticipantID(ref.ParticipantID) == canonicalCounterpartID {
			refCopy := ref
			return &refCopy
		}
	}
	return nil
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

	counterpartID := counterpartForConversation(viewerID, participants)
	state := &models.UserConversationState{
		ViewerID:       viewerID,
		ConversationID: conversationID,
		CounterpartID:  counterpartID,
		Folder:         models.UserConversationFolderHidden,
		SortAt:         sortAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if ref := counterpartRefForConversation(conversation, counterpartID); ref != nil {
		state.CounterpartType = ref.ParticipantType
		state.CounterpartAcct = ref.Acct
		state.CounterpartDomain = ref.Domain
		state.CounterpartResolvedAt = ref.ResolvedAt
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

func cloneUserConversationState(state *models.UserConversationState) *models.UserConversationState {
	if state == nil {
		return nil
	}

	cloned := *state
	return &cloned
}

func cloneConversationForViewer(conversation *models.Conversation, state *models.UserConversationState) *models.Conversation {
	if conversation == nil {
		return nil
	}

	clonedParticipants := make([]string, len(conversation.Participants))
	copy(clonedParticipants, conversation.Participants)

	unread := false
	if state != nil {
		unread = state.Unread
	}

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
		ViewerState:       cloneUserConversationState(state),
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
		CounterpartType:          state.CounterpartType,
		CounterpartAcct:          state.CounterpartAcct,
		CounterpartDomain:        state.CounterpartDomain,
		CounterpartResolvedAt:    state.CounterpartResolvedAt,
		Folder:                   state.Folder,
		RequestState:             state.RequestState,
		PreviewStatusID:          state.PreviewStatusID,
		PreviewStatusPublishedAt: state.PreviewStatusPublishedAt,
		SortAt:                   state.SortAt,
		Unread:                   state.Unread,
		UnreadCount:              state.UnreadCount,
		LastReadAt:               state.LastReadAt,
		DeletedAt:                state.DeletedAt,
		RequestedAt:              state.RequestedAt,
		AcceptedAt:               state.AcceptedAt,
		DeclinedAt:               state.DeclinedAt,
		CreatedAt:                state.CreatedAt,
		UpdatedAt:                state.UpdatedAt,
	}
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
		return r.GetDB().WithContext(ctx).Model(state).CreateOrUpdate()
	case err != nil:
		return err
	default:
		return nil
	}
}

// PutUserConversationState persists a canonical per-user DM state row.
func (r *ConversationRepository) PutUserConversationState(ctx context.Context, state *models.UserConversationState) error {
	return r.createOrUpdateUserConversationState(ctx, state)
}

func (r *ConversationRepository) createOrUpdateUserConversationStates(ctx context.Context, states []*models.UserConversationState) error {
	for _, state := range states {
		if err := r.createOrUpdateUserConversationState(ctx, state); err != nil {
			return err
		}
	}
	return nil
}

func (r *ConversationRepository) initializeUserConversationStates(ctx context.Context, conversation *models.Conversation) error {
	if conversation == nil {
		return storage.ErrInvalidInput
	}

	for _, participantID := range models.ConversationLocalParticipantIDs(conversation) {
		state := defaultUserConversationState(conversation, participantID)
		existing, err := r.getUserConversationStateModel(ctx, participantID, conversation.ID)
		if err == nil && existing != nil {
			state.Folder = existing.Folder
			state.RequestState = existing.RequestState
			state.Unread = existing.Unread
			state.UnreadCount = existing.UnreadCount
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
		conversations = append(conversations, cloneConversationForViewer(conversation, state))
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

	total := int64(-1)
	if len(items) == 0 && !hasMore {
		total = 0
	}

	return &interfaces.PaginatedResult[*interfaces.UserConversationStateContract]{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      total,
	}, nil
}

// ListUnreadUserConversationStates queries the viewer's sparse unread index.
func (r *ConversationRepository) listUnreadUserConversationStatesModels(ctx context.Context, viewerID string, opts interfaces.PaginationOptions) ([]*models.UserConversationState, string, bool, error) {
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
			return []*models.UserConversationState{}, "", false, nil
		}
		return nil, "", false, ErrorHandler.HandleQueryError(err, EntityConversation, "unread user conversation states")
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

// ListUnreadUserConversationStates queries the viewer's sparse unread index.
func (r *ConversationRepository) ListUnreadUserConversationStates(ctx context.Context, viewerID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*interfaces.UserConversationStateContract], error) {
	states, nextCursor, hasMore, err := r.listUnreadUserConversationStatesModels(ctx, viewerID, opts)
	if err != nil {
		return nil, err
	}

	items := make([]*interfaces.UserConversationStateContract, 0, len(states))
	for _, state := range states {
		items = append(items, stateContractFromModel(state))
	}

	total := int64(-1)
	if len(items) == 0 && !hasMore {
		total = 0
	}

	return &interfaces.PaginatedResult[*interfaces.UserConversationStateContract]{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      total,
	}, nil
}

// ListConversationParticipantStates reverse-queries all per-user DM rows for a shared conversation.
func (r *ConversationRepository) ListConversationParticipantStates(ctx context.Context, conversationID string) ([]*interfaces.UserConversationStateContract, error) {
	// The whole keyed gsi3 CONVERSATION#<id> partition must be read to return
	// every participant state, so the read is a bounded page walk (wave #1469):
	// Limit(500)/page, 100-page cap, fail-closed on exhaustion. The OrderBy ASC
	// is preserved across pages via cursors.
	var stateModels []models.UserConversationState
	err := walkKeyedPages(
		r.GetDB().WithContext(ctx).Model(&models.UserConversationState{}).
			Index("gsi3").
			Where("gsi3PK", "=", fmt.Sprintf("CONVERSATION#%s", conversationID)).
			OrderBy("gsi3SK", "ASC"),
		500, 100,
		func(page []models.UserConversationState) (bool, error) {
			stateModels = append(stateModels, page...)
			return false, nil
		},
	)
	if err != nil {
		// Cap exhaustion fails the read closed instead of silently answering
		// "no participants" — only other errors keep the pre-existing not-found
		// swallow.
		if stdErrors.Is(err, errBoundedPageCapExceeded) {
			return nil, err
		}
		if errors.IsNotFound(err) {
			return []*interfaces.UserConversationStateContract{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, EntityConversation, "conversation participant states")
	}
	states := make([]*models.UserConversationState, len(stateModels))
	for i := range stateModels {
		states[i] = &stateModels[i]
	}

	items := make([]*interfaces.UserConversationStateContract, 0, len(states))
	for _, state := range states {
		items = append(items, stateContractFromModel(state))
	}
	return items, nil
}
