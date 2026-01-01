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

// ExportRepository is a thread-safe in-memory implementation of interfaces.ExportRepository.
// It stores data in memory for integration-style testing without requiring DynamoDB.
type ExportRepository struct {
	mu sync.RWMutex

	// Exports storage: exportID -> Export
	exports map[string]*models.Export

	// Exports by user: username -> []exportID
	exportsByUser map[string][]string

	// Cost tracking: exportID -> []ExportCostTracking
	costTracking map[string][]*models.ExportCostTracking

	// Cost tracking by user: username -> []ExportCostTracking
	costByUser map[string][]*models.ExportCostTracking
}

// NewExportRepository creates a new in-memory export repository
func NewExportRepository() *ExportRepository {
	return &ExportRepository{
		exports:       make(map[string]*models.Export),
		exportsByUser: make(map[string][]string),
		costTracking:  make(map[string][]*models.ExportCostTracking),
		costByUser:    make(map[string][]*models.ExportCostTracking),
	}
}

// ===== Core Export Operations =====

// CreateExport creates a new export record
func (r *ExportRepository) CreateExport(_ context.Context, export *models.Export) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if export == nil || export.ID == "" {
		return fmt.Errorf("export ID is required")
	}

	if _, exists := r.exports[export.ID]; exists {
		return storage.ErrAlreadyExists
	}

	export.CreatedAt = time.Now()
	export.UpdatedAt = time.Now()

	r.exports[export.ID] = export
	r.exportsByUser[export.Username] = append(r.exportsByUser[export.Username], export.ID)

	return nil
}

// GetExport retrieves an export by ID
func (r *ExportRepository) GetExport(_ context.Context, exportID string) (*models.Export, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	export, exists := r.exports[exportID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return export, nil
}

// UpdateExportStatus updates the status and metadata of an export
func (r *ExportRepository) UpdateExportStatus(_ context.Context, exportID, status string, completionData map[string]any, errorMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	export, exists := r.exports[exportID]
	if !exists {
		return storage.ErrNotFound
	}

	export.Status = status
	export.UpdatedAt = time.Now()

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

		if status == "completed" {
			now := time.Now()
			export.CompletedAt = &now
		}
	}

	if errorMsg != "" {
		export.Error = errorMsg
	}

	return nil
}

// ===== User Export Operations =====

// GetExportsForUser retrieves all exports for a user
func (r *ExportRepository) GetExportsForUser(_ context.Context, username string, limit int, cursor string) ([]*models.Export, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	exportIDs := r.exportsByUser[username]
	if len(exportIDs) == 0 {
		return []*models.Export{}, "", nil
	}

	safeLimit := clampLimit(limit)

	// Find start index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, id := range exportIDs {
			if id == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []*models.Export
	for i := startIdx; i < len(exportIDs) && len(results) < safeLimit; i++ {
		if export, exists := r.exports[exportIDs[i]]; exists {
			results = append(results, export)
		}
	}

	var nextCursor string
	if startIdx+safeLimit < len(exportIDs) {
		nextCursor = exportIDs[startIdx+safeLimit-1]
	}

	return results, nextCursor, nil
}

// GetUserExportsByStatus retrieves exports for a user filtered by status
func (r *ExportRepository) GetUserExportsByStatus(_ context.Context, username string, statuses []string) ([]*models.Export, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	statusMap := make(map[string]bool)
	for _, s := range statuses {
		statusMap[s] = true
	}

	var results []*models.Export
	for _, exportID := range r.exportsByUser[username] {
		if export, exists := r.exports[exportID]; exists {
			if statusMap[export.Status] {
				results = append(results, export)
			}
		}
	}

	return results, nil
}

// ===== Cost Tracking Operations =====

// CreateExportCostTracking creates a new export cost tracking record
func (r *ExportRepository) CreateExportCostTracking(_ context.Context, costTracking *models.ExportCostTracking) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if costTracking == nil || costTracking.ExportID == "" {
		return fmt.Errorf("export ID is required")
	}

	r.costTracking[costTracking.ExportID] = append(r.costTracking[costTracking.ExportID], costTracking)
	r.costByUser[costTracking.Username] = append(r.costByUser[costTracking.Username], costTracking)

	return nil
}

// GetExportCostTracking retrieves export cost tracking records for an export
func (r *ExportRepository) GetExportCostTracking(_ context.Context, exportID string) ([]*models.ExportCostTracking, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	costs := r.costTracking[exportID]
	if costs == nil {
		return []*models.ExportCostTracking{}, nil
	}

	return costs, nil
}

// GetUserExportCosts retrieves export costs for a user within a date range
func (r *ExportRepository) GetUserExportCosts(_ context.Context, username string, startDate, endDate time.Time, limit int) ([]*models.ExportCostTracking, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.ExportCostTracking
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

// GetExportCostsByDateRange retrieves export costs for all users within a date range
func (r *ExportRepository) GetExportCostsByDateRange(_ context.Context, startDate, endDate time.Time, limit int) ([]*models.ExportCostTracking, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.ExportCostTracking
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

	if len(costs) == 0 {
		return summary, nil
	}

	for _, cost := range costs {
		summary.TotalExports++
		summary.TotalCostMicroCents += cost.TotalCostMicroCents
		summary.TotalFileSize += cost.FileSize
		summary.TotalRecordCount += cost.RecordCount
		summary.TotalMediaFiles += cost.MediaFilesIncluded

		switch cost.Status {
		case "completed":
			summary.CompletedExports++
		case "failed":
			summary.FailedExports++
		}
	}

	if summary.TotalExports > 0 {
		summary.AverageCostPerExport = float64(summary.TotalCostMicroCents) / 1_000_000.0 / float64(summary.TotalExports)
		summary.AverageExportSize = summary.TotalFileSize / summary.TotalExports
	}

	return summary, nil
}

// GetHighCostExports returns export operations that exceed a cost threshold
func (r *ExportRepository) GetHighCostExports(_ context.Context, thresholdMicroCents int64, startDate, endDate time.Time, limit int) ([]*models.ExportCostTracking, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.ExportCostTracking
	for _, costs := range r.costByUser {
		for _, cost := range costs {
			if cost.TotalCostMicroCents >= thresholdMicroCents &&
				cost.CreatedAt.After(startDate) && cost.CreatedAt.Before(endDate) {
				results = append(results, cost)
				if len(results) >= limit {
					// Sort by cost descending
					sort.Slice(results, func(i, j int) bool {
						return results[i].TotalCostMicroCents > results[j].TotalCostMicroCents
					})
					return results, nil
				}
			}
		}
	}

	// Sort by cost descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].TotalCostMicroCents > results[j].TotalCostMicroCents
	})

	return results, nil
}

// ===== Test Helper Methods =====

// Clear clears all data (test helper)
func (r *ExportRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.exports = make(map[string]*models.Export)
	r.exportsByUser = make(map[string][]string)
	r.costTracking = make(map[string][]*models.ExportCostTracking)
	r.costByUser = make(map[string][]*models.ExportCostTracking)
}

// Ensure ExportRepository implements interfaces.ExportRepository
var _ interfaces.ExportRepository = (*ExportRepository)(nil)
