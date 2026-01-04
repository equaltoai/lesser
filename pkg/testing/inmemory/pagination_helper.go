package inmemory

import (
	"sort"
	"strings"
)

// paginateItems paginates a slice of items based on a limit and a cursor.
// getCursor is a function that extracts the cursor value from an item.
func paginateItems[T any](items []T, limit int, cursor string, getCursor func(T) string) ([]T, string) {
	// Apply safe limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Find start index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, item := range items {
			if getCursor(item) == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []T
	var nextCursor string

	for i := startIdx; i < len(items) && len(results) < limit; i++ {
		results = append(results, items[i])
	}

	if startIdx+len(results) < len(items) && len(results) > 0 {
		nextCursor = getCursor(results[len(results)-1])
	}

	return results, nextCursor
}

func collectAndSort[T any](keys []string, lookup func(string) (T, bool), less func(a, b T) bool) []T {
	items := make([]T, 0, len(keys))
	for _, key := range keys {
		if item, ok := lookup(key); ok {
			items = append(items, item)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return less(items[i], items[j])
	})

	return items
}

func collectAndSortByKey[T any](keys []string, lookup func(string) (T, bool), getKey func(T) string) []T {
	return collectAndSort(keys, lookup, func(a, b T) bool {
		return getKey(a) < getKey(b)
	})
}

func ensurePrefix(cursor, prefix string) string {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" || prefix == "" {
		return cursor
	}
	if strings.HasPrefix(cursor, prefix) {
		return cursor
	}
	return prefix + cursor
}

func paginateByCursorAfterKey[T any](items []T, limit int, cursor string, cursorPrefix string, getKey func(T) string) ([]T, string) {
	if limit <= 0 {
		limit = 25
	}

	cursor = ensurePrefix(cursor, cursorPrefix)

	startIdx := 0
	if cursor != "" {
		found := false
		for i, item := range items {
			if getKey(item) > cursor {
				startIdx = i
				found = true
				break
			}
		}
		if !found {
			startIdx = len(items)
		}
	}

	endIdx := startIdx + limit
	if endIdx > len(items) {
		endIdx = len(items)
	}

	result := items[startIdx:endIdx]
	nextCursor := ""
	if endIdx < len(items) && len(result) > 0 {
		nextCursor = getKey(result[len(result)-1])
	}

	return result, nextCursor
}

func paginateFromIDs[T any](ids []string, limit int, cursor string, lookup func(string) (T, bool)) ([]T, string) {
	if len(ids) == 0 {
		return []T{}, ""
	}

	safeLimit := clampLimit(limit)

	startIdx := 0
	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		for i, id := range ids {
			if id == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	results := make([]T, 0, safeLimit)
	for i := startIdx; i < len(ids) && len(results) < safeLimit; i++ {
		if item, ok := lookup(ids[i]); ok {
			results = append(results, item)
		}
	}

	nextCursor := ""
	if startIdx+safeLimit < len(ids) {
		nextCursor = ids[startIdx+safeLimit-1]
	}

	return results, nextCursor
}
