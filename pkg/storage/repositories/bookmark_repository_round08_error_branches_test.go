package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBookmarkRepository_Round08_CountUserBookmarks_ErrorPath(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Count").Return(int64(0), errors.New("count failed")).Once()

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.CountUserBookmarks(ctx, "alice")
	require.Error(t, err)
}

func TestBookmarkRepository_Round08_QueryUnlockedTimeBookmarks_CursorAndError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.New("all failed")).Once()

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	_, _, err := repo.queryUnlockedTimeBookmarks(ctx, "alice", 10, "TIME#cursor")
	require.Error(t, err)
}

func TestBookmarkRepository_Round08_CascadeDeleteUserBookmarks_BatchDeleteSuccess(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	allCalls := 0
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		allCalls++
		dest := args.Get(0).(*[]*models.Bookmark)
		if allCalls == 1 {
			*dest = []*models.Bookmark{
				{PK: buildBookmarkPK("alice"), SK: "OBJECT#o1"},
			}
			return
		}
		*dest = []*models.Bookmark{}
	}).Return(nil).Twice()

	mockQuery.On("Delete").Return(nil).Maybe()

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	require.NoError(t, repo.CascadeDeleteUserBookmarks(ctx, "alice"))
}

func TestBookmarkRepository_Round08_DynamoLookup_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("ConsistentRead").Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	mockQuery.On("First", mock.Anything).Return(errors.New("first failed")).Once()
	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.dynamoGetObjectBookmark(ctx, "alice", "status-1")
	require.Error(t, err)

	mockQuery.On("All", mock.Anything).Return(errors.New("all failed")).Once()
	_, err = repo.dynamoFindTimeBookmarkByObject(ctx, "alice", "status-1")
	require.Error(t, err)
}

func TestBookmarkRepository_Round08_BatchGetBookmarks_ExecuteError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	builder := new(mocks.MockBatchGetBuilder)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("BatchGetBuilder").Return(builder)

	builder.On("Keys", mock.Anything).Return(builder)
	builder.On("Parallel", 4).Return(builder)
	builder.On("WithRetry", mock.AnythingOfType("*core.RetryPolicy")).Return(builder)
	builder.On("OnError", mock.Anything).Return(builder)
	builder.On("Execute", mock.Anything).Return(errors.New("execute failed"))

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.batchGetBookmarks(ctx, []any{core.NewKeyPair("pk", "sk")})
	require.Error(t, err)
}

func TestBookmarkRepository_Round08_CascadeDeleteObjectBookmarks_ScanError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Return(errors.New("scan failed")).Once()

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	require.Error(t, repo.CascadeDeleteObjectBookmarks(ctx, "obj-1"))
}

func TestBookmarkRepository_Round08_CreateBookmark_GetExistingError(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()
	repo.getObjectBookmarkFn = func(context.Context, string, string) (*models.Bookmark, error) {
		return nil, errors.New("boom")
	}

	_, err := repo.CreateBookmark(ctx, "alice", "status-1")
	require.Error(t, err)
}

func TestBookmarkRepository_Round08_DeleteBookmark_ObjectLookupError(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()
	repo.getObjectBookmarkFn = func(context.Context, string, string) (*models.Bookmark, error) {
		return nil, errors.New("boom")
	}

	require.Error(t, repo.DeleteBookmark(ctx, "alice", "status-1"))
}

func TestBookmarkRepository_Round08_CreateBookmark_InvalidInputs(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()

	_, err := repo.CreateBookmark(ctx, "", "status-1")
	require.Error(t, err)

	_, err = repo.CreateBookmark(ctx, "alice", "")
	require.Error(t, err)
}

func TestBookmarkRepository_Round08_CreateBookmark_TransactionErrorPath(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()

	repo.transactWriteFn = func(context.Context, func(core.TransactionBuilder) error) error {
		return errors.New("tx failed")
	}
	_, err := repo.CreateBookmark(ctx, "alice", "status-1")
	require.Error(t, err)
}

func TestBookmarkRepository_Round08_DeleteBookmark_TimeRecordLookupErrorPath(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()

	repo.getObjectBookmarkFn = func(context.Context, string, string) (*models.Bookmark, error) {
		return &models.Bookmark{
			PK:           buildBookmarkPK("alice"),
			SK:           buildObjectSK("status-1"),
			Username:     "alice",
			ObjectID:     "status-1",
			TimeRecordSK: "",
		}, nil
	}
	repo.findTimeBookmarkFn = func(context.Context, string, string) (*models.Bookmark, error) {
		return nil, errors.New("find failed")
	}
	require.Error(t, repo.DeleteBookmark(ctx, "alice", "status-1"))
}

func TestBookmarkRepository_Round08_DeleteBookmark_DeleteNotFoundIsNoop(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()

	repo.getObjectBookmarkFn = func(context.Context, string, string) (*models.Bookmark, error) {
		return &models.Bookmark{
			PK:           buildBookmarkPK("alice"),
			SK:           buildObjectSK("status-1"),
			Username:     "alice",
			ObjectID:     "status-1",
			TimeRecordSK: "TIME#t1",
		}, nil
	}
	repo.transactWriteFn = func(context.Context, func(core.TransactionBuilder) error) error {
		return dynamormerrors.ErrItemNotFound
	}
	require.NoError(t, repo.DeleteBookmark(ctx, "alice", "status-1"))
}

func TestBookmarkRepository_Round08_RepairLegacyBookmark_ErrorPaths(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()

	legacy, err := models.NewTimeOrderedBookmark("alice", "status-1", time.Now().UTC())
	require.NoError(t, err)
	repo.store[repo.makeKey(legacy.PK, legacy.SK)] = legacy

	repo.transactWriteFn = func(context.Context, func(core.TransactionBuilder) error) error {
		return errors.New("tx failed")
	}
	_, err = repo.repairLegacyBookmark(ctx, "alice", "status-1")
	require.Error(t, err)

	repo.findTimeBookmarkFn = func(context.Context, string, string) (*models.Bookmark, error) {
		return legacy, nil
	}
	repo.transactWriteFn = repo.mockTransactWrite
	_, err = repo.repairLegacyBookmark(ctx, "alice", "")
	require.Error(t, err)
}

func TestBookmarkRepository_Round08_CreateBookmark_WriteConditionFailed_ExistingPath(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()

	// Force transaction to return a condition-failed error; then confirm it returns existing object bookmark.
	existing := &models.Bookmark{PK: buildBookmarkPK("alice"), SK: buildObjectSK("status-1"), Username: "alice", ObjectID: "status-1"}
	repo.store[repo.makeKey(existing.PK, existing.SK)] = existing
	repo.transactWriteFn = func(context.Context, func(core.TransactionBuilder) error) error {
		return dynamormerrors.ErrConditionFailed
	}

	bookmark, err := repo.CreateBookmark(ctx, "alice", "status-1")
	require.NoError(t, err)
	require.Equal(t, "status-1", bookmark.ObjectID)
}

func TestBookmarkRepository_Round08_QueryUnlockedTimeBookmarks_HasMore(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Bookmark)
		pk := buildBookmarkPK("alice")
		*dest = []models.Bookmark{
			{PK: pk, SK: "TIME#3", ObjectID: "o3", Locked: false},
			{PK: pk, SK: "TIME#2", ObjectID: "o2", Locked: false},
		}
	}).Return(nil).Once()

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	items, cursor, err := repo.queryUnlockedTimeBookmarks(ctx, "alice", 1, "")
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "TIME#3", items[0].SK)
	require.Equal(t, "TIME#3", cursor)
}

func TestBookmarkRepository_Round08_DynamoFindTimeBookmarkByObject_EmptyList(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Bookmark)
		*dest = []models.Bookmark{}
	}).Return(nil).Once()

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	found, err := repo.dynamoFindTimeBookmarkByObject(ctx, "alice", "status-1")
	require.NoError(t, err)
	require.Nil(t, found)
}

func TestBookmarkRepository_Round08_RepairLegacyBookmark_NoLegacy(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()
	repo.findTimeBookmarkFn = func(context.Context, string, string) (*models.Bookmark, error) {
		return nil, nil
	}

	got, err := repo.repairLegacyBookmark(ctx, "alice", "status-1")
	require.NoError(t, err)
	require.Nil(t, got)
}
