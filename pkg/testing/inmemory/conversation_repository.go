// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

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
	// statuses stores conversation statuses keyed by "conversationID:statusID"
	statuses map[string]*storage.ConversationStatus
	// statusesByConv stores status keys keyed by conversation ID
	statusesByConv map[string][]string
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
		statuses:          make(map[string]*storage.ConversationStatus),
		statusesByConv:    make(map[string][]string),
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
	r.conversations[conversation.ID] = conversation
	r.participants[conversation.ID] = participants
	for _, p := range participants {
		r.userConversations[p] = append(r.userConversations[p], conversation.ID)
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

// AddStatusToConversation adds a status/message to a conversation
func (r *ConversationRepository) AddStatusToConversation(_ context.Context, conversationID, statusID, senderUsername string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := conversationID + ":" + statusID
	r.statuses[key] = &storage.ConversationStatus{
		ConversationID: conversationID,
		StatusID:       statusID,
		UserID:         senderUsername,
		CreatedAt:      time.Now(),
	}
	r.statusesByConv[conversationID] = append(r.statusesByConv[conversationID], key)
	return nil
}

// GetConversationStatuses retrieves messages in a conversation with pagination
func (r *ConversationRepository) GetConversationStatuses(_ context.Context, conversationID string, limit int, _ string) ([]*storage.ConversationStatus, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := r.statusesByConv[conversationID]
	var results []*storage.ConversationStatus
	for _, key := range keys {
		if status, exists := r.statuses[key]; exists {
			results = append(results, status)
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, "", nil
}

// RemoveStatusFromConversation removes a status from a conversation
func (r *ConversationRepository) RemoveStatusFromConversation(_ context.Context, conversationID, statusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := conversationID + ":" + statusID
	delete(r.statuses, key)
	return nil
}

// MarkStatusRead marks a specific status as read by a user
func (r *ConversationRepository) MarkStatusRead(_ context.Context, conversationID, statusID, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := conversationID + ":" + statusID + ":" + username
	r.readStatus[key] = true
	return nil
}

// GetUnreadStatusCount gets the count of unread statuses in a conversation for a user
func (r *ConversationRepository) GetUnreadStatusCount(_ context.Context, conversationID, username string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, key := range r.statusesByConv[conversationID] {
		readKey := key + ":" + username
		if !r.readStatus[readKey] {
			count++
		}
	}
	return count, nil
}

// UpdateConversationLastStatus updates the last status in a conversation
func (r *ConversationRepository) UpdateConversationLastStatus(_ context.Context, id, lastStatusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	conv, exists := r.conversations[id]
	if !exists {
		return storage.ErrNotFound
	}
	conv.LastStatusID = lastStatusID
	conv.UpdatedAt = time.Now()
	return nil
}

// AddParticipant adds a participant to a conversation
func (r *ConversationRepository) AddParticipant(_ context.Context, conversationID, participantID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.participants[conversationID] = append(r.participants[conversationID], participantID)
	r.userConversations[participantID] = append(r.userConversations[participantID], conversationID)
	return nil
}

// RemoveParticipant removes a participant from a conversation
func (r *ConversationRepository) RemoveParticipant(_ context.Context, conversationID, participantID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove from participants
	parts := r.participants[conversationID]
	for i, p := range parts {
		if p == participantID {
			r.participants[conversationID] = append(parts[:i], parts[i+1:]...)
			break
		}
	}
	// Remove from user conversations
	convs := r.userConversations[participantID]
	for i, c := range convs {
		if c == conversationID {
			r.userConversations[participantID] = append(convs[:i], convs[i+1:]...)
			break
		}
	}
	return nil
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

// LeaveConversation removes a participant from a conversation
func (r *ConversationRepository) LeaveConversation(ctx context.Context, conversationID, username string) error {
	return r.RemoveParticipant(ctx, conversationID, username)
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
	r.statuses = make(map[string]*storage.ConversationStatus)
	r.statusesByConv = make(map[string][]string)
	r.readStatus = make(map[string]bool)
	r.mutes = make(map[string]*storage.ConversationMute)
}

// Ensure ConversationRepository implements interfaces.ConversationRepository
var _ interfaces.ConversationRepository = (*ConversationRepository)(nil)
