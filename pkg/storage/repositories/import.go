package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ImportRepository handles import-related database operations using DynamORM
type ImportRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewImportRepository creates a new import repository
func NewImportRepository(db core.DB, tableName string, logger *zap.Logger) *ImportRepository {
	return &ImportRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreateImport creates a new import record
func (r *ImportRepository) CreateImport(ctx context.Context, importRecord *models.Import) error {
	importRecord.UpdateKeys()
	importRecord.CreatedAt = time.Now()
	importRecord.UpdatedAt = time.Now()
	
	err := r.db.Model(importRecord).Create()
	if err != nil {
		r.logger.Error("failed to create import",
			zap.String("import_id", importRecord.ID),
			zap.Error(err))
		return fmt.Errorf("failed to create import: %w", err)
	}
	
	r.logger.Info("created import record",
		zap.String("import_id", importRecord.ID),
		zap.String("username", importRecord.Username),
		zap.String("type", importRecord.Type),
		zap.String("mode", importRecord.Mode))
	
	return nil
}

// GetImport retrieves an import by ID
func (r *ImportRepository) GetImport(ctx context.Context, importID string) (*models.Import, error) {
	var importRecord models.Import
	
	err := r.db.Model(&models.Import{}).
		Where("PK", "=", fmt.Sprintf("IMPORT#%s", importID)).
		Where("SK", "=", fmt.Sprintf("IMPORT#%s", importID)).
		First(&importRecord)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("import not found: %s", importID)
		}
		r.logger.Error("failed to get import",
			zap.String("import_id", importID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get import: %w", err)
	}
	
	return &importRecord, nil
}

// UpdateImportStatus updates the status and metadata of an import
func (r *ImportRepository) UpdateImportStatus(ctx context.Context, importID, status string, completionData map[string]any, errorMsg string) error {
	importRecord, err := r.GetImport(ctx, importID)
	if err != nil {
		return err
	}
	
	// Update basic status
	importRecord.Status = status
	importRecord.UpdatedAt = time.Now()
	
	// Update completion data if provided
	if completionData != nil {
		if total, ok := completionData["total"].(int); ok {
			importRecord.Total = total
		}
		if success, ok := completionData["success"].(int); ok {
			importRecord.SuccessCount = success
		}
		if skipped, ok := completionData["skipped"].(int); ok {
			importRecord.SkipCount = skipped
		}
		if failed, ok := completionData["failed"].(int); ok {
			importRecord.ErrorCount = failed
		}
		if errors, ok := completionData["errors"].([]string); ok {
			importRecord.Errors = errors
		}
		
		// Set completion timestamp for completed imports
		if status == "completed" {
			now := time.Now()
			importRecord.CompletedAt = &now
		}
	}
	
	// Update error message if provided
	if errorMsg != "" {
		importRecord.Error = errorMsg
	}
	
	// Save the updated import
	importRecord.UpdateKeys()
	err = r.db.Model(importRecord).Update()
	if err != nil {
		r.logger.Error("failed to update import status",
			zap.String("import_id", importID),
			zap.String("status", status),
			zap.Error(err))
		return fmt.Errorf("failed to update import status: %w", err)
	}
	
	r.logger.Info("updated import status",
		zap.String("import_id", importID),
		zap.String("status", status),
		zap.String("error", errorMsg))
	
	return nil
}

// UpdateImportProgress updates the progress of an import
func (r *ImportRepository) UpdateImportProgress(ctx context.Context, importID string, progress int) error {
	importRecord, err := r.GetImport(ctx, importID)
	if err != nil {
		return err
	}
	
	importRecord.Progress = progress
	importRecord.UpdatedAt = time.Now()
	importRecord.UpdateKeys()
	
	err = r.db.Model(importRecord).Update()
	if err != nil {
		r.logger.Error("failed to update import progress",
			zap.String("import_id", importID),
			zap.Int("progress", progress),
			zap.Error(err))
		return fmt.Errorf("failed to update import progress: %w", err)
	}
	
	r.logger.Debug("updated import progress",
		zap.String("import_id", importID),
		zap.Int("progress", progress))
	
	return nil
}

// GetImportsForUser retrieves all imports for a user
func (r *ImportRepository) GetImportsForUser(ctx context.Context, username string, limit int, cursor string) ([]*models.Import, string, error) {
	var imports []*models.Import
	
	query := r.db.Model(&models.Import{}).
		Where("Username", "=", username).
		Limit(limit)
	
	if cursor != "" {
		// Add cursor-based pagination
		query = query.Where("CreatedAt", ">", cursor)
	}
	
	err := query.Scan(&imports)
	if err != nil {
		r.logger.Error("failed to get imports for user",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get imports for user: %w", err)
	}
	
	// Calculate next cursor
	var nextCursor string
	if len(imports) == limit {
		nextCursor = imports[len(imports)-1].CreatedAt.Format(time.RFC3339)
	}
	
	r.logger.Debug("retrieved imports for user",
		zap.String("username", username),
		zap.Int("count", len(imports)))
	
	return imports, nextCursor, nil
}