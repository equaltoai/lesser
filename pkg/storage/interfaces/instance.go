// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// InstanceRepository defines the interface for instance configuration and metrics operations.
// This handles instance settings, rules, metrics, and activity tracking.
type InstanceRepository interface {
	// Instance state operations

	// GetInstanceState returns the current instance activation state
	GetInstanceState(ctx context.Context) (*models.InstanceState, error)

	// EnsureInstanceState ensures the instance state record exists and returns it
	EnsureInstanceState(ctx context.Context) (*models.InstanceState, error)

	// SetInstanceLocked updates the instance lock state
	SetInstanceLocked(ctx context.Context, locked bool) error

	// SetBootstrapWalletAddress sets the bootstrap wallet address used for setup authentication
	SetBootstrapWalletAddress(ctx context.Context, address string) error

	// SetPrimaryAdminUsername records the primary admin username created during setup
	SetPrimaryAdminUsername(ctx context.Context, username string) error

	// Agent policy operations

	// GetAgentInstanceConfig returns the current instance agent policy.
	GetAgentInstanceConfig(ctx context.Context) (*models.AgentInstanceConfig, error)

	// EnsureAgentInstanceConfig ensures the instance agent policy record exists and returns it.
	EnsureAgentInstanceConfig(ctx context.Context) (*models.AgentInstanceConfig, error)

	// SetAgentInstanceConfig updates the instance agent policy.
	SetAgentInstanceConfig(ctx context.Context, cfg *models.AgentInstanceConfig) error

	// Instance rules operations

	// GetInstanceRules retrieves the instance rules
	GetInstanceRules(ctx context.Context) ([]storage.InstanceRule, error)

	// SetInstanceRules updates the instance rules
	SetInstanceRules(ctx context.Context, rules []storage.InstanceRule) error

	// GetRulesByCategory retrieves rules filtered by category
	GetRulesByCategory(ctx context.Context, category string) ([]storage.InstanceRule, error)

	// Instance description operations

	// GetExtendedDescription retrieves the instance extended description
	GetExtendedDescription(ctx context.Context) (string, time.Time, error)

	// SetExtendedDescription updates the instance extended description
	SetExtendedDescription(ctx context.Context, description string) error

	// Instance metrics operations

	// GetTotalUserCount returns the total number of users
	GetTotalUserCount(ctx context.Context) (int64, error)

	// GetTotalStatusCount returns the total number of statuses
	GetTotalStatusCount(ctx context.Context) (int64, error)

	// GetTotalDomainCount returns the total number of known domains
	GetTotalDomainCount(ctx context.Context) (int64, error)

	// GetActiveUserCount returns the number of active users in the last N days
	GetActiveUserCount(ctx context.Context, days int) (int64, error)

	// GetDailyActiveUserCount returns the number of daily active users
	GetDailyActiveUserCount(ctx context.Context) (int64, error)

	// GetLocalPostCount returns the number of local posts
	GetLocalPostCount(ctx context.Context) (int64, error)

	// GetLocalCommentCount returns the number of local comments
	GetLocalCommentCount(ctx context.Context) (int64, error)

	// Instance activity operations

	// GetWeeklyActivity retrieves weekly activity data for a specific week
	GetWeeklyActivity(ctx context.Context, weekTimestamp int64) (*storage.WeeklyActivity, error)

	// RecordActivity records activity data for analytics
	RecordActivity(ctx context.Context, activityType string, userID string, timestamp time.Time) error

	// Instance contact operations

	// GetContactAccount returns the contact account for the instance
	GetContactAccount(ctx context.Context) (*storage.ActorRecord, error)

	// Storage and metrics history operations

	// GetStorageUsage returns current storage usage statistics
	GetStorageUsage(ctx context.Context) (any, error)

	// GetStorageHistory returns storage usage history for the last N days
	GetStorageHistory(ctx context.Context, days int) ([]any, error)

	// GetUserGrowthHistory returns user growth data for the last N days
	GetUserGrowthHistory(ctx context.Context, days int) ([]any, error)

	// GetDomainStats returns statistics for a specific domain
	GetDomainStats(ctx context.Context, domain string) (any, error)

	// RecordDailyMetrics records daily historical metrics for the instance
	RecordDailyMetrics(ctx context.Context, date string, metrics map[string]interface{}) error

	// GetMetricsSummary returns aggregated metrics for a given time range
	GetMetricsSummary(ctx context.Context, timeRange string) (map[string]interface{}, error)
}
