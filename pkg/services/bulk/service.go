// Package bulk provides bulk operation services for the Lesser ActivityPub server.
//
// This service handles all bulk operations including:
// - Bulk follow/unfollow operations
// - Bulk mute/unmute operations
// - Bulk block/unblock operations
// - Bulk status deletion
// - Bulk list member management
// - Progress tracking and event emission for real-time updates
package bulk

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service provides business logic for bulk operations
type Service struct {
	statusRepo       *repositories.StatusRepository
	accountRepo      interfaces.AccountRepository
	socialRepo       interfaces.SocialRepository
	listRepo         *repositories.ListRepository
	relationshipRepo *repositories.RelationshipRepository
	publisher        streaming.Publisher
	federation       FederationService
	logger           *zap.Logger
	domain           string

	// Track ongoing operations
	operations sync.Map // map[string]*BulkOperation
}

// FederationService defines the interface for federation operations
type FederationService interface {
	QueueActivity(ctx context.Context, activity *activitypub.Activity) error
}

// NewService creates a new bulk operations service
func NewService(
	statusRepo *repositories.StatusRepository,
	accountRepo interfaces.AccountRepository,
	socialRepo interfaces.SocialRepository,
	listRepo *repositories.ListRepository,
	relationshipRepo *repositories.RelationshipRepository,
	publisher streaming.Publisher,
	federation FederationService,
	logger *zap.Logger,
	domain string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		statusRepo:       statusRepo,
		accountRepo:      accountRepo,
		socialRepo:       socialRepo,
		listRepo:         listRepo,
		relationshipRepo: relationshipRepo,
		publisher:        publisher,
		federation:       federation,
		logger:           logger,
		domain:           domain,
	}
}

// Commands and Queries (following CQRS pattern)

// BulkFollowCommand contains data for bulk follow operations
type BulkFollowCommand struct {
	Username   string   `json:"username" validate:"required"`
	AccountIDs []string `json:"account_ids" validate:"required,min=1,max=100"`
	Reblogs    bool     `json:"reblogs"`
	Notify     bool     `json:"notify"`
	Languages  []string `json:"languages"`
}

// BulkUnfollowCommand contains data for bulk unfollow operations
type BulkUnfollowCommand struct {
	Username   string   `json:"username" validate:"required"`
	AccountIDs []string `json:"account_ids" validate:"required,min=1,max=100"`
}

// BulkMuteCommand contains data for bulk mute operations
type BulkMuteCommand struct {
	Username     string        `json:"username" validate:"required"`
	AccountIDs   []string      `json:"account_ids" validate:"required,min=1,max=100"`
	Notifications bool         `json:"notifications"`
	Duration     *time.Duration `json:"duration"`
}

// BulkUnmuteCommand contains data for bulk unmute operations
type BulkUnmuteCommand struct {
	Username   string   `json:"username" validate:"required"`
	AccountIDs []string `json:"account_ids" validate:"required,min=1,max=100"`
}

// BulkBlockCommand contains data for bulk block operations
type BulkBlockCommand struct {
	Username   string   `json:"username" validate:"required"`
	AccountIDs []string `json:"account_ids" validate:"required,min=1,max=100"`
}

// BulkUnblockCommand contains data for bulk unblock operations
type BulkUnblockCommand struct {
	Username   string   `json:"username" validate:"required"`
	AccountIDs []string `json:"account_ids" validate:"required,min=1,max=100"`
}

// BulkDeleteStatusesCommand contains data for bulk status deletion
type BulkDeleteStatusesCommand struct {
	Username  string     `json:"username" validate:"required"`
	StatusIDs []string   `json:"status_ids" validate:"required,min=1,max=100"`
	DateRange *DateRange `json:"date_range"`
	KeepPinned bool      `json:"keep_pinned"`
}

// BulkListMembersCommand contains data for bulk list member operations
type BulkListMembersCommand struct {
	Username   string   `json:"username" validate:"required"`
	ListID     string   `json:"list_id" validate:"required"`
	AccountIDs []string `json:"account_ids" validate:"required,min=1,max=100"`
	Operation  string   `json:"operation" validate:"required,oneof=add remove"`
}

// GetOperationQuery retrieves the status of a bulk operation
type GetOperationQuery struct {
	OperationID string `json:"operation_id" validate:"required"`
	Username    string `json:"username" validate:"required"` // For authorization
}

// DateRange specifies a time range for operations
type DateRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Result types

// BulkOperation tracks the progress of a bulk operation
type BulkOperation struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Username    string            `json:"username"`
	Status      string            `json:"status"` // pending, processing, completed, failed
	Total       int               `json:"total"`
	Processed   int               `json:"processed"`
	Succeeded   int               `json:"succeeded"`
	Failed      int               `json:"failed"`
	Errors      []string          `json:"errors,omitempty"`
	StartedAt   time.Time         `json:"started_at"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	Events      []streaming.Event `json:"-"`
}

// BulkOperationResult contains the result of a bulk operation
type BulkOperationResult struct {
	Operation *BulkOperation    `json:"operation"`
	Events    []streaming.Event `json:"-"`
}

// BulkFollow performs bulk follow operations
func (s *Service) BulkFollow(ctx context.Context, cmd *BulkFollowCommand) (*BulkOperationResult, error) {
	// Create operation tracking
	operation := &BulkOperation{
		ID:        uuid.New().String(),
		Type:      "bulk_follow",
		Username:  cmd.Username,
		Status:    "processing",
		Total:     len(cmd.AccountIDs),
		Processed: 0,
		Succeeded: 0,
		Failed:    0,
		StartedAt: time.Now(),
	}

	// Store operation for tracking
	s.operations.Store(operation.ID, operation)

	// Process in background to avoid timeout
	go s.processBulkFollow(ctx, operation, cmd)

	// Emit start event
	if s.publisher != nil {
		event := s.createOperationEvent("bulk_operation.started", operation)
		_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
		operation.Events = append(operation.Events, event)
	}

	return &BulkOperationResult{
		Operation: operation,
		Events:    operation.Events,
	}, nil
}

// processBulkFollow processes bulk follow operations asynchronously
func (s *Service) processBulkFollow(ctx context.Context, operation *BulkOperation, cmd *BulkFollowCommand) {
	defer func() {
		now := time.Now()
		operation.CompletedAt = &now
		operation.Status = "completed"
		
		// Emit completion event
		if s.publisher != nil {
			event := s.createOperationEvent("bulk_operation.completed", operation)
			_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
		}
		
		// Clean up after 1 hour
		time.AfterFunc(time.Hour, func() {
			s.operations.Delete(operation.ID)
		})
	}()

	// Process each follow
	for _, accountID := range cmd.AccountIDs {
		// Create follow relationship using relationship repository
		activityID := fmt.Sprintf("https://%s/users/%s/follows/%s", s.domain, cmd.Username, accountID)
		err := s.relationshipRepo.CreateRelationship(ctx, cmd.Username, accountID, activityID)
		
		operation.Processed++
		
		if err != nil {
			operation.Failed++
			operation.Errors = append(operation.Errors, fmt.Sprintf("Failed to follow %s: %v", accountID, err))
			s.logger.Warn("failed to follow account in bulk operation",
				zap.String("username", cmd.Username),
				zap.String("target", accountID),
				zap.Error(err))
		} else {
			operation.Succeeded++
			
			// Queue federation activity
			if s.federation != nil {
				activity := s.createFollowActivity(cmd.Username, accountID)
				_ = s.federation.QueueActivity(ctx, activity)
			}
		}
		
		// Emit progress event every 10 items or at completion
		if operation.Processed%10 == 0 || operation.Processed == operation.Total {
			if s.publisher != nil {
				event := s.createProgressEvent(operation)
				_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
			}
		}
		
		// Small delay to avoid overwhelming the system
		if operation.Processed < operation.Total {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// BulkDeleteStatuses performs bulk status deletion
func (s *Service) BulkDeleteStatuses(ctx context.Context, cmd *BulkDeleteStatusesCommand) (*BulkOperationResult, error) {
	// Create operation tracking
	operation := &BulkOperation{
		ID:        uuid.New().String(),
		Type:      "bulk_delete_statuses",
		Username:  cmd.Username,
		Status:    "processing",
		Total:     len(cmd.StatusIDs),
		Processed: 0,
		Succeeded: 0,
		Failed:    0,
		StartedAt: time.Now(),
	}

	// Store operation for tracking
	s.operations.Store(operation.ID, operation)

	// Process in background
	go s.processBulkDeleteStatuses(ctx, operation, cmd)

	// Emit start event
	if s.publisher != nil {
		event := s.createOperationEvent("bulk_operation.started", operation)
		_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
		operation.Events = append(operation.Events, event)
	}

	return &BulkOperationResult{
		Operation: operation,
		Events:    operation.Events,
	}, nil
}

// processBulkDeleteStatuses processes bulk status deletion asynchronously
func (s *Service) processBulkDeleteStatuses(ctx context.Context, operation *BulkOperation, cmd *BulkDeleteStatusesCommand) {
	defer func() {
		now := time.Now()
		operation.CompletedAt = &now
		operation.Status = "completed"
		
		// Emit completion event
		if s.publisher != nil {
			event := s.createOperationEvent("bulk_operation.completed", operation)
			_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
		}
		
		// Clean up after 1 hour
		time.AfterFunc(time.Hour, func() {
			s.operations.Delete(operation.ID)
		})
	}()

	// Process each status deletion
	for _, statusID := range cmd.StatusIDs {
		// Get status to verify ownership
		status, err := s.statusRepo.GetStatus(ctx, statusID)
		if err != nil {
			operation.Failed++
			operation.Errors = append(operation.Errors, fmt.Sprintf("Status %s not found", statusID))
			operation.Processed++
			continue
		}

		// Verify ownership
		if status.AuthorUsername != cmd.Username {
			operation.Failed++
			operation.Errors = append(operation.Errors, fmt.Sprintf("Not authorized to delete status %s", statusID))
			operation.Processed++
			continue
		}

		// Note: Pinned status checking would need to be implemented in the status model
		// For now, we'll skip this check

		// Delete the status
		err = s.statusRepo.DeleteStatus(ctx, statusID)
		operation.Processed++
		
		if err != nil {
			operation.Failed++
			operation.Errors = append(operation.Errors, fmt.Sprintf("Failed to delete %s: %v", statusID, err))
		} else {
			operation.Succeeded++
			
			// Emit deletion event for timeline updates
			if s.publisher != nil {
				event := streaming.Event{
					Type:      "status.deleted",
					Stream:    "user",
					Payload:   map[string]interface{}{"id": statusID},
					Timestamp: time.Now(),
				}
				_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
			}
			
			// Queue federation deletion
			if s.federation != nil {
				activity := s.createDeleteActivity(cmd.Username, statusID)
				_ = s.federation.QueueActivity(ctx, activity)
			}
		}
		
		// Emit progress event every 10 items
		if operation.Processed%10 == 0 || operation.Processed == operation.Total {
			if s.publisher != nil {
				event := s.createProgressEvent(operation)
				_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
			}
		}
		
		// Small delay to avoid overwhelming the system
		if operation.Processed < operation.Total {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// GetOperation retrieves the status of a bulk operation
func (s *Service) GetOperation(ctx context.Context, query *GetOperationQuery) (*BulkOperationResult, error) {
	value, ok := s.operations.Load(query.OperationID)
	if !ok {
		return nil, fmt.Errorf("operation not found")
	}

	operation, ok := value.(*BulkOperation)
	if !ok {
		return nil, fmt.Errorf("invalid operation data")
	}

	// Verify ownership
	if operation.Username != query.Username {
		return nil, fmt.Errorf("unauthorized")
	}

	return &BulkOperationResult{
		Operation: operation,
	}, nil
}

// Helper methods

func (s *Service) createOperationEvent(eventType string, operation *BulkOperation) streaming.Event {
	return streaming.Event{
		Type:      eventType,
		Stream:    "user",
		Payload:   map[string]interface{}{
			"operation": operation,
		},
		Timestamp: time.Now(),
	}
}

func (s *Service) createProgressEvent(operation *BulkOperation) streaming.Event {
	percent := float64(operation.Processed) / float64(operation.Total) * 100
	return streaming.Event{
		Type:   "bulk_operation.progress",
		Stream: "user",
		Payload: map[string]interface{}{
			"operation_id": operation.ID,
			"type":         operation.Type,
			"processed":    operation.Processed,
			"total":        operation.Total,
			"succeeded":    operation.Succeeded,
			"failed":       operation.Failed,
			"percent":      percent,
		},
		Timestamp: time.Now(),
	}
}

func (s *Service) createFollowActivity(username, targetID string) *activitypub.Activity {
	actorURL := fmt.Sprintf("https://%s/users/%s", s.domain, username)
	targetURL := fmt.Sprintf("https://%s/users/%s", s.domain, targetID)
	
	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   fmt.Sprintf("%s/follows/%s", actorURL, targetID),
			Type: "Follow",
		},
		Actor:  actorURL,
		Object: targetURL,
	}
}

func (s *Service) createDeleteActivity(username, statusID string) *activitypub.Activity {
	actorURL := fmt.Sprintf("https://%s/users/%s", s.domain, username)
	objectURL := fmt.Sprintf("https://%s/users/%s/statuses/%s", s.domain, username, statusID)
	
	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   fmt.Sprintf("%s/delete/%s", actorURL, statusID),
			Type: "Delete",
		},
		Actor:  actorURL,
		Object: objectURL,
	}
}