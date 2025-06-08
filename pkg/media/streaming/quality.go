package streaming

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

// AdaptiveQualitySelector implements intelligent quality selection
type AdaptiveQualitySelector struct {
	logger         *zap.Logger
	metricsCache   sync.Map
	qualityWeights map[Quality]float64
}

// NewAdaptiveQualitySelector creates a new quality selector
func NewAdaptiveQualitySelector(logger *zap.Logger) *AdaptiveQualitySelector {
	return &AdaptiveQualitySelector{
		logger: logger,
		qualityWeights: map[Quality]float64{
			Quality4K:    1.0,
			Quality1080p: 0.85,
			Quality720p:  0.7,
			Quality480p:  0.5,
			Quality360p:  0.3,
			Quality240p:  0.1,
		},
	}
}

// SelectQuality chooses the optimal quality based on bandwidth and buffer health
func (aqs *AdaptiveQualitySelector) SelectQuality(bandwidth int, bufferHealth float64, availableQualities []Quality) Quality {
	// If buffer is critically low, drop quality aggressively
	if bufferHealth < 0.2 {
		return aqs.selectPanicQuality(bandwidth, availableQualities)
	}

	// Get qualities that can be supported by bandwidth
	supportedQualities := aqs.getSupportedQualities(bandwidth, availableQualities)

	if len(supportedQualities) == 0 {
		// No qualities match bandwidth, return lowest available
		return aqs.getLowestQuality(availableQualities)
	}

	// Apply buffer health adjustment
	if bufferHealth < 0.5 {
		// Buffer is low, be conservative
		return aqs.selectConservativeQuality(supportedQualities, bufferHealth)
	}

	// Buffer is healthy, select best quality within bandwidth
	return aqs.selectOptimalQuality(supportedQualities, bandwidth, bufferHealth)
}

// UpdateMetrics updates quality selection metrics
func (aqs *AdaptiveQualitySelector) UpdateMetrics(sessionID string, rebufferEvents int, qualitySwitches int) {
	metrics, _ := aqs.metricsCache.LoadOrStore(sessionID, &QualityMetrics{
		SessionID:         sessionID,
		LastQualityChange: time.Now(),
		TimeInEachQuality: make(map[Quality]time.Duration),
	})

	m := metrics.(*QualityMetrics)
	m.RebufferEvents += rebufferEvents
	m.QualitySwitches += qualitySwitches

	// Log if excessive rebuffering
	if rebufferEvents > 0 {
		aqs.logger.Warn("rebuffer events detected",
			zap.String("sessionID", sessionID),
			zap.Int("events", rebufferEvents),
			zap.Int("total", m.RebufferEvents))
	}
}

// GetQualityMetrics retrieves metrics for a session
func (aqs *AdaptiveQualitySelector) GetQualityMetrics(sessionID string) *QualityMetrics {
	if metrics, ok := aqs.metricsCache.Load(sessionID); ok {
		return metrics.(*QualityMetrics)
	}
	return nil
}

// Helper methods

func (aqs *AdaptiveQualitySelector) getSupportedQualities(bandwidth int, availableQualities []Quality) []Quality {
	var supported []Quality

	qualityBandwidths := map[Quality]int{
		Quality4K:    20000,
		Quality1080p: 8000,
		Quality720p:  4000,
		Quality480p:  2000,
		Quality360p:  1000,
		Quality240p:  500,
	}

	for _, q := range availableQualities {
		if requiredBandwidth, ok := qualityBandwidths[q]; ok {
			// Add 20% buffer to required bandwidth for stability
			if bandwidth >= int(float64(requiredBandwidth)*1.2) {
				supported = append(supported, q)
			}
		}
	}

	return supported
}

func (aqs *AdaptiveQualitySelector) selectPanicQuality(bandwidth int, availableQualities []Quality) Quality {
	// In panic mode, select quality that uses at most 50% of available bandwidth
	targetBandwidth := bandwidth / 2

	qualityBandwidths := map[Quality]int{
		Quality240p:  500,
		Quality360p:  1000,
		Quality480p:  2000,
		Quality720p:  4000,
		Quality1080p: 8000,
		Quality4K:    20000,
	}

	// Start from lowest quality and work up
	for _, q := range []Quality{Quality240p, Quality360p, Quality480p, Quality720p, Quality1080p, Quality4K} {
		if !aqs.isQualityAvailable(q, availableQualities) {
			continue
		}

		if bandwidth, ok := qualityBandwidths[q]; ok && bandwidth <= targetBandwidth {
			aqs.logger.Debug("selected panic quality",
				zap.String("quality", string(q)),
				zap.Int("bandwidth", bandwidth))
			return q
		}
	}

	return aqs.getLowestQuality(availableQualities)
}

func (aqs *AdaptiveQualitySelector) selectConservativeQuality(supportedQualities []Quality, bufferHealth float64) Quality {
	// With low buffer, select a quality one step down from maximum
	if len(supportedQualities) == 0 {
		return Quality480p // Default fallback
	}

	// Sort qualities by bandwidth requirement
	highestQuality := aqs.getHighestQuality(supportedQualities)

	// If buffer health is between 0.2 and 0.5, step down one quality level
	qualityOrder := []Quality{Quality4K, Quality1080p, Quality720p, Quality480p, Quality360p, Quality240p}

	for i, q := range qualityOrder {
		if q == highestQuality && i < len(qualityOrder)-1 {
			nextLower := qualityOrder[i+1]
			if aqs.isQualityAvailable(nextLower, supportedQualities) {
				aqs.logger.Debug("selected conservative quality",
					zap.String("quality", string(nextLower)),
					zap.Float64("bufferHealth", bufferHealth))
				return nextLower
			}
		}
	}

	return highestQuality
}

func (aqs *AdaptiveQualitySelector) selectOptimalQuality(supportedQualities []Quality, bandwidth int, bufferHealth float64) Quality {
	// With healthy buffer, select highest quality that fits comfortably in bandwidth
	bestQuality := aqs.getLowestQuality(supportedQualities)
	bestScore := 0.0

	for _, q := range supportedQualities {
		score := aqs.calculateQualityScore(q, bandwidth, bufferHealth)
		if score > bestScore {
			bestScore = score
			bestQuality = q
		}
	}

	aqs.logger.Debug("selected optimal quality",
		zap.String("quality", string(bestQuality)),
		zap.Float64("score", bestScore),
		zap.Int("bandwidth", bandwidth),
		zap.Float64("bufferHealth", bufferHealth))

	return bestQuality
}

func (aqs *AdaptiveQualitySelector) calculateQualityScore(quality Quality, bandwidth int, bufferHealth float64) float64 {
	// Base score from quality weight
	score := aqs.qualityWeights[quality]

	// Bandwidth efficiency factor
	qualityBandwidths := map[Quality]int{
		Quality4K:    20000,
		Quality1080p: 8000,
		Quality720p:  4000,
		Quality480p:  2000,
		Quality360p:  1000,
		Quality240p:  500,
	}

	requiredBandwidth := qualityBandwidths[quality]
	bandwidthUtilization := float64(requiredBandwidth) / float64(bandwidth)

	// Penalize if we're using more than 80% of available bandwidth
	if bandwidthUtilization > 0.8 {
		score *= 0.7
	}

	// Buffer health bonus
	score *= (0.5 + bufferHealth*0.5)

	return score
}

func (aqs *AdaptiveQualitySelector) getHighestQuality(qualities []Quality) Quality {
	qualityOrder := map[Quality]int{
		Quality4K:    6,
		Quality1080p: 5,
		Quality720p:  4,
		Quality480p:  3,
		Quality360p:  2,
		Quality240p:  1,
	}

	highest := Quality240p
	highestOrder := 0

	for _, q := range qualities {
		if order, ok := qualityOrder[q]; ok && order > highestOrder {
			highest = q
			highestOrder = order
		}
	}

	return highest
}

func (aqs *AdaptiveQualitySelector) getLowestQuality(qualities []Quality) Quality {
	qualityOrder := map[Quality]int{
		Quality240p:  1,
		Quality360p:  2,
		Quality480p:  3,
		Quality720p:  4,
		Quality1080p: 5,
		Quality4K:    6,
	}

	lowest := Quality4K
	lowestOrder := 7

	for _, q := range qualities {
		if order, ok := qualityOrder[q]; ok && order < lowestOrder {
			lowest = q
			lowestOrder = order
		}
	}

	return lowest
}

func (aqs *AdaptiveQualitySelector) isQualityAvailable(quality Quality, availableQualities []Quality) bool {
	for _, q := range availableQualities {
		if q == quality {
			return true
		}
	}
	return false
}

// CleanupMetrics removes old metrics from cache
func (aqs *AdaptiveQualitySelector) CleanupMetrics(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)

	aqs.metricsCache.Range(func(key, value interface{}) bool {
		if metrics, ok := value.(*QualityMetrics); ok {
			if metrics.LastQualityChange.Before(cutoff) {
				aqs.metricsCache.Delete(key)
			}
		}
		return true
	})
}
