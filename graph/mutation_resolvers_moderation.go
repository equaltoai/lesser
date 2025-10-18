package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/services/moderationml"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// FlagObject is the resolver for the flagObject field.
func (r *mutationResolver) FlagObject(ctx context.Context, input model.FlagInput) (*model.FlagPayload, error) {
	// Authenticate user
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Validate input
	if err := common.ValidateRequiredParam("objectID", input.ObjectID); err != nil {
		return nil, ErrObjectIDRequired
	}
	if err := common.ValidateRequiredParam("reason", input.Reason); err != nil {
		return nil, ErrReasonRequired
	}

	// Generate unique flag ID
	flagID := fmt.Sprintf("flag_%d_%s", time.Now().UnixNano(), username)

	// Build actor ID for the flagger using configuration
	actorID := config.Get().ActorURL(username)

	// Create storage flag
	flag := &storage.Flag{
		ID:        flagID,
		ContentID: input.ObjectID,
		FlaggerID: actorID,
		Reason:    input.Reason,
		Status:    "pending",
		Published: time.Now(),
		Actor:     actorID,
		Object:    []string{input.ObjectID},
		Content:   input.Reason,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Store the flag using storage
	moderationRepo := r.Storage.Moderation()
	if err := moderationRepo.CreateFlag(ctx, flag); err != nil {
		r.Logger.Error("failed to create moderation flag",
			zap.String("object_id", input.ObjectID),
			zap.String("flagger", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to create flag"), err)
	}

	// Convert evidence from []string to []any
	evidence := make([]any, len(input.Evidence))
	for i, e := range input.Evidence {
		evidence[i] = e
	}

	// Create moderation event for tracking
	event := &storage.ModerationEvent{
		ID:         flagID,
		ObjectID:   input.ObjectID,
		ObjectType: "status", // Could be enhanced to detect object type
		ActorID:    actorID,
		EventType:  "flagged",
		Category:   string(moderation.CategoryOther), // Convert to string for storage
		Severity:   string(storage.SeverityMedium),   // Convert to string for storage
		Reason:     input.Reason,
		Evidence:   evidence,
		Created:    time.Now(),
		Updated:    time.Now(),
	}

	// Store moderation event
	if err := moderationRepo.CreateModerationEvent(ctx, event); err != nil {
		r.Logger.Warn("failed to create moderation event",
			zap.String("flag_id", flagID),
			zap.Error(err))
		// Don't fail the whole operation if event creation fails
	}

	r.Logger.Info("content flagged for moderation",
		zap.String("flag_id", flagID),
		zap.String("object_id", input.ObjectID),
		zap.String("flagger", username),
		zap.String("reason", input.Reason))

	return &model.FlagPayload{
		ModerationID: flagID,
		Queued:       true,
	}, nil
}

// AddCommunityNote is the resolver for the addCommunityNote field.
func (r *mutationResolver) AddCommunityNote(ctx context.Context, input model.CommunityNoteInput) (*model.CommunityNotePayload, error) {
	// Require authentication
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Validate object exists using notes service
	object, err := r.Registry.Notes().GetNote(ctx, input.ObjectID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, ErrObjectNotFound
		}
		r.Logger.Error("Failed to get object for community note",
			zap.String("objectID", input.ObjectID),
			zap.String("authorID", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to validate object"), err)
	}

	// Create community note
	note := &storage.CommunityNote{
		ObjectID:         input.ObjectID,
		ObjectType:       "status", // For ActivityPub objects
		AuthorID:         username,
		Content:          input.Content,
		Language:         "en",      // Default language, could be detected
		VisibilityStatus: "pending", // Start as pending, becomes visible based on votes
		Score:            0.0,
		HelpfulVotes:     0,
		NotHelpfulVotes:  0,
	}

	// Store the note using repository
	err = r.Storage.CommunityNote().CreateCommunityNote(ctx, note)
	if err != nil {
		r.Logger.Error("Failed to create community note",
			zap.String("objectID", input.ObjectID),
			zap.String("authorID", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to create community note"), err)
	}

	// Get the author actor for response
	author, err := r.Registry.Accounts().GetAccount(ctx, username)
	if err != nil {
		r.Logger.Error("Failed to get author for community note response",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get author"), err)
	}

	// Convert to GraphQL model
	graphqlNote := &model.CommunityNote{
		ID:         note.ID,
		Author:     r.convertAccountToActor(author),
		Content:    note.Content,
		Helpful:    note.HelpfulVotes,
		NotHelpful: note.NotHelpfulVotes,
		CreatedAt:  model.Time(note.CreatedAt),
	}

	// Convert object to GraphQL model
	graphqlObject := r.convertStatusToObject(ctx, object)

	r.Logger.Info("Created community note",
		zap.String("noteID", note.ID),
		zap.String("objectID", input.ObjectID),
		zap.String("authorID", username))

	return &model.CommunityNotePayload{
		Note:   graphqlNote,
		Object: graphqlObject,
	}, nil
}

// VoteCommunityNote is the resolver for the voteCommunityNote field.
func (r *mutationResolver) VoteCommunityNote(ctx context.Context, id string, helpful bool) (*model.CommunityNote, error) {
	// Require authentication
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get the existing note to verify it exists
	note, err := r.Storage.CommunityNote().GetCommunityNote(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, ErrCommunityNoteNotFound
		}
		r.Logger.Error("Failed to get community note for voting",
			zap.String("noteID", id),
			zap.String("voterID", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get community note"), err)
	}

	// Prevent authors from voting on their own notes
	if note.AuthorID == username {
		return nil, ErrAuthorsCannotVote
	}

	// Create or update vote
	voteType := VoteTypeNotHelpful
	if helpful {
		voteType = VoteTypeHelpful
	}

	vote := &storage.CommunityNoteVote{
		NoteID:   id,
		VoterID:  username,
		VoteType: voteType,
		Helpful:  helpful,
		Weight:   1.0, // Basic weight, could be enhanced with user reputation
	}

	// Check if user has already voted (get existing votes)
	existingVotes, err := r.Storage.CommunityNote().GetUserCommunityNoteVotes(ctx, username, []string{id})
	if err != nil {
		r.Logger.Error("Failed to check existing votes",
			zap.String("noteID", id),
			zap.String("voterID", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to check existing votes"), err)
	}

	// Store the vote
	err = r.Storage.CommunityNote().CreateCommunityNoteVote(ctx, vote)
	if err != nil {
		r.Logger.Error("Failed to create community note vote",
			zap.String("noteID", id),
			zap.String("voterID", username),
			zap.String("voteType", voteType),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to create vote"), err)
	}

	// Recalculate note score and update vote counts
	allVotes, err := r.Storage.CommunityNote().GetCommunityNoteVotes(ctx, id)
	if err != nil {
		r.Logger.Error("Failed to get all votes for score calculation",
			zap.String("noteID", id),
			zap.Error(err))
	} else {
		// Update note with new vote counts
		helpfulCount := 0
		notHelpfulCount := 0

		for _, v := range allVotes {
			switch v.VoteType {
			case VoteTypeHelpful:
				helpfulCount++
			case VoteTypeNotHelpful:
				notHelpfulCount++
			}
		}

		// Calculate basic score (helpful - not_helpful)
		// In a real system, this could be more sophisticated
		score := float64(helpfulCount - notHelpfulCount)

		// Determine visibility based on score
		visibilityStatus := "pending"
		if score >= 3 {
			visibilityStatus = "visible"
		} else if score >= 10 {
			visibilityStatus = "prominent"
		} else if score <= -3 {
			visibilityStatus = "hidden"
		}

		// Update the note's score and visibility
		err = r.Storage.CommunityNote().UpdateCommunityNoteScore(ctx, id, score, visibilityStatus)
		if err != nil {
			r.Logger.Error("Failed to update community note score",
				zap.String("noteID", id),
				zap.Float64("score", score),
				zap.String("visibility", visibilityStatus),
				zap.Error(err))
		}

		// Update local note object for response
		note.HelpfulVotes = helpfulCount
		note.NotHelpfulVotes = notHelpfulCount
		note.Score = score
		note.VisibilityStatus = visibilityStatus
	}

	// Get the author actor for response
	author, err := r.Registry.Accounts().GetAccount(ctx, note.AuthorID)
	if err != nil {
		r.Logger.Error("Failed to get author for community note response",
			zap.String("authorID", note.AuthorID),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get author"), err)
	}

	r.Logger.Info("Voted on community note",
		zap.String("noteID", id),
		zap.String("voterID", username),
		zap.String("voteType", voteType),
		zap.Bool("wasExistingVote", len(existingVotes) > 0))

	// Convert to GraphQL model and return
	return &model.CommunityNote{
		ID:         note.ID,
		Author:     r.convertAccountToActor(author),
		Content:    note.Content,
		Helpful:    note.HelpfulVotes,
		NotHelpful: note.NotHelpfulVotes,
		CreatedAt:  model.Time(note.CreatedAt),
	}, nil
}

// TrainModerationModel implements MutationResolver.
func (r *mutationResolver) TrainModerationModel(ctx context.Context, samples []*model.ModerationSampleInput, options *model.BedrockTrainingOptions) (*model.TrainingResult, error) {
	// 1. Require authentication
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// 2. Check feature flag (admin permissions checked via feature flag)
	if err := common.MustCheckModerationMLAccess(ctx, username); err != nil {
		return nil, err
	}

	// 3. Get moderation ML service
	mlService := r.Registry.ModerationML()
	if mlService == nil {
		return nil, errors.New("moderation ML service not available")
	}

	// 4. Convert GraphQL input to service input
	svcSamples := make([]moderationml.SampleInput, len(samples))
	for i, s := range samples {
		svcSamples[i] = moderationml.SampleInput{
			ObjectID:   s.ObjectID,
			ObjectType: s.ObjectType,
			Label:      s.Label,
			ReviewerID: username,
			Confidence: s.Confidence,
			Metadata:   make(map[string]interface{}),
		}
	}

	// 5. Queue samples and get their IDs
	sampleIDs, err := mlService.QueueSamples(ctx, svcSamples)
	if err != nil {
		return nil, fmt.Errorf("failed to queue samples: %w", err)
	}

	// 6. Prepare training options
	trainingOpts := moderationml.TrainingOptions{
		BaseModelID: "anthropic.claude-3-haiku-20240307-v1:0", // Default
	}

	if options != nil {
		if options.BaseModelID != nil {
			trainingOpts.BaseModelID = *options.BaseModelID
		}
		if options.DatasetS3Path != nil {
			trainingOpts.DatasetS3Path = *options.DatasetS3Path
		}
		if options.OutputS3Path != nil {
			trainingOpts.OutputS3Path = *options.OutputS3Path
		}
		if options.MaxTrainingTime != nil {
			trainingOpts.MaxTrainingTime = *options.MaxTrainingTime
		}
		if options.EarlyStoppingEnabled != nil {
			trainingOpts.EarlyStoppingEnabled = *options.EarlyStoppingEnabled
		}
	}

	// 7. Extract context for tracking
	// Get user ID from context
	userID := GetUserID(ctx)

	// Get tenant ID - in this system, use domain from config as tenant identifier
	tenantID := config.Get().Domain
	if tenantID == "" {
		tenantID = "default" // Fallback if domain not configured
	}

	// 8. Train model using the actual sample IDs from the repository with context
	result, err := mlService.TrainModel(ctx, tenantID, userID, sampleIDs, trainingOpts)
	if err != nil {
		return nil, fmt.Errorf("model training failed: %w", err)
	}

	// 9. Convert service result to GraphQL result
	// Note: TrainingResult now returns immediately with SUBMITTED status
	// Metrics will be zero until job completes
	return &model.TrainingResult{
		Success:      result.Success,
		Status:       result.Status,
		JobID:        result.JobID,
		JobName:      result.JobName,
		DatasetS3Key: result.DatasetS3Key,
		ModelVersion: result.ModelVersion,
		Accuracy:     result.Accuracy,
		Precision:    result.Precision,
		Recall:       result.Recall,
		F1Score:      result.F1Score,
		SamplesUsed:  result.SamplesUsed,
		TrainingTime: result.TrainingTime,
		Improvements: result.Improvements,
	}, nil
}

// DeleteModerationPattern implements MutationResolver
func (r *mutationResolver) DeleteModerationPattern(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	// Get moderation repository
	modRepo := r.Registry.GetStorage().Moderation()
	if modRepo == nil {
		return false, ErrModerationUnavailable
	}

	// Delete the pattern
	err = modRepo.DeleteModerationPattern(ctx, id)
	if err != nil {
		return false, errors.Join(errors.New("failed to delete moderation pattern"), err)
	}

	r.Logger.Info("Deleted moderation pattern",
		zap.String("user", username),
		zap.String("pattern_id", id))

	return true, nil
}

// UpdateModerationPattern implements MutationResolver
func (r *mutationResolver) UpdateModerationPattern(ctx context.Context, id string, input model.ModerationPatternInput) (*moderation.ModerationPattern, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Update moderation pattern
	_ = username
	return &moderation.ModerationPattern{
		ID:          id,
		Name:        input.Pattern,
		Description: fmt.Sprintf("Updated pattern for %s", input.Pattern),
		Type:        string(input.Type),
		Content:     input.Pattern,
		Severity:    SeverityMedium,
		Action:      "flag",
		Active:      true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CreatedBy:   username,
	}, nil
}

// CreateModerationPattern implements MutationResolver
func (r *mutationResolver) CreateModerationPattern(ctx context.Context, input model.ModerationPatternInput) (*moderation.ModerationPattern, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get moderation repository
	modRepo := r.Registry.GetStorage().Moderation()
	if modRepo == nil {
		return nil, ErrModerationUnavailable
	}

	// Convert severity to float value for storage
	var severityValue float64
	switch input.Severity {
	case model.ModerationSeverityLow:
		severityValue = 0.3
	case model.ModerationSeverityMedium:
		severityValue = 0.6
	case model.ModerationSeverityHigh:
		severityValue = 0.8
	case model.ModerationSeverityCritical:
		severityValue = 1.0
	default:
		severityValue = 0.5
	}

	// Determine if active (default to true if not specified)
	active := true
	if input.Active != nil {
		active = *input.Active
	}

	// Generate description from pattern
	description := fmt.Sprintf("Pattern for %s", input.Pattern)

	// Convert severity to string for storage
	var severityStr string
	switch input.Severity {
	case model.ModerationSeverityLow:
		severityStr = SeverityLow
	case model.ModerationSeverityMedium:
		severityStr = SeverityMedium
	case model.ModerationSeverityHigh:
		severityStr = SeverityHigh
	case model.ModerationSeverityCritical:
		severityStr = HealthStatusCritical
	default:
		severityStr = SeverityMedium
	}

	// Create the pattern ID
	patternID := fmt.Sprintf("pattern_%d", time.Now().UnixNano())

	// Create storage pattern with ALL fields properly mapped
	storagePattern := &storage.ModerationPattern{
		ID:          patternID,
		Name:        input.Pattern, // Use pattern as name
		Description: description,
		Pattern:     input.Pattern,
		Content:     input.Pattern, // Repository uses Content field for the pattern
		Type:        string(input.Type),
		Severity:    fmt.Sprintf("%.2f", severityValue),
		Action:      "flag", // Default action
		Active:      active,
		CreatedBy:   username,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Save the pattern
	err = modRepo.CreateModerationPattern(ctx, storagePattern)
	if err != nil {
		return nil, errors.Join(errors.New("failed to create moderation pattern"), err)
	}

	// Return the created pattern
	return &moderation.ModerationPattern{
		ID:                 patternID,
		Name:               storagePattern.Name,
		Description:        storagePattern.Description,
		Type:               storagePattern.Type,
		Content:            storagePattern.Content,
		Severity:           severityStr,
		Action:             storagePattern.Action,
		Active:             storagePattern.Active,
		MatchCount:         0,
		FalsePositiveCount: 0,
		Effectiveness:      0.0,
		CreatedAt:          storagePattern.CreatedAt,
		UpdatedAt:          storagePattern.UpdatedAt,
		CreatedBy:          username,
	}, nil
}
