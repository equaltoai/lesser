// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ImportRepository is a thread-safe in-memory implementation of interfaces.ImportRepository.
// It stores data in memory for integration-style testing without requiring DynamoDB.
type ImportRepository struct {
	mu sync.RWMutex

	// Imports storage: importID -> Import
	imports map[string]*models.Import

	// Imports by user: username -> []importID
	importsByUser map[string][]string

	// Cost tracking: importID -> []ImportCostTracking
	costTracking map[string][]*models.ImportCostTracking

	// Cost tracking by user: username -> []ImportCostTracking
	costByUser map[string][]*models.ImportCostTracking

	// Budgets: username#period -> ImportBudget
	budgets map[string]*models.ImportBudget
}

// NewImportRepository creates a new in-memory import repository
func NewImportRepository() *ImportRepository {
	return &ImportRepository{
		imports:       make(map[string]*models.Import),
		importsByUser: make(map[string][]string),
		costTracking:  make(map[string][]*models.ImportCostTracking),
		costByUser:    make(map[string][]*models.ImportCostTracking),
		budgets:       make(map[string]*models.ImportBudget),
	}
}

// ===== Core Import Operations =====

// CreateImport creates a new import record
func (r *ImportRepository) CreateImport(_ context.Context, importRecord *models.Import) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if importRecord == nil || importRecord.ID == "" {
		return fmt.Errorf("import ID is required")
	}

	if _, exists := r.imports[importRecord.ID]; exists {
		return storage.ErrAlreadyExists
	}

	importRecord.CreatedAt = time.Now()
	importRecord.UpdatedAt = time.Now()

	r.imports[importRecord.ID] = importRecord
	r.importsByUser[importRecord.Username] = append(r.importsByUser[importRecord.Username], importRecord.ID)

	return nil
}

// GetImport retrieves an import by ID
func (r *ImportRepository) GetImport(_ context.Context, importID string) (*models.Import, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	importRecord, exists := r.imports[importID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return importRecord, nil
}

// UpdateImportStatus updates the status and metadata of an import
func (r *ImportRepository) UpdateImportStatus(_ context.Context, importID, status string, completionData map[string]any, errorMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	importRecord, exists := r.imports[importID]
	if !exists {
		return storage.ErrNotFound
	}

	importRecord.Status = status
	importRecord.UpdatedAt = time.Now()

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

		if status == "completed" {
			now := time.Now()
			importRecord.CompletedAt = &now
		}
	}

	if errorMsg != "" {
		importRecord.Error = errorMsg
	}

	return nil
}

// UpdateImportProgress updates the progress of an import
func (r *ImportRepository) UpdateImportProgress(_ context.Context, importID string, progress int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	importRecord, exists := r.imports[importID]
	if !exists {
		return storage.ErrNotFound
	}

	importRecord.Progress = progress
	importRecord.UpdatedAt = time.Now()

	return nil
}

// ===== User Import Operations =====

// GetImportsForUser retrieves all imports for a user
func (r *ImportRepository) GetImportsForUser(_ context.Context, username string, limit int, cursor string) ([]*models.Import, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	importIDs := r.importsByUser[username]
	if len(importIDs) == 0 {
		return []*models.Import{}, "", nil
	}

	safeLimit := clampLimit(limit)

	startIdx := 0
	if cursor != "" {
		for i, id := range importIDs {
			if id == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []*models.Import
	for i := startIdx; i < len(importIDs) && len(results) < safeLimit; i++ {
		if importRecord, exists := r.imports[importIDs[i]]; exists {
			results = append(results, importRecord)
		}
	}

	var nextCursor string
	if startIdx+safeLimit < len(importIDs) {
		nextCursor = importIDs[startIdx+safeLimit-1]
	}

	return results, nextCursor, nil
}

// GetUserImportsByStatus retrieves imports for a user filtered by status
func (r *ImportRepository) GetUserImportsByStatus(_ context.Context, username string, statuses []string) ([]*models.Import, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	statusMap := make(map[string]bool)
	for _, s := range statuses {
		statusMap[s] = true
	}

	var results []*models.Import
	for _, importID := range r.importsByUser[username] {
		if importRecord, exists := r.imports[importID]; exists {
			if statusMap[importRecord.Status] {
				results = append(results, importRecord)
			}
		}
	}

	return results, nil
}

// ===== Cost Tracking Operations =====

// CreateImportCostTracking creates a new import cost tracking record
func (r *ImportRepository) CreateImportCostTracking(_ context.Context, costTracking *models.ImportCostTracking) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if costTracking == nil || costTracking.ImportID == "" {
		return fmt.Errorf("import ID is required")
	}

	r.costTracking[costTracking.ImportID] = append(r.costTracking[costTracking.ImportID], costTracking)
	r.costByUser[costTracking.Username] = append(r.costByUser[costTracking.Username], costTracking)

	return nil
}

// GetImportCostTracking retrieves import cost tracking records for an import
func (r *ImportRepository) GetImportCostTracking(_ context.Context, importID string) ([]*models.ImportCostTracking, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	costs := r.costTracking[importID]
	if costs == nil {
		return []*models.ImportCostTracking{}, nil
	}

	return costs, nil
}

// GetUserImportCosts retrieves import costs for a user within a date range
func (r *ImportRepository) GetUserImportCosts(_ context.Context, username string, startDate, endDate time.Time, limit int) ([]*models.ImportCostTracking, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.ImportCostTracking
	for _, cost := range r.costByUser[username] {
		if cost.CreatedAt.After(startDate) && cost.CreatedAt.Before(endDate) {
			results = append(results, cost)
			if len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}

// GetImportCostsByDateRange retrieves import costs for all users within a date range
func (r *ImportRepository) GetImportCostsByDateRange(_ context.Context, startDate, endDate time.Time, limit int) ([]*models.ImportCostTracking, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.ImportCostTracking
	for _, costs := range r.costByUser {
		for _, cost := range costs {
			if cost.CreatedAt.After(startDate) && cost.CreatedAt.Before(endDate) {
				results = append(results, cost)
				if len(results) >= limit {
					return results, nil
				}
			}
		}
	}

	return results, nil
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

	for _, cost := range costs {
		summary.TotalImports++
		summary.TotalCostMicroCents += cost.TotalCostMicroCents
		summary.TotalRecordsProcessed += cost.ProcessedCount
		summary.TotalRecordsSucceeded += cost.SuccessCount
		summary.TotalRecordsFailed += cost.ErrorCount

		switch cost.Status {
		case "completed":
			summary.CompletedImports++
		case "failed":
			summary.FailedImports++
		}
	}

	if summary.TotalImports > 0 {
		summary.AverageCostPerImport = float64(summary.TotalCostMicroCents) / 1_000_000.0 / float64(summary.TotalImports)
	}

	if summary.TotalRecordsProcessed > 0 {
		summary.AverageCostPerRecord = float64(summary.TotalCostMicroCents) / 1_000_000.0 / float64(summary.TotalRecordsProcessed)
		summary.OverallSuccessRate = float64(summary.TotalRecordsSucceeded) / float64(summary.TotalRecordsProcessed)
	}

	return summary, nil
}

// GetHighCostImports returns import operations that exceed a cost threshold
func (r *ImportRepository) GetHighCostImports(_ context.Context, thresholdMicroCents int64, startDate, endDate time.Time, limit int) ([]*models.ImportCostTracking, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.ImportCostTracking
	for _, costs := range r.costByUser {
		for _, cost := range costs {
			if cost.TotalCostMicroCents >= thresholdMicroCents &&
				cost.CreatedAt.After(startDate) && cost.CreatedAt.Before(endDate) {
				results = append(results, cost)
				if len(results) >= limit {
					sort.Slice(results, func(i, j int) bool {
						return results[i].TotalCostMicroCents > results[j].TotalCostMicroCents
					})
					return results, nil
				}
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].TotalCostMicroCents > results[j].TotalCostMicroCents
	})

	return results, nil
}

// ===== Budget Management Operations =====

// CreateImportBudget creates a new import budget configuration
func (r *ImportRepository) CreateImportBudget(_ context.Context, budget *models.ImportBudget) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if budget == nil || budget.Username == "" || budget.Period == "" {
		return fmt.Errorf("budget username and period are required")
	}

	key := fmt.Sprintf("%s#%s", budget.Username, budget.Period)
	r.budgets[key] = budget

	return nil
}

// UpdateImportBudget updates an existing import budget
func (r *ImportRepository) UpdateImportBudget(_ context.Context, budget *models.ImportBudget) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if budget == nil || budget.Username == "" || budget.Period == "" {
		return fmt.Errorf("budget username and period are required")
	}

	key := fmt.Sprintf("%s#%s", budget.Username, budget.Period)
	r.budgets[key] = budget

	return nil
}

// GetImportBudget retrieves import budget configuration for a user
func (r *ImportRepository) GetImportBudget(_ context.Context, username, period string) (*models.ImportBudget, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("%s#%s", username, period)
	budget, exists := r.budgets[key]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return budget, nil
}

// CheckBudgetLimits checks if a user is within their budget limits
func (r *ImportRepository) CheckBudgetLimits(ctx context.Context, username string, importCostMicroCents, exportCostMicroCents int64) (*models.ImportBudget, bool, error) {
	periods := []string{"daily", "weekly", "monthly"}

	for _, period := range periods {
		budget, err := r.GetImportBudget(ctx, username, period)
		if err != nil {
			continue
		}

		if !budget.IsActive {
			continue
		}

		newImportCost := budget.CurrentImportCost + importCostMicroCents
		newExportCost := budget.CurrentExportCost + exportCostMicroCents
		newCombinedCost := budget.CurrentCombinedCost + importCostMicroCents + exportCostMicroCents

		if budget.ImportLimitMicroCents > 0 && newImportCost > budget.ImportLimitMicroCents {
			return budget, false, nil
		}

		if budget.ExportLimitMicroCents > 0 && newExportCost > budget.ExportLimitMicroCents {
			return budget, false, nil
		}

		if budget.CombinedLimitMicroCents > 0 && newCombinedCost > budget.CombinedLimitMicroCents {
			return budget, false, nil
		}

		return budget, true, nil
	}

	return nil, true, nil
}

// UpdateBudgetUsage updates the current usage for a user's budget
func (r *ImportRepository) UpdateBudgetUsage(ctx context.Context, username, period string, importCostMicroCents, exportCostMicroCents int64) error {
	budget, err := r.GetImportBudget(ctx, username, period)
	if err != nil {
		now := time.Now()
		budget = &models.ImportBudget{
			Username:                username,
			Period:                  period,
			ImportLimitMicroCents:   10000000,
			ExportLimitMicroCents:   10000000,
			CombinedLimitMicroCents: 20000000,
			AlertThresholdPercent:   80.0,
			AlertSendingEnabled:     true,
			IsActive:                true,
			PeriodStart:             now,
			PeriodEnd:               now.AddDate(0, 0, 1),
		}

		if err := r.CreateImportBudget(ctx, budget); err != nil {
			return err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	budget.CurrentImportCost += importCostMicroCents
	budget.CurrentExportCost += exportCostMicroCents
	budget.CurrentCombinedCost += importCostMicroCents + exportCostMicroCents

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

	return nil
}

// ===== Test Helper Methods =====

// Clear clears all data (test helper)
func (r *ImportRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.imports = make(map[string]*models.Import)
	r.importsByUser = make(map[string][]string)
	r.costTracking = make(map[string][]*models.ImportCostTracking)
	r.costByUser = make(map[string][]*models.ImportCostTracking)
	r.budgets = make(map[string]*models.ImportBudget)
}

// Ensure ImportRepository implements interfaces.ImportRepository
var _ interfaces.ImportRepository = (*ImportRepository)(nil)
