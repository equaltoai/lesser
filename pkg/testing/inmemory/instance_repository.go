// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// InstanceRepository is a thread-safe in-memory implementation of interfaces.InstanceRepository.
type InstanceRepository struct {
	mu sync.RWMutex

	state               *models.InstanceState
	rules               []storage.InstanceRule
	extendedDescription string
	descriptionUpdated  time.Time
	weeklyActivities    map[int64]*storage.WeeklyActivity
	dailyMetrics        map[string]map[string]interface{}
}

// NewInstanceRepository creates a new in-memory instance repository
func NewInstanceRepository() *InstanceRepository {
	return &InstanceRepository{
		state: &models.InstanceState{
			Locked: false,
		},
		rules:            []storage.InstanceRule{},
		weeklyActivities: make(map[int64]*storage.WeeklyActivity),
		dailyMetrics:     make(map[string]map[string]interface{}),
	}
}

// GetInstanceState returns the current instance activation state
func (r *InstanceRepository) GetInstanceState(_ context.Context) (*models.InstanceState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state, nil
}

// EnsureInstanceState ensures the instance state record exists and returns it
func (r *InstanceRepository) EnsureInstanceState(_ context.Context) (*models.InstanceState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state == nil {
		r.state = &models.InstanceState{Locked: false}
	}
	return r.state, nil
}

// SetInstanceLocked updates the instance lock state
func (r *InstanceRepository) SetInstanceLocked(_ context.Context, locked bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.state.Locked = locked
	return nil
}


// SetBootstrapWalletAddress sets the bootstrap wallet address used for setup authentication
func (r *InstanceRepository) SetBootstrapWalletAddress(_ context.Context, address string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.state.BootstrapWalletAddress = address
	return nil
}

// SetPrimaryAdminUsername records the primary admin username created during setup
func (r *InstanceRepository) SetPrimaryAdminUsername(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.state.PrimaryAdminUsername = username
	return nil
}

// GetInstanceRules retrieves the instance rules
func (r *InstanceRepository) GetInstanceRules(_ context.Context) ([]storage.InstanceRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.rules, nil
}

// SetInstanceRules updates the instance rules
func (r *InstanceRepository) SetInstanceRules(_ context.Context, rules []storage.InstanceRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rules = rules
	return nil
}

// GetRulesByCategory retrieves rules filtered by category
// Note: InstanceRule doesn't have a Category field, so this returns all rules
func (r *InstanceRepository) GetRulesByCategory(_ context.Context, category string) ([]storage.InstanceRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return all rules since InstanceRule doesn't have a Category field
	result := make([]storage.InstanceRule, 0, len(r.rules))
	for _, rule := range r.rules {
		result = append(result, rule)
	}
	return result, nil
}

// GetExtendedDescription retrieves the instance extended description
func (r *InstanceRepository) GetExtendedDescription(_ context.Context) (string, time.Time, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.extendedDescription, r.descriptionUpdated, nil
}

// SetExtendedDescription updates the instance extended description
func (r *InstanceRepository) SetExtendedDescription(_ context.Context, description string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.extendedDescription = description
	r.descriptionUpdated = time.Now()
	return nil
}

// GetTotalUserCount returns the total number of users
func (r *InstanceRepository) GetTotalUserCount(_ context.Context) (int64, error) {
	return 0, nil
}

// GetTotalStatusCount returns the total number of statuses
func (r *InstanceRepository) GetTotalStatusCount(_ context.Context) (int64, error) {
	return 0, nil
}


// GetTotalDomainCount returns the total number of known domains
func (r *InstanceRepository) GetTotalDomainCount(_ context.Context) (int64, error) {
	return 0, nil
}

// GetActiveUserCount returns the number of active users in the last N days
func (r *InstanceRepository) GetActiveUserCount(_ context.Context, days int) (int64, error) {
	return 0, nil
}

// GetDailyActiveUserCount returns the number of daily active users
func (r *InstanceRepository) GetDailyActiveUserCount(_ context.Context) (int64, error) {
	return 0, nil
}

// GetLocalPostCount returns the number of local posts
func (r *InstanceRepository) GetLocalPostCount(_ context.Context) (int64, error) {
	return 0, nil
}

// GetLocalCommentCount returns the number of local comments
func (r *InstanceRepository) GetLocalCommentCount(_ context.Context) (int64, error) {
	return 0, nil
}

// GetWeeklyActivity retrieves weekly activity data for a specific week
func (r *InstanceRepository) GetWeeklyActivity(_ context.Context, weekTimestamp int64) (*storage.WeeklyActivity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	activity, exists := r.weeklyActivities[weekTimestamp]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return activity, nil
}

// RecordActivity records activity data for analytics
func (r *InstanceRepository) RecordActivity(_ context.Context, activityType string, userID string, timestamp time.Time) error {
	return nil
}

// GetContactAccount returns the contact account for the instance
func (r *InstanceRepository) GetContactAccount(_ context.Context) (*storage.ActorRecord, error) {
	return nil, storage.ErrNotFound
}

// GetStorageUsage returns current storage usage statistics
func (r *InstanceRepository) GetStorageUsage(_ context.Context) (any, error) {
	return map[string]int64{}, nil
}

// GetStorageHistory returns storage usage history for the last N days
func (r *InstanceRepository) GetStorageHistory(_ context.Context, days int) ([]any, error) {
	return []any{}, nil
}

// GetUserGrowthHistory returns user growth data for the last N days
func (r *InstanceRepository) GetUserGrowthHistory(_ context.Context, days int) ([]any, error) {
	return []any{}, nil
}

// GetDomainStats returns statistics for a specific domain
func (r *InstanceRepository) GetDomainStats(_ context.Context, domain string) (any, error) {
	return map[string]any{}, nil
}


// RecordDailyMetrics records daily historical metrics for the instance
func (r *InstanceRepository) RecordDailyMetrics(_ context.Context, date string, metrics map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.dailyMetrics[date] = metrics
	return nil
}

// GetMetricsSummary returns aggregated metrics for a given time range
func (r *InstanceRepository) GetMetricsSummary(_ context.Context, timeRange string) (map[string]interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return map[string]interface{}{}, nil
}

// Clear clears all data (test helper)
func (r *InstanceRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.state = &models.InstanceState{Locked: false}
	r.rules = []storage.InstanceRule{}
	r.extendedDescription = ""
	r.weeklyActivities = make(map[int64]*storage.WeeklyActivity)
	r.dailyMetrics = make(map[string]map[string]interface{})
}

// Ensure InstanceRepository implements interfaces.InstanceRepository
var _ interfaces.InstanceRepository = (*InstanceRepository)(nil)
