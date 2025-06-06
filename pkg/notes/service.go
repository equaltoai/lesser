package notes

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"go.uber.org/zap"
)

// Service provides methods for managing community notes
type Service struct {
	dynamoClient *dynamodb.Client
	logger       *zap.Logger
}

// NewService creates a new notes service
func NewService(dynamoClient *dynamodb.Client, logger *zap.Logger) *Service {
	// Set the package-level client
	SetDynamoClient(dynamoClient)

	return &Service{
		dynamoClient: dynamoClient,
		logger:       logger,
	}
}

// CreateNote creates a new community note
func (s *Service) CreateNote(ctx context.Context, note *CommunityNote) error {
	note.ID = GenerateNoteID()
	note.CreatedAt = time.Now()
	note.UpdatedAt = time.Now()
	note.VisibilityStatus = VisibilityPending
	note.Score = 0

	return StoreNote(ctx, note)
}

// GetNote retrieves a note by ID
func (s *Service) GetNote(ctx context.Context, noteID string) (*CommunityNote, error) {
	return GetNote(ctx, noteID)
}

// GetVisibleNotes retrieves visible notes for an object
func (s *Service) GetVisibleNotes(ctx context.Context, objectID string) ([]CommunityNote, error) {
	return GetVisibleNotes(ctx, objectID)
}

// VoteOnNote records a vote on a note
func (s *Service) VoteOnNote(ctx context.Context, vote *Vote) error {
	return StoreVote(ctx, vote)
}

// GetVotesForNote retrieves all votes for a note
func (s *Service) GetVotesForNote(ctx context.Context, noteID string) ([]Vote, error) {
	return GetVotesForNote(ctx, noteID)
}

// GetUserVotes retrieves a user's votes on specific notes
func (s *Service) GetUserVotes(ctx context.Context, userID string, noteIDs []string) (map[string]Vote, error) {
	return GetUserVotes(ctx, userID, noteIDs)
}

// GetNotesByAuthor retrieves notes created by a specific author
func (s *Service) GetNotesByAuthor(ctx context.Context, authorID string, limit int32) ([]CommunityNote, error) {
	return GetNotesByAuthor(ctx, authorID, limit)
}

// RecalculateNoteScore recalculates a note's score based on current votes
func (s *Service) RecalculateNoteScore(ctx context.Context, noteID string) error {
	// Get the note
	note, err := GetNote(ctx, noteID)
	if err != nil {
		return err
	}

	// Get all votes
	votes, err := GetVotesForNote(ctx, noteID)
	if err != nil {
		return err
	}

	// Calculate new score
	newScore := CalculateNoteScore(note, votes)
	newStatus := DetermineVisibilityStatus(newScore)

	// Update the note
	return UpdateNoteScore(ctx, noteID, newScore, newStatus)
}

// CheckRateLimit checks if a user can create more notes
func (s *Service) CheckRateLimit(ctx context.Context, userID string, reputation float64) (bool, int) {
	limit := CalculateNoteLimit(reputation)
	return CheckNoteRateLimit(ctx, userID, limit)
}
