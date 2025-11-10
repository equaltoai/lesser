package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// EmojiRepository handles custom emoji operations using enhanced DynamORM patterns
type EmojiRepository struct {
	*EnhancedBaseRepository[*models.EmojiModel]
}

// NewEmojiRepository creates a new emoji repository with enhanced functionality and cost tracking
func NewEmojiRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *EmojiRepository {
	// Create enhanced repository for emoji operations
	enhancedRepo := NewEnhancedBaseRepository[*models.EmojiModel](db, tableName, logger, costService, "EmojiRepository", "emoji")

	// Set up enhanced services for emoji operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Cache emojis for performance
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &EmojiRepository{
		EnhancedBaseRepository: enhancedRepo,
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

	// Check if emoji already exists
	pk := fmt.Sprintf("EMOJI#%s", emoji.Shortcode)
	if emoji.Domain != "" {
		pk = fmt.Sprintf("EMOJI#%s@%s", emoji.Shortcode, emoji.Domain)
	}

	exists, err := r.Exists(ctx, pk, "EMOJI")
	if err != nil {
		return err
	}

	if exists {
		return storage.ErrAlreadyExists
	}

	// Create the emoji using enhanced validation and creation
	return r.ValidateAndCreate(ctx, model)
}

// GetCustomEmoji retrieves a custom emoji by shortcode
func (r *EmojiRepository) GetCustomEmoji(ctx context.Context, shortcode string) (*storage.CustomEmoji, error) {
	var model models.EmojiModel
	pk := fmt.Sprintf("EMOJI#%s", shortcode)

	err := r.Get(ctx, pk, "EMOJI", &model)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}

	// Convert models.EmojiModel to storage.CustomEmoji
	return r.convertModelToStorage(&model), nil
}

// GetCustomEmojis retrieves all custom emojis (not disabled)
func (r *EmojiRepository) GetCustomEmojis(ctx context.Context) ([]*storage.CustomEmoji, error) {
	// Query using GSI1 for all emojis
	emojiModels, err := r.queryEmojiGSI(ctx, "gsi1", "gsi1PK", "ALL_EMOJIS", 0)
	if err != nil {
		// If GSI doesn't exist or query fails, return empty list (graceful degradation)
		// This allows the system to work even if emojis haven't been set up yet
		if errors.IsNotFound(err) || strings.Contains(strings.ToLower(err.Error()), "not found") {
			r.logger.Debug("no emojis found or GSI not available, returning empty list")
			return []*storage.CustomEmoji{}, nil
		}
		r.logger.Warn("failed to query emojis, returning empty list",
			zap.Error(err))
		return []*storage.CustomEmoji{}, nil
	}

	// Filter out disabled emojis and convert to storage type
	emojis := make([]*storage.CustomEmoji, 0, len(emojiModels))
	for _, model := range emojiModels {
		// Skip disabled emojis unless they're remote emojis
		if model.Disabled && (common.ValidateRequiredParam("domain", model.Domain) != nil) {
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

	// Check if emoji exists first
	pk := fmt.Sprintf("EMOJI#%s", emoji.Shortcode)
	if emoji.Domain != "" {
		pk = fmt.Sprintf("EMOJI#%s@%s", emoji.Shortcode, emoji.Domain)
	}

	exists, err := r.Exists(ctx, pk, "EMOJI")
	if err != nil {
		return err
	}

	if !exists {
		return storage.ErrNotFound
	}

	// Update the emoji using BaseRepository
	return r.Update(ctx, model)
}

// GetRemoteEmoji retrieves a remote emoji by shortcode and domain
func (r *EmojiRepository) GetRemoteEmoji(ctx context.Context, shortcode, domain string) (*storage.CustomEmoji, error) {
	var model models.EmojiModel
	pk := fmt.Sprintf("EMOJI#%s@%s", shortcode, domain)

	err := r.Get(ctx, pk, "EMOJI", &model)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}

	// Convert to storage model
	return r.convertModelToStorage(&model), nil
}

// DeleteCustomEmoji deletes a custom emoji
func (r *EmojiRepository) DeleteCustomEmoji(ctx context.Context, shortcode string) error {
	pk := fmt.Sprintf("EMOJI#%s", shortcode)

	// Check if emoji exists first
	exists, err := r.Exists(ctx, pk, "EMOJI")
	if err != nil {
		return err
	}

	if !exists {
		return storage.ErrNotFound
	}

	// Delete the emoji using BaseRepository
	return r.Delete(ctx, pk, "EMOJI")
}

// GetCustomEmojisByCategory retrieves custom emojis by category
func (r *EmojiRepository) GetCustomEmojisByCategory(ctx context.Context, category string) ([]*storage.CustomEmoji, error) {
	// Query using GSI2 for category
	emojiModels, err := r.queryEmojiGSI(ctx, "gsi2", "gsi2PK", fmt.Sprintf("CATEGORY#%s", category), 0)
	if err != nil {
		return nil, err
	}

	// Filter out disabled emojis and convert to storage type
	emojis := make([]*storage.CustomEmoji, 0, len(emojiModels))
	for _, model := range emojiModels {
		// Skip disabled emojis unless they're remote emojis
		if model.Disabled && (common.ValidateRequiredParam("domain", model.Domain) != nil) {
			continue
		}

		emojis = append(emojis, r.convertModelToStorage(model))
	}

	return emojis, nil
}

// SearchEmojis performs sophisticated emoji searches with relevance scoring
func (r *EmojiRepository) SearchEmojis(ctx context.Context, query string, limit int) ([]*storage.CustomEmoji, error) {
	if err := common.ValidateRequiredParam("query", query); err != nil || limit <= 0 {
		return []*storage.CustomEmoji{}, nil
	}

	// Normalize query for search
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))

	// Try different search strategies
	var allModels []*models.EmojiModel

	// Strategy 1: Prefix search using GSI3
	if len(normalizedQuery) >= 3 {
		prefix := normalizedQuery[:3]
		prefixModels, err := r.queryEmojiGSI(ctx, "gsi3", "gsi3PK", fmt.Sprintf("SEARCH#%s", prefix), 0)
		if err == nil {
			allModels = append(allModels, prefixModels...)
		}
	}

	// Strategy 2: Get all emojis and perform in-memory search for broader matching
	allEmojis, err := r.queryEmojiGSI(ctx, "gsi1", "gsi1PK", "ALL_EMOJIS", 0)
	if err != nil {
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

	// Determine domain key
	domainKey := domain
	if err := common.ValidateRequiredParam("domain", domainKey); err != nil {
		domainKey = "local"
	}

	// Query using GSI4 for usage statistics, which sorts by usage count
	emojiModels, err := r.queryEmojiGSI(ctx, "gsi4", "gsi4PK", fmt.Sprintf("USAGE#%s", domainKey), limit*2)
	if err != nil {
		return nil, err
	}

	// Filter out disabled emojis and apply limit
	emojis := make([]*storage.CustomEmoji, 0, limit)
	for _, model := range emojiModels {
		// Skip disabled emojis unless they're remote emojis
		if model.Disabled && (common.ValidateRequiredParam("domain", model.Domain) != nil) {
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
	pk := fmt.Sprintf("EMOJI#%s", shortcode)

	err := r.Get(ctx, pk, "EMOJI", &model)
	if err != nil {
		if errors.IsNotFound(err) {
			return storage.ErrNotFound
		}
		return err
	}

	// Increment usage and update keys
	model.IncrementUsage()

	// Update the emoji in database using BaseRepository
	return r.Update(ctx, &model)
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
	if common.ValidateRequiredParam("category", model.Category) == nil && strings.Contains(strings.ToLower(model.Category), query) {
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

// queryEmojiGSI is a helper method for querying GSI with correct field names
func (r *EmojiRepository) queryEmojiGSI(ctx context.Context, indexName, pkField, pkValue string, limit int) ([]*models.EmojiModel, error) {
	var results []*models.EmojiModel

	// Create query
	query := r.db.WithContext(ctx).Model(&models.EmojiModel{}).
		Index(indexName).
		Where(pkField, "=", pkValue)

	if limit > 0 {
		query = query.Limit(limit)
	}

	// Execute query
	err := query.All(&results)
	if err != nil {
		r.logger.Error("failed to query emoji GSI",
			zap.Error(err),
			zap.String("index", indexName),
			zap.String("pkField", pkField),
			zap.String("pkValue", pkValue),
			zap.Int("limit", limit))
		return nil, ErrorHandler.HandleQueryError(err, EntityEmoji, fmt.Sprintf("GSI query %s", indexName))
	}

	// Track cost if cost service is available
	if r.GetCostService() != nil {
		itemCount := int64(len(results))
		estimatedRU := itemCount
		if estimatedRU == 0 {
			estimatedRU = 1
		}

		if trackErr := r.TrackRead(ctx, "QueryGSI", estimatedRU); trackErr != nil {
			r.logger.Warn("failed to track GSI query cost",
				zap.String("index", indexName),
				zap.Error(trackErr))
		}
	}

	return results, nil
}
