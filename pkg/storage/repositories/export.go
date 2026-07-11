package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	"github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
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
func (r *ExportRepository) CreateExport(_ context.Context, export *models.Export) error {
	export.UpdateKeys()
	export.CreatedAt = time.Now()
	export.UpdatedAt = time.Now()

	err := r.db.Model(export).Create()
	if err != nil {
		r.logger.Error("failed to create export",
			zap.String("export_id", export.ID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityExport, export.ID)
	}

	r.logger.Info("created export record",
		zap.String("export_id", export.ID),
		zap.String("username", export.Username),
		zap.String("type", export.Type),
		zap.String("format", export.Format))

	return nil
}

// GetExport retrieves an export by ID
func (r *ExportRepository) GetExport(_ context.Context, exportID string) (*models.Export, error) {
	var export models.Export

	err := r.db.Model(&models.Export{}).
		Where("PK", "=", fmt.Sprintf("EXPORT#%s", exportID)).
		Where("SK", "=", fmt.Sprintf("EXPORT#%s", exportID)).
		First(&export)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleNotFound(err, EntityExport, exportID)
		}
		r.logger.Error("failed to get export",
			zap.String("export_id", exportID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityExport, exportID)
	}

	return &export, nil
}

// UpdateExportStatus updates the status and metadata of an export
func (r *ExportRepository) UpdateExportStatus(ctx context.Context, exportID, status string, completionData map[string]any, errorMsg string) error {
	// Validate status using centralized validation
	if err := common.ValidateStatusState(status); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityExport, exportID)
	}

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
		if status == StatusCompleted {
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
		return ErrorHandler.HandleUpdateError(err, EntityExport, exportID)
	}

	r.logger.Info("updated export status",
		zap.String("export_id", exportID),
		zap.String("status", status),
		zap.String("error", errorMsg))

	return nil
}

// GetExportsForUser retrieves all exports for a user
func (r *ExportRepository) GetExportsForUser(ctx context.Context, username string, limit int, cursor string) ([]*models.Export, string, error) {
	result, nextCursor, err := getImportExportItemsForUser(ctx, r.db, r.logger, username, limit, cursor, "export", true)
	if err != nil {
		return nil, "", err
	}
	return result.([]*models.Export), nextCursor, nil
}

// GetUserExportsByStatus retrieves exports for a user filtered by status
func (r *ExportRepository) GetUserExportsByStatus(ctx context.Context, username string, statuses []string) ([]*models.Export, error) {
	result, err := getImportExportItemsByStatus(ctx, r.db, r.logger, username, statuses, "export", &models.Export{})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Export Cost Tracking Methods

// CreateExportCostTracking creates a new export cost tracking record
func (r *ExportRepository) CreateExportCostTracking(_ context.Context, costTracking *models.ExportCostTracking) error {
	if err := costTracking.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityExportCostTracking, costTracking.ExportID)
	}

	err := r.db.Model(costTracking).Create()
	if err != nil {
		r.logger.Error("failed to create export cost tracking",
			zap.String("export_id", costTracking.ExportID),
			zap.String("username", costTracking.Username),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityExportCostTracking, costTracking.ExportID)
	}

	r.logger.Debug("created export cost tracking",
		zap.String("export_id", costTracking.ExportID),
		zap.String("username", costTracking.Username),
		zap.Int64("total_cost_micro_cents", costTracking.TotalCostMicroCents),
		zap.Float64("total_cost_dollars", costTracking.GetTotalCostDollars()))

	return nil
}

// GetExportCostTracking retrieves export cost tracking records for an export
func (r *ExportRepository) GetExportCostTracking(_ context.Context, exportID string) ([]*models.ExportCostTracking, error) {
	var costTrackingRecords []*models.ExportCostTracking

	query := r.db.Model(&models.ExportCostTracking{}).
		Where("PK", "=", fmt.Sprintf("EXPORT_COST#%s", exportID))

	err := query.All(&costTrackingRecords)
	if err != nil {
		r.logger.Error("failed to get export cost tracking",
			zap.String("export_id", exportID),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityExportCostTracking, "by export ID")
	}

	return costTrackingRecords, nil
}

// GetUserExportCosts retrieves export costs for a user within a date range
func (r *ExportRepository) GetUserExportCosts(ctx context.Context, username string, startDate, endDate time.Time, limit int) ([]*models.ExportCostTracking, error) {
	result, err := getUserCosts(ctx, r.db, r.logger, username, startDate, endDate, limit, "export", &models.ExportCostTracking{})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetExportCostsByDateRange retrieves export costs for all users within a date range
func (r *ExportRepository) GetExportCostsByDateRange(ctx context.Context, startDate, endDate time.Time, limit int) ([]*models.ExportCostTracking, error) {
	result, err := getCostsByDateRange(ctx, r.db, r.logger, startDate, endDate, limit, "export", &models.ExportCostTracking{})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetExportCostSummary calculates cost summary for a user's exports
func (r *ExportRepository) GetExportCostSummary(ctx context.Context, username string, startDate, endDate time.Time) (*models.ExportCostSummary, error) {
	costs, err := r.GetUserExportCosts(ctx, username, startDate, endDate, 10000)
	if err != nil {
		return nil, err
	}

	summary := &models.ExportCostSummary{
		Username:      username,
		Period:        "custom",
		StartDate:     startDate,
		EndDate:       endDate,
		TypeBreakdown: make(map[string]*models.ExportTypeCostStats),
	}

	if err := common.ValidateSliceNotEmpty("costs", costs); err != nil {
		return summary, nil
	}

	// Calculate statistics
	for _, cost := range costs {
		summary.TotalExports++

		switch cost.Status {
		case StatusCompleted:
			summary.CompletedExports++
		case StatusFailed:
			summary.FailedExports++
		}

		summary.TotalLambdaCost += cost.LambdaExecutionCost
		summary.TotalS3Cost += cost.S3StorageCost + cost.S3PutRequestCost + cost.S3GetRequestCost + cost.S3DataTransferCost
		summary.TotalDynamoDBCost += cost.DynamoDBReadCost
		summary.TotalCostMicroCents += cost.TotalCostMicroCents

		summary.TotalFileSize += cost.FileSize
		summary.TotalRecordCount += cost.RecordCount
		summary.TotalMediaFiles += cost.MediaFilesIncluded

		// Track by export type
		typeStats, exists := summary.TypeBreakdown[cost.Type]
		if !exists {
			typeStats = &models.ExportTypeCostStats{
				Type: cost.Type,
			}
		}

		typeStats.Count++
		typeStats.TotalCostMicroCents += cost.TotalCostMicroCents
		typeStats.AverageFileSize = (typeStats.AverageFileSize*(typeStats.Count-1) + cost.FileSize) / typeStats.Count
		typeStats.AverageRecordCount = (typeStats.AverageRecordCount*(typeStats.Count-1) + cost.RecordCount) / typeStats.Count

		summary.TypeBreakdown[cost.Type] = typeStats
	}

	// Calculate averages and totals
	if summary.TotalExports > 0 {
		summary.AverageCostPerExport = float64(summary.TotalCostMicroCents) / 1_000_000.0 / float64(summary.TotalExports)
		summary.AverageExportSize = summary.TotalFileSize / summary.TotalExports
	}

	// Calculate type averages
	for exportType, typeStats := range summary.TypeBreakdown {
		if typeStats.Count > 0 {
			typeStats.AverageCostMicroCents = typeStats.TotalCostMicroCents / typeStats.Count
			typeStats.TotalCostDollars = float64(typeStats.TotalCostMicroCents) / 1_000_000.0
			summary.TypeBreakdown[exportType] = typeStats
		}
	}

	return summary, nil
}

// GetHighCostExports returns export operations that exceed a cost threshold
func (r *ExportRepository) GetHighCostExports(ctx context.Context, thresholdMicroCents int64, startDate, endDate time.Time, limit int) ([]*models.ExportCostTracking, error) {
	result, err := getHighCostOperations(ctx, r.db, r.logger, thresholdMicroCents, startDate, endDate, limit, "export", &models.ExportCostTracking{})
	if err != nil {
		return nil, err
	}
	return result, nil
}
