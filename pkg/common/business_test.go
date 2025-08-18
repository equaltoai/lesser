package common

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

// mockEventEmitter for testing
type mockEventEmitter struct {
	events []*StreamingEvent
}

func (m *mockEventEmitter) EmitEvents(ctx context.Context, events []*StreamingEvent) error {
	m.events = append(m.events, events...)
	return nil
}

func TestBusinessLogicService_NewBusinessLogicService(t *testing.T) {
	logger := zap.NewNop()
	emitter := &mockEventEmitter{}
	domain := "test.example.com"

	service := NewBusinessLogicService(logger, emitter, domain)

	if service == nil {
		t.Error("NewBusinessLogicService returned nil")
	}
	if service.logger != logger {
		t.Error("Logger not set correctly")
	}
	if service.eventEmitter != emitter {
		t.Error("EventEmitter not set correctly")
	}
	if service.domainName != domain {
		t.Error("Domain name not set correctly")
	}
}

func TestValidateCommand(t *testing.T) {
	ctx := context.Background()
	
	tests := []struct {
		name     string
		cmd      interface{}
		rules    ValidationRules
		expected bool
	}{
		{
			name: "valid command",
			cmd:  map[string]string{"field": "value"},
			rules: ValidationRules{
				Required: []string{"field"},
				MaxLen:   map[string]int{"field": 10},
			},
			expected: true,
		},
		{
			name: "nil command",
			cmd:  nil,
			rules: ValidationRules{
				Required: []string{"field"},
			},
			expected: false, // ValidateStruct should fail for nil data
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateCommand(ctx, tt.cmd, tt.rules)
			if result.IsValid != tt.expected {
				t.Errorf("ValidateCommand() = %v, want %v", result.IsValid, tt.expected)
			}
		})
	}
}

func TestEmitBusinessEvent(t *testing.T) {
	ctx := context.Background()
	eventType := "test.created"
	objectType := "test"
	objectID := "123"
	actorID := "actor123"
	metadata := map[string]interface{}{"key": "value"}

	event := EmitBusinessEvent(ctx, eventType, objectType, objectID, actorID, metadata)

	if event.Type != eventType {
		t.Errorf("Event type = %v, want %v", event.Type, eventType)
	}
	if event.Object != objectType {
		t.Errorf("Event object = %v, want %v", event.Object, objectType)
	}
	if event.ObjectID != objectID {
		t.Errorf("Event objectID = %v, want %v", event.ObjectID, objectID)
	}
	if event.ActorID != actorID {
		t.Errorf("Event actorID = %v, want %v", event.ActorID, actorID)
	}
	if event.Metadata["key"] != "value" {
		t.Errorf("Event metadata not set correctly")
	}
	if event.Timestamp.IsZero() {
		t.Error("Event timestamp not set")
	}
}

func TestEmitEntityCreatedEvents(t *testing.T) {
	ctx := context.Background()
	entityType := "note"
	entityID := "note123"
	actorID := "actor123"
	entity := map[string]string{"content": "test"}

	events := EmitEntityCreatedEvents(ctx, entityType, entityID, actorID, entity)

	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}

	// Check creation event
	creationEvent := events[0]
	if creationEvent.Type != "note.created" {
		t.Errorf("Creation event type = %v, want %v", creationEvent.Type, "note.created")
	}
	if creationEvent.ActorID != actorID {
		t.Errorf("Creation event actorID = %v, want %v", creationEvent.ActorID, actorID)
	}

	// Check timeline event
	timelineEvent := events[1]
	if timelineEvent.Type != "timeline.update" {
		t.Errorf("Timeline event type = %v, want %v", timelineEvent.Type, "timeline.update")
	}
	if timelineEvent.Metadata["visibility"] != "public" {
		t.Errorf("Timeline event visibility not set to public")
	}
}

func TestEmitEntityUpdatedEvents(t *testing.T) {
	ctx := context.Background()
	entityType := "note"
	entityID := "note123"
	actorID := "actor123"
	entity := map[string]string{"content": "updated"}
	changes := map[string]interface{}{"content": "changed"}

	events := EmitEntityUpdatedEvents(ctx, entityType, entityID, actorID, entity, changes)

	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.Type != "note.updated" {
		t.Errorf("Event type = %v, want %v", event.Type, "note.updated")
	}
	if event.Metadata["action"] != "update" {
		t.Errorf("Event action not set to update")
	}
	if event.Metadata["changes"] == nil {
		t.Error("Event changes not set")
	}
}

func TestEmitEntityDeletedEvents(t *testing.T) {
	ctx := context.Background()
	entityType := "note"
	entityID := "note123"
	actorID := "actor123"

	events := EmitEntityDeletedEvents(ctx, entityType, entityID, actorID)

	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.Type != "note.deleted" {
		t.Errorf("Event type = %v, want %v", event.Type, "note.deleted")
	}
	if event.Metadata["action"] != "delete" {
		t.Errorf("Event action not set to delete")
	}
}

func TestValidateBusinessContent(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		rules     ContentValidationRules
		expectErr bool
	}{
		{
			name:    "valid content",
			content: "This is valid content",
			rules: ContentValidationRules{
				MaxLength: 100,
				MinLength: 5,
			},
			expectErr: false,
		},
		{
			name:    "content too long",
			content: "This is very long content that exceeds the maximum length",
			rules: ContentValidationRules{
				MaxLength: 10,
				MinLength: 5,
			},
			expectErr: true,
		},
		{
			name:    "content too short",
			content: "Hi",
			rules: ContentValidationRules{
				MaxLength: 100,
				MinLength: 5,
			},
			expectErr: true,
		},
		{
			name:    "forbidden word",
			content: "This contains spam content",
			rules: ContentValidationRules{
				MaxLength:      100,
				MinLength:      5,
				ForbiddenWords: []string{"spam"},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBusinessContent(tt.content, tt.rules)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateBusinessContent() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestValidateUserRelationship(t *testing.T) {
	ctx := context.Background()
	
	tests := []struct {
		name             string
		actorID          string
		targetID         string
		relationshipType string
		expectErr        bool
	}{
		{
			name:             "valid relationship",
			actorID:          "actor1",
			targetID:         "actor2",
			relationshipType: "follow",
			expectErr:        false,
		},
		{
			name:             "self relationship",
			actorID:          "actor1",
			targetID:         "actor1",
			relationshipType: "follow",
			expectErr:        true,
		},
		{
			name:             "empty actor ID",
			actorID:          "",
			targetID:         "actor2",
			relationshipType: "follow",
			expectErr:        true,
		},
		{
			name:             "empty target ID",
			actorID:          "actor1",
			targetID:         "",
			relationshipType: "follow",
			expectErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUserRelationship(ctx, tt.actorID, tt.targetID, tt.relationshipType)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateUserRelationship() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestValidateBusinessVisibility(t *testing.T) {
	tests := []struct {
		name       string
		visibility VisibilityLevel
		actorID    string
		expectErr  bool
	}{
		{
			name:       "public visibility",
			visibility: VisibilityPublic,
			actorID:    "actor1",
			expectErr:  false,
		},
		{
			name:       "unlisted visibility",
			visibility: VisibilityUnlisted,
			actorID:    "actor1",
			expectErr:  false,
		},
		{
			name:       "private visibility",
			visibility: VisibilityPrivate,
			actorID:    "actor1",
			expectErr:  false,
		},
		{
			name:       "direct visibility",
			visibility: VisibilityDirect,
			actorID:    "actor1",
			expectErr:  false,
		},
		{
			name:       "invalid visibility",
			visibility: VisibilityLevel("invalid"),
			actorID:    "actor1",
			expectErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBusinessVisibility(tt.visibility, tt.actorID)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateBusinessVisibility() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestValidateStateTransition(t *testing.T) {
	tests := []struct {
		name         string
		currentState EntityState
		newState     EntityState
		entityType   string
		expectErr    bool
	}{
		{
			name:         "draft to active",
			currentState: StateDraft,
			newState:     StateActive,
			entityType:   "note",
			expectErr:    false,
		},
		{
			name:         "draft to deleted",
			currentState: StateDraft,
			newState:     StateDeleted,
			entityType:   "note",
			expectErr:    false,
		},
		{
			name:         "invalid transition draft to suspended",
			currentState: StateDraft,
			newState:     StateSuspended,
			entityType:   "note",
			expectErr:    true,
		},
		{
			name:         "invalid state",
			currentState: EntityState("invalid"),
			newState:     StateActive,
			entityType:   "note",
			expectErr:    true,
		},
		{
			name:         "transition from deleted",
			currentState: StateDeleted,
			newState:     StateActive,
			entityType:   "note",
			expectErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStateTransition(tt.currentState, tt.newState, tt.entityType)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateStateTransition() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestValidateResourceAccess(t *testing.T) {
	ctx := context.Background()
	
	tests := []struct {
		name           string
		actorID        string
		resourceID     string
		resourceType   string
		requiredAccess AccessLevel
		expectErr      bool
	}{
		{
			name:           "valid access",
			actorID:        "actor1",
			resourceID:     "resource1",
			resourceType:   "note",
			requiredAccess: AccessRead,
			expectErr:      false,
		},
		{
			name:           "empty actor ID",
			actorID:        "",
			resourceID:     "resource1",
			resourceType:   "note",
			requiredAccess: AccessRead,
			expectErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateResourceAccess(ctx, tt.actorID, tt.resourceID, tt.resourceType, tt.requiredAccess)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateResourceAccess() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestCreateFederationActivity(t *testing.T) {
	actorID := "actor1"
	activityType := "Create"
	object := map[string]string{"type": "Note"}

	activity := CreateFederationActivity(actorID, activityType, object)

	if activity.Type != activityType {
		t.Errorf("Activity type = %v, want %v", activity.Type, activityType)
	}
	if activity.Actor != actorID {
		t.Errorf("Activity actor = %v, want %v", activity.Actor, actorID)
	}
	if activity.Object == nil {
		t.Error("Activity object is nil")
	}
}

func TestQuotaValidator_ValidateQuota(t *testing.T) {
	ctx := context.Background()
	validator := &QuotaValidator{
		MaxActionsPerHour: 100,
		MaxActionsPerDay:  1000,
	}

	// Test basic quota validation (placeholder implementation returns nil)
	err := validator.ValidateQuota(ctx, "actor1", "create")
	if err != nil {
		t.Errorf("ValidateQuota() error = %v, want nil", err)
	}
}

// Test Business Operation Interface
type mockBusinessOperation struct {
	validateErr     error
	executeErr      error
	events          []*StreamingEvent
	recordMetricErr error
}

func (m *mockBusinessOperation) Validate(ctx context.Context) error {
	return m.validateErr
}

func (m *mockBusinessOperation) Execute(ctx context.Context) error {
	return m.executeErr
}

func (m *mockBusinessOperation) GetEvents(ctx context.Context) []*StreamingEvent {
	return m.events
}

func (m *mockBusinessOperation) RecordMetrics(ctx context.Context) error {
	return m.recordMetricErr
}

func TestBusinessLogicService_ExecuteBusinessOperation(t *testing.T) {
	logger := zap.NewNop()
	emitter := &mockEventEmitter{}
	service := NewBusinessLogicService(logger, emitter, "test.example.com")

	tests := []struct {
		name      string
		operation *mockBusinessOperation
		expectErr bool
	}{
		{
			name: "successful operation",
			operation: &mockBusinessOperation{
				events: []*StreamingEvent{
					{Type: "test.event", ActorID: "actor1", Timestamp: time.Now()},
				},
			},
			expectErr: false,
		},
		{
			name: "validation error",
			operation: &mockBusinessOperation{
				validateErr: &ValidationError{Message: "validation failed"},
			},
			expectErr: true,
		},
		{
			name: "execution error",
			operation: &mockBusinessOperation{
				executeErr: &ValidationError{Message: "execution failed"},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := service.ExecuteBusinessOperation(ctx, tt.operation)
			if (err != nil) != tt.expectErr {
				t.Errorf("ExecuteBusinessOperation() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}