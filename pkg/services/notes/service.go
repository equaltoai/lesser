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
	noteRepo     interfaces.NoteRepository
	accountRepo  interfaces.AccountRepository
	publisher    streaming.Publisher
	logger       *zap.Logger
	domainName   string
	federation   FederationService // Interface to be defined
}

// FederationService defines the interface for federation operations
type FederationService interface {
	QueueActivity(ctx context.Context, activity *activitypub.Activity) error
}

// NewService creates a new Notes Service with the required dependencies
func NewService(
	noteRepo interfaces.NoteRepository,
	accountRepo interfaces.AccountRepository,
	publisher streaming.Publisher,
	federation FederationService,
	logger *zap.Logger,
	domainName string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		noteRepo:    noteRepo,
		accountRepo: accountRepo,
		publisher:   publisher,
		federation:  federation,
		logger:      logger,
		domainName:  domainName,
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
	ToRecipients   []string `json:"to_recipients"`
	CcRecipients   []string `json:"cc_recipients"`
	BtoRecipients  []string `json:"bto_recipients"`
	BccRecipients  []string `json:"bcc_recipients"`
}

// UpdateNoteCommand contains all data needed to update an existing note
type UpdateNoteCommand struct {
	StatusID   string   `json:"status_id" validate:"required"`
	Content    string   `json:"content" validate:"required,max=5000"`
	Sensitive  bool     `json:"sensitive"`
	Language   string   `json:"language"`
	MediaIDs   []string `json:"media_ids"`
	UpdaterID  string   `json:"updater_id" validate:"required"` // Must be author
}

// DeleteNoteCommand contains data needed to delete a note
type DeleteNoteCommand struct {
	StatusID  string `json:"status_id" validate:"required"`
	DeleterID string `json:"deleter_id" validate:"required"` // Must be author or admin
	Reason    string `json:"reason"` // Optional reason for admin deletions
}

// GetNoteQuery contains parameters for retrieving a single note
type GetNoteQuery struct {
	StatusID string `json:"status_id" validate:"required"`
	ViewerID string `json:"viewer_id"` // User requesting the note (for privacy checks)
}

// ListNotesQuery contains parameters for listing notes with various filters
type ListNotesQuery struct {
	ViewerID       string                        `json:"viewer_id"` // User requesting the timeline
	TimelineType   string                        `json:"timeline_type" validate:"required,oneof=home public local conversations hashtag user"`
	AuthorID       string                        `json:"author_id"`       // For user timelines
	Hashtag        string                        `json:"hashtag"`         // For hashtag timelines
	ConversationID string                        `json:"conversation_id"` // For conversation threads
	ParentID       string                        `json:"parent_id"`       // For reply threads
	Pagination     interfaces.PaginationOptions `json:"pagination"`
	OnlyMedia      bool                          `json:"only_media"`
	ExcludeReplies bool                          `json:"exclude_replies"`
	ExcludeReblogs bool                          `json:"exclude_reblogs"`
	PinnedOnly     bool                          `json:"pinned_only"`
}

// Result structs for operations

// NoteResult contains a note and associated events that were emitted
type NoteResult struct {
	Note   *models.Status    `json:"note"`
	Events []*streaming.Event `json:"events"`
}

// Result contains multiple notes and pagination information
type Result struct {
	Notes      []*models.Status                       `json:"notes"`
	Pagination *interfaces.PaginatedResult[*models.Status] `json:"pagination"`
	Events     []*streaming.Event                      `json:"events"`
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
		// TODO: Add admin check here when admin service is available
		return fmt.Errorf("unauthorized: only the author can delete their posts")
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
func (s *Service) GetNote(ctx context.Context, query *GetNoteQuery) (*models.Status, error) {
	s.logger.Debug("getting note",
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

	// Check privacy/visibility
	if !status.IsVisibleTo(query.ViewerID) {
		return nil, fmt.Errorf("status not found") // Don't reveal it exists
	}

	// Sanitize for the viewer
	sanitized := status.SanitizeForActor(query.ViewerID)

	return sanitized, nil
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
	case "hashtag":
		if query.Hashtag == "" {
			return nil, fmt.Errorf("hashtag timeline requires hashtag")
		}
		result, err = s.noteRepo.GetStatusesByHashtag(ctx, query.Hashtag, query.Pagination)
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