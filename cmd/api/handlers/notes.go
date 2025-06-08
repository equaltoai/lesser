package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/cost"
	"github.com/aron23/lesser/pkg/notes"
	"github.com/aron23/lesser/pkg/reputation"
	"github.com/aron23/lesser/pkg/trust"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
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
	canCreate, remaining := notes.CheckNoteRateLimit(ctx, userID, limit)
	if !canCreate {
		return common.UnprocessableEntity(fmt.Errorf("note limit reached. %d notes allowed per day based on reputation", limit)), nil
	}

	// Create note
	note := &notes.CommunityNote{
		ID:               notes.GenerateNoteID(),
		ObjectID:         req.ObjectID,
		ObjectType:       req.ObjectType,
		AuthorID:         userID,
		AuthorRep:        float64(rep.TotalScore),
		Content:          req.Content,
		Language:         req.Language,
		Sources:          validateSources(req.Sources),
		Score:            0,
		VisibilityStatus: notes.VisibilityPending,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Store note
	if err := notes.StoreNote(ctx, note); err != nil {
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

	// Query notes
	visibleNotes, err := notes.GetVisibleNotes(ctx, objectID)
	if err != nil {
		h.logger.Error("Failed to get notes", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// If authenticated, enhance with user data
	var userVotes map[string]notes.Vote
	if userID != "" && len(visibleNotes) > 0 {
		noteIDs := make([]string, len(visibleNotes))
		for i, note := range visibleNotes {
			noteIDs[i] = note.ID
		}
		userVotes, _ = notes.GetUserVotes(ctx, userID, noteIDs)

		// Get trust scores if we have a trust service
		trustService, err := h.getTrustService()
		if err == nil {
			trustScores := make(map[string]float64)
			for _, note := range visibleNotes {
				if score, err := trustService.GetTrustScore(ctx, userID, note.AuthorID); err == nil && score != nil {
					trustScores[note.AuthorID] = score.Score
				}
			}
			// Rank by trust
			visibleNotes = notes.RankNotesByTrust(visibleNotes, userID, trustScores)
		}
	}

	// Format response
	formattedNotes := make([]map[string]interface{}, len(visibleNotes))
	for i, note := range visibleNotes {
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

		// Add user's vote if available
		if userVotes != nil {
			if vote, exists := userVotes[note.ID]; exists {
				noteData["user_vote"] = vote.VoteType
			}
		}

		formattedNotes[i] = noteData
	}

	response := map[string]interface{}{
		"notes": formattedNotes,
		"stats": notes.CalculateStats(visibleNotes),
	}

	// Add cost tracking
	resp := common.OK(response)
	resp.Headers["X-Cost-Micros"] = fmt.Sprintf("%d", 100*len(visibleNotes))
	resp.Headers["X-Cost-Details"] = fmt.Sprintf("DynamoDB: %d reads", len(visibleNotes))

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
	note, err := notes.GetNote(ctx, noteID)
	if err != nil {
		return common.NotFound(errors.New("note not found")), nil
	}

	// Can't vote on your own notes
	if note.AuthorID == userID {
		return common.Forbidden(errors.New("cannot vote on your own notes")), nil
	}

	// Calculate vote weight
	weight := notes.CalculateVoteWeight(float64(rep.TotalScore), req.VoteType)

	vote := &notes.Vote{
		NoteID:    noteID,
		VoterID:   userID,
		VoterRep:  float64(rep.TotalScore),
		VoteType:  req.VoteType,
		Reason:    req.Reason,
		Weight:    weight,
		CreatedAt: time.Now(),
	}

	// Store vote
	if err := notes.StoreVote(ctx, vote); err != nil {
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
	limit := int32(20)
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = int32(parsed)
		}
	}

	// Get notes
	userNotes, err := notes.GetNotesByAuthor(ctx, authorID, limit)
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
				"sources":     note.Sources,
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
	// Create AWS config
	awsCfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create DynamoDB client
	db := dynamodb.NewFromConfig(awsCfg)

	// Create cost tracker
	costTracker := cost.New()

	// Create service config
	cfg := &reputation.Config{
		DynamoClient:   db,
		Storage:        h.store,
		Logger:         h.logger,
		CostTracker:    costTracker,
		InstanceURL:    h.cfg.BaseURL(),
		PrivateKey:     "",                    // TODO: Load from environment/config
		RepTableName:   h.cfg.DynamoTableName, // Use the main table
		VouchTableName: h.cfg.DynamoTableName, // Use the main table
	}

	return reputation.NewService(cfg)
}

func (h *Handler) getTrustService() (*trust.Service, error) {
	// Create AWS config
	awsCfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create DynamoDB client
	db := dynamodb.NewFromConfig(awsCfg)

	return trust.NewService(db), nil
}

// validateSources validates and cleans source URLs
func validateSources(sources []notes.Source) []notes.Source {
	cleaned := make([]notes.Source, 0, len(sources))

	for _, source := range sources {
		// Parse and validate URL
		u, err := url.Parse(source.URL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			continue
		}

		// Clean up source
		cleanSource := notes.Source{
			URL:         u.String(),
			Title:       source.Title,
			Domain:      u.Host,
			Reliability: 0.5, // Initial reliability score
		}

		cleaned = append(cleaned, cleanSource)
	}

	return cleaned
}
