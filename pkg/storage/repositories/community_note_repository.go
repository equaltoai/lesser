package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// CommunityNoteRepository implements community note operations using enhanced repository patterns
type CommunityNoteRepository struct {
	*EnhancedBaseRepository[*models.CommunityNote]
}

// NewCommunityNoteRepository creates a new community note repository with enhanced functionality
func NewCommunityNoteRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *CommunityNoteRepository {
	// Create enhanced repository optimized for community note operations
	enhancedRepo := NewEnhancedBaseRepository[*models.CommunityNote](db, tableName, logger, costService, "CommunityNoteRepository", "community_note")

	// Set up enhanced services for community moderation
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService()) // Community moderation permissions
	enhancedRepo.SetCachingService(NewInMemoryCachingService())      // Cache community notes for performance
	enhancedRepo.SetEventService(NewDefaultEventService())           // Community moderation events

	return &CommunityNoteRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// GetUserVotingHistory retrieves a user's voting history for reputation calculation - COMMUNITY NOTES BUSINESS LOGIC
func (r *CommunityNoteRepository) GetUserVotingHistory(ctx context.Context, userID string, limit int) ([]*storage.CommunityNoteVote, error) {
	var votes []models.CommunityNoteVote

	// Query using GSI to get all votes by this user - preserve community voting history functionality
	err := r.GetDB().WithContext(ctx).Model(&models.CommunityNoteVote{}).
		Index("gsi1").
		Where("gsi1PK", "=", "VOTES#"+userID).
		OrderBy("gsi1SK", "DESC"). // Most recent first
		Limit(limit).
		All(&votes)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "community note", "user voting history")
	}

	// Convert to storage models - preserve exact conversion logic
	result := make([]*storage.CommunityNoteVote, len(votes))
	for i, model := range votes {
		result[i] = &storage.CommunityNoteVote{
			NoteID:    model.NoteID,
			VoterID:   model.VoterID,
			VoteType:  model.VoteType,
			Helpful:   model.Helpful,
			Weight:    model.Weight,
			CreatedAt: model.CreatedAt,
		}
	}

	return result, nil
}

// CreateCommunityNote creates a new community note - COMMUNITY NOTES BUSINESS LOGIC
func (r *CommunityNoteRepository) CreateCommunityNote(ctx context.Context, note *storage.CommunityNote) error {
	// Generate ID if not provided - preserve community note creation validation
	if err := common.ValidateRequiredParam("note.ID", note.ID); err != nil {
		note.ID = uuid.New().String()
	}

	// Set timestamps - preserve exact timestamp logic
	now := time.Now()
	note.CreatedAt = now
	note.UpdatedAt = now

	// Create model - preserve all community note fields and validation
	model := &models.CommunityNote{
		ID:               note.ID,
		ObjectID:         note.ObjectID,
		ObjectType:       note.ObjectType,
		AuthorID:         note.AuthorID,
		Content:          note.Content,
		Language:         note.Language,
		Sources:          note.Sources,
		HelpfulVotes:     note.HelpfulVotes,
		NotHelpfulVotes:  note.NotHelpfulVotes,
		Score:            note.Score,
		VisibilityStatus: note.VisibilityStatus,
		Sentiment:        note.Sentiment,
		Objectivity:      note.Objectivity,
		SourceQuality:    note.SourceQuality,
		CreatedAt:        note.CreatedAt,
		UpdatedAt:        note.UpdatedAt,
		TTL:              now.Add(90 * 24 * time.Hour).Unix(), // 90 days TTL - preserve exact TTL
	}

	// Use enhanced validation and creation for community moderation
	err := r.ValidateAndCreate(ctx, model)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, "community note", note.ID)
	}

	return nil
}

// GetCommunityNote retrieves a note by ID - COMMUNITY NOTES BUSINESS LOGIC
func (r *CommunityNoteRepository) GetCommunityNote(ctx context.Context, noteID string) (*storage.CommunityNote, error) {
	var model models.CommunityNote

	// Use BaseRepository Get method for consistent cost tracking and error handling
	err := r.Get(ctx, fmt.Sprintf("NOTE#%s", noteID), "METADATA", &model)
	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, "community note", noteID)
	}

	// Convert to storage type - preserve exact conversion logic
	note := &storage.CommunityNote{
		ID:               model.ID,
		ObjectID:         model.ObjectID,
		ObjectType:       model.ObjectType,
		AuthorID:         model.AuthorID,
		Content:          model.Content,
		Language:         model.Language,
		Sources:          model.Sources,
		HelpfulVotes:     model.HelpfulVotes,
		NotHelpfulVotes:  model.NotHelpfulVotes,
		Score:            model.Score,
		VisibilityStatus: model.VisibilityStatus,
		Sentiment:        model.Sentiment,
		Objectivity:      model.Objectivity,
		SourceQuality:    model.SourceQuality,
		CreatedAt:        model.CreatedAt,
		UpdatedAt:        model.UpdatedAt,
	}

	return note, nil
}

// GetVisibleCommunityNotes retrieves visible notes for an object - COMMUNITY NOTES BUSINESS LOGIC
func (r *CommunityNoteRepository) GetVisibleCommunityNotes(ctx context.Context, objectID string) ([]*storage.CommunityNote, error) {
	var modelsSlice []models.CommunityNote

	// Query by object ID using GSI1 - preserve community visibility filtering
	err := r.GetDB().WithContext(ctx).Model(&models.CommunityNote{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("OBJECT#%s#NOTES", objectID)).
		Limit(50).
		All(&modelsSlice)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.CommunityNote{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, "community note", "visible notes")
	}

	// Filter for visible notes and convert to storage type - PRESERVE COMMUNITY VISIBILITY LOGIC
	notes := make([]*storage.CommunityNote, 0, len(modelsSlice))
	for _, model := range modelsSlice {
		// Only include visible notes - preserve exact visibility filtering
		if model.VisibilityStatus == "visible" || model.VisibilityStatus == "prominent" {
			note := &storage.CommunityNote{
				ID:               model.ID,
				ObjectID:         model.ObjectID,
				ObjectType:       model.ObjectType,
				AuthorID:         model.AuthorID,
				Content:          model.Content,
				Language:         model.Language,
				Sources:          model.Sources,
				HelpfulVotes:     model.HelpfulVotes,
				NotHelpfulVotes:  model.NotHelpfulVotes,
				Score:            model.Score,
				VisibilityStatus: model.VisibilityStatus,
				Sentiment:        model.Sentiment,
				Objectivity:      model.Objectivity,
				SourceQuality:    model.SourceQuality,
				CreatedAt:        model.CreatedAt,
				UpdatedAt:        model.UpdatedAt,
			}
			notes = append(notes, note)
		}
	}

	return notes, nil
}

// UpdateCommunityNoteScore updates a note's score and visibility - COMMUNITY NOTES BUSINESS LOGIC
func (r *CommunityNoteRepository) UpdateCommunityNoteScore(ctx context.Context, noteID string, score float64, status string) error {
	// Get the current note to preserve other fields - preserve community score update logic
	note, err := r.GetCommunityNote(ctx, noteID)
	if err != nil {
		return err
	}

	// Update fields - preserve exact score and visibility update logic
	note.Score = score
	note.VisibilityStatus = status
	note.UpdatedAt = time.Now()

	// Create updated model - preserve all community note fields
	model := &models.CommunityNote{
		ID:               note.ID,
		ObjectID:         note.ObjectID,
		ObjectType:       note.ObjectType,
		AuthorID:         note.AuthorID,
		Content:          note.Content,
		Language:         note.Language,
		Sources:          note.Sources,
		HelpfulVotes:     note.HelpfulVotes,
		NotHelpfulVotes:  note.NotHelpfulVotes,
		Score:            note.Score,
		VisibilityStatus: note.VisibilityStatus,
		Sentiment:        note.Sentiment,
		Objectivity:      note.Objectivity,
		SourceQuality:    note.SourceQuality,
		CreatedAt:        note.CreatedAt,
		UpdatedAt:        note.UpdatedAt,
		TTL:              time.Now().Add(90 * 24 * time.Hour).Unix(), // preserve exact TTL
	}

	// Use BaseRepository Update method for consistent cost tracking
	err = r.Update(ctx, model)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, "community note", noteID)
	}

	return nil
}

// CreateCommunityNoteVote creates a vote on a note - COMMUNITY NOTES BUSINESS LOGIC
func (r *CommunityNoteRepository) CreateCommunityNoteVote(ctx context.Context, vote *storage.CommunityNoteVote) error {
	vote.CreatedAt = time.Now()

	// Create model - preserve community voting logic
	model := &models.CommunityNoteVote{
		NoteID:    vote.NoteID,
		VoterID:   vote.VoterID,
		VoteType:  vote.VoteType,
		Helpful:   vote.Helpful,
		Weight:    vote.Weight,
		CreatedAt: vote.CreatedAt,
		TTL:       time.Now().Add(90 * 24 * time.Hour).Unix(), // preserve exact TTL
	}

	// Create the vote using our own DB connection (no BaseRepository for votes)
	_ = model.UpdateKeys() // Ignore error as this is internal model operation
	err := r.GetDB().WithContext(ctx).Model(model).Create()
	if err != nil {
		return ErrorHandler.HandleCreateError(err, "community note vote", vote.NoteID)
	}

	return nil
}

// GetUserCommunityNoteVotes retrieves a user's votes on specific notes - COMMUNITY NOTES BUSINESS LOGIC
func (r *CommunityNoteRepository) GetUserCommunityNoteVotes(ctx context.Context, userID string, noteIDs []string) (map[string]*storage.CommunityNoteVote, error) {
	votes := make(map[string]*storage.CommunityNoteVote)

	// Batch get votes - preserve individual note vote retrieval logic
	for _, noteID := range noteIDs {
		var model models.CommunityNoteVote
		err := r.GetDB().WithContext(ctx).Model(&models.CommunityNoteVote{}).
			Where("PK", "=", fmt.Sprintf("NOTE#%s", noteID)).
			Where("SK", "=", fmt.Sprintf("VOTE#%s", userID)).
			First(&model)

		if err != nil {
			if errors.IsNotFound(err) {
				// Not found is ok - user hasn't voted on this note - preserve graceful handling
				continue
			}
			// Continue to next vote instead of failing entirely - preserve resilient behavior
			continue
		}

		// Convert to storage type - preserve exact conversion
		vote := &storage.CommunityNoteVote{
			NoteID:    model.NoteID,
			VoterID:   model.VoterID,
			VoteType:  model.VoteType,
			Helpful:   model.Helpful,
			Weight:    model.Weight,
			CreatedAt: model.CreatedAt,
		}
		votes[vote.NoteID] = vote
	}

	return votes, nil
}

// GetCommunityNotesByAuthor retrieves community notes authored by a specific actor - COMMUNITY NOTES BUSINESS LOGIC
func (r *CommunityNoteRepository) GetCommunityNotesByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*storage.CommunityNote, string, error) {
	var modelsSlice []models.CommunityNote

	// Build query using GSI3 - preserve community author query logic
	query := r.GetDB().WithContext(ctx).Model(&models.CommunityNote{}).
		Index("gsi3").
		Where("gsi3PK", "=", fmt.Sprintf("AUTHOR#%s#NOTES", authorID)).
		Limit(limit)

	// Add cursor if provided - preserve exact cursor logic
	if cursor != "" {
		// Parse cursor - expecting format "timestamp#noteID" - preserve exact parsing
		parts := strings.Split(cursor, "#")
		if len(parts) >= 2 {
			query = query.Where("gsi3SK", "<", cursor)
		}
	}

	// Execute query
	err := query.All(&modelsSlice)
	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.CommunityNote{}, "", nil
		}
		return nil, "", ErrorHandler.HandleQueryError(err, "community note", "by author")
	}

	// Convert to storage type - preserve exact conversion
	notes := make([]*storage.CommunityNote, 0, len(modelsSlice))
	for _, model := range modelsSlice {
		note := &storage.CommunityNote{
			ID:               model.ID,
			ObjectID:         model.ObjectID,
			ObjectType:       model.ObjectType,
			AuthorID:         model.AuthorID,
			Content:          model.Content,
			Language:         model.Language,
			Sources:          model.Sources,
			HelpfulVotes:     model.HelpfulVotes,
			NotHelpfulVotes:  model.NotHelpfulVotes,
			Score:            model.Score,
			VisibilityStatus: model.VisibilityStatus,
			Sentiment:        model.Sentiment,
			Objectivity:      model.Objectivity,
			SourceQuality:    model.SourceQuality,
			CreatedAt:        model.CreatedAt,
			UpdatedAt:        model.UpdatedAt,
		}
		notes = append(notes, note)
	}

	// Determine next cursor - preserve exact pagination logic
	var nextCursor string
	if len(modelsSlice) == limit && len(modelsSlice) > 0 {
		lastModel := modelsSlice[len(modelsSlice)-1]
		nextCursor = lastModel.GSI3SK
	}

	return notes, nextCursor, nil
}

// GetCommunityNoteVotes retrieves votes on a specific community note - COMMUNITY NOTES BUSINESS LOGIC
func (r *CommunityNoteRepository) GetCommunityNoteVotes(ctx context.Context, noteID string) ([]*storage.CommunityNoteVote, error) {
	var modelsSlice []models.CommunityNoteVote

	// Query votes for the note - preserve community vote retrieval logic
	err := r.GetDB().WithContext(ctx).Model(&models.CommunityNoteVote{}).
		Where("PK", "=", fmt.Sprintf("NOTE#%s", noteID)).
		Where("SK", "BEGINS_WITH", "VOTE#").
		All(&modelsSlice)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.CommunityNoteVote{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, "community note vote", "by note")
	}

	// Convert to storage type - preserve exact conversion with Helpful flag logic
	votes := make([]*storage.CommunityNoteVote, 0, len(modelsSlice))
	for _, model := range modelsSlice {
		// Set the Helpful flag based on VoteType for easier access - preserve exact logic
		model.Helpful = (model.VoteType == "helpful")

		vote := &storage.CommunityNoteVote{
			NoteID:    model.NoteID,
			VoterID:   model.VoterID,
			VoteType:  model.VoteType,
			Helpful:   model.Helpful,
			Weight:    model.Weight,
			CreatedAt: model.CreatedAt,
		}
		votes = append(votes, vote)
	}

	return votes, nil
}

// UpdateCommunityNoteAnalysis updates AI analysis results for a note - COMMUNITY NOTES BUSINESS LOGIC
func (r *CommunityNoteRepository) UpdateCommunityNoteAnalysis(ctx context.Context, noteID string, sentiment, objectivity, sourceQuality float64) error {
	// Get the current note to preserve other fields - preserve community analysis update logic
	note, err := r.GetCommunityNote(ctx, noteID)
	if err != nil {
		return err
	}

	// Update analysis fields - preserve exact AI analysis field updates
	note.Sentiment = sentiment
	note.Objectivity = objectivity
	note.SourceQuality = sourceQuality
	note.UpdatedAt = time.Now()

	// Create updated model - preserve all community note fields
	model := &models.CommunityNote{
		ID:               note.ID,
		ObjectID:         note.ObjectID,
		ObjectType:       note.ObjectType,
		AuthorID:         note.AuthorID,
		Content:          note.Content,
		Language:         note.Language,
		Sources:          note.Sources,
		HelpfulVotes:     note.HelpfulVotes,
		NotHelpfulVotes:  note.NotHelpfulVotes,
		Score:            note.Score,
		VisibilityStatus: note.VisibilityStatus,
		Sentiment:        note.Sentiment,
		Objectivity:      note.Objectivity,
		SourceQuality:    note.SourceQuality,
		CreatedAt:        note.CreatedAt,
		UpdatedAt:        note.UpdatedAt,
		TTL:              time.Now().Add(90 * 24 * time.Hour).Unix(), // preserve exact TTL
	}

	// Use BaseRepository Update method for consistent cost tracking
	err = r.Update(ctx, model)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, "community note", noteID)
	}

	return nil
}
