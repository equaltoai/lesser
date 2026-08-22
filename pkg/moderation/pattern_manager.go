package moderation

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// PatternStorage defines storage operations needed by pattern manager
type PatternStorage interface {
	CreateModerationPattern(ctx context.Context, pattern *ModerationPattern) error
	GetModerationPattern(ctx context.Context, patternID string) (*ModerationPattern, error)
	GetModerationPatterns(ctx context.Context, active bool, severity string, limit int) ([]*ModerationPattern, error)
	UpdateModerationPattern(ctx context.Context, pattern *ModerationPattern) error
	UpdatePatternStats(ctx context.Context, patternID string, matched bool, falsePositive bool) error
	RecordPatternMatch(ctx context.Context, patternID string, matched bool, timestamp time.Time) error
}

// EnhancedPatternRepository defines the interface for enhanced pattern operations
type EnhancedPatternRepository interface {
	CreatePattern(ctx context.Context, pattern *models.EnhancedModerationPattern) error
	GetPattern(ctx context.Context, patternID string) (*models.EnhancedModerationPattern, error)
	UpdatePattern(ctx context.Context, pattern *models.EnhancedModerationPattern) error
	DeletePattern(ctx context.Context, patternID string) error
	GetActivePatterns(ctx context.Context, limit int) ([]*models.EnhancedModerationPattern, error)
	RecordMatch(ctx context.Context, patternID string, isMatch bool, isTruePositive bool, matchTime float64) error
	GetPatternStatistics(ctx context.Context) (map[string]interface{}, error)
	// Include cache operations for the interface
	GetPatternCache(ctx context.Context, patternID, patternType string) (*models.PatternCache, error)
	SetPatternCache(ctx context.Context, cache *models.PatternCache) error
	InvalidatePatternCache(ctx context.Context, patternID, patternType string) error
}

// PatternManager manages moderation patterns and their effectiveness
type PatternManager struct {
	storage          PatternStorage
	enhancedRepo     EnhancedPatternRepository
	cacheManager     *PatternCacheManager
	patternValidator *PatternValidator
	enhancedEnabled  bool
	logger           *zap.Logger
}

// NewPatternManager creates a new pattern manager
func NewPatternManager() *PatternManager {
	return &PatternManager{
		enhancedEnabled: false,
	}
}

// NewEnhancedPatternManager creates a pattern manager with enhanced capabilities
func NewEnhancedPatternManager(storage PatternStorage, enhancedRepo EnhancedPatternRepository, logger *zap.Logger) *PatternManager {
	cacheManager := NewPatternCacheManager(enhancedRepo, DefaultCacheConfig(), logger)
	patternValidator := NewPatternValidator(logger)

	return &PatternManager{
		storage:          storage,
		enhancedRepo:     enhancedRepo,
		cacheManager:     cacheManager,
		patternValidator: patternValidator,
		enhancedEnabled:  true,
		logger:           logger,
	}
}

// CreatePattern creates a new moderation pattern
func (pm *PatternManager) CreatePattern(ctx context.Context, pattern *ModerationPattern) error {
	// Validate pattern
	if err := pm.validatePattern(pattern); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPattern, err)
	}

	// Set creation metadata
	pattern.CreatedAt = time.Now()
	pattern.UpdatedAt = time.Now()
	pattern.Active = true
	pattern.MatchCount = 0
	pattern.FalsePositiveCount = 0

	// Compile regex if it's a regex pattern
	if pattern.Type == "regex" {
		if _, err := regexp.Compile(pattern.Content); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidRegexPattern, err)
		}
	}

	// Store pattern
	return pm.storage.CreateModerationPattern(ctx, pattern)
}

// GetPatterns retrieves patterns based on criteria
func (pm *PatternManager) GetPatterns(ctx context.Context, active bool, severity string, limit int) ([]*ModerationPattern, error) {
	patterns, err := pm.storage.GetModerationPatterns(ctx, active, severity, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailedToGetPatterns, err)
	}

	// Filter and enrich patterns
	filteredPatterns := make([]*ModerationPattern, 0, len(patterns))
	for _, pattern := range patterns {
		// Calculate effectiveness
		pattern.Effectiveness = pm.calculateEffectiveness(pattern)
		filteredPatterns = append(filteredPatterns, pattern)
	}

	return filteredPatterns, nil
}

// MatchContent matches content against all active patterns
func (pm *PatternManager) MatchContent(ctx context.Context, content *ContentToModerate) ([]*PatternMatch, error) {
	// Get all active patterns
	patterns, err := pm.GetPatterns(ctx, true, "", 1000)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailedToGetPatterns, err)
	}

	var matches []*PatternMatch

	for _, pattern := range patterns {
		match := pm.matchPattern(pattern, content)
		if match != nil {
			matches = append(matches, match)

			// Record the match
			if err := pm.recordMatch(ctx, pattern.ID, true); err != nil {
				// Log error but don't fail the matching
				zap.L().Error("failed to record pattern match", zap.Error(err))
			}
		}
	}

	return matches, nil
}

// UpdatePatternStats updates statistics for a pattern
func (pm *PatternManager) UpdatePatternStats(ctx context.Context, patternID string, wasMatch bool, wasFalsePositive bool) error {
	pattern, err := pm.storage.GetModerationPattern(ctx, patternID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToGetPattern, err)
	}

	if wasMatch {
		pattern.MatchCount++
		if wasFalsePositive {
			pattern.FalsePositiveCount++
		}
	}

	pattern.UpdatedAt = time.Now()
	pattern.Effectiveness = pm.calculateEffectiveness(pattern)

	return pm.storage.UpdateModerationPattern(ctx, pattern)
}

// AnalyzePatternEffectiveness analyzes the effectiveness of patterns
func (pm *PatternManager) AnalyzePatternEffectiveness(ctx context.Context) (*PatternEffectivenessReport, error) {
	patterns, err := pm.GetPatterns(ctx, true, "", 1000)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailedToGetPatterns, err)
	}

	report := &PatternEffectivenessReport{
		GeneratedAt:     time.Now(),
		TotalPatterns:   len(patterns),
		PatternAnalysis: make([]*PatternAnalysis, 0, len(patterns)),
	}

	var totalMatches, totalFalsePositives int64
	var effectivenessSum float64
	var ineffectivePatterns []*ModerationPattern

	for _, pattern := range patterns {
		analysis := &PatternAnalysis{
			PatternID:          pattern.ID,
			PatternType:        pattern.Type,
			Severity:           pattern.Severity,
			MatchCount:         pattern.MatchCount,
			FalsePositiveCount: pattern.FalsePositiveCount,
			Effectiveness:      pattern.Effectiveness,
			TruePositiveRate:   pm.calculateTruePositiveRate(pattern),
			LastMatch:          pattern.LastMatch,
			CreatedAt:          pattern.CreatedAt,
		}

		// Categorize pattern performance
		if pattern.Effectiveness < 0.3 {
			analysis.Performance = "poor"
			ineffectivePatterns = append(ineffectivePatterns, pattern)
		} else if pattern.Effectiveness < 0.7 {
			analysis.Performance = "moderate"
		} else {
			analysis.Performance = "good"
		}

		// Add recommendations
		analysis.Recommendations = pm.generatePatternRecommendations(pattern)

		report.PatternAnalysis = append(report.PatternAnalysis, analysis)

		totalMatches += pattern.MatchCount
		totalFalsePositives += pattern.FalsePositiveCount
		effectivenessSum += pattern.Effectiveness
	}

	// Calculate overall metrics
	if err := common.ValidateSliceNotEmpty("patterns", patterns); err == nil {
		report.AverageEffectiveness = effectivenessSum / float64(len(patterns))
	}

	if totalMatches > 0 {
		report.OverallFalsePositiveRate = float64(totalFalsePositives) / float64(totalMatches)
	}

	report.InefficientPatterns = len(ineffectivePatterns)

	// Generate overall recommendations
	report.Recommendations = pm.generateOverallRecommendations(patterns, ineffectivePatterns)

	return report, nil
}

// OptimizePatterns suggests optimizations for pattern performance
func (pm *PatternManager) OptimizePatterns(ctx context.Context) ([]*PatternOptimization, error) {
	patterns, err := pm.GetPatterns(ctx, true, "", 1000)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailedToGetPatterns, err)
	}

	var optimizations []*PatternOptimization

	for _, pattern := range patterns {
		// Analyze pattern for optimization opportunities
		if optimization := pm.analyzePatternForOptimization(pattern); optimization != nil {
			optimizations = append(optimizations, optimization)
		}
	}

	return optimizations, nil
}

// Helper methods

func (pm *PatternManager) validatePattern(pattern *ModerationPattern) error {
	if err := common.ValidateRequiredParam("pattern.Name", pattern.Name); err != nil {
		return err
	}

	if err := common.ValidateRequiredParam("pattern.Content", pattern.Content); err != nil {
		return err
	}

	if err := common.ValidateRequiredParam("pattern.Type", pattern.Type); err != nil {
		return err
	}

	validTypes := []string{"keyword", "regex", "phrase", "domain", "ip", "hash"}
	validType := false
	for _, vt := range validTypes {
		if pattern.Type == vt {
			validType = true
			break
		}
	}
	if !validType {
		return fmt.Errorf("%w: %s", ErrInvalidPatternType, pattern.Type)
	}

	validSeverities := []string{"low", "medium", "high", "critical"}
	validSeverity := false
	for _, vs := range validSeverities {
		if pattern.Severity == vs {
			validSeverity = true
			break
		}
	}
	if !validSeverity {
		return fmt.Errorf("%w: %s", ErrInvalidSeverity, pattern.Severity)
	}

	return nil
}

func (pm *PatternManager) matchPattern(pattern *ModerationPattern, content *ContentToModerate) *PatternMatch {
	var matched bool
	var confidence float64
	var matchedText string

	switch pattern.Type {
	case "keyword":
		matched, matchedText = pm.matchKeyword(pattern.Content, content.Text)
		confidence = 0.8

	case "regex":
		matched, matchedText = pm.matchRegex(pattern.Content, content.Text)
		confidence = 0.9

	case "phrase":
		matched, matchedText = pm.matchPhrase(pattern.Content, content.Text)
		confidence = 0.85

	case "domain":
		matched, matchedText = pm.matchDomain(pattern.Content, content.Text)
		confidence = 0.95

	case "ip":
		matched, matchedText = pm.matchIP(pattern.Content, content.Text)
		confidence = 0.95

	case "hash":
		matched, matchedText = pm.matchHash(pattern.Content, content)
		confidence = 1.0
	}

	if !matched {
		return nil
	}

	return &PatternMatch{
		PatternID:   pattern.ID,
		PatternName: pattern.Name,
		PatternType: pattern.Type,
		Severity:    pattern.Severity,
		Confidence:  confidence,
		MatchedText: matchedText,
		Action:      pattern.Action,
		MatchedAt:   time.Now(),
	}
}

func (pm *PatternManager) matchKeyword(keyword, text string) (bool, string) {
	lowerText := strings.ToLower(text)
	lowerKeyword := strings.ToLower(keyword)

	if strings.Contains(lowerText, lowerKeyword) {
		return true, keyword
	}
	return false, ""
}

func (pm *PatternManager) matchRegex(regexPattern, text string) (bool, string) {
	re, err := regexp.Compile(regexPattern)
	if err != nil {
		return false, ""
	}

	match := re.FindString(text)
	if match != "" {
		return true, match
	}
	return false, ""
}

func (pm *PatternManager) matchPhrase(phrase, text string) (bool, string) {
	lowerText := strings.ToLower(text)
	lowerPhrase := strings.ToLower(phrase)

	if strings.Contains(lowerText, lowerPhrase) {
		return true, phrase
	}
	return false, ""
}

func (pm *PatternManager) matchDomain(domain, text string) (bool, string) {
	// Enhanced domain matching if available
	if pm.enhancedEnabled && pm.cacheManager != nil {
		return pm.matchEnhancedURL(domain, text, "url_domain")
	}

	// Fallback to simple domain matching
	if strings.Contains(text, domain) {
		return true, domain
	}
	return false, ""
}

func (pm *PatternManager) matchIP(ipPattern, text string) (bool, string) {
	// Enhanced IP matching if available
	if pm.enhancedEnabled && pm.cacheManager != nil {
		return pm.matchEnhancedIP(ipPattern, text, "ip_single")
	}

	// Fallback to simple IP matching
	if strings.Contains(text, ipPattern) {
		return true, ipPattern
	}
	return false, ""
}

func (pm *PatternManager) matchHash(hash string, content *ContentToModerate) (bool, string) {
	// Match against content hashes
	if content.TextHash == hash || content.ImageHash == hash {
		return true, hash
	}
	return false, ""
}

func (pm *PatternManager) calculateEffectiveness(pattern *ModerationPattern) float64 {
	if pattern.MatchCount == 0 {
		return 0.5 // Neutral for new patterns
	}

	truePositives := pattern.MatchCount - pattern.FalsePositiveCount
	if truePositives < 0 {
		truePositives = 0
	}

	effectiveness := float64(truePositives) / float64(pattern.MatchCount)

	// Adjust for recency - patterns that haven't matched recently are less effective
	if !pattern.LastMatch.IsZero() {
		daysSinceLastMatch := time.Since(pattern.LastMatch).Hours() / 24
		if daysSinceLastMatch > 30 {
			effectiveness *= 0.5 // Reduce effectiveness for stale patterns
		}
	}

	return effectiveness
}

func (pm *PatternManager) calculateTruePositiveRate(pattern *ModerationPattern) float64 {
	if pattern.MatchCount == 0 {
		return 0
	}

	truePositives := pattern.MatchCount - pattern.FalsePositiveCount
	if truePositives < 0 {
		truePositives = 0
	}

	return float64(truePositives) / float64(pattern.MatchCount)
}

func (pm *PatternManager) recordMatch(ctx context.Context, patternID string, matched bool) error {
	return pm.storage.RecordPatternMatch(ctx, patternID, matched, time.Now())
}

func (pm *PatternManager) generatePatternRecommendations(pattern *ModerationPattern) []string {
	var recommendations []string

	if pattern.Effectiveness < 0.3 {
		recommendations = append(recommendations, "Consider disabling or refining this pattern due to low effectiveness")
	}

	if pattern.FalsePositiveCount > pattern.MatchCount/2 {
		recommendations = append(recommendations, "High false positive rate - consider making pattern more specific")
	}

	if pattern.MatchCount == 0 && time.Since(pattern.CreatedAt) > 7*24*time.Hour {
		recommendations = append(recommendations, "Pattern has no matches after 7 days - consider reviewing relevance")
	}

	if time.Since(pattern.LastMatch) > 30*24*time.Hour && pattern.MatchCount > 0 {
		recommendations = append(recommendations, "No recent matches - pattern may be outdated")
	}

	return recommendations
}

func (pm *PatternManager) generateOverallRecommendations(patterns, ineffectivePatterns []*ModerationPattern) []string {
	var recommendations []string

	if err := common.ValidateSliceNotEmpty("ineffectivePatterns", ineffectivePatterns); err == nil && len(ineffectivePatterns) > len(patterns)/4 {
		recommendations = append(recommendations, "High number of ineffective patterns - consider pattern cleanup")
	}

	// Count patterns by type
	typeCount := make(map[string]int)
	for _, pattern := range patterns {
		typeCount[pattern.Type]++
	}

	if typeCount["regex"] > len(patterns)/2 {
		recommendations = append(recommendations, "High number of regex patterns - consider performance impact")
	}

	if len(patterns) < 10 {
		recommendations = append(recommendations, "Consider adding more patterns for comprehensive moderation")
	}

	return recommendations
}

func (pm *PatternManager) analyzePatternForOptimization(pattern *ModerationPattern) *PatternOptimization {
	if pattern.Effectiveness > 0.7 {
		return nil // Pattern is already effective
	}

	optimization := &PatternOptimization{
		PatternID:            pattern.ID,
		PatternName:          pattern.Name,
		CurrentEffectiveness: pattern.Effectiveness,
	}

	// Generate optimization suggestions based on pattern type and performance
	if pattern.Type == "keyword" && pattern.FalsePositiveCount > 0 {
		optimization.Suggestions = append(optimization.Suggestions, OptimizationSuggestion{
			Type:        "specificity",
			Description: "Consider converting to phrase or regex for more precise matching",
			Impact:      "medium",
		})
	}

	if pattern.MatchCount == 0 {
		optimization.Suggestions = append(optimization.Suggestions, OptimizationSuggestion{
			Type:        "relevance",
			Description: "Pattern has no matches - consider updating or removing",
			Impact:      "high",
		})
	}

	if pattern.FalsePositiveCount > pattern.MatchCount/3 {
		optimization.Suggestions = append(optimization.Suggestions, OptimizationSuggestion{
			Type:        "precision",
			Description: "High false positive rate - add context or refine pattern",
			Impact:      "high",
		})
	}

	if err := common.ValidateSliceNotEmpty("optimization.Suggestions", optimization.Suggestions); err != nil {
		return nil
	}

	return optimization
}

// Enhanced pattern matching methods

// matchEnhancedURL performs enhanced URL pattern matching
func (pm *PatternManager) matchEnhancedURL(pattern, text, patternType string) (bool, string) {
	if pm.cacheManager == nil {
		return false, ""
	}

	// Create a temporary enhanced pattern for matching
	enhancedPattern := &models.EnhancedModerationPattern{
		PatternID:      "temp",
		PatternContent: pattern,
		PatternType:    patternType,
		Active:         true,
	}

	ctx := context.Background()
	matched, matchedPattern, err := pm.cacheManager.MatchURL(ctx, text, []*models.EnhancedModerationPattern{enhancedPattern})
	if err != nil {
		pm.logger.Warn("enhanced URL matching failed, falling back to simple matching",
			zap.String("pattern", pattern),
			zap.Error(err))
		return false, ""
	}

	if matched && matchedPattern != nil {
		return true, matchedPattern.PatternContent
	}

	return false, ""
}

// matchEnhancedIP performs enhanced IP pattern matching
func (pm *PatternManager) matchEnhancedIP(pattern, text, patternType string) (bool, string) {
	if pm.cacheManager == nil {
		return false, ""
	}

	// Create a temporary enhanced pattern for matching
	enhancedPattern := &models.EnhancedModerationPattern{
		PatternID:      "temp",
		PatternContent: pattern,
		PatternType:    patternType,
		Active:         true,
	}

	ctx := context.Background()
	matched, matchedPattern, err := pm.cacheManager.MatchIP(ctx, text, []*models.EnhancedModerationPattern{enhancedPattern})
	if err != nil {
		pm.logger.Warn("enhanced IP matching failed, falling back to simple matching",
			zap.String("pattern", pattern),
			zap.Error(err))
		return false, ""
	}

	if matched && matchedPattern != nil {
		return true, matchedPattern.PatternContent
	}

	return false, ""
}

// CreateEnhancedPattern creates a new enhanced moderation pattern
func (pm *PatternManager) CreateEnhancedPattern(ctx context.Context, pattern *models.EnhancedModerationPattern) error {
	if !pm.enhancedEnabled || pm.enhancedRepo == nil {
		return ErrEnhancedPatternsNotAvailable
	}

	// Validate pattern first
	if pm.patternValidator != nil {
		validationResult, err := pm.patternValidator.ValidatePattern(ctx, pattern, DefaultSecurityTestConfig())
		if err != nil {
			return fmt.Errorf("%w: %w", ErrPatternValidationFailed, err)
		}

		if !validationResult.Valid {
			return fmt.Errorf("%w: %v", ErrPatternValidationFailed, validationResult.Errors)
		}

		// Update pattern with validation results
		pattern.ValidationScore = validationResult.Score
		pattern.ConfidenceScore = validationResult.AccuracyScore
	}

	return pm.enhancedRepo.CreatePattern(ctx, pattern)
}

// GetEnhancedPattern retrieves an enhanced pattern by ID
func (pm *PatternManager) GetEnhancedPattern(ctx context.Context, patternID string) (*models.EnhancedModerationPattern, error) {
	if !pm.enhancedEnabled || pm.enhancedRepo == nil {
		return nil, ErrEnhancedPatternsNotAvailable
	}

	return pm.enhancedRepo.GetPattern(ctx, patternID)
}

// UpdateEnhancedPattern updates an enhanced pattern
func (pm *PatternManager) UpdateEnhancedPattern(ctx context.Context, pattern *models.EnhancedModerationPattern) error {
	if !pm.enhancedEnabled || pm.enhancedRepo == nil {
		return ErrEnhancedPatternsNotAvailable
	}

	// Re-validate pattern if validator is available
	if pm.patternValidator != nil {
		validationResult, err := pm.patternValidator.ValidatePattern(ctx, pattern, DefaultSecurityTestConfig())
		if err != nil {
			pm.logger.Warn("pattern re-validation failed during update",
				zap.String("pattern_id", pattern.PatternID),
				zap.Error(err))
		} else {
			pattern.ValidationScore = validationResult.Score
		}
	}

	// Invalidate cache for this pattern
	if pm.cacheManager != nil {
		err := pm.cacheManager.InvalidatePattern(ctx, pattern.PatternID, pattern.PatternContent, pattern.PatternType)
		if err != nil {
			pm.logger.Warn("failed to invalidate pattern cache",
				zap.String("pattern_id", pattern.PatternID),
				zap.Error(err))
		}
	}

	return pm.enhancedRepo.UpdatePattern(ctx, pattern)
}

// DeleteEnhancedPattern deletes an enhanced pattern
func (pm *PatternManager) DeleteEnhancedPattern(ctx context.Context, patternID string) error {
	if !pm.enhancedEnabled || pm.enhancedRepo == nil {
		return ErrEnhancedPatternsNotAvailable
	}

	// Get pattern to get its details for cache invalidation
	pattern, err := pm.enhancedRepo.GetPattern(ctx, patternID)
	if err != nil {
		return err
	}

	// Invalidate cache for this pattern
	if pm.cacheManager != nil {
		err := pm.cacheManager.InvalidatePattern(ctx, pattern.PatternID, pattern.PatternContent, pattern.PatternType)
		if err != nil {
			pm.logger.Warn("failed to invalidate pattern cache during deletion",
				zap.String("pattern_id", patternID),
				zap.Error(err))
		}
	}

	return pm.enhancedRepo.DeletePattern(ctx, patternID)
}

// GetActiveEnhancedPatterns retrieves all active enhanced patterns
func (pm *PatternManager) GetActiveEnhancedPatterns(ctx context.Context, limit int) ([]*models.EnhancedModerationPattern, error) {
	if !pm.enhancedEnabled || pm.enhancedRepo == nil {
		return nil, ErrEnhancedPatternsNotAvailable
	}

	return pm.enhancedRepo.GetActivePatterns(ctx, limit)
}

// MatchContentEnhanced matches content against enhanced patterns
func (pm *PatternManager) MatchContentEnhanced(ctx context.Context, content *ContentToModerate) ([]*EnhancedPatternMatch, error) {
	if !pm.enhancedEnabled || pm.enhancedRepo == nil {
		return nil, ErrEnhancedPatternsNotAvailable
	}

	// Get active enhanced patterns
	patterns, err := pm.enhancedRepo.GetActivePatterns(ctx, 1000)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailedToGetEnhancedPatterns, err)
	}

	var matches []*EnhancedPatternMatch

	// Separate URL and IP patterns
	urlPatterns := make([]*models.EnhancedModerationPattern, 0)
	ipPatterns := make([]*models.EnhancedModerationPattern, 0)

	for _, pattern := range patterns {
		if isURLPatternType(pattern.PatternType) {
			urlPatterns = append(urlPatterns, pattern)
		} else if isIPPatternType(pattern.PatternType) {
			ipPatterns = append(ipPatterns, pattern)
		}
	}

	// Match URLs in content
	if err := common.ValidateSliceNotEmpty("urlPatterns", urlPatterns); err == nil {
		urlMatches := pm.extractAndMatchURLs(ctx, content.Text, urlPatterns)
		matches = append(matches, urlMatches...)
	}

	// Match IPs in content
	if err := common.ValidateSliceNotEmpty("ipPatterns", ipPatterns); err == nil {
		ipMatches := pm.extractAndMatchIPs(ctx, content.Text, ipPatterns)
		matches = append(matches, ipMatches...)
	}

	// Record matches for pattern statistics
	recordCtx := context.WithoutCancel(ctx)
	for _, match := range matches {
		go func(patternID string, matchTime float64) {
			err := pm.enhancedRepo.RecordMatch(recordCtx, patternID, true, true, matchTime)
			if err != nil {
				pm.logger.Warn("failed to record pattern match",
					zap.String("pattern_id", patternID),
					zap.Error(err))
			}
		}(match.PatternID, match.MatchTime)
	}

	return matches, nil
}

// extractAndMatchURLs extracts URLs from text and matches them against patterns
func (pm *PatternManager) extractAndMatchURLs(ctx context.Context, text string, patterns []*models.EnhancedModerationPattern) []*EnhancedPatternMatch {
	matches := make([]*EnhancedPatternMatch, 0)

	// Simple URL extraction - look for http/https URLs
	urlRegex := regexp.MustCompile(`https?://[^\s<>"{}|\\^` + "`" + `\[\]]+`)
	urls := urlRegex.FindAllString(text, -1)

	// Also check for domain-like patterns
	domainRegex := regexp.MustCompile(`[a-zA-Z0-9][a-zA-Z0-9-]{1,61}[a-zA-Z0-9]\.[a-zA-Z]{2,}`)
	domains := domainRegex.FindAllString(text, -1)

	allURLs := append(urls, domains...)

	for _, url := range allURLs {
		start := time.Now()
		matched, pattern, err := pm.cacheManager.MatchURL(ctx, url, patterns)
		matchTime := float64(time.Since(start).Nanoseconds()) / 1e6

		if err != nil {
			pm.logger.Warn("URL matching error",
				zap.String("url", url),
				zap.Error(err))
			continue
		}

		if matched && pattern != nil {
			matches = append(matches, &EnhancedPatternMatch{
				PatternID:     pattern.PatternID,
				PatternName:   pattern.Name,
				PatternType:   pattern.PatternType,
				Category:      pattern.Category,
				Severity:      pattern.Severity,
				Action:        pattern.Action,
				Priority:      pattern.Priority,
				Confidence:    pattern.ConfidenceScore,
				MatchedText:   url,
				MatchTime:     matchTime,
				MatchedAt:     time.Now(),
				Effectiveness: pattern.Effectiveness,
			})
		}
	}

	return matches
}

// extractAndMatchIPs extracts IP addresses from text and matches them against patterns
func (pm *PatternManager) extractAndMatchIPs(ctx context.Context, text string, patterns []*models.EnhancedModerationPattern) []*EnhancedPatternMatch {
	matches := make([]*EnhancedPatternMatch, 0)

	// Extract IPv4 addresses
	ipv4Regex := regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)
	ipv4s := ipv4Regex.FindAllString(text, -1)

	// Extract IPv6 addresses
	ipv6Regex := regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\b`)
	ipv6s := ipv6Regex.FindAllString(text, -1)

	allIPs := append(ipv4s, ipv6s...)

	for _, ip := range allIPs {
		start := time.Now()
		matched, pattern, err := pm.cacheManager.MatchIP(ctx, ip, patterns)
		matchTime := float64(time.Since(start).Nanoseconds()) / 1e6

		if err != nil {
			pm.logger.Warn("IP matching error",
				zap.String("ip", ip),
				zap.Error(err))
			continue
		}

		if matched && pattern != nil {
			matches = append(matches, &EnhancedPatternMatch{
				PatternID:     pattern.PatternID,
				PatternName:   pattern.Name,
				PatternType:   pattern.PatternType,
				Category:      pattern.Category,
				Severity:      pattern.Severity,
				Action:        pattern.Action,
				Priority:      pattern.Priority,
				Confidence:    pattern.ConfidenceScore,
				MatchedText:   ip,
				MatchTime:     matchTime,
				MatchedAt:     time.Now(),
				Effectiveness: pattern.Effectiveness,
			})
		}
	}

	return matches
}

// ValidateEnhancedPattern validates an enhanced pattern
func (pm *PatternManager) ValidateEnhancedPattern(ctx context.Context, pattern *models.EnhancedModerationPattern) (*ValidationResult, error) {
	if !pm.enhancedEnabled || pm.patternValidator == nil {
		return nil, ErrEnhancedPatternValidationNotAvailable
	}

	return pm.patternValidator.ValidatePattern(ctx, pattern, DefaultSecurityTestConfig())
}

// GetCacheStatistics returns pattern cache statistics
func (pm *PatternManager) GetCacheStatistics() *CacheStatistics {
	if !pm.enhancedEnabled || pm.cacheManager == nil {
		return nil
	}

	return pm.cacheManager.GetStatistics()
}

// GetPatternStatistics returns enhanced pattern statistics
func (pm *PatternManager) GetPatternStatistics(ctx context.Context) (map[string]interface{}, error) {
	if !pm.enhancedEnabled || pm.enhancedRepo == nil {
		return nil, ErrEnhancedPatternStatisticsNotAvailable
	}

	return pm.enhancedRepo.GetPatternStatistics(ctx)
}

// IsEnhancedEnabled returns whether enhanced pattern matching is enabled
func (pm *PatternManager) IsEnhancedEnabled() bool {
	return pm.enhancedEnabled
}

// EnhancedPatternMatch represents a match from enhanced pattern matching
type EnhancedPatternMatch struct {
	PatternID     string    `json:"pattern_id"`
	PatternName   string    `json:"pattern_name"`
	PatternType   string    `json:"pattern_type"`
	Category      string    `json:"category"`
	Severity      string    `json:"severity"`
	Action        string    `json:"action"`
	Priority      int       `json:"priority"`
	Confidence    float64   `json:"confidence"`
	MatchedText   string    `json:"matched_text"`
	MatchTime     float64   `json:"match_time"` // Time taken to match in milliseconds
	MatchedAt     time.Time `json:"matched_at"`
	Effectiveness float64   `json:"effectiveness"` // Pattern effectiveness score
}

// Types for pattern management

// ModerationPattern represents a pattern for content moderation
//
//nolint:revive // Moderation prefix clarifies this is moderation-specific pattern
type ModerationPattern struct {
	ID                 string    `json:"id" dynamodbav:"id"`
	Name               string    `json:"name" dynamodbav:"name"`
	Description        string    `json:"description" dynamodbav:"description"`
	Type               string    `json:"type" dynamodbav:"type"` // keyword/regex/phrase/domain/ip/hash
	Content            string    `json:"content" dynamodbav:"content"`
	Severity           string    `json:"severity" dynamodbav:"severity"` // low/medium/high/critical
	Action             string    `json:"action" dynamodbav:"action"`     // flag/hide/block/escalate
	Active             bool      `json:"active" dynamodbav:"active"`
	MatchCount         int64     `json:"match_count" dynamodbav:"match_count"`
	FalsePositiveCount int64     `json:"false_positive_count" dynamodbav:"false_positive_count"`
	Effectiveness      float64   `json:"effectiveness" dynamodbav:"effectiveness"`
	LastMatch          time.Time `json:"last_match" dynamodbav:"last_match"`
	CreatedAt          time.Time `json:"created_at" dynamodbav:"created_at"`
	CreatedBy          string    `json:"created_by" dynamodbav:"created_by"`
	UpdatedAt          time.Time `json:"updated_at" dynamodbav:"updated_at"`
	Tags               []string  `json:"tags,omitempty" dynamodbav:"tags,omitempty"`
}

// ContentToModerate represents content to be checked against patterns
type ContentToModerate struct {
	ID        string         `json:"id"`
	Text      string         `json:"text"`
	ImageHash string         `json:"image_hash,omitempty"`
	TextHash  string         `json:"text_hash,omitempty"`
	Author    string         `json:"author"`
	Type      string         `json:"type"` // post/comment/message/profile
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// PatternMatch represents a matched pattern in content
type PatternMatch struct {
	PatternID   string    `json:"pattern_id"`
	PatternName string    `json:"pattern_name"`
	PatternType string    `json:"pattern_type"`
	Severity    string    `json:"severity"`
	Confidence  float64   `json:"confidence"`
	MatchedText string    `json:"matched_text"`
	Action      string    `json:"action"`
	MatchedAt   time.Time `json:"matched_at"`
}

// PatternEffectivenessReport represents a report on pattern effectiveness
type PatternEffectivenessReport struct {
	GeneratedAt              time.Time          `json:"generated_at"`
	TotalPatterns            int                `json:"total_patterns"`
	AverageEffectiveness     float64            `json:"average_effectiveness"`
	OverallFalsePositiveRate float64            `json:"overall_false_positive_rate"`
	InefficientPatterns      int                `json:"inefficient_patterns"`
	PatternAnalysis          []*PatternAnalysis `json:"pattern_analysis"`
	Recommendations          []string           `json:"recommendations"`
}

// PatternAnalysis represents detailed analysis of a pattern
type PatternAnalysis struct {
	PatternID          string    `json:"pattern_id"`
	PatternType        string    `json:"pattern_type"`
	Severity           string    `json:"severity"`
	MatchCount         int64     `json:"match_count"`
	FalsePositiveCount int64     `json:"false_positive_count"`
	Effectiveness      float64   `json:"effectiveness"`
	TruePositiveRate   float64   `json:"true_positive_rate"`
	Performance        string    `json:"performance"` // poor/moderate/good
	LastMatch          time.Time `json:"last_match"`
	CreatedAt          time.Time `json:"created_at"`
	Recommendations    []string  `json:"recommendations"`
}

// PatternOptimization represents optimization recommendations for patterns
type PatternOptimization struct {
	PatternID            string                   `json:"pattern_id"`
	PatternName          string                   `json:"pattern_name"`
	CurrentEffectiveness float64                  `json:"current_effectiveness"`
	Suggestions          []OptimizationSuggestion `json:"suggestions"`
}

// OptimizationSuggestion represents a suggested optimization for a pattern
type OptimizationSuggestion struct {
	Type        string `json:"type"` // specificity/relevance/precision/performance
	Description string `json:"description"`
	Impact      string `json:"impact"` // low/medium/high
}
