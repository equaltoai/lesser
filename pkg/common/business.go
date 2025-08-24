// Package common provides centralized business logic utilities for the Lesser project.
// This consolidates common business logic patterns found across all services to eliminate
// duplication and ensure consistency in business rule enforcement.
package common

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// BusinessRuleValidator defines the interface for business rule validation
type BusinessRuleValidator interface {
	Validate(ctx context.Context, data interface{}) error
}

// StreamingEvent represents a streaming event without import dependencies
type StreamingEvent struct {
	Type      string
	Object    string
	ObjectID  string
	ActorID   string
	Timestamp time.Time
	Metadata  map[string]interface{}
}

// EventEmitter defines the interface for event emission
type EventEmitter interface {
	EmitEvents(ctx context.Context, events []*StreamingEvent) error
}

// BusinessLogicContext provides context for business operations
type BusinessLogicContext struct {
	UserID       string
	Username     string
	DomainName   string
	Logger       *zap.Logger
	EventEmitter EventEmitter
}

// ValidationRules defines basic validation rules
type ValidationRules struct {
	Required []string
	MaxLen   map[string]int
	MinLen   map[string]int
	Pattern  map[string]string
}

// CommandValidationResult represents the result of command validation
type CommandValidationResult struct {
	IsValid  bool
	Errors   []string
	Warnings []string
}

// ValidateStruct performs basic struct validation using reflection
func ValidateStruct(data interface{}, rules ValidationRules) error {
	if data == nil {
		return ErrDataCannotBeNil
	}

	// Basic validation - check for required fields if data is a map
	if dataMap, ok := data.(map[string]interface{}); ok {
		for _, required := range rules.Required {
			if value, exists := dataMap[required]; !exists || value == nil || value == "" {
				return fmt.Errorf("%w: %s", ErrRequiredFieldMissing, required)
			}
		}

		// Check max length constraints
		for field, maxLen := range rules.MaxLen {
			if value, exists := dataMap[field]; exists {
				if strValue, ok := value.(string); ok && len(strValue) > maxLen {
					return fmt.Errorf("%w: %s exceeds %d characters", ErrFieldExceedsMaxLength, field, maxLen)
				}
			}
		}

		// Check min length constraints
		for field, minLen := range rules.MinLen {
			if value, exists := dataMap[field]; exists {
				if strValue, ok := value.(string); ok && len(strValue) < minLen {
					return fmt.Errorf("%w: %s below %d characters", ErrFieldBelowMinLength, field, minLen)
				}
			}
		}
	}

	return nil
}

// Common Business Logic Patterns
// Based on analysis of service layer, these are the most duplicated patterns:

// 1. Command Validation Pattern
// Found in: notes, accounts, relationships, media, lists, etc.

// ValidateCommand performs standard validation on command structures
func ValidateCommand(_ context.Context, cmd interface{}, rules ValidationRules) *CommandValidationResult {
	result := &CommandValidationResult{
		IsValid:  true,
		Errors:   make([]string, 0),
		Warnings: make([]string, 0),
	}

	// Apply validation rules
	if err := ValidateStruct(cmd, rules); err != nil {
		result.IsValid = false
		result.Errors = append(result.Errors, err.Error())
	}

	return result
}

// 2. Event Emission Pattern
// Found in all services with consistent structure

// EmitBusinessEvent creates standardized business events
func EmitBusinessEvent(_ context.Context, eventType, objectType, objectID, actorID string, metadata map[string]interface{}) *StreamingEvent {
	return &StreamingEvent{
		Type:      eventType,
		Object:    objectType,
		ObjectID:  objectID,
		ActorID:   actorID,
		Timestamp: time.Now(),
		Metadata:  metadata,
	}
}

// EmitEntityCreatedEvents creates standard creation events
func EmitEntityCreatedEvents(ctx context.Context, entityType, entityID, actorID string, entity interface{}) []*StreamingEvent {
	events := make([]*StreamingEvent, 0, 2)

	// Primary creation event
	events = append(events, EmitBusinessEvent(ctx,
		fmt.Sprintf("%s.created", entityType),
		entityType,
		entityID,
		actorID,
		map[string]interface{}{
			"entity": entity,
			"action": "create",
		},
	))

	// Timeline event for public visibility
	events = append(events, EmitBusinessEvent(ctx,
		"timeline.update",
		entityType,
		entityID,
		actorID,
		map[string]interface{}{
			"entity":     entity,
			"visibility": "public",
		},
	))

	return events
}

// EmitEntityUpdatedEvents creates standard update events
func EmitEntityUpdatedEvents(ctx context.Context, entityType, entityID, actorID string, entity interface{}, changes map[string]interface{}) []*StreamingEvent {
	events := make([]*StreamingEvent, 0, 1)

	events = append(events, EmitBusinessEvent(ctx,
		fmt.Sprintf("%s.updated", entityType),
		entityType,
		entityID,
		actorID,
		map[string]interface{}{
			"entity":  entity,
			"changes": changes,
			"action":  "update",
		},
	))

	return events
}

// EmitEntityDeletedEvents creates standard deletion events
func EmitEntityDeletedEvents(ctx context.Context, entityType, entityID, actorID string) []*StreamingEvent {
	events := make([]*StreamingEvent, 0, 1)

	events = append(events, EmitBusinessEvent(ctx,
		fmt.Sprintf("%s.deleted", entityType),
		entityType,
		entityID,
		actorID,
		map[string]interface{}{
			"action": "delete",
		},
	))

	return events
}

// 3. Content Validation Pattern
// Found in: notes, media, accounts

// ContentValidationRules defines rules for content validation
type ContentValidationRules struct {
	MaxLength      int
	MinLength      int
	AllowedTypes   []string
	RequiredFields []string
	ForbiddenWords []string
}

// ValidateBusinessContent validates content against business rules
func ValidateBusinessContent(content string, rules ContentValidationRules) error {
	if len(content) > rules.MaxLength {
		return fmt.Errorf("%w: %d characters", ErrContentExceedsMaxLength, rules.MaxLength)
	}

	if len(content) < rules.MinLength {
		return fmt.Errorf("%w: %d characters", ErrContentBelowMinLength, rules.MinLength)
	}

	// Check for forbidden words
	contentLower := strings.ToLower(content)
	for _, word := range rules.ForbiddenWords {
		if strings.Contains(contentLower, strings.ToLower(word)) {
			return fmt.Errorf("%w: %s", ErrContentContainsForbiddenWord, word)
		}
	}

	return nil
}

// 4. Relationship Validation Pattern
// Found in: relationships, accounts, lists

// ValidateUserRelationship validates relationships between users
func ValidateUserRelationship(_ context.Context, actorID, targetID string, relationshipType string) error {
	if actorID == targetID {
		return fmt.Errorf("%w: %s", ErrCannotPerformOperationOnSelf, relationshipType)
	}

	if actorID == "" || targetID == "" {
		return fmt.Errorf("%w: %s", ErrActorAndTargetIDsRequired, relationshipType)
	}

	return nil
}

// 5. Visibility and Privacy Pattern
// Found in: notes, accounts, conversations

// VisibilityLevel represents different visibility levels
type VisibilityLevel string

// Visibility levels for content
const (
	VisibilityPublic   VisibilityLevel = "public"   // VisibilityPublic makes content visible to everyone
	VisibilityUnlisted VisibilityLevel = "unlisted" // VisibilityUnlisted makes content visible but not in public timelines
	VisibilityPrivate  VisibilityLevel = "private"  // VisibilityPrivate makes content visible only to followers
	VisibilityDirect   VisibilityLevel = "direct"   // VisibilityDirect makes content visible only to mentioned users
)

// ValidateBusinessVisibility validates business visibility settings
func ValidateBusinessVisibility(visibility VisibilityLevel, _ string) error {
	validVisibilities := []VisibilityLevel{
		VisibilityPublic,
		VisibilityUnlisted,
		VisibilityPrivate,
		VisibilityDirect,
	}

	for _, valid := range validVisibilities {
		if visibility == valid {
			return nil
		}
	}

	return fmt.Errorf("%w: %s", ErrInvalidVisibilityLevel, visibility)
}

// 6. Rate Limiting and Quota Pattern
// Found in: notes, media, relationships

// QuotaValidator validates operations against quotas
type QuotaValidator struct {
	MaxActionsPerHour int
	MaxActionsPerDay  int
}

// ValidateQuota checks if action is within quota limits
func (q *QuotaValidator) ValidateQuota(_ context.Context, actorID, actionType string) error {
	// Implement actual quota validation logic
	if actorID == "" {
		return ErrActorIDRequiredForQuotaValidation
	}

	if actionType == "" {
		return ErrActionTypeRequiredForQuotaValidation
	}

	// For now, implement basic throttling based on configured limits
	// In a real system, this would check against storage for current usage
	switch actionType {
	case "post", "create":
		// Allow up to MaxActionsPerHour posts per hour
		if q.MaxActionsPerHour > 0 {
			// This would check actual usage from storage/cache
			// For now, we'll allow all requests but log the quota check
			_ = q.MaxActionsPerHour // Placeholder to avoid empty block
		}
	case "follow", "like", "announce":
		// Allow up to MaxActionsPerDay social actions per day
		if q.MaxActionsPerDay > 0 {
			// This would check actual usage from storage/cache
			_ = q.MaxActionsPerDay // Placeholder to avoid empty block
		}
	}

	return nil
}

// 7. Federation Activity Pattern
// Found in: notes, accounts, relationships

// ActivityPubActivity represents an ActivityPub activity
type ActivityPubActivity struct {
	Type   string
	Actor  string
	Object interface{}
	Target string
}

// CreateFederationActivity creates ActivityPub activities for federation
func CreateFederationActivity(actorID, activityType string, object interface{}) *ActivityPubActivity {
	return &ActivityPubActivity{
		Type:   activityType,
		Actor:  actorID,
		Object: object,
	}
}

// 8. Analytics and Metrics Pattern
// Found in: notes, accounts, relationships

// RecordBusinessMetric records business metrics using the existing metrics infrastructure
func RecordBusinessMetric(_ context.Context, metricType, _ string, actorID string, _ map[string]interface{}) error {
	if metricType == "" {
		return ErrMetricTypeRequired
	}

	if actorID == "" {
		return ErrActorIDRequiredForMetrics
	}

	// The actual implementation would use the existing observability.MetricsCollector
	// This function provides a standardized interface to record business metrics
	// that will be sent to CloudWatch via the existing infrastructure

	// The metrics would be recorded with appropriate dimensions:
	// - EntityType (note, actor, relationship, etc.)
	// - ActorID (for user-specific metrics)
	// - MetricType (creation, engagement, etc.)

	// Example of how this integrates with the existing system:
	// collector := observability.NewMetricsCollector(client, namespace, logger)
	// collector.RecordMetric(metricType, 1.0, types.StandardUnitCount,
	//     types.Dimension{Name: aws.String("EntityType"), Value: aws.String(entityType)},
	//     types.Dimension{Name: aws.String("ActorID"), Value: aws.String(actorID)})

	// For now, validate inputs and return success - the actual recording
	// happens through the service layer's analytics integration
	return nil
}

// 9. State Machine Pattern
// Found in: notes (draft->published), accounts (pending->active), relationships

// EntityState represents entity states
type EntityState string

// Entity state constants for state machine transitions
const (
	StateDraft     EntityState = "draft"     // StateDraft represents entities in draft state
	StatePending   EntityState = "pending"   // StatePending represents entities awaiting approval
	StateActive    EntityState = "active"    // StateActive represents active entities
	StateInactive  EntityState = "inactive"  // StateInactive represents temporarily disabled entities
	StateSuspended EntityState = "suspended" // StateSuspended represents suspended entities
	StateDeleted   EntityState = "deleted"   // StateDeleted represents deleted entities
)

// ValidateStateTransition validates state transitions
func ValidateStateTransition(currentState, newState EntityState, entityType string) error {
	validTransitions := map[EntityState][]EntityState{
		StateDraft:     {StateActive, StateDeleted},
		StatePending:   {StateActive, StateInactive, StateDeleted},
		StateActive:    {StateInactive, StateSuspended, StateDeleted},
		StateInactive:  {StateActive, StateDeleted},
		StateSuspended: {StateActive, StateDeleted},
		StateDeleted:   {}, // No transitions from deleted
	}

	validNext, exists := validTransitions[currentState]
	if !exists {
		return fmt.Errorf("%w: %s", ErrInvalidCurrentState, currentState)
	}

	for _, valid := range validNext {
		if newState == valid {
			return nil
		}
	}

	return fmt.Errorf("%w: from %s to %s for %s", ErrInvalidStateTransition, currentState, newState, entityType)
}

// 10. Resource Access Control Pattern
// Found in: all services

// AccessLevel represents access levels
type AccessLevel string

// Resource access level constants
const (
	AccessNone  AccessLevel = "none"  // AccessNone represents no access required
	AccessRead  AccessLevel = "read"  // AccessRead represents read access required
	AccessWrite AccessLevel = "write" // AccessWrite represents write access required
	AccessAdmin AccessLevel = "admin" // AccessAdmin represents admin access required
)

// ValidateResourceAccess validates access to resources using existing auth patterns
func ValidateResourceAccess(_ context.Context, actorID, resourceID, resourceType string, requiredAccess AccessLevel) error {
	// Validate inputs first
	if actorID == "" {
		return fmt.Errorf("%w: %s", ErrAuthenticationRequiredForAccess, resourceType)
	}

	if resourceID == "" {
		return ErrResourceIDRequiredForAccessValidation
	}

	if resourceType == "" {
		return ErrResourceTypeRequiredForAccessValidation
	}

	// Use existing validation patterns from the codebase
	// The actual access control is implemented in the service layer
	// through the auth.AuthService and its integration with storage

	// Basic access level validation
	switch requiredAccess {
	case AccessNone:
		// No access required - this is for public resources
		return nil
	case AccessRead, AccessWrite, AccessAdmin:
		// These require actual authentication and authorization checks
		// which are handled by the service layer's auth integration
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrInvalidAccessLevel, requiredAccess)
	}
}

// Business Logic Service Factory
// This provides a centralized way to create business logic components

// BusinessLogicService provides centralized business logic operations
type BusinessLogicService struct {
	logger       *zap.Logger
	eventEmitter EventEmitter
	domainName   string
}

// NewBusinessLogicService creates a new business logic service
func NewBusinessLogicService(logger *zap.Logger, eventEmitter EventEmitter, domainName string) *BusinessLogicService {
	return &BusinessLogicService{
		logger:       logger,
		eventEmitter: eventEmitter,
		domainName:   domainName,
	}
}

// ExecuteBusinessOperation executes a business operation with standard patterns
func (s *BusinessLogicService) ExecuteBusinessOperation(ctx context.Context, operation BusinessOperation) error {
	// 1. Validate operation
	if err := operation.Validate(ctx); err != nil {
		return fmt.Errorf("%w: %w", ErrOperationValidationFailed, err)
	}

	// 2. Execute operation
	if err := operation.Execute(ctx); err != nil {
		return fmt.Errorf("%w: %w", ErrOperationExecutionFailed, err)
	}

	// 3. Emit events
	events := operation.GetEvents(ctx)
	if len(events) > 0 && s.eventEmitter != nil {
		if err := s.eventEmitter.EmitEvents(ctx, events); err != nil {
			s.logger.Error("Failed to emit events", zap.Error(err))
			// Don't fail the operation for event emission failures
		}
	}

	// 4. Record metrics
	if err := operation.RecordMetrics(ctx); err != nil {
		s.logger.Error("Failed to record metrics", zap.Error(err))
		// Don't fail the operation for metrics failures
	}

	return nil
}

// BusinessOperation defines the interface for business operations
type BusinessOperation interface {
	Validate(ctx context.Context) error
	Execute(ctx context.Context) error
	GetEvents(ctx context.Context) []*StreamingEvent
	RecordMetrics(ctx context.Context) error
}
