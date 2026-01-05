package inmemory

import (
	"sort"
	"time"
)

func highCostItems[T any](itemsByKey map[string][]T, thresholdMicroCents int64, startDate, endDate time.Time, limit int, getCost func(T) int64, getCreatedAt func(T) time.Time) []T {
	if limit <= 0 {
		limit = 25
	}

	results := make([]T, 0, limit)
	for _, items := range itemsByKey {
		for _, item := range items {
			createdAt := getCreatedAt(item)
			if getCost(item) >= thresholdMicroCents && createdAt.After(startDate) && createdAt.Before(endDate) {
				results = append(results, item)
				if len(results) >= limit {
					sort.Slice(results, func(i, j int) bool {
						return getCost(results[i]) > getCost(results[j])
					})
					return results
				}
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return getCost(results[i]) > getCost(results[j])
	})

	return results
}

