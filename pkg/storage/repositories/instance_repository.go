package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
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
		Where("PK", "=", storage.InstanceConfigKey).
		Where("SK", "=", "RULES").
		First(&config)

	if err != nil {
		if errors.IsNotFound(err) {
			// Return default instance rules if none configured
			return r.getDefaultInstanceRules(), nil
		}
		r.logger.Error("Failed to get instance rules", zap.Error(err))
		return nil, fmt.Errorf("failed to get instance rules: %w", err)
	}

	// Deserialize JSON rules with validation
	if err := common.ValidateRequiredParam("rules_json", config.RulesJSON); err != nil {
		return r.getDefaultInstanceRules(), nil
	}

	var result []storage.InstanceRule
	if err := json.Unmarshal([]byte(config.RulesJSON), &result); err != nil {
		r.logger.Error("Failed to unmarshal instance rules, falling back to defaults", zap.Error(err))
		return r.getDefaultInstanceRules(), nil
	}

	// Validate and filter rules
	validatedRules := r.validateAndFilterRules(result)
	r.logger.Debug("Retrieved instance rules", zap.Int("count", len(validatedRules)))

	return validatedRules, nil
}

// SetInstanceRules updates the instance rules
// Matches legacy: assigns IDs if not present, PK="INSTANCE#CONFIG", SK="RULES"
func (r *InstanceRepository) SetInstanceRules(ctx context.Context, rules []storage.InstanceRule) error {
	// Assign IDs if not present (matches legacy behavior)
	processedRules := make([]storage.InstanceRule, len(rules))
	for i, rule := range rules {
		processedRules[i] = rule
		if err := common.ValidateRequiredParam("rule_id", processedRules[i].ID); err != nil {
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
		Where("PK", "=", storage.InstanceConfigKey).
		Where("SK", "=", "EXTENDED_DESC").
		First(&config)

	if err != nil {
		if errors.IsNotFound(err) {
			// Return enhanced default description with instance info
			defaultDesc := r.generateDefaultDescription()
			return defaultDesc, time.Now(), nil
		}
		r.logger.Error("Failed to get extended description", zap.Error(err))
		return "", time.Time{}, fmt.Errorf("failed to get extended description: %w", err)
	}

	// Validate and sanitize the description
	sanitizedDesc := r.sanitizeDescription(config.ExtendedDescription)
	return sanitizedDesc, config.UpdatedAt, nil
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

	// Filter by category using rule text patterns and metadata
	filtered := make([]storage.InstanceRule, 0)
	for _, rule := range rules {
		if r.ruleMatchesCategory(rule, category) {
			filtered = append(filtered, rule)
		}
	}

	// If no rules match specific category, apply smart categorization
	if len(filtered) == 0 && category != "" {
		filtered = r.categorizeRulesSmartly(rules, category)
	}

	r.logger.Debug("Filtered rules by category",
		zap.String("category", category),
		zap.Int("total_rules", len(rules)),
		zap.Int("filtered_count", len(filtered)))

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

// GetLocalCommentCount returns the number of local comments (posts with InReplyToID)
func (r *InstanceRepository) GetLocalCommentCount(ctx context.Context) (int64, error) {
	var metric models.InstanceMetrics
	err := r.db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", "INSTANCE#METRICS").
		Where("SK", "=", "LOCAL_COMMENTS").
		First(&metric)

	if err != nil {
		if errors.IsNotFound(err) {
			// If metric doesn't exist, we need to count directly from statuses
			// This is more expensive but necessary for backwards compatibility
			return r.countLocalComments(ctx)
		}
		r.logger.Error("Failed to get local comment count", zap.Error(err))
		return 0, fmt.Errorf("failed to get local comment count: %w", err)
	}

	return metric.Value, nil
}

// countLocalComments counts local comments by using the replies GSI for efficient counting
func (r *InstanceRepository) countLocalComments(ctx context.Context) (int64, error) {
	// Use GSI4 (replies-index) to efficiently count comments
	// Comments are statuses with InReplyToID set, which populate GSI4PK with "REPLIES#<parent_id>"
	// We need to scan the GSI4 to count all entries, but this is more efficient than scanning the main table
	
	var comments []models.Status
	
	// Query the replies-index GSI for all comments
	// Since we only need the count, we'll use a projection that minimizes data transfer
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("replies-index").
		Where("GSI4PK", "begins_with", "REPLIES#").
		All(&comments)
	
	if err != nil {
		r.logger.Error("Failed to count local comments using GSI", zap.Error(err))
		// Fall back to returning 0 - the metric will be populated over time via real-time tracking
		r.logger.Warn("Failed to batch count comments, falling back to metric tracking over time")
		return 0, nil
	}
	
	// Filter for local comments (comments from local users)
	localDomain := os.Getenv("DOMAIN_NAME")
	if err := common.ValidateRequiredParam("local_domain", localDomain); err != nil {
		// If no domain configured, count all comments
		r.logger.Debug("No DOMAIN_NAME set, counting all comments as local")
		return int64(len(comments)), nil
	}
	
	localCount := int64(0)
	for _, comment := range comments {
		// Check if the comment author is from the local domain
		if strings.Contains(comment.AuthorID, localDomain) && !comment.Deleted {
			localCount++
		}
	}
	
	r.logger.Info("Successfully counted local comments using batch GSI query",
		zap.Int64("local_comments", localCount),
		zap.Int("total_comments_scanned", len(comments)))
	
	return localCount, nil
}

// GetWeeklyActivity retrieves weekly activity data for a specific week
func (r *InstanceRepository) GetWeeklyActivity(ctx context.Context, weekTimestamp int64) (*storage.WeeklyActivity, error) {
	var activity models.WeeklyActivity

	// Use the pattern from the model: PK="INSTANCE#ACTIVITY", SK="ACTIVITY#WEEK#{date}"
	weekStart := time.Unix(weekTimestamp, 0).Format(common.DateFormat)
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
		Week:          fmt.Sprintf("%d", activity.Week),
		Statuses:      int(activity.Statuses),
		Logins:        int(activity.Logins),
		Registrations: int(activity.Registrations),
	}, nil
}

// RecordActivity records activity data for analytics
func (r *InstanceRepository) RecordActivity(ctx context.Context, activityType string, _ string, timestamp time.Time) error {
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

	if err := common.ValidateSliceNotEmpty("users", users); err != nil {
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

// getMetricHistory is a consolidated helper that retrieves history for different metric types
func (r *InstanceRepository) getMetricHistory(ctx context.Context, days int, metricType, operation string, formatter func(models.InstanceHistory) map[string]interface{}) ([]any, error) {
	// Direct implementation since we don't have BaseRepository embedded
	if err := common.ValidateIntRange("days", days, 1, 365); err != nil {
		days = 30 // Default to 30 days
	}

	// Calculate date range
	startDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	endDate := time.Now().Format("2006-01-02")

	// Query daily metrics using GSI1
	var histories []models.InstanceHistory
	err := r.db.WithContext(ctx).Model(&models.InstanceHistory{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("METRIC#%s", metricType)).
		Where("GSI1SK", ">=", fmt.Sprintf("DATE#%s", startDate)).
		Where("GSI1SK", "<=", fmt.Sprintf("DATE#%s", endDate)).
		All(&histories)

	if err != nil {
		r.logger.Error(fmt.Sprintf("Failed to get %s", operation), zap.Error(err), zap.Int("days", days))
		return nil, fmt.Errorf("failed to get %s: %w", operation, err)
	}

	// Convert to expected format using the provided formatter
	result := make([]any, len(histories))
	for i, h := range histories {
		result[i] = formatter(h)
	}

	r.logger.Info(fmt.Sprintf("Retrieved %s", operation), zap.Int("days", days), zap.Int("records", len(result)))
	return result, nil
}

// GetStorageHistory returns storage usage history for the last N days
func (r *InstanceRepository) GetStorageHistory(ctx context.Context, days int) ([]any, error) {
	return r.getMetricHistory(ctx, days, "storage_bytes", "storage history", func(h models.InstanceHistory) map[string]interface{} {
		return map[string]interface{}{
			"date":           h.Date,
			"total_bytes":    h.StorageBytes,
			"media_bytes":    h.MediaBytes,
			"database_bytes": h.DatabaseBytes,
			"delta":          h.Delta,
			"recorded_at":    h.RecordedAt,
		}
	})
}

// GetUserGrowthHistory returns user growth data for the last N days
func (r *InstanceRepository) GetUserGrowthHistory(ctx context.Context, days int) ([]any, error) {
	return r.getMetricHistory(ctx, days, "user_count", "user growth history", func(h models.InstanceHistory) map[string]interface{} {
		return map[string]interface{}{
			"date":         h.Date,
			"total_users":  h.TotalUsers,
			"active_users": h.ActiveUsers,
			"new_users":    h.NewUsers,
			"delta":        h.Delta,
			"recorded_at":  h.RecordedAt,
		}
	})
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

// RecordDailyMetrics records daily historical metrics for the instance
func (r *InstanceRepository) RecordDailyMetrics(ctx context.Context, date string, metrics map[string]interface{}) error {
	now := time.Now()
	if err := common.ValidateRequiredParam("date", date); err != nil {
		date = now.Format("2006-01-02")
	}

	// Record user metrics
	if userCount, ok := metrics["total_users"].(int64); ok {
		userHistory := models.NewDailyInstanceHistory(date, "user_count")
		if activeUsers, hasActive := metrics["active_users"].(int64); hasActive {
			if newUsers, hasNew := metrics["new_users"].(int64); hasNew {
				userHistory.SetUserMetrics(userCount, activeUsers, newUsers)
			} else {
				userHistory.SetUserMetrics(userCount, activeUsers, 0)
			}
		} else {
			userHistory.SetUserMetrics(userCount, 0, 0)
		}

		// Get previous day's value for delta calculation
		if prevValue, err := r.getPreviousDayValue(ctx, date, "user_count"); err == nil {
			userHistory.CalculateDelta(prevValue)
		}

		if err := r.db.WithContext(ctx).Model(userHistory).Create(); err != nil {
			r.logger.Error("Failed to record daily user metrics", zap.Error(err), zap.String("date", date))
			return fmt.Errorf("failed to record user metrics: %w", err)
		}
	}

	// Record storage metrics
	if storageBytes, ok := metrics["storage_bytes"].(int64); ok {
		storageHistory := models.NewDailyInstanceHistory(date, "storage_bytes")
		mediaBytes, _ := metrics["media_bytes"].(int64)
		dbBytes, _ := metrics["database_bytes"].(int64)
		storageHistory.SetStorageMetrics(storageBytes, mediaBytes, dbBytes)

		// Get previous day's value for delta calculation
		if prevValue, err := r.getPreviousDayValue(ctx, date, "storage_bytes"); err == nil {
			storageHistory.CalculateDelta(prevValue)
		}

		if err := r.db.WithContext(ctx).Model(storageHistory).Create(); err != nil {
			r.logger.Error("Failed to record daily storage metrics", zap.Error(err), zap.String("date", date))
			return fmt.Errorf("failed to record storage metrics: %w", err)
		}
	}

	// Record post metrics
	if postCount, ok := metrics["total_posts"].(int64); ok {
		postHistory := models.NewDailyInstanceHistory(date, "post_count")
		newPosts, _ := metrics["new_posts"].(int64)
		localPosts, _ := metrics["local_posts"].(int64)
		federatedPosts, _ := metrics["federated_posts"].(int64)
		postHistory.SetPostMetrics(postCount, newPosts, localPosts, federatedPosts)

		// Get previous day's value for delta calculation
		if prevValue, err := r.getPreviousDayValue(ctx, date, "post_count"); err == nil {
			postHistory.CalculateDelta(prevValue)
		}

		if err := r.db.WithContext(ctx).Model(postHistory).Create(); err != nil {
			r.logger.Error("Failed to record daily post metrics", zap.Error(err), zap.String("date", date))
			return fmt.Errorf("failed to record post metrics: %w", err)
		}
	}

	// Record federation metrics
	if knownInstances, ok := metrics["known_instances"].(int64); ok {
		fedHistory := models.NewDailyInstanceHistory(date, "federation_count")
		activeInstances, _ := metrics["active_instances"].(int64)
		fedHistory.SetFederationMetrics(knownInstances, activeInstances)

		// Get previous day's value for delta calculation
		if prevValue, err := r.getPreviousDayValue(ctx, date, "federation_count"); err == nil {
			fedHistory.CalculateDelta(prevValue)
		}

		if err := r.db.WithContext(ctx).Model(fedHistory).Create(); err != nil {
			r.logger.Error("Failed to record daily federation metrics", zap.Error(err), zap.String("date", date))
			return fmt.Errorf("failed to record federation metrics: %w", err)
		}
	}

	r.logger.Info("Successfully recorded daily metrics", zap.String("date", date))
	return nil
}

// GetMetricsSummary returns aggregated metrics for a given time range
func (r *InstanceRepository) GetMetricsSummary(ctx context.Context, timeRange string) (map[string]interface{}, error) {
	var startDate, endDate string
	now := time.Now()

	switch timeRange {
	case "week":
		startDate = now.AddDate(0, 0, -7).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	case "month":
		startDate = now.AddDate(0, -1, 0).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	case "quarter":
		startDate = now.AddDate(0, -3, 0).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	case "year":
		startDate = now.AddDate(-1, 0, 0).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	default:
		startDate = now.AddDate(0, 0, -30).Format("2006-01-02")
		endDate = now.Format("2006-01-02")
	}

	summary := make(map[string]interface{})

	// Get metrics for each type
	metricTypes := []string{"user_count", "storage_bytes", "post_count", "federation_count"}

	for _, metricType := range metricTypes {
		var histories []models.InstanceHistory
		err := r.db.WithContext(ctx).Model(&models.InstanceHistory{}).
			Index("GSI1").
			Where("GSI1PK", "=", fmt.Sprintf("METRIC#%s", metricType)).
			Where("GSI1SK", ">=", fmt.Sprintf("DATE#%s", startDate)).
			Where("GSI1SK", "<=", fmt.Sprintf("DATE#%s", endDate)).
			All(&histories)

		if err != nil {
			r.logger.Error("Failed to get metrics summary", zap.Error(err), zap.String("metric_type", metricType))
			continue
		}

		if err := common.ValidateSliceNotEmpty("histories", histories); err == nil {
			// Get latest and earliest values for growth calculation
			latest := histories[len(histories)-1]
			earliest := histories[0]
			growth := float64(0)
			if earliest.Value > 0 {
				growth = ((float64(latest.Value) - float64(earliest.Value)) / float64(earliest.Value)) * 100
			}

			summary[metricType] = map[string]interface{}{
				"current_value": latest.Value,
				"start_value":   earliest.Value,
				"growth_pct":    growth,
				"total_change":  latest.Value - earliest.Value,
				"data_points":   len(histories),
			}
		}
	}

	summary["time_range"] = timeRange
	summary["start_date"] = startDate
	summary["end_date"] = endDate
	summary["generated_at"] = time.Now()

	return summary, nil
}

// getPreviousDayValue gets the value from the previous day for delta calculation
func (r *InstanceRepository) getPreviousDayValue(ctx context.Context, currentDate, metricType string) (int64, error) {
	// Parse current date and get previous day
	date, err := time.Parse("2006-01-02", currentDate)
	if err != nil {
		return 0, err
	}
	prevDate := date.AddDate(0, 0, -1).Format("2006-01-02")

	var history models.InstanceHistory
	err = r.db.WithContext(ctx).Model(&models.InstanceHistory{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("METRIC#%s", metricType)).
		Where("GSI1SK", "=", fmt.Sprintf("DATE#%s", prevDate)).
		First(&history)

	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil // No previous data, delta from 0
		}
		return 0, err
	}

	return history.Value, nil
}

// Helper function to get the start of the week for a given timestamp
func getWeekStart(t time.Time) time.Time {
	// Get Monday of the week
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday = 7
	}
	return t.AddDate(0, 0, -(weekday - 1)).Truncate(24 * time.Hour)
}

// getDefaultInstanceRules returns a set of default rules when none are configured
func (r *InstanceRepository) getDefaultInstanceRules() []storage.InstanceRule {
	return []storage.InstanceRule{
		{
			ID:   "1",
			Text: "Be respectful and kind to other users",
		},
		{
			ID:   "2",
			Text: "No harassment, hate speech, or discrimination",
		},
		{
			ID:   "3",
			Text: "No spam or excessive promotional content",
		},
		{
			ID:   "4",
			Text: "Use appropriate content warnings for sensitive material",
		},
		{
			ID:   "5",
			Text: "Follow local and international laws",
		},
	}
}

// validateAndFilterRules validates rules and removes invalid ones
func (r *InstanceRepository) validateAndFilterRules(rules []storage.InstanceRule) []storage.InstanceRule {
	validated := make([]storage.InstanceRule, 0, len(rules))
	seenIDs := make(map[string]bool)

	for i, rule := range rules {
		// Ensure rule has an ID
		if err := common.ValidateRequiredParam("rule_id", rule.ID); err != nil {
			rule.ID = fmt.Sprintf("rule_%d", i+1)
		}

		// Check for duplicate IDs
		if seenIDs[rule.ID] {
			// Generate unique ID for duplicates
			rule.ID = fmt.Sprintf("%s_dup_%d", rule.ID, i)
		}
		seenIDs[rule.ID] = true

		// Validate rule text
		if err := common.ValidateRequiredParam("rule_text", strings.TrimSpace(rule.Text)); err != nil {
			r.logger.Warn("Skipping rule with empty text", zap.String("id", rule.ID))
			continue
		}

		// Limit text length
		if len(rule.Text) > 500 {
			rule.Text = rule.Text[:497] + "..."
		}

		validated = append(validated, rule)
	}

	return validated
}

// generateDefaultDescription creates a dynamic default description
func (r *InstanceRepository) generateDefaultDescription() string {
	return fmt.Sprintf(`<div class="instance-description">
		<h2>Welcome to Lesser</h2>
		<p>This is a Lesser ActivityPub server, part of the decentralized social web.</p>
		
		<h3>About Lesser</h3>
		<p>Lesser is a lightweight, cost-effective ActivityPub implementation designed for 
		personal and small community use. It provides full compatibility with Mastodon and 
		other fediverse applications while maintaining minimal operational costs.</p>
		
		<h3>Features</h3>
		<ul>
			<li>Full Mastodon API compatibility</li>
			<li>ActivityPub federation</li>
			<li>WebSocket streaming</li>
			<li>GraphQL API</li>
			<li>Cost-optimized serverless architecture</li>
		</ul>
		
		<p><em>Generated on %s</em></p>
	</div>`, time.Now().Format("2006-01-02"))
}

// sanitizeDescription sanitizes HTML content in descriptions
func (r *InstanceRepository) sanitizeDescription(desc string) string {
	// Basic HTML sanitization - in production, use a proper HTML sanitizer
	desc = strings.ReplaceAll(desc, "<script", "&lt;script")
	desc = strings.ReplaceAll(desc, "</script>", "&lt;/script&gt;")
	desc = strings.ReplaceAll(desc, "javascript:", "")
	desc = strings.ReplaceAll(desc, "on=", "data-on=")

	// Limit length
	if err := common.ValidateStringLength("description", desc, 0, 10000); err != nil {
		desc = desc[:9997] + "..."
	}

	return desc
}

// ruleMatchesCategory checks if a rule matches a given category
func (r *InstanceRepository) ruleMatchesCategory(rule storage.InstanceRule, category string) bool {
	if err := common.ValidateRequiredParam("category", category); err != nil {
		return true // Return all rules for empty category
	}

	ruleTextLower := strings.ToLower(rule.Text)
	categoryLower := strings.ToLower(category)

	// Define category keywords
	categoryKeywords := map[string][]string{
		"harassment": {"harassment", "abuse", "bullying", "threatening", "intimidation"},
		"spam":       {"spam", "promotional", "advertising", "solicitation", "flooding"},
		"content":    {"content warning", "nsfw", "sensitive", "explicit", "graphic"},
		"legal":      {"illegal", "law", "legal", "copyright", "dmca", "piracy"},
		"conduct":    {"respectful", "kind", "civil", "behavior", "conduct", "etiquette"},
		"hate":       {"hate speech", "discrimination", "racism", "sexism", "homophobia"},
		"privacy":    {"privacy", "personal info", "doxxing", "private", "confidential"},
	}

	keywords, exists := categoryKeywords[categoryLower]
	if !exists {
		// Try partial matching for unknown categories
		return strings.Contains(ruleTextLower, categoryLower)
	}

	// Check if any keywords match
	for _, keyword := range keywords {
		if strings.Contains(ruleTextLower, keyword) {
			return true
		}
	}

	return false
}

// categorizeRulesSmartly applies intelligent categorization when no direct matches
func (r *InstanceRepository) categorizeRulesSmartly(rules []storage.InstanceRule, category string) []storage.InstanceRule {
	// If requesting a specific category but no matches, apply fuzzy logic
	filtered := make([]storage.InstanceRule, 0)
	categoryLower := strings.ToLower(category)

	// For unknown categories, do fuzzy text matching
	for _, rule := range rules {
		ruleTextLower := strings.ToLower(rule.Text)

		// Calculate similarity score (simple implementation)
		if r.calculateSimilarity(ruleTextLower, categoryLower) > 0.3 {
			filtered = append(filtered, rule)
		}
	}

	// If still no matches, return most relevant rules based on common sense
	if err := common.ValidateSliceNotEmpty("filtered", filtered); err != nil {
		switch categoryLower {
		case "safety", "security":
			// Return rules about harassment, abuse, etc.
			for _, rule := range rules {
				if r.ruleMatchesCategory(rule, "harassment") || r.ruleMatchesCategory(rule, "hate") {
					filtered = append(filtered, rule)
				}
			}
		case "posting", "content":
			// Return rules about content guidelines
			for _, rule := range rules {
				if r.ruleMatchesCategory(rule, "content") || r.ruleMatchesCategory(rule, "spam") {
					filtered = append(filtered, rule)
				}
			}
		default:
			// Return top 3 most important rules
			if err := common.ValidateSliceLength("rules", rules, 3); err != nil {
				filtered = rules[:3]
			} else {
				filtered = rules
			}
		}
	}

	return filtered
}

// calculateSimilarity calculates a simple text similarity score (0.0 to 1.0)
func (r *InstanceRepository) calculateSimilarity(text1, text2 string) float64 {
	words1 := strings.Fields(text1)
	words2 := strings.Fields(text2)

	if len(words1) == 0 || len(words2) == 0 {
		return 0.0
	}

	matches := 0
	for _, word1 := range words1 {
		for _, word2 := range words2 {
			if strings.Contains(word1, word2) || strings.Contains(word2, word1) {
				matches++
				break
			}
		}
	}

	// Return ratio of matching words to total unique words
	totalWords := len(words1) + len(words2)
	return float64(matches*2) / float64(totalWords)
}
