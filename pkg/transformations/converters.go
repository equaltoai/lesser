// Package transformations provides utility functions for converting ActivityPub
// objects and actors to Mastodon API compatible models and data structures.
package transformations

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
)

// Actor conversion utilities

// ActorToAccountWithCounts provides actor to account conversion with count values
func ActorToAccountWithCounts(actor *activitypub.Actor, baseURL string, followersCount, followingCount, statusesCount int) models.Account {
	if actor == nil {
		return models.Account{}
	}

	account := ActorToAccountBase(actor, baseURL)
	account.FollowersCount = followersCount
	account.FollowingCount = followingCount
	account.StatusesCount = statusesCount

	return account
}

// ActorToAccountBase provides base actor to account conversion
func ActorToAccountBase(actor *activitypub.Actor, baseURL string) models.Account {
	if actor == nil {
		return models.Account{}
	}

	var createdAt string
	if actor.Published != nil {
		createdAt = TransformTimestamp(*actor.Published, time.RFC3339)
	}

	return models.Account{
		ID:             GenerateNumericIDFromUsername(actor.PreferredUsername),
		Username:       actor.PreferredUsername,
		Acct:           actor.PreferredUsername,
		DisplayName:    actor.Name,
		Note:           actor.Summary,
		URL:            actor.URL,
		Avatar:         getAvatarURL(actor.Icon, baseURL),
		AvatarStatic:   getAvatarURL(actor.Icon, baseURL),
		Header:         getHeaderURL(actor.Image, baseURL),
		HeaderStatic:   getHeaderURL(actor.Image, baseURL),
		Locked:         actor.ManuallyApprovesFollowers,
		Bot:            isBot(actor.Type),
		CreatedAt:      createdAt,
		FollowersCount: 0, // To be set by caller
		FollowingCount: 0, // To be set by caller
		StatusesCount:  0, // To be set by caller
		Discoverable:   actor.Discoverable,
		Fields:         transformAttachments(actor.Attachment),
		Emojis:         []any{}, // To be populated by emoji service
	}
}

// Object conversion utilities

// ObjectToStatusAny provides flexible object to status conversion for any object type
func ObjectToStatusAny(obj any, actor *activitypub.Actor, baseURL string) models.Status {
	if obj == nil || actor == nil {
		return models.Status{}
	}

	// Convert object to map for unified processing
	objMap := convertObjectToMap(obj)
	return ObjectToStatusBase(objMap, actor, baseURL)
}

// ObjectToStatusBase provides base object to status conversion
func ObjectToStatusBase(obj map[string]interface{}, actor *activitypub.Actor, baseURL string) models.Status {
	if obj == nil || actor == nil {
		return models.Status{}
	}

	content, _ := obj["content"].(string)
	published, _ := obj["published"].(string)
	id, _ := obj["id"].(string)

	var language string
	if lang := extractLanguage(obj); lang != nil {
		language = *lang
	}

	status := models.Status{
		ID:                 GenerateNumericIDFromURL(id),
		URI:                id,
		URL:                id,
		Account:            ActorToAccountBase(actor, baseURL),
		InReplyToID:        extractReplyToID(obj),
		InReplyToAccountID: nil,
		Reblog:             nil,
		Content:            content,
		CreatedAt:          published,
		ReblogsCount:       0,
		FavouritesCount:    0,
		RepliesCount:       0,
		Reblogged:          false,
		Favourited:         false,
		Bookmarked:         false,
		Sensitive:          extractSensitive(obj),
		SpoilerText:        extractSpoilerText(obj),
		Visibility:         "public", // Default, to be determined by caller
		MediaAttachments:   transformMediaAttachments(obj),
		Mentions:           transformMentions(obj),
		Tags:               transformTags(obj),
		Emojis:             []any{},
		Card:               nil,
		Poll:               nil,
		Language:           language,
	}

	status.AgentAttribution = buildAgentPostAttribution(actor)

	return status
}

// Helper functions for transformations

func getAvatarURL(icon interface{}, baseURL string) string {
	if icon == nil {
		return baseURL + "/avatars/original/missing.png"
	}

	if iconMap, ok := icon.(map[string]interface{}); ok {
		if url, ok := iconMap["url"].(string); ok {
			return url
		}
	}

	return baseURL + "/avatars/original/missing.png"
}

func getHeaderURL(image interface{}, baseURL string) string {
	if image == nil {
		return baseURL + "/headers/original/missing.png"
	}

	if imageMap, ok := image.(map[string]interface{}); ok {
		if url, ok := imageMap["url"].(string); ok {
			return url
		}
	}

	return baseURL + "/headers/original/missing.png"
}

func isBot(actorType string) bool {
	botTypes := []string{"Service", "Application"}
	return common.ValidateEnumField(actorType, botTypes, "actor_type") == nil
}

func buildAgentPostAttribution(actor *activitypub.Actor) *models.AgentPostAttribution {
	if actor == nil {
		return nil
	}

	if !isBot(actor.Type) && actor.AgentManifest == nil {
		return nil
	}

	attribution := &models.AgentPostAttribution{
		ModelVersion: "unknown",
	}

	if actor.AgentManifest != nil {
		if v := strings.TrimSpace(actor.AgentManifest.Version); v != "" {
			attribution.ModelVersion = v
		}
		if operatedBy := strings.TrimSpace(actor.AgentManifest.OperatedBy); operatedBy != "" {
			attribution.DelegatedBy = operatedBy
		}
		attribution.Constraints = buildAgentConstraints(actor.AgentManifest.Capabilities)
	}

	return attribution
}

func buildAgentConstraints(caps *activitypub.AgentCapabilities) []string {
	if caps == nil {
		return nil
	}

	constraints := make([]string, 0, 4)
	if caps.MaxPostsPerHour > 0 {
		constraints = append(constraints, fmt.Sprintf("max_posts_per_hour:%d", caps.MaxPostsPerHour))
	}
	if caps.RequiresApproval {
		constraints = append(constraints, "requires_approval")
	}
	if len(caps.RestrictedDomains) > 0 {
		constraints = append(constraints, fmt.Sprintf("restricted_domains:%s", strings.Join(caps.RestrictedDomains, ",")))
	}

	return constraints
}

func transformAttachments(attachment interface{}) []any {
	// Transform ActivityPub attachment property to Mastodon Field format
	if attachment == nil {
		return []any{}
	}

	// Handle different attachment formats from ActivityPub
	switch att := attachment.(type) {
	case []interface{}:
		// Array of attachments
		fields := make([]any, 0, len(att))
		for _, item := range att {
			if field := processAttachmentItem(item); field != nil {
				fields = append(fields, field)
			}
		}
		return fields
	case interface{}:
		// Single attachment
		if field := processAttachmentItem(att); field != nil {
			return []any{field}
		}
	}

	return []any{}
}

// processAttachmentItem processes a single attachment item and converts it to Mastodon Field format
func processAttachmentItem(item interface{}) interface{} {
	itemMap, ok := item.(map[string]interface{})
	if !ok {
		return nil
	}

	// Extract name and value from ActivityPub attachment
	name, _ := itemMap["name"].(string)
	value, _ := itemMap["value"].(string)

	// If no name or value, skip this attachment
	if name == "" && value == "" {
		return nil
	}

	field := map[string]interface{}{
		"name":  name,
		"value": value,
	}

	// Add verified_at if present (for link verification)
	if verifiedAt, ok := itemMap["verified_at"].(string); ok && verifiedAt != "" {
		field["verified_at"] = verifiedAt
	}

	return field
}

func extractReplyToID(obj map[string]interface{}) *string {
	if inReplyTo, ok := obj["inReplyTo"].(string); ok && inReplyTo != "" {
		id := GenerateNumericIDFromURL(inReplyTo)
		return &id
	}
	return nil
}

func extractSensitive(obj map[string]interface{}) bool {
	if sensitive, ok := obj["sensitive"].(bool); ok {
		return sensitive
	}
	return false
}

func extractSpoilerText(obj map[string]interface{}) string {
	if spoiler, ok := obj["summary"].(string); ok {
		return spoiler
	}
	return ""
}

func extractLanguage(obj map[string]interface{}) *string {
	if lang, ok := obj["contentMap"].(map[string]interface{}); ok {
		for language := range lang {
			return &language
		}
	}
	return nil
}

func transformMediaAttachments(obj map[string]interface{}) []any {
	// Transform ActivityPub attachment array to Mastodon MediaAttachment format
	attachmentValue, exists := obj["attachment"]
	if !exists || attachmentValue == nil {
		return []any{}
	}

	attachments, ok := attachmentValue.([]interface{})
	if !ok {
		return []any{}
	}

	mediaAttachments := make([]any, 0, len(attachments))
	for _, attachment := range attachments {
		if mediaAttachment := processMediaAttachmentItem(attachment); mediaAttachment != nil {
			mediaAttachments = append(mediaAttachments, mediaAttachment)
		}
	}

	return mediaAttachments
}

// processMediaAttachmentItem processes a single media attachment item
func processMediaAttachmentItem(item interface{}) interface{} {
	itemMap, ok := item.(map[string]interface{})
	if !ok {
		return nil
	}

	// Extract media type to determine if this is a media attachment
	mediaType, _ := itemMap["mediaType"].(string)
	attachmentType, _ := itemMap["type"].(string)
	url, _ := itemMap["url"].(string)

	// Only process actual media attachments (not property value attachments)
	if attachmentType != "Document" && attachmentType != "Image" && attachmentType != "Video" && attachmentType != "Audio" {
		return nil
	}

	if url == "" {
		return nil
	}

	// Generate ID from URL
	id := GenerateNumericIDFromURL(url)

	// Determine Mastodon media type
	mastodonType := "unknown"
	if strings.HasPrefix(mediaType, "image/") || attachmentType == "Image" {
		mastodonType = "image"
	} else if strings.HasPrefix(mediaType, "video/") || attachmentType == "Video" {
		mastodonType = "video"
	} else if strings.HasPrefix(mediaType, "audio/") || attachmentType == "Audio" {
		mastodonType = "audio"
	}

	mediaAttachment := map[string]interface{}{
		"id":          id,
		"type":        mastodonType,
		"url":         url,
		"preview_url": url, // Use same URL for preview by default
		"remote_url":  url,
		"text_url":    url,
		"meta":        map[string]interface{}{},
		"description": "",
		"blurhash":    "",
	}

	// Add description if available
	if name, ok := itemMap["name"].(string); ok && name != "" {
		mediaAttachment["description"] = name
	}

	// Add width/height to meta if available
	if width, ok := itemMap["width"].(float64); ok {
		if mediaAttachment["meta"] == nil {
			mediaAttachment["meta"] = map[string]interface{}{}
		}
		meta := mediaAttachment["meta"].(map[string]interface{})
		meta["width"] = int(width)
	}
	if height, ok := itemMap["height"].(float64); ok {
		if mediaAttachment["meta"] == nil {
			mediaAttachment["meta"] = map[string]interface{}{}
		}
		meta := mediaAttachment["meta"].(map[string]interface{})
		meta["height"] = int(height)
	}

	return mediaAttachment
}

func transformMentions(obj map[string]interface{}) []any {
	// Transform ActivityPub tag array to Mastodon Mention format
	// In ActivityPub, mentions are stored in the "tag" property with type "Mention"
	tagValue, exists := obj["tag"]
	if !exists || tagValue == nil {
		return []any{}
	}

	tags, ok := tagValue.([]interface{})
	if !ok {
		return []any{}
	}

	mentions := make([]any, 0)
	for _, tag := range tags {
		if mention := processMentionTag(tag); mention != nil {
			mentions = append(mentions, mention)
		}
	}

	return mentions
}

// processMentionTag processes a single tag and extracts mention information
func processMentionTag(tag interface{}) interface{} {
	tagMap, ok := tag.(map[string]interface{})
	if !ok {
		return nil
	}

	// Only process Mention type tags
	tagType, _ := tagMap["type"].(string)
	if tagType != "Mention" {
		return nil
	}

	href, _ := tagMap["href"].(string)
	name, _ := tagMap["name"].(string)

	if href == "" || name == "" {
		return nil
	}

	// Extract username from the name (remove @ prefix)
	username := strings.TrimPrefix(name, "@")

	// Extract domain from href if it's a remote mention
	var domain *string
	if parsed, err := url.Parse(href); err == nil {
		if host := strings.TrimSpace(parsed.Hostname()); host != "" {
			domain = &host
		}
	}

	// Generate account ID from href
	id := GenerateNumericIDFromURL(href)

	mention := map[string]interface{}{
		"id":       id,
		"username": username,
		"acct":     username, // For remote users, this would be username@domain
		"url":      href,
	}

	// Add domain if it's a remote mention
	if domain != nil {
		mention["acct"] = username + "@" + *domain
	}

	return mention
}

func transformTags(obj map[string]interface{}) []any {
	// Transform ActivityPub tag array to Mastodon Tag format
	// In ActivityPub, hashtags are stored in the "tag" property with type "Hashtag"
	tagValue, exists := obj["tag"]
	if !exists || tagValue == nil {
		return []any{}
	}

	tags, ok := tagValue.([]interface{})
	if !ok {
		return []any{}
	}

	hashtags := make([]any, 0)
	for _, tag := range tags {
		if hashtag := processHashtagTag(tag); hashtag != nil {
			hashtags = append(hashtags, hashtag)
		}
	}

	return hashtags
}

// processHashtagTag processes a single tag and extracts hashtag information
func processHashtagTag(tag interface{}) interface{} {
	tagMap, ok := tag.(map[string]interface{})
	if !ok {
		return nil
	}

	// Only process Hashtag type tags
	tagType, _ := tagMap["type"].(string)
	if tagType != "Hashtag" {
		return nil
	}

	href, _ := tagMap["href"].(string)
	name, _ := tagMap["name"].(string)

	if href == "" || name == "" {
		return nil
	}

	// Extract hashtag name (remove # prefix)
	hashtagName := strings.TrimPrefix(name, "#")

	if hashtagName == "" {
		return nil
	}

	hashtag := map[string]interface{}{
		"name": hashtagName,
		"url":  href,
	}

	// Add empty history array for consistency with Mastodon API
	// In a real implementation, this would contain usage statistics
	hashtag["history"] = []interface{}{}

	return hashtag
}

// GenerateNumericIDFromUsername generates a stable numeric ID from username
func GenerateNumericIDFromUsername(username string) string {
	return generateNumericID(username)
}

// GenerateNumericIDFromURL generates a stable numeric ID from URL
func GenerateNumericIDFromURL(url string) string {
	// Extract the last segment of the URL for ID generation
	parts := strings.Split(strings.TrimSuffix(url, "/"), "/")
	if len(parts) > 0 {
		return generateNumericID(parts[len(parts)-1])
	}
	// Fallback to the entire URL
	return generateNumericID(url)
}

// generateNumericID generates a numeric ID from a string (avoids circular dependency)
func generateNumericID(input string) string {
	// Simple hash-based ID generation to avoid circular dependency
	hash := int64(0)
	for _, char := range input {
		hash = hash*31 + int64(char)
	}

	// Ensure positive and within reasonable range
	if hash < 0 {
		hash = -hash
	}

	// Convert to string and ensure it's a valid numeric ID
	return fmt.Sprintf("%d", hash)
}

// Wrapped transformation functions that work with context

// ActorToAccountWithContext provides context-aware actor to account conversion
func ActorToAccountWithContext(ctx context.Context, actor *activitypub.Actor) (models.Account, error) {
	// Extract baseURL from context or use default
	baseURL := "https://example.com" // Fallback
	if url, ok := ctx.Value("baseURL").(string); ok {
		baseURL = url
	}

	account := ActorToAccountBase(actor, baseURL)
	return account, nil
}

// ObjectToStatusWithContext provides context-aware object to status conversion
func ObjectToStatusWithContext(ctx context.Context, obj map[string]interface{}) (models.Status, error) {
	// Extract baseURL and actor from context
	baseURL := "https://example.com" // Fallback
	if url, ok := ctx.Value("baseURL").(string); ok {
		baseURL = url
	}

	var actor *activitypub.Actor
	if a, ok := ctx.Value("actor").(*activitypub.Actor); ok {
		actor = a
	}

	status := ObjectToStatusBase(obj, actor, baseURL)
	return status, nil
}

// ObjectToStatusWithContextAndCounts provides full context-aware object to status conversion with counts and user state
func ObjectToStatusWithContextAndCounts(_ context.Context, obj any, actor *activitypub.Actor, likeCount, reblogCount int, favorited, reblogged, bookmarked bool, baseURL string) models.Status {
	if obj == nil || actor == nil {
		return models.Status{}
	}

	// Convert object to map for unified processing
	objMap := convertObjectToMap(obj)
	status := ObjectToStatusBase(objMap, actor, baseURL)

	// Apply the counts and user state
	status.FavouritesCount = likeCount
	status.ReblogsCount = reblogCount
	status.Favourited = favorited
	status.Reblogged = reblogged
	status.Bookmarked = bookmarked

	return status
}

// NotesToStatusAny provides flexible note-to-status conversion for any note type
func NotesToStatusAny(note any, baseURL string) models.Status {
	if note == nil {
		return models.Status{}
	}

	// Convert note to map for unified processing
	noteMap := convertNoteToMap(note)

	// Extract actor info from the note if available
	var actor *activitypub.Actor
	if authorID, ok := noteMap["authorID"].(string); ok && authorID != "" {
		// Create minimal actor from authorID for transformation
		actor = &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID: authorID,
			},
			PreferredUsername: ExtractUsernameFromActorID(authorID),
		}
	}

	return ObjectToStatusBase(noteMap, actor, baseURL)
}

// ExtractUsernameFromActorID extracts username from an actor ID URL
func ExtractUsernameFromActorID(actorID string) string {
	if err := common.ValidateRequiredParam("actorID", actorID); err != nil {
		return ""
	}

	parts := strings.Split(actorID, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// convertNoteToMap converts various note types to map[string]interface{} for transformation framework
func convertNoteToMap(note any) map[string]interface{} {
	// Handle common note types that might be used in lift API
	switch n := note.(type) {
	case map[string]interface{}:
		// Already in the right format
		return n

	default:
		// For other types, try JSON marshaling/unmarshaling
		if bytes, err := json.Marshal(note); err == nil {
			var result map[string]interface{}
			if err := json.Unmarshal(bytes, &result); err == nil {
				return result
			}
		}

		// Return minimal map as fallback
		return map[string]interface{}{
			"id":      "unknown",
			"content": "",
		}
	}
}

// convertObjectToMap converts various object types to map[string]interface{} for transformation framework
func convertObjectToMap(obj any) map[string]interface{} {
	switch o := obj.(type) {
	case *activitypub.Note:
		objMap := map[string]interface{}{
			"id":        o.ID,
			"content":   o.Content,
			"summary":   o.Summary,
			"sensitive": o.Sensitive,
		}
		if o.Published != nil {
			objMap["published"] = o.Published.Format("2006-01-02T15:04:05.000Z")
		}
		if o.InReplyTo != "" {
			objMap["inReplyTo"] = o.InReplyTo
		}
		if o.AttributedTo != "" {
			objMap["attributedTo"] = o.AttributedTo
		}
		return objMap

	case map[string]interface{}:
		// Already in the right format
		return o

	default:
		// For other types, try JSON marshaling/unmarshaling
		if bytes, err := json.Marshal(obj); err == nil {
			var result map[string]interface{}
			if err := json.Unmarshal(bytes, &result); err == nil {
				return result
			}
		}

		// Return minimal map as fallback
		return map[string]interface{}{
			"id":      "unknown",
			"content": "",
		}
	}
}
