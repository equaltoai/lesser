package federation

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
)

// TrendAnalyzer analyzes federation flow trends and patterns
type TrendAnalyzer struct {
	storage core.RepositoryStorage
}

// NewTrendAnalyzer creates a new trend analyzer
func NewTrendAnalyzer(store core.RepositoryStorage) *TrendAnalyzer {
	return &TrendAnalyzer{
		storage: store,
	}
}

// AnalyzeTrends analyzes federation trends for a specific domain
func (ta *TrendAnalyzer) AnalyzeTrends(ctx context.Context, domain string, period time.Duration) (*TrendAnalysis, error) {
	// Get time series data for the domain
	endTime := time.Now()
	startTime := endTime.Add(-period)

	// This would require implementing GetFederationTimeSeriesRange
	// For now, we'll work with available connection data
	connections, err := ta.storage.Federation().GetInstanceConnections(ctx, domain, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get connections: %w", err)
	}

	// Filter connections within the time period
	var relevantConnections []*storage.InstanceConnection
	for _, conn := range connections {
		if conn.LastActivity.After(startTime) && conn.LastActivity.Before(endTime) {
			relevantConnections = append(relevantConnections, conn)
		}
	}

	analysis := &TrendAnalysis{
		Domain:    domain,
		Period:    period,
		StartTime: startTime,
		EndTime:   endTime,
	}

	// Analyze volume trends
	analysis.VolumeTrend = ta.analyzeVolumeTrend(relevantConnections, startTime, endTime)

	// Analyze response time trends
	analysis.ResponseTimeTrend = ta.analyzeResponseTimeTrend(relevantConnections, startTime, endTime)

	// Analyze error rate trends
	analysis.ErrorRateTrend = ta.analyzeErrorRateTrend(relevantConnections, startTime, endTime)

	// Identify trending instances
	analysis.TrendingInstances = ta.identifyTrendingInstances(relevantConnections)

	// Detect patterns
	analysis.Patterns = ta.detectPatterns(relevantConnections, startTime, endTime)

	// Calculate trend scores
	analysis.OverallTrendScore = ta.calculateOverallTrendScore(analysis)

	return analysis, nil
}

// analyzeVolumeTrend analyzes the volume trend over time
func (ta *TrendAnalyzer) analyzeVolumeTrend(connections []*storage.InstanceConnection, startTime, endTime time.Time) *VolumeTrend {
	// Group connections by time buckets (e.g., hourly)
	bucketSize := time.Hour
	buckets := make(map[time.Time]int64)

	for _, conn := range connections {
		bucket := conn.LastActivity.Truncate(bucketSize)
		buckets[bucket] += conn.VolumeIn + conn.VolumeOut
	}

	// Convert to sorted time series
	var times []time.Time
	var volumes []int64

	for t := range buckets {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool {
		return times[i].Before(times[j])
	})

	for _, t := range times {
		volumes = append(volumes, buckets[t])
	}

	// Calculate trend metrics
	trend := &VolumeTrend{
		DataPoints: make([]VolumeDataPoint, len(times)),
		Direction:  "stable",
		Slope:      0,
		R2:         0,
	}

	for i, t := range times {
		trend.DataPoints[i] = VolumeDataPoint{
			Timestamp: t,
			Volume:    volumes[i],
		}
	}

	if len(volumes) > 1 {
		slope, r2 := ta.calculateLinearRegression(volumes)
		trend.Slope = slope
		trend.R2 = r2

		// Determine direction based on slope
		if slope > 0.1 {
			trend.Direction = "increasing"
		} else if slope < -0.1 {
			trend.Direction = "decreasing"
		}

		// Calculate peak and total volume
		var total, peak int64
		for _, v := range volumes {
			total += v
			if v > peak {
				peak = v
			}
		}
		trend.TotalVolume = total
		trend.PeakVolume = peak
	}

	return trend
}

// analyzeResponseTimeTrend analyzes response time trends
func (ta *TrendAnalyzer) analyzeResponseTimeTrend(connections []*storage.InstanceConnection, startTime, endTime time.Time) *ResponseTimeTrend {
	// Group by time buckets and calculate average response times
	bucketSize := time.Hour
	buckets := make(map[time.Time][]float64)

	for _, conn := range connections {
		if conn.ResponseTimeMs > 0 {
			bucket := conn.LastActivity.Truncate(bucketSize)
			buckets[bucket] = append(buckets[bucket], conn.ResponseTimeMs)
		}
	}

	// Calculate averages
	var times []time.Time
	var avgResponseTimes []float64

	for t := range buckets {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool {
		return times[i].Before(times[j])
	})

	for _, t := range times {
		responseTimes := buckets[t]
		if len(responseTimes) > 0 {
			var sum float64
			for _, rt := range responseTimes {
				sum += rt
			}
			avgResponseTimes = append(avgResponseTimes, sum/float64(len(responseTimes)))
		} else {
			avgResponseTimes = append(avgResponseTimes, 0)
		}
	}

	trend := &ResponseTimeTrend{
		DataPoints: make([]ResponseTimeDataPoint, len(times)),
		Direction:  "stable",
	}

	for i, t := range times {
		trend.DataPoints[i] = ResponseTimeDataPoint{
			Timestamp:    t,
			ResponseTime: avgResponseTimes[i],
		}
	}

	if len(avgResponseTimes) > 1 {
		slope, _ := ta.calculateLinearRegression(ta.convertToInt64(avgResponseTimes))

		if slope > 10 { // 10ms increase per hour
			trend.Direction = "degrading"
		} else if slope < -10 {
			trend.Direction = "improving"
		}

		// Calculate statistics
		var sum, min, max float64
		min = math.Inf(1)
		max = math.Inf(-1)

		for _, rt := range avgResponseTimes {
			sum += rt
			if rt < min {
				min = rt
			}
			if rt > max {
				max = rt
			}
		}

		trend.AverageResponseTime = sum / float64(len(avgResponseTimes))
		trend.MinResponseTime = min
		trend.MaxResponseTime = max
	}

	return trend
}

// analyzeErrorRateTrend analyzes error rate trends
func (ta *TrendAnalyzer) analyzeErrorRateTrend(connections []*storage.InstanceConnection, startTime, endTime time.Time) *ErrorRateTrend {
	// Group by time buckets and calculate error rates
	bucketSize := time.Hour
	successBuckets := make(map[time.Time]int64)
	totalBuckets := make(map[time.Time]int64)

	for _, conn := range connections {
		bucket := conn.LastActivity.Truncate(bucketSize)
		totalBuckets[bucket]++
		if conn.Success {
			successBuckets[bucket]++
		}
	}

	// Calculate error rates
	var times []time.Time
	var errorRates []float64

	for t := range totalBuckets {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool {
		return times[i].Before(times[j])
	})

	for _, t := range times {
		total := totalBuckets[t]
		success := successBuckets[t]
		if total > 0 {
			errorRate := float64(total-success) / float64(total)
			errorRates = append(errorRates, errorRate)
		} else {
			errorRates = append(errorRates, 0)
		}
	}

	trend := &ErrorRateTrend{
		DataPoints: make([]ErrorRateDataPoint, len(times)),
		Direction:  "stable",
	}

	for i, t := range times {
		trend.DataPoints[i] = ErrorRateDataPoint{
			Timestamp: t,
			ErrorRate: errorRates[i],
		}
	}

	if len(errorRates) > 1 {
		slope, _ := ta.calculateLinearRegression(ta.convertToInt64(errorRates))

		if slope > 0.01 { // 1% increase per hour
			trend.Direction = "increasing"
		} else if slope < -0.01 {
			trend.Direction = "decreasing"
		}

		// Calculate statistics
		var sum, min, max float64
		min = math.Inf(1)
		max = math.Inf(-1)

		for _, er := range errorRates {
			sum += er
			if er < min {
				min = er
			}
			if er > max {
				max = er
			}
		}

		trend.AverageErrorRate = sum / float64(len(errorRates))
		trend.MinErrorRate = min
		trend.MaxErrorRate = max
	}

	return trend
}

// identifyTrendingInstances identifies instances with significant activity changes
func (ta *TrendAnalyzer) identifyTrendingInstances(connections []*storage.InstanceConnection) []*TrendingInstance {
	// Group connections by domain
	domainStats := make(map[string]*DomainStats)

	for _, conn := range connections {
		if _, exists := domainStats[conn.TargetDomain]; !exists {
			domainStats[conn.TargetDomain] = &DomainStats{
				Domain: conn.TargetDomain,
			}
		}

		stats := domainStats[conn.TargetDomain]
		stats.TotalVolume += conn.VolumeIn + conn.VolumeOut
		stats.ConnectionCount++
		stats.TotalResponseTime += conn.ResponseTimeMs

		if !conn.Success {
			stats.ErrorCount++
		}

		if conn.LastActivity.After(stats.LastActivity) {
			stats.LastActivity = conn.LastActivity
		}
	}

	// Calculate trend scores and identify trending instances
	var trending []*TrendingInstance

	for domain, stats := range domainStats {
		if stats.ConnectionCount == 0 {
			continue
		}

		avgResponseTime := stats.TotalResponseTime / float64(stats.ConnectionCount)
		errorRate := float64(stats.ErrorCount) / float64(stats.ConnectionCount)

		// Calculate trend score based on volume, recency, and quality
		volumeScore := float64(stats.TotalVolume) / 100.0 // Normalize
		if volumeScore > 10 {
			volumeScore = 10
		}

		recencyScore := 10.0 - (time.Since(stats.LastActivity).Hours() / 24.0) // Decay over days
		if recencyScore < 0 {
			recencyScore = 0
		}

		qualityScore := 10.0 * (1.0 - errorRate) // Higher score for lower error rate
		if avgResponseTime > 5000 {
			qualityScore *= 0.5 // Penalize high response times
		}

		trendScore := volumeScore*0.4 + recencyScore*0.3 + qualityScore*0.3

		if trendScore > 5.0 { // Threshold for "trending"
			trending = append(trending, &TrendingInstance{
				Domain:       domain,
				TrendScore:   trendScore,
				VolumeChange: stats.TotalVolume, // This would be calculated vs previous period
				ResponseTime: avgResponseTime,
				ErrorRate:    errorRate,
				LastActivity: stats.LastActivity,
				TrendReason:  ta.determineTrendReason(stats, trendScore),
			})
		}
	}

	// Sort by trend score descending
	sort.Slice(trending, func(i, j int) bool {
		return trending[i].TrendScore > trending[j].TrendScore
	})

	// Return top 10 trending instances
	if len(trending) > 10 {
		trending = trending[:10]
	}

	return trending
}

// detectPatterns detects patterns in federation activity
func (ta *TrendAnalyzer) detectPatterns(connections []*storage.InstanceConnection, startTime, endTime time.Time) []*ActivityPattern {
	var patterns []*ActivityPattern

	// Detect daily patterns
	hourlyActivity := make(map[int]int64)
	for _, conn := range connections {
		hour := conn.LastActivity.Hour()
		hourlyActivity[hour] += conn.VolumeIn + conn.VolumeOut
	}

	// Find peak hours
	var peakHour int
	var peakVolume int64
	for hour, volume := range hourlyActivity {
		if volume > peakVolume {
			peakVolume = volume
			peakHour = hour
		}
	}

	if peakVolume > 0 {
		patterns = append(patterns, &ActivityPattern{
			Type:        "daily_peak",
			Description: fmt.Sprintf("Peak activity at hour %d with %d total volume", peakHour, peakVolume),
			Confidence:  0.8,
			Metadata: map[string]any{
				"peak_hour":   peakHour,
				"peak_volume": peakVolume,
			},
		})
	}

	// Detect weekly patterns (if we have enough data)
	duration := endTime.Sub(startTime)
	if duration >= 7*24*time.Hour {
		weekdayActivity := make(map[time.Weekday]int64)
		for _, conn := range connections {
			weekday := conn.LastActivity.Weekday()
			weekdayActivity[weekday] += conn.VolumeIn + conn.VolumeOut
		}

		var peakWeekday time.Weekday
		var peakWeekdayVolume int64
		for weekday, volume := range weekdayActivity {
			if volume > peakWeekdayVolume {
				peakWeekdayVolume = volume
				peakWeekday = weekday
			}
		}

		if peakWeekdayVolume > 0 {
			patterns = append(patterns, &ActivityPattern{
				Type:        "weekly_peak",
				Description: fmt.Sprintf("Peak activity on %s with %d total volume", peakWeekday.String(), peakWeekdayVolume),
				Confidence:  0.7,
				Metadata: map[string]any{
					"peak_weekday": peakWeekday.String(),
					"peak_volume":  peakWeekdayVolume,
				},
			})
		}
	}

	return patterns
}

// Helper methods

func (ta *TrendAnalyzer) calculateLinearRegression(values []int64) (slope, r2 float64) {
	n := float64(len(values))
	if n < 2 {
		return 0, 0
	}

	// Convert to float64
	x := make([]float64, len(values))
	y := make([]float64, len(values))

	for i, v := range values {
		x[i] = float64(i)
		y[i] = float64(v)
	}

	// Calculate means
	var xSum, ySum float64
	for i := 0; i < len(x); i++ {
		xSum += x[i]
		ySum += y[i]
	}
	xMean := xSum / n
	yMean := ySum / n

	// Calculate slope and correlation
	var numerator, xDenominator, yDenominator float64
	for i := 0; i < len(x); i++ {
		xDiff := x[i] - xMean
		yDiff := y[i] - yMean
		numerator += xDiff * yDiff
		xDenominator += xDiff * xDiff
		yDenominator += yDiff * yDiff
	}

	if xDenominator == 0 {
		return 0, 0
	}

	slope = numerator / xDenominator

	if yDenominator == 0 {
		r2 = 0
	} else {
		correlation := numerator / math.Sqrt(xDenominator*yDenominator)
		r2 = correlation * correlation
	}

	return slope, r2
}

func (ta *TrendAnalyzer) convertToInt64(values []float64) []int64 {
	result := make([]int64, len(values))
	for i, v := range values {
		result[i] = int64(v)
	}
	return result
}

func (ta *TrendAnalyzer) calculateOverallTrendScore(analysis *TrendAnalysis) float64 {
	score := 5.0 // Baseline score

	// Adjust based on volume trend
	switch analysis.VolumeTrend.Direction {
	case "increasing":
		score += 2.0
	case "decreasing":
		score -= 1.0
	}

	// Adjust based on response time trend
	switch analysis.ResponseTimeTrend.Direction {
	case "improving":
		score += 1.0
	case "degrading":
		score -= 2.0
	}

	// Adjust based on error rate trend
	switch analysis.ErrorRateTrend.Direction {
	case "decreasing":
		score += 1.0
	case "increasing":
		score -= 2.0
	}

	// Adjust based on trending instances
	score += float64(len(analysis.TrendingInstances)) * 0.1

	// Clamp to 0-10 range
	if score < 0 {
		score = 0
	} else if score > 10 {
		score = 10
	}

	return score
}

func (ta *TrendAnalyzer) determineTrendReason(stats *DomainStats, score float64) string {
	if stats.TotalVolume > 1000 {
		return "high_volume"
	}
	if time.Since(stats.LastActivity) < time.Hour {
		return "recent_activity"
	}
	if stats.ErrorCount == 0 {
		return "high_reliability"
	}
	return "general_activity"
}

// Types for trend analysis

type TrendAnalysis struct {
	Domain            string              `json:"domain"`
	Period            time.Duration       `json:"period"`
	StartTime         time.Time           `json:"start_time"`
	EndTime           time.Time           `json:"end_time"`
	VolumeTrend       *VolumeTrend        `json:"volume_trend"`
	ResponseTimeTrend *ResponseTimeTrend  `json:"response_time_trend"`
	ErrorRateTrend    *ErrorRateTrend     `json:"error_rate_trend"`
	TrendingInstances []*TrendingInstance `json:"trending_instances"`
	Patterns          []*ActivityPattern  `json:"patterns"`
	OverallTrendScore float64             `json:"overall_trend_score"`
}

type VolumeTrend struct {
	DataPoints  []VolumeDataPoint `json:"data_points"`
	Direction   string            `json:"direction"` // increasing/decreasing/stable
	Slope       float64           `json:"slope"`
	R2          float64           `json:"r2"`
	TotalVolume int64             `json:"total_volume"`
	PeakVolume  int64             `json:"peak_volume"`
}

type VolumeDataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Volume    int64     `json:"volume"`
}

type ResponseTimeTrend struct {
	DataPoints          []ResponseTimeDataPoint `json:"data_points"`
	Direction           string                  `json:"direction"` // improving/degrading/stable
	AverageResponseTime float64                 `json:"average_response_time"`
	MinResponseTime     float64                 `json:"min_response_time"`
	MaxResponseTime     float64                 `json:"max_response_time"`
}

type ResponseTimeDataPoint struct {
	Timestamp    time.Time `json:"timestamp"`
	ResponseTime float64   `json:"response_time"`
}

type ErrorRateTrend struct {
	DataPoints       []ErrorRateDataPoint `json:"data_points"`
	Direction        string               `json:"direction"` // increasing/decreasing/stable
	AverageErrorRate float64              `json:"average_error_rate"`
	MinErrorRate     float64              `json:"min_error_rate"`
	MaxErrorRate     float64              `json:"max_error_rate"`
}

type ErrorRateDataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	ErrorRate float64   `json:"error_rate"`
}

type TrendingInstance struct {
	Domain       string    `json:"domain"`
	TrendScore   float64   `json:"trend_score"`
	VolumeChange int64     `json:"volume_change"`
	ResponseTime float64   `json:"response_time"`
	ErrorRate    float64   `json:"error_rate"`
	LastActivity time.Time `json:"last_activity"`
	TrendReason  string    `json:"trend_reason"`
}

type ActivityPattern struct {
	Type        string         `json:"type"`
	Description string         `json:"description"`
	Confidence  float64        `json:"confidence"`
	Metadata    map[string]any `json:"metadata"`
}

type DomainStats struct {
	Domain            string
	TotalVolume       int64
	ConnectionCount   int64
	ErrorCount        int64
	TotalResponseTime float64
	LastActivity      time.Time
}
