package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// QueryUtils provides common query patterns used across repositories
type QueryUtils struct {
	db     core.DB
	logger *zap.Logger
}

// NewQueryUtils creates a new QueryUtils instance
func NewQueryUtils(db core.DB, logger *zap.Logger) *QueryUtils {
	return &QueryUtils{
		db:     db,
		logger: logger,
	}
}

// QueryResult represents a generic query result with pagination
type QueryResult[T any] struct {
	Items      []T
	NextCursor string
	HasMore    bool
}

// QueryOptions represents common query options
type QueryOptions struct {
	Limit      int
	Cursor     string
	SortOrder  string // "ASC" or "DESC"
	IndexName  string
	FilterExpr string
}

// UserRelationshipQuery performs a common pattern for querying user relationships
func (q *QueryUtils) UserRelationshipQuery(ctx context.Context, username, relationshipType string, opts *QueryOptions) (*QueryResult[map[string]interface{}], error) {
	if opts == nil {
		opts = &QueryOptions{Limit: 50}
	}

	pk := Utils.Keys.UserKey(username)
	skPrefix := fmt.Sprintf("%s#", relationshipType)

	var results []map[string]interface{}
	query := q.db.WithContext(ctx).Model(&results).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", skPrefix)

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit + 1) // +1 to check for more results
	}

	if opts.IndexName != "" {
		query = query.Index(opts.IndexName)
	}

	err := query.All(&results)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "user relationship", relationshipType)
	}

	// Handle pagination
	hasMore := len(results) > opts.Limit
	if hasMore {
		results = results[:opts.Limit]
	}

	nextCursor := ""
	if hasMore && len(results) > 0 {
		lastItem := results[len(results)-1]
		if pk, ok := lastItem["PK"].(string); ok {
			if sk, ok := lastItem["SK"].(string); ok {
				nextCursor = Utils.Pagination.EncodeCursor(pk, sk)
			}
		}
	}

	return &QueryResult[map[string]interface{}]{
		Items:      results,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// TimeRangeQuery performs queries with time range filtering
func (q *QueryUtils) TimeRangeQuery(ctx context.Context, pk string, startTime, endTime int64, opts *QueryOptions) (*QueryResult[map[string]interface{}], error) {
	if opts == nil {
		opts = &QueryOptions{Limit: 50}
	}

	var results []map[string]interface{}
	query := q.db.WithContext(ctx).Model(&results).
		Where("PK", "=", pk)

	// Add time range conditions
	if startTime > 0 {
		query = query.Where("SK", ">=", fmt.Sprintf("TIME#%d", startTime))
	}
	if endTime > 0 {
		query = query.Where("SK", "<=", fmt.Sprintf("TIME#%d", endTime))
	}

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit + 1)
	}

	if opts.IndexName != "" {
		query = query.Index(opts.IndexName)
	}

	err := query.All(&results)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "time range", pk)
	}

	// Handle pagination
	hasMore := len(results) > opts.Limit
	if hasMore {
		results = results[:opts.Limit]
	}

	nextCursor := ""
	if hasMore && len(results) > 0 {
		lastItem := results[len(results)-1]
		if pk, ok := lastItem["PK"].(string); ok {
			if sk, ok := lastItem["SK"].(string); ok {
				nextCursor = Utils.Pagination.EncodeCursor(pk, sk)
			}
		}
	}

	return &QueryResult[map[string]interface{}]{
		Items:      results,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// GSIStatusQuery performs common status-based GSI queries
func (q *QueryUtils) GSIStatusQuery(ctx context.Context, indexName, status string, opts *QueryOptions) (*QueryResult[map[string]interface{}], error) {
	if opts == nil {
		opts = &QueryOptions{Limit: 50}
	}

	var results []map[string]interface{}
	gsiPK := Utils.GSI.StatusIndexKey(status)

	query := q.db.WithContext(ctx).Model(&results).
		Index(indexName).
		Where(fmt.Sprintf("%sPK", indexName), "=", gsiPK)

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit + 1)
	}

	err := query.All(&results)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "GSI status", status)
	}

	// Handle pagination
	hasMore := len(results) > opts.Limit
	if hasMore {
		results = results[:opts.Limit]
	}

	nextCursor := ""
	if hasMore && len(results) > 0 {
		lastItem := results[len(results)-1]
		if pk, ok := lastItem["PK"].(string); ok {
			if sk, ok := lastItem["SK"].(string); ok {
				nextCursor = Utils.Pagination.EncodeCursor(pk, sk)
			}
		}
	}

	return &QueryResult[map[string]interface{}]{
		Items:      results,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// CountQuery performs count operations with consistent error handling
func (q *QueryUtils) CountQuery(ctx context.Context, pk string, indexName string) (int, error) {
	var model map[string]interface{}
	query := q.db.WithContext(ctx).Model(&model).
		Where("PK", "=", pk)

	if indexName != "" {
		query = query.Index(indexName)
	}

	count, err := query.Count()
	if err != nil {
		return 0, ErrorHandler.HandleQueryError(err, "count", pk)
	}

	return int(count), nil
}

// ExistsQuery checks if an item exists with consistent error handling
func (q *QueryUtils) ExistsQuery(ctx context.Context, pk, sk string) (bool, error) {
	count, err := q.db.WithContext(ctx).Model(&map[string]interface{}{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Count()

	if err != nil {
		return false, ErrorHandler.HandleQueryError(err, "exists", fmt.Sprintf("%s#%s", pk, sk))
	}

	return count > 0, nil
}

// BatchDeleteQuery performs batch delete operations
func (q *QueryUtils) BatchDeleteQuery(ctx context.Context, keys []struct{ PK, SK string }) error {
	if len(keys) == 0 {
		return nil
	}

	// DynamoDB batch operations have limits, so process in batches
	batchSize := 25 // DynamoDB batch write limit
	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}

		batch := keys[i:end]
		for _, key := range batch {
			err := q.db.WithContext(ctx).Model(&map[string]interface{}{}).
				Where("PK", "=", key.PK).
				Where("SK", "=", key.SK).
				Delete()

			if err != nil {
				return ErrorHandler.HandleDeleteError(err, "batch item", fmt.Sprintf("%s#%s", key.PK, key.SK))
			}
		}
	}

	return nil
}

// FilterActiveItems filters out expired/inactive items based on common patterns
func (q *QueryUtils) FilterActiveItems(items []map[string]interface{}, currentTimestamp int64) []map[string]interface{} {
	var activeItems []map[string]interface{}

	for _, item := range items {
		// Check for expiration
		if expiresAt, ok := item["ExpiresAt"].(int64); ok && expiresAt > 0 {
			if currentTimestamp > expiresAt {
				continue // Skip expired item
			}
		}

		// Check for revoked status
		if revoked, ok := item["Revoked"].(bool); ok && revoked {
			continue // Skip revoked item
		}

		// Check for active status
		if active, ok := item["Active"].(bool); ok && !active {
			continue // Skip inactive item
		}

		activeItems = append(activeItems, item)
	}

	return activeItems
}

// Common query patterns used across repositories
type CommonQueries struct {
	*QueryUtils
}

// NewCommonQueries creates a new CommonQueries instance
func NewCommonQueries(db core.DB, logger *zap.Logger) *CommonQueries {
	return &CommonQueries{
		QueryUtils: NewQueryUtils(db, logger),
	}
}

// GetUserFollows retrieves follows for a user with pagination
func (c *CommonQueries) GetUserFollows(ctx context.Context, username string, limit int, cursor string) (*QueryResult[map[string]interface{}], error) {
	return c.UserRelationshipQuery(ctx, username, "FOLLOWING", &QueryOptions{
		Limit:  limit,
		Cursor: cursor,
	})
}

// GetUserFollowers retrieves followers for a user with pagination  
func (c *CommonQueries) GetUserFollowers(ctx context.Context, username string, limit int, cursor string) (*QueryResult[map[string]interface{}], error) {
	return c.GSIStatusQuery(ctx, "gsi1-index", fmt.Sprintf("follow#%s", username), &QueryOptions{
		Limit:  limit,
		Cursor: cursor,
	})
}

// GetActiveTokensForUser retrieves active tokens for a user
func (c *CommonQueries) GetActiveTokensForUser(ctx context.Context, username string) (*QueryResult[map[string]interface{}], error) {
	result, err := c.UserRelationshipQuery(ctx, username, "TOKEN", &QueryOptions{
		Limit: 100, // Reasonable limit for tokens
	})
	if err != nil {
		return nil, err
	}

	// Filter out expired tokens
	currentTime := time.Now().Unix()
	result.Items = c.FilterActiveItems(result.Items, currentTime)

	return result, nil
}