// Package sync provides thread synchronization utilities for ActivityPub conversation management.
package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

// ThreadSyncer handles synchronization of conversation threads across instances
type ThreadSyncer struct {
	storage    core.RepositoryStorage
	federation FederationClient
	cache      ThreadCache
	logger     *zap.Logger
}

// FederationClient interface for making federation requests
type FederationClient interface {
	FetchObject(ctx context.Context, url string) (any, error)
	FetchReplies(ctx context.Context, noteURL string) ([]*activitypub.Note, error)
	FetchContext(ctx context.Context, noteURL string) (*ThreadContext, error)
}

// ThreadCache interface for caching thread data
type ThreadCache interface {
	GetThread(ctx context.Context, conversationID string) (*Thread, error)
	SetThread(ctx context.Context, conversationID string, thread *Thread, ttl time.Duration) error
}

// ThreadSyncRequest represents a request to sync a thread
type ThreadSyncRequest struct {
	ConversationID string
	OriginServer   string
	Depth          int
	IncludeBoosts  bool
	IncludeReplies bool
	ForceRefresh   bool
}

// Thread represents a complete conversation thread
type Thread struct {
	ConversationID string
	RootNote       *activitypub.Note
	Replies        []*activitypub.Note
	Participants   []string
	LastUpdated    time.Time
	TotalPosts     int
	MissingPosts   []string // URLs of posts we know exist but couldn't fetch
}

// ThreadContext provides metadata about a thread from the origin server
type ThreadContext struct {
	ConversationID string    `json:"conversationId"`
	RootURL        string    `json:"rootUrl"`
	ReplyCount     int       `json:"replyCount"`
	Participants   []string  `json:"participants"`
	LastActivity   time.Time `json:"lastActivity"`
}

// NewThreadSyncer creates a new thread synchronization service
func NewThreadSyncer(storage core.RepositoryStorage, federation FederationClient, cache ThreadCache) *ThreadSyncer {
	return &ThreadSyncer{
		storage:    storage,
		federation: federation,
		cache:      cache,
		logger:     common.Logger(),
	}
}

// SyncThread synchronizes a complete thread from its origin server
func (t *ThreadSyncer) SyncThread(ctx context.Context, req ThreadSyncRequest) error {
	t.logger.Info("starting thread sync",
		zap.String("conversationID", req.ConversationID),
		zap.String("originServer", req.OriginServer),
		zap.Int("depth", req.Depth))

	// Check cache first unless force refresh
	if !req.ForceRefresh {
		cached, err := t.cache.GetThread(ctx, req.ConversationID)
		if err == nil && cached != nil {
			// Check if cache is fresh (less than 5 minutes old)
			if time.Since(cached.LastUpdated) < 5*time.Minute {
				t.logger.Debug("using cached thread data",
					zap.String("conversationID", req.ConversationID))
				return nil
			}
		}
	}

	// Fetch thread context from origin
	threadCtx, err := t.federation.FetchContext(ctx, fmt.Sprintf("https://%s/conversations/%s", req.OriginServer, req.ConversationID))
	if err != nil {
		t.logger.Error("failed to fetch thread context",
			zap.String("conversation_id", req.ConversationID),
			zap.String("origin_server", req.OriginServer),
			zap.String("operation", "fetch_thread_context"),
			zap.Int("thread_depth", req.Depth),
			zap.Error(err))
		return errors.Join(ErrFetchThreadContext, err)
	}

	// Initialize thread structure
	thread := &Thread{
		ConversationID: req.ConversationID,
		Participants:   threadCtx.Participants,
		LastUpdated:    time.Now(),
		MissingPosts:   make([]string, 0),
	}

	// Fetch root note
	rootObj, err := t.federation.FetchObject(ctx, threadCtx.RootURL)
	if err != nil {
		t.logger.Error("failed to fetch root note",
			zap.String("conversation_id", req.ConversationID),
			zap.String("root_url", threadCtx.RootURL),
			zap.String("operation", "fetch_root_note"),
			zap.Int("participant_count", len(threadCtx.Participants)),
			zap.Error(err))
		return errors.Join(ErrFetchRootNote, err)
	}

	rootNote, ok := rootObj.(*activitypub.Note)
	if !ok {
		return ErrInvalidRootObject
	}
	thread.RootNote = rootNote

	// Store root note
	if err := t.storeNote(ctx, rootNote); err != nil {
		t.logger.Warn("failed to store root note",
			zap.Error(err),
			zap.String("noteID", rootNote.ID))
	}

	// Fetch replies recursively
	if req.IncludeReplies {
		replies, missing := t.fetchRepliesRecursive(ctx, rootNote.ID, req.Depth, make(map[string]bool))
		thread.Replies = replies
		thread.MissingPosts = missing
		thread.TotalPosts = len(replies) + 1 // +1 for root
	}

	// Cache the complete thread
	if err := t.cache.SetThread(ctx, req.ConversationID, thread, 30*time.Minute); err != nil {
		t.logger.Warn("failed to cache thread",
			zap.Error(err),
			zap.String("conversationID", req.ConversationID))
	}

	// Update conversation metadata in storage
	if err := t.updateConversationMetadata(ctx, thread); err != nil {
		t.logger.Warn("failed to update conversation metadata",
			zap.Error(err),
			zap.String("conversationID", req.ConversationID))
	}

	t.logger.Info("thread sync completed",
		zap.String("conversationID", req.ConversationID),
		zap.Int("totalPosts", thread.TotalPosts),
		zap.Int("missingPosts", len(thread.MissingPosts)))

	return nil
}

// fetchRepliesRecursive fetches replies to a note recursively up to the specified depth
func (t *ThreadSyncer) fetchRepliesRecursive(ctx context.Context, noteURL string, depth int, visited map[string]bool) ([]*activitypub.Note, []string) {
	if depth <= 0 || visited[noteURL] {
		return nil, nil
	}

	visited[noteURL] = true
	allReplies := make([]*activitypub.Note, 0)
	missingURLs := make([]string, 0)

	// Fetch direct replies
	replies, err := t.federation.FetchReplies(ctx, noteURL)
	if err != nil {
		t.logger.Warn("failed to fetch replies",
			zap.Error(err),
			zap.String("noteURL", noteURL))
		return nil, nil
	}

	for _, reply := range replies {
		// Store the reply
		if err := t.storeNote(ctx, reply); err != nil {
			t.logger.Warn("failed to store reply",
				zap.Error(err),
				zap.String("replyID", reply.ID))
			missingURLs = append(missingURLs, reply.ID)
			continue
		}

		allReplies = append(allReplies, reply)

		// Recursively fetch replies to this reply
		subReplies, subMissing := t.fetchRepliesRecursive(ctx, reply.ID, depth-1, visited)
		allReplies = append(allReplies, subReplies...)
		missingURLs = append(missingURLs, subMissing...)
	}

	return allReplies, missingURLs
}

// storeNote stores a note in the local storage
func (t *ThreadSyncer) storeNote(ctx context.Context, note *activitypub.Note) error {
	// The storage layer accepts any, so we can pass the note directly
	return t.storage.Object().CreateObject(ctx, note)
}

// updateConversationMetadata updates the conversation metadata in storage
func (t *ThreadSyncer) updateConversationMetadata(_ context.Context, _ *Thread) error {
	// This would update conversation stats, participant list, etc.
	// Implementation depends on your storage schema
	return nil
}

// SyncMissingContext fetches missing context for existing notes
func (t *ThreadSyncer) SyncMissingContext(ctx context.Context, noteID string) error {
	// Fetch the note from storage
	obj, err := t.storage.Object().GetObject(ctx, noteID)
	if err != nil {
		t.logger.Error("failed to get note from storage",
			zap.String("note_id", noteID),
			zap.String("operation", "get_note"),
			zap.Error(err))
		return errors.Join(ErrGetNote, err)
	}

	// Type assert to Note
	note, ok := obj.(*activitypub.Note)
	if !ok {
		t.logger.Error("invalid note type",
			zap.String("note_id", noteID),
			zap.String("operation", "type_assert_note"),
			zap.String("actual_type", fmt.Sprintf("%T", obj)))
		return ErrInvalidNoteType
	}

	// If it has a conversation ID, sync the whole thread
	if note.ConversationID != "" {
		// Extract origin server from note ID
		originServer := extractDomain(noteID)

		req := ThreadSyncRequest{
			ConversationID: note.ConversationID,
			OriginServer:   originServer,
			Depth:          3, // Default depth
			IncludeReplies: true,
			ForceRefresh:   false,
		}

		return t.SyncThread(ctx, req)
	}

	// If it's a reply, fetch its parent
	if note.InReplyTo != "" {
		parentObj, err := t.federation.FetchObject(ctx, note.InReplyTo)
		if err != nil {
			t.logger.Error("failed to fetch parent note",
				zap.String("note_id", noteID),
				zap.String("parent_id", note.InReplyTo),
				zap.String("operation", "fetch_parent"),
				zap.String("conversation_id", note.ConversationID),
				zap.Error(err))
			return errors.Join(ErrFetchParent, err)
		}

		if parentNote, ok := parentObj.(*activitypub.Note); ok {
			if err := t.storeNote(ctx, parentNote); err != nil {
				t.logger.Error("failed to store parent note",
					zap.String("note_id", noteID),
					zap.String("parent_id", note.InReplyTo),
					zap.String("parent_note_id", parentNote.ID),
					zap.String("operation", "store_parent_note"),
					zap.String("conversation_id", note.ConversationID),
					zap.Error(err))
				return errors.Join(ErrStoreParentNote, err)
			}
		}
	}

	return nil
}

// extractDomain extracts the domain from an ActivityPub ID
func extractDomain(id string) string {
	// Simple implementation - would need proper URL parsing
	// Example: https://example.com/users/alice/statuses/123 -> example.com
	if len(id) > 8 && id[:8] == "https://" {
		end := 8
		for end < len(id) && id[end] != '/' {
			end++
		}
		return id[8:end]
	}
	return ""
}
