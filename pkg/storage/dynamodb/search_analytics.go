package dynamodb

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// SearchAnalytics tracks search queries and user interactions
type SearchAnalytics struct {
	dynamo    *dynamodb.Client
	tableName string
	logger    *zap.Logger
}

// NewSearchAnalytics creates a new search analytics service
func NewSearchAnalytics(dynamo *dynamodb.Client, tableName string, logger *zap.Logger) *SearchAnalytics {
	return &SearchAnalytics{
		dynamo:    dynamo,
		tableName: tableName,
		logger:    logger,
	}
}

// SearchEvent represents a search query event
type SearchEvent struct {
	Query         string    `dynamodbav:"Query"`
	ResultCount   int       `dynamodbav:"ResultCount"`
	SearchTime    int64     `dynamodbav:"SearchTime"` // milliseconds
	UserID        string    `dynamodbav:"UserID,omitempty"`
	Timestamp     time.Time `dynamodbav:"Timestamp"`
	ClickedResult string    `dynamodbav:"ClickedResult,omitempty"`
	SearchType    string    `dynamodbav:"SearchType"` // "accounts", "statuses", "hashtags"
}

// TrackSearch records a search event
func (a *SearchAnalytics) TrackSearch(ctx context.Context, query string, results []*SearchResult, searchTimeMs int64, userID string) error {
	event := SearchEvent{
		Query:       strings.ToLower(strings.TrimSpace(query)),
		ResultCount: len(results),
		SearchTime:  searchTimeMs,
		UserID:      userID,
		Timestamp:   time.Now(),
		SearchType:  "accounts",
	}

	// Store event with daily partitioning
	pk := fmt.Sprintf("SEARCH_LOG#%s", event.Timestamp.Format("2006-01-02"))
	sk := fmt.Sprintf("%d#%s#%s", event.Timestamp.UnixNano(), event.SearchType, event.Query)

	item, err := attributevalue.MarshalMap(event)
	if err != nil {
		a.logger.Error("failed to marshal search event", zap.Error(err))
		return fmt.Errorf("failed to marshal search event: %w", err)
	}

	item["PK"] = &types.AttributeValueMemberS{Value: pk}
	item["SK"] = &types.AttributeValueMemberS{Value: sk}

	// Set TTL to 90 days for analytics data
	ttl := time.Now().Add(90 * 24 * time.Hour).Unix()
	item["TTL"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(ttl, 10)}

	_, err = a.dynamo.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(a.tableName),
		Item:      item,
	})

	if err != nil {
		a.logger.Error("failed to store search event",
			zap.String("query", query),
			zap.Error(err))
		return fmt.Errorf("failed to store search event: %w", err)
	}

	// Also track popular queries for suggestions
	a.updatePopularQueries(ctx, event.Query)

	return nil
}

// TrackClick records when a user clicks on a search result
func (a *SearchAnalytics) TrackClick(ctx context.Context, query string, clickedActorID string, userID string) error {
	// Record the click event
	pk := fmt.Sprintf("SEARCH_CLICKS#%s", time.Now().Format("2006-01-02"))
	sk := fmt.Sprintf("%d#%s#%s", time.Now().UnixNano(), query, clickedActorID)

	item := map[string]types.AttributeValue{
		"PK":             &types.AttributeValueMemberS{Value: pk},
		"SK":             &types.AttributeValueMemberS{Value: sk},
		"Query":          &types.AttributeValueMemberS{Value: strings.ToLower(strings.TrimSpace(query))},
		"ClickedActorID": &types.AttributeValueMemberS{Value: clickedActorID},
		"UserID":         &types.AttributeValueMemberS{Value: userID},
		"Timestamp":      &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	// Set TTL
	ttl := time.Now().Add(90 * 24 * time.Hour).Unix()
	item["TTL"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(ttl, 10)}

	_, err := a.dynamo.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(a.tableName),
		Item:      item,
	})

	if err != nil {
		a.logger.Error("failed to track click",
			zap.String("query", query),
			zap.String("clicked", clickedActorID),
			zap.Error(err))
		return fmt.Errorf("failed to track click: %w", err)
	}

	// Update click-through rate for the query-result pair
	a.updateClickThroughRate(ctx, query, clickedActorID)

	return nil
}

// updatePopularQueries increments the count for a search query
func (a *SearchAnalytics) updatePopularQueries(ctx context.Context, query string) {
	pk := "POPULAR_QUERIES"
	sk := fmt.Sprintf("QUERY#%s", query)

	// Increment the query count
	_, err := a.dynamo.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(a.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		UpdateExpression: aws.String("ADD QueryCount :inc SET LastQueried = :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":inc": &types.AttributeValueMemberN{Value: "1"},
			":now": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	})

	if err != nil {
		a.logger.Warn("failed to update popular queries",
			zap.String("query", query),
			zap.Error(err))
	}
}

// updateClickThroughRate updates CTR for a query-result pair
func (a *SearchAnalytics) updateClickThroughRate(ctx context.Context, query string, actorID string) {
	pk := fmt.Sprintf("CTR#%s", strings.ToLower(strings.TrimSpace(query)))
	sk := fmt.Sprintf("ACTOR#%s", actorID)

	// Increment click count and update last clicked time
	_, err := a.dynamo.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(a.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		UpdateExpression: aws.String("ADD ClickCount :inc SET LastClicked = :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":inc": &types.AttributeValueMemberN{Value: "1"},
			":now": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	})

	if err != nil {
		a.logger.Warn("failed to update CTR",
			zap.String("query", query),
			zap.String("actor", actorID),
			zap.Error(err))
	}
}

// GetPopularQueries returns the most popular search queries
func (a *SearchAnalytics) GetPopularQueries(ctx context.Context, limit int) ([]string, error) {
	// Query popular queries sorted by count
	result, err := a.dynamo.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(a.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "POPULAR_QUERIES"},
		},
		ScanIndexForward: aws.Bool(false), // Sort descending
		Limit:            aws.Int32(int32(limit)),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get popular queries: %w", err)
	}

	queries := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		if sk, ok := item["SK"].(*types.AttributeValueMemberS); ok {
			// Extract query from SK (format: QUERY#<query>)
			parts := strings.Split(sk.Value, "#")
			if len(parts) >= 2 {
				queries = append(queries, parts[1])
			}
		}
	}

	return queries, nil
}

// GetClickThroughRate returns the CTR data for improving search ranking
func (a *SearchAnalytics) GetClickThroughRate(ctx context.Context, query string) (map[string]float64, error) {
	pk := fmt.Sprintf("CTR#%s", strings.ToLower(strings.TrimSpace(query)))

	result, err := a.dynamo.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(a.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get CTR data: %w", err)
	}

	ctrMap := make(map[string]float64)
	for _, item := range result.Items {
		if sk, ok := item["SK"].(*types.AttributeValueMemberS); ok {
			// Extract actor ID from SK (format: ACTOR#<id>)
			parts := strings.Split(sk.Value, "#")
			if len(parts) >= 2 {
				actorID := parts[1]

				// Get click count
				if clickCount, ok := item["ClickCount"].(*types.AttributeValueMemberN); ok {
					count, _ := strconv.ParseFloat(clickCount.Value, 64)
					// Simple CTR score based on click count
					// In production, this would factor in impressions
					ctrMap[actorID] = count
				}
			}
		}
	}

	return ctrMap, nil
}

// SearchMetrics provides aggregated search metrics
type SearchMetrics struct {
	TotalSearches      int64
	UniqueQueries      int64
	AverageSearchTime  float64
	AverageResultCount float64
	TopQueries         []string
}

// GetSearchMetrics returns aggregated metrics for a date range
func (a *SearchAnalytics) GetSearchMetrics(ctx context.Context, startDate, endDate time.Time) (*SearchMetrics, error) {
	metrics := &SearchMetrics{}

	// Query search logs for the date range
	start := startDate.Format("2006-01-02")
	end := endDate.Format("2006-01-02")

	// For simplicity, query each day separately
	// In production, use GSI for efficient date range queries
	current := startDate
	uniqueQueries := make(map[string]bool)
	totalSearchTime := int64(0)
	totalResults := int64(0)

	for current.Before(endDate) || current.Equal(endDate) {
		pk := fmt.Sprintf("SEARCH_LOG#%s", current.Format("2006-01-02"))

		result, err := a.dynamo.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(a.tableName),
			KeyConditionExpression: aws.String("PK = :pk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: pk},
			},
		})

		if err != nil {
			a.logger.Warn("failed to query search logs",
				zap.String("date", current.Format("2006-01-02")),
				zap.Error(err))
			current = current.Add(24 * time.Hour)
			continue
		}

		for _, item := range result.Items {
			var event SearchEvent
			if err := attributevalue.UnmarshalMap(item, &event); err == nil {
				metrics.TotalSearches++
				uniqueQueries[event.Query] = true
				totalSearchTime += event.SearchTime
				totalResults += int64(event.ResultCount)
			}
		}

		current = current.Add(24 * time.Hour)
	}

	metrics.UniqueQueries = int64(len(uniqueQueries))

	if metrics.TotalSearches > 0 {
		metrics.AverageSearchTime = float64(totalSearchTime) / float64(metrics.TotalSearches)
		metrics.AverageResultCount = float64(totalResults) / float64(metrics.TotalSearches)
	}

	// Get top queries
	topQueries, _ := a.GetPopularQueries(ctx, 10)
	metrics.TopQueries = topQueries

	a.logger.Info("search metrics calculated",
		zap.String("start", start),
		zap.String("end", end),
		zap.Int64("total_searches", metrics.TotalSearches))

	return metrics, nil
}
