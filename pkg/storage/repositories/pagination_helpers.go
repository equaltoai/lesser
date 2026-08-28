package repositories

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
)

type skGetter interface {
	GetSK() string
}

func listByPKSKPrefixPaginated[T skGetter](ctx context.Context, db dynamormCore.DB, model any, pk string, skPrefix string, limit int, cursor string) ([]T, string, error) {
	if limit <= 0 {
		limit = 25
	}

	// The SK window is EXACTLY ONE key condition (issue #1500: two range
	// conditions on the same sort key both compile into the
	// KeyConditionExpression and DynamoDB rejects them). The first page keys
	// BEGINS_WITH; a cursor page keys the exclusive `>` bound and demotes
	// BEGINS_WITH to a post-read FilterExpression. The matching rows of a
	// prefix form a contiguous SK block, so the Limit-before-Filter
	// interaction only ever shortens the final page — has-more detection stays
	// exact.
	query := db.WithContext(ctx).Model(model).
		Where("PK", "=", pk).
		OrderBy("SK", "ASC")

	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		query = query.Where("SK", "BEGINS_WITH", skPrefix)
	} else {
		query = query.Where("SK", ">", cursor).Filter("SK", "BEGINS_WITH", skPrefix)
	}

	query = query.Limit(limit + 1)

	var items []T
	if err := query.All(&items); err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if err := common.ValidateSliceLength("items", items, limit); err != nil {
		nextCursor = items[limit-1].GetSK()
		items = items[:limit]
	}

	return items, nextCursor, nil
}
