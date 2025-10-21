package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/pay-theory/lift/pkg/features"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/services"
)

const (
	userService1 = "user-service-1"
	userService2 = "user-service-2"
)

// DemoServiceRegistry implements a simple in-memory service discovery
type DemoServiceRegistry struct {
	services map[string][]*services.ServiceInstance
}

func NewDemoServiceRegistry() *DemoServiceRegistry {
	return &DemoServiceRegistry{
		services: make(map[string][]*services.ServiceInstance),
	}
}

func (d *DemoServiceRegistry) Register(_ context.Context, config *services.ServiceConfig) error {
	instance := &services.ServiceInstance{
		ID:          fmt.Sprintf("%s-%d", config.Name, time.Now().Unix()),
		ServiceName: config.Name,
		Version:     config.Version,
		Endpoint:    config.Endpoints[0],
		Health: services.HealthStatus{
			Status:    "healthy",
			Message:   "Service is running",
			Timestamp: time.Now(),
		},
		Metadata: config.Metadata,
		TenantID: config.TenantID,
		Weight:   config.Weight,
		LastSeen: time.Now(),
	}

	d.services[config.Name] = append(d.services[config.Name], instance)
	fmt.Printf("✅ Registered service: %s (instance: %s)\n", config.Name, instance.ID)
	return nil
}

func (d *DemoServiceRegistry) Deregister(_ context.Context, _ string) error {
	// Simple implementation - remove all instances for now
	for serviceName := range d.services {
		delete(d.services, serviceName)
	}
	return nil
}

func (d *DemoServiceRegistry) Discover(_ context.Context, serviceName string) ([]*services.ServiceInstance, error) {
	instances, exists := d.services[serviceName]
	if !exists {
		return nil, fmt.Errorf("service not found: %s", serviceName)
	}

	// Return copies to prevent external modification
	result := make([]*services.ServiceInstance, len(instances))
	copy(result, instances)

	return result, nil
}

func (d *DemoServiceRegistry) Watch(ctx context.Context, serviceName string) (<-chan []*services.ServiceInstance, error) {
	ch := make(chan []*services.ServiceInstance, 1)

	// Send initial state
	if instances, err := d.Discover(ctx, serviceName); err == nil {
		ch <- instances
	}

	return ch, nil
}

func (d *DemoServiceRegistry) HealthCheck(_ context.Context, _ *services.ServiceInstance) (*services.HealthStatus, error) {
	return &services.HealthStatus{
		Status:    "healthy",
		Message:   "Health check passed",
		Timestamp: time.Now(),
	}, nil
}

// Demo application
func main() {
	fmt.Println("🚀 Multi-Service Architecture Demo")
	fmt.Println("===================================")

	// Create service discovery backend
	discovery := NewDemoServiceRegistry()

	// Create load balancer
	loadBalancer := services.NewDefaultLoadBalancer()

	// Create service registry
	registryConfig := services.RegistryConfig{
		EnableCaching:       true,
		CacheTTL:            5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		EnableMetrics:       true,
		TenantIsolation:     true,
		MaxRetries:          3,
		RetryBackoff:        1 * time.Second,
	}

	registry := services.NewServiceRegistry(registryConfig, discovery, loadBalancer)

	// Register demo services
	registerDemoServices(registry)

	// Create service client
	clientConfig := services.ServiceClientConfig{
		DefaultTimeout:       30 * time.Second,
		MaxRetries:           3,
		RetryBackoff:         1 * time.Second,
		EnableTracing:        true,
		EnableMetrics:        true,
		EnableCircuitBreaker: true,
		TenantIsolation:      true,
		UserAgent:            "lift-demo/1.0",
	}

	serviceClient := services.NewServiceClient(registry, clientConfig)

	// Create Lift application
	app := lift.New()

	// Add service client middleware
	app.Use(services.ServiceClientMiddleware(serviceClient))

	// Add validation middleware for user creation
	userSchema := features.NewSchema().
		AddProperty("email", features.EmailValidation()).
		AddProperty("name", features.ValidationRule{
			Type:     "string",
			Required: true,
			Min:      1,
			Message:  "Name is required and must not be empty",
		}).
		AddRequired("email", "name")

	validationConfig := features.ValidationConfig{
		RequestSchema:   userSchema,
		ValidateRequest: true,
		StrictMode:      true,
	}
	app.Use(features.NewValidationMiddleware(validationConfig).Validate())

	// Demo routes
	setupDemoRoutes(app)

	// Start demo server
	fmt.Println("\n🌐 Starting demo server on :8080")
	fmt.Println("Try these endpoints:")
	fmt.Println("  GET  /demo/services - List registered services")
	fmt.Println("  GET  /demo/discovery - Test service discovery")
	fmt.Println("  GET  /demo/loadbalancer - Test load balancing")
	fmt.Println("  POST /demo/service-call - Test inter-service communication")
	fmt.Println("  GET  /demo/stats - View performance statistics")

	server := &http.Server{
		Addr:              ":8080",
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Convert HTTP request to Lift context
			ctx := createLiftContext(r)

			// Handle request through the app's test handler
			if err := app.HandleTestRequest(ctx); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}

			// Write response
			writeResponse(w, ctx.Response)
		}),
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func registerDemoServices(registry *services.ServiceRegistry) {
	ctx := context.Background()

	// Register user service instances
	userService1 := &services.ServiceConfig{
		Name:    "user-service",
		Version: "1.0.0",
		Endpoints: []services.ServiceEndpoint{
			{
				Protocol: "http",
				Host:     "user-service-1.internal",
				Port:     8080,
				Path:     "/api/v1",
			},
		},
		HealthCheck: services.HealthCheckConfig{
			Enabled:  true,
			Path:     "/health",
			Interval: 30 * time.Second,
			Timeout:  5 * time.Second,
		},
		Metadata: map[string]string{
			"region": "us-east-1",
			"zone":   "us-east-1a",
		},
		TenantID:    "tenant-1",
		Tags:        []string{"api", "users"},
		Weight:      100,
		Environment: "production",
	}

	userService2 := &services.ServiceConfig{
		Name:    "user-service",
		Version: "1.0.0",
		Endpoints: []services.ServiceEndpoint{
			{
				Protocol: "http",
				Host:     "user-service-2.internal",
				Port:     8080,
				Path:     "/api/v1",
			},
		},
		HealthCheck: services.HealthCheckConfig{
			Enabled:  true,
			Path:     "/health",
			Interval: 30 * time.Second,
			Timeout:  5 * time.Second,
		},
		Metadata: map[string]string{
			"region": "us-east-1",
			"zone":   "us-east-1b",
		},
		TenantID:    "tenant-1",
		Tags:        []string{"api", "users"},
		Weight:      150, // Higher weight
		Environment: "production",
	}

	// Register payment service
	paymentService := &services.ServiceConfig{
		Name:    "payment-service",
		Version: "2.1.0",
		Endpoints: []services.ServiceEndpoint{
			{
				Protocol: "https",
				Host:     "payment-service.internal",
				Port:     8443,
				Path:     "/api/v2",
			},
		},
		HealthCheck: services.HealthCheckConfig{
			Enabled:  true,
			Path:     "/health",
			Interval: 15 * time.Second,
			Timeout:  3 * time.Second,
		},
		Metadata: map[string]string{
			"region":   "us-east-1",
			"zone":     "us-east-1a",
			"security": "high",
		},
		TenantID:    "tenant-1",
		Tags:        []string{"api", "payments", "secure"},
		Weight:      200,
		Environment: "production",
	}

	// Register services
	if err := registry.Register(ctx, userService1); err != nil {
		log.Printf("Failed to register userService1: %v", err)
	}
	if err := registry.Register(ctx, userService2); err != nil {
		log.Printf("Failed to register userService2: %v", err)
	}
	if err := registry.Register(ctx, paymentService); err != nil {
		log.Printf("Failed to register paymentService: %v", err)
	}
}

func setupDemoRoutes(app *lift.App) {
	// Helpers to reduce repetitive error checks
	mustGET := func(path string, h func(*lift.Context) error) {
		if err := app.GET(path, h); err != nil {
			log.Fatalf("Failed to register GET %s: %v", path, err)
		}
	}
	mustPOST := func(path string, h func(*lift.Context) error) {
		if err := app.POST(path, h); err != nil {
			log.Fatalf("Failed to register POST %s: %v", path, err)
		}
	}

	// List registered services
	mustGET("/demo/services", func(ctx *lift.Context) error {
		// Get service client from context
		client := services.GetServiceClient(ctx)
		if client == nil {
			return ctx.Status(500).JSON(map[string]string{
				"error": "Service client not available",
			})
		}

		// Get registry from client (simplified for demo)
		services := []map[string]any{
			{
				"name":      "user-service",
				"version":   "1.0.0",
				"instances": 2,
				"status":    "healthy",
			},
			{
				"name":      "payment-service",
				"version":   "2.1.0",
				"instances": 1,
				"status":    "healthy",
			},
		}

		return ctx.JSON(map[string]any{
			"services": services,
			"total":    len(services),
		})
	})

	// Test service discovery
	mustGET("/demo/discovery", func(ctx *lift.Context) error {
		serviceName := ctx.Query("service")
		if serviceName == "" {
			serviceName = "user-service"
		}

		client := services.GetServiceClient(ctx)
		if client == nil {
			return ctx.Status(500).JSON(map[string]string{
				"error": "Service client not available",
			})
		}

		// Simulate service discovery
		discoveryResult := map[string]any{
			"service": serviceName,
			"instances": []map[string]any{
				{
					"id":     userService1,
					"host":   "user-service-1.internal",
					"port":   8080,
					"weight": 100,
					"health": "healthy",
					"region": "us-east-1",
					"zone":   "us-east-1a",
				},
				{
					"id":     "user-service-2",
					"host":   "user-service-2.internal",
					"port":   8080,
					"weight": 150,
					"health": "healthy",
					"region": "us-east-1",
					"zone":   "us-east-1b",
				},
			},
			"strategy": "round_robin",
			"selected": "user-service-2", // Higher weight
		}

		return ctx.JSON(discoveryResult)
	})

	// Test load balancing
	mustGET("/demo/loadbalancer", func(ctx *lift.Context) error {
		strategy := ctx.Query("strategy")
		if strategy == "" {
			strategy = "round_robin"
		}

		// Simulate load balancing results
		results := []map[string]any{}

		for i := 0; i < 10; i++ {
			var selected string
			switch strategy {
			case "weighted_random":
				if i%3 == 0 {
					selected = userService1
				} else {
					selected = "user-service-2" // Higher weight
				}
			case "least_connections":
				selected = userService1 // Assume fewer connections
			default: // round_robin
				if i%2 == 0 {
					selected = userService1
				} else {
					selected = "user-service-2"
				}
			}

			results = append(results, map[string]any{
				"request":  i + 1,
				"selected": selected,
			})
		}

		return ctx.JSON(map[string]any{
			"strategy": strategy,
			"requests": 10,
			"results":  results,
		})
	})

	// Test inter-service communication
	mustPOST("/demo/service-call", func(ctx *lift.Context) error {
		var request struct {
			Data    any    `json:"data"`
			Service string `json:"service"`
			Method  string `json:"method"`
			Path    string `json:"path"`
		}

		if err := ctx.ParseRequest(&request); err != nil {
			return ctx.Status(400).JSON(map[string]string{
				"error": "Invalid request format",
			})
		}

		// Simulate service call
		serviceCall := map[string]any{
			"request": map[string]any{
				"service":    request.Service,
				"method":     request.Method,
				"path":       request.Path,
				"data":       request.Data,
				"tenant_id":  ctx.TenantID(),
				"request_id": fmt.Sprintf("req_%d", time.Now().UnixNano()),
			},
			"discovery": map[string]any{
				"instance_id": "user-service-2",
				"endpoint":    "http://user-service-2.internal:8080/api/v1",
				"strategy":    "round_robin",
				"duration_ms": 2,
			},
			"response": map[string]any{
				"status_code": 200,
				"duration_ms": 45,
				"body": map[string]any{
					"id":    "user-123",
					"email": "demo@example.com",
					"name":  "Demo User",
				},
			},
			"metrics": map[string]any{
				"cache_hit":       true,
				"circuit_breaker": "closed",
				"retries":         0,
				"total_duration":  47,
			},
		}

		return ctx.JSON(serviceCall)
	})

	// Performance statistics
	mustGET("/demo/stats", func(ctx *lift.Context) error {
		stats := map[string]any{
			"service_registry": map[string]any{
				"registered_services": 2,
				"total_instances":     3,
				"healthy_instances":   3,
				"cache_hit_rate":      0.85,
			},
			"load_balancer": map[string]any{
				"total_requests":        1250,
				"successful_selections": 1248,
				"failed_selections":     2,
				"average_latency_ns":    850000,
			},
			"service_client": map[string]any{
				"total_calls":      456,
				"successful_calls": 452,
				"failed_calls":     4,
				"average_duration": 45,
				"circuit_breaker":  "closed",
				"cache_hit_rate":   0.78,
			},
			"caching": map[string]any{
				"hits":            1890,
				"misses":          234,
				"hit_rate":        0.89,
				"average_latency": "0.8µs",
				"memory_usage":    "2.4MB",
			},
		}

		return ctx.JSON(stats)
	})
}

// Helper functions

func createLiftContext(r *http.Request) *lift.Context {
	// Create a basic Lift context from HTTP request
	request := &lift.Request{
		Method:      r.Method,
		Path:        r.URL.Path,
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
	}

	// Copy headers
	for key, values := range r.Header {
		if len(values) > 0 {
			request.Headers[key] = values[0]
		}
	}

	// Copy query parameters
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			request.QueryParams[key] = values[0]
		}
	}

	// Read body if present
	if r.Body != nil {
		defer func() {
			if err := r.Body.Close(); err != nil {
				log.Printf("Warning: failed to close request body: %v", err)
			}
		}()
		if bodyBytes, err := json.Marshal(r.Body); err == nil {
			request.Body = bodyBytes
		}
	}

	ctx := lift.NewContext(r.Context(), request)

	// Set tenant ID from header
	if tenantID := r.Header.Get("X-Tenant-ID"); tenantID != "" {
		ctx.SetTenantID(tenantID)
	} else {
		ctx.SetTenantID("tenant-1") // Default for demo
	}

	return ctx
}

func writeResponse(w http.ResponseWriter, response *lift.Response) {
	// Set headers
	for key, value := range response.Headers {
		w.Header().Set(key, value)
	}

	// Set status code
	w.WriteHeader(response.StatusCode)

	// Write body
	if response.Body != nil {
		if bodyBytes, err := json.Marshal(response.Body); err == nil {
			if _, err := w.Write(bodyBytes); err != nil {
				// Log write error but don't return it since we're in a test handler
				log.Printf("Warning: failed to write response body: %v", err)
			}
		}
	}
}
