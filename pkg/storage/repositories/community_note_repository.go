package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// CommunityNoteRepository implements the community note operations using DynamORM
type CommunityNoteRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewCommunityNoteRepository creates a new community note repository
func NewCommunityNoteRepository(db core.DB, tableName string, logger *zap.Logger) *CommunityNoteRepository {
	return &CommunityNoteRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// GetDB returns the database connection for shared use
func (r *CommunityNoteRepository) GetDB() core.DB {
	return r.db
}

// GetTableName returns the table name for shared use
func (r *CommunityNoteRepository) GetTableName() string {
	return r.tableName
}

// GetUserVotingHistory retrieves a user's voting history for reputation calculation
func (r *CommunityNoteRepository) GetUserVotingHistory(ctx context.Context, userID string, limit int) ([]*storage.CommunityNoteVote, error) {
	var votes []models.CommunityNoteVote
	
	// Query using GSI to get all votes by this user
	err := r.db.WithContext(ctx).Model(&models.CommunityNoteVote{}).
		Index("user-votes-index").
		Where("GSI1PK", "=", "VOTES#"+userID).
		OrderBy("GSI1SK", "DESC"). // Most recent first
		Limit(limit).
		All(&votes)
		
	if err != nil {
		return nil, fmt.Errorf("failed to get user voting history: %w", err)
	}
	
	// Convert to storage models
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

// CreateCommunityNote creates a new community note
func (r *CommunityNoteRepository) CreateCommunityNote(_ context.Context, note *storage.CommunityNote) error {
	// Generate ID if not provided
	if note.ID == "" {
		note.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	note.CreatedAt = now
	note.UpdatedAt = now

	// Create model
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
		TTL:              now.Add(90 * 24 * time.Hour).Unix(), // 90 days TTL
	}

	// Update keys
	model.UpdateKeys()

	// Create in DynamoDB
	err := r.db.Model(model).Create()
	if err != nil {
		r.logger.Error("failed to create community note",
			zap.String("noteID", note.ID),
			zap.String("objectID", note.ObjectID),
			zap.String("authorID", note.AuthorID),
			zap.Error(err))
		return fmt.Errorf("failed to create community note: %w", err)
	}

	r.logger.Debug("Created community note",
		zap.String("noteID", note.ID),
		zap.String("objectID", note.ObjectID),
		zap.String("authorID", note.AuthorID))

	return nil
}

// GetCommunityNote retrieves a note by ID
func (r *CommunityNoteRepository) GetCommunityNote(_ context.Context, noteID string) (*storage.CommunityNote, error) {
	var model models.CommunityNote
	err := r.db.Model(&models.CommunityNote{}).
		Where("PK", "=", fmt.Sprintf("NOTE#%s", noteID)).
		Where("SK", "=", "METADATA").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("note not found")
		}
		r.logger.Error("failed to get community note",
			zap.String("noteID", noteID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get community note: %w", err)
	}

	// Convert to storage type
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

// GetVisibleCommunityNotes retrieves visible notes for an object
func (r *CommunityNoteRepository) GetVisibleCommunityNotes(_ context.Context, objectID string) ([]*storage.CommunityNote, error) {
	var modelsSlice []models.CommunityNote

	// Query by object ID using GSI1
	err := r.db.Model(&models.CommunityNote{}).
		Index("gsi1").
		Where("GSI1PK", "=", fmt.Sprintf("OBJECT#%s#NOTES", objectID)).
		Limit(50).
		All(&modelsSlice)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.CommunityNote{}, nil
		}
		r.logger.Error("failed to query community notes",
			zap.String("objectID", objectID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query community notes: %w", err)
	}

	// Filter for visible notes and convert to storage type
	notes := make([]*storage.CommunityNote, 0, len(modelsSlice))
	for _, model := range modelsSlice {
		// Only include visible notes
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

// UpdateCommunityNoteScore updates a note's score and visibility
func (r *CommunityNoteRepository) UpdateCommunityNoteScore(ctx context.Context, noteID string, score float64, status string) error {
	// Get the current note to preserve other fields
	note, err := r.GetCommunityNote(ctx, noteID)
	if err != nil {
		return err
	}

	// Update fields
	note.Score = score
	note.VisibilityStatus = status
	note.UpdatedAt = time.Now()

	// Create updated model
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
		CreatedAt:        note.CreatedAt,
		UpdatedAt:        note.UpdatedAt,
		TTL:              time.Now().Add(90 * 24 * time.Hour).Unix(),
	}

	// Update keys with new values
	model.UpdateKeys()

	// Update in DynamoDB
	err = r.db.Model(model).Update()

	if err != nil {
		r.logger.Error("failed to update community note score",
			zap.String("noteID", noteID),
			zap.Float64("score", score),
			zap.String("status", status),
			zap.Error(err))
		return fmt.Errorf("failed to update community note score: %w", err)
	}

	return nil
}

// CreateCommunityNoteVote creates a vote on a note
func (r *CommunityNoteRepository) CreateCommunityNoteVote(_ context.Context, vote *storage.CommunityNoteVote) error {
	vote.CreatedAt = time.Now()

	// Create model
	model := &models.CommunityNoteVote{
		NoteID:    vote.NoteID,
		VoterID:   vote.VoterID,
		VoteType:  vote.VoteType,
		Helpful:   vote.Helpful,
		Weight:    vote.Weight,
		CreatedAt: vote.CreatedAt,
		TTL:       time.Now().Add(90 * 24 * time.Hour).Unix(),
	}

	// Update keys
	model.UpdateKeys()

	// Create in DynamoDB
	err := r.db.Model(model).Create()
	if err != nil {
		r.logger.Error("failed to create community note vote",
			zap.String("noteID", vote.NoteID),
			zap.String("voterID", vote.VoterID),
			zap.String("voteType", vote.VoteType),
			zap.Error(err))
		return fmt.Errorf("failed to create community note vote: %w", err)
	}

	return nil
}

// GetUserCommunityNoteVotes retrieves a user's votes on specific notes
func (r *CommunityNoteRepository) GetUserCommunityNoteVotes(_ context.Context, userID string, noteIDs []string) (map[string]*storage.CommunityNoteVote, error) {
	votes := make(map[string]*storage.CommunityNoteVote)

	// Batch get votes - DynamORM doesn't have batch get, so we query individually
	for _, noteID := range noteIDs {
		var model models.CommunityNoteVote
		err := r.db.Model(&models.CommunityNoteVote{}).
			Where("PK", "=", fmt.Sprintf("NOTE#%s", noteID)).
			Where("SK", "=", fmt.Sprintf("VOTE#%s", userID)).
			First(&model)

		if err != nil {
			if errors.IsNotFound(err) {
				// Not found is ok - user hasn't voted on this note
				continue
			}
			r.logger.Error("failed to get community note vote",
				zap.String("noteID", noteID),
				zap.String("userID", userID),
				zap.Error(err))
			// Continue to next vote instead of failing entirely
			continue
		}

		// Convert to storage type
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

// GetCommunityNotesByAuthor retrieves community notes authored by a specific actor
func (r *CommunityNoteRepository) GetCommunityNotesByAuthor(_ context.Context, authorID string, limit int, cursor string) ([]*storage.CommunityNote, string, error) {
	var modelsSlice []models.CommunityNote

	// Build query using GSI3
	query := r.db.Model(&models.CommunityNote{}).
		Index("gsi3").
		Where("GSI3PK", "=", fmt.Sprintf("AUTHOR#%s#NOTES", authorID)).
		Limit(limit)

	// Add cursor if provided
	if cursor != "" {
		// Parse cursor - expecting format "timestamp#noteID"
		parts := strings.Split(cursor, "#")
		if len(parts) >= 2 {
			query = query.Where("GSI3SK", "<", cursor)
		}
	}

	// Execute query
	err := query.All(&modelsSlice)
	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.CommunityNote{}, "", nil
		}
		r.logger.Error("failed to query community notes by author",
			zap.String("authorID", authorID),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to query community notes by author: %w", err)
	}

	// Convert to storage type
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

	// Determine next cursor
	var nextCursor string
	if len(modelsSlice) == limit && len(modelsSlice) > 0 {
		lastModel := modelsSlice[len(modelsSlice)-1]
		nextCursor = lastModel.GSI3SK
	}

	return notes, nextCursor, nil
}

// GetCommunityNoteVotes retrieves votes on a specific community note
func (r *CommunityNoteRepository) GetCommunityNoteVotes(_ context.Context, noteID string) ([]*storage.CommunityNoteVote, error) {
	var modelsSlice []models.CommunityNoteVote

	// Query votes for the note
	err := r.db.Model(&models.CommunityNoteVote{}).
		Where("PK", "=", fmt.Sprintf("NOTE#%s", noteID)).
		Where("SK", "BEGINS_WITH", "VOTE#").
		All(&modelsSlice)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.CommunityNoteVote{}, nil
		}
		r.logger.Error("failed to query community note votes",
			zap.String("noteID", noteID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query community note votes: %w", err)
	}

	// Convert to storage type
	votes := make([]*storage.CommunityNoteVote, 0, len(modelsSlice))
	for _, model := range modelsSlice {
		// Set the Helpful flag based on VoteType for easier access
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

// UpdateCommunityNoteAnalysis updates AI analysis results for a note
func (r *CommunityNoteRepository) UpdateCommunityNoteAnalysis(ctx context.Context, noteID string, sentiment, objectivity, sourceQuality float64) error {
	// Get the current note to preserve other fields
	note, err := r.GetCommunityNote(ctx, noteID)
	if err != nil {
		return err
	}

	// Update analysis fields
	note.Sentiment = sentiment
	note.Objectivity = objectivity
	note.SourceQuality = sourceQuality
	note.UpdatedAt = time.Now()

	// Create updated model
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
		TTL:              time.Now().Add(90 * 24 * time.Hour).Unix(),
	}

	// Update keys
	model.UpdateKeys()

	// Update in DynamoDB
	err = r.db.Model(model).Update()

	if err != nil {
		r.logger.Error("failed to update community note analysis",
			zap.String("noteID", noteID),
			zap.Float64("sentiment", sentiment),
			zap.Float64("objectivity", objectivity),
			zap.Float64("sourceQuality", sourceQuality),
			zap.Error(err))
		return fmt.Errorf("failed to update community note analysis: %w", err)
	}

	return nil
}
