package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	"github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"go.uber.org/zap"
)

// PollRepository implements the PollRepository interface using enhanced DynamORM patterns
type PollRepository struct {
	*EnhancedBaseRepository[*models.Poll]
	voteRepo *EnhancedBaseRepository[*models.PollVote]
}

// NewPollRepository creates a new PollRepository with enhanced functionality and cost tracking
func NewPollRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *PollRepository {
	// Create enhanced repository optimized for poll operations
	enhancedRepo := NewEnhancedBaseRepository[*models.Poll](db, tableName, logger, costService, "PollRepository", "poll")

	// Set up enhanced services for poll operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Polls cached for vote performance
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for poll notifications

	// Create enhanced vote repository
	voteRepo := NewEnhancedBaseRepository[*models.PollVote](db, tableName, logger, costService, "PollVoteRepository", "poll_vote")
	voteRepo.SetValidationService(NewDefaultValidationService())
	voteRepo.SetPermissionService(NewDefaultPermissionService())
	voteRepo.SetCachingService(NewInMemoryCachingService()) // Cache votes for performance
	voteRepo.SetEventService(NewDefaultEventService())

	return &PollRepository{
		EnhancedBaseRepository: enhancedRepo,
		voteRepo:               voteRepo,
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
		return common.ValidationError{Field: "options", Message: "poll must have between 2 and 4 options"}
	}

	// Generate poll ID if not provided
	if err := common.ValidateRequiredParam("poll_id", poll.ID); err != nil {
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
		ID:         poll.ID,
		StatusID:   poll.StatusID,
		CreatedBy:  poll.CreatedBy,
		Options:    poll.Options,
		Multiple:   poll.Multiple,
		HideTotals: poll.HideTotals,
		ExpiresAt: func() time.Time {
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

	// Use enhanced validation and creation with automatic permission checking and event emission
	err := r.ValidateAndCreate(ctx, model)
	if err != nil {
		log.Error("failed to create poll with enhanced validation",
			zap.Bool("validation_enabled", r.HasValidation()),
			zap.Bool("events_enabled", r.HasEvents()),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "poll", poll.ID)
	}

	log.Info("poll created successfully",
		zap.Int("options_count", len(poll.Options)))

	return nil
}

// GetPoll retrieves a poll by ID
func (r *PollRepository) GetPoll(ctx context.Context, pollID string) (*storage.Poll, error) {

	var model models.Poll
	pk := fmt.Sprintf("POLL#%s", pollID)
	sk := models.SKMetadata

	// Use BaseRepository Get method
	err := r.Get(ctx, pk, sk, &model)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleNotFound(err, "poll", pollID)
		}
		return nil, ErrorHandler.HandleGetError(err, "poll", pollID)
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

	gsiPK := fmt.Sprintf("STATUS#%s", statusID)
	pollModels, err := r.QueryGSI(ctx, "gsi1", gsiPK, 1)

	if err != nil {
		log.Error("failed to query poll by status ID",
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "poll", "status")
	}

	if err := common.ValidateSliceNotEmpty("poll_models", pollModels); err != nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "poll", statusID)
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
		return ErrorHandler.HandleGetError(err, "poll", pollID)
	}

	// Check if poll has expired
	if poll.ExpiresAt != nil && time.Now().After(*poll.ExpiresAt) {
		return common.ValidationError{Field: "expires_at", Message: "poll has expired"}
	}

	// Validate choices
	for _, choice := range choices {
		if choice < 0 || choice >= len(poll.Options) {
			return common.ValidationError{Field: "choices", Message: fmt.Sprintf("invalid choice index: %d", choice)}
		}
	}

	// Check multiple choice constraint
	if !poll.Multiple && len(choices) > 1 {
		return common.ValidationError{Field: "multiple", Message: "poll does not allow multiple choices"}
	}

	// Check if user already voted by querying vote records
	var existingVote models.PollVote
	pk := fmt.Sprintf("POLL#%s", pollID)
	sk := fmt.Sprintf("VOTE#%s", voterID)

	err = r.voteRepo.Get(ctx, pk, sk, &existingVote)
	if err == nil {
		// Vote already exists
		return common.ValidationError{Field: "voter", Message: "user has already voted on this poll"}
	} else if !errors.IsNotFound(err) {
		// Some other error occurred
		return ErrorHandler.HandleGetError(err, "poll vote", voterID)
	}

	// Create vote record
	voteModel := &models.PollVote{
		VoterID: voterID,
		Choices: choices,
		VotedAt: time.Now(),
	}
	voteModel.SetPollID(pollID) // Set keys using new method

	// Create vote using BaseRepository
	err = r.voteRepo.Create(ctx, voteModel)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, "poll vote", voterID)
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

	pk := fmt.Sprintf("POLL#%s", pollID)
	const voteChunkLimit = 200

	var (
		voteModels []*models.PollVote
		cursor     string
	)

	for {
		page, err := r.voteRepo.QueryWithSKPrefixPaginated(ctx, pk, "VOTE#", BasePaginationOptions{
			Limit:  voteChunkLimit,
			Cursor: cursor,
			Order:  SortOrderAsc,
		})
		if err != nil {
			log.Error("failed to query votes",
				zap.Error(err))
			return nil, ErrorHandler.HandleQueryError(err, "poll vote", "voting")
		}

		voteModels = append(voteModels, page.Items...)
		if page.NextCursor == "" || len(page.Items) == 0 {
			break
		}
		cursor = page.NextCursor
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
	pk := fmt.Sprintf("POLL#%s", pollID)
	sk := fmt.Sprintf("VOTE#%s", userID)

	err := r.voteRepo.Get(ctx, pk, sk, &voteModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		log.Error("failed to check vote",
			zap.Error(err))
		return false, nil, ErrorHandler.HandleGetError(err, "poll vote", userID)
	}

	return true, voteModel.Choices, nil
}

// updatePollCounts updates the vote counts on a poll
func (r *PollRepository) updatePollCounts(ctx context.Context, poll *storage.Poll) error {
	// Calculate total votes from the per-option counts
	totalVotes := 0
	for _, count := range poll.VotesCount {
		totalVotes += count
	}

	model := &models.Poll{
		ID:         poll.ID,
		StatusID:   poll.StatusID,
		CreatedBy:  poll.CreatedBy,
		Options:    poll.Options,
		Multiple:   poll.Multiple,
		HideTotals: poll.HideTotals,
		ExpiresAt: func() time.Time {
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

	// Update using BaseRepository (handles key updates and cost tracking)
	err := r.Update(ctx, model)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, "poll", poll.ID)
	}

	return nil
}
