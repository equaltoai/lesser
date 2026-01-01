// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sync"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ThreadRepository is a thread-safe in-memory implementation of interfaces.ThreadRepository.
type ThreadRepository struct {
	mu sync.RWMutex

	// Thread syncs: statusID -> ThreadSync
	syncs map[string]*models.ThreadSync

	// Thread nodes: rootStatusID -> statusID -> ThreadNode
	nodes map[string]map[string]*models.ThreadNode

	// Nodes by status ID: statusID -> ThreadNode (for GSI lookup)
	nodesByStatusID map[string]*models.ThreadNode

	// Missing replies: rootStatusID -> replyID -> MissingReply
	missingReplies map[string]map[string]*models.MissingReply
}

// NewThreadRepository creates a new in-memory thread repository
func NewThreadRepository() *ThreadRepository {
	return &ThreadRepository{
		syncs:           make(map[string]*models.ThreadSync),
		nodes:           make(map[string]map[string]*models.ThreadNode),
		nodesByStatusID: make(map[string]*models.ThreadNode),
		missingReplies:  make(map[string]map[string]*models.MissingReply),
	}
}

// ===== Thread Sync Operations =====

// SaveThreadSync saves or updates a thread sync record
func (r *ThreadRepository) SaveThreadSync(_ context.Context, sync *models.ThreadSync) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.syncs[sync.StatusID] = sync
	return nil
}

// GetThreadSync retrieves a thread sync record by status ID
func (r *ThreadRepository) GetThreadSync(_ context.Context, statusID string) (*models.ThreadSync, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sync, exists := r.syncs[statusID]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return sync, nil
}

// ===== Thread Node Operations =====

// SaveThreadNode saves or updates a thread node
func (r *ThreadRepository) SaveThreadNode(_ context.Context, node *models.ThreadNode) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.nodes[node.RootStatusID] == nil {
		r.nodes[node.RootStatusID] = make(map[string]*models.ThreadNode)
	}
	r.nodes[node.RootStatusID][node.StatusID] = node
	r.nodesByStatusID[node.StatusID] = node

	return nil
}

// GetThreadNodes retrieves all nodes for a thread by root status ID
func (r *ThreadRepository) GetThreadNodes(_ context.Context, rootStatusID string) ([]*models.ThreadNode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodeMap := r.nodes[rootStatusID]
	if nodeMap == nil {
		return []*models.ThreadNode{}, nil
	}

	var result []*models.ThreadNode
	for _, node := range nodeMap {
		result = append(result, node)
	}
	return result, nil
}

// GetThreadNode retrieves a single thread node by status ID
func (r *ThreadRepository) GetThreadNode(_ context.Context, rootStatusID, statusID string) (*models.ThreadNode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodeMap := r.nodes[rootStatusID]
	if nodeMap == nil {
		return nil, storage.ErrNotFound
	}

	node, exists := nodeMap[statusID]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return node, nil
}

// GetThreadNodeByStatusID retrieves a thread node by status ID using GSI
func (r *ThreadRepository) GetThreadNodeByStatusID(_ context.Context, statusID string) (*models.ThreadNode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	node, exists := r.nodesByStatusID[statusID]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return node, nil
}

// BulkSaveThreadNodes saves multiple thread nodes in a batch
func (r *ThreadRepository) BulkSaveThreadNodes(ctx context.Context, nodes []*models.ThreadNode) error {
	for _, node := range nodes {
		if err := r.SaveThreadNode(ctx, node); err != nil {
			return err
		}
	}
	return nil
}

// ===== Missing Reply Operations =====

// MarkMissingReplies marks multiple replies as missing in a thread
func (r *ThreadRepository) MarkMissingReplies(_ context.Context, rootStatusID, parentStatusID string, replyIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.missingReplies[rootStatusID] == nil {
		r.missingReplies[rootStatusID] = make(map[string]*models.MissingReply)
	}

	for _, replyID := range replyIDs {
		missing := models.NewMissingReply(rootStatusID, parentStatusID, replyID)
		r.missingReplies[rootStatusID][replyID] = missing
	}

	return nil
}

// GetMissingReplies retrieves all missing replies for a thread
func (r *ThreadRepository) GetMissingReplies(_ context.Context, rootStatusID string) ([]*models.MissingReply, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	missingMap := r.missingReplies[rootStatusID]
	if missingMap == nil {
		return []*models.MissingReply{}, nil
	}

	var result []*models.MissingReply
	for _, missing := range missingMap {
		result = append(result, missing)
	}
	return result, nil
}

// SaveMissingReply saves or updates a missing reply record
func (r *ThreadRepository) SaveMissingReply(_ context.Context, missing *models.MissingReply) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.missingReplies[missing.RootStatusID] == nil {
		r.missingReplies[missing.RootStatusID] = make(map[string]*models.MissingReply)
	}
	r.missingReplies[missing.RootStatusID][missing.ReplyID] = missing

	return nil
}

// DeleteMissingReply deletes a missing reply record (used when resolved)
func (r *ThreadRepository) DeleteMissingReply(_ context.Context, rootStatusID, replyID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.missingReplies[rootStatusID] != nil {
		delete(r.missingReplies[rootStatusID], replyID)
	}
	return nil
}

// GetPendingMissingReplies retrieves missing replies that should be retried
func (r *ThreadRepository) GetPendingMissingReplies(_ context.Context, limit int) ([]*models.MissingReply, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.MissingReply
	for _, missingMap := range r.missingReplies {
		for _, missing := range missingMap {
			if missing.Status == "pending" {
				result = append(result, missing)
				if limit > 0 && len(result) >= limit {
					return result, nil
				}
			}
		}
	}
	return result, nil
}

// ===== Thread Context Operations =====

// GetThreadContext builds a complete thread context by querying nodes
func (r *ThreadRepository) GetThreadContext(ctx context.Context, statusID string) (*interfaces.ThreadContextResult, error) {
	// First, find the thread node for this status to get the root
	node, err := r.GetThreadNodeByStatusID(ctx, statusID)
	if err != nil {
		return nil, err
	}

	// Get all nodes in the thread
	nodes, err := r.GetThreadNodes(ctx, node.RootStatusID)
	if err != nil {
		return nil, err
	}

	// Get missing replies
	missingReplies, _ := r.GetMissingReplies(ctx, node.RootStatusID)

	// Build the context result
	result := &interfaces.ThreadContextResult{
		RootStatusID:      node.RootStatusID,
		RequestedStatusID: statusID,
		Nodes:             nodes,
		MissingReplies:    missingReplies,
	}

	// Calculate statistics
	r.calculateStats(result)

	return result, nil
}

// calculateStats calculates statistics for the thread context
func (r *ThreadRepository) calculateStats(result *interfaces.ThreadContextResult) {
	participants := make(map[string]bool)
	maxDepth := 0
	totalReplies := 0

	for _, node := range result.Nodes {
		participants[node.AuthorID] = true
		if node.Depth > maxDepth {
			maxDepth = node.Depth
		}
		totalReplies += node.ReplyCount
	}

	result.ParticipantCount = len(participants)
	result.TotalReplyCount = totalReplies
	result.MaxDepth = maxDepth
	result.MissingCount = len(result.MissingReplies)
}

// Clear clears all data (test helper)
func (r *ThreadRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.syncs = make(map[string]*models.ThreadSync)
	r.nodes = make(map[string]map[string]*models.ThreadNode)
	r.nodesByStatusID = make(map[string]*models.ThreadNode)
	r.missingReplies = make(map[string]map[string]*models.MissingReply)
}

// Ensure ThreadRepository implements interfaces.ThreadRepository
var _ interfaces.ThreadRepository = (*ThreadRepository)(nil)
