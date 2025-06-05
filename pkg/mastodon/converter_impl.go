package mastodon

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/storage"
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
		ID:             actor.PreferredUsername,
		Username:       actor.PreferredUsername,
		Acct:           actor.PreferredUsername,
		DisplayName:    actor.Name,
		URL:            actor.URL,
		Note:           actor.Summary,
		Locked:         actor.ManuallyApprovesFollowers,
		Bot:            actor.Type == "Service",
		Group:          actor.Type == "Group",
		Discoverable:   actor.Discoverable,
		CreatedAt:      time.Now().Format(time.RFC3339), // TODO: Store actor creation time
		FollowersCount: followers,
		FollowingCount: following,
		StatusesCount:  statuses,
		LastStatusAt:   "", // TODO: Track last status time
		Emojis:         []interface{}{},
		Fields:         []interface{}{}, // TODO: Add support for actor fields when available
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
func (c *converterImpl) ObjectToStatus(obj interface{}, actor *activitypub.Actor) models.Status {
	return c.ObjectToStatusWithContext(context.Background(), obj, actor, 0, 0, false, false, false)
}

// ObjectToStatusWithContext converts an object with additional context
func (c *converterImpl) ObjectToStatusWithContext(ctx context.Context, obj interface{}, actor *activitypub.Actor, likeCount, reblogCount int, favorited, reblogged, bookmarked bool) models.Status {
	status := models.Status{
		MediaAttachments: []interface{}{},
		Mentions:         []interface{}{},
		Tags:             []interface{}{},
		Emojis:           []interface{}{},
		Visibility:       "public", // Default
		Language:         "en",     // Default
		FavouritesCount:  likeCount,
		ReblogsCount:     reblogCount,
		Favourited:       favorited,
		Reblogged:        reblogged,
		Bookmarked:       bookmarked,
	}

	// Handle different object types
	switch o := obj.(type) {
	case *activitypub.Note:
		c.populateStatusFromNote(&status, o)
	case map[string]interface{}:
		c.populateStatusFromMap(&status, o)
	}

	// Set account information if actor is provided
	if actor != nil {
		status.Account = c.ActorToAccount(actor)
	}

	return status
}

// ConversationToAPI converts a storage Conversation to API format
func (c *converterImpl) ConversationToAPI(conv *storage.Conversation, participants []*activitypub.Actor, lastStatus interface{}, unread bool) models.Conversation {
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

	// Determine visibility
	status.Visibility = c.determineVisibility(note.To, note.CC)
}

func (c *converterImpl) populateStatusFromMap(status *models.Status, obj map[string]interface{}) {
	if id, ok := obj["id"].(string); ok {
		status.ID = c.ExtractIDFromURL(id)
		status.URI = id
		status.URL = id
	}
	if url, ok := obj["url"].(string); ok {
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
	if published, ok := obj["published"].(string); ok {
		status.CreatedAt = published
	}
	if inReplyTo, ok := obj["inReplyTo"].(string); ok && inReplyTo != "" {
		inReplyToID := c.ExtractIDFromURL(inReplyTo)
		status.InReplyToID = &inReplyToID
	}

	// Process attachments from map
	if attachments, ok := obj["attachment"].([]interface{}); ok && len(attachments) > 0 {
		status.MediaAttachments = c.processAttachmentsFromMap(attachments)
	}

	// Determine visibility from to/cc fields
	var to, cc []string
	if toField, ok := obj["to"].([]interface{}); ok {
		for _, t := range toField {
			if str, ok := t.(string); ok {
				to = append(to, str)
			}
		}
	}
	if ccField, ok := obj["cc"].([]interface{}); ok {
		for _, c := range ccField {
			if str, ok := c.(string); ok {
				cc = append(cc, str)
			}
		}
	}
	status.Visibility = c.determineVisibility(to, cc)
}

func (c *converterImpl) processAttachments(attachments []activitypub.Attachment) []interface{} {
	result := make([]interface{}, 0, len(attachments))
	for _, att := range attachments {
		attachment := map[string]interface{}{
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

func (c *converterImpl) processAttachmentsFromMap(attachments []interface{}) []interface{} {
	result := make([]interface{}, 0, len(attachments))
	for _, att := range attachments {
		if attMap, ok := att.(map[string]interface{}); ok {
			attachment := map[string]interface{}{
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

func (c *converterImpl) getStringFromMap(m map[string]interface{}, key, defaultValue string) string {
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
