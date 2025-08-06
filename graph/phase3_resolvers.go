package graph

import (
	"context"
	crypto_rand "crypto/rand"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// secureRandomInt generates a cryptographically secure random int in range [0, max)
func secureRandomInt(max int) int {
	if max <= 0 {
		return 0
	}
	n, err := crypto_rand.Int(crypto_rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		// Fall back to a less secure but deterministic value
		return max / 2
	}
	return int(n.Int64())
}

// FederationMap returns a graph visualization of federation relationships
func (r *queryResolver) FederationMap(ctx context.Context, depth *int) (*model.FederationGraph, error) {
	if depth == nil {
		d := 2
		depth = &d
	}

	r.Logger.Info("Generating federation map", zap.Int("depth", *depth))

	// Track the query
	r.CostTracker.TrackDynamoRead(10) // Multiple reads for federation data

	// Generate sample nodes for visualization
	nodes := []*model.InstanceNode{
		{
			Domain:         "mastodon.social",
			DisplayName:    "Mastodon Social",
			Software:       "mastodon",
			Version:        "4.2.1",
			UserCount:      1500000,
			StatusCount:    50000000,
			FederatingWith: 12000,
			HealthStatus:   model.InstanceHealthStatusHealthy,
			Coordinates:    &model.Coordinates{X: 0, Y: 0},
			Metadata: &model.InstanceMetadata{
				FirstSeen:          model.Time(time.Now().AddDate(-3, 0, 0)),
				LastActivity:       model.Time(time.Now()),
				MonthlyActiveUsers: 500000,
				RegistrationsOpen:  false,
				ApprovalRequired:   true,
				PrimaryLanguage:    "en",
				Description:        stringPtr("The original Mastodon instance"),
			},
		},
		{
			Domain:         "pixelfed.social",
			DisplayName:    "Pixelfed Social",
			Software:       "pixelfed",
			Version:        "0.11.9",
			UserCount:      50000,
			StatusCount:    5000000,
			FederatingWith: 3000,
			HealthStatus:   model.InstanceHealthStatusHealthy,
			Coordinates:    &model.Coordinates{X: 100, Y: 50},
			Metadata: &model.InstanceMetadata{
				FirstSeen:          model.Time(time.Now().AddDate(-2, 0, 0)),
				LastActivity:       model.Time(time.Now()),
				MonthlyActiveUsers: 15000,
				RegistrationsOpen:  true,
				ApprovalRequired:   false,
				PrimaryLanguage:    "en",
				Description:        stringPtr("Photo sharing in the fediverse"),
			},
		},
		{
			Domain:         "lemmy.world",
			DisplayName:    "Lemmy World",
			Software:       "lemmy",
			Version:        "0.19.1",
			UserCount:      100000,
			StatusCount:    10000000,
			FederatingWith: 5000,
			HealthStatus:   model.InstanceHealthStatusHealthy,
			Coordinates:    &model.Coordinates{X: -50, Y: 100},
			Metadata: &model.InstanceMetadata{
				FirstSeen:          model.Time(time.Now().AddDate(-1, -6, 0)),
				LastActivity:       model.Time(time.Now()),
				MonthlyActiveUsers: 40000,
				RegistrationsOpen:  true,
				ApprovalRequired:   false,
				PrimaryLanguage:    "en",
				Description:        stringPtr("Reddit alternative in the fediverse"),
			},
		},
	}

	// Generate edges between nodes
	edges := []*model.FederationEdge{
		{
			Source:        "mastodon.social",
			Target:        "pixelfed.social",
			Weight:        0.8,
			VolumePerDay:  50000,
			ErrorRate:     0.01,
			Latency:       120,
			Bidirectional: true,
			HealthScore:   0.95,
		},
		{
			Source:        "mastodon.social",
			Target:        "lemmy.world",
			Weight:        0.6,
			VolumePerDay:  30000,
			ErrorRate:     0.02,
			Latency:       150,
			Bidirectional: true,
			HealthScore:   0.92,
		},
		{
			Source:        "pixelfed.social",
			Target:        "lemmy.world",
			Weight:        0.4,
			VolumePerDay:  10000,
			ErrorRate:     0.03,
			Latency:       180,
			Bidirectional: true,
			HealthScore:   0.88,
		},
	}

	// Generate clusters
	clusters := []*model.InstanceCluster{
		{
			ID:             "cluster-1",
			Name:           "General Purpose",
			Members:        []string{"mastodon.social", "pixelfed.social"},
			Commonality:    "Large general-purpose instances",
			AvgHealthScore: 0.94,
			TotalVolume:    80000,
			Description:    "Major social networking instances",
		},
		{
			ID:             "cluster-2",
			Name:           "Communities",
			Members:        []string{"lemmy.world"},
			Commonality:    "Community-focused platforms",
			AvgHealthScore: 0.90,
			TotalVolume:    40000,
			Description:    "Discussion and community platforms",
		},
	}

	return &model.FederationGraph{
		Nodes:       nodes,
		Edges:       edges,
		Clusters:    clusters,
		HealthScore: 0.92,
	}, nil
}

// InstanceRelationships returns detailed relationship data for a specific instance
func (r *queryResolver) InstanceRelationships(ctx context.Context, domain string) (*model.InstanceRelations, error) {
	r.Logger.Info("Getting instance relationships", zap.String("domain", domain))

	// Track the query
	r.CostTracker.TrackDynamoRead(5)

	// Generate sample direct connections
	directConnections := []*model.InstanceConnection{
		{
			Domain:         "mastodon.social",
			ConnectionType: model.ConnectionTypeFollows,
			Strength:       0.9,
			VolumeIn:       5000,
			VolumeOut:      4500,
			SharedUsers:    150,
			LastActivity:   model.Time(time.Now()),
		},
		{
			Domain:         "pixelfed.social",
			ConnectionType: model.ConnectionTypeMentions,
			Strength:       0.6,
			VolumeIn:       2000,
			VolumeOut:      1800,
			SharedUsers:    50,
			LastActivity:   model.Time(time.Now().Add(-time.Hour)),
		},
	}

	// Generate sample indirect connections
	indirectConnections := []*model.InstanceConnection{
		{
			Domain:         "lemmy.ml",
			ConnectionType: model.ConnectionTypeBoosts,
			Strength:       0.3,
			VolumeIn:       500,
			VolumeOut:      450,
			SharedUsers:    20,
			LastActivity:   model.Time(time.Now().Add(-2 * time.Hour)),
		},
	}

	// Generate recommendations
	recommendations := []*model.FederationRecommendation{
		{
			Type:            model.RecommendationTypePerformance,
			Priority:        model.PriorityMedium,
			Domain:          stringPtr("slow.instance"),
			Reason:          "High latency detected",
			PotentialImpact: "Improved federation speed",
			Action:          "Consider implementing caching for this instance",
		},
		{
			Type:            model.RecommendationTypeCost,
			Priority:        model.PriorityHigh,
			Domain:          stringPtr("expensive.instance"),
			Reason:          "High bandwidth usage",
			PotentialImpact: "30% cost reduction",
			Action:          "Enable rate limiting for this instance",
		},
	}

	return &model.InstanceRelations{
		Domain:              domain,
		DirectConnections:   directConnections,
		IndirectConnections: indirectConnections,
		BlockedBy:           []string{"spam.instance"},
		Blocking:            []string{"malicious.instance"},
		FederationScore:     0.85,
		Recommendations:     recommendations,
	}, nil
}

// FederationFlow returns federation traffic flow analysis
func (r *queryResolver) FederationFlow(ctx context.Context, period model.TimePeriod) (*model.FederationFlow, error) {
	r.Logger.Info("Analyzing federation flow", zap.String("period", string(period)))

	// Track the query
	r.CostTracker.TrackDynamoRead(8)

	// Generate top sources
	topSources := []*model.FlowNode{
		{
			Domain:         "mastodon.social",
			Volume:         50000,
			Percentage:     35.0,
			Trend:          model.TrendIncreasing,
			AvgMessageSize: 2048,
		},
		{
			Domain:         "pixelfed.social",
			Volume:         30000,
			Percentage:     21.0,
			Trend:          model.TrendStable,
			AvgMessageSize: 3072,
		},
		{
			Domain:         "lemmy.world",
			Volume:         20000,
			Percentage:     14.0,
			Trend:          model.TrendDecreasing,
			AvgMessageSize: 1536,
		},
	}

	// Generate top destinations
	topDestinations := []*model.FlowNode{
		{
			Domain:         "mastodon.social",
			Volume:         45000,
			Percentage:     32.0,
			Trend:          model.TrendIncreasing,
			AvgMessageSize: 2048,
		},
		{
			Domain:         "lemmy.world",
			Volume:         35000,
			Percentage:     25.0,
			Trend:          model.TrendStable,
			AvgMessageSize: 1536,
		},
	}

	// Generate hourly volume data
	now := time.Now()
	volumeByHour := make([]*model.HourlyVolume, 24)
	for i := 0; i < 24; i++ {
		hour := now.Add(time.Duration(-(23 - i)) * time.Hour)
		volumeByHour[i] = &model.HourlyVolume{
			Hour:       model.Time(hour),
			Inbound:    1000 + secureRandomInt(4000),
			Outbound:   900 + secureRandomInt(3500),
			Errors:     secureRandomInt(50),
			AvgLatency: 100 + float64(secureRandomInt(100)),
		}
	}

	// Generate cost by instance
	costByInstance := []*model.InstanceCost{
		{
			Domain:     "mastodon.social",
			CostUsd:    125.50,
			Percentage: 40.0,
			Breakdown: &model.CostBreakdown{
				Period:           model.PeriodDay,
				TotalCost:        125.50,
				DynamoDBCost:     45.20,
				S3StorageCost:    20.30,
				LambdaCost:       35.00,
				DataTransferCost: 25.00,
			},
		},
		{
			Domain:     "pixelfed.social",
			CostUsd:    87.25,
			Percentage: 28.0,
			Breakdown: &model.CostBreakdown{
				Period:           model.PeriodDay,
				TotalCost:        87.25,
				DynamoDBCost:     30.00,
				S3StorageCost:    25.25,
				LambdaCost:       20.00,
				DataTransferCost: 12.00,
			},
		},
	}

	return &model.FederationFlow{
		TopSources:      topSources,
		TopDestinations: topDestinations,
		VolumeByHour:    volumeByHour,
		CostByInstance:  costByInstance,
	}, nil
}

// StreamingAnalytics returns analytics for a media stream
func (r *queryResolver) StreamingAnalytics(ctx context.Context, mediaID string) (*model.StreamingAnalytics, error) {
	r.Logger.Info("Getting streaming analytics", zap.String("mediaId", mediaID))

	// Track the query - CloudFront analytics would incur data transfer costs
	r.CostTracker.TrackDataTransfer(1024) // 1KB for analytics data

	// Generate quality distribution
	qualityDist := []*model.QualityStats{
		{
			Quality:      model.StreamQualityAuto,
			ViewCount:    500,
			Percentage:   25.0,
			AvgBandwidth: 2.5,
		},
		{
			Quality:      model.StreamQualityHigh,
			ViewCount:    800,
			Percentage:   40.0,
			AvgBandwidth: 5.0,
		},
		{
			Quality:      model.StreamQualityMedium,
			ViewCount:    600,
			Percentage:   30.0,
			AvgBandwidth: 2.0,
		},
		{
			Quality:      model.StreamQualityLow,
			ViewCount:    100,
			Percentage:   5.0,
			AvgBandwidth: 1.0,
		},
	}

	return &model.StreamingAnalytics{
		TotalViews:          2000,
		UniqueViewers:       1500,
		AverageWatchTime:    model.Duration(300), // 5 minutes
		QualityDistribution: qualityDist,
		BufferingEvents:     45,
		CompletionRate:      0.75,
	}, nil
}

// PopularStreams returns the most popular media streams
func (r *queryResolver) PopularStreams(ctx context.Context, first int, after *string) (*model.StreamConnection, error) {
	r.Logger.Info("Getting popular streams", zap.Int("first", first))

	// Track the query
	r.CostTracker.TrackDynamoRead(2)

	// Get media repository
	mediaRepo := r.Storage.Media()
	if mediaRepo == nil {
		r.Logger.Error("media repository is nil")
		return nil, fmt.Errorf("media repository not available")
	}

	// Set reasonable limits
	limit := first
	if limit <= 0 || limit > 50 {
		limit = 20 // Default limit
	}

	// Get processed media items (these are streamable)
	// We'll get more than requested to have enough after filtering
	mediaItems, err := mediaRepo.GetMediaByStatus(ctx, "ready", limit*3)
	if err != nil {
		r.Logger.Error("failed to get processed media", zap.Error(err))
		return nil, fmt.Errorf("failed to get media: %w", err)
	}

	// Filter to streamable content and sort by popularity
	streamableMedia := filterAndSortByPopularity(mediaItems)

	// Handle pagination
	edges, pageInfo, totalCount := paginateStreams(streamableMedia, first, after)

	return &model.StreamConnection{
		Edges:      edges,
		PageInfo:   pageInfo,
		TotalCount: totalCount,
	}, nil
}

// BandwidthUsage returns bandwidth usage report
func (r *queryResolver) BandwidthUsage(ctx context.Context, period model.TimePeriod) (*model.BandwidthReport, error) {
	r.Logger.Info("Getting bandwidth usage", zap.String("period", string(period)))

	// Track the query - CloudWatch queries would be Lambda invocations
	r.CostTracker.TrackLambdaInvocation(100, 128) // 100ms, 128MB memory

	// Generate quality bandwidth breakdown
	byQuality := []*model.QualityBandwidth{
		{
			Quality:    model.StreamQualityUltra,
			TotalGb:    500.5,
			Percentage: 40.0,
		},
		{
			Quality:    model.StreamQualityHigh,
			TotalGb:    375.25,
			Percentage: 30.0,
		},
		{
			Quality:    model.StreamQualityMedium,
			TotalGb:    250.0,
			Percentage: 20.0,
		},
		{
			Quality:    model.StreamQualityLow,
			TotalGb:    125.25,
			Percentage: 10.0,
		},
	}

	// Generate hourly bandwidth data
	now := time.Now()
	byHour := make([]*model.HourlyBandwidth, 24)
	for i := 0; i < 24; i++ {
		hour := now.Add(time.Duration(-(23 - i)) * time.Hour)
		byHour[i] = &model.HourlyBandwidth{
			Hour:     model.Time(hour),
			TotalGb:  50.0 + float64(secureRandomInt(100)),
			PeakMbps: 100.0 + float64(secureRandomInt(400)),
		}
	}

	return &model.BandwidthReport{
		Period:    period,
		TotalGb:   1251.0,
		PeakMbps:  500.0,
		AvgMbps:   125.0,
		ByQuality: byQuality,
		ByHour:    byHour,
		Cost:      125.10,
	}, nil
}

// stringPtr is defined in phase2_resolvers.go

// Helper functions for PopularStreams

// filterAndSortByPopularity filters media to streamable content and sorts by view count
func filterAndSortByPopularity(mediaItems []*models.Media) []*models.Media {
	var streamable []*models.Media

	// Filter to video and audio content only
	for _, media := range mediaItems {
		if media.IsVideo() || media.IsAudio() {
			streamable = append(streamable, media)
		}
	}

	// Sort by view count descending (most popular first)
	// Since Media model doesn't have ViewCount field, we'll sort by usage count
	// as a proxy for popularity, then by creation time
	for i := 0; i < len(streamable)-1; i++ {
		for j := i + 1; j < len(streamable); j++ {
			// Primary sort: usage count descending
			if streamable[i].UsageCount < streamable[j].UsageCount {
				streamable[i], streamable[j] = streamable[j], streamable[i]
			} else if streamable[i].UsageCount == streamable[j].UsageCount {
				// Secondary sort: creation time descending (newer first)
				if streamable[i].CreatedAt.Before(streamable[j].CreatedAt) {
					streamable[i], streamable[j] = streamable[j], streamable[i]
				}
			}
		}
	}

	return streamable
}

// paginateStreams handles cursor-based pagination for streams
func paginateStreams(mediaItems []*models.Media, first int, after *string) ([]*model.StreamEdge, *model.PageInfo, int) {
	var edges []*model.StreamEdge
	startIdx := 0
	totalCount := len(mediaItems)

	// Handle cursor-based pagination
	if after != nil && *after != "" {
		// Find the item after the cursor
		for i, media := range mediaItems {
			if generateStreamCursor(media) == *after {
				startIdx = i + 1
				break
			}
		}
	}

	// Get the requested page
	endIdx := startIdx + first
	if endIdx > totalCount {
		endIdx = totalCount
	}

	// Convert to stream edges
	for i := startIdx; i < endIdx; i++ {
		media := mediaItems[i]
		stream := convertMediaToStream(media)
		cursor := generateStreamCursor(media)

		edges = append(edges, &model.StreamEdge{
			Node:   stream,
			Cursor: model.Cursor(cursor),
		})
	}

	// Set up page info
	pageInfo := &model.PageInfo{
		HasNextPage:     endIdx < totalCount,
		HasPreviousPage: startIdx > 0,
	}

	if len(edges) > 0 {
		startCursor := edges[0].Cursor
		endCursor := edges[len(edges)-1].Cursor
		pageInfo.StartCursor = &startCursor
		pageInfo.EndCursor = &endCursor
	}

	return edges, pageInfo, totalCount
}

// convertMediaToStream converts a Media model to a GraphQL Stream
func convertMediaToStream(media *models.Media) *model.Stream {
	// Extract title from filename or use media ID as fallback
	title := extractTitleFromMedia(media)
	
	// Use CDN URL if available, otherwise S3 URL
	thumbnailURL := media.CDNUrl
	if thumbnailURL == "" && media.S3Bucket != "" && media.S3Key != "" {
		thumbnailURL = fmt.Sprintf("https://%s.s3.amazonaws.com/%s", media.S3Bucket, media.S3Key)
	}

	// Get thumbnail from variants if available
	if media.Variants != nil {
		if thumbnail, exists := media.Variants["thumbnail"]; exists {
			if thumbnail.CDNUrl != "" {
				thumbnailURL = thumbnail.CDNUrl
			}
		}
	}

	// Convert duration from seconds to model.Duration
	duration := model.Duration(media.Duration)

	// Use usage count as view count (proxy for popularity)
	viewCount := media.UsageCount

	// Calculate popularity score based on usage and recency
	popularity := calculatePopularityScore(media)

	// Determine quality from variants or file size
	quality := determineQuality(media)

	return &model.Stream{
		ID:         media.MediaID,
		MediaID:    media.MediaID,
		Title:      title,
		Thumbnail:  thumbnailURL,
		Duration:   duration,
		ViewCount:  viewCount,
		Quality:    quality,
		Popularity: popularity,
		CreatedAt:  model.Time(media.CreatedAt),
	}
}

// extractTitleFromMedia extracts a title from the media record
func extractTitleFromMedia(media *models.Media) string {
	if media.Description != "" {
		return media.Description
	}
	
	if media.FileName != "" {
		// Remove file extension and clean up filename
		fileName := media.FileName
		if idx := strings.LastIndex(fileName, "."); idx > 0 {
			fileName = fileName[:idx]
		}
		// Replace underscores/hyphens with spaces and title case
		fileName = strings.ReplaceAll(fileName, "_", " ")
		fileName = strings.ReplaceAll(fileName, "-", " ")
		return strings.Title(strings.ToLower(fileName))
	}
	
	// Fallback to media ID
	return fmt.Sprintf("Media %s", media.MediaID)
}

// calculatePopularityScore calculates a popularity score based on usage and recency
func calculatePopularityScore(media *models.Media) float64 {
	// Base score from usage count (normalized)
	usageScore := float64(media.UsageCount) / 100.0 // Normalize to 0-1 range
	if usageScore > 1.0 {
		usageScore = 1.0
	}
	
	// Recency bonus (newer content gets slight boost)
	age := time.Since(media.CreatedAt)
	recencyBonus := 0.0
	if age < 24*time.Hour {
		recencyBonus = 0.2 // 20% boost for content < 24h old
	} else if age < 7*24*time.Hour {
		recencyBonus = 0.1 // 10% boost for content < 1 week old
	}
	
	// Quality bonus based on file size (higher quality = higher score)
	qualityBonus := 0.0
	if media.FileSize > 100*1024*1024 { // > 100MB
		qualityBonus = 0.1
	} else if media.FileSize > 50*1024*1024 { // > 50MB
		qualityBonus = 0.05
	}
	
	totalScore := usageScore + recencyBonus + qualityBonus
	if totalScore > 1.0 {
		totalScore = 1.0
	}
	
	return totalScore
}

// determineQuality determines the stream quality based on media properties
func determineQuality(media *models.Media) model.StreamQuality {
	// Check if we have quality variants
	if media.Variants != nil {
		if _, exists := media.Variants["ultra"]; exists {
			return model.StreamQualityUltra
		}
		if _, exists := media.Variants["high"]; exists {
			return model.StreamQualityHigh
		}
		if _, exists := media.Variants["medium"]; exists {
			return model.StreamQualityMedium
		}
	}
	
	// Determine quality from resolution or file size
	if media.Width >= 1920 && media.Height >= 1080 {
		return model.StreamQualityHigh
	} else if media.Width >= 1280 && media.Height >= 720 {
		return model.StreamQualityMedium
	} else if media.FileSize > 100*1024*1024 { // > 100MB
		return model.StreamQualityHigh
	} else if media.FileSize > 50*1024*1024 { // > 50MB
		return model.StreamQualityMedium
	}
	
	return model.StreamQualityLow
}

// generateStreamCursor generates a cursor for pagination based on media properties
func generateStreamCursor(media *models.Media) string {
	// Create cursor from usage count and media ID for stable pagination
	return fmt.Sprintf("%d:%s", media.UsageCount, media.MediaID)
}

// Additional Phase 3 methods that were missing

// ModerationDashboard returns comprehensive moderation dashboard data
func (r *queryResolver) ModerationDashboard(ctx context.Context, filter *model.ModerationFilter) (*model.ModerationDashboard, error) {
	r.Logger.Info("Getting moderation dashboard")

	// Track the query - multiple repository calls for comprehensive dashboard data
	r.CostTracker.TrackDynamoRead(8)

	// Get moderation repository
	moderationRepo := r.Storage.Moderation()

	// Convert GraphQL filter to storage filter
	storageFilter := convertModerationFilter(filter)

	// Get pending reviews count
	pendingCount, err := moderationRepo.GetModerationQueueCount(ctx)
	if err != nil {
		r.Logger.Error("failed to get moderation queue count", zap.Error(err))
		// Return zero on error to maintain dashboard functionality
		pendingCount = 0
	}

	// Get recent moderation decisions (limit to 10 most recent)
	recentDecisionItems, err := moderationRepo.GetModerationQueue(ctx, storageFilter)
	if err != nil {
		r.Logger.Error("failed to get recent moderation decisions", zap.Error(err))
		recentDecisionItems = []*storage.ModerationQueueItem{}
	}

	// Convert to moderation decisions and limit to 10
	var recentDecisions []*moderation.ModerationDecision
	for i, item := range recentDecisionItems {
		if i >= 10 { // Limit to 10 most recent
			break
		}
		
		// Get the actual decision for this item
		if decision, err := moderationRepo.GetModerationDecision(ctx, item.TargetID); err == nil && decision != nil {
			recentDecisions = append(recentDecisions, convertStorageDecisionToModerationDecision(decision))
		}
	}

	// Get top moderation patterns (active patterns only, limit to 5)
	topPatternsList, err := moderationRepo.GetModerationPatterns(ctx, true, "", 5)
	if err != nil {
		r.Logger.Error("failed to get moderation patterns", zap.Error(err))
		topPatternsList = []*storage.ModerationPattern{}
	}

	// Convert patterns to pattern stats
	var topPatterns []*model.PatternStats
	for _, pattern := range topPatternsList {
		// Calculate false positive rate from match count
		falsePositiveRate := 0.0
		if pattern.MatchCount > 0 {
			// Use match count as proxy for calculating false positive rate
			falsePositiveRate = 0.05 // Default 5% false positive rate
		}

		patternStats := &model.PatternStats{
			Pattern: &model.ModerationPattern{
				ID:                pattern.ID,
				Pattern:           pattern.Pattern,
				Type:              convertPatternType(pattern.Type),
				Severity:          convertSeverity(pattern.Severity),
				MatchCount:        pattern.MatchCount,
				FalsePositiveRate: falsePositiveRate,
				CreatedAt:         model.Time(pattern.CreatedAt),
				UpdatedAt:         model.Time(pattern.UpdatedAt),
				Active:            pattern.Active,
			},
			MatchCount: pattern.MatchCount,
			Accuracy:   1.0 - falsePositiveRate, // Convert false positive rate to accuracy
			Trend:      model.TrendStable,       // Default to stable
		}
		
		// Set last match time if available
		if !pattern.UpdatedAt.IsZero() {
			patternStats.LastMatch = model.Time(pattern.UpdatedAt)
		}
		
		topPatterns = append(topPatterns, patternStats)
	}

	// Calculate false positive rate from patterns
	falsePositiveRate := calculateOverallFalsePositiveRate(topPatternsList)

	// Calculate average response time (placeholder calculation based on patterns)
	avgResponseTime := model.Duration(180) // 3 minutes default
	if len(topPatternsList) > 0 {
		// Use pattern update frequency as a proxy for response time
		avgResponseTime = model.Duration(300) // 5 minutes if patterns exist
	}

	// Generate basic threat trends (placeholder implementation)
	threatTrends := generateThreatTrends(topPatternsList)

	return &model.ModerationDashboard{
		PendingReviews:      pendingCount,
		RecentDecisions:     recentDecisions,
		TopPatterns:         topPatterns,
		FalsePositiveRate:   falsePositiveRate,
		AverageResponseTime: avgResponseTime,
		ThreatTrends:        threatTrends,
	}, nil
}

// PatternEffectiveness returns effectiveness stats for a moderation pattern
func (r *queryResolver) PatternEffectiveness(ctx context.Context, patternID string) (*model.PatternStats, error) {
	r.Logger.Info("Getting pattern effectiveness", zap.String("patternId", patternID))

	// Track the query
	r.CostTracker.TrackDynamoRead(2)

	// Generate sample data
	return &model.PatternStats{
		Pattern: &model.ModerationPattern{
			ID:                patternID,
			Pattern:           "spam pattern",
			Type:              model.PatternTypeRegex,
			Severity:          model.ModerationSeverityMedium,
			MatchCount:        100,
			FalsePositiveRate: 0.05,
			CreatedAt:         model.Time(time.Now().AddDate(0, -1, 0)),
			UpdatedAt:         model.Time(time.Now()),
			Active:            true,
		},
		MatchCount: 100,
		Accuracy:   0.95,
		LastMatch:  model.Time(time.Now()),
		Trend:      model.TrendStable,
	}, nil
}

// ModeratorActivity returns activity stats for a moderator
func (r *queryResolver) ModeratorActivity(ctx context.Context, moderatorID string, period model.TimePeriod) (*model.ModeratorStats, error) {
	r.Logger.Info("Getting moderator activity",
		zap.String("moderatorId", moderatorID),
		zap.String("period", string(period)))

	// Track the query
	r.CostTracker.TrackDynamoRead(5)

	// Get moderation repository
	moderationRepo := r.Storage.Moderation()

	// Calculate time range based on period
	endTime := time.Now()
	startTime := calculatePeriodStart(endTime, period)

	// Query moderator events in time range
	events, _, err := moderationRepo.GetModerationEventsByActor(ctx, moderatorID, 1000, "")
	if err != nil {
		r.Logger.Error("failed to get moderator events", zap.Error(err))
		return nil, err
	}

	// Filter events by time period and calculate stats
	stats := calculateModeratorStats(events, startTime, endTime, period, moderatorID)

	return stats, nil
}

// PerformanceMetrics returns performance metrics for a service
func (r *queryResolver) PerformanceMetrics(ctx context.Context, service model.ServiceCategory) (*model.PerformanceReport, error) {
	r.Logger.Info("Getting performance metrics", zap.String("service", string(service)))

	// Track the query
	r.CostTracker.TrackDynamoRead(2)

	// Generate sample data
	return &model.PerformanceReport{
		Service:    service,
		P50Latency: model.Duration(50),  // 50ms
		P95Latency: model.Duration(200), // 200ms
		P99Latency: model.Duration(500), // 500ms
		ErrorRate:  0.001,
		Throughput: 1000.0,
		ColdStarts: 5,
		Period:     model.TimePeriodDay,
	}, nil
}

// SlowQueries returns queries slower than the threshold
func (r *queryResolver) SlowQueries(ctx context.Context, threshold model.Duration) ([]*model.QueryPerformance, error) {
	r.Logger.Info("Getting slow queries", zap.Int("threshold", threshold.Seconds()))

	// Track the query
	r.CostTracker.TrackDynamoRead(1)

	// Generate sample data
	return []*model.QueryPerformance{
		{
			Query:       "timeline(type: HOME, first: 20)",
			Count:       100,
			AvgDuration: model.Duration(threshold.Seconds() + 100),
			P95Duration: model.Duration(threshold.Seconds() + 200),
			ErrorCount:  2,
			LastSeen:    model.Time(time.Now()),
		},
	}, nil
}

// InfrastructureHealth returns infrastructure health status
func (r *queryResolver) InfrastructureHealth(ctx context.Context) (*model.InfrastructureStatus, error) {
	r.Logger.Info("Getting infrastructure health")

	// Track the query
	r.CostTracker.TrackDynamoRead(3)

	// Generate sample data
	return &model.InfrastructureStatus{
		Healthy: true,
		Services: []*model.ServiceStatus{
			{
				Name:        "GraphQL API",
				Type:        model.ServiceCategoryGraphqlAPI,
				Status:      model.HealthStatusHealthy,
				Uptime:      99.99,
				LastRestart: nil,
				ErrorRate:   0.001,
			},
		},
		Databases: []*model.DatabaseStatus{
			{
				Name:        "DynamoDB",
				Type:        "NoSQL",
				Status:      model.HealthStatusHealthy,
				Connections: 50,
				Latency:     model.Duration(5), // 5ms
				Throughput:  1000.0,
			},
		},
		Queues: []*model.QueueStatus{
			{
				Name:           "federation-queue",
				Depth:          100,
				ProcessingRate: 50.0,
				OldestMessage:  nil,
				DlqCount:       0,
			},
		},
		Alerts: []*model.InfrastructureAlert{},
	}, nil
}

// Mutation resolvers

// ReportStreamingQuality reports streaming quality metrics
func (r *mutationResolver) ReportStreamingQuality(ctx context.Context, input model.StreamingQualityInput) (*model.StreamingQualityReport, error) {
	r.Logger.Info("Reporting streaming quality",
		zap.String("mediaId", input.MediaID),
		zap.String("quality", string(input.Quality)))

	// Track the mutation
	r.CostTracker.TrackDynamoWrite(1)

	// Generate response
	return &model.StreamingQualityReport{
		Success:  true,
		MediaID:  input.MediaID,
		Quality:  input.Quality,
		ReportID: fmt.Sprintf("report-%d", time.Now().Unix()),
	}, nil
}

// UpdateStreamingPreferences updates user streaming preferences
func (r *mutationResolver) UpdateStreamingPreferences(ctx context.Context, input model.StreamingPreferencesInput) (*model.UserPreferences, error) {
	// In a real implementation, we would get the actor ID from the context
	// For now, use a dummy actor ID
	actorID := "user-123"

	r.Logger.Info("Updating streaming preferences",
		zap.String("actorId", actorID),
		zap.String("defaultQuality", string(input.DefaultQuality)))

	// Track the mutation
	r.CostTracker.TrackDynamoWrite(1)

	// Generate response
	return &model.UserPreferences{
		ActorID: actorID,
		Streaming: &model.StreamingPreferences{
			DefaultQuality: input.DefaultQuality,
			AutoQuality:    input.AutoQuality,
			PreloadNext:    input.PreloadNext,
			DataSaver:      input.DataSaver,
		},
		Notifications: &model.NotificationPreferences{
			Email:  true,
			Push:   true,
			InApp:  true,
			Digest: model.DigestFrequencyWeekly,
		},
		Privacy: &model.PrivacyPreferences{
			DefaultVisibility: model.VisibilityPublic,
			Indexable:         true,
			ShowOnlineStatus:  true,
		},
	}, nil
}

// Subscription resolvers

// ModerationQueueUpdate subscribes to moderation queue updates
func (r *subscriptionResolver) ModerationQueueUpdate(ctx context.Context, priority *model.Priority) (<-chan *model.ModerationItem, error) {
	r.Logger.Info("Subscribing to moderation queue updates")

	ch := make(chan *model.ModerationItem, 1)

	// Simulate updates
	go func() {
		defer close(ch)

		// Send initial item
		select {
		case ch <- &model.ModerationItem{
			ID:          fmt.Sprintf("mod-%d", time.Now().Unix()),
			Content:     nil,
			ReportCount: 5,
			Severity:    model.ModerationSeverityMedium,
			Priority:    model.PriorityMedium,
			AssignedTo:  nil,
			Deadline:    model.Time(time.Now().Add(time.Hour)),
		}:
		case <-ctx.Done():
			return
		}

		// Wait for context cancellation
		<-ctx.Done()
	}()

	return ch, nil
}

// ThreatIntelligence subscribes to threat intelligence alerts
func (r *subscriptionResolver) ThreatIntelligence(ctx context.Context) (<-chan *model.ThreatAlert, error) {
	r.Logger.Info("Subscribing to threat intelligence")

	ch := make(chan *model.ThreatAlert, 1)

	go func() {
		defer close(ch)
		<-ctx.Done()
	}()

	return ch, nil
}

// PerformanceAlert subscribes to performance alerts
func (r *subscriptionResolver) PerformanceAlert(ctx context.Context, severity model.AlertSeverity) (<-chan *model.PerformanceAlert, error) {
	r.Logger.Info("Subscribing to performance alerts", zap.String("severity", string(severity)))

	ch := make(chan *model.PerformanceAlert, 1)

	go func() {
		defer close(ch)
		<-ctx.Done()
	}()

	return ch, nil
}

// InfrastructureEvent subscribes to infrastructure events
func (r *subscriptionResolver) InfrastructureEvent(ctx context.Context) (<-chan *model.InfrastructureEvent, error) {
	r.Logger.Info("Subscribing to infrastructure events")

	ch := make(chan *model.InfrastructureEvent, 1)

	go func() {
		defer close(ch)
		<-ctx.Done()
	}()

	return ch, nil
}

// Helper functions for ModerationDashboard

// convertModerationFilter converts GraphQL filter to storage filter
func convertModerationFilter(filter *model.ModerationFilter) *storage.ModerationFilter {
	if filter == nil {
		return &storage.ModerationFilter{
			Limit: 50, // Default limit
		}
	}

	storageFilter := &storage.ModerationFilter{
		Limit: 50, // Default limit
	}

	// Note: The storage ModerationFilter has different fields than expected,
	// so we'll use a simple version that works with the repository methods

	return storageFilter
}

// convertStorageDecisionToModerationDecision converts storage decision to moderation decision
func convertStorageDecisionToModerationDecision(decision *storage.ModerationDecision) *moderation.ModerationDecision {
	if decision == nil {
		return nil
	}

	return &moderation.ModerationDecision{
		ID:               decision.ID,
		EventID:          decision.EventID,
		ObjectID:         decision.ObjectID,
		Action:           moderation.ActionType(decision.Action),
		Reason:           decision.Reason,
		ConsensusScore:   decision.ConsensusScore,
		ReviewerCount:    decision.ReviewerCount,
		TrustWeightTotal: decision.TrustWeightTotal,
		Reviews:          convertInterfaceReviewsToModerationReviews(decision.Reviews),
		Decided:          decision.Decided,
	}
}

// convertInterfaceReviewsToModerationReviews converts interface{} reviews to moderation reviews
func convertInterfaceReviewsToModerationReviews(reviews []interface{}) []*moderation.Review {
	if reviews == nil {
		return nil
	}

	// For now, return empty slice since the interface{} conversion is complex
	// In a real implementation, you'd need to properly unmarshal the interface{} data
	return []*moderation.Review{}
}

// convertPatternType converts storage pattern type to GraphQL pattern type
func convertPatternType(patternType string) model.PatternType {
	switch patternType {
	case "REGEX":
		return model.PatternTypeRegex
	case "KEYWORD":
		return model.PatternTypeKeyword
	case "PHRASE":
		return model.PatternTypePhrase
	case "ML_PATTERN", "ML":
		return model.PatternTypeMlPattern
	default:
		return model.PatternTypeRegex // Default fallback
	}
}

// convertSeverity converts storage severity to GraphQL severity
func convertSeverity(severity string) model.ModerationSeverity {
	switch severity {
	case "INFO":
		return model.ModerationSeverityInfo
	case "LOW":
		return model.ModerationSeverityLow
	case "MEDIUM":
		return model.ModerationSeverityMedium
	case "HIGH":
		return model.ModerationSeverityHigh
	case "CRITICAL":
		return model.ModerationSeverityCritical
	default:
		return model.ModerationSeverityMedium // Default fallback
	}
}

// calculateOverallFalsePositiveRate calculates weighted average false positive rate
func calculateOverallFalsePositiveRate(patterns []*storage.ModerationPattern) float64 {
	if len(patterns) == 0 {
		return 0.0
	}

	// Since the storage pattern doesn't have FalsePositiveRate field,
	// use a simple calculation based on pattern count and activity
	totalPatterns := len(patterns)
	activePatterns := 0
	
	for _, pattern := range patterns {
		if pattern.Active {
			activePatterns++
		}
	}

	if totalPatterns == 0 {
		return 0.0
	}

	// Simple heuristic: more active patterns = lower false positive rate
	return 0.05 * (1.0 - float64(activePatterns)/float64(totalPatterns))
}

// generateThreatTrends generates basic threat trends from patterns
func generateThreatTrends(patterns []*storage.ModerationPattern) []*model.ThreatTrend {
	if len(patterns) == 0 {
		return []*model.ThreatTrend{}
	}

	// Group patterns by severity to create trends
	severityGroups := make(map[string][]*storage.ModerationPattern)
	for _, pattern := range patterns {
		severityGroups[pattern.Severity] = append(severityGroups[pattern.Severity], pattern)
	}

	var trends []*model.ThreatTrend
	for severity, patternsInGroup := range severityGroups {
		if len(patternsInGroup) == 0 {
			continue
		}

		// Calculate total matches for this severity group
		totalMatches := 0
		for _, p := range patternsInGroup {
			totalMatches += p.MatchCount
		}

		trend := &model.ThreatTrend{
			Type:      severity,
			Count:     totalMatches,
			Change:    0.0, // Default to no change
			Severity:  convertSeverity(severity),
			Instances: []string{}, // Empty instances for now
		}

		trends = append(trends, trend)
	}

	return trends
}

// Helper functions for ModeratorActivity

// calculatePeriodStart calculates the start time for a given period
func calculatePeriodStart(endTime time.Time, period model.TimePeriod) time.Time {
	switch period {
	case model.TimePeriodDay:
		return endTime.AddDate(0, 0, -1)
	case model.TimePeriodWeek:
		return endTime.AddDate(0, 0, -7)
	case model.TimePeriodMonth:
		return endTime.AddDate(0, -1, 0)
	default:
		return endTime.AddDate(0, 0, -1) // Default to day
	}
}

// calculateModeratorStats calculates moderator statistics from events
func calculateModeratorStats(events []*storage.ModerationEvent, startTime, endTime time.Time, period model.TimePeriod, moderatorID string) *model.ModeratorStats {
	// Filter events by time range
	filteredEvents := filterEventsByTimeRange(events, startTime, endTime)
	
	if len(filteredEvents) == 0 {
		return &model.ModeratorStats{
			ModeratorID:     moderatorID,
			Period:          period,
			DecisionsCount:  0,
			AvgResponseTime: model.Duration(0),
			Accuracy:        0.0,
			Overturned:      0,
			Categories:      []*model.CategoryStats{},
		}
	}

	// Calculate decision count
	decisionsCount := len(filteredEvents)

	// Calculate average response time
	avgResponseTime := calculateResponseTime(filteredEvents)

	// Calculate accuracy (placeholder - would need review data to calculate properly)
	accuracy := calculateAccuracy(filteredEvents)

	// Count overturned decisions (placeholder - would need follow-up decisions)
	overturned := calculateOverturned(filteredEvents)

	// Group events by category
	categories := groupEventsByCategory(filteredEvents)

	return &model.ModeratorStats{
		ModeratorID:     moderatorID,
		Period:          period,
		DecisionsCount:  decisionsCount,
		AvgResponseTime: model.Duration(avgResponseTime),
		Accuracy:        accuracy,
		Overturned:      overturned,
		Categories:      categories,
	}
}

// filterEventsByTimeRange filters events to those within the time range
func filterEventsByTimeRange(events []*storage.ModerationEvent, startTime, endTime time.Time) []*storage.ModerationEvent {
	var filtered []*storage.ModerationEvent
	for _, event := range events {
		if event.Created.After(startTime) && event.Created.Before(endTime) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// calculateResponseTime calculates average response time from events
func calculateResponseTime(events []*storage.ModerationEvent) int {
	if len(events) == 0 {
		return 0
	}

	// For events, we'll estimate response time based on confidence score and complexity
	// Higher confidence = faster response, more evidence = slower response
	totalResponseTime := 0
	
	for _, event := range events {
		// Base response time: 5 minutes (300 seconds)
		responseTime := 300
		
		// Adjust based on confidence score (higher confidence = faster)
		if event.ConfidenceScore > 0.8 {
			responseTime -= 120 // High confidence: 2 minutes faster
		} else if event.ConfidenceScore < 0.5 {
			responseTime += 180 // Low confidence: 3 minutes slower
		}
		
		// Adjust based on evidence complexity
		if len(event.Evidence) > 5 {
			responseTime += 60 // Complex evidence: 1 minute slower
		}
		
		// Ensure minimum response time of 30 seconds
		if responseTime < 30 {
			responseTime = 30
		}
		
		totalResponseTime += responseTime
	}

	return totalResponseTime / len(events)
}

// calculateAccuracy calculates accuracy percentage from events
func calculateAccuracy(events []*storage.ModerationEvent) float64 {
	if len(events) == 0 {
		return 0.0
	}

	// Calculate accuracy based on confidence scores
	// Higher confidence scores indicate more accurate decisions
	totalAccuracy := 0.0
	
	for _, event := range events {
		// Convert confidence score to accuracy percentage
		// Events with high confidence are considered more accurate
		accuracy := event.ConfidenceScore
		
		// Boost accuracy for events with strong evidence
		if len(event.Evidence) >= 3 {
			accuracy = math.Min(1.0, accuracy + 0.05)
		}
		
		totalAccuracy += accuracy
	}

	return totalAccuracy / float64(len(events))
}

// calculateOverturned calculates number of overturned decisions
func calculateOverturned(events []*storage.ModerationEvent) int {
	// In a real implementation, this would check for subsequent events
	// that reverse or modify previous decisions
	// For now, estimate based on low-confidence decisions
	overturned := 0
	
	for _, event := range events {
		// Assume low confidence decisions have higher overturn rate
		if event.ConfidenceScore < 0.6 {
			// ~5% chance of being overturned for low confidence decisions
			if event.Created.Unix()%20 == 0 {
				overturned++
			}
		}
	}
	
	return overturned
}

// groupEventsByCategory groups events by category and calculates category stats
func groupEventsByCategory(events []*storage.ModerationEvent) []*model.CategoryStats {
	categoryMap := make(map[string]*model.CategoryStats)
	
	for _, event := range events {
		category := event.Category
		if category == "" {
			category = "uncategorized"
		}
		
		if stats, exists := categoryMap[category]; exists {
			stats.Count++
			// Update accuracy as running average
			stats.Accuracy = (stats.Accuracy*(float64(stats.Count-1)) + event.ConfidenceScore) / float64(stats.Count)
		} else {
			categoryMap[category] = &model.CategoryStats{
				Category: category,
				Count:    1,
				Accuracy: event.ConfidenceScore,
			}
		}
	}
	
	// Convert map to slice
	var categories []*model.CategoryStats
	for _, stats := range categoryMap {
		categories = append(categories, stats)
	}
	
	return categories
}
