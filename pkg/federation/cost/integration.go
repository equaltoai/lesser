package cost

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
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
			m.logger.Error("failed to extract domain from federation URL",
				zap.String("instance_url", instanceURL),
				zap.Int64("activity_size", activitySize),
				zap.String("operation", "delivery"),
				zap.Error(err))
			return errors.Join(ErrDomainExtractionFailed, err)
		}

		// Check if we should federate
		shouldFederate, err := m.controller.ShouldFederate(ctx, domain)
		if err != nil {
			m.logger.Error("failed to check federation status for cost-aware delivery",
				zap.String("domain", domain),
				zap.String("instance_url", instanceURL),
				zap.Int64("activity_size", activitySize),
				zap.String("operation", "delivery"),
				zap.String("cost_check", "federation_allowed"),
				zap.Error(err))
			return errors.Join(ErrFederationCheckFailed, err)
		}

		if !shouldFederate {
			m.logger.Error("federation blocked or limited for cost optimization",
				zap.String("domain", domain),
				zap.String("instance_url", instanceURL),
				zap.Int64("activity_size", activitySize),
				zap.String("operation", "delivery"),
				zap.String("cost_decision", "blocked"),
				zap.String("reason", "federation_not_allowed"))
			return ErrFederationNotAllowed
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
				m.logger.Error("skipping retry for unhealthy instance in cost-aware policy",
					zap.String("domain", domain),
					zap.Int("attempt", attempt),
					zap.String("operation", "retry"),
					zap.String("cost_decision", "skip_unhealthy"),
					zap.Duration("backoff_duration", backoff),
					zap.String("health_status", "unhealthy"))
				return ErrInstanceUnhealthy
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

	m.logger.Error("federation operation failed after all retry attempts",
		zap.String("domain", domain),
		zap.Int("total_attempts", policy.MaxRetries+1),
		zap.Duration("final_backoff", backoff),
		zap.Duration("initial_backoff", policy.InitialBackoff),
		zap.Float64("backoff_factor", policy.BackoffFactor),
		zap.Duration("max_backoff", policy.MaxBackoff),
		zap.String("operation", "retry_with_policy"),
		zap.String("cost_impact", "retry_exhausted"),
		zap.Error(lastErr))
	return errors.Join(ErrOperationFailedAfterRetries, lastErr)
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
	if err := common.ValidateRequiredParam("instanceURL", instanceURL); err != nil {
		return "", ErrEmptyInstanceURL
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
