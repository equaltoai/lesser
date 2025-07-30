package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// HashtagRepository implements hashtag-related database operations using DynamORM
type HashtagRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
	domain    string
}

// NewHashtagRepository creates a new hashtag repository
func NewHashtagRepository(db core.DB, tableName string, logger *zap.Logger, domain string) *HashtagRepository {
	return &HashtagRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
		domain:    domain,
	}
}

// IndexHashtag indexes a hashtag when used in a status
func (r *HashtagRepository) IndexHashtag(ctx context.Context, hashtag string, statusID string, authorID string, visibility string) error {
	now := time.Now()
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))

	// First, try to get existing metadata
	var existingHashtag models.Hashtag
	err := r.db.Model(&models.Hashtag{}).
		Where("PK", "=", fmt.Sprintf("HASHTAG#%s", tagLower)).
		Where("SK", "=", "METADATA").
		First(&existingHashtag)

	var existingCount int64 = 0
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
func (r *HashtagRepository) GetHashtagInfo(ctx context.Context, hashtag string) (*storage.Hashtag, error) {
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
		UsageCount: hashtagModel.UsageCount,
		FirstSeen:  hashtagModel.FirstSeen,
		LastUsed:   hashtagModel.LastUsed,
	}, nil
}

// GetHashtagUsageHistory retrieves recent usage history for a hashtag
func (r *HashtagRepository) GetHashtagUsageHistory(ctx context.Context, hashtag string, days int) ([]int64, error) {
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
func (r *HashtagRepository) GetHashtagActivity(ctx context.Context, hashtag string, since time.Time) ([]*storage.Activity, error) {
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
			Date:       date,
			UsageCount: count,
			UserCount:  int64(uniqueUsers / 7), // Approximate distribution
		}
	}

	stats := &storage.HashtagStats{
		Name:          hashtagInfo.Name,
		UsageCount:    hashtagInfo.UsageCount,
		UniqueUsers:   int64(uniqueUsers),
		FirstSeen:     hashtagInfo.FirstSeen,
		LastUsed:      hashtagInfo.LastUsed,
		TrendingScore: float64(hashtagInfo.UsageCount) / time.Since(hashtagInfo.FirstSeen).Hours(),
		TotalUses:     hashtagInfo.UsageCount,
		TotalAccounts: int64(uniqueUsers),
		History:       historyEntries,
	}

	return stats, nil
}

// GetHashtagTimelineAdvanced retrieves hashtag timeline with advanced filtering
func (r *HashtagRepository) GetHashtagTimelineAdvanced(ctx context.Context, hashtag string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error) {
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
			StatusID:   usage.StatusID,
			AuthorID:   usage.AuthorID,
			Published:  usage.UsedAt,
			Content:    "", // Not available in usage record
			URL:        "", // Not available in usage record
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
func (r *HashtagRepository) GetSuggestedHashtags(ctx context.Context, userID string, limit int) ([]*storage.HashtagSearchResult, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}

	// Get recent popular hashtags
	// Query hashtags that have been used recently and frequently
	var hashtagModels []*models.Hashtag
	err := r.db.Model(&models.Hashtag{}).
		Where("SK", "=", "METADATA").
		Where("LastUsed", ">=", time.Now().AddDate(0, 0, -7).Format(time.RFC3339)). // Last 7 days
		OrderBy("UsageCount", "DESC"). // Most used first
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
			History: []*storage.TrendingHashtag{
				{
					Name:       hashtag.Name,
					URL:        hashtag.URL,
					UsageCount: hashtag.UsageCount,
					LastUsed:   hashtag.LastUsed,
					FirstSeen:  hashtag.FirstSeen,
				},
			},
		}
	}

	return results, nil
}
// FollowHashtag creates a hashtag follow relationship
func (r *HashtagRepository) FollowHashtag(ctx context.Context, userID string, hashtag string) error {
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
func (r *HashtagRepository) UnfollowHashtag(ctx context.Context, userID string, hashtag string) error {
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
func (r *HashtagRepository) IsFollowingHashtag(ctx context.Context, userID string, hashtag string) (bool, error) {
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
func (r *HashtagRepository) GetFollowedHashtags(ctx context.Context, userID string, limit int, cursor string) ([]string, string, error) {
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
func (r *HashtagRepository) UpdateHashtagNotificationSettings(ctx context.Context, userID, hashtag string, notify bool) error {
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
func (r *HashtagRepository) MuteHashtag(ctx context.Context, userID, hashtag string) error {
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
func (r *HashtagRepository) UnmuteHashtag(ctx context.Context, userID, hashtag string) error {
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
func (r *HashtagRepository) IsHashtagMuted(ctx context.Context, userID, hashtag string) (bool, error) {
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
