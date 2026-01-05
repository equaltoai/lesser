package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestRelationshipRepository_moves_notes_and_clear_collection(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()

	repo := NewRelationshipRepository(mockDB, "test-table", logger)
	repo.EnhancedBaseRepository.SetValidationService(nil)
	repo.EnhancedBaseRepository.SetPermissionService(nil)
	repo.EnhancedBaseRepository.SetCachingService(nil)
	repo.EnhancedBaseRepository.SetEventService(nil)

	t.Run("CreateMove handles conditional failure and success", func(t *testing.T) {
		mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
		err := repo.CreateMove(ctx, &storage.Move{ID: "m1", Actor: "alice", Target: "bob", Published: time.Now()})
		assert.Error(t, err)

		mockQuery.On("Create").Return(nil).Once()
		assert.NoError(t, repo.CreateMove(ctx, &storage.Move{ID: "m2", Actor: "alice", Target: "bob", Published: time.Now()}))
	})

	t.Run("GetMove not found and success", func(t *testing.T) {
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		_, err := repo.GetMove(ctx, "alice")
		assert.Error(t, err)

		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			m := args.Get(0).(*models.Move)
			m.ID = "m1"
			m.Actor = "alice"
			m.Target = "bob"
			m.Published = time.Now()
			m.CreatedAt = time.Now()
		}).Once()
		got, err := repo.GetMove(ctx, "alice")
		assert.NoError(t, err)
		assert.Equal(t, "m1", got.ID)
	})

	t.Run("GetPendingMoves clamps limit and converts", func(t *testing.T) {
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.Move)
			*out = []models.Move{{ID: "m1", Actor: "a", Target: "b"}}
		}).Once()
		moves, err := repo.GetPendingMoves(ctx, -1)
		assert.NoError(t, err)
		assert.Len(t, moves, 1)
	})

	t.Run("GetMoveByTarget warns on limit boundary", func(t *testing.T) {
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.Move)
			records := make([]models.Move, defaultMoveQueryLimit)
			for i := range records {
				records[i] = models.Move{ID: "m", Actor: "a", Target: "b"}
			}
			*out = records
		}).Once()
		moves, err := repo.GetMoveByTarget(ctx, "bob")
		assert.NoError(t, err)
		assert.Len(t, moves, defaultMoveQueryLimit)
	})

	t.Run("HasMovedFrom handles not found and success", func(t *testing.T) {
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		ok, err := repo.HasMovedFrom(ctx, "old", "new")
		assert.NoError(t, err)
		assert.False(t, ok)

		mockQuery.On("First", mock.Anything).Return(nil).Once()
		ok, err = repo.HasMovedFrom(ctx, "old", "new")
		assert.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("UpdateMoveProgress error path", func(t *testing.T) {
		mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
		assert.Error(t, repo.UpdateMoveProgress(ctx, "a", "b", nil))
	})

	t.Run("Relationship note not found and success", func(t *testing.T) {
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		_, err := repo.GetRelationshipNote(ctx, "alice", "bob")
		assert.Error(t, err)

		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			n := args.Get(0).(*models.AccountNote)
			n.Username = "alice"
			n.TargetActorID = "bob"
			n.Note = "hello"
			n.CreatedAt = time.Now()
			n.UpdatedAt = time.Now()
		}).Once()
		note, err := repo.GetRelationshipNote(ctx, "alice", "bob")
		assert.NoError(t, err)
		assert.Equal(t, "hello", note.Note)
	})

	t.Run("ClearCollection continues on delete errors", func(t *testing.T) {
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.CollectionItem)
			*out = []models.CollectionItem{
				{Collection: "c", ItemID: "i1", ItemType: "account", AddedBy: "alice", AddedAt: time.Now(), SK: "ITEM#i1"},
				{Collection: "c", ItemID: "i2", ItemType: "account", AddedBy: "alice", AddedAt: time.Now(), SK: "ITEM#i2"},
			}
		}).Once()

		mockQuery.On("Delete").Return(errors.New("delete failed")).Once()
		mockQuery.On("Delete").Return(nil).Once()

		assert.NoError(t, repo.ClearCollection(ctx, "c"))
	})
}

func TestRelationshipRepository_endorsements_and_delegating_wrappers(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	// IsEndorsed not found and success
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	ok, err := repo.IsEndorsed(ctx, "alice", "bob")
	assert.NoError(t, err)
	assert.False(t, ok)

	mockQuery.On("First", mock.Anything).Return(nil).Once()
	ok, err = repo.IsEndorsed(ctx, "alice", "bob")
	assert.NoError(t, err)
	assert.True(t, ok)

	// CreateEndorsement branches
	pin := &storage.AccountPin{Username: "alice", PinnedActorID: "https://example.com/users/bob", PinnedUsername: "bob"}

	// Not following -> HandleCreateError(nil, ...) returns nil (backward-compatible behavior)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	assert.NoError(t, repo.CreateEndorsement(ctx, pin))

	// Following, but limit reached -> nil
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.RelationshipRecord)
		model.State = models.RelationshipAccepted
	}).Once()
	mockQuery.On("Scan", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.AccountPin)
		*out = []models.AccountPin{{}, {}, {}, {}}
	}).Once()
	assert.NoError(t, repo.CreateEndorsement(ctx, pin))

	// Following, current pins error -> error
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.RelationshipRecord)
		model.State = models.RelationshipAccepted
	}).Once()
	mockQuery.On("Scan", mock.Anything).Return(errors.New("pins query failed")).Once()
	assert.Error(t, repo.CreateEndorsement(ctx, pin))

	// DeleteEndorsement delegates to SocialRepository
	mockQuery.On("Delete").Return(nil).Once()
	assert.NoError(t, repo.DeleteEndorsement(ctx, "https://example.com/users/alice", "https://example.com/users/bob"))

	// GetEndorsements delegates to SocialRepository
	mockQuery.On("Scan", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.AccountPin)
		*out = []models.AccountPin{{Username: "alice", PinnedActorID: "bob", PinnedUsername: "bob"}}
	}).Once()
	pins, cursor, err := repo.GetEndorsements(ctx, "https://example.com/users/alice", 10, "")
	assert.NoError(t, err)
	assert.Len(t, pins, 1)
	assert.Empty(t, cursor)

	// Fallbacks for delegated wrapper calls (added after the specific expectations above)
	mockQuery.On("First", mock.Anything).Return(nil).Maybe()
	mockQuery.On("All", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Scan", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(0), nil).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()

	// Wrapper calls to block/mute repos should not panic with permissive mocks
	_ = repo.CreateBlock(ctx, "alice", "bob", "act-1")
	_ = repo.DeleteBlock(ctx, "alice", "bob")
	_ = repo.Unfollow(ctx, "alice", "bob")
	_ = repo.UnblockUser(ctx, "alice", "bob")
	_ = repo.UnmuteUser(ctx, "alice", "bob")
	_, _ = repo.IsBlocked(ctx, "alice", "bob")
	_, _ = repo.IsBlockedBidirectional(ctx, "alice", "bob")
	_, _, _ = repo.GetBlockedUsers(ctx, "alice", 10, "")
	_, _, _ = repo.GetUsersWhoBlocked(ctx, "alice", 10, "")
	_, _ = repo.GetBlock(ctx, "alice", "bob")
	_, _ = repo.CountBlockedUsers(ctx, "alice")
	_, _ = repo.CountUsersWhoBlocked(ctx, "alice")

	_ = repo.CreateMute(ctx, "alice", "bob", "act-2", false, nil)
	_ = repo.DeleteMute(ctx, "alice", "bob")
	_, _ = repo.IsMuted(ctx, "alice", "bob")
	_, _, _ = repo.GetMutedUsers(ctx, "alice", 10, "")
	_, _, _ = repo.GetUsersWhoMuted(ctx, "alice", 10, "")
	_, _ = repo.GetMute(ctx, "alice", "bob")
	_, _ = repo.CountMutedUsers(ctx, "alice")
	_, _ = repo.CountUsersWhoMuted(ctx, "alice")
}
