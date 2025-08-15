package repositories

import (
	"context"
	"fmt"
	"strings"
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
		UsageCount:          emoji.UsageCount,
		LastUsedAt:          emoji.LastUsedAt,
		PopularityScore:     emoji.PopularityScore,
		SearchKeywords:      emoji.SearchKeywords,
		AltText:             emoji.AltText,
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
	return r.convertModelToStorage(&model), nil
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

		emojis = append(emojis, r.convertModelToStorage(model))
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
		UsageCount:          emoji.UsageCount,
		LastUsedAt:          emoji.LastUsedAt,
		PopularityScore:     emoji.PopularityScore,
		SearchKeywords:      emoji.SearchKeywords,
		AltText:             emoji.AltText,
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

// GetRemoteEmoji retrieves a remote emoji by shortcode and domain
func (r *EmojiRepository) GetRemoteEmoji(ctx context.Context, shortcode, domain string) (*storage.CustomEmoji, error) {
	var model models.EmojiModel
	query := r.db.WithContext(ctx).Model(&models.EmojiModel{})

	// Remote emojis use a different key pattern
	err := query.
		Where("PK", "=", fmt.Sprintf("EMOJI#%s@%s", shortcode, domain)).
		Where("SK", "=", "EMOJI").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		r.logger.Error("failed to get remote emoji",
			zap.String("shortcode", shortcode),
			zap.String("domain", domain),
			zap.Error(err))
		return nil, err
	}

	// Convert to storage model
	return r.convertModelToStorage(&model), nil
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

		emojis = append(emojis, r.convertModelToStorage(model))
	}

	return emojis, nil
}

// SearchEmojis performs sophisticated emoji searches with relevance scoring
func (r *EmojiRepository) SearchEmojis(ctx context.Context, query string, limit int) ([]*storage.CustomEmoji, error) {
	if query == "" || limit <= 0 {
		return []*storage.CustomEmoji{}, nil
	}

	// Normalize query for search
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	
	// Try different search strategies
	var allModels []*models.EmojiModel
	
	// Strategy 1: Prefix search using GSI3
	if len(normalizedQuery) >= 3 {
		prefix := normalizedQuery[:3]
		var prefixModels []*models.EmojiModel
		err := r.db.WithContext(ctx).Model(&models.EmojiModel{}).
			Index("gsi3").
			Where("GSI3PK", "=", fmt.Sprintf("SEARCH#%s", prefix)).
			All(&prefixModels)
		
		if err == nil {
			allModels = append(allModels, prefixModels...)
		}
	}
	
	// Strategy 2: Get all emojis and perform in-memory search for broader matching
	var allEmojis []*models.EmojiModel
	err := r.db.WithContext(ctx).Model(&models.EmojiModel{}).
		Index("gsi1").
		Where("GSI1PK", "=", "ALL_EMOJIS").
		All(&allEmojis)
	
	if err != nil {
		r.logger.Error("failed to search emojis", zap.Error(err))
		return nil, err
	}
	
	// Combine and deduplicate results
	seen := make(map[string]bool)
	uniqueModels := make([]*models.EmojiModel, 0)
	
	// Add prefix matches first (higher priority)
	for _, model := range allModels {
		key := model.PK + "#" + model.SK
		if !seen[key] {
			seen[key] = true
			uniqueModels = append(uniqueModels, model)
		}
	}
	
	// Add broader matches
	for _, model := range allEmojis {
		key := model.PK + "#" + model.SK
		if !seen[key] && r.matchesSearchQuery(model, normalizedQuery) {
			seen[key] = true
			uniqueModels = append(uniqueModels, model)
		}
	}
	
	// Score and sort results
	scored := r.scoreSearchResults(uniqueModels, normalizedQuery)
	
	// Limit results
	if len(scored) > limit {
		scored = scored[:limit]
	}
	
	// Convert to storage type
	results := make([]*storage.CustomEmoji, len(scored))
	for i, model := range scored {
		results[i] = r.convertModelToStorage(model)
	}
	
	return results, nil
}

// GetPopularEmojis retrieves emojis by popularity score, optionally filtered by domain
func (r *EmojiRepository) GetPopularEmojis(ctx context.Context, domain string, limit int) ([]*storage.CustomEmoji, error) {
	if limit <= 0 {
		limit = 20
	}
	
	var emojiModels []*models.EmojiModel
	
	// Determine domain key
	domainKey := domain
	if domainKey == "" {
		domainKey = "local"
	}
	
	// Query using GSI4 for usage statistics, which sorts by usage count
	err := r.db.WithContext(ctx).Model(&models.EmojiModel{}).
		Index("gsi4").
		Where("GSI4PK", "=", fmt.Sprintf("USAGE#%s", domainKey)).
		Limit(limit * 2). // Get more than needed to filter disabled ones
		All(&emojiModels)
	
	if err != nil {
		r.logger.Error("failed to get popular emojis", zap.Error(err))
		return nil, err
	}
	
	// Filter out disabled emojis and apply limit
	emojis := make([]*storage.CustomEmoji, 0, limit)
	for _, model := range emojiModels {
		// Skip disabled emojis unless they're remote emojis
		if model.Disabled && model.Domain == "" {
			continue
		}
		
		if len(emojis) >= limit {
			break
		}
		
		emojis = append(emojis, r.convertModelToStorage(model))
	}
	
	return emojis, nil
}

// IncrementEmojiUsage increments the usage count for an emoji
func (r *EmojiRepository) IncrementEmojiUsage(ctx context.Context, shortcode string) error {
	// Get current emoji
	var model models.EmojiModel
	query := r.db.WithContext(ctx).Model(&models.EmojiModel{})
	err := query.
		Where("PK", "=", fmt.Sprintf("EMOJI#%s", shortcode)).
		Where("SK", "=", "EMOJI").
		First(&model)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return storage.ErrNotFound
		}
		r.logger.Error("failed to get emoji for usage increment", zap.Error(err))
		return err
	}
	
	// Increment usage and update keys
	model.IncrementUsage()
	
	// Update the emoji in database
	err = r.db.WithContext(ctx).Model(&model).Create()
	if err != nil {
		r.logger.Error("failed to increment emoji usage", zap.Error(err))
		return err
	}
	
	return nil
}

// Helper methods

// convertModelToStorage converts models.EmojiModel to storage.CustomEmoji
func (r *EmojiRepository) convertModelToStorage(model *models.EmojiModel) *storage.CustomEmoji {
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
		UsageCount:          model.UsageCount,
		LastUsedAt:          model.LastUsedAt,
		PopularityScore:     model.PopularityScore,
		SearchKeywords:      model.SearchKeywords,
		AltText:             model.AltText,
	}
}

// matchesSearchQuery checks if an emoji model matches the search query
func (r *EmojiRepository) matchesSearchQuery(model *models.EmojiModel, query string) bool {
	// Check shortcode (primary match)
	if strings.Contains(strings.ToLower(model.Shortcode), query) {
		return true
	}
	
	// Check category
	if model.Category != "" && strings.Contains(strings.ToLower(model.Category), query) {
		return true
	}
	
	// Check search keywords
	for _, keyword := range model.SearchKeywords {
		if strings.Contains(strings.ToLower(keyword), query) {
			return true
		}
	}
	
	// Check alt text
	if model.AltText != "" && strings.Contains(strings.ToLower(model.AltText), query) {
		return true
	}
	
	return false
}

// scoreSearchResults scores and sorts search results by relevance
func (r *EmojiRepository) scoreSearchResults(emojiModels []*models.EmojiModel, query string) []*models.EmojiModel {
	type scoredModel struct {
		model *models.EmojiModel
		score float64
	}
	
	scored := make([]scoredModel, 0, len(emojiModels))
	
	for _, model := range emojiModels {
		score := r.calculateSearchScore(model, query)
		scored = append(scored, scoredModel{model: model, score: score})
	}
	
	// Sort by score (highest first)
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
	
	// Extract models
	result := make([]*models.EmojiModel, len(scored))
	for i, s := range scored {
		result[i] = s.model
	}
	
	return result
}

// calculateSearchScore calculates relevance score for search results
func (r *EmojiRepository) calculateSearchScore(model *models.EmojiModel, query string) float64 {
	score := 0.0
	shortcode := strings.ToLower(model.Shortcode)
	
	// Exact match gets highest score
	if shortcode == query {
		score += 100.0
	} else if strings.HasPrefix(shortcode, query) {
		// Prefix match gets high score
		score += 80.0
	} else if strings.Contains(shortcode, query) {
		// Contains match gets medium score
		score += 50.0
	}
	
	// Category match
	if model.Category != "" && strings.Contains(strings.ToLower(model.Category), query) {
		score += 20.0
	}
	
	// Search keywords match
	for _, keyword := range model.SearchKeywords {
		if strings.Contains(strings.ToLower(keyword), query) {
			score += 15.0
		}
	}
	
	// Alt text match
	if model.AltText != "" && strings.Contains(strings.ToLower(model.AltText), query) {
		score += 10.0
	}
	
	// Popularity boost
	score += model.PopularityScore * 5.0
	
	// Recent usage boost
	if !model.LastUsedAt.IsZero() {
		daysSinceLastUse := time.Since(model.LastUsedAt).Hours() / 24
		if daysSinceLastUse < 7 {
			score += (7 - daysSinceLastUse) * 2.0
		}
	}
	
	return score
}
