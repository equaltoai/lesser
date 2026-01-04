package inmemory

import (
	"strings"

	"github.com/equaltoai/lesser/pkg/storage"
)

func listByAuthorPaginated[T any](authorID string, limit int, cursor string, keysByAuthor map[string][]string, lookup func(string) (T, bool), getSK func(T) string) ([]T, string, error) {
	authorID = strings.TrimSpace(authorID)
	if authorID == "" {
		return nil, "", storage.ErrInvalidInput
	}

	keys := keysByAuthor[authorID]
	items := collectAndSortByKey(keys, lookup, getSK)
	result, nextCursor := paginateByCursorAfterKey(items, limit, cursor, "ID#", getSK)

	return result, nextCursor, nil
}

