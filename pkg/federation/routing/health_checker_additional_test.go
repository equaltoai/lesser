package routing

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	fedTypes "github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeFederationHTTPClient struct {
	resp *http.Response
	err  error

	calls int
	url   string
}

func (f *fakeFederationHTTPClient) Get(_ context.Context, url string) (*http.Response, error) {
	f.calls++
	f.url = url
	return f.resp, f.err
}

type fakeInstanceHealthRepo struct {
	saveCalls int
	saveErr   error
	saved     []*models.InstanceHealth

	history    []*models.InstanceHealth
	historyErr error

	summary    *models.InstanceHealthSummary
	summaryErr error
}

func (r *fakeInstanceHealthRepo) SaveHealthCheck(_ context.Context, health *models.InstanceHealth) error {
	r.saveCalls++
	r.saved = append(r.saved, health)
	return r.saveErr
}

func (r *fakeInstanceHealthRepo) GetHealthHistory(_ context.Context, _ string, _ time.Time, _ int) ([]*models.InstanceHealth, error) {
	return r.history, r.historyErr
}

func (r *fakeInstanceHealthRepo) GetHealthSummary(_ context.Context, _ string, _ time.Duration) (*models.InstanceHealthSummary, error) {
	return r.summary, r.summaryErr
}

type closeErrBody struct {
	closeErr error
}

func (b *closeErrBody) Read(_ []byte) (int, error) { return 0, io.EOF }
func (b *closeErrBody) Close() error               { return b.closeErr }

func TestInstanceHealthChecker_StartStop_NoOp(t *testing.T) {
	hc := &InstanceHealthChecker{
		healthRepo: &fakeInstanceHealthRepo{},
		logger:     zap.NewNop(),
		config:     defaultRoutingConfig(),
	}

	assert.NoError(t, hc.StartMonitoring(&fedTypes.Instance{ID: "x"}))
	assert.NoError(t, hc.StopMonitoring("x"))
}

func TestInstanceHealthChecker_CheckHealth_ErrorAndSuccessPaths(t *testing.T) {
	repo := &fakeInstanceHealthRepo{}

	hc := &InstanceHealthChecker{
		healthRepo: repo,
		logger:     zap.NewNop(),
		config:     defaultRoutingConfig(),
		federationClient: &fakeFederationHTTPClient{
			err: errors.New("dial failed"),
		},
	}

	status, err := hc.CheckHealth(&fedTypes.Instance{ID: "id", Domain: "example.com", InboxURL: "https://remote.example/inbox"})
	assert.NoError(t, err)
	assert.False(t, status.Reachable)
	assert.NotEmpty(t, status.ErrorMessage)
	assert.Equal(t, 1, repo.saveCalls)

	// 5xx sets error rate to 1.0 and parses headers.
	hc.federationClient = &fakeFederationHTTPClient{
		resp: &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Header: http.Header{
				"X-Inbox-Backlog":      []string{"123"},
				"X-Processing-Delay":   []string{"2s"},
				"Content-Type":         []string{"text/plain"},
				"X-Unused-Test-Header": []string{"x"},
			},
			Body: &closeErrBody{closeErr: errors.New("close failed")},
		},
	}

	status, err = hc.CheckHealth(&fedTypes.Instance{ID: "id", Domain: "example.com", InboxURL: "https://remote.example/inbox"})
	assert.NoError(t, err)
	assert.True(t, status.Reachable)
	assert.Equal(t, 123, status.InboxBacklog)
	assert.Equal(t, 2*time.Second, status.ProcessingDelay)
	assert.InDelta(t, 1.0, status.ErrorRate, 0.0001)
	assert.Equal(t, http.StatusServiceUnavailable, status.StatusCode)
	assert.Equal(t, 2, repo.saveCalls)

	// 4xx sets error rate to 0.5 and tolerates parse failures.
	hc.federationClient = &fakeFederationHTTPClient{
		resp: &http.Response{
			StatusCode: http.StatusNotFound,
			Header: http.Header{
				"X-Inbox-Backlog":    []string{"not-a-number"},
				"X-Processing-Delay": []string{"not-a-duration"},
			},
			Body: io.NopCloser(strings.NewReader("")),
		},
	}

	status, err = hc.CheckHealth(&fedTypes.Instance{ID: "id", Domain: "example.com", InboxURL: "https://remote.example/inbox"})
	assert.NoError(t, err)
	assert.True(t, status.Reachable)
	assert.InDelta(t, 0.5, status.ErrorRate, 0.0001)
	assert.Equal(t, http.StatusNotFound, status.StatusCode)
	assert.Equal(t, 3, repo.saveCalls)
}

func TestInstanceHealthChecker_GetHealthHistory_AndAggregatedHealth(t *testing.T) {
	now := time.Now()
	repo := &fakeInstanceHealthRepo{
		history: []*models.InstanceHealth{
			{
				Domain:          "example.com",
				Timestamp:       now.Add(-1 * time.Minute),
				Reachable:       true,
				ResponseTime:    100 * time.Millisecond,
				StatusCode:      200,
				InboxBacklog:    10,
				ProcessingDelay: 1 * time.Second,
				ErrorRate:       0,
			},
			{
				Domain:          "example.com",
				Timestamp:       now.Add(-2 * time.Minute),
				Reachable:       false,
				ResponseTime:    0,
				StatusCode:      503,
				ErrorMessage:    "down",
				InboxBacklog:    50,
				ProcessingDelay: 0,
				ErrorRate:       1.0,
			},
		},
	}

	hc := &InstanceHealthChecker{
		healthRepo: repo,
		logger:     zap.NewNop(),
		config:     defaultRoutingConfig(),
	}

	history, err := hc.GetHealthHistory("example.com", 10*time.Minute)
	assert.NoError(t, err)
	assert.Len(t, history, 2)
	assert.Equal(t, 10, history[0].InboxBacklog)
	assert.Equal(t, "down", history[1].ErrorMessage)

	// Summary hit should return without recomputing.
	repo.summary = &models.InstanceHealthSummary{
		Domain:           "example.com",
		Window:           10 * time.Minute,
		SampleCount:      99,
		LastUpdated:      now,
		Availability:     0.9,
		AvgResponseTime:  123 * time.Millisecond,
		ErrorRate:        0.1,
		AvgInboxBacklog:  5,
		MaxInboxBacklog:  20,
		HealthScore:      80.0,
		StatusCodeCounts: map[string]int{"200": 10, "500": 1},
	}

	agg, err := hc.GetAggregatedHealth("example.com", 10*time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 99, agg.SampleCount)
	assert.Equal(t, 10, agg.StatusCodes[200])

	// No data should return ErrNoHealthDataAvailable.
	repo.summary = nil
	repo.history = []*models.InstanceHealth{}
	_, err = hc.GetAggregatedHealth("example.com", 10*time.Minute)
	assert.ErrorIs(t, err, ErrNoHealthDataAvailable)

	// History error should be wrapped.
	repo.historyErr = errors.New("read failed")
	_, err = hc.GetHealthHistory("example.com", 10*time.Minute)
	assert.ErrorIs(t, err, ErrHealthHistoryRetrieveFailed)
}

func TestInstanceHealthChecker_GetAggregatedHealth_ComputesFallback(t *testing.T) {
	now := time.Now()
	repo := &fakeInstanceHealthRepo{
		history: []*models.InstanceHealth{
			{Timestamp: now.Add(-1 * time.Minute), Reachable: true, ResponseTime: 100 * time.Millisecond, StatusCode: 200, InboxBacklog: 10},
			{Timestamp: now.Add(-2 * time.Minute), Reachable: true, ResponseTime: 200 * time.Millisecond, StatusCode: 200, InboxBacklog: 20},
			{Timestamp: now.Add(-3 * time.Minute), Reachable: false, StatusCode: 503, InboxBacklog: 50, ErrorRate: 1.0},
		},
	}

	hc := &InstanceHealthChecker{
		healthRepo: repo,
		logger:     zap.NewNop(),
		config:     defaultRoutingConfig(),
	}

	agg, err := hc.GetAggregatedHealth("example.com", 10*time.Minute)
	require.NoError(t, err)
	assert.Equal(t, 3, agg.SampleCount)
	assert.InDelta(t, 2.0/3.0, agg.Availability, 0.0001)
	assert.InDelta(t, 1.0/3.0, agg.ErrorRate, 0.0001)
	assert.Equal(t, 26, agg.AvgBacklog)
	assert.Equal(t, 50, agg.MaxBacklog)
	assert.NotZero(t, agg.HealthScore)
}
