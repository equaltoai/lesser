package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestRelationshipRepository_resolveLocalDomain_and_count_wrappers(t *testing.T) {
	ctx := context.Background()

	cfg := config.Get()
	prevDomain := cfg.Domain
	defer func() { cfg.Domain = prevDomain }()

	cfg.Domain = ""
	assert.Equal(t, "localhost", resolveLocalDomain())

	cfg.Domain = "https://Example.com/"
	assert.Equal(t, "example.com", resolveLocalDomain())

	logger := zap.NewNop()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	mockQuery.On("Count").Return(int64(0), errors.New("count failed")).Once()
	_, err := repo.GetFollowerCount(ctx, "alice")
	assert.Error(t, err)

	mockQuery.On("Count").Return(int64(0), errors.New("count failed")).Once()
	_, err = repo.GetFollowingCount(ctx, "alice")
	assert.Error(t, err)
}

func TestRelationshipRepository_GetMove_and_endorsements_paginated_error(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	// GetMove not found -> error
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	_, err := repo.GetMove(ctx, "alice")
	assert.Error(t, err)

	// GetMove generic error -> error
	mockQuery.On("First", mock.Anything).Return(errors.New("db failed")).Once()
	_, err = repo.GetMove(ctx, "alice")
	assert.Error(t, err)

	// GetEndorsements: social repo query error propagates and is wrapped
	mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()
	_, _, err = repo.GetEndorsements(ctx, "https://example.com/users/alice", 2, "")
	assert.Error(t, err)
}

func TestRelationshipRepository_DeleteEndorsement_error_path(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	mockQuery.On("Delete").Return(errors.New("delete failed")).Once()
	assert.Error(t, repo.DeleteEndorsement(ctx, "https://example.com/users/alice", "https://example.com/users/bob"))
}

func TestRelationshipRepository_GetMoveByTarget_warn_and_HasMovedFrom_notfound(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	// GetMoveByTarget: warning path when len(moveRecords) == defaultMoveQueryLimit
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Move)
		*out = make([]models.Move, defaultMoveQueryLimit)
	}).Once()
	moves, err := repo.GetMoveByTarget(ctx, "bob")
	assert.NoError(t, err)
	assert.Len(t, moves, defaultMoveQueryLimit)

	// HasMovedFrom: not found -> false,nil
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	exists, err := repo.HasMovedFrom(ctx, "alice", "bob")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestRelationshipRepository_GetAccountMoves_warn_path(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Move)
		*out = make([]models.Move, defaultMoveQueryLimit)
	}).Once()
	moves, err := repo.GetAccountMoves(ctx, "alice")
	assert.NoError(t, err)
	assert.Len(t, moves, defaultMoveQueryLimit)
}

func TestRelationshipRepository_IsInCollection_branches(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	found, err := repo.IsInCollection(ctx, "c", "i")
	assert.NoError(t, err)
	assert.False(t, found)

	mockQuery.On("First", mock.Anything).Return(errors.New("query failed")).Once()
	_, err = repo.IsInCollection(ctx, "c", "i")
	assert.Error(t, err)

	mockQuery.On("First", mock.Anything).Return(nil).Once()
	found, err = repo.IsInCollection(ctx, "c", "i")
	assert.NoError(t, err)
	assert.True(t, found)
}

func TestRelationshipRepository_GetCollectionItems_cursor_and_next_cursor(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.CollectionItem)
		items := make([]models.CollectionItem, 21)
		for i := range items {
			items[i].SK = "ITEM#" + string(rune('a'+i))
			items[i].ItemID = items[i].SK
		}
		*out = items
	}).Once()

	items, next, err := repo.GetCollectionItems(ctx, "c", 0, "cur")
	assert.NoError(t, err)
	assert.Len(t, items, 20)
	assert.NotEmpty(t, next)
}

func TestRelationshipRepository_CountCollectionItems_error_path(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	mockQuery.On("Count").Return(int64(0), errors.New("count failed")).Once()
	_, err := repo.CountCollectionItems(ctx, "c")
	assert.Error(t, err)

	mockQuery.On("Count").Return(int64(3), nil).Once()
	count, err := repo.CountCollectionItems(ctx, "c")
	assert.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestRelationshipRepository_GetRelationshipNote_error_path(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

	repo := NewRelationshipRepository(mockDB, "test-table", logger)

	mockQuery.On("First", mock.Anything).Return(errors.New("get note failed")).Once()
	_, err := repo.GetRelationshipNote(ctx, "alice", "bob")
	assert.Error(t, err)

	// Success branch converts AccountNote model
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.AccountNote)
		m.Username = "alice"
		m.TargetActorID = "bob"
		m.Note = "hi"
	}).Once()
	note, err := repo.GetRelationshipNote(ctx, "alice", "bob")
	assert.NoError(t, err)
	assert.Equal(t, "hi", note.Note)
}
