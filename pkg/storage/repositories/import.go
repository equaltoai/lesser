package repositories

import (
	"context"
	"fmt"
	"sort"
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
func (r *ImportRepository) GetImport(_ context.Context, importID string) (*models.Import, error) {
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
func (r *ImportRepository) GetImportsForUser(_ context.Context, username string, limit int, cursor string) ([]*models.Import, string, error) {
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

// GetUserImportsByStatus retrieves imports for a user filtered by status
func (r *ImportRepository) GetUserImportsByStatus(_ context.Context, username string, statuses []string) ([]*models.Import, error) {
	var imports []*models.Import

	// Query using GSI1
	query := r.db.Model(&models.Import{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("USER#%s", username))

	// Get all imports for the user, then filter by status in memory
	// This is because DynamORM doesn't support complex OR filters on non-key attributes
	err := query.All(&imports)
	if err != nil {
		r.logger.Error("failed to query imports by GSI1",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get imports for user: %w", err)
	}

	// Filter by status if specified
	if len(statuses) > 0 {
		statusMap := make(map[string]bool)
		for _, status := range statuses {
			statusMap[status] = true
		}

		filtered := make([]*models.Import, 0)
		for _, imp := range imports {
			if statusMap[imp.Status] {
				filtered = append(filtered, imp)
			}
		}
		imports = filtered
	}

	// Sort by creation date descending (most recent first)
	for i, j := 0, len(imports)-1; i < j; i, j = i+1, j-1 {
		imports[i], imports[j] = imports[j], imports[i]
	}

	r.logger.Debug("retrieved imports by status",
		zap.String("username", username),
		zap.Strings("statuses", statuses),
		zap.Int("count", len(imports)))

	return imports, nil
}

// Import Cost Tracking Methods

// CreateImportCostTracking creates a new import cost tracking record
func (r *ImportRepository) CreateImportCostTracking(_ context.Context, costTracking *models.ImportCostTracking) error {
	if err := costTracking.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	err := r.db.Model(costTracking).Create()
	if err != nil {
		r.logger.Error("failed to create import cost tracking",
			zap.String("import_id", costTracking.ImportID),
			zap.String("username", costTracking.Username),
			zap.Error(err))
		return fmt.Errorf("failed to create import cost tracking: %w", err)
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
		return nil, fmt.Errorf("failed to get import cost tracking: %w", err)
	}

	return costTrackingRecords, nil
}

// GetUserImportCosts retrieves import costs for a user within a date range
func (r *ImportRepository) GetUserImportCosts(_ context.Context, username string, startDate, endDate time.Time, limit int) ([]*models.ImportCostTracking, error) {
	var costTrackingRecords []*models.ImportCostTracking

	startSK := fmt.Sprintf("COST#%s", startDate.Format(time.RFC3339))
	endSK := fmt.Sprintf("COST#%s", endDate.Format(time.RFC3339))

	query := r.db.Model(&models.ImportCostTracking{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("GSI1SK", ">=", startSK).
		Where("GSI1SK", "<=", endSK).
		OrderBy("GSI1SK", "DESC").
		Limit(limit)

	err := query.All(&costTrackingRecords)
	if err != nil {
		r.logger.Error("failed to get user import costs",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get user import costs: %w", err)
	}

	return costTrackingRecords, nil
}

// GetImportCostsByDateRange retrieves import costs for all users within a date range
func (r *ImportRepository) GetImportCostsByDateRange(_ context.Context, startDate, endDate time.Time, limit int) ([]*models.ImportCostTracking, error) {
	var allCosts []*models.ImportCostTracking

	// Query by daily partitions
	currentDate := startDate
	for currentDate.Before(endDate) || currentDate.Equal(endDate) {
		dateStr := currentDate.Format(common.CompactDateFormat)

		var dailyCosts []*models.ImportCostTracking
		query := r.db.Model(&models.ImportCostTracking{}).
			Index("GSI2").
			Where("GSI2PK", "=", fmt.Sprintf("IMPORT_COSTS#%s", dateStr)).
			OrderBy("GSI2SK", "DESC").
			Limit(limit)

		err := query.All(&dailyCosts)
		if err != nil {
			r.logger.Warn("failed to get import costs for date",
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

	if len(costs) == 0 {
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
	// Get all recent costs
	allCosts, err := r.GetImportCostsByDateRange(ctx, startDate, endDate, limit*10) // Get more to filter
	if err != nil {
		return nil, err
	}

	// Filter by threshold
	var highCostImports []*models.ImportCostTracking
	for _, cost := range allCosts {
		if cost.TotalCostMicroCents >= thresholdMicroCents {
			highCostImports = append(highCostImports, cost)
			if len(highCostImports) >= limit {
				break
			}
		}
	}

	return highCostImports, nil
}

// Budget Management Methods

// CreateImportBudget creates a new import budget configuration
func (r *ImportRepository) CreateImportBudget(_ context.Context, budget *models.ImportBudget) error {
	if err := budget.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	err := r.db.Model(budget).Create()
	if err != nil {
		r.logger.Error("failed to create import budget",
			zap.String("username", budget.Username),
			zap.String("period", budget.Period),
			zap.Error(err))
		return fmt.Errorf("failed to create import budget: %w", err)
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
		return fmt.Errorf("before update validation failed: %w", err)
	}

	err := r.db.Model(budget).Update()
	if err != nil {
		r.logger.Error("failed to update import budget",
			zap.String("username", budget.Username),
			zap.String("period", budget.Period),
			zap.Error(err))
		return fmt.Errorf("failed to update import budget: %w", err)
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
			return nil, fmt.Errorf("import budget not found for user %s period %s", username, period)
		}
		r.logger.Error("failed to get import budget",
			zap.String("username", username),
			zap.String("period", period),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get import budget: %w", err)
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
			return fmt.Errorf("failed to create default budget: %w", err)
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
