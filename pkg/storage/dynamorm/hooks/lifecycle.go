// Package hooks provides lifecycle hook management for DynamORM model operations with cost tracking integration.
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
	// BeforeCreate represents a before create hook
	BeforeCreate HookType = "before_create"
	// AfterCreate represents an after create hook
	AfterCreate HookType = "after_create"
	// BeforeUpdate represents a before update hook
	BeforeUpdate HookType = "before_update"
	// AfterUpdate represents an after update hook
	AfterUpdate HookType = "after_update"
	// BeforeDelete represents a before delete hook
	BeforeDelete HookType = "before_delete"
	// AfterDelete represents an after delete hook
	AfterDelete HookType = "after_delete"
	// AfterFind represents an after find hook
	AfterFind HookType = "after_find"
	// BeforeSave represents a before save hook
	BeforeSave HookType = "before_save"
	// AfterSave represents an after save hook
	AfterSave HookType = "after_save"
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
			}
			hr.logger.Debug("hook_executed", fields...)
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
				}
				hr.logger.Debug("conditional_hook_executed", fields...)
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
func ValidationHook(_ context.Context, model any) error {
	// Check if model implements Validator interface
	if validator, ok := model.(Validator); ok {
		return validator.Validate()
	}
	return nil
}

// TimestampHook updates timestamps on models
func TimestampHook(_ context.Context, model any) error {
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
	// Extract notification repository from context if available
	notificationRepo := getNotificationRepository(ctx)
	if notificationRepo == nil {
		// No notification repository available, skip notification creation
		if logger := getLoggerFromContext(ctx); logger != nil {
			logger.Debug("notification_repository_not_available",
				zap.String("model_type", reflect.TypeOf(model).String()))
		}
		return nil
	}

	switch m := model.(type) {
	case FollowModel:
		return createFollowNotification(ctx, m, notificationRepo)
	case StatusModel:
		return createStatusNotifications(ctx, m, notificationRepo)
	case MentionModel:
		return createMentionNotification(ctx, m, notificationRepo)
	case ReblogModel:
		return createReblogNotification(ctx, m, notificationRepo)
	case FavoriteModel:
		return createFavoriteNotification(ctx, m, notificationRepo)
	case PollModel:
		return createPollNotification(ctx, m, notificationRepo)
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

// ReblogModel interface for reblog-related models
type ReblogModel interface {
	GetUserID() string
	GetStatusID() string
	GetOriginalAuthorID() string
}

// FavoriteModel interface for favorite-related models
type FavoriteModel interface {
	GetUserID() string
	GetStatusID() string
	GetStatusAuthorID() string
}

// PollModel interface for poll-related models
type PollModel interface {
	GetPollID() string
	GetAuthorID() string
	GetVoterID() string
	HasEnded() bool
}

// NotificationRepository interface for notification operations
type NotificationRepository interface {
	CreateNotification(ctx context.Context, notification any) error
	GetUserPushSubscriptions(ctx context.Context, username string) ([]any, error)
	SendPushNotification(ctx context.Context, username string, notification any) error
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

func getNotificationRepository(ctx context.Context) NotificationRepository {
	if repo, ok := ctx.Value("notification_repository").(NotificationRepository); ok {
		return repo
	}
	return nil
}

// Notification creation functions

func createFollowNotification(ctx context.Context, follow FollowModel, repo NotificationRepository) error {
	notification := map[string]any{
		"type":       "follow",
		"user_id":    follow.GetFolloweeID(),
		"from_user":  follow.GetFollowerID(),
		"created_at": time.Now(),
	}

	// Create notification record
	if err := repo.CreateNotification(ctx, notification); err != nil {
		if logger := getLoggerFromContext(ctx); logger != nil {
			logger.Error("failed_to_create_follow_notification",
				zap.Error(err),
				zap.String("follower_id", follow.GetFollowerID()),
				zap.String("followee_id", follow.GetFolloweeID()),
			)
		}
		return err
	}

	// Send push notification asynchronously
	go func() {
		if err := repo.SendPushNotification(ctx, follow.GetFolloweeID(), notification); err != nil {
			if logger := getLoggerFromContext(ctx); logger != nil {
				logger.Warn("failed_to_send_push_notification",
					zap.Error(err),
					zap.String("type", "follow"),
					zap.String("user_id", follow.GetFolloweeID()),
				)
			}
		}
	}()

	return nil
}

func createStatusNotifications(ctx context.Context, status StatusModel, repo NotificationRepository) error {
	// Create notifications for mentions in status
	mentions := status.GetMentions()
	for _, mentionedUser := range mentions {
		notification := map[string]any{
			"type":       "mention",
			"user_id":    mentionedUser,
			"from_user":  status.GetUserID(),
			"status_id":  status.GetUserID(), // Assuming GetUserID returns status ID
			"created_at": time.Now(),
		}

		if err := repo.CreateNotification(ctx, notification); err != nil {
			if logger := getLoggerFromContext(ctx); logger != nil {
				logger.Error("failed_to_create_mention_notification",
					zap.Error(err),
					zap.String("mentioned_user", mentionedUser),
					zap.String("status_user_id", status.GetUserID()),
				)
			}
			// Continue with other mentions even if one fails
			continue
		}

		// Send push notification asynchronously
		go func(user string, notif map[string]any) {
			if err := repo.SendPushNotification(ctx, user, notif); err != nil {
				if logger := getLoggerFromContext(ctx); logger != nil {
					logger.Warn("failed_to_send_push_notification",
						zap.Error(err),
						zap.String("type", "mention"),
						zap.String("user_id", user),
					)
				}
			}
		}(mentionedUser, notification)
	}

	return nil
}

func createMentionNotification(ctx context.Context, mention MentionModel, repo NotificationRepository) error {
	notification := map[string]any{
		"type":       "mention",
		"user_id":    mention.GetMentionedUserID(),
		"from_user":  mention.GetUserID(),
		"status_id":  mention.GetStatusID(),
		"created_at": time.Now(),
	}

	if err := repo.CreateNotification(ctx, notification); err != nil {
		if logger := getLoggerFromContext(ctx); logger != nil {
			logger.Error("failed_to_create_mention_notification",
				zap.Error(err),
				zap.String("mentioned_user_id", mention.GetMentionedUserID()),
				zap.String("status_id", mention.GetStatusID()),
			)
		}
		return err
	}

	// Send push notification asynchronously
	go func() {
		if err := repo.SendPushNotification(ctx, mention.GetMentionedUserID(), notification); err != nil {
			if logger := getLoggerFromContext(ctx); logger != nil {
				logger.Warn("failed_to_send_push_notification",
					zap.Error(err),
					zap.String("type", "mention"),
					zap.String("user_id", mention.GetMentionedUserID()),
				)
			}
		}
	}()

	return nil
}

func createReblogNotification(ctx context.Context, reblog ReblogModel, repo NotificationRepository) error {
	notification := map[string]any{
		"type":       "reblog",
		"user_id":    reblog.GetOriginalAuthorID(),
		"from_user":  reblog.GetUserID(),
		"status_id":  reblog.GetStatusID(),
		"created_at": time.Now(),
	}

	if err := repo.CreateNotification(ctx, notification); err != nil {
		if logger := getLoggerFromContext(ctx); logger != nil {
			logger.Error("failed_to_create_reblog_notification",
				zap.Error(err),
				zap.String("reblogger_id", reblog.GetUserID()),
				zap.String("original_author_id", reblog.GetOriginalAuthorID()),
			)
		}
		return err
	}

	// Send push notification asynchronously
	go func() {
		if err := repo.SendPushNotification(ctx, reblog.GetOriginalAuthorID(), notification); err != nil {
			if logger := getLoggerFromContext(ctx); logger != nil {
				logger.Warn("failed_to_send_push_notification",
					zap.Error(err),
					zap.String("type", "reblog"),
					zap.String("user_id", reblog.GetOriginalAuthorID()),
				)
			}
		}
	}()

	return nil
}

func createFavoriteNotification(ctx context.Context, favorite FavoriteModel, repo NotificationRepository) error {
	notification := map[string]any{
		"type":       "favourite",
		"user_id":    favorite.GetStatusAuthorID(),
		"from_user":  favorite.GetUserID(),
		"status_id":  favorite.GetStatusID(),
		"created_at": time.Now(),
	}

	if err := repo.CreateNotification(ctx, notification); err != nil {
		if logger := getLoggerFromContext(ctx); logger != nil {
			logger.Error("failed_to_create_favorite_notification",
				zap.Error(err),
				zap.String("favoriter_id", favorite.GetUserID()),
				zap.String("status_author_id", favorite.GetStatusAuthorID()),
			)
		}
		return err
	}

	// Send push notification asynchronously
	go func() {
		if err := repo.SendPushNotification(ctx, favorite.GetStatusAuthorID(), notification); err != nil {
			if logger := getLoggerFromContext(ctx); logger != nil {
				logger.Warn("failed_to_send_push_notification",
					zap.Error(err),
					zap.String("type", "favourite"),
					zap.String("user_id", favorite.GetStatusAuthorID()),
				)
			}
		}
	}()

	return nil
}

func createPollNotification(ctx context.Context, poll PollModel, repo NotificationRepository) error {
	// Only send notification if poll has ended
	if !poll.HasEnded() {
		return nil
	}

	notification := map[string]any{
		"type":       "poll",
		"user_id":    poll.GetAuthorID(),
		"poll_id":    poll.GetPollID(),
		"created_at": time.Now(),
	}

	if err := repo.CreateNotification(ctx, notification); err != nil {
		if logger := getLoggerFromContext(ctx); logger != nil {
			logger.Error("failed_to_create_poll_notification",
				zap.Error(err),
				zap.String("poll_id", poll.GetPollID()),
				zap.String("author_id", poll.GetAuthorID()),
			)
		}
		return err
	}

	// Send push notification asynchronously
	go func() {
		if err := repo.SendPushNotification(ctx, poll.GetAuthorID(), notification); err != nil {
			if logger := getLoggerFromContext(ctx); logger != nil {
				logger.Warn("failed_to_send_push_notification",
					zap.Error(err),
					zap.String("type", "poll"),
					zap.String("user_id", poll.GetAuthorID()),
				)
			}
		}
	}()

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
