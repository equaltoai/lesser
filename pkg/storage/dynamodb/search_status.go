package dynamodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// SearchStatuses searches for statuses using the comprehensive search system
func (s *dynamoDBStorage) SearchStatuses(ctx context.Context, query string, limit int) ([]*storage.StatusSearchResult, error) {
	// Use the new method with default options
	options := storage.StatusSearchOptions{
		Limit: limit,
	}
	return s.SearchStatusesWithOptions(ctx, query, options)
}

// SearchStatusesWithOptions performs an advanced status search with filtering options
func (s *dynamoDBStorage) SearchStatusesWithOptions(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, error) {
	if s.statusSearchService == nil {
		s.logger().Warn("status search service not initialized")
		return []*storage.StatusSearchResult{}, nil
	}

	// Convert storage options to internal search options
	searchOpts := StatusSearchOptions{
		Limit:         options.Limit,
		Offset:        options.Offset,
		AccountID:     options.AccountID,
		FollowingOnly: options.FollowingOnly,
		LocalOnly:     options.LocalOnly,
		MediaOnly:     options.MediaOnly,
		Language:      options.Language,
		MinEngagement: options.MinEngagement,
		TimeRange: TimeRange{
			Start: options.TimeRange.Start,
			End:   options.TimeRange.End,
		},
	}

	// Perform the search
	results, err := s.statusSearchService.Search(ctx, query, searchOpts)
	if err != nil {
		return nil, fmt.Errorf("status search failed: %w", err)
	}

	// Convert internal results to storage results
	storageResults := make([]*storage.StatusSearchResult, len(results))
	for i, result := range results {
		storageResults[i] = &storage.StatusSearchResult{
			StatusID:       result.StatusID,
			Content:        result.Content,
			URL:            result.URL,
			AuthorID:       result.AuthorID,
			AuthorUsername: result.AuthorUsername,
			Published:      result.Published,
			Score:          result.Score,
			Highlights:     result.Highlights,
		}
	}

	return storageResults, nil
}

// basicStatusSearch provides a fallback basic search implementation
// TODO: This method is kept as a fallback option but is currently unused in favor of the more advanced StatusSearchService.
// It may be useful for debugging or as a simpler alternative when the search service is unavailable.
// nolint:unused
func (s *dynamoDBStorage) basicStatusSearch(ctx context.Context, query string, limit int) ([]*storage.StatusSearchResult, error) {
	if query == "" || limit <= 0 {
		return []*storage.StatusSearchResult{}, nil
	}

	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	results := make([]*storage.StatusSearchResult, 0)

	// Basic scan with filter (fallback implementation)
	scanInput := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(PK, :objectPrefix) AND SK = :metadata AND contains(#content, :query)"),
		ExpressionAttributeNames: map[string]string{
			"#content": "Object.content",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":objectPrefix": &types.AttributeValueMemberS{Value: "OBJECT#"},
			":metadata":     &types.AttributeValueMemberS{Value: "METADATA"},
			":query":        &types.AttributeValueMemberS{Value: normalizedQuery},
		},
		Limit: aws.Int32(int32(limit * 2)),
	}

	paginator := dynamodb.NewScanPaginator(s.client, scanInput)

	for paginator.HasMorePages() && len(results) < limit {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			s.logger().Error("status search scan failed", zap.Error(err))
			return nil, fmt.Errorf("status search failed: %w", err)
		}

		for _, item := range page.Items {
			var record ObjectRecord
			if err := s.UnmarshalItem(item, &record); err != nil {
				continue
			}

			if record.Object == nil || record.Object.Type != "Note" {
				continue
			}

			// Calculate basic relevance score
			score := s.calculateStatusScore(record.Object.Content, normalizedQuery)

			// Create highlight
			highlight := s.createHighlight(record.Object.Content, normalizedQuery, 150)

			result := &storage.StatusSearchResult{
				StatusID:  record.Object.ID,
				Content:   record.Object.Content,
				URL:       record.Object.URL,
				AuthorID:  record.Object.AttributedTo,
				Published: record.Object.Published,
				Score:     score,
				Highlights: map[string]string{
					"content": highlight,
				},
			}

			results = append(results, result)

			if len(results) >= limit {
				break
			}
		}
	}

	// Sort by score
	s.sortStatusResults(results)

	return results, nil
}

// SearchStatusesByURL searches for a status by exact URL
func (s *dynamoDBStorage) SearchStatusesByURL(ctx context.Context, url string) (*storage.StatusSearchResult, error) {
	// Try to get the object directly by ID
	obj, err := s.GetObject(ctx, url)
	if err != nil {
		return nil, err
	}

	// Check if it's a Note type
	if note, ok := obj.(*activitypub.Note); ok {
		published := time.Now()
		if note.Published != nil {
			published = *note.Published
		}

		return &storage.StatusSearchResult{
			StatusID:  note.ID,
			Content:   note.Content,
			URL:       note.ID, // URL is typically the ID for notes
			AuthorID:  note.AttributedTo,
			Published: published,
			Score:     1.0, // Perfect match for URL search
		}, nil
	}

	// Try to handle generic object types
	if objMap, ok := obj.(map[string]interface{}); ok {
		result := &storage.StatusSearchResult{
			Score:      1.0,
			Highlights: make(map[string]string),
			Published:  time.Now(), // Default
		}

		if id, ok := objMap["id"].(string); ok {
			result.StatusID = id
			result.URL = id // Default URL to ID
		}
		if content, ok := objMap["content"].(string); ok {
			result.Content = content
		}
		if url, ok := objMap["url"].(string); ok {
			result.URL = url
		}
		if attributedTo, ok := objMap["attributedTo"].(string); ok {
			result.AuthorID = attributedTo
			// Extract username from author ID
			parts := strings.Split(attributedTo, "/")
			if len(parts) > 0 {
				result.AuthorUsername = parts[len(parts)-1]
			}
		}
		if published, ok := objMap["published"].(string); ok {
			if t, err := time.Parse(time.RFC3339, published); err == nil {
				result.Published = t
			}
		}

		return result, nil
	}

	return nil, fmt.Errorf("object is not a status")
}

// calculateStatusScore calculates a relevance score for status content
func (s *dynamoDBStorage) calculateStatusScore(content, query string) float64 {
	contentLower := strings.ToLower(content)

	// Base score for containing the query
	score := 0.5

	// Bonus for exact match
	if contentLower == query {
		return 1.0
	}

	// Count occurrences
	count := strings.Count(contentLower, query)
	score += float64(count) * 0.1

	// Bonus for query at start
	if strings.HasPrefix(contentLower, query) {
		score += 0.2
	}

	// Consider query position (earlier = better)
	position := strings.Index(contentLower, query)
	if position >= 0 && len(content) > 0 {
		positionScore := 1.0 - (float64(position) / float64(len(content)))
		score += positionScore * 0.1
	}

	// Cap at 0.95 (reserve 1.0 for perfect matches)
	if score > 0.95 {
		score = 0.95
	}

	return score
}

// createHighlight creates a highlighted excerpt of content
func (s *dynamoDBStorage) createHighlight(content, query string, maxLength int) string {
	contentLower := strings.ToLower(content)
	index := strings.Index(contentLower, query)

	if index == -1 {
		// Query not found, return beginning of content
		if len(content) <= maxLength {
			return content
		}
		return content[:maxLength] + "..."
	}

	// Find a good starting point for the excerpt
	start := index - 50
	if start < 0 {
		start = 0
	}

	// Find a good ending point
	end := index + len(query) + 100
	if end > len(content) {
		end = len(content)
	}

	// Adjust to word boundaries if possible
	if start > 0 {
		// Find previous space
		for i := start; i < index && i < len(content); i++ {
			if content[i] == ' ' {
				start = i + 1
				break
			}
		}
	}

	if end < len(content) {
		// Find next space
		for i := end; i > index+len(query) && i > 0; i-- {
			if content[i-1] == ' ' {
				end = i - 1
				break
			}
		}
	}

	excerpt := content[start:end]

	// Add ellipsis if needed
	if start > 0 {
		excerpt = "..." + excerpt
	}
	if end < len(content) {
		excerpt = excerpt + "..."
	}

	// Highlight the query in the excerpt
	excerptLower := strings.ToLower(excerpt)
	highlightIndex := strings.Index(excerptLower, query)
	if highlightIndex >= 0 {
		highlighted := excerpt[:highlightIndex] +
			"<em>" + excerpt[highlightIndex:highlightIndex+len(query)] + "</em>" +
			excerpt[highlightIndex+len(query):]
		return highlighted
	}

	return excerpt
}

// sortStatusResults sorts status results by score (descending)
func (s *dynamoDBStorage) sortStatusResults(results []*storage.StatusSearchResult) {
	// Simple bubble sort for small result sets
	n := len(results)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if results[j].Score < results[j+1].Score {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}
}
