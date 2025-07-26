package dynamodb

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"go.uber.org/zap"
)

// SearchFilter applies additional filtering to search results
type SearchFilter interface {
	Filter(ctx context.Context, results []*SearchResult, userID string) ([]*SearchResult, error)
	Name() string
}

// FollowingOnlyFilter filters results to only show accounts the user follows
type FollowingOnlyFilter struct {
	storage *dynamoDBStorage
	logger  *zap.Logger
}

// NewFollowingOnlyFilter creates a new following-only filter
func NewFollowingOnlyFilter(storage *dynamoDBStorage, logger *zap.Logger) *FollowingOnlyFilter {
	return &FollowingOnlyFilter{
		storage: storage,
		logger:  logger,
	}
}

func (f *FollowingOnlyFilter) Name() string {
	return "following_only"
}

func (f *FollowingOnlyFilter) Filter(ctx context.Context, results []*SearchResult, userID string) ([]*SearchResult, error) {
	if userID == "" || len(results) == 0 {
		return results, nil
	}

	// Get list of accounts the user follows
	following, err := f.getFollowingList(ctx, userID)
	if err != nil {
		f.logger.Warn("failed to get following list",
			zap.String("user", userID),
			zap.Error(err))
		// Return unfiltered results on error
		return results, nil
	}

	// Create a map for O(1) lookup
	followingMap := make(map[string]bool)
	for _, actorID := range following {
		followingMap[actorID] = true
	}

	// Filter results
	filtered := make([]*SearchResult, 0)
	for _, result := range results {
		if result.Actor != nil && followingMap[result.Actor.ID] {
			filtered = append(filtered, result)
		}
	}

	return filtered, nil
}

func (f *FollowingOnlyFilter) getFollowingList(ctx context.Context, userID string) ([]string, error) {
	// Query the follows relationship
	// PK: ACTOR#<username>#FOLLOWS
	pk := fmt.Sprintf("ACTOR#%s#FOLLOWS", userID)

	result, err := f.storage.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(f.storage.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
		ProjectionExpression: aws.String("TargetActorID"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query following list: %w", err)
	}

	following := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		if targetID, ok := item["TargetActorID"].(*types.AttributeValueMemberS); ok {
			following = append(following, targetID.Value)
		}
	}

	return following, nil
}

// LocalOnlyFilter filters results to only show local accounts
type LocalOnlyFilter struct {
	domain string
	logger *zap.Logger
}

// NewLocalOnlyFilter creates a new local-only filter
func NewLocalOnlyFilter(domain string, logger *zap.Logger) *LocalOnlyFilter {
	return &LocalOnlyFilter{
		domain: domain,
		logger: logger,
	}
}

func (f *LocalOnlyFilter) Name() string {
	return "local_only"
}

func (f *LocalOnlyFilter) Filter(ctx context.Context, results []*SearchResult, userID string) ([]*SearchResult, error) {
	if len(results) == 0 {
		return results, nil
	}

	// Filter to only local actors
	filtered := make([]*SearchResult, 0)
	for _, result := range results {
		if result.Actor != nil && f.isLocalActor(result.Actor) {
			filtered = append(filtered, result)
		}
	}

	return filtered, nil
}

func (f *LocalOnlyFilter) isLocalActor(actor *activitypub.Actor) bool {
	// Check if the actor ID contains our domain
	expectedPrefix := fmt.Sprintf("https://%s/", f.domain)
	return len(actor.ID) > len(expectedPrefix) && actor.ID[:len(expectedPrefix)] == expectedPrefix
}

// CompositeFilter applies multiple filters in sequence
type CompositeFilter struct {
	filters []SearchFilter
	logger  *zap.Logger
}

// NewCompositeFilter creates a filter that applies multiple filters
func NewCompositeFilter(filters []SearchFilter, logger *zap.Logger) *CompositeFilter {
	return &CompositeFilter{
		filters: filters,
		logger:  logger,
	}
}

func (f *CompositeFilter) Name() string {
	return "composite"
}

func (f *CompositeFilter) Filter(ctx context.Context, results []*SearchResult, userID string) ([]*SearchResult, error) {
	filtered := results

	for _, filter := range f.filters {
		var err error
		filtered, err = filter.Filter(ctx, filtered, userID)
		if err != nil {
			f.logger.Warn("filter failed",
				zap.String("filter", filter.Name()),
				zap.Error(err))
			// Continue with other filters even if one fails
		}
	}

	return filtered, nil
}

// ApplySearchFilters applies the appropriate filters based on search options
func ApplySearchFilters(ctx context.Context, results []*SearchResult, options SearchOptions, userID string, storage *dynamoDBStorage, domain string, logger *zap.Logger) ([]*SearchResult, error) {
	filters := make([]SearchFilter, 0)

	// Add following-only filter if requested
	if options.FollowingOnly && userID != "" {
		filters = append(filters, NewFollowingOnlyFilter(storage, logger))
	}

	// Add local-only filter if requested
	if !options.IncludeRemote {
		filters = append(filters, NewLocalOnlyFilter(domain, logger))
	}

	// Apply filters if any
	if len(filters) > 0 {
		compositeFilter := NewCompositeFilter(filters, logger)
		return compositeFilter.Filter(ctx, results, userID)
	}

	return results, nil
}
