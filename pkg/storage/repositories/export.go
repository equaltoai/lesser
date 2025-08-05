package repositories

import (
	"context"
	"fmt"
	"sort"
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

// Export Cost Tracking Methods

// CreateExportCostTracking creates a new export cost tracking record
func (r *ExportRepository) CreateExportCostTracking(ctx context.Context, costTracking *models.ExportCostTracking) error {
	if err := costTracking.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	err := r.db.Model(costTracking).Create()
	if err != nil {
		r.logger.Error("failed to create export cost tracking",
			zap.String("export_id", costTracking.ExportID),
			zap.String("username", costTracking.Username),
			zap.Error(err))
		return fmt.Errorf("failed to create export cost tracking: %w", err)
	}

	r.logger.Debug("created export cost tracking",
		zap.String("export_id", costTracking.ExportID),
		zap.String("username", costTracking.Username),
		zap.Int64("total_cost_micro_cents", costTracking.TotalCostMicroCents),
		zap.Float64("total_cost_dollars", costTracking.GetTotalCostDollars()))

	return nil
}

// GetExportCostTracking retrieves export cost tracking records for an export
func (r *ExportRepository) GetExportCostTracking(ctx context.Context, exportID string) ([]*models.ExportCostTracking, error) {
	var costTrackingRecords []*models.ExportCostTracking

	query := r.db.Model(&models.ExportCostTracking{}).
		Where("PK", "=", fmt.Sprintf("EXPORT_COST#%s", exportID))

	err := query.All(&costTrackingRecords)
	if err != nil {
		r.logger.Error("failed to get export cost tracking",
			zap.String("export_id", exportID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get export cost tracking: %w", err)
	}

	return costTrackingRecords, nil
}

// GetUserExportCosts retrieves export costs for a user within a date range
func (r *ExportRepository) GetUserExportCosts(ctx context.Context, username string, startDate, endDate time.Time, limit int) ([]*models.ExportCostTracking, error) {
	var costTrackingRecords []*models.ExportCostTracking

	startSK := fmt.Sprintf("COST#%s", startDate.Format(time.RFC3339))
	endSK := fmt.Sprintf("COST#%s", endDate.Format(time.RFC3339))

	query := r.db.Model(&models.ExportCostTracking{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("GSI1SK", ">=", startSK).
		Where("GSI1SK", "<=", endSK).
		OrderBy("GSI1SK", "DESC").
		Limit(limit)

	err := query.All(&costTrackingRecords)
	if err != nil {
		r.logger.Error("failed to get user export costs",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get user export costs: %w", err)
	}

	return costTrackingRecords, nil
}

// GetExportCostsByDateRange retrieves export costs for all users within a date range
func (r *ExportRepository) GetExportCostsByDateRange(ctx context.Context, startDate, endDate time.Time, limit int) ([]*models.ExportCostTracking, error) {
	var allCosts []*models.ExportCostTracking

	// Query by daily partitions
	currentDate := startDate
	for currentDate.Before(endDate) || currentDate.Equal(endDate) {
		dateStr := currentDate.Format("20060102")
		
		var dailyCosts []*models.ExportCostTracking
		query := r.db.Model(&models.ExportCostTracking{}).
			Index("GSI2").
			Where("GSI2PK", "=", fmt.Sprintf("EXPORT_COSTS#%s", dateStr)).
			OrderBy("GSI2SK", "DESC").
			Limit(limit)

		err := query.All(&dailyCosts)
		if err != nil {
			r.logger.Warn("failed to get export costs for date",
				zap.String("date", dateStr),
				zap.Error(err))
			// Continue with next date
		} else {
			allCosts = append(allCosts, dailyCosts...)
		}

		// Move to next day
		currentDate = currentDate.AddDate(0, 0, 1)
		
		// Break if we have enough results
		if len(allCosts) >= limit {
			break
		}
	}

	// Sort by timestamp (newest first) and limit
	sort.Slice(allCosts, func(i, j int) bool {
		return allCosts[i].Timestamp.After(allCosts[j].Timestamp)
	})
	
	if len(allCosts) > limit {
		allCosts = allCosts[:limit]
	}

	return allCosts, nil
}

// GetExportCostSummary calculates cost summary for a user's exports
func (r *ExportRepository) GetExportCostSummary(ctx context.Context, username string, startDate, endDate time.Time) (*models.ExportCostSummary, error) {
	costs, err := r.GetUserExportCosts(ctx, username, startDate, endDate, 10000)
	if err != nil {
		return nil, err
	}

	summary := &models.ExportCostSummary{
		Username:    username,
		Period:      "custom",
		StartDate:   startDate,
		EndDate:     endDate,
		TypeBreakdown: make(map[string]*models.ExportTypeCostStats),
	}

	if len(costs) == 0 {
		return summary, nil
	}

	// Calculate statistics
	for _, cost := range costs {
		summary.TotalExports++
		
		if cost.Status == "completed" {
			summary.CompletedExports++
		} else if cost.Status == "failed" {
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
		typeStats.AverageFileSize = (typeStats.AverageFileSize*int64(typeStats.Count-1) + cost.FileSize) / int64(typeStats.Count)
		typeStats.AverageRecordCount = (typeStats.AverageRecordCount*int64(typeStats.Count-1) + cost.RecordCount) / int64(typeStats.Count)
		
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
	// Get all recent costs
	allCosts, err := r.GetExportCostsByDateRange(ctx, startDate, endDate, limit*10) // Get more to filter
	if err != nil {
		return nil, err
	}

	// Filter by threshold
	var highCostExports []*models.ExportCostTracking
	for _, cost := range allCosts {
		if cost.TotalCostMicroCents >= thresholdMicroCents {
			highCostExports = append(highCostExports, cost)
			if len(highCostExports) >= limit {
				break
			}
		}
	}

	return highCostExports, nil
}