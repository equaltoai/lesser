package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
)

// HealthEndpoints provides HTTP endpoints for health checks
type HealthEndpoints struct {
	// Interfaces/slices first
	logger      lift.Logger // optional structured logger
	manager     HealthManager
	corsOrigins []string
	// 8-byte aligned duration next
	timeout time.Duration
	// small scalars last (packed together)
	enableDetailedErrors bool
	enableCORS           bool
}

// HealthEndpointsConfig configures health endpoints
type HealthEndpointsConfig struct {
	// Interfaces/slices first
	Logger      lift.Logger
	CORSOrigins []string
	// 8-byte aligned
	Timeout time.Duration
	// small scalars last
	EnableDetailedErrors bool
	EnableCORS           bool
}

// NewHealthEndpoints creates new health endpoints
func NewHealthEndpoints(manager HealthManager, config HealthEndpointsConfig) *HealthEndpoints {
	return &HealthEndpoints{
		manager:              manager,
		enableDetailedErrors: config.EnableDetailedErrors,
		enableCORS:           config.EnableCORS,
		corsOrigins:          config.CORSOrigins,
		timeout:              config.Timeout,
		logger:               config.Logger,
	}
}

// DefaultHealthEndpointsConfig returns sensible defaults
func DefaultHealthEndpointsConfig() HealthEndpointsConfig {
	return HealthEndpointsConfig{
		EnableDetailedErrors: false, // Don't expose internal details by default
		EnableCORS:           true,
		CORSOrigins:          []string{"*"},
		Timeout:              10 * time.Second,
	}
}

// HealthResponse represents the JSON response for health endpoints
type HealthResponse struct {
	Status    string         `json:"status"`
	Timestamp string         `json:"timestamp"`
	Duration  string         `json:"duration"`
	Message   string         `json:"message,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// PlainTextResponse represents a simple text response
type PlainTextResponse struct {
	Status  string
	Message string
}

// RegisterRoutes registers health check routes with an HTTP mux
func (he *HealthEndpoints) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", he.HealthHandler)
	mux.HandleFunc("/health/", he.HealthHandler) // Handle trailing slash
	mux.HandleFunc("/health/ready", he.ReadinessHandler)
	mux.HandleFunc("/health/live", he.LivenessHandler)
	mux.HandleFunc("/health/components", he.ComponentsHandler)
}

// HealthHandler handles GET /health - overall health status
func (he *HealthEndpoints) HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		he.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	he.setCORSHeaders(w)

	ctx, cancel := context.WithTimeout(r.Context(), he.timeout)
	defer cancel()

	overall := he.manager.OverallHealth(ctx)

	// Determine HTTP status code
	statusCode := he.healthStatusToHTTPStatus(overall.Status)

	// Check if client wants plain text
	if he.wantsPlainText(r) {
		he.writePlainTextResponse(w, statusCode, overall)
		return
	}

	// Return JSON response
	he.writeJSONResponse(w, statusCode, overall)
}

// ReadinessHandler handles GET /health/ready - Kubernetes readiness probe
func (he *HealthEndpoints) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		he.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	he.setCORSHeaders(w)

	ctx, cancel := context.WithTimeout(r.Context(), he.timeout)
	defer cancel()

	overall := he.manager.OverallHealth(ctx)

	// For readiness, we consider degraded as ready (service can handle traffic)
	// Only unhealthy or unknown should return non-200
	statusCode := http.StatusOK
	if overall.Status == StatusUnhealthy || overall.Status == StatusUnknown {
		statusCode = http.StatusServiceUnavailable
	}

	if he.wantsPlainText(r) {
		he.writePlainTextResponse(w, statusCode, overall)
		return
	}

	he.writeJSONResponse(w, statusCode, overall)
}

// LivenessHandler handles GET /health/live - Kubernetes liveness probe
func (he *HealthEndpoints) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		he.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	he.setCORSHeaders(w)

	// For liveness, we just check if the service is running
	// This is a simple check that doesn't depend on external services
	status := HealthStatus{
		Status:    StatusHealthy,
		Timestamp: time.Now(),
		Duration:  time.Microsecond,
		Message:   "Service is alive",
	}

	if he.wantsPlainText(r) {
		he.writePlainTextResponse(w, http.StatusOK, status)
		return
	}

	he.writeJSONResponse(w, http.StatusOK, status)
}

// ComponentsHandler handles GET /health/components - individual component health
func (he *HealthEndpoints) ComponentsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		he.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	he.setCORSHeaders(w)

	ctx, cancel := context.WithTimeout(r.Context(), he.timeout)
	defer cancel()

	// Check if a specific component is requested
	component := r.URL.Query().Get("component")
	if component != "" {
		status, err := he.manager.CheckComponent(ctx, component)
		if err != nil {
			he.writeError(w, http.StatusNotFound, fmt.Sprintf("Component %s not found", component))
			return
		}

		statusCode := he.healthStatusToHTTPStatus(status.Status)
		he.writeJSONResponse(w, statusCode, status)
		return
	}

	// Return all components
	results := he.manager.CheckAll(ctx)

	// Convert to response format
	response := make(map[string]HealthResponse)
	overallStatus := StatusHealthy

	for name, status := range results {
		response[name] = he.healthStatusToResponse(status)

		// Determine overall status
		switch {
		case status.Status == StatusUnhealthy:
			overallStatus = StatusUnhealthy
		case status.Status == StatusDegraded && overallStatus != StatusUnhealthy:
			overallStatus = StatusDegraded
		case status.Status == StatusUnknown && overallStatus == StatusHealthy:
			overallStatus = StatusUnknown
		}
	}

	statusCode := he.healthStatusToHTTPStatus(overallStatus)

	w.Header().Set(lift.HeaderContentType, lift.ContentTypeJSON)
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		he.logError("Failed to encode health check response", err)
	}
}

// healthStatusToHTTPStatus converts health status to HTTP status code
func (he *HealthEndpoints) healthStatusToHTTPStatus(status string) int {
	switch status {
	case StatusHealthy:
		return http.StatusOK
	case StatusDegraded:
		return http.StatusOK // Still serving traffic
	case StatusUnhealthy:
		return http.StatusServiceUnavailable
	case StatusUnknown:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// healthStatusToResponse converts HealthStatus to HealthResponse
func (he *HealthEndpoints) healthStatusToResponse(status HealthStatus) HealthResponse {
	response := HealthResponse{
		Status:    status.Status,
		Timestamp: status.Timestamp.Format(time.RFC3339),
		Duration:  status.Duration.String(),
		Message:   status.Message,
	}

	// Include details if enabled
	if he.enableDetailedErrors && status.Details != nil {
		response.Details = status.Details
	}

	// Include error if enabled
	if he.enableDetailedErrors && status.Error != "" {
		response.Error = status.Error
	}

	return response
}

// writeJSONResponse writes a JSON health response
func (he *HealthEndpoints) writeJSONResponse(w http.ResponseWriter, statusCode int, status HealthStatus) {
	response := he.healthStatusToResponse(status)

	w.Header().Set(lift.HeaderContentType, lift.ContentTypeJSON)
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		he.logError("Failed to encode health check response", err)
	}
}

// writePlainTextResponse writes a plain text health response
func (he *HealthEndpoints) writePlainTextResponse(w http.ResponseWriter, statusCode int, status HealthStatus) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(statusCode)

	message := status.Status
	if status.Message != "" {
		message = fmt.Sprintf("%s: %s", status.Status, status.Message)
	}

	if _, err := fmt.Fprint(w, message); err != nil {
		he.logError("Failed to write plain text response", err)
	}
}

// writeError writes an error response
func (he *HealthEndpoints) writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set(lift.HeaderContentType, lift.ContentTypeJSON)
	w.WriteHeader(statusCode)

	errorResponse := map[string]string{
		"error":     message,
		"status":    "error",
		"timestamp": time.Now().Format(time.RFC3339),
	}

	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		he.logError("Failed to encode error response", err)
	}
}

// setCORSHeaders sets CORS headers if enabled
func (he *HealthEndpoints) setCORSHeaders(w http.ResponseWriter) {
	if !he.enableCORS {
		return
	}

	origin := "*"
	if len(he.corsOrigins) > 0 {
		origin = strings.Join(he.corsOrigins, ",")
	}

	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
}

// wantsPlainText checks if the client prefers plain text response
func (he *HealthEndpoints) wantsPlainText(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/plain") ||
		strings.Contains(accept, "text/*") ||
		(accept == "" && r.URL.Query().Get("format") == "text")
}

// logError logs via structured logger if available, else falls back to std log
func (he *HealthEndpoints) logError(msg string, err error) {
	if he.logger != nil {
		he.logger.Error(msg, map[string]any{"error": err.Error()})
		return
	}
	log.Printf("%s: %v", msg, err)
}

// HealthMiddleware provides middleware for automatic health monitoring
type HealthMiddleware struct {
	manager            HealthManager
	headerName         string
	enableHealthHeader bool
}

// HealthMiddlewareConfig configures health middleware
type HealthMiddlewareConfig struct {
	HeaderName         string
	EnableHealthHeader bool
}

// NewHealthMiddleware creates new health middleware
func NewHealthMiddleware(manager HealthManager, config HealthMiddlewareConfig) *HealthMiddleware {
	return &HealthMiddleware{
		manager:            manager,
		enableHealthHeader: config.EnableHealthHeader,
		headerName:         config.HeaderName,
	}
}

// DefaultHealthMiddlewareConfig returns sensible defaults
func DefaultHealthMiddlewareConfig() HealthMiddlewareConfig {
	return HealthMiddlewareConfig{
		EnableHealthHeader: true,
		HeaderName:         "X-Health-Status",
	}
}

// Handler wraps an HTTP handler with health monitoring
func (hm *HealthMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add health status header if enabled
		if hm.enableHealthHeader {
			ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
			defer cancel()

			overall := hm.manager.OverallHealth(ctx)
			w.Header().Set(hm.headerName, overall.Status)
		}

		next.ServeHTTP(w, r)
	})
}
