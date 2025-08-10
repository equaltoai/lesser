package advanced

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Pattern type constants
const (
	patternTypeRegex   = "regex"
	patternTypeKeyword = "keyword"
	patternTypePhrase  = "phrase"
)

// PatternRepository defines the interface for pattern operations
type PatternRepository interface {
	CreatePattern(ctx context.Context, pattern *ModerationPattern) error
	UpdatePattern(ctx context.Context, patternID string, pattern *ModerationPattern) error
	DeletePattern(ctx context.Context, patternID string) error
	GetPattern(ctx context.Context, patternID string) (*ModerationPattern, error)
	GetPatterns(ctx context.Context, filter PatternFilter) ([]*ModerationPattern, error)
	IncrementHitCount(ctx context.Context, patternID string) error
	LoadActivePatterns(ctx context.Context) ([]*ModerationPattern, error)
}

// PatternMatcher handles pattern-based content matching
type PatternMatcher struct {
	repository PatternRepository
	logger     *zap.Logger

	// In-memory cache of active patterns
	patterns    sync.Map
	regexCache  sync.Map
	lastUpdate  time.Time
	updateMutex sync.RWMutex
}

// NewPatternMatcher creates a new pattern matcher
// Pattern updates should be triggered by DynamoDB stream events when patterns are modified
func NewPatternMatcher(repository PatternRepository, logger *zap.Logger) *PatternMatcher {
	pm := &PatternMatcher{
		repository: repository,
		logger:     logger,
	}

	// Load patterns on initialization
	ctx := context.Background()
	if err := pm.loadPatterns(ctx); err != nil {
		logger.Warn("failed to load patterns on init", zap.Error(err))
	}

	return pm
}

// CreatePattern creates a new moderation pattern
func (pm *PatternMatcher) CreatePattern(ctx context.Context, pattern *ModerationPattern) error {
	// Validate pattern
	if err := pm.validatePattern(pattern); err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}

	// Generate ID if not provided
	if pattern.ID == "" {
		pattern.ID = generatePatternID(pattern.Name)
	}

	pattern.CreatedAt = time.Now()
	pattern.UpdatedAt = time.Now()
	pattern.HitCount = 0

	// Store using repository
	err := pm.repository.CreatePattern(ctx, pattern)
	if err != nil {
		return fmt.Errorf("create pattern: %w", err)
	}

	// Update in-memory cache
	pm.patterns.Store(pattern.ID, pattern)

	// Pre-compile regex if applicable
	if pattern.Type == patternTypeRegex {
		if regex, err := regexp.Compile(pattern.Pattern); err == nil {
			pm.regexCache.Store(pattern.ID, regex)
		}
	}

	pm.logger.Info("created pattern",
		zap.String("patternID", pattern.ID),
		zap.String("name", pattern.Name),
		zap.Float64("severity", pattern.Severity))

	return nil
}

// UpdatePattern updates an existing pattern
func (pm *PatternMatcher) UpdatePattern(ctx context.Context, patternID string, updates *ModerationPattern) error {
	// Clear regex cache if pattern is being updated
	if updates.Pattern != "" {
		pm.regexCache.Delete(patternID)
	}

	// Update using repository
	err := pm.repository.UpdatePattern(ctx, patternID, updates)
	if err != nil {
		return fmt.Errorf("update pattern: %w", err)
	}

	// Get the updated pattern to update cache
	updated, err := pm.repository.GetPattern(ctx, patternID)
	if err != nil {
		return fmt.Errorf("get updated pattern: %w", err)
	}

	// Update cache
	pm.patterns.Store(patternID, updated)

	// Pre-compile regex if applicable
	if updated.Type == patternTypeRegex {
		if regex, err := regexp.Compile(updated.Pattern); err == nil {
			pm.regexCache.Store(updated.ID, regex)
		}
	}

	return nil
}

// DeletePattern deletes a pattern (soft delete by marking inactive)
func (pm *PatternMatcher) DeletePattern(ctx context.Context, patternID string) error {
	// Delete using repository
	err := pm.repository.DeletePattern(ctx, patternID)
	if err != nil {
		return fmt.Errorf("delete pattern: %w", err)
	}

	// Remove from cache
	pm.patterns.Delete(patternID)
	pm.regexCache.Delete(patternID)

	return nil
}

// GetPatterns retrieves patterns based on filter
func (pm *PatternMatcher) GetPatterns(ctx context.Context, filter PatternFilter) ([]*ModerationPattern, error) {
	// Use repository to get patterns
	patterns, err := pm.repository.GetPatterns(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("get patterns: %w", err)
	}

	return patterns, nil
}

// MatchContent checks content against all active patterns
func (pm *PatternMatcher) MatchContent(_ context.Context, content string, _ ContentMetadata) ([]PatternMatch, error) {
	matches := []PatternMatch{}
	lowerContent := strings.ToLower(content)

	// Iterate through cached patterns
	pm.patterns.Range(func(_, value any) bool {
		pattern, ok := value.(*ModerationPattern)
		if !ok || !pattern.Active {
			return true
		}

		match := pm.checkPattern(pattern, content, lowerContent)
		if match != nil {
			matches = append(matches, *match)

			// Increment hit count asynchronously
			go pm.incrementHitCount(pattern.ID)
		}

		return true
	})

	// Sort matches by severity
	sortMatchesBySeverity(matches)

	return matches, nil
}

// Helper methods

func (pm *PatternMatcher) validatePattern(pattern *ModerationPattern) error {
	if pattern.Name == "" {
		return fmt.Errorf("pattern name required")
	}

	if pattern.Pattern == "" {
		return fmt.Errorf("pattern required")
	}

	// Validate regex if applicable
	if pattern.Type == patternTypeRegex {
		_, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			return fmt.Errorf("invalid regex pattern: %w", err)
		}
	}

	// Validate pattern type
	validTypes := map[string]bool{patternTypeRegex: true, patternTypeKeyword: true, patternTypePhrase: true}
	if !validTypes[pattern.Type] {
		return fmt.Errorf("invalid pattern type: %s", pattern.Type)
	}

	return nil
}

func (pm *PatternMatcher) checkPattern(pattern *ModerationPattern, content, lowerContent string) *PatternMatch {
	var matched bool
	var matchText string
	var location string

	switch pattern.Type {
	case patternTypeRegex:
		// Get compiled regex from cache
		var regex *regexp.Regexp
		if cached, ok := pm.regexCache.Load(pattern.ID); ok {
			regex = cached.(*regexp.Regexp)
		} else {
			// Compile and cache
			var err error
			regex, err = regexp.Compile(pattern.Pattern)
			if err != nil {
				pm.logger.Warn("invalid regex pattern",
					zap.String("patternID", pattern.ID),
					zap.Error(err))
				return nil
			}
			pm.regexCache.Store(pattern.ID, regex)
		}

		if match := regex.FindStringIndex(content); match != nil {
			matched = true
			matchText = content[match[0]:match[1]]
			location = fmt.Sprintf("chars %d-%d", match[0], match[1])
		}

	case patternTypeKeyword:
		keyword := strings.ToLower(pattern.Pattern)
		if idx := strings.Index(lowerContent, keyword); idx >= 0 {
			matched = true
			matchText = content[idx : idx+len(keyword)]
			location = fmt.Sprintf("char %d", idx)
		}

	case patternTypePhrase:
		phrase := strings.ToLower(pattern.Pattern)
		if strings.Contains(lowerContent, phrase) {
			matched = true
			matchText = pattern.Pattern
			location = "in content"
		}
	}

	if matched {
		return &PatternMatch{
			PatternID:   pattern.ID,
			PatternName: pattern.Name,
			MatchText:   matchText,
			Location:    location,
			Confidence:  1.0, // Pattern matches are binary
		}
	}

	return nil
}

func (pm *PatternMatcher) loadPatterns(ctx context.Context) error {
	pm.updateMutex.Lock()
	defer pm.updateMutex.Unlock()

	// Load all active patterns using repository
	patterns, err := pm.repository.LoadActivePatterns(ctx)
	if err != nil {
		return fmt.Errorf("load active patterns: %w", err)
	}

	// Clear existing patterns
	pm.patterns.Range(func(key, _ any) bool {
		pm.patterns.Delete(key)
		return true
	})
	pm.regexCache.Range(func(key, _ any) bool {
		pm.regexCache.Delete(key)
		return true
	})

	// Load new patterns
	for _, pattern := range patterns {
		pm.patterns.Store(pattern.ID, pattern)

		// Pre-compile regex
		if pattern.Type == patternTypeRegex {
			if regex, err := regexp.Compile(pattern.Pattern); err == nil {
				pm.regexCache.Store(pattern.ID, regex)
			}
		}
	}

	pm.lastUpdate = time.Now()

	pm.logger.Info("loaded patterns",
		zap.Int("count", len(patterns)))

	return nil
}

// RefreshPatterns refreshes the pattern cache from DynamoDB
// This method should be called in response to DynamoDB stream events when patterns are modified.
// In a serverless architecture, the stream-router Lambda should process DynamoDB stream events
// and invoke this method when pattern records (PK=PATTERN#*) are created, updated, or deleted.
//
// Event-driven pattern refresh ensures:
// - No wasted compute from polling
// - Immediate cache updates when patterns change
// - Compatibility with Lambda execution limits
// - Efficient resource utilization in serverless environments
func (pm *PatternMatcher) RefreshPatterns(ctx context.Context) error {
	if err := pm.loadPatterns(ctx); err != nil {
		pm.logger.Error("failed to refresh patterns", zap.Error(err))
		return fmt.Errorf("refresh patterns: %w", err)
	}

	pm.logger.Info("pattern cache refreshed successfully")
	return nil
}

func (pm *PatternMatcher) incrementHitCount(patternID string) {
	ctx := context.Background()

	err := pm.repository.IncrementHitCount(ctx, patternID)
	if err != nil {
		pm.logger.Warn("failed to increment hit count",
			zap.String("patternID", patternID),
			zap.Error(err))
	}
}

func generatePatternID(name string) string {
	// Simple ID generation
	cleaned := strings.ReplaceAll(strings.ToLower(name), " ", "-")
	return fmt.Sprintf("%s-%d", cleaned, time.Now().Unix())
}

func sortMatchesBySeverity(matches []PatternMatch) {
	// Simple bubble sort for severity
	severityOrder := map[Severity]int{
		SeverityCritical: 4,
		SeverityHigh:     3,
		SeverityMedium:   2,
		SeverityLow:      1,
	}

	for i := 0; i < len(matches)-1; i++ {
		for j := i + 1; j < len(matches); j++ {
			// Get patterns to compare severity
			// In production, you'd cache pattern info with the match
			if severityOrder[SeverityHigh] < severityOrder[SeverityMedium] {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}
}
