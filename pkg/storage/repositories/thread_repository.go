package repositories

import (
	"context"
	stdErrors "errors"
	"fmt"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ThreadRepository handles thread synchronization and traversal operations
type ThreadRepository struct {
	db     core.DB
	logger *zap.Logger
}

// NewThreadRepository creates a new ThreadRepository instance
func NewThreadRepository(db core.DB, logger *zap.Logger) *ThreadRepository {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ThreadRepository{
		db:     db,
		logger: logger,
	}
}

// SaveThreadSync saves or updates a thread sync record
func (r *ThreadRepository) SaveThreadSync(ctx context.Context, sync *models.ThreadSync) error {
	if sync == nil {
		return fmt.Errorf("thread sync cannot be nil")
	}

	// Update keys before saving
	if err := sync.UpdateKeys(); err != nil {
		return err
	}

	// Use Create to save (will upsert)
	err := r.db.WithContext(ctx).Model(sync).Create()
	if err != nil {
		r.logger.Error("failed to save thread sync",
			zap.String("status_id", sync.StatusID),
			zap.Error(err))
		return fmt.Errorf("failed to save thread sync %s: %w", sync.StatusID, err)
	}

	r.logger.Debug("saved thread sync",
		zap.String("status_id", sync.StatusID),
		zap.String("sync_status", sync.SyncStatus))

	return nil
}

// GetThreadSync retrieves a thread sync record by status ID
func (r *ThreadRepository) GetThreadSync(ctx context.Context, statusID string) (*models.ThreadSync, error) {
	pk := fmt.Sprintf("THREAD_SYNC#%s", statusID)
	sk := models.SKMetadata

	var sync models.ThreadSync
	err := r.db.WithContext(ctx).Model(&models.ThreadSync{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&sync)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "thread sync", statusID)
		}
		r.logger.Error("failed to get thread sync",
			zap.String("status_id", statusID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "thread sync", statusID)
	}

	return &sync, nil
}

// SaveThreadNode saves or updates a thread node
func (r *ThreadRepository) SaveThreadNode(ctx context.Context, node *models.ThreadNode) error {
	if node == nil {
		return fmt.Errorf("thread node cannot be nil")
	}

	// Update keys before saving
	if err := node.UpdateKeys(); err != nil {
		return err
	}

	// Use Create to save (will upsert)
	err := r.db.WithContext(ctx).Model(node).Create()
	if err != nil {
		r.logger.Error("failed to save thread node",
			zap.String("status_id", node.StatusID),
			zap.String("root_status_id", node.RootStatusID),
			zap.Error(err))
		return fmt.Errorf("failed to save thread node %s: %w", node.StatusID, err)
	}

	r.logger.Debug("saved thread node",
		zap.String("status_id", node.StatusID),
		zap.String("root_status_id", node.RootStatusID),
		zap.Int("depth", node.Depth))

	return nil
}

// GetThreadNodes retrieves all nodes for a thread by root status ID
func (r *ThreadRepository) GetThreadNodes(ctx context.Context, rootStatusID string) ([]*models.ThreadNode, error) {
	pk := fmt.Sprintf("THREAD#%s", rootStatusID)

	// Query all nodes with PK = THREAD#{rootStatusID} and SK begins_with NODE#
	var nodes []*models.ThreadNode
	err := r.db.WithContext(ctx).Model(&models.ThreadNode{}).
		Where("PK", "=", pk).
		Where("SK", "begins_with", "NODE#").
		Scan(&nodes)

	if err != nil {
		r.logger.Error("failed to get thread nodes",
			zap.String("root_status_id", rootStatusID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get thread nodes for %s: %w", rootStatusID, err)
	}

	r.logger.Debug("retrieved thread nodes",
		zap.String("root_status_id", rootStatusID),
		zap.Int("count", len(nodes)))

	return nodes, nil
}

// GetThreadNode retrieves a single thread node by status ID
func (r *ThreadRepository) GetThreadNode(ctx context.Context, rootStatusID, statusID string) (*models.ThreadNode, error) {
	pk := fmt.Sprintf("THREAD#%s", rootStatusID)
	sk := fmt.Sprintf("NODE#%s", statusID)

	var node models.ThreadNode
	err := r.db.WithContext(ctx).Model(&models.ThreadNode{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&node)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "thread node", statusID)
		}
		r.logger.Error("failed to get thread node",
			zap.String("status_id", statusID),
			zap.String("root_status_id", rootStatusID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "thread node", statusID)
	}

	return &node, nil
}

// GetThreadNodeByStatusID retrieves a thread node by status ID using GSI
func (r *ThreadRepository) GetThreadNodeByStatusID(ctx context.Context, statusID string) (*models.ThreadNode, error) {
	gsi1pk := fmt.Sprintf("STATUS#%s", statusID)
	gsi1sk := "THREAD_NODE"

	var node models.ThreadNode
	err := r.db.WithContext(ctx).Model(&models.ThreadNode{}).
		Index("gsi1").
		Where("gsi1PK", "=", gsi1pk).
		Where("gsi1SK", "=", gsi1sk).
		First(&node)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "thread node", statusID)
		}
		r.logger.Error("failed to get thread node by status ID",
			zap.String("status_id", statusID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "thread node", statusID)
	}

	return &node, nil
}

// MarkMissingReplies marks multiple replies as missing in a thread
func (r *ThreadRepository) MarkMissingReplies(ctx context.Context, rootStatusID, parentStatusID string, replyIDs []string) error {
	if len(replyIDs) == 0 {
		return nil // Nothing to mark
	}

	r.logger.Debug("marking missing replies",
		zap.String("root_status_id", rootStatusID),
		zap.String("parent_status_id", parentStatusID),
		zap.Int("count", len(replyIDs)))

	// Create and save MissingReply records for each reply ID
	for _, replyID := range replyIDs {
		missing := models.NewMissingReply(rootStatusID, parentStatusID, replyID)
		if err := missing.UpdateKeys(); err != nil {
			r.logger.Error("failed to update keys for missing reply",
				zap.String("reply_id", replyID),
				zap.Error(err))
			continue
		}

		err := r.db.WithContext(ctx).Model(missing).Create()
		if err != nil {
			r.logger.Error("failed to mark missing reply",
				zap.String("reply_id", replyID),
				zap.String("root_status_id", rootStatusID),
				zap.Error(err))
			// Continue with other replies even if one fails
			continue
		}
	}

	r.logger.Debug("marked missing replies",
		zap.String("root_status_id", rootStatusID),
		zap.Int("count", len(replyIDs)))

	return nil
}

// GetMissingReplies retrieves all missing replies for a thread
func (r *ThreadRepository) GetMissingReplies(ctx context.Context, rootStatusID string) ([]*models.MissingReply, error) {
	pk := fmt.Sprintf("THREAD#%s", rootStatusID)

	// Query all missing replies with PK = THREAD#{rootStatusID} and SK begins_with MISSING#
	var missing []*models.MissingReply
	err := r.db.WithContext(ctx).Model(&models.MissingReply{}).
		Where("PK", "=", pk).
		Where("SK", "begins_with", "MISSING#").
		Scan(&missing)

	if err != nil {
		r.logger.Error("failed to get missing replies",
			zap.String("root_status_id", rootStatusID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get missing replies for %s: %w", rootStatusID, err)
	}

	r.logger.Debug("retrieved missing replies",
		zap.String("root_status_id", rootStatusID),
		zap.Int("count", len(missing)))

	return missing, nil
}

// GetThreadContext builds a complete thread context by querying nodes
func (r *ThreadRepository) GetThreadContext(ctx context.Context, statusID string) (*ThreadContextResult, error) {
	// First, find the thread node for this status to get the root
	node, err := r.GetThreadNodeByStatusID(ctx, statusID)
	if err != nil {
		if stdErrors.Is(err, storage.ErrNotFound) {
			r.logger.Debug("no thread node found for status",
				zap.String("status_id", statusID))
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "thread context", statusID)
		}
		return nil, err
	}

	// Get all nodes in the thread
	nodes, err := r.GetThreadNodes(ctx, node.RootStatusID)
	if err != nil {
		return nil, err
	}

	// Get missing replies
	missingReplies, err := r.GetMissingReplies(ctx, node.RootStatusID)
	if err != nil {
		r.logger.Warn("failed to get missing replies, continuing without them",
			zap.String("root_status_id", node.RootStatusID),
			zap.Error(err))
		missingReplies = []*models.MissingReply{}
	}

	// Build the context result
	result := &ThreadContextResult{
		RootStatusID:      node.RootStatusID,
		RequestedStatusID: statusID,
		Nodes:             nodes,
		MissingReplies:    missingReplies,
	}

	// Calculate statistics
	result.calculateStats()

	r.logger.Debug("built thread context",
		zap.String("root_status_id", node.RootStatusID),
		zap.Int("total_nodes", len(nodes)),
		zap.Int("missing_count", len(missingReplies)))

	return result, nil
}

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

// calculateStats calculates statistics for the thread context
func (r *ThreadContextResult) calculateStats() {
	// Count unique participants
	participants := make(map[string]bool)
	maxDepth := 0
	totalReplies := 0

	for _, node := range r.Nodes {
		participants[node.AuthorID] = true
		if node.Depth > maxDepth {
			maxDepth = node.Depth
		}
		totalReplies += node.ReplyCount
	}

	r.ParticipantCount = len(participants)
	r.TotalReplyCount = totalReplies
	r.MaxDepth = maxDepth
	r.MissingCount = len(r.MissingReplies)
}

// GetRootNode returns the root node of the thread
func (r *ThreadContextResult) GetRootNode() *models.ThreadNode {
	for _, node := range r.Nodes {
		if node.IsRoot() {
			return node
		}
	}
	return nil
}

// GetNodesByDepth returns nodes organized by depth
func (r *ThreadContextResult) GetNodesByDepth() map[int][]*models.ThreadNode {
	byDepth := make(map[int][]*models.ThreadNode)
	for _, node := range r.Nodes {
		byDepth[node.Depth] = append(byDepth[node.Depth], node)
	}
	return byDepth
}

// GetChildren returns direct children of a given node
func (r *ThreadContextResult) GetChildren(parentID string) []*models.ThreadNode {
	var children []*models.ThreadNode
	for _, node := range r.Nodes {
		if node.ParentID == parentID {
			children = append(children, node)
		}
	}
	return children
}

// SaveMissingReply saves or updates a missing reply record
func (r *ThreadRepository) SaveMissingReply(ctx context.Context, missing *models.MissingReply) error {
	if missing == nil {
		return fmt.Errorf("missing reply cannot be nil")
	}

	// Update keys before saving
	if err := missing.UpdateKeys(); err != nil {
		return err
	}

	err := r.db.WithContext(ctx).Model(missing).Create()
	if err != nil {
		r.logger.Error("failed to save missing reply",
			zap.String("reply_id", missing.ReplyID),
			zap.String("root_status_id", missing.RootStatusID),
			zap.Error(err))
		return fmt.Errorf("failed to save missing reply %s: %w", missing.ReplyID, err)
	}

	r.logger.Debug("saved missing reply",
		zap.String("reply_id", missing.ReplyID),
		zap.String("status", missing.Status))

	return nil
}

// DeleteMissingReply deletes a missing reply record (used when resolved)
func (r *ThreadRepository) DeleteMissingReply(ctx context.Context, rootStatusID, replyID string) error {
	pk := fmt.Sprintf("THREAD#%s", rootStatusID)
	sk := fmt.Sprintf("MISSING#%s", replyID)

	missing := &models.MissingReply{}
	missing.PK = pk
	missing.SK = sk

	err := r.db.WithContext(ctx).Model(missing).Delete()
	if err != nil {
		r.logger.Error("failed to delete missing reply",
			zap.String("reply_id", replyID),
			zap.String("root_status_id", rootStatusID),
			zap.Error(err))
		return fmt.Errorf("failed to delete missing reply %s: %w", replyID, err)
	}

	r.logger.Debug("deleted missing reply",
		zap.String("reply_id", replyID))

	return nil
}

// GetPendingMissingReplies retrieves missing replies that should be retried
func (r *ThreadRepository) GetPendingMissingReplies(_ context.Context, _ int) ([]*models.MissingReply, error) {
	// This would ideally use a GSI on status, but for now we'll scan
	// In production, you'd want a GSI like: GSI2PK=MISSING_REPLY_STATUS#{status}, GSI2SK=nextRetryAt

	var allMissing []*models.MissingReply
	// We need to scan across all threads - this is expensive
	// In a real implementation, we'd use a dedicated GSI for this

	r.logger.Warn("GetPendingMissingReplies not optimized - needs GSI")

	return allMissing, nil
}

// BulkSaveThreadNodes saves multiple thread nodes in a batch
func (r *ThreadRepository) BulkSaveThreadNodes(ctx context.Context, nodes []*models.ThreadNode) error {
	if len(nodes) == 0 {
		return nil
	}

	r.logger.Debug("bulk saving thread nodes",
		zap.Int("count", len(nodes)))

	// Update keys for all nodes
	for _, node := range nodes {
		if err := node.UpdateKeys(); err != nil {
			r.logger.Error("failed to update keys for node",
				zap.String("status_id", node.StatusID),
				zap.Error(err))
			continue
		}
	}

	// Save in batches of 25 (DynamoDB batch write limit)
	const batchSize = 25
	for i := 0; i < len(nodes); i += batchSize {
		end := i + batchSize
		if end > len(nodes) {
			end = len(nodes)
		}

		batch := nodes[i:end]
		for _, node := range batch {
			err := r.db.WithContext(ctx).Model(node).Create()
			if err != nil {
				r.logger.Error("failed to save node in batch",
					zap.String("status_id", node.StatusID),
					zap.Error(err))
				// Continue with other nodes
				continue
			}
		}
	}

	r.logger.Debug("bulk saved thread nodes",
		zap.Int("count", len(nodes)))

	return nil
}
