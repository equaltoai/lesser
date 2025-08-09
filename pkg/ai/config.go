// Package ai provides configuration and threshold management for AI-based content moderation.
package ai

// ThresholdSet defines action thresholds for different confidence levels
type ThresholdSet struct {
	AutoRemove float64 // Very high confidence required
	AutoHide   float64 // High confidence
	Flag       float64 // Moderate confidence
	WarnAuthor float64 // Low confidence
}

// DefaultThresholds for different content types
var DefaultThresholds = map[string]ThresholdSet{
	"note": {
		AutoRemove: 0.95, // Very high confidence
		AutoHide:   0.85, // High confidence
		Flag:       0.70, // Moderate confidence
		WarnAuthor: 0.60, // Low confidence
	},
	"media": {
		AutoRemove: 0.90, // NSFW images
		AutoHide:   0.80,
		Flag:       0.65,
		WarnAuthor: 0.50,
	},
	"profile": {
		AutoRemove: 0.98, // Very careful with profiles
		AutoHide:   0.90,
		Flag:       0.75,
		WarnAuthor: 0.65,
	},
	"comment": {
		AutoRemove: 0.93,
		AutoHide:   0.83,
		Flag:       0.68,
		WarnAuthor: 0.58,
	},
}

// SignalWeights for different AI detection signals
var SignalWeights = map[string]float64{
	"toxicity":      0.3,  // High weight for toxicity
	"spam":          0.2,  // Moderate weight for spam
	"ai_generated":  0.1,  // Low weight unless undisclosed
	"nsfw":          0.3,  // High weight for NSFW
	"violence":      0.1,  // Moderate weight for violence
	"pii":           0.2,  // High weight for PII leaks
	"impersonation": 0.3,  // High weight for impersonation
	"harassment":    0.25, // High weight for harassment
}

// DefaultAIConfig returns a sensible default configuration
func DefaultAIConfig() *AIConfig {
	return &AIConfig{
		NSFWThreshold:       0.8,
		ToxicityThreshold:   0.7,
		SpamThreshold:       0.75,
		AIContentThreshold:  0.85,
		EnablePIIDetection:  true,
		EnableAIDetection:   true,
		EnableImageAnalysis: true,
		BedrockModelID:      "anthropic.claude-v2",
		ToxicityModelARN:    "", // Custom model if available
		S3Bucket:   "lesser-media-analysis",
		AIQueueURL: "", // Set from environment
	}
}

// GetThresholds returns the appropriate threshold set for a content type
func GetThresholds(contentType string) ThresholdSet {
	if thresholds, ok := DefaultThresholds[contentType]; ok {
		return thresholds
	}
	// Default to note thresholds
	return DefaultThresholds["note"]
}

// GetSignalWeight returns the weight for a specific signal type
func GetSignalWeight(signalType string) float64 {
	if weight, ok := SignalWeights[signalType]; ok {
		return weight
	}
	return 0.1 // Default low weight for unknown signals
}

// CostPerOperation defines AI service costs
var CostPerOperation = map[string]float64{
	"comprehend_sentiment":   0.0001, // Per 100 characters
	"comprehend_entities":    0.0001, // Per 100 characters
	"comprehend_pii":         0.0002, // Per 100 characters
	"comprehend_toxicity":    0.0005, // Custom model
	"rekognition_moderation": 0.001,  // Per image
	"rekognition_text":       0.001,  // Per image
	"rekognition_celebrity":  0.001,  // Per image
	"bedrock_claude":         0.002,  // Per request
	"total_average":          0.003,  // Average per content item
}
