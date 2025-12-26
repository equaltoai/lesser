package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"go.uber.org/zap"
)

// UpdateStatus is the resolver for the updateStatus field.
func (r *mutationResolver) UpdateStatus(ctx context.Context, id string, input model.UpdateStatusInput) (*model.Object, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	statusID := strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", statusID); err != nil {
		return nil, err
	}

	service := r.Registry.Notes()
	if service == nil {
		return nil, errors.New("notes service is not available")
	}

	current, err := service.GetNote(ctx, statusID)
	if err != nil || current == nil {
		r.Logger.Error("Failed to load status for update",
			zap.String("status_id", statusID),
			zap.String("user", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to load status for update"), err)
	}

	sensitive := current.Sensitive
	if input.Sensitive != nil {
		sensitive = *input.Sensitive
	}

	spoilerText := ""
	if current.Note != nil && current.Note.Get() != nil {
		spoilerText = current.Note.Get().Summary
	}
	if input.SpoilerText != nil {
		spoilerText = *input.SpoilerText
	}

	language := current.Language
	if input.Language != nil {
		language = *input.Language
	}

	cmd := &notes.UpdateNoteCommand{
		StatusID:    statusID,
		Content:     input.Content,
		Sensitive:   sensitive,
		SpoilerText: spoilerText,
		Language:    language,
		MediaIDs:    input.AttachmentIds,
		UpdaterID:   username,
	}

	result, err := service.UpdateNote(ctx, cmd)
	if err != nil {
		r.Logger.Error("Failed to update status",
			zap.String("status_id", statusID),
			zap.String("user", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to update status"), err)
	}

	r.trackDynamoOperation(ctx, "write", 1)

	return r.convertStatusToObject(ctx, result.Note), nil
}

// MuteStatus is the resolver for the muteStatus field.
func (r *mutationResolver) MuteStatus(ctx context.Context, id string, durationSeconds *int) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	statusID := strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", statusID); err != nil {
		return false, err
	}

	service := r.Registry.Notes()
	if service == nil {
		return false, errors.New("notes service is not available")
	}

	cmd := &notes.MuteNoteCommand{
		StatusID: statusID,
		MuterID:  username,
	}
	if durationSeconds != nil && *durationSeconds > 0 {
		cmd.DurationSeconds = *durationSeconds
	}

	if _, err := service.MuteNote(ctx, cmd); err != nil {
		r.Logger.Error("Failed to mute status thread",
			zap.String("status_id", statusID),
			zap.String("user", username),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to mute status thread"), err)
	}

	r.trackDynamoOperation(ctx, "write", 1)

	return true, nil
}

// UnmuteStatus is the resolver for the unmuteStatus field.
func (r *mutationResolver) UnmuteStatus(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	statusID := strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", statusID); err != nil {
		return false, err
	}

	service := r.Registry.Notes()
	if service == nil {
		return false, errors.New("notes service is not available")
	}

	if _, err := service.UnmuteNote(ctx, &notes.UnmuteNoteCommand{
		StatusID: statusID,
		MuterID:  username,
	}); err != nil {
		r.Logger.Error("Failed to unmute status thread",
			zap.String("status_id", statusID),
			zap.String("user", username),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to unmute status thread"), err)
	}

	r.trackDynamoOperation(ctx, "write", 1)

	return true, nil
}
