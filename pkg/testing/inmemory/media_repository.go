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
	"github.com/google/uuid"
)

// MediaRepository is a thread-safe in-memory implementation of interfaces.MediaRepository.
type MediaRepository struct {
	mu sync.RWMutex

	// Media: key = mediaID
	media map[string]*models.Media

	// Index by user: userID -> []mediaID
	byUser map[string][]string

	// Index by status: status -> []mediaID
	byStatus map[string][]string

	// Index by content type: contentType -> []mediaID
	byContentType map[string][]string

	// Media jobs: key = jobID
	jobs map[string]*models.MediaJob

	// Jobs by user: username -> []jobID
	jobsByUser map[string][]string

	// Jobs by status: status -> []jobID
	jobsByStatus map[string][]string

	// User media configs: key = userID
	configs map[string]*models.UserMediaConfig

	// Media spending: key = "userID:period"
	spending map[string]*models.MediaSpending

	// Spending transactions: key = transactionID
	transactions map[string]*models.MediaSpendingTransaction

	// Transactions by user: userID -> []transactionID
	transactionsByUser map[string][]string

	// Transcoding jobs: key = jobID
	transcodingJobs map[string]*models.TranscodingJob

	// Dependencies
	deps map[string]interface{}
}

// NewMediaRepository creates a new in-memory media repository
func NewMediaRepository() *MediaRepository {
	return &MediaRepository{
		media:              make(map[string]*models.Media),
		byUser:             make(map[string][]string),
		byStatus:           make(map[string][]string),
		byContentType:      make(map[string][]string),
		jobs:               make(map[string]*models.MediaJob),
		jobsByUser:         make(map[string][]string),
		jobsByStatus:       make(map[string][]string),
		configs:            make(map[string]*models.UserMediaConfig),
		spending:           make(map[string]*models.MediaSpending),
		transactions:       make(map[string]*models.MediaSpendingTransaction),
		transactionsByUser: make(map[string][]string),
		transcodingJobs:    make(map[string]*models.TranscodingJob),
		deps:               make(map[string]interface{}),
	}
}

// spendingKey generates a unique key for media spending
func spendingKey(userID, period string) string {
	return fmt.Sprintf("%s:%s", userID, period)
}

// CreateMedia creates a new media record
func (r *MediaRepository) CreateMedia(_ context.Context, media *models.Media) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if media == nil {
		return fmt.Errorf("media cannot be nil")
	}

	if media.MediaID == "" {
		media.MediaID = uuid.New().String()
	}

	if _, exists := r.media[media.MediaID]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	media.CreatedAt = now
	media.UpdatedAt = now
	media.UploadedAt = now

	r.media[media.MediaID] = media
	r.byUser[media.UserID] = append(r.byUser[media.UserID], media.MediaID)
	r.byStatus[media.Status] = append(r.byStatus[media.Status], media.MediaID)
	r.byContentType[media.ContentType] = append(r.byContentType[media.ContentType], media.MediaID)

	return nil
}

// GetMedia retrieves a media record by ID
func (r *MediaRepository) GetMedia(_ context.Context, mediaID string) (*models.Media, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	media, exists := r.media[mediaID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return media, nil
}

// UpdateMedia updates an existing media record
func (r *MediaRepository) UpdateMedia(_ context.Context, media *models.Media) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if media == nil {
		return fmt.Errorf("media cannot be nil")
	}

	existing, exists := r.media[media.MediaID]
	if !exists {
		return storage.ErrNotFound
	}

	// Update status index if changed
	if existing.Status != media.Status {
		r.byStatus[existing.Status] = removeMediaKeyFromSlice(r.byStatus[existing.Status], media.MediaID)
		r.byStatus[media.Status] = append(r.byStatus[media.Status], media.MediaID)
	}

	media.UpdatedAt = time.Now()
	r.media[media.MediaID] = media

	return nil
}

// UpdateMediaPublishedState models the production field-scoped writer: only the
// durable published-serving attributes change, other metadata is untouched.
func (r *MediaRepository) UpdateMediaPublishedState(_ context.Context, mediaID string, publishedS3Key, publishedURL string, publishedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.media[mediaID]
	if !exists {
		return storage.ErrNotFound
	}
	stored := *existing
	stored.PublishedS3Key = strings.TrimSpace(publishedS3Key)
	stored.PublishedURL = strings.TrimSpace(publishedURL)
	stored.PublishedAt = &publishedAt
	stored.UpdatedAt = time.Now()
	r.media[mediaID] = &stored
	return nil
}

// UpdateMediaEditorialState models the production field-scoped writer: only the
// lifecycle attributes change, other metadata is untouched.
func (r *MediaRepository) UpdateMediaEditorialState(_ context.Context, mediaID string, state models.EditorialLifecycle, supersededByMediaID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.media[mediaID]
	if !exists {
		return storage.ErrNotFound
	}
	state = models.EditorialLifecycle(strings.TrimSpace(string(state)))
	supersededByMediaID = strings.TrimSpace(supersededByMediaID)
	if state == models.EditorialLifecycleSuperseded && supersededByMediaID == "" {
		return fmt.Errorf("superseded editorial media must name the superseding asset")
	}
	stored := *existing
	if state == "" || state == models.EditorialLifecycleAvailable {
		stored.EditorialState = ""
	} else {
		stored.EditorialState = state
	}
	stored.SupersededByMediaID = supersededByMediaID
	stored.UpdatedAt = time.Now()
	r.media[mediaID] = &stored
	return nil
}

// DeleteMedia removes a media record
func (r *MediaRepository) DeleteMedia(_ context.Context, mediaID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	media, exists := r.media[mediaID]
	if !exists {
		return storage.ErrNotFound
	}

	r.byUser[media.UserID] = removeMediaKeyFromSlice(r.byUser[media.UserID], mediaID)
	r.byStatus[media.Status] = removeMediaKeyFromSlice(r.byStatus[media.Status], mediaID)
	r.byContentType[media.ContentType] = removeMediaKeyFromSlice(r.byContentType[media.ContentType], mediaID)
	delete(r.media, mediaID)

	return nil
}

// GetMediaByUser retrieves media for a user
func (r *MediaRepository) GetMediaByUser(_ context.Context, userID string, limit int) ([]*models.Media, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mediaIDs := r.byUser[userID]
	safeLimit := clampMediaLimit(limit)

	result := make([]*models.Media, 0, safeLimit)
	for i, id := range mediaIDs {
		if i >= safeLimit {
			break
		}
		if m, exists := r.media[id]; exists {
			result = append(result, m)
		}
	}

	return result, nil
}

// GetMediaByStatus retrieves media by status
func (r *MediaRepository) GetMediaByStatus(_ context.Context, status string, limit int) ([]*models.Media, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mediaIDs := r.byStatus[status]
	safeLimit := clampMediaLimit(limit)

	result := make([]*models.Media, 0, safeLimit)
	for i, id := range mediaIDs {
		if i >= safeLimit {
			break
		}
		if m, exists := r.media[id]; exists {
			result = append(result, m)
		}
	}

	return result, nil
}

// GetMediaByContentType retrieves media by content type
func (r *MediaRepository) GetMediaByContentType(_ context.Context, contentType string, limit int) ([]*models.Media, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mediaIDs := r.byContentType[contentType]
	safeLimit := clampMediaLimit(limit)

	result := make([]*models.Media, 0, safeLimit)
	for i, id := range mediaIDs {
		if i >= safeLimit {
			break
		}
		if m, exists := r.media[id]; exists {
			result = append(result, m)
		}
	}

	return result, nil
}

// GetUserMediaLegacy retrieves user media in legacy format
func (r *MediaRepository) GetUserMediaLegacy(_ context.Context, username string) ([]any, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mediaIDs := r.byUser[username]
	result := make([]any, 0, len(mediaIDs))

	for _, id := range mediaIDs {
		if m, exists := r.media[id]; exists {
			result = append(result, m)
		}
	}

	return result, nil
}

// GetUserMedia retrieves media for a user with pagination
func (r *MediaRepository) GetUserMedia(_ context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mediaIDs := r.byUser[userID]
	return paginateMediaByIDs(r.media, mediaIDs, opts), nil
}

// GetUserMediaByType retrieves media for a user by content type with pagination
func (r *MediaRepository) GetUserMediaByType(_ context.Context, userID, contentType string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mediaIDs := r.byUser[userID]
	filteredIDs := make([]string, 0)
	for _, id := range mediaIDs {
		if m, exists := r.media[id]; exists && m.ContentType == contentType {
			filteredIDs = append(filteredIDs, id)
		}
	}

	return paginateMediaByIDs(r.media, filteredIDs, opts), nil
}

// GetUnusedMedia retrieves unused media older than a given time
func (r *MediaRepository) GetUnusedMedia(_ context.Context, olderThan time.Time, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	unusedIDs := make([]string, 0)
	for id, m := range r.media {
		if m.UsageCount == 0 && m.CreatedAt.Before(olderThan) {
			unusedIDs = append(unusedIDs, id)
		}
	}

	return paginateMediaByIDs(r.media, unusedIDs, opts), nil
}

// MarkMediaUsed marks media as used
func (r *MediaRepository) MarkMediaUsed(_ context.Context, mediaID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	media, exists := r.media[mediaID]
	if !exists {
		return storage.ErrNotFound
	}

	media.UsageCount++
	now := time.Now()
	media.LastUsedAt = &now
	media.UpdatedAt = now

	return nil
}

// GetMediaUsageStats retrieves usage stats for media
func (r *MediaRepository) GetMediaUsageStats(_ context.Context, mediaID string) (usageCount int, lastUsed *time.Time, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	media, exists := r.media[mediaID]
	if !exists {
		return 0, nil, storage.ErrNotFound
	}

	return media.UsageCount, media.LastUsedAt, nil
}

// SetMediaModeration sets moderation results for media
func (r *MediaRepository) SetMediaModeration(_ context.Context, mediaID string, isNSFW bool, score float64, labels []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	media, exists := r.media[mediaID]
	if !exists {
		return storage.ErrNotFound
	}

	media.IsNSFW = isNSFW
	media.ModerationScore = score
	media.Labels = labels
	media.UpdatedAt = time.Now()

	return nil
}

// GetModerationPendingMedia retrieves media pending moderation
func (r *MediaRepository) GetModerationPendingMedia(_ context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pendingIDs := make([]string, 0)
	for id, m := range r.media {
		if m.ModerationScore == 0 && m.Status == "ready" {
			pendingIDs = append(pendingIDs, id)
		}
	}

	return paginateMediaByIDs(r.media, pendingIDs, opts), nil
}

// GetMediaByIDs retrieves multiple media by IDs
func (r *MediaRepository) GetMediaByIDs(_ context.Context, mediaIDs []string) ([]*models.Media, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*models.Media, 0, len(mediaIDs))
	for _, id := range mediaIDs {
		if m, exists := r.media[id]; exists {
			result = append(result, m)
		}
	}

	return result, nil
}

// DeleteExpiredMedia deletes media that expired before a given time
func (r *MediaRepository) DeleteExpiredMedia(_ context.Context, expiredBefore time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var count int64
	toDelete := make([]string, 0)

	for id, m := range r.media {
		if m.ExpiresAt > 0 && time.Unix(m.ExpiresAt, 0).Before(expiredBefore) {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		if m, exists := r.media[id]; exists {
			r.byUser[m.UserID] = removeMediaKeyFromSlice(r.byUser[m.UserID], id)
			r.byStatus[m.Status] = removeMediaKeyFromSlice(r.byStatus[m.Status], id)
			r.byContentType[m.ContentType] = removeMediaKeyFromSlice(r.byContentType[m.ContentType], id)
			delete(r.media, id)
			count++
		}
	}

	return count, nil
}

// GetMediaStorageUsage retrieves total storage usage for a user
func (r *MediaRepository) GetMediaStorageUsage(_ context.Context, userID string) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var total int64
	mediaIDs := r.byUser[userID]
	for _, id := range mediaIDs {
		if m, exists := r.media[id]; exists {
			total += m.FileSize
		}
	}

	return total, nil
}

// GetTotalStorageUsage retrieves total storage usage across all users
func (r *MediaRepository) GetTotalStorageUsage(_ context.Context) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var total int64
	for _, m := range r.media {
		total += m.FileSize
	}

	return total, nil
}

// MarkMediaProcessing marks media as processing
func (r *MediaRepository) MarkMediaProcessing(_ context.Context, mediaID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	media, exists := r.media[mediaID]
	if !exists {
		return storage.ErrNotFound
	}

	oldStatus := media.Status
	media.Status = "processing"
	media.UpdatedAt = time.Now()

	r.byStatus[oldStatus] = removeMediaKeyFromSlice(r.byStatus[oldStatus], mediaID)
	r.byStatus["processing"] = append(r.byStatus["processing"], mediaID)

	return nil
}

// MarkMediaReady marks media as ready
func (r *MediaRepository) MarkMediaReady(_ context.Context, mediaID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	media, exists := r.media[mediaID]
	if !exists {
		return storage.ErrNotFound
	}

	oldStatus := media.Status
	media.Status = "ready"
	now := time.Now()
	media.ProcessedAt = &now
	media.UpdatedAt = now

	r.byStatus[oldStatus] = removeMediaKeyFromSlice(r.byStatus[oldStatus], mediaID)
	r.byStatus["ready"] = append(r.byStatus["ready"], mediaID)

	return nil
}

// MarkMediaFailed marks media as failed
func (r *MediaRepository) MarkMediaFailed(_ context.Context, mediaID, errorMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	media, exists := r.media[mediaID]
	if !exists {
		return storage.ErrNotFound
	}

	oldStatus := media.Status
	media.Status = statusFailed
	media.Error = errorMsg
	now := time.Now()
	media.ProcessedAt = &now
	media.UpdatedAt = now

	r.byStatus[oldStatus] = removeMediaKeyFromSlice(r.byStatus[oldStatus], mediaID)
	r.byStatus[statusFailed] = append(r.byStatus[statusFailed], mediaID)

	return nil
}

// GetPendingMedia retrieves pending media with pagination
func (r *MediaRepository) GetPendingMedia(_ context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mediaIDs := r.byStatus[statusPending]
	return paginateMediaByIDs(r.media, mediaIDs, opts), nil
}

// GetProcessingMedia retrieves processing media with pagination
func (r *MediaRepository) GetProcessingMedia(_ context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mediaIDs := r.byStatus["processing"]
	return paginateMediaByIDs(r.media, mediaIDs, opts), nil
}

// AddMediaVariant adds a variant to media
func (r *MediaRepository) AddMediaVariant(_ context.Context, mediaID, variantName string, variant models.MediaVariant) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	media, exists := r.media[mediaID]
	if !exists {
		return storage.ErrNotFound
	}

	if media.Variants == nil {
		media.Variants = make(map[string]models.MediaVariant)
	}
	media.Variants[variantName] = variant
	media.UpdatedAt = time.Now()

	return nil
}

// GetMediaVariant retrieves a media variant
func (r *MediaRepository) GetMediaVariant(_ context.Context, mediaID, variantName string) (*models.MediaVariant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	media, exists := r.media[mediaID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	if media.Variants == nil {
		return nil, storage.ErrNotFound
	}

	variant, exists := media.Variants[variantName]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return &variant, nil
}

// DeleteMediaVariant removes a media variant
func (r *MediaRepository) DeleteMediaVariant(_ context.Context, mediaID, variantName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	media, exists := r.media[mediaID]
	if !exists {
		return storage.ErrNotFound
	}

	if media.Variants != nil {
		delete(media.Variants, variantName)
		media.UpdatedAt = time.Now()
	}

	return nil
}

// UpdateMediaAttachment updates media attachment fields
func (r *MediaRepository) UpdateMediaAttachment(_ context.Context, mediaID string, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	media, exists := r.media[mediaID]
	if !exists {
		return storage.ErrNotFound
	}

	// Apply updates
	if desc, ok := updates["description"].(string); ok {
		media.Description = desc
	}
	if focus, ok := updates["focus"].(string); ok {
		media.Focus = focus
	}
	media.UpdatedAt = time.Now()

	return nil
}

// UnmarkAllMediaAsSensitive unmarks all media for a user as sensitive
func (r *MediaRepository) UnmarkAllMediaAsSensitive(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	mediaIDs := r.byUser[username]
	for _, id := range mediaIDs {
		if m, exists := r.media[id]; exists {
			m.IsNSFW = false
			m.UpdatedAt = time.Now()
		}
	}

	return nil
}

// CreateMediaJob creates a new media job
func (r *MediaRepository) CreateMediaJob(_ context.Context, job *models.MediaJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}

	if job.JobID == "" {
		job.JobID = uuid.New().String()
	}

	if _, exists := r.jobs[job.JobID]; exists {
		return storage.ErrAlreadyExists
	}

	r.jobs[job.JobID] = job
	r.jobsByUser[job.Username] = append(r.jobsByUser[job.Username], job.JobID)
	r.jobsByStatus[job.Status] = append(r.jobsByStatus[job.Status], job.JobID)

	return nil
}

// GetMediaJob retrieves a media job by ID
func (r *MediaRepository) GetMediaJob(_ context.Context, jobID string) (*models.MediaJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	job, exists := r.jobs[jobID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return job, nil
}

// UpdateMediaJob updates a media job
func (r *MediaRepository) UpdateMediaJob(_ context.Context, job *models.MediaJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}

	existing, exists := r.jobs[job.JobID]
	if !exists {
		return storage.ErrNotFound
	}

	// Update status index if changed
	if existing.Status != job.Status {
		r.jobsByStatus[existing.Status] = removeMediaKeyFromSlice(r.jobsByStatus[existing.Status], job.JobID)
		r.jobsByStatus[job.Status] = append(r.jobsByStatus[job.Status], job.JobID)
	}

	r.jobs[job.JobID] = job

	return nil
}

// DeleteMediaJob removes a media job
func (r *MediaRepository) DeleteMediaJob(_ context.Context, jobID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, exists := r.jobs[jobID]
	if !exists {
		return storage.ErrNotFound
	}

	r.jobsByUser[job.Username] = removeMediaKeyFromSlice(r.jobsByUser[job.Username], jobID)
	r.jobsByStatus[job.Status] = removeMediaKeyFromSlice(r.jobsByStatus[job.Status], jobID)
	delete(r.jobs, jobID)

	return nil
}

// GetJobsByStatus retrieves jobs by status
func (r *MediaRepository) GetJobsByStatus(_ context.Context, status string, limit int) ([]*models.MediaJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	jobIDs := r.jobsByStatus[status]
	safeLimit := clampMediaLimit(limit)

	result := make([]*models.MediaJob, 0, safeLimit)
	for i, id := range jobIDs {
		if i >= safeLimit {
			break
		}
		if j, exists := r.jobs[id]; exists {
			result = append(result, j)
		}
	}

	return result, nil
}

// GetJobsByUser retrieves jobs by user
func (r *MediaRepository) GetJobsByUser(_ context.Context, username string, limit int) ([]*models.MediaJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	jobIDs := r.jobsByUser[username]
	safeLimit := clampMediaLimit(limit)

	result := make([]*models.MediaJob, 0, safeLimit)
	for i, id := range jobIDs {
		if i >= safeLimit {
			break
		}
		if j, exists := r.jobs[id]; exists {
			result = append(result, j)
		}
	}

	return result, nil
}

// CreateUserMediaConfig creates a user media config
func (r *MediaRepository) CreateUserMediaConfig(_ context.Context, config *models.UserMediaConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if _, exists := r.configs[config.UserID]; exists {
		return storage.ErrAlreadyExists
	}

	r.configs[config.UserID] = config

	return nil
}

// GetUserMediaConfig retrieves a user media config
func (r *MediaRepository) GetUserMediaConfig(_ context.Context, userID string) (*models.UserMediaConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	config, exists := r.configs[userID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return config, nil
}

// GetUserMediaConfigByUsername retrieves a user media config by username
func (r *MediaRepository) GetUserMediaConfigByUsername(_ context.Context, username string) (*models.UserMediaConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, config := range r.configs {
		if config.Username == username {
			return config, nil
		}
	}

	return nil, storage.ErrNotFound
}

// UpdateUserMediaConfig updates a user media config
func (r *MediaRepository) UpdateUserMediaConfig(_ context.Context, config *models.UserMediaConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if _, exists := r.configs[config.UserID]; !exists {
		return storage.ErrNotFound
	}

	r.configs[config.UserID] = config

	return nil
}

// DeleteUserMediaConfig removes a user media config
func (r *MediaRepository) DeleteUserMediaConfig(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.configs[userID]; !exists {
		return storage.ErrNotFound
	}

	delete(r.configs, userID)

	return nil
}

// CreateMediaSpending creates a media spending record
func (r *MediaRepository) CreateMediaSpending(_ context.Context, spending *models.MediaSpending) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if spending == nil {
		return fmt.Errorf("spending cannot be nil")
	}

	key := spendingKey(spending.UserID, spending.Period)
	if _, exists := r.spending[key]; exists {
		return storage.ErrAlreadyExists
	}

	r.spending[key] = spending

	return nil
}

// GetMediaSpending retrieves a media spending record
func (r *MediaRepository) GetMediaSpending(_ context.Context, userID, period string) (*models.MediaSpending, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := spendingKey(userID, period)
	spending, exists := r.spending[key]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return spending, nil
}

// UpdateMediaSpending updates a media spending record
func (r *MediaRepository) UpdateMediaSpending(_ context.Context, spending *models.MediaSpending) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if spending == nil {
		return fmt.Errorf("spending cannot be nil")
	}

	key := spendingKey(spending.UserID, spending.Period)
	if _, exists := r.spending[key]; !exists {
		return storage.ErrNotFound
	}

	r.spending[key] = spending

	return nil
}

// GetMediaSpendingByTimeRange retrieves spending records by time range
func (r *MediaRepository) GetMediaSpendingByTimeRange(_ context.Context, userID string, periodType string, limit int) ([]*models.MediaSpending, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	safeLimit := clampMediaLimit(limit)
	result := make([]*models.MediaSpending, 0, safeLimit)

	for _, s := range r.spending {
		if s.UserID == userID && s.PeriodType == periodType {
			result = append(result, s)
			if len(result) >= safeLimit {
				break
			}
		}
	}

	return result, nil
}

// GetOrCreateMediaSpending gets or creates a media spending record
func (r *MediaRepository) GetOrCreateMediaSpending(_ context.Context, userID, period, periodType string) (*models.MediaSpending, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := spendingKey(userID, period)
	if spending, exists := r.spending[key]; exists {
		return spending, nil
	}

	spending := &models.MediaSpending{
		UserID:     userID,
		Period:     period,
		PeriodType: periodType,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	r.spending[key] = spending

	return spending, nil
}

// CreateMediaSpendingTransaction creates a spending transaction
func (r *MediaRepository) CreateMediaSpendingTransaction(_ context.Context, transaction *models.MediaSpendingTransaction) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if transaction == nil {
		return fmt.Errorf("transaction cannot be nil")
	}

	if transaction.TransactionID == "" {
		transaction.TransactionID = uuid.New().String()
	}

	if _, exists := r.transactions[transaction.TransactionID]; exists {
		return storage.ErrAlreadyExists
	}

	r.transactions[transaction.TransactionID] = transaction
	r.transactionsByUser[transaction.UserID] = append(r.transactionsByUser[transaction.UserID], transaction.TransactionID)

	return nil
}

// GetMediaSpendingTransactions retrieves spending transactions for a user
func (r *MediaRepository) GetMediaSpendingTransactions(_ context.Context, userID string, limit int) ([]*models.MediaSpendingTransaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	txIDs := r.transactionsByUser[userID]
	safeLimit := clampMediaLimit(limit)

	result := make([]*models.MediaSpendingTransaction, 0, safeLimit)
	for i, id := range txIDs {
		if i >= safeLimit {
			break
		}
		if tx, exists := r.transactions[id]; exists {
			result = append(result, tx)
		}
	}

	return result, nil
}

// AddSpendingTransaction adds a spending transaction (alias for CreateMediaSpendingTransaction)
func (r *MediaRepository) AddSpendingTransaction(ctx context.Context, transaction *models.MediaSpendingTransaction) error {
	return r.CreateMediaSpendingTransaction(ctx, transaction)
}

// CreateTranscodingJob creates a transcoding job
func (r *MediaRepository) CreateTranscodingJob(_ context.Context, job *models.TranscodingJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}

	if job.JobID == "" {
		job.JobID = uuid.New().String()
	}

	if _, exists := r.transcodingJobs[job.JobID]; exists {
		return storage.ErrAlreadyExists
	}

	r.transcodingJobs[job.JobID] = job

	return nil
}

// GetTranscodingJob retrieves a transcoding job
func (r *MediaRepository) GetTranscodingJob(_ context.Context, jobID string) (*models.TranscodingJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	job, exists := r.transcodingJobs[jobID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return job, nil
}

// UpdateTranscodingJob updates a transcoding job
func (r *MediaRepository) UpdateTranscodingJob(_ context.Context, job *models.TranscodingJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}

	if _, exists := r.transcodingJobs[job.JobID]; !exists {
		return storage.ErrNotFound
	}

	r.transcodingJobs[job.JobID] = job

	return nil
}

// GetTranscodingJobsByUser retrieves transcoding jobs by user
func (r *MediaRepository) GetTranscodingJobsByUser(_ context.Context, userID string, limit int) ([]*models.TranscodingJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	safeLimit := clampMediaLimit(limit)
	result := make([]*models.TranscodingJob, 0, safeLimit)

	for _, job := range r.transcodingJobs {
		if job.UserID == userID {
			result = append(result, job)
			if len(result) >= safeLimit {
				break
			}
		}
	}

	return result, nil
}

// GetTranscodingJobsByMedia retrieves transcoding jobs by media
func (r *MediaRepository) GetTranscodingJobsByMedia(_ context.Context, mediaID string, limit int) ([]*models.TranscodingJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	safeLimit := clampMediaLimit(limit)
	result := make([]*models.TranscodingJob, 0, safeLimit)

	for _, job := range r.transcodingJobs {
		if job.MediaID == mediaID {
			result = append(result, job)
			if len(result) >= safeLimit {
				break
			}
		}
	}

	return result, nil
}

// GetTranscodingJobsByStatus retrieves transcoding jobs by status
func (r *MediaRepository) GetTranscodingJobsByStatus(_ context.Context, status string, limit int) ([]*models.TranscodingJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	safeLimit := clampMediaLimit(limit)
	result := make([]*models.TranscodingJob, 0, safeLimit)

	for _, job := range r.transcodingJobs {
		if job.Status == status {
			result = append(result, job)
			if len(result) >= safeLimit {
				break
			}
		}
	}

	return result, nil
}

// DeleteTranscodingJob removes a transcoding job
func (r *MediaRepository) DeleteTranscodingJob(_ context.Context, jobID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.transcodingJobs[jobID]; !exists {
		return storage.ErrNotFound
	}

	delete(r.transcodingJobs, jobID)

	return nil
}

// GetTranscodingCostsByUser retrieves transcoding costs by user
func (r *MediaRepository) GetTranscodingCostsByUser(_ context.Context, userID string, _ string) (map[string]int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	costs := make(map[string]int64)

	for _, job := range r.transcodingJobs {
		if job.UserID == userID {
			costs[job.JobType] += job.TotalCostMicros
		}
	}

	return costs, nil
}

// SetDependencies sets repository dependencies
func (r *MediaRepository) SetDependencies(deps map[string]interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deps = deps
}

// Helper functions

func removeMediaKeyFromSlice(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

func clampMediaLimit(limit int) int {
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

func paginateMediaByIDs(media map[string]*models.Media, ids []string, opts interfaces.PaginationOptions) *interfaces.PaginatedResult[*models.Media] {
	if len(ids) == 0 {
		return &interfaces.PaginatedResult[*models.Media]{Items: []*models.Media{}}
	}

	// Sort by upload time
	sortedMedia := make([]*models.Media, 0, len(ids))
	for _, id := range ids {
		if m, exists := media[id]; exists {
			sortedMedia = append(sortedMedia, m)
		}
	}
	sort.Slice(sortedMedia, func(i, j int) bool {
		return sortedMedia[i].UploadedAt.After(sortedMedia[j].UploadedAt)
	})

	limit := clampMediaLimit(opts.Limit)

	startIdx := 0
	if opts.Cursor != "" {
		for i, m := range sortedMedia {
			if m.MediaID == opts.Cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []*models.Media
	var nextCursor string

	for i := startIdx; i < len(sortedMedia) && len(results) < limit; i++ {
		results = append(results, sortedMedia[i])
	}

	if startIdx+limit < len(sortedMedia) && len(results) > 0 {
		nextCursor = results[len(results)-1].MediaID
	}

	return &interfaces.PaginatedResult[*models.Media]{
		Items:      results,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}
}

// Test helper methods

// Clear clears all data (test helper)
func (r *MediaRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.media = make(map[string]*models.Media)
	r.byUser = make(map[string][]string)
	r.byStatus = make(map[string][]string)
	r.byContentType = make(map[string][]string)
	r.jobs = make(map[string]*models.MediaJob)
	r.jobsByUser = make(map[string][]string)
	r.jobsByStatus = make(map[string][]string)
	r.configs = make(map[string]*models.UserMediaConfig)
	r.spending = make(map[string]*models.MediaSpending)
	r.transactions = make(map[string]*models.MediaSpendingTransaction)
	r.transactionsByUser = make(map[string][]string)
	r.transcodingJobs = make(map[string]*models.TranscodingJob)
}

// GetMediaCount returns the number of media records (test helper)
func (r *MediaRepository) GetMediaCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.media)
}

// GetJobCount returns the number of jobs (test helper)
func (r *MediaRepository) GetJobCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.jobs)
}

// GetTranscodingJobCount returns the number of transcoding jobs (test helper)
func (r *MediaRepository) GetTranscodingJobCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.transcodingJobs)
}

// Ensure MediaRepository implements interfaces.MediaRepository
var _ interfaces.MediaRepository = (*MediaRepository)(nil)
