package lift

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/emoji"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// Visibility constants
const (
	VisibilityPublic   = "public"
	VisibilityUnlisted = "unlisted"
	VisibilityPrivate  = "private"
	VisibilityDirect   = "direct"
)

// HandleCreateStatusLift creates a new status (post)
func (h *Handler) HandleCreateStatusLift(ctx *lift.Context) error {
	// Authenticate and validate request
	claims, err := h.authenticateAndValidateWriteScope(ctx)
	if err != nil {
		return err
	}

	// Parse and validate request
	req, err := h.parseCreateStatusRequest(ctx)
	if err != nil {
		return err
	}

	// Handle scheduled status
	if h.isScheduledStatus(req) {
		return h.HandleScheduleStatusLift(ctx, claims, req)
	}

	// Get actor and create note
	actor, err := h.getAuthenticatedActor(ctx, claims)
	if err != nil {
		return err
	}

	now := time.Now()
	note, hashtags := h.createNoteFromRequest(req, actor, now)

	// Handle poll creation
	pollResp, err := h.createPollIfRequested(ctx, req, note.ID, actor.ID)
	if err != nil {
		return err
	}

	// Process content and create objects
	parsedEmojis, err := h.processContentAndCreateNote(ctx, note)
	if err != nil {
		return err
	}

	// Handle reply processing
	h.handleReplyProcessing(ctx, req)

	// Create and store activity
	createActivity := h.createActivity(actor, note, now)
	if err := h.repos.Activity().CreateActivity(ctx.Context, createActivity); err != nil {
		h.logger.Error("failed to create activity", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Post-creation processing
	h.performPostCreationTasks(ctx, createActivity, req, hashtags, actor, claims, now)

	// Build and return response
	resp := h.buildStatusResponse(note, req, actor, parsedEmojis, pollResp, now)
	ctx.Status(http.StatusCreated)
	return ctx.JSON(resp)
}

// authenticateAndValidateWriteScope extracts and validates the bearer token and checks write scope
func (h *Handler) authenticateAndValidateWriteScope(ctx *lift.Context) (*auth.Claims, error) {
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		return nil, ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "missing token"})
	}

	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return nil, ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": err.Error()})
	}

	if !claims.HasScope(auth.ScopeWrite) {
		return nil, ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "insufficient scope"})
	}

	return claims, nil
}

// parseCreateStatusRequest parses and validates the create status request
func (h *Handler) parseCreateStatusRequest(ctx *lift.Context) (models.CreateStatusRequest, error) {
	var req models.CreateStatusRequest

	contentType := ctx.Header("Content-Type")
	if contentType == "" {
		contentType = ctx.Header("content-type")
	}

	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		if err := h.parseFormRequest(ctx, &req); err != nil {
			return req, err
		}
	} else {
		if err := ctx.ParseRequest(&req); err != nil {
			h.logger.Error("failed to parse create status request", zap.Error(err))
			return req, ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request format"})
		}
	}

	return h.validateStatusRequest(ctx, req)
}

// parseFormRequest parses form-encoded request data
func (h *Handler) parseFormRequest(ctx *lift.Context, req *models.CreateStatusRequest) error {
	body := string(ctx.Request.Body)
	params, err := common.ParseFormURLEncoded(body)
	if err != nil {
		h.logger.Error("failed to parse form data", zap.Error(err))
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid form data"})
	}

	req.Status = params["status"]
	req.InReplyToID = params["in_reply_to_id"]
	req.Visibility = params["visibility"]
	req.Sensitive = params["sensitive"] == boolTrue
	req.SpoilerText = params["spoiler_text"]
	req.Language = params["language"]

	if mediaIDs := params["media_ids[]"]; mediaIDs != "" {
		req.MediaIDs = strings.Split(mediaIDs, ",")
	}

	if scheduledAt := params["scheduled_at"]; scheduledAt != "" {
		req.ScheduledAt = &scheduledAt
	}

	return nil
}

// validateStatusRequest validates the parsed status request
func (h *Handler) validateStatusRequest(ctx *lift.Context, req models.CreateStatusRequest) (models.CreateStatusRequest, error) {
	if req.Status == "" {
		return req, ctx.Status(http.StatusUnprocessableEntity).JSON(map[string]string{"error": "status content is required"})
	}

	if len(req.Status) > 500 {
		return req, ctx.Status(http.StatusUnprocessableEntity).JSON(map[string]string{"error": "status content must not exceed 500 characters"})
	}

	return req, nil
}

// isScheduledStatus checks if this is a scheduled status
func (h *Handler) isScheduledStatus(req models.CreateStatusRequest) bool {
	return req.ScheduledAt != nil && *req.ScheduledAt != ""
}

// getAuthenticatedActor gets the actor for the authenticated user
func (h *Handler) getAuthenticatedActor(ctx *lift.Context, claims *auth.Claims) (*activitypub.Actor, error) {
	actor, err := h.repos.Account().GetActor(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return nil, ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}
	return actor, nil
}

// createNoteFromRequest creates a Note object from the request
func (h *Handler) createNoteFromRequest(req models.CreateStatusRequest, actor *activitypub.Actor, now time.Time) (*activitypub.Note, []string) {
	if req.Visibility == "" {
		req.Visibility = storageModels.VisibilityPublic
	}

	noteID := fmt.Sprintf("%s/objects/%d-%s", h.cfg.BaseURL(), now.Unix(), generateRandomStringLift())
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
		Visibility:   req.Visibility,
	}

	note.Published = &now

	// Process hashtags
	hashtags := mastodon.ExtractHashtagsWithCase(req.Status)
	if len(hashtags) > 0 {
		note.Tag = h.createHashtagTags(hashtags)
	}

	// Process media attachments
	if len(req.MediaIDs) > 0 {
		attachments := make([]activitypub.Attachment, 0, len(req.MediaIDs))
		for _, mediaID := range req.MediaIDs {
			if attachment, err := h.processMediaAttachment(context.Background(), mediaID); err == nil {
				attachments = append(attachments, attachment)
			}
		}
		note.Attachment = attachments
	}

	// Set addressing
	h.setNoteAddressing(note, req.Visibility, actor)

	// Handle reply
	if req.InReplyToID != "" {
		note.InReplyTo = req.InReplyToID
	}

	return note, hashtags
}

// createHashtagTags creates ActivityPub tags from hashtags
func (h *Handler) createHashtagTags(hashtags []string) []activitypub.Tag {
	tags := make([]activitypub.Tag, 0, len(hashtags))
	for _, tag := range hashtags {
		normalizedTag := mastodon.NormalizeHashtag(tag)
		tagURL := fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), normalizedTag)

		tags = append(tags, activitypub.Tag{
			Type: "Hashtag",
			Name: "#" + tag,
			Href: tagURL,
		})
	}
	return tags
}

// processMediaAttachment processes a single media attachment
func (h *Handler) processMediaAttachment(ctx context.Context, mediaID string) (activitypub.Attachment, error) {
	mediaObj, err := h.repos.Object().GetObject(ctx, fmt.Sprintf("MEDIA#%s", mediaID))
	if err != nil {
		h.logger.Warn("failed to get media metadata", zap.String("media_id", mediaID), zap.Error(err))
		return activitypub.Attachment{}, err
	}

	if mediaMap, ok := mediaObj.(map[string]any); ok {
		attachment := activitypub.Attachment{
			Type:      "Document",
			MediaType: getStringFromMap(mediaMap, "MimeType", "image/jpeg"),
			URL:       getStringFromMap(mediaMap, "URL", ""),
			Name:      getStringFromMap(mediaMap, "Description", ""),
		}
		return attachment, nil
	}

	return activitypub.Attachment{}, fmt.Errorf("invalid media object")
}

// setNoteAddressing sets the addressing fields based on visibility
func (h *Handler) setNoteAddressing(note *activitypub.Note, visibility string, actor *activitypub.Actor) {
	switch visibility {
	case VisibilityPublic:
		note.To = []string{activitypub.PublicAddress}
		note.CC = []string{actor.Followers}
	case VisibilityUnlisted:
		note.To = []string{actor.Followers}
		note.CC = []string{activitypub.PublicAddress}
	case VisibilityPrivate:
		note.To = []string{actor.Followers}
	case VisibilityDirect:
		mentions := h.extractMentions(note.Content)
		note.To = mentions
	}
}

// createPollIfRequested creates a poll if requested in the status
func (h *Handler) createPollIfRequested(ctx *lift.Context, req models.CreateStatusRequest, noteID, actorID string) (*models.Poll, error) {
	if req.Poll == nil || len(req.Poll.Options) == 0 {
		return nil, nil
	}

	if len(req.Poll.Options) < 2 || len(req.Poll.Options) > 4 {
		return nil, ctx.Status(http.StatusUnprocessableEntity).JSON(map[string]string{"error": "poll must have between 2 and 4 options"})
	}

	expiresAt := time.Now().Add(time.Duration(req.Poll.ExpiresIn) * time.Second)
	poll := &storage.Poll{
		StatusID:   noteID,
		CreatedBy:  actorID,
		Options:    req.Poll.Options,
		Multiple:   req.Poll.Multiple,
		HideTotals: req.Poll.HideTotals,
		ExpiresAt:  &expiresAt,
	}

	if err := h.repos.Poll().CreatePoll(ctx.Context, poll); err != nil {
		h.logger.Error("failed to create poll", zap.Error(err))
		return nil, ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	return h.buildPollResponse(poll), nil
}

// buildPollResponse builds the poll response for the API
func (h *Handler) buildPollResponse(poll *storage.Poll) *models.Poll {
	optionsData := make([]models.PollOption, len(poll.Options))
	for i, option := range poll.Options {
		optionsData[i] = models.PollOption{
			Title:      option,
			VotesCount: 0,
		}
	}

	return &models.Poll{
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

// processContentAndCreateNote processes content and creates the note object
func (h *Handler) processContentAndCreateNote(ctx *lift.Context, note *activitypub.Note) ([]mastodon.ParsedEmoji, error) {
	// Parse and process custom emojis
	emojiParser := mastodon.NewEmojiParser(h.repos)
	processedContent, parsedEmojis, err := emojiParser.ProcessContent(ctx.Context, note.Content)
	if err != nil {
		h.logger.Warn("failed to parse emojis in content", zap.Error(err))
		parsedEmojis = nil
	} else {
		note.Content = processedContent
	}

	// Create the Note object
	if err := h.repos.Object().CreateObject(ctx.Context, note); err != nil {
		h.logger.Error("failed to create note object", zap.Error(err))
		return nil, ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	return parsedEmojis, nil
}

// handleReplyProcessing handles reply-related processing
func (h *Handler) handleReplyProcessing(ctx *lift.Context, req models.CreateStatusRequest) {
	if req.InReplyToID == "" {
		return
	}

	// Record reply engagement
	if actor, err := h.repos.Account().GetActor(ctx.Context, ""); err == nil {
		if err := h.repos.Analytics().RecordStatusEngagement(ctx.Context, req.InReplyToID, "reply", actor.ID); err != nil {
			h.logger.Warn("failed to record reply engagement",
				zap.String("parent_status_id", req.InReplyToID),
				zap.Error(err))
		}
	}

	// Increment reply count
	if err := h.repos.Object().IncrementReplyCount(ctx.Context, req.InReplyToID); err != nil {
		h.logger.Warn("failed to increment reply count",
			zap.String("parent_id", req.InReplyToID),
			zap.Error(err))
	}
}

// createActivity creates the Create activity for the note
func (h *Handler) createActivity(actor *activitypub.Actor, note *activitypub.Note, now time.Time) *activitypub.Activity {
	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      activitypub.CreateType,
			ID:        fmt.Sprintf("%s/activities/create-%d-%s", actor.ID, now.Unix(), generateRandomStringLift()),
			To:        note.To,
			Published: &now,
		},
		Actor:  actor.ID,
		Object: note,
	}
}

// performPostCreationTasks performs all tasks after creating the activity
func (h *Handler) performPostCreationTasks(ctx *lift.Context, activity *activitypub.Activity, req models.CreateStatusRequest, hashtags []string, actor *activitypub.Actor, claims *auth.Claims, now time.Time) {
	// Fan out the post
	if err := h.repos.User().FanOutPost(ctx.Context, activity); err != nil {
		h.logger.Error("failed to fan out post to timelines", zap.Error(err))
	}

	// Record status creation activity
	if err := h.repos.Activity().RecordActivity(ctx.Context, "status", actor.ID, now); err != nil {
		h.logger.Warn("failed to record status activity", zap.Error(err))
	}

	// Record hashtags for trending
	h.recordHashtagUsage(ctx, hashtags, activity.Object.(*activitypub.Note).ID, actor.ID)

	// Record links for trending
	h.recordLinkUsage(ctx, req.Status, activity.Object.(*activitypub.Note).ID, actor.ID)

	// Update actor's last status time
	if err := h.repos.Actor().UpdateActorLastStatusTime(ctx.Context, claims.Username); err != nil {
		h.logger.Warn("failed to update actor last status time", zap.Error(err))
	}

	// Federation: Deliver Create activity to relevant recipients
	go func() {
		if err := h.deliverCreateActivity(context.Background(), activity, actor, req.Visibility); err != nil {
			h.logger.Error("failed to deliver create activity for federation",
				zap.String("activity_id", activity.ID),
				zap.Error(err))
		}
	}()
}

// recordHashtagUsage records hashtag usage for trending
func (h *Handler) recordHashtagUsage(ctx *lift.Context, hashtags []string, noteID, actorID string) {
	for _, hashtag := range hashtags {
		cleanHashtag := strings.TrimPrefix(hashtag, "#")
		if err := h.repos.Analytics().RecordHashtagUsage(ctx.Context, cleanHashtag, noteID, actorID); err != nil {
			h.logger.Warn("failed to record hashtag usage",
				zap.String("hashtag", cleanHashtag),
				zap.Error(err))
		}
	}
}

// recordLinkUsage records link usage for trending
func (h *Handler) recordLinkUsage(ctx *lift.Context, content, noteID, actorID string) {
	links := extractLinksFromContent(content)
	for _, link := range links {
		if err := h.repos.Analytics().RecordLinkShare(ctx.Context, link, noteID, actorID); err != nil {
			h.logger.Warn("failed to record link share",
				zap.String("link", link),
				zap.Error(err))
		}
	}
}

// buildStatusResponse builds the complete status response
func (h *Handler) buildStatusResponse(note *activitypub.Note, req models.CreateStatusRequest, actor *activitypub.Actor, parsedEmojis []mastodon.ParsedEmoji, pollResp *models.Poll, now time.Time) models.Status {
	mastodonTags := h.convertTagsToMastodonFormat(note.Tag)
	mastodonEmojis := h.convertEmojisToMastodonFormat(parsedEmojis)

	resp := models.Status{
		ID:               note.ID,
		CreatedAt:        note.Published.Format("2006-01-02T15:04:05.000Z"),
		Sensitive:        note.Sensitive,
		SpoilerText:      note.Summary,
		Visibility:       req.Visibility,
		Language:         req.Language,
		URI:              note.ID,
		URL:              note.ID,
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
		Account:          h.buildAccountFromActor(actor, now),
	}

	if req.InReplyToID != "" {
		resp.InReplyToID = &req.InReplyToID
		if accountID := h.getReplyToAccountID(context.Background(), req.InReplyToID); accountID != "" {
			resp.InReplyToAccountID = &accountID
		}
	}

	return resp
}

// convertTagsToMastodonFormat converts ActivityPub tags to Mastodon format
func (h *Handler) convertTagsToMastodonFormat(tags []activitypub.Tag) []any {
	mastodonTags := make([]any, 0, len(tags))
	for _, tag := range tags {
		if tag.Type == "Hashtag" {
			tagName := strings.TrimPrefix(tag.Name, "#")
			mastodonTags = append(mastodonTags, map[string]any{
				"name": tagName,
				"url":  tag.Href,
			})
		}
	}
	return mastodonTags
}

// convertEmojisToMastodonFormat converts parsed emojis to Mastodon format
func (h *Handler) convertEmojisToMastodonFormat(parsedEmojis []mastodon.ParsedEmoji) []any {
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
	return mastodonEmojis
}

// buildAccountFromActor builds an account object from an actor
func (h *Handler) buildAccountFromActor(actor *activitypub.Actor, now time.Time) models.Account {
	account := models.Account{
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
	}

	if actor.Icon != nil && actor.Icon.URL != "" {
		account.Avatar = actor.Icon.URL
		account.AvatarStatic = actor.Icon.URL
	}

	if actor.Image != nil && actor.Image.URL != "" {
		account.Header = actor.Image.URL
		account.HeaderStatic = actor.Image.URL
	}

	return account
}

// authenticateDeleteRequest handles token validation and authorization for delete requests
func (h *Handler) authenticateDeleteRequest(ctx *lift.Context) (*auth.Claims, *activitypub.Actor, error) {
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		return nil, nil, ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "missing token"})
	}

	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return nil, nil, ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": err.Error()})
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return nil, nil, ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Get the user's actor
	actor, err := h.repos.Account().GetActor(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return nil, nil, ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	return claims, actor, nil
}

// validateAndGetObject handles object retrieval and tombstone checking
func (h *Handler) validateAndGetObject(ctx *lift.Context, objectID string) (interface{}, error) {
	object, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		// Check if this is a tombstoned object (should return 410 Gone)
		if isTombstoned, tombErr := h.repos.Object().IsTombstoned(ctx.Context, objectID); tombErr == nil && isTombstoned {
			// Get tombstone details for better error message
			if tombstone, tErr := h.repos.Object().GetTombstone(ctx.Context, objectID); tErr == nil {
				return nil, ctx.Status(http.StatusGone).JSON(map[string]interface{}{
					"error": "status already deleted",
					"error_description": "This status has already been deleted",
					"deleted_at": tombstone.Deleted.Format(time.RFC3339),
					"former_type": tombstone.FormerType,
				})
			}
			// Fallback if we can't get tombstone details
			return nil, ctx.Status(http.StatusGone).JSON(map[string]string{
				"error": "status already deleted",
				"error_description": "This status has already been deleted",
			})
		}
		// Regular 404 for genuinely missing objects
		return nil, ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
	}
	return object, nil
}

// extractAttributedToWithError extracts the AttributedTo field with error handling
func (h *Handler) extractAttributedToWithError(ctx *lift.Context, object interface{}) (string, error) {
	attributedTo := h.extractAttributedTo(object)
	if attributedTo == "" {
		// Try reflection for unknown types
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
			return "", ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "unexpected object type"})
		}
	}
	return attributedTo, nil
}

// executeDeleteOperation performs the actual deletion and federation
func (h *Handler) executeDeleteOperation(ctx *lift.Context, objectID string, actor *activitypub.Actor, object interface{}) error {
	// Create a Delete activity
	deleteActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.DeleteType,
			ID:      fmt.Sprintf("%s/activities/delete-%d-%s", actor.ID, time.Now().Unix(), generateRandomStringLift()),
			To:      []string{activitypub.PublicAddress},
		},
		Actor:  actor.ID,
		Object: objectID,
	}
	now := time.Now()
	deleteActivity.Published = &now

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.repos.Activity().CreateActivity(ctx.Context, deleteActivity); err != nil {
		h.logger.Error("failed to create delete activity", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Perform cascade deletion operations before tombstoning
	if err := h.performLocalCascadeDeletion(ctx.Context, objectID, actor.ID); err != nil {
		h.logger.Warn("failed to perform complete cascade deletion",
			zap.String("object_id", objectID),
			zap.Error(err))
		// Continue - partial failure is acceptable
	}

	// Tombstone the object from storage
	if err := h.repos.Object().TombstoneObject(ctx.Context, objectID, actor.ID); err != nil {
		h.logger.Error("failed to tombstone object", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Federation: Deliver Delete activity to relevant recipients
	go func() {
		if err := h.deliverDeleteActivity(context.Background(), deleteActivity, actor, object); err != nil {
			h.logger.Error("failed to deliver delete activity for federation",
				zap.String("activity_id", deleteActivity.ID),
				zap.Error(err))
		}
	}()

	return nil
}

// HandleDeleteStatusLift deletes a status
func (h *Handler) HandleDeleteStatusLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Authenticate and authorize the request
	_, actor, err := h.authenticateDeleteRequest(ctx)
	if err != nil {
		return err
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Get the object and handle tombstone cases
	object, err := h.validateAndGetObject(ctx, objectID)
	if err != nil {
		return err
	}

	// Extract and verify ownership
	attributedTo, err := h.extractAttributedToWithError(ctx, object)
	if err != nil {
		return err
	}

	if attributedTo != actor.ID {
		return ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "you can only delete your own statuses"})
	}

	// Execute the deletion operation
	if err := h.executeDeleteOperation(ctx, objectID, actor, object); err != nil {
		return err
	}

	h.logger.Info("successfully deleted status with federation",
		zap.String("status_id", statusID),
		zap.String("object_id", objectID),
		zap.String("actor", actor.ID))

	// Return empty JSON object for successful deletion
	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{})
}

// HandleUpdateStatusLift updates an existing status
func (h *Handler) HandleUpdateStatusLift(ctx *lift.Context) error {
	// Validate status ID
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Authenticate and authorize user
	_, actor, err := h.authenticateStatusUpdate(ctx)
	if err != nil {
		return err
	}

	// Get and verify object ownership
	objectID := h.normalizeStatusIDForUpdate(statusID)
	object, err := h.getAndVerifyStatusOwnership(ctx, objectID, actor.ID)
	if err != nil {
		return err
	}

	// Parse update request
	req, err := h.parseUpdateStatusRequest(ctx)
	if err != nil {
		return err
	}

	// Convert object to Note and verify ownership
	note, err := h.convertObjectToNoteWithOwnershipCheck(ctx, object, actor.ID)
	if err != nil {
		return err
	}

	// Apply updates to note
	h.applyStatusUpdates(note, req)

	// Save updated note
	if err := h.saveUpdatedStatus(ctx, note); err != nil {
		return err
	}

	// Create and deliver update activity
	if err := h.createStatusUpdateActivity(ctx, note, actor); err != nil {
		return err
	}

	// Build and return response
	return h.buildUpdateStatusResponse(ctx, note, actor, req)
}

// authenticateStatusUpdate handles authentication for status updates
func (h *Handler) authenticateStatusUpdate(ctx *lift.Context) (*auth.Claims, *activitypub.Actor, error) {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		return nil, nil, ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "missing token"})
	}

	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return nil, nil, ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": err.Error()})
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return nil, nil, ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Get the user's actor
	actor, err := h.repos.Account().GetActor(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return nil, nil, ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	return claims, actor, nil
}

// normalizeStatusIDForUpdate normalizes a status ID to a full URL
func (h *Handler) normalizeStatusIDForUpdate(statusID string) string {
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		return fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}
	return statusID
}

// getAndVerifyStatusOwnership retrieves the object and initial ownership check
func (h *Handler) getAndVerifyStatusOwnership(ctx *lift.Context, objectID, _ string) (any, error) {
	object, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		// Check if this is a tombstoned object (should return 410 Gone)
		if isTombstoned, tombErr := h.repos.Object().IsTombstoned(ctx.Context, objectID); tombErr == nil && isTombstoned {
			// Get tombstone details for better error message
			if tombstone, tErr := h.repos.Object().GetTombstone(ctx.Context, objectID); tErr == nil {
				return nil, ctx.Status(http.StatusGone).JSON(map[string]interface{}{
					"error": "status deleted",
					"error_description": "This status has been deleted",
					"deleted_at": tombstone.Deleted.Format(time.RFC3339),
					"former_type": tombstone.FormerType,
				})
			}
			// Fallback if we can't get tombstone details
			return nil, ctx.Status(http.StatusGone).JSON(map[string]string{
				"error": "status deleted",
				"error_description": "This status has been deleted",
			})
		}
		// Regular 404 for genuinely missing objects
		return nil, ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
	}
	return object, nil
}

// parseUpdateStatusRequest parses the update status request
func (h *Handler) parseUpdateStatusRequest(ctx *lift.Context) (*models.UpdateStatusRequest, error) {
	var req models.UpdateStatusRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return nil, ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request format"})
	}
	return &req, nil
}

// convertObjectToNoteWithOwnershipCheck converts object to Note and verifies ownership
func (h *Handler) convertObjectToNoteWithOwnershipCheck(ctx *lift.Context, object any, actorID string) (*activitypub.Note, error) {
	switch obj := object.(type) {
	case *activitypub.Note:
		if obj.AttributedTo != actorID {
			return nil, ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "you can only update your own statuses"})
		}
		return obj, nil

	case map[string]any:
		if attr, ok := obj["attributedTo"].(string); ok && attr != actorID {
			return nil, ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "you can only update your own statuses"})
		}
		return h.convertMapToNote(ctx, obj)

	default:
		return h.convertUnknownObjectToNote(ctx, object, actorID)
	}
}

// convertMapToNote converts a map to a Note
func (h *Handler) convertMapToNote(ctx *lift.Context, obj map[string]any) (*activitypub.Note, error) {
	noteBytes, err := json.Marshal(obj)
	if err != nil {
		h.logger.Error("failed to marshal object to JSON", zap.Error(err))
		return nil, ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	note := &activitypub.Note{}
	if err := json.Unmarshal(noteBytes, note); err != nil {
		h.logger.Error("failed to unmarshal JSON to Note", zap.Error(err))
		return nil, ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	return note, nil
}

// convertUnknownObjectToNote handles conversion of unknown object types to Note
func (h *Handler) convertUnknownObjectToNote(ctx *lift.Context, object any, actorID string) (*activitypub.Note, error) {
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
		return nil, ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "unexpected object type"})
	}

	if attributedTo != actorID {
		return nil, ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "you can only update your own statuses"})
	}

	// Convert to Note via JSON marshaling
	noteBytes, _ := json.Marshal(object)
	note := &activitypub.Note{}
	if err := json.Unmarshal(noteBytes, note); err != nil {
		return nil, ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to convert object to Note"})
	}

	return note, nil
}

// applyStatusUpdates applies the requested updates to the note
func (h *Handler) applyStatusUpdates(note *activitypub.Note, req *models.UpdateStatusRequest) {
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
}

// saveUpdatedStatus saves the updated status to storage with edit history
func (h *Handler) saveUpdatedStatus(ctx *lift.Context, note *activitypub.Note) error {
	// Extract the username from the authentication token
	username := h.extractUsernameFromToken(ctx)
	if username == "" {
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to extract username for edit tracking"})
	}

	// Build actor ID for edit tracking
	actorID := fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), username)

	// Update object with history tracking using the new method
	if err := h.repos.Object().UpdateObjectWithHistory(ctx.Context, note, actorID); err != nil {
		h.logger.Error("failed to update object with history", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}
	return nil
}

// createStatusUpdateActivity creates and stores the update activity
func (h *Handler) createStatusUpdateActivity(ctx *lift.Context, note *activitypub.Note, actor *activitypub.Actor) error {
	now := time.Now()
	updateActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      activitypub.UpdateType,
			ID:        fmt.Sprintf("%s/activities/update-%d-%s", actor.ID, now.Unix(), generateRandomStringLift()),
			To:        note.To,
			CC:        note.CC,
			Published: &now,
		},
		Actor:  actor.ID,
		Object: note,
	}

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.repos.Activity().CreateActivity(ctx.Context, updateActivity); err != nil {
		h.logger.Error("failed to create update activity", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Federation: Deliver Update activity to relevant recipients
	go func() {
		if err := h.deliverUpdateActivity(context.Background(), updateActivity, actor, note); err != nil {
			h.logger.Error("failed to deliver update activity for federation",
				zap.String("activity_id", updateActivity.ID),
				zap.Error(err))
		}
	}()

	h.logger.Info("created update activity with federation",
		zap.String("activity_id", updateActivity.ID),
		zap.String("note_id", note.ID),
		zap.String("actor", actor.ID))

	return nil
}

// buildUpdateStatusResponse builds the response for the updated status
func (h *Handler) buildUpdateStatusResponse(ctx *lift.Context, note *activitypub.Note, actor *activitypub.Actor, req *models.UpdateStatusRequest) error {
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
	// Validate and normalize status ID
	objectID, err := h.validateAndNormalizeStatusID(ctx)
	if err != nil {
		return err
	}

	// Get the object
	object, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		// Check if this is a tombstoned object (should return 410 Gone)
		if isTombstoned, tombErr := h.repos.Object().IsTombstoned(ctx.Context, objectID); tombErr == nil && isTombstoned {
			// Get tombstone details for better error message
			if tombstone, tErr := h.repos.Object().GetTombstone(ctx.Context, objectID); tErr == nil {
				return ctx.Status(http.StatusGone).JSON(map[string]interface{}{
					"error": "status deleted",
					"error_description": "This status has been deleted",
					"deleted_at": tombstone.Deleted.Format(time.RFC3339),
					"former_type": tombstone.FormerType,
				})
			}
			// Fallback if we can't get tombstone details
			return ctx.Status(http.StatusGone).JSON(map[string]string{
				"error": "status deleted",
				"error_description": "This status has been deleted",
			})
		}
		// Regular 404 for genuinely missing objects
		return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
	}

	// Get the actor and convert to status
	actor := h.getStatusActor(ctx.Context, object, objectID)
	status := h.converter.ObjectToStatus(object, actor)
	
	// Enhanced privacy/visibility checking - return 404 (not 403) for unauthorized access
	requestingActorID, err := h.getRequestingActorID(ctx)
	if err != nil {
		// If we can't authenticate, only show public/unlisted statuses
		if status.Visibility != VisibilityPublic && status.Visibility != VisibilityUnlisted {
			return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
		}
	} else {
		// Enhanced visibility checking using Status model methods
		if !h.canActorSeeStatusEnhanced(&status, requestingActorID) {
			return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
		}
		
		// Sanitize status for this specific actor (removes BCC/BTo if not a recipient)
		status = *h.sanitizeStatusForActor(&status, requestingActorID)
	}

	// Parse and add emojis
	h.enrichStatusWithEmojis(ctx.Context, &status)

	// Add interaction counts
	h.enrichStatusWithInteractionCounts(ctx.Context, &status, objectID)

	// Add poll data if exists
	poll := h.enrichStatusWithPoll(ctx.Context, &status, objectID)

	// Add user interaction state
	h.enrichStatusWithUserInteractions(ctx, &status, objectID, poll)

	ctx.Status(http.StatusOK)
	return ctx.JSON(status)
}

// normalizeObjectID normalizes a status ID to a full URL if needed
func (h *Handler) normalizeObjectID(statusID string) string {
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		return fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}
	return statusID
}

// HandleGetStatusContextLift retrieves the context (ancestors and descendants) of a status
func (h *Handler) HandleGetStatusContextLift(ctx *lift.Context) error {
	// Validate and normalize status ID
	objectID, err := h.validateStatusIDForContext(ctx)
	if err != nil {
		return err
	}

	// Get ancestors and descendants
	ancestors := h.getStatusAncestors(ctx.Context, objectID)
	descendants := h.getStatusDescendants(ctx.Context, objectID)

	// Return context response
	resp := models.StatusContext{
		Ancestors:   ancestors,
		Descendants: descendants,
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// validateStatusIDForContext validates the status ID and checks it exists
func (h *Handler) validateStatusIDForContext(ctx *lift.Context) (string, error) {
	statusID := ctx.Param("id")
	if statusID == "" {
		return "", ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Normalize and validate the status ID
	objectID := h.normalizeObjectID(statusID)

	// Get the object to check it exists
	_, err := h.repos.Object().GetObject(ctx.Context, objectID)
	if err != nil {
		// Check if this is a tombstoned object (should return 410 Gone)
		if isTombstoned, tombErr := h.repos.Object().IsTombstoned(ctx.Context, objectID); tombErr == nil && isTombstoned {
			// Get tombstone details for better error message
			if tombstone, tErr := h.repos.Object().GetTombstone(ctx.Context, objectID); tErr == nil {
				return "", ctx.Status(http.StatusGone).JSON(map[string]interface{}{
					"error": "status deleted",
					"error_description": "This status has been deleted and its context is no longer available",
					"deleted_at": tombstone.Deleted.Format(time.RFC3339),
					"former_type": tombstone.FormerType,
				})
			}
			// Fallback if we can't get tombstone details
			return "", ctx.Status(http.StatusGone).JSON(map[string]string{
				"error": "status deleted",
				"error_description": "This status has been deleted and its context is no longer available",
			})
		}
		// Regular 404 for genuinely missing objects
		return "", ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
	}

	return objectID, nil
}

// getStatusAncestors retrieves the ancestors (parent statuses) of a status
func (h *Handler) getStatusAncestors(ctx context.Context, objectID string) []models.Status {
	ancestors := []models.Status{}
	currentID := objectID

	for i := 0; i < 10; i++ { // Limit depth to prevent infinite loops
		parentID := h.getParentStatusID(ctx, currentID)
		if parentID == "" {
			break
		}

		parentStatus := h.loadStatusWithActor(ctx, parentID)
		if parentStatus == nil {
			break
		}

		ancestors = append([]models.Status{*parentStatus}, ancestors...) // Prepend to maintain order
		currentID = parentID
	}

	return ancestors
}

// getParentStatusID gets the parent status ID from an object
func (h *Handler) getParentStatusID(ctx context.Context, objectID string) string {
	obj, err := h.repos.Object().GetObject(ctx, objectID)
	if err != nil {
		return ""
	}

	inReplyTo := h.extractInReplyTo(obj)
	if inReplyTo == "" {
		return ""
	}

	// Verify parent exists
	_, err = h.repos.Object().GetObject(ctx, inReplyTo)
	if err != nil {
		return ""
	}

	return inReplyTo
}

// extractInReplyTo extracts the InReplyTo field from various object types
func (h *Handler) extractInReplyTo(obj interface{}) string {
	switch o := obj.(type) {
	case *activitypub.Note:
		return o.InReplyTo
	case map[string]any:
		if reply, ok := o["inReplyTo"].(string); ok {
			return reply
		}
	default:
		return h.extractInReplyToViaReflection(obj)
	}
	return ""
}

// extractInReplyToViaReflection uses reflection to extract InReplyTo field
func (h *Handler) extractInReplyToViaReflection(obj interface{}) string {
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return ""
	}

	replyField := v.FieldByName("InReplyTo")
	if !replyField.IsValid() {
		return ""
	}

	// Handle pointer to string
	if replyField.Kind() == reflect.Ptr && !replyField.IsNil() {
		if replyField.Elem().Kind() == reflect.String {
			return replyField.Elem().String()
		}
	} else if replyField.Kind() == reflect.String {
		return replyField.String()
	}

	return ""
}

// loadStatusWithActor loads a status object and its associated actor
func (h *Handler) loadStatusWithActor(ctx context.Context, objectID string) *models.Status {
	obj, err := h.repos.Object().GetObject(ctx, objectID)
	if err != nil {
		return nil
	}

	actor := h.getActorForObject(ctx, obj)
	status := h.converter.ObjectToStatus(obj, actor)
	return &status
}

// getActorForObject retrieves the actor associated with an object
func (h *Handler) getActorForObject(ctx context.Context, obj interface{}) *activitypub.Actor {
	attributedTo := h.extractAttributedTo(obj)
	if attributedTo == "" {
		return nil
	}

	username := h.converter.ExtractUsernameFromActorID(attributedTo)
	if username == "" {
		return nil
	}

	actor, _ := h.repos.Account().GetActor(ctx, username)
	return actor
}

// extractAttributedToViaReflection uses reflection to extract AttributedTo field
//
//nolint:unused // Reserved for future ActivityPub object handling
func (h *Handler) extractAttributedToViaReflection(obj interface{}) string {
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return ""
	}

	attrField := v.FieldByName("AttributedTo")
	if !attrField.IsValid() || attrField.Kind() != reflect.String {
		return ""
	}

	return attrField.String()
}

// getStatusDescendants retrieves the descendants (replies) of a status
func (h *Handler) getStatusDescendants(ctx context.Context, objectID string) []models.Status {
	descendants := []models.Status{}

	// Fetch replies to this status
	replies, _, err := h.repos.Object().GetReplies(ctx, objectID, 100, "") // Get up to 100 replies
	if err != nil {
		h.logger.Warn("failed to get replies for context",
			zap.String("object_id", objectID),
			zap.Error(err))
		return descendants
	}

	for _, reply := range replies {
		if status := h.convertReplyToStatus(ctx, reply); status != nil {
			descendants = append(descendants, *status)
		}
	}

	h.logger.Debug("fetched descendants for context",
		zap.String("object_id", objectID),
		zap.Int("count", len(descendants)))

	return descendants
}

// convertReplyToStatus converts a reply object to a status
func (h *Handler) convertReplyToStatus(ctx context.Context, reply interface{}) *models.Status {
	actor := h.getActorForObject(ctx, reply)
	status := h.converter.ObjectToStatus(reply, actor)
	return &status
}

// HandleGetAccountStatusesLift retrieves statuses for a specific account
// accountStatusesParams holds the parameters for account statuses query
type accountStatusesParams struct {
	limit          int
	maxID          string
	onlyMedia      bool
	excludeReplies bool
	excludeReblogs bool
	tagged         string
}

// HandleGetAccountStatusesLift handles requests to get an account's statuses
func (h *Handler) HandleGetAccountStatusesLift(ctx *lift.Context) error {
	// Validate and get account ID
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
	params := h.parseAccountStatusesParams(ctx)

	// Get objects by this actor
	objects, cursor, err := h.repos.Object().GetObjectsByActor(ctx.Context, actor.ID, params.maxID, params.limit)
	if err != nil {
		h.logger.Error("failed to get objects by actor", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// Convert and filter objects to statuses
	statuses := h.convertAndFilterObjects(ctx, objects, actor, params)

	// Set pagination header if needed
	if cursor != "" {
		h.setPaginationHeader(ctx, accountID, cursor, params)
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(statuses)
}

// parseAccountStatusesParams parses query parameters for account statuses
func (h *Handler) parseAccountStatusesParams(ctx *lift.Context) accountStatusesParams {
	params := accountStatusesParams{
		limit:          20,
		maxID:          ctx.Query("max_id"),
		onlyMedia:      ctx.Query("only_media") == boolTrue,
		excludeReplies: ctx.Query("exclude_replies") == boolTrue,
		excludeReblogs: ctx.Query("exclude_reblogs") == boolTrue,
		tagged:         ctx.Query("tagged"),
	}

	// Parse limit parameter
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 40 {
			params.limit = parsedLimit
		}
	}

	return params
}

// convertAndFilterObjects converts objects to statuses with filtering
func (h *Handler) convertAndFilterObjects(_ *lift.Context, objects []any, actor *activitypub.Actor, params accountStatusesParams) []models.Status {
	statuses := []models.Status{}

	h.logger.Debug("converting objects to statuses",
		zap.Int("object_count", len(objects)),
		zap.String("actor_id", actor.ID))

	for _, obj := range objects {
		// Apply filters
		if h.shouldFilterObject(obj, params) {
			continue
		}

		status := h.converter.ObjectToStatus(obj, actor)
		h.logStatusConversion(status, obj)
		statuses = append(statuses, status)
	}

	return statuses
}

// shouldFilterObject checks if an object should be filtered out
func (h *Handler) shouldFilterObject(obj any, params accountStatusesParams) bool {
	if params.onlyMedia && !h.objectHasMedia(obj) {
		return true
	}

	if params.excludeReplies && h.objectIsReply(obj) {
		return true
	}

	// exclude_reblogs and tagged filters not yet implemented
	return false
}

// objectHasMedia checks if an object has media attachments
func (h *Handler) objectHasMedia(obj any) bool {
	switch o := obj.(type) {
	case *activitypub.Note:
		return len(o.Attachment) > 0
	case map[string]any:
		if attachments, ok := o["attachment"].([]any); ok {
			return len(attachments) > 0
		}
	}
	return false
}

// objectIsReply checks if an object is a reply
func (h *Handler) objectIsReply(obj any) bool {
	switch o := obj.(type) {
	case *activitypub.Note:
		return o.InReplyTo != ""
	case map[string]any:
		if inReplyTo, ok := o["inReplyTo"].(string); ok {
			return inReplyTo != ""
		}
	}
	return false
}

// logStatusConversion logs debug information about status conversion
func (h *Handler) logStatusConversion(status models.Status, obj any) {
	h.logger.Debug("converted status from object",
		zap.String("status_id", status.ID),
		zap.String("status_content", status.Content),
		zap.String("status_created_at", status.CreatedAt),
		zap.Any("object_type", fmt.Sprintf("%T", obj)))
}

// setPaginationHeader sets the Link header for pagination
func (h *Handler) setPaginationHeader(ctx *lift.Context, accountID, cursor string, params accountStatusesParams) {
	nextURL := h.buildPaginationURL(accountID, cursor, params)
	ctx.Response.Headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL)
}

// buildPaginationURL builds the next page URL
func (h *Handler) buildPaginationURL(accountID, cursor string, params accountStatusesParams) string {
	nextURL := fmt.Sprintf("%s/api/v1/accounts/%s/statuses?max_id=%s", h.cfg.BaseURL(), accountID, cursor)

	if params.limit != 20 {
		nextURL += fmt.Sprintf("&limit=%d", params.limit)
	}
	if params.onlyMedia {
		nextURL += "&only_media=true"
	}
	if params.excludeReplies {
		nextURL += "&exclude_replies=true"
	}
	if params.excludeReblogs {
		nextURL += "&exclude_reblogs=true"
	}
	if params.tagged != "" {
		nextURL += fmt.Sprintf("&tagged=%s", params.tagged)
	}

	return nextURL
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

	obj, err := h.repos.Object().GetObject(ctx, objectID)
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

// validateAndNormalizeStatusID validates and normalizes the status ID parameter
func (h *Handler) validateAndNormalizeStatusID(ctx *lift.Context) (string, error) {
	statusID := ctx.Param("id")
	if statusID == "" {
		return "", ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Normalize the status ID to a full URL if needed
	objectID := h.normalizeObjectID(statusID)
	return objectID, nil
}

// extractAuthorUsernameForStatus extracts the username from a status object
func (h *Handler) extractAuthorUsernameForStatus(ctx context.Context, objectID string) string {
	// Get the object
	obj, err := h.repos.Object().GetObject(ctx, objectID)
	if err != nil {
		return ""
	}

	// Extract attributedTo from the object
	var attributedTo string
	switch o := obj.(type) {
	case *activitypub.Note:
		attributedTo = o.AttributedTo
	case map[string]any:
		if attr, ok := o["attributedTo"].(string); ok {
			attributedTo = attr
		}
	}

	if attributedTo == "" {
		return ""
	}

	// Extract username from actor ID using converter
	return h.converter.ExtractUsernameFromActorID(attributedTo)
}

// getStatusActor retrieves the actor associated with a status object
func (h *Handler) getStatusActor(ctx context.Context, _ any, objectID string) *activitypub.Actor {
	// Extract author username from the object
	username := h.extractAuthorUsernameForStatus(ctx, objectID)
	if username == "" {
		return nil
	}

	// Get the actor
	actor, err := h.repos.Actor().GetActor(ctx, username)
	if err != nil {
		h.logger.Debug("failed to get status actor",
			zap.String("username", username),
			zap.Error(err))
		return nil
	}

	return actor
}

// enrichStatusWithEmojis parses and adds emoji data to a status
func (h *Handler) enrichStatusWithEmojis(ctx context.Context, status *models.Status) {
	// Create emoji parser
	emojiParser := emoji.NewParser(h.repos, h.logger)

	// Parse emojis from status content
	emojis, err := emojiParser.GetForStatus(ctx, status.Content)
	if err != nil {
		h.logger.Warn("Failed to parse emojis for status",
			zap.String("status_id", status.ID),
			zap.Error(err))
		status.Emojis = []any{}
		return
	}

	status.Emojis = emojis
}

// enrichStatusWithInteractionCounts adds like and reblog counts to a status
func (h *Handler) enrichStatusWithInteractionCounts(ctx context.Context, status *models.Status, objectID string) {
	// Get like count
	likeCount, err := h.repos.Like().GetLikeCount(ctx, objectID)
	if err == nil {
		status.FavouritesCount = int(likeCount)
	}

	// Get reblog/boost count
	boostCount, err := h.repos.Like().GetBoostCount(ctx, objectID)
	if err == nil {
		status.ReblogsCount = int(boostCount)
	}
}

// enrichStatusWithPoll adds poll data to a status if it exists
func (h *Handler) enrichStatusWithPoll(ctx context.Context, status *models.Status, objectID string) *storage.Poll {
	// Get poll if exists
	poll, err := h.repos.Poll().GetPollByStatusID(ctx, objectID)
	if err == nil && poll != nil {
		// Use converter to convert poll to API format
		apiPoll := h.converter.PollToAPI(poll, []int{}) // User votes will be set later in enrichStatusWithUserMetadata

		status.Poll = &apiPoll
		return poll
	}
	return nil
}

// extractUsernameFromToken extracts the username from authentication token
func (h *Handler) extractUsernameFromToken(ctx *lift.Context) string {
	// Check for test mode support
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}
	if testUsername != "" {
		return testUsername
	}

	// Get authorization header
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			// Validate token
			oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
			claims, err := oauthSvc.ValidateAccessToken(token)
			if err == nil {
				return claims.Username
			}
		}
	}

	return ""
}

// enrichStatusWithUserInteractions adds user-specific interaction state to a status
func (h *Handler) enrichStatusWithUserInteractions(ctx *lift.Context, status *models.Status, objectID string, poll *storage.Poll) {
	// Try to get authenticated user
	username := h.extractUsernameFromToken(ctx)
	if username == "" {
		return
	}

	// Build actor ID for the user
	actorID := fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), username)

	// Check if user has liked this status
	liked, err := h.repos.Like().HasLiked(ctx.Context, actorID, objectID)
	if err == nil {
		status.Favourited = liked
	}

	// Check if user has reblogged this status
	reblogged, err := h.repos.Like().HasReblogged(ctx.Context, actorID, objectID)
	if err == nil {
		status.Reblogged = reblogged
	} else {
		// Log error but don't fail - just assume not reblogged
		h.logger.Warn("failed to check reblog status",
			zap.String("actor_id", actorID),
			zap.String("object_id", objectID),
			zap.Error(err))
		status.Reblogged = false
	}

	// Check if user has bookmarked this status
	bookmarked, err := h.repos.User().IsBookmarked(ctx.Context, username, objectID)
	if err == nil {
		status.Bookmarked = bookmarked
	} else {
		// Log error but don't fail - just assume not bookmarked
		h.logger.Warn("failed to check bookmark status",
			zap.String("username", username),
			zap.String("object_id", objectID),
			zap.Error(err))
		status.Bookmarked = false
	}

	// Check if user has voted in poll
	if poll != nil && status.Poll != nil && username != "" {
		// Check if user has voted using the poll repository
		hasVoted, userVotes, err := h.repos.Poll().HasUserVoted(ctx.Context, poll.ID, actorID)
		if err != nil {
			// Log error but don't fail - just assume no votes
			h.logger.Warn("failed to get user poll votes",
				zap.String("poll_id", poll.ID),
				zap.String("actor_id", actorID),
				zap.Error(err))
			status.Poll.Voted = false
			status.Poll.OwnVotes = []int{}
		} else {
			status.Poll.Voted = hasVoted
			if userVotes != nil {
				status.Poll.OwnVotes = userVotes
			} else {
				status.Poll.OwnVotes = []int{}
			}
		}
	}
}

// performLocalCascadeDeletion performs cascade deletion operations for local status deletion
func (h *Handler) performLocalCascadeDeletion(ctx context.Context, objectID, actorID string) error {
	h.logger.Info("performing local cascade deletion operations",
		zap.String("object_id", objectID),
		zap.String("actor", actorID))

	// Remove likes for this object
	if err := h.cascadeDeleteLikes(ctx, objectID); err != nil {
		h.logger.Warn("failed to cascade delete likes", zap.Error(err))
	}

	// Remove announces/boosts for this object
	if err := h.cascadeDeleteAnnounces(ctx, objectID); err != nil {
		h.logger.Warn("failed to cascade delete announces", zap.Error(err))
	}

	// Remove from collections (featured, pinned, etc.)
	if err := h.cascadeDeleteFromCollections(ctx, objectID); err != nil {
		h.logger.Warn("failed to cascade delete from collections", zap.Error(err))
	}

	// Clean up bookmarks
	if err := h.cascadeDeleteBookmarks(ctx, objectID); err != nil {
		h.logger.Warn("failed to cascade delete bookmarks", zap.Error(err))
	}

	// Clean up polls if this status had one
	if err := h.cascadeDeletePolls(ctx, objectID); err != nil {
		h.logger.Warn("failed to cascade delete polls", zap.Error(err))
	}

	h.logger.Info("completed local cascade deletion operations", zap.String("object_id", objectID))
	return nil
}

// cascadeDeleteLikes removes all likes for the deleted object
func (h *Handler) cascadeDeleteLikes(ctx context.Context, objectID string) error {
	// Get all likes for this object (using a reasonable limit)
	likes, _, err := h.repos.Like().GetObjectLikes(ctx, objectID, 1000, "")
	if err != nil {
		return fmt.Errorf("failed to get object likes: %w", err)
	}

	// Delete each like
	for _, like := range likes {
		if err := h.repos.Like().DeleteLike(ctx, like.Actor, objectID); err != nil {
			h.logger.Warn("failed to delete like during cascade",
				zap.String("object_id", objectID),
				zap.String("like_actor", like.Actor),
				zap.Error(err))
		}
	}

	h.logger.Debug("cascade deleted likes",
		zap.String("object_id", objectID),
		zap.Int("count", len(likes)))

	return nil
}

// cascadeDeleteAnnounces removes all announces/boosts for the deleted object
func (h *Handler) cascadeDeleteAnnounces(ctx context.Context, objectID string) error {
	// Use the Social repository's CascadeDeleteAnnounces method
	if err := h.repos.Social().CascadeDeleteAnnounces(ctx, objectID); err != nil {
		h.logger.Warn("failed to cascade delete announces",
			zap.String("object_id", objectID),
			zap.Error(err))
		return err
	}

	h.logger.Debug("cascade deleted announces",
		zap.String("object_id", objectID))

	return nil
}

// cascadeDeleteFromCollections removes the object from collections
func (h *Handler) cascadeDeleteFromCollections(ctx context.Context, objectID string) error {
	// Remove from common collections
	collections := []string{"featured", "pinned"}

	for _, collection := range collections {
		if err := h.repos.Object().RemoveFromCollection(ctx, collection, objectID); err != nil {
			h.logger.Debug("failed to remove from collection during cascade",
				zap.String("object_id", objectID),
				zap.String("collection", collection),
				zap.Error(err))
		}
	}

	return nil
}

// cascadeDeleteBookmarks removes all bookmarks for the deleted object
func (h *Handler) cascadeDeleteBookmarks(_ context.Context, objectID string) error {
	// Note: The current Bookmark model doesn't have a GSI to efficiently query by ObjectID
	// This would require either:
	// 1. Adding a GSI to the Bookmark model (infrastructure change)
	// 2. Implementing a background cleanup process
	// 3. Accepting that orphaned bookmarks will exist (cleaned up on access)
	//
	// For now, we log this limitation and rely on the application layer to handle
	// orphaned bookmarks gracefully (e.g., returning 404 when trying to access a deleted status)
	
	h.logger.Debug("cascade delete bookmarks - skipped due to model limitations", 
		zap.String("object_id", objectID),
		zap.String("reason", "no GSI for efficient bookmark lookup by object_id"))
	
	// In a production system, you might want to add this to a cleanup queue
	// or implement a background job that periodically removes orphaned bookmarks
	
	return nil
}

// cascadeDeletePolls removes polls associated with the deleted status
func (h *Handler) cascadeDeletePolls(ctx context.Context, objectID string) error {
	// Check if this status has a poll
	poll, err := h.repos.Poll().GetPollByStatusID(ctx, objectID)
	if err != nil {
		return nil // No poll found, which is fine
	}

	if poll != nil {
		// For now, log that poll cleanup should happen
		// This would require implementing poll deletion methods
		h.logger.Debug("cascade delete poll - implementation framework ready",
			zap.String("object_id", objectID),
			zap.String("poll_id", poll.ID))
	}

	return nil
}

// deliverDeleteActivity delivers a Delete activity to relevant recipients for federation
func (h *Handler) deliverDeleteActivity(ctx context.Context, deleteActivity *activitypub.Activity, actor *activitypub.Actor, originalObject any) error {
	h.logger.Info("delivering delete activity for federation",
		zap.String("activity_id", deleteActivity.ID),
		zap.String("actor", actor.ID))

	// Determine delivery recipients based on the original object
	recipients, err := h.determineDeleteDeliveryRecipients(ctx, actor, originalObject)
	if err != nil {
		return fmt.Errorf("failed to determine delivery recipients: %w", err)
	}

	if len(recipients) == 0 {
		h.logger.Info("no recipients for delete activity delivery", zap.String("activity_id", deleteActivity.ID))
		return nil
	}

	// Deliver to each recipient
	deliveredCount := 0
	failedCount := 0

	for _, recipient := range recipients {
		if err := h.deliverDeleteToRecipient(ctx, deleteActivity, actor, recipient); err != nil {
			h.logger.Warn("failed to deliver delete activity to recipient",
				zap.String("recipient", recipient),
				zap.String("activity_id", deleteActivity.ID),
				zap.Error(err))
			failedCount++
		} else {
			deliveredCount++
		}
	}

	h.logger.Info("completed delete activity delivery",
		zap.String("activity_id", deleteActivity.ID),
		zap.Int("delivered", deliveredCount),
		zap.Int("failed", failedCount),
		zap.Int("total_recipients", len(recipients)))

	return nil
}

// determineDeleteDeliveryRecipients determines who should receive the Delete activity
func (h *Handler) determineDeleteDeliveryRecipients(ctx context.Context, actor *activitypub.Actor, originalObject any) ([]string, error) {
	recipients := make(map[string]bool)

	// Add followers to recipients
	followers, _, err := h.repos.Relationship().GetFollowers(ctx, actor.PreferredUsername, 1000, "")
	if err != nil {
		h.logger.Warn("failed to get followers for delete delivery", zap.Error(err))
	} else {
		for _, follower := range followers {
			recipients[follower] = true
		}
	}

	// Add mentioned users from original object
	mentions := h.extractMentionsFromObject(originalObject)
	for _, mention := range mentions {
		recipients[mention] = true
	}

	// Add users who were in original To/CC fields
	toUsers, ccUsers := h.extractToAndCCFromObject(originalObject)
	for _, user := range toUsers {
		if user != activitypub.PublicAddress {
			recipients[user] = true
		}
	}
	for _, user := range ccUsers {
		if user != activitypub.PublicAddress {
			recipients[user] = true
		}
	}

	// Convert to slice
	recipientList := make([]string, 0, len(recipients))
	for recipient := range recipients {
		recipientList = append(recipientList, recipient)
	}

	return recipientList, nil
}

// deliverDeleteToRecipient delivers the Delete activity to a specific recipient
func (h *Handler) deliverDeleteToRecipient(ctx context.Context, deleteActivity *activitypub.Activity, actor *activitypub.Actor, recipientID string) error {
	h.logger.Debug("delivering delete activity to recipient",
		zap.String("recipient_id", recipientID),
		zap.String("activity_id", deleteActivity.ID))

	// Create federation storage and delivery service
	federationStorage := federation.NewDynamORMFederationStorage(h.repos.GetDB(), h.repos.GetTableName())
	deliveryService := federation.NewDeliveryService(federationStorage)

	// Set recipient in activity To field for delivery
	deleteActivity.To = []string{recipientID}

	// Deliver using federation service
	if err := deliveryService.DeliverToRecipients(ctx, deleteActivity, actor); err != nil {
		h.logger.Error("failed to deliver delete activity to recipient",
			zap.String("recipient_id", recipientID),
			zap.String("activity_id", deleteActivity.ID),
			zap.Error(err))
		return err
	}

	h.logger.Info("delete activity delivered successfully",
		zap.String("recipient_id", recipientID),
		zap.String("activity_id", deleteActivity.ID))

	return nil
}

// extractMentionsFromObject extracts mentioned user IDs from an object
func (h *Handler) extractMentionsFromObject(object any) []string {
	var mentions []string

	if objMap, ok := object.(map[string]any); ok {
		if tagsInterface, ok := objMap["tag"]; ok {
			mentions = h.parseMentionsFromTags(tagsInterface)
		}
	} else if note, ok := object.(*activitypub.Note); ok {
		for _, tag := range note.Tag {
			if tag.Type == tagTypeMention && tag.Href != "" {
				mentions = append(mentions, tag.Href)
			}
		}
	}

	return mentions
}

// extractToAndCCFromObject extracts To and CC fields from an object
func (h *Handler) extractToAndCCFromObject(object any) ([]string, []string) {
	var toUsers, ccUsers []string

	if objMap, ok := object.(map[string]any); ok {
		if to, ok := objMap["to"].([]string); ok {
			toUsers = to
		}
		if cc, ok := objMap["cc"].([]string); ok {
			ccUsers = cc
		}
	} else if note, ok := object.(*activitypub.Note); ok {
		toUsers = note.To
		ccUsers = note.CC
	}

	return toUsers, ccUsers
}

const tagTypeMention = "Mention"

// parseMentionsFromTags parses mentions from various tag formats
func (h *Handler) parseMentionsFromTags(tagsInterface any) []string {
	var mentions []string

	switch tags := tagsInterface.(type) {
	case []any:
		mentions = h.parseMentionsFromAnySlice(tags)
	case []activitypub.Tag:
		mentions = h.parseMentionsFromTagSlice(tags)
	case string:
		mentions = h.parseMentionsFromString(tags)
	}

	return mentions
}

// parseMentionsFromAnySlice extracts mentions from a slice of any type
func (h *Handler) parseMentionsFromAnySlice(tags []any) []string {
	var mentions []string
	for _, tagInterface := range tags {
		if mention := h.extractMentionFromInterface(tagInterface); mention != "" {
			mentions = append(mentions, mention)
		}
	}
	return mentions
}

// extractMentionFromInterface extracts a mention URL from an interface{} if it's a mention tag
func (h *Handler) extractMentionFromInterface(tagInterface any) string {
	tagMap, ok := tagInterface.(map[string]any)
	if !ok {
		return ""
	}
	
	tagType, ok := tagMap["type"].(string)
	if !ok || tagType != tagTypeMention {
		return ""
	}
	
	href, ok := tagMap["href"].(string)
	if !ok || href == "" {
		return ""
	}
	
	return href
}

// parseMentionsFromTagSlice extracts mentions from a slice of ActivityPub tags
func (h *Handler) parseMentionsFromTagSlice(tags []activitypub.Tag) []string {
	var mentions []string
	for _, tag := range tags {
		if tag.Type == tagTypeMention && tag.Href != "" {
			mentions = append(mentions, tag.Href)
		}
	}
	return mentions
}

// parseMentionsFromString extracts mentions from a JSON string of tags
func (h *Handler) parseMentionsFromString(tags string) []string {
	var tagSlice []activitypub.Tag
	if err := json.Unmarshal([]byte(tags), &tagSlice); err != nil {
		return nil
	}
	return h.parseMentionsFromTagSlice(tagSlice)
}

// deliverUpdateActivity delivers an Update activity to relevant recipients for federation
func (h *Handler) deliverUpdateActivity(ctx context.Context, updateActivity *activitypub.Activity, actor *activitypub.Actor, note *activitypub.Note) error {
	h.logger.Info("delivering update activity for federation",
		zap.String("activity_id", updateActivity.ID),
		zap.String("actor", actor.ID))

	// Determine delivery recipients based on the updated note
	recipients, err := h.determineUpdateDeliveryRecipients(ctx, actor, note)
	if err != nil {
		return fmt.Errorf("failed to determine delivery recipients: %w", err)
	}

	if len(recipients) == 0 {
		h.logger.Info("no recipients for update activity delivery", zap.String("activity_id", updateActivity.ID))
		return nil
	}

	// Deliver to each recipient
	deliveredCount := 0
	failedCount := 0

	for _, recipient := range recipients {
		if err := h.deliverUpdateToRecipient(ctx, updateActivity, actor, recipient); err != nil {
			h.logger.Warn("failed to deliver update activity to recipient",
				zap.String("recipient", recipient),
				zap.String("activity_id", updateActivity.ID),
				zap.Error(err))
			failedCount++
		} else {
			deliveredCount++
		}
	}

	h.logger.Info("completed update activity delivery",
		zap.String("activity_id", updateActivity.ID),
		zap.Int("delivered", deliveredCount),
		zap.Int("failed", failedCount),
		zap.Int("total_recipients", len(recipients)))

	return nil
}

// determineUpdateDeliveryRecipients determines who should receive the Update activity
func (h *Handler) determineUpdateDeliveryRecipients(ctx context.Context, actor *activitypub.Actor, note *activitypub.Note) ([]string, error) {
	recipients := make(map[string]bool)

	// Add followers to recipients (they should be notified of edits)
	followers, _, err := h.repos.Relationship().GetFollowers(ctx, actor.PreferredUsername, 1000, "")
	if err != nil {
		h.logger.Warn("failed to get followers for update delivery", zap.Error(err))
	} else {
		for _, follower := range followers {
			recipients[follower] = true
		}
	}

	// Add mentioned users from the updated note
	for _, tag := range note.Tag {
		if tag.Type == "Mention" && tag.Href != "" {
			recipients[tag.Href] = true
		}
	}

	// Add users in To/CC fields
	for _, user := range note.To {
		if user != activitypub.PublicAddress {
			recipients[user] = true
		}
	}
	for _, user := range note.CC {
		if user != activitypub.PublicAddress {
			recipients[user] = true
		}
	}

	// Convert to slice
	recipientList := make([]string, 0, len(recipients))
	for recipient := range recipients {
		recipientList = append(recipientList, recipient)
	}

	return recipientList, nil
}

// deliverUpdateToRecipient delivers the Update activity to a specific recipient
func (h *Handler) deliverUpdateToRecipient(ctx context.Context, updateActivity *activitypub.Activity, actor *activitypub.Actor, recipientID string) error {
	h.logger.Debug("delivering update activity to recipient",
		zap.String("recipient_id", recipientID),
		zap.String("activity_id", updateActivity.ID))

	// Create federation storage and delivery service
	federationStorage := federation.NewDynamORMFederationStorage(h.repos.GetDB(), h.repos.GetTableName())
	deliveryService := federation.NewDeliveryService(federationStorage)

	// Set recipient in activity To field for delivery
	updateActivity.To = []string{recipientID}

	// Deliver using federation service
	if err := deliveryService.DeliverToRecipients(ctx, updateActivity, actor); err != nil {
		h.logger.Error("failed to deliver update activity to recipient",
			zap.String("recipient_id", recipientID),
			zap.String("activity_id", updateActivity.ID),
			zap.Error(err))
		return err
	}

	h.logger.Info("update activity delivered successfully",
		zap.String("recipient_id", recipientID),
		zap.String("activity_id", updateActivity.ID))

	return nil
}

// deliverCreateActivity delivers a Create activity to relevant recipients for federation
func (h *Handler) deliverCreateActivity(ctx context.Context, createActivity *activitypub.Activity, actor *activitypub.Actor, visibility string) error {
	h.logger.Info("delivering create activity for federation",
		zap.String("activity_id", createActivity.ID),
		zap.String("actor", actor.ID),
		zap.String("visibility", visibility))

	// Create federation storage and delivery service
	federationStorage := federation.NewDynamORMFederationStorage(h.repos.GetDB(), h.repos.GetTableName())
	deliveryService := federation.NewDeliveryService(federationStorage)

	// Deliver based on visibility
	switch visibility {
	case VisibilityPublic, VisibilityUnlisted:
		// Deliver to followers for public/unlisted content
		if err := deliveryService.DeliverToFollowers(ctx, createActivity, actor); err != nil {
			h.logger.Error("failed to deliver to followers", zap.Error(err))
			// Continue to deliver to specific recipients
		}
		
		// Also deliver to specific recipients (mentions, replies, etc.)
		if err := deliveryService.DeliverToRecipients(ctx, createActivity, actor); err != nil {
			h.logger.Error("failed to deliver to recipients", zap.Error(err))
			return err
		}

	case VisibilityPrivate:
		// Only deliver to followers
		if err := deliveryService.DeliverToFollowers(ctx, createActivity, actor); err != nil {
			h.logger.Error("failed to deliver to followers", zap.Error(err))
			return err
		}

	case VisibilityDirect:
		// Use privacy-aware direct message delivery
		if err := deliveryService.DeliverDirectMessage(ctx, createActivity, actor); err != nil {
			h.logger.Error("failed to deliver direct message", zap.Error(err))
			return err
		}

	default:
		h.logger.Info("no federation delivery for visibility",
			zap.String("visibility", visibility),
			zap.String("activity_id", createActivity.ID))
	}

	h.logger.Info("create activity federation delivery completed",
		zap.String("activity_id", createActivity.ID),
		zap.String("visibility", visibility))

	return nil
}

// getRequestingActorID extracts the actor ID from authentication context
func (h *Handler) getRequestingActorID(ctx *lift.Context) (string, error) {
	// Try to get authentication token
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		return "", fmt.Errorf("no authentication token")
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", err
	}

	// Get the actor
	actor, err := h.repos.Account().GetActor(ctx.Context, claims.Username)
	if err != nil {
		return "", err
	}

	return actor.ID, nil
}

// canActorSeeStatus checks if an actor can view a status based on visibility rules
func (h *Handler) canActorSeeStatus(status *models.Status, requestingActorID string) bool {
	switch status.Visibility {
	case VisibilityPublic, VisibilityUnlisted:
		return true
	case VisibilityPrivate:
		// Private posts are visible to author and followers
		if status.Account.ID == requestingActorID {
			return true
		}
		// Check if requesting actor is a follower
		return h.isFollower(requestingActorID, status.Account.ID)
	case VisibilityDirect:
		// Direct messages are only visible to author and mentioned users
		if status.Account.ID == requestingActorID {
			return true
		}
		// Check if requesting actor is mentioned
		return h.isMentioned(requestingActorID, status)
	default:
		return false
	}
}

// isFollower checks if one actor follows another
func (h *Handler) isFollower(followerID, followeeID string) bool {
	// Extract usernames from actor IDs for repository call
	followerUsername := h.extractUsernameFromActorID(followerID)
	followeeUsername := h.extractUsernameFromActorID(followeeID)
	
	if followerUsername == "" || followeeUsername == "" {
		return false
	}
	
	// Check if the relationship exists
	exists, err := h.repos.Relationship().IsFollowing(context.Background(), followerUsername, followeeUsername)
	if err != nil {
		h.logger.Warn("error checking follow relationship", zap.Error(err))
		return false
	}
	
	return exists
}

// isMentioned checks if an actor is mentioned in a status
func (h *Handler) isMentioned(actorID string, status *models.Status) bool {
	// Check if the actor is in the mentions
	for _, mention := range status.Mentions {
		if mention == actorID {
			return true
		}
	}
	return false
}

// extractUsernameFromActorID extracts username from an ActivityPub actor ID
func (h *Handler) extractUsernameFromActorID(actorID string) string {
	// Handle different actor ID formats
	// e.g., "https://example.com/users/username" -> "username"
	parts := strings.Split(actorID, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// canActorSeeStatusEnhanced performs enhanced visibility checking using Status model methods
func (h *Handler) canActorSeeStatusEnhanced(status *models.Status, requestingActorID string) bool {
	// Convert models.Status to our storage Status for visibility checks
	storageStatus := h.convertToStorageStatus(status)
	
	// Use the enhanced visibility checking from the Status model
	return storageStatus.IsVisibleTo(requestingActorID)
}

// sanitizeStatusForActor removes sensitive addressing information based on viewer's relationship
func (h *Handler) sanitizeStatusForActor(status *models.Status, viewerID string) *models.Status {
	// Convert to storage Status, sanitize, then convert back
	storageStatus := h.convertToStorageStatus(status)
	sanitized := storageStatus.SanitizeForActor(viewerID)
	return h.convertFromStorageStatus(sanitized)
}

// convertToStorageStatus converts API models.Status to storage models.Status
func (h *Handler) convertToStorageStatus(status *models.Status) *storageModels.Status {
	// Convert mentions from []any to []string
	mentions := make([]string, 0)
	if status.Mentions != nil {
		for _, mention := range status.Mentions {
			if str, ok := mention.(string); ok {
				mentions = append(mentions, str)
			}
		}
	}

	// Handle InReplyToID pointer
	inReplyToID := ""
	if status.InReplyToID != nil {
		inReplyToID = *status.InReplyToID
	}

	// Parse CreatedAt timestamp
	var createdAt time.Time
	if status.CreatedAt != "" {
		// Try to parse the timestamp - handle multiple potential formats
		if parsed, err := time.Parse(time.RFC3339, status.CreatedAt); err == nil {
			createdAt = parsed
		} else if parsed, err := time.Parse("2006-01-02T15:04:05.000Z", status.CreatedAt); err == nil {
			createdAt = parsed  
		} else if parsed, err := time.Parse("2006-01-02T15:04:05Z", status.CreatedAt); err == nil {
			createdAt = parsed
		} else {
			// If parsing fails, use current time as fallback
			createdAt = time.Now()
		}
	} else {
		createdAt = time.Now()
	}

	// Extract addressing from status content or ActivityPub Note if available
	// For now, extract basic visibility-based addressing
	toRecipients := []string{}
	ccRecipients := []string{}
	btoRecipients := []string{}
	bccRecipients := []string{}

	// Set up basic addressing based on visibility
	switch status.Visibility {
	case "public":
		// Public posts go to the Public collection
		toRecipients = append(toRecipients, "https://www.w3.org/ns/activitystreams#Public")
		// CC followers collection if we have author info
		if status.Account.ID != "" {
			ccRecipients = append(ccRecipients, fmt.Sprintf("https://%s/users/%s/followers", 
				h.getDomainFromConfig(), status.Account.Username))
		}
	case "unlisted":
		// Unlisted posts CC the Public collection
		ccRecipients = append(ccRecipients, "https://www.w3.org/ns/activitystreams#Public")
		// TO followers collection
		if status.Account.ID != "" {
			toRecipients = append(toRecipients, fmt.Sprintf("https://%s/users/%s/followers",
				h.getDomainFromConfig(), status.Account.Username))
		}
	case "private":
		// Private posts only to followers
		if status.Account.ID != "" {
			toRecipients = append(toRecipients, fmt.Sprintf("https://%s/users/%s/followers",
				h.getDomainFromConfig(), status.Account.Username))
		}
	case VisibilityDirect:
		// Direct messages - recipients would be extracted from mentions
		// For now, we'll leave empty as we don't have specific recipient info in the API model
	}

	// Add mentioned users to recipients for direct messages
	if status.Visibility == VisibilityDirect && len(mentions) > 0 {
		toRecipients = append(toRecipients, mentions...)
	}

	return &storageModels.Status{
		StatusID:      status.ID,
		AuthorID:      status.Account.ID,
		Visibility:    status.Visibility,
		ToRecipients:  toRecipients,
		CcRecipients:  ccRecipients,
		BtoRecipients: btoRecipients,
		BccRecipients: bccRecipients,
		Mentions:      mentions,
		InReplyToID:   inReplyToID,
		CreatedAt:     createdAt,
	}
}

// getDomainFromConfig returns the domain from environment configuration
func (h *Handler) getDomainFromConfig() string {
	domain := os.Getenv("DOMAIN")
	if domain == "" {
		domain = os.Getenv("DOMAIN_NAME")
	}
	if domain == "" {
		domain = "localhost" // Default fallback for local development
	}
	return domain
}

// convertFromStorageStatus converts storage models.Status back to API models.Status
func (h *Handler) convertFromStorageStatus(storageStatus *storageModels.Status) *models.Status {
	// Convert mentions from []string to []any
	mentions := make([]any, len(storageStatus.Mentions))
	for i, mention := range storageStatus.Mentions {
		mentions[i] = mention
	}

	// Handle InReplyToID pointer
	var inReplyToID *string
	if storageStatus.InReplyToID != "" {
		inReplyToID = &storageStatus.InReplyToID
	}

	// This is a simplified conversion - in a real implementation you'd need
	// to preserve all the original status fields while updating the addressing fields
	return &models.Status{
		ID:          storageStatus.StatusID,
		Visibility:  storageStatus.Visibility,
		Mentions:    mentions,
		InReplyToID: inReplyToID,
		// Other fields would be preserved from the original status
	}
}

// getTimelineStatuses retrieves statuses for a timeline with proper visibility filtering
//
//nolint:unused // TODO: Will be used when timeline repositories are implemented
func (h *Handler) getTimelineStatuses(_ *lift.Context, timelineType string, _ string, _ int) ([]models.Status, error) {
	// TODO: This would integrate with your timeline repository to get statuses
	// and filter them based on visibility
	
	h.logger.Info("getting timeline statuses",
		zap.String("timeline_type", timelineType))
	
	// For now, return empty slice - this would be implemented when timeline repositories are ready
	return []models.Status{}, nil
}

// getConversationsForActor retrieves direct message conversations for an actor
//
//nolint:unused // TODO: Will be used when conversation repositories are implemented
func (h *Handler) getConversationsForActor(_ *lift.Context, _ string, _ int) ([]models.Status, error) {
	// TODO: Get direct messages where the actor is a recipient
	
	h.logger.Info("getting conversations for actor")
	
	// For now, return empty slice - this would be implemented when conversation repositories are ready
	return []models.Status{}, nil
}


// CreateStatusRequest represents the enhanced request for creating a status with addressing fields
type CreateStatusRequest struct {
	Status        string   `json:"status"`
	Visibility    string   `json:"visibility"`
	InReplyToID   string   `json:"in_reply_to_id,omitempty"`
	Sensitive     bool     `json:"sensitive,omitempty"`
	SpoilerText   string   `json:"spoiler_text,omitempty"`
	Mentions      []string `json:"mentions,omitempty"`
	ToRecipients  []string `json:"to_recipients,omitempty"`  // Enhanced addressing
	CcRecipients  []string `json:"cc_recipients,omitempty"`  // Enhanced addressing
	BtoRecipients []string `json:"bto_recipients,omitempty"` // Enhanced addressing (hidden from others)
	BCCRecipients []string `json:"bcc_recipients,omitempty"` // Enhanced addressing (hidden from all)
}
