package main

import (
	"context"
	"fmt"
	"log"
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
	storage, err := dynamodbStorage.New()
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
	hashtagCount, err := aggregateHashtagTrends(ctx, storage, since)
	if err != nil {
		log.Printf("Error aggregating hashtag trends: %v", err)
	} else {
		processedCount += hashtagCount
	}

	// 2. Aggregate status trends
	statusCount, err := aggregateStatusTrends(ctx, storage, since)
	if err != nil {
		log.Printf("Error aggregating status trends: %v", err)
	} else {
		processedCount += statusCount
	}

	// 3. Aggregate link trends
	linkCount, err := aggregateLinkTrends(ctx, storage, since)
	if err != nil {
		log.Printf("Error aggregating link trends: %v", err)
	} else {
		processedCount += linkCount
	}

	// Clean up old trend data
	cleanupOldTrends(ctx, storage)

	return TrendAggregatorResponse{
		ProcessedItems: processedCount,
		TimeRange:      event.TimeRange,
		Duration:       time.Since(start).String(),
	}, nil
}

// aggregateHashtagTrends processes hashtag usage and updates trending scores
func aggregateHashtagTrends(ctx context.Context, storage storage.Storage, since time.Time) (int, error) {
	// TODO: Implement hashtag trend aggregation
	// 1. Query recent hashtag usage from GSI6 (hashtag index)
	// 2. Count unique users and total usage
	// 3. Calculate trend scores based on velocity and engagement
	// 4. Store top trending hashtags in a dedicated trend table/index

	log.Println("Aggregating hashtag trends...")
	return 0, nil
}

// aggregateStatusTrends processes status engagement and updates trending scores
func aggregateStatusTrends(ctx context.Context, storage storage.Storage, since time.Time) (int, error) {
	// TODO: Implement status trend aggregation
	// 1. Query recent status engagement (likes, boosts, replies)
	// 2. Factor in author trust scores
	// 3. Calculate trend scores based on engagement velocity
	// 4. Store top trending statuses

	log.Println("Aggregating status trends...")
	return 0, nil
}

// aggregateLinkTrends processes link shares and updates trending scores
func aggregateLinkTrends(ctx context.Context, storage storage.Storage, since time.Time) (int, error) {
	// TODO: Implement link trend aggregation
	// 1. Extract links from recent statuses
	// 2. Count shares and unique sharers
	// 3. Fetch link metadata (title, description, image)
	// 4. Calculate trend scores
	// 5. Store top trending links

	log.Println("Aggregating link trends...")
	return 0, nil
}

// cleanupOldTrends removes outdated trend data
func cleanupOldTrends(ctx context.Context, storage storage.Storage) {
	// TODO: Remove trend data older than 7 days
	log.Println("Cleaning up old trend data...")
}

func main() {
	lambda.Start(Handler)
}
