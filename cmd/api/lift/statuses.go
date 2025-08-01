package lift

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleCreateStatusLift creates a new status (post)
func (h *Handler) HandleCreateStatusLift(ctx *lift.Context) error {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "missing token"})
	}

	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": err.Error()})
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Check content type for form data
	contentType := ctx.Header("Content-Type")
	if contentType == "" {
		contentType = ctx.Header("content-type")
	}

	var req models.CreateStatusRequest

	// Handle form data
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		// Parse form data using common utility
		body := string(ctx.Request.Body)
		params, err := common.ParseFormURLEncoded(body)
		if err != nil {
			h.logger.Error("failed to parse form data", zap.Error(err))
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid form data"})
		}

		// Map form fields
		req.Status = params["status"]
		req.InReplyToID = params["in_reply_to_id"]
		req.Visibility = params["visibility"]
		req.Sensitive = params["sensitive"] == "true"
		req.SpoilerText = params["spoiler_text"]
		req.Language = params["language"]

		// Handle media IDs
		if mediaIDs := params["media_ids[]"]; mediaIDs != "" {
			req.MediaIDs = strings.Split(mediaIDs, ",")
		}

		// Handle scheduled_at
		if scheduledAt := params["scheduled_at"]; scheduledAt != "" {
			req.ScheduledAt = &scheduledAt
		}
	} else {
		// Parse as JSON
		if err := ctx.ParseRequest(&req); err != nil {
			h.logger.Error("failed to parse create status request", zap.Error(err))
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request format"})
		}
	}

	// Validate content
	if req.Status == "" {
		return ctx.Status(http.StatusUnprocessableEntity).JSON(map[string]string{"error": "status content is required"})
	}

	// Character limit check (500 characters)
	if len(req.Status) > 500 {
		return ctx.Status(http.StatusUnprocessableEntity).JSON(map[string]string{"error": "status content must not exceed 500 characters"})
	}

	// Check if this is a scheduled status
	if req.ScheduledAt != nil && *req.ScheduledAt != "" {
		return h.HandleScheduleStatusLift(ctx, claims, req)
	}

	// Default visibility
	if req.Visibility == "" {
		req.Visibility = "public"
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Create a Note object
	noteID := fmt.Sprintf("%s/objects/%d-%s", h.cfg.BaseURL(), time.Now().Unix(), generateRandomString(8))
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        noteID,
			Type:      activitypub.NoteType,
			Summary:   req.SpoilerText,
			Sensitive: req.Sensitive,
		},
		Content:      req.Status,
		AttributedTo: actor.ID,
	}

	// Set timestamps
	now := time.Now()
	note.Published = &now

	// Extract hashtags from content
	hashtags := mastodon.ExtractHashtagsWithCase(req.Status)
	if len(hashtags) > 0 {
		tags := make([]activitypub.Tag, 0, len(hashtags))
		for _, tag := range hashtags {
			// Create hashtag URL
			normalizedTag := mastodon.NormalizeHashtag(tag)
			tagURL := fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), normalizedTag)

			tags = append(tags, activitypub.Tag{
				Type: "Hashtag",
				Name: "#" + tag, // Keep original case for display
				Href: tagURL,
			})
		}
		note.Tag = tags
	}

	// Process media attachments
	if len(req.MediaIDs) > 0 {
		attachments := make([]activitypub.Attachment, 0, len(req.MediaIDs))
		for _, mediaID := range req.MediaIDs {
			// Get media metadata from storage (it was stored by the media upload handler)
			mediaObj, err := h.store.GetObject(ctx.Context, fmt.Sprintf("MEDIA#%s", mediaID))
			if err != nil {
				h.logger.Warn("failed to get media metadata", zap.String("media_id", mediaID), zap.Error(err))
				continue
			}

			// Extract media info
			if mediaMap, ok := mediaObj.(map[string]any); ok {
				attachment := activitypub.Attachment{
					Type:      "Document",
					MediaType: getStringFromMap(mediaMap, "MimeType", "image/jpeg"),
					URL:       getStringFromMap(mediaMap, "URL", ""),
					Name:      getStringFromMap(mediaMap, "Description", ""),
				}
				if attachment.URL != "" {
					attachments = append(attachments, attachment)
				}
			}
		}
		note.Attachment = attachments
	}

	// Set addressing based on visibility
	switch req.Visibility {
	case "public":
		note.To = []string{activitypub.PublicAddress}
		note.CC = []string{actor.Followers}
	case "unlisted":
		note.To = []string{actor.Followers}
		note.CC = []string{activitypub.PublicAddress}
	case "private":
		note.To = []string{actor.Followers}
	case "direct":
		// Extract mentions from content and add to To field
		mentions := h.extractMentions(req.Status)
		note.To = mentions
	}

	// Store visibility explicitly
	note.Visibility = req.Visibility

	// Handle reply
	if req.InReplyToID != "" {
		note.InReplyTo = req.InReplyToID

		// Record reply engagement for trending
		if err := h.store.RecordStatusEngagement(ctx.Context, req.InReplyToID, "reply", actor.ID); err != nil {
			h.logger.Warn("failed to record reply engagement",
				zap.String("parent_status_id", req.InReplyToID),
				zap.Error(err))
		}
	}

	// Handle poll creation if requested
	var pollResp *models.Poll
	if req.Poll != nil && len(req.Poll.Options) > 0 {
		// Validate poll options
		if len(req.Poll.Options) < 2 || len(req.Poll.Options) > 4 {
			return ctx.Status(http.StatusUnprocessableEntity).JSON(map[string]string{"error": "poll must have between 2 and 4 options"})
		}

		// Calculate expiration time
		expiresAt := time.Now().Add(time.Duration(req.Poll.ExpiresIn) * time.Second)

		// Create poll in storage
		poll := &storage.Poll{
			StatusID:   noteID,
			CreatedBy:  actor.ID,
			Options:    req.Poll.Options,
			Multiple:   req.Poll.Multiple,
			HideTotals: req.Poll.HideTotals,
			ExpiresAt:  expiresAt,
		}

		if err := h.store.CreatePoll(ctx.Context, poll); err != nil {
			h.logger.Error("failed to create poll", zap.Error(err))
			return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
		}

		// Build poll response
		optionsData := make([]models.PollOption, len(poll.Options))
		for i, option := range poll.Options {
			optionsData[i] = models.PollOption{
				Title:      option,
				VotesCount: 0,
			}
		}

		pollResp = &models.Poll{
			ID:          poll.ID,
			ExpiresAt:   poll.ExpiresAt.Format(time.RFC3339),
			Expired:     false,
			Multiple:    poll.Multiple,
			VotesCount:  0,
			VotersCount: 0,
			Voted:       false,
			OwnVotes:    nil,
			OptionsData: optionsData,
			Emojis:      []any{},
		}
	}

	// Parse and process custom emojis in content
	emojiParser := mastodon.NewEmojiParser(h.store)
	processedContent, parsedEmojis, err := emojiParser.ProcessContent(ctx.Context, note.Content)
	if err != nil {
		// Log error but don't fail the request
		h.logger.Warn("failed to parse emojis in content", zap.Error(err))
		parsedEmojis = nil
	} else {
		// Update the note content with processed content (emojis replaced with img tags)
		note.Content = processedContent
	}

	// Create the Note object
	if err := h.store.CreateObject(ctx.Context, note); err != nil {
		h.logger.Error("failed to create note object", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// If this is a reply, increment the reply count on the parent
	if req.InReplyToID != "" {
		if err := h.store.IncrementReplyCount(ctx.Context, req.InReplyToID); err != nil {
			// Log but don't fail the request
			h.logger.Warn("failed to increment reply count",
				zap.String("parent_id", req.InReplyToID),
				zap.Error(err))
		}
	}

	// Create a Create activity
	createActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      activitypub.CreateType,
			ID:        fmt.Sprintf("%s/activities/create-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
			To:        note.To,
			Published: &now,
		},
		Actor:  actor.ID,
		Object: note,
	}

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.store.CreateActivity(ctx.Context, createActivity); err != nil {
		h.logger.Error("failed to create activity", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Fan out the post to timelines
	if err := h.store.FanOutPost(ctx.Context, createActivity); err != nil {
		// Log the error but don't fail the request
		h.logger.Error("failed to fan out post to timelines", zap.Error(err))
	}

	// Record status creation activity for metrics
	if err := h.store.RecordActivity(ctx.Context, "status", actor.ID, now); err != nil {
		// Log the error but don't fail the request
		h.logger.Warn("failed to record status activity", zap.Error(err))
	}

	// Record hashtags for trending (using already extracted hashtags)
	for _, hashtag := range hashtags {
		// Remove the # prefix if present
		cleanHashtag := strings.TrimPrefix(hashtag, "#")
		if err := h.store.RecordHashtagUsage(ctx.Context, cleanHashtag, noteID, actor.ID); err != nil {
			h.logger.Warn("failed to record hashtag usage",
				zap.String("hashtag", cleanHashtag),
				zap.Error(err))
		}
	}

	// Extract and record links for trending
	links := extractLinksFromContent(req.Status)
	for _, link := range links {
		if err := h.store.RecordLinkShare(ctx.Context, link, noteID, actor.ID); err != nil {
			h.logger.Warn("failed to record link share",
				zap.String("link", link),
				zap.Error(err))
		}
	}

	// Update actor's last status time
	if err := h.store.UpdateActorLastStatusTime(ctx.Context, claims.Username); err != nil {
		h.logger.Warn("failed to update actor last status time", zap.Error(err))
	}

	// Convert hashtags for Mastodon API response
	mastodonTags := make([]any, 0, len(note.Tag))
	for _, tag := range note.Tag {
		if tag.Type == "Hashtag" {
			// Extract tag name without # prefix
			tagName := strings.TrimPrefix(tag.Name, "#")
			mastodonTags = append(mastodonTags, map[string]any{
				"name": tagName,
				"url":  tag.Href,
			})
		}
	}

	// Convert parsed emojis to Mastodon API format
	mastodonEmojis := make([]any, 0, len(parsedEmojis))
	for _, parsed := range parsedEmojis {
		if parsed.Emoji != nil {
			mastodonEmojis = append(mastodonEmojis, map[string]any{
				"shortcode":         parsed.Emoji.Shortcode,
				"url":               parsed.Emoji.URL,
				"static_url":        parsed.Emoji.StaticURL,
				"visible_in_picker": parsed.Emoji.VisibleInPicker,
				"category":          parsed.Emoji.Category,
			})
		}
	}

	// Return status response
	resp := models.Status{
		ID:               noteID,
		CreatedAt:        note.Published.Format("2006-01-02T15:04:05.000Z"),
		Sensitive:        note.Sensitive,
		SpoilerText:      note.Summary,
		Visibility:       req.Visibility,
		Language:         req.Language,
		URI:              noteID,
		URL:              noteID,
		Content:          note.Content,
		RepliesCount:     0,
		ReblogsCount:     0,
		FavouritesCount:  0,
		Favourited:       false,
		Reblogged:        false,
		Muted:            false,
		Bookmarked:       false,
		Pinned:           false,
		MediaAttachments: []any{},
		Mentions:         []any{},
		Tags:             mastodonTags,
		Emojis:           mastodonEmojis,
		Poll:             pollResp,
		Account: models.Account{
			ID:             actor.ID,
			Username:       actor.PreferredUsername,
			Acct:           actor.PreferredUsername,
			DisplayName:    actor.Name,
			URL:            actor.URL,
			CreatedAt:      now.Format("2006-01-02T15:04:05.000Z"),
			Note:           actor.Summary,
			Avatar:         "",
			AvatarStatic:   "",
			Header:         "",
			HeaderStatic:   "",
			FollowersCount: 0,
			FollowingCount: 0,
			StatusesCount:  0,
			Emojis:         []any{},
			Fields:         []any{},
		},
	}

	// Populate avatar from actor Icon
	if actor.Icon != nil && actor.Icon.URL != "" {
		resp.Account.Avatar = actor.Icon.URL
		resp.Account.AvatarStatic = actor.Icon.URL
	}

	// Populate header from actor Image
	if actor.Image != nil && actor.Image.URL != "" {
		resp.Account.Header = actor.Image.URL
		resp.Account.HeaderStatic = actor.Image.URL
	}

	// Handle reply fields
	if req.InReplyToID != "" {
		resp.InReplyToID = &req.InReplyToID
		// Extract account ID from the replied-to status
		if accountID := h.getReplyToAccountID(ctx.Context, req.InReplyToID); accountID != "" {
			resp.InReplyToAccountID = &accountID
		}
	}

	ctx.Status(http.StatusCreated)
	return ctx.JSON(resp)
}

// HandleDeleteStatusLift deletes a status
func (h *Handler) HandleDeleteStatusLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "missing token"})
	}

	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": err.Error()})
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the object to verify ownership
	object, err := h.store.GetObject(ctx.Context, objectID)
	if err != nil {
		return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
	}

	// Check if the user owns this object
	var attributedTo string
	switch obj := object.(type) {
	case *activitypub.Note:
		attributedTo = obj.AttributedTo
	case map[string]any:
		if attr, ok := obj["attributedTo"].(string); ok {
			attributedTo = attr
		}
	default:
		// Try to handle any object with AttributedTo field using reflection
		v := reflect.ValueOf(object)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}

		if v.Kind() == reflect.Struct {
			// Try to get AttributedTo field
			if attrField := v.FieldByName("AttributedTo"); attrField.IsValid() && attrField.Kind() == reflect.String {
				attributedTo = attrField.String()
			}
		}

		if attributedTo == "" {
			h.logger.Error("unexpected object type or missing AttributedTo",
				zap.String("type", fmt.Sprintf("%T", object)),
				zap.Any("object", object))
			return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "unexpected object type"})
		}
	}

	if attributedTo != actor.ID {
		return ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "you can only delete your own statuses"})
	}

	// Create a Delete activity
	deleteActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.DeleteType,
			ID:      fmt.Sprintf("%s/activities/delete-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
			To:      []string{activitypub.PublicAddress},
		},
		Actor:  actor.ID,
		Object: objectID,
	}
	now := time.Now()
	deleteActivity.Published = &now

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.store.CreateActivity(ctx.Context, deleteActivity); err != nil {
		h.logger.Error("failed to create delete activity", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Tombstone the object from storage
	if err := h.store.TombstoneObject(ctx.Context, objectID, actor.ID); err != nil {
		h.logger.Error("failed to tombstone object", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Return empty JSON object for successful deletion
	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{})
}

// HandleUpdateStatusLift updates an existing status
func (h *Handler) HandleUpdateStatusLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "missing token"})
	}

	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": err.Error()})
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the object to verify ownership
	object, err := h.store.GetObject(ctx.Context, objectID)
	if err != nil {
		return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
	}

	// Parse request
	var req models.UpdateStatusRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request format"})
	}

	// Check if the user owns this object and update it
	var note *activitypub.Note
	switch obj := object.(type) {
	case *activitypub.Note:
		if obj.AttributedTo != actor.ID {
			return ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "you can only update your own statuses"})
		}
		note = obj
	case map[string]any:
		if attr, ok := obj["attributedTo"].(string); ok && attr != actor.ID {
			return ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "you can only update your own statuses"})
		}
		// Convert map to Note
		noteBytes, err := json.Marshal(obj)
		if err != nil {
			h.logger.Error("failed to marshal object to JSON", zap.Error(err))
			return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
		}
		note = &activitypub.Note{}
		if err := json.Unmarshal(noteBytes, note); err != nil {
			h.logger.Error("failed to unmarshal JSON to Note", zap.Error(err))
			return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
		}
	default:
		// Try to handle any object with AttributedTo field using reflection
		v := reflect.ValueOf(object)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}

		var attributedTo string
		if v.Kind() == reflect.Struct {
			// Try to get AttributedTo field
			if attrField := v.FieldByName("AttributedTo"); attrField.IsValid() && attrField.Kind() == reflect.String {
				attributedTo = attrField.String()
			}
		}

		if attributedTo == "" {
			h.logger.Error("unexpected object type or missing AttributedTo",
				zap.String("type", fmt.Sprintf("%T", object)),
				zap.Any("object", object))
			return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "unexpected object type"})
		}

		if attributedTo != actor.ID {
			return ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "you can only update your own statuses"})
		}

		// Convert to Note via JSON marshaling
		noteBytes, _ := json.Marshal(object)
		note = &activitypub.Note{}
		if err := json.Unmarshal(noteBytes, note); err != nil {
			return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to convert object to Note"})
		}
	}

	// Update the note fields
	if req.Status != "" {
		note.Content = req.Status
	}
	if req.SpoilerText != "" {
		note.Summary = req.SpoilerText
	}
	note.Sensitive = req.Sensitive

	// Update timestamp
	now := time.Now()
	note.Updated = &now

	// Update the object in storage
	if err := h.store.UpdateObject(ctx.Context, note); err != nil {
		h.logger.Error("failed to update object", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Create an Update activity
	updateActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      activitypub.UpdateType,
			ID:        fmt.Sprintf("%s/activities/update-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
			To:        note.To,
			CC:        note.CC,
			Published: &now,
		},
		Actor:  actor.ID,
		Object: note,
	}

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.store.CreateActivity(ctx.Context, updateActivity); err != nil {
		h.logger.Error("failed to create update activity", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Return updated status response
	resp := h.converter.ObjectToStatus(note, actor)
	if req.Visibility != "" {
		resp.Visibility = req.Visibility
	}
	if req.Language != "" {
		resp.Language = req.Language
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// HandleGetStatusLift retrieves a single status by ID
func (h *Handler) HandleGetStatusLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the object
	object, err := h.store.GetObject(ctx.Context, objectID)
	if err != nil {
		return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
	}

	// Get the actor who created the object
	var attributedTo string
	switch obj := object.(type) {
	case *activitypub.Note:
		attributedTo = obj.AttributedTo
	case map[string]any:
		if attr, ok := obj["attributedTo"].(string); ok {
			attributedTo = attr
		}
	default:
		// Try to handle any object with AttributedTo field using reflection
		v := reflect.ValueOf(object)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}

		if v.Kind() == reflect.Struct {
			// Try to get AttributedTo field
			if attrField := v.FieldByName("AttributedTo"); attrField.IsValid() && attrField.Kind() == reflect.String {
				attributedTo = attrField.String()
			}
		}

		if attributedTo == "" {
			h.logger.Warn("object missing AttributedTo field",
				zap.String("type", fmt.Sprintf("%T", object)),
				zap.String("object_id", objectID))
			// Don't fail - just continue without actor info
		}
	}

	// Extract username from actor ID
	var actor *activitypub.Actor
	if attributedTo != "" {
		// Extract username from actor ID (format: https://domain/users/username)
		parts := strings.Split(attributedTo, "/")
		if len(parts) > 0 {
			username := parts[len(parts)-1]
			actor, _ = h.store.GetActor(ctx.Context, username)
		}
	}

	// Convert to status response
	status := h.converter.ObjectToStatus(object, actor)

	// Parse emojis from content
	emojiParser := mastodon.NewEmojiParser(h.store)
	parsedEmojis, err := emojiParser.ParseEmojis(ctx.Context, status.Content)
	if err != nil {
		// Log error but don't fail the request
		h.logger.Warn("failed to parse emojis in status content", zap.Error(err))
	} else if len(parsedEmojis) > 0 {
		// Convert parsed emojis to Mastodon API format
		mastodonEmojis := make([]any, 0, len(parsedEmojis))
		for _, parsed := range parsedEmojis {
			if parsed.Emoji != nil {
				mastodonEmojis = append(mastodonEmojis, map[string]any{
					"shortcode":         parsed.Emoji.Shortcode,
					"url":               parsed.Emoji.URL,
					"static_url":        parsed.Emoji.StaticURL,
					"visible_in_picker": parsed.Emoji.VisibleInPicker,
					"category":          parsed.Emoji.Category,
				})
			}
		}
		status.Emojis = mastodonEmojis
	}

	// Get interaction counts
	likeCount, _ := h.store.CountObjectLikes(ctx.Context, objectID)
	announceCount, _ := h.store.CountObjectAnnounces(ctx.Context, objectID)
	status.FavouritesCount = likeCount
	status.ReblogsCount = announceCount

	// Check if status has a poll
	poll, err := h.store.GetPollByStatusID(ctx.Context, objectID)
	if err == nil && poll != nil {
		// Calculate vote counts per option
		optionVotes := make([]int, len(poll.Options))
		for _, choices := range poll.Votes {
			for _, choice := range choices {
				if choice < len(optionVotes) {
					optionVotes[choice]++
				}
			}
		}

		// Check if poll has expired
		expired := !poll.ExpiresAt.IsZero() && time.Now().After(poll.ExpiresAt)

		// Build options data
		optionsData := make([]models.PollOption, len(poll.Options))
		for i, option := range poll.Options {
			optionsData[i] = models.PollOption{
				Title:      option,
				VotesCount: optionVotes[i],
			}
		}

		// Build poll response
		pollResp := &models.Poll{
			ID:          poll.ID,
			ExpiresAt:   poll.ExpiresAt.Format(time.RFC3339),
			Expired:     expired,
			Multiple:    poll.Multiple,
			VotesCount:  poll.VotesCount,
			VotersCount: poll.VotersCount,
			Voted:       false,
			OwnVotes:    nil,
			OptionsData: optionsData,
			Emojis:      []any{},
		}

		// Hide totals if requested and poll hasn't expired
		if poll.HideTotals && !expired {
			for i := range pollResp.OptionsData {
				pollResp.OptionsData[i].VotesCount = 0
			}
			pollResp.VotesCount = 0
			pollResp.VotersCount = 0
		}

		status.Poll = pollResp
	}

	// Check if the current user has interacted with this status
	token := h.getBearerTokenLift(ctx)
	if token != "" {
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
		if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
			if userActor, err := h.store.GetActor(ctx.Context, claims.Username); err == nil {
				// Check if user has liked this status
				if _, err := h.store.GetLike(ctx.Context, userActor.ID, objectID); err == nil {
					status.Favourited = true
				}
				// Check if user has reblogged this status
				if _, err := h.store.GetAnnounce(ctx.Context, userActor.ID, objectID); err == nil {
					status.Reblogged = true
				}
				// Check if user has voted on the poll
				if status.Poll != nil && poll != nil {
					if userVotes, ok := poll.Votes[userActor.ID]; ok {
						status.Poll.Voted = true
						status.Poll.OwnVotes = userVotes
					}
				}
			}
		}
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(status)
}

// HandleGetStatusContextLift retrieves the context (ancestors and descendants) of a status
func (h *Handler) HandleGetStatusContextLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the object to check it exists
	_, err := h.store.GetObject(ctx.Context, objectID)
	if err != nil {
		return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
	}

	// Get ancestors (statuses this replies to)
	ancestors := []models.Status{}
	currentID := objectID
	for i := 0; i < 10; i++ { // Limit depth to prevent infinite loops
		obj, err := h.store.GetObject(ctx.Context, currentID)
		if err != nil {
			break
		}

		var inReplyTo string
		switch o := obj.(type) {
		case *activitypub.Note:
			inReplyTo = o.InReplyTo
		case map[string]any:
			if reply, ok := o["inReplyTo"].(string); ok {
				inReplyTo = reply
			}
		default:
			// Try to handle any object with InReplyTo field using reflection
			v := reflect.ValueOf(obj)
			if v.Kind() == reflect.Ptr {
				v = v.Elem()
			}

			if v.Kind() == reflect.Struct {
				// Try to get InReplyTo field
				if replyField := v.FieldByName("InReplyTo"); replyField.IsValid() {
					// Handle pointer to string
					if replyField.Kind() == reflect.Ptr && !replyField.IsNil() {
						if replyField.Elem().Kind() == reflect.String {
							inReplyTo = replyField.Elem().String()
						}
					} else if replyField.Kind() == reflect.String {
						inReplyTo = replyField.String()
					}
				}
			}
		}

		if inReplyTo == "" {
			break
		}

		parentObj, err := h.store.GetObject(ctx.Context, inReplyTo)
		if err != nil {
			break
		}

		// Get actor for parent
		var parentActor *activitypub.Actor
		var attributedTo string
		switch o := parentObj.(type) {
		case *activitypub.Note:
			attributedTo = o.AttributedTo
		case map[string]any:
			if attr, ok := o["attributedTo"].(string); ok {
				attributedTo = attr
			}
		default:
			// Try to handle any object with AttributedTo field using reflection
			v := reflect.ValueOf(parentObj)
			if v.Kind() == reflect.Ptr {
				v = v.Elem()
			}

			if v.Kind() == reflect.Struct {
				// Try to get AttributedTo field
				if attrField := v.FieldByName("AttributedTo"); attrField.IsValid() && attrField.Kind() == reflect.String {
					attributedTo = attrField.String()
				}
			}
		}

		if attributedTo != "" {
			username := h.converter.ExtractUsernameFromActorID(attributedTo)
			if username != "" {
				parentActor, _ = h.store.GetActor(ctx.Context, username)
			}
		}

		status := h.converter.ObjectToStatus(parentObj, parentActor)
		ancestors = append([]models.Status{status}, ancestors...) // Prepend to maintain order
		currentID = inReplyTo
	}

	// Get descendants (replies to this status)
	descendants := []models.Status{}

	// Fetch replies to this status
	replies, _, err := h.store.GetReplies(ctx.Context, objectID, 100, "") // Get up to 100 replies
	if err != nil {
		h.logger.Warn("failed to get replies for context",
			zap.String("object_id", objectID),
			zap.Error(err))
	} else {
		for _, reply := range replies {
			// Get actor for reply
			var replyActor *activitypub.Actor
			var attributedTo string
			switch o := reply.(type) {
			case *activitypub.Note:
				attributedTo = o.AttributedTo
			case map[string]any:
				if attr, ok := o["attributedTo"].(string); ok {
					attributedTo = attr
				}
			}

			if attributedTo != "" {
				username := h.converter.ExtractUsernameFromActorID(attributedTo)
				if username != "" {
					replyActor, _ = h.store.GetActor(ctx.Context, username)
				}
			}

			status := h.converter.ObjectToStatus(reply, replyActor)
			descendants = append(descendants, status)
		}

		h.logger.Debug("fetched descendants for context",
			zap.String("object_id", objectID),
			zap.Int("count", len(descendants)))
	}

	// Return context response
	resp := models.StatusContext{
		Ancestors:   ancestors,
		Descendants: descendants,
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// HandleGetAccountStatusesLift retrieves statuses for a specific account
func (h *Handler) HandleGetAccountStatusesLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if accountID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing account id"})
	}

	// Resolve account ID to actor
	actor, err := h.resolveAccountID(ctx.Context, accountID)
	if err != nil {
		return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "account not found"})
	}

	// Parse query parameters
	limit := 20
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			limit = parsedLimit
		}
	}

	maxID := ctx.Query("max_id")
	onlyMedia := ctx.Query("only_media") == "true"
	excludeReplies := ctx.Query("exclude_replies") == "true"
	excludeReblogs := ctx.Query("exclude_reblogs") == "true"
	tagged := ctx.Query("tagged")

	// Get objects by this actor
	objects, cursor, err := h.store.GetObjectsByActor(ctx.Context, actor.ID, maxID, limit)
	if err != nil {
		h.logger.Error("failed to get objects by actor", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Convert objects to statuses
	statuses := []models.Status{}
	h.logger.Debug("converting objects to statuses",
		zap.Int("object_count", len(objects)),
		zap.String("actor_id", actor.ID))

	for _, obj := range objects {
		// Apply filters
		if onlyMedia {
			// Check if object has media attachments
			hasMedia := false
			switch o := obj.(type) {
			case *activitypub.Note:
				hasMedia = len(o.Attachment) > 0
			case map[string]any:
				if attachments, ok := o["attachment"].([]any); ok {
					hasMedia = len(attachments) > 0
				}
			}
			if !hasMedia {
				continue
			}
		}

		if excludeReplies {
			// Check if object is a reply
			isReply := false
			switch o := obj.(type) {
			case *activitypub.Note:
				isReply = o.InReplyTo != ""
			case map[string]any:
				if inReplyTo, ok := o["inReplyTo"].(string); ok {
					isReply = inReplyTo != ""
				}
			}
			if isReply {
				continue
			}
		}

		// Apply filters (exclude_reblogs and tagged filters not yet implemented)
		// This would require additional query parameters and filtering logic

		status := h.converter.ObjectToStatus(obj, actor)

		// Debug log to check the status data
		h.logger.Debug("converted status from object",
			zap.String("status_id", status.ID),
			zap.String("status_content", status.Content),
			zap.String("status_created_at", status.CreatedAt),
			zap.Any("object_type", fmt.Sprintf("%T", obj)))

		statuses = append(statuses, status)
	}

	// Set Link header for pagination if there's a cursor
	if cursor != "" {
		// Construct next URL
		nextURL := fmt.Sprintf("%s/api/v1/accounts/%s/statuses?max_id=%s", h.cfg.BaseURL(), accountID, cursor)
		if limit != 20 {
			nextURL += fmt.Sprintf("&limit=%d", limit)
		}
		if onlyMedia {
			nextURL += "&only_media=true"
		}
		if excludeReplies {
			nextURL += "&exclude_replies=true"
		}
		if excludeReblogs {
			nextURL += "&exclude_reblogs=true"
		}
		if tagged != "" {
			nextURL += fmt.Sprintf("&tagged=%s", tagged)
		}
		ctx.Response.Headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL)
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(statuses)
}

// Helper functions

// getStringFromMap safely gets a string from a map[string]any
func getStringFromMap(m map[string]any, key, defaultValue string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return defaultValue
}

// extractLinksFromContent extracts links from a given content
func extractLinksFromContent(content string) []string {
	links := []string{}
	words := strings.Fields(content)
	for _, word := range words {
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
			links = append(links, word)
		}
	}
	return links
}

// extractMentions extracts @mentions from content and returns actor URIs
func (h *Handler) extractMentions(content string) []string {
	mentions := []string{}
	words := strings.Fields(content)

	for _, word := range words {
		if strings.HasPrefix(word, "@") {
			// Remove @ and any trailing punctuation
			username := strings.TrimPrefix(word, "@")
			username = strings.TrimRight(username, ".,!?;:")

			if username != "" {
				// Convert username to actor URI
				actorURI := fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), username)
				mentions = append(mentions, actorURI)
			}
		}
	}

	return mentions
}

// getReplyToAccountID extracts account ID from a replied-to status
func (h *Handler) getReplyToAccountID(ctx context.Context, statusID string) string {
	// Get the object being replied to
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	obj, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return ""
	}

	// Extract attributedTo from the object
	switch o := obj.(type) {
	case *activitypub.Note:
		if o.AttributedTo != "" {
			// Extract username from actor URI
			parts := strings.Split(o.AttributedTo, "/")
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	case map[string]any:
		if attributedTo, ok := o["attributedTo"].(string); ok {
			parts := strings.Split(attributedTo, "/")
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	}

	return ""
}

// resolveAccountID resolves an account ID (which can be a username, numeric ID, or URL) to an actor
func (h *Handler) resolveAccountID(ctx context.Context, accountID string) (*activitypub.Actor, error) {
	// Handle different account ID formats
	if strings.HasPrefix(accountID, "http://") || strings.HasPrefix(accountID, "https://") {
		// Full ActivityPub actor URL
		// Extract username from URL like https://lesser.host/users/aron
		if strings.Contains(accountID, h.cfg.Domain) && strings.Contains(accountID, "/users/") {
			parts := strings.Split(accountID, "/users/")
			if len(parts) == 2 {
				username := parts[1]
				return h.store.GetActor(ctx, username)
			}
			return nil, fmt.Errorf("invalid account URL")
		}
		// Remote actor - not supported yet
		return nil, fmt.Errorf("remote accounts not yet supported")
	}

	// Check if it's a numeric ID (Mastodon compatibility)
	if _, err := strconv.ParseInt(accountID, 10, 64); err == nil && len(accountID) >= 10 {
		// It's a numeric ID - use the dedicated lookup method
		return h.store.GetActorByNumericID(ctx, accountID)
	}

	// Assume it's a username for local accounts
	return h.store.GetActor(ctx, accountID)
}