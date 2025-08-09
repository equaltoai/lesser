package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// EmojiRepository handles custom emoji operations using DynamORM
type EmojiRepository struct {
	db     core.DB
	logger *zap.Logger
}

// NewEmojiRepository creates a new emoji repository
func NewEmojiRepository(db core.DB, logger *zap.Logger) *EmojiRepository {
	return &EmojiRepository{
		db:     db,
		logger: logger,
	}
}

// CreateCustomEmoji creates a new custom emoji
func (r *EmojiRepository) CreateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error {
	now := time.Now()
	emoji.CreatedAt = now
	emoji.UpdatedAt = now
	emoji.ImageUpdatedAt = now

	// Convert storage.CustomEmoji to models.EmojiModel
	model := &models.EmojiModel{
		Shortcode:           emoji.Shortcode,
		URL:                 emoji.URL,
		StaticURL:           emoji.StaticURL,
		VisibleInPicker:     emoji.VisibleInPicker,
		Category:            emoji.Category,
		CreatedAt:           emoji.CreatedAt,
		UpdatedAt:           emoji.UpdatedAt,
		Disabled:            emoji.Disabled,
		Domain:              emoji.Domain,
		ImageRemoteURL:      emoji.ImageRemoteURL,
		ImageStorageVersion: emoji.ImageStorageVersion,
		ImageFileSize:       emoji.ImageFileSize,
		ImageContentType:    emoji.ImageContentType,
		ImageWidth:          emoji.ImageWidth,
		ImageHeight:         emoji.ImageHeight,
		ImageUpdatedAt:      emoji.ImageUpdatedAt,
	}

	// Update the composite keys
	model.UpdateKeys()

	// Check if emoji already exists
	var existing models.EmojiModel
	query := r.db.WithContext(ctx).Model(&models.EmojiModel{})
	err := query.
		Where("PK", "=", fmt.Sprintf("EMOJI#%s", emoji.Shortcode)).
		Where("SK", "=", "EMOJI").
		First(&existing)

	if err == nil {
		// Emoji already exists
		return storage.ErrAlreadyExists
	}

	if !errors.IsNotFound(err) {
		r.logger.Error("failed to check existing emoji", zap.Error(err))
		return err
	}

	// Create the emoji
	err = r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to create custom emoji", zap.Error(err))
		return err
	}

	return nil
}

// GetCustomEmoji retrieves a custom emoji by shortcode
func (r *EmojiRepository) GetCustomEmoji(ctx context.Context, shortcode string) (*storage.CustomEmoji, error) {
	var model models.EmojiModel
	query := r.db.WithContext(ctx).Model(&models.EmojiModel{})
	err := query.
		Where("PK", "=", fmt.Sprintf("EMOJI#%s", shortcode)).
		Where("SK", "=", "EMOJI").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		r.logger.Error("failed to get custom emoji", zap.Error(err))
		return nil, err
	}

	// Convert models.EmojiModel to storage.CustomEmoji
	return &storage.CustomEmoji{
		Shortcode:           model.Shortcode,
		URL:                 model.URL,
		StaticURL:           model.StaticURL,
		VisibleInPicker:     model.VisibleInPicker,
		Category:            model.Category,
		CreatedAt:           model.CreatedAt,
		UpdatedAt:           model.UpdatedAt,
		Disabled:            model.Disabled,
		Domain:              model.Domain,
		ImageRemoteURL:      model.ImageRemoteURL,
		ImageStorageVersion: model.ImageStorageVersion,
		ImageFileSize:       model.ImageFileSize,
		ImageContentType:    model.ImageContentType,
		ImageWidth:          model.ImageWidth,
		ImageHeight:         model.ImageHeight,
		ImageUpdatedAt:      model.ImageUpdatedAt,
	}, nil
}

// GetCustomEmojis retrieves all custom emojis (not disabled)
func (r *EmojiRepository) GetCustomEmojis(ctx context.Context) ([]*storage.CustomEmoji, error) {
	var emojiModels []*models.EmojiModel

	// Query using GSI1 for all emojis
	err := r.db.WithContext(ctx).Model(&models.EmojiModel{}).
		Index("gsi1").
		Where("GSI1PK", "=", "ALL_EMOJIS").
		All(&emojiModels)

	if err != nil {
		r.logger.Error("failed to get custom emojis", zap.Error(err))
		return nil, err
	}

	// Filter out disabled emojis and convert to storage type
	emojis := make([]*storage.CustomEmoji, 0, len(emojiModels))
	for _, model := range emojiModels {
		// Skip disabled emojis unless they're remote emojis
		if model.Disabled && model.Domain == "" {
			continue
		}

		emojis = append(emojis, &storage.CustomEmoji{
			Shortcode:           model.Shortcode,
			URL:                 model.URL,
			StaticURL:           model.StaticURL,
			VisibleInPicker:     model.VisibleInPicker,
			Category:            model.Category,
			CreatedAt:           model.CreatedAt,
			UpdatedAt:           model.UpdatedAt,
			Disabled:            model.Disabled,
			Domain:              model.Domain,
			ImageRemoteURL:      model.ImageRemoteURL,
			ImageStorageVersion: model.ImageStorageVersion,
			ImageFileSize:       model.ImageFileSize,
			ImageContentType:    model.ImageContentType,
			ImageWidth:          model.ImageWidth,
			ImageHeight:         model.ImageHeight,
			ImageUpdatedAt:      model.ImageUpdatedAt,
		})
	}

	return emojis, nil
}

// UpdateCustomEmoji updates an existing custom emoji
func (r *EmojiRepository) UpdateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error {
	emoji.UpdatedAt = time.Now()

	// Convert to model
	model := &models.EmojiModel{
		Shortcode:           emoji.Shortcode,
		URL:                 emoji.URL,
		StaticURL:           emoji.StaticURL,
		VisibleInPicker:     emoji.VisibleInPicker,
		Category:            emoji.Category,
		CreatedAt:           emoji.CreatedAt,
		UpdatedAt:           emoji.UpdatedAt,
		Disabled:            emoji.Disabled,
		Domain:              emoji.Domain,
		ImageRemoteURL:      emoji.ImageRemoteURL,
		ImageStorageVersion: emoji.ImageStorageVersion,
		ImageFileSize:       emoji.ImageFileSize,
		ImageContentType:    emoji.ImageContentType,
		ImageWidth:          emoji.ImageWidth,
		ImageHeight:         emoji.ImageHeight,
		ImageUpdatedAt:      emoji.ImageUpdatedAt,
	}

	// Update the composite keys
	model.UpdateKeys()

	// Check if emoji exists first
	var existing models.EmojiModel
	query := r.db.WithContext(ctx).Model(&models.EmojiModel{})
	err := query.
		Where("PK", "=", fmt.Sprintf("EMOJI#%s", emoji.Shortcode)).
		Where("SK", "=", "EMOJI").
		First(&existing)

	if err != nil {
		if errors.IsNotFound(err) {
			return storage.ErrNotFound
		}
		r.logger.Error("failed to check existing emoji", zap.Error(err))
		return err
	}

	// Update the emoji (DynamORM will handle the put operation)
	err = r.db.WithContext(ctx).Model(model).Create()

	if err != nil {
		r.logger.Error("failed to update custom emoji", zap.Error(err))
		return err
	}

	return nil
}

// DeleteCustomEmoji deletes a custom emoji
func (r *EmojiRepository) DeleteCustomEmoji(ctx context.Context, shortcode string) error {
	// Check if emoji exists first
	var existing models.EmojiModel
	query := r.db.WithContext(ctx).Model(&models.EmojiModel{})
	err := query.
		Where("PK", "=", fmt.Sprintf("EMOJI#%s", shortcode)).
		Where("SK", "=", "EMOJI").
		First(&existing)

	if err != nil {
		if errors.IsNotFound(err) {
			return storage.ErrNotFound
		}
		r.logger.Error("failed to check existing emoji", zap.Error(err))
		return err
	}

	// Delete the emoji
	err = r.db.WithContext(ctx).Model(&models.EmojiModel{}).
		Where("PK", "=", fmt.Sprintf("EMOJI#%s", shortcode)).
		Where("SK", "=", "EMOJI").
		Delete()

	if err != nil {
		r.logger.Error("failed to delete custom emoji", zap.Error(err))
		return err
	}

	return nil
}

// GetCustomEmojisByCategory retrieves custom emojis by category
func (r *EmojiRepository) GetCustomEmojisByCategory(ctx context.Context, category string) ([]*storage.CustomEmoji, error) {
	var emojiModels []*models.EmojiModel

	// Query using GSI2 for category
	err := r.db.WithContext(ctx).Model(&models.EmojiModel{}).
		Index("gsi2").
		Where("GSI2PK", "=", fmt.Sprintf("CATEGORY#%s", category)).
		All(&emojiModels)

	if err != nil {
		r.logger.Error("failed to get custom emojis by category", zap.Error(err))
		return nil, err
	}

	// Filter out disabled emojis and convert to storage type
	emojis := make([]*storage.CustomEmoji, 0, len(emojiModels))
	for _, model := range emojiModels {
		// Skip disabled emojis unless they're remote emojis
		if model.Disabled && model.Domain == "" {
			continue
		}

		emojis = append(emojis, &storage.CustomEmoji{
			Shortcode:           model.Shortcode,
			URL:                 model.URL,
			StaticURL:           model.StaticURL,
			VisibleInPicker:     model.VisibleInPicker,
			Category:            model.Category,
			CreatedAt:           model.CreatedAt,
			UpdatedAt:           model.UpdatedAt,
			Disabled:            model.Disabled,
			Domain:              model.Domain,
			ImageRemoteURL:      model.ImageRemoteURL,
			ImageStorageVersion: model.ImageStorageVersion,
			ImageFileSize:       model.ImageFileSize,
			ImageContentType:    model.ImageContentType,
			ImageWidth:          model.ImageWidth,
			ImageHeight:         model.ImageHeight,
			ImageUpdatedAt:      model.ImageUpdatedAt,
		})
	}

	return emojis, nil
}
