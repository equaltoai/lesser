package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/validation"
)

// Domain Models

// Tenant represents a tenant in the multi-tenant system
type Tenant struct {
	CreatedAt  time.Time `json:"created_at" `
	UpdatedAt  time.Time `json:"updated_at" `
	ID         string    `json:"id" `
	Name       string    `json:"name" `
	Email      string    `json:"email" `
	Plan       string    `json:"plan" `
	Status     string    `json:"status" `
	RateLimit  int       `json:"rate_limit" `
	BurstLimit int       `json:"burst_limit" `
}

// User represents a user within a tenant
type User struct {
	CreatedAt time.Time `json:"created_at" `
	UpdatedAt time.Time `json:"updated_at" `
	ID        string    `json:"id" `
	TenantID  string    `json:"tenant_id" `
	Email     string    `json:"email" `
	Name      string    `json:"name" `
	Role      string    `json:"role" `
	Status    string    `json:"status" `
}

// Project represents a project within a tenant
type Project struct {
	CreatedAt   time.Time `json:"created_at" `
	UpdatedAt   time.Time `json:"updated_at" `
	ID          string    `json:"id" `
	TenantID    string    `json:"tenant_id" `
	Name        string    `json:"name" `
	Description string    `json:"description" `
	Status      string    `json:"status" `
	OwnerID     string    `json:"owner_id" `
}

// Task represents a task within a project
type Task struct {
	CreatedAt   time.Time  `json:"created_at" `
	UpdatedAt   time.Time  `json:"updated_at" `
	DueDate     *time.Time `json:"due_date,omitempty" `
	ID          string     `json:"id" `
	TenantID    string     `json:"tenant_id" `
	ProjectID   string     `json:"project_id" `
	Title       string     `json:"title" `
	Description string     `json:"description" `
	Status      string     `json:"status" `
	Priority    string     `json:"priority" `
	AssigneeID  string     `json:"assignee_id" `
}

// Request/Response DTOs

// CreateTenantRequest represents a request to create a tenant
type CreateTenantRequest struct {
	Name  string `json:"name" validate:"required,min=2,max=100"`
	Email string `json:"email" validate:"required,email"`
	Plan  string `json:"plan" validate:"required,oneof=free pro enterprise"`
}

// CreateUserRequest represents a request to create a user
type CreateUserRequest struct {
	Email string `json:"email" validate:"required,email"`
	Name  string `json:"name" validate:"required,min=2,max=100"`
	Role  string `json:"role" validate:"required,oneof=admin user viewer"`
}

// CreateProjectRequest represents a request to create a project
type CreateProjectRequest struct {
	Name        string `json:"name" validate:"required,min=2,max=100"`
	Description string `json:"description" validate:"max=500"`
}

// CreateTaskRequest represents a request to create a task
type CreateTaskRequest struct {
	DueDate     *time.Time `json:"due_date,omitempty"`
	ProjectID   string     `json:"project_id" validate:"required"`
	Title       string     `json:"title" validate:"required,min=2,max=200"`
	Description string     `json:"description" validate:"max=1000"`
	Priority    string     `json:"priority" validate:"required,oneof=low medium high critical"`
	AssigneeID  string     `json:"assignee_id,omitempty"`
}

// UpdateTaskRequest represents a request to update a task
type UpdateTaskRequest struct {
	Title       *string    `json:"title,omitempty" validate:"omitempty,min=2,max=200"`
	Description *string    `json:"description,omitempty" validate:"omitempty,max=1000"`
	Status      *string    `json:"status,omitempty" validate:"omitempty,oneof=todo in_progress done"`
	Priority    *string    `json:"priority,omitempty" validate:"omitempty,oneof=low medium high critical"`
	AssigneeID  *string    `json:"assignee_id,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"`
}

// PaginatedResponse represents a paginated response
type PaginatedResponse struct {
	Data       any        `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// Pagination represents pagination metadata
type Pagination struct {
	NextPage   *int  `json:"next_page,omitempty"`
	PrevPage   *int  `json:"prev_page,omitempty"`
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// Mock database interface for demonstration
type MockDB interface {
	// Put stores an item in the database.
	// Parameters:
	//   - ctx: The context for the request
	//   - item: The item to store
	// Returns:
	//   - An error if the storage fails
	Put(ctx context.Context, item any) error

	// Get retrieves an item from the database by its ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - id: The ID of the item
	//   - item: The item to retrieve
	// Returns:
	//   - An error if the retrieval fails
	Get(ctx context.Context, id string, item any) error
}

type mockDB struct {
	data map[string]any
}

func newMockDB() MockDB {
	return &mockDB{
		data: make(map[string]any),
	}
}

func (db *mockDB) Put(_ context.Context, _ any) error {
	// Mock implementation
	return nil
}

func (db *mockDB) Get(_ context.Context, _ string, _ any) error {
	// Mock implementation
	return nil
}

// Services

// TenantService handles tenant operations
type TenantService struct {
	db MockDB
}

func NewTenantService(db MockDB) *TenantService {
	return &TenantService{db: db}
}

func (s *TenantService) CreateTenant(ctx context.Context, req CreateTenantRequest) (*Tenant, error) {
	tenant := &Tenant{
		ID:         generateID(),
		Name:       req.Name,
		Email:      req.Email,
		Plan:       req.Plan,
		Status:     "active",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		RateLimit:  getRateLimitForPlan(req.Plan),
		BurstLimit: getBurstLimitForPlan(req.Plan),
	}

	if err := s.db.Put(ctx, tenant); err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	return tenant, nil
}

func (s *TenantService) GetTenant(ctx context.Context, id string) (*Tenant, error) {
	tenant := &Tenant{}
	if err := s.db.Get(ctx, id, tenant); err != nil {
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	return tenant, nil
}

// UserService handles user operations
type UserService struct {
	db MockDB
}

func NewUserService(db MockDB) *UserService {
	return &UserService{db: db}
}

func (s *UserService) CreateUser(ctx context.Context, tenantID string, req CreateUserRequest) (*User, error) {
	user := &User{
		ID:        generateID(),
		TenantID:  tenantID,
		Email:     req.Email,
		Name:      req.Name,
		Role:      req.Role,
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.db.Put(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

func (s *UserService) GetUsersByTenant(_ context.Context, tenantID string, _, _ int) ([]*User, int64, error) {
	// This would use DynamORM's query capabilities
	// For now, return mock data
	users := []*User{
		{
			ID:       "user-1",
			TenantID: tenantID,
			Email:    "user1@example.com",
			Name:     "User One",
			Role:     "admin",
			Status:   "active",
		},
	}

	return users, 1, nil
}

// ProjectService handles project operations
type ProjectService struct {
	db MockDB
}

func NewProjectService(db MockDB) *ProjectService {
	return &ProjectService{db: db}
}

func (s *ProjectService) CreateProject(ctx context.Context, tenantID, userID string, req CreateProjectRequest) (*Project, error) {
	project := &Project{
		ID:          generateID(),
		TenantID:    tenantID,
		Name:        req.Name,
		Description: req.Description,
		Status:      "active",
		OwnerID:     userID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.db.Put(ctx, project); err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	return project, nil
}

func (s *ProjectService) GetProjectsByTenant(_ context.Context, tenantID string, _, _ int) ([]*Project, int64, error) {
	// This would use DynamORM's query capabilities
	// For now, return mock data
	projects := []*Project{
		{
			ID:          "project-1",
			TenantID:    tenantID,
			Name:        "Sample Project",
			Description: "A sample project",
			Status:      "active",
			OwnerID:     "user-1",
		},
	}

	return projects, 1, nil
}

// TaskService handles task operations
type TaskService struct {
	db MockDB
}

func NewTaskService(db MockDB) *TaskService {
	return &TaskService{db: db}
}

func (s *TaskService) CreateTask(ctx context.Context, tenantID, _ string, req CreateTaskRequest) (*Task, error) {
	task := &Task{
		ID:          generateID(),
		TenantID:    tenantID,
		ProjectID:   req.ProjectID,
		Title:       req.Title,
		Description: req.Description,
		Status:      "todo",
		Priority:    req.Priority,
		AssigneeID:  req.AssigneeID,
		DueDate:     req.DueDate,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.db.Put(ctx, task); err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	return task, nil
}

func (s *TaskService) UpdateTask(ctx context.Context, tenantID, taskID string, req UpdateTaskRequest) (*Task, error) {
	task := &Task{}
	if err := s.db.Get(ctx, taskID, task); err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	// Verify tenant isolation
	if task.TenantID != tenantID {
		return nil, fmt.Errorf("task not found")
	}

	// Update fields
	if req.Title != nil {
		task.Title = *req.Title
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.Status != nil {
		task.Status = *req.Status
	}
	if req.Priority != nil {
		task.Priority = *req.Priority
	}
	if req.AssigneeID != nil {
		task.AssigneeID = *req.AssigneeID
	}
	if req.DueDate != nil {
		task.DueDate = req.DueDate
	}

	task.UpdatedAt = time.Now()

	if err := s.db.Put(ctx, task); err != nil {
		return nil, fmt.Errorf("failed to update task: %w", err)
	}

	return task, nil
}

func (s *TaskService) GetTasksByProject(_ context.Context, tenantID, projectID string, _, _ int) ([]*Task, int64, error) {
	// This would use DynamORM's query capabilities
	// For now, return mock data
	tasks := []*Task{
		{
			ID:          "task-1",
			TenantID:    tenantID,
			ProjectID:   projectID,
			Title:       "Sample Task",
			Description: "A sample task",
			Status:      "todo",
			Priority:    "medium",
		},
	}

	return tasks, 1, nil
}

// Handler helpers

// Generic list handler to reduce duplication
func handleListRequest[T any](
	ctx *lift.Context,
	listFunc func(context.Context, string, int, int) ([]T, int64, error),
	resourceName string,
) error {
	tenantID := ctx.TenantID()
	if tenantID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Tenant ID is required", 400)
	}

	page, err := strconv.Atoi(ctx.Query("page"))
	if err != nil || page < 1 {
		page = 1
	}

	perPage, err := strconv.Atoi(ctx.Query("per_page"))
	if err != nil || perPage < 1 || perPage > 100 {
		perPage = 10
	}

	resources, total, err := listFunc(ctx.Context, tenantID, page, perPage)
	if err != nil {
		if logger := ctx.Logger; logger != nil {
			logger.WithField("error", err.Error()).Error(fmt.Sprintf("Failed to list %s", resourceName))
		}
		return lift.NewLiftError("INTERNAL_ERROR", fmt.Sprintf("Failed to list %s", resourceName), 500)
	}

	totalPages := int((total + int64(perPage) - 1) / int64(perPage))

	pagination := Pagination{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: totalPages,
	}

	if page < totalPages {
		nextPage := page + 1
		pagination.NextPage = &nextPage
	}

	if page > 1 {
		prevPage := page - 1
		pagination.PrevPage = &prevPage
	}

	response := PaginatedResponse{
		Data:       resources,
		Pagination: pagination,
	}

	return ctx.JSON(response)
}

// Generic create handler to reduce duplication
func handleCreateRequest[TReq any, TResp any](
	ctx *lift.Context,
	createFunc func(context.Context, string, string, TReq) (TResp, error),
	resourceName string,
) error {
	tenantID := ctx.TenantID()
	userID := ctx.UserID()

	if tenantID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Tenant ID is required", 400)
	}
	if userID == "" {
		return lift.NewLiftError("BAD_REQUEST", "User ID is required", 400)
	}

	var req TReq
	if err := ctx.ParseRequest(&req); err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid request body", 400)
	}

	if err := validation.Validate(req); err != nil {
		return lift.ValidationError(err.Error()).WithDetail("field", "validation")
	}

	resource, err := createFunc(ctx.Context, tenantID, userID, req)
	if err != nil {
		if logger := ctx.Logger; logger != nil {
			logger.WithField("error", err.Error()).Error(fmt.Sprintf("Failed to create %s", resourceName))
		}
		return lift.NewLiftError("INTERNAL_ERROR", fmt.Sprintf("Failed to create %s", resourceName), 500)
	}

	return ctx.Status(201).JSON(resource)
}

// Handlers

// TenantHandlers contains handlers for tenant operations
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
		return lift.ValidationError(err.Error()).WithDetail("field", "validation")
	}

	tenant, err := h.service.CreateTenant(ctx.Context, req)
	if err != nil {
		if logger := ctx.Logger; logger != nil {
			logger.WithField("error", err.Error()).Error("Failed to create tenant")
		}
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to create tenant", 500)
	}

	return ctx.Status(201).JSON(tenant)
}

func (h *TenantHandlers) GetTenant(ctx *lift.Context) error {
	tenantID := ctx.Param("id")
	if tenantID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Tenant ID is required", 400)
	}

	tenant, err := h.service.GetTenant(ctx.Context, tenantID)
	if err != nil {
		if logger := ctx.Logger; logger != nil {
			logger.WithField("error", err.Error()).Error("Failed to get tenant")
		}
		return lift.NotFound("Tenant not found")
	}

	return ctx.JSON(tenant)
}

// UserHandlers contains handlers for user operations
type UserHandlers struct {
	service *UserService
}

func NewUserHandlers(service *UserService) *UserHandlers {
	return &UserHandlers{service: service}
}

func (h *UserHandlers) CreateUser(ctx *lift.Context) error {
	tenantID := ctx.TenantID()
	if tenantID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Tenant ID is required", 400)
	}

	var req CreateUserRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid request body", 400)
	}

	if err := validation.Validate(req); err != nil {
		return lift.ValidationError(err.Error()).WithDetail("field", "validation")
	}

	user, err := h.service.CreateUser(ctx.Context, tenantID, req)
	if err != nil {
		if logger := ctx.Logger; logger != nil {
			logger.WithField("error", err.Error()).Error("Failed to create user")
		}
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to create user", 500)
	}

	return ctx.Status(201).JSON(user)
}

func (h *UserHandlers) ListUsers(ctx *lift.Context) error {
	return handleListRequest(ctx, h.service.GetUsersByTenant, "users")
}

// ProjectHandlers contains handlers for project operations
type ProjectHandlers struct {
	service *ProjectService
}

func NewProjectHandlers(service *ProjectService) *ProjectHandlers {
	return &ProjectHandlers{service: service}
}

func (h *ProjectHandlers) CreateProject(ctx *lift.Context) error {
	return handleCreateRequest(ctx, h.service.CreateProject, "project")
}

func (h *ProjectHandlers) ListProjects(ctx *lift.Context) error {
	return handleListRequest(ctx, h.service.GetProjectsByTenant, "projects")
}

// TaskHandlers contains handlers for task operations
type TaskHandlers struct {
	service *TaskService
}

func NewTaskHandlers(service *TaskService) *TaskHandlers {
	return &TaskHandlers{service: service}
}

func (h *TaskHandlers) CreateTask(ctx *lift.Context) error {
	return handleCreateRequest(ctx, h.service.CreateTask, "task")
}

func (h *TaskHandlers) UpdateTask(ctx *lift.Context) error {
	tenantID := ctx.TenantID()
	taskID := ctx.Param("id")

	if tenantID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Tenant ID is required", 400)
	}
	if taskID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Task ID is required", 400)
	}

	var req UpdateTaskRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid request body", 400)
	}

	if err := validation.Validate(req); err != nil {
		return lift.ValidationError(err.Error()).WithDetail("field", "validation")
	}

	task, err := h.service.UpdateTask(ctx.Context, tenantID, taskID, req)
	if err != nil {
		if logger := ctx.Logger; logger != nil {
			logger.WithField("error", err.Error()).Error("Failed to update task")
		}
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to update task", 500)
	}

	return ctx.JSON(task)
}

func (h *TaskHandlers) ListTasks(ctx *lift.Context) error {
	tenantID := ctx.TenantID()
	projectID := ctx.Query("project_id")

	if tenantID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Tenant ID is required", 400)
	}
	if projectID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Project ID is required", 400)
	}

	page, err := strconv.Atoi(ctx.Query("page"))
	if err != nil || page < 1 {
		page = 1
	}

	perPage, err := strconv.Atoi(ctx.Query("per_page"))
	if err != nil || perPage < 1 || perPage > 100 {
		perPage = 10
	}

	tasks, total, err := h.service.GetTasksByProject(ctx.Context, tenantID, projectID, page, perPage)
	if err != nil {
		if logger := ctx.Logger; logger != nil {
			logger.WithField("error", err.Error()).Error("Failed to list tasks")
		}
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to list tasks", 500)
	}

	totalPages := int((total + int64(perPage) - 1) / int64(perPage))

	pagination := Pagination{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: totalPages,
	}

	if page < totalPages {
		nextPage := page + 1
		pagination.NextPage = &nextPage
	}

	if page > 1 {
		prevPage := page - 1
		pagination.PrevPage = &prevPage
	}

	response := PaginatedResponse{
		Data:       tasks,
		Pagination: pagination,
	}

	return ctx.JSON(response)
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

func getBurstLimitForPlan(plan string) int {
	switch plan {
	case "free":
		return 10
	case "pro":
		return 50
	case "enterprise":
		return 200
	default:
		return 10
	}
}

// Main application setup

func main() {
	// Initialize mock database
	db := newMockDB()

	// Initialize services
	tenantService := NewTenantService(db)
	userService := NewUserService(db)
	projectService := NewProjectService(db)
	taskService := NewTaskService(db)

	// Initialize handlers
	tenantHandlers := NewTenantHandlers(tenantService)
	userHandlers := NewUserHandlers(userService)
	projectHandlers := NewProjectHandlers(projectService)
	taskHandlers := NewTaskHandlers(taskService)

	// Create Lift app
	app := lift.New()

	// Public routes (no authentication required)
	if err := app.POST("/api/tenants", tenantHandlers.CreateTenant); err != nil {
		log.Fatalf("Failed to register POST /api/tenants: %v", err)
	}
	if err := app.GET("/api/health", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{
			"status":    "healthy",
			"timestamp": time.Now().Format(time.RFC3339),
			"version":   "1.0.0",
		})
	}); err != nil {
		log.Fatalf("Failed to register GET /api/health: %v", err)
	}

	// Protected routes (simplified for demo)
	if err := app.GET("/api/tenants/:id", tenantHandlers.GetTenant); err != nil {
		log.Fatalf("Failed to register GET /api/tenants/:id: %v", err)
	}
	if err := app.POST("/api/users", userHandlers.CreateUser); err != nil {
		log.Fatalf("Failed to register POST /api/users: %v", err)
	}
	if err := app.GET("/api/users", userHandlers.ListUsers); err != nil {
		log.Fatalf("Failed to register GET /api/users: %v", err)
	}
	if err := app.POST("/api/projects", projectHandlers.CreateProject); err != nil {
		log.Fatalf("Failed to register POST /api/projects: %v", err)
	}
	if err := app.GET("/api/projects", projectHandlers.ListProjects); err != nil {
		log.Fatalf("Failed to register GET /api/projects: %v", err)
	}
	if err := app.POST("/api/tasks", taskHandlers.CreateTask); err != nil {
		log.Fatalf("Failed to register POST /api/tasks: %v", err)
	}
	if err := app.PUT("/api/tasks/:id", taskHandlers.UpdateTask); err != nil {
		log.Fatalf("Failed to register PUT /api/tasks/:id: %v", err)
	}
	if err := app.GET("/api/tasks", taskHandlers.ListTasks); err != nil {
		log.Fatalf("Failed to register GET /api/tasks: %v", err)
	}

	// Metrics and monitoring endpoints
	if err := app.GET("/metrics", func(ctx *lift.Context) error {
		// Return application metrics
		metrics := map[string]any{
			"uptime":           time.Since(time.Now()).String(),
			"memory_usage":     "N/A", // Would implement actual memory tracking
			"active_tenants":   "N/A", // Would implement actual tenant counting
			"requests_per_min": "N/A", // Would implement actual request tracking
		}
		return ctx.JSON(metrics)
	}); err != nil {
		log.Fatalf("Failed to register GET /metrics: %v", err)
	}

	// Start the Lambda handler
	lambda.Start(app.HandleRequest)
}
