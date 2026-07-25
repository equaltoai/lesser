package handlers

import (
	"fmt"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/notes"
	"github.com/equaltoai/lesser/pkg/reputation"
	"github.com/equaltoai/lesser/pkg/security/htmlsafe"
	servicenotes "github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

// authenticateNotesUser handles authentication for notes endpoints with userID formatting
func (h *Handler) authenticateNotesUser(ctx *apptheory.Context) (string, *apptheory.Response, error) {
	claims, resp, err := h.authenticatedClaimsWithResponder(
		ctx,
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
	)
	if resp != nil || err != nil {
		return "", resp, err
	}

	return fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, claims.Username), nil, nil
}

// HandleCreateNoteLift handles POST /api/v1/notes
func (h *Handler) HandleCreateNoteLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	userID, resp, err := h.authenticateNotesUser(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Initialize services
	repService, err := h.getNoteReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Check user's reputation
	rep, err := repService.GetReputation(ctx.Context(), userID)
	if err != nil || rep.TotalScore < notes.MinReputationToCreateNotes {
		return common.RespondForbidden(ctx, "insufficient reputation to create notes")
	}

	req, err := apptheory.BindRequest[apimodels.CreateCommunityNoteRequest](ctx, apptheory.BindConfig[apimodels.CreateCommunityNoteRequest]{
		Body: true,
	})
	if err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}
	if err := validateCreateCommunityNoteRequest(&req); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Validate sources
	if len(req.Sources) > notes.MaxSources {
		return common.RespondBadRequest(ctx, fmt.Sprintf("maximum %d sources allowed", notes.MaxSources))
	}

	// Check rate limit based on reputation
	limit := notes.CalculateNoteLimit(float64(rep.TotalScore))

	// Use the notes service to check rate limiting
	notesService := notes.NewService(h.repos, h.logger)
	canCreate, remaining := notesService.CheckRateLimit(ctx.Context(), userID, float64(rep.TotalScore))

	if !canCreate {
		return common.RespondRateLimited(ctx)
	}

	// Convert Source structs to string URLs
	sourceURLs := make([]string, len(req.Sources))
	for i, src := range req.Sources {
		sourceURLs[i] = strings.TrimSpace(src.URL)
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
	created, err := h.registry.Notes().CreateCommunityNote(ctx.Context(), &servicenotes.CreateCommunityNoteCommand{
		Note: note,
	})
	if err != nil {
		h.logger.Error("Failed to store note", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}
	if created != nil && created.Note != nil {
		note = created.Note
	}
	note.Content = safeCommunityNoteHTML(note.Content)

	// Add rate limit info to response
	response := apimodels.CreateCommunityNoteResponse{
		Note: note,
		RateLimit: apimodels.CommunityNoteRateLimit{
			Limit:     limit,
			Remaining: remaining - 1,
			Reset:     "24h",
		},
	}

	resp, err = apptheory.CreatedJSON(response)
	if err != nil {
		return nil, err
	}
	resp.SetHeader("X-Cost-Micros", "2000")
	resp.SetHeader("X-Cost-Details", "DynamoDB: 2 writes")
	return resp, nil
}

// HandleGetNotesLift handles GET /api/v1/notes/:object_id
func (h *Handler) HandleGetNotesLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	objectID := ctx.Param("object_id")
	if err := common.ValidateRequiredParam("object_id", objectID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Optional auth - for personalized scoring
	userID := ""
	if username := h.getOptionalAuthenticatedUser(ctx); username != "" {
		userID = fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, username)
	}

	// Get visible notes for the object using Notes service
	result, err := h.registry.Notes().GetVisibleCommunityNotes(ctx.Context(), &servicenotes.GetVisibleCommunityNotesQuery{
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
			Content:          safeCommunityNoteHTML(note.Content),
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

	resp, err := okJSON(response)
	if err != nil {
		return nil, err
	}
	setHeader(resp, "X-Cost-Micros", fmt.Sprintf("%d", 100*len(rankedNotes)))
	setHeader(resp, "X-Cost-Details", fmt.Sprintf("DynamoDB: %d reads", len(rankedNotes)))
	return resp, nil
}

// HandleVoteNoteLift handles POST /api/v1/notes/:id/vote
func (h *Handler) HandleVoteNoteLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	noteID := ctx.Param("id")
	if err := common.ValidateRequiredParam("id", noteID); err != nil {
		return common.RespondBadRequest(ctx, "note ID required")
	}

	userID, resp, err := h.authenticateNotesUser(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Parse vote with fallback pattern
	var req apimodels.VoteCommunityNoteRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}
	if err := validateVoteCommunityNoteRequest(&req); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Initialize services
	repService, err := h.getNoteReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Get user's reputation
	rep, err := repService.GetReputation(ctx.Context(), userID)
	if err != nil || rep.TotalScore < notes.MinReputationToVote {
		return common.RespondForbidden(ctx, "insufficient reputation to vote")
	}

	// Check if note exists using Notes service
	noteResult, err := h.registry.Notes().GetCommunityNote(ctx.Context(), &servicenotes.GetCommunityNoteQuery{
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
		Rationale: strings.TrimSpace(req.Reason),
		CreatedAt: time.Now(),
	}

	// Store vote using Notes service
	if _, err := h.registry.Notes().CreateCommunityNoteVote(ctx.Context(), &servicenotes.CreateCommunityNoteVoteCommand{
		Vote: vote,
	}); err != nil {
		h.logger.Error("Failed to store vote", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	response := apimodels.VoteCommunityNoteResponse{
		Vote:   vote,
		NoteID: noteID,
	}

	resp, err = okJSON(response)
	if err != nil {
		return nil, err
	}
	setHeader(resp, "X-Cost-Micros", "300")
	setHeader(resp, "X-Cost-Details", "DynamoDB: 2 reads, 1 write")
	return resp, nil
}

// HandleGetUserNotesLift handles GET /api/v1/accounts/:id/notes
func (h *Handler) HandleGetUserNotesLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	username := ctx.Param("id")
	if err := common.ValidateRequiredParam("id", username); err != nil {
		return common.RespondBadRequest(ctx, "username required")
	}

	// Convert username to actor ID
	authorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, username)

	// Parse limit with fallback
	limit := 20
	limitStr := queryValue(ctx, "limit")
	if limitStr != "" {
		parsed, err := common.ParseAdminLimit(limitStr)
		if err != nil {
			return common.RespondValidationError(ctx, err)
		}
		limit = parsed
	}

	// Get notes from storage using Notes service
	result, err := h.registry.Notes().GetCommunityNotesByAuthor(ctx.Context(), &servicenotes.GetCommunityNotesByAuthorQuery{
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
			Content:   fmt.Sprintf("<p>Community Note: %s</p>", safeCommunityNoteHTML(note.Content)),
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
	resp, err := okJSON(statuses)
	if err != nil {
		return nil, err
	}
	setHeader(resp, "X-Cost-Micros", fmt.Sprintf("%d", 100*len(userNotes)))
	setHeader(resp, "X-Cost-Details", fmt.Sprintf("DynamoDB: %d reads", len(userNotes)))
	return resp, nil
}

func validateCreateCommunityNoteRequest(req *apimodels.CreateCommunityNoteRequest) error {
	if req == nil {
		return common.ValidationError{Field: "body", Message: "required"}
	}
	req.ObjectID = strings.TrimSpace(req.ObjectID)
	req.ObjectType = strings.TrimSpace(req.ObjectType)
	req.Content = strings.TrimSpace(req.Content)
	req.Language = strings.TrimSpace(req.Language)

	if err := common.ValidateRequiredParam("object_id", req.ObjectID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("object_type", req.ObjectType); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("content", req.Content); err != nil {
		return err
	}
	if len(req.Content) < notes.MinNoteLength {
		return common.ValidationError{Field: "content", Message: fmt.Sprintf("must be at least %d characters", notes.MinNoteLength)}
	}
	if len(req.Content) > notes.MaxNoteLength {
		return common.ValidationError{Field: "content", Message: fmt.Sprintf("cannot be longer than %d characters", notes.MaxNoteLength)}
	}
	if len(req.Language) != 2 {
		return common.ValidationError{Field: "language", Message: "must be a 2-letter code"}
	}
	if len(req.Sources) > notes.MaxSources {
		return common.ValidationError{Field: "sources", Message: fmt.Sprintf("maximum %d sources allowed", notes.MaxSources)}
	}
	for i := range req.Sources {
		req.Sources[i].URL = strings.TrimSpace(req.Sources[i].URL)
		if err := common.ValidateRequiredParam(fmt.Sprintf("sources[%d].url", i), req.Sources[i].URL); err != nil {
			return err
		}
		if err := common.ValidateURL(req.Sources[i].URL, fmt.Sprintf("sources[%d].url", i)); err != nil {
			return err
		}
	}
	return nil
}

func validateVoteCommunityNoteRequest(req *apimodels.VoteCommunityNoteRequest) error {
	if req == nil {
		return common.ValidationError{Field: "body", Message: "required"}
	}
	req.VoteType = strings.TrimSpace(req.VoteType)
	req.Reason = strings.TrimSpace(req.Reason)

	switch notes.VoteType(req.VoteType) {
	case notes.VoteHelpful, notes.VoteNotHelpful, notes.VoteNeutral:
	default:
		return common.ValidationError{Field: "vote_type", Message: "must be one of helpful, not_helpful, neutral"}
	}
	if len(req.Reason) > notes.MaxReasonLength {
		return common.ValidationError{Field: "reason", Message: fmt.Sprintf("cannot be longer than %d characters", notes.MaxReasonLength)}
	}
	return nil
}

func safeCommunityNoteHTML(content string) string {
	return htmlsafe.EscapePlainTextHTML(content)
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
