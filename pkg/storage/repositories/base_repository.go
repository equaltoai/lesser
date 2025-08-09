package repositories

import (
	"context"
	"fmt"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// BaseModel interface that all DynamoDB models must implement
type BaseModel interface {
	UpdateKeys() error
	GetPK() string
	GetSK() string
}

// BaseRepository provides common CRUD operations for all repositories
type BaseRepository[T BaseModel] struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewBaseRepository creates a new base repository
func NewBaseRepository[T BaseModel](db core.DB, tableName string, logger *zap.Logger) *BaseRepository[T] {
	return &BaseRepository[T]{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// Create stores a new item in the database
func (r *BaseRepository[T]) Create(ctx context.Context, item T) error {
	// Update keys before saving
	if err := item.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
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
	if len(keys) == 0 {
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
