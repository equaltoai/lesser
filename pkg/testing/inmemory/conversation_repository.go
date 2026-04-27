// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ConversationRepository is a thread-safe in-memory implementation of interfaces.ConversationRepository.
type ConversationRepository struct {
	mu sync.RWMutex
	// conversations stores conversations keyed by ID
	conversations map[string]*models.Conversation
	// participants stores participant lists keyed by conversation ID
	participants map[string][]string
	// userConversations stores conversation IDs keyed by user ID
	userConversations map[string][]string
	// states stores canonical per-user DM state keyed by "viewerID:conversationID"
	states map[string]*models.UserConversationState
	// readStatus stores read status keyed by "conversationID:username"
	readStatus map[string]bool
	// mutes stores mutes keyed by "username:conversationID"
	mutes map[string]*storage.ConversationMute
}

// NewConversationRepository creates a new in-memory conversation repository
func NewConversationRepository() *ConversationRepository {
	return &ConversationRepository{
		conversations:     make(map[string]*models.Conversation),
		participants:      make(map[string][]string),
		userConversations: make(map[string][]string),
		states:            make(map[string]*models.UserConversationState),
		readStatus:        make(map[string]bool),
		mutes:             make(map[string]*storage.ConversationMute),
	}
}

// CreateConversation creates a new conversation with participants
func (r *ConversationRepository) CreateConversation(_ context.Context, conversation *models.Conversation, participants []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.conversations[conversation.ID]; exists {
		return storage.ErrAlreadyExists
	}
	conversation.ParticipantRefs = models.NormalizeConversationParticipantRefs(conversation.ParticipantRefs)
	if len(conversation.ParticipantRefs) > 0 {
		conversation.Participants = models.ConversationParticipantIDsFromRefs(conversation.ParticipantRefs)
	} else {
		conversation.Participants = models.CanonicalConversationParticipants(participants)
	}
	r.conversations[conversation.ID] = conversation
	r.participants[conversation.ID] = append([]string(nil), conversation.Participants...)
	for _, p := range models.ConversationLocalParticipantIDs(conversation) {
		r.userConversations[p] = append(r.userConversations[p], conversation.ID)
		key := p + ":" + conversation.ID
		r.states[key] = &models.UserConversationState{
			ViewerID:       p,
			ConversationID: conversation.ID,
			CounterpartID:  counterpartIDForInMemoryState(p, conversation.Participants),
			Folder:         models.UserConversationFolderInbox,
			SortAt:         conversation.UpdatedAt,
			CreatedAt:      conversation.CreatedAt,
			UpdatedAt:      conversation.UpdatedAt,
		}
	}
	return nil
}

// CreateConversationWithParticipantStates creates a new conversation while ignoring the
// explicit participant states in the in-memory test repository.
func (r *ConversationRepository) CreateConversationWithParticipantStates(ctx context.Context, conversation *models.Conversation, participants []string, participantStates []*models.UserConversationState) error {
	if err := r.CreateConversation(ctx, conversation, participants); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, state := range participantStates {
		if state == nil {
			continue
		}
		cloned := *state
		r.states[state.ViewerID+":"+state.ConversationID] = &cloned
	}
	return nil
}

// GetConversation retrieves a conversation by ID
func (r *ConversationRepository) GetConversation(_ context.Context, id string) (*models.Conversation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conv, exists := r.conversations[id]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return conv, nil
}

// UpdateConversation updates a conversation
func (r *ConversationRepository) UpdateConversation(_ context.Context, conversation *models.Conversation) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.conversations[conversation.ID]; !exists {
		return storage.ErrNotFound
	}
	r.conversations[conversation.ID] = conversation
	return nil
}

// ApplyDirectMessageSend applies the canonical DM send transition in memory.
func (r *ConversationRepository) ApplyDirectMessageSend(_ context.Context, transition *models.DirectMessageSendTransition, _ interfaces.DirectMessageStatusStageFn) error {
	if transition == nil || transition.Conversation == nil || transition.Status == nil {
		return storage.ErrInvalidInput
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	conversation := transition.Conversation
	if transition.CreateConversation {
		if _, exists := r.conversations[conversation.ID]; exists {
			return storage.ErrAlreadyExists
		}
		r.conversations[conversation.ID] = conversation
		r.participants[conversation.ID] = append([]string(nil), conversation.Participants...)
		for _, participantID := range models.ConversationLocalParticipantIDs(conversation) {
			r.userConversations[participantID] = append(r.userConversations[participantID], conversation.ID)
		}
	} else {
		if _, exists := r.conversations[conversation.ID]; !exists {
			return storage.ErrNotFound
		}
		r.conversations[conversation.ID] = conversation
	}

	for _, state := range transition.ParticipantStates {
		if state == nil {
			continue
		}
		cloned := *state
		r.states[state.ViewerID+":"+state.ConversationID] = &cloned
	}

	senderID := transition.Status.AuthorID
	for _, participantID := range models.ConversationLocalParticipantIDs(conversation) {
		r.readStatus[conversation.ID+":"+participantID] = participantID == senderID
	}

	return nil
}

// DeleteConversation deletes a conversation by ID
func (r *ConversationRepository) DeleteConversation(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.conversations[id]; !exists {
		return storage.ErrNotFound
	}
	// Remove from user conversations
	for _, p := range r.participants[id] {
		convs := r.userConversations[p]
		for i, cid := range convs {
			if cid == id {
				r.userConversations[p] = append(convs[:i], convs[i+1:]...)
				break
			}
		}
	}
	delete(r.conversations, id)
	delete(r.participants, id)
	return nil
}

// GetUserConversations retrieves conversations for a user with pagination
func (r *ConversationRepository) GetUserConversations(_ context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	convIDs := r.userConversations[userID]
	var results []*models.Conversation
	for _, id := range convIDs {
		if conv, exists := r.conversations[id]; exists {
			results = append(results, conv)
		}
	}

	// Sort by updated time descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].UpdatedAt.After(results[j].UpdatedAt)
	})

	// Apply pagination
	total := len(results)
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return &interfaces.PaginatedResult[*models.Conversation]{
		Items:   results,
		Total:   int64(total),
		HasMore: total > len(results),
	}, nil
}

// GetUserConversationsByFolder retrieves conversations filtered by the canonical viewer folder.
func (r *ConversationRepository) GetUserConversationsByFolder(ctx context.Context, userID string, folder models.UserConversationFolder, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	switch folder {
	case models.UserConversationFolderRequests:
		return r.GetUserConversationsByRequestState(ctx, userID, models.DmRequestStatePending, opts)
	default:
		return r.GetUserConversationsByRequestState(ctx, userID, models.DmRequestStateAccepted, opts)
	}
}

// GetUserConversationsByRequestState retrieves conversations for a user filtered by request state.
// The in-memory repository does not model DM request lifecycle; it treats all conversations as accepted.
func (r *ConversationRepository) GetUserConversationsByRequestState(ctx context.Context, userID string, requestState models.DmRequestState, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	if requestState != models.DmRequestStateAccepted {
		return &interfaces.PaginatedResult[*models.Conversation]{
			Items:   []*models.Conversation{},
			Total:   0,
			HasMore: false,
		}, nil
	}
	return r.GetUserConversations(ctx, userID, opts)
}

// GetConversationByParticipants finds a conversation with exact participants
func (r *ConversationRepository) GetConversationByParticipants(_ context.Context, participants []string) (*models.Conversation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sort.Strings(participants)
	for id, convParticipants := range r.participants {
		sorted := make([]string, len(convParticipants))
		copy(sorted, convParticipants)
		sort.Strings(sorted)
		if len(sorted) == len(participants) {
			match := true
			for i := range sorted {
				if sorted[i] != participants[i] {
					match = false
					break
				}
			}
			if match {
				return r.conversations[id], nil
			}
		}
	}
	return nil, storage.ErrNotFound
}

// GetConversationByParticipantRefs finds a conversation with exact typed participants.
func (r *ConversationRepository) GetConversationByParticipantRefs(_ context.Context, refs []models.ConversationParticipantRef) (*models.Conversation, error) {
	participants := models.ConversationParticipantIDsFromRefs(refs)
	return r.GetConversationByParticipants(context.Background(), participants)
}

func counterpartIDForInMemoryState(viewerID string, participants []string) string {
	for _, participantID := range participants {
		if participantID != viewerID {
			return participantID
		}
	}
	return ""
}

// GetUserConversationState retrieves the canonical per-user DM state for a conversation.
func (r *ConversationRepository) GetUserConversationState(_ context.Context, viewerID, conversationID string) (*interfaces.UserConversationStateContract, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	state, exists := r.states[viewerID+":"+conversationID]
	if !exists {
		return nil, storage.ErrNotFound
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
		LastReadAt:               state.LastReadAt,
		DeletedAt:                state.DeletedAt,
		RequestedAt:              state.RequestedAt,
		AcceptedAt:               state.AcceptedAt,
		DeclinedAt:               state.DeclinedAt,
		CreatedAt:                state.CreatedAt,
		UpdatedAt:                state.UpdatedAt,
	}, nil
}

// PutUserConversationState persists a canonical per-user DM state row.
func (r *ConversationRepository) PutUserConversationState(_ context.Context, state *models.UserConversationState) error {
	if state == nil {
		return storage.ErrInvalidInput
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	cloned := *state
	r.states[state.ViewerID+":"+state.ConversationID] = &cloned
	return nil
}

// ListUserConversationStatesByFolder lists canonical per-user DM state rows by folder.
func (r *ConversationRepository) ListUserConversationStatesByFolder(_ context.Context, viewerID string, folder interfaces.UserConversationFolder, _ interfaces.PaginationOptions) (*interfaces.PaginatedResult[*interfaces.UserConversationStateContract], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]*interfaces.UserConversationStateContract, 0)
	for key, state := range r.states {
		if !strings.HasPrefix(key, viewerID+":") || state == nil || state.Folder != folder {
			continue
		}
		items = append(items, &interfaces.UserConversationStateContract{
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
			LastReadAt:               state.LastReadAt,
			DeletedAt:                state.DeletedAt,
			RequestedAt:              state.RequestedAt,
			AcceptedAt:               state.AcceptedAt,
			DeclinedAt:               state.DeclinedAt,
			CreatedAt:                state.CreatedAt,
			UpdatedAt:                state.UpdatedAt,
		})
	}

	return &interfaces.PaginatedResult[*interfaces.UserConversationStateContract]{Items: items, Total: int64(len(items))}, nil
}

// ListUnreadUserConversationStates lists canonical unread per-user DM state rows.
func (r *ConversationRepository) ListUnreadUserConversationStates(_ context.Context, viewerID string, _ interfaces.PaginationOptions) (*interfaces.PaginatedResult[*interfaces.UserConversationStateContract], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]*interfaces.UserConversationStateContract, 0)
	for key, state := range r.states {
		if !strings.HasPrefix(key, viewerID+":") || state == nil || !state.Unread {
			continue
		}
		items = append(items, &interfaces.UserConversationStateContract{
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
			LastReadAt:               state.LastReadAt,
			DeletedAt:                state.DeletedAt,
			RequestedAt:              state.RequestedAt,
			AcceptedAt:               state.AcceptedAt,
			DeclinedAt:               state.DeclinedAt,
			CreatedAt:                state.CreatedAt,
			UpdatedAt:                state.UpdatedAt,
		})
	}

	return &interfaces.PaginatedResult[*interfaces.UserConversationStateContract]{Items: items, Total: int64(len(items))}, nil
}

// ListConversationParticipantStates lists all canonical per-user DM state rows for a conversation.
func (r *ConversationRepository) ListConversationParticipantStates(_ context.Context, conversationID string) ([]*interfaces.UserConversationStateContract, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]*interfaces.UserConversationStateContract, 0)
	suffix := ":" + conversationID
	for key, state := range r.states {
		if !strings.HasSuffix(key, suffix) || state == nil {
			continue
		}
		items = append(items, &interfaces.UserConversationStateContract{
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
			LastReadAt:               state.LastReadAt,
			DeletedAt:                state.DeletedAt,
			RequestedAt:              state.RequestedAt,
			AcceptedAt:               state.AcceptedAt,
			DeclinedAt:               state.DeclinedAt,
			CreatedAt:                state.CreatedAt,
			UpdatedAt:                state.UpdatedAt,
		})
	}
	return items, nil
}

// GetUnreadConversations retrieves unread conversations for a user
func (r *ConversationRepository) GetUnreadConversations(_ context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	convIDs := r.userConversations[userID]
	var results []*models.Conversation
	for _, id := range convIDs {
		key := id + ":" + userID
		if !r.readStatus[key] {
			if conv, exists := r.conversations[id]; exists {
				results = append(results, conv)
			}
		}
	}

	total := len(results)
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return &interfaces.PaginatedResult[*models.Conversation]{
		Items:   results,
		Total:   int64(total),
		HasMore: total > len(results),
	}, nil
}

// SearchConversations searches conversations for a user by query
func (r *ConversationRepository) SearchConversations(_ context.Context, userID, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	queryLower := strings.ToLower(query)
	convIDs := r.userConversations[userID]
	var results []*models.Conversation
	for _, id := range convIDs {
		if conv, exists := r.conversations[id]; exists {
			// Search in participant names
			for _, p := range conv.Participants {
				if strings.Contains(strings.ToLower(p), queryLower) {
					results = append(results, conv)
					break
				}
			}
		}
	}

	total := len(results)
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return &interfaces.PaginatedResult[*models.Conversation]{
		Items:   results,
		Total:   int64(total),
		HasMore: total > len(results),
	}, nil
}

// MarkConversationRead marks a conversation as read for a user
func (r *ConversationRepository) MarkConversationRead(_ context.Context, conversationID, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := conversationID + ":" + username
	r.readStatus[key] = true
	return nil
}

// MarkConversationUnread marks a conversation as unread for a user
func (r *ConversationRepository) MarkConversationUnread(_ context.Context, conversationID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := conversationID + ":" + userID
	r.readStatus[key] = false
	return nil
}

// GetUnreadConversationCount gets the count of unread conversations for a user
func (r *ConversationRepository) GetUnreadConversationCount(_ context.Context, username string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, id := range r.userConversations[username] {
		key := id + ":" + username
		if !r.readStatus[key] {
			count++
		}
	}
	return count, nil
}

// GetConversationParticipants retrieves the list of participants in a conversation
func (r *ConversationRepository) GetConversationParticipants(_ context.Context, conversationID string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	parts, exists := r.participants[conversationID]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return parts, nil
}

// CreateConversationMute creates a new conversation mute
func (r *ConversationRepository) CreateConversationMute(_ context.Context, mute *storage.ConversationMute) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := mute.Username + ":" + mute.ConversationID
	r.mutes[key] = mute
	return nil
}

// DeleteConversationMute removes a conversation mute
func (r *ConversationRepository) DeleteConversationMute(_ context.Context, username, conversationID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := username + ":" + conversationID
	delete(r.mutes, key)
	return nil
}

// IsConversationMuted checks if a conversation is muted by a user
func (r *ConversationRepository) IsConversationMuted(_ context.Context, username, conversationID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := username + ":" + conversationID
	_, exists := r.mutes[key]
	return exists, nil
}

// GetMutedConversations retrieves all muted conversations for a user
func (r *ConversationRepository) GetMutedConversations(_ context.Context, username string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []string
	prefix := username + ":"
	for key := range r.mutes {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			results = append(results, key[len(prefix):])
		}
	}
	return results, nil
}

// Clear clears all data (test helper)
func (r *ConversationRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conversations = make(map[string]*models.Conversation)
	r.participants = make(map[string][]string)
	r.userConversations = make(map[string][]string)
	r.states = make(map[string]*models.UserConversationState)
	r.readStatus = make(map[string]bool)
	r.mutes = make(map[string]*storage.ConversationMute)
}

// Ensure ConversationRepository implements interfaces.ConversationRepository
var _ interfaces.ConversationRepository = (*ConversationRepository)(nil)
