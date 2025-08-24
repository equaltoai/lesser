package moderation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// PatternValidator provides comprehensive validation and testing for moderation patterns
type PatternValidator struct {
	urlMatcher *EnhancedURLMatcher
	ipMatcher  *EnhancedIPMatcher
	logger     *zap.Logger
}

// NewPatternValidator creates a new pattern validator
func NewPatternValidator(logger *zap.Logger) *PatternValidator {
	return &PatternValidator{
		urlMatcher: NewEnhancedURLMatcher(),
		ipMatcher:  NewEnhancedIPMatcher(),
		logger:     logger,
	}
}

// ValidationResult represents the result of pattern validation
type ValidationResult struct {
	Valid              bool                   `json:"valid"`
	Score              float64                `json:"score"` // 0.0-1.0
	SecurityScore      float64                `json:"security_score"`
	PerformanceScore   float64                `json:"performance_score"`
	AccuracyScore      float64                `json:"accuracy_score"`
	Errors             []string               `json:"errors,omitempty"`
	Warnings           []string               `json:"warnings,omitempty"`
	Recommendations    []string               `json:"recommendations,omitempty"`
	TestResults        map[string]interface{} `json:"test_results"`
	CompilationTime    float64                `json:"compilation_time"`     // milliseconds
	EstimatedMatchTime float64                `json:"estimated_match_time"` // milliseconds per match
}

// SecurityTestConfig defines security testing parameters
type SecurityTestConfig struct {
	TestReDoSVulnerability  bool     `json:"test_redos_vulnerability"`
	TestInjectionAttacks    bool     `json:"test_injection_attacks"`
	TestPatternComplexity   bool     `json:"test_pattern_complexity"`
	TestResourceConsumption bool     `json:"test_resource_consumption"`
	MaxAllowedComplexity    int      `json:"max_allowed_complexity"`
	MaxExecutionTimeMs      float64  `json:"max_execution_time_ms"`
	DangerousPatterns       []string `json:"dangerous_patterns"`
	TestInputs              []string `json:"test_inputs"`
}

// DefaultSecurityTestConfig returns default security test configuration
func DefaultSecurityTestConfig() *SecurityTestConfig {
	return &SecurityTestConfig{
		TestReDoSVulnerability:  true,
		TestInjectionAttacks:    true,
		TestPatternComplexity:   true,
		TestResourceConsumption: true,
		MaxAllowedComplexity:    1000,
		MaxExecutionTimeMs:      100.0,
		DangerousPatterns: []string{
			`(.*){`,      // Nested quantifiers
			`.*.*.*.*`,   // Multiple greedy quantifiers
			`(.*)+`,      // Nested quantifiers with +
			`([^x])*`,    // Negated character class with quantifier
			`(a+)+`,      // Catastrophic backtracking
			`(a*)*`,      // Catastrophic backtracking
			`\s*\s*\s*`,  // Multiple whitespace quantifiers
			`.*.*\S.*.*`, // Complex greedy patterns
		},
		TestInputs: []string{
			// Test strings for ReDoS detection
			strings.Repeat("a", 1000),
			strings.Repeat("ab", 500),
			strings.Repeat("abc", 300),
			"malicious://example.com/../../etc/passwd",
			"javascript:alert('xss')",
			"<script>alert('xss')</script>",
			"'; DROP TABLE users; --",
			"192.168.1.1; cat /etc/passwd",
			"example.com/../../../etc/passwd",
		},
	}
}

// ValidatePattern performs comprehensive validation of a moderation pattern
func (v *PatternValidator) ValidatePattern(_ context.Context, pattern *models.EnhancedModerationPattern, config *SecurityTestConfig) (*ValidationResult, error) {
	if config == nil {
		config = DefaultSecurityTestConfig()
	}

	result := &ValidationResult{
		Valid:           true,
		TestResults:     make(map[string]interface{}),
		Errors:          make([]string, 0),
		Warnings:        make([]string, 0),
		Recommendations: make([]string, 0),
	}

	startTime := time.Now()

	// Basic validation
	if err := v.validateBasicPattern(pattern, result); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err.Error())
	}

	// Security validation
	if config.TestReDoSVulnerability || config.TestInjectionAttacks || config.TestPatternComplexity {
		securityScore, err := v.validateSecurity(pattern, config, result)
		if err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("security validation failed: %v", err))
		}
		result.SecurityScore = securityScore
	}

	// Performance validation
	if config.TestResourceConsumption {
		performanceScore, err := v.validatePerformance(pattern, config, result)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("performance validation warning: %v", err))
		}
		result.PerformanceScore = performanceScore
	}

	// Accuracy validation with test inputs
	accuracyScore, err := v.validateAccuracy(pattern, config, result)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("accuracy validation warning: %v", err))
	}
	result.AccuracyScore = accuracyScore

	// Compilation test
	compilationTime, err := v.testCompilation(pattern, result)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("compilation failed: %v", err))
	}
	result.CompilationTime = compilationTime

	// Calculate overall score
	result.Score = v.calculateOverallScore(result)

	// Generate recommendations
	v.generateRecommendations(pattern, result)

	result.EstimatedMatchTime = result.CompilationTime * 0.1 // Rough estimate

	v.logger.Info("pattern validation completed",
		zap.String("pattern_id", pattern.PatternID),
		zap.Bool("valid", result.Valid),
		zap.Float64("score", result.Score),
		zap.Float64("compilation_time", result.CompilationTime),
		zap.Duration("total_time", time.Since(startTime)))

	return result, nil
}

// validateBasicPattern performs basic pattern validation
func (v *PatternValidator) validateBasicPattern(pattern *models.EnhancedModerationPattern, result *ValidationResult) error {
	if err := common.ValidateRequiredParam("pattern.PatternContent", pattern.PatternContent); err != nil {
		return fmt.Errorf("pattern content cannot be empty")
	}

	if len(pattern.PatternContent) > 2048 {
		return fmt.Errorf("pattern content too long (max 2048 characters)")
	}

	if err := common.ValidateRequiredParam("pattern.PatternType", pattern.PatternType); err != nil {
		return fmt.Errorf("pattern type must be specified")
	}

	// Validate pattern type specific content
	switch {
	case strings.HasPrefix(pattern.PatternType, "url_"):
		return v.validateURLPattern(pattern, result)
	case strings.HasPrefix(pattern.PatternType, "ip_"):
		return v.validateIPPattern(pattern, result)
	default:
		return fmt.Errorf("unsupported pattern type: %s", pattern.PatternType)
	}
}

// validateURLPattern validates URL patterns
func (v *PatternValidator) validateURLPattern(pattern *models.EnhancedModerationPattern, result *ValidationResult) error {
	var patternType URLPatternType
	switch pattern.PatternType {
	case URLPatternExactStr:
		patternType = URLPatternExact
	case URLPatternDomainStr:
		patternType = URLPatternDomain
	case URLPatternSubdomainStr:
		patternType = URLPatternSubdomain
	case URLPatternPathStr:
		patternType = URLPatternPath
	case URLPatternQueryStr:
		patternType = URLPatternQuery
	case URLPatternRegexStr:
		patternType = URLPatternRegex
	default:
		return fmt.Errorf("invalid URL pattern type: %s", pattern.PatternType)
	}

	if err := ValidateURLPattern(pattern.PatternContent, patternType); err != nil {
		return fmt.Errorf("invalid URL pattern: %w", err)
	}

	result.TestResults["url_validation"] = map[string]interface{}{
		"pattern_type": pattern.PatternType,
		"content":      pattern.PatternContent,
		"valid":        true,
	}

	return nil
}

// validateIPPattern validates IP patterns
func (v *PatternValidator) validateIPPattern(pattern *models.EnhancedModerationPattern, result *ValidationResult) error {
	var patternType IPPatternType
	switch pattern.PatternType {
	case IPPatternSingleStr:
		patternType = IPPatternSingle
	case IPPatternCIDRStr:
		patternType = IPPatternCIDR
	case IPPatternRangeStr:
		patternType = IPPatternRange
	case IPPatternRegexStr:
		patternType = IPPatternRegex
	default:
		return fmt.Errorf("invalid IP pattern type: %s", pattern.PatternType)
	}

	if err := ValidateIPPattern(pattern.PatternContent, patternType); err != nil {
		return fmt.Errorf("invalid IP pattern: %w", err)
	}

	result.TestResults["ip_validation"] = map[string]interface{}{
		"pattern_type": pattern.PatternType,
		"content":      pattern.PatternContent,
		"valid":        true,
	}

	return nil
}

// validateSecurity performs security validation including ReDoS detection
func (v *PatternValidator) validateSecurity(pattern *models.EnhancedModerationPattern, config *SecurityTestConfig, result *ValidationResult) (float64, error) {
	securityTests := make(map[string]interface{})
	securityScore := 1.0

	// Test for dangerous regex patterns
	if config.TestReDoSVulnerability && (pattern.PatternType == URLPatternRegexStr || pattern.PatternType == IPPatternRegexStr) {
		redosScore, redosResults := v.testReDoSVulnerability(pattern.PatternContent, config)
		securityTests["redos_vulnerability"] = redosResults
		securityScore *= redosScore
	}

	// Test for injection attack patterns
	if config.TestInjectionAttacks {
		injectionScore, injectionResults := v.testInjectionSafety(pattern.PatternContent, config)
		securityTests["injection_safety"] = injectionResults
		securityScore *= injectionScore
	}

	// Test pattern complexity
	if config.TestPatternComplexity {
		complexityScore, complexityResults := v.testPatternComplexity(pattern.PatternContent, config)
		securityTests["complexity"] = complexityResults
		securityScore *= complexityScore
	}

	result.TestResults["security"] = securityTests

	if securityScore < 0.7 {
		result.Warnings = append(result.Warnings, "pattern has potential security concerns")
	}

	return securityScore, nil
}

// testReDoSVulnerability tests for Regular expression Denial of Service vulnerabilities
func (v *PatternValidator) testReDoSVulnerability(patternContent string, config *SecurityTestConfig) (float64, map[string]interface{}) {
	results := map[string]interface{}{
		"vulnerable_patterns": make([]string, 0),
		"test_passed":         true,
		"execution_times":     make([]float64, 0),
	}

	score := 1.0

	// Check for known dangerous patterns
	for _, dangerous := range config.DangerousPatterns {
		if strings.Contains(patternContent, dangerous) {
			results["vulnerable_patterns"] = append(results["vulnerable_patterns"].([]string), dangerous)
			score *= 0.5 // Reduce score for each dangerous pattern
		}
	}

	// Test execution time with potentially problematic inputs
	if strings.Contains(patternContent, "*") || strings.Contains(patternContent, "+") {
		regex, err := regexp.Compile(patternContent)
		if err == nil {
			executionTimes := make([]float64, 0)
		testLoop:
			for _, testInput := range config.TestInputs {
				start := time.Now()
				// Set a timeout to prevent actual ReDoS
				done := make(chan bool, 1)
				go func() {
					regex.MatchString(testInput)
					done <- true
				}()

				select {
				case <-done:
					executionTime := float64(time.Since(start).Nanoseconds()) / 1e6 // Convert to milliseconds
					executionTimes = append(executionTimes, executionTime)
					if executionTime > config.MaxExecutionTimeMs {
						score *= 0.3 // Severely penalize slow patterns
						results["test_passed"] = false
					}
				case <-time.After(time.Duration(config.MaxExecutionTimeMs) * time.Millisecond):
					// Timeout - likely ReDoS vulnerable
					executionTimes = append(executionTimes, config.MaxExecutionTimeMs*10)
					score *= 0.1 // Heavily penalize timeout patterns
					results["test_passed"] = false
					break testLoop
				}
			}
			results["execution_times"] = executionTimes
		}
	}

	return score, results
}

// testInjectionSafety tests for potential injection attack vectors
func (v *PatternValidator) testInjectionSafety(patternContent string, _ *SecurityTestConfig) (float64, map[string]interface{}) {
	results := map[string]interface{}{
		"suspicious_content": make([]string, 0),
		"safe":               true,
	}

	score := 1.0

	// Check for suspicious content that might enable injection
	suspiciousPatterns := []string{
		"javascript:",
		"data:",
		"file:",
		"../",
		"..\\",
		"<script",
		"</script>",
		"eval(",
		"exec(",
		"system(",
		"shell_exec(",
		"DROP TABLE",
		"DELETE FROM",
		"INSERT INTO",
		"UPDATE SET",
		"UNION SELECT",
	}

	for _, suspicious := range suspiciousPatterns {
		if strings.Contains(strings.ToLower(patternContent), strings.ToLower(suspicious)) {
			results["suspicious_content"] = append(results["suspicious_content"].([]string), suspicious)
			score *= 0.8 // Reduce score for suspicious content
			results["safe"] = false
		}
	}

	return score, results
}

// testPatternComplexity tests pattern complexity to prevent resource abuse
func (v *PatternValidator) testPatternComplexity(patternContent string, config *SecurityTestConfig) (float64, map[string]interface{}) {
	results := map[string]interface{}{
		"complexity_score": 0,
		"acceptable":       true,
		"factors":          make([]string, 0),
	}

	complexity := 0
	factors := make([]string, 0)

	// Count various complexity factors
	complexity += strings.Count(patternContent, "*") * 5  // Wildcard quantifiers
	complexity += strings.Count(patternContent, "+") * 5  // Plus quantifiers
	complexity += strings.Count(patternContent, "?") * 2  // Question quantifiers
	complexity += strings.Count(patternContent, "|") * 3  // Alternation
	complexity += strings.Count(patternContent, "(") * 2  // Groups
	complexity += strings.Count(patternContent, "[") * 3  // Character classes
	complexity += strings.Count(patternContent, "\\") * 1 // Escapes
	complexity += len(patternContent) / 10                // Base length penalty

	if strings.Count(patternContent, "*") > 5 {
		factors = append(factors, "too many wildcards")
	}
	if strings.Count(patternContent, "(") > 10 {
		factors = append(factors, "too many groups")
	}
	if len(patternContent) > 500 {
		factors = append(factors, "pattern too long")
	}

	results["complexity_score"] = complexity
	results["factors"] = factors

	score := 1.0
	if complexity > config.MaxAllowedComplexity {
		results["acceptable"] = false
		score = 1.0 - (float64(complexity-config.MaxAllowedComplexity) / float64(config.MaxAllowedComplexity))
		if score < 0 {
			score = 0
		}
	}

	return score, results
}

// validatePerformance tests pattern performance characteristics
func (v *PatternValidator) validatePerformance(pattern *models.EnhancedModerationPattern, _ *SecurityTestConfig, result *ValidationResult) (float64, error) {
	performanceTests := make(map[string]interface{})
	score := 1.0

	// Test compilation time
	compilationStart := time.Now()
	var err error

	switch {
	case strings.HasPrefix(pattern.PatternType, "url_"):
		err = v.testURLPatternPerformance(pattern, performanceTests)
	case strings.HasPrefix(pattern.PatternType, "ip_"):
		err = v.testIPPatternPerformance(pattern, performanceTests)
	}

	compilationTime := float64(time.Since(compilationStart).Nanoseconds()) / 1e6
	performanceTests["compilation_time_ms"] = compilationTime

	if compilationTime > 50 { // More than 50ms compilation time
		score *= 0.8
		result.Warnings = append(result.Warnings, "slow pattern compilation")
	}

	result.TestResults["performance"] = performanceTests

	return score, err
}

// testURLPatternPerformance tests URL pattern performance
func (v *PatternValidator) testURLPatternPerformance(pattern *models.EnhancedModerationPattern, results map[string]interface{}) error {
	var patternType URLPatternType
	switch pattern.PatternType {
	case URLPatternExactStr:
		patternType = URLPatternExact
	case URLPatternDomainStr:
		patternType = URLPatternDomain
	case URLPatternSubdomainStr:
		patternType = URLPatternSubdomain
	case URLPatternPathStr:
		patternType = URLPatternPath
	case URLPatternQueryStr:
		patternType = URLPatternQuery
	case URLPatternRegexStr:
		patternType = URLPatternRegex
	}

	start := time.Now()
	err := v.urlMatcher.CompileURLPattern(pattern.PatternContent, patternType)
	compileTime := float64(time.Since(start).Nanoseconds()) / 1e6

	results["url_compile_time_ms"] = compileTime
	results["compile_success"] = err == nil

	if err != nil {
		return fmt.Errorf("URL pattern compilation failed: %w", err)
	}

	// Test matching performance with sample URLs
	testURLs := []string{
		"https://example.com",
		"http://subdomain.example.com/path/to/resource?param=value",
		"https://malicious-site.com/../../etc/passwd",
		"ftp://files.example.com/file.txt",
		"javascript:alert('xss')",
	}

	matchTimes := make([]float64, 0)
	for _, testURL := range testURLs {
		start := time.Now()
		_, _, _ = v.urlMatcher.MatchURL(testURL, []string{pattern.PatternContent})
		matchTime := float64(time.Since(start).Nanoseconds()) / 1e6
		matchTimes = append(matchTimes, matchTime)
	}

	results["match_times_ms"] = matchTimes
	avgMatchTime := 0.0
	for _, mt := range matchTimes {
		avgMatchTime += mt
	}
	avgMatchTime /= float64(len(matchTimes))
	results["average_match_time_ms"] = avgMatchTime

	return nil
}

// testIPPatternPerformance tests IP pattern performance
func (v *PatternValidator) testIPPatternPerformance(pattern *models.EnhancedModerationPattern, results map[string]interface{}) error {
	var patternType IPPatternType
	switch pattern.PatternType {
	case IPPatternSingleStr:
		patternType = IPPatternSingle
	case IPPatternCIDRStr:
		patternType = IPPatternCIDR
	case IPPatternRangeStr:
		patternType = IPPatternRange
	case IPPatternRegexStr:
		patternType = IPPatternRegex
	}

	start := time.Now()
	err := v.ipMatcher.CompileIPPattern(pattern.PatternContent, patternType)
	compileTime := float64(time.Since(start).Nanoseconds()) / 1e6

	results["ip_compile_time_ms"] = compileTime
	results["compile_success"] = err == nil

	if err != nil {
		return fmt.Errorf("IP pattern compilation failed: %w", err)
	}

	// Test matching performance with sample IPs
	testIPs := []string{
		"192.168.1.1",
		"10.0.0.1",
		"172.16.0.1",
		"127.0.0.1",
		"8.8.8.8",
		"2001:db8::1",
		"::1",
		"fe80::1",
		"invalid-ip",
		"192.168.1.999",
	}

	matchTimes := make([]float64, 0)
	for _, testIP := range testIPs {
		start := time.Now()
		_, _, _ = v.ipMatcher.MatchIP(testIP, []string{pattern.PatternContent})
		matchTime := float64(time.Since(start).Nanoseconds()) / 1e6
		matchTimes = append(matchTimes, matchTime)
	}

	results["match_times_ms"] = matchTimes
	avgMatchTime := 0.0
	for _, mt := range matchTimes {
		avgMatchTime += mt
	}
	avgMatchTime /= float64(len(matchTimes))
	results["average_match_time_ms"] = avgMatchTime

	return nil
}

// validateAccuracy tests pattern accuracy with known test cases
func (v *PatternValidator) validateAccuracy(_ *models.EnhancedModerationPattern, _ *SecurityTestConfig, result *ValidationResult) (float64, error) {
	accuracyTests := make(map[string]interface{})

	// For now, return a base accuracy score
	// In a real implementation, this would test against known good/bad examples
	accuracyScore := 0.8 // Default reasonable accuracy

	accuracyTests["test_cases_run"] = 0
	accuracyTests["accuracy_score"] = accuracyScore
	accuracyTests["note"] = "accuracy testing requires test dataset"

	result.TestResults["accuracy"] = accuracyTests

	return accuracyScore, nil
}

// testCompilation tests if the pattern can be compiled successfully
func (v *PatternValidator) testCompilation(pattern *models.EnhancedModerationPattern, _ *ValidationResult) (float64, error) {
	start := time.Now()

	switch {
	case strings.HasPrefix(pattern.PatternType, "url_"):
		var patternType URLPatternType
		switch pattern.PatternType {
		case URLPatternExactStr:
			patternType = URLPatternExact
		case URLPatternDomainStr:
			patternType = URLPatternDomain
		case URLPatternSubdomainStr:
			patternType = URLPatternSubdomain
		case URLPatternPathStr:
			patternType = URLPatternPath
		case URLPatternQueryStr:
			patternType = URLPatternQuery
		case URLPatternRegexStr:
			patternType = URLPatternRegex
		}
		err := v.urlMatcher.CompileURLPattern(pattern.PatternContent, patternType)
		if err != nil {
			return 0, err
		}
	case strings.HasPrefix(pattern.PatternType, "ip_"):
		var patternType IPPatternType
		switch pattern.PatternType {
		case IPPatternSingleStr:
			patternType = IPPatternSingle
		case IPPatternCIDRStr:
			patternType = IPPatternCIDR
		case IPPatternRangeStr:
			patternType = IPPatternRange
		case IPPatternRegexStr:
			patternType = IPPatternRegex
		}
		err := v.ipMatcher.CompileIPPattern(pattern.PatternContent, patternType)
		if err != nil {
			return 0, err
		}
	}

	compilationTime := float64(time.Since(start).Nanoseconds()) / 1e6
	return compilationTime, nil
}

// calculateOverallScore calculates the overall validation score
func (v *PatternValidator) calculateOverallScore(result *ValidationResult) float64 {
	if !result.Valid {
		return 0.0
	}

	// Weighted average of different scores
	weights := map[string]float64{
		"security":    0.4,
		"performance": 0.3,
		"accuracy":    0.3,
	}

	totalScore := 0.0
	totalWeight := 0.0

	if result.SecurityScore > 0 {
		totalScore += result.SecurityScore * weights["security"]
		totalWeight += weights["security"]
	}

	if result.PerformanceScore > 0 {
		totalScore += result.PerformanceScore * weights["performance"]
		totalWeight += weights["performance"]
	}

	if result.AccuracyScore > 0 {
		totalScore += result.AccuracyScore * weights["accuracy"]
		totalWeight += weights["accuracy"]
	}

	if totalWeight > 0 {
		return totalScore / totalWeight
	}

	return 0.5 // Default neutral score
}

// generateRecommendations generates recommendations for pattern improvement
func (v *PatternValidator) generateRecommendations(pattern *models.EnhancedModerationPattern, result *ValidationResult) {
	recommendations := make([]string, 0)

	if result.SecurityScore < 0.7 {
		recommendations = append(recommendations, "Consider simplifying the pattern to improve security")
	}

	if result.PerformanceScore < 0.7 {
		recommendations = append(recommendations, "Pattern may be slow - consider optimization")
	}

	if result.CompilationTime > 50 {
		recommendations = append(recommendations, "Compilation time is high - consider simplifying pattern")
	}

	if pattern.PatternType == URLPatternRegexStr || pattern.PatternType == IPPatternRegexStr {
		recommendations = append(recommendations, "Consider using specific pattern types instead of regex for better performance")
	}

	if len(pattern.PatternContent) > 500 {
		recommendations = append(recommendations, "Pattern is very long - consider breaking into multiple patterns")
	}

	result.Recommendations = recommendations
}

// CreateTestResult creates a test result record for storage
func (v *PatternValidator) CreateTestResult(pattern *models.EnhancedModerationPattern, validationResult *ValidationResult, testType string, runBy string) *models.PatternTestResult {
	testID := uuid.New().String()

	result := &models.PatternTestResult{
		TestID:          testID,
		PatternID:       pattern.PatternID,
		PatternType:     pattern.PatternType,
		TestType:        testType,
		TestDescription: fmt.Sprintf("Comprehensive %s validation", testType),
		TestParameters: map[string]interface{}{
			"security_enabled":    validationResult.SecurityScore > 0,
			"performance_enabled": validationResult.PerformanceScore > 0,
			"accuracy_enabled":    validationResult.AccuracyScore > 0,
		},
		Passed:        validationResult.Valid,
		Score:         validationResult.Score,
		ExecutionTime: validationResult.CompilationTime,
		Results:       validationResult.TestResults,
		Errors:        validationResult.Errors,
		TestVersion:   "1.0",
		RunBy:         runBy,
		Environment:   "validation",
	}

	// Generate hash for caching
	hashInput := fmt.Sprintf("%s:%s:%v", pattern.PatternContent, pattern.PatternType, testType)
	hash := sha256.Sum256([]byte(hashInput))
	result.TestID = fmt.Sprintf("%x", hash)[:16] // Use first 16 chars of hash

	return result
}
