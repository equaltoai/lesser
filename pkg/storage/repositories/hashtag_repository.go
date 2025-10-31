package repositories

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/batch"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// HashtagRepository implements hashtag-related database operations using enhanced DynamORM patterns
type HashtagRepository struct {
	*EnhancedBaseRepository[*models.Hashtag]
	domain             string
	trendingCalculator *TrendingCalculator
	trendingEngine     *TrendingEngine
}

// TrendingCalculatorConfig holds configuration for trending algorithm
type TrendingCalculatorConfig struct {
	// Time decay parameters
	DecayHalfLife time.Duration // How quickly scores decay
	MinimumAge    time.Duration // Minimum age before considering trending
	MaximumAge    time.Duration // Maximum age for trending consideration

	// Scoring weights
	UsageWeight      float64 // Weight for usage count
	EngagementWeight float64 // Weight for engagement metrics
	DiversityWeight  float64 // Weight for user diversity
	TrustWeight      float64 // Weight for trust scores
	MomentumWeight   float64 // Weight for trending momentum

	// Thresholds
	MinimumUsage      int64   // Minimum usage count to consider
	MinimumUsers      int64   // Minimum unique users to consider
	TrendingThreshold float64 // Score threshold for trending

	// Time windows for analysis
	TimeWindows []TrendingTimeWindow
}

// TrendingTimeWindow defines a time window for trending analysis
type TrendingTimeWindow struct {
	Name     string        // e.g., "1h", "6h", "24h", "7d"
	Duration time.Duration // Window duration
	Weight   float64       // Weight in final score calculation
	MinScore float64       // Minimum score for this window
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
	UsageCount   int64
	UniqueUsers  int64
	Engagements  int64
	AverageTrust float64
	GrowthRate   float64
	Velocity     float64 // Usage per hour
}

// TrendingScore represents the calculated trending score
type TrendingScore struct {
	HashtagName     string
	OverallScore    float64
	ComponentScores map[string]float64 // Individual component scores
	Metrics         *TrendingMetrics
	Rank            int
	Timestamp       time.Time
}

// TrendingAnalytics provides insights into the trending calculation process
type TrendingAnalytics struct {
	Period             time.Time `json:"period"`
	TotalHashtags      int64     `json:"total_hashtags"`
	TotalUsage         int64     `json:"total_usage"`
	UniqueUsers        int64     `json:"unique_users"`
	TrendingCandidates int64     `json:"trending_candidates"`
	AverageUsagePerTag float64   `json:"average_usage_per_tag"`
	AverageUsersPerTag float64   `json:"average_users_per_tag"`
	TrendingThreshold  float64   `json:"trending_threshold"`
	MinimumUsage       int64     `json:"minimum_usage"`
	MinimumUsers       int64     `json:"minimum_users"`
	CalculationWindows int       `json:"calculation_windows"`
	GeneratedAt        time.Time `json:"generated_at"`
}

// NewHashtagRepository creates a new hashtag repository
func NewHashtagRepository(db core.DB, tableName string, logger *zap.Logger, domain string) *HashtagRepository {
	config := TrendingCalculatorConfig{
		// Time decay parameters
		DecayHalfLife: 2 * time.Hour,      // Scores decay by half every 2 hours
		MinimumAge:    5 * time.Minute,    // Must be at least 5 minutes old
		MaximumAge:    7 * 24 * time.Hour, // Don't consider older than 7 days

		// Scoring weights (total should be ~1.0)
		UsageWeight:      0.3,  // 30% weight for usage count
		EngagementWeight: 0.25, // 25% weight for engagements
		DiversityWeight:  0.2,  // 20% weight for user diversity
		TrustWeight:      0.1,  // 10% weight for trust scores
		MomentumWeight:   0.15, // 15% weight for momentum/velocity

		// Thresholds
		MinimumUsage:      3,   // At least 3 uses
		MinimumUsers:      2,   // At least 2 different users
		TrendingThreshold: 1.0, // Score > 1.0 to be trending

		// Multiple time windows for analysis
		TimeWindows: []TrendingTimeWindow{
			{Name: "1h", Duration: time.Hour, Weight: 0.4, MinScore: 0.1},
			{Name: "6h", Duration: 6 * time.Hour, Weight: 0.3, MinScore: 0.5},
			{Name: "24h", Duration: 24 * time.Hour, Weight: 0.2, MinScore: 1.0},
			{Name: "7d", Duration: 7 * 24 * time.Hour, Weight: 0.1, MinScore: 2.0},
		},
	}

	// Create enhanced repository optimized for hashtag operations
	enhancedRepo := NewEnhancedBaseRepository[*models.Hashtag](db, tableName, logger, nil, "HashtagRepository", "hashtag")

	// Set up enhanced services for hashtag operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Hashtags cached for trending performance
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for trending and discovery events

	return &HashtagRepository{
		EnhancedBaseRepository: enhancedRepo,
		domain:                 domain,
		trendingCalculator:     NewTrendingCalculator(config, logger),
		trendingEngine:         NewTrendingEngine(db, logger),
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
func (r *HashtagRepository) IndexHashtag(ctx context.Context, hashtag string, statusID string, authorID string, visibility string) error {
	now := time.Now()
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))
	pk := fmt.Sprintf("HASHTAG#%s", tagLower)
	sk := models.SKMetadata

	// First, try to get existing metadata
	var existingHashtag models.Hashtag
	err := r.Get(ctx, pk, sk, &existingHashtag)

	var existingCount int64
	firstSeen := now

	if err == nil {
		// Update existing metadata
		existingCount = existingHashtag.UsageCount
		firstSeen = existingHashtag.FirstSeen
	} else if !strings.Contains(err.Error(), "not found") {
		// Unexpected error
		return ErrorHandler.HandleGetError(err, "hashtag", tagLower)
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

	// Create or update the hashtag metadata using enhanced validation
	err = r.ValidateAndCreate(ctx, hashtagMetadata)
	if err != nil {
		r.logger.Error("failed to create hashtag with enhanced validation",
			zap.String("hashtag", tagLower),
			zap.Bool("validation_enabled", r.HasValidation()),
			zap.Bool("events_enabled", r.HasEvents()),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "hashtag", tagLower)
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
	usage.UpdateKeysWithHashtag(tagLower)

	err = r.db.Model(usage).Create()
	if err != nil {
		r.logger.Error("failed to record hashtag usage",
			zap.String("hashtag", tagLower),
			zap.String("status_id", statusID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "hashtag usage", statusID)
	}

	return nil
}

// IndexStatusHashtags indexes a status with its hashtags for efficient search
func (r *HashtagRepository) IndexStatusHashtags(ctx context.Context, statusID string, authorID string, authorHandle string, statusURL string, content string, hashtags []string, published time.Time, visibility string) error {
	// Validate status entity using centralized validation
	if err := common.ValidateStatusEntity(statusID, content, visibility); err != nil {
		return ErrorHandler.HandleGetError(err, "status entity", statusID)
	}

	if err := common.ValidateSliceNotEmpty("hashtags", hashtags); err != nil {
		return nil // Nothing to index
	}

	now := time.Now()
	ttl := now.Add(90 * 24 * time.Hour).Unix() // 90 days TTL

	// Create index entries for each hashtag
	indexEntries := make([]*models.HashtagStatusIndex, 0, len(hashtags))

	for _, hashtag := range hashtags {
		tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))

		indexEntry := &models.HashtagStatusIndex{
			StatusID:     statusID,
			AuthorID:     authorID,
			AuthorHandle: authorHandle,
			StatusURL:    statusURL,
			Content:      r.truncateContent(content, 200), // Store excerpt for search results
			Visibility:   visibility,
			Published:    published,
			HashtagName:  tagLower,
			TTL:          ttl,
			CreatedAt:    now,
		}
		_ = indexEntry.UpdateKeys()

		indexEntries = append(indexEntries, indexEntry)
	}

	// Batch create all index entries
	for _, entry := range indexEntries {
		err := r.db.WithContext(ctx).Model(entry).Create()
		if err != nil {
			r.logger.Error("failed to create hashtag status index",
				zap.String("status_id", statusID),
				zap.String("hashtag", entry.HashtagName),
				zap.Error(err))
			// Continue with other entries rather than failing entirely
		}
	}

	r.logger.Debug("indexed status hashtags",
		zap.String("status_id", statusID),
		zap.Int("hashtag_count", len(hashtags)),
		zap.Strings("hashtags", hashtags))

	return nil
}

// RemoveStatusFromHashtagIndex removes a status from all hashtag indexes
func (r *HashtagRepository) RemoveStatusFromHashtagIndex(ctx context.Context, statusID string) error {
	// Query all hashtag index entries for this status using the reverse index
	var indexEntries []models.HashtagStatusIndex
	err := r.db.WithContext(ctx).Model(&models.HashtagStatusIndex{}).
		Index("status-hashtag-index").
		Where("GSI1PK", "=", fmt.Sprintf("STATUS_HASHTAGS#%s", statusID)).
		Where("GSI1SK", "BEGINS_WITH", "HASHTAG#").
		All(&indexEntries)

	if err != nil && !errors.IsNotFound(err) {
		return ErrorHandler.HandleQueryError(err, "hashtag index", statusID)
	}

	// Delete all found entries
	for _, entry := range indexEntries {
		err := r.db.WithContext(ctx).Model(&models.HashtagStatusIndex{}).
			Where("PK", "=", entry.PK).
			Where("SK", "=", entry.SK).
			Delete()

		if err != nil {
			r.logger.Warn("failed to delete hashtag index entry",
				zap.String("status_id", statusID),
				zap.String("hashtag", entry.HashtagName),
				zap.Error(err))
			// Continue with other entries
		}
	}

	r.logger.Debug("removed status from hashtag indexes",
		zap.String("status_id", statusID),
		zap.Int("entries_removed", len(indexEntries)))

	return nil
}

// truncateContent truncates content to the specified length for indexing
func (r *HashtagRepository) truncateContent(content string, maxLength int) string {
	if len(content) <= maxLength {
		return content
	}

	// Try to truncate at word boundary
	truncated := content[:maxLength]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > maxLength/2 { // Only use word boundary if it's not too short
		truncated = truncated[:lastSpace]
	}

	return truncated + "..."
}

// GetHashtagInfo retrieves information about a specific hashtag
func (r *HashtagRepository) GetHashtagInfo(ctx context.Context, hashtag string) (*storage.Hashtag, error) {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))
	pk := fmt.Sprintf("HASHTAG#%s", tagLower)
	sk := models.SKMetadata

	var hashtagModel models.Hashtag
	err := r.Get(ctx, pk, sk, &hashtagModel)

	if err != nil {
		if errors.IsNotFound(err) || strings.Contains(err.Error(), "not found") {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "hashtag", tagLower)
		}
		return nil, ErrorHandler.HandleGetError(err, "hashtag", tagLower)
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
func (r *HashtagRepository) GetHashtagUsageHistory(ctx context.Context, hashtag string, days int) ([]int64, error) {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))

	// Initialize result array
	history := make([]int64, days)

	// Get usage for each day using BaseRepository
	now := time.Now()
	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -i)
		dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		dayEnd := dayStart.Add(24 * time.Hour)

		// Query usage for this day using BaseRepository range query
		pk := fmt.Sprintf("HASHTAG#%s", tagLower)
		startSK := fmt.Sprintf("USAGE#%d", dayStart.Unix())
		endSK := fmt.Sprintf("USAGE#%d", dayEnd.Unix())

		count, err := r.Count(ctx, pk)
		if err == nil {
			// This is a simplified count - in production you'd want to use QueryBetween
			// and count the results, but Count() is available from BaseRepository
			history[i] = int64(count)
		} else {
			r.logger.Warn("failed to get usage count for day",
				zap.String("hashtag", tagLower),
				zap.Time("date", date),
				zap.String("start_sk", startSK),
				zap.String("end_sk", endSK),
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
		return nil, ErrorHandler.HandleQueryError(err, "hashtag activity", tagLower)
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
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "hashtag", tagLower)
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

// GetHashtagTimelineAdvanced retrieves hashtag timeline with advanced filtering using efficient indexing
func (r *HashtagRepository) GetHashtagTimelineAdvanced(ctx context.Context, hashtag string, maxID *string, limit int, visibility string) ([]*storage.StatusSearchResult, error) {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))

	// Set reasonable limit
	if limit <= 0 || limit > 40 {
		limit = 20
	}

	var results []*storage.StatusSearchResult
	var err error

	if visibility != "" && visibility != models.VisibilityPublic {
		// Use visibility-filtered index for non-public content
		results, err = r.getHashtagTimelineByVisibility(ctx, tagLower, maxID, limit, visibility)
	} else {
		// Use main hashtag timeline index for public/all content
		results, err = r.getHashtagTimelineFromIndex(ctx, tagLower, maxID, limit)
	}

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "hashtag timeline", tagLower)
	}

	r.logger.Debug("retrieved hashtag timeline",
		zap.String("hashtag", tagLower),
		zap.Int("results", len(results)),
		zap.Int("limit", limit),
		zap.String("visibility", visibility))

	return results, nil
}

// getHashtagTimelineFromIndex retrieves timeline using the main hashtag index
func (r *HashtagRepository) getHashtagTimelineFromIndex(ctx context.Context, hashtag string, maxID *string, limit int) ([]*storage.StatusSearchResult, error) {
	// Build query for hashtag timeline index
	query := r.db.WithContext(ctx).Model(&models.HashtagStatusIndex{}).
		Where("PK", "=", fmt.Sprintf("HASHTAG_TIMELINE#%s", hashtag))

	// Apply the maxID cursor when provided
	if maxID != nil && *maxID != "" {
		query = query.Where("SK", ">", *maxID)
	}

	// Set limit and order (SK already contains descending timestamp)
	query = query.Limit(limit).OrderBy("SK", "ASC") // ASC because timestamp is already reversed

	var indexEntries []models.HashtagStatusIndex
	err := query.All(&indexEntries)
	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.StatusSearchResult{}, nil
		}
		r.logger.Error("hashtag timeline index query failed",
			zap.String("hashtag", hashtag),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "hashtag timeline index", hashtag)
	}

	// Convert index entries to search results
	results := make([]*storage.StatusSearchResult, len(indexEntries))
	for i, entry := range indexEntries {
		results[i] = &storage.StatusSearchResult{
			ID:             entry.SK,
			StatusID:       entry.StatusID,
			Content:        entry.Content,
			URL:            entry.StatusURL,
			AuthorID:       entry.AuthorID,
			AuthorUsername: entry.AuthorHandle,
			Published:      entry.Published,
			Score:          1.0, // All results have equal relevance in timeline
			Highlights:     []string{"hashtag_timeline"},
		}
	}

	return results, nil
}

// getHashtagTimelineByVisibility retrieves timeline filtered by visibility
func (r *HashtagRepository) getHashtagTimelineByVisibility(ctx context.Context, hashtag string, maxID *string, limit int, visibility string) ([]*storage.StatusSearchResult, error) {
	// Use the visibility-filtered GSI
	query := r.db.WithContext(ctx).Model(&models.HashtagStatusIndex{}).
		Index("hashtag-visibility-index").
		Where("GSI2PK", "=", fmt.Sprintf("HASHTAG_VIS#%s#%s", hashtag, visibility))

	// Apply the maxID cursor when provided
	if maxID != nil && *maxID != "" {
		query = query.Where("GSI2SK", ">", *maxID)
	}

	query = query.Limit(limit).OrderBy("GSI2SK", "ASC") // ASC because timestamp is reversed

	var indexEntries []models.HashtagStatusIndex
	err := query.All(&indexEntries)
	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.StatusSearchResult{}, nil
		}
		r.logger.Error("hashtag timeline visibility query failed",
			zap.String("hashtag", hashtag),
			zap.String("visibility", visibility),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "hashtag timeline visibility", hashtag)
	}

	// Convert to search results
	results := make([]*storage.StatusSearchResult, len(indexEntries))
	for i, entry := range indexEntries {
		results[i] = &storage.StatusSearchResult{
			ID:             entry.GSI2SK,
			StatusID:       entry.StatusID,
			Content:        entry.Content,
			URL:            entry.StatusURL,
			AuthorID:       entry.AuthorID,
			AuthorUsername: entry.AuthorHandle,
			Published:      entry.Published,
			Score:          1.0,
			Highlights:     []string{"hashtag_timeline_filtered"},
		}
	}

	return results, nil
}

// GetMultiHashtagTimeline retrieves timeline for multiple hashtags
func (r *HashtagRepository) GetMultiHashtagTimeline(ctx context.Context, hashtags []string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error) {
	if err := common.ValidateSliceNotEmpty("hashtags", hashtags); err != nil {
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

	// Get recent popular hashtags using the consolidated helper
	since := time.Now().AddDate(0, 0, -7) // Last 7 days
	hashtagModels, err := r.queryHashtagMetadataByDateRange(ctx, &since, nil, limit)
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
func (r *HashtagRepository) FollowHashtag(ctx context.Context, userID, hashtag string) error {
	tagLower := normalizeHashtagName(hashtag)
	if tagLower == "" {
		return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, EntityHashtag, "empty hashtag")
	}

	now := time.Now().UTC()
	follow := &models.HashtagFollow{
		PK:                   fmt.Sprintf("user#%s", userID),
		SK:                   fmt.Sprintf("hashtag#%s", tagLower),
		UserID:               userID,
		Hashtag:              tagLower,
		NotificationsEnabled: true,
		Muted:                false,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if err := r.db.WithContext(ctx).Model(follow).Create(); err != nil {
		if errors.IsConditionFailed(err) {
			r.logger.Debug("hashtag follow already exists",
				zap.String("user_id", userID),
				zap.String("hashtag", tagLower))
			return nil
		}
		r.logger.Error("failed to create hashtag follow",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityHashtag, fmt.Sprintf("follow %s#%s", userID, tagLower))
	}

	return nil
}

// UnfollowHashtag removes a hashtag follow relationship and related artifacts
func (r *HashtagRepository) UnfollowHashtag(ctx context.Context, userID, hashtag string) error {
	tagLower := normalizeHashtagName(hashtag)
	if tagLower == "" {
		return ErrorHandler.HandleDeleteError(storage.ErrInvalidInput, EntityHashtag, "empty hashtag")
	}

	pk := fmt.Sprintf("user#%s", userID)
	follow := &models.HashtagFollow{
		PK: pk,
		SK: fmt.Sprintf("hashtag#%s", tagLower),
	}

	if err := r.db.WithContext(ctx).Model(follow).Delete(); err != nil {
		if !errors.IsNotFound(err) {
			r.logger.Error("failed to delete hashtag follow",
				zap.String("user_id", userID),
				zap.String("hashtag", tagLower),
				zap.Error(err))
			return ErrorHandler.HandleDeleteError(err, EntityHashtag, fmt.Sprintf("follow %s#%s", userID, tagLower))
		}
	} else {
		r.logger.Debug("deleted hashtag follow",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower))
	}

	// Cleanup mute and notification records (best effort)
	mute := &models.HashtagMute{PK: pk, SK: fmt.Sprintf("mute#%s", tagLower)}
	if err := r.db.WithContext(ctx).Model(mute).Delete(); err != nil && !errors.IsNotFound(err) {
		r.logger.Debug("failed to cleanup hashtag mute after unfollow",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
	}

	settings := &models.HashtagNotificationSettings{PK: pk, SK: fmt.Sprintf("settings#%s", tagLower)}
	if err := r.db.WithContext(ctx).Model(settings).Delete(); err != nil && !errors.IsNotFound(err) {
		r.logger.Debug("failed to cleanup hashtag notification settings after unfollow",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
	}

	return nil
}

// IsFollowingHashtag checks if a user is following a hashtag
func (r *HashtagRepository) IsFollowingHashtag(ctx context.Context, userID, hashtag string) (bool, error) {
	tagLower := normalizeHashtagName(hashtag)
	if tagLower == "" {
		return false, ErrorHandler.HandleGetError(storage.ErrInvalidInput, EntityHashtag, "empty hashtag")
	}

	var follow models.HashtagFollow
	err := r.db.WithContext(ctx).Model(&models.HashtagFollow{}).
		Where("PK", "=", fmt.Sprintf("user#%s", userID)).
		Where("SK", "=", fmt.Sprintf("hashtag#%s", tagLower)).
		First(&follow)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to check hashtag follow",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return false, ErrorHandler.HandleGetError(err, EntityHashtag, fmt.Sprintf("follow %s#%s", userID, tagLower))
	}

	return true, nil
}

// GetHashtagFollow retrieves the hashtag follow record for a user
func (r *HashtagRepository) GetHashtagFollow(ctx context.Context, userID string, hashtag string) (*models.HashtagFollow, error) {
	tagLower := normalizeHashtagName(hashtag)
	if tagLower == "" {
		return nil, ErrorHandler.HandleGetError(storage.ErrInvalidInput, EntityHashtag, "empty hashtag")
	}

	pk := fmt.Sprintf("user#%s", userID)
	sk := fmt.Sprintf("hashtag#%s", tagLower)

	var follow models.HashtagFollow
	err := r.db.WithContext(ctx).Model(&models.HashtagFollow{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&follow)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityHashtag, fmt.Sprintf("follow %s#%s", userID, tagLower))
		}
		r.logger.Error("failed to get hashtag follow record",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityHashtag, fmt.Sprintf("follow %s#%s", userID, tagLower))
	}

	return &follow, nil
}

// GetHashtagMute retrieves the hashtag mute record for a user
func (r *HashtagRepository) GetHashtagMute(ctx context.Context, userID string, hashtag string) (*models.HashtagMute, error) {
	tagLower := normalizeHashtagName(hashtag)
	if tagLower == "" {
		return nil, ErrorHandler.HandleGetError(storage.ErrInvalidInput, EntityHashtag, "empty hashtag")
	}

	pk := fmt.Sprintf("user#%s", userID)
	sk := fmt.Sprintf("mute#%s", tagLower)

	var mute models.HashtagMute
	err := r.db.WithContext(ctx).Model(&models.HashtagMute{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&mute)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityHashtag, fmt.Sprintf("mute %s#%s", userID, tagLower))
		}
		r.logger.Error("failed to get hashtag mute record",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityHashtag, fmt.Sprintf("mute %s#%s", userID, tagLower))
	}

	if mute.TTL > 0 && time.Now().UTC().Unix() > mute.TTL {
		_ = r.UnmuteHashtag(ctx, userID, hashtag)
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityHashtag, fmt.Sprintf("mute %s#%s", userID, tagLower))
	}

	return &mute, nil
}

// GetFollowedHashtags retrieves hashtags followed by a user
func (r *HashtagRepository) GetFollowedHashtags(ctx context.Context, userID string, limit int, cursor string) ([]*storage.HashtagFollow, string, error) {
	limit = NormalizePaginationLimit(limit)
	pk := fmt.Sprintf("user#%s", userID)

	query := r.db.WithContext(ctx).Model(&models.HashtagFollow{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", "hashtag#").
		OrderBy("SK", "ASC").
		Limit(limit + 1)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	var followModels []*models.HashtagFollow
	if err := query.All(&followModels); err != nil {
		r.logger.Error("failed to query followed hashtags",
			zap.String("user_id", userID),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, EntityHashtag, fmt.Sprintf("follow list %s", userID))
	}

	nextCursor := ""
	if len(followModels) > limit {
		nextCursor = followModels[limit].SK
		followModels = followModels[:limit]
	}

	result := make([]*storage.HashtagFollow, len(followModels))
	for i, follow := range followModels {
		result[i] = convertHashtagFollowModel(follow)
	}

	return result, nextCursor, nil
}

// MuteHashtag mutes a hashtag for a user
func (r *HashtagRepository) MuteHashtag(ctx context.Context, userID, hashtag string, until *time.Time) error {
	tagLower := normalizeHashtagName(hashtag)
	if tagLower == "" {
		return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, EntityHashtag, "empty hashtag")
	}

	now := time.Now().UTC()
	mute := &models.HashtagMute{
		PK:        fmt.Sprintf("user#%s", userID),
		SK:        fmt.Sprintf("mute#%s", tagLower),
		Username:  userID,
		Hashtag:   tagLower,
		CreatedAt: now,
	}
	if until != nil {
		mute.TTL = until.UTC().Unix()
	}

	if err := r.db.WithContext(ctx).Model(mute).Create(); err != nil {
		if errors.IsConditionFailed(err) {
			return nil
		}
		r.logger.Error("failed to mute hashtag",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityHashtag, fmt.Sprintf("mute %s#%s", userID, tagLower))
	}

	return nil
}

// UnmuteHashtag unmutes a hashtag for a user
func (r *HashtagRepository) UnmuteHashtag(ctx context.Context, userID, hashtag string) error {
	tagLower := normalizeHashtagName(hashtag)
	if tagLower == "" {
		return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, EntityHashtag, "empty hashtag")
	}

	mute := &models.HashtagMute{
		PK: fmt.Sprintf("user#%s", userID),
		SK: fmt.Sprintf("mute#%s", tagLower),
	}

	if err := r.db.WithContext(ctx).Model(mute).Delete(); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		r.logger.Error("failed to unmute hashtag",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityHashtag, fmt.Sprintf("mute %s#%s", userID, tagLower))
	}

	return nil
}

// IsHashtagMuted checks if a hashtag is muted for a user
func (r *HashtagRepository) IsHashtagMuted(ctx context.Context, userID, hashtag string) (bool, error) {
	tagLower := normalizeHashtagName(hashtag)
	if tagLower == "" {
		return false, ErrorHandler.HandleGetError(storage.ErrInvalidInput, EntityHashtag, "empty hashtag")
	}

	var mute models.HashtagMute
	err := r.db.WithContext(ctx).Model(&models.HashtagMute{}).
		Where("PK", "=", fmt.Sprintf("user#%s", userID)).
		Where("SK", "=", fmt.Sprintf("mute#%s", tagLower)).
		First(&mute)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to check hashtag mute",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return false, ErrorHandler.HandleGetError(err, EntityHashtag, fmt.Sprintf("mute %s#%s", userID, tagLower))
	}

	if mute.TTL > 0 && time.Now().UTC().Unix() > mute.TTL {
		_ = r.UnmuteHashtag(ctx, userID, tagLower)
		return false, nil
	}

	return true, nil
}

func normalizeHashtagName(hashtag string) string {
	return strings.TrimSpace(strings.ToLower(strings.TrimPrefix(hashtag, "#")))
}

func convertHashtagFollowModel(model *models.HashtagFollow) *storage.HashtagFollow {
	if model == nil {
		return nil
	}

	return &storage.HashtagFollow{
		PK:                   model.PK,
		SK:                   model.SK,
		UserID:               model.UserID,
		Hashtag:              model.Hashtag,
		NotificationsEnabled: model.NotificationsEnabled,
		Muted:                model.Muted,
		CreatedAt:            model.CreatedAt,
		UpdatedAt:            model.UpdatedAt,
	}
}

func convertNotificationFiltersToModel(filters []*storage.NotificationFilter) []models.NotificationFilter {
	if len(filters) == 0 {
		return nil
	}

	result := make([]models.NotificationFilter, len(filters))
	for i, f := range filters {
		if f == nil {
			continue
		}
		result[i] = models.NotificationFilter{
			Types:        append([]string{}, f.Types...),
			AccountID:    f.AccountID,
			MinID:        f.MinID,
			MaxID:        f.MaxID,
			SinceID:      f.SinceID,
			Limit:        f.Limit,
			ExcludeTypes: append([]string{}, f.ExcludeTypes...),
		}
	}
	return result
}

func convertNotificationFiltersToStorage(filters []models.NotificationFilter) []*storage.NotificationFilter {
	if len(filters) == 0 {
		return nil
	}

	result := make([]*storage.NotificationFilter, len(filters))
	for i := range filters {
		f := filters[i]
		result[i] = &storage.NotificationFilter{
			Types:        append([]string{}, f.Types...),
			AccountID:    f.AccountID,
			MinID:        f.MinID,
			MaxID:        f.MaxID,
			SinceID:      f.SinceID,
			Limit:        f.Limit,
			ExcludeTypes: append([]string{}, f.ExcludeTypes...),
		}
	}
	return result
}

func convertHashtagNotificationSettingsModel(model *models.HashtagNotificationSettings) *storage.HashtagNotificationSettings {
	if model == nil {
		return nil
	}

	return &storage.HashtagNotificationSettings{
		PK:         model.PK,
		SK:         model.SK,
		UserID:     model.UserID,
		Hashtag:    model.Hashtag,
		Level:      model.Level,
		Muted:      model.Muted,
		MutedUntil: model.MutedUntil,
		Filters:    convertNotificationFiltersToStorage(model.Filters),
		CreatedAt:  model.CreatedAt,
		UpdatedAt:  model.UpdatedAt,
	}
}

// GetHashtagNotificationSettings retrieves notification preferences for a hashtag
func (r *HashtagRepository) GetHashtagNotificationSettings(ctx context.Context, userID, hashtag string) (*storage.HashtagNotificationSettings, error) {
	tagLower := normalizeHashtagName(hashtag)
	if tagLower == "" {
		return nil, ErrorHandler.HandleGetError(storage.ErrInvalidInput, EntityHashtag, "empty hashtag")
	}

	var model models.HashtagNotificationSettings
	err := r.db.WithContext(ctx).Model(&models.HashtagNotificationSettings{}).
		Where("PK", "=", fmt.Sprintf("user#%s", userID)).
		Where("SK", "=", fmt.Sprintf("settings#%s", tagLower)).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityHashtag, fmt.Sprintf("settings %s#%s", userID, tagLower))
		}
		r.logger.Error("failed to get hashtag notification settings",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityHashtag, fmt.Sprintf("settings %s#%s", userID, tagLower))
	}

	return convertHashtagNotificationSettingsModel(&model), nil
}

// UpdateHashtagNotificationSettings updates notification settings for a hashtag
func (r *HashtagRepository) UpdateHashtagNotificationSettings(ctx context.Context, userID, hashtag string, settings *storage.HashtagNotificationSettings) error {
	if settings == nil {
		return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, EntityHashtag, "nil settings")
	}

	tagLower := normalizeHashtagName(hashtag)
	if tagLower == "" {
		return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, EntityHashtag, "empty hashtag")
	}

	now := time.Now().UTC()
	pk := fmt.Sprintf("user#%s", userID)
	sk := fmt.Sprintf("settings#%s", tagLower)

	var existing models.HashtagNotificationSettings
	err := r.db.WithContext(ctx).Model(&models.HashtagNotificationSettings{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&existing)

	createdAt := now
	if err == nil {
		createdAt = existing.CreatedAt
	} else if !errors.IsNotFound(err) {
		r.logger.Error("failed to load existing hashtag notification settings",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return ErrorHandler.HandleGetError(err, EntityHashtag, fmt.Sprintf("settings %s#%s", userID, tagLower))
	}

	model := &models.HashtagNotificationSettings{
		PK:         pk,
		SK:         sk,
		UserID:     userID,
		Hashtag:    tagLower,
		Level:      settings.Level,
		Muted:      settings.Muted,
		MutedUntil: settings.MutedUntil,
		Filters:    convertNotificationFiltersToModel(settings.Filters),
		CreatedAt:  createdAt,
		UpdatedAt:  now,
	}

	if err = r.db.WithContext(ctx).Model(model).Create(); err != nil {
		// If item already exists, try Update instead (upsert behavior)
		if strings.Contains(strings.ToLower(err.Error()), "already exists") ||
		   strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			r.logger.Debug("hashtag notification settings already exist, updating instead",
				zap.String("user_id", userID),
				zap.String("hashtag", tagLower))
			// Update existing settings
			updateErr := r.db.WithContext(ctx).Model(model).Update()
			if updateErr != nil {
				r.logger.Error("failed to update hashtag notification settings",
					zap.String("user_id", userID),
					zap.String("hashtag", tagLower),
					zap.Error(updateErr))
				return ErrorHandler.HandleUpdateError(updateErr, EntityHashtag, fmt.Sprintf("settings %s#%s", userID, tagLower))
			}
		} else {
			r.logger.Error("failed to update hashtag notification settings",
				zap.String("user_id", userID),
				zap.String("hashtag", tagLower),
				zap.Error(err))
			return ErrorHandler.HandleUpdateError(err, EntityHashtag, fmt.Sprintf("settings %s#%s", userID, tagLower))
		}
	}

	notifyEnabled := !strings.EqualFold(settings.Level, "none") && !settings.Muted
	if err := updateHashtagFollowSetting(ctx, r.db, r.logger, userID, tagLower, HashtagFollowUpdateConfig{
		Operation:   "notification",
		BoolValue:   &notifyEnabled,
		ErrorPrefix: "sync hashtag notification flag",
	}); err != nil {
		r.logger.Debug("failed to sync notification flag on follow record",
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
	}

	return nil
}

// DeleteOldHashtagTrends deletes hashtag trend records older than the specified time
func (r *HashtagRepository) DeleteOldHashtagTrends(ctx context.Context, before time.Time) error {
	r.logger.Info("starting deletion of old hashtag trends",
		zap.Time("before", before))

	// Delete in multiple stages: HashtagTrend models, TrendingHashtag models, and old HashtagUsage
	var totalDeleted int
	var mu sync.Mutex

	// Delete old records using consolidated batch method
	modelTypes := []string{"hashtag_trend", "trending_hashtag", "hashtag_usage"}
	counts := make([]int, len(modelTypes))

	for i, modelType := range modelTypes {
		count, err := r.deleteOldHashtagRecordsBatch(ctx, before, modelType)
		if err != nil {
			r.logger.Error("failed to delete records",
				zap.String("model_type", modelType),
				zap.Error(err))
		} else {
			mu.Lock()
			totalDeleted += count
			counts[i] = count
			mu.Unlock()
		}
	}

	r.logger.Info("completed deletion of old hashtag trends",
		zap.Int("total_deleted", totalDeleted),
		zap.Time("before", before),
		zap.Int("hashtag_trends", counts[0]),
		zap.Int("trending_hashtags", counts[1]),
		zap.Int("usage_records", counts[2]))

	return nil
}

// queryHashtagMetadataByDateRange is a helper to query hashtag metadata within date ranges
func (r *HashtagRepository) queryHashtagMetadataByDateRange(ctx context.Context, startTime *time.Time, endTime *time.Time, limit int) ([]*models.Hashtag, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := r.db.WithContext(ctx).Model(&models.Hashtag{}).
		Filter("SK", "=", models.SKMetadata)

	if startTime != nil {
		query = query.Filter("LastUsed", ">=", startTime.Format(time.RFC3339))
	}
	if endTime != nil {
		query = query.Filter("LastUsed", "<=", endTime.Format(time.RFC3339))
	}

	var hashtagModels []*models.Hashtag
	err := query.OrderBy("LastUsed", "DESC").
		Limit(limit).
		Scan(&hashtagModels)

	if err != nil && !errors.IsNotFound(err) {
		return nil, ErrorHandler.HandleQueryError(err, "hashtag metadata", "date range")
	}

	return hashtagModels, nil
}

// convertHashtagsToTrendingHashtags converts Hashtag models to storage.TrendingHashtag
func (r *HashtagRepository) convertHashtagsToTrendingHashtags(hashtagModels []*models.Hashtag) []*storage.TrendingHashtag {
	result := make([]*storage.TrendingHashtag, len(hashtagModels))
	for i, h := range hashtagModels {
		result[i] = &storage.TrendingHashtag{
			Name:        h.Name,
			URL:         fmt.Sprintf("https://%s/tags/%s", r.domain, h.Name),
			UsageCount:  h.UsageCount,
			UniqueUsers: 0, // Not tracked in basic model
			LastUsed:    h.LastUsed,
			FirstSeen:   h.FirstSeen,
			UserID:      "", // Not applicable for hashtag metadata
			CreatedAt:   h.LastUsed,
		}
	}
	return result
}

// GetRecentHashtags returns hashtags that have been recently used
func (r *HashtagRepository) GetRecentHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	hashtagModels, err := r.queryHashtagMetadataByDateRange(ctx, &since, nil, limit)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "hashtag", "recent")
	}

	return r.convertHashtagsToTrendingHashtags(hashtagModels), nil
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

	// Use the enhanced trending engine for sophisticated trending analysis
	result, err := r.trendingEngine.CalculateTrending(ctx, since, limit)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "hashtag", "trending")
	}

	r.logger.Info("calculated trending hashtags using enhanced engine",
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
		_ = trendModel.UpdateKeys()

		err := r.db.Model(trendModel).Create()
		if err != nil {
			r.logger.Error("failed to store hashtag trend",
				zap.String("hashtag", trend.HashtagName),
				zap.Float64("score", trend.OverallScore),
				zap.Error(err))
			return ErrorHandler.HandleCreateError(err, "hashtag trend", trend.HashtagName)
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
		_ = trendModel.UpdateKeys()

		err := r.db.Model(trendModel).Create()
		if err != nil {
			r.logger.Error("failed to store storage hashtag trend",
				zap.String("hashtag", trend.Name),
				zap.Error(err))
			return ErrorHandler.HandleCreateError(err, "hashtag trend", trend.Name)
		}

	default:
		r.logger.Warn("unknown trend data type for storage",
			zap.String("type", fmt.Sprintf("%T", trendData)))
		return ErrorHandler.HandleGetError(storage.ErrInvalidInput, EntityHashtag, "unsupported trend data type")
	}

	return nil
}

// deleteOldHashtagRecordsBatch consolidates deletion of old hashtag-related records
func (r *HashtagRepository) deleteOldHashtagRecordsBatch(ctx context.Context, before time.Time, modelType string) (int, error) {
	configs := map[string]BatchDeleteConfig{
		"hashtag_trend": {
			ModelType:   "hashtag_trend",
			ErrorPrefix: "hashtag trend records",
			BatchSize:   25,
			QueryLimit:  100,
			FilterField: "UpdatedAt",
		},
		"trending_hashtag": {
			ModelType:   "trending_hashtag",
			ErrorPrefix: "trending hashtag records",
			BatchSize:   25,
			QueryLimit:  100,
			FilterField: "UpdatedAt",
		},
		"hashtag_usage": {
			ModelType:   "hashtag_usage",
			ErrorPrefix: "hashtag usage records",
			BatchSize:   25,
			QueryLimit:  200, // Larger limit for usage cleanup
			FilterField: "UsedAt",
		},
	}

	config, exists := configs[modelType]
	if !exists {
		return 0, ErrorHandler.HandleGetError(storage.ErrInvalidInput, EntityHashtag, "unknown model type")
	}

	return deleteOldRecordsBatch(ctx, r.db, r.logger, before, config)
}

// GetHashtagsByTimeRange retrieves hashtags within a specific time range
func (r *HashtagRepository) GetHashtagsByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	hashtagModels, err := r.queryHashtagMetadataByDateRange(ctx, &startTime, &endTime, limit)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "hashtag", "time range")
	}

	result := r.convertHashtagsToTrendingHashtags(hashtagModels)

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
		return nil, ErrorHandler.HandleQueryError(err, "hashtag trend", "score")
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
	if err := common.ValidateSliceNotEmpty("trends", trends); err != nil {
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
		_ = modelTrend.UpdateKeys()
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
		return ErrorHandler.HandleCreateError(err, "hashtag trend", "batch")
	}

	r.logger.Info("batch created hashtag trends",
		zap.Int("total_items", result.TotalItems),
		zap.Int("processed_items", result.ProcessedItems),
		zap.Int("failed_items", result.FailedItems),
		zap.Duration("duration", result.Duration))

	if result.FailedItems > 0 {
		return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, EntityHashtag, "batch creation failed")
	}

	return nil
}
