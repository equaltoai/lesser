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
	fetchLimit := limit + 1
	if cursor == "" {
		query = query.Where("SK", "BEGINS_WITH", skPrefix)
	} else {
		// Close the key range at the top of the block with the `~` sentinel
		// (0x7E sorts above every ASCII block member) so a final page that
		// exhausts the block cannot keep reading the partition tail. BETWEEN is
		// inclusive, so the cursor row is re-included and dropped post-read (one
		// extra item is over-fetched so the has-more detection stays exact).
		query = query.Where("SK", "BETWEEN", []any{cursor, skPrefix + "~"}).Filter("SK", "BEGINS_WITH", skPrefix)
		fetchLimit++
	}

	query = query.Limit(fetchLimit)

	var items []T
	if err := query.All(&items); err != nil {
		return nil, "", err
	}
	if cursor != "" {
		items = dropSortKeyCursorDuplicate(items, cursor, func(item T) string { return item.GetSK() })
	}

	nextCursor := ""
	if err := common.ValidateSliceLength("items", items, limit); err != nil {
		nextCursor = items[limit-1].GetSK()
		items = items[:limit]
	}

	return items, nextCursor, nil
}
