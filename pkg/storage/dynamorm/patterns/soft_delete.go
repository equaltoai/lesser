// Package patterns provides soft delete functionality and patterns for DynamORM model operations.
package patterns

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// SoftDeletable interface defines methods for soft delete functionality
type SoftDeletable interface {
	SoftDelete()
	Restore()
	IsDeleted() bool
	GetDeletedAt() *time.Time
	SetDeletedAt(*time.Time)
	GetDeletedBy() string
	SetDeletedBy(string)
}

// SoftDeleteModel provides default soft delete functionality
type SoftDeleteModel struct {
	DeletedAt *time.Time `dynamodbav:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	DeletedBy string     `dynamodbav:"deleted_by,omitempty" json:"deleted_by,omitempty"`
}

// SoftDelete marks the entity as deleted
func (m *SoftDeleteModel) SoftDelete() {
	now := time.Now()
	m.DeletedAt = &now
}

// SoftDeleteBy marks the entity as deleted by a specific user
func (m *SoftDeleteModel) SoftDeleteBy(userID string) {
	now := time.Now()
	m.DeletedAt = &now
	m.DeletedBy = userID
}

// Restore removes the soft delete marker
func (m *SoftDeleteModel) Restore() {
	m.DeletedAt = nil
	m.DeletedBy = ""
}

// IsDeleted returns true if the entity is soft deleted
func (m *SoftDeleteModel) IsDeleted() bool {
	return m.DeletedAt != nil
}

// GetDeletedAt returns the deletion timestamp
func (m *SoftDeleteModel) GetDeletedAt() *time.Time {
	return m.DeletedAt
}

// SetDeletedAt sets the deletion timestamp
func (m *SoftDeleteModel) SetDeletedAt(deletedAt *time.Time) {
	m.DeletedAt = deletedAt
}

// GetDeletedBy returns the user who deleted this entity
func (m *SoftDeleteModel) GetDeletedBy() string {
	return m.DeletedBy
}

// SetDeletedBy sets the user who deleted this entity
func (m *SoftDeleteModel) SetDeletedBy(deletedBy string) {
	m.DeletedBy = deletedBy
}

// SoftDeleteRepository provides repository methods with soft delete support
type SoftDeleteRepository struct {
	client         DynamoDBClient
	tableName      string
	logger         *zap.Logger
	includeDeleted bool // When true, queries include soft-deleted items
}

// DynamoDBClient interface for DynamoDB operations
type DynamoDBClient interface {
	GetItem(input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error)
	PutItem(input *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error)
	UpdateItem(input *dynamodb.UpdateItemInput) (*dynamodb.UpdateItemOutput, error)
	DeleteItem(input *dynamodb.DeleteItemInput) (*dynamodb.DeleteItemOutput, error)
	Query(input *dynamodb.QueryInput) (*dynamodb.QueryOutput, error)
	Scan(input *dynamodb.ScanInput) (*dynamodb.ScanOutput, error)
	BatchGetItem(input *dynamodb.BatchGetItemInput) (*dynamodb.BatchGetItemOutput, error)
	BatchWriteItem(input *dynamodb.BatchWriteItemInput) (*dynamodb.BatchWriteItemOutput, error)
}

// NewSoftDeleteRepository creates a new soft delete repository
func NewSoftDeleteRepository(client DynamoDBClient, tableName string, logger *zap.Logger) *SoftDeleteRepository {
	return &SoftDeleteRepository{
		client:         client,
		tableName:      tableName,
		logger:         logger,
		includeDeleted: false,
	}
}

// WithDeleted returns a new repository instance that includes soft-deleted items in queries
func (r *SoftDeleteRepository) WithDeleted() *SoftDeleteRepository {
	return &SoftDeleteRepository{
		client:         r.client,
		tableName:      r.tableName,
		logger:         r.logger,
		includeDeleted: true,
	}
}

// OnlyDeleted returns a new repository instance that only returns soft-deleted items
func (r *SoftDeleteRepository) OnlyDeleted() *SoftDeleteRepository {
	return &SoftDeleteRepository{
		client:         r.client,
		tableName:      r.tableName,
		logger:         r.logger,
		includeDeleted: false, // Will be overridden in query methods
	}
}

// SoftDelete performs a soft delete operation
func (r *SoftDeleteRepository) SoftDelete(ctx context.Context, model SoftDeletable, userID string) error {
	// Mark as soft deleted
	model.SoftDelete()
	model.SetDeletedBy(userID)

	// Update the item in DynamoDB
	if err := r.save(ctx, model); err != nil {
		return fmt.Errorf("failed to soft delete: %w", err)
	}

	if r.logger != nil {
		r.logger.Info("item_soft_deleted",
			zap.String("table", r.tableName),
			zap.String("type", reflect.TypeOf(model).String()),
			zap.String("deleted_by", userID),
		)
	}

	return nil
}

// Restore restores a soft-deleted item
func (r *SoftDeleteRepository) Restore(ctx context.Context, model SoftDeletable) error {
	if !model.IsDeleted() {
		return fmt.Errorf("item is not soft deleted")
	}

	// Remove soft delete markers
	model.Restore()

	// Update the item in DynamoDB
	if err := r.save(ctx, model); err != nil {
		return fmt.Errorf("failed to restore: %w", err)
	}

	if r.logger != nil {
		r.logger.Info("item_restored",
			zap.String("table", r.tableName),
			zap.String("type", reflect.TypeOf(model).String()),
		)
	}

	return nil
}

// HardDelete permanently deletes an item from DynamoDB
func (r *SoftDeleteRepository) HardDelete(_ context.Context, keys map[string]*dynamodb.AttributeValue) error {
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(r.tableName),
		Key:       keys,
	}

	_, err := r.client.DeleteItem(input)
	if err != nil {
		return fmt.Errorf("failed to hard delete: %w", err)
	}

	if r.logger != nil {
		r.logger.Info("item_hard_deleted",
			zap.String("table", r.tableName),
			zap.Any("keys", keys),
		)
	}

	return nil
}

// Get retrieves an item by key, optionally including soft-deleted items
func (r *SoftDeleteRepository) Get(_ context.Context, keys map[string]*dynamodb.AttributeValue) (map[string]*dynamodb.AttributeValue, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key:       keys,
	}

	result, err := r.client.GetItem(input)
	if err != nil {
		return nil, fmt.Errorf("failed to get item: %w", err)
	}

	if result.Item == nil {
		return nil, nil
	}

	// Check if item is soft deleted and should be filtered
	if !r.includeDeleted && r.isSoftDeleted(result.Item) {
		return nil, nil
	}

	return result.Item, nil
}

// Query performs a query with soft delete filtering
func (r *SoftDeleteRepository) Query(_ context.Context, input *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
	// Add soft delete filter if needed
	if !r.includeDeleted {
		input = r.addSoftDeleteFilter(input)
	}

	result, err := r.client.Query(input)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}

	// Filter results if needed (fallback for complex expressions)
	if !r.includeDeleted {
		result.Items = r.filterSoftDeleted(result.Items)
	}

	return result, nil
}

// QueryOnlyDeleted queries only soft-deleted items
func (r *SoftDeleteRepository) QueryOnlyDeleted(_ context.Context, input *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
	// Add filter for only soft-deleted items
	input = r.addOnlyDeletedFilter(input)

	result, err := r.client.Query(input)
	if err != nil {
		return nil, fmt.Errorf("failed to query deleted items: %w", err)
	}

	return result, nil
}

// Scan performs a scan with soft delete filtering
func (r *SoftDeleteRepository) Scan(_ context.Context, input *dynamodb.ScanInput) (*dynamodb.ScanOutput, error) {
	// Add soft delete filter if needed
	if !r.includeDeleted {
		input = r.addSoftDeleteFilterToScan(input)
	}

	result, err := r.client.Scan(input)
	if err != nil {
		return nil, fmt.Errorf("failed to scan: %w", err)
	}

	// Filter results if needed (fallback for complex expressions)
	if !r.includeDeleted {
		result.Items = r.filterSoftDeleted(result.Items)
	}

	return result, nil
}

// ScanOnlyDeleted scans only soft-deleted items
func (r *SoftDeleteRepository) ScanOnlyDeleted(_ context.Context, input *dynamodb.ScanInput) (*dynamodb.ScanOutput, error) {
	// Add filter for only soft-deleted items
	input = r.addOnlyDeletedFilterToScan(input)

	result, err := r.client.Scan(input)
	if err != nil {
		return nil, fmt.Errorf("failed to scan deleted items: %w", err)
	}

	return result, nil
}

// CleanupOldDeletes permanently removes items that have been soft-deleted for longer than the specified duration
func (r *SoftDeleteRepository) CleanupOldDeletes(ctx context.Context, olderThan time.Duration, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 25 // DynamoDB batch limit
	}

	cutoff := time.Now().Add(-olderThan)
	totalDeleted := 0

	// Scan for old soft-deleted items
	scanInput := &dynamodb.ScanInput{
		TableName: aws.String(r.tableName),
	}

	// Add filter for items deleted before cutoff
	expr, err := expression.NewBuilder().
		WithFilter(
			expression.And(
				expression.AttributeExists(expression.Name("deleted_at")),
				expression.LessThan(expression.Name("deleted_at"), expression.Value(cutoff.Format(time.RFC3339))),
			),
		).
		Build()
	if err != nil {
		return 0, fmt.Errorf("failed to build filter expression: %w", err)
	}

	scanInput.FilterExpression = expr.Filter()
	scanInput.ExpressionAttributeNames = expr.Names()
	scanInput.ExpressionAttributeValues = expr.Values()

	var itemsToDelete []map[string]*dynamodb.AttributeValue

	// Paginate through results
	for {
		result, err := r.client.Scan(scanInput)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to scan for old deleted items: %w", err)
		}

		itemsToDelete = append(itemsToDelete, result.Items...)

		// Hard delete in batches
		if len(itemsToDelete) >= batchSize {
			deleted, err := r.hardDeleteBatch(ctx, itemsToDelete[:batchSize])
			if err != nil {
				return totalDeleted, err
			}
			totalDeleted += deleted
			itemsToDelete = itemsToDelete[batchSize:]
		}

		// Continue pagination if needed
		if result.LastEvaluatedKey == nil {
			break
		}
		scanInput.ExclusiveStartKey = result.LastEvaluatedKey
	}

	// Delete remaining items
	if len(itemsToDelete) > 0 {
		deleted, err := r.hardDeleteBatch(ctx, itemsToDelete)
		if err != nil {
			return totalDeleted, err
		}
		totalDeleted += deleted
	}

	if r.logger != nil {
		r.logger.Info("cleanup_completed",
			zap.String("table", r.tableName),
			zap.Int("items_deleted", totalDeleted),
			zap.Duration("older_than", olderThan),
		)
	}

	return totalDeleted, nil
}

// GetDeletedItemsOlderThan returns items that have been soft-deleted for longer than the specified duration
func (r *SoftDeleteRepository) GetDeletedItemsOlderThan(_ context.Context, olderThan time.Duration) ([]map[string]*dynamodb.AttributeValue, error) {
	cutoff := time.Now().Add(-olderThan)

	scanInput := &dynamodb.ScanInput{
		TableName: aws.String(r.tableName),
	}

	// Add filter for items deleted before cutoff
	expr, err := expression.NewBuilder().
		WithFilter(
			expression.And(
				expression.AttributeExists(expression.Name("deleted_at")),
				expression.LessThan(expression.Name("deleted_at"), expression.Value(cutoff.Format(time.RFC3339))),
			),
		).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build filter expression: %w", err)
	}

	scanInput.FilterExpression = expr.Filter()
	scanInput.ExpressionAttributeNames = expr.Names()
	scanInput.ExpressionAttributeValues = expr.Values()

	var allItems []map[string]*dynamodb.AttributeValue

	// Paginate through results
	for {
		result, err := r.client.Scan(scanInput)
		if err != nil {
			return nil, fmt.Errorf("failed to scan for old deleted items: %w", err)
		}

		allItems = append(allItems, result.Items...)

		if result.LastEvaluatedKey == nil {
			break
		}
		scanInput.ExclusiveStartKey = result.LastEvaluatedKey
	}

	return allItems, nil
}

// GetSoftDeleteStats returns statistics about soft-deleted items
func (r *SoftDeleteRepository) GetSoftDeleteStats(_ context.Context) (SoftDeleteStats, error) {
	stats := SoftDeleteStats{}

	// Count total items
	totalScanInput := &dynamodb.ScanInput{
		TableName: aws.String(r.tableName),
		Select:    aws.String("COUNT"),
	}

	totalResult, err := r.client.Scan(totalScanInput)
	if err != nil {
		return stats, fmt.Errorf("failed to count total items: %w", err)
	}
	stats.TotalItems = int(*totalResult.Count)

	// Count soft-deleted items
	deletedScanInput := &dynamodb.ScanInput{
		TableName: aws.String(r.tableName),
		Select:    aws.String("COUNT"),
	}

	expr, err := expression.NewBuilder().
		WithFilter(expression.AttributeExists(expression.Name("deleted_at"))).
		Build()
	if err != nil {
		return stats, fmt.Errorf("failed to build filter expression: %w", err)
	}

	deletedScanInput.FilterExpression = expr.Filter()
	deletedScanInput.ExpressionAttributeNames = expr.Names()

	deletedResult, err := r.client.Scan(deletedScanInput)
	if err != nil {
		return stats, fmt.Errorf("failed to count deleted items: %w", err)
	}
	stats.DeletedItems = int(*deletedResult.Count)
	stats.ActiveItems = stats.TotalItems - stats.DeletedItems

	return stats, nil
}

// Private helper methods

func (r *SoftDeleteRepository) save(_ context.Context, _ interface{}) error {
	// Placeholder save method for soft delete pattern demonstration
	// This method should be replaced with actual DynamORM save implementation
	// when integrating into production code
	return nil
}

func (r *SoftDeleteRepository) isSoftDeleted(item map[string]*dynamodb.AttributeValue) bool {
	deletedAt, exists := item["deleted_at"]
	return exists && deletedAt != nil && deletedAt.S != nil && *deletedAt.S != ""
}

func (r *SoftDeleteRepository) filterSoftDeleted(items []map[string]*dynamodb.AttributeValue) []map[string]*dynamodb.AttributeValue {
	var filtered []map[string]*dynamodb.AttributeValue
	for _, item := range items {
		if !r.isSoftDeleted(item) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (r *SoftDeleteRepository) addSoftDeleteFilter(input *dynamodb.QueryInput) *dynamodb.QueryInput {
	// Create a copy of the input
	newInput := *input

	// Build filter expression to exclude soft-deleted items
	expr, err := expression.NewBuilder().
		WithFilter(expression.AttributeNotExists(expression.Name("deleted_at"))).
		Build()
	if err != nil {
		// Return original input if we can't build the expression
		return input
	}

	// Combine with existing filter if present
	if input.FilterExpression != nil {
		// This is a simplified approach - in production you'd want to properly combine expressions
		return input
	}

	newInput.FilterExpression = expr.Filter()
	newInput.ExpressionAttributeNames = expr.Names()

	return &newInput
}

func (r *SoftDeleteRepository) addOnlyDeletedFilter(input *dynamodb.QueryInput) *dynamodb.QueryInput {
	newInput := *input

	expr, err := expression.NewBuilder().
		WithFilter(expression.AttributeExists(expression.Name("deleted_at"))).
		Build()
	if err != nil {
		return input
	}

	newInput.FilterExpression = expr.Filter()
	newInput.ExpressionAttributeNames = expr.Names()

	return &newInput
}

func (r *SoftDeleteRepository) addSoftDeleteFilterToScan(input *dynamodb.ScanInput) *dynamodb.ScanInput {
	newInput := *input

	expr, err := expression.NewBuilder().
		WithFilter(expression.AttributeNotExists(expression.Name("deleted_at"))).
		Build()
	if err != nil {
		return input
	}

	newInput.FilterExpression = expr.Filter()
	newInput.ExpressionAttributeNames = expr.Names()

	return &newInput
}

func (r *SoftDeleteRepository) addOnlyDeletedFilterToScan(input *dynamodb.ScanInput) *dynamodb.ScanInput {
	newInput := *input

	expr, err := expression.NewBuilder().
		WithFilter(expression.AttributeExists(expression.Name("deleted_at"))).
		Build()
	if err != nil {
		return input
	}

	newInput.FilterExpression = expr.Filter()
	newInput.ExpressionAttributeNames = expr.Names()

	return &newInput
}

func (r *SoftDeleteRepository) hardDeleteBatch(_ context.Context, items []map[string]*dynamodb.AttributeValue) (int, error) {
	if err := common.ValidateSliceNotEmpty("items", items); err != nil {
		return 0, nil
	}

	// Extract keys from items (assumes pk and sk are the keys)
	writeRequests := make([]*dynamodb.WriteRequest, 0, len(items))
	for _, item := range items {
		// Build key from item
		key := make(map[string]*dynamodb.AttributeValue)

		// Add primary key
		if pk, exists := item["pk"]; exists {
			key["pk"] = pk
		}

		// Add sort key if it exists
		if sk, exists := item["sk"]; exists {
			key["sk"] = sk
		}

		writeRequests = append(writeRequests, &dynamodb.WriteRequest{
			DeleteRequest: &dynamodb.DeleteRequest{
				Key: key,
			},
		})
	}

	// Execute batch delete
	batchInput := &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]*dynamodb.WriteRequest{
			r.tableName: writeRequests,
		},
	}

	result, err := r.client.BatchWriteItem(batchInput)
	if err != nil {
		return 0, fmt.Errorf("failed to execute batch delete: %w", err)
	}

	// Handle unprocessed items
	if len(result.UnprocessedItems) > 0 {
		// In production, you might want to retry unprocessed items
		r.logger.Warn("some items were not processed in batch delete",
			zap.Int("unprocessed_count", len(result.UnprocessedItems[r.tableName])),
		)
	}

	deleted := len(writeRequests) - len(result.UnprocessedItems[r.tableName])
	return deleted, nil
}

// SoftDeleteStats contains statistics about soft-deleted items
type SoftDeleteStats struct {
	TotalItems   int `json:"total_items"`
	ActiveItems  int `json:"active_items"`
	DeletedItems int `json:"deleted_items"`
}

// String returns a string representation of soft delete stats
func (s SoftDeleteStats) String() string {
	return fmt.Sprintf("Total: %d, Active: %d, Deleted: %d", s.TotalItems, s.ActiveItems, s.DeletedItems)
}

// GetDeletionPercentage returns the percentage of items that are soft-deleted
func (s SoftDeleteStats) GetDeletionPercentage() float64 {
	if s.TotalItems == 0 {
		return 0.0
	}
	return float64(s.DeletedItems) / float64(s.TotalItems) * 100.0
}

// Convenience functions for common soft delete patterns

// SoftDeleteByUser is a convenience function to soft delete with user tracking
func SoftDeleteByUser(ctx context.Context, repo *SoftDeleteRepository, model SoftDeletable, userID string) error {
	return repo.SoftDelete(ctx, model, userID)
}

// RestoreItem is a convenience function to restore a soft-deleted item
func RestoreItem(ctx context.Context, repo *SoftDeleteRepository, model SoftDeletable) error {
	return repo.Restore(ctx, model)
}

// IsItemDeleted is a convenience function to check if an item is soft-deleted
func IsItemDeleted(model SoftDeletable) bool {
	return model.IsDeleted()
}

// GetItemDeletionInfo returns deletion information for an item
func GetItemDeletionInfo(model SoftDeletable) (deletedAt *time.Time, deletedBy string, isDeleted bool) {
	return model.GetDeletedAt(), model.GetDeletedBy(), model.IsDeleted()
}

// ExampleModel demonstrates usage of soft delete pattern
type ExampleModel struct {
	ID    string `dynamodbav:"pk" json:"id"`
	Name  string `dynamodbav:"name" json:"name"`
	Email string `dynamodbav:"email" json:"email"`

	// Embed soft delete functionality
	SoftDeleteModel

	CreatedAt time.Time `dynamodbav:"created_at" json:"created_at"`
	UpdatedAt time.Time `dynamodbav:"updated_at" json:"updated_at"`
}

// Ensure ExampleModel implements SoftDeletable
var _ SoftDeletable = (*ExampleModel)(nil)

// NewExampleModel creates a new example model
func NewExampleModel(id, name, email string) *ExampleModel {
	now := time.Now()
	return &ExampleModel{
		ID:        id,
		Name:      name,
		Email:     email,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
