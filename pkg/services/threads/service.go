// Package threads provides thread synchronization and traversal services
package threads

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// Constants for thread operations
const (
	MaxThreadDepth = 10 // Maximum depth for thread traversal
	DefaultDepth   = 3  // Default depth for sync operations

	// Sync status constants
	SyncStatusNone     = "NONE"
	SyncStatusComplete = "COMPLETE"
	SyncStatusPartial  = "PARTIAL"
	SyncStatusFailed   = "FAILED"
	SyncStatusSyncing  = "SYNCING"
)

// Service provides thread synchronization and traversal operations
type Service struct {
	threadRepo ThreadRepository
	statusRepo StatusRepository
	objectRepo ObjectRepository
	actorRepo  ActorRepository
	federation FederationClient
	publisher  Publisher
	logger     *zap.Logger
	domainName string
}

// ThreadRepository defines the interface for thread storage operations
type ThreadRepository interface {
	SaveThreadSync(ctx context.Context, sync *models.ThreadSync) error
	GetThreadSync(ctx context.Context, statusID string) (*models.ThreadSync, error)
	SaveThreadNode(ctx context.Context, node *models.ThreadNode) error
	GetThreadNodes(ctx context.Context, rootStatusID string) ([]*models.ThreadNode, error)
	GetThreadNode(ctx context.Context, rootStatusID, statusID string) (*models.ThreadNode, error)
	GetThreadNodeByStatusID(ctx context.Context, statusID string) (*models.ThreadNode, error)
	MarkMissingReplies(ctx context.Context, rootStatusID, parentStatusID string, replyIDs []string) error
	GetMissingReplies(ctx context.Context, rootStatusID string) ([]*models.MissingReply, error)
	GetThreadContext(ctx context.Context, statusID string) (*repositories.ThreadContextResult, error)
	SaveMissingReply(ctx context.Context, missing *models.MissingReply) error
	DeleteMissingReply(ctx context.Context, rootStatusID, replyID string) error
}

// StatusRepository defines minimal interface for status operations
type StatusRepository interface {
	GetReplies(ctx context.Context, parentStatusID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error)
}

// ObjectRepository defines the interface for object storage operations
type ObjectRepository interface {
	GetObject(ctx context.Context, objectID string) (any, error)
	CreateObject(ctx context.Context, object any) error
}

// ActorRepository defines the interface for actor operations
type ActorRepository interface {
	GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error)
}

// FederationClient defines the interface for federation operations
type FederationClient interface {
	FetchObject(ctx context.Context, objectURL string, signingActor *activitypub.Actor) (any, error)
}

// Publisher defines the interface for event publishing
type Publisher interface {
	PublishToStream(ctx context.Context, stream string, event *streaming.Event) error
}

// NewService creates a new threads service
func NewService(
	threadRepo ThreadRepository,
	statusRepo StatusRepository,
	objectRepo ObjectRepository,
	actorRepo ActorRepository,
	federation FederationClient,
	publisher Publisher,
	logger *zap.Logger,
	domainName string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		threadRepo: threadRepo,
		statusRepo: statusRepo,
		objectRepo: objectRepo,
		federation: federation,
		publisher:  publisher,
		logger:     logger,
		domainName: domainName,
		actorRepo:  actorRepo,
	}
}

// ThreadContextQuery represents a query for thread context
type ThreadContextQuery struct {
	NoteID      string
	ViewerID    string
	IncludeTree bool
}

// ThreadContextResult represents the complete context of a thread
type ThreadContextResult struct {
	RootNote         *activitypub.Note
	RequestedNote    *activitypub.Note
	Ancestors        []*activitypub.Note
	Descendants      []*models.ThreadNode
	ParticipantCount int
	ReplyCount       int
	MissingCount     int
	LastActivity     time.Time
	SyncStatus       string
}

// GetThreadContext retrieves the complete thread context for a note
func (s *Service) GetThreadContext(ctx context.Context, query ThreadContextQuery) (*ThreadContextResult, error) {
	if err := common.ValidateRequiredParam("noteID", query.NoteID); err != nil {
		return nil, errors.Join(ErrMissingRequiredParam, err)
	}

	s.logger.Debug("getting thread context",
		zap.String("note_id", query.NoteID),
		zap.String("viewer_id", query.ViewerID))

	// Get the requested note
	requestedNote, err := s.getNote(ctx, query.NoteID)
	if err != nil {
		s.logger.Error("failed to get requested note",
			zap.String("note_id", query.NoteID),
			zap.Error(err))
		return nil, errors.Join(ErrThreadNotFound, err)
	}

	// Find the thread root
	rootNote, ancestors, err := s.FindThreadRoot(ctx, requestedNote)
	if err != nil {
		s.logger.Error("failed to find thread root",
			zap.String("note_id", query.NoteID),
			zap.Error(err))
		return nil, errors.Join(ErrThreadRootNotFound, err)
	}

	// Get thread context from repository
	threadCtx, err := s.threadRepo.GetThreadContext(ctx, rootNote.ID)
	if err != nil {
		s.logger.Error("failed to get thread context from repository",
			zap.String("root_id", rootNote.ID),
			zap.Error(err))
		return nil, errors.Join(ErrGetThreadContext, err)
	}

	// Build result
	result := &ThreadContextResult{
		RootNote:      rootNote,
		RequestedNote: requestedNote,
		Ancestors:     ancestors,
		Descendants:   []*models.ThreadNode{},
	}

	// Set last activity from root note published time
	if rootNote.Published != nil {
		result.LastActivity = *rootNote.Published
	} else {
		result.LastActivity = time.Now()
	}

	if threadCtx != nil {
		result.ParticipantCount = threadCtx.ParticipantCount
		result.ReplyCount = threadCtx.TotalReplyCount
		result.MissingCount = threadCtx.MissingCount
		result.Descendants = threadCtx.Nodes

		// Calculate last activity
		for _, node := range threadCtx.Nodes {
			if node.LastReplyAt != nil && node.LastReplyAt.After(result.LastActivity) {
				result.LastActivity = *node.LastReplyAt
			}
		}

		// Determine sync status
		result.SyncStatus = s.calculateSyncStatus(threadCtx)
	} else {
		result.SyncStatus = SyncStatusNone
	}

	s.logger.Debug("retrieved thread context",
		zap.String("root_id", rootNote.ID),
		zap.Int("ancestor_count", len(ancestors)),
		zap.Int("participant_count", result.ParticipantCount))

	return result, nil
}

// FindThreadRoot finds the root of a thread by walking up the inReplyTo chain
func (s *Service) FindThreadRoot(ctx context.Context, note *activitypub.Note) (*activitypub.Note, []*activitypub.Note, error) {
	visited := make(map[string]bool)
	ancestors := []*activitypub.Note{}
	current := note

	// Walk up the inReplyTo chain
	for current.InReplyTo != "" && len(ancestors) < MaxThreadDepth {
		// Check for circular references
		if visited[current.ID] {
			s.logger.Error("circular reference detected in thread",
				zap.String("note_id", current.ID))
			return nil, nil, ErrCircularReference
		}
		visited[current.ID] = true

		// Try to fetch the parent
		parent, err := s.getNote(ctx, current.InReplyTo)
		if err != nil {
			s.logger.Warn("failed to fetch parent note, stopping traversal",
				zap.String("current_id", current.ID),
				zap.String("parent_id", current.InReplyTo),
				zap.Error(err))
			// If we can't fetch the parent, current is the effective root
			break
		}

		ancestors = append([]*activitypub.Note{parent}, ancestors...) // Prepend to maintain order
		current = parent
	}

	// Check if we hit max depth
	if len(ancestors) >= MaxThreadDepth {
		s.logger.Warn("max thread depth exceeded during root finding",
			zap.String("note_id", note.ID),
			zap.Int("depth", len(ancestors)))
		return nil, nil, ErrMaxDepthExceeded
	}

	s.logger.Debug("found thread root",
		zap.String("root_id", current.ID),
		zap.Int("ancestor_count", len(ancestors)))

	return current, ancestors, nil
}

// SyncRemoteThreadCommand represents a command to sync a remote thread
type SyncRemoteThreadCommand struct {
	NoteURL      string
	Depth        int
	ViewerID     string
	ForceRefresh bool
}

// SyncRemoteThreadResult represents the result of a thread sync operation
type SyncRemoteThreadResult struct {
	Success     bool
	ThreadRoot  *activitypub.Note
	SyncedPosts int
	ErrorCount  int
	Errors      []string
	SyncStatus  string
}

// SyncRemoteThread synchronizes a remote thread by fetching it and building the tree
func (s *Service) SyncRemoteThread(ctx context.Context, cmd SyncRemoteThreadCommand) (*SyncRemoteThreadResult, error) {
	if err := common.ValidateRequiredParam("noteURL", cmd.NoteURL); err != nil {
		return nil, errors.Join(ErrMissingRequiredParam, err)
	}

	s.logger.Info("syncing remote thread",
		zap.String("note_url", cmd.NoteURL),
		zap.Int("depth", cmd.Depth))

	// Validate and normalize depth
	if cmd.Depth <= 0 {
		cmd.Depth = DefaultDepth
	}
	if cmd.Depth > MaxThreadDepth {
		cmd.Depth = MaxThreadDepth
	}

	result := &SyncRemoteThreadResult{
		Success:     true,
		SyncedPosts: 0,
		ErrorCount:  0,
		Errors:      []string{},
	}

	// Get signing actor for federation
	signingActor, err := s.getSigningActor(ctx, cmd.ViewerID)
	if err != nil {
		s.logger.Error("failed to get signing actor",
			zap.String("viewer_id", cmd.ViewerID),
			zap.Error(err))
		return nil, errors.Join(ErrRemoteAuthFailed, err)
	}

	// Fetch the root note
	rootNote, err := s.fetchRemoteNote(ctx, cmd.NoteURL, signingActor)
	if err != nil {
		s.logger.Error("failed to fetch remote note",
			zap.String("note_url", cmd.NoteURL),
			zap.Error(err))
		result.Success = false
		result.ErrorCount++
		result.Errors = append(result.Errors, fmt.Sprintf("failed to fetch root: %v", err))
		result.SyncStatus = "FAILED"
		return result, errors.Join(ErrFetchRemoteNote, err)
	}

	result.ThreadRoot = rootNote

	// Check if sync is already in progress
	syncRecord, err := s.threadRepo.GetThreadSync(ctx, rootNote.ID)
	if err != nil {
		s.logger.Warn("failed to check sync status, continuing", zap.Error(err))
	} else if syncRecord != nil && !cmd.ForceRefresh {
		if syncRecord.IsRecentlyCompleted() {
			s.logger.Debug("thread was recently synced, skipping",
				zap.String("root_id", rootNote.ID))
			result.SyncStatus = SyncStatusComplete
			return result, nil
		}
		if syncRecord.SyncStatus == "syncing" {
			s.logger.Warn("sync already in progress",
				zap.String("root_id", rootNote.ID))
			result.SyncStatus = SyncStatusSyncing
			return result, ErrSyncInProgress
		}
	}

	// Create or update sync record
	if syncRecord == nil {
		syncRecord = models.NewThreadSync(rootNote.ID)
	}
	syncRecord.MarkSyncing()
	if err := s.threadRepo.SaveThreadSync(ctx, syncRecord); err != nil {
		s.logger.Warn("failed to save sync record", zap.Error(err))
	}

	// Build the thread tree
	syncedCount, syncErrors := s.buildThreadTree(ctx, rootNote, cmd.Depth, signingActor)
	result.SyncedPosts = syncedCount
	result.ErrorCount = len(syncErrors)
	result.Errors = syncErrors

	// Update sync record
	if len(syncErrors) == 0 {
		syncRecord.MarkCompleted()
		result.SyncStatus = SyncStatusComplete
	} else if syncedCount > 0 {
		syncRecord.SyncStatus = "completed" // Partial success
		result.SyncStatus = SyncStatusPartial
	} else {
		syncRecord.MarkFailed(fmt.Sprintf("failed to sync any notes: %v", syncErrors))
		result.SyncStatus = SyncStatusFailed
		result.Success = false
	}

	if err := s.threadRepo.SaveThreadSync(ctx, syncRecord); err != nil {
		s.logger.Warn("failed to update sync record", zap.Error(err))
	}

	s.logger.Info("thread sync completed",
		zap.String("root_id", rootNote.ID),
		zap.Int("synced", syncedCount),
		zap.Int("errors", len(syncErrors)),
		zap.String("status", result.SyncStatus))

	return result, nil
}

// buildThreadTree builds a complete thread tree from a root note
//
//nolint:gocognit // Complex thread building logic requires multiple checks
func (s *Service) buildThreadTree(ctx context.Context, rootNote *activitypub.Note, maxDepth int, signingActor *activitypub.Actor) (int, []string) {
	syncedCount := 0
	errors := []string{}

	// Save the root node
	rootNode := models.NewThreadNode(rootNote.ID, rootNote.ID, "", 0, rootNote.AttributedTo)
	rootNode.Content = rootNote.Content
	rootNode.Visibility = rootNote.Visibility
	rootNode.Sensitive = rootNote.Sensitive
	if rootNote.Published != nil {
		rootNode.CreatedAt = *rootNote.Published
	} else {
		rootNode.CreatedAt = time.Now()
	}
	rootNode.IsLocal = s.isLocalNote(rootNote.ID)
	if !rootNode.IsLocal {
		rootNode.RemoteURL = rootNote.ID
	}
	rootNode.UpdatePath("")

	if err := s.threadRepo.SaveThreadNode(ctx, rootNode); err != nil {
		s.logger.Error("failed to save root node", zap.Error(err))
		errors = append(errors, fmt.Sprintf("failed to save root: %v", err))
	} else {
		syncedCount++
	}

	// Recursively fetch and save replies
	s.fetchRepliesRecursive(ctx, rootNote, rootNode, 1, maxDepth, signingActor, &syncedCount, &errors)

	return syncedCount, errors
}

// fetchRepliesRecursive fetches replies recursively up to max depth
//
//nolint:gocognit // Complex recursive traversal requires multiple conditions
func (s *Service) fetchRepliesRecursive(ctx context.Context, parentNote *activitypub.Note, parentNode *models.ThreadNode, currentDepth, maxDepth int, signingActor *activitypub.Actor, syncedCount *int, errors *[]string) {
	if currentDepth > maxDepth {
		s.logger.Debug("max depth reached, stopping recursion",
			zap.Int("depth", currentDepth))
		return
	}

	// Try to get replies from local storage first (much faster than federation)
	localReplies, err := s.statusRepo.GetReplies(ctx, parentNote.ID, interfaces.PaginationOptions{Limit: 100})
	if err != nil {
		s.logger.Debug("failed to get local replies, will try federation",
			zap.String("note_id", parentNote.ID),
			zap.Error(err))
	}

	// Process local replies if found
	if localReplies != nil && len(localReplies.Items) > 0 {
		s.logger.Debug("found local replies",
			zap.String("parent_id", parentNote.ID),
			zap.Int("count", len(localReplies.Items)))

		for _, replyStatus := range localReplies.Items {
			// Convert status to note
			replyNote := s.statusToNote(replyStatus)

			// Create and save the thread node
			replyNode := models.NewThreadNode(parentNode.RootStatusID, replyNote.ID, parentNote.ID, currentDepth, replyNote.AttributedTo)
			replyNode.Content = replyNote.Content
			replyNode.Visibility = replyNote.Visibility
			replyNode.Sensitive = replyNote.Sensitive
			if replyNote.Published != nil {
				replyNode.CreatedAt = *replyNote.Published
			} else {
				replyNode.CreatedAt = time.Now()
			}
			replyNode.IsLocal = s.isLocalNote(replyNote.ID)
			if !replyNode.IsLocal {
				replyNode.RemoteURL = replyNote.ID
			}
			replyNode.UpdatePath(parentNode.Path)

			if err := s.threadRepo.SaveThreadNode(ctx, replyNode); err != nil {
				s.logger.Error("failed to save reply node",
					zap.String("reply_id", replyNote.ID),
					zap.Error(err))
				*errors = append(*errors, fmt.Sprintf("failed to save reply %s: %v", replyNote.ID, err))
				continue
			}

			*syncedCount++

			// Update parent node
			parentNode.AddChild(replyNote.ID)

			// Recursively fetch replies to this reply
			s.fetchRepliesRecursive(ctx, replyNote, replyNode, currentDepth+1, maxDepth, signingActor, syncedCount, errors)
		}

		// Save updated parent node
		if err := s.threadRepo.SaveThreadNode(ctx, parentNode); err != nil {
			s.logger.Warn("failed to update parent node", zap.Error(err))
		}

		return // Successfully processed local replies
	}

	// Fall back to federation for remote threads
	// This handles cases where we don't have local copies of the replies
	s.logger.Debug("no local replies found, trying federation",
		zap.String("note_id", parentNote.ID))

	// For remote notes, we would need to fetch the replies collection
	// This is a placeholder for future federation implementation
	replies := []string{}

	s.logger.Debug("found remote replies",
		zap.String("parent_id", parentNote.ID),
		zap.Int("count", len(replies)))

	// Fetch and save each reply from federation
	for _, replyURL := range replies {
		replyNote, err := s.fetchRemoteNote(ctx, replyURL, signingActor)
		if err != nil {
			s.logger.Warn("failed to fetch reply",
				zap.String("reply_url", replyURL),
				zap.Error(err))
			*errors = append(*errors, fmt.Sprintf("failed to fetch reply %s: %v", replyURL, err))

			// Mark as missing
			if err := s.threadRepo.MarkMissingReplies(ctx, parentNode.RootStatusID, parentNote.ID, []string{replyURL}); err != nil {
				s.logger.Warn("failed to mark missing reply", zap.Error(err))
			}
			continue
		}

		// Create and save the thread node
		replyNode := models.NewThreadNode(parentNode.RootStatusID, replyNote.ID, parentNote.ID, currentDepth, replyNote.AttributedTo)
		replyNode.Content = replyNote.Content
		replyNode.Visibility = replyNote.Visibility
		replyNode.Sensitive = replyNote.Sensitive
		if replyNote.Published != nil {
			replyNode.CreatedAt = *replyNote.Published
		} else {
			replyNode.CreatedAt = time.Now()
		}
		replyNode.IsLocal = s.isLocalNote(replyNote.ID)
		if !replyNode.IsLocal {
			replyNode.RemoteURL = replyNote.ID
		}
		replyNode.UpdatePath(parentNode.Path)

		if err := s.threadRepo.SaveThreadNode(ctx, replyNode); err != nil {
			s.logger.Error("failed to save reply node",
				zap.String("reply_id", replyNote.ID),
				zap.Error(err))
			*errors = append(*errors, fmt.Sprintf("failed to save reply %s: %v", replyNote.ID, err))
			continue
		}

		*syncedCount++

		// Update parent node
		parentNode.AddChild(replyNote.ID)

		// Recursively fetch replies to this reply
		s.fetchRepliesRecursive(ctx, replyNote, replyNode, currentDepth+1, maxDepth, signingActor, syncedCount, errors)
	}

	// Save updated parent node
	if err := s.threadRepo.SaveThreadNode(ctx, parentNode); err != nil {
		s.logger.Warn("failed to update parent node", zap.Error(err))
	}
}

// SyncMissingRepliesCommand represents a command to sync missing replies
type SyncMissingRepliesCommand struct {
	NoteID   string
	ViewerID string
}

// SyncMissingRepliesResult represents the result of syncing missing replies
type SyncMissingRepliesResult struct {
	Success       bool
	SyncedReplies int
	Errors        []string
}

// SyncMissingReplies syncs replies that were detected as missing
func (s *Service) SyncMissingReplies(ctx context.Context, cmd SyncMissingRepliesCommand) (*SyncMissingRepliesResult, error) {
	if err := common.ValidateRequiredParam("noteID", cmd.NoteID); err != nil {
		return nil, errors.Join(ErrMissingRequiredParam, err)
	}

	s.logger.Info("syncing missing replies",
		zap.String("note_id", cmd.NoteID))

	// Get the note
	note, err := s.getNote(ctx, cmd.NoteID)
	if err != nil {
		return nil, errors.Join(ErrThreadNotFound, err)
	}

	// Find the thread root
	rootNote, _, err := s.FindThreadRoot(ctx, note)
	if err != nil {
		return nil, errors.Join(ErrThreadRootNotFound, err)
	}

	// Get missing replies for this thread
	missingReplies, err := s.threadRepo.GetMissingReplies(ctx, rootNote.ID)
	if err != nil {
		return nil, errors.Join(ErrGetThreadContext, err)
	}

	if len(missingReplies) == 0 {
		s.logger.Debug("no missing replies to sync",
			zap.String("root_id", rootNote.ID))
		return &SyncMissingRepliesResult{
			Success:       true,
			SyncedReplies: 0,
			Errors:        []string{},
		}, nil
	}

	// Get signing actor
	signingActor, err := s.getSigningActor(ctx, cmd.ViewerID)
	if err != nil {
		return nil, errors.Join(ErrRemoteAuthFailed, err)
	}

	result := &SyncMissingRepliesResult{
		Success:       true,
		SyncedReplies: 0,
		Errors:        []string{},
	}

	// Attempt to fetch each missing reply
	for _, missing := range missingReplies {
		// Skip if permanent failure or recently attempted
		if missing.IsPermanentFailure() {
			continue
		}
		if !missing.ShouldRetry() {
			continue
		}

		// Mark as fetching
		missing.MarkFetching()
		if err := s.threadRepo.SaveMissingReply(ctx, missing); err != nil {
			s.logger.Warn("failed to update missing reply status", zap.Error(err))
		}

		// Attempt to fetch
		replyURL := missing.ReplyURL
		if replyURL == "" {
			replyURL = missing.ReplyID
		}

		_, err = s.fetchRemoteNote(ctx, replyURL, signingActor)
		if err != nil {
			s.logger.Warn("failed to fetch missing reply",
				zap.String("reply_url", replyURL),
				zap.Error(err))

			// Classify the error
			failureReason := classifyFetchError(err)
			missing.MarkFailed(err.Error(), failureReason)
			if err := s.threadRepo.SaveMissingReply(ctx, missing); err != nil {
				s.logger.Warn("failed to update missing reply", zap.Error(err))
			}

			result.Errors = append(result.Errors, fmt.Sprintf("failed to fetch %s: %v", replyURL, err))
			continue
		}

		// Successfully fetched - mark as resolved and delete the record
		missing.MarkResolved()
		if err := s.threadRepo.DeleteMissingReply(ctx, rootNote.ID, missing.ReplyID); err != nil {
			s.logger.Warn("failed to delete resolved missing reply", zap.Error(err))
		}

		result.SyncedReplies++
	}

	if len(result.Errors) > 0 && result.SyncedReplies == 0 {
		result.Success = false
	}

	s.logger.Info("missing replies sync completed",
		zap.String("root_id", rootNote.ID),
		zap.Int("synced", result.SyncedReplies),
		zap.Int("errors", len(result.Errors)))

	return result, nil
}

// Helper methods

func (s *Service) getNote(ctx context.Context, noteID string) (*activitypub.Note, error) {
	// Try local storage first
	obj, err := s.objectRepo.GetObject(ctx, noteID)
	if err == nil && obj != nil {
		if note, ok := obj.(*activitypub.Note); ok {
			return note, nil
		}
	}

	// If not found locally, it's an error (should be fetched first)
	return nil, ErrThreadNotFound
}

func (s *Service) fetchRemoteNote(ctx context.Context, noteURL string, signingActor *activitypub.Actor) (*activitypub.Note, error) {
	// Fetch the object
	obj, err := s.federation.FetchObject(ctx, noteURL, signingActor)
	if err != nil {
		return nil, err
	}

	// Convert to Note type
	noteMap, ok := obj.(map[string]any)
	if !ok {
		return nil, ErrNotANote
	}

	// Check type
	objType, ok := noteMap["type"].(string)
	if !ok || objType != "Note" {
		return nil, ErrNotANote
	}

	// Parse into Note struct (simplified - in production you'd use proper JSON marshaling)
	note := &activitypub.Note{}
	if id, ok := noteMap["id"].(string); ok {
		note.ID = id
	}
	if content, ok := noteMap["content"].(string); ok {
		note.Content = content
	}
	if attributedTo, ok := noteMap["attributedTo"].(string); ok {
		note.AttributedTo = attributedTo
	}
	if inReplyTo, ok := noteMap["inReplyTo"].(string); ok {
		note.InReplyTo = inReplyTo
	}
	if published, ok := noteMap["published"].(string); ok {
		if t, err := time.Parse(time.RFC3339, published); err == nil {
			note.Published = &t
		}
	}
	if visibility, ok := noteMap["visibility"].(string); ok {
		note.Visibility = visibility
	} else {
		note.Visibility = "public" // Default
	}
	if sensitive, ok := noteMap["sensitive"].(bool); ok {
		note.Sensitive = sensitive
	}

	// Store the fetched note locally
	if err := s.objectRepo.CreateObject(ctx, note); err != nil {
		s.logger.Warn("failed to store fetched note locally",
			zap.String("note_id", note.ID),
			zap.Error(err))
		// Don't fail the operation if storage fails
	}

	return note, nil
}

func (s *Service) getSigningActor(ctx context.Context, viewerID string) (*activitypub.Actor, error) {
	// If no viewer specified, use a system actor
	if viewerID == "" {
		// Get the first available local actor
		// In production, you'd have a dedicated system actor
		return s.actorRepo.GetActorByUsername(ctx, "system")
	}

	return s.actorRepo.GetActorByUsername(ctx, viewerID)
}

func (s *Service) isLocalNote(noteID string) bool {
	return strings.Contains(noteID, s.domainName)
}

func (s *Service) calculateSyncStatus(threadCtx *repositories.ThreadContextResult) string {
	if threadCtx == nil {
		return SyncStatusNone
	}

	if threadCtx.MissingCount == 0 {
		return SyncStatusComplete
	}

	if len(threadCtx.Nodes) > 0 {
		return SyncStatusPartial
	}

	return SyncStatusNone
}

// parseRepliesCollection is kept for future use when replies collections are implemented
//
//nolint:unused // Will be used when replies collection support is added
func (s *Service) parseRepliesCollection(obj any) ([]string, error) {
	repliesMap, ok := obj.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("replies is not a map")
	}

	// Check if it's a Collection or OrderedCollection
	collectionType, ok := repliesMap["type"].(string)
	if !ok {
		return nil, fmt.Errorf("replies collection missing type")
	}

	var items []any
	switch collectionType {
	case "Collection", "OrderedCollection":
		// Get items or orderedItems
		if orderedItems, ok := repliesMap["orderedItems"].([]any); ok {
			items = orderedItems
		} else if regularItems, ok := repliesMap["items"].([]any); ok {
			items = regularItems
		} else {
			return nil, fmt.Errorf("replies collection has no items")
		}
	case "CollectionPage", "OrderedCollectionPage":
		// Get items or orderedItems from page
		if orderedItems, ok := repliesMap["orderedItems"].([]any); ok {
			items = orderedItems
		} else if regularItems, ok := repliesMap["items"].([]any); ok {
			items = regularItems
		} else {
			return nil, fmt.Errorf("replies collection page has no items")
		}
	default:
		return nil, fmt.Errorf("unknown collection type: %s", collectionType)
	}

	// Extract URLs from items
	urls := make([]string, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case string:
			urls = append(urls, v)
		case map[string]any:
			if id, ok := v["id"].(string); ok {
				urls = append(urls, id)
			}
		}
	}

	return urls, nil
}

// statusToNote converts a storage Status to an ActivityPub Note
func (s *Service) statusToNote(status *models.Status) *activitypub.Note {
	var published *time.Time
	if !status.PublishedAt.IsZero() {
		published = &status.PublishedAt
	}

	return &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        status.StatusID,
			Type:      "Note",
			Published: published,
			InReplyTo: status.InReplyToID,
			To:        []string{},
			CC:        []string{},
			Sensitive: status.Sensitive,
		},
		Content:        status.Content,
		AttributedTo:   status.AuthorID,
		Attachment:     []activitypub.Attachment{},
		Tag:            []activitypub.Tag{},
		ConversationID: status.ConversationID,
		Visibility:     status.Visibility,
	}
}

func classifyFetchError(err error) string {
	errStr := err.Error()
	if strings.Contains(errStr, "404") || strings.Contains(errStr, "not found") {
		return models.FailureReasonNotFound
	}
	if strings.Contains(errStr, "410") || strings.Contains(errStr, "gone") {
		return models.FailureReasonDeleted
	}
	if strings.Contains(errStr, "403") || strings.Contains(errStr, "forbidden") {
		return models.FailureReasonForbidden
	}
	if strings.Contains(errStr, "timeout") {
		return models.FailureReasonTimeout
	}
	if strings.Contains(errStr, "unreachable") || strings.Contains(errStr, "connection") {
		return models.FailureReasonUnreachable
	}
	return models.FailureReasonInvalid
}

// ValidateNoteURL validates and normalizes a note URL
func ValidateNoteURL(noteURL string) error {
	if noteURL == "" {
		return ErrInvalidNoteURL
	}

	parsed, err := url.Parse(noteURL)
	if err != nil {
		return errors.Join(ErrInvalidNoteURL, err)
	}

	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return errors.Join(ErrInvalidNoteURL, fmt.Errorf("invalid scheme: %s", parsed.Scheme))
	}

	if parsed.Host == "" {
		return errors.Join(ErrInvalidNoteURL, fmt.Errorf("missing host"))
	}

	return nil
}
