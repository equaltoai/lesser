package repositories

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/batch"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// getCandidateHashtags retrieves hashtags that could potentially be trending
func (r *HashtagRepository) getCandidateHashtags(ctx context.Context, since time.Time, limit int) ([]*models.Hashtag, error) {
	// Get recently active hashtags with decent usage
	var candidates []*models.Hashtag
	err := r.db.WithContext(ctx).Model(&models.Hashtag{}).
		Where("SK", "=", "METADATA").
		Filter("LastUsed", ">=", since.Format(time.RFC3339)).
		Filter("UsageCount", ">=", r.trendingCalculator.config.MinimumUsage).
		OrderBy("LastUsed", "DESC").
		Limit(limit).
		Scan(&candidates)

	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get candidate hashtags: %w", err)
	}

	return candidates, nil
}

// calculateTrendingScores computes sophisticated trending scores for hashtag candidates
func (r *HashtagRepository) calculateTrendingScores(ctx context.Context, candidates []*models.Hashtag) ([]*TrendingScore, error) {
	scores := make([]*TrendingScore, 0, len(candidates))

	for _, hashtag := range candidates {
		// Gather comprehensive metrics for this hashtag
		metrics, err := r.gatherHashtagMetrics(ctx, hashtag)
		if err != nil {
			r.logger.Warn("failed to gather metrics for hashtag",
				zap.String("hashtag", hashtag.Name),
				zap.Error(err))
			continue
		}

		// Skip if doesn't meet minimum requirements
		if metrics.UniqueUsers < r.trendingCalculator.config.MinimumUsers {
			continue
		}

		// Calculate trending score using sophisticated algorithm
		score := r.trendingCalculator.CalculateTrendingScore(metrics)

		// Only include if score meets threshold
		if score.OverallScore >= r.trendingCalculator.config.TrendingThreshold {
			scores = append(scores, score)
		}
	}

	return scores, nil
}

// gatherHashtagMetrics collects comprehensive metrics for trending calculation
func (r *HashtagRepository) gatherHashtagMetrics(ctx context.Context, hashtag *models.Hashtag) (*TrendingMetrics, error) {
	metrics := &TrendingMetrics{
		HashtagName:     hashtag.Name,
		TotalUsage:      hashtag.UsageCount,
		UniqueUsers:     0, // Will be calculated
		Engagements:     0, // Will be calculated
		TrustScore:      0.5, // Default trust score
		FirstSeen:       hashtag.FirstSeen,
		LastUsed:        hashtag.LastUsed,
		TimeWindowData:  make(map[string]*WindowMetrics),
		HistoricalTrend: make([]float64, 0),
		MomentumScore:   0.0,
	}

	// Calculate metrics for each time window
	now := time.Now()
	for _, window := range r.trendingCalculator.config.TimeWindows {
		windowStart := now.Add(-window.Duration)
		windowMetrics, err := r.calculateWindowMetrics(ctx, hashtag.Name, windowStart, now)
		if err != nil {
			r.logger.Warn("failed to calculate window metrics",
				zap.String("hashtag", hashtag.Name),
				zap.String("window", window.Name),
				zap.Error(err))
			// Use default values on error
			windowMetrics = &WindowMetrics{}
		}
		metrics.TimeWindowData[window.Name] = windowMetrics
	}

	// Calculate overall unique users and engagements from largest window
	if largestWindow := metrics.TimeWindowData["7d"]; largestWindow != nil {
		metrics.UniqueUsers = largestWindow.UniqueUsers
		metrics.Engagements = largestWindow.Engagements
		metrics.TrustScore = largestWindow.AverageTrust
	} else if mediumWindow := metrics.TimeWindowData["24h"]; mediumWindow != nil {
		metrics.UniqueUsers = mediumWindow.UniqueUsers
		metrics.Engagements = mediumWindow.Engagements
		metrics.TrustScore = mediumWindow.AverageTrust
	}

	// Calculate momentum (rate of change)
	metrics.MomentumScore = r.calculateMomentum(metrics)

	// Get historical trend data
	history, _ := r.GetHashtagUsageHistory(ctx, hashtag.Name, 7)
	metrics.HistoricalTrend = make([]float64, len(history))
	for i, count := range history {
		metrics.HistoricalTrend[i] = float64(count)
	}

	return metrics, nil
}

// calculateWindowMetrics computes metrics for a specific time window
func (r *HashtagRepository) calculateWindowMetrics(ctx context.Context, hashtag string, start, end time.Time) (*WindowMetrics, error) {
	// Query usage records in the time window
	var usageRecords []*models.HashtagUsage
	err := r.db.WithContext(ctx).Model(&models.HashtagUsage{}).
		Where("PK", "=", fmt.Sprintf("HASHTAG#%s", strings.ToLower(hashtag))).
		Where("SK", ">=", fmt.Sprintf("USAGE#%d", start.Unix())).
		Where("SK", "<=", fmt.Sprintf("USAGE#%d", end.Unix())).
		All(&usageRecords)

	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get usage records: %w", err)
	}

	// Calculate metrics from usage records
	userSet := make(map[string]bool)
	var totalEngagements int64
	var trustSum float64
	usageCount := int64(len(usageRecords))

	for _, usage := range usageRecords {
		userSet[usage.AuthorID] = true
		// Note: In a full implementation, we'd query for actual engagement metrics
		// For now, we estimate based on visibility and usage patterns
		if usage.Visibility == VisibilityPublic {
			totalEngagements += 2 // Estimated engagements for public posts
		} else {
			totalEngagements++
		}
		// Default trust score per user (would be calculated from actual trust data)
		trustSum += 0.7
	}

	uniqueUsers := int64(len(userSet))
	velocity := float64(usageCount) / end.Sub(start).Hours() // Usage per hour

	// Calculate growth rate comparing first and second half of window
	midpoint := start.Add(end.Sub(start) / 2)
	firstHalfCount := int64(0)
	secondHalfCount := int64(0)

	for _, usage := range usageRecords {
		if usage.UsedAt.Before(midpoint) {
			firstHalfCount++
		} else {
			secondHalfCount++
		}
	}

	var growthRate float64
	if firstHalfCount > 0 {
		growthRate = float64(secondHalfCount-firstHalfCount) / float64(firstHalfCount)
	} else if secondHalfCount > 0 {
		growthRate = 1.0 // 100% growth from zero
	}

	var averageTrust float64
	if uniqueUsers > 0 {
		averageTrust = trustSum / float64(uniqueUsers)
	} else {
		averageTrust = 0.5 // Default
	}

	return &WindowMetrics{
		UsageCount:   usageCount,
		UniqueUsers:  uniqueUsers,
		Engagements:  totalEngagements,
		AverageTrust: averageTrust,
		GrowthRate:   growthRate,
		Velocity:     velocity,
	}, nil
}

// calculateMomentum computes the momentum score based on time window data
func (r *HashtagRepository) calculateMomentum(metrics *TrendingMetrics) float64 {
	// Compare velocity across different time windows to measure acceleration
	windows := []string{"1h", "6h", "24h"}
	velocities := make([]float64, 0, len(windows))

	for _, window := range windows {
		if windowData, exists := metrics.TimeWindowData[window]; exists {
			velocities = append(velocities, windowData.Velocity)
		}
	}

	if len(velocities) < 2 {
		return 0.0
	}

	// Calculate acceleration (change in velocity)
	var acceleration float64
	for i := 1; i < len(velocities); i++ {
		if velocities[i-1] > 0 {
			acceleration += (velocities[i-1] - velocities[i]) / velocities[i-1]
		}
	}

	return acceleration / float64(len(velocities)-1) // Average acceleration
}

// CalculateTrendingScore computes the overall trending score using multiple factors
func (tc *TrendingCalculator) CalculateTrendingScore(metrics *TrendingMetrics) *TrendingScore {
	now := time.Now()
	componentScores := make(map[string]float64)

	// 1. Usage Score - normalized and time-weighted
	usageScore := tc.calculateUsageScore(metrics, now)
	componentScores["usage"] = usageScore

	// 2. Engagement Score
	engagementScore := tc.calculateEngagementScore(metrics)
	componentScores["engagement"] = engagementScore

	// 3. Diversity Score - user diversity indicates broader appeal
	diversityScore := tc.calculateDiversityScore(metrics)
	componentScores["diversity"] = diversityScore

	// 4. Trust Score - higher trust users indicate quality content
	trustScore := tc.calculateTrustScore(metrics)
	componentScores["trust"] = trustScore

	// 5. Momentum Score - trending velocity and acceleration
	momentumScore := tc.calculateMomentumScore(metrics)
	componentScores["momentum"] = momentumScore

	// 6. Multi-window weighted score
	windowScore := tc.calculateMultiWindowScore(metrics)
	componentScores["windows"] = windowScore

	// Calculate overall weighted score
	overallScore := (usageScore * tc.config.UsageWeight) +
		(engagementScore * tc.config.EngagementWeight) +
		(diversityScore * tc.config.DiversityWeight) +
		(trustScore * tc.config.TrustWeight) +
		(momentumScore * tc.config.MomentumWeight) +
		(windowScore * 0.1) // Small boost for multi-window consistency

	return &TrendingScore{
		HashtagName:     metrics.HashtagName,
		OverallScore:    overallScore,
		ComponentScores: componentScores,
		Metrics:         metrics,
		Rank:            0, // Will be set after sorting
		Timestamp:       now,
	}
}

// calculateUsageScore computes time-weighted usage score with exponential decay
func (tc *TrendingCalculator) calculateUsageScore(metrics *TrendingMetrics, now time.Time) float64 {
	// Age factor - exponential decay based on last use
	age := now.Sub(metrics.LastUsed)
	ageFactor := math.Exp(-age.Hours() / tc.config.DecayHalfLife.Hours())

	// Base usage score with logarithmic scaling to prevent huge hashtags from dominating
	baseScore := math.Log1p(float64(metrics.TotalUsage))

	return baseScore * ageFactor
}

// calculateEngagementScore computes engagement-based score
func (tc *TrendingCalculator) calculateEngagementScore(metrics *TrendingMetrics) float64 {
	if metrics.TotalUsage == 0 {
		return 0.0
	}

	// Engagement rate = engagements per usage
	engagementRate := float64(metrics.Engagements) / float64(metrics.TotalUsage)
	return math.Log1p(engagementRate)
}

// calculateDiversityScore computes user diversity score
func (tc *TrendingCalculator) calculateDiversityScore(metrics *TrendingMetrics) float64 {
	if metrics.TotalUsage == 0 {
		return 0.0
	}

	// Diversity = unique users / total usage (higher is better)
	diversityRatio := float64(metrics.UniqueUsers) / float64(metrics.TotalUsage)

	// Apply logarithmic scaling and cap at reasonable maximum
	return math.Min(math.Log1p(diversityRatio*10), 2.0)
}

// calculateTrustScore computes trust-weighted score
func (tc *TrendingCalculator) calculateTrustScore(metrics *TrendingMetrics) float64 {
	// Simple trust multiplier - high trust users contribute more to trending
	return metrics.TrustScore
}

// calculateMomentumScore computes momentum/velocity score
func (tc *TrendingCalculator) calculateMomentumScore(metrics *TrendingMetrics) float64 {
	// Positive momentum boosts score, negative momentum reduces it
	return math.Max(0, metrics.MomentumScore) // Only positive momentum counts
}

// calculateMultiWindowScore provides consistency bonus for hashtags trending across multiple windows
func (tc *TrendingCalculator) calculateMultiWindowScore(metrics *TrendingMetrics) float64 {
	var consistentWindows int
	totalWindows := len(tc.config.TimeWindows)

	for _, window := range tc.config.TimeWindows {
		windowData, exists := metrics.TimeWindowData[window.Name]
		if !exists {
			continue
		}

		// Check if this window meets minimum criteria
		windowScore := float64(windowData.UsageCount) * windowData.Velocity
		if windowScore >= window.MinScore {
			consistentWindows++
		}
	}

	if totalWindows == 0 {
		return 0.0
	}

	// Bonus for consistency across time windows
	consistencyRatio := float64(consistentWindows) / float64(totalWindows)
	return consistencyRatio * consistencyRatio // Quadratic bonus for high consistency
}

// filterAndSortByScore filters trending scores by threshold and sorts by score
func (r *HashtagRepository) filterAndSortByScore(scores []*TrendingScore, limit int) []*TrendingScore {
	// Sort by overall score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].OverallScore > scores[j].OverallScore
	})

	// Set ranks
	for i, score := range scores {
		score.Rank = i + 1
	}

	// Apply limit
	if len(scores) > limit {
		scores = scores[:limit]
	}

	return scores
}

// storeTrendingResults stores trending results for historical analysis
func (r *HashtagRepository) storeTrendingResults(ctx context.Context, scores []*TrendingScore) error {
	if len(scores) == 0 {
		return nil
	}

	now := time.Now()
	trendModels := make([]*models.HashtagTrend, len(scores))

	for i, score := range scores {
		trendModel := &models.HashtagTrend{
			Name:        score.HashtagName,
			URL:         fmt.Sprintf("https://%s/tags/%s", r.domain, score.HashtagName),
			UsageCount:  score.Metrics.TotalUsage,
			UniqueUsers: score.Metrics.UniqueUsers,
			LastUsed:    score.Metrics.LastUsed,
			FirstSeen:   score.Metrics.FirstSeen,
			TrendScore:  score.OverallScore,
			UpdatedAt:   now,
		}
		trendModel.UpdateKeys()
		trendModels[i] = trendModel
	}

	// Batch create trend records
	batchWriter := batch.NewBatchWriter(r.db, batch.BatchWriterConfig{
		BatchSize: 25,
		Logger:    r.logger,
	})

	items := make([]any, len(trendModels))
	for i, model := range trendModels {
		items[i] = model
	}

	result, err := batchWriter.WriteItems(ctx, items)
	if err != nil {
		return fmt.Errorf("failed to batch store trending results: %w", err)
	}

	r.logger.Debug("stored trending results",
		zap.Int("total_items", result.TotalItems),
		zap.Int("processed_items", result.ProcessedItems),
		zap.Int("failed_items", result.FailedItems))

	return nil
}

// GetTrendingHashtagsAdvanced returns trending hashtags with advanced configuration options
func (r *HashtagRepository) GetTrendingHashtagsAdvanced(ctx context.Context, config TrendingCalculatorConfig, limit int) ([]*storage.TrendingHashtag, error) {
	// Temporarily override the calculator config
	originalCalculator := r.trendingCalculator
	r.trendingCalculator = NewTrendingCalculator(config, r.logger)
	defer func() { r.trendingCalculator = originalCalculator }()

	// Use the standard trending calculation with custom config
	return r.GetTrendingHashtags(ctx, time.Now().Add(-config.MaximumAge), limit)
}

// GetHashtagTrendingHistory returns historical trending data for a hashtag
func (r *HashtagRepository) GetHashtagTrendingHistory(ctx context.Context, hashtag string, days int) ([]*TrendingScore, error) {
	if days <= 0 || days > 30 {
		days = 7
	}

	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))
	startDate := time.Now().AddDate(0, 0, -days)

	var trendRecords []*models.HashtagTrend
	err := r.db.WithContext(ctx).Model(&models.HashtagTrend{}).
		Index("gsi8").
		Where("GSI8PK", "BEGINS_WITH", "TREND_TYPE#HASHTAG#").
		Filter("Name", "=", tagLower).
		Filter("UpdatedAt", ">=", startDate.Format(time.RFC3339)).
		OrderBy("UpdatedAt", "DESC").
		All(&trendRecords)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*TrendingScore{}, nil
		}
		return nil, fmt.Errorf("failed to get hashtag trending history: %w", err)
	}

	// Convert to trending scores
	history := make([]*TrendingScore, len(trendRecords))
	for i, record := range trendRecords {
		history[i] = &TrendingScore{
			HashtagName:  record.Name,
			OverallScore: record.TrendScore,
			Metrics: &TrendingMetrics{
				HashtagName: record.Name,
				TotalUsage:  record.UsageCount,
				UniqueUsers: record.UniqueUsers,
				LastUsed:    record.LastUsed,
				FirstSeen:   record.FirstSeen,
			},
			Timestamp: record.UpdatedAt,
		}
	}

	return history, nil
}

// GetTrendingAnalytics returns analytics about the trending calculation process
func (r *HashtagRepository) GetTrendingAnalytics(ctx context.Context, since time.Time) (*TrendingAnalytics, error) {
	// Get recent hashtag usage stats
	var hashtagCount int64
	var totalUsage int64
	var uniqueUsers int64

	// Query for basic analytics
	var hashtagModels []*models.Hashtag
	err := r.db.WithContext(ctx).Model(&models.Hashtag{}).
		Where("SK", "=", "METADATA").
		Filter("LastUsed", ">=", since.Format(time.RFC3339)).
		All(&hashtagModels)

	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get hashtag analytics: %w", err)
	}

	userSet := make(map[string]bool)
	for _, hashtag := range hashtagModels {
		hashtagCount++
		totalUsage += hashtag.UsageCount

		// Get unique users for this hashtag (simplified)
		var usageRecords []*models.HashtagUsage
		err = r.db.WithContext(ctx).Model(&models.HashtagUsage{}).
			Where("PK", "=", fmt.Sprintf("HASHTAG#%s", hashtag.Name)).
			Where("SK", ">=", fmt.Sprintf("USAGE#%d", since.Unix())).
			Limit(100). // Limit for performance
			All(&usageRecords)

		if err == nil {
			for _, usage := range usageRecords {
				userSet[usage.AuthorID] = true
			}
		}
	}

	uniqueUsers = int64(len(userSet))

	// Get trending threshold statistics
	config := r.trendingCalculator.config
	trendingCount := int64(0)

	// Count hashtags that would meet trending criteria
	for _, hashtag := range hashtagModels {
		if hashtag.UsageCount >= config.MinimumUsage {
			trendingCount++
		}
	}

	return &TrendingAnalytics{
		Period:              since,
		TotalHashtags:       hashtagCount,
		TotalUsage:          totalUsage,
		UniqueUsers:         uniqueUsers,
		TrendingCandidates:  trendingCount,
		AverageUsagePerTag:  float64(totalUsage) / math.Max(1, float64(hashtagCount)),
		AverageUsersPerTag:  float64(uniqueUsers) / math.Max(1, float64(hashtagCount)),
		TrendingThreshold:   config.TrendingThreshold,
		MinimumUsage:        config.MinimumUsage,
		MinimumUsers:        config.MinimumUsers,
		CalculationWindows:  len(config.TimeWindows),
		GeneratedAt:         time.Now(),
	}, nil
}

// ReconfigureTrendingCalculator updates the trending calculator configuration
func (r *HashtagRepository) ReconfigureTrendingCalculator(config TrendingCalculatorConfig) {
	r.trendingCalculator = NewTrendingCalculator(config, r.logger)
	r.logger.Info("reconfigured trending calculator",
		zap.Duration("decay_half_life", config.DecayHalfLife),
		zap.Float64("trending_threshold", config.TrendingThreshold),
		zap.Int64("minimum_usage", config.MinimumUsage),
		zap.Int64("minimum_users", config.MinimumUsers),
		zap.Int("time_windows", len(config.TimeWindows)))
}

// GetTrendingCalculatorConfig returns the current trending calculator configuration
func (r *HashtagRepository) GetTrendingCalculatorConfig() TrendingCalculatorConfig {
	return r.trendingCalculator.config
}