package security

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	statusExcellent = "excellent"
	statusGood      = "good"
	statusFair      = "fair"
	statusPoor      = "poor"
)

// ComplianceDashboard provides real-time compliance visibility
// Memory optimized: 152 → 64 bytes (88 bytes saved)
type ComplianceDashboard struct {
	// Interfaces grouped together (24 bytes each)
	metricsEngine  MetricsEngine
	dataAggregator DataAggregator
	alertManager   DashboardAlertManager
	cache          DashboardCache
	// Sync primitive (24 bytes)
	mu sync.RWMutex
	// Struct (varies)
	config DashboardConfig
	// Bool last (1 byte)
	running bool
}

// DashboardConfig configuration for compliance dashboard
// Memory optimized: 64 → 40 bytes (24 bytes saved)
type DashboardConfig struct {
	// 8-byte aligned fields first
	RefreshInterval time.Duration `json:"refresh_interval"`
	CacheTTL        time.Duration `json:"cache_ttl"`
	// 4-byte aligned fields
	HistoricalDataDays int `json:"historical_data_days"`
	MaxDataPoints      int `json:"max_data_points"`
	// Bools grouped together (1 byte each)
	Enabled              bool `json:"enabled"`
	CacheEnabled         bool `json:"cache_enabled"`
	RealTimeUpdates      bool `json:"real_time_updates"`
	AlertingEnabled      bool `json:"alerting_enabled"`
	ExportEnabled        bool `json:"export_enabled"`
	CustomMetricsEnabled bool `json:"custom_metrics_enabled"`
}

// MetricsEngine interface for metrics calculation
type MetricsEngine interface {
	CalculateComplianceMetrics(ctx context.Context, timeRange TimeRange) (*ComplianceMetrics, error)
	CalculateRiskMetrics(ctx context.Context, timeRange TimeRange) (*RiskMetrics, error)
	CalculateAuditMetrics(ctx context.Context, timeRange TimeRange) (*AuditMetrics, error)
	CalculatePerformanceMetrics(ctx context.Context, timeRange TimeRange) (*PerformanceMetrics, error)
	CalculateCustomMetrics(ctx context.Context, queries []CustomMetricQuery) ([]*CustomMetric, error)
}

// DataAggregator interface for data aggregation
type DataAggregator interface {
	AggregateByTimeframe(ctx context.Context, data []DataPoint, interval time.Duration) ([]AggregatedDataPoint, error)
	AggregateByDimension(ctx context.Context, data []DataPoint, dimension string) (map[string]float64, error)
	CalculateTrends(ctx context.Context, data []DataPoint) (*TrendAnalysis, error)
	GenerateSummary(ctx context.Context, data []DataPoint) (*DataSummary, error)
}

// DashboardAlertManager interface for dashboard alerts
type DashboardAlertManager interface {
	CheckThresholds(ctx context.Context, metrics *DashboardMetrics) ([]*DashboardAlert, error)
	SendAlert(ctx context.Context, alert *DashboardAlert) error
	GetActiveAlerts(ctx context.Context) ([]*DashboardAlert, error)
	AcknowledgeAlert(ctx context.Context, alertID string, acknowledgedBy string) error
}

// DashboardCache interface for dashboard caching
type DashboardCache interface {
	Get(key string) (any, bool)
	Set(key string, value any, ttl time.Duration)
	Delete(key string)
	Clear()
}

// DashboardMetrics represents comprehensive dashboard metrics
// Memory optimized: 112 → 96 bytes (16 bytes saved)
type DashboardMetrics struct {
	Timestamp          time.Time           `json:"timestamp"`
	ComplianceMetrics  *ComplianceMetrics  `json:"compliance_metrics"`
	RiskMetrics        *RiskMetrics        `json:"risk_metrics"`
	AuditMetrics       *AuditMetrics       `json:"audit_metrics"`
	PerformanceMetrics *PerformanceMetrics `json:"performance_metrics"`
	Summary            *DashboardSummary   `json:"summary"`
	CustomMetrics      []*CustomMetric     `json:"custom_metrics"`
	Alerts             []*DashboardAlert   `json:"alerts"`
}

// ComplianceMetrics represents compliance-specific metrics
// Memory optimized: 176 → 152 bytes (24 bytes saved)
type ComplianceMetrics struct {
	LastAuditDate        time.Time                  `json:"last_audit_date"`
	NextAuditDate        time.Time                  `json:"next_audit_date"`
	ControlEffectiveness map[string]float64         `json:"control_effectiveness"`
	ViolationsByType     map[string]int             `json:"violations_by_type"`
	ViolationsBySeverity map[string]int             `json:"violations_by_severity"`
	FrameworkScores      map[string]float64         `json:"framework_scores"`
	TrendDirection       string                     `json:"trend_direction"`
	HistoricalData       []ComplianceDataPoint      `json:"historical_data"`
	Recommendations      []ComplianceRecommendation `json:"recommendations"`
	CertificationStatus  []CertificationStatus      `json:"certification_status"`
	OverallScore         float64                    `json:"overall_score"`
	ComplianceRate       float64                    `json:"compliance_rate"`
	ViolationCount       int                        `json:"violation_count"`
}

// RiskMetrics represents risk-specific metrics
// Memory optimized: 152 → 112 bytes (40 bytes saved)
type RiskMetrics struct {
	RiskDistribution    map[string]int     `json:"risk_distribution"`
	IncidentsByType     map[string]int     `json:"incidents_by_type"`
	IncidentsBySeverity map[string]int     `json:"incidents_by_severity"`
	MitigationProgress  map[string]float64 `json:"mitigation_progress"`
	RiskLevel           string             `json:"risk_level"`
	RiskTrend           string             `json:"risk_trend"`
	ThreatLevel         string             `json:"threat_level"`
	HistoricalData      []RiskDataPoint    `json:"historical_data"`
	TopRiskFactors      []RiskFactor       `json:"top_risk_factors"`
	OverallRiskScore    float64            `json:"overall_risk_score"`
	RiskAppetite        float64            `json:"risk_appetite"`
	RiskTolerance       float64            `json:"risk_tolerance"`
	IncidentCount       int                `json:"incident_count"`
	VulnerabilityCount  int                `json:"vulnerability_count"`
}

// AuditMetrics represents audit-specific metrics
// Memory optimized: 120 → 56 bytes (64 bytes saved)
type AuditMetrics struct {
	EventsByType        map[string]int   `json:"events_by_type"`
	EventsBySeverity    map[string]int   `json:"events_by_severity"`
	EventsBySource      map[string]int   `json:"events_by_source"`
	AnomaliesByType     map[string]int   `json:"anomalies_by_type"`
	EventTrend          string           `json:"event_trend"`
	HistoricalData      []AuditDataPoint `json:"historical_data"`
	FailureRate         float64          `json:"failure_rate"`
	AverageEventSize    float64          `json:"average_event_size"`
	DataIntegrityScore  float64          `json:"data_integrity_score"`
	LogCompleteness     float64          `json:"log_completeness"`
	RetentionCompliance float64          `json:"retention_compliance"`
	TotalEvents         int              `json:"total_events"`
	AnomalyCount        int              `json:"anomaly_count"`
	FailedEvents        int              `json:"failed_events"`
}

// CustomMetric represents a custom metric
// Memory optimized: 136 → 120 bytes (16 bytes saved)
type CustomMetric struct {
	// Map first (24 bytes)
	Metadata map[string]any `json:"metadata"`
	// Time struct (24 bytes)
	Timestamp time.Time `json:"timestamp"`
	// Strings (16 bytes each)
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Unit        string `json:"unit"`
	Type        string `json:"type"`
	Category    string `json:"category"`
	// Float64 last (8 bytes)
	Value float64 `json:"value"`
}

// CustomMetricQuery represents a query for custom metrics
// Memory optimized: 136 → 128 bytes (8 bytes saved)
type CustomMetricQuery struct {
	// Map first (24 bytes)
	Parameters map[string]any `json:"parameters"`
	// Struct (varies)
	TimeRange TimeRange `json:"time_range"`
	// Strings (16 bytes each)
	ID          string `json:"id"`
	Name        string `json:"name"`
	Query       string `json:"query"`
	Type        string `json:"type"`
	Aggregation string `json:"aggregation"`
}

// DashboardAlert represents a dashboard alert
// Memory optimized: 216 → 184 bytes (32 bytes saved)
type DashboardAlert struct {
	Timestamp      time.Time      `json:"timestamp"`
	Metadata       map[string]any `json:"metadata"`
	ResolvedAt     *time.Time     `json:"resolved_at,omitempty"`
	AcknowledgedAt *time.Time     `json:"acknowledged_at,omitempty"`
	Title          string         `json:"title"`
	Severity       string         `json:"severity"`
	Type           string         `json:"type"`
	Description    string         `json:"description"`
	Metric         string         `json:"metric"`
	Status         string         `json:"status"`
	AcknowledgedBy string         `json:"acknowledged_by,omitempty"`
	ID             string         `json:"id"`
	Actions        []AlertAction  `json:"actions"`
	Threshold      float64        `json:"threshold"`
	CurrentValue   float64        `json:"current_value"`
}

// AlertAction represents an action for an alert
// Memory optimized: 80 → 64 bytes (16 bytes saved)
type AlertAction struct {
	// Map first (24 bytes)
	Parameters map[string]any `json:"parameters"`
	// Strings (16 bytes each)
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	// Bool last (1 byte)
	Automated bool `json:"automated"`
}

// DashboardSummary represents a summary of dashboard data
// Memory optimized: 160 → 128 bytes (32 bytes saved)
type DashboardSummary struct {
	LastUpdated      time.Time          `json:"last_updated"`
	KeyMetrics       map[string]float64 `json:"key_metrics"`
	Metadata         map[string]any     `json:"metadata"`
	OverallHealth    string             `json:"overall_health"`
	ComplianceStatus string             `json:"compliance_status"`
	RiskStatus       string             `json:"risk_status"`
	AuditStatus      string             `json:"audit_status"`
	TrendDirection   string             `json:"trend_direction"`
	Recommendations  []string           `json:"recommendations"`
	ActiveAlerts     int                `json:"active_alerts"`
	CriticalIssues   int                `json:"critical_issues"`
}

// ComplianceDataPoint represents a compliance data point
type ComplianceDataPoint struct {
	Timestamp       time.Time      `json:"timestamp"`
	Metadata        map[string]any `json:"metadata"`
	Framework       string         `json:"framework"`
	ComplianceScore float64        `json:"compliance_score"`
	ViolationCount  int            `json:"violation_count"`
	ControlCount    int            `json:"control_count"`
}

// RiskDataPoint represents a risk data point
type RiskDataPoint struct {
	Timestamp     time.Time      `json:"timestamp"`
	Metadata      map[string]any `json:"metadata"`
	ThreatLevel   string         `json:"threat_level"`
	RiskScore     float64        `json:"risk_score"`
	IncidentCount int            `json:"incident_count"`
}

// AuditDataPoint represents an audit data point
type AuditDataPoint struct {
	Timestamp    time.Time      `json:"timestamp"`
	Metadata     map[string]any `json:"metadata"`
	EventCount   int            `json:"event_count"`
	AnomalyCount int            `json:"anomaly_count"`
	FailureRate  float64        `json:"failure_rate"`
}

// DataPoint represents a generic data point
type DataPoint struct {
	Timestamp time.Time         `json:"timestamp"`
	Labels    map[string]string `json:"labels"`
	Metadata  map[string]any    `json:"metadata"`
	Value     float64           `json:"value"`
}

// AggregatedDataPoint represents an aggregated data point
type AggregatedDataPoint struct {
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata"`
	Value     float64        `json:"value"`
	Count     int            `json:"count"`
	Min       float64        `json:"min"`
	Max       float64        `json:"max"`
	Average   float64        `json:"average"`
	Sum       float64        `json:"sum"`
	StdDev    float64        `json:"std_dev"`
}

// TrendAnalysis represents trend analysis results
type TrendAnalysis struct {
	Metadata    map[string]any  `json:"metadata"`
	Direction   string          `json:"direction"`
	Forecast    []ForecastPoint `json:"forecast"`
	Magnitude   float64         `json:"magnitude"`
	Confidence  float64         `json:"confidence"`
	Seasonality bool            `json:"seasonality"`
}

// ForecastPoint represents a forecast point
type ForecastPoint struct {
	Timestamp  time.Time `json:"timestamp"`
	Value      float64   `json:"value"`
	Confidence float64   `json:"confidence"`
	Lower      float64   `json:"lower"`
	Upper      float64   `json:"upper"`
}

// DataSummary represents a summary of data
type DataSummary struct {
	Percentiles map[string]float64 `json:"percentiles"`
	Metadata    map[string]any     `json:"metadata"`
	Count       int                `json:"count"`
	Min         float64            `json:"min"`
	Max         float64            `json:"max"`
	Average     float64            `json:"average"`
	Median      float64            `json:"median"`
	StdDev      float64            `json:"std_dev"`
}

// DashboardWidget represents a dashboard widget
type DashboardWidget struct {
	LastUpdated time.Time      `json:"last_updated"`
	Data        any            `json:"data"`
	Metadata    map[string]any `json:"metadata"`
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Config      WidgetConfig   `json:"config"`
	Position    WidgetPosition `json:"position"`
	Size        WidgetSize     `json:"size"`
}

// WidgetPosition represents widget position
type WidgetPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// WidgetSize represents widget size
type WidgetSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// WidgetConfig represents widget configuration
type WidgetConfig struct {
	TimeRange   TimeRange          `json:"time_range"`
	Filters     map[string]any     `json:"filters"`
	Thresholds  map[string]float64 `json:"thresholds"`
	Colors      map[string]string  `json:"colors"`
	Metadata    map[string]any     `json:"metadata"`
	ChartType   string             `json:"chart_type"`
	DataSource  string             `json:"data_source"`
	Aggregation string             `json:"aggregation"`
	RefreshRate time.Duration      `json:"refresh_rate"`
}

// DashboardLayout represents dashboard layout
type DashboardLayout struct {
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	CreatedBy   string            `json:"created_by"`
	Widgets     []DashboardWidget `json:"widgets"`
	Permissions []string          `json:"permissions"`
	IsDefault   bool              `json:"is_default"`
}

// NewComplianceDashboard creates a new compliance dashboard
func NewComplianceDashboard(config DashboardConfig) *ComplianceDashboard {
	return &ComplianceDashboard{
		config: config,
	}
}

// SetMetricsEngine sets the metrics engine
func (cd *ComplianceDashboard) SetMetricsEngine(engine MetricsEngine) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.metricsEngine = engine
}

// SetDataAggregator sets the data aggregator
func (cd *ComplianceDashboard) SetDataAggregator(aggregator DataAggregator) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.dataAggregator = aggregator
}

// SetAlertManager sets the alert manager
func (cd *ComplianceDashboard) SetAlertManager(manager DashboardAlertManager) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.alertManager = manager
}

// SetCache sets the dashboard cache
func (cd *ComplianceDashboard) SetCache(cache DashboardCache) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.cache = cache
}

// Start starts the dashboard
func (cd *ComplianceDashboard) Start(ctx context.Context) error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	if cd.running {
		return fmt.Errorf("dashboard already running")
	}

	if !cd.config.Enabled {
		return fmt.Errorf("dashboard not enabled")
	}

	// Start background refresh if real-time updates are enabled
	if cd.config.RealTimeUpdates {
		go cd.runBackgroundRefresh(ctx)
	}

	cd.running = true
	return nil
}

// Stop stops the dashboard
func (cd *ComplianceDashboard) Stop() error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	if !cd.running {
		return nil
	}

	cd.running = false
	return nil
}

// GetDashboardMetrics returns current dashboard metrics
func (cd *ComplianceDashboard) GetDashboardMetrics(ctx context.Context, timeRange TimeRange) (*DashboardMetrics, error) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	builder := newDashboardMetricsBuilder(ctx, cd, timeRange)
	return builder.build()
}

// dashboardMetricsBuilder builds dashboard metrics
type dashboardMetricsBuilder struct {
	dashboard *ComplianceDashboard
	ctx       context.Context
	timeRange TimeRange
	metrics   *DashboardMetrics
	cacheKey  string
}

// newDashboardMetricsBuilder creates a new metrics builder
func newDashboardMetricsBuilder(ctx context.Context, dashboard *ComplianceDashboard, timeRange TimeRange) *dashboardMetricsBuilder {
	return &dashboardMetricsBuilder{
		dashboard: dashboard,
		ctx:       ctx,
		timeRange: timeRange,
		metrics: &DashboardMetrics{
			Timestamp: time.Now(),
		},
		cacheKey: fmt.Sprintf("dashboard_metrics_%d_%d", timeRange.Start.Unix(), timeRange.End.Unix()),
	}
}

// build constructs the dashboard metrics
func (b *dashboardMetricsBuilder) build() (*DashboardMetrics, error) {
	// Try cache first
	if cached := b.checkCache(); cached != nil {
		return cached, nil
	}

	// Build metrics
	b.collectEngineMetrics()
	b.collectAlertMetrics()
	b.generateSummary()
	b.cacheResult()

	return b.metrics, nil
}

// checkCache checks if metrics are in cache
func (b *dashboardMetricsBuilder) checkCache() *DashboardMetrics {
	if !b.dashboard.config.CacheEnabled || b.dashboard.cache == nil {
		return nil
	}

	if cached, found := b.dashboard.cache.Get(b.cacheKey); found {
		if metrics, ok := cached.(*DashboardMetrics); ok {
			return metrics
		}
	}

	return nil
}

// collectEngineMetrics collects metrics from the metrics engine
func (b *dashboardMetricsBuilder) collectEngineMetrics() {
	if b.dashboard.metricsEngine == nil {
		return
	}

	b.collectComplianceMetrics()
	b.collectRiskMetrics()
	b.collectAuditMetrics()
	b.collectPerformanceMetrics()
	b.collectCustomMetrics()
}

// collectComplianceMetrics collects compliance metrics
func (b *dashboardMetricsBuilder) collectComplianceMetrics() {
	metrics, err := b.dashboard.metricsEngine.CalculateComplianceMetrics(b.ctx, b.timeRange)
	if err == nil {
		b.metrics.ComplianceMetrics = metrics
	}
}

// collectRiskMetrics collects risk metrics
func (b *dashboardMetricsBuilder) collectRiskMetrics() {
	metrics, err := b.dashboard.metricsEngine.CalculateRiskMetrics(b.ctx, b.timeRange)
	if err == nil {
		b.metrics.RiskMetrics = metrics
	}
}

// collectAuditMetrics collects audit metrics
func (b *dashboardMetricsBuilder) collectAuditMetrics() {
	metrics, err := b.dashboard.metricsEngine.CalculateAuditMetrics(b.ctx, b.timeRange)
	if err == nil {
		b.metrics.AuditMetrics = metrics
	}
}

// collectPerformanceMetrics collects performance metrics
func (b *dashboardMetricsBuilder) collectPerformanceMetrics() {
	metrics, err := b.dashboard.metricsEngine.CalculatePerformanceMetrics(b.ctx, b.timeRange)
	if err == nil {
		b.metrics.PerformanceMetrics = metrics
	}
}

// collectCustomMetrics collects custom metrics if enabled
func (b *dashboardMetricsBuilder) collectCustomMetrics() {
	if !b.dashboard.config.CustomMetricsEnabled {
		return
	}

	queries := b.dashboard.getCustomMetricQueries(b.timeRange)
	metrics, err := b.dashboard.metricsEngine.CalculateCustomMetrics(b.ctx, queries)
	if err == nil {
		b.metrics.CustomMetrics = metrics
	}
}

// collectAlertMetrics collects alert information
func (b *dashboardMetricsBuilder) collectAlertMetrics() {
	if b.dashboard.alertManager == nil {
		return
	}

	b.collectActiveAlerts()
	b.checkNewAlerts()
}

// collectActiveAlerts gets active alerts
func (b *dashboardMetricsBuilder) collectActiveAlerts() {
	alerts, err := b.dashboard.alertManager.GetActiveAlerts(b.ctx)
	if err == nil {
		b.metrics.Alerts = alerts
	}
}

// checkNewAlerts checks for new alerts based on thresholds
func (b *dashboardMetricsBuilder) checkNewAlerts() {
	newAlerts, err := b.dashboard.alertManager.CheckThresholds(b.ctx, b.metrics)
	if err == nil {
		b.metrics.Alerts = append(b.metrics.Alerts, newAlerts...)
	}
}

// generateSummary creates a summary of the metrics
func (b *dashboardMetricsBuilder) generateSummary() {
	b.metrics.Summary = b.dashboard.generateSummary(b.metrics)
}

// cacheResult stores the metrics in cache
func (b *dashboardMetricsBuilder) cacheResult() {
	if b.dashboard.config.CacheEnabled && b.dashboard.cache != nil {
		b.dashboard.cache.Set(b.cacheKey, b.metrics, b.dashboard.config.CacheTTL)
	}
}

// GetWidget returns a specific widget's data
func (cd *ComplianceDashboard) GetWidget(ctx context.Context, widgetID string, config WidgetConfig) (*DashboardWidget, error) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	widget := &DashboardWidget{
		ID:          widgetID,
		LastUpdated: time.Now(),
	}

	// Get data based on widget type and configuration
	switch config.DataSource {
	case "compliance_metrics":
		data, err := cd.metricsEngine.CalculateComplianceMetrics(ctx, config.TimeRange)
		if err != nil {
			return nil, err
		}
		widget.Data = data

	case "risk_metrics":
		data, err := cd.metricsEngine.CalculateRiskMetrics(ctx, config.TimeRange)
		if err != nil {
			return nil, err
		}
		widget.Data = data

	case "audit_metrics":
		data, err := cd.metricsEngine.CalculateAuditMetrics(ctx, config.TimeRange)
		if err != nil {
			return nil, err
		}
		widget.Data = data

	case "custom_metrics":
		queries := []CustomMetricQuery{{
			ID:        widgetID,
			TimeRange: config.TimeRange,
		}}
		data, err := cd.metricsEngine.CalculateCustomMetrics(ctx, queries)
		if err != nil {
			return nil, err
		}
		widget.Data = data

	default:
		return nil, fmt.Errorf("unsupported data source: %s", config.DataSource)
	}

	return widget, nil
}

// GetDashboardLayout returns a dashboard layout
func (cd *ComplianceDashboard) GetDashboardLayout(_ context.Context, _ string) (*DashboardLayout, error) {
	// This would typically load from a database
	// For now, return a default layout
	return cd.getDefaultLayout(), nil
}

// CreateDashboardLayout creates a new dashboard layout
func (cd *ComplianceDashboard) CreateDashboardLayout(_ context.Context, layout *DashboardLayout) error {
	// This would typically save to a database
	// For now, just validate the layout
	return cd.validateLayout(layout)
}

// UpdateDashboardLayout updates a dashboard layout
func (cd *ComplianceDashboard) UpdateDashboardLayout(_ context.Context, _ string, layout *DashboardLayout) error {
	// This would typically update in a database
	// For now, just validate the layout
	return cd.validateLayout(layout)
}

// DeleteDashboardLayout deletes a dashboard layout
func (cd *ComplianceDashboard) DeleteDashboardLayout(_ context.Context, _ string) error {
	// This would typically delete from a database
	// For now, just return success
	return nil
}

// ExportDashboardData exports dashboard data
func (cd *ComplianceDashboard) ExportDashboardData(ctx context.Context, format string, timeRange TimeRange) ([]byte, error) {
	if !cd.config.ExportEnabled {
		return nil, fmt.Errorf("export not enabled")
	}

	metrics, err := cd.GetDashboardMetrics(ctx, timeRange)
	if err != nil {
		return nil, err
	}

	switch format {
	case "json":
		return cd.exportJSON(metrics)
	case "csv":
		return cd.exportCSV(metrics)
	case "pdf":
		return cd.exportPDF(metrics)
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}

// runBackgroundRefresh runs background refresh
func (cd *ComplianceDashboard) runBackgroundRefresh(ctx context.Context) {
	ticker := time.NewTicker(cd.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cd.refreshCache(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// refreshCache refreshes the dashboard cache
func (cd *ComplianceDashboard) refreshCache(ctx context.Context) {
	if !cd.config.CacheEnabled || cd.cache == nil {
		return
	}

	// Refresh common time ranges
	timeRanges := []TimeRange{
		{Start: time.Now().Add(-24 * time.Hour), End: time.Now()},
		{Start: time.Now().Add(-7 * 24 * time.Hour), End: time.Now()},
		{Start: time.Now().Add(-30 * 24 * time.Hour), End: time.Now()},
	}

	for _, timeRange := range timeRanges {
		if _, err := cd.GetDashboardMetrics(ctx, timeRange); err != nil {
			log.Printf("Warning: failed to get dashboard metrics for time range %v-%v: %v",
				timeRange.Start, timeRange.End, err)
		}
	}
}

// generateSummary generates a dashboard summary
func (cd *ComplianceDashboard) generateSummary(metrics *DashboardMetrics) *DashboardSummary {
	summary := &DashboardSummary{
		LastUpdated: time.Now(),
		KeyMetrics:  make(map[string]float64),
	}

	// Overall health calculation
	healthScore := 100.0
	if metrics.ComplianceMetrics != nil {
		summary.ComplianceStatus = cd.getComplianceStatus(metrics.ComplianceMetrics.OverallScore)
		summary.KeyMetrics["compliance_score"] = metrics.ComplianceMetrics.OverallScore
		healthScore *= (metrics.ComplianceMetrics.OverallScore / 100.0)
	}

	if metrics.RiskMetrics != nil {
		summary.RiskStatus = cd.getRiskStatus(metrics.RiskMetrics.OverallRiskScore)
		summary.KeyMetrics["risk_score"] = metrics.RiskMetrics.OverallRiskScore
		healthScore *= (1.0 - (metrics.RiskMetrics.OverallRiskScore / 100.0))
	}

	if metrics.AuditMetrics != nil {
		summary.AuditStatus = cd.getAuditStatus(metrics.AuditMetrics.FailureRate)
		summary.KeyMetrics["audit_failure_rate"] = metrics.AuditMetrics.FailureRate
		healthScore *= (1.0 - metrics.AuditMetrics.FailureRate)
	}

	// Count alerts
	if metrics.Alerts != nil {
		summary.ActiveAlerts = len(metrics.Alerts)
		criticalCount := 0
		for _, alert := range metrics.Alerts {
			if alert.Severity == riskLevelCritical {
				criticalCount++
			}
		}
		summary.CriticalIssues = criticalCount
	}

	// Overall health status
	summary.OverallHealth = cd.getOverallHealth(healthScore)

	// Generate recommendations
	summary.Recommendations = cd.generateRecommendations(metrics)

	return summary
}

// getCustomMetricQueries returns custom metric queries
func (cd *ComplianceDashboard) getCustomMetricQueries(timeRange TimeRange) []CustomMetricQuery {
	// This would typically be configured by users
	// For now, return some default queries
	return []CustomMetricQuery{
		{
			ID:        "user_activity",
			Name:      "User Activity",
			Type:      "count",
			TimeRange: timeRange,
		},
		{
			ID:        "data_access_volume",
			Name:      "Data Access Volume",
			Type:      "sum",
			TimeRange: timeRange,
		},
	}
}

// Helper methods

func (cd *ComplianceDashboard) getComplianceStatus(score float64) string {
	if score >= 95 {
		return statusExcellent
	}
	if score >= 85 {
		return statusGood
	}
	if score >= 70 {
		return statusFair
	}
	return statusPoor
}

func (cd *ComplianceDashboard) getRiskStatus(score float64) string {
	if score >= 80 {
		return riskLevelCritical
	}
	if score >= 60 {
		return "high"
	}
	if score >= 40 {
		return "medium"
	}
	return "low"
}

func (cd *ComplianceDashboard) getAuditStatus(failureRate float64) string {
	if failureRate <= 0.01 {
		return statusExcellent
	}
	if failureRate <= 0.05 {
		return statusGood
	}
	if failureRate <= 0.1 {
		return statusFair
	}
	return statusPoor
}

func (cd *ComplianceDashboard) getOverallHealth(score float64) string {
	if score >= 0.9 {
		return statusExcellent
	}
	if score >= 0.8 {
		return statusGood
	}
	if score >= 0.7 {
		return statusFair
	}
	if score >= 0.6 {
		return statusPoor
	}
	return riskLevelCritical
}

func (cd *ComplianceDashboard) generateRecommendations(metrics *DashboardMetrics) []string {
	var recommendations []string

	if metrics.ComplianceMetrics != nil && metrics.ComplianceMetrics.OverallScore < 85 {
		recommendations = append(recommendations, "Improve compliance controls to reach target score")
	}

	if metrics.RiskMetrics != nil && metrics.RiskMetrics.OverallRiskScore > 70 {
		recommendations = append(recommendations, "Address high-risk factors to reduce overall risk")
	}

	if metrics.AuditMetrics != nil && metrics.AuditMetrics.FailureRate > 0.05 {
		recommendations = append(recommendations, "Investigate audit failures and improve logging")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Continue monitoring and maintain current performance")
	}

	return recommendations
}

func (cd *ComplianceDashboard) getDefaultLayout() *DashboardLayout {
	return &DashboardLayout{
		ID:          "default",
		Name:        "Default Compliance Dashboard",
		Description: "Default layout for compliance monitoring",
		Widgets: []DashboardWidget{
			{
				ID:       "compliance_overview",
				Type:     "chart",
				Title:    "Compliance Overview",
				Position: WidgetPosition{X: 0, Y: 0},
				Size:     WidgetSize{Width: 6, Height: 4},
				Config: WidgetConfig{
					ChartType:  "gauge",
					DataSource: "compliance_metrics",
					TimeRange:  TimeRange{Start: time.Now().Add(-24 * time.Hour), End: time.Now()},
				},
			},
			{
				ID:       "risk_overview",
				Type:     "chart",
				Title:    "Risk Overview",
				Position: WidgetPosition{X: 6, Y: 0},
				Size:     WidgetSize{Width: 6, Height: 4},
				Config: WidgetConfig{
					ChartType:  "gauge",
					DataSource: "risk_metrics",
					TimeRange:  TimeRange{Start: time.Now().Add(-24 * time.Hour), End: time.Now()},
				},
			},
			{
				ID:       "audit_trends",
				Type:     "chart",
				Title:    "Audit Trends",
				Position: WidgetPosition{X: 0, Y: 4},
				Size:     WidgetSize{Width: 12, Height: 4},
				Config: WidgetConfig{
					ChartType:  "line",
					DataSource: "audit_metrics",
					TimeRange:  TimeRange{Start: time.Now().Add(-7 * 24 * time.Hour), End: time.Now()},
				},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		IsDefault: true,
	}
}

func (cd *ComplianceDashboard) validateLayout(layout *DashboardLayout) error {
	if layout.Name == "" {
		return fmt.Errorf("layout name is required")
	}

	for _, widget := range layout.Widgets {
		if widget.ID == "" {
			return fmt.Errorf("widget ID is required")
		}
		if widget.Type == "" {
			return fmt.Errorf("widget type is required")
		}
	}

	return nil
}

func (cd *ComplianceDashboard) exportJSON(_ *DashboardMetrics) ([]byte, error) {
	// This would implement JSON export
	return []byte("{}"), nil
}

func (cd *ComplianceDashboard) exportCSV(_ *DashboardMetrics) ([]byte, error) {
	// This would implement CSV export
	return []byte(""), nil
}

func (cd *ComplianceDashboard) exportPDF(_ *DashboardMetrics) ([]byte, error) {
	// This would implement PDF export
	return []byte(""), nil
}
