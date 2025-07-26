package hooks

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"go.uber.org/zap"
)

// HookType represents the type of lifecycle hook
type HookType string

const (
	BeforeCreate HookType = "before_create"
	AfterCreate  HookType = "after_create"
	BeforeUpdate HookType = "before_update"
	AfterUpdate  HookType = "after_update"
	BeforeDelete HookType = "before_delete"
	AfterDelete  HookType = "after_delete"
	AfterFind    HookType = "after_find"
	BeforeSave   HookType = "before_save"
	AfterSave    HookType = "after_save"
)

// HookFunc is a function that gets executed during model lifecycle events
type HookFunc func(ctx context.Context, model any) error

// AsyncHookFunc is a function that gets executed asynchronously
type AsyncHookFunc func(ctx context.Context, model any)

// ConditionalHookFunc is a hook function with a condition
type ConditionalHookFunc struct {
	Condition func(ctx context.Context, model any) bool
	Hook      HookFunc
}

// HookRegistry manages all registered hooks
type HookRegistry struct {
	hooks       map[reflect.Type]map[HookType][]HookFunc
	asyncHooks  map[reflect.Type]map[HookType][]AsyncHookFunc
	conditional map[reflect.Type]map[HookType][]ConditionalHookFunc
	logger      *zap.Logger
	mu          sync.RWMutex
}

// NewHookRegistry creates a new hook registry
func NewHookRegistry(logger *zap.Logger) *HookRegistry {
	return &HookRegistry{
		hooks:       make(map[reflect.Type]map[HookType][]HookFunc),
		asyncHooks:  make(map[reflect.Type]map[HookType][]AsyncHookFunc),
		conditional: make(map[reflect.Type]map[HookType][]ConditionalHookFunc),
		logger:      logger,
	}
}

// Register registers a synchronous hook for a model type
func (hr *HookRegistry) Register(modelType reflect.Type, hookType HookType, fn HookFunc) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if hr.hooks[modelType] == nil {
		hr.hooks[modelType] = make(map[HookType][]HookFunc)
	}

	hr.hooks[modelType][hookType] = append(hr.hooks[modelType][hookType], fn)

	if hr.logger != nil {
		hr.logger.Debug("hook_registered",
			zap.String("model_type", modelType.String()),
			zap.String("hook_type", string(hookType)),
		)
	}
}

// RegisterAsync registers an asynchronous hook for a model type
func (hr *HookRegistry) RegisterAsync(modelType reflect.Type, hookType HookType, fn AsyncHookFunc) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if hr.asyncHooks[modelType] == nil {
		hr.asyncHooks[modelType] = make(map[HookType][]AsyncHookFunc)
	}

	hr.asyncHooks[modelType][hookType] = append(hr.asyncHooks[modelType][hookType], fn)

	if hr.logger != nil {
		hr.logger.Debug("async_hook_registered",
			zap.String("model_type", modelType.String()),
			zap.String("hook_type", string(hookType)),
		)
	}
}

// RegisterConditional registers a conditional hook for a model type
func (hr *HookRegistry) RegisterConditional(modelType reflect.Type, hookType HookType, condition func(ctx context.Context, model any) bool, fn HookFunc) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if hr.conditional[modelType] == nil {
		hr.conditional[modelType] = make(map[HookType][]ConditionalHookFunc)
	}

	conditionalHook := ConditionalHookFunc{
		Condition: condition,
		Hook:      fn,
	}

	hr.conditional[modelType][hookType] = append(hr.conditional[modelType][hookType], conditionalHook)

	if hr.logger != nil {
		hr.logger.Debug("conditional_hook_registered",
			zap.String("model_type", modelType.String()),
			zap.String("hook_type", string(hookType)),
		)
	}
}

// Execute executes all registered hooks for a model and hook type
func (hr *HookRegistry) Execute(ctx context.Context, model any, hookType HookType) error {
	modelType := reflect.TypeOf(model)

	// Remove pointer if present
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	hr.mu.RLock()
	defer hr.mu.RUnlock()

	// Execute synchronous hooks
	if err := hr.executeSyncHooks(ctx, model, modelType, hookType); err != nil {
		return err
	}

	// Execute conditional hooks
	if err := hr.executeConditionalHooks(ctx, model, modelType, hookType); err != nil {
		return err
	}

	// Execute async hooks (non-blocking)
	hr.executeAsyncHooks(ctx, model, modelType, hookType)

	return nil
}

// executeSyncHooks executes synchronous hooks
func (hr *HookRegistry) executeSyncHooks(ctx context.Context, model any, modelType reflect.Type, hookType HookType) error {
	hooks, exists := hr.hooks[modelType][hookType]
	if !exists {
		return nil
	}

	for i, hook := range hooks {
		startTime := time.Now()
		err := hook(ctx, model)
		duration := time.Since(startTime)

		if hr.logger != nil {
			fields := []zap.Field{
				zap.String("model_type", modelType.String()),
				zap.String("hook_type", string(hookType)),
				zap.Int("hook_index", i),
				zap.Duration("duration", duration),
			}

			if err != nil {
				fields = append(fields, zap.Error(err))
				hr.logger.Error("hook_execution_failed", fields...)
				return fmt.Errorf("hook %s[%d] failed: %w", hookType, i, err)
			} else {
				hr.logger.Debug("hook_executed", fields...)
			}
		}

		// Track cost if tracker is available in context
		if tracker := cost.FromContext(ctx); tracker != nil && duration > time.Millisecond {
			// Track hook execution as lambda duration
			tracker.TrackLambdaInvocation(duration.Milliseconds(), 128) // Assume 128MB for hooks
		}
	}

	return nil
}

// executeConditionalHooks executes conditional hooks
func (hr *HookRegistry) executeConditionalHooks(ctx context.Context, model any, modelType reflect.Type, hookType HookType) error {
	conditionalHooks, exists := hr.conditional[modelType][hookType]
	if !exists {
		return nil
	}

	for i, conditionalHook := range conditionalHooks {
		if conditionalHook.Condition(ctx, model) {
			startTime := time.Now()
			err := conditionalHook.Hook(ctx, model)
			duration := time.Since(startTime)

			if hr.logger != nil {
				fields := []zap.Field{
					zap.String("model_type", modelType.String()),
					zap.String("hook_type", string(hookType)),
					zap.Int("conditional_hook_index", i),
					zap.Duration("duration", duration),
				}

				if err != nil {
					fields = append(fields, zap.Error(err))
					hr.logger.Error("conditional_hook_execution_failed", fields...)
					return fmt.Errorf("conditional hook %s[%d] failed: %w", hookType, i, err)
				} else {
					hr.logger.Debug("conditional_hook_executed", fields...)
				}
			}
		}
	}

	return nil
}

// executeAsyncHooks executes asynchronous hooks
func (hr *HookRegistry) executeAsyncHooks(ctx context.Context, model any, modelType reflect.Type, hookType HookType) {
	asyncHooks, exists := hr.asyncHooks[modelType][hookType]
	if !exists {
		return
	}

	for i, asyncHook := range asyncHooks {
		go func(index int, hook AsyncHookFunc) {
			defer func() {
				if r := recover(); r != nil && hr.logger != nil {
					hr.logger.Error("async_hook_panic",
						zap.String("model_type", modelType.String()),
						zap.String("hook_type", string(hookType)),
						zap.Int("hook_index", index),
						zap.Any("panic", r),
					)
				}
			}()

			startTime := time.Now()
			hook(ctx, model)
			duration := time.Since(startTime)

			if hr.logger != nil {
				hr.logger.Debug("async_hook_completed",
					zap.String("model_type", modelType.String()),
					zap.String("hook_type", string(hookType)),
					zap.Int("hook_index", index),
					zap.Duration("duration", duration),
				)
			}
		}(i, asyncHook)
	}
}

// GetRegisteredHooksCount returns the number of registered hooks for a model type
func (hr *HookRegistry) GetRegisteredHooksCount(modelType reflect.Type, hookType HookType) int {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	count := 0
	if hooks, exists := hr.hooks[modelType][hookType]; exists {
		count += len(hooks)
	}
	if asyncHooks, exists := hr.asyncHooks[modelType][hookType]; exists {
		count += len(asyncHooks)
	}
	if conditionalHooks, exists := hr.conditional[modelType][hookType]; exists {
		count += len(conditionalHooks)
	}

	return count
}

// Clear removes all hooks for a model type
func (hr *HookRegistry) Clear(modelType reflect.Type) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	delete(hr.hooks, modelType)
	delete(hr.asyncHooks, modelType)
	delete(hr.conditional, modelType)

	if hr.logger != nil {
		hr.logger.Debug("hooks_cleared",
			zap.String("model_type", modelType.String()),
		)
	}
}

// ClearAll removes all registered hooks
func (hr *HookRegistry) ClearAll() {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	hr.hooks = make(map[reflect.Type]map[HookType][]HookFunc)
	hr.asyncHooks = make(map[reflect.Type]map[HookType][]AsyncHookFunc)
	hr.conditional = make(map[reflect.Type]map[HookType][]ConditionalHookFunc)

	if hr.logger != nil {
		hr.logger.Debug("all_hooks_cleared")
	}
}

// Global hook registry instance
var (
	globalRegistry     *HookRegistry
	globalRegistryOnce sync.Once
)

// GetGlobalRegistry returns the global hook registry instance
func GetGlobalRegistry() *HookRegistry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewHookRegistry(zap.NewNop())
	})
	return globalRegistry
}

// SetGlobalRegistry sets the global hook registry instance
func SetGlobalRegistry(registry *HookRegistry) {
	globalRegistry = registry
}

// Convenience functions for global registry

// Register registers a hook with the global registry
func Register(modelType reflect.Type, hookType HookType, fn HookFunc) {
	GetGlobalRegistry().Register(modelType, hookType, fn)
}

// RegisterAsync registers an async hook with the global registry
func RegisterAsync(modelType reflect.Type, hookType HookType, fn AsyncHookFunc) {
	GetGlobalRegistry().RegisterAsync(modelType, hookType, fn)
}

// RegisterConditional registers a conditional hook with the global registry
func RegisterConditional(modelType reflect.Type, hookType HookType, condition func(ctx context.Context, model any) bool, fn HookFunc) {
	GetGlobalRegistry().RegisterConditional(modelType, hookType, condition, fn)
}

// Execute executes hooks with the global registry
func Execute(ctx context.Context, model any, hookType HookType) error {
	return GetGlobalRegistry().Execute(ctx, model, hookType)
}

// Pre-defined common hooks

// AuditHook creates an audit trail entry
func AuditHook(ctx context.Context, model any) error {
	// Extract audit information
	auditData := map[string]any{
		"model_type": reflect.TypeOf(model).String(),
		"timestamp":  time.Now(),
	}

	// Get user ID from context if available
	if userID := getUserIDFromContext(ctx); userID != "" {
		auditData["user_id"] = userID
	}

	// Get request ID from context if available
	if requestID := getRequestIDFromContext(ctx); requestID != "" {
		auditData["request_id"] = requestID
	}

	// In a real implementation, this would write to an audit log table
	// For now, we'll log it
	if logger := getLoggerFromContext(ctx); logger != nil {
		logger.Info("audit_trail", zap.Any("audit_data", auditData))
	}

	return nil
}

// ValidationHook validates the model using its Validate method if available
func ValidationHook(ctx context.Context, model any) error {
	// Check if model implements Validator interface
	if validator, ok := model.(Validator); ok {
		return validator.Validate()
	}
	return nil
}

// TimestampHook updates timestamps on models
func TimestampHook(ctx context.Context, model any) error {
	now := time.Now()
	modelValue := reflect.ValueOf(model)

	// Handle pointer
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}

	if modelValue.Kind() != reflect.Struct {
		return nil
	}

	// Update UpdatedAt field if it exists
	if field := modelValue.FieldByName("UpdatedAt"); field.IsValid() && field.CanSet() {
		if field.Type() == reflect.TypeOf(now) {
			field.Set(reflect.ValueOf(now))
		}
	}

	return nil
}

// NotificationHook creates notifications based on model changes
func NotificationHook(ctx context.Context, model any) error {
	// This would integrate with a notification system
	// For now, we'll create placeholder notifications

	switch m := model.(type) {
	case FollowModel:
		return createFollowNotification(ctx, m)
	case StatusModel:
		return createStatusNotifications(ctx, m)
	case MentionModel:
		return createMentionNotification(ctx, m)
	}

	return nil
}

// CacheInvalidationHook invalidates relevant caches
func CacheInvalidationHook(ctx context.Context, model any) error {
	// This would integrate with a caching system
	// For now, we'll log the cache invalidation

	if logger := getLoggerFromContext(ctx); logger != nil {
		logger.Debug("cache_invalidation_triggered",
			zap.String("model_type", reflect.TypeOf(model).String()),
		)
	}

	return nil
}

// SearchIndexHook updates search indexes
func SearchIndexHook(ctx context.Context, model any) error {
	// This would integrate with a search system like Elasticsearch
	// For now, we'll log the search index update

	if logger := getLoggerFromContext(ctx); logger != nil {
		logger.Debug("search_index_update_triggered",
			zap.String("model_type", reflect.TypeOf(model).String()),
		)
	}

	return nil
}

// Interfaces and types for hooks

// Validator interface for models that can be validated
type Validator interface {
	Validate() error
}

// FollowModel interface for follow-related models
type FollowModel interface {
	GetFollowerID() string
	GetFolloweeID() string
}

// StatusModel interface for status-related models
type StatusModel interface {
	GetUserID() string
	GetContent() string
	GetMentions() []string
}

// MentionModel interface for mention-related models
type MentionModel interface {
	GetUserID() string
	GetMentionedUserID() string
	GetStatusID() string
}

// Helper functions to extract context values

func getUserIDFromContext(ctx context.Context) string {
	if userID, ok := ctx.Value("user_id").(string); ok {
		return userID
	}
	return ""
}

func getRequestIDFromContext(ctx context.Context) string {
	if requestID, ok := ctx.Value("request_id").(string); ok {
		return requestID
	}
	return ""
}

func getLoggerFromContext(ctx context.Context) *zap.Logger {
	if logger, ok := ctx.Value("logger").(*zap.Logger); ok {
		return logger
	}
	return nil
}

// Notification creation functions (placeholder implementations)

func createFollowNotification(ctx context.Context, follow FollowModel) error {
	// Create follow notification
	if logger := getLoggerFromContext(ctx); logger != nil {
		logger.Debug("follow_notification_created",
			zap.String("follower_id", follow.GetFollowerID()),
			zap.String("followee_id", follow.GetFolloweeID()),
		)
	}
	return nil
}

func createStatusNotifications(ctx context.Context, status StatusModel) error {
	// Create notifications for mentions in status
	mentions := status.GetMentions()
	if len(mentions) > 0 && getLoggerFromContext(ctx) != nil {
		getLoggerFromContext(ctx).Debug("mention_notifications_created",
			zap.String("status_user_id", status.GetUserID()),
			zap.Strings("mentioned_users", mentions),
		)
	}
	return nil
}

func createMentionNotification(ctx context.Context, mention MentionModel) error {
	// Create mention notification
	if logger := getLoggerFromContext(ctx); logger != nil {
		logger.Debug("mention_notification_created",
			zap.String("mentioned_user_id", mention.GetMentionedUserID()),
			zap.String("status_id", mention.GetStatusID()),
		)
	}
	return nil
}

// Hook execution statistics

// HookStats tracks execution statistics for hooks
type HookStats struct {
	TotalExecutions int64
	TotalDuration   time.Duration
	ErrorCount      int64
	AverageDuration time.Duration
	LastExecution   time.Time
}

// HookStatsTracker tracks statistics for hook executions
type HookStatsTracker struct {
	stats map[string]*HookStats
	mu    sync.RWMutex
}

// NewHookStatsTracker creates a new hook statistics tracker
func NewHookStatsTracker() *HookStatsTracker {
	return &HookStatsTracker{
		stats: make(map[string]*HookStats),
	}
}

// TrackExecution tracks the execution of a hook
func (hst *HookStatsTracker) TrackExecution(modelType reflect.Type, hookType HookType, duration time.Duration, err error) {
	key := fmt.Sprintf("%s:%s", modelType.String(), string(hookType))

	hst.mu.Lock()
	defer hst.mu.Unlock()

	stats, exists := hst.stats[key]
	if !exists {
		stats = &HookStats{}
		hst.stats[key] = stats
	}

	stats.TotalExecutions++
	stats.TotalDuration += duration
	stats.AverageDuration = stats.TotalDuration / time.Duration(stats.TotalExecutions)
	stats.LastExecution = time.Now()

	if err != nil {
		stats.ErrorCount++
	}
}

// GetStats returns statistics for all hooks
func (hst *HookStatsTracker) GetStats() map[string]*HookStats {
	hst.mu.RLock()
	defer hst.mu.RUnlock()

	result := make(map[string]*HookStats)
	for key, stats := range hst.stats {
		result[key] = &HookStats{
			TotalExecutions: stats.TotalExecutions,
			TotalDuration:   stats.TotalDuration,
			ErrorCount:      stats.ErrorCount,
			AverageDuration: stats.AverageDuration,
			LastExecution:   stats.LastExecution,
		}
	}

	return result
}

// Reset clears all statistics
func (hst *HookStatsTracker) Reset() {
	hst.mu.Lock()
	defer hst.mu.Unlock()
	hst.stats = make(map[string]*HookStats)
}

// Global stats tracker
var (
	globalStatsTracker     *HookStatsTracker
	globalStatsTrackerOnce sync.Once
)

// GetGlobalStatsTracker returns the global hook statistics tracker
func GetGlobalStatsTracker() *HookStatsTracker {
	globalStatsTrackerOnce.Do(func() {
		globalStatsTracker = NewHookStatsTracker()
	})
	return globalStatsTracker
}
