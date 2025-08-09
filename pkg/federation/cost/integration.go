package cost

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// DeliveryMiddleware wraps federation delivery with cost tracking and health monitoring
type DeliveryMiddleware struct {
	controller Controller
	logger     *zap.Logger
}

// NewDeliveryMiddleware creates a new cost-aware delivery middleware
func NewDeliveryMiddleware(controller Controller, logger *zap.Logger) *DeliveryMiddleware {
	return &DeliveryMiddleware{
		controller: controller,
		logger:     logger,
	}
}

// WrapDelivery wraps a delivery function with cost-aware checks and tracking
func (m *DeliveryMiddleware) WrapDelivery(
	deliveryFunc func(ctx context.Context, instanceURL, activityJSON string) error,
) func(ctx context.Context, instanceURL, activityJSON string) error {
	return func(ctx context.Context, instanceURL, activityJSON string) error {
		start := time.Now()
		activitySize := int64(len(activityJSON))

		// Extract domain from instance URL
		domain, err := extractDomain(instanceURL)
		if err != nil {
			m.logger.Error("failed to extract domain",
				zap.String("url", instanceURL),
				zap.Error(err))
			return fmt.Errorf("extract domain: %w", err)
		}

		// Check if we should federate
		shouldFederate, err := m.controller.ShouldFederate(ctx, domain)
		if err != nil {
			m.logger.Error("failed to check federation status",
				zap.String("domain", domain),
				zap.Error(err))
			return fmt.Errorf("check federation: %w", err)
		}

		if !shouldFederate {
			m.logger.Info("federation blocked or limited",
				zap.String("domain", domain))
			return fmt.Errorf("federation not allowed for %s", domain)
		}

		// Track the activity
		if err := m.controller.TrackActivity(ctx, domain, "delivery", activitySize); err != nil {
			m.logger.Warn("failed to track activity",
				zap.String("domain", domain),
				zap.Error(err))
			// Don't fail delivery on tracking errors
		}

		// Execute the delivery
		deliveryErr := deliveryFunc(ctx, instanceURL, activityJSON)

		// Record result
		elapsed := time.Since(start)
		if deliveryErr != nil {
			if err := m.controller.RecordFailure(ctx, domain, deliveryErr); err != nil {
				m.logger.Warn("failed to record failure",
					zap.String("domain", domain),
					zap.Error(err))
			}
			return deliveryErr
		}

		// Record success
		if err := m.controller.RecordSuccess(ctx, domain, elapsed.Milliseconds()); err != nil {
			m.logger.Warn("failed to record success",
				zap.String("domain", domain),
				zap.Error(err))
		}

		return nil
	}
}

// RetryMiddleware implements intelligent retry logic based on instance health and cost
type RetryMiddleware struct {
	controller Controller
	logger     *zap.Logger
}

// NewRetryMiddleware creates a new retry middleware
func NewRetryMiddleware(controller Controller, logger *zap.Logger) *RetryMiddleware {
	return &RetryMiddleware{
		controller: controller,
		logger:     logger,
	}
}

// RetryWithPolicy executes a function with instance-specific retry policy
func (m *RetryMiddleware) RetryWithPolicy(
	ctx context.Context,
	domain string,
	operation func() error,
) error {
	policy, err := m.controller.GetRetryPolicy(ctx, domain)
	if err != nil {
		m.logger.Warn("failed to get retry policy, using default",
			zap.String("domain", domain),
			zap.Error(err))
		policy = DefaultRetryPolicy
	}

	var lastErr error
	backoff := policy.InitialBackoff

	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		if attempt > 0 {
			// Check if we should still retry based on health
			healthy, err := m.controller.IsHealthy(ctx, domain)
			if err != nil {
				m.logger.Warn("failed to check health",
					zap.String("domain", domain),
					zap.Error(err))
			} else if !healthy {
				m.logger.Info("skipping retry for unhealthy instance",
					zap.String("domain", domain),
					zap.Int("attempt", attempt))
				return fmt.Errorf("instance unhealthy: %s", domain)
			}

			// Apply backoff
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}

			// Increase backoff for next attempt
			backoff = time.Duration(float64(backoff) * policy.BackoffFactor)
			if backoff > policy.MaxBackoff {
				backoff = policy.MaxBackoff
			}
		}

		lastErr = operation()
		if lastErr == nil {
			return nil
		}

		m.logger.Debug("operation failed, will retry",
			zap.String("domain", domain),
			zap.Int("attempt", attempt),
			zap.Error(lastErr))
	}

	return fmt.Errorf("operation failed after %d attempts: %w", policy.MaxRetries+1, lastErr)
}

// HTTPTransportWrapper wraps HTTP transport with cost-aware headers
type HTTPTransportWrapper struct {
	base       http.RoundTripper
	controller Controller
	logger     *zap.Logger
}

// NewHTTPTransportWrapper creates a new HTTP transport wrapper
func NewHTTPTransportWrapper(
	base http.RoundTripper,
	controller Controller,
	logger *zap.Logger,
) *HTTPTransportWrapper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &HTTPTransportWrapper{
		base:       base,
		controller: controller,
		logger:     logger,
	}
}

// RoundTrip implements http.RoundTripper with cost tracking
func (t *HTTPTransportWrapper) RoundTrip(req *http.Request) (*http.Response, error) {
	domain := req.URL.Host
	ctx := req.Context()

	// Add cost transparency headers
	tier, err := t.controller.GetInstanceTier(ctx, domain)
	if err == nil {
		req.Header.Set("X-Federation-Tier", string(tier))
	}

	budget, err := t.controller.GetRemainingBudget(ctx, domain)
	if err == nil {
		req.Header.Set("X-Federation-Budget-Remaining", fmt.Sprintf("%.2f", budget))
	}

	// Execute request
	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	elapsed := time.Since(start)

	if err != nil {
		// Record failure
		if recordErr := t.controller.RecordFailure(ctx, domain, err); recordErr != nil {
			// Log but don't fail the original request
			zap.L().Warn("failed to record federation failure", zap.Error(recordErr))
		}
		return nil, err
	}

	// Track request size
	var requestSize int64
	if req.Body != nil && req.ContentLength > 0 {
		requestSize = req.ContentLength
	}

	// Track activity
	if err := t.controller.TrackActivity(ctx, domain, "http_request", requestSize); err != nil {
		t.logger.Warn("failed to track HTTP activity",
			zap.String("domain", domain),
			zap.Error(err))
	}

	// Record success
	if err := t.controller.RecordSuccess(ctx, domain, elapsed.Milliseconds()); err != nil {
		t.logger.Warn("failed to record HTTP success",
			zap.String("domain", domain),
			zap.Error(err))
	}

	// Add response headers about our cost tracking
	if resp != nil {
		resp.Header.Set("X-Federation-Cost-Tracked", "true")
		resp.Header.Set("X-Federation-Response-Time", fmt.Sprintf("%dms", elapsed.Milliseconds()))
	}

	return resp, nil
}

// Helper functions

func extractDomain(instanceURL string) (string, error) {
	// Simple domain extraction - can be enhanced
	if instanceURL == "" {
		return "", fmt.Errorf("empty instance URL")
	}

	// Remove protocol if present
	domain := instanceURL
	if idx := len("https://"); len(domain) > idx && domain[:idx] == "https://" {
		domain = domain[idx:]
	} else if idx := len("http://"); len(domain) > idx && domain[:idx] == "http://" {
		domain = domain[idx:]
	}

	// Remove path if present
	for idx := 0; idx < len(domain); idx++ {
		if domain[idx] == '/' {
			domain = domain[:idx]
			break
		}
	}

	return domain, nil
}
