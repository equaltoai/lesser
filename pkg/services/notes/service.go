// Package notes provides the core Notes Service for the Lesser project's API alignment.
// This service handles all status/post operations including creation, updates, deletion,
// and timeline operations. It emits appropriate events for real-time streaming and
// queues federation delivery for remote followers.
package notes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	svcErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/security/htmlsafe"
	"github.com/equaltoai/lesser/pkg/services/conversations"
	"github.com/equaltoai/lesser/pkg/services/notifications"
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
	noteRepo          interfaces.StatusRepository
	accountRepo       interfaces.AccountRepository
	bookmarkRepo      *repositories.BookmarkRepository
	relationshipRepo  interfaces.ConcreteRelationshipRepository
	mediaRepo         interfaces.MediaRepository
	likeRepo          *repositories.LikeRepository
	socialRepo        interfaces.SocialRepository
	conversationRepo  interfaces.ConversationRepository
	objectRepo        interfaces.ObjectRepository
	searchRepo        *repositories.SearchRepository
	communityNoteRepo *repositories.CommunityNoteRepository
	userRepo          interfaces.UserRepository
	pollRepo          *repositories.PollRepository
	scheduledRepo     ScheduledStatusRepository
	publisher         streaming.Publisher
	analytics         AnalyticsService
	logger            *zap.Logger
	domainName        string
	federation        FederationService // Interface to be defined
	notifications     notificationsService

	// Business logic services
	businessLogic    *common.BusinessLogicService
	activityPubLogic *common.ActivityPubBusinessLogic
	mastodonLogic    *common.MastodonBusinessLogic
}

type notificationsService interface {
	CreateNotification(ctx context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error)
}

// ScheduledStatusRepository defines the interface for scheduled status operations
type ScheduledStatusRepository interface {
	CreateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error
	GetScheduledStatus(ctx context.Context, id string) (*storage.ScheduledStatus, error)
	GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error)
	UpdateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error
	DeleteScheduledStatus(ctx context.Context, id string) error
}

// ensureAuthorUsername extracts username from Actor ID and populates AuthorUsername if empty
func (s *Service) ensureAuthorUsername(ctx context.Context, status *models.Status) {
	if status.AuthorUsername == "" && status.AuthorID != "" {
		// Extract username from Actor ID - try multiple formats
		// Format 1: https://domain.com/users/username
		if strings.Contains(status.AuthorID, "/users/") {
			parts := strings.Split(status.AuthorID, "/users/")
			if len(parts) == 2 {
				status.AuthorUsername = strings.Split(parts[1], "/")[0] // Get username before any trailing slash
			}
		}
		// Format 2: https://domain.com/@username
		if status.AuthorUsername == "" && strings.Contains(status.AuthorID, "/@") {
			parts := strings.Split(status.AuthorID, "/@")
			if len(parts) == 2 {
				status.AuthorUsername = strings.Split(parts[1], "/")[0]
			}
		}
		// Format 3: Just take the last path segment
		if status.AuthorUsername == "" {
			parts := strings.Split(strings.TrimSuffix(status.AuthorID, "/"), "/")
			if len(parts) > 0 {
				status.AuthorUsername = parts[len(parts)-1]
			}
		}
		// Last resort: try to get from account using AuthorID
		if status.AuthorUsername == "" && s.accountRepo != nil {
			if account, err := s.accountRepo.GetAccount(ctx, status.AuthorID); err == nil && account != nil {
				status.AuthorUsername = account.User.Username
			}
		}
	}
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

// streamingEventEmitter adapts streaming.Publisher to common.EventEmitter interface
type streamingEventEmitter struct {
	publisher streaming.Publisher
}

// EmitEvents implements the common.EventEmitter interface
func (e *streamingEventEmitter) EmitEvents(ctx context.Context, events []*common.StreamingEvent) error {
	// Convert common.StreamingEvent to streaming.Event
	streamingEvents := make([]*streaming.Event, len(events))
	for i, event := range events {
		streamingEvents[i] = &streaming.Event{
			Type:      event.Type,
			Stream:    "user", // Default stream, will be overridden
			Timestamp: event.Timestamp,
			Payload:   event.Metadata,
		}
	}

	// Emit using the publisher
	for _, event := range streamingEvents {
		if err := e.publisher.PublishToStream(ctx, event.Stream, event); err != nil {
			return err
		}
	}

	return nil
}

// NewService creates a new Notes Service with the required dependencies
func NewService(
	noteRepo interfaces.StatusRepository,
	accountRepo interfaces.AccountRepository,
	bookmarkRepo *repositories.BookmarkRepository,
	relationshipRepo interfaces.ConcreteRelationshipRepository,
	mediaRepo interfaces.MediaRepository,
	likeRepo *repositories.LikeRepository,
	socialRepo interfaces.SocialRepository,
	conversationRepo interfaces.ConversationRepository,
	objectRepo interfaces.ObjectRepository,
	searchRepo *repositories.SearchRepository,
	communityNoteRepo *repositories.CommunityNoteRepository,
	userRepo interfaces.UserRepository,
	pollRepo *repositories.PollRepository,
	publisher streaming.Publisher,
	analytics AnalyticsService,
	federation FederationService,
	notifier notificationsService,
	logger *zap.Logger,
	domainName string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Create business logic services
	businessLogic := common.NewBusinessLogicService(logger, &streamingEventEmitter{publisher: publisher}, domainName)

	activityPubConfig := &common.FederationConfig{
		Domain:         domainName,
		UserAgent:      "Lesser/1.0",
		MaxRetries:     3,
		RequestTimeout: 30 * time.Second,
	}
	activityPubLogic := common.NewActivityPubBusinessLogic(activityPubConfig, logger)

	mastodonConfig := common.DefaultMastodonConfig()
	mastodonConfig.Domain = domainName
	mastodonLogic := common.NewMastodonBusinessLogic(mastodonConfig, logger)

	return &Service{
		noteRepo:          noteRepo,
		accountRepo:       accountRepo,
		bookmarkRepo:      bookmarkRepo,
		relationshipRepo:  relationshipRepo,
		mediaRepo:         mediaRepo,
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
		notifications:     notifier,
		logger:            logger,
		domainName:        domainName,
		businessLogic:     businessLogic,
		activityPubLogic:  activityPubLogic,
		mastodonLogic:     mastodonLogic,
	}
}

// Command structs for operations

// CreateNoteCommand contains all data needed to create a new note
type CreateNoteCommand struct {
	AuthorID            string   `json:"author_id" validate:"required"`
	Content             string   `json:"content" validate:"required,max=5000"`
	Visibility          string   `json:"visibility" validate:"required,oneof=public unlisted private direct"`
	Sensitive           bool     `json:"sensitive"`
	SpoilerText         string   `json:"spoiler_text"`
	Language            string   `json:"language"`
	InReplyToID         string   `json:"in_reply_to_id"`
	ConversationID      string   `json:"conversation_id"`
	MediaIDs            []string `json:"media_ids"`
	PollOptions         []string `json:"poll_options"`
	PollExpiresIn       int      `json:"poll_expires_in"`  // Duration in seconds
	PollMultiple        bool     `json:"poll_multiple"`    // Allow multiple choices
	PollHideTotals      bool     `json:"poll_hide_totals"` // Hide vote counts until poll ends
	ToRecipients        []string `json:"to_recipients"`
	CcRecipients        []string `json:"cc_recipients"`
	BtoRecipients       []string `json:"bto_recipients"`
	BccRecipients       []string `json:"bcc_recipients"`
	QuoteTargetStatusID string   `json:"quote_target_status_id"`
	QuoteTargetAuthorID string   `json:"quote_target_author_id"`

	// Lesser extension: per-status agent transparency metadata (stored on the underlying Note).
	AgentAttribution *activitypub.AgentPostAttribution `json:"agent_attribution,omitempty"`
}

// UpdateNoteCommand contains all data needed to update an existing note
type UpdateNoteCommand struct {
	StatusID    string   `json:"status_id" validate:"required"`
	Content     string   `json:"content" validate:"required,max=5000"`
	Sensitive   bool     `json:"sensitive"`
	SpoilerText string   `json:"spoiler_text"`
	Language    string   `json:"language"`
	MediaIDs    []string `json:"media_ids"`
	UpdaterID   string   `json:"updater_id" validate:"required"` // Must be author
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

	rawContent := cmd.Content

	// Validate the command
	if err := s.validateCreateCommand(ctx, cmd); err != nil {
		return nil, err
	}

	// Enforce "HTML-by-contract" invariants at write time. (Mastodon-compatible clients render `content` as HTML.)
	sanitizedCmd := *cmd
	sanitizedCmd.Content = strings.TrimSpace(htmlsafe.SanitizeHTMLByContract(cmd.Content))
	sanitizedCmd.SpoilerText = strings.TrimSpace(htmlsafe.SanitizeHTMLByContract(cmd.SpoilerText))

	// Get author account
	author, err := s.accountRepo.GetAccount(ctx, cmd.AuthorID)
	if err != nil {
		return nil, ErrGetAuthorAccount
	}

	s.normalizeCreateAudience(&sanitizedCmd, author)

	// Generate unique status ID
	statusID := uuid.New().String()

	// Create ActivityPub Note
	note := s.buildActivityPubNote(&sanitizedCmd, statusID, author)

	publishedAt := time.Now()
	note.Published = &publishedAt

	// Enrich note with hashtags
	hashtagTags, normalizedHashtags := s.buildHashtagTags(rawContent)
	if len(hashtagTags) > 0 {
		note.Tag = append(note.Tag, hashtagTags...)
	}

	mentionTags, mentionedUsers := s.buildMentionTags(ctx, rawContent, author)
	if len(mentionTags) > 0 {
		note.Tag = append(note.Tag, mentionTags...)
		s.addMentionAudience(note, mentionTags)
	}

	// Attach media if provided
	attachments, mediaIDsToMark, err := s.prepareMediaAttachments(ctx, author, sanitizedCmd.MediaIDs)
	if err != nil {
		return nil, err
	}
	if len(attachments) > 0 {
		note.Attachment = attachments
	}

	status := s.composeStatus(&sanitizedCmd, author, statusID, note, normalizedHashtags, attachments, publishedAt)
	status.ConversationID = resolveConversationID(ctx, status, s.lookupParentStatus)

	// Store the status
	if err := s.noteRepo.CreateStatus(ctx, status); err != nil {
		requestErr := errors.Join(ErrCreateStatus, err)
		s.logger.Error("failed to persist created status",
			zap.String("status_id", statusID),
			zap.String("conversation_id", status.ConversationID),
			zap.Strings("root_causes", common.ErrorLeafMessages(err)),
			zap.Error(requestErr))
		return nil, requestErr
	}

	// Mark media attachments as used after successful persistence
	s.markMediaAsUsed(ctx, statusID, mediaIDsToMark)
	s.recordStatusCreationAnalytics(ctx, status)
	s.handlePollCreation(ctx, cmd, statusID)

	s.logger.Info("created note successfully",
		zap.String("status_id", statusID),
		zap.String("conversation_id", status.ConversationID))

	// Ensure AuthorUsername is populated before emitting events
	s.ensureAuthorUsername(ctx, status)
	s.notifyReply(ctx, status)
	s.notifyMentions(ctx, status, mentionedUsers)

	// Emit events and queue federation
	events := s.emitStatusCreatedEvents(ctx, status)
	s.queueFederationDelivery(ctx, status, "Create")

	return &NoteResult{
		Note:   status,
		Events: events,
	}, nil
}

func (s *Service) composeStatus(cmd *CreateNoteCommand, author *storage.Account, statusID string, note *activitypub.Note, hashtags []string, attachments []activitypub.Attachment, timestamp time.Time) *models.Status {
	toRecipients := appendUniqueAudience(nil, cmd.ToRecipients...)
	ccRecipients := appendUniqueAudience(nil, cmd.CcRecipients...)
	btoRecipients := appendUniqueAudience(nil, cmd.BtoRecipients...)
	bccRecipients := appendUniqueAudience(nil, cmd.BccRecipients...)

	if note != nil {
		toRecipients = appendUniqueAudience(nil, note.To...)
		ccRecipients = appendUniqueAudience(nil, note.CC...)
		btoRecipients = appendUniqueAudience(nil, note.BTo...)
		bccRecipients = appendUniqueAudience(nil, note.BCC...)
	}

	status := &models.Status{
		StatusID:            statusID,
		Note:                note,
		AuthorID:            cmd.AuthorID,
		AuthorUsername:      author.User.Username,
		Content:             cmd.Content,
		Visibility:          cmd.Visibility,
		Sensitive:           cmd.Sensitive,
		Language:            cmd.Language,
		InReplyToID:         cmd.InReplyToID,
		ConversationID:      cmd.ConversationID,
		ToRecipients:        toRecipients,
		CcRecipients:        ccRecipients,
		BtoRecipients:       btoRecipients,
		BccRecipients:       bccRecipients,
		QuoteTargetStatusID: cmd.QuoteTargetStatusID,
		QuoteTargetAuthorID: cmd.QuoteTargetAuthorID,
		PublishedAt:         timestamp,
		CreatedAt:           timestamp,
		ModifiedAt:          timestamp,
	}

	if len(hashtags) > 0 {
		status.Hashtags = hashtags
	}

	if len(attachments) > 0 {
		status.MediaCount = len(attachments)
	}

	return status
}

type parentStatusFetcher func(context.Context, string) (*models.Status, error)

func resolveConversationID(ctx context.Context, status *models.Status, fetch parentStatusFetcher) string {
	if status == nil {
		return ""
	}

	if status.ConversationID != "" {
		return status.ConversationID
	}

	if status.InReplyToID == "" {
		return status.StatusID
	}

	if fetch == nil {
		return status.InReplyToID
	}

	parent, err := fetch(ctx, status.InReplyToID)
	if err == nil && parent != nil && parent.ConversationID != "" {
		return parent.ConversationID
	}

	return status.InReplyToID
}

func (s *Service) lookupParentStatus(ctx context.Context, statusID string) (*models.Status, error) {
	if s.noteRepo == nil || statusID == "" {
		return nil, fmt.Errorf("note repository unavailable")
	}
	return s.noteRepo.GetStatus(ctx, statusID)
}

func (s *Service) markMediaAsUsed(ctx context.Context, statusID string, mediaIDs []string) {
	if len(mediaIDs) == 0 || s.mediaRepo == nil {
		return
	}

	for _, mediaID := range mediaIDs {
		if err := s.mediaRepo.MarkMediaUsed(ctx, mediaID); err != nil {
			s.logger.Warn("failed to mark media attachment as used",
				zap.String("status_id", statusID),
				zap.String("media_id", mediaID),
				zap.Error(err))
		}
	}
}

func (s *Service) recordStatusCreationAnalytics(ctx context.Context, status *models.Status) {
	if s.analytics == nil || status == nil {
		return
	}

	if len(status.Hashtags) > 0 && status.Note != nil {
		if err := s.analytics.RecordHashtagUsage(ctx, status.Hashtags, status.Note.ID, status.AuthorID); err != nil {
			s.logger.Warn("failed to record hashtag usage",
				zap.String("status_id", status.StatusID),
				zap.Strings("hashtags", status.Hashtags),
				zap.Error(err))
		}
	}

	activityType := "post"
	if status.InReplyToID != "" {
		activityType = "comment"
	}

	if err := s.analytics.RecordInstanceActivity(ctx, activityType, time.Now()); err != nil {
		s.logger.Warn("failed to record instance metrics",
			zap.String("activity_type", activityType),
			zap.String("status_id", status.StatusID),
			zap.Error(err))
	}
}

func (s *Service) handlePollCreation(ctx context.Context, cmd *CreateNoteCommand, statusID string) {
	if common.ValidateSliceNotEmpty("cmd.PollOptions", cmd.PollOptions) != nil {
		return
	}

	if err := s.createPollForStatus(ctx, cmd, statusID); err != nil {
		s.logger.Error("failed to create poll for status",
			zap.String("status_id", statusID),
			zap.Error(err))
	}
}

// UpdateNote updates an existing note, validates permission, stores changes, and emits events
func (s *Service) UpdateNote(ctx context.Context, cmd *UpdateNoteCommand) (*NoteResult, error) {
	s.logger.Info("updating note",
		zap.String("status_id", cmd.StatusID),
		zap.String("updater_id", cmd.UpdaterID))

	// Get existing status
	status, err := s.noteRepo.GetStatus(ctx, cmd.StatusID)
	if err != nil {
		return nil, ErrGetStatus
	}

	// Verify permission (only author can update)
	// Compare usernames, not full IDs (AuthorUsername is just the username, AuthorID is the full URL)
	if status.AuthorUsername != cmd.UpdaterID {
		s.logger.Warn("user cannot update post owned by another user",
			zap.String("updater_id", cmd.UpdaterID),
			zap.String("author_username", status.AuthorUsername),
			zap.String("author_id", status.AuthorID))
		return nil, common.ErrForbidden(ErrCannotUpdatePostOwnedByOther)
	}

	// Validate the update
	if err := s.validateUpdateCommand(ctx, cmd); err != nil {
		return nil, err
	}

	// Enforce "HTML-by-contract" invariants at write time.
	sanitizedContent := strings.TrimSpace(htmlsafe.SanitizeHTMLByContract(cmd.Content))
	sanitizedSpoiler := strings.TrimSpace(htmlsafe.SanitizeHTMLByContract(cmd.SpoilerText))

	// Update status fields
	status.Content = sanitizedContent
	status.Sensitive = cmd.Sensitive
	status.Language = cmd.Language
	status.UpdatedAt = time.Now()

	// Update the ActivityPub Note if present
	if status.Note != nil {
		status.Note.Content = sanitizedContent
		status.Note.Sensitive = cmd.Sensitive
		status.Note.Summary = sanitizedSpoiler
		now := time.Now()
		status.Note.Updated = &now
	}

	// Store the updated status
	if err := s.noteRepo.UpdateStatus(ctx, status); err != nil {
		return nil, ErrUpdateStatus
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
		return ErrGetStatus
	}

	// Ensure AuthorUsername is populated (may be empty if status was loaded from DB)
	s.ensureAuthorUsername(ctx, status)

	// Check if already deleted
	if status.Deleted {
		return nil // Idempotent operation
	}

	// Direct messages (visibility=direct) must not be globally deleted by the author.
	// DM v1 supports delete-for-me only (see conversations.DeleteMessage).
	isAdmin := false
	if status.Visibility == models.VisibilityDirect || status.AuthorUsername != cmd.DeleterID {
		if s.userRepo != nil {
			if deleter, err := s.userRepo.GetUser(ctx, cmd.DeleterID); err == nil && deleter != nil {
				isAdmin = deleter.Role == "admin"
			}
		}
	}
	if status.Visibility == models.VisibilityDirect && !isAdmin {
		return common.ErrForbidden(errors.New("direct messages cannot be deleted via deleteObject; use deleteMessage"))
	}

	// Verify permission (author or admin)
	// Compare usernames, not full IDs (AuthorUsername is just the username, AuthorID is the full URL)
	if status.AuthorUsername != cmd.DeleterID {
		// Check if deleter is an admin
		if !isAdmin {
			s.logger.Warn("user cannot delete post owned by another user without admin privileges",
				zap.String("deleter_id", cmd.DeleterID),
				zap.String("author_username", status.AuthorUsername),
				zap.String("author_id", status.AuthorID))
			return common.ErrForbidden(ErrCannotDeletePostOwnedByOther)
		}
	}

	// Perform soft delete
	now := time.Now()
	status.Deleted = true
	status.DeletedAt = &now
	status.ModifiedAt = now

	// Store the deletion - use UpdateStatus which uses UpdateBuilder() to prevent Note field corruption
	// No need to call UpdateKeys() here - keys are already set from creation and we're only updating Deleted flag
	if err := s.noteRepo.UpdateStatus(ctx, status); err != nil {
		return ErrDeleteStatus
	}

	s.logger.Info("deleted note successfully",
		zap.String("status_id", cmd.StatusID))

	// Emit events and queue federation tombstone
	s.emitStatusDeletedEvents(ctx, status)
	s.queueFederationTombstone(ctx, status)

	return nil
}

// GetNote retrieves a single note for public contexts.
//
// It enforces visibility as if the viewer is unauthenticated (public/unlisted only).
// Use GetNoteWithViewer for viewer-aware access to private/direct content.
func (s *Service) GetNote(ctx context.Context, statusID string) (*models.Status, error) {
	s.logger.Debug("getting note",
		zap.String("status_id", statusID))

	status, err := s.resolveStatusForRead(ctx, statusID)
	if err != nil {
		if statusLookupNotFound(err) {
			return nil, ErrStatusNotFound
		}
		return nil, ErrGetStatus
	}

	// Check if deleted
	if status.Deleted {
		return nil, ErrStatusNotFound // Don't reveal it was deleted
	}

	canView, err := s.checkViewPermissions(ctx, status, "")
	if err != nil {
		return nil, ErrCheckViewPermissions
	}
	if !canView {
		return nil, ErrStatusNotFound
	}

	return status, nil
}

// GetNoteWithViewer retrieves a note with viewer context for privacy checking
func (s *Service) GetNoteWithViewer(ctx context.Context, query *GetNoteQuery) (*models.Status, error) {
	if err := common.ValidateRequiredParam("status_id", query.StatusID); err != nil {
		return nil, ErrStatusIDRequired
	}

	s.logger.Debug("getting note with viewer context",
		zap.String("status_id", query.StatusID),
		zap.String("viewer_id", query.ViewerID))

	status, err := s.resolveStatusForRead(ctx, query.StatusID)
	if err != nil {
		if statusLookupNotFound(err) {
			return nil, ErrStatusNotFound
		}
		return nil, ErrGetStatus
	}

	// Check if deleted
	if status.Deleted {
		return nil, ErrStatusNotFound // Don't reveal it was deleted
	}

	// Check privacy permissions
	canView, err := s.checkViewPermissions(ctx, status, query.ViewerID)
	if err != nil {
		return nil, ErrCheckViewPermissions
	}

	if !canView {
		return nil, ErrStatusNotFound // Don't reveal access denied
	}

	return status, nil
}

func (s *Service) resolveStatusForRead(ctx context.Context, statusID string) (*models.Status, error) {
	if s.noteRepo == nil {
		return nil, ErrStatusRepositoryUnavailable
	}

	statusID = strings.TrimSpace(statusID)
	if statusID == "" {
		return nil, storage.ErrNotFound
	}

	if strings.HasPrefix(strings.ToLower(statusID), "http://") || strings.HasPrefix(strings.ToLower(statusID), "https://") {
		status, err := s.noteRepo.GetStatusByURL(ctx, statusID)
		if err == nil && status != nil {
			return status, nil
		}
		if err != nil && !statusLookupNotFound(err) {
			return nil, err
		}
	}

	var lastErr error
	for _, candidate := range models.StatusLookupCandidatesForDomain(statusID, s.domainName) {
		status, err := s.noteRepo.GetStatus(ctx, candidate)
		if err == nil && status != nil {
			return status, nil
		}
		if err != nil && !statusLookupNotFound(err) {
			return nil, err
		}
		if err != nil {
			lastErr = err
		}
	}

	if lastErr == nil {
		lastErr = storage.ErrNotFound
	}

	return nil, lastErr
}

func statusLookupNotFound(err error) bool {
	if err == nil {
		return false
	}
	if common.IsNotFound(err) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

// checkViewPermissions implements comprehensive privacy checking
func (s *Service) checkViewPermissions(ctx context.Context, status *models.Status, viewerID string) (bool, error) {
	switch status.Visibility {
	case models.VisibilityPublic, models.VisibilityUnlisted:
		return true, nil
	}

	if err := common.ValidateRequiredParam("viewerID", viewerID); err != nil {
		return false, nil
	}

	if status.AuthorUsername == viewerID {
		return true, nil
	}

	switch status.Visibility {
	case models.VisibilityPrivate:
		return s.canViewPrivateStatus(ctx, status, viewerID)
	case models.VisibilityDirect:
		return s.canViewDirectMessage(status, viewerID), nil
	default:
		s.logger.Warn("unknown visibility level",
			zap.String("status_id", status.StatusID),
			zap.String("visibility", status.Visibility))
		return false, nil
	}
}

func (s *Service) canViewPrivateStatus(ctx context.Context, status *models.Status, viewerID string) (bool, error) {
	isFollowing, err := s.relationshipRepo.IsFollowing(ctx, viewerID, status.AuthorUsername)
	if err != nil {
		s.logger.Error("failed to check following relationship",
			zap.String("status_id", status.StatusID),
			zap.String("viewer_id", viewerID),
			zap.String("author", status.AuthorUsername),
			zap.Error(err))
		return false, ErrCheckFollowingRelationship
	}
	return isFollowing, nil
}

func (s *Service) canViewDirectMessage(status *models.Status, viewerID string) bool {
	viewerUsername, viewerActorID := s.resolveViewerActorID(viewerID)
	if viewerUsername == "" || viewerActorID == "" {
		return false
	}

	for _, mention := range status.Mentions {
		if mentionMatchesViewer(mention, viewerUsername, viewerActorID) {
			return true
		}
	}
	return stringSliceContains(status.ToRecipients, viewerActorID) ||
		stringSliceContains(status.CcRecipients, viewerActorID) ||
		stringSliceContains(status.BtoRecipients, viewerActorID) ||
		stringSliceContains(status.BccRecipients, viewerActorID)
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func mentionMatchesViewer(mention, viewerUsername, viewerActorID string) bool {
	candidate := strings.TrimSpace(mention)
	if candidate == "" {
		return false
	}

	if strings.EqualFold(candidate, viewerUsername) || strings.EqualFold(candidate, viewerActorID) {
		return true
	}

	return strings.EqualFold(extractMentionUsername(candidate), viewerUsername)
}

func extractMentionUsername(mention string) string {
	candidate := strings.TrimSpace(mention)
	if candidate == "" {
		return ""
	}

	if strings.HasPrefix(candidate, "http://") || strings.HasPrefix(candidate, "https://") {
		parsed, err := url.Parse(candidate)
		if err == nil {
			path := strings.Trim(parsed.Path, "/")
			if path != "" {
				segments := strings.Split(path, "/")
				last := strings.TrimSpace(segments[len(segments)-1])
				return strings.TrimPrefix(last, "@")
			}
		}
	}

	return strings.TrimPrefix(candidate, "@")
}

func (s *Service) resolveViewerActorID(viewerID string) (viewerUsername string, viewerActorID string) {
	cleaned := strings.TrimSpace(viewerID)
	if cleaned == "" {
		return "", ""
	}

	if strings.Contains(cleaned, "://") {
		actorID := strings.TrimRight(cleaned, "/")
		username := ""
		if parsed, err := url.Parse(actorID); err == nil {
			path := strings.Trim(parsed.Path, "/")
			if path != "" {
				segments := strings.Split(path, "/")
				username = segments[len(segments)-1]
			}
		}
		if username == "" {
			username = cleaned
		}
		return username, actorID
	}

	username := cleaned
	if strings.TrimSpace(s.domainName) == "" {
		return username, username
	}
	return username, fmt.Sprintf("https://%s/users/%s", s.domainName, username)
}

// ListNotes retrieves notes based on various timeline types and filters
func (s *Service) ListNotes(ctx context.Context, query *ListNotesQuery) (*Result, error) {
	s.logger.Debug("listing notes",
		zap.String("timeline_type", query.TimelineType),
		zap.String("viewer_id", query.ViewerID),
		zap.String("author_id", query.AuthorID))

	result, err := s.routeTimelineQuery(ctx, query)
	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, ErrGetTimeline
	}

	s.logger.Info("timeline query result before filtering",
		zap.String("timeline_type", query.TimelineType),
		zap.Int("items_count", len(result.Items)))

	hydratedItems := s.hydrateTimelineStatuses(ctx, result.Items)
	filteredNotes := s.filterNotesForViewer(ctx, hydratedItems, query)

	return s.buildNotesResult(filteredNotes, result), nil
}

// routeTimelineQuery routes to the appropriate timeline method based on type
func (s *Service) routeTimelineQuery(ctx context.Context, query *ListNotesQuery) (*interfaces.PaginatedResult[*models.Status], error) {
	switch query.TimelineType {
	case VisibilityPublic, "local":
		return s.noteRepo.GetPublicTimeline(ctx, query.Pagination)
	case "home":
		if err := common.ValidateRequiredParam("viewer_id", query.ViewerID); err != nil {
			return nil, ErrHomeTimelineRequiresViewerID
		}
		return s.noteRepo.GetHomeTimeline(ctx, query.ViewerID, query.Pagination)
	case "user":
		if err := common.ValidateRequiredParam("author_id", query.AuthorID); err != nil {
			return nil, ErrUserTimelineRequiresAuthorID
		}
		return s.noteRepo.GetUserTimeline(ctx, query.AuthorID, query.Pagination)
	case "conversations":
		if err := common.ValidateRequiredParam("conversation_id", query.ConversationID); err != nil {
			return nil, ErrConversationsTimelineRequiresConversationID
		}
		return s.noteRepo.GetConversationThread(ctx, query.ConversationID, query.Pagination)
	case "direct":
		if err := common.ValidateRequiredParam("viewer_id", query.ViewerID); err != nil {
			return nil, ErrDirectTimelineRequiresViewerID
		}
		// Direct messages are handled differently - we get conversations first, then statuses
		directResult, err := s.getDirectTimeline(ctx, query)
		if err != nil {
			return nil, err
		}
		return directResult.Pagination, nil
	case "hashtag":
		if err := common.ValidateRequiredParam("hashtag", query.Hashtag); err != nil {
			return nil, ErrHashtagTimelineRequiresHashtag
		}
		normalizedHashtag := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(query.Hashtag), "#"))
		result, err := s.noteRepo.GetStatusesByHashtag(ctx, normalizedHashtag, query.Pagination)
		if err != nil {
			s.logger.Error("failed to get hashtag timeline",
				zap.String("hashtag", query.Hashtag),
				zap.String("normalized_hashtag", normalizedHashtag),
				zap.Error(err))
			return nil, errors.Join(ErrGetTimeline, fmt.Errorf("hashtag timeline error: %w", err))
		}
		return result, nil
	case "list":
		if err := common.ValidateRequiredParam("list_id", query.ListID); err != nil {
			return nil, ErrListTimelineRequiresListID
		}
		listResult, err := s.getListTimeline(ctx, query)
		if err != nil {
			return nil, err
		}
		return listResult.Pagination, nil
	default:
		s.logger.Warn("unsupported timeline type",
			zap.String("timeline_type", query.TimelineType))
		return nil, ErrUnsupportedTimelineType
	}
}

// filterNotesForViewer filters notes based on privacy, visibility, and query filters
func (s *Service) filterNotesForViewer(ctx context.Context, notes []*models.Status, query *ListNotesQuery) []*models.Status {
	filteredNotes := make([]*models.Status, 0, len(notes))
	isPublicTimeline := query.TimelineType == VisibilityPublic || query.TimelineType == "local"

	for _, status := range notes {
		if !s.shouldIncludeStatus(ctx, status, query, isPublicTimeline) {
			continue
		}

		sanitized := status.SanitizeForActor(query.ViewerID)
		filteredNotes = append(filteredNotes, sanitized)
	}

	return filteredNotes
}

func (s *Service) hydrateTimelineStatuses(ctx context.Context, statuses []*models.Status) []*models.Status {
	if len(statuses) == 0 {
		return statuses
	}

	hydrated := make([]*models.Status, 0, len(statuses))
	for _, status := range statuses {
		h, err := s.ensureStatusHydrated(ctx, status)
		if err != nil {
			if s.logger != nil {
				s.logger.Error("dropping incomplete timeline status",
					zap.String("pk", safeKey(status, true)),
					zap.String("sk", safeKey(status, false)),
					zap.Error(err))
			}
			continue
		}
		hydrated = append(hydrated, h)
	}
	return hydrated
}

func (s *Service) ensureStatusHydrated(ctx context.Context, status *models.Status) (*models.Status, error) {
	if status == nil {
		return nil, errors.New("nil status reference")
	}

	if status.StatusID == "" {
		status.StatusID = deriveStatusIDFromKeys(status.PK, status.SK)
	}
	if status.StatusID == "" && status.Note != nil {
		status.StatusID = extractStatusIDFromObjectURL(status.Note.ID)
	}
	if status.StatusID == "" {
		return nil, fmt.Errorf("missing status identifier (pk=%s, sk=%s)", status.PK, status.SK)
	}

	noteMissing := status.Note == nil
	if status.ReblogOfID != "" {
		noteMissing = false
	}

	contentMissing := status.Content == "" && status.ReblogOfID == ""

	needsReload := noteMissing ||
		status.AuthorUsername == "" ||
		contentMissing ||
		status.PublishedAt.IsZero()

	if needsReload {
		if s.noteRepo == nil {
			return nil, fmt.Errorf("status repository unavailable while hydrating %s", status.StatusID)
		}
		refreshed, err := s.noteRepo.GetStatus(ctx, status.StatusID)
		if err != nil {
			return nil, fmt.Errorf("failed to reload status %s: %w", status.StatusID, err)
		}
		status = refreshed
	}

	s.ensureAuthorUsername(ctx, status)

	if status.AuthorUsername == "" {
		return nil, fmt.Errorf("status %s missing author username after hydration", status.StatusID)
	}

	return status, nil
}

func deriveStatusIDFromKeys(pk, sk string) string {
	if id := trimStatusKey(pk); id != "" {
		return id
	}
	if id := trimStatusKey(sk); id != "" {
		return id
	}
	return ""
}

func trimStatusKey(key string) string {
	if key == "" {
		return ""
	}
	if strings.HasPrefix(key, "status#") {
		return strings.TrimPrefix(key, "status#")
	}
	return ""
}

func safeKey(status *models.Status, isPK bool) string {
	if status == nil {
		return ""
	}
	if isPK {
		return status.PK
	}
	return status.SK
}

// shouldIncludeStatus determines if a status should be included based on filters
func (s *Service) shouldIncludeStatus(ctx context.Context, status *models.Status, query *ListNotesQuery, isPublicTimeline bool) bool {
	// Skip deleted posts
	if status.Deleted {
		s.logger.Debug("skipping deleted status",
			zap.String("status_id", status.StatusID))
		return false
	}

	// Direct messages must never appear outside DM-specific timelines.
	if status.Visibility == models.VisibilityDirect && query.TimelineType != "direct" && query.TimelineType != "conversations" {
		return false
	}

	// For public/local timelines, skip visibility check since repository already filtered
	// For other timelines (home, user, etc.), check visibility
	if !isPublicTimeline {
		canView, err := s.checkViewPermissions(ctx, status, query.ViewerID)
		if err != nil {
			s.logger.Error("failed to check view permissions for timeline status",
				zap.String("status_id", status.StatusID),
				zap.String("visibility", status.Visibility),
				zap.String("viewer_id", query.ViewerID),
				zap.Error(err))
			return false
		}
		if !canView {
			s.logger.Debug("skipping status not visible to viewer",
				zap.String("status_id", status.StatusID),
				zap.String("visibility", status.Visibility),
				zap.String("viewer_id", query.ViewerID))
			return false
		}
	}

	// Apply additional filters
	if query.OnlyMedia && !status.HasMedia() {
		return false
	}
	if query.ExcludeReplies && status.IsReply() {
		return false
	}
	if query.ExcludeReblogs && status.IsReblog() {
		return false
	}
	// Note: PinnedOnly would require additional data

	return true
}

// buildNotesResult constructs the final Result from filtered notes and pagination
func (s *Service) buildNotesResult(filteredNotes []*models.Status, result *interfaces.PaginatedResult[*models.Status]) *Result {
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
	}
}

// Private helper methods

func (s *Service) validateCreateCommand(ctx context.Context, cmd *CreateNoteCommand) error {
	if cmd == nil {
		return ErrNotesValidationFailed
	}

	// Use centralized business logic validation
	rules := common.ValidationRules{
		Required: []string{"author_id", "content"},
		MaxLen: map[string]int{
			"content": 5000,
		},
	}

	// Validate basic command structure
	validationResult := common.ValidateCommand(ctx, cmd, rules)
	if !validationResult.IsValid {
		s.logger.Warn("notes validation failed",
			zap.Strings("validation_errors", validationResult.Errors))
		return ErrNotesValidationFailed
	}

	// Use Mastodon business logic for content validation
	if err := s.mastodonLogic.ValidateStatusContent(cmd.Content, len(cmd.MediaIDs), len(cmd.PollOptions)); err != nil {
		return err
	}

	// Use business logic for visibility validation
	visibility := common.VisibilityLevel(cmd.Visibility)
	if err := common.ValidateBusinessVisibility(visibility, cmd.AuthorID); err != nil {
		return err
	}

	// Validate spoiler text if provided using business logic
	if cmd.SpoilerText != "" {
		rules := common.ContentValidationRules{
			MaxLength: 500, // Spoiler text limit
		}
		if err := common.ValidateBusinessContent(cmd.SpoilerText, rules); err != nil {
			return ErrSpoilerTextValidationFailed
		}
	}

	// Validate in_reply_to_id if provided
	if cmd.InReplyToID != "" {
		_, err := s.noteRepo.GetStatus(ctx, cmd.InReplyToID)
		if err != nil {
			return ErrInvalidInReplyToID
		}
	}

	return nil
}

func (s *Service) validateUpdateCommand(_ context.Context, cmd *UpdateNoteCommand) error {
	// Use centralized validation patterns from business logic
	if err := common.ValidateRequiredParam("content", strings.TrimSpace(cmd.Content)); err != nil {
		return ErrContentCannotBeEmpty
	}
	if err := common.ValidateStringLength("content", cmd.Content, 0, 5000); err != nil {
		return ErrContentTooLong
	}

	// Additional Mastodon-specific validation
	if err := s.mastodonLogic.ValidateStatusContent(cmd.Content, 0, 0); err != nil {
		return err
	}

	// Use business logic validation for spoiler text
	if cmd.SpoilerText != "" {
		if err := common.ValidateStringLength("spoiler_text", cmd.SpoilerText, 0, 160); err != nil {
			return ErrSpoilerTextValidationFailed
		}
	}

	return nil
}

func (s *Service) normalizeCreateAudience(cmd *CreateNoteCommand, author *storage.Account) {
	if cmd == nil {
		return
	}

	toRecipients := appendUniqueAudience(nil, cmd.ToRecipients...)
	ccRecipients := appendUniqueAudience(nil, cmd.CcRecipients...)
	btoRecipients := appendUniqueAudience(nil, cmd.BtoRecipients...)
	bccRecipients := appendUniqueAudience(nil, cmd.BccRecipients...)
	followersCollection := s.followersCollectionForAuthor(author)

	switch cmd.Visibility {
	case models.VisibilityPublic:
		toRecipients = appendUniqueAudience(toRecipients, activitypub.PublicAddress)
		if followersCollection != "" {
			ccRecipients = appendUniqueAudience(ccRecipients, followersCollection)
		}
	case models.VisibilityUnlisted:
		if followersCollection != "" {
			toRecipients = appendUniqueAudience(toRecipients, followersCollection)
		}
		ccRecipients = appendUniqueAudience(ccRecipients, activitypub.PublicAddress)
	case models.VisibilityPrivate:
		if followersCollection != "" {
			toRecipients = appendUniqueAudience(toRecipients, followersCollection)
		}
	case models.VisibilityDirect:
		// Direct visibility never invents recipients.
	}

	cmd.ToRecipients = toRecipients
	cmd.CcRecipients = ccRecipients
	cmd.BtoRecipients = btoRecipients
	cmd.BccRecipients = bccRecipients
}

func (s *Service) buildActivityPubNote(cmd *CreateNoteCommand, statusID string, author *storage.Account) *activitypub.Note {
	now := time.Now()

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      "Note",
			ID:        fmt.Sprintf("https://%s/users/%s/statuses/%s", s.domainName, author.User.Username, statusID),
			Published: &now,
			To:        cmd.ToRecipients,
			CC:        cmd.CcRecipients,
			BTo:       cmd.BtoRecipients,
			BCC:       cmd.BccRecipients,
			Sensitive: cmd.Sensitive,
			Summary:   cmd.SpoilerText,
		},
		Content:          cmd.Content,
		AttributedTo:     fmt.Sprintf("https://%s/users/%s", s.domainName, author.User.Username),
		Visibility:       cmd.Visibility,
		AgentAttribution: cmd.AgentAttribution,
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

// buildHashtagTags extracts hashtags from content and builds ActivityPub tags plus normalized values.
func (s *Service) buildHashtagTags(content string) ([]activitypub.Tag, []string) {
	hashtags := mastodon.ExtractHashtagsWithCase(content)
	if len(hashtags) == 0 {
		return nil, nil
	}

	sort.SliceStable(hashtags, func(i, j int) bool {
		return strings.ToLower(hashtags[i]) < strings.ToLower(hashtags[j])
	})

	tags := make([]activitypub.Tag, 0, len(hashtags))
	normalized := make([]string, 0, len(hashtags))

	for _, tag := range hashtags {
		normalizedTag := mastodon.NormalizeHashtag(tag)
		if normalizedTag == "" {
			continue
		}

		tagURL := fmt.Sprintf("https://%s/tags/%s", s.domainName, normalizedTag)
		tags = append(tags, activitypub.Tag{
			Type: "Hashtag",
			Name: "#" + tag,
			Href: tagURL,
		})
		normalized = append(normalized, normalizedTag)
	}

	if len(tags) == 0 {
		return nil, nil
	}

	return tags, normalized
}

func (s *Service) buildMentionTags(ctx context.Context, content string, author *storage.Account) ([]activitypub.Tag, []string) {
	if s.accountRepo == nil {
		return nil, nil
	}

	extracted := extractMentionHandles(content)
	if len(extracted) == 0 {
		return nil, nil
	}

	localDomain := strings.TrimSpace(s.domainName)
	authorUsername := mentionAuthorUsername(author)
	remoteResolver := s.mentionActorResolver()
	seenActors := make(map[string]struct{}, len(extracted))
	tags := make([]activitypub.Tag, 0, len(extracted))
	usernames := make([]string, 0, len(extracted))

	for _, rawMention := range extracted {
		tag, username, ok := s.resolveMentionTag(ctx, strings.TrimSpace(rawMention), localDomain, authorUsername, remoteResolver, seenActors)
		if !ok {
			continue
		}

		tags = append(tags, tag)
		if username != "" {
			usernames = append(usernames, username)
		}
	}

	if len(tags) == 0 {
		return nil, nil
	}

	return tags, usernames
}

type mentionActorResolver interface {
	ResolveActor(ctx context.Context, handle string) (*activitypub.Actor, error)
}

func mentionAuthorUsername(author *storage.Account) string {
	if author == nil || author.User == nil {
		return ""
	}
	return strings.TrimSpace(author.User.Username)
}

func (s *Service) mentionActorResolver() mentionActorResolver {
	if s.federation == nil {
		return nil
	}

	resolver, _ := s.federation.(mentionActorResolver)
	return resolver
}

func (s *Service) resolveMentionTag(
	ctx context.Context,
	mention, localDomain, authorUsername string,
	remoteResolver mentionActorResolver,
	seenActors map[string]struct{},
) (activitypub.Tag, string, bool) {
	if mention == "" {
		return activitypub.Tag{}, "", false
	}

	username, domain := splitMentionHandle(mention)

	tag, localUsername, ok, err := s.resolveStoredMentionTag(ctx, mention, username, domain, localDomain, authorUsername, seenActors)
	if err == nil {
		return tag, localUsername, ok
	}

	return s.resolveRemoteMentionTag(ctx, mention, username, domain, localDomain, remoteResolver, seenActors, err)
}

func (s *Service) resolveStoredMentionTag(
	ctx context.Context,
	mention, username, domain, localDomain, authorUsername string,
	seenActors map[string]struct{},
) (activitypub.Tag, string, bool, error) {
	lookupID := mentionLookupID(mention, username, domain, localDomain)
	account, err := s.accountRepo.GetAccount(ctx, lookupID)
	if err != nil || account == nil || account.User == nil {
		return activitypub.Tag{}, "", false, err
	}

	username, domain = normalizeMentionAccount(account, username, domain, localDomain)
	actorID, ok := s.mentionActorID(mention, username, localDomain, account)
	if !ok || !markSeenMentionActor(seenActors, actorID) {
		return activitypub.Tag{}, "", false, nil
	}

	localUsername := ""
	if (domain == "" || strings.EqualFold(domain, localDomain)) && !strings.EqualFold(username, authorUsername) {
		localUsername = username
	}

	return newMentionTag(actorID, username, domain, localDomain), localUsername, true, nil
}

func (s *Service) resolveRemoteMentionTag(
	ctx context.Context,
	mention, username, domain, localDomain string,
	remoteResolver mentionActorResolver,
	seenActors map[string]struct{},
	lookupErr error,
) (activitypub.Tag, string, bool) {
	if domain == "" || remoteResolver == nil {
		s.logger.Debug("skipping unresolved local mention",
			zap.String("mention", mention),
			zap.Error(lookupErr))
		return activitypub.Tag{}, "", false
	}

	actor, err := remoteResolver.ResolveActor(ctx, mention)
	if err != nil || actor == nil {
		s.logger.Debug("skipping unresolved remote mention",
			zap.String("mention", mention),
			zap.Error(err))
		return activitypub.Tag{}, "", false
	}

	actorID := strings.TrimSpace(actor.ID)
	if actorID == "" {
		s.logger.Debug("skipping remote mention without actor id",
			zap.String("mention", mention))
		return activitypub.Tag{}, "", false
	}

	if !markSeenMentionActor(seenActors, actorID) {
		return activitypub.Tag{}, "", false
	}

	return newMentionTag(actorID, username, domain, localDomain), "", true
}

func mentionLookupID(mention, username, domain, localDomain string) string {
	if domain != "" && strings.EqualFold(domain, localDomain) {
		return username
	}
	return mention
}

func (s *Service) mentionActorID(mention, username, localDomain string, account *storage.Account) (string, bool) {
	actorID := ""
	if account != nil && account.Actor != nil {
		actorID = strings.TrimSpace(account.Actor.ID)
	}
	if actorID != "" {
		return actorID, true
	}
	if localDomain == "" {
		s.logger.Debug("skipping mention without actor id or domain",
			zap.String("mention", mention),
			zap.String("username", username))
		return "", false
	}
	return fmt.Sprintf("https://%s/users/%s", localDomain, username), true
}

func markSeenMentionActor(seenActors map[string]struct{}, actorID string) bool {
	key := strings.ToLower(strings.TrimSpace(actorID))
	if key == "" {
		return false
	}
	if _, ok := seenActors[key]; ok {
		return false
	}
	seenActors[key] = struct{}{}
	return true
}

func newMentionTag(actorID, username, domain, localDomain string) activitypub.Tag {
	return activitypub.Tag{
		Type: "Mention",
		Href: actorID,
		Name: formatMentionTagName(username, domain, localDomain),
	}
}

func extractMentionHandles(content string) []string {
	return conversations.ExtractMentionHandles(content)
}

func splitMentionHandle(mention string) (string, string) {
	username, domain, found := strings.Cut(strings.TrimSpace(mention), "@")
	if !found {
		return username, ""
	}
	return username, domain
}

func normalizeMentionAccount(account *storage.Account, username, domain, localDomain string) (string, string) {
	resolvedUsername := strings.TrimSpace(username)
	resolvedDomain := strings.TrimSpace(domain)
	normalizedLocalDomain := normalizeMentionDomain(localDomain)

	if account == nil {
		return resolvedUsername, resolvedDomain
	}

	if account.User != nil {
		storedUsername := strings.TrimSpace(account.User.Username)
		if storedUsername != "" {
			if parsedUsername, parsedDomain := splitMentionHandle(storedUsername); parsedDomain != "" {
				if resolvedUsername == "" || strings.EqualFold(resolvedUsername, storedUsername) {
					resolvedUsername = parsedUsername
				}
				if resolvedDomain == "" {
					resolvedDomain = parsedDomain
				}
			} else if resolvedUsername == "" {
				resolvedUsername = storedUsername
			}
		}
	}

	if account.Actor != nil {
		if preferred := strings.TrimSpace(account.Actor.PreferredUsername); preferred != "" {
			resolvedUsername = preferred
		}
		if actorDomain := mentionActorDomain(account.Actor.ID); actorDomain != "" && actorDomain != normalizedLocalDomain {
			resolvedDomain = actorDomain
		}
	}

	if normalizeMentionDomain(resolvedDomain) == normalizedLocalDomain {
		resolvedDomain = ""
	}

	return strings.TrimSpace(resolvedUsername), strings.TrimSpace(resolvedDomain)
}

func mentionActorDomain(actorID string) string {
	if strings.TrimSpace(actorID) == "" {
		return ""
	}

	parsed, err := url.Parse(actorID)
	if err != nil {
		return ""
	}

	return normalizeMentionDomain(parsed.Host)
}

func normalizeMentionDomain(domain string) string {
	normalized := strings.ToLower(strings.TrimSpace(domain))
	normalized = strings.TrimPrefix(normalized, "https://")
	normalized = strings.TrimPrefix(normalized, "http://")
	normalized = strings.TrimSuffix(normalized, "/")
	return normalized
}

func formatMentionTagName(username, domain, localDomain string) string {
	username = strings.TrimSpace(username)
	domain = strings.TrimSpace(domain)
	if username == "" {
		return ""
	}
	if domain == "" || strings.EqualFold(domain, strings.TrimSpace(localDomain)) {
		return "@" + username
	}
	return "@" + username + "@" + domain
}

func (s *Service) addMentionAudience(note *activitypub.Note, mentionTags []activitypub.Tag) {
	if note == nil || len(mentionTags) == 0 {
		return
	}

	recipients := mentionActorIDs(mentionTags)
	if len(recipients) == 0 {
		return
	}

	switch note.Visibility {
	case models.VisibilityPublic, models.VisibilityUnlisted:
		note.CC = appendUniqueAudience(note.CC, recipients...)
	case models.VisibilityDirect:
		note.To = appendUniqueAudience(note.To, recipients...)
	}
}

func (s *Service) followersCollectionForAuthor(author *storage.Account) string {
	if author == nil {
		return ""
	}

	if author.Actor != nil {
		if followers := strings.TrimSpace(author.Actor.Followers); followers != "" {
			return followers
		}

		if actorID := strings.TrimSpace(author.Actor.ID); actorID != "" {
			return strings.TrimRight(actorID, "/") + "/followers"
		}
	}

	if author.User == nil {
		return ""
	}

	username := strings.TrimSpace(author.User.Username)
	if username == "" || strings.TrimSpace(s.domainName) == "" {
		return ""
	}

	return fmt.Sprintf("https://%s/users/%s/followers", s.domainName, username)
}

func mentionActorIDs(tags []activitypub.Tag) []string {
	recipients := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag.Type != "Mention" {
			continue
		}
		href := strings.TrimSpace(tag.Href)
		if href == "" {
			continue
		}
		recipients = appendUniqueAudience(recipients, href)
	}
	return recipients
}

func appendUniqueAudience(existing []string, values ...string) []string {
	for _, value := range values {
		candidate := strings.TrimSpace(value)
		if candidate == "" {
			continue
		}

		duplicate := false
		for _, current := range existing {
			if strings.EqualFold(strings.TrimSpace(current), candidate) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}

		existing = append(existing, candidate)
	}

	return existing
}

// prepareMediaAttachments validates the provided media IDs, converts them to ActivityPub attachments,
// and returns the attachments alongside the IDs that should be marked as used.
func (s *Service) prepareMediaAttachments(ctx context.Context, author *storage.Account, mediaIDs []string) ([]activitypub.Attachment, []string, error) {
	if err := common.ValidateSliceNotEmpty("media_ids", mediaIDs); err != nil {
		return nil, nil, nil
	}

	if err := common.ValidateSliceLength("media_ids", mediaIDs, 4); err != nil {
		s.logger.Warn("too many media attachments for status",
			zap.Int("limit", 4),
			zap.Int("requested", len(mediaIDs)))
		return nil, nil, errors.Join(svcErrors.ErrValidationFailed, err)
	}

	if s.mediaRepo == nil {
		s.logger.Error("media repository unavailable for attachments")
		return nil, nil, svcErrors.ErrRetrieveMediaAttachment
	}

	attachments := make([]activitypub.Attachment, 0, len(mediaIDs))
	markIDs := make([]string, 0, len(mediaIDs))

	for idx, mediaID := range mediaIDs {
		media, err := s.mediaRepo.GetMedia(ctx, mediaID)
		if err != nil {
			s.logger.Error("failed to get media attachment",
				zap.String("media_id", mediaID),
				zap.Error(err))
			return nil, nil, errors.Join(svcErrors.ErrMediaAttachmentNotFound, err)
		}

		if !s.mediaBelongsToAuthor(media, author) {
			s.logger.Warn("media attachment does not belong to author",
				zap.String("media_id", mediaID),
				zap.String("media_owner", media.UserID))
			return nil, nil, svcErrors.ErrMediaAttachmentNotFound
		}

		if media.Status != "ready" && media.Status != "completed" {
			s.logger.Warn("media attachment not ready",
				zap.String("media_id", mediaID),
				zap.String("status", media.Status))
			return nil, nil, svcErrors.ErrMediaAttachmentNotReady
		}

		if media.ExpiresAt > 0 && time.Now().Unix() > media.ExpiresAt {
			s.logger.Warn("media attachment expired",
				zap.String("media_id", mediaID),
				zap.Int64("expires_at", media.ExpiresAt))
			return nil, nil, svcErrors.ErrMediaAttachmentExpired
		}

		attachments = append(attachments, s.mapMediaToAttachment(media))
		markIDs = append(markIDs, mediaID)

		s.logger.Debug("prepared media attachment for note",
			zap.Int("index", idx),
			zap.String("media_id", mediaID),
			zap.String("content_type", media.ContentType))
	}

	return attachments, markIDs, nil
}

// mediaBelongsToAuthor checks whether the media item is owned by the author creating the note.
func (s *Service) mediaBelongsToAuthor(media *models.Media, author *storage.Account) bool {
	if media == nil || author == nil || author.User == nil {
		return false
	}

	owner := strings.ToLower(strings.TrimSpace(media.UserID))
	username := strings.ToLower(strings.TrimSpace(author.User.Username))
	if owner == "" || username == "" {
		return false
	}

	if owner == username {
		return true
	}

	if author.Actor != nil {
		actorID := strings.ToLower(strings.TrimSpace(author.Actor.ID))
		if actorID != "" && owner == actorID {
			return true
		}
	}

	return false
}

// mapMediaToAttachment converts a media record into an ActivityPub attachment.
func (s *Service) mapMediaToAttachment(media *models.Media) activitypub.Attachment {
	if media == nil {
		return activitypub.Attachment{}
	}

	url := strings.TrimSpace(media.CDNUrl)
	if url == "" {
		url = fmt.Sprintf("https://%s/media/%s", s.domainName, media.MediaID)
	}

	attachment := activitypub.Attachment{
		Type:      mapMediaCategoryToAttachmentType(media.MediaCategory),
		MediaType: media.ContentType,
		URL:       url,
		Width:     media.Width,
		Height:    media.Height,
	}

	if media.Description != "" {
		attachment.Name = media.Description
	} else if media.FileName != "" {
		attachment.Name = media.FileName
	}

	if media.FileName != "" {
		attachment.Value = media.FileName
	}

	return attachment
}

func mapMediaCategoryToAttachmentType(category models.MediaCategory) string {
	switch category {
	case models.MediaCategoryImage, models.MediaCategoryGifv:
		return "Image"
	case models.MediaCategoryVideo:
		return "Video"
	case models.MediaCategoryAudio:
		return "Audio"
	default:
		return "Document"
	}
}

func extractStatusIDFromObjectURL(objectURL string) string {
	return models.CanonicalStatusID(objectURL)
}

func boostStatusIDFromAnnounceID(announceID string) string {
	if announceID == "" {
		return ""
	}

	hash := sha256.Sum256([]byte(announceID))
	return "boost_" + hex.EncodeToString(hash[:])
}

func boostStatusIDFromActors(boosterID, targetStatusID string) string {
	if boosterID == "" || targetStatusID == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s", boosterID, targetStatusID)))
	return "boost_" + hex.EncodeToString(hash[:])
}

func deriveBoostStatusID(original *models.Status, booster *storage.Account, announce *storage.Announce) string {
	if announce != nil {
		if id := boostStatusIDFromAnnounceID(announce.ID); id != "" {
			return id
		}
	}

	boosterID := ""
	if booster != nil && booster.Actor != nil {
		boosterID = booster.Actor.ID
	}
	if id := boostStatusIDFromActors(boosterID, safeStatusID(original)); id != "" {
		return id
	}

	return uuid.New().String()
}

func cloneRecipients(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	cloned := make([]string, len(input))
	copy(cloned, input)
	return cloned
}

func (s *Service) buildBoostStatus(original *models.Status, booster *storage.Account, announce *storage.Announce) *models.Status {
	if original == nil || booster == nil || booster.Actor == nil || booster.User == nil || announce == nil {
		return nil
	}

	statusID := deriveBoostStatusID(original, booster, announce)

	publishedAt := announce.Published
	if publishedAt.IsZero() {
		publishedAt = time.Now()
	}

	boost := &models.Status{
		StatusID:        statusID,
		AuthorID:        booster.Actor.ID,
		AuthorUsername:  booster.User.Username,
		Visibility:      original.Visibility,
		Sensitive:       original.Sensitive,
		Language:        original.Language,
		ConversationID:  original.ConversationID,
		ToRecipients:    cloneRecipients(original.ToRecipients),
		CcRecipients:    cloneRecipients(original.CcRecipients),
		BtoRecipients:   cloneRecipients(original.BtoRecipients),
		BccRecipients:   cloneRecipients(original.BccRecipients),
		Mentions:        nil,
		Hashtags:        nil,
		URLs:            nil,
		ReblogOfID:      original.StatusID,
		BoostOfStatusID: original.StatusID,
		BoostOfAuthorID: original.AuthorID,
		PublishedAt:     publishedAt,
		UpdatedAt:       publishedAt,
		CreatedAt:       publishedAt,
		ModifiedAt:      publishedAt,
		BoostAnnounceID: announce.ID,
	}

	return boost
}

func (s *Service) persistBoostStatus(ctx context.Context, original *models.Status, booster *storage.Account, announce *storage.Announce) *models.Status {
	if s.noteRepo == nil {
		s.logger.Warn("status repository unavailable while building boost status",
			zap.String("original_status_id", safeStatusID(original)))
		return nil
	}

	boost := s.buildBoostStatus(original, booster, announce)
	if boost == nil {
		s.logger.Warn("unable to construct boost status payload",
			zap.String("original_status_id", safeStatusID(original)))
		return nil
	}

	if err := s.noteRepo.CreateBoostStatus(ctx, boost); err != nil {
		if isAlreadyExistsError(err) {
			s.logger.Debug("boost status already exists, treating as idempotent",
				zap.String("status_id", boost.StatusID),
				zap.String("original_status_id", safeStatusID(original)))
			return nil
		}

		s.logger.Error("failed to persist boost status",
			zap.String("status_id", boost.StatusID),
			zap.String("original_status_id", safeStatusID(original)),
			zap.Error(err))
		return nil
	}

	return boost
}

func safeStatusID(status *models.Status) string {
	if status == nil {
		return ""
	}
	return status.StatusID
}

func (s *Service) deleteBoostStatus(ctx context.Context, boosterID, targetStatusID string) {
	if s.noteRepo == nil {
		return
	}

	if err := common.ValidateRequiredParam("boost_booster_id", boosterID); err != nil {
		s.logger.Debug("booster identifier missing; skipping status deletion",
			zap.Error(err))
		return
	}

	if err := common.ValidateRequiredParam("boost_target_status_id", targetStatusID); err != nil {
		s.logger.Debug("boost target missing; skipping status deletion",
			zap.Error(err))
		return
	}

	result, err := s.noteRepo.DeleteBoostStatus(ctx, boosterID, targetStatusID)
	if err != nil {
		s.logger.Error("failed to delete boost status",
			zap.String("booster_id", boosterID),
			zap.String("target_status_id", targetStatusID),
			zap.Error(err))
		return
	}

	if result == nil {
		return
	}

	now := time.Now()
	result.Deleted = true
	result.DeletedAt = &now
	s.emitStatusDeletedEvents(ctx, result)
}

func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}

	if appErr, ok := svcErrors.AsAppError(err); ok {
		if appErr.Code == svcErrors.CodeAlreadyExists {
			return true
		}
		message := strings.ToLower(appErr.Message)
		if strings.Contains(message, "already") {
			return true
		}
	}

	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "already") || strings.Contains(lower, "condition check failed")
}

func (s *Service) emitStatusCreatedEvents(ctx context.Context, status *models.Status) []*streaming.Event {
	// Use centralized business logic for event creation
	businessEvents := common.EmitEntityCreatedEvents(ctx, "status", status.StatusID, status.AuthorID, status)

	// Convert to streaming events and emit
	var streamingEvents []*streaming.Event
	for _, businessEvent := range businessEvents {
		streamingEvent := &streaming.Event{
			Type:      businessEvent.Type,
			Stream:    fmt.Sprintf("user:%s", status.AuthorUsername),
			Timestamp: businessEvent.Timestamp,
			Payload:   businessEvent.Metadata,
		}

		// Emit to user's stream
		userEvent := *streamingEvent
		userEvent.Stream = fmt.Sprintf("user:%s", status.AuthorUsername)
		if err := s.publisher.PublishToUser(ctx, status.AuthorUsername, &userEvent); err != nil {
			s.logger.Error("failed to publish to user stream", zap.Error(err))
		} else {
			streamingEvents = append(streamingEvents, &userEvent)
		}

		// Emit to public stream if public
		if status.IsPublic() {
			publicEvent := *streamingEvent
			publicEvent.Stream = "public"
			if err := s.publisher.PublishToStream(ctx, "public", &publicEvent); err != nil {
				s.logger.Error("failed to publish to public stream", zap.Error(err))
			} else {
				streamingEvents = append(streamingEvents, &publicEvent)
			}
		}

		// Emit to hashtag streams for this event
		for _, hashtag := range status.Hashtags {
			hashtagEvent := *streamingEvent
			hashtagEvent.Stream = fmt.Sprintf("hashtag:%s", hashtag)
			if err := s.publisher.PublishToStream(ctx, hashtagEvent.Stream, &hashtagEvent); err != nil {
				s.logger.Error("failed to publish to hashtag stream",
					zap.String("hashtag", hashtag),
					zap.Error(err))
			} else {
				streamingEvents = append(streamingEvents, &hashtagEvent)
			}
		}

		// If it's a direct message, emit to conversation
		if status.IsDirect() && status.ConversationID != "" {
			conversationEvent := *streamingEvent
			conversationEvent.Stream = fmt.Sprintf("conversation:%s", status.ConversationID)
			if err := s.publisher.PublishToConversation(ctx, status.ConversationID, &conversationEvent); err != nil {
				s.logger.Error("failed to publish to conversation stream", zap.Error(err))
			} else {
				streamingEvents = append(streamingEvents, &conversationEvent)
			}
		}
	}

	return streamingEvents
}

func (s *Service) emitStatusUpdatedEvents(ctx context.Context, status *models.Status) []*streaming.Event {
	// Use centralized business logic for event creation
	businessEvents := common.EmitEntityUpdatedEvents(ctx, "status", status.StatusID, status.AuthorID, status, map[string]interface{}{
		"visibility": status.Visibility,
		"author":     status.AuthorUsername,
	})

	// Convert to streaming events and emit
	var streamingEvents []*streaming.Event
	for _, businessEvent := range businessEvents {
		streamingEvent := &streaming.Event{
			Type:      businessEvent.Type,
			Stream:    fmt.Sprintf("user:%s", status.AuthorUsername),
			Timestamp: businessEvent.Timestamp,
			Payload:   businessEvent.Metadata,
		}

		// Emit to user's stream
		if err := s.publisher.PublishToUser(ctx, status.AuthorUsername, streamingEvent); err != nil {
			s.logger.Error("failed to publish update to user stream", zap.Error(err))
		} else {
			streamingEvents = append(streamingEvents, streamingEvent)
		}
	}

	return streamingEvents
}

func (s *Service) emitStatusDeletedEvents(ctx context.Context, status *models.Status) {
	// Skip if AuthorUsername is still empty (can't emit events without it)
	if status.AuthorUsername == "" {
		s.logger.Warn("skipping status deletion events - AuthorUsername is empty",
			zap.String("status_id", status.StatusID),
			zap.String("author_id", status.AuthorID))
		return
	}

	// Use centralized business logic for event creation
	businessEvents := common.EmitEntityDeletedEvents(ctx, "status", status.StatusID, status.AuthorID)

	// Convert to streaming events and emit
	for _, businessEvent := range businessEvents {
		streamingEvent := &streaming.Event{
			Type:      businessEvent.Type,
			Stream:    fmt.Sprintf("user:%s", status.AuthorUsername),
			Timestamp: businessEvent.Timestamp,
			Payload:   businessEvent.Metadata,
		}

		// Emit to user's stream
		if err := s.publisher.PublishToUser(ctx, status.AuthorUsername, streamingEvent); err != nil {
			s.logger.Error("failed to publish deletion to user stream", zap.Error(err))
		}
	}
}

func (s *Service) queueFederationDelivery(ctx context.Context, status *models.Status, activityType string) {
	if s.federation == nil {
		s.logger.Debug("federation service not available, skipping delivery")
		return
	}

	if strings.TrimSpace(s.domainName) == "" {
		s.logger.Debug("domain name not configured, skipping delivery")
		return
	}

	// Skip if Note is nil (can happen with deleted/corrupted statuses)
	if status.Note == nil {
		s.logger.Debug("status note is nil, skipping federation delivery",
			zap.String("status_id", status.StatusID))
		return
	}

	// Create ActivityPub Create activity
	note := status.Note
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activityType,
			ID:      fmt.Sprintf("%s#%s", note.ID, strings.ToLower(activityType)),
			To:      status.ToRecipients,
			CC:      status.CcRecipients,
			BTo:     status.BtoRecipients,
			BCC:     status.BccRecipients,
		},
		Actor:  note.AttributedTo,
		Object: note, // Pass underlying *activitypub.Note for federation
	}

	defer func() {
		if r := recover(); r != nil {
			s.logger.Warn("federation queue panic suppressed during delivery",
				zap.String("status_id", status.StatusID),
				zap.String("activity_type", activityType),
				zap.Any("reason", r))
		}
	}()

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

	// Skip if Note is nil (can happen with deleted/corrupted statuses)
	if status.Note == nil {
		s.logger.Debug("status note is nil, skipping federation tombstone",
			zap.String("status_id", status.StatusID))
		return
	}

	// Ensure we have an ID for the tombstone
	tombstoneID := status.Note.ID
	if tombstoneID == "" {
		// Fallback to StatusID if Note.ID is empty
		tombstoneID = status.StatusID
		s.logger.Warn("status note ID is empty, using status ID for tombstone",
			zap.String("status_id", status.StatusID))
	}

	// Create ActivityPub Delete activity with Tombstone
	tombstone := &activitypub.BaseObject{
		Type: "Tombstone",
		ID:   tombstoneID,
	}

	// Ensure ToRecipients and CcRecipients are not nil
	toRecipients := status.ToRecipients
	if toRecipients == nil {
		toRecipients = []string{}
	}
	ccRecipients := status.CcRecipients
	if ccRecipients == nil {
		ccRecipients = []string{}
	}

	// Use Note.AttributedTo if available, otherwise fallback to AuthorID
	actorID := status.AuthorID
	if status.Note.AttributedTo != "" {
		actorID = status.Note.AttributedTo
	}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    "Delete",
			ID:      fmt.Sprintf("%s#delete", tombstoneID),
			To:      toRecipients,
			CC:      ccRecipients,
		},
		Actor:  actorID,
		Object: tombstone,
	}

	if err := s.federation.QueueActivity(ctx, activity); err != nil {
		s.logger.Error("failed to queue federation tombstone",
			zap.String("status_id", status.StatusID),
			zap.Error(err))
		// Don't return error - tombstone queuing is best-effort
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
	if err := common.ValidateRequiredParam("status_id", query.StatusID); err != nil {
		return nil, ErrStatusIDRequired
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
		Type:      streaming.StatusBoosted,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"status":    status,
			"reblogger": rebloggerUsername,
		},
	}

	s.logger.Debug("emitting status.boosted event",
		zap.String("status_id", status.StatusID),
		zap.String("reblogger", rebloggerUsername),
		zap.Int("shares_count", status.ReblogCount))

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
		Type:      streaming.StatusUnboosted,
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
		return nil, ErrStatusNotFound
	}

	if s.bookmarkRepo == nil {
		return nil, ErrBookmarkStatus
	}

	if _, err := s.bookmarkRepo.CreateBookmark(ctx, cmd.BookmarkerID, cmd.StatusID); err != nil {
		return nil, ErrBookmarkStatus
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
		return nil, ErrStatusNotFound
	}

	if s.bookmarkRepo == nil {
		return nil, ErrUnbookmarkStatus
	}

	if err := s.bookmarkRepo.DeleteBookmark(ctx, cmd.UnbookmarkerID, cmd.StatusID); err != nil {
		return nil, ErrUnbookmarkStatus
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
	if s.bookmarkRepo == nil {
		return nil, ErrGetBookmarks
	}

	limit := query.Pagination.Limit
	if limit <= 0 {
		limit = 20
	}

	bookmarks, nextCursor, err := s.bookmarkRepo.GetUserBookmarks(ctx, query.UserID, limit, query.Pagination.Cursor)
	if err != nil {
		return nil, ErrGetBookmarks
	}

	if len(bookmarks) == 0 {
		empty := &interfaces.PaginatedResult[*models.Status]{
			Items:      []*models.Status{},
			NextCursor: "",
			HasMore:    false,
			Total:      0,
		}
		return &Result{
			Notes:      empty.Items,
			Pagination: empty,
		}, nil
	}

	statusIDs := make([]string, 0, len(bookmarks))
	for _, bookmark := range bookmarks {
		if bookmark == nil {
			continue
		}
		statusIDs = append(statusIDs, bookmark.ObjectID)
	}

	statuses, err := s.noteRepo.GetStatusesByIDs(ctx, statusIDs)
	if err != nil {
		return nil, ErrGetBookmarks
	}

	statusMap := make(map[string]*models.Status, len(statuses))
	for _, status := range statuses {
		if status != nil {
			statusMap[status.StatusID] = status
		}
	}

	ordered := make([]*models.Status, 0, len(statusMap))
	for _, bookmark := range bookmarks {
		if bookmark == nil {
			continue
		}
		if status, ok := statusMap[bookmark.ObjectID]; ok {
			ordered = append(ordered, status)
		}
	}

	pagination := &interfaces.PaginatedResult[*models.Status]{
		Items:      ordered,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
		Total:      -1,
	}

	return &Result{
		Notes:      ordered,
		Pagination: pagination,
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
	Status   *models.Status
	Events   []*streaming.Event
	Announce *storage.Announce
}

// UsersResult represents a list of users (for likers, rebloggers, etc.)
type UsersResult struct {
	Users      []*storage.Account                            `json:"users"`
	Pagination *interfaces.PaginatedResult[*storage.Account] `json:"pagination"`
}

// noteActionParams contains parameters for note actions
type noteActionParams struct {
	statusID     string
	actorID      string
	actorType    string
	actionFn     func(context.Context, string, string, string) error
	emitEventsFn func(context.Context, *models.Status, string) []*streaming.Event
	errorMsg     string
}

// executeNoteActionGeneric handles the common pattern for note actions
func (s *Service) executeNoteActionGeneric(ctx context.Context, params noteActionParams) (*LikeResult, error) {
	// Get the status first to validate it exists
	note, err := s.noteRepo.GetStatus(ctx, params.statusID)
	if err != nil {
		return nil, ErrStatusNotFound
	}

	// Get actor's account
	actor, err := s.accountRepo.GetAccount(ctx, params.actorID)
	if err != nil {
		s.logger.Error("failed to get actor account",
			zap.String("actor_id", params.actorID),
			zap.String("actor_type", params.actorType),
			zap.Error(err))
		return nil, errors.Join(ErrGetAuthorAccount, err)
	}

	// Create actor and object URLs
	actorURL := fmt.Sprintf("https://%s/users/%s", s.domainName, actor.User.Username)
	objectURL := fmt.Sprintf("https://%s/users/%s/statuses/%s", s.domainName, note.AuthorUsername, params.statusID)

	// Execute the action through repository interface
	if err := params.actionFn(ctx, actorURL, objectURL, note.AuthorUsername); err != nil {
		s.logger.Error("action execution failed",
			zap.String("error_msg", params.errorMsg),
			zap.String("actor_id", params.actorID),
			zap.String("status_id", params.statusID),
			zap.Error(err))
		return nil, errors.Join(ErrExecuteAction, err)
	}

	// Refresh status so counters and derived state stay consistent
	note = s.refreshStatus(ctx, params.statusID, note)

	// Emit events
	events := params.emitEventsFn(ctx, note, params.actorID)

	return &LikeResult{
		Status: note,
		Events: events,
	}, nil
}

// LikeNote adds a like to a status
func (s *Service) LikeNote(ctx context.Context, cmd *LikeNoteCommand) (*LikeResult, error) {
	result, err := s.executeNoteActionGeneric(ctx, noteActionParams{
		statusID:     cmd.StatusID,
		actorID:      cmd.LikerID,
		actorType:    "liker",
		actionFn:     s.createLike,
		emitEventsFn: s.emitLikeEvents,
		errorMsg:     "failed to like status",
	})
	if err != nil {
		return nil, err
	}

	if s.noteRepo == nil {
		return nil, ErrStatusRepositoryUnavailable
	}

	if err := s.noteRepo.LikeStatus(ctx, cmd.LikerID, cmd.StatusID); err != nil {
		s.logger.Error("failed to increment like counter",
			zap.String("status_id", cmd.StatusID),
			zap.String("liker", cmd.LikerID),
			zap.Error(err))
		return nil, errors.Join(ErrExecuteAction, err)
	}

	return result, nil
}

// UnlikeNote removes a like from a status
func (s *Service) UnlikeNote(ctx context.Context, cmd *UnlikeNoteCommand) (*LikeResult, error) {
	result, err := s.executeNoteActionGeneric(ctx, noteActionParams{
		statusID:     cmd.StatusID,
		actorID:      cmd.UnlikerID,
		actorType:    "unliker",
		actionFn:     s.deleteLike,
		emitEventsFn: s.emitUnlikeEvents,
		errorMsg:     "failed to unlike status",
	})
	if err != nil {
		return nil, err
	}

	if s.noteRepo == nil {
		return nil, ErrStatusRepositoryUnavailable
	}

	if err := s.noteRepo.UnlikeStatus(ctx, cmd.UnlikerID, cmd.StatusID); err != nil {
		s.logger.Error("failed to decrement like counter",
			zap.String("status_id", cmd.StatusID),
			zap.String("unliker", cmd.UnlikerID),
			zap.Error(err))
		return nil, errors.Join(ErrExecuteAction, err)
	}

	return result, nil
}

// GetLikers retrieves users who liked a status
func (s *Service) GetLikers(ctx context.Context, query *GetLikersQuery) (*UsersResult, error) {
	// Get the status first to validate it exists
	_, err := s.noteRepo.GetStatus(ctx, query.StatusID)
	if err != nil {
		return nil, ErrStatusNotFound
	}

	// Get likers through repository interface
	users, pagination, err := s.getLikers(ctx, query.StatusID, query.Pagination)
	if err != nil {
		return nil, ErrGetLikers
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
		return nil, ErrStatusNotFound
	}

	if !note.CanBeReblogged() {
		s.logger.Warn("status cannot be boosted due to visibility or deletion state",
			zap.String("status_id", cmd.StatusID),
			zap.String("visibility", note.Visibility),
			zap.Bool("deleted", note.Deleted))
		return nil, ErrReblogStatus
	}

	// Get reblogger's account
	reblogger, err := s.accountRepo.GetAccount(ctx, cmd.RebloggerID)
	if err != nil {
		return nil, ErrGetRebloggerAccount
	}

	// Create actor and object URLs for the reblog
	actorURL := fmt.Sprintf("https://%s/users/%s", s.domainName, reblogger.User.Username)
	objectURL := fmt.Sprintf("https://%s/users/%s/statuses/%s", s.domainName, note.AuthorUsername, cmd.StatusID)

	// Create the reblog through repository interface
	announce, err := s.createReblog(ctx, actorURL, objectURL, cmd.RebloggerID, cmd.StatusID)
	if err != nil {
		return nil, ErrReblogStatus
	}

	// Refresh status so downstream consumers receive updated counters/state
	note = s.refreshStatus(ctx, cmd.StatusID, note)

	// Emit events
	events := s.emitReblogEvents(ctx, note, cmd.RebloggerID)
	s.queueAnnounceActivity(ctx, note, announce)
	s.notifyBoost(ctx, note, cmd.RebloggerID)

	if boostStatus := s.persistBoostStatus(ctx, note, reblogger, announce); boostStatus != nil {
		s.emitStatusCreatedEvents(ctx, boostStatus)
	}

	return &LikeResult{
		Status:   note,
		Events:   events,
		Announce: announce,
	}, nil
}

// UnreblogNote removes a reblog/announce of a status
func (s *Service) UnreblogNote(ctx context.Context, cmd *UnreblogNoteCommand) (*LikeResult, error) {
	return s.executeNoteActionGeneric(ctx, noteActionParams{
		statusID:     cmd.StatusID,
		actorID:      cmd.UnrebloggerID,
		actorType:    "unreblogger",
		actionFn:     s.deleteReblog,
		emitEventsFn: s.emitUnreblogEvents,
		errorMsg:     "failed to unreblog status",
	})
}

// GetRebloggers retrieves users who reblogged a status
func (s *Service) GetRebloggers(ctx context.Context, query *GetRebloggersQuery) (*UsersResult, error) {
	// Get the status first to validate it exists
	_, err := s.noteRepo.GetStatus(ctx, query.StatusID)
	if err != nil {
		return nil, ErrStatusNotFound
	}

	// Get rebloggers through repository interface
	users, pagination, err := s.getRebloggers(ctx, query.StatusID, query.Pagination)
	if err != nil {
		return nil, ErrGetRebloggers
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

// pinActionParams contains parameters for pin/unpin actions
type pinActionParams struct {
	statusID     string
	pinnerID     string
	actionFn     func(context.Context, string, string) error
	eventType    streaming.EventType
	actorKey     string
	timestampKey string
	errorMsg     string
}

// executePinActionGeneric handles the common pattern for pin/unpin actions
func (s *Service) executePinActionGeneric(ctx context.Context, params pinActionParams) (*LikeResult, error) {
	// Get the status first to validate it exists
	note, err := s.noteRepo.GetStatus(ctx, params.statusID)
	if err != nil {
		return nil, ErrStatusNotFound
	}

	// Ensure AuthorUsername is populated (may be empty if status was loaded from DB)
	// AuthorID is a full URL (e.g., https://dev.lesser.host/users/admin)
	// but pinnerID is just a username (e.g., admin), so we need to compare usernames
	s.ensureAuthorUsername(ctx, note)

	// Verify the user owns the status (can only pin/unpin own statuses)
	// Compare usernames, not full IDs (AuthorUsername is just the username, AuthorID is the full URL)
	if note.AuthorUsername != params.pinnerID {
		s.logger.Warn("user cannot pin/unpin status owned by another user",
			zap.String("pinner_id", params.pinnerID),
			zap.String("author_username", note.AuthorUsername),
			zap.String("author_id", note.AuthorID))
		return nil, common.ErrForbidden(ErrCannotPinPostOwnedByOther)
	}

	// Execute the action through repository interface
	if err := params.actionFn(ctx, params.pinnerID, params.statusID); err != nil {
		s.logger.Error("pin action execution failed",
			zap.String("error_msg", params.errorMsg),
			zap.String("pinner_id", params.pinnerID),
			zap.String("status_id", params.statusID),
			zap.Error(err))
		return nil, errors.Join(ErrExecuteAction, err)
	}

	// Emit events
	events := []*streaming.Event{
		{
			Type:   string(params.eventType),
			Stream: streaming.UserStreamName(params.pinnerID),
			Payload: map[string]interface{}{
				"status_id":         params.statusID,
				params.actorKey:     params.pinnerID,
				params.timestampKey: time.Now(),
			},
			Timestamp: time.Now(),
		},
	}

	return &LikeResult{
		Status: note,
		Events: events,
	}, nil
}

// PinNote pins a status to user's profile
func (s *Service) PinNote(ctx context.Context, cmd *PinNoteCommand) (*LikeResult, error) {
	return s.executePinActionGeneric(ctx, pinActionParams{
		statusID:     cmd.StatusID,
		pinnerID:     cmd.PinnerID,
		actionFn:     s.pinStatus,
		eventType:    streaming.StatusPinned,
		actorKey:     "pinner_id",
		timestampKey: "pinned_at",
		errorMsg:     "failed to pin status",
	})
}

// UnpinNote unpins a status from user's profile
func (s *Service) UnpinNote(ctx context.Context, cmd *UnpinNoteCommand) (*LikeResult, error) {
	return s.executePinActionGeneric(ctx, pinActionParams{
		statusID:     cmd.StatusID,
		pinnerID:     cmd.PinnerID,
		actionFn:     s.unpinStatus,
		eventType:    streaming.StatusUnpinned,
		actorKey:     "unpinner_id",
		timestampKey: "unpinned_at",
		errorMsg:     "failed to unpin status",
	})
}

// Mute Commands

// MuteNoteCommand represents a request to mute a status
type MuteNoteCommand struct {
	StatusID        string `json:"status_id" validate:"required"`
	MuterID         string `json:"muter_id" validate:"required"`
	DurationSeconds int    `json:"duration_seconds"`
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
		return nil, ErrStatusNotFound
	}

	// Mute the status through repository interface
	if err := s.muteStatus(ctx, cmd.MuterID, cmd.StatusID, cmd.DurationSeconds); err != nil {
		return nil, ErrMuteStatus
	}

	// Emit mute events (conversation muted)
	events := []*streaming.Event{
		{
			Type:   streaming.ConversationUpdated,
			Stream: streaming.UserStreamName(cmd.MuterID),
			Payload: map[string]interface{}{
				"status_id": cmd.StatusID,
				"muter_id":  cmd.MuterID,
				"action":    "muted",
				"muted_at":  time.Now(),
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
		return nil, ErrStatusNotFound
	}

	// Unmute the status through repository interface
	if err := s.unmuteStatus(ctx, cmd.MuterID, cmd.StatusID); err != nil {
		return nil, ErrUnmuteStatus
	}

	// Emit unmute events (conversation unmuted)
	events := []*streaming.Event{
		{
			Type:   streaming.ConversationUpdated,
			Stream: streaming.UserStreamName(cmd.MuterID),
			Payload: map[string]interface{}{
				"status_id":  cmd.StatusID,
				"unmuter_id": cmd.MuterID,
				"action":     "unmuted",
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

func (s *Service) createLike(ctx context.Context, actorURL, objectURL, statusAuthorID string) error {
	_, err := s.likeRepo.CreateLike(ctx, actorURL, objectURL, statusAuthorID)
	if err != nil {
		return ErrCreateLike
	}
	return nil
}

func (s *Service) deleteLike(ctx context.Context, actorURL, objectURL, _ string) error {
	err := s.likeRepo.DeleteLike(ctx, actorURL, objectURL)
	if err != nil {
		return ErrDeleteLike
	}
	return nil
}

func (s *Service) getLikers(ctx context.Context, statusID string, pagination interfaces.PaginationOptions) ([]*storage.Account, *interfaces.PaginatedResult[*storage.Account], error) {
	// Get likes for the status
	likes, nextCursor, err := s.likeRepo.GetObjectLikes(ctx, statusID, pagination.Limit, pagination.Cursor)
	if err != nil {
		return nil, nil, ErrGetLikes
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

func (s *Service) createReblog(ctx context.Context, actorURL, objectURL, _, _ string) (*storage.Announce, error) {
	// Idempotency: if an announce already exists for this actor/object, skip creating duplicates
	if existing, err := s.socialRepo.GetAnnounce(ctx, actorURL, objectURL); err == nil {
		s.logger.Debug("announce already persisted, skipping duplicate reblog",
			zap.String("actor_url", actorURL),
			zap.String("object_url", objectURL),
			zap.String("announce_id", existing.ID))
		return existing, nil
	} else if appErr, ok := svcErrors.AsAppError(err); ok && appErr.Code != svcErrors.CodeNotFound {
		s.logger.Error("failed to check existing announce",
			zap.String("actor_url", actorURL),
			zap.String("object_url", objectURL),
			zap.String("error_code", string(appErr.Code)),
			zap.String("error_message", appErr.Message),
			zap.Bool("is_app_error", ok),
			zap.Error(err))
		return nil, ErrCreateReblog
	} else if err != nil {
		// Log if error is not an AppError at all
		s.logger.Warn("announce check returned non-AppError",
			zap.String("actor_url", actorURL),
			zap.String("object_url", objectURL),
			zap.String("error_type", fmt.Sprintf("%T", err)),
			zap.Error(err))
	}

	announce := &storage.Announce{
		Actor:     actorURL,
		Object:    objectURL,
		CreatedAt: time.Now(),
	}

	if err := s.socialRepo.CreateAnnounce(ctx, announce); err != nil {
		if appErr, ok := svcErrors.AsAppError(err); ok {
			s.logger.Debug("create announce app error details",
				zap.String("code", string(appErr.Code)),
				zap.String("message", appErr.Message),
				zap.Any("metadata", appErr.Metadata))
		}

		if isAlreadyExistsError(err) {
			s.logger.Debug("announce already exists, treating as idempotent success",
				zap.String("actor_url", actorURL),
				zap.String("object_url", objectURL))
			if existing, getErr := s.socialRepo.GetAnnounce(ctx, actorURL, objectURL); getErr == nil {
				announce = existing
			} else {
				s.logger.Error("failed to load existing announce after duplicate detection",
					zap.String("actor_url", actorURL),
					zap.String("object_url", objectURL),
					zap.Error(getErr))
				return nil, ErrCreateReblog
			}
		} else {
			return nil, ErrCreateReblog
		}
	}

	if s.noteRepo == nil {
		s.logger.Warn("status repository unavailable while recording reblog engagement",
			zap.String("actor_url", actorURL),
			zap.String("object_url", objectURL))
		return announce, nil
	}

	if announce == nil {
		s.logger.Error("announce metadata missing after create",
			zap.String("actor_url", actorURL),
			zap.String("object_url", objectURL))
		return nil, ErrCreateReblog
	}

	statusID := extractStatusIDFromObjectURL(objectURL)

	if err := common.ValidateRequiredParam("reblog_status_id", statusID); err != nil {
		s.logger.Error("failed to resolve status id from object url",
			zap.String("object_url", objectURL),
			zap.Error(err))
		return nil, ErrReblogStatus
	}

	engagementCreated := true
	if err := s.noteRepo.ReblogStatus(ctx, actorURL, statusID, announce.ID); err != nil {
		if isAlreadyExistsError(err) {
			s.logger.Debug("reblog engagement already recorded",
				zap.String("actor_url", actorURL),
				zap.String("status_id", statusID))
			engagementCreated = false
		} else {
			s.logger.Error("failed to record reblog engagement",
				zap.String("actor_url", actorURL),
				zap.String("status_id", statusID),
				zap.Error(err))
			return nil, ErrReblogStatus
		}
	}

	if !engagementCreated {
		// Nothing new was written; treat as idempotent success
		return announce, nil
	}

	return announce, nil
}

func (s *Service) deleteReblog(ctx context.Context, actorURL, objectURL, _ string) error {
	if err := s.socialRepo.DeleteAnnounce(ctx, actorURL, objectURL); err != nil {
		return ErrDeleteReblog
	}

	if s.noteRepo == nil {
		s.logger.Warn("status repository unavailable while removing reblog engagement",
			zap.String("actor_url", actorURL),
			zap.String("object_url", objectURL))
		return nil
	}

	statusID := extractStatusIDFromObjectURL(objectURL)

	if err := common.ValidateRequiredParam("reblog_status_id", statusID); err != nil {
		s.logger.Error("failed to resolve status id from object url for unreblog",
			zap.String("object_url", objectURL),
			zap.Error(err))
		return ErrDeleteReblog
	}

	if err := s.noteRepo.UnreblogStatus(ctx, actorURL, statusID); err != nil {
		s.logger.Error("failed to remove reblog engagement",
			zap.String("actor_url", actorURL),
			zap.String("status_id", statusID),
			zap.Error(err))
		return ErrDeleteReblog
	}

	s.deleteBoostStatus(ctx, actorURL, statusID)

	return nil
}

func (s *Service) getRebloggers(ctx context.Context, statusID string, pagination interfaces.PaginationOptions) ([]*storage.Account, *interfaces.PaginatedResult[*storage.Account], error) {
	// Get announces for the status
	announces, nextCursor, err := s.socialRepo.GetStatusAnnounces(ctx, statusID, pagination.Limit, pagination.Cursor)
	if err != nil {
		return nil, nil, ErrGetAnnounces
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

func (s *Service) refreshStatus(ctx context.Context, statusID string, fallback *models.Status) *models.Status {
	if s.noteRepo == nil {
		return fallback
	}

	updated, err := s.noteRepo.GetStatus(ctx, statusID)
	if err != nil {
		s.logger.Warn("failed to refresh status after engagement",
			zap.String("status_id", statusID),
			zap.Error(err))
		return fallback
	}

	return updated
}

func (s *Service) notifyBoost(ctx context.Context, status *models.Status, boosterUsername string) {
	if s.notifications == nil || status == nil {
		return
	}

	s.ensureAuthorUsername(ctx, status)
	recipient := strings.TrimSpace(status.AuthorUsername)
	if recipient == "" || recipient == boosterUsername {
		return
	}

	statusURL := s.buildStatusURL(status)
	title := fmt.Sprintf("%s boosted your post", boosterUsername)

	cmd := &notifications.CreateNotificationCommand{
		UserID:     recipient,
		Type:       common.NotificationTypeReblog,
		ActorID:    boosterUsername,
		ActorType:  "user",
		TargetID:   status.StatusID,
		TargetType: "status",
		Title:      title,
		Body:       title,
		GroupKey:   fmt.Sprintf("reblog:%s", status.StatusID),
		Data: map[string]interface{}{
			"status_id":  status.StatusID,
			"status_url": statusURL,
			"booster":    boosterUsername,
		},
	}

	if _, err := s.notifications.CreateNotification(ctx, cmd); err != nil {
		s.logger.Error("failed to create boost notification",
			zap.String("status_id", status.StatusID),
			zap.String("recipient", recipient),
			zap.Error(err))
	}
}

func (s *Service) notifyReply(ctx context.Context, status *models.Status) {
	if s.notifications == nil || status == nil {
		return
	}

	parentID := strings.TrimSpace(status.InReplyToID)
	if parentID == "" {
		return
	}

	parent, err := s.lookupParentStatus(ctx, parentID)
	if err != nil || parent == nil {
		return
	}

	s.ensureAuthorUsername(ctx, status)
	s.ensureAuthorUsername(ctx, parent)

	recipient := strings.TrimSpace(parent.AuthorUsername)
	replier := strings.TrimSpace(status.AuthorUsername)
	if recipient == "" || replier == "" || recipient == replier {
		return
	}

	statusURL := s.buildStatusURL(status)
	title := fmt.Sprintf("%s replied to your post", replier)

	cmd := &notifications.CreateNotificationCommand{
		UserID:     recipient,
		Type:       common.NotificationTypeReply,
		ActorID:    replier,
		ActorType:  "user",
		TargetID:   status.StatusID,
		TargetType: "status",
		Title:      title,
		Body:       title,
		GroupKey:   fmt.Sprintf("reply:%s", parentID),
		Data: map[string]interface{}{
			"parent_status_id": parentID,
			"replier":          replier,
			"status_id":        status.StatusID,
			"status_url":       statusURL,
		},
	}

	if _, err := s.notifications.CreateNotification(ctx, cmd); err != nil {
		s.logger.Error("failed to create reply notification",
			zap.String("status_id", status.StatusID),
			zap.String("parent_status_id", parentID),
			zap.String("recipient", recipient),
			zap.Error(err))
	}
}

func (s *Service) notifyMentions(ctx context.Context, status *models.Status, mentionedUsers []string) {
	if s.notifications == nil || status == nil || len(mentionedUsers) == 0 {
		return
	}

	s.ensureAuthorUsername(ctx, status)
	authorUsername := strings.TrimSpace(status.AuthorUsername)
	if authorUsername == "" {
		return
	}

	statusURL := s.buildStatusURL(status)
	title := fmt.Sprintf("%s mentioned you", authorUsername)
	seen := make(map[string]struct{}, len(mentionedUsers))

	for _, rawRecipient := range mentionedUsers {
		recipient := strings.TrimSpace(rawRecipient)
		if recipient == "" || strings.EqualFold(recipient, authorUsername) {
			continue
		}

		key := strings.ToLower(recipient)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		cmd := &notifications.CreateNotificationCommand{
			UserID:     recipient,
			Type:       common.NotificationTypeMention,
			ActorID:    authorUsername,
			ActorType:  "user",
			TargetID:   status.StatusID,
			TargetType: "status",
			Title:      title,
			Body:       title,
			GroupKey:   fmt.Sprintf("mention:%s", status.StatusID),
			Data: map[string]interface{}{
				"status_id":  status.StatusID,
				"status_url": statusURL,
				"mentioner":  authorUsername,
			},
		}

		if _, err := s.notifications.CreateNotification(ctx, cmd); err != nil {
			s.logger.Error("failed to create mention notification",
				zap.String("status_id", status.StatusID),
				zap.String("recipient", recipient),
				zap.Error(err))
		}
	}
}

func (s *Service) buildStatusURL(status *models.Status) string {
	if status == nil || status.StatusID == "" {
		return ""
	}

	domain := strings.TrimSpace(s.domainName)
	if domain == "" {
		domain = "localhost"
	}
	author := strings.TrimSpace(status.AuthorUsername)
	if author == "" && status.AuthorID != "" {
		trimmed := strings.TrimSuffix(strings.TrimPrefix(status.AuthorID, "https://"), "/")
		if strings.Contains(trimmed, "/") {
			parts := strings.Split(trimmed, "/")
			author = parts[len(parts)-1]
		} else {
			author = trimmed
		}
	}
	if author == "" {
		return ""
	}

	return fmt.Sprintf("https://%s/users/%s/statuses/%s", domain, author, status.StatusID)
}

func (s *Service) queueAnnounceActivity(ctx context.Context, status *models.Status, announce *storage.Announce) {
	if s.federation == nil || status == nil || announce == nil {
		return
	}

	published := announce.Published
	if published.IsZero() {
		published = time.Now()
	}
	publishedCopy := published

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      activitypub.AnnounceType,
			ID:        announce.ID,
			Published: &publishedCopy,
			To:        status.ToRecipients,
			CC:        status.CcRecipients,
		},
		Actor:  announce.Actor,
		Object: announce.Object,
	}

	if err := s.federation.QueueActivity(ctx, activity); err != nil {
		s.logger.Error("failed to queue announce activity",
			zap.String("status_id", status.StatusID),
			zap.String("actor", announce.Actor),
			zap.Error(err))
		return
	}

	s.logger.Debug("queued announce activity",
		zap.String("status_id", status.StatusID),
		zap.String("announce_id", announce.ID))
}

func (s *Service) pinStatus(ctx context.Context, userID, statusID string) error {
	pin := &storage.StatusPin{
		Username:  userID,
		StatusID:  statusID,
		CreatedAt: time.Now(),
	}
	err := s.socialRepo.CreateStatusPin(ctx, pin)
	if err != nil {
		return ErrPinStatus
	}
	return nil
}

func (s *Service) unpinStatus(ctx context.Context, userID, statusID string) error {
	err := s.socialRepo.DeleteStatusPin(ctx, userID, statusID)
	if err != nil {
		return ErrUnpinStatus
	}
	return nil
}

func (s *Service) muteStatus(ctx context.Context, userID, statusID string, durationSeconds int) error {
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

	if durationSeconds > 0 {
		mute.ExpiresAt = time.Now().Add(time.Duration(durationSeconds) * time.Second)
	}

	// Store the mute
	if s.conversationRepo == nil {
		s.logger.Warn("conversation repository not available",
			zap.String("user_id", userID),
			zap.String("conversation_id", conversationID))
		return ErrConversationServiceNotAvailable
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
		return ErrMuteConversation
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
		return ErrConversationServiceNotAvailable
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
		return ErrUnmuteConversation
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
	if len(allStatuses) > 1 {
		slices.SortStableFunc(allStatuses, func(a, b *models.Status) int {
			switch {
			case a == nil && b == nil:
				return 0
			case a == nil:
				return 1
			case b == nil:
				return -1
			case a.PublishedAt.After(b.PublishedAt):
				return -1
			case a.PublishedAt.Before(b.PublishedAt):
				return 1
			default:
				return 0
			}
		})
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
	if len(allStatuses) == limit && common.ValidateSliceNotEmpty("allStatuses", allStatuses) == nil {
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
		return nil, ErrConversationServiceNotAvailable
	}

	// Get user's conversations first
	opts := interfaces.PaginationOptions{
		Limit:  query.Pagination.Limit,
		Cursor: query.Pagination.Cursor,
	}
	result, err := s.conversationRepo.GetUserConversationsByFolder(ctx, query.ViewerID, models.UserConversationFolderInbox, opts)
	if err != nil {
		return nil, ErrGetConversations
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
	if err := common.ValidateRequiredParam("viewer_id", query.ViewerID); err != nil {
		return nil, ErrViewerIDRequiredForFavoritedTimeline
	}

	// Get the viewer's actor to use their actor ID for likes
	account, err := s.accountRepo.GetAccount(ctx, query.ViewerID)
	if err != nil {
		return nil, ErrGetViewerAccount
	}

	// Get liked objects using Like repository
	if s.likeRepo == nil {
		return nil, ErrLikeRepositoryNotAvailable
	}

	lookupIDs := s.viewerLikeLookupIDs(account, query.ViewerID)
	var (
		likes      []*models.Like
		nextCursor string
	)
	for idx, lookupID := range lookupIDs {
		likes, nextCursor, err = s.likeRepo.GetActorLikes(ctx, lookupID, query.Pagination.Limit, query.Pagination.Cursor)
		if err != nil {
			return nil, ErrGetLikedObjects
		}
		if len(likes) > 0 || nextCursor != "" || idx == len(lookupIDs)-1 {
			break
		}
	}

	// Collect the liked status IDs
	statusIDs := make([]string, 0, len(likes))
	for _, like := range likes {
		// Extract status ID from object ID if needed
		// Object IDs might be full URLs like https://example.com/users/test/statuses/123
		statusID := like.Object
		if strings.Contains(statusID, "/statuses/") {
			parts := strings.Split(statusID, "/statuses/")
			if common.ValidateSliceLength("parts", parts, 2) == nil {
				statusID = parts[1]
			}
		}
		statusIDs = append(statusIDs, statusID)
	}

	// Get the actual status objects
	statuses, err := s.noteRepo.GetStatusesByIDs(ctx, statusIDs)
	if err != nil {
		return nil, ErrGetStatuses
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

func (s *Service) viewerLikeLookupIDs(account *storage.Account, viewerID string) []string {
	lookupIDs := make([]string, 0, 3)

	viewerUsername, viewerActorID := s.resolveViewerActorID(viewerID)
	if account != nil {
		if account.Actor != nil && strings.TrimSpace(account.Actor.ID) != "" {
			viewerActorID = strings.TrimSpace(account.Actor.ID)
		}
		if account.User != nil && strings.TrimSpace(account.User.Username) != "" {
			viewerUsername = strings.TrimSpace(account.User.Username)
		}
	}

	lookupIDs = appendUniqueLookupID(lookupIDs, viewerActorID)
	lookupIDs = appendUniqueLookupID(lookupIDs, viewerUsername)
	if viewerUsername != "" && strings.TrimSpace(s.domainName) != "" {
		lookupIDs = appendUniqueLookupID(lookupIDs, fmt.Sprintf("https://%s/users/%s", s.domainName, viewerUsername))
	}

	return lookupIDs
}

func appendUniqueLookupID(values []string, raw string) []string {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return values
	}
	if slices.Contains(values, candidate) {
		return values
	}
	return append(values, candidate)
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
		return nil, err
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
		return nil, ErrCreateScheduledStatus
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
	// Use centralized validation patterns from business logic
	if err := common.ValidateRequiredParam("content", strings.TrimSpace(cmd.Content)); err != nil {
		return ErrContentCannotBeEmpty
	}
	if err := common.ValidateStringLength("content", cmd.Content, 0, 500); err != nil {
		return ErrContentTooLongShort
	}

	// Validate scheduled time using business logic
	if cmd.ScheduledAt.Before(time.Now()) {
		return ErrScheduledTimeInPast
	}

	// Additional Mastodon-specific validation
	if err := s.mastodonLogic.ValidateStatusContent(cmd.Content, 0, 0); err != nil {
		return err
	}

	// Use common validation for visibility
	if err := common.ValidateVisibility(cmd.Visibility); err != nil {
		return err
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
		return nil, ErrGetSearchSuggestions
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
	if cmd != nil && cmd.Note != nil {
		// Community notes are surfaced via an HTML-by-contract field in Mastodon-compatible responses.
		// Treat stored note content as plain text and escape so it is always safe to embed.
		cmd.Note.Content = htmlsafe.Escape(strings.TrimSpace(cmd.Note.Content))
	}

	// Create community note through repository
	if err := s.communityNoteRepo.CreateCommunityNote(ctx, cmd.Note); err != nil {
		return nil, ErrCreateCommunityNote
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
		return nil, ErrGetVisibleCommunityNotes
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
		return nil, ErrGetCommunityNote
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
		return nil, ErrCreateCommunityNoteVote
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
		return nil, ErrGetCommunityNotesByAuthor
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
		return 0, ErrCountStatusesByAuthor
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
		return nil, ErrGetUserTimeline
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
		return 0, ErrCountReplies
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
		return 0, ErrGetBoostCount
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
		return 0, ErrGetLikeCount
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
		return false, ErrCheckUserHasLiked
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
		return false, ErrCheckUserHasReblogged
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
		return false, ErrCheckUserHasBookmarked
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
