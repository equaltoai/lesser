package models

import "time"

// sortableTimestampLayout preserves nanosecond precision with a fixed width so
// DynamoDB string sort keys remain chronological. time.RFC3339Nano is not safe
// for ordered keys because it trims trailing fractional zeros.
const sortableTimestampLayout = "2006-01-02T15:04:05.000000000Z"

func formatSortableTimestamp(value time.Time) string {
	return value.UTC().Format(sortableTimestampLayout)
}
