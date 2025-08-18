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
	"github.com/equaltoai/lesser/pkg/storage"
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

// === CONSOLIDATION HELPER FUNCTIONS ===

// These helper functions eliminate code duplication patterns identified across repositories

// CollectionQueryConfig configures behavior for collection query operations
type CollectionQueryConfig struct {
	PKKey        string // What to use as PK value prefix (e.g., "object", "USER", "ACTOR")
	SKKey        string // What to use as SK value prefix (e.g., "likes", "PROFILE", "BLOCKED")
	IndexName    string // GSI index name if using GSI (empty for main table)
	GSIConfig    *GSIQueryConfig
	LogName      string // Name for logging (e.g., "likes", "blocks")
	ErrorPrefix  string // Error message prefix (e.g., "get likes", "query blocks")
}

// GSIQueryConfig configures GSI-specific query behavior
type GSIQueryConfig struct {
	PKField    string // GSI PK field name (e.g., "GSI1PK", "GSI2PK")
	SKField    string // GSI SK field name (e.g., "GSI1SK", "GSI2SK")
	PKValue    string // PK value for the GSI
	SKPattern  string // SK pattern (for BEGINS_WITH, range queries, etc.)
	UseCursor  bool   // Whether to support cursor-based pagination
	OrderBy    string // Sort order ("ASC" or "DESC")
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
	// Build and execute the query
	var models []M
	var err error

	if config.GSIConfig != nil {
		// GSI query
		gsi := config.GSIConfig
		pkValue := fmt.Sprintf(gsi.PKValue, entityID)
		
		query := r.db.WithContext(ctx).Model(new(M)).
			Index(config.IndexName).
			Where(gsi.PKField, "=", pkValue).
			Limit(limit)
			
		if gsi.SKPattern != "" {
			query = query.Filter(gsi.SKField, "BEGINS_WITH", gsi.SKPattern)
		}
		
		if gsi.UseCursor && cursor != "" {
			if gsi.OrderBy == "DESC" {
				query = query.Where(gsi.SKField, "<", cursor)
			} else {
				query = query.Where(gsi.SKField, ">", cursor)
			}
		}
		
		if gsi.OrderBy != "" {
			query = query.OrderBy(gsi.SKField, gsi.OrderBy)
		}
		
		err = query.All(&models)
	} else {
		// Main table query
		pkValue := fmt.Sprintf("%s#%s", config.PKKey, entityID)
		skPattern := config.SKKey
		
		query := r.db.WithContext(ctx).Model(new(M)).
			Where("PK", "=", pkValue).
			Limit(limit)
			
		if skPattern != "" {
			query = query.Filter("SK", "BEGINS_WITH", skPattern)
		}
		
		err = query.All(&models)
	}
	if err != nil {
		r.logger.Error(fmt.Sprintf("failed to %s", config.ErrorPrefix),
			zap.String("entity_id", entityID),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to %s: %w", config.ErrorPrefix, err)
	}

	// Convert to target type
	results, err := converter(models)
	if err != nil {
		return nil, "", fmt.Errorf("failed to convert %s: %w", config.LogName, err)
	}

	// Generate next cursor
	nextCursor := ""
	if len(models) == limit && len(models) > 0 {
		if config.GSIConfig != nil {
			nextCursor = getGSISK(models[len(models)-1], config.GSIConfig.SKField)
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
	model := new(M)
	
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
		return fmt.Errorf("failed to delete %s: %w", entityType, err)
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
	MetricType   string // The metric type (e.g., "storage_bytes", "user_count")
	IndexName    string // GSI index name
	PKField      string // GSI PK field name
	SKField      string // GSI SK field name
	LogName      string // Name for logging
	ErrorPrefix  string // Error message prefix
	Converter    func(interface{}) map[string]interface{} // Custom field converter
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
	err := r.db.WithContext(ctx).Model(new(M)).
		Index(config.IndexName).
		Where(config.PKField, "=", fmt.Sprintf("METRIC#%s", config.MetricType)).
		Where(config.SKField, ">=", fmt.Sprintf("DATE#%s", startDate)).
		Where(config.SKField, "<=", fmt.Sprintf("DATE#%s", endDate)).
		All(&models)

	if err != nil {
		r.logger.Error(fmt.Sprintf("Failed to get %s", config.LogName), 
			zap.Error(err), 
			zap.Int("days", days))
		return nil, fmt.Errorf("failed to get %s: %w", config.LogName, err)
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

	err := r.db.WithContext(ctx).Model(new(M)).
		Index(config.IndexName).
		Where(config.PKField, "=", pkValue).
		Where(config.SKField, ">=", startSK).
		Where(config.SKField, "<=", endSK).
		OrderBy(config.SKField, "DESC").
		All(&records)

	if err != nil {
		r.logger.Error(fmt.Sprintf("failed to get %s", config.LogName),
			zap.Error(err),
			zap.String("entity", entityName),
			zap.Time("startTime", startTime),
			zap.Time("endTime", endTime))
		return nil, MapErrorWithContext(err, fmt.Sprintf("failed to get %s", config.LogName))
	}

	return records, nil
}

// ReportConversionConfig configures report model to storage type conversion
type ReportConversionConfig struct {
	CursorField string // Field to use for cursor (e.g., "GSI2SK", "GSI3SK")
	LogContext  string // Context for logging (e.g., "status", "category")
}

// ConvertAndPaginateReports converts report models to storage types with pagination
// This eliminates duplication in GetReportsByStatus, GetReportsByCategory, etc.
func ConvertAndPaginateReports[M interface{}](
	models []M,
	limit int,
	config ReportConversionConfig,
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
	GSIField   string // GSI field for cursor (e.g., "GSI1SK", "GSI2SK")
	LogContext string // Context for logging
}

// ConvertAndPaginateAuditLogs converts audit log models to storage types with pagination
// This eliminates duplication in GetAuditLogsByAdmin, GetAuditLogsByTarget
func ConvertAndPaginateAuditLogs[M interface{}](
	models []M,
	config AuditLogConversionConfig,
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

// getGSISK extracts GSI SK value from a model using reflection or interface
// This is a helper function to get cursor values from different GSI fields
func getGSISK(model BaseModel, fieldName string) string {
	// This would need to be implemented based on the actual model structure
	// For now, return empty string - this should be customized per repository
	return ""
}
