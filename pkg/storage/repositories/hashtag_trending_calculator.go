package repositories

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
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
