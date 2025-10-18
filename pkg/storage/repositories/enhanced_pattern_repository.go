package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// EnhancedPatternRepository handles enhanced moderation pattern storage operations
type EnhancedPatternRepository struct {
	*EnhancedBaseRepository[*models.EnhancedModerationPattern]
	// Additional fields for pattern-specific operations
}

// NewEnhancedPatternRepository creates a new enhanced pattern repository with enhanced functionality
func NewEnhancedPatternRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *EnhancedPatternRepository {
	// Create enhanced repository optimized for enhanced pattern operations
	enhancedRepo := NewEnhancedBaseRepository[*models.EnhancedModerationPattern](db, tableName, logger, costService, "EnhancedPatternRepository", "enhanced_pattern")

	// Set up enhanced services for pattern operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Patterns cached for performance
	enhancedRepo.SetEventService(NewDefaultEventService())      // Pattern change events

	return &EnhancedPatternRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreatePattern creates a new enhanced moderation pattern
func (r *EnhancedPatternRepository) CreatePattern(ctx context.Context, pattern *models.EnhancedModerationPattern) error {
	if pattern == nil {
		return fmt.Errorf("%w: %w", storage.ErrPatternCreateFailed, storage.ErrNilPattern)
	}

	// Set timestamps and defaults
	now := time.Now()
	pattern.CreatedAt = now
	pattern.UpdatedAt = now
	pattern.Version = 1
	pattern.MatchCount = 0
	pattern.FalsePositiveCount = 0
	pattern.TruePositiveCount = 0
	pattern.ConfidenceScore = 0.5 // Default neutral confidence
	pattern.ValidationScore = 0.0 // Will be set after validation

	// Calculate initial effectiveness
	pattern.CalculateEffectiveness()

	// Use BaseRepository Create method
	err := r.ValidateAndCreate(ctx, pattern)
	if err != nil {
		return fmt.Errorf("%w: %w", storage.ErrPatternCreateFailed, err)
	}

	r.logger.Info("created enhanced moderation pattern",
		zap.String("pattern_id", pattern.PatternID),
		zap.String("pattern_type", pattern.PatternType),
		zap.String("category", pattern.Category),
		zap.Int("priority", pattern.Priority))

	return nil
}

// GetPattern retrieves an enhanced pattern by ID
func (r *EnhancedPatternRepository) GetPattern(ctx context.Context, patternID string) (*models.EnhancedModerationPattern, error) {
	pattern := &models.EnhancedModerationPattern{}
	pk := fmt.Sprintf("ENHANCED_PATTERN#%s", patternID)
	sk := models.SKMetadata

	err := r.Get(ctx, pk, sk, pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", storage.ErrPatternNotFound, err)
	}

	return pattern, nil
}

// UpdatePattern updates an existing enhanced pattern
func (r *EnhancedPatternRepository) UpdatePattern(ctx context.Context, pattern *models.EnhancedModerationPattern) error {
	if pattern == nil {
		return fmt.Errorf("%w: %w", storage.ErrPatternUpdateFailed, storage.ErrNilPattern)
	}

	// Update timestamp and recalculate effectiveness
	pattern.UpdatedAt = time.Now()
	pattern.CalculateEffectiveness()

	// Use BaseRepository Update method
	err := r.ValidateAndUpdate(ctx, pattern)
	if err != nil {
		return fmt.Errorf("%w: %w", storage.ErrPatternUpdateFailed, err)
	}

	r.logger.Info("updated enhanced moderation pattern",
		zap.String("pattern_id", pattern.PatternID),
		zap.Float64("effectiveness", pattern.Effectiveness))

	return nil
}

// DeletePattern soft deletes a pattern by marking it inactive
func (r *EnhancedPatternRepository) DeletePattern(ctx context.Context, patternID string) error {
	pattern, err := r.GetPattern(ctx, patternID)
	if err != nil {
		return fmt.Errorf("%w: %w", storage.ErrPatternNotFound, err)
	}

	// Soft delete by marking inactive
	pattern.Active = false
	pattern.UpdatedAt = time.Now()

	// Use BaseRepository Update method for soft delete
	err = r.ValidateAndUpdate(ctx, pattern)
	if err != nil {
		return fmt.Errorf("%w: %w", storage.ErrPatternDeleteFailed, err)
	}

	r.logger.Info("deleted enhanced moderation pattern",
		zap.String("pattern_id", patternID))

	return nil
}

// GetActivePatterns retrieves all active patterns ordered by priority
func (r *EnhancedPatternRepository) GetActivePatterns(ctx context.Context, limit int) ([]*models.EnhancedModerationPattern, error) {
	patterns := []*models.EnhancedModerationPattern{}

	query := r.db.WithContext(ctx).Model(&models.EnhancedModerationPattern{}).
		Where("GSI1PK", "=", "ENHANCED_PATTERNS#ACTIVE")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&patterns)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", storage.ErrPatternQueryFailed, err)
	}

	return patterns, nil
}

// GetPatternsByType retrieves patterns by type ordered by effectiveness
func (r *EnhancedPatternRepository) GetPatternsByType(ctx context.Context, patternType string, limit int) ([]*models.EnhancedModerationPattern, error) {
	patterns := []*models.EnhancedModerationPattern{}

	query := r.db.WithContext(ctx).Model(&models.EnhancedModerationPattern{}).
		Where("GSI2PK", "=", fmt.Sprintf("ENHANCED_PATTERNS#%s", patternType)).
		OrderBy("GSI2SK", "DESC") // Descending order for best effectiveness first

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&patterns)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", storage.ErrPatternQueryFailed, err)
	}

	return patterns, nil
}

// GetPatternsByCategory retrieves patterns by category ordered by effectiveness
func (r *EnhancedPatternRepository) GetPatternsByCategory(ctx context.Context, category string, limit int) ([]*models.EnhancedModerationPattern, error) {
	patterns := []*models.EnhancedModerationPattern{}

	query := r.db.WithContext(ctx).Model(&models.EnhancedModerationPattern{}).
		Where("GSI3PK", "=", fmt.Sprintf("PATTERN_METRICS#%s", category)).
		OrderBy("GSI3SK", "DESC") // Descending order for best effectiveness first

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&patterns)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", storage.ErrPatternQueryFailed, err)
	}

	return patterns, nil
}

// ===== ENHANCED PATTERN ANALYSIS AND DETECTION BUSINESS LOGIC =====
// These methods implement advanced pattern matching, spam detection, and content analysis

// AnalyzeContentPatterns performs ML-based content analysis using enhanced patterns
func (r *EnhancedPatternRepository) AnalyzeContentPatterns(ctx context.Context, content string, patterns []*models.EnhancedModerationPattern) (*PatternAnalysis, error) {
	analysis := &PatternAnalysis{
		Content:     content,
		Timestamp:   time.Now(),
		Matches:     make([]*PatternMatch, 0),
		RiskScore:   0.0,
		Categories:  make([]string, 0),
		Confidence:  0.0,
		ProcessTime: 0,
	}

	startTime := time.Now()
	defer func() {
		analysis.ProcessTime = time.Since(startTime).Milliseconds()
	}()

	// Analyze content against each pattern
	for _, pattern := range patterns {
		if !pattern.Active {
			continue
		}

		match, err := r.analyzePatternMatch(ctx, content, pattern)
		if err != nil {
			r.logger.Warn("failed to analyze pattern match",
				zap.String("pattern_id", pattern.PatternID),
				zap.Error(err))
			continue
		}

		if match.IsMatch {
			analysis.Matches = append(analysis.Matches, match)

			// Update pattern usage statistics
			go func(p *models.EnhancedModerationPattern, matchTime float64) {
				updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = r.RecordMatch(updateCtx, p.PatternID, true, false, matchTime) // Assume false positive initially
			}(pattern, match.MatchTime)
		}
	}

	// Calculate overall risk and confidence
	r.calculateAnalysisMetrics(analysis)

	r.logger.Info("completed content pattern analysis",
		zap.String("content_hash", fmt.Sprintf("%x", content[:minInt(10, len(content))])),
		zap.Int("total_patterns", len(patterns)),
		zap.Int("matches", len(analysis.Matches)),
		zap.Float64("risk_score", analysis.RiskScore),
		zap.Int64("process_time_ms", analysis.ProcessTime))

	return analysis, nil
}

// DetectSpamPatterns performs adaptive spam detection with false positive reduction
func (r *EnhancedPatternRepository) DetectSpamPatterns(ctx context.Context, content string, senderInfo *SenderInfo) (*SpamDetectionResult, error) {
	result := &SpamDetectionResult{
		IsSpam:         false,
		SpamScore:      0.0,
		Confidence:     0.0,
		DetectedBy:     make([]string, 0),
		ReasonCodes:    make([]string, 0),
		ProcessingTime: 0,
	}

	startTime := time.Now()
	defer func() {
		result.ProcessingTime = time.Since(startTime).Milliseconds()
	}()

	// Get active spam detection patterns
	spamPatterns, err := r.GetPatternsByCategory(ctx, "spam", 100)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", storage.ErrPatternQueryFailed, err)
	}

	// Filter patterns by effectiveness threshold
	effectivePatterns := make([]*models.EnhancedModerationPattern, 0)
	for _, pattern := range spamPatterns {
		if pattern.Effectiveness >= 0.7 { // Only use highly effective patterns
			effectivePatterns = append(effectivePatterns, pattern)
		}
	}

	// Analyze content against spam patterns
	var totalSpamScore float64
	var matchCount int

	for _, pattern := range effectivePatterns {
		match, err := r.analyzePatternMatch(ctx, content, pattern)
		if err != nil {
			continue
		}

		if match.IsMatch {
			matchCount++
			weightedScore := match.Confidence * pattern.Effectiveness
			totalSpamScore += weightedScore

			result.DetectedBy = append(result.DetectedBy, pattern.PatternID)
			result.ReasonCodes = append(result.ReasonCodes, fmt.Sprintf("%s_%s", pattern.Category, pattern.PatternType))

			// Record pattern usage
			go func(patternID string, matchTime float64) {
				updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = r.RecordMatch(updateCtx, patternID, true, false, matchTime)
			}(pattern.PatternID, match.MatchTime)
		}
	}

	// Calculate final spam score with sender reputation adjustment
	if matchCount > 0 {
		result.SpamScore = totalSpamScore / float64(len(effectivePatterns))

		// Adjust for sender reputation
		if senderInfo != nil {
			reputationAdjustment := r.calculateReputationAdjustment(senderInfo)
			result.SpamScore = result.SpamScore * reputationAdjustment
		}

		// Determine if spam based on threshold
		result.IsSpam = result.SpamScore > 0.6
		result.Confidence = minFloat64(result.SpamScore, 1.0)
	}

	r.logger.Info("completed spam detection analysis",
		zap.Bool("is_spam", result.IsSpam),
		zap.Float64("spam_score", result.SpamScore),
		zap.Float64("confidence", result.Confidence),
		zap.Int("patterns_matched", matchCount),
		zap.Int64("processing_time_ms", result.ProcessingTime))

	return result, nil
}

// UpdatePatternEffectiveness updates pattern effectiveness based on feedback and accuracy tracking
func (r *EnhancedPatternRepository) UpdatePatternEffectiveness(ctx context.Context, patternID string, feedback *PatternFeedback) error {
	pattern, err := r.GetPattern(ctx, patternID)
	if err != nil {
		return fmt.Errorf("%w: %w", storage.ErrPatternNotFound, err)
	}

	// Update statistics based on feedback
	switch feedback.FeedbackType {
	case "true_positive":
		pattern.TruePositiveCount++
	case "false_positive":
		pattern.FalsePositiveCount++
	case "false_negative":
		// This requires more complex handling as we need to track missed detections
		pattern.ValidationScore = maxFloat64(0.0, pattern.ValidationScore-0.1)
	}

	// Recalculate effectiveness with new data
	pattern.CalculateEffectiveness()

	// Update confidence based on recent accuracy
	recentAccuracy := r.calculateRecentAccuracy(ctx, patternID)
	pattern.ConfidenceScore = (pattern.ConfidenceScore * 0.8) + (recentAccuracy * 0.2) // Weighted moving average

	// Save updated pattern
	err = r.UpdatePattern(ctx, pattern)
	if err != nil {
		return fmt.Errorf("%w: %w", storage.ErrPatternUpdateFailed, err)
	}

	r.logger.Info("updated pattern effectiveness",
		zap.String("pattern_id", patternID),
		zap.String("feedback_type", feedback.FeedbackType),
		zap.Float64("new_effectiveness", pattern.Effectiveness),
		zap.Float64("new_confidence", pattern.ConfidenceScore))

	return nil
}

// GetOptimalPatterns retrieves patterns optimized for performance and accuracy
func (r *EnhancedPatternRepository) GetOptimalPatterns(ctx context.Context, category string, maxPatterns int) ([]*models.EnhancedModerationPattern, error) {
	// Get all patterns for category
	allPatterns, err := r.GetPatternsByCategory(ctx, category, 0) // No limit initially
	if err != nil {
		return nil, fmt.Errorf("%w: %w", storage.ErrPatternQueryFailed, err)
	}

	// Filter and score patterns based on multiple criteria
	scoredPatterns := make([]*ScoredPattern, 0, len(allPatterns))
	for _, pattern := range allPatterns {
		if !pattern.Active || pattern.Effectiveness < 0.3 {
			continue // Skip inactive or low-effectiveness patterns
		}

		score := r.calculateOptimalityScore(pattern)
		scoredPatterns = append(scoredPatterns, &ScoredPattern{
			Pattern: pattern,
			Score:   score,
		})
	}

	// Sort by optimality score
	for i := 0; i < len(scoredPatterns)-1; i++ {
		for j := i + 1; j < len(scoredPatterns); j++ {
			if scoredPatterns[i].Score < scoredPatterns[j].Score {
				scoredPatterns[i], scoredPatterns[j] = scoredPatterns[j], scoredPatterns[i]
			}
		}
	}

	// Return top patterns
	limit := minInt(maxPatterns, len(scoredPatterns))
	result := make([]*models.EnhancedModerationPattern, limit)
	for i := 0; i < limit; i++ {
		result[i] = scoredPatterns[i].Pattern
	}

	r.logger.Info("retrieved optimal patterns",
		zap.String("category", category),
		zap.Int("total_patterns", len(allPatterns)),
		zap.Int("optimal_patterns", len(result)),
		zap.Int("max_requested", maxPatterns))

	return result, nil
}

// LearnFromFeedback implements continuous improvement based on user feedback
func (r *EnhancedPatternRepository) LearnFromFeedback(ctx context.Context, feedbackBatch []*PatternFeedback) error {
	if len(feedbackBatch) == 0 {
		return nil
	}

	// Group feedback by pattern
	feedbackMap := make(map[string][]*PatternFeedback)
	for _, feedback := range feedbackBatch {
		feedbackMap[feedback.PatternID] = append(feedbackMap[feedback.PatternID], feedback)
	}

	// Process feedback for each pattern
	for patternID, feedbacks := range feedbackMap {
		err := r.processFeedbackBatch(ctx, patternID, feedbacks)
		if err != nil {
			r.logger.Error("failed to process feedback batch",
				zap.String("pattern_id", patternID),
				zap.Int("feedback_count", len(feedbacks)),
				zap.Error(err))
			continue
		}
	}

	// Analyze trends and suggest pattern improvements
	err := r.analyzeFeedbackTrends(ctx, feedbackBatch)
	if err != nil {
		r.logger.Warn("failed to analyze feedback trends", zap.Error(err))
	}

	r.logger.Info("completed feedback learning cycle",
		zap.Int("total_feedback", len(feedbackBatch)),
		zap.Int("patterns_updated", len(feedbackMap)))

	return nil
}

// RecordMatch records a pattern match and updates statistics
func (r *EnhancedPatternRepository) RecordMatch(ctx context.Context, patternID string, isMatch bool, isTruePositive bool, matchTime float64) error {
	pattern, err := r.GetPattern(ctx, patternID)
	if err != nil {
		return fmt.Errorf("%w: %w", storage.ErrPatternNotFound, err)
	}

	// Update match statistics
	if isMatch {
		pattern.MatchCount++
		pattern.LastMatch = time.Now()
		pattern.LastUsed = time.Now()

		if isTruePositive {
			pattern.TruePositiveCount++
		} else {
			pattern.FalsePositiveCount++
		}

		// Update average match time
		if pattern.AverageMatchTime == 0 {
			pattern.AverageMatchTime = matchTime
		} else {
			// Simple moving average
			pattern.AverageMatchTime = (pattern.AverageMatchTime + matchTime) / 2
		}
	}

	// Recalculate effectiveness and update
	pattern.CalculateEffectiveness()
	return r.UpdatePattern(ctx, pattern)
}

// ===== PATTERN CACHE MANAGEMENT =====

// GetPatternCache retrieves cached pattern data
func (r *EnhancedPatternRepository) GetPatternCache(ctx context.Context, patternID, patternType string) (*models.PatternCache, error) {
	cache := &models.PatternCache{}
	cache.PK = fmt.Sprintf("PATTERN_CACHE#%s", patternType)
	cache.SK = fmt.Sprintf("COMPILED#%s", patternID)

	err := r.db.WithContext(ctx).Model(cache).
		Where("PK", "=", cache.PK).
		Where("SK", "=", cache.SK).
		First(cache)

	if err != nil {
		return nil, fmt.Errorf("%w: %w", storage.ErrPatternCacheNotFound, err)
	}

	// Update last used and cache hits
	cache.LastUsed = time.Now()
	cache.CacheHits++
	_ = cache.UpdateKeys() // Ignore error as this is internal model operation

	// Update cache statistics (fire and forget)
	go func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = r.db.WithContext(updateCtx).Model(cache).Update()
	}()

	return cache, nil
}

// SetPatternCache stores compiled pattern data in cache
func (r *EnhancedPatternRepository) SetPatternCache(ctx context.Context, cache *models.PatternCache) error {
	if cache == nil {
		return fmt.Errorf("%w: %w", storage.ErrPatternCacheCreateFailed, storage.ErrNilPatternCache)
	}

	now := time.Now()
	cache.CreatedAt = now
	cache.UpdatedAt = now
	cache.LastUsed = now
	cache.CacheHits = 0

	_ = cache.UpdateKeys() // Ignore error as this is internal model operation

	err := r.db.WithContext(ctx).Model(cache).Create()
	if err != nil {
		// Try update if create fails (cache entry might exist)
		cache.UpdatedAt = now
		err = r.db.WithContext(ctx).Model(cache).Update()
		if err != nil {
			return fmt.Errorf("%w: %w", storage.ErrPatternCacheUpdateFailed, err)
		}
	}

	return nil
}

// InvalidatePatternCache removes cached pattern data
func (r *EnhancedPatternRepository) InvalidatePatternCache(ctx context.Context, patternID, patternType string) error {
	cache := &models.PatternCache{}
	cache.PK = fmt.Sprintf("PATTERN_CACHE#%s", patternType)
	cache.SK = fmt.Sprintf("COMPILED#%s", patternID)

	err := r.db.WithContext(ctx).Model(cache).
		Where("PK", "=", cache.PK).
		Where("SK", "=", cache.SK).
		Delete()

	if err != nil {
		r.logger.Warn("failed to invalidate pattern cache",
			zap.String("pattern_id", patternID),
			zap.String("pattern_type", patternType),
			zap.Error(err))
	}

	return nil
}

// ===== PERFORMANCE METRICS AND TESTING =====

// RecordPerformanceMetric records detailed performance metrics
func (r *EnhancedPatternRepository) RecordPerformanceMetric(ctx context.Context, metric *models.PatternPerformanceMetric) error {
	if metric == nil {
		return fmt.Errorf("%w: %w", storage.ErrPatternMetricsCreateFailed, storage.ErrNilPatternMetric)
	}

	now := time.Now()
	metric.CreatedAt = now
	metric.UpdatedAt = now

	// Calculate quality metrics
	metric.CalculateQualityMetrics()

	_ = metric.UpdateKeys() // Ignore error as this is internal model operation

	// Try to get existing metric for this hour
	existing := &models.PatternPerformanceMetric{}
	existing.PK = metric.PK
	existing.SK = metric.SK

	err := r.db.WithContext(ctx).Model(existing).
		Where("PK", "=", existing.PK).
		Where("SK", "=", existing.SK).
		First(existing)

	if err != nil {
		// Create new metric
		err = r.db.WithContext(ctx).Model(metric).Create()
		if err != nil {
			return fmt.Errorf("%w: %w", storage.ErrPatternMetricsCreateFailed, err)
		}
	} else {
		// Update existing metric
		existing.MatchAttempts += metric.MatchAttempts
		existing.SuccessfulMatches += metric.SuccessfulMatches
		existing.FalsePositives += metric.FalsePositives
		existing.TruePositives += metric.TruePositives
		existing.TotalMatchTime += metric.TotalMatchTime
		existing.MemoryUsage = metric.MemoryUsage // Use latest
		existing.CPUTime += metric.CPUTime

		// Recalculate averages
		if existing.MatchAttempts > 0 {
			existing.AverageMatchTime = existing.TotalMatchTime / float64(existing.MatchAttempts)
		}

		// Update min/max
		if metric.MaxMatchTime > existing.MaxMatchTime {
			existing.MaxMatchTime = metric.MaxMatchTime
		}
		if existing.MinMatchTime == 0 || (metric.MinMatchTime > 0 && metric.MinMatchTime < existing.MinMatchTime) {
			existing.MinMatchTime = metric.MinMatchTime
		}

		existing.UpdatedAt = now
		existing.CalculateQualityMetrics()
		_ = existing.UpdateKeys() // Ignore error as this is internal model operation

		err = r.db.WithContext(ctx).Model(existing).Update()
		if err != nil {
			return fmt.Errorf("%w: %w", storage.ErrPatternMetricsUpdateFailed, err)
		}
	}

	return nil
}

// CreateTestResult records pattern test results
func (r *EnhancedPatternRepository) CreateTestResult(ctx context.Context, result *models.PatternTestResult) error {
	if result == nil {
		return fmt.Errorf("%w: %w", storage.ErrPatternTestResultCreateFailed, storage.ErrNilPatternTestResult)
	}

	now := time.Now()
	result.CreatedAt = now
	result.RunAt = now

	_ = result.UpdateKeys() // Ignore error as this is internal model operation

	err := r.db.WithContext(ctx).Model(result).Create()
	if err != nil {
		return fmt.Errorf("%w: %w", storage.ErrPatternTestResultCreateFailed, err)
	}

	r.logger.Info("recorded pattern test result",
		zap.String("pattern_id", result.PatternID),
		zap.String("test_type", result.TestType),
		zap.Bool("passed", result.Passed),
		zap.Float64("score", result.Score))

	return nil
}

// GetTestResults retrieves test results for a pattern
func (r *EnhancedPatternRepository) GetTestResults(ctx context.Context, patternID string, testType string, limit int) ([]*models.PatternTestResult, error) {
	results := []*models.PatternTestResult{}

	query := r.db.WithContext(ctx).Model(&models.PatternTestResult{}).
		Where("PK", "=", fmt.Sprintf("PATTERN_TEST#%s", patternID))

	if testType != "" {
		query = query.Filter("TestType", "=", testType)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&results)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", storage.ErrPatternTestResultQueryFailed, err)
	}

	return results, nil
}

// GetLatestTestResult gets the most recent test result for a pattern and test type
func (r *EnhancedPatternRepository) GetLatestTestResult(ctx context.Context, patternID, testType string) (*models.PatternTestResult, error) {
	results, err := r.GetTestResults(ctx, patternID, testType, 1)
	if err != nil {
		return nil, err
	}

	if err := common.ValidateSliceNotEmpty("results", results); err != nil {
		return nil, fmt.Errorf("%w: no test results found for pattern %s", storage.ErrPatternTestResultNotFound, patternID)
	}

	return results[0], nil
}

// GetPerformanceMetrics retrieves performance metrics for a pattern and date range
func (r *EnhancedPatternRepository) GetPerformanceMetrics(ctx context.Context, patternID, startDate, endDate string) ([]*models.PatternPerformanceMetric, error) {
	metrics := []*models.PatternPerformanceMetric{}

	query := r.db.WithContext(ctx).Model(&models.PatternPerformanceMetric{}).
		Where("PK", "=", fmt.Sprintf("PATTERN_METRICS#%s", patternID))

	if startDate != "" {
		query = query.Filter("SK", ">=", fmt.Sprintf("TIME#%s#00", startDate))
	}
	if endDate != "" {
		query = query.Filter("SK", "<=", fmt.Sprintf("TIME#%s#23", endDate))
	}

	err := query.All(&metrics)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", storage.ErrPatternMetricsQueryFailed, err)
	}

	return metrics, nil
}

// ===== MAINTENANCE AND CLEANUP =====

// CleanupExpiredPatterns removes patterns that have expired
func (r *EnhancedPatternRepository) CleanupExpiredPatterns(ctx context.Context) (int, error) {
	// Get all patterns to check expiration
	patterns := []*models.EnhancedModerationPattern{}
	err := r.db.WithContext(ctx).Model(&models.EnhancedModerationPattern{}).
		Where("SK", "=", models.SKMetadata).
		All(&patterns)

	if err != nil {
		return 0, fmt.Errorf("%w: %w", storage.ErrPatternQueryFailed, err)
	}

	cleanedCount := 0
	for _, pattern := range patterns {
		if pattern.IsExpired() {
			err := r.DeletePattern(ctx, pattern.PatternID)
			if err != nil {
				r.logger.Warn("failed to cleanup expired pattern",
					zap.String("pattern_id", pattern.PatternID),
					zap.Error(err))
			} else {
				cleanedCount++
			}
		}
	}

	if cleanedCount > 0 {
		r.logger.Info("cleaned up expired patterns",
			zap.Int("count", cleanedCount))
	}

	return cleanedCount, nil
}

// GetPatternStatistics returns aggregate statistics for patterns
func (r *EnhancedPatternRepository) GetPatternStatistics(ctx context.Context) (map[string]interface{}, error) {
	patterns := []*models.EnhancedModerationPattern{}
	err := r.db.WithContext(ctx).Model(&models.EnhancedModerationPattern{}).
		Where("SK", "=", models.SKMetadata).
		All(&patterns)

	if err != nil {
		return nil, fmt.Errorf("%w: %w", storage.ErrPatternQueryFailed, err)
	}

	stats := map[string]interface{}{
		"total_patterns":          len(patterns),
		"active_patterns":         0,
		"average_effectiveness":   0.0,
		"total_matches":           int64(0),
		"total_false_positives":   int64(0),
		"total_true_positives":    int64(0),
		"patterns_by_type":        make(map[string]int),
		"patterns_by_category":    make(map[string]int),
		"patterns_by_severity":    make(map[string]int),
		"patterns_needing_review": 0,
	}

	var totalEffectiveness float64
	activeCount := 0

	for _, pattern := range patterns {
		if pattern.Active {
			activeCount++
			stats["active_patterns"] = activeCount
		}

		totalEffectiveness += pattern.Effectiveness
		stats["total_matches"] = stats["total_matches"].(int64) + pattern.MatchCount
		stats["total_false_positives"] = stats["total_false_positives"].(int64) + pattern.FalsePositiveCount
		stats["total_true_positives"] = stats["total_true_positives"].(int64) + pattern.TruePositiveCount

		// Count by type
		typeMap := stats["patterns_by_type"].(map[string]int)
		typeMap[pattern.PatternType]++

		// Count by category
		categoryMap := stats["patterns_by_category"].(map[string]int)
		categoryMap[pattern.Category]++

		// Count by severity
		severityMap := stats["patterns_by_severity"].(map[string]int)
		severityMap[pattern.Severity]++

		// Check if needs review (low effectiveness)
		if pattern.Effectiveness < 0.3 && pattern.MatchCount > 0 {
			stats["patterns_needing_review"] = stats["patterns_needing_review"].(int) + 1
		}
	}

	if len(patterns) > 0 {
		stats["average_effectiveness"] = totalEffectiveness / float64(len(patterns))
	}

	return stats, nil
}

// ===== HELPER METHODS FOR PATTERN ANALYSIS =====

// analyzePatternMatch analyzes if content matches a specific pattern
func (r *EnhancedPatternRepository) analyzePatternMatch(_ context.Context, content string, pattern *models.EnhancedModerationPattern) (*PatternMatch, error) {
	match := &PatternMatch{
		PatternID:   pattern.PatternID,
		PatternType: pattern.PatternType,
		Category:    pattern.Category,
		IsMatch:     false,
		Confidence:  0.0,
		MatchTime:   0,
		Position:    -1,
	}

	startTime := time.Now()
	defer func() {
		match.MatchTime = float64(time.Since(startTime).Nanoseconds()) / 1000000.0 // Convert to milliseconds
	}()

	// Implement pattern matching logic based on pattern type
	// This is a simplified version - in production this would use compiled regex, ML models, etc.
	switch pattern.PatternType {
	case "url_exact":
		match.IsMatch = content == pattern.PatternContent
		if match.IsMatch {
			match.Confidence = 1.0
		}
	case "url_domain", "url_subdomain":
		match.IsMatch = r.matchDomainPattern(content, pattern.PatternContent)
		if match.IsMatch {
			match.Confidence = 0.9
		}
	case "url_regex":
		// In production, this would use compiled regex from cache
		match.IsMatch = r.matchRegexPattern(content, pattern.PatternContent)
		if match.IsMatch {
			match.Confidence = pattern.ConfidenceScore
		}
	default:
		// Generic text matching
		match.IsMatch = r.matchTextPattern(content, pattern.PatternContent)
		if match.IsMatch {
			match.Confidence = pattern.ConfidenceScore
		}
	}

	return match, nil
}

// calculateAnalysisMetrics calculates overall risk and confidence from pattern matches
func (r *EnhancedPatternRepository) calculateAnalysisMetrics(analysis *PatternAnalysis) {
	if len(analysis.Matches) == 0 {
		return
	}

	var totalRisk float64
	var totalConfidence float64
	categorySet := make(map[string]bool)

	for _, match := range analysis.Matches {
		// Weight risk by pattern severity and confidence
		severityWeight := r.getSeverityWeight(match.Severity)
		weightedRisk := match.Confidence * severityWeight
		totalRisk += weightedRisk
		totalConfidence += match.Confidence

		categorySet[match.Category] = true
	}

	// Calculate averages and normalize
	analysis.RiskScore = minFloat64(totalRisk/float64(len(analysis.Matches)), 1.0)
	analysis.Confidence = totalConfidence / float64(len(analysis.Matches))

	// Extract categories
	analysis.Categories = make([]string, 0, len(categorySet))
	for category := range categorySet {
		analysis.Categories = append(analysis.Categories, category)
	}
}

// calculateReputationAdjustment adjusts spam score based on sender reputation
func (r *EnhancedPatternRepository) calculateReputationAdjustment(senderInfo *SenderInfo) float64 {
	// Base adjustment is neutral
	adjustment := 1.0

	// Adjust based on account age
	if senderInfo.AccountAge < 7 { // Less than a week old
		adjustment *= 1.3 // Increase spam likelihood
	} else if senderInfo.AccountAge > 365 { // Older than a year
		adjustment *= 0.8 // Decrease spam likelihood
	}

	// Adjust based on follower count
	if senderInfo.FollowerCount < 10 {
		adjustment *= 1.2
	} else if senderInfo.FollowerCount > 1000 {
		adjustment *= 0.7
	}

	// Adjust based on previous violations
	if senderInfo.ViolationCount > 0 {
		adjustment *= (1.0 + float64(senderInfo.ViolationCount)*0.2)
	}

	return minFloat64(adjustment, 2.0) // Cap at 2x
}

// calculateRecentAccuracy calculates recent accuracy for a pattern
func (r *EnhancedPatternRepository) calculateRecentAccuracy(ctx context.Context, patternID string) float64 {
	// Get recent performance metrics (last 7 days)
	endDate := time.Now().Format("2006-01-02")
	startDate := time.Now().AddDate(0, 0, -7).Format("2006-01-02")

	metrics, err := r.GetPerformanceMetrics(ctx, patternID, startDate, endDate)
	if err != nil || len(metrics) == 0 {
		return 0.5 // Default neutral
	}

	var totalTruePositives int64
	var totalFalsePositives int64

	for _, metric := range metrics {
		totalTruePositives += metric.TruePositives
		totalFalsePositives += metric.FalsePositives
	}

	if totalTruePositives+totalFalsePositives == 0 {
		return 0.5
	}

	return float64(totalTruePositives) / float64(totalTruePositives+totalFalsePositives)
}

// calculateOptimalityScore calculates a composite score for pattern optimality
func (r *EnhancedPatternRepository) calculateOptimalityScore(pattern *models.EnhancedModerationPattern) float64 {
	// Base score from effectiveness
	score := pattern.Effectiveness * 0.4

	// Add confidence score weight
	score += pattern.ConfidenceScore * 0.2

	// Add performance weight (inverse of average match time)
	if pattern.AverageMatchTime > 0 {
		performanceScore := 1.0 / (1.0 + pattern.AverageMatchTime/100.0) // Normalize around 100ms
		score += performanceScore * 0.2
	}

	// Add recency weight
	if !pattern.LastUsed.IsZero() {
		daysSinceUsed := time.Since(pattern.LastUsed).Hours() / 24
		recencyScore := 1.0 / (1.0 + daysSinceUsed/30.0) // Normalize around 30 days
		score += recencyScore * 0.1
	}

	// Add priority weight
	priorityScore := float64(pattern.Priority) / 10.0 // Normalize to 0-1
	score += priorityScore * 0.1

	return minFloat64(score, 1.0)
}

// processFeedbackBatch processes a batch of feedback for a single pattern
func (r *EnhancedPatternRepository) processFeedbackBatch(ctx context.Context, patternID string, feedbacks []*PatternFeedback) error {
	for _, feedback := range feedbacks {
		err := r.UpdatePatternEffectiveness(ctx, patternID, feedback)
		if err != nil {
			return fmt.Errorf("%w: %w", storage.ErrPatternUpdateFailed, err)
		}
	}
	return nil
}

// analyzeFeedbackTrends analyzes feedback trends to suggest improvements
func (r *EnhancedPatternRepository) analyzeFeedbackTrends(_ context.Context, feedbackBatch []*PatternFeedback) error {
	// Analyze patterns with high false positive rates
	falsePositiveThreshold := 0.3
	patternIssues := make(map[string]int)

	for _, feedback := range feedbackBatch {
		if feedback.FeedbackType == "false_positive" {
			patternIssues[feedback.PatternID]++
		}
	}

	// Log patterns that may need adjustment
	for patternID, falsePositiveCount := range patternIssues {
		if float64(falsePositiveCount)/float64(len(feedbackBatch)) > falsePositiveThreshold {
			r.logger.Warn("pattern shows high false positive rate",
				zap.String("pattern_id", patternID),
				zap.Int("false_positives", falsePositiveCount),
				zap.Float64("rate", float64(falsePositiveCount)/float64(len(feedbackBatch))))
		}
	}

	return nil
}

// Helper methods for pattern matching (simplified implementations)
func (r *EnhancedPatternRepository) matchDomainPattern(content, pattern string) bool {
	// Simplified domain matching - in production this would be more sophisticated
	return content == pattern ||
		(len(content) > len(pattern) && content[len(content)-len(pattern)-1:] == "."+pattern)
}

func (r *EnhancedPatternRepository) matchRegexPattern(content, pattern string) bool {
	// In production, this would use compiled regex from cache
	// For now, simplified text matching
	return r.matchTextPattern(content, pattern)
}

func (r *EnhancedPatternRepository) matchTextPattern(content, pattern string) bool {
	// Simplified text matching
	return content == pattern ||
		len(content) >= len(pattern) &&
			content[:len(pattern)] == pattern
}

func (r *EnhancedPatternRepository) getSeverityWeight(severity string) float64 {
	switch severity {
	case StatusCritical:
		return 1.0
	case StatusHigh:
		return 0.8
	case StatusMedium:
		return 0.6
	case StatusLow:
		return 0.4
	default:
		return 0.5
	}
}

// Helper functions for enhanced patterns (using common math functions)
func minFloat64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// ===== SUPPORT TYPES FOR PATTERN ANALYSIS =====

// PatternAnalysis represents the result of content analysis
type PatternAnalysis struct {
	Content     string          `json:"content"`
	Timestamp   time.Time       `json:"timestamp"`
	Matches     []*PatternMatch `json:"matches"`
	RiskScore   float64         `json:"risk_score"`
	Categories  []string        `json:"categories"`
	Confidence  float64         `json:"confidence"`
	ProcessTime int64           `json:"process_time_ms"`
}

// PatternMatch represents a single pattern match result
type PatternMatch struct {
	PatternID   string  `json:"pattern_id"`
	PatternType string  `json:"pattern_type"`
	Category    string  `json:"category"`
	Severity    string  `json:"severity"`
	IsMatch     bool    `json:"is_match"`
	Confidence  float64 `json:"confidence"`
	MatchTime   float64 `json:"match_time_ms"`
	Position    int     `json:"position"`
}

// SpamDetectionResult represents the result of spam detection
type SpamDetectionResult struct {
	IsSpam         bool     `json:"is_spam"`
	SpamScore      float64  `json:"spam_score"`
	Confidence     float64  `json:"confidence"`
	DetectedBy     []string `json:"detected_by"`
	ReasonCodes    []string `json:"reason_codes"`
	ProcessingTime int64    `json:"processing_time_ms"`
}

// SenderInfo contains information about the content sender
type SenderInfo struct {
	AccountAge     int `json:"account_age_days"`
	FollowerCount  int `json:"follower_count"`
	ViolationCount int `json:"violation_count"`
}

// PatternFeedback represents user feedback about pattern accuracy
type PatternFeedback struct {
	PatternID    string    `json:"pattern_id"`
	FeedbackType string    `json:"feedback_type"` // "true_positive", "false_positive", "false_negative"
	ContentHash  string    `json:"content_hash"`
	UserID       string    `json:"user_id"`
	Timestamp    time.Time `json:"timestamp"`
	Notes        string    `json:"notes,omitempty"`
}

// ScoredPattern represents a pattern with its optimality score
type ScoredPattern struct {
	Pattern *models.EnhancedModerationPattern
	Score   float64
}
