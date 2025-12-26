package repositories

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
)

type skGetter interface {
	GetSK() string
}

func listByPKSKPrefixPaginated[T skGetter](ctx context.Context, db dynamormCore.DB, model any, pk string, skPrefix string, limit int, cursor string) ([]T, string, error) {
	if limit <= 0 {
		limit = 25
	}

	query := db.WithContext(ctx).Model(model).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", skPrefix).
		OrderBy("SK", "ASC")

	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		query = query.Where("SK", ">", cursor)
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
