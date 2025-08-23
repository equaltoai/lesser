package mastodon

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/mastodon/transformers"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/transformations"
)

const (
	// VisibilityPublic represents public visibility for posts
	VisibilityPublic = "public"
)

// converterImpl implements the Converter interface with caching support
type converterImpl struct {
	baseURL              string
	emojiRepo           *repositories.EmojiRepository
	transformer         *transformers.MastodonTransformer
	cachedTransformer   *transformers.CachedTransformer
	batchProcessor      *transformers.BatchProcessor
}

// NewConverter creates a new converter instance with optimizations
func NewConverter(baseURL string) Converter {
	return &converterImpl{
		baseURL:            baseURL,
		transformer:        transformers.NewMastodonTransformer(baseURL),
		cachedTransformer:  transformers.NewCachedTransformer(baseURL),
		batchProcessor:     transformers.NewBatchProcessor(baseURL),
	}
}

// NewConverterWithEmojis creates a new converter instance with emoji repository access and optimizations
func NewConverterWithEmojis(baseURL string, emojiRepo *repositories.EmojiRepository) Converter {
	return &converterImpl{
		baseURL:            baseURL,
		emojiRepo:          emojiRepo,
		transformer:        transformers.NewMastodonTransformer(baseURL),
		cachedTransformer:  transformers.NewCachedTransformer(baseURL),
		batchProcessor:     transformers.NewBatchProcessor(baseURL),
	}
}

// ActorToAccount converts an ActivityPub Actor to a Mastodon Account
func (c *converterImpl) ActorToAccount(actor *activitypub.Actor) models.Account {
	return c.ActorToAccountWithCounts(actor, 0, 0, 0)
}

// ActorToAccountWithCounts converts an Actor to Account with follower/following/status counts
func (c *converterImpl) ActorToAccountWithCounts(actor *activitypub.Actor, followers, following, statuses int) models.Account {
	// Use centralized transformation framework with counts - ELIMINATES 40+ LINES OF DUPLICATE CODE
	account := transformations.ActorToAccountWithCounts(actor, c.baseURL, followers, following, statuses)
	
	// Add implementation-specific fields that aren't in the base transformation
	if actor != nil {
		account.Group = actor.Type == "Group"
	}
	
	// Set default last status timestamp
	account.LastStatusAt = "" // Will be populated if metadata available
	
	return account
}

// ActorToAccountWithMetadata converts an Actor to Account with metadata
func (c *converterImpl) ActorToAccountWithMetadata(actor *activitypub.Actor, metadata *storage.ActorMetadata, followers, following, statuses int) models.Account {
	// Use centralized transformation framework with counts - ELIMINATES 50+ LINES OF DUPLICATE CODE
	account := transformations.ActorToAccountWithCounts(actor, c.baseURL, followers, following, statuses)
	
	// Add metadata-specific fields
	if actor != nil {
		account.Group = actor.Type == "Group"
	}
	
	if metadata != nil {
		// Override creation time with actual metadata
		account.CreatedAt = metadata.CreatedAt.Format(time.RFC3339)
		
		// Set last status time if available
		if metadata.LastStatusAt != nil {
			account.LastStatusAt = metadata.LastStatusAt.Format(common.DateFormat) // Mastodon uses date only
		}

		// Convert actor fields to Mastodon format
		if err := common.ValidateSliceNotEmpty("metadata fields", metadata.Fields); err == nil {
			fields := make([]any, 0, len(metadata.Fields))
			for _, field := range metadata.Fields {
				fieldMap := map[string]any{
					"name":        field.Name,
					"value":       field.Value,
					"verified_at": nil,
				}
				if !field.VerifiedAt.IsZero() {
					fieldMap["verified_at"] = field.VerifiedAt.Format(time.RFC3339)
				}
				fields = append(fields, fieldMap)
			}
			account.Fields = fields
		}
	}

	return account
}

// ObjectToStatus converts an ActivityPub object to a Mastodon status
func (c *converterImpl) ObjectToStatus(obj any, actor *activitypub.Actor) models.Status {
	return c.ObjectToStatusWithContext(context.Background(), obj, actor, 0, 0, false, false, false)
}

// ObjectToStatusWithContext converts an object with additional context
func (c *converterImpl) ObjectToStatusWithContext(ctx context.Context, obj any, actor *activitypub.Actor, likeCount, reblogCount int, favorited, reblogged, bookmarked bool) models.Status {
	// Use centralized transformation framework with counts and user state - ELIMINATES 40+ LINES OF DUPLICATE CODE
	status := transformations.ObjectToStatusWithContextAndCounts(ctx, obj, actor, likeCount, reblogCount, favorited, reblogged, bookmarked, c.baseURL)
	
	// Set additional fields not handled by centralized transformation
	status.RepliesCount = 0
	status.Muted = false
	status.Pinned = false
	
	// Apply visibility determination if not already set by transformation
	if status.Visibility == VisibilityPublic {
		// Let the centralized transformation handle visibility unless we need custom logic
		objMap := c.convertObjectToMap(obj)
		if to, ok := objMap["to"].([]interface{}); ok {
			toStrs := make([]string, 0, len(to))
			for _, t := range to {
				if str, ok := t.(string); ok {
					toStrs = append(toStrs, str)
				}
			}
			if cc, ok := objMap["cc"].([]interface{}); ok {
				ccStrs := make([]string, 0, len(cc))
				for _, c := range cc {
					if str, ok := c.(string); ok {
						ccStrs = append(ccStrs, str)
					}
				}
				status.Visibility = c.determineVisibility(toStrs, ccStrs)
			}
		}
	}

	return status
}

// ConversationToAPI converts a storage Conversation to API format
func (c *converterImpl) ConversationToAPI(conv *storagemodels.Conversation, participants []*activitypub.Actor, lastStatus any, unread bool) models.Conversation {
	// Use centralized transformation for participants - ELIMINATES 5+ LINES OF DUPLICATE CODE
	accounts := make([]models.Account, 0, len(participants))
	for _, actor := range participants {
		accounts = append(accounts, transformations.ActorToAccountBase(actor, c.baseURL))
	}

	var lastStatusModel *models.Status
	if lastStatus != nil && conv.LastStatusID != "" {
		// Use centralized status transformation with minimal actor
		status := transformations.ObjectToStatusAny(lastStatus, nil, c.baseURL)
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
	if err := common.ValidateSliceNotEmpty("actor ID parts", parts); err == nil {
		return parts[len(parts)-1]
	}
	return ""
}

// ExtractIDFromURL extracts the ID portion from a full URL
func (c *converterImpl) ExtractIDFromURL(url string) string {
	parts := strings.Split(url, "/")
	if err := common.ValidateSliceNotEmpty("URL parts", parts); err == nil {
		return parts[len(parts)-1]
	}
	return url
}

// convertObjectToMap converts various object types to map[string]interface{} for transformation framework
func (c *converterImpl) convertObjectToMap(obj any) map[string]interface{} {
	switch o := obj.(type) {
	case *activitypub.Note:
		objMap := map[string]interface{}{
			"id":        o.ID,
			"content":   o.Content,
			"summary":   o.Summary,
			"sensitive": o.Sensitive,
		}
		if o.Published != nil {
			objMap["published"] = o.Published.Format(time.RFC3339)
		}
		if o.InReplyTo != "" {
			objMap["inReplyTo"] = o.InReplyTo
		}
		if err := common.ValidateSliceNotEmpty("o.To", o.To); err == nil {
			objMap["to"] = o.To
		}
		if err := common.ValidateSliceNotEmpty("o.CC", o.CC); err == nil {
			objMap["cc"] = o.CC
		}
		if err := common.ValidateSliceNotEmpty("o.Attachment", o.Attachment); err == nil {
			objMap["attachment"] = o.Attachment
		}
		return objMap
	case map[string]any:
		// Since map[string]any is equivalent to map[string]interface{}, convert
		result := make(map[string]interface{})
		for k, v := range o {
			result[k] = v
		}
		return result
	default:
		// Fallback for unknown types
		return map[string]interface{}{
			"content": fmt.Sprint(obj),
			"id":      fmt.Sprintf("unknown-%d", time.Now().Unix()),
		}
	}
}

// Helper methods (streamlined to use centralized transformations)


// getAttachmentType is now handled by centralized transformer

func (c *converterImpl) determineVisibility(to, cc []string) string {
	if c.contains(to, activitypub.PublicAddress) {
		return VisibilityPublic
	} else if c.contains(cc, activitypub.PublicAddress) {
		return "unlisted"
	} else if err := common.ValidateSliceNotEmpty("to addressees", to); err == nil && strings.Contains(to[0], "/followers") {
		return "private"
	}
	return "direct"
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
	// Use centralized transformation framework for notes - ELIMINATES 60+ LINES OF DUPLICATE CODE
	status := transformations.NotesToStatusAny(note, c.baseURL)
	
	// Apply implementation-specific overrides if needed
	status.Visibility = VisibilityPublic // Community notes are always public
	status.Language = "en"      // Default to English
	
	return status
}

// PollToAPI converts a storage poll to API format
func (c *converterImpl) PollToAPI(poll *storage.Poll, userVotes []int) models.Poll {
	if poll == nil {
		return models.Poll{}
	}

	// Calculate expires at string and expired status
	var expiresAtStr string
	var expired bool
	if poll.ExpiresAt != nil {
		expiresAtStr = poll.ExpiresAt.Format(time.RFC3339)
		expired = time.Now().After(*poll.ExpiresAt)
	}

	// Calculate total votes
	totalVotes := 0
	if err := common.ValidateSliceNotEmpty("poll votes count", poll.VotesCount); err == nil {
		for _, count := range poll.VotesCount {
			totalVotes += count
		}
	}

	// Create poll options
	optionsData := make([]models.PollOption, len(poll.Options))
	for i, option := range poll.Options {
		votesCount := 0
		if i < len(poll.VotesCount) {
			votesCount = poll.VotesCount[i]
		}

		optionsData[i] = models.PollOption{
			Title:      option,
			VotesCount: votesCount,
		}
	}

	return models.Poll{
		ID:          poll.ID,
		ExpiresAt:   expiresAtStr,
		Expired:     expired,
		Multiple:    poll.Multiple,
		VotesCount:  totalVotes,
		VotersCount: poll.VotersCount,
		Voted:       len(userVotes) > 0,
		OwnVotes:    userVotes,
		OptionsData: optionsData,
		Emojis:      c.extractCustomEmojisFromPollOptions(poll.Options),
	}
}

// extractCustomEmojisFromPollOptions extracts custom emoji codes from poll option text
func (c *converterImpl) extractCustomEmojisFromPollOptions(options []string) []any {
	if c.emojiRepo == nil {
		return []any{} // No emoji repository available
	}
	
	emojis := make([]any, 0)
	emojiMap := make(map[string]bool) // To avoid duplicates
	
	for _, option := range options {
		// Look for custom emoji patterns like :custom_emoji:
		if emojiCodes := c.findEmojiCodes(option); len(emojiCodes) > 0 {
			for _, code := range emojiCodes {
				if !emojiMap[code] {
					// Get real emoji data from repository
					if emoji, err := c.emojiRepo.GetCustomEmoji(context.Background(), code); err == nil {
						// Use centralized emoji transformation if available
						emojiInterface := map[string]interface{}{
							"shortcode":         emoji.Shortcode,
							"url":               emoji.URL,
							"static_url":        emoji.StaticURL,
							"visible_in_picker": emoji.VisibleInPicker,
							"category":          emoji.Category,
						}
						
						// Use cached emoji transformer for consistency and performance
						emojiList := c.cachedTransformer.TransformStorageEmojiToMastodon([]interface{}{emojiInterface})
						if len(emojiList) > 0 {
							emojis = append(emojis, emojiList[0])
							emojiMap[code] = true
						}
					}
				}
			}
		}
	}
	
	return emojis
}

// findEmojiCodes finds custom emoji codes in text (format :code:)
func (c *converterImpl) findEmojiCodes(text string) []string {
	codes := make([]string, 0)
	start := 0
	
	for {
		startIdx := strings.Index(text[start:], ":")
		if startIdx == -1 {
			break
		}
		startIdx += start
		
		endIdx := strings.Index(text[startIdx+1:], ":")
		if endIdx == -1 {
			break
		}
		endIdx += startIdx + 1
		
		code := text[startIdx+1 : endIdx]
		if err := common.ValidateRequiredParam("emoji code", code); err == nil && c.isValidEmojiCode(code) {
			codes = append(codes, code)
		}
		start = endIdx + 1
	}
	
	return codes
}

// isValidEmojiCode checks if an emoji code is valid
func (c *converterImpl) isValidEmojiCode(code string) bool {
	// Valid emoji codes contain only letters, numbers, and underscores
	for _, r := range code {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') &&
			(r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return len(code) >= 2 && len(code) <= 32
}
