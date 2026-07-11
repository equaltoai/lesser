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

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	"github.com/theory-cloud/tabletheory/v2/pkg/errors"
)

// FeaturedTagRepository implements featured tag operations using enhanced DynamORM patterns
type FeaturedTagRepository struct {
	*EnhancedBaseRepository[*models.FeaturedTag]
}

// NewFeaturedTagRepository creates a new featured tag repository with enhanced functionality and cost tracking
func NewFeaturedTagRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *FeaturedTagRepository {
	// Create enhanced repository for featured tag operations
	enhancedRepo := NewEnhancedBaseRepository[*models.FeaturedTag](db, tableName, logger, costService, "FeaturedTagRepository", "featured_tag")

	// Set up enhanced services for featured tag operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Cache featured tags for performance
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &FeaturedTagRepository{
		EnhancedBaseRepository: enhancedRepo,
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
		return ErrorHandler.HandleQueryError(err, EntityFeaturedTag, "existing tags check")
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

	// Create using BaseRepository
	err = r.ValidateAndCreate(ctx, featuredTagModel)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityFeaturedTag, tagName)
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
		return ErrorHandler.HandleQueryError(err, EntityFeaturedTag, "featured tags lookup")
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

	// Delete using BaseRepository
	pk := fmt.Sprintf("USER#%s", username)
	sk := fmt.Sprintf("FEATURED_TAG#%s", targetID)
	err = r.Delete(ctx, pk, sk)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return storage.ErrNotFound
		}
		return ErrorHandler.HandleDeleteError(err, EntityFeaturedTag, name)
	}

	return nil
}

// GetFeaturedTags returns all featured tags for a user
func (r *FeaturedTagRepository) GetFeaturedTags(ctx context.Context, username string) ([]*storage.FeaturedTag, error) {
	pk := fmt.Sprintf("USER#%s", username)
	const featuredTagChunkLimit = 100

	var (
		featuredTagModels []*models.FeaturedTag
		cursor            string
	)

	for {
		page, err := r.QueryWithSKPrefixPaginated(ctx, pk, "FEATURED_TAG#", BasePaginationOptions{
			Limit:  featuredTagChunkLimit,
			Cursor: cursor,
			Order:  SortOrderAsc,
		})
		if err != nil {
			return []*storage.FeaturedTag{}, nil
		}

		featuredTagModels = append(featuredTagModels, page.Items...)
		if page.NextCursor == "" || len(page.Items) == 0 {
			break
		}
		cursor = page.NextCursor
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
		return nil, ErrorHandler.HandleQueryError(err, EntityFeaturedTag, "featured tags lookup")
	}

	featuredMap := make(map[string]bool)
	for _, tag := range featuredTags {
		featuredMap[strings.ToLower(tag.Name)] = true
	}

	// Query user's recent statuses using GSI3
	var statusModels []models.Status
	err = r.GetDB().WithContext(ctx).Model(&models.Status{}).
		Index("gsi3").
		Where("gsi3PK", "=", fmt.Sprintf("USER_STATUS#%s", username)).
		OrderBy("gsi3SK", "DESC").
		Limit(100). // Analyze last 100 statuses
		All(&statusModels)
	if err != nil && !errors.IsNotFound(err) {
		return nil, ErrorHandler.HandleQueryError(err, EntityStatus, "user statuses for suggestions")
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
	err := r.GetDB().WithContext(ctx).Model(&models.Status{}).
		Index("gsi3").
		Where("gsi3PK", "=", fmt.Sprintf("USER_STATUS#%s", userID)).
		OrderBy("gsi3SK", "DESC"). // Most recent first
		All(&statusModels)
	if err != nil {
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
