package graph

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// deriveVisibility determines the visibility level based on To and CC fields
func deriveVisibility(to, cc []string) model.Visibility {
	// Check for public visibility
	publicURI := "https://www.w3.org/ns/activitystreams#Public"
	for _, t := range to {
		if t == publicURI {
			return model.VisibilityPublic
		}
	}
	for _, c := range cc {
		if c == publicURI {
			return model.VisibilityUnlisted
		}
	}

	// If it has followers collection, it's followers-only
	for _, t := range to {
		if strings.Contains(t, "/followers") {
			return model.VisibilityFollowers
		}
	}

	// Otherwise it's direct
	return model.VisibilityDirect
}

// convertMentions extracts mentions from tags
func convertMentions(tags []activitypub.Tag) []*model.Mention {
	mentions := make([]*model.Mention, 0)
	for _, tag := range tags {
		if tag.Type == "Mention" {
			mention := &model.Mention{
				ID:  tag.Href,
				URL: tag.Href,
			}
			// Extract username from href if possible
			if tag.Name != "" {
				mention.Username = strings.TrimPrefix(tag.Name, "@")
			}
			mentions = append(mentions, mention)
		}
	}
	return mentions
}

// convertTags filters tags to exclude mentions
func convertTags(tags []activitypub.Tag) []*activitypub.Tag {
	result := make([]*activitypub.Tag, 0, len(tags))
	for i := range tags {
		// Filter out mentions, keep only hashtags
		if tags[i].Type != "Mention" {
			result = append(result, &tags[i])
		}
	}
	return result
}

// convertAttachments converts attachment slice to pointer slice
func convertAttachments(attachments []activitypub.Attachment) []*activitypub.Attachment {
	result := make([]*activitypub.Attachment, 0, len(attachments))
	for i := range attachments {
		result = append(result, &attachments[i])
	}
	return result
}

// getTimeOrNow returns the time or current time if nil
func getTimeOrNow(t *time.Time) time.Time {
	if t != nil {
		return *t
	}
	return time.Now()
}

// getUsernameFromContext extracts username from authentication context
func getUsernameFromContext(ctx context.Context) string {
	// Extract claims from context
	if claims, ok := ctx.Value(auth.ContextKeyClaims).(*auth.Claims); ok && claims != nil {
		return claims.Username
	}
	return ""
}

// GetUserID extracts user ID from authentication context
func GetUserID(ctx context.Context) string {
	// Try to get claims from context
	if claims, ok := ctx.Value(auth.ContextKeyClaims).(*auth.Claims); ok && claims != nil {
		return claims.Username // In this system, username is used as user ID
	}
	return ""
}

// convertToGraphQLObject converts storage objects to GraphQL model objects
func (r *queryResolver) convertToGraphQLObject(ctx context.Context, obj any) *model.Object {
	// Reuse logic from Object resolver
	switch o := obj.(type) {
	case *activitypub.Note:
		result := &model.Object{
			ID:          o.ID,
			Type:        model.ObjectTypeNote,
			Content:     o.Content,
			Visibility:  deriveVisibility(o.To, o.CC),
			Sensitive:   o.Sensitive,
			Attachments: convertAttachments(o.Attachment),
			Tags:        convertTags(o.Tag),
			Mentions:    convertMentions(o.Tag),
			CreatedAt:   model.Time(getTimeOrNow(o.Published)),
			UpdatedAt:   model.Time(getTimeOrNow(o.Updated)),
			// Get interaction counts from storage
			RepliesCount: r.getObjectReplyCount(ctx, o.ID),
			LikesCount:   r.getObjectLikeCount(ctx, o.ID),
			SharesCount:  r.getObjectShareCount(ctx, o.ID),
		}

		// Handle spoiler text (content warning)
		if o.Summary != "" {
			result.SpoilerText = &o.Summary
		}

		// Load actor using DataLoader
		if o.AttributedTo != "" {
			actor, err := LoadActor(ctx, o.AttributedTo)
			if err == nil {
				result.Actor = actor
			}
		}
		return result

	case *activitypub.Article:
		result := &model.Object{
			ID:          o.ID,
			Type:        model.ObjectTypeArticle,
			Content:     o.Content,
			Visibility:  deriveVisibility(o.To, o.CC),
			Sensitive:   o.Sensitive,
			Attachments: convertAttachments(o.Attachment),
			Tags:        convertTags(o.Tag),
			Mentions:    convertMentions(o.Tag),
			CreatedAt:   model.Time(getTimeOrNow(o.Published)),
			UpdatedAt:   model.Time(getTimeOrNow(o.Updated)),
			// Get interaction counts from storage
			RepliesCount: r.getObjectReplyCount(ctx, o.ID),
			LikesCount:   r.getObjectLikeCount(ctx, o.ID),
			SharesCount:  r.getObjectShareCount(ctx, o.ID),
		}

		// Articles use Name for title
		if o.Name != "" {
			result.SpoilerText = &o.Name
		}

		// Load actor using DataLoader
		if o.AttributedTo != "" {
			actor, err := LoadActor(ctx, o.AttributedTo)
			if err == nil {
				result.Actor = actor
			}
		}
		return result

	case *activitypub.Image:
		result := &model.Object{
			ID:         o.ID,
			Type:       model.ObjectTypeImage,
			Content:    o.Summary, // Images use summary for description
			Visibility: deriveVisibility(o.To, o.CC),
			Sensitive:  o.Sensitive,
			Attachments: []*activitypub.Attachment{
				{
					Type:      "Image",
					MediaType: o.MediaType,
					URL:       o.URL,
					Width:     o.Width,
					Height:    o.Height,
				},
			},
			Tags:      []*activitypub.Tag{},
			Mentions:  []*model.Mention{},
			CreatedAt: model.Time(getTimeOrNow(o.Published)),
			UpdatedAt: model.Time(getTimeOrNow(o.Updated)),
			// Get interaction counts from storage
			RepliesCount: r.getObjectReplyCount(ctx, o.ID),
			LikesCount:   r.getObjectLikeCount(ctx, o.ID),
			SharesCount:  r.getObjectShareCount(ctx, o.ID),
		}
		return result

	default:
		// Log unsupported type and return nil
		r.Logger.Warn("Unsupported object type in timeline",
			zap.String("type", fmt.Sprintf("%T", obj)))
		return nil
	}
}

// validateNoteInput validates the input for creating a note
func validateNoteInput(input model.CreateNoteInput) error {
	if strings.TrimSpace(input.Content) == "" {
		return fmt.Errorf("content cannot be empty")
	}

	if len(input.Content) > 5000 {
		return fmt.Errorf("content exceeds maximum length of 5000 characters")
	}

	// Validate mentions format
	for _, mention := range input.Mentions {
		if !strings.HasPrefix(mention, "@") {
			return fmt.Errorf("invalid mention format: %s", mention)
		}
	}

	// Validate tags format
	for _, tag := range input.Tags {
		if strings.ContainsAny(tag, " \t\n\r") {
			return fmt.Errorf("invalid tag format: %s", tag)
		}
	}

	return nil
}

// extractDomainFromActorID extracts the domain from an actor ID
func extractDomainFromActorID(actorID string) string {
	// Format: https://domain.com/users/username
	if !strings.HasPrefix(actorID, "https://") && !strings.HasPrefix(actorID, "http://") {
		return ""
	}

	// Remove protocol
	url := actorID
	if strings.HasPrefix(url, "https://") {
		url = url[8:]
	} else if strings.HasPrefix(url, "http://") {
		url = url[7:]
	}

	// Extract domain
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		domain := parts[0]
		// Remove port if present
		if idx := strings.Index(domain, ":"); idx != -1 {
			domain = domain[:idx]
		}
		return domain
	}

	return ""
}

// generateUniqueID generates a unique ID for objects
func generateUniqueID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// determineAudience determines the To field based on visibility
func determineAudience(visibility model.Visibility, actorID string, mentions []string) []string {
	audience := []string{}

	switch visibility {
	case model.VisibilityPublic:
		audience = append(audience, activitypub.PublicAddress)
	case model.VisibilityUnlisted:
		// Unlisted posts go to followers in To field
		audience = append(audience, actorID+"/followers")
	case model.VisibilityFollowers:
		audience = append(audience, actorID+"/followers")
	case model.VisibilityDirect:
		// Direct messages only go to mentioned users
		for _, mention := range mentions {
			// Convert mention to actor ID (simplified - in real implementation would need to resolve)
			// For now, just add as-is if it looks like an actor ID
			if strings.Contains(mention, "@") && !strings.HasPrefix(mention, "https://") {
				// Skip @username format for now
				continue
			}
			audience = append(audience, mention)
		}
	}

	return audience
}

// determineCCAudience determines the CC field based on visibility
func determineCCAudience(visibility model.Visibility, actorID string) []string {
	cc := []string{}

	switch visibility {
	case model.VisibilityPublic:
		// Public posts CC followers
		cc = append(cc, actorID+"/followers")
	case model.VisibilityUnlisted:
		// Unlisted posts CC public
		cc = append(cc, activitypub.PublicAddress)
	}

	return cc
}

// getSensitive safely extracts the sensitive flag
func getSensitive(sensitive *bool) bool {
	if sensitive != nil {
		return *sensitive
	}
	return false
}

// getSpoilerText safely extracts the spoiler text
func getSpoilerText(spoilerText *string) string {
	if spoilerText != nil {
		return *spoilerText
	}
	return ""
}

// buildTags builds tag array from hashtags and mentions
func buildTags(hashtags []string, mentions []string) []activitypub.Tag {
	tags := []activitypub.Tag{}

	// Add hashtags
	for _, tag := range hashtags {
		// Ensure tag starts with #
		if !strings.HasPrefix(tag, "#") {
			tag = "#" + tag
		}

		tags = append(tags, activitypub.Tag{
			Type: "Hashtag",
			Name: tag,
			Href: fmt.Sprintf("https://localhost/tags/%s", strings.TrimPrefix(tag, "#")),
		})
	}

	// Add mentions
	for _, mention := range mentions {
		// For now, create basic mention tags
		// In real implementation, would need to resolve the mention to get proper href
		tags = append(tags, activitypub.Tag{
			Type: "Mention",
			Name: mention,
			Href: mention, // This should be the actor ID
		})
	}

	return tags
}

// buildAttachments builds attachment objects from media IDs
func (r *mutationResolver) buildAttachments(_ context.Context, attachmentIDs []string) ([]activitypub.Attachment, error) {
	attachments := []activitypub.Attachment{}

	for _, id := range attachmentIDs {
		// In a real implementation, would fetch media details from storage
		// For now, create a basic attachment
		attachments = append(attachments, activitypub.Attachment{
			Type:      "Document",
			MediaType: "image/jpeg", // Would be determined from actual media
			URL:       fmt.Sprintf("https://localhost/media/%s", id),
		})
	}

	return attachments, nil
}

// shouldFederate determines if an activity should be federated based on visibility
func shouldFederate(_ model.Visibility) bool {
	// Direct messages and followers-only posts should still be federated to the right recipients
	// Only truly local content wouldn't be federated (but we don't have that visibility level)
	return true
}

// convertToGraphQLObject converts an ActivityPub object to GraphQL Object type
func (r *mutationResolver) convertToGraphQLObject(ctx context.Context, obj any) *model.Object {
	// Reuse the query resolver's method
	qr := &queryResolver{r.Resolver}
	return qr.convertToGraphQLObject(ctx, obj)
}

// getObjectActorID extracts the actor ID from an object
func getObjectActorID(obj any) string {
	switch o := obj.(type) {
	case *activitypub.Note:
		return o.AttributedTo
	case *activitypub.Article:
		return o.AttributedTo
	case *activitypub.Image:
		// Images don't have AttributedTo, they're usually attachments
		return ""
	case *activitypub.Activity:
		return o.Actor
	default:
		return ""
	}
}

// determineModerationCategory categorizes the moderation reason
func determineModerationCategory(reason string) string {
	lowerReason := strings.ToLower(reason)

	if strings.Contains(lowerReason, "spam") {
		return "spam"
	}
	if strings.Contains(lowerReason, "violence") || strings.Contains(lowerReason, "hate") ||
		strings.Contains(lowerReason, "harassment") || strings.Contains(lowerReason, "abuse") {
		return "abuse"
	}
	if strings.Contains(lowerReason, "illegal") || strings.Contains(lowerReason, "law") {
		return "illegal"
	}
	if strings.Contains(lowerReason, "nsfw") || strings.Contains(lowerReason, "adult") ||
		strings.Contains(lowerReason, "sexual") {
		return "nsfw"
	}
	if strings.Contains(lowerReason, "misinformation") || strings.Contains(lowerReason, "fake") {
		return "misinformation"
	}

	return "other"
}

// Helper methods for getting object interaction counts
func (r *queryResolver) getObjectReplyCount(ctx context.Context, objectID string) int {
	count, err := r.Storage.Object().CountObjectReplies(ctx, objectID)
	if err != nil {
		r.Logger.Error("failed to get reply count", zap.Error(err), zap.String("objectID", objectID))
		return 0
	}
	return count
}

func (r *queryResolver) getObjectLikeCount(ctx context.Context, objectID string) int {
	count, err := r.Storage.Like().GetLikeCount(ctx, objectID)
	if err != nil {
		r.Logger.Error("failed to get like count", zap.Error(err), zap.String("objectID", objectID))
		return 0
	}
	return int(count)
}

func (r *queryResolver) getObjectShareCount(ctx context.Context, objectID string) int {
	count, err := r.Storage.Like().GetBoostCount(ctx, objectID)
	if err != nil {
		r.Logger.Error("failed to get share count", zap.Error(err), zap.String("objectID", objectID))
		return 0
	}
	return int(count)
}

// calculateMissingPosts calculates the number of missing posts in a thread
func (r *Resolver) calculateMissingPosts(ctx context.Context, threadContext *storage.ThreadContext) int {
	if threadContext == nil {
		return 0
	}

	// Count gaps in the thread by looking for missing replies
	// This is a simplified implementation - a more sophisticated version would:
	// 1. Analyze reply chains for gaps
	// 2. Check for orphaned replies (replies without visible parents)
	// 3. Compare with known reply counts from remote servers

	missingCount := 0

	// Check if we have ancestors/descendants that reference missing posts
	// StatusSearchResult doesn't have Object field, so we check if we can retrieve the actual object
	for _, ancestor := range threadContext.Ancestors {
		if ancestor != "" {
			if _, err := r.Storage.Object().GetObject(ctx, ancestor); err != nil {
				missingCount++
			}
		}
	}

	for _, descendant := range threadContext.Descendants {
		if descendant != "" {
			if _, err := r.Storage.Object().GetObject(ctx, descendant); err != nil {
				missingCount++
			}
		}
	}

	// Additional heuristic: if we have very few replies but the post seems popular
	// (based on likes/shares), there might be missing replies
	totalPosts := len(threadContext.Ancestors) + len(threadContext.Descendants)
	if totalPosts < 3 {
		// For small threads, estimate missing posts based on engagement
		// This is a rough heuristic and could be improved with more sophisticated analysis
		avgEngagement := r.calculateAverageEngagement(ctx, threadContext)
		if avgEngagement > 10 { // High engagement suggests missing replies
			missingCount += int(avgEngagement / 20) // Rough estimate
		}
	}

	return missingCount
}

// calculateAverageEngagement calculates average engagement for posts in the thread
func (r *Resolver) calculateAverageEngagement(ctx context.Context, threadContext *storage.ThreadContext) float64 {
	if threadContext == nil {
		return 0
	}

	totalEngagement := 0
	postCount := 0

	// Sum engagement from all posts in the thread
	allPosts := append(threadContext.Ancestors, threadContext.Descendants...)
	for _, post := range allPosts {
		if post != "" {
			// Get engagement metrics for this post using StatusID
			likes, _ := r.Storage.Like().GetLikeCount(ctx, post)
			shares, _ := r.Storage.Like().GetBoostCount(ctx, post)
			replies, _ := r.Storage.Object().CountObjectReplies(ctx, post)

			totalEngagement += int(likes) + int(shares) + replies
			postCount++
		}
	}

	if postCount == 0 {
		return 0
	}

	return float64(totalEngagement) / float64(postCount)
}

// getObjectID extracts the ID from an ActivityPub object
func (r *Resolver) getObjectID(obj any) string {
	switch o := obj.(type) {
	case *activitypub.Note:
		return o.ID
	case *activitypub.Article:
		return o.ID
	case *activitypub.Image:
		return o.ID
	default:
		return ""
	}
}
