package analytics

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

const defaultAnalysisInterval = 5 * time.Minute

// PerformanceAnalyticsEngine provides advanced performance analytics
type PerformanceAnalyticsEngine struct {
	dataStore       AnalyticsDataStore
	thresholdMgr    ThresholdManager
	anomalyDetector AnomalyDetector
	trendAnalyzer   TrendAnalyzer
	alertManager    AlertManager
	stopCh          chan struct{}
	config          PerformanceAnalyticsConfig
	mu              sync.RWMutex
	running         bool
	stopOnce        sync.Once
}

// PerformanceAnalyticsConfig configures the performance analytics engine
type PerformanceAnalyticsConfig struct {
	DataRetentionDays      int           `json:"data_retention_days"`
	AnalysisInterval       time.Duration `json:"analysis_interval"`
	AnomalyDetectionWindow time.Duration `json:"anomaly_detection_window"`
	TrendAnalysisWindow    time.Duration `json:"trend_analysis_window"`
	MetricSamplingRate     float64       `json:"metric_sampling_rate"`
	MaxConcurrentAnalysis  int           `json:"max_concurrent_analysis"`
	Enabled                bool          `json:"enabled"`
	AlertingEnabled        bool          `json:"alerting_enabled"`
	EnablePredictive       bool          `json:"enable_predictive"`
	EnableMachineLearning  bool          `json:"enable_machine_learning"`
}

// PerformanceMetric represents a performance metric data point
type PerformanceMetric struct {
	Timestamp     time.Time         `json:"timestamp"`
	Tags          map[string]string `json:"tags"`
	Dimensions    map[string]string `json:"dimensions"`
	Metadata      map[string]any    `json:"metadata"`
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Unit          string            `json:"unit"`
	Source        string            `json:"source"`
	AggregateType AggregateType     `json:"aggregate_type"`
	Value         float64           `json:"value"`
}

// PerformanceAnalysis represents the result of performance analysis
// Memory optimized: 376 → 272 bytes (104 bytes saved)
type PerformanceAnalysis struct {
	TimeRange       TimeRange                   `json:"time_range"`
	GeneratedAt     time.Time                   `json:"generated_at"`
	ID              string                      `json:"id"`
	Metrics         []PerformanceMetric         `json:"metrics"`
	Trends          []PerformanceTrend          `json:"trends"`
	Anomalies       []PerformanceAnomaly        `json:"anomalies"`
	Predictions     []PerformancePrediction     `json:"predictions"`
	Recommendations []PerformanceRecommendation `json:"recommendations"`
	Alerts          []PerformanceAlert          `json:"alerts"`
	Statistics      PerformanceStatistics       `json:"statistics"`
	Duration        time.Duration               `json:"duration"`
	HealthScore     float64                     `json:"health_score"`
}

// PerformanceStatistics provides statistical analysis of metrics
type PerformanceStatistics struct {
	Percentiles  map[string]float64   `json:"percentiles"`
	Outliers     []OutlierPoint       `json:"outliers"`
	Distribution DistributionAnalysis `json:"distribution"`
	Count        int64                `json:"count"`
	Mean         float64              `json:"mean"`
	Median       float64              `json:"median"`
	Mode         float64              `json:"mode"`
	StdDev       float64              `json:"std_dev"`
	Variance     float64              `json:"variance"`
	Min          float64              `json:"min"`
	Max          float64              `json:"max"`
	Range        float64              `json:"range"`
}

// PerformanceTrend represents a trend in performance metrics
type PerformanceTrend struct {
	Forecast    TrendForecast      `json:"forecast"`
	ID          string             `json:"id"`
	MetricName  string             `json:"metric_name"`
	Direction   TrendDirection     `json:"direction"`
	Seasonality SeasonalityPattern `json:"seasonality"`
	Strength    float64            `json:"strength"`
	Confidence  float64            `json:"confidence"`
	Duration    time.Duration      `json:"duration"`
	Slope       float64            `json:"slope"`
	StartValue  float64            `json:"start_value"`
	EndValue    float64            `json:"end_value"`
	ChangeRate  float64            `json:"change_rate"`
}

// PerformanceAnomaly represents an anomaly in performance metrics
type PerformanceAnomaly struct {
	Context         AnomalyContext  `json:"context"`
	Timestamp       time.Time       `json:"timestamp"`
	Impact          AnomalyImpact   `json:"impact"`
	Severity        AnomalySeverity `json:"severity"`
	ID              string          `json:"id"`
	Type            AnomalyType     `json:"type"`
	MetricName      string          `json:"metric_name"`
	Explanation     string          `json:"explanation"`
	Recommendations []string        `json:"recommendations"`
	RelatedMetrics  []string        `json:"related_metrics"`
	ExpectedValue   float64         `json:"expected_value"`
	Deviation       float64         `json:"deviation"`
	Confidence      float64         `json:"confidence"`
	Value           float64         `json:"value"`
}

// PerformancePrediction represents a prediction of future performance
type PerformancePrediction struct {
	GeneratedAt     time.Time            `json:"generated_at"`
	ID              string               `json:"id"`
	MetricName      string               `json:"metric_name"`
	PredictionType  PredictionType       `json:"prediction_type"`
	Methodology     string               `json:"methodology"`
	Assumptions     []string             `json:"assumptions"`
	RiskFactors     []RiskFactor         `json:"risk_factors"`
	Scenarios       []PredictionScenario `json:"scenarios"`
	PredictionBands PredictionBands      `json:"prediction_bands"`
	TimeHorizon     time.Duration        `json:"time_horizon"`
	PredictedValue  float64              `json:"predicted_value"`
	ConfidenceLevel float64              `json:"confidence_level"`
}

// PerformanceRecommendation represents an actionable recommendation
// Memory optimized: 256 → 232 bytes (24 bytes saved)
type PerformanceRecommendation struct {
	Effort      EffortLevel            `json:"effort"`
	Description string                 `json:"description"`
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Priority    Priority               `json:"priority"`
	Category    RecommendationCategory `json:"category"`
	Impact      ImpactLevel            `json:"impact"`
	Benefits    []string               `json:"benefits"`
	Risks       []string               `json:"risks"`
	Metrics     []string               `json:"metrics"`
	Actions     []RecommendedAction    `json:"actions"`
	Cost        CostEstimate           `json:"cost"`
	Timeline    time.Duration          `json:"timeline"`
}

// PerformanceAlert represents a performance-related alert
type PerformanceAlert struct {
	Context      AlertContext     `json:"context"`
	Timestamp    time.Time        `json:"timestamp"`
	MetricName   string           `json:"metric_name"`
	Severity     AlertSeverity    `json:"severity"`
	Priority     AlertPriority    `json:"priority"`
	Description  string           `json:"description"`
	ID           string           `json:"id"`
	TriggerType  AlertTriggerType `json:"trigger_type"`
	Title        string           `json:"title"`
	Actions      []AlertAction    `json:"actions"`
	Threshold    Threshold        `json:"threshold"`
	ActualValue  float64          `json:"actual_value"`
	Suppressed   bool             `json:"suppressed"`
	Acknowledged bool             `json:"acknowledged"`
}

// Supporting types and interfaces
type AnalyticsDataStore interface {
	StoreMetric(ctx context.Context, metric *PerformanceMetric) error
	GetMetrics(ctx context.Context, query MetricQuery) ([]PerformanceMetric, error)
	GetAggregatedMetrics(ctx context.Context, query AggregateQuery) ([]AggregatedMetric, error)
	DeleteOldMetrics(ctx context.Context, before time.Time) error
	GetMetricNames() []string
}

type ThresholdManager interface {
	GetThresholds(metricName string) []Threshold
	SetThreshold(metricName string, threshold Threshold) error
	UpdateDynamicThresholds(ctx context.Context, metrics []PerformanceMetric) error
	EvaluateThresholds(metric PerformanceMetric) []ThresholdViolation
}

type AnomalyDetector interface {
	DetectAnomalies(ctx context.Context, metrics []PerformanceMetric) ([]PerformanceAnomaly, error)
	TrainModel(ctx context.Context, historicalData []PerformanceMetric) error
	UpdateBaseline(ctx context.Context, metrics []PerformanceMetric) error
	GetDetectionSensitivity() float64
	SetDetectionSensitivity(sensitivity float64)
}

type TrendAnalyzer interface {
	AnalyzeTrends(ctx context.Context, metrics []PerformanceMetric) ([]PerformanceTrend, error)
	PredictTrends(ctx context.Context, metrics []PerformanceMetric, horizon time.Duration) ([]PerformancePrediction, error)
	DetectSeasonality(ctx context.Context, metrics []PerformanceMetric) (SeasonalityPattern, error)
	CalculateCorrelations(ctx context.Context, metrics map[string][]PerformanceMetric) (CorrelationMatrix, error)
}

type AlertManager interface {
	SendAlert(ctx context.Context, alert PerformanceAlert) error
	GetActiveAlerts() []PerformanceAlert
	AcknowledgeAlert(alertID string) error
	SuppressAlert(alertID string, duration time.Duration) error
}

// Enums and constants
type AggregateType string

const (
	AggregateTypeSum   AggregateType = "sum"
	AggregateTypeAvg   AggregateType = "avg"
	AggregateTypeMin   AggregateType = "min"
	AggregateTypeMax   AggregateType = "max"
	AggregateTypeCount AggregateType = "count"
	AggregateTypeP50   AggregateType = "p50"
	AggregateTypeP90   AggregateType = "p90"
	AggregateTypeP95   AggregateType = "p95"
	AggregateTypeP99   AggregateType = "p99"
)

type TrendDirection string

const (
	TrendDirectionUp       TrendDirection = "up"
	TrendDirectionDown     TrendDirection = "down"
	TrendDirectionStable   TrendDirection = "stable"
	TrendDirectionVolatile TrendDirection = "volatile"
)

type AnomalySeverity string

const (
	AnomalySeverityLow      AnomalySeverity = "low"
	AnomalySeverityMedium   AnomalySeverity = "medium"
	AnomalySeverityHigh     AnomalySeverity = "high"
	AnomalySeverityCritical AnomalySeverity = "critical"
)

type AnomalyType string

const (
	AnomalyTypeSpike    AnomalyType = "spike"
	AnomalyTypeDip      AnomalyType = "dip"
	AnomalyTypeLevel    AnomalyType = "level_shift"
	AnomalyTypeTrend    AnomalyType = "trend_change"
	AnomalyTypeVariance AnomalyType = "variance_change"
	AnomalyTypeSeasonal AnomalyType = "seasonal_anomaly"
)

type PredictionType string

const (
	PredictionTypeValue     PredictionType = "value"
	PredictionTypeTrend     PredictionType = "trend"
	PredictionTypeThreshold PredictionType = "threshold"
	PredictionTypeCapacity  PredictionType = "capacity"
)

type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityMedium   Priority = "medium"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

type RecommendationCategory string

const (
	RecommendationCategoryPerformance    RecommendationCategory = "performance"
	RecommendationCategoryScaling        RecommendationCategory = "scaling"
	RecommendationCategoryOptimization   RecommendationCategory = "optimization"
	RecommendationCategoryConfiguration  RecommendationCategory = "configuration"
	RecommendationCategoryInfrastructure RecommendationCategory = "infrastructure"
	RecommendationCategorySecurity       RecommendationCategory = "security"
)

type ImpactLevel string

const (
	ImpactLevelLow    ImpactLevel = "low"
	ImpactLevelMedium ImpactLevel = "medium"
	ImpactLevelHigh   ImpactLevel = "high"
)

type EffortLevel string

const (
	EffortLevelLow    EffortLevel = "low"
	EffortLevelMedium EffortLevel = "medium"
	EffortLevelHigh   EffortLevel = "high"
)

type AlertSeverity string

const (
	AlertSeverityInfo     AlertSeverity = "info"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityError    AlertSeverity = "error"
	AlertSeverityCritical AlertSeverity = "critical"
)

type AlertPriority string

const (
	AlertPriorityLow    AlertPriority = "low"
	AlertPriorityMedium AlertPriority = "medium"
	AlertPriorityHigh   AlertPriority = "high"
	AlertPriorityUrgent AlertPriority = "urgent"
)

type AlertTriggerType string

const (
	AlertTriggerThreshold AlertTriggerType = "threshold"
	AlertTriggerAnomaly   AlertTriggerType = "anomaly"
	AlertTriggerTrend     AlertTriggerType = "trend"
	AlertTriggerForecast  AlertTriggerType = "forecast"
)

// Complex types
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type DistributionAnalysis struct {
	Type       string  `json:"type"`
	Skewness   float64 `json:"skewness"`
	Kurtosis   float64 `json:"kurtosis"`
	IsNormal   bool    `json:"is_normal"`
	Confidence float64 `json:"confidence"`
}

type OutlierPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Severity  string    `json:"severity"`
	Value     float64   `json:"value"`
	ZScore    float64   `json:"z_score"`
}

type SeasonalityPattern struct {
	Patterns   []Pattern     `json:"patterns"`
	Cycle      time.Duration `json:"cycle"`
	Strength   float64       `json:"strength"`
	Confidence float64       `json:"confidence"`
	Detected   bool          `json:"detected"`
}

type Pattern struct {
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	Name       string    `json:"name"`
	Amplitude  float64   `json:"amplitude"`
	Frequency  float64   `json:"frequency"`
	Confidence float64   `json:"confidence"`
}

type TrendForecast struct {
	Methodology string          `json:"methodology"`
	Points      []ForecastPoint `json:"points"`
	Confidence  float64         `json:"confidence"`
}

type ForecastPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Lower     float64   `json:"lower"`
	Upper     float64   `json:"upper"`
}

type AnomalyContext struct {
	SystemState      map[string]any    `json:"system_state"`
	PrecedingTrend   string            `json:"preceding_trend"`
	ConcurrentEvents []ConcurrentEvent `json:"concurrent_events"`
	RelatedMetrics   []RelatedMetric   `json:"related_metrics"`
}

type ConcurrentEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Correlation float64   `json:"correlation"`
}

type RelatedMetric struct {
	Name        string  `json:"name"`
	Impact      string  `json:"impact"`
	Value       float64 `json:"value"`
	Correlation float64 `json:"correlation"`
}

type AnomalyImpact struct {
	UserExperience  ImpactLevel `json:"user_experience"`
	SystemStability ImpactLevel `json:"system_stability"`
	BusinessMetrics ImpactLevel `json:"business_metrics"`
	OperationalCost ImpactLevel `json:"operational_cost"`
	SecurityRisk    ImpactLevel `json:"security_risk"`
	ComplianceRisk  ImpactLevel `json:"compliance_risk"`
}

type PredictionBands struct {
	Upper95 float64 `json:"upper_95"`
	Upper80 float64 `json:"upper_80"`
	Lower80 float64 `json:"lower_80"`
	Lower95 float64 `json:"lower_95"`
}

type RiskFactor struct {
	Name        string  `json:"name"`
	Impact      string  `json:"impact"`
	Mitigation  string  `json:"mitigation"`
	Probability float64 `json:"probability"`
}

type PredictionScenario struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Conditions  []string `json:"conditions"`
	Probability float64  `json:"probability"`
	Value       float64  `json:"value"`
}

type RecommendedAction struct {
	Type        string         `json:"type"`
	Description string         `json:"description"`
	Command     string         `json:"command,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Validation  string         `json:"validation,omitempty"`
	Rollback    string         `json:"rollback,omitempty"`
}

type CostEstimate struct {
	Currency  string  `json:"currency"`
	Period    string  `json:"period"`
	Initial   float64 `json:"initial"`
	Recurring float64 `json:"recurring"`
}

type Threshold struct {
	Name     string            `json:"name"`
	Operator ThresholdOperator `json:"operator"`
	Severity AlertSeverity     `json:"severity"`
	Value    float64           `json:"value"`
	Duration time.Duration     `json:"duration"`
	Dynamic  bool              `json:"dynamic"`
}

type ThresholdOperator string

const (
	ThresholdOperatorGT  ThresholdOperator = "gt"
	ThresholdOperatorGTE ThresholdOperator = "gte"
	ThresholdOperatorLT  ThresholdOperator = "lt"
	ThresholdOperatorLTE ThresholdOperator = "lte"
	ThresholdOperatorEQ  ThresholdOperator = "eq"
	ThresholdOperatorNE  ThresholdOperator = "ne"
)

type ThresholdViolation struct {
	Timestamp   time.Time     `json:"timestamp"`
	Threshold   Threshold     `json:"threshold"`
	ActualValue float64       `json:"actual_value"`
	Duration    time.Duration `json:"duration"`
}

type AlertContext struct {
	SystemContext   map[string]any `json:"system_context"`
	UserContext     map[string]any `json:"user_context"`
	BusinessContext map[string]any `json:"business_context"`
	TriggerMetric   string         `json:"trigger_metric"`
	RelatedMetrics  []string       `json:"related_metrics"`
}

type AlertAction struct {
	Parameters  map[string]any `json:"parameters"`
	Type        string         `json:"type"`
	Description string         `json:"description"`
	Target      string         `json:"target"`
	Automated   bool           `json:"automated"`
}

type MetricQuery struct {
	TimeRange   TimeRange         `json:"time_range"`
	Tags        map[string]string `json:"tags"`
	MetricNames []string          `json:"metric_names"`
	Limit       int               `json:"limit"`
	Offset      int               `json:"offset"`
}

type AggregateQuery struct {
	AggregateType AggregateType `json:"aggregate_type"`
	GroupBy       []string      `json:"group_by"`
	MetricQuery
	Interval time.Duration `json:"interval"`
}

type AggregatedMetric struct {
	Timestamp time.Time         `json:"timestamp"`
	Tags      map[string]string `json:"tags"`
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
}

type CorrelationMatrix struct {
	CalculatedAt time.Time   `json:"calculated_at"`
	Metrics      []string    `json:"metrics"`
	Correlations [][]float64 `json:"correlations"`
	Significance [][]float64 `json:"significance"`
}

// NewPerformanceAnalyticsEngine creates a new performance analytics engine
func NewPerformanceAnalyticsEngine(config PerformanceAnalyticsConfig) *PerformanceAnalyticsEngine {
	engine := &PerformanceAnalyticsEngine{
		config: config,
	}
	engine.resetStopChannelLocked()
	return engine
}

func normalizePerformanceAnalyticsConfig(config PerformanceAnalyticsConfig) (PerformanceAnalyticsConfig, error) {
	if config.DataRetentionDays < 0 {
		return config, fmt.Errorf("data retention days must be non-negative")
	}

	if config.AnalysisInterval < 0 {
		return config, fmt.Errorf("analysis interval must be greater than zero")
	}

	if config.AnalysisInterval == 0 {
		config.AnalysisInterval = defaultAnalysisInterval
	}

	return config, nil
}

// Start starts the performance analytics engine
func (pae *PerformanceAnalyticsEngine) Start(ctx context.Context) error {
	pae.mu.Lock()
	defer pae.mu.Unlock()

	if pae.running {
		return fmt.Errorf("performance analytics engine already running")
	}

	if !pae.config.Enabled {
		return fmt.Errorf("performance analytics engine not enabled")
	}

	normalizedConfig, err := normalizePerformanceAnalyticsConfig(pae.config)
	if err != nil {
		return err
	}
	pae.config = normalizedConfig
	pae.resetStopChannelLocked()
	pae.running = true

	// Start background analysis
	go pae.runContinuousAnalysis(ctx)
	go pae.runDataCleanup(ctx)

	return nil
}

// Stop stops the performance analytics engine
func (pae *PerformanceAnalyticsEngine) Stop() error {
	pae.mu.Lock()
	defer pae.mu.Unlock()

	if !pae.running {
		return nil
	}

	pae.stopOnce.Do(func() {
		close(pae.stopCh)
	})
	pae.running = false

	return nil
}

func (pae *PerformanceAnalyticsEngine) resetStopChannelLocked() {
	pae.stopCh = make(chan struct{})
	pae.stopOnce = sync.Once{}
}

// AnalyzePerformance performs comprehensive performance analysis
func (pae *PerformanceAnalyticsEngine) AnalyzePerformance(ctx context.Context, timeRange TimeRange) (*PerformanceAnalysis, error) {
	startTime := time.Now()

	// Get metrics for analysis
	query := MetricQuery{
		TimeRange: timeRange,
		Limit:     10000, // Configurable limit
	}

	metrics, err := pae.dataStore.GetMetrics(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}

	if len(metrics) == 0 {
		return &PerformanceAnalysis{
			ID:          generateAnalysisID(),
			TimeRange:   timeRange,
			GeneratedAt: time.Now(),
			Duration:    time.Since(startTime),
		}, nil
	}

	analysis := &PerformanceAnalysis{
		ID:        generateAnalysisID(),
		TimeRange: timeRange,
		Metrics:   metrics,
	}

	// Perform statistical analysis
	analysis.Statistics = pae.calculateStatistics(metrics)

	// Analyze trends
	if pae.trendAnalyzer != nil {
		trends, err := pae.trendAnalyzer.AnalyzeTrends(ctx, metrics)
		if err == nil {
			analysis.Trends = trends
		}

		// Generate predictions if enabled
		if pae.config.EnablePredictive {
			predictions, err := pae.trendAnalyzer.PredictTrends(ctx, metrics, pae.config.TrendAnalysisWindow)
			if err == nil {
				analysis.Predictions = predictions
			}
		}
	}

	// Detect anomalies
	if pae.anomalyDetector != nil {
		anomalies, err := pae.anomalyDetector.DetectAnomalies(ctx, metrics)
		if err == nil {
			analysis.Anomalies = anomalies
		}
	}

	// Calculate health score
	analysis.HealthScore = pae.calculateHealthScore(analysis)

	// Generate recommendations
	analysis.Recommendations = pae.generateRecommendations(analysis)

	// Check for alerts
	if pae.config.AlertingEnabled {
		analysis.Alerts = pae.generateAlerts(analysis)
	}

	analysis.GeneratedAt = time.Now()
	analysis.Duration = time.Since(startTime)

	return analysis, nil
}

// calculateStatistics calculates statistical measures for metrics
func (pae *PerformanceAnalyticsEngine) calculateStatistics(metrics []PerformanceMetric) PerformanceStatistics {
	if len(metrics) == 0 {
		return PerformanceStatistics{}
	}

	values := make([]float64, len(metrics))
	for i, metric := range metrics {
		values[i] = metric.Value
	}

	sort.Float64s(values)

	stats := PerformanceStatistics{
		Count: int64(len(values)),
		Min:   values[0],
		Max:   values[len(values)-1],
		Range: values[len(values)-1] - values[0],
	}

	// Calculate mean
	var sum float64
	for _, value := range values {
		sum += value
	}
	stats.Mean = sum / float64(len(values))

	// Calculate median
	if len(values)%2 == 0 {
		stats.Median = (values[len(values)/2-1] + values[len(values)/2]) / 2
	} else {
		stats.Median = values[len(values)/2]
	}

	// Calculate variance and standard deviation
	var variance float64
	for _, value := range values {
		diff := value - stats.Mean
		variance += diff * diff
	}
	stats.Variance = variance / float64(len(values))
	stats.StdDev = math.Sqrt(stats.Variance)

	// Calculate percentiles
	stats.Percentiles = map[string]float64{
		"p50": calculatePercentile(values, 50),
		"p90": calculatePercentile(values, 90),
		"p95": calculatePercentile(values, 95),
		"p99": calculatePercentile(values, 99),
	}

	// Detect outliers (using IQR method)
	q1 := calculatePercentile(values, 25)
	q3 := calculatePercentile(values, 75)
	iqr := q3 - q1
	lowerBound := q1 - 1.5*iqr
	upperBound := q3 + 1.5*iqr

	outliers := []OutlierPoint{}
	for _, metric := range metrics {
		if metric.Value < lowerBound || metric.Value > upperBound {
			zScore := (metric.Value - stats.Mean) / stats.StdDev
			severity := "mild"
			if math.Abs(zScore) > 3 {
				severity = "extreme"
			} else if math.Abs(zScore) > 2 {
				severity = "moderate"
			}

			outliers = append(outliers, OutlierPoint{
				Timestamp: metric.Timestamp,
				Value:     metric.Value,
				ZScore:    zScore,
				Severity:  severity,
			})
		}
	}
	stats.Outliers = outliers

	// Distribution analysis
	stats.Distribution = analyzeDistribution(values, stats)

	return stats
}

// calculatePercentile calculates the specified percentile of sorted values
func calculatePercentile(sortedValues []float64, percentile float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}

	index := percentile / 100 * float64(len(sortedValues)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))

	if lower == upper {
		return sortedValues[lower]
	}

	weight := index - float64(lower)
	return sortedValues[lower]*(1-weight) + sortedValues[upper]*weight
}

// analyzeDistribution analyzes the distribution of values
func analyzeDistribution(values []float64, stats PerformanceStatistics) DistributionAnalysis {
	// Calculate skewness
	var skewness float64
	for _, value := range values {
		normalized := (value - stats.Mean) / stats.StdDev
		skewness += normalized * normalized * normalized
	}
	skewness /= float64(len(values))

	// Calculate kurtosis
	var kurtosis float64
	for _, value := range values {
		kurtosis += math.Pow((value-stats.Mean)/stats.StdDev, 4)
	}
	kurtosis = kurtosis/float64(len(values)) - 3

	// Simple normality test (Shapiro-Wilk approximation)
	isNormal := math.Abs(skewness) < 0.5 && math.Abs(kurtosis) < 0.5
	confidence := 1.0 - (math.Abs(skewness)+math.Abs(kurtosis))/2

	distributionType := "normal"
	if skewness > 0.5 {
		distributionType = "right_skewed"
	} else if skewness < -0.5 {
		distributionType = "left_skewed"
	}

	return DistributionAnalysis{
		Type:       distributionType,
		Skewness:   skewness,
		Kurtosis:   kurtosis,
		IsNormal:   isNormal,
		Confidence: confidence,
	}
}

// calculateHealthScore calculates an overall health score based on analysis
func (pae *PerformanceAnalyticsEngine) calculateHealthScore(analysis *PerformanceAnalysis) float64 {
	if len(analysis.Metrics) == 0 {
		return 0.0
	}

	score := 100.0

	// Deduct points for anomalies
	for _, anomaly := range analysis.Anomalies {
		switch anomaly.Severity {
		case AnomalySeverityCritical:
			score -= 20.0
		case AnomalySeverityHigh:
			score -= 10.0
		case AnomalySeverityMedium:
			score -= 5.0
		case AnomalySeverityLow:
			score -= 2.0
		}
	}

	// Deduct points for negative trends
	for _, trend := range analysis.Trends {
		if trend.Direction == TrendDirectionDown && trend.Confidence > 0.7 {
			score -= 5.0 * trend.Confidence
		}
	}

	// Deduct points for high variability
	if analysis.Statistics.StdDev > analysis.Statistics.Mean {
		score -= 10.0
	}

	// Ensure score doesn't go below 0
	if score < 0 {
		score = 0
	}

	return score
}

// generateRecommendations generates actionable recommendations
func (pae *PerformanceAnalyticsEngine) generateRecommendations(analysis *PerformanceAnalysis) []PerformanceRecommendation {
	recommendations := []PerformanceRecommendation{}

	// Recommendations based on anomalies
	for _, anomaly := range analysis.Anomalies {
		if anomaly.Severity == AnomalySeverityCritical || anomaly.Severity == AnomalySeverityHigh {
			recommendations = append(recommendations, PerformanceRecommendation{
				ID:          generateRecommendationID(),
				Title:       fmt.Sprintf("Address %s anomaly in %s", anomaly.Severity, anomaly.MetricName),
				Description: fmt.Sprintf("Critical anomaly detected with %.2f%% deviation from expected value", anomaly.Deviation*100),
				Priority:    Priority(anomaly.Severity),
				Category:    RecommendationCategoryPerformance,
				Impact:      ImpactLevelHigh,
				Effort:      EffortLevelMedium,
				Actions: []RecommendedAction{
					{
						Type:        "investigate",
						Description: "Investigate root cause of anomaly",
						Parameters: map[string]any{
							"metric":    anomaly.MetricName,
							"timestamp": anomaly.Timestamp,
							"value":     anomaly.Value,
						},
					},
				},
				Benefits: []string{"Improved system stability", "Better user experience"},
				Timeline: time.Hour * 4,
			})
		}
	}

	// Recommendations based on trends
	for _, trend := range analysis.Trends {
		if trend.Direction == TrendDirectionDown && trend.Confidence > 0.8 {
			recommendations = append(recommendations, PerformanceRecommendation{
				ID:          generateRecommendationID(),
				Title:       fmt.Sprintf("Address declining trend in %s", trend.MetricName),
				Description: fmt.Sprintf("Metric showing declining trend with %.2f%% confidence", trend.Confidence*100),
				Priority:    PriorityHigh,
				Category:    RecommendationCategoryOptimization,
				Impact:      ImpactLevelMedium,
				Effort:      EffortLevelMedium,
				Actions: []RecommendedAction{
					{
						Type:        "optimize",
						Description: "Optimize system to reverse declining trend",
						Parameters: map[string]any{
							"metric":      trend.MetricName,
							"trend":       trend.Direction,
							"change_rate": trend.ChangeRate,
						},
					},
				},
				Benefits: []string{"Prevent performance degradation", "Maintain SLA compliance"},
				Timeline: time.Hour * 24,
			})
		}
	}

	// General recommendations based on statistics
	if analysis.Statistics.StdDev > analysis.Statistics.Mean*0.5 {
		recommendations = append(recommendations, PerformanceRecommendation{
			ID:          generateRecommendationID(),
			Title:       "Reduce performance variability",
			Description: "High variability detected in performance metrics",
			Priority:    PriorityMedium,
			Category:    RecommendationCategoryConfiguration,
			Impact:      ImpactLevelMedium,
			Effort:      EffortLevelLow,
			Actions: []RecommendedAction{
				{
					Type:        "tune",
					Description: "Tune system configuration to reduce variability",
				},
			},
			Benefits: []string{"More predictable performance", "Better resource utilization"},
			Timeline: time.Hour * 8,
		})
	}

	return recommendations
}

// generateAlerts generates performance alerts
func (pae *PerformanceAnalyticsEngine) generateAlerts(analysis *PerformanceAnalysis) []PerformanceAlert {
	alerts := []PerformanceAlert{}

	// Alerts for critical anomalies
	for _, anomaly := range analysis.Anomalies {
		if anomaly.Severity == AnomalySeverityCritical {
			alerts = append(alerts, PerformanceAlert{
				ID:          generateAlertID(),
				Title:       fmt.Sprintf("Critical anomaly in %s", anomaly.MetricName),
				Description: fmt.Sprintf("Critical performance anomaly detected: %s", anomaly.Explanation),
				Severity:    AlertSeverityCritical,
				Priority:    AlertPriorityUrgent,
				Timestamp:   anomaly.Timestamp,
				MetricName:  anomaly.MetricName,
				ActualValue: anomaly.Value,
				TriggerType: AlertTriggerAnomaly,
			})
		}
	}

	// Alerts for threshold violations
	if pae.thresholdMgr != nil {
		for _, metric := range analysis.Metrics {
			violations := pae.thresholdMgr.EvaluateThresholds(metric)
			for _, violation := range violations {
				alerts = append(alerts, PerformanceAlert{
					ID:          generateAlertID(),
					Title:       fmt.Sprintf("Threshold violation: %s", metric.Name),
					Description: fmt.Sprintf("Metric %s exceeded threshold", metric.Name),
					Severity:    violation.Threshold.Severity,
					Priority:    AlertPriorityHigh,
					Timestamp:   metric.Timestamp,
					MetricName:  metric.Name,
					Threshold:   violation.Threshold,
					ActualValue: violation.ActualValue,
					TriggerType: AlertTriggerThreshold,
				})
			}
		}
	}

	return alerts
}

// Background processing
func (pae *PerformanceAnalyticsEngine) runContinuousAnalysis(ctx context.Context) {
	ticker := time.NewTicker(pae.config.AnalysisInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			timeRange := TimeRange{
				Start: time.Now().Add(-pae.config.AnalysisInterval),
				End:   time.Now(),
			}
			if _, err := pae.AnalyzePerformance(ctx, timeRange); err != nil {
				// Log error but continue processing
				// PerformanceAnalyticsEngine doesn't have a logger field
				_ = err
			}
		case <-pae.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (pae *PerformanceAnalyticsEngine) runDataCleanup(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cutoff := time.Now().AddDate(0, 0, -pae.config.DataRetentionDays)
			if pae.dataStore != nil {
				if err := pae.dataStore.DeleteOldMetrics(ctx, cutoff); err != nil {
					// Log error but continue cleanup
					// PerformanceAnalyticsEngine doesn't have a logger field
					_ = err
				}
			}
		case <-pae.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// SetDataStore sets the analytics data store
func (pae *PerformanceAnalyticsEngine) SetDataStore(dataStore AnalyticsDataStore) {
	pae.mu.Lock()
	defer pae.mu.Unlock()
	pae.dataStore = dataStore
}

// SetThresholdManager sets the threshold manager
func (pae *PerformanceAnalyticsEngine) SetThresholdManager(thresholdMgr ThresholdManager) {
	pae.mu.Lock()
	defer pae.mu.Unlock()
	pae.thresholdMgr = thresholdMgr
}

// SetAnomalyDetector sets the anomaly detector
func (pae *PerformanceAnalyticsEngine) SetAnomalyDetector(detector AnomalyDetector) {
	pae.mu.Lock()
	defer pae.mu.Unlock()
	pae.anomalyDetector = detector
}

// SetTrendAnalyzer sets the trend analyzer
func (pae *PerformanceAnalyticsEngine) SetTrendAnalyzer(analyzer TrendAnalyzer) {
	pae.mu.Lock()
	defer pae.mu.Unlock()
	pae.trendAnalyzer = analyzer
}

// SetAlertManager sets the alert manager
func (pae *PerformanceAnalyticsEngine) SetAlertManager(alertMgr AlertManager) {
	pae.mu.Lock()
	defer pae.mu.Unlock()
	pae.alertManager = alertMgr
}

// Utility functions
func generateAnalysisID() string {
	return fmt.Sprintf("analysis-%d", time.Now().UnixNano())
}

func generateRecommendationID() string {
	return fmt.Sprintf("recommendation-%d", time.Now().UnixNano())
}

func generateAlertID() string {
	return fmt.Sprintf("alert-%d", time.Now().UnixNano())
}
