package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// PollRepository implements the PollRepository interface using DynamORM
type PollRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewPollRepository creates a new PollRepository
func NewPollRepository(db core.DB, tableName string, logger *zap.Logger) *PollRepository {
	return &PollRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// generatePollID generates a unique poll ID
func (r *PollRepository) generatePollID() string {
	return fmt.Sprintf("%d-%s", time.Now().Unix(), uuid.New().String()[:8])
}

// CreatePoll creates a new poll in DynamoDB
func (r *PollRepository) CreatePoll(ctx context.Context, poll *storage.Poll) error {
	log := r.logger.With(
		zap.String("method", "CreatePoll"),
		zap.String("poll_id", poll.ID),
		zap.String("status_id", poll.StatusID),
	)

	// Validate poll
	if len(poll.Options) < 2 || len(poll.Options) > 4 {
		return fmt.Errorf("poll must have between 2 and 4 options")
	}

	// Generate poll ID if not provided
	if poll.ID == "" {
		poll.ID = r.generatePollID()
	}

	// Set timestamps
	now := time.Now()
	poll.CreatedAt = now
	poll.UpdatedAt = now

	// Initialize vote tracking
	poll.VotesCount = make([]int, len(poll.Options))
	poll.VotersCount = 0
	poll.Votes = make([]int, len(poll.Options))

	// Create DynamORM model
	model := &models.Poll{
		ID:          poll.ID,
		StatusID:    poll.StatusID,
		CreatedBy:   poll.CreatedBy,
		Options:     poll.Options,
		Multiple:    poll.Multiple,
		HideTotals:  poll.HideTotals,
		ExpiresAt:   func() time.Time {
			if poll.ExpiresAt != nil {
				return *poll.ExpiresAt
			}
			return time.Now().Add(24 * time.Hour) // Default to 24 hours
		}(),
		CreatedAt:   poll.CreatedAt,
		UpdatedAt:   poll.UpdatedAt,
		VotesCount:  0, // Initialize to 0
		VotersCount: poll.VotersCount,
		Votes:       make(map[string][]int), // Initialize empty votes map
	}

	// Update keys
	model.UpdateKeys()

	// Create the poll
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		log.Error("failed to create poll",
			zap.Error(err))
		return fmt.Errorf("failed to create poll: %w", err)
	}

	log.Info("poll created successfully",
		zap.Int("options_count", len(poll.Options)))

	return nil
}

// GetPoll retrieves a poll by ID
func (r *PollRepository) GetPoll(ctx context.Context, pollID string) (*storage.Poll, error) {
	log := r.logger.With(
		zap.String("method", "GetPoll"),
		zap.String("poll_id", pollID),
	)

	var model models.Poll
	err := r.db.WithContext(ctx).Model(&models.Poll{}).
		Where("PK", "=", fmt.Sprintf("POLL#%s", pollID)).
		Where("SK", "=", "METADATA").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("poll not found: %s", pollID)
		}
		log.Error("failed to get poll",
			zap.Error(err))
		return nil, fmt.Errorf("failed to get poll: %w", err)
	}

	// Convert to storage poll
	poll := &storage.Poll{
		ID:          model.ID,
		StatusID:    model.StatusID,
		CreatedBy:   model.CreatedBy,
		Options:     model.Options,
		Multiple:    model.Multiple,
		HideTotals:  model.HideTotals,
		ExpiresAt:   &model.ExpiresAt,
		CreatedAt:   model.CreatedAt,
		UpdatedAt:   model.UpdatedAt,
		VotesCount:  make([]int, len(model.Options)), // Convert from votes map
		VotersCount: model.VotersCount,
		Votes:       make([]int, len(model.Options)), // Convert from votes map
	}
	
	// Calculate votes per option from the votes map
	for _, indices := range model.Votes {
		for _, idx := range indices {
			if idx < len(poll.VotesCount) {
				poll.VotesCount[idx]++
				poll.Votes[idx]++
			}
		}
	}

	return poll, nil
}

// GetPollByStatusID retrieves a poll by its associated status ID
func (r *PollRepository) GetPollByStatusID(ctx context.Context, statusID string) (*storage.Poll, error) {
	log := r.logger.With(
		zap.String("method", "GetPollByStatusID"),
		zap.String("status_id", statusID),
	)

	var pollModels []models.Poll
	err := r.db.WithContext(ctx).Model(&models.Poll{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("STATUS#%s", statusID)).
		Where("GSI1SK", "=", "POLL").
		Limit(1).
		All(&pollModels)

	if err != nil {
		log.Error("failed to query poll by status ID",
			zap.Error(err))
		return nil, fmt.Errorf("failed to query poll: %w", err)
	}

	if len(pollModels) == 0 {
		return nil, fmt.Errorf("poll not found for status: %s", statusID)
	}

	model := pollModels[0]

	// Convert to storage poll
	poll := &storage.Poll{
		ID:          model.ID,
		StatusID:    model.StatusID,
		CreatedBy:   model.CreatedBy,
		Options:     model.Options,
		Multiple:    model.Multiple,
		HideTotals:  model.HideTotals,
		ExpiresAt:   &model.ExpiresAt,
		CreatedAt:   model.CreatedAt,
		UpdatedAt:   model.UpdatedAt,
		VotesCount:  make([]int, len(model.Options)), // Convert from votes map
		VotersCount: model.VotersCount,
		Votes:       make([]int, len(model.Options)), // Convert from votes map
	}
	
	// Calculate votes per option from the votes map
	for _, indices := range model.Votes {
		for _, idx := range indices {
			if idx < len(poll.VotesCount) {
				poll.VotesCount[idx]++
				poll.Votes[idx]++
			}
		}
	}

	return poll, nil
}

// VoteOnPoll records a vote on a poll
func (r *PollRepository) VoteOnPoll(ctx context.Context, pollID string, voterID string, choices []int) error {
	log := r.logger.With(
		zap.String("method", "VoteOnPoll"),
		zap.String("poll_id", pollID),
		zap.String("voter_id", voterID),
		zap.Ints("choices", choices),
	)

	// Get the poll first
	poll, err := r.GetPoll(ctx, pollID)
	if err != nil {
		return fmt.Errorf("failed to get poll: %w", err)
	}

	// Check if poll has expired
	if poll.ExpiresAt != nil && time.Now().After(*poll.ExpiresAt) {
		return fmt.Errorf("poll has expired")
	}

	// Validate choices
	for _, choice := range choices {
		if choice < 0 || choice >= len(poll.Options) {
			return fmt.Errorf("invalid choice index: %d", choice)
		}
	}

	// Check multiple choice constraint
	if !poll.Multiple && len(choices) > 1 {
		return fmt.Errorf("poll does not allow multiple choices")
	}

	// Check if user already voted by querying vote records
	existingVote := &models.PollVote{}
	err = r.db.WithContext(ctx).Model(&models.PollVote{}).
		Where("PK", "=", fmt.Sprintf("POLL#%s", pollID)).
		Where("SK", "=", fmt.Sprintf("VOTE#%s", voterID)).
		First(existingVote)
	
	if err == nil {
		// Vote already exists
		return fmt.Errorf("user has already voted on this poll")
	} else if !errors.IsNotFound(err) {
		// Some other error occurred
		return fmt.Errorf("failed to check existing vote: %w", err)
	}

	// Create vote record
	voteModel := &models.PollVote{
		VoterID: voterID,
		Choices: choices,
		VotedAt: time.Now(),
	}
	voteModel.UpdateKeys(pollID)

	// Create vote
	err = r.db.WithContext(ctx).Model(voteModel).Create()
	if err != nil {
		return fmt.Errorf("failed to record vote: %w", err)
	}

	// Update poll vote counts
	for _, choice := range choices {
		if choice < len(poll.Votes) {
			poll.Votes[choice]++
			poll.VotesCount[choice]++
		}
	}
	poll.VotersCount++
	poll.UpdatedAt = time.Now()

	// Update poll record
	if err := r.updatePollCounts(ctx, poll); err != nil {
		log.Error("failed to update poll counts",
			zap.Error(err))
		// Don't fail the vote, counts can be recalculated
	}

	log.Info("vote recorded successfully")

	return nil
}

// GetPollVotes retrieves all votes for a poll
func (r *PollRepository) GetPollVotes(ctx context.Context, pollID string) (map[string][]int, error) {
	log := r.logger.With(
		zap.String("method", "GetPollVotes"),
		zap.String("poll_id", pollID),
	)

	var voteModels []models.PollVote
	err := r.db.WithContext(ctx).Model(&models.PollVote{}).
		Where("PK", "=", fmt.Sprintf("POLL#%s", pollID)).
		Where("SK", "begins_with", "VOTE#").
		All(&voteModels)

	if err != nil {
		log.Error("failed to query votes",
			zap.Error(err))
		return nil, fmt.Errorf("failed to query votes: %w", err)
	}

	votes := make(map[string][]int)
	for _, voteModel := range voteModels {
		votes[voteModel.VoterID] = voteModel.Choices
	}

	return votes, nil
}

// HasUserVoted checks if a user has voted on a poll
func (r *PollRepository) HasUserVoted(ctx context.Context, pollID string, userID string) (bool, []int, error) {
	log := r.logger.With(
		zap.String("method", "HasUserVoted"),
		zap.String("poll_id", pollID),
		zap.String("user_id", userID),
	)

	var voteModel models.PollVote
	err := r.db.WithContext(ctx).Model(&models.PollVote{}).
		Where("PK", "=", fmt.Sprintf("POLL#%s", pollID)).
		Where("SK", "=", fmt.Sprintf("VOTE#%s", userID)).
		First(&voteModel)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		log.Error("failed to check vote",
			zap.Error(err))
		return false, nil, fmt.Errorf("failed to check vote: %w", err)
	}

	return true, voteModel.Choices, nil
}

// updatePollCounts updates the vote counts on a poll
func (r *PollRepository) updatePollCounts(ctx context.Context, poll *storage.Poll) error {
	// Create DynamORM model
	// Calculate total votes from the per-option counts
	totalVotes := 0
	for _, count := range poll.VotesCount {
		totalVotes += count
	}
	
	model := &models.Poll{
		ID:          poll.ID,
		StatusID:    poll.StatusID,
		CreatedBy:   poll.CreatedBy,
		Options:     poll.Options,
		Multiple:    poll.Multiple,
		HideTotals:  poll.HideTotals,
		ExpiresAt:   func() time.Time {
			if poll.ExpiresAt != nil {
				return *poll.ExpiresAt
			}
			return time.Now().Add(24 * time.Hour)
		}(),
		CreatedAt:   poll.CreatedAt,
		UpdatedAt:   poll.UpdatedAt,
		VotesCount:  totalVotes, // models.Poll expects total count, not per-option
		VotersCount: poll.VotersCount,
		Votes:       make(map[string][]int), // We don't maintain voter->choices map
	}

	// Update keys
	model.UpdateKeys()

	// Update the poll
	err := r.db.WithContext(ctx).Model(model).Update()
	if err != nil {
		return fmt.Errorf("failed to update poll: %w", err)
	}

	return nil
}