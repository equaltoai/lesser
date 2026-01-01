// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// TimelineRepository is a thread-safe in-memory implementation of interfaces.TimelineRepository.
// It stores data in memory for integration-style testing without requiring DynamoDB.
type TimelineRepository struct {
	mu sync.RWMutex

	// Core timeline data - keyed by composite key (timelineType#timelineID#entryID)
	entries map[string]*timelineEntry

	// Indexes for efficient querying
	byTimeline   map[string][]string // timelineType#timelineID -> entry keys
	byPost       map[string][]string // postID -> entry keys
	byActor      map[string][]string // actorID -> entry keys
	byVisibility map[string][]string // visibility -> entry keys
	byLanguage   map[string][]string // language -> entry keys

	// Conversation data for GetConversations support
	conversations map[string]*models.Conversation // conversationID -> conversation
	userConvs     map[string][]string             // username -> conversationIDs
}

// timelineEntry stores a timeline entry with metadata
type timelineEntry struct {
	entry     *models.Timeline
	createdAt time.Time
	updatedAt time.Time
}

// NewTimelineRepository creates a new in-memory timeline repository
func NewTimelineRepository() *TimelineRepository {
	return &TimelineRepository{
		entries:       make(map[string]*timelineEntry),
		byTimeline:    make(map[string][]string),
		byPost:        make(map[string][]string),
		byActor:       make(map[string][]string),
		byVisibility:  make(map[string][]string),
		byLanguage:    make(map[string][]string),
		conversations: make(map[string]*models.Conversation),
		userConvs:     make(map[string][]string),
	}
}


// makeEntryKey creates a unique key for a timeline entry
func makeEntryKey(timelineType, timelineID, entryID string, timelineAt time.Time) string {
	return fmt.Sprintf("%s#%s#%d#%s", timelineType, timelineID, timelineAt.Unix(), entryID)
}

// makeTimelineKey creates a key for timeline index
func makeTimelineKey(timelineType, timelineID string) string {
	return fmt.Sprintf("%s#%s", timelineType, timelineID)
}

// copyTimeline creates a deep copy of a timeline entry
func copyTimeline(t *models.Timeline) *models.Timeline {
	if t == nil {
		return nil
	}
	copy := *t
	return &copy
}

// Core timeline entry operations

// CreateTimelineEntry creates a new timeline entry
func (r *TimelineRepository) CreateTimelineEntry(_ context.Context, entry *models.Timeline) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if entry == nil {
		return fmt.Errorf("timeline entry is required")
	}

	// Generate entry ID if not set
	if entry.EntryID == "" {
		entry.EntryID = fmt.Sprintf("%d_%s", entry.TimelineAt.Unix(), entry.PostID)
	}

	key := makeEntryKey(entry.TimelineType, entry.TimelineID, entry.EntryID, entry.TimelineAt)

	if _, exists := r.entries[key]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	r.entries[key] = &timelineEntry{
		entry:     copyTimeline(entry),
		createdAt: now,
		updatedAt: now,
	}

	// Update indexes
	r.addToIndexes(key, entry)

	return nil
}

// CreateTimelineEntries creates multiple timeline entries in batch
func (r *TimelineRepository) CreateTimelineEntries(ctx context.Context, entries []*models.Timeline) error {
	for _, entry := range entries {
		if err := r.CreateTimelineEntry(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

// GetTimelineEntry retrieves a specific timeline entry
func (r *TimelineRepository) GetTimelineEntry(_ context.Context, timelineType, timelineID, entryID string, timelineAt time.Time) (*models.Timeline, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := makeEntryKey(timelineType, timelineID, entryID, timelineAt)
	te, exists := r.entries[key]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return copyTimeline(te.entry), nil
}

// UpdateTimelineEntry updates an existing timeline entry
func (r *TimelineRepository) UpdateTimelineEntry(_ context.Context, entry *models.Timeline) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if entry == nil {
		return fmt.Errorf("timeline entry is required")
	}

	key := makeEntryKey(entry.TimelineType, entry.TimelineID, entry.EntryID, entry.TimelineAt)
	te, exists := r.entries[key]
	if !exists {
		return storage.ErrNotFound
	}

	te.entry = copyTimeline(entry)
	te.updatedAt = time.Now()

	return nil
}

// DeleteTimelineEntry deletes a specific timeline entry
func (r *TimelineRepository) DeleteTimelineEntry(_ context.Context, timelineType, timelineID, entryID string, timelineAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := makeEntryKey(timelineType, timelineID, entryID, timelineAt)
	te, exists := r.entries[key]
	if !exists {
		return storage.ErrNotFound
	}

	// Remove from indexes
	r.removeFromIndexes(key, te.entry)

	delete(r.entries, key)
	return nil
}


// Timeline retrieval by type

// GetHomeTimeline retrieves home timeline entries for a user
func (r *TimelineRepository) GetHomeTimeline(_ context.Context, username string, limit int, cursor string) ([]*models.Timeline, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.getTimelineEntries("HOME", username, limit, cursor)
}

// GetPublicTimeline retrieves public timeline entries
func (r *TimelineRepository) GetPublicTimeline(_ context.Context, local bool, limit int, cursor string) ([]*models.Timeline, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	timelineID := "FEDERATED"
	if local {
		timelineID = "LOCAL"
	}
	return r.getTimelineEntries("PUBLIC", timelineID, limit, cursor)
}

// GetListTimeline retrieves timeline entries for a specific list
func (r *TimelineRepository) GetListTimeline(_ context.Context, listID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.getTimelineEntries("LIST", listID, limit, cursor)
}

// GetDirectTimeline retrieves direct message timeline entries for a user
func (r *TimelineRepository) GetDirectTimeline(_ context.Context, username string, limit int, cursor string) ([]*models.Timeline, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.getTimelineEntries("DIRECT", username, limit, cursor)
}

// GetHashtagTimeline retrieves timeline entries for a specific hashtag
func (r *TimelineRepository) GetHashtagTimeline(_ context.Context, hashtag string, local bool, limit int, cursor string) ([]*models.Timeline, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	timelineID := hashtag
	if local {
		timelineID = hashtag + "#LOCAL"
	}
	return r.getTimelineEntries("HASHTAG", timelineID, limit, cursor)
}

// getTimelineEntries is a helper to retrieve timeline entries with pagination
func (r *TimelineRepository) getTimelineEntries(timelineType, timelineID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	tlKey := makeTimelineKey(timelineType, timelineID)
	keys, exists := r.byTimeline[tlKey]
	if !exists {
		return []*models.Timeline{}, "", nil
	}

	// Sort by timeline time (newest first using reverse timestamp logic)
	sortedKeys := make([]string, len(keys))
	copy(sortedKeys, keys)
	sort.Slice(sortedKeys, func(i, j int) bool {
		ei := r.entries[sortedKeys[i]]
		ej := r.entries[sortedKeys[j]]
		return ei.entry.TimelineAt.After(ej.entry.TimelineAt)
	})

	return r.paginateEntries(sortedKeys, limit, cursor)
}


// Timeline retrieval by index

// GetTimelineEntriesByPost retrieves all timeline entries for a specific post
func (r *TimelineRepository) GetTimelineEntriesByPost(_ context.Context, postID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys, exists := r.byPost[postID]
	if !exists {
		return []*models.Timeline{}, "", nil
	}

	return r.paginateEntries(keys, limit, cursor)
}

// GetTimelineEntriesByActor retrieves all timeline entries by a specific actor
func (r *TimelineRepository) GetTimelineEntriesByActor(_ context.Context, actorID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys, exists := r.byActor[actorID]
	if !exists {
		return []*models.Timeline{}, "", nil
	}

	return r.paginateEntries(keys, limit, cursor)
}

// GetTimelineEntriesByVisibility retrieves timeline entries by visibility level
func (r *TimelineRepository) GetTimelineEntriesByVisibility(_ context.Context, visibility string, limit int, cursor string) ([]*models.Timeline, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys, exists := r.byVisibility[visibility]
	if !exists {
		return []*models.Timeline{}, "", nil
	}

	return r.paginateEntries(keys, limit, cursor)
}

// GetTimelineEntriesByLanguage retrieves timeline entries by language
func (r *TimelineRepository) GetTimelineEntriesByLanguage(_ context.Context, language string, limit int, cursor string) ([]*models.Timeline, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys, exists := r.byLanguage[language]
	if !exists {
		return []*models.Timeline{}, "", nil
	}

	return r.paginateEntries(keys, limit, cursor)
}

// Advanced timeline queries

// GetTimelineEntriesInRange retrieves timeline entries within a time range
func (r *TimelineRepository) GetTimelineEntriesInRange(_ context.Context, timelineType, timelineID string, startTime, endTime time.Time, limit int) ([]*models.Timeline, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tlKey := makeTimelineKey(timelineType, timelineID)
	keys, exists := r.byTimeline[tlKey]
	if !exists {
		return []*models.Timeline{}, nil
	}

	var results []*models.Timeline
	for _, key := range keys {
		te := r.entries[key]
		if te.entry.TimelineAt.After(startTime) && te.entry.TimelineAt.Before(endTime) {
			results = append(results, copyTimeline(te.entry))
			if len(results) >= limit {
				break
			}
		}
	}

	// Sort by timeline time (newest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].TimelineAt.After(results[j].TimelineAt)
	})

	return results, nil
}

// GetTimelineEntriesWithFilters retrieves timeline entries with various filters
func (r *TimelineRepository) GetTimelineEntriesWithFilters(_ context.Context, timelineType, timelineID string, filters interfaces.TimelineFilters, limit int, cursor string) ([]*models.Timeline, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tlKey := makeTimelineKey(timelineType, timelineID)
	keys, exists := r.byTimeline[tlKey]
	if !exists {
		return []*models.Timeline{}, "", nil
	}

	// Filter entries
	var filteredKeys []string
	for _, key := range keys {
		te := r.entries[key]
		if r.matchesFilters(te.entry, filters) {
			filteredKeys = append(filteredKeys, key)
		}
	}

	// Sort by timeline time (newest first)
	sort.Slice(filteredKeys, func(i, j int) bool {
		ei := r.entries[filteredKeys[i]]
		ej := r.entries[filteredKeys[j]]
		return ei.entry.TimelineAt.After(ej.entry.TimelineAt)
	})

	return r.paginateEntries(filteredKeys, limit, cursor)
}

// CountTimelineEntries counts the number of entries in a timeline
func (r *TimelineRepository) CountTimelineEntries(_ context.Context, timelineType, timelineID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tlKey := makeTimelineKey(timelineType, timelineID)
	keys, exists := r.byTimeline[tlKey]
	if !exists {
		return 0, nil
	}

	return len(keys), nil
}


// Batch operations

// DeleteTimelineEntriesByPost deletes all timeline entries for a specific post
func (r *TimelineRepository) DeleteTimelineEntriesByPost(_ context.Context, postID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	keys, exists := r.byPost[postID]
	if !exists {
		return nil // Nothing to delete
	}

	// Make a copy of keys since we'll be modifying the slice
	keysToDelete := make([]string, len(keys))
	copy(keysToDelete, keys)

	for _, key := range keysToDelete {
		te, exists := r.entries[key]
		if !exists {
			continue
		}
		r.removeFromIndexes(key, te.entry)
		delete(r.entries, key)
	}

	return nil
}

// DeleteExpiredTimelineEntries deletes timeline entries that have expired
func (r *TimelineRepository) DeleteExpiredTimelineEntries(_ context.Context, before time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var keysToDelete []string
	for key, te := range r.entries {
		if te.entry.TTL > 0 && time.Unix(te.entry.TTL, 0).Before(before) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		te := r.entries[key]
		r.removeFromIndexes(key, te.entry)
		delete(r.entries, key)
	}

	return nil
}

// RemoveFromTimelines removes timeline entries for a specific object across all timelines
func (r *TimelineRepository) RemoveFromTimelines(ctx context.Context, objectID string) error {
	return r.DeleteTimelineEntriesByPost(ctx, objectID)
}

// Conversation support

// GetConversations retrieves conversations for a user
func (r *TimelineRepository) GetConversations(_ context.Context, username string, limit int, cursor string) ([]*models.Conversation, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	convIDs, exists := r.userConvs[username]
	if !exists {
		return []*models.Conversation{}, "", nil
	}

	// Normalize limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Sort by updated time (newest first)
	sortedIDs := make([]string, len(convIDs))
	copy(sortedIDs, convIDs)
	sort.Slice(sortedIDs, func(i, j int) bool {
		ci := r.conversations[sortedIDs[i]]
		cj := r.conversations[sortedIDs[j]]
		if ci == nil || cj == nil {
			return false
		}
		return ci.UpdatedAt.After(cj.UpdatedAt)
	})

	// Apply cursor-based pagination
	startIdx := 0
	if cursor != "" {
		for i, id := range sortedIDs {
			if id == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Get results
	var results []*models.Conversation
	for i := startIdx; i < len(sortedIDs) && len(results) < limit+1; i++ {
		conv := r.conversations[sortedIDs[i]]
		if conv != nil {
			convCopy := *conv
			results = append(results, &convCopy)
		}
	}

	// Determine next cursor
	var nextCursor string
	if len(results) > limit {
		nextCursor = results[limit-1].ID
		results = results[:limit]
	}

	return results, nextCursor, nil
}


// Helper methods

// addToIndexes adds an entry to all relevant indexes
func (r *TimelineRepository) addToIndexes(key string, entry *models.Timeline) {
	// Timeline index
	tlKey := makeTimelineKey(entry.TimelineType, entry.TimelineID)
	r.byTimeline[tlKey] = append(r.byTimeline[tlKey], key)

	// Post index
	if entry.PostID != "" {
		r.byPost[entry.PostID] = append(r.byPost[entry.PostID], key)
	}

	// Actor index
	if entry.ActorID != "" {
		r.byActor[entry.ActorID] = append(r.byActor[entry.ActorID], key)
	}

	// Visibility index
	if entry.Visibility != "" {
		r.byVisibility[entry.Visibility] = append(r.byVisibility[entry.Visibility], key)
	}

	// Language index
	if entry.Language != "" {
		r.byLanguage[entry.Language] = append(r.byLanguage[entry.Language], key)
	}
}

// removeFromIndexes removes an entry from all indexes
func (r *TimelineRepository) removeFromIndexes(key string, entry *models.Timeline) {
	// Timeline index
	tlKey := makeTimelineKey(entry.TimelineType, entry.TimelineID)
	r.byTimeline[tlKey] = removeFromSlice(r.byTimeline[tlKey], key)

	// Post index
	if entry.PostID != "" {
		r.byPost[entry.PostID] = removeFromSlice(r.byPost[entry.PostID], key)
	}

	// Actor index
	if entry.ActorID != "" {
		r.byActor[entry.ActorID] = removeFromSlice(r.byActor[entry.ActorID], key)
	}

	// Visibility index
	if entry.Visibility != "" {
		r.byVisibility[entry.Visibility] = removeFromSlice(r.byVisibility[entry.Visibility], key)
	}

	// Language index
	if entry.Language != "" {
		r.byLanguage[entry.Language] = removeFromSlice(r.byLanguage[entry.Language], key)
	}
}

// matchesFilters checks if an entry matches the given filters
func (r *TimelineRepository) matchesFilters(entry *models.Timeline, filters interfaces.TimelineFilters) bool {
	if filters.OnlyMedia && !entry.HasMedia {
		return false
	}

	if filters.ExcludeReplies && entry.IsReply {
		return false
	}

	if filters.ExcludeBoosts && entry.IsBoost {
		return false
	}

	if filters.Language != "" && !strings.EqualFold(entry.Language, filters.Language) {
		return false
	}

	return true
}

// paginateEntries paginates a list of entry keys
func (r *TimelineRepository) paginateEntries(keys []string, limit int, cursor string) ([]*models.Timeline, string, error) {
	// Normalize limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Find start index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, key := range keys {
			te := r.entries[key]
			if te != nil && te.entry.SK == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Collect results
	var results []*models.Timeline
	for i := startIdx; i < len(keys) && len(results) < limit+1; i++ {
		te := r.entries[keys[i]]
		if te != nil {
			results = append(results, copyTimeline(te.entry))
		}
	}

	// Determine next cursor
	var nextCursor string
	if len(results) > limit {
		nextCursor = results[limit-1].SK
		results = results[:limit]
	}

	return results, nextCursor, nil
}

// AddConversation adds a conversation for testing purposes
func (r *TimelineRepository) AddConversation(conv *models.Conversation, participants []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if conv == nil || conv.ID == "" {
		return
	}

	r.conversations[conv.ID] = conv
	for _, participant := range participants {
		r.userConvs[participant] = append(r.userConvs[participant], conv.ID)
	}
}

// Ensure TimelineRepository implements interfaces.TimelineRepository
var _ interfaces.TimelineRepository = (*TimelineRepository)(nil)
