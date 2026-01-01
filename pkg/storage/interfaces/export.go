// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ExportRepository defines the interface for export operations.
// This handles data export jobs, status tracking, and cost management.
type ExportRepository interface {
	// ===== Core Export Operations =====

	// CreateExport creates a new export record
	CreateExport(ctx context.Context, export *models.Export) error

	// GetExport retrieves an export by ID
	GetExport(ctx context.Context, exportID string) (*models.Export, error)

	// UpdateExportStatus updates the status and metadata of an export
	UpdateExportStatus(ctx context.Context, exportID, status string, completionData map[string]any, errorMsg string) error

	// ===== User Export Operations =====

	// GetExportsForUser retrieves all exports for a user
	GetExportsForUser(ctx context.Context, username string, limit int, cursor string) ([]*models.Export, string, error)

	// GetUserExportsByStatus retrieves exports for a user filtered by status
	GetUserExportsByStatus(ctx context.Context, username string, statuses []string) ([]*models.Export, error)

	// ===== Cost Tracking Operations =====

	// CreateExportCostTracking creates a new export cost tracking record
	CreateExportCostTracking(ctx context.Context, costTracking *models.ExportCostTracking) error

	// GetExportCostTracking retrieves export cost tracking records for an export
	GetExportCostTracking(ctx context.Context, exportID string) ([]*models.ExportCostTracking, error)

	// GetUserExportCosts retrieves export costs for a user within a date range
	GetUserExportCosts(ctx context.Context, username string, startDate, endDate time.Time, limit int) ([]*models.ExportCostTracking, error)

	// GetExportCostsByDateRange retrieves export costs for all users within a date range
	GetExportCostsByDateRange(ctx context.Context, startDate, endDate time.Time, limit int) ([]*models.ExportCostTracking, error)

	// GetExportCostSummary calculates cost summary for a user's exports
	GetExportCostSummary(ctx context.Context, username string, startDate, endDate time.Time) (*models.ExportCostSummary, error)

	// GetHighCostExports returns export operations that exceed a cost threshold
	GetHighCostExports(ctx context.Context, thresholdMicroCents int64, startDate, endDate time.Time, limit int) ([]*models.ExportCostTracking, error)
}
