// Package lists provides the core Lists Service for the Lesser project's API alignment.
// This service handles all list operations including creation, management, membership,
// and timeline generation. It emits appropriate events for real-time streaming.
package lists

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	serviceerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// Service provides list operations
type Service struct {
	listRepo   interfaces.ListRepository
	statusRepo interfaces.StatusRepository
	publisher  streaming.Publisher
	logger     *zap.Logger
}

// NewService creates a new Lists Service with the required dependencies
func NewService(
	listRepo interfaces.ListRepository,
	statusRepo interfaces.StatusRepository,
	publisher streaming.Publisher,
	logger *zap.Logger,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		listRepo:   listRepo,
		statusRepo: statusRepo,
		publisher:  publisher,
		logger:     logger,
	}
}

// Command structs for operations

// CreateListCommand contains all data needed to create a new list
type CreateListCommand struct {
	Username      string `json:"username" validate:"required"`
	Title         string `json:"title" validate:"required,min=1,max=100"`
	RepliesPolicy string `json:"replies_policy" validate:"oneof=followed list none"`
	CreatorID     string `json:"creator_id" validate:"required"` // Must be the list owner
}

// UpdateListCommand contains data needed to update an existing list
type UpdateListCommand struct {
	ListID        string `json:"list_id" validate:"required"`
	Title         string `json:"title,omitempty" validate:"omitempty,min=1,max=100"`
	RepliesPolicy string `json:"replies_policy,omitempty" validate:"omitempty,oneof=followed list none"`
	UpdaterID     string `json:"updater_id" validate:"required"` // Must be the list owner
}

// DeleteListCommand contains data needed to delete a list
type DeleteListCommand struct {
	ListID    string `json:"list_id" validate:"required"`
	DeleterID string `json:"deleter_id" validate:"required"` // Must be the list owner
}

// AddToListCommand contains data needed to add a member to a list
type AddToListCommand struct {
	ListID         string `json:"list_id" validate:"required"`
	MemberUsername string `json:"member_username" validate:"required"`
	AdderID        string `json:"adder_id" validate:"required"` // Must be the list owner
}

// RemoveFromListCommand contains data needed to remove a member from a list
type RemoveFromListCommand struct {
	ListID         string `json:"list_id" validate:"required"`
	MemberUsername string `json:"member_username" validate:"required"`
	RemoverID      string `json:"remover_id" validate:"required"` // Must be the list owner
}

// GetListQuery contains parameters for retrieving a list
type GetListQuery struct {
	ListID   string `json:"list_id" validate:"required"`
	ViewerID string `json:"viewer_id"` // User requesting the list (for privacy checks)
}

// ListUserListsQuery contains parameters for listing a user's lists
type ListUserListsQuery struct {
	Username   string                       `json:"username" validate:"required"`
	ViewerID   string                       `json:"viewer_id"` // User requesting the lists
	Pagination interfaces.PaginationOptions `json:"pagination"`
}

// GetListTimelineQuery contains parameters for retrieving a list timeline
type GetListTimelineQuery struct {
	ListID     string                       `json:"list_id" validate:"required"`
	ViewerID   string                       `json:"viewer_id" validate:"required"` // Must be list owner or member
	Pagination interfaces.PaginationOptions `json:"pagination"`
}

// GetListMembersQuery contains parameters for retrieving list members
type GetListMembersQuery struct {
	ListID     string                       `json:"list_id" validate:"required"`
	ViewerID   string                       `json:"viewer_id" validate:"required"` // Must be list owner
	Pagination interfaces.PaginationOptions `json:"pagination"`
}

// Result structs for operations

// ListResult contains a list and associated events that were emitted
type ListResult struct {
	List   *models.List       `json:"list"`
	Events []*streaming.Event `json:"events"`
}

// Result contains multiple lists with pagination and events
type Result struct {
	Lists      []*models.List                            `json:"lists"`
	Pagination *interfaces.PaginatedResult[*models.List] `json:"pagination"`
	Events     []*streaming.Event                        `json:"events"`
}

// TimelineResult contains timeline posts with pagination and events
type TimelineResult struct {
	Statuses   []*models.Status                            `json:"statuses"`
	Pagination *interfaces.PaginatedResult[*models.Status] `json:"pagination"`
	Events     []*streaming.Event                          `json:"events"`
}

// MembershipResult contains membership operation result and events
type MembershipResult struct {
	Success bool               `json:"success"`
	Events  []*streaming.Event `json:"events"`
}

// MembersResult contains list members with pagination and events
type MembersResult struct {
	Members    []*storage.Account                            `json:"members"`
	Pagination *interfaces.PaginatedResult[*storage.Account] `json:"pagination"`
	Events     []*streaming.Event                            `json:"events"`
}

// Core service methods

// CreateList creates a new list, validates input, stores it, and emits events
func (s *Service) CreateList(ctx context.Context, cmd *CreateListCommand) (*ListResult, error) {
	s.logger.Info("creating list",
		zap.String("username", cmd.Username),
		zap.String("title", cmd.Title),
		zap.String("creator_id", cmd.CreatorID))

	// Validate the command
	if err := s.validateCreateListCommand(ctx, cmd); err != nil {
		return nil, serviceerrors.ErrListValidationFailed
	}

	// Verify permission (only user can create their own lists)
	if cmd.Username != cmd.CreatorID {
		return nil, common.ErrForbidden(serviceerrors.ErrListUnauthorizedCreate)
	}

	// Generate a unique list ID
	listID := s.generateListID()

	// Set default replies policy if not specified
	repliesPolicy := cmd.RepliesPolicy
	if err := common.ValidateRequiredParam("repliesPolicy", repliesPolicy); err != nil {
		repliesPolicy = "list"
	}

	// Create the list model
	list := &models.List{
		ID:            listID,
		Username:      cmd.Username,
		Title:         cmd.Title,
		RepliesPolicy: repliesPolicy,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Store the list
	if err := s.listRepo.CreateList(ctx, list); err != nil {
		return nil, serviceerrors.ErrListCreateFailed
	}

	s.logger.Info("created list successfully",
		zap.String("list_id", listID),
		zap.String("username", cmd.Username))

	// Emit events
	events := s.emitListCreatedEvents(ctx, list)

	return &ListResult{
		List:   list,
		Events: events,
	}, nil
}

// UpdateList updates an existing list and emits events
func (s *Service) UpdateList(ctx context.Context, cmd *UpdateListCommand) (*ListResult, error) {
	s.logger.Info("updating list",
		zap.String("list_id", cmd.ListID),
		zap.String("updater_id", cmd.UpdaterID))

	// Validate the command
	if err := s.validateUpdateListCommand(ctx, cmd); err != nil {
		return nil, serviceerrors.ErrListValidationFailed
	}

	// Get existing list
	list, err := s.listRepo.GetList(ctx, cmd.ListID)
	if err != nil {
		return nil, serviceerrors.ErrListGetFailed
	}

	// Verify permission (only list owner can update)
	if list.Username != cmd.UpdaterID {
		return nil, common.ErrForbidden(serviceerrors.ErrListUnauthorizedUpdate)
	}

	// Update fields that were provided
	updated := false
	if cmd.Title != "" && cmd.Title != list.Title {
		list.Title = cmd.Title
		updated = true
	}
	if cmd.RepliesPolicy != "" && cmd.RepliesPolicy != list.RepliesPolicy {
		list.RepliesPolicy = cmd.RepliesPolicy
		updated = true
	}

	// Only update if there were changes
	if !updated {
		s.logger.Debug("no changes to list", zap.String("list_id", cmd.ListID))
		return &ListResult{
			List:   list,
			Events: []*streaming.Event{},
		}, nil
	}

	// Update timestamp
	list.UpdatedAt = time.Now()

	// Store the updated list
	if err := s.listRepo.UpdateList(ctx, list); err != nil {
		return nil, serviceerrors.ErrListUpdateFailed
	}

	s.logger.Info("updated list successfully",
		zap.String("list_id", cmd.ListID))

	// Emit events
	events := s.emitListUpdatedEvents(ctx, list)

	return &ListResult{
		List:   list,
		Events: events,
	}, nil
}

// DeleteList deletes a list and emits events
func (s *Service) DeleteList(ctx context.Context, cmd *DeleteListCommand) error {
	s.logger.Info("deleting list",
		zap.String("list_id", cmd.ListID),
		zap.String("deleter_id", cmd.DeleterID))

	// Validate the command
	if err := s.validateDeleteListCommand(ctx, cmd); err != nil {
		return serviceerrors.ErrListValidationFailed
	}

	// Get existing list
	list, err := s.listRepo.GetList(ctx, cmd.ListID)
	if err != nil {
		return serviceerrors.ErrListGetFailed
	}

	// Verify permission (only list owner can delete)
	if list.Username != cmd.DeleterID {
		return common.ErrForbidden(serviceerrors.ErrListUnauthorizedDelete)
	}

	// Delete the list
	if err := s.listRepo.DeleteList(ctx, cmd.ListID); err != nil {
		return serviceerrors.ErrListDeleteFailed
	}

	s.logger.Info("deleted list successfully",
		zap.String("list_id", cmd.ListID))

	// Emit events
	s.emitListDeletedEvents(ctx, list)

	return nil
}

// AddToList adds an account to a list and emits events
func (s *Service) AddToList(ctx context.Context, cmd *AddToListCommand) (*MembershipResult, error) {
	s.logger.Info("adding to list",
		zap.String("list_id", cmd.ListID),
		zap.String("member_username", cmd.MemberUsername),
		zap.String("adder_id", cmd.AdderID))

	// Validate the command
	if err := s.validateAddToListCommand(ctx, cmd); err != nil {
		return nil, serviceerrors.ErrListValidationFailed
	}

	// Get existing list
	list, err := s.listRepo.GetList(ctx, cmd.ListID)
	if err != nil {
		return nil, serviceerrors.ErrListGetFailed
	}

	// Verify permission (only list owner can add members)
	if list.Username != cmd.AdderID {
		return nil, common.ErrForbidden(serviceerrors.ErrListUnauthorizedAddMember)
	}

	// Check if already a member
	isMember, err := s.listRepo.IsListMember(ctx, cmd.ListID, cmd.MemberUsername)
	if err != nil {
		return nil, serviceerrors.ErrListMembershipCheckFailed
	}

	if isMember {
		s.logger.Debug("user already in list",
			zap.String("list_id", cmd.ListID),
			zap.String("member_username", cmd.MemberUsername))
		return &MembershipResult{
			Success: true,
			Events:  []*streaming.Event{},
		}, nil
	}

	// Add member to list
	if err := s.listRepo.AddListMember(ctx, cmd.ListID, cmd.MemberUsername); err != nil {
		return nil, serviceerrors.ErrListMemberAddFailed
	}

	s.logger.Info("added member to list successfully",
		zap.String("list_id", cmd.ListID),
		zap.String("member_username", cmd.MemberUsername))

	// Emit events
	events := s.emitMemberAddedEvents(ctx, list, cmd.MemberUsername)

	return &MembershipResult{
		Success: true,
		Events:  events,
	}, nil
}

// RemoveFromList removes an account from a list and emits events
func (s *Service) RemoveFromList(ctx context.Context, cmd *RemoveFromListCommand) (*MembershipResult, error) {
	s.logger.Info("removing from list",
		zap.String("list_id", cmd.ListID),
		zap.String("member_username", cmd.MemberUsername),
		zap.String("remover_id", cmd.RemoverID))

	// Validate the command
	if err := s.validateRemoveFromListCommand(ctx, cmd); err != nil {
		return nil, serviceerrors.ErrListValidationFailed
	}

	// Get existing list
	list, err := s.listRepo.GetList(ctx, cmd.ListID)
	if err != nil {
		return nil, serviceerrors.ErrListGetFailed
	}

	// Verify permission (only list owner can remove members)
	if list.Username != cmd.RemoverID {
		return nil, common.ErrForbidden(serviceerrors.ErrListUnauthorizedRemoveMember)
	}

	// Check if actually a member
	isMember, err := s.listRepo.IsListMember(ctx, cmd.ListID, cmd.MemberUsername)
	if err != nil {
		return nil, serviceerrors.ErrListMembershipCheckFailed
	}

	if !isMember {
		s.logger.Debug("user not in list",
			zap.String("list_id", cmd.ListID),
			zap.String("member_username", cmd.MemberUsername))
		return &MembershipResult{
			Success: true,
			Events:  []*streaming.Event{},
		}, nil
	}

	// Remove member from list
	if err := s.listRepo.RemoveListMember(ctx, cmd.ListID, cmd.MemberUsername); err != nil {
		return nil, serviceerrors.ErrListMemberRemoveFailed
	}

	s.logger.Info("removed member from list successfully",
		zap.String("list_id", cmd.ListID),
		zap.String("member_username", cmd.MemberUsername))

	// Emit events
	events := s.emitMemberRemovedEvents(ctx, list, cmd.MemberUsername)

	return &MembershipResult{
		Success: true,
		Events:  events,
	}, nil
}

// GetList retrieves a single list with privacy checks
func (s *Service) GetList(ctx context.Context, query *GetListQuery) (*models.List, error) {
	s.logger.Debug("getting list",
		zap.String("list_id", query.ListID),
		zap.String("viewer_id", query.ViewerID))

	// Get the list
	list, err := s.listRepo.GetList(ctx, query.ListID)
	if err != nil {
		return nil, serviceerrors.ErrListGetFailed
	}

	// Apply privacy checks - only list owner can view their lists
	if query.ViewerID != list.Username {
		return nil, common.ErrForbidden(serviceerrors.ErrListUnauthorizedView)
	}

	return list, nil
}

// ListUserLists retrieves a user's lists with pagination
func (s *Service) ListUserLists(ctx context.Context, query *ListUserListsQuery) (*Result, error) {
	s.logger.Debug("listing user lists",
		zap.String("username", query.Username),
		zap.String("viewer_id", query.ViewerID))

	// Privacy check - only users can view their own lists
	if query.ViewerID != query.Username {
		return nil, common.ErrForbidden(serviceerrors.ErrListUnauthorizedViewLists)
	}

	// Get user's lists
	result, err := s.listRepo.GetUserLists(ctx, query.Username, query.Pagination)
	if err != nil {
		return nil, serviceerrors.ErrGetUserLists
	}

	return &Result{
		Lists:      result.Items,
		Pagination: result,
		Events:     []*streaming.Event{}, // No events for read operations
	}, nil
}

// GetListTimeline retrieves posts from list members with pagination
func (s *Service) GetListTimeline(ctx context.Context, query *GetListTimelineQuery) (*TimelineResult, error) {
	s.logger.Debug("getting list timeline",
		zap.String("list_id", query.ListID),
		zap.String("viewer_id", query.ViewerID))

	// Get the list
	list, err := s.listRepo.GetList(ctx, query.ListID)
	if err != nil {
		return nil, serviceerrors.ErrListGetFailed
	}

	// Verify permission (only list owner can view timeline)
	if query.ViewerID != list.Username {
		return nil, common.ErrForbidden(serviceerrors.ErrListUnauthorizedViewTimeline)
	}

	// Get list timeline
	result, err := s.listRepo.GetListTimeline(ctx, query.ListID, query.Pagination)
	if err != nil {
		return nil, serviceerrors.ErrGetListTimeline
	}

	return &TimelineResult{
		Statuses:   result.Items,
		Pagination: result,
		Events:     []*streaming.Event{}, // No events for read operations
	}, nil
}

// Private helper methods

func (s *Service) generateListID() string {
	return fmt.Sprintf("list_%d", time.Now().UnixNano())
}

func (s *Service) validateCreateListCommand(_ context.Context, cmd *CreateListCommand) error {
	if err := common.ValidateRequiredParam("username", cmd.Username); err != nil {
		return serviceerrors.ErrListUsernameRequired
	}
	if err := common.ValidateRequiredParam("creator_id", cmd.CreatorID); err != nil {
		return serviceerrors.ErrListCreatorIDRequired
	}
	if err := common.ValidateRequiredParam("title", strings.TrimSpace(cmd.Title)); err != nil {
		return serviceerrors.ErrListTitleRequired
	}
	if err := common.ValidateListTitle(cmd.Title); err != nil {
		return err
	}
	if cmd.RepliesPolicy != "" {
		if err := common.ValidateListRepliesPolicy(cmd.RepliesPolicy); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) validateUpdateListCommand(_ context.Context, cmd *UpdateListCommand) error {
	if err := common.ValidateRequiredParam("list_id", cmd.ListID); err != nil {
		return serviceerrors.ErrListIDRequired
	}
	if err := common.ValidateRequiredParam("updater_id", cmd.UpdaterID); err != nil {
		return serviceerrors.ErrListUpdaterIDRequired
	}
	if cmd.Title != "" {
		if err := common.ValidateRequiredParam("title", strings.TrimSpace(cmd.Title)); err != nil {
			return serviceerrors.ErrListTitleEmpty
		}
		if err := common.ValidateListTitle(cmd.Title); err != nil {
			return err
		}
	}
	if cmd.RepliesPolicy != "" {
		if err := common.ValidateListRepliesPolicy(cmd.RepliesPolicy); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) validateDeleteListCommand(_ context.Context, cmd *DeleteListCommand) error {
	if err := common.ValidateRequiredParam("list_id", cmd.ListID); err != nil {
		return serviceerrors.ErrListIDRequired
	}
	if err := common.ValidateRequiredParam("deleter_id", cmd.DeleterID); err != nil {
		return serviceerrors.ErrListDeleterIDRequired
	}
	return nil
}

func (s *Service) validateAddToListCommand(_ context.Context, cmd *AddToListCommand) error {
	if err := common.ValidateRequiredParam("list_id", cmd.ListID); err != nil {
		return serviceerrors.ErrListIDRequired
	}
	if err := common.ValidateRequiredParam("member_username", cmd.MemberUsername); err != nil {
		return serviceerrors.ErrListMemberUsernameRequired
	}
	if err := common.ValidateRequiredParam("adder_id", cmd.AdderID); err != nil {
		return serviceerrors.ErrListAdderIDRequired
	}
	return nil
}

func (s *Service) validateRemoveFromListCommand(_ context.Context, cmd *RemoveFromListCommand) error {
	if err := common.ValidateRequiredParam("list_id", cmd.ListID); err != nil {
		return serviceerrors.ErrListIDRequired
	}
	if err := common.ValidateRequiredParam("member_username", cmd.MemberUsername); err != nil {
		return serviceerrors.ErrListMemberUsernameRequired
	}
	if err := common.ValidateRequiredParam("remover_id", cmd.RemoverID); err != nil {
		return serviceerrors.ErrListRemoverIDRequired
	}
	return nil
}

// Event emission methods

func (s *Service) emitListCreatedEvents(ctx context.Context, list *models.List) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      "list.created",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"list": list,
		},
	}

	// Emit to list owner's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", list.Username)
	if err := s.publisher.PublishToUser(ctx, list.Username, &userEvent); err != nil {
		s.logger.Error("failed to publish list created event to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}

func (s *Service) emitListUpdatedEvents(ctx context.Context, list *models.List) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      "list.updated",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"list": list,
		},
	}

	// Emit to list owner's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", list.Username)
	if err := s.publisher.PublishToUser(ctx, list.Username, &userEvent); err != nil {
		s.logger.Error("failed to publish list updated event to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}

func (s *Service) emitListDeletedEvents(ctx context.Context, list *models.List) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      "list.deleted",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"list_id": list.ID,
			"list":    list,
		},
	}

	// Emit to list owner's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", list.Username)
	if err := s.publisher.PublishToUser(ctx, list.Username, &userEvent); err != nil {
		s.logger.Error("failed to publish list deleted event to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}

func (s *Service) emitMemberAddedEvents(ctx context.Context, list *models.List, memberUsername string) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      "list.member_added",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"list_id":         list.ID,
			"member_username": memberUsername,
			"list":            list,
		},
	}

	// Emit to list owner's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", list.Username)
	if err := s.publisher.PublishToUser(ctx, list.Username, &userEvent); err != nil {
		s.logger.Error("failed to publish member added event to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}

// GetListMembers retrieves all members of a list with pagination
func (s *Service) GetListMembers(ctx context.Context, query *GetListMembersQuery) (*MembersResult, error) {
	s.logger.Debug("getting list members",
		zap.String("list_id", query.ListID),
		zap.String("viewer_id", query.ViewerID))

	// Get the list to verify ownership
	list, err := s.listRepo.GetList(ctx, query.ListID)
	if err != nil {
		return nil, serviceerrors.ErrListNotFound
	}

	// Verify permission (only owner can see members)
	if list.Username != query.ViewerID {
		return nil, common.ErrForbidden(serviceerrors.ErrListUnauthorizedViewMembers)
	}

	// Get list members from repository
	membersResult, err := s.listRepo.GetListMembers(ctx, query.ListID, query.Pagination)
	if err != nil {
		return nil, serviceerrors.ErrGetListMembers
	}

	return &MembersResult{
		Members:    membersResult.Items,
		Pagination: membersResult,
		Events:     []*streaming.Event{}, // No events for reads
	}, nil
}

func (s *Service) emitMemberRemovedEvents(ctx context.Context, list *models.List, memberUsername string) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      "list.member_removed",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"list_id":         list.ID,
			"member_username": memberUsername,
			"list":            list,
		},
	}

	// Emit to list owner's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", list.Username)
	if err := s.publisher.PublishToUser(ctx, list.Username, &userEvent); err != nil {
		s.logger.Error("failed to publish member removed event to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}
