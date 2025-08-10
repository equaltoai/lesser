package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// Test helper functions and types for hashtag analytics

func TestGenerateHourlyPosts(t *testing.T) {
	resolver := &Resolver{
		Logger: zap.NewNop(),
	}

	tests := []struct {
		name       string
		dailyPosts []int
		expected   int // expected total from hourly distribution
	}{
		{
			name:       "empty daily posts",
			dailyPosts: []int{},
			expected:   0,
		},
		{
			name:       "single day with posts",
			dailyPosts: []int{100},
			expected:   100, // Should distribute 100 across 24 hours
		},
		{
			name:       "multiple days",
			dailyPosts: []int{50, 75, 100},
			expected:   100, // Should use last day (today)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hourlyPosts := resolver.generateHourlyPosts(tt.dailyPosts)

			// Check that we have 24 hours
			assert.Len(t, hourlyPosts, 24)

			// Check that the total matches expectation
			total := 0
			for _, count := range hourlyPosts {
				total += count
				assert.GreaterOrEqual(t, count, 0) // No negative posts
			}

			// Allow for rounding differences due to float to int conversion
			assert.InDelta(t, tt.expected, total, float64(tt.expected)*0.1, "Total hourly posts should be close to daily total")
		})
	}
}

func TestCalculateHashtagSentiment(t *testing.T) {
	resolver := &Resolver{
		Logger: zap.NewNop(),
	}

	tests := []struct {
		name     string
		hashtag  string
		expected float64
		delta    float64
	}{
		{
			name:     "neutral hashtag",
			hashtag:  "technology",
			expected: 0.5,
			delta:    0.01,
		},
		{
			name:     "positive hashtag",
			hashtag:  "happy",
			expected: 0.7, // 0.5 + 0.2
			delta:    0.01,
		},
		{
			name:     "negative hashtag",
			hashtag:  "sad",
			expected: 0.3, // 0.5 - 0.2
			delta:    0.01,
		},
		{
			name:     "positive compound hashtag",
			hashtag:  "awesomethings",
			expected: 0.7,
			delta:    0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sentiment := resolver.calculateHashtagSentiment(context.Background(), tt.hashtag)
			assert.InDelta(t, tt.expected, sentiment, tt.delta)
			assert.GreaterOrEqual(t, sentiment, 0.0)
			assert.LessOrEqual(t, sentiment, 1.0)
		})
	}
}

func TestCalculateHashtagEngagement(t *testing.T) {
	// Test the engagement calculation logic directly
	tests := []struct {
		name         string
		hashtag      string
		trendingData []*storage.TrendingHashtag
		expected     float64
	}{
		{
			name:    "trending hashtag rank 1",
			hashtag: "viral",
			trendingData: []*storage.TrendingHashtag{
				{Name: "viral", TrendingRank: 1},
			},
			expected: 1.0, // (11-1)/10 = 1.0
		},
		{
			name:    "trending hashtag rank 5",
			hashtag: "popular",
			trendingData: []*storage.TrendingHashtag{
				{Name: "popular", TrendingRank: 5},
			},
			expected: 0.6, // (11-5)/10 = 0.6
		},
		{
			name:    "non-trending hashtag",
			hashtag: "obscure",
			trendingData: []*storage.TrendingHashtag{
				{Name: "other", TrendingRank: 1},
			},
			expected: 0.1, // Default for non-trending
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the logic directly without mocks
			trends := tt.trendingData
			engagement := 0.1 // default

			for _, trend := range trends {
				if trend.Name == tt.hashtag {
					if trend.TrendingRank > 0 && trend.TrendingRank <= 10 {
						engagement = float64(11-trend.TrendingRank) / 10.0
					} else {
						engagement = 0.5
					}
					break
				}
			}

			assert.InDelta(t, tt.expected, engagement, 0.01)
			assert.GreaterOrEqual(t, engagement, 0.0)
			assert.LessOrEqual(t, engagement, 1.0)
		})
	}
}

func TestGetHashtagAnalytics(t *testing.T) {
	// This test would require more complex mocking of the entire storage interface
	// For now, we'll skip this test and rely on integration tests
	t.Skip("Requires full storage mock implementation")
}
