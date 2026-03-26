// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ConversationRepository defines the interface for conversation operations.
// This handles direct message conversations, participants, and message threading.
//
// NOTE: This is the active legacy DM repository surface. The target rewrite contract is
// DirectMessageRepository in direct_message_contract.go, which replaces snapshot-hydrated
// participant records and scan-shaped list queries with canonical per-user DM state reads.
type ConversationRepository interface {
	// ===== Core Conversation Operations =====

	// CreateConversation creates a new conversation with participants
	CreateConversation(ctx context.Context, conversation *models.Conversation, participants []string) error

	// GetConversation retrieves a conversation by ID
	GetConversation(ctx context.Context, id string) (*models.Conversation, error)

	// UpdateConversation updates a conversation
	UpdateConversation(ctx context.Context, conversation *models.Conversation) error

	// DeleteConversation deletes a conversation by ID
	DeleteConversation(ctx context.Context, id string) error

	// ===== User Conversation Operations =====

	// GetUserConversations retrieves conversations for a user with pagination.
	// Legacy note: DM rewrite M1 replaces this snapshot-shaped list method with
	// DirectMessageRepository.ListUserConversationStatesByFolder.
	GetUserConversations(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Conversation], error)

	// GetUserConversationsByRequestState retrieves conversations for a user filtered by the participant
	// request state (e.g., inbox vs requests). Legacy note: DM rewrite M1 replaces request-state
	// filtering with keyed folder queries on canonical per-user DM state.
	GetUserConversationsByRequestState(ctx context.Context, userID string, requestState models.DmRequestState, opts PaginationOptions) (*PaginatedResult[*models.Conversation], error)

	// GetConversationByParticipants finds a conversation with exact participants
	GetConversationByParticipants(ctx context.Context, participants []string) (*models.Conversation, error)

	// GetUnreadConversations retrieves unread conversations for a user.
	// Legacy note: DM rewrite M1 replaces legacy unread listing with
	// DirectMessageRepository.ListUnreadUserConversationStates.
	GetUnreadConversations(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Conversation], error)

	// SearchConversations searches conversations for a user by query
	SearchConversations(ctx context.Context, userID, query string, opts PaginationOptions) (*PaginatedResult[*models.Conversation], error)

	// ===== Read Status Operations =====

	// MarkConversationRead marks a conversation as read for a user.
	// Legacy note: DM rewrite M4 moves read/unread truth onto canonical per-user DM state
	// instead of ConversationStatus compatibility rows.
	MarkConversationRead(ctx context.Context, conversationID, username string) error

	// MarkConversationUnread marks a conversation as unread for a user.
	// Legacy note: DM rewrite M4 moves read/unread truth onto canonical per-user DM state
	// instead of ConversationStatus compatibility rows.
	MarkConversationUnread(ctx context.Context, conversationID, userID string) error

	// GetUnreadConversationCount gets the count of unread conversations for a user.
	// Legacy note: DM rewrite M4/M5 replaces fan-out unread counting with keyed unread-state queries.
	GetUnreadConversationCount(ctx context.Context, username string) (int, error)

	// ===== Status/Message Operations =====

	// AddStatusToConversation adds a status/message to a conversation.
	// Legacy note: DM rewrite M3/M8 removes ConversationMessage as a canonical DM write path.
	AddStatusToConversation(ctx context.Context, conversationID, statusID, senderUsername string) error

	// GetConversationStatuses retrieves messages in a conversation with pagination.
	// Legacy note: DM rewrite M5 keeps thread reads on StatusRepository.GetConversationThread
	// instead of conversation-local message rows.
	GetConversationStatuses(ctx context.Context, conversationID string, limit int, cursor string) ([]*storage.ConversationStatus, string, error)

	// RemoveStatusFromConversation removes a status from a conversation.
	// Legacy note: DM rewrite M3/M8 removes ConversationMessage as a canonical DM write path.
	RemoveStatusFromConversation(ctx context.Context, conversationID, statusID string) error

	// MarkStatusRead marks a specific status as read by a user.
	// Legacy note: DM rewrite M4 removes message-level read truth from the conversation repository.
	MarkStatusRead(ctx context.Context, conversationID, statusID, username string) error

	// GetUnreadStatusCount gets the count of unread statuses in a conversation for a user.
	// Legacy note: DM rewrite M4 removes unread truth from ConversationStatus compatibility rows.
	GetUnreadStatusCount(ctx context.Context, conversationID, username string) (int, error)

	// UpdateConversationLastStatus updates the last status in a conversation
	UpdateConversationLastStatus(ctx context.Context, id, lastStatusID string) error

	// ===== Participant Operations =====

	// AddParticipant adds a participant to a conversation
	AddParticipant(ctx context.Context, conversationID, participantID string) error

	// RemoveParticipant removes a participant from a conversation
	RemoveParticipant(ctx context.Context, conversationID, participantID string) error

	// GetConversationParticipants retrieves the list of participants in a conversation
	GetConversationParticipants(ctx context.Context, conversationID string) ([]string, error)

	// GetConversationParticipantRecord retrieves the most recent participant record for a given
	// (conversationID, participantID) pair. Legacy note: DM rewrite M1 replaces snapshot-hydrated
	// participant records with point-readable UserConversationState rows.
	GetConversationParticipantRecord(ctx context.Context, conversationID, participantID string) (*models.ConversationParticipantRecord, error)

	// UpdateConversationParticipantRecord persists an updated participant record.
	// Legacy note: DM rewrite M1 replaces snapshot-hydrated participant records with canonical
	// per-user DM state writes.
	UpdateConversationParticipantRecord(ctx context.Context, record *models.ConversationParticipantRecord) error

	// LeaveConversation removes a participant from a conversation
	LeaveConversation(ctx context.Context, conversationID, username string) error

	// ===== Mute Operations =====

	// CreateConversationMute creates a new conversation mute
	CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error

	// DeleteConversationMute removes a conversation mute
	DeleteConversationMute(ctx context.Context, username, conversationID string) error

	// IsConversationMuted checks if a conversation is muted by a user
	IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error)

	// GetMutedConversations retrieves all muted conversations for a user
	GetMutedConversations(ctx context.Context, username string) ([]string, error)
}
