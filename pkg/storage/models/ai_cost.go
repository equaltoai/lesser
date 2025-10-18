package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// AICost represents AI/ML operation costs with detailed Bedrock usage tracking
type AICost struct {
	// Primary keys - AI cost uses AI_COST#{operation_id} pattern
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// GSI1 for time-based queries - AI_COSTS#{date}, TS#{timestamp}#{operation_id}
	GSI1PK string `dynamorm:"index:time-index,pk" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:time-index,sk" json:"gsi1_sk"`

	// GSI2 for operation type queries - AI_TYPE#{operation_type}, MODEL#{model}#{timestamp}
	GSI2PK string `dynamorm:"index:operation-type-index,pk" json:"gsi2_pk"`
	GSI2SK string `dynamorm:"index:operation-type-index,sk" json:"gsi2_sk"`

	// GSI3 for cost analysis - AI_COST_RANGE#{cost_tier}, COST#{cost_microcents}#{timestamp}
	GSI3PK string `dynamorm:"index:cost-analysis-index,pk" json:"gsi3_pk"`
	GSI3SK string `dynamorm:"index:cost-analysis-index,sk" json:"gsi3_sk"`

	// Core operation metadata
	OperationID   string `json:"operation_id"`
	OperationType string `json:"operation_type"` // sentiment_analysis, content_moderation, text_summarization
	RequestID     string `json:"request_id"`     // Associated request ID for tracing
	UserID        string `json:"user_id,omitempty"`
	ActorID       string `json:"actor_id,omitempty"`
	ObjectID      string `json:"object_id,omitempty"` // Note ID, post ID, etc.

	// AI Model details
	ModelFamily   string `json:"model_family"`   // claude, titan, jurassic
	ModelName     string `json:"model_name"`     // claude-3-haiku, claude-3-sonnet, claude-3-opus
	ModelVersion  string `json:"model_version"`  // 20240307, etc.
	ModelRegion   string `json:"model_region"`   // us-east-1, us-west-2
	ModelEndpoint string `json:"model_endpoint"` // Bedrock endpoint used

	// Input/Output token tracking
	InputTokens        int64   `json:"input_tokens"`
	OutputTokens       int64   `json:"output_tokens"`
	TotalTokens        int64   `json:"total_tokens"`
	InputCharacters    int64   `json:"input_characters"`
	OutputCharacters   int64   `json:"output_characters"`
	TokensPerSecond    float64 `json:"tokens_per_second"`
	CharactersPerToken float64 `json:"characters_per_token"`

	// Content complexity analysis
	ComplexityScore    float64  `json:"complexity_score"`   // 0.0-1.0 based on content analysis
	ComplexityFactors  []string `json:"complexity_factors"` // factors that contributed to complexity
	LanguageDetected   string   `json:"language_detected"`
	ContentLength      int64    `json:"content_length"`
	ContentType        string   `json:"content_type"` // text, markdown, json, etc.
	SentimentPolarity  float64  `json:"sentiment_polarity,omitempty"`
	SentimentMagnitude float64  `json:"sentiment_magnitude,omitempty"`

	// Cost breakdown (all in microcents)
	InputTokenCost        int64 `json:"input_token_cost"`        // Cost for input tokens
	OutputTokenCost       int64 `json:"output_token_cost"`       // Cost for output tokens
	ModelInferenceCost    int64 `json:"model_inference_cost"`    // Base inference cost
	ComplexityPenaltyCost int64 `json:"complexity_penalty_cost"` // Additional cost for complex operations
	TotalCostMicroCents   int64 `json:"total_cost_micro_cents"`

	// Performance metrics
	RequestLatencyMs    int64 `json:"request_latency_ms"`     // Total request latency
	ModelLatencyMs      int64 `json:"model_latency_ms"`       // Model processing time
	QueueWaitTimeMs     int64 `json:"queue_wait_time_ms"`     // Time waiting in queue
	TokenGenerationMs   int64 `json:"token_generation_ms"`    // Token generation time
	FirstTokenLatencyMs int64 `json:"first_token_latency_ms"` // Time to first token
	StreamingEnabled    bool  `json:"streaming_enabled"`

	// Error tracking
	Success      bool   `json:"success"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	RetryCount   int    `json:"retry_count"`
	RetryCost    int64  `json:"retry_cost"` // Additional cost from retries

	// Context and configuration
	Temperature    float64                `json:"temperature,omitempty"`
	MaxTokens      int64                  `json:"max_tokens,omitempty"`
	TopP           float64                `json:"top_p,omitempty"`
	ModelConfig    map[string]interface{} `json:"model_config,omitempty"`
	SystemPrompt   string                 `json:"system_prompt,omitempty"` // Truncated for storage
	UserPrompt     string                 `json:"user_prompt,omitempty"`   // Truncated for storage
	ResponseFormat string                 `json:"response_format,omitempty"`

	// Business context
	OperationContext map[string]string `json:"operation_context,omitempty"` // Additional context
	BillingPeriod    string            `json:"billing_period"`              // YYYY-MM format
	CostTier         string            `json:"cost_tier"`                   // low, medium, high, premium
	Priority         string            `json:"priority"`                    // low, normal, high, urgent

	// Efficiency metrics
	CostPerInputToken     float64 `json:"cost_per_input_token"`
	CostPerOutputToken    float64 `json:"cost_per_output_token"`
	CostPerCharacter      float64 `json:"cost_per_character"`
	EfficiencyScore       float64 `json:"efficiency_score"`        // Quality/cost ratio
	QualityScore          float64 `json:"quality_score"`           // Output quality assessment
	RelevanceScore        float64 `json:"relevance_score"`         // Output relevance to prompt
	ComprehensivenesScore float64 `json:"comprehensiveness_score"` // How complete the output is

	// Timestamps
	Timestamp       time.Time `json:"timestamp"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	ProcessingStart time.Time `json:"processing_start"`
	ProcessingEnd   time.Time `json:"processing_end"`

	// TTL for automatic cleanup (90 days for AI cost records)
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the primary keys for the AICost model
func (a *AICost) UpdateKeys() error {
	timestampStr := a.Timestamp.Format(common.CompactTimeFormat)
	dateStr := a.Timestamp.Format(common.CompactDateFormat)

	a.PK = fmt.Sprintf("AI_COST#%s", a.OperationID)
	a.SK = SKMetadata

	// GSI1 for time-based queries
	a.GSI1PK = fmt.Sprintf("AI_COSTS#%s", dateStr)
	a.GSI1SK = fmt.Sprintf("TS#%s#%s", timestampStr, a.OperationID)

	// GSI2 for operation type queries
	a.GSI2PK = fmt.Sprintf("AI_TYPE#%s", a.OperationType)
	a.GSI2SK = fmt.Sprintf("MODEL#%s#%s", a.ModelName, timestampStr)

	// GSI3 for cost analysis - tier based on cost
	a.GSI3PK = fmt.Sprintf("AI_COST_RANGE#%s", a.CostTier)
	a.GSI3SK = fmt.Sprintf("COST#%012d#%s", a.TotalCostMicroCents, timestampStr)

	return nil
}

// BeforeCreate is called before creating the record
func (a *AICost) BeforeCreate() error {
	now := time.Now()
	if a.Timestamp.IsZero() {
		a.Timestamp = now
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	if a.ProcessingStart.IsZero() {
		a.ProcessingStart = now
	}
	a.UpdatedAt = now

	// Set billing period
	if err := common.ValidateRequiredParam("a.BillingPeriod", a.BillingPeriod); err != nil {
		a.BillingPeriod = a.Timestamp.Format("2006-01")
	}

	// Initialize slices and maps
	if a.ComplexityFactors == nil {
		a.ComplexityFactors = make([]string, 0)
	}
	if a.ModelConfig == nil {
		a.ModelConfig = make(map[string]interface{})
	}
	if a.OperationContext == nil {
		a.OperationContext = make(map[string]string)
	}

	// Calculate derived fields
	a.TotalTokens = a.InputTokens + a.OutputTokens
	if a.InputTokens > 0 && a.InputCharacters > 0 {
		a.CharactersPerToken = float64(a.InputCharacters) / float64(a.InputTokens)
	}

	// Calculate cost tier based on total cost
	a.determineCostTier()

	// Calculate efficiency metrics
	a.calculateEfficiencyMetrics()

	// Set TTL to 90 days from creation
	a.TTL = now.AddDate(0, 0, 90).Unix()

	if err := a.UpdateKeys(); err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToUpdateKeys, err)
	}
	a.CalculateTotalCost()
	return nil
}

// BeforeUpdate is called before updating the record
func (a *AICost) BeforeUpdate() error {
	a.UpdatedAt = time.Now()
	if !a.ProcessingEnd.IsZero() && a.ProcessingStart.IsZero() {
		a.ProcessingEnd = time.Now()
	}

	a.calculateEfficiencyMetrics()
	if err := a.UpdateKeys(); err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToUpdateKeys, err)
	}
	a.CalculateTotalCost()
	return nil
}

// CalculateTotalCost calculates the total cost from all components
func (a *AICost) CalculateTotalCost() {
	a.TotalCostMicroCents = a.InputTokenCost +
		a.OutputTokenCost +
		a.ModelInferenceCost +
		a.ComplexityPenaltyCost +
		a.RetryCost

	// Calculate per-unit costs
	if a.InputTokens > 0 {
		a.CostPerInputToken = float64(a.InputTokenCost) / float64(a.InputTokens) / 1_000_000.0
	}
	if a.OutputTokens > 0 {
		a.CostPerOutputToken = float64(a.OutputTokenCost) / float64(a.OutputTokens) / 1_000_000.0
	}
	if a.InputCharacters > 0 {
		a.CostPerCharacter = float64(a.TotalCostMicroCents) / float64(a.InputCharacters) / 1_000_000.0
	}

	a.determineCostTier()
}

// determineCostTier sets the cost tier based on total cost
func (a *AICost) determineCostTier() {
	costDollars := float64(a.TotalCostMicroCents) / 1_000_000.0

	switch {
	case costDollars >= 1.0:
		a.CostTier = CostTierPremium
	case costDollars >= 0.1:
		a.CostTier = CostTierHigh
	case costDollars >= 0.01:
		a.CostTier = string(AdvancedSeverityMedium)
	default:
		a.CostTier = CostTierLow
	}
}

// calculateEfficiencyMetrics calculates efficiency and quality metrics
func (a *AICost) calculateEfficiencyMetrics() {
	// Efficiency Score = (QualityScore + RelevanceScore) / (CostPerToken * ComplexityFactor)
	if a.CostPerOutputToken > 0 && a.QualityScore > 0 {
		complexityFactor := 1.0 + a.ComplexityScore
		a.EfficiencyScore = (a.QualityScore + a.RelevanceScore) / (a.CostPerOutputToken * complexityFactor)
	}

	// Calculate tokens per second if we have timing data
	if !a.ProcessingStart.IsZero() && !a.ProcessingEnd.IsZero() && a.OutputTokens > 0 {
		duration := a.ProcessingEnd.Sub(a.ProcessingStart).Seconds()
		if duration > 0 {
			a.TokensPerSecond = float64(a.OutputTokens) / duration
		}
	}
}

// GetTotalCostDollars returns the total cost in dollars
func (a *AICost) GetTotalCostDollars() float64 {
	return float64(a.TotalCostMicroCents) / 1_000_000.0
}

// AddComplexityFactor adds a factor that contributed to operation complexity
func (a *AICost) AddComplexityFactor(factor string) {
	if a.ComplexityFactors == nil {
		a.ComplexityFactors = make([]string, 0)
	}

	// Avoid duplicates
	for _, existing := range a.ComplexityFactors {
		if existing == factor {
			return
		}
	}

	a.ComplexityFactors = append(a.ComplexityFactors, factor)
}

// SetModelPricing sets the cost based on model-specific pricing
func (a *AICost) SetModelPricing() {
	// Bedrock pricing as of 2024 (in microcents per token)
	var inputCostPerToken, outputCostPerToken int64

	switch a.ModelName {
	case "claude-3-haiku":
		inputCostPerToken = 25   // $0.00025 per 1K tokens = $0.00000025 per token = 25 microcents
		outputCostPerToken = 125 // $0.00125 per 1K tokens
	case "claude-3-sonnet":
		inputCostPerToken = 300   // $0.003 per 1K tokens
		outputCostPerToken = 1500 // $0.015 per 1K tokens
	case "claude-3-opus":
		inputCostPerToken = 1500  // $0.015 per 1K tokens
		outputCostPerToken = 7500 // $0.075 per 1K tokens
	case "titan-text-express":
		inputCostPerToken = 13  // $0.00013 per 1K tokens
		outputCostPerToken = 17 // $0.00017 per 1K tokens
	case "titan-text-lite":
		inputCostPerToken = 15  // $0.00015 per 1K tokens
		outputCostPerToken = 20 // $0.0002 per 1K tokens
	default:
		// Default to Haiku pricing for unknown models
		inputCostPerToken = 25
		outputCostPerToken = 125
	}

	a.InputTokenCost = a.InputTokens * inputCostPerToken
	a.OutputTokenCost = a.OutputTokens * outputCostPerToken

	// Base inference cost (minimum per request)
	a.ModelInferenceCost = 100 // $0.0001 minimum

	// Complexity penalty (additional cost for complex operations)
	complexityMultiplier := 1.0 + a.ComplexityScore
	a.ComplexityPenaltyCost = int64(float64(a.InputTokenCost+a.OutputTokenCost) * (complexityMultiplier - 1.0))
}

// GetOperationSummary returns a summary of the AI operation for logging
func (a *AICost) GetOperationSummary() map[string]interface{} {
	return map[string]interface{}{
		"operation_id":       a.OperationID,
		"operation_type":     a.OperationType,
		"model_name":         a.ModelName,
		"input_tokens":       a.InputTokens,
		"output_tokens":      a.OutputTokens,
		"total_cost_dollars": a.GetTotalCostDollars(),
		"cost_tier":          a.CostTier,
		"complexity_score":   a.ComplexityScore,
		"efficiency_score":   a.EfficiencyScore,
		"processing_time_ms": a.RequestLatencyMs,
		"success":            a.Success,
		"timestamp":          a.Timestamp,
	}
}

// GetPerformanceMetrics returns performance-related metrics
func (a *AICost) GetPerformanceMetrics() map[string]interface{} {
	return map[string]interface{}{
		"tokens_per_second":      a.TokensPerSecond,
		"cost_per_input_token":   a.CostPerInputToken,
		"cost_per_output_token":  a.CostPerOutputToken,
		"cost_per_character":     a.CostPerCharacter,
		"efficiency_score":       a.EfficiencyScore,
		"quality_score":          a.QualityScore,
		"request_latency_ms":     a.RequestLatencyMs,
		"model_latency_ms":       a.ModelLatencyMs,
		"first_token_latency_ms": a.FirstTokenLatencyMs,
		"complexity_score":       a.ComplexityScore,
		"characters_per_token":   a.CharactersPerToken,
	}
}

// GetPK returns the partition key
func (a *AICost) GetPK() string {
	return a.PK
}

// GetSK returns the sort key
func (a *AICost) GetSK() string {
	return a.SK
}

// TableName returns the DynamoDB table name
func (a *AICost) TableName() string {
	return "" // Will be set by the repository
}

// AIAggregatedCost represents aggregated AI cost metrics for efficient querying
type AIAggregatedCost struct {
	// Primary keys - aggregated costs use AI_AGG#{period}#{operation_type} pattern
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// GSI1 for time-based aggregation queries
	GSI1PK string `dynamorm:"index:time-index,pk" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:time-index,sk" json:"gsi1_sk"`

	// Aggregation metadata
	Period        string    `json:"period"` // hour, day, week, month
	PeriodStart   time.Time `json:"period_start"`
	PeriodEnd     time.Time `json:"period_end"`
	OperationType string    `json:"operation_type"`
	ModelName     string    `json:"model_name"`

	// Aggregated metrics
	TotalOperations      int64   `json:"total_operations"`
	SuccessfulOperations int64   `json:"successful_operations"`
	FailedOperations     int64   `json:"failed_operations"`
	SuccessRate          float64 `json:"success_rate"`

	// Token aggregates
	TotalInputTokens  int64   `json:"total_input_tokens"`
	TotalOutputTokens int64   `json:"total_output_tokens"`
	AvgInputTokens    float64 `json:"avg_input_tokens"`
	AvgOutputTokens   float64 `json:"avg_output_tokens"`

	// Cost aggregates
	TotalCostMicroCents  int64 `json:"total_cost_micro_cents"`
	AvgCostMicroCents    int64 `json:"avg_cost_micro_cents"`
	MinCostMicroCents    int64 `json:"min_cost_micro_cents"`
	MaxCostMicroCents    int64 `json:"max_cost_micro_cents"`
	MedianCostMicroCents int64 `json:"median_cost_micro_cents"`

	// Performance aggregates
	AvgLatencyMs       float64 `json:"avg_latency_ms"`
	P95LatencyMs       float64 `json:"p95_latency_ms"`
	P99LatencyMs       float64 `json:"p99_latency_ms"`
	AvgTokensPerSecond float64 `json:"avg_tokens_per_second"`
	AvgComplexityScore float64 `json:"avg_complexity_score"`
	AvgEfficiencyScore float64 `json:"avg_efficiency_score"`

	// Cost efficiency metrics
	CostPerInputToken  float64 `json:"cost_per_input_token"`
	CostPerOutputToken float64 `json:"cost_per_output_token"`
	TotalCostDollars   float64 `json:"total_cost_dollars"`

	// Quality metrics
	AvgQualityScore           float64 `json:"avg_quality_score"`
	AvgRelevanceScore         float64 `json:"avg_relevance_score"`
	AvgComprehensivenessScore float64 `json:"avg_comprehensiveness_score"`

	// Most frequent complexity factors
	TopComplexityFactors []string `json:"top_complexity_factors"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// TTL for automatic cleanup (1 year for aggregated records)
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the primary keys for the AIAggregatedCost model
func (a *AIAggregatedCost) UpdateKeys() error {
	periodStr := a.PeriodStart.Format(common.CompactDateFormat)
	if a.Period == "hour" {
		periodStr = a.PeriodStart.Format(common.CompactTimeFormat)[:13] // YYYYMMDDHH
	}

	a.PK = fmt.Sprintf("AI_AGG#%s#%s", a.Period, periodStr)
	a.SK = fmt.Sprintf("%s#%s", a.OperationType, a.ModelName)

	a.GSI1PK = fmt.Sprintf("AI_AGG_TIME#%s", a.Period)
	a.GSI1SK = fmt.Sprintf("%s#%s#%s", periodStr, a.OperationType, a.ModelName)

	return nil
}

// BeforeCreate is called before creating the aggregated record
func (a *AIAggregatedCost) BeforeCreate() error {
	now := time.Now()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now

	// Calculate derived metrics
	if a.TotalOperations > 0 {
		a.SuccessRate = float64(a.SuccessfulOperations) / float64(a.TotalOperations)
		a.AvgInputTokens = float64(a.TotalInputTokens) / float64(a.TotalOperations)
		a.AvgOutputTokens = float64(a.TotalOutputTokens) / float64(a.TotalOperations)
		a.AvgCostMicroCents = a.TotalCostMicroCents / a.TotalOperations
	}

	a.TotalCostDollars = float64(a.TotalCostMicroCents) / 1_000_000.0

	if a.TotalInputTokens > 0 {
		a.CostPerInputToken = float64(a.TotalCostMicroCents) / float64(a.TotalInputTokens) / 1_000_000.0
	}
	if a.TotalOutputTokens > 0 {
		a.CostPerOutputToken = float64(a.TotalCostMicroCents) / float64(a.TotalOutputTokens) / 1_000_000.0
	}

	// Initialize slices
	if a.TopComplexityFactors == nil {
		a.TopComplexityFactors = make([]string, 0)
	}

	// Set TTL to 1 year from creation
	a.TTL = now.AddDate(1, 0, 0).Unix()

	if err := a.UpdateKeys(); err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToUpdateKeys, err)
	}
	return nil
}

// BeforeUpdate is called before updating the aggregated record
func (a *AIAggregatedCost) BeforeUpdate() error {
	a.UpdatedAt = time.Now()
	if err := a.UpdateKeys(); err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToUpdateKeys, err)
	}
	return nil
}

// GetPK returns the partition key
func (a *AIAggregatedCost) GetPK() string {
	return a.PK
}

// GetSK returns the sort key
func (a *AIAggregatedCost) GetSK() string {
	return a.SK
}

// TableName returns the DynamoDB table name
func (a *AIAggregatedCost) TableName() string {
	return "" // Will be set by the repository
}
