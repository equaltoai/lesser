package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/common"
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
	// Update keys before saving
	if err := item.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
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
		return fmt.Errorf("failed to create item: %w", err)
	}

	return nil
}

// Get retrieves a single item by primary and sort key
func (r *BaseRepository[T]) Get(ctx context.Context, pk, sk string, result T) error {
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
			return fmt.Errorf("item not found: pk=%s, sk=%s", pk, sk)
		}
		r.logger.Error("failed to get item",
			zap.Error(err),
			zap.String("pk", pk),
			zap.String("sk", sk))
		return fmt.Errorf("failed to get item: %w", err)
	}

	return nil
}

// Update updates an existing item
// Note: In DynamORM, you need to update the model fields before calling Update()
// This method is provided for consistency but may need adaptation per repository
func (r *BaseRepository[T]) Update(ctx context.Context, item T) error {
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
		return fmt.Errorf("failed to update item: %w", err)
	}

	return nil
}

// Delete removes an item from the database
func (r *BaseRepository[T]) Delete(ctx context.Context, pk, sk string) error {
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

	// Create a zero value of T to get the model type
	var model T

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
		return fmt.Errorf("failed to delete item: %w", err)
	}

	return nil
}

// Query performs a query operation on a partition key
func (r *BaseRepository[T]) Query(ctx context.Context, pk string, limit int) ([]T, error) {
	var results []T

	// Create query
	query := r.db.WithContext(ctx).Model(new(T)).
		Where("PK", "=", pk)

	if limit > 0 {
		query = query.Limit(limit)
	}

	// Execute query
	err := query.All(&results)
	
	// Track cost if cost service is available
	if r.costService != nil {
		itemCount := int64(len(results))
		estimatedRU := itemCount // Estimate 1 RU per item
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
			OperationID:        fmt.Sprintf("%s_query_%d", r.repoName, time.Now().UnixNano()),
		}
		
		if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
			r.logger.Warn("failed to track DynamoDB query operation cost",
				zap.String("repository", r.repoName),
				zap.String("pk", pk),
				zap.Error(trackErr))
		}
	}

	if err != nil {
		r.logger.Error("failed to query items",
			zap.Error(err),
			zap.String("pk", pk),
			zap.Int("limit", limit))
		return nil, fmt.Errorf("failed to query items: %w", err)
	}

	return results, nil
}

// QueryWithSKPrefix performs a query with a sort key prefix
func (r *BaseRepository[T]) QueryWithSKPrefix(ctx context.Context, pk, skPrefix string, limit int) ([]T, error) {
	var results []T

	// Create query
	query := r.db.WithContext(ctx).Model(new(T)).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", skPrefix)

	if limit > 0 {
		query = query.Limit(limit)
	}

	// Execute query
	err := query.All(&results)
	if err != nil {
		r.logger.Error("failed to query items with SK prefix",
			zap.Error(err),
			zap.String("pk", pk),
			zap.String("skPrefix", skPrefix),
			zap.Int("limit", limit))
		return nil, fmt.Errorf("failed to query items: %w", err)
	}

	return results, nil
}

// QueryGSI performs a query on a Global Secondary Index
func (r *BaseRepository[T]) QueryGSI(ctx context.Context, indexName, pk string, limit int) ([]T, error) {
	var results []T

	// Create query
	query := r.db.WithContext(ctx).Model(new(T)).
		Index(indexName).
		Where(fmt.Sprintf("%sPK", indexName), "=", pk)

	if limit > 0 {
		query = query.Limit(limit)
	}

	// Execute query
	err := query.All(&results)
	if err != nil {
		r.logger.Error("failed to query GSI",
			zap.Error(err),
			zap.String("index", indexName),
			zap.String("pk", pk),
			zap.Int("limit", limit))
		return nil, fmt.Errorf("failed to query GSI: %w", err)
	}

	return results, nil
}

// BatchGet retrieves multiple items by their keys
func (r *BaseRepository[T]) BatchGet(ctx context.Context, keys []struct{ PK, SK string }) ([]T, error) {
	if err := common.ValidateSliceNotEmpty("keys", keys); err != nil {
		return []T{}, nil
	}

	var results []T

	// DynamoDB batch get has a limit of 100 items
	batchSize := 100
	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}

		batch := keys[i:end]
		var batchResults []T

		// Create batch get request
		batchGet := r.db.WithContext(ctx).Model(new(T))
		for _, key := range batch {
			batchGet = batchGet.Where("PK", "=", key.PK).Where("SK", "=", key.SK)
		}

		// Execute batch get
		err := batchGet.All(&batchResults)
		if err != nil {
			r.logger.Error("failed to batch get items",
				zap.Error(err),
				zap.Int("batchSize", len(batch)))
			return nil, fmt.Errorf("failed to batch get items: %w", err)
		}

		results = append(results, batchResults...)
	}

	return results, nil
}

// Count returns the number of items for a given partition key
func (r *BaseRepository[T]) Count(ctx context.Context, pk string) (int, error) {
	count, err := r.db.WithContext(ctx).Model(new(T)).
		Where("PK", "=", pk).
		Count()

	if err != nil {
		r.logger.Error("failed to count items",
			zap.Error(err),
			zap.String("pk", pk))
		return 0, fmt.Errorf("failed to count items: %w", err)
	}

	return int(count), nil
}

// Exists checks if an item exists
func (r *BaseRepository[T]) Exists(ctx context.Context, pk, sk string) (bool, error) {
	count, err := r.db.WithContext(ctx).Model(new(T)).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Count()

	if err != nil {
		r.logger.Error("failed to check if item exists",
			zap.Error(err),
			zap.String("pk", pk),
			zap.String("sk", sk))
		return false, fmt.Errorf("failed to check existence: %w", err)
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
