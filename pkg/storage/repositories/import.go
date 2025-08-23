package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
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
func (r *ImportRepository) CreateImport(_ context.Context, importRecord *models.Import) error {
	importRecord.UpdateKeys()
	importRecord.CreatedAt = time.Now()
	importRecord.UpdatedAt = time.Now()

	err := r.db.Model(importRecord).Create()
	if err != nil {
		r.logger.Error("failed to create import",
			zap.String("import_id", importRecord.ID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "import", importRecord.ID)
	}

	r.logger.Info("created import record",
		zap.String("import_id", importRecord.ID),
		zap.String("username", importRecord.Username),
		zap.String("type", importRecord.Type),
		zap.String("mode", importRecord.Mode))

	return nil
}

// GetImport retrieves an import by ID
func (r *ImportRepository) GetImport(_ context.Context, importID string) (*models.Import, error) {
	var importRecord models.Import

	err := r.db.Model(&models.Import{}).
		Where("PK", "=", fmt.Sprintf("IMPORT#%s", importID)).
		Where("SK", "=", fmt.Sprintf("IMPORT#%s", importID)).
		First(&importRecord)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(err, "import", importID)
		}
		r.logger.Error("failed to get import",
			zap.String("import_id", importID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "import", importID)
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
		return ErrorHandler.HandleUpdateError(err, "import", importID)
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
		return ErrorHandler.HandleUpdateError(err, "import", importID)
	}

	r.logger.Debug("updated import progress",
		zap.String("import_id", importID),
		zap.Int("progress", progress))

	return nil
}

// GetImportsForUser retrieves all imports for a user
func (r *ImportRepository) GetImportsForUser(ctx context.Context, username string, limit int, cursor string) ([]*models.Import, string, error) {
	result, nextCursor, err := getImportExportItemsForUser(ctx, r.db, r.logger, username, limit, cursor, "import", false)
	if err != nil {
		return nil, "", err
	}
	return result.([]*models.Import), nextCursor, nil
}

// GetUserImportsByStatus retrieves imports for a user filtered by status
func (r *ImportRepository) GetUserImportsByStatus(ctx context.Context, username string, statuses []string) ([]*models.Import, error) {
	result, err := getImportExportItemsByStatus(ctx, r.db, r.logger, username, statuses, "import", &models.Import{})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Import Cost Tracking Methods

// CreateImportCostTracking creates a new import cost tracking record
func (r *ImportRepository) CreateImportCostTracking(_ context.Context, costTracking *models.ImportCostTracking) error {
	if err := costTracking.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, "import cost tracking", costTracking.ImportID)
	}

	err := r.db.Model(costTracking).Create()
	if err != nil {
		r.logger.Error("failed to create import cost tracking",
			zap.String("import_id", costTracking.ImportID),
			zap.String("username", costTracking.Username),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "import cost tracking", costTracking.ImportID)
	}

	r.logger.Debug("created import cost tracking",
		zap.String("import_id", costTracking.ImportID),
		zap.String("username", costTracking.Username),
		zap.Int64("total_cost_micro_cents", costTracking.TotalCostMicroCents),
		zap.Float64("total_cost_dollars", costTracking.GetTotalCostDollars()),
		zap.Float64("success_rate", costTracking.GetSuccessRate()))

	return nil
}

// GetImportCostTracking retrieves import cost tracking records for an import
func (r *ImportRepository) GetImportCostTracking(_ context.Context, importID string) ([]*models.ImportCostTracking, error) {
	var costTrackingRecords []*models.ImportCostTracking

	query := r.db.Model(&models.ImportCostTracking{}).
		Where("PK", "=", fmt.Sprintf("IMPORT_COST#%s", importID))

	err := query.All(&costTrackingRecords)
	if err != nil {
		r.logger.Error("failed to get import cost tracking",
			zap.String("import_id", importID),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "import cost tracking", "cost tracking query")
	}

	return costTrackingRecords, nil
}

// GetUserImportCosts retrieves import costs for a user within a date range
func (r *ImportRepository) GetUserImportCosts(ctx context.Context, username string, startDate, endDate time.Time, limit int) ([]*models.ImportCostTracking, error) {
	result, err := getUserCosts(ctx, r.db, r.logger, username, startDate, endDate, limit, "import", &models.ImportCostTracking{})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetImportCostsByDateRange retrieves import costs for all users within a date range
func (r *ImportRepository) GetImportCostsByDateRange(ctx context.Context, startDate, endDate time.Time, limit int) ([]*models.ImportCostTracking, error) {
	result, err := getCostsByDateRange(ctx, r.db, r.logger, startDate, endDate, limit, "import", &models.ImportCostTracking{})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetImportCostSummary calculates cost summary for a user's imports
func (r *ImportRepository) GetImportCostSummary(ctx context.Context, username string, startDate, endDate time.Time) (*models.ImportCostSummary, error) {
	costs, err := r.GetUserImportCosts(ctx, username, startDate, endDate, 10000)
	if err != nil {
		return nil, err
	}

	summary := &models.ImportCostSummary{
		Username:      username,
		Period:        "custom",
		StartDate:     startDate,
		EndDate:       endDate,
		TypeBreakdown: make(map[string]*models.ImportTypeCostStats),
	}

	if err := common.ValidateSliceNotEmpty("costs", costs); err != nil {
		return summary, nil
	}

	// Calculate statistics
	for _, cost := range costs {
		summary.TotalImports++

		switch cost.Status {
		case StatusCompleted:
			summary.CompletedImports++
		case StatusFailed:
			summary.FailedImports++
		}

		summary.TotalLambdaCost += cost.LambdaExecutionCost
		summary.TotalS3Cost += cost.S3StorageCost + cost.S3GetRequestCost + cost.S3DataTransferCost
		summary.TotalDynamoDBCost += cost.DynamoDBWriteCost + cost.DynamoDBReadCost
		summary.TotalNetworkCost += cost.ExternalAPICallCost
		summary.TotalCostMicroCents += cost.TotalCostMicroCents

		summary.TotalRecordsProcessed += cost.ProcessedCount
		summary.TotalRecordsSucceeded += cost.SuccessCount
		summary.TotalRecordsFailed += cost.ErrorCount

		// Track by import type
		typeStats, exists := summary.TypeBreakdown[cost.Type]
		if !exists {
			typeStats = &models.ImportTypeCostStats{
				Type: cost.Type,
			}
		}

		typeStats.Count++
		typeStats.TotalCostMicroCents += cost.TotalCostMicroCents
		typeStats.TotalRecords += cost.RecordCount
		typeStats.SuccessfulRecords += cost.SuccessCount
		typeStats.FailedRecords += cost.ErrorCount

		summary.TypeBreakdown[cost.Type] = typeStats
	}

	// Calculate averages and totals
	if summary.TotalImports > 0 {
		summary.AverageCostPerImport = float64(summary.TotalCostMicroCents) / 1_000_000.0 / float64(summary.TotalImports)
	}

	if summary.TotalRecordsProcessed > 0 {
		summary.AverageCostPerRecord = float64(summary.TotalCostMicroCents) / 1_000_000.0 / float64(summary.TotalRecordsProcessed)
		summary.OverallSuccessRate = float64(summary.TotalRecordsSucceeded) / float64(summary.TotalRecordsProcessed)
	}

	// Calculate type averages
	for importType, typeStats := range summary.TypeBreakdown {
		if typeStats.Count > 0 {
			typeStats.AverageCostMicroCents = typeStats.TotalCostMicroCents / typeStats.Count
			typeStats.TotalCostDollars = float64(typeStats.TotalCostMicroCents) / 1_000_000.0

			if typeStats.TotalRecords > 0 {
				typeStats.SuccessRate = float64(typeStats.SuccessfulRecords) / float64(typeStats.TotalRecords)
			}

			summary.TypeBreakdown[importType] = typeStats
		}
	}

	return summary, nil
}

// GetHighCostImports returns import operations that exceed a cost threshold
func (r *ImportRepository) GetHighCostImports(ctx context.Context, thresholdMicroCents int64, startDate, endDate time.Time, limit int) ([]*models.ImportCostTracking, error) {
	result, err := getHighCostOperations(ctx, r.db, r.logger, thresholdMicroCents, startDate, endDate, limit, "import", &models.ImportCostTracking{})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Budget Management Methods

// CreateImportBudget creates a new import budget configuration
func (r *ImportRepository) CreateImportBudget(_ context.Context, budget *models.ImportBudget) error {
	if err := budget.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, "import budget", budget.Username)
	}

	err := r.db.Model(budget).Create()
	if err != nil {
		r.logger.Error("failed to create import budget",
			zap.String("username", budget.Username),
			zap.String("period", budget.Period),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "import budget", budget.Username)
	}

	r.logger.Info("created import budget",
		zap.String("username", budget.Username),
		zap.String("period", budget.Period),
		zap.Int64("combined_limit_micro_cents", budget.CombinedLimitMicroCents))

	return nil
}

// UpdateImportBudget updates an existing import budget
func (r *ImportRepository) UpdateImportBudget(_ context.Context, budget *models.ImportBudget) error {
	if err := budget.BeforeUpdate(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "import budget", budget.Username)
	}

	err := r.db.Model(budget).Update()
	if err != nil {
		r.logger.Error("failed to update import budget",
			zap.String("username", budget.Username),
			zap.String("period", budget.Period),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, "import budget", budget.Username)
	}

	return nil
}

// GetImportBudget retrieves import budget configuration for a user
func (r *ImportRepository) GetImportBudget(_ context.Context, username, period string) (*models.ImportBudget, error) {
	var budget models.ImportBudget

	pk := fmt.Sprintf("USER_BUDGET#%s#%s", username, period)
	sk := "CONFIG"

	err := r.db.Model(&models.ImportBudget{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&budget)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(err, "import budget", fmt.Sprintf("%s:%s", username, period))
		}
		r.logger.Error("failed to get import budget",
			zap.String("username", username),
			zap.String("period", period),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "import budget", fmt.Sprintf("%s:%s", username, period))
	}

	return &budget, nil
}

// CheckBudgetLimits checks if a user is within their budget limits
func (r *ImportRepository) CheckBudgetLimits(ctx context.Context, username string, importCostMicroCents, exportCostMicroCents int64) (*models.ImportBudget, bool, error) {
	// Try to get daily budget first, then weekly, then monthly
	periods := []string{"daily", "weekly", "monthly"}

	for _, period := range periods {
		budget, err := r.GetImportBudget(ctx, username, period)
		if err != nil {
			// Budget doesn't exist for this period, try next
			continue
		}

		if !budget.IsActive {
			continue
		}

		// Check if we would exceed limits
		newImportCost := budget.CurrentImportCost + importCostMicroCents
		newExportCost := budget.CurrentExportCost + exportCostMicroCents
		newCombinedCost := budget.CurrentCombinedCost + importCostMicroCents + exportCostMicroCents

		// Check individual limits
		if budget.ImportLimitMicroCents > 0 && newImportCost > budget.ImportLimitMicroCents {
			return budget, false, nil
		}

		if budget.ExportLimitMicroCents > 0 && newExportCost > budget.ExportLimitMicroCents {
			return budget, false, nil
		}

		// Check combined limit
		if budget.CombinedLimitMicroCents > 0 && newCombinedCost > budget.CombinedLimitMicroCents {
			return budget, false, nil
		}

		return budget, true, nil
	}

	// No budget found, allow operation
	return nil, true, nil
}

// UpdateBudgetUsage updates the current usage for a user's budget
func (r *ImportRepository) UpdateBudgetUsage(ctx context.Context, username, period string, importCostMicroCents, exportCostMicroCents int64) error {
	budget, err := r.GetImportBudget(ctx, username, period)
	if err != nil {
		// Budget doesn't exist, create a default one
		now := time.Now()
		budget = &models.ImportBudget{
			Username:                username,
			Period:                  period,
			ImportLimitMicroCents:   10000000, // $10 default
			ExportLimitMicroCents:   10000000, // $10 default
			CombinedLimitMicroCents: 20000000, // $20 default
			AlertThresholdPercent:   80.0,
			AlertSendingEnabled:     true,
			IsActive:                true,
			PeriodStart:             now,
			PeriodEnd:               now.AddDate(0, 0, 1), // Default to daily
		}

		if err := r.CreateImportBudget(ctx, budget); err != nil {
			return ErrorHandler.HandleCreateError(err, "import budget", username)
		}
	}

	// Update usage
	budget.CurrentImportCost += importCostMicroCents
	budget.CurrentExportCost += exportCostMicroCents
	budget.CurrentCombinedCost += importCostMicroCents + exportCostMicroCents

	// Update counters
	if importCostMicroCents > 0 {
		budget.ImportCount++
		now := time.Now()
		budget.LastImportAt = &now
	}

	if exportCostMicroCents > 0 {
		budget.ExportCount++
		now := time.Now()
		budget.LastExportAt = &now
	}

	return r.UpdateImportBudget(ctx, budget)
}
