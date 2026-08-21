package repositories

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/errors"
)

// ValidationService provides standardized validation across repositories
type ValidationService interface {
	ValidateModel(ctx context.Context, model BaseModel) error
	ValidateBusinessRules(ctx context.Context, model BaseModel, action string) error
	ValidateRequiredFields(ctx context.Context, model BaseModel) error
}

// PermissionService provides standardized permission checking
type PermissionService interface {
	CheckPermissions(ctx context.Context, actor string, action string, resource BaseModel) error
	HasPermission(ctx context.Context, actor string, permission string) bool
}

// CachingService provides repository-level caching
type CachingService interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	InvalidatePattern(ctx context.Context, pattern string) error
}

// EventService provides standardized event emission
type EventService interface {
	Emit(ctx context.Context, event Event) error
}

// Event represents a repository event
type Event struct {
	Type      string      `json:"type"`
	Entity    string      `json:"entity"`
	EntityID  string      `json:"entity_id"`
	Action    string      `json:"action"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
	Actor     string      `json:"actor,omitempty"`
}

// NewEvent creates a new repository event
func NewEvent(eventType, entity, entityID, action string, data interface{}) Event {
	return Event{
		Type:      eventType,
		Entity:    entity,
		EntityID:  entityID,
		Action:    action,
		Data:      data,
		Timestamp: time.Now(),
	}
}

// NewCreatedEvent creates an event for entity creation
func NewCreatedEvent(model BaseModel) Event {
	return NewEvent("entity.created", "base", model.GetPK(), "create", model)
}

// NewUpdatedEvent creates an event for entity updates
func NewUpdatedEvent(model BaseModel) Event {
	return NewEvent("entity.updated", "base", model.GetPK(), "update", model)
}

// NewDeletedEvent creates an event for entity deletion
func NewDeletedEvent(pk, sk string) Event {
	return NewEvent("entity.deleted", "base", pk, "delete", map[string]string{"pk": pk, "sk": sk})
}

// EnhancedBaseRepository provides comprehensive CRUD + business logic patterns
type EnhancedBaseRepository[T BaseModel] struct {
	*BaseRepository[T] // Embed existing functionality

	// Enhanced services
	validator   ValidationService
	permissions PermissionService
	caching     CachingService
	events      EventService

	// Configuration
	entityName string // Friendly name for this repository's entities
}

// NewEnhancedBaseRepository creates a new enhanced base repository
func NewEnhancedBaseRepository[T BaseModel](
	db core.DB,
	tableName string,
	logger *zap.Logger,
	costService *cost.TrackingService,
	repoName string,
	entityName string,
) *EnhancedBaseRepository[T] {
	baseRepo := NewBaseRepositoryWithCostTracking[T](db, tableName, logger, costService, repoName)

	return &EnhancedBaseRepository[T]{
		BaseRepository: baseRepo,
		entityName:     entityName,
	}
}

// SetValidationService sets the validation service
func (r *EnhancedBaseRepository[T]) SetValidationService(validator ValidationService) {
	r.validator = validator
}

// SetPermissionService sets the permission service
func (r *EnhancedBaseRepository[T]) SetPermissionService(permissions PermissionService) {
	r.permissions = permissions
}

// SetCachingService sets the caching service
func (r *EnhancedBaseRepository[T]) SetCachingService(caching CachingService) {
	r.caching = caching
}

// SetEventService sets the event service
func (r *EnhancedBaseRepository[T]) SetEventService(events EventService) {
	r.events = events
}

// === ENHANCED CRUD OPERATIONS ===

// ValidateAndCreate performs validation and creates with event emission
func (r *EnhancedBaseRepository[T]) ValidateAndCreate(ctx context.Context, model T) error {
	// 1. Validate required fields
	if r.validator != nil {
		if err := r.validator.ValidateRequiredFields(ctx, model); err != nil {
			return errors.ValidationFailed("required fields", err.Error())
		}

		// 2. Validate business rules
		if err := r.validator.ValidateBusinessRules(ctx, model, "create"); err != nil {
			return errors.ValidationFailed("business rules", err.Error())
		}
	}

	// 3. Check permissions
	if err := r.checkCreatePermissions(ctx, model); err != nil {
		return err
	}

	// 4. Execute create with cost tracking
	if err := r.Create(ctx, model); err != nil {
		return err
	}

	// 5. Emit creation event
	if r.events != nil {
		event := NewEvent("entity.created", r.entityName, model.GetPK(), "create", model)
		event.Actor = r.getActorFromContext(ctx)
		_ = r.events.Emit(ctx, event)
	}

	return nil
}

// ValidateAndCreateOrUpdate preserves the existing create validation,
// permission, and event pipeline while opting into TableTheory's explicit
// overwrite semantics. It is reserved for deterministic rows whose contract is
// to replace an earlier value; true creates must use ValidateAndCreate.
func (r *EnhancedBaseRepository[T]) ValidateAndCreateOrUpdate(ctx context.Context, model T) error {
	if r.validator != nil {
		if err := r.validator.ValidateRequiredFields(ctx, model); err != nil {
			return errors.ValidationFailed("required fields", err.Error())
		}
		if err := r.validator.ValidateBusinessRules(ctx, model, "create"); err != nil {
			return errors.ValidationFailed("business rules", err.Error())
		}
	}

	if err := r.checkCreatePermissions(ctx, model); err != nil {
		return err
	}

	if err := r.CreateOrUpdate(ctx, model); err != nil {
		return err
	}

	// Keep the pre-v3.0.5 event contract: these paths historically ran through
	// ValidateAndCreate even though the underlying Put was an upsert.
	if r.events != nil {
		event := NewEvent("entity.created", r.entityName, model.GetPK(), "create", model)
		event.Actor = r.getActorFromContext(ctx)
		_ = r.events.Emit(ctx, event)
	}

	return nil
}

// ValidateAndUpdate performs validation and updates with event emission
func (r *EnhancedBaseRepository[T]) ValidateAndUpdate(ctx context.Context, model T) error {
	// 1. Validate model
	if r.validator != nil {
		if err := r.validator.ValidateModel(ctx, model); err != nil {
			return errors.ValidationFailed("model validation", err.Error())
		}

		// 2. Validate business rules
		if err := r.validator.ValidateBusinessRules(ctx, model, "update"); err != nil {
			return errors.ValidationFailed("business rules", err.Error())
		}
	}

	// 3. Check permissions
	if err := r.checkUpdatePermissions(ctx, model); err != nil {
		return err
	}

	// 4. Execute update
	if err := r.Update(ctx, model); err != nil {
		return err
	}

	// 5. Emit update event
	if r.events != nil {
		event := NewEvent("entity.updated", r.entityName, model.GetPK(), "update", model)
		event.Actor = r.getActorFromContext(ctx)
		_ = r.events.Emit(ctx, event)
	}

	// 6. Invalidate cache if caching is enabled
	if r.caching != nil {
		cacheKey := r.generateCacheKey("entity", model.GetPK(), model.GetSK())
		_ = r.caching.Delete(ctx, cacheKey)
	}

	return nil
}

// ValidateAndDelete performs permission checks and deletes with event emission
func (r *EnhancedBaseRepository[T]) ValidateAndDelete(ctx context.Context, pk, sk string) error {
	// 1. Check permissions (get entity first if needed)
	if r.permissions != nil {
		// Create a pointer to T for DynamORM - Get needs a pointer to populate
		// Use reflection to get the type of T
		var zeroValue T
		var modelPtr T

		// Get the type of T
		tType := reflect.TypeOf(zeroValue)

		// Create a pointer instance
		if tType.Kind() == reflect.Ptr {
			// T is already a pointer type (e.g., *models.User)
			// Create a new instance of the element type
			elemType := tType.Elem()
			ptrValue := reflect.New(elemType)
			modelPtr = ptrValue.Interface().(T)
		} else {
			// T is a value type (e.g., models.User)
			// Create a pointer to it
			ptrValue := reflect.New(tType)
			modelPtr = ptrValue.Interface().(T)
		}

		if err := r.Get(ctx, pk, sk, modelPtr); err != nil {
			// If item doesn't exist, skip permission check (item may already be deleted)
			errStr := err.Error()
			if !strings.Contains(errStr, "not found") && !strings.Contains(errStr, "NotFound") {
				return err
			}
		} else if err := r.checkDeletePermissions(ctx, modelPtr); err != nil {
			// Use the model directly for permission check (it's already populated)
			return err
		}
	}

	// 2. Execute delete
	if err := r.Delete(ctx, pk, sk); err != nil {
		return err
	}

	// 3. Emit delete event
	if r.events != nil {
		event := NewEvent("entity.deleted", r.entityName, pk, "delete", map[string]string{"pk": pk, "sk": sk})
		event.Actor = r.getActorFromContext(ctx)
		_ = r.events.Emit(ctx, event)
	}

	// 4. Invalidate cache if caching is enabled
	if r.caching != nil {
		cacheKey := r.generateCacheKey("entity", pk, sk)
		_ = r.caching.Delete(ctx, cacheKey)
	}

	return nil
}

// === ENHANCED QUERY OPERATIONS ===

// FindWithCache retrieves entities with caching support
func (r *EnhancedBaseRepository[T]) FindWithCache(ctx context.Context, pk string, cacheTTL time.Duration) ([]T, error) {
	cacheKey := r.generateCacheKey("find", pk, "")

	// Try cache first if caching is enabled
	if r.caching != nil {
		var cached []T
		if err := r.caching.Get(ctx, cacheKey, &cached); err == nil {
			return cached, nil
		}
	}

	// Execute query
	results, err := r.FindByPK(ctx, pk)
	if err != nil {
		return nil, err
	}

	// Cache results if caching is enabled
	if r.caching != nil && len(results) > 0 {
		if cacheTTL == 0 {
			cacheTTL = time.Hour // Default 1 hour TTL
		}
		_ = r.caching.Set(ctx, cacheKey, results, cacheTTL)
	}

	return results, nil
}

// GetWithCache retrieves a single entity with caching support
func (r *EnhancedBaseRepository[T]) GetWithCache(ctx context.Context, pk, sk string, result T, cacheTTL time.Duration) error {
	cacheKey := r.generateCacheKey("get", pk, sk)

	// Try cache first if caching is enabled
	if r.caching != nil {
		if err := r.caching.Get(ctx, cacheKey, result); err == nil {
			return nil
		}
	}

	// Execute get
	if err := r.Get(ctx, pk, sk, result); err != nil {
		return err
	}

	// Cache result if caching is enabled
	if r.caching != nil {
		if cacheTTL == 0 {
			cacheTTL = time.Hour // Default 1 hour TTL
		}
		_ = r.caching.Set(ctx, cacheKey, result, cacheTTL)
	}

	return nil
}

// FindByMultipleFields performs queries with multiple field filters
func (r *EnhancedBaseRepository[T]) FindByMultipleFields(ctx context.Context, filters map[string]interface{}) ([]T, error) {
	return r.QueryWithFilter(ctx, "", filters, 0) // Use base implementation
}

// CountWhere counts entities matching specific conditions
func (r *EnhancedBaseRepository[T]) CountWhere(ctx context.Context, conditions map[string]interface{}) (int64, error) {
	// This would need to be implemented based on specific conditions
	// For now, return basic count
	if pk, ok := conditions["PK"].(string); ok {
		count, err := r.Count(ctx, pk)
		return int64(count), err
	}
	return 0, errors.ValidationFailed("count conditions", "no valid PK condition provided")
}

// === PERMISSION CHECKING ===

func (r *EnhancedBaseRepository[T]) checkCreatePermissions(ctx context.Context, model T) error {
	if r.permissions == nil {
		return nil // No permission service configured
	}

	actor := r.getActorFromContext(ctx)
	if actor == "" {
		// No authentication context available, skip permission check
		return nil
	}

	return r.permissions.CheckPermissions(ctx, actor, "create", model)
}

func (r *EnhancedBaseRepository[T]) checkUpdatePermissions(ctx context.Context, model T) error {
	if r.permissions == nil {
		return nil
	}

	actor := r.getActorFromContext(ctx)
	if actor == "" {
		// No authentication context available, skip permission check
		return nil
	}

	return r.permissions.CheckPermissions(ctx, actor, "update", model)
}

func (r *EnhancedBaseRepository[T]) checkDeletePermissions(ctx context.Context, model T) error {
	if r.permissions == nil {
		return nil
	}

	actor := r.getActorFromContext(ctx)
	if actor == "" {
		// No authentication context available, skip permission check
		return nil
	}

	return r.permissions.CheckPermissions(ctx, actor, "delete", model)
}

// === CACHE KEY GENERATION ===

func (r *EnhancedBaseRepository[T]) generateCacheKey(operation, pk, sk string) string {
	if sk == "" {
		return fmt.Sprintf("%s:%s:%s", r.entityName, operation, pk)
	}
	return fmt.Sprintf("%s:%s:%s:%s", r.entityName, operation, pk, sk)
}

// === BATCH OPERATIONS WITH VALIDATION ===

// ValidateAndBatchCreate creates multiple items with validation
func (r *EnhancedBaseRepository[T]) ValidateAndBatchCreate(ctx context.Context, items []T) error {
	if len(items) == 0 {
		return nil
	}

	// Validate all items first
	for _, item := range items {
		if r.validator != nil {
			if err := r.validator.ValidateRequiredFields(ctx, item); err != nil {
				return errors.ValidationFailed("batch required fields", err.Error())
			}

			if err := r.validator.ValidateBusinessRules(ctx, item, "create"); err != nil {
				return errors.ValidationFailed("batch business rules", err.Error())
			}
		}

		if err := r.checkCreatePermissions(ctx, item); err != nil {
			return err
		}
	}

	// Create batch
	if err := r.BatchCreate(ctx, items); err != nil {
		return err
	}

	// Emit events for all created items
	if r.events != nil {
		actor := r.getActorFromContext(ctx)
		for _, item := range items {
			event := NewEvent("entity.created", r.entityName, item.GetPK(), "batch_create", item)
			event.Actor = actor
			_ = r.events.Emit(ctx, event)
		}
	}

	return nil
}

// === UTILITY METHODS ===

// GetEntityName returns the friendly name for this repository's entities
func (r *EnhancedBaseRepository[T]) GetEntityName() string {
	return r.entityName
}

// HasValidation returns true if validation service is configured
func (r *EnhancedBaseRepository[T]) HasValidation() bool {
	return r.validator != nil
}

// HasPermissions returns true if permission service is configured
func (r *EnhancedBaseRepository[T]) HasPermissions() bool {
	return r.permissions != nil
}

// HasCaching returns true if caching service is configured
func (r *EnhancedBaseRepository[T]) HasCaching() bool {
	return r.caching != nil
}

// HasEvents returns true if event service is configured
func (r *EnhancedBaseRepository[T]) HasEvents() bool {
	return r.events != nil
}

// getActorFromContext extracts the actor (username) from the request context
// It looks for JWT claims in the context that contain the authenticated user's username
func (r *EnhancedBaseRepository[T]) getActorFromContext(ctx context.Context) string {
	// Try to get claims from context using common package's context key
	if claims, ok := ctx.Value(common.ContextKeyClaims).(common.Claims); ok && claims != nil {
		// Return the username from the claims if available
		username := claims.GetUsername()
		if username != "" {
			return username
		}
	}

	// If no claims or username found, return empty string
	return ""
}
