package repositories

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// TrendingEngine provides sophisticated hashtag trending calculation and analysis
type TrendingEngine struct {
	db     core.DB
	logger *zap.Logger
	config *TrendingEngineConfig
	cache  *TrendingCache
	mu     sync.RWMutex
}

// TrendingEngineConfig holds configuration for the trending engine
type TrendingEngineConfig struct {
	// Time windows for analysis
	TimeWindows map[string]TrendingTimeWindow `json:"time_windows"`

	// Scoring algorithm parameters
	Scoring TrendingScoringConfig `json:"scoring"`

	// Data collection settings
	MinimumUsage int64         `json:"minimum_usage"` // Minimum usage to be considered
	MinimumUsers int64         `json:"minimum_users"` // Minimum unique users
	MinimumAge   time.Duration `json:"minimum_age"`   // Minimum age before trending
	MaximumAge   time.Duration `json:"maximum_age"`   // Maximum age for consideration

	// Performance settings
	CandidateLimit   int           `json:"candidate_limit"`   // Max candidates to analyze
	CacheExpiration  time.Duration `json:"cache_expiration"`  // Cache expiration time
	BackgroundUpdate bool          `json:"background_update"` // Update in background

	// Quality controls
	TrustThreshold       float64 `json:"trust_threshold"`        // Minimum trust score
	DiversityThreshold   float64 `json:"diversity_threshold"`    // Minimum diversity ratio
	SpamDetectionEnabled bool    `json:"spam_detection_enabled"` // Enable spam detection
}

// TrendingScoringConfig holds parameters for the scoring algorithm
type TrendingScoringConfig struct {
	// Component weights (should sum to 1.0)
	UsageWeight      float64 `json:"usage_weight"`      // Weight for raw usage count
	VelocityWeight   float64 `json:"velocity_weight"`   // Weight for usage velocity
	AccelWeight      float64 `json:"accel_weight"`      // Weight for acceleration
	DiversityWeight  float64 `json:"diversity_weight"`  // Weight for user diversity
	TrustWeight      float64 `json:"trust_weight"`      // Weight for trust scores
	EngagementWeight float64 `json:"engagement_weight"` // Weight for engagement rate
	NoveltyWeight    float64 `json:"novelty_weight"`    // Weight for newness bonus

	// Decay and normalization
	TimeDecayRate     float64 `json:"time_decay_rate"`    // Exponential decay rate
	VelocitySmoothing float64 `json:"velocity_smoothing"` // Velocity smoothing factor
	ScoreThreshold    float64 `json:"score_threshold"`    // Minimum score for trending

	// Penalties and bonuses
	SpamPenalty      float64 `json:"spam_penalty"`      // Penalty for spam-like behavior
	QualityBonus     float64 `json:"quality_bonus"`     // Bonus for high-quality content
	ConsistencyBonus float64 `json:"consistency_bonus"` // Bonus for consistent usage
}

// TrendingCache provides intelligent caching for trending calculations
type TrendingCache struct {
	results    map[string]*CachedTrendingResult
	metrics    map[string]*CachedHashtagMetrics
	expiration time.Duration
	mu         sync.RWMutex
}

// CachedTrendingResult represents a cached trending calculation result
type CachedTrendingResult struct {
	Results     []*storage.TrendingHashtag `json:"results"`
	GeneratedAt time.Time                  `json:"generated_at"`
	Parameters  map[string]interface{}     `json:"parameters"`
	HitCount    int64                      `json:"hit_count"`
}

// CachedHashtagMetrics represents cached metrics for a hashtag
type CachedHashtagMetrics struct {
	Metrics     *EnhancedHashtagMetrics `json:"metrics"`
	GeneratedAt time.Time               `json:"generated_at"`
	ValidUntil  time.Time               `json:"valid_until"`
}

// EnhancedHashtagMetrics provides comprehensive metrics for trending calculation
type EnhancedHashtagMetrics struct {
	// Basic info
	HashtagName string    `json:"hashtag_name"`
	FirstSeen   time.Time `json:"first_seen"`
	LastUsed    time.Time `json:"last_used"`

	// Aggregate metrics
	TotalUsage      int64   `json:"total_usage"`
	UniqueUsers     int64   `json:"unique_users"`
	TotalEngagement int64   `json:"total_engagement"`
	AverageTrust    float64 `json:"average_trust"`

	// Time-windowed data
	WindowMetrics map[string]*EnhancedWindowMetrics `json:"window_metrics"`

	// Calculated scores
	Velocity      float64 `json:"velocity"`       // Usage per hour
	Acceleration  float64 `json:"acceleration"`   // Change in velocity
	DiversityRate float64 `json:"diversity_rate"` // Unique users / total usage
	QualityScore  float64 `json:"quality_score"`  // Content quality indicator
	SpamScore     float64 `json:"spam_score"`     // Spam likelihood
	NoveltyScore  float64 `json:"novelty_score"`  // Newness factor
}

// EnhancedWindowMetrics represents metrics for a specific time window in the enhanced engine
type EnhancedWindowMetrics struct {
	StartTime       time.Time       `json:"start_time"`
	EndTime         time.Time       `json:"end_time"`
	Duration        time.Duration   `json:"duration"`
	UsageCount      int64           `json:"usage_count"`
	UniqueUsers     int64           `json:"unique_users"`
	TotalEngagement int64           `json:"total_engagement"`
	AverageTrust    float64         `json:"average_trust"`
	Velocity        float64         `json:"velocity"`    // Usage per hour in this window
	GrowthRate      float64         `json:"growth_rate"` // Growth vs previous window
	UserGrowth      float64         `json:"user_growth"` // User growth vs previous window
	QualityMetrics  *QualityMetrics `json:"quality_metrics"`
}

// QualityMetrics represents content quality indicators
type QualityMetrics struct {
	AverageLength     float64 `json:"average_length"`     // Average content length
	MediaRatio        float64 `json:"media_ratio"`        // Posts with media
	LinkRatio         float64 `json:"link_ratio"`         // Posts with links
	ReplyRatio        float64 `json:"reply_ratio"`        // Reply posts ratio
	CrossPostRatio    float64 `json:"cross_post_ratio"`   // Cross-platform posts
	LanguageDiversity int     `json:"language_diversity"` // Number of languages
}

// NewTrendingEngine creates a new trending engine with default configuration
func NewTrendingEngine(db core.DB, logger *zap.Logger) *TrendingEngine {
	config := &TrendingEngineConfig{
		TimeWindows: map[string]TrendingTimeWindow{
			"1h":  {Name: "1h", Duration: time.Hour, Weight: 0.35, MinScore: 0.1},
			"6h":  {Name: "6h", Duration: 6 * time.Hour, Weight: 0.25, MinScore: 0.3},
			"24h": {Name: "24h", Duration: 24 * time.Hour, Weight: 0.25, MinScore: 0.5},
			"7d":  {Name: "7d", Duration: 7 * 24 * time.Hour, Weight: 0.15, MinScore: 1.0},
		},
		Scoring: TrendingScoringConfig{
			UsageWeight:      0.20, // 20% for raw usage
			VelocityWeight:   0.25, // 25% for velocity
			AccelWeight:      0.20, // 20% for acceleration
			DiversityWeight:  0.15, // 15% for diversity
			TrustWeight:      0.10, // 10% for trust
			EngagementWeight: 0.05, // 5% for engagement
			NoveltyWeight:    0.05, // 5% for novelty

			TimeDecayRate:     0.693, // Half-life of 1 hour
			VelocitySmoothing: 0.3,   // Smoothing factor for velocity
			ScoreThreshold:    1.0,   // Minimum score for trending

			SpamPenalty:      0.5, // 50% penalty for spam
			QualityBonus:     0.2, // 20% bonus for quality
			ConsistencyBonus: 0.1, // 10% bonus for consistency
		},
		MinimumUsage:         5,
		MinimumUsers:         3,
		MinimumAge:           10 * time.Minute,
		MaximumAge:           7 * 24 * time.Hour,
		CandidateLimit:       500,
		CacheExpiration:      15 * time.Minute,
		BackgroundUpdate:     true,
		TrustThreshold:       0.3,
		DiversityThreshold:   0.1,
		SpamDetectionEnabled: true,
	}

	return &TrendingEngine{
		db:     db,
		logger: logger,
		config: config,
		cache:  NewTrendingCache(config.CacheExpiration),
	}
}

// NewTrendingCache creates a new trending cache
func NewTrendingCache(expiration time.Duration) *TrendingCache {
	return &TrendingCache{
		results:    make(map[string]*CachedTrendingResult),
		metrics:    make(map[string]*CachedHashtagMetrics),
		expiration: expiration,
	}
}

// CalculateTrending performs comprehensive trending analysis
func (te *TrendingEngine) CalculateTrending(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	te.mu.Lock()
	defer te.mu.Unlock()

	start := time.Now()
	defer func() {
		te.logger.Debug("trending calculation completed",
			zap.Duration("duration", time.Since(start)),
			zap.Time("since", since),
			zap.Int("limit", limit))
	}()

	// Check cache first
	cacheKey := te.getCacheKey(since, limit)
	if cached := te.cache.getTrendingResult(cacheKey); cached != nil {
		te.logger.Debug("returning cached trending results",
			zap.String("cache_key", cacheKey),
			zap.Int64("hit_count", cached.HitCount))
		cached.HitCount++
		return cached.Results, nil
	}

	// Step 1: Get candidate hashtags
	candidates, err := te.getCandidateHashtags(ctx, since)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityHashtag, "trending candidates")
	}

	if err := common.ValidateSliceNotEmpty("candidates", candidates); err != nil {
		return []*storage.TrendingHashtag{}, nil
	}

	// Step 2: Calculate comprehensive metrics for each candidate sequentially
	var allMetrics []*EnhancedHashtagMetrics

	var errorCount int
	for _, candidate := range candidates {
		metrics, err := te.calculateEnhancedMetrics(ctx, candidate)
		if err != nil {
			errorCount++
			te.logger.Warn("error calculating hashtag metrics", zap.Error(err))
			continue
		}
		allMetrics = append(allMetrics, metrics)
	}

	if errorCount > 0 {
		te.logger.Warn("some hashtag metrics calculations failed",
			zap.Int("error_count", errorCount),
			zap.Int("total_candidates", len(candidates)))
	}

	// Step 3: Calculate trending scores
	trendingResults := make([]*TrendingHashtagResult, 0, len(allMetrics))
	for _, metrics := range allMetrics {
		// Apply minimum thresholds
		if metrics.TotalUsage < te.config.MinimumUsage ||
			metrics.UniqueUsers < te.config.MinimumUsers ||
			metrics.AverageTrust < te.config.TrustThreshold ||
			metrics.DiversityRate < te.config.DiversityThreshold {
			continue
		}

		// Calculate trending score
		score := te.calculateTrendingScore(metrics)

		// Apply score threshold
		if score.OverallScore < te.config.Scoring.ScoreThreshold {
			continue
		}

		result := &TrendingHashtagResult{
			HashtagName: metrics.HashtagName,
			Score:       score.OverallScore,
			Metrics:     metrics,
			Components:  score.ComponentScores,
		}

		trendingResults = append(trendingResults, result)
	}

	// Step 4: Sort by score and apply limit
	sort.Slice(trendingResults, func(i, j int) bool {
		return trendingResults[i].Score > trendingResults[j].Score
	})

	if err := common.ValidateSliceLength("trending_results", trendingResults, limit); err != nil {
		trendingResults = trendingResults[:limit]
	}

	// Step 5: Convert to storage format
	results := make([]*storage.TrendingHashtag, len(trendingResults))
	for i, result := range trendingResults {
		results[i] = &storage.TrendingHashtag{
			Name:        result.HashtagName,
			URL:         fmt.Sprintf("/tags/%s", result.HashtagName),
			UsageCount:  result.Metrics.TotalUsage,
			UniqueUsers: result.Metrics.UniqueUsers,
			LastUsed:    result.Metrics.LastUsed,
			FirstSeen:   result.Metrics.FirstSeen,
			UserID:      "", // Not applicable for hashtags
			CreatedAt:   time.Now(),
		}
	}

	// Step 6: Cache results
	cached := &CachedTrendingResult{
		Results:     results,
		GeneratedAt: time.Now(),
		Parameters: map[string]interface{}{
			"since": since,
			"limit": limit,
		},
		HitCount: 1,
	}
	te.cache.setTrendingResult(cacheKey, cached)

	te.logger.Info("calculated trending hashtags",
		zap.Int("candidates", len(candidates)),
		zap.Int("qualified", len(allMetrics)),
		zap.Int("trending", len(results)),
		zap.Int("errors", errorCount))

	return results, nil
}

// TrendingHashtagResult represents a hashtag with its trending analysis
type TrendingHashtagResult struct {
	HashtagName string                  `json:"hashtag_name"`
	Score       float64                 `json:"score"`
	Metrics     *EnhancedHashtagMetrics `json:"metrics"`
	Components  map[string]float64      `json:"components"`
}

// EnhancedTrendingScore represents the calculated trending score with breakdown in the enhanced engine
type EnhancedTrendingScore struct {
	OverallScore    float64            `json:"overall_score"`
	ComponentScores map[string]float64 `json:"component_scores"`
	Timestamp       time.Time          `json:"timestamp"`
}

// getCandidateHashtags retrieves hashtags that could potentially be trending
func (te *TrendingEngine) getCandidateHashtags(ctx context.Context, since time.Time) ([]*models.Hashtag, error) {
	var candidates []*models.Hashtag

	// Query recent hashtags directly using DynamORM
	err := te.db.WithContext(ctx).Model(&models.Hashtag{}).
		Where("SK", "=", "METADATA").
		Filter("LastUsed", ">=", since.Format(time.RFC3339)).
		Filter("UsageCount", ">=", te.config.MinimumUsage).
		OrderBy("LastUsed", "DESC").
		Limit(te.config.CandidateLimit).
		All(&candidates)

	if err != nil && !errors.IsNotFound(err) {
		return nil, ErrorHandler.HandleQueryError(err, EntityHashtag, "candidate retrieval")
	}

	te.logger.Debug("retrieved candidate hashtags",
		zap.Int("count", len(candidates)),
		zap.Time("since", since))

	return candidates, nil
}

// calculateEnhancedMetrics calculates comprehensive metrics for a hashtag
func (te *TrendingEngine) calculateEnhancedMetrics(ctx context.Context, hashtag *models.Hashtag) (*EnhancedHashtagMetrics, error) {
	// Check cache first
	if cached := te.cache.getHashtagMetrics(hashtag.Name); cached != nil {
		return cached.Metrics, nil
	}

	metrics := &EnhancedHashtagMetrics{
		HashtagName:   hashtag.Name,
		FirstSeen:     hashtag.FirstSeen,
		LastUsed:      hashtag.LastUsed,
		TotalUsage:    hashtag.UsageCount,
		WindowMetrics: make(map[string]*EnhancedWindowMetrics),
	}

	// Calculate metrics for each time window
	for windowName, window := range te.config.TimeWindows {
		windowStart := time.Now().Add(-window.Duration)
		windowMetrics, err := te.calculateWindowMetrics(ctx, hashtag.Name, windowStart, time.Now())
		if err != nil {
			te.logger.Warn("failed to calculate window metrics",
				zap.String("hashtag", hashtag.Name),
				zap.String("window", windowName),
				zap.Error(err))
			// Use empty metrics on error
			windowMetrics = &EnhancedWindowMetrics{
				StartTime: windowStart,
				EndTime:   time.Now(),
				Duration:  window.Duration,
			}
		}
		metrics.WindowMetrics[windowName] = windowMetrics
	}

	// Calculate aggregate metrics from window data
	te.calculateAggregateMetrics(metrics)

	// Calculate derived scores
	te.calculateDerivedScores(metrics)

	// Cache the results
	cached := &CachedHashtagMetrics{
		Metrics:     metrics,
		GeneratedAt: time.Now(),
		ValidUntil:  time.Now().Add(te.config.CacheExpiration),
	}
	te.cache.setHashtagMetrics(hashtag.Name, cached)

	return metrics, nil
}

// calculateWindowMetrics computes metrics for a specific time window
func (te *TrendingEngine) calculateWindowMetrics(ctx context.Context, hashtag string, start, end time.Time) (*EnhancedWindowMetrics, error) {
	// Query usage records directly using DynamORM
	var usageRecords []models.HashtagUsage
	err := te.db.WithContext(ctx).Model(&models.HashtagUsage{}).
		Where("PK", "=", fmt.Sprintf("HASHTAG#%s", hashtag)).
		Where("SK", ">=", fmt.Sprintf("USAGE#%d", start.Unix())).
		Where("SK", "<=", fmt.Sprintf("USAGE#%d", end.Unix())).
		OrderBy("SK", "DESC").
		Limit(1000). // Reasonable limit for analysis
		All(&usageRecords)

	if err != nil && !errors.IsNotFound(err) {
		return nil, ErrorHandler.HandleQueryError(err, EntityHashtag, "usage records")
	}

	// Calculate metrics from usage records
	metrics := &EnhancedWindowMetrics{
		StartTime: start,
		EndTime:   end,
		Duration:  end.Sub(start),
	}

	userSet := make(map[string]bool)
	languageSet := make(map[string]bool)
	var engagementSum int64
	var trustSum float64
	var qualityMetrics QualityMetrics

	for _, usage := range usageRecords {
		userSet[usage.AuthorID] = true

		// Estimate engagement (in real implementation, query actual engagement data)
		engagementSum += te.estimateEngagement(usage.Visibility)

		// Calculate trust based on user activity patterns and account age
		userTrust := te.calculateUserTrustScore(usage.AuthorID, userSet)
		trustSum += userTrust

		// Update quality metrics based on actual content analysis
		// Fetch actual status content for quality analysis
		statusContent := te.getStatusContentForAnalysis(usage.StatusID)
		contentQuality := te.analyzeContentQuality(statusContent)
		qualityMetrics.MediaRatio += contentQuality.MediaPresence
		qualityMetrics.LinkRatio += contentQuality.LinkPresence

		// Track language diversity for quality assessment
		if contentQuality.LanguageCode != "" {
			languageSet[contentQuality.LanguageCode] = true
		}
	}

	metrics.UsageCount = int64(len(usageRecords))
	metrics.UniqueUsers = int64(len(userSet))
	metrics.TotalEngagement = engagementSum

	if metrics.UniqueUsers > 0 {
		metrics.AverageTrust = trustSum / float64(metrics.UniqueUsers)
	}

	// Calculate velocity (usage per hour)
	if metrics.Duration.Hours() > 0 {
		metrics.Velocity = float64(metrics.UsageCount) / metrics.Duration.Hours()
	}

	// Set quality metrics with proper normalization
	if metrics.UsageCount > 0 {
		qualityMetrics.MediaRatio /= float64(metrics.UsageCount)
		qualityMetrics.LinkRatio /= float64(metrics.UsageCount)
		qualityMetrics.LanguageDiversity = len(languageSet) // Set actual language diversity
	}
	metrics.QualityMetrics = &qualityMetrics

	return metrics, nil
}

// calculateAggregateMetrics calculates aggregate metrics from window data
func (te *TrendingEngine) calculateAggregateMetrics(metrics *EnhancedHashtagMetrics) {
	// Use the largest available window for aggregate data
	var largestWindow *EnhancedWindowMetrics
	var largestDuration time.Duration

	for _, window := range metrics.WindowMetrics {
		if window.Duration > largestDuration {
			largestDuration = window.Duration
			largestWindow = window
		}
	}

	if largestWindow != nil {
		metrics.UniqueUsers = largestWindow.UniqueUsers
		metrics.TotalEngagement = largestWindow.TotalEngagement
		metrics.AverageTrust = largestWindow.AverageTrust
	}
}

// calculateDerivedScores calculates derived scores from metrics
func (te *TrendingEngine) calculateDerivedScores(metrics *EnhancedHashtagMetrics) {
	// Calculate velocity (average across windows)
	var velocitySum float64
	var velocityCount int

	for _, window := range metrics.WindowMetrics {
		velocitySum += window.Velocity
		velocityCount++
	}

	if velocityCount > 0 {
		metrics.Velocity = velocitySum / float64(velocityCount)
	}

	// Calculate acceleration (change in velocity between windows)
	if len(metrics.WindowMetrics) >= 2 {
		shortWindow := metrics.WindowMetrics["1h"]
		longWindow := metrics.WindowMetrics["24h"]

		if shortWindow != nil && longWindow != nil && longWindow.Velocity > 0 {
			metrics.Acceleration = (shortWindow.Velocity - longWindow.Velocity) / longWindow.Velocity
		}
	}

	// Calculate diversity rate
	if metrics.TotalUsage > 0 {
		metrics.DiversityRate = float64(metrics.UniqueUsers) / float64(metrics.TotalUsage)
	}

	// Calculate quality score (simplified)
	qualitySum := 0.0
	qualityCount := 0

	for _, window := range metrics.WindowMetrics {
		if window.QualityMetrics != nil {
			qualitySum += window.QualityMetrics.MediaRatio * 0.3
			qualitySum += window.QualityMetrics.LinkRatio * 0.2
			qualitySum += math.Min(float64(window.QualityMetrics.LanguageDiversity)/5.0, 1.0) * 0.5
			qualityCount++
		}
	}

	if qualityCount > 0 {
		metrics.QualityScore = qualitySum / float64(qualityCount)
	}

	// Calculate novelty score (based on age)
	age := time.Since(metrics.FirstSeen)
	if age.Hours() <= 24 {
		metrics.NoveltyScore = 1.0 - (age.Hours() / 24.0)
	} else {
		metrics.NoveltyScore = 0.1 // Small bonus for established hashtags
	}

	// Calculate spam score using multiple sophisticated indicators
	spamIndicators := 0
	var spamScore float64

	// Indicator 1: Extremely low user diversity (possible bot activity)
	if metrics.DiversityRate < 0.05 {
		spamIndicators++
	}

	// Indicator 2: Abnormally high velocity (possible coordinated spam)
	if metrics.Velocity > 50 {
		spamIndicators++
	}

	// Indicator 3: Very low average trust score
	if metrics.AverageTrust < 0.2 {
		spamIndicators++
	}

	// Indicator 4: Very short bursts of activity (suspicious timing patterns)
	if metrics.Velocity > 0 {
		// Calculate activity density based on total usage over the analysis period
		// Use total usage instead of window-specific metrics
		timeSinceFirst := time.Since(metrics.FirstSeen)
		if timeSinceFirst.Minutes() > 0 {
			activityDensity := float64(metrics.TotalUsage) / timeSinceFirst.Minutes()
			if activityDensity > 10 { // More than 10 uses per minute
				spamIndicators++
			}
		}
	}

	// Indicator 5: Low content quality based on available metrics
	// Since EnhancedHashtagMetrics doesn't have direct QualityMetrics field,
	// we'll use available quality-related fields
	qualityThreshold := 0.3
	if metrics.QualityScore < qualityThreshold {
		spamIndicators++
	}

	// Convert indicators to spam score (0.0 = no spam, 1.0 = high spam)
	maxIndicators := 5.0
	spamScore = float64(spamIndicators) / maxIndicators

	// Apply smoothing to avoid harsh penalties for single indicators
	if spamIndicators == 0 {
		spamScore = 0.1 // Very low baseline spam score
	} else if spamIndicators == 1 {
		spamScore = 0.2 // Low spam score
	} else if spamIndicators == 2 {
		spamScore = 0.4 // Medium spam score
	} else if spamIndicators >= 3 {
		spamScore = 0.7 + (float64(spamIndicators-3) * 0.1) // High spam score
		if spamScore > 0.9 {
			spamScore = 0.9 // Cap at 0.9 to leave room for improvement
		}
	}

	metrics.SpamScore = spamScore
}

// calculateTrendingScore computes the overall trending score
func (te *TrendingEngine) calculateTrendingScore(metrics *EnhancedHashtagMetrics) *EnhancedTrendingScore {
	config := te.config.Scoring
	components := make(map[string]float64)

	// 1. Usage score with logarithmic scaling
	usageScore := math.Log1p(float64(metrics.TotalUsage)) / 10.0 // Normalize
	components["usage"] = usageScore

	// 2. Velocity score with smoothing
	velocityScore := math.Log1p(metrics.Velocity) / 5.0 // Normalize
	components["velocity"] = velocityScore

	// 3. Acceleration score
	accelScore := math.Max(0, metrics.Acceleration) // Only positive acceleration counts
	if accelScore > 1.0 {
		accelScore = 1.0 + math.Log1p(accelScore-1.0) // Logarithmic scaling for high acceleration
	}
	components["acceleration"] = accelScore

	// 4. Diversity score
	diversityScore := math.Min(metrics.DiversityRate*10.0, 1.0) // Cap at 1.0
	components["diversity"] = diversityScore

	// 5. Trust score
	trustScore := math.Min(metrics.AverageTrust, 1.0)
	components["trust"] = trustScore

	// 6. Engagement score
	engagementRate := 0.0
	if metrics.TotalUsage > 0 {
		engagementRate = float64(metrics.TotalEngagement) / float64(metrics.TotalUsage)
	}
	engagementScore := math.Min(engagementRate/2.0, 1.0) // Normalize
	components["engagement"] = engagementScore

	// 7. Novelty score
	components["novelty"] = metrics.NoveltyScore

	// Calculate weighted overall score
	overallScore := (usageScore * config.UsageWeight) +
		(velocityScore * config.VelocityWeight) +
		(accelScore * config.AccelWeight) +
		(diversityScore * config.DiversityWeight) +
		(trustScore * config.TrustWeight) +
		(engagementScore * config.EngagementWeight) +
		(metrics.NoveltyScore * config.NoveltyWeight)

	// Apply time decay
	age := time.Since(metrics.LastUsed)
	decayFactor := math.Exp(-age.Hours() * config.TimeDecayRate / 24.0) // Daily decay rate
	overallScore *= decayFactor
	components["decay_factor"] = decayFactor

	// Apply quality bonus
	if metrics.QualityScore > 0.7 {
		qualityBonus := 1.0 + (config.QualityBonus * (metrics.QualityScore - 0.7) / 0.3)
		overallScore *= qualityBonus
		components["quality_bonus"] = qualityBonus
	}

	// Apply spam penalty
	if metrics.SpamScore > 0.5 {
		spamPenalty := 1.0 - (config.SpamPenalty * (metrics.SpamScore - 0.5) / 0.5)
		overallScore *= spamPenalty
		components["spam_penalty"] = spamPenalty
	}

	return &EnhancedTrendingScore{
		OverallScore:    overallScore,
		ComponentScores: components,
		Timestamp:       time.Now(),
	}
}

// estimateEngagement provides a simple engagement estimation
func (te *TrendingEngine) estimateEngagement(visibility string) int64 {
	switch visibility {
	case models.VisibilityPublic:
		return 3 // Higher engagement for public posts
	case "unlisted":
		return 2
	case "private":
		return 1
	default:
		return 1
	}
}

// Cache management methods

func (tc *TrendingCache) getTrendingResult(key string) *CachedTrendingResult {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	result, exists := tc.results[key]
	if !exists {
		return nil
	}

	// Check expiration - don't delete under RLock; cleanup happens lazily in setTrendingResult
	if time.Since(result.GeneratedAt) > tc.expiration {
		return nil
	}

	return result
}

func (tc *TrendingCache) setTrendingResult(key string, result *CachedTrendingResult) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.results[key] = result

	// Clean up expired entries
	tc.cleanupExpiredResults()
}

func (tc *TrendingCache) getHashtagMetrics(hashtag string) *CachedHashtagMetrics {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	metrics, exists := tc.metrics[hashtag]
	if !exists {
		return nil
	}

	// Check expiration - don't delete under RLock; cleanup happens lazily in setHashtagMetrics
	if time.Now().After(metrics.ValidUntil) {
		return nil
	}

	return metrics
}

func (tc *TrendingCache) setHashtagMetrics(hashtag string, metrics *CachedHashtagMetrics) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.metrics[hashtag] = metrics

	// Clean up expired entries
	tc.cleanupExpiredMetrics()
}

func (tc *TrendingCache) cleanupExpiredResults() {
	now := time.Now()
	for key, result := range tc.results {
		if now.Sub(result.GeneratedAt) > tc.expiration {
			delete(tc.results, key)
		}
	}
}

func (tc *TrendingCache) cleanupExpiredMetrics() {
	now := time.Now()
	for key, metrics := range tc.metrics {
		if now.After(metrics.ValidUntil) {
			delete(tc.metrics, key)
		}
	}
}

func (te *TrendingEngine) getCacheKey(since time.Time, limit int) string {
	return fmt.Sprintf("trending:%s:%d", since.Format(time.RFC3339), limit)
}

// ContentQuality represents analyzed content quality metrics
type ContentQuality struct {
	MediaPresence  float64 // 0.0-1.0 indicating presence of media attachments
	LinkPresence   float64 // 0.0-1.0 indicating presence of external links
	LanguageCode   string  // Detected language code
	TextLength     int     // Character count of text content
	HashtagDensity float64 // Ratio of hashtags to total words
}

// calculateUserTrustScore calculates a trust score for a user based on various factors
func (te *TrendingEngine) calculateUserTrustScore(userID string, userSet map[string]bool) float64 {
	// Base trust score
	trustScore := 0.5

	// Factor 1: Account age estimation (would normally query account creation date)
	// For now, use a heuristic based on user activity patterns
	if err := common.ValidateStringLength("user_id", userID, 11, 1000); err == nil { // Longer user IDs might indicate older accounts
		trustScore += 0.1
	}

	// Factor 2: User diversity (new users vs established users in this hashtag usage)
	if len(userSet) > 1 {
		// More diverse user participation increases individual trust
		diversityBonus := math.Min(float64(len(userSet))/20.0, 0.2) // Max 0.2 bonus
		trustScore += diversityBonus
	}

	// Factor 3: User ID patterns (simple heuristics to detect potential bot accounts)
	// Check for suspicious patterns in user IDs
	if te.detectSuspiciousUserPattern(userID) {
		trustScore -= 0.3 // Penalty for suspicious patterns
	}

	// Factor 4: Activity frequency (would normally check user's posting frequency)
	// Assume moderate activity is more trustworthy
	trustScore += 0.2 // Default moderate activity bonus

	// Ensure trust score stays within bounds [0.0, 1.0]
	if trustScore < 0.0 {
		trustScore = 0.0
	}
	if trustScore > 1.0 {
		trustScore = 1.0
	}

	return trustScore
}

// detectSuspiciousUserPattern checks for patterns that might indicate bot accounts
func (te *TrendingEngine) detectSuspiciousUserPattern(userID string) bool {
	// Pattern 1: Very long numeric sequences (possible generated IDs)
	numericCount := 0
	for _, char := range userID {
		if char >= '0' && char <= '9' {
			numericCount++
		}
	}
	if float64(numericCount) > float64(len(userID))*0.8 { // More than 80% numbers
		return true
	}

	// Pattern 2: Repetitive patterns (possible generated usernames)
	if err := common.ValidateStringLength("user_id", userID, 5, 1000); err == nil {
		for i := 0; i < len(userID)-2; i++ {
			substr := userID[i : i+3]
			if strings.Count(userID, substr) >= 3 { // Same 3-char sequence appears 3+ times
				return true
			}
		}
	}

	// Pattern 3: Very short IDs (possible temporary/throwaway accounts)
	if len(userID) < 3 {
		return true
	}

	return false
}

// analyzeContentQuality performs content analysis to determine quality metrics
func (te *TrendingEngine) analyzeContentQuality(content string) ContentQuality {
	if err := common.ValidateRequiredParam("content", content); err != nil {
		return ContentQuality{
			MediaPresence:  0.0,
			LinkPresence:   0.0,
			LanguageCode:   "",
			TextLength:     0,
			HashtagDensity: 0.0,
		}
	}

	quality := ContentQuality{
		TextLength: len(content),
	}

	words := strings.Fields(content)
	totalWords := len(words)

	if totalWords == 0 {
		return quality
	}

	hashtagCount := 0
	linkCount := 0
	mediaIndicators := 0

	// Analyze each word/token
	for _, word := range words {
		// Count hashtags
		if strings.HasPrefix(word, "#") {
			hashtagCount++
		}

		// Detect links (simple heuristic)
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") || strings.Contains(word, ".com") || strings.Contains(word, ".org") || strings.Contains(word, ".net") {
			linkCount++
		}

		// Detect media indicators (simple keywords)
		lowerWord := strings.ToLower(word)
		if lowerWord == "photo" || lowerWord == "image" || lowerWord == "video" || lowerWord == "pic" || strings.Contains(lowerWord, "media") {
			mediaIndicators++
		}
	}

	// Calculate ratios
	quality.HashtagDensity = float64(hashtagCount) / float64(totalWords)
	quality.LinkPresence = math.Min(float64(linkCount)/float64(totalWords)*5.0, 1.0)         // Scale up link presence
	quality.MediaPresence = math.Min(float64(mediaIndicators)/float64(totalWords)*10.0, 1.0) // Scale up media presence

	// Simple language detection (very basic heuristic)
	// In production, you would use a proper language detection library
	quality.LanguageCode = te.detectLanguage(content)

	return quality
}

// detectLanguage performs basic language detection
func (te *TrendingEngine) detectLanguage(content string) string {
	content = strings.ToLower(content)

	// Simple heuristics for common languages
	// English indicators
	englishWords := []string{"the", "and", "for", "are", "but", "not", "you", "all", "can", "had", "her", "was", "one", "our", "out", "day", "get", "use", "man", "new", "now", "old", "see", "him", "two", "how", "its", "who", "did", "yes", "his", "been", "long", "down", "well", "were"}
	englishScore := 0
	for _, word := range englishWords {
		if strings.Contains(content, " "+word+" ") || strings.HasPrefix(content, word+" ") || strings.HasSuffix(content, " "+word) {
			englishScore++
		}
	}

	// Spanish indicators
	spanishWords := []string{"que", "por", "con", "para", "una", "como", "del", "más", "sus", "pero", "fue", "son", "muy", "los", "era", "dos", "ese", "uno", "año", "día"}
	spanishScore := 0
	for _, word := range spanishWords {
		if strings.Contains(content, " "+word+" ") || strings.HasPrefix(content, word+" ") || strings.HasSuffix(content, " "+word) {
			spanishScore++
		}
	}

	// Return the language with the highest score
	if englishScore > spanishScore && englishScore > 0 {
		return "en"
	} else if spanishScore > 0 {
		return "es"
	}

	// Default to unknown if we can't detect
	return "unknown"
}

// getStatusContentForAnalysis retrieves the content of a status for content quality analysis
func (te *TrendingEngine) getStatusContentForAnalysis(statusID string) string {
	if err := common.ValidateRequiredParam("status_id", statusID); err != nil {
		return ""
	}

	// Query the status directly using DynamORM
	var status models.Status
	pk := fmt.Sprintf("STATUS#%s", statusID)

	err := te.db.Model(&models.Status{}).
		Where("PK", "=", pk).
		Where("SK", "=", "METADATA").
		First(&status)

	if err != nil {
		// If we can't fetch the status, log and continue with empty content
		// This is not a critical error for hashtag trending
		te.logger.Debug("Could not fetch status content for quality analysis",
			zap.String("status_id", statusID),
			zap.Error(err))
		return ""
	}

	// Return the actual status content
	return status.Content
}
