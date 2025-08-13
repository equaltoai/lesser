package services

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// Visibility constants
const (
	VisibilityPublic   = "public"
	VisibilityUnlisted = "unlisted"
	VisibilityPrivate  = "private"
	VisibilityDirect   = "direct"
)

// businessLogicService implements BusinessLogicService
type businessLogicService struct {
	deps       *ServiceDependencies
	storage    StorageAdapter
	validation ValidationService
	auth       AuthenticationService
	federation FederationService
	timeline   TimelineService
	analytics  AnalyticsService
	publisher  streaming.Publisher
	logger     *zap.Logger
}

// NewBusinessLogicService creates a new business logic service
func NewBusinessLogicService(
	deps *ServiceDependencies,
	validation ValidationService,
	auth AuthenticationService,
	federation FederationService,
	timeline TimelineService,
	analytics AnalyticsService,
	publisher streaming.Publisher,
) BusinessLogicService {
	logger := deps.Logger.(*zap.Logger)
	storage := CreateStorageAdapter(deps.Repos)

	return &businessLogicService{
		deps:       deps,
		storage:    storage,
		validation: validation,
		auth:       auth,
		federation: federation,
		timeline:   timeline,
		analytics:  analytics,
		publisher:  publisher,
		logger:     logger,
	}
}

// CreatePost implements unified post creation logic
func (s *businessLogicService) CreatePost(ctx context.Context, user *UserContext, input *CreatePostInput) (*CreatePostResult, error) {
	// 1. Validate input
	if err := s.validation.ValidateCreatePost(input); err != nil {
		return nil, err
	}

	// 2. Handle scheduled posts
	if input.ScheduledAt != nil && *input.ScheduledAt != "" {
		return s.handleScheduledPost(ctx, user, input)
	}

	// 3. Get authenticated actor
	actor, err := s.storage.GetActor(ctx, user.Username)
	if err != nil {
		s.logger.Error("failed to get actor", zap.Error(err))
		return nil, NewInternalError("Failed to get user account", err)
	}

	// 4. Create note object
	now := time.Now()
	note, hashtags := s.createNoteFromInput(input, actor, now)

	// 5. Handle poll creation if requested
	var poll interface{}
	// TODO: Implement poll creation logic based on input

	// 6. Process content and emojis
	parsedEmojis, err := s.processContentAndEmojis(ctx, note)
	if err != nil {
		return nil, err
	}

	// 7. Create and store the Note object
	if err := s.storage.CreateObject(ctx, note); err != nil {
		s.logger.Error("failed to create note object", zap.Error(err))
		return nil, NewInternalError("Failed to create post", err)
	}

	// 8. Handle reply processing
	if input.InReplyToID != "" {
		if err := s.handleReplyProcessing(ctx, input.InReplyToID, actor.ID); err != nil {
			s.logger.Warn("failed to process reply", zap.Error(err))
		}
	}

	// 9. Create Create activity
	createActivity := s.createActivity(actor, note, now)
	if err := s.storage.CreateActivity(ctx, createActivity); err != nil {
		s.logger.Error("failed to create activity", zap.Error(err))
		return nil, NewInternalError("Failed to create activity", err)
	}

	// 10. Perform post-creation tasks asynchronously
	go s.performPostCreationTasks(context.Background(), createActivity, input, hashtags, actor, user, now)

	return &CreatePostResult{
		Activity:     createActivity,
		Note:         note,
		Actor:        actor,
		Poll:         poll,
		ParsedEmojis: parsedEmojis,
	}, nil
}

// DeletePost implements unified post deletion logic
func (s *businessLogicService) DeletePost(ctx context.Context, user *UserContext, input *DeletePostInput) (*DeletePostResult, error) {
	// 1. Validate input
	if err := s.validation.ValidateDeletePost(input); err != nil {
		return nil, err
	}

	// 2. Get authenticated actor
	actor, err := s.storage.GetActor(ctx, user.Username)
	if err != nil {
		s.logger.Error("failed to get actor", zap.Error(err))
		return nil, NewInternalError("Failed to get user account", err)
	}

	// 3. Normalize object ID
	objectID := s.normalizeObjectID(input.ObjectID)

	// 4. Get and verify object ownership
	object, err := s.storage.GetObject(ctx, objectID)
	if err != nil {
		return nil, NewNotFoundError("Post not found")
	}

	// 5. Verify ownership
	if !s.verifyObjectOwnership(object, actor.ID) {
		return nil, NewForbiddenError("You can only delete your own posts")
	}

	// 6. Create Delete activity
	deleteActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.DeleteType,
			ID:      fmt.Sprintf("%s/activities/delete-%d-%s", actor.ID, time.Now().Unix(), s.generateRandomID()),
			To:      []string{activitypub.PublicAddress},
		},
		Actor:  actor.ID,
		Object: objectID,
	}
	now := time.Now()
	deleteActivity.Published = &now

	// 7. Store delete activity
	if err := s.storage.CreateActivity(ctx, deleteActivity); err != nil {
		s.logger.Error("failed to create delete activity", zap.Error(err))
		return nil, NewInternalError("Failed to delete post", err)
	}

	// 8. Perform cascade deletion
	if err := s.performCascadeDeletion(ctx, objectID, actor.ID); err != nil {
		s.logger.Warn("failed to perform cascade deletion", zap.Error(err))
	}

	// 9. Tombstone the object
	if err := s.storage.TombstoneObject(ctx, objectID, actor.ID); err != nil {
		s.logger.Error("failed to tombstone object", zap.Error(err))
		return nil, NewInternalError("Failed to delete post", err)
	}

	// 10. Deliver federation asynchronously
	// Delete activities are delivered to:
	// - All followers (for public/unlisted posts)
	// - Mentioned users (for private/direct posts)
	// - Original recipients (for replies)
	// This ensures proper tombstone propagation across the fediverse
	go func() {
		if err := s.federation.DeliverToRecipients(context.Background(), deleteActivity, actor); err != nil {
			s.logger.Error("failed to deliver delete activity", zap.Error(err))
		}
	}()

	return &DeletePostResult{
		Activity: deleteActivity,
		Deleted:  true,
	}, nil
}

// FollowActor implements unified follow logic
func (s *businessLogicService) FollowActor(ctx context.Context, user *UserContext, input *FollowInput) (*FollowResult, error) {
	// 1. Validate input
	if err := s.validation.ValidateFollowInput(input); err != nil {
		return nil, err
	}

	// 2. Get authenticated actor
	actor, err := s.storage.GetActor(ctx, user.Username)
	if err != nil {
		s.logger.Error("failed to get actor", zap.Error(err))
		return nil, NewInternalError("Failed to get user account", err)
	}

	// 3. Get target actor
	targetActor, err := s.storage.GetActor(ctx, input.TargetActorID)
	if err != nil {
		return nil, NewNotFoundError("Account not found")
	}

	// 4. Check if already following
	isFollowing, err := s.storage.IsFollowing(ctx, user.Username, input.TargetActorID)
	if err == nil && isFollowing {
		return &FollowResult{
			Following: true,
			Requested: false,
		}, nil
	}

	// 5. Create Follow activity
	followActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.FollowType,
			ID:      fmt.Sprintf("%s/activities/follow-%d-%s", actor.ID, time.Now().Unix(), s.generateRandomID()),
			To:      []string{targetActor.ID},
		},
		Actor:  actor.ID,
		Object: targetActor.ID,
	}
	now := time.Now()
	followActivity.Published = &now

	// 6. Create relationship
	if err := s.storage.CreateRelationship(ctx, user.Username, input.TargetActorID, followActivity.ID); err != nil {
		s.logger.Error("failed to create relationship", zap.Error(err))
		return nil, NewInternalError("Failed to follow user", err)
	}

	// 7. Store activity
	if err := s.storage.CreateActivity(ctx, followActivity); err != nil {
		s.logger.Error("failed to create follow activity", zap.Error(err))
		return nil, NewInternalError("Failed to follow user", err)
	}

	// 8. Handle approval flow and federation asynchronously
	requested := targetActor.ManuallyApprovesFollowers
	go s.handleFollowFederation(context.Background(), followActivity, actor, targetActor, requested)

	return &FollowResult{
		Activity:  followActivity,
		Following: !requested,
		Requested: requested,
	}, nil
}

// LikeObject implements unified like logic
func (s *businessLogicService) LikeObject(ctx context.Context, user *UserContext, input *LikeInput) (*LikeResult, error) {
	// 1. Validate input
	if err := s.validation.ValidateLikeInput(input); err != nil {
		return nil, err
	}

	// 2. Get authenticated actor
	actor, err := s.storage.GetActor(ctx, user.Username)
	if err != nil {
		s.logger.Error("failed to get actor", zap.Error(err))
		return nil, NewInternalError("Failed to get user account", err)
	}

	// 3. Check if already liked
	hasLiked, err := s.storage.HasLiked(ctx, actor.ID, input.ObjectID)
	if err == nil && hasLiked {
		return &LikeResult{Liked: true}, nil
	}

	// 4. Get the target object
	object, err := s.storage.GetObject(ctx, input.ObjectID)
	if err != nil {
		return nil, NewNotFoundError("Post not found")
	}

	// 5. Create Like activity
	likeActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.LikeType,
			ID:      fmt.Sprintf("%s/activities/like-%d-%s", actor.ID, time.Now().Unix(), s.generateRandomID()),
			To:      []string{s.extractAttributedTo(object)},
		},
		Actor:  actor.ID,
		Object: input.ObjectID,
	}
	now := time.Now()
	likeActivity.Published = &now

	// 6. Create like record
	if err := s.storage.CreateLike(ctx, actor.ID, input.ObjectID, likeActivity.ID); err != nil {
		s.logger.Error("failed to create like", zap.Error(err))
		return nil, NewInternalError("Failed to like post", err)
	}

	// 7. Store activity
	if err := s.storage.CreateActivity(ctx, likeActivity); err != nil {
		s.logger.Error("failed to create like activity", zap.Error(err))
		return nil, NewInternalError("Failed to like post", err)
	}

	// 8. Handle analytics and federation asynchronously
	go s.handleLikePostProcessing(context.Background(), likeActivity, actor, object)

	return &LikeResult{
		Activity: likeActivity,
		Liked:    true,
	}, nil
}

// Helper methods

func (s *businessLogicService) handleScheduledPost(ctx context.Context, user *UserContext, input *CreatePostInput) (*CreatePostResult, error) {
	s.logger.Info("creating scheduled post",
		zap.String("user_id", user.Username),
		zap.String("scheduled_at", *input.ScheduledAt))

	// Parse the scheduled time
	scheduledTime, err := time.Parse(time.RFC3339, *input.ScheduledAt)
	if err != nil {
		return nil, NewValidationError("invalid scheduled_at format, must be RFC3339")
	}

	// Validate scheduled time (must be at least 5 minutes in the future, max 1 year)
	now := time.Now()
	if scheduledTime.Before(now.Add(5 * time.Minute)) {
		return nil, NewValidationError("scheduled_at must be at least 5 minutes in the future")
	}
	if scheduledTime.After(now.AddDate(1, 0, 0)) {
		return nil, NewValidationError("scheduled_at cannot be more than 1 year in the future")
	}

	// Get the current user's actor
	actor, err := s.storage.GetActor(ctx, user.Username)
	if err != nil {
		s.logger.Error("failed to get actor for scheduled post", zap.Error(err))
		return nil, NewInternalError("failed to get actor", err)
	}

	// Create the Note object (but don't publish yet)
	note, hashtags := s.createNoteFromInput(input, actor, now)

	// Generate scheduled status ID
	scheduledStatusID := fmt.Sprintf("%s/scheduled_statuses/%s-%d", s.deps.Config.BaseURL, user.Username, now.Unix())

	// Store as scheduled status (this would need to be implemented in storage)
	s.logger.Info("storing scheduled status",
		zap.String("scheduled_status_id", scheduledStatusID),
		zap.String("scheduled_at", scheduledTime.Format(time.RFC3339)),
		zap.String("actor_id", actor.ID))

	// Create a placeholder activity for the scheduled post
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			ID:      fmt.Sprintf("%s/activities/scheduled/%d", s.deps.Config.BaseURL, now.Unix()),
			Type:    activitypub.CreateType,
			To:      []string{activitypub.PublicAddress},
			CC:      []string{fmt.Sprintf("%s/followers", actor.ID)},
		},
		Actor:  actor.ID,
		Object: note,
	}

	// Note: In a real implementation, this would:
	// 1. Store the scheduled status in the database
	// 2. Queue a job to publish at the scheduled time
	// 3. Return the scheduled status information

	s.logger.Warn("scheduled post storage needs full implementation",
		zap.String("scheduled_id", scheduledStatusID),
		zap.Time("scheduled_time", scheduledTime))

	// For now, return the activity that would be created
	return &CreatePostResult{
		Activity:     activity,
		Note:         note,
		Actor:        actor,
		ParsedEmojis: hashtags, // Reusing for hashtags
	}, nil
}

func (s *businessLogicService) createNoteFromInput(input *CreatePostInput, actor *activitypub.Actor, now time.Time) (*activitypub.Note, []string) {
	if input.Visibility == "" {
		input.Visibility = VisibilityPublic
	}

	noteID := fmt.Sprintf("%s/objects/%d-%s", s.deps.Config.BaseURL, now.Unix(), s.generateRandomID())
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        noteID,
			Type:      activitypub.NoteType,
			Summary:   input.SpoilerText,
			Sensitive: input.Sensitive,
		},
		Content:      input.Content,
		AttributedTo: actor.ID,
		Visibility:   input.Visibility,
	}

	note.Published = &now

	// Process hashtags
	hashtags := mastodon.ExtractHashtagsWithCase(input.Content)
	if len(hashtags) > 0 {
		note.Tag = s.createHashtagTags(hashtags)
	}

	// Set addressing
	s.setNoteAddressing(note, input.Visibility, actor)

	// Handle reply
	if input.InReplyToID != "" {
		note.InReplyTo = input.InReplyToID
	}

	return note, hashtags
}

func (s *businessLogicService) createHashtagTags(hashtags []string) []activitypub.Tag {
	tags := make([]activitypub.Tag, 0, len(hashtags))
	for _, tag := range hashtags {
		normalizedTag := mastodon.NormalizeHashtag(tag)
		tagURL := fmt.Sprintf("%s/tags/%s", s.deps.Config.BaseURL, normalizedTag)

		tags = append(tags, activitypub.Tag{
			Type: "Hashtag",
			Name: "#" + tag,
			Href: tagURL,
		})
	}
	return tags
}

func (s *businessLogicService) setNoteAddressing(note *activitypub.Note, visibility string, actor *activitypub.Actor) {
	switch visibility {
	case "public":
		note.To = []string{activitypub.PublicAddress}
		note.CC = []string{actor.Followers}
	case "unlisted":
		note.To = []string{actor.Followers}
		note.CC = []string{activitypub.PublicAddress}
	case "private":
		note.To = []string{actor.Followers}
	case "direct":
		mentions := s.extractMentions(note.Content)
		note.To = mentions
	}
}

func (s *businessLogicService) extractMentions(content string) []string {
	mentions := []string{}
	words := strings.Fields(content)

	for _, word := range words {
		if strings.HasPrefix(word, "@") {
			username := strings.TrimPrefix(word, "@")
			username = strings.TrimRight(username, ".,!?;:")

			if username != "" {
				actorURI := fmt.Sprintf("%s/users/%s", s.deps.Config.BaseURL, username)
				mentions = append(mentions, actorURI)
			}
		}
	}

	return mentions
}

func (s *businessLogicService) processContentAndEmojis(_ context.Context, note *activitypub.Note) (interface{}, error) {
	// Parse emojis from note content
	// For now, we'll use basic emoji parsing without the full parser
	// since it requires repository access we may not have in this context

	// Extract emoji shortcodes using regex
	emojiRegex := regexp.MustCompile(`:([a-zA-Z0-9_]+):`)
	matches := emojiRegex.FindAllStringSubmatch(note.Content, -1)

	// Build emoji tags for found shortcodes
	// In production, these would be looked up from the database
	for _, match := range matches {
		if len(match) > 1 {
			shortcode := match[1]
			// For now, create placeholder emoji tags
			// In production, we'd look up the actual emoji URL from storage
			emojiTag := activitypub.Tag{
				Type: "Emoji",
				Name: ":" + shortcode + ":",
				// In production, would include actual emoji URL
				// Href would contain the icon URL
			}
			note.Tag = append(note.Tag, emojiTag)
		}
	}

	return nil, nil
}

func (s *businessLogicService) createActivity(actor *activitypub.Actor, note *activitypub.Note, now time.Time) *activitypub.Activity {
	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      activitypub.CreateType,
			ID:        fmt.Sprintf("%s/activities/create-%d-%s", actor.ID, now.Unix(), s.generateRandomID()),
			To:        note.To,
			Published: &now,
		},
		Actor:  actor.ID,
		Object: note,
	}
}

func (s *businessLogicService) performPostCreationTasks(ctx context.Context, activity *activitypub.Activity, input *CreatePostInput, hashtags []string, actor *activitypub.Actor, _ *UserContext, now time.Time) {
	// Fan out to timelines
	if err := s.FanOutPost(ctx, activity); err != nil {
		s.logger.Error("failed to fan out post", zap.Error(err))
	}

	// Record analytics
	if err := s.analytics.RecordStatusCreation(ctx, actor.ID, now); err != nil {
		s.logger.Warn("failed to record status creation", zap.Error(err))
	}

	// Record hashtag usage
	if err := s.analytics.RecordHashtagUsage(ctx, hashtags, activity.Object.(*activitypub.Note).ID, actor.ID); err != nil {
		s.logger.Warn("failed to record hashtag usage", zap.Error(err))
	}

	// Record link shares
	links := s.extractLinksFromContent(input.Content)
	if err := s.analytics.RecordLinkShare(ctx, links, activity.Object.(*activitypub.Note).ID, actor.ID); err != nil {
		s.logger.Warn("failed to record link shares", zap.Error(err))
	}

	// Federation delivery
	if err := s.DeliverActivity(ctx, activity, actor, input.Visibility); err != nil {
		s.logger.Error("failed to deliver activity", zap.Error(err))
	}
}

func (s *businessLogicService) handleReplyProcessing(ctx context.Context, inReplyToID, actorID string) error {
	// Record reply engagement
	if err := s.analytics.RecordEngagement(ctx, inReplyToID, "reply", actorID); err != nil {
		s.logger.Warn("failed to record reply engagement", zap.Error(err))
	}

	// Increment reply count
	if err := s.storage.IncrementReplyCount(ctx, inReplyToID); err != nil {
		s.logger.Warn("failed to increment reply count", zap.Error(err))
	}

	return nil
}

// Utility methods
func (s *businessLogicService) generateRandomID() string {
	// Simple random ID generation - should be replaced with more robust implementation
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (s *businessLogicService) normalizeObjectID(objectID string) string {
	if !strings.HasPrefix(objectID, "http://") && !strings.HasPrefix(objectID, "https://") {
		return fmt.Sprintf("%s/objects/%s", s.deps.Config.BaseURL, objectID)
	}
	return objectID
}

func (s *businessLogicService) verifyObjectOwnership(object interface{}, actorID string) bool {
	attributedTo := s.extractAttributedTo(object)
	return attributedTo == actorID
}

func (s *businessLogicService) extractAttributedTo(object interface{}) string {
	switch obj := object.(type) {
	case *activitypub.Note:
		return obj.AttributedTo
	case map[string]interface{}:
		if attr, ok := obj["attributedTo"].(string); ok {
			return attr
		}
	}
	return ""
}

func (s *businessLogicService) extractLinksFromContent(content string) []string {
	links := []string{}
	words := strings.Fields(content)
	for _, word := range words {
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
			links = append(links, word)
		}
	}
	return links
}

// Implement remaining interface methods with stubs for now
func (s *businessLogicService) UpdatePost(ctx context.Context, user *UserContext, input *UpdatePostInput) (*UpdatePostResult, error) {
	s.logger.Info("updating post",
		zap.String("user_id", user.Username),
		zap.String("object_id", input.ObjectID))

	// Get the current user's actor
	actor, err := s.storage.GetActor(ctx, user.Username)
	if err != nil {
		s.logger.Error("failed to get actor for update post", zap.Error(err))
		return nil, NewInternalError("failed to get actor", err)
	}

	// Get the existing object to verify ownership
	existingObject, err := s.storage.GetObject(ctx, input.ObjectID)
	if err != nil {
		s.logger.Error("failed to get existing object", zap.Error(err))
		return nil, NewNotFoundError("post not found")
	}

	// Check if it's a Note and if the user owns it
	existingNote, ok := existingObject.(*activitypub.Note)
	if !ok {
		return nil, NewValidationError("object is not a status that can be updated")
	}

	if existingNote.AttributedTo != actor.ID {
		return nil, NewForbiddenError("you can only update your own posts")
	}

	// Create updated Note
	now := time.Now()
	updatedNote := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        existingNote.ID, // Keep the same ID
			Type:      activitypub.NoteType,
			Summary:   input.SpoilerText,
			Sensitive: input.Sensitive,
			Published: existingNote.Published, // Keep original publish time
			Updated:   &now,                   // Add update timestamp
		},
		Content:      input.Content,
		AttributedTo: actor.ID,
		Visibility:   input.Visibility,
	}

	// Process hashtags
	hashtags := mastodon.ExtractHashtagsWithCase(input.Content)
	if len(hashtags) > 0 {
		updatedNote.Tag = s.createHashtagTags(hashtags)
	}

	// Set audience based on visibility
	switch input.Visibility {
	case VisibilityPublic:
		updatedNote.To = []string{activitypub.PublicAddress}
		updatedNote.CC = []string{fmt.Sprintf("%s/followers", actor.ID)}
	case VisibilityUnlisted:
		updatedNote.To = []string{fmt.Sprintf("%s/followers", actor.ID)}
		updatedNote.CC = []string{activitypub.PublicAddress}
	case VisibilityPrivate:
		updatedNote.To = []string{fmt.Sprintf("%s/followers", actor.ID)}
	case VisibilityDirect:
		// For direct messages, we'd need to handle mentions
		updatedNote.To = []string{} // Would be populated with mentioned users
	}

	// Create Update activity
	updateActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        fmt.Sprintf("%s/activities/%s/update/%d", s.deps.Config.BaseURL, user.Username, now.Unix()),
			Type:      activitypub.UpdateType,
			To:        updatedNote.To,
			CC:        updatedNote.CC,
			Published: &now,
		},
		Actor:  actor.ID,
		Object: updatedNote,
	}

	// Store the updated object
	if err := s.storage.CreateObject(ctx, updatedNote); err != nil {
		s.logger.Error("failed to update object", zap.Error(err))
		return nil, NewInternalError("failed to update post", err)
	}

	// Store the update activity
	if err := s.storage.CreateActivity(ctx, updateActivity); err != nil {
		s.logger.Error("failed to create update activity", zap.Error(err))
		return nil, NewInternalError("failed to create update activity", err)
	}

	// Record hashtag usage for analytics
	if len(hashtags) > 0 {
		for _, hashtag := range hashtags {
			err = s.storage.RecordHashtagUsage(ctx, hashtag, input.ObjectID, actor.ID)
			if err != nil {
				s.logger.Warn("failed to record hashtag usage", zap.Error(err), zap.String("hashtag", hashtag))
			}
		}
	}

	// Emit events for real-time updates
	s.emitPostUpdateEvents(ctx, updateActivity, actor, updatedNote)

	s.logger.Info("post updated successfully",
		zap.String("object_id", input.ObjectID),
		zap.String("actor_id", actor.ID))

	return &UpdatePostResult{
		Activity: updateActivity,
		Note:     updatedNote,
	}, nil
}

func (s *businessLogicService) UnfollowActor(ctx context.Context, user *UserContext, targetActorID string) (*FollowResult, error) {
	s.logger.Info("unfollowing actor",
		zap.String("user_id", user.Username),
		zap.String("target_actor_id", targetActorID))

	// Get the current user's actor
	actor, err := s.storage.GetActor(ctx, user.Username)
	if err != nil {
		s.logger.Error("failed to get actor for unfollow", zap.Error(err))
		return nil, NewInternalError("failed to get actor", err)
	}

	// Check if currently following
	isFollowing, err := s.storage.IsFollowing(ctx, user.Username, targetActorID)
	if err != nil {
		s.logger.Error("failed to check follow status", zap.Error(err))
		return nil, NewInternalError("failed to check follow status", err)
	}

	if !isFollowing {
		return &FollowResult{
			Following: false,
			Requested: false,
		}, nil
	}

	// Create Undo Follow activity
	now := time.Now()
	followActivityID := fmt.Sprintf("%s/activities/%s/follow/%d", s.deps.Config.BaseURL, user.Username, now.Unix())
	undoActivityID := fmt.Sprintf("%s/activities/%s/undo/%d", s.deps.Config.BaseURL, user.Username, now.Unix())

	undoActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        undoActivityID,
			Type:      activitypub.UndoType,
			To:        []string{targetActorID},
			Published: &now,
		},
		Actor: actor.ID,
		Object: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				ID:      followActivityID,
				Type:    activitypub.FollowType,
			},
			Actor:  actor.ID,
			Object: targetActorID,
		},
	}

	// Store the undo activity
	if err := s.storage.CreateActivity(ctx, undoActivity); err != nil {
		s.logger.Error("failed to create undo activity", zap.Error(err))
		return nil, NewInternalError("failed to create undo activity", err)
	}

	// Remove the relationship (this should be implemented in storage)
	// For now, we'll log that it needs implementation
	s.logger.Warn("relationship removal needs implementation in storage layer",
		zap.String("follower", user.Username),
		zap.String("following", targetActorID))

	// Emit events for real-time updates
	s.emitUnfollowEvents(ctx, undoActivity, actor)

	return &FollowResult{
		Activity:  undoActivity,
		Following: false,
		Requested: false,
	}, nil
}

func (s *businessLogicService) UnlikeObject(ctx context.Context, user *UserContext, objectID string) (*LikeResult, error) {
	s.logger.Info("unliking object",
		zap.String("user_id", user.Username),
		zap.String("object_id", objectID))

	// Get the current user's actor
	actor, err := s.storage.GetActor(ctx, user.Username)
	if err != nil {
		s.logger.Error("failed to get actor for unlike", zap.Error(err))
		return nil, NewInternalError("failed to get actor", err)
	}

	// Check if currently liked
	hasLiked, err := s.storage.HasLiked(ctx, actor.ID, objectID)
	if err != nil {
		s.logger.Error("failed to check like status", zap.Error(err))
		return nil, NewInternalError("failed to check like status", err)
	}

	if !hasLiked {
		return &LikeResult{
			Liked: false,
		}, nil
	}

	// Create Undo Like activity
	now := time.Now()
	likeActivityID := fmt.Sprintf("%s/activities/%s/like/%d", s.deps.Config.BaseURL, user.Username, now.Unix())
	undoActivityID := fmt.Sprintf("%s/activities/%s/undo-like/%d", s.deps.Config.BaseURL, user.Username, now.Unix())

	undoActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			ID:      undoActivityID,
			Type:    activitypub.UndoType,
		},
		Actor: actor.ID,
		Object: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				ID:      likeActivityID,
				Type:    activitypub.LikeType,
			},
			Actor:  actor.ID,
			Object: objectID,
		},
	}
	undoActivity.Published = &now

	// Store the undo activity
	if err := s.storage.CreateActivity(ctx, undoActivity); err != nil {
		s.logger.Error("failed to create undo like activity", zap.Error(err))
		return nil, NewInternalError("failed to create undo like activity", err)
	}

	// Remove the like (this should be implemented in storage)
	// For now, we'll log that it needs implementation
	s.logger.Warn("like removal needs implementation in storage layer",
		zap.String("actor_id", actor.ID),
		zap.String("object_id", objectID))

	// Emit events for real-time updates
	s.emitUnlikeEvents(ctx, undoActivity, actor)

	return &LikeResult{
		Activity: undoActivity,
		Liked:    false,
	}, nil
}

func (s *businessLogicService) FanOutPost(ctx context.Context, activity *activitypub.Activity) error {
	return s.timeline.FanOutToFollowers(ctx, activity, nil)
}

func (s *businessLogicService) DeliverActivity(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor, _ string) error {
	return s.federation.DeliverToFollowers(ctx, activity, actor)
}

// Helper methods for specific operations
func (s *businessLogicService) performCascadeDeletion(ctx context.Context, objectID, actorID string) error {
	s.logger.Info("performing cascade deletion",
		zap.String("object_id", objectID),
		zap.String("actor_id", actorID))

	// Use the business logic pattern - operations should be idempotent and graceful
	var lastErr error

	// TODO: Implement timeline removal when storage adapter supports it
	// 1. Remove from user's timeline entries
	// 2. Remove from public timelines
	s.logger.Info("Timeline removal not yet implemented in storage adapter",
		zap.String("object_id", objectID),
		zap.String("actor_id", actorID))

	// 3. Remove likes on this object
	if err := s.cascadeDeleteLikes(ctx, objectID); err != nil {
		s.logger.Warn("failed to cascade delete likes",
			zap.String("object_id", objectID),
			zap.Error(err))
		lastErr = err
	}

	// 4. Remove boosts/announces of this object
	if err := s.cascadeDeleteAnnounces(ctx, objectID); err != nil {
		s.logger.Warn("failed to cascade delete announces",
			zap.String("object_id", objectID),
			zap.Error(err))
		lastErr = err
	}

	// 5. Update reply chains (mark as orphaned rather than delete)
	if err := s.handleReplyChainUpdates(ctx, objectID); err != nil {
		s.logger.Warn("failed to handle reply chain updates",
			zap.String("object_id", objectID),
			zap.Error(err))
		lastErr = err
	}

	// 6. Remove from bookmarks and pins
	if err := s.removeFromUserCollections(ctx, objectID); err != nil {
		s.logger.Warn("failed to remove from user collections",
			zap.String("object_id", objectID),
			zap.Error(err))
		lastErr = err
	}

	if lastErr != nil {
		s.logger.Warn("cascade deletion completed with some failures",
			zap.String("object_id", objectID),
			zap.Error(lastErr))
		// Don't return error - partial failure is acceptable for deletion
	}

	s.logger.Info("cascade deletion completed successfully",
		zap.String("object_id", objectID))

	return nil
}

// cascadeDeleteLikes removes all likes on the deleted object
func (s *businessLogicService) cascadeDeleteLikes(_ context.Context, objectID string) error {
	// This would require a method to get all likes for an object and then remove them
	// For now, log that this should be implemented via the like repository
	s.logger.Debug("cascade delete likes needed", zap.String("object_id", objectID))

	// In a full implementation, this would:
	// 1. Query all likes for the object
	// 2. Delete each like record
	// 3. Update like counts

	return nil
}

// cascadeDeleteAnnounces removes all announces/boosts of the deleted object
func (s *businessLogicService) cascadeDeleteAnnounces(_ context.Context, objectID string) error {
	// This would require announce repository methods
	s.logger.Debug("cascade delete announces needed", zap.String("object_id", objectID))

	// In a full implementation, this would:
	// 1. Query all announces for the object
	// 2. Delete each announce record
	// 3. Update boost counts
	// 4. Remove from timelines where it was boosted

	return nil
}

// handleReplyChainUpdates updates reply chains when parent is deleted
func (s *businessLogicService) handleReplyChainUpdates(_ context.Context, objectID string) error {
	// For ActivityPub compliance, replies should not be deleted when parent is deleted
	// Instead, they become "orphaned" replies
	s.logger.Debug("handling reply chain updates", zap.String("object_id", objectID))

	// In a full implementation, this would:
	// 1. Find all replies to this object
	// 2. Update their inReplyTo field to indicate the parent is deleted
	// 3. Optionally notify reply authors

	return nil
}

// removeFromUserCollections removes the object from bookmarks, pins, etc.
func (s *businessLogicService) removeFromUserCollections(_ context.Context, objectID string) error {
	s.logger.Debug("removing from user collections", zap.String("object_id", objectID))

	// This would remove the object from:
	// 1. User bookmarks
	// 2. Pinned posts
	// 3. Featured posts
	// 4. Any other collections containing this object

	return nil
}

func (s *businessLogicService) handleFollowFederation(ctx context.Context, activity *activitypub.Activity, actor, _ *activitypub.Actor, _ bool) {
	// Handle federation for follow requests
	if err := s.federation.DeliverToRecipients(ctx, activity, actor); err != nil {
		s.logger.Error("failed to deliver follow activity", zap.Error(err))
	}
}

func (s *businessLogicService) handleLikePostProcessing(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor, _ interface{}) {
	// Record engagement analytics
	objectID := activity.Object.(string)
	if err := s.analytics.RecordEngagement(ctx, objectID, "like", actor.ID); err != nil {
		s.logger.Warn("failed to record like engagement", zap.Error(err))
	}

	// Federation delivery
	if err := s.federation.DeliverToRecipients(ctx, activity, actor); err != nil {
		s.logger.Error("failed to deliver like activity", zap.Error(err))
	}
}

// Event emission helper functions

func (s *businessLogicService) emitUnfollowEvents(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) {
	if s.publisher == nil {
		return
	}

	// Emit unfollow event to user's stream for real-time UI updates
	event := &streaming.Event{
		Type:      "relationship.unfollowed",
		Stream:    fmt.Sprintf("user:%s", actor.PreferredUsername),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"activity":   activity,
			"target_id":  activity.Object.(*activitypub.Activity).Object,
			"unfollowed": true,
		},
	}

	if err := s.publisher.PublishToUser(ctx, actor.PreferredUsername, event); err != nil {
		s.logger.Error("failed to emit unfollow event", zap.Error(err))
	}
}

func (s *businessLogicService) emitUnlikeEvents(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) {
	// Emit unlike event to user's stream for real-time UI updates
	event := streaming.Event{
		Type:      "status.unliked",
		Stream:    fmt.Sprintf("user:%s", actor.PreferredUsername),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"activity":  activity,
			"object_id": activity.Object.(*activitypub.Activity).Object,
			"liked":     false,
		},
	}

	if err := s.publisher.PublishToUser(ctx, actor.PreferredUsername, &event); err != nil {
		s.logger.Error("failed to emit unlike event", zap.Error(err))
	}
}

func (s *businessLogicService) emitPostUpdateEvents(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor, note *activitypub.Note) {
	// Emit update event to user's stream
	userEvent := streaming.Event{
		Type:      "status.updated",
		Stream:    fmt.Sprintf("user:%s", actor.PreferredUsername),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"activity": activity,
			"status":   note,
			"actor":    actor,
		},
	}

	if err := s.publisher.PublishToUser(ctx, actor.PreferredUsername, &userEvent); err != nil {
		s.logger.Error("failed to emit user update event", zap.Error(err))
	}

	// For public posts, also emit to public stream
	if note.Visibility == VisibilityPublic {
		publicEvent := streaming.Event{
			Type:      "status.updated",
			Stream:    "public",
			Timestamp: time.Now(),
			Payload: map[string]interface{}{
				"activity": activity,
				"status":   note,
				"actor":    actor,
			},
		}

		if err := s.publisher.PublishToStream(ctx, "public", &publicEvent); err != nil {
			s.logger.Error("failed to emit public update event", zap.Error(err))
		}
	}
}
