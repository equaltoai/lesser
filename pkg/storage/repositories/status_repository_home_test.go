package repositories

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
)

func TestSanitizeHomeTimelineLimit(t *testing.T) {
	assert.Equal(t, defaultHomeTimelinePageLimit, sanitizeHomeTimelineLimit(0))
	assert.Equal(t, defaultHomeTimelinePageLimit, sanitizeHomeTimelineLimit(-5))
	assert.Equal(t, defaultHomeTimelinePageLimit, sanitizeHomeTimelineLimit(maxHomeTimelinePageLimit+1))

	valid := 12
	assert.Equal(t, valid, sanitizeHomeTimelineLimit(valid))
}

func TestPaginateHomeTimeline(t *testing.T) {
	statuses := []models.Status{
		{StatusID: "s1", PublishedAt: time.Now()},
		{StatusID: "s2", PublishedAt: time.Now().Add(-time.Minute)},
		{StatusID: "s3", PublishedAt: time.Now().Add(-2 * time.Minute)},
	}

	result := paginateHomeTimeline(statuses, 2)
	if assert.NotNil(t, result) {
		assert.Len(t, result.Items, 2)
		assert.True(t, result.HasMore)
		assert.Equal(t, int64(len(statuses)), result.Total)
		assert.Equal(t, "s2", result.NextCursor)
	}
}

func TestPaginateHomeTimelineEmpty(t *testing.T) {
	result := paginateHomeTimeline([]models.Status{}, 5)
	if assert.NotNil(t, result) {
		assert.Empty(t, result.Items)
		assert.False(t, result.HasMore)
		assert.Zero(t, result.Total)
		assert.Empty(t, result.NextCursor)
	}
}
