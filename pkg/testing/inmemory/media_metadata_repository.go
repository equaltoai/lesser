// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// MediaMetadataRepository is a thread-safe in-memory implementation of interfaces.MediaMetadataRepository.
type MediaMetadataRepository struct {
	mu sync.RWMutex

	// Metadata: key = mediaID
	metadata map[string]*models.MediaMetadata

	// Index by status: status -> []mediaID
	byStatus map[string][]string
}

// NewMediaMetadataRepository creates a new in-memory media metadata repository
func NewMediaMetadataRepository() *MediaMetadataRepository {
	return &MediaMetadataRepository{
		metadata: make(map[string]*models.MediaMetadata),
		byStatus: make(map[string][]string),
	}
}

// CreateMediaMetadata creates a new media metadata record
func (r *MediaMetadataRepository) CreateMediaMetadata(_ context.Context, metadata *models.MediaMetadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if metadata == nil {
		return fmt.Errorf("metadata cannot be nil")
	}

	if _, exists := r.metadata[metadata.MediaID]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	metadata.CreatedAt = now
	metadata.UpdatedAt = now

	if metadata.Status == "" {
		metadata.Status = statusPending
	}

	r.metadata[metadata.MediaID] = metadata
	r.byStatus[metadata.Status] = append(r.byStatus[metadata.Status], metadata.MediaID)

	return nil
}

// GetMediaMetadata retrieves media metadata by ID
func (r *MediaMetadataRepository) GetMediaMetadata(_ context.Context, mediaID string) (*models.MediaMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metadata, exists := r.metadata[mediaID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return metadata, nil
}

// UpdateMediaMetadata updates an existing media metadata record
func (r *MediaMetadataRepository) UpdateMediaMetadata(_ context.Context, metadata *models.MediaMetadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if metadata == nil {
		return fmt.Errorf("metadata cannot be nil")
	}

	existing, exists := r.metadata[metadata.MediaID]
	if !exists {
		return storage.ErrNotFound
	}

	// Update status index if changed
	if existing.Status != metadata.Status {
		r.byStatus[existing.Status] = removeMetadataKeyFromSlice(r.byStatus[existing.Status], metadata.MediaID)
		r.byStatus[metadata.Status] = append(r.byStatus[metadata.Status], metadata.MediaID)
	}

	metadata.UpdatedAt = time.Now()
	r.metadata[metadata.MediaID] = metadata

	return nil
}

// DeleteMediaMetadata removes a media metadata record
func (r *MediaMetadataRepository) DeleteMediaMetadata(_ context.Context, mediaID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	metadata, exists := r.metadata[mediaID]
	if !exists {
		return storage.ErrNotFound
	}

	r.byStatus[metadata.Status] = removeMetadataKeyFromSlice(r.byStatus[metadata.Status], mediaID)
	delete(r.metadata, mediaID)

	return nil
}

// GetMediaMetadataByStatus retrieves metadata by status
func (r *MediaMetadataRepository) GetMediaMetadataByStatus(_ context.Context, status string, limit int) ([]*models.MediaMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mediaIDs := r.byStatus[status]
	safeLimit := clampMetadataLimit(limit)

	result := make([]*models.MediaMetadata, 0, safeLimit)
	for i, id := range mediaIDs {
		if i >= safeLimit {
			break
		}
		if m, exists := r.metadata[id]; exists {
			result = append(result, m)
		}
	}

	return result, nil
}

// GetPendingMediaMetadata retrieves pending metadata
func (r *MediaMetadataRepository) GetPendingMediaMetadata(ctx context.Context, limit int) ([]*models.MediaMetadata, error) {
	return r.GetMediaMetadataByStatus(ctx, statusPending, limit)
}

// GetProcessingMediaMetadata retrieves processing metadata
func (r *MediaMetadataRepository) GetProcessingMediaMetadata(ctx context.Context, limit int) ([]*models.MediaMetadata, error) {
	return r.GetMediaMetadataByStatus(ctx, "processing", limit)
}

// MarkProcessingStarted marks metadata as processing
func (r *MediaMetadataRepository) MarkProcessingStarted(_ context.Context, mediaID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	metadata, exists := r.metadata[mediaID]
	if !exists {
		return storage.ErrNotFound
	}

	oldStatus := metadata.Status
	metadata.Status = "processing"
	metadata.UpdatedAt = time.Now()

	r.byStatus[oldStatus] = removeMetadataKeyFromSlice(r.byStatus[oldStatus], mediaID)
	r.byStatus["processing"] = append(r.byStatus["processing"], mediaID)

	return nil
}

// MarkProcessingComplete marks metadata as complete with result
func (r *MediaMetadataRepository) MarkProcessingComplete(_ context.Context, mediaID string, result interfaces.ProcessingResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	metadata, exists := r.metadata[mediaID]
	if !exists {
		return storage.ErrNotFound
	}

	oldStatus := metadata.Status
	metadata.Status = "complete"
	metadata.Width = result.Width
	metadata.Height = result.Height
	metadata.Duration = float64(result.Duration) / 1000.0 // Convert ms to seconds
	metadata.FileSize = int64(result.FileSize)
	metadata.Blurhash = result.Blurhash
	now := time.Now()
	metadata.ProcessedAt = now
	metadata.UpdatedAt = now

	r.byStatus[oldStatus] = removeMetadataKeyFromSlice(r.byStatus[oldStatus], mediaID)
	r.byStatus["complete"] = append(r.byStatus["complete"], mediaID)

	return nil
}

// MarkProcessingFailed marks metadata as failed
func (r *MediaMetadataRepository) MarkProcessingFailed(_ context.Context, mediaID string, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	metadata, exists := r.metadata[mediaID]
	if !exists {
		return storage.ErrNotFound
	}

	oldStatus := metadata.Status
	metadata.Status = statusFailed
	now := time.Now()
	metadata.ProcessedAt = now
	metadata.UpdatedAt = now
	// Set TTL for cleanup
	metadata.TTL = now.Add(7 * 24 * time.Hour).Unix()

	r.byStatus[oldStatus] = removeMetadataKeyFromSlice(r.byStatus[oldStatus], mediaID)
	r.byStatus[statusFailed] = append(r.byStatus[statusFailed], mediaID)

	return nil
}

// CleanupExpiredMetadata removes expired metadata
func (r *MediaMetadataRepository) CleanupExpiredMetadata(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().Unix()
	toDelete := make([]string, 0)

	for id, m := range r.metadata {
		if m.TTL > 0 && m.TTL < now {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		if m, exists := r.metadata[id]; exists {
			r.byStatus[m.Status] = removeMetadataKeyFromSlice(r.byStatus[m.Status], id)
			delete(r.metadata, id)
		}
	}

	return nil
}

// Helper functions

func removeMetadataKeyFromSlice(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

func clampMetadataLimit(limit int) int {
	const defaultLimit = 20
	const maxLimit = 100

	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// Test helper methods

// Clear clears all data (test helper)
func (r *MediaMetadataRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.metadata = make(map[string]*models.MediaMetadata)
	r.byStatus = make(map[string][]string)
}

// GetMetadataCount returns the number of metadata records (test helper)
func (r *MediaMetadataRepository) GetMetadataCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.metadata)
}

// Ensure MediaMetadataRepository implements interfaces.MediaMetadataRepository
var _ interfaces.MediaMetadataRepository = (*MediaMetadataRepository)(nil)
