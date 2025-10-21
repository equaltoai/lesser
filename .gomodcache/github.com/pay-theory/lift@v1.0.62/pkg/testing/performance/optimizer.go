package performance

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"
)

// PerformanceOptimizer provides comprehensive performance monitoring and optimization
type PerformanceOptimizer struct {
	monitors   []PerformanceMonitor
	benchmarks []Benchmark
	analyzers  []PerformanceAnalyzer
	optimizers []Optimizer
	config     PerformanceConfig
	mu         sync.RWMutex
}

// PerformanceConfig configures performance optimization behavior
type PerformanceConfig struct {
	OptimizationLevel   OptimizationLevel
	AlertThresholds     AlertThresholds
	MonitoringInterval  time.Duration
	BenchmarkTimeout    time.Duration
	TrendAnalysisWindow time.Duration
	RegressionThreshold float64
	EnablePredictive    bool
	EnableAutoOptimize  bool
}

type OptimizationLevel string

const (
	OptimizationLevelBasic      OptimizationLevel = "basic"
	OptimizationLevelStandard   OptimizationLevel = "standard"
	OptimizationLevelAggressive OptimizationLevel = "aggressive"
	OptimizationLevelExpert     OptimizationLevel = "expert"
)

type AlertThresholds struct {
	ResponseTime   time.Duration
	Throughput     float64
	ErrorRate      float64
	MemoryUsage    float64
	CPUUsage       float64
	DiskUsage      float64
	NetworkLatency time.Duration
}

// Core performance types
type PerformanceMetrics struct {
	Timestamp      time.Time
	CustomMetrics  map[string]float64
	CPUUsage       CPUMetrics
	MemoryUsage    MemoryMetrics
	DiskUsage      DiskMetrics
	NetworkMetrics NetworkMetrics
	ResponseTime   time.Duration
	RequestCount   int64
	ErrorCount     int64
	ActiveUsers    int64
	Throughput     float64
	ErrorRate      float64
}

type MemoryMetrics struct {
	GCPauses    []time.Duration
	Used        uint64
	Available   uint64
	Total       uint64
	Percentage  float64
	Allocations uint64
	Frees       uint64
}

type CPUMetrics struct {
	LoadAverage []float64
	Usage       float64
	Cores       int
	Frequency   float64
	Temperature float64
}

type DiskMetrics struct {
	ReadOps    uint64
	WriteOps   uint64
	ReadBytes  uint64
	WriteBytes uint64
	Usage      float64
	IOPS       float64
	Latency    time.Duration
}

type NetworkMetrics struct {
	BytesIn     uint64
	BytesOut    uint64
	PacketsIn   uint64
	PacketsOut  uint64
	Latency     time.Duration
	Bandwidth   float64
	Connections int
}

// Performance interfaces
type PerformanceMonitor interface {
	StartMonitoring(ctx context.Context, target string) error
	StopMonitoring() error
	GetMetrics(ctx context.Context) (PerformanceMetrics, error)
	DetectRegression(current, baseline PerformanceMetrics) bool
	GetThresholds() PerformanceThresholds
	SetThresholds(thresholds PerformanceThresholds) error
}

type PerformanceThresholds struct {
	MaxResponseTime   time.Duration
	MinThroughput     float64
	MaxErrorRate      float64
	MaxMemoryUsage    float64
	MaxCPUUsage       float64
	MaxDiskUsage      float64
	MaxNetworkLatency time.Duration
}

type Benchmark interface {
	Run(ctx context.Context, config BenchmarkConfig) (BenchmarkResult, error)
	GetBaseline() BenchmarkResult
	SetBaseline(baseline BenchmarkResult) error
	Compare(current, baseline BenchmarkResult) ComparisonResult
	GenerateReport() BenchmarkReport
}

type BenchmarkConfig struct {
	// Slices first (24 bytes each)
	Scenarios []BenchmarkScenario
	Targets   []string
	// 8-byte aligned fields
	Duration         time.Duration
	RequestRate      float64
	WarmupDuration   time.Duration
	CooldownDuration time.Duration
	// 4-byte fields grouped
	Concurrency int
	PayloadSize int
}

type BenchmarkScenario struct {
	Name        string
	Operations  []Operation
	Constraints []Constraint
	Weight      float64
}

type Operation struct {
	Headers map[string]string
	Type    OperationType
	Target  string
	Payload []byte
	Timeout time.Duration
	Retries int
}

type OperationType string

const (
	OperationTypeHTTPGet    OperationType = "http_get"
	OperationTypeHTTPPost   OperationType = "http_post"
	OperationTypeHTTPPut    OperationType = "http_put"
	OperationTypeHTTPDelete OperationType = "http_delete"
	OperationTypeDatabase   OperationType = "database"
	OperationTypeCache      OperationType = "cache"
	OperationTypeCompute    OperationType = "compute"
)

type Constraint struct {
	Type     ConstraintType
	Operator ConstraintOperator
	Metric   string
	Value    float64
}

type ConstraintType string

const (
	ConstraintTypeResponseTime ConstraintType = "response_time"
	ConstraintTypeThroughput   ConstraintType = "throughput"
	ConstraintTypeErrorRate    ConstraintType = "error_rate"
	ConstraintTypeMemory       ConstraintType = "memory"
	ConstraintTypeCPU          ConstraintType = "cpu"
)

type ConstraintOperator string

const (
	ConstraintOperatorLT  ConstraintOperator = "lt"
	ConstraintOperatorLTE ConstraintOperator = "lte"
	ConstraintOperatorGT  ConstraintOperator = "gt"
	ConstraintOperatorGTE ConstraintOperator = "gte"
	ConstraintOperatorEQ  ConstraintOperator = "eq"
)

type BenchmarkResult struct {
	StartTime      time.Time
	EndTime        time.Time
	CustomMetrics  map[string]any
	ErrorStats     ErrorStats
	Config         BenchmarkConfig
	ResourceUsage  ResourceUsageStats
	ResponseTimes  ResponseTimeStats
	Percentiles    PercentileStats
	Throughput     ThroughputStats
	Duration       time.Duration
	FailedRequests int64
	SuccessfulReqs int64
	TotalRequests  int64
}

type ResponseTimeStats struct {
	Min    time.Duration
	Max    time.Duration
	Mean   time.Duration
	Median time.Duration
	StdDev time.Duration
	P95    time.Duration
	P99    time.Duration
	P999   time.Duration
}

type ThroughputStats struct {
	RequestsPerSecond float64
	BytesPerSecond    float64
	Peak              float64
	Average           float64
	Minimum           float64
}

type ErrorStats struct {
	ErrorTypes    map[string]int64
	TotalErrors   int64
	ErrorRate     float64
	TimeoutErrors int64
	NetworkErrors int64
	ServerErrors  int64
}

type ResourceUsageStats struct {
	Memory  MemoryUsageStats
	CPU     CPUUsageStats
	Disk    DiskUsageStats
	Network NetworkUsageStats
}

type MemoryUsageStats struct {
	// 8-byte aligned fields first
	Peak    uint64
	Average uint64
	Minimum uint64
	GCTime  time.Duration
	// 4-byte field last
	GCCount int
}

type CPUUsageStats struct {
	// 8-byte aligned fields first
	Peak    float64
	Average float64
	Minimum float64
	// 4-byte field last
	Cores int
}

type DiskUsageStats struct {
	ReadBytes   uint64
	WriteBytes  uint64
	ReadOps     uint64
	WriteOps    uint64
	AverageIOPS float64
}

type NetworkUsageStats struct {
	// 8-byte aligned fields first
	BytesIn    uint64
	BytesOut   uint64
	PacketsIn  uint64
	PacketsOut uint64
	// 4-byte field last
	Connections int
}

type PercentileStats struct {
	P50  time.Duration
	P75  time.Duration
	P90  time.Duration
	P95  time.Duration
	P99  time.Duration
	P999 time.Duration
}

type ComparisonResult struct {
	OverallChange   PerformanceChange
	Improvements    []Improvement
	Regressions     []Regression
	Recommendations []string
	Significance    StatisticalSignificance
	Baseline        BenchmarkResult
	Current         BenchmarkResult
}

type Improvement struct {
	Metric       string
	OldValue     float64
	NewValue     float64
	Change       float64
	Percentage   float64
	Significance float64
}

type Regression struct {
	Metric     string
	Severity   RegressionSeverity
	OldValue   float64
	NewValue   float64
	Change     float64
	Percentage float64
}

type RegressionSeverity string

const (
	RegressionSeverityMinor    RegressionSeverity = "minor"
	RegressionSeverityModerate RegressionSeverity = "moderate"
	RegressionSeverityMajor    RegressionSeverity = "major"
	RegressionSeverityCritical RegressionSeverity = "critical"
)

type PerformanceChange string

const (
	PerformanceChangeImproved  PerformanceChange = "improved"
	PerformanceChangeRegressed PerformanceChange = "regressed"
	PerformanceChangeUnchanged PerformanceChange = "unchanged"
	PerformanceChangeMixed     PerformanceChange = "mixed"
)

type StatisticalSignificance struct {
	TestType    string
	PValue      float64
	Confidence  float64
	Significant bool
}

type BenchmarkReport struct {
	Timestamp   time.Time
	Version     string
	Results     []BenchmarkResult
	Comparisons []ComparisonResult
	Trends      TrendAnalysis
	Summary     BenchmarkSummary
}

type BenchmarkSummary struct {
	TotalBenchmarks int
	Improvements    int
	Regressions     int
	Unchanged       int
	OverallScore    float64
}

type PerformanceAnalyzer interface {
	Analyze(ctx context.Context, metrics []PerformanceMetrics) (AnalysisResult, error)
	IdentifyBottlenecks(metrics PerformanceMetrics) []Bottleneck
	GenerateRecommendations(analysis AnalysisResult) []Recommendation
	PredictScaling(trends []PerformanceMetrics) ScalingPrediction
}

type AnalysisResult struct {
	Timestamp       time.Time
	AnalysisType    AnalysisType
	Bottlenecks     []Bottleneck
	Patterns        []PerformancePattern
	Anomalies       []Anomaly
	Trends          []Trend
	Predictions     []Prediction
	Recommendations []Recommendation
	Score           PerformanceScore
}

type AnalysisType string

const (
	AnalysisTypeRealTime    AnalysisType = "realtime"
	AnalysisTypeBatch       AnalysisType = "batch"
	AnalysisTypePredictive  AnalysisType = "predictive"
	AnalysisTypeComparative AnalysisType = "comparative"
)

type Bottleneck struct {
	Type        BottleneckType
	Component   string
	Severity    BottleneckSeverity
	Impact      ImpactLevel
	Description string
	Metrics     map[string]float64
	Suggestions []string
	Priority    int
}

type BottleneckType string

const (
	BottleneckTypeCPU      BottleneckType = "cpu"
	BottleneckTypeMemory   BottleneckType = "memory"
	BottleneckTypeDisk     BottleneckType = "disk"
	BottleneckTypeNetwork  BottleneckType = "network"
	BottleneckTypeDatabase BottleneckType = "database"
	BottleneckTypeCache    BottleneckType = "cache"
	BottleneckTypeAPI      BottleneckType = "api"
)

type BottleneckSeverity string

const (
	BottleneckSeverityLow      BottleneckSeverity = "low"
	BottleneckSeverityMedium   BottleneckSeverity = "medium"
	BottleneckSeverityHigh     BottleneckSeverity = "high"
	BottleneckSeverityCritical BottleneckSeverity = "critical"
)

type ImpactLevel string

const (
	ImpactLevelMinimal     ImpactLevel = "minimal"
	ImpactLevelLow         ImpactLevel = "low"
	ImpactLevelModerate    ImpactLevel = "moderate"
	ImpactLevelHigh        ImpactLevel = "high"
	ImpactLevelSignificant ImpactLevel = "significant"
)

type PerformancePattern struct {
	Type        PatternType
	Description string
	Impact      ImpactLevel
	Examples    []PatternExample
	Frequency   time.Duration
	Confidence  float64
}

type PatternType string

const (
	PatternTypeCyclic   PatternType = "cyclic"
	PatternTypeTrending PatternType = "trending"
	PatternTypeSpike    PatternType = "spike"
	PatternTypeDrop     PatternType = "drop"
	PatternTypeStable   PatternType = "stable"
	PatternTypeVolatile PatternType = "volatile"
)

type PatternExample struct {
	Timestamp time.Time
	Context   string
	Value     float64
}

type Anomaly struct {
	Timestamp   time.Time
	Type        AnomalyType
	Metric      string
	Severity    AnomalySeverity
	Description string
	Causes      []string
	Expected    float64
	Actual      float64
	Deviation   float64
}

type AnomalyType string

const (
	AnomalyTypeSpike     AnomalyType = "spike"
	AnomalyTypeDrop      AnomalyType = "drop"
	AnomalyTypeFlat      AnomalyType = "flat"
	AnomalyTypeOscillate AnomalyType = "oscillate"
	AnomalyTypeShift     AnomalyType = "shift"
)

type AnomalySeverity string

const (
	AnomalySeverityLow      AnomalySeverity = "low"
	AnomalySeverityMedium   AnomalySeverity = "medium"
	AnomalySeverityHigh     AnomalySeverity = "high"
	AnomalySeverityCritical AnomalySeverity = "critical"
)

type Trend struct {
	Metric     string
	Direction  TrendDirection
	Projection TrendProjection
	Slope      float64
	Confidence float64
	Duration   time.Duration
}

type TrendDirection string

const (
	TrendDirectionUp       TrendDirection = "up"
	TrendDirectionDown     TrendDirection = "down"
	TrendDirectionStable   TrendDirection = "stable"
	TrendDirectionVolatile TrendDirection = "volatile"
)

type TrendProjection struct {
	Values      []ProjectedValue
	TimeHorizon time.Duration
	Confidence  float64
}

type ProjectedValue struct {
	Timestamp  time.Time
	Value      float64
	LowerBound float64
	UpperBound float64
}

type Prediction struct {
	Type        PredictionType
	Metric      string
	Scenario    PredictionScenario
	Assumptions []string
	TimeHorizon time.Duration
	Value       float64
	Confidence  float64
}

type PredictionType string

const (
	PredictionTypeCapacity    PredictionType = "capacity"
	PredictionTypePerformance PredictionType = "performance"
	PredictionTypeFailure     PredictionType = "failure"
	PredictionTypeScaling     PredictionType = "scaling"
)

type PredictionScenario string

const (
	PredictionScenarioBest     PredictionScenario = "best"
	PredictionScenarioExpected PredictionScenario = "expected"
	PredictionScenarioWorst    PredictionScenario = "worst"
)

type PerformanceScore struct {
	Breakdown    map[string]float64
	Overall      float64
	ResponseTime float64
	Throughput   float64
	Reliability  float64
	Efficiency   float64
	Scalability  float64
}

type Recommendation struct {
	ID          string
	Title       string
	Description string
	Type        RecommendationType
	Priority    RecommendationPriority
	Steps       []RecommendationStep
	References  []string
	Tags        []string
	Effort      EffortEstimate
	Impact      ImpactEstimate
}

type RecommendationType string

const (
	RecommendationTypeOptimization   RecommendationType = "optimization"
	RecommendationTypeScaling        RecommendationType = "scaling"
	RecommendationTypeConfiguration  RecommendationType = "configuration"
	RecommendationTypeArchitecture   RecommendationType = "architecture"
	RecommendationTypeInfrastructure RecommendationType = "infrastructure"
)

type RecommendationPriority string

const (
	RecommendationPriorityLow      RecommendationPriority = "low"
	RecommendationPriorityMedium   RecommendationPriority = "medium"
	RecommendationPriorityHigh     RecommendationPriority = "high"
	RecommendationPriorityCritical RecommendationPriority = "critical"
)

type ImpactEstimate struct {
	Performance float64
	Cost        float64
	Risk        float64
	Timeline    time.Duration
	Confidence  float64
}

type EffortEstimate struct {
	Complexity   ComplexityLevel
	Skills       []string
	Resources    []string
	Dependencies []string
	Hours        float64
}

type ComplexityLevel string

const (
	ComplexityLevelLow    ComplexityLevel = "low"
	ComplexityLevelMedium ComplexityLevel = "medium"
	ComplexityLevelHigh   ComplexityLevel = "high"
	ComplexityLevelExpert ComplexityLevel = "expert"
)

type RecommendationStep struct {
	Description string
	Action      string
	Validation  string
	Rollback    string
	Order       int
}

type ScalingPrediction struct {
	Recommendations []ScalingRecommendation
	Timeline        ScalingTimeline
	PredictedDemand DemandForecast
	CurrentCapacity CapacityMetrics
	ScalingNeeds    ScalingRequirements
	CostEstimate    CostEstimate
}

type CapacityMetrics struct {
	CPU     CapacityInfo
	Memory  CapacityInfo
	Storage CapacityInfo
	Network CapacityInfo
}

type CapacityInfo struct {
	Unit        string
	Current     float64
	Maximum     float64
	Utilization float64
	Available   float64
}

type DemandForecast struct {
	Methodology string
	Scenarios   []DemandScenario
	TimeHorizon time.Duration
	Confidence  float64
}

type DemandScenario struct {
	Name        string
	Growth      GrowthPattern
	Peak        PeakDemand
	Probability float64
}

type GrowthPattern struct {
	Type        GrowthType
	Seasonality []SeasonalPattern
	Rate        float64
}

type GrowthType string

const (
	GrowthTypeLinear      GrowthType = "linear"
	GrowthTypeExponential GrowthType = "exponential"
	GrowthTypeLogarithmic GrowthType = "logarithmic"
	GrowthTypeSeasonal    GrowthType = "seasonal"
)

type SeasonalPattern struct {
	Period    time.Duration
	Amplitude float64
	Phase     time.Duration
}

type PeakDemand struct {
	Timestamp time.Time
	Value     float64
	Duration  time.Duration
	Frequency time.Duration
}

type ScalingRequirements struct {
	CPU     ScalingRequirement
	Memory  ScalingRequirement
	Storage ScalingRequirement
	Network ScalingRequirement
}

type ScalingRequirement struct {
	Priority ScalingPriority
	Current  float64
	Required float64
	Increase float64
	Timeline time.Duration
}

type ScalingPriority string

const (
	ScalingPriorityLow      ScalingPriority = "low"
	ScalingPriorityMedium   ScalingPriority = "medium"
	ScalingPriorityHigh     ScalingPriority = "high"
	ScalingPriorityCritical ScalingPriority = "critical"
)

type ScalingRecommendation struct {
	Type        ScalingType
	Description string
	Risk        RiskLevel
	Resources   []ResourceRecommendation
	Benefits    []string
	Timeline    time.Duration
	Cost        float64
}

type ScalingType string

const (
	ScalingTypeVertical   ScalingType = "vertical"
	ScalingTypeHorizontal ScalingType = "horizontal"
	ScalingTypeAuto       ScalingType = "auto"
	ScalingTypeElastic    ScalingType = "elastic"
)

type ResourceRecommendation struct {
	Type     ResourceType
	Reason   string
	Current  ResourceSpec
	Proposed ResourceSpec
}

type ResourceType string

const (
	ResourceTypeCPU      ResourceType = "cpu"
	ResourceTypeMemory   ResourceType = "memory"
	ResourceTypeStorage  ResourceType = "storage"
	ResourceTypeNetwork  ResourceType = "network"
	ResourceTypeInstance ResourceType = "instance"
)

type ResourceSpec struct {
	Unit  string
	Type  string
	Value float64
}

type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

type ScalingTimeline struct {
	Phases []ScalingPhase
	Total  time.Duration
}

type ScalingPhase struct {
	Name         string
	Actions      []string
	Milestones   []string
	Dependencies []string
	Duration     time.Duration
}

type CostEstimate struct {
	Current   CostBreakdown
	Projected CostBreakdown
	Savings   CostBreakdown
	ROI       ROIAnalysis
}

type CostBreakdown struct {
	Compute float64
	Storage float64
	Network float64
	Other   float64
	Total   float64
	Period  time.Duration
}

type ROIAnalysis struct {
	Investment    float64
	Returns       float64
	PaybackPeriod time.Duration
	NPV           float64
	IRR           float64
}

// Optimizer interface for performance optimization
type Optimizer interface {
	Optimize(ctx context.Context, target OptimizationTarget) (OptimizationResult, error)
	GetOptimizationType() OptimizationType
	EstimateImpact(target OptimizationTarget) ImpactEstimate
	ValidateOptimization(result OptimizationResult) ValidationResult
}

type OptimizationTarget struct {
	Type        TargetType
	Component   string
	Constraints []OptimizationConstraint
	Objectives  []OptimizationObjective
	Metrics     PerformanceMetrics
}

type TargetType string

const (
	TargetTypeApplication    TargetType = "application"
	TargetTypeDatabase       TargetType = "database"
	TargetTypeCache          TargetType = "cache"
	TargetTypeNetwork        TargetType = "network"
	TargetTypeInfrastructure TargetType = "infrastructure"
)

type OptimizationConstraint struct {
	Type     ConstraintType
	Operator ConstraintOperator
	Value    float64
	Priority int
}

type OptimizationObjective struct {
	Metric   string
	Target   float64
	Weight   float64
	Priority int
}

type OptimizationType string

const (
	OptimizationTypeMemory        OptimizationType = "memory"
	OptimizationTypeCPU           OptimizationType = "cpu"
	OptimizationTypeIO            OptimizationType = "io"
	OptimizationTypeNetwork       OptimizationType = "network"
	OptimizationTypeAlgorithmic   OptimizationType = "algorithmic"
	OptimizationTypeArchitectural OptimizationType = "architectural"
)

type OptimizationResult struct {
	Timestamp     time.Time
	Target        OptimizationTarget
	Optimizations []AppliedOptimization
	Errors        []string
	BeforeMetrics PerformanceMetrics
	AfterMetrics  PerformanceMetrics
	Improvement   ImprovementMetrics
	Duration      time.Duration
	Success       bool
}

type AppliedOptimization struct {
	Parameters   map[string]any
	Type         OptimizationType
	Description  string
	RollbackInfo string
	Impact       ImpactMetrics
	Reversible   bool
}

type ImpactMetrics struct {
	Performance PerformanceImpact
	Resource    ResourceImpact
	Cost        CostImpact
	Risk        RiskImpact
}

type PerformanceImpact struct {
	ResponseTime float64
	Throughput   float64
	ErrorRate    float64
	Availability float64
}

type ResourceImpact struct {
	CPU     float64
	Memory  float64
	Storage float64
	Network float64
}

type CostImpact struct {
	Operational    float64
	Infrastructure float64
	Development    float64
	Maintenance    float64
}

type RiskImpact struct {
	Stability   float64
	Security    float64
	Compliance  float64
	Operational float64
}

type ImprovementMetrics struct {
	ResponseTimeImprovement float64
	ThroughputImprovement   float64
	ErrorRateImprovement    float64
	ResourceEfficiency      float64
	CostReduction           float64
	OverallScore            float64
}

type ValidationResult struct {
	Issues      []ValidationIssue
	Warnings    []string
	Suggestions []string
	Score       float64
	Valid       bool
}

type ValidationIssue struct {
	Type        IssueType
	Severity    IssueSeverity
	Description string
	Impact      string
	Resolution  string
}

type IssueType string

const (
	IssueTypePerformance IssueType = "performance"
	IssueTypeStability   IssueType = "stability"
	IssueTypeSecurity    IssueType = "security"
	IssueTypeCompliance  IssueType = "compliance"
)

type IssueSeverity string

const (
	IssueSeverityInfo     IssueSeverity = "info"
	IssueSeverityWarning  IssueSeverity = "warning"
	IssueSeverityError    IssueSeverity = "error"
	IssueSeverityCritical IssueSeverity = "critical"
)

// AlertingSystem interface for performance alerts
type AlertingSystem interface {
	RegisterAlert(alert AlertRule) error
	TriggerAlert(ctx context.Context, alert Alert) error
	GetActiveAlerts() []Alert
	AcknowledgeAlert(alertID string) error
	ResolveAlert(alertID string) error
}

type AlertRule struct {
	ID          string
	Name        string
	Description string
	Metric      string
	Condition   AlertCondition
	Severity    AlertSeverity
	Actions     []AlertAction
	Threshold   float64
	Duration    time.Duration
	Enabled     bool
}

type AlertCondition string

const (
	AlertConditionGreater AlertCondition = "greater"
	AlertConditionLess    AlertCondition = "less"
	AlertConditionEqual   AlertCondition = "equal"
	AlertConditionChange  AlertCondition = "change"
)

type AlertSeverity string

const (
	AlertSeverityInfo     AlertSeverity = "info"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityError    AlertSeverity = "error"
	AlertSeverityCritical AlertSeverity = "critical"
)

type AlertAction struct {
	Parameters map[string]any
	Type       ActionType
	Target     string
}

type ActionType string

const (
	ActionTypeEmail     ActionType = "email"
	ActionTypeSlack     ActionType = "slack"
	ActionTypeWebhook   ActionType = "webhook"
	ActionTypePagerDuty ActionType = "pagerduty"
	ActionTypeAutoScale ActionType = "autoscale"
	ActionTypeRestart   ActionType = "restart"
)

type Alert struct {
	Timestamp   time.Time
	Context     map[string]any
	ID          string
	RuleID      string
	Metric      string
	Severity    AlertSeverity
	Status      AlertStatus
	Description string
	Value       float64
	Threshold   float64
}

type AlertStatus string

const (
	AlertStatusActive       AlertStatus = "active"
	AlertStatusAcknowledged AlertStatus = "acknowledged"
	AlertStatusResolved     AlertStatus = "resolved"
	AlertStatusSuppressed   AlertStatus = "suppressed"
)

// TrendAnalyzer interface for trend analysis
type TrendAnalyzer interface {
	AnalyzeTrends(ctx context.Context, metrics []PerformanceMetrics) (TrendAnalysis, error)
	PredictTrends(ctx context.Context, historical []PerformanceMetrics, horizon time.Duration) (TrendPrediction, error)
	DetectAnomalies(ctx context.Context, metrics []PerformanceMetrics) ([]Anomaly, error)
	GenerateTrendReport(analysis TrendAnalysis) TrendReport
}

type TrendAnalysis struct {
	// slices (24 bytes each)
	Trends    []Trend
	Patterns  []PerformancePattern
	Anomalies []Anomaly
	// time.Time (24 bytes)
	Timestamp time.Time
	// struct
	Summary TrendSummary
	// 8-byte aligned fields
	Period     time.Duration
	Confidence float64
}

type TrendSummary struct {
	OverallDirection TrendDirection
	Seasonality      []SeasonalPattern
	KeyInsights      []string
	Stability        float64
	Volatility       float64
}

type TrendPrediction struct {
	Methodology string
	Predictions []Prediction
	Scenarios   []PredictionScenario
	Assumptions []string
	TimeHorizon time.Duration
	Confidence  float64
}

type TrendReport struct {
	Timestamp       time.Time
	Insights        []TrendInsight
	Recommendations []Recommendation
	Predictions     TrendPrediction
	Analysis        TrendAnalysis
}

type TrendInsight struct {
	Type        InsightType
	Description string
	Impact      ImpactLevel
	Evidence    []string
	Actions     []string
	Confidence  float64
}

type InsightType string

const (
	InsightTypePerformance InsightType = "performance"
	InsightTypeCapacity    InsightType = "capacity"
	InsightTypeEfficiency  InsightType = "efficiency"
	InsightTypeReliability InsightType = "reliability"
	InsightTypeCost        InsightType = "cost"
)

// RecommendationEngine interface for generating optimization recommendations
type RecommendationEngine interface {
	GenerateRecommendations(ctx context.Context, analysis AnalysisResult) ([]Recommendation, error)
	PrioritizeRecommendations(recommendations []Recommendation) []Recommendation
	EstimateImpact(recommendation Recommendation) ImpactEstimate
	ValidateRecommendation(recommendation Recommendation) ValidationResult
}

// NewPerformanceOptimizer creates a new performance optimizer
func NewPerformanceOptimizer(config PerformanceConfig) *PerformanceOptimizer {
	return &PerformanceOptimizer{
		monitors:   make([]PerformanceMonitor, 0),
		benchmarks: make([]Benchmark, 0),
		analyzers:  make([]PerformanceAnalyzer, 0),
		optimizers: make([]Optimizer, 0),
		config:     config,
	}
}

// AddMonitor adds a performance monitor
func (po *PerformanceOptimizer) AddMonitor(monitor PerformanceMonitor) {
	po.mu.Lock()
	defer po.mu.Unlock()
	po.monitors = append(po.monitors, monitor)
}

// AddBenchmark adds a benchmark
func (po *PerformanceOptimizer) AddBenchmark(benchmark Benchmark) {
	po.mu.Lock()
	defer po.mu.Unlock()
	po.benchmarks = append(po.benchmarks, benchmark)
}

// AddAnalyzer adds a performance analyzer
func (po *PerformanceOptimizer) AddAnalyzer(analyzer PerformanceAnalyzer) {
	po.mu.Lock()
	defer po.mu.Unlock()
	po.analyzers = append(po.analyzers, analyzer)
}

// AddOptimizer adds an optimizer
func (po *PerformanceOptimizer) AddOptimizer(optimizer Optimizer) {
	po.mu.Lock()
	defer po.mu.Unlock()
	po.optimizers = append(po.optimizers, optimizer)
}

// OptimizePerformance performs comprehensive performance optimization
func (po *PerformanceOptimizer) OptimizePerformance(ctx context.Context, target string) (*PerformanceOptimizationResult, error) {
	result := &PerformanceOptimizationResult{
		Target:        target,
		StartTime:     time.Now(),
		Monitoring:    make(map[string]PerformanceMetrics),
		Benchmarks:    make(map[string]BenchmarkResult),
		Analysis:      make(map[string]AnalysisResult),
		Optimizations: make(map[string]OptimizationResult),
	}

	// Collect current performance metrics
	po.mu.RLock()
	monitors := po.monitors
	po.mu.RUnlock()

	for _, monitor := range monitors {
		metrics, err := monitor.GetMetrics(ctx)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Monitor failed: %v", err))
			continue
		}
		result.Monitoring[fmt.Sprintf("monitor_%d", len(result.Monitoring))] = metrics
	}

	// Run performance benchmarks
	po.mu.RLock()
	benchmarks := po.benchmarks
	po.mu.RUnlock()

	for _, benchmark := range benchmarks {
		benchConfig := BenchmarkConfig{
			Duration:    po.config.BenchmarkTimeout,
			Concurrency: runtime.NumCPU(),
			Targets:     []string{target},
		}

		benchResult, err := benchmark.Run(ctx, benchConfig)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Benchmark failed: %v", err))
			continue
		}
		result.Benchmarks[fmt.Sprintf("benchmark_%d", len(result.Benchmarks))] = benchResult
	}

	// Analyze performance data
	po.mu.RLock()
	analyzers := po.analyzers
	po.mu.RUnlock()

	allMetrics := make([]PerformanceMetrics, 0, len(result.Monitoring))
	for _, metrics := range result.Monitoring {
		allMetrics = append(allMetrics, metrics)
	}

	for _, analyzer := range analyzers {
		analysis, err := analyzer.Analyze(ctx, allMetrics)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Analysis failed: %v", err))
			continue
		}
		result.Analysis[fmt.Sprintf("analysis_%d", len(result.Analysis))] = analysis
	}

	// Execute registered optimizers when auto-optimization is enabled
	if po.config.EnableAutoOptimize {
		po.mu.RLock()
		optimizers := append([]Optimizer(nil), po.optimizers...)
		po.mu.RUnlock()

		if len(optimizers) > 0 {
			targetMetrics := po.selectOptimizationMetrics(allMetrics)
			optimizationTarget := OptimizationTarget{
				Type:      TargetTypeApplication,
				Component: target,
				Metrics:   targetMetrics,
			}

			for _, optimizer := range optimizers {
				typeID := optimizer.GetOptimizationType()
				optimization, err := optimizer.Optimize(ctx, optimizationTarget)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("optimizer %s failed: %v", typeID, err))
					continue
				}
				result.Optimizations[string(typeID)] = optimization
			}
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Calculate overall performance score
	result.PerformanceScore = po.calculatePerformanceScore(result)

	return result, nil
}

func (po *PerformanceOptimizer) selectOptimizationMetrics(metrics []PerformanceMetrics) PerformanceMetrics {
	if len(metrics) == 0 {
		return PerformanceMetrics{}
	}

	selected := metrics[0]
	for _, metric := range metrics[1:] {
		if metric.Timestamp.After(selected.Timestamp) {
			selected = metric
		}
	}

	return selected
}

// calculatePerformanceScore calculates overall performance score
func (po *PerformanceOptimizer) calculatePerformanceScore(result *PerformanceOptimizationResult) PerformanceScore {
	score := PerformanceScore{
		Breakdown: make(map[string]float64),
	}

	// Calculate component scores
	var responseTimeScore, throughputScore, reliabilityScore, efficiencyScore, scalabilityScore float64
	var count int

	for _, metrics := range result.Monitoring {
		// Response time score (lower is better)
		if metrics.ResponseTime > 0 {
			responseTimeScore += math.Max(0, 100-float64(metrics.ResponseTime.Milliseconds()))
			count++
		}

		// Throughput score (higher is better)
		if metrics.Throughput > 0 {
			throughputScore += math.Min(100, metrics.Throughput)
		}

		// Reliability score (lower error rate is better)
		reliabilityScore += math.Max(0, 100-metrics.ErrorRate*100)

		// Efficiency score based on resource utilization
		cpuEfficiency := math.Max(0, 100-metrics.CPUUsage.Usage)
		memoryEfficiency := math.Max(0, 100-metrics.MemoryUsage.Percentage)
		efficiencyScore += (cpuEfficiency + memoryEfficiency) / 2
	}

	if count > 0 {
		responseTimeScore /= float64(count)
		throughputScore /= float64(count)
		reliabilityScore /= float64(count)
		efficiencyScore /= float64(count)
		scalabilityScore = 75.0 // Default scalability score
	}

	score.ResponseTime = responseTimeScore
	score.Throughput = throughputScore
	score.Reliability = reliabilityScore
	score.Efficiency = efficiencyScore
	score.Scalability = scalabilityScore

	// Calculate overall score as weighted average
	score.Overall = (responseTimeScore*0.25 + throughputScore*0.25 + reliabilityScore*0.25 + efficiencyScore*0.15 + scalabilityScore*0.10)

	score.Breakdown["response_time"] = responseTimeScore
	score.Breakdown["throughput"] = throughputScore
	score.Breakdown["reliability"] = reliabilityScore
	score.Breakdown["efficiency"] = efficiencyScore
	score.Breakdown["scalability"] = scalabilityScore

	return score
}

// PerformanceOptimizationResult contains comprehensive optimization results
type PerformanceOptimizationResult struct {
	StartTime        time.Time
	EndTime          time.Time
	Monitoring       map[string]PerformanceMetrics
	Benchmarks       map[string]BenchmarkResult
	Analysis         map[string]AnalysisResult
	Optimizations    map[string]OptimizationResult
	Target           string
	Errors           []string
	PerformanceScore PerformanceScore
	Duration         time.Duration
}
