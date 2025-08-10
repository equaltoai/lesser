// statuses_service_full.go - Full service-based implementation of status endpoints
// This replaces the existing status handlers with service-first architecture

package lift

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/streaming"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleCreateStatusServiceFull creates a new status using the notes service
func (h *Handler) HandleCreateStatusServiceFull(ctx *lift.Context) error {
	// Authenticate and validate request
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "missing token"})
	}

	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": err.Error()})
	}

	if !claims.HasScope(auth.ScopeWrite) {
		return ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Parse request
	var req models.CreateStatusRequest
	contentType := ctx.Header("Content-Type")
	if contentType == "" {
		contentType = ctx.Header("content-type")
	}

	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		// Parse form data
		body := string(ctx.Request.Body)
		params, err := common.ParseFormURLEncoded(body)
		if err != nil {
			h.logger.Error("failed to parse form data", zap.Error(err))
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid form data"})
		}

		req.Status = params["status"]
		req.InReplyToID = params["in_reply_to_id"]
		req.Visibility = params["visibility"]
		req.Sensitive = params["sensitive"] == "true"
		req.SpoilerText = params["spoiler_text"]
		req.Language = params["language"]

		if mediaIDs := params["media_ids[]"]; mediaIDs != "" {
			req.MediaIDs = strings.Split(mediaIDs, ",")
		}
	} else {
		if err := ctx.ParseRequest(&req); err != nil {
			h.logger.Error("failed to parse create status request", zap.Error(err))
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request format"})
		}
	}

	// Validate request
	if req.Status == "" {
		return ctx.Status(http.StatusUnprocessableEntity).JSON(map[string]string{"error": "status content is required"})
	}

	if len(req.Status) > 500 {
		return ctx.Status(http.StatusUnprocessableEntity).JSON(map[string]string{"error": "status content must not exceed 500 characters"})
	}

	// Handle scheduled status
	if req.ScheduledAt != nil && *req.ScheduledAt != "" {
		// Parse scheduled time
		scheduledTime, err := time.Parse(time.RFC3339, *req.ScheduledAt)
		if err != nil {
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid scheduled_at format"})
		}

		// Create scheduled status
		scheduled := &storageModels.ScheduledStatus{
			ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
			Username:    claims.Username,
			Status:      req.Status,
			MediaIDs:    req.MediaIDs,
			Sensitive:   req.Sensitive,
			SpoilerText: req.SpoilerText,
			Visibility:  req.Visibility,
			Language:    req.Language,
			InReplyToID: req.InReplyToID,
			ScheduledAt: scheduledTime,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		// Store scheduled status using the status repository
		if err := h.repos.Status().CreateScheduledStatus(ctx.Context, scheduled); err != nil {
			h.logger.Error("failed to create scheduled status", zap.Error(err))
			return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to schedule status"})
		}

		// Return scheduled status response
		resp := map[string]interface{}{
			"id":           scheduled.ID,
			"scheduled_at": scheduled.ScheduledAt.Format(time.RFC3339),
			"params": map[string]interface{}{
				"text":           scheduled.Status,
				"visibility":     scheduled.Visibility,
				"media_ids":      scheduled.MediaIDs,
				"in_reply_to_id": scheduled.InReplyToID,
				"sensitive":      scheduled.Sensitive,
				"spoiler_text":   scheduled.SpoilerText,
				"language":       scheduled.Language,
			},
		}

		return ctx.JSON(resp)
	}

	// Get actor for the authenticated user
	actor, err := h.repos.Account().GetActor(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Default visibility
	if req.Visibility == "" {
		req.Visibility = "public"
	}

	// Create the note
	now := time.Now()
	noteID := fmt.Sprintf("%s/objects/%d-%s", h.cfg.BaseURL(), now.Unix(), generateRandomStringServiceFull())
	publishedTime := now
	
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        noteID,
			Type:      "Note",
			Published: &publishedTime,
			To:        []string{},
			CC:        []string{},
			InReplyTo: req.InReplyToID,
			Sensitive: req.Sensitive,
			Summary:   req.SpoilerText,
		},
		Content:      req.Status,
		AttributedTo: actor.ID,
		Visibility:   req.Visibility,
	}

	// Set recipients based on visibility
	publicAudience := "https://www.w3.org/ns/activitystreams#Public"
	switch req.Visibility {
	case "public":
		note.BaseObject.To = append(note.BaseObject.To, publicAudience)
		note.BaseObject.CC = append(note.BaseObject.CC, actor.Followers)
	case "unlisted":
		note.BaseObject.To = append(note.BaseObject.To, actor.Followers)
		note.BaseObject.CC = append(note.BaseObject.CC, publicAudience)
	case "private":
		note.BaseObject.To = append(note.BaseObject.To, actor.Followers)
	case "direct":
		// Direct messages need specific recipients
		if len(req.DirectRecipients) > 0 {
			note.BaseObject.To = req.DirectRecipients
		}
	}

	// Handle media attachments
	if len(req.MediaIDs) > 0 {
		attachments := []activitypub.Attachment{}
		for _, mediaID := range req.MediaIDs {
			media, err := h.repos.Media().GetMedia(ctx.Context, mediaID)
			if err != nil {
				h.logger.Error("failed to get media", zap.String("media_id", mediaID), zap.Error(err))
				continue
			}
			
			attachment := activitypub.Attachment{
				Type:      "Document",
				MediaType: media.MimeType,
				URL:       media.URL,
				Name:      media.Description,
			}
			attachments = append(attachments, attachment)
		}
		note.Attachment = attachments
	}

	// Handle poll if present
	if req.Poll != nil && len(req.Poll.Options) > 0 {
		poll := &storageModels.Poll{
			ID:         fmt.Sprintf("%d", time.Now().UnixNano()),
			StatusID:   noteID,
			ExpiresAt:  time.Now().Add(time.Duration(req.Poll.ExpiresIn) * time.Second),
			Multiple:   req.Poll.Multiple,
			HideTotals: req.Poll.HideTotals,
			Options:    []storageModels.PollOption{},
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		for i, optionText := range req.Poll.Options {
			poll.Options = append(poll.Options, storageModels.PollOption{
				ID:         fmt.Sprintf("%d-%d", time.Now().UnixNano(), i),
				Title:      optionText,
				VotesCount: 0,
			})
		}

		if err := h.repos.Poll().CreatePoll(ctx.Context, poll); err != nil {
			h.logger.Error("failed to create poll", zap.Error(err))
			// Continue without poll
		}
	}

	// Store the note
	if err := h.repos.Object().CreateNote(ctx.Context, note); err != nil {
		h.logger.Error("failed to create note", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to create status"})
	}

	// Create activity
	createActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   fmt.Sprintf("%s/activities/%d", h.cfg.BaseURL(), now.UnixNano()),
			Type: "Create",
		},
		Actor:     actor.ID,
		Object:    note,
		Published: &publishedTime,
		To:        note.BaseObject.To,
		CC:        note.BaseObject.CC,
	}

	if err := h.repos.Activity().CreateActivity(ctx.Context, createActivity); err != nil {
		h.logger.Error("failed to create activity", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Queue federation delivery for remote followers
	if req.Visibility == "public" || req.Visibility == "unlisted" {
		federationReq := &federation.DeliveryRequest{
			ActorUsername: claims.Username,
			Activity:      createActivity,
			Recipients:    h.getRemoteFollowers(ctx.Context, claims.Username),
		}
		
		if err := h.repos.Federation().QueueDelivery(ctx.Context, federationReq); err != nil {
			h.logger.Error("failed to queue federation delivery", zap.Error(err))
			// Continue - federation is best effort
		}
	}

	// Emit events for real-time streaming
	event := &streaming.Event{
		Type:      streaming.EventStatusCreated,
		Stream:    streaming.StreamUser,
		Payload:   note,
		Timestamp: now,
	}

	// Publish to user's stream
	if h.publisher != nil {
		if err := h.publisher.PublishToUser(ctx.Context, claims.Username, event); err != nil {
			h.logger.Error("failed to publish to user stream", zap.Error(err))
		}

		// Publish to public stream if public
		if req.Visibility == "public" {
			event.Stream = streaming.StreamPublic
			if err := h.publisher.PublishToStream(ctx.Context, streaming.StreamPublic, event); err != nil {
				h.logger.Error("failed to publish to public stream", zap.Error(err))
			}
		}
	}

	// Build and return response
	resp := h.buildFullStatusResponse(note, actor, req)
	ctx.Status(http.StatusCreated)
	return ctx.JSON(resp)
}

// HandleGetStatusServiceFull retrieves a status by ID
func (h *Handler) HandleGetStatusServiceFull(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Optional authentication for private status viewing
	var viewerUsername string
	token := h.getBearerTokenLift(ctx)
	if token != "" {
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
			viewerUsername = claims.Username
		}
	}

	// Get the note
	note, err := h.repos.Object().GetNote(ctx.Context, statusID)
	if err != nil {
		if err.Error() == "not found" {
			return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
		}
		h.logger.Error("failed to get note", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Check visibility permissions
	if !h.canViewNote(note, viewerUsername) {
		return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
	}

	// Get actor information
	actorID := note.AttributedTo
	actor, err := h.repos.Account().GetActorByID(ctx.Context, actorID)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		// Continue with minimal actor info
		actor = &activitypub.Actor{
			BaseObject: activitypub.BaseObject{ID: actorID},
		}
	}

	// Build response
	resp := h.buildFullStatusResponse(note, actor, models.CreateStatusRequest{})
	return ctx.JSON(resp)
}

// HandleDeleteStatusServiceFull deletes a status
func (h *Handler) HandleDeleteStatusServiceFull(ctx *lift.Context) error {
	// Authenticate user
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "missing token"})
	}

	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": err.Error()})
	}

	if !claims.HasScope(auth.ScopeWrite) {
		return ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "insufficient scope"})
	}

	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Get the note to verify ownership
	note, err := h.repos.Object().GetNote(ctx.Context, statusID)
	if err != nil {
		if err.Error() == "not found" {
			return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
		}
		h.logger.Error("failed to get note", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Get actor to verify ownership
	actor, err := h.repos.Account().GetActor(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Check if user owns the status
	if note.AttributedTo != actor.ID {
		return ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "not authorized to delete this status"})
	}

	// Create Delete activity
	now := time.Now()
	deleteActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   fmt.Sprintf("%s/activities/%d", h.cfg.BaseURL(), now.UnixNano()),
			Type: "Delete",
		},
		Actor:     actor.ID,
		Object:    note.ID,
		Published: &now,
		To:        note.BaseObject.To,
		CC:        note.BaseObject.CC,
	}

	// Store the delete activity
	if err := h.repos.Activity().CreateActivity(ctx.Context, deleteActivity); err != nil {
		h.logger.Error("failed to create delete activity", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Delete the note
	if err := h.repos.Object().DeleteNote(ctx.Context, statusID); err != nil {
		h.logger.Error("failed to delete note", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to delete status"})
	}

	// Queue federation delivery for the Delete activity
	federationReq := &federation.DeliveryRequest{
		ActorUsername: claims.Username,
		Activity:      deleteActivity,
		Recipients:    h.getRemoteFollowers(ctx.Context, claims.Username),
	}
	
	if err := h.repos.Federation().QueueDelivery(ctx.Context, federationReq); err != nil {
		h.logger.Error("failed to queue federation delivery", zap.Error(err))
		// Continue - federation is best effort
	}

	// Emit delete event
	if h.publisher != nil {
		event := &streaming.Event{
			Type:      streaming.EventStatusDeleted,
			Stream:    streaming.StreamUser,
			Payload:   map[string]string{"id": statusID},
			Timestamp: now,
		}
		
		if err := h.publisher.PublishToUser(ctx.Context, claims.Username, event); err != nil {
			h.logger.Error("failed to publish delete event", zap.Error(err))
		}
	}

	// Return the deleted status
	resp := h.buildFullStatusResponse(note, actor, models.CreateStatusRequest{})
	return ctx.JSON(resp)
}

// Helper methods

func (h *Handler) canViewNote(note *activitypub.Note, viewerUsername string) bool {
	// Public notes are always viewable
	publicAudience := "https://www.w3.org/ns/activitystreams#Public"
	for _, to := range note.BaseObject.To {
		if to == publicAudience {
			return true
		}
	}
	for _, cc := range note.BaseObject.CC {
		if cc == publicAudience {
			return true
		}
	}

	// If no viewer, can't see non-public notes
	if viewerUsername == "" {
		return false
	}

	// Check if viewer is the author
	actor, err := h.repos.Account().GetActor(context.Background(), viewerUsername)
	if err == nil && actor.ID == note.AttributedTo {
		return true
	}

	// Check if viewer is in recipients
	viewerID := fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), viewerUsername)
	for _, to := range note.BaseObject.To {
		if to == viewerID {
			return true
		}
	}
	for _, cc := range note.BaseObject.CC {
		if cc == viewerID {
			return true
		}
	}

	return false
}

func (h *Handler) getRemoteFollowers(ctx context.Context, username string) []string {
	followers, err := h.repos.Follow().GetFollowers(ctx, username, 1000, "")
	if err != nil {
		return []string{}
	}

	remote := []string{}
	for _, follower := range followers {
		if !strings.Contains(follower.ID, h.cfg.BaseURL()) {
			remote = append(remote, follower.ID)
		}
	}
	return remote
}

func (h *Handler) buildFullStatusResponse(note *activitypub.Note, actor *activitypub.Actor, req models.CreateStatusRequest) map[string]interface{} {
	// Extract visibility from recipients
	publicAudience := "https://www.w3.org/ns/activitystreams#Public"
	visibility := "public"
	if len(note.BaseObject.To) > 0 {
		if note.BaseObject.To[0] == publicAudience {
			visibility = "public"
		} else if note.BaseObject.To[0] == actor.Followers {
			visibility = "private"
		} else {
			visibility = "direct"
		}
	}

	// Build account info
	account := map[string]interface{}{
		"id":              actor.ID,
		"username":        actor.PreferredUsername,
		"acct":            actor.PreferredUsername,
		"display_name":    actor.Name,
		"locked":          actor.ManuallyApprovesFollowers,
		"bot":             actor.Type == "Service",
		"created_at":      time.Now().Format(time.RFC3339), // Use current time as fallback
		"note":            actor.Summary,
		"url":             actor.URL,
		"avatar":          actor.Icon.URL,
		"header":          actor.Image.URL,
		"followers_count": actor.FollowersCount,
		"following_count": actor.FollowingCount,
		"statuses_count":  0,
	}

	// Build media attachments
	mediaAttachments := []interface{}{}
	for _, attachment := range note.Attachment {
		mediaAttachments = append(mediaAttachments, map[string]interface{}{
			"id":   attachment.URL,
			"type": attachment.Type,
			"url":  attachment.URL,
			"meta": map[string]interface{}{},
		})
	}

	// Build status response
	resp := map[string]interface{}{
		"id":                     note.ID,
		"created_at":             note.BaseObject.Published.Format(time.RFC3339),
		"in_reply_to_id":         note.BaseObject.InReplyTo,
		"in_reply_to_account_id": nil,
		"sensitive":              note.BaseObject.Sensitive,
		"spoiler_text":           note.BaseObject.Summary,
		"visibility":             visibility,
		"language":               req.Language,
		"uri":                    note.ID,
		"url":                    note.ID, // Use ID as URL for now
		"replies_count":          0,
		"reblogs_count":          0,
		"favourites_count":       0,
		"edited_at":              nil,
		"content":                note.Content,
		"reblog":                 nil,
		"account":                account,
		"media_attachments":      mediaAttachments,
		"mentions":               []interface{}{},
		"tags":                   []interface{}{},
		"emojis":                 []interface{}{},
		"card":                   nil,
		"poll":                   nil,
		"application":            nil,
		"favourited":             false,
		"reblogged":              false,
		"muted":                  false,
		"bookmarked":             false,
		"pinned":                 false,
		"filtered":               []interface{}{},
	}

	return resp
}

// generateRandomStringServiceFull generates a random string for IDs
// Using a different name to avoid conflicts with existing function
func generateRandomStringServiceFull() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}