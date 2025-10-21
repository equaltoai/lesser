package security

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

const (
	riskLevelCritical = "critical"
	riskLevelHigh     = "high"
	riskLevelMedium   = "medium"
	riskLevelLow      = "low"
)

// MLRiskScorer implements ML-based risk scoring
type MLRiskScorer struct {
	model       RiskModel
	riskFactors []RiskFactor
	feedback    []RiskFeedback
	config      RiskScoringConfig
	mu          sync.RWMutex
}

// RiskScoringConfig configuration for risk scoring
type RiskScoringConfig struct {
	RiskFactorWeights  map[string]float64  `json:"risk_factor_weights"`
	ModelType          string              `json:"model_type"`
	ContextualFactors  []string            `json:"contextual_factors"`
	TemporalFactors    []string            `json:"temporal_factors"`
	BehavioralFactors  []string            `json:"behavioral_factors"`
	ThresholdConfig    RiskThresholdConfig `json:"threshold_config"`
	BaselineUpdateFreq time.Duration       `json:"baseline_update_freq"`
	LearningRate       float64             `json:"learning_rate"`
	FeedbackWeight     float64             `json:"feedback_weight"`
	Enabled            bool                `json:"enabled"`
	AdaptiveLearning   bool                `json:"adaptive_learning"`
}

// RiskThresholdConfig defines risk level thresholds
type RiskThresholdConfig struct {
	CriticalThreshold float64 `json:"critical_threshold"`
	HighThreshold     float64 `json:"high_threshold"`
	MediumThreshold   float64 `json:"medium_threshold"`
	LowThreshold      float64 `json:"low_threshold"`
}

// RiskModel interface for different risk models
type RiskModel interface {
	Predict(features []float64) (float64, error)
	Train(trainingData []TrainingExample) error
	Update(feedback []RiskFeedback) error
	GetFeatureImportance() map[string]float64
	GetModelMetrics() *ModelMetrics
}

// TrainingExample represents a training example for the risk model
type TrainingExample struct {
	Metadata map[string]any `json:"metadata"`
	Features []float64      `json:"features"`
	Label    float64        `json:"label"`
	Weight   float64        `json:"weight"`
}

// ModelMetrics represents metrics for the risk model
type ModelMetrics struct {
	LastUpdated       time.Time          `json:"last_updated"`
	FeatureImportance map[string]float64 `json:"feature_importance"`
	Accuracy          float64            `json:"accuracy"`
	Precision         float64            `json:"precision"`
	Recall            float64            `json:"recall"`
	F1Score           float64            `json:"f1_score"`
	AUC               float64            `json:"auc"`
	RMSE              float64            `json:"rmse"`
	TrainingExamples  int                `json:"training_examples"`
}

// RiskBaseline represents baseline risk metrics
type RiskBaseline struct {
	UpdatedAt        time.Time          `json:"updated_at"`
	RiskDistribution map[string]float64 `json:"risk_distribution"`
	FactorBaselines  map[string]float64 `json:"factor_baselines"`
	TemporalPatterns map[string]float64 `json:"temporal_patterns"`
	Metadata         map[string]any     `json:"metadata"`
	AverageRisk      float64            `json:"average_risk"`
	SampleSize       int                `json:"sample_size"`
}

// ContextualRiskFactor represents a contextual risk factor
type ContextualRiskFactor struct {
	Conditions map[string]any `json:"conditions"`
	Context    string         `json:"context"`
	RiskFactor
	Multiplier float64 `json:"multiplier"`
	Temporal   bool    `json:"temporal"`
	Behavioral bool    `json:"behavioral"`
}

// RiskFeatureExtractor extracts features from audit events
type RiskFeatureExtractor struct {
}

// FeatureExtractionConfig configuration for feature extraction
type FeatureExtractionConfig struct {
	FeatureWeights      map[string]float64 `json:"feature_weights"`
	Metadata            map[string]any     `json:"metadata"`
	NormalizationMethod string             `json:"normalization_method"`
	EnabledFeatures     []string           `json:"enabled_features"`
	TemporalWindow      time.Duration      `json:"temporal_window"`
	BehavioralWindow    time.Duration      `json:"behavioral_window"`
	ContextualDepth     int                `json:"contextual_depth"`
}

// FeatureExtractor interface for extracting specific features
type FeatureExtractor interface {
	Extract(event *AuditEvent, context *RiskContext) (float64, error)
	GetName() string
	GetDescription() string
	GetWeight() float64
}

// RiskContext provides context for risk assessment
type RiskContext struct {
	TimeOfDay     time.Time           `json:"time_of_day"`
	UserProfile   *UserRiskProfile    `json:"user_profile"`
	TenantProfile *TenantRiskProfile  `json:"tenant_profile"`
	ThreatIntel   *ThreatIntelligence `json:"threat_intel"`
	Metadata      map[string]any      `json:"metadata"`
	UserHistory   []*AuditEvent       `json:"user_history"`
	TenantHistory []*AuditEvent       `json:"tenant_history"`
	RecentEvents  []*AuditEvent       `json:"recent_events"`
	DayOfWeek     time.Weekday        `json:"day_of_week"`
}

// UserRiskProfile represents a user's risk profile
type UserRiskProfile struct {
	LastUpdated      time.Time          `json:"last_updated"`
	BehaviorPatterns map[string]float64 `json:"behavior_patterns"`
	AccessPatterns   map[string]float64 `json:"access_patterns"`
	Metadata         map[string]any     `json:"metadata"`
	UserID           string             `json:"user_id"`
	RiskTrend        string             `json:"risk_trend"`
	AnomalyHistory   []AnomalyRecord    `json:"anomaly_history"`
	BaselineRisk     float64            `json:"baseline_risk"`
}

// TenantRiskProfile represents a tenant's risk profile
type TenantRiskProfile struct {
	LastUpdated     time.Time          `json:"last_updated"`
	SecurityPosture map[string]float64 `json:"security_posture"`
	Metadata        map[string]any     `json:"metadata"`
	TenantID        string             `json:"tenant_id"`
	RiskTrend       string             `json:"risk_trend"`
	IncidentHistory []IncidentRecord   `json:"incident_history"`
	BaselineRisk    float64            `json:"baseline_risk"`
	ComplianceScore float64            `json:"compliance_score"`
}

// AnomalyRecord represents an anomaly record
type AnomalyRecord struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Description string    `json:"description"`
	Score       float64   `json:"score"`
	Resolved    bool      `json:"resolved"`
}

// IncidentRecord represents an incident record
type IncidentRecord struct {
	Timestamp  time.Time `json:"timestamp"`
	Type       string    `json:"type"`
	Severity   string    `json:"severity"`
	Impact     string    `json:"impact"`
	Resolution string    `json:"resolution"`
	Resolved   bool      `json:"resolved"`
}

// ThreatIntelligence represents threat intelligence data
type ThreatIntelligence struct {
	LastUpdated     time.Time          `json:"last_updated"`
	GeographicRisks map[string]float64 `json:"geographic_risks"`
	Metadata        map[string]any     `json:"metadata"`
	ThreatLevel     string             `json:"threat_level"`
	ActiveThreats   []ThreatIndicator  `json:"active_threats"`
	RiskFactors     []ThreatRiskFactor `json:"risk_factors"`
	IndustryThreats []string           `json:"industry_threats"`
}

// ThreatIndicator represents a threat indicator
type ThreatIndicator struct {
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	Type        string    `json:"type"`
	Value       string    `json:"value"`
	Severity    string    `json:"severity"`
	Source      string    `json:"source"`
	Description string    `json:"description"`
	Confidence  float64   `json:"confidence"`
}

// ThreatRiskFactor represents a threat-based risk factor
type ThreatRiskFactor struct {
	ThreatType string `json:"threat_type"`
	Mitigation string `json:"mitigation"`
	RiskFactor
	Prevalence    float64 `json:"prevalence"`
	Effectiveness float64 `json:"effectiveness"`
}

// NewMLRiskScorer creates a new ML-based risk scorer
func NewMLRiskScorer(config RiskScoringConfig) *MLRiskScorer {
	return &MLRiskScorer{
		config:      config,
		riskFactors: getDefaultRiskFactors(),
		feedback:    make([]RiskFeedback, 0),
	}
}

// SetModel sets the risk model
func (mrs *MLRiskScorer) SetModel(model RiskModel) {
	mrs.mu.Lock()
	defer mrs.mu.Unlock()
	mrs.model = model
}

// CalculateRiskScore calculates risk score for an audit event
func (mrs *MLRiskScorer) CalculateRiskScore(ctx context.Context, event *AuditEvent) (*RiskScore, error) {
	mrs.mu.RLock()
	defer mrs.mu.RUnlock()

	if !mrs.config.Enabled {
		return nil, fmt.Errorf("risk scoring not enabled")
	}

	// Extract features from the event
	features, riskFactors, err := mrs.extractFeatures(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("failed to extract features: %w", err)
	}

	// Calculate base risk score
	var baseScore float64
	if mrs.model != nil {
		baseScore, err = mrs.model.Predict(features)
		if err != nil {
			return nil, fmt.Errorf("model prediction failed: %w", err)
		}
	} else {
		// Fallback to weighted sum if no ML model
		baseScore = mrs.calculateWeightedScore(riskFactors)
	}

	// Apply contextual adjustments
	contextualScore := mrs.applyContextualAdjustments(ctx, event, baseScore)

	// Apply temporal adjustments
	temporalScore := mrs.applyTemporalAdjustments(ctx, event, contextualScore)

	// Apply behavioral adjustments
	finalScore := mrs.applyBehavioralAdjustments(ctx, event, temporalScore)

	// Normalize score to 0-100 range
	normalizedScore := math.Max(0, math.Min(100, finalScore))

	// Determine risk level
	riskLevel := mrs.determineRiskLevel(normalizedScore)

	// Calculate confidence
	confidence := mrs.calculateConfidence(features, riskFactors)

	// Generate recommendations
	recommendations := mrs.generateRecommendations(riskLevel, riskFactors)

	riskScore := &RiskScore{
		Score:           normalizedScore,
		Level:           riskLevel,
		Confidence:      confidence,
		Factors:         riskFactors,
		Recommendations: recommendations,
		Timestamp:       time.Now(),
		Metadata: map[string]any{
			"base_score":       baseScore,
			"contextual_score": contextualScore,
			"temporal_score":   temporalScore,
			"model_used":       mrs.model != nil,
			"feature_count":    len(features),
		},
	}

	return riskScore, nil
}

// CalculateAggregateRisk calculates aggregate risk for multiple events
func (mrs *MLRiskScorer) CalculateAggregateRisk(ctx context.Context, events []*AuditEvent) (*AggregateRiskScore, error) {
	mrs.mu.RLock()
	defer mrs.mu.RUnlock()

	if len(events) == 0 {
		return nil, fmt.Errorf("no events provided")
	}

	var totalScore float64
	var riskDistribution = make(map[string]int)
	var allRiskFactors []RiskFactor
	var recommendations []string

	// Calculate individual risk scores
	for _, event := range events {
		riskScore, err := mrs.CalculateRiskScore(ctx, event)
		if err != nil {
			continue // Skip events that can't be scored
		}

		totalScore += riskScore.Score
		riskDistribution[riskScore.Level]++
		allRiskFactors = append(allRiskFactors, riskScore.Factors...)
		recommendations = append(recommendations, riskScore.Recommendations...)
	}

	// Calculate overall score
	overallScore := totalScore / float64(len(events))

	// Determine overall risk level
	overallLevel := mrs.determineRiskLevel(overallScore)

	// Get top risk factors
	topRiskFactors := mrs.getTopRiskFactors(allRiskFactors, 10)

	// Determine trend direction
	trendDirection := mrs.calculateTrendDirection(events)

	// Deduplicate recommendations
	uniqueRecommendations := mrs.deduplicateRecommendations(recommendations)

	aggregateRisk := &AggregateRiskScore{
		OverallScore:     overallScore,
		Level:            overallLevel,
		EventCount:       len(events),
		TimeRange:        mrs.getTimeRange(events),
		RiskDistribution: riskDistribution,
		TopRiskFactors:   topRiskFactors,
		TrendDirection:   trendDirection,
		Recommendations:  uniqueRecommendations,
		Metadata: map[string]any{
			"total_score":   totalScore,
			"scored_events": len(events),
			"analysis_time": time.Now(),
		},
	}

	return aggregateRisk, nil
}

// UpdateRiskModel updates the risk model with feedback
func (mrs *MLRiskScorer) UpdateRiskModel(_ context.Context, feedback []*RiskFeedback) error {
	mrs.mu.Lock()
	defer mrs.mu.Unlock()

	if !mrs.config.AdaptiveLearning {
		return fmt.Errorf("adaptive learning not enabled")
	}

	// Store feedback - convert from []*RiskFeedback to []RiskFeedback
	for _, f := range feedback {
		if f != nil {
			mrs.feedback = append(mrs.feedback, *f)
		}
	}

	// Update model if available - convert to []RiskFeedback
	if mrs.model != nil {
		var feedbackSlice []RiskFeedback
		for _, f := range feedback {
			if f != nil {
				feedbackSlice = append(feedbackSlice, *f)
			}
		}
		return mrs.model.Update(feedbackSlice)
	}

	return nil
}

// GetRiskFactors returns the current risk factors
func (mrs *MLRiskScorer) GetRiskFactors() []RiskFactor {
	mrs.mu.RLock()
	defer mrs.mu.RUnlock()

	factors := make([]RiskFactor, len(mrs.riskFactors))
	copy(factors, mrs.riskFactors)
	return factors
}

// extractFeatures extracts features from an audit event
func (mrs *MLRiskScorer) extractFeatures(ctx context.Context, event *AuditEvent) ([]float64, []RiskFactor, error) {
	var features []float64
	var riskFactors []RiskFactor

	// Extract basic features
	features = append(features, mrs.extractBasicFeatures(event)...)

	// Extract temporal features
	temporalFeatures, temporalFactors := mrs.extractTemporalFeatures(event)
	features = append(features, temporalFeatures...)
	riskFactors = append(riskFactors, temporalFactors...)

	// Extract behavioral features
	behavioralFeatures, behavioralFactors := mrs.extractBehavioralFeatures(ctx, event)
	features = append(features, behavioralFeatures...)
	riskFactors = append(riskFactors, behavioralFactors...)

	// Extract contextual features
	contextualFeatures, contextualFactors := mrs.extractContextualFeatures(ctx, event)
	features = append(features, contextualFeatures...)
	riskFactors = append(riskFactors, contextualFactors...)

	return features, riskFactors, nil
}

// extractBasicFeatures extracts basic features from an event
func (mrs *MLRiskScorer) extractBasicFeatures(event *AuditEvent) []float64 {
	var features []float64

	// Event type risk
	eventTypeRisk := mrs.getEventTypeRisk(event.EventType)
	features = append(features, eventTypeRisk)

	// Action risk
	actionRisk := mrs.getActionRisk(event.Action)
	features = append(features, actionRisk)

	// Result risk (failure = higher risk)
	resultRisk := 0.0
	if event.Result == "failure" || event.Result == "error" {
		resultRisk = 1.0
	}
	features = append(features, resultRisk)

	// Severity risk
	severityRisk := mrs.getSeverityRisk(event.Severity)
	features = append(features, severityRisk)

	// Duration risk (very short or very long = higher risk)
	durationRisk := mrs.getDurationRisk(event.Duration)
	features = append(features, durationRisk)

	return features
}

// extractTemporalFeatures extracts temporal features
func (mrs *MLRiskScorer) extractTemporalFeatures(event *AuditEvent) ([]float64, []RiskFactor) {
	var features []float64
	var factors []RiskFactor

	// Time of day risk
	hour := event.Timestamp.Hour()
	timeRisk := mrs.getTimeOfDayRisk(hour)
	features = append(features, timeRisk)

	if timeRisk > 0.5 {
		factors = append(factors, RiskFactor{
			ID:          "time_of_day",
			Name:        "Off-hours Access",
			Category:    "temporal",
			Weight:      0.3,
			Value:       timeRisk,
			Impact:      timeRisk * 0.3,
			Description: fmt.Sprintf("Access at %d:00 is outside normal business hours", hour),
			Mitigation:  "Review off-hours access policies",
		})
	}

	// Day of week risk
	dayRisk := mrs.getDayOfWeekRisk(event.Timestamp.Weekday())
	features = append(features, dayRisk)

	if dayRisk > 0.5 {
		factors = append(factors, RiskFactor{
			ID:          "day_of_week",
			Name:        "Weekend Access",
			Category:    "temporal",
			Weight:      0.2,
			Value:       dayRisk,
			Impact:      dayRisk * 0.2,
			Description: "Access during weekend",
			Mitigation:  "Review weekend access policies",
		})
	}

	return features, factors
}

// extractBehavioralFeatures extracts behavioral features
func (mrs *MLRiskScorer) extractBehavioralFeatures(ctx context.Context, event *AuditEvent) ([]float64, []RiskFactor) {
	var features []float64
	var factors []RiskFactor

	// Frequency risk (too frequent = higher risk)
	frequencyRisk := mrs.getFrequencyRisk(ctx, event)
	features = append(features, frequencyRisk)

	// Location risk (unusual location = higher risk)
	locationRisk := mrs.getLocationRisk(event.IPAddress)
	features = append(features, locationRisk)

	if locationRisk > 0.7 {
		factors = append(factors, RiskFactor{
			ID:          "unusual_location",
			Name:        "Unusual Location",
			Category:    "behavioral",
			Weight:      0.4,
			Value:       locationRisk,
			Impact:      locationRisk * 0.4,
			Description: fmt.Sprintf("Access from unusual IP: %s", event.IPAddress),
			Mitigation:  "Verify user identity and location",
		})
	}

	// User agent risk
	userAgentRisk := mrs.getUserAgentRisk(event.UserAgent)
	features = append(features, userAgentRisk)

	return features, factors
}

// extractContextualFeatures extracts contextual features
func (mrs *MLRiskScorer) extractContextualFeatures(_ context.Context, event *AuditEvent) ([]float64, []RiskFactor) {
	var features []float64
	var factors []RiskFactor

	// Data sensitivity risk
	dataSensitivityRisk := mrs.getDataSensitivityRisk(event.DataAccessed)
	features = append(features, dataSensitivityRisk)

	if dataSensitivityRisk > 0.8 {
		factors = append(factors, RiskFactor{
			ID:          "sensitive_data",
			Name:        "Sensitive Data Access",
			Category:    "contextual",
			Weight:      0.5,
			Value:       dataSensitivityRisk,
			Impact:      dataSensitivityRisk * 0.5,
			Description: "Access to highly sensitive data",
			Mitigation:  "Ensure proper authorization for sensitive data access",
		})
	}

	// Compliance risk
	complianceRisk := mrs.getComplianceRisk(event.Compliance)
	features = append(features, complianceRisk)

	// Security context risk
	securityRisk := mrs.getSecurityContextRisk(event.Security)
	features = append(features, securityRisk)

	return features, factors
}

// Helper methods for risk calculation

func (mrs *MLRiskScorer) getEventTypeRisk(eventType string) float64 {
	riskMap := map[string]float64{
		"authentication": 0.3,
		"authorization":  0.4,
		"data_access":    0.6,
		"data_modify":    0.8,
		"admin_action":   0.9,
		"security_event": 1.0,
	}
	if risk, exists := riskMap[eventType]; exists {
		return risk
	}
	return 0.5 // Default risk
}

func (mrs *MLRiskScorer) getActionRisk(action string) float64 {
	riskMap := map[string]float64{
		"login":  0.2,
		"logout": 0.1,
		"read":   0.3,
		"create": 0.5,
		"update": 0.6,
		"delete": 0.9,
		"admin":  1.0,
	}
	if risk, exists := riskMap[action]; exists {
		return risk
	}
	return 0.5
}

func (mrs *MLRiskScorer) getSeverityRisk(severity string) float64 {
	riskMap := map[string]float64{
		riskLevelLow:      0.2,
		riskLevelMedium:   0.5,
		riskLevelHigh:     0.8,
		riskLevelCritical: 1.0,
	}
	if risk, exists := riskMap[severity]; exists {
		return risk
	}
	return 0.3
}

func (mrs *MLRiskScorer) getDurationRisk(duration time.Duration) float64 {
	seconds := duration.Seconds()
	if seconds < 0.1 || seconds > 300 { // Very fast or very slow
		return 0.8
	}
	if seconds < 1 || seconds > 60 {
		return 0.5
	}
	return 0.2
}

func (mrs *MLRiskScorer) getTimeOfDayRisk(hour int) float64 {
	// Business hours (9 AM - 5 PM) = low risk
	if hour >= 9 && hour <= 17 {
		return 0.2
	}
	// Evening (6 PM - 10 PM) = medium risk
	if hour >= 18 && hour <= 22 {
		return 0.5
	}
	// Night/early morning = high risk
	return 0.9
}

func (mrs *MLRiskScorer) getDayOfWeekRisk(day time.Weekday) float64 {
	// Weekdays = low risk
	if day >= time.Monday && day <= time.Friday {
		return 0.2
	}
	// Weekends = higher risk
	return 0.7
}

func (mrs *MLRiskScorer) getFrequencyRisk(_ context.Context, _ *AuditEvent) float64 {
	// This would analyze frequency patterns
	// For now, return a default value
	return 0.3
}

func (mrs *MLRiskScorer) getLocationRisk(_ string) float64 {
	// This would check against known good/bad IP ranges
	// For now, return a default value
	return 0.3
}

func (mrs *MLRiskScorer) getUserAgentRisk(_ string) float64 {
	// This would analyze user agent patterns
	// For now, return a default value
	return 0.2
}

func (mrs *MLRiskScorer) getDataSensitivityRisk(dataAccessed []string) float64 {
	if len(dataAccessed) == 0 {
		return 0.1
	}

	maxRisk := 0.0
	for _, data := range dataAccessed {
		risk := mrs.getDataTypeRisk(data)
		if risk > maxRisk {
			maxRisk = risk
		}
	}
	return maxRisk
}

func (mrs *MLRiskScorer) getDataTypeRisk(dataType string) float64 {
	riskMap := map[string]float64{
		"public":       0.1,
		"internal":     0.3,
		"confidential": 0.7,
		"restricted":   0.9,
		"pii":          0.8,
		"phi":          0.9,
		"payment":      1.0,
	}
	if risk, exists := riskMap[dataType]; exists {
		return risk
	}
	return 0.5
}

func (mrs *MLRiskScorer) getComplianceRisk(compliance ComplianceContext) float64 {
	if len(compliance.Violations) > 0 {
		return 1.0
	}
	if compliance.RiskLevel == riskLevelHigh || compliance.RiskLevel == riskLevelCritical {
		return 0.8
	}
	return 0.3
}

func (mrs *MLRiskScorer) getSecurityContextRisk(security SecurityContext) float64 {
	risk := 0.0

	// Threat level
	switch security.ThreatLevel {
	case riskLevelCritical:
		risk += 0.4
	case riskLevelHigh:
		risk += 0.3
	case riskLevelMedium:
		risk += 0.2
	}

	// Encryption
	if !security.EncryptionUsed {
		risk += 0.3
	}

	// Threat indicators
	if len(security.ThreatIndicators) > 0 {
		risk += 0.4
	}

	return math.Min(1.0, risk)
}

// calculateWeightedScore calculates weighted score from risk factors
func (mrs *MLRiskScorer) calculateWeightedScore(factors []RiskFactor) float64 {
	var totalWeight, weightedSum float64

	for _, factor := range factors {
		weightedSum += factor.Value * factor.Weight
		totalWeight += factor.Weight
	}

	if totalWeight == 0 {
		return 0
	}

	return (weightedSum / totalWeight) * 100
}

// applyContextualAdjustments applies contextual adjustments to the score
func (mrs *MLRiskScorer) applyContextualAdjustments(_ context.Context, _ *AuditEvent, baseScore float64) float64 {
	// This would apply contextual adjustments based on user/tenant profiles
	// For now, return the base score
	return baseScore
}

// applyTemporalAdjustments applies temporal adjustments to the score
func (mrs *MLRiskScorer) applyTemporalAdjustments(_ context.Context, _ *AuditEvent, score float64) float64 {
	// This would apply temporal pattern adjustments
	// For now, return the score
	return score
}

// applyBehavioralAdjustments applies behavioral adjustments to the score
func (mrs *MLRiskScorer) applyBehavioralAdjustments(_ context.Context, _ *AuditEvent, score float64) float64 {
	// This would apply behavioral pattern adjustments
	// For now, return the score
	return score
}

// determineRiskLevel determines risk level from score
func (mrs *MLRiskScorer) determineRiskLevel(score float64) string {
	if score >= mrs.config.ThresholdConfig.CriticalThreshold {
		return riskLevelCritical
	}
	if score >= mrs.config.ThresholdConfig.HighThreshold {
		return riskLevelHigh
	}
	if score >= mrs.config.ThresholdConfig.MediumThreshold {
		return riskLevelMedium
	}
	return riskLevelLow
}

// calculateConfidence calculates confidence in the risk score
func (mrs *MLRiskScorer) calculateConfidence(features []float64, factors []RiskFactor) float64 {
	// Base confidence on number of features and model metrics
	baseConfidence := 0.7

	// Adjust based on feature count
	featureBonus := math.Min(0.2, float64(len(features))*0.02)

	// Adjust based on factor quality
	factorBonus := math.Min(0.1, float64(len(factors))*0.01)

	return math.Min(1.0, baseConfidence+featureBonus+factorBonus)
}

// generateRecommendations generates recommendations based on risk level and factors
func (mrs *MLRiskScorer) generateRecommendations(riskLevel string, factors []RiskFactor) []string {
	var recommendations []string

	switch riskLevel {
	case riskLevelCritical:
		recommendations = append(recommendations, "Immediate investigation required")
		recommendations = append(recommendations, "Consider blocking user/session")
		recommendations = append(recommendations, "Escalate to security team")
	case riskLevelHigh:
		recommendations = append(recommendations, "Enhanced monitoring required")
		recommendations = append(recommendations, "Additional authentication may be needed")
		recommendations = append(recommendations, "Review user permissions")
	case riskLevelMedium:
		recommendations = append(recommendations, "Monitor for patterns")
		recommendations = append(recommendations, "Consider additional logging")
	case riskLevelLow:
		recommendations = append(recommendations, "Continue normal monitoring")
	}

	// Add factor-specific recommendations
	for _, factor := range factors {
		if factor.Mitigation != "" {
			recommendations = append(recommendations, factor.Mitigation)
		}
	}

	return recommendations
}

// getTopRiskFactors returns top risk factors by impact
func (mrs *MLRiskScorer) getTopRiskFactors(factors []RiskFactor, limit int) []RiskFactor {
	// Sort by impact (descending)
	sort.Slice(factors, func(i, j int) bool {
		return factors[i].Impact > factors[j].Impact
	})

	if len(factors) <= limit {
		return factors
	}
	return factors[:limit]
}

// calculateTrendDirection calculates trend direction from events
func (mrs *MLRiskScorer) calculateTrendDirection(events []*AuditEvent) string {
	if len(events) < 2 {
		return "stable"
	}

	// Sort events by timestamp
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	// Simple trend calculation based on event severity
	firstHalf := events[:len(events)/2]
	secondHalf := events[len(events)/2:]

	firstAvg := mrs.calculateAverageSeverity(firstHalf)
	secondAvg := mrs.calculateAverageSeverity(secondHalf)

	if secondAvg > firstAvg*1.1 {
		return "increasing"
	}
	if secondAvg < firstAvg*0.9 {
		return "decreasing"
	}
	return "stable"
}

// calculateAverageSeverity calculates average severity of events
func (mrs *MLRiskScorer) calculateAverageSeverity(events []*AuditEvent) float64 {
	if len(events) == 0 {
		return 0
	}

	total := 0.0
	for _, event := range events {
		total += mrs.getSeverityRisk(event.Severity)
	}
	return total / float64(len(events))
}

// deduplicateRecommendations removes duplicate recommendations
func (mrs *MLRiskScorer) deduplicateRecommendations(recommendations []string) []string {
	seen := make(map[string]bool)
	var unique []string

	for _, rec := range recommendations {
		if !seen[rec] {
			seen[rec] = true
			unique = append(unique, rec)
		}
	}

	return unique
}

// getTimeRange gets time range from events
func (mrs *MLRiskScorer) getTimeRange(events []*AuditEvent) TimeRange {
	if len(events) == 0 {
		now := time.Now()
		return TimeRange{Start: now, End: now}
	}

	start := events[0].Timestamp
	end := events[0].Timestamp

	for _, event := range events {
		if event.Timestamp.Before(start) {
			start = event.Timestamp
		}
		if event.Timestamp.After(end) {
			end = event.Timestamp
		}
	}

	return TimeRange{Start: start, End: end}
}

// getDefaultRiskFactors returns default risk factors
func getDefaultRiskFactors() []RiskFactor {
	return []RiskFactor{
		{
			ID:          "event_type",
			Name:        "Event Type Risk",
			Category:    "basic",
			Weight:      0.3,
			Description: "Risk based on event type",
		},
		{
			ID:          "action_risk",
			Name:        "Action Risk",
			Category:    "basic",
			Weight:      0.3,
			Description: "Risk based on action performed",
		},
		{
			ID:          "temporal_risk",
			Name:        "Temporal Risk",
			Category:    "temporal",
			Weight:      0.2,
			Description: "Risk based on timing patterns",
		},
		{
			ID:          "behavioral_risk",
			Name:        "Behavioral Risk",
			Category:    "behavioral",
			Weight:      0.2,
			Description: "Risk based on behavioral patterns",
		},
	}
}
