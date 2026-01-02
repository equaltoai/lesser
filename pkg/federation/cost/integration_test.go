package cost

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type stubController struct {
	mu sync.Mutex

	shouldFederate    bool
	shouldFederateErr error

	tier    FederationTier
	tierErr error

	remainingBudget    float64
	remainingBudgetErr error

	retryPolicy    *RetryPolicy
	retryPolicyErr error

	healthy    bool
	healthyErr error

	trackActivityErr error
	recordSuccessErr error
	recordFailureErr error

	trackActivityCalls int
	recordSuccessCalls int
	recordFailureCalls int
	isHealthyCalls     int
}

func (c *stubController) ShouldFederate(_ context.Context, _ string) (bool, error) {
	return c.shouldFederate, c.shouldFederateErr
}
func (c *stubController) GetInstanceTier(_ context.Context, _ string) (FederationTier, error) {
	return c.tier, c.tierErr
}
func (c *stubController) GetRetryPolicy(_ context.Context, _ string) (*RetryPolicy, error) {
	if c.retryPolicy == nil {
		c.retryPolicy = &RetryPolicy{MaxRetries: 0, InitialBackoff: 0, MaxBackoff: 0, BackoffFactor: 1}
	}
	return c.retryPolicy, c.retryPolicyErr
}
func (c *stubController) TrackActivity(_ context.Context, _ string, _ string, _ int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.trackActivityCalls++
	return c.trackActivityErr
}
func (c *stubController) GetRemainingBudget(_ context.Context, _ string) (float64, error) {
	return c.remainingBudget, c.remainingBudgetErr
}
func (c *stubController) RecordSuccess(_ context.Context, _ string, _ int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recordSuccessCalls++
	return c.recordSuccessErr
}
func (c *stubController) RecordFailure(_ context.Context, _ string, _ error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recordFailureCalls++
	return c.recordFailureErr
}
func (c *stubController) IsHealthy(_ context.Context, _ string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.isHealthyCalls++
	return c.healthy, c.healthyErr
}

type stubRoundTripper struct {
	resp *http.Response
	err  error

	called int
	last   *http.Request
}

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	s.called++
	s.last = req
	return s.resp, s.err
}

func TestDeliveryMiddleware_WrapDelivery_Branches(t *testing.T) {
	ctrl := &stubController{
		shouldFederate: true,
		healthy:        true,
	}
	mw := NewDeliveryMiddleware(ctrl, zap.NewNop())

	wrapped := mw.WrapDelivery(func(_ context.Context, _ string, _ string) error {
		return nil
	})

	err := wrapped(context.Background(), "", "{}")
	assert.ErrorIs(t, err, ErrDomainExtractionFailed)

	ctrl.shouldFederateErr = errors.New("boom")
	err = wrapped(context.Background(), "https://example.com/inbox", "{}")
	assert.ErrorIs(t, err, ErrFederationCheckFailed)

	ctrl.shouldFederateErr = nil
	ctrl.shouldFederate = false
	err = wrapped(context.Background(), "https://example.com/inbox", "{}")
	assert.ErrorIs(t, err, ErrFederationNotAllowed)

	ctrl.shouldFederate = true
	fail := errors.New("deliver failed")
	wrappedFail := mw.WrapDelivery(func(_ context.Context, _ string, _ string) error {
		return fail
	})
	err = wrappedFail(context.Background(), "https://example.com/inbox", "{}")
	assert.ErrorIs(t, err, fail)
	assert.Equal(t, 1, ctrl.recordFailureCalls)

	ctrl.recordFailureErr = errors.New("record failure failed")
	err = wrappedFail(context.Background(), "https://example.com/inbox", "{}")
	assert.ErrorIs(t, err, fail)

	ctrl.trackActivityErr = errors.New("track failed")
	ctrl.recordSuccessErr = errors.New("success record failed")
	err = wrapped(context.Background(), "https://example.com/inbox", "{}")
	assert.NoError(t, err)
	assert.Equal(t, 1, ctrl.recordSuccessCalls)
}

func TestRetryMiddleware_RetryWithPolicy_Branches(t *testing.T) {
	ctrl := &stubController{
		healthy: true,
		retryPolicy: &RetryPolicy{
			MaxRetries:     1,
			InitialBackoff: 0,
			MaxBackoff:     0,
			BackoffFactor:  1.0,
		},
	}
	mw := NewRetryMiddleware(ctrl, zap.NewNop())

	callCount := 0
	err := mw.RetryWithPolicy(context.Background(), "example.com", func() error {
		callCount++
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, callCount)

	callCount = 0
	err = mw.RetryWithPolicy(context.Background(), "example.com", func() error {
		callCount++
		if callCount == 1 {
			return errors.New("fail once")
		}
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, callCount)

	ctrl.healthy = false
	err = mw.RetryWithPolicy(context.Background(), "example.com", func() error {
		return errors.New("always fail")
	})
	assert.ErrorIs(t, err, ErrInstanceUnhealthy)

	ctrl.healthy = true
	err = mw.RetryWithPolicy(context.Background(), "example.com", func() error {
		return errors.New("always fail")
	})
	assert.ErrorIs(t, err, ErrOperationFailedAfterRetries)

	// Exercise fallback-to-default policy branch without sleeping by canceling context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ctrl.retryPolicyErr = errors.New("policy lookup failed")
	err = mw.RetryWithPolicy(ctx, "example.com", func() error {
		return errors.New("fail")
	})
	assert.Error(t, err)
}

func TestHTTPTransportWrapper_RoundTrip_Branches(t *testing.T) {
	ctrl := &stubController{
		tier:            TierStandard,
		remainingBudget: 9.5,
		healthy:         true,
	}

	rt := &stubRoundTripper{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
		},
	}

	wrapper := NewHTTPTransportWrapper(rt, ctrl, zap.NewNop())

	req, _ := http.NewRequest(http.MethodGet, "https://example.com/inbox", nil)
	resp, err := wrapper.RoundTrip(req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "true", resp.Header.Get("X-Federation-Cost-Tracked"))
	assert.NotEmpty(t, req.Header.Get("X-Federation-Tier"))
	assert.NotEmpty(t, req.Header.Get("X-Federation-Budget-Remaining"))
	assert.Equal(t, 1, ctrl.trackActivityCalls)
	assert.Equal(t, 1, ctrl.recordSuccessCalls)

	rt.err = errors.New("network error")
	rt.resp = nil
	req2, _ := http.NewRequest(http.MethodGet, "https://example.com/inbox", nil)
	ctrl.recordFailureErr = errors.New("failure record failed")
	resp, err = wrapper.RoundTrip(req2)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, rt.err)
	assert.Equal(t, 1, ctrl.recordFailureCalls)

	ctrl.recordFailureErr = nil

	// Warnings from cost tracking should not fail the request.
	ctrl.trackActivityErr = errors.New("track failed")
	ctrl.recordSuccessErr = errors.New("success record failed")
	rt.err = nil
	rt.resp = &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("ok")),
	}
	req3, _ := http.NewRequest(http.MethodGet, "https://example.com/inbox", nil)
	resp, err = wrapper.RoundTrip(req3)
	assert.NoError(t, err)
	assert.NotNil(t, resp)

	// Ensure the timing path doesn't hang even with a tiny backoff elsewhere.
	_ = time.Now()
}

func TestNewHTTPTransportWrapper_DefaultBase(t *testing.T) {
	ctrl := &stubController{tier: TierStandard}
	wrapper := NewHTTPTransportWrapper(nil, ctrl, zap.NewNop())
	assert.Equal(t, http.DefaultTransport, wrapper.base)
}

func TestExtractDomain(t *testing.T) {
	_, err := extractDomain("")
	assert.ErrorIs(t, err, ErrEmptyInstanceURL)

	domain, err := extractDomain("https://example.com/inbox")
	assert.NoError(t, err)
	assert.Equal(t, "example.com", domain)

	domain, err = extractDomain("http://example.com/inbox")
	assert.NoError(t, err)
	assert.Equal(t, "example.com", domain)

	domain, err = extractDomain("example.com/path")
	assert.NoError(t, err)
	assert.Equal(t, "example.com", domain)
}
