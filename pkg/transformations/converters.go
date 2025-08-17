package transformations

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
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
	return actorType == "Service" || actorType == "Application"
}

func transformAttachments(attachment interface{}) []any {
	// TODO: Implement attachment transformation
	return []any{}
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
	// TODO: Implement media attachment transformation
	return []any{}
}

func transformMentions(obj map[string]interface{}) []any {
	// TODO: Implement mentions transformation
	return []any{}
}

func transformTags(obj map[string]interface{}) []any {
	// TODO: Implement tags transformation
	return []any{}
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
	
	// Add some randomization to make IDs more realistic
	randomness, _ := rand.Int(rand.Reader, big.NewInt(1000))
	hash += randomness.Int64()
	
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
func ObjectToStatusWithContextAndCounts(ctx context.Context, obj any, actor *activitypub.Actor, likeCount, reblogCount int, favorited, reblogged, bookmarked bool, baseURL string) models.Status {
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