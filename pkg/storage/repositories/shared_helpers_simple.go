package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
)

// PaginationConfig holds common pagination parameters
type PaginationConfig struct {
	Limit  int
	Cursor string
}

// NormalizePaginationLimit ensures pagination limit is within valid bounds
func NormalizePaginationLimit(limit int) int {
	if err := common.ValidateQueryLimit(limit, 100, "pagination"); err != nil {
		return 20 // default on validation error
	}
	return limit
}

// AuditLogQueryHelper is a shared helper for audit log queries with time range
func AuditLogQueryHelper(
	ctx context.Context,
	db core.DB,
	indexName string,
	pkValue string,
	limit int,
	startTime, endTime time.Time,
	entityName string,
) ([]*models.AuthAuditLog, error) {
	var logs []*models.AuthAuditLog

	attrPrefix := strings.ToLower(indexName)
	query := db.WithContext(ctx).Model(&models.AuthAuditLog{}).
		Index(attrPrefix).
		Where(fmt.Sprintf("%sPK", attrPrefix), "=", pkValue)

	// Add time range filter if specified
	if !startTime.IsZero() && !endTime.IsZero() {
		startTimestamp := fmt.Sprintf("AUDIT#%d", startTime.Unix())
		endTimestamp := fmt.Sprintf("AUDIT#%d", endTime.Unix())
		query = query.Where(fmt.Sprintf("%sSK", attrPrefix), ">=", startTimestamp).
			Where(fmt.Sprintf("%sSK", attrPrefix), "<=", endTimestamp)
	}

	// Apply limit
	if limit > 0 {
		query = query.Limit(limit)
	}

	// Execute query
	if err := query.All(&logs); err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityAudit, fmt.Sprintf("%s audit logs", entityName))
	}

	return logs, nil
}
