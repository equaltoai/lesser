package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/notes"
	"github.com/aron23/lesser/pkg/reputation"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleCreateNote handles POST /api/v1/notes
func (h *Handler) HandleCreateNote(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Get user ID
	userID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, claims.Username)

	// Initialize services
	repService, err := h.getNoteReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Check user's reputation
	rep, err := repService.GetReputation(ctx, userID)
	if err != nil || rep.TotalScore < notes.MinReputationToCreateNotes {
		return common.Forbidden(errors.New("insufficient reputation to create notes")), nil
	}

	// Parse request
	var req notes.CreateNoteRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return common.BadRequest(errors.New("invalid request body")), nil
	}

	// Validate sources
	if len(req.Sources) > notes.MaxSources {
		return common.BadRequest(fmt.Errorf("maximum %d sources allowed", notes.MaxSources)), nil
	}

	// Check rate limit
	limit := notes.CalculateNoteLimit(float64(rep.TotalScore))
	canCreate, remaining, err := h.store.CheckCommunityNoteRateLimit(ctx, userID, limit)
	if err != nil {
		h.logger.Error("Failed to check rate limit", zap.Error(err))
		return common.InternalServerError(err), nil
	}
	if !canCreate {
		return common.UnprocessableEntity(fmt.Errorf("note limit reached. %d notes allowed per day based on reputation", limit)), nil
	}

	// Convert Source structs to string URLs
	sourceURLs := make([]string, len(req.Sources))
	for i, src := range req.Sources {
		sourceURLs[i] = src.URL
	}

	// Create note
	note := &storage.CommunityNote{
		ID:               "", // Let storage generate ID
		ObjectID:         req.ObjectID,
		ObjectType:       req.ObjectType,
		AuthorID:         userID,
		Content:          req.Content,
		Language:         req.Language,
		Sources:          sourceURLs,
		Score:            0,
		VisibilityStatus: "pending",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		HelpfulVotes:     0,
		NotHelpfulVotes:  0,
	}

	// Store note
	if err := h.store.CreateCommunityNote(ctx, note); err != nil {
		h.logger.Error("Failed to store note", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Add rate limit info to response
	response := map[string]interface{}{
		"note": note,
		"rate_limit": map[string]interface{}{
			"limit":     limit,
			"remaining": remaining - 1,
			"reset":     "24h",
		},
	}

	// Add cost tracking headers
	resp := common.Created(response)
	resp.Headers["X-Cost-Micros"] = "2000" // ~$0.00002
	resp.Headers["X-Cost-Details"] = "DynamoDB: 2 writes"

	return resp, nil
}

// HandleGetNotes handles GET /api/v1/notes/:object_id
func (h *Handler) HandleGetNotes(ctx context.Context, request events.APIGatewayV2HTTPRequest, objectID string) (*events.APIGatewayV2HTTPResponse, error) {
	if objectID == "" {
		return common.BadRequest(errors.New("object ID required")), nil
	}

	// Optional auth - for personalized scoring
	var userID string
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
			claims, err := oauthSvc.ValidateAccessToken(token)
			if err == nil {
				userID = fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, claims.Username)
			}
		}
	}

	// Get visible notes for the object
	visibleNotes, err := h.store.GetVisibleCommunityNotes(ctx, objectID)
	if err != nil {
		h.logger.Error("Failed to get visible notes", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get trust scores for ranking if user is authenticated
	var rankedNotes []*storage.CommunityNote
	if userID != "" {
		// Convert storage notes to notes package format for ranking
		notesForRanking := make([]notes.CommunityNote, len(visibleNotes))
		for i, note := range visibleNotes {
			// Convert string sources to Source structs
			sources := make([]notes.Source, len(note.Sources))
			for j, src := range note.Sources {
				sources[j] = notes.Source{
					URL:         src,
					Title:       "",
					Domain:      "",
					Reliability: 0,
				}
			}

			notesForRanking[i] = notes.CommunityNote{
				ID:               note.ID,
				ObjectID:         note.ObjectID,
				ObjectType:       note.ObjectType,
				AuthorID:         note.AuthorID,
				Content:          note.Content,
				Language:         note.Language,
				Sources:          sources,
				HelpfulVotes:     note.HelpfulVotes,
				NotHelpfulVotes:  note.NotHelpfulVotes,
				Score:            note.Score,
				VisibilityStatus: notes.VisibilityStatus(note.VisibilityStatus),
				CreatedAt:        note.CreatedAt,
				UpdatedAt:        note.UpdatedAt,
			}
		}

		// Get trust scores for ranking
		trustScores := make(map[string]float64)
		// TODO: Get actual trust scores from trust service once available
		rankedNotesResult := notes.RankNotesByTrust(notesForRanking, userID, trustScores)

		// Convert back to storage format
		rankedNotes = make([]*storage.CommunityNote, len(rankedNotesResult))
		for i, note := range rankedNotesResult {
			// Convert Source structs back to strings
			sources := make([]string, len(note.Sources))
			for j, src := range note.Sources {
				sources[j] = src.URL
			}

			rankedNotes[i] = &storage.CommunityNote{
				ID:               note.ID,
				ObjectID:         note.ObjectID,
				ObjectType:       note.ObjectType,
				AuthorID:         note.AuthorID,
				Content:          note.Content,
				Language:         note.Language,
				Sources:          sources,
				HelpfulVotes:     note.HelpfulVotes,
				NotHelpfulVotes:  note.NotHelpfulVotes,
				Score:            note.Score,
				VisibilityStatus: string(note.VisibilityStatus),
				CreatedAt:        note.CreatedAt,
				UpdatedAt:        note.UpdatedAt,
			}
		}
	} else {
		rankedNotes = visibleNotes
	}

	// Format response
	formattedNotes := make([]map[string]interface{}, len(rankedNotes))
	for i, note := range rankedNotes {
		noteData := map[string]interface{}{
			"id":                note.ID,
			"object_id":         note.ObjectID,
			"author_id":         note.AuthorID,
			"content":           note.Content,
			"language":          note.Language,
			"sources":           note.Sources,
			"helpful_votes":     note.HelpfulVotes,
			"not_helpful_votes": note.NotHelpfulVotes,
			"score":             note.Score,
			"visibility_status": note.VisibilityStatus,
			"created_at":        note.CreatedAt,
		}

		formattedNotes[i] = noteData
	}

	response := map[string]interface{}{
		"notes": formattedNotes,
		"stats": calculateNotesStats(rankedNotes),
	}

	// Add cost tracking
	resp := common.OK(response)
	resp.Headers["X-Cost-Micros"] = fmt.Sprintf("%d", 100*len(rankedNotes))
	resp.Headers["X-Cost-Details"] = fmt.Sprintf("DynamoDB: %d reads", len(rankedNotes))

	return resp, nil
}

// HandleVoteNote handles POST /api/v1/notes/:id/vote
func (h *Handler) HandleVoteNote(ctx context.Context, request events.APIGatewayV2HTTPRequest, noteID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	if noteID == "" {
		return common.BadRequest(errors.New("note ID required")), nil
	}

	userID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, claims.Username)

	// Parse vote
	var req notes.VoteRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return common.BadRequest(errors.New("invalid request body")), nil
	}

	// Initialize services
	repService, err := h.getNoteReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get user's reputation
	rep, err := repService.GetReputation(ctx, userID)
	if err != nil || rep.TotalScore < notes.MinReputationToVote {
		return common.Forbidden(errors.New("insufficient reputation to vote")), nil
	}

	// Check if note exists
	note, err := h.store.GetCommunityNote(ctx, noteID)
	if err != nil {
		return common.NotFound(errors.New("note not found")), nil
	}

	// Can't vote on your own notes
	if note.AuthorID == userID {
		return common.Forbidden(errors.New("cannot vote on your own notes")), nil
	}

	// Calculate vote weight
	weight := notes.CalculateVoteWeight(float64(rep.TotalScore), req.VoteType)

	vote := &storage.CommunityNoteVote{
		NoteID:    noteID,
		VoterID:   userID,
		VoteType:  string(req.VoteType),
		Weight:    weight,
		CreatedAt: time.Now(),
	}

	// Store vote
	if err := h.store.CreateCommunityNoteVote(ctx, vote); err != nil {
		h.logger.Error("Failed to store vote", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	response := map[string]interface{}{
		"vote":    vote,
		"note_id": noteID,
	}

	// Add cost tracking
	resp := common.OK(response)
	resp.Headers["X-Cost-Micros"] = "300"
	resp.Headers["X-Cost-Details"] = "DynamoDB: 2 reads, 1 write"

	return resp, nil
}

// HandleGetUserNotes handles GET /api/v1/accounts/:id/notes
func (h *Handler) HandleGetUserNotes(ctx context.Context, request events.APIGatewayV2HTTPRequest, username string) (*events.APIGatewayV2HTTPResponse, error) {
	if username == "" {
		return common.BadRequest(errors.New("username required")), nil
	}

	// Convert username to actor ID
	authorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, username)

	// Parse limit
	limit := 20
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	// Get notes from storage
	userNotes, _, err := h.store.GetCommunityNotesByAuthor(ctx, authorID, limit, "")
	if err != nil {
		h.logger.Error("Failed to get user notes", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert notes to Mastodon status format
	statuses := make([]interface{}, len(userNotes))
	for i, note := range userNotes {
		// Convert note to status-like format
		status := map[string]interface{}{
			"id":         note.ID,
			"content":    fmt.Sprintf("<p>Community Note: %s</p>", note.Content),
			"created_at": note.CreatedAt.Format(time.RFC3339),
			"account": map[string]interface{}{
				"id":       note.AuthorID,
				"username": username,
				"acct":     username,
			},
			"visibility":        "public",
			"sensitive":         false,
			"spoiler_text":      "",
			"media_attachments": []interface{}{},
			"mentions":          []interface{}{},
			"tags":              []interface{}{},
			"emojis":            []interface{}{},
			"reblogs_count":     0,
			"favourites_count":  note.HelpfulVotes,
			"replies_count":     note.NotHelpfulVotes,
			"url":               fmt.Sprintf("https://%s/notes/%s", h.cfg.Domain, note.ID),
			"card": map[string]interface{}{
				"type":        "note",
				"object_id":   note.ObjectID,
				"object_type": note.ObjectType,
				"score":       note.Score,
				"sources":     nil, // Sources are not in storage.CommunityNote
			},
		}
		statuses[i] = status
	}

	// Return as an array of statuses (Mastodon format)
	// Add cost tracking
	resp := common.OK(statuses)
	resp.Headers["X-Cost-Micros"] = fmt.Sprintf("%d", 100*len(userNotes))
	resp.Headers["X-Cost-Details"] = fmt.Sprintf("DynamoDB: %d reads", len(userNotes))

	return resp, nil
}

// Helper functions

// Helper method to get reputation service
func (h *Handler) getNoteReputationService() (*reputation.Service, error) {
	// Create service config using existing store
	cfg := &reputation.Config{
		Storage:     h.store,
		Logger:      h.logger,
		InstanceURL: h.cfg.BaseURL(),
		PrivateKey:  "", // TODO: Load from environment/config
	}

	return reputation.NewService(cfg)
}

func calculateNotesStats(notes []*storage.CommunityNote) map[string]interface{} {
	if len(notes) == 0 {
		return map[string]interface{}{
			"total":           0,
			"visible":         0,
			"average_score":   0,
			"average_helpful": 0,
		}
	}

	totalScore := 0.0
	totalHelpful := 0
	totalNotHelpful := 0
	visibleCount := 0

	for _, note := range notes {
		totalScore += note.Score
		totalHelpful += note.HelpfulVotes
		totalNotHelpful += note.NotHelpfulVotes
		if note.VisibilityStatus == "visible" {
			visibleCount++
		}
	}

	return map[string]interface{}{
		"total":           len(notes),
		"visible":         visibleCount,
		"average_score":   totalScore / float64(len(notes)),
		"average_helpful": float64(totalHelpful) / float64(len(notes)),
	}
}
