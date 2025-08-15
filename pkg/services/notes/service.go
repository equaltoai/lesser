// Package notes provides the core Notes Service for the Lesser project's API alignment.
// This service handles all status/post operations including creation, updates, deletion,
// and timeline operations. It emits appropriate events for real-time streaming and
// queues federation delivery for remote followers.
package notes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// VisibilityPublic represents public visibility
	VisibilityPublic = "public"
)

// Service provides notes/status operations
type Service struct {
	noteRepo          *repositories.StatusRepository
	accountRepo       interfaces.AccountRepository
	relationshipRepo  *repositories.RelationshipRepository
	likeRepo          *repositories.LikeRepository
	socialRepo        interfaces.SocialRepository
	conversationRepo  interfaces.ConversationRepository
	objectRepo        *repositories.ObjectRepository
	searchRepo        *repositories.SearchRepository
	communityNoteRepo *repositories.CommunityNoteRepository
	userRepo          *repositories.UserRepository
	pollRepo          *repositories.PollRepository
	scheduledRepo     ScheduledStatusRepository
	publisher         streaming.Publisher
	analytics         AnalyticsService
	logger            *zap.Logger
	domainName        string
	federation        FederationService // Interface to be defined
}

// ScheduledStatusRepository defines the interface for scheduled status operations
type ScheduledStatusRepository interface {
	CreateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error
	GetScheduledStatus(ctx context.Context, id string) (*storage.ScheduledStatus, error)
	GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error)
	UpdateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error
	DeleteScheduledStatus(ctx context.Context, id string) error
}

// FederationService defines the interface for federation operations
type FederationService interface {
	QueueActivity(ctx context.Context, activity *activitypub.Activity) error
}

// AnalyticsService defines the interface for analytics operations needed by the notes service
type AnalyticsService interface {
	RecordStatusCreation(ctx context.Context, actorID string, timestamp time.Time) error
	RecordHashtagUsage(ctx context.Context, hashtags []string, objectID, actorID string) error
	RecordLinkShare(ctx context.Context, links []string, objectID, actorID string) error
	RecordEngagement(ctx context.Context, objectID, engagementType, actorID string) error
	RecordInstanceActivity(ctx context.Context, activityType string, timestamp time.Time) error
}

// NewService creates a new Notes Service with the required dependencies
func NewService(
	noteRepo *repositories.StatusRepository,
	accountRepo interfaces.AccountRepository,
	relationshipRepo *repositories.RelationshipRepository,
	likeRepo *repositories.LikeRepository,
	socialRepo interfaces.SocialRepository,
	conversationRepo interfaces.ConversationRepository,
	objectRepo *repositories.ObjectRepository,
	searchRepo *repositories.SearchRepository,
	communityNoteRepo *repositories.CommunityNoteRepository,
	userRepo *repositories.UserRepository,
	pollRepo *repositories.PollRepository,
	publisher streaming.Publisher,
	analytics AnalyticsService,
	federation FederationService,
	logger *zap.Logger,
	domainName string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		noteRepo:          noteRepo,
		accountRepo:       accountRepo,
		relationshipRepo:  relationshipRepo,
		likeRepo:          likeRepo,
		socialRepo:        socialRepo,
		conversationRepo:  conversationRepo,
		objectRepo:        objectRepo,
		searchRepo:        searchRepo,
		communityNoteRepo: communityNoteRepo,
		userRepo:          userRepo,
		pollRepo:          pollRepo,
		publisher:         publisher,
		analytics:         analytics,
		federation:        federation,
		logger:            logger,
		domainName:        domainName,
	}
}

// Command structs for operations

// CreateNoteCommand contains all data needed to create a new note
type CreateNoteCommand struct {
	AuthorID       string   `json:"author_id" validate:"required"`
	Content        string   `json:"content" validate:"required,max=5000"`
	Visibility     string   `json:"visibility" validate:"required,oneof=public unlisted private direct"`
	Sensitive      bool     `json:"sensitive"`
	Language       string   `json:"language"`
	InReplyToID    string   `json:"in_reply_to_id"`
	ConversationID string   `json:"conversation_id"`
	MediaIDs       []string `json:"media_ids"`
	PollOptions    []string `json:"poll_options"`
	PollExpiresIn  int      `json:"poll_expires_in"`  // Duration in seconds
	PollMultiple   bool     `json:"poll_multiple"`    // Allow multiple choices
	PollHideTotals bool     `json:"poll_hide_totals"` // Hide vote counts until poll ends
	ToRecipients   []string `json:"to_recipients"`
	CcRecipients   []string `json:"cc_recipients"`
	BtoRecipients  []string `json:"bto_recipients"`
	BccRecipients  []string `json:"bcc_recipients"`
}

// UpdateNoteCommand contains all data needed to update an existing note
type UpdateNoteCommand struct {
	StatusID  string   `json:"status_id" validate:"required"`
	Content   string   `json:"content" validate:"required,max=5000"`
	Sensitive bool     `json:"sensitive"`
	Language  string   `json:"language"`
	MediaIDs  []string `json:"media_ids"`
	UpdaterID string   `json:"updater_id" validate:"required"` // Must be author
}

// DeleteNoteCommand contains data needed to delete a note
type DeleteNoteCommand struct {
	StatusID  string `json:"status_id" validate:"required"`
	DeleterID string `json:"deleter_id" validate:"required"` // Must be author or admin
	Reason    string `json:"reason"`                         // Optional reason for admin deletions
}

// GetNoteQuery contains parameters for retrieving a single note
type GetNoteQuery struct {
	StatusID string `json:"status_id" validate:"required"`
	ViewerID string `json:"viewer_id"` // User requesting the note (for privacy checks)
}

// ListNotesQuery contains parameters for listing notes with various filters
type ListNotesQuery struct {
	ViewerID       string                       `json:"viewer_id"` // User requesting the timeline
	TimelineType   string                       `json:"timeline_type" validate:"required,oneof=home public local conversations hashtag user direct list"`
	AuthorID       string                       `json:"author_id"`       // For user timelines
	Hashtag        string                       `json:"hashtag"`         // For hashtag timelines
	ConversationID string                       `json:"conversation_id"` // For conversation threads
	ParentID       string                       `json:"parent_id"`       // For reply threads
	ListID         string                       `json:"list_id"`         // For list timelines
	Pagination     interfaces.PaginationOptions `json:"pagination"`
	OnlyMedia      bool                         `json:"only_media"`
	ExcludeReplies bool                         `json:"exclude_replies"`
	ExcludeReblogs bool                         `json:"exclude_reblogs"`
	PinnedOnly     bool                         `json:"pinned_only"`
	SinceID        string                       `json:"since_id"` // Get items newer than this ID
	MinID          string                       `json:"min_id"`   // Get items immediately newer than this ID
}

// Result structs for operations

// NoteResult contains a note and associated events that were emitted
type NoteResult struct {
	Note   *models.Status     `json:"note"`
	Events []*streaming.Event `json:"events"`
}

// Result contains multiple notes and pagination information
type Result struct {
	Notes      []*models.Status                            `json:"notes"`
	Pagination *interfaces.PaginatedResult[*models.Status] `json:"pagination"`
	Events     []*streaming.Event                          `json:"events"`
}

// CreateNote creates a new note, validates input, stores it, emits events, and queues federation
func (s *Service) CreateNote(ctx context.Context, cmd *CreateNoteCommand) (*NoteResult, error) {
	s.logger.Info("creating note",
		zap.String("author_id", cmd.AuthorID),
		zap.String("visibility", cmd.Visibility),
		zap.Int("content_length", len(cmd.Content)))

	// Validate the command
	if err := s.validateCreateCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Get author account
	author, err := s.accountRepo.GetAccount(ctx, cmd.AuthorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get author account: %w", err)
	}

	// Generate unique status ID
	statusID := uuid.New().String()

	// Create ActivityPub Note
	note := s.buildActivityPubNote(cmd, statusID, author)

	// Create Status model
	status := &models.Status{
		StatusID:       statusID,
		Note:           note,
		AuthorID:       cmd.AuthorID,
		AuthorUsername: author.User.Username,
		Content:        cmd.Content,
		Visibility:     cmd.Visibility,
		Sensitive:      cmd.Sensitive,
		Language:       cmd.Language,
		InReplyToID:    cmd.InReplyToID,
		ConversationID: cmd.ConversationID,
		ToRecipients:   cmd.ToRecipients,
		CcRecipients:   cmd.CcRecipients,
		BtoRecipients:  cmd.BtoRecipients,
		BccRecipients:  cmd.BccRecipients,
		PublishedAt:    time.Now(),
	}

	// Handle conversation ID - create new conversation if not provided and it's a top-level post
	if status.ConversationID == "" && status.InReplyToID == "" {
		status.ConversationID = statusID
	} else if status.ConversationID == "" && status.InReplyToID != "" {
		// Get parent status to inherit conversation ID
		parent, err := s.noteRepo.GetStatus(ctx, status.InReplyToID)
		if err == nil && parent.ConversationID != "" {
			status.ConversationID = parent.ConversationID
		} else {
			status.ConversationID = status.InReplyToID
		}
	}

	// Store the status
	if err := s.noteRepo.CreateStatus(ctx, status); err != nil {
		return nil, fmt.Errorf("failed to create status: %w", err)
	}

	// Create poll if poll options are provided
	if len(cmd.PollOptions) > 0 {
		if err := s.createPollForStatus(ctx, cmd, statusID); err != nil {
			// Log error but don't fail status creation
			s.logger.Error("failed to create poll for status",
				zap.String("status_id", statusID),
				zap.Error(err))
		}
	}

	// Update instance metrics after successful creation
	if s.analytics != nil {
		activityType := "post" // Default to post
		if status.InReplyToID != "" {
			activityType = "comment" // This is a reply/comment
		}
		
		if err := s.analytics.RecordInstanceActivity(ctx, activityType, time.Now()); err != nil {
			// Log the error but don't fail the creation - metrics are not critical
			s.logger.Warn("failed to record instance metrics", 
				zap.String("activity_type", activityType),
				zap.String("status_id", statusID),
				zap.Error(err))
		}
	}

	s.logger.Info("created note successfully",
		zap.String("status_id", statusID),
		zap.String("conversation_id", status.ConversationID))

	// Emit events and queue federation
	events := s.emitStatusCreatedEvents(ctx, status)
	s.queueFederationDelivery(ctx, status, "Create")

	return &NoteResult{
		Note:   status,
		Events: events,
	}, nil
}

// UpdateNote updates an existing note, validates permission, stores changes, and emits events
func (s *Service) UpdateNote(ctx context.Context, cmd *UpdateNoteCommand) (*NoteResult, error) {
	s.logger.Info("updating note",
		zap.String("status_id", cmd.StatusID),
		zap.String("updater_id", cmd.UpdaterID))

	// Get existing status
	status, err := s.noteRepo.GetStatus(ctx, cmd.StatusID)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	// Verify permission (only author can update)
	if status.AuthorID != cmd.UpdaterID {
		return nil, fmt.Errorf("unauthorized: only the author can update their posts")
	}

	// Validate the update
	if err := s.validateUpdateCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Update status fields
	status.Content = cmd.Content
	status.Sensitive = cmd.Sensitive
	status.Language = cmd.Language
	status.UpdatedAt = time.Now()

	// Update the ActivityPub Note if present
	if status.Note != nil {
		status.Note.Content = cmd.Content
		status.Note.Sensitive = cmd.Sensitive
		now := time.Now()
		status.Note.Updated = &now
	}

	// Store the updated status
	if err := s.noteRepo.UpdateStatus(ctx, status); err != nil {
		return nil, fmt.Errorf("failed to update status: %w", err)
	}

	s.logger.Info("updated note successfully",
		zap.String("status_id", cmd.StatusID))

	// Emit events and queue federation
	events := s.emitStatusUpdatedEvents(ctx, status)
	s.queueFederationDelivery(ctx, status, "Update")

	return &NoteResult{
		Note:   status,
		Events: events,
	}, nil
}

// DeleteNote performs a soft delete on a note, emits events, and queues federation tombstone
func (s *Service) DeleteNote(ctx context.Context, cmd *DeleteNoteCommand) error {
	s.logger.Info("deleting note",
		zap.String("status_id", cmd.StatusID),
		zap.String("deleter_id", cmd.DeleterID))

	// Get existing status
	status, err := s.noteRepo.GetStatus(ctx, cmd.StatusID)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	// Check if already deleted
	if status.Deleted {
		return nil // Idempotent operation
	}

	// Verify permission (author or admin)
	if status.AuthorID != cmd.DeleterID {
		// Check if deleter is an admin
		isAdmin := false
		if s.userRepo != nil {
			if deleter, err := s.userRepo.GetUser(ctx, cmd.DeleterID); err == nil && deleter != nil {
				isAdmin = deleter.Role == "admin"
			}
		}
		
		if !isAdmin {
			return fmt.Errorf("unauthorized: only the author or admin can delete posts")
		}
	}

	// Perform soft delete
	now := time.Now()
	status.Deleted = true
	status.DeletedAt = &now
	status.ModifiedAt = now

	// Store the deletion
	if err := s.noteRepo.UpdateStatus(ctx, status); err != nil {
		return fmt.Errorf("failed to delete status: %w", err)
	}

	s.logger.Info("deleted note successfully",
		zap.String("status_id", cmd.StatusID))

	// Emit events and queue federation tombstone
	s.emitStatusDeletedEvents(ctx, status)
	s.queueFederationTombstone(ctx, status)

	return nil
}

// GetNote retrieves a single note with privacy checks
func (s *Service) GetNote(ctx context.Context, statusID string) (*models.Status, error) {
	s.logger.Debug("getting note",
		zap.String("status_id", statusID))

	// Get the status
	status, err := s.noteRepo.GetStatus(ctx, statusID)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	// Check if deleted
	if status.Deleted {
		return nil, fmt.Errorf("status not found") // Don't reveal it was deleted
	}

	// Return the status directly since we simplified the method
	return status, nil
}

// GetNoteWithViewer retrieves a note with viewer context for privacy checking
func (s *Service) GetNoteWithViewer(ctx context.Context, query *GetNoteQuery) (*models.Status, error) {
	if query.StatusID == "" {
		return nil, fmt.Errorf("status_id is required")
	}

	s.logger.Debug("getting note with viewer context",
		zap.String("status_id", query.StatusID),
		zap.String("viewer_id", query.ViewerID))

	// Get the status
	status, err := s.noteRepo.GetStatus(ctx, query.StatusID)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	// Check if deleted
	if status.Deleted {
		return nil, fmt.Errorf("status not found") // Don't reveal it was deleted
	}

	// Check privacy permissions
	canView, err := s.checkViewPermissions(ctx, status, query.ViewerID)
	if err != nil {
		return nil, fmt.Errorf("failed to check view permissions: %w", err)
	}

	if !canView {
		return nil, fmt.Errorf("status not found") // Don't reveal access denied
	}

	return status, nil
}

// checkViewPermissions implements comprehensive privacy checking
func (s *Service) checkViewPermissions(ctx context.Context, status *models.Status, viewerID string) (bool, error) {
	// Public and unlisted posts are viewable by anyone
	if status.Visibility == models.VisibilityPublic || status.Visibility == models.VisibilityUnlisted {
		return true, nil
	}

	// Unauthenticated users can only see public/unlisted posts
	if viewerID == "" {
		return false, nil
	}

	// Status author can always view their own posts
	if status.AuthorUsername == viewerID {
		return true, nil
	}

	// Handle private (followers-only) posts
	if status.Visibility == models.VisibilityPrivate {
		// Check if viewer follows the author using relationship repository
		isFollowing, err := s.relationshipRepo.IsFollowing(ctx, viewerID, status.AuthorUsername)
		if err != nil {
			s.logger.Error("failed to check following relationship",
				zap.String("status_id", status.StatusID),
				zap.String("viewer_id", viewerID),
				zap.String("author", status.AuthorUsername),
				zap.Error(err))
			return false, fmt.Errorf("failed to check following relationship: %w", err)
		}
		return isFollowing, nil
	}

	// Handle direct messages
	if status.Visibility == models.VisibilityDirect {
		// Check if viewer is explicitly mentioned
		for _, mention := range status.Mentions {
			if mention == viewerID {
				return true, nil
			}
		}

		// Check explicit recipients (simplified - in full implementation would check actor IDs)
		viewerUsername := viewerID
		for _, recipient := range status.ToRecipients {
			if strings.Contains(recipient, viewerUsername) {
				return true, nil
			}
		}
		
		for _, recipient := range status.CcRecipients {
			if strings.Contains(recipient, viewerUsername) {
				return true, nil
			}
		}

		// Not a recipient of direct message
		return false, nil
	}

	// Unknown visibility - default deny
	s.logger.Warn("unknown visibility level",
		zap.String("status_id", status.StatusID),
		zap.String("visibility", status.Visibility))
	return false, nil
}

// ListNotes retrieves notes based on various timeline types and filters
func (s *Service) ListNotes(ctx context.Context, query *ListNotesQuery) (*Result, error) {
	s.logger.Debug("listing notes",
		zap.String("timeline_type", query.TimelineType),
		zap.String("viewer_id", query.ViewerID),
		zap.String("author_id", query.AuthorID))

	var result *interfaces.PaginatedResult[*models.Status]
	var err error

	// Route to appropriate timeline method based on type
	switch query.TimelineType {
	case VisibilityPublic:
		result, err = s.noteRepo.GetPublicTimeline(ctx, query.Pagination)
	case "home":
		if query.ViewerID == "" {
			return nil, fmt.Errorf("home timeline requires viewer_id")
		}
		result, err = s.noteRepo.GetHomeTimeline(ctx, query.ViewerID, query.Pagination)
	case "user":
		if query.AuthorID == "" {
			return nil, fmt.Errorf("user timeline requires author_id")
		}
		result, err = s.noteRepo.GetUserTimeline(ctx, query.AuthorID, query.Pagination)
	case "conversations":
		if query.ConversationID == "" {
			return nil, fmt.Errorf("conversations timeline requires conversation_id")
		}
		result, err = s.noteRepo.GetConversationThread(ctx, query.ConversationID, query.Pagination)
	case "direct":
		if query.ViewerID == "" {
			return nil, fmt.Errorf("direct timeline requires viewer_id")
		}
		// Direct messages are handled differently - we get conversations first, then statuses
		return s.getDirectTimeline(ctx, query)
	case "hashtag":
		if query.Hashtag == "" {
			return nil, fmt.Errorf("hashtag timeline requires hashtag")
		}
		result, err = s.noteRepo.GetStatusesByHashtag(ctx, query.Hashtag, query.Pagination)
	case "list":
		if query.ListID == "" {
			return nil, fmt.Errorf("list timeline requires list_id")
		}
		// Get list timeline - statuses from members of the list
		listResult, listErr := s.getListTimeline(ctx, query)
		if listErr != nil {
			return nil, listErr
		}
		return listResult, nil
	default:
		return nil, fmt.Errorf("unsupported timeline type: %s", query.TimelineType)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get timeline: %w", err)
	}

	// Filter results based on privacy and other criteria
	filteredNotes := make([]*models.Status, 0, len(result.Items))
	for _, status := range result.Items {
		// Skip deleted posts
		if status.Deleted {
			continue
		}

		// Check visibility
		if !status.IsVisibleTo(query.ViewerID) {
			continue
		}

		// Apply additional filters
		if query.OnlyMedia && !status.HasMedia() {
			continue
		}
		if query.ExcludeReplies && status.IsReply() {
			continue
		}
		// Note: ExcludeReblogs and PinnedOnly would require additional data

		// Sanitize for viewer
		sanitized := status.SanitizeForActor(query.ViewerID)
		filteredNotes = append(filteredNotes, sanitized)
	}

	// Update the result with filtered items
	filteredResult := &interfaces.PaginatedResult[*models.Status]{
		Items:      filteredNotes,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
		Total:      int64(len(filteredNotes)),
	}

	return &Result{
		Notes:      filteredNotes,
		Pagination: filteredResult,
		Events:     []*streaming.Event{}, // No events for read operations
	}, nil
}

// Private helper methods

func (s *Service) validateCreateCommand(ctx context.Context, cmd *CreateNoteCommand) error {
	if cmd.AuthorID == "" {
		return fmt.Errorf("author_id is required")
	}

	if strings.TrimSpace(cmd.Content) == "" {
		return fmt.Errorf("content cannot be empty")
	}

	if len(cmd.Content) > 5000 {
		return fmt.Errorf("content too long (max 5000 characters)")
	}

	validVisibilities := map[string]bool{
		models.VisibilityPublic:   true,
		models.VisibilityUnlisted: true,
		models.VisibilityPrivate:  true,
		models.VisibilityDirect:   true,
	}

	if !validVisibilities[cmd.Visibility] {
		return fmt.Errorf("invalid visibility: %s", cmd.Visibility)
	}

	// Validate in_reply_to_id if provided
	if cmd.InReplyToID != "" {
		_, err := s.noteRepo.GetStatus(ctx, cmd.InReplyToID)
		if err != nil {
			return fmt.Errorf("invalid in_reply_to_id: %w", err)
		}
	}

	return nil
}

func (s *Service) validateUpdateCommand(_ context.Context, cmd *UpdateNoteCommand) error {
	if strings.TrimSpace(cmd.Content) == "" {
		return fmt.Errorf("content cannot be empty")
	}

	if len(cmd.Content) > 5000 {
		return fmt.Errorf("content too long (max 5000 characters)")
	}

	return nil
}

func (s *Service) buildActivityPubNote(cmd *CreateNoteCommand, statusID string, author *storage.Account) *activitypub.Note {
	now := time.Now()

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   "https://www.w3.org/ns/activitystreams",
			Type:      "Note",
			ID:        fmt.Sprintf("https://%s/users/%s/statuses/%s", s.domainName, author.User.Username, statusID),
			Published: &now,
			To:        cmd.ToRecipients,
			CC:        cmd.CcRecipients,
			BTo:       cmd.BtoRecipients,
			BCC:       cmd.BccRecipients,
			Sensitive: cmd.Sensitive,
		},
		Content:      cmd.Content,
		AttributedTo: fmt.Sprintf("https://%s/users/%s", s.domainName, author.User.Username),
		Visibility:   cmd.Visibility,
	}

	// Set conversation ID
	if cmd.ConversationID != "" {
		note.ConversationID = cmd.ConversationID
	}

	// Set in reply to
	if cmd.InReplyToID != "" {
		note.InReplyTo = fmt.Sprintf("https://%s/users/%s/statuses/%s", s.domainName, author.User.Username, cmd.InReplyToID)
	}

	return note
}

func (s *Service) emitStatusCreatedEvents(ctx context.Context, status *models.Status) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      "status.created",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"status": status,
		},
	}

	// Emit to user's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", status.AuthorUsername)
	if err := s.publisher.PublishToUser(ctx, status.AuthorID, &userEvent); err != nil {
		s.logger.Error("failed to publish to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	// Emit to public stream if public
	if status.IsPublic() {
		publicEvent := *event
		publicEvent.Stream = VisibilityPublic
		if err := s.publisher.PublishToStream(ctx, "public", &publicEvent); err != nil {
			s.logger.Error("failed to publish to public stream", zap.Error(err))
		} else {
			events = append(events, &publicEvent)
		}
	}

	// Emit to hashtag streams
	for _, hashtag := range status.Hashtags {
		hashtagEvent := *event
		hashtagEvent.Stream = fmt.Sprintf("hashtag:%s", hashtag)
		if err := s.publisher.PublishToStream(ctx, hashtagEvent.Stream, &hashtagEvent); err != nil {
			s.logger.Error("failed to publish to hashtag stream",
				zap.String("hashtag", hashtag),
				zap.Error(err))
		} else {
			events = append(events, &hashtagEvent)
		}
	}

	// If it's a direct message, emit to conversation
	if status.IsDirect() && status.ConversationID != "" {
		conversationEvent := *event
		conversationEvent.Stream = fmt.Sprintf("conversation:%s", status.ConversationID)
		if err := s.publisher.PublishToConversation(ctx, status.ConversationID, &conversationEvent); err != nil {
			s.logger.Error("failed to publish to conversation stream", zap.Error(err))
		} else {
			events = append(events, &conversationEvent)
		}
	}

	return events
}

func (s *Service) emitStatusUpdatedEvents(ctx context.Context, status *models.Status) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      "status.updated",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"status": status,
		},
	}

	// Emit to user's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", status.AuthorUsername)
	if err := s.publisher.PublishToUser(ctx, status.AuthorID, &userEvent); err != nil {
		s.logger.Error("failed to publish update to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	// Emit to public stream if public
	if status.IsPublic() {
		publicEvent := *event
		publicEvent.Stream = VisibilityPublic
		if err := s.publisher.PublishToStream(ctx, "public", &publicEvent); err != nil {
			s.logger.Error("failed to publish update to public stream", zap.Error(err))
		} else {
			events = append(events, &publicEvent)
		}
	}

	return events
}

func (s *Service) emitStatusDeletedEvents(ctx context.Context, status *models.Status) {
	// Create base event
	event := &streaming.Event{
		Type:      "status.deleted",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"status_id": status.StatusID,
		},
	}

	// Emit to user's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", status.AuthorUsername)
	if err := s.publisher.PublishToUser(ctx, status.AuthorID, &userEvent); err != nil {
		s.logger.Error("failed to publish deletion to user stream", zap.Error(err))
	}

	// Emit to public stream if it was public
	if status.IsPublic() {
		publicEvent := *event
		publicEvent.Stream = VisibilityPublic
		if err := s.publisher.PublishToStream(ctx, "public", &publicEvent); err != nil {
			s.logger.Error("failed to publish deletion to public stream", zap.Error(err))
		}
	}
}

func (s *Service) queueFederationDelivery(ctx context.Context, status *models.Status, activityType string) {
	if s.federation == nil {
		s.logger.Debug("federation service not available, skipping delivery")
		return
	}

	// Create ActivityPub Create activity
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: "https://www.w3.org/ns/activitystreams",
			Type:    activityType,
			ID:      fmt.Sprintf("%s#%s", status.Note.ID, strings.ToLower(activityType)),
			To:      status.ToRecipients,
			CC:      status.CcRecipients,
			BTo:     status.BtoRecipients,
			BCC:     status.BccRecipients,
		},
		Actor:  status.Note.AttributedTo,
		Object: status.Note,
	}

	if err := s.federation.QueueActivity(ctx, activity); err != nil {
		s.logger.Error("failed to queue federation delivery",
			zap.String("status_id", status.StatusID),
			zap.String("activity_type", activityType),
			zap.Error(err))
	}
}

func (s *Service) queueFederationTombstone(ctx context.Context, status *models.Status) {
	if s.federation == nil {
		s.logger.Debug("federation service not available, skipping tombstone")
		return
	}

	// Create ActivityPub Delete activity with Tombstone
	tombstone := &activitypub.BaseObject{
		Type: "Tombstone",
		ID:   status.Note.ID,
	}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: "https://www.w3.org/ns/activitystreams",
			Type:    "Delete",
			ID:      fmt.Sprintf("%s#delete", status.Note.ID),
			To:      status.ToRecipients,
			CC:      status.CcRecipients,
		},
		Actor:  status.Note.AttributedTo,
		Object: tombstone,
	}

	if err := s.federation.QueueActivity(ctx, activity); err != nil {
		s.logger.Error("failed to queue federation tombstone",
			zap.String("status_id", status.StatusID),
			zap.Error(err))
	}
}

func (s *Service) emitLikeEvents(ctx context.Context, status *models.Status, likerUsername string) []*streaming.Event {
	var events []*streaming.Event

	// Create like event
	event := &streaming.Event{
		Type:      "status.liked",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"status": status,
			"liker":  likerUsername,
		},
	}

	// Emit to user's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", likerUsername)
	if err := s.publisher.PublishToUser(ctx, likerUsername, &userEvent); err != nil {
		s.logger.Error("failed to publish like to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}

func (s *Service) emitUnlikeEvents(ctx context.Context, status *models.Status, unlikerUsername string) []*streaming.Event {
	var events []*streaming.Event

	// Create unlike event
	event := &streaming.Event{
		Type:      "status.unliked",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"status":  status,
			"unliker": unlikerUsername,
		},
	}

	// Emit to user's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", unlikerUsername)
	if err := s.publisher.PublishToUser(ctx, unlikerUsername, &userEvent); err != nil {
		s.logger.Error("failed to publish unlike to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}

// GetUpdateHistoryQuery contains parameters for getting update history
type GetUpdateHistoryQuery struct {
	StatusID string
	Limit    int
}

// GetUpdateHistoryResult contains the update history for a status
type GetUpdateHistoryResult struct {
	History []*storage.UpdateHistory
	Events  []*streaming.Event
}

// GetUpdateHistory retrieves the edit history for a status
func (s *Service) GetUpdateHistory(ctx context.Context, query *GetUpdateHistoryQuery) (*GetUpdateHistoryResult, error) {
	if query.StatusID == "" {
		return nil, fmt.Errorf("status ID is required")
	}

	// Normalize the status ID to object ID
	objectID := query.StatusID
	if !strings.HasPrefix(query.StatusID, "http://") && !strings.HasPrefix(query.StatusID, "https://") {
		objectID = fmt.Sprintf("%s/objects/%s", s.domainName, query.StatusID)
	}

	// Get update history from repository
	limit := query.Limit
	if limit <= 0 {
		limit = 100 // Default limit
	}

	history, err := s.objectRepo.GetUpdateHistory(ctx, objectID, limit)
	if err != nil {
		s.logger.Error("failed to get update history",
			zap.String("object_id", objectID),
			zap.Error(err))
		// Return empty history instead of error for not found
		return &GetUpdateHistoryResult{
			History: []*storage.UpdateHistory{},
			Events:  []*streaming.Event{},
		}, nil
	}

	return &GetUpdateHistoryResult{
		History: history,
		Events:  []*streaming.Event{},
	}, nil
}

func (s *Service) emitReblogEvents(ctx context.Context, status *models.Status, rebloggerUsername string) []*streaming.Event {
	var events []*streaming.Event

	// Create reblog event
	event := &streaming.Event{
		Type:      "status.reblogged",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"status":    status,
			"reblogger": rebloggerUsername,
		},
	}

	// Emit to user's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", rebloggerUsername)
	if err := s.publisher.PublishToUser(ctx, rebloggerUsername, &userEvent); err != nil {
		s.logger.Error("failed to publish reblog to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	// Emit to public stream if public
	if status.IsPublic() {
		publicEvent := *event
		publicEvent.Stream = "public"
		if err := s.publisher.PublishToStream(ctx, "public", &publicEvent); err != nil {
			s.logger.Error("failed to publish reblog to public stream", zap.Error(err))
		} else {
			events = append(events, &publicEvent)
		}
	}

	return events
}

func (s *Service) emitUnreblogEvents(ctx context.Context, status *models.Status, unrebloggerUsername string) []*streaming.Event {
	var events []*streaming.Event

	// Create unreblog event
	event := &streaming.Event{
		Type:      "status.unreblogged",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"status":      status,
			"unreblogger": unrebloggerUsername,
		},
	}

	// Emit to user's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", unrebloggerUsername)
	if err := s.publisher.PublishToUser(ctx, unrebloggerUsername, &userEvent); err != nil {
		s.logger.Error("failed to publish unreblog to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}

// Bookmark Commands

// BookmarkNoteCommand represents a request to bookmark a status
type BookmarkNoteCommand struct {
	StatusID     string `json:"status_id" validate:"required"`
	BookmarkerID string `json:"bookmarker_id" validate:"required"`
}

// UnbookmarkNoteCommand represents a request to unbookmark a status
type UnbookmarkNoteCommand struct {
	StatusID       string `json:"status_id" validate:"required"`
	UnbookmarkerID string `json:"unbookmarker_id" validate:"required"`
}

// GetBookmarksQuery represents a request to get user's bookmarked statuses
type GetBookmarksQuery struct {
	UserID     string                       `json:"user_id" validate:"required"`
	Pagination interfaces.PaginationOptions `json:"pagination"`
}

// GetTimelineQuery represents a request to get a timeline
type GetTimelineQuery struct {
	UserID   string `json:"user_id" validate:"required"`
	Timeline string `json:"timeline" validate:"required,oneof=home public direct list hashtag"` // Type of timeline
	ListID   string `json:"list_id,omitempty"`                                                  // For list timeline
	Hashtag  string `json:"hashtag,omitempty"`                                                  // For hashtag timeline
	Limit    int    `json:"limit,omitempty"`                                                    // Max number of items
	SinceID  string `json:"since_id,omitempty"`                                                 // Get items newer than this ID
	MaxID    string `json:"max_id,omitempty"`                                                   // Get items older than this ID
	MinID    string `json:"min_id,omitempty"`                                                   // Get items immediately newer than this ID
}

// BookmarkResult represents the result of a bookmark operation
type BookmarkResult struct {
	Status *models.Status     `json:"status"`
	Events []*streaming.Event `json:"events"`
}

// BookmarkNote adds a status to user's bookmarks
func (s *Service) BookmarkNote(ctx context.Context, cmd *BookmarkNoteCommand) (*BookmarkResult, error) {
	// Get the status first to validate it exists
	note, err := s.noteRepo.GetStatus(ctx, cmd.StatusID)
	if err != nil {
		return nil, fmt.Errorf("status not found: %w", err)
	}

	// Add bookmark through account repository
	if err := s.accountRepo.AddBookmark(ctx, cmd.BookmarkerID, cmd.StatusID); err != nil {
		return nil, fmt.Errorf("failed to bookmark status: %w", err)
	}

	// Emit events
	events := s.emitBookmarkEvents(ctx, note, cmd.BookmarkerID)

	return &BookmarkResult{
		Status: note,
		Events: events,
	}, nil
}

// UnbookmarkNote removes a status from user's bookmarks
func (s *Service) UnbookmarkNote(ctx context.Context, cmd *UnbookmarkNoteCommand) (*BookmarkResult, error) {
	// Get the status first to validate it exists
	note, err := s.noteRepo.GetStatus(ctx, cmd.StatusID)
	if err != nil {
		return nil, fmt.Errorf("status not found: %w", err)
	}

	// Remove bookmark through account repository
	if err := s.accountRepo.RemoveBookmark(ctx, cmd.UnbookmarkerID, cmd.StatusID); err != nil {
		return nil, fmt.Errorf("failed to unbookmark status: %w", err)
	}

	// Emit events
	events := s.emitUnbookmarkEvents(ctx, note, cmd.UnbookmarkerID)

	return &BookmarkResult{
		Status: note,
		Events: events,
	}, nil
}

// GetBookmarks retrieves user's bookmarked statuses
func (s *Service) GetBookmarks(ctx context.Context, query *GetBookmarksQuery) (*Result, error) {
	// Get bookmarked statuses through account repository
	result, err := s.accountRepo.GetBookmarkedStatuses(ctx, query.UserID, query.Pagination)
	if err != nil {
		return nil, fmt.Errorf("failed to get bookmarks: %w", err)
	}

	return &Result{
		Notes:      result.Items,
		Pagination: result,
	}, nil
}

// GetTimeline retrieves a specific timeline for the user
func (s *Service) GetTimeline(ctx context.Context, query *GetTimelineQuery) (*Result, error) {
	// Convert timeline query to ListNotesQuery
	listQuery := &ListNotesQuery{
		ViewerID:     query.UserID,
		TimelineType: query.Timeline,
		ListID:       query.ListID,
		Hashtag:      query.Hashtag,
		Pagination: interfaces.PaginationOptions{
			Limit:  query.Limit,
			Cursor: query.MaxID,
		},
	}

	// If SinceID is specified, we need to handle it specially
	if query.SinceID != "" {
		listQuery.SinceID = query.SinceID
	}

	// If MinID is specified, we need to handle it specially
	if query.MinID != "" {
		listQuery.MinID = query.MinID
	}

	// Use the existing ListNotes method which handles all timeline types
	return s.ListNotes(ctx, listQuery)
}

// emitBookmarkEvents creates and publishes bookmark events
func (s *Service) emitBookmarkEvents(ctx context.Context, status *models.Status, bookmarkerUsername string) []*streaming.Event {
	var events []*streaming.Event

	// Create bookmark event
	event := &streaming.Event{
		Type:      "status.bookmarked",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"status":     status,
			"bookmarker": bookmarkerUsername,
		},
	}

	// Emit to user's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", bookmarkerUsername)
	if err := s.publisher.PublishToUser(ctx, bookmarkerUsername, &userEvent); err != nil {
		s.logger.Error("failed to publish bookmark to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}

// emitUnbookmarkEvents creates and publishes unbookmark events
func (s *Service) emitUnbookmarkEvents(ctx context.Context, status *models.Status, unbookmarkerUsername string) []*streaming.Event {
	var events []*streaming.Event

	// Create unbookmark event
	event := &streaming.Event{
		Type:      "status.unbookmarked",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"status":       status,
			"unbookmarker": unbookmarkerUsername,
		},
	}

	// Emit to user's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", unbookmarkerUsername)
	if err := s.publisher.PublishToUser(ctx, unbookmarkerUsername, &userEvent); err != nil {
		s.logger.Error("failed to publish unbookmark to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}

// Like/Favorite Commands and Queries

// LikeNoteCommand represents a request to like a status
type LikeNoteCommand struct {
	StatusID string `json:"status_id" validate:"required"`
	LikerID  string `json:"liker_id" validate:"required"`
}

// UnlikeNoteCommand represents a request to unlike a status
type UnlikeNoteCommand struct {
	StatusID  string `json:"status_id" validate:"required"`
	UnlikerID string `json:"unliker_id" validate:"required"`
}

// GetLikersQuery represents a request to get users who liked a status
type GetLikersQuery struct {
	StatusID   string                       `json:"status_id" validate:"required"`
	Pagination interfaces.PaginationOptions `json:"pagination"`
}

// LikeResult represents the result of a like operation
type LikeResult struct {
	Status *models.Status     `json:"status"`
	Events []*streaming.Event `json:"events"`
}

// UsersResult represents a list of users (for likers, rebloggers, etc.)
type UsersResult struct {
	Users      []*storage.Account                            `json:"users"`
	Pagination *interfaces.PaginatedResult[*storage.Account] `json:"pagination"`
}

// LikeNote adds a like to a status
func (s *Service) LikeNote(ctx context.Context, cmd *LikeNoteCommand) (*LikeResult, error) {
	// Get the status first to validate it exists
	note, err := s.noteRepo.GetStatus(ctx, cmd.StatusID)
	if err != nil {
		return nil, fmt.Errorf("status not found: %w", err)
	}

	// Get liker's account
	liker, err := s.accountRepo.GetAccount(ctx, cmd.LikerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get liker account: %w", err)
	}

	// Create actor and object URLs for the like
	actorURL := fmt.Sprintf("https://%s/users/%s", s.domainName, liker.User.Username)
	objectURL := fmt.Sprintf("https://%s/users/%s/statuses/%s", s.domainName, note.AuthorUsername, cmd.StatusID)

	// Create the like through repository interface
	if err := s.createLike(ctx, actorURL, objectURL); err != nil {
		return nil, fmt.Errorf("failed to like status: %w", err)
	}

	// Emit events
	events := s.emitLikeEvents(ctx, note, cmd.LikerID)

	return &LikeResult{
		Status: note,
		Events: events,
	}, nil
}

// UnlikeNote removes a like from a status
func (s *Service) UnlikeNote(ctx context.Context, cmd *UnlikeNoteCommand) (*LikeResult, error) {
	// Get the status first to validate it exists
	note, err := s.noteRepo.GetStatus(ctx, cmd.StatusID)
	if err != nil {
		return nil, fmt.Errorf("status not found: %w", err)
	}

	// Get unliker's account
	unliker, err := s.accountRepo.GetAccount(ctx, cmd.UnlikerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get unliker account: %w", err)
	}

	// Create actor and object URLs for the unlike
	actorURL := fmt.Sprintf("https://%s/users/%s", s.domainName, unliker.User.Username)
	objectURL := fmt.Sprintf("https://%s/users/%s/statuses/%s", s.domainName, note.AuthorUsername, cmd.StatusID)

	// Remove the like through repository interface
	if err := s.deleteLike(ctx, actorURL, objectURL); err != nil {
		return nil, fmt.Errorf("failed to unlike status: %w", err)
	}

	// Emit events
	events := s.emitUnlikeEvents(ctx, note, cmd.UnlikerID)

	return &LikeResult{
		Status: note,
		Events: events,
	}, nil
}

// GetLikers retrieves users who liked a status
func (s *Service) GetLikers(ctx context.Context, query *GetLikersQuery) (*UsersResult, error) {
	// Get the status first to validate it exists
	_, err := s.noteRepo.GetStatus(ctx, query.StatusID)
	if err != nil {
		return nil, fmt.Errorf("status not found: %w", err)
	}

	// Get likers through repository interface
	users, pagination, err := s.getLikers(ctx, query.StatusID, query.Pagination)
	if err != nil {
		return nil, fmt.Errorf("failed to get likers: %w", err)
	}

	return &UsersResult{
		Users:      users,
		Pagination: pagination,
	}, nil
}

// Reblog/Announce Commands and Queries

// ReblogNoteCommand represents a request to reblog a status
type ReblogNoteCommand struct {
	StatusID    string `json:"status_id" validate:"required"`
	RebloggerID string `json:"reblogger_id" validate:"required"`
}

// UnreblogNoteCommand represents a request to unreblog a status
type UnreblogNoteCommand struct {
	StatusID      string `json:"status_id" validate:"required"`
	UnrebloggerID string `json:"unreblogger_id" validate:"required"`
}

// GetRebloggersQuery represents a request to get users who reblogged a status
type GetRebloggersQuery struct {
	StatusID   string                       `json:"status_id" validate:"required"`
	Pagination interfaces.PaginationOptions `json:"pagination"`
}

// ReblogNote creates a reblog/announce of a status
func (s *Service) ReblogNote(ctx context.Context, cmd *ReblogNoteCommand) (*LikeResult, error) {
	// Get the status first to validate it exists
	note, err := s.noteRepo.GetStatus(ctx, cmd.StatusID)
	if err != nil {
		return nil, fmt.Errorf("status not found: %w", err)
	}

	// Get reblogger's account
	reblogger, err := s.accountRepo.GetAccount(ctx, cmd.RebloggerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get reblogger account: %w", err)
	}

	// Create actor and object URLs for the reblog
	actorURL := fmt.Sprintf("https://%s/users/%s", s.domainName, reblogger.User.Username)
	objectURL := fmt.Sprintf("https://%s/users/%s/statuses/%s", s.domainName, note.AuthorUsername, cmd.StatusID)

	// Create the reblog through repository interface
	if err := s.createReblog(ctx, actorURL, objectURL, cmd.RebloggerID, cmd.StatusID); err != nil {
		return nil, fmt.Errorf("failed to reblog status: %w", err)
	}

	// Emit events
	events := s.emitReblogEvents(ctx, note, cmd.RebloggerID)

	return &LikeResult{
		Status: note,
		Events: events,
	}, nil
}

// UnreblogNote removes a reblog/announce of a status
func (s *Service) UnreblogNote(ctx context.Context, cmd *UnreblogNoteCommand) (*LikeResult, error) {
	// Get the status first to validate it exists
	note, err := s.noteRepo.GetStatus(ctx, cmd.StatusID)
	if err != nil {
		return nil, fmt.Errorf("status not found: %w", err)
	}

	// Get unreblogger's account
	unreblogger, err := s.accountRepo.GetAccount(ctx, cmd.UnrebloggerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get unreblogger account: %w", err)
	}

	// Create actor and object URLs for the unreblog
	actorURL := fmt.Sprintf("https://%s/users/%s", s.domainName, unreblogger.User.Username)
	objectURL := fmt.Sprintf("https://%s/users/%s/statuses/%s", s.domainName, note.AuthorUsername, cmd.StatusID)

	// Remove the reblog through repository interface
	if err := s.deleteReblog(ctx, actorURL, objectURL); err != nil {
		return nil, fmt.Errorf("failed to unreblog status: %w", err)
	}

	// Emit events
	events := s.emitUnreblogEvents(ctx, note, cmd.UnrebloggerID)

	return &LikeResult{
		Status: note,
		Events: events,
	}, nil
}

// GetRebloggers retrieves users who reblogged a status
func (s *Service) GetRebloggers(ctx context.Context, query *GetRebloggersQuery) (*UsersResult, error) {
	// Get the status first to validate it exists
	_, err := s.noteRepo.GetStatus(ctx, query.StatusID)
	if err != nil {
		return nil, fmt.Errorf("status not found: %w", err)
	}

	// Get rebloggers through repository interface
	users, pagination, err := s.getRebloggers(ctx, query.StatusID, query.Pagination)
	if err != nil {
		return nil, fmt.Errorf("failed to get rebloggers: %w", err)
	}

	return &UsersResult{
		Users:      users,
		Pagination: pagination,
	}, nil
}

// Pin Commands

// PinNoteCommand represents a request to pin a status
type PinNoteCommand struct {
	StatusID string `json:"status_id" validate:"required"`
	PinnerID string `json:"pinner_id" validate:"required"`
}

// UnpinNoteCommand represents a request to unpin a status
type UnpinNoteCommand struct {
	StatusID string `json:"status_id" validate:"required"`
	PinnerID string `json:"pinner_id" validate:"required"`
}

// PinNote pins a status to user's profile
func (s *Service) PinNote(ctx context.Context, cmd *PinNoteCommand) (*LikeResult, error) {
	// Get the status first to validate it exists
	note, err := s.noteRepo.GetStatus(ctx, cmd.StatusID)
	if err != nil {
		return nil, fmt.Errorf("status not found: %w", err)
	}

	// Verify the user owns the status (can only pin own statuses)
	if note.AuthorID != cmd.PinnerID {
		return nil, fmt.Errorf("unauthorized: can only pin own statuses")
	}

	// Pin the status through repository interface
	if err := s.pinStatus(ctx, cmd.PinnerID, cmd.StatusID); err != nil {
		return nil, fmt.Errorf("failed to pin status: %w", err)
	}

	// Emit pin events
	events := []*streaming.Event{
		{
			Type: streaming.StatusPinned,
			Stream: streaming.UserStreamName(cmd.PinnerID),
			Payload: map[string]interface{}{
				"status_id": cmd.StatusID,
				"pinner_id": cmd.PinnerID,
				"pinned_at": time.Now(),
			},
			Timestamp: time.Now(),
		},
	}

	return &LikeResult{
		Status: note,
		Events: events,
	}, nil
}

// UnpinNote unpins a status from user's profile
func (s *Service) UnpinNote(ctx context.Context, cmd *UnpinNoteCommand) (*LikeResult, error) {
	// Get the status first to validate it exists
	note, err := s.noteRepo.GetStatus(ctx, cmd.StatusID)
	if err != nil {
		return nil, fmt.Errorf("status not found: %w", err)
	}

	// Verify the user owns the status (can only unpin own statuses)
	if note.AuthorID != cmd.PinnerID {
		return nil, fmt.Errorf("unauthorized: can only unpin own statuses")
	}

	// Unpin the status through repository interface
	if err := s.unpinStatus(ctx, cmd.PinnerID, cmd.StatusID); err != nil {
		return nil, fmt.Errorf("failed to unpin status: %w", err)
	}

	// Emit unpin events
	events := []*streaming.Event{
		{
			Type: streaming.StatusUnpinned,
			Stream: streaming.UserStreamName(cmd.PinnerID),
			Payload: map[string]interface{}{
				"status_id": cmd.StatusID,
				"unpinner_id": cmd.PinnerID,
				"unpinned_at": time.Now(),
			},
			Timestamp: time.Now(),
		},
	}

	return &LikeResult{
		Status: note,
		Events: events,
	}, nil
}

// Mute Commands

// MuteNoteCommand represents a request to mute a status
type MuteNoteCommand struct {
	StatusID string `json:"status_id" validate:"required"`
	MuterID  string `json:"muter_id" validate:"required"`
}

// UnmuteNoteCommand represents a request to unmute a status
type UnmuteNoteCommand struct {
	StatusID string `json:"status_id" validate:"required"`
	MuterID  string `json:"muter_id" validate:"required"`
}

// MuteNote mutes a status for a user
func (s *Service) MuteNote(ctx context.Context, cmd *MuteNoteCommand) (*LikeResult, error) {
	// Get the status first to validate it exists
	note, err := s.noteRepo.GetStatus(ctx, cmd.StatusID)
	if err != nil {
		return nil, fmt.Errorf("status not found: %w", err)
	}

	// Mute the status through repository interface
	if err := s.muteStatus(ctx, cmd.MuterID, cmd.StatusID); err != nil {
		return nil, fmt.Errorf("failed to mute status: %w", err)
	}

	// Emit mute events (conversation muted)
	events := []*streaming.Event{
		{
			Type: streaming.ConversationUpdated,
			Stream: streaming.UserStreamName(cmd.MuterID),
			Payload: map[string]interface{}{
				"status_id": cmd.StatusID,
				"muter_id": cmd.MuterID,
				"action": "muted",
				"muted_at": time.Now(),
			},
			Timestamp: time.Now(),
		},
	}

	return &LikeResult{
		Status: note,
		Events: events,
	}, nil
}

// UnmuteNote unmutes a status for a user
func (s *Service) UnmuteNote(ctx context.Context, cmd *UnmuteNoteCommand) (*LikeResult, error) {
	// Get the status first to validate it exists
	note, err := s.noteRepo.GetStatus(ctx, cmd.StatusID)
	if err != nil {
		return nil, fmt.Errorf("status not found: %w", err)
	}

	// Unmute the status through repository interface
	if err := s.unmuteStatus(ctx, cmd.MuterID, cmd.StatusID); err != nil {
		return nil, fmt.Errorf("failed to unmute status: %w", err)
	}

	// Emit unmute events (conversation unmuted)
	events := []*streaming.Event{
		{
			Type: streaming.ConversationUpdated,
			Stream: streaming.UserStreamName(cmd.MuterID),
			Payload: map[string]interface{}{
				"status_id": cmd.StatusID,
				"unmuter_id": cmd.MuterID,
				"action": "unmuted",
				"unmuted_at": time.Now(),
			},
			Timestamp: time.Now(),
		},
	}

	return &LikeResult{
		Status: note,
		Events: events,
	}, nil
}

// Private helper methods - these need to be implemented to interface with repositories

func (s *Service) createLike(ctx context.Context, actorURL, objectURL string) error {
	_, err := s.likeRepo.CreateLike(ctx, actorURL, objectURL)
	if err != nil {
		return fmt.Errorf("failed to create like: %w", err)
	}
	return nil
}

func (s *Service) deleteLike(ctx context.Context, actorURL, objectURL string) error {
	err := s.likeRepo.DeleteLike(ctx, actorURL, objectURL)
	if err != nil {
		return fmt.Errorf("failed to delete like: %w", err)
	}
	return nil
}

func (s *Service) getLikers(ctx context.Context, statusID string, pagination interfaces.PaginationOptions) ([]*storage.Account, *interfaces.PaginatedResult[*storage.Account], error) {
	// Get likes for the status
	likes, nextCursor, err := s.likeRepo.GetObjectLikes(ctx, statusID, pagination.Limit, pagination.Cursor)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get likes: %w", err)
	}

	// Extract unique actor IDs and get their accounts
	accounts := make([]*storage.Account, 0, len(likes))
	for _, like := range likes {
		account, err := s.accountRepo.GetAccount(ctx, like.Actor)
		if err != nil {
			continue // Skip if account not found
		}
		accounts = append(accounts, account)
	}

	// Build pagination result
	result := &interfaces.PaginatedResult[*storage.Account]{
		Items:      accounts,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}

	return accounts, result, nil
}

func (s *Service) createReblog(ctx context.Context, actorURL, objectURL, _, _ string) error {
	announce := &storage.Announce{
		Actor:     actorURL,
		Object:    objectURL,
		CreatedAt: time.Now(),
	}
	err := s.socialRepo.CreateAnnounce(ctx, announce)
	if err != nil {
		return fmt.Errorf("failed to create reblog: %w", err)
	}
	return nil
}

func (s *Service) deleteReblog(ctx context.Context, actorURL, objectURL string) error {
	err := s.socialRepo.DeleteAnnounce(ctx, actorURL, objectURL)
	if err != nil {
		return fmt.Errorf("failed to delete reblog: %w", err)
	}
	return nil
}

func (s *Service) getRebloggers(ctx context.Context, statusID string, pagination interfaces.PaginationOptions) ([]*storage.Account, *interfaces.PaginatedResult[*storage.Account], error) {
	// Get announces for the status
	announces, nextCursor, err := s.socialRepo.GetStatusAnnounces(ctx, statusID, pagination.Limit, pagination.Cursor)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get announces: %w", err)
	}

	// Extract unique actor IDs and get their accounts
	accounts := make([]*storage.Account, 0, len(announces))
	for _, announce := range announces {
		account, err := s.accountRepo.GetAccount(ctx, announce.Actor)
		if err != nil {
			continue // Skip if account not found
		}
		accounts = append(accounts, account)
	}

	// Build pagination result
	result := &interfaces.PaginatedResult[*storage.Account]{
		Items:      accounts,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}

	return accounts, result, nil
}

func (s *Service) pinStatus(ctx context.Context, userID, statusID string) error {
	pin := &storage.StatusPin{
		Username:  userID,
		StatusID:  statusID,
		CreatedAt: time.Now(),
	}
	err := s.socialRepo.CreateStatusPin(ctx, pin)
	if err != nil {
		return fmt.Errorf("failed to pin status: %w", err)
	}
	return nil
}

func (s *Service) unpinStatus(ctx context.Context, userID, statusID string) error {
	err := s.socialRepo.DeleteStatusPin(ctx, userID, statusID)
	if err != nil {
		return fmt.Errorf("failed to unpin status: %w", err)
	}
	return nil
}

func (s *Service) muteStatus(ctx context.Context, userID, statusID string) error {
	// Muting a status actually mutes its conversation
	// First normalize the status ID to get the conversation ID
	conversationID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		conversationID = fmt.Sprintf("https://%s/objects/%s", s.domainName, statusID)
	}

	// Create conversation mute
	mute := &storage.ConversationMute{
		Username:       userID,
		ConversationID: conversationID,
		CreatedAt:      time.Now(),
	}

	// Store the mute
	if s.conversationRepo == nil {
		s.logger.Warn("conversation repository not available",
			zap.String("user_id", userID),
			zap.String("conversation_id", conversationID))
		return fmt.Errorf("conversation service not available")
	}

	err := s.conversationRepo.CreateConversationMute(ctx, mute)
	if err != nil {
		if strings.Contains(err.Error(), "already muted") {
			// Mute already exists, this is idempotent
			s.logger.Debug("conversation already muted",
				zap.String("user_id", userID),
				zap.String("conversation_id", conversationID))
			return nil
		}
		s.logger.Error("failed to mute conversation",
			zap.String("user_id", userID),
			zap.String("conversation_id", conversationID),
			zap.Error(err))
		return fmt.Errorf("failed to mute conversation: %w", err)
	}

	s.logger.Info("muted conversation",
		zap.String("user_id", userID),
		zap.String("conversation_id", conversationID))
	return nil
}

func (s *Service) unmuteStatus(ctx context.Context, userID, statusID string) error {
	// Unmuting a status unmutes its conversation
	// First normalize the status ID to get the conversation ID
	conversationID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		conversationID = fmt.Sprintf("https://%s/objects/%s", s.domainName, statusID)
	}

	// Check if conversation repository is available
	if s.conversationRepo == nil {
		s.logger.Warn("conversation repository not available",
			zap.String("user_id", userID),
			zap.String("conversation_id", conversationID))
		return fmt.Errorf("conversation service not available")
	}

	// Delete the conversation mute
	err := s.conversationRepo.DeleteConversationMute(ctx, userID, conversationID)
	if err != nil {
		// Not found is okay for unmute (idempotent)
		if strings.Contains(err.Error(), "not found") {
			s.logger.Debug("conversation mute not found",
				zap.String("user_id", userID),
				zap.String("conversation_id", conversationID))
			return nil
		}
		s.logger.Error("failed to unmute conversation",
			zap.String("user_id", userID),
			zap.String("conversation_id", conversationID),
			zap.Error(err))
		return fmt.Errorf("failed to unmute conversation: %w", err)
	}

	s.logger.Info("unmuted conversation",
		zap.String("user_id", userID),
		zap.String("conversation_id", conversationID))
	return nil
}

// getDirectTimeline retrieves the direct message timeline for a user
func (s *Service) getListTimeline(_ context.Context, query *ListNotesQuery) (*Result, error) {
	// For now, we'll just return an empty list since we don't have list service integration
	// In a complete implementation, this would:
	// 1. Get the list membership
	// 2. Query statuses from list members
	// 3. Apply the list's reply policy

	// Get statuses from list members (simplified for now)
	// This would need to be implemented with proper list service integration
	allStatuses := make([]*models.Status, 0)

	// In production, we would:
	// - Get list members from the list repository
	// - Query statuses from those members
	// - Apply list-specific filters (replies policy, etc.)

	s.logger.Warn("list timeline not fully implemented",
		zap.String("list_id", query.ListID),
		zap.String("viewer_id", query.ViewerID))

	// Sort by published date (newest first)
	for i := 0; i < len(allStatuses)-1; i++ {
		for j := i + 1; j < len(allStatuses); j++ {
			if allStatuses[i].PublishedAt.Before(allStatuses[j].PublishedAt) {
				allStatuses[i], allStatuses[j] = allStatuses[j], allStatuses[i]
			}
		}
	}

	// Apply pagination limits
	limit := query.Pagination.Limit
	if limit == 0 {
		limit = 20
	}
	if len(allStatuses) > limit {
		allStatuses = allStatuses[:limit]
	}

	// Create paginated result
	var nextCursor string
	hasMore := false
	if len(allStatuses) == limit && len(allStatuses) > 0 {
		nextCursor = allStatuses[len(allStatuses)-1].StatusID
		hasMore = true
	}

	return &Result{
		Notes: allStatuses,
		Pagination: &interfaces.PaginatedResult[*models.Status]{
			Items:      allStatuses,
			NextCursor: nextCursor,
			HasMore:    hasMore,
			Total:      int64(len(allStatuses)),
		},
	}, nil
}

func (s *Service) getDirectTimeline(ctx context.Context, query *ListNotesQuery) (*Result, error) {
	if s.conversationRepo == nil {
		return nil, fmt.Errorf("conversation service not available")
	}

	// Get user's conversations first
	opts := interfaces.PaginationOptions{
		Limit:  query.Pagination.Limit,
		Cursor: query.Pagination.Cursor,
	}
	result, err := s.conversationRepo.GetUserConversations(ctx, query.ViewerID, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversations: %w", err)
	}
	conversations := result.Items
	nextCursor := result.NextCursor

	// Collect all direct message statuses from conversations
	allStatuses := make([]*models.Status, 0)

	for _, conversation := range conversations {
		// Get the conversation thread - this returns direct messages
		threadResult, err := s.noteRepo.GetConversationThread(ctx, conversation.ID, interfaces.PaginationOptions{
			Limit: 50, // Get more messages per conversation to ensure we have recent ones
		})
		if err != nil {
			s.logger.Warn("failed to get conversation thread",
				zap.String("conversation_id", conversation.ID),
				zap.Error(err))
			continue
		}

		// Filter for direct messages only and add to result
		for _, status := range threadResult.Items {
			if status.IsDirect() {
				allStatuses = append(allStatuses, status)
			}
		}
	}

	// Sort by published date (newest first) - this is a simple sort
	// In a production system, you might want to use the database for proper sorting/pagination
	for i := 0; i < len(allStatuses)-1; i++ {
		for j := i + 1; j < len(allStatuses); j++ {
			if allStatuses[i].PublishedAt.Before(allStatuses[j].PublishedAt) {
				allStatuses[i], allStatuses[j] = allStatuses[j], allStatuses[i]
			}
		}
	}

	// Apply pagination limits
	limit := query.Pagination.Limit
	if limit <= 0 {
		limit = 20
	}

	var paginatedStatuses []*models.Status
	var finalCursor string

	if len(allStatuses) > limit {
		paginatedStatuses = allStatuses[:limit]
		finalCursor = allStatuses[limit-1].StatusID // Use last status ID as cursor
	} else {
		paginatedStatuses = allStatuses
		finalCursor = nextCursor // Use conversation cursor if we have fewer statuses than requested
	}

	// Build pagination result
	paginatedResult := &interfaces.PaginatedResult[*models.Status]{
		Items:      paginatedStatuses,
		NextCursor: finalCursor,
		HasMore:    finalCursor != "",
		Total:      int64(len(allStatuses)),
	}

	return &Result{
		Notes:      paginatedStatuses,
		Pagination: paginatedResult,
		Events:     []*streaming.Event{}, // No events for read operations
	}, nil
}

// GetFavoritedNotes returns the favorited/liked statuses for a user
func (s *Service) GetFavoritedNotes(ctx context.Context, query *ListNotesQuery) (*Result, error) {
	if query.ViewerID == "" {
		return nil, fmt.Errorf("viewer_id is required for favorited timeline")
	}

	// Get the viewer's actor to use their actor ID for likes
	account, err := s.accountRepo.GetAccount(ctx, query.ViewerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get viewer account: %w", err)
	}

	// Get liked objects using Like repository
	if s.likeRepo == nil {
		return nil, fmt.Errorf("like repository not available")
	}

	// Get the likes for the actor
	likes, nextCursor, err := s.likeRepo.GetActorLikes(ctx, account.Actor.ID, query.Pagination.Limit, query.Pagination.Cursor)
	if err != nil {
		return nil, fmt.Errorf("failed to get liked objects: %w", err)
	}

	// Collect the liked status IDs
	statusIDs := make([]string, 0, len(likes))
	for _, like := range likes {
		// Extract status ID from object ID if needed
		// Object IDs might be full URLs like https://example.com/users/test/statuses/123
		statusID := like.Object
		if strings.Contains(statusID, "/statuses/") {
			parts := strings.Split(statusID, "/statuses/")
			if len(parts) > 1 {
				statusID = parts[1]
			}
		}
		statusIDs = append(statusIDs, statusID)
	}

	// Get the actual status objects
	statuses, err := s.noteRepo.GetStatusesByIDs(ctx, statusIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get statuses: %w", err)
	}

	// Filter out deleted statuses
	filteredStatuses := make([]*models.Status, 0, len(statuses))
	for _, status := range statuses {
		if !status.Deleted {
			// Note: The engagement status (favorited, reblogged, bookmarked) will be handled
			// by the API layer when converting to API models, as these are viewer-specific
			// and not part of the core Status model
			filteredStatuses = append(filteredStatuses, status)
		}
	}

	// Build pagination result
	paginatedResult := &interfaces.PaginatedResult[*models.Status]{
		Items:      filteredStatuses,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
		Total:      int64(len(filteredStatuses)),
	}

	return &Result{
		Notes:      filteredStatuses,
		Pagination: paginatedResult,
		Events:     []*streaming.Event{}, // No events for read operations
	}, nil
}

// Scheduled Status Commands and Results

// CreateScheduledNoteCommand contains all data needed to create a scheduled note
type CreateScheduledNoteCommand struct {
	AuthorID    string    `json:"author_id" validate:"required"`
	Content     string    `json:"content" validate:"required,max=500"`
	Visibility  string    `json:"visibility" validate:"oneof=public unlisted private direct"`
	Sensitive   bool      `json:"sensitive"`
	Language    string    `json:"language"`
	InReplyToID string    `json:"in_reply_to_id"`
	MediaIDs    []string  `json:"media_ids"`
	ScheduledAt time.Time `json:"scheduled_at" validate:"required"`
}

// ScheduledNoteResult contains the result of scheduled note operations
type ScheduledNoteResult struct {
	ScheduledStatus *storage.ScheduledStatus `json:"scheduled_status"`
	Events          []*streaming.Event       `json:"events"`
}

// CreateScheduledNote creates a scheduled note to be published later
func (s *Service) CreateScheduledNote(ctx context.Context, cmd *CreateScheduledNoteCommand) (*ScheduledNoteResult, error) {
	s.logger.Info("creating scheduled note",
		zap.String("author_id", cmd.AuthorID),
		zap.Time("scheduled_at", cmd.ScheduledAt))

	// Validate command
	if err := s.validateCreateScheduledNoteCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Create scheduled status
	scheduled := &storage.ScheduledStatus{
		ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
		Username:    cmd.AuthorID,
		Status:      cmd.Content,
		MediaIDs:    cmd.MediaIDs,
		Sensitive:   cmd.Sensitive,
		Visibility:  cmd.Visibility,
		Language:    cmd.Language,
		InReplyToID: cmd.InReplyToID,
		ScheduledAt: cmd.ScheduledAt,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Store in repository
	if err := s.scheduledRepo.CreateScheduledStatus(ctx, scheduled); err != nil {
		return nil, fmt.Errorf("failed to create scheduled status: %w", err)
	}

	s.logger.Info("created scheduled note successfully",
		zap.String("scheduled_id", scheduled.ID),
		zap.String("author_id", cmd.AuthorID))

	// Emit events for scheduled status creation
	events := s.emitScheduledNoteCreatedEvents(ctx, scheduled)

	return &ScheduledNoteResult{
		ScheduledStatus: scheduled,
		Events:          events,
	}, nil
}

// validateCreateScheduledNoteCommand validates the create scheduled note command
func (s *Service) validateCreateScheduledNoteCommand(_ context.Context, cmd *CreateScheduledNoteCommand) error {
	if strings.TrimSpace(cmd.Content) == "" {
		return fmt.Errorf("content is required")
	}
	if len(cmd.Content) > 500 {
		return fmt.Errorf("content too long (max 500 characters)")
	}
	if cmd.ScheduledAt.Before(time.Now()) {
		return fmt.Errorf("scheduled time must be in the future")
	}
	validVisibilities := map[string]bool{
		"public": true, "unlisted": true, "private": true, "direct": true,
	}
	if !validVisibilities[cmd.Visibility] {
		return fmt.Errorf("invalid visibility: %s", cmd.Visibility)
	}
	return nil
}

// emitScheduledNoteCreatedEvents emits events for scheduled note creation
func (s *Service) emitScheduledNoteCreatedEvents(ctx context.Context, scheduled *storage.ScheduledStatus) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      "scheduled_status.created",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"scheduled_status": scheduled,
		},
	}

	// Emit to user stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", scheduled.Username)
	if err := s.publisher.PublishToUser(ctx, scheduled.Username, &userEvent); err != nil {
		s.logger.Error("failed to publish scheduled status created event", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}

// Search related queries and results

// GetSearchSuggestionsQuery represents a request to get search suggestions
type GetSearchSuggestionsQuery struct {
	Prefix string `json:"prefix" validate:"required,min=2"`
	Limit  int    `json:"limit" validate:"min=1,max=50"`
}

// GetSearchSuggestionsResult represents search suggestions
type GetSearchSuggestionsResult struct {
	Suggestions []*storage.SearchSuggestion `json:"suggestions"`
}

// GetSearchSuggestions retrieves search suggestions based on a prefix
func (s *Service) GetSearchSuggestions(ctx context.Context, query *GetSearchSuggestionsQuery) (*GetSearchSuggestionsResult, error) {
	// Get suggestions from the search repository
	modelSuggestions, err := s.searchRepo.GetSearchSuggestions(ctx, query.Prefix, query.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get search suggestions: %w", err)
	}

	// Convert models to storage types
	suggestions := make([]*storage.SearchSuggestion, len(modelSuggestions))
	for i, modelSuggestion := range modelSuggestions {
		suggestions[i] = &storage.SearchSuggestion{
			Type:        modelSuggestion.Type,
			Value:       modelSuggestion.Term,
			Score:       modelSuggestion.Score,
			Description: "", // Not available in model
		}
	}

	return &GetSearchSuggestionsResult{
		Suggestions: suggestions,
	}, nil
}

// Community Notes related types and methods

// CreateCommunityNoteCommand represents a request to create a community note
type CreateCommunityNoteCommand struct {
	Note *storage.CommunityNote `json:"note" validate:"required"`
}

// CreateCommunityNoteResult represents the result of creating a community note
type CreateCommunityNoteResult struct {
	Note *storage.CommunityNote `json:"note"`
}

// CreateCommunityNote creates a new community note
func (s *Service) CreateCommunityNote(ctx context.Context, cmd *CreateCommunityNoteCommand) (*CreateCommunityNoteResult, error) {
	// Create community note through repository
	if err := s.communityNoteRepo.CreateCommunityNote(ctx, cmd.Note); err != nil {
		return nil, fmt.Errorf("failed to create community note: %w", err)
	}

	return &CreateCommunityNoteResult{
		Note: cmd.Note,
	}, nil
}

// GetVisibleCommunityNotesQuery represents a request to get visible community notes
type GetVisibleCommunityNotesQuery struct {
	ObjectID string `json:"object_id" validate:"required"`
}

// GetVisibleCommunityNotesResult represents visible community notes
type GetVisibleCommunityNotesResult struct {
	Notes []*storage.CommunityNote `json:"notes"`
}

// GetVisibleCommunityNotes retrieves visible community notes for an object
func (s *Service) GetVisibleCommunityNotes(ctx context.Context, query *GetVisibleCommunityNotesQuery) (*GetVisibleCommunityNotesResult, error) {
	// Get visible notes through repository
	notes, err := s.communityNoteRepo.GetVisibleCommunityNotes(ctx, query.ObjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get visible community notes: %w", err)
	}

	return &GetVisibleCommunityNotesResult{
		Notes: notes,
	}, nil
}

// GetCommunityNoteQuery represents a request to get a specific community note
type GetCommunityNoteQuery struct {
	NoteID string `json:"note_id" validate:"required"`
}

// GetCommunityNoteResult represents a community note
type GetCommunityNoteResult struct {
	Note *storage.CommunityNote `json:"note"`
}

// GetCommunityNote retrieves a specific community note
func (s *Service) GetCommunityNote(ctx context.Context, query *GetCommunityNoteQuery) (*GetCommunityNoteResult, error) {
	// Get note through repository
	note, err := s.communityNoteRepo.GetCommunityNote(ctx, query.NoteID)
	if err != nil {
		return nil, fmt.Errorf("failed to get community note: %w", err)
	}

	return &GetCommunityNoteResult{
		Note: note,
	}, nil
}

// CreateCommunityNoteVoteCommand represents a request to vote on a community note
type CreateCommunityNoteVoteCommand struct {
	Vote *storage.CommunityNoteVote `json:"vote" validate:"required"`
}

// CreateCommunityNoteVoteResult represents the result of voting on a community note
type CreateCommunityNoteVoteResult struct {
	Vote *storage.CommunityNoteVote `json:"vote"`
}

// CreateCommunityNoteVote creates a vote for a community note
func (s *Service) CreateCommunityNoteVote(ctx context.Context, cmd *CreateCommunityNoteVoteCommand) (*CreateCommunityNoteVoteResult, error) {
	// Create vote through repository
	if err := s.communityNoteRepo.CreateCommunityNoteVote(ctx, cmd.Vote); err != nil {
		return nil, fmt.Errorf("failed to create community note vote: %w", err)
	}

	return &CreateCommunityNoteVoteResult{
		Vote: cmd.Vote,
	}, nil
}

// GetCommunityNotesByAuthorQuery represents a request to get community notes by author
type GetCommunityNotesByAuthorQuery struct {
	AuthorID string `json:"author_id" validate:"required"`
	Limit    int    `json:"limit" validate:"min=1,max=100"`
	Cursor   string `json:"cursor"`
}

// GetCommunityNotesByAuthorResult represents community notes by an author
type GetCommunityNotesByAuthorResult struct {
	Notes      []*storage.CommunityNote `json:"notes"`
	NextCursor string                   `json:"next_cursor,omitempty"`
}

// GetCommunityNotesByAuthor retrieves community notes by a specific author
func (s *Service) GetCommunityNotesByAuthor(ctx context.Context, query *GetCommunityNotesByAuthorQuery) (*GetCommunityNotesByAuthorResult, error) {
	// Get notes through repository
	notes, nextCursor, err := s.communityNoteRepo.GetCommunityNotesByAuthor(ctx, query.AuthorID, query.Limit, query.Cursor)
	if err != nil {
		return nil, fmt.Errorf("failed to get community notes by author: %w", err)
	}

	return &GetCommunityNotesByAuthorResult{
		Notes:      notes,
		NextCursor: nextCursor,
	}, nil
}

// CountNotesByAuthorQuery represents a request to count notes by an author
type CountNotesByAuthorQuery struct {
	AuthorID string `json:"author_id" validate:"required"`
}

// CountNotesByAuthor counts the number of notes by an author
func (s *Service) CountNotesByAuthor(ctx context.Context, authorID string) (int64, error) {
	// Count statuses through repository
	count, err := s.noteRepo.CountStatusesByAuthor(ctx, authorID)
	if err != nil {
		return 0, fmt.Errorf("failed to count statuses by author: %w", err)
	}

	return int64(count), nil
}

// GetUserTimelineQuery represents a request to get a user's timeline
type GetUserTimelineQuery struct {
	ActorID    string                       `json:"actor_id" validate:"required"`
	Pagination interfaces.PaginationOptions `json:"pagination"`
}

// GetUserTimelineResult represents a user's timeline
type GetUserTimelineResult struct {
	Items      []*models.Status `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

// GetUserTimeline retrieves a user's timeline
func (s *Service) GetUserTimeline(ctx context.Context, actorID string, opts interfaces.PaginationOptions) (*GetUserTimelineResult, error) {
	// Get timeline through status repository
	result, err := s.noteRepo.GetUserTimeline(ctx, actorID, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get user timeline: %w", err)
	}

	return &GetUserTimelineResult{
		Items:      result.Items,
		NextCursor: result.NextCursor,
	}, nil
}

// CountRepliesQuery represents a request to count replies to a status
type CountRepliesQuery struct {
	StatusID string `json:"status_id" validate:"required"`
}

// CountReplies counts the number of replies to a status
func (s *Service) CountReplies(ctx context.Context, statusID string) (int, error) {
	// Count replies through repository
	count, err := s.noteRepo.CountReplies(ctx, statusID)
	if err != nil {
		return 0, fmt.Errorf("failed to count replies: %w", err)
	}

	return count, nil
}

// GetBoostCountQuery represents a request to get boost count
type GetBoostCountQuery struct {
	StatusID string `json:"status_id" validate:"required"`
}

// GetBoostCount gets the number of boosts for a status
func (s *Service) GetBoostCount(ctx context.Context, statusID string) (int64, error) {
	// Get boost count through like repository
	count, err := s.likeRepo.GetBoostCount(ctx, statusID)
	if err != nil {
		return 0, fmt.Errorf("failed to get boost count: %w", err)
	}

	return int64(count), nil
}

// GetLikeCountQuery represents a request to get like count
type GetLikeCountQuery struct {
	StatusID string `json:"status_id" validate:"required"`
}

// GetLikeCount gets the number of likes for a status
func (s *Service) GetLikeCount(ctx context.Context, statusID string) (int64, error) {
	// Get like count through like repository
	count, err := s.likeRepo.GetLikeCount(ctx, statusID)
	if err != nil {
		return 0, fmt.Errorf("failed to get like count: %w", err)
	}

	return int64(count), nil
}

// HasLikedQuery represents a request to check if a user has liked a status
type HasLikedQuery struct {
	UserID   string `json:"user_id" validate:"required"`
	StatusID string `json:"status_id" validate:"required"`
}

// HasLiked checks if a user has liked a status
func (s *Service) HasLiked(ctx context.Context, userID, statusID string) (bool, error) {
	// Check through like repository
	hasLiked, err := s.likeRepo.HasLiked(ctx, userID, statusID)
	if err != nil {
		return false, fmt.Errorf("failed to check if user has liked: %w", err)
	}

	return hasLiked, nil
}

// HasRebloggedQuery represents a request to check if a user has reblogged a status
type HasRebloggedQuery struct {
	UserID   string `json:"user_id" validate:"required"`
	StatusID string `json:"status_id" validate:"required"`
}

// HasReblogged checks if a user has reblogged a status
func (s *Service) HasReblogged(ctx context.Context, userID, statusID string) (bool, error) {
	// Check through like repository
	hasReblogged, err := s.likeRepo.HasReblogged(ctx, userID, statusID)
	if err != nil {
		return false, fmt.Errorf("failed to check if user has reblogged: %w", err)
	}

	return hasReblogged, nil
}

// IsBookmarkedQuery represents a request to check if a user has bookmarked a status
type IsBookmarkedQuery struct {
	UserID   string `json:"user_id" validate:"required"`
	StatusID string `json:"status_id" validate:"required"`
}

// IsBookmarked checks if a user has bookmarked a status
func (s *Service) IsBookmarked(ctx context.Context, userID, statusID string) (bool, error) {
	// Check through user repository
	isBookmarked, err := s.userRepo.IsBookmarked(ctx, userID, statusID)
	if err != nil {
		return false, fmt.Errorf("failed to check if user has bookmarked: %w", err)
	}

	return isBookmarked, nil
}

// createPollForStatus creates a poll associated with a status
func (s *Service) createPollForStatus(ctx context.Context, cmd *CreateNoteCommand, statusID string) error {
	// Calculate expiration time
	var expiresAt *time.Time
	if cmd.PollExpiresIn > 0 {
		expiry := time.Now().Add(time.Duration(cmd.PollExpiresIn) * time.Second)
		expiresAt = &expiry
	} else {
		// Default to 24 hours if no expiration specified
		expiry := time.Now().Add(24 * time.Hour)
		expiresAt = &expiry
	}

	// Create poll
	poll := &storage.Poll{
		StatusID:   statusID,
		CreatedBy:  fmt.Sprintf("https://%s/users/%s", s.domainName, cmd.AuthorID),
		Options:    cmd.PollOptions,
		Multiple:   cmd.PollMultiple,
		HideTotals: cmd.PollHideTotals,
		ExpiresAt:  expiresAt,
	}

	return s.pollRepo.CreatePoll(ctx, poll)
}
