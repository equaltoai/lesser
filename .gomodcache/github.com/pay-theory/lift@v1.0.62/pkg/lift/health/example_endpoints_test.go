package health_test

import (
	"context"
	"net/http"

	h "github.com/pay-theory/lift/pkg/lift/health"
)

// exampleChecker is a trivial HealthChecker used for documentation examples.
type exampleChecker struct{ name string }

func (e exampleChecker) Name() string { return e.name }
func (e exampleChecker) Check(_ context.Context) h.HealthStatus {
	return h.HealthStatus{Status: h.StatusHealthy}
}

// Example showing how to register health endpoints on an http.ServeMux.
func ExampleHealthEndpoints_RegisterRoutes() {
	// Create a health manager with defaults and register a custom checker.
	manager := h.NewHealthManager(h.DefaultHealthManagerConfig())
	_ = manager.RegisterChecker("database", exampleChecker{name: "database"})

	// Create endpoints with default configuration.
	endpoints := h.NewHealthEndpoints(manager, h.DefaultHealthEndpointsConfig())
	mux := http.NewServeMux()
	endpoints.RegisterRoutes(mux)

	// The mux now handles:
	//  GET /health
	//  GET /health/ready
	//  GET /health/live
	//  GET /health/components
}

// Example showing how to add a health status header via HealthMiddleware.
func ExampleHealthMiddleware_Handler() {
	manager := h.NewHealthManager(h.DefaultHealthManagerConfig())
	mw := h.NewHealthMiddleware(manager, h.DefaultHealthMiddlewareConfig())

	mux := http.NewServeMux()
	mux.Handle("/", mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})))
}
