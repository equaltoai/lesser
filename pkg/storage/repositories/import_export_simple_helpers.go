package repositories

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// getImportExportItemsForUser is a shared helper for getting items with pagination
func getImportExportItemsForUser(
	_ context.Context,
	db core.DB,
	logger *zap.Logger,
	username string,
	limit int,
	cursor string,
	itemType string, // "export" or "import"
	isExport bool,
) (interface{}, string, error) {
	if isExport {
		var exports []*models.Export
		query := db.Model(&models.Export{}).
			Where("Username", "=", username).
			Limit(limit)

		if cursor != "" {
			query = query.Where("CreatedAt", ">", cursor)
		}

		err := query.Scan(&exports)
		if err != nil {
			logger.Error(fmt.Sprintf("failed to get %ss for user", itemType),
				zap.String("username", username),
				zap.Error(err))
			return nil, "", ErrorHandler.HandleQueryError(err, EntityExport, username)
		}

		var nextCursor string
		if len(exports) == limit {
			nextCursor = exports[len(exports)-1].CreatedAt.Format(time.RFC3339)
		}

		logger.Debug(fmt.Sprintf("retrieved %ss for user", itemType),
			zap.String("username", username),
			zap.Int("count", len(exports)))

		return exports, nextCursor, nil
	}

	var imports []*models.Import
	query := db.Model(&models.Import{}).
		Where("Username", "=", username).
		Limit(limit)

	if cursor != "" {
		query = query.Where("CreatedAt", ">", cursor)
	}

	err := query.Scan(&imports)
	if err != nil {
		logger.Error(fmt.Sprintf("failed to get %ss for user", itemType),
			zap.String("username", username),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, EntityImport, username)
	}

	var nextCursor string
	if len(imports) == limit {
		nextCursor = imports[len(imports)-1].CreatedAt.Format(time.RFC3339)
	}

	logger.Debug(fmt.Sprintf("retrieved %ss for user", itemType),
		zap.String("username", username),
		zap.Int("count", len(imports)))

	return imports, nextCursor, nil
}

// ImportExportItem interface for models that support import/export operations
type ImportExportItem interface {
	GetStatus() string
	GetCreatedAt() time.Time
}

// getImportExportItemsByStatus is a generic helper for getting items by status
func getImportExportItemsByStatus[T ImportExportItem](
	_ context.Context,
	db core.DB,
	logger *zap.Logger,
	username string,
	statuses []string,
	itemType string,
	modelPtr T,
) ([]T, error) {
	var items []T

	query := db.Model(modelPtr).
		Index("GSI1").
		Where("gsi1PK", "=", fmt.Sprintf("USER#%s", username))

	err := query.All(&items)
	if err != nil {
		logger.Error(fmt.Sprintf("failed to query %ss by GSI1", itemType),
			zap.String("username", username),
			zap.Error(err))
		if itemType == "export" {
			return nil, ErrorHandler.HandleQueryError(err, EntityExport, username)
		}
		return nil, ErrorHandler.HandleQueryError(err, EntityImport, username)
	}

	// Filter by status
	items = filterItemsByStatus(items, statuses, func(item T) string { return item.GetStatus() })

	// Sort by created at
	sort.Slice(items, func(i, j int) bool {
		return items[i].GetCreatedAt().After(items[j].GetCreatedAt())
	})

	logger.Debug(fmt.Sprintf("retrieved %ss by status", itemType),
		zap.String("username", username),
		zap.Strings("statuses", statuses),
		zap.Int("count", len(items)))

	return items, nil
}

// filterItemsByStatus is a generic helper for filtering items by status
func filterItemsByStatus[T any](items []T, statuses []string, getStatus func(T) string) []T {
	if len(statuses) == 0 {
		return items
	}

	statusMap := make(map[string]bool)
	for _, status := range statuses {
		statusMap[status] = true
	}

	filtered := make([]T, 0)
	for _, item := range items {
		if statusMap[getStatus(item)] {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// CostTrackingItem interface for cost tracking models
type CostTrackingItem interface {
	GetTimestamp() time.Time
}

// getCostsByDateRange is a generic helper for getting cost data by date range
func getCostsByDateRange[T CostTrackingItem](
	_ context.Context,
	db core.DB,
	logger *zap.Logger,
	startDate, endDate time.Time,
	limit int,
	itemType string,
	modelPtr T,
) ([]T, error) {
	costTypeUpper := fmt.Sprintf("%s_COSTS", strings.ToUpper(itemType))
	var allCosts []T

	currentDate := startDate
	for currentDate.Before(endDate) || currentDate.Equal(endDate) {
		dateStr := currentDate.Format(common.CompactDateFormat)

		var dailyCosts []T
		query := db.Model(modelPtr).
			Index("GSI2").
			Where("gsi2PK", "=", fmt.Sprintf("%s#%s", costTypeUpper, dateStr)).
			OrderBy("gsi2SK", "DESC").
			Limit(limit)

		err := query.All(&dailyCosts)
		if err != nil {
			logger.Warn(fmt.Sprintf("failed to get %s costs for date", itemType),
				zap.String("date", dateStr),
				zap.Error(err))
		} else {
			allCosts = append(allCosts, dailyCosts...)
		}

		currentDate = currentDate.AddDate(0, 0, 1)
		if len(allCosts) >= limit {
			break
		}
	}

	sort.Slice(allCosts, func(i, j int) bool {
		return allCosts[i].GetTimestamp().After(allCosts[j].GetTimestamp())
	})

	if len(allCosts) > limit {
		allCosts = allCosts[:limit]
	}

	return allCosts, nil
}

// getUserCosts is a generic helper for getting user cost data
func getUserCosts[T any](
	_ context.Context,
	db core.DB,
	logger *zap.Logger,
	username string,
	startDate, endDate time.Time,
	limit int,
	itemType string,
	modelPtr T,
) ([]T, error) {
	startSK := fmt.Sprintf("COST#%s", startDate.Format(time.RFC3339))
	endSK := fmt.Sprintf("COST#%s", endDate.Format(time.RFC3339))

	var costTrackingRecords []T

	query := db.Model(modelPtr).
		Index("GSI1").
		Where("gsi1PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("gsi1SK", ">=", startSK).
		Where("gsi1SK", "<=", endSK).
		OrderBy("gsi1SK", "DESC").
		Limit(limit)

	err := query.All(&costTrackingRecords)
	if err != nil {
		logger.Error(fmt.Sprintf("failed to get user %s costs", itemType),
			zap.String("username", username),
			zap.Error(err))
		if itemType == "export" {
			return nil, ErrorHandler.HandleQueryError(err, EntityExportCostTracking, username)
		}
		return nil, ErrorHandler.HandleQueryError(err, EntityImportCostTracking, username)
	}

	return costTrackingRecords, nil
}

// CostTrackable interface for items with cost information
type CostTrackable interface {
	CostTrackingItem
	GetTotalCostMicroCents() int64
}

// getHighCostOperations is a generic helper for getting high cost operations
func getHighCostOperations[T CostTrackable](
	ctx context.Context,
	db core.DB,
	logger *zap.Logger,
	thresholdMicroCents int64,
	startDate, endDate time.Time,
	limit int,
	itemType string,
	modelPtr T,
) ([]T, error) {
	// Get all recent costs
	allCosts, err := getCostsByDateRange(ctx, db, logger, startDate, endDate, limit*10, itemType, modelPtr)
	if err != nil {
		return nil, err
	}

	return filterHighCostOperations(allCosts, thresholdMicroCents, limit, func(c T) int64 { return c.GetTotalCostMicroCents() }), nil
}

// filterHighCostOperations is a generic helper for filtering high cost operations
func filterHighCostOperations[T any](items []T, thresholdMicroCents int64, limit int, getCost func(T) int64) []T {
	var filtered []T
	for _, item := range items {
		if getCost(item) >= thresholdMicroCents {
			filtered = append(filtered, item)
			if len(filtered) >= limit {
				break
			}
		}
	}
	return filtered
}
