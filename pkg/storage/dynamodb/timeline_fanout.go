package dynamodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	cfg "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// FanOutPost writes a post to all relevant timelines (followers' home timelines, public timeline, etc.)
func (s *dynamoDBStorage) FanOutPost(ctx context.Context, activity *activitypub.Activity) error {
	log := common.Logger().With(
		zap.String("activity_id", activity.ID),
		zap.String("activity_type", activity.Type),
		zap.String("actor", activity.Actor),
	)

	// Only fan out Create activities
	if activity.Type != activitypub.CreateType {
		return nil
	}

	// Extract the object from the activity
	var object map[string]any
	var tags []activitypub.Tag

	switch obj := activity.Object.(type) {
	case map[string]any:
		object = obj
		// Extract tags if present
		if tagList, ok := obj["tag"].([]any); ok {
			for _, t := range tagList {
				if tagMap, ok := t.(map[string]any); ok {
					tag := activitypub.Tag{
						Type: getStringFromMap(tagMap, "type"),
						Name: getStringFromMap(tagMap, "name"),
						Href: getStringFromMap(tagMap, "href"),
					}
					tags = append(tags, tag)
				}
			}
		}
	case *activitypub.Note:
		// Convert Note to map for easier processing
		// In production, you'd want a proper conversion function
		object = map[string]any{
			"id":           obj.ID,
			"type":         obj.Type,
			"content":      obj.Content,
			"attributedTo": obj.AttributedTo,
			"to":           obj.To,
			"cc":           obj.CC,
			"inReplyTo":    obj.InReplyTo,
			"sensitive":    obj.Sensitive,
			"summary":      obj.Summary,
		}
		// Use tags directly from Note
		tags = obj.Tag
	default:
		log.Warn("unsupported object type for fan-out", zap.Any("object", activity.Object))
		return nil
	}

	// Extract basic info from the object
	objectID, _ := object["id"].(string)
	objectType, _ := object["type"].(string)
	content, _ := object["content"].(string)
	attributedTo, _ := object["attributedTo"].(string)
	inReplyTo, _ := object["inReplyTo"].(string)
	sensitive, _ := object["sensitive"].(bool)
	summary, _ := object["summary"].(string)

	if objectID == "" || attributedTo == "" {
		log.Error("missing required fields in object", zap.Any("object", object))
		return fmt.Errorf("object missing required fields")
	}

	// Extract username from actor ID
	username := extractUsernameFromActorID(attributedTo)
	if username == "" {
		log.Error("failed to extract username from actor", zap.String("actor", attributedTo))
		return fmt.Errorf("invalid actor ID")
	}

	// Determine visibility
	visibility := s.determineVisibility(object)

	// Create base timeline entry
	baseEntry := &storage.TimelineEntry{
		PostID:      objectID,
		ActorID:     attributedTo,
		ActorHandle: username,
		Content:     truncateContent(content, 500),
		ContentType: objectType,
		HasMedia:    hasMediaAttachments(object),
		IsReply:     inReplyTo != "",
		InReplyTo:   inReplyTo,
		IsBoost:     false,
		Visibility:  visibility,
		Language:    extractLanguage(object),
		Sensitive:   sensitive,
		SpoilerText: summary,
		CreatedAt:   extractPublishedTime(object),
		TimelineAt:  time.Now(),
	}

	var entries []*storage.TimelineEntry

	// Fan out to followers' home timelines (for all visibility levels except direct)
	if visibility != "direct" {
		followerEntries, err := s.createFollowerTimelineEntries(ctx, username, baseEntry)
		if err != nil {
			log.Error("failed to create follower timeline entries", zap.Error(err))
			// Continue with other timelines even if this fails
		} else {
			entries = append(entries, followerEntries...)
		}
	}

	// Add to public timelines if public or unlisted
	if visibility == "public" || visibility == "unlisted" {
		// Add to federated public timeline
		publicEntry := *baseEntry
		publicEntry.TimelineType = "PUBLIC"
		publicEntry.TimelineID = "FEDERATED"
		publicEntry.EntryID = s.timelineSK(publicEntry.TimelineAt, publicEntry.PostID)
		entries = append(entries, &publicEntry)

		// Add to local public timeline if it's a local user
		if strings.HasPrefix(attributedTo, cfg.Get().BaseURL()) {
			localEntry := *baseEntry
			localEntry.TimelineType = "PUBLIC"
			localEntry.TimelineID = "LOCAL"
			localEntry.EntryID = s.timelineSK(localEntry.TimelineAt, localEntry.PostID)
			entries = append(entries, &localEntry)
		}
	}

	// Add to hashtag timelines for public posts
	if visibility == "public" && len(tags) > 0 {
		for _, tag := range tags {
			if tag.Type == "Hashtag" && tag.Name != "" {
				// Extract hashtag name (remove # prefix if present)
				hashtagName := strings.TrimPrefix(tag.Name, "#")
				hashtagName = strings.ToLower(hashtagName) // Normalize to lowercase

				hashtagEntry := *baseEntry
				hashtagEntry.TimelineType = "HASHTAG"
				hashtagEntry.TimelineID = hashtagName
				hashtagEntry.EntryID = s.timelineSK(hashtagEntry.TimelineAt, hashtagEntry.PostID)
				entries = append(entries, &hashtagEntry)
			}
		}
	}

	// Add to list timelines
	// For posts that are public, unlisted, or private (not direct messages)
	if visibility != "direct" {
		listEntries, err := s.createListTimelineEntries(ctx, username, baseEntry)
		if err != nil {
			log.Error("failed to create list timeline entries", zap.Error(err))
			// Continue even if this fails
		} else {
			entries = append(entries, listEntries...)
		}
	}

	// Write all entries to timelines
	if len(entries) > 0 {
		if err := s.WriteToTimelines(ctx, entries); err != nil {
			log.Error("failed to write to timelines", zap.Error(err), zap.Int("entry_count", len(entries)))
			return fmt.Errorf("failed to write to timelines: %w", err)
		}
	}

	log.Info("successfully fanned out post", zap.Int("timeline_count", len(entries)))
	return nil
}

// createFollowerTimelineEntries creates timeline entries for all followers
func (s *dynamoDBStorage) createFollowerTimelineEntries(ctx context.Context, username string, baseEntry *storage.TimelineEntry) ([]*storage.TimelineEntry, error) {
	log := common.Logger().With(zap.String("username", username))

	var entries []*storage.TimelineEntry
	cursor := ""

	// Paginate through all followers
	for {
		followers, nextCursor, err := s.GetFollowers(ctx, username, 100, cursor)
		if err != nil {
			log.Error("failed to get followers", zap.Error(err))
			return nil, fmt.Errorf("failed to get followers: %w", err)
		}

		// Create timeline entry for each follower
		for _, followerID := range followers {
			// Extract follower username
			followerUsername := extractUsernameFromActorID(followerID)
			if followerUsername == "" {
				log.Warn("invalid follower ID", zap.String("follower_id", followerID))
				continue
			}

			// Create timeline entry for this follower
			entry := *baseEntry
			entry.TimelineType = "HOME"
			entry.TimelineID = followerUsername
			entry.EntryID = s.timelineSK(entry.TimelineAt, entry.PostID)
			entries = append(entries, &entry)
		}

		// Check if there are more followers
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	log.Debug("created follower timeline entries", zap.Int("count", len(entries)))
	return entries, nil
}

// createListTimelineEntries creates timeline entries for lists containing the actor
func (s *dynamoDBStorage) createListTimelineEntries(ctx context.Context, username string, baseEntry *storage.TimelineEntry) ([]*storage.TimelineEntry, error) {
	log := common.Logger().With(zap.String("username", username))

	// Get all lists that contain this account
	lists, err := s.GetListsContainingAccount(ctx, username, "")
	if err != nil {
		log.Error("failed to get lists containing account", zap.Error(err))
		return nil, fmt.Errorf("failed to get lists containing account: %w", err)
	}

	var entries []*storage.TimelineEntry

	for _, list := range lists {
		// Check list replies policy
		shouldInclude := false
		switch list.RepliesPolicy {
		case "none":
			// No replies
			shouldInclude = baseEntry.InReplyTo == ""
		case "followed":
			// Replies to followed accounts only
			// For now, include all non-replies. In the future, check if replied-to account is followed
			shouldInclude = baseEntry.InReplyTo == ""
		case "list":
			// All posts including replies
			shouldInclude = true
		default:
			// Default to list policy
			shouldInclude = true
		}

		if shouldInclude {
			// Create timeline entry for this list
			entry := *baseEntry
			entry.TimelineType = "LIST"
			entry.TimelineID = list.ID
			entry.EntryID = s.timelineSK(entry.TimelineAt, entry.PostID)
			entries = append(entries, &entry)
		}
	}

	log.Debug("created list timeline entries", zap.Int("count", len(entries)))
	return entries, nil
}

// determineVisibility determines the visibility of a post based on addressing
func (s *dynamoDBStorage) determineVisibility(object map[string]any) string {
	to := convertToStringSlice(object["to"])
	cc := convertToStringSlice(object["cc"])

	// Direct message - no public addressing
	if !contains(to, activitypub.PublicAddress) && !contains(cc, activitypub.PublicAddress) {
		return "direct"
	}

	// Public - addressed to public in 'to'
	if contains(to, activitypub.PublicAddress) {
		return "public"
	}

	// Unlisted - public in 'cc'
	if contains(cc, activitypub.PublicAddress) {
		return "unlisted"
	}

	// Private - followers only
	return "private"
}

// Helper functions

func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen]
}

func hasMediaAttachments(object map[string]any) bool {
	attachments, ok := object["attachment"].([]any)
	return ok && len(attachments) > 0
}

func extractLanguage(object map[string]any) string {
	if lang, ok := object["language"].(string); ok {
		return lang
	}
	if langMap, ok := object["contentMap"].(map[string]any); ok && len(langMap) > 0 {
		// Return the first language found
		for lang := range langMap {
			return lang
		}
	}
	return "en" // Default to English
}

func extractPublishedTime(object map[string]any) time.Time {
	if published, ok := object["published"].(string); ok {
		if t, err := time.Parse(time.RFC3339, published); err == nil {
			return t
		}
	}
	return time.Now()
}

func convertToStringSlice(v any) []string {
	if v == nil {
		return []string{}
	}

	switch val := v.(type) {
	case []string:
		return val
	case []any:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	case string:
		return []string{val}
	default:
		return []string{}
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func getStringFromMap(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
