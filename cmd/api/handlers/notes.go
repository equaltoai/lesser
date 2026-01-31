package lift

import (
	"encoding/json"
	"fmt"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/notes"
	"github.com/equaltoai/lesser/pkg/reputation"
	servicenotes "github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// authenticateNotesUser handles authentication for notes endpoints with userID formatting
func (h *Handler) authenticateNotesUser(ctx *lift.Context) (string, bool, error) {
	// Extract and validate token
	authHeader := ctx.Header("Authorization")
	if common.ValidateRequiredParam("authHeader", authHeader) != nil {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if common.ValidateRequiredParam("authHeader", authHeader) != nil && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if common.ValidateRequiredParam("authHeader", authHeader) != nil {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		_ = common.RespondUnauthorized(ctx)
		return "", true, nil
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		_ = common.RespondUnauthorized(ctx)
		return "", true, nil
	}

	return fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, claims.Username), false, nil
}

// HandleCreateNoteLift handles POST /api/v1/notes
func (h *Handler) HandleCreateNoteLift(ctx *lift.Context) error {
	userID, handled, err := h.authenticateNotesUser(ctx)
	if err != nil || handled {
		return err
	}

	// Initialize services
	repService, err := h.getNoteReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Check user's reputation
	rep, err := repService.GetReputation(ctx.Context, userID)
	if err != nil || rep.TotalScore < notes.MinReputationToCreateNotes {
		return common.RespondForbidden(ctx, "insufficient reputation to create notes")
	}

	// Parse request with fallback pattern
	var req apimodels.CreateCommunityNoteRequest
	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if jsonErr := json.Unmarshal(ctx.Request.Body, &req); jsonErr != nil {
				return common.RespondBadRequest(ctx, "invalid request body")
			}
		} else {
			return common.RespondBadRequest(ctx, "invalid request body")
		}
	}

	// Validate sources
	if len(req.Sources) > notes.MaxSources {
		return common.RespondBadRequest(ctx, fmt.Sprintf("maximum %d sources allowed", notes.MaxSources))
	}

	// Check rate limit based on reputation
	limit := notes.CalculateNoteLimit(float64(rep.TotalScore))

	// Use the notes service to check rate limiting
	notesService := notes.NewService(h.repos, h.logger)
	canCreate, remaining := notesService.CheckRateLimit(ctx.Context, userID, float64(rep.TotalScore))

	if !canCreate {
		return common.RespondRateLimited(ctx)
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

	// Store note using Notes service
	if _, err := h.registry.Notes().CreateCommunityNote(ctx.Context, &servicenotes.CreateCommunityNoteCommand{
		Note: note,
	}); err != nil {
		h.logger.Error("Failed to store note", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Add rate limit info to response
	response := apimodels.CreateCommunityNoteResponse{
		Note: note,
		RateLimit: apimodels.CommunityNoteRateLimit{
			Limit:     limit,
			Remaining: remaining - 1,
			Reset:     "24h",
		},
	}

	// Add cost tracking headers and return
	ctx.Response.Headers = map[string]string{
		"X-Cost-Micros":  "2000",
		"X-Cost-Details": "DynamoDB: 2 writes",
	}
	return ctx.Status(201).JSON(response)
}

// HandleGetNotesLift handles GET /api/v1/notes/:object_id
func (h *Handler) HandleGetNotesLift(ctx *lift.Context) error {
	objectID := ctx.Param("object_id")
	if err := common.ValidateRequiredParam("object_id", objectID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Optional auth - for personalized scoring
	var userID string
	authHeader := ctx.Header("Authorization")
	if common.ValidateRequiredParam("authHeader", authHeader) != nil {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if common.ValidateRequiredParam("authHeader", authHeader) != nil && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if common.ValidateRequiredParam("authHeader", authHeader) != nil {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
			claims, err := oauthSvc.ValidateAccessToken(token)
			if err == nil {
				userID = fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, claims.Username)
			}
		}
	}

	// Get visible notes for the object using Notes service
	result, err := h.registry.Notes().GetVisibleCommunityNotes(ctx.Context, &servicenotes.GetVisibleCommunityNotesQuery{
		ObjectID: objectID,
	})
	if err != nil {
		h.logger.Error("Failed to get visible notes", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}
	visibleNotes := result.Notes

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
		// Use reputation service to calculate trust scores
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
	formattedNotes := make([]apimodels.CommunityNoteSummary, len(rankedNotes))
	for i, note := range rankedNotes {
		noteData := apimodels.CommunityNoteSummary{
			ID:               note.ID,
			ObjectID:         note.ObjectID,
			AuthorID:         note.AuthorID,
			Content:          note.Content,
			Language:         note.Language,
			Sources:          note.Sources,
			HelpfulVotes:     note.HelpfulVotes,
			NotHelpfulVotes:  note.NotHelpfulVotes,
			Score:            note.Score,
			VisibilityStatus: note.VisibilityStatus,
			CreatedAt:        note.CreatedAt,
		}

		formattedNotes[i] = noteData
	}

	response := apimodels.CommunityNotesResponse{
		Notes: formattedNotes,
		Stats: calculateNotesStats(rankedNotes),
	}

	// Add cost tracking and return
	ctx.Response.Headers = map[string]string{
		"X-Cost-Micros":  fmt.Sprintf("%d", 100*len(rankedNotes)),
		"X-Cost-Details": fmt.Sprintf("DynamoDB: %d reads", len(rankedNotes)),
	}
	return ctx.Status(200).JSON(response)
}

// HandleVoteNoteLift handles POST /api/v1/notes/:id/vote
func (h *Handler) HandleVoteNoteLift(ctx *lift.Context) error {
	noteID := ctx.Param("id")
	if err := common.ValidateRequiredParam("id", noteID); err != nil {
		return common.RespondBadRequest(ctx, "note ID required")
	}

	userID, handled, err := h.authenticateNotesUser(ctx)
	if err != nil || handled {
		return err
	}

	// Parse vote with fallback pattern
	var req apimodels.VoteCommunityNoteRequest
	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if jsonErr := json.Unmarshal(ctx.Request.Body, &req); jsonErr != nil {
				return common.RespondBadRequest(ctx, "invalid request body")
			}
		} else {
			return common.RespondBadRequest(ctx, "invalid request body")
		}
	}

	// Initialize services
	repService, err := h.getNoteReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Get user's reputation
	rep, err := repService.GetReputation(ctx.Context, userID)
	if err != nil || rep.TotalScore < notes.MinReputationToVote {
		return common.RespondForbidden(ctx, "insufficient reputation to vote")
	}

	// Check if note exists using Notes service
	noteResult, err := h.registry.Notes().GetCommunityNote(ctx.Context, &servicenotes.GetCommunityNoteQuery{
		NoteID: noteID,
	})
	if err != nil {
		return common.RespondNotFound(ctx, "note not found")
	}
	note := noteResult.Note

	// Can't vote on your own notes
	if note.AuthorID == userID {
		return common.RespondForbidden(ctx, "cannot vote on your own notes")
	}

	// Calculate vote weight
	weight := notes.CalculateVoteWeight(float64(rep.TotalScore), notes.VoteType(req.VoteType))

	vote := &storage.CommunityNoteVote{
		NoteID:    noteID,
		VoterID:   userID,
		VoteType:  req.VoteType,
		Weight:    weight,
		CreatedAt: time.Now(),
	}

	// Store vote using Notes service
	if _, err := h.registry.Notes().CreateCommunityNoteVote(ctx.Context, &servicenotes.CreateCommunityNoteVoteCommand{
		Vote: vote,
	}); err != nil {
		h.logger.Error("Failed to store vote", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	response := apimodels.VoteCommunityNoteResponse{
		Vote:   vote,
		NoteID: noteID,
	}

	// Add cost tracking and return
	ctx.Response.Headers = map[string]string{
		"X-Cost-Micros":  "300",
		"X-Cost-Details": "DynamoDB: 2 reads, 1 write",
	}
	return ctx.Status(200).JSON(response)
}

// HandleGetUserNotesLift handles GET /api/v1/accounts/:id/notes
func (h *Handler) HandleGetUserNotesLift(ctx *lift.Context) error {
	username := ctx.Param("id")
	if err := common.ValidateRequiredParam("id", username); err != nil {
		return common.RespondBadRequest(ctx, "username required")
	}

	// Convert username to actor ID
	authorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, username)

	// Parse limit with fallback
	limit := 20
	limitStr := ctx.Query("limit")
	if common.ValidateRequiredParam("limitStr", limitStr) != nil && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	if limitStr != "" {
		parsed, err := common.ParseAdminLimit(limitStr)
		if err != nil {
			return common.RespondValidationError(ctx, err)
		}
		limit = parsed
	}

	// Get notes from storage using Notes service
	result, err := h.registry.Notes().GetCommunityNotesByAuthor(ctx.Context, &servicenotes.GetCommunityNotesByAuthorQuery{
		AuthorID: authorID,
		Limit:    limit,
		Cursor:   "",
	})
	if err != nil {
		h.logger.Error("Failed to get user notes", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}
	userNotes := result.Notes

	// Convert notes to Mastodon status format
	statuses := make([]apimodels.UserNoteStatus, len(userNotes))
	for i, note := range userNotes {
		statuses[i] = apimodels.UserNoteStatus{
			ID:        note.ID,
			Content:   fmt.Sprintf("<p>Community Note: %s</p>", note.Content),
			CreatedAt: note.CreatedAt.Format(time.RFC3339),
			Account: apimodels.UserNoteAccount{
				ID:       note.AuthorID,
				Username: username,
				Acct:     username,
			},
			Visibility:       "public",
			Sensitive:        false,
			SpoilerText:      "",
			MediaAttachments: []any{},
			Mentions:         []any{},
			Tags:             []any{},
			Emojis:           []any{},
			ReblogsCount:     0,
			FavouritesCount:  note.HelpfulVotes,
			RepliesCount:     note.NotHelpfulVotes,
			URL:              fmt.Sprintf("https://%s/notes/%s", h.cfg.Domain, note.ID),
			Card: apimodels.UserNoteCard{
				Type:       "note",
				ObjectID:   note.ObjectID,
				ObjectType: note.ObjectType,
				Score:      note.Score,
			},
		}
	}

	// Return as an array of statuses (Mastodon format)
	// Add cost tracking and return
	ctx.Response.Headers = map[string]string{
		"X-Cost-Micros":  fmt.Sprintf("%d", 100*len(userNotes)),
		"X-Cost-Details": fmt.Sprintf("DynamoDB: %d reads", len(userNotes)),
	}
	return ctx.Status(200).JSON(statuses)
}

// Helper functions

// Helper method to get reputation service
func (h *Handler) getNoteReputationService() (*reputation.Service, error) {
	// Create service config using existing store
	cfg := &reputation.Config{
		Storage:     h.repos,
		Logger:      h.logger,
		InstanceURL: h.cfg.BaseURL(),
		PrivateKey:  h.cfg.ReputationPrivateKey, // Load from config
	}

	return reputation.NewService(cfg)
}

func calculateNotesStats(notes []*storage.CommunityNote) apimodels.CommunityNoteStats {
	if err := common.ValidateSliceNotEmpty("notes", notes); err != nil {
		return apimodels.CommunityNoteStats{}
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

	return apimodels.CommunityNoteStats{
		Total:          len(notes),
		Visible:        visibleCount,
		AverageScore:   totalScore / float64(len(notes)),
		AverageHelpful: float64(totalHelpful) / float64(len(notes)),
	}
}
