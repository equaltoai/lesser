// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// TrendingRepository is a thread-safe in-memory implementation of interfaces.TrendingRepository.
type TrendingRepository struct {
	mu sync.RWMutex

	// Hashtag usage records
	hashtagUsage map[string][]*hashtagUsageRecord

	// Status engagement records
	statusEngagement map[string][]*engagementRecord

	// Link share records
	linkShares map[string][]*linkShareRecord

	// Engagement metrics: key = statusID
	engagementMetrics map[string]*storage.EngagementMetrics

	// Status repository for cross-repository operations
	statusRepo interface{}
}

type hashtagUsageRecord struct {
	StatusID  string
	AuthorID  string
	Timestamp time.Time
}

type engagementRecord struct {
	Type      string
	UserID    string
	Timestamp time.Time
}

type linkShareRecord struct {
	StatusID  string
	AuthorID  string
	Timestamp time.Time
}

func buildTrending[K comparable, R any, Out any](records map[K][]R, since time.Time, limit int, getTime func(R) time.Time, build func(K, int) Out) []Out {
	type count struct {
		key   K
		count int
	}

	counts := make([]count, 0, len(records))
	for key, recs := range records {
		c := 0
		for _, rec := range recs {
			if getTime(rec).After(since) {
				c++
			}
		}
		if c > 0 {
			counts = append(counts, count{key: key, count: c})
		}
	}

	sort.Slice(counts, func(i, j int) bool {
		return counts[i].count > counts[j].count
	})

	if limit > 0 && len(counts) > limit {
		counts = counts[:limit]
	}

	result := make([]Out, 0, len(counts))
	for _, c := range counts {
		result = append(result, build(c.key, c.count))
	}

	return result
}

// NewTrendingRepository creates a new in-memory trending repository
func NewTrendingRepository() *TrendingRepository {
	return &TrendingRepository{
		hashtagUsage:      make(map[string][]*hashtagUsageRecord),
		statusEngagement:  make(map[string][]*engagementRecord),
		linkShares:        make(map[string][]*linkShareRecord),
		engagementMetrics: make(map[string]*storage.EngagementMetrics),
	}
}

// RecordHashtagUsage records when a hashtag is used in a status
func (r *TrendingRepository) RecordHashtagUsage(_ context.Context, hashtag string, statusID string, authorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.hashtagUsage[hashtag] = append(r.hashtagUsage[hashtag], &hashtagUsageRecord{
		StatusID:  statusID,
		AuthorID:  authorID,
		Timestamp: time.Now(),
	})
	return nil
}

// RecordStatusEngagement records engagement on a status (like, boost, reply)
func (r *TrendingRepository) RecordStatusEngagement(_ context.Context, statusID string, engagementType string, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.statusEngagement[statusID] = append(r.statusEngagement[statusID], &engagementRecord{
		Type:      engagementType,
		UserID:    userID,
		Timestamp: time.Now(),
	})
	return nil
}

// RecordLinkShare records when a link is shared in a status
func (r *TrendingRepository) RecordLinkShare(_ context.Context, linkURL string, statusID string, authorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.linkShares[linkURL] = append(r.linkShares[linkURL], &linkShareRecord{
		StatusID:  statusID,
		AuthorID:  authorID,
		Timestamp: time.Now(),
	})
	return nil
}

// GetTrendingHashtags returns the top trending hashtags since the given time
func (r *TrendingRepository) GetTrendingHashtags(_ context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return buildTrending(r.hashtagUsage, since, limit, func(rec *hashtagUsageRecord) time.Time {
		return rec.Timestamp
	}, func(hashtag string, count int) *storage.TrendingHashtag {
		return &storage.TrendingHashtag{
			Name:       hashtag,
			UsageCount: int64(count),
		}
	}), nil
}

// GetTrendingStatuses returns the top trending statuses since the given time
func (r *TrendingRepository) GetTrendingStatuses(_ context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return buildTrending(r.statusEngagement, since, limit, func(rec *engagementRecord) time.Time {
		return rec.Timestamp
	}, func(statusID string, count int) *storage.TrendingStatus {
		return &storage.TrendingStatus{
			StatusID: statusID,
			Score:    float64(count),
		}
	}), nil
}

// GetTrendingLinks returns the top trending links since the given time
func (r *TrendingRepository) GetTrendingLinks(_ context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return buildTrending(r.linkShares, since, limit, func(rec *linkShareRecord) time.Time {
		return rec.Timestamp
	}, func(url string, count int) *storage.TrendingLink {
		return &storage.TrendingLink{
			URL:        url,
			ShareCount: int64(count),
		}
	}), nil
}

// GetRecentStatusesWithEngagement returns recent statuses with engagement since the given time
func (r *TrendingRepository) GetRecentStatusesWithEngagement(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	return r.GetTrendingStatuses(ctx, since, limit)
}

// GetRecentLinks returns recent links since the given time
func (r *TrendingRepository) GetRecentLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	return r.GetTrendingLinks(ctx, since, limit)
}

// StoreEngagementMetrics stores engagement metrics for a status
func (r *TrendingRepository) StoreEngagementMetrics(_ context.Context, metrics *storage.EngagementMetrics) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.engagementMetrics[metrics.StatusID] = metrics
	return nil
}

// GetEngagementMetrics retrieves stored engagement metrics for a status
func (r *TrendingRepository) GetEngagementMetrics(_ context.Context, statusID string) (*storage.EngagementMetrics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metrics, exists := r.engagementMetrics[statusID]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return metrics, nil
}

// StoreHashtagTrend stores a hashtag trend record
func (r *TrendingRepository) StoreHashtagTrend(_ context.Context, _ any) error {
	return nil
}

// StoreStatusTrend stores a status trend record
func (r *TrendingRepository) StoreStatusTrend(_ context.Context, _ any) error {
	return nil
}

// StoreLinkTrend stores a link trend record
func (r *TrendingRepository) StoreLinkTrend(_ context.Context, _ any) error {
	return nil
}

// SetStatusRepository sets the status repository dependency for cross-repository operations
func (r *TrendingRepository) SetStatusRepository(statusRepo interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statusRepo = statusRepo
}

// Clear clears all data (test helper)
func (r *TrendingRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.hashtagUsage = make(map[string][]*hashtagUsageRecord)
	r.statusEngagement = make(map[string][]*engagementRecord)
	r.linkShares = make(map[string][]*linkShareRecord)
	r.engagementMetrics = make(map[string]*storage.EngagementMetrics)
}

// Ensure TrendingRepository implements interfaces.TrendingRepository
var _ interfaces.TrendingRepository = (*TrendingRepository)(nil)
