// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ImportRepository defines the interface for import operations.
// This handles data import jobs, status tracking, progress updates, and budget management.
type ImportRepository interface {
	// ===== Core Import Operations =====

	// CreateImport creates a new import record
	CreateImport(ctx context.Context, importRecord *models.Import) error

	// GetImport retrieves an import by ID
	GetImport(ctx context.Context, importID string) (*models.Import, error)

	// UpdateImportStatus updates the status and metadata of an import
	UpdateImportStatus(ctx context.Context, importID, status string, completionData map[string]any, errorMsg string) error

	// UpdateImportProgress updates the progress of an import
	UpdateImportProgress(ctx context.Context, importID string, progress int) error

	// ===== User Import Operations =====

	// GetImportsForUser retrieves all imports for a user
	GetImportsForUser(ctx context.Context, username string, limit int, cursor string) ([]*models.Import, string, error)

	// GetUserImportsByStatus retrieves imports for a user filtered by status
	GetUserImportsByStatus(ctx context.Context, username string, statuses []string) ([]*models.Import, error)

	// ===== Cost Tracking Operations =====

	// CreateImportCostTracking creates a new import cost tracking record
	CreateImportCostTracking(ctx context.Context, costTracking *models.ImportCostTracking) error

	// GetImportCostTracking retrieves import cost tracking records for an import
	GetImportCostTracking(ctx context.Context, importID string) ([]*models.ImportCostTracking, error)

	// GetUserImportCosts retrieves import costs for a user within a date range
	GetUserImportCosts(ctx context.Context, username string, startDate, endDate time.Time, limit int) ([]*models.ImportCostTracking, error)

	// GetImportCostsByDateRange retrieves import costs for all users within a date range
	GetImportCostsByDateRange(ctx context.Context, startDate, endDate time.Time, limit int) ([]*models.ImportCostTracking, error)

	// GetImportCostSummary calculates cost summary for a user's imports
	GetImportCostSummary(ctx context.Context, username string, startDate, endDate time.Time) (*models.ImportCostSummary, error)

	// GetHighCostImports returns import operations that exceed a cost threshold
	GetHighCostImports(ctx context.Context, thresholdMicroCents int64, startDate, endDate time.Time, limit int) ([]*models.ImportCostTracking, error)

	// ===== Budget Management Operations =====

	// CreateImportBudget creates a new import budget configuration
	CreateImportBudget(ctx context.Context, budget *models.ImportBudget) error

	// UpdateImportBudget updates an existing import budget
	UpdateImportBudget(ctx context.Context, budget *models.ImportBudget) error

	// GetImportBudget retrieves import budget configuration for a user
	GetImportBudget(ctx context.Context, username, period string) (*models.ImportBudget, error)

	// CheckBudgetLimits checks if a user is within their budget limits
	CheckBudgetLimits(ctx context.Context, username string, importCostMicroCents, exportCostMicroCents int64) (*models.ImportBudget, bool, error)

	// UpdateBudgetUsage updates the current usage for a user's budget
	UpdateBudgetUsage(ctx context.Context, username, period string, importCostMicroCents, exportCostMicroCents int64) error
}
