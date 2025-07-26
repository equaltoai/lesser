package testing

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// IntegrationTestSuite provides a framework for integration testing
type IntegrationTestSuite struct {
	Storage  *MockStorage
	TestData *TestDataManager
	Cleanup  []func()
	Config   *IntegrationConfig
	t        *testing.T
}

// IntegrationConfig holds configuration for integration tests
type IntegrationConfig struct {
	// DynamoDB Local configuration
	DynamoDBEndpoint string
	DynamoDBRegion   string
	TableName        string

	// Test timeouts
	DefaultTimeout time.Duration

	// Environment setup
	SkipTableCreation bool
	PreserveData      bool

	// Performance thresholds
	MaxResponseTime time.Duration
	MaxMemoryUsage  int64
}

// DefaultIntegrationConfig returns default integration test configuration
func DefaultIntegrationConfig() *IntegrationConfig {
	return &IntegrationConfig{
		DynamoDBEndpoint:  "http://localhost:8000",
		DynamoDBRegion:    "us-east-1",
		TableName:         "lesser-test",
		DefaultTimeout:    30 * time.Second,
		SkipTableCreation: false,
		PreserveData:      false,
		MaxResponseTime:   5 * time.Second,
		MaxMemoryUsage:    128 * 1024 * 1024, // 128MB
	}
}

// NewIntegrationTestSuite creates a new integration test suite
func NewIntegrationTestSuite(t *testing.T, config *IntegrationConfig) *IntegrationTestSuite {
	if config == nil {
		config = DefaultIntegrationConfig()
	}

	suite := &IntegrationTestSuite{
		TestData: NewTestDataManager(),
		Cleanup:  make([]func(), 0),
		Config:   config,
		t:        t,
	}

	// Setup storage connection
	suite.setupStorage()

	// Setup test tables if needed
	if !config.SkipTableCreation {
		suite.setupTables()
	}

	return suite
}

// setupStorage initializes the storage connection
func (suite *IntegrationTestSuite) setupStorage() {
	// In a real implementation, this would set up DynamoDB connection
	// For now, we'll use a mock that can be replaced in actual integration tests
	suite.Storage = &MockStorage{}

	// Add cleanup for storage connection
	suite.AddCleanup(func() {
		// Close storage connections
	})
}

// setupTables creates test tables
func (suite *IntegrationTestSuite) setupTables() {
	// Create test tables with proper schema
	// This would use actual DynamoDB table creation in real implementation

	// Add cleanup for tables
	if !suite.Config.PreserveData {
		suite.AddCleanup(func() {
			suite.dropTables()
		})
	}
}

// dropTables removes test tables
func (suite *IntegrationTestSuite) dropTables() {
	// Drop test tables
	// Implementation would delete DynamoDB tables
}

// AddCleanup adds a cleanup function to be called at the end of tests
func (suite *IntegrationTestSuite) AddCleanup(fn func()) {
	suite.Cleanup = append(suite.Cleanup, fn)
}

// RunCleanup executes all cleanup functions
func (suite *IntegrationTestSuite) RunCleanup() {
	for i := len(suite.Cleanup) - 1; i >= 0; i-- {
		suite.Cleanup[i]()
	}
}

// CreateTestApp creates a test app configured for integration testing
func (suite *IntegrationTestSuite) CreateTestApp() *TestApp {
	app := NewTestApp()

	// Configure app with real storage (not mocks)
	// This would integrate with the actual Lift middleware stack

	return app
}

// TestDataManager manages test data lifecycle
type TestDataManager struct {
	CreatedActors     []*models.Actor
	CreatedStatuses   []*models.Status
	CreatedActivities []*models.Activity
	cleanup           []func() error
}

// NewTestDataManager creates a new test data manager
func NewTestDataManager() *TestDataManager {
	return &TestDataManager{
		CreatedActors:     make([]*models.Actor, 0),
		CreatedStatuses:   make([]*models.Status, 0),
		CreatedActivities: make([]*models.Activity, 0),
		cleanup:           make([]func() error, 0),
	}
}

// CreateTestActor creates a test actor and tracks it for cleanup
func (tdm *TestDataManager) CreateTestActor(ctx context.Context, storage *MockStorage, username string) (*models.Actor, error) {
	actor := BuildTestActor(username)

	err := storage.CreateActor(ctx, actor)
	if err != nil {
		return nil, err
	}

	tdm.CreatedActors = append(tdm.CreatedActors, actor)
	tdm.cleanup = append(tdm.cleanup, func() error {
		return storage.DeleteActor(ctx, actor.PK)
	})

	return actor, nil
}

// CreateTestStatus creates a test status and tracks it for cleanup
func (tdm *TestDataManager) CreateTestStatus(ctx context.Context, storage *MockStorage, actorID, content string) (*models.Status, error) {
	status := BuildTestStatus(actorID, content)

	err := storage.CreateStatus(ctx, status)
	if err != nil {
		return nil, err
	}

	tdm.CreatedStatuses = append(tdm.CreatedStatuses, status)
	tdm.cleanup = append(tdm.cleanup, func() error {
		return storage.DeleteStatus(ctx, status.PK)
	})

	return status, nil
}

// CreateTestActivity creates a test activity and tracks it for cleanup
func (tdm *TestDataManager) CreateTestActivity(ctx context.Context, storage *MockStorage, actorID, activityType string) (*models.Activity, error) {
	activity := BuildTestActivity(actorID, activityType)

	err := storage.CreateActivity(ctx, actorID, activityType)
	if err != nil {
		return nil, err
	}

	tdm.CreatedActivities = append(tdm.CreatedActivities, activity)

	return activity, nil
}

// Cleanup removes all created test data
func (tdm *TestDataManager) Cleanup() error {
	var lastError error

	// Run cleanup functions in reverse order
	for i := len(tdm.cleanup) - 1; i >= 0; i-- {
		if err := tdm.cleanup[i](); err != nil {
			lastError = err
		}
	}

	// Clear tracking arrays
	tdm.CreatedActors = tdm.CreatedActors[:0]
	tdm.CreatedStatuses = tdm.CreatedStatuses[:0]
	tdm.CreatedActivities = tdm.CreatedActivities[:0]
	tdm.cleanup = tdm.cleanup[:0]

	return lastError
}

// Integration Test Helpers

// RunIntegrationTest runs an integration test with proper setup and teardown
func RunIntegrationTest(t *testing.T, config *IntegrationConfig, testFn func(*IntegrationTestSuite)) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if integration tests should run
	if os.Getenv("INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TESTS=true to run")
	}

	suite := NewIntegrationTestSuite(t, config)
	defer suite.RunCleanup()

	testFn(suite)
}

// AssertIntegrationResponse validates integration test responses
func AssertIntegrationResponse(t *testing.T, response *TestResponse, expectedStatus int) {
	t.Helper()

	require.NotNil(t, response, "Response should not be nil")
	assert.Equal(t, expectedStatus, response.StatusCode, "Unexpected status code")

	if expectedStatus >= 200 && expectedStatus < 300 {
		assert.NotEmpty(t, response.Body, "Success response should have body")
	}
}

// AssertPerformanceThresholds validates performance requirements
func AssertPerformanceThresholds(t *testing.T, duration time.Duration, config *IntegrationConfig) {
	t.Helper()

	if config.MaxResponseTime > 0 {
		assert.LessOrEqual(t, duration, config.MaxResponseTime,
			"Response time %v exceeded threshold %v", duration, config.MaxResponseTime)
	}
}

// Database Integration Helpers

// WaitForDatabase waits for database to be ready
func WaitForDatabase(endpoint string, timeout time.Duration) error {
	// Implementation would ping DynamoDB Local
	// For now, return success
	return nil
}

// CreateTestTables creates all necessary test tables
func CreateTestTables(storage *MockStorage) error {
	// Implementation would create actual DynamoDB tables
	return nil
}

// DropTestTables removes all test tables
func DropTestTables(storage *MockStorage) error {
	// Implementation would drop actual DynamoDB tables
	return nil
}

// Environment Management

// TestEnvironmentManager manages test environment state
type TestEnvironmentManager struct {
	startupTime time.Time
	endpoints   map[string]string
	processes   []string
}

// NewTestEnvironmentManager creates a new environment manager
func NewTestEnvironmentManager() *TestEnvironmentManager {
	return &TestEnvironmentManager{
		startupTime: time.Now(),
		endpoints:   make(map[string]string),
		processes:   make([]string, 0),
	}
}

// StartServices starts external test services (like DynamoDB Local)
func (tem *TestEnvironmentManager) StartServices() error {
	// Implementation would start DynamoDB Local, etc.
	return nil
}

// StopServices stops all started services
func (tem *TestEnvironmentManager) StopServices() error {
	// Implementation would stop all services
	return nil
}

// GetServiceEndpoint returns the endpoint for a service
func (tem *TestEnvironmentManager) GetServiceEndpoint(service string) string {
	return tem.endpoints[service]
}

// IsServiceReady checks if a service is ready to accept connections
func (tem *TestEnvironmentManager) IsServiceReady(service string) bool {
	// Implementation would check service health
	return true
}

// Integration Test Patterns

// TestWorkflow represents a complete integration test workflow
type TestWorkflow struct {
	Name     string
	Setup    func(*IntegrationTestSuite) error
	Execute  func(*IntegrationTestSuite) (*TestResponse, error)
	Validate func(*TestResponse) error
	Cleanup  func(*IntegrationTestSuite) error
}

// RunWorkflow executes a complete test workflow
func (suite *IntegrationTestSuite) RunWorkflow(workflow *TestWorkflow) error {
	// Setup
	if workflow.Setup != nil {
		if err := workflow.Setup(suite); err != nil {
			return fmt.Errorf("setup failed: %w", err)
		}
	}

	// Execute
	response, err := workflow.Execute(suite)
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	// Validate
	if workflow.Validate != nil {
		if err := workflow.Validate(response); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}

	// Cleanup
	if workflow.Cleanup != nil {
		if err := workflow.Cleanup(suite); err != nil {
			return fmt.Errorf("cleanup failed: %w", err)
		}
	}

	return nil
}

// Common Integration Test Workflows

// CreateUserWorkflow tests complete user creation flow
func CreateUserWorkflow(username string) *TestWorkflow {
	return &TestWorkflow{
		Name: fmt.Sprintf("CreateUser-%s", username),
		Setup: func(suite *IntegrationTestSuite) error {
			// Setup test data if needed
			return nil
		},
		Execute: func(suite *IntegrationTestSuite) (*TestResponse, error) {
			app := suite.CreateTestApp()
			request := NewCreateUserRequest(username)
			response := app.POST("/api/v1/accounts", request)
			return response, nil
		},
		Validate: func(response *TestResponse) error {
			if !response.IsSuccess() {
				return fmt.Errorf("expected success, got %d", response.StatusCode)
			}
			return nil
		},
		Cleanup: func(suite *IntegrationTestSuite) error {
			// Cleanup created user if needed
			return suite.TestData.Cleanup()
		},
	}
}

// FollowWorkflow tests complete follow relationship flow
func FollowWorkflow(followerID, followeeID string) *TestWorkflow {
	return &TestWorkflow{
		Name: fmt.Sprintf("Follow-%s-%s", followerID, followeeID),
		Setup: func(suite *IntegrationTestSuite) error {
			// Ensure both actors exist
			ctx := context.Background()
			_, err := suite.TestData.CreateTestActor(ctx, suite.Storage, followerID)
			if err != nil {
				return err
			}
			_, err = suite.TestData.CreateTestActor(ctx, suite.Storage, followeeID)
			return err
		},
		Execute: func(suite *IntegrationTestSuite) (*TestResponse, error) {
			app := suite.CreateTestApp()
			token := GenerateTestToken(followerID, "standard")
			response := app.
				WithHeader("Authorization", "Bearer "+token).
				POST(fmt.Sprintf("/api/v1/accounts/%s/follow", followeeID), nil)
			return response, nil
		},
		Validate: func(response *TestResponse) error {
			if response.StatusCode != 200 {
				return fmt.Errorf("expected 200, got %d", response.StatusCode)
			}
			return nil
		},
		Cleanup: func(suite *IntegrationTestSuite) error {
			return suite.TestData.Cleanup()
		},
	}
}
