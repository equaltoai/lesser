package services

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/mastodon"
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

func (s *businessLogicService) handleScheduledPost(_ context.Context, _ *UserContext, _ *CreatePostInput) (*CreatePostResult, error) {
	// TODO: Implement scheduled post logic
	return nil, NewValidationError("Scheduled posts not yet implemented")
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
func (s *businessLogicService) UpdatePost(_ context.Context, _ *UserContext, _ *UpdatePostInput) (*UpdatePostResult, error) {
	return nil, NewValidationError("Update post not yet implemented")
}

func (s *businessLogicService) UnfollowActor(_ context.Context, _ *UserContext, _ string) (*FollowResult, error) {
	return nil, NewValidationError("Unfollow actor not yet implemented")
}

func (s *businessLogicService) UnlikeObject(_ context.Context, _ *UserContext, _ string) (*LikeResult, error) {
	return nil, NewValidationError("Unlike object not yet implemented")
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