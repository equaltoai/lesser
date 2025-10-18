//go:build enhanced

package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/ai"
	"go.uber.org/zap"
)

// Enhanced Analysis Methods

// performEnhancedAnalysis conducts comprehensive device analysis using available infrastructure
func (dfm *DeviceFingerprintManager) performEnhancedAnalysis(ctx context.Context, fingerprint *EnhancedDeviceFingerprint) {
	// VPN/Proxy Detection using advanced techniques
	fingerprint.VPNProbability = dfm.detectVPNProbability(ctx, fingerprint)
	fingerprint.ProxyScore = dfm.detectProxyScore(ctx, fingerprint)

	// Bot Detection using behavioral analysis
	fingerprint.BotProbability = dfm.detectBotProbability(ctx, fingerprint)

	// ML-based Anomaly Detection
	if dfm.config.EnableMLAnomalyDetection && dfm.aiService != nil {
		fingerprint.AnomalyScore = dfm.performMLAnomalyDetection(ctx, fingerprint)
	}

	// Behavioral Risk Assessment
	if dfm.config.EnableBehaviorialAnalysis {
		fingerprint.BehavioralRisk = dfm.assessBehavioralRisk(ctx, fingerprint)
	}

	// Network Risk Analysis
	if dfm.config.EnableNetworkAnalysis {
		fingerprint.NetworkRisk = dfm.analyzeNetworkRisk(ctx, fingerprint)
	}

	// Threat Intelligence Score
	fingerprint.ThreatIntelScore = dfm.getThreatIntelligenceScore(ctx, fingerprint)
}

// detectVPNProbability uses advanced techniques to detect VPN/proxy usage
func (dfm *DeviceFingerprintManager) detectVPNProbability(ctx context.Context, fingerprint *EnhancedDeviceFingerprint) float64 {
	probability := 0.0

	// Start with basic VPN detection
	if dfm.isLikelyVPN(fingerprint.IPAddress) {
		probability += 0.3
	}

	// Advanced detection using AI service if available
	if dfm.aiService != nil {
		// Create content for analysis
		content := &ai.Content{
			ID:   fmt.Sprintf("device-analysis-%d", time.Now().UnixNano()),
			Type: "device_fingerprint",
			Text: fmt.Sprintf("IP: %s, UserAgent: %s, Platform: %s",
				fingerprint.IPAddress, fingerprint.UserAgent, fingerprint.Platform),
		}

		// Use AI service for advanced analysis (fallback on error)
		analysis, err := dfm.aiService.AnalyzeContent(ctx, content)
		if err == nil && analysis != nil {
			// Extract VPN indicators from spam analysis (proxy patterns)
			if analysis.SpamAnalysis != nil {
				for _, indicator := range analysis.SpamAnalysis.SpamIndicators {
					if strings.Contains(strings.ToLower(indicator.Type), "proxy") ||
						strings.Contains(strings.ToLower(indicator.Type), "vpn") {
						probability += indicator.Severity * 0.4
					}
				}
			}
		}
	}

	// Additional heuristics
	// Simplified VPN detection - production would have more sophisticated checks
	if strings.Contains(strings.ToLower(fingerprint.UserAgent), "vpn") {
		probability += 0.15
	}

	// Cap at 1.0
	if probability > 1.0 {
		probability = 1.0
	}

	return probability
}

// detectProxyScore calculates proxy detection confidence
func (dfm *DeviceFingerprintManager) detectProxyScore(_ context.Context, fingerprint *EnhancedDeviceFingerprint) float64 {
	score := 0.0

	// Check for common proxy indicators
	if strings.Contains(strings.ToLower(fingerprint.UserAgent), "proxy") {
		score += 0.5
	}

	// Check for suspicious headers patterns
	if fingerprint.AcceptEncoding == "" || fingerprint.AcceptLang == "" {
		score += 0.2
	}

	// Simplified proxy detection - production would have more sophisticated checks

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// detectBotProbability uses behavioral patterns to detect automated access
func (dfm *DeviceFingerprintManager) detectBotProbability(_ context.Context, fingerprint *EnhancedDeviceFingerprint) float64 {
	probability := 0.0

	// Use existing suspicious user agent detection
	if dfm.isSuspiciousUserAgent(fingerprint.UserAgent) {
		probability += 0.4
	}

	// Check for bot-like timing patterns
	if fingerprint.RequestTiming < 100*time.Millisecond {
		probability += 0.3 // Very fast requests indicate automation
	}

	// Missing browser characteristics that humans typically have
	missingCharacteristics := 0
	if fingerprint.Timezone == "" {
		missingCharacteristics++
	}
	if fingerprint.ScreenRes == "" {
		missingCharacteristics++
	}
	if fingerprint.ColorDepth == "" {
		missingCharacteristics++
	}

	// Each missing characteristic increases bot probability
	probability += float64(missingCharacteristics) * 0.15

	// Perfect scores are suspicious (real browsers have variations)
	if fingerprint.FingerprintEntropy == 1.0 {
		probability += 0.2
	}

	// Cap at 1.0
	if probability > 1.0 {
		probability = 1.0
	}

	return probability
}

// performMLAnomalyDetection uses AI service for anomaly detection
func (dfm *DeviceFingerprintManager) performMLAnomalyDetection(ctx context.Context, fingerprint *EnhancedDeviceFingerprint) float64 {
	if dfm.aiService == nil {
		return 0.0
	}

	// Create content for ML analysis
	content := &ai.Content{
		ID:   fmt.Sprintf("anomaly-detection-%d", time.Now().UnixNano()),
		Type: "device_anomaly",
		Text: fmt.Sprintf("Fingerprint Analysis: UA=%s, IP=%s, Platform=%s, Timezone=%s, Screen=%s",
			fingerprint.UserAgent, fingerprint.IPAddress, fingerprint.Platform,
			fingerprint.Timezone, fingerprint.ScreenRes),
	}

	// Use AI service for anomaly detection
	analysis, err := dfm.aiService.AnalyzeContent(ctx, content)
	if err != nil {
		dfm.logger.Warn("ML anomaly detection failed", zap.Error(err))
		return 0.0
	}

	if analysis == nil {
		return 0.0
	}

	// Extract anomaly indicators from AI analysis
	anomalyScore := 0.0

	// Use AI detection results as anomaly indicators
	if analysis.AIDetection != nil {
		anomalyScore += analysis.AIDetection.AIGeneratedProbability * 0.3
		anomalyScore += (1.0 - analysis.AIDetection.PatternConsistency) * 0.2
	}

	// Use spam analysis as anomaly indicator
	if analysis.SpamAnalysis != nil {
		anomalyScore += analysis.SpamAnalysis.SpamScore * 0.4
	}

	// Use text analysis sentiment as behavioral anomaly indicator
	if analysis.TextAnalysis != nil {
		// Extreme sentiment scores can indicate anomalous behavior
		if analysis.TextAnalysis.SentimentScores != nil {
			if negative, ok := analysis.TextAnalysis.SentimentScores["negative"]; ok && negative > 0.8 {
				anomalyScore += 0.1
			}
		}
	}

	// Cap at 1.0
	if anomalyScore > 1.0 {
		anomalyScore = 1.0
	}

	return anomalyScore
}

// assessBehavioralRisk analyzes behavioral patterns for risk assessment
func (dfm *DeviceFingerprintManager) assessBehavioralRisk(_ context.Context, fingerprint *EnhancedDeviceFingerprint) float64 {
	risk := 0.0

	// Timing-based behavioral analysis
	if fingerprint.RequestTiming < 50*time.Millisecond {
		risk += 0.4 // Extremely fast requests
	} else if fingerprint.RequestTiming < 100*time.Millisecond {
		risk += 0.2 // Very fast requests
	}

	// Request order analysis (if available)
	if len(fingerprint.RequestOrder) > 0 {
		// Look for patterns that indicate automation
		for _, order := range fingerprint.RequestOrder {
			if strings.Contains(strings.ToLower(order), "bot") {
				risk += 0.3
			}
		}
	}

	// Simplified fingerprint consistency checks - production would have more sophisticated checks

	// Entropy analysis - both too low and too high can be suspicious
	if fingerprint.FingerprintEntropy < 0.2 {
		risk += 0.2 // Too generic
	} else if fingerprint.FingerprintEntropy > 0.95 {
		risk += 0.15 // Too unique (potentially fabricated)
	}

	// Cap at 1.0
	if risk > 1.0 {
		risk = 1.0
	}

	return risk
}

// analyzeNetworkRisk performs network-level risk analysis
func (dfm *DeviceFingerprintManager) analyzeNetworkRisk(_ context.Context, fingerprint *EnhancedDeviceFingerprint) float64 {
	risk := 0.0

	// IP-based risk factors
	if dfm.isLikelyVPN(fingerprint.IPAddress) {
		risk += 0.3
	}

	// Simplified network risk checks - production would have more sophisticated checks

	// Cap at 1.0
	if risk > 1.0 {
		risk = 1.0
	}

	return risk
}

// getThreatIntelligenceScore queries threat intelligence for the device
func (dfm *DeviceFingerprintManager) getThreatIntelligenceScore(_ context.Context, fingerprint *EnhancedDeviceFingerprint) float64 {
	score := 0.0

	// Basic threat intelligence based on known patterns
	// In a production system, this would integrate with external threat feeds

	// Simplified threat intelligence - production would integrate with external feeds

	// Suspicious timing patterns common in attacks
	if fingerprint.RequestTiming < 10*time.Millisecond {
		score += 0.3 // Extremely fast requests common in attacks
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	return score
}
