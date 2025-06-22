package moderation

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
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

// PatternManager manages moderation patterns and their effectiveness
type PatternManager struct {
	storage PatternStorage
}

// NewPatternManager creates a new pattern manager
func NewPatternManager() *PatternManager {
	return &PatternManager{}
}

// CreatePattern creates a new moderation pattern
func (pm *PatternManager) CreatePattern(ctx context.Context, pattern *ModerationPattern) error {
	// Validate pattern
	if err := pm.validatePattern(pattern); err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
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
			return fmt.Errorf("invalid regex pattern: %w", err)
		}
	}

	// Store pattern
	return pm.storage.CreateModerationPattern(ctx, pattern)
}

// GetPatterns retrieves patterns based on criteria
func (pm *PatternManager) GetPatterns(ctx context.Context, active bool, severity string, limit int) ([]*ModerationPattern, error) {
	patterns, err := pm.storage.GetModerationPatterns(ctx, active, severity, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get patterns: %w", err)
	}

	// Filter and enrich patterns
	var filteredPatterns []*ModerationPattern
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
		return nil, fmt.Errorf("failed to get patterns: %w", err)
	}

	var matches []*PatternMatch
	
	for _, pattern := range patterns {
		match := pm.matchPattern(pattern, content)
		if match != nil {
			matches = append(matches, match)
			
			// Record the match
			if err := pm.recordMatch(ctx, pattern.ID, true); err != nil {
				// Log error but don't fail the matching
				fmt.Printf("Failed to record pattern match: %v\n", err)
			}
		}
	}

	return matches, nil
}

// UpdatePatternStats updates statistics for a pattern
func (pm *PatternManager) UpdatePatternStats(ctx context.Context, patternID string, wasMatch bool, wasFalsePositive bool) error {
	pattern, err := pm.storage.GetModerationPattern(ctx, patternID)
	if err != nil {
		return fmt.Errorf("failed to get pattern: %w", err)
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
		return nil, fmt.Errorf("failed to get patterns: %w", err)
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
			PatternID:        pattern.ID,
			PatternType:      pattern.Type,
			Severity:         pattern.Severity,
			MatchCount:       pattern.MatchCount,
			FalsePositiveCount: pattern.FalsePositiveCount,
			Effectiveness:    pattern.Effectiveness,
			TruePositiveRate: pm.calculateTruePositiveRate(pattern),
			LastMatch:        pattern.LastMatch,
			CreatedAt:        pattern.CreatedAt,
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
	if len(patterns) > 0 {
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
		return nil, fmt.Errorf("failed to get patterns: %w", err)
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
	if pattern.Name == "" {
		return fmt.Errorf("pattern name is required")
	}
	
	if pattern.Content == "" {
		return fmt.Errorf("pattern content is required")
	}
	
	if pattern.Type == "" {
		return fmt.Errorf("pattern type is required")
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
		return fmt.Errorf("invalid pattern type: %s", pattern.Type)
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
		return fmt.Errorf("invalid severity: %s", pattern.Severity)
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
		PatternID:    pattern.ID,
		PatternName:  pattern.Name,
		PatternType:  pattern.Type,
		Severity:     pattern.Severity,
		Confidence:   confidence,
		MatchedText:  matchedText,
		Action:       pattern.Action,
		MatchedAt:    time.Now(),
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
	// Simple domain matching - could be enhanced with better URL parsing
	if strings.Contains(text, domain) {
		return true, domain
	}
	return false, ""
}

func (pm *PatternManager) matchIP(ipPattern, text string) (bool, string) {
	// Simple IP matching - could be enhanced with CIDR support
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
	
	if len(ineffectivePatterns) > len(patterns)/4 {
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
		PatternID:   pattern.ID,
		PatternName: pattern.Name,
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
	
	if len(optimization.Suggestions) == 0 {
		return nil
	}
	
	return optimization
}

// Types for pattern management

type ModerationPattern struct {
	ID                 string    `json:"id" dynamodbav:"id"`
	Name               string    `json:"name" dynamodbav:"name"`
	Description        string    `json:"description" dynamodbav:"description"`
	Type               string    `json:"type" dynamodbav:"type"` // keyword/regex/phrase/domain/ip/hash
	Content            string    `json:"content" dynamodbav:"content"`
	Severity           string    `json:"severity" dynamodbav:"severity"` // low/medium/high/critical
	Action             string    `json:"action" dynamodbav:"action"` // flag/hide/block/escalate
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

type ContentToModerate struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	ImageHash string `json:"image_hash,omitempty"`
	TextHash  string `json:"text_hash,omitempty"`
	Author    string `json:"author"`
	Type      string `json:"type"` // post/comment/message/profile
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

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

type PatternEffectivenessReport struct {
	GeneratedAt              time.Time          `json:"generated_at"`
	TotalPatterns            int                `json:"total_patterns"`
	AverageEffectiveness     float64            `json:"average_effectiveness"`
	OverallFalsePositiveRate float64            `json:"overall_false_positive_rate"`
	InefficientPatterns      int                `json:"inefficient_patterns"`
	PatternAnalysis          []*PatternAnalysis `json:"pattern_analysis"`
	Recommendations          []string           `json:"recommendations"`
}

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

type PatternOptimization struct {
	PatternID            string                   `json:"pattern_id"`
	PatternName          string                   `json:"pattern_name"`
	CurrentEffectiveness float64                  `json:"current_effectiveness"`
	Suggestions          []OptimizationSuggestion `json:"suggestions"`
}

type OptimizationSuggestion struct {
	Type        string `json:"type"`        // specificity/relevance/precision/performance
	Description string `json:"description"`
	Impact      string `json:"impact"`      // low/medium/high
}