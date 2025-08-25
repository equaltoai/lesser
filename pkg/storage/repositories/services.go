package repositories

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/errors"
)

// === VALIDATION SERVICE IMPLEMENTATION ===

// DefaultValidationService provides standard validation logic
type DefaultValidationService struct {
	logger interface{} //nolint:unused // Use interface to avoid import cycles
}

// NewDefaultValidationService creates a new validation service
func NewDefaultValidationService() *DefaultValidationService {
	return &DefaultValidationService{}
}

// ValidateModel validates a model's structure and constraints
func (v *DefaultValidationService) ValidateModel(ctx context.Context, model BaseModel) error {
	if model == nil {
		return errors.ValidationFailed("model", "model is nil")
	}

	// Validate that keys can be generated
	if err := model.UpdateKeys(); err != nil {
		return errors.ValidationFailed("key_generation", err.Error())
	}

	// Validate PK and SK are not empty
	if model.GetPK() == "" {
		return errors.ValidationFailed("pk", "PK cannot be empty")
	}

	if model.GetSK() == "" {
		return errors.ValidationFailed("sk", "SK cannot be empty")
	}

	return nil
}

// ValidateBusinessRules validates business logic rules
func (v *DefaultValidationService) ValidateBusinessRules(ctx context.Context, model BaseModel, action string) error {
	// Get the underlying value and type
	val := reflect.ValueOf(model)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return errors.ValidationFailed("model_pointer", "model pointer is nil")
		}
		val = val.Elem()
	}

	typ := val.Type()

	// Check for business rule violations based on action
	switch action {
	case common.OperationCreate:
		return v.validateCreateRules(ctx, val, typ)
	case common.OperationUpdate:
		return v.validateUpdateRules(ctx, val, typ)
	case common.OperationDelete:
		return v.validateDeleteRules(ctx, val, typ)
	default:
		return nil // No specific rules for other actions
	}
}

// ValidateRequiredFields validates that required fields are present
func (v *DefaultValidationService) ValidateRequiredFields(ctx context.Context, model BaseModel) error {
	// Get the underlying value
	val := reflect.ValueOf(model)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return errors.ValidationFailed("model_pointer", "model pointer is nil")
		}
		val = val.Elem()
	}

	typ := val.Type()

	// Check each field for required validation
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Check if field has required tag
		if tag := fieldType.Tag.Get("validate"); tag != "" {
			if strings.Contains(tag, "required") {
				if v.isFieldEmpty(field) {
					return errors.ValidationFailed(
						fieldType.Name,
						fmt.Sprintf("required field '%s' is empty", fieldType.Name),
					)
				}
			}
		}

		// Check for common required patterns
		if v.isCommonRequiredField(fieldType.Name) && v.isFieldEmpty(field) {
			return errors.ValidationFailed(
				fieldType.Name,
				fmt.Sprintf("field '%s' is required", fieldType.Name),
			)
		}
	}

	return nil
}

// validateCreateRules validates rules specific to entity creation
func (v *DefaultValidationService) validateCreateRules(ctx context.Context, val reflect.Value, typ reflect.Type) error {
	// Check for timestamp fields that should be set on creation
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Check if this is a CreatedAt field that should be set
		if strings.Contains(strings.ToLower(fieldType.Name), "createdat") && field.Kind() == reflect.Struct {
			if field.Type() == reflect.TypeOf(time.Time{}) {
				if field.Interface().(time.Time).IsZero() {
					return errors.ValidationFailed(
						fieldType.Name,
						fmt.Sprintf("creation timestamp field '%s' should be set", fieldType.Name),
					)
				}
			}
		}
	}

	return nil
}

// validateUpdateRules validates rules specific to entity updates
func (v *DefaultValidationService) validateUpdateRules(ctx context.Context, val reflect.Value, typ reflect.Type) error {
	// Check for timestamp fields that should be updated
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Check if this is an UpdatedAt field that should be set
		if strings.Contains(strings.ToLower(fieldType.Name), "updatedat") && field.Kind() == reflect.Struct {
			if field.Type() == reflect.TypeOf(time.Time{}) {
				if field.Interface().(time.Time).IsZero() {
					return errors.ValidationFailed(
						fieldType.Name,
						fmt.Sprintf("update timestamp field '%s' should be set", fieldType.Name),
					)
				}
			}
		}
	}

	return nil
}

// validateDeleteRules validates rules specific to entity deletion
func (v *DefaultValidationService) validateDeleteRules(ctx context.Context, val reflect.Value, typ reflect.Type) error {
	// Default implementation - can be overridden by specific repositories
	return nil
}

// isFieldEmpty checks if a field is empty based on its type
func (v *DefaultValidationService) isFieldEmpty(field reflect.Value) bool {
	switch field.Kind() {
	case reflect.String:
		return field.String() == ""
	case reflect.Slice, reflect.Map, reflect.Array:
		return field.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return field.IsNil()
	case reflect.Struct:
		if field.Type() == reflect.TypeOf(time.Time{}) {
			return field.Interface().(time.Time).IsZero()
		}
		return false // Non-time structs are considered non-empty if they exist
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return field.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return field.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return field.Float() == 0
	case reflect.Bool:
		return !field.Bool()
	default:
		return false
	}
}

// isCommonRequiredField checks if a field name indicates it should be required
func (v *DefaultValidationService) isCommonRequiredField(fieldName string) bool {
	commonRequired := []string{
		"ID", "Id", "Username", "Email", "Name", "Title", "Type", "Status",
	}

	for _, req := range commonRequired {
		if fieldName == req {
			return true
		}
	}

	return false
}

// === PERMISSION SERVICE IMPLEMENTATION ===

// DefaultPermissionService provides standard permission checking logic
type DefaultPermissionService struct {
	logger interface{} //nolint:unused // Use interface to avoid import cycles
}

// NewDefaultPermissionService creates a new permission service
func NewDefaultPermissionService() *DefaultPermissionService {
	return &DefaultPermissionService{}
}

// CheckPermissions checks if an actor has permission to perform an action on a resource
func (p *DefaultPermissionService) CheckPermissions(ctx context.Context, actor string, action string, resource BaseModel) error {
	if actor == "" {
		return errors.ValidationFailed("authentication", "authentication required")
	}

	// Get resource information
	resourceType := p.getResourceType(resource)
	resourceID := resource.GetPK()

	// Check basic permissions based on action
	switch action {
	case "create":
		return p.checkCreatePermission(ctx, actor, resourceType)
	case "read", "get", "query":
		return p.checkReadPermission(ctx, actor, resourceType, resourceID)
	case "update":
		return p.checkUpdatePermission(ctx, actor, resourceType, resourceID)
	case "delete":
		return p.checkDeletePermission(ctx, actor, resourceType, resourceID)
	default:
		return p.checkGenericPermission(ctx, actor, action, resourceType, resourceID)
	}
}

// HasPermission checks if an actor has a specific permission
func (p *DefaultPermissionService) HasPermission(ctx context.Context, actor string, permission string) bool {
	if actor == "" {
		return false
	}

	// Note: GetActorFromContext doesn't exist yet, so we'll use the actor parameter
	actorInfo := actor

	// Check for admin permissions
	if strings.Contains(strings.ToLower(actorInfo), "admin") {
		return true // Admins have all permissions
	}

	// Check for specific permission patterns
	switch permission {
	case "admin", "superuser":
		return strings.Contains(strings.ToLower(actorInfo), "admin")
	case "moderator":
		return strings.Contains(strings.ToLower(actorInfo), "mod")
	case "create":
		return true // Most users can create
	case "read":
		return true // Most users can read
	default:
		return false
	}
}

// checkCreatePermission checks creation permissions
func (p *DefaultPermissionService) checkCreatePermission(ctx context.Context, actor string, resourceType string) error {
	// Default: most actors can create most resources
	// This can be overridden by specific permission services
	return nil
}

// checkReadPermission checks read permissions
func (p *DefaultPermissionService) checkReadPermission(ctx context.Context, actor string, resourceType string, resourceID string) error {
	// Default: most actors can read most resources
	// This can be overridden for private resources
	return nil
}

// checkUpdatePermission checks update permissions
func (p *DefaultPermissionService) checkUpdatePermission(ctx context.Context, actor string, resourceType string, resourceID string) error {
	// Default: actors can update their own resources
	if p.isOwner(actor, resourceID) {
		return nil
	}

	// Check for admin permissions
	if p.HasPermission(ctx, actor, "admin") {
		return nil
	}

	return errors.InsufficientPermissions(fmt.Sprintf("update %s %s", resourceType, resourceID))
}

// checkDeletePermission checks delete permissions
func (p *DefaultPermissionService) checkDeletePermission(ctx context.Context, actor string, resourceType string, resourceID string) error {
	// Default: actors can delete their own resources
	if p.isOwner(actor, resourceID) {
		return nil
	}

	// Check for admin permissions
	if p.HasPermission(ctx, actor, "admin") {
		return nil
	}

	return errors.InsufficientPermissions(fmt.Sprintf("delete %s %s", resourceType, resourceID))
}

// checkGenericPermission checks generic permissions
func (p *DefaultPermissionService) checkGenericPermission(ctx context.Context, actor string, action string, resourceType string, resourceID string) error {
	// Default: check if user has admin permissions
	if p.HasPermission(ctx, actor, "admin") {
		return nil
	}

	return errors.InsufficientPermissions(fmt.Sprintf("%s %s %s", action, resourceType, resourceID))
}

// getResourceType extracts the resource type from a model
func (p *DefaultPermissionService) getResourceType(resource BaseModel) string {
	typ := reflect.TypeOf(resource)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	name := typ.Name()
	// Convert CamelCase to lowercase
	return strings.ToLower(name)
}

// isOwner checks if an actor owns a resource (basic implementation)
func (p *DefaultPermissionService) isOwner(actor string, resourceID string) bool {
	// Simple check: if the resource ID contains the actor name
	// This is a basic implementation and should be enhanced for production
	return strings.Contains(strings.ToUpper(resourceID), strings.ToUpper(actor))
}

// === CACHING SERVICE IMPLEMENTATION ===

// InMemoryCachingService provides a simple in-memory cache implementation
type InMemoryCachingService struct {
	cache  map[string]cacheEntry
	logger interface{} //nolint:unused // Use interface to avoid import cycles
}

type cacheEntry struct {
	value     interface{}
	expiresAt time.Time
}

// NewInMemoryCachingService creates a new in-memory caching service
func NewInMemoryCachingService() *InMemoryCachingService {
	return &InMemoryCachingService{
		cache: make(map[string]cacheEntry),
	}
}

// Get retrieves a value from the cache
func (c *InMemoryCachingService) Get(ctx context.Context, key string, dest interface{}) error {
	entry, exists := c.cache[key]
	if !exists {
		return errors.ItemNotFound("cache_entry")
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		delete(c.cache, key)
		return errors.ItemNotFound("cache_entry")
	}

	// Copy the value to destination
	// This is a simplified implementation
	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Ptr {
		return errors.ValidationFailed("destination", "destination must be a pointer")
	}

	sourceVal := reflect.ValueOf(entry.value)
	destVal.Elem().Set(sourceVal)

	return nil
}

// Set stores a value in the cache with TTL
func (c *InMemoryCachingService) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	c.cache[key] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

// Delete removes a value from the cache
func (c *InMemoryCachingService) Delete(ctx context.Context, key string) error {
	delete(c.cache, key)
	return nil
}

// InvalidatePattern removes all cache entries matching a pattern
func (c *InMemoryCachingService) InvalidatePattern(ctx context.Context, pattern string) error {
	for key := range c.cache {
		if strings.Contains(key, pattern) {
			delete(c.cache, key)
		}
	}
	return nil
}

// === EVENT SERVICE IMPLEMENTATION ===

// DefaultEventService provides basic event emission
type DefaultEventService struct {
	handlers []EventHandler
	logger   interface{} //nolint:unused // Use interface to avoid import cycles
}

// EventHandler handles emitted events
type EventHandler interface {
	Handle(ctx context.Context, event Event) error
}

// NewDefaultEventService creates a new event service
func NewDefaultEventService() *DefaultEventService {
	return &DefaultEventService{
		handlers: make([]EventHandler, 0),
	}
}

// AddHandler adds an event handler
func (e *DefaultEventService) AddHandler(handler EventHandler) {
	e.handlers = append(e.handlers, handler)
}

// Emit emits an event synchronously
func (e *DefaultEventService) Emit(ctx context.Context, event Event) error {
	for _, handler := range e.handlers {
		if err := handler.Handle(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// EmitAsync emits an event asynchronously
func (e *DefaultEventService) EmitAsync(ctx context.Context, event Event) error {
	// For now, just emit synchronously
	// In production, this would use goroutines or a message queue
	go func() {
		_ = e.Emit(ctx, event)
	}()
	return nil
}

// LogEventHandler logs all events (useful for debugging)
type LogEventHandler struct {
	logger interface{} //nolint:unused // Reserved for future logging functionality
}

// NewLogEventHandler creates a new log event handler
func NewLogEventHandler() *LogEventHandler {
	return &LogEventHandler{}
}

// Handle logs the event
func (h *LogEventHandler) Handle(ctx context.Context, event Event) error {
	// In a real implementation, this would use proper logging
	fmt.Printf("Event: %s.%s on %s/%s by %s\n",
		event.Entity, event.Action, event.EntityID, event.Type, event.Actor)
	return nil
}
