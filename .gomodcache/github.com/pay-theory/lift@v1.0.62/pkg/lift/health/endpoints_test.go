package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthEndpoints_HealthHandler(t *testing.T) {
	manager := NewHealthManager(DefaultHealthManagerConfig())
	config := DefaultHealthEndpointsConfig()
	endpoints := NewHealthEndpoints(manager, config)

	// Register some checkers
	if err := manager.RegisterChecker("healthy", NewAlwaysHealthyChecker("healthy")); err != nil {
		t.Fatalf("Failed to register healthy checker: %v", err)
	}
	if err := manager.RegisterChecker("unhealthy", NewAlwaysUnhealthyChecker("unhealthy")); err != nil {
		t.Fatalf("Failed to register unhealthy checker: %v", err)
	}

	t.Run("JSON Response", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()

		endpoints.HealthHandler(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
		}

		var response HealthResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if response.Status != StatusUnhealthy {
			t.Errorf("expected status %s, got %s", StatusUnhealthy, response.Status)
		}
	})

	t.Run("Plain Text Response", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		req.Header.Set("Accept", "text/plain")
		w := httptest.NewRecorder()

		endpoints.HealthHandler(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
		}

		contentType := w.Header().Get("Content-Type")
		if contentType != "text/plain" {
			t.Errorf("expected content type text/plain, got %s", contentType)
		}

		body := strings.TrimSpace(w.Body.String())
		if !strings.Contains(body, StatusUnhealthy) {
			t.Errorf("expected body to contain %s, got %s", StatusUnhealthy, body)
		}
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/health", nil)
		w := httptest.NewRecorder()

		endpoints.HealthHandler(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
		}
	})
}

func TestHealthEndpoints_ReadinessHandler(t *testing.T) {
	manager := NewHealthManager(DefaultHealthManagerConfig())
	config := DefaultHealthEndpointsConfig()
	endpoints := NewHealthEndpoints(manager, config)

	t.Run("Healthy Service", func(t *testing.T) {
		err := manager.RegisterChecker("healthy", NewAlwaysHealthyChecker("healthy"))
		if err != nil {
			t.Fatalf("Failed to register checker: %v", err)
		}

		req := httptest.NewRequest("GET", "/health/ready", nil)
		w := httptest.NewRecorder()

		endpoints.ReadinessHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("Degraded Service (Still Ready)", func(t *testing.T) {
		err := manager.UnregisterChecker("healthy")
		if err != nil {
			t.Fatalf("Failed to unregister checker: %v", err)
		}
		degradedChecker := NewCustomHealthChecker("degraded", func(_ context.Context) HealthStatus {
			return HealthStatus{
				Status:    StatusDegraded,
				Timestamp: time.Now(),
				Duration:  time.Microsecond,
				Message:   "Degraded but ready",
			}
		})
		err = manager.RegisterChecker("degraded", degradedChecker)
		if err != nil {
			t.Fatalf("Failed to register checker: %v", err)
		}

		req := httptest.NewRequest("GET", "/health/ready", nil)
		w := httptest.NewRecorder()

		endpoints.ReadinessHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d for degraded service, got %d", http.StatusOK, w.Code)
		}
	})

	t.Run("Unhealthy Service", func(t *testing.T) {
		err := manager.UnregisterChecker("degraded")
		if err != nil {
			t.Fatalf("Failed to unregister checker: %v", err)
		}
		err = manager.RegisterChecker("unhealthy", NewAlwaysUnhealthyChecker("unhealthy"))
		if err != nil {
			t.Fatalf("Failed to register checker: %v", err)
		}

		req := httptest.NewRequest("GET", "/health/ready", nil)
		w := httptest.NewRecorder()

		endpoints.ReadinessHandler(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
		}
	})
}

func TestHealthEndpoints_LivenessHandler(t *testing.T) {
	manager := NewHealthManager(DefaultHealthManagerConfig())
	config := DefaultHealthEndpointsConfig()
	endpoints := NewHealthEndpoints(manager, config)

	req := httptest.NewRequest("GET", "/health/live", nil)
	w := httptest.NewRecorder()

	endpoints.LivenessHandler(w, req)

	// Liveness should always return OK (service is running)
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response.Status != StatusHealthy {
		t.Errorf("expected status %s, got %s", StatusHealthy, response.Status)
	}

	if response.Message != "Service is alive" {
		t.Errorf("expected message 'Service is alive', got %s", response.Message)
	}
}

func TestHealthEndpoints_ComponentsHandler(t *testing.T) {
	manager := NewHealthManager(DefaultHealthManagerConfig())
	config := DefaultHealthEndpointsConfig()
	endpoints := NewHealthEndpoints(manager, config)

	// Register test checkers
	if err := manager.RegisterChecker("healthy", NewAlwaysHealthyChecker("healthy")); err != nil {
		t.Fatalf("Failed to register healthy checker: %v", err)
	}
	if err := manager.RegisterChecker("unhealthy", NewAlwaysUnhealthyChecker("unhealthy")); err != nil {
		t.Fatalf("Failed to register unhealthy checker: %v", err)
	}

	t.Run("All Components", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health/components", nil)
		w := httptest.NewRecorder()

		endpoints.ComponentsHandler(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
		}

		var response map[string]HealthResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if len(response) != 2 {
			t.Errorf("expected 2 components, got %d", len(response))
		}

		if response["healthy"].Status != StatusHealthy {
			t.Errorf("expected healthy component to be healthy, got %s", response["healthy"].Status)
		}

		if response["unhealthy"].Status != StatusUnhealthy {
			t.Errorf("expected unhealthy component to be unhealthy, got %s", response["unhealthy"].Status)
		}
	})

	t.Run("Specific Component", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health/components?component=healthy", nil)
		w := httptest.NewRecorder()

		endpoints.ComponentsHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		var response HealthResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if response.Status != StatusHealthy {
			t.Errorf("expected status %s, got %s", StatusHealthy, response.Status)
		}
	})

	t.Run("Non-existent Component", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health/components?component=nonexistent", nil)
		w := httptest.NewRecorder()

		endpoints.ComponentsHandler(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestHealthEndpoints_RegisterRoutes(t *testing.T) {
	manager := NewHealthManager(DefaultHealthManagerConfig())
	config := DefaultHealthEndpointsConfig()
	endpoints := NewHealthEndpoints(manager, config)

	mux := http.NewServeMux()
	endpoints.RegisterRoutes(mux)

	// Test that routes are registered
	testCases := []struct {
		path           string
		expectedStatus int
	}{
		{"/health", http.StatusServiceUnavailable}, // No checkers registered
		{"/health/", http.StatusServiceUnavailable},
		{"/health/ready", http.StatusServiceUnavailable},
		{"/health/live", http.StatusOK},
		{"/health/components", http.StatusOK}, // Empty components list
	}

	for _, tc := range testCases {
		req := httptest.NewRequest("GET", tc.path, nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		// Just check that the route exists (doesn't return 404)
		if w.Code == http.StatusNotFound {
			t.Errorf("route %s not found", tc.path)
		}
	}
}

func TestHealthEndpoints_CORS(t *testing.T) {
	manager := NewHealthManager(DefaultHealthManagerConfig())
	config := DefaultHealthEndpointsConfig()
	config.EnableCORS = true
	config.CORSOrigins = []string{"https://example.com"}
	endpoints := NewHealthEndpoints(manager, config)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	endpoints.HealthHandler(w, req)

	corsOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if corsOrigin != "https://example.com" {
		t.Errorf("expected CORS origin 'https://example.com', got %s", corsOrigin)
	}

	corsMethod := w.Header().Get("Access-Control-Allow-Methods")
	if corsMethod != "GET, OPTIONS" {
		t.Errorf("expected CORS methods 'GET, OPTIONS', got %s", corsMethod)
	}
}

func TestHealthEndpoints_DetailedErrors(t *testing.T) {
	manager := NewHealthManager(DefaultHealthManagerConfig())

	t.Run("Detailed Errors Disabled", func(t *testing.T) {
		config := DefaultHealthEndpointsConfig()
		config.EnableDetailedErrors = false
		endpoints := NewHealthEndpoints(manager, config)

		if err := manager.RegisterChecker("unhealthy", NewAlwaysUnhealthyChecker("unhealthy")); err != nil {
			t.Fatalf("Failed to register checker: %v", err)
		}

		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()

		endpoints.HealthHandler(w, req)

		var response HealthResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if response.Error != "" {
			t.Error("expected no error details when disabled")
		}

		if response.Details != nil {
			t.Error("expected no details when disabled")
		}
	})

	t.Run("Detailed Errors Enabled", func(t *testing.T) {
		config := DefaultHealthEndpointsConfig()
		config.EnableDetailedErrors = true
		endpoints := NewHealthEndpoints(manager, config)

		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()

		endpoints.HealthHandler(w, req)

		var response HealthResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		// Should include details when enabled
		if response.Details == nil {
			t.Error("expected details when enabled")
		}
	})
}

func TestHealthMiddleware(t *testing.T) {
	manager := NewHealthManager(DefaultHealthManagerConfig())
	if err := manager.RegisterChecker("healthy", NewAlwaysHealthyChecker("healthy")); err != nil {
		t.Fatalf("Failed to register checker: %v", err)
	}

	config := DefaultHealthMiddlewareConfig()
	middleware := NewHealthMiddleware(manager, config)

	// Create a simple handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			// Log write error but don't fail since headers are already sent
			t.Logf("Failed to write response: %v", err)
		}
	})

	// Wrap with health middleware
	wrappedHandler := middleware.Handler(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	// Check that health header was added
	healthHeader := w.Header().Get("X-Health-Status")
	if healthHeader != StatusHealthy {
		t.Errorf("expected health header %s, got %s", StatusHealthy, healthHeader)
	}

	// Check that original response is preserved
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Body.String() != "OK" {
		t.Errorf("expected body 'OK', got %s", w.Body.String())
	}
}

func TestHealthEndpoints_StatusMapping(t *testing.T) {
	manager := NewHealthManager(DefaultHealthManagerConfig())
	config := DefaultHealthEndpointsConfig()
	endpoints := NewHealthEndpoints(manager, config)

	testCases := []struct {
		status       string
		expectedHTTP int
	}{
		{StatusHealthy, http.StatusOK},
		{StatusDegraded, http.StatusOK},
		{StatusUnhealthy, http.StatusServiceUnavailable},
		{StatusUnknown, http.StatusServiceUnavailable},
		{"invalid", http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		httpStatus := endpoints.healthStatusToHTTPStatus(tc.status)
		if httpStatus != tc.expectedHTTP {
			t.Errorf("status %s: expected HTTP %d, got %d", tc.status, tc.expectedHTTP, httpStatus)
		}
	}
}

func BenchmarkHealthEndpoints_HealthHandler(b *testing.B) {
	manager := NewHealthManager(DefaultHealthManagerConfig())
	if err := manager.RegisterChecker("test", NewAlwaysHealthyChecker("test")); err != nil {
		b.Fatalf("Failed to register checker: %v", err)
	}

	config := DefaultHealthEndpointsConfig()
	endpoints := NewHealthEndpoints(manager, config)

	req := httptest.NewRequest("GET", "/health", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		endpoints.HealthHandler(w, req)
	}
}

func BenchmarkHealthMiddleware_Handler(b *testing.B) {
	manager := NewHealthManager(DefaultHealthManagerConfig())
	if err := manager.RegisterChecker("test", NewAlwaysHealthyChecker("test")); err != nil {
		b.Fatalf("Failed to register checker: %v", err)
	}

	config := DefaultHealthMiddlewareConfig()
	middleware := NewHealthMiddleware(manager, config)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware.Handler(handler)
	req := httptest.NewRequest("GET", "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)
	}
}
