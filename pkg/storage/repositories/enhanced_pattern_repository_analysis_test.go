package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ============================================================================
// Low-Level Matching Helpers Tests
// ============================================================================

func TestMatchDomainPattern(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewEnhancedPatternRepository(mockDB, "test-table", zap.NewNop(), nil)

	tests := []struct {
		name     string
		content  string
		pattern  string
		expected bool
	}{
		{
			name:     "exact_domain_match",
			content:  "example.com",
			pattern:  "example.com",
			expected: true,
		},
		{
			name:     "subdomain_match",
			content:  "foo.example.com",
			pattern:  "example.com",
			expected: true,
		},
		{
			name:     "subdomain_multiple_levels",
			content:  "bar.foo.example.com",
			pattern:  "example.com",
			expected: true,
		},
		{
			name:     "no_match_different_domain",
			content:  "example.org",
			pattern:  "example.com",
			expected: false,
		},
		{
			name:     "no_match_partial_suffix",
			content:  "notexample.com",
			pattern:  "example.com",
			expected: false,
		},
		{
			name:     "no_match_prefix",
			content:  "example.com.evil.org",
			pattern:  "example.com",
			expected: false,
		},
		{
			name:     "empty_pattern",
			content:  "example.com",
			pattern:  "",
			expected: false,
		},
		{
			name:     "empty_content",
			content:  "",
			pattern:  "example.com",
			expected: false,
		},
		{
			name:     "content_shorter_than_pattern",
			content:  "a.com",
			pattern:  "example.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.matchDomainPattern(tt.content, tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchTextPattern(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewEnhancedPatternRepository(mockDB, "test-table", zap.NewNop(), nil)

	tests := []struct {
		name     string
		content  string
		pattern  string
		expected bool
	}{
		{
			name:     "exact_match",
			content:  "test content",
			pattern:  "test content",
			expected: true,
		},
		{
			name:     "prefix_match",
			content:  "test content with more text",
			pattern:  "test content",
			expected: true,
		},
		{
			name:     "prefix_single_char",
			content:  "hello world",
			pattern:  "h",
			expected: true,
		},
		{
			name:     "no_match_not_prefix",
			content:  "hello world",
			pattern:  "world",
			expected: false,
		},
		{
			name:     "no_match_different_content",
			content:  "hello world",
			pattern:  "goodbye",
			expected: false,
		},
		{
			name:     "empty_pattern_exact_match",
			content:  "",
			pattern:  "",
			expected: true,
		},
		{
			name:     "empty_pattern_with_content",
			content:  "hello",
			pattern:  "",
			expected: true,
		},
		{
			name:     "empty_content_with_pattern",
			content:  "",
			pattern:  "hello",
			expected: false,
		},
		{
			name:     "content_shorter_than_pattern",
			content:  "test",
			pattern:  "testing",
			expected: false,
		},
		{
			name:     "case_sensitive_no_match",
			content:  "Hello World",
			pattern:  "hello world",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.matchTextPattern(tt.content, tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchRegexPattern(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewEnhancedPatternRepository(mockDB, "test-table", zap.NewNop(), nil)

	// matchRegexPattern currently delegates to matchTextPattern
	tests := []struct {
		name     string
		content  string
		pattern  string
		expected bool
	}{
		{
			name:     "exact_match_parity",
			content:  "test content",
			pattern:  "test content",
			expected: true,
		},
		{
			name:     "prefix_match_parity",
			content:  "test content with more",
			pattern:  "test content",
			expected: true,
		},
		{
			name:     "no_match_parity",
			content:  "hello world",
			pattern:  "test",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.matchRegexPattern(tt.content, tt.pattern)
			// Assert parity with matchTextPattern
			textResult := repo.matchTextPattern(tt.content, tt.pattern)
			assert.Equal(t, textResult, result, "matchRegexPattern should have parity with matchTextPattern")
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetSeverityWeight(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewEnhancedPatternRepository(mockDB, "test-table", zap.NewNop(), nil)

	tests := []struct {
		name     string
		severity string
		expected float64
	}{
		{
			name:     "critical_severity",
			severity: StatusCritical,
			expected: 1.0,
		},
		{
			name:     "high_severity",
			severity: StatusHigh,
			expected: 0.8,
		},
		{
			name:     "medium_severity",
			severity: StatusMedium,
			expected: 0.6,
		},
		{
			name:     "low_severity",
			severity: StatusLow,
			expected: 0.4,
		},
		{
			name:     "unknown_severity_default",
			severity: "unknown",
			expected: 0.5,
		},
		{
			name:     "empty_severity_default",
			severity: "",
			expected: 0.5,
		},
		{
			name:     "random_string_default",
			severity: "some_random_value",
			expected: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.getSeverityWeight(tt.severity)
			assert.InDelta(t, tt.expected, result, 0.001)
		})
	}
}

// ============================================================================
// analyzePatternMatch Tests
// ============================================================================

func TestAnalyzePatternMatch(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewEnhancedPatternRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	tests := []struct {
		name               string
		content            string
		pattern            *models.EnhancedModerationPattern
		expectedIsMatch    bool
		expectedConfidence float64
		checkConfidence    bool // Some tests need exact confidence, others just need match
	}{
		{
			name:    "url_exact_match",
			content: "https://exact.example.com/path",
			pattern: &models.EnhancedModerationPattern{
				PatternID:       "pattern-1",
				PatternType:     "url_exact",
				PatternContent:  "https://exact.example.com/path",
				Category:        "spam",
				ConfidenceScore: 0.8,
			},
			expectedIsMatch:    true,
			expectedConfidence: 1.0,
			checkConfidence:    true,
		},
		{
			name:    "url_exact_no_match",
			content: "https://other.example.com/path",
			pattern: &models.EnhancedModerationPattern{
				PatternID:       "pattern-2",
				PatternType:     "url_exact",
				PatternContent:  "https://exact.example.com/path",
				Category:        "spam",
				ConfidenceScore: 0.8,
			},
			expectedIsMatch: false,
		},
		{
			name:    "url_domain_match",
			content: "example.com",
			pattern: &models.EnhancedModerationPattern{
				PatternID:       "pattern-3",
				PatternType:     "url_domain",
				PatternContent:  "example.com",
				Category:        "phishing",
				ConfidenceScore: 0.85,
			},
			expectedIsMatch:    true,
			expectedConfidence: 0.9,
			checkConfidence:    true,
		},
		{
			name:    "url_subdomain_match",
			content: "sub.example.com",
			pattern: &models.EnhancedModerationPattern{
				PatternID:       "pattern-4",
				PatternType:     "url_subdomain",
				PatternContent:  "example.com",
				Category:        "malware",
				ConfidenceScore: 0.75,
			},
			expectedIsMatch:    true,
			expectedConfidence: 0.9,
			checkConfidence:    true,
		},
		{
			name:    "url_domain_no_match",
			content: "notexample.org",
			pattern: &models.EnhancedModerationPattern{
				PatternID:       "pattern-5",
				PatternType:     "url_domain",
				PatternContent:  "example.com",
				Category:        "spam",
				ConfidenceScore: 0.8,
			},
			expectedIsMatch: false,
		},
		{
			name:    "url_regex_match",
			content: "matching-prefix-content",
			pattern: &models.EnhancedModerationPattern{
				PatternID:       "pattern-6",
				PatternType:     "url_regex",
				PatternContent:  "matching-prefix",
				Category:        "spam",
				ConfidenceScore: 0.7,
			},
			expectedIsMatch:    true,
			expectedConfidence: 0.7, // Uses pattern.ConfidenceScore
			checkConfidence:    true,
		},
		{
			name:    "url_regex_no_match",
			content: "no-match-here",
			pattern: &models.EnhancedModerationPattern{
				PatternID:       "pattern-7",
				PatternType:     "url_regex",
				PatternContent:  "matching-prefix",
				Category:        "spam",
				ConfidenceScore: 0.7,
			},
			expectedIsMatch: false,
		},
		{
			name:    "default_generic_text_match",
			content: "some generic text content",
			pattern: &models.EnhancedModerationPattern{
				PatternID:       "pattern-8",
				PatternType:     "text",
				PatternContent:  "some generic text",
				Category:        "hate_speech",
				ConfidenceScore: 0.65,
			},
			expectedIsMatch:    true,
			expectedConfidence: 0.65, // Uses pattern.ConfidenceScore
			checkConfidence:    true,
		},
		{
			name:    "default_generic_text_no_match",
			content: "other text content",
			pattern: &models.EnhancedModerationPattern{
				PatternID:       "pattern-9",
				PatternType:     "text",
				PatternContent:  "some generic text",
				Category:        "hate_speech",
				ConfidenceScore: 0.65,
			},
			expectedIsMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := repo.analyzePatternMatch(ctx, tt.content, tt.pattern)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedIsMatch, match.IsMatch)
			assert.Equal(t, tt.pattern.PatternID, match.PatternID)
			assert.Equal(t, tt.pattern.PatternType, match.PatternType)
			assert.Equal(t, tt.pattern.Category, match.Category)
			assert.Equal(t, -1, match.Position, "Position should always be -1")
			assert.GreaterOrEqual(t, match.MatchTime, float64(0), "MatchTime should be >= 0")

			if tt.checkConfidence && tt.expectedIsMatch {
				assert.InDelta(t, tt.expectedConfidence, match.Confidence, 0.001)
			}
			if !tt.expectedIsMatch {
				assert.InDelta(t, 0.0, match.Confidence, 0.001)
			}
		})
	}
}

// ============================================================================
// calculateAnalysisMetrics Tests
// ============================================================================

func TestCalculateAnalysisMetrics(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewEnhancedPatternRepository(mockDB, "test-table", zap.NewNop(), nil)

	tests := []struct {
		name                string
		matches             []*PatternMatch
		expectedRiskScore   float64
		expectedConfidence  float64
		expectedCategories  []string
		checkRiskCap        bool
		expectedRiskAtLeast float64
	}{
		{
			name:               "empty_matches",
			matches:            []*PatternMatch{},
			expectedRiskScore:  0.0,
			expectedConfidence: 0.0,
			expectedCategories: []string{},
		},
		{
			name: "single_critical_match",
			matches: []*PatternMatch{
				{
					PatternID:  "p1",
					Category:   "spam",
					Severity:   StatusCritical,
					Confidence: 1.0,
				},
			},
			expectedRiskScore:  1.0, // 1.0 * 1.0 / 1 = 1.0
			expectedConfidence: 1.0,
			expectedCategories: []string{"spam"},
		},
		{
			name: "single_low_match",
			matches: []*PatternMatch{
				{
					PatternID:  "p2",
					Category:   "phishing",
					Severity:   StatusLow,
					Confidence: 0.5,
				},
			},
			expectedRiskScore:  0.2, // 0.5 * 0.4 / 1 = 0.2
			expectedConfidence: 0.5,
			expectedCategories: []string{"phishing"},
		},
		{
			name: "multiple_matches_different_categories",
			matches: []*PatternMatch{
				{
					PatternID:  "p1",
					Category:   "spam",
					Severity:   StatusHigh,
					Confidence: 0.8,
				},
				{
					PatternID:  "p2",
					Category:   "malware",
					Severity:   StatusMedium,
					Confidence: 0.6,
				},
			},
			// Risk: (0.8 * 0.8 + 0.6 * 0.6) / 2 = (0.64 + 0.36) / 2 = 0.5
			expectedRiskScore:  0.5,
			expectedConfidence: 0.7, // (0.8 + 0.6) / 2 = 0.7
			expectedCategories: []string{"spam", "malware"},
		},
		{
			name: "same_category_multiple_matches",
			matches: []*PatternMatch{
				{
					PatternID:  "p1",
					Category:   "spam",
					Severity:   StatusHigh,
					Confidence: 0.9,
				},
				{
					PatternID:  "p2",
					Category:   "spam",
					Severity:   StatusCritical,
					Confidence: 1.0,
				},
			},
			// Risk: (0.9 * 0.8 + 1.0 * 1.0) / 2 = (0.72 + 1.0) / 2 = 0.86
			expectedRiskScore:  0.86,
			expectedConfidence: 0.95, // (0.9 + 1.0) / 2 = 0.95
			expectedCategories: []string{"spam"},
		},
		{
			name: "risk_score_capped_at_1",
			matches: []*PatternMatch{
				{
					PatternID:  "p1",
					Category:   "spam",
					Severity:   StatusCritical,
					Confidence: 1.0,
				},
				{
					PatternID:  "p2",
					Category:   "spam",
					Severity:   StatusCritical,
					Confidence: 1.0,
				},
				{
					PatternID:  "p3",
					Category:   "spam",
					Severity:   StatusCritical,
					Confidence: 1.0,
				},
			},
			// Raw: (1.0 * 1.0 * 3) / 3 = 1.0, capped at 1.0
			expectedRiskScore:  1.0,
			expectedConfidence: 1.0,
			expectedCategories: []string{"spam"},
			checkRiskCap:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := &PatternAnalysis{
				Matches: tt.matches,
			}

			repo.calculateAnalysisMetrics(analysis)

			assert.InDelta(t, tt.expectedRiskScore, analysis.RiskScore, 0.01, "RiskScore mismatch")
			assert.InDelta(t, tt.expectedConfidence, analysis.Confidence, 0.01, "Confidence mismatch")

			if tt.checkRiskCap {
				assert.LessOrEqual(t, analysis.RiskScore, 1.0, "RiskScore should be capped at 1.0")
			}

			// Check categories as a set (order doesn't matter)
			if len(tt.expectedCategories) == 0 {
				assert.Empty(t, analysis.Categories)
			} else {
				assert.ElementsMatch(t, tt.expectedCategories, analysis.Categories)
			}
		})
	}
}

// ============================================================================
// calculateReputationAdjustment Tests
// ============================================================================

func TestCalculateReputationAdjustment(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewEnhancedPatternRepository(mockDB, "test-table", zap.NewNop(), nil)

	tests := []struct {
		name        string
		senderInfo  *SenderInfo
		expectedMin float64
		expectedMax float64
		description string
	}{
		{
			name: "brand_new_account_low_followers",
			senderInfo: &SenderInfo{
				AccountAge:     3,
				FollowerCount:  5,
				ViolationCount: 0,
			},
			expectedMin: 1.5, // 1.0 * 1.3 * 1.2 = 1.56
			expectedMax: 1.6,
			description: "New account with low followers should have high adjustment",
		},
		{
			name: "brand_new_account_high_followers",
			senderInfo: &SenderInfo{
				AccountAge:     3,
				FollowerCount:  2000,
				ViolationCount: 0,
			},
			expectedMin: 0.9, // 1.0 * 1.3 * 0.7 = 0.91
			expectedMax: 0.95,
			description: "New account with many followers should have reduced adjustment",
		},
		{
			name: "old_account_low_followers",
			senderInfo: &SenderInfo{
				AccountAge:     400,
				FollowerCount:  5,
				ViolationCount: 0,
			},
			expectedMin: 0.9, // 1.0 * 0.8 * 1.2 = 0.96
			expectedMax: 1.0,
			description: "Old account with few followers should have modest adjustment",
		},
		{
			name: "old_account_high_followers",
			senderInfo: &SenderInfo{
				AccountAge:     400,
				FollowerCount:  5000,
				ViolationCount: 0,
			},
			expectedMin: 0.5, // 1.0 * 0.8 * 0.7 = 0.56
			expectedMax: 0.6,
			description: "Old account with many followers should have low adjustment",
		},
		{
			name: "account_with_violations",
			senderInfo: &SenderInfo{
				AccountAge:     100,
				FollowerCount:  100,
				ViolationCount: 3,
			},
			expectedMin: 1.5, // 1.0 * 1.0 * 1.0 * (1 + 0.6) = 1.6
			expectedMax: 1.7,
			description: "Account with violations should have increased adjustment",
		},
		{
			name: "max_cap_at_2",
			senderInfo: &SenderInfo{
				AccountAge:     1,
				FollowerCount:  1,
				ViolationCount: 10,
			},
			// Raw: 1.0 * 1.3 * 1.2 * (1 + 2.0) = 4.68, but capped at 2.0
			expectedMin: 2.0,
			expectedMax: 2.0,
			description: "Adjustment should be capped at 2.0",
		},
		{
			name: "neutral_account",
			senderInfo: &SenderInfo{
				AccountAge:     30,
				FollowerCount:  100,
				ViolationCount: 0,
			},
			expectedMin: 0.95,
			expectedMax: 1.05,
			description: "Normal account should have neutral adjustment",
		},
		{
			name: "edge_account_age_7",
			senderInfo: &SenderInfo{
				AccountAge:     7,
				FollowerCount:  100,
				ViolationCount: 0,
			},
			expectedMin: 0.95,
			expectedMax: 1.05,
			description: "Account exactly 7 days old should not trigger new account penalty",
		},
		{
			name: "edge_account_age_365",
			senderInfo: &SenderInfo{
				AccountAge:     365,
				FollowerCount:  100,
				ViolationCount: 0,
			},
			expectedMin: 0.95,
			expectedMax: 1.05,
			description: "Account exactly 365 days old should not trigger old account bonus",
		},
		{
			name: "edge_follower_count_10",
			senderInfo: &SenderInfo{
				AccountAge:     100,
				FollowerCount:  10,
				ViolationCount: 0,
			},
			expectedMin: 0.95,
			expectedMax: 1.05,
			description: "Account with exactly 10 followers should not trigger low follower penalty",
		},
		{
			name: "edge_follower_count_1000",
			senderInfo: &SenderInfo{
				AccountAge:     100,
				FollowerCount:  1000,
				ViolationCount: 0,
			},
			expectedMin: 0.95,
			expectedMax: 1.05,
			description: "Account with exactly 1000 followers should not trigger high follower bonus",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.calculateReputationAdjustment(tt.senderInfo)
			assert.GreaterOrEqual(t, result, tt.expectedMin, tt.description)
			assert.LessOrEqual(t, result, tt.expectedMax, tt.description)
			assert.LessOrEqual(t, result, 2.0, "Adjustment should never exceed cap of 2.0")
		})
	}
}

// ============================================================================
// calculateOptimalityScore Tests
// ============================================================================

func TestCalculateOptimalityScore(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewEnhancedPatternRepository(mockDB, "test-table", zap.NewNop(), nil)

	tests := []struct {
		name        string
		pattern     *models.EnhancedModerationPattern
		expectedMin float64
		expectedMax float64
		description string
	}{
		{
			name: "high_effectiveness_high_confidence",
			pattern: &models.EnhancedModerationPattern{
				Effectiveness:    0.9,
				ConfidenceScore:  0.9,
				AverageMatchTime: 0,
				Priority:         5,
			},
			// Score: 0.9 * 0.4 + 0.9 * 0.2 + 0 (no match time) + 0 (no lastUsed) + 5/10 * 0.1 = 0.36 + 0.18 + 0.05 = 0.59
			expectedMin: 0.55,
			expectedMax: 0.65,
			description: "High effectiveness and confidence should yield high score",
		},
		{
			name: "with_average_match_time",
			pattern: &models.EnhancedModerationPattern{
				Effectiveness:    0.5,
				ConfidenceScore:  0.5,
				AverageMatchTime: 50, // 50ms
				Priority:         5,
			},
			// Performance score: 1 / (1 + 50/100) = 1 / 1.5 = 0.667
			// Total: 0.5 * 0.4 + 0.5 * 0.2 + 0.667 * 0.2 + 0 (no lastUsed) + 5/10 * 0.1
			// = 0.2 + 0.1 + 0.133 + 0.05 = 0.483
			expectedMin: 0.45,
			expectedMax: 0.55,
			description: "Average match time should contribute to score",
		},
		{
			name: "with_last_used_recent",
			pattern: &models.EnhancedModerationPattern{
				Effectiveness:    0.5,
				ConfidenceScore:  0.5,
				AverageMatchTime: 0,
				LastUsed:         time.Now().Add(-24 * time.Hour), // 1 day ago
				Priority:         5,
			},
			// Recency: 1 / (1 + 1/30) ~= 0.968
			// Total: 0.5 * 0.4 + 0.5 * 0.2 + 0 + 0.968 * 0.1 + 5/10 * 0.1
			// = 0.2 + 0.1 + 0.097 + 0.05 = 0.447
			expectedMin: 0.40,
			expectedMax: 0.50,
			description: "Recent last used should contribute to score",
		},
		{
			name: "with_last_used_stale",
			pattern: &models.EnhancedModerationPattern{
				Effectiveness:    0.5,
				ConfidenceScore:  0.5,
				AverageMatchTime: 0,
				LastUsed:         time.Now().Add(-60 * 24 * time.Hour), // 60 days ago
				Priority:         5,
			},
			// Recency: 1 / (1 + 60/30) = 1 / 3 = 0.333
			// Total: 0.5 * 0.4 + 0.5 * 0.2 + 0 + 0.333 * 0.1 + 5/10 * 0.1
			// = 0.2 + 0.1 + 0.033 + 0.05 = 0.383
			expectedMin: 0.35,
			expectedMax: 0.45,
			description: "Stale last used should contribute less",
		},
		{
			name: "high_priority",
			pattern: &models.EnhancedModerationPattern{
				Effectiveness:    0.5,
				ConfidenceScore:  0.5,
				AverageMatchTime: 0,
				Priority:         10,
			},
			// Total: 0.5 * 0.4 + 0.5 * 0.2 + 0 + 0 + 10/10 * 0.1
			// = 0.2 + 0.1 + 0.1 = 0.4
			expectedMin: 0.35,
			expectedMax: 0.45,
			description: "High priority should contribute to score",
		},
		{
			name: "low_priority",
			pattern: &models.EnhancedModerationPattern{
				Effectiveness:    0.5,
				ConfidenceScore:  0.5,
				AverageMatchTime: 0,
				Priority:         1,
			},
			// Total: 0.5 * 0.4 + 0.5 * 0.2 + 0 + 0 + 1/10 * 0.1
			// = 0.2 + 0.1 + 0.01 = 0.31
			expectedMin: 0.28,
			expectedMax: 0.35,
			description: "Low priority should contribute less",
		},
		{
			name: "score_capped_at_1",
			pattern: &models.EnhancedModerationPattern{
				Effectiveness:    1.0,
				ConfidenceScore:  1.0,
				AverageMatchTime: 1, // Very fast
				LastUsed:         time.Now(),
				Priority:         10,
			},
			expectedMin: 0.9,
			expectedMax: 1.0,
			description: "Score should be capped at 1.0",
		},
		{
			name: "zero_values",
			pattern: &models.EnhancedModerationPattern{
				Effectiveness:    0,
				ConfidenceScore:  0,
				AverageMatchTime: 0,
				Priority:         0,
			},
			expectedMin: 0.0,
			expectedMax: 0.01,
			description: "Zero values should result in near-zero score",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.calculateOptimalityScore(tt.pattern)
			assert.GreaterOrEqual(t, result, tt.expectedMin, tt.description)
			assert.LessOrEqual(t, result, tt.expectedMax, tt.description)
			assert.LessOrEqual(t, result, 1.0, "Score should be capped at 1.0")
		})
	}
}

// ============================================================================
// AnalyzeContentPatterns Tests (Safe Coverage - Non-Matching Patterns)
// ============================================================================

func TestAnalyzeContentPatterns_NoMatches(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewEnhancedPatternRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	t.Run("active_patterns_no_match", func(t *testing.T) {
		// Patterns that are active but will NOT match the content
		patterns := []*models.EnhancedModerationPattern{
			{
				PatternID:       "pattern-1",
				PatternType:     "url_exact",
				PatternContent:  "https://spam.com/page",
				Category:        "spam",
				Active:          true,
				ConfidenceScore: 0.8,
			},
			{
				PatternID:       "pattern-2",
				PatternType:     "url_domain",
				PatternContent:  "phishing.example.org",
				Category:        "phishing",
				Active:          true,
				ConfidenceScore: 0.9,
			},
		}

		content := "This is safe content without any matching patterns"

		analysis, err := repo.AnalyzeContentPatterns(ctx, content, patterns)

		require.NoError(t, err)
		assert.NotNil(t, analysis)
		assert.Empty(t, analysis.Matches, "No patterns should match safe content")
		assert.GreaterOrEqual(t, analysis.ProcessTime, int64(0), "ProcessTime should be set")
		assert.InDelta(t, 0.0, analysis.RiskScore, 0.001, "RiskScore should be 0 with no matches")
		assert.InDelta(t, 0.0, analysis.Confidence, 0.001, "Confidence should be 0 with no matches")
		assert.Equal(t, content, analysis.Content)
		assert.Empty(t, analysis.Categories)
	})

	t.Run("inactive_patterns_ignored", func(t *testing.T) {
		// Pattern that WOULD match but is inactive
		patterns := []*models.EnhancedModerationPattern{
			{
				PatternID:       "pattern-1",
				PatternType:     "text",
				PatternContent:  "matching text",
				Category:        "spam",
				Active:          false, // Inactive!
				ConfidenceScore: 0.8,
			},
		}

		content := "matching text should be ignored because pattern is inactive"

		analysis, err := repo.AnalyzeContentPatterns(ctx, content, patterns)

		require.NoError(t, err)
		assert.NotNil(t, analysis)
		assert.Empty(t, analysis.Matches, "Inactive patterns should be skipped")
		assert.InDelta(t, 0.0, analysis.RiskScore, 0.001)
		assert.InDelta(t, 0.0, analysis.Confidence, 0.001)
	})

	t.Run("empty_patterns_list", func(t *testing.T) {
		patterns := []*models.EnhancedModerationPattern{}
		content := "any content here"

		analysis, err := repo.AnalyzeContentPatterns(ctx, content, patterns)

		require.NoError(t, err)
		assert.NotNil(t, analysis)
		assert.Empty(t, analysis.Matches)
		assert.GreaterOrEqual(t, analysis.ProcessTime, int64(0))
		assert.InDelta(t, 0.0, analysis.RiskScore, 0.001)
		assert.InDelta(t, 0.0, analysis.Confidence, 0.001)
	})

	t.Run("mixed_active_inactive_no_matches", func(t *testing.T) {
		patterns := []*models.EnhancedModerationPattern{
			{
				PatternID:       "pattern-1",
				PatternType:     "url_exact",
				PatternContent:  "https://no-match.com",
				Category:        "spam",
				Active:          true,
				ConfidenceScore: 0.8,
			},
			{
				PatternID:       "pattern-2",
				PatternType:     "text",
				PatternContent:  "inactive pattern",
				Category:        "phishing",
				Active:          false,
				ConfidenceScore: 0.9,
			},
			{
				PatternID:       "pattern-3",
				PatternType:     "url_domain",
				PatternContent:  "notfound.org",
				Category:        "malware",
				Active:          true,
				ConfidenceScore: 0.7,
			},
		}

		content := "content that doesn't match any active patterns"

		analysis, err := repo.AnalyzeContentPatterns(ctx, content, patterns)

		require.NoError(t, err)
		assert.Empty(t, analysis.Matches)
		assert.GreaterOrEqual(t, analysis.ProcessTime, int64(0))
	})
}

// ============================================================================
// Helper Function Tests: minFloat64 and maxFloat64
// ============================================================================

func TestMinFloat64(t *testing.T) {
	tests := []struct {
		name     string
		a        float64
		b        float64
		expected float64
	}{
		{name: "a_smaller", a: 1.0, b: 2.0, expected: 1.0},
		{name: "b_smaller", a: 3.0, b: 2.0, expected: 2.0},
		{name: "equal", a: 1.5, b: 1.5, expected: 1.5},
		{name: "negative", a: -1.0, b: 1.0, expected: -1.0},
		{name: "both_negative", a: -2.0, b: -1.0, expected: -2.0},
		{name: "zero", a: 0.0, b: 1.0, expected: 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := minFloat64(tt.a, tt.b)
			assert.InDelta(t, tt.expected, result, 0.0001)
		})
	}
}

func TestMaxFloat64(t *testing.T) {
	tests := []struct {
		name     string
		a        float64
		b        float64
		expected float64
	}{
		{name: "a_larger", a: 2.0, b: 1.0, expected: 2.0},
		{name: "b_larger", a: 2.0, b: 3.0, expected: 3.0},
		{name: "equal", a: 1.5, b: 1.5, expected: 1.5},
		{name: "negative", a: -1.0, b: 1.0, expected: 1.0},
		{name: "both_negative", a: -2.0, b: -1.0, expected: -1.0},
		{name: "zero", a: 0.0, b: -1.0, expected: 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maxFloat64(tt.a, tt.b)
			assert.InDelta(t, tt.expected, result, 0.0001)
		})
	}
}
