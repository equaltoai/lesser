package repositories

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/equaltoai/lesser/pkg/common"
)

// FeaturedTagRepository implements featured tag operations using DynamORM
type FeaturedTagRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewFeaturedTagRepository creates a new featured tag repository
func NewFeaturedTagRepository(db core.DB, tableName string, logger *zap.Logger) *FeaturedTagRepository {
	return &FeaturedTagRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreateFeaturedTag creates a new featured tag for a user
func (r *FeaturedTagRepository) CreateFeaturedTag(ctx context.Context, tag *storage.FeaturedTag) error {
	// Normalize tag name (remove # if present)
	tagName := strings.TrimPrefix(tag.Name, "#")
	tagName = strings.ToLower(tagName)

	// Check if already featured
	existing, err := r.GetFeaturedTags(ctx, tag.Username)
	if err != nil {
		return fmt.Errorf("failed to check existing featured tags: %w", err)
	}

	for _, existingTag := range existing {
		if existingTag.Name == tagName {
			return storage.ErrAlreadyExists
		}
	}

	// Calculate tag statistics
	statusesCount, lastStatusAt := r.calculateTagStatistics(ctx, tag.Username, tagName)

	// Create new featured tag with proper fields
	id := uuid.New().String()
	featuredTagModel := &models.FeaturedTag{
		ID:            id,
		Username:      tag.Username,
		Name:          tagName,
		URL:           fmt.Sprintf("https://%s/tags/%s", DefaultDomain, tagName), // Placeholder domain
		StatusesCount: statusesCount,
		LastStatusAt: func() string {
			if lastStatusAt != nil {
				return lastStatusAt.Format(time.RFC3339)
			}
			return ""
		}(),
		CreatedAt: time.Now(),
	}

	// Update keys
	featuredTagModel.UpdateKeys()

	// Create using DynamORM
	err = r.db.WithContext(ctx).Model(featuredTagModel).Create()
	if err != nil {
		return fmt.Errorf("failed to create featured tag: %w", err)
	}

	// Update the original tag with the generated values
	tag.ID = id
	tag.Name = tagName
	tag.URL = featuredTagModel.URL
	tag.StatusesCount = statusesCount
	tag.LastStatusAt = lastStatusAt
	tag.CreatedAt = featuredTagModel.CreatedAt

	return nil
}

// DeleteFeaturedTag removes a featured tag
func (r *FeaturedTagRepository) DeleteFeaturedTag(ctx context.Context, username, name string) error {
	// First, get the featured tag to find its ID
	featuredTags, err := r.GetFeaturedTags(ctx, username)
	if err != nil {
		return fmt.Errorf("failed to get featured tags: %w", err)
	}

	var targetID string
	for _, tag := range featuredTags {
		if strings.EqualFold(tag.Name, name) {
			targetID = tag.ID
			break
		}
	}

	if err := common.ValidateRequiredParam("targetID", targetID); err != nil {
		return storage.ErrNotFound
	}

	// Create a model with the correct keys for deletion
	featuredTagModel := &models.FeaturedTag{
		ID:       targetID,
		Username: username,
	}
	featuredTagModel.UpdateKeys()

	// Delete using DynamORM
	err = r.db.WithContext(ctx).Model(featuredTagModel).Delete()
	if err != nil {
		if errors.IsNotFound(err) {
			return storage.ErrNotFound
		}
		return fmt.Errorf("failed to delete featured tag: %w", err)
	}

	return nil
}

// GetFeaturedTags returns all featured tags for a user
func (r *FeaturedTagRepository) GetFeaturedTags(ctx context.Context, username string) ([]*storage.FeaturedTag, error) {
	var featuredTagModels []models.FeaturedTag

	err := r.db.WithContext(ctx).Model(&models.FeaturedTag{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Filter("SK", "BEGINS_WITH", "FEATURED_TAG#").
		All(&featuredTagModels)
	if err != nil {
		if errors.IsNotFound(err) {
			// Return empty slice when no featured tags found (not an error)
			return []*storage.FeaturedTag{}, nil
		}
		return nil, fmt.Errorf("failed to query featured tags: %w", err)
	}

	// Convert models to storage types
	tags := make([]*storage.FeaturedTag, 0, len(featuredTagModels))
	for _, model := range featuredTagModels {
		tags = append(tags, &storage.FeaturedTag{
			ID:            model.ID,
			Username:      model.Username,
			Name:          model.Name,
			URL:           model.URL,
			StatusesCount: model.StatusesCount,
			LastStatusAt: func() *time.Time {
				if model.LastStatusAt != "" {
					if t, err := time.Parse(time.RFC3339, model.LastStatusAt); err == nil {
						return &t
					}
				}
				return nil
			}(),
			CreatedAt: model.CreatedAt,
		})
	}

	return tags, nil
}

// GetTagSuggestions returns suggested tags based on user's usage
func (r *FeaturedTagRepository) GetTagSuggestions(ctx context.Context, username string, limit int) ([]string, error) {
	// Get already featured tags to exclude them
	featuredTags, err := r.GetFeaturedTags(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get featured tags: %w", err)
	}

	featuredMap := make(map[string]bool)
	for _, tag := range featuredTags {
		featuredMap[strings.ToLower(tag.Name)] = true
	}

	// Query user's recent statuses using GSI3
	var statusModels []models.Status
	err = r.db.WithContext(ctx).Model(&models.Status{}).
		Index("GSI3").
		Where("GSI3PK", "=", fmt.Sprintf("USER_STATUS#%s", username)).
		OrderBy("GSI3SK", "DESC").
		Limit(100). // Analyze last 100 statuses
		All(&statusModels)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to query user statuses: %w", err)
	}

	// Count tag usage
	tagCount := make(map[string]int)
	hashtagRegex := regexp.MustCompile(`#[a-zA-Z0-9_]+`)

	for _, statusModel := range statusModels {
		// Extract content from the Note
		if statusModel.Note != nil && statusModel.Note.Content != "" {
			// Extract all hashtags from content
			matches := hashtagRegex.FindAllString(statusModel.Note.Content, -1)
			for _, match := range matches {
				tag := strings.ToLower(strings.TrimPrefix(match, "#"))
				// Skip if already featured
				if !featuredMap[tag] {
					tagCount[tag]++
				}
			}
		}
	}

	// Sort tags by usage count
	type tagFreq struct {
		tag   string
		count int
	}

	tagFreqs := make([]tagFreq, 0, len(tagCount))
	for tag, count := range tagCount {
		tagFreqs = append(tagFreqs, tagFreq{tag: tag, count: count})
	}

	sort.Slice(tagFreqs, func(i, j int) bool {
		return tagFreqs[i].count > tagFreqs[j].count
	})

	// Return top suggestions
	suggestions := make([]string, 0, limit)
	for i := 0; i < len(tagFreqs) && i < limit; i++ {
		suggestions = append(suggestions, tagFreqs[i].tag)
	}

	return suggestions, nil
}

// calculateTagStatistics calculates the count and last usage time for a tag
func (r *FeaturedTagRepository) calculateTagStatistics(ctx context.Context, userID string, tagName string) (int, *time.Time) {
	// Query user's statuses using GSI3 to find those with the tag
	var statusModels []models.Status
	err := r.db.WithContext(ctx).Model(&models.Status{}).
		Index("GSI3").
		Where("GSI3PK", "=", fmt.Sprintf("USER_STATUS#%s", userID)).
		OrderBy("GSI3SK", "DESC"). // Most recent first
		All(&statusModels)
	if err != nil {
		r.logger.Warn("failed to query user statuses for tag statistics",
			zap.String("user_id", userID),
			zap.String("tag", tagName),
			zap.Error(err))
		return 0, nil
	}

	count := 0
	var lastStatusAt *time.Time
	tagPattern := fmt.Sprintf("#%s", tagName)

	for _, statusModel := range statusModels {
		// Check if status contains the tag
		if statusModel.Note != nil && statusModel.Note.Content != "" {
			// Simple case-insensitive check for the hashtag
			if strings.Contains(strings.ToLower(statusModel.Note.Content), strings.ToLower(tagPattern)) {
				count++

				// Get the timestamp of the first (most recent) match
				if lastStatusAt == nil && statusModel.Note != nil && statusModel.Note.Published != nil {
					lastStatusAt = statusModel.Note.Published
				}
			}
		}
	}

	return count, lastStatusAt
}
