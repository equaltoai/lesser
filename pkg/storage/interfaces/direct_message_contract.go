// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// UserConversationFolder identifies the viewer-visible placement of a DM thread.
// This is the M0 contract that the future UserConversationState storage model will materialize.
type UserConversationFolder string

const (
	// UserConversationFolderInbox is the default accepted DM list.
	UserConversationFolderInbox UserConversationFolder = "INBOX"
	// UserConversationFolderRequests contains inbound threads awaiting viewer acceptance.
	UserConversationFolderRequests UserConversationFolder = "REQUESTS"
	// UserConversationFolderDeclined contains threads the viewer explicitly declined.
	UserConversationFolderDeclined UserConversationFolder = "DECLINED"
	// UserConversationFolderHidden contains viewer-hidden threads (delete-for-me/archive).
	UserConversationFolderHidden UserConversationFolder = "HIDDEN"
)

// UserConversationStateContract is the M0 contract shape for canonical per-user DM state.
// M1 will add the concrete storage model under pkg/storage/models and make repository
// implementations return that row rather than snapshot-hydrated conversation data.
type UserConversationStateContract struct {
	ViewerID                 string
	ConversationID           string
	CounterpartID            string
	Folder                   UserConversationFolder
	RequestState             models.DmRequestState
	PreviewStatusID          string
	PreviewStatusPublishedAt time.Time
	SortAt                   time.Time
	Unread                   bool
	LastReadAt               *time.Time
	DeletedAt                *time.Time
	RequestedAt              *time.Time
	AcceptedAt               *time.Time
	DeclinedAt               *time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// DirectMessageRepository is the target DM-facing repository contract for the rewrite.
// It intentionally describes stable point reads and keyed queries over canonical per-user
// state instead of scan-shaped list methods or snapshot-hydrated participant records.
type DirectMessageRepository interface {
	// GetConversation retrieves shared conversation metadata by conversation ID.
	GetConversation(ctx context.Context, id string) (*models.Conversation, error)

	// GetConversationByParticipants resolves the 1:1 conversation identity for an exact participant set.
	GetConversationByParticipants(ctx context.Context, participants []string) (*models.Conversation, error)

	// GetUserConversationState point-reads the viewer's canonical DM state for a conversation.
	GetUserConversationState(ctx context.Context, viewerID, conversationID string) (*UserConversationStateContract, error)

	// ListUserConversationStatesByFolder queries the viewer's folder index without scan-side filtering.
	ListUserConversationStatesByFolder(ctx context.Context, viewerID string, folder UserConversationFolder, opts PaginationOptions) (*PaginatedResult[*UserConversationStateContract], error)

	// ListUnreadUserConversationStates queries the viewer's sparse unread index without consulting legacy unread rows.
	ListUnreadUserConversationStates(ctx context.Context, viewerID string, opts PaginationOptions) (*PaginatedResult[*UserConversationStateContract], error)

	// ListConversationParticipantStates reverse-queries all per-user DM rows for a shared conversation.
	ListConversationParticipantStates(ctx context.Context, conversationID string) ([]*UserConversationStateContract, error)
}
