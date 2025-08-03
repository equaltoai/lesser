package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// InstanceRepository implements instance operations using DynamORM
type InstanceRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewInstanceRepository creates a new instance repository
func NewInstanceRepository(db core.DB, tableName string, logger *zap.Logger) *InstanceRepository {
	return &InstanceRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// GetInstanceRules retrieves the instance rules
// Matches legacy: PK="INSTANCE#CONFIG", SK="RULES"
func (r *InstanceRepository) GetInstanceRules(ctx context.Context) ([]storage.InstanceRule, error) {
	var config models.InstanceConfig
	err := r.db.WithContext(ctx).Model(&models.InstanceConfig{}).
		Where("PK", "=", "INSTANCE#CONFIG").
		Where("SK", "=", "RULES").
		First(&config)

	if err != nil {
		if errors.IsNotFound(err) {
			// Return empty rules if not set (matches legacy behavior)
			return []storage.InstanceRule{}, nil
		}
		r.logger.Error("Failed to get instance rules", zap.Error(err))
		return nil, fmt.Errorf("failed to get instance rules: %w", err)
	}

	// Deserialize JSON rules
	if config.RulesJSON == "" {
		return []storage.InstanceRule{}, nil
	}

	var result []storage.InstanceRule
	if err := json.Unmarshal([]byte(config.RulesJSON), &result); err != nil {
		r.logger.Error("Failed to unmarshal instance rules", zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal instance rules: %w", err)
	}

	return result, nil
}

// SetInstanceRules updates the instance rules
// Matches legacy: assigns IDs if not present, PK="INSTANCE#CONFIG", SK="RULES"
func (r *InstanceRepository) SetInstanceRules(ctx context.Context, rules []storage.InstanceRule) error {
	// Assign IDs if not present (matches legacy behavior)
	processedRules := make([]storage.InstanceRule, len(rules))
	for i, rule := range rules {
		processedRules[i] = rule
		if processedRules[i].ID == "" {
			processedRules[i].ID = fmt.Sprintf("%d", i+1)
		}
	}

	// Serialize rules to JSON
	rulesJSON, err := json.Marshal(processedRules)
	if err != nil {
		r.logger.Error("Failed to marshal instance rules", zap.Error(err))
		return fmt.Errorf("failed to marshal instance rules: %w", err)
	}

	config := models.NewInstanceRulesConfig(string(rulesJSON))

	err = r.db.WithContext(ctx).Model(config).Create()
	if err != nil {
		r.logger.Error("Failed to save instance rules", zap.Error(err))
		return fmt.Errorf("failed to save instance rules: %w", err)
	}

	return nil
}

// GetExtendedDescription retrieves the instance extended description
// Matches legacy: PK="INSTANCE#CONFIG", SK="EXTENDED_DESC", returns default if not set
func (r *InstanceRepository) GetExtendedDescription(ctx context.Context) (string, time.Time, error) {
	var config models.InstanceConfig
	err := r.db.WithContext(ctx).Model(&models.InstanceConfig{}).
		Where("PK", "=", "INSTANCE#CONFIG").
		Where("SK", "=", "EXTENDED_DESC").
		First(&config)

	if err != nil {
		if errors.IsNotFound(err) {
			// Return default if not set (matches legacy behavior)
			return "<p>Welcome to Lesser ActivityPub Server</p>", time.Now(), nil
		}
		r.logger.Error("Failed to get extended description", zap.Error(err))
		return "", time.Time{}, fmt.Errorf("failed to get extended description: %w", err)
	}

	return config.ExtendedDescription, config.UpdatedAt, nil
}

// SetExtendedDescription updates the instance extended description
// Matches legacy: PK="INSTANCE#CONFIG", SK="EXTENDED_DESC"
func (r *InstanceRepository) SetExtendedDescription(ctx context.Context, description string) error {
	config := models.NewExtendedDescriptionConfig(description)

	err := r.db.WithContext(ctx).Model(config).Create()
	if err != nil {
		r.logger.Error("Failed to save extended description", zap.Error(err))
		return fmt.Errorf("failed to save extended description: %w", err)
	}

	return nil
}

// GetRulesByCategory retrieves rules filtered by category
// Since legacy doesn't implement this, we'll use the instance rules model with category filtering
func (r *InstanceRepository) GetRulesByCategory(ctx context.Context, category string) ([]storage.InstanceRule, error) {
	rules, err := r.GetInstanceRules(ctx)
	if err != nil {
		return nil, err
	}

	// Filter by category
	var filtered []storage.InstanceRule
	for _, rule := range rules {
		// Since storage.InstanceRule doesn't have Category field, we'll need to work with what we have
		// For now, return all rules - this method may need interface updates
		filtered = append(filtered, rule)
	}

	return filtered, nil
}

// GetTotalUserCount returns the total number of users
// Since legacy doesn't implement this, use instance metrics pattern
func (r *InstanceRepository) GetTotalUserCount(ctx context.Context) (int64, error) {
	var metric models.InstanceMetrics
	err := r.db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", "INSTANCE#METRICS").
		Where("SK", "=", "TOTAL_USERS").
		First(&metric)

	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil
		}
		r.logger.Error("Failed to get total user count", zap.Error(err))
		return 0, fmt.Errorf("failed to get total user count: %w", err)
	}

	return metric.TotalUsers, nil
}

// GetTotalStatusCount returns the total number of statuses
func (r *InstanceRepository) GetTotalStatusCount(ctx context.Context) (int64, error) {
	var metric models.InstanceMetrics
	err := r.db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", "INSTANCE#METRICS").
		Where("SK", "=", "TOTAL_STATUSES").
		First(&metric)

	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil
		}
		r.logger.Error("Failed to get total status count", zap.Error(err))
		return 0, fmt.Errorf("failed to get total status count: %w", err)
	}

	return metric.TotalStatuses, nil
}

// GetTotalDomainCount returns the total number of known domains
func (r *InstanceRepository) GetTotalDomainCount(ctx context.Context) (int64, error) {
	var metric models.InstanceMetrics
	err := r.db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", "INSTANCE#METRICS").
		Where("SK", "=", "TOTAL_DOMAINS").
		First(&metric)

	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil
		}
		r.logger.Error("Failed to get total domain count", zap.Error(err))
		return 0, fmt.Errorf("failed to get total domain count: %w", err)
	}

	return metric.Value, nil
}

// GetActiveUserCount returns the number of active users in the last N days
func (r *InstanceRepository) GetActiveUserCount(ctx context.Context, days int) (int64, error) {
	metricType := fmt.Sprintf("ACTIVE_USERS_%dD", days)
	var metric models.InstanceMetrics
	err := r.db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", "INSTANCE#METRICS").
		Where("SK", "=", metricType).
		First(&metric)

	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil
		}
		r.logger.Error("Failed to get active user count", zap.Error(err), zap.Int("days", days))
		return 0, fmt.Errorf("failed to get active user count: %w", err)
	}

	return metric.Value, nil
}

// GetDailyActiveUserCount returns the number of daily active users
func (r *InstanceRepository) GetDailyActiveUserCount(ctx context.Context) (int64, error) {
	return r.GetActiveUserCount(ctx, 1)
}

// GetLocalPostCount returns the number of local posts
func (r *InstanceRepository) GetLocalPostCount(ctx context.Context) (int64, error) {
	var metric models.InstanceMetrics
	err := r.db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", "INSTANCE#METRICS").
		Where("SK", "=", "LOCAL_POSTS").
		First(&metric)

	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil
		}
		r.logger.Error("Failed to get local post count", zap.Error(err))
		return 0, fmt.Errorf("failed to get local post count: %w", err)
	}

	return metric.Value, nil
}

// GetWeeklyActivity retrieves weekly activity data for a specific week
func (r *InstanceRepository) GetWeeklyActivity(ctx context.Context, weekTimestamp int64) (*storage.WeeklyActivity, error) {
	var activity models.WeeklyActivity
	
	// Use the pattern from the model: PK="INSTANCE#ACTIVITY", SK="ACTIVITY#WEEK#{date}"
	weekStart := time.Unix(weekTimestamp, 0).Format("2006-01-02")
	err := r.db.WithContext(ctx).Model(&models.WeeklyActivity{}).
		Where("PK", "=", "INSTANCE#ACTIVITY").
		Where("SK", "=", fmt.Sprintf("ACTIVITY#WEEK#%s", weekStart)).
		First(&activity)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		r.logger.Error("Failed to get weekly activity", zap.Error(err), zap.Int64("week", weekTimestamp))
		return nil, fmt.Errorf("failed to get weekly activity: %w", err)
	}

	return &storage.WeeklyActivity{
		Week:          activity.Week,
		Statuses:      activity.Statuses,
		Logins:        activity.Logins,
		Registrations: activity.Registrations,
	}, nil
}

// RecordActivity records activity data for analytics
func (r *InstanceRepository) RecordActivity(ctx context.Context, activityType string, actorID string, timestamp time.Time) error {
	// Create activity record for instance-wide tracking
	week := getWeekStart(timestamp)
	
	// Create a new weekly activity using the model's constructor
	activity := models.NewWeeklyActivity(week)
	
	// Try to get existing record first
	err := r.db.WithContext(ctx).Model(&models.WeeklyActivity{}).
		Where("PK", "=", activity.PK).
		Where("SK", "=", activity.SK).
		First(activity)

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("Failed to get existing weekly activity", zap.Error(err))
		return fmt.Errorf("failed to get existing weekly activity: %w", err)
	}

	// Update activity counters based on type
	switch strings.ToLower(activityType) {
	case "status", "post":
		activity.IncrementStatuses(1)
	case "login":
		activity.IncrementLogins(1)
	case "registration", "signup":
		activity.IncrementRegistrations(1)
	}

	// Save the updated activity
	err = r.db.WithContext(ctx).Model(activity).Create()
	if err != nil {
		r.logger.Error("Failed to record activity", zap.Error(err), zap.String("type", activityType))
		return fmt.Errorf("failed to record activity: %w", err)
	}

	return nil
}

// GetContactAccount returns the contact account for the instance
// This returns the first admin user as the contact account
func (r *InstanceRepository) GetContactAccount(ctx context.Context) (*storage.ActorRecord, error) {
	// Look for the first admin user to serve as contact account
	var users []models.User
	err := r.db.WithContext(ctx).Model(&models.User{}).
		Index("role-index").
		Where("GSI3PK", "=", "ROLE#admin").
		Limit(1).
		All(&users)

	if err != nil {
		r.logger.Error("Failed to query admin users for contact account", zap.Error(err))
		return nil, fmt.Errorf("failed to query admin users: %w", err)
	}

	if len(users) == 0 {
		// No admin users found, return nil (no contact account)
		return nil, nil
	}

	user := users[0]

	// Get the corresponding actor for this user
	var actor models.Actor
	err = r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", fmt.Sprintf("ACTOR#%s", user.Username)).
		Where("SK", "=", "PROFILE").
		First(&actor)

	if err != nil {
		if errors.IsNotFound(err) {
			// Admin user exists but no actor profile - this is unusual but not an error
			return nil, nil
		}
		r.logger.Error("Failed to get actor for contact account", 
			zap.String("username", user.Username), 
			zap.Error(err))
		return nil, fmt.Errorf("failed to get actor for contact user: %w", err)
	}

	// Convert the actor model to storage.ActorRecord format
	actorRecord := &storage.ActorRecord{
		PK:       actor.PK,
		SK:       actor.SK,
		Actor:    actor.Actor,
		Username: actor.Username,
		// PrivateKey is not included for security reasons when returning contact info
	}

	return actorRecord, nil
}

// GetStorageUsage returns current storage usage statistics
func (r *InstanceRepository) GetStorageUsage(ctx context.Context) (any, error) {
	var metric models.InstanceMetrics
	err := r.db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", "INSTANCE#METRICS").
		Where("SK", "=", "STORAGE_USAGE").
		First(&metric)

	if err != nil {
		if errors.IsNotFound(err) {
			return map[string]interface{}{
				"total_bytes": 0,
				"media_bytes": 0,
				"db_bytes":    0,
			}, nil
		}
		r.logger.Error("Failed to get storage usage", zap.Error(err))
		return nil, fmt.Errorf("failed to get storage usage: %w", err)
	}

	return map[string]interface{}{
		"total_bytes": metric.Value,
		"updated_at":  metric.UpdatedAt,
	}, nil
}

// GetStorageHistory returns storage usage history for the last N days
func (r *InstanceRepository) GetStorageHistory(ctx context.Context, days int) ([]any, error) {
	// TODO: Implement proper query once DynamORM query patterns are established
	// For now, return empty history
	r.logger.Info("GetStorageHistory called but not fully implemented", zap.Int("days", days))
	return []any{}, nil
}

// GetUserGrowthHistory returns user growth data for the last N days
func (r *InstanceRepository) GetUserGrowthHistory(ctx context.Context, days int) ([]any, error) {
	// TODO: Implement proper query once DynamORM query patterns are established
	// For now, return empty history
	r.logger.Info("GetUserGrowthHistory called but not fully implemented", zap.Int("days", days))
	return []any{}, nil
}

// GetDomainStats returns statistics for a specific domain
func (r *InstanceRepository) GetDomainStats(ctx context.Context, domain string) (any, error) {
	var metric models.InstanceMetrics
	err := r.db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", fmt.Sprintf("DOMAIN#%s", domain)).
		Where("SK", "=", "STATS").
		First(&metric)

	if err != nil {
		if errors.IsNotFound(err) {
			return map[string]interface{}{
				"domain":        domain,
				"actor_count":   0,
				"status_count":  0,
				"last_activity": nil,
			}, nil
		}
		r.logger.Error("Failed to get domain stats", zap.Error(err), zap.String("domain", domain))
		return nil, fmt.Errorf("failed to get domain stats: %w", err)
	}

	return map[string]interface{}{
		"domain":        domain,
		"actor_count":   metric.Value,
		"last_activity": metric.UpdatedAt,
	}, nil
}

// Helper function to get the start of the week for a given timestamp
func getWeekStart(t time.Time) time.Time {
	// Get Monday of the week
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday = 7
	}
	return t.AddDate(0, 0, -(weekday-1)).Truncate(24 * time.Hour)
}