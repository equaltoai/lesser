// Package observability provides monitoring service configuration for alert routing and management
package observability

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"

	awsinit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// MonitoringService provides centralized monitoring and alerting configuration
type MonitoringService struct {
	logger         *zap.Logger
	alertingSystem *AlertingSystem
	latencyAlerter *LatencyAlerter
	enabled        bool

	// Configuration
	environment string
	serviceName string
	region      string

	// Alert routing configuration
	alertRoutes  map[string]*AlertRoute
	defaultRoute *AlertRoute
}

// AlertRoute defines how alerts should be routed based on criteria
type AlertRoute struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`

	// Matching criteria
	AlertTypes     []string `json:"alert_types,omitempty"`     // error_rate, latency, cost, etc.
	SeverityLevels []string `json:"severity_levels,omitempty"` // critical, error, warning, info
	Services       []string `json:"services,omitempty"`        // api, federation, etc.
	Priorities     []string `json:"priorities,omitempty"`      // P0, P1, P2, P3

	// Delivery configuration
	WebhookURLs    []string `json:"webhook_urls"`
	SNSTopicARNs   []string `json:"sns_topic_arns,omitempty"`
	EmailAddresses []string `json:"email_addresses,omitempty"`
	SlackChannels  []string `json:"slack_channels,omitempty"`

	// Timing and throttling
	Throttle        *ThrottleConfig   `json:"throttle,omitempty"`
	Schedule        *ScheduleConfig   `json:"schedule,omitempty"`
	EscalationRules []*EscalationRule `json:"escalation_rules,omitempty"`
}

// ThrottleConfig controls alert throttling
type ThrottleConfig struct {
	MaxAlertsPerMinute int           `json:"max_alerts_per_minute"`
	MaxAlertsPerHour   int           `json:"max_alerts_per_hour"`
	CooldownPeriod     time.Duration `json:"cooldown_period"`
	GroupBy            []string      `json:"group_by"` // Fields to group alerts by for throttling
}

// ScheduleConfig controls when alerts are sent
type ScheduleConfig struct {
	BusinessHoursOnly bool     `json:"business_hours_only"`
	Timezone          string   `json:"timezone"`
	BusinessHours     string   `json:"business_hours"`  // e.g., "9:00-17:00"
	BusinessDays      []string `json:"business_days"`   // e.g., ["mon", "tue", "wed", "thu", "fri"]
	SuppressDuring    []string `json:"suppress_during"` // Maintenance windows
}

// EscalationRule defines alert escalation behavior
type EscalationRule struct {
	Level              int           `json:"level"`               // 1, 2, 3, etc.
	TriggerAfter       time.Duration `json:"trigger_after"`       // Escalate after this duration
	AdditionalChannels []string      `json:"additional_channels"` // Additional delivery channels
	RequiresManualAck  bool          `json:"requires_manual_ack"` // Requires manual acknowledgment
}

// MonitoringServiceConfig contains configuration for the monitoring service
type MonitoringServiceConfig struct {
	Logger      *zap.Logger
	DB          core.DB
	TableName   string
	CostService *cost.TrackingService

	// AWS services
	SNSClient   *sns.Client
	SNSTopicArn string

	// Webhook configuration
	WebhookURL     string
	WebhookHeaders map[string]string

	// Service identification
	Environment string
	ServiceName string
	Region      string

	// Alert routing
	AlertRoutes  []*AlertRoute
	DefaultRoute *AlertRoute

	Enabled bool
}

// NewMonitoringService creates a new monitoring service with comprehensive configuration
func NewMonitoringService(monitoringConfig *MonitoringServiceConfig) (*MonitoringService, error) {
	if monitoringConfig.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Set defaults from centralized config
	globalCfg := config.Get()
	if monitoringConfig.Environment == "" {
		monitoringConfig.Environment = globalCfg.Environment
	}
	if monitoringConfig.ServiceName == "" {
		monitoringConfig.ServiceName = globalCfg.ServiceName
	}
	if monitoringConfig.Region == "" {
		monitoringConfig.Region = globalCfg.Region
	}

	// Create alerting system
	alertingConfig := &AlertingConfig{
		Logger:         monitoringConfig.Logger,
		DB:             monitoringConfig.DB,
		TableName:      monitoringConfig.TableName,
		CostService:    monitoringConfig.CostService,
		SNSClient:      monitoringConfig.SNSClient,
		SNSTopicArn:    monitoringConfig.SNSTopicArn,
		WebhookURL:     monitoringConfig.WebhookURL,
		WebhookHeaders: monitoringConfig.WebhookHeaders,
		Environment:    monitoringConfig.Environment,
		Region:         monitoringConfig.Region,
		ServiceName:    monitoringConfig.ServiceName,
		Enabled:        monitoringConfig.Enabled,
	}

	alertingSystem, err := NewAlertingSystem(alertingConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create alerting system: %w", err)
	}

	// Create latency alerter
	createMetricFn := func(_ context.Context, metric *models.MetricRecord) error {
		monitoringConfig.Logger.Debug("storing monitoring metric",
			zap.String("metric_type", metric.MetricType),
			zap.String("service_name", metric.ServiceName))
		return nil
	}
	metricsRecorder := NewDefaultMetricsRecorder(createMetricFn, monitoringConfig.ServiceName)
	latencyAlerter := NewLatencyAlerter(monitoringConfig.Logger, metricsRecorder)

	// Set up alert routes
	alertRoutes := make(map[string]*AlertRoute)
	for _, route := range monitoringConfig.AlertRoutes {
		alertRoutes[route.Name] = route
	}

	// Create default route if not provided
	defaultRoute := monitoringConfig.DefaultRoute
	if defaultRoute == nil {
		defaultRoute = createDefaultAlertRoute(monitoringConfig.WebhookURL, monitoringConfig.SNSTopicArn)
	}

	ms := &MonitoringService{
		logger:         monitoringConfig.Logger,
		alertingSystem: alertingSystem,
		latencyAlerter: latencyAlerter,
		enabled:        monitoringConfig.Enabled,
		environment:    monitoringConfig.Environment,
		serviceName:    monitoringConfig.ServiceName,
		region:         monitoringConfig.Region,
		alertRoutes:    alertRoutes,
		defaultRoute:   defaultRoute,
	}

	return ms, nil
}

// NewMonitoringServiceFromEnv creates a monitoring service from centralized configuration
func NewMonitoringServiceFromEnv(logger *zap.Logger, db core.DB) (*MonitoringService, error) {
	// Initialize AWS services
	awsConfig := awsinit.APIServiceConfig()
	awsConfig.RequiresSNS = true
	awsServices, err := awsinit.InitializeServices(context.Background(), awsConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AWS services: %w", err)
	}

	// Get centralized configuration
	globalCfg := config.Get()

	monitoringConfig := &MonitoringServiceConfig{
		Logger:      logger,
		DB:          db,
		TableName:   globalCfg.DynamoTableName,
		SNSClient:   awsServices.SNS,
		SNSTopicArn: globalCfg.AlertSNSTopicArn,
		WebhookURL:  globalCfg.AlertWebhookURL,
		WebhookHeaders: map[string]string{
			"Content-Type": "application/json",
		},
		Enabled: globalCfg.MonitoringEnabled,
	}

	// Add default alert routes
	monitoringConfig.AlertRoutes = createDefaultAlertRoutes()

	return NewMonitoringService(monitoringConfig)
}

// SendAlert routes an alert based on configured rules
func (ms *MonitoringService) SendAlert(ctx context.Context, alertReq *AlertRequest) error {
	if !ms.enabled {
		ms.logger.Debug("monitoring service disabled, skipping alert",
			zap.String("type", alertReq.Type),
			zap.String("title", alertReq.Title))
		return nil
	}

	// Find matching route
	route := ms.findMatchingRoute(alertReq)
	if route == nil {
		route = ms.defaultRoute
	}

	if route == nil || !route.Enabled {
		ms.logger.Debug("no enabled route found for alert",
			zap.String("type", alertReq.Type),
			zap.String("severity", alertReq.Severity),
			zap.String("service", alertReq.Service))
		return nil
	}

	// Check throttling
	if route.Throttle != nil && ms.shouldThrottleAlert(alertReq, route.Throttle) {
		ms.logger.Debug("alert throttled",
			zap.String("type", alertReq.Type),
			zap.String("route", route.Name))
		return nil
	}

	// Check schedule
	if route.Schedule != nil && ms.shouldSuppressAlertBySchedule(alertReq, route.Schedule) {
		ms.logger.Debug("alert suppressed by schedule",
			zap.String("type", alertReq.Type),
			zap.String("route", route.Name))
		return nil
	}

	// Send alert through alerting system
	err := ms.alertingSystem.SendAlert(ctx, alertReq)
	if err != nil {
		ms.logger.Error("failed to send alert through alerting system",
			zap.String("type", alertReq.Type),
			zap.String("route", route.Name),
			zap.Error(err))
		return err
	}

	ms.logger.Info("alert routed successfully",
		zap.String("type", alertReq.Type),
		zap.String("severity", alertReq.Severity),
		zap.String("route", route.Name),
		zap.String("service", alertReq.Service))

	return nil
}

// CheckErrorRate monitors error rates and sends alerts
func (ms *MonitoringService) CheckErrorRate(ctx context.Context, service string, errorRate float64) {
	ms.alertingSystem.CheckErrorRate(ctx, service, errorRate)
}

// CheckLatency monitors latency and sends alerts
func (ms *MonitoringService) CheckLatency(ctx context.Context, service, operation string, latencyMs, p95Ms, p99Ms float64) {
	ms.alertingSystem.CheckLatency(ctx, service, operation, latencyMs, p95Ms, p99Ms)
	ms.latencyAlerter.CheckLatency(ctx, operation, service, latencyMs, p95Ms, p99Ms)
}

// CheckCost monitors costs and sends alerts
func (ms *MonitoringService) CheckCost(ctx context.Context, costMicroCents float64) {
	ms.alertingSystem.CheckCost(ctx, costMicroCents)
}

// CheckHealth monitors service health and sends alerts
func (ms *MonitoringService) CheckHealth(ctx context.Context, service string, isHealthy bool, errorMsg string) {
	ms.alertingSystem.CheckHealth(ctx, service, isHealthy, errorMsg)
}

// CheckSecurity monitors security events and sends alerts
func (ms *MonitoringService) CheckSecurity(ctx context.Context, eventType, severity string, details map[string]interface{}) {
	ms.alertingSystem.CheckSecurity(ctx, eventType, severity, details)
}

// ProcessRetries processes failed alert deliveries
func (ms *MonitoringService) ProcessRetries(ctx context.Context) error {
	return ms.alertingSystem.ProcessRetries(ctx)
}

// GetActiveAlerts retrieves currently active alerts
func (ms *MonitoringService) GetActiveAlerts(ctx context.Context, limit int) ([]*AlertSummary, error) {
	alerts, err := ms.alertingSystem.GetActiveAlerts(ctx, limit)
	if err != nil {
		return nil, err
	}

	summaries := make([]*AlertSummary, len(alerts))
	for i, alert := range alerts {
		summaries[i] = &AlertSummary{
			AlertID:    alert.AlertID,
			Type:       alert.Type,
			Severity:   alert.Severity,
			Priority:   alert.Priority,
			Status:     alert.Status,
			Title:      alert.Title,
			Service:    alert.Service,
			FiredAt:    alert.FiredAt,
			RunbookURL: alert.RunbookURL,
		}
	}

	return summaries, nil
}

// Cleanup performs maintenance tasks
func (ms *MonitoringService) Cleanup(ctx context.Context) error {
	return ms.alertingSystem.Cleanup(ctx)
}

// AddAlertRoute adds a new alert route
func (ms *MonitoringService) AddAlertRoute(route *AlertRoute) {
	ms.alertRoutes[route.Name] = route
	ms.logger.Info("added alert route",
		zap.String("route_name", route.Name),
		zap.Bool("enabled", route.Enabled))
}

// RemoveAlertRoute removes an alert route
func (ms *MonitoringService) RemoveAlertRoute(routeName string) {
	delete(ms.alertRoutes, routeName)
	ms.logger.Info("removed alert route", zap.String("route_name", routeName))
}

// Helper methods

func (ms *MonitoringService) findMatchingRoute(alertReq *AlertRequest) *AlertRoute {
	for _, route := range ms.alertRoutes {
		if ms.routeMatches(route, alertReq) {
			return route
		}
	}
	return nil
}

func (ms *MonitoringService) routeMatches(route *AlertRoute, alertReq *AlertRequest) bool {
	// Check alert type
	if len(route.AlertTypes) > 0 && !stringInSlice(alertReq.Type, route.AlertTypes) {
		return false
	}

	// Check severity
	if len(route.SeverityLevels) > 0 && !stringInSlice(alertReq.Severity, route.SeverityLevels) {
		return false
	}

	// Check service
	if len(route.Services) > 0 && !stringInSlice(alertReq.Service, route.Services) {
		return false
	}

	// Check priority
	if len(route.Priorities) > 0 && !stringInSlice(alertReq.Priority, route.Priorities) {
		return false
	}

	return true
}

// stringInSlice checks if a string is in a slice of strings
func stringInSlice(str string, slice []string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func (ms *MonitoringService) shouldThrottleAlert(_ *AlertRequest, _ *ThrottleConfig) bool {
	// Simplified throttling logic - in practice, you'd implement proper rate limiting
	// with a cache or database to track alert counts
	return false
}

func (ms *MonitoringService) shouldSuppressAlertBySchedule(_ *AlertRequest, schedule *ScheduleConfig) bool {
	// Simplified schedule checking - in practice, you'd implement proper time zone
	// and business hours checking
	if schedule.BusinessHoursOnly {
		now := time.Now()
		hour := now.Hour()
		// Simple business hours check (9 AM to 5 PM)
		if hour < 9 || hour >= 17 {
			return true
		}
	}
	return false
}

// Default configurations

func createDefaultAlertRoute(webhookURL, snsTopicArn string) *AlertRoute {
	route := &AlertRoute{
		Name:        "default",
		Description: "Default alert route for all alerts",
		Enabled:     true,
		AlertTypes:  []string{}, // Match all types
		WebhookURLs: []string{},
		Throttle: &ThrottleConfig{
			MaxAlertsPerMinute: 10,
			MaxAlertsPerHour:   100,
			CooldownPeriod:     5 * time.Minute,
			GroupBy:            []string{"service", "type"},
		},
	}

	if webhookURL != "" {
		route.WebhookURLs = append(route.WebhookURLs, webhookURL)
	}

	if snsTopicArn != "" {
		route.SNSTopicARNs = append(route.SNSTopicARNs, snsTopicArn)
	}

	return route
}

func createDefaultAlertRoutes() []*AlertRoute {
	return []*AlertRoute{
		{
			Name:        "critical_alerts",
			Description: "Route for critical priority alerts",
			Enabled:     true,
			Priorities:  []string{"P0"},
			WebhookURLs: []string{},
			Throttle: &ThrottleConfig{
				MaxAlertsPerMinute: 5,
				MaxAlertsPerHour:   20,
				CooldownPeriod:     2 * time.Minute,
			},
			EscalationRules: []*EscalationRule{
				{
					Level:              1,
					TriggerAfter:       5 * time.Minute,
					AdditionalChannels: []string{"sms", "phone"},
					RequiresManualAck:  true,
				},
			},
		},
		{
			Name:           "high_priority_alerts",
			Description:    "Route for high priority alerts",
			Enabled:        true,
			Priorities:     []string{"P1"},
			SeverityLevels: []string{"error", "critical"},
			WebhookURLs:    []string{},
			Throttle: &ThrottleConfig{
				MaxAlertsPerMinute: 10,
				MaxAlertsPerHour:   50,
				CooldownPeriod:     5 * time.Minute,
			},
		},
		{
			Name:           "business_hours_only",
			Description:    "Route for non-critical alerts during business hours",
			Enabled:        true,
			Priorities:     []string{"P2", "P3"},
			SeverityLevels: []string{"warning", "info"},
			WebhookURLs:    []string{},
			Schedule: &ScheduleConfig{
				BusinessHoursOnly: true,
				Timezone:          "UTC",
				BusinessHours:     "9:00-17:00",
				BusinessDays:      []string{"mon", "tue", "wed", "thu", "fri"},
			},
			Throttle: &ThrottleConfig{
				MaxAlertsPerMinute: 5,
				MaxAlertsPerHour:   30,
				CooldownPeriod:     10 * time.Minute,
			},
		},
	}
}

// AlertSummary provides a summary view of an alert
type AlertSummary struct {
	AlertID    string    `json:"alert_id"`
	Type       string    `json:"type"`
	Severity   string    `json:"severity"`
	Priority   string    `json:"priority"`
	Status     string    `json:"status"`
	Title      string    `json:"title"`
	Service    string    `json:"service"`
	FiredAt    time.Time `json:"fired_at"`
	RunbookURL string    `json:"runbook_url,omitempty"`
}
