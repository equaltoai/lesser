// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

const draftStatusDraft = "draft"

// DraftRepository is a thread-safe in-memory implementation of interfaces.DraftRepository.
type DraftRepository struct {
	mu sync.RWMutex

	// Drafts by composite key: authorID:draftID -> draft
	drafts map[string]*models.Draft

	// Drafts by author: authorID -> []draftKey
	draftsByAuthor map[string][]string

	// Scheduled drafts: status -> []draftKey
	draftsByStatus map[string][]string
}

// ListDraftReviewGrantsByOwner satisfies the owner review projection used by CMS GraphQL.
// This lightweight draft repository does not persist review assignments.
func (r *DraftRepository) ListDraftReviewGrantsByOwner(_ context.Context, _ string) ([]*models.DraftReviewGrant, error) {
	return []*models.DraftReviewGrant{}, nil
}

// NewDraftRepository creates a new in-memory draft repository
func NewDraftRepository() *DraftRepository {
	return &DraftRepository{
		drafts:         make(map[string]*models.Draft),
		draftsByAuthor: make(map[string][]string),
		draftsByStatus: make(map[string][]string),
	}
}

// draftKey creates a composite key for a draft
func draftKey(authorID, draftID string) string {
	return fmt.Sprintf("%s:%s", authorID, draftID)
}

// CreateDraft creates a new draft
func (r *DraftRepository) CreateDraft(_ context.Context, draft *models.Draft) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if draft == nil || draft.AuthorID == "" || draft.ID == "" {
		return storage.ErrInvalidInput
	}

	key := draftKey(draft.AuthorID, draft.ID)
	if _, exists := r.drafts[key]; exists {
		return storage.ErrAlreadyExists
	}

	// Store draft
	r.drafts[key] = draft

	// Index by author
	r.draftsByAuthor[draft.AuthorID] = append(r.draftsByAuthor[draft.AuthorID], key)

	// Index by status
	status := strings.ToLower(strings.TrimSpace(draft.Status))
	if status == "" {
		status = draftStatusDraft
	}
	r.draftsByStatus[status] = append(r.draftsByStatus[status], key)

	return nil
}

// GetDraft retrieves a draft by author ID and draft ID
func (r *DraftRepository) GetDraft(_ context.Context, authorID, draftID string) (*models.Draft, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := draftKey(authorID, draftID)
	draft, exists := r.drafts[key]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return draft, nil
}

// UpdateDraft updates an existing draft
func (r *DraftRepository) UpdateDraft(_ context.Context, authorID string, draft *models.Draft) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Content writers never write PublishAttemptedAt (the transition lane is its
	// only writer), so preserve the stored publish-attempt stamp: an author
	// editing a crash-stuck publishing draft must not re-arm the sweep.
	return r.storeDraftLocked(authorID, draft, true)
}

// TransitionDraftToPublishing applies the publish transition: the status,
// cleared schedule, transition timestamps, the publish-attempt stamp, and the
// status-index move. It models the production field-scoped lane, which is the
// only writer of PublishAttemptedAt (it stamps the incoming attempt time).
func (r *DraftRepository) TransitionDraftToPublishing(_ context.Context, authorID string, draft *models.Draft) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.storeDraftLocked(authorID, draft, false)
}

// storeDraftLocked validates and stores one draft under the held lock, moving
// the status index when the status changes and preserving the stored
// editorial-media binding (it has its own field-scoped writer).
// preserveStoredStamp selects which publish-attempt stamp to keep: content
// writers preserve the stored stamp (nil stays nil), while the transition lane
// takes the incoming stamp.
func (r *DraftRepository) storeDraftLocked(authorID string, draft *models.Draft, preserveStoredStamp bool) error {
	if draft == nil || strings.TrimSpace(authorID) == "" || draft.AuthorID == "" || draft.ID == "" {
		return storage.ErrInvalidInput
	}
	if strings.TrimSpace(draft.AuthorID) != strings.TrimSpace(authorID) {
		return storage.ErrNotFound
	}

	key := draftKey(authorID, draft.ID)
	oldDraft, exists := r.drafts[key]
	if !exists {
		return storage.ErrNotFound
	}

	// Update status index if status changed
	oldStatus := strings.ToLower(strings.TrimSpace(oldDraft.Status))
	if oldStatus == "" {
		oldStatus = draftStatusDraft
	}
	newStatus := strings.ToLower(strings.TrimSpace(draft.Status))
	if newStatus == "" {
		newStatus = draftStatusDraft
	}

	if oldStatus != newStatus {
		r.draftsByStatus[oldStatus] = draftRemoveFromSlice(r.draftsByStatus[oldStatus], key)
		r.draftsByStatus[newStatus] = append(r.draftsByStatus[newStatus], key)
	}

	stamp := draft.PublishAttemptedAt
	if preserveStoredStamp {
		stamp = oldDraft.PublishAttemptedAt
	}
	updatedDraft := *draft
	updatedDraft.EditorialMedia = append([]models.DraftMediaUsage(nil), oldDraft.EditorialMedia...)
	// The version attribute is owned by the field-scoped editorial-media writer
	// (CAS); content writers preserve the stored version and never advance it.
	updatedDraft.ModelVersion = oldDraft.ModelVersion
	if stamp != nil {
		cloned := *stamp
		updatedDraft.PublishAttemptedAt = &cloned
	} else {
		updatedDraft.PublishAttemptedAt = nil
	}
	r.drafts[key] = &updatedDraft
	return nil
}

// UpdateDraftEditorialMedia replaces only the draft's editorial-media binding
// and update timestamp, matching the production field-scoped writer, and
// enforces the same version-conditioned CAS: a stale snapshot (version read
// before a concurrent media-set landed) conflicts instead of silently losing
// the update. The winner bumps the version.
func (r *DraftRepository) UpdateDraftEditorialMedia(_ context.Context, authorID string, draft *models.Draft) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if draft == nil || strings.TrimSpace(authorID) == "" || draft.AuthorID == "" || draft.ID == "" {
		return storage.ErrInvalidInput
	}
	if strings.TrimSpace(draft.AuthorID) != strings.TrimSpace(authorID) {
		return storage.ErrNotFound
	}

	stored, exists := r.drafts[draftKey(authorID, draft.ID)]
	if !exists {
		return storage.ErrNotFound
	}
	if stored.ModelVersion != draft.ModelVersion {
		return apperrors.DynamoDBConditionalCheckFailed("draft " + draft.ID).
			WithInternalError(storage.ErrVersionConflict)
	}
	nextVersion := stored.ModelVersion + 1
	if nextVersion <= 0 {
		nextVersion = 1
	}
	stored.EditorialMedia = append([]models.DraftMediaUsage(nil), draft.EditorialMedia...)
	stored.UpdatedAt = draft.UpdatedAt
	stored.ModelVersion = nextVersion
	// Parity with the production repo: the winning caller's snapshot carries the
	// bumped version so a follow-up media-set on the same snapshot CASes cleanly.
	draft.ModelVersion = nextVersion
	return nil
}

// DeleteDraft deletes a draft
func (r *DraftRepository) DeleteDraft(_ context.Context, authorID, draftID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	authorID = strings.TrimSpace(authorID)
	draftID = strings.TrimSpace(draftID)
	if authorID == "" || draftID == "" {
		return storage.ErrInvalidInput
	}

	key := draftKey(authorID, draftID)
	draft, exists := r.drafts[key]
	if !exists {
		return storage.ErrNotFound
	}

	// Remove from author index
	r.draftsByAuthor[authorID] = draftRemoveFromSlice(r.draftsByAuthor[authorID], key)

	// Remove from status index
	status := strings.ToLower(strings.TrimSpace(draft.Status))
	if status == "" {
		status = draftStatusDraft
	}
	r.draftsByStatus[status] = draftRemoveFromSlice(r.draftsByStatus[status], key)

	delete(r.drafts, key)
	return nil
}

// ListDraftsByAuthor lists drafts for an author
func (r *DraftRepository) ListDraftsByAuthor(ctx context.Context, authorID string, limit int) ([]*models.Draft, error) {
	drafts, _, err := r.ListDraftsByAuthorPaginated(ctx, authorID, limit, "")
	return drafts, err
}

// ListDraftsByAuthorPaginated lists drafts for an author with cursor pagination
func (r *DraftRepository) ListDraftsByAuthorPaginated(_ context.Context, authorID string, limit int, cursor string) ([]*models.Draft, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return listByAuthorPaginated(authorID, limit, cursor, r.draftsByAuthor, func(key string) (*models.Draft, bool) {
		draft, exists := r.drafts[key]
		return draft, exists
	}, func(d *models.Draft) string {
		return d.SK
	})
}

// ListScheduledDraftsDuePaginated lists drafts scheduled to publish at or before the provided time
func (r *DraftRepository) ListScheduledDraftsDuePaginated(_ context.Context, dueBefore time.Time, limit int, cursor string) ([]*models.Draft, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if dueBefore.IsZero() {
		dueBefore = time.Now()
	}
	if limit <= 0 {
		limit = 25
	}

	// Get scheduled drafts
	keys := r.draftsByStatus["scheduled"]
	drafts := make([]*models.Draft, 0, len(keys))
	for _, key := range keys {
		if draft, exists := r.drafts[key]; exists {
			// Filter by scheduled time
			if draft.ScheduledAt != nil && !draft.ScheduledAt.After(dueBefore) {
				drafts = append(drafts, draft)
			}
		}
	}

	// Sort by GSI4SK ascending
	sort.Slice(drafts, func(i, j int) bool {
		return drafts[i].GSI4SK < drafts[j].GSI4SK
	})

	// Apply cursor. A cursor past the last key (no key exceeds it) must yield an
	// empty page with an empty next cursor so the caller terminates; defaulting
	// startIdx to len(drafts) does that, matching the production repository's
	// past-end behavior instead of re-emitting page one forever.
	startIdx := 0
	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		startIdx = len(drafts)
		for i, draft := range drafts {
			if draft.GSI4SK > cursor {
				startIdx = i
				break
			}
		}
	}

	// Apply limit
	endIdx := startIdx + limit
	if endIdx > len(drafts) {
		endIdx = len(drafts)
	}

	result := drafts[startIdx:endIdx]
	nextCursor := ""
	if endIdx < len(drafts) && len(result) > 0 {
		nextCursor = result[len(result)-1].GSI4SK
	}

	return result, nextCursor, nil
}

// ListDraftsByStatusPaginated lists drafts in one status, paginated by GSI4SK
// cursor values. It powers orphan reconciliation over terminally failed drafts
// whose bound media mints may have survived a best-effort rollback.
func (r *DraftRepository) ListDraftsByStatusPaginated(_ context.Context, status string, limit int, cursor string) ([]*models.Draft, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 25
	}

	keys := r.draftsByStatus[strings.ToLower(strings.TrimSpace(status))]
	drafts := make([]*models.Draft, 0, len(keys))
	for _, key := range keys {
		if draft, exists := r.drafts[key]; exists {
			drafts = append(drafts, draft)
		}
	}

	sort.Slice(drafts, func(i, j int) bool {
		return drafts[i].GSI4SK < drafts[j].GSI4SK
	})

	startIdx := 0
	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		startIdx = len(drafts)
		for i, draft := range drafts {
			if draft.GSI4SK > cursor {
				startIdx = i
				break
			}
		}
	}

	endIdx := startIdx + limit
	if endIdx > len(drafts) {
		endIdx = len(drafts)
	}

	result := drafts[startIdx:endIdx]
	nextCursor := ""
	if endIdx < len(drafts) && len(result) > 0 {
		nextCursor = result[len(result)-1].GSI4SK
	}

	return result, nextCursor, nil
}

// Clear clears all data (test helper)
func (r *DraftRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.drafts = make(map[string]*models.Draft)
	r.draftsByAuthor = make(map[string][]string)
	r.draftsByStatus = make(map[string][]string)
}

// draftRemoveFromSlice removes an element from a slice
func draftRemoveFromSlice(slice []string, element string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != element {
			result = append(result, s)
		}
	}
	return result
}

// Ensure DraftRepository implements interfaces.DraftRepository
var _ interfaces.DraftRepository = (*DraftRepository)(nil)
