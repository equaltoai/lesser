package dynamodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// ContentWordSearchStrategy searches by indexed words using GSI5
type ContentWordSearchStrategy struct {
	service *StatusSearchService
}

func (s *ContentWordSearchStrategy) Name() string {
	return "content_word_search"
}

func (s *ContentWordSearchStrategy) Search(ctx context.Context, query string, options StatusSearchOptions) ([]*StatusSearchResult, error) {
	// Extract significant words from query
	words := extractSignificantWords(query)
	if len(words) == 0 {
		return []*StatusSearchResult{}, nil
	}

	// For multi-word queries, search for each word and intersect results
	resultsByStatus := make(map[string]*StatusSearchResult)
	wordCounts := make(map[string]int)

	for _, word := range words {
		// Query GSI5 for this word
		gsi5pk := fmt.Sprintf("WORD#%s", word)

		expr, err := expression.NewBuilder().
			WithKeyCondition(
				expression.Key("GSI5PK").Equal(expression.Value(gsi5pk)),
			).
			Build()

		if err != nil {
			continue
		}

		queryInput := &dynamodb.QueryInput{
			TableName:                 aws.String(s.service.tableName),
			IndexName:                 aws.String("GSI5"),
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			Limit:                     safeInt32(options.Limit * 2), // Get more to filter later
		}

		result, err := s.service.dynamo.Query(ctx, queryInput)
		if err != nil {
			continue
		}

		// Process results
		for _, item := range result.Items {
			// Extract status ID from GSI5SK
			if skAttr, ok := item["GSI5SK"]; ok {
				if skStr, ok := skAttr.(*types.AttributeValueMemberS); ok {
					// Format: STATUS#<timestamp>#<status-id>
					parts := strings.Split(skStr.Value, "#")
					if len(parts) >= 3 {
						statusID := parts[2]
						wordCounts[statusID]++

						// Get status details if not already fetched
						if _, exists := resultsByStatus[statusID]; !exists {
							if statusResult := s.fetchStatusDetails(ctx, statusID, query); statusResult != nil {
								resultsByStatus[statusID] = statusResult
							}
						}
					}
				}
			}
		}
	}

	// Convert to slice and calculate scores based on word matches
	results := make([]*StatusSearchResult, 0)
	for statusID, result := range resultsByStatus {
		// Higher score for matching more words
		matchRatio := float64(wordCounts[statusID]) / float64(len(words))
		result.Score = 0.7 * matchRatio // Base score for word matching

		// Bonus for exact phrase match
		if strings.Contains(strings.ToLower(result.Content), strings.ToLower(query)) {
			result.Score += 0.3
		}

		result.MatchedFields = []string{"content"}
		results = append(results, result)
	}

	return results, nil
}

// HashtagSearchStrategy searches by hashtags using GSI6
type HashtagSearchStrategy struct {
	service *StatusSearchService
}

func (s *HashtagSearchStrategy) Name() string {
	return "hashtag_search"
}

func (s *HashtagSearchStrategy) Search(ctx context.Context, query string, options StatusSearchOptions) ([]*StatusSearchResult, error) {
	hashtags := extractHashtags(query)
	if len(hashtags) == 0 {
		return []*StatusSearchResult{}, nil
	}

	results := make([]*StatusSearchResult, 0)
	seen := make(map[string]bool)

	for _, tag := range hashtags {
		// Query GSI6 for this hashtag
		gsi6pk := fmt.Sprintf("TAG#%s", tag)

		expr, err := expression.NewBuilder().
			WithKeyCondition(
				expression.Key("GSI6PK").Equal(expression.Value(gsi6pk)),
			).
			Build()

		if err != nil {
			continue
		}

		queryInput := &dynamodb.QueryInput{
			TableName:                 aws.String(s.service.tableName),
			IndexName:                 aws.String("GSI6"),
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			Limit:                     safeInt32(options.Limit),
			ScanIndexForward:          aws.Bool(false), // Most recent first
		}

		result, err := s.service.dynamo.Query(ctx, queryInput)
		if err != nil {
			continue
		}

		// Process results
		for _, item := range result.Items {
			// Extract status ID from GSI6SK
			if skAttr, ok := item["GSI6SK"]; ok {
				if skStr, ok := skAttr.(*types.AttributeValueMemberS); ok {
					// Format: STATUS#<timestamp>#<status-id>
					parts := strings.Split(skStr.Value, "#")
					if len(parts) >= 3 {
						statusID := parts[2]

						if !seen[statusID] {
							seen[statusID] = true

							if statusResult := s.fetchStatusDetails(ctx, statusID, query); statusResult != nil {
								statusResult.Score = 0.9 // High score for hashtag matches
								statusResult.MatchedFields = []string{"hashtag"}

								// Create highlight for hashtag
								statusResult.Highlights["hashtag"] = fmt.Sprintf("<em>#%s</em>", tag)

								results = append(results, statusResult)
							}
						}
					}
				}
			}
		}
	}

	return results, nil
}

// URLSearchStrategy searches for exact URL matches
type URLSearchStrategy struct {
	service *StatusSearchService
}

func (s *URLSearchStrategy) Name() string {
	return "url_search"
}

func (s *URLSearchStrategy) Search(ctx context.Context, query string, options StatusSearchOptions) ([]*StatusSearchResult, error) {
	urls := extractURLs(query)
	if len(urls) == 0 {
		return []*StatusSearchResult{}, nil
	}

	results := make([]*StatusSearchResult, 0)

	for _, url := range urls {
		// Try to get object directly by URL (which is often the ID)
		obj, err := s.service.storage.GetObject(ctx, url)
		if err != nil {
			continue
		}

		// Convert to search result
		if result := s.convertObjectToResult(obj, url); result != nil {
			result.Score = 1.0 // Perfect score for URL match
			result.MatchedFields = []string{"url"}
			results = append(results, result)
		}
	}

	return results, nil
}

// AuthorSearchStrategy searches for posts by specific authors using GSI7
type AuthorSearchStrategy struct {
	service *StatusSearchService
}

func (s *AuthorSearchStrategy) Name() string {
	return "author_search"
}

func (s *AuthorSearchStrategy) Search(ctx context.Context, query string, options StatusSearchOptions) ([]*StatusSearchResult, error) {
	// Extract author from query or use AccountID from options
	var authorID string

	if options.AccountID != "" {
		authorID = options.AccountID
	} else {
		// Try to extract from mentions in query
		mentions := extractMentions(query)
		if len(mentions) > 0 {
			// Convert username to author ID
			// For now, assume local user format
			authorID = fmt.Sprintf("https://%s/users/%s", s.service.storage.domain, mentions[0])
		}
	}

	if authorID == "" {
		return []*StatusSearchResult{}, nil
	}

	// Query GSI7 for this author
	gsi7pk := fmt.Sprintf("AUTHOR#%s", authorID)

	builder := expression.NewBuilder().
		WithKeyCondition(
			expression.Key("GSI7PK").Equal(expression.Value(gsi7pk)),
		)

	// Add time range filter if specified
	if !options.TimeRange.Start.IsZero() || !options.TimeRange.End.IsZero() {
		var skCondition expression.KeyConditionBuilder

		if !options.TimeRange.Start.IsZero() && !options.TimeRange.End.IsZero() {
			startSK := fmt.Sprintf("STATUS#%d", options.TimeRange.Start.Unix())
			endSK := fmt.Sprintf("STATUS#%d", options.TimeRange.End.Unix())
			skCondition = expression.Key("GSI7SK").Between(expression.Value(startSK), expression.Value(endSK))
		} else if !options.TimeRange.Start.IsZero() {
			startSK := fmt.Sprintf("STATUS#%d", options.TimeRange.Start.Unix())
			skCondition = expression.Key("GSI7SK").GreaterThanEqual(expression.Value(startSK))
		} else {
			endSK := fmt.Sprintf("STATUS#%d", options.TimeRange.End.Unix())
			skCondition = expression.Key("GSI7SK").LessThanEqual(expression.Value(endSK))
		}

		builder = builder.WithKeyCondition(
			expression.Key("GSI7PK").Equal(expression.Value(gsi7pk)).And(skCondition),
		)
	}

	expr, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build expression: %w", err)
	}

	queryInput := &dynamodb.QueryInput{
		TableName:                 aws.String(s.service.tableName),
		IndexName:                 aws.String("GSI7"),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     safeInt32(options.Limit),
		ScanIndexForward:          aws.Bool(false), // Most recent first
	}

	result, err := s.service.dynamo.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("author search failed: %w", err)
	}

	results := make([]*StatusSearchResult, 0)

	for _, item := range result.Items {
		// Extract status ID from GSI7SK
		if skAttr, ok := item["GSI7SK"]; ok {
			if skStr, ok := skAttr.(*types.AttributeValueMemberS); ok {
				// Format: STATUS#<timestamp>#<status-id>
				parts := strings.Split(skStr.Value, "#")
				if len(parts) >= 3 {
					statusID := parts[2]

					if statusResult := s.fetchStatusDetails(ctx, statusID, query); statusResult != nil {
						statusResult.Score = 0.8 // Good score for author match
						statusResult.MatchedFields = []string{"author"}

						// If query contains keywords, check content match
						keywords := extractSignificantWords(query)
						if len(keywords) > 0 {
							lowerContent := strings.ToLower(statusResult.Content)
							matchCount := 0
							for _, word := range keywords {
								if strings.Contains(lowerContent, word) {
									matchCount++
								}
							}
							if matchCount > 0 {
								statusResult.Score += 0.1 * float64(matchCount) / float64(len(keywords))
								statusResult.MatchedFields = append(statusResult.MatchedFields, "content")
							}
						}

						results = append(results, statusResult)
					}
				}
			}
		}
	}

	return results, nil
}

// TrendingSearchStrategy searches for popular/trending content using GSI8
type TrendingSearchStrategy struct {
	service *StatusSearchService
}

func (s *TrendingSearchStrategy) Name() string {
	return "trending_search"
}

func (s *TrendingSearchStrategy) Search(ctx context.Context, query string, options StatusSearchOptions) ([]*StatusSearchResult, error) {
	// Determine engagement buckets to search
	buckets := []string{"10000", "5000", "1000", "500", "100"}

	if options.MinEngagement > 0 {
		// Filter buckets based on minimum engagement
		filteredBuckets := []string{}
		for _, bucket := range buckets {
			var bucketValue int
			if _, err := fmt.Sscanf(bucket, "%d", &bucketValue); err != nil {
				s.service.logger.Warn("failed to parse engagement bucket",
					zap.String("bucket", bucket),
					zap.Error(err))
				continue
			}
			if bucketValue >= options.MinEngagement {
				filteredBuckets = append(filteredBuckets, bucket)
			}
		}
		buckets = filteredBuckets
	}

	results := make([]*StatusSearchResult, 0)

	for _, bucket := range buckets {
		// Query GSI8 for this engagement bucket
		gsi8pk := fmt.Sprintf("ENGAGEMENT#%s", bucket)

		expr, err := expression.NewBuilder().
			WithKeyCondition(
				expression.Key("GSI8PK").Equal(expression.Value(gsi8pk)),
			).
			Build()

		if err != nil {
			continue
		}

		queryInput := &dynamodb.QueryInput{
			TableName:                 aws.String(s.service.tableName),
			IndexName:                 aws.String("GSI8"),
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			Limit:                     safeInt32(options.Limit),
			ScanIndexForward:          aws.Bool(false), // Highest scores first
		}

		result, err := s.service.dynamo.Query(ctx, queryInput)
		if err != nil {
			continue
		}

		// Process results
		for _, item := range result.Items {
			// Extract status ID from GSI8SK
			if skAttr, ok := item["GSI8SK"]; ok {
				if skStr, ok := skAttr.(*types.AttributeValueMemberS); ok {
					// Format: SCORE#<score>#<status-id>
					parts := strings.Split(skStr.Value, "#")
					if len(parts) >= 3 {
						statusID := parts[2]

						if statusResult := s.fetchStatusDetails(ctx, statusID, query); statusResult != nil {
							// Check if content matches query (if query provided)
							if query != "" {
								lowerContent := strings.ToLower(statusResult.Content)
								lowerQuery := strings.ToLower(query)

								if !strings.Contains(lowerContent, lowerQuery) {
									// Skip if no content match
									continue
								}

								statusResult.Highlights["content"] = highlightMatch(statusResult.Content, query, 200)
							}

							statusResult.Score = 0.6 // Base score for trending
							statusResult.MatchedFields = []string{"trending"}

							results = append(results, statusResult)
						}
					}
				}
			}
		}

		// Stop if we have enough results
		if len(results) >= options.Limit {
			break
		}
	}

	return results, nil
}

// Helper methods

func (s *ContentWordSearchStrategy) fetchStatusDetails(ctx context.Context, statusID string, query string) *StatusSearchResult {
	return fetchStatusDetailsHelper(ctx, s.service.storage, statusID, query)
}

func (s *HashtagSearchStrategy) fetchStatusDetails(ctx context.Context, statusID string, query string) *StatusSearchResult {
	return fetchStatusDetailsHelper(ctx, s.service.storage, statusID, query)
}

func (s *AuthorSearchStrategy) fetchStatusDetails(ctx context.Context, statusID string, query string) *StatusSearchResult {
	return fetchStatusDetailsHelper(ctx, s.service.storage, statusID, query)
}

func (s *TrendingSearchStrategy) fetchStatusDetails(ctx context.Context, statusID string, query string) *StatusSearchResult {
	return fetchStatusDetailsHelper(ctx, s.service.storage, statusID, query)
}

// fetchStatusDetailsHelper fetches full status details
func fetchStatusDetailsHelper(ctx context.Context, storage *dynamoDBStorage, statusID string, query string) *StatusSearchResult {
	obj, err := storage.GetObject(ctx, statusID)
	if err != nil {
		return nil
	}

	result := convertObjectToSearchResult(obj, statusID)
	if result != nil && query != "" {
		// Add highlight
		result.Highlights["content"] = highlightMatch(result.Content, query, 200)
	}

	return result
}

// convertObjectToSearchResult converts an object to a search result
func convertObjectToSearchResult(obj interface{}, statusID string) *StatusSearchResult {
	result := &StatusSearchResult{
		StatusID:   statusID,
		Highlights: make(map[string]string),
		Published:  time.Now(), // Default
	}

	// Try different object types
	switch o := obj.(type) {
	case *Object:
		result.Content = o.Content
		result.URL = o.URL
		result.AuthorID = o.AttributedTo
		result.Published = o.Published

		// Extract username from author ID
		if o.AttributedTo != "" {
			parts := strings.Split(o.AttributedTo, "/")
			if len(parts) > 0 {
				result.AuthorUsername = parts[len(parts)-1]
			}
		}

		// Count attachments as media
		result.HasMedia = len(o.Attachment) > 0

		// Set visibility
		if len(o.To) > 0 {
			if strings.Contains(o.To[0], "Public") {
				result.Visibility = "public"
			} else {
				result.Visibility = "unlisted"
			}
		}

	case map[string]interface{}:
		// Handle generic map representation
		if content, ok := o["content"].(string); ok {
			result.Content = content
		}
		if url, ok := o["url"].(string); ok {
			result.URL = url
		}
		if authorID, ok := o["attributedTo"].(string); ok {
			result.AuthorID = authorID
			parts := strings.Split(authorID, "/")
			if len(parts) > 0 {
				result.AuthorUsername = parts[len(parts)-1]
			}
		}
		if published, ok := o["published"].(string); ok {
			if t, err := time.Parse(time.RFC3339, published); err == nil {
				result.Published = t
			}
		}
	}

	return result
}

func (s *URLSearchStrategy) convertObjectToResult(obj interface{}, url string) *StatusSearchResult {
	return convertObjectToSearchResult(obj, url)
}
