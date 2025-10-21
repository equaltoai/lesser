package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pay-theory/lift/pkg/lift/health"
	"github.com/pay-theory/lift/pkg/lift/resources"
)

// User represents a user in our system
type User struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	ID        int       `json:"id"`
}

// CreateUserRequest represents the request to create a user
type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// UpdateUserRequest represents the request to update a user
type UpdateUserRequest struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

// APIError represents an API error
type APIError struct {
	Details any    `json:"details,omitempty"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (e APIError) Error() string {
	return e.Message
}

// UserService handles user operations
type UserService struct {
	pool   resources.ConnectionPool
	health health.HealthManager
	users  map[int]*User
	nextID int
}

// NewUserService creates a new user service
func NewUserService(pool resources.ConnectionPool, healthManager health.HealthManager) *UserService {
	return &UserService{
		users:  make(map[int]*User),
		nextID: 1,
		pool:   pool,
		health: healthManager,
	}
}

// CreateUser creates a new user
func (s *UserService) CreateUser(ctx context.Context, req CreateUserRequest) (*User, error) {
	// Validate request
	if req.Name == "" {
		return nil, APIError{
			Type:    "validation",
			Message: "name is required",
			Details: map[string]string{"field": "name"},
		}
	}

	if req.Email == "" {
		return nil, APIError{
			Type:    "validation",
			Message: "email is required",
			Details: map[string]string{"field": "email"},
		}
	}

	// Check if email already exists
	for _, user := range s.users {
		if user.Email == req.Email {
			return nil, APIError{
				Type:    "conflict",
				Message: "email already exists",
				Details: map[string]any{"email": req.Email},
			}
		}
	}

	// Get resource from pool (simulating database connection)
	resource, err := s.pool.Get(ctx)
	if err != nil {
		return nil, APIError{
			Type:    "internal",
			Message: "failed to get database connection",
			Details: err.Error(),
		}
	}
	defer func() {
		if err := s.pool.Put(resource); err != nil {
			log.Printf("Error returning resource to pool: %v", err)
		}
	}()

	// Create user
	user := &User{
		ID:        s.nextID,
		Name:      req.Name,
		Email:     req.Email,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	s.users[user.ID] = user
	s.nextID++

	return user, nil
}

// GetUser retrieves a user by ID
func (s *UserService) GetUser(ctx context.Context, id int) (*User, error) {
	// Get resource from pool
	resource, err := s.pool.Get(ctx)
	if err != nil {
		return nil, APIError{
			Type:    "internal",
			Message: "failed to get database connection",
			Details: err.Error(),
		}
	}
	defer func() {
		if err := s.pool.Put(resource); err != nil {
			log.Printf("Error returning resource to pool: %v", err)
		}
	}()

	user, exists := s.users[id]
	if !exists {
		return nil, APIError{
			Type:    "not_found",
			Message: "user not found",
			Details: map[string]any{"user_id": id},
		}
	}

	return user, nil
}

// UpdateUser updates an existing user
func (s *UserService) UpdateUser(ctx context.Context, id int, req UpdateUserRequest) (*User, error) {
	// Get resource from pool
	resource, err := s.pool.Get(ctx)
	if err != nil {
		return nil, APIError{
			Type:    "internal",
			Message: "failed to get database connection",
			Details: err.Error(),
		}
	}
	defer func() {
		if err := s.pool.Put(resource); err != nil {
			log.Printf("Error returning resource to pool: %v", err)
		}
	}()

	user, exists := s.users[id]
	if !exists {
		return nil, APIError{
			Type:    "not_found",
			Message: "user not found",
			Details: map[string]any{"user_id": id},
		}
	}

	// Check email uniqueness if updating email
	if req.Email != "" && req.Email != user.Email {
		for _, existingUser := range s.users {
			if existingUser.Email == req.Email {
				return nil, APIError{
					Type:    "conflict",
					Message: "email already exists",
					Details: map[string]any{"email": req.Email},
				}
			}
		}
	}

	// Update user
	if req.Name != "" {
		user.Name = req.Name
	}
	if req.Email != "" {
		user.Email = req.Email
	}
	user.UpdatedAt = time.Now()

	return user, nil
}

// DeleteUser deletes a user
func (s *UserService) DeleteUser(ctx context.Context, id int) error {
	// Get resource from pool
	resource, err := s.pool.Get(ctx)
	if err != nil {
		return APIError{
			Type:    "internal",
			Message: "failed to get database connection",
			Details: err.Error(),
		}
	}
	defer func() {
		if err := s.pool.Put(resource); err != nil {
			log.Printf("Error returning resource to pool: %v", err)
		}
	}()

	_, exists := s.users[id]
	if !exists {
		return APIError{
			Type:    "not_found",
			Message: "user not found",
			Details: map[string]any{"user_id": id},
		}
	}

	delete(s.users, id)
	return nil
}

// ListUsers lists all users
func (s *UserService) ListUsers(ctx context.Context) ([]*User, error) {
	// Get resource from pool
	resource, err := s.pool.Get(ctx)
	if err != nil {
		return nil, APIError{
			Type:    "internal",
			Message: "failed to get database connection",
			Details: err.Error(),
		}
	}
	defer func() {
		if err := s.pool.Put(resource); err != nil {
			log.Printf("Error returning resource to pool: %v", err)
		}
	}()

	users := make([]*User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}

	return users, nil
}

// ProductionAPI represents our complete production API
type ProductionAPI struct {
	userService      *UserService
	healthManager    health.HealthManager
	healthEndpoints  *health.HealthEndpoints
	healthMiddleware *health.HealthMiddleware
	resourceManager  *resources.ResourceManager
}

// MockResource implements the resources.Resource interface for demonstration
type MockResource struct {
	lastUsed time.Time
	id       string
	valid    bool
}

func (r *MockResource) Initialize(_ context.Context) error {
	r.lastUsed = time.Now()
	r.valid = true
	return nil
}

func (r *MockResource) HealthCheck(_ context.Context) error {
	if !r.valid {
		return fmt.Errorf("resource %s is invalid", r.id)
	}
	return nil
}

func (r *MockResource) Cleanup() error {
	r.valid = false
	return nil
}

func (r *MockResource) IsValid() bool {
	return r.valid && time.Since(r.lastUsed) < 5*time.Minute
}

func (r *MockResource) LastUsed() time.Time {
	return r.lastUsed
}

func (r *MockResource) MarkUsed() {
	r.lastUsed = time.Now()
}

// MockResourceFactory creates mock resources
type MockResourceFactory struct{}

func (f *MockResourceFactory) Create(_ context.Context) (resources.Resource, error) {
	resource := &MockResource{
		id:       fmt.Sprintf("resource-%d", time.Now().UnixNano()),
		lastUsed: time.Now(),
		valid:    true,
	}
	return resource, nil
}

func (f *MockResourceFactory) Validate(resource resources.Resource) bool {
	return resource.IsValid()
}

// NewProductionAPI creates a new production API with all integrations
func NewProductionAPI() *ProductionAPI {
	// 1. Setup Resource Management
	poolConfig := resources.DefaultPoolConfig()
	poolConfig.MaxActive = 20
	poolConfig.MaxIdle = 10
	poolConfig.MinIdle = 5
	poolConfig.IdleTimeout = 5 * time.Minute

	factory := &MockResourceFactory{}
	pool := resources.NewConnectionPool(poolConfig, factory)

	resourceManagerConfig := resources.DefaultResourceManagerConfig()
	resourceManager := resources.NewResourceManager(resourceManagerConfig)
	if err := resourceManager.RegisterPool("database", pool); err != nil {
		log.Printf("Error registering database pool: %v", err)
	}

	// Pre-warm the pool
	preWarmer := resources.NewDefaultPreWarmer("database", 5, 10*time.Second)
	if err := resourceManager.RegisterPreWarmer("database", preWarmer); err != nil {
		log.Printf("Error registering pre-warmer: %v", err)
	}
	if err := resourceManager.PreWarmAll(context.Background()); err != nil {
		log.Printf("Error pre-warming resources: %v", err)
	}

	// 2. Setup Health Monitoring
	healthConfig := health.DefaultHealthManagerConfig()
	healthConfig.ParallelChecks = true
	healthConfig.CacheEnabled = true
	healthConfig.CacheDuration = 30 * time.Second
	healthManager := health.NewHealthManager(healthConfig)

	// Register health checkers
	if err := healthManager.RegisterChecker("memory", health.NewMemoryHealthChecker("memory")); err != nil {
		log.Printf("Error registering memory health checker: %v", err)
	}
	if err := healthManager.RegisterChecker("database-pool", health.NewPoolHealthChecker("database-pool", pool)); err != nil {
		log.Printf("Error registering database pool health checker: %v", err)
	}

	// Custom business logic health checker
	businessChecker := health.NewCustomHealthChecker("business-logic", func(_ context.Context) health.HealthStatus {
		start := time.Now()

		// Check if we have users (business logic validation)
		poolStats := pool.Stats()
		userCount := poolStats.Active

		status := health.StatusHealthy
		message := "Business logic is healthy"

		if userCount > 15 { // Simulate high load
			status = health.StatusDegraded
			message = "High load detected - performance may be impacted"
		}

		return health.HealthStatus{
			Status:    status,
			Timestamp: time.Now(),
			Duration:  time.Since(start),
			Message:   message,
			Details: map[string]any{
				"active_connections": userCount,
				"load_threshold":     15,
			},
		}
	})
	if err := healthManager.RegisterChecker("business-logic", businessChecker); err != nil {
		log.Printf("Error registering business logic health checker: %v", err)
	}

	// HTTP service health checker (external dependency)
	httpChecker := health.NewHTTPHealthChecker("external-api", "https://httpbin.org/status/200")
	if err := healthManager.RegisterChecker("external-api", httpChecker); err != nil {
		log.Printf("Error registering external API health checker: %v", err)
	}

	// 3. Setup Health Endpoints
	endpointsConfig := health.DefaultHealthEndpointsConfig()
	endpointsConfig.EnableDetailedErrors = true // Enable for demo
	healthEndpoints := health.NewHealthEndpoints(healthManager, endpointsConfig)

	// 4. Setup Health Middleware
	middlewareConfig := health.DefaultHealthMiddlewareConfig()
	healthMiddleware := health.NewHealthMiddleware(healthManager, middlewareConfig)

	// 5. Create User Service
	userService := NewUserService(pool, healthManager)

	return &ProductionAPI{
		userService:      userService,
		healthManager:    healthManager,
		healthEndpoints:  healthEndpoints,
		healthMiddleware: healthMiddleware,
		resourceManager:  resourceManager,
	}
}

// HTTP Handlers

func (api *ProductionAPI) createUserHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeErrorResponse(w, APIError{
			Type:    "validation",
			Message: "invalid JSON",
			Details: err.Error(),
		})
		return
	}

	user, err := api.userService.CreateUser(ctx, req)
	if err != nil {
		api.writeErrorResponse(w, err)
		return
	}

	api.writeJSONResponse(w, http.StatusCreated, user)
}

func (api *ProductionAPI) getUserHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := strings.TrimPrefix(r.URL.Path, "/api/users/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		api.writeErrorResponse(w, APIError{
			Type:    "validation",
			Message: "invalid user ID",
			Details: err.Error(),
		})
		return
	}

	user, err := api.userService.GetUser(ctx, id)
	if err != nil {
		api.writeErrorResponse(w, err)
		return
	}

	api.writeJSONResponse(w, http.StatusOK, user)
}

func (api *ProductionAPI) updateUserHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := strings.TrimPrefix(r.URL.Path, "/api/users/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		api.writeErrorResponse(w, APIError{
			Type:    "validation",
			Message: "invalid user ID",
			Details: err.Error(),
		})
		return
	}

	var req UpdateUserRequest
	if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
		api.writeErrorResponse(w, APIError{
			Type:    "validation",
			Message: "invalid JSON",
			Details: decodeErr.Error(),
		})
		return
	}

	user, err := api.userService.UpdateUser(ctx, id, req)
	if err != nil {
		api.writeErrorResponse(w, err)
		return
	}

	api.writeJSONResponse(w, http.StatusOK, user)
}

func (api *ProductionAPI) deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := strings.TrimPrefix(r.URL.Path, "/api/users/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		api.writeErrorResponse(w, APIError{
			Type:    "validation",
			Message: "invalid user ID",
			Details: err.Error(),
		})
		return
	}

	err = api.userService.DeleteUser(ctx, id)
	if err != nil {
		api.writeErrorResponse(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (api *ProductionAPI) listUsersHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	users, err := api.userService.ListUsers(ctx)
	if err != nil {
		api.writeErrorResponse(w, err)
		return
	}

	api.writeJSONResponse(w, http.StatusOK, users)
}

func (api *ProductionAPI) metricsHandler(w http.ResponseWriter, r *http.Request) {
	// Get pool statistics
	allStats := api.resourceManager.Stats()
	poolStats := allStats["database"]

	// Get health status
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	overall := api.healthManager.OverallHealth(ctx)

	metrics := map[string]any{
		"timestamp": time.Now().Format(time.RFC3339),
		"health": map[string]any{
			"status":   overall.Status,
			"duration": overall.Duration.String(),
			"message":  overall.Message,
		},
		"resources": map[string]any{
			"database_pool": map[string]any{
				"active":   poolStats.Active,
				"idle":     poolStats.Idle,
				"total":    poolStats.Total,
				"gets":     poolStats.Gets,
				"puts":     poolStats.Puts,
				"hits":     poolStats.Hits,
				"misses":   poolStats.Misses,
				"timeouts": poolStats.Timeouts,
				"errors":   poolStats.Errors,
			},
		},
		"performance": map[string]any{
			"user_count": len(api.userService.users),
		},
	}

	api.writeJSONResponse(w, http.StatusOK, metrics)
}

func (api *ProductionAPI) statusHandler(w http.ResponseWriter, _ *http.Request) {
	status := map[string]any{
		"service":   "production-api",
		"version":   "1.0.0",
		"timestamp": time.Now().Format(time.RFC3339),
		"uptime":    time.Since(time.Now().Add(-time.Hour)).String(), // Mock uptime
		"features": []string{
			"resource-management",
			"health-monitoring",
			"performance-optimization",
		},
	}

	api.writeJSONResponse(w, http.StatusOK, status)
}

// Helper methods

func (api *ProductionAPI) writeJSONResponse(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

func (api *ProductionAPI) writeErrorResponse(w http.ResponseWriter, err error) {
	// Map error types to HTTP status codes
	statusCode := http.StatusInternalServerError

	if apiErr, ok := err.(APIError); ok {
		switch apiErr.Type {
		case "validation":
			statusCode = http.StatusBadRequest
		case "not_found":
			statusCode = http.StatusNotFound
		case "conflict":
			statusCode = http.StatusConflict
		case "internal":
			statusCode = http.StatusInternalServerError
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if encErr := json.NewEncoder(w).Encode(apiErr); encErr != nil {
			log.Printf("Error encoding JSON error response: %v", encErr)
		}
	} else {
		// Generic error
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if encErr := json.NewEncoder(w).Encode(APIError{
			Type:    "internal",
			Message: err.Error(),
		}); encErr != nil {
			log.Printf("Error encoding JSON error response: %v", encErr)
		}
	}
}

// Setup routes
func (api *ProductionAPI) setupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// Health endpoints
	api.healthEndpoints.RegisterRoutes(mux)

	// API endpoints with health middleware
	mux.Handle("/api/users", api.healthMiddleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			api.createUserHandler(w, r)
		case http.MethodGet:
			api.listUsersHandler(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))

	// User by ID endpoints
	mux.Handle("/api/users/", api.healthMiddleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			api.getUserHandler(w, r)
		case http.MethodPut:
			api.updateUserHandler(w, r)
		case http.MethodDelete:
			api.deleteUserHandler(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))

	// Metrics and status endpoints
	mux.Handle("/metrics", api.healthMiddleware.Handler(http.HandlerFunc(api.metricsHandler)))
	mux.Handle("/status", api.healthMiddleware.Handler(http.HandlerFunc(api.statusHandler)))

	// Root endpoint with documentation
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		if _, err := fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>Lift Production API</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .endpoint { margin: 10px 0; padding: 10px; background: #f5f5f5; border-radius: 5px; }
        .method { font-weight: bold; color: #0066cc; }
        .desc { color: #666; margin-top: 5px; }
        .feature { background: #e8f5e8; padding: 5px; margin: 5px 0; border-radius: 3px; }
    </style>
</head>
<body>
    <h1>🚀 Lift Production API</h1>
    <p>A comprehensive production-ready API showcasing Lift framework capabilities.</p>
    
    <h2>🏥 Health Endpoints</h2>
    <div class="endpoint">
        <span class="method">GET</span> <a href="/health">/health</a>
        <div class="desc">Overall health status</div>
    </div>
    <div class="endpoint">
        <span class="method">GET</span> <a href="/health/ready">/health/ready</a>
        <div class="desc">Kubernetes readiness probe</div>
    </div>
    <div class="endpoint">
        <span class="method">GET</span> <a href="/health/live">/health/live</a>
        <div class="desc">Kubernetes liveness probe</div>
    </div>
    <div class="endpoint">
        <span class="method">GET</span> <a href="/health/components">/health/components</a>
        <div class="desc">Individual component health</div>
    </div>
    
    <h2>👥 User Management API</h2>
    <div class="endpoint">
        <span class="method">POST</span> /api/users
        <div class="desc">Create a new user</div>
    </div>
    <div class="endpoint">
        <span class="method">GET</span> /api/users
        <div class="desc">List all users</div>
    </div>
    <div class="endpoint">
        <span class="method">GET</span> /api/users/{id}
        <div class="desc">Get user by ID</div>
    </div>
    <div class="endpoint">
        <span class="method">PUT</span> /api/users/{id}
        <div class="desc">Update user by ID</div>
    </div>
    <div class="endpoint">
        <span class="method">DELETE</span> /api/users/{id}
        <div class="desc">Delete user by ID</div>
    </div>
    
    <h2>📊 Monitoring Endpoints</h2>
    <div class="endpoint">
        <span class="method">GET</span> <a href="/metrics">/metrics</a>
        <div class="desc">Performance metrics and statistics</div>
    </div>
    <div class="endpoint">
        <span class="method">GET</span> <a href="/status">/status</a>
        <div class="desc">Service status and information</div>
    </div>
    
    <h2>🎯 Integrated Features</h2>
    <div class="feature">✅ <strong>Resource Management:</strong> Connection pooling with health monitoring</div>
    <div class="feature">✅ <strong>Health Monitoring:</strong> Multi-component health checking with caching</div>
    <div class="feature">✅ <strong>Performance:</strong> Sub-millisecond response times with minimal overhead</div>
    <div class="feature">✅ <strong>Type Safety:</strong> Comprehensive request/response validation</div>
    <div class="feature">✅ <strong>Observability:</strong> Request tracing and performance metrics</div>
    
    <h2>🧪 Try It Out</h2>
    <p>Example API calls:</p>
    <pre>
# Create a user
curl -X POST http://localhost:8080/api/users \
  -H "Content-Type: application/json" \
  -d '{"name": "John Doe", "email": "john@example.com"}'

# Get all users
curl http://localhost:8080/api/users

# Check health
curl http://localhost:8080/health

# View metrics
curl http://localhost:8080/metrics
    </pre>
</body>
</html>`); err != nil {
			log.Printf("Warning: failed to write response: %v", err)
		}
	})

	return mux
}

func main() {
	fmt.Println("🚀 Lift Production API")
	fmt.Println("======================")

	// Create production API with all integrations
	api := NewProductionAPI()

	// Setup routes
	mux := api.setupRoutes()

	// Run initial health check
	fmt.Println("\n🔍 Running Initial Health Checks...")
	ctx := context.Background()

	checkers := api.healthManager.ListCheckers()
	fmt.Printf("Registered health checkers: %v\n", checkers)

	for _, name := range checkers {
		status, err := api.healthManager.CheckComponent(ctx, name)
		if err != nil {
			fmt.Printf("❌ %s: ERROR - %v\n", name, err)
			continue
		}

		var emoji string
		switch status.Status {
		case health.StatusDegraded:
			emoji = "⚠️"
		case health.StatusUnhealthy:
			emoji = "❌"
		default:
			emoji = "✅"
		}

		fmt.Printf("%s %s: %s (%v) - %s\n",
			emoji, name, status.Status, status.Duration, status.Message)
	}

	// Check overall health
	fmt.Println("\n📋 Overall Health Status:")
	overall := api.healthManager.OverallHealth(ctx)
	var overallEmoji string
	switch overall.Status {
	case health.StatusDegraded:
		overallEmoji = "⚠️"
	case health.StatusUnhealthy:
		overallEmoji = "❌"
	default:
		overallEmoji = "✅"
	}

	fmt.Printf("%s Overall: %s (%v) - %s\n",
		overallEmoji, overall.Status, overall.Duration, overall.Message)

	// Show resource pool status
	fmt.Println("\n🔗 Resource Pool Status:")
	allStats := api.resourceManager.Stats()
	poolStats := allStats["database"]
	fmt.Printf("Database Pool: %d active, %d idle, %d total connections\n",
		poolStats.Active, poolStats.Idle, poolStats.Total)

	// Start server
	port := ":8080"
	fmt.Printf("\n🌐 Starting Production API on http://localhost%s\n", port)
	fmt.Println("\nAPI Endpoints:")
	fmt.Printf("  • http://localhost%s/           - API documentation\n", port)
	fmt.Printf("  • http://localhost%s/health     - Health status\n", port)
	fmt.Printf("  • http://localhost%s/api/users  - User management\n", port)
	fmt.Printf("  • http://localhost%s/metrics    - Performance metrics\n", port)
	fmt.Printf("  • http://localhost%s/status     - Service status\n", port)

	fmt.Println("\n💡 Features Demonstrated:")
	fmt.Println("  • Resource management with connection pooling")
	fmt.Println("  • Health monitoring with multiple checkers")
	fmt.Println("  • Performance optimization with minimal overhead")
	fmt.Println("  • Type-safe request/response handling")
	fmt.Println("  • Production-ready observability")

	server := &http.Server{
		Addr:              port,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB
		Handler:           mux,
	}

	log.Fatal(server.ListenAndServe())
}
