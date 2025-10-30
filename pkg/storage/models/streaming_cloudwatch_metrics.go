package models

import (
	"fmt"
	"time"
)

// Quality constants to avoid import cycles
const (
	defaultVideoQuality = "720p"
)

// StreamingCloudWatchMetrics caches CloudWatch metrics for streaming optimization
type StreamingCloudWatchMetrics struct {
	// DynamoDB Keys
	PK string `dynamorm:"pk" json:"pk"` // STREAMING_METRICS#{metric_type}
	SK string `dynamorm:"sk" json:"sk"` // {media_id}#{timestamp}

	// GSI keys for time-based queries
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1pk"` // METRIC_TIME#{date}
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1sk"` // {metric_type}#{timestamp}

	// GSI2 for media-specific queries
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"gsi2pk"` // MEDIA#{media_id}
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"gsi2sk"` // {metric_type}#{timestamp}

	// Business fields
	MediaID    string    `json:"media_id"`
	MetricType string    `json:"metric_type"` // quality_breakdown, geographic_data, concurrent_viewers, performance
	Timestamp  time.Time `json:"timestamp"`
	Date       string    `json:"date"` // YYYY-MM-DD for daily aggregation

	// Quality breakdown metrics (when MetricType = "quality_breakdown")
	QualityMetrics map[string]QualityMetric `json:"quality_metrics,omitempty"`

	// Geographic distribution metrics (when MetricType = "geographic_data")
	GeographicMetrics map[string]GeographicMetric `json:"geographic_metrics,omitempty"`

	// Concurrent viewer metrics (when MetricType = "concurrent_viewers")
	ConcurrentViewers ConcurrentViewerMetrics `json:"concurrent_viewers,omitempty"`

	// Performance metrics (when MetricType = "performance")
	PerformanceMetrics StreamingPerformanceMetrics `json:"performance_metrics,omitempty"`

	// Caching metadata
	CloudWatchQueryTime time.Time `json:"cloudwatch_query_time"` // When the data was fetched from CloudWatch
	DataFreshness       int64     `json:"data_freshness"`        // Seconds since CloudWatch query
	CacheExpiry         time.Time `json:"cache_expiry"`          // When this cache entry expires

	// TTL for automatic cleanup
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// TableName returns the DynamoDB table backing StreamingCloudWatchMetrics.
func (StreamingCloudWatchMetrics) TableName() string {
	return MainTableName
}

// QualityMetric represents metrics for a specific streaming quality
type QualityMetric struct {
	Quality            string  `json:"quality"`             // 480p, 720p, 1080p, 4k
	ViewerCount        int64   `json:"viewer_count"`        // Current viewers at this quality
	ViewerPercentage   float64 `json:"viewer_percentage"`   // Percentage of total viewers
	BufferingRate      float64 `json:"buffering_rate"`      // Buffering events per viewer per hour
	AverageLatencyMs   int64   `json:"average_latency_ms"`  // Average response time for this quality
	ErrorRate          float64 `json:"error_rate"`          // Error rate for this quality
	BitrateUtilization float64 `json:"bitrate_utilization"` // Actual vs target bitrate
	StartupTimeMs      int64   `json:"startup_time_ms"`     // Time to first frame
}

// TableName returns the DynamoDB table backing QualityMetric.
func (QualityMetric) TableName() string {
	return MainTableName
}

// GeographicMetric represents metrics for a specific geographic region
type GeographicMetric struct {
	Region             string  `json:"region"`               // US, EU, AS, etc.
	ViewerCount        int64   `json:"viewer_count"`         // Viewers in this region
	ViewerPercentage   float64 `json:"viewer_percentage"`    // Percentage of total viewers
	AverageLatencyMs   int64   `json:"average_latency_ms"`   // CDN latency for this region
	PreferredQuality   string  `json:"preferred_quality"`    // Most popular quality in this region
	CacheHitRate       float64 `json:"cache_hit_rate"`       // CDN cache performance
	BandwidthUsageMbps float64 `json:"bandwidth_usage_mbps"` // Average bandwidth usage
}

// TableName returns the DynamoDB table backing GeographicMetric.
func (GeographicMetric) TableName() string {
	return MainTableName
}

// ConcurrentViewerMetrics represents concurrent viewing statistics
type ConcurrentViewerMetrics struct {
	CurrentViewers   int64     `json:"current_viewers"`    // Real-time viewer count
	PeakViewers      int64     `json:"peak_viewers"`       // Peak viewers in last 24h
	PeakViewerTime   time.Time `json:"peak_viewer_time"`   // When peak occurred
	AverageViewers   int64     `json:"average_viewers"`    // Average over measurement period
	ViewerGrowthRate float64   `json:"viewer_growth_rate"` // Percentage change in viewers
	SessionDuration  float64   `json:"session_duration"`   // Average session length in minutes
	NewViewers       int64     `json:"new_viewers"`        // New viewers in last hour
	ReturningViewers int64     `json:"returning_viewers"`  // Returning viewers in last hour
}

// TableName returns the DynamoDB table backing ConcurrentViewerMetrics.
func (ConcurrentViewerMetrics) TableName() string {
	return MainTableName
}

// StreamingPerformanceMetrics represents overall streaming performance
type StreamingPerformanceMetrics struct {
	OverallLatencyMs     int64   `json:"overall_latency_ms"`     // Overall CDN latency
	OverallErrorRate     float64 `json:"overall_error_rate"`     // Overall error rate across all qualities
	OverallBufferingRate float64 `json:"overall_buffering_rate"` // Overall buffering rate
	ThroughputMbps       float64 `json:"throughput_mbps"`        // Current throughput
	CDNHitRate           float64 `json:"cdn_hit_rate"`           // Overall CDN cache hit rate
	EdgeLocations        int     `json:"edge_locations"`         // Number of active edge locations
	AutoQualityEvents    int64   `json:"auto_quality_events"`    // Number of quality switches in last hour
	StartupLatencyMs     int64   `json:"startup_latency_ms"`     // Average startup latency across all qualities
}

// TableName returns the DynamoDB table backing StreamingPerformanceMetrics.
func (StreamingPerformanceMetrics) TableName() string {
	return MainTableName
}

// UpdateKeys sets the GSI keys based on the current values
func (s *StreamingCloudWatchMetrics) UpdateKeys() error {
	// Validate required fields
	if s.MediaID == "" {
		return fmt.Errorf("MediaID is required")
	}
	if s.MetricType == "" {
		return fmt.Errorf("MetricType is required")
	}
	if s.Date == "" {
		return fmt.Errorf("Date is required")
	}

	// Note: PK and SK are set by helper methods (SetQualityBreakdown, SetGeographicData, etc.)
	// Validate they exist
	if s.PK == "" || s.SK == "" {
		return fmt.Errorf("PK and SK must be set before calling UpdateKeys")
	}

	// Set GSI1 keys for time-based queries
	s.GSI1PK = fmt.Sprintf("METRIC_TIME#%s", s.Date)
	s.GSI1SK = fmt.Sprintf("%s#%s", s.MetricType, s.Timestamp.Format(time.RFC3339))

	// Set GSI2 keys for media-specific queries
	s.GSI2PK = fmt.Sprintf("MEDIA#%s", s.MediaID)
	s.GSI2SK = fmt.Sprintf("%s#%s", s.MetricType, s.Timestamp.Format(time.RFC3339))
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (s *StreamingCloudWatchMetrics) GetPK() string {
	return s.PK
}

// GetSK returns the sort key for BaseModel interface
func (s *StreamingCloudWatchMetrics) GetSK() string {
	return s.SK
}

// SetQualityBreakdown configures this record for quality breakdown metrics
func (s *StreamingCloudWatchMetrics) SetQualityBreakdown(mediaID string, metrics map[string]QualityMetric) {
	s.MediaID = mediaID
	s.MetricType = "quality_breakdown"
	s.QualityMetrics = metrics
	s.Timestamp = time.Now()
	s.Date = s.Timestamp.Format("2006-01-02")
	s.CloudWatchQueryTime = time.Now()
	s.DataFreshness = 0

	// Set primary keys
	s.PK = fmt.Sprintf("STREAMING_METRICS#%s", s.MetricType)
	s.SK = fmt.Sprintf("%s#%s", mediaID, s.Timestamp.Format(time.RFC3339))

	// Cache for 5 minutes
	s.CacheExpiry = time.Now().Add(5 * time.Minute)
	s.TTL = time.Now().Add(24 * time.Hour).Unix()

	_ = s.UpdateKeys() // Ignore error as this is internal model operation
}

// SetGeographicData configures this record for geographic distribution metrics
func (s *StreamingCloudWatchMetrics) SetGeographicData(mediaID string, metrics map[string]GeographicMetric) {
	s.MediaID = mediaID
	s.MetricType = "geographic_data"
	s.GeographicMetrics = metrics
	s.Timestamp = time.Now()
	s.Date = s.Timestamp.Format("2006-01-02")
	s.CloudWatchQueryTime = time.Now()
	s.DataFreshness = 0

	// Set primary keys
	s.PK = fmt.Sprintf("STREAMING_METRICS#%s", s.MetricType)
	s.SK = fmt.Sprintf("%s#%s", mediaID, s.Timestamp.Format(time.RFC3339))

	// Cache for 10 minutes
	s.CacheExpiry = time.Now().Add(10 * time.Minute)
	s.TTL = time.Now().Add(24 * time.Hour).Unix()

	_ = s.UpdateKeys() // Ignore error as this is internal model operation
}

// SetConcurrentViewers configures this record for concurrent viewer metrics
func (s *StreamingCloudWatchMetrics) SetConcurrentViewers(mediaID string, metrics ConcurrentViewerMetrics) {
	s.MediaID = mediaID
	s.MetricType = "concurrent_viewers"
	s.ConcurrentViewers = metrics
	s.Timestamp = time.Now()
	s.Date = s.Timestamp.Format("2006-01-02")
	s.CloudWatchQueryTime = time.Now()
	s.DataFreshness = 0

	// Set primary keys
	s.PK = fmt.Sprintf("STREAMING_METRICS#%s", s.MetricType)
	s.SK = fmt.Sprintf("%s#%s", mediaID, s.Timestamp.Format(time.RFC3339))

	// Cache for 1 minute (most real-time)
	s.CacheExpiry = time.Now().Add(1 * time.Minute)
	s.TTL = time.Now().Add(6 * time.Hour).Unix()

	_ = s.UpdateKeys() // Ignore error as this is internal model operation
}

// SetPerformanceMetrics configures this record for performance metrics
func (s *StreamingCloudWatchMetrics) SetPerformanceMetrics(mediaID string, metrics StreamingPerformanceMetrics) {
	s.MediaID = mediaID
	s.MetricType = "performance"
	s.PerformanceMetrics = metrics
	s.Timestamp = time.Now()
	s.Date = s.Timestamp.Format("2006-01-02")
	s.CloudWatchQueryTime = time.Now()
	s.DataFreshness = 0

	// Set primary keys
	s.PK = fmt.Sprintf("STREAMING_METRICS#%s", s.MetricType)
	s.SK = fmt.Sprintf("%s#%s", mediaID, s.Timestamp.Format(time.RFC3339))

	// Cache for 2 minutes
	s.CacheExpiry = time.Now().Add(2 * time.Minute)
	s.TTL = time.Now().Add(12 * time.Hour).Unix()

	_ = s.UpdateKeys() // Ignore error as this is internal model operation
}

// IsExpired checks if the cached data has expired
func (s *StreamingCloudWatchMetrics) IsExpired() bool {
	return time.Now().After(s.CacheExpiry)
}

// UpdateFreshness updates the data freshness metric
func (s *StreamingCloudWatchMetrics) UpdateFreshness() {
	s.DataFreshness = int64(time.Since(s.CloudWatchQueryTime).Seconds())
}

// GetBestQuality returns the quality with the lowest buffering rate and good performance
func (s *StreamingCloudWatchMetrics) GetBestQuality() string {
	if s.QualityMetrics == nil {
		return defaultVideoQuality // Default fallback
	}

	bestQuality := "720p"
	bestScore := -1.0

	for quality, metrics := range s.QualityMetrics {
		// Score based on low buffering rate, low latency, and high viewer percentage
		score := (1.0-metrics.BufferingRate)*0.4 + // 40% weight on low buffering
			(1.0-float64(metrics.AverageLatencyMs)/1000.0)*0.3 + // 30% weight on low latency
			metrics.ViewerPercentage*0.3 // 30% weight on popularity

		if score > bestScore {
			bestScore = score
			bestQuality = quality
		}
	}

	return bestQuality
}

// GetBestRegionQuality returns the best quality for a specific region
func (s *StreamingCloudWatchMetrics) GetBestRegionQuality(region string) string {
	if s.GeographicMetrics == nil {
		return defaultVideoQuality // Default fallback
	}

	if regionMetric, exists := s.GeographicMetrics[region]; exists {
		return regionMetric.PreferredQuality
	}

	// Fall back to overall best quality
	return s.GetBestQuality()
}

// ShouldAdaptQuality determines if quality should be adapted based on performance metrics
func (s *StreamingCloudWatchMetrics) ShouldAdaptQuality(currentQuality string) (bool, string) {
	if s.QualityMetrics == nil {
		return false, currentQuality
	}

	currentMetrics, exists := s.QualityMetrics[currentQuality]
	if !exists {
		return true, s.GetBestQuality()
	}

	// Adapt down if high buffering rate or high latency
	if currentMetrics.BufferingRate > 0.1 || currentMetrics.AverageLatencyMs > 2000 {
		// Find a lower quality
		qualityOrder := []string{"4k", "1080p", "720p", "480p"}
		for i, quality := range qualityOrder {
			if quality == currentQuality && i < len(qualityOrder)-1 {
				return true, qualityOrder[i+1]
			}
		}
	}

	// Adapt up if very good performance and high bandwidth utilization
	if currentMetrics.BufferingRate < 0.02 && currentMetrics.AverageLatencyMs < 500 && currentMetrics.BitrateUtilization > 0.9 {
		// Find a higher quality
		qualityOrder := []string{"480p", "720p", "1080p", "4k"}
		for i, quality := range qualityOrder {
			if quality == currentQuality && i < len(qualityOrder)-1 {
				return true, qualityOrder[i+1]
			}
		}
	}

	return false, currentQuality
}
