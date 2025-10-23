package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewActivityMetric(t *testing.T) {
	ts := time.Date(2025, 10, 22, 12, 34, 56, 789000000, time.UTC)

	record := NewActivityMetric("push_delivery_success", "admin", ts)

	assert.Equal(t, MainTableName, record.TableName())
	assert.Equal(t, "activity_metric#admin", record.PK)
	assert.Equal(t, "push_delivery_success#2025-10-22T12:34:56.789Z", record.SK)
	assert.Equal(t, "push_delivery_success", record.ActivityType)
	assert.Equal(t, "admin", record.ActorID)
	assert.Equal(t, "2025-10-22T12:34:56Z", record.Timestamp)
	assert.Equal(t, ts, record.CreatedAt)
	assert.Equal(t, "activity_metric", record.Type)
}
