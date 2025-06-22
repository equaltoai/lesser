package handlers

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"

	"github.com/aron23/lesser/pkg/mastodon"
)

// HandleCreateStatus creates a new status (post)
func (h *Handler) HandleCreateStatus(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// The body should already be decoded by the router if it was base64 encoded
	body := request.Body

	// Check content type for form data
	contentType := request.Headers["content-type"]
	if contentType == "" {
		contentType = request.Headers["Content-Type"]
	}

	var req models.CreateStatusRequest

	// Handle form data
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		// Parse form data
		values, err := common.ParseFormValues(body)
		if err != nil {
			h.logger.Error("failed to parse form data",
				zap.Error(err),
				zap.String("body", body))
			return common.BadRequest(fmt.Errorf("invalid form data: %w", err)), nil
		}

		// Map form fields to request struct
		req.Status = values.Get("status")
		req.InReplyToID = values.Get("in_reply_to_id")
		req.Visibility = values.Get("visibility")
		req.Sensitive = values.Get("sensitive") == "true"
		req.SpoilerText = values.Get("spoiler_text")
		req.Language = values.Get("language")

		// Handle media IDs
		if mediaIDs := values.Get("media_ids[]"); mediaIDs != "" {
			req.MediaIDs = strings.Split(mediaIDs, ",")
		}

		// Handle poll
		if values.Get("poll[options][]") != "" {
			// Poll parsing not yet implemented - skip for now
			// This would require extending the models to support polls
		}

		// Handle scheduled_at
		if scheduledAt := values.Get("scheduled_at"); scheduledAt != "" {
			req.ScheduledAt = &scheduledAt
		}
	} else {
		// Parse as JSON
		if err := common.ParseRequestBody([]byte(body), &req); err != nil {
			h.logger.Error("failed to parse create status request",
				zap.Error(err),
				zap.String("body", body),
				zap.String("original_body", request.Body),
				zap.Bool("is_base64", request.IsBase64Encoded),
				zap.String("content_type", contentType))
			return common.BadRequest(fmt.Errorf("invalid request format: %w", err)), nil
		}
	}

	// Validate content
	if req.Status == "" {
		return common.UnprocessableEntity(errors.New("status content is required")), nil
	}

	// Character limit check (500 characters)
	if len(req.Status) > 500 {
		return common.UnprocessableEntity(errors.New("status content must not exceed 500 characters")), nil
	}

	// Check if this is a scheduled status
	if req.ScheduledAt != nil && *req.ScheduledAt != "" {
		return h.HandleScheduleStatus(ctx, claims, req)
	}

	// Default visibility
	if req.Visibility == "" {
		req.Visibility = "public"
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
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
			mediaObj, err := h.store.GetObject(ctx, fmt.Sprintf("MEDIA#%s", mediaID))
			if err != nil {
				h.logger.Warn("failed to get media metadata", zap.String("media_id", mediaID), zap.Error(err))
				continue
			}

			// Extract media info
			if mediaMap, ok := mediaObj.(map[string]interface{}); ok {
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
		if err := h.store.RecordStatusEngagement(ctx, req.InReplyToID, "reply", actor.ID); err != nil {
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
			return common.UnprocessableEntity(errors.New("poll must have between 2 and 4 options")), nil
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

		if err := h.store.CreatePoll(ctx, poll); err != nil {
			h.logger.Error("failed to create poll", zap.Error(err))
			return common.InternalServerError(err), nil
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
			Emojis:      []interface{}{},
		}
	}

	// Parse and process custom emojis in content
	processedContent, parsedEmojis, err := h.emojiParser.ProcessContent(ctx, note.Content)
	if err != nil {
		// Log error but don't fail the request
		h.logger.Warn("failed to parse emojis in content", zap.Error(err))
		parsedEmojis = nil
	} else {
		// Update the note content with processed content (emojis replaced with img tags)
		note.Content = processedContent
	}

	// Create the Note object
	if err := h.store.CreateObject(ctx, note); err != nil {
		h.logger.Error("failed to create note object", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// If this is a reply, increment the reply count on the parent
	if req.InReplyToID != "" {
		if err := h.store.IncrementReplyCount(ctx, req.InReplyToID); err != nil {
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
	if err := h.store.CreateActivity(ctx, createActivity); err != nil {
		h.logger.Error("failed to create activity", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Fan out the post to timelines
	if err := h.store.FanOutPost(ctx, createActivity); err != nil {
		// Log the error but don't fail the request
		h.logger.Error("failed to fan out post to timelines", zap.Error(err))
	}

	// Record status creation activity for metrics
	if err := h.store.RecordActivity(ctx, "status", actor.ID, now); err != nil {
		// Log the error but don't fail the request
		h.logger.Warn("failed to record status activity", zap.Error(err))
	}

	// Record hashtags for trending (using already extracted hashtags)
	for _, hashtag := range hashtags {
		// Remove the # prefix if present
		cleanHashtag := strings.TrimPrefix(hashtag, "#")
		if err := h.store.RecordHashtagUsage(ctx, cleanHashtag, noteID, actor.ID); err != nil {
			h.logger.Warn("failed to record hashtag usage",
				zap.String("hashtag", cleanHashtag),
				zap.Error(err))
		}
	}

	// Extract and record links for trending
	links := extractLinksFromContent(req.Status)
	for _, link := range links {
		if err := h.store.RecordLinkShare(ctx, link, noteID, actor.ID); err != nil {
			h.logger.Warn("failed to record link share",
				zap.String("link", link),
				zap.Error(err))
		}
	}

	// Update actor's last status time
	if err := h.store.UpdateActorLastStatusTime(ctx, claims.Username); err != nil {
		h.logger.Warn("failed to update actor last status time", zap.Error(err))
	}

	// Convert hashtags for Mastodon API response
	mastodonTags := make([]interface{}, 0, len(note.Tag))
	for _, tag := range note.Tag {
		if tag.Type == "Hashtag" {
			// Extract tag name without # prefix
			tagName := strings.TrimPrefix(tag.Name, "#")
			mastodonTags = append(mastodonTags, map[string]interface{}{
				"name": tagName,
				"url":  tag.Href,
			})
		}
	}

	// Convert parsed emojis to Mastodon API format
	mastodonEmojis := make([]interface{}, 0, len(parsedEmojis))
	for _, parsed := range parsedEmojis {
		if parsed.Emoji != nil {
			mastodonEmojis = append(mastodonEmojis, map[string]interface{}{
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
		MediaAttachments: []interface{}{},
		Mentions:         []interface{}{},
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
			Emojis:         []interface{}{},
			Fields:         []interface{}{},
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
		if accountID := h.getReplyToAccountID(ctx, req.InReplyToID); accountID != "" {
			resp.InReplyToAccountID = &accountID
		}
	}

	respBody, _ := json.Marshal(resp)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusCreated,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(respBody),
	}, nil
}

// HandleFavourite favorites a status
func (h *Handler) HandleFavourite(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Create a Like activity
	likeActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.LikeType,
			ID:      fmt.Sprintf("%s/activities/like-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
			To:      []string{activitypub.PublicAddress},
		},
		Actor:  actor.ID,
		Object: objectID,
	}
	now := time.Now()
	likeActivity.Published = &now

	// Create the Like record in dedicated storage
	like := &storage.Like{
		Actor:     actor.ID,
		Object:    objectID,
		ID:        likeActivity.ID,
		Published: *likeActivity.Published,
	}

	if err := h.store.CreateLike(ctx, like); err != nil {
		h.logger.Error("failed to create like", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.store.CreateActivity(ctx, likeActivity); err != nil {
		h.logger.Error("failed to create like activity", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Record engagement for trending
	if err := h.store.RecordStatusEngagement(ctx, objectID, "like", actor.ID); err != nil {
		h.logger.Warn("failed to record status engagement",
			zap.String("status_id", statusID),
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	// Get object to return status information
	_, err = h.store.GetObject(ctx, objectID)
	if err != nil {
		// If object not found locally, still return success but with minimal info
		h.logger.Warn("object not found locally", zap.String("object_id", objectID), zap.Error(err))
	}

	// Get like count for the object
	likeCount, _ := h.store.CountObjectLikes(ctx, objectID)

	// Return a simplified status response
	resp := models.FavouriteResponse{
		ID:              statusID,
		CreatedAt:       likeActivity.Published.Format("2006-01-02T15:04:05.000Z"),
		Favourited:      true,
		FavouritesCount: likeCount,
		URI:             objectID,
		URL:             objectID,
		Content:         "", // Would be populated from object
		Visibility:      "public",
		Language:        "en",
	}

	body, _ := json.Marshal(resp)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleUnfavourite removes a favorite from a status
func (h *Handler) HandleUnfavourite(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Check if the like exists
	_, err = h.store.GetLike(ctx, actor.ID, objectID)
	if err != nil {
		// Like doesn't exist, return success anyway for idempotency
		h.logger.Info("like not found",
			zap.String("actor", actor.ID),
			zap.String("object", objectID))
	} else {
		// Create an Undo Like activity
		undoActivity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      fmt.Sprintf("%s/activities/undo-like-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
				To:      []string{activitypub.PublicAddress},
			},
			Actor: actor.ID,
			Object: map[string]interface{}{
				"type":   activitypub.LikeType,
				"actor":  actor.ID,
				"object": objectID,
			},
		}
		now := time.Now()
		undoActivity.Published = &now

		// Delete the Like record from dedicated storage
		if err := h.store.DeleteLike(ctx, actor.ID, objectID); err != nil {
			h.logger.Error("failed to delete like", zap.Error(err))
			return common.InternalServerError(err), nil
		}

		// Store the activity in the outbox (this will trigger delivery)
		if err := h.store.CreateActivity(ctx, undoActivity); err != nil {
			h.logger.Error("failed to create undo like activity", zap.Error(err))
			return common.InternalServerError(err), nil
		}
	}

	// Get like count for the object
	likeCount, _ := h.store.CountObjectLikes(ctx, objectID)

	// Return a simplified status response
	resp := models.FavouriteResponse{
		ID:              statusID,
		CreatedAt:       time.Now().Format("2006-01-02T15:04:05.000Z"),
		Favourited:      false,
		FavouritesCount: likeCount,
		URI:             objectID,
		URL:             objectID,
		Content:         "", // Would be populated from object
		Visibility:      "public",
		Language:        "en",
	}

	body, _ := json.Marshal(resp)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleReblog reblogs (announces) a status
func (h *Handler) HandleReblog(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Create an Announce activity
	announceActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.AnnounceType,
			ID:      fmt.Sprintf("%s/activities/announce-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
			To:      []string{activitypub.PublicAddress},
		},
		Actor:  actor.ID,
		Object: objectID,
	}
	if actor.Followers != "" {
		announceActivity.CC = []string{actor.Followers}
	}
	now := time.Now()
	announceActivity.Published = &now

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.store.CreateActivity(ctx, announceActivity); err != nil {
		h.logger.Error("failed to create announce activity", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Record engagement for trending
	if err := h.store.RecordStatusEngagement(ctx, objectID, "boost", actor.ID); err != nil {
		h.logger.Warn("failed to record status engagement",
			zap.String("status_id", statusID),
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	// Get object to return status information
	_, err = h.store.GetObject(ctx, objectID)
	if err != nil {
		// If object not found locally, still return success but with minimal info
		h.logger.Warn("object not found locally", zap.String("object_id", objectID), zap.Error(err))
	}

	// Get announce count for the object
	announceCount, _ := h.store.CountObjectAnnounces(ctx, objectID)

	// Return a simplified status response
	resp := models.FavouriteResponse{
		ID:           statusID,
		CreatedAt:    announceActivity.Published.Format("2006-01-02T15:04:05.000Z"),
		Reblogged:    true,
		ReblogsCount: announceCount,
		URI:          objectID,
		URL:          objectID,
		Content:      "", // Would be populated from object
		Visibility:   "public",
		Language:     "en",
	}

	body, _ := json.Marshal(resp)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleUnreblog removes a reblog (announce) from a status
func (h *Handler) HandleUnreblog(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Check if the announce exists
	_, err = h.store.GetAnnounce(ctx, actor.ID, objectID)
	if err != nil {
		// Announce doesn't exist, return success anyway for idempotency
		h.logger.Info("announce not found",
			zap.String("actor", actor.ID),
			zap.String("object", objectID))
	} else {
		// Create an Undo Announce activity
		undoActivity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    activitypub.UndoType,
				ID:      fmt.Sprintf("%s/activities/undo-announce-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
				To:      []string{activitypub.PublicAddress},
			},
			Actor: actor.ID,
			Object: map[string]interface{}{
				"type":   activitypub.AnnounceType,
				"actor":  actor.ID,
				"object": objectID,
			},
		}
		now := time.Now()
		undoActivity.Published = &now

		// Delete the Announce record from dedicated storage
		if err := h.store.DeleteAnnounce(ctx, actor.ID, objectID); err != nil {
			h.logger.Error("failed to delete announce", zap.Error(err))
			return common.InternalServerError(err), nil
		}

		// Store the activity in the outbox (this will trigger delivery)
		if err := h.store.CreateActivity(ctx, undoActivity); err != nil {
			h.logger.Error("failed to create undo announce activity", zap.Error(err))
			return common.InternalServerError(err), nil
		}
	}

	// Get announce count for the object
	announceCount, _ := h.store.CountObjectAnnounces(ctx, objectID)

	// Return a simplified status response
	resp := models.FavouriteResponse{
		ID:           statusID,
		CreatedAt:    time.Now().Format("2006-01-02T15:04:05.000Z"),
		Reblogged:    false,
		ReblogsCount: announceCount,
		URI:          objectID,
		URL:          objectID,
		Content:      "", // Would be populated from object
		Visibility:   "public",
		Language:     "en",
	}

	body, _ := json.Marshal(resp)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleDeleteStatus deletes a status
func (h *Handler) HandleDeleteStatus(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the object to verify ownership
	object, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found")), nil
	}

	// Check if the user owns this object
	var attributedTo string
	switch obj := object.(type) {
	case *activitypub.Note:
		attributedTo = obj.AttributedTo
	case map[string]interface{}:
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
			return common.InternalServerError(fmt.Errorf("unexpected object type")), nil
		}
	}

	if attributedTo != actor.ID {
		return common.Forbidden(errors.New("you can only delete your own statuses")), nil
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
	if err := h.store.CreateActivity(ctx, deleteActivity); err != nil {
		h.logger.Error("failed to create delete activity", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Tombstone the object from storage
	if err := h.store.TombstoneObject(ctx, objectID, actor.ID); err != nil {
		h.logger.Error("failed to tombstone object", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Return empty JSON object for successful deletion
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: "{}",
	}, nil
}

// HandleUpdateStatus updates an existing status
func (h *Handler) HandleUpdateStatus(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the object to verify ownership
	object, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found")), nil
	}

	// Parse request
	var req models.UpdateStatusRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Check if the user owns this object and update it
	var note *activitypub.Note
	switch obj := object.(type) {
	case *activitypub.Note:
		if obj.AttributedTo != actor.ID {
			return common.Forbidden(errors.New("you can only update your own statuses")), nil
		}
		note = obj
	case map[string]interface{}:
		if attr, ok := obj["attributedTo"].(string); ok && attr != actor.ID {
			return common.Forbidden(errors.New("you can only update your own statuses")), nil
		}
		// Convert map to Note
		noteBytes, err := json.Marshal(obj)
		if err != nil {
			h.logger.Error("failed to marshal object to JSON", zap.Error(err))
			return common.InternalServerError(err), nil
		}
		note = &activitypub.Note{}
		if err := json.Unmarshal(noteBytes, note); err != nil {
			h.logger.Error("failed to unmarshal JSON to Note", zap.Error(err))
			return common.InternalServerError(err), nil
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
			return common.InternalServerError(fmt.Errorf("unexpected object type")), nil
		}

		if attributedTo != actor.ID {
			return common.Forbidden(errors.New("you can only update your own statuses")), nil
		}

		// Convert to Note via JSON marshaling
		noteBytes, _ := json.Marshal(object)
		note = &activitypub.Note{}
		if err := json.Unmarshal(noteBytes, note); err != nil {
			return common.InternalServerError(fmt.Errorf("failed to convert object to Note")), nil
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
	if err := h.store.UpdateObject(ctx, note); err != nil {
		h.logger.Error("failed to update object", zap.Error(err))
		return common.InternalServerError(err), nil
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
	if err := h.store.CreateActivity(ctx, updateActivity); err != nil {
		h.logger.Error("failed to create update activity", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Return updated status response
	resp := h.converter.ObjectToStatus(note, actor)
	if req.Visibility != "" {
		resp.Visibility = req.Visibility
	}
	if req.Language != "" {
		resp.Language = req.Language
	}

	body, _ := json.Marshal(resp)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleGetStatus retrieves a single status by ID
func (h *Handler) HandleGetStatus(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the object
	object, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found")), nil
	}

	// Get the actor who created the object
	var attributedTo string
	switch obj := object.(type) {
	case *activitypub.Note:
		attributedTo = obj.AttributedTo
	case map[string]interface{}:
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
			actor, _ = h.store.GetActor(ctx, username)
		}
	}

	// Convert to status response
	status := h.converter.ObjectToStatus(object, actor)

	// Parse emojis from content
	parsedEmojis, err := h.emojiParser.ParseEmojis(ctx, status.Content)
	if err != nil {
		// Log error but don't fail the request
		h.logger.Warn("failed to parse emojis in status content", zap.Error(err))
	} else if len(parsedEmojis) > 0 {
		// Convert parsed emojis to Mastodon API format
		mastodonEmojis := make([]interface{}, 0, len(parsedEmojis))
		for _, parsed := range parsedEmojis {
			if parsed.Emoji != nil {
				mastodonEmojis = append(mastodonEmojis, map[string]interface{}{
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
	likeCount, _ := h.store.CountObjectLikes(ctx, objectID)
	announceCount, _ := h.store.CountObjectAnnounces(ctx, objectID)
	status.FavouritesCount = likeCount
	status.ReblogsCount = announceCount

	// Check if status has a poll
	poll, err := h.store.GetPollByStatusID(ctx, objectID)
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
			Emojis:      []interface{}{},
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
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	if token, err := auth.ExtractBearerToken(authHeader); err == nil {
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
		if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
			if userActor, err := h.store.GetActor(ctx, claims.Username); err == nil {
				// Check if user has liked this status
				if _, err := h.store.GetLike(ctx, userActor.ID, objectID); err == nil {
					status.Favourited = true
				}
				// Check if user has reblogged this status
				if _, err := h.store.GetAnnounce(ctx, userActor.ID, objectID); err == nil {
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

	body, _ := json.Marshal(status)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleGetStatusContext retrieves the context (ancestors and descendants) of a status
func (h *Handler) HandleGetStatusContext(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the object to check it exists
	_, err := h.store.GetObject(ctx, objectID)
	if err != nil {
		return common.NotFound(fmt.Errorf("status not found")), nil
	}

	// Get ancestors (statuses this replies to)
	ancestors := []models.Status{}
	currentID := objectID
	for i := 0; i < 10; i++ { // Limit depth to prevent infinite loops
		obj, err := h.store.GetObject(ctx, currentID)
		if err != nil {
			break
		}

		var inReplyTo string
		switch o := obj.(type) {
		case *activitypub.Note:
			inReplyTo = o.InReplyTo
		case map[string]interface{}:
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

		parentObj, err := h.store.GetObject(ctx, inReplyTo)
		if err != nil {
			break
		}

		// Get actor for parent
		var parentActor *activitypub.Actor
		var attributedTo string
		switch o := parentObj.(type) {
		case *activitypub.Note:
			attributedTo = o.AttributedTo
		case map[string]interface{}:
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
				parentActor, _ = h.store.GetActor(ctx, username)
			}
		}

		status := h.converter.ObjectToStatus(parentObj, parentActor)
		ancestors = append([]models.Status{status}, ancestors...) // Prepend to maintain order
		currentID = inReplyTo
	}

	// Get descendants (replies to this status)
	descendants := []models.Status{}

	// Fetch replies to this status
	replies, _, err := h.store.GetReplies(ctx, objectID, 100, "") // Get up to 100 replies
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
			case map[string]interface{}:
				if attr, ok := o["attributedTo"].(string); ok {
					attributedTo = attr
				}
			}

			if attributedTo != "" {
				username := h.converter.ExtractUsernameFromActorID(attributedTo)
				if username != "" {
					replyActor, _ = h.store.GetActor(ctx, username)
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

	body, _ := json.Marshal(resp)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleGetAccountStatuses retrieves statuses for a specific account
func (h *Handler) HandleGetAccountStatuses(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Resolve account ID to actor
	actor, err := h.resolveAccountID(ctx, accountID)
	if err != nil {
		return common.NotFound(fmt.Errorf("account not found")), nil
	}

	// Parse query parameters
	limit := 20
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			limit = parsedLimit
		}
	}

	maxID := request.QueryStringParameters["max_id"]
	onlyMedia := request.QueryStringParameters["only_media"] == "true"
	excludeReplies := request.QueryStringParameters["exclude_replies"] == "true"
	excludeReblogs := request.QueryStringParameters["exclude_reblogs"] == "true"
	tagged := request.QueryStringParameters["tagged"]

	// Get objects by this actor
	objects, cursor, err := h.store.GetObjectsByActor(ctx, actor.ID, maxID, limit)
	if err != nil {
		h.logger.Error("failed to get objects by actor", zap.Error(err))
		return common.InternalServerError(err), nil
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
			case map[string]interface{}:
				if attachments, ok := o["attachment"].([]interface{}); ok {
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
			case map[string]interface{}:
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
	headers := map[string]string{
		"Content-Type": "application/json",
	}
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
		headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL)
	}

	body, _ := json.Marshal(statuses)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    headers,
		Body:       string(body),
	}, nil
}

// Helper functions

// generateRandomString generates a random string of the specified length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// getStringFromMap safely gets a string from a map[string]interface{}
func getStringFromMap(m map[string]interface{}, key, defaultValue string) string {
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
	case map[string]interface{}:
		if attributedTo, ok := o["attributedTo"].(string); ok {
			parts := strings.Split(attributedTo, "/")
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	}

	return ""
}
