package transformations

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// Global transformation registry for ActivityPub transformations
var ActivityPubRegistry *common.TransformationRegistry

// Initialize the global registry
func init() {
	ActivityPubRegistry = common.NewTransformationRegistry(zap.NewNop())
	registerActivityPubTransformers()
}

// registerActivityPubTransformers registers all ActivityPub transformation functions
func registerActivityPubTransformers() {
	// Actor transformations
	ActivityPubRegistry.Register("actor_to_storage", NewActorToStorageTransformer())
	ActivityPubRegistry.Register("storage_to_actor", NewStorageToActorTransformer())
	ActivityPubRegistry.Register("actor_to_mastodon", NewActorToMastodonTransformer())
	
	// Object transformations
	ActivityPubRegistry.Register("object_to_storage", NewObjectToStorageTransformer())
	ActivityPubRegistry.Register("storage_to_object", NewStorageToObjectTransformer())
	ActivityPubRegistry.Register("object_to_mastodon", NewObjectToMastodonTransformer())
	
	// Activity transformations
	ActivityPubRegistry.Register("activity_to_storage", NewActivityToStorageTransformer())
	ActivityPubRegistry.Register("storage_to_activity", NewStorageToActivityTransformer())
	
	// Batch transformers for performance
	ActivityPubRegistry.Register("actors_to_mastodon_batch", NewActorBatchToMastodonTransformer())
	ActivityPubRegistry.Register("objects_to_mastodon_batch", NewObjectBatchToMastodonTransformer())
}

// Actor Transformations

// ActivityPubActorToStorage converts an ActivityPub Actor to a storage Actor model
func ActivityPubActorToStorage(actor *activitypub.Actor) (*storagemodels.Actor, error) {
	if actor == nil {
		return nil, common.ValidationError{Field: "actor", Message: "actor cannot be nil"}
	}

	// Extract username from preferredUsername
	username := actor.PreferredUsername
	if username == "" {
		return nil, common.ValidationError{Field: "preferredUsername", Message: "preferredUsername is required"}
	}

	storageActor := &storagemodels.Actor{
		// Set primary keys according to storage model requirements
		PK:       fmt.Sprintf("ACTOR#%s", username),
		SK:       "PROFILE",
		Username: username,
		Actor:    actor, // Store the entire ActivityPub actor
		NumericID: generateNumericIDFromUsername(username),
	}

	// Set timestamps
	now := time.Now()
	storageActor.CreatedAt = now
	storageActor.UpdatedAt = now
	
	if actor.LastStatusAt != nil {
		storageActor.LastStatusAt = actor.LastStatusAt
	}

	// Convert attachments to profile fields
	storageActor.Fields = convertAttachmentsToActorFields(actor.Attachment)

	return storageActor, nil
}

// StorageActorToActivityPub converts a storage Actor to an ActivityPub Actor
func StorageActorToActivityPub(actor *storagemodels.Actor) (*activitypub.Actor, error) {
	if actor == nil {
		return nil, common.ValidationError{Field: "actor", Message: "actor cannot be nil"}
	}

	// If the Actor field is populated, return it directly
	if actor.Actor != nil {
		return actor.Actor, nil
	}

	// Otherwise, construct from individual fields (fallback)
	activityPubActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    "Person",
		},
		PreferredUsername: actor.Username,
	}

	// Set timestamps
	if !actor.CreatedAt.IsZero() {
		activityPubActor.CreatedAt = &actor.CreatedAt
	}
	if !actor.UpdatedAt.IsZero() {
		activityPubActor.Updated = &actor.UpdatedAt
	}
	if actor.LastStatusAt != nil {
		activityPubActor.LastStatusAt = actor.LastStatusAt
	}

	// Convert profile fields to attachments
	activityPubActor.Attachment = convertActorFieldsToAttachments(actor.Fields)

	return activityPubActor, nil
}

// ActivityPubActorToMastodon converts an ActivityPub Actor to a Mastodon Account
func ActivityPubActorToMastodon(actor *activitypub.Actor, baseURL string) (*models.Account, error) {
	if actor == nil {
		return nil, common.ValidationError{Field: "actor", Message: "actor cannot be nil"}
	}

	account := &models.Account{
		ID:              generateNumericIDFromUsername(actor.PreferredUsername),
		Username:        actor.PreferredUsername,
		Acct:            actor.PreferredUsername,
		DisplayName:     actor.Name,
		Note:            actor.Summary,
		URL:             actor.URL,
		Locked:          actor.ManuallyApprovesFollowers,
		Bot:             isBot(actor.Type),
		Discoverable:    actor.Discoverable,
		FollowersCount:  0, // To be populated by caller
		FollowingCount:  0, // To be populated by caller
		StatusesCount:   0, // To be populated by caller
		Fields:          convertAttachmentsToMastodonFields(actor.Attachment),
		Emojis:          []any{}, // To be populated by emoji service
	}

	// Handle timestamps
	if actor.CreatedAt != nil {
		account.CreatedAt = actor.CreatedAt.Format(time.RFC3339)
	} else if actor.Published != nil {
		account.CreatedAt = actor.Published.Format(time.RFC3339)
	}

	if actor.LastStatusAt != nil {
		account.LastStatusAt = actor.LastStatusAt.Format("2006-01-02")
	}

	// Handle avatar/header with fallbacks
	account.Avatar = getAvatarURL(actor.Icon, baseURL)
	account.AvatarStatic = account.Avatar
	account.Header = getHeaderURL(actor.Image, baseURL)
	account.HeaderStatic = account.Header

	return account, nil
}

// Object Transformations

// ActivityPubObjectToStorage converts an ActivityPub Note to a storage Object model
func ActivityPubObjectToStorage(obj *activitypub.Note) (*storagemodels.Object, error) {
	if obj == nil {
		return nil, common.ValidationError{Field: "object", Message: "object cannot be nil"}
	}

	if obj.ID == "" {
		return nil, common.ValidationError{Field: "id", Message: "object ID is required"}
	}

	storageObj := &storagemodels.Object{
		// Set primary keys according to storage model requirements
		PK:           fmt.Sprintf("object#%s", obj.ID),
		SK:           fmt.Sprintf("object#%s", obj.ID),
		ID:           obj.ID,
		Type:         obj.Type,
		Content:      obj.Content,
		Summary:      obj.Summary,
		AttributedTo: obj.AttributedTo,
		Sensitive:    obj.Sensitive,
	}

	// Handle timestamps
	if obj.Published != nil {
		storageObj.Published = *obj.Published
	} else {
		storageObj.Published = time.Now()
	}
	
	if obj.Updated != nil {
		storageObj.Updated = *obj.Updated
	} else {
		storageObj.Updated = storageObj.Published
	}

	// Handle addressing
	storageObj.To = obj.To
	storageObj.CC = obj.CC
	storageObj.BTo = obj.BTo
	storageObj.BCC = obj.BCC

	// Handle reply relationship
	if obj.InReplyTo != "" {
		storageObj.InReplyTo = &obj.InReplyTo
	}

	// Handle attachments and tags as JSON
	if len(obj.Attachment) > 0 {
		attachmentJSON, err := convertAttachmentsToJSON(obj.Attachment)
		if err == nil {
			storageObj.AttachmentJSON = attachmentJSON
		}
	}

	if len(obj.Tag) > 0 {
		tagJSON, err := convertTagsToJSON(obj.Tag)
		if err == nil {
			storageObj.TagJSON = tagJSON
		}
	}

	return storageObj, nil
}

// StorageObjectToActivityPub converts a storage Object to an ActivityPub Note
func StorageObjectToActivityPub(obj *storagemodels.Object) (*activitypub.Note, error) {
	if obj == nil {
		return nil, common.ValidationError{Field: "object", Message: "object cannot be nil"}
	}

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        obj.ID,
			Type:      obj.Type,
			To:        obj.To,
			CC:        obj.CC,
			BTo:       obj.BTo,
			BCC:       obj.BCC,
			Summary:   obj.Summary,
			Sensitive: obj.Sensitive,
		},
		Content:      obj.Content,
		AttributedTo: obj.AttributedTo,
	}

	// Handle timestamps
	if !obj.Published.IsZero() {
		note.Published = &obj.Published
	}
	if !obj.Updated.IsZero() {
		note.Updated = &obj.Updated
	}

	// Handle reply relationship
	if obj.InReplyTo != nil && *obj.InReplyTo != "" {
		note.InReplyTo = *obj.InReplyTo
	}

	// Parse JSON fields back to objects
	if obj.AttachmentJSON != "" {
		attachments, err := parseAttachmentsFromJSON(obj.AttachmentJSON)
		if err == nil {
			note.Attachment = attachments
		}
	}

	if obj.TagJSON != "" {
		tags, err := parseTagsFromJSON(obj.TagJSON)
		if err == nil {
			note.Tag = tags
		}
	}

	return note, nil
}

// ActivityPubObjectToMastodon converts an ActivityPub Note to a Mastodon Status
func ActivityPubObjectToMastodon(obj *activitypub.Note, actor *activitypub.Actor, baseURL string) (*models.Status, error) {
	if obj == nil {
		return nil, common.ValidationError{Field: "object", Message: "object cannot be nil"}
	}
	if actor == nil {
		return nil, common.ValidationError{Field: "actor", Message: "actor cannot be nil"}
	}

	// Convert actor to account first
	account, err := ActivityPubActorToMastodon(actor, baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to convert actor: %w", err)
	}

	status := &models.Status{
		ID:                 generateNumericIDFromURL(obj.ID),
		URI:                obj.ID,
		URL:                obj.ID,
		Account:            *account,
		Content:            obj.Content,
		Sensitive:          obj.Sensitive,
		SpoilerText:        obj.Summary,
		Visibility:         determineVisibility(obj.To, obj.CC, baseURL),
		Language:           extractLanguageFromContent(obj.Content),
		ReblogsCount:       0, // To be populated by caller
		FavouritesCount:    0, // To be populated by caller
		RepliesCount:       0, // To be populated by caller
		Reblogged:          false, // To be populated by caller
		Favourited:         false, // To be populated by caller
		Bookmarked:         false, // To be populated by caller
		MediaAttachments:   convertAttachmentsToMastodonMedia(obj.Attachment),
		Mentions:           convertTagsToMastodonMentions(obj.Tag, baseURL),
		Tags:               convertTagsToMastodonTags(obj.Tag, baseURL),
		Emojis:             []any{}, // To be populated by emoji service
	}

	// Handle timestamps
	if obj.Published != nil {
		status.CreatedAt = obj.Published.Format(time.RFC3339)
	}

	// Handle reply information
	if obj.InReplyTo != "" {
		inReplyToID := generateNumericIDFromURL(obj.InReplyTo)
		status.InReplyToID = &inReplyToID
		// InReplyToAccountID would need to be resolved separately
	}

	return status, nil
}

// Activity Transformations

// ActivityPubActivityToStorage converts an ActivityPub Activity to a storage Activity model
func ActivityPubActivityToStorage(activity *activitypub.Activity) (*storagemodels.Activity, error) {
	if activity == nil {
		return nil, common.ValidationError{Field: "activity", Message: "activity cannot be nil"}
	}

	if activity.ID == "" {
		return nil, common.ValidationError{Field: "id", Message: "activity ID is required"}
	}

	// Extract username from actor
	username := extractUsernameFromActorID(activity.Actor)
	if username == "" {
		return nil, common.ValidationError{Field: "actor", Message: "cannot extract username from actor"}
	}

	// Generate timestamp for sort key
	timestamp := time.Now().Unix()

	storageActivity := &storagemodels.Activity{
		// Set primary keys according to storage model requirements
		PK:       fmt.Sprintf("ACTOR#%s", username),
		SK:       fmt.Sprintf("ACTIVITY#%d#%s", timestamp, activity.ID),
		Activity: activity, // Store the entire ActivityPub activity
		CreatedAt: time.Now(),
	}

	return storageActivity, nil
}

// StorageActivityToActivityPub converts a storage Activity to an ActivityPub Activity
func StorageActivityToActivityPub(activity *storagemodels.Activity) (*activitypub.Activity, error) {
	if activity == nil {
		return nil, common.ValidationError{Field: "activity", Message: "activity cannot be nil"}
	}

	// If the Activity field is populated, return it directly
	if activity.Activity != nil {
		return activity.Activity, nil
	}

	// Return error if no activity data is available
	return nil, common.ValidationError{Field: "activity", Message: "no activity data available"}
}

// Transformer Factory Functions

// NewActorToStorageTransformer creates a new Actor to Storage transformer
func NewActorToStorageTransformer() common.Transformer[*activitypub.Actor, *storagemodels.Actor] {
	return common.NewBaseTransformer(
		"actor_to_storage",
		func(ctx context.Context, actor *activitypub.Actor) (*storagemodels.Actor, error) {
			return ActivityPubActorToStorage(actor)
		},
		nil,
	)
}

// NewStorageToActorTransformer creates a new Storage to Actor transformer
func NewStorageToActorTransformer() common.Transformer[*storagemodels.Actor, *activitypub.Actor] {
	return common.NewBaseTransformer(
		"storage_to_actor",
		func(ctx context.Context, actor *storagemodels.Actor) (*activitypub.Actor, error) {
			return StorageActorToActivityPub(actor)
		},
		nil,
	)
}

// NewActorToMastodonTransformer creates a new Actor to Mastodon transformer
func NewActorToMastodonTransformer() common.Transformer[*activitypub.Actor, *models.Account] {
	return common.NewBaseTransformer(
		"actor_to_mastodon",
		func(ctx context.Context, actor *activitypub.Actor) (*models.Account, error) {
			baseURL := "https://example.com" // Default fallback
			if url, ok := ctx.Value("baseURL").(string); ok {
				baseURL = url
			}
			return ActivityPubActorToMastodon(actor, baseURL)
		},
		nil,
	)
}

// NewObjectToStorageTransformer creates a new Object to Storage transformer
func NewObjectToStorageTransformer() common.Transformer[*activitypub.Note, *storagemodels.Object] {
	return common.NewBaseTransformer(
		"object_to_storage",
		func(ctx context.Context, obj *activitypub.Note) (*storagemodels.Object, error) {
			return ActivityPubObjectToStorage(obj)
		},
		nil,
	)
}

// NewStorageToObjectTransformer creates a new Storage to Object transformer
func NewStorageToObjectTransformer() common.Transformer[*storagemodels.Object, *activitypub.Note] {
	return common.NewBaseTransformer(
		"storage_to_object",
		func(ctx context.Context, obj *storagemodels.Object) (*activitypub.Note, error) {
			return StorageObjectToActivityPub(obj)
		},
		nil,
	)
}

// NewObjectToMastodonTransformer creates a new Object to Mastodon transformer
func NewObjectToMastodonTransformer() common.Transformer[*activitypub.Note, *models.Status] {
	return common.NewBaseTransformer(
		"object_to_mastodon",
		func(ctx context.Context, obj *activitypub.Note) (*models.Status, error) {
			// Extract actor and baseURL from context
			actor, ok := ctx.Value("actor").(*activitypub.Actor)
			if !ok {
				return nil, common.ValidationError{Field: "context", Message: "actor not found in context"}
			}
			
			baseURL := "https://example.com" // Default fallback
			if url, ok := ctx.Value("baseURL").(string); ok {
				baseURL = url
			}
			
			return ActivityPubObjectToMastodon(obj, actor, baseURL)
		},
		nil,
	)
}

// NewActivityToStorageTransformer creates a new Activity to Storage transformer
func NewActivityToStorageTransformer() common.Transformer[*activitypub.Activity, *storagemodels.Activity] {
	return common.NewBaseTransformer(
		"activity_to_storage",
		func(ctx context.Context, activity *activitypub.Activity) (*storagemodels.Activity, error) {
			return ActivityPubActivityToStorage(activity)
		},
		nil,
	)
}

// NewStorageToActivityTransformer creates a new Storage to Activity transformer
func NewStorageToActivityTransformer() common.Transformer[*storagemodels.Activity, *activitypub.Activity] {
	return common.NewBaseTransformer(
		"storage_to_activity",
		func(ctx context.Context, activity *storagemodels.Activity) (*activitypub.Activity, error) {
			return StorageActivityToActivityPub(activity)
		},
		nil,
	)
}

// Batch Transformers for Performance

// NewActorBatchToMastodonTransformer creates a batch transformer for actors to Mastodon accounts
func NewActorBatchToMastodonTransformer() common.BatchTransformer[*activitypub.Actor, *models.Account] {
	return common.NewBatchTransformer(
		"actors_to_mastodon_batch",
		func(ctx context.Context, actor *activitypub.Actor) (*models.Account, error) {
			baseURL := "https://example.com" // Default fallback
			if url, ok := ctx.Value("baseURL").(string); ok {
				baseURL = url
			}
			return ActivityPubActorToMastodon(actor, baseURL)
		},
		func(ctx context.Context, actors []*activitypub.Actor) ([]*models.Account, error) {
			baseURL := "https://example.com" // Default fallback
			if url, ok := ctx.Value("baseURL").(string); ok {
				baseURL = url
			}
			
			accounts := make([]*models.Account, 0, len(actors))
			for _, actor := range actors {
				account, err := ActivityPubActorToMastodon(actor, baseURL)
				if err != nil {
					return nil, err
				}
				accounts = append(accounts, account)
			}
			return accounts, nil
		},
		nil,
	)
}

// NewObjectBatchToMastodonTransformer creates a batch transformer for objects to Mastodon statuses
func NewObjectBatchToMastodonTransformer() common.BatchTransformer[*activitypub.Note, *models.Status] {
	return common.NewBatchTransformer(
		"objects_to_mastodon_batch",
		func(ctx context.Context, obj *activitypub.Note) (*models.Status, error) {
			// This would need actor from context
			actor, ok := ctx.Value("actor").(*activitypub.Actor)
			if !ok {
				return nil, common.ValidationError{Field: "context", Message: "actor not found in context"}
			}
			
			baseURL := "https://example.com" // Default fallback
			if url, ok := ctx.Value("baseURL").(string); ok {
				baseURL = url
			}
			
			return ActivityPubObjectToMastodon(obj, actor, baseURL)
		},
		nil, // Single transform function will be used for batch
		nil,
	)
}

// Utility functions for transformations

// convertAttachmentsToActorFields converts ActivityPub attachments to storage actor fields
func convertAttachmentsToActorFields(attachments []activitypub.Attachment) []storagemodels.ActorField {
	if len(attachments) == 0 {
		return nil
	}
	
	fields := make([]storagemodels.ActorField, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.Type == "PropertyValue" {
			fields = append(fields, storagemodels.ActorField{
				Name:  attachment.Name,
				Value: attachment.Value,
			})
		}
	}
	
	return fields
}

// convertActorFieldsToAttachments converts storage actor fields to ActivityPub attachments
func convertActorFieldsToAttachments(fields []storagemodels.ActorField) []activitypub.Attachment {
	if len(fields) == 0 {
		return nil
	}
	
	attachments := make([]activitypub.Attachment, 0, len(fields))
	for _, field := range fields {
		attachments = append(attachments, activitypub.Attachment{
			Type:  "PropertyValue",
			Name:  field.Name,
			Value: field.Value,
		})
	}
	
	return attachments
}

// convertAttachmentsToMastodonFields converts ActivityPub attachments to Mastodon fields
func convertAttachmentsToMastodonFields(attachments []activitypub.Attachment) []any {
	if len(attachments) == 0 {
		return []any{}
	}
	
	fields := make([]any, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.Type == "PropertyValue" {
			fields = append(fields, map[string]any{
				"name":  attachment.Name,
				"value": attachment.Value,
			})
		}
	}
	
	return fields
}

// convertAttachmentsToMastodonMedia converts ActivityPub attachments to Mastodon media
func convertAttachmentsToMastodonMedia(attachments []activitypub.Attachment) []any {
	if len(attachments) == 0 {
		return []any{}
	}
	
	media := make([]any, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.Type != "PropertyValue" { // Skip profile fields
			media = append(media, map[string]any{
				"id":          generateNumericIDFromURL(attachment.URL),
				"type":        attachment.Type,
				"url":         attachment.URL,
				"preview_url": attachment.URL,
				"description": attachment.Name,
			})
		}
	}
	
	return media
}

// convertTagsToMastodonMentions converts ActivityPub tags to Mastodon mentions
func convertTagsToMastodonMentions(tags []activitypub.Tag, baseURL string) []any {
	var mentions []any
	
	for _, tag := range tags {
		if tag.Type == "Mention" {
			mentions = append(mentions, map[string]any{
				"id":       generateNumericIDFromURL(tag.Href),
				"username": strings.TrimPrefix(tag.Name, "@"),
				"url":      tag.Href,
				"acct":     strings.TrimPrefix(tag.Name, "@"),
			})
		}
	}
	
	return mentions
}

// convertTagsToMastodonTags converts ActivityPub tags to Mastodon hashtags
func convertTagsToMastodonTags(tags []activitypub.Tag, baseURL string) []any {
	var hashtagResults []any
	
	for _, tag := range tags {
		if tag.Type == "Hashtag" {
			name := strings.TrimPrefix(tag.Name, "#")
			hashtagResults = append(hashtagResults, map[string]any{
				"name": name,
				"url":  fmt.Sprintf("%s/tags/%s", baseURL, name),
			})
		}
	}
	
	return hashtagResults
}

// Helper utility functions

// generateNumericIDFromUsername generates a numeric ID from username
func generateNumericIDFromUsername(username string) string {
	return generateNumericID(username)
}

// generateNumericIDFromURL generates a numeric ID from URL
func generateNumericIDFromURL(url string) string {
	if url == "" {
		return "0"
	}
	
	// Extract the last segment of the URL for ID generation
	parts := strings.Split(strings.TrimSuffix(url, "/"), "/")
	if len(parts) > 0 {
		return generateNumericID(parts[len(parts)-1])
	}
	
	return generateNumericID(url)
}

// Use existing generateNumericID from converters.go

// extractUsernameFromActorID extracts username from an actor ID URL
func extractUsernameFromActorID(actorID string) string {
	if actorID == "" {
		return ""
	}
	
	// Handle full URLs like "https://example.com/users/alice"
	if strings.Contains(actorID, "/users/") {
		parts := strings.Split(actorID, "/users/")
		if len(parts) == 2 {
			return parts[1]
		}
	}
	
	// Handle other formats
	parts := strings.Split(strings.TrimSuffix(actorID, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	
	return actorID
}

// Use existing isBot from converters.go

// Use existing getAvatarURL from converters.go

// Use existing getHeaderURL from converters.go

// determineVisibility determines Mastodon visibility from ActivityPub addressing
func determineVisibility(to, cc []string, baseURL string) string {
	publicURL := "https://www.w3.org/ns/activitystreams#Public"
	followersURL := baseURL + "/followers"
	
	// Check if public
	for _, recipient := range to {
		if recipient == publicURL {
			return "public"
		}
	}
	
	// Check if unlisted (public in CC)
	for _, recipient := range cc {
		if recipient == publicURL {
			return "unlisted"
		}
	}
	
	// Check if followers-only
	for _, recipient := range to {
		if recipient == followersURL {
			return "private"
		}
	}
	
	// Default to direct if none of the above
	return "direct"
}

// extractLanguageFromContent attempts to detect language from content
func extractLanguageFromContent(content string) string {
	// This is a simplified implementation
	// In practice, you might use a language detection library
	if content == "" {
		return ""
	}
	
	// Default to English for now
	return "en"
}

// JSON conversion utilities

// convertAttachmentsToJSON converts attachments to JSON string
func convertAttachmentsToJSON(attachments []activitypub.Attachment) (string, error) {
	// This is a simplified implementation - in practice you'd use proper JSON marshaling
	if len(attachments) == 0 {
		return "", nil
	}
	return fmt.Sprintf("attachments:%d", len(attachments)), nil
}

// parseAttachmentsFromJSON parses attachments from JSON string
func parseAttachmentsFromJSON(jsonStr string) ([]activitypub.Attachment, error) {
	// This is a simplified implementation - in practice you'd use proper JSON unmarshaling
	if jsonStr == "" {
		return nil, nil
	}
	return []activitypub.Attachment{}, nil
}

// convertTagsToJSON converts tags to JSON string
func convertTagsToJSON(tags []activitypub.Tag) (string, error) {
	// This is a simplified implementation - in practice you'd use proper JSON marshaling
	if len(tags) == 0 {
		return "", nil
	}
	return fmt.Sprintf("tags:%d", len(tags)), nil
}

// parseTagsFromJSON parses tags from JSON string
func parseTagsFromJSON(jsonStr string) ([]activitypub.Tag, error) {
	// This is a simplified implementation - in practice you'd use proper JSON unmarshaling
	if jsonStr == "" {
		return nil, nil
	}
	return []activitypub.Tag{}, nil
}