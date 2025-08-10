package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// GetTrendingHashtagsAdvanced allows custom configuration for trending calculation
func (r *HashtagRepository) GetTrendingHashtagsAdvanced(ctx context.Context, config TrendingCalculatorConfig, limit int) ([]*storage.TrendingHashtag, error) {
	// Temporarily override the calculator config
	originalCalculator := r.trendingCalculator
	r.trendingCalculator = NewTrendingCalculator(config, r.logger)
	defer func() { r.trendingCalculator = originalCalculator }()

	// Use the standard trending calculation with custom config
	return r.GetTrendingHashtags(ctx, time.Now().Add(-config.MaximumAge), limit)
}

// GetHashtagTrendingHistory returns historical trending data for a hashtag
func (r *HashtagRepository) GetHashtagTrendingHistory(_ context.Context, _ string, _ int) ([]*TrendingScore, error) {
	// For now, return empty history as the full implementation requires HashtagTrending model
	// which doesn't exist yet
	return []*TrendingScore{}, nil
}

// GetTrendingAnalytics provides aggregated analytics for trending hashtags
func (r *HashtagRepository) GetTrendingAnalytics(ctx context.Context, since time.Time) (*TrendingAnalytics, error) {
	analytics := &TrendingAnalytics{
		Period:             time.Now(),
		TotalHashtags:      0,
		TotalUsage:         0,
		UniqueUsers:        0,
		TrendingCandidates: 0,
		AverageUsagePerTag: 0,
		AverageUsersPerTag: 0,
		TrendingThreshold:  0,
		MinimumUsage:       0,
		MinimumUsers:       0,
		CalculationWindows: 0,
		GeneratedAt:        time.Now(),
	}

	// Get all hashtags active in the period
	var activeHashtags []*models.Hashtag
	err := r.db.WithContext(ctx).Model(&models.Hashtag{}).
		Where("SK", "=", "METADATA").
		Filter("LastUsed", ">=", since.Format(time.RFC3339)).
		Scan(&activeHashtags)

	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get active hashtags: %w", err)
	}

	analytics.TotalHashtags = int64(len(activeHashtags))

	// Count trending hashtags
	trending, err := r.GetTrendingHashtags(ctx, since, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to get trending hashtags: %w", err)
	}
	analytics.TrendingCandidates = int64(len(trending))

	// Calculate aggregate metrics
	var totalUsage int64
	for _, hashtag := range activeHashtags {
		totalUsage += int64(hashtag.UsageCount)
	}
	analytics.TotalUsage = totalUsage

	if analytics.TotalHashtags > 0 {
		analytics.AverageUsagePerTag = float64(totalUsage) / float64(analytics.TotalHashtags)
	}

	return analytics, nil
}

// ReconfigureTrendingCalculator updates the trending calculator configuration
func (r *HashtagRepository) ReconfigureTrendingCalculator(config TrendingCalculatorConfig) {
	r.trendingCalculator = NewTrendingCalculator(config, r.logger)

	r.logger.Info("reconfigured trending calculator",
		zap.Any("config", config))
}

// GetTrendingCalculatorConfig returns the current trending calculator configuration
func (r *HashtagRepository) GetTrendingCalculatorConfig() TrendingCalculatorConfig {
	if r.trendingCalculator == nil {
		// Return a default config if calculator is not initialized
		return TrendingCalculatorConfig{
			DecayHalfLife:     2 * time.Hour,
			MinimumAge:        1 * time.Hour,
			MaximumAge:        24 * time.Hour,
			UsageWeight:       0.3,
			EngagementWeight:  0.2,
			DiversityWeight:   0.2,
			TrustWeight:       0.15,
			MomentumWeight:    0.15,
			MinimumUsage:      5,
			MinimumUsers:      3,
			TrendingThreshold: 0.5,
			TimeWindows:       []TrendingTimeWindow{},
		}
	}

	return r.trendingCalculator.config
}
