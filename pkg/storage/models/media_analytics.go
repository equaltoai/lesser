package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// MediaAnalytics tracks media streaming analytics with variant-level cost attribution
type MediaAnalytics struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB Keys
	PK string `theorydb:"pk,attr:PK" json:"pk"` // MEDIA_ANALYTICS#{format} or MANIFEST#{format}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // {timestamp}#{mediaID} or {date}#{mediaID}

	// GSI keys for querying
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1pk"` // DATE#{date}
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1sk"` // {format}#{timestamp}

	// GSI2 for variant-level queries
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"gsi2pk"` // VARIANT#{variant_key}
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"gsi2sk"` // COST#{timestamp}

	// Business fields
	MediaID   string    `theorydb:"attr:mediaID" json:"media_id"`
	Format    string    `theorydb:"attr:format" json:"format"`       // hls, dash
	Duration  float64   `theorydb:"attr:duration" json:"duration"`   // Media duration in seconds
	Timestamp time.Time `theorydb:"attr:timestamp" json:"timestamp"` // When the event occurred
	Date      string    `theorydb:"attr:date" json:"date"`           // YYYY-MM-DD for daily aggregation

	// Metadata
	EventType string `theorydb:"attr:eventType" json:"event_type,omitempty"` // manifest_generated, quality_changed, etc.
	UserID    string `theorydb:"attr:userID" json:"user_id,omitempty"`       // User requesting the manifest
	Quality   string `theorydb:"attr:quality" json:"quality,omitempty"`      // Video quality if applicable

	// NEW: Variant-level cost metrics (all costs in microdollars)
	VariantCosts     map[string]MediaVariantCost `theorydb:"attr:variantCosts" json:"variant_costs"`          // Per-variant cost breakdown
	TotalVariantCost int64                       `theorydb:"attr:totalVariantCost" json:"total_variant_cost"` // Sum of all variant costs
	DominantVariant  string                      `theorydb:"attr:dominantVariant" json:"dominant_variant"`    // Most expensive variant

	// NEW: Processing cost breakdown by service
	MediaConvertCost int64 `theorydb:"attr:mediaConvertCost" json:"mediaconvert_cost"` // MediaConvert processing costs
	S3StorageCost    int64 `theorydb:"attr:s3StorageCost" json:"s3_storage_cost"`      // S3 storage costs for variants
	CloudFrontCost   int64 `theorydb:"attr:cloudFrontCost" json:"cloudfront_cost"`     // CDN delivery costs
	LambdaCost       int64 `theorydb:"attr:lambdaCost" json:"lambda_cost"`             // Lambda processing costs
	RekognitionCost  int64 `theorydb:"attr:rekognitionCost" json:"rekognition_cost"`   // Content analysis costs

	// NEW: Bandwidth and streaming metrics
	TotalBandwidthBytes int64            `theorydb:"attr:totalBandwidthBytes" json:"total_bandwidth_bytes"` // Total bytes delivered
	VariantBandwidth    map[string]int64 `theorydb:"attr:variantBandwidth" json:"variant_bandwidth"`        // Bytes per variant
	StreamingSessions   int              `theorydb:"attr:streamingSessions" json:"streaming_sessions"`      // Number of active sessions
	QualityDistribution map[string]int   `theorydb:"attr:qualityDistribution" json:"quality_distribution"`  // Viewer count per quality

	// NEW: Performance metrics per variant
	VariantLatency      map[string]int64   `theorydb:"attr:variantLatency" json:"variant_latency"`             // Response time per variant (ms)
	VariantErrorRate    map[string]float64 `theorydb:"attr:variantErrorRate" json:"variant_error_rate"`        // Error rate per variant
	VariantCacheHitRate map[string]float64 `theorydb:"attr:variantCacheHitRate" json:"variant_cache_hit_rate"` // CDN cache hit rate per variant

	// TTL for automatic cleanup
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing MediaAnalytics.
func (MediaAnalytics) TableName() string {
	return MainTableName
}

// MediaVariantCost represents cost metrics for a specific media variant
type MediaVariantCost struct {
	// Variant identification
	Resolution string `json:"resolution"`  // e.g., "1080p", "720p", "480p"
	Codec      string `json:"codec"`       // e.g., "h264", "h265", "av1"
	Bitrate    int    `json:"bitrate"`     // Target bitrate in kbps
	VariantKey string `json:"variant_key"` // Unique identifier: "{resolution}_{codec}_{bitrate}"

	// Processing costs (microdollars)
	ProcessingCost int64 `json:"processing_cost"` // Cost to create this variant
	StorageCost    int64 `json:"storage_cost"`    // Storage cost for variant files
	BandwidthCost  int64 `json:"bandwidth_cost"`  // Delivery cost for this variant
	TotalCost      int64 `json:"total_cost"`      // Sum of all costs for this variant

	// Processing metrics
	ProcessingTimeMs int64   `json:"processing_time_ms"` // Time to process this variant
	OutputSizeBytes  int64   `json:"output_size_bytes"`  // Size of processed variant
	CompressionRatio float64 `json:"compression_ratio"`  // Compression efficiency

	// Quality metrics
	VMAF float64 `json:"vmaf,omitempty"` // Video quality score (if available)
	PSNR float64 `json:"psnr,omitempty"` // Peak signal-to-noise ratio
	SSIM float64 `json:"ssim,omitempty"` // Structural similarity index

	// Usage metrics
	DeliveryCount  int64 `json:"delivery_count"`  // Number of times served
	BandwidthBytes int64 `json:"bandwidth_bytes"` // Total bytes delivered
	ViewerMinutes  int64 `json:"viewer_minutes"`  // Total viewing time in minutes

	// Performance metrics
	AverageLatencyMs int64   `json:"average_latency_ms"` // Average response time
	ErrorRate        float64 `json:"error_rate"`         // Error rate for this variant
	CacheHitRate     float64 `json:"cache_hit_rate"`     // CDN cache performance
}

// TableName returns the DynamoDB table backing MediaVariantCost.
func (MediaVariantCost) TableName() string {
	return MainTableName
}

// UpdateKeys sets the GSI keys based on the current values
func (m *MediaAnalytics) UpdateKeys() error {
	// Note: PK and SK are set by helper methods (SetManifestGeneration, SetQualityChange, SetGeneralEvent)
	// based on the specific event type. We validate they exist but don't reconstruct them here.
	if m.MediaID == "" {
		return fmt.Errorf("media ID is required")
	}
	if m.Date == "" {
		return fmt.Errorf("date is required")
	}
	if m.PK == "" || m.SK == "" {
		return fmt.Errorf("PK and SK must be set before calling UpdateKeys")
	}

	// Set GSI1 keys for date-based queries
	m.GSI1PK = fmt.Sprintf("DATE#%s", m.Date)
	m.GSI1SK = fmt.Sprintf("%s#%s", m.Format, m.Timestamp.Format(time.RFC3339))

	// Set GSI2 keys for variant-level queries
	if m.DominantVariant != "" {
		m.GSI2PK = fmt.Sprintf("VARIANT#%s", m.DominantVariant)
		m.GSI2SK = fmt.Sprintf("COST#%s", m.Timestamp.Format(time.RFC3339))
	}
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (m *MediaAnalytics) GetPK() string {
	return m.PK
}

// GetSK returns the sort key for BaseModel interface
func (m *MediaAnalytics) GetSK() string {
	return m.SK
}

// SetManifestGeneration configures this record for manifest generation tracking
func (m *MediaAnalytics) SetManifestGeneration(mediaID, format string, duration float64) {
	m.MediaID = mediaID
	m.Format = format
	m.Duration = duration
	m.EventType = "manifest_generated"
	m.Timestamp = time.Now()
	m.Date = m.Timestamp.Format(common.DateFormat)

	// Initialize maps for variant tracking
	m.initializeMaps()

	// Set primary keys
	m.PK = fmt.Sprintf("MANIFEST#%s", format)
	m.SK = fmt.Sprintf("%d#%s", m.Timestamp.Unix(), mediaID)

	// Set TTL to 30 days
	m.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()

	_ = m.UpdateKeys() // Ignore error as this is internal model operation
}

// SetQualityChange configures this record for quality change tracking
func (m *MediaAnalytics) SetQualityChange(mediaID, userID, _, newQuality string) {
	m.MediaID = mediaID
	m.UserID = userID
	m.Quality = newQuality
	m.EventType = "quality_changed"
	m.Timestamp = time.Now()
	m.Date = m.Timestamp.Format(common.DateFormat)

	// Initialize maps for variant tracking
	m.initializeMaps()

	// Set primary keys
	m.PK = fmt.Sprintf("QUALITY_CHANGE#%s", userID)
	m.SK = fmt.Sprintf("%d#%s#%s", m.Timestamp.Unix(), mediaID, newQuality)

	// Set TTL to 7 days
	m.TTL = time.Now().Add(7 * 24 * time.Hour).Unix()

	_ = m.UpdateKeys() // Ignore error as this is internal model operation
}

// SetGeneralEvent configures this record for general media events
func (m *MediaAnalytics) SetGeneralEvent(eventType, mediaID, userID string) {
	m.MediaID = mediaID
	m.UserID = userID
	m.EventType = eventType
	m.Timestamp = time.Now()
	m.Date = m.Timestamp.Format(common.DateFormat)

	// Initialize maps for variant tracking
	m.initializeMaps()

	// Set primary keys
	m.PK = fmt.Sprintf("MEDIA_EVENT#%s", eventType)
	m.SK = fmt.Sprintf("%d#%s", m.Timestamp.Unix(), mediaID)

	// Set TTL based on event type
	switch eventType {
	case "session_start", "session_end":
		m.TTL = time.Now().Add(7 * 24 * time.Hour).Unix() // 7 days
	default:
		m.TTL = time.Now().Add(30 * 24 * time.Hour).Unix() // 30 days
	}

	_ = m.UpdateKeys() // Ignore error as this is internal model operation
}

// initializeMaps initializes all maps for variant tracking
func (m *MediaAnalytics) initializeMaps() {
	if m.VariantCosts == nil {
		m.VariantCosts = make(map[string]MediaVariantCost)
	}
	if m.VariantBandwidth == nil {
		m.VariantBandwidth = make(map[string]int64)
	}
	if m.QualityDistribution == nil {
		m.QualityDistribution = make(map[string]int)
	}
	if m.VariantLatency == nil {
		m.VariantLatency = make(map[string]int64)
	}
	if m.VariantErrorRate == nil {
		m.VariantErrorRate = make(map[string]float64)
	}
	if m.VariantCacheHitRate == nil {
		m.VariantCacheHitRate = make(map[string]float64)
	}
}

// AddVariantCost adds cost tracking for a specific media variant
func (m *MediaAnalytics) AddVariantCost(resolution, codec string, bitrate int, cost MediaVariantCost) {
	m.initializeMaps()

	variantKey := fmt.Sprintf("%s_%s_%d", resolution, codec, bitrate)
	cost.VariantKey = variantKey
	cost.Resolution = resolution
	cost.Codec = codec
	cost.Bitrate = bitrate

	// Calculate total cost for this variant
	cost.TotalCost = cost.ProcessingCost + cost.StorageCost + cost.BandwidthCost

	m.VariantCosts[variantKey] = cost

	// Update totals
	m.recalculateTotals()
}

// TrackVariantDelivery tracks delivery metrics for a specific variant
func (m *MediaAnalytics) TrackVariantDelivery(variantKey string, bytes int64, latencyMs int64, cacheHit bool, success bool) {
	m.initializeMaps()

	// Update bandwidth tracking
	m.VariantBandwidth[variantKey] += bytes
	m.TotalBandwidthBytes += bytes

	// Update latency (running average)
	if existingLatency, exists := m.VariantLatency[variantKey]; exists {
		m.VariantLatency[variantKey] = (existingLatency + latencyMs) / 2
	} else {
		m.VariantLatency[variantKey] = latencyMs
	}

	// Update cache hit rate
	if existingHitRate, exists := m.VariantCacheHitRate[variantKey]; exists {
		// Simple running average - in production would use more sophisticated tracking
		if cacheHit {
			m.VariantCacheHitRate[variantKey] = (existingHitRate + 1.0) / 2.0
		} else {
			m.VariantCacheHitRate[variantKey] = existingHitRate / 2.0
		}
	} else {
		if cacheHit {
			m.VariantCacheHitRate[variantKey] = 1.0
		} else {
			m.VariantCacheHitRate[variantKey] = 0.0
		}
	}

	// Update error rate
	if existingErrorRate, exists := m.VariantErrorRate[variantKey]; exists {
		if success {
			m.VariantErrorRate[variantKey] = existingErrorRate * 0.95 // Decay error rate
		} else {
			m.VariantErrorRate[variantKey] = existingErrorRate*0.95 + 0.05 // Increase error rate
		}
	} else {
		if success {
			m.VariantErrorRate[variantKey] = 0.0
		} else {
			m.VariantErrorRate[variantKey] = 1.0
		}
	}

	// Update variant cost delivery counts
	if cost, exists := m.VariantCosts[variantKey]; exists {
		cost.DeliveryCount++
		cost.BandwidthBytes += bytes
		cost.AverageLatencyMs = latencyMs
		cost.ErrorRate = m.VariantErrorRate[variantKey]
		cost.CacheHitRate = m.VariantCacheHitRate[variantKey]
		m.VariantCosts[variantKey] = cost
	}
}

// AddQualityViewer tracks a viewer switching to a specific quality
func (m *MediaAnalytics) AddQualityViewer(quality string) {
	m.initializeMaps()
	m.QualityDistribution[quality]++
	m.StreamingSessions++
}

// RemoveQualityViewer tracks a viewer leaving a specific quality
func (m *MediaAnalytics) RemoveQualityViewer(quality string) {
	m.initializeMaps()
	if m.QualityDistribution[quality] > 0 {
		m.QualityDistribution[quality]--
		m.StreamingSessions--
	}
}

// recalculateTotals recalculates total costs and determines dominant variant
func (m *MediaAnalytics) recalculateTotals() {
	m.TotalVariantCost = 0
	m.MediaConvertCost = 0
	m.S3StorageCost = 0
	m.CloudFrontCost = 0
	m.LambdaCost = 0

	var maxCost int64
	maxVariant := ""

	for variantKey, cost := range m.VariantCosts {
		m.TotalVariantCost += cost.TotalCost
		m.MediaConvertCost += cost.ProcessingCost
		m.S3StorageCost += cost.StorageCost
		m.CloudFrontCost += cost.BandwidthCost

		if cost.TotalCost > maxCost {
			maxCost = cost.TotalCost
			maxVariant = variantKey
		}
	}

	m.DominantVariant = maxVariant
}

// GetMostPopularQuality returns the quality with the most viewers
func (m *MediaAnalytics) GetMostPopularQuality() (string, int) {
	maxViewers := 0
	popularQuality := ""

	for quality, viewers := range m.QualityDistribution {
		if viewers > maxViewers {
			maxViewers = viewers
			popularQuality = quality
		}
	}

	return popularQuality, maxViewers
}

// GetCostEfficiencyMetrics returns cost efficiency metrics per variant
func (m *MediaAnalytics) GetCostEfficiencyMetrics() map[string]interface{} {
	metrics := make(map[string]interface{})

	for variantKey, cost := range m.VariantCosts {
		variantMetrics := make(map[string]interface{})

		if cost.ViewerMinutes > 0 {
			variantMetrics["cost_per_viewer_minute"] = float64(cost.TotalCost) / float64(cost.ViewerMinutes)
		}
		if cost.BandwidthBytes > 0 {
			variantMetrics["cost_per_gb"] = float64(cost.BandwidthCost) / (float64(cost.BandwidthBytes) / (1024 * 1024 * 1024))
		}
		if cost.DeliveryCount > 0 {
			variantMetrics["cost_per_delivery"] = float64(cost.TotalCost) / float64(cost.DeliveryCount)
		}

		variantMetrics["compression_efficiency"] = cost.CompressionRatio
		variantMetrics["average_latency_ms"] = cost.AverageLatencyMs
		variantMetrics["error_rate"] = cost.ErrorRate
		variantMetrics["cache_hit_rate"] = cost.CacheHitRate

		metrics[variantKey] = variantMetrics
	}

	// Overall metrics
	overallMetrics := make(map[string]interface{})
	if m.StreamingSessions > 0 {
		overallMetrics["cost_per_session"] = float64(m.TotalVariantCost) / float64(m.StreamingSessions)
	}
	if m.TotalBandwidthBytes > 0 {
		overallMetrics["total_cost_per_gb"] = float64(m.TotalVariantCost) / (float64(m.TotalBandwidthBytes) / (1024 * 1024 * 1024))
	}
	overallMetrics["dominant_variant"] = m.DominantVariant
	overallMetrics["total_variants"] = len(m.VariantCosts)

	metrics["overall"] = overallMetrics
	return metrics
}
