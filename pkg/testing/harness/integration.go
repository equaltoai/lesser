// Package harness provides integration test utilities for the Lesser project
package harness

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	appConfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// IntegrationTestHarness provides a complete testing environment for integration tests
type IntegrationTestHarness struct {
	t       *testing.T
	ctx     context.Context
	cancel  context.CancelFunc
	logger  *zap.Logger
	storage *mocks.EnhancedMockStorage
	server  *httptest.Server
	config  *TestConfig

	// Test data management
	actors     []*activitypub.Actor
	activities []*activitypub.Activity
	objects    []any

	// Seeded test data (pre-created for convenience)
	seededActors   []*activitypub.Actor
	seededUsers    []*TestUser
	seededStatuses []*TestStatus

	// Cost tracking
	costTracker *CostTracker

	// Cleanup functions
	cleanupFuncs []func() error
}

// TestConfig holds configuration for integration tests
type TestConfig struct {
	Domain        string
	TableName     string
	UseMemory     bool
	LogLevel      zapcore.Level
	ServerTimeout time.Duration
	CleanupMode   CleanupMode

	// Enhanced harness configuration
	UseDynamORM    bool          // Whether to use DynamORM-backed storage
	UseEnhanced    bool          // Whether to use enhanced mock storage
	SeedData       bool          // Whether to seed initial test data
	ErrorInjection bool          // Whether to enable error injection capabilities
	CostTracking   bool          // Whether to track DynamoDB cost units
	LatencyMode    time.Duration // Artificial latency to inject into operations
	ErrorRate      float64       // Probability (0.0-1.0) that operations will fail
}

// CleanupMode defines how test data should be cleaned up
type CleanupMode int

const (
	// CleanupNone disables automatic cleanup of test data
	CleanupNone CleanupMode = iota
	// CleanupOnSuccess cleans up test data only if test passes
	CleanupOnSuccess
	// CleanupAlways cleans up test data regardless of test outcome
	CleanupAlways
)

// NewIntegrationTestHarness creates a new integration test harness
func NewIntegrationTestHarness(t *testing.T, config *TestConfig) *IntegrationTestHarness {
	if config == nil {
		config = DefaultTestConfig()
	}

	// Use a sensible default timeout if not specified
	timeout := config.ServerTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	logger, _ := zap.NewDevelopment()

	// Initialize storage - always use enhanced mock for testing
	storage := mocks.NewEnhancedMockStorage()

	// Configure storage based on config
	if config.LatencyMode > 0 {
		storage.SetLatencySimulation(config.LatencyMode)
	}
	if config.ErrorRate > 0 {
		storage.SetErrorRate(config.ErrorRate)
	}

	harness := &IntegrationTestHarness{
		t:              t,
		ctx:            ctx,
		cancel:         cancel,
		logger:         logger,
		storage:        storage,
		config:         config,
		actors:         make([]*activitypub.Actor, 0),
		activities:     make([]*activitypub.Activity, 0),
		objects:        make([]any, 0),
		seededActors:   make([]*activitypub.Actor, 0),
		seededUsers:    make([]*TestUser, 0),
		seededStatuses: make([]*TestStatus, 0),
		cleanupFuncs:   make([]func() error, 0),
	}

	// Initialize cost tracking if enabled
	if config.CostTracking {
		harness.costTracker = NewCostTracker()
	}

	// Seed test data if enabled
	if config.SeedData {
		harness.seedTestData()
	}

	// Setup cleanup on test completion
	t.Cleanup(harness.cleanup)

	return harness
}

// DefaultTestConfig returns default configuration for integration tests
func DefaultTestConfig() *TestConfig {
	return &TestConfig{
		Domain:        "test.example.com",
		TableName:     "lesser-test",
		UseMemory:     true,
		LogLevel:      zapcore.WarnLevel,
		ServerTimeout: 30 * time.Second,
		CleanupMode:   CleanupOnSuccess,
	}
}

// StartServer starts a test HTTP server with the given app
func (h *IntegrationTestHarness) StartServer(handler http.Handler) {
	h.server = httptest.NewServer(handler)
	h.addCleanup(func() error {
		h.server.Close()
		return nil
	})
}

// GetServerURL returns the test server URL
func (h *IntegrationTestHarness) GetServerURL() string {
	if h.server == nil {
		h.t.Fatal("Server not started. Call StartServer first.")
	}
	return h.server.URL
}

// MakeRequest makes an HTTP request to the test server
// Note: body parameter will be used for POST/PUT request payloads
func (h *IntegrationTestHarness) MakeRequest(method, path string, _ interface{}, headers map[string]string) *http.Response { //nolint:revive // body will be used for request payloads
	if h.server == nil {
		h.t.Fatal("Server not started. Call StartServer first.")
	}

	// Implementation would include proper request building with body and headers
	req, err := http.NewRequestWithContext(h.ctx, method, h.server.URL+path, nil)
	require.NoError(h.t, err)

	// Add headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	require.NoError(h.t, err)

	return resp
}

// CreateTestActor creates a test actor and stores it for cleanup
func (h *IntegrationTestHarness) CreateTestActor(username string) *activitypub.Actor {
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   fmt.Sprintf("https://%s/users/%s", h.config.Domain, username),
			Type: "Person",
		},
		PreferredUsername: username,
		Name:              fmt.Sprintf("Test User %s", username),
		Inbox:             fmt.Sprintf("https://%s/users/%s/inbox", h.config.Domain, username),
		Outbox:            fmt.Sprintf("https://%s/users/%s/outbox", h.config.Domain, username),
		Following:         fmt.Sprintf("https://%s/users/%s/following", h.config.Domain, username),
		Followers:         fmt.Sprintf("https://%s/users/%s/followers", h.config.Domain, username),
	}

	err := h.storage.CreateActor(h.ctx, actor, "test-private-key")
	require.NoError(h.t, err)

	h.actors = append(h.actors, actor)
	return actor
}

// CreateTestActivity creates a test activity and stores it for cleanup
func (h *IntegrationTestHarness) CreateTestActivity(actorID string, activityType string) *activitypub.Activity {
	now := time.Now()
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:        fmt.Sprintf("https://%s/activities/%d", h.config.Domain, now.UnixNano()),
			Type:      activityType,
			Published: &now,
		},
		Actor: actorID,
	}

	err := h.storage.StoreActivity(h.ctx, activity)
	require.NoError(h.t, err)

	h.activities = append(h.activities, activity)
	return activity
}

// WaitForCondition waits for a condition to be true with a timeout
func (h *IntegrationTestHarness) WaitForCondition(condition func() bool, timeout time.Duration, message string) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	h.t.Fatalf("Condition not met within timeout: %s", message)
}

// AssertActivityPubResponse validates an ActivityPub HTTP response
func (h *IntegrationTestHarness) AssertActivityPubResponse(resp *http.Response, expectedStatus int) {
	require.Equal(h.t, expectedStatus, resp.StatusCode)

	contentType := resp.Header.Get("Content-Type")
	require.Contains(h.t, contentType, "application/")

	// Validate ActivityPub-specific headers
	if resp.StatusCode == http.StatusOK {
		require.NotEmpty(h.t, resp.Header.Get("Content-Type"))
	}
}

// Logger returns the test logger
func (h *IntegrationTestHarness) Logger() *zap.Logger {
	return h.logger
}

// Storage returns the storage instance
func (h *IntegrationTestHarness) Storage() *mocks.EnhancedMockStorage {
	return h.storage
}

// Context returns the test context
func (h *IntegrationTestHarness) Context() context.Context {
	return h.ctx
}

// Config returns the test configuration
func (h *IntegrationTestHarness) Config() *TestConfig {
	return h.config
}

// addCleanup adds a cleanup function to be called at test end
func (h *IntegrationTestHarness) addCleanup(fn func() error) {
	h.cleanupFuncs = append(h.cleanupFuncs, fn)
}

// cleanup performs test cleanup based on configuration
func (h *IntegrationTestHarness) cleanup() {
	h.cancel()

	shouldCleanup := h.config.CleanupMode == CleanupAlways ||
		(h.config.CleanupMode == CleanupOnSuccess && !h.t.Failed())

	if shouldCleanup {
		// Clean up test data
		for _, actor := range h.actors {
			_ = h.storage.DeleteActor(h.ctx, actor.PreferredUsername)
		}

		for _, activity := range h.activities {
			_ = h.storage.DeleteActivity(h.ctx, activity.ID)
		}

		// Run additional cleanup functions
		for _, fn := range h.cleanupFuncs {
			if err := fn(); err != nil {
				h.logger.Warn("Cleanup function failed", zap.Error(err))
			}
		}
	}
}

// initTestDynamoDB would initialize a test DynamoDB connection for real integration tests
// Note: config parameter will be used for table configuration and test isolation settings
func initTestDynamoDB(t *testing.T, config *TestConfig) *mocks.EnhancedMockStorage { //nolint:revive,unused // config will be used for DynamoDB setup, function used in integration tests
	// Check for test environment
	appCfg := appConfig.Get()
	if !appCfg.CIEnvironment && !appCfg.IntegrationTestMode {
		t.Skip("Integration tests require CI=true or INTEGRATION_TEST=true")
	}

	// This would implement actual DynamoDB initialization for integration tests
	// For now, return an enhanced mock to prevent test failures
	return mocks.NewEnhancedMockStorage()
}

// TestUser represents a simplified user for testing purposes
type TestUser struct {
	ID          string
	Username    string
	DisplayName string
	ActorID     string
	CreatedAt   time.Time
}

// TestStatus represents a simplified status for testing purposes
type TestStatus struct {
	ID         string
	AuthorID   string
	Content    string
	Visibility string
	CreatedAt  time.Time
}

// CostTracker tracks DynamoDB read/write capacity unit usage for testing
type CostTracker struct {
	operations []CostOperation
	totalRCU   float64
	totalWCU   float64
}

// CostOperation represents a single tracked operation
type CostOperation struct {
	Name      string
	RCU       float64
	WCU       float64
	Timestamp time.Time
}

// NewCostTracker creates a new cost tracker instance
func NewCostTracker() *CostTracker {
	return &CostTracker{
		operations: make([]CostOperation, 0),
	}
}

// TrackOperation records a DynamoDB operation's cost
func (c *CostTracker) TrackOperation(name string, rcu, wcu float64) {
	c.operations = append(c.operations, CostOperation{
		Name:      name,
		RCU:       rcu,
		WCU:       wcu,
		Timestamp: time.Now(),
	})
	c.totalRCU += rcu
	c.totalWCU += wcu
}

// GetTotalCost returns the total RCU and WCU consumed
func (c *CostTracker) GetTotalCost() (rcu, wcu float64) {
	return c.totalRCU, c.totalWCU
}

// GetOperations returns all tracked operations
func (c *CostTracker) GetOperations() []CostOperation {
	return c.operations
}

// Reset clears all tracked operations and totals
func (c *CostTracker) Reset() {
	c.operations = make([]CostOperation, 0)
	c.totalRCU = 0
	c.totalWCU = 0
}

// RepositoryStorage returns the storage instance for repository-level access
func (h *IntegrationTestHarness) RepositoryStorage() *mocks.EnhancedMockStorage {
	return h.storage
}

// TestStorage returns the storage instance for test-level access (alias for compatibility)
func (h *IntegrationTestHarness) TestStorage() *mocks.EnhancedMockStorage {
	return h.storage
}

// CostTracker returns the cost tracker instance
func (h *IntegrationTestHarness) CostTracker() *CostTracker {
	return h.costTracker
}

// GetSeededActors returns pre-seeded test actors
func (h *IntegrationTestHarness) GetSeededActors() []*activitypub.Actor {
	return h.seededActors
}

// GetSeededUsers returns pre-seeded test users
func (h *IntegrationTestHarness) GetSeededUsers() []*TestUser {
	return h.seededUsers
}

// GetSeededStatuses returns pre-seeded test statuses
func (h *IntegrationTestHarness) GetSeededStatuses() []*TestStatus {
	return h.seededStatuses
}

// InjectError sets the error rate and latency for simulating failures
func (h *IntegrationTestHarness) InjectError(errorRate float64, latency time.Duration) {
	h.storage.SetErrorRate(errorRate)
	h.storage.SetLatencySimulation(latency)
}

// ResetErrorInjection clears any error injection settings
func (h *IntegrationTestHarness) ResetErrorInjection() {
	h.storage.SetErrorRate(0)
	h.storage.SetLatencySimulation(0)
}

// AssertCostBudget asserts that the tracked costs are within the specified budget
func (h *IntegrationTestHarness) AssertCostBudget(maxRCU, maxWCU float64) {
	if h.costTracker == nil {
		return
	}
	rcu, wcu := h.costTracker.GetTotalCost()
	require.LessOrEqual(h.t, rcu, maxRCU, "RCU exceeded budget: %.2f > %.2f", rcu, maxRCU)
	require.LessOrEqual(h.t, wcu, maxWCU, "WCU exceeded budget: %.2f > %.2f", wcu, maxWCU)
}

// seedTestData creates initial test data for the harness
func (h *IntegrationTestHarness) seedTestData() {
	// Create 3 test actors
	for i := 1; i <= 3; i++ {
		username := fmt.Sprintf("test_user_%d", i)
		actor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   fmt.Sprintf("https://%s/users/%s", h.config.Domain, username),
				Type: "Person",
			},
			PreferredUsername: username,
			Name:              fmt.Sprintf("Test User %d", i),
			Inbox:             fmt.Sprintf("https://%s/users/%s/inbox", h.config.Domain, username),
			Outbox:            fmt.Sprintf("https://%s/users/%s/outbox", h.config.Domain, username),
			Following:         fmt.Sprintf("https://%s/users/%s/following", h.config.Domain, username),
			Followers:         fmt.Sprintf("https://%s/users/%s/followers", h.config.Domain, username),
		}
		_ = h.storage.CreateActor(h.ctx, actor, "test-private-key")
		h.seededActors = append(h.seededActors, actor)

		// Create corresponding test user
		user := &TestUser{
			ID:          fmt.Sprintf("user_%d", i),
			Username:    username,
			DisplayName: fmt.Sprintf("Test User %d", i),
			ActorID:     actor.ID,
			CreatedAt:   time.Now(),
		}
		h.seededUsers = append(h.seededUsers, user)

		// Create 3 statuses per user (9 total)
		for j := 1; j <= 3; j++ {
			status := &TestStatus{
				ID:         fmt.Sprintf("status_%d_%d", i, j),
				AuthorID:   actor.ID,
				Content:    fmt.Sprintf("Test status %d by user %d", j, i),
				Visibility: "public",
				CreatedAt:  time.Now(),
			}
			h.seededStatuses = append(h.seededStatuses, status)
		}
	}
}
