package moderation

import (
	"context"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPatternValidator_NewPatternValidator(t *testing.T) {
	logger := zap.NewNop()
	validator := NewPatternValidator(logger)
	assert.NotNil(t, validator)
	assert.NotNil(t, validator.urlMatcher)
	assert.NotNil(t, validator.ipMatcher)
}

func TestPatternValidator_ValidatePattern_URLPatterns(t *testing.T) {
	logger := zap.NewNop()
	validator := NewPatternValidator(logger)
	ctx := context.Background()
	config := DefaultSecurityTestConfig()

	tests := []struct {
		name        string
		pattern     *models.EnhancedModerationPattern
		expectValid bool
	}{
		{
			name: "valid exact URL pattern",
			pattern: &models.EnhancedModerationPattern{
				PatternID:      "url-1",
				PatternContent: "https://example.com",
				PatternType:    URLPatternExactStr,
			},
			expectValid: true,
		},
		{
			name: "valid domain pattern",
			pattern: &models.EnhancedModerationPattern{
				PatternID:      "url-2",
				PatternContent: "example.com",
				PatternType:    URLPatternDomainStr,
			},
			expectValid: true,
		},
		{
			name: "valid subdomain pattern",
			pattern: &models.EnhancedModerationPattern{
				PatternID:      "url-3",
				PatternContent: "*.example.com",
				PatternType:    URLPatternSubdomainStr,
			},
			expectValid: true,
		},
		{
			name: "valid path pattern",
			pattern: &models.EnhancedModerationPattern{
				PatternID:      "url-4",
				PatternContent: "/admin/*",
				PatternType:    URLPatternPathStr,
			},
			expectValid: true,
		},
		{
			name: "empty pattern content fails",
			pattern: &models.EnhancedModerationPattern{
				PatternID:      "url-5",
				PatternContent: "",
				PatternType:    URLPatternDomainStr,
			},
			expectValid: false,
		},
		{
			name: "missing pattern type fails",
			pattern: &models.EnhancedModerationPattern{
				PatternID:      "url-6",
				PatternContent: "example.com",
				PatternType:    "",
			},
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.ValidatePattern(ctx, tt.pattern, config)
			require.NoError(t, err)
			assert.Equal(t, tt.expectValid, result.Valid)
			if !tt.expectValid {
				assert.NotEmpty(t, result.Errors)
			}
		})
	}
}

func TestPatternValidator_ValidatePattern_IPPatterns(t *testing.T) {
	logger := zap.NewNop()
	validator := NewPatternValidator(logger)
	ctx := context.Background()
	config := DefaultSecurityTestConfig()

	tests := []struct {
		name        string
		pattern     *models.EnhancedModerationPattern
		expectValid bool
	}{
		{
			name: "valid single IPv4",
			pattern: &models.EnhancedModerationPattern{
				PatternID:      "ip-1",
				PatternContent: "192.168.1.1",
				PatternType:    IPPatternSingleStr,
			},
			expectValid: true,
		},
		{
			name: "valid CIDR block",
			pattern: &models.EnhancedModerationPattern{
				PatternID:      "ip-2",
				PatternContent: "10.0.0.0/8",
				PatternType:    IPPatternCIDRStr,
			},
			expectValid: true,
		},
		{
			name: "valid IP range",
			pattern: &models.EnhancedModerationPattern{
				PatternID:      "ip-3",
				PatternContent: "192.168.1.1-192.168.1.255",
				PatternType:    IPPatternRangeStr,
			},
			expectValid: true,
		},
		{
			name: "invalid IP fails",
			pattern: &models.EnhancedModerationPattern{
				PatternID:      "ip-4",
				PatternContent: "999.999.999.999",
				PatternType:    IPPatternSingleStr,
			},
			expectValid: false,
		},
		{
			name: "invalid CIDR fails",
			pattern: &models.EnhancedModerationPattern{
				PatternID:      "ip-5",
				PatternContent: "10.0.0.0/99",
				PatternType:    IPPatternCIDRStr,
			},
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.ValidatePattern(ctx, tt.pattern, config)
			require.NoError(t, err)
			assert.Equal(t, tt.expectValid, result.Valid)
		})
	}
}

func TestPatternValidator_SecurityTesting(t *testing.T) {
	logger := zap.NewNop()
	validator := NewPatternValidator(logger)
	ctx := context.Background()
	config := DefaultSecurityTestConfig()

	tests := []struct {
		name                string
		pattern             *models.EnhancedModerationPattern
		minSecurityScore    float64
		maxSecurityScore    float64
		expectSecurityTests bool
	}{
		{
			name: "simple pattern has high security score",
			pattern: &models.EnhancedModerationPattern{
				PatternID:      "sec-1",
				PatternContent: "example.com",
				PatternType:    URLPatternDomainStr,
			},
			minSecurityScore:    0.8,
			maxSecurityScore:    1.0,
			expectSecurityTests: true,
		},
		{
			name: "safe regex pattern",
			pattern: &models.EnhancedModerationPattern{
				PatternID:      "sec-2",
				PatternContent: `^https://example\.com/path$`,
				PatternType:    URLPatternRegexStr,
			},
			minSecurityScore:    0.5,
			maxSecurityScore:    1.0,
			expectSecurityTests: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.ValidatePattern(ctx, tt.pattern, config)
			require.NoError(t, err)

			if tt.expectSecurityTests {
				assert.GreaterOrEqual(t, result.SecurityScore, tt.minSecurityScore)
				assert.LessOrEqual(t, result.SecurityScore, tt.maxSecurityScore)
			}
		})
	}
}

func TestPatternValidator_PerformanceTesting(t *testing.T) {
	logger := zap.NewNop()
	validator := NewPatternValidator(logger)
	ctx := context.Background()
	config := DefaultSecurityTestConfig()

	tests := []struct {
		name              string
		pattern           *models.EnhancedModerationPattern
		expectCompileTime bool
		maxCompileTimeMs  float64
	}{
		{
			name: "simple pattern compiles fast",
			pattern: &models.EnhancedModerationPattern{
				PatternID:      "perf-1",
				PatternContent: "example.com",
				PatternType:    URLPatternDomainStr,
			},
			expectCompileTime: true,
			maxCompileTimeMs:  100.0,
		},
		{
			name: "IP pattern compiles fast",
			pattern: &models.EnhancedModerationPattern{
				PatternID:      "perf-2",
				PatternContent: "192.168.1.0/24",
				PatternType:    IPPatternCIDRStr,
			},
			expectCompileTime: true,
			maxCompileTimeMs:  100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.ValidatePattern(ctx, tt.pattern, config)
			require.NoError(t, err)

			if tt.expectCompileTime {
				assert.LessOrEqual(t, result.CompilationTime, tt.maxCompileTimeMs)
				assert.GreaterOrEqual(t, result.CompilationTime, 0.0)
			}
		})
	}
}

func TestPatternValidator_CalculateOverallScore(t *testing.T) {
	logger := zap.NewNop()
	validator := NewPatternValidator(logger)

	tests := []struct {
		name             string
		result           *ValidationResult
		expectedMinScore float64
		expectedMaxScore float64
	}{
		{
			name: "invalid result returns 0",
			result: &ValidationResult{
				Valid:            false,
				SecurityScore:    1.0,
				PerformanceScore: 1.0,
				AccuracyScore:    1.0,
			},
			expectedMinScore: 0.0,
			expectedMaxScore: 0.0,
		},
		{
			name: "all high scores returns high overall",
			result: &ValidationResult{
				Valid:            true,
				SecurityScore:    0.9,
				PerformanceScore: 0.8,
				AccuracyScore:    0.9,
			},
			expectedMinScore: 0.8,
			expectedMaxScore: 1.0,
		},
		{
			name: "mixed scores returns weighted average",
			result: &ValidationResult{
				Valid:            true,
				SecurityScore:    0.5,
				PerformanceScore: 0.5,
				AccuracyScore:    0.5,
			},
			expectedMinScore: 0.4,
			expectedMaxScore: 0.6,
		},
		{
			name: "no scores returns default",
			result: &ValidationResult{
				Valid:            true,
				SecurityScore:    0,
				PerformanceScore: 0,
				AccuracyScore:    0,
			},
			expectedMinScore: 0.4,
			expectedMaxScore: 0.6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := validator.calculateOverallScore(tt.result)
			assert.GreaterOrEqual(t, score, tt.expectedMinScore)
			assert.LessOrEqual(t, score, tt.expectedMaxScore)
		})
	}
}

func TestSecurityTestConfig_Defaults(t *testing.T) {
	config := DefaultSecurityTestConfig()

	assert.True(t, config.TestReDoSVulnerability)
	assert.True(t, config.TestInjectionAttacks)
	assert.True(t, config.TestPatternComplexity)
	assert.True(t, config.TestResourceConsumption)
	assert.Equal(t, 1000, config.MaxAllowedComplexity)
	assert.Equal(t, 100.0, config.MaxExecutionTimeMs)
	assert.NotEmpty(t, config.DangerousPatterns)
	assert.NotEmpty(t, config.TestInputs)
}

func TestPatternValidator_TestReDoSVulnerability(t *testing.T) {
	logger := zap.NewNop()
	validator := NewPatternValidator(logger)
	config := DefaultSecurityTestConfig()

	tests := []struct {
		name           string
		patternContent string
		minScore       float64
		maxScore       float64
	}{
		{
			name:           "safe pattern has high score",
			patternContent: `^[a-z]+$`,
			minScore:       0.8,
			maxScore:       1.0,
		},
		{
			name:           "pattern with single wildcard is ok",
			patternContent: `https://*.example.com`,
			minScore:       0.5,
			maxScore:       1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, results := validator.testReDoSVulnerability(tt.patternContent, config)
			assert.GreaterOrEqual(t, score, tt.minScore)
			assert.LessOrEqual(t, score, tt.maxScore)
			assert.NotNil(t, results)
		})
	}
}

func TestPatternValidator_TestPatternComplexity(t *testing.T) {
	logger := zap.NewNop()
	validator := NewPatternValidator(logger)
	config := DefaultSecurityTestConfig()

	tests := []struct {
		name             string
		patternContent   string
		expectAcceptable bool
	}{
		{
			name:             "simple pattern is acceptable",
			patternContent:   "example.com",
			expectAcceptable: true,
		},
		{
			name:             "moderate complexity is acceptable",
			patternContent:   `https?://[a-z]+\.example\.(com|org|net)`,
			expectAcceptable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, results := validator.testPatternComplexity(tt.patternContent, config)
			assert.Greater(t, score, 0.0)
			acceptable, ok := results["acceptable"].(bool)
			if ok {
				assert.Equal(t, tt.expectAcceptable, acceptable)
			}
		})
	}
}

func TestPatternValidator_TestInjectionSafety(t *testing.T) {
	logger := zap.NewNop()
	validator := NewPatternValidator(logger)
	config := DefaultSecurityTestConfig()

	tests := []struct {
		name           string
		patternContent string
		expectSafe     bool
		minScore       float64
	}{
		{
			name:           "normal domain is safe",
			patternContent: "example.com",
			expectSafe:     true,
			minScore:       1.0,
		},
		{
			name:           "path pattern is safe",
			patternContent: "/admin/users",
			expectSafe:     true,
			minScore:       1.0,
		},
		{
			name:           "pattern with javascript: is suspicious",
			patternContent: "javascript:alert",
			expectSafe:     false,
			minScore:       0.0,
		},
		{
			name:           "pattern with SQL is suspicious",
			patternContent: "DROP TABLE users",
			expectSafe:     false,
			minScore:       0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, results := validator.testInjectionSafety(tt.patternContent, config)
			assert.GreaterOrEqual(t, score, tt.minScore)
			safe, ok := results["safe"].(bool)
			if ok {
				assert.Equal(t, tt.expectSafe, safe)
			}
		})
	}
}

func TestPatternValidator_generateRecommendations_CoversAllSignals(t *testing.T) {
	validator := NewPatternValidator(zap.NewNop())

	pattern := &models.EnhancedModerationPattern{
		PatternID:      "rec-1",
		PatternType:    URLPatternRegexStr,
		PatternContent: strings.Repeat("x", 501),
	}
	result := &ValidationResult{
		SecurityScore:    0.6,
		PerformanceScore: 0.6,
		CompilationTime:  100,
	}

	validator.generateRecommendations(pattern, result)
	assert.Len(t, result.Recommendations, 5)
}

func TestPatternValidator_CreateTestResult(t *testing.T) {
	logger := zap.NewNop()
	validator := NewPatternValidator(logger)

	pattern := &models.EnhancedModerationPattern{
		PatternID:      "test-123",
		PatternContent: "example.com",
		PatternType:    URLPatternDomainStr,
	}

	validationResult := &ValidationResult{
		Valid:            true,
		Score:            0.9,
		SecurityScore:    0.95,
		PerformanceScore: 0.85,
		AccuracyScore:    0.8,
		CompilationTime:  1.5,
		TestResults:      map[string]interface{}{"test": "data"},
		Errors:           []string{},
	}

	testResult := validator.CreateTestResult(pattern, validationResult, "validation", "system")

	assert.NotEmpty(t, testResult.TestID)
	assert.Equal(t, pattern.PatternID, testResult.PatternID)
	assert.Equal(t, pattern.PatternType, testResult.PatternType)
	assert.Equal(t, "validation", testResult.TestType)
	assert.True(t, testResult.Passed)
	assert.Equal(t, validationResult.Score, testResult.Score)
	assert.Equal(t, validationResult.CompilationTime, testResult.ExecutionTime)
	assert.Equal(t, "system", testResult.RunBy)
}
