package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"

	"github.com/theory-cloud/tabletheory/v2"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
)

type fakeCostRepo struct {
	topUsers    []*repositories.WebSocketUserCostRanking
	topUsersErr error

	budgetByUser map[string]*repositories.BudgetStatus
	budgetErr    error

	createErrs []error
	created    []*models.WebSocketCostRecord
}

func (f *fakeCostRepo) GetTopCostlyUsers(_ context.Context, _ time.Time, _ time.Time, _ int) ([]*repositories.WebSocketUserCostRanking, error) {
	if f.topUsersErr != nil {
		return nil, f.topUsersErr
	}
	return f.topUsers, nil
}

func (f *fakeCostRepo) CheckBudgetLimits(_ context.Context, userID string) (*repositories.BudgetStatus, error) {
	if f.budgetErr != nil {
		return nil, f.budgetErr
	}
	if f.budgetByUser == nil {
		return &repositories.BudgetStatus{UserID: userID}, nil
	}
	if status, ok := f.budgetByUser[userID]; ok {
		return status, nil
	}
	return &repositories.BudgetStatus{UserID: userID}, nil
}

func (f *fakeCostRepo) Create(_ context.Context, record *models.WebSocketCostRecord) error {
	f.created = append(f.created, record)
	if len(f.createErrs) > 0 {
		err := f.createErrs[0]
		f.createErrs = f.createErrs[1:]
		return err
	}
	return nil
}

type fakeConnectionRepo struct {
	idleConnections  []models.WebSocketConnection
	idleErr          error
	staleConnections []models.WebSocketConnection
	staleErr         error

	deleteErrByConn map[string]error
	deleteSubsErr   error
	deleted         []string
}

func (f *fakeConnectionRepo) GetIdleConnections(_ context.Context, _ time.Time) ([]models.WebSocketConnection, error) {
	if f.idleErr != nil {
		return nil, f.idleErr
	}
	return f.idleConnections, nil
}

func (f *fakeConnectionRepo) GetStaleConnections(_ context.Context, _ time.Time) ([]models.WebSocketConnection, error) {
	if f.staleErr != nil {
		return nil, f.staleErr
	}
	return f.staleConnections, nil
}

func (f *fakeConnectionRepo) DeleteConnection(_ context.Context, connectionID string) error {
	f.deleted = append(f.deleted, connectionID)
	if f.deleteErrByConn != nil {
		if err, ok := f.deleteErrByConn[connectionID]; ok {
			return err
		}
	}
	return nil
}

func (f *fakeConnectionRepo) DeleteAllSubscriptions(_ context.Context, _ string) error {
	return f.deleteSubsErr
}

type fakeCostTracker struct {
	trackErrs []error
	aggErrs   []error

	trackedBatches int
	aggCalls       []aggCall
}

func (f *fakeCostTracker) TrackIdleConnections(_ context.Context, _ []models.WebSocketConnection) error {
	f.trackedBatches++
	if len(f.trackErrs) > 0 {
		err := f.trackErrs[0]
		f.trackErrs = f.trackErrs[1:]
		return err
	}
	return nil
}

type aggCall struct {
	period string
	start  time.Time
	end    time.Time
}

func (f *fakeCostTracker) PerformCostAggregation(_ context.Context, period string, start, end time.Time) error {
	f.aggCalls = append(f.aggCalls, aggCall{period: period, start: start, end: end})
	if len(f.aggErrs) > 0 {
		err := f.aggErrs[0]
		f.aggErrs = f.aggErrs[1:]
		return err
	}
	return nil
}

type fakeSNS struct {
	publishErr error
	inputs     []*sns.PublishInput
}

func (f *fakeSNS) Publish(_ context.Context, params *sns.PublishInput, _ ...func(*sns.Options)) (*sns.PublishOutput, error) {
	f.inputs = append(f.inputs, params)
	if f.publishErr != nil {
		return nil, f.publishErr
	}
	return &sns.PublishOutput{MessageId: aws.String("msg")}, nil
}

func TestWebSocketCostAggregator_Helpers(t *testing.T) {
	require.Equal(t, 3, severityToLevel("CRITICAL"))
	require.Equal(t, 2, severityToLevel("WARNING"))
	require.Equal(t, 1, severityToLevel("INFO"))
	require.Equal(t, 0, severityToLevel("other"))

	user := &repositories.WebSocketUserCostRanking{UserID: "u1", Username: "admin", TotalCostDollars: 1.23}

	exceeded := &repositories.BudgetStatus{ExceededBudgets: []string{"monthly"}}
	require.Equal(t, "CRITICAL", determineSeverity(exceeded))
	require.Contains(t, formatAlertMessage(user, exceeded), "exceeded budget limits")

	warn := &repositories.BudgetStatus{WarningBudgets: []string{"daily"}}
	require.Equal(t, "WARNING", determineSeverity(warn))
	require.Contains(t, formatAlertMessage(user, warn), "approaching budget limits")

	ok := &repositories.BudgetStatus{}
	require.Equal(t, "INFO", determineSeverity(ok))
	require.Contains(t, formatAlertMessage(user, ok), "Budget status update")

	handler := &WebSocketCostAggregatorHandler{webhookURL: "x"}
	require.Equal(t, 1, countConfiguredAlertMethods(handler))
	handler.snsClient = &fakeSNS{}
	handler.snsTopicArn = "arn:aws:sns:us-east-1:123:topic"
	require.Equal(t, 2, countConfiguredAlertMethods(handler))
	handler.webhookURL = ""
	require.Equal(t, 1, countConfiguredAlertMethods(handler))
}

func TestSendWebhookAlert_SuccessAndFailures(t *testing.T) {
	config.ResetForTests()
	t.Setenv("DOMAIN", "example.com")

	var captured *http.Request
	h := &WebSocketCostAggregatorHandler{
		logger:     zap.NewNop(),
		webhookURL: "https://example.com/webhook",
		httpClient: &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
			captured = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    req,
			}, nil
		}},
	}

	require.NoError(t, h.sendWebhookAlert(context.Background(), []byte(`{"ok":true}`)))
	require.NotNil(t, captured)
	require.Equal(t, "budget-alert", captured.Header.Get("X-Alert-Type"))
	require.Equal(t, "example.com", captured.Header.Get("X-Instance-Domain"))

	h.webhookURL = "://bad"
	require.Error(t, h.sendWebhookAlert(context.Background(), []byte(`{}`)))

	h.webhookURL = "https://example.com/webhook"
	h.httpClient = &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Request:    req,
		}, nil
	}}
	require.Error(t, h.sendWebhookAlert(context.Background(), []byte(`{}`)))
}

func TestSendWebhookAlert_ClientDoError(t *testing.T) {
	config.ResetForTests()
	t.Setenv("DOMAIN", "example.com")

	h := &WebSocketCostAggregatorHandler{
		logger:     zap.NewNop(),
		webhookURL: "https://example.com/webhook",
		httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) {
			return nil, errors.New("client error")
		}},
	}
	require.Error(t, h.sendWebhookAlert(context.Background(), []byte(`{"ok":true}`)))
}

func TestSendSNSAlert_SuccessAndFailures(t *testing.T) {
	config.ResetForTests()
	t.Setenv("DOMAIN", "example.com")

	snsClient := &fakeSNS{}
	h := &WebSocketCostAggregatorHandler{
		logger:      zap.NewNop(),
		snsClient:   snsClient,
		snsTopicArn: "arn:aws:sns:us-east-1:123:topic",
	}

	require.NoError(t, h.sendSNSAlert(context.Background(), []byte(`{"ok":true}`)))
	require.Len(t, snsClient.inputs, 1)
	require.Equal(t, "arn:aws:sns:us-east-1:123:topic", aws.ToString(snsClient.inputs[0].TopicArn))
	require.Equal(t, "Lesser WebSocket Budget Alert", aws.ToString(snsClient.inputs[0].Subject))
	require.Equal(t, "BudgetAlert", aws.ToString(snsClient.inputs[0].MessageAttributes["AlertType"].StringValue))

	snsClient.publishErr = errors.New("boom")
	require.Error(t, h.sendSNSAlert(context.Background(), []byte(`{"ok":true}`)))
}

func TestSendBudgetAlert_SNS_Success(t *testing.T) {
	config.ResetForTests()
	t.Setenv("DOMAIN", "example.com")

	snsClient := &fakeSNS{}
	costRepo := &fakeCostRepo{}
	h := &WebSocketCostAggregatorHandler{
		costRepo:    costRepo,
		logger:      zap.NewNop(),
		snsClient:   snsClient,
		snsTopicArn: "arn:aws:sns:us-east-1:123:topic",
	}

	user := &repositories.WebSocketUserCostRanking{UserID: "u1", Username: "admin", TotalCostDollars: 1.0}
	status := &repositories.BudgetStatus{UserID: "u1", ExceededBudgets: []string{"monthly"}}
	require.NoError(t, h.sendBudgetAlert(context.Background(), user, status))
	require.Len(t, snsClient.inputs, 1)
}

func TestTrackIdleConnections_BatchesAndErrorHandling(t *testing.T) {
	config.ResetForTests()
	t.Setenv("IDLE_TIMEOUT_MINUTES", "1")

	now := time.Now().UTC()
	connections := make([]models.WebSocketConnection, 0, 60)
	for i := 0; i < 60; i++ {
		connections = append(connections, models.WebSocketConnection{
			ConnectionID: "c" + string(rune('a'+i)),
			UserID:       "u1",
			Username:     "admin",
			LastActivity: now.Add(-5 * time.Minute),
		})
	}

	h := &WebSocketCostAggregatorHandler{
		costRepo:       &fakeCostRepo{},
		connectionRepo: &fakeConnectionRepo{idleConnections: connections},
		costTracker:    &fakeCostTracker{trackErrs: []error{errors.New("first batch failed"), nil}},
		logger:         zap.NewNop(),
	}

	require.NoError(t, h.trackIdleConnections(context.Background()))

	h.connectionRepo = &fakeConnectionRepo{idleErr: errors.New("query failed")}
	require.Error(t, h.trackIdleConnections(context.Background()))

	idleThreshold := now.Add(-2 * time.Minute)
	batchCost, err := h.trackIdleConnectionsBatch(context.Background(), []models.WebSocketConnection{
		{ConnectionID: "c1", LastActivity: now.Add(-10 * time.Minute)},
		{ConnectionID: "c2", LastActivity: now.Add(-time.Minute)},
	}, idleThreshold)
	require.NoError(t, err)
	require.GreaterOrEqual(t, batchCost, int64(0))
}

func TestCleanupStaleConnections_EmptyAndBatchFailures(t *testing.T) {
	config.ResetForTests()
	t.Setenv("STALE_TIMEOUT_HOURS", "1")

	connRepo := &fakeConnectionRepo{}
	h := &WebSocketCostAggregatorHandler{
		costRepo:       &fakeCostRepo{createErrs: []error{errors.New("create failed")}},
		connectionRepo: connRepo,
		costTracker:    &fakeCostTracker{},
		logger:         zap.NewNop(),
	}

	require.NoError(t, h.cleanupStaleConnections(context.Background()))

	connRepo.staleErr = errors.New("query failed")
	require.Error(t, h.cleanupStaleConnections(context.Background()))
	connRepo.staleErr = nil

	now := time.Now().UTC()
	connRepo.staleConnections = []models.WebSocketConnection{
		{ConnectionID: "c1", UserID: "u1", Username: "admin", LastActivity: now.Add(-300 * 24 * time.Hour)},
		{ConnectionID: "c2", Username: "anon", LastActivity: now.Add(-2 * time.Hour)},
	}
	connRepo.deleteErrByConn = map[string]error{"c1": errors.New("delete failed")}
	connRepo.deleteSubsErr = errors.New("delete subs failed")

	results := h.cleanupStaleConnectionsBatch(context.Background(), connRepo.staleConnections)
	require.Equal(t, 1, results.Cleaned)
	require.Equal(t, 1, results.Errors)
	require.GreaterOrEqual(t, results.ReclaimedCostMicroCents, int64(0))
}

func TestCleanupStaleConnections_SendsCleanupAlert(t *testing.T) {
	config.ResetForTests()
	t.Setenv("DOMAIN", "example.com")
	t.Setenv("STALE_TIMEOUT_HOURS", "1")

	type captured struct {
		header http.Header
		body   []byte
	}
	received := make(chan captured, 1)

	now := time.Now().UTC()
	connRepo := &fakeConnectionRepo{
		staleConnections: []models.WebSocketConnection{
			{ConnectionID: "c1", UserID: "u1", Username: "admin", LastActivity: now.Add(-300 * 24 * time.Hour)},
			{ConnectionID: "c2", Username: "anon", LastActivity: now.Add(-300 * 24 * time.Hour)},
		},
	}
	costRepo := &fakeCostRepo{createErrs: []error{nil, errors.New("create failed")}}
	snsClient := &fakeSNS{}
	h := &WebSocketCostAggregatorHandler{
		costRepo:       costRepo,
		connectionRepo: connRepo,
		costTracker:    &fakeCostTracker{},
		logger:         zap.NewNop(),
		webhookURL:     "https://example.com/webhook",
		httpClient: &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
			if req.Body != nil {
				body, _ := io.ReadAll(req.Body)
				_ = req.Body.Close()
				received <- captured{header: req.Header.Clone(), body: body}
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    req,
			}, nil
		}},
		snsTopicArn: "arn:aws:sns:us-east-1:123:topic",
		snsClient:   snsClient,
	}

	require.NoError(t, h.cleanupStaleConnections(context.Background()))
	require.Len(t, snsClient.inputs, 1)

	req := <-received
	require.Equal(t, "example.com", req.header.Get("X-Instance-Domain"))

	var alert CleanupAlertMessage
	require.NoError(t, json.Unmarshal(req.body, &alert))
	require.Equal(t, "CLEANUP_ALERT", alert.AlertType)
	require.Equal(t, 2, alert.CleanedConnections)
	require.Greater(t, alert.ReclaimedCostMicroCents, int64(100_000))
	require.Contains(t, alert.Message, "Cleaned up 2 stale WebSocket connections")
}

func TestSendCleanupAlert_LogsFailures(t *testing.T) {
	config.ResetForTests()
	t.Setenv("DOMAIN", "example.com")

	received := make(chan struct{}, 1)

	snsClient := &fakeSNS{publishErr: errors.New("sns failed")}
	h := &WebSocketCostAggregatorHandler{
		logger:     zap.NewNop(),
		webhookURL: "https://example.com/webhook",
		httpClient: &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
			received <- struct{}{}
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    req,
			}, nil
		}},
		snsTopicArn: "arn:aws:sns:us-east-1:123:topic",
		snsClient:   snsClient,
	}

	h.sendCleanupAlert(context.Background(), 1, 200_000)
	require.Len(t, snsClient.inputs, 1)

	select {
	case <-received:
	default:
		t.Fatal("expected webhook attempt")
	}
}

func TestBudgetAlerts_AndAllMethodsFailed(t *testing.T) {
	config.ResetForTests()
	t.Setenv("DOMAIN", "example.com")

	costRepo := &fakeCostRepo{
		topUsers: []*repositories.WebSocketUserCostRanking{
			{UserID: "u1", Username: "admin", TotalCostDollars: 1.0},
			{UserID: "u2", Username: "alice", TotalCostDollars: 2.0},
		},
		budgetByUser: map[string]*repositories.BudgetStatus{
			"u1": {UserID: "u1", ExceededBudgets: []string{"monthly"}},
			"u2": {UserID: "u2", WarningBudgets: []string{"daily"}},
		},
		createErrs: []error{errors.New("create failed")},
	}

	snsClient := &fakeSNS{publishErr: errors.New("sns failed")}

	h := &WebSocketCostAggregatorHandler{
		costRepo:       costRepo,
		connectionRepo: &fakeConnectionRepo{},
		costTracker:    &fakeCostTracker{},
		logger:         zap.NewNop(),
		webhookURL:     "https://example.com/webhook",
		httpClient: &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    req,
			}, nil
		}},
		snsTopicArn: "arn:aws:sns:us-east-1:123:topic",
		snsClient:   snsClient,
	}

	require.Error(t, h.sendBudgetAlert(context.Background(), costRepo.topUsers[0], costRepo.budgetByUser["u1"]))

	h.httpClient = &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Request:    req,
		}, nil
	}}
	require.NoError(t, h.sendBudgetAlert(context.Background(), costRepo.topUsers[0], costRepo.budgetByUser["u1"]))

	costRepo.topUsersErr = errors.New("query failed")
	require.Error(t, h.updateBudgetAlerts(context.Background()))
}

func TestUpdateBudgetAlerts_LogsSendBudgetAlertError(t *testing.T) {
	config.ResetForTests()
	t.Setenv("DOMAIN", "example.com")

	costRepo := &fakeCostRepo{
		topUsers: []*repositories.WebSocketUserCostRanking{
			{UserID: "u1", Username: "admin", TotalCostDollars: 1.0},
		},
		budgetByUser: map[string]*repositories.BudgetStatus{
			"u1": {UserID: "u1", ExceededBudgets: []string{"monthly"}},
		},
	}

	h := &WebSocketCostAggregatorHandler{
		costRepo:       costRepo,
		connectionRepo: &fakeConnectionRepo{},
		costTracker:    &fakeCostTracker{},
		logger:         zap.NewNop(),
		webhookURL:     "https://example.com/webhook",
		httpClient: &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    req,
			}, nil
		}},
		snsTopicArn: "arn:aws:sns:us-east-1:123:topic",
		snsClient:   &fakeSNS{publishErr: errors.New("sns failed")},
	}

	require.NoError(t, h.updateBudgetAlerts(context.Background()))
}

func TestUpdateBudgetAlerts_SendsOnlyWhenNeeded(t *testing.T) {
	config.ResetForTests()
	t.Setenv("DOMAIN", "example.com")

	received := make(chan []byte, 10)

	costRepo := &fakeCostRepo{
		topUsers: []*repositories.WebSocketUserCostRanking{
			{UserID: "u1", Username: "admin", TotalCostDollars: 1.0},
			{UserID: "u2", Username: "bob", TotalCostDollars: 0.1},
		},
		budgetByUser: map[string]*repositories.BudgetStatus{
			"u1": {UserID: "u1", ExceededBudgets: []string{"monthly"}},
			"u2": {UserID: "u2"},
		},
	}

	h := &WebSocketCostAggregatorHandler{
		costRepo:       costRepo,
		connectionRepo: &fakeConnectionRepo{},
		costTracker:    &fakeCostTracker{},
		logger:         zap.NewNop(),
		webhookURL:     "https://example.com/webhook",
		httpClient: &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
			if req.Body != nil {
				body, _ := io.ReadAll(req.Body)
				_ = req.Body.Close()
				received <- body
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    req,
			}, nil
		}},
	}

	require.NoError(t, h.updateBudgetAlerts(context.Background()))

	body := <-received
	var msg BudgetAlertMessage
	require.NoError(t, json.Unmarshal(body, &msg))
	require.Equal(t, "BUDGET_ALERT", msg.AlertType)
	require.Equal(t, "u1", msg.UserID)
	require.Equal(t, "CRITICAL", msg.Severity)

	select {
	case <-received:
		t.Fatal("expected only one alert")
	default:
	}
}

type httpDoerStub struct {
	doFn func(req *http.Request) (*http.Response, error)
}

func (d *httpDoerStub) Do(req *http.Request) (*http.Response, error) {
	return d.doFn(req)
}

func TestUpdateBudgetAlerts_ContinuesOnBudgetCheckError(t *testing.T) {
	costRepo := &fakeCostRepo{
		topUsers: []*repositories.WebSocketUserCostRanking{
			{UserID: "u1", Username: "admin", TotalCostDollars: 1.0},
		},
		budgetErr: errors.New("check failed"),
	}

	h := &WebSocketCostAggregatorHandler{
		costRepo:    costRepo,
		costTracker: &fakeCostTracker{},
		logger:      zap.NewNop(),
	}

	require.NoError(t, h.updateBudgetAlerts(context.Background()))
}

func TestHandleScheduledEvent_ParsesOperations(t *testing.T) {
	costRepo := &fakeCostRepo{topUsers: []*repositories.WebSocketUserCostRanking{}}
	connRepo := &fakeConnectionRepo{}
	h := &WebSocketCostAggregatorHandler{
		costRepo:       costRepo,
		connectionRepo: connRepo,
		costTracker:    &fakeCostTracker{},
		logger:         zap.NewNop(),
	}

	event := events.EventBridgeEvent{
		Source: "aws.events",
		Time:   time.Unix(0, 0).UTC(),
		Detail: json.RawMessage(`{"operations":["idle_tracking","cleanup","unknown",123]}`),
	}

	_, err := h.HandleScheduledEvent(&apptheory.EventContext{RequestID: "req"}, event)
	require.NoError(t, err)
}

func TestHandleScheduledEvent_CleanupErrorPath(t *testing.T) {
	h := &WebSocketCostAggregatorHandler{
		costRepo:       &fakeCostRepo{},
		connectionRepo: &fakeConnectionRepo{staleErr: errors.New("query failed")},
		costTracker:    &fakeCostTracker{},
		logger:         zap.NewNop(),
	}

	event := events.EventBridgeEvent{
		Source: "aws.events",
		Time:   time.Unix(0, 0).UTC(),
		Detail: json.RawMessage(`{"operations":["cleanup"]}`),
	}

	_, err := h.HandleScheduledEvent(&apptheory.EventContext{RequestID: "req"}, event)
	require.NoError(t, err)
}

func TestHandleScheduledEvent_DefaultOperations(t *testing.T) {
	originalTimeNow := timeNowFn
	t.Cleanup(func() { timeNowFn = originalTimeNow })

	now := time.Date(2020, 1, 2, 12, 0, 0, 0, time.UTC)
	timeNowFn = func() time.Time { return now }

	tracker := &fakeCostTracker{}
	h := &WebSocketCostAggregatorHandler{
		costRepo:       &fakeCostRepo{topUsersErr: errors.New("budget query failed")},
		connectionRepo: &fakeConnectionRepo{idleErr: errors.New("idle query failed")},
		costTracker:    tracker,
		logger:         zap.NewNop(),
	}

	event := events.EventBridgeEvent{
		Source: "aws.events",
		Time:   time.Unix(0, 0).UTC(),
	}

	_, err := h.HandleScheduledEvent(&apptheory.EventContext{RequestID: "req"}, event)
	require.NoError(t, err)

	require.Len(t, tracker.aggCalls, 1)
	require.Equal(t, "hour", tracker.aggCalls[0].period)
}

func TestPerformCostAggregation_HourlyAndDaily(t *testing.T) {
	originalTimeNow := timeNowFn
	t.Cleanup(func() { timeNowFn = originalTimeNow })

	t.Run("hourly only", func(t *testing.T) {
		tracker := &fakeCostTracker{aggErrs: []error{errors.New("hour failed")}}
		h := &WebSocketCostAggregatorHandler{
			costTracker: tracker,
			logger:      zap.NewNop(),
		}

		now := time.Date(2020, 1, 2, 15, 23, 45, 0, time.UTC)
		timeNowFn = func() time.Time { return now }

		require.NoError(t, h.performCostAggregation(context.Background()))
		require.Len(t, tracker.aggCalls, 1)
		require.Equal(t, "hour", tracker.aggCalls[0].period)
		require.Equal(t, time.Date(2020, 1, 2, 14, 0, 0, 0, time.UTC), tracker.aggCalls[0].start)
		require.Equal(t, time.Date(2020, 1, 2, 15, 0, 0, 0, time.UTC), tracker.aggCalls[0].end)
	})

	t.Run("hourly and daily", func(t *testing.T) {
		tracker := &fakeCostTracker{aggErrs: []error{errors.New("hour failed"), errors.New("day failed")}}
		h := &WebSocketCostAggregatorHandler{
			costTracker: tracker,
			logger:      zap.NewNop(),
		}

		now := time.Date(2020, 1, 2, 0, 5, 0, 0, time.UTC)
		timeNowFn = func() time.Time { return now }

		require.NoError(t, h.performCostAggregation(context.Background()))
		require.Len(t, tracker.aggCalls, 2)
		require.Equal(t, "hour", tracker.aggCalls[0].period)
		require.Equal(t, time.Date(2020, 1, 1, 23, 0, 0, 0, time.UTC), tracker.aggCalls[0].start)
		require.Equal(t, time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC), tracker.aggCalls[0].end)
		require.Equal(t, "day", tracker.aggCalls[1].period)
		require.Equal(t, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), tracker.aggCalls[1].start)
		require.Equal(t, time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC), tracker.aggCalls[1].end)
	})
}

func TestInitializeAndMain_RoutesEventBridge(t *testing.T) {
	originalMustInit := mustInitializeLambdaFn
	originalGetDB := getLambdaClientFn
	originalLoadAWS := loadAWSConfigFn
	originalLambdaStart := lambdaStartFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = originalMustInit
		getLambdaClientFn = originalGetDB
		loadAWSConfigFn = originalLoadAWS
		lambdaStartFn = originalLambdaStart
	})

	config.ResetForTests()
	t.Setenv("DOMAIN", "example.com")
	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	fakeCtx := &common.LambdaContext{
		Config:   &config.Config{DynamoTableName: "", Domain: "example.com"},
		Logger:   zap.NewNop(),
		Repos:    nil,
		DynamoDB: nil,
	}
	mustInitializeLambdaFn = func(_ common.LambdaConfig) *common.LambdaContext { return fakeCtx }
	getLambdaClientFn = func(context.Context) (core.DB, error) { return &tabletheory.LambdaDB{}, nil }
	loadAWSConfigFn = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}

	initializeWebSocketCostAggregator()
	require.NotNil(t, lambdaCtx)
	require.NotNil(t, handler)

	// Replace global handler with a safe fake for the request path.
	costRepo := &fakeCostRepo{}
	connRepo := &fakeConnectionRepo{}
	handler = &WebSocketCostAggregatorHandler{
		costRepo:       costRepo,
		connectionRepo: connRepo,
		costTracker:    &fakeCostTracker{},
		logger:         zap.NewNop(),
	}

	var started any
	lambdaStartFn = func(h any) { started = h }

	main()
	require.NotNil(t, started)

	handleRequest, ok := started.(func(context.Context, json.RawMessage) (any, error))
	require.True(t, ok)

	event := map[string]any{
		"source":      "aws.events",
		"detail-type": "Scheduled Event",
		"detail": map[string]any{
			"operations": []any{"idle_tracking"},
		},
		"resources": []any{
			"arn:aws:events:us-east-1:123456789012:rule/lesser-dev-websocket-cost-aggregator-schedule-0",
		},
		"time": time.Now().UTC().Format(time.RFC3339),
		"id":   "evt",
	}

	raw, err := json.Marshal(event)
	require.NoError(t, err)

	_, err = handleRequest(context.Background(), raw)
	require.NoError(t, err)

	_, err = handleRequest(context.Background(), json.RawMessage(`{"time":"not-a-time","resources":["arn:aws:events:us-east-1:123456789012:rule/lesser-dev-websocket-cost-aggregator-schedule-0"],"source":"aws.events","detail-type":"Scheduled Event","detail":{}}`))
	require.Error(t, err)
}

func TestInitializeWebSocketCostAggregator_LoadAWSConfigError(t *testing.T) {
	originalMustInit := mustInitializeLambdaFn
	originalGetDB := getLambdaClientFn
	originalLoadAWS := loadAWSConfigFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = originalMustInit
		getLambdaClientFn = originalGetDB
		loadAWSConfigFn = originalLoadAWS
	})

	config.ResetForTests()
	t.Setenv("DOMAIN", "example.com")

	fakeCtx := &common.LambdaContext{
		Config:   &config.Config{DynamoTableName: "", Domain: "example.com"},
		Logger:   zap.NewNop(),
		Repos:    nil,
		DynamoDB: nil,
	}
	mustInitializeLambdaFn = func(_ common.LambdaConfig) *common.LambdaContext { return fakeCtx }
	getLambdaClientFn = func(context.Context) (core.DB, error) { return &tabletheory.LambdaDB{}, nil }
	loadAWSConfigFn = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, errors.New("load failed")
	}

	initializeWebSocketCostAggregator()
	require.NotNil(t, handler)
	require.Nil(t, handler.snsClient)
}

func TestHandleScheduledEvent_WithNilContext(t *testing.T) {
	costRepo := &fakeCostRepo{topUsers: []*repositories.WebSocketUserCostRanking{}}
	connRepo := &fakeConnectionRepo{}
	h := &WebSocketCostAggregatorHandler{
		costRepo:       costRepo,
		connectionRepo: connRepo,
		costTracker:    &fakeCostTracker{},
		logger:         zap.NewNop(),
	}

	_, err := h.HandleScheduledEvent(nil, events.EventBridgeEvent{
		Source:     "aws.events",
		DetailType: "Scheduled Event",
		Detail:     json.RawMessage(`{"operations":["idle_tracking"]}`),
		Time:       time.Unix(0, 0).UTC(),
	})
	require.NoError(t, err)
}
