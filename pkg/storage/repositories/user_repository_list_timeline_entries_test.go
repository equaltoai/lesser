package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestUserRepository_createListTimelineEntries_ReturnsErrorOnDepsFailure(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())
	deps := new(mockUserRepositoryDeps)
	repo.SetDependencies(deps)

	deps.On("GetListsContainingAccount", mock.Anything, "alice", "").Return([]*storage.List{}, assert.AnError).Once()

	_, err := repo.createListTimelineEntries(context.Background(), "alice", &models.Timeline{PostID: "p1", TimelineAt: time.Now()})
	assert.Error(t, err)
}

func TestUserRepository_createListTimelineEntries_CoversRepliesPolicies(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())
	deps := new(mockUserRepositoryDeps)
	repo.SetDependencies(deps)

	listNone := &storage.List{ID: "l1", Username: "owner1", RepliesPolicy: "none"}
	listFollowedInclude := &storage.List{ID: "l2", Username: "owner2", RepliesPolicy: "followed"}
	listFollowedExclude := &storage.List{ID: "l3", Username: "owner3", RepliesPolicy: "followed"}
	listAll := &storage.List{ID: "l4", Username: "owner4", RepliesPolicy: "list"}
	listDefault := &storage.List{ID: "l5", Username: "owner5", RepliesPolicy: "unknown"}

	deps.On("GetListsContainingAccount", mock.Anything, "alice", "").Return([]*storage.List{
		listNone,
		listFollowedInclude,
		listFollowedExclude,
		listAll,
		listDefault,
	}, nil)

	// Non-reply: every list should include.
	base := &models.Timeline{PostID: "p1", TimelineAt: time.Now()}
	entries, err := repo.createListTimelineEntries(context.Background(), "alice", base)
	assert.NoError(t, err)
	assert.Len(t, entries, 5)

	// Reply: "none" excludes; "followed" checks follower list; others include.
	replyBase := &models.Timeline{PostID: "p2", TimelineAt: time.Now(), InReplyTo: "POST#repliedTo#123"}
	deps.On("GetFollowers", mock.Anything, "repliedTo", 1000, "").Return([]string{listFollowedInclude.Username}, "", nil).Once()
	deps.On("GetFollowers", mock.Anything, "repliedTo", 1000, "").Return([]string{"someone_else"}, "", nil).Once()

	entries, err = repo.createListTimelineEntries(context.Background(), "alice", replyBase)
	assert.NoError(t, err)
	assert.Len(t, entries, 3)
}

func TestUserRepository_addListEntries_ReturnsBaseEntriesOnError(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())
	deps := new(mockUserRepositoryDeps)
	repo.SetDependencies(deps)

	deps.On("GetListsContainingAccount", mock.Anything, "alice", "").Return([]*storage.List{}, assert.AnError)

	baseEntry := &models.Timeline{PostID: "p1", TimelineAt: time.Now()}
	entries := []*models.Timeline{baseEntry}

	out := repo.addListEntries(context.Background(), "alice", baseEntry, entries, zap.NewNop())
	assert.Len(t, out, 1)
	assert.Same(t, baseEntry, out[0])
}
