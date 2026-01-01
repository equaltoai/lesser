// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// RevisionRepository is a thread-safe in-memory implementation of interfaces.RevisionRepository.
type RevisionRepository struct {
	mu sync.RWMutex

	// Revisions by composite key: objectID:version -> revision
	revisions map[string]*models.Revision

	// Revisions by object: objectID -> []revisionKey
	revisionsByObject map[string][]string
}

// NewRevisionRepository creates a new in-memory revision repository
func NewRevisionRepository() *RevisionRepository {
	return &RevisionRepository{
		revisions:         make(map[string]*models.Revision),
		revisionsByObject: make(map[string][]string),
	}
}

// revisionKey creates a composite key for a revision
func revisionKey(objectID string, version int) string {
	return fmt.Sprintf("%s:%08d", objectID, version)
}

// CreateRevision creates a new revision
func (r *RevisionRepository) CreateRevision(_ context.Context, revision *models.Revision) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if revision == nil || revision.ObjectID == "" {
		return storage.ErrInvalidInput
	}

	key := revisionKey(revision.ObjectID, revision.Version)
	if _, exists := r.revisions[key]; exists {
		return storage.ErrAlreadyExists
	}

	// Store revision
	r.revisions[key] = revision

	// Index by object
	r.revisionsByObject[revision.ObjectID] = append(r.revisionsByObject[revision.ObjectID], key)

	return nil
}

// GetRevision retrieves a revision by object ID and version
func (r *RevisionRepository) GetRevision(_ context.Context, objectID string, version int) (*models.Revision, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := revisionKey(objectID, version)
	revision, exists := r.revisions[key]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return revision, nil
}

// ListRevisions lists revisions for an object
func (r *RevisionRepository) ListRevisions(ctx context.Context, objectID string, limit int) ([]*models.Revision, error) {
	revisions, _, err := r.ListRevisionsPaginated(ctx, objectID, limit, "")
	return revisions, err
}

// ListRevisionsPaginated lists revisions for an object with cursor pagination
func (r *RevisionRepository) ListRevisionsPaginated(_ context.Context, objectID string, limit int, cursor string) ([]*models.Revision, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	objectID = strings.TrimSpace(objectID)
	if objectID == "" {
		return nil, "", storage.ErrInvalidInput
	}

	if limit <= 0 {
		limit = 25
	}

	// Get revisions for object
	keys := r.revisionsByObject[objectID]
	revisions := make([]*models.Revision, 0, len(keys))
	for _, key := range keys {
		if revision, exists := r.revisions[key]; exists {
			revisions = append(revisions, revision)
		}
	}

	// Sort by SK (VERSION#...) descending (newest first)
	sort.Slice(revisions, func(i, j int) bool {
		return revisions[i].SK > revisions[j].SK
	})

	// Apply cursor
	startIdx := 0
	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		if !strings.HasPrefix(cursor, "VERSION#") {
			cursor = fmt.Sprintf("VERSION#%s", cursor)
		}
		for i, revision := range revisions {
			if revision.SK < cursor {
				startIdx = i
				break
			}
		}
	}

	// Apply limit
	endIdx := startIdx + limit
	if endIdx > len(revisions) {
		endIdx = len(revisions)
	}

	result := revisions[startIdx:endIdx]
	nextCursor := ""
	if endIdx < len(revisions) && len(result) > 0 {
		nextCursor = result[len(result)-1].SK
	}

	return result, nextCursor, nil
}

// Clear clears all data (test helper)
func (r *RevisionRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.revisions = make(map[string]*models.Revision)
	r.revisionsByObject = make(map[string][]string)
}

// Delete deletes a revision by PK and SK
func (r *RevisionRepository) Delete(_ context.Context, pk, sk string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Extract objectID from PK (format: OBJECT#<objectID>#REVISION)
	pk = strings.TrimSpace(pk)
	if !strings.HasPrefix(pk, "OBJECT#") || !strings.HasSuffix(pk, "#REVISION") {
		return storage.ErrInvalidInput
	}
	objectID := strings.TrimSuffix(strings.TrimPrefix(pk, "OBJECT#"), "#REVISION")

	// Extract version from SK (format: VERSION#<version>)
	sk = strings.TrimSpace(sk)
	if !strings.HasPrefix(sk, "VERSION#") {
		return storage.ErrInvalidInput
	}
	versionStr := strings.TrimPrefix(sk, "VERSION#")
	var version int
	if _, err := fmt.Sscanf(versionStr, "%08d", &version); err != nil {
		return storage.ErrInvalidInput
	}

	key := revisionKey(objectID, version)
	if _, exists := r.revisions[key]; !exists {
		return storage.ErrNotFound
	}

	// Remove from revisions map
	delete(r.revisions, key)

	// Remove from revisionsByObject index
	keys := r.revisionsByObject[objectID]
	for i, k := range keys {
		if k == key {
			r.revisionsByObject[objectID] = append(keys[:i], keys[i+1:]...)
			break
		}
	}

	return nil
}

// Ensure RevisionRepository implements interfaces.RevisionRepository
var _ interfaces.RevisionRepository = (*RevisionRepository)(nil)
