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

// StatusRepository is a thread-safe in-memory implementation of interfaces.StatusRepository.
// It stores data in memory for integration-style testing without requiring DynamoDB.
type StatusRepository struct {
	mu sync.RWMutex

	// Core status data
	statuses    map[string]*statusEntry // keyed by statusID
	statusByURL map[string]string       // URL -> statusID mapping

	// Timeline indexes
	publicTimeline []string            // statusIDs in chronological order (newest first)
	userTimelines  map[string][]string // userID -> statusIDs
	conversations  map[string][]string // conversationID -> statusIDs
	replies        map[string][]string // parentStatusID -> reply statusIDs
	hashtags       map[string][]string // hashtag -> statusIDs

	// Engagement data
	likes     map[string]map[string]bool // statusID -> userID -> liked
	reblogs   map[string]map[string]bool // statusID -> userID -> reblogged
	bookmarks map[string]map[string]bool // statusID -> userID -> bookmarked

	// Moderation data
	flaggedStatuses map[string]bool // statusID -> flagged
}

// statusEntry stores a status with its metadata
type statusEntry struct {
	status    *models.Status
	createdAt time.Time
	updatedAt time.Time
}

// NewStatusRepository creates a new in-memory status repository
func NewStatusRepository() *StatusRepository {
	return &StatusRepository{
		statuses:        make(map[string]*statusEntry),
		statusByURL:     make(map[string]string),
		publicTimeline:  make([]string, 0),
		userTimelines:   make(map[string][]string),
		conversations:   make(map[string][]string),
		replies:         make(map[string][]string),
		hashtags:        make(map[string][]string),
		likes:           make(map[string]map[string]bool),
		reblogs:         make(map[string]map[string]bool),
		bookmarks:       make(map[string]map[string]bool),
		flaggedStatuses: make(map[string]bool),
	}
}

// Core CRUD operations

// CreateStatus creates a new status
func (r *StatusRepository) CreateStatus(_ context.Context, status *models.Status) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if status == nil {
		return fmt.Errorf("status is required")
	}

	if status.StatusID == "" {
		return fmt.Errorf("status ID is required")
	}

	if _, exists := r.statuses[status.StatusID]; exists {
		return storage.ErrAlreadyExists
	}

	// Make a copy to avoid external mutations
	statusCopy := copyStatus(status)
	now := time.Now()

	entry := &statusEntry{
		status:    statusCopy,
		createdAt: now,
		updatedAt: now,
	}

	r.statuses[status.StatusID] = entry

	// Update URL index
	if status.Note != nil && status.Note.ID != "" {
		r.statusByURL[status.Note.ID] = status.StatusID
	}

	// Update timeline indexes
	r.updateTimelineIndexes(statusCopy)

	return nil
}

// CreateBoostStatus creates a boost status
func (r *StatusRepository) CreateBoostStatus(ctx context.Context, status *models.Status) error {
	if status == nil {
		return fmt.Errorf("boost status payload is required")
	}

	if status.BoostOfStatusID == "" && status.ReblogOfID == "" {
		return fmt.Errorf("boost status missing target reference")
	}

	if status.AuthorID == "" {
		return fmt.Errorf("boost status missing author id")
	}

	return r.CreateStatus(ctx, status)
}

// GetStatus retrieves a status by ID
func (r *StatusRepository) GetStatus(_ context.Context, statusID string) (*models.Status, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.statuses[statusID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	if entry.status.Deleted {
		return nil, storage.ErrNotFound
	}

	return copyStatus(entry.status), nil
}

// GetStatusByURL retrieves a status by its URL
func (r *StatusRepository) GetStatusByURL(_ context.Context, url string) (*models.Status, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	statusID, exists := r.statusByURL[url]
	if !exists {
		return nil, storage.ErrNotFound
	}

	entry, exists := r.statuses[statusID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	if entry.status.Deleted {
		return nil, storage.ErrNotFound
	}

	return copyStatus(entry.status), nil
}

// UpdateStatus updates an existing status
func (r *StatusRepository) UpdateStatus(_ context.Context, status *models.Status) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if status == nil {
		return fmt.Errorf("status is required")
	}

	entry, exists := r.statuses[status.StatusID]
	if !exists {
		return storage.ErrNotFound
	}

	// Update the status
	entry.status = copyStatus(status)
	entry.updatedAt = time.Now()

	return nil
}

// DeleteStatus marks a status as deleted
func (r *StatusRepository) DeleteStatus(_ context.Context, statusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.statuses[statusID]
	if !exists {
		return storage.ErrNotFound
	}

	now := time.Now()
	entry.status.Deleted = true
	entry.status.DeletedAt = &now
	entry.updatedAt = now

	// Remove from timeline indexes
	r.removeFromTimelineIndexes(statusID)

	return nil
}

// DeleteBoostStatus removes a boost status
func (r *StatusRepository) DeleteBoostStatus(_ context.Context, boosterID, targetStatusID string) (*models.Status, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Find the boost status
	var boostStatus *models.Status
	for _, entry := range r.statuses {
		if entry.status.AuthorID == boosterID &&
			(entry.status.BoostOfStatusID == targetStatusID || entry.status.ReblogOfID == targetStatusID) &&
			!entry.status.Deleted {
			boostStatus = copyStatus(entry.status)
			break
		}
	}

	if boostStatus == nil {
		return nil, nil // No boost found, not an error
	}

	// Mark as deleted
	entry := r.statuses[boostStatus.StatusID]
	now := time.Now()
	entry.status.Deleted = true
	entry.status.DeletedAt = &now
	entry.updatedAt = now

	r.removeFromTimelineIndexes(boostStatus.StatusID)

	return boostStatus, nil
}

// Timeline operations

// GetPublicTimeline retrieves the public timeline
func (r *StatusRepository) GetPublicTimeline(_ context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.paginateStatuses(r.publicTimeline, opts)
}

// GetHomeTimeline retrieves the home timeline for a user
func (r *StatusRepository) GetHomeTimeline(_ context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// For simplicity, return the user's own timeline
	// In a real implementation, this would include followed users' statuses
	timeline, exists := r.userTimelines[userID]
	if !exists {
		return &interfaces.PaginatedResult[*models.Status]{
			Items:      []*models.Status{},
			NextCursor: "",
			HasMore:    false,
			Total:      0,
		}, nil
	}

	return r.paginateStatuses(timeline, opts)
}

// GetUserTimeline retrieves a user's timeline
func (r *StatusRepository) GetUserTimeline(_ context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	timeline, exists := r.userTimelines[userID]
	if !exists {
		return &interfaces.PaginatedResult[*models.Status]{
			Items:      []*models.Status{},
			NextCursor: "",
			HasMore:    false,
			Total:      0,
		}, nil
	}

	return r.paginateStatuses(timeline, opts)
}

// GetConversationThread retrieves a conversation thread
func (r *StatusRepository) GetConversationThread(_ context.Context, conversationID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	thread, exists := r.conversations[conversationID]
	if !exists {
		return &interfaces.PaginatedResult[*models.Status]{
			Items:      []*models.Status{},
			NextCursor: "",
			HasMore:    false,
			Total:      0,
		}, nil
	}

	return r.paginateStatuses(thread, opts)
}

// GetReplies retrieves replies to a status
func (r *StatusRepository) GetReplies(_ context.Context, parentStatusID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	replyIDs, exists := r.replies[parentStatusID]
	if !exists {
		return &interfaces.PaginatedResult[*models.Status]{
			Items:      []*models.Status{},
			NextCursor: "",
			HasMore:    false,
			Total:      0,
		}, nil
	}

	return r.paginateStatuses(replyIDs, opts)
}

// Search and discovery

// SearchStatuses searches statuses by content
func (r *StatusRepository) SearchStatuses(_ context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	if normalizedQuery == "" {
		return &interfaces.PaginatedResult[*models.Status]{
			Items:      []*models.Status{},
			NextCursor: "",
			HasMore:    false,
			Total:      0,
		}, nil
	}

	var matchingIDs []string
	for statusID, entry := range r.statuses {
		if entry.status.Deleted {
			continue
		}
		if strings.Contains(strings.ToLower(entry.status.Content), normalizedQuery) {
			matchingIDs = append(matchingIDs, statusID)
		}
	}

	// Sort by published date (newest first)
	sort.Slice(matchingIDs, func(i, j int) bool {
		return r.statuses[matchingIDs[i]].status.PublishedAt.After(r.statuses[matchingIDs[j]].status.PublishedAt)
	})

	return r.paginateStatuses(matchingIDs, opts)
}

// GetStatusesByHashtag retrieves statuses by hashtag
func (r *StatusRepository) GetStatusesByHashtag(_ context.Context, hashtag string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if hashtag == "" {
		return nil, fmt.Errorf("hashtag cannot be empty")
	}
	if strings.HasPrefix(hashtag, "#") {
		return nil, fmt.Errorf("hashtag must not include # prefix (got: %q)", hashtag)
	}
	if hashtag != strings.ToLower(hashtag) {
		return nil, fmt.Errorf("hashtag must be lowercase (got: %q)", hashtag)
	}

	statusIDs, exists := r.hashtags[hashtag]
	if !exists {
		return &interfaces.PaginatedResult[*models.Status]{
			Items:      []*models.Status{},
			NextCursor: "",
			HasMore:    false,
			Total:      0,
		}, nil
	}

	return r.paginateStatuses(statusIDs, opts)
}

// GetTrendingStatuses retrieves trending statuses
func (r *StatusRepository) GetTrendingStatuses(_ context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get statuses with engagement, sorted by engagement score
	var statusIDs []string
	for statusID, entry := range r.statuses {
		if entry.status.Deleted {
			continue
		}
		if entry.status.LikeCount > 0 || entry.status.ReblogCount > 0 {
			statusIDs = append(statusIDs, statusID)
		}
	}

	// Sort by engagement (likes + reblogs)
	sort.Slice(statusIDs, func(i, j int) bool {
		si := r.statuses[statusIDs[i]].status
		sj := r.statuses[statusIDs[j]].status
		scoreI := si.LikeCount + si.ReblogCount
		scoreJ := sj.LikeCount + sj.ReblogCount
		return scoreI > scoreJ
	})

	return r.paginateStatuses(statusIDs, opts)
}

// Engagement operations

// LikeStatus likes a status
func (r *StatusRepository) LikeStatus(_ context.Context, userID, statusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.statuses[statusID]
	if !exists {
		return storage.ErrNotFound
	}

	if r.likes[statusID] == nil {
		r.likes[statusID] = make(map[string]bool)
	}

	if !r.likes[statusID][userID] {
		r.likes[statusID][userID] = true
		entry.status.LikeCount++
	}

	return nil
}

// UnlikeStatus unlikes a status
func (r *StatusRepository) UnlikeStatus(_ context.Context, userID, statusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.statuses[statusID]
	if !exists {
		return storage.ErrNotFound
	}

	if r.likes[statusID] != nil && r.likes[statusID][userID] {
		delete(r.likes[statusID], userID)
		if entry.status.LikeCount > 0 {
			entry.status.LikeCount--
		}
	}

	return nil
}

// ReblogStatus reblogs a status
func (r *StatusRepository) ReblogStatus(_ context.Context, userID, statusID string, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.statuses[statusID]
	if !exists {
		return storage.ErrNotFound
	}

	if r.reblogs[statusID] == nil {
		r.reblogs[statusID] = make(map[string]bool)
	}

	if !r.reblogs[statusID][userID] {
		r.reblogs[statusID][userID] = true
		entry.status.ReblogCount++
	}

	return nil
}

// UnreblogStatus unreblogs a status
func (r *StatusRepository) UnreblogStatus(_ context.Context, userID, statusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.statuses[statusID]
	if !exists {
		return storage.ErrNotFound
	}

	if r.reblogs[statusID] != nil && r.reblogs[statusID][userID] {
		delete(r.reblogs[statusID], userID)
		if entry.status.ReblogCount > 0 {
			entry.status.ReblogCount--
		}
	}

	return nil
}

// BookmarkStatus bookmarks a status
func (r *StatusRepository) BookmarkStatus(_ context.Context, userID, statusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.statuses[statusID]; !exists {
		return storage.ErrNotFound
	}

	if r.bookmarks[statusID] == nil {
		r.bookmarks[statusID] = make(map[string]bool)
	}

	r.bookmarks[statusID][userID] = true
	return nil
}

// UnbookmarkStatus unbookmarks a status
func (r *StatusRepository) UnbookmarkStatus(_ context.Context, userID, statusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.statuses[statusID]; !exists {
		return storage.ErrNotFound
	}

	if r.bookmarks[statusID] != nil {
		delete(r.bookmarks[statusID], userID)
	}

	return nil
}

// Moderation operations

// FlagStatus flags a status for moderation
func (r *StatusRepository) FlagStatus(_ context.Context, statusID, _ string, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.statuses[statusID]
	if !exists {
		return storage.ErrNotFound
	}

	entry.status.Flagged = true
	r.flaggedStatuses[statusID] = true

	return nil
}

// UnflagStatus unflags a status
func (r *StatusRepository) UnflagStatus(_ context.Context, statusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.statuses[statusID]
	if !exists {
		return storage.ErrNotFound
	}

	entry.status.Flagged = false
	delete(r.flaggedStatuses, statusID)

	return nil
}

// GetFlaggedStatuses retrieves flagged statuses
func (r *StatusRepository) GetFlaggedStatuses(_ context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var flaggedIDs []string
	for statusID := range r.flaggedStatuses {
		if entry, exists := r.statuses[statusID]; exists && !entry.status.Deleted {
			flaggedIDs = append(flaggedIDs, statusID)
		}
	}

	// Sort by flagged time (newest first)
	sort.Slice(flaggedIDs, func(i, j int) bool {
		return r.statuses[flaggedIDs[i]].updatedAt.After(r.statuses[flaggedIDs[j]].updatedAt)
	})

	return r.paginateStatuses(flaggedIDs, opts)
}

// Batch operations

// GetStatusesByIDs retrieves multiple statuses by their IDs
func (r *StatusRepository) GetStatusesByIDs(_ context.Context, statusIDs []string) ([]*models.Status, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.Status
	for _, statusID := range statusIDs {
		if entry, exists := r.statuses[statusID]; exists && !entry.status.Deleted {
			results = append(results, copyStatus(entry.status))
		}
	}

	return results, nil
}

// GetStatusCounts retrieves engagement counts for a status
func (r *StatusRepository) GetStatusCounts(_ context.Context, statusID string) (likes, reblogs, replies int, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.statuses[statusID]
	if !exists {
		return 0, 0, 0, storage.ErrNotFound
	}

	return entry.status.LikeCount, entry.status.ReblogCount, entry.status.ReplyCount, nil
}

// Context and metadata

// GetStatusContext retrieves ancestors and descendants of a status
func (r *StatusRepository) GetStatusContext(_ context.Context, statusID string) (ancestors, descendants []*models.Status, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.statuses[statusID]
	if !exists {
		return nil, nil, storage.ErrNotFound
	}

	// Get ancestors by following the reply chain
	ancestors = []*models.Status{}
	currentStatus := entry.status
	for currentStatus.InReplyToID != "" {
		parentEntry, exists := r.statuses[currentStatus.InReplyToID]
		if !exists || parentEntry.status.Deleted {
			break
		}
		ancestors = append([]*models.Status{copyStatus(parentEntry.status)}, ancestors...)
		currentStatus = parentEntry.status
	}

	// Get descendants (replies)
	descendants = []*models.Status{}
	if replyIDs, exists := r.replies[statusID]; exists {
		for _, replyID := range replyIDs {
			if replyEntry, exists := r.statuses[replyID]; exists && !replyEntry.status.Deleted {
				descendants = append(descendants, copyStatus(replyEntry.status))
			}
		}
	}

	return ancestors, descendants, nil
}

// GetStatusEngagement retrieves user's engagement state with a status
func (r *StatusRepository) GetStatusEngagement(_ context.Context, statusID, userID string) (liked, reblogged, bookmarked bool, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, exists := r.statuses[statusID]; !exists {
		return false, false, false, storage.ErrNotFound
	}

	liked = r.likes[statusID] != nil && r.likes[statusID][userID]
	reblogged = r.reblogs[statusID] != nil && r.reblogs[statusID][userID]
	bookmarked = r.bookmarks[statusID] != nil && r.bookmarks[statusID][userID]

	return liked, reblogged, bookmarked, nil
}

// Helper functions

// updateTimelineIndexes updates all timeline indexes for a status
func (r *StatusRepository) updateTimelineIndexes(status *models.Status) {
	// Add to public timeline if public
	if status.Visibility == models.VisibilityPublic {
		r.publicTimeline = append([]string{status.StatusID}, r.publicTimeline...)
	}

	// Add to user timeline
	if status.AuthorID != "" {
		r.userTimelines[status.AuthorID] = append([]string{status.StatusID}, r.userTimelines[status.AuthorID]...)
	}

	// Add to conversation
	if status.ConversationID != "" {
		r.conversations[status.ConversationID] = append(r.conversations[status.ConversationID], status.StatusID)
	}

	// Add to replies
	if status.InReplyToID != "" {
		r.replies[status.InReplyToID] = append(r.replies[status.InReplyToID], status.StatusID)
	}

	// Add to hashtag indexes
	for _, tag := range status.Hashtags {
		normalized := strings.ToLower(strings.TrimPrefix(tag, "#"))
		if normalized != "" {
			r.hashtags[normalized] = append([]string{status.StatusID}, r.hashtags[normalized]...)
		}
	}
}

// removeFromTimelineIndexes removes a status from all timeline indexes
func (r *StatusRepository) removeFromTimelineIndexes(statusID string) {
	// Remove from public timeline
	r.publicTimeline = removeFromSlice(r.publicTimeline, statusID)

	// Remove from user timelines
	for userID, timeline := range r.userTimelines {
		r.userTimelines[userID] = removeFromSlice(timeline, statusID)
	}

	// Remove from conversations
	for convID, thread := range r.conversations {
		r.conversations[convID] = removeFromSlice(thread, statusID)
	}

	// Remove from replies
	for parentID, replyList := range r.replies {
		r.replies[parentID] = removeFromSlice(replyList, statusID)
	}

	// Remove from hashtags
	for tag, statusList := range r.hashtags {
		r.hashtags[tag] = removeFromSlice(statusList, statusID)
	}
}

// paginateStatuses paginates a list of status IDs
func (r *StatusRepository) paginateStatuses(statusIDs []string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Filter out deleted statuses and collect results
	var results []*models.Status
	for _, statusID := range statusIDs {
		if entry, exists := r.statuses[statusID]; exists && !entry.status.Deleted {
			results = append(results, copyStatus(entry.status))
		}
		if len(results) >= limit+1 {
			break
		}
	}

	hasMore := len(results) > limit
	if hasMore {
		results = results[:limit]
	}

	var nextCursor string
	if hasMore && len(results) > 0 {
		nextCursor = results[len(results)-1].StatusID
	}

	return &interfaces.PaginatedResult[*models.Status]{
		Items:      results,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      int64(len(statusIDs)),
	}, nil
}

// copyStatus creates a deep copy of a status
func copyStatus(status *models.Status) *models.Status {
	if status == nil {
		return nil
	}

	statusCopy := *status

	// Copy slices
	if status.Hashtags != nil {
		statusCopy.Hashtags = make([]string, len(status.Hashtags))
		copy(statusCopy.Hashtags, status.Hashtags)
	}

	if status.URLs != nil {
		statusCopy.URLs = make([]string, len(status.URLs))
		copy(statusCopy.URLs, status.URLs)
	}

	if status.ToRecipients != nil {
		statusCopy.ToRecipients = make([]string, len(status.ToRecipients))
		copy(statusCopy.ToRecipients, status.ToRecipients)
	}

	if status.CcRecipients != nil {
		statusCopy.CcRecipients = make([]string, len(status.CcRecipients))
		copy(statusCopy.CcRecipients, status.CcRecipients)
	}

	return &statusCopy
}

// Count operations

// CountStatusesByAuthor counts the total number of statuses by an author
func (r *StatusRepository) CountStatusesByAuthor(_ context.Context, authorID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, entry := range r.statuses {
		if entry.status.AuthorID == authorID && !entry.status.Deleted {
			count++
		}
	}

	return count, nil
}

// CountReplies counts the number of replies to a status
func (r *StatusRepository) CountReplies(_ context.Context, statusID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	replyIDs, exists := r.replies[statusID]
	if !exists {
		return 0, nil
	}

	// Count only non-deleted replies
	count := 0
	for _, replyID := range replyIDs {
		if entry, exists := r.statuses[replyID]; exists && !entry.status.Deleted {
			count++
		}
	}

	return count, nil
}

// statusMatchesAdminFilter checks if a status matches the given admin filter criteria
func statusMatchesAdminFilter(status *models.Status, filter *interfaces.StatusFilter) bool {
	if filter == nil {
		return true
	}

	if filter.Visibility != "" && status.Visibility != filter.Visibility {
		return false
	}

	if filter.Flagged != nil && status.Flagged != *filter.Flagged {
		return false
	}

	if filter.Sensitive != nil && status.Sensitive != *filter.Sensitive {
		return false
	}

	if filter.WithMedia != nil {
		hasMedia := status.MediaCount > 0
		if *filter.WithMedia != hasMedia {
			return false
		}
	}

	if filter.MinDate != nil && status.PublishedAt.Before(*filter.MinDate) {
		return false
	}

	if filter.MaxDate != nil && status.PublishedAt.After(*filter.MaxDate) {
		return false
	}

	if filter.ByDomain != "" && !strings.Contains(status.AuthorID, filter.ByDomain) {
		return false
	}

	return true
}

// ListStatusesForAdmin retrieves statuses with comprehensive admin filtering
func (r *StatusRepository) ListStatusesForAdmin(_ context.Context, filter *interfaces.StatusFilter, limit int, cursor string) ([]*models.Status, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit = normalizeLimit(limit, 20, 100)

	// Collect all non-deleted statuses that match the filter
	var allStatuses []*models.Status
	for _, entry := range r.statuses {
		if entry.status.Deleted {
			continue
		}
		if !statusMatchesAdminFilter(entry.status, filter) {
			continue
		}
		allStatuses = append(allStatuses, copyStatus(entry.status))
	}

	// Sort by published date (newest first)
	sort.Slice(allStatuses, func(i, j int) bool {
		return allStatuses[i].PublishedAt.After(allStatuses[j].PublishedAt)
	})

	// Apply cursor-based pagination
	startIdx := findCursorIndex(allStatuses, cursor)

	// Get the page
	endIdx := startIdx + limit
	if endIdx > len(allStatuses) {
		endIdx = len(allStatuses)
	}

	result := allStatuses[startIdx:endIdx]

	// Generate next cursor
	var nextCursor string
	if endIdx < len(allStatuses) && len(result) > 0 {
		nextCursor = result[len(result)-1].StatusID
	}

	return result, nextCursor, nil
}

// normalizeLimit ensures limit is within bounds
func normalizeLimit(limit, defaultLimit, maxLimit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// findCursorIndex finds the starting index after the cursor
func findCursorIndex(statuses []*models.Status, cursor string) int {
	if cursor == "" {
		return 0
	}
	for i, status := range statuses {
		if status.StatusID == cursor {
			return i + 1
		}
	}
	return 0
}
