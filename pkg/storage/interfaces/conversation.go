// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ConversationRepository defines the interface for conversation operations.
// This handles direct message conversations, participants, and message threading.
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

	// GetUserConversations retrieves conversations for a user with pagination
	GetUserConversations(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Conversation], error)

	// GetConversationByParticipants finds a conversation with exact participants
	GetConversationByParticipants(ctx context.Context, participants []string) (*models.Conversation, error)

	// GetUnreadConversations retrieves unread conversations for a user
	GetUnreadConversations(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Conversation], error)

	// SearchConversations searches conversations for a user by query
	SearchConversations(ctx context.Context, userID, query string, opts PaginationOptions) (*PaginatedResult[*models.Conversation], error)

	// ===== Read Status Operations =====

	// MarkConversationRead marks a conversation as read for a user
	MarkConversationRead(ctx context.Context, conversationID, username string) error

	// MarkConversationUnread marks a conversation as unread for a user
	MarkConversationUnread(ctx context.Context, conversationID, userID string) error

	// GetUnreadConversationCount gets the count of unread conversations for a user
	GetUnreadConversationCount(ctx context.Context, username string) (int, error)

	// ===== Status/Message Operations =====

	// AddStatusToConversation adds a status/message to a conversation
	AddStatusToConversation(ctx context.Context, conversationID, statusID, senderUsername string) error

	// GetConversationStatuses retrieves messages in a conversation with pagination
	GetConversationStatuses(ctx context.Context, conversationID string, limit int, cursor string) ([]*storage.ConversationStatus, string, error)

	// RemoveStatusFromConversation removes a status from a conversation
	RemoveStatusFromConversation(ctx context.Context, conversationID, statusID string) error

	// MarkStatusRead marks a specific status as read by a user
	MarkStatusRead(ctx context.Context, conversationID, statusID, username string) error

	// GetUnreadStatusCount gets the count of unread statuses in a conversation for a user
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
