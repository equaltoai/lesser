package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/scheduled"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// ====================================================================
// MUTATION RESOLVERS - CORE STATUS OPERATIONS
// ====================================================================

// CreateNote is the resolver for the createNote field.
func (r *mutationResolver) CreateNote(ctx context.Context, input model.CreateNoteInput) (*model.CreateNotePayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Validate input
	if err := common.ValidateContentOrAttachments(input.Content, input.AttachmentIds); err != nil {
		return nil, err
	}

	var quoteTargetID string
	if input.QuoteID != nil {
		quoteTargetID = strings.TrimSpace(*input.QuoteID)
	}

	// Build create command
	cmd := &notes.CreateNoteCommand{
		AuthorID:   username,
		Content:    input.Content,
		Visibility: strings.ToLower(input.Visibility.String()),
		Sensitive:  input.Sensitive != nil && *input.Sensitive,
	}

	if input.SpoilerText != nil {
		cmd.SpoilerText = *input.SpoilerText
	}

	if input.InReplyToID != nil {
		cmd.InReplyToID = *input.InReplyToID
	}

	if input.AttachmentIds != nil {
		cmd.MediaIDs = input.AttachmentIds
	}

	if input.Poll != nil {
		pollOptionsAny := make([]any, len(input.Poll.Options))
		for i, option := range input.Poll.Options {
			pollOptionsAny[i] = option
		}

		pollParams := map[string]any{
			"options":    pollOptionsAny,
			"expires_in": float64(input.Poll.ExpiresIn),
		}

		if input.Poll.Multiple != nil {
			pollParams["multiple"] = *input.Poll.Multiple
		}

		if input.Poll.HideTotals != nil {
			pollParams["hide_totals"] = *input.Poll.HideTotals
		}

		if err := common.ValidatePollParams(pollParams); err != nil {
			return nil, err
		}

		cmd.PollOptions = input.Poll.Options
		cmd.PollExpiresIn = input.Poll.ExpiresIn
		cmd.PollMultiple = input.Poll.Multiple != nil && *input.Poll.Multiple
		cmd.PollHideTotals = input.Poll.HideTotals != nil && *input.Poll.HideTotals
	}

	// Handle mentions and tags
	// These would be parsed from content in the service

	var requestLogger *zap.Logger
	if r.Logger != nil {
		requestLogger = r.Logger
		requestLogger.Info("createNote resolver started",
			zap.String("user", username),
			zap.Int("content_length", len(input.Content)))
	}

	// Create note using service
	serviceStart := time.Now()
	result, err := r.Registry.Notes().CreateNote(ctx, cmd)
	if err != nil {
		r.Logger.Error("Failed to create note",
			zap.String("user", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to create note"), err)
	}

	if quoteTargetID != "" && result != nil && result.Note != nil {
		if err := r.attachQuoteToTarget(ctx, username, quoteTargetID, result.Note); err != nil {
			return nil, err
		}
	}

	if requestLogger != nil && result != nil && result.Note != nil {
		requestLogger.Info("createNote service completed",
			zap.String("user", username),
			zap.String("status_id", result.Note.StatusID),
			zap.Duration("duration", time.Since(serviceStart)))
	}

	// Track costs
	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	if err := common.ValidateSliceNotEmpty("media_ids", cmd.MediaIDs); err == nil {
		// Track S3 cost using centralized tracker
		r.trackS3Operation(ctx, "get", len(cmd.MediaIDs))
	}

	// Build response
	now := time.Now()
	convertStart := time.Now()
	objectModel := r.convertStatusToObject(ctx, result.Note)
	if requestLogger != nil && result != nil && result.Note != nil {
		requestLogger.Info("createNote object conversion completed",
			zap.String("status_id", result.Note.StatusID),
			zap.Duration("duration", time.Since(convertStart)),
			zap.Bool("actor_loaded", objectModel != nil && objectModel.Actor != nil))
	}

	payload := &model.CreateNotePayload{
		Object: objectModel,
		Activity: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:        generateID(),
				Type:      activitypub.CreateType,
				Published: &now,
			},
			Actor:  username,
			Object: result.Note,
		},
		Cost: &model.CostUpdate{
			OperationCost:     100,  // 100 microcents = $0.001 for object creation
			DailyTotal:        0.10, // Estimated daily total
			MonthlyProjection: 3.00, // Estimated monthly projection
		},
	}

	if requestLogger != nil && result != nil && result.Note != nil {
		requestLogger.Info("createNote resolver completed",
			zap.String("status_id", result.Note.StatusID),
			zap.Duration("total_duration", time.Since(serviceStart)))
	}

	return payload, nil
}

func (r *mutationResolver) attachQuoteToTarget(ctx context.Context, username, targetStatusID string, quoteStatus *models.Status) error {
	if quoteStatus == nil {
		return errors.New("quote status not available for attachment")
	}

	quotesService := r.Registry.Quotes()
	if quotesService == nil {
		r.Logger.Error("quotes service not available for quote attachment")
		return errors.New("quotes service is not available")
	}

	_, err := quotesService.AttachQuoteToStatus(ctx, quoteStatus, targetStatusID)
	if err != nil {
		r.Logger.Error("failed to attach quote relationship",
			zap.String("user", username),
			zap.String("quote_status_id", quoteStatus.StatusID),
			zap.String("target_status_id", targetStatusID),
			zap.Error(err))
		return err
	}

	r.trackDynamoOperation(ctx, "write", 1)

	if r.Logger != nil {
		r.Logger.Info("attached quote relationship",
			zap.String("user", username),
			zap.String("quote_status_id", quoteStatus.StatusID),
			zap.String("target_status_id", targetStatusID))
	}

	return nil
}

// DeleteObject is the resolver for the deleteObject field.
func (r *mutationResolver) DeleteObject(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	// Delete using notes service
	err = r.Registry.Notes().DeleteNote(ctx, &notes.DeleteNoteCommand{
		StatusID:  id,
		DeleterID: username,
	})
	if err != nil {
		r.Logger.Error("Failed to delete object",
			zap.String("user", username),
			zap.String("object", id),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to delete object"), err)
	}

	// Track costs
	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)

	return true, nil
}

// ScheduleStatus is the resolver for the scheduleStatus field.
func (r *mutationResolver) ScheduleStatus(ctx context.Context, input model.ScheduleStatusInput) (*model.ScheduledStatus, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	scheduledTime := time.Time(input.ScheduledAt)
	if scheduledTime.Before(time.Now().Add(5 * time.Minute)) {
		return nil, ErrScheduledTimeMinimum
	}
	if scheduledTime.After(time.Now().AddDate(1, 0, 0)) {
		return nil, ErrScheduledTimeMaximum
	}

	cmd := &scheduled.CreateScheduledStatusCommand{
		Username:    username,
		Status:      input.Text,
		ScheduledAt: scheduledTime,
	}

	if input.Visibility != nil {
		cmd.Visibility = string(*input.Visibility)
	} else {
		cmd.Visibility = StreamNamePublic
	}

	if input.Sensitive != nil {
		cmd.Sensitive = *input.Sensitive
	}

	if input.SpoilerText != nil {
		cmd.SpoilerText = *input.SpoilerText
	}

	if input.InReplyToID != nil {
		cmd.InReplyToID = *input.InReplyToID
	}

	if input.Language != nil {
		cmd.Language = *input.Language
	}

	if input.MediaIds != nil {
		cmd.MediaIDs = input.MediaIds
	}

	if input.Poll != nil {
		cmd.Poll = map[string]any{
			"options":     input.Poll.Options,
			"expires_in":  input.Poll.ExpiresIn,
			"multiple":    input.Poll.Multiple != nil && *input.Poll.Multiple,
			"hide_totals": input.Poll.HideTotals != nil && *input.Poll.HideTotals,
		}
	}

	result, err := r.Registry.Scheduled().CreateScheduledStatus(ctx, cmd)
	if err != nil {
		r.Logger.Error("Failed to create scheduled status",
			zap.String("user", username),
			zap.Time("scheduledAt", scheduledTime),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to schedule status"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return r.convertScheduledStatusToGraphQL(ctx, result.ScheduledStatus), nil
}

// UpdateScheduledStatus is the resolver for the updateScheduledStatus field.
func (r *mutationResolver) UpdateScheduledStatus(ctx context.Context, id string, input model.UpdateScheduledStatusInput) (*model.ScheduledStatus, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	scheduledTime := time.Time(input.ScheduledAt)
	if scheduledTime.Before(time.Now().Add(5 * time.Minute)) {
		return nil, ErrScheduledTimeMinimum
	}
	if scheduledTime.After(time.Now().AddDate(1, 0, 0)) {
		return nil, ErrScheduledTimeMaximum
	}

	result, err := r.Registry.Scheduled().UpdateScheduledStatus(ctx, &scheduled.UpdateScheduledStatusCommand{
		Username:    username,
		ID:          id,
		ScheduledAt: &scheduledTime,
	})
	if err != nil {
		r.Logger.Error("Failed to update scheduled status",
			zap.String("user", username),
			zap.String("id", id),
			zap.Time("newTime", scheduledTime),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to update scheduled status"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return r.convertScheduledStatusToGraphQL(ctx, result.ScheduledStatus), nil
}

// CancelScheduledStatus is the resolver for the cancelScheduledStatus field.
func (r *mutationResolver) CancelScheduledStatus(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	err = r.Registry.Scheduled().DeleteScheduledStatus(ctx, &scheduled.DeleteScheduledStatusCommand{
		Username: username,
		ID:       id,
	})
	if err != nil {
		r.Logger.Error("Failed to cancel scheduled status",
			zap.String("user", username),
			zap.String("id", id),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to cancel scheduled status"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return true, nil
}

// CreateQuoteNote implements MutationResolver
func (r *mutationResolver) CreateQuoteNote(ctx context.Context, input model.CreateQuoteNoteInput) (*model.CreateNotePayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Set default visibility if not provided
	// Convert GraphQL enum (PUBLIC, UNLISTED, etc.) to lowercase (public, unlisted, etc.)
	visibility := "public"
	if input.Visibility != nil {
		visibility = strings.ToLower(string(*input.Visibility))
	}

	// Create the quote note using the notes service
	// Note: QuoteURL would need to be parsed to extract the note ID for quoting
	cmd := &notes.CreateNoteCommand{
		AuthorID:   username,
		Content:    fmt.Sprintf("%s\n\nQuoting: %s", input.Content, input.QuoteURL),
		Visibility: visibility,
		Sensitive:  input.Sensitive != nil && *input.Sensitive,
		MediaIDs:   input.MediaIds,
	}

	if input.SpoilerText != nil {
		cmd.SpoilerText = *input.SpoilerText
	}

	result, err := r.Registry.Notes().CreateNote(ctx, cmd)
	if err != nil {
		return nil, errors.Join(errors.New("failed to create quote note"), err)
	}

	// Convert result to GraphQL model
	if result.Note == nil {
		return nil, ErrNoteCreationReturnedNoNote
	}

	return &model.CreateNotePayload{
		Object: r.convertStatusToObject(ctx, result.Note),
	}, nil
}
