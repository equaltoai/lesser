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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	serviceerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Constants for bulk operations
const (
	StatusCompleted = "completed"
	StatusStatus    = "status"
)

// Service provides business logic for bulk operations
type Service struct {
	statusRepo       statusRepository
	accountRepo      interfaces.AccountRepository
	socialRepo       interfaces.SocialRepository
	listRepo         listRepository
	relationshipRepo relationshipRepository
	publisher        streaming.Publisher
	federation       FederationService
	logger           *zap.Logger
	domain           string

	// Track ongoing operations
	operations sync.Map // map[string]*Operation
}

// FederationService defines the interface for federation operations
type FederationService interface {
	QueueActivity(ctx context.Context, activity *activitypub.Activity) error
}

type statusRepository interface {
	GetStatus(ctx context.Context, statusID string) (*models.Status, error)
	DeleteStatus(ctx context.Context, statusID string) error
	UpdateStatus(ctx context.Context, status *models.Status) error
}

type listRepository interface {
	AddListMember(ctx context.Context, listID, memberUsername string) error
	RemoveListMember(ctx context.Context, listID, memberUsername string) error
}

type relationshipRepository interface {
	CreateRelationship(ctx context.Context, followerUsername, followingUsername, activityID string) error
	DeleteBlock(ctx context.Context, blockerActor, blockedActor string) error
	CreateMute(ctx context.Context, muterActor, mutedActor, activityID string, hideNotifications bool, duration *time.Duration) error
	CreateBlock(ctx context.Context, blockerActor, blockedActor, activityID string) error
}

// NewService creates a new bulk operations service
func NewService(
	statusRepo interfaces.StatusRepository,
	accountRepo interfaces.AccountRepository,
	socialRepo interfaces.SocialRepository,
	listRepo *repositories.ListRepository,
	relationshipRepo interfaces.ConcreteRelationshipRepository,
	publisher streaming.Publisher,
	federation FederationService,
	logger *zap.Logger,
	domain string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	var statusRepository statusRepository
	if statusRepo != nil {
		statusRepository = statusRepo
	}
	var listRepository listRepository
	if listRepo != nil {
		listRepository = listRepo
	}
	var relationshipRepository relationshipRepository
	if relationshipRepo != nil {
		relationshipRepository = relationshipRepo
	}

	return &Service{
		statusRepo:       statusRepository,
		accountRepo:      accountRepo,
		socialRepo:       socialRepo,
		listRepo:         listRepository,
		relationshipRepo: relationshipRepository,
		publisher:        publisher,
		federation:       federation,
		logger:           logger,
		domain:           domain,
	}
}

// Commands and Queries (following CQRS pattern)

// FollowCommand contains data for bulk follow operations
type FollowCommand struct {
	Username   string   `json:"username" validate:"required"`
	AccountIDs []string `json:"account_ids" validate:"required,min=1,max=100"`
	Reblogs    bool     `json:"reblogs"`
	Notify     bool     `json:"notify"`
	Languages  []string `json:"languages"`
}

// UnfollowCommand contains data for bulk unfollow operations
type UnfollowCommand struct {
	Username   string   `json:"username" validate:"required"`
	AccountIDs []string `json:"account_ids" validate:"required,min=1,max=100"`
}

// MuteCommand contains data for bulk mute operations
type MuteCommand struct {
	Username      string         `json:"username" validate:"required"`
	AccountIDs    []string       `json:"account_ids" validate:"required,min=1,max=100"`
	Notifications bool           `json:"notifications"`
	Duration      *time.Duration `json:"duration"`
}

// UnmuteCommand contains data for bulk unmute operations
type UnmuteCommand struct {
	Username   string   `json:"username" validate:"required"`
	AccountIDs []string `json:"account_ids" validate:"required,min=1,max=100"`
}

// BlockCommand contains data for bulk block operations
type BlockCommand struct {
	Username   string   `json:"username" validate:"required"`
	AccountIDs []string `json:"account_ids" validate:"required,min=1,max=100"`
}

// UnblockCommand contains data for bulk unblock operations
type UnblockCommand struct {
	Username   string   `json:"username" validate:"required"`
	AccountIDs []string `json:"account_ids" validate:"required,min=1,max=100"`
}

// DeleteStatusesCommand contains data for bulk status deletion
type DeleteStatusesCommand struct {
	Username   string     `json:"username" validate:"required"`
	StatusIDs  []string   `json:"status_ids" validate:"required,min=1,max=100"`
	DateRange  *DateRange `json:"date_range"`
	KeepPinned bool       `json:"keep_pinned"`
}

// ListMembersCommand contains data for bulk list member operations
type ListMembersCommand struct {
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

// DeleteCommand contains data for bulk content deletion
type DeleteCommand struct {
	Username    string     `json:"username" validate:"required"`
	ContentIDs  []string   `json:"content_ids" validate:"required,min=1,max=100"`
	ContentType string     `json:"content_type"` // "status", "media", "all" - defaults to "status"
	DateRange   *DateRange `json:"date_range"`
	Permanent   bool       `json:"permanent"` // If true, permanently delete; if false, soft delete
}

// ArchiveCommand contains data for bulk content archiving
type ArchiveCommand struct {
	Username    string     `json:"username" validate:"required"`
	ContentIDs  []string   `json:"content_ids" validate:"required,min=1,max=100"`
	ContentType string     `json:"content_type"` // "status", "media", "all" - defaults to "status"
	DateRange   *DateRange `json:"date_range"`
}

// RestoreCommand contains data for bulk content restoration
type RestoreCommand struct {
	Username    string     `json:"username" validate:"required"`
	ContentIDs  []string   `json:"content_ids" validate:"required,min=1,max=100"`
	ContentType string     `json:"content_type"` // "status", "media", "all" - defaults to "status"
	DateRange   *DateRange `json:"date_range"`
}

// ExportCommand contains data for bulk content export
type ExportCommand struct {
	Username     string     `json:"username" validate:"required"`
	ContentIDs   []string   `json:"content_ids" validate:"required,min=1,max=100"`
	Format       string     `json:"format" validate:"required"` // "json", "csv", "activitypub"
	ContentType  string     `json:"content_type"`               // "status", "media", "all" - defaults to "status"
	IncludeMedia bool       `json:"include_media"`
	DateRange    *DateRange `json:"date_range"`
}

// DateRange specifies a time range for operations
type DateRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Result types

// Operation tracks the progress of a bulk operation
type Operation struct {
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

// OperationResult contains the result of a bulk operation
type OperationResult struct {
	Operation *Operation        `json:"operation"`
	Events    []streaming.Event `json:"-"`
}

// BulkFollow performs bulk follow operations
func (s *Service) BulkFollow(ctx context.Context, cmd *FollowCommand) (*OperationResult, error) {
	// Create operation tracking
	operation := &Operation{
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

	// Emit start event
	if s.publisher != nil {
		event := s.createOperationEvent("bulk_operation.started", operation)
		_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
		operation.Events = append(operation.Events, event)
	}

	// Process in background to avoid timeout
	go s.processBulkFollow(ctx, operation, cmd)

	return &OperationResult{
		Operation: operation,
		Events:    operation.Events,
	}, nil
}

// processBulkFollow processes bulk follow operations asynchronously
func (s *Service) processBulkFollow(ctx context.Context, operation *Operation, cmd *FollowCommand) {
	defer func() {
		now := time.Now()
		operation.CompletedAt = &now
		operation.Status = StatusCompleted

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
func (s *Service) BulkDeleteStatuses(ctx context.Context, cmd *DeleteStatusesCommand) (*OperationResult, error) {
	// Create operation tracking
	operation := &Operation{
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

	// Emit start event
	if s.publisher != nil {
		event := s.createOperationEvent("bulk_operation.started", operation)
		_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
		operation.Events = append(operation.Events, event)
	}

	// Process in background
	go s.processBulkDeleteStatuses(ctx, operation, cmd)

	return &OperationResult{
		Operation: operation,
		Events:    operation.Events,
	}, nil
}

// processBulkDeleteStatuses processes bulk status deletion asynchronously
func (s *Service) processBulkDeleteStatuses(ctx context.Context, operation *Operation, cmd *DeleteStatusesCommand) {
	defer func() {
		now := time.Now()
		operation.CompletedAt = &now
		operation.Status = StatusCompleted

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
func (s *Service) GetOperation(_ context.Context, query *GetOperationQuery) (*OperationResult, error) {
	value, ok := s.operations.Load(query.OperationID)
	if !ok {
		return nil, serviceerrors.ErrBulkOperationNotFound
	}

	operation, ok := value.(*Operation)
	if !ok {
		return nil, serviceerrors.ErrBulkOperationInvalidData
	}

	// Verify ownership
	if operation.Username != query.Username {
		s.logger.Warn("unauthorized bulk operation access",
			zap.String("requesting_user", query.Username),
			zap.String("operation_user", operation.Username),
			zap.String("operation_id", query.OperationID))
		return nil, common.ErrForbidden(serviceerrors.ErrBulkOperationUnauthorizedAccess)
	}

	return &OperationResult{
		Operation: operation,
	}, nil
}

// Helper methods

func (s *Service) createOperationEvent(eventType string, operation *Operation) streaming.Event {
	return streaming.Event{
		Type:   eventType,
		Stream: "user",
		Payload: map[string]interface{}{
			"operation": operation,
		},
		Timestamp: time.Now(),
	}
}

func (s *Service) createProgressEvent(operation *Operation) streaming.Event {
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

// BulkDelete performs bulk content deletion operations
func (s *Service) BulkDelete(ctx context.Context, cmd *DeleteCommand) (*OperationResult, error) {
	// Create operation tracking
	operation := &Operation{
		ID:        uuid.New().String(),
		Type:      "bulk_delete",
		Username:  cmd.Username,
		Status:    "processing",
		Total:     len(cmd.ContentIDs),
		Processed: 0,
		Succeeded: 0,
		Failed:    0,
		StartedAt: time.Now(),
	}

	// Store operation for tracking
	s.operations.Store(operation.ID, operation)

	// Emit start event
	if s.publisher != nil {
		event := s.createOperationEvent("bulk_operation.started", operation)
		_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
		operation.Events = append(operation.Events, event)
	}

	// Process in background to avoid timeout
	go s.processBulkDelete(ctx, operation, cmd)

	return &OperationResult{
		Operation: operation,
		Events:    operation.Events,
	}, nil
}

// finalizeBulkOperation handles completion and cleanup of bulk operation
func (s *Service) finalizeBulkOperation(ctx context.Context, operation *Operation, cmd *DeleteCommand) {
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
}

// validateContentOwnership verifies user owns the content to delete
func (s *Service) validateContentOwnership(ctx context.Context, contentID, username string) (*models.Status, error) {
	status, err := s.statusRepo.GetStatus(ctx, contentID)
	if err != nil {
		s.logger.Debug("bulk content not found",
			zap.String("content_id", contentID),
			zap.String("username", username))
		return nil, errors.Join(serviceerrors.ErrBulkContentNotFound, err)
	}

	if status.AuthorUsername != username {
		s.logger.Warn("unauthorized bulk content deletion",
			zap.String("content_id", contentID),
			zap.String("content_owner", status.AuthorUsername),
			zap.String("requesting_user", username))
		return nil, serviceerrors.ErrBulkContentUnauthorizedDelete
	}

	return status, nil
}

// publishDeletionEvents publishes events for successful content deletion
func (s *Service) publishDeletionEvents(ctx context.Context, contentID, username string) {
	// Emit deletion event for timeline updates
	if s.publisher != nil {
		event := streaming.Event{
			Type:      "content.deleted",
			Stream:    "user",
			Payload:   map[string]interface{}{"id": contentID, "type": StatusStatus},
			Timestamp: time.Now(),
		}
		_ = s.publisher.PublishToUser(ctx, username, &event)
	}

	// Queue federation deletion
	if s.federation != nil {
		activity := s.createDeleteActivity(username, contentID)
		_ = s.federation.QueueActivity(ctx, activity)
	}
}

// processStatusDeletion handles deletion of a single status
func (s *Service) processStatusDeletion(ctx context.Context, contentID string, cmd *DeleteCommand, operation *Operation) {
	// Validate ownership
	_, err := s.validateContentOwnership(ctx, contentID, cmd.Username)
	if err != nil {
		operation.Failed++
		operation.Errors = append(operation.Errors, err.Error())
		return
	}

	// Delete the status
	err = s.statusRepo.DeleteStatus(ctx, contentID)
	if err != nil {
		operation.Failed++
		operation.Errors = append(operation.Errors, fmt.Sprintf("Failed to delete %s: %v", contentID, err))
		return
	}

	operation.Succeeded++
	s.publishDeletionEvents(ctx, contentID, cmd.Username)
}

// processContentDeletion handles deletion of a single content item
func (s *Service) processContentDeletion(ctx context.Context, contentID string, cmd *DeleteCommand, operation *Operation) {
	if cmd.ContentType == "" || cmd.ContentType == StatusStatus {
		s.processStatusDeletion(ctx, contentID, cmd, operation)
	} else {
		operation.Failed++
		operation.Errors = append(operation.Errors, fmt.Sprintf("Unsupported content type: %s", cmd.ContentType))
	}
}

// shouldEmitProgress determines if progress update should be sent
func (s *Service) shouldEmitProgress(operation *Operation) bool {
	return operation.Processed%10 == 0 || operation.Processed == operation.Total
}

// emitProgressUpdate sends progress update to user
func (s *Service) emitProgressUpdate(ctx context.Context, operation *Operation, username string) {
	if s.publisher != nil {
		event := s.createProgressEvent(operation)
		_ = s.publisher.PublishToUser(ctx, username, &event)
	}
}

// processBulkDelete processes bulk content deletion asynchronously
func (s *Service) processBulkDelete(ctx context.Context, operation *Operation, cmd *DeleteCommand) {
	defer s.finalizeBulkOperation(ctx, operation, cmd)

	// Process each content item deletion
	for _, contentID := range cmd.ContentIDs {
		operation.Processed++

		s.processContentDeletion(ctx, contentID, cmd, operation)

		// Emit progress event periodically
		if s.shouldEmitProgress(operation) {
			s.emitProgressUpdate(ctx, operation, cmd.Username)
		}

		// Small delay to avoid overwhelming the system
		if operation.Processed < operation.Total {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// BulkArchive performs bulk content archiving operations
func (s *Service) BulkArchive(ctx context.Context, cmd *ArchiveCommand) (*OperationResult, error) {
	// Create operation tracking
	operation := &Operation{
		ID:        uuid.New().String(),
		Type:      "bulk_archive",
		Username:  cmd.Username,
		Status:    "processing",
		Total:     len(cmd.ContentIDs),
		Processed: 0,
		Succeeded: 0,
		Failed:    0,
		StartedAt: time.Now(),
	}

	// Store operation for tracking
	s.operations.Store(operation.ID, operation)

	// Emit start event
	if s.publisher != nil {
		event := s.createOperationEvent("bulk_operation.started", operation)
		_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
		operation.Events = append(operation.Events, event)
	}

	// Process in background to avoid timeout
	go s.processBulkArchive(ctx, operation, cmd)

	return &OperationResult{
		Operation: operation,
		Events:    operation.Events,
	}, nil
}

// processBulkArchive processes bulk content archiving asynchronously
func (s *Service) processBulkArchive(ctx context.Context, operation *Operation, cmd *ArchiveCommand) {
	defer func() {
		now := time.Now()
		operation.CompletedAt = &now
		operation.Status = StatusCompleted

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

	// Process each content item archiving
	for _, contentID := range cmd.ContentIDs {
		operation.Processed++

		// For now, we only support status archiving
		if cmd.ContentType == "" || cmd.ContentType == StatusStatus {
			// Get status to verify ownership
			status, err := s.statusRepo.GetStatus(ctx, contentID)
			if err != nil {
				operation.Failed++
				operation.Errors = append(operation.Errors, fmt.Sprintf("Content %s not found", contentID))
				continue
			}

			// Verify ownership
			if status.AuthorUsername != cmd.Username {
				operation.Failed++
				operation.Errors = append(operation.Errors, fmt.Sprintf("Not authorized to archive content %s", contentID))
				continue
			}

			// Archive the status by setting visibility to private or a custom archived state
			// For now, we'll just mark it as flagged (which serves as an archive marker)
			status.Flagged = true
			err = s.statusRepo.UpdateStatus(ctx, status)
			if err != nil {
				operation.Failed++
				operation.Errors = append(operation.Errors, fmt.Sprintf("Failed to archive %s: %v", contentID, err))
			} else {
				operation.Succeeded++

				// Emit archive event
				if s.publisher != nil {
					event := streaming.Event{
						Type:      "content.archived",
						Stream:    "user",
						Payload:   map[string]interface{}{"id": contentID, "type": StatusStatus},
						Timestamp: time.Now(),
					}
					_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
				}
			}
		} else {
			operation.Failed++
			operation.Errors = append(operation.Errors, fmt.Sprintf("Unsupported content type: %s", cmd.ContentType))
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

// BulkRestore performs bulk content restoration operations
func (s *Service) BulkRestore(ctx context.Context, cmd *RestoreCommand) (*OperationResult, error) {
	// Create operation tracking
	operation := &Operation{
		ID:        uuid.New().String(),
		Type:      "bulk_restore",
		Username:  cmd.Username,
		Status:    "processing",
		Total:     len(cmd.ContentIDs),
		Processed: 0,
		Succeeded: 0,
		Failed:    0,
		StartedAt: time.Now(),
	}

	// Store operation for tracking
	s.operations.Store(operation.ID, operation)

	// Emit start event
	if s.publisher != nil {
		event := s.createOperationEvent("bulk_operation.started", operation)
		_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
		operation.Events = append(operation.Events, event)
	}

	// Process in background to avoid timeout
	go s.processBulkRestore(ctx, operation, cmd)

	return &OperationResult{
		Operation: operation,
		Events:    operation.Events,
	}, nil
}

// processBulkRestore processes bulk content restoration asynchronously
func (s *Service) processBulkRestore(ctx context.Context, operation *Operation, cmd *RestoreCommand) {
	defer func() {
		now := time.Now()
		operation.CompletedAt = &now
		operation.Status = StatusCompleted

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

	// Process each content item restoration
	for _, contentID := range cmd.ContentIDs {
		operation.Processed++

		// For now, we only support status restoration
		if cmd.ContentType == "" || cmd.ContentType == StatusStatus {
			// Get status to verify ownership
			status, err := s.statusRepo.GetStatus(ctx, contentID)
			if err != nil {
				operation.Failed++
				operation.Errors = append(operation.Errors, fmt.Sprintf("Content %s not found", contentID))
				continue
			}

			// Verify ownership
			if status.AuthorUsername != cmd.Username {
				operation.Failed++
				operation.Errors = append(operation.Errors, fmt.Sprintf("Not authorized to restore content %s", contentID))
				continue
			}

			// Restore the status by removing archive markers
			status.Flagged = false
			status.Deleted = false
			status.DeletedAt = nil
			err = s.statusRepo.UpdateStatus(ctx, status)
			if err != nil {
				operation.Failed++
				operation.Errors = append(operation.Errors, fmt.Sprintf("Failed to restore %s: %v", contentID, err))
			} else {
				operation.Succeeded++

				// Emit restore event
				if s.publisher != nil {
					event := streaming.Event{
						Type:      "content.restored",
						Stream:    "user",
						Payload:   map[string]interface{}{"id": contentID, "type": StatusStatus},
						Timestamp: time.Now(),
					}
					_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
				}
			}
		} else {
			operation.Failed++
			operation.Errors = append(operation.Errors, fmt.Sprintf("Unsupported content type: %s", cmd.ContentType))
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

// BulkExport performs bulk content export operations
func (s *Service) BulkExport(ctx context.Context, cmd *ExportCommand) (*OperationResult, error) {
	// Create operation tracking
	operation := &Operation{
		ID:        uuid.New().String(),
		Type:      "bulk_export",
		Username:  cmd.Username,
		Status:    "processing",
		Total:     len(cmd.ContentIDs),
		Processed: 0,
		Succeeded: 0,
		Failed:    0,
		StartedAt: time.Now(),
	}

	// Store operation for tracking
	s.operations.Store(operation.ID, operation)

	// Emit start event
	if s.publisher != nil {
		event := s.createOperationEvent("bulk_operation.started", operation)
		_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
		operation.Events = append(operation.Events, event)
	}

	// Process in background to avoid timeout
	go s.processBulkExport(ctx, operation, cmd)

	return &OperationResult{
		Operation: operation,
		Events:    operation.Events,
	}, nil
}

// processBulkExport processes bulk content export asynchronously
func (s *Service) processBulkExport(ctx context.Context, operation *Operation, cmd *ExportCommand) {
	defer func() {
		now := time.Now()
		operation.CompletedAt = &now
		operation.Status = StatusCompleted

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

	// Collect exported data
	var exportedData []interface{}

	// Process each content item for export
	for _, contentID := range cmd.ContentIDs {
		operation.Processed++

		// For now, we only support status export
		if cmd.ContentType == "" || cmd.ContentType == StatusStatus {
			// Get status to verify ownership
			status, err := s.statusRepo.GetStatus(ctx, contentID)
			if err != nil {
				operation.Failed++
				operation.Errors = append(operation.Errors, fmt.Sprintf("Content %s not found", contentID))
				continue
			}

			// Verify ownership
			if status.AuthorUsername != cmd.Username {
				operation.Failed++
				operation.Errors = append(operation.Errors, fmt.Sprintf("Not authorized to export content %s", contentID))
				continue
			}

			// Format data based on export format
			var exportItem interface{}
			switch cmd.Format {
			case "json":
				exportItem = status
			case "csv":
				// For CSV, we'll export simplified data
				exportItem = map[string]interface{}{
					"id":           status.StatusID,
					"content":      status.Content,
					"published_at": status.PublishedAt,
					"visibility":   status.Visibility,
					"like_count":   status.LikeCount,
					"reblog_count": status.ReblogCount,
					"reply_count":  status.ReplyCount,
				}
			case "activitypub":
				exportItem = status.Note
			}

			exportedData = append(exportedData, exportItem)
			operation.Succeeded++
		} else {
			operation.Failed++
			operation.Errors = append(operation.Errors, fmt.Sprintf("Unsupported content type: %s", cmd.ContentType))
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

	// For a complete implementation, you would save the exported data to S3 or another storage
	// and provide a download URL in the completion event
	if s.publisher != nil {
		event := streaming.Event{
			Type:   "bulk_export.completed",
			Stream: "user",
			Payload: map[string]interface{}{
				"operation_id": operation.ID,
				"format":       cmd.Format,
				"item_count":   len(exportedData),
				"download_url": fmt.Sprintf("/exports/%s", operation.ID), // Placeholder URL
			},
			Timestamp: time.Now(),
		}
		_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
	}
}

// BulkListMembers performs bulk list member operations (add/remove)
func (s *Service) BulkListMembers(ctx context.Context, cmd *ListMembersCommand) (*OperationResult, error) {
	// Create operation tracking
	operation := &Operation{
		ID:        fmt.Sprintf("bulk_list_members_%s_%d", cmd.Operation, time.Now().Unix()),
		Type:      fmt.Sprintf("bulk_list_members_%s", cmd.Operation),
		Status:    "processing",
		Total:     len(cmd.AccountIDs),
		Username:  cmd.Username,
		StartedAt: time.Now(),
	}

	// Store the operation for tracking
	s.operations.Store(operation.ID, operation)

	// Launch async processing
	go s.processBulkListMembers(ctx, cmd, operation)

	// Return operation result
	return &OperationResult{
		Operation: operation,
	}, nil
}

// processBulkListMembers handles the actual bulk list member processing
func (s *Service) processBulkListMembers(ctx context.Context, cmd *ListMembersCommand, operation *Operation) {
	defer func() {
		operation.Status = "completed"
		now := time.Now()
		operation.CompletedAt = &now

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

	// Process each account ID
	for _, accountID := range cmd.AccountIDs {
		operation.Processed++

		var err error
		switch cmd.Operation {
		case "add":
			err = s.listRepo.AddListMember(ctx, cmd.ListID, accountID)
		case "remove":
			err = s.listRepo.RemoveListMember(ctx, cmd.ListID, accountID)
		default:
			operation.Failed++
			operation.Errors = append(operation.Errors, fmt.Sprintf("Invalid operation: %s", cmd.Operation))
			continue
		}

		if err != nil {
			operation.Failed++
			operation.Errors = append(operation.Errors, fmt.Sprintf("Failed to %s account %s: %v", cmd.Operation, accountID, err))
			s.logger.Error("bulk list member operation failed",
				zap.String("operation", cmd.Operation),
				zap.String("list_id", cmd.ListID),
				zap.String("account_id", accountID),
				zap.Error(err))
		} else {
			operation.Succeeded++

			// Emit individual member event
			if s.publisher != nil {
				event := streaming.Event{
					Type:   fmt.Sprintf("list_member.%s", strings.TrimSuffix(cmd.Operation, "d")+"ed"), // add->added, remove->removed
					Stream: "user",
					Payload: map[string]interface{}{
						"list_id":    cmd.ListID,
						"account_id": accountID,
						"operation":  cmd.Operation,
					},
					Timestamp: time.Now(),
				}
				_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
			}
		}

		// Progress update for long operations
		if operation.Processed%10 == 0 || operation.Processed == operation.Total {
			if s.publisher != nil {
				event := s.createOperationEvent("bulk_operation.progress", operation)
				_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
			}
		}

		// Small delay to avoid overwhelming the system
		if operation.Processed < operation.Total {
			time.Sleep(50 * time.Millisecond)
		}
	}
}

// BulkUnblock performs bulk account unblocking operations
func (s *Service) BulkUnblock(ctx context.Context, cmd *UnblockCommand) (*OperationResult, error) {
	// Create operation tracking
	operation := &Operation{
		ID:        uuid.New().String(),
		Type:      "bulk_unblock",
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

	// Emit start event
	if s.publisher != nil {
		event := s.createOperationEvent("bulk_operation.started", operation)
		_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
		operation.Events = append(operation.Events, event)
	}

	// Process in background to avoid timeout
	go s.processBulkUnblock(ctx, operation, cmd)

	return &OperationResult{
		Operation: operation,
		Events:    operation.Events,
	}, nil
}

// processBulkUnblock processes bulk unblock operations asynchronously
func (s *Service) processBulkUnblock(ctx context.Context, operation *Operation, cmd *UnblockCommand) {
	defer s.finalizeModerationOperation(ctx, operation, cmd.Username)

	// Process each unblock
	for _, accountID := range cmd.AccountIDs {
		operation.Processed++

		// Remove block relationship using relationship repository
		err := s.relationshipRepo.DeleteBlock(ctx, cmd.Username, accountID)

		if err != nil {
			operation.Failed++
			operation.Errors = append(operation.Errors, fmt.Sprintf("Failed to unblock %s: %v", accountID, err))
			s.logger.Warn("failed to unblock account in bulk operation",
				zap.String("username", cmd.Username),
				zap.String("target", accountID),
				zap.Error(err))
		} else {
			operation.Succeeded++

			// Emit unblock event
			if s.publisher != nil {
				event := streaming.Event{
					Type:      "relationship.unblock",
					Stream:    "user",
					Payload:   map[string]interface{}{"account_id": accountID},
					Timestamp: time.Now(),
				}
				_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
			}

			// Queue federation activity
			if s.federation != nil {
				activity := s.createUndoActivity(cmd.Username, accountID, "Block")
				_ = s.federation.QueueActivity(ctx, activity)
			}
		}

		// Emit progress update
		if s.shouldEmitProgress(operation) {
			s.emitProgressUpdate(ctx, operation, cmd.Username)
		}

		// Small delay
		if operation.Processed < operation.Total {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// BulkMute performs bulk account muting operations
func (s *Service) BulkMute(ctx context.Context, cmd *MuteCommand) (*OperationResult, error) {
	// Create operation tracking
	operation := &Operation{
		ID:        uuid.New().String(),
		Type:      "bulk_mute",
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

	// Emit start event
	if s.publisher != nil {
		event := s.createOperationEvent("bulk_operation.started", operation)
		_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
		operation.Events = append(operation.Events, event)
	}

	// Process in background to avoid timeout
	go s.processBulkMute(ctx, operation, cmd)

	return &OperationResult{
		Operation: operation,
		Events:    operation.Events,
	}, nil
}

// processBulkMute processes bulk mute operations asynchronously
func (s *Service) processBulkMute(ctx context.Context, operation *Operation, cmd *MuteCommand) {
	defer s.finalizeModerationOperation(ctx, operation, cmd.Username)

	// Process each mute
	for _, accountID := range cmd.AccountIDs {
		operation.Processed++

		// Create mute relationship using relationship repository
		activityID := fmt.Sprintf("https://%s/users/%s/mutes/%s", s.domain, cmd.Username, accountID)
		err := s.relationshipRepo.CreateMute(ctx, cmd.Username, accountID, activityID, cmd.Notifications, cmd.Duration)

		if err != nil {
			operation.Failed++
			operation.Errors = append(operation.Errors, fmt.Sprintf("Failed to mute %s: %v", accountID, err))
			s.logger.Warn("failed to mute account in bulk operation",
				zap.String("username", cmd.Username),
				zap.String("target", accountID),
				zap.Error(err))
		} else {
			operation.Succeeded++

			// Emit mute event
			if s.publisher != nil {
				event := streaming.Event{
					Type:   "relationship.mute",
					Stream: "user",
					Payload: map[string]interface{}{
						"account_id":    accountID,
						"notifications": cmd.Notifications,
						"duration":      cmd.Duration,
					},
					Timestamp: time.Now(),
				}
				_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
			}
		}

		// Emit progress update
		if s.shouldEmitProgress(operation) {
			s.emitProgressUpdate(ctx, operation, cmd.Username)
		}

		// Small delay
		if operation.Processed < operation.Total {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// BulkBlock performs bulk account blocking operations
func (s *Service) BulkBlock(ctx context.Context, cmd *BlockCommand) (*OperationResult, error) {
	// Create operation tracking
	operation := &Operation{
		ID:        uuid.New().String(),
		Type:      "bulk_block",
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

	// Emit start event
	if s.publisher != nil {
		event := s.createOperationEvent("bulk_operation.started", operation)
		_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
		operation.Events = append(operation.Events, event)
	}

	// Process in background to avoid timeout
	go s.processBulkBlock(ctx, operation, cmd)

	return &OperationResult{
		Operation: operation,
		Events:    operation.Events,
	}, nil
}

// processBulkBlock processes bulk block operations asynchronously
func (s *Service) processBulkBlock(ctx context.Context, operation *Operation, cmd *BlockCommand) {
	defer s.finalizeModerationOperation(ctx, operation, cmd.Username)

	// Process each block
	for _, accountID := range cmd.AccountIDs {
		operation.Processed++

		// Create block relationship using relationship repository
		activityID := fmt.Sprintf("https://%s/users/%s/blocks/%s", s.domain, cmd.Username, accountID)
		err := s.relationshipRepo.CreateBlock(ctx, cmd.Username, accountID, activityID)

		if err != nil {
			operation.Failed++
			operation.Errors = append(operation.Errors, fmt.Sprintf("Failed to block %s: %v", accountID, err))
			s.logger.Warn("failed to block account in bulk operation",
				zap.String("username", cmd.Username),
				zap.String("target", accountID),
				zap.Error(err))
		} else {
			operation.Succeeded++

			// Emit block event
			if s.publisher != nil {
				event := streaming.Event{
					Type:      "relationship.block",
					Stream:    "user",
					Payload:   map[string]interface{}{"account_id": accountID},
					Timestamp: time.Now(),
				}
				_ = s.publisher.PublishToUser(ctx, cmd.Username, &event)
			}

			// Queue federation activity
			if s.federation != nil {
				activity := s.createBlockActivity(cmd.Username, accountID)
				_ = s.federation.QueueActivity(ctx, activity)
			}
		}

		// Emit progress update
		if s.shouldEmitProgress(operation) {
			s.emitProgressUpdate(ctx, operation, cmd.Username)
		}

		// Small delay
		if operation.Processed < operation.Total {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// Helper methods for moderation operations

// finalizeModerationOperation handles completion and cleanup of moderation operations
func (s *Service) finalizeModerationOperation(ctx context.Context, operation *Operation, username string) {
	now := time.Now()
	operation.CompletedAt = &now
	operation.Status = StatusCompleted

	// Emit completion event
	if s.publisher != nil {
		event := s.createOperationEvent("bulk_operation.completed", operation)
		_ = s.publisher.PublishToUser(ctx, username, &event)
	}

	// Clean up after 1 hour
	time.AfterFunc(time.Hour, func() {
		s.operations.Delete(operation.ID)
	})
}

// createUndoActivity creates an Undo activity for reversing actions
func (s *Service) createUndoActivity(username, targetID, actionType string) *activitypub.Activity {
	actorURL := fmt.Sprintf("https://%s/users/%s", s.domain, username)
	targetURL := fmt.Sprintf("https://%s/users/%s", s.domain, targetID)

	// Create the original activity to undo
	originalActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   fmt.Sprintf("%s/%ss/%s", actorURL, strings.ToLower(actionType), targetID),
			Type: actionType,
		},
		Actor:  actorURL,
		Object: targetURL,
	}

	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   fmt.Sprintf("%s/undo/%s/%s", actorURL, strings.ToLower(actionType), targetID),
			Type: "Undo",
		},
		Actor:  actorURL,
		Object: originalActivity,
	}
}

// createBlockActivity creates a Block activity for federation
func (s *Service) createBlockActivity(username, targetID string) *activitypub.Activity {
	actorURL := fmt.Sprintf("https://%s/users/%s", s.domain, username)
	targetURL := fmt.Sprintf("https://%s/users/%s", s.domain, targetID)

	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   fmt.Sprintf("%s/blocks/%s", actorURL, targetID),
			Type: "Block",
		},
		Actor:  actorURL,
		Object: targetURL,
	}
}
