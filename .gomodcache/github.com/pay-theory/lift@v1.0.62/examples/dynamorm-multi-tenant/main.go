package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/middleware"
	"github.com/pay-theory/lift/pkg/validation"
)

// Tenant represents a tenant in the multi-tenant DynamORM system
// Tenant represents a tenant in a multi-tenant system.
// It includes metadata such as creation and update timestamps, primary and sort keys,
// tenant ID, entity type, and other relevant information.
type Tenant struct {
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	PK         string    `dynamorm:"pk" json:"pk"`
	SK         string    `dynamorm:"sk" json:"sk"`
	TenantID   string    `dynamorm:"index:tenant-entity,pk" json:"tenant_id"`
	EntityType string    `dynamorm:"index:tenant-entity,sk" json:"entity_type"`
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Email      string    `json:"email"`
	Plan       string    `json:"plan"`
	Status     string    `json:"status"`
	RateLimit  int       `json:"rate_limit"`
}

// User represents a user within a tenant using DynamORM patterns
// User represents a user in a multi-tenant system.
// It includes metadata such as creation and update timestamps, primary and sort keys,
// tenant ID, entity type, and other relevant information.
type User struct {
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	PK         string    `dynamorm:"pk" json:"pk"`
	SK         string    `dynamorm:"sk" json:"sk"`
	TenantID   string    `dynamorm:"index:tenant-entity,pk" json:"tenant_id"`
	EntityType string    `dynamorm:"index:tenant-entity,sk" json:"entity_type"`
	ID         string    `json:"id"`
	Email      string    `json:"email"`
	Name       string    `json:"name"`
	Role       string    `json:"role"`
	Status     string    `json:"status"`
}

// Project represents a project within a tenant using DynamORM patterns
// Project represents a project in a multi-tenant system.
// It includes metadata such as creation and update timestamps, primary and sort keys,
// tenant ID, entity type, and other relevant information.
type Project struct {
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	PK          string    `dynamorm:"pk" json:"pk"`
	SK          string    `dynamorm:"sk" json:"sk"`
	TenantID    string    `dynamorm:"index:tenant-entity,pk" json:"tenant_id"`
	EntityType  string    `dynamorm:"index:tenant-entity,sk" json:"entity_type"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	OwnerID     string    `json:"owner_id"`
}

// Request DTOs
// CreateTenantRequest represents a request to create a new tenant.
// It includes fields for the tenant's name, email, and plan.
type CreateTenantRequest struct {
	Name  string `json:"name" validate:"required,min=2,max=100"`
	Email string `json:"email" validate:"required,email"`
	Plan  string `json:"plan" validate:"required,oneof=free pro enterprise"`
}

// CreateUserRequest represents a request to create a new user.
// It includes fields for the user's email, name, and role.
type CreateUserRequest struct {
	Email string `json:"email" validate:"required,email"`
	Name  string `json:"name" validate:"required,min=2,max=100"`
	Role  string `json:"role" validate:"required,oneof=admin user viewer"`
}

// CreateProjectRequest represents a request to create a new project.
// It includes fields for the project's name and description.
type CreateProjectRequest struct {
	Name        string `json:"name" validate:"required,min=2,max=100"`
	Description string `json:"description" validate:"max=500"`
}

// Mock DynamORM service to demonstrate patterns
// DynamORMService provides a service for interacting with DynamoDB.
// It includes methods for creating, reading, updating, and deleting items in DynamoDB.
type DynamORMService struct {
	tableName string
}

func NewDynamORMService(tableName string) *DynamORMService {
	return &DynamORMService{tableName: tableName}
}

func (s *DynamORMService) PutItem(_ context.Context, _ interface{}) error {
	// In real implementation, this would use DynamORM client
	fmt.Printf("DynamORM: Putting item to table %s\n", s.tableName)
	return nil
}

func (s *DynamORMService) GetItem(_ context.Context, pk, sk string, _ interface{}) error {
	// In real implementation, this would use DynamORM client
	fmt.Printf("DynamORM: Getting item from table %s with PK=%s, SK=%s\n", s.tableName, pk, sk)
	return nil
}

func (s *DynamORMService) QueryByTenant(_ context.Context, tenantID string, entityType string, _ interface{}) error {
	// In real implementation, this would query the tenant GSI
	fmt.Printf("DynamORM: Querying tenant %s for entity type %s\n", tenantID, entityType)
	return nil
}

// Services implementing DynamORM patterns
// TenantService provides a service for managing tenants.
// It includes methods for creating, reading, updating, and deleting tenants.
type TenantService struct {
	db *DynamORMService
}

func NewTenantService(db *DynamORMService) *TenantService {
	return &TenantService{db: db}
}

func (s *TenantService) CreateTenant(ctx context.Context, req CreateTenantRequest) (*Tenant, error) {
	id := generateID()
	tenant := &Tenant{
		PK:         fmt.Sprintf("tenant#%s", id),
		SK:         fmt.Sprintf("tenant#%s", id),
		TenantID:   id,
		EntityType: "tenant",
		ID:         id,
		Name:       req.Name,
		Email:      req.Email,
		Plan:       req.Plan,
		Status:     "active",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		RateLimit:  getRateLimitForPlan(req.Plan),
	}

	return tenant, s.db.PutItem(ctx, tenant)
}

func (s *TenantService) GetTenant(ctx context.Context, id string) (*Tenant, error) {
	tenant := &Tenant{}
	pk := fmt.Sprintf("tenant#%s", id)
	sk := fmt.Sprintf("tenant#%s", id)

	if err := s.db.GetItem(ctx, pk, sk, tenant); err != nil {
		return nil, fmt.Errorf("tenant not found: %w", err)
	}

	return tenant, nil
}

// UserService provides a service for managing users.
// It includes methods for creating, reading, updating, and deleting users.
type UserService struct {
	db *DynamORMService
}

func NewUserService(db *DynamORMService) *UserService {
	return &UserService{db: db}
}

func (s *UserService) CreateUser(ctx context.Context, tenantID string, req CreateUserRequest) (*User, error) {
	id := generateID()
	user := &User{
		PK:         fmt.Sprintf("tenant#%s", tenantID),
		SK:         fmt.Sprintf("user#%s", id),
		TenantID:   tenantID,
		EntityType: "user",
		ID:         id,
		Email:      req.Email,
		Name:       req.Name,
		Role:       req.Role,
		Status:     "active",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	return user, s.db.PutItem(ctx, user)
}

func (s *UserService) GetUsersByTenant(ctx context.Context, tenantID string) ([]*User, error) {
	var users []*User
	err := s.db.QueryByTenant(ctx, tenantID, "user", &users)
	return users, err
}

// ProjectService provides a service for managing projects.
// It includes methods for creating, reading, updating, and deleting projects.
type ProjectService struct {
	db *DynamORMService
}

func NewProjectService(db *DynamORMService) *ProjectService {
	return &ProjectService{db: db}
}

func (s *ProjectService) CreateProject(ctx context.Context, tenantID, ownerID string, req CreateProjectRequest) (*Project, error) {
	id := generateID()
	project := &Project{
		PK:          fmt.Sprintf("tenant#%s", tenantID),
		SK:          fmt.Sprintf("project#%s", id),
		TenantID:    tenantID,
		EntityType:  "project",
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Status:      "active",
		OwnerID:     ownerID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	return project, s.db.PutItem(ctx, project)
}

func (s *ProjectService) GetProjectsByTenant(ctx context.Context, tenantID string) ([]*Project, error) {
	var projects []*Project
	err := s.db.QueryByTenant(ctx, tenantID, "project", &projects)
	return projects, err
}

// Handlers with tenant isolation
// TenantHandlers provides HTTP handlers for managing tenants.
// It includes methods for handling HTTP requests related to tenants.
type TenantHandlers struct {
	service *TenantService
}

func NewTenantHandlers(service *TenantService) *TenantHandlers {
	return &TenantHandlers{service: service}
}

func (h *TenantHandlers) CreateTenant(ctx *lift.Context) error {
	var req CreateTenantRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid request body", 400)
	}

	if err := validation.Validate(req); err != nil {
		return lift.ValidationError(err.Error())
	}

	tenant, err := h.service.CreateTenant(ctx.Context, req)
	if err != nil {
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to create tenant", 500)
	}

	// Set tenant ID in response headers for client tracking
	ctx.Response.Header("X-Tenant-ID", tenant.ID)
	return ctx.Status(201).JSON(tenant)
}

func (h *TenantHandlers) GetTenant(ctx *lift.Context) error {
	tenantID := ctx.Param("id")
	if tenantID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Tenant ID is required", 400)
	}

	tenant, err := h.service.GetTenant(ctx.Context, tenantID)
	if err != nil {
		return lift.NotFound("Tenant not found")
	}

	return ctx.JSON(tenant)
}

// UserHandlers provides HTTP handlers for managing users.
// It includes methods for handling HTTP requests related to users.
type UserHandlers struct {
	service *UserService
}

func NewUserHandlers(service *UserService) *UserHandlers {
	return &UserHandlers{service: service}
}

func (h *UserHandlers) CreateUser(ctx *lift.Context) error {
	// Extract tenant ID from context (would be set by middleware)
	tenantID := ctx.TenantID()
	if tenantID == "" {
		return lift.NewLiftError("UNAUTHORIZED", "Tenant context required", 401)
	}

	var req CreateUserRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid request body", 400)
	}

	if err := validation.Validate(req); err != nil {
		return lift.ValidationError(err.Error())
	}

	user, err := h.service.CreateUser(ctx.Context, tenantID, req)
	if err != nil {
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to create user", 500)
	}

	return ctx.Status(201).JSON(user)
}

func (h *UserHandlers) ListUsers(ctx *lift.Context) error {
	tenantID := ctx.TenantID()
	if tenantID == "" {
		return lift.NewLiftError("UNAUTHORIZED", "Tenant context required", 401)
	}

	users, err := h.service.GetUsersByTenant(ctx.Context, tenantID)
	if err != nil {
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to list users", 500)
	}

	return ctx.JSON(map[string]interface{}{
		"users":     users,
		"tenant_id": tenantID,
		"count":     len(users),
	})
}

// ProjectHandlers provides HTTP handlers for managing projects.
// It includes methods for handling HTTP requests related to projects.
type ProjectHandlers struct {
	service *ProjectService
}

func NewProjectHandlers(service *ProjectService) *ProjectHandlers {
	return &ProjectHandlers{service: service}
}

func (h *ProjectHandlers) CreateProject(ctx *lift.Context) error {
	tenantID := ctx.TenantID()
	userID := ctx.UserID()

	if tenantID == "" || userID == "" {
		return lift.NewLiftError("UNAUTHORIZED", "Tenant and user context required", 401)
	}

	var req CreateProjectRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid request body", 400)
	}

	if err := validation.Validate(req); err != nil {
		return lift.ValidationError(err.Error())
	}

	project, err := h.service.CreateProject(ctx.Context, tenantID, userID, req)
	if err != nil {
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to create project", 500)
	}

	return ctx.Status(201).JSON(project)
}

func (h *ProjectHandlers) ListProjects(ctx *lift.Context) error {
	tenantID := ctx.TenantID()
	if tenantID == "" {
		return lift.NewLiftError("UNAUTHORIZED", "Tenant context required", 401)
	}

	projects, err := h.service.GetProjectsByTenant(ctx.Context, tenantID)
	if err != nil {
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to list projects", 500)
	}

	return ctx.JSON(map[string]interface{}{
		"projects":  projects,
		"tenant_id": tenantID,
		"count":     len(projects),
	})
}

// Tenant isolation middleware
func TenantIsolationMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Extract tenant ID from header or JWT token
			tenantID := ctx.Header("X-Tenant-ID")
			if tenantID == "" {
				// In a real app, you'd extract this from a JWT token
				tenantID = "demo-tenant"
			}

			// Set tenant ID in context
			ctx.Set("tenant_id", tenantID)

			// Log tenant access for monitoring
			if logger := ctx.Logger; logger != nil {
				logger.WithField("tenant_id", tenantID).Info("Tenant access")
			}

			return next.Handle(ctx)
		})
	}
}

// Utility functions
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func getRateLimitForPlan(plan string) int {
	switch plan {
	case "free":
		return 100
	case "pro":
		return 1000
	case "enterprise":
		return 10000
	default:
		return 100
	}
}

// Main application demonstrating DynamORM multi-tenant patterns
func main() {
	// Initialize DynamORM service (in real app, this would be properly configured)
	tableName := "DynamORMMultiTenantTable"
	db := NewDynamORMService(tableName)

	// Initialize services
	tenantService := NewTenantService(db)
	userService := NewUserService(db)
	projectService := NewProjectService(db)

	// Initialize handlers
	tenantHandlers := NewTenantHandlers(tenantService)
	userHandlers := NewUserHandlers(userService)
	projectHandlers := NewProjectHandlers(projectService)

	// Create Lift app
	app := lift.New()

	// Add middleware
	app.Use(lift.Middleware(middleware.Logger()))
	app.Use(lift.Middleware(middleware.Recover()))
	app.Use(lift.Middleware(middleware.CORS([]string{"*"})))

	// Add tenant-specific rate limiting
	rateLimiter, err := middleware.TenantRateLimitWithLimited(1000, time.Hour)
	if err == nil {
		app.Use(rateLimiter)
	}

	// Public routes
	if err := app.POST("/api/tenants", tenantHandlers.CreateTenant); err != nil {
		log.Fatalf("Failed to register POST /api/tenants: %v", err)
	}
	if err := app.GET("/api/health", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]interface{}{
			"status":     "healthy",
			"timestamp":  time.Now().Format(time.RFC3339),
			"version":    "1.0.0",
			"table_name": tableName,
			"features": []string{
				"multi-tenant",
				"dynamorm",
				"tenant-isolation",
				"rate-limiting",
				"monitoring",
			},
		})
	}); err != nil {
		log.Fatalf("Failed to register GET /api/health: %v", err)
	}

	// Add tenant isolation middleware
	app.Use(TenantIsolationMiddleware())

	// Tenant-scoped routes
	tenantGroup := app.Group("/api")

	// Tenant management
	if err := tenantGroup.GET("/tenants/:id", tenantHandlers.GetTenant); err != nil {
		log.Fatalf("Failed to register GET /api/tenants/:id: %v", err)
	}

	// User management (tenant-scoped)
	if err := tenantGroup.POST("/users", userHandlers.CreateUser); err != nil {
		log.Fatalf("Failed to register POST /api/users: %v", err)
	}
	if err := tenantGroup.GET("/users", userHandlers.ListUsers); err != nil {
		log.Fatalf("Failed to register GET /api/users: %v", err)
	}

	// Project management (tenant-scoped)
	if err := tenantGroup.POST("/projects", projectHandlers.CreateProject); err != nil {
		log.Fatalf("Failed to register POST /api/projects: %v", err)
	}
	if err := tenantGroup.GET("/projects", projectHandlers.ListProjects); err != nil {
		log.Fatalf("Failed to register GET /api/projects: %v", err)
	}

	// Metrics endpoint showing tenant-specific data
	if err := tenantGroup.GET("/metrics", func(ctx *lift.Context) error {
		tenantID := ctx.TenantID()
		return ctx.JSON(map[string]interface{}{
			"tenant_id":  tenantID,
			"table_name": tableName,
			"access_patterns": []string{
				fmt.Sprintf("PK: tenant#%s", tenantID),
				"SK: user#{id}, project#{id}",
				"GSI1: tenant_id, entity_type",
				"GSI2: status, tenant_id",
			},
			"isolation_features": []string{
				"IAM policies with tenant boundary enforcement",
				"DynamORM tenant-scoped GSIs",
				"CloudWatch metrics with tenant dimensions",
				"X-Ray tracing with tenant context",
			},
		})
	}); err != nil {
		log.Fatalf("Failed to register GET /api/metrics: %v", err)
	}

	// Admin endpoints (would require admin authentication in real app)
	if err := app.GET("/admin/tenants", func(ctx *lift.Context) error {
		// This would query all tenants (admin only)
		return ctx.JSON(map[string]interface{}{
			"message": "Admin endpoint - would list all tenants",
			"note":    "Requires admin authentication in production",
		})
	}); err != nil {
		log.Fatalf("Failed to register GET /admin/tenants: %v", err)
	}

	// Start the Lambda handler
	lambda.Start(app.HandleRequest)
}
