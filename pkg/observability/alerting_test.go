package observability

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormtesting "github.com/theory-cloud/tabletheory/pkg/testing"
	"go.uber.org/zap/zaptest"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

type snsPublishStub struct {
	inputs []*sns.PublishInput
	out    *sns.PublishOutput
	err    error
}

func (s *snsPublishStub) Publish(_ context.Context, params *sns.PublishInput, _ ...func(*sns.Options)) (*sns.PublishOutput, error) {
	s.inputs = append(s.inputs, params)
	if s.out != nil {
		return s.out, s.err
	}
	return &sns.PublishOutput{MessageId: aws.String("mid")}, s.err
}

func TestNewAlertingSystem_ValidatesConfig(t *testing.T) {
	testDB := dynamormtesting.NewTestDB()

	_, err := NewAlertingSystem(&AlertingConfig{Logger: nil, DB: testDB.MockDB})
	require.Error(t, err)

	_, err = NewAlertingSystem(&AlertingConfig{Logger: zaptest.NewLogger(t), DB: nil})
	require.Error(t, err)
}

func TestAlertingSystem_SendAlert_DisabledNoop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sys := &AlertingSystem{logger: logger, enabled: false}
	err := sys.SendAlert(context.Background(), &AlertRequest{Type: "t", Title: "x"})
	require.NoError(t, err)
}

func TestAlertingSystem_SendAlert_StoresAndOptionallySNS(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	testDB := dynamormtesting.NewTestDB()
	// Allow creates for alerts; multiple calls in this test.
	testDB.MockQuery.On("Create").Return(nil).Maybe()

	snsStub := &snsPublishStub{}

	sys, err := NewAlertingSystem(&AlertingConfig{
		Logger:      logger,
		DB:          testDB.MockDB,
		TableName:   "table",
		SNSClient:   snsStub,
		SNSTopicArn: "arn:aws:sns:us-east-1:123:topic",
		Environment: "test",
		Region:      "us-east-1",
		ServiceName: "lesser",
		Enabled:     true,
	})
	require.NoError(t, err)

	// Keep the test focused on alerting; avoid webhook repo interactions.
	sys.webhookDelivery.enabled = false

	longTitle := strings.Repeat("x", 200)
	err = sys.SendAlert(ctx, &AlertRequest{
		Type:        "error_rate",
		Severity:    "critical",
		Priority:    "P1",
		Title:       longTitle,
		Description: "d",
		Service:     "api",
		Region:      "us-east-1",
		Source:      "test",
	})
	require.NoError(t, err)

	require.Len(t, snsStub.inputs, 1)
	subject := aws.ToString(snsStub.inputs[0].Subject)
	require.LessOrEqual(t, len(subject), 100)
	assert.True(t, strings.HasSuffix(subject, "..."))
	assert.Equal(t, "arn:aws:sns:us-east-1:123:topic", aws.ToString(snsStub.inputs[0].TopicArn))
	assert.Contains(t, snsStub.inputs[0].MessageAttributes, "AlertType")

	// Cover no-op when SNS is not configured.
	sys.snsClient = nil
	sys.snsTopicArn = ""
	err = sys.SendAlert(ctx, &AlertRequest{
		Type:        "latency",
		Severity:    "critical",
		Priority:    "P0",
		Title:       "t",
		Description: "d",
		Service:     "api",
		Region:      "us-east-1",
		Source:      "test",
	})
	require.NoError(t, err)
}

func TestAlertingSystem_HelperMethods(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sys := &AlertingSystem{logger: logger, environment: "env", region: "us-east-1", serviceName: "lesser"}

	alert := sys.createAlertFromRequest(&AlertRequest{
		Type:        "t",
		Severity:    "critical",
		Priority:    "P0",
		Title:       "title",
		Description: "desc",
		Service:     "svc",
		Region:      "us-east-1",
		Source:      "src",
	})
	require.NotNil(t, alert)
	assert.Equal(t, "title", alert.Message)
	assert.Equal(t, "env", alert.Dimensions["environment"])

	msg := sys.formatSNSMessage(alert)
	assert.Contains(t, msg, "Alert:")

	assert.Equal(t, HealthStatusCritical, sys.getSeverityForErrorRate(AlertP0ErrorRatePercent))
	assert.Equal(t, "P0", sys.getPriorityForErrorRate(AlertP0ErrorRatePercent))
	assert.Equal(t, HealthStatusWarning, sys.getSeverityForErrorRate(AlertP2ErrorRatePercent))
	assert.Equal(t, AlertSeverityInfo, sys.getSeverityForErrorRate(0))
	assert.Equal(t, "P3", sys.getPriorityForErrorRate(0))

	assert.Equal(t, HealthStatusCritical, sys.getSeverityForLatency(AlertP0LatencyP99Milliseconds))
	assert.Equal(t, HealthStatusWarning, sys.getSeverityForLatency(AlertP2LatencyP90Milliseconds))
	assert.Equal(t, "info", sys.getSeverityForLatency(0))
	assert.Equal(t, "P3", sys.getPriorityForLatency(0))

	assert.Equal(t, AlertSeverityCritical, sys.getSeverityForCost(1000001))
	assert.Equal(t, "P2", sys.getPriorityForCost(10001))
	assert.Equal(t, "info", sys.getSeverityForCost(0))
	assert.Equal(t, "P3", sys.getPriorityForCost(0))

	assert.Equal(t, "P0", sys.getPriorityForSecurity("critical"))
	assert.Equal(t, "P1", sys.getPriorityForSecurity(AlertSeverityError))
	assert.Equal(t, "P2", sys.getPriorityForSecurity("warning"))
	assert.Equal(t, "P3", sys.getPriorityForSecurity("other"))
}

func TestAlertingSystem_ProcessRetries_BasicFlow(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	testDB := dynamormtesting.NewTestDB()
	snsStub := &snsPublishStub{}

	sys, err := NewAlertingSystem(&AlertingConfig{
		Logger:      logger,
		DB:          testDB.MockDB,
		TableName:   "table",
		SNSClient:   snsStub,
		SNSTopicArn: "arn:aws:sns:us-east-1:123:topic",
		Environment: "test",
		Region:      "us-east-1",
		ServiceName: "lesser",
		Enabled:     true,
	})
	require.NoError(t, err)

	sys.webhookDelivery.enabled = false

	// Repo expectations for GetAlertsNeedingRetry.
	testDB.ExpectIndex(models.IndexGSI3).
		ExpectWhere("gsi3PK", "=", "STATUS#firing")
	testDB.MockQuery.On("Filter", "DeliveryAttempts", "<", 5).Return(testDB.MockQuery).Once()
	testDB.MockQuery.On("Filter", "NextRetryAt", "<=", mock.Anything).Return(testDB.MockQuery).Once()
	testDB.ExpectOrderBy("gsi3SK", "ASC").
		ExpectLimit(50)

	past := time.Now().Add(-time.Minute)
	raw := []*models.Alert{{AlertID: "a1", DeliveryAttempts: 0, NextRetryAt: &past}}
	testDB.MockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Alert)
		*dest = raw
	}).Return(nil).Once()

	require.NoError(t, sys.ProcessRetries(ctx))
}

func TestAlertingSystem_SendSNSAlert_ErrorIsWrapped(t *testing.T) {
	logger := zaptest.NewLogger(t)
	snsStub := &snsPublishStub{err: errors.New("boom")}
	sys := &AlertingSystem{
		logger:      logger,
		snsClient:   snsStub,
		snsTopicArn: "arn",
	}

	err := sys.sendSNSAlert(context.Background(), &models.Alert{
		AlertID:    "a1",
		Type:       "t",
		Severity:   "critical",
		Priority:   "P0",
		Service:    "svc",
		Title:      "title",
		FiredAt:    time.Now(),
		Metadata:   map[string]interface{}{"k": "v"},
		RunbookURL: "https://example.com",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSNSPublishFailed)
}

func TestAlertingSystem_CheckMethods(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	testDB := dynamormtesting.NewTestDB()
	testDB.MockQuery.On("Create").Return(nil).Maybe()

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

	sys.webhookDelivery.enabled = false

	sys.CheckErrorRate(ctx, "api", AlertP1ErrorRatePercent)
	sys.CheckLatency(ctx, "api", "op", AlertP1LatencyP90Milliseconds, 0, 0)
	sys.CheckCost(ctx, 20000)
	sys.CheckHealth(ctx, "api", false, "unhealthy")
	sys.CheckSecurity(ctx, "event", "warning", nil)

	stats, err := sys.GetAlertStats(ctx, time.Now())
	require.NoError(t, err)
	require.NotNil(t, stats)
}

func TestAlertingSystem_ResolveAndCleanup(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	t.Run("resolve_alert_delegates", func(t *testing.T) {
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

		// Expectations for alertRepo.ResolveAlert -> GetByID + Update.
		testDB.ExpectWhere("PK", "=", "ALERT#a1").
			ExpectWhere("SK", "=", "METADATA")
		testDB.MockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Alert)
			*dest = models.Alert{AlertID: "a1", PK: "ALERT#a1", SK: "METADATA", Status: "firing"}
		}).Return(nil).Once()
		testDB.MockQuery.On("Update", []string(nil)).Return(nil).Once()

		require.NoError(t, sys.ResolveAlert(ctx, "a1"))
		testDB.AssertExpectations(t)
	})

	t.Run("cleanup_calls_repository", func(t *testing.T) {
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

		// Loosely allow the underlying cleanup query chain; return one alert once.
		testDB.MockQuery.On("Index", mock.Anything).Return(testDB.MockQuery).Maybe()
		testDB.MockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(testDB.MockQuery).Maybe()
		testDB.MockQuery.On("Limit", mock.Anything).Return(testDB.MockQuery).Maybe()

		allCalls := 0
		testDB.MockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Alert)
			if allCalls == 0 {
				*dest = []*models.Alert{{AlertID: "a1", PK: "ALERT#a1", SK: "METADATA"}}
			} else {
				*dest = nil
			}
			allCalls++
		}).Return(nil).Maybe()

		testDB.MockQuery.On("Delete").Return(nil).Maybe()

		require.NoError(t, sys.Cleanup(ctx))
	})
}
