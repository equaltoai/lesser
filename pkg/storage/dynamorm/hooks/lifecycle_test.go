package hooks

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

const (
	// These keys intentionally match the string keys used by this package's helpers
	// (see getUserIDFromContext/getLoggerFromContext/etc).
	loggerKey    = "logger"
	userIDKey    = "user_id"
	requestIDKey = "request_id"

	notificationRepositoryKey = "notification_repository"
)

// Test model types
type TestUser struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (u *TestUser) Validate() error {
	if u.Username == "" {
		return errors.New("username is required")
	}
	if u.Email == "" {
		return errors.New("email is required")
	}
	return nil
}

func (u *TestUser) GetFollowerID() string      { return u.ID }
func (u *TestUser) GetFolloweeID() string      { return u.ID }
func (u *TestUser) GetUserID() string          { return u.ID }
func (u *TestUser) GetContent() string         { return u.Username }
func (u *TestUser) GetMentions() []string      { return []string{} }
func (u *TestUser) GetMentionedUserID() string { return u.ID }
func (u *TestUser) GetStatusID() string        { return u.ID }

type TestStatus struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Content   string    `json:"content"`
	Mentions  []string  `json:"mentions"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *TestStatus) Validate() error {
	if s.Content == "" {
		return errors.New("content is required")
	}
	return nil
}

func (s *TestStatus) GetUserID() string     { return s.UserID }
func (s *TestStatus) GetContent() string    { return s.Content }
func (s *TestStatus) GetMentions() []string { return s.Mentions }

func TestNewHookRegistry(t *testing.T) {
	logger := zap.NewNop()
	registry := NewHookRegistry(logger)

	assert.NotNil(t, registry)
	assert.NotNil(t, registry.hooks)
	assert.NotNil(t, registry.asyncHooks)
	assert.NotNil(t, registry.conditional)
	assert.Equal(t, logger, registry.logger)
}

func TestHookRegistry_Register(t *testing.T) {
	registry := NewHookRegistry(zap.NewNop())
	userType := reflect.TypeOf(TestUser{})

	hookCalled := false
	testHook := func(_ context.Context, _ any) error {
		hookCalled = true
		return nil
	}

	registry.Register(userType, BeforeCreate, testHook)

	count := registry.GetRegisteredHooksCount(userType, BeforeCreate)
	assert.Equal(t, 1, count)

	// Execute the hook
	err := registry.Execute(context.Background(), &TestUser{}, BeforeCreate)
	assert.NoError(t, err)
	assert.True(t, hookCalled)
}

func TestHookRegistry_RegisterAsync(t *testing.T) {
	registry := NewHookRegistry(zap.NewNop())
	userType := reflect.TypeOf(TestUser{})

	hookCalled := false
	var wg sync.WaitGroup
	wg.Add(1)

	asyncHook := func(_ context.Context, _ any) {
		hookCalled = true
		wg.Done()
	}

	registry.RegisterAsync(userType, AfterCreate, asyncHook)

	count := registry.GetRegisteredHooksCount(userType, AfterCreate)
	assert.Equal(t, 1, count)

	// Execute the async hook
	err := registry.Execute(context.Background(), &TestUser{}, AfterCreate)
	assert.NoError(t, err)

	// Wait for async hook to complete
	wg.Wait()
	assert.True(t, hookCalled)
}

func TestHookRegistry_RegisterConditional(t *testing.T) {
	registry := NewHookRegistry(zap.NewNop())
	userType := reflect.TypeOf(TestUser{})

	hookCalled := false
	condition := func(_ context.Context, model any) bool {
		user := model.(*TestUser)
		return user.Username == "test"
	}

	conditionalHook := func(_ context.Context, _ any) error {
		hookCalled = true
		return nil
	}

	registry.RegisterConditional(userType, BeforeUpdate, condition, conditionalHook)

	count := registry.GetRegisteredHooksCount(userType, BeforeUpdate)
	assert.Equal(t, 1, count)

	// Execute with condition that should trigger
	user1 := &TestUser{Username: "test"}
	err := registry.Execute(context.Background(), user1, BeforeUpdate)
	assert.NoError(t, err)
	assert.True(t, hookCalled)

	// Reset and execute with condition that should not trigger
	hookCalled = false
	user2 := &TestUser{Username: "other"}
	err = registry.Execute(context.Background(), user2, BeforeUpdate)
	assert.NoError(t, err)
	assert.False(t, hookCalled)
}

func TestHookRegistry_Execute_Error(t *testing.T) {
	registry := NewHookRegistry(zap.NewNop())
	userType := reflect.TypeOf(TestUser{})

	expectedError := errors.New("hook failed")
	failingHook := func(_ context.Context, _ any) error {
		return expectedError
	}

	registry.Register(userType, BeforeCreate, failingHook)

	err := registry.Execute(context.Background(), &TestUser{}, BeforeCreate)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hook failed")
}

func TestHookRegistry_Execute_MultipleHooks(t *testing.T) {
	registry := NewHookRegistry(zap.NewNop())
	userType := reflect.TypeOf(TestUser{})

	execOrder := make([]int, 0)
	var mu sync.Mutex

	hook1 := func(_ context.Context, _ any) error {
		mu.Lock()
		execOrder = append(execOrder, 1)
		mu.Unlock()
		return nil
	}

	hook2 := func(_ context.Context, _ any) error {
		mu.Lock()
		execOrder = append(execOrder, 2)
		mu.Unlock()
		return nil
	}

	hook3 := func(_ context.Context, _ any) error {
		mu.Lock()
		execOrder = append(execOrder, 3)
		mu.Unlock()
		return nil
	}

	registry.Register(userType, BeforeCreate, hook1)
	registry.Register(userType, BeforeCreate, hook2)
	registry.Register(userType, BeforeCreate, hook3)

	err := registry.Execute(context.Background(), &TestUser{}, BeforeCreate)
	assert.NoError(t, err)

	mu.Lock()
	assert.Equal(t, []int{1, 2, 3}, execOrder)
	mu.Unlock()
}

func TestHookRegistry_Execute_WithCostTracking(t *testing.T) {
	registry := NewHookRegistry(zap.NewNop())
	userType := reflect.TypeOf(TestUser{})

	slowHook := func(_ context.Context, _ any) error {
		time.Sleep(2 * time.Millisecond) // Small delay to trigger cost tracking
		return nil
	}

	registry.Register(userType, BeforeCreate, slowHook)

	// Create context with cost tracker
	tracker := cost.New()
	ctx := cost.WithTracker(context.Background(), tracker)

	err := registry.Execute(ctx, &TestUser{}, BeforeCreate)
	assert.NoError(t, err)

	// Verify cost was tracked
	costSummary := tracker.CalculateCost()
	assert.Greater(t, costSummary.LambdaInvocations, int64(0))
}

func TestHookRegistry_Clear(t *testing.T) {
	registry := NewHookRegistry(zap.NewNop())
	userType := reflect.TypeOf(TestUser{})

	testHook := func(_ context.Context, _ any) error { return nil }
	registry.Register(userType, BeforeCreate, testHook)

	count := registry.GetRegisteredHooksCount(userType, BeforeCreate)
	assert.Equal(t, 1, count)

	registry.Clear(userType)

	count = registry.GetRegisteredHooksCount(userType, BeforeCreate)
	assert.Equal(t, 0, count)
}

func TestHookRegistry_ClearAll(t *testing.T) {
	registry := NewHookRegistry(zap.NewNop())
	userType := reflect.TypeOf(TestUser{})
	statusType := reflect.TypeOf(TestStatus{})

	testHook := func(_ context.Context, _ any) error { return nil }
	registry.Register(userType, BeforeCreate, testHook)
	registry.Register(statusType, BeforeCreate, testHook)

	assert.Equal(t, 1, registry.GetRegisteredHooksCount(userType, BeforeCreate))
	assert.Equal(t, 1, registry.GetRegisteredHooksCount(statusType, BeforeCreate))

	registry.ClearAll()

	assert.Equal(t, 0, registry.GetRegisteredHooksCount(userType, BeforeCreate))
	assert.Equal(t, 0, registry.GetRegisteredHooksCount(statusType, BeforeCreate))
}

func TestGlobalRegistry(t *testing.T) {
	// Clear any existing global registry
	SetGlobalRegistry(NewHookRegistry(zap.NewNop()))

	userType := reflect.TypeOf(TestUser{})
	hookCalled := false

	testHook := func(_ context.Context, _ any) error {
		hookCalled = true
		return nil
	}

	// Test global registration
	Register(userType, BeforeCreate, testHook)

	// Test global execution
	err := Execute(context.Background(), &TestUser{}, BeforeCreate)
	assert.NoError(t, err)
	assert.True(t, hookCalled)

	// Test global async registration
	asyncCalled := false
	var wg sync.WaitGroup
	wg.Add(1)

	asyncHook := func(_ context.Context, _ any) {
		asyncCalled = true
		wg.Done()
	}

	RegisterAsync(userType, AfterCreate, asyncHook)
	err = Execute(context.Background(), &TestUser{}, AfterCreate)
	assert.NoError(t, err)

	wg.Wait()
	assert.True(t, asyncCalled)

	// Test global conditional registration
	conditionalCalled := false
	condition := func(_ context.Context, _ any) bool { return true }
	conditionalHook := func(_ context.Context, _ any) error {
		conditionalCalled = true
		return nil
	}

	RegisterConditional(userType, BeforeUpdate, condition, conditionalHook)
	err = Execute(context.Background(), &TestUser{}, BeforeUpdate)
	assert.NoError(t, err)
	assert.True(t, conditionalCalled)
}

// Test predefined hooks

func TestAuditHook(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.WithValue(context.Background(), loggerKey, logger)
	ctx = context.WithValue(ctx, userIDKey, "user123")
	ctx = context.WithValue(ctx, requestIDKey, "req456")

	user := &TestUser{Username: "test"}
	err := AuditHook(ctx, user)
	assert.NoError(t, err)
}

func TestValidationHook(t *testing.T) {
	// Test with valid model
	validUser := &TestUser{Username: "test", Email: "test@example.com"}
	err := ValidationHook(context.Background(), validUser)
	assert.NoError(t, err)

	// Test with invalid model
	invalidUser := &TestUser{Username: "", Email: ""}
	err = ValidationHook(context.Background(), invalidUser)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "username is required")
}

func TestTimestampHook(t *testing.T) {
	user := &TestUser{
		Username:  "test",
		UpdatedAt: time.Time{}, // Zero time
	}

	err := TimestampHook(context.Background(), user)
	assert.NoError(t, err)
	assert.False(t, user.UpdatedAt.IsZero())
}

func TestNotificationHook(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.WithValue(context.Background(), loggerKey, logger)

	// Test with FollowModel
	user := &TestUser{ID: "user1"}
	err := NotificationHook(ctx, user)
	assert.NoError(t, err)

	// Test with StatusModel
	status := &TestStatus{
		ID:       "status1",
		UserID:   "user1",
		Content:  "Hello @user2",
		Mentions: []string{"user2"},
	}
	err = NotificationHook(ctx, status)
	assert.NoError(t, err)
}

func TestCacheInvalidationHook(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.WithValue(context.Background(), loggerKey, logger)

	user := &TestUser{Username: "test"}
	err := CacheInvalidationHook(ctx, user)
	assert.NoError(t, err)
}

func TestSearchIndexHook(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.WithValue(context.Background(), loggerKey, logger)

	user := &TestUser{Username: "test"}
	err := SearchIndexHook(ctx, user)
	assert.NoError(t, err)
}

// Test hook statistics

func TestNewHookStatsTracker(t *testing.T) {
	tracker := NewHookStatsTracker()
	assert.NotNil(t, tracker)
	assert.NotNil(t, tracker.stats)
}

func TestHookStatsTracker_TrackExecution(t *testing.T) {
	tracker := NewHookStatsTracker()
	userType := reflect.TypeOf(TestUser{})

	// Track successful execution
	tracker.TrackExecution(userType, BeforeCreate, 10*time.Millisecond, nil)

	stats := tracker.GetStats()
	key := "hooks.TestUser:before_create"
	assert.Contains(t, stats, key)

	stat := stats[key]
	assert.Equal(t, int64(1), stat.TotalExecutions)
	assert.Equal(t, 10*time.Millisecond, stat.TotalDuration)
	assert.Equal(t, 10*time.Millisecond, stat.AverageDuration)
	assert.Equal(t, int64(0), stat.ErrorCount)
	assert.False(t, stat.LastExecution.IsZero())

	// Track failed execution
	tracker.TrackExecution(userType, BeforeCreate, 5*time.Millisecond, errors.New("failed"))

	stats = tracker.GetStats()
	stat = stats[key]
	assert.Equal(t, int64(2), stat.TotalExecutions)
	assert.Equal(t, 15*time.Millisecond, stat.TotalDuration)
	assert.Equal(t, 7500*time.Microsecond, stat.AverageDuration) // 15ms / 2
	assert.Equal(t, int64(1), stat.ErrorCount)
}

func TestHookStatsTracker_Reset(t *testing.T) {
	tracker := NewHookStatsTracker()
	userType := reflect.TypeOf(TestUser{})

	tracker.TrackExecution(userType, BeforeCreate, 10*time.Millisecond, nil)
	stats := tracker.GetStats()
	assert.Len(t, stats, 1)

	tracker.Reset()
	stats = tracker.GetStats()
	assert.Len(t, stats, 0)
}

func TestGetGlobalStatsTracker(t *testing.T) {
	tracker1 := GetGlobalStatsTracker()
	tracker2 := GetGlobalStatsTracker()

	assert.NotNil(t, tracker1)
	assert.Equal(t, tracker1, tracker2) // Should be same instance
}

// Integration tests

func TestHookRegistry_Integration(t *testing.T) {
	registry := NewHookRegistry(zap.NewNop())
	userType := reflect.TypeOf(TestUser{})

	// Register multiple types of hooks
	syncCalled := false
	asyncCalled := false
	conditionalCalled := false

	var wg sync.WaitGroup
	wg.Add(1)

	syncHook := func(_ context.Context, _ any) error {
		syncCalled = true
		return nil
	}

	asyncHook := func(_ context.Context, _ any) {
		asyncCalled = true
		wg.Done()
	}

	condition := func(_ context.Context, _ any) bool {
		return true
	}

	conditionalHook := func(_ context.Context, _ any) error {
		conditionalCalled = true
		return nil
	}

	registry.Register(userType, BeforeCreate, syncHook)
	registry.RegisterAsync(userType, BeforeCreate, asyncHook)
	registry.RegisterConditional(userType, BeforeCreate, condition, conditionalHook)

	// Execute all hooks
	user := &TestUser{Username: "test", Email: "test@example.com"}
	err := registry.Execute(context.Background(), user, BeforeCreate)

	assert.NoError(t, err)
	assert.True(t, syncCalled)
	assert.True(t, conditionalCalled)

	// Wait for async hook
	wg.Wait()
	assert.True(t, asyncCalled)
}

// Benchmark tests

func BenchmarkHookRegistry_Execute_SingleHook(b *testing.B) {
	registry := NewHookRegistry(zap.NewNop())
	userType := reflect.TypeOf(TestUser{})

	hook := func(_ context.Context, _ any) error {
		return nil
	}

	registry.Register(userType, BeforeCreate, hook)
	user := &TestUser{Username: "test"}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.Execute(ctx, user, BeforeCreate)
	}
}

func BenchmarkHookRegistry_Execute_MultipleHooks(b *testing.B) {
	registry := NewHookRegistry(zap.NewNop())
	userType := reflect.TypeOf(TestUser{})

	// Register 10 hooks
	for i := 0; i < 10; i++ {
		hook := func(_ context.Context, _ any) error {
			return nil
		}
		registry.Register(userType, BeforeCreate, hook)
	}

	user := &TestUser{Username: "test"}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.Execute(ctx, user, BeforeCreate)
	}
}

func BenchmarkHookStatsTracker_TrackExecution(b *testing.B) {
	tracker := NewHookStatsTracker()
	userType := reflect.TypeOf(TestUser{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.TrackExecution(userType, BeforeCreate, time.Microsecond, nil)
	}
}

// Test concurrent access

func TestHookRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewHookRegistry(zap.NewNop())
	userType := reflect.TypeOf(TestUser{})

	var wg sync.WaitGroup
	numGoroutines := 10
	hooksPerGoroutine := 10

	// Concurrent registration
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()
			for j := 0; j < hooksPerGoroutine; j++ {
				hook := func(_ context.Context, _ any) error {
					return nil
				}
				registry.Register(userType, BeforeCreate, hook)
			}
		}(i)
	}

	wg.Wait()

	// Verify all hooks were registered
	count := registry.GetRegisteredHooksCount(userType, BeforeCreate)
	assert.Equal(t, numGoroutines*hooksPerGoroutine, count)

	// Concurrent execution
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			user := &TestUser{Username: "test"}
			_ = registry.Execute(context.Background(), user, BeforeCreate)
		}()
	}

	wg.Wait()
}

func TestHookStatsTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewHookStatsTracker()
	userType := reflect.TypeOf(TestUser{})

	var wg sync.WaitGroup
	numGoroutines := 10
	executionsPerGoroutine := 100

	// Concurrent tracking
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < executionsPerGoroutine; j++ {
				tracker.TrackExecution(userType, BeforeCreate, time.Microsecond, nil)
			}
		}()
	}

	wg.Wait()

	// Verify stats
	stats := tracker.GetStats()
	key := "hooks.TestUser:before_create"
	stat := stats[key]
	assert.Equal(t, int64(numGoroutines*executionsPerGoroutine), stat.TotalExecutions)
}
