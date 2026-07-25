package main

import (
	"context"
	"testing"
	"time"

	apiHandlers "github.com/equaltoai/lesser/cmd/api/handlers"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/observability"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

type mainTestRepos struct {
	*apiHandlers.MockRepositoryStorage
	account      *repositories.AccountRepository
	metricRecord *repositories.MetricRecordRepository
}

func (r *mainTestRepos) Account() *repositories.AccountRepository { return r.account }
func (r *mainTestRepos) MetricRecord() *repositories.MetricRecordRepository {
	return r.metricRecord
}

func newMainTestRepos(t *testing.T) *mainTestRepos {
	t.Helper()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdate := new(mocks.MockUpdateBuilder)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("UpdateBuilder").Return(mockUpdate).Maybe()
	mockUpdate.On("Set", mock.Anything, mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("Execute").Return(nil).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *models.User:
			dest.Username = "alice"
			dest.Role = "user"
			dest.Approved = true
			dest.Version = 1
		}
	}).Maybe()

	logger := zap.NewNop()
	account := repositories.NewAccountRepository(mockDB, "test-table", "example.com", logger)
	metric := repositories.NewMetricRecordRepository(mockDB, "test-table", logger, nil)

	mockRepos := &apiHandlers.MockRepositoryStorage{}
	mockRepos.On("PushSubscription").Return((*repositories.PushSubscriptionRepository)(nil)).Maybe()

	return &mainTestRepos{
		MockRepositoryStorage: mockRepos,
		account:               account,
		metricRecord:          metric,
	}
}

func TestMainHelpers(t *testing.T) {
	ctx := &apptheory.Context{
		Request: apptheory.Request{
			Method: "GET",
			Path:   "/health",
			Headers: map[string][]string{
				"user-agent":      {"test"},
				"x-forwarded-for": {"203.0.113.1"},
			},
		},
		RequestID: "test-request-id",
	}

	info := extractRequestInfo(ctx)
	require.Equal(t, "GET", info.method)
	require.Equal(t, "/health", info.path)

	start := time.Now().Add(-1 * time.Second)
	addLatencyContextValues(ctx, info, start)
	require.NotNil(t, ctx.Get(string(latencyStartKey)))

	p95, p99 := calculatePercentiles(map[string]interface{}{"Percentiles": map[string]float64{"p95": 10, "p99": 20}}, 5*time.Millisecond)
	require.Equal(t, 10.0, p95)
	require.Equal(t, 20.0, p99)

	require.Equal(t, observability.ErrorTypeAuthentication, determineErrorType(401))
	require.Equal(t, observability.ErrorTypeValidation, determineErrorType(422))
}

func TestMainHelpers_Round22(t *testing.T) {
	require.Equal(t, "", headerValue(nil, "origin"))

	ctx := &apptheory.Context{Request: apptheory.Request{Headers: map[string][]string{}}}
	require.Equal(t, "", headerValue(ctx, "origin"))

	// Nil contexts should be ignored safely by latency helpers.
	addLatencyContextValues(nil, requestInfo{endpoint: "GET /"}, time.Now())
}

func TestRecordLatencyMetric(t *testing.T) {
	repos = newMainTestRepos(t)
	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	err := recordLatencyMetricWithRepos(context.Background(), repos, "api_endpoint", "GET /", 15*time.Millisecond, map[string]string{"endpoint": "/"})
	require.NoError(t, err)
}

func TestMiddlewareHelpers(t *testing.T) {
	repos = newMainTestRepos(t)
	logger = zap.NewNop()
	emfMetrics = observability.NewEMFMetrics(logger, "Lesser/Test", "api")
	latencyAggregator = nil
	latencyAlerter = nil

	mw := createLatencyTrackingMiddleware()
	ctx := &apptheory.Context{Request: apptheory.Request{Method: "GET", Path: "/"}}
	handler := mw(func(c *apptheory.Context) (*apptheory.Response, error) { return apptheory.Text(200, ""), nil })
	_, err := handler(ctx)
	require.NoError(t, err)

	emfMW := createEMFMetricsMiddleware()
	handler = emfMW(func(c *apptheory.Context) (*apptheory.Response, error) { return apptheory.Text(200, ""), nil })
	_, err = handler(ctx)
	require.NoError(t, err)
}
