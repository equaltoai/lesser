package dynamodb

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SearchAnalytics tracks search queries and user interactions
type SearchAnalytics struct {
	dynamo    DynamoDBAPI
	tableName string
	logger    *zap.Logger
}

// NewSearchAnalytics creates a new search analytics service
func NewSearchAnalytics(dynamo DynamoDBAPI, tableName string, logger *zap.Logger) *SearchAnalytics {
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

// TrackStatusSearch records a status search event
func (a *SearchAnalytics) TrackStatusSearch(ctx context.Context, query string, results []*StatusSearchResult, searchTimeMs int64, userID string) error {
	event := SearchEvent{
		Query:       strings.ToLower(strings.TrimSpace(query)),
		ResultCount: len(results),
		SearchTime:  searchTimeMs,
		UserID:      userID,
		Timestamp:   time.Now(),
		SearchType:  "statuses",
	}

	// Store event with daily partitioning
	pk := fmt.Sprintf("SEARCH_LOG#%s", event.Timestamp.Format("2006-01-02"))
	sk := fmt.Sprintf("%d#%s#%s", event.Timestamp.UnixNano(), event.SearchType, event.Query)

	item, err := attributevalue.MarshalMap(event)
	if err != nil {
		a.logger.Error("failed to marshal status search event", zap.Error(err))
		return fmt.Errorf("failed to marshal status search event: %w", err)
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
		a.logger.Error("failed to store status search event",
			zap.String("query", query),
			zap.Error(err))
		return fmt.Errorf("failed to store status search event: %w", err)
	}

	// Track popular status queries separately
	a.updatePopularStatusQueries(ctx, event.Query)

	return nil
}

// updatePopularStatusQueries increments the count for a status search query
func (a *SearchAnalytics) updatePopularStatusQueries(ctx context.Context, query string) {
	pk := "POPULAR_STATUS_QUERIES"
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
		a.logger.Warn("failed to update popular status queries",
			zap.String("query", query),
			zap.Error(err))
	}
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
		Limit:            safeInt32(limit),
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

// TrackSearchQuery records a search query for analytics and suggestions
func (s *dynamoDBStorage) TrackSearchQuery(ctx context.Context, userID, query string, resultCount int) error {
	// Normalize query
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	if normalizedQuery == "" {
		return nil
	}

	// Create search history entry
	entry := storage.SearchHistoryEntry{
		UserID:      userID,
		Query:       normalizedQuery,
		ResultCount: resultCount,
		SearchedAt:  time.Now(),
	}

	av, err := attributevalue.MarshalMap(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal search entry: %w", err)
	}

	// Store in user's search history
	av["PK"] = &types.AttributeValueMemberS{Value: "USER#" + userID}
	av["SK"] = &types.AttributeValueMemberS{Value: "SEARCH#" + entry.SearchedAt.Format(time.RFC3339Nano)}
	av["TTL"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(30*24*time.Hour).Unix())} // 30 days TTL

	// Also store in global search analytics
	globalAv := make(map[string]types.AttributeValue)
	for k, v := range av {
		globalAv[k] = v
	}
	globalAv["PK"] = &types.AttributeValueMemberS{Value: "SEARCH_QUERY#" + normalizedQuery}
	globalAv["SK"] = &types.AttributeValueMemberS{Value: "INSTANCE#" + uuid.New().String()}

	// Write both items
	writeRequests := []types.WriteRequest{
		{
			PutRequest: &types.PutRequest{Item: av},
		},
		{
			PutRequest: &types.PutRequest{Item: globalAv},
		},
	}

	input := &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			*s.getTableName(): writeRequests,
		},
	}

	if _, err := s.client.BatchWriteItem(ctx, input); err != nil {
		return fmt.Errorf("failed to track search query: %w", err)
	}

	return nil
}

// GetPopularSearchQueries retrieves the most popular search queries
func (s *dynamoDBStorage) GetPopularSearchQueries(ctx context.Context, limit int, timeWindow time.Duration) ([]storage.SearchQueryStats, error) {
	// Calculate time cutoff
	cutoff := time.Now().Add(-timeWindow)

	// Scan for recent search queries
	expr, err := expression.NewBuilder().
		WithFilter(
			expression.And(
				expression.BeginsWith(expression.Name("PK"), "SEARCH_QUERY#"),
				expression.GreaterThanEqual(expression.Name("SearchedAt"), expression.Value(cutoff.Format(time.RFC3339))),
			),
		).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build expression: %w", err)
	}

	input := &dynamodb.ScanInput{
		TableName:                 s.getTableName(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		FilterExpression:          expr.Filter(),
	}

	// Collect all queries
	queryMap := make(map[string]*storage.SearchQueryStats)
	userMap := make(map[string]map[string]bool) // query -> set of users

	paginator := dynamodb.NewScanPaginator(s.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to scan queries: %w", err)
		}

		for _, item := range page.Items {
			var entry storage.SearchHistoryEntry
			if err := attributevalue.UnmarshalMap(item, &entry); err != nil {
				continue
			}

			// Update stats
			if stats, exists := queryMap[entry.Query]; exists {
				stats.Count++
				stats.AvgResults = (stats.AvgResults*float64(stats.Count-1) + float64(entry.ResultCount)) / float64(stats.Count)
				if entry.SearchedAt.After(stats.LastUsed) {
					stats.LastUsed = entry.SearchedAt
				}
			} else {
				queryMap[entry.Query] = &storage.SearchQueryStats{
					Query:      entry.Query,
					Count:      1,
					AvgResults: float64(entry.ResultCount),
					LastUsed:   entry.SearchedAt,
				}
				userMap[entry.Query] = make(map[string]bool)
			}

			// Track unique users
			userMap[entry.Query][entry.UserID] = true
		}
	}

	// Calculate unique user counts and convert to slice
	results := make([]storage.SearchQueryStats, 0, len(queryMap))
	for query, stats := range queryMap {
		stats.UserCount = len(userMap[query])
		results = append(results, *stats)
	}

	// Sort by count (most popular first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Count > results[j].Count
	})

	// Apply limit
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// GetUserSearchHistory retrieves a user's search history
func (s *dynamoDBStorage) GetUserSearchHistory(ctx context.Context, userID string, limit int) ([]storage.SearchHistoryEntry, error) {
	expr, err := expression.NewBuilder().
		WithKeyCondition(
			expression.KeyAnd(
				expression.Key("PK").Equal(expression.Value("USER#"+userID)),
				expression.Key("SK").BeginsWith("SEARCH#"),
			),
		).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build expression: %w", err)
	}

	input := &dynamodb.QueryInput{
		TableName:                 s.getTableName(),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ScanIndexForward:          aws.Bool(false), // Most recent first
		Limit:                     safeInt32(limit),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query search history: %w", err)
	}

	entries := make([]storage.SearchHistoryEntry, 0, len(result.Items))
	for _, item := range result.Items {
		var entry storage.SearchHistoryEntry
		if err := attributevalue.UnmarshalMap(item, &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// GenerateSearchSuggestions generates search suggestions based on user history and popular queries
func (s *dynamoDBStorage) GenerateSearchSuggestions(ctx context.Context, userID, partialQuery string, limit int) ([]string, error) {
	normalizedQuery := strings.ToLower(strings.TrimSpace(partialQuery))
	if normalizedQuery == "" {
		return []string{}, nil
	}

	suggestions := make(map[string]float64) // suggestion -> score

	// Get user's search history
	userHistory, err := s.GetUserSearchHistory(ctx, userID, 50)
	if err == nil {
		for _, entry := range userHistory {
			if strings.HasPrefix(entry.Query, normalizedQuery) {
				// Score based on recency and result count
				daysSince := time.Since(entry.SearchedAt).Hours() / 24
				recencyScore := 1.0 / (1.0 + daysSince)
				resultScore := 1.0
				if entry.ResultCount > 0 {
					resultScore = 1.0 + (float64(entry.ResultCount) / 100.0)
				}
				suggestions[entry.Query] = recencyScore * resultScore * 2.0 // Boost personal history
			}
		}
	}

	// Get popular queries
	popularQueries, err := s.GetPopularSearchQueries(ctx, 100, 7*24*time.Hour)
	if err == nil {
		for _, stats := range popularQueries {
			if strings.HasPrefix(stats.Query, normalizedQuery) {
				// Score based on popularity and average results
				popularityScore := float64(stats.Count) / 100.0
				if popularityScore > 1.0 {
					popularityScore = 1.0
				}
				resultScore := stats.AvgResults / 10.0
				if resultScore > 1.0 {
					resultScore = 1.0
				}

				existingScore := suggestions[stats.Query]
				suggestions[stats.Query] = existingScore + (popularityScore * resultScore)
			}
		}
	}

	// Sort suggestions by score
	type scoredSuggestion struct {
		query string
		score float64
	}
	scored := make([]scoredSuggestion, 0, len(suggestions))
	for query, score := range suggestions {
		scored = append(scored, scoredSuggestion{query, score})
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Extract top suggestions
	result := make([]string, 0, limit)
	for i, s := range scored {
		if i >= limit {
			break
		}
		result = append(result, s.query)
	}

	return result, nil
}
