package dynamodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/storage"
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
