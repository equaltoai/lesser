package repositories

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// TrendingRepository implements trending and analytics operations using DynamORM
type TrendingRepository struct {
	db     core.DB
	logger *zap.Logger
}

// NewTrendingRepository creates a new trending repository
func NewTrendingRepository(db core.DB, logger *zap.Logger) *TrendingRepository {
	return &TrendingRepository{
		db:     db,
		logger: logger,
	}
}

// RecordHashtagUsage records when a hashtag is used in a status
func (r *TrendingRepository) RecordHashtagUsage(ctx context.Context, hashtag string, statusID string, authorID string) error {
	now := time.Now()
	usage := &models.HashtagUsage{
		StatusID:   statusID,
		AuthorID:   authorID,
		UsedAt:     now,
		Visibility: "public", // Default visibility
		TTL:        now.Add(30 * 24 * time.Hour).Unix(), // 30 days as per existing model
		CreatedAt:  now,
	}
	
	// Set keys using the existing UpdateKeys method
	usage.UpdateKeys(hashtag)

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
		PK:             fmt.Sprintf("STATUS_ENGAGEMENT#%s", statusID),
		SK:             fmt.Sprintf("%s#%s#%s", engagementType, time.Now().Format("20060102150405"), userID),
		StatusID:       statusID,
		EngagementType: engagementType,
		UserID:         userID,
		EngagedAt:      time.Now(),
		TTL:            time.Now().Add(7 * 24 * time.Hour).Unix(),
	}

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
		PK:       fmt.Sprintf("LINK_SHARE#%s", linkURL),
		SK:       fmt.Sprintf("STATUS#%s", statusID),
		URL:      linkURL,
		StatusID: statusID,
		AuthorID: authorID,
		SharedAt: time.Now(),
		TTL:      time.Now().Add(7 * 24 * time.Hour).Unix(),
	}

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
func (r *TrendingRepository) GetTrendingHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	timeBucket := time.Now().Format("2006-01-02")
	pk := fmt.Sprintf("TREND_TYPE#HASHTAG#%s", timeBucket)

	var trendModels []models.HashtagTrend
	err := r.db.WithContext(ctx).Model(&models.HashtagTrend{}).
		Where("GSI8PK", "=", pk).
		OrderBy("GSI8SK", "DESC"). // Sort by score descending
		Limit(limit).
		All(&trendModels)

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("no trending hashtags found", zap.String("timeBucket", timeBucket))
			return []*storage.TrendingHashtag{}, nil
		}
		r.logger.Error("failed to get trending hashtags", zap.Error(err))
		return nil, err
	}

	trends := make([]*storage.TrendingHashtag, 0, len(trendModels))
	for _, model := range trendModels {
		trend := &storage.TrendingHashtag{
			Name:        model.Name,
			URL:         model.URL,
			UsageCount:  model.UsageCount,
			UniqueUsers: model.UniqueUsers,
			LastUsed:    model.LastUsed,
			FirstSeen:   model.FirstSeen,
		}
		trends = append(trends, trend)
	}

	return trends, nil
}

// GetTrendingStatuses returns the top trending statuses since the given time
func (r *TrendingRepository) GetTrendingStatuses(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	timeBucket := time.Now().Format("2006-01-02")
	pk := fmt.Sprintf("TREND_TYPE#STATUS#%s", timeBucket)

	var trendModels []models.StatusTrend
	err := r.db.WithContext(ctx).Model(&models.StatusTrend{}).
		Where("GSI8PK", "=", pk).
		OrderBy("GSI8SK", "DESC"). // Sort by score descending
		Limit(limit).
		All(&trendModels)

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("no trending statuses found", zap.String("timeBucket", timeBucket))
			return []*storage.TrendingStatus{}, nil
		}
		r.logger.Error("failed to get trending statuses", zap.Error(err))
		return nil, err
	}

	trends := make([]*storage.TrendingStatus, 0, len(trendModels))
	for _, model := range trendModels {
		trend := &storage.TrendingStatus{
			ID:          model.ID,
			URL:         model.URL,
			AuthorID:    model.AuthorID,
			Content:     model.Content,
			Engagements: model.Engagements,
			PublishedAt: model.PublishedAt,
		}
		trends = append(trends, trend)
	}

	return trends, nil
}

// GetTrendingLinks returns the top trending links since the given time
func (r *TrendingRepository) GetTrendingLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	timeBucket := time.Now().Format("2006-01-02")
	pk := fmt.Sprintf("TREND_TYPE#LINK#%s", timeBucket)

	var trendModels []models.LinkTrend
	err := r.db.WithContext(ctx).Model(&models.LinkTrend{}).
		Where("GSI8PK", "=", pk).
		OrderBy("GSI8SK", "DESC"). // Sort by score descending
		Limit(limit).
		All(&trendModels)

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("no trending links found", zap.String("timeBucket", timeBucket))
			return []*storage.TrendingLink{}, nil
		}
		r.logger.Error("failed to get trending links", zap.Error(err))
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
	timeBucket := now.Format("2006-01-02")
	paddedScore := fmt.Sprintf("%010.0f", score*1000) // Pad for proper sorting

	trendItem := &models.HashtagTrend{
		PK:          fmt.Sprintf("TREND_TYPE#HASHTAG#%s", timeBucket),
		SK:          fmt.Sprintf("SCORE#%s#%s", paddedScore, hashtag),
		Name:        hashtag,
		URL:         fmt.Sprintf("https://example.com/tags/%s", hashtag), // TODO: use actual domain
		UsageCount:  usageCount,
		UniqueUsers: int64(len(uniqueUsers)),
		LastUsed:    now,
		FirstSeen:   now, // Would be updated if exists
		TrendScore:  score,
		UpdatedAt:   now,
		TTL:         now.Add(7 * 24 * time.Hour).Unix(),
	}
	trendItem.UpdateKeys()

	err = r.db.WithContext(ctx).Model(trendItem).Create()
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
	timeBucket := now.Format("2006-01-02")
	paddedScore := fmt.Sprintf("%010.0f", score*1000)

	trendItem := &models.StatusTrend{
		PK:           fmt.Sprintf("TREND_TYPE#STATUS#%s", timeBucket),
		SK:           fmt.Sprintf("SCORE#%s#%s", paddedScore, statusID),
		ID:           statusID,
		URL:          fmt.Sprintf("https://example.com/statuses/%s", statusID), // TODO: use actual domain
		AuthorID:     "", // Would be filled from actual status
		Content:      "", // Would be filled from actual status
		Engagements:  totalEngagements,
		PublishedAt:  publishedAt,
		TrendScore:   score,
		UpdatedAt:    now,
		TTL:          now.Add(7 * 24 * time.Hour).Unix(),
	}
	trendItem.UpdateKeys()

	err = r.db.WithContext(ctx).Model(trendItem).Create()
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
		linkType = "video"
		title = "YouTube Video"
	} else if strings.Contains(lowerURL, ".jpg") || strings.Contains(lowerURL, ".png") ||
		strings.Contains(lowerURL, ".gif") || strings.Contains(lowerURL, ".webp") {
		linkType = "photo"
		title = "Image"
		image = linkURL
	}

	// Calculate trend score
	now := time.Now()
	diversityFactor := float64(len(uniqueSharers)) / float64(shareCount+1)
	score := float64(shareCount) * (1 + diversityFactor)

	// Update trending index entry
	timeBucket := now.Format("2006-01-02")
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
	trendItem.UpdateKeys()

	err = r.db.WithContext(ctx).Model(trendItem).Create()
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
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Since DynamoDB doesn't allow direct queries on non-key attributes like UsedAt,
	// we'll use the existing hashtag metadata approach which tracks LastUsed
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
	// Query recent status engagements
	var engagements []models.StatusEngagement
	err := r.db.WithContext(ctx).Model(&models.StatusEngagement{}).
		Where("EngagedAt", ">=", since).
		OrderBy("EngagedAt", "DESC").
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
			URL:         fmt.Sprintf("https://example.com/statuses/%s", statusID), // TODO: use actual domain
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

	if len(statuses) > limit {
		statuses = statuses[:limit]
	}

	return statuses, nil
}

// GetRecentLinks returns recent links since the given time (no trending calculation)
func (r *TrendingRepository) GetRecentLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	// Query recent link shares
	var shares []models.LinkShare
	err := r.db.WithContext(ctx).Model(&models.LinkShare{}).
		Where("SharedAt", ">=", since).
		OrderBy("SharedAt", "DESC").
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

	if len(links) > limit {
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

	err := r.db.WithContext(ctx).Model(model).Create()
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
			return nil, nil // Return nil without error for not found
		}
		r.logger.Error("failed to get engagement metrics",
			zap.String("statusID", statusID),
			zap.Error(err))
		return nil, err
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
	// Convert from interface{} to expected type
	hashtagTrend, ok := trend.(*models.HashtagTrend)
	if !ok {
		// Try to convert from storage type
		storageTrend, ok := trend.(*storage.TrendingHashtag)
		if !ok {
			return fmt.Errorf("invalid trend type: expected *models.HashtagTrend or *storage.TrendingHashtag")
		}
		
		// Convert storage to model
		hashtagTrend = &models.HashtagTrend{
			Name:        storageTrend.Name,
			URL:         storageTrend.URL,
			UsageCount:  storageTrend.UsageCount,
			UniqueUsers: storageTrend.UniqueUsers,
			LastUsed:    storageTrend.LastUsed,
			FirstSeen:   storageTrend.FirstSeen,
			TrendScore:  float64(storageTrend.UsageCount), // Calculate based on usage
			UpdatedAt:   time.Now(),
		}
	}
	
	// Update keys
	hashtagTrend.UpdateKeys()
	
	// Store the trend
	err := r.db.WithContext(ctx).Model(hashtagTrend).Create()
	if err != nil {
		r.logger.Error("failed to store hashtag trend",
			zap.String("hashtag", hashtagTrend.Name),
			zap.Error(err))
		return fmt.Errorf("failed to store hashtag trend: %w", err)
	}
	
	return nil
}

// StoreStatusTrend stores a status trend record
func (r *TrendingRepository) StoreStatusTrend(ctx context.Context, trend any) error {
	// Convert from interface{} to expected type
	statusTrend, ok := trend.(*models.StatusTrend)
	if !ok {
		// Try to convert from storage type
		storageTrend, ok := trend.(*storage.TrendingStatus)
		if !ok {
			return fmt.Errorf("invalid trend type: expected *models.StatusTrend or *storage.TrendingStatus")
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
	
	// Update keys
	statusTrend.UpdateKeys()
	
	// Store the trend
	err := r.db.WithContext(ctx).Model(statusTrend).Create()
	if err != nil {
		r.logger.Error("failed to store status trend",
			zap.String("statusID", statusTrend.ID),
			zap.Error(err))
		return fmt.Errorf("failed to store status trend: %w", err)
	}
	
	return nil
}

// StoreLinkTrend stores a link trend record
func (r *TrendingRepository) StoreLinkTrend(ctx context.Context, trend any) error {
	// Convert from interface{} to expected type
	linkTrend, ok := trend.(*models.LinkTrend)
	if !ok {
		// Try to convert from storage type
		storageTrend, ok := trend.(*storage.TrendingLink)
		if !ok {
			return fmt.Errorf("invalid trend type: expected *models.LinkTrend or *storage.TrendingLink")
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
	
	// Update keys
	linkTrend.UpdateKeys()
	
	// Store the trend
	err := r.db.WithContext(ctx).Model(linkTrend).Create()
	if err != nil {
		r.logger.Error("failed to store link trend",
			zap.String("url", linkTrend.URL),
			zap.Error(err))
		return fmt.Errorf("failed to store link trend: %w", err)
	}
	
	return nil
}

// DeleteOldHashtagTrends deletes hashtag trend records older than the specified time
func (r *TrendingRepository) DeleteOldHashtagTrends(ctx context.Context, before time.Time) error {
	// Query for old hashtag trends
	var trends []models.HashtagTrend
	
	// DynamORM doesn't support direct batch delete, so we need to query and delete
	err := r.db.WithContext(ctx).Model(&models.HashtagTrend{}).
		Filter("UpdatedAt", "<", before).
		Limit(100). // Process in batches
		Scan(&trends)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return nil // No old trends to delete
		}
		r.logger.Error("failed to query old hashtag trends", zap.Error(err))
		return fmt.Errorf("failed to query old hashtag trends: %w", err)
	}
	
	// Delete each trend
	for _, trend := range trends {
		err := r.db.WithContext(ctx).Model(&trend).Delete()
		if err != nil {
			r.logger.Warn("failed to delete hashtag trend",
				zap.String("hashtag", trend.Name),
				zap.Error(err))
			// Continue with other deletions
		}
	}
	
	r.logger.Info("deleted old hashtag trends",
		zap.Int("count", len(trends)),
		zap.Time("before", before))
	
	return nil
}

// DeleteOldLinkTrends deletes link trend records older than the specified time
func (r *TrendingRepository) DeleteOldLinkTrends(ctx context.Context, before time.Time) error {
	// Query for old link trends
	var trends []models.LinkTrend
	
	err := r.db.WithContext(ctx).Model(&models.LinkTrend{}).
		Filter("UpdatedAt", "<", before).
		Limit(100). // Process in batches
		Scan(&trends)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return nil // No old trends to delete
		}
		r.logger.Error("failed to query old link trends", zap.Error(err))
		return fmt.Errorf("failed to query old link trends: %w", err)
	}
	
	// Delete each trend
	for _, trend := range trends {
		err := r.db.WithContext(ctx).Model(&trend).Delete()
		if err != nil {
			r.logger.Warn("failed to delete link trend",
				zap.String("url", trend.URL),
				zap.Error(err))
			// Continue with other deletions
		}
	}
	
	r.logger.Info("deleted old link trends",
		zap.Int("count", len(trends)),
		zap.Time("before", before))
	
	return nil
}

// DeleteOldStatusTrends deletes status trend records older than the specified time
func (r *TrendingRepository) DeleteOldStatusTrends(ctx context.Context, before time.Time) error {
	// Query for old status trends
	var trends []models.StatusTrend
	
	err := r.db.WithContext(ctx).Model(&models.StatusTrend{}).
		Filter("UpdatedAt", "<", before).
		Limit(100). // Process in batches
		Scan(&trends)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return nil // No old trends to delete
		}
		r.logger.Error("failed to query old status trends", zap.Error(err))
		return fmt.Errorf("failed to query old status trends: %w", err)
	}
	
	// Delete each trend
	for _, trend := range trends {
		err := r.db.WithContext(ctx).Model(&trend).Delete()
		if err != nil {
			r.logger.Warn("failed to delete status trend",
				zap.String("statusID", trend.ID),
				zap.Error(err))
			// Continue with other deletions
		}
	}
	
	r.logger.Info("deleted old status trends",
		zap.Int("count", len(trends)),
		zap.Time("before", before))
	
	return nil
}

// TrackSearchQuery records a search query for analytics
func (r *TrendingRepository) TrackSearchQuery(ctx context.Context, userID, query string, resultCount int) error {
	// Normalize query
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	if normalizedQuery == "" {
		return nil
	}
	
	// Create search query record
	searchQuery := &models.SearchQuery{
		Query:       normalizedQuery,
		UserID:      userID,
		ResultCount: resultCount,
		SearchedAt:  time.Now(),
	}
	
	// Update keys
	searchQuery.UpdateKeys()
	
	// Store the query
	err := r.db.WithContext(ctx).Model(searchQuery).Create()
	if err != nil {
		r.logger.Error("failed to track search query",
			zap.String("query", query),
			zap.String("userID", userID),
			zap.Error(err))
		return fmt.Errorf("failed to track search query: %w", err)
	}
	
	// Also update popular queries index
	r.updatePopularQueries(ctx, normalizedQuery)
	
	return nil
}

// GetPopularSearchQueries retrieves the most popular search queries
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
		return nil, fmt.Errorf("failed to query search queries: %w", err)
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
	if len(results) > limit {
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
		return nil, fmt.Errorf("failed to query user search history: %w", err)
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

// updatePopularQueries increments the count for a search query
func (r *TrendingRepository) updatePopularQueries(ctx context.Context, query string) {
	// This would typically use an atomic counter update
	// For now, we'll log this as a TODO
	r.logger.Debug("TODO: implement popular query counter update",
		zap.String("query", query))
}

// GetStatusesByLink retrieves statuses that contain a specific link
func (r *TrendingRepository) GetStatusesByLink(ctx context.Context, linkURL string, limit int) ([]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	
	// Query link trends that match the URL
	var linkTrends []*models.LinkTrend
	err := r.db.Model(&models.LinkTrend{}).
		Where("URL", "=", linkURL).
		OrderBy("UsageCount", "DESC").
		Limit(limit).
		All(&linkTrends)
		
	if err != nil {
		r.logger.Error("failed to get statuses by link",
			zap.String("link_url", linkURL),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get statuses by link: %w", err)
	}
	
	// Convert to any slice (would normally fetch actual status objects)
	results := make([]any, len(linkTrends))
	for i, trend := range linkTrends {
		// In a real implementation, we would fetch the actual status objects
		// For now, return basic info
		results[i] = map[string]interface{}{
			"url":         trend.URL,
			"title":       trend.Title,
			"share_count": trend.ShareCount,
			"updated_at":  trend.UpdatedAt,
			"trend_score": trend.TrendScore,
		}
	}
	
	return results, nil
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
		return fmt.Errorf("failed to index by engagement: %w", err)
	}

	r.logger.Debug("indexed status by engagement",
		zap.String("statusID", statusID),
		zap.String("bucket", bucket))

	return nil
}

// GenerateSearchSuggestions generates search suggestions based on user history and popular queries
func (r *TrendingRepository) GenerateSearchSuggestions(ctx context.Context, userID, partialQuery string, limit int) ([]string, error) {
	normalizedQuery := strings.ToLower(strings.TrimSpace(partialQuery))
	if normalizedQuery == "" {
		return []string{}, nil
	}

	suggestions := make(map[string]float64) // suggestion -> score

	// Get user's search history
	userHistory, err := r.GetUserSearchHistory(ctx, userID, 50)
	if err == nil {
		// Score user's past searches that match the partial query
		for _, entry := range userHistory {
			if strings.HasPrefix(strings.ToLower(entry.Query), normalizedQuery) {
				// Recent searches get higher scores
				age := time.Since(entry.SearchedAt)
				recencyScore := 1.0 / (1 + age.Hours()/24) // Decay over days
				suggestions[entry.Query] += recencyScore * 2 // User history weighted 2x
			}
		}
	}

	// Get popular queries
	popularQueries, err := r.GetPopularSearchQueries(ctx, 100, 7*24*time.Hour) // Last 7 days
	if err == nil {
		for _, stat := range popularQueries {
			if strings.HasPrefix(strings.ToLower(stat.Query), normalizedQuery) {
				// Popular queries scored by frequency and recency
				age := time.Since(stat.LastUsed)
				recencyScore := 1.0 / (1 + age.Hours()/168) // Decay over weeks
				popularityScore := math.Log1p(float64(stat.Count)) // Logarithmic scaling
				suggestions[stat.Query] += recencyScore * popularityScore
			}
		}
	}

	// Also add some prefix-based suggestions from trending hashtags
	if strings.HasPrefix(normalizedQuery, "#") || len(normalizedQuery) >= 2 {
		hashtags, err := r.GetRecentHashtags(ctx, time.Now().Add(-7*24*time.Hour), 20)
		if err == nil {
			for _, hashtag := range hashtags {
				tagName := strings.ToLower(hashtag.Name)
				if strings.Contains(tagName, normalizedQuery) || strings.Contains(normalizedQuery, "#") {
					// Add hashtag with # prefix if not already present
					suggestion := hashtag.Name
					if !strings.HasPrefix(suggestion, "#") {
						suggestion = "#" + suggestion
					}
					suggestions[suggestion] += float64(hashtag.UsageCount) / 100
				}
			}
		}
	}

	// Convert map to sorted slice
	type scoredSuggestion struct {
		query string
		score float64
	}
	scoredList := make([]scoredSuggestion, 0, len(suggestions))
	for query, score := range suggestions {
		scoredList = append(scoredList, scoredSuggestion{query, score})
	}

	// Sort by score descending
	for i := 0; i < len(scoredList)-1; i++ {
		for j := i + 1; j < len(scoredList); j++ {
			if scoredList[i].score < scoredList[j].score {
				scoredList[i], scoredList[j] = scoredList[j], scoredList[i]
			}
		}
	}

	// Extract top suggestions
	result := make([]string, 0, limit)
	for i := 0; i < len(scoredList) && i < limit; i++ {
		result = append(result, scoredList[i].query)
	}

	return result, nil
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
	
	metrics.UpdateKeys()
	
	err := r.db.WithContext(ctx).Model(metrics).Create()
	if err != nil {
		r.logger.Error("failed to record engagement",
			zap.String("metricType", metricType),
			zap.String("targetID", targetID),
			zap.Error(err))
		return fmt.Errorf("failed to record engagement: %w", err)
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
			return nil, nil // Return nil without error for not found
		}
		r.logger.Error("failed to get engagement metrics",
			zap.String("metricType", metricType),
			zap.String("targetID", targetID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get engagement metrics: %w", err)
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
	// Query using GSI8 for date range
	startPK := fmt.Sprintf("METRICS#%s#%s", metricType, startDate)
	endPK := fmt.Sprintf("METRICS#%s#%s", metricType, endDate)
	
	var metricsRecords []models.EngagementMetrics
	err := r.db.WithContext(ctx).Model(&models.EngagementMetrics{}).
		Where("GSI8PK", ">=", startPK).
		Where("GSI8PK", "<=", endPK).
		OrderBy("GSI8PK", "ASC").
		Limit(limit).
		All(&metricsRecords)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.EngagementMetricsSummary{}, nil
		}
		r.logger.Error("failed to get engagement by date range",
			zap.String("metricType", metricType),
			zap.String("startDate", startDate),
			zap.String("endDate", endDate),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get engagement by date range: %w", err)
	}
	
	// Convert to summary format
	summaries := make([]*storage.EngagementMetricsSummary, len(metricsRecords))
	for i, record := range metricsRecords {
		summaries[i] = &storage.EngagementMetricsSummary{
			Date:        record.Date,
			MetricType:  record.MetricType,
			TargetID:    record.TargetID,
			TotalViews:  record.Views,
			TotalLikes:  record.Likes,
			TotalShares: record.Shares,
			UniqueUsers: record.UniqueUsers,
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
		return nil, fmt.Errorf("failed to get top engaged content: %w", err)
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
	if len(rankings) > limit {
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
	
	trending.UpdateKeys()
	
	err := r.db.WithContext(ctx).Model(trending).Create()
	if err != nil {
		r.logger.Error("failed to update trending hashtag",
			zap.String("hashtag", hashtag),
			zap.String("date", date),
			zap.Error(err))
		return fmt.Errorf("failed to update trending hashtag: %w", err)
	}
	
	return nil
}

// GetTrendingHashtagsForDate retrieves trending hashtags for a specific date
func (r *TrendingRepository) GetTrendingHashtagsForDate(ctx context.Context, date string, limit int) ([]*storage.TrendingHashtagData, error) {
	pk := fmt.Sprintf("TRENDING#%s", date)
	
	var trendingRecords []models.TrendingHashtag
	err := r.db.WithContext(ctx).Model(&models.TrendingHashtag{}).
		Where("PK", "=", pk).
		OrderBy("SK", "DESC"). // Sort by score descending
		Limit(limit).
		All(&trendingRecords)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.TrendingHashtagData{}, nil
		}
		r.logger.Error("failed to get trending hashtags",
			zap.String("date", date),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get trending hashtags: %w", err)
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
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		pk := fmt.Sprintf("TRENDING#%s", date)
		
		var trendingRecords []models.TrendingHashtag
		err := r.db.WithContext(ctx).Model(&models.TrendingHashtag{}).
			Where("PK", "=", pk).
			Filter("SK", "CONTAINS", hashtag).
			All(&trendingRecords)
		
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
func (r *TrendingRepository) PruneStaleTrends(ctx context.Context, before time.Time) error {
	// TTL should handle this automatically, but we can also manually prune
	beforeDate := before.Format("2006-01-02")
	
	// Query for old trends
	var oldTrends []models.TrendingHashtag
	err := r.db.WithContext(ctx).Model(&models.TrendingHashtag{}).
		Where("Date", "<", beforeDate).
		Limit(100). // Process in batches
		All(&oldTrends)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		r.logger.Error("failed to query stale trends", zap.Error(err))
		return fmt.Errorf("failed to query stale trends: %w", err)
	}
	
	// Delete each trend
	for _, trend := range oldTrends {
		err := r.db.WithContext(ctx).Model(&trend).Delete()
		if err != nil {
			r.logger.Warn("failed to delete stale trend",
				zap.String("hashtag", trend.Hashtag),
				zap.String("date", trend.Date),
				zap.Error(err))
		}
	}
	
	r.logger.Info("pruned stale trends",
		zap.Int("count", len(oldTrends)),
		zap.Time("before", before))
	
	return nil
}

// ========== InstanceMetrics Methods ==========

// RecordInstanceMetric records a platform-wide metric
func (r *TrendingRepository) RecordInstanceMetric(ctx context.Context, date, metricType string, value int64) error {
	now := time.Now()
	
	// Get previous value to calculate delta
	var previousValue int64
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
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
	
	metrics.UpdateKeys()
	
	err = r.db.WithContext(ctx).Model(metrics).Create()
	if err != nil {
		r.logger.Error("failed to record instance metric",
			zap.String("metricType", metricType),
			zap.String("date", date),
			zap.Int64("value", value),
			zap.Error(err))
		return fmt.Errorf("failed to record instance metric: %w", err)
	}
	
	return nil
}

// GetInstanceMetrics retrieves instance metrics for a specific date and type
func (r *TrendingRepository) GetInstanceMetrics(ctx context.Context, date, metricType string) (*storage.InstanceMetricData, error) {
	pk := fmt.Sprintf("INSTANCE_METRICS#%s", date)
	sk := fmt.Sprintf("METRIC#%s", metricType)
	
	var metrics models.InstanceMetrics
	err := r.db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&metrics)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		r.logger.Error("failed to get instance metrics",
			zap.String("date", date),
			zap.String("metricType", metricType),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get instance metrics: %w", err)
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
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
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
		return nil, fmt.Errorf("failed to get start metric: %w", err)
	}
	
	endMetric, err := r.GetInstanceMetrics(ctx, endDate, metricType)
	if err != nil || endMetric == nil {
		return nil, fmt.Errorf("failed to get end metric: %w", err)
	}
	
	// Calculate growth rate
	var growthRate float64
	if startMetric.Value > 0 {
		growthRate = float64(endMetric.Value-startMetric.Value) / float64(startMetric.Value) * 100
	}
	
	return &storage.GrowthRate{
		MetricType:  metricType,
		StartDate:   startDate,
		EndDate:     endDate,
		StartValue:  startMetric.Value,
		EndValue:    endMetric.Value,
		GrowthRate:  growthRate,
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
		return fmt.Errorf("failed to record manifest generation: %w", err)
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
		return fmt.Errorf("failed to record quality change: %w", err)
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
		return fmt.Errorf("failed to record media event: %w", err)
	}
	
	return nil
}

// GetManifestGenerationStats retrieves manifest generation statistics for a date range
func (r *TrendingRepository) GetManifestGenerationStats(ctx context.Context, format, startDate, endDate string) (map[string]int64, error) {
	stats := make(map[string]int64)
	
	// Query manifest generation records
	var analytics []models.MediaAnalytics
	err := r.db.WithContext(ctx).Model(&models.MediaAnalytics{}).
		Where("PK", "=", fmt.Sprintf("MANIFEST#%s", format)).
		Where("Date", ">=", startDate).
		Where("Date", "<=", endDate).
		All(&analytics)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return stats, nil
		}
		r.logger.Error("failed to get manifest generation stats",
			zap.String("format", format),
			zap.String("startDate", startDate),
			zap.String("endDate", endDate),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get manifest generation stats: %w", err)
	}
	
	// Count by date
	for _, record := range analytics {
		stats[record.Date]++
	}
	
	return stats, nil
}

// GetMediaEventStats retrieves general media event statistics
func (r *TrendingRepository) GetMediaEventStats(ctx context.Context, eventType, startDate, endDate string) (map[string]int64, error) {
	stats := make(map[string]int64)
	
	// Query media event records
	var analytics []models.MediaAnalytics
	err := r.db.WithContext(ctx).Model(&models.MediaAnalytics{}).
		Where("PK", "=", fmt.Sprintf("MEDIA_EVENT#%s", eventType)).
		Where("Date", ">=", startDate).
		Where("Date", "<=", endDate).
		All(&analytics)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return stats, nil
		}
		r.logger.Error("failed to get media event stats",
			zap.String("eventType", eventType),
			zap.String("startDate", startDate),
			zap.String("endDate", endDate),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get media event stats: %w", err)
	}
	
	// Count by date
	for _, record := range analytics {
		stats[record.Date]++
	}
	
	return stats, nil
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
		return fmt.Errorf("failed to get existing moderation analytics: %w", err)
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
	
	analytics.UpdateKeys()
	
	// Use Create to save (DynamORM doesn't have Save/Upsert)
	err = r.db.WithContext(ctx).Model(analytics).Create()
	if err != nil {
		r.logger.Error("failed to record moderation action",
			zap.String("date", date),
			zap.String("reportType", reportType),
			zap.Error(err))
		return fmt.Errorf("failed to record moderation action: %w", err)
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
			return nil, nil
		}
		r.logger.Error("failed to get moderation analytics",
			zap.String("date", date),
			zap.String("reportType", reportType),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get moderation analytics: %w", err)
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
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
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
			date := now.AddDate(0, 0, -i).Format("2006-01-02")
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