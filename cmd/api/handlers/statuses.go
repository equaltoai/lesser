package handlers

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
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

	// Parse request
	var req models.CreateStatusRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate request
	if req.Status == "" {
		return common.UnprocessableEntity(errors.New("status text is required")), nil
	}

	// Set default visibility if not specified
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
		// TODO: Extract mentions and add to To field
		note.To = []string{}
	}

	// Handle reply
	if req.InReplyToID != "" {
		note.InReplyTo = req.InReplyToID
	}

	// Create the Note object
	if err := h.store.CreateObject(ctx, note); err != nil {
		h.logger.Error("failed to create note object", zap.Error(err))
		return common.InternalServerError(err), nil
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
		Emojis:           []interface{}{},
		Account: models.Account{
			ID:             actor.ID,
			Username:       actor.PreferredUsername,
			Acct:           actor.PreferredUsername,
			DisplayName:    actor.Name,
			URL:            actor.URL,
			CreatedAt:      now.Format("2006-01-02T15:04:05.000Z"),
			Note:           actor.Summary,
			Avatar:         "", // TODO: Implement avatars
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

	// Handle reply fields
	if req.InReplyToID != "" {
		resp.InReplyToID = &req.InReplyToID
		// TODO: Extract account ID from the replied-to status
	}

	body, _ := json.Marshal(resp)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusCreated,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
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

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.store.CreateActivity(ctx, likeActivity); err != nil {
		h.logger.Error("failed to create like activity", zap.Error(err))
		return common.InternalServerError(err), nil
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

	// Delete the object from storage
	if err := h.store.DeleteObject(ctx, objectID); err != nil {
		h.logger.Error("failed to delete object", zap.Error(err))
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
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
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
		noteBytes, _ := json.Marshal(obj)
		note = &activitypub.Note{}
		json.Unmarshal(noteBytes, note)
	default:
		return common.InternalServerError(fmt.Errorf("unexpected object type")), nil
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

	// Get interaction counts
	likeCount, _ := h.store.CountObjectLikes(ctx, objectID)
	announceCount, _ := h.store.CountObjectAnnounces(ctx, objectID)
	status.FavouritesCount = likeCount
	status.ReblogsCount = announceCount

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
	// TODO: Implement GetReplies in storage
	// For now, just return empty descendants
	/*
		replies, _, err := h.store.GetReplies(ctx, objectID, 20, "")
		if err == nil {
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
					parts := strings.Split(attributedTo, "/")
					if len(parts) > 0 {
						username := parts[len(parts)-1]
						replyActor, _ = h.store.GetActor(ctx, username)
					}
				}

				status := ObjectToStatus(reply, replyActor)
				descendants = append(descendants, status)
			}
		}
	*/

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
	// In our implementation, account ID is the username
	username := accountID

	// Get the actor
	actor, err := h.store.GetActor(ctx, username)
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

		// TODO: Implement exclude_reblogs and tagged filters

		status := h.converter.ObjectToStatus(obj, actor)
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
