package health

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type stubFederationHTTPClient struct {
	getFn func(context.Context, string) (*http.Response, error)
}

func (s *stubFederationHTTPClient) Get(ctx context.Context, url string) (*http.Response, error) {
	if s.getFn != nil {
		return s.getFn(ctx, url)
	}
	return nil, errors.New("not implemented")
}

type stubHealthRepo struct {
	saveChecksCalls int
	savedChecks     []*models.InstanceHealth
	saveChecksErr   error

	calcSummaryFn func(context.Context, string, time.Duration) (*models.InstanceHealthSummary, error)
	saveSummaryFn func(context.Context, *models.InstanceHealthSummary) error
	cleanupFn     func(context.Context, time.Duration) (int, error)

	getLatestFn    func(context.Context, string) (*models.InstanceHealth, error)
	getHistoryFn   func(context.Context, string, time.Time, int) ([]*models.InstanceHealth, error)
	getSummaryFn   func(context.Context, string, time.Duration) (*models.InstanceHealthSummary, error)
	getUnhealthyFn func(context.Context, float64) ([]string, error)
}

func (s *stubHealthRepo) SaveHealthChecks(_ context.Context, checks []*models.InstanceHealth) error {
	s.saveChecksCalls++
	s.savedChecks = checks
	return s.saveChecksErr
}

func (s *stubHealthRepo) CalculateHealthSummary(ctx context.Context, domain string, window time.Duration) (*models.InstanceHealthSummary, error) {
	if s.calcSummaryFn != nil {
		return s.calcSummaryFn(ctx, domain, window)
	}
	return &models.InstanceHealthSummary{Domain: domain}, nil
}

func (s *stubHealthRepo) SaveHealthSummary(ctx context.Context, summary *models.InstanceHealthSummary) error {
	if s.saveSummaryFn != nil {
		return s.saveSummaryFn(ctx, summary)
	}
	return nil
}

func (s *stubHealthRepo) CleanupOldHealthData(ctx context.Context, retentionDuration time.Duration) (int, error) {
	if s.cleanupFn != nil {
		return s.cleanupFn(ctx, retentionDuration)
	}
	return 0, nil
}

func (s *stubHealthRepo) GetLatestHealthCheck(ctx context.Context, domain string) (*models.InstanceHealth, error) {
	if s.getLatestFn != nil {
		return s.getLatestFn(ctx, domain)
	}
	return nil, nil
}

func (s *stubHealthRepo) GetHealthHistory(ctx context.Context, domain string, since time.Time, limit int) ([]*models.InstanceHealth, error) {
	if s.getHistoryFn != nil {
		return s.getHistoryFn(ctx, domain, since, limit)
	}
	return nil, nil
}

func (s *stubHealthRepo) GetHealthSummary(ctx context.Context, domain string, window time.Duration) (*models.InstanceHealthSummary, error) {
	if s.getSummaryFn != nil {
		return s.getSummaryFn(ctx, domain, window)
	}
	return nil, nil
}

func (s *stubHealthRepo) GetUnhealthyInstances(ctx context.Context, threshold float64) ([]string, error) {
	if s.getUnhealthyFn != nil {
		return s.getUnhealthyFn(ctx, threshold)
	}
	return nil, nil
}

func TestServerlessHealthChecker_ProcessHealthCheckEvent_ValidationError(t *testing.T) {
	t.Parallel()

	checker := &ServerlessHealthChecker{
		healthRepo:       &stubHealthRepo{},
		logger:           zaptest.NewLogger(t),
		federationClient: &stubFederationHTTPClient{},
		config:           DefaultConfig(),
	}

	_, err := checker.ProcessHealthCheckEvent(context.Background(), &HealthCheckEvent{Detail: HealthCheckDetail{Action: ""}})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHealthCheckEventValidationFailed)
}

func TestServerlessHealthChecker_ProcessHealthCheckEvent_UnknownAction(t *testing.T) {
	t.Parallel()

	checker := &ServerlessHealthChecker{
		healthRepo:       &stubHealthRepo{},
		logger:           zaptest.NewLogger(t),
		federationClient: &stubFederationHTTPClient{},
		config:           DefaultConfig(),
	}

	event := &HealthCheckEvent{Detail: HealthCheckDetail{Action: "nope"}}
	event.DetailType = "Health Check Request"
	event.Source = "lesser.federation.health"

	_, err := checker.ProcessHealthCheckEvent(context.Background(), event)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownAction)
}

func TestServerlessHealthChecker_checkInstanceHealth_RequiresDomains(t *testing.T) {
	t.Parallel()

	checker := &ServerlessHealthChecker{
		healthRepo:       &stubHealthRepo{},
		logger:           zaptest.NewLogger(t),
		federationClient: &stubFederationHTTPClient{},
		config:           DefaultConfig(),
	}

	_, err := checker.checkInstanceHealth(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoDomains)
}

func TestServerlessHealthChecker_ProcessHealthCheckEvent_CheckHealth_EmptyDomainsAllowedViaInstanceIDs(t *testing.T) {
	t.Parallel()

	checker := &ServerlessHealthChecker{
		healthRepo:       &stubHealthRepo{},
		logger:           zaptest.NewLogger(t),
		federationClient: &stubFederationHTTPClient{},
		config:           DefaultConfig(),
	}

	// Event validation passes because InstanceIDs are present, but the checker still receives empty Domains.
	event := &HealthCheckEvent{
		Detail: HealthCheckDetail{
			Action:      "check_health",
			InstanceIDs: []string{"i1"},
			Domains:     nil,
		},
	}

	_, err := checker.ProcessHealthCheckEvent(context.Background(), event)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoDomains)
}

func TestNewServerlessHealthChecker_Constructs(t *testing.T) {
	t.Parallel()

	checker := NewServerlessHealthChecker(nil, "table", zaptest.NewLogger(t), DefaultConfig())
	require.NotNil(t, checker)
	require.NotNil(t, checker.healthRepo)
	require.NotNil(t, checker.federationClient)
}

func TestServerlessHealthChecker_checkSingleDomain_SuccessHeadersAndInvalidParses(t *testing.T) {
	t.Parallel()

	healthRepo := &stubHealthRepo{}

	client := &stubFederationHTTPClient{
		getFn: func(context.Context, string) (*http.Response, error) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
			}
			resp.Header.Set("X-Inbox-Backlog", "10")
			resp.Header.Set("X-Processing-Delay", "1s")
			return resp, nil
		},
	}

	checker := &ServerlessHealthChecker{
		healthRepo:       healthRepo,
		logger:           zaptest.NewLogger(t),
		federationClient: client,
		config: CheckerConfig{
			MaxRetries:          0,
			RetryBackoff:        0,
			MaxConcurrentChecks: 1,
			ValidStatusCodes:    []int{http.StatusOK},
		},
	}

	result, err := checker.checkSingleDomain(context.Background(), "example.com")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Reachable)
	assert.True(t, result.Success)
	assert.Equal(t, 10, result.InboxBacklog)
	assert.Equal(t, time.Second, result.ProcessingDelay)

	// Invalid status code path
	client.getFn = func(context.Context, string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	}
	result, err = checker.checkSingleDomain(context.Background(), "example.com")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Reachable)
	assert.False(t, result.Success)
	assert.Contains(t, result.ErrorMessage, "invalid status code")

	// Header parse errors are tolerated
	client.getFn = func(context.Context, string) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}
		resp.Header.Set("X-Inbox-Backlog", "not-an-int")
		resp.Header.Set("X-Processing-Delay", "not-a-duration")
		return resp, nil
	}
	result, err = checker.checkSingleDomain(context.Background(), "example.com")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, 0, result.InboxBacklog)
	assert.Equal(t, time.Duration(0), result.ProcessingDelay)
}

func TestServerlessHealthChecker_checkSingleDomain_RequestErrorReturnsResult(t *testing.T) {
	t.Parallel()

	client := &stubFederationHTTPClient{
		getFn: func(context.Context, string) (*http.Response, error) {
			return nil, errors.New("network down")
		},
	}

	checker := &ServerlessHealthChecker{
		healthRepo:       &stubHealthRepo{},
		logger:           zaptest.NewLogger(t),
		federationClient: client,
		config: CheckerConfig{
			MaxRetries:          1,
			RetryBackoff:        0,
			MaxConcurrentChecks: 1,
			ValidStatusCodes:    []int{http.StatusOK},
		},
	}

	result, err := checker.checkSingleDomain(context.Background(), "example.com")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Reachable)
	assert.False(t, result.Success)
	assert.Contains(t, result.ErrorMessage, "request failed after")
}

func TestServerlessHealthChecker_checkInstanceHealth_SavesResults(t *testing.T) {
	t.Parallel()

	repo := &stubHealthRepo{}
	client := &stubFederationHTTPClient{
		getFn: func(context.Context, string) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		},
	}

	checker := &ServerlessHealthChecker{
		healthRepo:       repo,
		logger:           zaptest.NewLogger(t),
		federationClient: client,
		config: CheckerConfig{
			MaxConcurrentChecks: 1,
			MaxRetries:          0,
			RetryBackoff:        0,
			ValidStatusCodes:    []int{http.StatusOK},
		},
	}

	result, err := checker.checkInstanceHealth(context.Background(), []string{"a.example", "b.example"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, result.SuccessfulChecks)
	assert.Equal(t, 0, result.FailedChecks)
	assert.Equal(t, 1, repo.saveChecksCalls)
	assert.Len(t, repo.savedChecks, 2)
}

func TestServerlessHealthChecker_ProcessHealthCheckEvent_SuccessPaths(t *testing.T) {
	t.Parallel()

	repo := &stubHealthRepo{
		calcSummaryFn: func(context.Context, string, time.Duration) (*models.InstanceHealthSummary, error) {
			return &models.InstanceHealthSummary{Domain: "example.com", Window: time.Hour}, nil
		},
	}

	client := &stubFederationHTTPClient{
		getFn: func(context.Context, string) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		},
	}

	checker := &ServerlessHealthChecker{
		healthRepo:       repo,
		logger:           zaptest.NewLogger(t),
		federationClient: client,
		config: CheckerConfig{
			MaxConcurrentChecks: 1,
			MaxRetries:          0,
			RetryBackoff:        0,
			ValidStatusCodes:    []int{http.StatusOK},
		},
	}

	// check_health
	result, err := checker.ProcessHealthCheckEvent(context.Background(), &HealthCheckEvent{
		Detail: HealthCheckDetail{
			Action:    "check_health",
			Domains:   []string{"example.com"},
			BatchSize: 1,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.SuccessfulChecks)
	assert.NotZero(t, result.TotalDuration)
	assert.NotZero(t, result.AvgDuration)

	// aggregate_summary
	result, err = checker.ProcessHealthCheckEvent(context.Background(), &HealthCheckEvent{
		Detail: HealthCheckDetail{
			Action:         "aggregate_summary",
			Domains:        []string{"example.com"},
			SummaryWindows: []string{"1h"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, strings.HasPrefix(result.EventID, "agg-"))

	// cleanup
	repo.cleanupFn = func(context.Context, time.Duration) (int, error) { return 2, nil }
	result, err = checker.ProcessHealthCheckEvent(context.Background(), &HealthCheckEvent{
		Detail: HealthCheckDetail{
			Action:        "cleanup",
			RetentionDays: 1,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, result.SuccessfulChecks)
}

func TestServerlessHealthChecker_aggregateHealthSummaries_ValidationAndWindows(t *testing.T) {
	t.Parallel()

	checker := &ServerlessHealthChecker{
		healthRepo:       &stubHealthRepo{},
		logger:           zaptest.NewLogger(t),
		federationClient: &stubFederationHTTPClient{},
		config:           DefaultConfig(),
	}

	err := checker.aggregateHealthSummaries(context.Background(), nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAggregationRequired)

	err = checker.aggregateHealthSummaries(context.Background(), []string{"example.com"}, []string{"nope"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsupportedWindow)
}

func TestServerlessHealthChecker_aggregateHealthSummaries_Loops(t *testing.T) {
	t.Parallel()

	calls := 0
	repo := &stubHealthRepo{
		calcSummaryFn: func(_ context.Context, domain string, window time.Duration) (*models.InstanceHealthSummary, error) {
			calls++
			if domain == "bad.example" {
				return nil, errors.New("boom")
			}
			return &models.InstanceHealthSummary{Domain: domain, Window: window}, nil
		},
		saveSummaryFn: func(_ context.Context, summary *models.InstanceHealthSummary) error {
			if summary.Domain == "error.example" {
				return errors.New("write failed")
			}
			return nil
		},
	}

	checker := &ServerlessHealthChecker{
		healthRepo:       repo,
		logger:           zaptest.NewLogger(t),
		federationClient: &stubFederationHTTPClient{},
		config:           DefaultConfig(),
	}

	err := checker.aggregateHealthSummaries(context.Background(), []string{"good.example", "bad.example", "error.example"}, []string{"1h", "24h"})
	require.NoError(t, err)
	assert.Equal(t, 6, calls)
}

func TestServerlessHealthChecker_cleanupOldData_DefaultsAndErrors(t *testing.T) {
	t.Parallel()

	repo := &stubHealthRepo{
		cleanupFn: func(_ context.Context, retention time.Duration) (int, error) {
			assert.Equal(t, 7*24*time.Hour, retention)
			return 0, errors.New("boom")
		},
	}

	checker := &ServerlessHealthChecker{
		healthRepo:       repo,
		logger:           zaptest.NewLogger(t),
		federationClient: &stubFederationHTTPClient{},
		config:           DefaultConfig(),
	}

	_, err := checker.cleanupOldData(context.Background(), 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHealthDataCleanupFailed)

	repo.cleanupFn = func(context.Context, time.Duration) (int, error) { return 3, nil }
	count, err := checker.cleanupOldData(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestServerlessHealthChecker_WrapperMethods(t *testing.T) {
	t.Parallel()

	repo := &stubHealthRepo{
		getLatestFn: func(context.Context, string) (*models.InstanceHealth, error) {
			return &models.InstanceHealth{Domain: "example.com"}, nil
		},
		getHistoryFn: func(context.Context, string, time.Time, int) ([]*models.InstanceHealth, error) {
			return []*models.InstanceHealth{{Domain: "example.com"}}, nil
		},
		getSummaryFn: func(context.Context, string, time.Duration) (*models.InstanceHealthSummary, error) {
			return &models.InstanceHealthSummary{Domain: "example.com"}, nil
		},
		getUnhealthyFn: func(_ context.Context, threshold float64) ([]string, error) {
			assert.Equal(t, 50.0, threshold)
			return []string{"bad.example"}, nil
		},
	}

	checker := &ServerlessHealthChecker{
		healthRepo:       repo,
		logger:           zaptest.NewLogger(t),
		federationClient: &stubFederationHTTPClient{},
		config:           DefaultConfig(),
	}

	_, err := checker.GetHealthStatus(context.Background(), "example.com")
	require.NoError(t, err)

	_, err = checker.GetHealthHistory(context.Background(), "example.com", time.Now(), 10)
	require.NoError(t, err)

	_, err = checker.GetHealthSummary(context.Background(), "example.com", time.Hour)
	require.NoError(t, err)

	instances, err := checker.GetUnhealthyInstances(context.Background(), 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"bad.example"}, instances)
}
