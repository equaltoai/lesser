// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ThreadContextResult represents the complete context of a thread
type ThreadContextResult struct {
	RootStatusID      string
	RequestedStatusID string
	Nodes             []*models.ThreadNode
	MissingReplies    []*models.MissingReply

	// Calculated stats
	ParticipantCount int
	TotalReplyCount  int
	MissingCount     int
	MaxDepth         int
}

// ThreadRepository defines the interface for thread synchronization and traversal operations.
// This handles thread sync records, thread nodes, and missing reply tracking.
type ThreadRepository interface {
	// ===== Thread Sync Operations =====

	// SaveThreadSync saves or updates a thread sync record
	SaveThreadSync(ctx context.Context, sync *models.ThreadSync) error

	// GetThreadSync retrieves a thread sync record by status ID
	GetThreadSync(ctx context.Context, statusID string) (*models.ThreadSync, error)

	// ===== Thread Node Operations =====

	// SaveThreadNode saves or updates a thread node
	SaveThreadNode(ctx context.Context, node *models.ThreadNode) error

	// GetThreadNodes retrieves all nodes for a thread by root status ID
	GetThreadNodes(ctx context.Context, rootStatusID string) ([]*models.ThreadNode, error)

	// GetThreadNode retrieves a single thread node by status ID
	GetThreadNode(ctx context.Context, rootStatusID, statusID string) (*models.ThreadNode, error)

	// GetThreadNodeByStatusID retrieves a thread node by status ID using GSI
	GetThreadNodeByStatusID(ctx context.Context, statusID string) (*models.ThreadNode, error)

	// BulkSaveThreadNodes saves multiple thread nodes in a batch
	BulkSaveThreadNodes(ctx context.Context, nodes []*models.ThreadNode) error

	// ===== Missing Reply Operations =====

	// MarkMissingReplies marks multiple replies as missing in a thread
	MarkMissingReplies(ctx context.Context, rootStatusID, parentStatusID string, replyIDs []string) error

	// GetMissingReplies retrieves all missing replies for a thread
	GetMissingReplies(ctx context.Context, rootStatusID string) ([]*models.MissingReply, error)

	// SaveMissingReply saves or updates a missing reply record
	SaveMissingReply(ctx context.Context, missing *models.MissingReply) error

	// DeleteMissingReply deletes a missing reply record (used when resolved)
	DeleteMissingReply(ctx context.Context, rootStatusID, replyID string) error

	// GetPendingMissingReplies retrieves missing replies that should be retried
	GetPendingMissingReplies(ctx context.Context, limit int) ([]*models.MissingReply, error)

	// ===== Thread Context Operations =====

	// GetThreadContext builds a complete thread context by querying nodes
	GetThreadContext(ctx context.Context, statusID string) (*ThreadContextResult, error)
}
