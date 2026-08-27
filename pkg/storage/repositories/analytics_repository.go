package repositories

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

const (
	// DefaultDomain is the default domain for local development
	DefaultDomain = "localhost"
	// LinkTypePhoto represents photo link type
	LinkTypePhoto = "photo"
	// LinkTypeVideo represents video link type
	LinkTypeVideo = "video"
)

// TrendingRepository implements trending and analytics operations using enhanced patterns
type TrendingRepository struct {
	*EnhancedBaseRepository[*models.TrendingHashtag]
	db         core.DB
	logger     *zap.Logger
	domain     string
	statusRepo interface{} // Generic interface to avoid circular dependency
}

// NewTrendingRepository creates a new trending repository
func NewTrendingRepository(db core.DB, logger *zap.Logger, costService *cost.TrackingService) *TrendingRepository {
	// Get domain from config, default to localhost for development
	cfg := config.Get()
	domain := cfg.Domain
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		domain = DefaultDomain
	}

	// Create enhanced repository optimized for trending and analytics operations
	enhancedRepo := NewEnhancedBaseRepository[*models.TrendingHashtag](
		db,
		cfg.DynamoTableName,
		logger,
		costService,
		"TrendingRepository",
		"trending",
	)

	// Set up enhanced services for trending operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Trending data heavily cached
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for trend tracking

	return &TrendingRepository{
		EnhancedBaseRepository: enhancedRepo,
		db:                     db,
		logger:                 logger,
		domain:                 domain,
	}
}

// SetStatusRepository sets the status repository dependency for cross-repository operations
func (r *TrendingRepository) SetStatusRepository(statusRepo interface{}) {
	r.statusRepo = statusRepo
}

// RecordHashtagUsage records when a hashtag is used in a status
func (r *TrendingRepository) RecordHashtagUsage(ctx context.Context, hashtag string, statusID string, authorID string) error {
	now := time.Now()
	usage := &models.HashtagUsage{
		StatusID:   statusID,
		AuthorID:   authorID,
		UsedAt:     now,
		Visibility: "public",                            // Default visibility
		TTL:        now.Add(30 * 24 * time.Hour).Unix(), // 30 days as per existing model
		CreatedAt:  now,
	}

	// Set keys using the parameterized UpdateKeys method
	usage.UpdateKeysWithHashtag(hashtag)

	err := r.db.WithContext(ctx).Model(usage).Create()
	if err != nil {
		r.logger.Error("failed to record hashtag usage",
			zap.String("hashtag", hashtag),
			zap.String("statusID", statusID),
			zap.Error(err))
		return err
	}

	// Update hashtag trend score
	return r.updateHashtagTrendScore(ctx, hashtag)
}

// RecordStatusEngagement records engagement on a status (like, boost, reply)
func (r *TrendingRepository) RecordStatusEngagement(ctx context.Context, statusID string, engagementType string, userID string) error {
	engagement := &models.StatusEngagement{
		StatusID:       statusID,
		EngagementType: engagementType,
		UserID:         userID,
		EngagedAt:      time.Now(),
		TTL:            time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	_ = engagement.UpdateKeys() // sets PK/SK + GSI1 global listing (wave batch E)

	err := r.db.WithContext(ctx).Model(engagement).Create()
	if err != nil {
		r.logger.Error("failed to record status engagement",
			zap.String("statusID", statusID),
			zap.String("engagementType", engagementType),
			zap.Error(err))
		return err
	}

	// Update status trend score
	return r.updateStatusTrendScore(ctx, statusID)
}

// RecordLinkShare records when a link is shared in a status
func (r *TrendingRepository) RecordLinkShare(ctx context.Context, linkURL string, statusID string, authorID string) error {
	share := &models.LinkShare{
		URL:      linkURL,
		StatusID: statusID,
		AuthorID: authorID,
		SharedAt: time.Now(),
		TTL:      time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	_ = share.UpdateKeys() // sets PK/SK + GSI1 global listing (wave batch E)

	err := r.db.WithContext(ctx).Model(share).Create()
	if err != nil {
		r.logger.Error("failed to record link share",
			zap.String("url", linkURL),
			zap.String("statusID", statusID),
			zap.Error(err))
		return err
	}

	// Update link trend score
	return r.updateLinkTrendScore(ctx, linkURL)
}

// GetTrendingHashtags returns the top trending hashtags since the given time
func (r *TrendingRepository) GetTrendingHashtags(ctx context.Context, _ time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	return r.getTrendingHashtagsInternal(ctx, "HASHTAG", limit)
}

// GetTrendingStatuses returns the top trending statuses since the given time
func (r *TrendingRepository) GetTrendingStatuses(ctx context.Context, _ time.Time, limit int) ([]*storage.TrendingStatus, error) {
	return r.getTrendingStatusesInternal(ctx, "STATUS", limit)
}

// GetTrendingLinks returns the top trending links since the given time
func (r *TrendingRepository) GetTrendingLinks(ctx context.Context, _ time.Time, limit int) ([]*storage.TrendingLink, error) {
	return r.getTrendingLinksInternal(ctx, "LINK", limit)
}

// Generic helper functions to eliminate duplication

// getTrendingItemsGeneric is a generic function to fetch trending items
func (r *TrendingRepository) getTrendingItemsGeneric(ctx context.Context, trendType string, limit int, modelInstance interface{}, converter func(interface{}) interface{}) (interface{}, error) {
	timeBucket := time.Now().Format(common.DateFormat)
	pk := fmt.Sprintf("TREND_TYPE#%s#%s", trendType, timeBucket)

	// Use reflection to create a slice of the appropriate type
	modelType := reflect.TypeOf(modelInstance)
	sliceType := reflect.SliceOf(modelType)
	trendsValue := reflect.New(sliceType).Elem()

	err := r.db.WithContext(ctx).Model(modelInstance).
		Where("gsi8PK", "=", pk).
		OrderBy("gsi8SK", "DESC"). // Sort by score descending
		Limit(limit).
		All(trendsValue.Addr().Interface())

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug(fmt.Sprintf("no trending %s found", strings.ToLower(trendType)), zap.String("timeBucket", timeBucket))
			// Return empty slice of the appropriate result type
			resultType := reflect.TypeOf(converter(modelInstance))
			resultSliceType := reflect.SliceOf(resultType)
			return reflect.New(resultSliceType).Elem().Interface(), nil
		}
		r.logger.Error(fmt.Sprintf("failed to get trending %s", strings.ToLower(trendType)), zap.Error(err))
		return nil, err
	}

	// Convert models to result type
	count := trendsValue.Len()
	resultType := reflect.TypeOf(converter(modelInstance))
	resultSliceType := reflect.SliceOf(resultType)
	resultSlice := reflect.MakeSlice(resultSliceType, 0, count)

	for i := 0; i < count; i++ {
		model := trendsValue.Index(i).Interface()
		converted := converter(model)
		resultSlice = reflect.Append(resultSlice, reflect.ValueOf(converted))
	}

	return resultSlice.Interface(), nil
}

// getTrendingHashtagsInternal handles the actual hashtag trend fetching
func (r *TrendingRepository) getTrendingHashtagsInternal(ctx context.Context, trendType string, limit int) ([]*storage.TrendingHashtag, error) {
	result, err := r.getTrendingItemsGeneric(ctx, trendType, limit, &models.HashtagTrend{}, func(model interface{}) interface{} {
		m := model.(*models.HashtagTrend)
		return &storage.TrendingHashtag{
			Name:        m.Name,
			URL:         m.URL,
			UsageCount:  m.UsageCount,
			UniqueUsers: m.UniqueUsers,
			LastUsed:    m.LastUsed,
			FirstSeen:   m.FirstSeen,
		}
	})
	if err != nil {
		return nil, err
	}
	return result.([]*storage.TrendingHashtag), nil
}

// getTrendingStatusesInternal handles the actual status trend fetching
func (r *TrendingRepository) getTrendingStatusesInternal(ctx context.Context, trendType string, limit int) ([]*storage.TrendingStatus, error) {
	result, err := r.getTrendingItemsGeneric(ctx, trendType, limit, &models.StatusTrend{}, func(model interface{}) interface{} {
		m := model.(*models.StatusTrend)
		return &storage.TrendingStatus{
			ID:          m.ID,
			URL:         m.URL,
			AuthorID:    m.AuthorID,
			Content:     m.Content,
			Engagements: m.Engagements,
			PublishedAt: m.PublishedAt,
		}
	})
	if err != nil {
		return nil, err
	}
	return result.([]*storage.TrendingStatus), nil
}

// getTrendingLinksInternal handles the actual link trend fetching
func (r *TrendingRepository) getTrendingLinksInternal(ctx context.Context, trendType string, limit int) ([]*storage.TrendingLink, error) {
	timeBucket := time.Now().Format(common.DateFormat)
	pk := fmt.Sprintf("TREND_TYPE#%s#%s", trendType, timeBucket)

	var trendModels []models.LinkTrend
	err := r.db.WithContext(ctx).Model(&models.LinkTrend{}).
		Where("gsi8PK", "=", pk).
		OrderBy("gsi8SK", "DESC"). // Sort by score descending
		Limit(limit).
		All(&trendModels)

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug(fmt.Sprintf("no trending %s found", strings.ToLower(trendType)), zap.String("timeBucket", timeBucket))
			return []*storage.TrendingLink{}, nil
		}
		r.logger.Error(fmt.Sprintf("failed to get trending %s", strings.ToLower(trendType)), zap.Error(err))
		return nil, err
	}

	trends := make([]*storage.TrendingLink, 0, len(trendModels))
	for _, model := range trendModels {
		trend := &storage.TrendingLink{
			URL:         model.URL,
			Title:       model.Title,
			Description: model.Description,
			Type:        model.Type,
			AuthorName:  model.AuthorName,
			Image:       model.Image,
			ShareCount:  model.ShareCount,
		}
		trends = append(trends, trend)
	}

	return trends, nil
}

// TrendModel represents any trend model that can be stored
type TrendModel interface {
	UpdateKeys() error
}

// storeTrendInternal handles the common pattern for storing any type of trend
func (r *TrendingRepository) storeTrendInternal(ctx context.Context, model TrendModel, trendType string, identifier string) error {
	// Update keys
	if err := model.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "trend", fmt.Sprintf("%s:%s", trendType, identifier))
	}

	// Store the trend
	err := r.db.WithContext(ctx).Model(model).CreateOrUpdate()
	if err != nil {
		r.logger.Error(fmt.Sprintf("failed to store %s trend", trendType),
			zap.String("identifier", identifier),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "trend", trendType)
	}

	return nil
}

// storeHashtagTrendInternal handles the common pattern for storing hashtag trends
func (r *TrendingRepository) storeHashtagTrendInternal(ctx context.Context, trend any) error {
	// Convert from interface{} to expected type
	hashtagTrend, ok := trend.(*models.HashtagTrend)
	if !ok {
		// Try to convert from storage type
		storageTrend, ok := trend.(*storage.TrendingHashtag)
		if !ok {
			return ErrInvalidHashtagTrendType
		}

		// Convert storage to model
		hashtagTrend = &models.HashtagTrend{
			Name:        storageTrend.Name,
			URL:         storageTrend.URL,
			UsageCount:  storageTrend.UsageCount,
			UniqueUsers: storageTrend.UniqueUsers,
			LastUsed:    storageTrend.LastUsed,
			FirstSeen:   storageTrend.FirstSeen,
			TrendScore:  float64(storageTrend.UsageCount),
			UpdatedAt:   time.Now(),
		}
	}

	return r.storeTrendInternal(ctx, hashtagTrend, "hashtag", hashtagTrend.Name)
}

// storeStatusTrendInternal handles the common pattern for storing status trends
func (r *TrendingRepository) storeStatusTrendInternal(ctx context.Context, trend any) error {
	// Convert from interface{} to expected type
	statusTrend, ok := trend.(*models.StatusTrend)
	if !ok {
		// Try to convert from storage type
		storageTrend, ok := trend.(*storage.TrendingStatus)
		if !ok {
			return ErrInvalidStatusTrendType
		}

		// Convert storage to model
		statusTrend = &models.StatusTrend{
			ID:          storageTrend.ID,
			URL:         storageTrend.URL,
			AuthorID:    storageTrend.AuthorID,
			Content:     storageTrend.Content,
			Engagements: storageTrend.Engagements,
			PublishedAt: storageTrend.PublishedAt,
			TrendScore:  float64(storageTrend.Engagements),
			UpdatedAt:   time.Now(),
		}
	}

	return r.storeTrendInternal(ctx, statusTrend, "status", statusTrend.ID)
}

// storeLinkTrendInternal handles the common pattern for storing link trends
func (r *TrendingRepository) storeLinkTrendInternal(ctx context.Context, trend any) error {
	// Convert from interface{} to expected type
	linkTrend, ok := trend.(*models.LinkTrend)
	if !ok {
		// Try to convert from storage type
		storageTrend, ok := trend.(*storage.TrendingLink)
		if !ok {
			return ErrInvalidLinkTrendType
		}

		// Convert storage to model
		linkTrend = &models.LinkTrend{
			URL:         storageTrend.URL,
			Title:       storageTrend.Title,
			Description: storageTrend.Description,
			Type:        storageTrend.Type,
			AuthorName:  storageTrend.AuthorName,
			Image:       storageTrend.Image,
			ShareCount:  storageTrend.ShareCount,
			TrendScore:  float64(storageTrend.ShareCount),
			UpdatedAt:   time.Now(),
		}
	}

	return r.storeTrendInternal(ctx, linkTrend, "link", linkTrend.URL)
}

// Helper methods for updating trend scores

func (r *TrendingRepository) updateHashtagTrendScore(ctx context.Context, hashtag string) error {
	// Query recent usage (last 24 hours)
	since := time.Now().Add(-24 * time.Hour)
	// Use the correct key pattern from the existing model
	pk := fmt.Sprintf("HASHTAG#%s", strings.ToLower(strings.TrimPrefix(hashtag, "#")))

	var usageRecords []models.HashtagUsage
	err := r.db.WithContext(ctx).Model(&models.HashtagUsage{}).
		Where("PK", "=", pk).
		All(&usageRecords)

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to query hashtag usage", zap.String("hashtag", hashtag), zap.Error(err))
		return err
	}

	// Count unique users and recent usage
	uniqueUsers := make(map[string]bool)
	usageCount := int64(0)

	for _, record := range usageRecords {
		uniqueUsers[record.AuthorID] = true
		if record.UsedAt.After(since) {
			usageCount++
		}
	}

	// Calculate trend score using time-decay algorithm
	now := time.Now()
	ageFactor := 1.0 // New hashtags get full score
	diversityFactor := float64(len(uniqueUsers)) / float64(usageCount+1)
	score := float64(usageCount) * ageFactor * (1 + diversityFactor)

	// Update trending index entry
	timeBucket := now.Format(common.DateFormat)
	paddedScore := fmt.Sprintf("%010.0f", score*1000) // Pad for proper sorting

	trendItem := &models.HashtagTrend{
		PK:          fmt.Sprintf("TREND_TYPE#HASHTAG#%s", timeBucket),
		SK:          fmt.Sprintf("SCORE#%s#%s", paddedScore, hashtag),
		Name:        hashtag,
		URL:         fmt.Sprintf("https://%s/tags/%s", r.domain, hashtag),
		UsageCount:  usageCount,
		UniqueUsers: int64(len(uniqueUsers)),
		LastUsed:    now,
		FirstSeen:   now, // Would be updated if exists
		TrendScore:  score,
		UpdatedAt:   now,
		TTL:         now.Add(7 * 24 * time.Hour).Unix(),
	}
	_ = trendItem.UpdateKeys() // Ignore error as this is internal model operation

	err = r.db.WithContext(ctx).Model(trendItem).CreateOrUpdate()
	if err != nil {
		r.logger.Error("failed to update hashtag trend score", zap.String("hashtag", hashtag), zap.Error(err))
		return err
	}

	return nil
}

func (r *TrendingRepository) updateStatusTrendScore(ctx context.Context, statusID string) error {
	// For simplicity, we'll create a basic trend entry without fetching the actual status
	// In a full implementation, you'd fetch the status details first

	pk := fmt.Sprintf("STATUS_ENGAGEMENT#%s", statusID)

	var engagements []models.StatusEngagement
	err := r.db.WithContext(ctx).Model(&models.StatusEngagement{}).
		Where("PK", "=", pk).
		All(&engagements)

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to query status engagement", zap.String("statusID", statusID), zap.Error(err))
		return err
	}

	// Count engagement types
	engagementCounts := map[string]int{
		"like":  0,
		"boost": 0,
		"reply": 0,
	}
	uniqueEngagers := make(map[string]bool)

	for _, engagement := range engagements {
		engagementCounts[engagement.EngagementType]++
		uniqueEngagers[engagement.UserID] = true
	}

	// Calculate engagement score
	totalEngagements := int64(engagementCounts["like"] + engagementCounts["boost"]*2 + engagementCounts["reply"]*3)

	// Calculate trend score
	now := time.Now()
	publishedAt := now // Simplified - would get from actual status
	age := now.Sub(publishedAt)
	ageFactor := 1.0 / (1 + age.Hours()/2) // 2-hour half-life
	diversityFactor := float64(len(uniqueEngagers)) / float64(totalEngagements+1)
	score := float64(totalEngagements) * ageFactor * (1 + diversityFactor)

	// Update trending index entry
	timeBucket := now.Format(common.DateFormat)
	paddedScore := fmt.Sprintf("%010.0f", score*1000)

	trendItem := &models.StatusTrend{
		PK:          fmt.Sprintf("TREND_TYPE#STATUS#%s", timeBucket),
		SK:          fmt.Sprintf("SCORE#%s#%s", paddedScore, statusID),
		ID:          statusID,
		URL:         fmt.Sprintf("https://%s/statuses/%s", r.domain, statusID),
		AuthorID:    "", // Would be filled from actual status
		Content:     "", // Would be filled from actual status
		Engagements: totalEngagements,
		PublishedAt: publishedAt,
		TrendScore:  score,
		UpdatedAt:   now,
		TTL:         now.Add(7 * 24 * time.Hour).Unix(),
	}
	_ = trendItem.UpdateKeys() // Ignore error as this is internal model operation

	err = r.db.WithContext(ctx).Model(trendItem).CreateOrUpdate()
	if err != nil {
		r.logger.Error("failed to update status trend score", zap.String("statusID", statusID), zap.Error(err))
		return err
	}

	return nil
}

func (r *TrendingRepository) updateLinkTrendScore(ctx context.Context, linkURL string) error {
	// Query recent link shares
	pk := fmt.Sprintf("LINK_SHARE#%s", linkURL)

	var shares []models.LinkShare
	err := r.db.WithContext(ctx).Model(&models.LinkShare{}).
		Where("PK", "=", pk).
		All(&shares)

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to query link shares", zap.String("url", linkURL), zap.Error(err))
		return err
	}

	// Count unique sharers and recent shares
	uniqueSharers := make(map[string]bool)
	shareCount := int64(0)
	since := time.Now().Add(-24 * time.Hour)

	for _, share := range shares {
		uniqueSharers[share.AuthorID] = true
		if share.SharedAt.After(since) {
			shareCount++
		}
	}

	// Extract basic link metadata
	title := extractDomainFromURL(linkURL)
	description := ""
	image := ""
	linkType := "link"

	// Determine link type based on URL patterns
	lowerURL := strings.ToLower(linkURL)
	if strings.Contains(lowerURL, "youtube.com") || strings.Contains(lowerURL, "youtu.be") {
		linkType = LinkTypeVideo
		title = "YouTube Video"
	} else if strings.Contains(lowerURL, ".jpg") || strings.Contains(lowerURL, ".png") ||
		strings.Contains(lowerURL, ".gif") || strings.Contains(lowerURL, ".webp") {
		linkType = LinkTypePhoto
		title = "Image"
		image = linkURL
	}

	// Calculate trend score
	now := time.Now()
	diversityFactor := float64(len(uniqueSharers)) / float64(shareCount+1)
	score := float64(shareCount) * (1 + diversityFactor)

	// Update trending index entry
	timeBucket := now.Format(common.DateFormat)
	paddedScore := fmt.Sprintf("%010.0f", score*1000)

	trendItem := &models.LinkTrend{
		PK:          fmt.Sprintf("TREND_TYPE#LINK#%s", timeBucket),
		SK:          fmt.Sprintf("SCORE#%s#%s", paddedScore, linkURL),
		URL:         linkURL,
		Title:       title,
		Description: description,
		Type:        linkType,
		AuthorName:  "", // Could extract from first sharer
		Image:       image,
		ShareCount:  shareCount,
		TrendScore:  score,
		UpdatedAt:   now,
		TTL:         now.Add(7 * 24 * time.Hour).Unix(),
	}
	_ = trendItem.UpdateKeys() // Ignore error as this is internal model operation

	err = r.db.WithContext(ctx).Model(trendItem).CreateOrUpdate()
	if err != nil {
		r.logger.Error("failed to update link trend score", zap.String("url", linkURL), zap.Error(err))
		return err
	}

	return nil
}

// extractDomainFromURL extracts the domain name from a URL for use as a title
func extractDomainFromURL(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	domain := parsedURL.Hostname()
	// Remove www. prefix if present
	domain = strings.TrimPrefix(domain, "www.")

	return domain
}

// GetRecentHashtags returns recent hashtags since the given time (no trending calculation)
func (r *TrendingRepository) GetRecentHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	// Validate limit using centralized validation
	if err := common.ValidateQueryLimit(limit, 100, "analytics"); err != nil {
		limit = 20
	}

	// Hashtag metadata rows (HASHTAG#<name> / METADATA) are only written by
	// HashtagRepository.IndexHashtag, which has zero production callers — no
	// live writer maintains them, so no rows exist to key. A GSI listing key
	// added to the model would never be populated and keying this read on it
	// would silently return nothing, so this stays on the baselined
	// SK = METADATA scan (disposition "elimination pending — wave #1469" with
	// a no-live-writer note; see docs/architecture/dynamodb-scan-inventory.md).
	// The wave part 2 batch E GSI1 conversion was reverted for that reason.
	var hashtagModels []*models.Hashtag
	err := r.db.WithContext(ctx).Model(&models.Hashtag{}).
		Where("SK", "=", "METADATA").
		Where("LastUsed", ">=", since.Format(time.RFC3339)).
		OrderBy("LastUsed", "DESC"). // Recent first
		Limit(limit).
		All(&hashtagModels)

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("no recent hashtags found", zap.Time("since", since))
			return []*storage.TrendingHashtag{}, nil
		}
		r.logger.Error("failed to query recent hashtags", zap.Error(err))
		return nil, err
	}

	// Convert to storage TrendingHashtag format
	hashtags := make([]*storage.TrendingHashtag, len(hashtagModels))
	for i, hashtag := range hashtagModels {
		hashtags[i] = &storage.TrendingHashtag{
			Name:        hashtag.Name,
			URL:         hashtag.URL,
			UsageCount:  hashtag.UsageCount,
			UniqueUsers: 0, // Not calculated in this query for performance
			LastUsed:    hashtag.LastUsed,
			FirstSeen:   hashtag.FirstSeen,
		}
	}

	return hashtags, nil
}

// GetRecentStatusesWithEngagement returns recent statuses with engagement since the given time
func (r *TrendingRepository) GetRecentStatusesWithEngagement(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	// Recent status engagements resolve through the GSI1 global listing
	// (ENGAGEMENTS#ALL / <EngagedAt RFC3339>#<statusID>#<userID>) maintained
	// by StatusEngagement.UpdateKeys (wave part 2 batch E, #1469); the since
	// window is a keyed sort-key range. Legacy engagement rows written before
	// the GSI1 shape carry no index keys and are not returned until next
	// written (engagements are TTL-transient, 7d).
	var engagements []models.StatusEngagement
	err := r.db.WithContext(ctx).Model(&models.StatusEngagement{}).
		Index("gsi1").
		Where("gsi1PK", "=", "ENGAGEMENTS#ALL").
		Where("gsi1SK", ">=", since.Format(time.RFC3339)).
		OrderBy("gsi1SK", "DESC").
		Limit(limit * 10). // Get more to aggregate by status
		All(&engagements)

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("no recent status engagements found", zap.Time("since", since))
			return []*storage.TrendingStatus{}, nil
		}
		r.logger.Error("failed to query recent status engagements", zap.Error(err))
		return nil, err
	}

	// Count engagements for each status
	statusEngagements := make(map[string]map[string]int64)
	for _, engagement := range engagements {
		statusID := engagement.StatusID
		if statusEngagements[statusID] == nil {
			statusEngagements[statusID] = map[string]int64{
				"like":  0,
				"boost": 0,
				"reply": 0,
			}
		}
		statusEngagements[statusID][engagement.EngagementType]++
	}

	// Convert to trending statuses
	statuses := make([]*storage.TrendingStatus, 0, len(statusEngagements))
	for statusID, counts := range statusEngagements {
		// Calculate engagement score: likes * 1 + boosts * 2 + replies * 3
		score := counts["like"] + (counts["boost"] * 2) + (counts["reply"] * 3)

		status := &storage.TrendingStatus{
			ID:          statusID,
			URL:         fmt.Sprintf("https://%s/statuses/%s", r.domain, statusID),
			AuthorID:    "", // Would need to query status to get this
			Content:     "", // Would need to query status to get this
			Engagements: score,
			PublishedAt: time.Now(), // Simplified - would get from actual status
		}
		statuses = append(statuses, status)
	}

	// Sort by engagement score and limit
	for i := 0; i < len(statuses)-1; i++ {
		for j := i + 1; j < len(statuses); j++ {
			if statuses[i].Engagements < statuses[j].Engagements {
				statuses[i], statuses[j] = statuses[j], statuses[i]
			}
		}
	}

	if limit > 0 && len(statuses) > limit {
		statuses = statuses[:limit]
	}

	return statuses, nil
}

// GetRecentLinks returns recent links since the given time (no trending calculation)
func (r *TrendingRepository) GetRecentLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	// Recent link shares resolve through the GSI1 global listing
	// (LINK_SHARES#ALL / <SharedAt RFC3339>#<url>#<statusID>) maintained by
	// LinkShare.UpdateKeys (wave part 2 batch E, #1469); the since window is a
	// keyed sort-key range. Legacy share rows written before the GSI1 shape
	// carry no index keys and are not returned until next written (shares are
	// TTL-transient, 7d).
	var shares []models.LinkShare
	err := r.db.WithContext(ctx).Model(&models.LinkShare{}).
		Index("gsi1").
		Where("gsi1PK", "=", "LINK_SHARES#ALL").
		Where("gsi1SK", ">=", since.Format(time.RFC3339)).
		OrderBy("gsi1SK", "DESC").
		Limit(limit * 5). // Get more to aggregate by URL
		All(&shares)

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("no recent link shares found", zap.Time("since", since))
			return []*storage.TrendingLink{}, nil
		}
		r.logger.Error("failed to query recent link shares", zap.Error(err))
		return nil, err
	}

	// Count shares for each URL
	shareCounts := make(map[string]int64)
	linkData := make(map[string]*models.LinkShare)

	for _, share := range shares {
		shareCounts[share.URL]++
		if linkData[share.URL] == nil {
			linkData[share.URL] = &share
		}
	}

	// Convert to trending links
	links := make([]*storage.TrendingLink, 0, len(shareCounts))
	for url, count := range shareCounts {
		// Extract basic link metadata
		title := extractDomainFromURL(url)
		description := ""
		image := ""
		linkType := "link"

		// Determine link type based on URL patterns
		lowerURL := strings.ToLower(url)
		if strings.Contains(lowerURL, "youtube.com") || strings.Contains(lowerURL, "youtu.be") {
			linkType = "video"
			title = "YouTube Video"
		} else if strings.Contains(lowerURL, ".jpg") || strings.Contains(lowerURL, ".png") ||
			strings.Contains(lowerURL, ".gif") || strings.Contains(lowerURL, ".webp") {
			linkType = "photo"
			title = "Image"
			image = url
		}

		link := &storage.TrendingLink{
			URL:         url,
			Title:       title,
			Description: description,
			Type:        linkType,
			AuthorName:  "", // Could extract from first sharer
			Image:       image,
			ShareCount:  count,
		}
		links = append(links, link)
	}

	// Sort by share count and limit
	for i := 0; i < len(links)-1; i++ {
		for j := i + 1; j < len(links); j++ {
			if links[i].ShareCount < links[j].ShareCount {
				links[i], links[j] = links[j], links[i]
			}
		}
	}

	if limit > 0 && len(links) > limit {
		links = links[:limit]
	}

	return links, nil
}

// StoreEngagementMetrics stores engagement metrics for a status
func (r *TrendingRepository) StoreEngagementMetrics(ctx context.Context, metrics *storage.EngagementMetrics) error {
	// Create DynamORM model from storage model
	model := &models.EngagementMetrics{
		PK:               fmt.Sprintf("STATUS#%s", metrics.StatusID),
		SK:               "ENGAGEMENT#METRICS",
		StatusID:         metrics.StatusID,
		LikeCount:        metrics.LikeCount,
		BoostCount:       metrics.BoostCount,
		ReplyCount:       metrics.ReplyCount,
		Score:            metrics.Score,
		EngagementBucket: metrics.EngagementBucket,
		TTL:              time.Now().Add(90 * 24 * time.Hour).Unix(), // 90 days TTL
	}

	err := r.db.WithContext(ctx).Model(model).CreateOrUpdate()
	if err != nil {
		r.logger.Error("failed to store engagement metrics",
			zap.String("statusID", metrics.StatusID),
			zap.Error(err))
		return err
	}

	return nil
}

// GetEngagementMetrics retrieves stored engagement metrics for a status
func (r *TrendingRepository) GetEngagementMetrics(ctx context.Context, statusID string) (*storage.EngagementMetrics, error) {
	var model models.EngagementMetrics
	err := r.db.WithContext(ctx).Model(&models.EngagementMetrics{}).
		Where("PK", "=", fmt.Sprintf("STATUS#%s", statusID)).
		Where("SK", "=", "ENGAGEMENT#METRICS").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("no engagement metrics found", zap.String("statusID", statusID))
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "engagement metrics", statusID)
		}
		r.logger.Error("failed to get engagement metrics",
			zap.String("statusID", statusID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "engagement metrics", statusID)
	}

	// Convert to storage model
	metrics := &storage.EngagementMetrics{
		StatusID:         model.StatusID,
		LikeCount:        model.LikeCount,
		BoostCount:       model.BoostCount,
		ReplyCount:       model.ReplyCount,
		Score:            model.Score,
		EngagementBucket: model.EngagementBucket,
	}

	return metrics, nil
}

// StoreHashtagTrend stores a hashtag trend record
func (r *TrendingRepository) StoreHashtagTrend(ctx context.Context, trend any) error {
	return r.storeHashtagTrendInternal(ctx, trend)
}

// StoreStatusTrend stores a status trend record
func (r *TrendingRepository) StoreStatusTrend(ctx context.Context, trend any) error {
	return r.storeStatusTrendInternal(ctx, trend)
}

// StoreLinkTrend stores a link trend record
func (r *TrendingRepository) StoreLinkTrend(ctx context.Context, trend any) error {
	return r.storeLinkTrendInternal(ctx, trend)
}

// TrendDeletable defines the interface for trend models that can be deleted
type TrendDeletable interface {
	GetIdentifier() string // Returns a string that identifies the trend for logging
}

// deleteOldTrendsGeneric is a generic function to delete old trend records
func (r *TrendingRepository) deleteOldTrendsGeneric(_ context.Context, before time.Time, trendType string, _ interface{}, _ func(interface{}) string) error {
	// IMPORTANT:
	// TableTheory's `.Scan(...)` issues a DynamoDB Scan, and using a non-key filter like
	// `UpdatedAt < before` can match and deserialize *any* item that has an `updatedAt` attribute
	// (user, actor, etc.). The previous implementation deleted those items by PK/SK, which is a
	// catastrophic data-loss risk.
	//
	// Trend models already write TTLs (`ttl = updatedAt + 7d`) in `pkg/storage/models/trends.go`,
	// so we rely on DynamoDB TTL for expiration and do not perform manual deletion here.
	if r.logger != nil {
		r.logger.Info("skipping manual trend cleanup (ttl handles expiration)",
			zap.String("trend_type", trendType),
			zap.Time("before", before),
		)
	}
	return nil
}

// DeleteOldHashtagTrends deletes hashtag trend records older than the specified time
func (r *TrendingRepository) DeleteOldHashtagTrends(ctx context.Context, before time.Time) error {
	return r.deleteOldTrendsGeneric(ctx, before, "hashtag", &models.HashtagTrend{}, func(trend interface{}) string {
		return trend.(*models.HashtagTrend).Name
	})
}

// DeleteOldLinkTrends deletes link trend records older than the specified time
func (r *TrendingRepository) DeleteOldLinkTrends(ctx context.Context, before time.Time) error {
	return r.deleteOldTrendsGeneric(ctx, before, "link", &models.LinkTrend{}, func(trend interface{}) string {
		return trend.(*models.LinkTrend).URL
	})
}

// DeleteOldStatusTrends deletes status trend records older than the specified time
func (r *TrendingRepository) DeleteOldStatusTrends(ctx context.Context, before time.Time) error {
	return r.deleteOldTrendsGeneric(ctx, before, "status", &models.StatusTrend{}, func(trend interface{}) string {
		return trend.(*models.StatusTrend).ID
	})
}

// TrackSearchQuery records a search query for analytics
func (r *TrendingRepository) TrackSearchQuery(ctx context.Context, userID, query string, resultCount int) error {
	// Normalize and validate query using centralized validation
	normalizedQuery, err := common.ValidateNormalizedQuery(query)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, "search query", "validation")
	}

	// Create search query record
	searchQuery := &models.SearchQuery{
		Query:       normalizedQuery,
		UserID:      userID,
		ResultCount: resultCount,
		SearchedAt:  time.Now(),
	}

	// Update keys
	_ = searchQuery.UpdateKeys() // Ignore error as this is internal model operation

	// Store the query
	err = r.db.WithContext(ctx).Model(searchQuery).Create()
	if err != nil {
		r.logger.Error("failed to track search query",
			zap.String("query", query),
			zap.String("userID", userID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "search query", query)
	}

	// Also update popular queries index
	r.updatePopularQueries(ctx, normalizedQuery)

	return nil
}

// GetPopularSearchQueries retrieves the most popular search queries
//
// NOTE (wave part 2 batch E rework, #1469): this stays on the baselined
// full-table scan over raw SearchQuery rows and is NOT delegated to the GSI8
// PopularQueryCounter path. GetTopQueries answers a different question than
// the caller's window: the counter's GSI8 partition key re-points on every
// increment (PopularQueryCounter.UpdateKeys sets GSI8PK = POPULAR#<bucket>#<date>
// from the Date of the last write, moving the item between partitions), so
// only today's partition is ever populated and a 7-day read would silently
// return just today's counters — no per-day partitions exist to aggregate
// across the window. The only source that answers the caller-visible 7-day
// window (scorePopularQueries, the sole caller) is the raw-row aggregation
// below, so the delegation was reverted; the site is dispositioned
// "elimination pending — wave #1469" with this semantic note (see
// docs/architecture/dynamodb-scan-inventory.md).
func (r *TrendingRepository) GetPopularSearchQueries(ctx context.Context, limit int, timeWindow time.Duration) ([]storage.SearchQueryStats, error) {
	// Calculate time cutoff
	cutoff := time.Now().Add(-timeWindow)

	// Query recent search queries
	var queries []models.SearchQuery
	err := r.db.WithContext(ctx).Model(&models.SearchQuery{}).
		Where("SearchedAt", ">=", cutoff).
		OrderBy("SearchedAt", "DESC").
		Limit(1000). // Get more to aggregate
		Scan(&queries)

	if err != nil {
		if errors.IsNotFound(err) {
			return []storage.SearchQueryStats{}, nil
		}
		r.logger.Error("failed to query search queries", zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "search query", "popular queries")
	}

	// Aggregate by query
	queryMap := make(map[string]*storage.SearchQueryStats)
	userMap := make(map[string]map[string]bool) // query -> set of users

	for _, q := range queries {
		if stats, exists := queryMap[q.Query]; exists {
			stats.Count++
			stats.AvgResults = (stats.AvgResults*float64(stats.Count-1) + float64(q.ResultCount)) / float64(stats.Count)
			if q.SearchedAt.After(stats.LastUsed) {
				stats.LastUsed = q.SearchedAt
			}
		} else {
			queryMap[q.Query] = &storage.SearchQueryStats{
				Query:      q.Query,
				Count:      1,
				AvgResults: float64(q.ResultCount),
				LastUsed:   q.SearchedAt,
			}
			userMap[q.Query] = make(map[string]bool)
		}

		// Track unique users
		userMap[q.Query][q.UserID] = true
	}

	// Calculate unique user counts and convert to slice
	results := make([]storage.SearchQueryStats, 0, len(queryMap))
	for query, stats := range queryMap {
		stats.UserCount = len(userMap[query])
		results = append(results, *stats)
	}

	// Sort by count (most popular first)
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Count < results[j].Count {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Apply limit
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// GetUserSearchHistory retrieves a user's search history
func (r *TrendingRepository) GetUserSearchHistory(ctx context.Context, userID string, limit int) ([]storage.SearchHistoryEntry, error) {
	var queries []models.SearchQuery

	err := r.db.WithContext(ctx).Model(&models.SearchQuery{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", userID)).
		Filter("SK", "BEGINS_WITH", "SEARCH#").
		OrderBy("SK", "DESC"). // Most recent first
		Limit(limit).
		Scan(&queries)

	if err != nil {
		if errors.IsNotFound(err) {
			return []storage.SearchHistoryEntry{}, nil
		}
		r.logger.Error("failed to query user search history",
			zap.String("userID", userID),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "search query", "user history")
	}

	// Convert to storage type
	entries := make([]storage.SearchHistoryEntry, len(queries))
	for i, q := range queries {
		entries[i] = storage.SearchHistoryEntry{
			UserID:      q.UserID,
			Query:       q.Query,
			ResultCount: q.ResultCount,
			SearchedAt:  q.SearchedAt,
		}
	}

	return entries, nil
}

// updatePopularQueries increments the count for a search query using atomic operations
func (r *TrendingRepository) updatePopularQueries(ctx context.Context, query string) {
	// Implement atomic counter updates as requested in the audit
	if err := r.IncrementQueryCount(ctx, query, 1); err != nil {
		r.logger.Error("failed to increment popular query counter",
			zap.String("query", query),
			zap.Error(err))
	}
}

// GetStatusesByLink retrieves statuses that contain a specific link
func (r *TrendingRepository) GetStatusesByLink(ctx context.Context, linkURL string, limit int) ([]any, error) {
	// Implement limit validation
	if limit <= 0 {
		limit = 20 // Default limit
	}
	if limit > 100 {
		limit = 100 // Maximum limit to prevent excessive resource usage
	}

	// Check if statusRepo dependency is available
	if r.statusRepo == nil {
		r.logger.Error("statusRepo dependency not set for GetStatusesByLink")
		return nil, ErrStatusRepoDependencyMissing
	}

	// Cast statusRepo to the proper interface
	statusRepo, ok := r.statusRepo.(interfaces.StatusRepository)
	if !ok {
		r.logger.Error("statusRepo does not implement StatusRepository interface")
		return nil, fmt.Errorf("invalid status repository type")
	}

	// Use StatusRepository to search for statuses that contain the link
	// We'll use the search functionality to find statuses containing the URL
	opts := interfaces.PaginationOptions{
		Limit: limit,
	}

	statuses, err := statusRepo.SearchStatuses(ctx, linkURL, opts)
	if err != nil {
		r.logger.Error("failed to get statuses by link",
			zap.String("link_url", linkURL),
			zap.Int("limit", limit),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get statuses by link: %w", err)
	}

	// Filter results to publicly visible statuses that actually contain the exact link.
	// The link timeline is unauthenticated, so private/direct/unlisted/deleted statuses
	// must not be exposed even when legacy status search rows still match the URL.
	matchingStatuses := make([]*storage.TrendingStatus, 0, len(statuses.Items))
	for _, status := range statuses.Items {
		if !publicStatusContainsLink(status, linkURL) {
			continue
		}
		matchingStatuses = append(matchingStatuses, trendingStatusFromStatus(status))
	}

	// Convert status objects to any slice for interface compatibility
	results := make([]any, len(matchingStatuses))
	for i, status := range matchingStatuses {
		results[i] = status
	}

	r.logger.Info("successfully retrieved statuses by link",
		zap.String("link_url", linkURL),
		zap.Int("limit", limit),
		zap.Int("count", len(results)))

	return results, nil
}

func publicStatusContainsLink(status *models.Status, linkURL string) bool {
	if status == nil || status.Deleted || status.Visibility != models.VisibilityPublic {
		return false
	}

	normalizedLink := strings.TrimSpace(linkURL)
	if normalizedLink == "" {
		return false
	}

	for _, candidate := range status.URLs {
		if strings.EqualFold(strings.TrimSpace(candidate), normalizedLink) {
			return true
		}
	}

	return strings.Contains(status.Content, normalizedLink)
}

func trendingStatusFromStatus(status *models.Status) *storage.TrendingStatus {
	if status == nil {
		return nil
	}

	statusURL := ""
	if status.Note != nil {
		statusURL = strings.TrimSpace(status.Note.ID)
	}

	return &storage.TrendingStatus{
		ID:              status.StatusID,
		StatusID:        status.StatusID,
		AuthorID:        status.AuthorID,
		Content:         status.Content,
		URL:             statusURL,
		ReblogsCount:    status.ReblogCount,
		FavouritesCount: status.LikeCount,
		RepliesCount:    status.ReplyCount,
		Engagements:     int64(status.LikeCount + status.ReblogCount + status.ReplyCount),
		PublishedAt:     status.PublishedAt,
		UpdatedAt:       status.UpdatedAt,
		CreatedAt:       status.CreatedAt,
		Likes:           status.LikeCount,
		Boosts:          status.ReblogCount,
		Replies:         status.ReplyCount,
	}
}

// IndexByEngagement creates an index entry for engagement-based discovery
func (r *TrendingRepository) IndexByEngagement(ctx context.Context, statusID string, bucket string) error {
	timestamp := time.Now().Unix()
	ttl := time.Now().Add(90 * 24 * time.Hour).Unix() // 90 days TTL

	// Create an engagement index entry
	// This allows for efficient retrieval of statuses by engagement level
	engagement := &models.EngagementMetrics{
		PK:               fmt.Sprintf("ENGAGEMENT#%s", bucket),
		SK:               fmt.Sprintf("STATUS#%d#%s", timestamp, statusID),
		GSI8PK:           "ENGAGEMENT_INDEX",
		GSI8SK:           fmt.Sprintf("%s#%d#%s", bucket, timestamp, statusID),
		StatusID:         statusID,
		EngagementBucket: bucket,
		TTL:              ttl,
	}

	err := r.db.WithContext(ctx).Model(engagement).Create()
	if err != nil {
		r.logger.Error("failed to index by engagement",
			zap.String("statusID", statusID),
			zap.String("bucket", bucket),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrFailedIndexByEngagement, err)
	}

	r.logger.Debug("indexed status by engagement",
		zap.String("statusID", statusID),
		zap.String("bucket", bucket))

	return nil
}

// GenerateSearchSuggestions generates search suggestions based on user history and popular queries
func (r *TrendingRepository) GenerateSearchSuggestions(ctx context.Context, userID, partialQuery string, limit int) ([]string, error) {
	normalizedQuery := strings.ToLower(strings.TrimSpace(partialQuery))
	if err := common.ValidateRequiredParam("normalizedQuery", normalizedQuery); err != nil {
		return []string{}, nil
	}

	suggestions := make(map[string]float64) // suggestion -> score

	// Score suggestions from different sources
	r.scoreUserHistory(ctx, userID, normalizedQuery, suggestions)
	r.scorePopularQueries(ctx, normalizedQuery, suggestions)
	r.scoreHashtagSuggestions(ctx, normalizedQuery, suggestions)

	// Sort and return top suggestions
	return r.extractTopSuggestions(suggestions, limit), nil
}

// scoreUserHistory scores suggestions from user's search history
func (r *TrendingRepository) scoreUserHistory(ctx context.Context, userID, normalizedQuery string, suggestions map[string]float64) {
	userHistory, err := r.GetUserSearchHistory(ctx, userID, 50)
	if err != nil {
		return
	}

	for _, entry := range userHistory {
		if !strings.HasPrefix(strings.ToLower(entry.Query), normalizedQuery) {
			continue
		}

		score := r.calculateUserHistoryScore(entry.SearchedAt)
		suggestions[entry.Query] += score
	}
}

// calculateUserHistoryScore calculates score based on recency
func (r *TrendingRepository) calculateUserHistoryScore(searchedAt time.Time) float64 {
	age := time.Since(searchedAt)
	recencyScore := 1.0 / (1 + age.Hours()/24) // Decay over days
	return recencyScore * 2                    // User history weighted 2x
}

// scorePopularQueries scores suggestions from popular queries
func (r *TrendingRepository) scorePopularQueries(ctx context.Context, normalizedQuery string, suggestions map[string]float64) {
	popularQueries, err := r.GetPopularSearchQueries(ctx, 100, 7*24*time.Hour) // Last 7 days
	if err != nil {
		return
	}

	for _, stat := range popularQueries {
		if !strings.HasPrefix(strings.ToLower(stat.Query), normalizedQuery) {
			continue
		}

		score := r.calculatePopularQueryScore(stat.LastUsed, stat.Count)
		suggestions[stat.Query] += score
	}
}

// calculatePopularQueryScore calculates score based on popularity and recency
func (r *TrendingRepository) calculatePopularQueryScore(lastUsed time.Time, count int) float64 {
	age := time.Since(lastUsed)
	recencyScore := 1.0 / (1 + age.Hours()/168)   // Decay over weeks
	popularityScore := math.Log1p(float64(count)) // Logarithmic scaling
	return recencyScore * popularityScore
}

// scoreHashtagSuggestions scores suggestions from trending hashtags
func (r *TrendingRepository) scoreHashtagSuggestions(ctx context.Context, normalizedQuery string, suggestions map[string]float64) {
	if !r.shouldIncludeHashtags(normalizedQuery) {
		return
	}

	hashtags, err := r.GetRecentHashtags(ctx, time.Now().Add(-7*24*time.Hour), 20)
	if err != nil {
		return
	}

	for _, hashtag := range hashtags {
		if !r.hashtagMatchesQuery(hashtag.Name, normalizedQuery) {
			continue
		}

		suggestion := r.formatHashtagSuggestion(hashtag.Name)
		score := float64(hashtag.UsageCount) / 100
		suggestions[suggestion] += score
	}
}

// shouldIncludeHashtags checks if hashtag suggestions should be included
func (r *TrendingRepository) shouldIncludeHashtags(normalizedQuery string) bool {
	return strings.HasPrefix(normalizedQuery, "#") || len(normalizedQuery) >= 2
}

// hashtagMatchesQuery checks if a hashtag matches the query
func (r *TrendingRepository) hashtagMatchesQuery(hashtagName, normalizedQuery string) bool {
	tagName := strings.ToLower(hashtagName)
	return strings.Contains(tagName, normalizedQuery) || strings.Contains(normalizedQuery, "#")
}

// formatHashtagSuggestion formats a hashtag for suggestion
func (r *TrendingRepository) formatHashtagSuggestion(hashtagName string) string {
	if !strings.HasPrefix(hashtagName, "#") {
		return "#" + hashtagName
	}
	return hashtagName
}

// extractTopSuggestions extracts the top suggestions from scored map
func (r *TrendingRepository) extractTopSuggestions(suggestions map[string]float64, limit int) []string {
	// Convert to slice
	type scoredSuggestion struct {
		query string
		score float64
	}

	scoredList := make([]scoredSuggestion, 0, len(suggestions))
	for query, score := range suggestions {
		scoredList = append(scoredList, scoredSuggestion{query, score})
	}

	// Sort by score descending
	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].score > scoredList[j].score
	})

	// Extract top suggestions
	result := make([]string, 0, limit)
	for i := 0; i < len(scoredList) && i < limit; i++ {
		result = append(result, scoredList[i].query)
	}

	return result
}

// ========== EngagementMetrics Methods ==========

// RecordEngagement records engagement metrics for content
func (r *TrendingRepository) RecordEngagement(ctx context.Context, metricType, targetID, date string, engagement *storage.EngagementData) error {
	now := time.Now()

	// Create metrics record with exact key pattern from legacy
	metrics := &models.EngagementMetrics{
		PK:          fmt.Sprintf("METRICS#%s#%s", metricType, date),
		SK:          fmt.Sprintf("target#%s", targetID),
		MetricType:  metricType,
		TargetID:    targetID,
		Date:        date,
		Views:       engagement.Views,
		Likes:       engagement.Likes,
		Shares:      engagement.Shares,
		Replies:     engagement.Replies,
		UniqueUsers: engagement.UniqueUsers,
		UpdatedAt:   now,
		TTL:         now.Add(90 * 24 * time.Hour).Unix(), // 90 days retention
	}

	_ = metrics.UpdateKeys() // Ignore error as this is internal model operation

	err := r.db.WithContext(ctx).Model(metrics).CreateOrUpdate()
	if err != nil {
		r.logger.Error("failed to record engagement",
			zap.String("metricType", metricType),
			zap.String("targetID", targetID),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrFailedRecordEngagement, err)
	}

	return nil
}

// GetEngagementMetricsData retrieves engagement metrics for a specific target
func (r *TrendingRepository) GetEngagementMetricsData(ctx context.Context, metricType, targetID, date string) (*storage.EngagementData, error) {
	pk := fmt.Sprintf("METRICS#%s#%s", metricType, date)
	sk := fmt.Sprintf("target#%s", targetID)

	var metrics models.EngagementMetrics
	err := r.db.WithContext(ctx).Model(&models.EngagementMetrics{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&metrics)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "engagement metrics", fmt.Sprintf("%s#%s", metricType, targetID))
		}
		r.logger.Error("failed to get engagement metrics",
			zap.String("metricType", metricType),
			zap.String("targetID", targetID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "engagement metrics", fmt.Sprintf("%s#%s", metricType, targetID))
	}

	return &storage.EngagementData{
		Views:       metrics.Views,
		Likes:       metrics.Likes,
		Shares:      metrics.Shares,
		Replies:     metrics.Replies,
		UniqueUsers: metrics.UniqueUsers,
	}, nil
}

// GetEngagementByDateRange retrieves engagement metrics within a date range
func (r *TrendingRepository) GetEngagementByDateRange(ctx context.Context, metricType string, startDate, endDate string, limit int) ([]*storage.EngagementMetricsSummary, error) {
	start, err := time.Parse(common.DateFormat, startDate)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid startDate %q", ErrFailedGetEngagementByDate, startDate)
	}
	end, err := time.Parse(common.DateFormat, endDate)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid endDate %q", ErrFailedGetEngagementByDate, endDate)
	}
	if end.Before(start) {
		return nil, fmt.Errorf("%w: endDate must be on/after startDate", ErrFailedGetEngagementByDate)
	}

	summaries := make([]*storage.EngagementMetricsSummary, 0)
	for current := start; !current.After(end); current = current.AddDate(0, 0, 1) {
		pk := fmt.Sprintf("METRICS#%s#%s", metricType, current.Format(common.DateFormat))

		var records []models.EngagementMetrics
		query := r.db.WithContext(ctx).Model(&models.EngagementMetrics{}).
			Where("PK", "=", pk).
			OrderBy("SK", "ASC")

		if limit > 0 {
			remaining := limit - len(summaries)
			if remaining <= 0 {
				break
			}
			query = query.Limit(remaining)
		}

		if err := query.All(&records); err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			r.logger.Error("failed to get engagement by date range",
				zap.String("metricType", metricType),
				zap.String("startDate", startDate),
				zap.String("endDate", endDate),
				zap.Error(err))
			return nil, fmt.Errorf("%w: %w", ErrFailedGetEngagementByDate, err)
		}

		for _, record := range records {
			summaries = append(summaries, &storage.EngagementMetricsSummary{
				Date:        record.Date,
				MetricType:  record.MetricType,
				TargetID:    record.TargetID,
				TotalViews:  record.Views,
				TotalLikes:  record.Likes,
				TotalShares: record.Shares,
				UniqueUsers: record.UniqueUsers,
			})

			if limit > 0 && len(summaries) >= limit {
				break
			}
		}

		if limit > 0 && len(summaries) >= limit {
			break
		}
	}

	return summaries, nil
}

// GetTopEngagedContent retrieves the most engaged content
func (r *TrendingRepository) GetTopEngagedContent(ctx context.Context, metricType string, date string, limit int) ([]*storage.EngagementRanking, error) {
	pk := fmt.Sprintf("METRICS#%s#%s", metricType, date)

	var metricsRecords []models.EngagementMetrics
	err := r.db.WithContext(ctx).Model(&models.EngagementMetrics{}).
		Where("PK", "=", pk).
		Limit(limit * 2). // Get more to sort
		All(&metricsRecords)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.EngagementRanking{}, nil
		}
		r.logger.Error("failed to get top engaged content",
			zap.String("metricType", metricType),
			zap.String("date", date),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %w", ErrFailedGetTopContent, err)
	}

	// Calculate engagement scores and rank
	rankings := make([]*storage.EngagementRanking, 0, len(metricsRecords))
	for _, record := range metricsRecords {
		// Engagement score: views + likes*2 + shares*3 + replies*4
		score := float64(record.Views) +
			float64(record.Likes)*2 +
			float64(record.Shares)*3 +
			float64(record.Replies)*4

		ranking := &storage.EngagementRanking{
			TargetID:    record.TargetID,
			Score:       score,
			Views:       record.Views,
			Likes:       record.Likes,
			Shares:      record.Shares,
			Replies:     record.Replies,
			UniqueUsers: record.UniqueUsers,
		}
		rankings = append(rankings, ranking)
	}

	// Sort by score descending
	for i := 0; i < len(rankings)-1; i++ {
		for j := i + 1; j < len(rankings); j++ {
			if rankings[i].Score < rankings[j].Score {
				rankings[i], rankings[j] = rankings[j], rankings[i]
			}
		}
	}

	// Apply limit
	if limit > 0 && len(rankings) > limit {
		rankings = rankings[:limit]
	}

	return rankings, nil
}

// AggregateEngagementMetrics aggregates metrics across multiple dates
func (r *TrendingRepository) AggregateEngagementMetrics(ctx context.Context, metricType string, dates []string) (*storage.AggregatedEngagement, error) {
	aggregated := &storage.AggregatedEngagement{
		MetricType:  metricType,
		DateRange:   fmt.Sprintf("%s to %s", dates[0], dates[len(dates)-1]),
		TotalViews:  0,
		TotalLikes:  0,
		TotalShares: 0,
		UniqueUsers: make(map[string]bool),
	}

	for _, date := range dates {
		pk := fmt.Sprintf("METRICS#%s#%s", metricType, date)

		var metricsRecords []models.EngagementMetrics
		err := r.db.WithContext(ctx).Model(&models.EngagementMetrics{}).
			Where("PK", "=", pk).
			All(&metricsRecords)

		if err != nil && !errors.IsNotFound(err) {
			r.logger.Error("failed to aggregate engagement metrics",
				zap.String("metricType", metricType),
				zap.String("date", date),
				zap.Error(err))
			continue // Skip this date but continue aggregating
		}

		for _, record := range metricsRecords {
			aggregated.TotalViews += record.Views
			aggregated.TotalLikes += record.Likes
			aggregated.TotalShares += record.Shares
			// Track unique users across all dates
			for i := int64(0); i < record.UniqueUsers; i++ {
				aggregated.UniqueUsers[fmt.Sprintf("%s_%d", record.TargetID, i)] = true
			}
		}
	}

	aggregated.TotalUniqueUsers = int64(len(aggregated.UniqueUsers))
	aggregated.UniqueUsers = nil // Clear the map to save memory

	return aggregated, nil
}

// ========== TrendingHashtag Methods ==========

// UpdateTrendingHashtag updates or creates a trending hashtag entry
func (r *TrendingRepository) UpdateTrendingHashtag(ctx context.Context, hashtag string, date string, useCount, userCount int64) error {
	now := time.Now()

	// Calculate trend score
	score := float64(useCount) * (1 + float64(userCount)/float64(useCount+1))
	paddedScore := fmt.Sprintf("%010.0f", score*1000) // Pad for proper sorting

	trending := &models.TrendingHashtag{
		PK:        fmt.Sprintf("TRENDING#%s", date),
		SK:        fmt.Sprintf("HASHTAG#%s#%s", paddedScore, hashtag),
		Hashtag:   hashtag,
		Date:      date,
		Score:     score,
		UseCount:  useCount,
		UserCount: userCount,
		History:   []float64{score}, // Initialize with current score
		UpdatedAt: now,
		TTL:       now.Add(30 * 24 * time.Hour).Unix(), // 30 days retention
	}

	// Use enhanced repository for validation and creation
	err := r.ValidateAndCreateOrUpdate(ctx, trending)
	if err != nil {
		r.logger.Error("failed to update trending hashtag",
			zap.String("hashtag", hashtag),
			zap.String("date", date),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrFailedUpdateTrendingTag, err)
	}

	return nil
}

// GetTrendingHashtagsForDate retrieves trending hashtags for a specific date
func (r *TrendingRepository) GetTrendingHashtagsForDate(ctx context.Context, date string, limit int) ([]*storage.TrendingHashtagData, error) {
	pk := fmt.Sprintf("TRENDING#%s", date)

	trendingRecords, err := r.Query(ctx, pk, limit)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.TrendingHashtagData{}, nil
		}
		r.logger.Error("failed to get trending hashtags",
			zap.String("date", date),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %w", ErrFailedGetTrendingTags, err)
	}

	// Convert to storage format
	trending := make([]*storage.TrendingHashtagData, len(trendingRecords))
	for i, record := range trendingRecords {
		trending[i] = &storage.TrendingHashtagData{
			Hashtag:   record.Hashtag,
			Score:     record.Score,
			UseCount:  record.UseCount,
			UserCount: record.UserCount,
			History:   record.History,
			UpdatedAt: record.UpdatedAt,
		}
	}

	return trending, nil
}

// GetHashtagTrend retrieves the trend history for a specific hashtag
func (r *TrendingRepository) GetHashtagTrend(ctx context.Context, hashtag string, days int) (*storage.HashtagTrendHistory, error) {
	history := &storage.HashtagTrendHistory{
		Hashtag: hashtag,
		Days:    make([]storage.DailyTrend, 0, days),
	}

	// Query for the last N days
	now := time.Now()
	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -i).Format(common.DateFormat)
		pk := fmt.Sprintf("TRENDING#%s", date)

		// For hashtag filtering, use QueryWithFilter
		filters := map[string]interface{}{
			"SK": hashtag, // CONTAINS filter
		}
		trendingRecords, err := r.QueryWithFilter(ctx, pk, filters, 100)

		if err != nil && !errors.IsNotFound(err) {
			r.logger.Warn("failed to get hashtag trend for date",
				zap.String("hashtag", hashtag),
				zap.String("date", date),
				zap.Error(err))
			continue
		}

		// Find the specific hashtag
		for _, record := range trendingRecords {
			if record.Hashtag == hashtag {
				history.Days = append(history.Days, storage.DailyTrend{
					Date:      date,
					Score:     record.Score,
					UseCount:  record.UseCount,
					UserCount: record.UserCount,
				})
				break
			}
		}
	}

	// Reverse to have oldest first
	for i, j := 0, len(history.Days)-1; i < j; i, j = i+1, j-1 {
		history.Days[i], history.Days[j] = history.Days[j], history.Days[i]
	}

	return history, nil
}

// PruneStaleTrends removes old trending entries
func (r *TrendingRepository) PruneStaleTrends(_ context.Context, before time.Time) error {
	// Trending entries are TTL-driven (`ttl` on the item, `ttl` configured on the table). Manual
	// cleanup required a scan on a non-key attribute (`Date < ...`), which is expensive and
	// unnecessary.
	if r.logger != nil {
		r.logger.Info("skipping manual stale trend cleanup (ttl handles expiration)",
			zap.Time("before", before),
		)
	}
	return nil
}

// ========== InstanceMetrics Methods ==========

// RecordInstanceMetric records a platform-wide metric
func (r *TrendingRepository) RecordInstanceMetric(ctx context.Context, date, metricType string, value int64) error {
	now := time.Now()

	// Get previous value to calculate delta
	var previousValue int64
	yesterday := now.AddDate(0, 0, -1).Format(common.DateFormat)
	prev, err := r.GetInstanceMetrics(ctx, yesterday, metricType)
	if err == nil && prev != nil {
		previousValue = prev.Value
	}

	metrics := &models.InstanceMetrics{
		PK:         fmt.Sprintf("INSTANCE_METRICS#%s", date),
		SK:         fmt.Sprintf("METRIC#%s", metricType),
		Date:       date,
		MetricType: metricType,
		Value:      value,
		Delta:      value - previousValue,
		UpdatedAt:  now,
		TTL:        now.Add(365 * 24 * time.Hour).Unix(), // 1 year retention
	}

	_ = metrics.UpdateKeys() // Ignore error as this is internal model operation

	err = r.db.WithContext(ctx).Model(metrics).Create()
	if err == nil {
		return nil
	}

	if errors.IsConditionFailed(err) || strings.Contains(strings.ToLower(err.Error()), "conditionalcheckfailed") || strings.Contains(strings.ToLower(err.Error()), "same key already exists") {
		// Record already exists for this day; increment the value instead of failing
		current, getErr := r.GetInstanceMetrics(ctx, date, metricType)
		if getErr != nil {
			r.logger.Error("failed to load existing instance metric after conditional check",
				zap.String("metricType", metricType),
				zap.String("date", date),
				zap.Error(getErr))
			return fmt.Errorf("%w: %w", ErrFailedRecordInstanceMetric, getErr)
		}

		newValue := current.Value + value
		newDelta := newValue - previousValue

		updateBuilder := r.db.WithContext(ctx).
			Model(&models.InstanceMetrics{}).
			Where("PK", "=", metrics.PK).
			Where("SK", "=", metrics.SK).
			UpdateBuilder()

		updateBuilder.
			Set("Value", newValue).
			Set("Delta", newDelta).
			Set("UpdatedAt", now).
			Set("TTL", metrics.TTL)

		if updateErr := updateBuilder.Execute(); updateErr != nil {
			r.logger.Error("failed to update existing instance metric",
				zap.String("metricType", metricType),
				zap.String("date", date),
				zap.Int64("value", newValue),
				zap.Error(updateErr))
			return fmt.Errorf("%w: %w", ErrFailedRecordInstanceMetric, updateErr)
		}

		return nil
	}

	r.logger.Error("failed to record instance metric",
		zap.String("metricType", metricType),
		zap.String("date", date),
		zap.Int64("value", value),
		zap.Error(err))
	return fmt.Errorf("%w: %w", ErrFailedRecordInstanceMetric, err)
}

// GetInstanceMetrics retrieves instance metrics for a specific date and type
func (r *TrendingRepository) GetInstanceMetrics(ctx context.Context, date, metricType string) (*storage.InstanceMetricData, error) {
	pk := fmt.Sprintf("INSTANCE_METRICS#%s", date)
	sk := fmt.Sprintf("METRIC#%s", metricType)

	var metrics models.InstanceMetrics
	err := r.db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		ConsistentRead().
		First(&metrics)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "instance metrics", fmt.Sprintf("%s#%s", date, metricType))
		}
		r.logger.Error("failed to get instance metrics",
			zap.String("date", date),
			zap.String("metricType", metricType),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "instance metrics", fmt.Sprintf("%s#%s", date, metricType))
	}

	return &storage.InstanceMetricData{
		Date:       metrics.Date,
		MetricType: metrics.MetricType,
		Value:      metrics.Value,
		Delta:      metrics.Delta,
		UpdatedAt:  metrics.UpdatedAt,
	}, nil
}

// GetMetricHistory retrieves the history of a specific metric type
func (r *TrendingRepository) GetMetricHistory(ctx context.Context, metricType string, days int) ([]*storage.MetricHistoryPoint, error) {
	history := make([]*storage.MetricHistoryPoint, 0, days)
	now := time.Now()

	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -i).Format(common.DateFormat)
		metric, err := r.GetInstanceMetrics(ctx, date, metricType)

		if err != nil {
			r.logger.Warn("failed to get metric for date",
				zap.String("date", date),
				zap.String("metricType", metricType),
				zap.Error(err))
			continue
		}

		if metric != nil {
			history = append(history, &storage.MetricHistoryPoint{
				Date:  date,
				Value: metric.Value,
				Delta: metric.Delta,
			})
		}
	}

	// Reverse to have oldest first
	for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
		history[i], history[j] = history[j], history[i]
	}

	return history, nil
}

// CalculateGrowthRate calculates growth rate between two dates
func (r *TrendingRepository) CalculateGrowthRate(ctx context.Context, metricType, startDate, endDate string) (*storage.GrowthRate, error) {
	startMetric, err := r.GetInstanceMetrics(ctx, startDate, metricType)
	if err != nil || startMetric == nil {
		return nil, fmt.Errorf("%w: %w", ErrFailedGetStartMetric, err)
	}

	endMetric, err := r.GetInstanceMetrics(ctx, endDate, metricType)
	if err != nil || endMetric == nil {
		return nil, fmt.Errorf("%w: %w", ErrFailedGetEndMetric, err)
	}

	// Calculate growth rate
	var growthRate float64
	if startMetric.Value > 0 {
		growthRate = float64(endMetric.Value-startMetric.Value) / float64(startMetric.Value) * 100
	}

	return &storage.GrowthRate{
		MetricType:     metricType,
		StartDate:      startDate,
		EndDate:        endDate,
		StartValue:     startMetric.Value,
		EndValue:       endMetric.Value,
		GrowthRate:     growthRate,
		AbsoluteChange: endMetric.Value - startMetric.Value,
	}, nil
}

// ========== MediaAnalytics Methods ==========

// RecordManifestGeneration records when a media manifest is generated
func (r *TrendingRepository) RecordManifestGeneration(ctx context.Context, mediaID, format string, duration float64) error {
	analytics := &models.MediaAnalytics{}
	analytics.SetManifestGeneration(mediaID, format, duration)

	err := r.db.WithContext(ctx).Model(analytics).Create()
	if err != nil {
		r.logger.Error("failed to record manifest generation",
			zap.String("mediaID", mediaID),
			zap.String("format", format),
			zap.Float64("duration", duration),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrFailedRecordManifest, err)
	}

	r.logger.Debug("recorded manifest generation",
		zap.String("mediaID", mediaID),
		zap.String("format", format),
		zap.Float64("duration", duration))

	return nil
}

// RecordQualityChange records when a user changes video quality
func (r *TrendingRepository) RecordQualityChange(ctx context.Context, mediaID, userID, oldQuality, newQuality string) error {
	analytics := &models.MediaAnalytics{}
	analytics.SetQualityChange(mediaID, userID, oldQuality, newQuality)

	err := r.db.WithContext(ctx).Model(analytics).Create()
	if err != nil {
		r.logger.Error("failed to record quality change",
			zap.String("mediaID", mediaID),
			zap.String("userID", userID),
			zap.String("oldQuality", oldQuality),
			zap.String("newQuality", newQuality),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrFailedRecordQualityChange, err)
	}

	return nil
}

// RecordMediaEvent records general media streaming events
func (r *TrendingRepository) RecordMediaEvent(ctx context.Context, eventType, mediaID, userID string) error {
	analytics := &models.MediaAnalytics{}
	analytics.SetGeneralEvent(eventType, mediaID, userID)

	err := r.db.WithContext(ctx).Model(analytics).Create()
	if err != nil {
		r.logger.Error("failed to record media event",
			zap.String("eventType", eventType),
			zap.String("mediaID", mediaID),
			zap.String("userID", userID),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrFailedRecordMediaEvent, err)
	}

	return nil
}

// getMediaAnalyticsStatsGeneric is a generic function to retrieve media analytics statistics
func (r *TrendingRepository) getMediaAnalyticsStatsGeneric(ctx context.Context, pkPrefix, identifier, startDate, endDate string) (map[string]int64, error) {
	stats := make(map[string]int64)

	// Query media analytics records
	var analytics []models.MediaAnalytics
	err := r.db.WithContext(ctx).Model(&models.MediaAnalytics{}).
		Where("PK", "=", fmt.Sprintf("%s#%s", pkPrefix, identifier)).
		Where("Date", ">=", startDate).
		Where("Date", "<=", endDate).
		All(&analytics)

	if err != nil {
		if errors.IsNotFound(err) {
			return stats, nil
		}
		r.logger.Error(fmt.Sprintf("failed to get %s stats", strings.ToLower(pkPrefix)),
			zap.String("identifier", identifier),
			zap.String("startDate", startDate),
			zap.String("endDate", endDate),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %s: %w", ErrFailedGetStats, strings.ToLower(pkPrefix), err)
	}

	// Count by date
	for _, record := range analytics {
		stats[record.Date]++
	}

	return stats, nil
}

// GetManifestGenerationStats retrieves manifest generation statistics for a date range
func (r *TrendingRepository) GetManifestGenerationStats(ctx context.Context, format, startDate, endDate string) (map[string]int64, error) {
	return r.getMediaAnalyticsStatsGeneric(ctx, "MANIFEST", format, startDate, endDate)
}

// GetMediaEventStats retrieves general media event statistics
func (r *TrendingRepository) GetMediaEventStats(ctx context.Context, eventType, startDate, endDate string) (map[string]int64, error) {
	return r.getMediaAnalyticsStatsGeneric(ctx, "MEDIA_EVENT", eventType, startDate, endDate)
}

// initializeAnalyticsData creates and initializes streaming analytics data structure
func (r *TrendingRepository) initializeAnalyticsData(mediaID string) *storage.StreamingAnalyticsData {
	return &storage.StreamingAnalyticsData{
		MediaID:             mediaID,
		TotalViews:          0,
		UniqueViewers:       0,
		AverageWatchTime:    0,
		QualityDistribution: make(map[string]*storage.QualityStats),
		BufferingEvents:     0,
		CompletionRate:      0.0,
		RecentMetrics:       make(map[string]interface{}),
	}
}

// querySessionEvents retrieves session start events for analysis
func (r *TrendingRepository) querySessionEvents(ctx context.Context, last7Days time.Time) ([]models.MediaAnalytics, error) {
	var sessionEvents []models.MediaAnalytics
	err := r.db.WithContext(ctx).Model(&models.MediaAnalytics{}).
		Where("PK", "=", "MEDIA_EVENT#session_start").
		Where("SK", "begins_with", fmt.Sprintf("%d", last7Days.Unix())).
		All(&sessionEvents)

	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("%w: %w", ErrFailedQuerySessionEvents, err)
	}

	return sessionEvents, nil
}

// processSessionEvent processes a single session event for the target media
func (r *TrendingRepository) processSessionEvent(
	event models.MediaAnalytics,
	mediaID string,
	analytics *storage.StreamingAnalyticsData,
	uniqueUsers map[string]bool,
	sessionMetrics *struct {
		totalWatchTime         float64
		totalCompletedSessions int
	},
) {
	if event.MediaID != mediaID {
		return
	}

	analytics.TotalViews++

	// Track unique viewers
	if event.UserID != "" {
		uniqueUsers[event.UserID] = true
	}

	// Aggregate streaming session metrics
	analytics.StreamingSessions += event.StreamingSessions

	// Calculate watch time from duration
	if event.Duration > 0 {
		sessionMetrics.totalWatchTime += event.Duration
		// If session has duration, consider it completed
		sessionMetrics.totalCompletedSessions++
	}

	// Process quality distribution
	r.updateQualityDistribution(event, analytics)

	// Track bandwidth usage by variant
	for _, bandwidth := range event.VariantBandwidth {
		analytics.TotalBandwidthBytes += bandwidth
	}
}

// updateQualityDistribution updates quality distribution data from event
func (r *TrendingRepository) updateQualityDistribution(event models.MediaAnalytics, analytics *storage.StreamingAnalyticsData) {
	for qualityLevel, viewerCount := range event.QualityDistribution {
		if existing, ok := analytics.QualityDistribution[qualityLevel]; ok {
			existing.ViewCount += viewerCount
			existing.TotalBandwidth += event.VariantBandwidth[qualityLevel]
		} else {
			analytics.QualityDistribution[qualityLevel] = &storage.QualityStats{
				Quality:        qualityLevel,
				ViewCount:      viewerCount,
				Percentage:     0, // Will calculate after processing all events
				TotalBandwidth: event.VariantBandwidth[qualityLevel],
				AverageBitrate: 0, // Will calculate from variant data
			}
		}
	}
}

// processSessionEvents processes all session events for analytics
func (r *TrendingRepository) processSessionEvents(
	sessionEvents []models.MediaAnalytics,
	mediaID string,
	analytics *storage.StreamingAnalyticsData,
) (map[string]bool, *struct {
	totalWatchTime         float64
	totalCompletedSessions int
}) {
	uniqueUsers := make(map[string]bool)
	sessionMetrics := &struct {
		totalWatchTime         float64
		totalCompletedSessions int
	}{}

	for _, event := range sessionEvents {
		r.processSessionEvent(event, mediaID, analytics, uniqueUsers, sessionMetrics)
	}

	return uniqueUsers, sessionMetrics
}

// finalizeBasicMetrics calculates final analytics metrics
func (r *TrendingRepository) finalizeBasicMetrics(
	analytics *storage.StreamingAnalyticsData,
	uniqueUsers map[string]bool,
	sessionMetrics *struct {
		totalWatchTime         float64
		totalCompletedSessions int
	},
) {
	// Set unique viewers count
	analytics.UniqueViewers = len(uniqueUsers)

	// Calculate average watch time
	if analytics.TotalViews > 0 {
		analytics.AverageWatchTime = sessionMetrics.totalWatchTime / float64(analytics.TotalViews)
	}

	// Calculate completion rate
	if analytics.TotalViews > 0 {
		analytics.CompletionRate = float64(sessionMetrics.totalCompletedSessions) / float64(analytics.TotalViews)
	}
}

// queryBufferingEvents retrieves and counts buffering events
func (r *TrendingRepository) queryBufferingEvents(ctx context.Context, mediaID string, last7Days time.Time) (int, error) {
	var bufferingEvents []models.MediaAnalytics
	err := r.db.WithContext(ctx).Model(&models.MediaAnalytics{}).
		Where("PK", "=", "MEDIA_EVENT#rebuffer_start").
		Where("SK", "begins_with", fmt.Sprintf("%d", last7Days.Unix())).
		All(&bufferingEvents)

	if err != nil {
		return 0, nil // Ignore errors for buffering events
	}

	totalBufferingEvents := 0
	for _, event := range bufferingEvents {
		if event.MediaID == mediaID {
			totalBufferingEvents++
		}
	}

	return totalBufferingEvents, nil
}

// calculateQualityMetrics calculates quality distribution percentages and bitrates
func (r *TrendingRepository) calculateQualityMetrics(analytics *storage.StreamingAnalyticsData) {
	// Calculate total quality views
	totalQualityViews := 0
	for _, stats := range analytics.QualityDistribution {
		totalQualityViews += stats.ViewCount
	}

	// Calculate percentages and bitrates
	for _, stats := range analytics.QualityDistribution {
		if totalQualityViews > 0 {
			stats.Percentage = float64(stats.ViewCount) / float64(totalQualityViews) * 100
		}

		// Calculate average bitrate from bandwidth and view count
		if stats.ViewCount > 0 && stats.TotalBandwidth > 0 {
			// Convert bytes to bits and average over sessions
			stats.AverageBitrate = float64(stats.TotalBandwidth*8) / float64(stats.ViewCount) / 1000 // kbps
		}
	}
}

// addRecentMetrics adds recent performance metrics to analytics
func (r *TrendingRepository) addRecentMetrics(
	analytics *storage.StreamingAnalyticsData,
	sessionEvents []models.MediaAnalytics,
	mediaID string,
	last24Hours time.Time,
) {
	analytics.RecentMetrics["last_24h_views"] = r.countRecentSessions(sessionEvents, mediaID, last24Hours)
	analytics.RecentMetrics["peak_concurrent_sessions"] = analytics.StreamingSessions
	analytics.RecentMetrics["total_bandwidth_gb"] = float64(analytics.TotalBandwidthBytes) / (1024 * 1024 * 1024)
}

// queryQualityChangeEvents retrieves and counts quality change events
func (r *TrendingRepository) queryQualityChangeEvents(ctx context.Context, mediaID string, last7Days time.Time) (int, error) {
	// Resolve quality-change rows per media through GSI3 instead of scanning
	// the whole table by PK prefix.
	var qualityChangeEvents []models.MediaAnalytics
	err := r.db.WithContext(ctx).Model(&models.MediaAnalytics{}).
		Index("gsi3").
		Where("gsi3PK", "=", fmt.Sprintf("MEDIA_QUALITY#%s", mediaID)).
		Where("gsi3SK", ">=", fmt.Sprintf("TS#%d", last7Days.Unix())).
		All(&qualityChangeEvents)

	if err != nil {
		return 0, nil // Ignore errors for quality change events
	}

	return len(qualityChangeEvents), nil
}

// GetStreamingAnalytics retrieves comprehensive streaming analytics for a media item
func (r *TrendingRepository) GetStreamingAnalytics(ctx context.Context, mediaID string) (*storage.StreamingAnalyticsData, error) {
	now := time.Now()
	last24Hours := now.Add(-24 * time.Hour)
	last7Days := now.Add(-7 * 24 * time.Hour)

	// Initialize analytics data structure
	analytics := r.initializeAnalyticsData(mediaID)

	// Query session events
	sessionEvents, err := r.querySessionEvents(ctx, last7Days)
	if err != nil {
		r.logger.Error("failed to query session events", zap.String("mediaID", mediaID), zap.Error(err))
		return nil, err
	}

	// Process session events
	uniqueUsers, sessionMetrics := r.processSessionEvents(sessionEvents, mediaID, analytics)

	// Calculate basic metrics
	r.finalizeBasicMetrics(analytics, uniqueUsers, sessionMetrics)

	// Query and set buffering events
	bufferingEvents, _ := r.queryBufferingEvents(ctx, mediaID, last7Days)
	analytics.BufferingEvents = bufferingEvents

	// Calculate quality distribution metrics
	r.calculateQualityMetrics(analytics)

	// Add recent performance metrics
	r.addRecentMetrics(analytics, sessionEvents, mediaID, last24Hours)

	// Query and add quality change events
	qualityChanges, _ := r.queryQualityChangeEvents(ctx, mediaID, last7Days)
	analytics.RecentMetrics["quality_changes_7d"] = qualityChanges

	r.logger.Debug("retrieved streaming analytics",
		zap.String("mediaID", mediaID),
		zap.Int("totalViews", analytics.TotalViews),
		zap.Int("uniqueViewers", analytics.UniqueViewers),
		zap.Float64("averageWatchTime", analytics.AverageWatchTime),
		zap.Int("bufferingEvents", analytics.BufferingEvents))

	return analytics, nil
}

// countRecentSessions counts sessions within a specific time period
func (r *TrendingRepository) countRecentSessions(events []models.MediaAnalytics, mediaID string, since time.Time) int {
	count := 0
	for _, event := range events {
		if event.MediaID == mediaID && event.Timestamp.After(since) {
			count++
		}
	}
	return count
}

// ========== ModerationAnalytics Methods ==========

// RecordModerationAction records a moderation action for analytics
func (r *TrendingRepository) RecordModerationAction(ctx context.Context, date, reportType string, action *storage.ModerationAction) error {
	now := time.Now()

	// First, get existing analytics to update counts
	pk := fmt.Sprintf("MOD_ANALYTICS#%s", date)
	sk := fmt.Sprintf("type#%s", reportType)

	var existing models.ModerationAnalytics
	err := r.db.WithContext(ctx).Model(&models.ModerationAnalytics{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&existing)

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to get existing moderation analytics",
			zap.String("date", date),
			zap.String("reportType", reportType),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrFailedGetModerationAnalytics, err)
	}

	// Initialize or update analytics
	analytics := &models.ModerationAnalytics{
		PK:                    pk,
		SK:                    sk,
		Date:                  date,
		ReportType:            reportType,
		Count:                 existing.Count + 1,
		ResolvedCount:         existing.ResolvedCount,
		AverageResolutionTime: existing.AverageResolutionTime,
		ModeratorActions:      existing.ModeratorActions,
		UpdatedAt:             now,
		TTL:                   now.Add(90 * 24 * time.Hour).Unix(), // 90 days retention
	}

	if analytics.ModeratorActions == nil {
		analytics.ModeratorActions = make(map[string]int64)
	}

	// Update based on action
	if action.Resolved {
		analytics.ResolvedCount++
		// Update average resolution time
		if analytics.ResolvedCount > 1 {
			analytics.AverageResolutionTime = (analytics.AverageResolutionTime*float64(analytics.ResolvedCount-1) + action.ResolutionTime) / float64(analytics.ResolvedCount)
		} else {
			analytics.AverageResolutionTime = action.ResolutionTime
		}
	}

	// Track moderator actions
	if action.ModeratorID != "" {
		analytics.ModeratorActions[action.ModeratorID]++
	}

	_ = analytics.UpdateKeys() // Ignore error as this is internal model operation

	// Moderation analytics is a deterministic aggregate and must overwrite explicitly.
	err = r.db.WithContext(ctx).Model(analytics).CreateOrUpdate()
	if err != nil {
		r.logger.Error("failed to record moderation action",
			zap.String("date", date),
			zap.String("reportType", reportType),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrFailedRecordModerationAction, err)
	}

	return nil
}

// GetModerationAnalytics retrieves moderation analytics for a date and type
func (r *TrendingRepository) GetModerationAnalytics(ctx context.Context, date, reportType string) (*storage.ModerationAnalyticsData, error) {
	pk := fmt.Sprintf("MOD_ANALYTICS#%s", date)
	sk := fmt.Sprintf("type#%s", reportType)

	var analytics models.ModerationAnalytics
	err := r.db.WithContext(ctx).Model(&models.ModerationAnalytics{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&analytics)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "moderation analytics", fmt.Sprintf("%s#%s", date, reportType))
		}
		r.logger.Error("failed to get moderation analytics",
			zap.String("date", date),
			zap.String("reportType", reportType),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "moderation analytics", fmt.Sprintf("%s#%s", date, reportType))
	}

	return &storage.ModerationAnalyticsData{
		Date:                  analytics.Date,
		ReportType:            analytics.ReportType,
		Count:                 analytics.Count,
		ResolvedCount:         analytics.ResolvedCount,
		AverageResolutionTime: analytics.AverageResolutionTime,
		ModeratorActions:      analytics.ModeratorActions,
		UpdatedAt:             analytics.UpdatedAt,
	}, nil
}

// GetModeratorStats retrieves statistics for a specific moderator
func (r *TrendingRepository) GetModeratorStats(ctx context.Context, moderatorID string, days int) (*storage.ModeratorStatistics, error) {
	stats := &storage.ModeratorStatistics{
		ModeratorID:   moderatorID,
		TotalActions:  0,
		ActionsByType: make(map[string]int64),
		DailyActions:  make([]storage.DailyModeratorAction, 0, days),
	}

	now := time.Now()
	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -i).Format(common.DateFormat)
		pk := fmt.Sprintf("MOD_ANALYTICS#%s", date)

		// Query all report types for this date
		var analyticsRecords []models.ModerationAnalytics
		err := r.db.WithContext(ctx).Model(&models.ModerationAnalytics{}).
			Where("PK", "=", pk).
			All(&analyticsRecords)

		if err != nil && !errors.IsNotFound(err) {
			r.logger.Warn("failed to get moderation analytics for date",
				zap.String("date", date),
				zap.Error(err))
			continue
		}

		dailyActions := int64(0)
		for _, record := range analyticsRecords {
			if actions, ok := record.ModeratorActions[moderatorID]; ok {
				stats.TotalActions += actions
				stats.ActionsByType[record.ReportType] += actions
				dailyActions += actions
			}
		}

		if dailyActions > 0 {
			stats.DailyActions = append(stats.DailyActions, storage.DailyModeratorAction{
				Date:    date,
				Actions: dailyActions,
			})
		}
	}

	// Calculate average actions per day
	if days > 0 {
		stats.AverageActionsPerDay = float64(stats.TotalActions) / float64(days)
	}

	return stats, nil
}

// GetReportTrends retrieves trends for different report types
func (r *TrendingRepository) GetReportTrends(ctx context.Context, reportTypes []string, days int) (map[string]*storage.ReportTrend, error) {
	trends := make(map[string]*storage.ReportTrend)
	now := time.Now()

	for _, reportType := range reportTypes {
		trend := &storage.ReportTrend{
			ReportType: reportType,
			Daily:      make([]storage.DailyReportCount, 0, days),
		}

		for i := 0; i < days; i++ {
			date := now.AddDate(0, 0, -i).Format(common.DateFormat)
			analytics, err := r.GetModerationAnalytics(ctx, date, reportType)

			if err != nil {
				r.logger.Warn("failed to get analytics for report trend",
					zap.String("date", date),
					zap.String("reportType", reportType),
					zap.Error(err))
				continue
			}

			if analytics != nil {
				trend.Daily = append(trend.Daily, storage.DailyReportCount{
					Date:          date,
					Count:         analytics.Count,
					ResolvedCount: analytics.ResolvedCount,
				})
				trend.TotalCount += analytics.Count
				trend.TotalResolved += analytics.ResolvedCount
			}
		}

		// Calculate resolution rate
		if trend.TotalCount > 0 {
			trend.ResolutionRate = float64(trend.TotalResolved) / float64(trend.TotalCount) * 100
		}

		// Reverse daily data to have oldest first
		for i, j := 0, len(trend.Daily)-1; i < j; i, j = i+1, j-1 {
			trend.Daily[i], trend.Daily[j] = trend.Daily[j], trend.Daily[i]
		}

		trends[reportType] = trend
	}

	return trends, nil
}

// GetActiveUserCount returns the number of active users in the last N days.
//
// O(1) read: sums the maintained per-UTC-day distinct-actor counters over the
// window (see instance_counts.go). The sum is an upper bound on the true
// window-distinct count — an actor active on multiple days is counted once per
// day — documented as acceptable for the public instance stats surface. Every
// read is a point read of a maintained day counter; missing days sum as zero,
// and the read NEVER scans. The rollup is populated off the request path
// (activity write path + the offline recount).
func (r *TrendingRepository) GetActiveUserCount(ctx context.Context, days int) (int, error) {
	return readActiveMonthCount(ctx, r.db, r.logger, days)
}

// GetTotalUserCount returns the total number of users.
//
// O(1) read: returns the maintained TOTAL_USERS counter (point read; an
// unseeded counter reads as the documented default 0), kept current by the
// user/account write paths and seeded off the request path by the offline
// `lesser recount-instance-counts` tool. No scan ever runs here (see
// instance_counts.go).
func (r *TrendingRepository) GetTotalUserCount(ctx context.Context) (int, error) {
	count, err := readTotalUsersCount(ctx, r.db, r.logger)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// GetTotalStatusCount returns the total number of statuses.
//
// O(1) read: returns the maintained TOTAL_STATUSES counter, kept current by
// the status write path.
func (r *TrendingRepository) GetTotalStatusCount(ctx context.Context) (*int, error) {
	count, err := readTotalStatusesCount(ctx, r.db, r.logger)
	if err != nil {
		return nil, err
	}
	c := int(count)
	return &c, nil
}

// GetTotalDomainCount returns the total number of known domains.
//
// O(1) read: returns the maintained TOTAL_DOMAINS counter (point read; an
// unseeded counter reads as the documented default 0), kept current by the
// actor/account write paths and seeded off the request path by the offline
// `lesser recount-instance-counts` tool. No scan ever runs here (see
// instance_counts.go).
func (r *TrendingRepository) GetTotalDomainCount(ctx context.Context) (int, error) {
	count, err := readTotalDomainsCount(ctx, r.db, r.logger)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// ========== Popular Query Atomic Counter Methods ==========

// IncrementQueryCount atomically increments the count for a search query
func (r *TrendingRepository) IncrementQueryCount(ctx context.Context, query string, count int) error {
	if query == "" || count <= 0 {
		return ErrInvalidQueryParameters
	}

	// Normalize and validate query using centralized validation
	normalizedQuery, err := common.ValidateNormalizedQuery(query)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidQueryForCounting, err)
	}
	queryHash := r.hashQuery(normalizedQuery)

	now := time.Now()
	date := now.Format(common.DateFormat)

	// Update counters for different time buckets
	timeBuckets := []string{"daily", "weekly", "monthly"}

	for _, bucket := range timeBuckets {
		if err := r.incrementCounterForBucket(ctx, queryHash, normalizedQuery, bucket, date, count, now); err != nil {
			r.logger.Error("failed to increment counter for bucket",
				zap.String("query", normalizedQuery),
				zap.String("bucket", bucket),
				zap.Error(err))
			// Continue with other buckets
		}
	}

	return nil
}

// GetQueryCount retrieves the current count for a query
func (r *TrendingRepository) GetQueryCount(ctx context.Context, query string) (int, error) {
	// Normalize and validate query using centralized validation
	normalizedQuery, err := common.ValidateNormalizedQuery(query)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalidQueryForCount, err)
	}
	queryHash := r.hashQuery(normalizedQuery)

	// Get daily count by default
	pk := fmt.Sprintf("POPULAR_QUERY#%s", queryHash)
	sk := "COUNTER#daily"

	var counter models.PopularQueryCounter
	err = r.db.WithContext(ctx).Model(&models.PopularQueryCounter{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&counter)

	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil
		}
		r.logger.Error("failed to get query count",
			zap.String("query", normalizedQuery),
			zap.Error(err))
		return 0, fmt.Errorf("%w: %w", ErrFailedGetQueryCount, err)
	}

	return int(counter.Count), nil
}

// GetTopQueries retrieves the most popular queries within a time range
func (r *TrendingRepository) GetTopQueries(ctx context.Context, limit int, timeRange time.Duration) ([]storage.SearchQueryStats, error) {
	// Validate limit using centralized validation
	if err := common.ValidateQueryLimit(limit, 100, "analytics"); err != nil {
		limit = 20
	}

	// Determine time bucket based on range
	bucket := models.PeriodDaily
	if timeRange > 24*time.Hour && timeRange <= 7*24*time.Hour {
		bucket = models.PeriodWeekly
	} else if timeRange > 7*24*time.Hour {
		bucket = models.PeriodMonthly
	}

	// Calculate date range
	now := time.Now()
	endDate := now.Format(common.DateFormat)

	// Query using GSI8 for ranking
	gsi8PK := fmt.Sprintf("POPULAR#%s#%s", bucket, endDate)

	var counters []models.PopularQueryCounter
	err := r.db.WithContext(ctx).Model(&models.PopularQueryCounter{}).
		Where("gsi8PK", "=", gsi8PK).
		OrderBy("gsi8SK", "DESC"). // Highest counts first
		Limit(limit).
		All(&counters)

	if err != nil {
		if errors.IsNotFound(err) {
			return []storage.SearchQueryStats{}, nil
		}
		r.logger.Error("failed to get top queries",
			zap.String("bucket", bucket),
			zap.String("date", endDate),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %w", ErrFailedGetTopQueries, err)
	}

	// Convert to storage type
	stats := make([]storage.SearchQueryStats, 0, len(counters))
	for _, counter := range counters {
		stat := storage.SearchQueryStats{
			Query:       counter.Query,
			Count:       int(counter.Count),
			UserCount:   int(counter.UserCount),
			AvgResults:  counter.AvgResults,
			LastUsed:    counter.LastQueried,
			LastQueried: counter.LastQueried,
		}
		stats = append(stats, stat)
	}

	return stats, nil
}

// incrementCounterForBucket atomically increments counter for specific time bucket
func (r *TrendingRepository) incrementCounterForBucket(ctx context.Context, queryHash, query, bucket, date string, count int, now time.Time) error {
	pk := fmt.Sprintf("POPULAR_QUERY#%s", queryHash)
	sk := fmt.Sprintf("COUNTER#%s", bucket)

	// Try to get existing counter
	var existing models.PopularQueryCounter
	err := r.db.WithContext(ctx).Model(&models.PopularQueryCounter{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&existing)

	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("%w: %w", ErrFailedGetExistingCounter, err)
	}

	// Create or update counter
	counter := &models.PopularQueryCounter{
		QueryHash:  queryHash,
		Query:      query,
		TimeBucket: bucket,
		Date:       date,
		UpdatedAt:  now,
	}

	if errors.IsNotFound(err) {
		// Create new counter
		counter.Count = int64(count)
		counter.UserCount = 1
		counter.AvgResults = 0 // Will be calculated later
		counter.FirstQueried = now
		counter.LastQueried = now
	} else {
		// Update existing counter atomically
		counter.Count = existing.Count + int64(count)
		counter.UserCount = existing.UserCount // Will be updated separately for unique users
		counter.AvgResults = existing.AvgResults
		counter.FirstQueried = existing.FirstQueried
		counter.LastQueried = now
	}

	// Update keys
	_ = counter.UpdateKeys() // Ignore error as this is internal model operation

	// Use Create for new or Update for existing
	if errors.IsNotFound(err) {
		err = r.db.WithContext(ctx).Model(counter).Create()
	} else {
		// Copy existing PK/SK to maintain consistency
		counter.PK = existing.PK
		counter.SK = existing.SK
		err = r.db.WithContext(ctx).Model(counter).Update()
	}

	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedSaveCounter, err)
	}

	return nil
}

// hashQuery creates a consistent hash for queries (for privacy)
func (r *TrendingRepository) hashQuery(query string) string {
	// Simple hash for now - in production, use SHA-256 or similar
	// This allows for privacy-preserving analytics
	h := fmt.Sprintf("%x", query) // Simple hex conversion
	if len(h) > 32 {
		h = h[:32] // Limit length
	}
	return h
}
