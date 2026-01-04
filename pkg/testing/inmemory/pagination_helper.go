package inmemory

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
