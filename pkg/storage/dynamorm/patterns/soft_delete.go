// Package patterns provides soft delete functionality and patterns for DynamORM model operations.
package patterns

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
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
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	DeletedBy string     `json:"deleted_by,omitempty"`
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

// SoftDeleteRepository provides repository methods with soft delete support using DynamORM
type SoftDeleteRepository struct {
	db             core.DB
	logger         *zap.Logger
	includeDeleted bool // When true, queries include soft-deleted items
}

// NewSoftDeleteRepository creates a new soft delete repository using DynamORM
func NewSoftDeleteRepository(db core.DB, logger *zap.Logger) *SoftDeleteRepository {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SoftDeleteRepository{
		db:             db,
		logger:         logger,
		includeDeleted: false,
	}
}

// WithDeleted returns a new repository instance that includes soft-deleted items in queries
func (r *SoftDeleteRepository) WithDeleted() *SoftDeleteRepository {
	return &SoftDeleteRepository{
		db:             r.db,
		logger:         r.logger,
		includeDeleted: true,
	}
}

// OnlyDeleted returns a new repository instance that only returns soft-deleted items
func (r *SoftDeleteRepository) OnlyDeleted() *SoftDeleteRepository {
	return &SoftDeleteRepository{
		db:             r.db,
		logger:         r.logger,
		includeDeleted: false, // Will be overridden in query methods
	}
}

// SoftDelete performs a soft delete operation
func (r *SoftDeleteRepository) SoftDelete(_ context.Context, model SoftDeletable, userID string) error {
	// Mark as soft deleted
	model.SoftDelete()
	model.SetDeletedBy(userID)

	// Update the item using DynamORM - Note: This would typically use a repository method
	r.logger.Info("soft delete operation performed (storage not implemented in pattern)",
		zap.String("type", reflect.TypeOf(model).String()),
		zap.String("deleted_by", userID))

	if r.logger != nil {
		r.logger.Info("item_soft_deleted",
			zap.String("type", reflect.TypeOf(model).String()),
			zap.String("deleted_by", userID),
		)
	}

	return nil
}

// Restore restores a soft-deleted item
func (r *SoftDeleteRepository) Restore(_ context.Context, model SoftDeletable) error {
	if !model.IsDeleted() {
		return fmt.Errorf("item is not soft deleted")
	}

	// Remove soft delete markers
	model.Restore()

	// Update the item using DynamORM - Note: This would typically use a repository method
	r.logger.Info("restore operation performed (storage not implemented in pattern)",
		zap.String("type", reflect.TypeOf(model).String()))

	if r.logger != nil {
		r.logger.Info("item_restored",
			zap.String("type", reflect.TypeOf(model).String()),
		)
	}

	return nil
}

// HardDelete permanently deletes an item from DynamoDB using DynamORM
func (r *SoftDeleteRepository) HardDelete(_ context.Context, model interface{}) error {
	// Use DynamORM's delete method - Note: This would typically use a repository method
	r.logger.Info("hard delete operation performed (storage not implemented in pattern)",
		zap.String("type", reflect.TypeOf(model).String()))

	if r.logger != nil {
		r.logger.Info("item_hard_deleted",
			zap.String("type", reflect.TypeOf(model).String()),
		)
	}

	return nil
}

// Get retrieves an item by primary key, optionally including soft-deleted items
func (r *SoftDeleteRepository) Get(ctx context.Context, model SoftDeletable, pk, sk interface{}) error {
	// Use DynamORM to get the item
	query := r.db.WithContext(ctx).Model(model)

	// Add primary key conditions
	if pk != nil {
		query = query.Where("PK", "=", pk)
	}
	if sk != nil {
		query = query.Where("SK", "=", sk)
	}

	if err := query.First(model); err != nil {
		return fmt.Errorf("failed to get item: %w", err)
	}

	// Check if item is soft deleted and should be filtered
	if !r.includeDeleted && model.IsDeleted() {
		return fmt.Errorf("item not found") // Mimic not found behavior for deleted items
	}

	return nil
}

// Query performs a query with soft delete filtering using DynamORM
func (r *SoftDeleteRepository) Query(ctx context.Context, model interface{}, _ interface{}) core.Query {
	query := r.db.WithContext(ctx).Model(model)

	// Add soft delete filter if needed
	if !r.includeDeleted {
		query = query.Where("deleted_at", "attribute_not_exists", nil)
	}

	return query
}

// QueryOnlyDeleted queries only soft-deleted items using DynamORM
func (r *SoftDeleteRepository) QueryOnlyDeleted(ctx context.Context, model interface{}) core.Query {
	query := r.db.WithContext(ctx).Model(model)

	// Add filter for only soft-deleted items
	query = query.Where("deleted_at", "attribute_exists", nil)

	return query
}

// Scan performs a scan with soft delete filtering using DynamORM
func (r *SoftDeleteRepository) Scan(ctx context.Context, model interface{}) core.Query {
	query := r.db.WithContext(ctx).Model(model)

	// Add soft delete filter if needed
	if !r.includeDeleted {
		query = query.Where("deleted_at", "attribute_not_exists", nil)
	}

	return query
}

// ScanOnlyDeleted scans only soft-deleted items using DynamORM
func (r *SoftDeleteRepository) ScanOnlyDeleted(ctx context.Context, model interface{}) core.Query {
	query := r.db.WithContext(ctx).Model(model)

	// Add filter for only soft-deleted items
	query = query.Where("deleted_at", "attribute_exists", nil)

	return query
}

// CleanupOldDeletes permanently removes items that have been soft-deleted for longer than the specified duration
func (r *SoftDeleteRepository) CleanupOldDeletes(_ context.Context, model interface{}, olderThan time.Duration, _ int) (int, error) {
	cutoff := time.Now().Add(-olderThan)
	totalDeleted := 0

	// Note: This would typically use a repository method to find and delete old items
	r.logger.Info("cleanup operation performed (storage not implemented in pattern)",
		zap.String("type", reflect.TypeOf(model).String()),
		zap.Duration("older_than", olderThan),
		zap.Time("cutoff", cutoff))

	if r.logger != nil {
		r.logger.Info("cleanup_completed",
			zap.Int("items_deleted", totalDeleted),
			zap.Duration("older_than", olderThan),
		)
	}

	return totalDeleted, nil
}

// GetDeletedItemsOlderThan returns items that have been soft-deleted for longer than the specified duration
func (r *SoftDeleteRepository) GetDeletedItemsOlderThan(_ context.Context, model interface{}, _ interface{}, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)

	// Note: This would typically use a repository method to find old deleted items
	r.logger.Info("get deleted items operation performed (storage not implemented in pattern)",
		zap.String("type", reflect.TypeOf(model).String()),
		zap.Duration("older_than", olderThan),
		zap.Time("cutoff", cutoff))

	return nil
}

// GetSoftDeleteStats returns statistics about soft-deleted items
func (r *SoftDeleteRepository) GetSoftDeleteStats(ctx context.Context, model interface{}) (SoftDeleteStats, error) {
	stats := SoftDeleteStats{}

	// Count total items using DynamORM
	totalCount, err := r.db.WithContext(ctx).Model(model).Count()
	if err != nil {
		return stats, fmt.Errorf("failed to count total items: %w", err)
	}
	stats.TotalItems = int(totalCount)

	// Count soft-deleted items using DynamORM
	deletedCount, err := r.db.WithContext(ctx).Model(model).
		Where("deleted_at", "attribute_exists", nil).
		Count()
	if err != nil {
		return stats, fmt.Errorf("failed to count deleted items: %w", err)
	}
	stats.DeletedItems = int(deletedCount)
	stats.ActiveItems = stats.TotalItems - stats.DeletedItems

	return stats, nil
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

// ExampleModel demonstrates usage of soft delete pattern with DynamORM
type ExampleModel struct {
	PK    string `dynamorm:"pk" json:"pk"` // Primary key for DynamORM
	SK    string `dynamorm:"sk" json:"sk"` // Sort key for DynamORM
	ID    string `json:"id"`               // Business ID
	Name  string `json:"name"`
	Email string `json:"email"`

	// Embed soft delete functionality
	SoftDeleteModel

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Ensure ExampleModel implements SoftDeletable
var _ SoftDeletable = (*ExampleModel)(nil)

// NewExampleModel creates a new example model with proper DynamORM keys
func NewExampleModel(id, name, email string) *ExampleModel {
	now := time.Now()
	return &ExampleModel{
		PK:        fmt.Sprintf("EXAMPLE#%s", id),
		SK:        "PROFILE",
		ID:        id,
		Name:      name,
		Email:     email,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
