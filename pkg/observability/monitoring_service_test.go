package observability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormtesting "github.com/theory-cloud/tabletheory/v2/pkg/testing"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	awsinit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

func TestNewMonitoringService_Basics(t *testing.T) {
	logger := zaptest.NewLogger(t)
	testDB := dynamormtesting.NewTestDB()

	_, err := NewMonitoringService(&MonitoringServiceConfig{Logger: nil})
	require.Error(t, err)

	ms, err := NewMonitoringService(&MonitoringServiceConfig{
		Logger:      logger,
		DB:          testDB.MockDB,
		TableName:   "table",
		Environment: "test",
		ServiceName: "svc",
		Region:      "us-east-1",
		Enabled:     true,
	})
	require.NoError(t, err)
	require.NotNil(t, ms.alertingSystem)
	require.NotNil(t, ms.latencyAlerter)
	require.NotNil(t, ms.defaultRoute)

	// Cover pass-through helpers without invoking storage.
	ms.alertingSystem.enabled = false
	ms.CheckErrorRate(context.Background(), "api", 99)
	ms.CheckLatency(context.Background(), "api", "op", 99, 0, 0)
	ms.CheckCost(context.Background(), 20000)
	ms.CheckHealth(context.Background(), "api", false, "unhealthy")
	ms.CheckSecurity(context.Background(), "event", "warning", nil)
	require.NoError(t, ms.ProcessRetries(context.Background()))
}

func TestMonitoringService_RouteMatchingAndSendAlert(t *testing.T) {
	logger := zaptest.NewLogger(t)

	alerting := &AlertingSystem{logger: logger, enabled: false}
	ms := &MonitoringService{
		logger:         logger,
		alertingSystem: alerting,
		enabled:        true,
		alertRoutes:    make(map[string]*AlertRoute),
		defaultRoute:   &AlertRoute{Name: "default", Enabled: true},
	}

	ms.AddAlertRoute(&AlertRoute{
		Name:           "only_latency",
		Enabled:        true,
		AlertTypes:     []string{"latency"},
		SeverityLevels: []string{"critical"},
		Services:       []string{"api"},
		Priorities:     []string{"P0"},
		Schedule: &ScheduleConfig{
			BusinessHoursOnly: true,
		},
	})

	route := ms.findMatchingRoute(&AlertRequest{Type: "latency", Severity: "critical", Service: "api", Priority: "P0"})
	require.NotNil(t, route)
	assert.Equal(t, "only_latency", route.Name)
	assert.False(t, ms.routeMatches(route, &AlertRequest{Type: "error_rate"}))

	// Schedule suppression is deterministic via monitoringTimeNow override.
	origNow := monitoringTimeNow
	monitoringTimeNow = func() time.Time { return time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { monitoringTimeNow = origNow })

	err := ms.SendAlert(context.Background(), &AlertRequest{
		Type:        "latency",
		Severity:    "critical",
		Priority:    "P0",
		Title:       "t",
		Description: "d",
		Service:     "api",
	})
	require.NoError(t, err)

	// Enabled route but no suppression -> calls into alerting system (disabled there, so no-op).
	monitoringTimeNow = func() time.Time { return time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC) }
	route.Schedule = nil
	route.Throttle = &ThrottleConfig{MaxAlertsPerMinute: 1, MaxAlertsPerHour: 1, CooldownPeriod: time.Minute}
	err = ms.SendAlert(context.Background(), &AlertRequest{
		Type:        "latency",
		Severity:    "critical",
		Priority:    "P0",
		Title:       "t",
		Description: "d",
		Service:     "api",
	})
	require.NoError(t, err)

	ms.enabled = false
	require.NoError(t, ms.SendAlert(context.Background(), &AlertRequest{Type: "t", Title: "x"}))

	ms.enabled = true
	ms.RemoveAlertRoute("only_latency")
	require.NoError(t, ms.SendAlert(context.Background(), &AlertRequest{Type: "t", Title: "x"}))
}

func TestMonitoringService_RouteMatches_AllCriteriaBranches(t *testing.T) {
	ms := &MonitoringService{}
	route := &AlertRoute{
		AlertTypes:     []string{"latency"},
		SeverityLevels: []string{"critical"},
		Services:       []string{"api"},
		Priorities:     []string{"P0"},
	}

	assert.True(t, ms.routeMatches(route, &AlertRequest{Type: "latency", Severity: "critical", Service: "api", Priority: "P0"}))
	assert.False(t, ms.routeMatches(route, &AlertRequest{Type: "latency", Severity: "warning", Service: "api", Priority: "P0"}))
	assert.False(t, ms.routeMatches(route, &AlertRequest{Type: "latency", Severity: "critical", Service: "federation", Priority: "P0"}))
	assert.False(t, ms.routeMatches(route, &AlertRequest{Type: "latency", Severity: "critical", Service: "api", Priority: "P2"}))
}

func TestMonitoringService_ShouldSuppressAlertBySchedule_FalseDuringBusinessHours(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ms := &MonitoringService{
		logger:         logger,
		alertingSystem: &AlertingSystem{logger: logger, enabled: false},
		enabled:        true,
		alertRoutes:    make(map[string]*AlertRoute),
	}

	origNow := monitoringTimeNow
	monitoringTimeNow = func() time.Time { return time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { monitoringTimeNow = origNow })

	suppress := ms.shouldSuppressAlertBySchedule(&AlertRequest{Type: "latency"}, &ScheduleConfig{BusinessHoursOnly: true})
	assert.False(t, suppress)
}

func TestMonitoringService_SendAlert_NoEnabledRouteOrDefault(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ms := &MonitoringService{
		logger:         logger,
		alertingSystem: &AlertingSystem{logger: logger, enabled: false},
		enabled:        true,
		alertRoutes: map[string]*AlertRoute{
			"disabled": {Name: "disabled", Enabled: false, AlertTypes: []string{"latency"}},
		},
		defaultRoute: nil,
	}

	require.NoError(t, ms.SendAlert(context.Background(), &AlertRequest{Type: "latency", Title: "x"}))
}

func TestNewMonitoringServiceFromEnv_UsesCentralConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	testDB := dynamormtesting.NewTestDB()

	global := config.Get()
	origTable := global.DynamoTableName
	origSNSTopic := global.AlertSNSTopicArn
	origWebhook := global.AlertWebhookURL
	origMonitoring := global.MonitoringEnabled
	origEnvironment := global.Environment
	origService := global.ServiceName
	origRegion := global.Region
	t.Cleanup(func() {
		global.DynamoTableName = origTable
		global.AlertSNSTopicArn = origSNSTopic
		global.AlertWebhookURL = origWebhook
		global.MonitoringEnabled = origMonitoring
		global.Environment = origEnvironment
		global.ServiceName = origService
		global.Region = origRegion
	})

	global.DynamoTableName = "lesser-table"
	global.AlertSNSTopicArn = "arn:aws:sns:us-east-1:123:topic"
	global.AlertWebhookURL = "https://example.com/hook"
	global.MonitoringEnabled = true
	global.Environment = "test"
	global.ServiceName = "api"
	global.Region = "us-east-1"

	origAPI := monitoringAPIServiceConfig
	origInit := monitoringInitializeServices
	t.Cleanup(func() {
		monitoringAPIServiceConfig = origAPI
		monitoringInitializeServices = origInit
	})

	monitoringAPIServiceConfig = func() awsinit.ServiceConfig {
		return awsinit.ServiceConfig{ServiceName: "api", Region: "us-east-1"}
	}

	var sawConfig awsinit.ServiceConfig
	monitoringInitializeServices = func(_ context.Context, cfg awsinit.ServiceConfig, _ *zap.Logger) (*awsinit.AWSServices, error) {
		sawConfig = cfg
		return &awsinit.AWSServices{SNS: nil}, nil
	}

	ms, err := NewMonitoringServiceFromEnv(logger, testDB.MockDB)
	require.NoError(t, err)
	require.NotNil(t, ms)
	assert.True(t, sawConfig.RequiresSNS)
	assert.NotEmpty(t, ms.alertRoutes)
	assert.True(t, ms.enabled)
}

func TestNewMonitoringServiceFromEnv_ReturnsErrorWhenAWSInitFails(t *testing.T) {
	logger := zaptest.NewLogger(t)
	testDB := dynamormtesting.NewTestDB()

	origInit := monitoringInitializeServices
	t.Cleanup(func() {
		monitoringInitializeServices = origInit
	})

	monitoringInitializeServices = func(_ context.Context, _ awsinit.ServiceConfig, _ *zap.Logger) (*awsinit.AWSServices, error) {
		return nil, errors.New("init failed")
	}

	_, err := NewMonitoringServiceFromEnv(logger, testDB.MockDB)
	require.Error(t, err)
}

func TestMonitoringService_DefaultRoutesHelpers(t *testing.T) {
	route := createDefaultAlertRoute("https://example.com", "arn")
	require.NotNil(t, route)
	assert.NotEmpty(t, route.WebhookURLs)
	assert.NotEmpty(t, route.SNSTopicARNs)

	routes := createDefaultAlertRoutes()
	require.Len(t, routes, 3)
}

func TestMonitoringService_GetActiveAlerts_ConvertsToSummaries(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	testDB := dynamormtesting.NewTestDB()

	testDB.ExpectIndex(models.IndexGSI3).
		ExpectWhere("gsi3PK", "=", "STATUS#firing").
		ExpectOrderBy("gsi3SK", "DESC").
		ExpectLimit(2)

	raw := []*models.Alert{
		{AlertID: "a1", Type: "t", Severity: "critical", Priority: "P0", Status: "firing", Title: "x", Service: "svc", FiredAt: time.Now()},
		{AlertID: "a2", Type: "t2", Severity: "warning", Priority: "P2", Status: "firing", Title: "y", Service: "svc", FiredAt: time.Now()},
	}

	testDB.MockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Alert)
		*dest = raw
	}).Return(nil).Once()

	alertRepo := NewStandaloneAlertRepository(testDB.MockDB, "table", logger, nil)
	alerting := &AlertingSystem{alertRepo: alertRepo}

	ms := &MonitoringService{alertingSystem: alerting}
	summaries, err := ms.GetActiveAlerts(ctx, 2)
	require.NoError(t, err)
	require.Len(t, summaries, 2)
	assert.Equal(t, "a1", summaries[0].AlertID)
	testDB.AssertExpectations(t)
}

func TestMonitoringService_GetActiveAlerts_PropagatesError(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	testDB := dynamormtesting.NewTestDB()

	testDB.ExpectIndex(models.IndexGSI3).
		ExpectWhere("gsi3PK", "=", "STATUS#firing").
		ExpectOrderBy("gsi3SK", "DESC").
		ExpectLimit(1)

	testDB.MockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

	alertRepo := NewStandaloneAlertRepository(testDB.MockDB, "table", logger, nil)
	alerting := &AlertingSystem{alertRepo: alertRepo}
	ms := &MonitoringService{alertingSystem: alerting}

	summaries, err := ms.GetActiveAlerts(ctx, 1)
	require.Error(t, err)
	require.Nil(t, summaries)
	testDB.AssertExpectations(t)
}

func TestMonitoringService_Cleanup_DelegatesToAlertingSystem(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	testDB := dynamormtesting.NewTestDB()

	sys, err := NewAlertingSystem(&AlertingConfig{
		Logger:      logger,
		DB:          testDB.MockDB,
		TableName:   "table",
		Environment: "test",
		Region:      "us-east-1",
		ServiceName: "lesser",
		Enabled:     true,
	})
	require.NoError(t, err)

	// Loosely allow cleanup query chain; return empty for all alert types.
	testDB.MockQuery.On("Index", mock.Anything).Return(testDB.MockQuery).Maybe()
	testDB.MockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(testDB.MockQuery).Maybe()
	testDB.MockQuery.On("Limit", mock.Anything).Return(testDB.MockQuery).Maybe()
	testDB.MockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Alert)
		*dest = nil
	}).Return(nil).Maybe()
	testDB.MockQuery.On("Delete").Return(nil).Maybe()

	ms := &MonitoringService{alertingSystem: sys}
	require.NoError(t, ms.Cleanup(ctx))
}
