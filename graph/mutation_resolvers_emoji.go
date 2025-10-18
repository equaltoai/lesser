package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/emoji"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// CreateEmoji is the resolver for the createEmoji field.
func (r *mutationResolver) CreateEmoji(ctx context.Context, input model.CreateEmojiInput) (*model.CustomEmoji, error) {
	username, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	cmd := &emoji.CreateEmojiCommand{
		Shortcode: input.Shortcode,
		ImageURL:  input.Image,
	}

	if input.Category != nil {
		cmd.Category = *input.Category
	}

	if input.VisibleInPicker != nil {
		cmd.VisibleInPicker = *input.VisibleInPicker
	} else {
		cmd.VisibleInPicker = true
	}

	result, err := r.Registry.Emoji().CreateEmoji(ctx, cmd)
	if err != nil {
		r.Logger.Error("Failed to create emoji",
			zap.String("admin", username),
			zap.String("shortcode", input.Shortcode),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to create emoji"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	// Track S3 cost using centralized tracker
	r.trackS3Operation(ctx, "put", 1)
	return r.convertEmojiToGraphQL(result.Emoji), nil
}

// UpdateEmoji is the resolver for the updateEmoji field.
func (r *mutationResolver) UpdateEmoji(ctx context.Context, shortcode string, input model.UpdateEmojiInput) (*model.CustomEmoji, error) {
	username, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	cmd := &emoji.UpdateEmojiCommand{
		Shortcode: shortcode,
	}

	if input.Category != nil {
		cmd.Category = input.Category
	}

	if input.VisibleInPicker != nil {
		cmd.VisibleInPicker = input.VisibleInPicker
	}

	result, err := r.Registry.Emoji().UpdateEmoji(ctx, cmd)
	if err != nil {
		r.Logger.Error("Failed to update emoji",
			zap.String("admin", username),
			zap.String("shortcode", shortcode),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to update emoji"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return r.convertEmojiToGraphQL(result.Emoji), nil
}

// DeleteEmoji is the resolver for the deleteEmoji field.
func (r *mutationResolver) DeleteEmoji(ctx context.Context, shortcode string) (bool, error) {
	username, err := r.requireAdmin(ctx)
	if err != nil {
		return false, err
	}

	err = r.Registry.Emoji().DeleteEmoji(ctx, &emoji.DeleteEmojiCommand{
		Shortcode: shortcode,
	})
	if err != nil {
		r.Logger.Error("Failed to delete emoji",
			zap.String("admin", username),
			zap.String("shortcode", shortcode),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to delete emoji"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return true, nil
}
