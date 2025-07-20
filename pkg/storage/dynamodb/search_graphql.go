package dynamodb

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/federation"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// SearchAll performs a comprehensive search across all content types
func (s *dynamoDBStorage) SearchAll(ctx context.Context, query string, limit int, accountID string) (*storage.SearchResults, error) {
	if strings.TrimSpace(query) == "" {
		return &storage.SearchResults{}, nil
	}

	// Execute searches in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := &storage.SearchResults{
		Accounts: []*activitypub.Actor{},
		Statuses: []*storage.StatusSearchResult{},
		Hashtags: []*storage.HashtagSearchResult{},
	}

	// Search accounts
	wg.Add(1)
	go func() {
		defer wg.Done()
		accounts, err := s.SearchAccountsAdvanced(ctx, query, false, limit/3, 0, false, accountID)
		if err != nil {
			s.logger().Warn("account search failed", zap.Error(err))
			return
		}
		mu.Lock()
		results.Accounts = accounts
		mu.Unlock()
	}()

	// Search statuses
	wg.Add(1)
	go func() {
		defer wg.Done()
		statuses, err := s.SearchStatusesAdvanced(ctx, query, limit/3, nil, nil, accountID)
		if err != nil {
			s.logger().Warn("status search failed", zap.Error(err))
			return
		}
		mu.Lock()
		results.Statuses = statuses
		mu.Unlock()
	}()

	// Search hashtags
	wg.Add(1)
	go func() {
		defer wg.Done()
		hashtags, err := s.SearchHashtagsAdvanced(ctx, query, limit/3, accountID)
		if err != nil {
			s.logger().Warn("hashtag search failed", zap.Error(err))
			return
		}
		mu.Lock()
		results.Hashtags = hashtags
		mu.Unlock()
	}()

	wg.Wait()
	return results, nil
}

// SearchAccountsAdvanced performs advanced account search with additional filtering
func (s *dynamoDBStorage) SearchAccountsAdvanced(ctx context.Context, query string, resolve bool, limit int, offset int, following bool, accountID string) ([]*activitypub.Actor, error) {
	// Start with basic search
	accounts, err := s.SearchAccounts(ctx, query, limit+offset, following, 0)
	if err != nil {
		return nil, fmt.Errorf("basic account search failed: %w", err)
	}

	// Apply offset
	if offset > 0 {
		if offset >= len(accounts) {
			return []*activitypub.Actor{}, nil
		}
		accounts = accounts[offset:]
	}

	// Apply limit
	if len(accounts) > limit {
		accounts = accounts[:limit]
	}

	// If resolve is true and we have few results, attempt remote resolution
	if resolve && len(accounts) < limit && isValidFederatedHandle(query) {
		// Create remote search service
		remoteSearchSvc := federation.NewRemoteSearchService(s)

		// Search for remote actors
		remoteResults, err := remoteSearchSvc.SearchRemoteActors(ctx, query, limit-len(accounts))
		if err != nil {
			s.logger().Debug("remote actor resolution failed",
				zap.String("query", query),
				zap.Error(err))
		} else if len(remoteResults) > 0 {
			// Add remote actors to results
			for _, result := range remoteResults {
				if result.Actor != nil {
					accounts = append(accounts, result.Actor)
				}
			}

			s.logger().Debug("remote actors resolved",
				zap.String("query", query),
				zap.Int("remote_results", len(remoteResults)))
		}
	}

	return accounts, nil
}

// SearchStatusesAdvanced performs advanced status search with timeline filtering
func (s *dynamoDBStorage) SearchStatusesAdvanced(ctx context.Context, query string, limit int, maxID, minID *string, accountID string) ([]*storage.StatusSearchResult, error) {
	// Use existing status search service
	if s.statusSearchService == nil {
		return []*storage.StatusSearchResult{}, nil
	}

	options := StatusSearchOptions{
		Limit:     limit,
		AccountID: accountID,
	}

	results, err := s.statusSearchService.Search(ctx, query, options)
	if err != nil {
		return nil, fmt.Errorf("status search failed: %w", err)
	}

	// Convert results to storage type
	storageResults := make([]*storage.StatusSearchResult, len(results))
	for i, result := range results {
		storageResults[i] = &storage.StatusSearchResult{
			StatusID:       result.StatusID,
			Content:        result.Content,
			AuthorID:       result.AuthorID,
			AuthorUsername: result.AuthorUsername,
			Published:      result.Published,
			Score:          result.Score,
		}
	}

	return storageResults, nil
}

// SearchHashtagsAdvanced performs advanced hashtag search with follow status
func (s *dynamoDBStorage) SearchHashtagsAdvanced(ctx context.Context, query string, limit int, accountID string) ([]*storage.HashtagSearchResult, error) {
	// Clean query
	cleanQuery := strings.TrimPrefix(strings.ToLower(query), "#")
	if cleanQuery == "" {
		return []*storage.HashtagSearchResult{}, nil
	}

	// Search for hashtags
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("hashtag-search"),
		KeyConditionExpression: aws.String("begins_with(#hashtag, :query)"),
		ExpressionAttributeNames: map[string]string{
			"#hashtag": "Hashtag",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":query": &types.AttributeValueMemberS{Value: cleanQuery},
		},
		Limit: aws.Int32(int32(limit * 2)), // Get more for ranking
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If GSI doesn't exist, fall back to basic search
		return s.fallbackHashtagSearch(ctx, cleanQuery, limit, accountID)
	}

	// Process results
	hashtagResults := make([]*storage.HashtagSearchResult, 0)
	seen := make(map[string]bool)

	for _, item := range result.Items {
		hashtag := ""
		if val, ok := item["Hashtag"]; ok {
			if s, ok := val.(*types.AttributeValueMemberS); ok {
				hashtag = s.Value
			}
		}

		if hashtag == "" || seen[hashtag] {
			continue
		}
		seen[hashtag] = true

		// Check if user is following this hashtag
		var following *bool
		if accountID != "" {
			isFollowing, err := s.IsFollowingHashtag(ctx, accountID, hashtag)
			if err == nil {
				following = &isFollowing
			}
		}

		hashtagResult := &storage.HashtagSearchResult{
			Name:      hashtag,
			URL:       fmt.Sprintf("https://%s/tags/%s", s.domain, hashtag),
			History:   []*storage.TrendingHashtag{}, // TODO: Implement tag history
			Following: following,
		}

		hashtagResults = append(hashtagResults, hashtagResult)

		if len(hashtagResults) >= limit {
			break
		}
	}

	return hashtagResults, nil
}

// fallbackHashtagSearch provides a fallback when GSI is not available
func (s *dynamoDBStorage) fallbackHashtagSearch(ctx context.Context, query string, limit int, accountID string) ([]*storage.HashtagSearchResult, error) {
	// Simple scan for hashtags (expensive, but fallback)
	input := &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("begins_with(SK, :prefix) AND contains(#hashtag, :query)"),
		ExpressionAttributeNames: map[string]string{
			"#hashtag": "Hashtag",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefix": &types.AttributeValueMemberS{Value: "HASHTAG#"},
			":query":  &types.AttributeValueMemberS{Value: query},
		},
		Limit: aws.Int32(int32(limit)),
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		s.logger().Warn("hashtag fallback search failed", zap.Error(err))
		return []*storage.HashtagSearchResult{}, nil
	}

	hashtagResults := make([]*storage.HashtagSearchResult, 0)
	for _, item := range result.Items {
		hashtag := ""
		if val, ok := item["Hashtag"]; ok {
			if s, ok := val.(*types.AttributeValueMemberS); ok {
				hashtag = s.Value
			}
		}

		if hashtag == "" {
			continue
		}

		var following *bool
		if accountID != "" {
			isFollowing, err := s.IsFollowingHashtag(ctx, accountID, hashtag)
			if err == nil {
				following = &isFollowing
			}
		}

		hashtagResult := &storage.HashtagSearchResult{
			Name:      hashtag,
			URL:       fmt.Sprintf("https://%s/tags/%s", s.domain, hashtag),
			History:   []*storage.TrendingHashtag{},
			Following: following,
		}

		hashtagResults = append(hashtagResults, hashtagResult)
	}

	return hashtagResults, nil
}

// isValidFederatedHandle checks if a query is a valid federated handle (@user@domain)
func isValidFederatedHandle(query string) bool {
	// Simple check for @user@domain pattern
	if len(query) < 5 {
		return false
	}

	atCount := 0
	for _, ch := range query {
		if ch == '@' {
			atCount++
		}
	}

	// Should have exactly 2 @ symbols for federated handle or 1 @ at the start
	return atCount == 2 || (atCount == 1 && query[0] == '@')
}
