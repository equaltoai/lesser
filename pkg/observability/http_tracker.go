// Package observability provides HTTP client latency tracking for federation and external API calls
package observability

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// Additional context key types for HTTP tracking
const (
	federationTargetKey contextKey = "federation_target"
	operationTypeKey    contextKey = "operation_type"
	unknownValue        = "unknown"
)

// HTTPTracker wraps HTTP client operations with comprehensive latency tracking
type HTTPTracker struct {
	client          *http.Client
	logger          *zap.Logger
	metricsRecorder MetricsRecorder
	serviceName     string
}

// HTTPMetrics represents detailed HTTP call metrics
type HTTPMetrics struct {
	URL            string
	Method         string
	StatusCode     int
	RequestSize    int64
	ResponseSize   int64
	DNSTime        time.Duration
	TCPTime        time.Duration
	TLSTime        time.Duration
	FirstByteTime  time.Duration
	TotalTime      time.Duration
	Success        bool
	ErrorType      string
	RetryAttempts  int
}

// NewHTTPTracker creates a new HTTP client with latency tracking
func NewHTTPTracker(client *http.Client, logger *zap.Logger, recorder MetricsRecorder, serviceName string) *HTTPTracker {
	if client == nil {
		client = http.DefaultClient
	}

	return &HTTPTracker{
		client:          client,
		logger:          logger,
		metricsRecorder: recorder,
		serviceName:     serviceName,
	}
}

// Do executes an HTTP request with comprehensive tracking
func (ht *HTTPTracker) Do(ctx context.Context, req *http.Request) (*http.Response, *HTTPMetrics, error) {
	startTime := time.Now()
	
	// Extract URL components
	host := req.URL.Host
	if err := common.ValidateRequiredParam("host", host); err != nil {
		host = unknownValue
	}
	
	// Prepare metrics
	metrics := &HTTPMetrics{
		URL:         req.URL.String(),
		Method:      req.Method,
		RequestSize: req.ContentLength,
	}

	// Add request ID if available in context
	requestID := ""
	if reqID, ok := ctx.Value("request_id").(string); ok {
		requestID = reqID
	}

	// Execute request
	resp, err := ht.client.Do(req.WithContext(ctx))
	totalDuration := time.Since(startTime)
	
	metrics.TotalTime = totalDuration
	metrics.Success = err == nil && (resp != nil && resp.StatusCode < 400)

	if resp != nil {
		metrics.StatusCode = resp.StatusCode
		metrics.ResponseSize = resp.ContentLength
	} else {
		metrics.StatusCode = 0
	}

	// Determine error type
	if err != nil {
		metrics.ErrorType = categorizeHTTPError(err)
	} else if resp != nil && resp.StatusCode >= 400 {
		metrics.ErrorType = categorizeHTTPStatusError(resp.StatusCode)
	}

	// Record federation-specific metrics
	operationType := "http_request"
	if isFederationRequest(req.URL) {
		operationType = "federation_request"
	}

	// Prepare dimensions
	dimensions := map[string]string{
		"method":        req.Method,
		"host":          host,
		"status_code":   fmt.Sprintf("%d", metrics.StatusCode),
		"operation":     operationType,
		"request_id":    requestID,
	}

	if metrics.ErrorType != "" {
		dimensions["error_type"] = metrics.ErrorType
	}

	// Record metrics
	go func() {
		if err := ht.recordHTTPMetrics(context.Background(), operationType, host, metrics, dimensions); err != nil {
			ht.logger.Warn("failed to record HTTP metrics",
				zap.String("url", req.URL.String()),
				zap.String("method", req.Method),
				zap.Duration("duration", totalDuration),
				zap.Error(err))
		}
	}()

	// Log the request with appropriate level based on latency and success
	logLevel := zap.DebugLevel
	if !metrics.Success || totalDuration > 5*time.Second {
		logLevel = zap.WarnLevel
	} else if totalDuration > 2*time.Second {
		logLevel = zap.InfoLevel
	}

	ht.logger.Log(logLevel, "HTTP request completed",
		zap.String("method", req.Method),
		zap.String("url", req.URL.String()),
		zap.String("host", host),
		zap.Int("status_code", metrics.StatusCode),
		zap.Duration("duration", totalDuration),
		zap.Bool("success", metrics.Success),
		zap.String("error_type", metrics.ErrorType),
		zap.String("request_id", requestID),
		zap.Error(err))

	return resp, metrics, err
}

// DoFederation executes a federation HTTP request with specialized tracking
func (ht *HTTPTracker) DoFederation(ctx context.Context, req *http.Request, targetInstance string) (*http.Response, *HTTPMetrics, error) {
	// Add federation-specific context
	ctx = context.WithValue(ctx, federationTargetKey, targetInstance)
	ctx = context.WithValue(ctx, operationTypeKey, "federation")

	resp, metrics, err := ht.Do(ctx, req)

	// Record federation-specific metrics
	if ht.metricsRecorder != nil && metrics != nil {
		federationDimensions := map[string]string{
			"target_instance": targetInstance,
			"federation_type": getFederationType(req.URL.Path),
			"method":         req.Method,
			"success":        fmt.Sprintf("%t", metrics.Success),
		}

		// Record federation latency
		federationMetric := &models.MetricRecord{
			MetricType:       "federation_latency",
			ServiceName:      ht.serviceName,
			Timestamp:        time.Now(),
			AggregationLevel: "raw",
			Unit:             "ms",
			Dimensions:       federationDimensions,
			Count:            1,
			Sum:              float64(metrics.TotalTime.Milliseconds()),
			Min:              float64(metrics.TotalTime.Milliseconds()),
			Max:              float64(metrics.TotalTime.Milliseconds()),
			P50:              float64(metrics.TotalTime.Milliseconds()),
			P95:              float64(metrics.TotalTime.Milliseconds()),
			P99:              float64(metrics.TotalTime.Milliseconds()),
		}

		// Add target instance to dimensions
		federationMetric.AddDimension("target_instance", targetInstance)

		// Try to record the federation metric
		if dmr, ok := ht.metricsRecorder.(*DefaultMetricsRecorder); ok {
			if err := dmr.createMetricFn(ctx, federationMetric); err != nil {
				ht.logger.Warn("failed to record federation metric",
					zap.String("target_instance", targetInstance),
					zap.Duration("duration", metrics.TotalTime),
					zap.Error(err))
			}
		}
	}

	return resp, metrics, err
}

// recordHTTPMetrics records HTTP call metrics
func (ht *HTTPTracker) recordHTTPMetrics(ctx context.Context, operation, host string, metrics *HTTPMetrics, dimensions map[string]string) error {
	if ht.metricsRecorder == nil {
		return nil
	}

	return ht.metricsRecorder.RecordLatency(ctx, operation, host, metrics.TotalTime, metrics.Success, dimensions)
}

// categorizeHTTPError categorizes HTTP errors for metrics
func categorizeHTTPError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()
	
	// Timeout errors
	if isTimeoutError(err) {
		return ErrorTypeTimeout
	}
	
	// DNS errors
	if isDNSError(errStr) {
		return "dns_error"
	}
	
	// Connection errors
	if isConnectionError(errStr) {
		return "connection_error"
	}
	
	// TLS/SSL errors
	if isTLSError(errStr) {
		return "tls_error"
	}
	
	// Context cancellation
	if isContextError(err) {
		return "context_canceled"
	}

	return "network_error"
}

// categorizeHTTPStatusError categorizes HTTP status code errors
func categorizeHTTPStatusError(statusCode int) string {
	switch {
	case statusCode >= 400 && statusCode < 500:
		switch statusCode {
		case 401:
			return ErrorTypeAuthentication
		case 403:
			return ErrorTypeAuthorization
		case 404:
			return ErrorTypeNotFound
		case 408:
			return ErrorTypeTimeout
		case 409:
			return ErrorTypeConflict
		case 429:
			return ErrorTypeRateLimit
		default:
			return ErrorTypeValidation
		}
	case statusCode >= 500:
		return "server_error"
	default:
		return ""
	}
}

// Helper functions to identify error types
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	// Check for timeout in error message
	errStr := err.Error()
	return containsAny(errStr, []string{"timeout", "deadline", "timed out"})
}

func isDNSError(errStr string) bool {
	return containsAny(errStr, []string{"no such host", "dns", "name resolution"})
}

func isConnectionError(errStr string) bool {
	return containsAny(errStr, []string{"connection refused", "connection reset", "no route to host"})
}

func isTLSError(errStr string) bool {
	return containsAny(errStr, []string{"tls", "ssl", "certificate", "x509"})
}

func isContextError(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if contains(s, sub) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && 
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		 findSubstringHTTP(s, substr))))
}

func findSubstringHTTP(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// isFederationRequest determines if a request is for federation
func isFederationRequest(url *url.URL) bool {
	if url == nil {
		return false
	}
	
	path := url.Path
	// Common ActivityPub and federation endpoints
	federationPaths := []string{
		"/inbox",
		"/outbox", 
		"/.well-known/webfinger",
		"/.well-known/nodeinfo",
		"/users/",
		"/actors/",
		"/activities/",
		"/objects/",
	}
	
	for _, fedPath := range federationPaths {
		if path == fedPath || (len(path) > len(fedPath) && path[:len(fedPath)] == fedPath) {
			return true
		}
	}
	
	return false
}

// getFederationType determines the type of federation operation
func getFederationType(path string) string {
	switch {
	case path == "/inbox":
		return "inbox"
	case path == "/outbox":
		return "outbox"
	case contains(path, ".well-known"):
		return "discovery"
	case contains(path, "/users/") || contains(path, "/actors/"):
		return "actor"
	case contains(path, "/activities/"):
		return "activity"
	case contains(path, "/objects/"):
		return "object"
	default:
		return "other"
	}
}

// HTTPLatencyTracker provides a simple interface for tracking HTTP latencies
type HTTPLatencyTracker struct {
	recorder    MetricsRecorder
	serviceName string
	logger      *zap.Logger
}

// NewHTTPLatencyTracker creates a simple HTTP latency tracker
func NewHTTPLatencyTracker(recorder MetricsRecorder, serviceName string, logger *zap.Logger) *HTTPLatencyTracker {
	return &HTTPLatencyTracker{
		recorder:    recorder,
		serviceName: serviceName,
		logger:      logger,
	}
}

// TrackRequest tracks an HTTP request latency
func (hlt *HTTPLatencyTracker) TrackRequest(ctx context.Context, method, url string, statusCode int, duration time.Duration) {
	if hlt.recorder == nil {
		return
	}

	success := statusCode >= 200 && statusCode < 400
	host := unknownValue
	if u, err := urlParse(url); err == nil {
		host = u.Host
	}

	dimensions := map[string]string{
		"method":      method,
		"host":        host,
		"status_code": fmt.Sprintf("%d", statusCode),
	}

	operation := "http_request"
	if isFederationURL(url) {
		operation = "federation_request"
		dimensions["federation_type"] = getFederationTypeFromURL(url)
	}

	if err := hlt.recorder.RecordLatency(ctx, operation, host, duration, success, dimensions); err != nil {
		hlt.logger.Warn("failed to track HTTP request latency",
			zap.String("method", method),
			zap.String("url", url),
			zap.Duration("duration", duration),
			zap.Error(err))
	}
}

// Helper functions for URL parsing
func urlParse(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}

func isFederationURL(rawURL string) bool {
	u, err := urlParse(rawURL)
	if err != nil {
		return false
	}
	return isFederationRequest(u)
}

func getFederationTypeFromURL(rawURL string) string {
	u, err := urlParse(rawURL)
	if err != nil {
		return unknownValue
	}
	return getFederationType(u.Path)
}