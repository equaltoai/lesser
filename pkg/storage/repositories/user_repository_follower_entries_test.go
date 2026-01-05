package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestUserRepository_addFollowerEntries_ReturnsBaseEntriesOnError(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())
	deps := new(mockUserRepositoryDeps)
	repo.SetDependencies(deps)

	deps.On("GetFollowers", mock.Anything, "alice", 100, "").Return([]string{}, "", assert.AnError)

	baseEntry := &models.Timeline{PostID: "p1", TimelineAt: time.Now()}

	_, err := repo.createFollowerTimelineEntries(context.Background(), "alice", baseEntry)
	assert.Error(t, err)

	entries := []*models.Timeline{baseEntry}
	out := repo.addFollowerEntries(context.Background(), "alice", baseEntry, entries, zap.NewNop())
	assert.Len(t, out, 1)
	assert.Same(t, baseEntry, out[0])
}
