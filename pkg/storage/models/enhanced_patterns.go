package models

import (
	"fmt"
	"time"
)

// EnhancedModerationPattern represents an enhanced moderation pattern with advanced matching capabilities
type EnhancedModerationPattern struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "ENHANCED_PATTERN#{pattern_id}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // "METADATA"

	// GSI1 - Active pattern queries with priority
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK" json:"gsi1_pk,omitempty"` // "ENHANCED_PATTERNS#ACTIVE" (when active)
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK" json:"gsi1_sk,omitempty"` // "{priority}#{type}#{severity}#{pattern_id}"

	// GSI2 - Type-based queries
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsI2PK" json:"gsi2_pk,omitempty"` // "ENHANCED_PATTERNS#{type}"
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsI2SK" json:"gsi2_sk,omitempty"` // "{effectiveness}#{updated_at}#{pattern_id}"

	// GSI3 - Performance metric queries
	GSI3PK string `dynamorm:"index:gsi3,pk,attr:gsI3PK" json:"gsi3_pk,omitempty"` // "PATTERN_METRICS#{category}"
	GSI3SK string `dynamorm:"index:gsi3,sk,attr:gsI3SK" json:"gsi3_sk,omitempty"` // "{effectiveness}#{match_count}#{pattern_id}"

	// Type marker
	Type string `dynamorm:"attr:type" json:"type"` // "ENHANCED_PATTERN"

	// Pattern identification
	PatternID   string `dynamorm:"attr:patternID" json:"pattern_id"`
	Name        string `dynamorm:"attr:name" json:"name"`
	Description string `dynamorm:"attr:description" json:"description"`
	Version     int    `dynamorm:"attr:version" json:"version"`

	// Pattern configuration
	PatternType     string `dynamorm:"attr:patternType" json:"pattern_type"`       // "url_exact", ...
	PatternContent  string `dynamorm:"attr:patternContent" json:"pattern_content"` // The actual pattern string
	Category        string `dynamorm:"attr:category" json:"category"`              // "spam", ...
	Severity        string `dynamorm:"attr:severity" json:"severity"`              // "low", "medium", etc.
	Priority        int    `dynamorm:"attr:priority" json:"priority"`              // 1-10, higher is more important
	Active          bool   `dynamorm:"attr:active" json:"active"`
	Compiled        bool   `dynamorm:"attr:compiled" json:"compiled"`                // Whether pattern has been compiled successfully
	CompilationHash string `dynamorm:"attr:compilationHash" json:"compilation_hash"` // Hash of compiled pattern for cache invalidation

	// Pattern behavior
	Action              string   `dynamorm:"attr:action" json:"action"`                            // "flag", etc.
	BlockDuration       int64    `dynamorm:"attr:blockDuration" json:"block_duration"`             // Duration in seconds, 0 for permanent
	EscalationThreshold int      `dynamorm:"attr:escalationThreshold" json:"escalation_threshold"` // Number of matches before escalation
	WhitelistOverride   bool     `dynamorm:"attr:whitelistOverride" json:"whitelist_override"`     // Whether this pattern can be overridden by whitelist
	Tags                []string `dynamorm:"attr:tags" json:"tags,omitempty"`                      // Additional categorization tags

	// Performance metrics
	MatchCount         int64     `dynamorm:"attr:matchCount" json:"match_count"`
	FalsePositiveCount int64     `dynamorm:"attr:falsePositiveCount" json:"false_positive_count"`
	TruePositiveCount  int64     `dynamorm:"attr:truePositiveCount" json:"true_positive_count"`
	Effectiveness      float64   `dynamorm:"attr:effectiveness" json:"effectiveness"` // Calculated effectiveness score 0.0-1.0
	ConfidenceScore    float64   `dynamorm:"attr:confidenceScore" json:"confidence_score"`
	LastMatch          time.Time `dynamorm:"attr:lastMatch" json:"last_match,omitempty"`
	AverageMatchTime   float64   `dynamorm:"attr:averageMatchTime" json:"average_match_time"` // Average time to match in milliseconds

	// Pattern validation metrics
	TestResults     map[string]interface{} `dynamorm:"attr:testResults" json:"test_results,omitempty"` // Results from pattern testing
	ValidationScore float64                `dynamorm:"attr:validationScore" json:"validation_score"`   // 0.0-1.0 based on test results
	LastValidated   time.Time              `dynamorm:"attr:lastValidated" json:"last_validated,omitempty"`

	// Metadata
	CreatedBy string    `dynamorm:"attr:createdBy" json:"created_by"`
	UpdatedBy string    `dynamorm:"attr:updatedBy" json:"updated_by,omitempty"`
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
	LastUsed  time.Time `dynamorm:"attr:lastUsed" json:"last_used,omitempty"`
	ExpiresAt time.Time `dynamorm:"attr:expiresAt" json:"expires_at,omitempty"`

	// DynamoDB TTL (90 days default)
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (EnhancedModerationPattern) TableName() string {
	return MainTableName
}

// GetPK returns the partition key
func (p *EnhancedModerationPattern) GetPK() string {
	return p.PK
}

// GetSK returns the sort key
func (p *EnhancedModerationPattern) GetSK() string {
	return p.SK
}

// UpdateKeys updates the GSI keys based on current field values
func (p *EnhancedModerationPattern) UpdateKeys() error {
	p.PK = fmt.Sprintf("ENHANCED_PATTERN#%s", p.PatternID)
	p.SK = SKMetadata

	// GSI1 - Active patterns with priority ordering
	if p.Active {
		p.GSI1PK = "ENHANCED_PATTERNS#ACTIVE"
		p.GSI1SK = fmt.Sprintf("%02d#%s#%s#%s", p.Priority, p.PatternType, p.Severity, p.PatternID)
	} else {
		p.GSI1PK = ""
		p.GSI1SK = ""
	}

	// GSI2 - Type-based queries with effectiveness
	p.GSI2PK = fmt.Sprintf("ENHANCED_PATTERNS#%s", p.PatternType)
	effectivenessStr := fmt.Sprintf("%06.3f", p.Effectiveness)
	p.GSI2SK = fmt.Sprintf("%s#%s#%s", effectivenessStr, p.UpdatedAt.Format(time.RFC3339), p.PatternID)

	// GSI3 - Performance metrics queries
	p.GSI3PK = fmt.Sprintf("PATTERN_METRICS#%s", p.Category)
	matchCountStr := fmt.Sprintf("%010d", p.MatchCount)
	p.GSI3SK = fmt.Sprintf("%06.3f#%s#%s", p.Effectiveness, matchCountStr, p.PatternID)

	// Set type marker
	p.Type = "ENHANCED_PATTERN"

	// Set TTL (90 days default)
	if p.TTL == 0 {
		p.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()
	}
	return nil
}

// CalculateEffectiveness calculates the pattern effectiveness score
func (p *EnhancedModerationPattern) CalculateEffectiveness() {
	if p.MatchCount == 0 {
		p.Effectiveness = 0.5 // Neutral for new patterns
		return
	}

	// Base effectiveness = true positives / total matches
	baseEffectiveness := float64(p.TruePositiveCount) / float64(p.MatchCount)

	// Adjust for confidence score
	confidenceAdjustment := p.ConfidenceScore * 0.2

	// Adjust for recency - patterns that haven't matched recently are less effective
	recencyAdjustment := 1.0
	if !p.LastMatch.IsZero() {
		daysSinceLastMatch := time.Since(p.LastMatch).Hours() / 24
		if daysSinceLastMatch > 30 {
			recencyAdjustment = 0.7 // Reduce effectiveness for stale patterns
		} else if daysSinceLastMatch > 7 {
			recencyAdjustment = 0.9
		}
	}

	// Adjust for validation score
	validationAdjustment := p.ValidationScore * 0.1

	// Calculate final effectiveness
	p.Effectiveness = (baseEffectiveness + confidenceAdjustment + validationAdjustment) * recencyAdjustment

	// Ensure bounds
	if p.Effectiveness > 1.0 {
		p.Effectiveness = 1.0
	}
	if p.Effectiveness < 0.0 {
		p.Effectiveness = 0.0
	}
}

// IsExpired checks if the pattern has expired
func (p *EnhancedModerationPattern) IsExpired() bool {
	return !p.ExpiresAt.IsZero() && time.Now().After(p.ExpiresAt)
}

// ShouldEscalate checks if pattern matches have reached escalation threshold
func (p *EnhancedModerationPattern) ShouldEscalate() bool {
	return p.EscalationThreshold > 0 && p.MatchCount >= int64(p.EscalationThreshold)
}

// PatternCache represents cached compiled patterns for performance
type PatternCache struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "PATTERN_CACHE#{pattern_type}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "COMPILED#{pattern_id}"

	// GSI1 - Cache invalidation queries
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK" json:"gsi1_pk,omitempty"` // "PATTERN_CACHE#ACTIVE"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK" json:"gsi1_sk,omitempty"` // "{last_updated}#{pattern_id}"

	// Type marker
	Type string `dynamorm:"attr:type" json:"type"` // "PATTERN_CACHE"

	// Cache data
	PatternID       string                 `dynamorm:"attr:patternID" json:"pattern_id"`
	PatternType     string                 `dynamorm:"attr:patternType" json:"pattern_type"`
	CompilationHash string                 `dynamorm:"attr:compilationHash" json:"compilation_hash"`
	CompiledData    map[string]interface{} `dynamorm:"attr:compiledData" json:"compiled_data"` // Serialized compiled pattern data
	CompileTime     float64                `dynamorm:"attr:compileTime" json:"compile_time"`   // Time taken to compile in milliseconds
	CacheHits       int64                  `dynamorm:"attr:cacheHits" json:"cache_hits"`       // Number of times this cache entry was used
	LastUsed        time.Time              `dynamorm:"attr:lastUsed" json:"last_used"`
	CreatedAt       time.Time              `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt       time.Time              `dynamorm:"attr:updatedAt" json:"updated_at"`

	// TTL for cache expiration (24 hours default)
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (PatternCache) TableName() string {
	return MainTableName
}

// GetPK returns the partition key
func (c *PatternCache) GetPK() string {
	return c.PK
}

// GetSK returns the sort key
func (c *PatternCache) GetSK() string {
	return c.SK
}

// UpdateKeys updates the GSI keys based on current field values
func (c *PatternCache) UpdateKeys() error {
	c.PK = fmt.Sprintf("PATTERN_CACHE#%s", c.PatternType)
	c.SK = fmt.Sprintf("COMPILED#%s", c.PatternID)

	// GSI1 - Cache management
	c.GSI1PK = "PATTERN_CACHE#ACTIVE"
	c.GSI1SK = fmt.Sprintf("%s#%s", c.UpdatedAt.Format(time.RFC3339), c.PatternID)

	// Set type marker
	c.Type = "PATTERN_CACHE"

	// Set TTL (24 hours default)
	if c.TTL == 0 {
		c.TTL = time.Now().Add(24 * time.Hour).Unix()
	}
	return nil
}

// PatternPerformanceMetric tracks detailed performance metrics for patterns
type PatternPerformanceMetric struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "PATTERN_METRICS#{pattern_id}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "TIME#{date}#{hour}"

	// GSI1 - Pattern performance queries
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK" json:"gsi1_pk,omitempty"` // Format: "METRICS#{pattern_type}#{date}"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK" json:"gsi1_sk,omitempty"` // Format: "{hour}#{pattern_id}"

	// GSI2 - Aggregated metrics queries
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsI2PK" json:"gsi2_pk,omitempty"` // "PATTERN_PERFORMANCE"
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsI2SK" json:"gsi2_sk,omitempty"` // "{date}#{pattern_type}#{pattern_id}"

	// Type marker
	Type string `dynamorm:"attr:type" json:"type"` // "PATTERN_METRIC"

	// Metric data
	PatternID   string `dynamorm:"attr:patternID" json:"pattern_id"`
	PatternType string `dynamorm:"attr:patternType" json:"pattern_type"`
	Date        string `dynamorm:"attr:date" json:"date"` // YYYY-MM-DD
	Hour        int    `dynamorm:"attr:hour" json:"hour"` // 0-23

	// Performance counters
	MatchAttempts     int64   `dynamorm:"attr:matchAttempts" json:"match_attempts"`         // Total match attempts
	SuccessfulMatches int64   `dynamorm:"attr:successfulMatches" json:"successful_matches"` // Actual matches
	FalsePositives    int64   `dynamorm:"attr:falsePositives" json:"false_positives"`       // Confirmed false positives
	TruePositives     int64   `dynamorm:"attr:truePositives" json:"true_positives"`         // Confirmed true positives
	AverageMatchTime  float64 `dynamorm:"attr:averageMatchTime" json:"average_match_time"`  // Average time per match in milliseconds
	MaxMatchTime      float64 `dynamorm:"attr:maxMatchTime" json:"max_match_time"`          // Maximum time for a single match
	MinMatchTime      float64 `dynamorm:"attr:minMatchTime" json:"min_match_time"`          // Minimum time for a single match
	TotalMatchTime    float64 `dynamorm:"attr:totalMatchTime" json:"total_match_time"`      // Total time spent matching
	MemoryUsage       int64   `dynamorm:"attr:memoryUsage" json:"memory_usage"`             // Memory usage in bytes
	CPUTime           float64 `dynamorm:"attr:cpuTime" json:"cpu_time"`                     // CPU time used in milliseconds

	// Quality metrics
	Precision float64 `dynamorm:"attr:precision" json:"precision"` // true_positives / ...
	Recall    float64 `dynamorm:"attr:recall" json:"recall"`       // true_positives / (true_positives + false_negatives)
	F1Score   float64 `dynamorm:"attr:f1Score" json:"f1_score"`    // 2 * (precision * recall) / (precision + recall)

	// Metadata
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`

	// TTL (30 days for detailed metrics)
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (PatternPerformanceMetric) TableName() string {
	return MainTableName
}

// GetPK returns the partition key
func (m *PatternPerformanceMetric) GetPK() string {
	return m.PK
}

// GetSK returns the sort key
func (m *PatternPerformanceMetric) GetSK() string {
	return m.SK
}

// UpdateKeys updates the GSI keys based on current field values
func (m *PatternPerformanceMetric) UpdateKeys() error {
	m.PK = fmt.Sprintf("PATTERN_METRICS#%s", m.PatternID)
	m.SK = fmt.Sprintf("TIME#%s#%02d", m.Date, m.Hour)

	// GSI1 - Pattern type and date queries
	m.GSI1PK = fmt.Sprintf("METRICS#%s#%s", m.PatternType, m.Date)
	m.GSI1SK = fmt.Sprintf("%02d#%s", m.Hour, m.PatternID)

	// GSI2 - Aggregated metrics
	m.GSI2PK = "PATTERN_PERFORMANCE"
	m.GSI2SK = fmt.Sprintf("%s#%s#%s", m.Date, m.PatternType, m.PatternID)

	// Set type marker
	m.Type = "PATTERN_METRIC"

	// Set TTL (30 days)
	if m.TTL == 0 {
		m.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	}
	return nil
}

// CalculateQualityMetrics calculates precision, recall, and F1 score
func (m *PatternPerformanceMetric) CalculateQualityMetrics() {
	if m.TruePositives+m.FalsePositives > 0 {
		m.Precision = float64(m.TruePositives) / float64(m.TruePositives+m.FalsePositives)
	}

	// Note: Recall calculation requires false negatives, which we don't track per hour
	// This would need to be calculated at a higher level with additional data

	if m.Precision > 0 && m.Recall > 0 {
		m.F1Score = 2 * (m.Precision * m.Recall) / (m.Precision + m.Recall)
	}
}

// PatternTestResult stores results from pattern testing and validation
type PatternTestResult struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "PATTERN_TEST#{pattern_id}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "TEST#{test_id}"

	// GSI1 - Test result queries by type
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK" json:"gsi1_pk,omitempty"` // Format: "PATTERN_TESTS#{test_type}"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK" json:"gsi1_sk,omitempty"` // Format: "{score}#{timestamp}#{test_id}"

	// Type marker
	Type string `dynamorm:"attr:type" json:"type"` // "PATTERN_TEST"

	// Test identification
	TestID      string `dynamorm:"attr:testID" json:"test_id"`
	PatternID   string `dynamorm:"attr:patternID" json:"pattern_id"`
	PatternType string `dynamorm:"attr:patternType" json:"pattern_type"`
	TestType    string `dynamorm:"attr:testType" json:"test_type"` // "validation", ...

	// Test configuration
	TestDescription string                 `dynamorm:"attr:testDescription" json:"test_description"`
	TestParameters  map[string]interface{} `dynamorm:"attr:testParameters" json:"test_parameters"`
	TestData        []string               `dynamorm:"attr:testData" json:"test_data,omitempty"` // Test inputs

	// Test results
	Passed          bool                   `dynamorm:"attr:passed" json:"passed"`
	Score           float64                `dynamorm:"attr:score" json:"score"`                  // 0.0-1.0
	ExecutionTime   float64                `dynamorm:"attr:executionTime" json:"execution_time"` // Time in milliseconds
	MemoryUsage     int64                  `dynamorm:"attr:memoryUsage" json:"memory_usage"`     // Memory usage in bytes
	Results         map[string]interface{} `dynamorm:"attr:results" json:"results"`              // Detailed test results
	ExpectedResults []string               `dynamorm:"attr:expectedResults" json:"expected_results,omitempty"`
	ActualResults   []string               `dynamorm:"attr:actualResults" json:"actual_results,omitempty"`
	Errors          []string               `dynamorm:"attr:errors" json:"errors,omitempty"`

	// Test metadata
	TestVersion string    `dynamorm:"attr:testVersion" json:"test_version"`
	RunBy       string    `dynamorm:"attr:runBy" json:"run_by"`
	RunAt       time.Time `dynamorm:"attr:runAt" json:"run_at"`
	Environment string    `dynamorm:"attr:environment" json:"environment"` // "development", "staging", "production"
	CreatedAt   time.Time `dynamorm:"attr:createdAt" json:"created_at"`

	// TTL (90 days for test results)
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (PatternTestResult) TableName() string {
	return MainTableName
}

// GetPK returns the partition key
func (t *PatternTestResult) GetPK() string {
	return t.PK
}

// GetSK returns the sort key
func (t *PatternTestResult) GetSK() string {
	return t.SK
}

// UpdateKeys updates the GSI keys based on current field values
func (t *PatternTestResult) UpdateKeys() error {
	t.PK = fmt.Sprintf("PATTERN_TEST#%s", t.PatternID)
	t.SK = fmt.Sprintf("TEST#%s", t.TestID)

	// GSI1 - Test results by type and score
	scoreStr := fmt.Sprintf("%06.3f", t.Score)
	t.GSI1PK = fmt.Sprintf("PATTERN_TESTS#%s", t.TestType)
	t.GSI1SK = fmt.Sprintf("%s#%s#%s", scoreStr, t.RunAt.Format(time.RFC3339), t.TestID)

	// Set type marker
	t.Type = "PATTERN_TEST"

	// Set TTL (90 days)
	if t.TTL == 0 {
		t.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()
	}
	return nil
}

// IsSecurityTest checks if this is a security-related test
func (t *PatternTestResult) IsSecurityTest() bool {
	return t.TestType == "security"
}

// GetSecurityScore returns the security score if this is a security test
func (t *PatternTestResult) GetSecurityScore() float64 {
	if t.IsSecurityTest() {
		return t.Score
	}
	return 0.0
}
