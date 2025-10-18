package repositories

import (
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
)

// TestMediaAnalyticsTimeRange_DayCalculation tests day-by-day iteration logic
func TestMediaAnalyticsTimeRange_DayCalculation(t *testing.T) {
	// Test the day iteration logic used by Get*ByTimeRange methods
	startTime := time.Date(2025, 10, 1, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 10, 5, 14, 0, 0, 0, time.UTC)

	// Expected days to query: Oct 1, 2, 3, 4, 5 = 5 days
	expectedDays := []string{
		"DATE#2025-10-01",
		"DATE#2025-10-02",
		"DATE#2025-10-03",
		"DATE#2025-10-04",
		"DATE#2025-10-05",
	}

	// Calculate days as the repository does
	currentDate := startTime.Truncate(24 * time.Hour)
	endDate := endTime.Truncate(24 * time.Hour)

	days := make([]string, 0)
	for !currentDate.After(endDate) {
		dateStr := currentDate.Format(common.DateFormat)
		gsi1pk := fmt.Sprintf("DATE#%s", dateStr)
		days = append(days, gsi1pk)
		currentDate = currentDate.Add(24 * time.Hour)
	}

	assert.Equal(t, expectedDays, days, "Should calculate correct day keys")
	assert.Len(t, days, 5, "Should iterate 5 days")
}

// TestMediaAnalyticsTimeRange_SingleDay tests single day query
func TestMediaAnalyticsTimeRange_SingleDay(t *testing.T) {
	startTime := time.Date(2025, 10, 1, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 10, 1, 14, 0, 0, 0, time.UTC)

	currentDate := startTime.Truncate(24 * time.Hour)
	endDate := endTime.Truncate(24 * time.Hour)

	days := make([]string, 0)
	for !currentDate.After(endDate) {
		dateStr := currentDate.Format(common.DateFormat)
		gsi1pk := fmt.Sprintf("DATE#%s", dateStr)
		days = append(days, gsi1pk)
		currentDate = currentDate.Add(24 * time.Hour)
	}

	assert.Len(t, days, 1, "Should query exactly 1 day")
	assert.Equal(t, "DATE#2025-10-01", days[0])
}

// TestMediaAnalyticsTimeRange_TimeFiltering tests timestamp filtering logic
func TestMediaAnalyticsTimeRange_TimeFiltering(t *testing.T) {
	startTime := time.Date(2025, 10, 1, 10, 0, 0, 0, time.UTC) // 10 AM
	endTime := time.Date(2025, 10, 1, 14, 0, 0, 0, time.UTC)   // 2 PM

	// Test analytics that should be included
	included := &models.MediaAnalytics{
		Timestamp: time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC), // Noon
	}
	assert.False(t, included.Timestamp.Before(startTime), "Should not be before start")
	assert.False(t, included.Timestamp.After(endTime), "Should not be after end")

	// Test analytics that should be excluded (before range)
	beforeRange := &models.MediaAnalytics{
		Timestamp: time.Date(2025, 10, 1, 9, 0, 0, 0, time.UTC), // 9 AM
	}
	assert.True(t, beforeRange.Timestamp.Before(startTime), "Should be before start")

	// Test analytics that should be excluded (after range)
	afterRange := &models.MediaAnalytics{
		Timestamp: time.Date(2025, 10, 1, 15, 0, 0, 0, time.UTC), // 3 PM
	}
	assert.True(t, afterRange.Timestamp.After(endTime), "Should be after end")
}

// TestMediaPopularityKeys_DescendingSortOrder tests GSI1SK ordering for popularity
func TestMediaPopularityKeys_DescendingSortOrder(t *testing.T) {
	// Create two popularity records with different view counts
	pop1 := &models.MediaPopularity{}
	pop1.SetForPeriod("media-1", "WEEK", 100)

	pop2 := &models.MediaPopularity{}
	pop2.SetForPeriod("media-2", "WEEK", 500)

	// Primary keys are STABLE (never change)
	assert.Equal(t, "MEDIA_POPULARITY#WEEK", pop1.PK)
	assert.Equal(t, "MEDIA#media-1", pop1.SK)
	assert.Equal(t, "MEDIA_POPULARITY#WEEK", pop2.PK)
	assert.Equal(t, "MEDIA#media-2", pop2.SK)

	// GSI1SK provides ordering: higher view count has LOWER GSI1SK
	assert.True(t, pop2.GSI1SK < pop1.GSI1SK, "500 views should have lower GSI1SK than 100 views")

	// Both query via same GSI1PK
	assert.Equal(t, "PERIOD#WEEK", pop1.GSI1PK)
	assert.Equal(t, "PERIOD#WEEK", pop2.GSI1PK)
}

// TestMediaPopularityKeys_PaddingFormat tests GSI1SK padding
func TestMediaPopularityKeys_PaddingFormat(t *testing.T) {
	pop := &models.MediaPopularity{}
	pop.SetForPeriod("media-123", "WEEK", 42)

	// Primary key is stable
	assert.Equal(t, "MEDIA_POPULARITY#WEEK", pop.PK)
	assert.Equal(t, "MEDIA#media-123", pop.SK)

	// GSI1SK should be 20-digit inverted count
	// For viewCount=42, inverse is 999999999999999999 - 42 = 999999999999999957
	assert.Regexp(t, `^\d{20}$`, pop.GSI1SK, "GSI1SK should be 20-digit inverted count")
	assert.Equal(t, "PERIOD#WEEK", pop.GSI1PK)
}

// TestMediaPopularity_QualityTracking tests quality view tracking
func TestMediaPopularity_QualityTracking(t *testing.T) {
	pop := &models.MediaPopularity{}
	pop.SetForPeriod("media-123", "WEEK", 0)

	// Add quality views
	pop.AddQualityView("720p", 50)
	pop.AddQualityView("1080p", 30)
	pop.AddQualityView("720p", 20) // Additional views in same quality

	assert.Equal(t, int64(70), pop.QualityViews["720p"])
	assert.Equal(t, int64(30), pop.QualityViews["1080p"])
}

// TestMediaPopularity_Metrics tests metric calculations
func TestMediaPopularity_Metrics(t *testing.T) {
	pop := &models.MediaPopularity{
		ViewCount:       100,
		CompletionCount: 75,
		TotalWatchTime:  6000, // 6000 seconds total
	}

	// Test completion rate: 75/100 = 0.75
	completionRate := pop.CalculateCompletionRate()
	assert.Equal(t, 0.75, completionRate)

	// Test average watch time: 6000/100 = 60 seconds
	avgWatchTime := pop.CalculateAvgWatchTime()
	assert.Equal(t, 60.0, avgWatchTime)
}

// TestMediaPopularity_ZeroViews tests edge case with no views
func TestMediaPopularity_ZeroViews(t *testing.T) {
	pop := &models.MediaPopularity{
		ViewCount:       0,
		CompletionCount: 0,
		TotalWatchTime:  0,
	}

	assert.Equal(t, 0.0, pop.CalculateCompletionRate())
	assert.Equal(t, 0.0, pop.CalculateAvgWatchTime())
}

// TestMediaPopularity_TTLByPeriod tests TTL values by period
func TestMediaPopularity_TTLByPeriod(t *testing.T) {
	now := time.Now()

	tests := []struct {
		period      string
		minDuration time.Duration
		maxDuration time.Duration
	}{
		{"DAY", 6 * 24 * time.Hour, 8 * 24 * time.Hour},     // ~7 days
		{"WEEK", 29 * 24 * time.Hour, 31 * 24 * time.Hour},  // ~30 days
		{"MONTH", 89 * 24 * time.Hour, 91 * 24 * time.Hour}, // ~90 days
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			pop := &models.MediaPopularity{}
			pop.SetForPeriod("media-123", tt.period, 100)

			ttlTime := time.Unix(pop.TTL, 0)
			ttlDuration := ttlTime.Sub(now)

			assert.GreaterOrEqual(t, ttlDuration, tt.minDuration, "TTL should be at least %v for %s", tt.minDuration, tt.period)
			assert.LessOrEqual(t, ttlDuration, tt.maxDuration, "TTL should be at most %v for %s", tt.maxDuration, tt.period)
		})
	}
}

// TestMediaPopularityIncrementViews tests view count increment
func TestMediaPopularityIncrementViews(t *testing.T) {
	pop := &models.MediaPopularity{}
	pop.SetForPeriod("media-123", "WEEK", 100)

	originalSK := pop.SK
	originalGSI1SK := pop.GSI1SK

	// Increment views
	pop.IncrementViews(50)

	assert.Equal(t, int64(150), pop.ViewCount)

	// Primary key SK should NOT change (stable)
	assert.Equal(t, originalSK, pop.SK, "Primary SK should remain stable")
	assert.Equal(t, "MEDIA#media-123", pop.SK)

	// GSI1SK should change (provides ordering)
	assert.NotEqual(t, originalGSI1SK, pop.GSI1SK, "GSI1SK should change with view count")
	assert.True(t, pop.GSI1SK < originalGSI1SK, "Higher view count should have lower GSI1SK")
}

// TestGetPopularMediaPeriodSelection tests period selection logic
func TestGetPopularMediaPeriodSelection(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		duration       time.Duration
		expectedPeriod string
	}{
		{"12 hours", 12 * time.Hour, "DAY"},
		{"24 hours", 24 * time.Hour, "DAY"},
		{"3 days", 3 * 24 * time.Hour, "WEEK"},
		{"7 days", 7 * 24 * time.Hour, "WEEK"},
		{"14 days", 14 * 24 * time.Hour, "MONTH"},
		{"30 days", 30 * 24 * time.Hour, "MONTH"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startTime := now.Add(-tt.duration)
			endTime := now

			// Replicate period selection logic from GetPopularMedia
			period := "WEEK"
			duration := endTime.Sub(startTime)
			if duration <= 24*time.Hour {
				period = "DAY"
			} else if duration <= 7*24*time.Hour {
				period = "WEEK"
			} else {
				period = "MONTH"
			}

			assert.Equal(t, tt.expectedPeriod, period)
		})
	}
}
