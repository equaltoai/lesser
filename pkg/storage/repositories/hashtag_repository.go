package repositories

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/batch"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// HashtagRepository implements hashtag-related database operations using DynamORM
type HashtagRepository struct {
	db                core.DB
	tableName         string
	logger            *zap.Logger
	domain            string
	trendingCalculator *TrendingCalculator
}

// TrendingCalculatorConfig holds configuration for trending algorithm
type TrendingCalculatorConfig struct {
	// Time decay parameters
	DecayHalfLife          time.Duration // How quickly scores decay
	MinimumAge             time.Duration // Minimum age before considering trending
	MaximumAge             time.Duration // Maximum age for trending consideration
	
	// Scoring weights
	UsageWeight            float64 // Weight for usage count
	EngagementWeight       float64 // Weight for engagement metrics
	DiversityWeight        float64 // Weight for user diversity
	TrustWeight            float64 // Weight for trust scores
	MomentumWeight         float64 // Weight for trending momentum
	
	// Thresholds
	MinimumUsage           int64   // Minimum usage count to consider
	MinimumUsers           int64   // Minimum unique users to consider
	TrendingThreshold      float64 // Score threshold for trending
	
	// Time windows for analysis
	TimeWindows            []TrendingTimeWindow
}

// TrendingTimeWindow defines a time window for trending analysis
type TrendingTimeWindow struct {
	Name        string        // e.g., "1h", "6h", "24h", "7d"
	Duration    time.Duration // Window duration
	Weight      float64       // Weight in final score calculation
	MinScore    float64       // Minimum score for this window
}

// TrendingCalculator handles sophisticated hashtag trending computation
type TrendingCalculator struct {
	config TrendingCalculatorConfig
	logger *zap.Logger
}

// TrendingMetrics holds metrics for trending calculation
type TrendingMetrics struct {
	HashtagName     string
	TotalUsage      int64
	UniqueUsers     int64
	Engagements     int64
	TrustScore      float64
	FirstSeen       time.Time
	LastUsed        time.Time
	TimeWindowData  map[string]*WindowMetrics
	HistoricalTrend []float64 // 7-day historical scores
	MomentumScore   float64   // Rate of change
}

// WindowMetrics holds metrics for a specific time window
type WindowMetrics struct {
	UsageCount     int64
	UniqueUsers    int64
	Engagements    int64
	AverageTrust   float64
	GrowthRate     float64
	Velocity       float64 // Usage per hour
}

// TrendingScore represents the calculated trending score
type TrendingScore struct {
	HashtagName    string
	OverallScore   float64
	ComponentScores map[string]float64 // Individual component scores
	Metrics        *TrendingMetrics
	Rank           int
	Timestamp      time.Time
}

// TrendingAnalytics provides insights into the trending calculation process
type TrendingAnalytics struct {
	Period              time.Time `json:"period"`
	TotalHashtags       int64     `json:"total_hashtags"`
	TotalUsage          int64     `json:"total_usage"`
	UniqueUsers         int64     `json:"unique_users"`
	TrendingCandidates  int64     `json:"trending_candidates"`
	AverageUsagePerTag  float64   `json:"average_usage_per_tag"`
	AverageUsersPerTag  float64   `json:"average_users_per_tag"`
	TrendingThreshold   float64   `json:"trending_threshold"`
	MinimumUsage        int64     `json:"minimum_usage"`
	MinimumUsers        int64     `json:"minimum_users"`
	CalculationWindows  int       `json:"calculation_windows"`
	GeneratedAt         time.Time `json:"generated_at"`
}

// NewHashtagRepository creates a new hashtag repository
func NewHashtagRepository(db core.DB, tableName string, logger *zap.Logger, domain string) *HashtagRepository {
	config := TrendingCalculatorConfig{
		// Time decay parameters
		DecayHalfLife: 2 * time.Hour,     // Scores decay by half every 2 hours
		MinimumAge:    5 * time.Minute,    // Must be at least 5 minutes old
		MaximumAge:    7 * 24 * time.Hour, // Don't consider older than 7 days
		
		// Scoring weights (total should be ~1.0)
		UsageWeight:      0.3,  // 30% weight for usage count
		EngagementWeight: 0.25, // 25% weight for engagements
		DiversityWeight:  0.2,  // 20% weight for user diversity
		TrustWeight:      0.1,  // 10% weight for trust scores
		MomentumWeight:   0.15, // 15% weight for momentum/velocity
		
		// Thresholds
		MinimumUsage:      3,    // At least 3 uses
		MinimumUsers:      2,    // At least 2 different users
		TrendingThreshold: 1.0,  // Score > 1.0 to be trending
		
		// Multiple time windows for analysis
		TimeWindows: []TrendingTimeWindow{
			{Name: "1h", Duration: time.Hour, Weight: 0.4, MinScore: 0.1},
			{Name: "6h", Duration: 6 * time.Hour, Weight: 0.3, MinScore: 0.5},
			{Name: "24h", Duration: 24 * time.Hour, Weight: 0.2, MinScore: 1.0},
			{Name: "7d", Duration: 7 * 24 * time.Hour, Weight: 0.1, MinScore: 2.0},
		},
	}
	
	return &HashtagRepository{
		db:                 db,
		tableName:          tableName,
		logger:             logger,
		domain:             domain,
		trendingCalculator: NewTrendingCalculator(config, logger),
	}
}

// NewTrendingCalculator creates a new trending calculator with the given configuration
func NewTrendingCalculator(config TrendingCalculatorConfig, logger *zap.Logger) *TrendingCalculator {
	return &TrendingCalculator{
		config: config,
		logger: logger,
	}
}

// IndexHashtag indexes a hashtag when used in a status
func (r *HashtagRepository) IndexHashtag(_ context.Context, hashtag string, statusID string, authorID string, visibility string) error {
	now := time.Now()
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))

	// First, try to get existing metadata
	var existingHashtag models.Hashtag
	err := r.db.Model(&models.Hashtag{}).
		Where("PK", "=", fmt.Sprintf("HASHTAG#%s", tagLower)).
		Where("SK", "=", "METADATA").
		First(&existingHashtag)

	var existingCount int64
	firstSeen := now

	if err == nil {
		// Update existing metadata
		existingCount = existingHashtag.UsageCount
		firstSeen = existingHashtag.FirstSeen
	} else if !errors.IsNotFound(err) {
		// Unexpected error
		r.logger.Error("failed to get existing hashtag metadata",
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return fmt.Errorf("failed to get existing hashtag metadata: %w", err)
	}

	// Create/update hashtag metadata
	hashtagMetadata := &models.Hashtag{
		Name:       tagLower,
		URL:        fmt.Sprintf("https://%s/tags/%s", r.domain, tagLower),
		UsageCount: existingCount + 1,
		FirstSeen:  firstSeen,
		LastUsed:   now,
		UpdatedAt:  now,
		CreatedAt:  now,
	}
	hashtagMetadata.UpdateKeys()

	// Create or update the hashtag metadata
	err = r.db.Model(hashtagMetadata).Create()
	if err != nil {
		r.logger.Error("failed to update hashtag metadata",
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return fmt.Errorf("failed to update hashtag metadata: %w", err)
	}

	// Record usage
	usage := &models.HashtagUsage{
		StatusID:   statusID,
		AuthorID:   authorID,
		UsedAt:     now,
		Visibility: visibility,
		TTL:        now.Add(30 * 24 * time.Hour).Unix(), // 30 days TTL
		CreatedAt:  now,
	}
	usage.UpdateKeys(tagLower)

	err = r.db.Model(usage).Create()
	if err != nil {
		r.logger.Error("failed to record hashtag usage",
			zap.String("hashtag", tagLower),
			zap.String("status_id", statusID),
			zap.Error(err))
		return fmt.Errorf("failed to record hashtag usage: %w", err)
	}

	return nil
}

// GetHashtagInfo retrieves information about a specific hashtag
func (r *HashtagRepository) GetHashtagInfo(_ context.Context, hashtag string) (*storage.Hashtag, error) {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))

	var hashtagModel models.Hashtag
	err := r.db.Model(&models.Hashtag{}).
		Where("PK", "=", fmt.Sprintf("HASHTAG#%s", tagLower)).
		Where("SK", "=", "METADATA").
		First(&hashtagModel)

	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		r.logger.Error("failed to get hashtag info",
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get hashtag info: %w", err)
	}

	return &storage.Hashtag{
		Name:       hashtagModel.Name,
		URL:        hashtagModel.URL,
		UsageCount: int(hashtagModel.UsageCount),
		FirstSeen:  hashtagModel.FirstSeen,
		LastUsed:   hashtagModel.LastUsed,
	}, nil
}

// GetHashtagUsageHistory retrieves recent usage history for a hashtag
func (r *HashtagRepository) GetHashtagUsageHistory(_ context.Context, hashtag string, days int) ([]int64, error) {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))

	// Initialize result array
	history := make([]int64, days)

	// Get usage for each day
	now := time.Now()
	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -i)
		dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		dayEnd := dayStart.Add(24 * time.Hour)

		// Query usage for this day
		count, err := r.db.Model(&models.HashtagUsage{}).
			Where("PK", "=", fmt.Sprintf("HASHTAG#%s", tagLower)).
			Where("SK", ">=", fmt.Sprintf("USAGE#%d", dayStart.Unix())).
			Where("SK", "<=", fmt.Sprintf("USAGE#%d", dayEnd.Unix())).
			Count()

		if err == nil {
			history[i] = count
		} else {
			r.logger.Warn("failed to get usage count for day",
				zap.String("hashtag", tagLower),
				zap.Time("date", date),
				zap.Error(err))
		}
	}

	return history, nil
}

// GetHashtagActivity retrieves activities for a hashtag since a specific time
func (r *HashtagRepository) GetHashtagActivity(_ context.Context, hashtag string, since time.Time) ([]*storage.Activity, error) {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))

	var usageRecords []*models.HashtagUsage
	err := r.db.Model(&models.HashtagUsage{}).
		Where("PK", "=", fmt.Sprintf("HASHTAG#%s", tagLower)).
		Where("SK", ">=", fmt.Sprintf("USAGE#%d", since.Unix())).
		OrderBy("SK", "DESC"). // Descending order
		All(&usageRecords)

	if err != nil {
		r.logger.Error("failed to get hashtag activity",
			zap.String("hashtag", tagLower),
			zap.Time("since", since),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get hashtag activity: %w", err)
	}

	// Convert to Activity records
	activities := make([]*storage.Activity, len(usageRecords))
	for i, usage := range usageRecords {
		activities[i] = &storage.Activity{
			ID:        usage.StatusID,
			Type:      "Note",
			Object:    usage.StatusID,
			Actor:     usage.AuthorID,
			Published: usage.UsedAt,
		}
	}

	return activities, nil
}

// GetHashtagStats retrieves hashtag statistics
func (r *HashtagRepository) GetHashtagStats(ctx context.Context, hashtag string) (any, error) {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))

	// Get basic hashtag info
	hashtagInfo, err := r.GetHashtagInfo(ctx, hashtag)
	if err != nil {
		return nil, err
	}
	if hashtagInfo == nil {
		return nil, nil
	}

	// Get usage history for the past 7 days
	history, err := r.GetHashtagUsageHistory(ctx, hashtag, 7)
	if err != nil {
		r.logger.Warn("failed to get usage history for stats",
			zap.String("hashtag", tagLower),
			zap.Error(err))
		history = make([]int64, 7) // Default to zeros
	}

	// Calculate unique users from recent usage (simplified for DynamORM compatibility)
	var usageRecords []*models.HashtagUsage
	err = r.db.Model(&models.HashtagUsage{}).
		Where("PK", "=", fmt.Sprintf("HASHTAG#%s", tagLower)).
		Where("SK", ">=", fmt.Sprintf("USAGE#%d", time.Now().AddDate(0, 0, -30).Unix())).
		All(&usageRecords)

	uniqueUsers := 0
	if err == nil {
		// Count unique users manually
		userSet := make(map[string]bool)
		for _, record := range usageRecords {
			userSet[record.AuthorID] = true
		}
		uniqueUsers = len(userSet)
	} else {
		r.logger.Warn("failed to get usage records for unique user count",
			zap.String("hashtag", tagLower),
			zap.Error(err))
	}

	// Build hashtag history entries
	historyEntries := make([]storage.HashtagHistoryEntry, len(history))
	for i, count := range history {
		date := time.Now().AddDate(0, 0, -i)
		historyEntries[i] = storage.HashtagHistoryEntry{
			Date:       date.Format(common.DateFormat),
			UsageCount: fmt.Sprintf("%d", count),
			UserCount:  fmt.Sprintf("%d", uniqueUsers/7), // Approximate distribution
		}
	}

	stats := &storage.HashtagStats{
		Name:          hashtagInfo.Name,
		UsageCount:    hashtagInfo.UsageCount,
		UniqueUsers:   uniqueUsers,
		FirstSeen:     hashtagInfo.FirstSeen,
		LastUsed:      hashtagInfo.LastUsed,
		TrendingScore: float64(hashtagInfo.UsageCount) / time.Since(hashtagInfo.FirstSeen).Hours(),
		TotalUses:     int64(hashtagInfo.UsageCount),
		TotalAccounts: int64(uniqueUsers),
		History:       historyEntries,
	}

	return stats, nil
}

// GetHashtagTimelineAdvanced retrieves hashtag timeline with advanced filtering
func (r *HashtagRepository) GetHashtagTimelineAdvanced(_ context.Context, hashtag string, maxID *string, limit int, _ string) ([]*storage.StatusSearchResult, error) {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))

	// Build query for hashtag usage
	query := r.db.Model(&models.HashtagUsage{}).
		Where("PK", "=", fmt.Sprintf("HASHTAG#%s", tagLower))

	// Add maxID filter if provided
	if maxID != nil && *maxID != "" {
		query = query.Where("SK", "<", fmt.Sprintf("USAGE#%s", *maxID))
	}

	// Set limit
	if limit <= 0 || limit > 40 {
		limit = 20
	}
	query = query.Limit(limit).OrderBy("SK", "DESC") // Descending order

	var usageRecords []*models.HashtagUsage
	err := query.All(&usageRecords)
	if err != nil {
		r.logger.Error("failed to get hashtag timeline",
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get hashtag timeline: %w", err)
	}

	// Convert to status search results
	results := make([]*storage.StatusSearchResult, len(usageRecords))
	for i, usage := range usageRecords {
		results[i] = &storage.StatusSearchResult{
			StatusID:  usage.StatusID,
			AuthorID:  usage.AuthorID,
			Published: usage.UsedAt,
			Content:   "", // Not available in usage record
			URL:       "", // Not available in usage record
		}
	}

	return results, nil
}

// GetMultiHashtagTimeline retrieves timeline for multiple hashtags
func (r *HashtagRepository) GetMultiHashtagTimeline(ctx context.Context, hashtags []string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error) {
	if len(hashtags) == 0 {
		return []*storage.StatusSearchResult{}, nil
	}

	if limit <= 0 || limit > 40 {
		limit = 20
	}

	allResults := make([]*storage.StatusSearchResult, 0)

	// Query each hashtag separately and merge results
	for _, hashtag := range hashtags {
		results, err := r.GetHashtagTimelineAdvanced(ctx, hashtag, maxID, limit, userID)
		if err != nil {
			r.logger.Warn("failed to get timeline for hashtag in multi-query",
				zap.String("hashtag", hashtag),
				zap.Error(err))
			continue
		}
		allResults = append(allResults, results...)
	}

	// Sort by creation time descending and limit
	if len(allResults) > limit {
		// Simple sort by Published - in production you'd want more sophisticated sorting
		for i := 0; i < len(allResults)-1; i++ {
			for j := i + 1; j < len(allResults); j++ {
				if allResults[i].Published.Before(allResults[j].Published) {
					allResults[i], allResults[j] = allResults[j], allResults[i]
				}
			}
		}
		allResults = allResults[:limit]
	}

	return allResults, nil
}

// GetSuggestedHashtags gets suggested hashtags for a user
func (r *HashtagRepository) GetSuggestedHashtags(_ context.Context, userID string, limit int) ([]*storage.HashtagSearchResult, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}

	// Get recent popular hashtags
	// Query hashtags that have been used recently and frequently
	var hashtagModels []*models.Hashtag
	err := r.db.Model(&models.Hashtag{}).
		Where("SK", "=", "METADATA").
		Where("LastUsed", ">=", time.Now().AddDate(0, 0, -7).Format(time.RFC3339)). // Last 7 days
		OrderBy("UsageCount", "DESC").                                              // Most used first
		Limit(limit).
		All(&hashtagModels)

	if err != nil {
		r.logger.Error("failed to get suggested hashtags",
			zap.String("user_id", userID),
			zap.Error(err))
		return []*storage.HashtagSearchResult{}, nil // Return empty slice, not error
	}

	// Convert to hashtag search results
	results := make([]*storage.HashtagSearchResult, len(hashtagModels))
	for i, hashtag := range hashtagModels {
		results[i] = &storage.HashtagSearchResult{
			Name: hashtag.Name,
			URL:  hashtag.URL,
			History: []storage.HashtagHistoryEntry{
				{
					Day:        time.Now().Format(common.DateFormat),
					Date:       time.Now().Format(common.DateFormat),
					Uses:       fmt.Sprintf("%d", hashtag.UsageCount),
					UsageCount: fmt.Sprintf("%d", hashtag.UsageCount),
					Accounts:   "1",
				},
			},
		}
	}

	return results, nil
}

// FollowHashtag creates a hashtag follow relationship
func (r *HashtagRepository) FollowHashtag(_ context.Context, userID string, hashtag string) error {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))
	now := time.Now()

	follow := &models.HashtagFollow{
		UserID:               userID,
		Hashtag:              tagLower,
		NotificationsEnabled: true, // Default to enabled
		Muted:                false,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	follow.UpdateKeys(userID, tagLower)

	err := r.db.Model(follow).Create()
	if err != nil {
		r.logger.Error("failed to create hashtag follow",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return fmt.Errorf("failed to create hashtag follow: %w", err)
	}

	return nil
}

// UnfollowHashtag removes a hashtag follow relationship
func (r *HashtagRepository) UnfollowHashtag(_ context.Context, userID string, hashtag string) error {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))

	follow := &models.HashtagFollow{}
	follow.UpdateKeys(userID, tagLower)

	err := r.db.Model(follow).Delete()
	if err != nil {
		r.logger.Error("failed to delete hashtag follow",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return fmt.Errorf("failed to delete hashtag follow: %w", err)
	}

	return nil
}

// IsFollowingHashtag checks if a user is following a hashtag
func (r *HashtagRepository) IsFollowingHashtag(_ context.Context, userID string, hashtag string) (bool, error) {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))

	var follow models.HashtagFollow
	err := r.db.Model(&models.HashtagFollow{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", userID)).
		Where("SK", "=", fmt.Sprintf("HASHTAG_FOLLOW#%s", tagLower)).
		First(&follow)

	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		r.logger.Error("failed to check hashtag follow",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return false, fmt.Errorf("failed to check hashtag follow: %w", err)
	}

	return true, nil
}

// GetFollowedHashtags retrieves hashtags followed by a user
func (r *HashtagRepository) GetFollowedHashtags(_ context.Context, userID string, limit int, cursor string) ([]string, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := r.db.Model(&models.HashtagFollow{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", userID)).
		Where("SK", "BEGINS_WITH", "HASHTAG_FOLLOW#").
		Limit(limit)

	// Add cursor if provided
	if cursor != "" {
		query = query.Where("SK", ">", cursor)
	}

	var follows []*models.HashtagFollow
	err := query.All(&follows)
	if err != nil {
		r.logger.Error("failed to get followed hashtags",
			zap.String("user_id", userID),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get followed hashtags: %w", err)
	}

	// Extract hashtag names
	hashtags := make([]string, len(follows))
	for i, follow := range follows {
		hashtags[i] = follow.Hashtag
	}

	// Determine next cursor
	nextCursor := ""
	if len(follows) == limit {
		nextCursor = follows[len(follows)-1].SK
	}

	return hashtags, nextCursor, nil
}

// UpdateHashtagNotificationSettings updates notification settings for a followed hashtag
func (r *HashtagRepository) UpdateHashtagNotificationSettings(_ context.Context, userID, hashtag string, notify bool) error {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))
	now := time.Now()

	// Get existing follow
	var existingFollow models.HashtagFollow
	err := r.db.Model(&models.HashtagFollow{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", userID)).
		Where("SK", "=", fmt.Sprintf("HASHTAG_FOLLOW#%s", tagLower)).
		First(&existingFollow)

	if err != nil {
		r.logger.Error("failed to get hashtag follow for update",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return fmt.Errorf("failed to get hashtag follow: %w", err)
	}

	// Update the fields
	existingFollow.NotificationsEnabled = notify
	existingFollow.UpdatedAt = now

	// Save by recreating (DynamORM pattern)
	err = r.db.Model(&existingFollow).Create()
	if err != nil {
		r.logger.Error("failed to update hashtag notification settings",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Bool("notify", notify),
			zap.Error(err))
		return fmt.Errorf("failed to update hashtag notification settings: %w", err)
	}

	return nil
}

// MuteHashtag mutes a hashtag for a user
func (r *HashtagRepository) MuteHashtag(_ context.Context, userID, hashtag string) error {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))
	now := time.Now()

	// Get existing follow
	var existingFollow models.HashtagFollow
	err := r.db.Model(&models.HashtagFollow{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", userID)).
		Where("SK", "=", fmt.Sprintf("HASHTAG_FOLLOW#%s", tagLower)).
		First(&existingFollow)

	if err != nil {
		r.logger.Error("failed to get hashtag follow for mute",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return fmt.Errorf("failed to get hashtag follow: %w", err)
	}

	// Update the fields
	existingFollow.Muted = true
	existingFollow.UpdatedAt = now

	// Save by recreating (DynamORM pattern)
	err = r.db.Model(&existingFollow).Create()
	if err != nil {
		r.logger.Error("failed to mute hashtag",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return fmt.Errorf("failed to mute hashtag: %w", err)
	}

	return nil
}

// UnmuteHashtag unmutes a hashtag for a user
func (r *HashtagRepository) UnmuteHashtag(_ context.Context, userID, hashtag string) error {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))
	now := time.Now()

	// Get existing follow
	var existingFollow models.HashtagFollow
	err := r.db.Model(&models.HashtagFollow{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", userID)).
		Where("SK", "=", fmt.Sprintf("HASHTAG_FOLLOW#%s", tagLower)).
		First(&existingFollow)

	if err != nil {
		r.logger.Error("failed to get hashtag follow for unmute",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return fmt.Errorf("failed to get hashtag follow: %w", err)
	}

	// Update the fields
	existingFollow.Muted = false
	existingFollow.UpdatedAt = now

	// Save by recreating (DynamORM pattern)
	err = r.db.Model(&existingFollow).Create()
	if err != nil {
		r.logger.Error("failed to unmute hashtag",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return fmt.Errorf("failed to unmute hashtag: %w", err)
	}

	return nil
}

// IsHashtagMuted checks if a hashtag is muted for a user
func (r *HashtagRepository) IsHashtagMuted(_ context.Context, userID, hashtag string) (bool, error) {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))

	var follow models.HashtagFollow
	err := r.db.Model(&models.HashtagFollow{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", userID)).
		Where("SK", "=", fmt.Sprintf("HASHTAG_FOLLOW#%s", tagLower)).
		First(&follow)

	if errors.IsNotFound(err) {
		return false, nil // Not following, so not muted
	}
	if err != nil {
		r.logger.Error("failed to check if hashtag is muted",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return false, fmt.Errorf("failed to check if hashtag is muted: %w", err)
	}

	return follow.Muted, nil
}

// DeleteOldHashtagTrends deletes hashtag trend records older than the specified time
func (r *HashtagRepository) DeleteOldHashtagTrends(ctx context.Context, before time.Time) error {
	r.logger.Info("starting deletion of old hashtag trends",
		zap.Time("before", before))

	// Delete in multiple stages: HashtagTrend models, TrendingHashtag models, and old HashtagUsage
	var totalDeleted int
	var mu sync.Mutex

	// Stage 1: Delete old HashtagTrend records (from trends.go model)
	count1, err := r.deleteOldHashtagTrendRecords(ctx, before)
	if err != nil {
		r.logger.Error("failed to delete hashtag trend records", zap.Error(err))
	} else {
		mu.Lock()
		totalDeleted += count1
		mu.Unlock()
	}

	// Stage 2: Delete old TrendingHashtag records (from trending_hashtag.go model)
	count2, err := r.deleteOldTrendingHashtagRecords(ctx, before)
	if err != nil {
		r.logger.Error("failed to delete trending hashtag records", zap.Error(err))
	} else {
		mu.Lock()
		totalDeleted += count2
		mu.Unlock()
	}

	// Stage 3: Delete old HashtagUsage records (cleanup expired TTL items)
	count3, err := r.deleteOldHashtagUsage(ctx, before)
	if err != nil {
		r.logger.Error("failed to delete old hashtag usage records", zap.Error(err))
	} else {
		mu.Lock()
		totalDeleted += count3
		mu.Unlock()
	}

	r.logger.Info("completed deletion of old hashtag trends",
		zap.Int("total_deleted", totalDeleted),
		zap.Time("before", before),
		zap.Int("hashtag_trends", count1),
		zap.Int("trending_hashtags", count2),
		zap.Int("usage_records", count3))

	return nil
}

// GetRecentHashtags returns hashtags that have been recently used
func (r *HashtagRepository) GetRecentHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	// Safety check on limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Query hashtag metadata entries that have been used recently
	var hashtagModels []*models.Hashtag
	err := r.db.WithContext(ctx).Model(&models.Hashtag{}).
		Where("SK", "=", "METADATA").
		Where("LastUsed", ">=", since.Format(time.RFC3339)).
		OrderBy("LastUsed", "DESC").
		Limit(limit).
		All(&hashtagModels)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.TrendingHashtag{}, nil
		}
		return nil, fmt.Errorf("failed to get recent hashtags: %w", err)
	}

	// Convert to storage.TrendingHashtag
	result := make([]*storage.TrendingHashtag, len(hashtagModels))
	for i, h := range hashtagModels {
		result[i] = &storage.TrendingHashtag{
			Name:        h.Name,
			URL:         fmt.Sprintf("https://%s/tags/%s", r.domain, h.Name),
			UsageCount:  h.UsageCount,
			UniqueUsers: 0, // Not tracked in simple model
			LastUsed:    h.LastUsed,
			FirstSeen:   h.FirstSeen,
			UserID:      "", // Not tracked in metadata
			CreatedAt:   h.LastUsed,
		}
	}

	return result, nil
}

// GetTrendingHashtags returns trending hashtags using sophisticated scoring algorithms
func (r *HashtagRepository) GetTrendingHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	start := time.Now()
	defer func() {
		r.logger.Debug("GetTrendingHashtags completed",
			zap.Duration("duration", time.Since(start)),
			zap.Int("limit", limit),
			zap.Time("since", since))
	}()
	
	// Safety check on limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	
	// Step 1: Get candidate hashtags that have been active recently
	candidates, err := r.getCandidateHashtags(ctx, since, limit*3) // Get more candidates for better filtering
	if err != nil {
		return nil, fmt.Errorf("failed to get candidate hashtags: %w", err)
	}
	
	if len(candidates) == 0 {
		r.logger.Info("no candidate hashtags found for trending analysis")
		return []*storage.TrendingHashtag{}, nil
	}
	
	// Step 2: Calculate trending scores for each candidate
	trendingScores, err := r.calculateTrendingScores(ctx, candidates)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate trending scores: %w", err)
	}
	
	// Step 3: Filter by trending threshold and sort by score
	filtered := r.filterAndSortByScore(trendingScores, limit)
	
	// Step 4: Store trending results for historical tracking
	go func() {
		if err := r.storeTrendingResults(context.Background(), filtered); err != nil {
			r.logger.Warn("failed to store trending results", zap.Error(err))
		}
	}()
	
	// Step 5: Convert to storage format
	result := make([]*storage.TrendingHashtag, len(filtered))
	for i, score := range filtered {
		result[i] = &storage.TrendingHashtag{
			Name:        score.HashtagName,
			URL:         fmt.Sprintf("https://%s/tags/%s", r.domain, score.HashtagName),
			UsageCount:  score.Metrics.TotalUsage,
			UniqueUsers: score.Metrics.UniqueUsers,
			LastUsed:    score.Metrics.LastUsed,
			FirstSeen:   score.Metrics.FirstSeen,
			UserID:      "", // Not applicable for hashtags
			CreatedAt:   score.Timestamp,
		}
	}
	
	r.logger.Info("calculated trending hashtags",
		zap.Int("candidates", len(candidates)),
		zap.Int("scored", len(trendingScores)),
		zap.Int("trending", len(result)),
		zap.Duration("calculation_time", time.Since(start)))
	
	return result, nil
}

// StoreHashtagTrend stores trending data for a hashtag
func (r *HashtagRepository) StoreHashtagTrend(_ context.Context, trendData any) error {
	// Store trending data by converting to HashtagTrend model and saving
	switch trend := trendData.(type) {
	case *TrendingScore:
		trendModel := &models.HashtagTrend{
			Name:        trend.HashtagName,
			URL:         fmt.Sprintf("https://%s/tags/%s", r.domain, trend.HashtagName),
			UsageCount:  trend.Metrics.TotalUsage,
			UniqueUsers: trend.Metrics.UniqueUsers,
			LastUsed:    trend.Metrics.LastUsed,
			FirstSeen:   trend.Metrics.FirstSeen,
			TrendScore:  trend.OverallScore,
			UpdatedAt:   trend.Timestamp,
		}
		trendModel.UpdateKeys()
		
		err := r.db.Model(trendModel).Create()
		if err != nil {
			r.logger.Error("failed to store hashtag trend",
				zap.String("hashtag", trend.HashtagName),
				zap.Float64("score", trend.OverallScore),
				zap.Error(err))
			return fmt.Errorf("failed to store hashtag trend: %w", err)
		}
		
		r.logger.Debug("stored hashtag trend",
			zap.String("hashtag", trend.HashtagName),
			zap.Float64("score", trend.OverallScore))
		
	case *storage.TrendingHashtag:
		// Convert storage type to model
		trendModel := &models.HashtagTrend{
			Name:        trend.Name,
			URL:         trend.URL,
			UsageCount:  trend.UsageCount,
			UniqueUsers: trend.UniqueUsers,
			LastUsed:    trend.LastUsed,
			FirstSeen:   trend.FirstSeen,
			TrendScore:  float64(trend.UsageCount), // Simple score fallback
			UpdatedAt:   trend.CreatedAt,
		}
		trendModel.UpdateKeys()
		
		err := r.db.Model(trendModel).Create()
		if err != nil {
			r.logger.Error("failed to store storage hashtag trend",
				zap.String("hashtag", trend.Name),
				zap.Error(err))
			return fmt.Errorf("failed to store hashtag trend: %w", err)
		}
		
	default:
		r.logger.Warn("unknown trend data type for storage",
			zap.String("type", fmt.Sprintf("%T", trendData)))
		return fmt.Errorf("unsupported trend data type: %T", trendData)
	}
	
	return nil
}

// deleteOldHashtagTrendRecords deletes HashtagTrend model records older than specified time
func (r *HashtagRepository) deleteOldHashtagTrendRecords(ctx context.Context, before time.Time) (int, error) {
	var trends []*models.HashtagTrend
	var deletedCount int
	batchSize := 25 // DynamoDB batch limit

	// Query old trend records using Filter and Scan
	err := r.db.WithContext(ctx).Model(&models.HashtagTrend{}).
		Filter("UpdatedAt", "<", before.Format(time.RFC3339)).
		Limit(100). // Process in chunks
		Scan(&trends)

	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil // No old trends to delete
		}
		return 0, fmt.Errorf("failed to query old hashtag trends: %w", err)
	}

	// Use batch delete for efficiency
	if len(trends) > 0 {
		// Convert to []any for batch operations
		items := make([]any, len(trends))
		for i, trend := range trends {
			items[i] = trend
		}

		// Process in batches to respect DynamoDB limits
		for i := 0; i < len(items); i += batchSize {
			end := i + batchSize
			if end > len(items) {
				end = len(items)
			}

			batchItems := items[i:end]
			err := r.deleteTrendBatch(ctx, batchItems)
			if err != nil {
				r.logger.Warn("failed to delete trend batch",
					zap.Int("batch_start", i),
					zap.Int("batch_size", len(batchItems)),
					zap.Error(err))
				// Continue with other batches
			} else {
				deletedCount += len(batchItems)
			}
		}
	}

	r.logger.Debug("deleted hashtag trend records",
		zap.Int("count", deletedCount))

	return deletedCount, nil
}

// deleteOldTrendingHashtagRecords deletes TrendingHashtag model records older than specified time
func (r *HashtagRepository) deleteOldTrendingHashtagRecords(ctx context.Context, before time.Time) (int, error) {
	var trends []*models.TrendingHashtag
	var deletedCount int
	batchSize := 25

	// Query old trending hashtag records
	err := r.db.WithContext(ctx).Model(&models.TrendingHashtag{}).
		Filter("UpdatedAt", "<", before.Format(time.RFC3339)).
		Limit(100).
		Scan(&trends)

	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to query old trending hashtags: %w", err)
	}

	// Batch delete trending hashtags
	if len(trends) > 0 {
		items := make([]any, len(trends))
		for i, trend := range trends {
			items[i] = trend
		}

		for i := 0; i < len(items); i += batchSize {
			end := i + batchSize
			if end > len(items) {
				end = len(items)
			}

			batchItems := items[i:end]
			err := r.deleteTrendBatch(ctx, batchItems)
			if err != nil {
				r.logger.Warn("failed to delete trending hashtag batch",
					zap.Int("batch_start", i),
					zap.Error(err))
			} else {
				deletedCount += len(batchItems)
			}
		}
	}

	r.logger.Debug("deleted trending hashtag records",
		zap.Int("count", deletedCount))

	return deletedCount, nil
}

// deleteOldHashtagUsage removes expired hashtag usage records
func (r *HashtagRepository) deleteOldHashtagUsage(ctx context.Context, before time.Time) (int, error) {
	var usageRecords []*models.HashtagUsage
	var deletedCount int
	batchSize := 25

	// Query old usage records that haven't been cleaned up by TTL
	err := r.db.WithContext(ctx).Model(&models.HashtagUsage{}).
		Filter("UsedAt", "<", before.Format(time.RFC3339)).
		Limit(200). // Larger limit for usage cleanup
		Scan(&usageRecords)

	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to query old hashtag usage: %w", err)
	}

	// Batch delete usage records
	if len(usageRecords) > 0 {
		items := make([]any, len(usageRecords))
		for i, usage := range usageRecords {
			items[i] = usage
		}

		for i := 0; i < len(items); i += batchSize {
			end := i + batchSize
			if end > len(items) {
				end = len(items)
			}

			batchItems := items[i:end]
			err := r.deleteTrendBatch(ctx, batchItems)
			if err != nil {
				r.logger.Warn("failed to delete usage batch",
					zap.Int("batch_start", i),
					zap.Error(err))
			} else {
				deletedCount += len(batchItems)
			}
		}
	}

	r.logger.Debug("deleted hashtag usage records",
		zap.Int("count", deletedCount))

	return deletedCount, nil
}

// deleteTrendBatch performs batch delete using DynamORM
func (r *HashtagRepository) deleteTrendBatch(ctx context.Context, items []any) error {
	if len(items) == 0 {
		return nil
	}

	// Use DynamORM batch delete - delete items individually since BatchDelete may not be available
	for _, item := range items {
		if err := r.db.WithContext(ctx).Model(item).Delete(); err != nil {
			r.logger.Warn("failed to delete individual item", zap.Error(err))
			// Continue with other items rather than failing the whole batch
		}
	}

	return nil
}

// GetHashtagsByTimeRange retrieves hashtags within a specific time range
func (r *HashtagRepository) GetHashtagsByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Query hashtag metadata that was active in the time range
	var hashtagModels []*models.Hashtag
	err := r.db.WithContext(ctx).Model(&models.Hashtag{}).
		Where("SK", "=", "METADATA").
		Filter("LastUsed", ">=", startTime.Format(time.RFC3339)).
		Filter("LastUsed", "<=", endTime.Format(time.RFC3339)).
		OrderBy("LastUsed", "DESC").
		Limit(limit).
		Scan(&hashtagModels)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.TrendingHashtag{}, nil
		}
		return nil, fmt.Errorf("failed to get hashtags by time range: %w", err)
	}

	// Convert to storage.TrendingHashtag
	result := make([]*storage.TrendingHashtag, len(hashtagModels))
	for i, h := range hashtagModels {
		result[i] = &storage.TrendingHashtag{
			Name:        h.Name,
			URL:         h.URL,
			UsageCount:  h.UsageCount,
			UniqueUsers: 0, // Not tracked in basic model
			LastUsed:    h.LastUsed,
			FirstSeen:   h.FirstSeen,
			UserID:      "", // Not applicable for hashtag metadata
			CreatedAt:   h.UpdatedAt,
		}
	}

	r.logger.Debug("retrieved hashtags by time range",
		zap.Time("start_time", startTime),
		zap.Time("end_time", endTime),
		zap.Int("count", len(result)),
		zap.Int("limit", limit))

	return result, nil
}

// GetHashtagTrendsByScore retrieves trending hashtags ordered by trend score
func (r *HashtagRepository) GetHashtagTrendsByScore(ctx context.Context, date time.Time, limit int, ascending bool) ([]*storage.TrendingHashtag, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	dateStr := date.Format(common.DateFormat)
	order := "DESC"
	if ascending {
		order = "ASC"
	}

	// Query using GSI8 for efficient trending queries
	var trendModels []*models.HashtagTrend
	err := r.db.WithContext(ctx).Model(&models.HashtagTrend{}).
		Index("gsi8").
		Where("GSI8PK", "=", fmt.Sprintf("TREND_TYPE#HASHTAG#%s", dateStr)).
		Where("GSI8SK", "BEGINS_WITH", "SCORE#").
		OrderBy("GSI8SK", order).
		Limit(limit).
		All(&trendModels)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.TrendingHashtag{}, nil
		}
		return nil, fmt.Errorf("failed to get hashtag trends by score: %w", err)
	}

	// Convert to storage format
	result := make([]*storage.TrendingHashtag, len(trendModels))
	for i, trend := range trendModels {
		result[i] = &storage.TrendingHashtag{
			Name:        trend.Name,
			URL:         trend.URL,
			UsageCount:  trend.UsageCount,
			UniqueUsers: trend.UniqueUsers,
			LastUsed:    trend.LastUsed,
			FirstSeen:   trend.FirstSeen,
			UserID:      "", // Not applicable for trends
			CreatedAt:   trend.UpdatedAt,
		}
	}

	r.logger.Debug("retrieved hashtag trends by score",
		zap.String("date", dateStr),
		zap.Int("count", len(result)),
		zap.Int("limit", limit),
		zap.Bool("ascending", ascending))

	return result, nil
}

// BatchCreateHashtagTrends creates multiple hashtag trend records efficiently
func (r *HashtagRepository) BatchCreateHashtagTrends(ctx context.Context, trends []*storage.TrendingHashtag) error {
	if len(trends) == 0 {
		return nil
	}

	// Convert to models and update keys
	modelTrends := make([]*models.HashtagTrend, len(trends))
	for i, trend := range trends {
		modelTrend := &models.HashtagTrend{
			Name:        trend.Name,
			URL:         trend.URL,
			UsageCount:  trend.UsageCount,
			UniqueUsers: trend.UniqueUsers,
			LastUsed:    trend.LastUsed,
			FirstSeen:   trend.FirstSeen,
			TrendScore:  float64(trend.UsageCount), // Simple score based on usage
			UpdatedAt:   trend.CreatedAt,
		}
		modelTrend.UpdateKeys()
		modelTrends[i] = modelTrend
	}

	// Use batch writer for efficient creation
	batchWriter := batch.NewBatchWriter(r.db, batch.BatchWriterConfig{
		BatchSize: 25, // DynamoDB limit
		Logger:    r.logger,
	})

	// Convert to []any for batch operations
	items := make([]any, len(modelTrends))
	for i, trend := range modelTrends {
		items[i] = trend
	}

	result, err := batchWriter.WriteItems(ctx, items)
	if err != nil {
		return fmt.Errorf("failed to batch create hashtag trends: %w", err)
	}

	r.logger.Info("batch created hashtag trends",
		zap.Int("total_items", result.TotalItems),
		zap.Int("processed_items", result.ProcessedItems),
		zap.Int("failed_items", result.FailedItems),
		zap.Duration("duration", result.Duration))

	if result.FailedItems > 0 {
		return fmt.Errorf("batch creation had %d failed items", result.FailedItems)
	}

	return nil
}
