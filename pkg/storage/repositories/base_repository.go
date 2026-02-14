// Package repositories provides DynamoDB-backed repository implementations using DynamORM.
// This package contains the core repository layer that abstracts database operations,
// implements the Lift framework patterns, and provides comprehensive cost tracking.
package repositories

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
)

// Sort order constants
const (
	SortOrderAsc  = "ASC"
	SortOrderDesc = "DESC"
)

// Default limit constants for shared repository helpers
const (
	defaultBaseQueryLimit       = 100
	maxBaseQueryLimit           = 500
	defaultFilterQueryLimit     = 50
	maxFilterQueryLimit         = 200
	defaultCollectionQueryLimit = 50
	maxCollectionQueryLimit     = 200
	defaultAggregatedQueryLimit = 100
	maxAggregatedQueryLimit     = 500
)

// BaseModel interface that all DynamoDB models must implement
type BaseModel interface {
	UpdateKeys() error
	GetPK() string
	GetSK() string
}

// BaseRepository provides common CRUD operations for all repositories with integrated cost tracking
type BaseRepository[T BaseModel] struct {
	db          core.DB
	tableName   string
	logger      *zap.Logger
	costService *cost.TrackingService
	repoName    string
}

// modelPrototypeOf returns a pointer to the underlying struct type for a generic parameter.
func modelPrototypeOf[T BaseModel]() interface{} {
	modelType := reflect.TypeOf((*T)(nil)).Elem()
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}
	return reflect.New(modelType).Interface()
}

// clampLimit normalizes a requested limit to the configured defaults.
// Returns the sanitized limit, whether it was clamped, and whether the default was applied.
func clampLimit(limit, defaultLimit, maxLimit int) (sanitized int, clamped bool, usedDefault bool) {
	switch {
	case limit <= 0:
		return defaultLimit, true, true
	case limit > maxLimit:
		return maxLimit, true, false
	default:
		return limit, false, false
	}
}

// NewBaseRepository creates a new base repository
func NewBaseRepository[T BaseModel](db core.DB, tableName string, logger *zap.Logger) *BaseRepository[T] {
	return &BaseRepository[T]{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// NewBaseRepositoryWithCostTracking creates a new base repository with integrated cost tracking
func NewBaseRepositoryWithCostTracking[T BaseModel](db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService, repoName string) *BaseRepository[T] {
	return &BaseRepository[T]{
		db:          db,
		tableName:   tableName,
		logger:      logger,
		costService: costService,
		repoName:    repoName,
	}
}

// Create stores a new item in the database
func (r *BaseRepository[T]) Create(ctx context.Context, item T) error {
	if r == nil {
		return pkgErrors.DatabaseConnectionFailed(fmt.Errorf("repository is nil"))
	}
	if r.db == nil {
		return pkgErrors.DatabaseConnectionFailed(fmt.Errorf("%s database client is nil", r.repoName))
	}

	// Update keys before saving
	if err := item.UpdateKeys(); err != nil {
		return ErrorHandler.HandleCreateError(err, "base entity keys", item.GetPK())
	}

	// Track cost if cost service is available
	if r.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "PutItem",
			TableName:          r.tableName,
			ConsumedReadUnits:  0,
			ConsumedWriteUnits: 1, // Estimated 1 WU for item creation
			ItemCount:          1,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("%s_create_%d", r.repoName, time.Now().UnixNano()),
		}

		defer func() {
			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn("failed to track DynamoDB create operation cost",
					zap.String("repository", r.repoName),
					zap.String("pk", item.GetPK()),
					zap.Error(trackErr))
			}
		}()
	}

	// Create the item
	err := r.db.WithContext(ctx).Model(item).Create()
	if err != nil {
		r.logger.Error("failed to create item",
			zap.Error(err),
			zap.String("pk", item.GetPK()),
			zap.String("sk", item.GetSK()))
		return ErrorHandler.HandleCreateError(err, "base entity", item.GetPK())
	}

	return nil
}

// CreateIfNotExists stores a new item in the database only if its primary key does not already exist.
func (r *BaseRepository[T]) CreateIfNotExists(ctx context.Context, item T) error {
	// Update keys before saving
	if err := item.UpdateKeys(); err != nil {
		return ErrorHandler.HandleCreateError(err, "base entity keys", item.GetPK())
	}

	// Track cost if cost service is available
	if r.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "PutItem",
			TableName:          r.tableName,
			ConsumedReadUnits:  0,
			ConsumedWriteUnits: 1, // Estimated 1 WU for item creation attempt
			ItemCount:          1,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("%s_create_if_not_exists_%d", r.repoName, time.Now().UnixNano()),
		}

		defer func() {
			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn("failed to track DynamoDB create operation cost",
					zap.String("repository", r.repoName),
					zap.String("pk", item.GetPK()),
					zap.Error(trackErr))
			}
		}()
	}

	// Create the item with a conditional check
	err := r.db.WithContext(ctx).Model(item).IfNotExists().Create()
	if err != nil {
		r.logger.Error("failed to create item",
			zap.Error(err),
			zap.String("pk", item.GetPK()),
			zap.String("sk", item.GetSK()))
		return ErrorHandler.HandleCreateError(err, "base entity", item.GetPK())
	}

	return nil
}

// BatchGetItems retrieves multiple items by their keys in a single batch operation
func (r *BaseRepository[T]) BatchGetItems(ctx context.Context, keys []map[string]interface{}) ([]T, error) {
	if len(keys) == 0 {
		return []T{}, nil
	}

	// DynamoDB BatchGetItem has a limit of 100 items
	const batchSize = 100
	var allResults []T

	// Track cost if cost service is available
	if r.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "BatchGetItem",
			TableName:          r.tableName,
			ConsumedReadUnits:  int64(len(keys)), // Estimated 1 RU per item
			ConsumedWriteUnits: 0,
			ItemCount:          int64(len(keys)),
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("%s_batch_get_%d", r.repoName, time.Now().UnixNano()),
		}

		defer func() {
			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn("failed to track DynamoDB batch get operation cost",
					zap.String("repository", r.repoName),
					zap.Int("key_count", len(keys)),
					zap.Error(trackErr))
			}
		}()
	}

	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}

		batchKeys := keys[i:end]
		batchResults, err := r.batchGetBatch(ctx, batchKeys)
		if err != nil {
			r.logger.Error("failed to batch get items",
				zap.Error(err),
				zap.Int("batch_size", len(batchKeys)))
			return nil, ErrorHandler.HandleGetError(err, "batch items", fmt.Sprintf("batch_%d", i))
		}

		allResults = append(allResults, batchResults...)
	}

	return allResults, nil
}

// BatchWriteItems writes multiple items in a single batch operation
func (r *BaseRepository[T]) BatchWriteItems(ctx context.Context, items []T) error {
	if len(items) == 0 {
		return nil
	}

	// Update keys for all items before writing
	for i, item := range items {
		if err := item.UpdateKeys(); err != nil {
			return ErrorHandler.HandleCreateError(err, "batch entity keys", fmt.Sprintf("item_%d_%s", i, item.GetPK()))
		}
	}

	// DynamoDB BatchWriteItem has a limit of 25 items
	const batchSize = 25

	// Track cost if cost service is available
	if r.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "BatchWriteItem",
			TableName:          r.tableName,
			ConsumedReadUnits:  0,
			ConsumedWriteUnits: int64(len(items)), // Estimated 1 WU per item
			ItemCount:          int64(len(items)),
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("%s_batch_write_%d", r.repoName, time.Now().UnixNano()),
		}

		defer func() {
			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn("failed to track DynamoDB batch write operation cost",
					zap.String("repository", r.repoName),
					zap.Int("item_count", len(items)),
					zap.Error(trackErr))
			}
		}()
	}

	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}

		batch := items[i:end]
		if err := r.batchWriteBatch(ctx, batch); err != nil {
			r.logger.Error("failed to batch write items",
				zap.Error(err),
				zap.Int("batch_size", len(batch)))
			return ErrorHandler.HandleCreateError(err, "batch items", fmt.Sprintf("batch_%d", i))
		}
	}

	return nil
}

// TransactWrite performs a transaction with multiple write operations
func (r *BaseRepository[T]) TransactWrite(ctx context.Context, items []T) error {
	if len(items) == 0 {
		return nil
	}

	// DynamoDB TransactWriteItems has a limit of 25 items
	if len(items) > 25 {
		return fmt.Errorf("transaction write supports maximum 25 items, got %d", len(items))
	}

	// Update keys for all items
	for i, item := range items {
		if err := item.UpdateKeys(); err != nil {
			return ErrorHandler.HandleCreateError(err, "transaction entity keys", fmt.Sprintf("item_%d_%s", i, item.GetPK()))
		}
	}

	// Track cost if cost service is available
	if r.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "TransactWriteItems",
			TableName:          r.tableName,
			ConsumedReadUnits:  0,
			ConsumedWriteUnits: int64(len(items) * 2), // Estimated 2 WU per item for transactions
			ItemCount:          int64(len(items)),
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("%s_transact_write_%d", r.repoName, time.Now().UnixNano()),
		}

		defer func() {
			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn("failed to track DynamoDB transaction write operation cost",
					zap.String("repository", r.repoName),
					zap.Int("item_count", len(items)),
					zap.Error(trackErr))
			}
		}()
	}

	return r.transactWriteBatch(ctx, items)
}

// Helper methods for batch operations

func (r *BaseRepository[T]) batchGetBatch(ctx context.Context, keys []map[string]interface{}) ([]T, error) {
	var results []T
	var model T

	// This would need to be implemented using the DynamORM batch get capabilities
	// For now, fall back to individual gets (this should be optimized in the future)
	for _, key := range keys {
		pk, ok := key["PK"].(string)
		if !ok {
			continue
		}
		sk, ok := key["SK"].(string)
		if !ok {
			continue
		}

		err := r.db.WithContext(ctx).Model(&model).
			Where("PK", "=", pk).
			Where("SK", "=", sk).
			First(&model)

		if err != nil {
			if errors.IsNotFound(err) {
				continue // Skip not found items in batch operations
			}
			return nil, err
		}

		results = append(results, model)
	}

	return results, nil
}

func (r *BaseRepository[T]) batchWriteBatch(ctx context.Context, items []T) error {
	// This would need to be implemented using DynamORM batch write capabilities
	// For now, fall back to individual creates (this should be optimized in the future)
	for _, item := range items {
		if err := r.db.WithContext(ctx).Model(item).Create(); err != nil {
			return err
		}
	}
	return nil
}

func (r *BaseRepository[T]) transactWriteBatch(ctx context.Context, items []T) error {
	// This would need to be implemented using DynamORM transaction capabilities
	// For now, fall back to individual creates (this should be optimized in the future)
	// In a real implementation, this should use DynamoDB transactions for atomicity
	for _, item := range items {
		if err := r.db.WithContext(ctx).Model(item).Create(); err != nil {
			return err
		}
	}
	return nil
}

// Get retrieves a single item by primary and sort key
func (r *BaseRepository[T]) Get(ctx context.Context, pk, sk string, result T) error {
	if r == nil {
		return pkgErrors.DatabaseConnectionFailed(fmt.Errorf("repository is nil"))
	}
	if r.db == nil {
		return pkgErrors.DatabaseConnectionFailed(fmt.Errorf("%s database client is nil", r.repoName))
	}

	// Track cost if cost service is available
	if r.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "GetItem",
			TableName:          r.tableName,
			ConsumedReadUnits:  1, // Estimated 1 RU for item retrieval
			ConsumedWriteUnits: 0,
			ItemCount:          1,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("%s_get_%d", r.repoName, time.Now().UnixNano()),
		}

		defer func() {
			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn("failed to track DynamoDB get operation cost",
					zap.String("repository", r.repoName),
					zap.String("pk", pk),
					zap.Error(trackErr))
			}
		}()
	}

	// Get the item
	err := r.db.WithContext(ctx).Model(result).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(result)

	if err != nil {
		if errors.IsNotFound(err) {
			return ErrorHandler.HandleGetError(err, "base entity", fmt.Sprintf("pk=%s, sk=%s", pk, sk))
		}
		r.logger.Error("failed to get item",
			zap.Error(err),
			zap.String("table", r.tableName),
			zap.String("pk", pk),
			zap.String("sk", sk))
		return ErrorHandler.HandleGetError(err, "base entity", fmt.Sprintf("pk=%s, sk=%s", pk, sk))
	}

	return nil
}

// Update updates an existing item
// Note: In DynamORM, you need to update the model fields before calling Update()
// This method is provided for consistency but may need adaptation per repository
func (r *BaseRepository[T]) Update(ctx context.Context, item T) error {
	if r == nil {
		return pkgErrors.DatabaseConnectionFailed(fmt.Errorf("repository is nil"))
	}
	if r.db == nil {
		return pkgErrors.DatabaseConnectionFailed(fmt.Errorf("%s database client is nil", r.repoName))
	}

	// Track cost if cost service is available
	if r.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "UpdateItem",
			TableName:          r.tableName,
			ConsumedReadUnits:  0,
			ConsumedWriteUnits: 1, // Estimated 1 WU for item update
			ItemCount:          1,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("%s_update_%d", r.repoName, time.Now().UnixNano()),
		}

		defer func() {
			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn("failed to track DynamoDB update operation cost",
					zap.String("repository", r.repoName),
					zap.String("pk", item.GetPK()),
					zap.Error(trackErr))
			}
		}()
	}

	// Update the item
	err := r.db.WithContext(ctx).Model(item).Update()

	if err != nil {
		r.logger.Error("failed to update item",
			zap.Error(err),
			zap.String("pk", item.GetPK()),
			zap.String("sk", item.GetSK()))
		return ErrorHandler.HandleUpdateError(err, "base entity", item.GetPK())
	}

	return nil
}

// Delete removes an item from the database
func (r *BaseRepository[T]) Delete(ctx context.Context, pk, sk string) error {
	if r == nil {
		return pkgErrors.DatabaseConnectionFailed(fmt.Errorf("repository is nil"))
	}
	if r.db == nil {
		return pkgErrors.DatabaseConnectionFailed(fmt.Errorf("%s database client is nil", r.repoName))
	}

	// Track cost if cost service is available
	if r.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "DeleteItem",
			TableName:          r.tableName,
			ConsumedReadUnits:  0,
			ConsumedWriteUnits: 1, // Estimated 1 WU for item deletion
			ItemCount:          1,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("%s_delete_%d", r.repoName, time.Now().UnixNano()),
		}

		defer func() {
			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn("failed to track DynamoDB delete operation cost",
					zap.String("repository", r.repoName),
					zap.String("pk", pk),
					zap.Error(trackErr))
			}
		}()
	}

	// Create a proper model instance for DynamORM
	// Handle pointer types correctly - DynamORM needs a non-nil pointer or value
	// Use reflection to get the concrete type of T
	var zero T
	tType := reflect.TypeOf((*T)(nil)).Elem()

	var model T
	// If T is a pointer type, create a new instance
	if tType.Kind() == reflect.Ptr {
		elemType := tType.Elem()
		ptrValue := reflect.New(elemType)
		model = ptrValue.Interface().(T)
	} else {
		// T is a value type, use zero value
		model = zero
	}

	// Delete the item
	err := r.db.WithContext(ctx).Model(model).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		r.logger.Error("failed to delete item",
			zap.Error(err),
			zap.String("pk", pk),
			zap.String("sk", sk))
		return ErrorHandler.HandleDeleteError(err, "base entity", fmt.Sprintf("pk=%s, sk=%s", pk, sk))
	}

	return nil
}

// Query performs a query operation on a partition key
func (r *BaseRepository[T]) Query(ctx context.Context, pk string, limit int) ([]T, error) {
	var results []T

	safeLimit, clamped, usedDefault := clampLimit(limit, defaultBaseQueryLimit, maxBaseQueryLimit)
	if clamped && r.logger != nil {
		fields := []zap.Field{
			zap.String("pk", pk),
			zap.Int("requested_limit", limit),
			zap.Int("applied_limit", safeLimit),
		}
		message := "base query limit clamped"
		if usedDefault {
			message = "base query called without limit; applying default"
		}
		r.logger.Warn(message, append(fields, zap.String("repository", r.repoName))...)
	}

	// Create query with sentinel fetch
	query := r.db.WithContext(ctx).Model(modelPrototypeOf[T]()).
		Where("PK", "=", pk).
		Limit(safeLimit + 1) // fetch sentinel record to detect additional pages

	// Execute query
	err := query.All(&results)
	if err != nil {
		r.logger.Error("failed to query items",
			zap.Error(err),
			zap.String("pk", pk),
			zap.Int("limit", safeLimit))
		return nil, ErrorHandler.HandleQueryError(err, "base entity", "partition query")
	}

	hasMore := len(results) > safeLimit
	if hasMore {
		results = results[:safeLimit]
		if r.logger != nil {
			r.logger.Debug("base query returned additional pages; follow-up pagination recommended",
				zap.String("pk", pk),
				zap.Int("limit", safeLimit),
				zap.String("repository", r.repoName))
		}
	}

	// Track cost if cost service is available
	if r.costService != nil {
		itemCount := int64(len(results))
		if itemCount == 0 {
			itemCount = 1 // minimum charge for the query execution
		}

		operation := cost.DynamoOperation{
			Type:               "Query",
			TableName:          r.tableName,
			ConsumedReadUnits:  itemCount,
			ConsumedWriteUnits: 0,
			ItemCount:          itemCount,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("%s_query_%d", r.repoName, time.Now().UnixNano()),
		}

		if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
			r.logger.Warn("failed to track DynamoDB query operation cost",
				zap.String("repository", r.repoName),
				zap.String("pk", pk),
				zap.Error(trackErr))
		}
	}

	return results, nil
}

// QueryWithSKPrefix performs a query with a sort key prefix
func (r *BaseRepository[T]) QueryWithSKPrefix(ctx context.Context, pk, skPrefix string, limit int) ([]T, error) {
	result, err := r.QueryWithSKPrefixPaginated(ctx, pk, skPrefix, BasePaginationOptions{
		Limit: limit,
		Order: SortOrderAsc,
	})
	if err != nil {
		return nil, err
	}

	if result.HasMore && r.logger != nil {
		r.logger.Debug("sk prefix query returned additional pages; consider paginating",
			zap.String("pk", pk),
			zap.String("sk_prefix", skPrefix),
			zap.Int("limit", len(result.Items)),
			zap.String("repository", r.repoName))
	}

	return result.Items, nil
}

// QueryWithSKPrefixPaginated performs a prefix query with cursor support.
func (r *BaseRepository[T]) QueryWithSKPrefixPaginated(ctx context.Context, pk, skPrefix string, opts BasePaginationOptions) (*BasePaginatedResult[T], error) {
	order := opts.Order
	if order == "" {
		order = SortOrderAsc
	}

	safeLimit, clamped, usedDefault := clampLimit(opts.Limit, defaultBaseQueryLimit, maxBaseQueryLimit)
	if clamped && r.logger != nil {
		fields := []zap.Field{
			zap.String("pk", pk),
			zap.String("sk_prefix", skPrefix),
			zap.Int("requested_limit", opts.Limit),
			zap.Int("applied_limit", safeLimit),
		}
		message := "sk prefix query limit clamped"
		if usedDefault {
			message = "sk prefix query called without limit; applying default"
		}
		r.logger.Warn(message, append(fields, zap.String("repository", r.repoName))...)
	}

	var results []T

	query := r.db.WithContext(ctx).Model(modelPrototypeOf[T]()).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", skPrefix).
		OrderBy("SK", order).
		Limit(safeLimit + 1)

	if opts.Cursor != "" {
		if order == SortOrderDesc {
			query = query.Where("SK", "<", opts.Cursor)
		} else {
			query = query.Where("SK", ">", opts.Cursor)
		}
	}

	if err := query.All(&results); err != nil {
		r.logger.Error("failed to query items with SK prefix",
			zap.Error(err),
			zap.String("pk", pk),
			zap.String("skPrefix", skPrefix),
			zap.Int("limit", safeLimit))
		return nil, ErrorHandler.HandleQueryError(err, "base entity", "prefix query")
	}

	hasMore := len(results) > safeLimit
	if hasMore {
		results = results[:safeLimit]
	}

	nextCursor := ""
	if hasMore && len(results) > 0 {
		nextCursor = results[len(results)-1].GetSK()
	}

	return &BasePaginatedResult[T]{
		Items:      results,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// QueryGSI performs a query on a Global Secondary Index
func (r *BaseRepository[T]) QueryGSI(ctx context.Context, indexName, pk string, limit int) ([]T, error) {
	result, err := r.QueryGSIPaginated(ctx, indexName, pk, BasePaginationOptions{
		Limit: limit,
		Order: SortOrderAsc,
	})
	if err != nil {
		return nil, err
	}

	if result.HasMore && r.logger != nil {
		r.logger.Debug("gsi query returned additional pages; consider paginating",
			zap.String("index", indexName),
			zap.String("pk", pk),
			zap.Int("limit", len(result.Items)),
			zap.String("repository", r.repoName))
	}

	return result.Items, nil
}

// QueryGSIPaginated performs a GSI query with cursor and ordering support.
func (r *BaseRepository[T]) QueryGSIPaginated(ctx context.Context, indexName, pk string, opts BasePaginationOptions) (*BasePaginatedResult[T], error) {
	order := opts.Order
	if order == "" {
		order = SortOrderAsc
	}

	safeLimit, clamped, usedDefault := clampLimit(opts.Limit, defaultBaseQueryLimit, maxBaseQueryLimit)
	if clamped && r.logger != nil {
		fields := []zap.Field{
			zap.String("index", indexName),
			zap.String("pk", pk),
			zap.Int("requested_limit", opts.Limit),
			zap.Int("applied_limit", safeLimit),
		}
		message := "gsi query limit clamped"
		if usedDefault {
			message = "gsi query called without limit; applying default"
		}
		r.logger.Warn(message, append(fields, zap.String("repository", r.repoName))...)
	}

	var results []T
	attrPrefix := strings.ToLower(indexName)
	skField := fmt.Sprintf("%sSK", attrPrefix)

	query := r.db.WithContext(ctx).Model(modelPrototypeOf[T]()).
		Index(attrPrefix).
		Where(fmt.Sprintf("%sPK", attrPrefix), "=", pk).
		OrderBy(skField, order).
		Limit(safeLimit + 1)

	if opts.Cursor != "" {
		if order == SortOrderDesc {
			query = query.Where(skField, "<", opts.Cursor)
		} else {
			query = query.Where(skField, ">", opts.Cursor)
		}
	}

	if err := query.All(&results); err != nil {
		r.logger.Error("failed to query GSI",
			zap.Error(err),
			zap.String("index", indexName),
			zap.String("pk", pk),
			zap.Int("limit", safeLimit))
		return nil, ErrorHandler.HandleQueryError(err, "base entity", fmt.Sprintf("GSI query on %s", indexName))
	}

	hasMore := len(results) > safeLimit
	if hasMore {
		results = results[:safeLimit]
	}

	nextCursor := ""
	if hasMore && len(results) > 0 {
		if cursor, ok := extractStringField(results[len(results)-1], skField); ok {
			nextCursor = cursor
		}
	}

	return &BasePaginatedResult[T]{
		Items:      results,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// BatchGet retrieves multiple items by their keys
func (r *BaseRepository[T]) BatchGet(ctx context.Context, keys []struct{ PK, SK string }) ([]T, error) {
	if err := common.ValidateSliceNotEmpty("keys", keys); err != nil {
		return []T{}, nil
	}

	const batchSize = 100
	results := make([]T, 0, len(keys))

	modelPrototype := modelPrototypeOf[T]()
	modelType := reflect.TypeOf(modelPrototype)
	if modelType.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("model prototype must be a pointer type")
	}
	elemType := modelType.Elem()

	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}

		batch := keys[i:end]
		keyModels := make([]any, 0, len(batch))
		for _, key := range batch {
			modelValue := reflect.New(elemType)

			if !setStringField(modelValue, "PK", key.PK) || !setStringField(modelValue, "SK", key.SK) {
				r.logger.Error("failed to assign key fields for batch get item",
					zap.String("pk", key.PK),
					zap.String("sk", key.SK))
				return nil, ErrorHandler.HandleGetError(fmt.Errorf("missing PK/SK field on model"), "batch entity", fmt.Sprintf("%s#%s", key.PK, key.SK))
			}

			keyModels = append(keyModels, modelValue.Interface())
		}

		var batchResults []T
		query := r.db.WithContext(ctx).Model(modelPrototypeOf[T]())
		if err := query.BatchGet(keyModels, &batchResults); err != nil {
			r.logger.Error("failed to batch get items",
				zap.Error(err),
				zap.Int("batch_size", len(batch)))
			return nil, ErrorHandler.HandleGetError(err, "base entity batch", fmt.Sprintf("batch size %d", len(batch)))
		}

		results = append(results, batchResults...)
	}

	return results, nil
}

// Count returns the number of items for a given partition key
func (r *BaseRepository[T]) Count(ctx context.Context, pk string) (int, error) {
	count, err := r.db.WithContext(ctx).Model(modelPrototypeOf[T]()).
		Where("PK", "=", pk).
		Count()

	if err != nil {
		r.logger.Error("failed to count items",
			zap.Error(err),
			zap.String("pk", pk))
		return 0, ErrorHandler.HandleQueryError(err, "base entity", "count query")
	}

	return int(count), nil
}

// Exists checks if an item exists
func (r *BaseRepository[T]) Exists(ctx context.Context, pk, sk string) (bool, error) {
	count, err := r.db.WithContext(ctx).Model(modelPrototypeOf[T]()).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Count()

	if err != nil {
		r.logger.Error("failed to check if item exists",
			zap.Error(err),
			zap.String("pk", pk),
			zap.String("sk", sk))
		return false, ErrorHandler.HandleGetError(err, "base entity", fmt.Sprintf("pk=%s, sk=%s", pk, sk))
	}

	return count > 0, nil
}

// === COST TRACKING UTILITY METHODS ===

// TrackRead provides a simple way to track read operations
func (r *BaseRepository[T]) TrackRead(ctx context.Context, operationType string, readUnits int64) error {
	if r.costService == nil {
		return nil // Silently skip if no cost service
	}

	operation := cost.DynamoOperation{
		Type:               operationType,
		TableName:          r.tableName,
		ConsumedReadUnits:  readUnits,
		ConsumedWriteUnits: 0,
		ItemCount:          1,
		Timestamp:          time.Now(),
		OperationID:        fmt.Sprintf("%s_%s_%d", r.repoName, operationType, time.Now().UnixNano()),
	}

	return r.costService.TrackDynamoOperation(ctx, operation)
}

// TrackWrite provides a simple way to track write operations
func (r *BaseRepository[T]) TrackWrite(ctx context.Context, operationType string, writeUnits int64) error {
	if r.costService == nil {
		return nil // Silently skip if no cost service
	}

	operation := cost.DynamoOperation{
		Type:               operationType,
		TableName:          r.tableName,
		ConsumedReadUnits:  0,
		ConsumedWriteUnits: writeUnits,
		ItemCount:          1,
		Timestamp:          time.Now(),
		OperationID:        fmt.Sprintf("%s_%s_%d", r.repoName, operationType, time.Now().UnixNano()),
	}

	return r.costService.TrackDynamoOperation(ctx, operation)
}

// TrackCustomOperation provides a way to track custom operations with specific parameters
func (r *BaseRepository[T]) TrackCustomOperation(ctx context.Context, operation cost.DynamoOperation) error {
	if r.costService == nil {
		return nil // Silently skip if no cost service
	}

	// Fill in default values if not provided
	if err := common.ValidateRequiredParam("operation.TableName", operation.TableName); err != nil {
		operation.TableName = r.tableName
	}
	if operation.Timestamp.IsZero() {
		operation.Timestamp = time.Now()
	}
	if err := common.ValidateRequiredParam("operation.OperationID", operation.OperationID); err != nil {
		operation.OperationID = fmt.Sprintf("%s_%s_%d", r.repoName, operation.Type, time.Now().UnixNano())
	}

	return r.costService.TrackDynamoOperation(ctx, operation)
}

// GetCostService returns the cost tracking service for direct access if needed
func (r *BaseRepository[T]) GetCostService() *cost.TrackingService {
	return r.costService
}

// SetCostService allows setting or updating the cost service after repository creation
func (r *BaseRepository[T]) SetCostService(costService *cost.TrackingService) {
	r.costService = costService
}

// SetRepoName allows setting the repository name for better cost tracking identification
func (r *BaseRepository[T]) SetRepoName(repoName string) {
	r.repoName = repoName
}

// GetDB returns the underlying database connection for complex queries that can't use BaseRepository methods
func (r *BaseRepository[T]) GetDB() core.DB {
	return r.db
}

// === CONSOLIDATION HELPER FUNCTIONS ===

// These helper functions eliminate code duplication patterns identified across repositories

// CollectionQueryConfig configures behavior for collection query operations
type CollectionQueryConfig struct {
	PKKey       string // What to use as PK value prefix (e.g., "object", "USER", "ACTOR")
	SKKey       string // What to use as SK value prefix (e.g., "likes", "PROFILE", "BLOCKED")
	IndexName   string // GSI index name if using GSI (empty for main table)
	GSIConfig   *GSIQueryConfig
	LogName     string // Name for logging (e.g., "likes", "blocks")
	ErrorPrefix string // Error message prefix (e.g., "get likes", "query blocks")
}

// GSIQueryConfig configures GSI-specific query behavior
type GSIQueryConfig struct {
	PKField   string // GSI PK field name (e.g., "gsi1PK", "gsi2PK")
	SKField   string // GSI SK field name (e.g., "gsi1SK", "gsi2SK")
	PKValue   string // PK value for the GSI
	SKPattern string // SK pattern (for BEGINS_WITH, range queries, etc.)
	UseCursor bool   // Enables cursor-based pagination on the configured sort key
	OrderBy   string // Sort order (SortOrderAsc or SortOrderDesc)
}

// QueryCollectionWithConversion performs paginated collection queries with type conversion
// This eliminates duplication in social relationship queries (likes, blocks, follows, etc.)
func QueryCollectionWithConversion[M BaseModel, R any](
	ctx context.Context,
	r *BaseRepository[M],
	config CollectionQueryConfig,
	entityID string,
	limit int,
	cursor string,
	converter func([]M) ([]R, error),
) ([]R, string, error) {
	safeLimit, clamped, usedDefault := clampLimit(limit, defaultCollectionQueryLimit, maxCollectionQueryLimit)
	if clamped && r.logger != nil {
		fields := []zap.Field{
			zap.String("entity_id", entityID),
			zap.String("collection", config.LogName),
			zap.Int("requested_limit", limit),
			zap.Int("applied_limit", safeLimit),
		}
		message := "collection query limit clamped"
		if usedDefault {
			message = "collection query called without limit; applying default"
		}
		r.logger.Warn(message, append(fields, zap.String("repository", r.repoName))...)
	}

	// Build and execute the query
	var models []M
	var err error

	if config.GSIConfig != nil {
		// GSI query
		gsi := config.GSIConfig
		pkValue := fmt.Sprintf(gsi.PKValue, entityID)

		order := gsi.OrderBy
		if order == "" {
			order = SortOrderAsc
		}

		query := r.db.WithContext(ctx).Model(modelPrototypeOf[M]()).
			Index(config.IndexName).
			Where(gsi.PKField, "=", pkValue).
			OrderBy(gsi.SKField, order).
			Limit(safeLimit + 1)

		if gsi.SKPattern != "" {
			query = query.Where(gsi.SKField, "BEGINS_WITH", gsi.SKPattern)
		}

		if gsi.UseCursor && cursor != "" {
			if order == SortOrderDesc {
				query = query.Where(gsi.SKField, "<", cursor)
			} else {
				query = query.Where(gsi.SKField, ">", cursor)
			}
		}

		err = query.All(&models)
	} else {
		// Main table query
		pkValue := fmt.Sprintf("%s#%s", config.PKKey, entityID)
		skPattern := config.SKKey

		query := r.db.WithContext(ctx).Model(modelPrototypeOf[M]()).
			Where("PK", "=", pkValue).
			Limit(safeLimit + 1)

		if skPattern != "" {
			query = query.Where("SK", "BEGINS_WITH", skPattern)
		}

		if cursor != "" {
			query = query.Where("SK", ">", cursor)
		}

		err = query.All(&models)
	}
	if err != nil {
		r.logger.Error(fmt.Sprintf("failed to %s", config.ErrorPrefix),
			zap.String("entity_id", entityID),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, "collection entity", config.ErrorPrefix)
	}

	hasMore := len(models) > safeLimit
	if hasMore {
		models = models[:safeLimit]
	}

	// Convert to target type
	results, err := converter(models)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "collection conversion", config.LogName)
	}

	// Generate next cursor
	nextCursor := ""
	if hasMore && len(models) > 0 {
		if config.GSIConfig != nil {
			if cursorValue, ok := extractStringField(models[len(models)-1], config.GSIConfig.SKField); ok {
				nextCursor = cursorValue
			}
		} else {
			nextCursor = models[len(models)-1].GetSK()
		}
	}

	return results, nextCursor, nil
}

// DeleteEntityWithLogging performs safe delete operations with consistent error handling and logging
// This eliminates duplication in delete operations across repositories
func DeleteEntityWithLogging[M BaseModel](
	ctx context.Context,
	r *BaseRepository[M],
	pk, sk string,
	entityType string,
	identifiers map[string]string, // key-value pairs for logging (e.g., "actor": actorID, "object": objectID)
) error {
	// Use var instead of new() to avoid double pointer when M is already a pointer type
	// When M = *models.Like, new(M) creates **models.Like which DynamORM rejects
	var model M

	err := r.db.WithContext(ctx).Model(model).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		if errors.IsNotFound(err) {
			logFields := []zap.Field{zap.String("entity_type", entityType)}
			for key, value := range identifiers {
				logFields = append(logFields, zap.String(key, value))
			}
			r.logger.Debug(fmt.Sprintf("%s not found", entityType), logFields...)
			return nil
		}

		logFields := []zap.Field{zap.Error(err), zap.String("entity_type", entityType)}
		for key, value := range identifiers {
			logFields = append(logFields, zap.String(key, value))
		}
		r.logger.Error(fmt.Sprintf("failed to delete %s", entityType), logFields...)
		return ErrorHandler.HandleDeleteError(err, entityType, fmt.Sprintf("pk=%s, sk=%s", pk, sk))
	}

	logFields := []zap.Field{zap.String("entity_type", entityType)}
	for key, value := range identifiers {
		logFields = append(logFields, zap.String(key, value))
	}
	r.logger.Info(fmt.Sprintf("deleted %s", entityType), logFields...)

	return nil
}

// HistoryQueryConfig configures behavior for history/metrics query operations
type HistoryQueryConfig struct {
	MetricType  string                                   // The metric type (e.g., "storage_bytes", "user_count")
	IndexName   string                                   // GSI index name
	PKField     string                                   // GSI PK field name
	SKField     string                                   // GSI SK field name
	LogName     string                                   // Name for logging
	ErrorPrefix string                                   // Error message prefix
	Converter   func(interface{}) map[string]interface{} // Custom field converter
}

// QueryHistoryWithDateRange performs time-range queries for metrics/history data
// This eliminates duplication in GetStorageHistory, GetUserGrowthHistory, etc.
func QueryHistoryWithDateRange[M BaseModel](
	ctx context.Context,
	r *BaseRepository[M],
	config HistoryQueryConfig,
	days int,
) ([]any, error) {
	// Validate and default days parameter
	if err := common.ValidateIntRange("days", days, 1, 365); err != nil {
		days = 30 // Default to 30 days
	}

	// Calculate date range
	startDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	endDate := time.Now().Format("2006-01-02")

	// Query using GSI
	var models []M
	err := r.db.WithContext(ctx).Model(modelPrototypeOf[M]()).
		Index(config.IndexName).
		Where(config.PKField, "=", fmt.Sprintf("METRIC#%s", config.MetricType)).
		Where(config.SKField, ">=", fmt.Sprintf("DATE#%s", startDate)).
		Where(config.SKField, "<=", fmt.Sprintf("DATE#%s", endDate)).
		All(&models)

	if err != nil {
		r.logger.Error(fmt.Sprintf("Failed to get %s", config.LogName),
			zap.Error(err),
			zap.Int("days", days))
		return nil, ErrorHandler.HandleQueryError(err, "history entity", config.LogName)
	}

	// Convert to expected format
	result := make([]any, len(models))
	for i, model := range models {
		if config.Converter != nil {
			result[i] = config.Converter(model)
		} else {
			// Default conversion - this would need to be customized per use case
			result[i] = model
		}
	}

	r.logger.Info(fmt.Sprintf("Retrieved %s", config.LogName),
		zap.Int("days", days),
		zap.Int("records", len(result)))

	return result, nil
}

// MetricsQueryConfig configures behavior for metrics query operations
type MetricsQueryConfig struct {
	IndexName   string // GSI index name
	PKField     string // GSI PK field name
	SKField     string // GSI SK field name
	PKPattern   string // PK value pattern (e.g., "SERVICE#%s", "METRIC_TYPE#%s")
	LogName     string // Name for logging
	ErrorPrefix string // Error message prefix
}

// QueryMetricsByTimeRange performs time-range queries for metric records
// This eliminates duplication in GetMetricsByService, GetMetricsByType, GetMetricsByAggregationLevel
func QueryMetricsByTimeRange[M BaseModel](
	ctx context.Context,
	r *BaseRepository[M],
	config MetricsQueryConfig,
	entityName string,
	startTime, endTime time.Time,
) ([]M, error) {
	var records []M

	pkValue := fmt.Sprintf(config.PKPattern, entityName)
	startSK := fmt.Sprintf("TIMESTAMP#%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("TIMESTAMP#%s", endTime.Format(time.RFC3339))

	err := r.db.WithContext(ctx).Model(modelPrototypeOf[M]()).
		Index(config.IndexName).
		Where(config.PKField, "=", pkValue).
		Where(config.SKField, ">=", startSK).
		Where(config.SKField, "<=", endSK).
		OrderBy(config.SKField, SortOrderDesc).
		All(&records)

	if err != nil {
		r.logger.Error(fmt.Sprintf("failed to get %s", config.LogName),
			zap.Error(err),
			zap.String("entity", entityName),
			zap.Time("startTime", startTime),
			zap.Time("endTime", endTime))
		return nil, ErrorHandler.HandleQueryError(err, "metrics entity", config.LogName)
	}

	return records, nil
}

// ReportConversionConfig configures report model to storage type conversion
type ReportConversionConfig struct {
	CursorField string // Field to use for cursor (e.g., "gsi2SK", "gsi3SK")
	LogContext  string // Context for logging (e.g., "status", "category")
}

// ConvertAndPaginateReports converts report models to storage types with pagination
// This eliminates duplication in GetReportsByStatus, GetReportsByCategory, etc.
func ConvertAndPaginateReports[M interface{}](
	models []M,
	limit int,
	_ ReportConversionConfig,
	converter func(M) *storage.Report,
	cursorExtractor func(M) string,
) ([]*storage.Report, string, error) {
	// Convert models to storage types
	reports := make([]*storage.Report, len(models))
	for i, model := range models {
		reports[i] = converter(model)
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("models", models, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = cursorExtractor(models[limit-1])
		models = models[:limit] // Trim to requested limit

		// Re-process the trimmed models to create reports
		reports = make([]*storage.Report, len(models))
		for i, model := range models {
			reports[i] = converter(model)
		}
	}

	return reports, nextCursor, nil
}

// AuditLogConversionConfig configures audit log model to storage type conversion
type AuditLogConversionConfig struct {
	GSIField   string // GSI field for cursor (e.g., "gsi1SK", "gsi2SK")
	LogContext string // Context for logging
}

// ConvertAndPaginateAuditLogs converts audit log models to storage types with pagination
// This eliminates duplication in GetAuditLogsByAdmin, GetAuditLogsByTarget
func ConvertAndPaginateAuditLogs[M interface{}](
	models []M,
	_ AuditLogConversionConfig,
	converter func(M) *storage.AuditLog,
	cursorExtractor func(M) string,
) ([]*storage.AuditLog, string) {
	// Convert models to storage types
	logs := make([]*storage.AuditLog, 0, len(models))
	for _, model := range models {
		log := converter(model)
		logs = append(logs, log)
	}

	// Get next cursor - use the last item's GSI field if we got results
	nextCursor := ""
	if common.ValidateSliceNotEmpty("models", models) == nil {
		nextCursor = cursorExtractor(models[len(models)-1])
	}

	return logs, nextCursor
}

// === AGGREGATED DATA QUERY HELPER ===

// AggregatedQueryConfig configures behavior for aggregated period queries
type AggregatedQueryConfig struct {
	PKPrefix    string // PK prefix (e.g., "cost_agg", "metrics_agg")
	LogContext  string // Context for logging (e.g., "cost tracking", "metrics")
	ErrorPrefix string // Error message prefix (e.g., "failed to list aggregated cost tracking")
}

// ListAggregatedByPeriod performs time-range queries for aggregated data
// This eliminates duplication between cost tracking and metrics repositories
func ListAggregatedByPeriod[T BaseModel](
	ctx context.Context,
	db core.DB,
	config AggregatedQueryConfig,
	period, entityType string,
	startTime, endTime time.Time,
	limit int,
	cursor string,
) ([]T, string, error) {
	var aggregatedList []T

	safeLimit, clamped, usedDefault := clampLimit(limit, defaultAggregatedQueryLimit, maxAggregatedQueryLimit)
	if clamped {
		msg := "aggregated query limit clamped"
		if usedDefault {
			msg = "aggregated query applied default limit"
		}
		zap.L().Warn(msg,
			zap.String("pk_prefix", config.PKPrefix),
			zap.String("entity_type", entityType),
			zap.Int("requested_limit", limit),
			zap.Int("applied_limit", safeLimit))
	}

	// Build consistent key patterns
	pk := fmt.Sprintf("%s#%s#%s", config.PKPrefix, period, entityType)
	startSK := fmt.Sprintf("window#%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("window#%s", endTime.Format(time.RFC3339))

	// Execute query with time range filtering
	query := db.WithContext(ctx).Model(modelPrototypeOf[T]()).
		Where("PK", "=", pk).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", SortOrderDesc).
		Limit(safeLimit + 1)

	if cursor != "" {
		query = query.Where("SK", "<", cursor)
	}

	err := query.All(&aggregatedList)
	if err != nil {
		return nil, "", MapErrorWithContext(err, config.ErrorPrefix)
	}

	hasMore := len(aggregatedList) > safeLimit
	if hasMore {
		aggregatedList = aggregatedList[:safeLimit]
	}

	nextCursor := ""
	if hasMore && len(aggregatedList) > 0 {
		nextCursor = aggregatedList[len(aggregatedList)-1].GetSK()
	}

	return aggregatedList, nextCursor, nil
}

// === ENHANCED CRUD OPERATIONS FOR TASK 1.2.1 ===

// BasePaginationOptions configures pagination behavior for BaseRepository
type BasePaginationOptions struct {
	Limit  int    // Maximum number of items to return
	Cursor string // Pagination cursor for next page
	Order  string // Sort order: SortOrderAsc or SortOrderDesc
}

// BasePaginatedResult contains paginated query results from BaseRepository
type BasePaginatedResult[T BaseModel] struct {
	Items      []T    // The retrieved items
	NextCursor string // Cursor for next page (empty if no more pages)
	HasMore    bool   // Whether there are more pages available
}

// FindByPK retrieves all items with a specific partition key
func (r *BaseRepository[T]) FindByPK(ctx context.Context, pk string) ([]T, error) {
	var results []T

	err := r.db.WithContext(ctx).Model(modelPrototypeOf[T]()).
		Where("PK", "=", pk).
		All(&results)

	// Track cost if cost service is available
	if r.costService != nil {
		itemCount := int64(len(results))
		estimatedRU := itemCount
		if estimatedRU == 0 {
			estimatedRU = 1 // Minimum for the query operation itself
		}

		operation := cost.DynamoOperation{
			Type:               "Query",
			TableName:          r.tableName,
			ConsumedReadUnits:  estimatedRU,
			ConsumedWriteUnits: 0,
			ItemCount:          itemCount,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("%s_findByPK_%d", r.repoName, time.Now().UnixNano()),
		}

		if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
			r.logger.Warn("failed to track DynamoDB findByPK operation cost",
				zap.String("repository", r.repoName),
				zap.String("pk", pk),
				zap.Error(trackErr))
		}
	}

	if err != nil {
		r.logger.Error("failed to find items by PK",
			zap.Error(err),
			zap.String("pk", pk))
		return nil, ErrorHandler.HandleQueryError(err, "base entity", "find by PK")
	}

	return results, nil
}

// FindBySK retrieves all items with a specific sort key (across all partitions)
// Note: This requires a GSI with SK as the partition key
func (r *BaseRepository[T]) FindBySK(ctx context.Context, sk string, gsiName string) ([]T, error) {
	var results []T

	attrPrefix := strings.ToLower(gsiName)
	err := r.db.WithContext(ctx).Model(modelPrototypeOf[T]()).
		Index(gsiName).
		Where(fmt.Sprintf("%sPK", attrPrefix), "=", sk).
		All(&results)

	// Track cost if cost service is available
	if r.costService != nil {
		itemCount := int64(len(results))
		estimatedRU := itemCount
		if estimatedRU == 0 {
			estimatedRU = 1
		}

		operation := cost.DynamoOperation{
			Type:               "Query",
			TableName:          r.tableName,
			ConsumedReadUnits:  estimatedRU,
			ConsumedWriteUnits: 0,
			ItemCount:          itemCount,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("%s_findBySK_%d", r.repoName, time.Now().UnixNano()),
		}

		if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
			r.logger.Warn("failed to track DynamoDB findBySK operation cost",
				zap.String("repository", r.repoName),
				zap.String("sk", sk),
				zap.Error(trackErr))
		}
	}

	if err != nil {
		r.logger.Error("failed to find items by SK",
			zap.Error(err),
			zap.String("sk", sk),
			zap.String("gsi", gsiName))
		return nil, ErrorHandler.HandleQueryError(err, "base entity", fmt.Sprintf("find by SK on %s", gsiName))
	}

	return results, nil
}

// FindWithPagination performs paginated queries with cursor support
func (r *BaseRepository[T]) FindWithPagination(ctx context.Context, pk string, opts BasePaginationOptions) (*BasePaginatedResult[T], error) {
	// Validate and set defaults
	if opts.Limit <= 0 {
		opts.Limit = 20 // Default limit
	}
	if opts.Limit > 100 {
		opts.Limit = 100 // Maximum limit to prevent abuse
	}
	if opts.Order == "" {
		opts.Order = SortOrderAsc
	}

	var results []T

	// Build query
	query := r.db.WithContext(ctx).Model(modelPrototypeOf[T]()).
		Where("PK", "=", pk).
		Limit(opts.Limit+1). // Get one extra to check if there are more
		OrderBy("SK", opts.Order)

	// Apply cursor if provided
	if opts.Cursor != "" {
		if opts.Order == SortOrderDesc {
			query = query.Where("SK", "<", opts.Cursor)
		} else {
			query = query.Where("SK", ">", opts.Cursor)
		}
	}

	// Execute query
	err := query.All(&results)
	if err != nil {
		r.logger.Error("failed to execute paginated query",
			zap.Error(err),
			zap.String("pk", pk),
			zap.Int("limit", opts.Limit))
		return nil, ErrorHandler.HandleQueryError(err, "base entity", "paginated query")
	}

	// Determine if there are more pages
	hasMore := len(results) > opts.Limit
	if hasMore {
		results = results[:opts.Limit] // Trim to requested limit
	}

	// Generate next cursor
	nextCursor := ""
	if hasMore && len(results) > 0 {
		nextCursor = results[len(results)-1].GetSK()
	}

	// Track cost
	if r.costService != nil {
		itemCount := int64(len(results))
		estimatedRU := itemCount
		if estimatedRU == 0 {
			estimatedRU = 1
		}

		operation := cost.DynamoOperation{
			Type:               "Query",
			TableName:          r.tableName,
			ConsumedReadUnits:  estimatedRU,
			ConsumedWriteUnits: 0,
			ItemCount:          itemCount,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("%s_paginated_%d", r.repoName, time.Now().UnixNano()),
		}

		if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
			r.logger.Warn("failed to track paginated query cost",
				zap.String("repository", r.repoName),
				zap.String("pk", pk),
				zap.Error(trackErr))
		}
	}

	return &BasePaginatedResult[T]{
		Items:      results,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// BatchCreate creates multiple items efficiently using DynamoDB batch operations
func (r *BaseRepository[T]) BatchCreate(ctx context.Context, items []T) error {
	if err := common.ValidateSliceNotEmpty("items", items); err != nil {
		return nil // Silently return if no items to create
	}

	// Update keys for all items
	for _, item := range items {
		if err := item.UpdateKeys(); err != nil {
			return ErrorHandler.HandleCreateError(err, "batch entity keys", item.GetPK())
		}
	}

	// DynamoDB batch write has a limit of 25 items
	batchSize := 25
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}

		batch := items[i:end]

		// Create batch write request
		// Note: This is a simplified implementation - would need proper batch write implementation
		for _, item := range batch {
			err := r.db.WithContext(ctx).Model(item).Create()
			if err != nil {
				r.logger.Error("failed to create item in batch",
					zap.Error(err),
					zap.String("pk", item.GetPK()),
					zap.String("sk", item.GetSK()))
				return ErrorHandler.HandleCreateError(err, "batch entity", item.GetPK())
			}
		}

		// Track cost for batch
		if r.costService != nil {
			operation := cost.DynamoOperation{
				Type:               "BatchWriteItem",
				TableName:          r.tableName,
				ConsumedReadUnits:  0,
				ConsumedWriteUnits: int64(len(batch)), // 1 WU per item
				ItemCount:          int64(len(batch)),
				Timestamp:          time.Now(),
				OperationID:        fmt.Sprintf("%s_batchCreate_%d", r.repoName, time.Now().UnixNano()),
			}

			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn("failed to track batch create cost",
					zap.String("repository", r.repoName),
					zap.Int("batchSize", len(batch)),
					zap.Error(trackErr))
			}
		}
	}

	return nil
}

// BatchDelete removes multiple items efficiently using DynamoDB batch operations
func (r *BaseRepository[T]) BatchDelete(ctx context.Context, keys []struct{ PK, SK string }) error {
	if err := common.ValidateSliceNotEmpty("keys", keys); err != nil {
		return nil // Silently return if no keys to delete
	}

	// DynamoDB batch write has a limit of 25 items
	batchSize := 25
	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}

		batch := keys[i:end]

		// Create batch delete request
		// Note: This is a simplified implementation - would need proper batch delete implementation
		for _, key := range batch {
			err := r.Delete(ctx, key.PK, key.SK)
			if err != nil {
				r.logger.Error("failed to delete item in batch",
					zap.Error(err),
					zap.String("pk", key.PK),
					zap.String("sk", key.SK))
				return ErrorHandler.HandleDeleteError(err, "batch entity", fmt.Sprintf("pk=%s, sk=%s", key.PK, key.SK))
			}
		}

		// Track cost for batch
		if r.costService != nil {
			operation := cost.DynamoOperation{
				Type:               "BatchWriteItem",
				TableName:          r.tableName,
				ConsumedReadUnits:  0,
				ConsumedWriteUnits: int64(len(batch)), // 1 WU per item
				ItemCount:          int64(len(batch)),
				Timestamp:          time.Now(),
				OperationID:        fmt.Sprintf("%s_batchDelete_%d", r.repoName, time.Now().UnixNano()),
			}

			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn("failed to track batch delete cost",
					zap.String("repository", r.repoName),
					zap.Int("batchSize", len(batch)),
					zap.Error(trackErr))
			}
		}
	}

	return nil
}

// QueryWithFilter performs queries with additional filter conditions
func (r *BaseRepository[T]) QueryWithFilter(ctx context.Context, pk string, filters map[string]interface{}, limit int) ([]T, error) {
	var results []T

	safeLimit, clamped, usedDefault := clampLimit(limit, defaultFilterQueryLimit, maxFilterQueryLimit)
	if clamped && r.logger != nil {
		fields := []zap.Field{
			zap.String("pk", pk),
			zap.Int("requested_limit", limit),
			zap.Int("applied_limit", safeLimit),
		}
		message := "filtered query limit clamped"
		if usedDefault {
			message = "filtered query used default limit"
		}
		r.logger.Warn(message, append(fields, zap.String("repository", r.repoName))...)
	}

	// Build query
	query := r.db.WithContext(ctx).Model(modelPrototypeOf[T]()).
		Where("PK", "=", pk).
		Limit(safeLimit + 1)

	// Apply filters
	for field, value := range filters {
		switch v := value.(type) {
		case string:
			query = query.Filter(field, "=", v)
		case map[string]interface{}:
			// Handle complex filter conditions
			if op, ok := v["op"].(string); ok {
				if val, ok := v["value"]; ok {
					query = query.Filter(field, op, val)
				}
			}
		}
	}

	// Execute query
	err := query.All(&results)
	if err != nil {
		r.logger.Error("failed to query with filters",
			zap.Error(err),
			zap.String("pk", pk),
			zap.Any("filters", filters))
		return nil, ErrorHandler.HandleQueryError(err, "base entity", "filtered query")
	}

	hasMore := len(results) > safeLimit
	if hasMore {
		results = results[:safeLimit]
		if r.logger != nil {
			r.logger.Debug("filtered query returned additional pages; consider pagination",
				zap.String("pk", pk),
				zap.Int("limit", safeLimit),
				zap.String("repository", r.repoName))
		}
	}

	// Track cost
	if r.costService != nil {
		itemCount := int64(len(results))
		estimatedRU := itemCount
		if estimatedRU == 0 {
			estimatedRU = 1
		}

		operation := cost.DynamoOperation{
			Type:               "Query",
			TableName:          r.tableName,
			ConsumedReadUnits:  estimatedRU,
			ConsumedWriteUnits: 0,
			ItemCount:          itemCount,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("%s_queryFilter_%d", r.repoName, time.Now().UnixNano()),
		}

		if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
			r.logger.Warn("failed to track filtered query cost",
				zap.String("repository", r.repoName),
				zap.String("pk", pk),
				zap.Error(trackErr))
		}
	}

	return results, nil
}

// QueryBetween performs range queries between two sort key values
func (r *BaseRepository[T]) QueryBetween(ctx context.Context, pk, startSK, endSK string, limit int) ([]T, error) {
	result, err := r.QueryBetweenPaginated(ctx, pk, startSK, endSK, BasePaginationOptions{
		Limit: limit,
		Order: SortOrderAsc,
	})
	if err != nil {
		return nil, err
	}

	if result.HasMore && r.logger != nil {
		r.logger.Debug("range query returned additional pages; consider pagination",
			zap.String("pk", pk),
			zap.String("start_sk", startSK),
			zap.String("end_sk", endSK),
			zap.Int("limit", len(result.Items)),
			zap.String("repository", r.repoName))
	}

	return result.Items, nil
}

// QueryBetweenPaginated performs range queries with cursor support.
func (r *BaseRepository[T]) QueryBetweenPaginated(ctx context.Context, pk, startSK, endSK string, opts BasePaginationOptions) (*BasePaginatedResult[T], error) {
	order := opts.Order
	if order == "" {
		order = SortOrderAsc
	}

	safeLimit, clamped, usedDefault := clampLimit(opts.Limit, defaultBaseQueryLimit, maxBaseQueryLimit)
	if clamped && r.logger != nil {
		fields := []zap.Field{
			zap.String("pk", pk),
			zap.String("start_sk", startSK),
			zap.String("end_sk", endSK),
			zap.Int("requested_limit", opts.Limit),
			zap.Int("applied_limit", safeLimit),
		}
		message := "range query limit clamped"
		if usedDefault {
			message = "range query applied default limit"
		}
		r.logger.Warn(message, append(fields, zap.String("repository", r.repoName))...)
	}

	var results []T

	query := r.db.WithContext(ctx).Model(modelPrototypeOf[T]()).
		Where("PK", "=", pk).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", order).
		Limit(safeLimit + 1)

	if opts.Cursor != "" {
		if order == SortOrderDesc {
			query = query.Where("SK", "<", opts.Cursor)
		} else {
			query = query.Where("SK", ">", opts.Cursor)
		}
	}

	if err := query.All(&results); err != nil {
		r.logger.Error("failed to query between range",
			zap.Error(err),
			zap.String("pk", pk),
			zap.String("startSK", startSK),
			zap.String("endSK", endSK))
		return nil, ErrorHandler.HandleQueryError(err, "base entity", "range query")
	}

	hasMore := len(results) > safeLimit
	if hasMore {
		results = results[:safeLimit]
	}

	nextCursor := ""
	if hasMore && len(results) > 0 {
		nextCursor = results[len(results)-1].GetSK()
	}

	// Track cost
	if r.costService != nil {
		itemCount := int64(len(results))
		if itemCount == 0 {
			itemCount = 1
		}

		operation := cost.DynamoOperation{
			Type:               "Query",
			TableName:          r.tableName,
			ConsumedReadUnits:  itemCount,
			ConsumedWriteUnits: 0,
			ItemCount:          itemCount,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("%s_queryBetween_%d", r.repoName, time.Now().UnixNano()),
		}

		if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
			r.logger.Warn("failed to track range query cost",
				zap.String("repository", r.repoName),
				zap.String("pk", pk),
				zap.Error(trackErr))
		}
	}

	return &BasePaginatedResult[T]{
		Items:      results,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// setStringField assigns a string value to a named field using reflection.
func setStringField(val reflect.Value, fieldName, fieldValue string) bool {
	if val.Kind() != reflect.Ptr {
		return false
	}

	elem := val.Elem()
	if !elem.IsValid() {
		return false
	}

	field := elem.FieldByName(fieldName)
	if !field.IsValid() || field.Kind() != reflect.String || !field.CanSet() {
		return false
	}

	field.SetString(fieldValue)
	return true
}

// extractStringField retrieves a string field by name from the provided model.
func extractStringField(model any, fieldName string) (string, bool) {
	value := reflect.ValueOf(model)
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	if !value.IsValid() {
		return "", false
	}

	field := value.FieldByName(fieldName)
	if !field.IsValid() || field.Kind() != reflect.String {
		return "", false
	}

	return field.String(), true
}
