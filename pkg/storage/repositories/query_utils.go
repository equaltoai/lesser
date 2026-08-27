package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
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

// paginateResults is a generic helper to handle pagination logic for any result type
func paginateResults[T any](results []T, opts *QueryOptions, extractKeys func(T) (pk, sk string)) (items []T, nextCursor string, hasMore bool) {
	if opts == nil || opts.Limit <= 0 {
		return results, "", false
	}

	hasMore = len(results) > opts.Limit
	if hasMore {
		results = results[:opts.Limit]
	}

	nextCursor = ""
	if hasMore && len(results) > 0 && extractKeys != nil {
		if pk, sk := extractKeys(results[len(results)-1]); pk != "" && sk != "" {
			nextCursor = Utils.Pagination.EncodeCursor(pk, sk)
		}
	}

	return results, nextCursor, hasMore
}

// mapExtractKeys extracts PK and SK from a map[string]interface{}
func mapExtractKeys(item map[string]interface{}) (pk, sk string) {
	if p, ok := item["PK"].(string); ok {
		pk = p
	}
	if s, ok := item["SK"].(string); ok {
		sk = s
	}
	return pk, sk
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

// QueryUtils default and hard-max limits for the conditional-limit builders
// (wave #1469 BOUNDED-QUERY class): a caller that omits the limit (or passes
// 0) gets the default page size instead of an unbounded partition read, and
// anything above the hard max is clamped so no caller can request an
// unbounded read.
const (
	defaultQueryUtilsLimit = 50
	maxQueryUtilsLimit     = 100
)

// sanitizeQueryLimit normalizes a requested limit for the query-utils
// conditional-limit builders (wave #1469): <= 0 becomes the default page size
// and > maxQueryUtilsLimit is clamped, so the builders always issue a bounded
// read.
func sanitizeQueryLimit(limit int) int {
	if limit <= 0 {
		return defaultQueryUtilsLimit
	}
	if limit > maxQueryUtilsLimit {
		return maxQueryUtilsLimit
	}
	return limit
}

// errBoundedPageCapExceeded is the fail-closed guard for bounded whole-partition
// iterations (wave #1469 BOUNDED-QUERY class): a keyed partition read that
// cannot be exhausted within the sanctioned page cap fails closed with this
// sentinel instead of reading the partition unboundedly.
var errBoundedPageCapExceeded = fmt.Errorf("bounded page iteration exceeded the page cap")

// walkKeyedPages iterates a keyed TableTheory query in bounded pages with an
// explicit page cap (wave #1469). Each page carries pageSize items; iteration
// stops when the query reports no more pages (HasMore false or no cursor),
// when consume returns stop=true, or when the page cap is exhausted — in which
// case the read fails closed with errBoundedPageCapExceeded. Page order follows
// the query's sort order via ExclusiveStartKey cursors, so ordering, window,
// and filter semantics of the underlying query are preserved.
func walkKeyedPages[T any](query core.Query, pageSize, maxPages int, consume func(page []T) (stop bool, err error)) error {
	if pageSize <= 0 {
		pageSize = 500
	}
	if maxPages <= 0 {
		maxPages = 100
	}
	query = query.Limit(pageSize)
	var cursor string
	for page := 0; ; page++ {
		if page >= maxPages {
			return fmt.Errorf("%w: exhausted %d pages of %d", errBoundedPageCapExceeded, maxPages, pageSize)
		}
		paged := query
		if cursor != "" {
			paged = paged.Cursor(cursor)
		}
		var items []T
		res, err := paged.AllPaginated(&items)
		if err != nil {
			return err
		}
		stop, err := consume(items)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
		if res == nil || !res.HasMore || res.NextCursor == "" {
			return nil
		}
		cursor = res.NextCursor
	}
}

// UserRelationshipQuery performs a common pattern for querying user relationships
func (q *QueryUtils) UserRelationshipQuery(ctx context.Context, username, relationshipType string, opts *QueryOptions) (*QueryResult[map[string]interface{}], error) {
	if opts == nil {
		opts = &QueryOptions{Limit: 50}
	}

	pk := Utils.Keys.UserKey(username)
	skPrefix := fmt.Sprintf("%s#", relationshipType)

	// Clamp the requested limit (wave #1469): the read is always bounded — a
	// 0/omitted limit gets the default page size and oversized limits are cut.
	opts.Limit = sanitizeQueryLimit(opts.Limit)

	var results []map[string]interface{}
	query := q.db.WithContext(ctx).Model(&results).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", skPrefix).
		Limit(opts.Limit + 1) // +1 to check for more results

	if opts.IndexName != "" {
		query = query.Index(opts.IndexName)
	}

	err := query.All(&results)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "user relationship", relationshipType)
	}

	// Use generic pagination helper
	items, nextCursor, hasMore := paginateResults(results, opts, mapExtractKeys)

	return &QueryResult[map[string]interface{}]{
		Items:      items,
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

	// Clamp the requested limit (wave #1469): the read is always bounded.
	opts.Limit = sanitizeQueryLimit(opts.Limit)

	// Add time range conditions
	if startTime > 0 {
		query = query.Where("SK", ">=", fmt.Sprintf("TIME#%d", startTime))
	}
	if endTime > 0 {
		query = query.Where("SK", "<=", fmt.Sprintf("TIME#%d", endTime))
	}

	query = query.Limit(opts.Limit + 1) // +1 to check for more results

	if opts.IndexName != "" {
		query = query.Index(opts.IndexName)
	}

	err := query.All(&results)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "time range", pk)
	}

	// Use generic pagination helper
	items, nextCursor, hasMore := paginateResults(results, opts, mapExtractKeys)

	return &QueryResult[map[string]interface{}]{
		Items:      items,
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

	// Clamp the requested limit (wave #1469): the read is always bounded.
	opts.Limit = sanitizeQueryLimit(opts.Limit)

	query := q.db.WithContext(ctx).Model(&results).
		Index(indexName).
		Where(fmt.Sprintf("%sPK", strings.ToLower(indexName)), "=", gsiPK).
		Limit(opts.Limit + 1) // +1 to check for more results

	err := query.All(&results)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "GSI status", status)
	}

	// Use generic pagination helper
	items, nextCursor, hasMore := paginateResults(results, opts, mapExtractKeys)

	return &QueryResult[map[string]interface{}]{
		Items:      items,
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
	if err := common.ValidateSliceNotEmpty("keys", keys); err != nil {
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
	activeItems := make([]map[string]interface{}, 0, len(items))

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

// GetItemByPK retrieves a single item by its primary key and sort key
func (q *QueryUtils) GetItemByPK(ctx context.Context, pk, sk string, result interface{}) error {
	err := q.db.WithContext(ctx).Model(result).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(result)

	if err != nil {
		return ErrorHandler.HandleQueryError(err, "get item", fmt.Sprintf("%s#%s", pk, sk))
	}

	return nil
}

// UpdateItem performs a generic update operation with error handling
func (q *QueryUtils) UpdateItem(ctx context.Context, model interface{}) error {
	err := q.db.WithContext(ctx).Model(model).Update()
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, "update item", "")
	}
	return nil
}

// DeleteItem performs a generic delete operation with error handling
func (q *QueryUtils) DeleteItem(ctx context.Context, pk, sk string, model interface{}) error {
	err := q.db.WithContext(ctx).Model(model).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		return ErrorHandler.HandleDeleteError(err, "delete item", fmt.Sprintf("%s#%s", pk, sk))
	}

	return nil
}

// DeleteWithNotFoundHandling performs a delete operation that treats NotFound as success
// This is a common pattern where deletion is idempotent (deleting non-existing items succeeds)
func (q *QueryUtils) DeleteWithNotFoundHandling(ctx context.Context, pk, sk string, model interface{}, operationType, param1, param2 string) error {
	err := q.db.WithContext(ctx).Model(model).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		if errors.IsNotFound(err) {
			q.logger.Debug(fmt.Sprintf("%s not found", operationType),
				zap.String("param1", param1),
				zap.String("param2", param2))
			return nil
		}
		q.logger.Error(fmt.Sprintf("failed to %s", operationType),
			zap.String("param1", param1),
			zap.String("param2", param2),
			zap.Error(err))
		return fmt.Errorf("%w: %s: %w", ErrQueryOperationFailed, operationType, err)
	}

	q.logger.Info(fmt.Sprintf("%s successful", operationType),
		zap.String("param1", param1),
		zap.String("param2", param2))

	return nil
}

// QueryByGSI performs a generic GSI query with pagination
func (q *QueryUtils) QueryByGSI(ctx context.Context, indexName, gsiPK, gsiSK string, opts *QueryOptions) (*QueryResult[map[string]interface{}], error) {
	if opts == nil {
		opts = &QueryOptions{Limit: 50}
	}

	var results []map[string]interface{}
	query := q.db.WithContext(ctx).Model(&results).
		Index(indexName).
		Where(fmt.Sprintf("%sPK", strings.ToLower(indexName)), "=", gsiPK)

	// Clamp the requested limit (wave #1469): the read is always bounded.
	opts.Limit = sanitizeQueryLimit(opts.Limit)

	if gsiSK != "" {
		query = query.Where(fmt.Sprintf("%sSK", strings.ToLower(indexName)), "=", gsiSK)
	}

	query = query.Limit(opts.Limit + 1) // +1 to check for more results

	err := query.All(&results)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "GSI query", gsiPK)
	}

	// Use generic pagination helper
	items, nextCursor, hasMore := paginateResults(results, opts, mapExtractKeys)

	return &QueryResult[map[string]interface{}]{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// QueryWithPrefix performs a query with SK prefix matching
func (q *QueryUtils) QueryWithPrefix(ctx context.Context, pk, skPrefix string, opts *QueryOptions) (*QueryResult[map[string]interface{}], error) {
	if opts == nil {
		opts = &QueryOptions{Limit: 50}
	}

	var results []map[string]interface{}
	query := q.db.WithContext(ctx).Model(&results).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", skPrefix)

	// Clamp the requested limit (wave #1469): the read is always bounded.
	opts.Limit = sanitizeQueryLimit(opts.Limit)

	query = query.Limit(opts.Limit + 1) // +1 to check for more results

	if opts.IndexName != "" {
		query = query.Index(opts.IndexName)
	}

	err := query.All(&results)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "prefix query", pk)
	}

	// Use generic pagination helper
	items, nextCursor, hasMore := paginateResults(results, opts, mapExtractKeys)

	return &QueryResult[map[string]interface{}]{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// CreateWithCondition creates an item with condition checking (for idempotency)
func (q *QueryUtils) CreateWithCondition(ctx context.Context, model interface{}) error {
	err := q.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		// Check if it's a duplicate key error
		if errors.IsConditionFailed(err) {
			q.logger.Debug("item already exists", zap.Any("model", model))
			return nil // Idempotent - don't fail if already exists
		}
		return ErrorHandler.HandleCreateError(err, "create item", "")
	}
	return nil
}

// GenericQuery performs a type-safe query with automatic struct mapping
func GenericQuery[T any](ctx context.Context, q *QueryUtils, pk, sk string) (*T, error) {
	var result T
	err := q.db.WithContext(ctx).Model(&result).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&result)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "generic query", fmt.Sprintf("%s#%s", pk, sk))
		}
		return nil, ErrorHandler.HandleQueryError(err, "generic query", fmt.Sprintf("%s#%s", pk, sk))
	}

	return &result, nil
}

// GenericList performs a type-safe list query with pagination
func GenericList[T any](ctx context.Context, q *QueryUtils, pk, skPrefix string, opts *QueryOptions) (*QueryResult[T], error) {
	if opts == nil {
		opts = &QueryOptions{Limit: 50}
	}

	var results []T
	query := q.db.WithContext(ctx).Model(&results).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", skPrefix)

	// Clamp the requested limit (wave #1469): the read is always bounded.
	opts.Limit = sanitizeQueryLimit(opts.Limit)

	query = query.Limit(opts.Limit + 1) // +1 to check for more results

	if opts.IndexName != "" {
		query = query.Index(opts.IndexName)
	}

	err := query.All(&results)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "generic list", pk)
	}

	// Handle pagination
	hasMore := len(results) > opts.Limit
	if hasMore {
		results = results[:opts.Limit]
	}

	// For typed results, we need a way to extract cursor from the struct
	// This is a simplified version - in practice you'd want a Keyer interface
	nextCursor := ""
	if hasMore && len(results) > 0 {
		// This would need proper implementation based on your model structure
		nextCursor = fmt.Sprintf("next_%d", opts.Limit)
	}

	return &QueryResult[T]{
		Items:      results,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// BatchGet performs a batch get operation for multiple items
func BatchGet[T any](ctx context.Context, q *QueryUtils, keys []struct{ PK, SK string }) ([]T, error) {
	if err := common.ValidateSliceNotEmpty("keys", keys); err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "batch get", "keys")
	}

	results := make([]T, 0, len(keys))
	for _, key := range keys {
		var item T
		err := q.db.WithContext(ctx).Model(&item).
			Where("PK", "=", key.PK).
			Where("SK", "=", key.SK).
			First(&item)

		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, ErrorHandler.HandleQueryError(err, "batch get", fmt.Sprintf("%s#%s", key.PK, key.SK))
			}
			// Skip not found items
			continue
		}

		results = append(results, item)
	}

	return results, nil
}

// QueryBuilder provides a fluent interface for building queries with less duplication
type QueryBuilder[T any] struct {
	q         *QueryUtils
	ctx       context.Context
	pk        string
	sk        string
	skPrefix  string
	indexName string
	limit     int
}

// NewQueryBuilder creates a new query builder
func NewQueryBuilder[T any](ctx context.Context, q *QueryUtils) *QueryBuilder[T] {
	return &QueryBuilder[T]{
		q:     q,
		ctx:   ctx,
		limit: 50,
	}
}

// WithPK sets the partition key
func (qb *QueryBuilder[T]) WithPK(pk string) *QueryBuilder[T] {
	qb.pk = pk
	return qb
}

// WithSK sets the sort key
func (qb *QueryBuilder[T]) WithSK(sk string) *QueryBuilder[T] {
	qb.sk = sk
	return qb
}

// WithSKPrefix sets the sort key prefix for BEGINS_WITH queries
func (qb *QueryBuilder[T]) WithSKPrefix(prefix string) *QueryBuilder[T] {
	qb.skPrefix = prefix
	return qb
}

// WithIndex sets the GSI index name
func (qb *QueryBuilder[T]) WithIndex(indexName string) *QueryBuilder[T] {
	qb.indexName = indexName
	return qb
}

// WithLimit sets the query limit
func (qb *QueryBuilder[T]) WithLimit(limit int) *QueryBuilder[T] {
	qb.limit = limit
	return qb
}

// Execute runs the query and returns paginated results
func (qb *QueryBuilder[T]) Execute() (*QueryResult[T], error) {
	// Clamp the requested limit (wave #1469): the read is always bounded —
	// WithLimit(0) (or an omitted limit) gets the default page size.
	limit := sanitizeQueryLimit(qb.limit)

	var results []T
	query := qb.q.db.WithContext(qb.ctx).Model(&results)

	if qb.indexName != "" {
		query = query.Index(qb.indexName)
	}

	if qb.pk != "" {
		query = query.Where("PK", "=", qb.pk)
	}

	if qb.sk != "" {
		query = query.Where("SK", "=", qb.sk)
	} else if qb.skPrefix != "" {
		query = query.Where("SK", "BEGINS_WITH", qb.skPrefix)
	}

	query = query.Limit(limit + 1) // +1 to check for more results

	err := query.All(&results)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "query builder", qb.pk)
	}

	// For generic types, we need a type-specific key extractor
	// This is a simplified version - could be improved with interfaces
	opts := &QueryOptions{Limit: limit}
	items, nextCursor, hasMore := paginateResults(results, opts, nil)

	return &QueryResult[T]{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// CommonQueries contains common query patterns used across repositories
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
	return c.GSIStatusQuery(ctx, "gsi1", Utils.Keys.FollowKey(username), &QueryOptions{
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

// AddToCollectionHelper provides a shared implementation for adding items to collections
// This eliminates duplication between ObjectRepository and RelationshipRepository AddToCollection methods
func (q *QueryUtils) AddToCollectionHelper(ctx context.Context, collection string, item *storage.CollectionItem, db core.DB) error {
	collectionItem := models.NewCollectionItem(collection, item.ItemID, item.ItemType, item.AddedBy)
	collectionItem.Position = item.Position

	if err := db.WithContext(ctx).Model(collectionItem).Create(); err != nil {
		if errors.IsConditionFailed(err) {
			q.logger.Info("item already in collection",
				zap.String("collection", collection),
				zap.String("item_id", item.ItemID))
			return nil // Not an error to add something already in collection
		}
		q.logger.Error("failed to add to collection",
			zap.String("collection", collection),
			zap.String("item_id", item.ItemID),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrQueryCollectionAddFailed, err)
	}

	q.logger.Info("added item to collection",
		zap.String("collection", collection),
		zap.String("item_id", item.ItemID))

	return nil
}

// QueryAndConvert performs a database query and converts the results to storage types
// This eliminates the common pattern of: query → error check → convert loop → return
func QueryAndConvert[M any, S any](
	_ context.Context,
	q *QueryUtils,
	queryFunc func() ([]M, error),
	convertFunc func(M) S,
	operationName string,
	operationParam string,
) ([]S, error) {
	// Execute the query
	models, err := queryFunc()
	if err != nil {
		q.logger.Error(fmt.Sprintf("Failed to %s", operationName),
			zap.String("param", operationParam),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %s: %w", ErrQueryExecutionFailed, operationName, err)
	}

	// Convert models to storage types
	results := make([]S, len(models))
	for i, model := range models {
		results[i] = convertFunc(model)
	}

	return results, nil
}

// QueryWithPKAndSKPrefix eliminates the PK/SK prefix query duplication pattern
// This consolidates the common "Where PK = X, Where/Filter SK BEGINS_WITH Y" pattern
func QueryWithPKAndSKPrefix[M any, S any](
	ctx context.Context,
	q *QueryUtils,
	modelFactory func() *M,
	pkValue, skPrefix string,
	useFilter bool, // true for Filter("SK", "BEGINS_WITH"), false for Where("SK", "BEGINS_WITH")
	convertFunc func(M) S,
	operationName string,
	operationParam string,
) ([]S, error) {
	var models []M
	var err error

	if useFilter {
		err = q.db.WithContext(ctx).Model(modelFactory()).
			Where("PK", "=", pkValue).
			Filter("SK", "BEGINS_WITH", skPrefix).
			Scan(&models)
	} else {
		// The contract returns the full partition, so the read is a bounded page
		// walk with an explicit page cap (wave #1469): 500 items/page, 100 pages
		// max, fail-closed on exhaustion.
		err = walkKeyedPages(
			q.db.WithContext(ctx).Model(modelFactory()).
				Where("PK", "=", pkValue).
				Where("SK", "BEGINS_WITH", skPrefix),
			500, 100,
			func(page []M) (bool, error) {
				models = append(models, page...)
				return false, nil
			},
		)
	}

	if err != nil {
		q.logger.Error(fmt.Sprintf("Failed to %s", operationName),
			zap.String("param", operationParam),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %s: %w", ErrQueryExecutionFailed, operationName, err)
	}

	// Convert models to storage types
	results := make([]S, len(models))
	for i, model := range models {
		results[i] = convertFunc(model)
	}

	return results, nil
}
