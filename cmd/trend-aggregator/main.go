package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	dynamodbStorage "github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/lambda"
)

// TrendAggregatorEvent represents the Lambda event
type TrendAggregatorEvent struct {
	TimeRange string `json:"timeRange"` // "hour", "day", "week"
}

// TrendAggregatorResponse represents the Lambda response
type TrendAggregatorResponse struct {
	ProcessedItems int    `json:"processedItems"`
	TimeRange      string `json:"timeRange"`
	Duration       string `json:"duration"`
}

// Handler is the Lambda function handler
func Handler(ctx context.Context, event TrendAggregatorEvent) (TrendAggregatorResponse, error) {
	start := time.Now()

	// Initialize storage (it will use the configured table name)
	store, err := dynamodbStorage.New()
	if err != nil {
		return TrendAggregatorResponse{}, fmt.Errorf("failed to create storage: %w", err)
	}

	// Determine time range
	var since time.Time
	switch event.TimeRange {
	case "hour":
		since = time.Now().Add(-1 * time.Hour)
	case "week":
		since = time.Now().Add(-7 * 24 * time.Hour)
	default: // "day" or empty
		since = time.Now().Add(-24 * time.Hour)
		event.TimeRange = "day"
	}

	// Process different types of trends
	processedCount := 0

	// 1. Aggregate hashtag trends
	hashtagCount, err := aggregateHashtagTrends(ctx, store, since)
	if err != nil {
		log.Printf("Error aggregating hashtag trends: %v", err)
	} else {
		processedCount += hashtagCount
	}

	// 2. Aggregate status trends
	statusCount, err := aggregateStatusTrends(ctx, store, since)
	if err != nil {
		log.Printf("Error aggregating status trends: %v", err)
	} else {
		processedCount += statusCount
	}

	// 3. Aggregate link trends
	linkCount, err := aggregateLinkTrends(ctx, store, since)
	if err != nil {
		log.Printf("Error aggregating link trends: %v", err)
	} else {
		processedCount += linkCount
	}

	// Clean up old trend data
	cleanupOldTrends(ctx, store)

	return TrendAggregatorResponse{
		ProcessedItems: processedCount,
		TimeRange:      event.TimeRange,
		Duration:       time.Since(start).String(),
	}, nil
}

// aggregateHashtagTrends processes hashtag usage and updates trending scores
func aggregateHashtagTrends(ctx context.Context, store storage.Storage, since time.Time) (int, error) {
	// Implement hashtag trend aggregation
	// 1. Query recent hashtag usage from GSI6 (hashtag index)
	hashtags, err := store.GetRecentHashtags(ctx, since, 1000)
	if err != nil {
		return 0, fmt.Errorf("failed to get recent hashtags: %w", err)
	}

	// 2. Count unique users and total usage
	hashtagStats := make(map[string]*HashtagTrendData)
	for _, hashtag := range hashtags {
		if stats, exists := hashtagStats[hashtag.Name]; exists {
			stats.TotalUses++
			stats.UniqueUsers[hashtag.UserID] = true
		} else {
			hashtagStats[hashtag.Name] = &HashtagTrendData{
				Name:        hashtag.Name,
				TotalUses:   1,
				UniqueUsers: map[string]bool{hashtag.UserID: true},
				FirstSeen:   hashtag.CreatedAt,
				LastSeen:    hashtag.CreatedAt,
			}
		}
		if hashtag.CreatedAt.After(hashtagStats[hashtag.Name].LastSeen) {
			hashtagStats[hashtag.Name].LastSeen = hashtag.CreatedAt
		}
	}

	// 3. Calculate trend scores and store trending hashtags
	trendingCount := 0
	for name, stats := range hashtagStats {
		// Calculate trend score based on velocity and engagement
		uniqueUserCount := len(stats.UniqueUsers)
		timeSpan := stats.LastSeen.Sub(stats.FirstSeen).Hours()
		if timeSpan == 0 {
			timeSpan = 1 // Prevent division by zero
		}
		velocity := float64(stats.TotalUses) / timeSpan
		trendScore := velocity * float64(uniqueUserCount) * 100

		// Only store if meets minimum threshold
		if trendScore > 10.0 && stats.TotalUses > 3 {
			trend := &storage.HashtagTrend{
				TrendingHashtag: &storage.TrendingHashtag{
					Name:        name,
					UsageCount:  int64(stats.TotalUses),
					UniqueUsers: int64(uniqueUserCount),
					FirstSeen:   stats.FirstSeen,
					LastUsed:    stats.LastSeen,
				},
				TrendingScore: trendScore,
				Velocity:      velocity,
			}
			if err := store.StoreHashtagTrend(ctx, trend); err != nil {
				log.Printf("Failed to store hashtag trend for %s: %v", name, err)
			} else {
				trendingCount++
			}
		}
	}

	log.Printf("Aggregated %d hashtag trends", trendingCount)
	return trendingCount, nil
}

// aggregateStatusTrends processes status engagement and updates trending scores
func aggregateStatusTrends(ctx context.Context, store storage.Storage, since time.Time) (int, error) {
	// Implement status trend aggregation
	// 1. Query recent status engagement (likes, boosts, replies)
	statuses, err := store.GetRecentStatusesWithEngagement(ctx, since, 1000)
	if err != nil {
		return 0, fmt.Errorf("failed to get recent statuses: %w", err)
	}

	trendingCount := 0
	for _, status := range statuses {
		// 2. Factor in author trust scores
		authorTrust, _ := store.GetUserTrustScore(ctx, status.AuthorID)
		if authorTrust == 0 {
			authorTrust = 1.0 // Default trust score
		}

		// 3. Calculate trend scores based on engagement velocity
		age := time.Since(status.CreatedAt).Hours()
		if age == 0 {
			age = 0.1 // Prevent division by zero
		}

		// Weighted engagement score
		engagementScore := (status.Likes * 1) + (status.Boosts * 2) + (status.Replies * 3)
		velocity := float64(engagementScore) / age
		trendScore := velocity * authorTrust

		// 4. Store top trending statuses (minimum threshold)
		if trendScore > 5.0 && engagementScore > 5 {
			trend := &storage.StatusTrend{
				TrendingStatus: &storage.TrendingStatus{
					ID:          status.ID,
					AuthorID:    status.AuthorID,
					Engagements: int64(engagementScore),
					CreatedAt:   status.CreatedAt,
				},
				TrendingScore: trendScore,
				Velocity:      velocity,
			}
			if err := store.StoreStatusTrend(ctx, trend); err != nil {
				log.Printf("Failed to store status trend for %s: %v", status.ID, err)
			} else {
				trendingCount++
			}
		}
	}

	log.Printf("Aggregated %d status trends", trendingCount)
	return trendingCount, nil
}

// aggregateLinkTrends processes link shares and updates trending scores
func aggregateLinkTrends(ctx context.Context, store storage.Storage, since time.Time) (int, error) {
	// Implement link trend aggregation
	// 1. Extract links from recent statuses
	links, err := store.GetRecentLinks(ctx, since, 1000)
	if err != nil {
		return 0, fmt.Errorf("failed to get recent links: %w", err)
	}

	// 2. Count shares and unique sharers
	linkStats := make(map[string]*LinkTrendData)
	for _, link := range links {
		if stats, exists := linkStats[link.URL]; exists {
			stats.ShareCount++
			stats.UniqueSharers[link.UserID] = true
		} else {
			linkStats[link.URL] = &LinkTrendData{
				URL:           link.URL,
				ShareCount:    1,
				UniqueSharers: map[string]bool{link.UserID: true},
				FirstShared:   link.CreatedAt,
				LastShared:    link.CreatedAt,
			}
		}
		if link.CreatedAt.After(linkStats[link.URL].LastShared) {
			linkStats[link.URL].LastShared = link.CreatedAt
		}
	}

	trendingCount := 0
	for url, stats := range linkStats {
		// 3. Calculate trend scores
		uniqueSharerCount := len(stats.UniqueSharers)
		timeSpan := stats.LastShared.Sub(stats.FirstShared).Hours()
		if timeSpan == 0 {
			timeSpan = 1
		}
		velocity := float64(stats.ShareCount) / timeSpan
		trendScore := velocity * float64(uniqueSharerCount) * 50

		// 4. Store top trending links
		if trendScore > 5.0 && stats.ShareCount > 2 {
			trend := &storage.LinkTrend{
				TrendingLink: &storage.TrendingLink{
					URL:        url,
					ShareCount: int64(stats.ShareCount),
					CreatedAt:  stats.FirstShared,
				},
				TrendingScore: trendScore,
				Velocity:      velocity,
			}
			if err := store.StoreLinkTrend(ctx, trend); err != nil {
				log.Printf("Failed to store link trend for %s: %v", url, err)
			} else {
				trendingCount++
			}
		}
	}

	log.Printf("Aggregated %d link trends", trendingCount)
	return trendingCount, nil
}

// cleanupOldTrends removes outdated trend data
func cleanupOldTrends(ctx context.Context, store storage.Storage) {
	// Remove trend data older than 7 days
	cutoff := time.Now().AddDate(0, 0, -7)

	// Clean up hashtag trends
	if err := store.DeleteOldHashtagTrends(ctx, cutoff); err != nil {
		log.Printf("Failed to clean up old hashtag trends: %v", err)
	}

	// Clean up status trends
	if err := store.DeleteOldStatusTrends(ctx, cutoff); err != nil {
		log.Printf("Failed to clean up old status trends: %v", err)
	}

	// Clean up link trends
	if err := store.DeleteOldLinkTrends(ctx, cutoff); err != nil {
		log.Printf("Failed to clean up old link trends: %v", err)
	}

	log.Println("Completed cleanup of old trend data")
}

// HashtagTrendData holds hashtag trending information
type HashtagTrendData struct {
	Name        string
	TotalUses   int
	UniqueUsers map[string]bool
	FirstSeen   time.Time
	LastSeen    time.Time
}

// LinkTrendData holds link trending information
type LinkTrendData struct {
	URL           string
	ShareCount    int
	UniqueSharers map[string]bool
	FirstShared   time.Time
	LastShared    time.Time
}

// extractDomainFromURL extracts domain from URL for basic metadata
func extractDomainFromURL(url string) string {
	if strings.HasPrefix(url, "http://") {
		url = strings.TrimPrefix(url, "http://")
	} else if strings.HasPrefix(url, "https://") {
		url = strings.TrimPrefix(url, "https://")
	}

	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return url
}

func main() {
	lambda.Start(Handler)
}
