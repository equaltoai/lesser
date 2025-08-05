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

// ExportRepository handles export-related database operations using DynamORM
type ExportRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewExportRepository creates a new export repository
func NewExportRepository(db core.DB, tableName string, logger *zap.Logger) *ExportRepository {
	return &ExportRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreateExport creates a new export record
func (r *ExportRepository) CreateExport(ctx context.Context, export *models.Export) error {
	export.UpdateKeys()
	export.CreatedAt = time.Now()
	export.UpdatedAt = time.Now()
	
	err := r.db.Model(export).Create()
	if err != nil {
		r.logger.Error("failed to create export",
			zap.String("export_id", export.ID),
			zap.Error(err))
		return fmt.Errorf("failed to create export: %w", err)
	}
	
	r.logger.Info("created export record",
		zap.String("export_id", export.ID),
		zap.String("username", export.Username),
		zap.String("type", export.Type),
		zap.String("format", export.Format))
	
	return nil
}

// GetExport retrieves an export by ID
func (r *ExportRepository) GetExport(ctx context.Context, exportID string) (*models.Export, error) {
	var export models.Export
	
	err := r.db.Model(&models.Export{}).
		Where("PK", "=", fmt.Sprintf("EXPORT#%s", exportID)).
		Where("SK", "=", fmt.Sprintf("EXPORT#%s", exportID)).
		First(&export)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("export not found: %s", exportID)
		}
		r.logger.Error("failed to get export",
			zap.String("export_id", exportID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get export: %w", err)
	}
	
	return &export, nil
}

// UpdateExportStatus updates the status and metadata of an export
func (r *ExportRepository) UpdateExportStatus(ctx context.Context, exportID, status string, completionData map[string]any, errorMsg string) error {
	export, err := r.GetExport(ctx, exportID)
	if err != nil {
		return err
	}
	
	// Update basic status
	export.Status = status
	export.UpdatedAt = time.Now()
	
	// Update completion data if provided
	if completionData != nil {
		if url, ok := completionData["download_url"].(string); ok {
			export.DownloadURL = url
		}
		if expiresAt, ok := completionData["expires_at"].(time.Time); ok {
			export.ExpiresAt = &expiresAt
		}
		if size, ok := completionData["file_size"].(int); ok {
			export.FileSize = int64(size)
		}
		if count, ok := completionData["record_count"].(int); ok {
			export.RecordCount = int64(count)
		}
		if s3Key, ok := completionData["s3_key"].(string); ok {
			export.S3Key = s3Key
		}
		
		// Set completion timestamp for successful exports
		if status == "completed" {
			now := time.Now()
			export.CompletedAt = &now
		}
	}
	
	// Update error message if provided
	if errorMsg != "" {
		export.Error = errorMsg
	}
	
	// Save the updated export
	export.UpdateKeys()
	err = r.db.Model(export).Update()
	if err != nil {
		r.logger.Error("failed to update export status",
			zap.String("export_id", exportID),
			zap.String("status", status),
			zap.Error(err))
		return fmt.Errorf("failed to update export status: %w", err)
	}
	
	r.logger.Info("updated export status",
		zap.String("export_id", exportID),
		zap.String("status", status),
		zap.String("error", errorMsg))
	
	return nil
}

// GetExportsForUser retrieves all exports for a user
func (r *ExportRepository) GetExportsForUser(ctx context.Context, username string, limit int, cursor string) ([]*models.Export, string, error) {
	var exports []*models.Export
	
	query := r.db.Model(&models.Export{}).
		Where("Username", "=", username).
		Limit(limit)
	
	if cursor != "" {
		// Add cursor-based pagination
		query = query.Where("CreatedAt", ">", cursor)
	}
	
	err := query.Scan(&exports)
	if err != nil {
		r.logger.Error("failed to get exports for user",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get exports for user: %w", err)
	}
	
	// Calculate next cursor
	var nextCursor string
	if len(exports) == limit {
		nextCursor = exports[len(exports)-1].CreatedAt.Format(time.RFC3339)
	}
	
	r.logger.Debug("retrieved exports for user",
		zap.String("username", username),
		zap.Int("count", len(exports)))
	
	return exports, nextCursor, nil
}

// GetUserExportsByStatus retrieves exports for a user filtered by status
func (r *ExportRepository) GetUserExportsByStatus(ctx context.Context, username string, statuses []string) ([]*models.Export, error) {
	var exports []*models.Export
	
	// Query using GSI1
	query := r.db.Model(&models.Export{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("USER#%s", username))
	
	// Get all exports for the user, then filter by status in memory
	// This is because DynamORM doesn't support complex OR filters on non-key attributes
	err := query.All(&exports)
	if err != nil {
		r.logger.Error("failed to query exports by GSI1",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get exports for user: %w", err)
	}
	
	// Filter by status if specified
	if len(statuses) > 0 {
		statusMap := make(map[string]bool)
		for _, status := range statuses {
			statusMap[status] = true
		}
		
		filtered := make([]*models.Export, 0)
		for _, export := range exports {
			if statusMap[export.Status] {
				filtered = append(filtered, export)
			}
		}
		exports = filtered
	}
	
	// Sort by creation date descending (most recent first)
	for i, j := 0, len(exports)-1; i < j; i, j = i+1, j-1 {
		exports[i], exports[j] = exports[j], exports[i]
	}
	
	r.logger.Debug("retrieved exports by status",
		zap.String("username", username),
		zap.Strings("statuses", statuses),
		zap.Int("count", len(exports)))
	
	return exports, nil
}