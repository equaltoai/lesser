package dynamodb

import (
	"context"
	"crypto/md5"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// SearchCache provides caching for search results using DynamoDB with TTL
type SearchCache struct {
	dynamo    *dynamodb.Client
	tableName string
	ttl       time.Duration
}

// NewSearchCache creates a new search cache
func NewSearchCache(dynamo *dynamodb.Client, tableName string) *SearchCache {
	return &SearchCache{
		dynamo:    dynamo,
		tableName: tableName,
		ttl:       5 * time.Minute, // Default TTL
	}
}

// CachedResult represents a cached search result
type CachedResult struct {
	PK        string          `dynamodbav:"PK"`
	SK        string          `dynamodbav:"SK"`
	Results   []*SearchResult `dynamodbav:"Results"`
	Query     string          `dynamodbav:"Query"`
	CreatedAt time.Time       `dynamodbav:"CreatedAt"`
	TTL       int64           `dynamodbav:"TTL"`
}

// BuildKey creates a cache key from query and options
func (c *SearchCache) BuildKey(query string, options SearchOptions) string {
	// Create a deterministic key from query and options
	data := fmt.Sprintf("%s:%d:%d:%t:%s",
		query,
		options.Limit,
		options.Offset,
		options.FollowingOnly,
		options.Language)

	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// Get retrieves cached results if they exist and aren't expired
func (c *SearchCache) Get(ctx context.Context, key string) ([]*SearchResult, bool) {
	pk := fmt.Sprintf("SEARCH_CACHE#%s", key)
	sk := "RESULTS"

	result, err := c.dynamo.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(c.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	})

	if err != nil || result.Item == nil {
		return nil, false
	}

	// Check TTL
	if ttlAttr, ok := result.Item["TTL"]; ok {
		if ttlNum, ok := ttlAttr.(*types.AttributeValueMemberN); ok {
			ttl, _ := strconv.ParseInt(ttlNum.Value, 10, 64)
			if time.Now().Unix() > ttl {
				// Expired
				return nil, false
			}
		}
	}

	// Unmarshal results
	var cached CachedResult
	if err := attributevalue.UnmarshalMap(result.Item, &cached); err != nil {
		return nil, false
	}

	return cached.Results, true
}

// Set stores search results in the cache
func (c *SearchCache) Set(ctx context.Context, key string, results []*SearchResult) error {
	cached := CachedResult{
		PK:        fmt.Sprintf("SEARCH_CACHE#%s", key),
		SK:        "RESULTS",
		Results:   results,
		CreatedAt: time.Now(),
		TTL:       time.Now().Add(c.ttl).Unix(),
	}

	item, err := attributevalue.MarshalMap(cached)
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	// Store in DynamoDB
	_, err = c.dynamo.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(c.tableName),
		Item:      item,
	})

	return err
}

// Clear removes a cache entry
func (c *SearchCache) Clear(ctx context.Context, key string) error {
	_, err := c.dynamo.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(c.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("SEARCH_CACHE#%s", key)},
			"SK": &types.AttributeValueMemberS{Value: "RESULTS"},
		},
	})
	return err
}

// SetTTL updates the cache TTL duration
func (c *SearchCache) SetTTL(ttl time.Duration) {
	c.ttl = ttl
}
