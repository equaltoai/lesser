package streaming

import (
	"context"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// MetricsTracker implements comprehensive streaming metrics tracking with CloudWatch integration
type MetricsTracker struct {
	cloudWatch *cloudwatch.Client
	logger     *zap.Logger
	namespace  string

	// Session metrics cache for efficient tracking
	sessionMetrics sync.Map

	// Batch publishing to reduce CloudWatch costs
	metricsBatch  []types.MetricDatum
	batchMutex    sync.RWMutex
	batchSize     int
	batchInterval time.Duration
	lastPublish   time.Time
}

// NewMetricsTracker creates a new metrics tracker with CloudWatch integration
func NewMetricsTracker(cloudWatch *cloudwatch.Client, logger *zap.Logger) *MetricsTracker {
	tracker := &MetricsTracker{
		cloudWatch:    cloudWatch,
		logger:        logger,
		namespace:     "Lesser/Streaming",
		batchSize:     20,               // CloudWatch allows max 20 metrics per request
		batchInterval: 30 * time.Second, // More frequent publishing for real-time metrics
		lastPublish:   time.Now(),
	}

	// Start background batch publisher
	go tracker.batchPublisher()

	return tracker
}

// SessionMetrics tracks detailed metrics for a streaming session
type SessionMetrics struct {
	SessionID  string
	UserID     string
	MediaID    string
	StartTime  time.Time
	LastUpdate time.Time

	// Quality tracking
	CurrentQuality    Quality
	QualitySwitches   int
	TimeInEachQuality map[Quality]time.Duration
	LastQualityChange time.Time

	// Rebuffer tracking
	RebufferEvents    int
	TotalRebufferTime time.Duration
	LastRebufferTime  time.Time

	// Performance metrics
	StartupTime        time.Duration
	SegmentSuccessRate float64
	TotalSegments      int
	FailedSegments     int
	BytesTransferred   int64

	// Buffer health history
	BufferHealthHistory []BufferHealthSample

	// QoE calculation
	QoEScore      float64
	LastQoEUpdate time.Time
}

// BufferHealthSample captures buffer health at a point in time
type BufferHealthSample struct {
	Timestamp   time.Time
	BufferLevel time.Duration
	Quality     Quality
	Bandwidth   int // kbps
}

// EventType represents types of streaming events
type EventType string

// Streaming event types
const (
	// EventSessionStart indicates a streaming session has started
	EventSessionStart EventType = "session_start"
	// EventQualitySwitch indicates a quality level change
	EventQualitySwitch EventType = "quality_switch"
	// EventRebufferStart indicates buffering has started
	EventRebufferStart EventType = "rebuffer_start"
	// EventRebufferEnd indicates buffering has ended
	EventRebufferEnd EventType = "rebuffer_end"
	// EventSegmentLoad indicates a segment was successfully loaded
	EventSegmentLoad EventType = "segment_load"
	// EventSegmentFail indicates a segment failed to load
	EventSegmentFail EventType = "segment_fail"
	// EventSessionEnd indicates a streaming session has ended
	EventSessionEnd EventType = "session_end"
)

// Event represents a streaming event for metrics tracking
type Event struct {
	SessionID string
	UserID    string
	MediaID   string
	EventType EventType
	Timestamp time.Time

	// Event-specific data
	Quality         Quality
	PreviousQuality Quality
	Duration        time.Duration
	BytesLoaded     int64
	ErrorCode       string
	BufferHealth    float64
	Bandwidth       int
}

// StartSession initializes metrics tracking for a new session
func (smt *MetricsTracker) StartSession(sessionID, userID, mediaID string) {
	metrics := &SessionMetrics{
		SessionID:           sessionID,
		UserID:              userID,
		MediaID:             mediaID,
		StartTime:           time.Now(),
		LastUpdate:          time.Now(),
		TimeInEachQuality:   make(map[Quality]time.Duration),
		BufferHealthHistory: make([]BufferHealthSample, 0, 100), // Pre-allocate for 100 samples
		QoEScore:            0.5,                                // Start with neutral score
		LastQoEUpdate:       time.Now(),
	}

	smt.sessionMetrics.Store(sessionID, metrics)

	// Record session start event
	smt.recordEvent(&Event{
		SessionID: sessionID,
		UserID:    userID,
		MediaID:   mediaID,
		EventType: EventSessionStart,
		Timestamp: time.Now(),
	})
}

// TrackQualitySwitch records a quality change event
func (smt *MetricsTracker) TrackQualitySwitch(sessionID string, fromQuality, toQuality Quality) {
	if metricsData, ok := smt.sessionMetrics.Load(sessionID); ok {
		metrics := metricsData.(*SessionMetrics)
		now := time.Now()

		// Update time spent in previous quality
		if !metrics.LastQualityChange.IsZero() {
			timeInPrevious := now.Sub(metrics.LastQualityChange)
			metrics.TimeInEachQuality[metrics.CurrentQuality] += timeInPrevious
		}

		// Update current quality and switch count
		metrics.CurrentQuality = toQuality
		metrics.QualitySwitches++
		metrics.LastQualityChange = now
		metrics.LastUpdate = now

		// Record CloudWatch event
		smt.recordEvent(&Event{
			SessionID:       sessionID,
			UserID:          metrics.UserID,
			MediaID:         metrics.MediaID,
			EventType:       EventQualitySwitch,
			Timestamp:       now,
			Quality:         toQuality,
			PreviousQuality: fromQuality,
		})

		smt.logger.Debug("quality switch tracked",
			zap.String("sessionID", sessionID),
			zap.String("from", string(fromQuality)),
			zap.String("to", string(toQuality)),
			zap.Int("totalSwitches", metrics.QualitySwitches))
	}
}

// TrackRebufferEvent records rebuffering events
func (smt *MetricsTracker) TrackRebufferEvent(sessionID string, duration time.Duration) {
	if metricsData, ok := smt.sessionMetrics.Load(sessionID); ok {
		metrics := metricsData.(*SessionMetrics)
		now := time.Now()

		metrics.RebufferEvents++
		metrics.TotalRebufferTime += duration
		metrics.LastRebufferTime = now
		metrics.LastUpdate = now

		// Record rebuffer start event
		smt.recordEvent(&Event{
			SessionID: sessionID,
			UserID:    metrics.UserID,
			MediaID:   metrics.MediaID,
			EventType: EventRebufferStart,
			Timestamp: now,
			Duration:  duration,
			Quality:   metrics.CurrentQuality,
		})

		// Update QoE score (rebuffering heavily impacts QoE)
		smt.updateQoEScore(metrics)

		smt.logger.Info("rebuffer event tracked",
			zap.String("sessionID", sessionID),
			zap.Duration("duration", duration),
			zap.Int("totalEvents", metrics.RebufferEvents),
			zap.Duration("totalTime", metrics.TotalRebufferTime))
	}
}

// TrackSegmentLoad records successful segment loading
func (smt *MetricsTracker) TrackSegmentLoad(sessionID string, segmentIndex int, loadTime time.Duration, bytes int64) {
	if metricsData, ok := smt.sessionMetrics.Load(sessionID); ok {
		metrics := metricsData.(*SessionMetrics)
		now := time.Now()

		metrics.TotalSegments++
		metrics.BytesTransferred += bytes
		metrics.LastUpdate = now

		// Calculate success rate
		metrics.SegmentSuccessRate = float64(metrics.TotalSegments-metrics.FailedSegments) / float64(metrics.TotalSegments)

		// Record segment load event (sampled to avoid too many metrics)
		if segmentIndex%10 == 0 { // Sample every 10th segment
			smt.recordEvent(&Event{
				SessionID:   sessionID,
				UserID:      metrics.UserID,
				MediaID:     metrics.MediaID,
				EventType:   EventSegmentLoad,
				Timestamp:   now,
				Duration:    loadTime,
				BytesLoaded: bytes,
				Quality:     metrics.CurrentQuality,
			})
		}

		// Update QoE score periodically
		if now.Sub(metrics.LastQoEUpdate) > 30*time.Second {
			smt.updateQoEScore(metrics)
		}
	}
}

// TrackSegmentFailure records segment loading failures
func (smt *MetricsTracker) TrackSegmentFailure(sessionID string, segmentIndex int, errorCode string) {
	if metricsData, ok := smt.sessionMetrics.Load(sessionID); ok {
		metrics := metricsData.(*SessionMetrics)
		now := time.Now()

		metrics.TotalSegments++
		metrics.FailedSegments++
		metrics.LastUpdate = now

		// Calculate success rate
		metrics.SegmentSuccessRate = float64(metrics.TotalSegments-metrics.FailedSegments) / float64(metrics.TotalSegments)

		// Record segment failure event
		smt.recordEvent(&Event{
			SessionID: sessionID,
			UserID:    metrics.UserID,
			MediaID:   metrics.MediaID,
			EventType: EventSegmentFail,
			Timestamp: now,
			ErrorCode: errorCode,
			Quality:   metrics.CurrentQuality,
		})

		// Update QoE score (failures impact QoE)
		smt.updateQoEScore(metrics)

		smt.logger.Warn("segment failure tracked",
			zap.String("sessionID", sessionID),
			zap.Int("segmentIndex", segmentIndex),
			zap.String("errorCode", errorCode),
			zap.Float64("successRate", metrics.SegmentSuccessRate))
	}
}

// TrackBufferHealth records buffer health samples for adaptive bitrate decisions
func (smt *MetricsTracker) TrackBufferHealth(sessionID string, bufferLevel time.Duration, bandwidth int) {
	if metricsData, ok := smt.sessionMetrics.Load(sessionID); ok {
		metrics := metricsData.(*SessionMetrics)
		now := time.Now()

		sample := BufferHealthSample{
			Timestamp:   now,
			BufferLevel: bufferLevel,
			Quality:     metrics.CurrentQuality,
			Bandwidth:   bandwidth,
		}

		// Keep only last 100 samples to limit memory usage
		if len(metrics.BufferHealthHistory) >= 100 {
			// Remove oldest sample
			metrics.BufferHealthHistory = metrics.BufferHealthHistory[1:]
		}
		metrics.BufferHealthHistory = append(metrics.BufferHealthHistory, sample)
		metrics.LastUpdate = now
	}
}

// EndSession finalizes metrics tracking for a session
func (smt *MetricsTracker) EndSession(sessionID string) *SessionMetrics {
	if metricsData, ok := smt.sessionMetrics.LoadAndDelete(sessionID); ok {
		metrics := metricsData.(*SessionMetrics)
		now := time.Now()

		// Finalize time in current quality
		if !metrics.LastQualityChange.IsZero() {
			timeInCurrent := now.Sub(metrics.LastQualityChange)
			metrics.TimeInEachQuality[metrics.CurrentQuality] += timeInCurrent
		}

		sessionDuration := now.Sub(metrics.StartTime)

		// Calculate final QoE score
		smt.updateQoEScore(metrics)

		// Record session end event with comprehensive metrics
		smt.recordEvent(&Event{
			SessionID:   sessionID,
			UserID:      metrics.UserID,
			MediaID:     metrics.MediaID,
			EventType:   EventSessionEnd,
			Timestamp:   now,
			Duration:    sessionDuration,
			BytesLoaded: metrics.BytesTransferred,
		})

		// Publish final session metrics to CloudWatch
		smt.publishSessionMetrics(metrics, sessionDuration)

		smt.logger.Info("session ended with metrics",
			zap.String("sessionID", sessionID),
			zap.Duration("duration", sessionDuration),
			zap.Int("qualitySwitches", metrics.QualitySwitches),
			zap.Int("rebufferEvents", metrics.RebufferEvents),
			zap.Float64("qoeScore", metrics.QoEScore),
			zap.Int64("bytesTransferred", metrics.BytesTransferred))

		return metrics
	}
	return nil
}

// GetSessionMetrics retrieves current metrics for a session
func (smt *MetricsTracker) GetSessionMetrics(sessionID string) *SessionMetrics {
	if metricsData, ok := smt.sessionMetrics.Load(sessionID); ok {
		// Return a copy to avoid concurrent modification
		original := metricsData.(*SessionMetrics)
		metricsCopy := *original
		metricsCopy.TimeInEachQuality = make(map[Quality]time.Duration)
		for k, v := range original.TimeInEachQuality {
			metricsCopy.TimeInEachQuality[k] = v
		}
		return &metricsCopy
	}
	return nil
}

// updateQoEScore calculates Quality of Experience score based on multiple factors
func (smt *MetricsTracker) updateQoEScore(metrics *SessionMetrics) {
	now := time.Now()
	sessionDuration := now.Sub(metrics.StartTime)

	if sessionDuration < time.Second {
		return // Too early to calculate meaningful QoE
	}

	// Start with base quality score (0.0 to 1.0)
	qualityScore := smt.calculateQualityScore(metrics)

	// Rebuffer penalty (heavily impacts QoE)
	rebufferRatio := float64(metrics.TotalRebufferTime) / float64(sessionDuration)
	rebufferPenalty := rebufferRatio * 3.0 // Heavy penalty for rebuffering

	// Quality switches penalty (moderate impact)
	switchPenalty := float64(metrics.QualitySwitches) * 0.02 // 2% penalty per switch

	// Segment failure penalty
	failurePenalty := (1.0 - metrics.SegmentSuccessRate) * 0.5 // Up to 50% penalty

	// Calculate final QoE score (0.0 to 1.0)
	qoeScore := qualityScore - rebufferPenalty - switchPenalty - failurePenalty
	if qoeScore < 0.0 {
		qoeScore = 0.0
	}
	if qoeScore > 1.0 {
		qoeScore = 1.0
	}

	metrics.QoEScore = qoeScore
	metrics.LastQoEUpdate = now
}

// calculateQualityScore computes quality score based on time spent in each quality
func (smt *MetricsTracker) calculateQualityScore(metrics *SessionMetrics) float64 {
	if err := common.ValidateSliceNotEmpty("metrics.TimeInEachQuality", metrics.TimeInEachQuality); err != nil {
		return 0.5 // Neutral score if no quality data
	}

	// Quality weights (higher quality = higher score)
	qualityWeights := map[Quality]float64{
		Quality240p:  0.2,
		Quality360p:  0.3,
		Quality480p:  0.5,
		Quality720p:  0.7,
		Quality1080p: 0.9,
		Quality4K:    1.0,
	}

	totalTime := time.Duration(0)
	weightedScore := 0.0

	for quality, duration := range metrics.TimeInEachQuality {
		totalTime += duration
		if weight, ok := qualityWeights[quality]; ok {
			weightedScore += weight * float64(duration)
		}
	}

	if totalTime == 0 {
		return 0.5
	}

	return weightedScore / float64(totalTime)
}

// recordEvent adds a streaming event to the metrics batch
func (smt *MetricsTracker) recordEvent(event *Event) {
	metricDatum := types.MetricDatum{
		MetricName: aws.String("Event"),
		Dimensions: []types.Dimension{
			{
				Name:  aws.String("EventType"),
				Value: aws.String(string(event.EventType)),
			},
			{
				Name:  aws.String("MediaID"),
				Value: aws.String(event.MediaID),
			},
		},
		Timestamp: aws.Time(event.Timestamp),
		Value:     aws.Float64(1.0),
		Unit:      types.StandardUnitCount,
	}

	// Add quality dimension if relevant
	if event.Quality != "" {
		metricDatum.Dimensions = append(metricDatum.Dimensions, types.Dimension{
			Name:  aws.String("Quality"),
			Value: aws.String(string(event.Quality)),
		})
	}

	// Add duration metric for time-based events
	if event.Duration > 0 {
		durationMetric := types.MetricDatum{
			MetricName: aws.String("EventDuration"),
			Dimensions: metricDatum.Dimensions,
			Timestamp:  aws.Time(event.Timestamp),
			Value:      aws.Float64(event.Duration.Seconds()),
			Unit:       types.StandardUnitSeconds,
		}
		smt.addToBatch(durationMetric)
	}

	// Add bytes metric for data events
	if event.BytesLoaded > 0 {
		bytesMetric := types.MetricDatum{
			MetricName: aws.String("BytesTransferred"),
			Dimensions: metricDatum.Dimensions,
			Timestamp:  aws.Time(event.Timestamp),
			Value:      aws.Float64(float64(event.BytesLoaded)),
			Unit:       types.StandardUnitBytes,
		}
		smt.addToBatch(bytesMetric)
	}

	smt.addToBatch(metricDatum)
}

// publishSessionMetrics publishes comprehensive session metrics to CloudWatch
func (smt *MetricsTracker) publishSessionMetrics(metrics *SessionMetrics, sessionDuration time.Duration) {
	timestamp := time.Now()

	baseDimensions := []types.Dimension{
		{
			Name:  aws.String("MediaID"),
			Value: aws.String(metrics.MediaID),
		},
	}

	// Session-level metrics
	sessionMetrics := []types.MetricDatum{
		{
			MetricName: aws.String("SessionDuration"),
			Dimensions: baseDimensions,
			Timestamp:  aws.Time(timestamp),
			Value:      aws.Float64(sessionDuration.Seconds()),
			Unit:       types.StandardUnitSeconds,
		},
		{
			MetricName: aws.String("QualitySwitches"),
			Dimensions: baseDimensions,
			Timestamp:  aws.Time(timestamp),
			Value:      aws.Float64(float64(metrics.QualitySwitches)),
			Unit:       types.StandardUnitCount,
		},
		{
			MetricName: aws.String("RebufferEvents"),
			Dimensions: baseDimensions,
			Timestamp:  aws.Time(timestamp),
			Value:      aws.Float64(float64(metrics.RebufferEvents)),
			Unit:       types.StandardUnitCount,
		},
		{
			MetricName: aws.String("RebufferRatio"),
			Dimensions: baseDimensions,
			Timestamp:  aws.Time(timestamp),
			Value:      aws.Float64(float64(metrics.TotalRebufferTime) / float64(sessionDuration)),
			Unit:       types.StandardUnitPercent,
		},
		{
			MetricName: aws.String("QoEScore"),
			Dimensions: baseDimensions,
			Timestamp:  aws.Time(timestamp),
			Value:      aws.Float64(metrics.QoEScore * 100), // Convert to 0-100 scale
			Unit:       types.StandardUnitPercent,
		},
		{
			MetricName: aws.String("SegmentSuccessRate"),
			Dimensions: baseDimensions,
			Timestamp:  aws.Time(timestamp),
			Value:      aws.Float64(metrics.SegmentSuccessRate * 100),
			Unit:       types.StandardUnitPercent,
		},
		{
			MetricName: aws.String("BytesTransferred"),
			Dimensions: baseDimensions,
			Timestamp:  aws.Time(timestamp),
			Value:      aws.Float64(float64(metrics.BytesTransferred)),
			Unit:       types.StandardUnitBytes,
		},
	}

	// Add all session metrics to batch
	for _, metric := range sessionMetrics {
		smt.addToBatch(metric)
	}

	// Quality distribution metrics
	for quality, duration := range metrics.TimeInEachQuality {
		qualityMetric := types.MetricDatum{
			MetricName: aws.String("QualityTime"),
			Dimensions: append(baseDimensions, types.Dimension{
				Name:  aws.String("Quality"),
				Value: aws.String(string(quality)),
			}),
			Timestamp: aws.Time(timestamp),
			Value:     aws.Float64(duration.Seconds()),
			Unit:      types.StandardUnitSeconds,
		}
		smt.addToBatch(qualityMetric)
	}
}

// addToBatch adds a metric to the batch for publishing
func (smt *MetricsTracker) addToBatch(metric types.MetricDatum) {
	smt.batchMutex.Lock()
	defer smt.batchMutex.Unlock()

	smt.metricsBatch = append(smt.metricsBatch, metric)

	// Publish immediately if batch is full
	if len(smt.metricsBatch) >= smt.batchSize {
		go smt.publishBatch()
	}
}

// batchPublisher runs in a goroutine to periodically publish metrics batches
func (smt *MetricsTracker) batchPublisher() {
	ticker := time.NewTicker(smt.batchInterval)
	defer ticker.Stop()

	for {
		<-ticker.C
		if time.Since(smt.lastPublish) >= smt.batchInterval {
			smt.publishBatch()
		}
	}
}

// publishBatch publishes the current metrics batch to CloudWatch
func (smt *MetricsTracker) publishBatch() {
	smt.batchMutex.Lock()
	if err := common.ValidateSliceNotEmpty("smt.metricsBatch", smt.metricsBatch); err != nil {
		smt.batchMutex.Unlock()
		return
	}

	// Copy batch and clear it
	batch := make([]types.MetricDatum, len(smt.metricsBatch))
	copy(batch, smt.metricsBatch)
	smt.metricsBatch = smt.metricsBatch[:0] // Clear slice but keep capacity
	smt.lastPublish = time.Now()
	smt.batchMutex.Unlock()

	// Publish to CloudWatch
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	input := &cloudwatch.PutMetricDataInput{
		Namespace:  aws.String(smt.namespace),
		MetricData: batch,
	}

	_, err := smt.cloudWatch.PutMetricData(ctx, input)
	if err != nil {
		smt.logger.Error("failed to publish metrics batch to CloudWatch",
			zap.Int("batchSize", len(batch)),
			zap.Error(err))

		// Re-add metrics to batch for retry (with some loss acceptable to prevent memory leaks)
		smt.batchMutex.Lock()
		if len(smt.metricsBatch) < smt.batchSize {
			// Only retry half to prevent infinite growth
			retryBatch := make([]types.MetricDatum, 0, len(batch)/2)
			for i, datum := range batch {
				if i%2 == 0 { // Take every other metric to reduce load
					retryBatch = append(retryBatch, datum)
				}
			}
			smt.metricsBatch = append(smt.metricsBatch, retryBatch...)
		}
		smt.batchMutex.Unlock()
	} else {
		smt.logger.Debug("published metrics batch to CloudWatch",
			zap.Int("batchSize", len(batch)),
			zap.String("namespace", smt.namespace))
	}
}

// Cleanup removes old session metrics to prevent memory leaks
func (smt *MetricsTracker) Cleanup(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)

	smt.sessionMetrics.Range(func(key, value any) bool {
		if metrics, ok := value.(*SessionMetrics); ok {
			if metrics.LastUpdate.Before(cutoff) {
				smt.sessionMetrics.Delete(key)
				smt.logger.Debug("cleaned up old session metrics",
					zap.String("sessionID", metrics.SessionID))
			}
		}
		return true
	})
}
