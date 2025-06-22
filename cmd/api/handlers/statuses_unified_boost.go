package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleUnifiedReblog handles both traditional boosts and quote boosts
func (h *Handler) HandleUnifiedReblog(ctx context.Context, request events.APIGatewayV2HTTPRequest, statusID string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Parse request body if present
	var req models.ReblogRequest
	if request.Body != "" {
		if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
			// Treat as traditional boost if can't parse
			req = models.ReblogRequest{}
		}
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Check if this is a quote boost
	if req.Comment != nil && *req.Comment != "" {
		// Create a quote boost
		return h.createQuoteBoost(ctx, statusID, objectID, *req.Comment, req.Visibility, claims, actor)
	}

	// Traditional boost - create Announce activity
	return h.createPureBoost(ctx, statusID, objectID, actor)
}

// createPureBoost creates a traditional ActivityPub Announce
func (h *Handler) createPureBoost(ctx context.Context, statusID, objectID string, actor *activitypub.Actor) (*events.APIGatewayV2HTTPResponse, error) {
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

// createQuoteBoost creates a new status with a quote relationship
func (h *Handler) createQuoteBoost(ctx context.Context, statusID, objectID, comment, visibility string, claims *auth.Claims, actor *activitypub.Actor) (*events.APIGatewayV2HTTPResponse, error) {
	// Default visibility if not specified
	if visibility == "" {
		visibility = "public"
	}

	// Generate note ID
	noteID := fmt.Sprintf("%d-%s", time.Now().Unix(), generateRandomString(8))

	// Create the QuoteNote object with quote content
	note := &activitypub.QuoteNote{
		Note: activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), noteID),
				Type: "Note",
			},
			Content:      comment,
			AttributedTo: actor.ID,
		},
		QuoteURL:           objectID,
		Quoteable:          true,
		QuoteNotifications: true,
	}

	// Set timestamps
	now := time.Now()
	note.Published = &now

	// Set addressing based on visibility
	switch visibility {
	case "public":
		note.To = []string{activitypub.PublicAddress}
		note.CC = []string{actor.Followers}
	case "unlisted":
		note.To = []string{actor.Followers}
		note.CC = []string{activitypub.PublicAddress}
	case "private":
		note.To = []string{actor.Followers}
	case "direct":
		note.To = []string{}
	}

	// Create the Note object
	if err := h.store.CreateObject(ctx, note); err != nil {
		h.logger.Error("failed to create quote note object", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Create quote relationship
	quoteRelationship := &storage.QuoteRelationship{
		ID:           fmt.Sprintf("%s-quotes-%s", noteID, statusID),
		QuoterNoteID: noteID,
		TargetNoteID: objectID,
		QuoterID:     actor.ID,
		Timestamp:    now,
	}

	if err := h.store.CreateQuoteRelationship(ctx, quoteRelationship); err != nil {
		h.logger.Error("failed to create quote relationship", zap.Error(err))
		// Don't fail the request - the note is already created
	}

	// Increment reblog count on the quoted status (unified counting)
	if err := h.store.IncrementReblogCount(ctx, objectID); err != nil {
		h.logger.Warn("failed to increment reblog count for quote",
			zap.String("quoted_status_id", objectID),
			zap.Error(err))
	}

	// Create a Create activity for federation
	createActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      activitypub.CreateType,
			ID:        fmt.Sprintf("%s/activities/create-%d-%s", actor.ID, time.Now().Unix(), generateRandomString(8)),
			To:        note.To,
			CC:        note.CC,
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
		h.logger.Error("failed to fan out quote boost to timelines", zap.Error(err))
	}

	// Record engagement for trending
	if err := h.store.RecordStatusEngagement(ctx, objectID, "quote", actor.ID); err != nil {
		h.logger.Warn("failed to record quote engagement",
			zap.String("quoted_status_id", objectID),
			zap.Error(err))
	}

	// Get the quoted status to include in response
	var quotedStatus *models.Status
	if _, getErr := h.store.GetObject(ctx, objectID); getErr == nil {
		// Convert to status (simplified for now)
		// TODO: properly convert quotedObj to models.Status
		quotedStatus = &models.Status{
			ID:      statusID,
			URI:     objectID,
			URL:     objectID,
			Content: "", // Would be populated from object
		}
	}

	// Build response
	resp := models.Status{
		ID:               noteID,
		CreatedAt:        now.Format("2006-01-02T15:04:05.000Z"),
		Sensitive:        false,
		SpoilerText:      "",
		Visibility:       visibility,
		Language:         "en",
		URI:              note.ID,
		URL:              note.ID,
		RepliesCount:     0,
		ReblogsCount:     0,
		FavouritesCount:  0,
		Favourited:       false,
		Reblogged:        false,
		Muted:            false,
		Bookmarked:       false,
		Pinned:           false,
		Content:          comment,
		IsQuoteBoost:     true,
		QuotedStatus:     quotedStatus,
		QuotedStatusID:   &objectID,
		MediaAttachments: []interface{}{},
		Mentions:         []interface{}{},
		Tags:             []interface{}{},
		Emojis:           []interface{}{},
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

	body, _ := json.Marshal(resp)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}