package mastodon

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/notes"
	"github.com/equaltoai/lesser/pkg/storage"
)

// converterImpl implements the Converter interface
type converterImpl struct {
	baseURL string
}

// NewConverter creates a new converter instance
func NewConverter(baseURL string) Converter {
	return &converterImpl{baseURL: baseURL}
}

// ActorToAccount converts an ActivityPub Actor to a Mastodon Account
func (c *converterImpl) ActorToAccount(actor *activitypub.Actor) models.Account {
	return c.ActorToAccountWithCounts(actor, 0, 0, 0)
}

// ActorToAccountWithCounts converts an Actor to Account with follower/following/status counts
func (c *converterImpl) ActorToAccountWithCounts(actor *activitypub.Actor, followers, following, statuses int) models.Account {
	account := models.Account{
		ID:             GenerateNumericID(actor.PreferredUsername), // Generate stable numeric ID
		Username:       actor.PreferredUsername,
		Acct:           actor.PreferredUsername,
		DisplayName:    actor.Name,
		URL:            actor.URL,
		Note:           actor.Summary,
		Locked:         actor.ManuallyApprovesFollowers,
		Bot:            actor.Type == "Service",
		Group:          actor.Type == "Group",
		Discoverable:   actor.Discoverable,
		CreatedAt:      time.Now().Format(time.RFC3339), // Default creation time
		FollowersCount: followers,
		FollowingCount: following,
		StatusesCount:  statuses,
		LastStatusAt:   "", // Will be populated if metadata available
		Emojis:         []any{},
		Fields:         []any{}, // Use ActorToAccountWithMetadata for field support
	}

	// Set avatar
	if actor.Icon != nil {
		account.Avatar = actor.Icon.URL
		account.AvatarStatic = actor.Icon.URL
	} else {
		// Default avatar
		account.Avatar = fmt.Sprintf("%s/avatars/default.png", c.baseURL)
		account.AvatarStatic = account.Avatar
	}

	// Set header
	if actor.Image != nil {
		account.Header = actor.Image.URL
		account.HeaderStatic = actor.Image.URL
	} else {
		account.Header = ""
		account.HeaderStatic = ""
	}

	return account
}

// ActorToAccountWithMetadata converts an Actor to Account with metadata
func (c *converterImpl) ActorToAccountWithMetadata(actor *activitypub.Actor, metadata *storage.ActorMetadata, followers, following, statuses int) models.Account {
	account := models.Account{
		ID:             GenerateNumericID(actor.PreferredUsername), // Generate stable numeric ID
		Username:       actor.PreferredUsername,
		Acct:           actor.PreferredUsername,
		DisplayName:    actor.Name,
		URL:            actor.URL,
		Note:           actor.Summary,
		Locked:         actor.ManuallyApprovesFollowers,
		Bot:            actor.Type == "Service",
		Group:          actor.Type == "Group",
		Discoverable:   actor.Discoverable,
		CreatedAt:      metadata.CreatedAt.Format(time.RFC3339), // Use actual creation time
		FollowersCount: followers,
		FollowingCount: following,
		StatusesCount:  statuses,
		LastStatusAt:   "", // Default empty
		Emojis:         []any{},
		Fields:         []any{}, // Default empty
	}

	// Set last status time if available
	if metadata.LastStatusAt != nil {
		account.LastStatusAt = metadata.LastStatusAt.Format("2006-01-02") // Mastodon uses date only
	}

	// Convert actor fields to Mastodon format
	if len(metadata.Fields) > 0 {
		fields := make([]any, 0, len(metadata.Fields))
		for _, field := range metadata.Fields {
			fieldMap := map[string]any{
				"name":        field.Name,
				"value":       field.Value,
				"verified_at": nil,
			}
			if field.VerifiedAt != nil {
				fieldMap["verified_at"] = field.VerifiedAt.Format(time.RFC3339)
			}
			fields = append(fields, fieldMap)
		}
		account.Fields = fields
	}

	// Set avatar
	if actor.Icon != nil {
		account.Avatar = actor.Icon.URL
		account.AvatarStatic = actor.Icon.URL
	} else {
		// Default avatar
		account.Avatar = fmt.Sprintf("%s/avatars/default.png", c.baseURL)
		account.AvatarStatic = account.Avatar
	}

	// Set header
	if actor.Image != nil {
		account.Header = actor.Image.URL
		account.HeaderStatic = actor.Image.URL
	} else {
		account.Header = ""
		account.HeaderStatic = ""
	}

	return account
}

// ObjectToStatus converts an ActivityPub object to a Mastodon status
func (c *converterImpl) ObjectToStatus(obj any, actor *activitypub.Actor) models.Status {
	return c.ObjectToStatusWithContext(context.Background(), obj, actor, 0, 0, false, false, false)
}

// ObjectToStatusWithContext converts an object with additional context
func (c *converterImpl) ObjectToStatusWithContext(ctx context.Context, obj any, actor *activitypub.Actor, likeCount, reblogCount int, favorited, reblogged, bookmarked bool) models.Status {
	status := models.Status{
		MediaAttachments: []any{},
		Mentions:         []any{},
		Tags:             []any{},
		Emojis:           []any{},
		Visibility:       "public", // Default
		Language:         "en",     // Default
		FavouritesCount:  likeCount,
		ReblogsCount:     reblogCount,
		Favourited:       favorited,
		Reblogged:        reblogged,
		Bookmarked:       bookmarked,
		RepliesCount:     0,
		Muted:            false,
		Pinned:           false,
	}

	// Handle different object types
	switch o := obj.(type) {
	case *activitypub.Note:
		c.populateStatusFromNote(&status, o)
	case map[string]any:
		c.populateStatusFromMap(&status, o)
	default:
		// If we get an unexpected type, try to handle it gracefully
		// Set some default values
		status.ID = fmt.Sprintf("unknown-%d", time.Now().Unix())
		status.CreatedAt = time.Now().Format("2006-01-02T15:04:05.000Z")
		status.Content = ""
		status.URI = ""
		status.URL = ""
	}

	// Set account information if actor is provided
	if actor != nil {
		status.Account = c.ActorToAccount(actor)
	}

	return status
}

// ConversationToAPI converts a storage Conversation to API format
func (c *converterImpl) ConversationToAPI(conv *storage.Conversation, participants []*activitypub.Actor, lastStatus any, unread bool) models.Conversation {
	accounts := make([]models.Account, 0, len(participants))
	for _, actor := range participants {
		accounts = append(accounts, c.ActorToAccount(actor))
	}

	var lastStatusModel *models.Status
	if lastStatus != nil && conv.LastStatusID != "" {
		// Convert last status (without actor to avoid recursion)
		status := c.ObjectToStatus(lastStatus, nil)
		status.ID = conv.LastStatusID
		status.CreatedAt = conv.UpdatedAt.Format("2006-01-02T15:04:05.000Z")
		lastStatusModel = &status
	}

	return models.Conversation{
		ID:         conv.ID,
		Unread:     unread,
		Accounts:   accounts,
		LastStatus: lastStatusModel,
	}
}

// ExtractUsernameFromActorID extracts the username from an actor ID URL
func (c *converterImpl) ExtractUsernameFromActorID(actorID string) string {
	parts := strings.Split(actorID, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// ExtractIDFromURL extracts the ID portion from a full URL
func (c *converterImpl) ExtractIDFromURL(url string) string {
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return url
}

// Helper methods

func (c *converterImpl) populateStatusFromNote(status *models.Status, note *activitypub.Note) {
	status.ID = c.ExtractIDFromURL(note.ID)
	status.URI = note.ID
	status.URL = note.ID // Note doesn't have URL field, use ID
	status.Content = note.Content
	status.SpoilerText = note.Summary
	status.Sensitive = note.Sensitive
	if note.Published != nil {
		status.CreatedAt = note.Published.Format("2006-01-02T15:04:05.000Z")
	}
	if note.InReplyTo != "" {
		inReplyToID := c.ExtractIDFromURL(note.InReplyTo)
		status.InReplyToID = &inReplyToID
	}

	// Process attachments
	if len(note.Attachment) > 0 {
		status.MediaAttachments = c.processAttachments(note.Attachment)
	}

	// Use explicit visibility if available, otherwise determine from To/CC
	if note.Visibility != "" {
		status.Visibility = note.Visibility
	} else {
		status.Visibility = c.determineVisibility(note.To, note.CC)
	}
}

func (c *converterImpl) populateStatusFromMap(status *models.Status, obj map[string]any) {
	if id, ok := obj["id"].(string); ok && id != "" {
		status.ID = c.ExtractIDFromURL(id)
		status.URI = id
		status.URL = id
	}
	if url, ok := obj["url"].(string); ok && url != "" {
		status.URL = url
	}
	if content, ok := obj["content"].(string); ok {
		status.Content = content
	}
	if summary, ok := obj["summary"].(string); ok {
		status.SpoilerText = summary
	}
	if sensitive, ok := obj["sensitive"].(bool); ok {
		status.Sensitive = sensitive
	}
	if published, ok := obj["published"].(string); ok && published != "" {
		status.CreatedAt = published
	} else {
		// Fallback to current time if published is missing
		status.CreatedAt = time.Now().Format("2006-01-02T15:04:05.000Z")
	}
	if inReplyTo, ok := obj["inReplyTo"].(string); ok && inReplyTo != "" {
		inReplyToID := c.ExtractIDFromURL(inReplyTo)
		status.InReplyToID = &inReplyToID
	}

	// Process attachments from map
	if attachments, ok := obj["attachment"].([]any); ok && len(attachments) > 0 {
		status.MediaAttachments = c.processAttachmentsFromMap(attachments)
	}

	// Check for explicit visibility field first
	if visibility, ok := obj["visibility"].(string); ok && visibility != "" {
		status.Visibility = visibility
	} else {
		// Fallback to determining visibility from to/cc fields
		var to, cc []string
		if toField, ok := obj["to"].([]any); ok {
			for _, t := range toField {
				if str, ok := t.(string); ok {
					to = append(to, str)
				}
			}
		}
		if ccField, ok := obj["cc"].([]any); ok {
			for _, c := range ccField {
				if str, ok := c.(string); ok {
					cc = append(cc, str)
				}
			}
		}
		status.Visibility = c.determineVisibility(to, cc)
	}
}

func (c *converterImpl) processAttachments(attachments []activitypub.Attachment) []any {
	result := make([]any, 0, len(attachments))
	for _, att := range attachments {
		attachment := map[string]any{
			"id":          c.generateRandomString(8),
			"type":        c.getAttachmentType(att.MediaType),
			"url":         att.URL,
			"preview_url": att.URL,
			"text_url":    att.URL,
			"description": att.Name,
		}
		result = append(result, attachment)
	}
	return result
}

func (c *converterImpl) processAttachmentsFromMap(attachments []any) []any {
	result := make([]any, 0, len(attachments))
	for _, att := range attachments {
		if attMap, ok := att.(map[string]any); ok {
			attachment := map[string]any{
				"id":          c.generateRandomString(8),
				"type":        "image",
				"url":         c.getStringFromMap(attMap, "url", ""),
				"preview_url": c.getStringFromMap(attMap, "url", ""),
				"text_url":    c.getStringFromMap(attMap, "url", ""),
				"description": c.getStringFromMap(attMap, "name", ""),
			}
			if mediaType := c.getStringFromMap(attMap, "mediaType", ""); mediaType != "" {
				attachment["type"] = c.getAttachmentType(mediaType)
			}
			result = append(result, attachment)
		}
	}
	return result
}

func (c *converterImpl) getAttachmentType(mediaType string) string {
	if strings.HasPrefix(mediaType, "video/") {
		return "video"
	} else if strings.HasPrefix(mediaType, "audio/") {
		return "audio"
	}
	return "image"
}

func (c *converterImpl) determineVisibility(to, cc []string) string {
	if c.contains(to, activitypub.PublicAddress) {
		return "public"
	} else if c.contains(cc, activitypub.PublicAddress) {
		return "unlisted"
	} else if len(to) > 0 && strings.Contains(to[0], "/followers") {
		return "private"
	}
	return "direct"
}

func (c *converterImpl) generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

func (c *converterImpl) getStringFromMap(m map[string]any, key, defaultValue string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return defaultValue
}

func (c *converterImpl) contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// NotesToStatus converts a community note to a Mastodon status format
func (c *converterImpl) NotesToStatus(note any) models.Status {
	// Handle different note types
	var content string
	var createdAt time.Time
	var id string
	var authorID string

	// Type switch to handle different note structures
	switch n := note.(type) {
	case *notes.CommunityNote:
		// Handle CommunityNote struct
		content = n.Content
		id = n.ID
		authorID = n.AuthorID
		createdAt = n.CreatedAt
	case notes.CommunityNote:
		// Handle CommunityNote struct (non-pointer)
		content = n.Content
		id = n.ID
		authorID = n.AuthorID
		createdAt = n.CreatedAt
	case map[string]any:
		// Handle map representation (from JSON)
		content, _ = n["content"].(string)
		id, _ = n["id"].(string)
		authorID, _ = n["author_id"].(string)
		if createdAtStr, ok := n["created_at"].(string); ok {
			createdAt, _ = time.Parse(time.RFC3339, createdAtStr)
		} else if createdAtTime, ok := n["created_at"].(time.Time); ok {
			createdAt = createdAtTime
		}
	default:
		// For other types, create a simple representation
		content = fmt.Sprint(note)
		id = "note-" + time.Now().Format("20060102150405")
		createdAt = time.Now()
	}

	// Create a basic status representation for the note
	status := models.Status{
		ID:                 id,
		CreatedAt:          createdAt.Format(time.RFC3339),
		InReplyToID:        nil,
		InReplyToAccountID: nil,
		Sensitive:          false,
		SpoilerText:        "",
		Visibility:         "public",
		Language:           "en", // Default to English
		URI:                fmt.Sprintf("%s/notes/%s", c.baseURL, id),
		URL:                fmt.Sprintf("%s/notes/%s", c.baseURL, id),
		RepliesCount:       0,
		ReblogsCount:       0,
		FavouritesCount:    0,
		Favourited:         false,
		Reblogged:          false,
		Muted:              false,
		Bookmarked:         false,
		Pinned:             false,
		Content:            content,
		Reblog:             nil,
		Application:        nil,
		Account:            models.Account{}, // Will be populated by caller if needed
		MediaAttachments:   []any{},
		Mentions:           []any{},
		Tags:               []any{},
		Emojis:             []any{},
		Card:               nil,
		Poll:               nil,
	}

	// If we have an author ID, set a basic account
	if authorID != "" {
		// Extract username from author ID if it's a full URI
		username := authorID
		if strings.Contains(authorID, "/users/") {
			parts := strings.Split(authorID, "/users/")
			if len(parts) > 1 {
				username = parts[1]
			}
		}

		status.Account = models.Account{
			ID:             authorID,
			Username:       username,
			Acct:           username,
			URL:            authorID,
			DisplayName:    username,
			CreatedAt:      time.Now().Format(time.RFC3339),
			Avatar:         fmt.Sprintf("%s/avatars/default.png", c.baseURL),
			AvatarStatic:   fmt.Sprintf("%s/avatars/default.png", c.baseURL),
			Header:         fmt.Sprintf("%s/headers/default.png", c.baseURL),
			HeaderStatic:   fmt.Sprintf("%s/headers/default.png", c.baseURL),
			Locked:         false,
			Bot:            false,
			Discoverable:   true,
			Group:          false,
			Note:           "",
			FollowersCount: 0,
			FollowingCount: 0,
			StatusesCount:  0,
			LastStatusAt:   "",
			Emojis:         []any{},
			Fields:         []any{},
		}
	}

	return status
}
